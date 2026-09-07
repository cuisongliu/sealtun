package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/routes"
	"github.com/labring/sealtun/pkg/session"
	"github.com/labring/sealtun/pkg/tunnel"
	"github.com/spf13/cobra"
)

func mustAtoiStr(t *testing.T, value string) int {
	t.Helper()
	n, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFormatCompactBytes(t *testing.T) {
	cases := map[int64]string{0: "0B", 512: "512B", 2048: "2.0KB", 5 << 20: "5.0MB"}
	for in, want := range cases {
		if got := formatCompactBytes(in); got != want {
			t.Fatalf("formatCompactBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintRequestsTableShape(t *testing.T) {
	var buf bytes.Buffer
	printRequestsTable(&buf, []tunnel.RequestLogEntry{{
		Seq: 7, Time: "2026-09-07T12:00:00Z", Method: "POST", Path: "/api/users",
		Status: 201, BytesOut: 512, DurationMs: 42, ClientIP: "203.0.113.1",
	}})
	out := buf.String()
	for _, want := range []string{"TIME", "METHOD", "PATH", "STATUS", "POST", "/api/users", "201", "42ms"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
	var empty bytes.Buffer
	printRequestsTable(&empty, nil)
	if !strings.Contains(empty.String(), "No requests captured") {
		t.Fatalf("empty log should say so, got %q", empty.String())
	}
}

func TestFetchTunnelRequestsAgainstStub(t *testing.T) {
	var sawAuth string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		sawAuth = r.Header.Get("Authorization")
		if sawAuth != "Bearer s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requests": []map[string]any{{"seq": 1, "method": "GET", "path": "/health", "status": 200}},
		})
	}))
	defer server.Close()
	stubRequestsFetch(t, server)

	sess := session.TunnelSession{
		TunnelID: "t1",
		Host:     strings.TrimPrefix(server.URL, "http://"),
		Secret:   "s3cret",
	}
	payload, err := fetchTunnelRequests(t.Context(), sess, 10)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(payload.Requests) != 1 || payload.Requests[0].Path != "/health" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestFetchTunnelRequestsDetectsOldImage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // old image: no /_sealtun/requests route
	}))
	defer server.Close()
	stubRequestsFetch(t, server)

	sess := session.TunnelSession{TunnelID: "t1", Host: strings.TrimPrefix(server.URL, "http://"), Secret: "x"}
	_, err := fetchTunnelRequests(t.Context(), sess, 10)
	if err == nil || !strings.Contains(err.Error(), "does not serve the request log") {
		t.Fatalf("expected old-image guidance, got %v", err)
	}
}

func stubRequestsFetch(t *testing.T, server *httptest.Server) {
	t.Helper()
	origClient, origEndpoint := requestsHTTPClient, requestsEndpoint
	t.Cleanup(func() { requestsHTTPClient, requestsEndpoint = origClient, origEndpoint })
	requestsHTTPClient = server.Client
	requestsEndpoint = func(session.TunnelSession, int) (string, error) {
		return server.URL + "/_sealtun/requests?limit=10", nil
	}
}

func TestRunRequestsValidatesFlags(t *testing.T) {
	origLimit, origInterval := requestsLimit, requestsInterval
	t.Cleanup(func() { requestsLimit, requestsInterval = origLimit, origInterval })
	blank := &cobra.Command{}

	requestsLimit = 0
	if err := runRequests(blank, "x"); err == nil || !strings.Contains(err.Error(), "--limit") {
		t.Fatalf("expected limit validation, got %v", err)
	}
	requestsLimit = 50
	requestsInterval = 0
	if err := runRequests(blank, "x"); err == nil || !strings.Contains(err.Error(), "--interval") {
		t.Fatalf("expected interval validation, got %v", err)
	}
}

func TestReplayTargetURLRouting(t *testing.T) {
	sess := session.TunnelSession{
		TunnelID:  "t1",
		LocalPort: "3000",
		// local-port sessions always carry the default localhost TargetURL;
		// the route table must still win.
		TargetURL: "http://localhost:3000",
		Routes:    []routes.Route{{Path: "/api", Port: 8080}},
	}
	entry := &tunnel.RequestLogEntry{URI: "/api/users?x=1"}
	target, err := replayTargetURL(sess, entry)
	if err != nil || target != "http://localhost:8080/users?x=1" {
		t.Fatalf("routed replay target = %q, %v", target, err)
	}
	entry = &tunnel.RequestLogEntry{URI: "/other?y=2"}
	target, err = replayTargetURL(sess, entry)
	if err != nil || target != "http://localhost:3000/other?y=2" {
		t.Fatalf("fallback replay target = %q, %v", target, err)
	}

	upstream := session.TunnelSession{TunnelID: "t2", TargetURL: "https://10.0.0.12:8443"}
	target, err = replayTargetURL(upstream, entry)
	if err != nil || target != "https://10.0.0.12:8443/other?y=2" {
		t.Fatalf("upstream replay target = %q, %v", target, err)
	}
}

func TestReplaySkippedHeader(t *testing.T) {
	for _, h := range []string{"Host", "Content-Length", "Connection", "Upgrade"} {
		if !replaySkippedHeader(h) {
			t.Fatalf("%s must be skipped", h)
		}
	}
	if replaySkippedHeader("X-Hub-Signature-256") {
		t.Fatal("webhook signature headers must pass through")
	}
}

func TestRequestsReplayEndToEnd(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotSig, gotBody string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		gotSig = r.Header.Get("X-Hub-Signature-256")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("replayed-ok"))
	}))
	defer app.Close()
	appPort := strings.TrimPrefix(app.URL, "http://127.0.0.1:")

	stub := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"requests": []map[string]any{{
			"seq": 9, "time": "2026-09-07T00:00:00Z", "method": "POST", "path": "/api/hook?<redacted>",
			"status": 200, "uri": "/api/hook?source=test",
			"headers": map[string]string{"Authorization": "(redacted)", "X-Hub-Signature-256": "sha256=abc", "Content-Type": "application/json"},
			"body":    `{"event":"ping"}`,
		}}})
	}))
	defer stub.Close()
	stubRequestsFetch(t, stub)

	sess := session.TunnelSession{
		TunnelID: "t1",
		Secret:   "x",
		Routes:   []routes.Route{{Path: "/api", Port: mustAtoiStr(t, appPort)}},
	}
	originalFind := findSession
	findSession = func(string) (*session.TunnelSession, error) { return &sess, nil }
	t.Cleanup(func() { findSession = originalFind })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runRequestsReplay(cmd, "t1", "9"); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/hook?source=test" {
		t.Fatalf("replay dispatch wrong: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "" {
		t.Fatalf("redacted Authorization must not be replayed, got %q", gotAuth)
	}
	if gotSig != "sha256=abc" {
		t.Fatalf("signature header lost: %q", gotSig)
	}
	if gotBody != `{"event":"ping"}` {
		t.Fatalf("body mismatch: %q", gotBody)
	}
	if !strings.Contains(out.String(), "201") || !strings.Contains(out.String(), "replayed-ok") {
		t.Fatalf("response not surfaced: %q", out.String())
	}
}

func TestRequestsSessionUsable(t *testing.T) {
	stopped := &session.TunnelSession{TunnelID: "t1", Secret: "x", ConnectionState: session.ConnectionStateStopped}
	if err := requestsSessionUsable(stopped); err == nil || !strings.Contains(err.Error(), "is stopped") {
		t.Fatalf("stopped session must be rejected with guidance, got %v", err)
	}
	noSecret := &session.TunnelSession{TunnelID: "t1"}
	if err := requestsSessionUsable(noSecret); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("missing secret must be rejected, got %v", err)
	}
	ok := &session.TunnelSession{TunnelID: "t1", Secret: "x"}
	if err := requestsSessionUsable(ok); err != nil {
		t.Fatalf("usable session rejected: %v", err)
	}
}
