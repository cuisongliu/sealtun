# Sealtun Full Code Release Review

## Goal
Review the entire `/Users/sealos/sealtun` codebase for release readiness, not only the latest changes. Remove or fix redundant code, unnecessary dependencies, risky behavior, and safe optimization opportunities without damaging existing CLI/dashboard behavior.

## Success Criteria
- Every production module is reviewed at least once by ownership area.
- Found P0/P1 correctness, security, lifecycle, or UX issues are fixed with focused changes.
- Redundant or misleading code/docs introduced by recent work are removed or corrected.
- Skills are synchronized after any skill change.
- The full release gate passes:
  - `go test ./...`
  - `go vet ./...`
  - `go test -race ./cmd ./pkg/session ./pkg/accesspolicy ./pkg/publicauth ./pkg/k8s`
  - `go build`
  - `git diff --check`
  - `node --check scripts/build-npm-packages.mjs`
  - workflow artifact validation

## Current Context
- Branch: `master`.
- Existing local changes include new CLI/dashboard features and prior release-review fixes.
- `.workflow/*` artifacts and `docs/marketing/*` are local/untracked and must not be treated as release source unless explicitly requested.

## Constraints
- Do not commit, push, publish, or run external release operations unless the user asks.
- Do not revert unrelated local changes.
- Prefer surgical fixes over broad refactors.
- Do not weaken dashboard token/confirm protections or secret redaction.

## Risks
- Accidental cleanup semantics drift could delete active or diagnostically useful tunnels.
- Dashboard read/write API changes can affect remote mutation safety.
- Kubernetes resource rendering must not leak Secret data.
- Skill descriptions must stay short enough for Codex/Agents validation.

## Approval Required
No approval is required for local review, tests, formatting, workflow notes, or safe source edits. Approval is required for npm/GitHub release, push, tag, destructive Kubernetes operations, or modifying external accounts.

## Work Packets
- `packet-1-cli-lifecycle`: CLI commands, lifecycle semantics, cleanup/start/stop/init/resources/watch/doctor.
- `packet-2-dashboard-api-ui`: dashboard routes, auth/confirm, SSE, command preview, frontend state.
- `packet-3-k8s-tunnel-runtime`: Kubernetes client/resource handling, tunnel provisioning, TCP/SSH/HTTPS behavior.
- `packet-4-security-session-daemon`: auth, sessions, daemon, access policy, public auth, files and secrets.
- `packet-5-deps-docs-skills-release`: dependencies, scripts, workflows, README, skills, generated package boundaries.
- `packet-6-integration-gate`: integrated fixes, regression tests, release gate.

## Integration Policy
Packet findings are accepted only when they are backed by source inspection or tests. Changes must be small, traceable to a packet finding, and verified before moving to final gate.

## Verification
Run narrow tests after fixes, then full release gate. Record final command evidence in `final-report.md`.

## Reusable Artifacts
Keep this workflow as a repeatable release-review recipe for future Sealtun release cycles.
