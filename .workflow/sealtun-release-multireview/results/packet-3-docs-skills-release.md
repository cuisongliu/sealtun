# Packet 3: Docs/Skills/Release

## Scope

Reviewed README/README_EN parity, Sealtun skill references, description length, global skill sync, Makefile/npm script, GitHub Actions release split, gitignore and untracked release boundaries.

## Accepted Findings

- Documentation and skill references described cleanup as stopped/expired/stale only, while reviewed code now also treats error-state tunnels as cleanup-eligible. This would confuse users and agents.

## Fixes

- Updated README and README_EN doctor/cleanup text to include error tunnels.
- Updated `skills/sealtun/SKILL.md`, `references/cli.md`, and `references/troubleshooting.md`.
- Synced repo skill to `/Users/sealos/.codex/skills/sealtun` and `/Users/sealos/.agents/skills/sealtun`.

## Rejected / Non-Issues

- Release workflows still match the existing strategy: branch push/PR validates only; tag push publishes GitHub Release and GHCR.
- `packages/` and `homepage/` remain gitignored generated artifacts.
- `docs/` and `.workflow/` are local untracked artifacts and should not be staged unless explicitly requested.

## Verification

- Skill description length is 271 characters in repo, Codex global skill, and agents global skill.
- `diff -qr` confirms repo skill matches both global skill directories.
- README/skill stale-only cleanup wording scan returned no stale-only matches.
