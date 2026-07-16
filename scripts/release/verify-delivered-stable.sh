#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"

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
' || {
  printf 'Release workflow did not complete successfully for stable baseline %s at %s\n' \
    "$TAG" "$EXPECTED_COMMIT" >&2
  exit 1
}

printf 'Delivered stable baseline verified: %s -> %s\n' "$TAG" "$EXPECTED_COMMIT"
