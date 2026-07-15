#!/usr/bin/env bash
# Publish locally-built release artifacts to the matching Gitee release.
#
# Intended to run inside Gitee Go after building artifacts in China. This avoids
# the unreliable GitHub Actions -> Gitee cross-border upload path.

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_REPO="${GITEE_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
GITEE_TOKEN="${GITEE_TOKEN:-${GITEE_ACCESS_TOKEN:-}}"
GITEE_CURL_CONNECT_TIMEOUT="${GITEE_CURL_CONNECT_TIMEOUT:-15}"
GITEE_CURL_MAX_TIME="${GITEE_CURL_MAX_TIME:-120}"

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

[ -n "$GITEE_TOKEN" ] || err "GITEE_TOKEN is required"
[ -d "$DIST_DIR" ] || err "dist dir not found: $DIST_DIR"

VERSION="${VERSION:-$(git describe --tags --exact-match 2>/dev/null || true)}"
[ -n "$VERSION" ] || err "VERSION is required or HEAD must be an exact tag"
case "$VERSION" in
  v*) ;;
  *) VERSION="v$VERSION" ;;
esac

OWNER="${GITEE_REPO%%/*}"
NAME="${GITEE_REPO##*/}"
base="${GITEE_API}/repos/${OWNER}/${NAME}"

api_get() {
  curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" "$@"
}

echo "📦 Publishing local artifacts for ${VERSION} to Gitee ${GITEE_REPO}"

target_commit="$(git rev-parse "${VERSION}^{commit}" 2>/dev/null || git rev-parse HEAD)"

rel_json="$(api_get "${base}/releases/tags/${VERSION}?access_token=${GITEE_TOKEN}" 2>/dev/null || true)"
release_id="$(printf '%s' "$rel_json" | grep -o '"id":[ ]*[0-9]*' | head -1 | grep -o '[0-9]*' || true)"

if [ -z "$release_id" ]; then
  echo "   No Gitee release for ${VERSION} yet — creating it."
  rel_json="$(curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" \
    -X POST "${base}/releases" \
    -F "access_token=${GITEE_TOKEN}" \
    -F "tag_name=${VERSION}" \
    -F "name=${VERSION}" \
    -F "body=Gitee-local build of ${VERSION} for China users." \
    -F "target_commitish=${target_commit}" 2>/dev/null || true)"
  release_id="$(printf '%s' "$rel_json" | grep -o '"id":[ ]*[0-9]*' | head -1 | grep -o '[0-9]*' || true)"
fi
[ -n "$release_id" ] || err "could not get/create Gitee release for ${VERSION}. Response: ${rel_json}"
echo "   Gitee release id = ${release_id}"

DIST_DIR="$DIST_DIR" \
GITEE_API="$GITEE_API" \
GITEE_TOKEN="$GITEE_TOKEN" \
GITEE_REPO="$GITEE_REPO" \
GITEE_CURL_CONNECT_TIMEOUT="$GITEE_CURL_CONNECT_TIMEOUT" \
GITEE_CURL_MAX_TIME="$GITEE_CURL_MAX_TIME" \
GITEE_RELEASE_ID="$release_id" \
  "$SCRIPT_DIR/reconcile-gitee-assets.sh"
