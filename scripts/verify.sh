#!/usr/bin/env bash
# verify.sh — local gate mirroring CI (ci.yml + release pipeline validate job).
# Anything that fails here would fail on GitHub Actions; run this BEFORE
# pushing so CI never gets to deliver bad news first. Raw exit codes only.
set -uo pipefail

fail=0
step() { echo ""; echo "== $1 =="; }

step "gofmt"
unformatted=$(gofmt -l cmd/ pkg/)
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  fail=1
fi

step "go vet (ci)"
go vet ./... || fail=1

step "go test (ci)"
go test ./... || fail=1

step "go test -race (ci package set)"
go test -race ./cmd ./pkg/session ./pkg/accesspolicy ./pkg/publicauth ./pkg/k8s ./pkg/tunnel ./pkg/daemon || fail=1

step "gosec (ci paths)"
gosec -quiet ./cmd ./pkg/... ./assets ./ || fail=1

step "go mod tidy -diff (ci)"
go mod tidy -diff || fail=1

step "node --check npm scripts (ci)"
node --check scripts/build-npm-packages.mjs || fail=1
node --check scripts/publish-npm-packages.mjs || fail=1

step "cross-compile (goreleaser targets: linux/windows/darwin x amd64/arm64)"
for goos in linux windows darwin; do
  for goarch in amd64 arm64; do
    echo "  GOOS=$goos GOARCH=$goarch"
    GOOS=$goos GOARCH=$goarch go build ./... || fail=1
  done
done

echo ""
if [ "$fail" != "0" ]; then
  echo "VERIFY FAILED"
  exit 1
fi
echo "VERIFY OK"
