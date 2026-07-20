#!/bin/sh
set -eu

TAG="${1:-}"
REPOSITORY="${GITHUB_REPOSITORY:-}"
RELEASE_ID="${DWS_GITHUB_RELEASE_ID:-}"

[ -n "$TAG" ] && [ -n "$REPOSITORY" ] || {
  printf 'usage: GITHUB_REPOSITORY=owner/repo [DWS_GITHUB_RELEASE_ID=id] verify-github-release-assets.sh <tag>\n' >&2
  exit 2
}
command -v gh >/dev/null 2>&1 || { printf 'gh is required\n' >&2; exit 1; }

if [ -z "$RELEASE_ID" ]; then
  RELEASE_ID="$(
    gh release view "$TAG" \
      --repo "$REPOSITORY" \
      --json databaseId,isDraft \
      --jq 'select(.isDraft == true) | .databaseId' 2>/dev/null || true
  )"
fi
if [ -n "$RELEASE_ID" ]; then
  printf '%s\n' "$RELEASE_ID" | grep -Eq '^[1-9][0-9]*$' || {
    printf 'invalid GitHub Release ID: %s\n' "$RELEASE_ID" >&2
    exit 2
  }
  release_endpoint="repos/$REPOSITORY/releases/$RELEASE_ID"
else
  release_endpoint="repos/$REPOSITORY/releases/tags/$TAG"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/dws-github-assets.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cat > "$tmp/expected" <<'EOF'
checksums.txt
dws-darwin-amd64.tar.gz
dws-darwin-arm64.tar.gz
dws-linux-amd64.tar.gz
dws-linux-arm64.tar.gz
dws-skills.zip
dws-windows-amd64.zip
dws-windows-arm64.zip
EOF
LC_ALL=C sort "$tmp/expected" -o "$tmp/expected"

resolved_tag="$(
  gh api -H 'Accept: application/vnd.github+json' \
    "$release_endpoint" \
    --jq '.tag_name'
)"
[ "$resolved_tag" = "$TAG" ] || {
  printf 'GitHub Release ID/tag mismatch: expected %s, got %s\n' "$TAG" "$resolved_tag" >&2
  exit 1
}
gh api -H 'Accept: application/vnd.github+json' \
  "$release_endpoint" \
  --jq '.assets[].name' | LC_ALL=C sort > "$tmp/actual"
if ! diff -u "$tmp/expected" "$tmp/actual"; then
  printf 'GitHub Release %s must contain exactly the supported assets\n' "$TAG" >&2
  exit 1
fi

printf 'GitHub Release asset set verified: %s\n' "$TAG"
