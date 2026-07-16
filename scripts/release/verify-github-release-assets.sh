#!/bin/sh
set -eu

TAG="${1:-}"
REPOSITORY="${GITHUB_REPOSITORY:-}"

[ -n "$TAG" ] && [ -n "$REPOSITORY" ] || {
  printf 'usage: GITHUB_REPOSITORY=owner/repo verify-github-release-assets.sh <tag>\n' >&2
  exit 2
}
command -v gh >/dev/null 2>&1 || { printf 'gh is required\n' >&2; exit 1; }

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

gh api -H 'Accept: application/vnd.github+json' \
  "repos/$REPOSITORY/releases/tags/$TAG" \
  --jq '.assets[].name' | LC_ALL=C sort > "$tmp/actual"
if ! diff -u "$tmp/expected" "$tmp/actual"; then
  printf 'GitHub Release %s must contain exactly the supported assets\n' "$TAG" >&2
  exit 1
fi

printf 'GitHub Release asset set verified: %s\n' "$TAG"
