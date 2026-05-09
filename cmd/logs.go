package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/labring/sealtun/pkg/k8s"
	"github.com/spf13/cobra"
)

var logsTail int64
var logsSince time.Duration
var logsFollow bool

var logsCmd = &cobra.Command{
	Use:          "logs [tunnel-id]",
	Short:        "Show remote tunnel pod logs",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateLogOptions(logsTail, logsSince); err != nil {
			return err
		}
		sess, err := findSession(args[0])
		if err != nil {
			return err
		}
		client, err := k8sClientForSession(*sess)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		cancel := func() {}
		if !logsFollow {
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		}
		defer cancel()

		opts := k8s.TunnelLogOptions{
			TailLines: logsTail,
			Follow:    logsFollow,
		}
		if logsSince > 0 {
			opts.SinceSeconds = int64(logsSince.Seconds())
		}
		if err := client.WithNamespace(sess.Namespace).StreamTunnelLogs(ctx, sess.TunnelID, cmd.OutOrStdout(), opts); err != nil {
			return fmt.Errorf("stream logs for tunnel %s: %w", sess.TunnelID, err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().Int64Var(&logsTail, "tail", 100, "Number of recent log lines to show")
	logsCmd.Flags().DurationVar(&logsSince, "since", 0, "Only return logs newer than this duration, e.g. 10m")
	logsCmd.Flags().BoolVar(&logsFollow, "follow", false, "Follow log output")
}

func validateLogOptions(tail int64, since time.Duration) error {
	if tail < 0 {
		return fmt.Errorf("--tail must be greater than or equal to 0")
	}
	if since < 0 {
		return fmt.Errorf("--since must be greater than or equal to 0")
	}
	return nil
}
