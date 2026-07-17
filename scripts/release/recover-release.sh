#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"

VERSION="${1:-}"
REMOTE=""
FAILED_RUN_ID=""
EXPECTED_REPOSITORY="DingTalk-Real-AI/dingtalk-workspace-cli"

usage() {
  cat >&2 <<'EOF'
usage: recover-release.sh <version> --remote <name> [--failed-run <run-id>]

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
command -v gh >/dev/null 2>&1 || { printf '%s\n' 'gh is required for release recovery' >&2; exit 1; }

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
git merge-base --is-ancestor "$commit" "refs/remotes/$REMOTE/main" || {
  printf '%s commit %s is not contained in %s/main\n' "$VERSION" "$commit" "$REMOTE" >&2
  exit 1
}

if [ -z "$FAILED_RUN_ID" ]; then
  failed_runs="$(
    gh api \
      -H 'Accept: application/vnd.github+json' \
      "repos/$EXPECTED_REPOSITORY/actions/workflows/release.yml/runs?branch=$VERSION&event=push&status=completed&per_page=100" \
      --jq ".workflow_runs[] | select(.head_sha == \"$commit\" and .head_branch == \"$VERSION\" and (.conclusion as \\$c | [\"failure\", \"cancelled\", \"timed_out\", \"startup_failure\", \"stale\"] | index(\\$c))) | .id"
  )" || {
    printf 'could not query failed Release runs for %s\n' "$VERSION" >&2
    exit 1
  }
  FAILED_RUN_ID="$(printf '%s\n' "$failed_runs" | sed -n '1p')"
  [ -n "$FAILED_RUN_ID" ] || {
    printf 'no failed exact-tag Release run found for %s at %s\n' "$VERSION" "$commit" >&2
    exit 1
  }
fi

printf 'Recovery target:\n'
printf '  version:    %s\n' "$VERSION"
printf '  tag object: %s\n' "$tag_object"
printf '  commit:     %s\n' "$commit"
printf '  failed run: https://github.com/%s/actions/runs/%s\n' "$EXPECTED_REPOSITORY" "$FAILED_RUN_ID"
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
