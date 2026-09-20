package cmd

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/labring/sealtun/pkg/auth"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "List and switch Sealos workspaces (namespaces) for the current login",
	Args:  cobra.NoArgs,
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces available under the current login",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		authData, namespaces, current, err := loadWorkspaceState(cmd)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAMESPACE\tNAME\tROLE\tCURRENT")
		for _, ns := range namespaces {
			marker := ""
			if ns.ID == current || ns.UID == current {
				marker = "*"
			}
			fmt.Fprintf(tw, "%s\t%s\t%v\t%s\n", ns.ID, valueOr(ns.TeamName, "-"), ns.Role, marker)
		}
		_ = tw.Flush()
		_ = authData
		return nil
	},
}

var workspaceCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the workspace new tunnels are created in",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		authData, err := auth.LoadAuthData()
		if err != nil {
			return fmt.Errorf("not logged in. Please run 'sealtun login' first: %w", err)
		}
		kubeconfig, err := auth.ActiveKubeconfig()
		if err != nil {
			return err
		}
		namespace, err := auth.KubeconfigNamespace(kubeconfig)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Namespace: %s\n", namespace)
		if ws := authData.CurrentWorkspace; ws != nil {
			fmt.Fprintf(out, "Workspace: %s\n", valueOr(ws.TeamName, ws.ID))
		}
		return nil
	},
}

var workspaceUseCmd = &cobra.Command{
	Use:   "use <namespace-or-name>",
	Short: "Switch the workspace new tunnels are created in",
	Long: `Switch the active workspace (namespace) for the current login. Accepts a
namespace ID (ns-xxxx) or a workspace display name. Existing tunnels stay in
their original workspace; new tunnels are created in the selected one.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorkspaceUse(cmd, args[0])
	},
}

func init() {
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceListCmd, workspaceCurrentCmd, workspaceUseCmd)
}

type workspaceState struct {
	authData     *auth.AuthData
	regionalKube string
}

// ensureFreshRegionalToken returns a valid regional token, renewing the whole
// credential chain through the stored refresh token when the cached tokens
// have expired (they expire independently of the kubeconfig).
func ensureFreshRegionalToken() (*auth.AuthData, string, error) {
	authData, err := auth.LoadAuthData()
	if err != nil {
		return nil, "", fmt.Errorf("not logged in. Please run 'sealtun login' first: %w", err)
	}
	regionData, err := auth.GetRegionToken(authData.Region, authData.AccessToken)
	if err == nil {
		return authData, regionData.Data.Token, nil
	}
	refreshed, refreshErr := tryAuthRefresh()
	if refreshErr != nil || !refreshed {
		return nil, "", fmt.Errorf("login session expired; run `sealtun login %s` again: %w", authData.Region, err)
	}
	authData, err = auth.LoadAuthData()
	if err != nil {
		return nil, "", err
	}
	regionData, err = auth.GetRegionToken(authData.Region, authData.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get region token after refresh: %w", err)
	}
	return authData, regionData.Data.Token, nil
}

func loadWorkspaceState(cmd *cobra.Command) (*auth.AuthData, []auth.Namespace, string, error) {
	authData, regionalToken, err := ensureFreshRegionalToken()
	if err != nil {
		return nil, nil, "", err
	}
	nsData, err := auth.ListWorkspaces(authData.Region, regionalToken)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list workspaces: %w", err)
	}
	if nsData == nil || len(nsData.Data.Namespaces) == 0 {
		return nil, nil, "", fmt.Errorf("no workspaces found for the current login")
	}
	kubeconfig, err := auth.ActiveKubeconfig()
	if err != nil {
		return nil, nil, "", err
	}
	current, err := auth.KubeconfigNamespace(kubeconfig)
	if err != nil {
		return nil, nil, "", err
	}
	return authData, nsData.Data.Namespaces, current, nil
}

// matchWorkspace resolves a user-supplied workspace reference to exactly one
// namespace, by ID, UID, or display name (case-insensitive).
func matchWorkspace(namespaces []auth.Namespace, target string) (*auth.Namespace, error) {
	var match *auth.Namespace
	for i := range namespaces {
		ns := &namespaces[i]
		if strings.EqualFold(ns.ID, target) || strings.EqualFold(ns.TeamName, target) || strings.EqualFold(ns.UID, target) {
			if match != nil {
				return nil, fmt.Errorf("workspace %q is ambiguous; use the namespace ID (ns-xxxx)", target)
			}
			match = ns
		}
	}
	if match == nil {
		available := make([]string, 0, len(namespaces))
		for _, ns := range namespaces {
			available = append(available, ns.ID)
		}
		return nil, fmt.Errorf("workspace %q not found; available: %s", target, strings.Join(available, ", "))
	}
	return match, nil
}

func runWorkspaceUse(cmd *cobra.Command, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("workspace name is required")
	}
	authData, namespaces, current, err := loadWorkspaceState(cmd)
	if err != nil {
		return err
	}

	match, err := matchWorkspace(namespaces, target)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if match.ID == current {
		fmt.Fprintf(out, "Already on workspace %s (%s).\n", valueOr(match.TeamName, match.ID), match.ID)
		return nil
	}

	kubeconfig, err := auth.ActiveKubeconfig()
	if err != nil {
		return err
	}
	rewritten, err := auth.RewriteKubeconfigNamespace(kubeconfig, match.ID)
	if err != nil {
		return err
	}

	// Verify RBAC before persisting: the regional token must have API access
	// in the target workspace, otherwise switching would strand every command.
	if _, probeErr := workspaceProbe(cmd, rewritten, authData); probeErr != nil {
		return fmt.Errorf("cannot use workspace %s (%s): the current login has no cluster access there (%v)", valueOr(match.TeamName, match.ID), match.ID, probeErr)
	}

	renewed := *authData
	renewed.CurrentWorkspace = &auth.Workspace{UID: match.UID, ID: match.ID, TeamName: match.TeamName}
	if err := auth.SaveAuthData(renewed, rewritten); err != nil {
		return fmt.Errorf("failed to save workspace switch: %w", err)
	}
	if profileName, err := auth.CurrentProfileName(); err == nil && profileName != "" {
		_, _ = auth.SaveProfile(profileName, renewed, rewritten)
	}
	fmt.Fprintf(out, "[+] Workspace switched to %s (%s). New tunnels will be created there; existing tunnels stay in their original namespace.\n", valueOr(match.TeamName, match.ID), match.ID)
	return nil
}

func workspaceProbe(cmd *cobra.Command, kubeconfig string, authData *auth.AuthData) (bool, error) {
	if workspaceProbeFn != nil {
		return workspaceProbeFn(cmd, kubeconfig, authData)
	}
	return false, errors.New("workspace probe is not wired")
}

// workspaceProbeFn is replaced by the k8s-backed probe in workspace_probe.go.
var workspaceProbeFn func(cmd *cobra.Command, kubeconfig string, authData *auth.AuthData) (bool, error)
