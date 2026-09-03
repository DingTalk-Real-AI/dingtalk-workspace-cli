#!/bin/sh
set -eu
set -f

usage() {
	printf 'usage: %s <output-profile>\n' "$0" >&2
	exit 2
}

[ "$#" -eq 1 ] || usage

TOOLS_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
ROOT="${DWS_COVERAGE_ROOT:-$TOOLS_ROOT}"
case "$1" in
	/*) output="$1" ;;
	*) output="$(pwd)/$1" ;;
esac

git -C "$ROOT" rev-parse --show-toplevel >/dev/null

scratch_root="${RUNNER_TEMP:-${TMPDIR:-.}}"
workdir="$(mktemp -d "$scratch_root/dws-full-coverage.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT HUP INT TERM

profiles=''
for shard in app cli generators helpers remaining; do
	profile="$workdir/coverage-$shard.txt"
	DWS_COVERAGE_ROOT="$ROOT" \
		"$TOOLS_ROOT/scripts/ci/run-coverage-shard.sh" run "$shard" "$profile"
	profiles="$profiles $profile"
done

# shellcheck disable=SC2086
"$TOOLS_ROOT/scripts/ci/merge-coverage-profiles.sh" "$output" $profiles
