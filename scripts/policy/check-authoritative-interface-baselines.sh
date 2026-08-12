#!/bin/sh
set -eu

# Compatibility decisions are centralized in check-command-compatibility.sh.
# This historical entry point remains as the Makefile/user-facing wrapper.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""
STABLE_REF=""
CANDIDATE_REF="HEAD"

usage() {
  printf '%s\n' "usage: $0 --base-ref <ref> [--stable-ref <ref>] [--candidate-ref <ref>]" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      BASE_REF="$2"
      shift 2
      ;;
    --stable-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      STABLE_REF="$2"
      shift 2
      ;;
    --candidate-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      CANDIDATE_REF="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'error: unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

[ -n "$BASE_REF" ] || { usage; exit 2; }
cd "$ROOT"
. "$ROOT/scripts/release/release-lib.sh"

git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
  printf 'error: interface authority is not available locally: %s\n' "$BASE_REF" >&2
  exit 2
}

if [ -z "$STABLE_REF" ]; then
  for tag in $(git tag --merged "$BASE_REF" --list 'v*' --sort=-version:refname); do
    release_is_stable_version "$tag" || continue
    if git rev-parse --verify --quiet "refs/tags/withdrawn/$tag" >/dev/null; then
      continue
    fi
    STABLE_REF="$tag"
    break
  done
fi
if [ -z "$STABLE_REF" ]; then
  printf 'error: no stable release tag is reachable from interface authority %s\n' "$BASE_REF" >&2
  exit 2
fi

exec "$ROOT/scripts/policy/check-command-compatibility.sh" \
  --base-ref "$BASE_REF" \
  --stable-ref "$STABLE_REF" \
  --candidate-ref "$CANDIDATE_REF"
