#!/usr/bin/env sh
# Copyright (c) 2026 Tiago de Carvalho Vilas Boas. SPDX-License-Identifier: BUSL-1.1
#
# harness-downshift installer
# Downloads the latest (or a specific) release binary for your OS and arch.
#
# Usage:
#   # Latest release:
#   curl -fsSL https://raw.githubusercontent.com/tiagovilasboas/harness-downshift/main/install.sh | sh
#
#   # Specific version:
#   curl -fsSL https://raw.githubusercontent.com/tiagovilasboas/harness-downshift/main/install.sh | sh -s v0.1.0-beta.1
#
# The binary is installed to /usr/local/bin/downshift (or ~/bin/downshift if
# /usr/local/bin is not writable without sudo).

set -e

REPO="tiagovilasboas/harness-downshift"
BINARY="downshift"

# ── Resolve version ───────────────────────────────────────────────────────────
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
fi
if [ -z "$VERSION" ]; then
  echo "error: could not determine latest version. Pass a version explicitly:" >&2
  echo "  sh install.sh v0.1.0-beta.1" >&2
  exit 1
fi

# ── Detect OS and arch ────────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux)  OS="linux"  ;;
  darwin) OS="darwin" ;;
  *)
    echo "error: unsupported OS: $OS" >&2
    echo "Install via Go toolchain instead:" >&2
    echo "  go install github.com/${REPO}/cmd/downshift@${VERSION}" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64 | amd64)  ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)
    echo "error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# ── Build download URL ────────────────────────────────────────────────────────
# Strip leading 'v' from version for the archive name
VER="${VERSION#v}"
ARCHIVE="downshift_${VER}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

# ── Choose install dir ────────────────────────────────────────────────────────
if [ -w "/usr/local/bin" ]; then
  INSTALL_DIR="/usr/local/bin"
elif [ -d "$HOME/.local/bin" ]; then
  INSTALL_DIR="$HOME/.local/bin"
elif [ -d "$HOME/bin" ]; then
  INSTALL_DIR="$HOME/bin"
else
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

# ── Download and install ──────────────────────────────────────────────────────
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Downloading harness-downshift ${VERSION} (${OS}/${ARCH})..."
curl -fsSL "$URL" -o "$TMP/$ARCHIVE"

echo "Extracting..."
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"

echo "Installing to ${INSTALL_DIR}/downshift ..."
mv "$TMP/$BINARY" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "✓ harness-downshift ${VERSION} installed to ${INSTALL_DIR}/downshift"
echo ""

# Verify
if command -v downshift >/dev/null 2>&1; then
  downshift --help 2>/dev/null | head -3 || true
else
  echo "⚠️  '${INSTALL_DIR}' is not in your PATH."
  echo ""
  echo "Add it now (current session):"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  echo ""
  echo "Add it permanently (zsh):"
  echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
  echo ""
  echo "Or move the binary to a directory already in PATH:"
  echo "  sudo mv ${INSTALL_DIR}/downshift /usr/local/bin/downshift"
fi
