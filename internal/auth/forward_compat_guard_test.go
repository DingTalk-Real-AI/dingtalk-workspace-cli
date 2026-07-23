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
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestDefaultBlankCurrentRejectsSameCorpGlobalMirrorOwnedByExactIdentity(t *testing.T) {
	originalResolveProfile := tokenResolveProfile
	originalLoadOrganization := tokenLoadKeychainForCorpID
	originalLoadGlobal := tokenLoadKeychain
	t.Cleanup(func() {
		tokenResolveProfile = originalResolveProfile
		tokenLoadKeychainForCorpID = originalLoadOrganization
		tokenLoadKeychain = originalLoadGlobal
	})

	const corpID = "corp_blank_default_guard"
	tokenResolveProfile = func(string, string) (*Profile, error) {
		return &Profile{Name: "Legacy Contractor", CorpID: corpID}, nil
	}
	tokenLoadKeychainForCorpID = func(string) (*TokenData, error) {
		return nil, ErrTokenDataNotFound
	}
	tokenLoadKeychain = func() (*TokenData, error) {
		return &TokenData{
			AccessToken: "exact-global-token",
			CorpID:      corpID,
			UserID:      "different-exact-user",
		}, nil
	}

	loaded, err := loadTokenDataForProfileLocked(t.TempDir(), "")
	if !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("loadTokenDataForProfileLocked() error = %v, want %v", err, ErrTokenDataNotFound)
	}
	if loaded != nil {
		t.Fatalf("loadTokenDataForProfileLocked() = %#v, want nil", loaded)
	}
}

func TestTokenLoadIsolationMatrix(t *testing.T) {
	const (
		corpID      = "corp_token_load_matrix"
		exactUserID = "matrix-user-a"
		otherUserID = "matrix-user-b"
	)
	type profileKind string
	type tokenSlot string
	type uidRelation string
	const (
		blankProfile profileKind = "blank"
		exactProfile profileKind = "exact"
		identitySlot tokenSlot   = "identity"
		orgSlot      tokenSlot   = "organization"
		globalSlot   tokenSlot   = "global"
		emptyUID     uidRelation = "empty"
		sameUID      uidRelation = "same"
		differentUID uidRelation = "different"
	)

	var tests []struct {
		name         string
		profile      Profile
		slot         tokenSlot
		tokenUserID  string
		wantAccepted bool
	}
	for _, kind := range []profileKind{blankProfile, exactProfile} {
		profile := Profile{Name: string(kind) + " matrix profile", CorpID: corpID}
		if kind == exactProfile {
			profile.UserID = exactUserID
		}
		for _, slot := range []tokenSlot{identitySlot, orgSlot, globalSlot} {
			for _, relation := range []uidRelation{emptyUID, sameUID, differentUID} {
				tokenUserID := ""
				switch relation {
				case sameUID:
					tokenUserID = profile.UserID
				case differentUID:
					tokenUserID = otherUserID
				}
				wantAccepted := false
				switch slot {
				case identitySlot:
					wantAccepted = kind == exactProfile && tokenUserID == profile.UserID
				case orgSlot, globalSlot:
					wantAccepted = tokenUserID == profile.UserID
				}
				tests = append(tests, struct {
					name         string
					profile      Profile
					slot         tokenSlot
					tokenUserID  string
					wantAccepted bool
				}{
					name:         string(kind) + "/" + string(slot) + "/" + string(relation),
					profile:      profile,
					slot:         slot,
					tokenUserID:  tokenUserID,
					wantAccepted: wantAccepted,
				})
			}
		}
	}
	if len(tests) != 18 {
		t.Fatalf("token isolation matrix has %d cells, want 18", len(tests))
	}

	originalResolveProfile := tokenResolveProfile
	originalLoadIdentity := tokenLoadKeychainIdentity
	originalLoadOrganization := tokenLoadKeychainForCorpID
	originalLoadGlobal := tokenLoadKeychain
	originalSaveIdentity := tokenSaveKeychainForIdentity
	t.Cleanup(func() {
		tokenResolveProfile = originalResolveProfile
		tokenLoadKeychainIdentity = originalLoadIdentity
		tokenLoadKeychainForCorpID = originalLoadOrganization
		tokenLoadKeychain = originalLoadGlobal
		tokenSaveKeychainForIdentity = originalSaveIdentity
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &TokenData{
				AccessToken: "matrix-token-" + strings.ReplaceAll(tt.name, "/", "-"),
				CorpID:      corpID,
				UserID:      tt.tokenUserID,
			}
			tokenResolveProfile = func(string, string) (*Profile, error) {
				profile := tt.profile
				return &profile, nil
			}
			tokenLoadKeychainIdentity = func(string, string) (*TokenData, error) {
				if tt.slot == identitySlot {
					return token, nil
				}
				return nil, ErrTokenDataNotFound
			}
			tokenLoadKeychainForCorpID = func(string) (*TokenData, error) {
				if tt.slot == orgSlot {
					return token, nil
				}
				return nil, ErrTokenDataNotFound
			}
			tokenLoadKeychain = func() (*TokenData, error) {
				if tt.slot == globalSlot {
					return token, nil
				}
				return nil, ErrTokenDataNotFound
			}
			tokenSaveKeychainForIdentity = func(string, string, *TokenData) error { return nil }

			loaded, err := loadTokenDataForProfileLocked(t.TempDir(), "")
			if tt.wantAccepted {
				if err != nil || loaded == nil || loaded.AccessToken != token.AccessToken {
					t.Fatalf("matrix cell accepted = false: loaded=%#v err=%v", loaded, err)
				}
				return
			}
			if err == nil || loaded != nil {
				t.Fatalf("matrix cell accepted = true: loaded=%#v err=%v", loaded, err)
			}
		})
	}

	// Slot names do not make a token from another organization trustworthy.
	for _, profile := range []Profile{
		{Name: "blank wrong-corp matrix profile", CorpID: corpID},
		{Name: "exact wrong-corp matrix profile", CorpID: corpID, UserID: exactUserID},
	} {
		t.Run(profile.Name, func(t *testing.T) {
			tokenResolveProfile = func(string, string) (*Profile, error) {
				selected := profile
				return &selected, nil
			}
			tokenLoadKeychainIdentity = func(string, string) (*TokenData, error) {
				return &TokenData{AccessToken: "wrong-corp", CorpID: "corp_other", UserID: profile.UserID}, nil
			}
			tokenLoadKeychainForCorpID = func(string) (*TokenData, error) {
				return &TokenData{AccessToken: "wrong-corp", CorpID: "corp_other", UserID: profile.UserID}, nil
			}
			tokenLoadKeychain = func() (*TokenData, error) { return nil, ErrTokenDataNotFound }
			if loaded, err := loadTokenDataForProfileLocked(t.TempDir(), ""); err == nil || loaded != nil {
				t.Fatalf("wrong-corp token accepted: loaded=%#v err=%v", loaded, err)
			}
		})
	}
}

func TestTokenPersistenceWritePlanKeepsOrganizationOwnershipIsolated(t *testing.T) {
	const (
		corpID       = "corp_write_plan"
		resolvedUID  = "resolved-user"
		coexistingID = "coexisting-user"
	)
	tests := []struct {
		name             string
		cfg              *ProfilesConfig
		data             *TokenData
		runtimeSelector  string
		wantUpgrade      bool
		wantOrganization bool
		wantMakeCurrent  bool
	}{
		{
			name: "explicit reauth completes sole unresolved profile",
			cfg: &ProfilesConfig{
				Version:  profilesVersion,
				Profiles: []Profile{{Name: "Historical Worker", CorpID: corpID}},
			},
			data:             &TokenData{CorpID: corpID, UserID: resolvedUID},
			runtimeSelector:  corpID,
			wantUpgrade:      true,
			wantOrganization: true,
		},
		{
			name: "coexisting exact identity cannot overwrite unresolved owner",
			cfg: &ProfilesConfig{
				Version: profilesUnresolvedSelectorVersion,
				Profiles: []Profile{
					{Name: "Historical Worker", CorpID: corpID},
					{Name: "Exact Worker", CorpID: corpID, UserID: coexistingID},
				},
			},
			data:             &TokenData{CorpID: corpID, UserID: coexistingID},
			runtimeSelector:  profileSelector(corpID, coexistingID),
			wantOrganization: false,
		},
		{
			name: "default exact login without unresolved owner updates organization",
			cfg: &ProfilesConfig{
				Version:  profilesVersion,
				Profiles: []Profile{{Name: "Exact Worker", CorpID: corpID, UserID: resolvedUID}},
			},
			data:             &TokenData{CorpID: corpID, UserID: resolvedUID},
			wantOrganization: true,
			wantMakeCurrent:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := planTokenPersistenceWrites(tt.cfg, tt.data, tt.runtimeSelector)
			if !plan.WriteIdentity || !plan.WriteGlobal {
				t.Fatalf("write plan = %#v, want identity and global slots included", plan)
			}
			if plan.UpgradesLegacyProfile != tt.wantUpgrade {
				t.Fatalf("UpgradesLegacyProfile = %v, want %v", plan.UpgradesLegacyProfile, tt.wantUpgrade)
			}
			if plan.WriteOrganization != tt.wantOrganization {
				t.Fatalf("WriteOrganization = %v, want %v", plan.WriteOrganization, tt.wantOrganization)
			}
			if plan.MakeCurrent != tt.wantMakeCurrent {
				t.Fatalf("MakeCurrent = %v, want %v", plan.MakeCurrent, tt.wantMakeCurrent)
			}
		})
	}
}

func TestFreshUIDLessExactLoginCannotOverwriteExistingUnresolvedProfile(t *testing.T) {
	fixture := seedLegacyBlankAndExactIdentitySlots(t)
	exactSelector := profileSelector(fixture.corpID, fixture.alpha.UserID)
	previousRuntimeProfile := RuntimeProfile()
	SetRuntimeProfile(exactSelector)
	t.Cleanup(func() { SetRuntimeProfile(previousRuntimeProfile) })

	freshUnknown := testToken("fresh-unknown-exact-target", fixture.corpID, fixture.blank.CorpName)
	freshUnknown.UserID = ""
	freshUnknown.UserName = ""
	freshUnknown.LegacyOrgScopedProfile = exactSelector
	freshUnknown.FreshAuthorization = true

	if err := preflightTokenWritePersistence(fixture.configDir, freshUnknown); err == nil ||
		!strings.Contains(err.Error(), "refusing to save UID-less token") {
		t.Fatalf("preflightTokenWritePersistence() error = %v, want unresolved-sibling protection", err)
	}
	if err := SaveTokenData(fixture.configDir, freshUnknown); err == nil ||
		!strings.Contains(err.Error(), "refusing to save UID-less token") {
		t.Fatalf("SaveTokenData() error = %v, want unresolved-sibling protection", err)
	}

	assertOrganizationTokenAccessForTest(t, fixture.corpID, fixture.blank.AccessToken, "")
	assertIdentityTokenAccessForTest(t, fixture.corpID, fixture.alpha.UserID, fixture.alpha.AccessToken)
	assertIdentityTokenAccessForTest(t, fixture.corpID, fixture.beta.UserID, fixture.beta.AccessToken)

	// The same fresh authorization is also ambiguous without --profile. It must
	// not inherit or overwrite whichever same-corp account happens to be current.
	SetRuntimeProfile("")
	freshUnknown.LegacyOrgScopedProfile = ""
	if err := preflightTokenWritePersistence(fixture.configDir, freshUnknown); err == nil ||
		!strings.Contains(err.Error(), "fresh UID-less token") {
		t.Fatalf("implicit preflight error = %v, want multi-account unresolved protection", err)
	}
	if err := SaveLoginTokenData(fixture.configDir, freshUnknown); err == nil ||
		!strings.Contains(err.Error(), "fresh UID-less token") {
		t.Fatalf("implicit SaveLoginTokenData() error = %v, want multi-account unresolved protection", err)
	}
	assertOrganizationTokenAccessForTest(t, fixture.corpID, fixture.blank.AccessToken, "")
	assertIdentityTokenAccessForTest(t, fixture.corpID, fixture.alpha.UserID, fixture.alpha.AccessToken)

	// Explicitly targeting the unresolved account remains a supported reauth
	// path for external workers whose contact response still has no userId.
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	unresolved := unresolvedProfileForCorp(cfg, fixture.corpID)
	if unresolved == nil {
		t.Fatal("unresolved profile missing")
	}
	unresolvedSelector := storedProfileSelector(cfg, unresolved)
	SetRuntimeProfile(unresolvedSelector)
	freshUnknown.LegacyOrgScopedProfile = unresolvedSelector
	if err := SaveTokenData(fixture.configDir, freshUnknown); err != nil {
		t.Fatalf("SaveTokenData(explicit unresolved reauth) error = %v", err)
	}
	assertOrganizationTokenAccessForTest(t, fixture.corpID, freshUnknown.AccessToken, "")
}

func TestCanonicalStoredSelectorKeepsCorpIDPriorityOverLegacyLocalName(t *testing.T) {
	const (
		organizationCorpID = "corp_selector_priority"
		legacyCorpID       = "corp_legacy_local_name"
	)
	cfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{
			{
				Name:     "Organization Account",
				CorpID:   organizationCorpID,
				CorpName: "Organization",
				UserID:   "organization-user",
			},
			{
				Name:     organizationCorpID,
				CorpID:   legacyCorpID,
				CorpName: "Legacy Organization",
			},
			{
				Name:     "Legacy Exact Account",
				CorpID:   legacyCorpID,
				CorpName: "Legacy Organization",
				UserID:   "legacy-exact-user",
			},
		},
	}

	want := profileSelector(organizationCorpID, "organization-user")
	if got := canonicalStoredSelector(cfg, organizationCorpID); got != want {
		t.Fatalf("canonicalStoredSelector(CorpID conflict) = %q, want organization selector %q", got, want)
	}
}

func TestCanonicalStoredSelectorKeepsCorpNamePriorityOverLegacyLocalName(t *testing.T) {
	const (
		organizationCorpID = "corp_name_priority"
		legacyCorpID       = "corp_legacy_corp_name"
		organizationName   = "Shared Organization Name"
	)
	cfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{
			{
				Name:     "Organization Account",
				CorpID:   organizationCorpID,
				CorpName: organizationName,
				UserID:   "organization-user",
			},
			{
				Name:     organizationName,
				CorpID:   legacyCorpID,
				CorpName: "Legacy Organization",
			},
			{
				Name:     "Legacy Exact Account",
				CorpID:   legacyCorpID,
				CorpName: "Legacy Organization",
				UserID:   "legacy-exact-user",
			},
		},
	}

	want := profileSelector(organizationCorpID, "organization-user")
	if got := canonicalStoredSelector(cfg, organizationName); got != want {
		t.Fatalf("canonicalStoredSelector(CorpName conflict) = %q, want organization selector %q", got, want)
	}
}

func TestCanonicalStoredSelectorRecoversUncontestedLegacyLocalName(t *testing.T) {
	const (
		corpID    = "corp_uncontested_local_name"
		localName = "Historical External Worker"
	)
	cfg := &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{
			{
				Name:     localName,
				CorpID:   corpID,
				CorpName: "Local Name Organization",
			},
			{
				Name:     "Exact Account",
				CorpID:   corpID,
				CorpName: "Local Name Organization",
				UserID:   "exact-user",
			},
		},
	}

	got := canonicalStoredSelector(cfg, localName)
	if got != localName {
		t.Fatalf("canonicalStoredSelector(uncontested local name) = %q, want %q", got, localName)
	}
	selected, exact, err := resolveProfileSelection("", cfg, got)
	if err != nil {
		t.Fatalf("resolveProfileSelection(recovered local name) error = %v", err)
	}
	if !exact || selected == nil || selected.CorpID != corpID || selected.UserID != "" {
		t.Fatalf("recovered local selector resolved to %#v exact=%v, want unresolved profile", selected, exact)
	}
}

func TestSetCurrentProfileRejectsUnreadableExactIdentityBesideBlankProfile(t *testing.T) {
	fixture := installUnreadableExactProfileSelectionFixture(t, false)

	selected, err := setCurrentProfileLocked(fixture.configDir, fixture.unreadableSelector)
	if !errors.Is(err, fixture.unreadableErr) {
		t.Fatalf("setCurrentProfileLocked() = %#v, %v; want unreadable token error", selected, err)
	}
	fixture.assertUnchanged(t)
}

func TestUsePreviousProfileRejectsUnreadableExactIdentityBesideBlankProfile(t *testing.T) {
	fixture := installUnreadableExactProfileSelectionFixture(t, true)

	selected, err := usePreviousProfileLocked(fixture.configDir)
	if !errors.Is(err, fixture.unreadableErr) {
		t.Fatalf("usePreviousProfileLocked() = %#v, %v; want unreadable token error", selected, err)
	}
	fixture.assertUnchanged(t)
}

func TestSetCurrentProfileRejectsMismatchedExactIdentitySlotBesideBlankProfile(t *testing.T) {
	fixture := installUnreadableExactProfileSelectionFixture(t, false)
	fixture.unreadableToken = &TokenData{
		AccessToken: "readable-but-wrong-identity-token",
		CorpID:      fixture.corpID,
		UserID:      "another-exact-user",
	}

	selected, err := setCurrentProfileLocked(fixture.configDir, fixture.unreadableSelector)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("setCurrentProfileLocked() = %#v, %v; want identity mismatch error", selected, err)
	}
	fixture.assertUnchanged(t)
}

func TestReservedUnresolvedSelectorPromotesSchemaToDowngradeGuardVersion(t *testing.T) {
	const wantDowngradeGuardVersion = 3
	configDir := t.TempDir()
	corpID := "corp_reserved_schema_guard"
	reserved := unresolvedProfileSelector(corpID)
	cfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: reserved,
		Profiles: []Profile{
			{Name: "Reserved Organization", CorpID: corpID, CorpName: "Reserved Organization"},
			{Name: "Exact Account", CorpID: corpID, CorpName: "Reserved Organization", UserID: "exact-user"},
		},
	}

	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	loaded, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if loaded.Version != wantDowngradeGuardVersion {
		t.Fatalf("profiles version = %d, want downgrade guard version %d", loaded.Version, wantDowngradeGuardVersion)
	}
	if loaded.CurrentProfile != reserved {
		t.Fatalf("current profile = %q, want %q", loaded.CurrentProfile, reserved)
	}
	if err := ensureProfilesWritable(loaded); err != nil {
		t.Fatalf("current client rejected reserved-selector schema: %v", err)
	}
}

func TestV2ProfilesWithoutReservedSelectorRemainDowngradeReadable(t *testing.T) {
	configDir := t.TempDir()
	cfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: "corp_v2_safe:user_v2_safe",
		Profiles: []Profile{{
			Name:   "V2 Safe Account",
			CorpID: "corp_v2_safe",
			UserID: "user_v2_safe",
		}},
	}

	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	loaded, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if loaded.Version != profilesVersion {
		t.Fatalf("ordinary profiles version = %d, want v2 compatibility version %d", loaded.Version, profilesVersion)
	}
}

func TestInvalidReservedOrgCurrentSelectorNormalizesBackToV2(t *testing.T) {
	configDir := t.TempDir()
	corpID := "corp_reserved_org_current_guard"
	cfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: profileSelector(corpID, "exact-user"),
		OrgCurrentProfiles: map[string]string{
			corpID: unresolvedProfileSelector(corpID),
		},
		Profiles: []Profile{
			{Name: "Reserved Organization", CorpID: corpID, CorpName: "Reserved Organization"},
			{Name: "Exact Account", CorpID: corpID, CorpName: "Reserved Organization", UserID: "exact-user"},
		},
	}

	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	raw, err := os.ReadFile(ProfilesPath(configDir))
	if err != nil {
		t.Fatalf("ReadFile(profiles.json) error = %v", err)
	}
	var persisted struct {
		Version            int               `json:"version"`
		OrgCurrentProfiles map[string]string `json:"orgCurrentProfiles"`
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("json.Unmarshal(profiles.json) error = %v", err)
	}
	if persisted.Version != profilesVersion {
		t.Fatalf("persisted profiles version = %d, want %d", persisted.Version, profilesVersion)
	}
	if _, exists := persisted.OrgCurrentProfiles[corpID]; exists {
		t.Fatalf("invalid organization-current selector survived normalization: %#v", persisted.OrgCurrentProfiles)
	}
}

func TestEnsureMigrationPersistsDowngradeGuardForRawV2ReservedSelector(t *testing.T) {
	const wantDowngradeGuardVersion = 3
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	corpID := "corp_raw_v2_reserved_guard"
	cfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: unresolvedProfileSelector(corpID),
		Profiles: []Profile{
			{Name: "Raw V2 Organization", CorpID: corpID, CorpName: "Raw V2 Organization"},
			{Name: "Exact Account", CorpID: corpID, CorpName: "Raw V2 Organization", UserID: "exact-user"},
		},
	}
	writeRawProfilesFixture(t, configDir, cfg)

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	if got := readRawProfilesVersion(t, configDir); got != wantDowngradeGuardVersion {
		t.Fatalf("persisted profiles version = %d, want %d", got, wantDowngradeGuardVersion)
	}
}

func TestV1ReservedSelectorMigrationStillRunsBeforeDowngradeGuardPromotion(t *testing.T) {
	const wantDowngradeGuardVersion = 3
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	corpID := "corp_raw_v1_reserved_guard"
	cfg := &ProfilesConfig{
		Version:        1,
		CurrentProfile: unresolvedProfileSelector(corpID),
		Profiles: []Profile{
			{Name: "Raw V1 Organization", CorpID: corpID, CorpName: "Raw V1 Organization"},
			{Name: "Exact Account", CorpID: corpID, CorpName: "Raw V1 Organization", UserID: "exact-user"},
		},
	}
	writeRawProfilesFixture(t, configDir, cfg)

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	if got := readRawProfilesVersion(t, configDir); got != wantDowngradeGuardVersion {
		t.Fatalf("persisted profiles version = %d, want %d", got, wantDowngradeGuardVersion)
	}
}

func TestV1MigrationStillPromotesOnlyToIdentitySchema(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	token := testToken("legacy-v1-token", "corp_v1_schema", "V1 Schema Organization")
	token.UserID = "legacy-v1-user"
	if err := SaveTokenDataKeychainForCorpID(token.CorpID, token); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        1,
		CurrentProfile: profileSelector(token.CorpID, token.UserID),
		Profiles: []Profile{{
			Name:     token.CorpName,
			CorpID:   token.CorpID,
			CorpName: token.CorpName,
			UserID:   token.UserID,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles(v1 fixture) error = %v", err)
	}

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	loaded, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if loaded.Version != profilesVersion {
		t.Fatalf("migrated profiles version = %d, want identity schema %d", loaded.Version, profilesVersion)
	}
	if !TokenDataExistsKeychainForIdentity(token.CorpID, token.UserID) {
		t.Fatal("v1 migration did not create the exact identity token slot")
	}
}

func TestExplicitReauthUpgradingSoleBlankProfilePublishesAllTokenMirrors(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	legacy := testToken("legacy-blank-access", "corp_explicit_reauth", "Explicit Reauth Organization")
	legacy.UserID = ""
	legacy.UserName = ""
	if err := SaveTokenData(configDir, legacy); err != nil {
		t.Fatalf("SaveTokenData(legacy blank) error = %v", err)
	}

	SetRuntimeProfile(legacy.CorpID)
	t.Cleanup(func() { SetRuntimeProfile("") })
	reauthorized := *legacy
	reauthorized.AccessToken = "reauthorized-exact-access"
	reauthorized.RefreshToken = "reauthorized-exact-refresh"
	reauthorized.UserID = "resolved-external-worker"
	reauthorized.UserName = "Resolved External Worker"
	if err := preflightTokenWritePersistence(configDir, &reauthorized); err != nil {
		t.Fatalf("preflightTokenWritePersistence(explicit reauth) error = %v", err)
	}
	if err := SaveTokenData(configDir, &reauthorized); err != nil {
		t.Fatalf("SaveTokenData(explicit reauth) error = %v", err)
	}

	assertIdentityTokenAccessForTest(t, reauthorized.CorpID, reauthorized.UserID, reauthorized.AccessToken)
	assertOrganizationTokenAccessForTest(t, reauthorized.CorpID, reauthorized.AccessToken, reauthorized.UserID)
	global, err := LoadTokenDataKeychain()
	if err != nil {
		t.Fatalf("LoadTokenDataKeychain() error = %v", err)
	}
	if global.AccessToken != reauthorized.AccessToken || global.UserID != reauthorized.UserID {
		t.Fatalf("global token after explicit reauth = %#v, want reauthorized exact token", global)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].UserID != reauthorized.UserID {
		t.Fatalf("profiles after explicit reauth = %#v, want one resolved identity", cfg.Profiles)
	}
}

func TestExplicitReauthLegacyUpgradeRejectsUnreadableOrganizationMirror(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	legacy := testToken("legacy-corrupt-org-access", "corp_explicit_reauth_corrupt", "Corrupt Reauth Organization")
	legacy.UserID = ""
	legacy.UserName = ""
	if err := SaveTokenData(configDir, legacy); err != nil {
		t.Fatalf("SaveTokenData(legacy blank) error = %v", err)
	}
	if err := keychain.Set(keychain.Service, TokenAccountForCorpID(legacy.CorpID), "{unreadable"); err != nil {
		t.Fatalf("write unreadable organization mirror: %v", err)
	}

	SetRuntimeProfile(legacy.CorpID)
	t.Cleanup(func() { SetRuntimeProfile("") })
	reauthorized := *legacy
	reauthorized.AccessToken = "must-not-persist"
	reauthorized.RefreshToken = "must-not-persist-refresh"
	reauthorized.UserID = "resolved-external-worker"
	reauthorized.UserName = "Resolved External Worker"
	err := preflightTokenWritePersistence(configDir, &reauthorized)
	if err == nil || !strings.Contains(err.Error(), "profile token slot") ||
		!strings.Contains(err.Error(), "parse token data") {
		t.Fatalf("preflightTokenWritePersistence() error = %v, want unreadable organization mirror", err)
	}
	if TokenDataExistsKeychainForIdentity(reauthorized.CorpID, reauthorized.UserID) {
		t.Fatal("rejected explicit reauth created an identity token")
	}
	global, loadErr := LoadTokenDataKeychain()
	if loadErr != nil {
		t.Fatalf("LoadTokenDataKeychain() error = %v", loadErr)
	}
	if global.AccessToken != legacy.AccessToken || global.UserID != "" {
		t.Fatalf("rejected explicit reauth changed global token: %#v", global)
	}
}

func TestV3DowngradesAfterBlankProfileIdentityCompletion(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	corpID := "corp_v3_identity_completion"
	resolved := testToken("resolved-org-access", corpID, "V3 Completion Organization")
	resolved.UserID = "resolved-user"
	resolved.UserName = "Resolved User"
	if err := SaveTokenDataKeychainForCorpID(corpID, resolved); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	writeRawProfilesFixture(t, configDir, &ProfilesConfig{
		Version:        profilesUnresolvedSelectorVersion,
		CurrentProfile: unresolvedProfileSelector(corpID),
		Profiles: []Profile{{
			Name:     "Historical External Worker",
			CorpID:   corpID,
			CorpName: resolved.CorpName,
		}},
	})

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.Version != profilesVersion {
		t.Fatalf("profiles version after identity completion = %d, want %d", cfg.Version, profilesVersion)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].UserID != resolved.UserID {
		t.Fatalf("profiles after identity completion = %#v", cfg.Profiles)
	}
	if strings.Contains(string(readRawProfilesForTest(t, configDir)), unresolvedProfileSelectorPrefix) {
		t.Fatal("identity completion left reserved selector grammar on disk")
	}
}

func TestV3DowngradesAfterUnresolvedProfileDeletion(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	fixture := seedBlankProfileSelectorFixture(t, "Deletion Organization", "Deletion Organization", true)
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Version = profilesUnresolvedSelectorVersion
	if err := SaveProfiles(fixture.configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles(v3 fixture) error = %v", err)
	}
	if err := DeleteTokenDataForProfile(fixture.configDir, fixture.blankSelector); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(blank) error = %v", err)
	}

	cfg, err = LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles(after delete) error = %v", err)
	}
	if cfg.Version != profilesVersion {
		t.Fatalf("profiles version after blank delete = %d, want %d", cfg.Version, profilesVersion)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].UserID != fixture.exactUserID {
		t.Fatalf("profiles after blank delete = %#v, want exact identity only", cfg.Profiles)
	}
}

func TestV3DowngradesWhenExactConflictDeletionLeavesSoleBlankProfile(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	fixture := seedBlankProfileSelectorFixture(
		t,
		"Conflict Organization",
		"Conflict Organization",
		true,
	)
	cfg, err := LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Version = profilesUnresolvedSelectorVersion
	cfg.CurrentProfile = fixture.blankSelector
	cfg.PreviousProfile = fixture.exactSelector
	if err := SaveProfiles(fixture.configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles(v3 conflict fixture) error = %v", err)
	}
	if got := readRawProfilesVersion(t, fixture.configDir); got != profilesUnresolvedSelectorVersion {
		t.Fatalf("fixture profiles version = %d, want %d", got, profilesUnresolvedSelectorVersion)
	}

	if err := DeleteTokenDataForProfile(fixture.configDir, fixture.exactSelector); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(exact conflict) error = %v", err)
	}

	cfg, err = LoadProfiles(fixture.configDir)
	if err != nil {
		t.Fatalf("LoadProfiles(after exact delete) error = %v", err)
	}
	if cfg.Version != profilesVersion {
		t.Fatalf("profiles version after exact conflict delete = %d, want %d", cfg.Version, profilesVersion)
	}
	if cfg.CurrentProfile != fixture.corpID {
		t.Fatalf("current profile after exact conflict delete = %q, want corpId %q", cfg.CurrentProfile, fixture.corpID)
	}
	if len(cfg.Profiles) != 1 ||
		cfg.Profiles[0].CorpID != fixture.corpID ||
		cfg.Profiles[0].UserID != "" {
		t.Fatalf("profiles after exact conflict delete = %#v, want sole unresolved profile", cfg.Profiles)
	}
	if strings.Contains(string(readRawProfilesForTest(t, fixture.configDir)), unresolvedProfileSelectorPrefix) {
		t.Fatal("exact conflict deletion left reserved selector grammar on disk")
	}
	loaded, err := LoadTokenDataForProfile(fixture.configDir, "")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(current blank) error = %v", err)
	}
	if loaded.AccessToken != fixture.blankToken.AccessToken || loaded.UserID != "" {
		t.Fatalf("current token after exact conflict delete = %#v, want unresolved token %#v", loaded, fixture.blankToken)
	}
}

func TestV3DowngradesWhenReservedSelectorCanonicalizesToSafeLocalName(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	corpID := "corp_v3_safe_selector"
	const safeName = "Historical External Worker"
	writeRawProfilesFixture(t, configDir, &ProfilesConfig{
		Version:        profilesUnresolvedSelectorVersion,
		CurrentProfile: unresolvedProfileSelector(corpID),
		Profiles: []Profile{
			{Name: safeName, CorpID: corpID, CorpName: "Safe Selector Organization"},
			{Name: "Exact Account", CorpID: corpID, CorpName: "Safe Selector Organization", UserID: "exact-user"},
		},
	})

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.Version != profilesVersion || cfg.CurrentProfile != safeName {
		t.Fatalf("normalized profiles = version %d current %q, want v%d current %q", cfg.Version, cfg.CurrentProfile, profilesVersion, safeName)
	}
	if strings.Contains(string(readRawProfilesForTest(t, configDir)), unresolvedProfileSelectorPrefix) {
		t.Fatal("safe selector normalization left reserved grammar on disk")
	}
}

type unreadableExactProfileSelectionFixture struct {
	configDir          string
	corpID             string
	unreadableSelector string
	unreadableErr      error
	unreadableToken    *TokenData
	state              *ProfilesConfig
	before             *ProfilesConfig
	organizationToken  *TokenData
	globalToken        *TokenData
	beforeOrganization *TokenData
	beforeGlobal       *TokenData
}

func installUnreadableExactProfileSelectionFixture(t *testing.T, unreadableIsPrevious bool) *unreadableExactProfileSelectionFixture {
	t.Helper()

	originalEnsureMigration := profilesEnsureMigration
	originalLoad := profilesLoad
	originalSave := profilesSave
	originalLoadIdentity := profilesLoadIdentity
	originalLoadOrganization := profilesLoadCorp
	originalSaveOrganization := profilesSaveCorp
	originalDeleteOrganization := profilesDeleteCorp
	originalLoadGlobal := profilesLoadLegacy
	originalSaveGlobal := profilesSaveLegacy
	originalDeleteGlobal := profilesDeleteLegacy
	t.Cleanup(func() {
		profilesEnsureMigration = originalEnsureMigration
		profilesLoad = originalLoad
		profilesSave = originalSave
		profilesLoadIdentity = originalLoadIdentity
		profilesLoadCorp = originalLoadOrganization
		profilesSaveCorp = originalSaveOrganization
		profilesDeleteCorp = originalDeleteOrganization
		profilesLoadLegacy = originalLoadGlobal
		profilesSaveLegacy = originalSaveGlobal
		profilesDeleteLegacy = originalDeleteGlobal
	})

	const (
		corpID        = "corp_profile_switch_guard"
		unreadableUID = "unreadable-exact-user"
		currentUID    = "readable-current-user"
	)
	unreadableSelector := profileSelector(corpID, unreadableUID)
	currentSelector := profileSelector(corpID, currentUID)
	previousSelector := "Legacy Contractor"
	if unreadableIsPrevious {
		previousSelector = unreadableSelector
	}
	state := &ProfilesConfig{
		Version:         profilesVersion,
		CurrentProfile:  currentSelector,
		PreviousProfile: previousSelector,
		OrgCurrentProfiles: map[string]string{
			corpID: currentSelector,
		},
		Profiles: []Profile{
			{Name: "Legacy Contractor", CorpID: corpID},
			{Name: "Unreadable Account", CorpID: corpID, UserID: unreadableUID},
			{Name: "Readable Account", CorpID: corpID, UserID: currentUID},
		},
	}
	organizationToken := &TokenData{AccessToken: "legacy-organization-token", CorpID: corpID}
	globalToken := &TokenData{AccessToken: "readable-global-token", CorpID: corpID, UserID: currentUID}
	fixture := &unreadableExactProfileSelectionFixture{
		configDir:          t.TempDir(),
		corpID:             corpID,
		unreadableSelector: unreadableSelector,
		unreadableErr:      errors.New("decrypt exact identity token: key mismatch"),
		state:              cloneProfilesConfig(state),
		before:             cloneProfilesConfig(state),
		organizationToken:  cloneTokenDataForTest(organizationToken),
		globalToken:        cloneTokenDataForTest(globalToken),
		beforeOrganization: cloneTokenDataForTest(organizationToken),
		beforeGlobal:       cloneTokenDataForTest(globalToken),
	}

	profilesEnsureMigration = func(string) error { return nil }
	profilesLoad = func(string) (*ProfilesConfig, error) {
		return cloneProfilesConfig(fixture.state), nil
	}
	profilesSave = func(_ string, cfg *ProfilesConfig) error {
		fixture.state = cloneProfilesConfig(cfg)
		return nil
	}
	profilesLoadIdentity = func(_ string, userID string) (*TokenData, error) {
		switch userID {
		case unreadableUID:
			if fixture.unreadableToken != nil {
				return cloneTokenDataForTest(fixture.unreadableToken), nil
			}
			return nil, fixture.unreadableErr
		case currentUID:
			return cloneTokenDataForTest(fixture.globalToken), nil
		default:
			return nil, ErrTokenDataNotFound
		}
	}
	profilesLoadCorp = func(string) (*TokenData, error) {
		return cloneTokenDataForTest(fixture.organizationToken), nil
	}
	profilesSaveCorp = func(_ string, data *TokenData) error {
		fixture.organizationToken = cloneTokenDataForTest(data)
		return nil
	}
	profilesDeleteCorp = func(string) error {
		fixture.organizationToken = nil
		return nil
	}
	profilesLoadLegacy = func() (*TokenData, error) {
		if fixture.globalToken == nil {
			return nil, ErrTokenDataNotFound
		}
		return cloneTokenDataForTest(fixture.globalToken), nil
	}
	profilesSaveLegacy = func(data *TokenData) error {
		fixture.globalToken = cloneTokenDataForTest(data)
		return nil
	}
	profilesDeleteLegacy = func() error {
		fixture.globalToken = nil
		return nil
	}

	return fixture
}

func (f *unreadableExactProfileSelectionFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	if !reflect.DeepEqual(f.state, f.before) {
		t.Fatalf("profiles changed after rejected switch:\n got: %#v\nwant: %#v", f.state, f.before)
	}
	if !reflect.DeepEqual(f.organizationToken, f.beforeOrganization) {
		t.Fatalf("organization mirror changed after rejected switch: got %#v want %#v", f.organizationToken, f.beforeOrganization)
	}
	if !reflect.DeepEqual(f.globalToken, f.beforeGlobal) {
		t.Fatalf("global mirror changed after rejected switch: got %#v want %#v", f.globalToken, f.beforeGlobal)
	}
}

func cloneTokenDataForTest(data *TokenData) *TokenData {
	if data == nil {
		return nil
	}
	clone := *data
	return &clone
}

func writeRawProfilesFixture(t *testing.T, configDir string, cfg *ProfilesConfig) {
	t.Helper()
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(profiles fixture) error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(profiles fixture) error = %v", err)
	}
}

func readRawProfilesVersion(t *testing.T, configDir string) int {
	t.Helper()
	raw, err := os.ReadFile(ProfilesPath(configDir))
	if err != nil {
		t.Fatalf("ReadFile(profiles.json) error = %v", err)
	}
	var persisted struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("json.Unmarshal(profiles.json) error = %v", err)
	}
	return persisted.Version
}

func readRawProfilesForTest(t *testing.T, configDir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(ProfilesPath(configDir))
	if err != nil {
		t.Fatalf("ReadFile(profiles.json) error = %v", err)
	}
	return raw
}
