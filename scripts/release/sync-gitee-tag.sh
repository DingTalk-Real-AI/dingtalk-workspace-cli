#!/usr/bin/env bash
# Synchronize one immutable release tag to Gitee without moving an existing tag.

set -euo pipefail

VERSION="${VERSION:-}"
GITEE_REPO="${GITEE_REPO:-}"
GITEE_GIT_TIMEOUT_SECONDS="${GITEE_GIT_TIMEOUT_SECONDS:-180}"
GITEE_TAG_TIMEOUT_SECONDS="${GITEE_TAG_TIMEOUT_SECONDS:-300}"
GITEE_TAG_VERIFY_ATTEMPTS="${GITEE_TAG_VERIFY_ATTEMPTS:-12}"
GITEE_TAG_VERIFY_DELAY="${GITEE_TAG_VERIFY_DELAY:-5}"
GITEE_SOURCE_REMOTE="${GITEE_SOURCE_REMOTE-origin}"
GITEE_PARENT_DEADLINE_EPOCH="${GITEE_PARENT_DEADLINE_EPOCH:-}"

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
  GITEE_GIT_TIMEOUT_SECONDS \
  GITEE_TAG_TIMEOUT_SECONDS \
  GITEE_TAG_VERIFY_ATTEMPTS; do
  require_positive_integer "$setting" "${!setting}"
done
require_nonnegative_integer GITEE_TAG_VERIFY_DELAY "$GITEE_TAG_VERIFY_DELAY"
if [ -n "$GITEE_PARENT_DEADLINE_EPOCH" ]; then
  require_positive_integer GITEE_PARENT_DEADLINE_EPOCH "$GITEE_PARENT_DEADLINE_EPOCH"
fi

[ -n "$VERSION" ] || err "VERSION is required"
case "$VERSION" in
  v*) ;;
  *) err "VERSION must be a v-prefixed release tag: ${VERSION}" ;;
esac

if [ -z "${GITEE_GIT_REMOTE:-}" ]; then
  [ -n "${GITEE_USER:-}" ] || err "GITEE_USER is required"
  [ -n "${GITEE_TOKEN:-}" ] || err "GITEE_TOKEN is required"
  [ -n "$GITEE_REPO" ] || err "GITEE_REPO is required"
  GITEE_GIT_REMOTE="https://${GITEE_USER}:${GITEE_TOKEN}@gitee.com/${GITEE_REPO}.git"
fi
if [ -z "${GITEE_PUBLIC_GIT_REMOTE:-}" ]; then
  [ -n "$GITEE_REPO" ] || err "GITEE_REPO is required"
  GITEE_PUBLIC_GIT_REMOTE="https://gitee.com/${GITEE_REPO}.git"
fi

now_seconds() {
  date +%s
}

tag_deadline=$(( $(now_seconds) + GITEE_TAG_TIMEOUT_SECONDS ))
if [ -n "$GITEE_PARENT_DEADLINE_EPOCH" ] && [ "$GITEE_PARENT_DEADLINE_EPOCH" -lt "$tag_deadline" ]; then
  tag_deadline="$GITEE_PARENT_DEADLINE_EPOCH"
fi

deadline_remaining() {
  local remaining=$(( tag_deadline - $(now_seconds) ))
  [ "$remaining" -gt 0 ] || return 1
  printf '%s\n' "$remaining"
}

bounded_git_timeout() {
  local remaining
  remaining="$(deadline_remaining)" || return 1
  if [ "$GITEE_GIT_TIMEOUT_SECONDS" -lt "$remaining" ]; then
    printf '%s\n' "$GITEE_GIT_TIMEOUT_SECONDS"
  else
    printf '%s\n' "$remaining"
  fi
}

run_python_bounded() {
  local seconds="$1"
  shift
  python3 - "$seconds" "$@" <<'PY'
import os
import signal
import subprocess
import sys

timeout = int(sys.argv[1])
process = subprocess.Popen(sys.argv[2:], start_new_session=True)
try:
    returncode = process.wait(timeout=timeout)
except subprocess.TimeoutExpired:
    os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()
    sys.exit(124)
sys.exit(returncode)
PY
}

run_bounded() {
  local seconds
  seconds="$(bounded_git_timeout)" || {
    printf 'error: Gitee tag synchronization deadline exhausted\n' >&2
    return 124
  }
  if command -v timeout >/dev/null 2>&1; then
    timeout --signal=TERM "${seconds}s" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout --signal=TERM "${seconds}s" "$@"
  elif command -v python3 >/dev/null 2>&1; then
    run_python_bounded "$seconds" "$@"
  else
    printf 'error: timeout, gtimeout, or python3 is required for bounded git operations\n' >&2
    return 127
  fi
}

sleep_within_deadline() {
  local seconds="$1" remaining
  [ "$seconds" -gt 0 ] || return 0
  remaining="$(deadline_remaining)" || return 1
  [ "$seconds" -lt "$remaining" ] || return 1
  sleep "$seconds"
}

# A tag checkout already has the local ref. The fetch repairs shallow/manual
# checkouts, while the rev-parse below remains the authoritative requirement.
fetch_failed=0
if [ -n "$GITEE_SOURCE_REMOTE" ]; then
  run_bounded git fetch --force --tags "$GITEE_SOURCE_REMOTE" \
    "refs/tags/${VERSION}:refs/tags/${VERSION}" >/dev/null 2>&1 || fetch_failed=1
fi
target_commit="$(git rev-parse --verify "${VERSION}^{commit}" 2>/dev/null || true)"
if [ -z "$target_commit" ]; then
  if [ "$fetch_failed" -eq 1 ]; then
    err "source tag fetch failed and local release tag ${VERSION} could not be resolved"
  fi
  err "could not resolve local release tag ${VERSION}"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

remote_tag_commit() {
  local refs_file="${tmp_dir}/remote-refs"
  if ! run_bounded git ls-remote "$GITEE_PUBLIC_GIT_REMOTE" \
    "refs/tags/${VERSION}" "refs/tags/${VERSION}^{}" >"$refs_file"; then
    printf 'error: could not query Gitee tag %s\n' "$VERSION" >&2
    return 1
  fi
  awk -v tag="refs/tags/${VERSION}" '
    $2 == tag "^{}" { peeled=$1 }
    $2 == tag { direct=$1 }
    END {
      if (peeled != "") print peeled
      else if (direct != "") print direct
    }
  ' "$refs_file"
}

if ! remote_commit="$(remote_tag_commit)"; then
  exit 1
fi
if [ "$remote_commit" = "$target_commit" ]; then
  echo "   Gitee tag ${VERSION} already aligned at ${target_commit} — skip push."
  exit 0
fi
if [ -n "$remote_commit" ]; then
  err "Gitee tag ${VERSION} already points to ${remote_commit}; refusing to move it to ${target_commit}"
fi

echo "   Pushing missing Gitee tag ${VERSION} -> ${target_commit}"
if ! run_bounded git push "$GITEE_GIT_REMOTE" \
  "refs/tags/${VERSION}:refs/tags/${VERSION}" >/dev/null; then
  # A concurrent mirror may have created the same immutable tag while the push
  # was in flight. Accept only the exact expected commit.
  if remote_commit="$(remote_tag_commit 2>/dev/null)" && [ "$remote_commit" = "$target_commit" ]; then
    echo "   Gitee tag ${VERSION} became aligned concurrently — continue."
    exit 0
  fi
  err "failed to push missing Gitee tag ${VERSION}"
fi

attempt=1
remote_commit=""
while [ "$attempt" -le "$GITEE_TAG_VERIFY_ATTEMPTS" ]; do
  if remote_commit="$(remote_tag_commit 2>/dev/null)" && [ "$remote_commit" = "$target_commit" ]; then
    echo "   Gitee tag ${VERSION} is aligned."
    exit 0
  fi
  attempt=$((attempt + 1))
  if [ "$attempt" -le "$GITEE_TAG_VERIFY_ATTEMPTS" ]; then
    sleep_within_deadline "$GITEE_TAG_VERIFY_DELAY" || break
  fi
done

err "Gitee tag ${VERSION} is not aligned after push: got ${remote_commit:-<missing>}, want ${target_commit}"
