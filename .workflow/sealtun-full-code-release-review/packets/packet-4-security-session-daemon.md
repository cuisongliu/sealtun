Packet ID: packet-4-security-session-daemon
Objective: Review security-sensitive state: auth, sessions, daemon, access policy, public auth.
Files / sources: pkg/auth, pkg/session, pkg/daemon, pkg/accesspolicy, pkg/publicauth, related cmd files.
Do: Check permissions, path handling, token/password redaction, TTL/expiry, allow/deny matching, process lifecycle.
Do not: Read or expose real secrets in notes.
Expected output: Result note with accepted/deferred findings and verification.
Verification: package tests and race tests.
