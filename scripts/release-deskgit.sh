#!/usr/bin/env bash
# One-command deskgit release.
#
# Usage:
#   scripts/release-deskgit.sh <version>    e.g.  scripts/release-deskgit.sh 0.5.8
#
# What it does:
#   1. Tags HEAD as deskgit-v<version>
#   2. Pushes the tag → GitHub Actions updates the Homebrew tap formula
#   3. Uploads cross-platform binaries + latest.json + versions.json to R2
#      (requires R2 credentials: source .env.r2 first, or export them manually)
#
# Prerequisites:
#   • R2 credentials exported (see scripts/upload-deskgit.sh header)
#   • gh CLI authenticated
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "Usage: scripts/release-deskgit.sh <version>  (e.g. 0.5.8)" >&2
  exit 1
fi
VERSION="${VERSION#v}"  # strip leading v if supplied
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

# 1. Tag
git tag "$TAG"
echo "    tagged $TAG"

# 2. Push tag (DT_STRICT=0 because release commits have no task code)
echo "    pushing $TAG to GitHub (triggers tap update)…"
DT_STRICT=0 git push origin "$TAG"

# 3. Upload to R2
echo "    uploading binaries to R2…"
scripts/upload-deskgit.sh "$VERSION"

echo ""
echo "Done! deskgit $VERSION is released."
echo "  Homebrew tap:  brew upgrade deskgit"
echo "  OTA update:    deskgit update"
