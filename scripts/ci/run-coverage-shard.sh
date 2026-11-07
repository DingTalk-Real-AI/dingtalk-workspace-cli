#!/bin/sh
set -eu
set -f

usage() {
	printf '%s\n' \
		"usage: $0 run <app|cli|generators|helpers|remaining> <output-profile>" \
		"       $0 list-app-partitions <app|cli|generators|helpers|remaining>" >&2
	exit 2
}

TOOLS_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
ROOT="${DWS_COVERAGE_ROOT:-$TOOLS_ROOT}"

app_partitions_for_shard() {
	case "$1" in
		app) printf '%s\n' 'c-p-r a-b c-other s-z-example-fuzz' ;;
		generators) printf '%s\n' 'd-r c-m-o' ;;
		helpers) printf '%s\n' 'c-a-l schema' ;;
		cli) printf '%s\n' 'c-s-z' ;;
		remaining) printf '%s\n' '' ;;
		*)
			printf 'unknown coverage shard: %s\n' "$1" >&2
			exit 2
			;;
	esac
}

mode="${1:-}"
shard="${2:-}"
[ -n "$shard" ] || usage
app_partitions="$(app_partitions_for_shard "$shard")"

case "$mode" in
	list-app-partitions)
		[ "$#" -eq 2 ] || usage
		printf '%s\n' "$app_partitions"
		exit 0
		;;
	run)
		[ "$#" -eq 3 ] || usage
		case "$3" in
			/*) output="$3" ;;
			*) output="$(pwd)/$3" ;;
		esac
		;;
	*) usage ;;
esac

scratch_root="${RUNNER_TEMP:-${TMPDIR:-.}}"
workdir="$(mktemp -d "$scratch_root/dws-coverage-shard.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT HUP INT TERM

package_output="$(
	DWS_TEST_PACKAGES_ROOT="$ROOT" \
		"$TOOLS_ROOT/scripts/ci/test-packages.sh" list-coverage "$shard"
)"
[ -n "$package_output" ] || {
	printf 'coverage package shard is empty: %s\n' "$shard" >&2
	exit 1
}

app_package="$(
	DWS_TEST_PACKAGES_ROOT="$ROOT" \
		"$TOOLS_ROOT/scripts/ci/test-packages.sh" list-coverage app
)"
app_package_count="$(printf '%s\n' "$app_package" | wc -l | tr -d ' ')"
[ "$app_package_count" -eq 1 ] || {
	printf 'app coverage shard must contain exactly one package\n' >&2
	exit 1
}

profiles=''
if [ "$shard" != app ]; then
	package_profile="$workdir/coverage-packages-$shard.txt"
	package_parallelism=1
	if [ "$shard" = remaining ]; then
		# The remaining set contains independent long-running packages. Let two
		# package test binaries overlap on this runner instead of serializing
		# both long tails; this does not request another Actions runner.
		package_parallelism=2
	fi
	cd "$ROOT"
	# Package paths cannot contain shell whitespace. Parallelism stays bounded
	# inside this hosted runner to control CPU and memory pressure.
	# shellcheck disable=SC2086
	go test -count=1 -p "$package_parallelism" \
		-coverprofile="$package_profile" \
		-covermode=atomic \
		$package_output
	profiles="$profiles $package_profile"
fi

for partition in $app_partitions; do
	partition_profile="$workdir/coverage-app-$partition.txt"
	DWS_APP_TEST_ROOT="$ROOT" \
		"$TOOLS_ROOT/scripts/ci/run-app-race-tests.sh" \
		coverage "$app_package" "$partition_profile" "$partition"
	profiles="$profiles $partition_profile"
done

# shellcheck disable=SC2086
"$TOOLS_ROOT/scripts/ci/merge-coverage-profiles.sh" "$output" $profiles
