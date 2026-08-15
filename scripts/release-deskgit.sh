#!/usr/bin/env bash
# One-command deskgit release.
#
# Usage:
#   scripts/release-deskgit.sh <version>           # OTA-only (default)
#   scripts/release-deskgit.sh <version> --tap     # also push tag → update Homebrew formula
#
# The Homebrew formula is pinned at 0.5.7 as a permanent OTA-capable
# baseline. Use --tap only when you deliberately want to bump the formula
# (e.g. a new baseline with a breaking change). Routine releases go to R2
# only — deskgit's first-launch check and `deskgit update` handle delivery.
#
# Prerequisites:
#   • R2 credentials exported (source .env.r2 first)
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-}"
PUSH_TAP=0
for arg in "$@"; do
  [[ "$arg" == "--tap" ]] && PUSH_TAP=1
done

if [[ -z "$VERSION" || "$VERSION" == --* ]]; then
  echo "Usage: scripts/release-deskgit.sh <version> [--tap]" >&2
  exit 1
fi
VERSION="${VERSION#v}"
TAG="deskgit-v$VERSION"

# Sanity checks
if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is not clean — commit or stash changes first" >&2
  exit 1
fi
if git tag --list "$TAG" | grep -q .; then
  echo "error: tag $TAG already exists locally" >&2
  exit 1
fi

echo "==> Releasing deskgit $VERSION"

# 1. Tag locally
git tag "$TAG"
echo "    tagged $TAG"

# 2. Optionally push tag to GitHub (updates Homebrew tap via CI)
if [[ "$PUSH_TAP" == 1 ]]; then
  echo "    pushing $TAG to GitHub (triggers Homebrew tap update)…"
  DT_STRICT=0 git push origin "$TAG"
else
  echo "    skipping GitHub push — brew formula stays at baseline (use --tap to bump it)"
fi

# 3. Upload to R2 (OTA delivery)
echo "    uploading binaries to R2…"
scripts/upload-deskgit.sh "$VERSION"

echo ""
echo "Done! deskgit $VERSION is live via OTA."
echo "  Users update with:  deskgit update"
if [[ "$PUSH_TAP" == 1 ]]; then
  echo "  Homebrew updated:   brew upgrade deskgit"
fi
