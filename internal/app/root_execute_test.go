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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	mockmcp "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mock_mcp"
)

// patLikeError simulates an edition-specific PAT error that implements both
// ExitCoder (exit code 4) and RawStderrError (raw JSON to stderr).
type patLikeError struct{ raw string }

func (e *patLikeError) Error() string     { return e.raw }
func (e *patLikeError) ExitCode() int     { return 4 }
func (e *patLikeError) RawStderr() string { return e.raw }

func TestPrintExecutionErrorDefaultsToJSON(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := printExecutionError(root, &stdout, &stderr, apperrors.NewValidation(
		"bad flag",
		apperrors.WithHint("Pass the required flag and retry."),
	))
	if err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when errors go to stderr", stdout.String())
	}
	if !strings.Contains(stderr.String(), "\"category\": \"validation\"") {
		t.Fatalf("stderr = %q, want JSON error payload", stderr.String())
	}
}

func TestPrintExecutionErrorUsesJSONWhenFormatIsJSON(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	if err := root.PersistentFlags().Set("format", "json"); err != nil {
		t.Fatalf("Set(format) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := printExecutionError(root, &stdout, &stderr, apperrors.NewValidation("bad flag"))
	if err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when errors go to stderr", stdout.String())
	}
	if !strings.Contains(stderr.String(), "\"category\": \"validation\"") {
		t.Fatalf("stderr = %q, want JSON error payload", stderr.String())
	}
}

func TestPrintExecutionErrorUsesJSONWhenCommandSetsJSONFlag(t *testing.T) {
	setupRuntimeCommandTest(t)

	server := mockmcp.DefaultServer()
	defer server.Close()
	t.Setenv(cli.CatalogFixtureEnv, writeDocCatalogFixture(t, server.RemoteURL("/server/doc"), false))

	root := NewRootCommand()
	root.SetArgs([]string{"mcp", "doc", "search_documents", "--json", "{"})

	executed, execErr := root.ExecuteC()
	if execErr == nil {
		t.Fatal("ExecuteC() error = nil, want validation error")
	}
	if executed == nil {
		t.Fatal("ExecuteC() returned nil command")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := printExecutionError(executed, &stdout, &stderr, execErr)
	if err != nil {
		t.Fatalf("printExecutionError() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when errors go to stderr", stdout.String())
	}
	if !strings.Contains(stderr.String(), "\"category\": \"validation\"") {
		t.Fatalf("stderr = %q, want JSON error payload", stderr.String())
	}
}

func TestCompletionCommandUsesConfiguredWriter(t *testing.T) {
	setupRuntimeCommandTest(t)

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"completion", "bash"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "bash completion for dws") {
		t.Fatalf("output = %q, want completion script in configured writer", out.String())
	}
}

func TestUnknownSubcommandShowsHelp(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"cache", "nonexistent-cmd"})

	executed, err := root.ExecuteC()
	if err == nil {
		t.Fatal("ExecuteC() error = nil, want unknown command error")
	}
	if !isUnknownCommandError(err) {
		t.Fatalf("isUnknownCommandError() = false for error: %v", err)
	}

	// Simulate what Execute() does: redirect output to stderr and print help
	if executed == nil {
		executed = root
	}
	executed.SetOut(&out)
	_ = executed.Help()

	combined := out.String()
	// Help text should include the parent command's usage
	if !strings.Contains(combined, "cache") {
		t.Fatalf("output should contain parent command name 'cache', got:\n%s", combined)
	}
	// Help text should list available subcommands
	if !strings.Contains(combined, "Available Commands") {
		t.Fatalf("output should contain 'Available Commands', got:\n%s", combined)
	}
	if !strings.Contains(combined, "refresh") {
		t.Fatalf("output should list 'refresh' subcommand, got:\n%s", combined)
	}
}

func TestVersionCommandDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(version) error = %v", err)
	}
	if !strings.Contains(out.String(), "Version:") {
		t.Fatalf("version output missing Version line:\n%s", out.String())
	}
}

func TestVersionCommandUsesCachedRegistryWithoutBlockingAgedDiscovery(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")

	cacheDir := t.TempDir()
	t.Setenv(cli.CacheDirEnv, cacheDir)
	store := cache.NewStore(cacheDir)
	if err := store.SaveRegistry("default/default", cache.RegistrySnapshot{
		SavedAt: time.Now().UTC().Add(-2 * time.Hour),
		Servers: []market.ServerDescriptor{minimalCLIServer("cached", "https://mcp.dingtalk.com/cached/v1")},
	}); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(marketListResponse("network-server"))
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})

	start := time.Now()
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(version) error = %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("Execute(version) took %v, want cached startup under 200ms", elapsed)
	}
	if !strings.Contains(out.String(), "Version:") {
		t.Fatalf("version output missing Version line:\n%s", out.String())
	}
}

func TestRootHelpDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 1, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("aiapp", "AI应用管理", nil, map[string]any{
				"create_ai_app": map[string]any{
					"cliName": "create",
					"flags":   map[string]any{},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--help) error = %v", err)
	}
	if !strings.Contains(out.String(), "Discovered MCP Services:") {
		t.Fatalf("root help output missing MCP summary:\n%s", out.String())
	}
	for _, want := range []string{"Utility Commands:", "skill", "auth", "version"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("root help output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRootShortHelpDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 1, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("devdoc", "开放平台文档搜索", map[string]any{
				"article": map[string]any{"description": "文档文章"},
			}, map[string]any{
				"search_article": map[string]any{
					"cliName": "search",
					"group":   "article",
					"flags":   map[string]any{},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(-h) error = %v", err)
	}
	if !strings.Contains(out.String(), "Discovered MCP Services:") {
		t.Fatalf("root short help output missing MCP summary:\n%s", out.String())
	}
}

func TestNestedShortHelpDoesNotRequirePINOrLogin(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(cli.CatalogFixtureEnv, "")
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	response := map[string]any{
		"metadata": map[string]any{"count": 1, "nextCursor": ""},
		"servers": []any{
			discoveryServerEntry("devdoc", "开放平台文档搜索", map[string]any{
				"article": map[string]any{"description": "文档文章"},
			}, map[string]any{
				"search_article": map[string]any{
					"cliName": "search",
					"group":   "article",
					"flags": map[string]any{
						"keyword": map[string]any{"alias": "keyword"},
					},
				},
			}),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	SetDiscoveryBaseURL(srv.URL)
	t.Cleanup(func() { SetDiscoveryBaseURL("") })

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"devdoc", "article", "search", "-h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(devdoc article search -h) error = %v", err)
	}
	if !strings.Contains(out.String(), "devdoc/search") {
		t.Fatalf("nested short help output missing command title:\n%s", out.String())
	}
}

func TestPrintExecutionError_RawStderrError_writes_raw_JSON_to_stderr(t *testing.T) {
	t.Parallel()

	rawJSON := `{"success":false,"code":"PAT_LOW_RISK_NO_PERMISSION","data":{}}`
	err := &patLikeError{raw: rawJSON}

	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	writeErr := printExecutionError(root, &stdout, &stderr, err)
	if writeErr != nil {
		t.Fatalf("printExecutionError() error = %v", writeErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty for RawStderrError", stdout.String())
	}
	got := strings.TrimSpace(stderr.String())
	if got != rawJSON {
		t.Fatalf("stderr = %q, want raw JSON %q", got, rawJSON)
	}
}

func TestPrintExecutionError_RawStderrError_exit_code_is_4(t *testing.T) {
	t.Parallel()

	err := &patLikeError{raw: `{"code":"PAT_MEDIUM_RISK_NO_PERMISSION"}`}
	exitCode := apperrors.ExitCode(err)
	if exitCode != 4 {
		t.Fatalf("apperrors.ExitCode(patLikeError) = %d, want 4", exitCode)
	}
}

func TestPrintExecutionError_RawStderrError_takes_precedence_over_JSON_mode(t *testing.T) {
	t.Parallel()

	rawJSON := `{"success":false,"code":"PAT_HIGH_RISK_NO_PERMISSION"}`
	err := &patLikeError{raw: rawJSON}

	root := NewRootCommand()
	_ = root.PersistentFlags().Set("format", "json")

	var stdout, stderr bytes.Buffer
	writeErr := printExecutionError(root, &stdout, &stderr, err)
	if writeErr != nil {
		t.Fatalf("printExecutionError() error = %v", writeErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty — RawStderrError should bypass JSON mode", stdout.String())
	}
	if !strings.Contains(stderr.String(), "PAT_HIGH_RISK_NO_PERMISSION") {
		t.Fatalf("stderr = %q, want raw PAT JSON", stderr.String())
	}
}

// simulateExecuteWithPanic mirrors the recovery pattern in Execute():
// named return + defer recover → exitCode = 5 on panic.
func simulateExecuteWithPanic(doPanic bool) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			exitCode = 5
		}
	}()
	if doPanic {
		panic("test panic")
	}
	return 0
}

func TestExecute_panic_recovery_returns_exit_5(t *testing.T) {
	t.Parallel()
	code := simulateExecuteWithPanic(true)
	if code != 5 {
		t.Fatalf("panic recovery exitCode = %d, want 5", code)
	}
}

func TestExecute_no_panic_returns_0(t *testing.T) {
	t.Parallel()
	code := simulateExecuteWithPanic(false)
	if code != 0 {
		t.Fatalf("no-panic exitCode = %d, want 0", code)
	}
}
