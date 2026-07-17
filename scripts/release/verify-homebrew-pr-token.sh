#!/bin/sh
set -eu

REPOSITORY="${1:-}"
MODE="${2:-}"
TOKEN="${HOMEBREW_PR_TOKEN:-}"

usage() {
  printf '%s\n' 'usage: verify-homebrew-pr-token.sh <owner/repository> [--canary]' >&2
}

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

[ -n "$REPOSITORY" ] || {
  usage
  exit 2
}
case "$REPOSITORY" in
  */*) ;;
  *) usage; exit 2 ;;
esac
case "$MODE" in
  ""|--canary) ;;
  *) usage; exit 2 ;;
esac
[ -n "$TOKEN" ] || err "HOMEBREW_PR_TOKEN is required to open Formula PRs from official releases"
command -v gh >/dev/null 2>&1 || err "gh is required to verify HOMEBREW_PR_TOKEN"

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-homebrew-token-XXXXXX")"
CANARY_ROOT=""
CANARY_BRANCH=""
CANARY_COMMIT=""
CANARY_BRANCH_MAY_EXIST="false"
CANARY_PR_MAY_EXIST="false"
CANARY_PR_NUMBER=""
cleanup() {
  status=$?
  trap - EXIT INT TERM
  set +e
  cleanup_failed="false"
  if [ "$CANARY_PR_MAY_EXIST" = "true" ]; then
    cleanup_pr_number="$CANARY_PR_NUMBER"
    if [ -z "$cleanup_pr_number" ]; then
      if ! cleanup_pr_number="$(find_canary_pr_number)"; then
        printf 'error: could not inspect Homebrew token canary PR for branch %s\n' \
          "$CANARY_BRANCH" >&2
        cleanup_failed="true"
      fi
    fi
    if [ -n "$cleanup_pr_number" ] &&
      ! close_canary_pr "$cleanup_pr_number"; then
      printf 'error: could not clean up Homebrew token canary PR #%s\n' \
        "$cleanup_pr_number" >&2
      cleanup_failed="true"
    fi
  fi
  if [ "$CANARY_BRANCH_MAY_EXIST" = "true" ]; then
    if ! delete_canary_branch >/dev/null 2>&1; then
      printf 'error: could not clean up Homebrew token canary branch %s\n' \
        "$CANARY_BRANCH" >&2
      cleanup_failed="true"
    fi
  fi
  if ! rm -rf "$TMP_ROOT"; then
    printf 'error: could not clean up Homebrew token canary workspace\n' >&2
    cleanup_failed="true"
  fi
  if [ "$cleanup_failed" = "true" ] && [ "$status" -eq 0 ]; then
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

response="$TMP_ROOT/repository-response"
if ! GH_TOKEN="$TOKEN" gh api \
  -i \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2026-03-10' \
  "repos/$REPOSITORY" \
  --jq '[.full_name, (.permissions.push // false), .default_branch] | @tsv' \
  > "$response"; then
  err "HOMEBREW_PR_TOKEN could not authenticate to $REPOSITORY"
fi

scopes="$(
  awk '
    tolower($0) ~ /^x-oauth-scopes:/ {
      sub(/^[^:]*:[[:space:]]*/, "")
      sub(/\r$/, "")
      print
      exit
    }
  ' "$response"
)"
credential_kind="fine-grained"
if [ -n "$scopes" ]; then
  [ "$scopes" = "public_repo" ] || {
    err "classic HOMEBREW_PR_TOKEN must have only public_repo scope"
  }
  credential_kind="classic-public_repo"
fi

repository_state="$(tail -n 1 "$response")"
actual_repository="$(printf '%s\n' "$repository_state" | cut -f1)"
can_push="$(printf '%s\n' "$repository_state" | cut -f2)"
default_branch="$(printf '%s\n' "$repository_state" | cut -f3)"
[ "$actual_repository" = "$REPOSITORY" ] || {
  err "HOMEBREW_PR_TOKEN resolved unexpected repository $actual_repository"
}
[ "$can_push" = "true" ] || {
  err "HOMEBREW_PR_TOKEN owner does not have push permission to $REPOSITORY"
}
[ -n "$default_branch" ] || err "HOMEBREW_PR_TOKEN returned an empty default branch"

actor="$(
  GH_TOKEN="$TOKEN" gh api \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2026-03-10' \
    user \
    --jq '.login'
)" || err "HOMEBREW_PR_TOKEN could not resolve its owner"
[ -n "$actor" ] || err "HOMEBREW_PR_TOKEN returned an empty owner"

GH_TOKEN="$TOKEN" gh api \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2026-03-10' \
  "repos/$REPOSITORY/pulls?state=open&per_page=1" \
  --silent \
  || err "HOMEBREW_PR_TOKEN cannot access pull requests in $REPOSITORY"

if [ "$MODE" = "--canary" ]; then
  command -v git >/dev/null 2>&1 || err "git is required to run the Homebrew token canary"
  nonce="$(basename "$TMP_ROOT" | tr -cd 'A-Za-z0-9')"
  run_id="${GITHUB_RUN_ID:-manual-$(date +%s)-$nonce}"
  run_attempt="${GITHUB_RUN_ATTEMPT:-1}"
  case "$run_id-$run_attempt" in
    *[!A-Za-z0-9_-]*) err "invalid Homebrew token canary identity" ;;
  esac

  CANARY_BRANCH="automation/homebrew-token-canary-$run_id-$run_attempt"
  CANARY_ROOT="$TMP_ROOT/canary"
  canonical_remote="https://github.com/$REPOSITORY.git"
  askpass="$TMP_ROOT/git-askpass.sh"
  printf '%s\n' \
    '#!/bin/sh' \
    'case "$1" in' \
    '  *Username*) printf "%s\n" "x-access-token" ;;' \
    '  *) printf "%s\n" "$HOMEBREW_PR_TOKEN" ;;' \
    'esac' > "$askpass"
  chmod 700 "$askpass"
  export HOMEBREW_PR_TOKEN GIT_ASKPASS="$askpass" GIT_TERMINAL_PROMPT=0
  export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_GLOBAL=/dev/null
  export GIT_CONFIG_COUNT=0

  git_with_homebrew_token() {
    git -c credential.helper= -c http.extraHeader= "$@"
  }
  find_canary_pr_number() {
    owner="${REPOSITORY%%/*}"
    numbers="$(
      GH_TOKEN="$TOKEN" gh api \
        -H 'Accept: application/vnd.github+json' \
        -H 'X-GitHub-Api-Version: 2026-03-10' \
        "repos/$REPOSITORY/pulls?state=open&head=$owner:$CANARY_BRANCH&base=$default_branch&per_page=2" \
        --jq '.[].number'
    )" || return 1
    case "$numbers" in
      "") return 0 ;;
      *[!0-9]*) return 1 ;;
      *) printf '%s\n' "$numbers" ;;
    esac
  }
  close_canary_pr() {
    GH_TOKEN="$TOKEN" gh api \
      -X PATCH \
      -H 'Accept: application/vnd.github+json' \
      -H 'X-GitHub-Api-Version: 2026-03-10' \
      "repos/$REPOSITORY/pulls/$1" \
      -f state=closed \
      --silent
  }
  delete_canary_branch() {
    [ -n "$CANARY_COMMIT" ] || return 1
    remote_ref="$(
      git_with_homebrew_token -C "$CANARY_ROOT" ls-remote \
        --heads "$canonical_remote" "refs/heads/$CANARY_BRANCH"
    )" || return 1
    [ -n "$remote_ref" ] || return 0
    remote_sha="$(printf '%s\n' "$remote_ref" | awk 'NR == 1 { print $1 }')"
    [ "$remote_sha" = "$CANARY_COMMIT" ] || return 1
    git_with_homebrew_token -C "$CANARY_ROOT" push \
      --force-with-lease="refs/heads/$CANARY_BRANCH:$CANARY_COMMIT" \
      "$canonical_remote" ":refs/heads/$CANARY_BRANCH"
  }

  git_with_homebrew_token clone \
    --depth=1 \
    --no-tags \
    --single-branch \
    --branch "$default_branch" \
    "$canonical_remote" \
    "$CANARY_ROOT" >/dev/null \
    || err "HOMEBREW_PR_TOKEN cannot clone $REPOSITORY over canonical HTTPS"
  git -C "$CANARY_ROOT" checkout -b "$CANARY_BRANCH" >/dev/null \
    || err "could not create local Homebrew token canary branch"

  mkdir -p "$CANARY_ROOT/.dws"
  printf 'run=%s attempt=%s actor=%s\n' "$run_id" "$run_attempt" "$actor" \
    > "$CANARY_ROOT/.dws/homebrew-token-canary.txt"
  git -C "$CANARY_ROOT" config user.name "DWS Release Bot"
  git -C "$CANARY_ROOT" config user.email "dws-release-bot@example.com"
  git -C "$CANARY_ROOT" add .dws/homebrew-token-canary.txt
  git -C "$CANARY_ROOT" commit -m "chore: verify Homebrew token [skip ci]" >/dev/null \
    || err "could not create Homebrew token canary commit"
  CANARY_COMMIT="$(git -C "$CANARY_ROOT" rev-parse HEAD)"

  CANARY_BRANCH_MAY_EXIST="true"
  git_with_homebrew_token -C "$CANARY_ROOT" push \
    "$canonical_remote" "HEAD:refs/heads/$CANARY_BRANCH" >/dev/null \
    || err "HOMEBREW_PR_TOKEN cannot push a canary branch to $REPOSITORY"

  CANARY_PR_MAY_EXIST="true"
  canary_pr_url="$(
    GH_TOKEN="$TOKEN" gh pr create \
      --repo "$REPOSITORY" \
      --base "$default_branch" \
      --head "$CANARY_BRANCH" \
      --title "chore: verify Homebrew PR token" \
      --body "Automated permission canary. This PR is closed and its branch deleted by the same workflow run." \
      --draft
  )" || err "HOMEBREW_PR_TOKEN cannot create a canary pull request in $REPOSITORY"
  CANARY_PR_NUMBER="${canary_pr_url##*/}"
  case "$CANARY_PR_NUMBER" in
    ""|*[!0-9]*) err "Homebrew token canary returned an invalid pull request URL" ;;
  esac

  close_canary_pr "$CANARY_PR_NUMBER" \
    || err "HOMEBREW_PR_TOKEN could not close canary PR #$CANARY_PR_NUMBER"
  CANARY_PR_MAY_EXIST="false"
  CANARY_PR_NUMBER=""

  delete_canary_branch >/dev/null \
    || err "HOMEBREW_PR_TOKEN could not delete canary branch $CANARY_BRANCH"
  CANARY_BRANCH_MAY_EXIST="false"
fi

printf 'Homebrew PR token capability passed for %s as %s (%s)\n' \
  "$REPOSITORY" "$actor" "$credential_kind"
