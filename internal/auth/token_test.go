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
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// cleanupKeychain isolates keychain state to a per-test temporary directory
// so that concurrent test packages (notably internal/app) don't read tokens
// written by these tests, and removes test data on completion.
func cleanupKeychain(t *testing.T) {
	t.Helper()
	SetRuntimeProfile("")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	t.Cleanup(func() {
		SetRuntimeProfile("")
		_ = keychain.Remove(keychain.Service, keychain.AccountToken)
	})
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
	if cfg.PrimaryProfile != "corp_a:user_corp_a" || cfg.CurrentProfile != "corp_b:user_corp_b" || cfg.PreviousProfile != "corp_a:user_corp_a" {
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
		t.Fatalf("current profile = %q, want corp_a", cfg.CurrentProfile)
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
	if len(cfg.Profiles) != 1 || cfg.CurrentProfile != "corp_a:user_corp_a" {
		t.Fatalf("profiles after delete = %#v", cfg)
	}
}

func TestSameCorpDifferentUsersCreateDistinctProfiles(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "旧组织名")
	first.UserID = "user_first"
	first.UserName = "First User"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	second := testToken("at_second", "corp_same", "新组织名")
	second.UserID = "user_second"
	second.UserName = "Second User"
	second.ClientID = "client_updated"
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
	if cfg.PrimaryProfile != "corp_same:user_first" || cfg.CurrentProfile != "corp_same:user_second" {
		t.Fatalf("profile pointers = primary %q current %q, want first/current second", cfg.PrimaryProfile, cfg.CurrentProfile)
	}
	if _, err := LoadTokenDataForProfile(configDir, "corp_same"); err == nil {
		t.Fatal("LoadTokenDataForProfile(corp_same) error = nil, want ambiguous selector")
	}
	loaded, err := LoadTokenDataForProfile(configDir, "corp_same:user_first")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(first profileId) error = %v", err)
	}
	if loaded.AccessToken != "at_first" {
		t.Fatalf("first access token = %q, want at_first", loaded.AccessToken)
	}
	loaded, err = LoadTokenDataForProfile(configDir, "corp_same:user_second")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(second profileId) error = %v", err)
	}
	if loaded.AccessToken != "at_second" {
		t.Fatalf("second access token = %q, want at_second", loaded.AccessToken)
	}
}

func TestProfileIDFromTokenRecomputesLegacyCorpOnlyIDWhenUserAppears(t *testing.T) {
	data := testToken("at", "corp_same", "Same Org")
	data.ProfileID = data.CorpID
	data.UserID = "user_later"

	if got := ProfileIDFromToken(data); got != "corp_same:user_later" {
		t.Fatalf("ProfileIDFromToken() = %q, want corp_same:user_later", got)
	}
}

func TestSaveTokenHookReceivesProfileID(t *testing.T) {
	var payload TokenData
	edition.Override(&edition.Hooks{
		SaveToken: func(configDir string, data []byte) error {
			return json.Unmarshal(data, &payload)
		},
	})
	t.Cleanup(func() { edition.Override(&edition.Hooks{}) })

	if err := SaveTokenData(t.TempDir(), &TokenData{
		AccessToken: "at",
		CorpID:      "corp_same",
		UserID:      "user_1",
		UserName:    "张三",
	}); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if payload.ProfileID != "corp_same:user_1" {
		t.Fatalf("hook profile_id = %q, want corp_same:user_1", payload.ProfileID)
	}
}

func TestSaveTokenHookProfileIDFallsBackToCorpID(t *testing.T) {
	var payload TokenData
	edition.Override(&edition.Hooks{
		SaveToken: func(configDir string, data []byte) error {
			return json.Unmarshal(data, &payload)
		},
	})
	t.Cleanup(func() { edition.Override(&edition.Hooks{}) })

	if err := SaveTokenData(t.TempDir(), &TokenData{
		AccessToken: "at",
		CorpID:      "corp_only",
	}); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if payload.ProfileID != "corp_only" {
		t.Fatalf("hook profile_id = %q, want corp_only", payload.ProfileID)
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

func TestLoadProfilesMigratesLegacyPointersToProfileID(t *testing.T) {
	configDir := t.TempDir()
	raw := `{
  "version": 1,
  "primaryProfile": "corp_same",
  "currentProfile": "corp_same",
  "profiles": [
    {
      "name": "Legacy Org",
      "corpId": "corp_same",
      "corpName": "Legacy Org",
      "userId": "user_legacy"
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
	if cfg.Version != 2 {
		t.Fatalf("version = %d, want 2", cfg.Version)
	}
	if cfg.PrimaryProfile != "corp_same:user_legacy" || cfg.CurrentProfile != "corp_same:user_legacy" {
		t.Fatalf("profile pointers = primary %q current %q, want migrated profileId", cfg.PrimaryProfile, cfg.CurrentProfile)
	}
	if cfg.Profiles[0].ProfileID != "corp_same:user_legacy" {
		t.Fatalf("profileId = %q, want corp_same:user_legacy", cfg.Profiles[0].ProfileID)
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
	if cfg.PrimaryProfile != "corp_legacy:user_corp_legacy" || cfg.CurrentProfile != "corp_legacy:user_corp_legacy" {
		t.Fatalf("profile pointers after migration = %#v", cfg)
	}
	if !TokenDataExistsKeychainForProfileID("corp_legacy:user_corp_legacy") {
		t.Fatal("profile-scoped token should exist after migration")
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
