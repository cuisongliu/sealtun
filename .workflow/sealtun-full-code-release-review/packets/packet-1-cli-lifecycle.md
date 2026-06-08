Packet ID: packet-1-cli-lifecycle
Objective: Review CLI command lifecycle behavior and recent command additions for release readiness.
Files / sources: cmd/*.go, cmd/*_test.go.
Do: Check destructive semantics, command output, helper duplication, invalid input handling, tests.
Do not: Refactor unrelated command style or change public CLI contracts without a bug.
Expected output: Result note with accepted/deferred findings and verification.
Verification: Targeted cmd tests plus full gate.
