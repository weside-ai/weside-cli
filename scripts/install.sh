#!/bin/sh
# Install weside CLI
# Usage: curl -fsSL https://raw.githubusercontent.com/weside-ai/weside-cli/main/scripts/install.sh | sh

set -e

REPO="weside-ai/weside-cli"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="weside"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest release tag
echo "Fetching latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest release"
  exit 1
fi

echo "Installing $BINARY_NAME $LATEST ($OS/$ARCH)..."

# Download archive
ARCHIVE="${BINARY_NAME}_${LATEST#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$LATEST/$ARCHIVE"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$URL" -o "$TMP_DIR/$ARCHIVE"
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

# Install binary
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
else
  sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
fi

chmod +x "$INSTALL_DIR/$BINARY_NAME"

echo ""
echo "$BINARY_NAME $LATEST installed to $INSTALL_DIR/$BINARY_NAME"
echo ""
echo "Get started:"
echo "  weside auth login"
echo "  weside companions list"
echo "  weside chat -m 'Hello!'"
