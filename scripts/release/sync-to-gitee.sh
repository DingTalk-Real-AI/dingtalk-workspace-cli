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

missing=""
[ -z "${GITEE_TOKEN:-}" ] && missing="$missing GITEE_TOKEN"
[ -z "${GITEE_USER:-}" ]  && missing="$missing GITEE_USER"
[ -z "${GITEE_REPO:-}" ]  && missing="$missing GITEE_REPO"
if [ -n "$missing" ]; then
  echo "ℹ️  Gitee mirror sync skipped — missing:${missing}"
  echo "   Set these as repo secrets to auto-mirror releases to Gitee for China users."
  exit 0
fi

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
OWNER="${GITEE_REPO%%/*}"
NAME="${GITEE_REPO##*/}"
base="${GITEE_API}/repos/${OWNER}/${NAME}"

echo "📦 Mirroring release ${VERSION} → Gitee ${GITEE_REPO}"

# ── Keep the Gitee git tag aligned with the GitHub release tag ───────────────
# Creating a Gitee release with target_commitish=main can silently create a tag
# at the Gitee-localized main commit. The helper skips an already-aligned tag,
# pushes only a missing tag, and refuses to move a published tag.
VERSION="$VERSION" "$SCRIPT_DIR/sync-gitee-tag.sh"
target_commit="$(git rev-parse "${VERSION}^{commit}")"

api_get() {
  curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
    --max-time "$GITEE_CURL_MAX_TIME" "$@"
}

# ── Resolve or create the Gitee release for this tag ──────────────────────────
rel_json="$(api_get "${base}/releases/tags/${VERSION}?access_token=${GITEE_TOKEN}" 2>/dev/null || true)"
release_id="$(printf '%s' "$rel_json" | grep -o '"id":[ ]*[0-9]*' | head -1 | grep -o '[0-9]*' || true)"

if [ -z "$release_id" ]; then
  echo "   No Gitee release for ${VERSION} yet — creating it."
  rel_json="$(curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
    --max-time "$GITEE_CURL_MAX_TIME" -X POST "${base}/releases" \
    -F "access_token=${GITEE_TOKEN}" \
    -F "tag_name=${VERSION}" \
    -F "name=${VERSION}" \
    -F "body=Mirror of GitHub release ${VERSION} for China users." \
    -F "target_commitish=${target_commit}" 2>/dev/null || true)"
  release_id="$(printf '%s' "$rel_json" | grep -o '"id":[ ]*[0-9]*' | head -1 | grep -o '[0-9]*' || true)"
fi
[ -n "$release_id" ] || { echo "❌ Could not get/create Gitee release for ${VERSION}. Response: ${rel_json}" >&2; exit 1; }
echo "   Gitee release id = ${release_id}"

# ── Reconcile the complete, byte-verified release asset set ──────────────────
DIST_DIR="$DIST_DIR" \
GITEE_API="$GITEE_API" \
GITEE_TOKEN="$GITEE_TOKEN" \
GITEE_REPO="$GITEE_REPO" \
GITEE_CURL_CONNECT_TIMEOUT="$GITEE_CURL_CONNECT_TIMEOUT" \
GITEE_CURL_MAX_TIME="$GITEE_CURL_MAX_TIME" \
GITEE_RELEASE_ID="$release_id" \
  "$SCRIPT_DIR/reconcile-gitee-assets.sh"

echo "   China install:  DWS_GITEE_REPO=${GITEE_REPO} \\"
echo "     curl -fsSL https://gitee.com/${GITEE_REPO}/raw/main/scripts/install.sh | sh"
