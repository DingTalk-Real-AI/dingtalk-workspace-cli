#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"

ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
CHANNEL=""
VERSION=""
CONTEXT="local"
REMOTE="origin"
BRANCH="main"
FROM_BETA=""
FROM_BETA_COMMIT=""
CHANGELOG=""
NOTES_OUTPUT=""
METADATA_OUTPUT=""

usage() {
  cat >&2 <<'EOF'
usage: release-contract.sh --channel <prerelease|stable> --version <tag> [options]

Options:
  --context <local|ci>       Local pre-push checks or tag-workflow checks
  --remote <name>            Release remote (default: origin)
  --branch <name>            Sealed release branch (default: main)
  --from-beta <tag>          Required stable promotion baseline
  --repo-root <path>         Override repository root (primarily for tests)
  --changelog <path>         Override CHANGELOG.md path
  --notes-output <path>      Write the exact CHANGELOG section body
  --metadata-output <path>   Append channel/version/baseline key-value output
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --channel) [ "$#" -ge 2 ] || { usage; exit 2; }; CHANNEL="$2"; shift 2 ;;
    --version) [ "$#" -ge 2 ] || { usage; exit 2; }; VERSION="$2"; shift 2 ;;
    --context) [ "$#" -ge 2 ] || { usage; exit 2; }; CONTEXT="$2"; shift 2 ;;
    --remote) [ "$#" -ge 2 ] || { usage; exit 2; }; REMOTE="$2"; shift 2 ;;
    --branch) [ "$#" -ge 2 ] || { usage; exit 2; }; BRANCH="$2"; shift 2 ;;
    --from-beta) [ "$#" -ge 2 ] || { usage; exit 2; }; FROM_BETA="$2"; shift 2 ;;
    --repo-root) [ "$#" -ge 2 ] || { usage; exit 2; }; ROOT="$2"; shift 2 ;;
    --changelog) [ "$#" -ge 2 ] || { usage; exit 2; }; CHANGELOG="$2"; shift 2 ;;
    --notes-output) [ "$#" -ge 2 ] || { usage; exit 2; }; NOTES_OUTPUT="$2"; shift 2 ;;
    --metadata-output) [ "$#" -ge 2 ] || { usage; exit 2; }; METADATA_OUTPUT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; usage; exit 2 ;;
  esac
done

[ -n "$CHANNEL" ] && [ -n "$VERSION" ] || { usage; exit 2; }
case "$CONTEXT" in local|ci) ;; *) printf 'invalid context: %s\n' "$CONTEXT" >&2; exit 2 ;; esac
release_validate_version_channel "$CHANNEL" "$VERSION"

cd "$ROOT"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  printf 'not a Git worktree: %s\n' "$ROOT" >&2
  exit 1
}
[ -n "$CHANGELOG" ] || CHANGELOG="$ROOT/CHANGELOG.md"
[ -f "$CHANGELOG" ] || { printf 'CHANGELOG not found: %s\n' "$CHANGELOG" >&2; exit 1; }

head_commit="$(git rev-parse HEAD)"
remote_main="refs/remotes/$REMOTE/$BRANCH"
git remote get-url "$REMOTE" >/dev/null 2>&1 || {
  printf 'release remote does not exist: %s\n' "$REMOTE" >&2
  exit 1
}
git rev-parse --verify --quiet "$remote_main^{commit}" >/dev/null || {
  printf 'release branch is not available locally: %s/%s\n' "$REMOTE" "$BRANCH" >&2
  exit 1
}
remote_main_commit="$(git rev-parse "$remote_main^{commit}")"

if [ "$CONTEXT" = "local" ]; then
  [ -z "$(git status --porcelain --untracked-files=all)" ] || {
    printf 'release worktree must be clean (staged, unstaged, and untracked files are blocked)\n' >&2
    exit 1
  }
  current_branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  [ "$current_branch" = "$BRANCH" ] || {
    printf 'local release must run from branch %s (current: %s)\n' "$BRANCH" "${current_branch:-detached HEAD}" >&2
    exit 1
  }
  [ "$head_commit" = "$remote_main_commit" ] || {
    printf 'HEAD must exactly match %s/%s before release\n' "$REMOTE" "$BRANCH" >&2
    exit 1
  }
  if git rev-parse --verify --quiet "refs/tags/$VERSION" >/dev/null; then
    printf 'release tag already exists locally: %s\n' "$VERSION" >&2
    exit 1
  fi
  remote_tag="$(git ls-remote --tags "$REMOTE" "refs/tags/$VERSION" "refs/tags/$VERSION^{}")" || {
    printf 'could not query release tags from remote: %s\n' "$REMOTE" >&2
    exit 1
  }
  [ -z "$remote_tag" ] || {
    printf 'release tag already exists on %s: %s\n' "$REMOTE" "$VERSION" >&2
    exit 1
  }
else
  git rev-parse --verify --quiet "refs/tags/$VERSION^{commit}" >/dev/null || {
    printf 'CI release tag is not available: %s\n' "$VERSION" >&2
    exit 1
  }
  [ "$(git rev-parse "refs/tags/$VERSION^{commit}")" = "$head_commit" ] || {
    printf 'CI checkout HEAD does not match tag %s\n' "$VERSION" >&2
    exit 1
  }
  [ "$(git cat-file -t "refs/tags/$VERSION")" = "tag" ] || {
    printf 'release tag must be annotated: %s\n' "$VERSION" >&2
    exit 1
  }
  git merge-base --is-ancestor HEAD "$remote_main" || {
    printf 'release tag commit must be contained in %s/%s\n' "$REMOTE" "$BRANCH" >&2
    exit 1
  }
fi

previous_stable=""
previous_stable_commit=""
for tag in $(git tag --list 'v*' --sort=-version:refname); do
  [ "$tag" = "$VERSION" ] && continue
  if release_is_stable_version "$tag"; then
    previous_stable="$tag"
    previous_stable_commit="$(git rev-parse "$tag^{commit}")"
    break
  fi
done
if [ -n "$previous_stable" ] && ! release_core_is_greater "$VERSION" "$previous_stable"; then
  printf 'release version %s must be greater than latest stable %s\n' "$VERSION" "$previous_stable" >&2
  exit 1
fi

core_tag="$(release_core_tag "$VERSION")"
if [ "$CHANNEL" = "prerelease" ]; then
  [ -z "$FROM_BETA" ] || { printf -- '--from-beta is only valid for stable releases\n' >&2; exit 1; }
  if git rev-parse --verify --quiet "refs/tags/$core_tag" >/dev/null; then
    printf 'cannot publish prerelease after stable tag exists: %s\n' "$core_tag" >&2
    exit 1
  fi
  previous_beta=""
  for tag in $(git tag --list "$core_tag-beta.*" --sort=-version:refname); do
    [ "$tag" = "$VERSION" ] && continue
    if release_is_prerelease_version "$tag"; then
      previous_beta="$tag"
      break
    fi
  done
  beta_number="$(release_beta_number "$VERSION")"
  if [ -z "$previous_beta" ]; then
    [ "$beta_number" -eq 1 ] || {
      printf 'the first prerelease for %s must be beta.1\n' "$core_tag" >&2
      exit 1
    }
  else
    previous_beta_number="$(release_beta_number "$previous_beta")"
    expected_beta_number=$((previous_beta_number + 1))
    [ "$beta_number" -eq "$expected_beta_number" ] || {
      printf 'prerelease after %s must be %s-beta.%s\n' "$previous_beta" "$core_tag" "$expected_beta_number" >&2
      exit 1
    }
  fi
else
  if [ -z "$FROM_BETA" ] && [ "$CONTEXT" = "ci" ]; then
    FROM_BETA="$(git for-each-ref "refs/tags/$VERSION" --format='%(contents)' | awk '/^From-Beta:[[:space:]]*/ { sub(/^From-Beta:[[:space:]]*/, ""); print }')"
  fi
  [ -n "$FROM_BETA" ] || {
    printf 'stable release requires an explicit --from-beta tag\n' >&2
    exit 1
  }
  release_is_prerelease_version "$FROM_BETA" || {
    printf 'invalid stable beta baseline: %s\n' "$FROM_BETA" >&2
    exit 1
  }
  [ "$(release_core_tag "$FROM_BETA")" = "$VERSION" ] || {
    printf 'stable version %s does not match beta baseline %s\n' "$VERSION" "$FROM_BETA" >&2
    exit 1
  }
  git rev-parse --verify --quiet "refs/tags/$FROM_BETA^{commit}" >/dev/null || {
    printf 'stable beta baseline is not available locally: %s\n' "$FROM_BETA" >&2
    exit 1
  }
  FROM_BETA_COMMIT="$(git rev-parse "refs/tags/$FROM_BETA^{commit}")"
  git merge-base --is-ancestor "$FROM_BETA^{commit}" HEAD || {
    printf 'stable beta baseline is not an ancestor of HEAD: %s\n' "$FROM_BETA" >&2
    exit 1
  }
  if ! git diff --quiet "$FROM_BETA^{commit}" HEAD -- . ':(exclude)CHANGELOG.md'; then
    printf 'stable source drifted from %s; only CHANGELOG.md may differ\n' "$FROM_BETA" >&2
    git diff --name-only "$FROM_BETA^{commit}" HEAD -- . ':(exclude)CHANGELOG.md' >&2
    exit 1
  fi
fi

semver="$(release_semver "$VERSION")"
if [ -n "$NOTES_OUTPUT" ]; then
  release_extract_changelog "$CHANGELOG" "$semver" "$NOTES_OUTPUT"
else
  notes_tmp="$(mktemp "${TMPDIR:-/tmp}/dws-release-contract.XXXXXX")"
  trap 'rm -f "$notes_tmp"' EXIT HUP INT TERM
  release_extract_changelog "$CHANGELOG" "$semver" "$notes_tmp"
fi

if [ -n "$METADATA_OUTPUT" ]; then
  mkdir -p "$(dirname "$METADATA_OUTPUT")"
  {
    printf 'channel=%s\n' "$CHANNEL"
    printf 'version=%s\n' "$VERSION"
    printf 'semver=%s\n' "$semver"
    printf 'previous_stable=%s\n' "$previous_stable"
    printf 'previous_stable_commit=%s\n' "$previous_stable_commit"
    printf 'from_beta=%s\n' "$FROM_BETA"
    printf 'from_beta_commit=%s\n' "$FROM_BETA_COMMIT"
  } >> "$METADATA_OUTPUT"
fi

printf 'Release contract passed: channel=%s version=%s commit=%s' "$CHANNEL" "$VERSION" "$head_commit"
[ -z "$FROM_BETA" ] || printf ' from-beta=%s' "$FROM_BETA"
printf '\n'
