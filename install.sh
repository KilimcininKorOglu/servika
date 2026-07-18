#!/usr/bin/env bash
# Servika one-line installation bootstrap
#   curl -fsSL https://raw.githubusercontent.com/servika/servika/main/install.sh | bash
#
# This bootstrap downloads the complete repository, including the installer,
# prebuilt binaries, and configuration files, then runs servika-install.sh.
set -euo pipefail

REPO="servika/servika"
BRANCH="main"

c_b="\033[1;34m"; c_g="\033[32m"; c_r="\033[31m"; c_0="\033[0m"
[ -t 1 ] || { c_b=; c_g=; c_r=; c_0=; }

[ "$(id -u)" = 0 ] || { echo -e "${c_r}✗ root is required:  curl ... | sudo bash${c_0}"; exit 1; }
command -v curl >/dev/null 2>&1 || { echo -e "${c_r}✗ curl is required${c_0}"; exit 1; }
command -v tar  >/dev/null 2>&1 || { echo -e "${c_r}✗ tar is required${c_0}"; exit 1; }

echo -e "${c_b}══ Downloading Servika (github.com/$REPO) ══${c_0}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
if ! curl -fsSL "https://codeload.github.com/$REPO/tar.gz/refs/heads/$BRANCH" | tar xz -C "$TMP"; then
  echo -e "${c_r}✗ download failed, is the repository public and does the $BRANCH branch exist?${c_0}"; exit 1
fi
SRC=$(find "$TMP" -maxdepth 1 -type d -name "*-$BRANCH" | head -1)
[ -z "$SRC" ] && SRC=$(find "$TMP" -maxdepth 1 -mindepth 1 -type d | head -1)
cd "$SRC" || { echo -e "${c_r}✗ package could not be opened${c_0}"; exit 1; }
chmod +x servika-install.sh assets/servika-server assets/servika-seed-admin assets/ops/* 2>/dev/null || true

echo -e "${c_g}✓ downloaded, starting installation${c_0}\n"
exec bash servika-install.sh "$@"
