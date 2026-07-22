// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"encoding/json"
	"os"
	"testing"
)

type blankProfileSelectorFixture struct {
	configDir     string
	corpID        string
	blankName     string
	blankSelector string
	exactUserID   string
	exactSelector string
	blankToken    *TokenData
	exactToken    *TokenData
}

func seedBlankProfileSelectorFixture(
	t *testing.T,
	blankName string,
	corpName string,
	blankCurrent bool,
) blankProfileSelectorFixture {
	t.Helper()
	cleanupKeychain(t)

	configDir := t.TempDir()
	corpID := "corp_selector_fixture"
	exactUserID := "identity_exact_fixture"
	exactSelector := profileSelector(corpID, exactUserID)
	currentSelector := exactSelector
	previousSelector := blankName
	if blankCurrent {
		currentSelector = blankName
		previousSelector = exactSelector
	}

	blankToken := testToken("at_unresolved_fixture", corpID, corpName)
	blankToken.UserID = ""
	blankToken.UserName = ""
	exactToken := testToken("at_exact_fixture", corpID, corpName)
	exactToken.UserID = exactUserID
	exactToken.UserName = "Exact Fixture Account"

	if err := SaveTokenDataKeychainForCorpID(corpID, blankToken); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID(blank) error = %v", err)
	}
	if err := SaveTokenDataKeychainForIdentity(corpID, exactUserID, exactToken); err != nil {
		t.Fatalf("SaveTokenDataKeychainForIdentity(exact) error = %v", err)
	}
	if err := SaveTokenDataKeychain(exactToken); err != nil {
		t.Fatalf("SaveTokenDataKeychain(exact mirror) error = %v", err)
	}
	if err := WriteTokenMarker(configDir); err != nil {
		t.Fatalf("WriteTokenMarker() error = %v", err)
	}

	cfg := &ProfilesConfig{
		Version:         profilesVersion,
		PrimaryProfile:  exactSelector,
		CurrentProfile:  currentSelector,
		PreviousProfile: previousSelector,
		OrgCurrentProfiles: map[string]string{
			corpID: exactSelector,
		},
		Profiles: []Profile{
			{
				Name:     blankName,
				CorpID:   corpID,
				CorpName: corpName,
				Status:   ProfileStatusActive,
			},
			{
				Name:     "Exact Fixture Account",
				CorpID:   corpID,
				CorpName: corpName,
				UserID:   exactUserID,
				UserName: "Exact Fixture Account",
				Status:   ProfileStatusActive,
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(profiles) error = %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(ProfilesPath(configDir), data, 0o600); err != nil {
		t.Fatalf("os.WriteFile(profiles.json) error = %v", err)
	}

	return blankProfileSelectorFixture{
		configDir:     configDir,
		corpID:        corpID,
		blankName:     blankName,
		blankSelector: ProfileSelectionSelector(cfg.Profiles[0], cfg),
		exactUserID:   exactUserID,
		exactSelector: exactSelector,
		blankToken:    blankToken,
		exactToken:    exactToken,
	}
}

func TestCrossPlatformCoverageBlankProfileNameMatchingCorpNameResolvesCurrentProfileExactly(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", true)

	selected, exact, err := ResolveProfileWithScope(fixture.configDir, "")
	if err != nil {
		t.Fatalf("ResolveProfileWithScope(current) error = %v", err)
	}
	if selected == nil || selected.CorpID != fixture.corpID || selected.UserID != "" {
		t.Fatalf("resolved current profile = %#v, want unresolved profile", selected)
	}
	if !exact {
		t.Fatal("current local-name selector should resolve one exact unresolved profile")
	}
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != fixture.blankSelector {
		t.Fatalf("current profile = %q, want stable unresolved selector %q", cfg.CurrentProfile, fixture.blankSelector)
	}
}

func TestCrossPlatformCoverageBlankProfileNameMatchingCorpNameLoadsOrganizationToken(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", false)

	if fixture.blankSelector == fixture.blankName || fixture.blankSelector == fixture.corpID {
		t.Fatalf("unsafe blank selector = %q, want reserved exact selector", fixture.blankSelector)
	}
	loaded, err := LoadTokenDataForProfile(fixture.configDir, fixture.blankSelector)
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(blank local name) error = %v", err)
	}
	if loaded.UserID != "" || loaded.AccessToken != fixture.blankToken.AccessToken {
		t.Fatalf("loaded token = %#v, want unresolved organization token", loaded)
	}
}

func TestCrossPlatformCoverageBlankProfileNameMatchingCorpNameRoundTripsPreviousProfile(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", false)

	selected, err := UsePreviousProfile(fixture.configDir)
	if err != nil {
		t.Fatalf("UsePreviousProfile() error = %v", err)
	}
	if selected == nil || selected.CorpID != fixture.corpID || selected.UserID != "" {
		t.Fatalf("selected previous profile = %#v, want unresolved profile", selected)
	}

	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != fixture.blankSelector || cfg.PreviousProfile != fixture.exactSelector {
		t.Fatalf(
			"profile pointers = current %q previous %q, want %q and %q",
			cfg.CurrentProfile,
			cfg.PreviousProfile,
			fixture.blankSelector,
			fixture.exactSelector,
		)
	}
}

func TestCrossPlatformCoverageBlankProfileNameMatchingCorpNameDeletesOnlyUnresolvedProfile(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", false)

	if err := DeleteTokenDataForProfile(fixture.configDir, fixture.blankSelector); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(blank local name) error = %v", err)
	}

	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].CorpID != fixture.corpID || cfg.Profiles[0].UserID != fixture.exactUserID {
		t.Fatalf("profiles after blank deletion = %#v, want only exact account", cfg.Profiles)
	}
	loaded, err := LoadTokenDataForProfile(fixture.configDir, fixture.exactSelector)
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(exact after blank deletion) error = %v", err)
	}
	if loaded.UserID != fixture.exactUserID || loaded.AccessToken != fixture.exactToken.AccessToken {
		t.Fatalf("exact token after blank deletion = %#v, want exact account preserved", loaded)
	}
}

func TestCrossPlatformCoverageBlankProfileNameContainingColonWinsOverIdentitySyntax(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "legacy:outsourced", "Fixture Organization", true)

	if fixture.blankSelector == fixture.blankName {
		t.Fatalf("colon-containing name leaked as selector %q", fixture.blankSelector)
	}
	if _, _, parsedAsIdentity := ParseIdentitySelector(fixture.blankSelector); parsedAsIdentity {
		t.Fatalf("stable blank selector %q was parsed as an identity", fixture.blankSelector)
	}
	selected, exact, err := ResolveProfileWithScope(fixture.configDir, "")
	if err != nil {
		t.Fatalf("ResolveProfileWithScope(colon local name) error = %v", err)
	}
	if selected == nil || selected.CorpID != fixture.corpID || selected.UserID != "" {
		t.Fatalf("resolved colon-name profile = %#v, want unresolved profile", selected)
	}
	if !exact {
		t.Fatal("colon-containing local name should resolve one exact unresolved profile")
	}

	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != fixture.blankSelector {
		t.Fatalf("current profile = %q, want migrated colon-name selector %q", cfg.CurrentProfile, fixture.blankSelector)
	}

	loaded, err := LoadTokenDataForProfile(fixture.configDir, fixture.blankSelector)
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(colon local name) error = %v", err)
	}
	if loaded.UserID != "" || loaded.AccessToken != fixture.blankToken.AccessToken {
		t.Fatalf("loaded colon-name token = %#v, want unresolved organization token", loaded)
	}
}

func TestCrossPlatformCoverageRealExactSelectorWinsOverMatchingBlankLegacyName(t *testing.T) {
	const exactSelector = "corp_selector_fixture:identity_exact_fixture"
	fixture := seedBlankProfileSelectorFixture(t, exactSelector, "Fixture Organization", true)

	selected, exact, err := ResolveProfileWithScope(fixture.configDir, "")
	if err != nil {
		t.Fatalf("ResolveProfileWithScope(current exact collision) error = %v", err)
	}
	if selected == nil || selected.UserID != fixture.exactUserID || !exact {
		t.Fatalf("resolved current collision = %#v exact=%v, want real exact identity", selected, exact)
	}
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != exactSelector {
		t.Fatalf("current collision selector = %q, want real exact %q", cfg.CurrentProfile, exactSelector)
	}

	blank, err := LoadTokenDataForProfile(fixture.configDir, fixture.blankSelector)
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(reserved blank collision) error = %v", err)
	}
	if blank.UserID != "" || blank.AccessToken != fixture.blankToken.AccessToken {
		t.Fatalf("reserved blank collision token = %#v", blank)
	}
	if err := DeleteTokenDataForProfile(fixture.configDir, fixture.blankSelector); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(reserved blank collision) error = %v", err)
	}
	exactToken, err := LoadTokenDataForProfile(fixture.configDir, exactSelector)
	if err != nil || exactToken.UserID != fixture.exactUserID {
		t.Fatalf("exact identity after blank collision delete = %#v, %v", exactToken, err)
	}
}

func TestCrossPlatformCoverageUnrelatedProfileDeletionPreservesBlankOrganizationCurrentMapping(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", true)
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Profiles = append(cfg.Profiles, Profile{
		Name:   "Unrelated Account",
		CorpID: "corp_unrelated_fixture",
		UserID: "identity_unrelated_fixture",
		Status: ProfileStatusActive,
	})
	if err := SaveProfiles(fixture.configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles(unrelated) error = %v", err)
	}
	if _, err := RemoveProfile(fixture.configDir, "corp_unrelated_fixture:identity_unrelated_fixture"); err != nil {
		t.Fatalf("RemoveProfile(unrelated) error = %v", err)
	}

	cfg, err = LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles(after unrelated delete) error = %v", err)
	}
	if cfg.CurrentProfile != fixture.blankSelector {
		t.Fatalf("blank current after unrelated delete = %q, want %q", cfg.CurrentProfile, fixture.blankSelector)
	}
	if got := cfg.OrgCurrentProfiles[fixture.corpID]; got != fixture.exactSelector {
		t.Fatalf("organization current after unrelated delete = %q, want %q", got, fixture.exactSelector)
	}
	selected, err := ResolveProfile(fixture.configDir, fixture.corpID)
	if err != nil || selected.UserID != fixture.exactUserID {
		t.Fatalf("organization selector after unrelated delete = %#v, %v", selected, err)
	}
}

func TestCrossPlatformCoverageSameCorpNonCurrentDeletionPreservesExactOrganizationCurrent(t *testing.T) {
	fixture := seedBlankProfileSelectorFixture(t, "Fixture Organization", "Fixture Organization", true)
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Profiles = append(cfg.Profiles, Profile{
		Name:     "Another Exact Account",
		CorpID:   fixture.corpID,
		CorpName: "Fixture Organization",
		UserID:   "identity_noncurrent_fixture",
		Status:   ProfileStatusActive,
	})
	if err := SaveProfiles(fixture.configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles(non-current exact) error = %v", err)
	}
	if _, err := RemoveProfile(fixture.configDir, fixture.corpID+":identity_noncurrent_fixture"); err != nil {
		t.Fatalf("RemoveProfile(non-current exact) error = %v", err)
	}

	cfg, err = LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles(after same-corp delete) error = %v", err)
	}
	if cfg.CurrentProfile != fixture.blankSelector {
		t.Fatalf("blank current after same-corp delete = %q, want %q", cfg.CurrentProfile, fixture.blankSelector)
	}
	if got := cfg.OrgCurrentProfiles[fixture.corpID]; got != fixture.exactSelector {
		t.Fatalf("organization current after same-corp delete = %q, want %q", got, fixture.exactSelector)
	}
	selected, err := ResolveProfile(fixture.configDir, fixture.corpID)
	if err != nil || selected.UserID != fixture.exactUserID {
		t.Fatalf("organization selector after same-corp delete = %#v, %v", selected, err)
	}
}
