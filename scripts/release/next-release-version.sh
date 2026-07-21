#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"

ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
CHANNEL=""
REQUESTED_CHANNEL=""
BUMP="patch"

usage() {
  cat >&2 <<'EOF'
usage: next-release-version.sh --channel <beta|prerelease|stable> [options]

Options:
  --bump <patch|minor|major>  Core bump when starting a new beta line (default: patch)
  --repo-root <path>          Override repository root (primarily for tests)

Output:
  release_version=<vX.Y.Z[-beta.N]>
  from_beta=<vX.Y.Z-beta.N or empty>
  channel=<prerelease|stable>
  base=<latest allocated stable tag or empty>

Both ordinary release tags and refs/tags/withdrawn/v... tombstones reserve a
version permanently. The caller must create the returned release tag
atomically; this script only calculates the next candidate.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --channel)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      REQUESTED_CHANNEL="$2"
      shift 2
      ;;
    --bump)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      BUMP="$2"
      shift 2
      ;;
    --repo-root)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      ROOT="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

case "$REQUESTED_CHANNEL" in
  beta|prerelease) CHANNEL="prerelease" ;;
  stable) CHANNEL="stable" ;;
  *)
    printf 'invalid release channel: %s (expected beta, prerelease, or stable)\n' "${REQUESTED_CHANNEL:-<empty>}" >&2
    exit 2
    ;;
esac
case "$BUMP" in
  patch|minor|major) ;;
  *)
    printf 'invalid version bump: %s (expected patch, minor, or major)\n' "$BUMP" >&2
    exit 2
    ;;
esac

cd "$ROOT"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  printf 'not a Git worktree: %s\n' "$ROOT" >&2
  exit 1
}

allocated_version_for_ref() {
  _avfr_ref="$1"
  case "$_avfr_ref" in
    refs/tags/withdrawn/v*) printf '%s\n' "${_avfr_ref#refs/tags/withdrawn/}" ;;
    refs/tags/v*) printf '%s\n' "${_avfr_ref#refs/tags/}" ;;
    *) return 1 ;;
  esac
}

stable_core_is_allocated() {
  _scia_core="$1"
  git show-ref --verify --quiet "refs/tags/$_scia_core" ||
    git show-ref --verify --quiet "refs/tags/withdrawn/$_scia_core"
}

version_is_withdrawn() {
  git show-ref --verify --quiet "refs/tags/withdrawn/$1"
}

bump_core() {
  _bc_version="${1#v}"
  _bc_major="${_bc_version%%.*}"
  _bc_remainder="${_bc_version#*.}"
  _bc_minor="${_bc_remainder%%.*}"
  _bc_patch="${_bc_remainder#*.}"
  case "$2" in
    patch) _bc_patch=$((_bc_patch + 1)) ;;
    minor)
      _bc_minor=$((_bc_minor + 1))
      _bc_patch=0
      ;;
    major)
      _bc_major=$((_bc_major + 1))
      _bc_minor=0
      _bc_patch=0
      ;;
  esac
  printf 'v%s.%s.%s\n' "$_bc_major" "$_bc_minor" "$_bc_patch"
}

tag_refs="$(git for-each-ref --format='%(refname)' refs/tags)"
latest_stable=""
for ref in $tag_refs; do
  version="$(allocated_version_for_ref "$ref" 2>/dev/null || true)"
  [ -n "$version" ] || continue
  if release_is_stable_version "$version"; then
    if [ -z "$latest_stable" ] || release_core_is_greater "$version" "$latest_stable"; then
      latest_stable="$version"
    fi
  fi
done

highest_open_beta_core=""
for ref in $tag_refs; do
  version="$(allocated_version_for_ref "$ref" 2>/dev/null || true)"
  [ -n "$version" ] || continue
  release_is_prerelease_version "$version" || continue
  core="$(release_core_tag "$version")"
  stable_core_is_allocated "$core" && continue
  if [ -n "$latest_stable" ] && ! release_core_is_greater "$core" "$latest_stable"; then
    continue
  fi
  if [ -z "$highest_open_beta_core" ] || release_core_is_greater "$core" "$highest_open_beta_core"; then
    highest_open_beta_core="$core"
  fi
done

release_version=""
from_beta=""
base="$latest_stable"

if [ "$CHANNEL" = "prerelease" ]; then
  if [ -n "$highest_open_beta_core" ]; then
    latest_beta=""
    for ref in $tag_refs; do
      version="$(allocated_version_for_ref "$ref" 2>/dev/null || true)"
      [ -n "$version" ] || continue
      release_is_prerelease_version "$version" || continue
      [ "$(release_core_tag "$version")" = "$highest_open_beta_core" ] || continue
      if [ -z "$latest_beta" ] || release_version_is_greater "$version" "$latest_beta"; then
        latest_beta="$version"
      fi
    done
    next_beta_number=$(( $(release_beta_number "$latest_beta") + 1 ))
    release_version="$highest_open_beta_core-beta.$next_beta_number"
  else
    [ -n "$latest_stable" ] || {
      printf 'cannot start a beta line without an allocated stable baseline\n' >&2
      exit 1
    }
    next_core="$(bump_core "$latest_stable" "$BUMP")"
    release_version="$next_core-beta.1"
  fi
else
  [ -n "$highest_open_beta_core" ] || {
    printf 'cannot create a stable release without an open beta line newer than the latest allocated stable\n' >&2
    exit 1
  }
  latest_beta=""
  for ref in $tag_refs; do
    version="$(allocated_version_for_ref "$ref" 2>/dev/null || true)"
    [ -n "$version" ] || continue
    release_is_prerelease_version "$version" || continue
    [ "$(release_core_tag "$version")" = "$highest_open_beta_core" ] || continue
    if [ -z "$latest_beta" ] || release_version_is_greater "$version" "$latest_beta"; then
      latest_beta="$version"
    fi
  done
  if version_is_withdrawn "$latest_beta" ||
    ! git show-ref --verify --quiet "refs/tags/$latest_beta"; then
    printf 'latest beta %s is withdrawn; create the next beta before stable promotion\n' "$latest_beta" >&2
    exit 1
  fi
  release_version="$highest_open_beta_core"
  from_beta="$latest_beta"
fi

printf 'release_version=%s\n' "$release_version"
printf 'from_beta=%s\n' "$from_beta"
printf 'channel=%s\n' "$CHANNEL"
printf 'base=%s\n' "$base"
