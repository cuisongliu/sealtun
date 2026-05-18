package tunnel

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/accesspolicy"
	"github.com/labring/sealtun/pkg/publicauth"
)

func TestServerUnavailablePageWhenClientDisconnected(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "3000")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 status, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sealtun Tunnel Status") {
		t.Fatal("missing fallback page shell")
	}
	if !strings.Contains(body, "localhost:3000") {
		t.Fatal("fallback page should include expected local port")
	}
}

func TestServerHealthzReflectsDisconnectedClient(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "3000")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/healthz", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 status, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"clientConnected":false`) {
		t.Fatalf("unexpected health body: %s", body)
	}
	if !strings.Contains(body, `"protocol":"https"`) {
		t.Fatalf("health body should include protocol: %s", body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", got)
	}
}

func TestServerHealthzRejectsUnsupportedMethods(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "3000")
	req := httptest.NewRequest(http.MethodPost, "https://example.test/_sealtun/healthz", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 status, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("expected Allow GET, HEAD header, got %q", got)
	}
}

func TestServerMetricsCountsPublicTraffic(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "3000")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/app", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer secret")
	metricsRec := httptest.NewRecorder()
	server.ServeHTTP(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", metricsRec.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(metricsRec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["totalRequests"].(float64) != 1 {
		t.Fatalf("expected one counted public request, got %v", payload["totalRequests"])
	}
	if payload["total5xx"].(float64) != 1 {
		t.Fatalf("expected one 5xx request, got %v", payload["total5xx"])
	}
	if payload["lastStatus"].(float64) != http.StatusBadGateway {
		t.Fatalf("expected last status 502, got %v", payload["lastStatus"])
	}
}

func TestServerMetricsIncludesRawTCPCounters(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "tcp", "5432")
	server.totalTCPConnections.Store(2)
	server.activeTCPConnections.Store(1)
	server.totalTCPBytes.Store(128)
	server.totalTCPErrors.Store(1)
	server.lastTCPConnectedAt.Store(1779098400)

	metricsReq := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer secret")
	metricsRec := httptest.NewRecorder()
	server.ServeHTTP(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", metricsRec.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(metricsRec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["totalTCPConnections"].(float64) != 2 {
		t.Fatalf("expected tcp connection counter, got %v", payload["totalTCPConnections"])
	}
	if payload["activeTCPConnections"].(float64) != 1 {
		t.Fatalf("expected active tcp connection counter, got %v", payload["activeTCPConnections"])
	}
	if payload["totalTCPBytes"].(float64) != 128 {
		t.Fatalf("expected tcp bytes counter, got %v", payload["totalTCPBytes"])
	}
	if payload["totalTCPErrors"].(float64) != 1 {
		t.Fatalf("expected tcp errors counter, got %v", payload["totalTCPErrors"])
	}
	if payload["lastTCPConnectedAt"] == "" {
		t.Fatalf("expected last tcp connected timestamp, got %#v", payload)
	}
}

func TestExpectedRelayClose(t *testing.T) {
	t.Parallel()

	if !expectedRelayClose(io.EOF) {
		t.Fatal("io.EOF should be treated as a normal relay close")
	}
	if !expectedRelayClose(net.ErrClosed) {
		t.Fatal("net.ErrClosed should be treated as a normal relay close")
	}
	if !expectedRelayClose(assertErr("use of closed network connection")) {
		t.Fatal("closed network connection should be treated as a normal relay close")
	}
	if expectedRelayClose(assertErr("permission denied")) {
		t.Fatal("unexpected relay errors should not be treated as normal closes")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestServerMetricsRequiresAuthorization(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "3000")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/metrics", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 status, got %d", rec.Code)
	}
}

func TestServerMetricsRejectsUnsupportedMethods(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "3000")
	req := httptest.NewRequest(http.MethodPost, "https://example.test/_sealtun/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 status, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("expected Allow GET, HEAD header, got %q", got)
	}
}

func TestServerTCPEndpointRequiresAuthorization(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "22")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/tcp", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 status, got %d", rec.Code)
	}
}

func TestServerTCPEndpointRequiresConnectedClient(t *testing.T) {
	t.Parallel()

	server := NewServer("secret", 8080, "https", "22")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/tcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 status, got %d", rec.Code)
	}
}

func TestServerBasicAuthProtectsPublicTrafficOnly(t *testing.T) {
	t.Parallel()

	basicAuth, err := publicauth.NewBasicAuth("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServerWithOptions("tunnel-secret", 8080, "https", "3000", ServerOptions{BasicAuth: basicAuth})

	publicReq := httptest.NewRequest(http.MethodGet, "https://example.test/app", nil)
	publicRec := httptest.NewRecorder()
	server.ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected public traffic to require basic auth, got %d", publicRec.Code)
	}
	if got := publicRec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Basic") {
		t.Fatalf("expected Basic challenge header, got %q", got)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "https://example.test/_sealtun/healthz", nil)
	healthRec := httptest.NewRecorder()
	server.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("health endpoint should not require basic auth, got %d", healthRec.Code)
	}
}

func TestServerBasicAuthAcceptsMatchingCredentials(t *testing.T) {
	t.Parallel()

	basicAuth, err := publicauth.NewBasicAuth("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServerWithOptions("tunnel-secret", 8080, "https", "3000", ServerOptions{BasicAuth: basicAuth})
	req := httptest.NewRequest(http.MethodGet, "https://example.test/app", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected authenticated request to reach proxy path, got %d", rec.Code)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("basic auth header should be consumed before proxying, got %q", got)
	}
}

func TestServerBearerTokenProtectsPublicTraffic(t *testing.T) {
	t.Parallel()

	hash, err := accesspolicy.HashToken("access-token")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServerWithOptions("tunnel-secret", 8080, "https", "3000", ServerOptions{
		AccessPolicy: &accesspolicy.Policy{BearerTokenHashes: []string{hash}},
	})

	unauthorized := httptest.NewRecorder()
	server.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "https://example.test/app", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing bearer token to be rejected, got %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.test/app", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected bearer-authenticated request to reach proxy, got %d", rec.Code)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("bearer auth header should be consumed before proxying, got %q", got)
	}
}

func TestServerIPAllowlistAndDenylist(t *testing.T) {
	t.Parallel()

	server := NewServerWithOptions("tunnel-secret", 8080, "https", "3000", ServerOptions{
		AccessPolicy: &accesspolicy.Policy{
			IPAllowlist: []string{"10.0.0.0/8"},
			IPDenylist:  []string{"10.0.0.5"},
		},
	})

	deniedReq := httptest.NewRequest(http.MethodGet, "https://example.test/app", nil)
	deniedReq.Header.Set("X-Forwarded-For", "10.0.0.5")
	deniedRec := httptest.NewRecorder()
	server.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected denied IP to be rejected, got %d", deniedRec.Code)
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "https://example.test/app", nil)
	allowedReq.Header.Set("X-Forwarded-For", "10.0.0.6")
	allowedRec := httptest.NewRecorder()
	server.ServeHTTP(allowedRec, allowedReq)
	if allowedRec.Code != http.StatusBadGateway {
		t.Fatalf("expected allowed IP to reach proxy path, got %d", allowedRec.Code)
	}
}

func TestServerTemporaryAccessTokenIsStrippedBeforeProxy(t *testing.T) {
	t.Parallel()

	hash, err := accesspolicy.HashToken("preview-token")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServerWithOptions("tunnel-secret", 8080, "https", "3000", ServerOptions{
		AccessPolicy: &accesspolicy.Policy{TemporaryTokens: []accesspolicy.TemporaryToken{{
			TokenHash: hash,
			ExpiresAt: "2099-01-01T00:00:00Z",
		}}},
	})
	req := httptest.NewRequest(http.MethodGet, "https://example.test/app?_sealtun_token=preview-token&a=1", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected temporary token request to reach proxy path, got %d", rec.Code)
	}
	if got := req.URL.Query().Get(accesspolicy.TemporaryTokenQueryParam); got != "" {
		t.Fatalf("temporary token query should be stripped, got %q", got)
	}
	if got := req.URL.Query().Get("a"); got != "1" {
		t.Fatalf("unrelated query should remain, got %q", got)
	}
}

func TestRedactedRequestPathDropsQueryValues(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "https://example.test/callback?token=secret", nil)

	got := redactedRequestPath(req)
	if got != "/callback?<redacted>" {
		t.Fatalf("expected redacted path, got %q", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("redacted path leaked query value: %q", got)
	}
}

func TestStatusRecorderPreservesFirstStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	status := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	status.WriteHeader(http.StatusCreated)
	status.WriteHeader(http.StatusInternalServerError)

	if status.status != http.StatusCreated {
		t.Fatalf("expected first status to be preserved, got %d", status.status)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected underlying recorder status 201, got %d", rec.Code)
	}
}
