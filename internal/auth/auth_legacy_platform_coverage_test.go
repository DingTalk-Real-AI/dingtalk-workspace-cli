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
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// The native coverage jobs intentionally execute only TestCrossPlatformCoverage
// entry points. Keep the compatibility assertions below as independently named
// regression tests for the stable make target, and exercise the same functions
// here as isolated subtests so their cleanup hooks run between cases.
func TestCrossPlatformCoverageAuthLegacyCompatibilityRegressions(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{"prepare unique global owner", TestPrepareLoginPersistenceRepairsOnlySafeGlobalOwner},
		{"token load isolation matrix", TestTokenLoadIsolationMatrix},
		{"reject future profiles before remote work", TestPersistingLoginFlowsRejectFutureProfilesBeforeRemoteWork},
		{"fresh UID-less isolation", TestFreshUIDLessExactLoginCannotOverwriteExistingUnresolvedProfile},
		{"reject mismatched exact switch", TestSetCurrentProfileRejectsMismatchedExactIdentitySlotBesideBlankProfile},
		{"reject unreadable previous identity", TestUsePreviousProfileRejectsUnreadableExactIdentityBesideBlankProfile},
		{"reauthorization guidance without profile", TestLegacyRefreshReauthorizationGuidanceWithoutProfileStillExplainsLogin},
		{"repair v3 unresolved profile", TestPrepareLoginPersistenceV3UnresolvedProfileRepair},
		{"device ignores unrelated unreadable profile", TestDeviceLoginIgnoresUnreadableUnrelatedProfile},
		{"reject unreadable global before login", TestPrepareLoginPersistenceUnreadableGlobalFailsClosedBeforeRemote},
		{"device validates resolved target", TestDeviceFlowChecksResolvedTargetBeforeSave},
		{"accept recoverable credential material", TestPrepareLoginPersistenceRequiresCredentialMaterialButNotValidity},
		{"auth code validates resolved target", TestExchangeAuthCodeChecksResolvedTargetBeforeSave},
		{"standalone exchange prepares and marks fresh", TestExchangeCodeForTokenPreparesBeforeRemoteAndMarksFresh},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestCrossPlatformCoverageHalfMigratedGlobalRepairRemainingEdges(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())

	t.Run("nil global token", func(t *testing.T) {
		isolateHalfMigratedRepairHooks(t)
		profilesLoadLegacy = func() (*TokenData, error) { return nil, nil }
		cfg := &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{CorpID: "corp_nil_global"}}}
		if err := repairHalfMigratedGlobalTokenLocked(cfg); err != nil {
			t.Fatalf("repairHalfMigratedGlobalTokenLocked() error = %v", err)
		}
	})

	t.Run("global token without organization", func(t *testing.T) {
		isolateHalfMigratedRepairHooks(t)
		profilesLoadLegacy = func() (*TokenData, error) {
			return &TokenData{AccessToken: "legacy"}, nil
		}
		cfg := &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{CorpID: "corp_other"}}}
		if err := repairHalfMigratedGlobalTokenLocked(cfg); err != nil {
			t.Fatalf("repairHalfMigratedGlobalTokenLocked() error = %v", err)
		}
	})

	t.Run("orphan global organization", func(t *testing.T) {
		isolateHalfMigratedRepairHooks(t)
		profilesLoadLegacy = func() (*TokenData, error) {
			return &TokenData{AccessToken: "legacy", CorpID: "corp_orphan"}, nil
		}
		cfg := &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{CorpID: "corp_other"}}}
		if err := repairHalfMigratedGlobalTokenLocked(cfg); err != nil {
			t.Fatalf("repairHalfMigratedGlobalTokenLocked() error = %v", err)
		}
	})

	t.Run("identity repair write failure", func(t *testing.T) {
		isolateHalfMigratedRepairHooks(t)
		failure := errors.New("identity repair write failure")
		const corpID, userID = "corp_identity_repair", "user_identity_repair"
		profilesLoadLegacy = func() (*TokenData, error) {
			return &TokenData{AccessToken: "legacy", CorpID: corpID}, nil
		}
		profilesLoadCorp = func(string) (*TokenData, error) {
			return &TokenData{AccessToken: "organization", CorpID: corpID}, nil
		}
		profilesSaveIdentity = func(string, string, *TokenData) error { return failure }
		cfg := &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{CorpID: corpID, UserID: userID}}}
		if err := repairHalfMigratedGlobalTokenLocked(cfg); !errors.Is(err, failure) {
			t.Fatalf("repairHalfMigratedGlobalTokenLocked() error = %v, want %v", err, failure)
		}
	})

	t.Run("organization repair write failure", func(t *testing.T) {
		isolateHalfMigratedRepairHooks(t)
		failure := errors.New("organization repair write failure")
		const corpID = "corp_organization_repair"
		profilesLoadLegacy = func() (*TokenData, error) {
			return &TokenData{AccessToken: "legacy", CorpID: corpID}, nil
		}
		profilesSaveCorp = func(string, *TokenData) error { return failure }
		cfg := &ProfilesConfig{Version: profilesVersion, Profiles: []Profile{{CorpID: corpID}}}
		if err := repairHalfMigratedGlobalTokenLocked(cfg); !errors.Is(err, failure) {
			t.Fatalf("repairHalfMigratedGlobalTokenLocked() error = %v, want %v", err, failure)
		}
	})

	t.Run("nil profile is not canonical", func(t *testing.T) {
		if loginProfileHasUsableCanonicalToken(nil, nil, nil) {
			t.Fatal("nil profile was treated as a usable canonical token")
		}
	})

	t.Run("global write preflight reports unreadable slot", func(t *testing.T) {
		oldGet := authKeychainGet
		failure := errors.New("global ciphertext is unreadable")
		authKeychainGet = func(_, account string) (string, error) {
			if account == keychain.AccountToken {
				return "", failure
			}
			return "", nil
		}
		t.Cleanup(func() { authKeychainGet = oldGet })

		err := preflightTokenWritePersistence(t.TempDir(), &TokenData{AccessToken: "fresh"})
		if !errors.Is(err, failure) {
			t.Fatalf("preflightTokenWritePersistence() error = %v, want %v", err, failure)
		}
	})
}

func TestCrossPlatformCoverageLegacyProfileGuardRemainingEdges(t *testing.T) {
	t.Run("empty v3 registry normalizes and persists", func(t *testing.T) {
		oldLoad := profilesLoad
		oldSave := profilesSave
		cfg := &ProfilesConfig{Version: profilesUnresolvedSelectorVersion}
		profilesLoad = func(string) (*ProfilesConfig, error) { return cfg, nil }
		saves := 0
		profilesSave = func(string, *ProfilesConfig) error {
			saves++
			return nil
		}
		t.Cleanup(func() {
			profilesLoad = oldLoad
			profilesSave = oldSave
		})

		if err := ensureProfilesMigrationLocked(t.TempDir()); err != nil {
			t.Fatalf("ensureProfilesMigrationLocked() error = %v", err)
		}
		if cfg.Version != profilesVersion || saves != 1 {
			t.Fatalf("normalized registry = version %d saves %d", cfg.Version, saves)
		}
	})

	if normalizeProfilesVersionForSelectors(nil) {
		t.Fatal("nil profile registry reported a version change")
	}
	future := &ProfilesConfig{Version: profilesMaxVersion + 1}
	if normalizeProfilesVersionForSelectors(future) {
		t.Fatal("future profile registry reported a version change")
	}
	if profilesConfigContainsUnresolvedSelector(nil) {
		t.Fatal("nil profile registry contained an unresolved selector")
	}
	reserved := unresolvedProfileSelector("corp_org_current_guard")
	if !profilesConfigContainsUnresolvedSelector(&ProfilesConfig{
		OrgCurrentProfiles: map[string]string{"corp_org_current_guard": reserved},
	}) {
		t.Fatal("reserved organization-current selector was not detected")
	}
	if selectorConflictsWithOrganizationGrammar(nil, "corp") {
		t.Fatal("nil profile registry reported an organization-selector conflict")
	}
	if err := validateIdentityOnlyProfileToken(Profile{}); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("validateIdentityOnlyProfileToken(blank) error = %v", err)
	}

	oldLoadIdentity := profilesLoadIdentity
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, nil }
	t.Cleanup(func() { profilesLoadIdentity = oldLoadIdentity })
	if err := validateIdentityOnlyProfileToken(Profile{CorpID: "corp_nil_identity", UserID: "user_nil_identity"}); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("validateIdentityOnlyProfileToken(nil token) error = %v", err)
	}
}

func TestCrossPlatformCoverageTokenPersistenceRemainingEdges(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())

	plan := planTokenPersistenceWrites(&ProfilesConfig{}, nil, "")
	if !plan.WriteGlobal || plan.CorpID != "" {
		t.Fatalf("nil-token write plan = %#v", plan)
	}
	if err := SaveLoginTokenData(t.TempDir(), nil); err == nil {
		t.Fatal("SaveLoginTokenData(nil) succeeded")
	}

	futureDir := t.TempDir()
	writeFutureProfilesForLoginPreflight(t, futureDir)
	err := SaveLoginTokenData(futureDir, &TokenData{AccessToken: "fresh"})
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("SaveLoginTokenData(future schema) error = %v", err)
	}

	t.Run("nil identity token", func(t *testing.T) {
		isolateTokenProfileLoadHooks(t)
		tokenLoadKeychainIdentity = func(string, string) (*TokenData, error) { return nil, nil }
		_, err := tokenLoadProfileIdentity(Profile{CorpID: "corp_nil_identity", UserID: "user_nil_identity"})
		if !errors.Is(err, ErrTokenDataNotFound) {
			t.Fatalf("tokenLoadProfileIdentity() error = %v", err)
		}
	})

	t.Run("nil organization fallback", func(t *testing.T) {
		isolateTokenProfileLoadHooks(t)
		tokenLoadKeychainForCorpID = func(string) (*TokenData, error) { return nil, nil }
		_, err := tokenLoadProfileIdentity(Profile{CorpID: "corp_nil_org", UserID: "user_nil_org"})
		if !errors.Is(err, ErrTokenDataNotFound) {
			t.Fatalf("tokenLoadProfileIdentity() error = %v", err)
		}
	})

	t.Run("organization fallback belongs to another organization", func(t *testing.T) {
		isolateTokenProfileLoadHooks(t)
		tokenLoadKeychainForCorpID = func(string) (*TokenData, error) {
			return &TokenData{AccessToken: "wrong", CorpID: "corp_other", UserID: "user_wrong_org"}, nil
		}
		_, err := tokenLoadProfileIdentity(Profile{CorpID: "corp_expected", UserID: "user_wrong_org"})
		if err == nil || !strings.Contains(err.Error(), "contains token for corpId") {
			t.Fatalf("tokenLoadProfileIdentity() error = %v", err)
		}
	})

	t.Run("identity repair write failure", func(t *testing.T) {
		isolateTokenProfileLoadHooks(t)
		failure := errors.New("identity repair write failure")
		const corpID, userID = "corp_identity_save", "user_identity_save"
		tokenLoadKeychainForCorpID = func(string) (*TokenData, error) {
			return &TokenData{AccessToken: "organization", CorpID: corpID, UserID: userID}, nil
		}
		tokenSaveKeychainForIdentity = func(string, string, *TokenData) error { return failure }
		_, err := tokenLoadProfileIdentity(Profile{CorpID: corpID, UserID: userID})
		if !errors.Is(err, failure) {
			t.Fatalf("tokenLoadProfileIdentity() error = %v, want %v", err, failure)
		}
	})
}

func isolateHalfMigratedRepairHooks(t *testing.T) {
	t.Helper()
	oldLoadLegacy := profilesLoadLegacy
	oldLoadCorp := profilesLoadCorp
	oldLoadIdentity := profilesLoadIdentity
	oldSaveCorp := profilesSaveCorp
	oldSaveIdentity := profilesSaveIdentity
	t.Cleanup(func() {
		profilesLoadLegacy = oldLoadLegacy
		profilesLoadCorp = oldLoadCorp
		profilesLoadIdentity = oldLoadIdentity
		profilesSaveCorp = oldSaveCorp
		profilesSaveIdentity = oldSaveIdentity
	})
	profilesLoadLegacy = func() (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesLoadCorp = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesLoadIdentity = func(string, string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	profilesSaveCorp = func(string, *TokenData) error { return nil }
	profilesSaveIdentity = func(string, string, *TokenData) error { return nil }
}

func isolateTokenProfileLoadHooks(t *testing.T) {
	t.Helper()
	oldLoadIdentity := tokenLoadKeychainIdentity
	oldLoadCorp := tokenLoadKeychainForCorpID
	oldSaveIdentity := tokenSaveKeychainForIdentity
	t.Cleanup(func() {
		tokenLoadKeychainIdentity = oldLoadIdentity
		tokenLoadKeychainForCorpID = oldLoadCorp
		tokenSaveKeychainForIdentity = oldSaveIdentity
	})
	tokenLoadKeychainIdentity = func(string, string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	tokenLoadKeychainForCorpID = func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound }
	tokenSaveKeychainForIdentity = func(string, string, *TokenData) error { return nil }
}
