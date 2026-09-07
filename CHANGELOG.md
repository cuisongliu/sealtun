# Changelog

All notable changes to this project will be documented in this file.

## [v0.0.41] - 2026-09-07

### Added
- **`sealtun requests`**: live public request log from the relay pod's in-memory ring buffer (200 entries). Captures method, redacted path, status, duration, client IP, redacted headers, and a bounded body preview; WebSocket upgrades and binary bodies are excluded. Served only over the tunnel-secret-protected `/_sealtun/requests` endpoint, so request data never leaves your namespace. Supports `--follow`, `--json`, and `--limit`.
- **`sealtun requests replay <id> <seq>`**: re-sends a captured request against the local service, following the tunnel's route table with prefix stripping. Redacted headers (Authorization, Cookie) are not sent, webhook signature headers pass through, and an `X-Sealtun-Replayed-At` marker is added. Full textual bodies up to 32KiB are stored for replay.
- **Multi-service routes**: `expose 3000 --route /api=8080` or YAML `routes: [{path, port}]` multiplexes several local services into one HTTPS tunnel by path prefix (longest match, segment-aware). Matched prefixes are stripped before forwarding; everything else falls back to the primary target. Absolute-path `Location` redirects from routed apps are re-prefixed automatically.
- **`expose --qr`**: prints a terminal QR code for the public URL (or the temporary access URL when token auth is set) for instant mobile debugging; warns when the terminal is too narrow to scan.
- **`expose --ttl` and guided TTL**: tunnels can auto-delete after a duration; guided `up` suggests a 2h TTL by default so forgotten tunnels stop billing.
- **Automatic credential renewal**: expired credentials and cluster CA rotation now self-heal via the stored OAuth refresh token (cross-process locked, fail-safe to re-login guidance). Logins created before this version gain it on their next login.
- **Daemon self-healing**: the daemon restarts automatically when its binary fingerprint no longer matches the CLI (upgrades cannot silently drop new session fields), and daemon-mode tunnels now survive daemon crashes instead of being deleted as stale.
- **Routed-service health**: `inspect`, `list --check`, and `doctor` probe every route's local port; a dead routed service degrades the tunnel and is named by doctor. `list` shows a `(+N routes)` marker.

### Changed
- WebSocket upgrades are now audited with status 101 instead of a misleading 200.
- `apply` plan output shows the routes being created; `apply` diff reports route changes.
- Daemon-mode sessions are no longer auto-deleted when the daemon's PID dies; the restarted daemon re-adopts them.

### Fixed
- **Security: replay cannot be steered off-box**: replay URIs are rebuilt from path+query (origin form) so proxy-style absolute-form request lines cannot redirect a replay.
- **Security: routed reverse proxy timeouts**: the routed forwarding HTTP server now sets read-header and idle timeouts (gosec G114); hijacked WebSocket connections are unaffected.
- Body capture now runs before the proxy drains the request, and replay route resolution precedes the session's default localhost target — two bugs only visible with real traffic.
- `requests`/`replay` on stopped tunnels now fail upfront with start guidance instead of a misleading old-image message; SSH/TCP tunnels get a clear HTTPS-only explanation.
- Route prefixes containing `%` escapes or dot segments are rejected as dead configuration; a root (`/`) route that shadows the primary target now prints a warning in both `expose` and `apply`.
- Error hints are locked to stay silent for route validation messages, and YAML route ports hit the same octal/leading-zero rejection as `localPort`.

### Docs
- QuickStart and skill references now document the verified routing boundary: keep the SPA/frontend as the primary target (works end to end, including Vite HMR) and put APIs behind route prefixes; an SPA mounted under a prefix breaks its absolute asset URLs.

## [v0.0.40] - 2026-09-03

### Removed
- **Region switch duplication**: Removed `region use`; switch regions with `login <region>` (identical underlying flow).
- **Share list duplication**: Removed `share list`; temporary link names and expiry metadata are visible in `policy show <tunnel-id>`.

### Added
- **Inline secret warnings**: `apply` now warns when YAML carries plaintext `basicAuth` passwords, bearer tokens, or temporary link tokens inline, and points to the `*Env` fields.
- **cleanup --all confirmation**: interactive sessions must type `yes`; non-interactive use requires `--yes`.
- **logs escape filtering**: `logs` strips terminal escape and control sequences from remote pod logs by default (attacker-influenced content cannot hijack the terminal); `--raw` restores the unfiltered stream.
- **Rollback visibility**: failed `apply` runs now report which new tunnels were deleted and which existing tunnels were restored, and rollbacks wait for rollout readiness.

### Changed
- **Domain diagnostics merged**: `domain doctor` is now `domain status --verbose`.
- **Diff merged into apply**: `sealtun diff` is now `sealtun apply --dry-run --format diff`.
- **dry-run parity**: `apply --dry-run` stays offline but now runs local scope and secret-availability checks for YAML tunnels that match existing sessions.
- **Toolchain**: builds now require Go 1.25.13, fixing five reachable standard library CVEs (net/http, crypto/tls, net/url, encoding/asn1, x/net/idna).
- **Docs**: README and QuickStart restructured for a shorter main path; the sealtun skill synced to the current command surface.

### Fixed
- **Security: kubeconfig validation**: login now rejects server-supplied kubeconfigs containing exec credential plugins (local RCE), auth-provider plugins, non-TLS non-localhost clusters (token relay), or malformed structure.
- **Security: raw TCP resource bounds**: unauthenticated raw TCP connections are capped at 256 concurrent and expire after 10 minutes idle; temporary Accept errors no longer tear down the whole tunnel server; the `/_sealtun/tcp` WebSocket now has the same keepalive/read-limit discipline as the control channel.
- **Security: empty IP lists rejected**: `--ip-allowlist`/`--ip-denylist` containing only empty entries (e.g. `,`) previously normalized to an unrestricted tunnel; they now fail validation.
- **Security: log-injection hints**: error hints no longer misfire on local domain validation or local file permission errors, no longer reference the removed `resources set` command, and cluster CA failures now suggest re-login instead of `--target-insecure-skip-verify`.
- **Security: YAML octal ports**: `localPort: 03000` (parsed as octal 1536 by YAML 1.1) is now rejected with the intended decimal port and line number.
- **apply: duplicate temporary link names** in one tunnel are now rejected.
- **apply: leading/trailing-whitespace tunnel names** are trimmed for both ID and display.
- **daemon: stop-overwrite races**: all daemon session writes (connecting/connected/error) now use atomic compare-and-swap, so a concurrent `sealtun stop` can never be silently reverted.
- **expire parsing is no longer fail-destructive**: an unparseable `expiresAt` no longer counts as expired (the daemon auto-deletes expired tunnels).
- **expose: SIGINT during provisioning** now cancels through the rollback path instead of orphaning remote resources.
- **rotate/share/policy: committed-error surfacing**: if the remote secret/policy was rotated but the local session save failed, the new secret or share URL is still printed instead of being silently lost.
- **k8s: `-app`-suffixed tunnel IDs** no longer produce broken ingress backends or let another tunnel's cleanup delete their ingress.
- **relay drain**: when one direction of a relay fails (e.g. target RST), the surviving direction gets a short drain window instead of dropping buffered tail data.
- **Input consistency**: eight no-arg commands (`list`, `status`, `region list/current`, `profile list/current`, `apply`, `logout`) now reject extra positional arguments instead of silently ignoring them; scope checks normalize region strings before comparing; protocol values written back from remote state are normalized.
- **doctor report redaction** now also covers URL userinfo (`https://user:pass@host`).

## [v0.0.39] - 2026-08-31

### Removed
- **Alpha command surface**: Removed `mesh`, `connect`, `tui`/`console`, `ssh connect`, standalone `discover`, `template`, `export`, `init`, `events`, `metrics`, `resources`, and `watch` commands along with their dedicated packages and TUI dependencies.
- **Compatibility entries**: Removed `repair` (use `doctor --fix`), `domain set` (use `domain add`), and all command aliases (`resume`, `share add`/`delete`/`remove`, `profile rm`/`remove`).

### Changed
- **Diagnostics consolidated into `inspect`**: `inspect --remote` covers remote Kubernetes diagnostics and recent events, `inspect --metrics` covers local/Kubernetes/server counters, and `inspect --resources` covers the resource inventory with occupancy hints.
- **Continuous refresh**: `list --watch` and `inspect --watch` replace the standalone `watch` command, with `--interval` and `--count` controls and NDJSON samples in `--json` mode.
- **Resource sizing**: Tunnel Pod CPU/memory changes now go exclusively through YAML `resources` declarations and `sealtun apply`; there are no imperative resource-set commands.
- **Port discovery and protocol templates**: These capabilities remain available inside `up` (guided flow and `--template`) without standalone commands.

## [v0.0.11] - 2026-05-07

### Added
- **Custom Domains**: Added `expose --domain` plus `sealtun domain set/clear`; custom domains are attached only after CNAME ownership verification.
- **Certificate Resources**: Custom-domain tunnels now create cert-manager `Issuer` and `Certificate` resources and keep the Sealos host as the CNAME target.
- **Custom Domain Diagnostics**: `inspect --remote` and `sealtun domain verify` report DNS CNAME, Ingress host/TLS, and custom-domain certificate status.
- **Domain Readiness Wait**: Added `expose --wait-domain` and `sealtun domain verify --wait` for explicit DNS, Ingress attachment, and certificate readiness waiting.

### Fixed
- **Custom Domain Safety**: Reject IP/custom domains that point at generated or reserved Sealos hosts, require verified CNAME before writing custom hosts to Ingress, validate wait timeouts, and include Ingress host/TLS plus certificate DNS names in readiness checks.
- **Cleanup Reliability**: Tunnel cleanup now always attempts Sealtun-owned Certificate, Issuer, and TLS Secret deletion by tunnel ID, even if local custom-domain session metadata is missing.

## [v0.0.10] - 2026-05-07

### Added
- **Region Management**: Added `sealtun region list`, `sealtun region current`, and `sealtun region use` for built-in Sealos Cloud regions.
- **Sealos Domain Discovery**: Login now fetches Launchpad init data and stores `SEALOS_DOMAIN` for ingress host generation.
- **Diagnostics Controls**: Added `list --check` for local target port probing and `inspect --remote` for opt-in Kubernetes diagnostics.

### Changed
- **Session Health Model**: `inspect` and `doctor` now report degraded tunnels when the tunnel owner is alive but the local target port is unreachable.
- **Legacy Migration**: First-run migration from `~/.sealos` now copies only auth and kubeconfig files, not old tunnel session records.
- **Region Contract**: Login now accepts only built-in regions to avoid partially supported custom region endpoint combinations.

### Fixed
- **Login Browser URL Handling**: Device authorization URLs are selected before printing and are restricted to safe `http`/`https` schemes.
- **Doctor Reliability**: Remote diagnostics now use bounded worker scheduling to reduce noisy timeout cascades on slow clusters.
- **Cleanup Safety**: Cleanup paths use session-scoped kubeconfig data and avoid broad app-deploy-manager label deletion.

## [v0.0.8] - 2026-04-20

### Added
- **Session Management**: Added `sealtun status` and `sealtun logout` commands for inspecting and clearing the local login session.
- **Structured Status Output**: `sealtun status` now supports `--json` output and reports kubeconfig context, cluster, namespace, and local warning conditions.
- **Test Coverage**: Added unit tests for expose validation, auth config lifecycle, and tunnel unavailable responses.

### Changed
- **Configuration Directory**: Standardized auth storage under `~/.sealtun` and added automatic migration from the legacy `~/.sealos` path.
- **Expose Validation**: `sealtun expose` now validates the local port and protocol before provisioning remote resources.
- **Readiness Handling**: Added a configurable `--ready-timeout` for waiting on the remote tunnel pod.

### Fixed
- **Tunnel Error UX**: When the local app is not listening, public requests now return a Sealtun-branded status page explaining that the local port is offline.
- **Kubernetes Apply Semantics**: Resource reconciliation now distinguishes `NotFound` from real API errors when creating or updating Deployments, Services, and Ingresses.

## [v0.0.1] - 2026-04-07

### Added
- **Authentication**: Fully aligned login flow with `sealos-auth.mjs` using OAuth2 Device Grant.
- **Browser Integration**: Automatic browser opening for a seamless authorization experience.
- **Configuration Management**: Unified storage under `~/.sealos` directory, consistent with the Sealos ecosystem.
- **Workspace Identification**: Robust automatic detection of private workspaces (supports both numeric and string `nstype`).
- **Release Automation**: Integrated GoReleaser for automated multi-platform binary builds (Linux, Windows, macOS).

### Fixed
- **Ingress Logic**: Resolved TLS certificate verification issues by ensuring one-level subdomains with the `.sealosgzg.site` suffix.
- **Protocol Mapping**: Optimized `backend-protocol` rendering to default to HTTPS and only apply special mappings (like GRPC or WS) when explicitly requested.
