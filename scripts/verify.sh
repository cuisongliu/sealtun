#!/usr/bin/env bash
# verify.sh — the single local gate that cannot lie about failures.
# Runs format check, vet, full tests, and gosec with raw exit codes.
set -uo pipefail

fail=0

echo "== gofmt =="
unformatted=$(gofmt -l cmd/ pkg/)
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  fail=1
fi

echo "== go vet =="
go vet ./... || fail=1

echo "== go test =="
go test ./... || fail=1

echo "== gosec =="
gosec -quiet ./... || fail=1

if [ "$fail" != "0" ]; then
  echo "VERIFY FAILED"
  exit 1
fi
echo "VERIFY OK"
