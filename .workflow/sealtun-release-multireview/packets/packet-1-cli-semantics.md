Packet ID: packet-1-cli-semantics
Objective: Review init/resources/watch/doctor/cleanup/start/stop for bad defaults, wrong exits, data loss, and missing tests.
Context: Current Sealtun release candidate includes guided init, resources, watch, doctor --fix, cleanup [tunnel-id], stop/start semantics.
Files / sources: cmd/init.go, cmd/resources.go, cmd/watch.go, cmd/doctor.go, cmd/cleanup.go, cmd/start.go, cmd/stop.go, cmd/session_helpers.go, related tests, pkg/session.
Ownership: CLI behavior and local session lifecycle only.
Do: Find P0/P1 behavior risks, fix with focused tests, run narrow checks.
Do not: Mutate real cloud resources, publish, stage, commit, or alter unrelated modules.
Expected output: result note under results/packet-1-cli-semantics.md.
Verification: go test ./cmd.
