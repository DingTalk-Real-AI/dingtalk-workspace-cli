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
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// cleanupKeychain removes test data from keychain after test completes.
func cleanupKeychain(t *testing.T, configDirs ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, configDir := range configDirs {
			_ = DeleteTokenData(configDir)
		}
		_ = keychain.Remove(keychain.Service, keychain.AccountToken)
	})
}

func TestTokenSaveLoadAndDelete(t *testing.T) {
	configDir := t.TempDir()
	cleanupKeychain(t, configDir)
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
	if !TokenDataExistsKeychain(configDir) {
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
	if TokenDataExistsKeychain(configDir) {
		t.Fatal("TokenDataExistsKeychain() should be false after delete")
	}
	if _, err := LoadTokenData(configDir); err == nil {
		t.Fatal("LoadTokenData() error = nil after delete, want failure")
	}
}

func TestTokenOverwrite(t *testing.T) {
	configDir := t.TempDir()
	cleanupKeychain(t, configDir)

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

func TestTokenDataExistsKeychain(t *testing.T) {
	configDir := t.TempDir()
	cleanupKeychain(t, configDir)

	// Should be false before save
	if TokenDataExistsKeychain(configDir) {
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
	if !TokenDataExistsKeychain(configDir) {
		t.Fatal("TokenDataExistsKeychain() should be true after save")
	}
}

func TestTokenDataIsScopedByConfigDir(t *testing.T) {
	configDirA := t.TempDir()
	configDirB := t.TempDir()
	cleanupKeychain(t, configDirA, configDirB)

	if err := SaveTokenData(configDirA, &TokenData{
		AccessToken: "token-a",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenData(configDirA) error = %v", err)
	}
	if err := SaveTokenData(configDirB, &TokenData{
		AccessToken: "token-b",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenData(configDirB) error = %v", err)
	}

	loadedA, err := LoadTokenData(configDirA)
	if err != nil {
		t.Fatalf("LoadTokenData(configDirA) error = %v", err)
	}
	if loadedA.AccessToken != "token-a" {
		t.Fatalf("LoadTokenData(configDirA) access_token = %q, want token-a", loadedA.AccessToken)
	}

	loadedB, err := LoadTokenData(configDirB)
	if err != nil {
		t.Fatalf("LoadTokenData(configDirB) error = %v", err)
	}
	if loadedB.AccessToken != "token-b" {
		t.Fatalf("LoadTokenData(configDirB) access_token = %q, want token-b", loadedB.AccessToken)
	}
}

func TestDeleteTokenDataOnlyRemovesRequestedConfigDir(t *testing.T) {
	configDirA := t.TempDir()
	configDirB := t.TempDir()
	cleanupKeychain(t, configDirA, configDirB)

	if err := SaveTokenData(configDirA, &TokenData{
		AccessToken: "token-a",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenData(configDirA) error = %v", err)
	}
	if err := SaveTokenData(configDirB, &TokenData{
		AccessToken: "token-b",
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenData(configDirB) error = %v", err)
	}

	if err := DeleteTokenData(configDirA); err != nil {
		t.Fatalf("DeleteTokenData(configDirA) error = %v", err)
	}
	if _, err := LoadTokenData(configDirA); err == nil {
		t.Fatal("LoadTokenData(configDirA) error = nil after delete, want failure")
	}

	loadedB, err := LoadTokenData(configDirB)
	if err != nil {
		t.Fatalf("LoadTokenData(configDirB) error = %v", err)
	}
	if loadedB.AccessToken != "token-b" {
		t.Fatalf("LoadTokenData(configDirB) access_token = %q, want token-b", loadedB.AccessToken)
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
