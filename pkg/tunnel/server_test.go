package tunnel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
