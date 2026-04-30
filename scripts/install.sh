#!/bin/sh
# install.sh — download and install the latest em release
set -e

REPO=danlafeir/em
BINARY=em
INSTALL_DIR=~/.local/bin

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS=linux  ;;
  darwin) OS=darwin ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect ARCH
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)   ARCH=amd64 ;;
  arm64|aarch64)  ARCH=arm64 ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

ASSET_NAME="em-${OS}-${ARCH}"

# Fetch the latest GitHub Release and extract the download URL for this platform
RELEASES_URL="https://api.github.com/repos/${REPO}/releases/latest"
RELEASE_JSON=$(curl -sSfL "$RELEASES_URL" 2>/dev/null || true)

if [ -n "$RELEASE_JSON" ]; then
  # Parse the browser_download_url for the matching asset.
  # python3 is available on macOS and most Linux distributions.
  DOWNLOAD_URL=$(printf '%s' "$RELEASE_JSON" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for a in data.get('assets', []):
    if a['name'] == '${ASSET_NAME}':
        print(a['browser_download_url'])
        break
" 2>/dev/null || true)
fi

if [ -z "$DOWNLOAD_URL" ]; then
  echo "No GitHub Release found for ${ASSET_NAME}. Falling back to legacy bin/ path..." >&2
  # Legacy path: binaries committed directly to bin/ with a git-hash suffix.
  # This supports users upgrading from pre-release builds.
  API_URL="https://api.github.com/repos/${REPO}/contents/bin/"
  FILENAME=$(curl -sSL "$API_URL" 2>/dev/null | \
    grep -o '"name": *"'"${BINARY}-${OS}-${ARCH}"'-[a-zA-Z0-9]*"' | \
    sed 's/.*: *"//;s/"//' | sort | tail -n1)
  if [ -z "$FILENAME" ]; then
    echo "Could not find a binary for ${OS}/${ARCH} via either path." >&2
    exit 1
  fi
  DOWNLOAD_URL="https://raw.githubusercontent.com/${REPO}/main/bin/${FILENAME}"
fi

mkdir -p "$INSTALL_DIR"

TMP=$(mktemp)
echo "Downloading ${DOWNLOAD_URL} ..."
curl -sSfL "$DOWNLOAD_URL" -o "$TMP"
chmod +x "$TMP"

echo "Installing to ${INSTALL_DIR}/${BINARY} ..."
mv "$TMP" "${INSTALL_DIR}/${BINARY}"

if command -v "$BINARY" >/dev/null 2>&1; then
  echo "${BINARY} installed successfully."
  echo "Ensure ${INSTALL_DIR} is in your PATH."
else
  echo "Installed to ${INSTALL_DIR}/${BINARY} — add it to PATH if needed."
fi
