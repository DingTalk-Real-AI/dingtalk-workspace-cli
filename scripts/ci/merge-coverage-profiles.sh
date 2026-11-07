#!/bin/sh
set -eu

usage() {
	printf 'usage: %s <output-profile> <input-profile>...\n' "$0" >&2
	exit 2
}

[ "$#" -ge 2 ] || usage

output="$1"
shift

for profile in "$@"; do
	[ "$profile" != "$output" ] || {
		printf 'coverage merge output must not also be an input: %s\n' "$output" >&2
		exit 2
	}
	[ -s "$profile" ] || {
		printf 'coverage profile is missing or empty: %s\n' "$profile" >&2
		exit 1
	}
done

scratch_root="${RUNNER_TEMP:-${TMPDIR:-.}}"
workdir="$(mktemp -d "$scratch_root/dws-merge-coverage.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT HUP INT TERM

blocks="$workdir/blocks"
sorted="$workdir/sorted"

# App coverage is collected from multiple fresh test processes so framework
# registries are released between partitions. Every process emits the same
# source blocks; keep each block once and retain the greatest atomic count.
# Coverage policy uses the same union rule when it reads multiple profiles.
awk '
	FNR == 1 {
		if ($0 != "mode: atomic") {
			printf "%s: expected mode: atomic\n", FILENAME > "/dev/stderr"
			invalid = 1
		}
		next
	}
	NF != 3 || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/ {
		printf "%s:%d: invalid coverage profile line\n", FILENAME, FNR > "/dev/stderr"
		invalid = 1
		next
	}
	{
		key = $1 " " $2
		if (!(key in counts) || $3 + 0 > counts[key]) {
			counts[key] = $3 + 0
		}
	}
	END {
		if (invalid) {
			exit 1
		}
		for (key in counts) {
			print key " " counts[key]
		}
	}
' "$@" > "$blocks"

LC_ALL=C sort "$blocks" > "$sorted"
{
	printf 'mode: atomic\n'
	cat "$sorted"
} > "$output"

test -s "$output"
test "$(head -n 1 "$output")" = "mode: atomic"
