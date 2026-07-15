#!/bin/sh
set -eu

# Discover production Go packages changed by the candidate and buildable on
# the current native runner, then generate and enforce their coverage profile.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BASE_REF=""
PROFILE=""
TIMEOUT="${COVERAGE_PLATFORM_TIMEOUT:-10m}"

usage() {
	printf '%s\n' "usage: $0 --base-ref <ref> --profile <file>" >&2
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--base-ref)
		[ "$#" -ge 2 ] || { usage; exit 2; }
		BASE_REF="$2"
		shift 2
		;;
	--profile)
		[ "$#" -ge 2 ] || { usage; exit 2; }
		PROFILE="$2"
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

[ -n "$BASE_REF" ] && [ -n "$PROFILE" ] || { usage; exit 2; }
cd "$ROOT"
git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
	printf 'error: coverage base ref is not available locally: %s\n' "$BASE_REF" >&2
	exit 2
}

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-platform-coverage.XXXXXX")"
PACKAGES="$TMP_ROOT/packages"
SORTED_PACKAGES="$TMP_ROOT/packages.sorted"
cleanup() {
	rm -rf "$TMP_ROOT"
}
trap cleanup EXIT HUP INT TERM
: >"$PACKAGES"

git diff --name-only --diff-filter=ACMR "$BASE_REF" -- '*.go' |
	while IFS= read -r file; do
		case "$file" in
		*_test.go|test/*) continue ;;
		esac
		directory="$(dirname -- "$file")"
		package="./$directory"
		[ "$directory" = "." ] && package="."
		active_files="$(go list -e -f '{{range .GoFiles}}{{.}}{{"\n"}}{{end}}{{range .CgoFiles}}{{.}}{{"\n"}}{{end}}' "$package")"
		if printf '%s\n' "$active_files" | grep -Fqx -- "$(basename -- "$file")"; then
			printf '%s\n' "$package"
		fi
	done >"$PACKAGES"

sort -u "$PACKAGES" >"$SORTED_PACKAGES"
set --
COVERPKG=""
while IFS= read -r package; do
	if [ -n "$package" ]; then
		set -- "$@" "$package"
		if [ -z "$COVERPKG" ]; then
			COVERPKG="$package"
		else
			COVERPKG="$COVERPKG,$package"
		fi
	fi
done <"$SORTED_PACKAGES"

if [ "$#" -eq 0 ]; then
	printf '%s\n' 'mode: atomic' >"$PROFILE"
else
	printf 'native changed coverage packages:'
	printf ' %s' "$@"
	printf '\n'
	# The shortcut integration harness lives in internal/shortcut/builtin but
	# executes code from every product package. Cross-package instrumentation is
	# required for those real executions to count toward the owning source file.
	#
	# Platform runners intentionally execute only the exhaustive shortcut
	# harness and explicitly cross-platform tests. All changed production
	# packages remain instrumented, so unexercised statements still count as
	# uncovered. This keeps the native Windows gate independent of unrelated
	# tests that require Unix permissions, shell scripts, or process semantics.
	go test -count=1 -timeout="$TIMEOUT" -run '^(TestAllShortcuts|TestCrossPlatformCoverage)' \
		-coverpkg="$COVERPKG" -coverprofile="$PROFILE" -covermode=atomic "$@"
fi

./scripts/policy/check-coverage-gate.sh \
	--base-ref "$BASE_REF" \
	--changed-only \
	--scope-buildable \
	--diff-profile "$PROFILE"
