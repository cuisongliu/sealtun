package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/session"
	"github.com/labring/sealtun/pkg/tunnel"
	"github.com/spf13/cobra"
)

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
