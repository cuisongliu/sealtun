Packet: packet-6-integration-gate
Status: completed

Accepted fixes integrated:
- Corrected `doctor --fix` docs/skills to match conservative implementation: automatic cleanup is expired/stale only, while manual `cleanup` can target stopped/expired/stale/error eligible tunnels.
- Added temp-file cleanup on session and daemon atomic-write rename failures.

Full release gate:
- `/opt/homebrew/bin/go test ./...` passed.
- `/opt/homebrew/bin/go vet ./...` passed.
- `/opt/homebrew/bin/go test -race ./cmd ./pkg/session ./pkg/accesspolicy ./pkg/publicauth ./pkg/k8s` passed.
- `/opt/homebrew/bin/go build` passed.
- `git diff --check` passed.
- `node --check scripts/build-npm-packages.mjs` passed.
- `go mod tidy -diff` produced no diff.
- `python3 /Users/sealos/.codex/skills/codex-dynamic-workflows/scripts/verify_workflow.py .workflow/sealtun-full-code-release-review` passed.
- `diff -qr skills/sealtun /Users/sealos/.codex/skills/sealtun` passed.
- `diff -qr skills/sealtun /Users/sealos/.agents/skills/sealtun` passed.

Residual non-release items:
- Ignored local `.DS_Store` files exist and are not part of release source.
- `.workflow/*` artifacts and `docs/marketing/*` are local untracked artifacts and should not be staged unless explicitly wanted.
