# Packet 2: Dashboard/API/Security

## Scope

Reviewed dashboard API routes, token/confirm handling, command previews, resources/logs/metrics/events reads, active-scope restrictions, and Kubernetes resource redaction.

## Accepted Findings

- Dashboard `cleanup` mutation enforced token and confirm, but the backend did not independently reject active/non-cleanup-eligible tunnels. UI normally hides the button for active tunnels, but scripted API calls could still clean an active in-scope tunnel.

## Fixes

- Added shared `sessionCleanupEligible` helper.
- Updated CLI default cleanup and `cleanup [tunnel-id]` to treat stopped, expired, stale, or error sessions as cleanup-eligible and active sessions as protected.
- Updated dashboard `cleanupTunnelByID` to reject active/non-cleanup-eligible sessions before remote cleanup.
- Added regression tests for error-session cleanup, active cleanup refusal, and dashboard active cleanup refusal.

## Rejected / Non-Issues

- Token checks are present for `/api/watch`, `/api/discover`, and all `/api/*` paths reviewed.
- Remote dashboard HTML does not embed the dashboard token.
- Resources API uses active-scope checks and Secret resources only expose metadata/status/hints, not data.
- Dashboard command preview flags match current `sealtun expose` flags.

## Verification

- `/opt/homebrew/bin/go test ./cmd -run 'Test(Dashboard|Cleanup|Doctor)'` passed.
