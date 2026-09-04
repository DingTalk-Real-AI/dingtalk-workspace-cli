#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

GO_BIN="${GO:-go}"

run_tests() {
	package="$1"
	pattern="$2"
	shift 2
	if ! output=$("$GO_BIN" test -v -count=1 "$package" -run "$pattern" 2>&1); then
		printf '%s\n' "$output" >&2
		return 1
	fi
	printf '%s\n' "$output"
	for test_name in "$@"; do
		case "$output" in
			*"=== RUN   $test_name"*) ;;
			*)
				printf 'typed validation error gate did not run %s in %s\n' "$test_name" "$package" >&2
				return 1
				;;
		esac
	done
}

run_tests ./internal/errors '^TestCrossPlatformCoverageNormalizeValidation$' \
	TestCrossPlatformCoverageNormalizeValidation
run_tests ./internal/corecmd '^TestCrossPlatformCoverage(ValidationAdapters|FrameworkValidationHooksAreTyped)$' \
	TestCrossPlatformCoverageValidationAdapters \
	TestCrossPlatformCoverageFrameworkValidationHooksAreTyped
run_tests ./internal/helpers '^TestCrossPlatformCoverageDeclareLeafMetadataValidateWithoutConfirmRunsInner$' \
	TestCrossPlatformCoverageDeclareLeafMetadataValidateWithoutConfirmRunsInner
run_tests ./internal/app '^TestTypedValidationErrorGate(FinalCommandTree|RepresentativeCommands)$' \
	TestTypedValidationErrorGateFinalCommandTree \
	TestTypedValidationErrorGateRepresentativeCommands

printf '%s\n' 'typed validation error gate ok (framework lifecycle boundaries)'
