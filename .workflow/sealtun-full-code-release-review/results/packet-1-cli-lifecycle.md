Packet: packet-1-cli-lifecycle
Status: completed

Reviewed:
- `cmd/init.go`, `cmd/discover.go`, `cmd/watch.go`, `cmd/resources.go`
- lifecycle commands: `apply`, `start`, `stop`, `cleanup`, `doctor`, `domain`, `expose`, `logout`, `daemon`
- command tests for apply, cleanup, doctor, dashboard, discover, init, resources, watch

Accepted findings:
- Documentation and skill references overstated `doctor --fix` behavior by saying it cleans `error` sessions. Implementation is intentionally more conservative and only generates cleanup actions for expired/stale eligible sessions. Fixed README/README_EN and skill references.

Deferred findings:
- Many older commands use `fmt.Printf` instead of `cmd.OutOrStdout()`. This is a testability/style issue, not a release blocker, and changing it broadly would be unrelated churn.

Verification:
- Targeted lifecycle behavior is covered by existing and prior-added tests.
