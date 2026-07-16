#!/bin/sh
set -eu

# Compare the current Cobra command surface with two local Git revisions:
# the PR merge-base/main reference and the latest stable GA tag. The caller must
# pass the highest non-prerelease SemVer tag (excluding beta/rc tags). Tag
# governance treats that tag as proof of a successfully published GA release;
# release automation must not leave a GA tag behind when publication fails.
#
# The script never fetches refs or snapshots from the network. With checkout
# fetch-depth=0, both references are materialized in temporary worktrees. A
# reference uses its own snapshot helper when available; older revisions are
# bootstrapped with the candidate helper.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""
STABLE_REF=""

usage() {
  printf '%s\n' "usage: $0 --base-ref <ref> --stable-ref <ref>" >&2
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

[ -n "$BASE_REF" ] && [ -n "$STABLE_REF" ] || { usage; exit 2; }

cd "$ROOT"
git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
  printf 'base ref is not available locally: %s\n' "$BASE_REF" >&2
  exit 2
}
git rev-parse --verify --quiet "${STABLE_REF}^{commit}" >/dev/null || {
  printf 'stable ref is not available locally: %s\n' "$STABLE_REF" >&2
  exit 2
}

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-command-compat.XXXXXX")"
BASE_WORKTREE="$TMP_ROOT/base-worktree"
STABLE_WORKTREE="$TMP_ROOT/stable-worktree"

cleanup() {
  git -C "$ROOT" worktree remove --force "$BASE_WORKTREE" >/dev/null 2>&1 || true
  git -C "$ROOT" worktree remove --force "$STABLE_WORKTREE" >/dev/null 2>&1 || true
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

ensure_snapshot_helper() {
  worktree="$1"

	# Once a baseline contains the helper, use that revision's own capture
	# rules. This makes a rule/filter change in the candidate visible as a
	# snapshot_rules_changed failure instead of applying the new exclusion to
	# both sides and accidentally hiding a removed command. Older revisions
	# created before this helper existed are bootstrapped with the candidate
	# implementation.
	if [ -f "$worktree/cmd/interface-snapshot/main.go" ] && \
	   [ -f "$worktree/internal/interfacesnapshot/snapshot.go" ] && \
	   [ -f "$worktree/internal/interfacesnapshot/compare.go" ]; then
		return
	fi
	if [ -e "$worktree/cmd/interface-snapshot" ] || \
	   [ -e "$worktree/internal/interfacesnapshot" ]; then
		printf 'baseline contains an incomplete interface snapshot helper: %s\n' "$worktree" >&2
		exit 2
	fi

	mkdir -p "$worktree/cmd/interface-snapshot" "$worktree/internal/interfacesnapshot"
	cp -R "$ROOT/cmd/interface-snapshot/." "$worktree/cmd/interface-snapshot/"
	cp -R "$ROOT/internal/interfacesnapshot/." "$worktree/internal/interfacesnapshot/"
}

generate_ref_snapshot() {
  ref="$1"
  worktree="$2"
  output="$3"

  git -C "$ROOT" worktree add --detach "$worktree" "$ref" >/dev/null
  ensure_snapshot_helper "$worktree"
  (
    cd "$worktree"
    go run ./cmd/interface-snapshot generate --output "$output"
  )
}

CANDIDATE="$TMP_ROOT/candidate.json"
BASELINE="$TMP_ROOT/merge-base.json"
STABLE="$TMP_ROOT/stable.json"

go run ./cmd/interface-snapshot generate --output "$CANDIDATE"
generate_ref_snapshot "$BASE_REF" "$BASE_WORKTREE" "$BASELINE"
generate_ref_snapshot "$STABLE_REF" "$STABLE_WORKTREE" "$STABLE"

go run ./cmd/interface-snapshot compare \
  --current "$CANDIDATE" \
  --base "$BASELINE" \
  --stable "$STABLE"
