#!/usr/bin/env bash
# Reconcile the complete DWS release asset set with one Gitee release.

set -euo pipefail

DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_CURL_CONNECT_TIMEOUT="${GITEE_CURL_CONNECT_TIMEOUT:-15}"
GITEE_CURL_MAX_TIME="${GITEE_CURL_MAX_TIME:-120}"
GITEE_LIST_MAX_TIME="${GITEE_LIST_MAX_TIME:-20}"
GITEE_VERIFY_MAX_TIME="${GITEE_VERIFY_MAX_TIME:-60}"
GITEE_MUTATION_MAX_TIME="${GITEE_MUTATION_MAX_TIME:-20}"
GITEE_UPLOAD_MAX_TIME="${GITEE_UPLOAD_MAX_TIME:-300}"
GITEE_UPLOAD_RETRIES="${GITEE_UPLOAD_RETRIES:-2}"
GITEE_UPLOAD_RETRY_DELAY="${GITEE_UPLOAD_RETRY_DELAY:-5}"
GITEE_EXISTING_VERIFY_ATTEMPTS="${GITEE_EXISTING_VERIFY_ATTEMPTS:-1}"
GITEE_POST_UPLOAD_VERIFY_ATTEMPTS="${GITEE_POST_UPLOAD_VERIFY_ATTEMPTS:-2}"
GITEE_VERIFY_RETRY_DELAY="${GITEE_VERIFY_RETRY_DELAY:-5}"
GITEE_ASSET_TIMEOUT_SECONDS="${GITEE_ASSET_TIMEOUT_SECONDS:-600}"
GITEE_OVERALL_TIMEOUT_SECONDS="${GITEE_OVERALL_TIMEOUT_SECONDS:-4920}"

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
  GITEE_LIST_MAX_TIME \
  GITEE_VERIFY_MAX_TIME \
  GITEE_MUTATION_MAX_TIME \
  GITEE_UPLOAD_MAX_TIME \
  GITEE_UPLOAD_RETRIES \
  GITEE_EXISTING_VERIFY_ATTEMPTS \
  GITEE_POST_UPLOAD_VERIFY_ATTEMPTS \
  GITEE_ASSET_TIMEOUT_SECONDS \
  GITEE_OVERALL_TIMEOUT_SECONDS; do
  require_positive_integer "$setting" "${!setting}"
done
for setting in \
  GITEE_UPLOAD_RETRY_DELAY \
  GITEE_VERIFY_RETRY_DELAY; do
  require_nonnegative_integer "$setting" "${!setting}"
done

[ -n "${GITEE_TOKEN:-}" ] || err "GITEE_TOKEN is required"
[ -n "${GITEE_REPO:-}" ] || err "GITEE_REPO is required"
[ -n "${GITEE_RELEASE_ID:-}" ] || err "GITEE_RELEASE_ID is required"
[ -d "$DIST_DIR" ] || err "dist dir not found: ${DIST_DIR}"

OWNER="${GITEE_REPO%%/*}"
NAME="${GITEE_REPO##*/}"
base="${GITEE_API}/repos/${OWNER}/${NAME}"
release_id="$GITEE_RELEASE_ID"

now_seconds() {
  date +%s
}

overall_deadline=$(( $(now_seconds) + GITEE_OVERALL_TIMEOUT_SECONDS ))
active_deadline="$overall_deadline"

deadline_remaining() {
  local remaining=$(( active_deadline - $(now_seconds) ))
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

start_asset_deadline() {
  local now candidate
  now="$(now_seconds)"
  [ "$now" -lt "$overall_deadline" ] || return 1
  candidate=$(( now + GITEE_ASSET_TIMEOUT_SECONDS ))
  if [ "$candidate" -lt "$overall_deadline" ]; then
    active_deadline="$candidate"
  else
    active_deadline="$overall_deadline"
  fi
}

sleep_within_deadline() {
  local seconds="$1" remaining
  [ "$seconds" -gt 0 ] || return 0
  remaining="$(deadline_remaining)" || return 1
  [ "$seconds" -lt "$remaining" ] || return 1
  sleep "$seconds"
}

required_assets=(
  dws-darwin-amd64.tar.gz
  dws-darwin-arm64.tar.gz
  dws-linux-amd64.tar.gz
  dws-linux-arm64.tar.gz
  dws-windows-amd64.zip
  dws-windows-arm64.zip
  dws-skills.zip
  checksums.txt
)

missing_assets=()
for name in "${required_assets[@]}"; do
  [ -f "${DIST_DIR}/${name}" ] || missing_assets+=("$name")
done
if [ "${#missing_assets[@]}" -gt 0 ]; then
  err "required release assets are missing from ${DIST_DIR}: ${missing_assets[*]}"
fi

api_get() {
  local configured_max_time="$1" max_time
  shift
  max_time="$(bounded_max_time "$configured_max_time")" || return 1
  curl -fsSL --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
    --max-time "$max_time" "$@"
}

list_assets() {
  api_get "$GITEE_LIST_MAX_TIME" \
    -H "Authorization: token ${GITEE_TOKEN}" \
    "${base}/releases/${release_id}/attach_files" \
    | python3 -c 'import json,sys
data=json.load(sys.stdin)
rows=data if isinstance(data,list) else data.get("attach_files",[])
if not isinstance(rows,list):
    raise ValueError("attach_files must be a list")
for asset in rows:
    name=asset.get("name","")
    asset_id=asset.get("id","")
    url=asset.get("browser_download_url","")
    if name and asset_id != "":
        print("%s\t%s\t%s" % (name, asset_id, url))'
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ${1:+"$1"} | awk '{print $1}'
  else
    shasum -a 256 ${1:+"$1"} | awk '{print $1}'
  fi
}

asset_ids() {
  local assets_map="$1" name="$2"
  printf '%s\n' "$assets_map" | awk -F'\t' -v n="$name" '$1 == n {print $2}'
}

asset_url() {
  local assets_map="$1" name="$2"
  printf '%s\n' "$assets_map" | awk -F'\t' -v n="$name" '$1 == n {print $3; exit}'
}

asset_count() {
  local assets_map="$1" name="$2"
  printf '%s\n' "$assets_map" | awk -F'\t' -v n="$name" '$1 == n {count++} END {print count + 0}'
}

verify_asset_once() {
  local file="$1" name local_sha assets_map count url remote_sha
  name="$(basename "$file")"
  local_sha="$(sha256_of "$file")"
  if ! assets_map="$(list_assets)"; then
    return 1
  fi
  count="$(asset_count "$assets_map" "$name")"
  [ "$count" -eq 1 ] || return 1
  url="$(asset_url "$assets_map" "$name")"
  [ -n "$url" ] || return 1
  if ! remote_sha="$(api_get "$GITEE_VERIFY_MAX_TIME" "$url" | sha256_of)"; then
    return 1
  fi
  [ "$remote_sha" = "$local_sha" ]
}

verify_asset_with_retries() {
  local file="$1" attempts="$2" attempt=1
  while [ "$attempt" -le "$attempts" ]; do
    if verify_asset_once "$file"; then
      return 0
    fi
    attempt=$((attempt + 1))
    if [ "$attempt" -le "$attempts" ]; then
      sleep_within_deadline "$GITEE_VERIFY_RETRY_DELAY" || return 1
    fi
  done
  return 1
}

delete_asset() {
  local asset_id="$1" max_time
  max_time="$(bounded_max_time "$GITEE_MUTATION_MAX_TIME")" || return 1
  curl -fsS --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
    --max-time "$max_time" \
    -H "Authorization: token ${GITEE_TOKEN}" \
    -X DELETE "${base}/releases/${release_id}/attach_files/${asset_id}" \
    >/dev/null
}

delete_named_assets() {
  local name="$1" assets_map ids asset_id
  if ! assets_map="$(list_assets)"; then
    return 1
  fi
  ids="$(asset_ids "$assets_map" "$name")"
  while IFS= read -r asset_id; do
    [ -n "$asset_id" ] || continue
    delete_asset "$asset_id" || return 1
  done <<<"$ids"
  return 0
}

gitee_attach() {
  local file="$1" name attempt response status max_time
  name="$(basename "$file")"
  attempt=1
  while [ "$attempt" -le "$GITEE_UPLOAD_RETRIES" ]; do
    status=0
    max_time="$(bounded_max_time "$GITEE_UPLOAD_MAX_TIME")" || return 1
    if response="$(curl -fsS --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
      --max-time "$max_time" \
      -X POST "${base}/releases/${release_id}/attach_files" \
      -H "Authorization: token ${GITEE_TOKEN}" \
      -H "Expect:" \
      -F "file=@${file}" 2>&1)"; then
      status=0
    else
      status=$?
    fi

    # Gitee can commit an upload but let the HTTP response time out. Probe the
    # release before retrying so a lost response does not create duplicates.
    if verify_asset_with_retries "$file" "$GITEE_POST_UPLOAD_VERIFY_ATTEMPTS"; then
      if [ "$status" -ne 0 ]; then
        echo "   ✓ ${name} appeared with the expected SHA after a lost upload response"
      fi
      return 0
    fi

    echo "   ⚠ upload attempt ${attempt}/${GITEE_UPLOAD_RETRIES} failed for ${name}: $(printf '%s' "$response" | head -c 240)" >&2
    attempt=$((attempt + 1))
    if [ "$attempt" -le "$GITEE_UPLOAD_RETRIES" ]; then
      # Remove any partial, stale, or duplicate attachment before the one
      # explicit retry. There is deliberately no nested curl retry layer.
      delete_named_assets "$name" || return 1
      sleep_within_deadline "$GITEE_UPLOAD_RETRY_DELAY" || return 1
    fi
  done
  return 1
}

uploaded=0
replaced=0
skipped=0
failed=0
index=0
total="${#required_assets[@]}"

for name in "${required_assets[@]}"; do
  index=$((index + 1))
  file="${DIST_DIR}/${name}"
  if ! start_asset_deadline; then
    err "overall Gitee reconciliation deadline exhausted before ${name}"
  fi
  if ! assets_map="$(list_assets)"; then
    if deadline_remaining >/dev/null; then
      echo "   ❌ could not list Gitee assets before reconciling ${name}" >&2
    else
      echo "   ❌ ${name} exceeded its Gitee reconciliation deadline" >&2
    fi
    failed=$((failed + 1))
    continue
  fi
  count="$(asset_count "$assets_map" "$name")"
  existed="$count"

  if [ "$count" -eq 1 ] && verify_asset_with_retries "$file" "$GITEE_EXISTING_VERIFY_ATTEMPTS"; then
    echo "   ✓ [${index}/${total}] ${name} already correct on Gitee — skip"
    skipped=$((skipped + 1))
    continue
  fi

  if [ "$count" -gt 0 ]; then
    echo "   ↻ [${index}/${total}] ${name} is stale or duplicated (${count} copies) — replacing"
    if ! delete_named_assets "$name"; then
      echo "   ❌ failed to delete stale Gitee attachment(s) for ${name}" >&2
      failed=$((failed + 1))
      continue
    fi
  else
    echo "   ⬆ [${index}/${total}] ${name} (new)"
  fi

  if gitee_attach "$file"; then
    if [ "$existed" -eq 0 ]; then
      uploaded=$((uploaded + 1))
    else
      replaced=$((replaced + 1))
    fi
  else
    if deadline_remaining >/dev/null; then
      echo "   ❌ upload failed for ${name}" >&2
    else
      echo "   ❌ upload failed for ${name}: per-asset or overall deadline exhausted" >&2
    fi
    failed=$((failed + 1))
  fi
done

active_deadline="$overall_deadline"
deadline_remaining >/dev/null || err "overall Gitee reconciliation deadline exhausted before final verification"
if ! final_assets_map="$(list_assets)"; then
  err "could not list Gitee assets for final verification"
fi
for name in "${required_assets[@]}"; do
  count="$(asset_count "$final_assets_map" "$name")"
  if [ "$count" -ne 1 ]; then
    echo "   ❌ final verification expected exactly one ${name}, found ${count}" >&2
    failed=$((failed + 1))
  fi
done

[ "$failed" -eq 0 ] || err "Gitee release reconciliation finished with ${failed} failure(s)"
echo "✅ Gitee release assets: uploaded ${uploaded}, replaced ${replaced}, skipped ${skipped} (all ${total} verified)."
