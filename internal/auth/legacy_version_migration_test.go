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
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// The fixture builders spell out the exact v1 profiles and token JSON keys.
// They deliberately avoid the current TokenData and ProfilesConfig serializers
// so that historical field omission and account names stay part of the upgrade
// contract exercised by these tests.
func TestCrossPlatformCoverageV1044GlobalSlotWithoutProfilesMigrates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		userID string
	}{
		{name: "known user", userID: "legacy-user-v1044"},
		{name: "unresolved external worker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanupHistoricalKeychain(t)
			configDir := t.TempDir()
			corpID := "ding_v1044_" + strings.ReplaceAll(tc.name, " ", "_")
			seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
				t, "global-v1044-"+tc.name, corpID, "V1044 Org", tc.userID, "",
			))

			loaded, err := LoadTokenData(configDir)
			if err != nil {
				t.Fatalf("LoadTokenData() error = %v", err)
			}
			if loaded.CorpID != corpID || loaded.UserID != tc.userID {
				t.Fatalf("migrated v1.0.44 token = %#v", loaded)
			}
			if !TokenDataExistsKeychainForCorpID(corpID) {
				t.Fatal("v1.0.44 global token was not copied to its organization slot")
			}
			if tc.userID != "" && !TokenDataExistsKeychainForIdentity(corpID, tc.userID) {
				t.Fatal("v1.0.44 known identity slot was not created")
			}
		})
	}
}

func TestCrossPlatformCoverageV1050AndV1051GlobalSlotWithV1ProfilesMigratesIdentity(t *testing.T) {
	tests := []struct {
		name        string
		corpID      string
		corpName    string
		userID      string
		userName    string
		tokenHasUID bool
	}{
		{
			name:        "v1.0.50 token and profile both carry userId",
			corpID:      "ding_v1050",
			corpName:    "V1050 Org",
			userID:      "legacy-user-v1050",
			userName:    "V1050 User",
			tokenHasUID: true,
		},
		{
			name:        "v1.0.51 profile supplies omitted token userId",
			corpID:      "ding_v1051",
			corpName:    "V1051 Org",
			userID:      "legacy-user-v1051",
			userName:    "V1051 User",
			tokenHasUID: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanupHistoricalKeychain(t)
			configDir := t.TempDir()
			writeHistoricalV1Profiles(t, configDir, []historicalV1Profile{{
				name:     tc.corpName,
				corpID:   tc.corpID,
				corpName: tc.corpName,
				userID:   tc.userID,
				userName: tc.userName,
				clientID: "ding-client-" + tc.corpID,
			}}, tc.corpID, "", tc.corpID)

			tokenUserID, tokenUserName := "", ""
			if tc.tokenHasUID {
				tokenUserID, tokenUserName = tc.userID, tc.userName
			}
			seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
				t,
				"global-"+tc.corpID,
				tc.corpID,
				tc.corpName,
				tokenUserID,
				tokenUserName,
			))

			// An ordinary first read is the upgrade trigger. A recoverable global
			// token must populate both the organization and exact-identity slots
			// even when a version-1 profiles registry already exists.
			if _, err := LoadTokenData(configDir); err != nil {
				t.Fatalf("LoadTokenData() error = %v", err)
			}
			migrated, err := LoadTokenDataKeychainForIdentity(tc.corpID, tc.userID)
			if err != nil {
				t.Fatalf("LoadTokenDataKeychainForIdentity(%q, %q) error = %v", tc.corpID, tc.userID, err)
			}
			if migrated.CorpID != tc.corpID || migrated.UserID != tc.userID {
				t.Fatalf("migrated identity token = %#v", migrated)
			}
			if migrated.AccessToken != "global-"+tc.corpID ||
				migrated.RefreshToken != "refresh-global-"+tc.corpID ||
				migrated.PersistentCode != "persistent-global-"+tc.corpID ||
				migrated.ClientID != "ding-client-historical" ||
				migrated.Source != "mcp" {
				t.Fatalf("migrated token fields were not preserved: %#v", migrated)
			}
			orgMirror, err := LoadTokenDataKeychainForCorpID(tc.corpID)
			if err != nil {
				t.Fatalf("LoadTokenDataKeychainForCorpID(%q) error = %v", tc.corpID, err)
			}
			if orgMirror.AccessToken != "global-"+tc.corpID {
				t.Fatalf("organization token slot %q = %#v", TokenAccountForCorpID(tc.corpID), orgMirror)
			}
			if !tc.tokenHasUID && orgMirror.UserID != "" {
				t.Fatalf("organization mirror inferred userId %q; want untouched historical blob", orgMirror.UserID)
			}
		})
	}

}

func TestCrossPlatformCoverageV1052RawMultiOrganizationSlotsMigrateEveryIdentity(t *testing.T) {
	cleanupHistoricalKeychain(t)
	configDir := t.TempDir()
	organizations := seedHistoricalV1052MultiOrganizationState(t, configDir)

	// Reading only the current organization must upgrade the complete registry,
	// including inactive organizations that are not selected.
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.CorpID != organizations[1].corpID || loaded.UserID != organizations[1].userID {
		t.Fatalf("current token after migration = %#v", loaded)
	}
	assertHistoricalV1052IdentitySlots(t, organizations, nil)
}

func TestCrossPlatformCoverageV1052UnresolvedMultiOrganizationProfilesRemainUsable(t *testing.T) {
	cleanupHistoricalKeychain(t)
	configDir := t.TempDir()
	organizations := []historicalV1052Organization{
		{corpID: "ding_v1052_external_a", corpName: "External Org A", accessToken: "external-access-a"},
		{corpID: "ding_v1052_external_b", corpName: "External Org B", accessToken: "external-access-b"},
		{corpID: "ding_v1052_external_c", corpName: "External Org C", accessToken: "external-access-c"},
	}
	profiles := make([]historicalV1Profile, 0, len(organizations))
	for _, organization := range organizations {
		profiles = append(profiles, historicalV1Profile{
			name: organization.corpName, corpID: organization.corpID, corpName: organization.corpName,
		})
		seedHistoricalTokenSlot(t, TokenAccountForCorpID(organization.corpID), historicalTokenJSON(
			t, organization.accessToken, organization.corpID, organization.corpName, "", "",
		))
	}
	writeHistoricalV1Profiles(
		t,
		configDir,
		profiles,
		organizations[0].corpID,
		organizations[0].corpID,
		organizations[1].corpID,
	)

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	for _, organization := range organizations {
		loaded, err := LoadTokenDataForProfile(configDir, organization.corpID)
		if err != nil {
			t.Fatalf("LoadTokenDataForProfile(%q) error = %v", organization.corpID, err)
		}
		if loaded.AccessToken != organization.accessToken || loaded.CorpID != organization.corpID || loaded.UserID != "" {
			t.Fatalf("unresolved organization token for %q = %#v", organization.corpID, loaded)
		}
	}
}

func TestCrossPlatformCoverageV1052FirstSaveMigratesAllOrganizationsBeforeV2Commit(t *testing.T) {
	cleanupHistoricalKeychain(t)
	configDir := t.TempDir()
	organizations := seedHistoricalV1052MultiOrganizationState(t, configDir)

	// A login or refresh can make SaveTokenData the first new-version action.
	// It must migrate every old organization before profiles.json becomes v2,
	// otherwise the remaining organization mirrors are stranded permanently.
	firstWrite := &TokenData{
		AccessToken:    "new-first-action-access",
		RefreshToken:   "new-first-action-refresh",
		PersistentCode: "new-first-action-persistent",
		CorpID:         organizations[1].corpID,
		CorpName:       organizations[1].corpName,
		UserID:         organizations[1].userID,
		UserName:       organizations[1].userName,
		ClientID:       "ding-client-new-first-action",
		Source:         "mcp",
	}
	if err := SaveTokenData(configDir, firstWrite); err != nil {
		t.Fatalf("SaveTokenData(first new-version action) error = %v", err)
	}

	assertHistoricalV1052IdentitySlots(t, organizations, map[string]string{
		organizations[1].corpID: firstWrite.AccessToken,
	})
	migratedCurrent, err := LoadTokenDataKeychainForIdentity(firstWrite.CorpID, firstWrite.UserID)
	if err != nil {
		t.Fatalf("LoadTokenDataKeychainForIdentity(first write) error = %v", err)
	}
	if migratedCurrent.RefreshToken != firstWrite.RefreshToken ||
		migratedCurrent.PersistentCode != firstWrite.PersistentCode ||
		migratedCurrent.ClientID != firstWrite.ClientID ||
		migratedCurrent.Source != firstWrite.Source {
		t.Fatalf("first-write identity token fields were not preserved: %#v", migratedCurrent)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.Version != profilesVersion || len(cfg.Profiles) != len(organizations) {
		t.Fatalf("profiles after first save = %#v", cfg)
	}
}

func TestCrossPlatformCoverageLegacyGlobalFallbackIsStrictlyScoped(t *testing.T) {
	t.Run("different organization is never reused", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		writeHistoricalV1Profiles(t, configDir, []historicalV1Profile{{
			name:     "Expected Org",
			corpID:   "ding_expected",
			corpName: "Expected Org",
			userID:   "expected-user",
			userName: "Expected User",
			clientID: "ding-client-expected",
		}}, "ding_expected", "", "ding_expected")
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t,
			"wrong-org-access",
			"ding_other",
			"Other Org",
			"other-user",
			"Other User",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID("ding_expected") ||
			TokenDataExistsKeychainForIdentity("ding_expected", "expected-user") {
			t.Fatal("global token from another organization was reused")
		}
	})

	t.Run("version 2 empty tombstone never imports global slot", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		if err := SaveProfiles(configDir, &ProfilesConfig{Version: profilesVersion}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t,
			"stale-v2-global",
			"ding_v2",
			"V2 Org",
			"v2-user",
			"V2 User",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID("ding_v2") ||
			TokenDataExistsKeychainForIdentity("ding_v2", "v2-user") {
			t.Fatal("version 2 logout tombstone imported the stale global slot")
		}
	})

	t.Run("multiple same organization accounts never receive guessed identity", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		writeHistoricalV1Profiles(t, configDir, []historicalV1Profile{
			{
				name:     "Shared Org One",
				corpID:   "ding_shared",
				corpName: "Shared Org",
				userID:   "shared-user-one",
				userName: "Shared User One",
				clientID: "ding-client-shared",
			},
			{
				name:     "Shared Org Two",
				corpID:   "ding_shared",
				corpName: "Shared Org",
				userID:   "shared-user-two",
				userName: "Shared User Two",
				clientID: "ding-client-shared",
			},
		}, "ding_shared", "", "ding_shared")
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t,
			"shared-without-user",
			"ding_shared",
			"Shared Org",
			"",
			"",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if !TokenDataExistsKeychainForCorpID("ding_shared") {
			t.Fatal("matching organization mirror was not restored")
		}
		for _, userID := range []string{"shared-user-one", "shared-user-two"} {
			if TokenDataExistsKeychainForIdentity("ding_shared", userID) {
				t.Fatalf("ambiguous global token was copied to identity %q", userID)
			}
		}
	})
}

func TestCrossPlatformCoverageV1053PartialV2RegistryRepairsFromMatchingGlobalSlot(t *testing.T) {
	tests := []struct {
		name          string
		profileUserID string
		tokenUserID   string
	}{
		{
			name:          "matching token identity",
			profileUserID: "user_v1053_matching",
			tokenUserID:   "user_v1053_matching",
		},
		{name: "token omitted identity", profileUserID: "user_v1053_token_omitted"},
		{name: "sole unresolved profile"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanupHistoricalKeychain(t)
			configDir := t.TempDir()
			suffix := strings.ReplaceAll(tc.name, " ", "_")
			corpID := "ding_v1053_partial_" + suffix
			profile := Profile{
				Name:     "V1053 Partial Org",
				CorpID:   corpID,
				CorpName: "V1053 Partial Org",
				UserID:   tc.profileUserID,
				UserName: "V1053 Partial User",
			}
			identityLoads := 0
			if profile.UserID == "" {
				originalLoadIdentity := profilesLoadIdentity
				profilesLoadIdentity = func(corpID, userID string) (*TokenData, error) {
					identityLoads++
					return originalLoadIdentity(corpID, userID)
				}
				t.Cleanup(func() { profilesLoadIdentity = originalLoadIdentity })
			}
			selector := ProfileSelector(profile)
			if err := SaveProfiles(configDir, &ProfilesConfig{
				Version:            profilesVersion,
				CurrentProfile:     selector,
				OrgCurrentProfiles: map[string]string{corpID: selector},
				Profiles:           []Profile{profile},
			}); err != nil {
				t.Fatalf("SaveProfiles() error = %v", err)
			}
			seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
				t,
				"v1053-global-"+suffix,
				corpID,
				profile.CorpName,
				tc.tokenUserID,
				"",
			))

			if TokenDataExistsKeychainForCorpID(corpID) {
				t.Fatal("partial v2 fixture unexpectedly contained an organization or identity slot")
			}
			if profile.UserID != "" && TokenDataExistsKeychainForIdentity(corpID, profile.UserID) {
				t.Fatal("partial v2 fixture unexpectedly contained an identity slot")
			}
			loaded, err := LoadTokenDataForProfile(configDir, selector)
			if err != nil {
				t.Fatalf("LoadTokenDataForProfile() error = %v", err)
			}
			if loaded.AccessToken != "v1053-global-"+suffix ||
				loaded.CorpID != corpID || loaded.UserID != profile.UserID {
				t.Fatalf("repaired v2 token = %#v", loaded)
			}

			if profile.UserID != "" {
				identityToken, identityErr := LoadTokenDataKeychainForIdentity(corpID, profile.UserID)
				if identityErr != nil {
					t.Fatalf("LoadTokenDataKeychainForIdentity() error = %v", identityErr)
				}
				if identityToken.AccessToken != loaded.AccessToken || identityToken.UserID != profile.UserID {
					t.Fatalf("repaired identity token = %#v", identityToken)
				}
			}
			orgMirror, err := LoadTokenDataKeychainForCorpID(corpID)
			if err != nil {
				t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v", err)
			}
			if orgMirror.AccessToken != loaded.AccessToken || orgMirror.UserID != tc.tokenUserID {
				t.Fatalf("repaired organization mirror = %#v", orgMirror)
			}
			if identityLoads != 0 {
				t.Fatalf("unresolved profile caused %d identity-slot reads; want 0", identityLoads)
			}
		})
	}

	t.Run("matching organization among multiple organizations", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		profiles := []Profile{
			{Name: "Partial Org A", CorpID: "ding_v1053_partial_a", UserID: "user_v1053_partial_a"},
			{Name: "Partial Org B", CorpID: "ding_v1053_partial_b", UserID: "user_v1053_partial_b"},
		}
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version:        profilesVersion,
			CurrentProfile: ProfileSelector(profiles[1]),
			Profiles:       profiles,
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t,
			"v1053-global-multi-org",
			profiles[1].CorpID,
			profiles[1].Name,
			profiles[1].UserID,
			"",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID(profiles[0].CorpID) ||
			TokenDataExistsKeychainForIdentity(profiles[0].CorpID, profiles[0].UserID) {
			t.Fatal("global token was copied into the non-matching organization")
		}
		repaired, err := LoadTokenDataKeychainForIdentity(profiles[1].CorpID, profiles[1].UserID)
		if err != nil {
			t.Fatalf("LoadTokenDataKeychainForIdentity(matching organization) error = %v", err)
		}
		if repaired.AccessToken != "v1053-global-multi-org" || repaired.UserID != profiles[1].UserID {
			t.Fatalf("multi-organization v2 repair token = %#v", repaired)
		}
	})
}

func TestCrossPlatformCoverageV1053PartialV2RegistryRejectsUnsafeGlobalSlot(t *testing.T) {
	t.Run("global token organization differs", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		profile := Profile{Name: "Expected Org", CorpID: "ding_v2_expected", UserID: "user_v2_expected"}
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version:        profilesVersion,
			CurrentProfile: ProfileSelector(profile),
			Profiles:       []Profile{profile},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t, "cross-corp-global", "ding_v2_other", "Other Org", profile.UserID, "",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID(profile.CorpID) ||
			TokenDataExistsKeychainForIdentity(profile.CorpID, profile.UserID) {
			t.Fatal("cross-organization global token was imported")
		}
	})

	t.Run("unresolved profile rejects global token identity", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		profile := Profile{Name: "Unresolved External", CorpID: "ding_v2_unresolved"}
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version:        profilesVersion,
			CurrentProfile: ProfileSelector(profile),
			Profiles:       []Profile{profile},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t, "unexpected-identity-global", profile.CorpID, profile.Name, "user_v2_unexpected", "",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID(profile.CorpID) {
			t.Fatal("global token userId was attached to an unresolved profile")
		}
	})

	t.Run("global token identity differs", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		profile := Profile{Name: "Expected User", CorpID: "ding_v2_uid", UserID: "user_v2_expected"}
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version:        profilesVersion,
			CurrentProfile: ProfileSelector(profile),
			Profiles:       []Profile{profile},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t, "wrong-user-global", profile.CorpID, profile.Name, "user_v2_other", "",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID(profile.CorpID) ||
			TokenDataExistsKeychainForIdentity(profile.CorpID, profile.UserID) {
			t.Fatal("global token with a different userId was imported")
		}
	})

	t.Run("multiple accounts in one organization stay unresolved", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		corpID := "ding_v2_shared"
		profiles := []Profile{
			{Name: "Shared User One", CorpID: corpID, UserID: "user_v2_shared_one"},
			{Name: "Shared User Two", CorpID: corpID, UserID: "user_v2_shared_two"},
		}
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version:        profilesVersion,
			CurrentProfile: ProfileSelector(profiles[0]),
			Profiles:       profiles,
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t, "ambiguous-global", corpID, "Shared Org", profiles[0].UserID, "",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID(corpID) {
			t.Fatal("multi-account organization imported the mutable global mirror")
		}
		for _, profile := range profiles {
			if TokenDataExistsKeychainForIdentity(corpID, profile.UserID) {
				t.Fatalf("multi-account global token was copied to identity %q", profile.UserID)
			}
		}
	})

	t.Run("existing exact identity is not overwritten", func(t *testing.T) {
		cleanupHistoricalKeychain(t)
		configDir := t.TempDir()
		profile := Profile{Name: "Existing User", CorpID: "ding_v2_existing", UserID: "user_v2_existing"}
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version:        profilesVersion,
			CurrentProfile: ProfileSelector(profile),
			Profiles:       []Profile{profile},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		seedHistoricalTokenSlot(t, TokenAccountForIdentity(profile.CorpID, profile.UserID), historicalTokenJSON(
			t, "existing-exact", profile.CorpID, profile.Name, profile.UserID, "",
		))
		seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
			t, "stale-global", profile.CorpID, profile.Name, profile.UserID, "",
		))

		if err := EnsureProfilesMigration(configDir); err != nil {
			t.Fatalf("EnsureProfilesMigration() error = %v", err)
		}
		if TokenDataExistsKeychainForCorpID(profile.CorpID) {
			t.Fatal("global mirror recreated an organization slot beside an existing exact identity")
		}
		exact, err := LoadTokenDataKeychainForIdentity(profile.CorpID, profile.UserID)
		if err != nil {
			t.Fatalf("LoadTokenDataKeychainForIdentity() error = %v", err)
		}
		if exact.AccessToken != "existing-exact" {
			t.Fatalf("existing exact identity was overwritten: %#v", exact)
		}
	})
}

func TestCrossPlatformCoverageLegacyMigrationPersistenceErrors(t *testing.T) {
	oldLoad := profilesLoad
	oldSave := profilesSave
	oldLoadLegacy := profilesLoadLegacy
	oldSaveCorp := profilesSaveCorp
	oldLoadCorp := profilesLoadCorp
	oldLoadIdentity := profilesLoadIdentity
	oldSaveIdentity := profilesSaveIdentity
	t.Cleanup(func() {
		profilesLoad = oldLoad
		profilesSave = oldSave
		profilesLoadLegacy = oldLoadLegacy
		profilesSaveCorp = oldSaveCorp
		profilesLoadCorp = oldLoadCorp
		profilesLoadIdentity = oldLoadIdentity
		profilesSaveIdentity = oldSaveIdentity
	})

	fail := errors.New("legacy migration persistence failed")
	baseConfig := func(version int) *ProfilesConfig {
		return &ProfilesConfig{
			Version: version,
			Profiles: []Profile{{
				Name: "Legacy User", CorpID: "ding_legacy_error", UserID: "legacy-user",
			}},
		}
	}
	profilesSave = func(string, *ProfilesConfig) error { return nil }
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesSaveIdentity = func(string, string, *TokenData) error { return nil }

	t.Run("global compatibility slot read", func(t *testing.T) {
		profilesLoad = func(string) (*ProfilesConfig, error) { return baseConfig(1), nil }
		profilesLoadLegacy = func() (*TokenData, error) { return nil, fail }
		profilesSaveCorp = func(string, *TokenData) error { return nil }
		if err := ensureProfilesMigrationLocked("cfg"); !errors.Is(err, fail) {
			t.Fatalf("ensureProfilesMigrationLocked() error = %v, want %v", err, fail)
		}
	})

	t.Run("restored organization slot write", func(t *testing.T) {
		profilesLoad = func(string) (*ProfilesConfig, error) { return baseConfig(1), nil }
		profilesLoadLegacy = func() (*TokenData, error) {
			return &TokenData{CorpID: "ding_legacy_error", AccessToken: "legacy-access"}, nil
		}
		profilesSaveCorp = func(string, *TokenData) error { return fail }
		if err := ensureProfilesMigrationLocked("cfg"); !errors.Is(err, fail) {
			t.Fatalf("ensureProfilesMigrationLocked() error = %v, want %v", err, fail)
		}
	})

	t.Run("repaired identity slot write", func(t *testing.T) {
		profilesLoad = func(string) (*ProfilesConfig, error) { return baseConfig(profilesVersion), nil }
		profilesLoadCorp = func(string) (*TokenData, error) {
			return &TokenData{CorpID: "ding_legacy_error", AccessToken: "legacy-access"}, nil
		}
		profilesSaveIdentity = func(string, string, *TokenData) error { return fail }
		if err := ensureProfilesMigrationLocked("cfg"); !errors.Is(err, fail) {
			t.Fatalf("ensureProfilesMigrationLocked() error = %v, want %v", err, fail)
		}
	})

	t.Run("partial v2 identity slot read", func(t *testing.T) {
		profilesLoad = func(string) (*ProfilesConfig, error) { return baseConfig(profilesVersion), nil }
		profilesLoadCorp = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
		profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, fail }
		if err := ensureProfilesMigrationLocked("cfg"); !errors.Is(err, fail) {
			t.Fatalf("ensureProfilesMigrationLocked() error = %v, want %v", err, fail)
		}
	})
}

type historicalV1Profile struct {
	name     string
	corpID   string
	corpName string
	userID   string
	userName string
	clientID string
}

type historicalV1052Organization struct {
	corpID      string
	corpName    string
	userID      string
	userName    string
	accessToken string
}

func seedHistoricalV1052MultiOrganizationState(t *testing.T, configDir string) []historicalV1052Organization {
	t.Helper()
	organizations := []historicalV1052Organization{
		{corpID: "ding_v1052_a", corpName: "V1052 Org A", userID: "legacy-user-v1052-a", userName: "V1052 User A", accessToken: "v1052-access-a"},
		{corpID: "ding_v1052_b", corpName: "V1052 Org B", userID: "legacy-user-v1052-b", userName: "V1052 User B", accessToken: "v1052-access-b"},
		{corpID: "ding_v1052_c", corpName: "V1052 Org C", userID: "legacy-user-v1052-c", userName: "V1052 User C", accessToken: "v1052-access-c"},
	}
	profiles := make([]historicalV1Profile, 0, len(organizations))
	for _, organization := range organizations {
		profiles = append(profiles, historicalV1Profile{
			name:     organization.corpName,
			corpID:   organization.corpID,
			corpName: organization.corpName,
			userID:   organization.userID,
			userName: organization.userName,
			clientID: "ding-client-v1052",
		})
	}
	writeHistoricalV1Profiles(
		t,
		configDir,
		profiles,
		organizations[0].corpID,
		organizations[0].corpID,
		organizations[1].corpID,
	)

	for _, organization := range organizations {
		// v1.0.52 MCP responses commonly omitted userId while profiles.json
		// retained a uniquely known identity.
		seedHistoricalTokenSlot(t, TokenAccountForCorpID(organization.corpID), historicalTokenJSON(
			t,
			organization.accessToken,
			organization.corpID,
			organization.corpName,
			"",
			"",
		))
	}
	// v1.0.52 also mirrored the selected organization into the global account.
	current := organizations[1]
	seedHistoricalTokenSlot(t, keychain.AccountToken, historicalTokenJSON(
		t,
		current.accessToken,
		current.corpID,
		current.corpName,
		"",
		"",
	))
	return organizations
}

func assertHistoricalV1052IdentitySlots(
	t *testing.T,
	organizations []historicalV1052Organization,
	accessOverrides map[string]string,
) {
	t.Helper()
	for _, organization := range organizations {
		migrated, err := LoadTokenDataKeychainForIdentity(organization.corpID, organization.userID)
		if err != nil {
			t.Errorf("LoadTokenDataKeychainForIdentity(%q, %q) error = %v", organization.corpID, organization.userID, err)
			continue
		}
		wantAccess := organization.accessToken
		if override := accessOverrides[organization.corpID]; override != "" {
			wantAccess = override
		}
		if migrated.AccessToken != wantAccess ||
			migrated.CorpID != organization.corpID ||
			migrated.UserID != organization.userID {
			t.Errorf(
				"migrated token for %q = %#v, want access=%q userId=%q",
				organization.corpID,
				migrated,
				wantAccess,
				organization.userID,
			)
		}
		if accessOverrides[organization.corpID] == "" &&
			(migrated.RefreshToken != "refresh-"+organization.accessToken ||
				migrated.PersistentCode != "persistent-"+organization.accessToken ||
				migrated.ClientID != "ding-client-historical" ||
				migrated.Source != "mcp") {
			t.Errorf("historical token fields for %q were not preserved: %#v", organization.corpID, migrated)
		}
	}
}

func historicalTokenJSON(t *testing.T, accessToken, corpID, corpName, userID, userName string) string {
	t.Helper()
	fixture := map[string]any{
		"access_token":       accessToken,
		"refresh_token":      "refresh-" + accessToken,
		"persistent_code":    "persistent-" + accessToken,
		"expires_at":         "2030-01-02T03:04:05Z",
		"refresh_expires_at": "2030-02-02T03:04:05Z",
		"corp_id":            corpID,
		"corp_name":          corpName,
		"client_id":          "ding-client-historical",
		"source":             "mcp",
	}
	if userID != "" {
		fixture["user_id"] = userID
	}
	if userName != "" {
		fixture["user_name"] = userName
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("marshal historical token fixture: %v", err)
	}
	return string(data)
}

func writeHistoricalV1Profiles(
	t *testing.T,
	configDir string,
	profiles []historicalV1Profile,
	primaryProfile string,
	previousProfile string,
	currentProfile string,
) {
	t.Helper()
	rawProfiles := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		rawProfiles = append(rawProfiles, map[string]any{
			"name":         profile.name,
			"corpId":       profile.corpID,
			"corpName":     profile.corpName,
			"userId":       profile.userID,
			"userName":     profile.userName,
			"clientId":     profile.clientID,
			"status":       "active",
			"expiresAt":    "2030-01-02T03:04:05Z",
			"refreshExpAt": "2030-02-02T03:04:05Z",
		})
	}
	fixture := map[string]any{
		"version":         1,
		"primaryProfile":  primaryProfile,
		"currentProfile":  currentProfile,
		"previousProfile": previousProfile,
		"profiles":        rawProfiles,
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("marshal historical profiles fixture: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create historical config directory: %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write historical profiles.json: %v", err)
	}
}

func seedHistoricalTokenSlot(t *testing.T, account, raw string) {
	t.Helper()
	if err := keychain.Set(keychain.Service, account, raw); err != nil {
		t.Fatalf("seed historical keychain account %q: %v", account, err)
	}
}

func cleanupHistoricalKeychain(t *testing.T) {
	t.Helper()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
}
