#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"
RECOVERY_MANIFEST="$SCRIPT_DIR/delivered-stable-recoveries.json"

TAG="${1:-}"
EXPECTED_COMMIT="${2:-}"
REPOSITORY="${DWS_RELEASE_OFFICIAL_REPOSITORY:-DingTalk-Real-AI/dingtalk-workspace-cli}"

[ -n "$TAG" ] && [ -n "$EXPECTED_COMMIT" ] || {
  printf 'usage: verify-delivered-stable.sh <stable-tag> <commit>\n' >&2
  exit 2
}
release_is_stable_version "$TAG" || {
  printf 'invalid stable delivery baseline: %s\n' "$TAG" >&2
  exit 2
}
command -v curl >/dev/null 2>&1 || { printf 'curl is required to verify stable delivery\n' >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { printf 'python3 is required to verify stable delivery\n' >&2; exit 1; }

API_TOKEN="${DWS_RELEASE_GITHUB_TOKEN:-}"
if [ -z "$API_TOKEN" ] && [ "${GITHUB_ACTIONS:-false}" != "true" ] && command -v gh >/dev/null 2>&1; then
  API_TOKEN="$(gh auth token 2>/dev/null || true)"
fi
if [ -z "$API_TOKEN" ]; then
  API_TOKEN="${GITHUB_TOKEN:-}"
fi
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

if git rev-parse --verify --quiet "refs/tags/$TAG^{commit}" >/dev/null; then
  local_commit="$(git rev-parse "refs/tags/$TAG^{commit}")"
  [ "$local_commit" = "$EXPECTED_COMMIT" ] || {
    printf 'stable tag %s resolves to %s, expected %s\n' "$TAG" "$local_commit" "$EXPECTED_COMMIT" >&2
    exit 1
  }
fi

stable_state="$(
  github_get "repos/$REPOSITORY/releases/tags/$TAG" \
    | python3 -c 'import json,sys
r=json.load(sys.stdin)
print("%s\t%s\t%s" % (r.get("tag_name", ""), str(r.get("draft", "")).lower(), str(r.get("prerelease", "")).lower()))'
)" || {
  printf 'could not query previous stable release %s in %s\n' "$TAG" "$REPOSITORY" >&2
  exit 1
}
[ "$stable_state" = "$(printf '%s\tfalse\tfalse' "$TAG")" ] || {
  printf '%s is not a public stable GitHub Release in %s (got: %s)\n' \
    "$TAG" "$REPOSITORY" "$stable_state" >&2
  exit 1
}

stable_runs="$(
  github_get "repos/$REPOSITORY/actions/workflows/release.yml/runs?branch=$TAG&event=push&status=completed&per_page=100" \
    | python3 -c 'import json,sys
for run in json.load(sys.stdin).get("workflow_runs", []):
    print("%s\t%s\t%s" % (run.get("head_sha", ""), run.get("head_branch", ""), run.get("conclusion", "")))'
)" || {
  printf 'could not query Release workflow delivery for %s in %s\n' "$TAG" "$REPOSITORY" >&2
  exit 1
}
printf '%s\n' "$stable_runs" | awk -F '\t' -v sha="$EXPECTED_COMMIT" -v tag="$TAG" '
  $1 == sha && $2 == tag && $3 == "success" { found = 1 }
  END { exit(found ? 0 : 1) }
' && delivered_by_push=1 || delivered_by_push=0

if [ "$delivered_by_push" -ne 1 ]; then
  recovery_proof="$(
    python3 - "$RECOVERY_MANIFEST" "$TAG" "$EXPECTED_COMMIT" <<'PY'
import json
import sys

path, tag, commit = sys.argv[1:]
with open(path, encoding="utf-8") as handle:
    payload = json.load(handle)
if set(payload) != {"recoveries"} or not isinstance(payload["recoveries"], list):
    raise SystemExit("invalid delivered stable recovery manifest")

required = {
    "tag", "commit", "run_id", "workflow_sha", "head_branch",
    "run_attempt", "reviewed", "review_reason",
}
matches = []
identities = set()
for entry in payload["recoveries"]:
    if not isinstance(entry, dict) or set(entry) != required:
        raise SystemExit("invalid delivered stable recovery entry")
    identity = (entry["tag"], entry["commit"])
    if identity in identities:
        raise SystemExit("duplicate delivered stable recovery entry")
    identities.add(identity)
    if (
        not isinstance(entry["tag"], str)
        or not isinstance(entry["commit"], str)
        or not isinstance(entry["run_id"], int)
        or entry["run_id"] <= 0
        or not isinstance(entry["workflow_sha"], str)
        or not isinstance(entry["head_branch"], str)
        or not isinstance(entry["run_attempt"], int)
        or entry["run_attempt"] <= 0
        or entry["reviewed"] is not True
        or not isinstance(entry["review_reason"], str)
        or not entry["review_reason"].strip()
    ):
        raise SystemExit("invalid delivered stable recovery evidence")
    if identity == (tag, commit):
        matches.append(entry)

if len(matches) > 1:
    raise SystemExit("ambiguous delivered stable recovery evidence")
if matches:
    entry = matches[0]
    print(entry["run_id"])
    print(entry["workflow_sha"])
    print(entry["head_branch"])
    print(entry["run_attempt"])
PY
  )" || exit 1
  [ -n "$recovery_proof" ] || {
    printf 'Release workflow did not complete successfully for stable baseline %s at %s\n' \
      "$TAG" "$EXPECTED_COMMIT" >&2
    exit 1
  }

  recovery_run_id="$(printf '%s\n' "$recovery_proof" | sed -n '1p')"
  recovery_workflow_sha="$(printf '%s\n' "$recovery_proof" | sed -n '2p')"
  recovery_head_branch="$(printf '%s\n' "$recovery_proof" | sed -n '3p')"
  recovery_run_attempt="$(printf '%s\n' "$recovery_proof" | sed -n '4p')"
  recovery_run_state="$(
    github_get "repos/$REPOSITORY/actions/runs/$recovery_run_id" \
      | python3 -c 'import json,sys
r=json.load(sys.stdin)
print("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s" % (
    r.get("event", ""), r.get("status", ""), r.get("conclusion", ""),
    r.get("head_sha", ""), r.get("head_branch", ""), r.get("run_attempt", ""),
    r.get("path", ""), r.get("repository", {}).get("full_name", "")))'
  )" || {
    printf 'could not query reviewed recovery run %s for %s\n' "$recovery_run_id" "$TAG" >&2
    exit 1
  }
  expected_recovery_state="$(printf 'workflow_dispatch\tcompleted\tsuccess\t%s\t%s\t%s\t.github/workflows/release.yml\t%s' \
    "$recovery_workflow_sha" "$recovery_head_branch" "$recovery_run_attempt" "$REPOSITORY")"
  [ "$recovery_run_state" = "$expected_recovery_state" ] || {
    printf 'reviewed recovery run %s does not match the pinned delivery proof for %s\n' \
      "$recovery_run_id" "$TAG" >&2
    exit 1
  }

  recovery_jobs="$(
    github_get "repos/$REPOSITORY/actions/runs/$recovery_run_id/jobs?per_page=100" \
      | python3 -c 'import json,sys
for job in json.load(sys.stdin).get("jobs", []):
    print("%s\t%s\t%s\t%s" % (
        job.get("name", ""), job.get("status", ""),
        job.get("conclusion", ""), job.get("head_sha", "")))'
  )" || {
    printf 'could not query reviewed recovery jobs for %s\n' "$TAG" >&2
    exit 1
  }
  for recovery_job in release verify-darwin-signatures publish-release; do
    printf '%s\n' "$recovery_jobs" | awk -F '\t' -v name="$recovery_job" -v sha="$recovery_workflow_sha" '
      $1 == name && $2 == "completed" && $3 == "success" && $4 == sha { found++ }
      END { exit(found == 1 ? 0 : 1) }
    ' || {
      printf 'reviewed recovery run %s is missing successful job %s for %s\n' \
        "$recovery_run_id" "$recovery_job" "$TAG" >&2
      exit 1
    }
  done
  printf 'Delivered stable baseline verified through reviewed recovery run %s: %s -> %s\n' \
    "$recovery_run_id" "$TAG" "$EXPECTED_COMMIT"
  exit 0
fi

printf 'Delivered stable baseline verified: %s -> %s\n' "$TAG" "$EXPECTED_COMMIT"
