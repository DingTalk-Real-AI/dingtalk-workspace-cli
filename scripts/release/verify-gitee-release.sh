#!/bin/sh
# Verify that a Gitee release exposes every standard DWS release asset.

set -eu

VERSION="${VERSION:-}"
GITEE_REPO="${GITEE_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
DIST_DIR="${DIST_DIR:-dist}"
GITEE_VERIFY_RETRIES="${GITEE_VERIFY_RETRIES:-6}"
GITEE_VERIFY_RETRY_DELAY="${GITEE_VERIFY_RETRY_DELAY:-5}"
REQUIRED_ASSETS="${GITEE_REQUIRED_ASSETS:-checksums.txt dws-darwin-amd64.tar.gz dws-darwin-arm64.tar.gz dws-linux-amd64.tar.gz dws-linux-arm64.tar.gz dws-skills.zip dws-windows-amd64.zip dws-windows-arm64.zip}"

[ -n "$VERSION" ] || {
  echo "error: VERSION is required" >&2
  exit 2
}

for asset in $REQUIRED_ASSETS; do
  [ -f "$DIST_DIR/$asset" ] || {
    echo "error: local release asset is missing: $DIST_DIR/$asset" >&2
    exit 1
  }
done

endpoint="${GITEE_API}/repos/${GITEE_REPO}/releases/tags/${VERSION}"
if [ -n "${GITEE_TOKEN:-}" ]; then
  endpoint="${endpoint}?access_token=${GITEE_TOKEN}"
fi

attempt=1
missing=""
while [ "$attempt" -le "$GITEE_VERIFY_RETRIES" ]; do
  release_json="$(curl -fsSL "$endpoint" 2>/dev/null || true)"
  compact="$(printf '%s' "$release_json" | tr -d '[:space:]')"
  missing=""
  for asset in $REQUIRED_ASSETS; do
    count="$(printf '%s' "$compact" | grep -oF "\"name\":\"${asset}\"" | wc -l | tr -d ' ')"
    if [ "$count" -ne 1 ]; then
      missing="${missing} ${asset}(count=${count})"
    fi
  done
  if [ -n "$release_json" ] && [ "$release_json" != "null" ] && [ -z "$missing" ]; then
    echo "Gitee release ${VERSION} is complete (${GITEE_REPO})."
    exit 0
  fi
  attempt=$((attempt + 1))
  [ "$attempt" -le "$GITEE_VERIFY_RETRIES" ] && sleep "$GITEE_VERIFY_RETRY_DELAY"
done

echo "error: Gitee release ${VERSION} is incomplete:${missing:- release not found}" >&2
exit 1
