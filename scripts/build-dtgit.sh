#!/bin/sh
# Builds the dtgit + dt-hook binaries with version info baked in.
# Usage: scripts/build-dtgit.sh [output-dir]
set -e

cd "$(dirname "$0")/.."
OUT_DIR="${1:-.}"

VERSION="${DTGIT_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE -X main.buildSource=build-dtgit"

go build -ldflags "$LDFLAGS" -o "$OUT_DIR/dtgit" .
go build -o "$OUT_DIR/dt-hook" ./cmd/dt-hook

echo "Built $OUT_DIR/dtgit and $OUT_DIR/dt-hook ($VERSION)"
