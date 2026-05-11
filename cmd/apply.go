package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labring/sealtun/pkg/auth"
	"github.com/labring/sealtun/pkg/k8s"
	tunnelprotocol "github.com/labring/sealtun/pkg/protocol"
	"github.com/labring/sealtun/pkg/publicauth"
	"github.com/labring/sealtun/pkg/session"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type applyFile struct {
	Version string        `json:"version" yaml:"version"`
	Tunnels []applyTunnel `json:"tunnels" yaml:"tunnels"`
}

type applyTunnel struct {
	Name          string          `json:"name" yaml:"name"`
	LocalPort     int             `json:"localPort" yaml:"localPort"`
	Port          int             `json:"port,omitempty" yaml:"port,omitempty"`
	Protocol      string          `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Domain        string          `json:"domain,omitempty" yaml:"domain,omitempty"`
	WaitDomain    bool            `json:"waitDomain,omitempty" yaml:"waitDomain,omitempty"`
	ReadyTimeout  string          `json:"readyTimeout,omitempty" yaml:"readyTimeout,omitempty"`
	DomainTimeout string          `json:"domainTimeout,omitempty" yaml:"domainTimeout,omitempty"`
	BasicAuth     *applyBasicAuth `json:"basicAuth,omitempty" yaml:"basicAuth,omitempty"`
}

type applyBasicAuth struct {
	Credential  string `json:"credential,omitempty" yaml:"credential,omitempty"`
	Username    string `json:"username" yaml:"username"`
	Password    string `json:"password,omitempty" yaml:"password,omitempty"`
	PasswordEnv string `json:"passwordEnv,omitempty" yaml:"passwordEnv,omitempty"`
}

type applyResult struct {
	Name          string                 `json:"name"`
	TunnelID      string                 `json:"tunnelId"`
	Host          string                 `json:"host"`
	SealosHost    string                 `json:"sealosHost,omitempty"`
	CustomDomain  string                 `json:"customDomain,omitempty"`
	LocalPort     string                 `json:"localPort"`
	BasicAuth     bool                   `json:"basicAuth"`
	BasicAuthUser string                 `json:"basicAuthUser,omitempty"`
	Status        string                 `json:"status"`
	Warnings      []string               `json:"warnings,omitempty"`
	NewTunnel     bool                   `json:"-"`
	Previous      *session.TunnelSession `json:"-"`
}

type normalizedApplyTunnel struct {
	Name          string
	TunnelID      string
	LocalPort     string
	Protocol      string
	CustomDomain  string
	BasicAuth     *session.BasicAuthConfig
	BasicAuthPass string
	WaitDomain    bool
	ReadyTimeout  time.Duration
	DomainTimeout time.Duration
}

var applyFilePath string
var applyJSON bool
var applyDryRun bool

var applyNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,53}[a-z0-9])?$`)

const applyFileMaxBytes = 1 << 20

var applyCmd = &cobra.Command{
	Use:          "apply -f sealtun.yaml",
	Short:        "Apply declarative Sealtun tunnel configuration",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(applyFilePath) == "" {
			return fmt.Errorf("missing -f/--file")
		}
		results, err := runApply(cmd.Context(), applyFilePath, applyDryRun)
		if err != nil {
			return err
		}
		if applyJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}
		printApplyResults(cmd, results, applyDryRun)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().StringVarP(&applyFilePath, "file", "f", "", "Path to sealtun.yaml")
	applyCmd.Flags().BoolVar(&applyJSON, "json", false, "Output apply results as JSON")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Validate and show planned tunnels without changing local or cloud state")
}

func runApply(ctx context.Context, path string, dryRun bool) ([]applyResult, error) {
	config, err := loadApplyFile(path)
	if err != nil {
		return nil, err
	}
	if len(config.Tunnels) == 0 {
		return nil, fmt.Errorf("apply file has no tunnels")
	}
	if err := validateApplyTunnelNames(config.Tunnels); err != nil {
		return nil, err
	}
	if dryRun {
		results := make([]applyResult, 0, len(config.Tunnels))
		for _, item := range config.Tunnels {
			normalized, err := normalizeApplyTunnel(item)
			if err != nil {
				return results, err
			}
			results = append(results, applyResult{
				Name:          normalized.Name,
				TunnelID:      normalized.TunnelID,
				LocalPort:     normalized.LocalPort,
				BasicAuth:     normalized.BasicAuth != nil && normalized.BasicAuth.Enabled,
				BasicAuthUser: basicAuthUsername(normalized.BasicAuth),
				Status:        "planned",
			})
		}
		return results, nil
	}

	authData, err := auth.LoadAuthData()
	if err != nil {
		return nil, fmt.Errorf("not logged in. Please run 'sealtun login' first: %w", err)
	}
	root, err := auth.GetSealosDir()
	if err != nil {
		return nil, err
	}
	kubeconfigPath := filepath.Join(root, "kubeconfig")
	kubeconfig, err := auth.ActiveKubeconfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read kubeconfig: %w", err)
	}
	client, err := k8s.NewClient(kubeconfigPath, authData)
	if err != nil {
		return nil, fmt.Errorf("failed to init k8s client: %w", err)
	}

	results := make([]applyResult, 0, len(config.Tunnels))
	for _, item := range config.Tunnels {
		result, err := applyOneTunnel(ctx, item, authData, client, kubeconfig, dryRun)
		if err != nil {
			rollbackApplyResults(client, results)
			return results, err
		}
		results = append(results, result)
	}
	if !dryRun {
		if err := ensureDaemonRunning(); err != nil {
			rollbackApplyResults(client, results)
			return results, fmt.Errorf("failed to start local daemon: %w", err)
		}
		for _, result := range results {
			if err := waitForDaemonSession(result.TunnelID, daemonConnectTimeout); err != nil {
				rollbackApplyResults(client, results)
				return results, err
			}
		}
	}
	return results, nil
}

func loadApplyFile(path string) (*applyFile, error) {
	file, err := os.Open(path) // #nosec G304 -- apply file path is provided by the user.
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, applyFileMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > applyFileMaxBytes {
		return nil, fmt.Errorf("apply file %s is too large; limit is %d bytes", path, applyFileMaxBytes)
	}
	var config applyFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return nil, fmt.Errorf("parse %s: multiple YAML documents are not supported", path)
	}
	if config.Version == "" {
		config.Version = "v1"
	}
	if config.Version != "v1" {
		return nil, fmt.Errorf("unsupported apply file version %q", config.Version)
	}
	return &config, nil
}

func validateApplyTunnelNames(items []applyTunnel) error {
	seen := map[string]struct{}{}
	for _, item := range items {
		tunnelID, err := applyTunnelID(item.Name)
		if err != nil {
			return err
		}
		if _, ok := seen[tunnelID]; ok {
			return fmt.Errorf("duplicate tunnel name %q in apply file", item.Name)
		}
		seen[tunnelID] = struct{}{}
	}
	return nil
}

func applyOneTunnel(ctx context.Context, item applyTunnel, authData *auth.AuthData, client *k8s.Client, kubeconfig string, dryRun bool) (result applyResult, err error) {
	normalized, err := normalizeApplyTunnel(item)
	if err != nil {
		return applyResult{}, err
	}

	result = applyResult{
		Name:          normalized.Name,
		TunnelID:      normalized.TunnelID,
		LocalPort:     normalized.LocalPort,
		BasicAuth:     normalized.BasicAuth != nil && normalized.BasicAuth.Enabled,
		BasicAuthUser: basicAuthUsername(normalized.BasicAuth),
		Status:        "planned",
	}
	secret := uuid.New().String()
	createdAt := ""
	alreadyExisted := false
	var existingSession *session.TunnelSession
	if !dryRun {
		existing, err := session.Get(normalized.TunnelID)
		if err == nil {
			alreadyExisted = true
			existingSession = existing
			currentNamespace := ""
			if client != nil {
				currentNamespace = client.Namespace()
			}
			if err := validateExistingApplySessionScope(*existing, authData, currentNamespace); err != nil {
				return result, err
			}
			if existing.Secret != "" {
				secret = existing.Secret
			}
			reuseExistingBasicAuthHash(&normalized, existing.BasicAuth)
			createdAt = existing.CreatedAt
		} else if !os.IsNotExist(err) {
			return result, fmt.Errorf("tunnel %s: load existing session: %w", normalized.TunnelID, err)
		}
	}

	result.NewTunnel = !alreadyExisted
	result.Previous = existingSession
	if dryRun {
		return result, nil
	}

	desiredCustomDomain := normalized.CustomDomain
	customDomainVerified := false
	sealosHost := ""
	if existingSession != nil {
		sealosHost = sessionSealosHostForDomain(*existingSession, "")
	}
	if sealosHost == "" && client != nil {
		sealosHost = client.SealosHost(normalized.TunnelID)
	}
	if desiredCustomDomain != "" {
		if verifyErr := requireDomainCNAME(ctx, desiredCustomDomain, sealosHost); verifyErr != nil {
			if alreadyExisted {
				return result, fmt.Errorf("tunnel %s: custom domain DNS must be verified before updating an existing tunnel: %w", normalized.TunnelID, verifyErr)
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("custom domain not attached: %v", verifyErr))
			result.Warnings = append(result.Warnings, fmt.Sprintf("configure CNAME %s -> %s, then run `sealtun domain set %s %s`", desiredCustomDomain, sealosHost, normalized.TunnelID, desiredCustomDomain))
			desiredCustomDomain = ""
		} else {
			customDomainVerified = true
		}
	}
	if client == nil {
		return result, fmt.Errorf("tunnel %s: kubernetes client is unavailable", normalized.TunnelID)
	}

	remoteChanged := false
	defer func() {
		if err == nil || !remoteChanged {
			return
		}
		if result.NewTunnel {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), tunnelCleanupTimeout)
			defer cancel()
			_ = client.CleanupTunnel(cleanupCtx, normalized.TunnelID)
			_ = session.Delete(normalized.TunnelID)
			return
		}
		if existingSession != nil {
			if rollbackErr := rollbackExistingApplyTunnel(client, *existingSession); rollbackErr != nil {
				err = fmt.Errorf("%w; rollback of existing tunnel failed: %v", err, rollbackErr)
			}
		}
	}()

	options := k8s.TunnelOptions{}
	options.BasicAuth = basicAuthToK8s(normalized.BasicAuth)
	if customDomainVerified {
		options.CustomDomain = desiredCustomDomain
		options.SealosHost = sealosHost
	}
	hosts, err := client.EnsureTunnelWithOptions(ctx, normalized.TunnelID, secret, normalized.Protocol, normalized.LocalPort, options)
	if err != nil {
		if alreadyExisted && existingSession != nil {
			if rollbackErr := rollbackExistingApplyTunnel(client, *existingSession); rollbackErr != nil {
				return result, fmt.Errorf("tunnel %s: provision on Sealos: %w; rollback of existing tunnel failed: %v", normalized.TunnelID, err, rollbackErr)
			}
		}
		return result, fmt.Errorf("tunnel %s: provision on Sealos: %w", normalized.TunnelID, err)
	}
	remoteChanged = true

	if alreadyExisted && existingSession != nil && existingSession.CustomDomain != "" && desiredCustomDomain == "" {
		clearedHosts, clearErr := client.WithNamespace(client.Namespace()).ClearCustomDomain(ctx, normalized.TunnelID, hosts.SealosHost)
		if clearErr != nil {
			return result, fmt.Errorf("tunnel %s: clear custom domain: %w", normalized.TunnelID, clearErr)
		}
		hosts = clearedHosts
	}

	waitCtx, cancel := context.WithTimeout(ctx, normalized.ReadyTimeout)
	err = client.WaitForReady(waitCtx, normalized.TunnelID)
	cancel()
	if err != nil {
		return result, fmt.Errorf("tunnel %s: wait for ready: %w", normalized.TunnelID, err)
	}

	record := buildApplySessionRecord(normalized, authData, client.Namespace(), kubeconfig, secret, hosts, createdAt)
	if err := session.Save(record); err != nil {
		return result, fmt.Errorf("tunnel %s: save session: %w", normalized.TunnelID, err)
	}

	if hosts.CustomDomain != "" && normalized.WaitDomain {
		verify, waitErr := waitForDomainReady(ctx, session.TunnelSession{
			TunnelID:     normalized.TunnelID,
			Host:         hosts.PublicHost,
			SealosHost:   hosts.SealosHost,
			CustomDomain: hosts.CustomDomain,
			Namespace:    client.Namespace(),
			Kubeconfig:   kubeconfig,
			Region:       authData.Region,
		}, normalized.DomainTimeout)
		if verify != nil && !verify.Ready {
			result.Warnings = append(result.Warnings, "custom domain is not fully ready yet")
		}
		if waitErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("custom domain readiness wait failed: %v", waitErr))
		}
	}

	result.Host = hosts.PublicHost
	result.SealosHost = hosts.SealosHost
	result.CustomDomain = hosts.CustomDomain
	result.BasicAuth = normalized.BasicAuth != nil && normalized.BasicAuth.Enabled
	result.BasicAuthUser = basicAuthUsername(normalized.BasicAuth)
	result.Status = "applied"
	remoteChanged = false
	return result, nil
}

func buildApplySessionRecord(normalized normalizedApplyTunnel, authData *auth.AuthData, namespace, kubeconfig, secret string, hosts k8s.TunnelHosts, createdAt string) session.TunnelSession {
	region := ""
	if authData != nil {
		region = authData.Region
	}
	return session.TunnelSession{
		TunnelID:        normalized.TunnelID,
		Region:          region,
		Namespace:       namespace,
		Kubeconfig:      kubeconfig,
		Protocol:        normalized.Protocol,
		Host:            hosts.PublicHost,
		SealosHost:      hosts.SealosHost,
		CustomDomain:    hosts.CustomDomain,
		LocalPort:       normalized.LocalPort,
		Secret:          secret,
		BasicAuth:       normalized.BasicAuth,
		Mode:            "daemon",
		PID:             0,
		ConnectionState: session.ConnectionStatePending,
		CreatedAt:       createdAt,
		Resources:       []string{fmt.Sprintf("sealtun-%s", normalized.TunnelID)},
	}
}

func validateExistingApplySessionScope(existing session.TunnelSession, authData *auth.AuthData, currentNamespace string) error {
	currentRegion := ""
	if authData != nil {
		currentRegion = authData.Region
	}
	if existing.Region == "" || currentRegion == "" {
		return fmt.Errorf("tunnel %s already exists but region metadata is incomplete; run `sealtun inspect %s` and clean it up before apply", existing.TunnelID, existing.TunnelID)
	}
	if existing.Region != currentRegion {
		return fmt.Errorf("tunnel %s already belongs to region %s; current region is %s", existing.TunnelID, existing.Region, currentRegion)
	}
	if currentNamespace != "" {
		if existing.Namespace == "" {
			return fmt.Errorf("tunnel %s already exists but namespace metadata is incomplete; clean it up before apply", existing.TunnelID)
		}
		if existing.Namespace != currentNamespace {
			return fmt.Errorf("tunnel %s already belongs to namespace %s; current namespace is %s", existing.TunnelID, existing.Namespace, currentNamespace)
		}
	}
	if strings.TrimSpace(existing.Secret) == "" {
		return fmt.Errorf("tunnel %s already exists but its local secret is unavailable; stop or cleanup the old session before apply", existing.TunnelID)
	}
	return nil
}

func restoreExistingApplyTunnel(client *k8s.Client, previous session.TunnelSession) error {
	if client == nil {
		return nil
	}
	if strings.TrimSpace(previous.Secret) == "" || strings.TrimSpace(previous.LocalPort) == "" {
		return fmt.Errorf("previous session %s is missing secret or local port", previous.TunnelID)
	}
	protocol := previous.Protocol
	if protocol == "" {
		protocol = "https"
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), tunnelCleanupTimeout)
	defer cancel()
	_, err := client.WithNamespace(previous.Namespace).EnsureTunnelWithOptions(cleanupCtx, previous.TunnelID, previous.Secret, protocol, previous.LocalPort, k8s.TunnelOptions{
		CustomDomain: previous.CustomDomain,
		SealosHost:   previous.SealosHost,
		BasicAuth:    basicAuthToK8s(previous.BasicAuth),
	})
	return err
}

func rollbackExistingApplyTunnel(client *k8s.Client, previous session.TunnelSession) error {
	var firstErr error
	if err := restoreExistingApplyTunnel(client, previous); err != nil {
		firstErr = err
	}
	if err := session.Save(previous); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func rollbackApplyResults(client *k8s.Client, results []applyResult) {
	for i := len(results) - 1; i >= 0; i-- {
		result := results[i]
		if result.TunnelID == "" {
			continue
		}
		if result.NewTunnel {
			if client != nil {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), tunnelCleanupTimeout)
				_ = client.CleanupTunnel(cleanupCtx, result.TunnelID)
				cancel()
			}
			_ = session.Delete(result.TunnelID)
			continue
		}
		if result.Previous != nil {
			_ = rollbackExistingApplyTunnel(client, *result.Previous)
		}
	}
}

func normalizeApplyTunnel(item applyTunnel) (normalizedApplyTunnel, error) {
	tunnelID, err := applyTunnelID(item.Name)
	if err != nil {
		return normalizedApplyTunnel{}, err
	}
	port := item.LocalPort
	if port == 0 {
		port = item.Port
	}
	localPort := strconv.Itoa(port)
	if err := validateLocalPort(localPort); err != nil {
		return normalizedApplyTunnel{}, fmt.Errorf("tunnel %s: %w", tunnelID, err)
	}
	protocol := item.Protocol
	if protocol == "" {
		protocol = "https"
	}
	if err := validateProtocol(protocol); err != nil {
		return normalizedApplyTunnel{}, fmt.Errorf("tunnel %s: %w", tunnelID, err)
	}
	customDomain, err := validateCustomDomain(item.Domain)
	if err != nil {
		return normalizedApplyTunnel{}, fmt.Errorf("tunnel %s: %w", tunnelID, err)
	}
	effectiveReadyTimeout, err := parseApplyDuration(item.ReadyTimeout, readyTimeout)
	if err != nil {
		return normalizedApplyTunnel{}, fmt.Errorf("tunnel %s readyTimeout: %w", tunnelID, err)
	}
	effectiveDomainTimeout, err := parseApplyDuration(item.DomainTimeout, domainWaitTimeout)
	if err != nil {
		return normalizedApplyTunnel{}, fmt.Errorf("tunnel %s domainTimeout: %w", tunnelID, err)
	}
	basicAuth, basicAuthPass, err := resolveApplyBasicAuth(item.BasicAuth)
	if err != nil {
		return normalizedApplyTunnel{}, fmt.Errorf("tunnel %s: %w", tunnelID, err)
	}
	return normalizedApplyTunnel{
		Name:          item.Name,
		TunnelID:      tunnelID,
		LocalPort:     localPort,
		Protocol:      tunnelprotocol.Normalize(protocol),
		CustomDomain:  customDomain,
		BasicAuth:     basicAuth,
		BasicAuthPass: basicAuthPass,
		WaitDomain:    item.WaitDomain,
		ReadyTimeout:  effectiveReadyTimeout,
		DomainTimeout: effectiveDomainTimeout,
	}, nil
}

func resolveApplyBasicAuth(config *applyBasicAuth) (*session.BasicAuthConfig, string, error) {
	if config == nil {
		return nil, "", nil
	}
	input := basicAuthInput{
		Credential:  config.Credential,
		Username:    config.Username,
		Password:    config.Password,
		PasswordEnv: config.PasswordEnv,
	}
	username, password, ok, err := resolveBasicAuthCredentials(input, os.Getenv)
	if err != nil || !ok {
		return nil, "", err
	}
	basicAuth, err := newSessionBasicAuth(username, password)
	if err != nil {
		return nil, "", err
	}
	return basicAuth, password, nil
}

func reuseExistingBasicAuthHash(normalized *normalizedApplyTunnel, existing *session.BasicAuthConfig) {
	if normalized == nil || normalized.BasicAuth == nil || existing == nil || !existing.Enabled {
		return
	}
	existingHash := basicAuthPasswordHash(existing)
	if existingHash == "" || normalized.BasicAuthPass == "" || existing.Username != normalized.BasicAuth.Username {
		return
	}
	if !publicauth.Check(publicauth.BasicAuth{Username: existing.Username, PasswordHash: existingHash}, normalized.BasicAuth.Username, normalized.BasicAuthPass) {
		return
	}
	if existing.PasswordHash == "" {
		normalized.BasicAuth.PasswordSHA256 = ""
		return
	}
	normalized.BasicAuth.PasswordHash = existingHash
	normalized.BasicAuth.PasswordSHA256 = ""
}

func basicAuthUsername(config *session.BasicAuthConfig) string {
	if config == nil || !config.Enabled {
		return ""
	}
	return config.Username
}

func applyTunnelID(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("tunnel name is required")
	}
	if name != strings.ToLower(name) || !applyNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid tunnel name %q: use lowercase DNS-compatible names, e.g. web or api-dev", name)
	}
	return name, nil
}

func parseApplyDuration(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	return duration, nil
}

func printApplyResults(cmd *cobra.Command, results []applyResult, dryRun bool) {
	out := cmd.OutOrStdout()
	if dryRun {
		fmt.Fprintln(out, "Sealtun Apply Plan")
	} else {
		fmt.Fprintln(out, "Sealtun Apply Results")
	}
	for _, result := range results {
		fmt.Fprintf(out, "  - %s (%s): %s localhost:%s", result.Name, result.TunnelID, result.Status, result.LocalPort)
		if result.Host != "" {
			fmt.Fprintf(out, " -> https://%s", result.Host)
		}
		fmt.Fprintln(out)
		if result.SealosHost != "" {
			fmt.Fprintf(out, "    Sealos host: %s\n", result.SealosHost)
		}
		if result.CustomDomain != "" {
			fmt.Fprintf(out, "    Custom domain: %s\n", result.CustomDomain)
		}
		if result.BasicAuth {
			fmt.Fprintf(out, "    Basic Auth: enabled")
			if result.BasicAuthUser != "" {
				fmt.Fprintf(out, " (user: %s)", result.BasicAuthUser)
			}
			fmt.Fprintln(out)
		}
		for _, warning := range result.Warnings {
			fmt.Fprintf(out, "    Warning: %s\n", warning)
		}
	}
}
