Packet ID: packet-3-docs-skills-release
Objective: Review README parity, skill precision and sync, release scripts/workflows, gitignore boundaries, and release gate commands.
Context: Repo has README/README_EN, repo/global skills, tag-only release workflow, generated packages/homepage ignored, and untracked docs/workflow artifacts.
Files / sources: README.md, README_EN.md, skills/sealtun, Makefile, scripts/build-npm-packages.mjs, .github/workflows, .gitignore.
Ownership: Documentation, skill behavior, and release process consistency only.
Do: Fix inconsistencies with current CLI behavior, validate description length and sync.
Do not: Commit generated packages, docs, homepage, or workflow artifacts unless explicitly requested.
Expected output: result note under results/packet-3-docs-skills-release.md.
Verification: skill diff sync, description length, docs wording scan, release gate.
