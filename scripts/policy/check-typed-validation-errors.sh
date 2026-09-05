#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

GO_BIN="${GO:-go}"

"$ROOT/scripts/policy/check-cobra-patch.sh"

pattern='^TestCrossPlatformCoverage(NormalizeValidation|PreserveClassification|WithValidation|PrepareCommandTree|ValidationTraversal|ValidationPipelineTierParity|ValidationAdapters|FrameworkValidationHooksAreTyped|DeclareLeafMetadataValidateWithoutConfirmRunsInner|OAListByAdminValidationErrorsAreTyped|ProxyParseValidationBoundary|NewPreParseValidationErrorPreservesAuthoritativeErrors|TypedValidationErrorGate(FinalCommandTree|RepresentativeCommands|Extensions))$'
if ! output=$("$GO_BIN" test -json -count=1 \
	./internal/errors ./internal/corecmd ./internal/helpers ./internal/app \
	-run "$pattern" 2>&1); then
	printf '%s\n' "$output" >&2
	exit 1
fi

module='github.com/DingTalk-Real-AI/dingtalk-workspace-cli'
require_test() {
	package="$module/$1"
	test_name="$2"
	run_marker="\"Action\":\"run\",\"Package\":\"$package\",\"Test\":\"$test_name\""
	pass_marker="\"Action\":\"pass\",\"Package\":\"$package\",\"Test\":\"$test_name\""
	case "$output" in
		*"$run_marker"*"$pass_marker"*) ;;
		*)
			printf 'typed validation error gate did not run and pass %s in %s\n' "$test_name" "$package" >&2
			exit 1
			;;
	esac
}

require_test internal/errors TestCrossPlatformCoverageNormalizeValidation
require_test internal/errors TestCrossPlatformCoveragePreserveClassification
require_test internal/corecmd TestCrossPlatformCoverageWithValidation
require_test internal/corecmd TestCrossPlatformCoveragePrepareCommandTree
require_test internal/corecmd TestCrossPlatformCoverageValidationTraversal
require_test internal/helpers TestCrossPlatformCoverageValidationPipelineTierParity
require_test internal/app TestCrossPlatformCoverageTypedValidationErrorGateExtensions
require_test internal/corecmd TestCrossPlatformCoverageValidationAdapters
require_test internal/corecmd TestCrossPlatformCoverageFrameworkValidationHooksAreTyped
require_test internal/helpers TestCrossPlatformCoverageDeclareLeafMetadataValidateWithoutConfirmRunsInner
require_test internal/helpers TestCrossPlatformCoverageOAListByAdminValidationErrorsAreTyped
require_test internal/helpers TestCrossPlatformCoverageProxyParseValidationBoundary
require_test internal/app TestCrossPlatformCoverageNewPreParseValidationErrorPreservesAuthoritativeErrors
require_test internal/app TestCrossPlatformCoverageTypedValidationErrorGateFinalCommandTree
require_test internal/app TestCrossPlatformCoverageTypedValidationErrorGateRepresentativeCommands

printf '%s\n' "$output" | sed -n 's/.*distribution validation coverage: \([0-9][0-9]* nodes\).*/distribution validation coverage: \1/p'
printf '%s\n' "$output" | awk '
    /"Action":"pass"/ && /"Test":"TestCrossPlatformCoverageTypedValidationErrorGateExtensions\// { count++ }
    END { printf "runtime extension validation coverage: %d executed cases\n", count }
'
printf '%s\n' 'typed validation error gate ok (framework lifecycle boundaries)'
