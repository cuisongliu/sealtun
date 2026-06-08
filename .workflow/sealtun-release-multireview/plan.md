# Sealtun Release Multi Review

## Goal
Review the current Sealtun working tree multiple times, fix release-blocking issues, and produce a release-ready verdict.

## Success Criteria
- Current CLI/dashboard/skills changes are internally consistent.
- No P0/P1 security, data-loss, or severe UX issues remain in the reviewed surface.
- Any issue found during review is fixed with focused tests.
- Full release gate passes: Go tests, vet, race subset, build, diff check, npm packaging syntax check, skill sync/description validation.
- No generated docs/packages/homepage artifacts are staged or treated as release source.

## Current Context
- Branch: master.
- Current changes include README/README_EN, dashboard command preview, doctor --fix, init/resources/watch commands, cleanup [tunnel-id], and skill updates.
- docs/ is currently untracked and should remain outside release scope.
- packages/ and homepage/ are gitignored generated artifacts.

## Constraints
- Do not commit, push, publish, or mutate external systems.
- Use current source tree as authority.
- Keep edits surgical and directly tied to release readiness.
- Do not print secrets.

## Risks
- Over-cleaning active daemon tunnels.
- Dashboard command previews diverging from real CLI/API behavior.
- New read APIs leaking Secret data or cross-scope resources.
- CLI commands returning success on error states.
- Skill description becoming too broad or too long.

## Approval Required
No approval required for local review, local test execution, and focused file edits. Approval would be required for commit, push, release, npm publish, external tunnel creation, or destructive cleanup against real resources.

## Work Packets
1. CLI semantics packet: init/resources/watch/doctor/cleanup/start/stop behavior and tests.
2. Dashboard/API/security packet: token/confirm, command preview, read endpoints, secret handling, scope handling.
3. Docs/skills/release packet: README parity, skill precision/sync, workflow files, release gate.
4. Final integration packet: accept/reject findings, rerun gates, final release verdict.

## Integration Policy
Accept only findings backed by current source or failing checks. Fix issues directly when local and low risk. Re-run narrow checks after fixes, then full gate.

## Verification
- rg/static review over changed and related files.
- go test ./cmd after fixes.
- go test ./...
- go vet ./...
- go test -race ./cmd ./pkg/session ./pkg/accesspolicy ./pkg/publicauth ./pkg/k8s
- go build
- git diff --check
- node --check scripts/build-npm-packages.mjs
- skill diff sync and description length checks.
- local smoke commands with temporary HOME where useful.

## Reusable Artifacts
This workflow artifact can be reused as the release multi-review recipe for future Sealtun releases.
