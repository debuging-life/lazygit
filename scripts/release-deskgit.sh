#!/usr/bin/env bash
# Cut a deskgit release: tag the current main and bump the Homebrew tap formula.
#
# There is no CI that publishes on a tag push (the release.yml in this repo is
# upstream lazygit's). The tap formula is a source-build formula, so a release
# is just: push a `deskgit-vX.Y.Z` tag, then point the formula at that tag's
# source tarball + its sha256. This script does both.
#
# Usage:
#   scripts/release-deskgit.sh [patch|minor|major]   # bump from the latest tag (default: patch)
#   scripts/release-deskgit.sh vX.Y.Z                # release an explicit version
#   scripts/release-deskgit.sh -y patch              # skip the confirmation prompt
#
# Assumes: clean tree, on `main`, HEAD already pushed to origin/main, and push
# access to both the fork and the tap (derived from origin's owner).
set -euo pipefail

cd "$(dirname "$0")/.."

REPO="lazygit"
TAP_REPO="homebrew-tap"
FORMULA="Formula/deskgit.rb"

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

# Owner comes from origin so this works regardless of the local SSH host alias:
#   git@<host>:OWNER/lazygit.git  |  https://github.com/OWNER/lazygit.git
origin_url="$(git remote get-url origin)"
owner="$(printf '%s' "$origin_url" | sed -E 's#.*[:/]([^/]+)/'"$REPO"'(\.git)?$#\1#')"
[ -n "$owner" ] && [ "$owner" != "$origin_url" ] || die "could not parse owner from origin: $origin_url"
tap_url="$(printf '%s' "$origin_url" | sed "s#/$REPO\(\.git\)\{0,1\}\$#/$TAP_REPO.git#")"

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
echo "Fork           : $owner/$REPO @ $(git rev-parse --short HEAD)"
echo "Tap            : $tap_url"
if [ "$assume_yes" != true ]; then
  read -r -p "Proceed? [y/N] " reply
  [ "$reply" = "y" ] || [ "$reply" = "Y" ] || die "aborted"
fi

# ── Tag the fork ─────────────────────────────────────────────────────────────
git tag "$tag" -a -m "Release $tag"
git push origin "refs/tags/$tag"
echo "Pushed tag $tag"

# ── Checksum the tag's source tarball (GitHub generates it on demand) ─────────
tarball="https://github.com/$owner/$REPO/archive/refs/tags/$tag.tar.gz"
sha=""
for attempt in 1 2 3 4 5; do
  sha="$(curl -fsSL "$tarball" | shasum -a 256 | awk '{print $1}')" && [ -n "$sha" ] && break
  echo "waiting for GitHub to generate the source tarball (attempt $attempt)…"
  sleep 3
done
[ -n "$sha" ] || die "could not fetch/checksum $tarball"
echo "sha256 $sha"

# ── Bump the tap formula ─────────────────────────────────────────────────────
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
git clone --quiet "$tap_url" "$work/tap"
formula="$work/tap/$FORMULA"
[ -f "$formula" ] || die "formula not found in tap: $FORMULA"

perl -pi -e "s|^(\s*url ).*|\${1}\"$tarball\"|"  "$formula"
perl -pi -e "s|^(\s*sha256 ).*|\${1}\"$sha\"|"   "$formula"
perl -pi -e "s|^(\s*version ).*|\${1}\"$version\"|" "$formula"

git -C "$work/tap" add "$FORMULA"
git -C "$work/tap" commit --quiet -m "deskgit $version"
git -C "$work/tap" push --quiet origin HEAD
echo "Bumped tap formula to $version"

echo
echo "Released deskgit $version. Users update with:"
echo "  brew update && brew upgrade deskgit    # or: brew reinstall deskgit"
