package cmd

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labring/sealtun/pkg/auth"
	"github.com/labring/sealtun/pkg/routes"
	"github.com/labring/sealtun/pkg/session"
	"github.com/spf13/cobra"
)

// apiOpLock serializes operations that mutate the CLI's package-level flag
// globals (expose/policy/share/domain/lifecycle paths were written for a
// single-threaded process). The console server is concurrent, so every such
// operation runs under this lock; read-only handlers stay lock-free.
var apiOpLock sync.Mutex

// ── Tunnels ─────────────────────────────────────────────────────────────

type uiTunnelItem struct {
	TunnelID    string         `json:"tunnelId"`
	Status      string         `json:"status"`
	Protocol    string         `json:"protocol"`
	Endpoint    string         `json:"endpoint"`
	Target      string         `json:"target"`
	Routes      []routes.Route `json:"routes,omitempty"`
	BasicAuth   bool           `json:"basicAuth"`
	Access      bool           `json:"access"`
	Mode        string         `json:"mode"`
	Namespace   string         `json:"namespace"`
	ExpiresAt   string         `json:"expiresAt,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	RouteHealth []RouteHealth  `json:"routeHealth,omitempty"`
}

func (b *cliBackend) ListTunnels(ctx context.Context) ([]uiTunnelItem, error) {
	items, err := collectListItemsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]uiTunnelItem, 0, len(items))
	for _, item := range items {
		out = append(out, uiTunnelItem{
			TunnelID:  item.TunnelID,
			Status:    item.Status,
			Protocol:  item.Protocol,
			Endpoint:  item.Endpoint,
			Target:    item.TargetURL,
			BasicAuth: item.BasicAuth,
			Access:    item.AccessPolicy,
			Mode:      item.Mode,
			Namespace: item.Namespace,
			ExpiresAt: item.ExpiresAt,
			CreatedAt: item.CreatedAt,
		})
	}
	// listItem does not carry routes; enrich from sessions where available.
	sessions, err := session.List()
	if err == nil {
		byID := map[string]session.TunnelSession{}
		for _, sess := range sessions {
			byID[sess.TunnelID] = sess
		}
		for i := range out {
			if sess, ok := byID[out[i].TunnelID]; ok {
				out[i].Routes = sess.Routes
				if len(sess.Routes) > 0 {
					out[i].RouteHealth = probeRouteHealth(sess)
				}
			}
		}
	}
	return out, nil
}

func (b *cliBackend) InspectTunnel(ctx context.Context, tunnelID string) (*inspectPayload, error) {
	return collectInspectPayload(tunnelID)
}

type uiCreateTunnelRequest struct {
	Port              string         `json:"port"`
	Target            string         `json:"target,omitempty"`
	Protocol          string         `json:"protocol,omitempty"`
	TTL               string         `json:"ttl,omitempty"`
	Routes            []routes.Route `json:"routes,omitempty"`
	Domain            string         `json:"domain,omitempty"`
	RateLimit         string         `json:"rateLimit,omitempty"`
	Audit             bool           `json:"audit,omitempty"`
	BasicAuth         bool           `json:"basicAuth,omitempty"`
	BasicAuthUser     string         `json:"basicAuthUser,omitempty"`
	BasicAuthPassword string         `json:"basicAuthPassword,omitempty"`
}

type uiCreateTunnelResponse struct {
	TunnelID  string `json:"tunnelId"`
	PublicURL string `json:"publicUrl"`
	Output    string `json:"output"`
}

func (b *cliBackend) CreateTunnel(ctx context.Context, req uiCreateTunnelRequest) (*uiCreateTunnelResponse, error) {
	if strings.TrimSpace(req.Port) == "" && strings.TrimSpace(req.Target) == "" {
		return nil, fmt.Errorf("port or target is required")
	}
	if err := validateLocalPort(req.Port); req.Target == "" && err != nil {
		return nil, err
	}

	apiOpLock.Lock()
	defer apiOpLock.Unlock()

	snapshot := snapshotExposeFlags()
	defer snapshot.restore()

	protocol = "https"
	if req.Protocol != "" {
		protocol = req.Protocol
	}
	exposeTarget = strings.TrimSpace(req.Target)
	exposeRoutes = nil
	for _, route := range req.Routes {
		exposeRoutes = append(exposeRoutes, fmt.Sprintf("%s=%d", routes.NormalizePath(route.Path), route.Port))
	}
	customDomain = strings.TrimSpace(req.Domain)
	accessRateLimit = strings.TrimSpace(req.RateLimit)
	accessAuditEnabled = req.Audit
	if req.BasicAuth {
		basicAuthUser = req.BasicAuthUser
		basicAuthPassword = req.BasicAuthPassword
	}
	if req.TTL != "" {
		ttl, err := time.ParseDuration(strings.TrimSpace(req.TTL))
		if err != nil || ttl <= 0 {
			return nil, fmt.Errorf("invalid ttl %q; use e.g. 30m, 2h, 24h", req.TTL)
		}
		exposeTTL = ttl
	}
	foreground = false

	args := []string{}
	if exposeTarget == "" {
		args = append(args, strings.TrimSpace(req.Port))
	}
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := runExposeCommand(cmd, args); err != nil {
		return nil, fmt.Errorf("%v\n%s", err, tailLines(out.String(), 5))
	}
	output := out.String()
	return &uiCreateTunnelResponse{
		TunnelID:  extractTunnelIDFromOutput(output),
		PublicURL: extractLineValue(output, "Public URL: "),
		Output:    output,
	}, nil
}

// extractTunnelIDFromOutput prefers the public URL (stable
// sealtun-<16hex>-ns-... shape) and falls back to the "Preparing tunnel"
// line, whose trailing ellipsis must be stripped — a bare suffix read turns
// the id into <id>... and the detail page then reports the tunnel missing.
func extractTunnelIDFromOutput(output string) string {
	if url := extractLineValue(output, "Public URL: "); url != "" {
		rest := strings.TrimPrefix(url, "https://sealtun-")
		if rest != url {
			if idx := strings.Index(rest, "-"); idx > 0 {
				return rest[:idx]
			}
		}
	}
	id := extractLineValue(output, "Preparing tunnel ")
	return strings.TrimRight(id, ". ")
}

func (b *cliBackend) TunnelAction(ctx context.Context, tunnelID, action string) (string, error) {
	apiOpLock.Lock()
	defer apiOpLock.Unlock()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	var err error
	switch action {
	case "stop":
		err = runStopTunnel(cmd, tunnelID)
	case "start":
		err = runStartTunnel(cmd, tunnelID)
	case "delete":
		cleanupYes = true
		err = runSharedCommand(cleanupCmd, ctx, []string{tunnelID}, &out)
		if err != nil && strings.Contains(err.Error(), "refusing cleanup") {
			// Console delete means "get rid of it": an active tunnel is stopped
			// first, then removed. CLI cleanup semantics stay untouched.
			if stopErr := runStopTunnel(cmd, tunnelID); stopErr != nil {
				err = fmt.Errorf("stop before delete failed: %w", stopErr)
			} else {
				err = runSharedCommand(cleanupCmd, ctx, []string{tunnelID}, &out)
			}
		}
	default:
		err = fmt.Errorf("unknown action %q", action)
	}
	if err != nil {
		return "", fmt.Errorf("%v\n%s", err, tailLines(out.String(), 5))
	}
	return out.String(), nil
}

// ── Requests / logs ─────────────────────────────────────────────────────

func (b *cliBackend) TunnelRequests(ctx context.Context, tunnelID string, limit int) (*requestsPayload, error) {
	sess, err := findSession(tunnelID)
	if err != nil {
		return nil, err
	}
	if err := requestsSessionUsable(sess); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return fetchTunnelRequests(ctx, *sess, limit)
}

func (b *cliBackend) TunnelLogs(ctx context.Context, tunnelID string, tail int64) (string, error) {
	if tail <= 0 || tail > 2000 {
		tail = 200
	}
	apiOpLock.Lock()
	defer apiOpLock.Unlock()
	logsTail = tail
	logsFollow = false
	logsSince = 0
	var out bytes.Buffer
	if err := runSharedCommand(logsCmd, ctx, []string{tunnelID}, &out); err != nil {
		return "", fmt.Errorf("%v\n%s", err, tailLines(out.String(), 5))
	}
	return out.String(), nil
}

func (b *cliBackend) TunnelRequestReplay(ctx context.Context, tunnelID string, seq int64) (string, error) {
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runRequestsReplay(cmd, tunnelID, strconv.FormatInt(seq, 10)); err != nil {
		return "", fmt.Errorf("%v\n%s", err, tailLines(out.String(), 5))
	}
	return out.String(), nil
}

// ── Access policy / shares / domain ─────────────────────────────────────

func (b *cliBackend) PolicyShow(ctx context.Context, tunnelID string) (*policyShowPayload, error) {
	return showPolicy(tunnelID, nowUTC())
}

func (b *cliBackend) PolicySet(ctx context.Context, tunnelID string, rateLimit string, clearRateLimit bool, auditEnabled, auditDisabled bool) (*policyShowPayload, error) {
	apiOpLock.Lock()
	defer apiOpLock.Unlock()
	return setPolicy(ctx, tunnelID, rateLimit, clearRateLimit, auditEnabled, auditDisabled)
}

type uiShareCreateRequest struct {
	Name string `json:"name,omitempty"`
	TTL  string `json:"ttl,omitempty"`
}

func (b *cliBackend) ShareCreate(ctx context.Context, tunnelID string, req uiShareCreateRequest) (*shareCreatePayload, error) {
	ttl := time.Hour
	if strings.TrimSpace(req.TTL) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(req.TTL))
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid ttl %q; use e.g. 30m, 1h, 24h", req.TTL)
		}
		ttl = parsed
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "default"
	}
	return createShareLink(ctx, tunnelID, name, ttl, "")
}

func (b *cliBackend) ShareRevoke(ctx context.Context, tunnelID, name string) error {
	return revokeShareLink(ctx, tunnelID, name)
}

func (b *cliBackend) DomainSet(ctx context.Context, tunnelID, domain string) (string, error) {
	apiOpLock.Lock()
	defer apiOpLock.Unlock()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := runDomainAdd(cmd, []string{tunnelID, domain}, false, 0); err != nil {
		return "", fmt.Errorf("%v\n%s", err, tailLines(out.String(), 5))
	}
	return out.String(), nil
}

func (b *cliBackend) DomainClear(ctx context.Context, tunnelID string) (string, error) {
	apiOpLock.Lock()
	defer apiOpLock.Unlock()
	if _, err := clearSessionCustomDomain(ctx, tunnelID); err != nil {
		return "", err
	}
	return fmt.Sprintf("custom domain cleared for tunnel %s", tunnelID), nil
}

// ── Identity / diagnostics ──────────────────────────────────────────────

type uiProfileItem struct {
	Name      string `json:"name"`
	Region    string `json:"region,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Current   bool   `json:"current"`
}

func (b *cliBackend) ListProfiles(ctx context.Context) ([]uiProfileItem, error) {
	profiles, err := auth.ListProfiles()
	if err != nil {
		return nil, err
	}
	current, _ := auth.CurrentProfileName()
	out := make([]uiProfileItem, 0, len(profiles))
	for _, profile := range profiles {
		item := uiProfileItem{
			Name:      profile.Name,
			Region:    profile.Region,
			Workspace: profile.WorkspaceName,
			Current:   profile.Name == current,
		}
		out = append(out, item)
	}
	return out, nil
}

func (b *cliBackend) ProfileUse(ctx context.Context, name string) error {
	if _, _, err := auth.LoadProfile(name); err != nil {
		return err
	}
	return auth.ActivateProfile(name)
}

func (b *cliBackend) ProfileDelete(ctx context.Context, name string) error {
	current, _ := auth.CurrentProfileName()
	if current == name {
		return fmt.Errorf("profile %q is active; switch to another profile (or plain login) before deleting it", name)
	}
	if _, _, err := auth.LoadProfile(name); err != nil {
		return err
	}
	return auth.DeleteProfile(name)
}

type uiRegionItem struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	SealosDomain string `json:"sealosDomain"`
	Current      bool   `json:"current"`
}

func (b *cliBackend) ListRegions(ctx context.Context) ([]uiRegionItem, error) {
	current := ""
	if authData, err := auth.LoadAuthData(); err == nil {
		current = authData.Region
	}
	out := make([]uiRegionItem, 0)
	for _, region := range auth.KnownRegions() {
		out = append(out, uiRegionItem{
			Name:         region.Name,
			URL:          region.URL,
			SealosDomain: region.SealosDomain,
			Current:      region.URL == current,
		})
	}
	return out, nil
}

func (b *cliBackend) Doctor(ctx context.Context) (*doctorPayload, error) {
	return collectDoctorPayloadWithContext(ctx)
}

func (b *cliBackend) Logout(ctx context.Context) (string, error) {
	apiOpLock.Lock()
	defer apiOpLock.Unlock()
	var out bytes.Buffer
	if err := runSharedCommand(logoutCmd, ctx, nil, &out); err != nil {
		return "", fmt.Errorf("%v\n%s", err, tailLines(out.String(), 5))
	}
	return out.String(), nil
}

// ── Flag snapshot/restore + small helpers ────────────────────────────────

type exposeFlagSnapshot struct {
	protocol, exposeTarget, customDomain, basicAuthUser, basicAuthPassword string
	accessRateLimit, exposeTargetTLS, tempToken, tempTokenEnv              string
	exposeRoutes                                                           []string
	exposeTTL                                                              time.Duration
	accessAudit, basicAuthSet                                              bool
}

func snapshotExposeFlags() *exposeFlagSnapshot {
	return &exposeFlagSnapshot{
		protocol: protocol, exposeTarget: exposeTarget, customDomain: customDomain,
		basicAuthUser: basicAuthUser, basicAuthPassword: basicAuthPassword,
		accessRateLimit: accessRateLimit, exposeRoutes: exposeRoutes, exposeTTL: exposeTTL,
		accessAudit: accessAuditEnabled,
	}
}

func (s *exposeFlagSnapshot) restore() {
	protocol = s.protocol
	exposeTarget = s.exposeTarget
	customDomain = s.customDomain
	basicAuthUser = s.basicAuthUser
	basicAuthPassword = s.basicAuthPassword
	accessRateLimit = s.accessRateLimit
	exposeRoutes = s.exposeRoutes
	exposeTTL = s.exposeTTL
	accessAuditEnabled = s.accessAudit
	foreground = false
	exposeQR = false
	targetTLSInsecureSkipVerify = false
	basicAuthCredential = ""
	basicAuthPasswordEnv = ""
	bearerToken, bearerTokenEnv = "", ""
	ipAllowlist, ipDenylist = nil, nil
	temporaryAccessToken, temporaryAccessTokenEnv = "", ""
	waitDomain = false
}

func extractLineValue(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if idx := strings.Index(line, prefix); idx >= 0 {
			return strings.TrimSpace(line[idx+len(prefix):])
		}
	}
	return ""
}

func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// runSharedCommand executes a package-level cobra command with a context and
// captured output. Shared command objects have no context of their own (cobra
// only sets one during rootCmd.Execute), so API handlers must install one;
// apiOpLock guarantees no concurrent context/output swaps.
func runSharedCommand(shared *cobra.Command, ctx context.Context, args []string, out *bytes.Buffer) error {
	shared.SetContext(ctx)
	shared.SetOut(out)
	shared.SetErr(out)
	return shared.RunE(shared, args)
}
