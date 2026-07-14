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
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestAuthInstanceHelpContracts(t *testing.T) {
	setupAuthInstanceCommandTest(t)

	loginHelp, err := executeAuthInstanceCommand(t, "auth", "login", "--help")
	if err != nil {
		t.Fatalf("auth login --help error = %v\noutput:\n%s", err, loginHelp)
	}
	if !strings.Contains(loginHelp, "--new-instance") {
		t.Fatalf("auth login --help missing --new-instance:\n%s", loginHelp)
	}

	authHelp, err := executeAuthInstanceCommand(t, "auth", "--help")
	if err != nil {
		t.Fatalf("auth --help error = %v\noutput:\n%s", err, authHelp)
	}
	for _, command := range []string{"list", "use"} {
		if !strings.Contains(authHelp, "\n  "+command) {
			t.Fatalf("auth --help missing %q subcommand:\n%s", command, authHelp)
		}
	}
}

func TestAuthInstanceListLegacyIsReadOnly(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)

	out, err := executeAuthInstanceCommand(t, "--format", "json", "auth", "list")
	if err != nil {
		t.Fatalf("auth list error = %v\noutput:\n%s", err, out)
	}
	var response struct {
		CurrentInstanceID string `json:"currentInstanceId"`
		Instances         []struct {
			ID      string                   `json:"id"`
			Alias   string                   `json:"alias"`
			Kind    authpkg.AuthInstanceKind `json:"kind"`
			Current bool                     `json:"current"`
		} `json:"instances"`
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("unmarshal auth list output: %v\noutput:\n%s", err, out)
	}
	if response.CurrentInstanceID != "" {
		t.Fatalf("legacy list unexpectedly enabled registry: %#v", response)
	}
	if len(response.Instances) != 1 || response.Instances[0].Alias != authpkg.DefaultAuthInstanceAlias ||
		response.Instances[0].Kind != authpkg.AuthInstanceKindLegacy || !response.Instances[0].Current {
		t.Fatalf("legacy auth list = %#v, want one current default instance", response.Instances)
	}
	if _, err := os.Stat(filepath.Join(configDir, "auth")); !os.IsNotExist(err) {
		t.Fatalf("read-only auth list created auth directory, stat error = %v", err)
	}
}

func TestAuthLoginNewInstanceCommitsAfterTokenPersistence(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)

	out, err := executeAuthInstanceCommand(t,
		"--format", "json", "auth", "login",
		"--token", "isolated-login-token", "--new-instance", "personal", "--yes",
	)
	if err != nil {
		t.Fatalf("auth login --new-instance error = %v\noutput:\n%s", err, out)
	}
	instances, currentID, err := authpkg.ListAuthInstances(configDir)
	if err != nil {
		t.Fatalf("ListAuthInstances() error = %v", err)
	}
	if currentID == "" || len(instances) != 2 {
		t.Fatalf("instances = %#v, currentID = %q; want committed two-instance registry", instances, currentID)
	}
	current := authInstanceByID(instances, currentID)
	if current == nil || current.Alias != "personal" || current.Kind != authpkg.AuthInstanceKindIsolated {
		t.Fatalf("current instance = %#v, want isolated personal", current)
	}
	store := authpkg.ResolveAuthStore(configDir)
	if !store.Isolated() || store.InstanceID != currentID {
		t.Fatalf("active store = %#v, want committed personal store", store)
	}
	data, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if data.AccessToken != "isolated-login-token" {
		t.Fatalf("isolated access token = %q, want isolated-login-token", data.AccessToken)
	}
}

func TestAuthLoginNewInstanceCanTargetExistingProfile(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	if err := authpkg.SaveTokenData(configDir, authAccountTestToken("corp-target", "user-target", "legacy-token")); err != nil {
		t.Fatal(err)
	}

	out, err := executeAuthInstanceCommand(t,
		"--profile", "corp-target", "--format", "json", "auth", "login",
		"--token", "isolated-token", "--new-instance", "targeted", "--yes",
	)
	if err != nil {
		t.Fatalf("auth login --new-instance with --profile error = %v\noutput:\n%s", err, out)
	}
	instances, currentID, err := authpkg.ListAuthInstances(configDir)
	if err != nil {
		t.Fatal(err)
	}
	current := authInstanceByID(instances, currentID)
	if current == nil || current.Alias != "targeted" || current.Kind != authpkg.AuthInstanceKindIsolated {
		t.Fatalf("current instance = %#v, want targeted isolated instance", current)
	}
}

func TestAuthInstanceCLIUseAndPreviousClearRuntimeTokenCache(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	createAuthInstanceFixture(t, configDir, "personal",
		[]*authpkg.TokenData{authAccountTestToken("corp-default", "user-default", "token-default")},
		[]*authpkg.TokenData{authAccountTestToken("corp-personal", "user-personal", "token-personal")},
	)

	// Simulate a fresh process: no runtime selection exists before root command
	// construction. NewRootCommand must activate the persisted main account
	// before plugin construction performs any token read.
	authpkg.DeactivateAuthStore(configDir)
	cmd := NewRootCommand()
	if got := authpkg.ResolveAuthStore(configDir); got.InstanceAlias != "personal" || !got.Isolated() {
		t.Fatalf("root construction did not pre-activate persisted main account: %#v", got)
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"auth", "use", "default"})
	setAuthInstanceRuntimeTokenCache("stale-personal")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth use default error = %v\noutput:\n%s", err, output.String())
	}
	assertAuthInstanceRuntimeTokenCacheEmpty(t)
	if got := authpkg.ResolveAuthStore(configDir); got.InstanceAlias != "default" || got.Isolated() {
		t.Fatalf("active store after auth use default = %#v", got)
	}

	output.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"auth", "use", "-"})
	setAuthInstanceRuntimeTokenCache("stale-default")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth use - error = %v\noutput:\n%s", err, output.String())
	}
	assertAuthInstanceRuntimeTokenCacheEmpty(t)
	if got := authpkg.ResolveAuthStore(configDir); got.InstanceAlias != "personal" || !got.Isolated() {
		t.Fatalf("active store after auth use - = %#v", got)
	}
}

func TestRootFailsClosedWhenAuthInstanceChangesAfterPluginConstruction(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	createAuthInstanceFixture(t, configDir, "personal",
		[]*authpkg.TokenData{authAccountTestToken("corp-default", "user-default", "token-default")},
		[]*authpkg.TokenData{authAccountTestToken("corp-personal", "user-personal", "token-personal")},
	)

	for _, tc := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "token-reading-command", args: []string{"auth", "status"}, wantError: true},
		{name: "token-free-auth-list", args: []string{"auth", "list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := authpkg.UseAuthInstance(configDir, "default"); err != nil {
				t.Fatal(err)
			}
			authpkg.DeactivateAuthStore(configDir)
			cmd := NewRootCommand()
			// Simulate another process changing registry.json after plugin
			// construction but before Cobra PersistentPreRunE.
			if _, err := authpkg.UseAuthInstance(configDir, "personal"); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "登录态实例在命令初始化期间发生变化") {
					t.Fatalf("Execute() error = %v, output=%q; want fail-closed instance-change error", err, output.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("token-free recovery command error = %v, output=%q", err, output.String())
			}
		})
	}
}

func TestAuthInstanceLogoutByUserIDOnlyTouchesCurrentInstance(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	createAuthInstanceFixture(t, configDir, "personal",
		[]*authpkg.TokenData{authAccountTestToken("corp-default", "shared-user", "token-default")},
		[]*authpkg.TokenData{
			authAccountTestToken("corp-target", "shared-user", "token-target"),
			authAccountTestToken("corp-keep", "other-user", "token-keep"),
		},
	)

	originalTransport := http.DefaultTransport
	http.DefaultTransport = authAccountRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("remote revoke disabled in auth account test")
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	out, err := executeAuthInstanceCommand(t, "auth", "logout", "--user-id", "shared-user")
	if err != nil {
		t.Fatalf("auth logout --user-id error = %v\noutput:\n%s", err, out)
	}
	if got := authpkg.ResolveAuthStore(configDir); got.InstanceAlias != "personal" || !got.Isolated() {
		t.Fatalf("logout changed active account: %#v", got)
	}
	profiles, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles(personal) error = %v", err)
	}
	if len(profiles.Profiles) != 1 || profiles.Profiles[0].CorpID != "corp-keep" {
		t.Fatalf("personal profiles after logout = %#v, want only corp-keep", profiles.Profiles)
	}
	if authpkg.TokenDataExistsKeychainForCorpIDAt(configDir, "corp-target") {
		t.Fatal("target token still exists in current account")
	}
	if !authpkg.TokenDataExistsKeychainForCorpIDAt(configDir, "corp-keep") {
		t.Fatal("unmatched token was deleted from current account")
	}

	if _, err := authpkg.UseAuthInstance(configDir, "default"); err != nil {
		t.Fatalf("UseAuthInstance(default) error = %v", err)
	}
	legacy, err := authpkg.LoadTokenDataForProfile(configDir, "corp-default")
	if err != nil {
		t.Fatalf("legacy account token was removed: %v", err)
	}
	if legacy.AccessToken != "token-default" || legacy.UserID != "shared-user" {
		t.Fatalf("legacy account token = %#v, want untouched shared-user token", legacy)
	}
}

func TestAuthInstanceScopedResetPreservesOtherInstancesAndSharedAppConfig(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	if err := authpkg.SaveAppConfig(configDir, &authpkg.AppConfig{ClientID: "shared-client"}); err != nil {
		t.Fatalf("SaveAppConfig() error = %v", err)
	}
	for _, name := range []string{"mcp_url", "token"} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte("shared-"+name), 0o600); err != nil {
			t.Fatalf("write shared %s: %v", name, err)
		}
	}
	createAuthInstanceFixture(t, configDir, "personal",
		[]*authpkg.TokenData{authAccountTestToken("corp-default", "user-default", "token-default")},
		[]*authpkg.TokenData{authAccountTestToken("corp-personal", "user-personal", "token-personal")},
	)
	inputPath := filepath.Join(t.TempDir(), "auth-bundle")
	if err := os.WriteFile(inputPath, []byte("not-used"), 0o600); err != nil {
		t.Fatalf("write import stub: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "export", args: []string{"auth", "export", "--base64"}},
		{name: "import", args: []string{"auth", "import", "--input", inputPath}},
		{name: "migrate", args: []string{"auth", "migrate-keychain", "--dry-run"}},
	} {
		out, err := executeAuthInstanceCommand(t, tc.args...)
		if err == nil {
			t.Fatalf("isolated auth %s unexpectedly succeeded\noutput:\n%s", tc.name, out)
		}
		if !strings.Contains(err.Error(), "auth use default") {
			t.Fatalf("isolated auth %s error = %v, want default-account hint", tc.name, err)
		}
	}

	out, err := executeAuthInstanceCommand(t, "auth", "reset")
	if err != nil {
		t.Fatalf("isolated auth reset error = %v\noutput:\n%s", err, out)
	}
	shared, err := authpkg.LoadAppConfig(configDir)
	if err != nil {
		t.Fatalf("LoadAppConfig() after isolated reset error = %v", err)
	}
	if shared == nil || shared.ClientID != "shared-client" {
		t.Fatalf("shared app config after isolated reset = %#v", shared)
	}
	if authpkg.TokenDataExistsKeychainForCorpIDAt(configDir, "corp-personal") {
		t.Fatal("isolated reset retained personal token")
	}

	out, err = executeAuthInstanceCommand(t, "auth", "use", "default")
	if err != nil {
		t.Fatalf("auth use default error = %v\noutput:\n%s", err, out)
	}
	if err := rejectIsolatedAuthOperation(configDir, "export"); err != nil {
		t.Fatalf("portable guard rejected legacy-backed account after auth use default: %v", err)
	}
	legacy, err := authpkg.LoadTokenDataForProfile(configDir, "corp-default")
	if err != nil || legacy.AccessToken != "token-default" {
		t.Fatalf("isolated reset changed legacy token: token=%#v err=%v", legacy, err)
	}
	if _, err := authpkg.UseAuthInstance(configDir, "personal"); err != nil {
		t.Fatal(err)
	}
	if err := authpkg.SaveTokenData(configDir, authAccountTestToken("corp-personal", "user-personal", "token-personal-restored")); err != nil {
		t.Fatal(err)
	}
	if _, err := authpkg.UseAuthInstance(configDir, "default"); err != nil {
		t.Fatal(err)
	}

	out, err = executeAuthInstanceCommand(t, "auth", "reset")
	if err != nil {
		t.Fatalf("default instance reset error = %v\noutput:\n%s", err, out)
	}
	shared, err = authpkg.LoadAppConfig(configDir)
	if err != nil || shared == nil || shared.ClientID != "shared-client" {
		t.Fatalf("shared app config after default instance reset = %#v, %v", shared, err)
	}
	for _, name := range []string{"mcp_url", "token"} {
		if _, err := os.Stat(filepath.Join(configDir, name)); err != nil {
			t.Fatalf("default instance reset removed shared %s: %v", name, err)
		}
	}
	if authpkg.TokenDataExistsKeychainForCorpIDAt(configDir, "corp-default") {
		t.Fatal("default instance reset retained its own user token")
	}
	out, err = executeAuthInstanceCommand(t, "auth", "use", "personal")
	if err != nil {
		t.Fatalf("auth use personal after default reset error = %v\noutput:\n%s", err, out)
	}
	personal, err := authpkg.LoadTokenDataForProfile(configDir, "corp-personal")
	if err != nil || personal.AccessToken != "token-personal-restored" {
		t.Fatalf("default reset changed personal instance: token=%#v err=%v", personal, err)
	}
}

func TestAuthInstanceLegacyResetKeepsHistoricalFullResetBeforeOptIn(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	if err := authpkg.SaveAppConfig(configDir, &authpkg.AppConfig{ClientID: "legacy-client"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mcp_url", "token"} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte("legacy-"+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := authpkg.SaveTokenData(configDir, authAccountTestToken("legacy-corp", "legacy-user", "legacy-access")); err != nil {
		t.Fatal(err)
	}

	out, err := executeAuthInstanceCommand(t, "auth", "reset")
	if err != nil {
		t.Fatalf("legacy auth reset error = %v\noutput:\n%s", err, out)
	}
	if authpkg.HasAppConfig(configDir) {
		t.Fatal("zero-registry reset retained historical app config")
	}
	for _, name := range []string{"mcp_url", "token"} {
		if _, err := os.Stat(filepath.Join(configDir, name)); !os.IsNotExist(err) {
			t.Fatalf("zero-registry reset retained %s: %v", name, err)
		}
	}
}

func TestAuthInstanceHookEditionCanListAndRecoverToDefault(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	createAuthInstanceFixture(t, configDir, "personal", nil,
		[]*authpkg.TokenData{authAccountTestToken("corp-personal", "user-personal", "token-personal")},
	)

	previous := edition.Get()
	preRunCalls := 0
	edition.Override(&edition.Hooks{
		Name: "hooked",
		LoadToken: func(string) ([]byte, error) {
			return nil, authpkg.ErrTokenDataNotFound
		},
		AfterPersistentPreRun: func(*cobra.Command, []string) error {
			preRunCalls++
			return nil
		},
	})
	t.Cleanup(func() { edition.Override(previous) })

	out, err := executeAuthInstanceCommand(t, "--format", "json", "auth", "list")
	if err != nil || !strings.Contains(out, `"alias":"personal"`) {
		t.Fatalf("hook edition auth list = %q, %v", out, err)
	}
	if preRunCalls != 1 {
		t.Fatalf("hook edition pre-run calls after list = %d, want 1", preRunCalls)
	}
	out, err = executeAuthInstanceCommand(t, "auth", "status")
	if err == nil || !strings.Contains(err.Error(), "failed to activate login state") {
		t.Fatalf("hook edition token command = %q, %v; want fail-closed", out, err)
	}
	if preRunCalls != 1 {
		t.Fatalf("failed-closed status unexpectedly ran edition pre-run: %d", preRunCalls)
	}
	out, err = executeAuthInstanceCommand(t, "auth", "use", "default")
	if err != nil {
		t.Fatalf("hook edition auth use default = %q, %v", out, err)
	}
	if preRunCalls != 2 {
		t.Fatalf("hook edition pre-run calls after use = %d, want 2", preRunCalls)
	}
	store, err := authpkg.ActivateCurrentInstance(configDir)
	if err != nil || store.Isolated() || store.ConfigDir != configDir {
		t.Fatalf("hook edition recovered store = %#v, %v", store, err)
	}
}

func TestAuthInstanceCorruptRegistryRootPreRunFailsClosed(t *testing.T) {
	configDir := setupAuthInstanceCommandTest(t)
	if err := authpkg.SaveTokenData(configDir, authAccountTestToken("corp-default", "user-default", "legacy-secret")); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	registryPath := authpkg.AuthRegistryPath(configDir)
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o700); err != nil {
		t.Fatalf("create registry dir: %v", err)
	}
	corrupt := []byte(`{"version":1,"revision":1,"instance":`)
	if err := os.WriteFile(registryPath, corrupt, 0o600); err != nil {
		t.Fatalf("write corrupt registry: %v", err)
	}

	out, err := executeAuthInstanceCommand(t, "--format", "json", "auth", "list")
	if err == nil {
		t.Fatalf("auth list accepted corrupt registry\noutput:\n%s", out)
	}
	if !strings.Contains(err.Error(), "failed to activate login state") ||
		!strings.Contains(err.Error(), "parse auth registry") {
		t.Fatalf("corrupt registry error = %v, want fail-closed activation error", err)
	}
	if strings.Contains(out, `"instances"`) {
		t.Fatalf("auth list body ran despite corrupt registry:\n%s", out)
	}
	if got := authpkg.ResolveAuthStore(configDir); got.InstanceID != "" {
		t.Fatalf("corrupt registry left an account activated: %#v", got)
	}
	data, readErr := os.ReadFile(registryPath)
	if readErr != nil || !bytes.Equal(data, corrupt) {
		t.Fatalf("corrupt registry was modified: data=%q err=%v", data, readErr)
	}
}

func setupAuthInstanceCommandTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, filepath.Join(root, "keychain"))
	authpkg.DeactivateAuthStore(configDir)
	authpkg.SetRuntimeProfile("")
	ResetRuntimeTokenCache()
	clearCompatCache()
	t.Cleanup(func() {
		authpkg.DeactivateAuthStore(configDir)
		authpkg.SetRuntimeProfile("")
		ResetRuntimeTokenCache()
		clearCompatCache()
		CloseFileLogger()
	})
	return configDir
}

func executeAuthInstanceCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.Execute()
	CloseFileLogger()
	return output.String(), err
}

func createAuthInstanceFixture(t *testing.T, configDir, alias string, legacy, isolated []*authpkg.TokenData) {
	t.Helper()
	for _, token := range legacy {
		if err := authpkg.SaveTokenData(configDir, token); err != nil {
			t.Fatalf("SaveTokenData(legacy %s) error = %v", token.CorpID, err)
		}
	}
	tx, err := authpkg.BeginNewInstance(configDir, alias)
	if err != nil {
		t.Fatalf("BeginNewInstance(%s) error = %v", alias, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, token := range isolated {
		if err := authpkg.SaveTokenData(configDir, token); err != nil {
			t.Fatalf("SaveTokenData(isolated %s) error = %v", token.CorpID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit(%s) error = %v", alias, err)
	}
	committed = true
	ResetRuntimeTokenCache()
}

func authAccountTestToken(corpID, userID, accessToken string) *authpkg.TokenData {
	return &authpkg.TokenData{
		AccessToken:  accessToken,
		RefreshToken: "refresh-" + corpID,
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       corpID,
		CorpName:     corpID + " org",
		UserID:       userID,
		UserName:     userID + " name",
		ClientID:     "client-" + corpID,
	}
}

func authInstanceByID(instances []authpkg.AuthInstance, id string) *authpkg.AuthInstance {
	for i := range instances {
		if instances[i].ID == id {
			return &instances[i]
		}
	}
	return nil
}

func setAuthInstanceRuntimeTokenCache(token string) {
	cachedRuntimeTokenMu.Lock()
	defer cachedRuntimeTokenMu.Unlock()
	cachedRuntimeTokens = map[string]string{"sentinel": token}
}

func assertAuthInstanceRuntimeTokenCacheEmpty(t *testing.T) {
	t.Helper()
	cachedRuntimeTokenMu.Lock()
	defer cachedRuntimeTokenMu.Unlock()
	if len(cachedRuntimeTokens) != 0 {
		t.Fatalf("runtime token cache was not cleared: %#v", cachedRuntimeTokens)
	}
}

type authAccountRoundTripFunc func(*http.Request) (*http.Response, error)

func (f authAccountRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
