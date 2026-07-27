#!/usr/bin/env bash
# Cut a deskgit release by tagging main. Pushing the `deskgit-vX.Y.Z` tag fires
# the "Release deskgit" workflow (.github/workflows/deskgit-release.yml), which
# bumps the Homebrew tap formula and creates the GitHub Release. This script
# only computes the next version and pushes the tag — the tap update is the
# pipeline's job, so the two never race to push the same formula.
#
# Usage:
#   scripts/release-deskgit.sh [patch|minor|major]   # bump from the latest tag (default: patch)
#   scripts/release-deskgit.sh vX.Y.Z                # release an explicit version
#   scripts/release-deskgit.sh -y patch              # skip the confirmation prompt
#
# Assumes: clean tree, on `main`, HEAD already pushed to origin/main.
set -euo pipefail

cd "$(dirname "$0")/.."

assume_yes=false
if [ "${1:-}" = "-y" ]; then
  assume_yes=true
  shift
fi
arg="${1:-patch}"

die() { echo "error: $*" >&2; exit 1; }

# ── Preflight ────────────────────────────────────────────────────────────────
[ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || die "not on main"
[ -z "$(git status --porcelain)" ] || die "working tree is dirty — commit or stash first"

git fetch --quiet origin main --tags
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || \
  die "HEAD is not in sync with origin/main — push (or pull) first"

# ── Determine the new version ────────────────────────────────────────────────
latest="$(git tag --list 'deskgit-v*' --sort=-v:refname | head -1)"
[ -n "$latest" ] || latest="deskgit-v0.0.0"
cur="${latest#deskgit-v}"
IFS='.' read -r major minor patch <<<"$cur"

case "$arg" in
  patch) patch=$((patch + 1)) ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  major) major=$((major + 1)); minor=0; patch=0 ;;
  v*.*.*|*.*.*)
    version="${arg#v}"
    [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "invalid version: $arg (expected X.Y.Z)"
    ;;
  *) die "unknown bump '$arg' (use patch|minor|major or an explicit vX.Y.Z)" ;;
esac
version="${version:-$major.$minor.$patch}"
tag="deskgit-v$version"

git rev-parse -q --verify "refs/tags/$tag" >/dev/null && die "tag $tag already exists"

echo "Latest release : ${latest#deskgit-v}"
echo "New release    : $version  (tag $tag)"
echo "Commit         : $(git rev-parse --short HEAD)"
if [ "$assume_yes" != true ]; then
  read -r -p "Tag and push $tag? [y/N] " reply
  [ "$reply" = "y" ] || [ "$reply" = "Y" ] || die "aborted"
fi

git tag "$tag" -a -m "Release $tag"
git push origin "refs/tags/$tag"

echo
echo "Pushed $tag. The 'Release deskgit' workflow will bump the tap formula and"
echo "publish the release. Users update with:"
echo "  brew update && brew upgrade deskgit    # or: brew reinstall deskgit"
