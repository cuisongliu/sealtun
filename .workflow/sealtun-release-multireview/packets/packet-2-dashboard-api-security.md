Packet ID: packet-2-dashboard-api-security
Objective: Review dashboard token/confirm handling, command previews, read APIs, resources payloads, secret redaction, and active-scope restrictions.
Context: Dashboard has read/write APIs, remote mode token handling, live status, resources tab, command preview, and cleanup/start/stop/domain/apply mutations.
Files / sources: cmd/dashboard.go, cmd/dashboard_api.go, cmd/dashboard_test.go, pkg/k8s/client.go, pkg/k8s/client_test.go.
Ownership: Dashboard/API/security behavior only.
Do: Find backend trust-boundary bugs, secret leaks, cross-scope reads/writes, preview/API mismatches.
Do not: Add frontend framework, create real tunnels, or change external systems.
Expected output: result note under results/packet-2-dashboard-api-security.md.
Verification: targeted dashboard/cleanup/doctor tests plus full gate.
