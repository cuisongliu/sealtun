# Sealtun Release And npm Publishing

Use this for maintainer operations: tests, tag release, GitHub Release, GHCR Docker image, and npm binary package publishing.

## Repository Policy

- Pushes and PRs to `main` or `master` run tests, GoReleaser snapshot, and Docker build without publishing GitHub Release or GHCR artifacts.
- Pushing a `v*.*.*` tag publishes GitHub Release artifacts through GoReleaser and pushes GHCR Docker images.
- The release workflow checks that the tag commit is contained in `origin/main` or `origin/master`.
- Generated `packages/` and `homepage/` directories are ignored by git and are not expected in master.

## Pre-Release Checks

```bash
go test ./...
go vet ./...
node --check scripts/build-npm-packages.mjs
make build
./sealtun --version
```

Use focused tests for changed packages during development, then full checks before tagging.

## Tag Release

```bash
git status --short
git push origin master
git tag vX.Y.Z
git push origin vX.Y.Z
```

After pushing the tag, wait for GitHub Actions to complete the Release workflow. GoReleaser creates assets named like:

```text
sealtun_darwin_amd64.tar.gz
sealtun_darwin_arm64.tar.gz
sealtun_linux_amd64.tar.gz
sealtun_linux_arm64.tar.gz
sealtun_windows_amd64.zip
sealtun_windows_arm64.zip
```

The Docker workflow publishes `ghcr.io/gitlayzer/sealtun:<version>` and `latest` for tag releases.

## npm Package Generation

The npm flow mirrors sharp-style optional binary packages. `scripts/build-npm-packages.mjs` downloads release assets and generates:

- Main package in `packages/`, default name `sealtun`.
- Platform packages in `packages/<target>/`, default scope `@gitlayzer`, such as `@gitlayzer/sealtun-darwin-arm64`.
- Main package `bin/sealtun.js` launcher selecting the correct optional dependency at runtime.

Generate local package files:

```bash
NPM_VERSION=X.Y.Z NPM_RELEASE_TAG=vX.Y.Z make npm-packages
```

Create local tarballs:

```bash
NPM_VERSION=X.Y.Z NPM_RELEASE_TAG=vX.Y.Z make npm-pack
```

Dry-run publish:

```bash
NPM_VERSION=X.Y.Z NPM_RELEASE_TAG=vX.Y.Z make npm-publish-dry-run
```

Publish:

```bash
NPM_VERSION=X.Y.Z NPM_RELEASE_TAG=vX.Y.Z make npm-publish
```

`make npm-publish` publishes platform packages first, then the main package. It passes `--access public` by default. `NPM_DIST_TAG`, `NPM_PACKAGE_NAME`, `NPM_BINARY_PACKAGE_SCOPE`, `NPM_GITHUB_REPO`, and `NPM_PACKAGES_DIR` can override defaults.

## npm Publishing Safety

- Confirm `npm whoami` and 2FA readiness before publishing.
- Use the dry-run target first when feasible.
- If a publish partially succeeds, inspect npm package versions before retrying. Retrying an already published version will fail for that package; do not bump versions blindly without confirming the release state.
- The GitHub Release assets must exist before npm generation, because the npm builder downloads them from the release.

## Cleanup

```bash
make npm-clean
```

This removes generated `packages/` content only.
