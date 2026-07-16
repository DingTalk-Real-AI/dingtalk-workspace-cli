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
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
	printf 'error: coverage base ref is not available locally: %s\n' "$BASE_REF" >&2
	exit 2
}

TMP_ROOT="$(policy_runtime_mktemp_dir dws-platform-coverage)"
PACKAGES="$TMP_ROOT/packages"
SORTED_PACKAGES="$TMP_ROOT/packages.sorted"
TEST_PACKAGES="$TMP_ROOT/test-packages"
DEPENDENCIES="$TMP_ROOT/dependencies"
cleanup() {
	rm -rf "$TMP_ROOT"
}
trap cleanup EXIT HUP INT TERM
: >"$PACKAGES"
: >"$TEST_PACKAGES"

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
FIRST_PACKAGE=""
while IFS= read -r package; do
	if [ -n "$package" ]; then
		set -- "$@" "$package"
		[ -n "$FIRST_PACKAGE" ] || FIRST_PACKAGE="$package"
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
	for package in "$@"; do
		tests="$(go test -list '^(TestAllShortcuts|TestCrossPlatformCoverage)' "$package")"
		if printf '%s\n' "$tests" | grep -Eq '^(TestAllShortcuts|TestCrossPlatformCoverage)'; then
			printf '%s\n' "$package" >>"$TEST_PACKAGES"
		fi
	done
	set --
	while IFS= read -r package; do
		[ -n "$package" ] && set -- "$@" "$package"
	done <"$TEST_PACKAGES"
	if [ "$#" -eq 0 ]; then
		# Keep a valid zero-coverage profile so the fail-closed gate can report
		# the missing coverage when no explicit platform test exists.
		set -- "$FIRST_PACKAGE"
	fi
	go list -deps "$@" >"$DEPENDENCIES"
	module="$(go list -m -f '{{.Path}}')"
	while IFS= read -r package; do
		[ -n "$package" ] || continue
		case "$package" in
		.) import_path="$module" ;;
		./*) import_path="$module/${package#./}" ;;
		*) import_path="$package" ;;
		esac
		if ! grep -Fqx -- "$import_path" "$DEPENDENCIES"; then
			printf '%s\n' "$package" >>"$TEST_PACKAGES"
		fi
	done <"$SORTED_PACKAGES"
	sort -u "$TEST_PACKAGES" >"$TEST_PACKAGES.sorted"
	set --
	while IFS= read -r package; do
		[ -n "$package" ] && set -- "$@" "$package"
	done <"$TEST_PACKAGES.sorted"
	printf 'native coverage test packages:'
	printf ' %s' "$@"
	printf '\n'
	# The shortcut integration harness lives in internal/shortcut/builtin but
	# executes code from every product package. Cross-package instrumentation is
	# required for those real executions to count toward the owning source file.
	#
	# Platform runners execute packages that declare an exhaustive shortcut
	# harness or explicit cross-platform tests, plus changed packages outside
	# those packages' dependency graph. All changed production packages remain
	# instrumented via coverpkg, so unexercised statements still count as
	# uncovered while redundant zero-test driver binaries are avoided.
	go test -count=1 -timeout="$TIMEOUT" -run '^(TestAllShortcuts|TestCrossPlatformCoverage)' \
		-coverpkg="$COVERPKG" -coverprofile="$PROFILE" -covermode=atomic "$@"
fi

./scripts/policy/check-coverage-gate.sh \
	--base-ref "$BASE_REF" \
	--changed-only \
	--scope-buildable \
	--diff-profile "$PROFILE"
