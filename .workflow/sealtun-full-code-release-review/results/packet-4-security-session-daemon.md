Packet: packet-4-security-session-daemon
Status: completed

Reviewed:
- `pkg/auth`
- `pkg/session`
- `pkg/daemon`
- `pkg/accesspolicy`
- `pkg/publicauth`

Accepted findings:
- Session and daemon atomic writes left temporary files behind if `os.Rename` failed. Fixed both paths to remove the temp file on rename failure.

Security notes:
- Config/session/daemon paths reject symlinks and non-regular files.
- Config directories are restricted to 0700 and sensitive files to 0600.
- Basic Auth uses bcrypt with legacy SHA-256 migration support.
- Access tokens and temporary tokens are hashed and matched in constant time.

Verification:
- `/opt/homebrew/bin/go test ./pkg/session ./pkg/daemon` passed after the temp-file cleanup patch.
