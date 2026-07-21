#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"

VERSION=""
FROM_BETA=""
REMOTE=""
MODE="check"
MODE_SET=0
WIZARD=0

usage() {
  cat >&2 <<'EOF'
usage: dws-release [version] [options]
       dws-release config [--remote <name>]
       dws-release recover <version> [--failed-run <run-id>] [--failed-attempt <attempt>] [--remote <name>]

With no arguments, starts an interactive release guide. For a version that has
no CHANGELOG section yet, prepares the template and stops. Otherwise, runs the
full guarded preflight; add --publish to publish after the preflight succeeds.

Options:
  --from-beta <tag>   Required stable promotion baseline
  --remote <name>     Override the configured release remote
  --check             Validate only (default)
  --publish           Run the preflight, then create and push the tag
  -h, --help           Show this help

Examples:
  dws-release config --remote origin
  dws-release v1.2.3-beta.1
  dws-release v1.2.3-beta.1 --publish
  dws-release v1.2.3 --from-beta v1.2.3-beta.1
  dws-release v1.2.3 --from-beta v1.2.3-beta.1 --publish
  dws-release recover v1.2.3-beta.1
EOF
}

die_usage() {
  printf '%s\n' "$1" >&2
  usage
  exit 2
}

is_interactive() {
  [ -t 0 ] && [ -t 1 ]
}

repository_identity() {
  identity_url="$1"
  case "$identity_url" in
    https://github.com/*) identity_path="${identity_url#https://github.com/}" ;;
    http://github.com/*) identity_path="${identity_url#http://github.com/}" ;;
    git://github.com/*) identity_path="${identity_url#git://github.com/}" ;;
    git@github.com:*) identity_path="${identity_url#git@github.com:}" ;;
    ssh://git@github.com/*) identity_path="${identity_url#ssh://git@github.com/}" ;;
    *) printf '%s\n' "$identity_url"; return 0 ;;
  esac
  identity_path="${identity_path%/}"
  identity_path="${identity_path%.git}"
  printf 'github.com/%s\n' "$identity_path"
}

release_remote_identity() {
  identity_remote="$1"
  identity_fetch_url="$(git remote get-url "$identity_remote" 2>/dev/null)" || {
    printf 'configured release remote does not exist: %s\n' "$identity_remote" >&2
    return 1
  }
  identity_push_urls="$(git remote get-url --push --all "$identity_remote" 2>/dev/null)" || {
    printf 'could not resolve push URL for release remote: %s\n' "$identity_remote" >&2
    return 1
  }
  identity_push_count="$(printf '%s\n' "$identity_push_urls" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
  [ "$identity_push_count" -eq 1 ] || {
    printf 'release remote must have exactly one push URL: %s\n' "$identity_remote" >&2
    return 1
  }
  identity_push_url="$(printf '%s\n' "$identity_push_urls" | sed -n '1p')"
  identity_fetch="$(repository_identity "$identity_fetch_url")"
  identity_push="$(repository_identity "$identity_push_url")"
  [ "$identity_fetch" = "$identity_push" ] || {
    printf 'release remote fetch and push URLs target different repositories:\n  fetch: %s\n  push:  %s\n' \
      "$identity_fetch_url" "$identity_push_url" >&2
    return 1
  }
  printf '%s\n' "$identity_fetch"
}

set_mode() {
  requested_mode="$1"
  if [ "$MODE_SET" -eq 1 ] && [ "$MODE" != "$requested_mode" ]; then
    die_usage '--check and --publish cannot be used together'
  fi
  MODE="$requested_mode"
  MODE_SET=1
}

require_remote() {
  if [ -z "$REMOTE" ]; then
    REMOTE="$(git config --local --get dws.releaseRemote 2>/dev/null || true)"
  fi
  if [ -z "$REMOTE" ] && is_interactive; then
    printf 'Configured remotes:\n' >&2
    git remote -v >&2
    printf 'Release remote: ' >&2
    IFS= read -r REMOTE
  fi
  [ -n "$REMOTE" ] || {
    printf '%s\n' 'release remote is not configured; run: dws-release config --remote <name>' >&2
    exit 1
  }
  expected_repository="$(git config --local --get dws.releaseRepository 2>/dev/null || true)"
  [ -n "$expected_repository" ] || {
    printf '%s\n' 'release repository identity is not configured; rerun: dws-release config --remote <name>' >&2
    exit 1
  }
  actual_repository="$(release_remote_identity "$REMOTE")" || exit 1
  [ "$actual_repository" = "$expected_repository" ] || {
    printf 'release remote %s changed repository identity:\n  configured: %s\n  current:    %s\n' \
      "$REMOTE" "$expected_repository" "$actual_repository" >&2
    printf '%s\n' 'Review the remote and run dws-release config again only if this change is intentional.' >&2
    exit 1
  }
}

sync_main_if_safe() {
  [ -z "$(git status --porcelain --untracked-files=all)" ] || {
    printf '%s\n' 'release worktree must be clean before synchronization' >&2
    exit 1
  }

  printf '==> Synchronizing %s/main\n' "$REMOTE"
  git fetch --force "$REMOTE" "+refs/heads/main:refs/remotes/$REMOTE/main"
  remote_main="refs/remotes/$REMOTE/main"
  head_commit="$(git rev-parse HEAD)"
  remote_commit="$(git rev-parse "$remote_main^{commit}")"
  if [ "$head_commit" = "$remote_commit" ]; then
    return 0
  fi
  current_branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  if [ "$current_branch" != "main" ]; then
    if git merge-base --is-ancestor HEAD "$remote_main"; then
      return 0
    fi
    printf 'HEAD is not contained in %s/main history; merge it through a reviewed PR before release\n' "$REMOTE" >&2
    exit 1
  fi
  if git merge-base --is-ancestor HEAD "$remote_main"; then
    git merge --ff-only "$remote_main"
    return 0
  fi
  if git merge-base --is-ancestor "$remote_main" HEAD; then
    printf 'local main is ahead of %s/main; publish only after the commits are reviewed and pushed\n' "$REMOTE" >&2
  else
    printf 'local main has diverged from %s/main; resolve it explicitly before release\n' "$REMOTE" >&2
  fi
  exit 1
}

configure_remote() {
  shift
  config_remote=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --remote)
        [ "$#" -ge 2 ] || die_usage '--remote requires a value'
        config_remote="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *) die_usage "unknown config argument: $1" ;;
    esac
  done

  cd "$ROOT"
  if [ -z "$config_remote" ]; then
    configured="$(git config --local --get dws.releaseRemote 2>/dev/null || true)"
    configured_repository="$(git config --local --get dws.releaseRepository 2>/dev/null || true)"
    printf 'Configured release remote: %s\n' "${configured:-none}"
    printf 'Configured repository:     %s\n' "${configured_repository:-none}"
    git remote -v
    exit 0
  fi
  configured_repository="$(release_remote_identity "$config_remote")" || exit 1
  git config --local dws.releaseRemote "$config_remote"
  git config --local dws.releaseRepository "$configured_repository"
  printf 'Configured release remote: %s\n' "$config_remote"
  printf 'Configured repository:     %s\n' "$configured_repository"
  exit 0
}

recover_release() {
  shift
  recovery_version="${1:-}"
  [ -n "$recovery_version" ] || die_usage 'recover requires a release version'
  case "$recovery_version" in -h|--help) usage; exit 0 ;; esac
  shift
  recovery_failed_run=""
  recovery_failed_attempt=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --failed-run)
        [ "$#" -ge 2 ] || die_usage '--failed-run requires a workflow run ID'
        recovery_failed_run="$2"
        shift 2
        ;;
      --failed-attempt)
        [ "$#" -ge 2 ] || die_usage '--failed-attempt requires a run attempt'
        recovery_failed_attempt="$2"
        shift 2
        ;;
      --remote)
        [ "$#" -ge 2 ] || die_usage '--remote requires a value'
        REMOTE="$2"
        shift 2
        ;;
      -h|--help) usage; exit 0 ;;
      *) die_usage "unknown recovery argument: $1" ;;
    esac
  done

  cd "$ROOT"
  require_remote
  sync_main_if_safe
  set -- "$recovery_version" --remote "$REMOTE"
  if [ -n "$recovery_failed_run" ]; then
    set -- "$@" --failed-run "$recovery_failed_run"
  fi
  if [ -n "$recovery_failed_attempt" ]; then
    set -- "$@" --failed-attempt "$recovery_failed_attempt"
  fi
  exec "$SCRIPT_DIR/recover-release.sh" "$@"
}

if [ "${1:-}" = "config" ]; then
  configure_remote "$@"
fi
if [ "${1:-}" = "recover" ]; then
  recover_release "$@"
fi

[ "$#" -gt 0 ] || WIZARD=1
while [ "$#" -gt 0 ]; do
  case "$1" in
    --from-beta)
      [ "$#" -ge 2 ] || die_usage '--from-beta requires a value'
      FROM_BETA="$2"
      shift 2
      ;;
    --remote)
      [ "$#" -ge 2 ] || die_usage '--remote requires a value'
      REMOTE="$2"
      shift 2
      ;;
    --check) set_mode check; shift ;;
    --publish) set_mode publish; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) die_usage "unknown argument: $1" ;;
    *)
      [ -z "$VERSION" ] || die_usage "unexpected argument: $1"
      VERSION="$1"
      shift
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  is_interactive || { usage; exit 2; }
  printf 'Release version (vX.Y.Z-beta.N or vX.Y.Z): ' >&2
  IFS= read -r VERSION
fi

CHANNEL="$(release_channel_for_version "$VERSION")" || exit 2
if [ "$CHANNEL" = "stable" ]; then
  if [ -z "$FROM_BETA" ] && is_interactive; then
    printf 'Validated beta tag for %s: ' "$VERSION" >&2
    IFS= read -r FROM_BETA
  fi
  [ -n "$FROM_BETA" ] || die_usage 'stable release requires --from-beta <tag>'
  release_validate_version_channel prerelease "$FROM_BETA" || exit 2
  [ "$(release_core_tag "$FROM_BETA")" = "$VERSION" ] || {
    printf 'stable version %s does not match beta baseline %s\n' "$VERSION" "$FROM_BETA" >&2
    exit 2
  }
else
  [ -z "$FROM_BETA" ] || die_usage '--from-beta is only valid for stable releases'
fi

if [ "$WIZARD" -eq 1 ]; then
  printf 'Mode [check/publish] (default: check): ' >&2
  IFS= read -r selected_mode
  case "$selected_mode" in
    ''|check) MODE=check ;;
    publish) MODE=publish ;;
    *) die_usage "invalid mode: $selected_mode" ;;
  esac
fi
[ -z "${DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE:-}" ] || {
  printf '%s\n' 'DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE is test-only and cannot be set through dws-release' >&2
  exit 1
}
[ -z "${DWS_RELEASE_OFFICIAL_TAGS_URL:-}" ] || {
  printf '%s\n' 'DWS_RELEASE_OFFICIAL_TAGS_URL cannot override release authority through dws-release' >&2
  exit 1
}
[ -z "${DWS_RELEASE_OFFICIAL_REPOSITORY:-}" ] || {
  printf '%s\n' 'DWS_RELEASE_OFFICIAL_REPOSITORY cannot override release authority through dws-release' >&2
  exit 1
}

cd "$ROOT"
CHANGELOG="$ROOT/CHANGELOG.md"
[ -f "$CHANGELOG" ] || { printf 'CHANGELOG not found: %s\n' "$CHANGELOG" >&2; exit 1; }
MAIN_SYNCED=0
current_branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
if [ "$current_branch" = "main" ]; then
  require_remote
  sync_main_if_safe
  MAIN_SYNCED=1
fi
semver="$(release_semver "$VERSION")"
section_count="$(awk -v wanted="$semver" '
  BEGIN { prefix = "## [" wanted "] - "; count = 0 }
  index($0, prefix) == 1 { count++ }
  END { print count }
' "$CHANGELOG")"

if [ "$section_count" -eq 0 ]; then
  printf '==> Preparing the missing CHANGELOG section for %s\n' "$VERSION"
  if [ "$CHANNEL" = "stable" ]; then
    "$SCRIPT_DIR/prepare-changelog.sh" stable "$VERSION" --from-beta "$FROM_BETA"
  else
    "$SCRIPT_DIR/prepare-changelog.sh" prerelease "$VERSION"
  fi
  printf '\nCHANGELOG preparation is complete; publishing intentionally stopped.\n'
  printf 'Replace TODO, review the diff, commit it, and merge it to main. Then run dws-release for %s again.\n' "$VERSION"
  exit 0
fi

notes_tmp="$(mktemp "${TMPDIR:-/tmp}/dws-release-entry.XXXXXX")"
cleanup() { rm -f "$notes_tmp"; }
trap cleanup EXIT HUP INT TERM
release_extract_changelog "$CHANGELOG" "$semver" "$notes_tmp"

if [ "$MAIN_SYNCED" -ne 1 ]; then
  require_remote
  sync_main_if_safe
fi

printf '==> DWS release entry\n'
printf '  mode:    %s\n' "$MODE"
printf '  channel: %s\n' "$CHANNEL"
printf '  version: %s\n' "$VERSION"
printf '  remote:  %s\n' "$REMOTE"
[ -z "$FROM_BETA" ] || printf '  beta:    %s\n' "$FROM_BETA"

set -- "$CHANNEL" "$VERSION" --remote "$REMOTE"
if [ -n "$FROM_BETA" ]; then
  set -- "$@" --from-beta "$FROM_BETA"
fi
if [ "$MODE" = "publish" ]; then
  set -- "$@" --publish
fi
cleanup
trap - EXIT HUP INT TERM
exec "$SCRIPT_DIR/release.sh" "$@"
