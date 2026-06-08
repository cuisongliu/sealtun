# Orchestration: Sealtun Full Code Release Review

## Execution Rules

- Keep the original objective intact.
- Ask for approval before risky, expensive, external, or destructive actions.
- Keep immediate blocking work local.
- Delegate only bounded, disjoint, materially useful packets.
- Integrate packet results before final verification.

## Branching Rules
- If a packet finds a P0/P1 issue, fix immediately, add/adjust tests where feasible, then continue the same packet.
- If a finding is speculative or would require broad redesign, record it as residual risk rather than refactoring.
- If any release gate fails, root-cause and fix before final report.
- If a change would touch external services, stop and request approval.

## Packet Prompts
- `packet-1-cli-lifecycle`: Read all `cmd/*.go` command handlers and tests. Check command semantics, confirmation/destructive boundaries, output UX, duplicate or misleading helper code, and recent lifecycle behavior.
- `packet-2-dashboard-api-ui`: Review dashboard API/server/frontend. Check auth token enforcement, confirm contracts, SSE behavior, resource/log/metrics/events/domain flows, command preview consistency, and secret leakage.
- `packet-3-k8s-tunnel-runtime`: Review Kubernetes client and tunnel packages. Check managed labels, scope isolation, resource cleanup, NodePort/TCP/SSH/HTTPS resource behavior, secret metadata only, and performance of list/watch style calls.
- `packet-4-security-session-daemon`: Review auth/session/daemon/accesspolicy/publicauth packages. Check file permissions, symlink/path safety, token/password handling, TTLs, allow/deny matching, and goroutine/process lifecycle.
- `packet-5-deps-docs-skills-release`: Review Go/npm dependencies, scripts, Makefile, workflows, README/README_EN, skills and global sync. Check docs match actual behavior and no generated packages/docs are accidentally required for release commits.
- `packet-6-integration-gate`: Run full release gate, workflow validation, and final worktree audit.

## Completion Audit
- All packets have result notes.
- Accepted findings are patched or explicitly deferred with rationale.
- Full release gate passes.
- Final report names changed files, checks, and residual risks.
