package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/labring/sealtun/pkg/session"
	"github.com/labring/sealtun/pkg/tunnel"
	"github.com/spf13/cobra"
)

var requestsFollow bool
var requestsJSON bool
var requestsLimit int
var requestsInterval time.Duration

var requestsCmd = &cobra.Command{
	Use:   "requests <tunnel-id>",
	Short: "Show recent public HTTP requests captured by the tunnel",
	Long: `Streams the tunnel's recent public HTTP requests (method, path, status, duration,
headers, and body preview) from the relay pod's in-memory ring buffer. The log
lives only inside your own namespace and is served over the tunnel secret.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRequests(cmd, args[0])
	},
}

func init() {
	rootCmd.AddCommand(requestsCmd)
	requestsCmd.Flags().BoolVarP(&requestsFollow, "follow", "f", false, "Keep polling and print new requests as they arrive")
	requestsCmd.Flags().BoolVar(&requestsJSON, "json", false, "Print the raw request log payload as JSON")
	requestsCmd.Flags().IntVar(&requestsLimit, "limit", 50, "Maximum number of recent requests to show (1-200)")
	requestsCmd.Flags().DurationVar(&requestsInterval, "interval", 2*time.Second, "Polling interval for --follow (1s-30s)")
}

type requestsPayload struct {
	Requests []tunnel.RequestLogEntry `json:"requests"`
}

func runRequests(cmd *cobra.Command, tunnelID string) error {
	if requestsLimit < 1 || requestsLimit > 200 {
		return fmt.Errorf("--limit must be between 1 and 200")
	}
	if requestsInterval < time.Second || requestsInterval > 30*time.Second {
		return fmt.Errorf("--interval must be between 1s and 30s")
	}
	sess, err := findSession(tunnelID)
	if err != nil {
		return err
	}
	if sess.Secret == "" {
		return fmt.Errorf("session secret for %s is unavailable; the request log requires a tunnel created with this Sealtun version", tunnelID)
	}

	out := cmd.OutOrStdout()
	if !requestsFollow {
		payload, err := fetchTunnelRequests(cmd.Context(), *sess, requestsLimit)
		if err != nil {
			return err
		}
		if requestsJSON {
			return printRequestsJSON(out, payload)
		}
		printRequestsTable(out, payload.Requests)
		return nil
	}

	fmt.Fprintf(out, "Following requests for %s (Ctrl+C to stop)...\n", sess.TunnelID)
	var lastSeq int64
	for {
		payload, err := fetchTunnelRequests(cmd.Context(), *sess, 200)
		if err != nil {
			return err
		}
		newEntries := make([]tunnel.RequestLogEntry, 0, len(payload.Requests))
		for _, entry := range payload.Requests {
			if entry.Seq > lastSeq {
				newEntries = append(newEntries, entry)
			}
		}
		for i := len(newEntries) - 1; i >= 0; i-- {
			entry := newEntries[i]
			if entry.Seq > lastSeq {
				lastSeq = entry.Seq
			}
			if requestsJSON {
				if err := printRequestsJSON(out, &requestsPayload{Requests: []tunnel.RequestLogEntry{entry}}); err != nil {
					return err
				}
			} else {
				printRequestsTableRow(tabwriter.NewWriter(out, 0, 4, 2, ' ', 0), entry)
			}
		}
		select {
		case <-cmd.Context().Done():
			return nil
		case <-time.After(requestsInterval):
		}
	}
}

// Injectable for tests: production resolves the tunnel's public host over
// HTTPS; tests substitute a local stub server.
var requestsHTTPClient = func() *http.Client { return newMetricsHTTPClient() }

var requestsEndpoint = func(sess session.TunnelSession, limit int) (string, error) {
	host, err := normalizePublicHostname(sessionControlHost(sess))
	if err != nil {
		return "", fmt.Errorf("invalid session requests host: %w", err)
	}
	return (&url.URL{Scheme: "https", Host: host, Path: "/_sealtun/requests", RawQuery: url.Values{
		"limit": {strconv.Itoa(limit)},
	}.Encode()}).String(), nil
}

func fetchTunnelRequests(ctx context.Context, sess session.TunnelSession, limit int) (*requestsPayload, error) {
	endpoint, err := requestsEndpoint(sess, limit)
	if err != nil {
		return nil, err
	}
	client := requestsHTTPClient()

	probeReq, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil) // #nosec G107 -- host is validated as a DNS hostname before constructing the URL.
	if err != nil {
		return nil, err
	}
	probeResp, err := client.Do(probeReq)
	if err != nil {
		return nil, err
	}
	_ = probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("remote tunnel image does not serve the request log yet; recreate the tunnel with this Sealtun version")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil) // #nosec G107 -- host is validated as a DNS hostname before constructing the URL.
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sess.Secret)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requests endpoint returned %s", resp.Status)
	}
	var payload requestsPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode requests payload: %w", err)
	}
	return &payload, nil
}

func printRequestsJSON(out io.Writer, payload *requestsPayload) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printRequestsTable(out io.Writer, entries []tunnel.RequestLogEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(out, "No requests captured yet.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tMETHOD\tPATH\tSTATUS\tBYTES\tDURATION\tCLIENT IP")
	for _, entry := range entries {
		printRequestsTableRow(tw, entry)
	}
	_ = tw.Flush()
}

func printRequestsTableRow(tw *tabwriter.Writer, entry tunnel.RequestLogEntry) {
	fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
		entry.Time,
		entry.Method,
		entry.Path,
		entry.Status,
		formatCompactBytes(entry.BytesOut),
		formatRequestDuration(entry.DurationMs),
		valueOr(entry.ClientIP, "-"),
	)
	_ = tw.Flush()
}

func formatCompactBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func formatRequestDuration(ms int64) string {
	if ms >= 1000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%dms", ms)
}
