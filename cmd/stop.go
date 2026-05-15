package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/labring/sealtun/pkg/session"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [tunnel-id]",
	Short: "Stop a Sealtun tunnel while preserving its domain and remote entry resources",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sess, err := findSession(args[0])
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()

		ownerAlive := sessionOwnerAlive(*sess)
		if err := pauseSessionResources(ctx, *sess); err != nil {
			return fmt.Errorf("pause tunnel %s: %w", sess.TunnelID, err)
		}
		sess.PID = 0
		sess.ConnectionState = session.ConnectionStateStopped
		sess.LastError = ""
		if err := session.Update(*sess); err != nil {
			return fmt.Errorf("update local session %s: %w", sess.TunnelID, err)
		}

		if sess.Protocol == "ssh" {
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped SSH tunnel %s. TCP NodePort, control resources, and local session were preserved.\n", sess.TunnelID)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped tunnel %s. Domain, Service, Ingress, and local session were preserved.\n", sess.TunnelID)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Run `sealtun start %s` to reopen it, or `sealtun cleanup` to delete stopped tunnel resources.\n", sess.TunnelID)
		if ownerAlive {
			fmt.Fprintln(cmd.OutOrStdout(), "Note: the local expose process may still be running until it exits on its own.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
