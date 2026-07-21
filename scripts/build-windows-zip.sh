#!/bin/sh
# Cross-compiles the Windows binaries and zips them together with LICENSE and
# README.md, so every published archive carries the MIT license text.
# Usage: scripts/build-windows-zip.sh [output-dir] [arch]
set -e

cd "$(dirname "$0")/.."
OUT_DIR="${1:-.}"
ARCH="${2:-amd64}"

VERSION="${DESKGIT_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE -X main.buildSource=build-windows-zip"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

GOOS=windows GOARCH="$ARCH" go build -ldflags "$LDFLAGS" -o "$STAGE/deskgit.exe" .
GOOS=windows GOARCH="$ARCH" go build -o "$STAGE/dt-hook.exe" ./cmd/dt-hook
cp LICENSE README.md "$STAGE/"

mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"
ZIP_PATH="$OUT_DIR/deskgit_${VERSION}_windows_${ARCH}.zip"
rm -f "$ZIP_PATH"
(cd "$STAGE" && zip -q "$ZIP_PATH" deskgit.exe dt-hook.exe LICENSE README.md)

echo "Built $ZIP_PATH ($VERSION)"
