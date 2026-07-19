#!/usr/bin/env bash
# Builds Linux release binaries for amd64 and arm64.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

export CGO_ENABLED=0
export GOOS=linux

# ── linux/amd64 ──────────────────────────────────────────────────────────
export GOARCH=amd64
export GOAMD64=v1

printf 'Building servika-server (linux/amd64, GOAMD64=%s)\n' "$GOAMD64"
mkdir -p assets/linux_amd64
go build -trimpath -o assets/linux_amd64/servika-server ./cmd/server

printf 'Building servika-seed-admin (linux/amd64, GOAMD64=%s)\n' "$GOAMD64"
go build -trimpath -o assets/linux_amd64/servika-seed-admin scripts/seed_admin.go

for binary in assets/linux_amd64/servika-server assets/linux_amd64/servika-seed-admin; do
  if ! go version -m "$binary" | grep -Eq '^[[:space:]]*build[[:space:]]+GOAMD64=v1$'; then
    printf 'Error: %s was not built with GOAMD64=v1\n' "$binary" >&2
    exit 1
  fi
done
printf 'Release binaries built and verified for linux/amd64 (GOAMD64=v1).\n'

# ── linux/arm64 ──────────────────────────────────────────────────────────
export GOARCH=arm64
unset GOAMD64

printf '\nBuilding servika-server (linux/arm64)\n'
mkdir -p assets/linux_arm64
go build -trimpath -o assets/linux_arm64/servika-server ./cmd/server

printf 'Building servika-seed-admin (linux/arm64)\n'
go build -trimpath -o assets/linux_arm64/servika-seed-admin scripts/seed_admin.go

for binary in assets/linux_arm64/servika-server assets/linux_arm64/servika-seed-admin; do
  if ! go version -m "$binary" | grep -Eq '^[[:space:]]*build[[:space:]]+GOARCH=arm64$'; then
    printf 'Error: %s was not built with GOARCH=arm64\n' "$binary" >&2
    exit 1
  fi
done
printf 'Release binaries built and verified for linux/arm64.\n'

echo ''
printf 'All release binaries ready: linux/amd64 + linux/arm64.\n'
