#!/bin/sh
set -eu

# Compare the candidate command tree against a Git-owned historical source
# revision. CI invokes this for both the PR merge-base and latest stable tag; a
# fixture from the candidate branch is never used as authority.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""

usage() {
  printf '%s\n' "usage: $0 --base-ref <ref>" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-ref)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      BASE_REF="$2"
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
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
  printf 'error: interface authority is not available locally: %s\n' "$BASE_REF" >&2
  exit 2
}

TMP_ROOT="$(policy_runtime_mktemp_dir dws-interface-authority)"
BASE_WORKTREE="$TMP_ROOT/base-worktree"
BASELINE="$TMP_ROOT/merge-base.txt"
CANDIDATE_BIN="$TMP_ROOT/interface-baseline"
CANDIDATE_HOME="$TMP_ROOT/candidate-home"

cleanup() {
  git -C "$ROOT" worktree remove --force "$BASE_WORKTREE" >/dev/null 2>&1 || true
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

install_snapshot_helper() {
  worktree="$1"
  helper="$worktree/scripts/policy/interface-baseline"

  # Use one contract format on both sides while constructing the command tree
  # from the authoritative historical source revision.
  rm -rf "$helper"
  mkdir -p "$helper"
  cp -R "$ROOT/scripts/policy/interface-baseline/." "$helper/"
}

generate_reference() {
  ref="$1"
  worktree="$2"
  output="$3"
  home="$4"
  bin="$TMP_ROOT/reference-$(basename "$worktree")"

  git -C "$ROOT" worktree add --detach "$worktree" "$ref" >/dev/null
  install_snapshot_helper "$worktree"
  mkdir -p "$home"
  (
    cd "$worktree"
    go build -o "$bin" ./scripts/policy/interface-baseline
    HOME="$home" DWS_LANG=zh "$bin" >"$output"
  )
}

generate_reference "$BASE_REF" "$BASE_WORKTREE" "$BASELINE" "$TMP_ROOT/base-home"

mkdir -p "$CANDIDATE_HOME"
go build -o "$CANDIDATE_BIN" ./scripts/policy/interface-baseline

printf 'checking candidate against authoritative interface ref %s\n' "$BASE_REF"
HOME="$CANDIDATE_HOME" DWS_LANG=zh "$CANDIDATE_BIN" --check "$BASELINE"
