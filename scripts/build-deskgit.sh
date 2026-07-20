#!/bin/sh
# Builds the deskgit + dt-hook binaries with version info baked in.
# Usage: scripts/build-deskgit.sh [output-dir]
set -e

cd "$(dirname "$0")/.."
OUT_DIR="${1:-.}"

VERSION="${DESKGIT_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE -X main.buildSource=build-deskgit"

go build -ldflags "$LDFLAGS" -o "$OUT_DIR/deskgit" .
go build -o "$OUT_DIR/dt-hook" ./cmd/dt-hook

echo "Built $OUT_DIR/deskgit and $OUT_DIR/dt-hook ($VERSION)"
