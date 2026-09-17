#!/bin/sh
set -eu
set -f

# Called from the candidate repository root. Keep the platform test selection
# and cross-package instrumentation intact while releasing app command trees
# held by process-global registries between small, fresh test processes.
[ "$#" -ge 4 ] || {
	printf 'usage: %s <profile> <coverpkg> <timeout> <package>...\n' "$0" >&2
	exit 2
}
profile="$1"
coverpkg="$2"
timeout="$3"
shift 3
tools_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
scratch="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-.}}/dws-platform-tests.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
pattern='^(TestAllShortcuts|TestCrossPlatformCoverage)'
app_package=''
: > "$scratch/packages"
for package in "$@"; do
	case "$package" in
		./internal/app|*/internal/app) app_package="$package" ;;
		*) printf '%s\n' "$package" >> "$scratch/packages" ;;
	esac
done

set --
while IFS= read -r package; do
	set -- "$@" "$package"
done < "$scratch/packages"
if [ "$#" -gt 0 ]; then
	go test -count=1 -timeout="$timeout" -run "$pattern" \
		-coverpkg="$coverpkg" -coverprofile="$scratch/packages.txt" -covermode=atomic "$@"
fi

if [ -n "$app_package" ]; then
	# Discover on this OS, then include every selected top-level test exactly
	# once. Exact anchors also avoid accidentally rerunning prefix neighbours.
	go test -list "$pattern" "$app_package" > "$scratch/list"
	awk '/^(TestAllShortcuts|TestCrossPlatformCoverage)/ {
		if (count % 12 == 0) {
			if (count) print ")$"
			printf "^("
		} else printf "|"
		printf "%s", $1
		count++
	}
	END { if (count) print ")$"; else print "^$" }' "$scratch/list" > "$scratch/batches"
	batch=0
	while IFS= read -r batch_pattern; do
		batch=$((batch + 1))
		printf 'native app coverage batch %s: %s\n' "$batch" "$batch_pattern"
		go test -count=1 -timeout="$timeout" -run "$batch_pattern" \
			-coverpkg="$coverpkg" -coverprofile="$scratch/app-$batch.txt" -covermode=atomic "$app_package"
	done < "$scratch/batches"
fi

# Only publish a complete union after every package/batch passes. Preserve
# uncovered blocks and use the existing atomic-profile merge implementation.
set +f
sh "$tools_dir/merge-coverage-profiles.sh" "$profile" "$scratch"/*.txt
