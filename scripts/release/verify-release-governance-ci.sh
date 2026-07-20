#!/bin/sh
set -eu

REPOSITORY="${1:-}"
COMMIT="${2:-}"
WORKFLOW="release.yml"
DEFAULT_BRANCH="main"
TIMEOUT_SECONDS="${DWS_RELEASE_GOVERNANCE_TIMEOUT_SECONDS:-180}"
POLL_SECONDS="${DWS_RELEASE_GOVERNANCE_POLL_SECONDS:-2}"

usage() {
  printf '%s\n' 'usage: verify-release-governance-ci.sh <owner/repository> <commit-sha>' >&2
}

[ -n "$REPOSITORY" ] && [ -n "$COMMIT" ] || { usage; exit 2; }
printf '%s\n' "$COMMIT" | grep -Eq '^[0-9a-f]{40}$' || {
  printf 'invalid release commit: %s\n' "$COMMIT" >&2
  exit 2
}
printf '%s\n' "$TIMEOUT_SECONDS" | grep -Eq '^[1-9][0-9]*$' || {
  printf 'invalid governance timeout: %s\n' "$TIMEOUT_SECONDS" >&2
  exit 2
}
printf '%s\n' "$POLL_SECONDS" | grep -Eq '^[1-9][0-9]*$' || {
  printf 'invalid governance poll interval: %s\n' "$POLL_SECONDS" >&2
  exit 2
}
command -v gh >/dev/null 2>&1 || {
  printf '%s\n' 'gh is required to run the Release governance preflight' >&2
  exit 1
}

nonce="${COMMIT}-$(date +%s)-$$"
expected_title="Release governance preflight $nonce"

printf '==> Dispatching Release governance preflight for %s\n' "$COMMIT"
gh workflow run "$WORKFLOW" \
  --repo "$REPOSITORY" \
  --ref "$DEFAULT_BRANCH" \
  -f "governance_preflight_commit=$COMMIT" \
  -f "governance_preflight_nonce=$nonce"

started_at="$(date +%s)"
run_id=""
while :; do
  # nonce is restricted to a commit SHA, decimal timestamp, PID, and hyphens,
  # so it is safe to embed in this jq string literal.
  run_id="$(
    gh api \
      -H 'Accept: application/vnd.github+json' \
      "repos/$REPOSITORY/actions/workflows/$WORKFLOW/runs?event=workflow_dispatch&branch=$DEFAULT_BRANCH&per_page=100" \
      --jq ".workflow_runs[] | select(.display_title == \"$expected_title\" and .head_sha == \"$COMMIT\") | .id" \
      | sed -n '1p'
  )" || {
    printf '%s\n' 'could not query the dispatched Release governance preflight' >&2
    exit 1
  }
  [ -z "$run_id" ] || break

  now="$(date +%s)"
  if [ $((now - started_at)) -ge "$TIMEOUT_SECONDS" ]; then
    printf 'timed out waiting for Release governance preflight: %s\n' "$expected_title" >&2
    exit 1
  fi
  sleep "$POLL_SECONDS"
done

run_state=""
while :; do
  run_state="$(
    gh api \
      -H 'Accept: application/vnd.github+json' \
      "repos/$REPOSITORY/actions/runs/$run_id" \
      --jq '[.display_title, .event, .status, .conclusion, .head_branch, .head_sha, .path, .repository.full_name] | @tsv'
  )" || {
    printf 'could not verify Release governance preflight run %s\n' "$run_id" >&2
    exit 1
  }
  run_status="$(printf '%s\n' "$run_state" | cut -f3)"
  [ "$run_status" != "completed" ] || break
  now="$(date +%s)"
  if [ $((now - started_at)) -ge "$TIMEOUT_SECONDS" ]; then
    printf 'timed out waiting for Release governance preflight run %s\n' "$run_id" >&2
    exit 1
  fi
  sleep "$POLL_SECONDS"
done

expected_state="$(printf '%s\tworkflow_dispatch\tcompleted\tsuccess\t%s\t%s\t.github/workflows/release.yml\t%s' \
  "$expected_title" "$DEFAULT_BRANCH" "$COMMIT" "$REPOSITORY")"
[ "$run_state" = "$expected_state" ] || {
  printf 'Release governance preflight run identity mismatch:\n  expected: %s\n  actual:   %s\n' \
    "$expected_state" "$run_state" >&2
  gh run view "$run_id" --repo "$REPOSITORY" --log-failed >&2 || true
  exit 1
}

printf 'Release governance preflight passed: https://github.com/%s/actions/runs/%s\n' \
  "$REPOSITORY" "$run_id"
