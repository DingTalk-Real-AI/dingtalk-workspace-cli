#!/usr/bin/env bash
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Mirror release artifacts to a Gitee release so China users can install without
# hitting GitHub. The repo code itself is kept in sync by Gitee's repo-mirror
# feature; this script handles what the mirror does NOT carry — the GitHub
# Release *attachments* (binaries, checksums, skills zip) — by uploading them to
# the matching Gitee release via the Gitee OpenAPI v5.
#
# Consumed by install.sh when DWS_GITEE_REPO is set (it resolves each asset's
# real download_url from the Gitee API, since Gitee attachment URLs carry an
# unstable numeric id).
#
# Required environment (CI secrets):
#   GITEE_TOKEN   Gitee private access token (scopes: projects)
#   GITEE_USER    Gitee username for git push authentication
#   GITEE_REPO    "owner/repo" on Gitee, e.g. DingTalk-Real-AI/dingtalk-workspace-cli
# Optional:
#   VERSION       release tag (default: git describe)
#   DIST_DIR      artifacts dir (default: ./dist)
#   GITEE_API     API base (default: https://gitee.com/api/v5)
#
# Gating: if GITEE_TOKEN / GITEE_USER / GITEE_REPO are unset, exit 0 with a
# notice so the step can live in release.yml without breaking forks.

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_CURL_CONNECT_TIMEOUT="${GITEE_CURL_CONNECT_TIMEOUT:-15}"
GITEE_CURL_MAX_TIME="${GITEE_CURL_MAX_TIME:-120}"
GITEE_SYNC_TIMEOUT_SECONDS="${GITEE_SYNC_TIMEOUT_SECONDS:-18840}"
GITEE_TAG_TIMEOUT_SECONDS="${GITEE_TAG_TIMEOUT_SECONDS:-300}"
GITEE_RELEASE_LOOKUP_MAX_TIME="${GITEE_RELEASE_LOOKUP_MAX_TIME:-60}"
GITEE_RELEASE_LOOKUP_RETRIES="${GITEE_RELEASE_LOOKUP_RETRIES:-2}"
GITEE_RELEASE_LOOKUP_RETRY_DELAY="${GITEE_RELEASE_LOOKUP_RETRY_DELAY:-2}"
GITEE_RELEASE_CREATE_MAX_TIME="${GITEE_RELEASE_CREATE_MAX_TIME:-60}"
GITEE_RECONCILE_TIMEOUT_SECONDS="${GITEE_RECONCILE_TIMEOUT_SECONDS:-18300}"
GITEE_CHILD_DEADLINE_RESERVE_SECONDS="${GITEE_CHILD_DEADLINE_RESERVE_SECONDS:-5}"

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_positive_integer() {
  local name="$1" value="$2"
  case "$value" in
    ''|*[!0-9]*) err "${name} must be a positive integer: ${value}" ;;
  esac
  [ "$value" -gt 0 ] || err "${name} must be greater than zero"
}

require_nonnegative_integer() {
  local name="$1" value="$2"
  case "$value" in
    ''|*[!0-9]*) err "${name} must be a non-negative integer: ${value}" ;;
  esac
}

for setting in \
  GITEE_CURL_CONNECT_TIMEOUT \
  GITEE_CURL_MAX_TIME \
  GITEE_SYNC_TIMEOUT_SECONDS \
  GITEE_TAG_TIMEOUT_SECONDS \
  GITEE_RELEASE_LOOKUP_MAX_TIME \
  GITEE_RELEASE_LOOKUP_RETRIES \
  GITEE_RELEASE_CREATE_MAX_TIME \
  GITEE_RECONCILE_TIMEOUT_SECONDS \
  GITEE_CHILD_DEADLINE_RESERVE_SECONDS; do
  require_positive_integer "$setting" "${!setting}"
done
require_nonnegative_integer \
  GITEE_RELEASE_LOOKUP_RETRY_DELAY \
  "$GITEE_RELEASE_LOOKUP_RETRY_DELAY"

missing=""
[ -z "${GITEE_TOKEN:-}" ] && missing="$missing GITEE_TOKEN"
[ -z "${GITEE_USER:-}" ]  && missing="$missing GITEE_USER"
[ -z "${GITEE_REPO:-}" ]  && missing="$missing GITEE_REPO"
if [ -n "$missing" ]; then
  if [ "${DWS_REQUIRE_GITEE:-0}" = "1" ]; then
    echo "error: Gitee mirror sync is enabled but credentials are missing:${missing}" >&2
    exit 1
  fi
  echo "ℹ️  Gitee mirror sync skipped — missing:${missing}"
  echo "   Set these as repo secrets to auto-mirror releases to Gitee for China users."
  exit 0
fi

now_seconds() {
  date +%s
}

sync_deadline=$(( $(now_seconds) + GITEE_SYNC_TIMEOUT_SECONDS ))

deadline_remaining() {
  local remaining=$(( sync_deadline - $(now_seconds) ))
  [ "$remaining" -gt 0 ] || return 1
  printf '%s\n' "$remaining"
}

bounded_max_time() {
  local configured="$1" remaining
  remaining="$(deadline_remaining)" || return 1
  if [ "$configured" -lt "$remaining" ]; then
    printf '%s\n' "$configured"
  else
    printf '%s\n' "$remaining"
  fi
}

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
OWNER="${GITEE_REPO%%/*}"
NAME="${GITEE_REPO##*/}"
base="${GITEE_API}/repos/${OWNER}/${NAME}"

echo "📦 Mirroring release ${VERSION} → Gitee ${GITEE_REPO}"

# ── Keep the Gitee git tag aligned with the GitHub release tag ───────────────
# Creating a Gitee release with target_commitish=main can silently create a tag
# at the Gitee-localized main commit. The helper skips an already-aligned tag,
# pushes only a missing tag, and refuses to move a published tag.
VERSION="$VERSION" \
GITEE_PARENT_DEADLINE_EPOCH="$sync_deadline" \
GITEE_TAG_TIMEOUT_SECONDS="$GITEE_TAG_TIMEOUT_SECONDS" \
  "$SCRIPT_DIR/sync-gitee-tag.sh"
deadline_remaining >/dev/null || err "overall Gitee sync deadline exhausted during tag synchronization"
target_commit="$(git rev-parse --verify "${VERSION}^{commit}")"

release_id_from_json() {
  python3 -c 'import json, sys
data = json.load(sys.stdin)
value = data.get("id") if isinstance(data, dict) else None
if isinstance(value, int) and not isinstance(value, bool) and value > 0:
    print(value)
elif isinstance(value, str) and value.isdigit() and int(value) > 0:
    print(value)' 2>/dev/null
}

# ── Resolve or create the Gitee release for this tag ──────────────────────────
# A transient lookup failure must not be mistaken for a missing release: doing
# so can race a duplicate create and hides the actual Gitee availability issue.
release_lookup_body="$(mktemp)"
trap 'rm -f "$release_lookup_body"' EXIT HUP INT TERM
lookup_max_time="$(bounded_max_time "$GITEE_RELEASE_LOOKUP_MAX_TIME")" \
  || err "overall Gitee sync deadline exhausted before release lookup"
if ! release_status="$(curl -sS --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
  --max-time "$lookup_max_time" \
  --retry "$GITEE_RELEASE_LOOKUP_RETRIES" \
  --retry-all-errors \
  --retry-delay "$GITEE_RELEASE_LOOKUP_RETRY_DELAY" \
  --retry-max-time "$lookup_max_time" \
  -o "$release_lookup_body" \
  -w '%{http_code}' \
  -H "Authorization: token ${GITEE_TOKEN}" \
  "${base}/releases/tags/${VERSION}")"; then
  deadline_remaining >/dev/null \
    || err "overall Gitee sync deadline exhausted during release lookup"
  err "could not query Gitee release ${VERSION} after bounded retries"
fi
rel_json="$(<"$release_lookup_body")"
case "$release_status" in
  200)
    release_id="$(printf '%s' "$rel_json" | release_id_from_json || true)"
    [ -n "$release_id" ] || err "Gitee release lookup returned HTTP 200 without a valid release id"
    ;;
  404)
    release_id=""
    ;;
  *)
    err "Gitee release lookup returned HTTP ${release_status} for ${VERSION}"
    ;;
esac

if [ "$release_status" = "404" ]; then
  deadline_remaining >/dev/null || err "overall Gitee sync deadline exhausted during release lookup"
  echo "   No Gitee release for ${VERSION} yet — creating it."
  create_max_time="$(bounded_max_time "$GITEE_RELEASE_CREATE_MAX_TIME")" \
    || err "overall Gitee sync deadline exhausted before release creation"
  rel_json="$(curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
    --max-time "$create_max_time" -X POST "${base}/releases" \
    -H "Authorization: token ${GITEE_TOKEN}" \
    -F "tag_name=${VERSION}" \
    -F "name=${VERSION}" \
    -F "body=Mirror of GitHub release ${VERSION} for China users." \
    -F "target_commitish=${target_commit}" 2>/dev/null || true)"
  release_id="$(printf '%s' "$rel_json" | release_id_from_json || true)"
fi
[ -n "$release_id" ] || { echo "❌ Could not get/create Gitee release for ${VERSION}. Response: ${rel_json}" >&2; exit 1; }
echo "   Gitee release id = ${release_id}"

# ── Reconcile the complete, byte-verified release asset set ──────────────────
remaining="$(deadline_remaining)" || err "overall Gitee sync deadline exhausted before asset reconciliation"
reconcile_timeout="$GITEE_RECONCILE_TIMEOUT_SECONDS"
max_child_timeout=$(( remaining - GITEE_CHILD_DEADLINE_RESERVE_SECONDS ))
[ "$max_child_timeout" -gt 0 ] || err "insufficient Gitee sync budget for asset reconciliation"
if [ "$reconcile_timeout" -gt "$max_child_timeout" ]; then
  reconcile_timeout="$max_child_timeout"
fi

DIST_DIR="$DIST_DIR" \
GITEE_API="$GITEE_API" \
GITEE_TOKEN="$GITEE_TOKEN" \
GITEE_REPO="$GITEE_REPO" \
GITEE_CURL_CONNECT_TIMEOUT="$GITEE_CURL_CONNECT_TIMEOUT" \
GITEE_CURL_MAX_TIME="$GITEE_CURL_MAX_TIME" \
GITEE_RELEASE_ID="$release_id" \
GITEE_OVERALL_TIMEOUT_SECONDS="$reconcile_timeout" \
  "$SCRIPT_DIR/reconcile-gitee-assets.sh"

deadline_remaining >/dev/null || err "overall Gitee sync deadline exhausted after asset reconciliation"

echo "   China install:  DWS_GITEE_REPO=${GITEE_REPO} \\"
echo "     curl -fsSL https://gitee.com/${GITEE_REPO}/raw/main/scripts/install.sh | sh"
