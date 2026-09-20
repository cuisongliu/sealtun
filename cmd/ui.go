package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var uiPort int
var uiNoOpen bool

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the local Sealtun web console in your browser",
	Long: `Starts a local web console on 127.0.0.1 and opens it in your browser. The
server is reachable only from this machine and every API call requires the
session token printed in the URL, so other local processes cannot drive it.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUI(cmd)
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.Flags().IntVar(&uiPort, "port", 0, "Port to listen on (0 picks a random free port)")
	uiCmd.Flags().BoolVar(&uiNoOpen, "no-open", false, "Do not open the browser automatically")
}

const uiSessionTokenBytes = 16

func runUI(cmd *cobra.Command) error {
	token, err := newUISessionToken()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if uiPort == 0 {
			return err
		}
		listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", uiPort))
		if err != nil {
			return err
		}
	}

	mux := newUIMux(newCLIBackend(), token)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", listener.Addr().(*net.TCPAddr).Port, token)
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Sealtun web console: %s\n", url)
	fmt.Fprintln(out, "Press Ctrl+C to stop.")
	if !uiNoOpen {
		openBrowser(url)
	}

	go func() {
		<-cmd.Context().Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func newUISessionToken() (string, error) {
	buf := make([]byte, uiSessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
