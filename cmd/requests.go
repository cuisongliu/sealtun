package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"crypto/tls"

	"github.com/labring/sealtun/pkg/routes"
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
	if err := requestsSessionUsable(sess); err != nil {
		return err
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
		return nil, fmt.Errorf("request log is unreachable (the tunnel pod may still be starting or the tunnel is stopped): %w", err)
	}
	_ = probeResp.Body.Close()
	if probeResp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("remote tunnel image does not serve the request log yet; recreate the tunnel with this Sealtun version (if the tunnel is stopped, run `sealtun start` first)")
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

var requestsReplayCmd = &cobra.Command{
	Use:   "replay <tunnel-id> <seq>",
	Short: "Replay a captured request against the local service",
	Long: `Re-sends one captured request to the local service the tunnel forwards to,
following the tunnel's route table for path-prefix dispatch. Redacted headers
(Authorization, Cookie) are not sent; webhook signature headers pass through.
The response status and body are printed for inspection.`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRequestsReplay(cmd, args[0], args[1])
	},
}

func init() {
	requestsCmd.AddCommand(requestsReplayCmd)
}

func runRequestsReplay(cmd *cobra.Command, tunnelID, seqText string) error {
	seq, err := strconv.ParseInt(seqText, 10, 64)
	if err != nil || seq < 1 {
		return fmt.Errorf("seq must be a positive integer from `sealtun requests %s`", tunnelID)
	}
	sess, err := findSession(tunnelID)
	if err != nil {
		return err
	}
	if err := requestsSessionUsable(sess); err != nil {
		return err
	}
	payload, err := fetchTunnelRequests(cmd.Context(), *sess, 200)
	if err != nil {
		return err
	}
	var entry *tunnel.RequestLogEntry
	for i := range payload.Requests {
		if payload.Requests[i].Seq == seq {
			entry = &payload.Requests[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("request #%d is no longer in the log; the ring buffer keeps the latest %d entries", seq, 200)
	}

	target, err := replayTargetURL(*sess, entry)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if entry.BodyOmitted {
		fmt.Fprintf(out, "[!] Request #%d body was not stored (binary or over 32KiB); replaying without it.\n", seq)
	}
	fmt.Fprintf(out, "[+] Replaying %s %s -> %s\n", entry.Method, entry.Path, target)

	req, err := http.NewRequestWithContext(cmd.Context(), entry.Method, target, strings.NewReader(entry.Body))
	if err != nil {
		return err
	}
	for name, value := range entry.Headers {
		if value == "(redacted)" || replaySkippedHeader(name) {
			continue
		}
		req.Header.Set(name, value)
	}
	req.Header.Set("X-Sealtun-Replayed-At", entry.Time)

	resp, err := replayHTTPClient(*sess).Do(req)
	if err != nil {
		return fmt.Errorf("replay request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	fmt.Fprintf(out, "[+] Response: %s\n", resp.Status)
	if len(body) > 0 {
		fmt.Fprintf(out, "%s\n", body)
	}
	return nil
}

// replayTargetURL resolves where a captured request should be re-sent,
// mirroring the tunnel's live dispatch: route match (with prefix stripping)
// or the primary target.
func replayTargetURL(sess session.TunnelSession, entry *tunnel.RequestLogEntry) (string, error) {
	uri := entry.URI
	if uri == "" {
		uri = entry.Path
	}
	// Routes win over TargetURL because local-port sessions always carry a
	// default localhost TargetURL; checking it first would bypass the route
	// table entirely. Route and explicit --target upstreams are mutually
	// exclusive by validation, so a matched route is always authoritative.
	if route, ok := routes.MatchRoute(sess.Routes, requestURIPath(uri)); ok {
		return "http://localhost:" + strconv.Itoa(route.Port) + routes.StripPrefix(route.Path, requestURIPath(uri)) + requestURIQuery(uri), nil
	}
	if strings.TrimSpace(sess.TargetURL) != "" {
		return strings.TrimRight(sess.TargetURL, "/") + uri, nil
	}
	if sess.LocalPort == "" {
		return "", fmt.Errorf("session %s has no local port to replay against", sess.TunnelID)
	}
	return "http://localhost:" + sess.LocalPort + uri, nil
}

func requestURIPath(uri string) string {
	if i := strings.Index(uri, "?"); i >= 0 {
		return uri[:i]
	}
	return uri
}

func requestURIQuery(uri string) string {
	if i := strings.Index(uri, "?"); i >= 0 {
		return uri[i:]
	}
	return ""
}

// replaySkippedHeader lists hop-by-hop and transport-managed headers that the
// replay client must set itself.
func replaySkippedHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Host", "Content-Length", "Connection", "Upgrade", "X-Sealtun-Replayed-At":
		return true
	}
	return false
}

// replayHTTPClient mirrors the session's target TLS behavior when replaying
// against an https upstream target.
func replayHTTPClient(sess session.TunnelSession) *http.Client {
	client := &http.Client{Timeout: 15 * time.Second}
	if targetTLSInsecureSkipVerifyEnabled(sess.TargetTLS) {
		client.Transport = &http.Transport{TLSClientConfig: replayTLSConfig()}
	}
	return client
}

func replayTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- mirrors the session's explicit per-target TLS setting for private upstreams.
}

// requestsSessionUsable blocks misleading remote errors with upfront local
// state checks: the request log lives in the relay pod, so a stopped tunnel
// or a session without a secret can never serve it.
func requestsSessionUsable(sess *session.TunnelSession) error {
	if sess.ConnectionState == session.ConnectionStateStopped {
		return fmt.Errorf("tunnel %s is stopped; run `sealtun start %s` before reading the request log", sess.TunnelID, sess.TunnelID)
	}
	if sess.Secret == "" {
		return fmt.Errorf("session secret for %s is unavailable; the request log requires a tunnel created with this Sealtun version", sess.TunnelID)
	}
	return nil
}
