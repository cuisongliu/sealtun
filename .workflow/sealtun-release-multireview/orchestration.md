# Orchestration: Sealtun Release Multi Review

## Execution Rules

- Keep the original objective intact.
- Ask for approval before risky, expensive, external, or destructive actions.
- Keep immediate blocking work local.
- Delegate only bounded, disjoint, materially useful packets.
- Integrate packet results before final verification.

## Branching Rules
- If a packet finds a P0/P1 issue, stop broad scanning, fix it, add a regression test, run narrow tests, then resume review.
- If a finding is speculative and not backed by code or behavior, record it as residual risk rather than changing code.
- If a check requires external systems, skip unless explicitly approved and report the skipped scope.
- If workflow files become untracked, keep them local unless the user asks to commit them.

## Packet Prompts
### Packet 1: CLI Semantics
Objective: Review init/resources/watch/doctor/cleanup/start/stop for bad defaults, wrong exits, data loss, and missing tests.
Files: cmd/init.go, cmd/resources.go, cmd/watch.go, cmd/doctor.go, cmd/cleanup.go, cmd/start.go, cmd/stop.go, related tests, pkg/session.
Expected output: findings, accepted fixes, tests run.

### Packet 2: Dashboard/API/Security
Objective: Review dashboard write confirmations, token handling, command previews, resources API, secret redaction, active scope restrictions.
Files: cmd/dashboard.go, cmd/dashboard_api.go, cmd/dashboard_test.go, pkg/k8s/client.go, pkg/k8s/client_test.go.
Expected output: findings, accepted fixes, tests run.

### Packet 3: Docs/Skills/Release
Objective: Review README/README_EN parity, skills precision and length, global sync, release scripts/workflows.
Files: README.md, README_EN.md, skills/sealtun, Makefile, .github/workflows, scripts/build-npm-packages.mjs, .gitignore.
Expected output: findings, accepted fixes, checks run.

### Packet 4: Integration
Objective: Integrate packet results, rerun full release gate, summarize remaining risks and release verdict.
Files: all modified files and workflow state.
Expected output: final report and release gate evidence.

## Completion Audit
- All packets have result notes.
- Any accepted finding has a code/doc/test fix.
- Full release gate passes after final edits.
- Final answer states whether release-ready and lists evidence.
