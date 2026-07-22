#!/usr/bin/env bash
# Reconcile the complete DWS release asset set with one Gitee release.

set -euo pipefail

DIST_DIR="${DIST_DIR:-dist}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_CURL_CONNECT_TIMEOUT="${GITEE_CURL_CONNECT_TIMEOUT:-15}"
GITEE_CURL_MAX_TIME="${GITEE_CURL_MAX_TIME:-120}"
GITEE_LIST_MAX_TIME="${GITEE_LIST_MAX_TIME:-20}"
# Attachment-list reads are safe to retry and are used both before an upload
# and to detect a committed upload whose HTTP response was lost. Keep retrying
# long enough to bridge a short Gitee TLS/API outage without ever blindly
# replaying the upload itself.
GITEE_LIST_RETRIES="${GITEE_LIST_RETRIES:-24}"
GITEE_LIST_RETRY_DELAY="${GITEE_LIST_RETRY_DELAY:-20}"
GITEE_LIST_RETRY_WINDOW_SECONDS="${GITEE_LIST_RETRY_WINDOW_SECONDS:-420}"
GITEE_VERIFY_MAX_TIME="${GITEE_VERIFY_MAX_TIME:-60}"
GITEE_MUTATION_MAX_TIME="${GITEE_MUTATION_MAX_TIME:-20}"
# Gitee does not return a response until an attachment upload is committed.
# From GitHub-hosted runners, a near-10 MiB DWS binary can legitimately take
# more than five minutes, so keep the transfer deadline above that observed
# floor while the per-asset and overall deadlines retain hard upper bounds.
GITEE_UPLOAD_MAX_TIME="${GITEE_UPLOAD_MAX_TIME:-1200}"
# The per-asset deadline guarantees one complete slow upload plus one complete
# attachment-list recovery on each side of that upload. The second upload
# attempt is allowed only for zero-byte failures before HTTP begins; a first
# attempt that consumes the full transfer deadline intentionally exhausts the
# retry budget instead of holding a runner for another full slow upload.
GITEE_UPLOAD_RETRIES="${GITEE_UPLOAD_RETRIES:-2}"
GITEE_UPLOAD_RETRY_DELAY="${GITEE_UPLOAD_RETRY_DELAY:-5}"
GITEE_EXISTING_VERIFY_ATTEMPTS="${GITEE_EXISTING_VERIFY_ATTEMPTS:-1}"
GITEE_POST_UPLOAD_VERIFY_ATTEMPTS="${GITEE_POST_UPLOAD_VERIFY_ATTEMPTS:-2}"
GITEE_VERIFY_RETRY_DELAY="${GITEE_VERIFY_RETRY_DELAY:-5}"
GITEE_ASSET_TIMEOUT_SECONDS="${GITEE_ASSET_TIMEOUT_SECONDS:-2220}"
GITEE_OVERALL_TIMEOUT_SECONDS="${GITEE_OVERALL_TIMEOUT_SECONDS:-18300}"

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
  GITEE_LIST_RETRIES \
  GITEE_LIST_RETRY_WINDOW_SECONDS \
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
  GITEE_LIST_RETRY_DELAY \
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

list_assets_once() {
  local max_time="$1" payload
  payload="$(api_get "$max_time" \
    -H "Authorization: token ${GITEE_TOKEN}" \
    "${base}/releases/${release_id}/attach_files")" || return 1
  printf '%s\n' "$payload" | python3 -c 'import json,sys
data=json.load(sys.stdin)
if isinstance(data,list):
    rows=data
elif isinstance(data,dict) and "attach_files" in data:
    rows=data["attach_files"]
else:
    raise ValueError("attachment list response must be a list or contain attach_files")
if not isinstance(rows,list):
    raise ValueError("attach_files must be a list")
for asset in rows:
    if not isinstance(asset,dict):
        raise ValueError("each attachment must be an object")
    name=asset.get("name","")
    asset_id=asset.get("id","")
    url=asset.get("browser_download_url","")
    valid_id=(isinstance(asset_id,int) and not isinstance(asset_id,bool) and asset_id > 0) or (isinstance(asset_id,str) and asset_id.isdigit() and int(asset_id) > 0)
    if not isinstance(name,str) or not name or not valid_id or not isinstance(url,str) or not url:
        raise ValueError("attachment identity and download URL are required")
    print("%s\t%s\t%s" % (name, asset_id, url))'
}

list_assets() {
  local attempt=1 candidate now retry_deadline remaining max_time delay
  now="$(now_seconds)"
  retry_deadline=$(( now + GITEE_LIST_RETRY_WINDOW_SECONDS ))
  if [ "$retry_deadline" -gt "$active_deadline" ]; then
    retry_deadline="$active_deadline"
  fi
  while [ "$attempt" -le "$GITEE_LIST_RETRIES" ]; do
    now="$(now_seconds)"
    remaining=$(( retry_deadline - now ))
    [ "$remaining" -gt 0 ] || break
    max_time="$GITEE_LIST_MAX_TIME"
    if [ "$max_time" -gt "$remaining" ]; then
      max_time="$remaining"
    fi
    # Publish one complete parse atomically. A malformed response can make the
    # parser emit valid leading rows before it fails; leaking those rows into a
    # later successful retry would manufacture duplicates and could delete a
    # correct attachment.
    if candidate="$(list_assets_once "$max_time")"; then
      now="$(now_seconds)"
      if [ "$now" -lt "$retry_deadline" ]; then
        printf '%s\n' "$candidate"
        return 0
      fi
      break
    fi
    if [ "$attempt" -ge "$GITEE_LIST_RETRIES" ]; then
      break
    fi
    attempt=$((attempt + 1))
    now="$(now_seconds)"
    remaining=$(( retry_deadline - now ))
    [ "$remaining" -gt 0 ] || break
    delay="$GITEE_LIST_RETRY_DELAY"
    if [ "$delay" -gt 0 ]; then
      # Do not turn a configured backoff into a burst of zero-delay requests
      # during the final wall-clock second. Explicit delay=0 remains available
      # to fast unit tests and tightly controlled callers.
      [ "$remaining" -gt 1 ] || break
      if [ "$delay" -ge "$remaining" ]; then
        delay=$(( remaining - 1 ))
      fi
    fi
    echo "   ⚠ Gitee attachment list attempt $((attempt - 1))/${GITEE_LIST_RETRIES} failed; retrying in ${delay}s" >&2
    sleep_within_deadline "$delay" || return 1
  done
  return 1
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
  local file="$1" name attempt response status max_time transfer_log metrics http_code size_upload safe_to_retry
  name="$(basename "$file")"
  attempt=1
  while [ "$attempt" -le "$GITEE_UPLOAD_RETRIES" ]; do
    status=0
    max_time="$(bounded_max_time "$GITEE_UPLOAD_MAX_TIME")" || return 1
    transfer_log="$(mktemp)" || return 1
    if metrics="$(curl -fsS --connect-timeout "$GITEE_CURL_CONNECT_TIMEOUT" \
      --max-time "$max_time" \
      -X POST "${base}/releases/${release_id}/attach_files" \
      -H "Authorization: token ${GITEE_TOKEN}" \
      -H "Expect:" \
      -F "file=@${file}" \
      -o /dev/null \
      -w '%{http_code}\t%{size_upload}' \
      2>"$transfer_log")"; then
      status=0
    else
      status=$?
    fi
    response="$(<"$transfer_log")"
    rm -f "$transfer_log"
    http_code="${metrics%%$'\t'*}"
    if [ "$metrics" = "$http_code" ]; then
      size_upload=""
    else
      size_upload="${metrics#*$'\t'}"
    fi
    safe_to_retry=0
    if [ "$status" -ne 0 ] && [ "$http_code" = "000" ] && \
      awk -v value="$size_upload" 'BEGIN { exit !((value + 0) == 0 && value ~ /^[0-9]+([.][0-9]+)?$/) }'; then
      # A request that never reached HTTP and uploaded zero bytes cannot have
      # committed an attachment. TLS/DNS/connect failures are therefore the
      # only transport failures safe to replay automatically.
      safe_to_retry=1
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
    if [ "$safe_to_retry" -ne 1 ]; then
      echo "   ⚠ ${name} upload outcome is ambiguous (HTTP ${http_code:-unknown}, uploaded ${size_upload:-unknown} bytes); refusing to replay POST" >&2
      return 1
    fi
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
