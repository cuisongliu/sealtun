package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labring/sealtun/pkg/auth"
	"github.com/labring/sealtun/pkg/k8s"
	"github.com/labring/sealtun/pkg/session"
	"github.com/spf13/cobra"
)

// clusterAuthCommands are the top-level commands that contact the cluster API
// on every run; their pre-run probe verifies credentials still work and
// refreshes them when the cluster says otherwise. Purely local, login, and
// internal commands are excluded so they never pay the probe latency.
var clusterAuthCommands = map[string]bool{
	"expose": true, "up": true, "apply": true,
	"start": true, "stop": true, "cleanup": true, "rotate": true,
	"share": true, "domain": true, "policy": true,
	"inspect": true, "doctor": true, "logs": true,
}

func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		refreshCredentialsIfClusterRejects(cmd)
		return nil
	}
}

// refreshCredentialsIfClusterRejects is a best-effort pre-flight: it probes
// the cluster API once and, only on a definitive credential failure (x509 or
// 401), attempts a refresh-token renewal before the command does any real
// work. It never fails the command: every other error is left for the
// command's own error path to report, keeping today's behavior unchanged.
func refreshCredentialsIfClusterRejects(cmd *cobra.Command) {
	if !clusterAuthCommands[topLevelCommandName(cmd)] {
		return
	}
	authData, err := auth.LoadAuthData()
	if err != nil || strings.TrimSpace(authData.RefreshToken) == "" {
		return
	}
	root, err := auth.GetSealosDir()
	if err != nil {
		return
	}
	client, err := k8s.NewClient(filepath.Join(root, "kubeconfig"), authData)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 4*time.Second)
	defer cancel()
	if err := client.ProbeAPIServer(ctx); err == nil || !isClusterAuthFailure(err) {
		return
	}
	refreshed, err := tryAuthRefresh()
	if err != nil || !refreshed {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "[+] Credentials were renewed automatically via the stored refresh token.")
}

func topLevelCommandName(cmd *cobra.Command) string {
	for cmd.Parent() != nil && cmd.Parent().Parent() != nil {
		cmd = cmd.Parent()
	}
	return cmd.Name()
}

// isClusterAuthFailure matches only definitive credential rejections from the
// cluster API — never transient network or DNS errors, which must not trigger
// a refresh (and must not be hidden by one either).
func isClusterAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "x509: certificate signed by unknown authority"):
		return true
	case strings.Contains(msg, "tls: failed to verify certificate"):
		return true
	case strings.Contains(msg, "Unauthorized") || strings.Contains(msg, "401"):
		return true
	case strings.Contains(msg, "token expired") || strings.Contains(msg, "expired token"):
		return true
	}
	return false
}

// tryAuthRefresh renews the access token, regional token, and kubeconfig
// using the stored refresh token. A cross-process lock makes concurrent
// sealtun invocations single-flight so a rotated refresh token cannot be
// consumed twice. It reports (false, nil) whenever renewal is impossible —
// the caller then proceeds to the command's normal error path, which today
// means "please run sealtun login again".
func tryAuthRefresh() (bool, error) {
	root, err := auth.GetSealosDir()
	if err != nil {
		return false, err
	}
	lockFile, err := os.OpenFile(filepath.Join(root, "auth-refresh.lock"), os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed path under the user-owned config directory.
	if err != nil {
		return false, err
	}
	defer lockFile.Close()
	release, err := session.LockFileForExternalUse(lockFile, 10*time.Second)
	if err != nil {
		// Another process is refreshing right now; let this command proceed and
		// rely on its own error path rather than queueing behind the lock.
		return false, nil
	}
	defer release()

	authData, err := auth.LoadAuthData()
	if err != nil {
		return false, err
	}
	refreshToken := strings.TrimSpace(authData.RefreshToken)
	if refreshToken == "" {
		return false, nil
	}

	tokenRes, err := auth.RefreshTokens(authData.Region, refreshToken)
	if err != nil {
		// A rejected refresh token cannot be repaired locally; every other
		// failure is transient and must not destroy the working credentials.
		return false, nil
	}
	regionData, err := auth.GetRegionToken(authData.Region, tokenRes.AccessToken)
	if err != nil {
		return false, nil
	}
	if strings.TrimSpace(regionData.Data.Kubeconfig) == "" {
		return false, nil
	}
	kubeconfig := regionData.Data.Kubeconfig

	renewed := *authData
	renewed.AccessToken = tokenRes.AccessToken
	if strings.TrimSpace(tokenRes.RefreshToken) != "" {
		renewed.RefreshToken = tokenRes.RefreshToken
	}
	renewed.RegionalToken = regionData.Data.Token
	renewed.AuthenticatedAt = time.Now().Format(time.RFC3339)

	if err := auth.SaveAuthData(renewed, kubeconfig); err != nil {
		return false, fmt.Errorf("save renewed credentials: %w", err)
	}
	// Keep the named profile's stored copy in sync so a later `profile use`
	// does not resurrect the stale credentials this refresh just replaced.
	if profileName, err := auth.CurrentProfileName(); err == nil && profileName != "" {
		_, _ = auth.SaveProfile(profileName, renewed, kubeconfig)
	}
	return true, nil
}
