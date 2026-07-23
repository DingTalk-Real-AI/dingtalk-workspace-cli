// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"errors"
	"testing"
)

func TestCrossPlatformCoverageLegacySelectorCompatibilityEdges(t *testing.T) {
	upgradeDir := t.TempDir()
	upgradeCfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{{
			Name:   "Legacy Organization",
			CorpID: "corp_upgrade_fixture",
		}},
	}
	if err := upsertProfileFromToken(upgradeDir, upgradeCfg, &TokenData{
		CorpID:   "corp_upgrade_fixture",
		UserID:   "identity_upgrade_fixture",
		UserName: "Upgraded Account",
	}, false); err != nil {
		t.Fatalf("upsertProfileFromToken(upgrade legacy profile) error = %v", err)
	}
	if len(upgradeCfg.Profiles) != 1 || upgradeCfg.Profiles[0].UserID != "identity_upgrade_fixture" {
		t.Fatalf("upgraded profiles = %#v", upgradeCfg.Profiles)
	}

	renameDir := t.TempDir()
	renameCfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{
			{Name: "duplicate", CorpID: "corp_rename_fixture"},
			{Name: "duplicate", CorpID: "corp_rename_fixture", UserID: "identity_exact_fixture"},
		},
	}
	if err := upsertProfileFromToken(renameDir, renameCfg, &TokenData{
		CorpID:   "corp_rename_fixture",
		CorpName: "Renamed Organization",
	}, false); err != nil {
		t.Fatalf("upsertProfileFromToken(rename blank profile) error = %v", err)
	}
	if renameCfg.Profiles[0].Name != "Renamed Organization" {
		t.Fatalf("blank profile name = %q, want conflict-free organization name", renameCfg.Profiles[0].Name)
	}

	blank := Profile{CorpID: "corp_selector_fixture"}
	if got := storedProfileSelector(nil, nil); got != "" {
		t.Fatalf("storedProfileSelector(nil profile) = %q", got)
	}
	if got := storedProfileSelector(nil, &blank); got != blank.CorpID {
		t.Fatalf("storedProfileSelector(nil config) = %q", got)
	}
	if localProfileSelectorIsSafe(nil, &blank, "local") ||
		localProfileSelectorIsSafe(&ProfilesConfig{}, nil, "local") {
		t.Fatal("nil selector inputs were treated as safe")
	}
	if got := unresolvedProfileSelector("   "); got != "" {
		t.Fatalf("unresolvedProfileSelector(blank) = %q", got)
	}
	if _, ok := parseUnresolvedProfileSelector(unresolvedProfileSelectorPrefix + "!"); ok {
		t.Fatal("invalid base64 legacy selector parsed successfully")
	}
	if _, ok := parseUnresolvedProfileSelector(unresolvedProfileSelectorPrefix + "IA"); ok {
		t.Fatal("blank decoded legacy selector parsed successfully")
	}

	previousRuntime := RuntimeProfile()
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile(previousRuntime) })
	if got := StableTokenProfileSelector(t.TempDir(), nil); got != "" {
		t.Fatalf("StableTokenProfileSelector(nil) = %q", got)
	}

	ambiguousDir := t.TempDir()
	ambiguousCfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: "corp_ambiguous_fixture",
		Profiles: []Profile{
			{Name: "First", CorpID: "corp_ambiguous_fixture", UserID: "identity_first_fixture"},
			{Name: "Second", CorpID: "corp_ambiguous_fixture", UserID: "identity_second_fixture"},
		},
	}
	if err := SaveProfiles(ambiguousDir, ambiguousCfg); err != nil {
		t.Fatalf("SaveProfiles(ambiguous current) error = %v", err)
	}
	ambiguousToken := &TokenData{CorpID: "corp_ambiguous_fixture", UserID: "identity_first_fixture"}
	if got, want := StableTokenProfileSelector(ambiguousDir, ambiguousToken), "corp_ambiguous_fixture:identity_first_fixture"; got != want {
		t.Fatalf("StableTokenProfileSelector(ambiguous organization) = %q, want %q", got, want)
	}

	reserved := unresolvedProfileSelector("corp_missing_fixture")
	emptyCfg := &ProfilesConfig{}
	if _, _, err := resolveProfileSelection("", emptyCfg, reserved); err == nil {
		t.Fatal("missing reserved profile selection succeeded")
	}
	if _, _, err := resolveProfileDeletionSelection(emptyCfg, reserved); err == nil {
		t.Fatal("missing reserved profile deletion succeeded")
	}
	if got := canonicalStoredSelector(emptyCfg, reserved); got != "" {
		t.Fatalf("canonical missing reserved selector = %q", got)
	}
	if !selectorTargetsCorp(reserved, "corp_missing_fixture") {
		t.Fatal("reserved selector did not target its organization")
	}

	localCfg := &ProfilesConfig{Profiles: []Profile{{
		Name:   "local-profile-fixture",
		CorpID: "corp_local_fixture",
		UserID: "identity_local_fixture",
	}}}
	if got, want := canonicalStoredSelector(localCfg, "local-profile-fixture"), "corp_local_fixture:identity_local_fixture"; got != want {
		t.Fatalf("canonical local selector = %q, want %q", got, want)
	}
	if unresolvedProfileForCorp(nil, "corp") != nil || unresolvedProfileForLocalName(nil, "local") != nil {
		t.Fatal("nil profile registry returned an unresolved profile")
	}
	if unresolvedProfileForLocalName(localCfg, "   ") != nil {
		t.Fatal("blank local name returned an unresolved profile")
	}
	duplicateCfg := &ProfilesConfig{Profiles: []Profile{
		{Name: "duplicate-blank", CorpID: "corp_duplicate_one"},
		{Name: "Exact One", CorpID: "corp_duplicate_one", UserID: "identity_one"},
		{Name: "duplicate-blank", CorpID: "corp_duplicate_two"},
		{Name: "Exact Two", CorpID: "corp_duplicate_two", UserID: "identity_two"},
	}}
	if unresolvedProfileForLocalName(duplicateCfg, "duplicate-blank") != nil {
		t.Fatal("duplicate unresolved local name selected an arbitrary profile")
	}

	oldLoadCorp := tokenLoadKeychainForCorpID
	t.Cleanup(func() { tokenLoadKeychainForCorpID = oldLoadCorp })
	tokenLoadKeychainForCorpID = func(string) (*TokenData, error) { return nil, nil }
	if _, err := tokenLoadProfileIdentity(Profile{CorpID: "corp_nil_token_fixture"}); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("nil organization token error = %v", err)
	}
}

func TestCrossPlatformCoverageLegacyProfileLifecycleErrorEdges(t *testing.T) {
	oldEnsure := profilesEnsureMigration
	oldLoad := profilesLoad
	oldSave := profilesSave
	oldLoadCorp := profilesLoadCorp
	oldLoadLegacy := profilesLoadLegacy
	oldLoadIdentity := profilesLoadIdentity
	oldSaveCorp := profilesSaveCorp
	oldDeleteCorp := profilesDeleteCorp
	oldDeleteLegacy := profilesDeleteLegacy
	oldDeleteMarker := profilesDeleteMarker
	t.Cleanup(func() {
		profilesEnsureMigration = oldEnsure
		profilesLoad = oldLoad
		profilesSave = oldSave
		profilesLoadCorp = oldLoadCorp
		profilesLoadLegacy = oldLoadLegacy
		profilesLoadIdentity = oldLoadIdentity
		profilesSaveCorp = oldSaveCorp
		profilesDeleteCorp = oldDeleteCorp
		profilesDeleteLegacy = oldDeleteLegacy
		profilesDeleteMarker = oldDeleteMarker
	})

	identityFailure := errors.New("identity load failure")
	profilesEnsureMigration = func(string) error { return nil }
	profilesSave = func(string, *ProfilesConfig) error { return nil }
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesLoadLegacy = func() (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, identityFailure }
	profilesSaveCorp = func(string, *TokenData) error { return nil }
	profilesDeleteCorp = func(string) error { return nil }
	profilesDeleteLegacy = func() error { return nil }
	profilesDeleteMarker = func(string) error { return nil }

	setCurrentCfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: "corp_set_fixture:identity_set_fixture",
		OrgCurrentProfiles: map[string]string{
			"corp_set_fixture": "corp_set_fixture:identity_set_fixture",
		},
		Profiles: []Profile{{
			Name:   "Set Account",
			CorpID: "corp_set_fixture",
			UserID: "identity_set_fixture",
		}},
	}
	profilesLoad = func(string) (*ProfilesConfig, error) { return setCurrentCfg, nil }
	if _, err := setCurrentProfileLocked(t.TempDir(), "corp_set_fixture:identity_set_fixture"); !errors.Is(err, identityFailure) {
		t.Fatalf("setCurrentProfileLocked sync error = %v", err)
	}

	usePreviousCfg := &ProfilesConfig{
		Version:         profilesVersion,
		CurrentProfile:  "corp_previous_fixture:identity_current_fixture",
		PreviousProfile: "corp_previous_fixture:identity_previous_fixture",
		OrgCurrentProfiles: map[string]string{
			"corp_previous_fixture": "corp_previous_fixture:identity_current_fixture",
		},
		Profiles: []Profile{
			{Name: "Current", CorpID: "corp_previous_fixture", UserID: "identity_current_fixture"},
			{Name: "Previous", CorpID: "corp_previous_fixture", UserID: "identity_previous_fixture"},
		},
	}
	profilesLoad = func(string) (*ProfilesConfig, error) { return usePreviousCfg, nil }
	if _, err := usePreviousProfileLocked(t.TempDir()); !errors.Is(err, identityFailure) {
		t.Fatalf("usePreviousProfileLocked sync error = %v", err)
	}

	snapshotFailure := errors.New("organization snapshot failure")
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, snapshotFailure }
	if _, err := snapshotProfileSelectionMirrors(t.TempDir(), "corp_snapshot_fixture", true); !errors.Is(err, snapshotFailure) {
		t.Fatalf("snapshot organization error = %v", err)
	}
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }

	operationFailure := errors.New("selection operation failure")
	organizationRestoreFailure := errors.New("organization restore failure")
	profilesSaveCorp = func(string, *TokenData) error { return organizationRestoreFailure }
	withOrganization := profileSelectionMirrorSnapshot{
		organization: tokenSlotSnapshot{known: true, exists: true, token: &TokenData{CorpID: "corp_rollback_fixture"}},
		marker:       tokenMarkerSnapshot{known: true},
	}
	if err := rollbackProfileSelection(t.TempDir(), &ProfilesConfig{}, "corp_rollback_fixture", withOrganization, operationFailure); !errors.Is(err, operationFailure) || !errors.Is(err, organizationRestoreFailure) {
		t.Fatalf("rollback organization save error = %v", err)
	}

	organizationDeleteFailure := errors.New("organization delete failure")
	profilesSaveCorp = func(string, *TokenData) error { return nil }
	profilesDeleteCorp = func(string) error { return organizationDeleteFailure }
	withoutOrganization := profileSelectionMirrorSnapshot{
		organization: tokenSlotSnapshot{known: true},
		marker:       tokenMarkerSnapshot{known: true},
	}
	if err := rollbackProfileSelection(t.TempDir(), &ProfilesConfig{}, "corp_rollback_fixture", withoutOrganization, operationFailure); !errors.Is(err, operationFailure) || !errors.Is(err, organizationDeleteFailure) {
		t.Fatalf("rollback organization delete error = %v", err)
	}
	profilesDeleteCorp = func(string) error { return nil }

	remainingCfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: "corp_removed_fixture:identity_removed_fixture",
		OrgCurrentProfiles: map[string]string{
			"corp_removed_fixture": "corp_removed_fixture:identity_removed_fixture",
		},
		Profiles: []Profile{
			{Name: "Removed", CorpID: "corp_removed_fixture", UserID: "identity_removed_fixture"},
			{Name: "Remaining", CorpID: "corp_remaining_fixture", UserID: "identity_remaining_fixture"},
		},
	}
	profilesLoad = func(string) (*ProfilesConfig, error) { return remainingCfg, nil }
	if _, err := removeProfileLocked(t.TempDir(), "corp_removed_fixture:identity_removed_fixture"); err != nil {
		t.Fatalf("removeProfileLocked(single fallback) error = %v", err)
	}
	if remainingCfg.CurrentProfile != "corp_remaining_fixture:identity_remaining_fixture" {
		t.Fatalf("fallback current profile = %q", remainingCfg.CurrentProfile)
	}

	blankCfg := &ProfilesConfig{
		Version:         profilesVersion,
		CurrentProfile:  "corp_blank_fixture:identity_exact_fixture",
		PreviousProfile: "legacy-blank-fixture",
		OrgCurrentProfiles: map[string]string{
			"corp_blank_fixture": "corp_blank_fixture:identity_exact_fixture",
		},
		Profiles: []Profile{
			{Name: "legacy-blank-fixture", CorpID: "corp_blank_fixture"},
			{Name: "Exact", CorpID: "corp_blank_fixture", UserID: "identity_exact_fixture"},
		},
	}
	profilesLoad = func(string) (*ProfilesConfig, error) { return blankCfg, nil }
	if _, err := removeProfileLocked(t.TempDir(), "corp_blank_fixture:identity_exact_fixture"); err != nil {
		t.Fatalf("removeProfileLocked(blank fallback) error = %v", err)
	}
	if blankCfg.CurrentProfile != "corp_blank_fixture" || blankCfg.OrgCurrentProfiles["corp_blank_fixture"] != "" {
		t.Fatalf("blank fallback selection = current %q org %q", blankCfg.CurrentProfile, blankCfg.OrgCurrentProfiles["corp_blank_fixture"])
	}
}
