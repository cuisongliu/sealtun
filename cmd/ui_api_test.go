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
	statusErr         error
	lastDeviceRegion  string
	lastDeviceProfile string
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
func (s *stubUIBackend) DeviceStart(ctx context.Context, region, profile string) (*uiDeviceStartResponse, error) {
	s.lastDeviceRegion = region
	s.lastDeviceProfile = profile
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

	// device start with an optional profile name reaches the backend
	backend := &stubUIBackend{}
	profileServer := newTestUIServer(t, backend, "tok")
	defer profileServer.Close()
	resp4, err := http.Post(profileServer.URL+"/api/v1/auth/device?token=tok", "application/json", strings.NewReader(`{"region":"gzg","profile":"  my-team  "}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("device start with profile must be 200, got %d", resp4.StatusCode)
	}
	if backend.lastDeviceRegion != "gzg" || backend.lastDeviceProfile != "  my-team  " {
		t.Fatalf("profile/region not passed through: %#v %#v", backend.lastDeviceRegion, backend.lastDeviceProfile)
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

// ── full-interface stub additions ───────────────────────────────────────

func (s *stubUIBackend) ListTunnels(ctx context.Context) ([]uiTunnelItem, error) {
	return []uiTunnelItem{{TunnelID: "t1", Status: "active", Protocol: "https", Endpoint: "https://x.example", Target: "http://localhost:3000"}}, nil
}
func (s *stubUIBackend) InspectTunnel(ctx context.Context, tunnelID string) (*inspectPayload, error) {
	if tunnelID == "missing" {
		return nil, errors.New("tunnel session \"missing\" not found")
	}
	return &inspectPayload{TunnelID: tunnelID, Status: "active"}, nil
}
func (s *stubUIBackend) CreateTunnel(ctx context.Context, req uiCreateTunnelRequest) (*uiCreateTunnelResponse, error) {
	if req.Port == "" && req.Target == "" {
		return nil, errors.New("port or target is required")
	}
	return &uiCreateTunnelResponse{TunnelID: "newt", PublicURL: "https://sealtun-newt.example"}, nil
}
func (s *stubUIBackend) TunnelAction(ctx context.Context, tunnelID, action string) (string, error) {
	if action == "explode" {
		return "", errors.New("unknown action \"explode\"")
	}
	return action + " ok", nil
}
func (s *stubUIBackend) TunnelRequests(ctx context.Context, tunnelID string, limit int) (*requestsPayload, error) {
	return &requestsPayload{Requests: nil}, nil
}
func (s *stubUIBackend) TunnelRequestReplay(ctx context.Context, tunnelID string, seq int64) (string, error) {
	return "replayed", nil
}
func (s *stubUIBackend) TunnelLogs(ctx context.Context, tunnelID string, tail int64) (string, error) {
	return "line1\nline2", nil
}
func (s *stubUIBackend) PolicyShow(ctx context.Context, tunnelID string) (*policyShowPayload, error) {
	return &policyShowPayload{}, nil
}
func (s *stubUIBackend) PolicySet(ctx context.Context, tunnelID string, rateLimit string, clearRateLimit bool, auditEnabled, auditDisabled bool) (*policyShowPayload, error) {
	return &policyShowPayload{}, nil
}
func (s *stubUIBackend) ShareCreate(ctx context.Context, tunnelID string, req uiShareCreateRequest) (*shareCreatePayload, error) {
	return &shareCreatePayload{TunnelID: tunnelID, Name: "default", URL: "https://x?_sealtun_token=t"}, nil
}
func (s *stubUIBackend) ShareRevoke(ctx context.Context, tunnelID, name string) error { return nil }
func (s *stubUIBackend) DomainSet(ctx context.Context, tunnelID, domain string) (string, error) {
	return "domain set", nil
}
func (s *stubUIBackend) DomainClear(ctx context.Context, tunnelID string) (string, error) {
	return "domain cleared", nil
}
func (s *stubUIBackend) ListProfiles(ctx context.Context) ([]uiProfileItem, error) {
	return []uiProfileItem{{Name: "work", Current: true}}, nil
}
func (s *stubUIBackend) ProfileUse(ctx context.Context, name string) error {
	if name == "missing" {
		return errors.New("profile not found")
	}
	return nil
}
func (s *stubUIBackend) ProfileDelete(ctx context.Context, name string) error {
	if name == "work" {
		return errors.New("profile \"work\" is active; switch to another profile (or plain login) before deleting it")
	}
	return nil
}
func (s *stubUIBackend) ListRegions(ctx context.Context) ([]uiRegionItem, error) {
	return []uiRegionItem{{Name: "gzg", Current: true}}, nil
}
func (s *stubUIBackend) Doctor(ctx context.Context) (*doctorPayload, error) {
	return &doctorPayload{}, nil
}
func (s *stubUIBackend) Logout(ctx context.Context) (string, error) { return "logged out", nil }

func TestUITunnelRoutes(t *testing.T) {
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
	post := func(path, bodyText string) (int, map[string]interface{}) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(bodyText))
		req.Header.Set("X-Sealtun-Token", "tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}

	code, _ := get("/api/v1/tunnels")
	if code != http.StatusOK {
		t.Fatalf("tunnels list route wrong: %d", code)
	}

	code, body := get("/api/v1/tunnels/t1")
	if code != http.StatusOK || body["tunnelId"] != "t1" {
		t.Fatalf("inspect route wrong: %d %#v", code, body)
	}
	code, body = get("/api/v1/tunnels/missing")
	if code != http.StatusBadRequest || body["error"] == nil {
		t.Fatalf("missing tunnel must surface error: %d %#v", code, body)
	}

	code, body = post("/api/v1/tunnels", `{"port":"3000"}`)
	if code != http.StatusOK || body["tunnelId"] != "newt" {
		t.Fatalf("create route wrong: %d %#v", code, body)
	}
	code, body = post("/api/v1/tunnels", `{}`)
	if code != http.StatusBadRequest {
		t.Fatalf("empty create must fail: %d", code)
	}
	code, body = post("/api/v1/tunnels/t1/stop", "")
	if code != http.StatusOK || body["output"] != "stop ok" {
		t.Fatalf("stop route wrong: %d %#v", code, body)
	}
	code, _ = post("/api/v1/tunnels/t1/explode", "")
	if code != http.StatusNotFound {
		t.Fatalf("bad action must be 404: %d", code)
	}

	code, _ = get("/api/v1/tunnels/t1/requests?limit=10")
	if code != http.StatusOK {
		t.Fatalf("requests route wrong: %d", code)
	}
	code, body = post("/api/v1/tunnels/t1/requests/3/replay", "")
	if code != http.StatusOK || body["output"] != "replayed" {
		t.Fatalf("replay route wrong: %d %#v", code, body)
	}
	code, body = get("/api/v1/tunnels/t1/logs?tail=50")
	if code != http.StatusOK || !strings.Contains(body["output"].(string), "line1") {
		t.Fatalf("logs route wrong: %d %#v", code, body)
	}
	code, _ = get("/api/v1/tunnels/t1/access")
	if code != http.StatusOK {
		t.Fatalf("access show route wrong: %d", code)
	}
	code, _ = post("/api/v1/tunnels/t1/shares", `{"ttl":"1h"}`)
	if code != http.StatusOK {
		t.Fatalf("share create route wrong: %d", code)
	}

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/tunnels/t1/shares/default", nil)
	req.Header.Set("X-Sealtun-Token", "tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share revoke route wrong: %d", resp.StatusCode)
	}

	code, _ = get("/api/v1/profiles")
	if code != http.StatusOK {
		t.Fatalf("profiles route wrong: %d", code)
	}
	code, _ = post("/api/v1/profiles/work/use", "")
	if code != http.StatusOK {
		t.Fatalf("profile use route wrong: %d", code)
	}
	code, body = post("/api/v1/profiles/missing/use", "")
	if code != http.StatusBadRequest {
		t.Fatalf("missing profile must fail: %d %#v", code, body)
	}
	{
		req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/profiles/work", nil)
		req.Header.Set("X-Sealtun-Token", "tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("deleting the active profile must be refused: %d", resp.StatusCode)
		}
	}
	code, _ = get("/api/v1/regions")
	if code != http.StatusOK {
		t.Fatalf("regions route wrong: %d", code)
	}
	code, _ = get("/api/v1/doctor")
	if code != http.StatusOK {
		t.Fatalf("doctor route wrong: %d", code)
	}
	code, body = post("/api/v1/auth/logout", "")
	if code != http.StatusOK || body["output"] != "logged out" {
		t.Fatalf("logout route wrong: %d %#v", code, body)
	}
}

func TestDeviceStartRejectsInvalidProfile(t *testing.T) {
	backend := &cliBackend{}
	// Invalid profile names must fail before any network access is attempted.
	if _, err := backend.DeviceStart(context.Background(), "gzg", "bad name!!"); err == nil {
		t.Fatal("invalid profile name must be rejected")
	}
	if _, err := backend.DeviceStart(context.Background(), "gzg", ".."); err == nil {
		t.Fatal("dotdot profile name must be rejected")
	}
}
