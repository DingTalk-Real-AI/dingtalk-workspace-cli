#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
DEFAULT_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
target_root="$DEFAULT_ROOT"
output=""

usage() {
	printf '%s\n' "usage: $0 [--root <repository>] --output <profile>" >&2
	exit 2
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--root)
			[ "$#" -ge 2 ] || usage
			target_root="$2"
			shift 2
			;;
		--output)
			[ "$#" -ge 2 ] || usage
			output="$2"
			shift 2
			;;
		*) usage ;;
	esac
done

[ -n "$output" ] || usage
target_root="$(CDPATH= cd -- "$target_root" && pwd)"
case "$output" in
	/*) ;;
	*) output="$(pwd)/$output" ;;
esac

workdir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dws-full-coverage.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT HUP INT TERM
profile_list="$workdir/profiles"
: > "$profile_list"

record_profile() {
	profile="$1"
	test -s "$profile"
	test "$(head -n 1 "$profile")" = "mode: atomic"
	printf '%s\n' "$profile" >> "$profile_list"
}

package_output="$(
	DWS_TEST_ROOT="$target_root" \
		"$SCRIPT_DIR/test-packages.sh" list-coverage app
)"
set --
while IFS= read -r package; do
	[ -n "$package" ] || continue
	set -- "$@" "$package"
done <<EOF
$package_output
EOF
test "$#" -eq 1
app_package="$1"

for partition in $("$SCRIPT_DIR/run-app-race-tests.sh" list-partitions); do
	profile="$workdir/app-$partition.txt"
	DWS_TEST_ROOT="$target_root" \
		"$SCRIPT_DIR/run-app-race-tests.sh" coverage \
		"$app_package" "$partition" "$profile"
	record_profile "$profile"
done

for shard in cli generators helpers remaining; do
	package_output="$(
		DWS_TEST_ROOT="$target_root" \
			"$SCRIPT_DIR/test-packages.sh" list-coverage "$shard"
	)"
	set --
	while IFS= read -r package; do
		[ -n "$package" ] || continue
		set -- "$@" "$package"
	done <<EOF
$package_output
EOF
	test "$#" -gt 0
	profile="$workdir/$shard.txt"
	(
		cd "$target_root"
		go test -v -count=1 -p 1 \
			-coverprofile="$profile" \
			-covermode=atomic \
			"$@"
	)
	record_profile "$profile"
done

# App partitions instrument the same package, so their profiles intentionally
# contain duplicate blocks. Go's coverage parser and the repository gate merge
# identical atomic blocks; concatenating them preserves whether each block was
# covered while the short test processes release app registries between runs.
printf 'mode: atomic\n' > "$output"
while IFS= read -r profile; do
	tail -n +2 "$profile" >> "$output"
done < "$profile_list"

(
	cd "$target_root"
	go tool cover -func="$output" | tail -n 1
)
