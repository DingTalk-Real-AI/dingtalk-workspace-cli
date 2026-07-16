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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestAuthExportImportBase64RoundTrip(t *testing.T) {
	originalSupported := authPortableExportSupported
	originalReady := authPortableSourceReady
	originalExport := authExportPortableBundle
	originalTarget := authPortableTargetPopulated
	originalImport := authImportPortableBundle
	t.Cleanup(func() {
		authPortableExportSupported = originalSupported
		authPortableSourceReady = originalReady
		authExportPortableBundle = originalExport
		authPortableTargetPopulated = originalTarget
		authImportPortableBundle = originalImport
	})

	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	bundle := []byte("portable-auth-bundle")
	authPortableExportSupported = func() bool { return true }
	authPortableSourceReady = func() bool { return true }
	authExportPortableBundle = func(_ string, w io.Writer) error {
		_, err := w.Write(bundle)
		return err
	}

	exportCmd := newAuthExportCommand()
	var exported bytes.Buffer
	exportCmd.SetOut(&exported)
	exportCmd.SetErr(&bytes.Buffer{})
	exportCmd.SetArgs([]string{"--base64"})
	if err := exportCmd.Execute(); err != nil {
		t.Fatalf("auth export --base64 error = %v", err)
	}
	if strings.TrimSpace(exported.String()) == "" {
		t.Fatal("auth export --base64 produced empty output")
	}

	targetRoot := t.TempDir()
	inputPath := filepath.Join(targetRoot, "dws-auth.b64")
	if err := os.WriteFile(inputPath, exported.Bytes(), 0o600); err != nil {
		t.Fatalf("write input bundle error = %v", err)
	}

	authPortableTargetPopulated = func(string) bool { return false }
	var imported []byte
	authImportPortableBundle = func(_ string, r io.Reader) (authpkg.PortableImportReport, error) {
		var err error
		imported, err = io.ReadAll(r)
		return authpkg.PortableImportReport{}, err
	}

	importCmd := newAuthImportCommand()
	importCmd.SetOut(&bytes.Buffer{})
	importCmd.SetErr(&bytes.Buffer{})
	importCmd.SetArgs([]string{"--input", inputPath, "--base64"})
	if err := importCmd.Execute(); err != nil {
		t.Fatalf("auth import --base64 error = %v", err)
	}
	if !bytes.Equal(imported, bundle) {
		t.Fatalf("imported bundle = %q, want %q", imported, bundle)
	}
}

func TestAuthImportRequiresForceWhenPopulated(t *testing.T) {
	originalTarget := authPortableTargetPopulated
	authPortableTargetPopulated = func(string) bool { return true }
	t.Cleanup(func() { authPortableTargetPopulated = originalTarget })

	root := t.TempDir()
	configDir := filepath.Join(root, ".dws")
	t.Setenv("DWS_CONFIG_DIR", configDir)

	bundlePath := filepath.Join(root, "bundle.tar.gz")
	if err := os.WriteFile(bundlePath, []byte("not-a-real-bundle"), 0o600); err != nil {
		t.Fatalf("write bundle stub error = %v", err)
	}

	importCmd := newAuthImportCommand()
	var stderr bytes.Buffer
	importCmd.SetOut(&bytes.Buffer{})
	importCmd.SetErr(&stderr)
	importCmd.SetArgs([]string{"--input", bundlePath})
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

func TestAuthStatusJSONReportsKeychainUnavailable(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))

	prev := edition.Get()
	edition.Override(&edition.Hooks{
		LoadToken: func(configDir string) ([]byte, error) {
			return nil, keychain.NewUnavailableError("read DEK from macOS Keychain", errors.New("default keychain missing"))
		},
	})
	t.Cleanup(func() {
		edition.Override(prev)
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "json", "auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status --format json error = %v\noutput:\n%s", err, out.String())
	}

	var resp struct {
		Success       bool   `json:"success"`
		Authenticated bool   `json:"authenticated"`
		Reason        string `json:"reason"`
		Message       string `json:"message"`
		Hint          string `json:"hint"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal auth status JSON error = %v\noutput:\n%s", err, out.String())
	}
	if !resp.Success {
		t.Fatalf("success = false, want true; response=%+v", resp)
	}
	if resp.Authenticated {
		t.Fatalf("authenticated = true, want false; response=%+v", resp)
	}
	if resp.Reason != "keychain_unavailable" {
		t.Fatalf("reason = %q, want keychain_unavailable; response=%+v", resp.Reason, resp)
	}
	if !strings.Contains(resp.Message, "Keychain") && !strings.Contains(resp.Message, "钥匙串") {
		t.Fatalf("message should mention Keychain/钥匙串; response=%+v", resp)
	}
	if !strings.Contains(resp.Hint, keychain.DisableKeychainEnv) {
		t.Fatalf("hint should mention %s; response=%+v", keychain.DisableKeychainEnv, resp)
	}
}

func TestAuthStatusJSONReportsDEKMissing(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))

	prev := edition.Get()
	edition.Override(&edition.Hooks{
		LoadToken: func(configDir string) ([]byte, error) {
			return nil, fmt.Errorf("load from keychain: %w", keychain.ErrDEKMissing)
		},
	})
	t.Cleanup(func() {
		edition.Override(prev)
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "json", "auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status --format json error = %v\noutput:\n%s", err, out.String())
	}

	var resp struct {
		Success       bool   `json:"success"`
		Authenticated bool   `json:"authenticated"`
		Reason        string `json:"reason"`
		Message       string `json:"message"`
		Hint          string `json:"hint"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal auth status JSON error = %v\noutput:\n%s", err, out.String())
	}
	if !resp.Success {
		t.Fatalf("success = false, want true; response=%+v", resp)
	}
	if resp.Authenticated {
		t.Fatalf("authenticated = true, want false; response=%+v", resp)
	}
	if resp.Reason != "dek_missing" {
		t.Fatalf("reason = %q, want dek_missing; response=%+v", resp.Reason, resp)
	}
	if !strings.Contains(resp.Message, "登录密钥") {
		t.Fatalf("message should mention 登录密钥; response=%+v", resp)
	}
	if !strings.Contains(resp.Hint, "重新登录") {
		t.Fatalf("hint should mention 重新登录; response=%+v", resp)
	}
	if !strings.Contains(resp.Hint, "dws auth reset") {
		t.Fatalf("hint should mention dws auth reset; response=%+v", resp)
	}
}

func TestAuthStatusDiagnosticReportsCiphertextKeyMismatch(t *testing.T) {
	diagnostic := authStatusDiagnosticFromError(fmt.Errorf("load token: %w", keychain.ErrCiphertextKeyMismatch))
	if diagnostic == nil {
		t.Fatal("authStatusDiagnosticFromError() = nil")
	}
	if diagnostic.Reason != "ciphertext_key_mismatch" {
		t.Fatalf("reason = %q, want ciphertext_key_mismatch", diagnostic.Reason)
	}
	if !strings.Contains(diagnostic.Hint, keychain.DisableKeychainEnv) {
		t.Fatalf("hint should mention %s: %q", keychain.DisableKeychainEnv, diagnostic.Hint)
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

func TestAuthStatusTableIncludesCorpName(t *testing.T) {
	setupAuthLogoutProfiles(t, authLogoutTestToken("corp_primary"))

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "auth", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status --format table error = %v\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{"企业:", "corp_primary org", "企业 ID:", "corp_primary"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("auth status table missing %q in output:\n%s", want, out.String())
		}
	}
}

func TestAuthStatusProfileOverrideDoesNotSwitchCurrentProfile(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "table", "auth", "status", "--profile", "corp_primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status --profile error = %v\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{"corp_primary org", "corp_primary"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("auth status --profile output missing %q:\n%s", want, out.String())
		}
	}
	if bytes.Contains(out.Bytes(), []byte("corp_secondary org")) {
		t.Fatalf("auth status --profile should render selected profile, got:\n%s", out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.CurrentProfile != "corp_secondary" {
		t.Fatalf("currentProfile = %q, want unchanged corp_secondary", cfg.CurrentProfile)
	}
}

func TestAuthMigrateKeychainDryRunAndConfirmedExecution(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "")
	oldMigrate := migrateKeychainToFileDEK
	t.Cleanup(func() { migrateKeychainToFileDEK = oldMigrate })

	calls := 0
	migrateKeychainToFileDEK = func(_ string, dryRun bool) (int, error) {
		calls++
		if calls == 1 && !dryRun {
			t.Fatal("first migration call should be dry-run")
		}
		if calls == 2 && dryRun {
			t.Fatal("second migration call should execute")
		}
		return 4, nil
	}

	newRoot := func() (*cobra.Command, *bytes.Buffer) {
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().Bool("dry-run", false, "")
		root.PersistentFlags().Bool("yes", false, "")
		root.PersistentFlags().String("format", "json", "")
		root.AddCommand(newAuthMigrateKeychainCommand())
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		return root, &out
	}

	root, out := newRoot()
	root.SetArgs([]string{"migrate-keychain", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("migrate-keychain --dry-run error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"dry_run":true`) || !strings.Contains(out.String(), `"entries":4`) {
		t.Fatalf("dry-run output = %q", out.String())
	}

	root, out = newRoot()
	root.SetArgs([]string{"migrate-keychain", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("migrate-keychain --yes error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"dry_run":false`) || !strings.Contains(out.String(), `"entries":4`) {
		t.Fatalf("migration output = %q", out.String())
	}
	if calls != 2 {
		t.Fatalf("migration calls = %d, want 2", calls)
	}
}

func TestAuthMigrateKeychainRequiresConfirmationAndSystemMode(t *testing.T) {
	oldMigrate := migrateKeychainToFileDEK
	t.Cleanup(func() { migrateKeychainToFileDEK = oldMigrate })
	migrateKeychainToFileDEK = func(_ string, _ bool) (int, error) {
		t.Fatal("migration backend should not be called")
		return 0, nil
	}

	newRoot := func() *cobra.Command {
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().Bool("dry-run", false, "")
		root.PersistentFlags().Bool("yes", false, "")
		root.PersistentFlags().String("format", "json", "")
		root.AddCommand(newAuthMigrateKeychainCommand())
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		return root
	}

	t.Setenv(keychain.DisableKeychainEnv, "")
	root := newRoot()
	root.SetArgs([]string{"migrate-keychain"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("unconfirmed migration error = %v, want --yes guidance", err)
	}

	t.Setenv(keychain.DisableKeychainEnv, "1")
	root = newRoot()
	root.SetArgs([]string{"migrate-keychain", "--dry-run"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "env -u") {
		t.Fatalf("file-DEK mode migration error = %v, want system-mode guidance", err)
	}
}

func TestAuthLogoutDefaultDeletesAllProfilesAndPreservesAppConfig(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)
	if err := authpkg.SaveAppConfig(configDir, &authpkg.AppConfig{
		ClientID:     "client-app",
		ClientSecret: authpkg.PlainSecret("secret-app"),
	}); err != nil {
		t.Fatalf("SaveAppConfig() error = %v", err)
	}

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("remote revoke disabled in unit test")
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "logout"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth logout error = %v\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{"[OK] 已清除认证信息", "重新登录"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("auth logout output missing %q:\n%s", want, out.String())
		}
	}

	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.PrimaryProfile != "" || cfg.CurrentProfile != "" || cfg.PreviousProfile != "" || len(cfg.Profiles) != 0 {
		t.Fatalf("profiles after logout = %#v, want empty", cfg)
	}
	if authpkg.TokenDataExistsKeychainForCorpID("corp_primary") {
		t.Fatal("primary profile token should be deleted")
	}
	if authpkg.TokenDataExistsKeychainForCorpID("corp_secondary") {
		t.Fatal("secondary profile token should be deleted")
	}
	if authpkg.TokenDataExistsKeychain() {
		t.Fatal("legacy auth-token mirror should be deleted")
	}
	appConfig, err := authpkg.LoadAppConfig(configDir)
	if err != nil {
		t.Fatalf("LoadAppConfig() error = %v", err)
	}
	if appConfig == nil || appConfig.ClientID != "client-app" {
		t.Fatalf("app config after logout = %#v, want preserved client-app", appConfig)
	}
}

func TestAuthLogoutProfileDeletesOnlySelectedProfile(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_primary"),
		authLogoutTestToken("corp_secondary"),
	)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("remote revoke disabled in unit test")
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "logout", "--profile", "corp_primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth logout --profile corp_primary error = %v\noutput:\n%s", err, out.String())
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	if cfg.PrimaryProfile != "corp_secondary" || cfg.CurrentProfile != "corp_secondary" {
		t.Fatalf("profiles pointers = primary %q current %q, want corp_secondary/corp_secondary", cfg.PrimaryProfile, cfg.CurrentProfile)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].CorpID != "corp_secondary" {
		t.Fatalf("profiles = %#v, want only corp_secondary retained", cfg.Profiles)
	}
	if authpkg.TokenDataExistsKeychainForCorpID("corp_primary") {
		t.Fatal("selected primary profile token should be deleted")
	}
	if !authpkg.TokenDataExistsKeychainForCorpID("corp_secondary") {
		t.Fatal("unselected secondary profile token should be retained")
	}
	loaded, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.CorpID != "corp_secondary" || loaded.AccessToken != "access-corp_secondary" {
		t.Fatalf("default token = (%q, %q), want retained secondary token", loaded.CorpID, loaded.AccessToken)
	}
}

func TestAuthLoginPostLoginTUIModeRespectsRecommendAndFormat(t *testing.T) {
	newRoot := func(t *testing.T) *cobra.Command {
		t.Helper()
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().String("format", "json", "")
		return root
	}

	t.Run("recommend skips tui but keeps human auth for interactive login", func(t *testing.T) {
		root := newRoot(t)
		if authLoginShouldShowPostLoginTUIForTerminal(root, "json", true, true) {
			t.Fatal("--recommend must not show the post-login product TUI")
		}
		if !authLoginShouldUseHumanAuthorizationModeForTerminal(root, "json", true, true) {
			t.Fatal("default interactive --recommend should still use human authorization flow")
		}
	})

	t.Run("without recommend shows two-step authorization tui", func(t *testing.T) {
		root := newRoot(t)
		if !authLoginShouldShowPostLoginTUIForTerminal(root, "json", false, true) {
			t.Fatal("default interactive login should show post-login authorization TUI")
		}
		if !authLoginShouldUseHumanAuthorizationModeForTerminal(root, "json", true, true) {
			t.Fatal("default interactive post-login authorization should use human authorization flow")
		}
	})

	t.Run("explicit json keeps machine mode", func(t *testing.T) {
		root := newRoot(t)
		if err := root.PersistentFlags().Set("format", "json"); err != nil {
			t.Fatalf("set format: %v", err)
		}
		if authLoginShouldShowPostLoginTUIForTerminal(root, "json", false, true) {
			t.Fatal("explicit --format json must not show post-login TUI")
		}
		if authLoginShouldUseHumanAuthorizationModeForTerminal(root, "json", true, true) {
			t.Fatal("explicit --format json must keep machine-readable authorization flow")
		}
	})

	t.Run("table without recommend shows authorization tui", func(t *testing.T) {
		root := newRoot(t)
		if err := root.PersistentFlags().Set("format", "table"); err != nil {
			t.Fatalf("set format: %v", err)
		}
		if !authLoginShouldShowPostLoginTUIForTerminal(root, "table", false, true) {
			t.Fatal("table format should show post-login TUI without --recommend")
		}
		if !authLoginShouldUseHumanAuthorizationModeForTerminal(root, "table", true, true) {
			t.Fatal("table format should use human authorization flow in an interactive terminal")
		}
	})

	t.Run("non interactive skips selector", func(t *testing.T) {
		root := newRoot(t)
		if authLoginShouldShowPostLoginTUIForTerminal(root, "json", false, false) {
			t.Fatal("non-interactive login should skip post-login TUI")
		}
		if authLoginShouldUseHumanAuthorizationModeForTerminal(root, "json", true, false) {
			t.Fatal("non-interactive login should keep machine-readable authorization flow")
		}
	})

	t.Run("without authorization flow keeps normal login output contract", func(t *testing.T) {
		root := newRoot(t)
		if authLoginShouldUseHumanAuthorizationModeForTerminal(root, "json", false, true) {
			t.Fatal("login without a post-login authorization flow should not switch default json to human mode")
		}
	})
}

func TestLoginRecommendProductLabelMatchesTUITarget(t *testing.T) {
	label := loginRecommendProductLabel(pat.LoginRecommendProduct{
		ProductCode: "approval",
		ProductName: "审批",
		Summary:     "审批实例，审批模板，审批任务管理",
		ScopeCount:  12,
	})
	if label != "approval   审批 - 审批实例，审批模板，审批任务管理" {
		t.Fatalf("label = %q", label)
	}
}

func TestResolveAuthLoginConfigReadsInheritedYes(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("yes", false, "")
	login := &cobra.Command{Use: "login"}
	login.Flags().String("token", "", "")
	login.Flags().Bool("device", false, "")
	login.Flags().Bool("force", false, "")
	login.Flags().Bool("recommend", false, "")
	root.AddCommand(login)

	if err := root.PersistentFlags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	if err := login.Flags().Set("recommend", "true"); err != nil {
		t.Fatalf("set recommend: %v", err)
	}

	cfg, err := resolveAuthLoginConfig(login)
	if err != nil {
		t.Fatalf("resolveAuthLoginConfig error = %v", err)
	}
	if !cfg.Recommend {
		t.Fatal("Recommend = false, want true")
	}
	if !cfg.Yes {
		t.Fatal("Yes = false, want true")
	}
}

func TestAuthLoginForcesAuthorizationByDefault(t *testing.T) {
	if !authLoginForcesAuthorization(authLoginConfig{}) {
		t.Fatal("auth login should force authorization by default so each login can add an organization profile")
	}
	if !authLoginForcesAuthorization(authLoginConfig{Force: false}) {
		t.Fatal("Force=false should still force authorization")
	}
}

func TestAuthLoginRecommendSkipsPostLoginTUI(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	oldGuideSelector := authLoginGuideActionSelector
	oldGuideApplier := authLoginGuideActionApplier
	oldScopeSelector := loginRecommendScopeModeSelector
	oldProductSelector := loginRecommendProductSelector
	oldInteractiveTerminal := authLoginInteractiveTerminal
	t.Cleanup(func() {
		authLoginGuideActionSelector = oldGuideSelector
		authLoginGuideActionApplier = oldGuideApplier
		loginRecommendScopeModeSelector = oldScopeSelector
		loginRecommendProductSelector = oldProductSelector
		authLoginInteractiveTerminal = oldInteractiveTerminal
	})
	authLoginInteractiveTerminal = func() bool { return true }
	authLoginGuideActionSelector = func() (authLoginGuideAction, error) {
		t.Fatal("--recommend must not call the post-login guide selector")
		return "", nil
	}
	authLoginGuideActionApplier = func(*cobra.Command, string, authLoginGuideAction) error {
		t.Fatal("--recommend must not apply a post-login guide action")
		return nil
	}
	loginRecommendScopeModeSelector = func() (pat.LoginRecommendScopeMode, error) {
		t.Fatal("--recommend must not call the scope-mode TUI")
		return "", nil
	}
	loginRecommendProductSelector = func([]pat.LoginRecommendProduct) ([]string, error) {
		t.Fatal("--recommend must not call the product-domain TUI")
		return nil, nil
	}

	fake := &authLoginRecommendSequenceCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["calendar.event:read"]}}`,
	}}
	cmd := newAuthLoginCommand(fake)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--token", "login-token", "--recommend"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login --recommend error = %v\noutput:\n%s", err, out.String())
	}
	if len(fake.tools) != 2 {
		t.Fatalf("CallTool count = %d, want plan + grant", len(fake.tools))
	}
	if fake.tools[0] != "pat.batch_plan" || fake.tools[1] != "pat.batch_grant" {
		t.Fatalf("tool sequence = %v, want plan, grant", fake.tools)
	}
	if got := fake.args[0]["recommend"]; got != true {
		t.Fatalf("--recommend plan recommend = %#v, want true", got)
	}
}

func TestAuthLoginDefaultTUIModeSkipsSelectorWhenAllGranted(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	oldGuideSelector := authLoginGuideActionSelector
	oldGuideApplier := authLoginGuideActionApplier
	oldScopeSelector := loginRecommendScopeModeSelector
	oldProductSelector := loginRecommendProductSelector
	oldInteractiveTerminal := authLoginInteractiveTerminal
	t.Cleanup(func() {
		authLoginGuideActionSelector = oldGuideSelector
		authLoginGuideActionApplier = oldGuideApplier
		loginRecommendScopeModeSelector = oldScopeSelector
		loginRecommendProductSelector = oldProductSelector
		authLoginInteractiveTerminal = oldInteractiveTerminal
	})
	authLoginInteractiveTerminal = func() bool { return true }
	authLoginGuideActionSelector = func() (authLoginGuideAction, error) {
		t.Fatal("default auth login must not call the operation guide selector")
		return "", nil
	}
	authLoginGuideActionApplier = func(*cobra.Command, string, authLoginGuideAction) error {
		t.Fatal("default auth login must not apply a post-login guide action")
		return nil
	}
	loginRecommendScopeModeSelector = func() (pat.LoginRecommendScopeMode, error) {
		t.Fatal("all-granted recommend plan must not call the scope-mode TUI")
		return "", nil
	}
	loginRecommendProductSelector = func([]pat.LoginRecommendProduct) ([]string, error) {
		t.Fatal("all-granted recommend plan must not call the product-domain TUI")
		return nil, nil
	}

	fake := &authLoginRecommendSequenceCaller{responses: []string{
		`{"success":true,"data":{"allGranted":true,"selectedScopes":[]}}`,
	}}
	cmd := newAuthLoginCommand(fake)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--token", "login-token"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login error = %v\noutput:\n%s", err, out.String())
	}
	if len(fake.tools) != 1 {
		t.Fatalf("CallTool count = %d, want only preflight plan", len(fake.tools))
	}
	if fake.tools[0] != "pat.batch_plan" {
		t.Fatalf("tool sequence = %v, want only plan", fake.tools)
	}
	if !strings.Contains(out.String(), "推荐权限已全部授权或没有可授权项") {
		t.Fatalf("output = %q, want all-granted message", out.String())
	}
}

func TestAuthLoginDefaultTUIModeRecommendedAlreadyGrantedSkipsTUIAndAuthorizationPage(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	oldGuideSelector := authLoginGuideActionSelector
	oldGuideApplier := authLoginGuideActionApplier
	oldScopeSelector := loginRecommendScopeModeSelector
	oldProductSelector := loginRecommendProductSelector
	oldInteractiveTerminal := authLoginInteractiveTerminal
	t.Cleanup(func() {
		authLoginGuideActionSelector = oldGuideSelector
		authLoginGuideActionApplier = oldGuideApplier
		loginRecommendScopeModeSelector = oldScopeSelector
		loginRecommendProductSelector = oldProductSelector
		authLoginInteractiveTerminal = oldInteractiveTerminal
	})
	authLoginInteractiveTerminal = func() bool { return true }
	authLoginGuideActionSelector = func() (authLoginGuideAction, error) {
		t.Fatal("default auth login must not call the operation guide selector")
		return "", nil
	}
	authLoginGuideActionApplier = func(*cobra.Command, string, authLoginGuideAction) error {
		t.Fatal("default auth login must not apply a post-login guide action")
		return nil
	}
	loginRecommendScopeModeSelector = func() (pat.LoginRecommendScopeMode, error) {
		t.Fatal("already-granted recommended auth must not call the scope-mode TUI")
		return "", nil
	}
	loginRecommendProductSelector = func([]pat.LoginRecommendProduct) ([]string, error) {
		t.Fatal("already-granted recommended auth must not call product-domain TUI")
		return nil, nil
	}

	fake := &authLoginRecommendSequenceCaller{responses: []string{
		`{"success":true,"data":{"allGranted":false,"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":[]}}`,
	}}
	cmd := newAuthLoginCommand(fake)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--token", "login-token"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login error = %v\noutput:\n%s", err, out.String())
	}
	if len(fake.tools) != 1 {
		t.Fatalf("CallTool count = %d, want only preflight recommend plan", len(fake.tools))
	}
	if fake.tools[0] != "pat.batch_plan" {
		t.Fatalf("tool sequence = %v, want only plan", fake.tools)
	}
	if !strings.Contains(out.String(), "推荐权限已全部授权或没有可授权项") {
		t.Fatalf("output = %q, want already-granted message", out.String())
	}
}

func TestAuthLoginDefaultTUIRunsAfterLoginTokenSaved(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)

	oldGuideSelector := authLoginGuideActionSelector
	oldGuideApplier := authLoginGuideActionApplier
	oldScopeSelector := loginRecommendScopeModeSelector
	oldProductSelector := loginRecommendProductSelector
	oldInteractiveTerminal := authLoginInteractiveTerminal
	t.Cleanup(func() {
		authLoginGuideActionSelector = oldGuideSelector
		authLoginGuideActionApplier = oldGuideApplier
		loginRecommendScopeModeSelector = oldScopeSelector
		loginRecommendProductSelector = oldProductSelector
		authLoginInteractiveTerminal = oldInteractiveTerminal
	})
	authLoginInteractiveTerminal = func() bool { return true }

	var sawTokenBeforeScopeTUI bool
	var sawTokenBeforeProductTUI bool
	var sawTokenBeforePlan bool
	authLoginGuideActionSelector = func() (authLoginGuideAction, error) {
		t.Fatal("default login must not call the operation guide selector")
		return "", nil
	}
	authLoginGuideActionApplier = func(*cobra.Command, string, authLoginGuideAction) error {
		t.Fatal("default login must not apply a post-login guide action")
		return nil
	}
	loginRecommendScopeModeSelector = func() (pat.LoginRecommendScopeMode, error) {
		token, err := authpkg.LoadTokenData(configDir)
		if err != nil {
			t.Fatalf("LoadTokenData before scope TUI error = %v", err)
		}
		if token.AccessToken != "login-token" {
			t.Fatalf("AccessToken before scope TUI = %q, want login-token", token.AccessToken)
		}
		sawTokenBeforeScopeTUI = true
		return pat.LoginRecommendScopeAll, nil
	}
	loginRecommendProductSelector = func(products []pat.LoginRecommendProduct) ([]string, error) {
		token, err := authpkg.LoadTokenData(configDir)
		if err != nil {
			t.Fatalf("LoadTokenData before product TUI error = %v", err)
		}
		if token.AccessToken != "login-token" {
			t.Fatalf("AccessToken before product TUI = %q, want login-token", token.AccessToken)
		}
		sawTokenBeforeProductTUI = true
		if len(products) != 1 || products[0].ProductCode != "calendar" {
			t.Fatalf("selector products = %+v, want calendar", products)
		}
		return []string{"calendar"}, nil
	}

	fake := &authLoginRecommendSequenceCaller{responses: []string{
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"items":[{"scope":"calendar.event:read","productCode":"calendar","productName":"日历"}],"selectedScopes":["calendar.event:read"]}}`,
		`{"success":true,"data":{"grantedScopes":["calendar.event:read"]}}`,
	}, beforeCall: func(toolName string) {
		token, err := authpkg.LoadTokenData(configDir)
		if err != nil {
			t.Fatalf("LoadTokenData before %s error = %v", toolName, err)
		}
		if token.AccessToken != "login-token" {
			t.Fatalf("AccessToken before %s = %q, want login-token", toolName, token.AccessToken)
		}
		sawTokenBeforePlan = true
	}}
	cmd := newAuthLoginCommand(fake)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--token", "login-token"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login error = %v\noutput:\n%s", err, out.String())
	}
	if !sawTokenBeforeScopeTUI {
		t.Fatal("scope-mode TUI was not called after token save")
	}
	if !sawTokenBeforeProductTUI {
		t.Fatal("product-domain TUI was not called after token save")
	}
	if !sawTokenBeforePlan {
		t.Fatal("authorization plan was not called after token save")
	}
	if len(fake.tools) != 3 {
		t.Fatalf("CallTool count = %d, want discovery plan + selected plan + grant", len(fake.tools))
	}
	if fake.tools[0] != "pat.batch_plan" || fake.tools[1] != "pat.batch_plan" || fake.tools[2] != "pat.batch_grant" {
		t.Fatalf("tool sequence = %v, want plan, plan, grant", fake.tools)
	}
	if got := fake.args[0]["recommend"]; got != true {
		t.Fatalf("discovery plan recommend = %#v, want true", got)
	}
	if got := fake.args[1]["recommend"]; got != false {
		t.Fatalf("selected all-scope plan recommend = %#v, want false", got)
	}
	if got := fake.args[1]["productCodes"]; !stringSliceArgEqual(got, []string{"calendar"}) {
		t.Fatalf("selected all-scope plan productCodes = %#v, want calendar", got)
	}
}

func TestEnrichAuthLoginProfileFromContactPersistsCorpName(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)

	token := &authpkg.TokenData{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       "ding32fff839a3e0105d",
		ClientID:     "client-id",
		Source:       "mcp",
	}
	if err := authpkg.SaveTokenData(configDir, token); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}

	fake := &authLoginRecommendSequenceCaller{responses: []string{
		`{"success":true,"result":[{"orgEmployeeModel":{"corpId":"ding32fff839a3e0105d","orgName":"钉钉（中国）信息技术有限公司","userId":"011352590165863362195","orgUserName":"玄玦(主用钉)"}}]}`,
	}}
	if err := enrichAuthLoginProfileFromContact(context.Background(), configDir, fake, token); err != nil {
		t.Fatalf("enrichAuthLoginProfileFromContact() error = %v", err)
	}
	if token.CorpName != "钉钉（中国）信息技术有限公司" {
		t.Fatalf("token corpName = %q, want 钉钉（中国）信息技术有限公司", token.CorpName)
	}
	if token.UserID != "011352590165863362195" || token.UserName != "玄玦(主用钉)" {
		t.Fatalf("token user identity = (%q, %q), want contact result", token.UserID, token.UserName)
	}

	loaded, err := authpkg.LoadTokenDataForProfile(configDir, "ding32fff839a3e0105d")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile() error = %v", err)
	}
	if loaded.CorpName != "钉钉（中国）信息技术有限公司" {
		t.Fatalf("persisted corpName = %q, want 钉钉（中国）信息技术有限公司", loaded.CorpName)
	}
	if len(fake.tools) != 1 || fake.tools[0] != "get_current_user_profile" {
		t.Fatalf("tool calls = %v, want get_current_user_profile", fake.tools)
	}
	if got := fake.args[0]["profile"]; got != "ding32fff839a3e0105d" {
		t.Fatalf("contact profile arg = %#v, want ding32fff839a3e0105d", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type authLoginRecommendSequenceCaller struct {
	responses  []string
	tools      []string
	args       []map[string]any
	beforeCall func(toolName string)
}

func (f *authLoginRecommendSequenceCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	if f.beforeCall != nil {
		f.beforeCall(toolName)
	}
	f.tools = append(f.tools, toolName)
	copiedArgs := make(map[string]any, len(args))
	for key, value := range args {
		copiedArgs[key] = value
	}
	f.args = append(f.args, copiedArgs)
	response := `{"success":true,"data":{}}`
	if len(f.responses) > 0 {
		response = f.responses[0]
		f.responses = f.responses[1:]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}}, nil
}

func (f *authLoginRecommendSequenceCaller) Format() string { return "table" }

func (f *authLoginRecommendSequenceCaller) DryRun() bool { return false }

func (f *authLoginRecommendSequenceCaller) Fields() string { return "" }

func (f *authLoginRecommendSequenceCaller) JQ() string { return "" }

func stringSliceArgEqual(got any, want []string) bool {
	if got == nil {
		return len(want) == 0
	}
	switch values := got.(type) {
	case []string:
		if len(values) != len(want) {
			return false
		}
		for i := range values {
			if values[i] != want[i] {
				return false
			}
		}
		return true
	case []any:
		if len(values) != len(want) {
			return false
		}
		for i := range values {
			if values[i] != want[i] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func setupAuthLogoutProfiles(t *testing.T, tokens ...*authpkg.TokenData) string {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, filepath.Join(root, "keychain"))
	t.Setenv("DWS_CONFIG_DIR", configDir)
	authpkg.SetRuntimeProfile("")
	ResetRuntimeTokenCache()
	clearCompatCache()
	t.Cleanup(func() {
		_ = authpkg.DeleteAllTokenData(configDir)
		authpkg.SetRuntimeProfile("")
		ResetRuntimeTokenCache()
		clearCompatCache()
		CloseFileLogger()
	})

	for _, token := range tokens {
		if err := authpkg.SaveTokenData(configDir, token); err != nil {
			t.Fatalf("SaveTokenData(%s) error = %v", token.CorpID, err)
		}
	}
	return configDir
}

func authLogoutTestToken(corpID string) *authpkg.TokenData {
	return &authpkg.TokenData{
		AccessToken:  "access-" + corpID,
		RefreshToken: "refresh-" + corpID,
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       corpID,
		CorpName:     corpID + " org",
		UserID:       "user-" + corpID,
		UserName:     "User " + corpID,
		ClientID:     "client-" + corpID,
	}
}
