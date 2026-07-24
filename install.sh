#!/usr/bin/env bash
# Servika one-line installation bootstrap
#   curl -fsSL https://raw.githubusercontent.com/KilimcininKorOglu/servika/main/install.sh | bash
#
# This bootstrap downloads the latest published release bundle for the host
# architecture from GitHub Releases, then runs servika-install.sh from it.
# Set SERVIKA_RELEASE_TAG to install a specific release, e.g.
#   SERVIKA_RELEASE_TAG=v1.0.0 bash install.sh
# Any remaining arguments (such as --admin-password) are forwarded to the installer.
set -euo pipefail

REPO="KilimcininKorOglu/servika"

c_b="\033[1;34m"; c_g="\033[32m"; c_r="\033[31m"; c_0="\033[0m"
[ -t 1 ] || { c_b=; c_g=; c_r=; c_0=; }

[ "$(id -u)" = 0 ] || { echo -e "${c_r}✗ root is required:  curl ... | sudo bash${c_0}"; exit 1; }
command -v curl >/dev/null 2>&1 || { echo -e "${c_r}✗ curl is required${c_0}"; exit 1; }
command -v tar  >/dev/null 2>&1 || { echo -e "${c_r}✗ tar is required${c_0}"; exit 1; }

MACHINE=$(uname -m)
case "$MACHINE" in
  x86_64)  ARCH=linux_amd64 ;;
  aarch64) ARCH=linux_arm64 ;;
  *)       echo -e "${c_r}✗ unsupported architecture: $MACHINE (expected x86_64 or aarch64)${c_0}"; exit 1 ;;
esac

TAG="${SERVIKA_RELEASE_TAG:-}"
if [ -z "$TAG" ]; then
  TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)
fi
[ -n "$TAG" ] || { echo -e "${c_r}✗ could not determine the latest release tag${c_0}"; exit 1; }
VERSION="${TAG#v}"

URL="https://github.com/${REPO}/releases/download/${TAG}/servika-${VERSION}-${ARCH}.tar.gz"
echo -e "${c_b}══ Downloading Servika ${VERSION} (${ARCH}) ══${c_0}"

TMP=$(mktemp -d)
if ! curl -fsSL "$URL" | tar xz -C "$TMP"; then
  echo -e "${c_r}✗ download failed: $URL${c_0}"; rm -rf "$TMP"; exit 1
fi

cd "$TMP"
chmod +x servika-install.sh "assets/$ARCH/servika-server" "assets/$ARCH/servika-seed-admin" assets/ops/* 2>/dev/null || true
echo -e "${c_g}✓ downloaded, starting installation${c_0}\n"
exec bash servika-install.sh "$@"
