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

DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"

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

echo "   Syncing Gitee tag ${VERSION} -> ${target_commit}"
git push --force "$git_remote" "refs/tags/${VERSION}:refs/tags/${VERSION}" >/dev/null

gitee_commit=""
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

# ── Mirror each artifact, using file size to skip unchanged assets ────────────
# Pull the current attachment list (name + attach id + download url + size) so we
# can, per file:
#   • skip it when it is already on Gitee, unique, AND same byte size,
#   • REPLACE it when present but different size (stale),
#   • DEDUP it when the same name has >1 attachment — a prior run that failed to
#     delete (see below) left the old copy *and* an extra upload; Gitee then
#     serves the OLDER one by name, so the stale binary wins. We delete every
#     copy and upload one fresh.
#   • upload it when missing.
# Size comparison avoids downloading every binary from Gitee just to SHA256 it,
# which took ~40 min on GitHub-hosted runners (slow Gitee download from US).
#
# We list attachments via the dedicated /attach_files endpoint, NOT the release
# detail (/releases/{id}) endpoint: the latter's "assets" array omits the attach
# id, so DELETE /attach_files/{id} was previously called with an empty id and
# silently no-op'd — leaving stale + duplicate darwin binaries on Gitee.
assets_map="$(curl -fsSL "${base}/releases/${release_id}/attach_files?access_token=${GITEE_TOKEN}" 2>/dev/null \
  | python3 -c 'import json,sys
try:
    data=json.load(sys.stdin)
    rows=data if isinstance(data,list) else data.get("attach_files",[])
    for a in rows:
        n=a.get("name",""); i=a.get("id",""); u=a.get("browser_download_url",""); s=a.get("size","")
        if n and i!="":
            print("%s\t%s\t%s\t%s" % (n, i, u, s))
except Exception:
    pass' 2>/dev/null || true)"

gitee_attach() {  # upload file $1; success when the response carries a download url
  printf '%s' "$(curl -fsSL -X POST "${base}/releases/${release_id}/attach_files" \
    -F "access_token=${GITEE_TOKEN}" -F "file=@${1}" 2>/dev/null || true)" \
    | grep -q '"browser_download_url"'
}

gitee_delete() {  # delete attachment by id $1
  curl -fsSL -X DELETE "${base}/releases/${release_id}/attach_files/${1}?access_token=${GITEE_TOKEN}" \
    >/dev/null 2>&1 || true
}

uploaded=0
replaced=0
skipped=0
for f in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/checksums.txt; do
  [ -f "$f" ] || continue
  fn="$(basename "$f")"
  if stat --version >/dev/null 2>&1; then local_size="$(stat -c%s "$f")"; else local_size="$(stat -f%z "$f")"; fi
  # Every attach id currently carrying this name (may be >1 from a botched run).
  ids="$(printf '%s\n' "$assets_map" | awk -F'\t' -v n="$fn" '$1==n {print $2}')"
  gitee_size="$(printf '%s\n' "$assets_map" | awk -F'\t' -v n="$fn" '$1==n {print $4; exit}')"
  count="$(printf '%s' "$ids" | grep -c . || true)"

  if [ "$count" -eq 0 ]; then
    echo "   ⬆ ${fn} (new)"
    if gitee_attach "$f"; then uploaded=$((uploaded + 1)); else echo "   ⚠ upload may have failed for ${fn}" >&2; fi
    continue
  fi

  if [ "$count" -eq 1 ]; then
    if [ "$gitee_size" = "$local_size" ]; then
      echo "   ✓ ${fn} size matches on Gitee (${local_size} bytes) — skip"
      skipped=$((skipped + 1))
      continue
    fi
    echo "   ↻ ${fn} size mismatch on Gitee (local=${local_size}, gitee=${gitee_size}) — deleting + re-uploading"
  else
    echo "   ↻ ${fn} has ${count} copies on Gitee (dup) — deleting all + re-uploading one"
  fi

  # Delete every copy, then upload exactly one fresh, correct file.
  printf '%s\n' "$ids" | while read -r aid; do
    [ -n "$aid" ] && gitee_delete "$aid"
  done
  if gitee_attach "$f"; then replaced=$((replaced + 1)); else echo "   ⚠ re-upload may have failed for ${fn}" >&2; fi
done

if [ "$uploaded" -eq 0 ] && [ "$replaced" -eq 0 ] && [ "$skipped" -eq 0 ]; then
  echo "❌ No artifacts found to mirror. Did the build (goreleaser) run / were assets downloaded into ${DIST_DIR}?" >&2
  exit 1
fi
echo "✅ Gitee release ${VERSION}: uploaded ${uploaded}, replaced ${replaced}, skipped ${skipped} (already correct)."
echo "   China install:  DWS_GITEE_REPO=${GITEE_REPO} \\"
echo "     curl -fsSL https://gitee.com/${GITEE_REPO}/raw/main/scripts/install.sh | sh"
