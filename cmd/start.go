package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/labring/sealtun/pkg/session"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:     "start [tunnel-id]",
	Aliases: []string{"resume"},
	Short:   "Restart a stopped Sealtun tunnel",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sess, err := findSession(args[0])
		if err != nil {
			return err
		}
		if sess.Secret == "" {
			return fmt.Errorf("tunnel %s cannot be started because its local secret is unavailable; run cleanup and recreate the tunnel", sess.TunnelID)
		}
		if sessionExpired(*sess, time.Now()) {
			return fmt.Errorf("tunnel %s has expired; run cleanup and recreate the tunnel", sess.TunnelID)
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()

		if err := resumeSessionResources(ctx, *sess); err != nil {
			return fmt.Errorf("resume tunnel %s: %w", sess.TunnelID, err)
		}

		sess.Mode = "daemon"
		sess.PID = 0
		sess.ConnectionState = session.ConnectionStatePending
		sess.LastError = ""
		if err := session.Update(*sess); err != nil {
			return fmt.Errorf("update local session %s: %w", sess.TunnelID, err)
		}

		if err := ensureDaemonRunning(); err != nil {
			return err
		}
		if err := waitForDaemonSession(sess.TunnelID, daemonConnectTimeout); err != nil {
			return err
		}
		ensureSessionPublicPort(cmd.Context(), sess)

		if sess.Protocol == "ssh" && sess.PublicPort != 0 {
			endpoint := endpointDisplay(sess.Protocol, sess.Host, sess.SealosHost, sess.PublicPort)
			fmt.Fprintf(cmd.OutOrStdout(), "Started tunnel %s.\n", sess.TunnelID)
			fmt.Fprintf(cmd.OutOrStdout(), "  Public SSH host: %s\n", endpoint.Host)
			fmt.Fprintf(cmd.OutOrStdout(), "  Public SSH port: %d\n", endpoint.Port)
			fmt.Fprintf(cmd.OutOrStdout(), "  SSH command: %s\n", endpoint.Command)
			fmt.Fprintf(cmd.OutOrStdout(), "  Local target: localhost:%s\n", valueOr(sess.LocalPort, "unknown"))
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Started tunnel %s.\n", sess.TunnelID)
		fmt.Fprintf(cmd.OutOrStdout(), "  Public URL: %s\n", endpointLabel(sess.Protocol, sess.Host, sess.SealosHost, sess.PublicPort))
		fmt.Fprintf(cmd.OutOrStdout(), "  Local target: localhost:%s\n", valueOr(sess.LocalPort, "unknown"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
