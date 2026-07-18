#!/usr/bin/env bash
# Builds Linux release binaries for baseline AMD64 CPUs.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export GOAMD64=v1

printf 'Building servika-server (GOAMD64=%s, CGO_ENABLED=%s)\n' "$GOAMD64" "$CGO_ENABLED"
go build -trimpath -o assets/servika-server ./cmd/server

printf 'Building servika-seed-admin (GOAMD64=%s, CGO_ENABLED=%s)\n' "$GOAMD64" "$CGO_ENABLED"
go build -trimpath -o assets/servika-seed-admin scripts/seed_admin.go

for binary in assets/servika-server assets/servika-seed-admin; do
  if ! go version -m "$binary" | grep -Eq '^[[:space:]]*build[[:space:]]+GOAMD64=v1$'; then
    printf 'Error: %s was not built with GOAMD64=v1\n' "$binary" >&2
    exit 1
  fi
done

printf 'Release binaries built and verified for GOAMD64=v1.\n'
