Packet ID: packet-2-dashboard-api-ui
Objective: Review dashboard API/server/frontend for auth, confirm, state refresh, and UX correctness.
Files / sources: cmd/dashboard*.go, dashboard tests.
Do: Check token enforcement, confirm fields, SSE, resource/log/metrics/events/domain APIs, command previews, secret leakage.
Do not: Introduce a frontend framework or broaden remote mutation behavior.
Expected output: Result note with accepted/deferred findings and verification.
Verification: Dashboard tests plus full gate.
