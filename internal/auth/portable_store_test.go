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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestCrossPlatformCoveragePortableExportSupported(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		if PortableExportSupported() {
			t.Fatal("PortableExportSupported() should be false for the Windows DPAPI backend")
		}
		var bundle bytes.Buffer
		err := ExportPortableAuthBundle(t.TempDir(), &bundle)
		if err == nil || !strings.Contains(err.Error(), "DPAPI") {
			t.Fatalf("ExportPortableAuthBundle() error = %v, want Windows DPAPI unsupported error", err)
		}
		if bundle.Len() != 0 {
			t.Fatalf("ExportPortableAuthBundle() wrote %d bytes on unsupported Windows backend", bundle.Len())
		}
		return
	case "darwin":
		t.Setenv(keychain.DisableKeychainEnv, "")
		if PortableExportSupported() {
			t.Fatal("PortableExportSupported() should be false on darwin without file DEK")
		}
		t.Setenv(keychain.DisableKeychainEnv, "1")
		if !PortableExportSupported() {
			t.Fatal("PortableExportSupported() should be true when file DEK is enabled")
		}
		return
	default:
		if !PortableExportSupported() {
			t.Fatal("PortableExportSupported() should be true for file-DEK platforms")
		}
	}
}

func TestCrossPlatformCoveragePortableExportSupportError(t *testing.T) {
	tests := []struct {
		name            string
		goos            string
		disableKeychain string
		wantError       string
	}{
		{name: "windows dpapi", goos: "windows", disableKeychain: "1", wantError: "DPAPI"},
		{name: "darwin keychain", goos: "darwin", wantError: keychain.DisableKeychainEnv},
		{name: "darwin file dek", goos: "darwin", disableKeychain: "1"},
		{name: "linux file dek", goos: "linux"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := portableExportSupportError(test.goos, test.disableKeychain)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("portableExportSupportError() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("portableExportSupportError() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestCrossPlatformCoveragePortableImportSupportError(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		wantError string
	}{
		{name: "windows dpapi", goos: "windows", wantError: "DPAPI"},
		{name: "darwin", goos: "darwin"},
		{name: "linux", goos: "linux"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := portableImportSupportError(test.goos)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("portableImportSupportError() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("portableImportSupportError() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestCrossPlatformCoverageImportPortableAuthBundleRejectsWindowsDPAPIWithoutMutation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows DPAPI contract requires a native Windows runner")
	}

	root := t.TempDir()
	configDir := filepath.Join(root, ".dws")
	keychainDir := filepath.Join(root, "keychain")
	t.Setenv(keychain.StorageDirEnv, keychainDir)
	bundle := validPortableAuthBundleForTest(t)

	if _, err := ImportPortableAuthBundle(configDir, bytes.NewReader(bundle)); err == nil || !strings.Contains(err.Error(), "DPAPI") {
		t.Fatalf("ImportPortableAuthBundle() error = %v, want Windows DPAPI unsupported error", err)
	}
	for _, path := range []string{configDir, keychainDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unsupported Windows import touched %s: stat error = %v", path, err)
		}
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

func validPortableAuthBundleForTest(t *testing.T) []byte {
	t.Helper()

	var bundle bytes.Buffer
	gz := gzip.NewWriter(&bundle)
	tw := tar.NewWriter(gz)
	manifest := portableAuthBundleManifest{
		Version:         1,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		OS:              "linux",
		KeychainService: keychain.Service,
		ConfigFiles:     []string{"app.json"},
	}
	if err := writePortableManifest(tw, manifest); err != nil {
		t.Fatalf("writePortableManifest() error = %v", err)
	}
	entries := map[string]string{
		path.Join("keychain", keychain.Service, keychain.AccountToken+".enc"): "encrypted-token",
		"config/app.json": "{}",
	}
	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s) error = %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close() error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip.Close() error = %v", err)
	}
	return bundle.Bytes()
}
