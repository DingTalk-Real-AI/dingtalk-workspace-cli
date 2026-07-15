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
	"os"
	"path/filepath"
	"strings"
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
	if cfg.PrimaryProfile != "corp_a" || cfg.CurrentProfile != "corp_b" || cfg.PreviousProfile != "corp_a" {
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
	if cfg.CurrentProfile != "corp_a" {
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
	if len(cfg.Profiles) != 1 || cfg.CurrentProfile != "corp_a" {
		t.Fatalf("profiles after delete = %#v", cfg)
	}
}

func TestUpsertProfileFromTokenOverwritesSameCorp(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	first := testToken("at_first", "corp_same", "旧组织名")
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	second := testToken("at_second", "corp_same", "新组织名")
	second.UserID = "user_updated"
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
	if profile.UserID != "user_updated" || profile.UserName != "Updated User" || profile.ClientID != "client_updated" {
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
	if cfg.PrimaryProfile != "corp_legacy" || cfg.CurrentProfile != "corp_legacy" {
		t.Fatalf("profile pointers after migration = %#v", cfg)
	}
	if !TokenDataExistsKeychainForCorpID("corp_legacy") {
		t.Fatal("corp-scoped token should exist after migration")
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

func TestLoadValidAccessTokenReadOnlyUsesSelectedProfileWithoutProfileMutation(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()

	if err := SaveTokenData(configDir, testToken("at_read_only", "corp_read_only", "Read Only Org")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	profilesPath := ProfilesPath(configDir)
	before, err := os.ReadFile(profilesPath)
	if err != nil {
		t.Fatalf("ReadFile(profiles before) error = %v", err)
	}

	token, err := LoadValidAccessTokenReadOnly(configDir, "  Read Only Org  ")
	if err != nil {
		t.Fatalf("LoadValidAccessTokenReadOnly() error = %v", err)
	}
	if token != "at_read_only" {
		t.Fatalf("token = %q, want at_read_only", token)
	}
	after, err := os.ReadFile(profilesPath)
	if err != nil {
		t.Fatalf("ReadFile(profiles after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("read-only token lookup mutated profiles.json")
	}
}

func TestLoadTokenDataForProfileReadOnlyDoesNotRepairMalformedProfiles(t *testing.T) {
	cleanupKeychain(t)
	configDir := t.TempDir()
	profilesPath := ProfilesPath(configDir)
	malformed := []byte(`{"version":1,"profiles":[`)
	if err := os.WriteFile(profilesPath, malformed, 0o600); err != nil {
		t.Fatalf("WriteFile(profiles) error = %v", err)
	}

	if _, err := LoadTokenDataForProfileReadOnly(configDir, ""); err == nil || !strings.Contains(err.Error(), "不会自动修复") {
		t.Fatalf("LoadTokenDataForProfileReadOnly() error = %v, want non-repairing parse error", err)
	}
	after, err := os.ReadFile(profilesPath)
	if err != nil {
		t.Fatalf("ReadFile(profiles after) error = %v", err)
	}
	if string(after) != string(malformed) {
		t.Fatal("malformed profiles.json was modified")
	}
	matches, err := filepath.Glob(profilesPath + ".corrupt-*")
	if err != nil {
		t.Fatalf("Glob(corrupt profiles) error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("read-only lookup quarantined profiles.json: %v", matches)
	}
}

func TestValidAccessTokenReadOnly(t *testing.T) {
	tests := []struct {
		name    string
		data    *TokenData
		want    string
		wantErr string
	}{
		{name: "missing data", wantErr: "未找到登录凭证"},
		{name: "blank token", data: &TokenData{AccessToken: "  "}, wantErr: "未找到登录凭证"},
		{name: "expired token", data: &TokenData{AccessToken: "expired", ExpiresAt: time.Now().Add(-time.Hour)}, wantErr: "登录凭证已过期"},
		{name: "valid token is trimmed", data: &TokenData{AccessToken: "  valid-token  ", ExpiresAt: time.Now().Add(time.Hour)}, want: "valid-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validAccessTokenReadOnly(tt.data)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validAccessTokenReadOnly() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validAccessTokenReadOnly() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("validAccessTokenReadOnly() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadTokenDataForProfileReadOnlyEditionHookContract(t *testing.T) {
	previous := edition.Get()
	t.Cleanup(func() { edition.Override(previous) })

	loadErr := errors.New("host token unavailable")
	tests := []struct {
		name      string
		profile   string
		loadToken func(string) ([]byte, error)
		wantToken string
		wantErr   string
	}{
		{
			name:    "profile selection is rejected",
			profile: "work",
			loadToken: func(string) ([]byte, error) {
				t.Fatal("LoadToken must not run when a profile selector is present")
				return nil, nil
			},
			wantErr: "不支持 --profile",
		},
		{
			name: "host load error is preserved",
			loadToken: func(string) ([]byte, error) {
				return nil, loadErr
			},
			wantErr: "host token unavailable",
		},
		{
			name: "malformed host token is rejected",
			loadToken: func(string) ([]byte, error) {
				return []byte(`{"accessToken":`), nil
			},
			wantErr: "无法解析登录凭证",
		},
		{
			name: "valid host token is loaded",
			loadToken: func(string) ([]byte, error) {
				return []byte(`{"access_token":"host-token"}`), nil
			},
			wantToken: "host-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edition.Override(&edition.Hooks{LoadToken: tt.loadToken})
			data, err := LoadTokenDataForProfileReadOnly(t.TempDir(), tt.profile)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadTokenDataForProfileReadOnly() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadTokenDataForProfileReadOnly() error = %v", err)
			}
			if data == nil || data.AccessToken != tt.wantToken {
				t.Fatalf("LoadTokenDataForProfileReadOnly() = %#v, want token %q", data, tt.wantToken)
			}
		})
	}
}

func TestLoadProfilesReadOnlyMissingAndUnreadable(t *testing.T) {
	missingDir := t.TempDir()
	profiles, err := loadProfilesReadOnly(missingDir)
	if err != nil {
		t.Fatalf("loadProfilesReadOnly(missing) error = %v", err)
	}
	if profiles.Version != 1 || len(profiles.Profiles) != 0 {
		t.Fatalf("loadProfilesReadOnly(missing) = %#v, want empty version 1 config", profiles)
	}

	unreadableDir := t.TempDir()
	if err := os.Mkdir(ProfilesPath(unreadableDir), 0o700); err != nil {
		t.Fatalf("Mkdir(profiles path) error = %v", err)
	}
	if _, err := loadProfilesReadOnly(unreadableDir); err == nil || !strings.Contains(err.Error(), "无法只读加载 profiles") {
		t.Fatalf("loadProfilesReadOnly(directory) error = %v, want read failure", err)
	}
}

func TestLoadTokenDataFromKeychainReadOnlySelectorFailures(t *testing.T) {
	cleanupKeychain(t)
	profiles := &ProfilesConfig{Profiles: []Profile{{Name: "broken", CorpID: ""}}}

	if _, err := loadTokenDataFromKeychainReadOnly(profiles, "missing"); err == nil || !strings.Contains(err.Error(), `profile "missing" not found`) {
		t.Fatalf("missing selector error = %v", err)
	}
	if _, err := loadTokenDataFromKeychainReadOnly(profiles, "broken"); err == nil || !strings.Contains(err.Error(), "corpId is required") {
		t.Fatalf("broken selector error = %v, want keychain load error", err)
	}
}

func TestLoadTokenDataFromKeychainReadOnlyResolutionOrder(t *testing.T) {
	t.Run("current missing then primary succeeds", func(t *testing.T) {
		cleanupKeychain(t)
		profiles := &ProfilesConfig{
			CurrentProfile: "current",
			PrimaryProfile: "primary",
			Profiles: []Profile{
				{Name: "current", CorpID: "corp_current"},
				{Name: "primary", CorpID: "corp_primary"},
			},
		}
		want := testToken("at_primary", "corp_primary", "Primary")
		if err := SaveTokenDataKeychainForCorpID(want.CorpID, want); err != nil {
			t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
		}
		got, err := loadTokenDataFromKeychainReadOnly(profiles, "")
		if err != nil {
			t.Fatalf("loadTokenDataFromKeychainReadOnly() error = %v", err)
		}
		if got.AccessToken != want.AccessToken {
			t.Fatalf("token = %#v, want primary token", got)
		}
	})

	t.Run("duplicate default profile falls back to matching legacy token", func(t *testing.T) {
		cleanupKeychain(t)
		profiles := &ProfilesConfig{
			CurrentProfile: "corp_current",
			PrimaryProfile: "corp_current",
			Profiles:       []Profile{{Name: "current", CorpID: "corp_current"}},
		}
		want := testToken("at_legacy", "corp_current", "Current")
		if err := SaveTokenDataKeychain(want); err != nil {
			t.Fatalf("SaveTokenDataKeychain() error = %v", err)
		}
		got, err := loadTokenDataFromKeychainReadOnly(profiles, "")
		if err != nil {
			t.Fatalf("loadTokenDataFromKeychainReadOnly() error = %v", err)
		}
		if got.AccessToken != want.AccessToken {
			t.Fatalf("token = %#v, want matching legacy token", got)
		}
	})

	t.Run("mismatched legacy organization is rejected", func(t *testing.T) {
		cleanupKeychain(t)
		profiles := &ProfilesConfig{
			CurrentProfile: "corp_current",
			Profiles:       []Profile{{Name: "current", CorpID: "corp_current"}},
		}
		if err := SaveTokenDataKeychain(testToken("at_other", "corp_other", "Other")); err != nil {
			t.Fatalf("SaveTokenDataKeychain() error = %v", err)
		}
		if _, err := loadTokenDataFromKeychainReadOnly(profiles, ""); err == nil || !strings.Contains(err.Error(), "不会切换到其他组织") {
			t.Fatalf("loadTokenDataFromKeychainReadOnly() error = %v, want organization mismatch", err)
		}
	})

	t.Run("invalid profile metadata fails closed", func(t *testing.T) {
		cleanupKeychain(t)
		profiles := &ProfilesConfig{
			CurrentProfile: "broken",
			Profiles:       []Profile{{Name: "broken", CorpID: ""}},
		}
		if _, err := loadTokenDataFromKeychainReadOnly(profiles, ""); err == nil || !strings.Contains(err.Error(), "corpId is required") {
			t.Fatalf("loadTokenDataFromKeychainReadOnly() error = %v, want invalid profile failure", err)
		}
	})

	t.Run("missing all token slots remains read only", func(t *testing.T) {
		cleanupKeychain(t)
		if _, err := LoadValidAccessTokenReadOnly(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "不会迁移旧凭证") {
			t.Fatalf("LoadValidAccessTokenReadOnly() error = %v, want missing credential guidance", err)
		}
	})
}

func TestReadOnlyTokenLoadErrorClassification(t *testing.T) {
	if err := readOnlyTokenLoadError(ErrTokenDataNotFound); err == nil || !strings.Contains(err.Error(), "不会迁移旧凭证") {
		t.Fatalf("readOnlyTokenLoadError(not found) = %v", err)
	}
	want := errors.New("keychain unavailable")
	if err := readOnlyTokenLoadError(want); err == nil || !errors.Is(err, want) {
		t.Fatalf("readOnlyTokenLoadError(unavailable) = %v, want wrapped source", err)
	}
}
