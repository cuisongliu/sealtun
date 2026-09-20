package cmd

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labring/sealtun/pkg/auth"
	"github.com/spf13/cobra"
)

// uiBackend is everything the web console API needs from the CLI's own
// operations; tests substitute a stub.
type uiBackend interface {
	Status(ctx context.Context) (*uiStatusResponse, error)
	ListWorkspaces(ctx context.Context) (*uiWorkspacesResponse, error)
	WorkspaceCurrent(ctx context.Context) (*uiWorkspaceCurrentResponse, error)
	WorkspaceUse(ctx context.Context, target string) (*uiWorkspaceUseResponse, error)
	DeviceStart(ctx context.Context, region string) (*uiDeviceStartResponse, error)
	DevicePoll(ctx context.Context, sessionID string) (*uiDeviceStatusResponse, error)

	ListTunnels(ctx context.Context) ([]uiTunnelItem, error)
	InspectTunnel(ctx context.Context, tunnelID string) (*inspectPayload, error)
	CreateTunnel(ctx context.Context, req uiCreateTunnelRequest) (*uiCreateTunnelResponse, error)
	TunnelAction(ctx context.Context, tunnelID, action string) (string, error)
	TunnelRequests(ctx context.Context, tunnelID string, limit int) (*requestsPayload, error)
	TunnelRequestReplay(ctx context.Context, tunnelID string, seq int64) (string, error)
	TunnelLogs(ctx context.Context, tunnelID string, tail int64) (string, error)
	PolicyShow(ctx context.Context, tunnelID string) (*policyShowPayload, error)
	PolicySet(ctx context.Context, tunnelID string, rateLimit string, clearRateLimit bool, auditEnabled, auditDisabled bool) (*policyShowPayload, error)
	ShareCreate(ctx context.Context, tunnelID string, req uiShareCreateRequest) (*shareCreatePayload, error)
	ShareRevoke(ctx context.Context, tunnelID, name string) error
	DomainSet(ctx context.Context, tunnelID, domain string) (string, error)
	DomainClear(ctx context.Context, tunnelID string) (string, error)
	ListProfiles(ctx context.Context) ([]uiProfileItem, error)
	ProfileUse(ctx context.Context, name string) error
	ListRegions(ctx context.Context) ([]uiRegionItem, error)
	Doctor(ctx context.Context) (*doctorPayload, error)
	Logout(ctx context.Context) (string, error)
}

type uiStatusResponse struct {
	LoggedIn  bool   `json:"loggedIn"`
	Region    string `json:"region,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

type uiWorkspaceItem struct {
	ID       string `json:"id"`
	UID      string `json:"uid,omitempty"`
	TeamName string `json:"teamName,omitempty"`
	Role     string `json:"role,omitempty"`
	Current  bool   `json:"current"`
}

type uiWorkspacesResponse struct {
	Workspaces []uiWorkspaceItem `json:"workspaces"`
}

type uiWorkspaceCurrentResponse struct {
	Namespace string `json:"namespace"`
	Workspace string `json:"workspace,omitempty"`
}

type uiWorkspaceUseResponse struct {
	Switched  bool   `json:"switched"`
	Namespace string `json:"namespace"`
	Workspace string `json:"workspace"`
}

type uiDeviceStartResponse struct {
	Session         string `json:"session"`
	VerificationURL string `json:"verificationUrl"`
	UserCode        string `json:"userCode"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

type uiDeviceStatusResponse struct {
	State   string `json:"state"` // pending, authorized, error, expired
	Message string `json:"message,omitempty"`
}

// newUIMux wires the console API. Every /api/v1 route except health requires
// the per-session token (header X-Sealtun-Token or ?token=), which is only
// ever printed to this machine's terminal.
func newUIMux(backend uiBackend, token string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	serveUIStatic(mux)
	mux.Handle("/api/v1/", uiTokenGate(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/qr" {
			handleQR(w, r)
			return
		}
		routeUI(w, r, backend)
	})))
	return mux
}

func uiTokenGate(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Sealtun-Token")
		if provided == "" {
			provided = r.URL.Query().Get("token")
		}
		if provided == "" || subtle_compare(provided, token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid session token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func subtle_compare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) != 1
}

func routeUI(w http.ResponseWriter, r *http.Request, backend uiBackend) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	switch {
	case path == "status" && r.Method == http.MethodGet:
		payload, err := backend.Status(r.Context())
		respond(w, payload, err)
	case path == "workspaces" && r.Method == http.MethodGet:
		payload, err := backend.ListWorkspaces(r.Context())
		respond(w, payload, err)
	case path == "workspaces/current" && r.Method == http.MethodGet:
		payload, err := backend.WorkspaceCurrent(r.Context())
		respond(w, payload, err)
	case path == "workspaces/use" && r.Method == http.MethodPost:
		var req struct {
			Target string `json:"target"`
		}
		if err := decodeJSONBody(w, r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		payload, err := backend.WorkspaceUse(r.Context(), req.Target)
		respond(w, payload, err)
	case path == "auth/device" && r.Method == http.MethodPost:
		var req struct {
			Region string `json:"region"`
		}
		if err := decodeJSONBody(w, r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		payload, err := backend.DeviceStart(r.Context(), req.Region)
		respond(w, payload, err)
	case path == "auth/device/status" && r.Method == http.MethodGet:
		payload, err := backend.DevicePoll(r.Context(), r.URL.Query().Get("session"))
		respond(w, payload, err)
	case path == "tunnels" && r.Method == http.MethodGet:
		payload, err := backend.ListTunnels(r.Context())
		respond(w, payload, err)
	case path == "tunnels" && r.Method == http.MethodPost:
		var req uiCreateTunnelRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		payload, err := backend.CreateTunnel(r.Context(), req)
		respond(w, payload, err)
	case path == "profiles" && r.Method == http.MethodGet:
		payload, err := backend.ListProfiles(r.Context())
		respond(w, payload, err)
	case strings.HasPrefix(path, "profiles/") && r.Method == http.MethodPost:
		name := strings.TrimSuffix(strings.TrimPrefix(path, "profiles/"), "/use")
		if !strings.HasSuffix(path, "/use") || name == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown endpoint"})
			return
		}
		respond(w, map[string]bool{"ok": true}, backend.ProfileUse(r.Context(), name))
	case path == "regions" && r.Method == http.MethodGet:
		payload, err := backend.ListRegions(r.Context())
		respond(w, payload, err)
	case path == "discover" && r.Method == http.MethodGet:
		items, err := discoverLocalPorts(r.Context(), discoverOptions{Limit: 30, Protocol: "auto"}, systemPortDiscoverer{})
		if err != nil {
			respond(w, nil, err)
			return
		}
		respond(w, items, nil)
	case path == "doctor" && r.Method == http.MethodGet:
		payload, err := backend.Doctor(r.Context())
		respond(w, payload, err)
	case path == "auth/logout" && r.Method == http.MethodPost:
		payload, err := backend.Logout(r.Context())
		respond(w, map[string]string{"output": payload}, err)
	case strings.HasPrefix(path, "tunnels/"):
		routeTunnel(w, r, backend, strings.TrimPrefix(path, "tunnels/"))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown endpoint"})
	}
}

// routeTunnel dispatches /api/v1/tunnels/:id[/action] paths.
func routeTunnel(w http.ResponseWriter, r *http.Request, backend uiBackend, rest string) {
	parts := strings.SplitN(rest, "/", 2)
	tunnelID := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	if tunnelID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown endpoint"})
		return
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		payload, err := backend.InspectTunnel(r.Context(), tunnelID)
		respond(w, payload, err)
	case sub == "requests" && r.Method == http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		payload, err := backend.TunnelRequests(r.Context(), tunnelID, limit)
		respond(w, payload, err)
	case strings.HasPrefix(sub, "requests/") && strings.HasSuffix(sub, "/replay") && r.Method == http.MethodPost:
		seqText := strings.TrimSuffix(strings.TrimPrefix(sub, "requests/"), "/replay")
		seq, err := strconv.ParseInt(seqText, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid seq"})
			return
		}
		payload, err := backend.TunnelRequestReplay(r.Context(), tunnelID, seq)
		respond(w, map[string]string{"output": payload}, err)
	case sub == "logs" && r.Method == http.MethodGet:
		tail, _ := strconv.ParseInt(r.URL.Query().Get("tail"), 10, 64)
		payload, err := backend.TunnelLogs(r.Context(), tunnelID, tail)
		respond(w, map[string]string{"output": payload}, err)
	case sub == "access" && r.Method == http.MethodGet:
		payload, err := backend.PolicyShow(r.Context(), tunnelID)
		respond(w, payload, err)
	case sub == "access" && r.Method == http.MethodPut:
		var req struct {
			RateLimit      string `json:"rateLimit,omitempty"`
			ClearRateLimit bool   `json:"clearRateLimit,omitempty"`
			Audit          *bool  `json:"audit,omitempty"`
		}
		if err := decodeJSONBody(w, r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		auditEnabled, auditDisabled := false, false
		if req.Audit != nil {
			auditEnabled = *req.Audit
			auditDisabled = !*req.Audit
		}
		payload, err := backend.PolicySet(r.Context(), tunnelID, req.RateLimit, req.ClearRateLimit, auditEnabled, auditDisabled)
		respond(w, payload, err)
	case sub == "access/audit" && r.Method == http.MethodGet:
		since := 10 * time.Minute
		if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid since; use e.g. 10m, 1h"})
				return
			}
			since = parsed
		}
		payload, err := collectPolicyAudit(r.Context(), tunnelID, since, 200)
		respond(w, payload, err)
	case sub == "shares" && r.Method == http.MethodPost:
		var req uiShareCreateRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		payload, err := backend.ShareCreate(r.Context(), tunnelID, req)
		respond(w, payload, err)
	case strings.HasPrefix(sub, "shares/") && r.Method == http.MethodDelete:
		name := strings.TrimPrefix(sub, "shares/")
		respond(w, map[string]bool{"ok": true}, backend.ShareRevoke(r.Context(), tunnelID, name))
	case sub == "domain" && r.Method == http.MethodPut:
		var req struct {
			Domain string `json:"domain"`
		}
		if err := decodeJSONBody(w, r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		payload, err := backend.DomainSet(r.Context(), tunnelID, req.Domain)
		respond(w, map[string]string{"output": payload}, err)
	case sub == "domain" && r.Method == http.MethodDelete:
		payload, err := backend.DomainClear(r.Context(), tunnelID)
		respond(w, map[string]string{"output": payload}, err)
	case (sub == "stop" || sub == "start" || sub == "delete") && r.Method == http.MethodPost:
		payload, err := backend.TunnelAction(r.Context(), tunnelID, sub)
		respond(w, map[string]string{"output": payload}, err)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown endpoint"})
	}
}

func respond(w http.ResponseWriter, payload interface{}, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

const uiMaxJSONBody = 4 << 10

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target interface{}) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, uiMaxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// ── CLI backend ──────────────────────────────────────────────────────────

type cliBackend struct {
	deviceMu      sync.Mutex
	deviceSession *pendingDeviceLogin
}

func newCLIBackend() uiBackend {
	return &cliBackend{}
}

func (b *cliBackend) Status(ctx context.Context) (*uiStatusResponse, error) {
	authData, err := auth.LoadAuthData()
	if err != nil {
		return &uiStatusResponse{LoggedIn: false}, nil
	}
	res := &uiStatusResponse{LoggedIn: true, Region: authData.Region}
	if kubeconfig, err := auth.ActiveKubeconfig(); err == nil {
		if ns, err := auth.KubeconfigNamespace(kubeconfig); err == nil {
			res.Namespace = ns
		}
	}
	if ws := authData.CurrentWorkspace; ws != nil {
		res.Workspace = valueOr(ws.TeamName, ws.ID)
	}
	if name, err := auth.CurrentProfileName(); err == nil {
		res.Profile = name
	}
	return res, nil
}

func (b *cliBackend) ListWorkspaces(ctx context.Context) (*uiWorkspacesResponse, error) {
	_, namespaces, current, err := loadWorkspaceState(commandContext(ctx))
	if err != nil {
		return nil, err
	}
	res := &uiWorkspacesResponse{Workspaces: make([]uiWorkspaceItem, 0, len(namespaces))}
	for _, ns := range namespaces {
		res.Workspaces = append(res.Workspaces, uiWorkspaceItem{
			ID:       ns.ID,
			UID:      ns.UID,
			TeamName: ns.TeamName,
			Role:     fmt.Sprint(ns.Role),
			Current:  ns.ID == current || ns.UID == current,
		})
	}
	return res, nil
}

func (b *cliBackend) WorkspaceCurrent(ctx context.Context) (*uiWorkspaceCurrentResponse, error) {
	authData, err := auth.LoadAuthData()
	if err != nil {
		return nil, errors.New("not logged in")
	}
	kubeconfig, err := auth.ActiveKubeconfig()
	if err != nil {
		return nil, err
	}
	ns, err := auth.KubeconfigNamespace(kubeconfig)
	if err != nil {
		return nil, err
	}
	res := &uiWorkspaceCurrentResponse{Namespace: ns}
	if ws := authData.CurrentWorkspace; ws != nil {
		res.Workspace = valueOr(ws.TeamName, ws.ID)
	}
	return res, nil
}

func (b *cliBackend) WorkspaceUse(ctx context.Context, target string) (*uiWorkspaceUseResponse, error) {
	if strings.TrimSpace(target) == "" {
		return nil, errors.New("target is required")
	}
	if err := runWorkspaceUse(commandContext(ctx), target); err != nil {
		return nil, err
	}
	current, err := b.WorkspaceCurrent(ctx)
	if err != nil {
		return nil, err
	}
	return &uiWorkspaceUseResponse{Switched: true, Namespace: current.Namespace, Workspace: current.Workspace}, nil
}

type pendingDeviceLogin struct {
	region     string
	deviceCode string
	expiresAt  time.Time
	interval   int
}

func (b *cliBackend) DeviceStart(ctx context.Context, region string) (*uiDeviceStartResponse, error) {
	resolved, err := auth.ResolveRegion(region)
	if err != nil {
		return nil, err
	}
	deviceAuth, err := auth.RequestDeviceAuthorization(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to request device authorization: %w", err)
	}
	if err := validateDeviceAuthorization(deviceAuth); err != nil {
		return nil, err
	}
	authURL, err := verificationURL(deviceAuth)
	if err != nil {
		return nil, err
	}
	b.deviceMu.Lock()
	b.deviceSession = &pendingDeviceLogin{
		region:     resolved,
		deviceCode: deviceAuth.DeviceCode,
		expiresAt:  time.Now().Add(time.Duration(deviceAuth.ExpiresIn) * time.Second),
		interval:   deviceAuth.Interval,
	}
	b.deviceMu.Unlock()
	return &uiDeviceStartResponse{
		Session:         "current",
		VerificationURL: authURL,
		UserCode:        deviceAuth.UserCode,
		ExpiresIn:       deviceAuth.ExpiresIn,
		Interval:        deviceAuth.Interval,
	}, nil
}

func (b *cliBackend) DevicePoll(ctx context.Context, sessionID string) (*uiDeviceStatusResponse, error) {
	b.deviceMu.Lock()
	pending := b.deviceSession
	b.deviceMu.Unlock()
	if pending == nil {
		return &uiDeviceStatusResponse{State: "expired", Message: "no device login in progress; start one first"}, nil
	}
	if time.Now().After(pending.expiresAt) {
		b.deviceMu.Lock()
		b.deviceSession = nil
		b.deviceMu.Unlock()
		return &uiDeviceStatusResponse{State: "expired", Message: "device code expired; start a new login"}, nil
	}
	tokenRes, state, err := auth.PollDeviceTokenOnce(pending.region, pending.deviceCode)
	if err != nil {
		return nil, err
	}
	if state != "" {
		return &uiDeviceStatusResponse{State: state}, nil
	}
	if _, err := performLoginExchange(pending.region, tokenRes, ""); err != nil {
		return &uiDeviceStatusResponse{State: "error", Message: err.Error()}, nil
	}
	b.deviceMu.Lock()
	b.deviceSession = nil
	b.deviceMu.Unlock()
	return &uiDeviceStatusResponse{State: "authorized"}, nil
}

// commandContext wraps a context in a bare cobra command for CLI helpers that
// expect one.
func commandContext(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	return cmd
}
