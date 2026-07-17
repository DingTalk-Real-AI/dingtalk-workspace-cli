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
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// cleanupKeychain isolates keychain state to a per-test temporary directory
// so that concurrent test packages (notably internal/app) don't read tokens
// written by these tests, and removes test data on completion.
func cleanupKeychain(t *testing.T) string {
	t.Helper()
	SetRuntimeProfile("")
	storageDir := t.TempDir()
	t.Setenv(keychain.StorageDirEnv, storageDir)
	if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
		t.Fatalf("remove auth token entries before test: %v", err)
	}
	t.Cleanup(func() {
		SetRuntimeProfile("")
		if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
			t.Errorf("remove auth token entries after test: %v", err)
		}
	})
	return storageDir
}

func TestCleanupKeychainRemovesAllAuthTokenSlots(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	_ = keychain.RemoveAuthTokenEntries(keychain.Service)
	t.Cleanup(func() { _ = keychain.RemoveAuthTokenEntries(keychain.Service) })

	var storageDir string
	t.Run("seed isolated token slots", func(t *testing.T) {
		storageDir = cleanupKeychain(t)
		for _, account := range []string{
			keychain.AccountToken,
			TokenAccountForCorpID("corp_same"),
			TokenAccountForIdentity("corp_same", "user_2"),
		} {
			if err := keychain.Set(keychain.Service, account, "test-token"); err != nil {
				t.Fatalf("keychain.Set(%q) error = %v", account, err)
			}
		}
	})

	t.Setenv(keychain.StorageDirEnv, storageDir)
	for _, account := range []string{
		keychain.AccountToken,
		TokenAccountForCorpID("corp_same"),
		TokenAccountForIdentity("corp_same", "user_2"),
	} {
		if value, err := keychain.Get(keychain.Service, account); err != nil || value != "" {
			t.Fatalf("token slot %q after cleanup = %q, %v; want empty", account, value, err)
		}
	}
}

func TestTokenSaveLoadAndDelete(t *testing.T) {
	cleanupKeychain(t)

	configDir := t.TempDir()
	now := time.Now().UTC()
	original := &TokenData{
		AccessToken:    "at_test_123",
		RefreshToken:   "rt_test_456",
		PersistentCode: "pc_test_789",
		ExpiresAt:      now.Add(2 * time.Hour),
		RefreshExpAt:   now.Add(30 * 24 * time.Hour),
		CorpID:         "ding123",
		UserID:         "user001",
		UserName:       "张三",
		CorpName:       "测试科技",
	}

	// Save to keychain
	if err := SaveTokenData(configDir, original); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	// Verify data exists in keychain
	if !TokenDataExistsKeychain() {
		t.Fatal("TokenDataExistsKeychain() should be true after save")
	}

	// Load and verify
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != original.AccessToken || loaded.PersistentCode != original.PersistentCode {
		t.Fatalf("loaded token = %#v, want access/persistent code preserved", loaded)
	}
	if loaded.UserID != original.UserID {
		t.Fatalf("loaded user id = %q, want %q", loaded.UserID, original.UserID)
	}
	if loaded.CorpID != original.CorpID {
		t.Fatalf("loaded corp_id = %q, want %q", loaded.CorpID, original.CorpID)
	}

	// Delete and verify
	if err := DeleteTokenData(configDir); err != nil {
		t.Fatalf("DeleteTokenData() error = %v", err)
	}
	if TokenDataExistsKeychain() {
		t.Fatal("TokenDataExistsKeychain() should be false after delete")
	}
	if _, err := LoadTokenData(configDir); err == nil {
		t.Fatal("LoadTokenData() error = nil after delete, want failure")
	}
}

func TestTokenOverwrite(t *testing.T) {
	cleanupKeychain(t)

	configDir := t.TempDir()

	// Save first version
	data1 := &TokenData{
		AccessToken:  "at_v1",
		RefreshToken: "rt_v1",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "corp_v1",
	}
	if err := SaveTokenData(configDir, data1); err != nil {
		t.Fatalf("SaveTokenData(v1) error = %v", err)
	}

	// Save second version (overwrite)
	data2 := &TokenData{
		AccessToken:  "at_v2",
		RefreshToken: "rt_v2",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		RefreshExpAt: time.Now().Add(48 * time.Hour),
		CorpID:       "corp_v2",
	}
	if err := SaveTokenData(configDir, data2); err != nil {
		t.Fatalf("SaveTokenData(v2) error = %v", err)
	}

	// Load should return v2
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != "at_v2" {
		t.Fatalf("access_token = %q, want %q", loaded.AccessToken, "at_v2")
	}
	if loaded.CorpID != "corp_v2" {
		t.Fatalf("corp_id = %q, want %q", loaded.CorpID, "corp_v2")
	}
}

func TestManualTokenRemainsLoadableWithV2Profiles(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	profileToken := testToken("profile-token", "corp_same", "同一组织")
	profileToken.UserID = "user_1"
	if err := SaveTokenData(configDir, profileToken); err != nil {
		t.Fatalf("SaveTokenData(profile) error = %v", err)
	}

	manualToken := &TokenData{
		AccessToken: "manual-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveTokenData(configDir, manualToken); err != nil {
		t.Fatalf("SaveTokenData(manual) error = %v", err)
	}

	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != manualToken.AccessToken {
		t.Fatalf("default token = %q, want manual token", loaded.AccessToken)
	}

	explicit, err := LoadTokenDataForProfile(configDir, ProfileSelector(Profile{
		CorpID: profileToken.CorpID,
		UserID: profileToken.UserID,
	}))
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile() error = %v", err)
	}
	if explicit.AccessToken != profileToken.AccessToken {
		t.Fatalf("explicit profile token = %q, want %q", explicit.AccessToken, profileToken.AccessToken)
	}

	refreshed := *profileToken
	refreshed.AccessToken = "profile-token-refreshed"
	SetRuntimeProfile(ProfileSelector(Profile{CorpID: profileToken.CorpID, UserID: profileToken.UserID}))
	if err := SaveTokenData(configDir, &refreshed); err != nil {
		t.Fatalf("SaveTokenData(explicit refresh) error = %v", err)
	}
	SetRuntimeProfile("")
	loaded, err = LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after explicit refresh error = %v", err)
	}
	if loaded.AccessToken != manualToken.AccessToken {
		t.Fatalf("default token after explicit refresh = %q, want manual token", loaded.AccessToken)
	}
	explicit, err = LoadTokenDataForProfile(
		configDir,
		ProfileSelector(Profile{CorpID: profileToken.CorpID, UserID: profileToken.UserID}),
	)
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile() after refresh error = %v", err)
	}
	if explicit.AccessToken != refreshed.AccessToken {
		t.Fatalf("explicit refreshed token = %q, want %q", explicit.AccessToken, refreshed.AccessToken)
	}

	if err := DeleteTokenDataForProfile(
		configDir,
		ProfileSelector(Profile{CorpID: profileToken.CorpID, UserID: profileToken.UserID}),
	); err != nil {
		t.Fatalf("DeleteTokenDataForProfile() error = %v", err)
	}
	loaded, err = LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after selective logout error = %v", err)
	}
	if loaded.AccessToken != manualToken.AccessToken {
		t.Fatalf("default token after selective logout = %q, want manual token", loaded.AccessToken)
	}
}

func TestManualTokenRemainsLoadableWithEmptyV2Tombstone(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	if err := SaveProfiles(configDir, &ProfilesConfig{Version: profilesVersion}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	manualToken := &TokenData{
		AccessToken: "manual-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveTokenData(configDir, manualToken); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != manualToken.AccessToken {
		t.Fatalf("loaded token = %q, want %q", loaded.AccessToken, manualToken.AccessToken)
	}
}

func TestDefaultDeleteRemovesManualTokenWithoutDeletingProfiles(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	profileToken := testToken("profile-token", "corp_same", "同一组织")
	profileToken.UserID = "user_1"
	if err := SaveTokenData(configDir, profileToken); err != nil {
		t.Fatalf("SaveTokenData(profile) error = %v", err)
	}
	manualToken := &TokenData{
		AccessToken: "manual-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveTokenData(configDir, manualToken); err != nil {
		t.Fatalf("SaveTokenData(manual) error = %v", err)
	}

	if err := DeleteTokenData(configDir); err != nil {
		t.Fatalf("DeleteTokenData() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 || cfg.CurrentProfile != "corp_same:user_1" {
		t.Fatalf("profiles after default manual delete = %#v", cfg)
	}
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after manual delete error = %v", err)
	}
	if loaded.AccessToken != profileToken.AccessToken {
		t.Fatalf("default token after manual delete = %q, want profile token", loaded.AccessToken)
	}
}

func TestMultiProfileSaveLoadAndSwitch(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	dataA := testToken("at_a", "corp_a", "A Org")
	dataB := testToken("at_b", "corp_b", "B Org")
	if err := SaveTokenData(configDir, dataA); err != nil {
		t.Fatalf("SaveTokenData(A) error = %v", err)
	}
	if err := SaveTokenData(configDir, dataB); err != nil {
		t.Fatalf("SaveTokenData(B) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.PrimaryProfile != "" ||
		cfg.CurrentProfile != "corp_b:user_corp_b" ||
		cfg.PreviousProfile != "corp_a:user_corp_a" {
		t.Fatalf("profile pointers = primary %q current %q previous %q", cfg.PrimaryProfile, cfg.CurrentProfile, cfg.PreviousProfile)
	}

	loadedB, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loadedB.AccessToken != "at_b" {
		t.Fatalf("default token = %q, want at_b", loadedB.AccessToken)
	}
	loadedA, err := LoadTokenDataForProfile(configDir, "A Org")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(A Org) error = %v", err)
	}
	if loadedA.AccessToken != "at_a" {
		t.Fatalf("profile A token = %q, want at_a", loadedA.AccessToken)
	}

	if _, err := SetCurrentProfile(configDir, "corp_a"); err != nil {
		t.Fatalf("SetCurrentProfile(A) error = %v", err)
	}
	loadedA, err = LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after switch error = %v", err)
	}
	if loadedA.AccessToken != "at_a" {
		t.Fatalf("default token after switch = %q, want at_a", loadedA.AccessToken)
	}
	if _, err := UsePreviousProfile(configDir); err != nil {
		t.Fatalf("UsePreviousProfile() error = %v", err)
	}
	loadedB, err = LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after previous error = %v", err)
	}
	if loadedB.AccessToken != "at_b" {
		t.Fatalf("default token after previous = %q, want at_b", loadedB.AccessToken)
	}
}

func TestProfileLoginInitializesTimeMetadata(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	if err := SaveTokenData(configDir, testToken("at_login", "corp_login", "登录组织")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(cfg.Profiles))
	}
	profile := cfg.Profiles[0]
	for field, value := range map[string]string{
		"lastLoginAt": profile.LastLoginAt,
		"lastUsedAt":  profile.LastUsedAt,
		"updatedAt":   profile.UpdatedAt,
	} {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Fatalf("%s = %q, want RFC3339 timestamp: %v", field, value, err)
		}
	}
}

func TestProfileLoginUpdatesExistingTimeMetadata(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	token := testToken("at_first", "corp_login", "登录组织")
	if err := SaveTokenData(configDir, token); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	const oldTime = "2000-01-02T03:04:05Z"
	cfg.Profiles[0].LastLoginAt = oldTime
	cfg.Profiles[0].LastUsedAt = oldTime
	cfg.Profiles[0].UpdatedAt = oldTime
	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	token.AccessToken = "at_second"
	if err := SaveTokenData(configDir, token); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}
	cfg, err = LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() after second login error = %v", err)
	}
	profile := cfg.Profiles[0]
	for field, value := range map[string]string{
		"lastLoginAt": profile.LastLoginAt,
		"lastUsedAt":  profile.LastUsedAt,
		"updatedAt":   profile.UpdatedAt,
	} {
		if value == oldTime {
			t.Fatalf("%s after second login was not updated", field)
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Fatalf("%s = %q, want RFC3339 timestamp: %v", field, value, err)
		}
	}
}

func TestProfileSwitchUpdatesUsageTimeWithoutChangingLoginTime(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	const oldTime = "2000-01-02T03:04:05Z"
	for i := range cfg.Profiles {
		cfg.Profiles[i].LastLoginAt = oldTime
		cfg.Profiles[i].LastUsedAt = oldTime
		cfg.Profiles[i].UpdatedAt = oldTime
	}
	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	if _, err := SetCurrentProfile(configDir, "corp_same:user_1"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}
	cfg, err = LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() after switch error = %v", err)
	}
	selected := findExactProfile(cfg, "corp_same", "user_1")
	if selected == nil {
		t.Fatal("selected profile not found")
	}
	if selected.LastLoginAt != oldTime {
		t.Fatalf("lastLoginAt after switch = %q, want unchanged %q", selected.LastLoginAt, oldTime)
	}
	for field, value := range map[string]string{
		"lastUsedAt": selected.LastUsedAt,
		"updatedAt":  selected.UpdatedAt,
	} {
		if value == oldTime {
			t.Fatalf("%s after switch was not updated", field)
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Fatalf("%s = %q, want RFC3339 timestamp: %v", field, value, err)
		}
	}
}

func TestUsePreviousProfileUpdatesUsageTimeWithoutChangingLoginTime(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}
	if _, err := SetCurrentProfile(configDir, "corp_same:user_1"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	const oldTime = "2000-01-02T03:04:05Z"
	previous := findExactProfile(cfg, "corp_same", "user_2")
	previous.LastLoginAt = oldTime
	previous.LastUsedAt = oldTime
	previous.UpdatedAt = oldTime
	if err := SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	if _, err := UsePreviousProfile(configDir); err != nil {
		t.Fatalf("UsePreviousProfile() error = %v", err)
	}
	cfg, err = LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() after switch error = %v", err)
	}
	selected := findExactProfile(cfg, "corp_same", "user_2")
	if selected.LastLoginAt != oldTime {
		t.Fatalf("lastLoginAt after previous switch = %q, want unchanged %q", selected.LastLoginAt, oldTime)
	}
	for field, value := range map[string]string{
		"lastUsedAt": selected.LastUsedAt,
		"updatedAt":  selected.UpdatedAt,
	} {
		if value == oldTime {
			t.Fatalf("%s after previous switch was not updated", field)
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Fatalf("%s = %q, want RFC3339 timestamp: %v", field, value, err)
		}
	}
}

func TestRuntimeProfileOverrideDoesNotMutateCurrent(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	if err := SaveTokenData(configDir, testToken("at_a", "corp_a", "A Org")); err != nil {
		t.Fatalf("SaveTokenData(A) error = %v", err)
	}
	if err := SaveTokenData(configDir, testToken("at_b", "corp_b", "B Org")); err != nil {
		t.Fatalf("SaveTokenData(B) error = %v", err)
	}
	if _, err := SetCurrentProfile(configDir, "corp_a"); err != nil {
		t.Fatalf("SetCurrentProfile(A) error = %v", err)
	}

	SetRuntimeProfile("corp_b")
	if err := SaveTokenData(configDir, testToken("at_b_refreshed", "corp_b", "B Org")); err != nil {
		t.Fatalf("SaveTokenData(B refresh) error = %v", err)
	}
	SetRuntimeProfile("")

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_a:user_corp_a" {
		t.Fatalf("current profile = %q, want corp_a:user_corp_a", cfg.CurrentProfile)
	}
	loadedB, err := LoadTokenDataForProfile(configDir, "corp_b")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(B) error = %v", err)
	}
	if loadedB.AccessToken != "at_b_refreshed" {
		t.Fatalf("profile B token = %q, want at_b_refreshed", loadedB.AccessToken)
	}
	loadedDefault, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loadedDefault.AccessToken != "at_a" {
		t.Fatalf("default token = %q, want at_a", loadedDefault.AccessToken)
	}
}

func TestSaveTokenDataRollsBackPartialProfilePersistence(t *testing.T) {
	for _, failurePoint := range []string{"organization", "legacy", "marker"} {
		t.Run(failurePoint, func(t *testing.T) {
			cleanupKeychain(t)
			configDir := t.TempDir()
			existing := testToken("at_existing", "corp_existing", "现有组织")
			if err := SaveTokenData(configDir, existing); err != nil {
				t.Fatalf("SaveTokenData(existing) error = %v", err)
			}
			before, err := LoadProfiles(configDir)
			if err != nil {
				t.Fatalf("LoadProfiles() error = %v", err)
			}

			incoming := testToken("at_incoming", "corp_incoming", "新组织")
			failure := errors.New("persist " + failurePoint)
			originalSaveOrg := tokenSaveKeychainForCorpID
			originalSaveLegacy := tokenSaveKeychain
			originalWriteMarker := tokenWriteMarker
			switch failurePoint {
			case "organization":
				tokenSaveKeychainForCorpID = func(corpID string, data *TokenData) error {
					if corpID == incoming.CorpID {
						return failure
					}
					return originalSaveOrg(corpID, data)
				}
			case "legacy":
				tokenSaveKeychain = func(data *TokenData) error {
					if data != nil && data.CorpID == incoming.CorpID {
						return failure
					}
					return originalSaveLegacy(data)
				}
			case "marker":
				tokenWriteMarker = func(string) error { return failure }
			}

			err = SaveTokenData(configDir, incoming)
			tokenSaveKeychainForCorpID = originalSaveOrg
			tokenSaveKeychain = originalSaveLegacy
			tokenWriteMarker = originalWriteMarker
			if !errors.Is(err, failure) {
				t.Fatalf("SaveTokenData() error = %v, want %v", err, failure)
			}

			after, err := LoadProfiles(configDir)
			if err != nil {
				t.Fatalf("LoadProfiles() after rollback error = %v", err)
			}
			if len(after.Profiles) != len(before.Profiles) ||
				after.CurrentProfile != before.CurrentProfile ||
				after.PreviousProfile != before.PreviousProfile {
				t.Fatalf("profiles after rollback = %#v, want %#v", after, before)
			}
			if TokenDataExistsKeychainForIdentity(incoming.CorpID, incoming.UserID) ||
				TokenDataExistsKeychainForCorpID(incoming.CorpID) {
				t.Fatal("failed login left incoming token slots behind")
			}
			legacy, err := LoadTokenDataKeychain()
			if err != nil {
				t.Fatalf("LoadTokenDataKeychain() after rollback error = %v", err)
			}
			if legacy.CorpID != existing.CorpID || legacy.UserID != existing.UserID {
				t.Fatalf("legacy mirror after rollback = %#v, want existing identity", legacy)
			}
		})
	}
}

func TestSetCurrentProfileRollsBackWhenLegacyMirrorSyncFails(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	for _, userID := range []string{"user_1", "user_2"} {
		data := testToken("at_"+userID, "corp_same", "同一组织")
		data.UserID = userID
		if err := SaveTokenData(configDir, data); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", userID, err)
		}
	}

	before, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	originalSaveLegacy := profilesSaveLegacy
	failure := errors.New("save legacy mirror")
	profilesSaveLegacy = func(*TokenData) error { return failure }
	_, err = SetCurrentProfile(configDir, "corp_same:user_1")
	profilesSaveLegacy = originalSaveLegacy
	if !errors.Is(err, failure) {
		t.Fatalf("SetCurrentProfile() error = %v, want %v", err, failure)
	}
	assertProfileSwitchRolledBack(t, configDir, before)
}

func TestUsePreviousProfileRollsBackWhenLegacyMirrorSyncFails(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	for _, userID := range []string{"user_1", "user_2"} {
		data := testToken("at_"+userID, "corp_same", "同一组织")
		data.UserID = userID
		if err := SaveTokenData(configDir, data); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", userID, err)
		}
	}

	before, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	originalSaveLegacy := profilesSaveLegacy
	failure := errors.New("save legacy mirror")
	profilesSaveLegacy = func(*TokenData) error { return failure }
	_, err = UsePreviousProfile(configDir)
	profilesSaveLegacy = originalSaveLegacy
	if !errors.Is(err, failure) {
		t.Fatalf("UsePreviousProfile() error = %v, want %v", err, failure)
	}
	assertProfileSwitchRolledBack(t, configDir, before)
}

func assertProfileSwitchRolledBack(t *testing.T, configDir string, before *ProfilesConfig) {
	t.Helper()
	after, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() after rollback error = %v", err)
	}
	if after.CurrentProfile != before.CurrentProfile ||
		after.PreviousProfile != before.PreviousProfile ||
		after.OrgCurrentProfiles["corp_same"] != before.OrgCurrentProfiles["corp_same"] {
		t.Fatalf("profile selection after rollback = %#v, want %#v", after, before)
	}
	_, currentUserID, _ := ParseIdentitySelector(before.CurrentProfile)
	org, err := LoadTokenDataKeychainForCorpID("corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v", err)
	}
	if org.UserID != currentUserID {
		t.Fatalf("organization mirror user = %q, want %q", org.UserID, currentUserID)
	}
	legacy, err := LoadTokenDataKeychain()
	if err != nil {
		t.Fatalf("LoadTokenDataKeychain() error = %v", err)
	}
	if legacy.UserID != currentUserID {
		t.Fatalf("legacy mirror user = %q, want %q", legacy.UserID, currentUserID)
	}
}

func TestFutureProfilesVersionIsNotDowngradedOrOverwritten(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	future := []byte(`{
  "version": 3,
  "currentProfile": "corp_future:user_1",
  "orgCurrentProfiles": {"corp_future": "corp_future:user_1"},
  "futureField": {"preserve": true},
  "profiles": [{
    "name": "未来账号",
    "corpId": "corp_future",
    "userId": "user_1",
    "futureProfileField": "preserve"
  }]
}
`)
	if err := os.WriteFile(ProfilesPath(configDir), future, 0o600); err != nil {
		t.Fatalf("write future profiles fixture: %v", err)
	}

	incoming := testToken("at_new", "corp_new", "新组织")
	if err := preflightTokenPersistence(configDir); err == nil ||
		!strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("preflightTokenPersistence() error = %v, want future version rejection", err)
	}
	if err := preflightTokenRefreshPersistence(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("preflightTokenRefreshPersistence() error = %v, want future version rejection", err)
	}
	if err := SaveTokenData(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("SaveTokenData() error = %v, want future version rejection", err)
	}
	if TokenDataExistsKeychainForIdentity(incoming.CorpID, incoming.UserID) {
		t.Fatal("future version rejection left a new identity token behind")
	}
	if _, err := SetCurrentProfile(configDir, "corp_future:user_1"); err == nil ||
		!strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("SetCurrentProfile() error = %v, want future version rejection", err)
	}
	if _, _, err := ResolveProfileDeletionScope(configDir, "corp_future:user_1"); err == nil ||
		!strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("ResolveProfileDeletionScope() error = %v, want future version rejection", err)
	}

	after, err := os.ReadFile(ProfilesPath(configDir))
	if err != nil {
		t.Fatalf("read future profiles fixture: %v", err)
	}
	if string(after) != string(future) {
		t.Fatalf("future profiles file was rewritten:\n%s", after)
	}
}

func TestDeleteProfilePreservesOtherProfiles(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	if err := SaveTokenData(configDir, testToken("at_a", "corp_a", "A Org")); err != nil {
		t.Fatalf("SaveTokenData(A) error = %v", err)
	}
	if err := SaveTokenData(configDir, testToken("at_b", "corp_b", "B Org")); err != nil {
		t.Fatalf("SaveTokenData(B) error = %v", err)
	}
	if err := DeleteTokenDataForProfile(configDir, "corp_b"); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(B) error = %v", err)
	}
	if _, err := LoadTokenDataForProfile(configDir, "corp_b"); err == nil {
		t.Fatal("LoadTokenDataForProfile(B) error = nil after delete, want failure")
	}
	loadedA, err := LoadTokenDataForProfile(configDir, "corp_a")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(A) error = %v", err)
	}
	if loadedA.AccessToken != "at_a" {
		t.Fatalf("profile A token = %q, want at_a", loadedA.AccessToken)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 ||
		cfg.CurrentProfile != "corp_a:user_corp_a" ||
		cfg.PreviousProfile != "" {
		t.Fatalf("profiles after delete = %#v", cfg)
	}
}

func TestDeleteMissingProfileDoesNotDeleteCurrentToken(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	if err := SaveTokenData(configDir, testToken("at_current", "corp_current", "Current Org")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	if err := DeleteTokenDataForProfile(configDir, "corp_missing"); err == nil {
		t.Fatal("DeleteTokenDataForProfile(missing) error = nil, want not found")
	}
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() after missing delete error = %v", err)
	}
	if loaded.AccessToken != "at_current" {
		t.Fatalf("current token after missing delete = %q, want at_current", loaded.AccessToken)
	}
}

func TestUpsertProfileFromTokenUpdatesSameIdentity(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "旧组织名")
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	second := testToken("at_second", "corp_same", "新组织名")
	second.UserID = first.UserID
	second.UserName = "Updated User"
	second.ClientID = "client_updated"
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1: %#v", len(cfg.Profiles), cfg.Profiles)
	}
	profile := cfg.Profiles[0]
	if profile.CorpName != "新组织名" {
		t.Fatalf("corpName = %q, want 新组织名", profile.CorpName)
	}
	if profile.UserID != first.UserID || profile.UserName != "Updated User" || profile.ClientID != "client_updated" {
		t.Fatalf("profile metadata was not overwritten: %#v", profile)
	}
	loaded, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile() error = %v", err)
	}
	if loaded.AccessToken != "at_second" {
		t.Fatalf("access token = %q, want at_second", loaded.AccessToken)
	}
}

func TestSameCorpDifferentUsersAreStoredSeparately(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	first.UserName = "账号一"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	second.UserName = "账号二"

	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2: %#v", len(cfg.Profiles), cfg.Profiles)
	}

	loadedFirst, err := LoadTokenDataForProfile(configDir, "corp_same:user_1")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(first identity) error = %v", err)
	}
	if loadedFirst.AccessToken != "at_first" {
		t.Fatalf("first access token = %q, want at_first", loadedFirst.AccessToken)
	}
	loadedSecond, err := LoadTokenDataForProfile(configDir, "corp_same:user_2")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(second identity) error = %v", err)
	}
	if loadedSecond.AccessToken != "at_second" {
		t.Fatalf("second access token = %q, want at_second", loadedSecond.AccessToken)
	}

	loadedOrg, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(org) error = %v", err)
	}
	if loadedOrg.UserID != "user_2" {
		t.Fatalf("org current user = %q, want user_2", loadedOrg.UserID)
	}
}

func TestSaveTokenDataLogsIdentitySlotDecisionWithoutCredentials(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv("DWS_DEBUG_AUTH", "1")
	configDir := t.TempDir()
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	token := testToken("secret-access-token", "corp_same", "同一组织")
	token.RefreshToken = "secret-refresh-token"
	token.UserID = "user_two"
	token.UserName = "账号二"
	if err := SaveTokenData(configDir, token); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	got := logs.String()
	for _, want := range []string{
		`"msg":"auth.token.persist.plan"`,
		`"msg":"auth.token.persist.done"`,
		`"corp_id":"corp_same"`,
		`"user_id":"user_two"`,
		`"identity_selector":"corp_same:user_two"`,
		`"write_identity_slot":true`,
		`"write_org_mirror":true`,
		`"write_global_mirror":true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostic logs missing %q:\n%s", want, got)
		}
	}
	for _, secret := range []string{"secret-access-token", "secret-refresh-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostic logs exposed credential %q:\n%s", secret, got)
		}
	}
}

func TestMigrationBuildsOrgCurrentFromMatchingCorpMirror(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenDataKeychainForIdentity(first.CorpID, first.UserID, first); err != nil {
		t.Fatalf("SaveTokenDataKeychainForIdentity(first) error = %v", err)
	}
	if err := SaveTokenDataKeychainForIdentity(second.CorpID, second.UserID, second); err != nil {
		t.Fatalf("SaveTokenDataKeychainForIdentity(second) error = %v", err)
	}
	if err := SaveTokenDataKeychainForCorpID(second.CorpID, second); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID(second) error = %v", err)
	}
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        1,
		CurrentProfile: "corp_same",
		Profiles: []Profile{
			{Name: "账号一", CorpID: "corp_same", CorpName: "同一组织", UserID: "user_1"},
			{Name: "账号二", CorpID: "corp_same", CorpName: "同一组织", UserID: "user_2"},
		},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if got := cfg.OrgCurrentProfiles["corp_same"]; got != "corp_same:user_2" {
		t.Fatalf("org current = %q, want corp_same:user_2", got)
	}
	if cfg.CurrentProfile != "corp_same:user_2" {
		t.Fatalf("current profile = %q, want corp_same:user_2", cfg.CurrentProfile)
	}
}

func TestMigrationDoesNotGuessOrgCurrentForMultipleAccounts(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	for _, userID := range []string{"user_1", "user_2"} {
		token := testToken("at_"+userID, "corp_same", "同一组织")
		token.UserID = userID
		if err := SaveTokenDataKeychainForIdentity(token.CorpID, token.UserID, token); err != nil {
			t.Fatalf("SaveTokenDataKeychainForIdentity(%s) error = %v", userID, err)
		}
	}
	raw := `{
  "version": 1,
  "currentProfile": "corp_same",
  "profiles": [
    {"name":"账号一","corpId":"corp_same","corpName":"同一组织","userId":"user_1"},
    {"name":"账号二","corpId":"corp_same","corpName":"同一组织","userId":"user_2"}
  ]
}`
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile(profiles.json) error = %v", err)
	}

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if got := cfg.OrgCurrentProfiles["corp_same"]; got != "" {
		t.Fatalf("org current = %q, want empty", got)
	}
	if _, err := LoadTokenDataForProfile(configDir, "corp_same"); err == nil {
		t.Fatal("LoadTokenDataForProfile(corp) error = nil, want ambiguous default failure")
	}
}

func TestLoginStoresExactCurrentAndPreviousSelectors(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_first", "第一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_second", "第二组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_second:user_2" {
		t.Fatalf("current profile = %q, want corp_second:user_2", cfg.CurrentProfile)
	}
	if cfg.PreviousProfile != "corp_first:user_1" {
		t.Fatalf("previous profile = %q, want corp_first:user_1", cfg.PreviousProfile)
	}
	if got := cfg.OrgCurrentProfiles["corp_first"]; got != "corp_first:user_1" {
		t.Fatalf("first org current = %q, want corp_first:user_1", got)
	}
	if got := cfg.OrgCurrentProfiles["corp_second"]; got != "corp_second:user_2" {
		t.Fatalf("second org current = %q, want corp_second:user_2", got)
	}
}

func TestProfileSelectorSupportsOrganizationAndAccountNames(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "钉钉")
	first.UserID = "user_1"
	first.UserName = "孙博文"
	second := testToken("at_second", "corp_same", "钉钉")
	second.UserID = "user_2"
	second.UserName = "钉三多"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	for _, selector := range []string{
		"corp_same:user_1",
		"corp_same:孙博文",
		"钉钉:user_1",
		"钉钉:孙博文",
	} {
		loaded, err := LoadTokenDataForProfile(configDir, selector)
		if err != nil {
			t.Fatalf("LoadTokenDataForProfile(%q) error = %v", selector, err)
		}
		if loaded.UserID != "user_1" || loaded.AccessToken != "at_first" {
			t.Fatalf("LoadTokenDataForProfile(%q) = %#v, want first account", selector, loaded)
		}
	}
}

func TestProfileSelectorRejectsDuplicateAccountName(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	for _, userID := range []string{"user_1", "user_2"} {
		token := testToken("at_"+userID, "corp_same", "钉钉")
		token.UserID = userID
		token.UserName = "同名用户"
		if err := SaveTokenData(configDir, token); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", userID, err)
		}
	}

	_, err := LoadTokenDataForProfile(configDir, "钉钉:同名用户")
	if err == nil {
		t.Fatal("LoadTokenDataForProfile(duplicate userName) error = nil")
	}
	for _, want := range []string{"ambiguous", "corp_same:user_1", "corp_same:user_2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestProfileSelectorRejectsDuplicateOrganizationName(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	for _, corpID := range []string{"corp_1", "corp_2"} {
		token := testToken("at_"+corpID, corpID, "同名组织")
		token.UserID = "user_1"
		token.UserName = "同一用户"
		if err := SaveTokenData(configDir, token); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", corpID, err)
		}
	}

	_, err := LoadTokenDataForProfile(configDir, "同名组织:同一用户")
	if err == nil {
		t.Fatal("LoadTokenDataForProfile(duplicate corpName) error = nil")
	}
	for _, want := range []string{"ambiguous", "corp_1", "corp_2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestExactProfileSwitchPreservesPreviousIdentityAndUpdatesOrgCurrent(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	selected, err := SetCurrentProfile(configDir, "corp_same:user_1")
	if err != nil {
		t.Fatalf("SetCurrentProfile(exact) error = %v", err)
	}
	if selected.UserID != "user_1" {
		t.Fatalf("selected user = %q, want user_1", selected.UserID)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_same:user_1" {
		t.Fatalf("current profile = %q, want corp_same:user_1", cfg.CurrentProfile)
	}
	if cfg.PreviousProfile != "corp_same:user_2" {
		t.Fatalf("previous profile = %q, want corp_same:user_2", cfg.PreviousProfile)
	}
	loadedOrg, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(org) error = %v", err)
	}
	if loadedOrg.UserID != "user_1" {
		t.Fatalf("org current user = %q, want user_1", loadedOrg.UserID)
	}
	previous, err := UsePreviousProfile(configDir)
	if err != nil {
		t.Fatalf("UsePreviousProfile() error = %v", err)
	}
	if previous.UserID != "user_2" {
		t.Fatalf("previous user = %q, want user_2", previous.UserID)
	}
	cfg, err = LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() after previous error = %v", err)
	}
	if cfg.CurrentProfile != "corp_same:user_2" || cfg.PreviousProfile != "corp_same:user_1" {
		t.Fatalf("profile pointers after previous = %#v", cfg)
	}
}

func TestExactRuntimeRefreshDoesNotChangeOrgCurrent(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	refreshed := *first
	refreshed.AccessToken = "at_first_refreshed"
	SetRuntimeProfile("corp_same:user_1")
	if err := SaveTokenData(configDir, &refreshed); err != nil {
		t.Fatalf("SaveTokenData(exact refresh) error = %v", err)
	}
	SetRuntimeProfile("")

	loadedOrg, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(org) error = %v", err)
	}
	if loadedOrg.UserID != "user_2" || loadedOrg.AccessToken != "at_second" {
		t.Fatalf("org token changed after exact refresh: %#v", loadedOrg)
	}
	loadedFirst, err := LoadTokenDataForProfile(configDir, "corp_same:user_1")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(first) error = %v", err)
	}
	if loadedFirst.AccessToken != "at_first_refreshed" {
		t.Fatalf("exact refreshed token = %q, want at_first_refreshed", loadedFirst.AccessToken)
	}
}

func TestDeleteExactProfilePreservesOtherAccountInSameCorp(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	if err := DeleteTokenDataForProfile(configDir, "corp_same:user_2"); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(exact current) error = %v", err)
	}
	if _, err := LoadTokenDataForProfile(configDir, "corp_same:user_2"); err == nil {
		t.Fatal("deleted exact profile is still loadable")
	}
	loadedFirst, err := LoadTokenDataForProfile(configDir, "corp_same:user_1")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(remaining) error = %v", err)
	}
	if loadedFirst.AccessToken != "at_first" {
		t.Fatalf("remaining access token = %q, want at_first", loadedFirst.AccessToken)
	}
	loadedOrg, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(org) error = %v", err)
	}
	if loadedOrg.UserID != "user_1" {
		t.Fatalf("org current user after delete = %q, want user_1", loadedOrg.UserID)
	}
}

func TestDeleteOrgCurrentLeavesMultipleAccountsUnselected(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	for _, userID := range []string{"user_1", "user_2", "user_3"} {
		token := testToken("at_"+userID, "corp_same", "同一组织")
		token.UserID = userID
		token.UserName = "账号" + userID
		if err := SaveTokenData(configDir, token); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", userID, err)
		}
	}

	if err := DeleteTokenDataForProfile(configDir, "corp_same:user_3"); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(current) error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if got := cfg.OrgCurrentProfiles["corp_same"]; got != "" {
		t.Fatalf("org current = %q, want empty", got)
	}
	if _, err := LoadTokenDataForProfile(configDir, "corp_same"); err == nil {
		t.Fatal("LoadTokenDataForProfile(corp) error = nil, want explicit account requirement")
	}

	if err := DeleteTokenDataForProfile(configDir, "corp_same"); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(unselected org) error = %v", err)
	}
	cfg, err = LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() after organization delete error = %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("profiles after unselected organization delete = %#v, want empty", cfg.Profiles)
	}
}

func TestDeleteOrgProfileRemovesAllAccountsInCorp(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "同一组织")
	first.UserID = "user_1"
	second := testToken("at_second", "corp_same", "同一组织")
	second.UserID = "user_2"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	if err := DeleteTokenDataForProfile(configDir, "corp_same"); err != nil {
		t.Fatalf("DeleteTokenDataForProfile(org) error = %v", err)
	}
	if TokenDataExistsKeychainForIdentity("corp_same", "user_1") ||
		TokenDataExistsKeychainForIdentity("corp_same", "user_2") ||
		TokenDataExistsKeychainForCorpID("corp_same") {
		t.Fatal("organization logout left account token slots behind")
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("profiles after org logout = %#v, want empty", cfg.Profiles)
	}
}

func TestDeleteExactProfileRollsBackWhenOrganizationMirrorDeleteFails(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("at_user_1", "corp_same", "同一组织")
	data.UserID = "user_1"
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	originalDeleteCorp := tokenDeleteKeychainForCorpID
	failure := errors.New("delete organization mirror")
	tokenDeleteKeychainForCorpID = func(string) error { return failure }
	err := DeleteTokenDataForProfile(configDir, "corp_same:user_1")
	tokenDeleteKeychainForCorpID = originalDeleteCorp
	if !errors.Is(err, failure) {
		t.Fatalf("DeleteTokenDataForProfile() error = %v, want %v", err, failure)
	}

	assertProfileDeletionRolledBack(t, configDir, []string{"corp_same:user_1"}, "corp_same:user_1")
}

func TestDeleteExactProfileRollsBackWhenIdentityDeleteFails(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	for _, userID := range []string{"user_1", "user_2"} {
		data := testToken("at_"+userID, "corp_same", "同一组织")
		data.UserID = userID
		if err := SaveTokenData(configDir, data); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", userID, err)
		}
	}

	originalDeleteIdentity := tokenDeleteKeychainIdentity
	failure := errors.New("delete identity")
	tokenDeleteKeychainIdentity = func(corpID, userID string) error {
		if corpID == "corp_same" && userID == "user_2" {
			return failure
		}
		return originalDeleteIdentity(corpID, userID)
	}
	err := DeleteTokenDataForProfile(configDir, "corp_same:user_2")
	tokenDeleteKeychainIdentity = originalDeleteIdentity
	if !errors.Is(err, failure) {
		t.Fatalf("DeleteTokenDataForProfile() error = %v, want %v", err, failure)
	}

	assertProfileDeletionRolledBack(
		t,
		configDir,
		[]string{"corp_same:user_1", "corp_same:user_2"},
		"corp_same:user_2",
	)
}

func TestDeleteOrganizationRollsBackPreviouslyDeletedIdentities(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	for _, userID := range []string{"user_1", "user_2"} {
		data := testToken("at_"+userID, "corp_same", "同一组织")
		data.UserID = userID
		if err := SaveTokenData(configDir, data); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", userID, err)
		}
	}

	originalDeleteIdentity := tokenDeleteKeychainIdentity
	deleteCalls := 0
	failure := errors.New("delete second identity")
	tokenDeleteKeychainIdentity = func(corpID, userID string) error {
		deleteCalls++
		if deleteCalls == 2 {
			return failure
		}
		return originalDeleteIdentity(corpID, userID)
	}
	err := DeleteTokenDataForProfile(configDir, "corp_same")
	tokenDeleteKeychainIdentity = originalDeleteIdentity
	if !errors.Is(err, failure) {
		t.Fatalf("DeleteTokenDataForProfile() error = %v, want %v", err, failure)
	}

	assertProfileDeletionRolledBack(
		t,
		configDir,
		[]string{"corp_same:user_1", "corp_same:user_2"},
		"corp_same:user_2",
	)
}

func TestDeleteProfileAllowsUnreadableTargetTokenSlots(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("at_user_1", "corp_same", "同一组织")
	data.UserID = "user_1"
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	for _, account := range []string{
		TokenAccountForIdentity(data.CorpID, data.UserID),
		TokenAccountForCorpID(data.CorpID),
		keychain.AccountToken,
	} {
		if err := keychain.Set(keychain.Service, account, "{unreadable"); err != nil {
			t.Fatalf("write unreadable token slot %q: %v", account, err)
		}
	}

	if err := DeleteTokenDataForProfile(configDir, ProfileSelector(Profile{
		CorpID: data.CorpID,
		UserID: data.UserID,
	})); err != nil {
		t.Fatalf("DeleteTokenDataForProfile() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("profiles after unreadable target deletion = %#v, want empty", cfg.Profiles)
	}
	if TokenDataExistsKeychainForIdentity(data.CorpID, data.UserID) ||
		TokenDataExistsKeychainForCorpID(data.CorpID) ||
		TokenDataExistsKeychain() {
		t.Fatal("unreadable target token slots still exist after logout")
	}
}

func assertProfileDeletionRolledBack(t *testing.T, configDir string, selectors []string, current string) {
	t.Helper()
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != len(selectors) {
		t.Fatalf("profiles after rollback = %#v, want %d profiles", cfg.Profiles, len(selectors))
	}
	if cfg.CurrentProfile != current {
		t.Fatalf("current profile after rollback = %q, want %q", cfg.CurrentProfile, current)
	}
	for _, selector := range selectors {
		if _, err := LoadTokenDataForProfile(configDir, selector); err != nil {
			t.Fatalf("LoadTokenDataForProfile(%q) after rollback error = %v", selector, err)
		}
	}
	legacy, err := LoadTokenDataKeychain()
	if err != nil {
		t.Fatalf("LoadTokenDataKeychain() after rollback error = %v", err)
	}
	_, currentUserID, _ := ParseIdentitySelector(current)
	if legacy.UserID != currentUserID {
		t.Fatalf("legacy mirror user after rollback = %q, want %q", legacy.UserID, currentUserID)
	}
	org, err := LoadTokenDataKeychainForCorpID("corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataKeychainForCorpID() after rollback error = %v", err)
	}
	if org.UserID != currentUserID {
		t.Fatalf("organization mirror user after rollback = %q, want %q", org.UserID, currentUserID)
	}
}

func TestOAuthLoginEnrichesIdentityBeforePersistingSameCorpAccount(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	existing := testToken("at_existing", "corp_same", "同一组织")
	existing.UserID = "user_1"
	if err := SaveTokenData(configDir, existing); err != nil {
		t.Fatalf("SaveTokenData(existing) error = %v", err)
	}

	incoming := testToken("at_incoming", "corp_same", "同一组织")
	incoming.UserID = ""
	incoming.UserName = ""
	provider := NewOAuthProvider(configDir, nil)
	provider.IdentityEnricher = func(_ context.Context, data *TokenData) error {
		loaded, err := LoadTokenDataForProfile(configDir, "corp_same")
		if err != nil {
			t.Fatalf("LoadTokenDataForProfile(existing during enrichment) error = %v", err)
		}
		if loaded.UserID != "user_1" || loaded.AccessToken != "at_existing" {
			t.Fatalf("existing account was changed before identity enrichment: %#v", loaded)
		}
		data.UserID = "user_2"
		data.UserName = "账号二"
		return nil
	}

	if err := provider.persistLoginToken(context.Background(), incoming); err != nil {
		t.Fatalf("persistLoginToken() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2: %#v", len(cfg.Profiles), cfg.Profiles)
	}
}

func TestOAuthLoginIdentityFailureLeavesExistingSameCorpAccountUntouched(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	existing := testToken("at_existing", "corp_same", "同一组织")
	existing.UserID = "user_1"
	if err := SaveTokenData(configDir, existing); err != nil {
		t.Fatalf("SaveTokenData(existing) error = %v", err)
	}

	incoming := testToken("at_incoming", "corp_same", "同一组织")
	incoming.UserID = ""
	provider := NewOAuthProvider(configDir, nil)
	provider.IdentityEnricher = func(context.Context, *TokenData) error {
		return errors.New("identity lookup failed")
	}
	if err := provider.persistLoginToken(context.Background(), incoming); err == nil {
		t.Fatal("persistLoginToken() error = nil, want identity lookup failure")
	}

	loaded, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(existing) error = %v", err)
	}
	if loaded.UserID != "user_1" || loaded.AccessToken != "at_existing" {
		t.Fatalf("existing account changed after identity failure: %#v", loaded)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(cfg.Profiles))
	}
}

func TestOAuthLoginMissingIdentityCannotOverwriteLegacySameCorpAccount(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	existing := testToken("at_existing", "corp_same", "同一组织")
	existing.UserID = ""
	existing.UserName = ""
	if err := SaveTokenData(configDir, existing); err != nil {
		t.Fatalf("SaveTokenData(existing legacy) error = %v", err)
	}

	incoming := testToken("at_incoming", "corp_same", "同一组织")
	incoming.UserID = ""
	incoming.UserName = ""
	provider := NewOAuthProvider(configDir, nil)
	provider.IdentityEnricher = func(context.Context, *TokenData) error { return nil }
	if err := provider.persistLoginToken(context.Background(), incoming); err == nil {
		t.Fatal("persistLoginToken() error = nil, want missing identity failure")
	}

	loaded, err := LoadTokenDataForProfile(configDir, "corp_same")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(existing) error = %v", err)
	}
	if loaded.AccessToken != "at_existing" {
		t.Fatalf("legacy account changed after missing identity: %#v", loaded)
	}
}

func TestLoadProfilesKeepsDifferentIdentitiesInSameCorp(t *testing.T) {
	configDir := t.TempDir()
	raw := `{
  "version": 1,
  "primaryProfile": "corp_same:user_1",
  "currentProfile": "corp_same:user_2",
  "previousProfile": "corp_same:user_1",
  "profiles": [
    {
      "name": "账号一",
      "corpId": "corp_same",
      "userId": "user_1"
    },
    {
      "name": "账号二",
      "corpId": "corp_same",
      "userId": "user_2"
    },
    {
      "name": "账号二重复记录",
      "corpId": "corp_same",
      "userId": "user_2"
    }
  ]
}`
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile(profiles.json) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2: %#v", len(cfg.Profiles), cfg.Profiles)
	}
	if cfg.PrimaryProfile != "corp_same:user_1" ||
		cfg.CurrentProfile != "corp_same:user_2" ||
		cfg.PreviousProfile != "corp_same:user_1" {
		t.Fatalf("identity pointers were not preserved: %#v", cfg)
	}
}

func TestUpsertProfileFromTokenPromotesCorpIDNameToCorpName(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "")
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	second := testToken("at_second", "corp_same", "新组织名")
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1: %#v", len(cfg.Profiles), cfg.Profiles)
	}
	if cfg.Profiles[0].Name != "新组织名" {
		t.Fatalf("profile name = %q, want 新组织名", cfg.Profiles[0].Name)
	}

	resolved, err := ResolveProfile(configDir, "新组织名")
	if err != nil {
		t.Fatalf("ResolveProfile(corpName) error = %v", err)
	}
	if resolved.CorpID != "corp_same" {
		t.Fatalf("resolved corpId = %q, want corp_same", resolved.CorpID)
	}
}

func TestLoadProfilesPromotesLegacyCorpIDNameToCorpName(t *testing.T) {
	configDir := t.TempDir()
	raw := `{
  "version": 1,
  "primaryProfile": "corp_same",
  "currentProfile": "corp_same",
  "profiles": [
    {
      "name": "corp_same",
      "corpId": "corp_same",
      "corpName": "新组织名"
    }
  ]
}`
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile(profiles.json) error = %v", err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(cfg.Profiles))
	}
	if cfg.Profiles[0].Name != "新组织名" {
		t.Fatalf("profile name = %q, want 新组织名", cfg.Profiles[0].Name)
	}
}

func TestLegacyKeychainMigrationInitializesProfile(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	legacy := testToken("at_legacy", "corp_legacy", "Legacy Org")
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain() error = %v", err)
	}
	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != "at_legacy" {
		t.Fatalf("loaded token = %q, want at_legacy", loaded.AccessToken)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.PrimaryProfile != "" ||
		cfg.CurrentProfile != "corp_legacy:user_corp_legacy" ||
		cfg.OrgCurrentProfiles["corp_legacy"] != "corp_legacy:user_corp_legacy" {
		t.Fatalf("profile pointers after migration = %#v", cfg)
	}
	if !TokenDataExistsKeychainForCorpID("corp_legacy") {
		t.Fatal("corp-scoped token should exist after migration")
	}
	if !TokenDataExistsKeychainForIdentity("corp_legacy", legacy.UserID) {
		t.Fatal("identity-scoped token should exist after migration")
	}
}

func TestLegacyCorpScopedTokenMigratesToIdentitySlot(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	legacy := testToken("at_legacy_corp", "corp_legacy", "Legacy Org")
	if err := SaveTokenDataKeychainForCorpID(legacy.CorpID, legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        1,
		PrimaryProfile: legacy.CorpID,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
			UserID:   legacy.UserID,
			UserName: legacy.UserName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	if err := EnsureProfilesMigration(configDir); err != nil {
		t.Fatalf("EnsureProfilesMigration() error = %v", err)
	}
	if !TokenDataExistsKeychainForIdentity(legacy.CorpID, legacy.UserID) {
		t.Fatal("identity-scoped token should exist after corp-scoped migration")
	}
	if !TokenDataExistsKeychainForCorpID(legacy.CorpID) {
		t.Fatal("legacy corp-scoped token should remain after migration")
	}
}

func TestTokenDataExistsKeychain(t *testing.T) {
	cleanupKeychain(t)

	configDir := t.TempDir()

	// Should be false before save
	if TokenDataExistsKeychain() {
		t.Fatal("TokenDataExistsKeychain() should be false before save")
	}

	// Save data
	data := &TokenData{
		AccessToken: "at_test",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	// Should be true after save
	if !TokenDataExistsKeychain() {
		t.Fatal("TokenDataExistsKeychain() should be true after save")
	}
}

func TestProfileReadPathsWaitForAuthLock(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	for _, tc := range []struct {
		name string
		read func() (*Profile, *TokenData, error)
	}{
		{
			name: "resolve profile",
			read: func() (*Profile, *TokenData, error) {
				profile, err := ResolveProfile(configDir, "corp_locked:user_locked")
				return profile, nil, err
			},
		},
		{
			name: "load token",
			read: func() (*Profile, *TokenData, error) {
				data, err := LoadTokenDataForProfile(configDir, "corp_locked:user_locked")
				return nil, data, err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(ProfilesPath(configDir))
			_ = DeleteTokenDataKeychainForIdentity("corp_locked", "user_locked")

			lock, err := AcquireDualLock(context.Background(), configDir)
			if err != nil {
				t.Fatalf("AcquireDualLock() error = %v", err)
			}

			type result struct {
				profile *Profile
				data    *TokenData
				err     error
			}
			done := make(chan result, 1)
			go func() {
				profile, data, err := tc.read()
				done <- result{profile: profile, data: data, err: err}
			}()

			select {
			case got := <-done:
				lock.Release()
				t.Fatalf("read completed before auth lock release: %+v", got)
			case <-time.After(100 * time.Millisecond):
			}

			data := testToken("at_locked", "corp_locked", "Locked Org")
			data.UserID = "user_locked"
			if err := SaveTokenDataKeychainForIdentity(data.CorpID, data.UserID, data); err != nil {
				lock.Release()
				t.Fatalf("SaveTokenDataKeychainForIdentity() error = %v", err)
			}
			if err := SaveProfiles(configDir, &ProfilesConfig{
				Version:        profilesVersion,
				CurrentProfile: "corp_locked:user_locked",
				OrgCurrentProfiles: map[string]string{
					"corp_locked": "corp_locked:user_locked",
				},
				Profiles: []Profile{{
					Name:     "Locked Org",
					CorpID:   "corp_locked",
					CorpName: "Locked Org",
					UserID:   "user_locked",
				}},
			}); err != nil {
				lock.Release()
				t.Fatalf("SaveProfiles() error = %v", err)
			}
			lock.Release()

			select {
			case got := <-done:
				if got.err != nil {
					t.Fatalf("read after auth lock release error = %v", got.err)
				}
				if got.profile != nil && ProfileSelector(*got.profile) != "corp_locked:user_locked" {
					t.Fatalf("ResolveProfile() = %#v", got.profile)
				}
				if got.data != nil && got.data.AccessToken != "at_locked" {
					t.Fatalf("LoadTokenDataForProfile() = %#v", got.data)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("read remained blocked after auth lock release")
			}
		})
	}
}

func TestV2EmptyProfilesDoesNotRestoreLegacyToken(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	legacy := testToken("at_legacy_stale", "corp_stale", "Stale Org")
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain() error = %v", err)
	}
	if err := SaveProfiles(configDir, &ProfilesConfig{Version: profilesVersion}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	if _, err := LoadTokenData(configDir); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("LoadTokenData() error = %v, want ErrTokenDataNotFound", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("v2 empty config restored stale profile: %#v", cfg.Profiles)
	}
}

func TestIdentityLoadRejectsOrganizationMirrorWithoutUserID(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: "corp_same:user_1",
		OrgCurrentProfiles: map[string]string{
			"corp_same": "corp_same:user_1",
		},
		Profiles: []Profile{{
			Name:   "账号一",
			CorpID: "corp_same",
			UserID: "user_1",
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	legacy := testToken("at_unknown_identity", "corp_same", "同一组织")
	legacy.UserID = ""
	if err := SaveTokenDataKeychainForCorpID("corp_same", legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}

	_, err := LoadTokenDataForProfile(configDir, "corp_same:user_1")
	if err == nil || !strings.Contains(err.Error(), "has no userId") {
		t.Fatalf("LoadTokenDataForProfile() error = %v, want missing mirror identity error", err)
	}
	if TokenDataExistsKeychainForIdentity("corp_same", "user_1") {
		t.Fatal("organization mirror without userId was persisted as an identity token")
	}
}

func TestProfilesMigrationDoesNotOverwriteUnreadableIdentityToken(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("at_org_mirror", "corp_same", "同一组织")
	data.UserID = "user_1"
	if err := SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	if err := keychain.Set(keychain.Service, TokenAccountForIdentity(data.CorpID, data.UserID), "{unreadable"); err != nil {
		t.Fatalf("write unreadable identity slot: %v", err)
	}
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version: profilesVersion,
		Profiles: []Profile{{
			Name:   "账号一",
			CorpID: data.CorpID,
			UserID: data.UserID,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	err := EnsureProfilesMigration(configDir)
	if err == nil || !strings.Contains(err.Error(), "parse token data") {
		t.Fatalf("EnsureProfilesMigration() error = %v, want unreadable identity error", err)
	}
	raw, getErr := keychain.Get(keychain.Service, TokenAccountForIdentity(data.CorpID, data.UserID))
	if getErr != nil {
		t.Fatalf("read identity slot after migration: %v", getErr)
	}
	if raw != "{unreadable" {
		t.Fatalf("migration overwrote unreadable identity slot: %q", raw)
	}
}

func TestProfilesMigrationDoesNotIgnoreUnreadableOrganizationMirror(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version: 1,
		Profiles: []Profile{{
			Name:   "账号一",
			CorpID: "corp_same",
			UserID: "user_1",
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	if err := keychain.Set(keychain.Service, TokenAccountForCorpID("corp_same"), "{unreadable"); err != nil {
		t.Fatalf("write unreadable organization mirror: %v", err)
	}

	err := EnsureProfilesMigration(configDir)
	if err == nil || !strings.Contains(err.Error(), "parse token data") {
		t.Fatalf("EnsureProfilesMigration() error = %v, want unreadable mirror error", err)
	}
	cfg, loadErr := LoadProfiles(configDir)
	if loadErr != nil {
		t.Fatalf("LoadProfiles() error = %v", loadErr)
	}
	if cfg.Version != 1 {
		t.Fatalf("profiles version after failed migration = %d, want 1", cfg.Version)
	}
}

func TestProfileSelectorRoundTripsUserIDContainingColon(t *testing.T) {
	profile := Profile{CorpID: "corp", UserID: "user:with:colon"}
	selector := ProfileSelector(profile)
	if selector != "corp:user:with:colon" {
		t.Fatalf("ProfileSelector() = %q", selector)
	}
	corpID, userID, exact := ParseIdentitySelector(selector)
	if !exact || corpID != profile.CorpID || userID != profile.UserID {
		t.Fatalf("ParseIdentitySelector(%q) = %q, %q, %v", selector, corpID, userID, exact)
	}
}

func TestMissingCurrentIdentityClearsLegacyMirror(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("at_user_1", "corp_same", "同一组织")
	data.UserID = "user_1"
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := DeleteTokenDataKeychainForIdentity(data.CorpID, data.UserID); err != nil {
		t.Fatalf("DeleteTokenDataKeychainForIdentity() error = %v", err)
	}
	if err := DeleteTokenDataKeychainForCorpID(data.CorpID); err != nil {
		t.Fatalf("DeleteTokenDataKeychainForCorpID() error = %v", err)
	}

	if err := SyncLegacyTokenMirror(configDir); err != nil {
		t.Fatalf("SyncLegacyTokenMirror() error = %v", err)
	}
	if TokenDataExistsKeychain() {
		t.Fatal("legacy mirror still exists after current identity token was confirmed missing")
	}
	if _, err := os.Stat(filepath.Join(configDir, tokenJSONFile)); !os.IsNotExist(err) {
		t.Fatalf("token marker stat error = %v, want not exist", err)
	}
}

func TestOAuthLoginRequiresIdentityForFirstProfile(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	incoming := testToken("at_missing_identity", "corp_first", "First Org")
	incoming.UserID = ""
	incoming.UserName = ""

	provider := NewOAuthProvider(configDir, nil)
	provider.IdentityEnricher = func(context.Context, *TokenData) error { return nil }
	if err := provider.persistLoginToken(context.Background(), incoming); err == nil ||
		!strings.Contains(err.Error(), "userId is required") {
		t.Fatalf("persistLoginToken() error = %v, want required userId", err)
	}
	if TokenDataExistsKeychain() || TokenDataExistsKeychainForCorpID("corp_first") {
		t.Fatal("missing-identity login persisted token data")
	}
}

func TestDeleteAllTokenDataRemovesOrphanIdentitySlots(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	orphan := testToken("at_orphan", "corp_orphan", "Orphan Org")
	orphan.UserID = "user_orphan"
	if err := SaveTokenDataKeychainForIdentity(orphan.CorpID, orphan.UserID, orphan); err != nil {
		t.Fatalf("SaveTokenDataKeychainForIdentity() error = %v", err)
	}

	if err := DeleteAllTokenData(configDir); err != nil {
		t.Fatalf("DeleteAllTokenData() error = %v", err)
	}
	if TokenDataExistsKeychainForIdentity(orphan.CorpID, orphan.UserID) {
		t.Fatal("DeleteAllTokenData() left orphan identity token behind")
	}
}

func testToken(accessToken, corpID, corpName string) *TokenData {
	now := time.Now().UTC()
	return &TokenData{
		AccessToken:  accessToken,
		RefreshToken: "rt_" + accessToken,
		ExpiresAt:    now.Add(2 * time.Hour),
		RefreshExpAt: now.Add(30 * 24 * time.Hour),
		CorpID:       corpID,
		CorpName:     corpName,
		UserID:       "user_" + corpID,
		UserName:     "User " + corpID,
		ClientID:     "client_" + corpID,
	}
}

func TestTokenValidityChecks(t *testing.T) {
	t.Parallel()

	valid := &TokenData{
		AccessToken:  "at_valid",
		RefreshToken: "rt_valid",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
	}
	if !valid.IsAccessTokenValid() {
		t.Fatal("access token expiring in 2h should be valid")
	}
	if !valid.IsRefreshTokenValid() {
		t.Fatal("refresh token expiring in 24h should be valid")
	}

	expiringSoon := &TokenData{
		AccessToken: "at_soon",
		ExpiresAt:   time.Now().Add(3 * time.Minute),
	}
	if expiringSoon.IsAccessTokenValid() {
		t.Fatal("access token expiring inside 5m buffer should be invalid")
	}

	expiredRefresh := &TokenData{
		RefreshToken: "rt_expired",
		RefreshExpAt: time.Now().Add(-1 * time.Hour),
	}
	if expiredRefresh.IsRefreshTokenValid() {
		t.Fatal("expired refresh token should be invalid")
	}
}
