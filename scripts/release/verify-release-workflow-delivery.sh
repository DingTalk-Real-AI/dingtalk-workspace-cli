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
elif [ "${1:-}" = "--npm-repair" ]; then
  MODE="npm-repair"
  shift
fi

TAG="${1:-}"
EXPECTED_COMMIT="${2:-}"
REPOSITORY="${DWS_RELEASE_OFFICIAL_REPOSITORY:-DingTalk-Real-AI/dingtalk-workspace-cli}"

[ -n "$TAG" ] && [ -n "$EXPECTED_COMMIT" ] || {
  printf 'usage: verify-release-workflow-delivery.sh [--channel-repair <oss|gitee> | --npm-repair] <tag> <commit>\n' >&2
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

find_cloud_delivery_identity() {
  tag_ref="$(
    github_get "repos/$REPOSITORY/git/ref/tags/$TAG" \
      | python3 -c 'import json,sys
ref=json.load(sys.stdin)
obj=ref.get("object", {})
if obj.get("type") == "tag" and obj.get("sha"):
    print(obj["sha"])'
  )" || return 1
  [ -n "$tag_ref" ] || return 1
  github_get "repos/$REPOSITORY/git/tags/$tag_ref" \
    | python3 -c 'import json,re,sys
tag,commit=sys.argv[1:]
payload=json.load(sys.stdin)
if payload.get("tag") != tag or payload.get("object", {}).get("type") != "commit":
    raise SystemExit(1)
if payload.get("object", {}).get("sha") != commit:
    raise SystemExit(1)
fields={}
for line in payload.get("message", "").splitlines():
    if ": " not in line:
        continue
    key,value=line.split(": ", 1)
    if key in fields:
        raise SystemExit(1)
    fields[key]=value
required={
    "Channel", "Release-Run", "Release-Run-Attempt", "Requested-By",
    "Requested-By-ID", "Sealed-Commit", "Workflow-Commit",
    "Allocation-Fingerprint",
}
if not required.issubset(fields):
    raise SystemExit(1)
if fields["Sealed-Commit"] != commit or fields["Workflow-Commit"] != commit:
    raise SystemExit(1)
if not re.fullmatch(r"[1-9][0-9]*", fields["Release-Run"]):
    raise SystemExit(1)
if not re.fullmatch(r"[1-9][0-9]*", fields["Release-Run-Attempt"]):
    raise SystemExit(1)
if not re.fullmatch(r"[1-9][0-9]*", fields["Requested-By-ID"]):
    raise SystemExit(1)
if not fields["Requested-By"] or not re.fullmatch(r"[0-9a-f]{64}", fields["Allocation-Fingerprint"]):
    raise SystemExit(1)
is_beta="-beta." in tag
if fields["Channel"] != ("prerelease" if is_beta else "stable"):
    raise SystemExit(1)
from_beta=fields.get("From-Beta", "")
if is_beta:
    if from_beta:
        raise SystemExit(1)
else:
    core=re.escape(tag)
    if not re.fullmatch(core + r"-beta\.[1-9][0-9]*", from_beta):
        raise SystemExit(1)
print("\t".join([
    fields["Release-Run"],
    fields["Release-Run-Attempt"],
    fields["Requested-By"],
    fields["Requested-By-ID"],
    fields["Workflow-Commit"],
]))' "$TAG" "$EXPECTED_COMMIT"
}

verify_cloud_delivery() {
  cloud_identity="$1"
  cloud_run_id="$(printf '%s\n' "$cloud_identity" | cut -f1)"
  cloud_run_attempt="$(printf '%s\n' "$cloud_identity" | cut -f2)"
  cloud_actor="$(printf '%s\n' "$cloud_identity" | cut -f3)"
  cloud_actor_id="$(printf '%s\n' "$cloud_identity" | cut -f4)"
  cloud_workflow_sha="$(printf '%s\n' "$cloud_identity" | cut -f5)"
  [ -n "$cloud_run_id" ] && [ -n "$cloud_run_attempt" ] &&
    [ -n "$cloud_actor" ] && [ -n "$cloud_actor_id" ] &&
    [ -n "$cloud_workflow_sha" ] || return 1

  cloud_run_state="$(
    github_get "repos/$REPOSITORY/actions/runs/$cloud_run_id/attempts/$cloud_run_attempt" \
      | python3 -c 'import json,sys
r=json.load(sys.stdin)
print("\t".join(str(value) for value in (
    r.get("id", ""),
    r.get("run_attempt", ""),
    r.get("repository", {}).get("full_name", ""),
    r.get("path", ""),
    r.get("event", ""),
    r.get("status", ""),
    r.get("conclusion", ""),
    r.get("head_branch", ""),
    r.get("head_sha", ""),
    r.get("actor", {}).get("login", ""),
    r.get("actor", {}).get("id", ""),
)))'
  )" || return 1
  expected_cloud_core="$(printf '%s\t%s\t%s\t.github/workflows/release.yml\tworkflow_dispatch\tcompleted\tsuccess\tmain\t%s' \
    "$cloud_run_id" "$cloud_run_attempt" "$REPOSITORY" "$cloud_workflow_sha")"
  [ "$(printf '%s\n' "$cloud_run_state" | cut -f1-9)" = "$expected_cloud_core" ] ||
    return 1
  [ -n "$(printf '%s\n' "$cloud_run_state" | cut -f10)" ] || return 1
  [ "$(printf '%s\n' "$cloud_run_state" | cut -f11)" = "$cloud_actor_id" ] ||
    return 1
  [ "$cloud_workflow_sha" = "$EXPECTED_COMMIT" ] || return 1

  jobs_dir="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-cloud-jobs.XXXXXX")"
  page=1
  while :; do
    jobs_page="$jobs_dir/jobs-$page.json"
    if ! github_get "repos/$REPOSITORY/actions/runs/$cloud_run_id/attempts/$cloud_run_attempt/jobs?per_page=100&page=$page" \
      >"$jobs_page"; then
      rm -rf "$jobs_dir"
      return 1
    fi
    page_count="$(
      python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1])).get("jobs", [])))' \
        "$jobs_page"
    )" || {
      rm -rf "$jobs_dir"
      return 1
    }
    [ "$page_count" -eq 100 ] || break
    page=$((page + 1))
  done

  result=0
  python3 - "$cloud_workflow_sha" "$jobs_dir"/jobs-*.json <<'PY' || result=$?
import json
import sys

workflow_sha, *pages = sys.argv[1:]
jobs = []
for page in pages:
    with open(page, encoding="utf-8") as handle:
        jobs.extend(json.load(handle).get("jobs", []))

required = (
    "Plan next cloud release",
    "Seal cloud release tag",
    "release-contract",
    "Build signed release artifacts",
    "Verify Apple Developer ID signatures",
    "Publish immutable GitHub Release",
    "Publish npm and mirrors",
    "Release delivery gate",
)
for name in required:
    matches = [job for job in jobs if job.get("name") == name]
    if len(matches) != 1:
        raise SystemExit(1)
    job = matches[0]
    if (
        job.get("head_sha") != workflow_sha
        or job.get("status") != "completed"
        or job.get("conclusion") != "success"
    ):
        raise SystemExit(1)

seal = next(job for job in jobs if job.get("name") == "Seal cloud release tag")
steps = [
    step for step in seal.get("steps", [])
    if step.get("name") == "Create one immutable annotated release tag"
]
if (
    len(steps) != 1
    or steps[0].get("status") != "completed"
    or steps[0].get("conclusion") != "success"
):
    raise SystemExit(1)
PY
  rm -rf "$jobs_dir"
  return "$result"
}

verify_failed_cloud_delivery_identity() {
  cloud_identity="$1"
  cloud_run_id="$(printf '%s\n' "$cloud_identity" | cut -f1)"
  cloud_run_attempt="$(printf '%s\n' "$cloud_identity" | cut -f2)"
  cloud_actor="$(printf '%s\n' "$cloud_identity" | cut -f3)"
  cloud_actor_id="$(printf '%s\n' "$cloud_identity" | cut -f4)"
  cloud_workflow_sha="$(printf '%s\n' "$cloud_identity" | cut -f5)"
  [ "$cloud_workflow_sha" = "$EXPECTED_COMMIT" ] || return 1
  cloud_run_state="$(
    github_get "repos/$REPOSITORY/actions/runs/$cloud_run_id/attempts/$cloud_run_attempt" \
      | python3 -c 'import json,sys
r=json.load(sys.stdin)
print("\t".join(str(value) for value in (
    r.get("id", ""),
    r.get("run_attempt", ""),
    r.get("repository", {}).get("full_name", ""),
    r.get("path", ""),
    r.get("event", ""),
    r.get("status", ""),
    r.get("conclusion", ""),
    r.get("head_branch", ""),
    r.get("head_sha", ""),
    r.get("actor", {}).get("login", ""),
    r.get("actor", {}).get("id", ""),
)))'
  )" || return 1
  expected_cloud_core="$(printf '%s\t%s\t%s\t.github/workflows/release.yml\tworkflow_dispatch\tcompleted\tfailure\tmain\t%s' \
    "$cloud_run_id" "$cloud_run_attempt" "$REPOSITORY" "$cloud_workflow_sha")"
  [ "$(printf '%s\n' "$cloud_run_state" | cut -f1-9)" = "$expected_cloud_core" ] &&
    [ -n "$(printf '%s\n' "$cloud_run_state" | cut -f10)" ] &&
    [ "$(printf '%s\n' "$cloud_run_state" | cut -f11)" = "$cloud_actor_id" ]
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

verify_npm_repair_delivery() {
  run_id="$1"
  run_attempt="$2"
  require_cloud_seal="$3"
  jobs_dir="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-npm-repair-jobs.XXXXXX")"
  page=1
  while :; do
    jobs_page="$jobs_dir/jobs-$page.json"
    if ! github_get "repos/$REPOSITORY/actions/runs/$run_id/attempts/$run_attempt/jobs?per_page=100&page=$page" \
      >"$jobs_page"; then
      rm -rf "$jobs_dir"
      return 1
    fi
    if ! page_count="$(
      python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1])).get("jobs", [])))' \
        "$jobs_page"
    )"; then
      rm -rf "$jobs_dir"
      return 1
    fi
    [ "$page_count" -eq 100 ] || break
    page=$((page + 1))
  done

  result=0
  python3 - "$EXPECTED_COMMIT" "$run_id" "$run_attempt" "$TAG" "$require_cloud_seal" "$jobs_dir"/jobs-*.json <<'PY' || result=$?
import json
import sys

commit, run_id, run_attempt, tag, require_cloud_seal, *pages = sys.argv[1:]
jobs = []
for page in pages:
    with open(page, encoding="utf-8") as handle:
        jobs.extend(json.load(handle).get("jobs", []))

def fail(message):
    print(
        f"Release run {run_id} attempt {run_attempt} is not safe npm-repair "
        f"authority for {tag}: {message}",
        file=sys.stderr,
    )
    raise SystemExit(1)

def one_job(name):
    matches = [job for job in jobs if job.get("name") == name]
    if len(matches) != 1:
        fail(f"expected exactly one job {name!r}, found {len(matches)}")
    job = matches[0]
    if (
        job.get("head_sha") != commit
        or job.get("status") != "completed"
        or job.get("conclusion") != "success"
    ):
        fail(f"required job {name!r} did not succeed at {commit}")
    return job

for name in (
    "release-contract",
    "Build signed release artifacts",
    "Verify Apple Developer ID signatures",
):
    one_job(name)

if require_cloud_seal == "true":
    one_job("Plan next cloud release")
    seal = one_job("Seal cloud release tag")
    seal_steps = [
        step for step in seal.get("steps", [])
        if step.get("name") == "Create one immutable annotated release tag"
    ]
    if (
        len(seal_steps) != 1
        or seal_steps[0].get("status") != "completed"
        or seal_steps[0].get("conclusion") != "success"
    ):
        fail("cloud release seal step did not succeed")
elif require_cloud_seal != "false":
    fail(f"invalid cloud seal requirement {require_cloud_seal!r}")

published = one_job("Publish immutable GitHub Release")
steps = [
    step for step in published.get("steps", [])
    if step.get("name") == "Require immutable published GitHub Release"
]
if (
    len(steps) != 1
    or steps[0].get("status") != "completed"
    or steps[0].get("conclusion") != "success"
):
    fail("immutable GitHub Release verification step did not succeed")

channels = [job for job in jobs if job.get("name") == "Publish npm and mirrors"]
if len(channels) != 1:
    fail(f"expected exactly one npm publication job, found {len(channels)}")
channel = channels[0]
if (
    channel.get("head_sha") != commit
    or channel.get("status") != "completed"
    or channel.get("conclusion") not in {"success", "failure"}
):
    fail("npm publication job is not a completed success/failure at the release commit")
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

cloud_delivery_identity="$(find_cloud_delivery_identity || true)"
if [ -n "$cloud_delivery_identity" ] &&
  verify_cloud_delivery "$cloud_delivery_identity"; then
  cloud_delivery_run="$(printf '%s\n' "$cloud_delivery_identity" | cut -f1)"
  printf 'Release workflow delivery verified through cloud release run %s: %s -> %s\n' \
    "$cloud_delivery_run" "$TAG" "$EXPECTED_COMMIT"
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
  if [ -n "$cloud_delivery_identity" ] &&
    verify_failed_cloud_delivery_identity "$cloud_delivery_identity"; then
    failed_cloud_run="$(printf '%s\n' "$cloud_delivery_identity" | cut -f1)"
    failed_cloud_attempt="$(printf '%s\n' "$cloud_delivery_identity" | cut -f2)"
    if verify_channel_repair_delivery "$failed_cloud_run" "$failed_cloud_attempt"; then
      printf 'Release %s channel-repair authority verified through failed cloud release run %s attempt %s: %s -> %s\n' \
        "$CHANNEL_REPAIR_TARGET" "$failed_cloud_run" "$failed_cloud_attempt" "$TAG" "$EXPECTED_COMMIT"
      exit 0
    fi
  fi
fi

if [ "$MODE" = "npm-repair" ]; then
  failed_push_identity="$(find_failed_push_delivery || true)"
  failed_push_run="$(printf '%s\n' "$failed_push_identity" | cut -f1)"
  failed_push_attempt="$(printf '%s\n' "$failed_push_identity" | cut -f2)"
  if [ -n "$failed_push_run" ] &&
    verify_npm_repair_delivery "$failed_push_run" "$failed_push_attempt" false; then
    printf 'Release npm-repair authority verified through failed exact-tag push run %s attempt %s: %s -> %s\n' \
      "$failed_push_run" "$failed_push_attempt" "$TAG" "$EXPECTED_COMMIT"
    exit 0
  fi
  if [ -n "$cloud_delivery_identity" ] &&
    verify_failed_cloud_delivery_identity "$cloud_delivery_identity"; then
    failed_cloud_run="$(printf '%s\n' "$cloud_delivery_identity" | cut -f1)"
    failed_cloud_attempt="$(printf '%s\n' "$cloud_delivery_identity" | cut -f2)"
    if verify_npm_repair_delivery "$failed_cloud_run" "$failed_cloud_attempt" true; then
      printf 'Release npm-repair authority verified through failed cloud release run %s attempt %s: %s -> %s\n' \
        "$failed_cloud_run" "$failed_cloud_attempt" "$TAG" "$EXPECTED_COMMIT"
      exit 0
    fi
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
  printf 'Release workflow did not deliver %s at %s through a tag push, cloud release, or protected recovery\n' \
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
