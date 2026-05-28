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

package app

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestAuthExportImportBase64RoundTrip(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	sourceKeychain := filepath.Join(t.TempDir(), "source-keychain")
	sourceConfig := filepath.Join(t.TempDir(), ".dws")
	t.Setenv(keychain.StorageDirEnv, sourceKeychain)
	t.Setenv("DWS_CONFIG_DIR", sourceConfig)

	original := &authpkg.TokenData{
		AccessToken:  "access-cli",
		RefreshToken: "refresh-cli",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		ClientID:     "client-cli",
		Source:       "mcp",
	}
	if err := authpkg.SaveTokenData(sourceConfig, original); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	exportCmd := NewRootCommand()
	var exported bytes.Buffer
	exportCmd.SetOut(&exported)
	exportCmd.SetErr(&bytes.Buffer{})
	exportCmd.SetArgs([]string{"auth", "export", "--base64"})
	if err := exportCmd.Execute(); err != nil {
		t.Fatalf("auth export --base64 error = %v", err)
	}
	if strings.TrimSpace(exported.String()) == "" {
		t.Fatal("auth export --base64 produced empty output")
	}

	targetRoot := t.TempDir()
	inputPath := filepath.Join(targetRoot, "dws-auth.b64")
	if err := os.WriteFile(inputPath, []byte(exported.String()), 0o600); err != nil {
		t.Fatalf("write input bundle error = %v", err)
	}

	targetKeychain := filepath.Join(targetRoot, "target-keychain")
	targetConfig := filepath.Join(targetRoot, ".dws")
	t.Setenv(keychain.StorageDirEnv, targetKeychain)
	t.Setenv("DWS_CONFIG_DIR", targetConfig)

	importCmd := NewRootCommand()
	importCmd.SetOut(&bytes.Buffer{})
	importCmd.SetErr(&bytes.Buffer{})
	importCmd.SetArgs([]string{"auth", "import", "--input", inputPath, "--base64"})
	if err := importCmd.Execute(); err != nil {
		t.Fatalf("auth import --base64 error = %v", err)
	}

	loaded, err := authpkg.LoadTokenData(targetConfig)
	if err != nil {
		t.Fatalf("LoadTokenData() after CLI import error = %v", err)
	}
	if loaded.RefreshToken != original.RefreshToken {
		t.Fatalf("refresh token = %q, want %q", loaded.RefreshToken, original.RefreshToken)
	}
	if !loaded.IsRefreshTokenValid() {
		t.Fatal("refresh token should remain valid after CLI import")
	}
}

func TestAuthImportRequiresForceWhenPopulated(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	root := t.TempDir()
	configDir := filepath.Join(root, ".dws")
	t.Setenv(keychain.StorageDirEnv, filepath.Join(root, "keychain"))
	t.Setenv("DWS_CONFIG_DIR", configDir)

	if err := authpkg.SaveTokenData(configDir, &authpkg.TokenData{
		AccessToken:  "existing",
		RefreshToken: "existing-refresh",
		RefreshExpAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	bundlePath := filepath.Join(root, "bundle.tar.gz")
	if err := os.WriteFile(bundlePath, []byte("not-a-real-bundle"), 0o600); err != nil {
		t.Fatalf("write bundle stub error = %v", err)
	}

	importCmd := NewRootCommand()
	var stderr bytes.Buffer
	importCmd.SetOut(&bytes.Buffer{})
	importCmd.SetErr(&stderr)
	importCmd.SetArgs([]string{"auth", "import", "--input", bundlePath})
	err := importCmd.Execute()
	if err == nil {
		t.Fatal("auth import without --force should fail when auth exists")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Category != apperrors.CategoryValidation {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %v, want --force hint", err)
	}
}

func TestAuthStatusRefreshFailureLeavesStoredTokenIntact(t *testing.T) {
	// Isolate keychain storage to a per-test directory so the saved
	// token can't leak into other test packages running in parallel.
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	t.Cleanup(func() {
		_ = keychain.Remove(keychain.Service, keychain.AccountToken)
	})

	root := t.TempDir()
	configDir := filepath.Join(root, "config")

	t.Setenv("DWS_CONFIG_DIR", configDir)

	err := authpkg.SaveTokenData(configDir, &authpkg.TokenData{
		AccessToken:  "expired-access",
		RefreshToken: "refresh-123",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "dingcorp",
	})
	if err != nil {
		t.Skipf("SaveTokenData() unavailable in this environment: %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("refresh failed")
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	// Verify token data still exists in keychain after refresh failure
	if !authpkg.TokenDataExistsKeychain() {
		t.Fatal("secure token data should remain in keychain after refresh failure")
	}

	if !bytes.Contains(out.Bytes(), []byte("\"authenticated\"")) {
		t.Fatalf("output should still report authenticated status:\n%s", out.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
