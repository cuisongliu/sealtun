---
name: sealtun
description: "Use this skill for Sealtun-specific local-to-public tunnel work or Sealtun repo maintenance/release. Trigger for sealtun, sealtun.yaml, Sealos tunnel, ngrok/cloudflared-style tunnel, expose localhost/local port/local dev server, public HTTPS URL/domain for local app, public SSH/TCP tunnel, NodePort SSH, ProxyCommand fallback, webhook/payment/OAuth/bot callback to local service, preview/demo link, custom domain/CNAME, Basic Auth, Bearer token, IP allowlist/denylist, temporary access links, ttl auto-expire, apply/diff multi-tunnel config, stop/start/resume, cleanup, daemon/session/logs/metrics/dashboard/doctor, npm binary packages, GitHub Release, GoReleaser, GHCR. Chinese triggers: 内网穿透, 本地服务公网访问, 本地端口暴露, localhost 暴露到公网, 公网预览链接, 公网域名, 公网 SSH, SSH 隧道, TCP 隧道, 第三方回调到本地, 隧道认证, 访问控制, 声明式配置, 发版. Do not use for generic Kubernetes/Ingress/DNS/SSH unless Sealtun is involved."
---

# Sealtun

## First Decision

Classify the request before answering or editing:

- User operation: install, login, expose HTTPS or SSH, secure public HTTP traffic, bind a domain, inspect state, stop/start/resume, clean up, or use the dashboard. Read `references/cli.md`.
- Declarative configuration: `sealtun.yaml`, `apply -f`, `diff -f`, multi-tunnel management, stable names, `ttl`, HTTPS access policies, or SSH tunnel declarations. Read `references/declarative.md`.
- Troubleshooting: login/profile mismatch, daemon/session issues, local port failures, SSH direct TCP/NodePort problems, remote Kubernetes problems, DNS, Ingress, certificate, logs, metrics, or dashboard behavior. Read `references/troubleshooting.md`.
- Maintainer release work: GitHub tag release, GoReleaser, GHCR Docker image, npm packages, `packages/`, or publish dry-runs. Read `references/release.md`.

If the request is inside the Sealtun repository, prefer the current source tree and README over these references when they conflict. Use `rg` to inspect Cobra commands, flags, Makefile targets, workflows, and tests before changing repo behavior.

## Required Execution Flow

Follow this flow after the skill triggers:

1. Scope gate: verify the request is about making a local/dev service publicly reachable, operating a Sealtun tunnel, troubleshooting Sealtun, declarative Sealtun config, or Sealtun release/npm work. If it is only generic production deployment, buying a domain, or DNS-only configuration without local-service tunneling, do not force Sealtun into the answer.
2. Select one mode before acting:
   - Guidance mode: user asks how to use Sealtun. Load the matching reference and give commands; do not run live tunnel/cloud/npm commands.
   - Live operation mode: user explicitly asks to execute, create, apply, stop, clean up, bind a domain, or publish. Run preflight checks first, then the requested command, then verification.
   - Repository change mode: user asks to modify Sealtun. Inspect source with `rg`, edit narrowly, run focused tests, then summarize changed files and verification.
   - Troubleshooting mode: user reports a problem. Run non-mutating diagnostics first, identify the likely layer, then propose or perform fixes only when the requested action is clear.
   - Release mode: user asks to release or publish npm. Run tests and dry-runs first when available, wait for required GitHub release assets before npm publishing, and verify final package/release state.
3. Gather minimum context. Inside this repo, inspect current code/README before relying on references. Outside the repo, use the references as the command source. Prefer non-mutating checks such as `sealtun --version`, `sealtun status`, `sealtun profile current`, `sealtun region current`, `sealtun list`, `sealtun inspect`, and `sealtun doctor`.
4. Control mutations. Do not run `sealtun expose`, `sealtun apply`, `sealtun domain set/clear`, `sealtun stop`, `sealtun cleanup`, `sealtun logout`, `git tag`, `git push`, or `npm publish` unless the user explicitly asked for execution or release work in the current task. For declarative changes, prefer `apply --dry-run` and `diff` before real `apply`.
5. Verify completion. After live operations, inspect the resulting tunnel/session/domain/release/npm package state. After code changes, run tests relevant to the touched code. Report the exact command sequence and final state, without printing secrets.

## Operating Rules

- Do not expose user secrets in final answers, logs, commits, or generated docs. Prefer `*Env` fields and environment variables for passwords and tokens unless the user explicitly wants a one-shot inline example.
- Explain that Sealtun public access controls are enforced in the Sealtun server proxy layer, not by Ingress annotations. They protect HTTPS public business traffic, not the internal `/_sealtun/ws` control channel and not SSH direct TCP NodePort traffic.
- For SSH exposure, prefer `sealtun expose 22 --protocol ssh` when the region supports public TCP NodePort. Use `sealtun ssh connect <tunnel-id>` only as a WebSocket ProxyCommand fallback.
- For declarative work, run or recommend `sealtun apply -f sealtun.yaml --dry-run` and `sealtun diff -f sealtun.yaml` before a real apply when feasible.
- For release or npm publishing, verify tests and dry-runs first when feasible. Release/GHCR publishing is tag-driven; master or PR CI builds and tests without publishing release or GHCR artifacts.
- Treat `packages/` and `homepage/` as generated local directories in this project; they are ignored and should not be committed unless the repo policy changes.
- Use exact command names and flags from the repository when modifying instructions. Supported tunnel protocols are `https` and the dedicated `ssh` mode; generic TCP/UDP/gRPC are not supported unless the repo adds them.

## Response Shape

For usage questions, give a short working command sequence and explain only the relevant gotchas. For repo changes, implement the change, run focused tests, then summarize changed files and verification. For troubleshooting, start with the lowest-cost local checks, then escalate to remote Kubernetes diagnostics.
