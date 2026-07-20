#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"

MODE="strict"
CHANNEL_REPAIR_TARGET=""
if [ "${1:-}" = "--channel-repair" ]; then
  MODE="channel-repair"
  shift
  CHANNEL_REPAIR_TARGET="${1:-}"
  shift
  case "$CHANNEL_REPAIR_TARGET" in
    oss|gitee) ;;
    *)
      printf 'channel repair target must be oss or gitee\n' >&2
      exit 2
      ;;
  esac
fi

TAG="${1:-}"
EXPECTED_COMMIT="${2:-}"
REPOSITORY="${DWS_RELEASE_OFFICIAL_REPOSITORY:-DingTalk-Real-AI/dingtalk-workspace-cli}"

[ -n "$TAG" ] && [ -n "$EXPECTED_COMMIT" ] || {
  printf 'usage: verify-release-workflow-delivery.sh [--channel-repair <oss|gitee>] <tag> <commit>\n' >&2
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

find_failed_push_delivery() {
  matches=""
  page=1
  while :; do
    page_result="$(
      github_get "repos/$REPOSITORY/actions/workflows/release.yml/runs?branch=$TAG&event=push&status=completed&per_page=100&page=$page" \
        | python3 -c 'import json,sys
tag,commit,repository=sys.argv[1:]
runs=json.load(sys.stdin).get("workflow_runs", [])
print(len(runs))
for run in runs:
    if (run.get("head_sha") == commit and run.get("head_branch") == tag
            and run.get("event") == "push" and run.get("status") == "completed"
            and run.get("conclusion") == "failure"
            and run.get("path") == ".github/workflows/release.yml"
            and run.get("repository", {}).get("full_name") == repository):
        run_id=run.get("id", "")
        attempt=run.get("run_attempt", "")
        if isinstance(run_id, int) and run_id > 0 and isinstance(attempt, int) and attempt > 0:
            print(f"{run_id}\t{attempt}")' "$TAG" "$EXPECTED_COMMIT" "$REPOSITORY"
    )" || return 1
    page_count="$(printf '%s\n' "$page_result" | sed -n '1p')"
    page_matches="$(printf '%s\n' "$page_result" | sed '1d')"
    matches="$(printf '%s\n%s\n' "$matches" "$page_matches" | sed '/^$/d')"
    [ "$page_count" -eq 100 ] || break
    page=$((page + 1))
  done
  match_count="$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')"
  [ "$match_count" -eq 1 ] || {
    printf 'expected exactly one failed exact-tag push run for channel repair, found %s\n' \
      "$match_count" >&2
    return 1
  }
  printf '%s\n' "$matches"
}

verify_channel_repair_delivery() {
  run_id="$1"
  run_attempt="$2"
  jobs_dir="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-channel-jobs.XXXXXX")"
  page=1
  while :; do
    jobs_page="$jobs_dir/jobs-$page.json"
    if ! github_get "repos/$REPOSITORY/actions/runs/$run_id/attempts/$run_attempt/jobs?per_page=100&page=$page" \
      >"$jobs_page"; then
      rm -rf "$jobs_dir"
      return 1
    fi
    if ! page_count="$(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1])).get("jobs", [])))' "$jobs_page")"; then
      rm -rf "$jobs_dir"
      return 1
    fi
    [ "$page_count" -eq 100 ] || break
    page=$((page + 1))
  done

  result=0
  python3 - "$EXPECTED_COMMIT" "$run_id" "$run_attempt" "$TAG" "$CHANNEL_REPAIR_TARGET" "$jobs_dir"/jobs-*.json <<'PY' || result=$?
import json
import sys

commit, run_id, run_attempt, tag, target, *pages = sys.argv[1:]
jobs = []
for page in pages:
    with open(page, encoding="utf-8") as handle:
        jobs.extend(json.load(handle).get("jobs", []))

def fail(message):
    print(
        f"failed Release run {run_id} attempt {run_attempt} is not safe "
        f"channel-repair authority for {tag}: {message}",
        file=sys.stderr,
    )
    raise SystemExit(1)

def one_job(name):
    matches = [job for job in jobs if job.get("name") == name]
    if len(matches) != 1:
        fail(f"expected exactly one latest-attempt job {name!r}, found {len(matches)}")
    job = matches[0]
    if job.get("head_sha") != commit:
        fail(f"job {name!r} is not bound to {commit}")
    if job.get("status") != "completed":
        fail(f"job {name!r} is not completed")
    return job

for name in (
    "release-contract",
    "Build signed release artifacts",
    "Verify Apple Developer ID signatures",
    "Publish immutable GitHub Release",
):
    if one_job(name).get("conclusion") != "success":
        fail(f"required job {name!r} did not succeed")

publish_release = one_job("Publish immutable GitHub Release")
immutable_steps = [
    step for step in publish_release.get("steps", [])
    if step.get("name") == "Require immutable published GitHub Release"
]
if len(immutable_steps) != 1:
    fail("expected exactly one immutable GitHub Release verification step")
if (
    immutable_steps[0].get("status") != "completed"
    or immutable_steps[0].get("conclusion") != "success"
):
    fail("immutable GitHub Release verification did not succeed")

channels = one_job("Publish npm and mirrors")
if channels.get("conclusion") not in {"success", "failure"}:
    fail("channel publication job was not completed with a conclusive result")

steps = channels.get("steps", [])
for name in (
    "Download and verify immutable GitHub Release",
    "Verify immutable npm package without publication credentials",
    "Inspect npm channel state",
    "Verify npm channel delivery",
):
    matches = [step for step in steps if step.get("name") == name]
    if len(matches) != 1:
        fail(f"expected exactly one channel step {name!r}, found {len(matches)}")
    step = matches[0]
    if step.get("status") != "completed" or step.get("conclusion") != "success":
        fail(f"required channel step {name!r} did not succeed")

failed_channel_steps = [
    step.get("name", "")
    for step in steps
    if step.get("conclusion") == "failure"
]
if channels.get("conclusion") == "failure":
    if failed_channel_steps != ["Sync release artifacts to China OSS mirror"]:
        fail(
            "failed channel publication must have exactly one failed OSS mirror step, "
            f"got {failed_channel_steps!r}"
        )
elif failed_channel_steps:
    fail(f"successful channel publication contains failed steps {failed_channel_steps!r}")

gitee = one_job("Mirror immutable release to Gitee")
if gitee.get("conclusion") not in {"success", "skipped", "failure"}:
    fail("Gitee mirror job did not complete with an allowed channel result")

delivery_gate = one_job("Release delivery gate")
if delivery_gate.get("conclusion") != "failure":
    fail("failed channel-repair run must end in a failed delivery gate")

allowed_failures = {
    "Publish npm and mirrors",
    "Mirror immutable release to Gitee",
    "Release delivery gate",
}
for job in jobs:
    if job.get("status") != "completed":
        fail(f"job {job.get('name', '')!r} is not completed")
    conclusion = job.get("conclusion")
    if conclusion not in {"success", "skipped", "failure"}:
        fail(f"job {job.get('name', '')!r} has disallowed conclusion {conclusion!r}")
    if conclusion == "failure" and job.get("name") not in allowed_failures:
        fail(f"unrelated job {job.get('name', '')!r} failed")

business_failures = [
    job.get("name")
    for job in (channels, gitee)
    if job.get("conclusion") == "failure"
]
if len(business_failures) != 1:
    fail(f"expected exactly one failed downstream channel job, got {business_failures!r}")
if target == "oss":
    if business_failures != ["Publish npm and mirrors"] or gitee.get("conclusion") != "skipped":
        fail(
            "OSS repair requires the OSS mirror step to be the only failed "
            "downstream channel and Gitee to be skipped"
        )
elif target == "gitee":
    if gitee.get("conclusion") == "failure":
        if business_failures != ["Mirror immutable release to Gitee"]:
            fail("Gitee repair evidence contains a different failed downstream channel")
    elif gitee.get("conclusion") == "skipped":
        if business_failures != ["Publish npm and mirrors"]:
            fail(
                "skipped Gitee backfill requires the upstream OSS mirror to be "
                "the only failed downstream channel"
            )
    else:
        fail("Gitee repair requires its mirror job to be failed or skipped")
else:
    fail(f"unsupported channel repair target {target!r}")
PY
  rm -rf "$jobs_dir"
  return "$result"
}

push_delivery="$(find_push_delivery || true)"
if [ -n "$push_delivery" ]; then
  printf 'Release workflow delivery verified through exact-tag push run %s: %s -> %s\n' \
    "$push_delivery" "$TAG" "$EXPECTED_COMMIT"
  exit 0
fi

if [ "$MODE" = "channel-repair" ]; then
  failed_push_identity="$(find_failed_push_delivery || true)"
  failed_push_delivery="$(printf '%s\n' "$failed_push_identity" | cut -f1)"
  failed_push_attempt="$(printf '%s\n' "$failed_push_identity" | cut -f2)"
  if [ -n "$failed_push_delivery" ] &&
    verify_channel_repair_delivery "$failed_push_delivery" "$failed_push_attempt"; then
    printf 'Release %s channel-repair authority verified through failed exact-tag push run %s attempt %s: %s -> %s\n' \
      "$CHANNEL_REPAIR_TARGET" "$failed_push_delivery" "$failed_push_attempt" "$TAG" "$EXPECTED_COMMIT"
    exit 0
  fi
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
