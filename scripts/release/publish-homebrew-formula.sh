#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
FORMULA_SOURCE="${DWS_FORMULA_SOURCE:-$ROOT/dist/homebrew/dingtalk-workspace-cli.rb}"
TAP_REPO_URL="${DWS_TAP_REPO_URL:-}"
TAP_BRANCH="${DWS_TAP_BRANCH:-main}"
TAP_FORMULA_PATH="${DWS_TAP_FORMULA_PATH:-Formula/dingtalk-workspace-cli.rb}"
TAP_GITHUB_TOKEN="${DWS_TAP_GITHUB_TOKEN:-}"
TAP_SSH_KEY="${DWS_TAP_SSH_KEY:-}"
PR_REPOSITORY="${DWS_TAP_PR_REPOSITORY:-}"
PR_BRANCH="${DWS_TAP_PR_BRANCH:-}"
PR_TITLE="${DWS_TAP_PR_TITLE:-chore: update Homebrew formula}"
COMMIT_MESSAGE="${DWS_TAP_COMMIT_MESSAGE:-chore: update dingtalk-workspace-cli formula}"
GIT_NAME="${DWS_GIT_NAME:-DWS Release Bot}"
GIT_EMAIL="${DWS_GIT_EMAIL:-dws-release-bot@example.com}"

say() {
  printf '%s\n' "$*"
}

err() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

need_file() {
  [ -f "$1" ] || err "required file not found: $1"
}

need_env() {
  name="$1"
  value="$2"
  [ -n "$value" ] || err "missing required environment variable: $name"
}

checkout_tap_branch() {
  repo_url="$1"
  branch="$2"
  target_dir="$3"

  if git clone --branch "$branch" "$repo_url" "$target_dir" >/dev/null 2>&1; then
    return
  fi

  git clone "$repo_url" "$target_dir" >/dev/null 2>&1
  (
    cd "$target_dir"
    if git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
      git checkout -B "$branch" "origin/$branch" >/dev/null 2>&1
      exit 0
    fi
    if git show-ref --verify --quiet "refs/heads/$branch"; then
      git checkout "$branch" >/dev/null 2>&1
      exit 0
    fi
    git checkout --orphan "$branch" >/dev/null 2>&1
  )
}

need_cmd git
need_cmd ruby
need_env "DWS_TAP_REPO_URL" "$TAP_REPO_URL"
need_file "$FORMULA_SOURCE"

if grep -Eq '__[A-Z0-9_]+__' "$FORMULA_SOURCE"; then
  err "formula contains unresolved template placeholders: $FORMULA_SOURCE"
fi
ruby -c "$FORMULA_SOURCE" >/dev/null || err "formula has invalid Ruby syntax: $FORMULA_SOURCE"

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-homebrew-publish-XXXXXX")"
cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

if [ -n "$TAP_SSH_KEY" ]; then
  SSH_KEY_PATH="$TMP_ROOT/tap-deploy-key"
  printf '%s\n' "$TAP_SSH_KEY" > "$SSH_KEY_PATH"
  chmod 600 "$SSH_KEY_PATH"
  export GIT_SSH_COMMAND="ssh -i $SSH_KEY_PATH -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"
  export GIT_TERMINAL_PROMPT=0
fi
if [ -n "$TAP_GITHUB_TOKEN" ]; then
  ASKPASS="$TMP_ROOT/git-askpass.sh"
  printf '%s\n' \
    '#!/bin/sh' \
    'case "$1" in' \
    '  *Username*) printf "%s\n" "x-access-token" ;;' \
    '  *) printf "%s\n" "$DWS_TAP_GITHUB_TOKEN" ;;' \
    'esac' > "$ASKPASS"
  chmod 700 "$ASKPASS"
  export DWS_TAP_GITHUB_TOKEN GIT_ASKPASS="$ASKPASS"
  export GIT_TERMINAL_PROMPT=0
fi

if [ -n "$PR_REPOSITORY" ] || [ -n "$PR_BRANCH" ]; then
  need_env "DWS_TAP_PR_REPOSITORY" "$PR_REPOSITORY"
  need_env "DWS_TAP_PR_BRANCH" "$PR_BRANCH"
  need_env "DWS_TAP_GITHUB_TOKEN" "$TAP_GITHUB_TOKEN"
  need_cmd gh
fi

TAP_DIR="$TMP_ROOT/tap"
checkout_tap_branch "$TAP_REPO_URL" "$TAP_BRANCH" "$TAP_DIR"

DEST_PATH="$TAP_DIR/$TAP_FORMULA_PATH"
mkdir -p "$(dirname "$DEST_PATH")"
cp "$FORMULA_SOURCE" "$DEST_PATH"

(
  cd "$TAP_DIR"
  if [ -z "$(git status --short -- "$TAP_FORMULA_PATH")" ]; then
    say "No formula changes to publish."
    exit 0
  fi

  git config user.name "$GIT_NAME"
  git config user.email "$GIT_EMAIL"
  git add "$TAP_FORMULA_PATH"
  git commit -m "$COMMIT_MESSAGE" >/dev/null

  if [ -n "$PR_REPOSITORY" ]; then
    git push --force-with-lease origin "HEAD:$PR_BRANCH" >/dev/null
    pr_url="$(
      GH_TOKEN="$TAP_GITHUB_TOKEN" gh pr list \
        --repo "$PR_REPOSITORY" \
        --head "$PR_BRANCH" \
        --state open \
        --json url \
        --jq '.[0].url'
    )"
    if [ -z "$pr_url" ]; then
      pr_url="$(
        GH_TOKEN="$TAP_GITHUB_TOKEN" gh pr create \
          --repo "$PR_REPOSITORY" \
          --base "$TAP_BRANCH" \
          --head "$PR_BRANCH" \
          --title "$PR_TITLE" \
          --body "Automated Homebrew Formula update. Merge after required checks pass."
      )"
    fi
    say "Opened Homebrew formula PR: $pr_url"
    exit 0
  fi

  git push origin "HEAD:$TAP_BRANCH" >/dev/null
)

if [ -z "$PR_REPOSITORY" ]; then
  say "Published Homebrew formula to $TAP_REPO_URL ($TAP_BRANCH)"
fi
