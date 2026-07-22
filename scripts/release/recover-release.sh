#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"

VERSION="${1:-}"
REMOTE=""
FAILED_RUN_ID=""
FAILED_RUN_ATTEMPT=""
EXPECTED_REPOSITORY="DingTalk-Real-AI/dingtalk-workspace-cli"

usage() {
  cat >&2 <<'EOF'
usage: recover-release.sh <version> --remote <name> [--failed-run <run-id>] [--failed-attempt <attempt>]

Recovers one failed existing release tag through the protected default-branch
Release workflow. It never creates, moves, or deletes a tag.
EOF
}

[ -n "$VERSION" ] || { usage; exit 2; }
shift
while [ "$#" -gt 0 ]; do
  case "$1" in
    --remote) [ "$#" -ge 2 ] || { usage; exit 2; }; REMOTE="$2"; shift 2 ;;
    --failed-run) [ "$#" -ge 2 ] || { usage; exit 2; }; FAILED_RUN_ID="$2"; shift 2 ;;
    --failed-attempt) [ "$#" -ge 2 ] || { usage; exit 2; }; FAILED_RUN_ATTEMPT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown recovery argument: %s\n' "$1" >&2; usage; exit 2 ;;
  esac
done

channel="$(release_channel_for_version "$VERSION")" || exit 2
release_validate_version_channel "$channel" "$VERSION"
[ -n "$REMOTE" ] || { printf '%s\n' '--remote is required' >&2; exit 2; }
if [ -n "$FAILED_RUN_ID" ]; then
  printf '%s\n' "$FAILED_RUN_ID" | grep -Eq '^[1-9][0-9]*$' || {
    printf 'invalid failed Release run ID: %s\n' "$FAILED_RUN_ID" >&2
    exit 2
  }
fi
if [ -n "$FAILED_RUN_ATTEMPT" ]; then
  [ -n "$FAILED_RUN_ID" ] || {
    printf '%s\n' '--failed-attempt requires --failed-run' >&2
    exit 2
  }
  printf '%s\n' "$FAILED_RUN_ATTEMPT" | grep -Eq '^[1-9][0-9]*$' || {
    printf 'invalid failed Release run attempt: %s\n' "$FAILED_RUN_ATTEMPT" >&2
    exit 2
  }
fi
command -v gh >/dev/null 2>&1 || { printf '%s\n' 'gh is required for release recovery' >&2; exit 1; }

find_latest_failed_attempt() {
  find_run_id="$1"
  find_latest="$(
    gh api \
      -H 'Accept: application/vnd.github+json' \
      "repos/$EXPECTED_REPOSITORY/actions/runs/$find_run_id" \
      --jq .run_attempt
  )" || return 1
  printf '%s\n' "$find_latest" | grep -Eq '^[1-9][0-9]*$' || return 1
  find_attempt="$find_latest"
  while [ "$find_attempt" -ge 1 ]; do
    find_conclusion="$(
      gh api \
        -H 'Accept: application/vnd.github+json' \
        "repos/$EXPECTED_REPOSITORY/actions/runs/$find_run_id/attempts/$find_attempt" \
        --jq .conclusion
    )" || return 1
    case "$find_conclusion" in
      failure|cancelled|timed_out|startup_failure|stale)
        printf '%s\n' "$find_attempt"
        return 0
        ;;
    esac
    find_attempt=$((find_attempt - 1))
  done
  return 1
}

cd "$ROOT"
[ "$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)" = "main" ] || {
  printf '%s\n' 'release recovery must run from the main worktree' >&2
  exit 1
}
[ -z "$(git status --porcelain --untracked-files=all)" ] || {
  printf '%s\n' 'release recovery requires a clean main worktree' >&2
  exit 1
}

push_url="$(git remote get-url --push "$REMOTE" 2>/dev/null)" || {
  printf 'unknown release remote: %s\n' "$REMOTE" >&2
  exit 1
}
case "$push_url" in
  https://github.com/*) repository_path="${push_url#https://github.com/}" ;;
  git@github.com:*) repository_path="${push_url#git@github.com:}" ;;
  ssh://git@github.com/*) repository_path="${push_url#ssh://git@github.com/}" ;;
  *) printf 'release recovery requires the official GitHub remote, got: %s\n' "$push_url" >&2; exit 1 ;;
esac
repository_path="${repository_path%/}"
repository_path="${repository_path%.git}"
[ "$repository_path" = "$EXPECTED_REPOSITORY" ] || {
  printf 'release recovery is restricted to %s, got %s\n' "$EXPECTED_REPOSITORY" "$repository_path" >&2
  exit 1
}

printf '==> Refreshing main and exact recovery tag %s\n' "$VERSION"
git fetch --force "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main"
recovery_ref="refs/dws-release-recovery/$VERSION"
cleanup() { git update-ref -d "$recovery_ref" >/dev/null 2>&1 || true; }
trap cleanup EXIT HUP INT TERM
git fetch --force --no-tags "$REMOTE" "+refs/tags/$VERSION:$recovery_ref"
[ "$(git cat-file -t "$recovery_ref")" = "tag" ] || {
  printf '%s must be an annotated tag\n' "$VERSION" >&2
  exit 1
}
tag_object="$(git rev-parse "$recovery_ref")"
commit="$(git rev-parse "$recovery_ref^{commit}")"
tag_message="$(git for-each-ref "$recovery_ref" --format='%(contents)')"
cloud_run_id="$(printf '%s\n' "$tag_message" | awk -F ': ' '$1 == "Release-Run" { print $2 }')"
cloud_run_attempt="$(printf '%s\n' "$tag_message" | awk -F ': ' '$1 == "Release-Run-Attempt" { print $2 }')"
cloud_actor="$(printf '%s\n' "$tag_message" | awk -F ': ' '$1 == "Requested-By" { print $2 }')"
cloud_actor_id="$(printf '%s\n' "$tag_message" | awk -F ': ' '$1 == "Requested-By-ID" { print $2 }')"
cloud_sealed_commit="$(printf '%s\n' "$tag_message" | awk -F ': ' '$1 == "Sealed-Commit" { print $2 }')"
cloud_marker_count=0
for cloud_value in "$cloud_run_id" "$cloud_run_attempt" "$cloud_actor" "$cloud_actor_id" "$cloud_sealed_commit"; do
  [ -z "$cloud_value" ] || cloud_marker_count=$((cloud_marker_count + 1))
done
if [ "$cloud_marker_count" -ne 0 ] && [ "$cloud_marker_count" -ne 5 ]; then
  printf '%s contains incomplete cloud release metadata\n' "$VERSION" >&2
  exit 1
fi
if [ "$cloud_marker_count" -eq 5 ]; then
  for cloud_number in "$cloud_run_id" "$cloud_run_attempt" "$cloud_actor_id"; do
    printf '%s\n' "$cloud_number" | grep -Eq '^[1-9][0-9]*$' || {
      printf '%s contains invalid cloud release identity\n' "$VERSION" >&2
      exit 1
    }
  done
  [ "$cloud_sealed_commit" = "$commit" ] || {
    printf '%s cloud release metadata is not bound to %s\n' "$VERSION" "$commit" >&2
    exit 1
  }
fi
git merge-base --is-ancestor "$commit" "refs/remotes/$REMOTE/main" || {
  printf '%s commit %s is not contained in %s/main\n' "$VERSION" "$commit" "$REMOTE" >&2
  exit 1
}

if [ -z "$FAILED_RUN_ID" ] && [ "$cloud_marker_count" -eq 5 ]; then
  FAILED_RUN_ID="$cloud_run_id"
  FAILED_RUN_ATTEMPT="$cloud_run_attempt"
elif [ -z "$FAILED_RUN_ID" ]; then
  candidate_runs="$(
    gh api \
      -H 'Accept: application/vnd.github+json' \
      "repos/$EXPECTED_REPOSITORY/actions/workflows/release.yml/runs?branch=$VERSION&event=push&status=completed&per_page=100" \
      --jq ".workflow_runs[] | select(.head_sha == \"$commit\" and .head_branch == \"$VERSION\") | .id"
  )" || {
    printf 'could not query Release runs for %s\n' "$VERSION" >&2
    exit 1
  }
  for candidate_run_id in $candidate_runs; do
    if candidate_attempt="$(find_latest_failed_attempt "$candidate_run_id")"; then
      FAILED_RUN_ID="$candidate_run_id"
      FAILED_RUN_ATTEMPT="$candidate_attempt"
      break
    fi
  done
  [ -n "$FAILED_RUN_ID" ] || {
    printf 'no failed exact-tag Release run found for %s at %s\n' "$VERSION" "$commit" >&2
    exit 1
  }
elif [ -z "$FAILED_RUN_ATTEMPT" ]; then
  FAILED_RUN_ATTEMPT="$(find_latest_failed_attempt "$FAILED_RUN_ID")" || {
    printf 'Release run %s has no failed attempt\n' "$FAILED_RUN_ID" >&2
    exit 1
  }
fi
printf '%s\n' "$FAILED_RUN_ATTEMPT" | grep -Eq '^[1-9][0-9]*$' || {
  printf 'invalid failed Release run attempt: %s\n' "$FAILED_RUN_ATTEMPT" >&2
  exit 1
}
attempt_record="$(
  gh api \
    -H 'Accept: application/vnd.github+json' \
    "repos/$EXPECTED_REPOSITORY/actions/runs/$FAILED_RUN_ID/attempts/$FAILED_RUN_ATTEMPT" \
    --jq '[.id, .run_attempt, .repository.full_name, .path, .event, .status, .conclusion, .head_branch, .head_sha, .actor.login, .actor.id] | @tsv'
)" || {
  printf 'could not query Release run %s attempt %s\n' "$FAILED_RUN_ID" "$FAILED_RUN_ATTEMPT" >&2
  exit 1
}
attempt_id="$(printf '%s\n' "$attempt_record" | cut -f1)"
attempt_number="$(printf '%s\n' "$attempt_record" | cut -f2)"
attempt_repository="$(printf '%s\n' "$attempt_record" | cut -f3)"
attempt_path="$(printf '%s\n' "$attempt_record" | cut -f4)"
attempt_event="$(printf '%s\n' "$attempt_record" | cut -f5)"
attempt_status="$(printf '%s\n' "$attempt_record" | cut -f6)"
attempt_conclusion="$(printf '%s\n' "$attempt_record" | cut -f7)"
attempt_branch="$(printf '%s\n' "$attempt_record" | cut -f8)"
attempt_commit="$(printf '%s\n' "$attempt_record" | cut -f9)"
attempt_actor_id="$(printf '%s\n' "$attempt_record" | cut -f11)"
case "$attempt_conclusion" in
  failure|cancelled|timed_out|startup_failure|stale) ;;
  *)
    printf 'Release run %s attempt %s is not failed: %s\n' \
      "$FAILED_RUN_ID" "$FAILED_RUN_ATTEMPT" "$attempt_conclusion" >&2
    exit 1
    ;;
esac
if [ "$cloud_marker_count" -eq 5 ]; then
  expected_attempt_event="workflow_dispatch"
  expected_attempt_branch="main"
else
  expected_attempt_event="push"
  expected_attempt_branch="$VERSION"
fi
if [ "$attempt_id" != "$FAILED_RUN_ID" ] ||
   [ "$attempt_number" != "$FAILED_RUN_ATTEMPT" ] ||
   [ "$attempt_repository" != "$EXPECTED_REPOSITORY" ] ||
   [ "$attempt_path" != ".github/workflows/release.yml" ] ||
   [ "$attempt_event" != "$expected_attempt_event" ] ||
   [ "$attempt_status" != "completed" ] ||
   [ "$attempt_branch" != "$expected_attempt_branch" ] ||
   [ "$attempt_commit" != "$commit" ]; then
  printf 'Release run %s attempt %s does not match %s at %s\n' \
    "$FAILED_RUN_ID" "$FAILED_RUN_ATTEMPT" "$VERSION" "$commit" >&2
  exit 1
fi
if [ "$cloud_marker_count" -eq 5 ] &&
   { [ "$cloud_run_id" != "$FAILED_RUN_ID" ] ||
     [ "$cloud_run_attempt" != "$FAILED_RUN_ATTEMPT" ] ||
     [ "$cloud_actor_id" != "$attempt_actor_id" ]; }; then
  printf 'Release run %s attempt %s is not bound by the cloud seal for %s\n' \
    "$FAILED_RUN_ID" "$FAILED_RUN_ATTEMPT" "$VERSION" >&2
  exit 1
fi

printf 'Recovery target:\n'
printf '  version:    %s\n' "$VERSION"
printf '  tag object: %s\n' "$tag_object"
printf '  commit:     %s\n' "$commit"
printf '  failed run: https://github.com/%s/actions/runs/%s/attempts/%s\n' \
  "$EXPECTED_REPOSITORY" "$FAILED_RUN_ID" "$FAILED_RUN_ATTEMPT"
[ -t 0 ] || { printf '%s\n' 'interactive recovery confirmation is required' >&2; exit 1; }
printf 'Type %s to request protected recovery: ' "$VERSION"
IFS= read -r confirmation
[ "$confirmation" = "$VERSION" ] || { printf '%s\n' 'release recovery cancelled' >&2; exit 1; }

nonce="${commit}-$(date +%s)-$$"
workflow_sha="$(git rev-parse "refs/remotes/$REMOTE/main")"
gh workflow run release.yml \
  --repo "$EXPECTED_REPOSITORY" \
  --ref main \
  -f "recover_release_version=$VERSION" \
  -f "recover_release_tag_object=$tag_object" \
  -f "recover_release_commit=$commit" \
  -f "recover_failed_run_id=$FAILED_RUN_ID" \
  -f "recover_failed_run_attempt=$FAILED_RUN_ATTEMPT" \
  -f "recover_release_nonce=$nonce" \
  -f "recover_release_confirmation=$VERSION"

expected_title="Release recovery $VERSION at $commit $nonce"
started_at="$(date +%s)"
run_id=""
while :; do
  run_id="$(
    gh api \
      -H 'Accept: application/vnd.github+json' \
      "repos/$EXPECTED_REPOSITORY/actions/workflows/release.yml/runs?event=workflow_dispatch&branch=main&per_page=100" \
      --jq ".workflow_runs[] | select(.display_title == \"$expected_title\" and .head_sha == \"$workflow_sha\") | .id" \
      | sed -n '1p'
  )" || exit 1
  [ -z "$run_id" ] || break
  now="$(date +%s)"
  [ $((now - started_at)) -lt 60 ] || {
    printf 'recovery was dispatched but its run could not be located: %s\n' "$expected_title" >&2
    exit 1
  }
  sleep 2
done

printf 'Protected recovery run: https://github.com/%s/actions/runs/%s\n' "$EXPECTED_REPOSITORY" "$run_id"
printf '%s\n' 'Approve the release-recovery environment when prompted; this command will follow the run.'
if ! gh run watch "$run_id" --repo "$EXPECTED_REPOSITORY" --exit-status; then
  gh run view "$run_id" --repo "$EXPECTED_REPOSITORY" --log-failed >&2 || true
  exit 1
fi
printf 'Release recovery completed: %s\n' "$VERSION"
