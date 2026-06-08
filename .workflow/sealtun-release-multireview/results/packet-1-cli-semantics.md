# Packet 1: CLI Semantics

## Scope

Reviewed `init`, `resources`, `watch`, `doctor --fix`, `cleanup [tunnel-id]`, `start`, and `stop` command paths plus related tests.

## Accepted Findings

- `doctor --fix` could execute allowed actions, record per-action failures in the payload, and still return success from the CLI. This is not release-grade for automation.

## Fixes

- Added `doctorFixExecutionError` so non-dry-run `doctor --fix` returns non-zero when any allowed fix action fails, after still printing/encoding the result payload.
- Added regression coverage for failed-action error reporting and dry-run behavior.

## Rejected / Non-Issues

- `doctor --fix --dry-run` remains success even when planned actions would fail because it does not execute mutations.
- `cleanup --all <id>` conflict handling already exists and remains correct.
- `watch` returns non-zero on collector errors and keeps JSON event output for observability.

## Verification

- `/opt/homebrew/bin/go test ./cmd` passed.
