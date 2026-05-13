# Sealtun CLI Reference

Use this for interactive Sealtun operation: install, login, expose, secure, observe, bind domains, and clean up tunnels.

## Install

```bash
npm install -g sealtun
sealtun --version

npx sealtun@latest --version
npx sealtun@latest login
```

Direct binaries are published on GitHub Releases. The npm package installs a platform-specific optional binary package for macOS, Linux, or Windows on x64/amd64 and arm64.

## Login, Regions, Profiles

```bash
sealtun login
sealtun region list
sealtun region current
sealtun region use hzh

sealtun login gzg --profile gzg-main
sealtun profile list
sealtun profile current
sealtun profile save hzh-dev
sealtun profile use hzh-dev
sealtun profile delete hzh-dev
```

Known regions include `gzg`, `hzh`, `bja`, `cloud`, and `usw`. Login state, kubeconfig, and profiles live under `~/.sealtun`.

## Expose A Port

```bash
sealtun expose 3000
sealtun expose 3000 --foreground
sealtun expose 3000 --ready-timeout 2m
```

`expose` defaults to `https` and daemon mode. The daemon maintains the local side in the background. Use `--foreground` when the current terminal should own the tunnel lifecycle.

## Public Access Controls

Access controls are enforced by the Sealtun server proxy layer, independent of Ingress annotations. They apply to public business traffic, not `/_sealtun/ws`, health checks, or internal metrics protected by the tunnel secret.

Prefer environment variables for credentials:

```bash
export SEALTUN_BASIC_AUTH_PASSWORD='change-me'
sealtun expose 3000 \
  --basic-auth-user admin \
  --basic-auth-password-env SEALTUN_BASIC_AUTH_PASSWORD

export SEALTUN_BEARER_TOKEN='share-secret'
sealtun expose 3000 --bearer-token-env SEALTUN_BEARER_TOKEN

sealtun expose 3000 \
  --ip-allowlist 203.0.113.10,198.51.100.0/24 \
  --ip-denylist 198.51.100.9

export SEALTUN_TEMP_TOKEN='review-link-secret'
sealtun expose 3000 \
  --temporary-access-token-env SEALTUN_TEMP_TOKEN \
  --temporary-access-ttl 1h
```

One-shot forms exist, but warn that they can enter shell history:

```bash
sealtun expose 3000 --basic-auth admin:change-me
sealtun expose 3000 --bearer-token share-secret
sealtun expose 3000 --temporary-access-token review-link-secret --temporary-access-ttl 1h
```

Token constraints and behavior:

- Bearer and temporary tokens must be at least 8 characters.
- Stored runtime policy uses SHA-256 token hashes.
- Temporary access uses `?_sealtun_token=<token>` and strips that query parameter before forwarding upstream.
- IP rules accept individual IPs or CIDR ranges. Sealtun reads `X-Real-IP`, then the nearest valid `X-Forwarded-For`, then `RemoteAddr`.
- When Basic Auth and Bearer or temporary links are both configured, either authentication path can allow the request, subject to IP rules.

## Custom Domains

```bash
sealtun expose 3000 --domain app.example.com
sealtun expose 3000 --domain app.example.com --wait-domain --domain-timeout 5m

sealtun domain set <tunnel-id> app.example.com
sealtun domain verify <tunnel-id>
sealtun domain verify <tunnel-id> --wait --timeout 5m
sealtun domain status
sealtun domain doctor <tunnel-id>
sealtun domain clear <tunnel-id>
```

Sealtun keeps a generated Sealos host as the control-plane host and CNAME target. The user must configure:

```text
CNAME app.example.com -> <sealos-host>
```

Only after CNAME ownership verification does Sealtun write the custom host to Ingress and manage cert-manager resources.

## Observe And Manage

```bash
sealtun status
sealtun status --json

sealtun list
sealtun list --check
sealtun list --json

sealtun inspect <tunnel-id>
sealtun inspect <tunnel-id> --remote
sealtun inspect <tunnel-id> --json

sealtun logs <tunnel-id>
sealtun logs <tunnel-id> --tail 200 --follow
sealtun logs <tunnel-id> --since 10m

sealtun metrics <tunnel-id>
sealtun metrics <tunnel-id> --json

sealtun dashboard
sealtun dashboard --addr 127.0.0.1 --port 19777

sealtun doctor
sealtun doctor --json
```

Dashboard is local and read-only by default. `--allow-remote` allows a non-loopback dashboard address and should be treated as a security-sensitive choice.

## Stop And Clean Up

```bash
sealtun stop <tunnel-id>
sealtun cleanup
sealtun cleanup --all
sealtun logout
sealtun logout --force
```

`logout` first tries to clean up locally tracked tunnel resources before deleting credentials. Use `--force` only when cleanup cannot complete and local credentials must be removed anyway.
