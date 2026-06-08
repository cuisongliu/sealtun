Packet: packet-2-dashboard-api-ui
Status: completed

Reviewed:
- `cmd/dashboard.go`
- `cmd/dashboard_api.go`
- `cmd/dashboard_test.go`

Accepted findings:
- No P0/P1 dashboard API security issue found.
- Existing tests cover token enforcement, remote home token non-leakage, Basic Auth wrapping, SSE token non-leakage, confirm enforcement, active-scope filtering, dashboard diff scope rejection, and active cleanup refusal.

Deferred findings:
- `serveCreateTunnel` constructs a success message using `result.TunnelID` even when creation fails and the result is empty; `writeDashboardResult` returns the error JSON, so this has no user-visible success-path impact.

Verification:
- Covered by `go test ./cmd` and full release gate.
