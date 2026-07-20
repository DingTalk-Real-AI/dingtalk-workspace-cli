#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
TAG="${1:-}"
DEST_DIR="${2:-}"
REPOSITORY="${GITHUB_REPOSITORY:-}"
RELEASE_ID="${DWS_GITHUB_RELEASE_ID:-}"

[ -n "$TAG" ] && [ -n "$DEST_DIR" ] && [ -n "$REPOSITORY" ] || {
  printf 'usage: GITHUB_REPOSITORY=owner/repo [DWS_GITHUB_RELEASE_ID=id] download-github-release-assets.sh <tag> <destination>\n' >&2
  exit 2
}
[ -n "$RELEASE_ID" ] || {
  printf 'DWS_GITHUB_RELEASE_ID is required for an exact release download\n' >&2
  exit 2
}
printf '%s\n' "$RELEASE_ID" | grep -Eq '^[1-9][0-9]*$' || {
  printf 'invalid GitHub Release ID: %s\n' "$RELEASE_ID" >&2
  exit 2
}
command -v gh >/dev/null 2>&1 || { printf 'gh is required\n' >&2; exit 1; }

DWS_GITHUB_RELEASE_ID="$RELEASE_ID" \
  "$SCRIPT_DIR/verify-github-release-assets.sh" "$TAG"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/dws-github-release-download.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
tab="$(printf '\t')"
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
  "repos/$REPOSITORY/releases/$RELEASE_ID" \
  --jq '.assets[] | [.id, .name] | @tsv' > "$tmp/assets.tsv"
cut -f 2 "$tmp/assets.tsv" | LC_ALL=C sort > "$tmp/actual"
if ! diff -u "$tmp/expected" "$tmp/actual"; then
  printf 'GitHub Release %s must contain exactly the supported assets\n' "$TAG" >&2
  exit 1
fi

while IFS="$tab" read -r asset_id asset_name; do
    printf '%s\n' "$asset_id" | grep -Eq '^[1-9][0-9]*$' || {
      printf 'invalid GitHub Release asset ID: %s\n' "$asset_id" >&2
      exit 1
    }
    case "$asset_name" in
      checksums.txt | \
        dws-darwin-amd64.tar.gz | dws-darwin-arm64.tar.gz | \
        dws-linux-amd64.tar.gz | dws-linux-arm64.tar.gz | \
        dws-windows-amd64.zip | dws-windows-arm64.zip | \
        dws-skills.zip) ;;
      *)
        printf 'unsupported GitHub Release asset name: %s\n' "$asset_name" >&2
        exit 1
        ;;
    esac
    gh api -H 'Accept: application/octet-stream' \
      "repos/$REPOSITORY/releases/assets/$asset_id" > "$tmp/$asset_name"
done < "$tmp/assets.tsv"

mkdir -p "$DEST_DIR"
for asset_name in \
  checksums.txt \
  dws-darwin-amd64.tar.gz \
  dws-darwin-arm64.tar.gz \
  dws-linux-amd64.tar.gz \
  dws-linux-arm64.tar.gz \
  dws-windows-amd64.zip \
  dws-windows-arm64.zip \
  dws-skills.zip; do
  [ -f "$tmp/$asset_name" ] || {
    printf 'GitHub Release asset was not downloaded: %s\n' "$asset_name" >&2
    exit 1
  }
  mv -f "$tmp/$asset_name" "$DEST_DIR/$asset_name"
done

printf 'GitHub Release assets downloaded and verified: %s\n' "$TAG"
