#!/usr/bin/env bash
# Upload a deskgit release to Cloudflare R2.
#
# Builds cross-platform binaries, verifies them, uploads to R2, and writes a
# latest.json manifest so `deskgit update` can find the new release.
#
# Prerequisites:
#   • aws CLI installed (brew install awscli)
#   • R2 credentials exported:
#       export AWS_ACCESS_KEY_ID=<your-r2-access-key-id>
#       export AWS_SECRET_ACCESS_KEY=<your-r2-secret-access-key>
#       export R2_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
#       export R2_BUCKET=<bucket-name>            # default: deskgit-releases
#       export R2_PUBLIC_URL=https://cdn.desktimers.com/deskgit
#
# Usage:
#   scripts/upload-deskgit.sh [vX.Y.Z]   # defaults to latest deskgit-v* tag
#
# The script is idempotent: re-running for the same version overwrites the
# objects in R2 and regenerates latest.json.
set -euo pipefail

cd "$(dirname "$0")/.."

# ── Config ───────────────────────────────────────────────────────────────────
BUCKET="${R2_BUCKET:-deskgit-releases}"
PUBLIC_URL="${R2_PUBLIC_URL:-https://cdn.desktimers.com}"
ENDPOINT="${R2_ENDPOINT:?R2_ENDPOINT not set — export it before running}"

# ── Version ──────────────────────────────────────────────────────────────────
arg="${1:-}"
if [[ "$arg" == v*.*.* || "$arg" == *.*.* ]]; then
  VERSION="${arg#v}"
else
  latest_tag="$(git tag --list 'deskgit-v*' --sort=-v:refname | head -1)"
  [ -n "$latest_tag" ] || { echo "error: no deskgit-v* tag found" >&2; exit 1; }
  VERSION="${latest_tag#deskgit-v}"
fi
TAG="deskgit-v$VERSION"
COMMIT="$(git rev-parse --short HEAD)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE -X main.buildSource=r2"

echo "Building deskgit $VERSION ($COMMIT) for all platforms..."

# ── Platforms ────────────────────────────────────────────────────────────────
PLATFORMS=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/arm64"
  "linux/amd64"
  "windows/amd64"
  "windows/arm64"
)

OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT

s3() {
  aws --endpoint-url "$ENDPOINT" s3 "$@"
}

sha256_file() {
  if command -v sha256sum &>/dev/null; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

MANIFEST_ASSETS=""  # will be built as JSON fragments

for platform in "${PLATFORMS[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  suffix=""
  [ "$GOOS" = "windows" ] && suffix=".exe"

  bin_name="deskgit-${GOOS}-${GOARCH}${suffix}"
  hook_name="dt-hook-${GOOS}-${GOARCH}${suffix}"
  bin_out="$OUT/$bin_name"
  hook_out="$OUT/$hook_name"

  echo "  compiling ${platform}..."
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "$bin_out" . 2>/dev/null
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "$hook_out" ./cmd/dt-hook 2>/dev/null

  bin_sha="$(sha256_file "$bin_out")"
  hook_sha="$(sha256_file "$hook_out")"

  r2_prefix="$VERSION"
  bin_url="$PUBLIC_URL/$r2_prefix/$bin_name"
  hook_url="$PUBLIC_URL/$r2_prefix/$hook_name"

  echo "  uploading $bin_name..."
  s3 cp "$bin_out" "s3://$BUCKET/$r2_prefix/$bin_name" \
    --content-type application/octet-stream

  echo "  uploading $hook_name..."
  s3 cp "$hook_out" "s3://$BUCKET/$r2_prefix/$hook_name" \
    --content-type application/octet-stream

  # Append JSON fragment for this platform
  fragment="$(cat <<JSON
    "${GOOS}/${GOARCH}": {
      "url": "$bin_url",
      "sha256": "$bin_sha",
      "hook_url": "$hook_url",
      "hook_sha256": "$hook_sha"
    }
JSON
)"
  if [ -z "$MANIFEST_ASSETS" ]; then
    MANIFEST_ASSETS="$fragment"
  else
    MANIFEST_ASSETS="$MANIFEST_ASSETS,
$fragment"
  fi
done

# ── Changelog ────────────────────────────────────────────────────────────────
CHANGELOG=""
CHANGELOG_FILE="changelogs/${VERSION}.md"
if [ -f "$CHANGELOG_FILE" ]; then
  CHANGELOG="$(cat "$CHANGELOG_FILE")"
  echo "Including changelog from $CHANGELOG_FILE"
else
  echo "Warning: no changelog file at $CHANGELOG_FILE — changelog will be empty"
fi

# JSON-encode the changelog using Python so all special chars are escaped.
if [ -n "$CHANGELOG" ]; then
  CHANGELOG_ESCAPED="$(python3 -c "import sys,json; print(json.dumps(sys.stdin.read())[1:-1])" <<< "$CHANGELOG")"
else
  CHANGELOG_ESCAPED=""
fi

# ── Write + upload latest.json ────────────────────────────────────────────────
MANIFEST_FILE="$OUT/latest.json"
cat > "$MANIFEST_FILE" <<JSON
{
  "version": "$VERSION",
  "published_at": "$DATE",
  "changelog": "$CHANGELOG_ESCAPED",
  "assets": {
$MANIFEST_ASSETS
  }
}
JSON

echo ""
echo "Manifest:"
cat "$MANIFEST_FILE"
echo ""

# The Windows installer is fetched by its address (irm …/install.ps1 | iex),
# so it lives beside latest.json and is replaced with every release.
echo "Uploading install.ps1..."
s3 cp scripts/install-deskgit.ps1 "s3://$BUCKET/install.ps1" \
  --content-type "text/plain; charset=utf-8" \
  --cache-control "no-cache, no-store"

echo "Uploading latest.json..."
s3 cp "$MANIFEST_FILE" "s3://$BUCKET/latest.json" \
  --content-type application/json \
  --cache-control "no-cache, no-store"

# ── Update versions.json (version history) ───────────────────────────────────
VERSIONS_FILE="$OUT/versions.json"
VERSIONS_TMP="$OUT/versions_existing.json"

# Try to fetch existing versions.json; start fresh if not found.
if aws --endpoint-url "$ENDPOINT" s3 cp "s3://$BUCKET/versions.json" "$VERSIONS_TMP" 2>/dev/null; then
  # Prepend new entry to existing versions array.
  NEW_ENTRY="{\"version\":\"$VERSION\",\"published_at\":\"$DATE\",\"changelog\":\"$CHANGELOG_ESCAPED\"}"
  python3 - "$VERSIONS_TMP" "$NEW_ENTRY" "$VERSIONS_FILE" <<'PYEOF'
import json, sys
existing_path, new_entry_json, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
with open(existing_path) as f:
    data = json.load(f)
versions = data.get("versions", [])
new_entry = json.loads(new_entry_json)
# Remove any existing entry for this version, then prepend.
versions = [v for v in versions if v.get("version") != new_entry["version"]]
versions.insert(0, new_entry)
with open(out_path, "w") as f:
    json.dump({"versions": versions}, f, indent=2)
PYEOF
else
  # No existing versions.json — create one with just this release.
  python3 - "$VERSIONS_FILE" "$VERSION" "$DATE" "$CHANGELOG_ESCAPED" <<'PYEOF'
import json, sys
out_path, version, published_at, changelog = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
entry = {"version": version, "published_at": published_at, "changelog": changelog}
with open(out_path, "w") as f:
    json.dump({"versions": [entry]}, f, indent=2)
PYEOF
fi

echo "Uploading versions.json..."
s3 cp "$VERSIONS_FILE" "s3://$BUCKET/versions.json" \
  --content-type application/json \
  --cache-control "no-cache, no-store"

echo ""
echo "Done. deskgit $VERSION is live at $PUBLIC_URL/latest.json"
echo "Users can update with:  deskgit update"
echo "Users can view history: deskgit changelog"
