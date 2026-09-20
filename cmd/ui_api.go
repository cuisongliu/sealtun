package cmd

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	mux.Handle("/api/v1/", uiTokenGate(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
