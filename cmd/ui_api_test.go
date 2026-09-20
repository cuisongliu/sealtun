package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/auth"
)

type stubUIBackend struct {
	statusErr error
}

func (s *stubUIBackend) Status(ctx context.Context) (*uiStatusResponse, error) {
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	return &uiStatusResponse{LoggedIn: true, Region: "https://gzg.sealos.run", Namespace: "ns-x"}, nil
}
func (s *stubUIBackend) ListWorkspaces(ctx context.Context) (*uiWorkspacesResponse, error) {
	return &uiWorkspacesResponse{Workspaces: []uiWorkspaceItem{{ID: "ns-x", Current: true}}}, nil
}
func (s *stubUIBackend) WorkspaceCurrent(ctx context.Context) (*uiWorkspaceCurrentResponse, error) {
	return &uiWorkspaceCurrentResponse{Namespace: "ns-x"}, nil
}
func (s *stubUIBackend) WorkspaceUse(ctx context.Context, target string) (*uiWorkspaceUseResponse, error) {
	if target == "" {
		return nil, errors.New("target is required")
	}
	return &uiWorkspaceUseResponse{Switched: true, Namespace: target}, nil
}
func (s *stubUIBackend) DeviceStart(ctx context.Context, region string) (*uiDeviceStartResponse, error) {
	return &uiDeviceStartResponse{Session: "current", VerificationURL: "https://example/device", UserCode: "ABCD-1234"}, nil
}
func (s *stubUIBackend) DevicePoll(ctx context.Context, sessionID string) (*uiDeviceStatusResponse, error) {
	return &uiDeviceStatusResponse{State: "pending"}, nil
}

func newTestUIServer(t *testing.T, backend uiBackend, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(newUIMux(backend, token))
}

func TestUITokenGate(t *testing.T) {
	server := newTestUIServer(t, &stubUIBackend{}, "secret-token")
	defer server.Close()

	for _, tc := range []struct {
		name   string
		url    string
		header string
		want   int
	}{
		{"no token", server.URL + "/api/v1/status", "", http.StatusUnauthorized},
		{"wrong token", server.URL + "/api/v1/status", "nope", http.StatusUnauthorized},
		{"header token", server.URL + "/api/v1/status", "secret-token", http.StatusOK},
		{"query token", server.URL + "/api/v1/status?token=secret-token", "", http.StatusOK},
		{"health is open", server.URL + "/api/v1/health", "", http.StatusOK},
	} {
		req, _ := http.NewRequest(http.MethodGet, tc.url, nil)
		if tc.header != "" {
			req.Header.Set("X-Sealtun-Token", tc.header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}

func TestUIRoutes(t *testing.T) {
	server := newTestUIServer(t, &stubUIBackend{}, "tok")
	defer server.Close()
	get := func(path string) (int, map[string]interface{}) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		req.Header.Set("X-Sealtun-Token", "tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}

	code, body := get("/api/v1/status")
	if code != http.StatusOK || body["loggedIn"] != true || body["namespace"] != "ns-x" {
		t.Fatalf("status route wrong: %d %#v", code, body)
	}
	code, body = get("/api/v1/workspaces")
	if code != http.StatusOK || body["workspaces"] == nil {
		t.Fatalf("workspaces route wrong: %d %#v", code, body)
	}
	code, body = get("/api/v1/workspaces/current")
	if code != http.StatusOK || body["namespace"] != "ns-x" {
		t.Fatalf("workspace current wrong: %d %#v", code, body)
	}

	// POST workspaces/use
	resp, err := http.Post(server.URL+"/api/v1/workspaces/use?token=tok", "application/json", strings.NewReader(`{"target":"ns-y"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var useBody map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&useBody)
	if useBody["switched"] != true || useBody["namespace"] != "ns-y" {
		t.Fatalf("workspace use wrong: %#v", useBody)
	}

	// malformed JSON is rejected
	resp2, err := http.Post(server.URL+"/api/v1/workspaces/use?token=tok", "application/json", strings.NewReader(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad JSON must be 400, got %d", resp2.StatusCode)
	}

	// device start + poll
	resp3, err := http.Post(server.URL+"/api/v1/auth/device?token=tok", "application/json", strings.NewReader(`{"region":"gzg"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	var start map[string]interface{}
	_ = json.NewDecoder(resp3.Body).Decode(&start)
	if start["userCode"] != "ABCD-1234" {
		t.Fatalf("device start wrong: %#v", start)
	}
	code, body = get("/api/v1/auth/device/status?session=current")
	if code != http.StatusOK || body["state"] != "pending" {
		t.Fatalf("device status wrong: %d %#v", code, body)
	}

	// unknown endpoint
	code, _ = get("/api/v1/nope")
	if code != http.StatusNotFound {
		t.Fatalf("unknown endpoint must be 404, got %d", code)
	}
}

func TestPollDeviceTokenOnceStates(t *testing.T) {
	var grant string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant = r.FormValue("grant_type")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer server.Close()

	_, state, err := auth.PollDeviceTokenOnce(server.URL, "code")
	if err != nil || state != "pending" {
		t.Fatalf("expected pending, got %q %v", state, err)
	}
	if !strings.Contains(grant, "device_code") {
		t.Fatalf("wrong grant_type: %q", grant)
	}
}
