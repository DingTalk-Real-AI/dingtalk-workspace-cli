#!/usr/bin/env bash
# Withdraw one already-published DWS release from every distribution channel
# without pretending that installed clients can be downgraded.
#
# The withdrawal is deliberately resumable. A permanent annotated tombstone is
# created before any channel mutation. Reruns accept only that exact tombstone;
# the ref is never updated or deleted.

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/release/release-lib.sh
. "$SCRIPT_DIR/release-lib.sh"

VERSION="${1:-}"
REASON="${2:-}"
CONFIRMATION="${3:-}"

OFFICIAL_REPOSITORY="${DWS_RELEASE_OFFICIAL_REPOSITORY:-DingTalk-Real-AI/dingtalk-workspace-cli}"
PACKAGE_NAME="${DWS_NPM_PACKAGE:-dingtalk-workspace-cli}"
DEFAULT_BRANCH="${DWS_DEFAULT_BRANCH:-${GITHUB_EVENT_DEFAULT_BRANCH:-}}"
TOMBSTONE="withdrawn/${VERSION}"
GITEE_API="${GITEE_API:-https://gitee.com/api/v5}"
GITEE_REPO="${GITEE_REPO:-DingTalk-Real-AI/dingtalk-workspace-cli}"
OSS_PREFIX="${OSS_PREFIX:-dws}"
DELIVERY_VERIFIER="${DWS_DELIVERY_VERIFIER:-$SCRIPT_DIR/verify-release-workflow-delivery.sh}"
STABLE_DELIVERY_VERIFIER="${DWS_STABLE_DELIVERY_VERIFIER:-$SCRIPT_DIR/verify-delivered-stable.sh}"
GITHUB_DOWNLOAD_HELPER="${DWS_GITHUB_DOWNLOAD_HELPER:-$SCRIPT_DIR/download-github-release-assets.sh}"
ARTIFACT_VERIFY_HELPER="${DWS_ARTIFACT_VERIFY_HELPER:-$SCRIPT_DIR/verify-release-artifacts.sh}"
GITEE_SYNC_HELPER="${DWS_GITEE_SYNC_HELPER:-$SCRIPT_DIR/sync-to-gitee.sh}"

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

say() {
  printf '%s\n' "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

need_env() {
  local name="$1" value="$2"
  [ -n "$value" ] || err "missing required environment variable: $name"
}

sha256_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    shasum -a 256 | awk '{print $1}'
  fi
}

github_api() {
  gh api -H 'X-GitHub-Api-Version: 2026-03-10' "$@"
}

github_optional() {
  local error_file status
  error_file="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-github-optional.XXXXXX")"
  set +e
  github_api "$@" 2>"$error_file"
  status=$?
  set -e
  if [ "$status" -eq 0 ]; then
    rm -f "$error_file"
    return 0
  fi
  if grep -Eq '\(HTTP 404\)|HTTP 404([^0-9]|$)' "$error_file"; then
    rm -f "$error_file"
    return 4
  fi
  cat "$error_file" >&2
  rm -f "$error_file"
  return "$status"
}

github_expect_404() {
  local endpoint="$1" error_file
  error_file="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-github-404.XXXXXX")"
  if github_api "$endpoint" >/dev/null 2>"$error_file"; then
    rm -f "$error_file"
    err "GitHub resource still exists after deletion: $endpoint"
  fi
  if ! grep -Eq '\(HTTP 404\)|HTTP 404([^0-9]|$)' "$error_file"; then
    cat "$error_file" >&2
    rm -f "$error_file"
    err "could not prove GitHub resource deletion: $endpoint"
  fi
  rm -f "$error_file"
}

github_delete_if_present() {
  local endpoint="$1" description="$2" error_file status
  error_file="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-github-delete.XXXXXX")"
  set +e
  github_api --method DELETE "$endpoint" >/dev/null 2>"$error_file"
  status=$?
  set -e
  if [ "$status" -eq 0 ]; then
    rm -f "$error_file"
    return 0
  fi
  if grep -Eq '\(HTTP 404\)|HTTP 404([^0-9]|$)' "$error_file"; then
    rm -f "$error_file"
    say "$description was already absent."
    return 0
  fi
  cat "$error_file" >&2
  rm -f "$error_file"
  err "could not delete $description"
}

release_json() {
  github_api "repos/${OFFICIAL_REPOSITORY}/releases/tags/$1"
}

release_field() {
  local version="$1" expression="$2"
  release_json "$version" | python3 -c '
import json
import sys

data = json.load(sys.stdin)
path = sys.argv[1].split(".")
value = data
for item in path:
    if item:
        value = value.get(item) if isinstance(value, dict) else None
if value is True:
    print("true")
elif value is False:
    print("false")
elif value is not None:
    print(value)
' "$expression"
}

tag_commit() {
  git rev-parse --verify "$1^{commit}" 2>/dev/null
}

tag_object() {
  git rev-parse --verify "refs/tags/$1" 2>/dev/null
}

has_tombstone() {
  git show-ref --verify --quiet "refs/tags/withdrawn/$1"
}

expected_prerelease() {
  if [ "$1" = prerelease ]; then
    printf '%s\n' true
  else
    printf '%s\n' false
  fi
}

npm_exact_version() {
  npm view "${PACKAGE_NAME}@${1#v}" version --registry=https://registry.npmjs.org 2>/dev/null
}

npm_deprecation() {
  npm view "${PACKAGE_NAME}@${1#v}" deprecated --registry=https://registry.npmjs.org 2>/dev/null
}

release_is_eligible_rollback() {
  local candidate="$1" channel="$2" state commit deprecation immutable

  release_validate_version_channel "$channel" "$candidate" >/dev/null 2>&1 || return 1
  has_tombstone "$candidate" && return 1
  [ "$(git cat-file -t "$(tag_object "$candidate")" 2>/dev/null || true)" = tag ] || return 1
  commit="$(tag_commit "$candidate" 2>/dev/null || true)"
  printf '%s\n' "$commit" | grep -Eq '^[0-9a-f]{40}$' || return 1

  state="$(
    release_json "$candidate" 2>/dev/null |
      python3 -c 'import json,sys
d=json.load(sys.stdin)
print("%s\t%s\t%s" % (
    str(bool(d.get("draft"))).lower(),
    str(bool(d.get("prerelease"))).lower(),
    "withdrawn" if "<!-- dws-release-withdrawn" in (d.get("body") or "") else "active",
))' 2>/dev/null || true
  )"
  [ "$state" = "$(printf 'false\t%s\tactive' "$(expected_prerelease "$channel")")" ] || return 1
  immutable="$(release_field "$candidate" immutable 2>/dev/null || true)"
  [ "$(npm_exact_version "$candidate" || true)" = "${candidate#v}" ] || return 1
  if ! deprecation="$(npm_deprecation "$candidate")"; then
    return 1
  fi
  [ -z "$deprecation" ] || return 1
  if [ "$immutable" = true ]; then
    DWS_RELEASE_OFFICIAL_REPOSITORY="$OFFICIAL_REPOSITORY" \
      DWS_RELEASE_GITHUB_TOKEN="${GITHUB_TOKEN:-}" \
      "$DELIVERY_VERIFIER" "$candidate" "$commit" >/dev/null 2>&1
    return
  fi
  [ "$channel" = stable ] || return 1
  DWS_RELEASE_OFFICIAL_REPOSITORY="$OFFICIAL_REPOSITORY" \
    DWS_RELEASE_GITHUB_TOKEN="${GITHUB_TOKEN:-}" \
    "$STABLE_DELIVERY_VERIFIER" "$candidate" "$commit" >/dev/null 2>&1
}

find_rollback_version() {
  local channel="$1" candidate
  while IFS= read -r candidate; do
    [ -n "$candidate" ] || continue
    [ "$candidate" != "$VERSION" ] || continue
    release_version_is_greater "$VERSION" "$candidate" || continue
    if release_is_eligible_rollback "$candidate" "$channel"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done < <(
    # shellcheck disable=SC2154 # Defined by sourced release-lib.sh.
    if [ "$channel" = stable ]; then
      git tag --list 'v*' | grep -E "$release_stable_pattern" | sort -Vr
    else
      git tag --list 'v*' | grep -E "$release_prerelease_pattern" | sort -Vr
    fi
  )
  return 1
}

require_safe_channel_pointer() {
  local version="$1" channel="$2"
  release_validate_version_channel "$channel" "$version" >/dev/null 2>&1 ||
    err "channel pointer contains an invalid $channel version: $version"
  if [ "$version" = "$VERSION" ]; then
    return 0
  fi
  release_is_eligible_rollback "$version" "$channel" ||
    err "channel pointer $version is not a delivered, non-withdrawn $channel release"
}

validate_inputs() {
  release_channel_for_version "$VERSION" >/dev/null ||
    err "version must be exactly vX.Y.Z or vX.Y.Z-beta.N"
  [ "$CONFIRMATION" = "WITHDRAW $VERSION" ] ||
    err "confirmation must be exactly: WITHDRAW $VERSION"
  python3 - "$REASON" <<'PY' || err "reason must be a trimmed, printable single line between 8 and 300 characters"
import sys
import unicodedata

reason = sys.argv[1]
if not 8 <= len(reason) <= 300 or reason != reason.strip():
    raise SystemExit(1)
if any(ch in "\r\n" or unicodedata.category(ch).startswith("C") for ch in reason):
    raise SystemExit(1)
PY
}

validate_context() {
  need_env GITHUB_REPOSITORY "${GITHUB_REPOSITORY:-}"
  need_env GITHUB_REF_NAME "${GITHUB_REF_NAME:-}"
  need_env GITHUB_SHA "${GITHUB_SHA:-}"
  need_env GITHUB_RUN_ID "${GITHUB_RUN_ID:-}"
  need_env GITHUB_ACTOR "${GITHUB_ACTOR:-}"
  need_env DWS_DEFAULT_BRANCH "$DEFAULT_BRANCH"
  [ "${GITHUB_ACTIONS:-}" = true ] || err "withdrawal may run only inside GitHub Actions"
  [ "$GITHUB_REPOSITORY" = "$OFFICIAL_REPOSITORY" ] ||
    err "withdrawal is restricted to $OFFICIAL_REPOSITORY"
  [ "$GITHUB_REF_NAME" = "$DEFAULT_BRANCH" ] ||
    err "withdrawal must run from the default branch $DEFAULT_BRANCH"
  printf '%s\n' "$GITHUB_SHA" | grep -Eq '^[0-9a-f]{40}$' ||
    err "GITHUB_SHA must be a full lowercase commit SHA"
  [ "$(git rev-parse HEAD)" = "$GITHUB_SHA" ] ||
    err "checked out withdrawal tooling is not the dispatched commit"

  local default_head
  default_head="$(
    github_api "repos/${OFFICIAL_REPOSITORY}/git/ref/heads/${DEFAULT_BRANCH}" --jq .object.sha
  )"
  [ "$default_head" = "$GITHUB_SHA" ] ||
    err "default branch advanced to $default_head; re-dispatch withdrawal from the new head"
}

ensure_ossutil() {
  OSSUTIL="${OSSUTIL:-ossutil}"
  if command -v "$OSSUTIL" >/dev/null 2>&1; then
    return
  fi

  local os arch expected archive extract found actual install_dir
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=mac ;;
    *) err "unsupported ossutil operating system: $(uname -s)" ;;
  esac
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
  esac
  case "${os}-${arch}" in
    linux-amd64) expected=3ae4d9fc85a7a6e9f5654d1599766f1a3a42a3692870887b5ae9338d582ef65a ;;
    linux-arm64) expected=f6c95ba0c2d2ef30290af686ce4d706c701f4734ce8090bee4288a77e3f1d764 ;;
    mac-amd64) expected=8437fdd3ef1a3eb12310f61fcf1c00a5bff5cdab47b4fea815527472e7cf896c ;;
    mac-arm64) expected=058fd048f321f8c80def8b748030531646eefe3a82837bf16b581ba7d9c84ac7 ;;
    *) err "unsupported ossutil architecture: ${os}-${arch}" ;;
  esac

  archive="$(mktemp "${TMPDIR:-/tmp}/ossutil-2.3.0.XXXXXX.zip")"
  extract="$(mktemp -d "${TMPDIR:-/tmp}/ossutil-2.3.0.XXXXXX")"
  curl -fsSL \
    "https://gosspublic.alicdn.com/ossutil/v2/2.3.0/ossutil-2.3.0-${os}-${arch}.zip" \
    -o "$archive"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
  fi
  [ "$actual" = "$expected" ] || {
    rm -rf "$archive" "$extract"
    err "ossutil archive checksum mismatch for ${os}-${arch}"
  }
  unzip -qo "$archive" -d "$extract"
  found="$(find "$extract" -name ossutil -type f | head -1)"
  [ -n "$found" ] || {
    rm -rf "$archive" "$extract"
    err "verified ossutil archive does not contain the executable"
  }
  install_dir="$(mktemp -d "${TMPDIR:-/tmp}/dws-withdraw-ossutil.XXXXXX")"
  cp "$found" "$install_dir/ossutil"
  chmod 700 "$install_dir/ossutil"
  rm -rf "$archive" "$extract"
  OSSUTIL="$install_dir/ossutil"
  WITHDRAW_OSSUTIL_DIR="$install_dir"
}

oss_cp() {
  "$OSSUTIL" cp -f --endpoint "$OSS_ENDPOINT" "$1" "$2"
}

oss_get_pointer() {
  local pointer_name="$1" output="$2"
  oss_cp "oss://${OSS_BUCKET}/${OSS_PREFIX}/${pointer_name}" "$output"
}

read_oss_pointer() {
  local pointer_name="$1" output
  output="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-oss-pointer.XXXXXX")"
  if ! oss_get_pointer "$pointer_name" "$output" >/dev/null; then
    rm -f "$output"
    err "could not read required OSS channel pointer ${pointer_name}"
  fi
  tr -d '[:space:]' <"$output"
  rm -f "$output"
}

write_oss_pointer() {
  local pointer_name="$1" version="$2" pointer verify
  pointer="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-oss-pointer.XXXXXX")"
  verify="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-oss-verify.XXXXXX")"
  printf '%s\n' "$version" >"$pointer"
  oss_cp "$pointer" "oss://${OSS_BUCKET}/${OSS_PREFIX}/${pointer_name}" >/dev/null
  oss_get_pointer "$pointer_name" "$verify" >/dev/null
  [ "$(tr -d '[:space:]' <"$verify")" = "$version" ] || {
    rm -f "$pointer" "$verify"
    err "OSS ${pointer_name} did not move to $version"
  }
  rm -f "$pointer" "$verify"
}

ensure_oss_rollback() {
  local asset downloaded remote release_id
  WITHDRAW_OSS_ROLLBACK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dws-withdraw-oss-rollback.XXXXXX")"
  release_id="$(release_field "$ROLLBACK_VERSION" id)"
  DWS_GITHUB_RELEASE_ID="$release_id" \
    "$GITHUB_DOWNLOAD_HELPER" "$ROLLBACK_VERSION" "$WITHDRAW_OSS_ROLLBACK_DIR"
  DWS_PACKAGE_DIST_DIR="$WITHDRAW_OSS_ROLLBACK_DIR" \
    "$ARTIFACT_VERIFY_HELPER" "$ROLLBACK_VERSION"

  for asset in \
    checksums.txt \
    dws-darwin-amd64.tar.gz dws-darwin-arm64.tar.gz \
    dws-linux-amd64.tar.gz dws-linux-arm64.tar.gz \
    dws-windows-amd64.zip dws-windows-arm64.zip \
    dws-skills.zip; do
    remote="oss://${OSS_BUCKET}/${OSS_PREFIX}/download/${ROLLBACK_VERSION}/${asset}"
    oss_cp "$WITHDRAW_OSS_ROLLBACK_DIR/$asset" "$remote" >/dev/null
    downloaded="$WITHDRAW_OSS_ROLLBACK_DIR/verified-$asset"
    oss_cp "$remote" "$downloaded" >/dev/null
    cmp -s "$WITHDRAW_OSS_ROLLBACK_DIR/$asset" "$downloaded" ||
      err "OSS rollback asset verification failed: $asset"
  done
  rm -rf "$WITHDRAW_OSS_ROLLBACK_DIR"
  WITHDRAW_OSS_ROLLBACK_DIR=""
  say "OSS rollback release $ROLLBACK_VERSION is complete before moving the channel pointer."
}

prepare_tombstone_message() {
  local reason_sha="$1" output="$2"
  {
    printf 'Withdrawn DWS release %s\n\n' "$VERSION"
    printf 'Version: %s\n' "$VERSION"
    printf 'Original-Tag-Object: %s\n' "$TARGET_TAG_OBJECT"
    printf 'Original-Commit: %s\n' "$TARGET_COMMIT"
    printf 'Original-Release-ID: %s\n' "$TARGET_RELEASE_ID"
    printf 'Channel: %s\n' "$CHANNEL"
    printf 'Reason-SHA256: %s\n' "$reason_sha"
    printf 'Reason: %s\n' "$REASON"
    printf 'Requested-By: %s\n' "$GITHUB_ACTOR"
    printf 'Request-Run: %s\n' "$GITHUB_RUN_ID"
  } >"$output"
}

verify_existing_tombstone() {
  local reason_sha="$1" ref_sha tombstone_json parsed status
  local actual_target original_tag_object original_commit original_release_id
  local tombstone_channel tombstone_version tombstone_reason_sha tombstone_reason
  set +e
  ref_sha="$(
    github_optional \
      "repos/${OFFICIAL_REPOSITORY}/git/ref/tags/${TOMBSTONE}" \
      --jq .object.sha
  )"
  status=$?
  set -e
  case "$status" in
    0) ;;
    4) return 1 ;;
    *) err "could not query permanent tombstone $TOMBSTONE" ;;
  esac
  printf '%s\n' "$ref_sha" | grep -Eq '^[0-9a-f]{40}$' ||
    err "existing tombstone $TOMBSTONE has an invalid tag object"
  tombstone_json="$(
    github_api "repos/${OFFICIAL_REPOSITORY}/git/tags/${ref_sha}"
  )" || err "existing tombstone $TOMBSTONE is not an annotated tag"
  parsed="$(
    printf '%s' "$tombstone_json" |
      python3 -c '
import json
import re
import sys

expected_tag = sys.argv[1]
payload = json.load(sys.stdin)
obj = payload.get("object", {})
if (
    payload.get("tag") != expected_tag
    or obj.get("type") != "commit"
    or not re.fullmatch(r"[0-9a-f]{40}", obj.get("sha", ""))
):
    raise SystemExit(1)
fields = {}
for line in payload.get("message", "").splitlines():
    if ": " not in line:
        continue
    key, value = line.split(": ", 1)
    if key in fields:
        raise SystemExit(1)
    fields[key] = value
required = (
    "Version",
    "Original-Tag-Object",
    "Original-Commit",
    "Original-Release-ID",
    "Channel",
    "Reason-SHA256",
    "Reason",
    "Requested-By",
    "Request-Run",
)
if any(not fields.get(key) for key in required):
    raise SystemExit(1)
if not re.fullmatch(r"[0-9a-f]{40}", fields["Original-Tag-Object"]):
    raise SystemExit(1)
if not re.fullmatch(r"[0-9a-f]{40}", fields["Original-Commit"]):
    raise SystemExit(1)
if not re.fullmatch(r"[1-9][0-9]*", fields["Original-Release-ID"]):
    raise SystemExit(1)
if not re.fullmatch(r"[0-9a-f]{64}", fields["Reason-SHA256"]):
    raise SystemExit(1)
print("\t".join((
    obj["sha"],
    fields["Original-Tag-Object"],
    fields["Original-Commit"],
    fields["Original-Release-ID"],
    fields["Channel"],
    fields["Version"],
    fields["Reason-SHA256"],
    fields["Reason"],
)))' "$TOMBSTONE"
  )" || err "existing tombstone $TOMBSTONE has invalid immutable withdrawal metadata"
  actual_target="$(printf '%s\n' "$parsed" | cut -f1)"
  original_tag_object="$(printf '%s\n' "$parsed" | cut -f2)"
  original_commit="$(printf '%s\n' "$parsed" | cut -f3)"
  original_release_id="$(printf '%s\n' "$parsed" | cut -f4)"
  tombstone_channel="$(printf '%s\n' "$parsed" | cut -f5)"
  tombstone_version="$(printf '%s\n' "$parsed" | cut -f6)"
  tombstone_reason_sha="$(printf '%s\n' "$parsed" | cut -f7)"
  tombstone_reason="$(printf '%s\n' "$parsed" | cut -f8-)"
  [ "$actual_target" = "$original_commit" ] &&
    [ "$tombstone_version" = "$VERSION" ] &&
    [ "$tombstone_channel" = "$CHANNEL" ] &&
    [ "$tombstone_reason_sha" = "$reason_sha" ] &&
    [ "$tombstone_reason" = "$REASON" ] ||
    err "existing tombstone $TOMBSTONE has different immutable withdrawal metadata"
  if [ -n "${TARGET_TAG_OBJECT:-}" ] && [ "$TARGET_TAG_OBJECT" != "$original_tag_object" ]; then
    err "existing tombstone $TOMBSTONE records a different original tag object"
  fi
  if [ -n "${TARGET_COMMIT:-}" ] && [ "$TARGET_COMMIT" != "$original_commit" ]; then
    err "existing tombstone $TOMBSTONE records a different original commit"
  fi
  if [ -n "${TARGET_RELEASE_ID:-}" ] && [ "$TARGET_RELEASE_ID" != "$original_release_id" ]; then
    err "existing tombstone $TOMBSTONE records a different original release"
  fi
  TARGET_TAG_OBJECT="$original_tag_object"
  TARGET_COMMIT="$original_commit"
  TARGET_RELEASE_ID="$original_release_id"
}

create_tombstone() {
  local message_file message reason_sha tag_sha ref_sha create_ref_error
  reason_sha="$(printf '%s' "$REASON" | sha256_text)"
  message_file="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-tombstone.XXXXXX")"
  prepare_tombstone_message "$reason_sha" "$message_file"
  message="$(cat "$message_file")"
  rm -f "$message_file"

  if verify_existing_tombstone "$reason_sha"; then
    say "Permanent tombstone $TOMBSTONE already exists with the exact withdrawal metadata."
    return
  fi

  tag_sha="$(
    github_api --method POST "repos/${OFFICIAL_REPOSITORY}/git/tags" \
      -f tag="$TOMBSTONE" \
      -f message="$message" \
      -f object="$TARGET_COMMIT" \
      -f type=commit \
      --jq .sha
  )"
  printf '%s\n' "$tag_sha" | grep -Eq '^[0-9a-f]{40}$' ||
    err "GitHub returned an invalid tombstone tag object"

  create_ref_error="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-ref-error.XXXXXX")"
  if ! github_api --method POST "repos/${OFFICIAL_REPOSITORY}/git/refs" \
    -f ref="refs/tags/${TOMBSTONE}" \
    -f sha="$tag_sha" >/dev/null 2>"$create_ref_error"; then
    if verify_existing_tombstone "$reason_sha"; then
      say "Permanent tombstone $TOMBSTONE was created concurrently with identical metadata."
      rm -f "$create_ref_error"
      return
    fi
    cat "$create_ref_error" >&2
    rm -f "$create_ref_error"
    err "could not create permanent tombstone $TOMBSTONE"
  fi
  rm -f "$create_ref_error"
  verify_existing_tombstone "$reason_sha" ||
    err "permanent tombstone $TOMBSTONE could not be verified"
  say "Created permanent tombstone $TOMBSTONE. This ref must never be moved or deleted."
}

update_github_release() {
  local original_body begin_marker end_marker block notes payload state
  if [ "${TARGET_RELEASE_EXISTS:-false}" != true ]; then
    say "GitHub Release $VERSION is already absent; permanent tombstone metadata authorizes resumption."
    return
  fi
  begin_marker="<!-- dws-release-withdrawn-begin version=${VERSION} -->"
  end_marker="<!-- dws-release-withdrawn-end version=${VERSION} -->"
  original_body="$(release_field "$VERSION" body)"
  block="${begin_marker}
## Withdrawn

This release has been withdrawn from distribution.

- Reason: ${REASON}
- Tombstone: \`${TOMBSTONE}\`

Already-installed clients cannot be remotely downgraded. Install the current ${CHANNEL} channel or a later fixed version.
${end_marker}"
  notes="$(
    ORIGINAL_BODY="$original_body" WITHDRAW_BLOCK="$block" \
      BEGIN_MARKER="$begin_marker" END_MARKER="$end_marker" python3 - <<'PY'
import os

body = os.environ["ORIGINAL_BODY"]
block = os.environ["WITHDRAW_BLOCK"]
begin = os.environ["BEGIN_MARKER"]
end = os.environ["END_MARKER"]
if begin in body and end in body and body.index(begin) < body.index(end):
    prefix, remainder = body.split(begin, 1)
    _, suffix = remainder.split(end, 1)
    print(prefix + block + suffix, end="")
else:
    separator = "\n\n" if body else ""
    print(body + separator + block, end="")
PY
  )"

  payload="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-release-json.XXXXXX")"
  NOTES="$notes" python3 - "$VERSION" >"$payload" <<'PY'
import json
import os
import sys

print(json.dumps({
    "name": f"[WITHDRAWN] {sys.argv[1]}",
    "body": os.environ["NOTES"],
}))
PY
  github_api --method PATCH \
    "repos/${OFFICIAL_REPOSITORY}/releases/${TARGET_RELEASE_ID}" \
    --input "$payload" >/dev/null
  rm -f "$payload"

  state="$(
    release_json "$VERSION" |
      python3 -c 'import json,sys
d=json.load(sys.stdin)
print("%s\t%s\t%s" % (
    d.get("name", ""),
    "marked" if "<!-- dws-release-withdrawn-begin " in (d.get("body") or "") else "unmarked",
    str(bool(d.get("immutable"))).lower(),
))'
  )"
  [ "$state" = "$(printf '[WITHDRAWN] %s\tmarked\ttrue' "$VERSION")" ] ||
    err "GitHub Release was not durably marked withdrawn"
}

update_npm_channel() {
  local deprecation_message delivered_deprecation delivered_pointer current_pointer
  deprecation_message="WITHDRAWN ${VERSION}: ${REASON}. Use ${ROLLBACK_VERSION} or a later fixed version."
  npm deprecate "${PACKAGE_NAME}@${VERSION#v}" "$deprecation_message" \
    --registry=https://registry.npmjs.org
  if ! delivered_deprecation="$(npm_deprecation "$VERSION")"; then
    err "could not verify npm deprecation for ${VERSION#v}"
  fi
  [ "$delivered_deprecation" = "$deprecation_message" ] ||
    err "npm did not retain the exact withdrawal deprecation for ${VERSION#v}"

  current_pointer="$(
    npm view "$PACKAGE_NAME" "dist-tags.${NPM_TAG}" --registry=https://registry.npmjs.org
  )"
  require_safe_channel_pointer "v$current_pointer" "$CHANNEL"
  if [ "$current_pointer" = "${VERSION#v}" ]; then
    npm dist-tag add "${PACKAGE_NAME}@${ROLLBACK_VERSION#v}" "$NPM_TAG" \
      --registry=https://registry.npmjs.org
  fi
  delivered_pointer="$(
    npm view "$PACKAGE_NAME" "dist-tags.${NPM_TAG}" --registry=https://registry.npmjs.org
  )"
  [ "$delivered_pointer" != "${VERSION#v}" ] ||
    err "npm $NPM_TAG still points to withdrawn ${VERSION#v}"
  require_safe_channel_pointer "v$delivered_pointer" "$CHANNEL"
  say "npm ${NPM_TAG} now resolves to v${delivered_pointer}; ${VERSION#v} is deprecated."
}

update_oss_channel() {
  local pointer_name="$1" current listing base
  current="$(read_oss_pointer "$pointer_name")"
  require_safe_channel_pointer "$current" "$CHANNEL"
  if [ "$current" = "$VERSION" ]; then
    write_oss_pointer "$pointer_name" "$ROLLBACK_VERSION"
    current="$ROLLBACK_VERSION"
  fi
  require_safe_channel_pointer "$current" "$CHANNEL"

  base="oss://${OSS_BUCKET}/${OSS_PREFIX}/download/${VERSION}/"
  "$OSSUTIL" rm -rf --endpoint "$OSS_ENDPOINT" "$base" >/dev/null
  listing="$("$OSSUTIL" ls --endpoint "$OSS_ENDPOINT" "$base")"
  if printf '%s\n' "$listing" | grep -Fq "${OSS_PREFIX}/download/${VERSION}/"; then
    err "OSS still exposes objects beneath ${OSS_PREFIX}/download/${VERSION}/"
  fi
  say "OSS ${pointer_name} resolves to $current; the withdrawn version prefix was removed."
}

gitee_release_id() {
  local lookup_version="$1" response status release_id
  local -a auth_args
  auth_args=()
  if [ -n "${GITEE_TOKEN:-}" ]; then
    auth_args=(-H "Authorization: token ${GITEE_TOKEN}")
  fi
  response="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-gitee-release.XXXXXX")"
  if ! status="$(
    curl -sS --connect-timeout 15 --max-time 60 \
      -o "$response" -w '%{http_code}' \
      "${auth_args[@]}" \
      "${GITEE_API}/repos/${GITEE_REPO}/releases/tags/${lookup_version}"
  )"; then
    rm -f "$response"
    printf 'could not query Gitee release %s\n' "$lookup_version" >&2
    return 1
  fi
  case "$status" in
    200)
      release_id="$(
        python3 -c 'import json,sys
d=json.load(sys.stdin)
value=d.get("id")
if isinstance(value, int) and value > 0:
    print(value)
' <"$response" 2>/dev/null || true
      )"
      rm -f "$response"
      [ -n "$release_id" ] || {
        printf 'Gitee returned invalid release metadata for %s\n' "$lookup_version" >&2
        return 1
      }
      printf '%s\n' "$release_id"
      ;;
    404)
      rm -f "$response"
      return 4
      ;;
    *)
      rm -f "$response"
      printf 'Gitee release lookup for %s returned HTTP %s\n' "$lookup_version" "$status" >&2
      return 1
      ;;
  esac
}

gitee_remote_tag_commit() {
  local version="${1:-$VERSION}" refs
  refs="$(
    git ls-remote "$GITEE_PUBLIC_GIT_REMOTE" \
      "refs/tags/${version}" "refs/tags/${version}^{}"
  )" || return 1
  printf '%s\n' "$refs" | awk -v tag="refs/tags/${version}" '
    $2 == tag "^{}" { peeled=$1 }
    $2 == tag { direct=$1 }
    END {
      if (peeled != "") print peeled
      else if (direct != "") print direct
    }
  '
}

ensure_gitee_rollback() {
  local dist rollback_commit rollback_remote_commit rollback_release_id release_id lookup_status
  WITHDRAW_GITEE_ROLLBACK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dws-withdraw-gitee-rollback.XXXXXX")"
  dist="$WITHDRAW_GITEE_ROLLBACK_DIR"
  rollback_release_id="$(release_field "$ROLLBACK_VERSION" id)"
  DWS_GITHUB_RELEASE_ID="$rollback_release_id" \
    "$GITHUB_DOWNLOAD_HELPER" "$ROLLBACK_VERSION" "$dist"
  DWS_PACKAGE_DIST_DIR="$dist" "$ARTIFACT_VERIFY_HELPER" "$ROLLBACK_VERSION"
  VERSION="$ROLLBACK_VERSION" \
    DIST_DIR="$dist" \
    GITEE_TOKEN="$GITEE_TOKEN" \
    GITEE_USER="$GITEE_USER" \
    GITEE_REPO="$GITEE_REPO" \
    GITEE_GIT_REMOTE="$GITEE_GIT_REMOTE" \
    GITEE_PUBLIC_GIT_REMOTE="$GITEE_PUBLIC_GIT_REMOTE" \
    DWS_REQUIRE_GITEE=1 \
    "$GITEE_SYNC_HELPER"
  rm -rf "$dist"
  WITHDRAW_GITEE_ROLLBACK_DIR=""

  rollback_commit="$(tag_commit "$ROLLBACK_VERSION")"
  rollback_remote_commit="$(gitee_remote_tag_commit "$ROLLBACK_VERSION")" ||
    err "could not verify Gitee rollback tag $ROLLBACK_VERSION"
  [ "$rollback_remote_commit" = "$rollback_commit" ] ||
    err "Gitee rollback tag $ROLLBACK_VERSION is not aligned to $rollback_commit"
  set +e
  release_id="$(gitee_release_id "$ROLLBACK_VERSION")"
  lookup_status=$?
  set -e
  [ "$lookup_status" -eq 0 ] && [ -n "$release_id" ] ||
    err "Gitee rollback release $ROLLBACK_VERSION is not available"
  say "Gitee rollback release $ROLLBACK_VERSION is complete before withdrawing $VERSION."
}

withdraw_gitee() {
  local remote_commit release_id lookup_status tmp askpass remaining_commit remaining_status
  remote_commit="$(gitee_remote_tag_commit)" ||
    err "could not inspect the Gitee tag before withdrawal"
  set +e
  release_id="$(gitee_release_id "$VERSION")"
  lookup_status=$?
  set -e
  case "$lookup_status" in
    0) ;;
    4) release_id="" ;;
    *) err "could not determine whether Gitee release $VERSION exists" ;;
  esac
  if [ -n "$remote_commit" ]; then
    [ "$remote_commit" = "$TARGET_COMMIT" ] ||
      err "Gitee tag $VERSION points to $remote_commit, not the GitHub release commit"
  fi
  if [ -n "$release_id" ]; then
    curl -fsS -X DELETE \
      -H "Authorization: token ${GITEE_TOKEN}" \
      "${GITEE_API}/repos/${GITEE_REPO}/releases/${release_id}" >/dev/null
    set +e
    gitee_release_id "$VERSION" >/dev/null
    lookup_status=$?
    set -e
    case "$lookup_status" in
      4) ;;
      0) err "Gitee release $VERSION still exists after deletion" ;;
      *) err "could not verify deletion of Gitee release $VERSION" ;;
    esac
  fi
  if [ -z "$remote_commit" ]; then
    say "Gitee release and tag $VERSION are already absent."
    return
  fi

  tmp="$(mktemp -d "${TMPDIR:-/tmp}/dws-withdraw-gitee.XXXXXX")"
  askpass="$tmp/askpass.sh"
  {
    printf '%s\n' '#!/bin/sh'
    # shellcheck disable=SC2016 # These variables belong to the generated script.
    printf '%s\n' 'case "$1" in'
    # shellcheck disable=SC2016 # These variables belong to the generated script.
    printf '%s\n' '  *Username*) printf "%s\n" "$DWS_GITEE_USER" ;;'
    # shellcheck disable=SC2016 # These variables belong to the generated script.
    printf '%s\n' '  *) printf "%s\n" "$DWS_GITEE_TOKEN" ;;'
    printf '%s\n' 'esac'
  } >"$askpass"
  chmod 700 "$askpass"
  DWS_GITEE_USER="$GITEE_USER" \
  DWS_GITEE_TOKEN="$GITEE_TOKEN" \
    GIT_ASKPASS="$askpass" \
    GIT_TERMINAL_PROMPT=0 \
    git push "$GITEE_GIT_REMOTE" ":refs/tags/${VERSION}" >/dev/null
  rm -rf "$tmp"
  set +e
  remaining_commit="$(gitee_remote_tag_commit)"
  remaining_status=$?
  set -e
  [ "$remaining_status" -eq 0 ] ||
    err "could not verify Gitee tag deletion for $VERSION"
  [ -z "$remaining_commit" ] ||
    err "Gitee tag $VERSION still exists after deletion"
  say "Removed the withdrawn release and tag from the Gitee distribution mirror."
}

checksum_for() {
  local file="$1" asset="$2"
  awk -v wanted="$asset" '
    $2 == wanted && $1 ~ /^[0-9a-f]{64}$/ { print $1 }
  ' "$file"
}

render_homebrew_rollback_formula() {
  local output="$1" checksum_file="$2" release_version="${3:-$ROLLBACK_VERSION}"
  local class_name description keg_only caveat semver base
  local darwin_amd64 darwin_arm64 linux_amd64 linux_arm64 skills
  semver="${release_version#v}"
  base="https://github.com/${OFFICIAL_REPOSITORY}/releases/download/${release_version}"
  darwin_amd64="$(checksum_for "$checksum_file" dws-darwin-amd64.tar.gz)"
  darwin_arm64="$(checksum_for "$checksum_file" dws-darwin-arm64.tar.gz)"
  linux_amd64="$(checksum_for "$checksum_file" dws-linux-amd64.tar.gz)"
  linux_arm64="$(checksum_for "$checksum_file" dws-linux-arm64.tar.gz)"
  skills="$(checksum_for "$checksum_file" dws-skills.zip)"
  for digest in "$darwin_amd64" "$darwin_arm64" "$linux_amd64" "$linux_arm64" "$skills"; do
    printf '%s\n' "$digest" | grep -Eq '^[0-9a-f]{64}$' ||
      err "rollback release checksums are incomplete"
  done

  class_name=DingtalkWorkspaceCli
  description='Automate DingTalk workspace tasks from the terminal'
  keg_only=''
  caveat=''
  if [ "$CHANNEL" = prerelease ]; then
    class_name=DingtalkWorkspaceCliBeta
    description="${description} (beta channel)"
    keg_only='  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"'
    # shellcheck disable=SC2016 # Preserve Homebrew's Ruby interpolation.
    caveat='      This beta is keg-only. Add #{opt_bin} to PATH to use its `dws` binary.'
  fi

  sed \
    -e "s|__CLASS_NAME__|$class_name|g" \
    -e "s|__DESCRIPTION__|$description|g" \
    -e "s|__VERSION__|$semver|g" \
    -e "s|__DARWIN_AMD64_URL__|$base/dws-darwin-amd64.tar.gz|g" \
    -e "s|__DARWIN_AMD64_SHA256__|$darwin_amd64|g" \
    -e "s|__DARWIN_ARM64_URL__|$base/dws-darwin-arm64.tar.gz|g" \
    -e "s|__DARWIN_ARM64_SHA256__|$darwin_arm64|g" \
    -e "s|__LINUX_AMD64_URL__|$base/dws-linux-amd64.tar.gz|g" \
    -e "s|__LINUX_AMD64_SHA256__|$linux_amd64|g" \
    -e "s|__LINUX_ARM64_URL__|$base/dws-linux-arm64.tar.gz|g" \
    -e "s|__LINUX_ARM64_SHA256__|$linux_arm64|g" \
    -e "s|__SKILLS_URL__|$base/dws-skills.zip|g" \
    -e "s|__SKILLS_SHA256__|$skills|g" \
    -e "s|__KEG_ONLY_LINE__|$keg_only|g" \
    -e "s|__CHANNEL_CAVEAT__|$caveat|g" \
    "$ROOT/build/homebrew-release.rb.tmpl" >"$output"
}

formula_version() {
  awk '$1 == "version" { gsub(/"/, "", $2); print "v" $2; exit }' "$1"
}

update_homebrew() {
  local formula_path source_path current desired checksum_dir checksum_file
  formula_path=Formula/dingtalk-workspace-cli.rb
  source_path="$(mktemp "${TMPDIR:-/tmp}/dws-withdraw-formula.XXXXXX")"
  if [ "$CHANNEL" = prerelease ]; then
    formula_path=Formula/dingtalk-workspace-cli-beta.rb
  fi
  [ -f "$ROOT/$formula_path" ] ||
    err "tracked Homebrew channel formula is missing: $formula_path"
  current="$(formula_version "$ROOT/$formula_path")"
  desired="$ROLLBACK_VERSION"
  if [ "$current" != "$VERSION" ]; then
    require_safe_channel_pointer "$current" "$CHANNEL"
    desired="$current"
  fi

  checksum_dir="$(mktemp -d "${TMPDIR:-/tmp}/dws-withdraw-checksums.XXXXXX")"
  gh release download "$desired" \
    --repo "$OFFICIAL_REPOSITORY" \
    --pattern checksums.txt \
    --dir "$checksum_dir"
  checksum_file="$checksum_dir/checksums.txt"
  [ -s "$checksum_file" ] || err "rollback release checksums.txt is missing"
  render_homebrew_rollback_formula "$source_path" "$checksum_file" "$desired"
  ruby -c "$source_path" >/dev/null || err "rendered rollback Homebrew formula is invalid"

  if [ "$current" != "$VERSION" ]; then
    cmp -s "$ROOT/$formula_path" "$source_path" ||
      err "tracked Homebrew formula for $current does not exactly match its published checksums"
    rm -rf "$checksum_dir"
    rm -f "$source_path"
    say "Homebrew ${formula_path} is fully verified at $current and avoids $VERSION."
    return
  fi

  DWS_FORMULA_SOURCE="$source_path" \
    DWS_TAP_FORMULA_PATH="$formula_path" \
    DWS_TAP_REPO_URL="${DWS_TAP_REPO_URL:-https://github.com/${OFFICIAL_REPOSITORY}.git}" \
    DWS_TAP_GITHUB_TOKEN="$HOMEBREW_PR_TOKEN" \
    DWS_TAP_PR_REPOSITORY="$OFFICIAL_REPOSITORY" \
    DWS_TAP_PR_BRANCH="automation/withdraw-${VERSION//./-}" \
    DWS_TAP_PR_TITLE="revert: withdraw ${VERSION} and restore ${ROLLBACK_VERSION}" \
  DWS_TAP_COMMIT_MESSAGE="revert: restore formula to ${ROLLBACK_VERSION}" \
    "$SCRIPT_DIR/publish-homebrew-formula.sh"
  rm -rf "$checksum_dir"
  rm -f "$source_path"
  HOMEBREW_ROLLBACK_PENDING=true
  say "Homebrew rollback PR requires independent review and merge; this run will finish the other channel withdrawals and then remain failed."
}

delete_github_release_and_tag() {
  local latest_before latest_after reason_sha
  latest_before="$(
    github_api "repos/${OFFICIAL_REPOSITORY}/releases/latest" --jq .tag_name
  )"

  github_delete_if_present \
    "repos/${OFFICIAL_REPOSITORY}/releases/${TARGET_RELEASE_ID}" \
    "GitHub Release $VERSION"
  github_expect_404 "repos/${OFFICIAL_REPOSITORY}/releases/tags/${VERSION}"

  github_delete_if_present \
    "repos/${OFFICIAL_REPOSITORY}/git/refs/tags/${VERSION}" \
    "GitHub tag $VERSION"
  github_expect_404 "repos/${OFFICIAL_REPOSITORY}/git/ref/tags/${VERSION}"

  reason_sha="$(printf '%s' "$REASON" | sha256_text)"
  verify_existing_tombstone "$reason_sha" ||
    err "permanent tombstone $TOMBSTONE disappeared during final GitHub cleanup"

  latest_after="$(
    github_api "repos/${OFFICIAL_REPOSITORY}/releases/latest" --jq .tag_name
  )"
  if [ "$CHANNEL" = stable ] && [ "$latest_before" = "$VERSION" ]; then
    [ "$latest_after" = "$ROLLBACK_VERSION" ] ||
      err "GitHub latest resolved to $latest_after, expected rollback $ROLLBACK_VERSION"
  else
    [ "$latest_after" = "$latest_before" ] ||
      err "GitHub latest changed unexpectedly from $latest_before to $latest_after"
    release_is_eligible_rollback "$latest_after" stable ||
      err "GitHub latest does not resolve to a safe stable release: $latest_after"
  fi
  say "Deleted GitHub Release and original tag $VERSION after package and mirror rollback; permanent tombstone $TOMBSTONE remains."
}

cleanup() {
  if [ -n "${WITHDRAW_OSSUTIL_DIR:-}" ]; then
    rm -rf "$WITHDRAW_OSSUTIL_DIR"
  fi
  if [ -n "${WITHDRAW_OSS_ROLLBACK_DIR:-}" ]; then
    rm -rf "$WITHDRAW_OSS_ROLLBACK_DIR"
  fi
  if [ -n "${WITHDRAW_GITEE_ROLLBACK_DIR:-}" ]; then
    rm -rf "$WITHDRAW_GITEE_ROLLBACK_DIR"
  fi
}
trap cleanup EXIT HUP INT TERM

validate_inputs
for command in git gh npm curl python3 awk sed grep sort unzip ruby cmp cut; do
  need_cmd "$command"
done
need_env GITHUB_TOKEN "${GITHUB_TOKEN:-}"
need_env NODE_AUTH_TOKEN "${NODE_AUTH_TOKEN:-}"
need_env OSS_ACCESS_KEY_ID "${OSS_ACCESS_KEY_ID:-}"
need_env OSS_ACCESS_KEY_SECRET "${OSS_ACCESS_KEY_SECRET:-}"
need_env OSS_ENDPOINT "${OSS_ENDPOINT:-}"
need_env OSS_BUCKET "${OSS_BUCKET:-}"
case "$OSS_PREFIX" in
  ''|/*|*'..'*|*'//'*) err "OSS_PREFIX must be a non-empty safe relative prefix" ;;
  *[!A-Za-z0-9._/-]*) err "OSS_PREFIX contains unsupported characters" ;;
esac

validate_context
git fetch --force --tags origin

CHANNEL="$(release_channel_for_version "$VERSION")"
TARGET_TAG_OBJECT=""
TARGET_COMMIT=""
TARGET_RELEASE_ID=""
TARGET_TAG_EXISTS=false
TARGET_RELEASE_EXISTS=false

set +e
remote_tag_object="$(
  github_optional \
    "repos/${OFFICIAL_REPOSITORY}/git/ref/tags/${VERSION}" \
    --jq .object.sha
)"
remote_tag_status=$?
set -e
case "$remote_tag_status" in
  0)
    TARGET_TAG_EXISTS=true
    TARGET_TAG_OBJECT="$(tag_object "$VERSION" || true)"
    TARGET_COMMIT="$(tag_commit "$VERSION" || true)"
    [ "$TARGET_TAG_OBJECT" = "$remote_tag_object" ] ||
      err "local $VERSION tag object differs from the authoritative GitHub ref"
    [ "$(git cat-file -t "$TARGET_TAG_OBJECT" 2>/dev/null || true)" = tag ] ||
      err "$VERSION must be an annotated GitHub release tag"
    printf '%s\n' "$TARGET_COMMIT" | grep -Eq '^[0-9a-f]{40}$' ||
      err "could not resolve the release commit for $VERSION"
    ;;
  4) ;;
  *) err "could not inspect the authoritative GitHub tag $VERSION" ;;
esac

set +e
target_json="$(
  github_optional "repos/${OFFICIAL_REPOSITORY}/releases/tags/$VERSION"
)"
target_release_status=$?
set -e
case "$target_release_status" in
  0)
    TARGET_RELEASE_EXISTS=true
    target_state="$(
      printf '%s' "$target_json" |
        python3 -c 'import json,sys
d=json.load(sys.stdin)
print("%s\t%s\t%s\t%s\t%s" % (
    d.get("tag_name", ""),
    str(bool(d.get("draft"))).lower(),
    str(bool(d.get("prerelease"))).lower(),
    str(bool(d.get("immutable"))).lower(),
    d.get("id", ""),
))'
    )"
    expected_state="$(printf '%s\tfalse\t%s\ttrue' "$VERSION" "$(expected_prerelease "$CHANNEL")")"
    case "$target_state" in
      "$expected_state"$'\t'*) ;;
      *) err "$VERSION is not the exact public immutable GitHub Release for its channel" ;;
    esac
    TARGET_RELEASE_ID="${target_state##*$'\t'}"
    printf '%s\n' "$TARGET_RELEASE_ID" | grep -Eq '^[1-9][0-9]*$' ||
      err "GitHub Release has an invalid database ID"
    ;;
  4) ;;
  *) err "could not inspect the authoritative GitHub Release $VERSION" ;;
esac

if [ "$TARGET_TAG_EXISTS" != true ] || [ "$TARGET_RELEASE_EXISTS" != true ]; then
  reason_sha="$(printf '%s' "$REASON" | sha256_text)"
  verify_existing_tombstone "$reason_sha" ||
    err "$VERSION is partially absent without the exact permanent withdrawal tombstone"
  say "Resuming withdrawal for $VERSION from exact permanent tombstone metadata."
fi

[ "$(npm_exact_version "$VERSION" || true)" = "${VERSION#v}" ] ||
  err "npm exact version ${PACKAGE_NAME}@${VERSION#v} is not published"
if [ "$TARGET_TAG_EXISTS" = true ] && [ "$TARGET_RELEASE_EXISTS" = true ]; then
  DWS_RELEASE_OFFICIAL_REPOSITORY="$OFFICIAL_REPOSITORY" \
    DWS_RELEASE_GITHUB_TOKEN="$GITHUB_TOKEN" \
    "$DELIVERY_VERIFIER" "$VERSION" "$TARGET_COMMIT" >/dev/null ||
    err "$VERSION has no complete Release workflow delivery proof"
fi

ROLLBACK_VERSION="$(find_rollback_version "$CHANNEL" || true)"
[ -n "$ROLLBACK_VERSION" ] ||
  err "no earlier delivered, non-withdrawn $CHANNEL release is available for rollback"
say "Withdrawal plan: $VERSION ($CHANNEL) -> $ROLLBACK_VERSION"

if [ "$CHANNEL" = stable ]; then
  NPM_TAG=latest
else
  NPM_TAG=beta
fi
NPM_POINTER_BEFORE="$(
  npm view "$PACKAGE_NAME" "dist-tags.${NPM_TAG}" --registry=https://registry.npmjs.org
)"
[ -n "$NPM_POINTER_BEFORE" ] || err "npm $NPM_TAG is empty"
require_safe_channel_pointer "v$NPM_POINTER_BEFORE" "$CHANNEL"

ensure_ossutil
if [ -z "${OSS_REGION:-}" ]; then
  OSS_REGION="$(
    printf '%s' "$OSS_ENDPOINT" |
      sed -n 's#^\(https\{0,1\}://\)\{0,1\}oss-\([a-z0-9-]*[a-z0-9]\)\.aliyuncs\.com.*#\2#p' |
      sed 's#-internal$##'
  )"
fi
[ -n "$OSS_REGION" ] || err "could not derive OSS_REGION; set it explicitly"
export OSS_REGION
if [ "$CHANNEL" = stable ]; then
  OSS_POINTER_NAME=latest.txt
else
  OSS_POINTER_NAME=beta.txt
fi
OSS_POINTER_BEFORE="$(read_oss_pointer "$OSS_POINTER_NAME")"
require_safe_channel_pointer "$OSS_POINTER_BEFORE" "$CHANNEL"

GITEE_ENABLED="${DWS_GITEE_ENABLED:-false}"
case "$GITEE_ENABLED" in
  true)
    need_env GITEE_TOKEN "${GITEE_TOKEN:-}"
    need_env GITEE_USER "${GITEE_USER:-}"
    need_env GITEE_REPO "$GITEE_REPO"
    GITEE_GIT_REMOTE="${GITEE_GIT_REMOTE:-https://gitee.com/${GITEE_REPO}.git}"
    GITEE_PUBLIC_GIT_REMOTE="${GITEE_PUBLIC_GIT_REMOTE:-https://gitee.com/${GITEE_REPO}.git}"
    ;;
  false)
    GITEE_PUBLIC_GIT_REMOTE="${GITEE_PUBLIC_GIT_REMOTE:-https://gitee.com/${GITEE_REPO}.git}"
    if ! disabled_gitee_commit="$(gitee_remote_tag_commit)"; then
      err "could not prove that disabled Gitee distribution does not expose $VERSION"
    fi
    if [ -n "$disabled_gitee_commit" ]; then
      err "Gitee still exposes $VERSION but automated mirror withdrawal is disabled"
    fi
    set +e
    disabled_gitee_release_id="$(gitee_release_id "$VERSION")"
    disabled_gitee_release_status=$?
    set -e
    case "$disabled_gitee_release_status" in
      4) ;;
      0)
        err "Gitee Release $VERSION still exists (id $disabled_gitee_release_id) but automated mirror withdrawal is disabled"
        ;;
      *) err "could not prove that disabled Gitee distribution has no Release for $VERSION" ;;
    esac
    ;;
  *) err "DWS_GITEE_ENABLED must be true or false" ;;
esac

need_env HOMEBREW_PR_TOKEN "${HOMEBREW_PR_TOKEN:-}"
[ "$HOMEBREW_PR_TOKEN" != "$GITHUB_TOKEN" ] ||
  err "HOMEBREW_PR_TOKEN must be a separate least-privilege identity"

HOMEBREW_ROLLBACK_PENDING=false
create_tombstone
update_homebrew
update_github_release
update_npm_channel
ensure_oss_rollback
update_oss_channel "$OSS_POINTER_NAME"
if [ "$GITEE_ENABLED" = true ]; then
  ensure_gitee_rollback
  withdraw_gitee
else
  say "Gitee did not contain $VERSION; no mirror mutation was needed."
fi
delete_github_release_and_tag

if [ "$HOMEBREW_ROLLBACK_PENDING" = true ]; then
  err "Homebrew rollback PR requires independent review and merge; rerun this workflow after merge"
fi

say "Withdrawal completed for all configured distribution channels."
say "Already-installed clients cannot be remotely downgraded; users must install $ROLLBACK_VERSION or a later fixed version."
