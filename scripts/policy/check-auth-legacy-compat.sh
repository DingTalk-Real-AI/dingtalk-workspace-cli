#!/bin/sh
set -eu

# Keep the old-token migration contract reviewable as one stable, zero-argument
# policy check. Every group is enumerated explicitly: discovery must find the
# expected number of top-level tests before any selected test is executed.

if [ "$#" -ne 0 ]; then
	printf '%s\n' "usage: $0" >&2
	exit 2
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
GO="${GO:-go}"
cd "$ROOT"
. "$ROOT/scripts/policy/policy-runtime.sh"
policy_prepare_runtime "$ROOT"

TMP_ROOT="$(policy_runtime_mktemp_dir dws-auth-legacy-compat)"
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM
export DWS_DISABLE_KEYCHAIN=1
export DWS_KEYCHAIN_DIR="$TMP_ROOT/keychain"
mkdir -p "$DWS_KEYCHAIN_DIR"
DISCOVERED_TOTAL=0
EXPECTED_TOTAL=39

MIGRATION_PATTERN='^(TestCrossPlatformCoverageV1044GlobalSlotWithoutProfilesMigrates|TestCrossPlatformCoverageV1050AndV1051GlobalSlotWithV1ProfilesMigratesIdentity|TestCrossPlatformCoverageV1052RawMultiOrganizationSlotsMigrateEveryIdentity|TestCrossPlatformCoverageV1052UnresolvedMultiOrganizationProfilesRemainUsable|TestCrossPlatformCoverageV1052FirstSaveMigratesAllOrganizationsBeforeV2Commit|TestCrossPlatformCoverageV1053PartialV2RegistryRepairsFromMatchingGlobalSlot|TestPersistingLoginFlowsRepairHalfMigratedGlobalBeforeRemoteAndSave|TestExchangeCodeForTokenPreparesBeforeRemoteAndMarksFresh|TestSaveLoginTokenDataRepairsHalfMigratedStateBeforeManualGlobalOverwrite|TestManualLoginSaveRepairsHalfMigratedGlobalBeforeOverwrite|TestCLISmoke_LegacyAuthSlotsMigrateThroughStatus)$'
ISOLATION_PATTERN='^(TestDefaultBlankCurrentRejectsSameCorpGlobalMirrorOwnedByExactIdentity|TestTokenLoadIsolationMatrix|TestTokenPersistenceWritePlanKeepsOrganizationOwnershipIsolated|TestFreshUIDLessExactLoginCannotOverwriteExistingUnresolvedProfile|TestOAuthPersistLoginTokenMarksFreshBeforeUIDLessIsolationCheck|TestDeviceLoginIgnoresUnreadableUnrelatedProfile|TestPATFreshAuthorizationSaveUsesLoginIsolationBoundary|TestSetCurrentProfileRejectsUnreadableExactIdentityBesideBlankProfile|TestUsePreviousProfileRejectsUnreadableExactIdentityBesideBlankProfile|TestExplicitReauthUpgradingSoleBlankProfilePublishesAllTokenMirrors|TestCrossPlatformCoverageLegacyBlankAndExactIdentitySlotsPersistAcrossRepeatedSaves|TestCrossPlatformCoverageExactIdentityRefreshDoesNotOverwriteLegacyBlankOrganizationSlot|TestCrossPlatformCoverageOAuthAndDeviceKeepFreshUnknownIdentityIsolatedFromExactHistory)$'
PARENT_CHILD_PATTERN='^(TestPersonalBusProfileSelectorUsesDefaultBlankCurrentBeforeRuntimeEnrichedIdentity|TestPersonalBusProfileSelectorUsesDefaultExactCurrent|TestPersonalBusProfileSelectorPrefersExplicitRuntimeSelector|TestPersonalBusSpawnArgs_ForwardsProfile|TestCrossPlatformCoveragePersonalBusSpawnArgsPreservesReservedBlankProfile)$'
SELECTOR_PATTERN='^(TestReservedUnresolvedSelectorPromotesSchemaToDowngradeGuardVersion|TestV2ProfilesWithoutReservedSelectorRemainDowngradeReadable|TestInvalidReservedOrgCurrentSelectorNormalizesBackToV2|TestEnsureMigrationPersistsDowngradeGuardForRawV2ReservedSelector|TestV3DowngradesAfterBlankProfileIdentityCompletion|TestV3DowngradesAfterUnresolvedProfileDeletion|TestV3DowngradesWhenExactConflictDeletionLeavesSoleBlankProfile|TestV3DowngradesWhenReservedSelectorCanonicalizesToSafeLocalName)$'
GUIDANCE_PATTERN='^(TestLegacyRefreshReauthorizationGuidanceTreatsProfileAsDisplayData|TestLegacyRefreshReauthorizationGuidanceWithoutProfileStillExplainsLogin)$'

validate_group() {
	group="$1"
	expected="$2"
	pattern="$3"
	shift 3
	list_file="$TMP_ROOT/$group.list"

	if ! "$GO" test -list "$pattern" "$@" >"$list_file"; then
		printf 'AUTH_LEGACY_COMPAT phase=discovery group=%s state=FAIL reason=list-error\n' "$group" >&2
		exit 1
	fi
	actual="$(awk '/^Test[[:alnum:]_]+$/ { count++ } END { print count + 0 }' "$list_file")"
	if [ "$expected" -le 0 ] || [ "$actual" -ne "$expected" ]; then
		printf 'AUTH_LEGACY_COMPAT phase=discovery group=%s state=FAIL expected=%s actual=%s\n' \
			"$group" "$expected" "$actual" >&2
		cat "$list_file" >&2
		exit 1
	fi
	DISCOVERED_TOTAL=$((DISCOVERED_TOTAL + actual))
	printf 'AUTH_LEGACY_COMPAT phase=discovery group=%s state=PASS top_level_tests=%s\n' \
		"$group" "$actual"
}

run_group() {
	group="$1"
	expected="$2"
	pattern="$3"
	shift 3

	if ! "$GO" test -p=1 -count=1 -run "$pattern" "$@"; then
		printf 'AUTH_LEGACY_COMPAT phase=execution group=%s state=FAIL top_level_tests=%s\n' \
			"$group" "$expected" >&2
		exit 1
	fi
	printf 'AUTH_LEGACY_COMPAT phase=execution group=%s state=PASS top_level_tests=%s\n' \
		"$group" "$expected"
}

# Discovery is intentionally completed for every group before execution starts.
validate_group migration 11 "$MIGRATION_PATTERN" ./internal/auth ./internal/app ./test/smoke
validate_group isolation 13 "$ISOLATION_PATTERN" ./internal/auth ./internal/app
validate_group parent-child 5 "$PARENT_CHILD_PATTERN" ./internal/app
validate_group selector 8 "$SELECTOR_PATTERN" ./internal/auth
validate_group guidance 2 "$GUIDANCE_PATTERN" ./internal/auth
if [ "$DISCOVERED_TOTAL" -ne "$EXPECTED_TOTAL" ]; then
	printf 'AUTH_LEGACY_COMPAT phase=discovery group=summary state=FAIL expected=%s actual=%s\n' \
		"$EXPECTED_TOTAL" "$DISCOVERED_TOTAL" >&2
	exit 1
fi

run_group migration 11 "$MIGRATION_PATTERN" ./internal/auth ./internal/app ./test/smoke
run_group isolation 13 "$ISOLATION_PATTERN" ./internal/auth ./internal/app
run_group parent-child 5 "$PARENT_CHILD_PATTERN" ./internal/app
run_group selector 8 "$SELECTOR_PATTERN" ./internal/auth
run_group guidance 2 "$GUIDANCE_PATTERN" ./internal/auth

printf 'AUTH_LEGACY_COMPAT summary state=PASS groups=5 top_level_tests=%s isolation_matrix_cells=18 states=migration:PASS,isolation:PASS,parent-child:PASS,selector:PASS,guidance:PASS\n' \
	"$DISCOVERED_TOTAL"
