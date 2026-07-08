#!/usr/bin/env bash
# Publish locally-built release artifacts to the matching Gitee release.
#
# Intended to run inside Gitee Go after building artifacts in China. This avoids
# the unreliable GitHub Actions -> Gitee cross-border upload path.

set -eu

DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_REPO="${GITEE_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
GITEE_TOKEN="${GITEE_TOKEN:-${GITEE_ACCESS_TOKEN:-}}"
GITEE_CURL_CONNECT_TIMEOUT="${GITEE_CURL_CONNECT_TIMEOUT:-15}"
GITEE_CURL_MAX_TIME="${GITEE_CURL_MAX_TIME:-120}"
GITEE_UPLOAD_MAX_TIME="${GITEE_UPLOAD_MAX_TIME:-300}"
GITEE_UPLOAD_RETRIES="${GITEE_UPLOAD_RETRIES:-3}"
GITEE_UPLOAD_RETRY_DELAY="${GITEE_UPLOAD_RETRY_DELAY:-10}"

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

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum ${1:+"$1"} | awk '{print $1}'
  else shasum -a 256 ${1:+"$1"} | awk '{print $1}'
  fi
}

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

assets_map="$(api_get "${base}/releases/${release_id}/attach_files?access_token=${GITEE_TOKEN}" 2>/dev/null \
  | python3 -c 'import json,sys
try:
    data=json.load(sys.stdin)
    rows=data if isinstance(data,list) else data.get("attach_files",[])
    for a in rows:
        n=a.get("name",""); i=a.get("id",""); u=a.get("browser_download_url","")
        if n and i!="":
            print("%s\t%s\t%s" % (n, i, u))
except Exception:
    pass' 2>/dev/null || true)"

gitee_attach() {
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

gitee_delete() {
  curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" --max-time "$GITEE_CURL_MAX_TIME" \
    -X DELETE "${base}/releases/${release_id}/attach_files/${1}?access_token=${GITEE_TOKEN}" \
    >/dev/null 2>&1 || true
}

uploaded=0
replaced=0
skipped=0
failed=0
for f in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/checksums.txt "$DIST_DIR"/dws-skills.zip; do
  [ -f "$f" ] || continue
  fn="$(basename "$f")"
  local_sha="$(sha256_of "$f")"
  ids="$(printf '%s\n' "$assets_map" | awk -F'\t' -v n="$fn" '$1==n {print $2}')"
  aurl="$(printf '%s\n' "$assets_map" | awk -F'\t' -v n="$fn" '$1==n {print $3; exit}')"
  count="$(printf '%s' "$ids" | grep -c . || true)"

  if [ "$count" -eq 1 ]; then
    gitee_sha="$(api_get "$aurl" 2>/dev/null | sha256_of || true)"
    if [ "$gitee_sha" = "$local_sha" ]; then
      echo "   ✓ ${fn} already correct on Gitee — skip"
      skipped=$((skipped + 1))
      continue
    fi
    echo "   ↻ ${fn} differs on Gitee — replacing"
  elif [ "$count" -gt 1 ]; then
    echo "   ↻ ${fn} has ${count} copies on Gitee — replacing"
  else
    echo "   ⬆ ${fn} (new)"
  fi

  printf '%s\n' "$ids" | while read -r aid; do
    [ -n "$aid" ] && gitee_delete "$aid"
  done
  if gitee_attach "$f"; then
    if [ "$count" -eq 0 ]; then uploaded=$((uploaded + 1)); else replaced=$((replaced + 1)); fi
  else
    echo "   ❌ upload failed for ${fn}" >&2
    failed=$((failed + 1))
  fi
done

[ "$failed" -eq 0 ] || err "Gitee publish finished with ${failed} failed upload(s)"
echo "✅ Gitee release ${VERSION}: uploaded ${uploaded}, replaced ${replaced}, skipped ${skipped}"
