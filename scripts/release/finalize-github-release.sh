#!/bin/sh
set -eu

# Replace GoReleaser's original assets with post-processed artifacts, verify
# that GitHub serves exactly those bytes, and only then make the Draft public.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
DIST_DIR="${DWS_PACKAGE_DIST_DIR:-$ROOT/dist}"
TAG="${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
REPOSITORY="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
PUBLISH_RELEASE="${DWS_PUBLISH_RELEASE:-true}"
DIGEST_ATTEMPTS="${DWS_RELEASE_DIGEST_ATTEMPTS:-5}"
DIGEST_RETRY_DELAY="${DWS_RELEASE_DIGEST_RETRY_DELAY:-2}"

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

sha256_file() {
  target="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$target" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$target" | awk '{print $1}'
}

case "$PUBLISH_RELEASE" in
  1|true|yes) publish_release=1 ;;
  0|false|no) publish_release=0 ;;
  *) err "invalid DWS_PUBLISH_RELEASE value: $PUBLISH_RELEASE" ;;
esac

case "$DIGEST_ATTEMPTS" in
  ''|*[!0-9]*) err "DWS_RELEASE_DIGEST_ATTEMPTS must be a positive integer" ;;
  0) err "DWS_RELEASE_DIGEST_ATTEMPTS must be greater than zero" ;;
esac

command -v gh >/dev/null 2>&1 || err "gh is required"

set -- \
  "$DIST_DIR/dws-darwin-amd64.tar.gz" \
  "$DIST_DIR/dws-darwin-arm64.tar.gz" \
  "$DIST_DIR/checksums.txt" \
  "$DIST_DIR/dws-skills.zip"

for asset in "$@"; do
  [ -f "$asset" ] || err "finalized release asset missing: $asset"
done

gh release upload "$TAG" "$@" \
  --repo "$REPOSITORY" \
  --clobber

# Fail before publication if GitHub still serves a pre-signing archive. Asset
# digests can take a moment to settle after replacement, so retry.
for asset in "$@"; do
  name="$(basename "$asset")"
  local_digest="sha256:$(sha256_file "$asset")"
  attempt=1
  while [ "$attempt" -le "$DIGEST_ATTEMPTS" ]; do
    remote_digest="$(
      gh release view "$TAG" --repo "$REPOSITORY" --json assets \
        --jq ".assets[] | select(.name == \"$name\") | .digest"
    )"
    if [ "$remote_digest" = "$local_digest" ]; then
      break
    fi
    if [ "$attempt" -eq "$DIGEST_ATTEMPTS" ]; then
      printf 'release asset digest mismatch for %s\n' "$name" >&2
      printf '  local:  %s\n' "$local_digest" >&2
      printf '  remote: %s\n' "${remote_digest:-missing}" >&2
      exit 1
    fi
    attempt=$((attempt + 1))
    sleep "$DIGEST_RETRY_DELAY"
  done
done

if [ "$publish_release" -eq 1 ]; then
  gh release edit "$TAG" --repo "$REPOSITORY" --draft=false
else
  printf 'finalized assets verified; keeping release %s as Draft\n' "$TAG"
fi
