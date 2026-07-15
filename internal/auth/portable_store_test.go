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
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestPortableExportSupported(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if !PortableExportSupported() {
			t.Fatal("PortableExportSupported() should be true on non-darwin")
		}
		return
	}
	t.Setenv(keychain.DisableKeychainEnv, "")
	if PortableExportSupported() {
		t.Fatal("PortableExportSupported() should be false on darwin without file DEK")
	}
	t.Setenv(keychain.DisableKeychainEnv, "1")
	if !PortableExportSupported() {
		t.Fatal("PortableExportSupported() should be true when file DEK is enabled")
	}
}

func TestExportPortableAuthBundleRequiresAuthToken(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	keychainRoot := filepath.Join(t.TempDir(), "empty-keychain")
	if err := os.MkdirAll(filepath.Join(keychainRoot, keychain.Service), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Setenv(keychain.StorageDirEnv, keychainRoot)
	configDir := filepath.Join(t.TempDir(), ".dws")

	var bundle bytes.Buffer
	err := ExportPortableAuthBundle(configDir, &bundle)
	if err == nil {
		t.Fatal("ExportPortableAuthBundle() should fail without auth-token.enc")
	}
	if bundle.Len() != 0 {
		t.Fatalf("ExportPortableAuthBundle() wrote %d bytes, want 0", bundle.Len())
	}
}

func TestPortableAuthTargetPopulated(t *testing.T) {
	requirePortableFileBackend(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	root := t.TempDir()
	configDir := filepath.Join(root, ".dws")
	t.Setenv(keychain.StorageDirEnv, filepath.Join(root, "keychain"))

	if PortableAuthTargetPopulated(configDir) {
		t.Fatal("PortableAuthTargetPopulated() should be false before save")
	}
	if err := SaveTokenData(configDir, &TokenData{
		AccessToken:  "token",
		RefreshToken: "refresh",
		RefreshExpAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if !PortableAuthTargetPopulated(configDir) {
		t.Fatal("PortableAuthTargetPopulated() should be true after save")
	}
}

func TestPortableAuthBundleRoundTripPreservesRefreshToken(t *testing.T) {
	requirePortableFileBackend(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	sourceKeychain := filepath.Join(t.TempDir(), "source-keychain")
	t.Setenv(keychain.StorageDirEnv, sourceKeychain)
	sourceConfig := filepath.Join(t.TempDir(), ".dws")

	original := &TokenData{
		AccessToken:  "access-source",
		RefreshToken: "refresh-source",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(30 * 24 * time.Hour),
		CorpID:       "dingcorp",
		ClientID:     "client-from-mcp",
		Source:       "mcp",
	}
	if err := SaveTokenData(sourceConfig, original); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := SaveAppConfig(sourceConfig, &AppConfig{ClientID: "client-from-mcp"}); err != nil {
		t.Fatalf("SaveAppConfig() error = %v", err)
	}

	var bundle bytes.Buffer
	if err := ExportPortableAuthBundle(sourceConfig, &bundle); err != nil {
		t.Fatalf("ExportPortableAuthBundle() error = %v", err)
	}
	if bundle.Len() == 0 {
		t.Fatal("ExportPortableAuthBundle() wrote an empty bundle")
	}

	targetKeychain := filepath.Join(t.TempDir(), "target-keychain")
	t.Setenv(keychain.StorageDirEnv, targetKeychain)
	targetConfig := filepath.Join(t.TempDir(), ".dws")

	if _, err := ImportPortableAuthBundle(targetConfig, bytes.NewReader(bundle.Bytes())); err != nil {
		t.Fatalf("ImportPortableAuthBundle() error = %v", err)
	}

	loaded, err := LoadTokenData(targetConfig)
	if err != nil {
		t.Fatalf("LoadTokenData() after import error = %v", err)
	}
	if loaded.AccessToken != original.AccessToken {
		t.Fatalf("access token = %q, want %q", loaded.AccessToken, original.AccessToken)
	}
	if loaded.RefreshToken != original.RefreshToken {
		t.Fatalf("refresh token = %q, want %q", loaded.RefreshToken, original.RefreshToken)
	}
	if !loaded.IsRefreshTokenValid() {
		t.Fatal("refresh token should remain valid after import")
	}
	if cfg, err := LoadAppConfig(targetConfig); err != nil {
		t.Fatalf("LoadAppConfig() after import error = %v", err)
	} else if cfg == nil || cfg.ClientID != "client-from-mcp" {
		t.Fatalf("imported app config = %#v, want client ID preserved", cfg)
	}
}

func TestPortableAuthBundleRoundTripPreservesProfiles(t *testing.T) {
	requirePortableFileBackend(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	SetRuntimeProfile("")
	t.Cleanup(func() { SetRuntimeProfile("") })

	sourceKeychain := filepath.Join(t.TempDir(), "source-keychain")
	t.Setenv(keychain.StorageDirEnv, sourceKeychain)
	sourceConfig := filepath.Join(t.TempDir(), ".dws")

	tokenA := &TokenData{
		AccessToken:  "access-a",
		RefreshToken: "refresh-a",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(30 * 24 * time.Hour),
		CorpID:       "corp_a",
		CorpName:     "A Org",
		ClientID:     "client-a",
	}
	tokenB := &TokenData{
		AccessToken:  "access-b",
		RefreshToken: "refresh-b",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(30 * 24 * time.Hour),
		CorpID:       "corp_b",
		CorpName:     "B Org",
		ClientID:     "client-b",
	}
	if err := SaveTokenData(sourceConfig, tokenA); err != nil {
		t.Fatalf("SaveTokenData(A) error = %v", err)
	}
	if err := SaveTokenData(sourceConfig, tokenB); err != nil {
		t.Fatalf("SaveTokenData(B) error = %v", err)
	}

	var bundle bytes.Buffer
	if err := ExportPortableAuthBundle(sourceConfig, &bundle); err != nil {
		t.Fatalf("ExportPortableAuthBundle() error = %v", err)
	}

	targetKeychain := filepath.Join(t.TempDir(), "target-keychain")
	t.Setenv(keychain.StorageDirEnv, targetKeychain)
	targetConfig := filepath.Join(t.TempDir(), ".dws")
	if _, err := ImportPortableAuthBundle(targetConfig, bytes.NewReader(bundle.Bytes())); err != nil {
		t.Fatalf("ImportPortableAuthBundle() error = %v", err)
	}

	cfg, err := LoadProfiles(targetConfig)
	if err != nil {
		t.Fatalf("LoadProfiles() after import error = %v", err)
	}
	if cfg.PrimaryProfile != "corp_a" || cfg.CurrentProfile != "corp_b" || cfg.PreviousProfile != "corp_a" {
		t.Fatalf("profiles after import = %#v", cfg)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2: %#v", len(cfg.Profiles), cfg.Profiles)
	}

	loadedA, err := LoadTokenDataForProfile(targetConfig, "corp_a")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(A) after import error = %v", err)
	}
	if loadedA.AccessToken != "access-a" {
		t.Fatalf("profile A token = %q, want access-a", loadedA.AccessToken)
	}
	loadedB, err := LoadTokenDataForProfile(targetConfig, "corp_b")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(B) after import error = %v", err)
	}
	if loadedB.AccessToken != "access-b" {
		t.Fatalf("profile B token = %q, want access-b", loadedB.AccessToken)
	}
}

func requirePortableFileBackend(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("portable bundle round trips require a file-DEK backend; Windows uses DPAPI registry storage")
	}
}
