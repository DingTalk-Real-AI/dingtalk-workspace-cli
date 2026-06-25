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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
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
	if err := os.WriteFile(inputPath, exported.Bytes(), 0o600); err != nil {
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
