#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"

TAG="${1:-}"
EXPECTED_COMMIT="${2:-}"
REPOSITORY="${DWS_RELEASE_OFFICIAL_REPOSITORY:-DingTalk-Real-AI/dingtalk-workspace-cli}"

[ -n "$TAG" ] && [ -n "$EXPECTED_COMMIT" ] || {
  printf 'usage: verify-release-workflow-delivery.sh <tag> <commit>\n' >&2
  exit 2
}
if ! release_is_stable_version "$TAG" && ! release_is_prerelease_version "$TAG"; then
  printf 'invalid delivered release tag: %s\n' "$TAG" >&2
  exit 2
fi
printf '%s\n' "$EXPECTED_COMMIT" | grep -Eq '^[0-9a-f]{40}$' || {
  printf 'invalid delivered release commit: %s\n' "$EXPECTED_COMMIT" >&2
  exit 2
}
command -v curl >/dev/null 2>&1 || { printf '%s\n' 'curl is required to verify release delivery' >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { printf '%s\n' 'python3 is required to verify release delivery' >&2; exit 1; }

API_TOKEN="${DWS_RELEASE_GITHUB_TOKEN:-}"
if [ -z "$API_TOKEN" ] && [ "${GITHUB_ACTIONS:-false}" != "true" ] && command -v gh >/dev/null 2>&1; then
  API_TOKEN="$(gh auth token 2>/dev/null || true)"
fi
if [ -z "$API_TOKEN" ]; then API_TOKEN="${GITHUB_TOKEN:-}"; fi

github_get() {
  if [ -n "$API_TOKEN" ]; then
    curl -fsSL \
      -H 'Accept: application/vnd.github+json' \
      -H 'X-GitHub-Api-Version: 2026-03-10' \
      -H "Authorization: Bearer $API_TOKEN" \
      "https://api.github.com/$1" 2>/dev/null && return 0
  fi
  curl -fsSL \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2026-03-10' \
    "https://api.github.com/$1"
}

find_push_delivery() {
  page=1
  while :; do
    page_result="$(
      github_get "repos/$REPOSITORY/actions/workflows/release.yml/runs?branch=$TAG&event=push&status=completed&per_page=100&page=$page" \
        | python3 -c 'import json,sys
tag,commit=sys.argv[1:]
runs=json.load(sys.stdin).get("workflow_runs", [])
print(len(runs))
for run in runs:
    if run.get("head_sha") == commit and run.get("head_branch") == tag and run.get("conclusion") == "success":
        print(run.get("id", ""))
        break' "$TAG" "$EXPECTED_COMMIT"
    )" || return 1
    page_count="$(printf '%s\n' "$page_result" | sed -n '1p')"
    page_match="$(printf '%s\n' "$page_result" | sed -n '2p')"
    if [ -n "$page_match" ]; then printf '%s\n' "$page_match"; return 0; fi
    [ "$page_count" -eq 100 ] || return 1
    page=$((page + 1))
  done
}

push_delivery="$(find_push_delivery || true)"
if [ -n "$push_delivery" ]; then
  printf 'Release workflow delivery verified through exact-tag push run %s: %s -> %s\n' \
    "$push_delivery" "$TAG" "$EXPECTED_COMMIT"
  exit 0
fi

find_recovery_identity() {
  page=1
  while :; do
    page_result="$(
      github_get "repos/$REPOSITORY/actions/workflows/release.yml/runs?branch=main&event=workflow_dispatch&status=completed&per_page=100&page=$page" \
        | python3 -c 'import json,sys
tag,commit,repository=sys.argv[1:]
title=f"Release recovery {tag} at {commit}"
runs=json.load(sys.stdin).get("workflow_runs", [])
print(len(runs))
for run in runs:
    display=run.get("display_title", "")
    nonce=display[len(title) + 1:] if display.startswith(title + " ") else ""
    if (__import__("re").fullmatch(__import__("re").escape(commit) + r"-[0-9]+-[0-9]+", nonce)
            and run.get("event") == "workflow_dispatch"
            and run.get("status") == "completed" and run.get("conclusion") == "success"
            and run.get("head_branch") == "main" and run.get("path") == ".github/workflows/release.yml"
            and run.get("repository", {}).get("full_name") == repository):
        print("%s\t%s" % (run.get("id", ""), run.get("head_sha", "")))
        break' "$TAG" "$EXPECTED_COMMIT" "$REPOSITORY"
    )" || return 1
    page_count="$(printf '%s\n' "$page_result" | sed -n '1p')"
    page_match="$(printf '%s\n' "$page_result" | sed -n '2p')"
    if [ -n "$page_match" ]; then printf '%s\n' "$page_match"; return 0; fi
    [ "$page_count" -eq 100 ] || return 1
    page=$((page + 1))
  done
}

recovery_identity="$(find_recovery_identity || true)"
[ -n "$recovery_identity" ] || {
  printf 'Release workflow did not deliver %s at %s through a tag push or protected recovery\n' \
    "$TAG" "$EXPECTED_COMMIT" >&2
  exit 1
}
recovery_run_id="$(printf '%s\n' "$recovery_identity" | cut -f1)"
recovery_workflow_sha="$(printf '%s\n' "$recovery_identity" | cut -f2)"

workflow_status="$({
  github_get "repos/$REPOSITORY/compare/$recovery_workflow_sha...main" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status", ""))'
} || true)"
case "$workflow_status" in ahead|identical) ;; *)
  printf 'protected recovery workflow %s is not contained in current main\n' \
    "$recovery_workflow_sha" >&2
  exit 1
esac

passed_jobs=""
page=1
while :; do
  page_result="$(
    github_get "repos/$REPOSITORY/actions/runs/$recovery_run_id/jobs?filter=all&per_page=100&page=$page" \
      | python3 -c 'import json,sys
workflow_sha=sys.argv[1]
jobs=json.load(sys.stdin).get("jobs", [])
print(len(jobs))
for job in jobs:
    if (job.get("status") == "completed" and job.get("conclusion") == "success"
            and job.get("head_sha") == workflow_sha):
        print(job.get("name", ""))' "$recovery_workflow_sha"
  )" || exit 1
  page_count="$(printf '%s\n' "$page_result" | sed -n '1p')"
  page_jobs="$(printf '%s\n' "$page_result" | sed '1d')"
  passed_jobs="$(printf '%s\n%s\n' "$passed_jobs" "$page_jobs" | sed '/^$/d')"
  [ "$page_count" -eq 100 ] || break
  page=$((page + 1))
done
for required_job in \
  "Build signed release artifacts" \
  "Verify Apple Developer ID signatures" \
  "Publish immutable GitHub Release" \
  "Publish npm and mirrors"; do
  printf '%s\n' "$passed_jobs" | grep -Fqx "$required_job" || {
  printf 'protected recovery run %s did not complete the shared release job graph for %s\n' \
    "$recovery_run_id" "$TAG" >&2
  exit 1
  }
done

printf 'Release workflow delivery verified through protected recovery run %s: %s -> %s\n' \
  "$recovery_run_id" "$TAG" "$EXPECTED_COMMIT"
