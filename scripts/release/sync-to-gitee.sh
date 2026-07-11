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

set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_CURL_CONNECT_TIMEOUT="${GITEE_CURL_CONNECT_TIMEOUT:-15}"
GITEE_CURL_MAX_TIME="${GITEE_CURL_MAX_TIME:-120}"
GITEE_UPLOAD_MAX_TIME="${GITEE_UPLOAD_MAX_TIME:-300}"
GITEE_UPLOAD_RETRIES="${GITEE_UPLOAD_RETRIES:-3}"
GITEE_UPLOAD_RETRY_DELAY="${GITEE_UPLOAD_RETRY_DELAY:-10}"

missing=""
[ -z "${GITEE_TOKEN:-}" ] && missing="$missing GITEE_TOKEN"
[ -z "${GITEE_USER:-}" ]  && missing="$missing GITEE_USER"
[ -z "${GITEE_REPO:-}" ]  && missing="$missing GITEE_REPO"
if [ -n "$missing" ]; then
  if [ "${DWS_REQUIRE_GITEE:-0}" = "1" ]; then
    echo "❌ Gitee mirror sync is enabled but credentials are missing:${missing}" >&2
    exit 1
  fi
  echo "ℹ️  Gitee mirror sync skipped — missing:${missing}"
  echo "   Set these as repo secrets to auto-mirror releases to Gitee for China users."
  exit 0
fi

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
DWS_PACKAGE_DIST_DIR="$DIST_DIR" "$SCRIPT_DIR/verify-release-artifacts.sh" "$VERSION"
OWNER="${GITEE_REPO%%/*}"
NAME="${GITEE_REPO##*/}"
base="${GITEE_API}/repos/${OWNER}/${NAME}"
git_remote="https://${GITEE_USER}:${GITEE_TOKEN}@gitee.com/${GITEE_REPO}.git"
public_git_remote="https://gitee.com/${GITEE_REPO}.git"

echo "📦 Mirroring release ${VERSION} → Gitee ${GITEE_REPO}"

# ── Keep the Gitee git tag aligned with the GitHub release tag ───────────────
# Creating a Gitee release with target_commitish=main can silently create a tag
# at the Gitee-localized main commit. Always push the exact GitHub tag first,
# then verify the peeled commit before touching release attachments.
git fetch --force --tags origin "refs/tags/${VERSION}:refs/tags/${VERSION}" >/dev/null 2>&1 || true
target_commit="$(git rev-parse "${VERSION}^{commit}" 2>/dev/null || true)"
[ -n "$target_commit" ] || { echo "❌ Could not resolve local release tag ${VERSION}; fetch tags before syncing." >&2; exit 1; }

gitee_tag_commit() {
  git ls-remote "$public_git_remote" "refs/tags/${VERSION}*" 2>/dev/null \
    | awk -v tag="refs/tags/${VERSION}" '
        $2 == tag "^{}" { peeled=$1 }
        $2 == tag { direct=$1 }
        END {
          if (peeled != "") print peeled;
          else if (direct != "") print direct;
        }'
}

gitee_commit="$(gitee_tag_commit || true)"
if [ -n "$gitee_commit" ] && [ "$gitee_commit" != "$target_commit" ]; then
  echo "❌ Existing Gitee tag ${VERSION} points to ${gitee_commit}, expected ${target_commit}; refusing to move it." >&2
  exit 1
fi
if [ -z "$gitee_commit" ]; then
  echo "   Creating Gitee tag ${VERSION} -> ${target_commit}"
  git push "$git_remote" "refs/tags/${VERSION}:refs/tags/${VERSION}" >/dev/null
else
  echo "   Gitee tag ${VERSION} already points to ${target_commit}."
fi

for _ in 1 2 3 4 5 6 7 8 9 10 11 12; do
  gitee_commit="$(gitee_tag_commit || true)"
  [ "$gitee_commit" = "$target_commit" ] && break
  sleep 5
done
[ "$gitee_commit" = "$target_commit" ] || {
  echo "❌ Gitee tag ${VERSION} is not aligned after push: got ${gitee_commit:-<missing>}, want ${target_commit}" >&2
  exit 1
}
echo "   Gitee tag ${VERSION} is aligned."

# ── Resolve or create the Gitee release for this tag ──────────────────────────
rel_json="$(curl -fsSL "${base}/releases/tags/${VERSION}?access_token=${GITEE_TOKEN}" 2>/dev/null || true)"
release_id="$(printf '%s' "$rel_json" | grep -o '"id":[ ]*[0-9]*' | head -1 | grep -o '[0-9]*' || true)"

if [ -z "$release_id" ]; then
  echo "   No Gitee release for ${VERSION} yet — creating it."
  rel_json="$(curl -fsSL -X POST "${base}/releases" \
    -F "access_token=${GITEE_TOKEN}" \
    -F "tag_name=${VERSION}" \
    -F "name=${VERSION}" \
    -F "body=Mirror of GitHub release ${VERSION} for China users." \
    -F "target_commitish=${target_commit}" 2>/dev/null || true)"
  release_id="$(printf '%s' "$rel_json" | grep -o '"id":[ ]*[0-9]*' | head -1 | grep -o '[0-9]*' || true)"
fi
[ -n "$release_id" ] || { echo "❌ Could not get/create Gitee release for ${VERSION}. Response: ${rel_json}" >&2; exit 1; }
echo "   Gitee release id = ${release_id}"

# ── Mirror each artifact, verifying content so re-runs self-heal ─────────────
# Pull the current attachment list (name + attach id + download url) so we can,
# per file:
#   • skip it when it is already on Gitee, unique, AND byte-identical,
#   • REPLACE it when present but stale (different bytes) — e.g. the darwin
#     binaries are re-signed and so differ between GitHub and a prior mirror
#     run, which breaks install.sh's checksums.txt verification on macOS,
#   • DEDUP it when the same name has >1 attachment — a prior run that failed to
#     delete (see below) left the old copy *and* an extra upload; Gitee then
#     serves the OLDER one by name, so the stale binary wins. We delete every
#     copy and upload one fresh.
#   • upload it when missing.
# This brings the Gitee release into byte-for-byte agreement with $DIST_DIR,
# which the caller fills from the GitHub release whose checksums.txt is what
# install.sh verifies against.
#
# We list attachments via the dedicated /attach_files endpoint, NOT the release
# detail (/releases/{id}) endpoint: the latter's "assets" array omits the attach
# id, so DELETE /attach_files/{id} was previously called with an empty id and
# silently no-op'd — leaving stale + duplicate darwin binaries on Gitee.
load_assets_map() {
  curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" \
    "${base}/releases/${release_id}/attach_files?access_token=${GITEE_TOKEN}" \
    | python3 -c 'import json,sys
try:
    data=json.load(sys.stdin)
    rows=data if isinstance(data,list) else data.get("attach_files",[])
    for a in rows:
        n=a.get("name",""); i=a.get("id",""); u=a.get("browser_download_url","")
        if n and i!="":
            print("%s\t%s\t%s" % (n, i, u))
except Exception:
    sys.exit(1)'
}

assets_map="$(load_assets_map)" || {
  echo "❌ Could not list Gitee release attachments; refusing a blind upload." >&2
  exit 1
}

sha256_of() {  # sha256 of a file ($1) or, with no arg, of stdin
  if command -v sha256sum >/dev/null 2>&1; then sha256sum ${1:+"$1"} | awk '{print $1}';
  else shasum -a 256 ${1:+"$1"} | awk '{print $1}'; fi
}

gitee_attach() {  # upload file $1; success when the response carries a download url
  file="$1"
  fn="$(basename "$file")"
  attempt=1
  while [ "$attempt" -le "$GITEE_UPLOAD_RETRIES" ]; do
    response="$(curl -fsS --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_UPLOAD_MAX_TIME" \
      --retry 2 --retry-delay 5 --retry-all-errors \
      -X POST "${base}/releases/${release_id}/attach_files" \
      -F "access_token=${GITEE_TOKEN}" -F "file=@${file}" 2>&1 || true)"
    if printf '%s' "$response" | grep -q '"browser_download_url"'; then
      return 0
    fi
    echo "   ⚠ upload attempt ${attempt}/${GITEE_UPLOAD_RETRIES} failed for ${fn}: $(printf '%s' "$response" | head -c 240)" >&2
    attempt=$((attempt + 1))
    [ "$attempt" -le "$GITEE_UPLOAD_RETRIES" ] && sleep "$GITEE_UPLOAD_RETRY_DELAY"
  done
  return 1
}

gitee_delete() {  # delete attachment by id $1
  curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" \
    -X DELETE "${base}/releases/${release_id}/attach_files/${1}?access_token=${GITEE_TOKEN}" \
    >/dev/null
}

uploaded=0
replaced=0
skipped=0
failed=0
for f in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/checksums.txt; do
  [ -f "$f" ] || continue
  fn="$(basename "$f")"
  local_sha="$(sha256_of "$f")"
  # Every attach id currently carrying this name (may be >1 from a botched run).
  ids="$(printf '%s\n' "$assets_map" | awk -F'\t' -v n="$fn" '$1==n {print $2}')"
  aurl="$(printf '%s\n' "$assets_map" | awk -F'\t' -v n="$fn" '$1==n {print $3; exit}')"
  count="$(printf '%s' "$ids" | grep -c . || true)"

  if [ "$count" -eq 0 ]; then
    echo "   ⬆ ${fn} (new)"
    if gitee_attach "$f"; then
      uploaded=$((uploaded + 1))
    else
      echo "   ❌ upload failed for ${fn}" >&2
      failed=$((failed + 1))
    fi
    continue
  fi

  if [ "$count" -eq 1 ]; then
    gitee_sha="$(curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" "$aurl" 2>/dev/null | sha256_of || true)"
    if [ "$gitee_sha" = "$local_sha" ]; then
      echo "   ✓ ${fn} already correct on Gitee — skip"
      skipped=$((skipped + 1))
      continue
    fi
    echo "   ↻ ${fn} differs on Gitee (stale) — deleting + re-uploading"
  else
    echo "   ↻ ${fn} has ${count} copies on Gitee (dup) — deleting all + re-uploading one"
  fi

  # Delete every copy, then upload exactly one fresh, correct file.
  delete_failed=0
  for aid in $ids; do
    if ! gitee_delete "$aid"; then
      echo "   ❌ failed to delete stale Gitee attachment ${aid} for ${fn}" >&2
      delete_failed=1
    fi
  done
  if [ "$delete_failed" -ne 0 ]; then
    failed=$((failed + 1))
    continue
  fi
  if gitee_attach "$f"; then
    replaced=$((replaced + 1))
  else
    echo "   ❌ re-upload failed for ${fn}" >&2
    failed=$((failed + 1))
  fi
done

if [ "$uploaded" -eq 0 ] && [ "$replaced" -eq 0 ] && [ "$skipped" -eq 0 ]; then
  echo "❌ No artifacts found to mirror. Did the build (goreleaser) run / were assets downloaded into ${DIST_DIR}?" >&2
  exit 1
fi

# Gitee may accept an upload before the attachment list is consistent. Re-read
# the release and require every supported asset to be unique and byte-identical.
verified=0
for verify_attempt in 1 2 3 4 5; do
  final_map="$(load_assets_map || true)"
  verify_failed=0
  for f in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/checksums.txt; do
    fn="$(basename "$f")"
    rows="$(printf '%s\n' "$final_map" | awk -F'\t' -v n="$fn" '$1==n')"
    count="$(printf '%s\n' "$rows" | grep -c . || true)"
    if [ "$count" -ne 1 ]; then
      verify_failed=1
      continue
    fi
    aurl="$(printf '%s\n' "$rows" | awk -F'\t' 'NR==1 {print $3}')"
    remote_sha="$(curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" "$aurl" 2>/dev/null | sha256_of || true)"
    [ "$remote_sha" = "$(sha256_of "$f")" ] || verify_failed=1
  done
  if [ "$verify_failed" -eq 0 ] && [ "$failed" -eq 0 ]; then
    verified=1
    break
  fi
  [ "$verify_attempt" -lt 5 ] && sleep 5
done
[ "$verified" -eq 1 ] || {
  echo "❌ Gitee release ${VERSION} does not contain one byte-identical copy of every release asset." >&2
  exit 1
}
echo "✅ Gitee release ${VERSION}: uploaded ${uploaded}, replaced ${replaced}, skipped ${skipped} (already correct)."
echo "   China install:  DWS_GITEE_REPO=${GITEE_REPO} \\"
echo "     curl -fsSL https://gitee.com/${GITEE_REPO}/raw/main/scripts/install.sh | sh"
