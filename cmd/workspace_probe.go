package cmd

import (
	"context"
	"time"

	"github.com/labring/sealtun/pkg/auth"
	"github.com/labring/sealtun/pkg/k8s"
	"github.com/spf13/cobra"
)

func init() {
	workspaceProbeFn = probeWorkspaceAccess
}

// probeWorkspaceAccess verifies the rewritten kubeconfig actually has API
// access in the target workspace before the switch is persisted.
func probeWorkspaceAccess(cmd *cobra.Command, kubeconfig string, authData *auth.AuthData) (bool, error) {
	client, err := k8s.NewClientFromKubeconfig(kubeconfig, authData)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Second)
	defer cancel()
	if err := client.ProbeAPIServer(ctx); err != nil {
		return false, err
	}
	return true, nil
}
