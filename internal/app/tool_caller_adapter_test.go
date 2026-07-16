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
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	patpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageToolCallerAdapter_GlobalDryRunAllowsOnlyExactPATReadOnlyPreviews(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("claw-type"); got != edition.DefaultOSSClawType {
			t.Errorf("claw-type = %q, want %q", got, edition.DefaultOSSClawType)
		}
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.Method != "tools/call" ||
			(request.Params.Name != "pat.batch_plan" && request.Params.Name != "pat.scope_revoke") ||
			request.Params.Arguments["dryRun"] != true {
			t.Errorf("unexpected real request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"content": map[string]any{"tool": request.Params.Name, "preview": "server-preview"},
			},
		})
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(server.URL), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}

	flags := &GlobalFlags{DryRun: true, Format: "json", Token: "test-token", Timeout: 5}
	runner := &runtimeRunner{
		loader:      cli.StaticLoader{},
		transport:   transport.NewClient(server.Client()),
		globalFlags: flags,
		fallback:    executor.EchoRunner{},
		auditSink:   audit.NopSink{},
	}
	caller := newToolCallerAdapter(runner, flags)

	plan, err := caller.CallTool(context.Background(), "pat", "pat.batch_plan", map[string]any{"dryRun": true})
	if err != nil {
		t.Fatalf("pat.batch_plan dry-run error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("real call count after plan = %d, want 1", got)
	}
	if len(plan.Content) != 1 || !strings.Contains(plan.Content[0].Text, "server-preview") {
		t.Fatalf("plan content = %#v, want server response", plan.Content)
	}
	revoke, err := caller.CallTool(context.Background(), "pat", "pat.scope_revoke", map[string]any{
		"scope":  "calendar.event:read",
		"dryRun": true,
	})
	if err != nil {
		t.Fatalf("pat.scope_revoke dry-run error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("real call count after revoke preview = %d, want 2", got)
	}
	if len(revoke.Content) != 1 || !strings.Contains(revoke.Content[0].Text, "pat.scope_revoke") {
		t.Fatalf("revoke content = %#v, want server response", revoke.Content)
	}

	blocked := []struct {
		name    string
		product string
		tool    string
		args    map[string]any
	}{
		{name: "grant", product: "pat", tool: "pat.batch_grant", args: map[string]any{"dryRun": true}},
		{name: "revoke false", product: "pat", tool: "pat.scope_revoke", args: map[string]any{"dryRun": false}},
		{name: "revoke missing", product: "pat", tool: "pat.scope_revoke", args: map[string]any{}},
		{name: "revoke string true", product: "pat", tool: "pat.scope_revoke", args: map[string]any{"dryRun": "true"}},
		{name: "plan false", product: "pat", tool: "pat.batch_plan", args: map[string]any{"dryRun": false}},
		{name: "plan missing", product: "pat", tool: "pat.batch_plan", args: map[string]any{}},
		{name: "other tool catalog miss", product: "dry-run-test-missing", tool: "read_only_tool", args: map[string]any{"dryRun": true}},
	}
	for _, tt := range blocked {
		t.Run(tt.name, func(t *testing.T) {
			result, err := caller.CallTool(context.Background(), tt.product, tt.tool, tt.args)
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, `"dry_run":true`) {
				t.Fatalf("dry-run echo content = %#v", result.Content)
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("real call count = %d, want 2", got)
			}
		})
	}
}

type dryRunInvocationCaptureRunner struct {
	invocation executor.Invocation
}

func (r *dryRunInvocationCaptureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.invocation = invocation
	return executor.Result{Invocation: invocation, Response: map[string]any{"content": map[string]any{"ok": true}}}, nil
}

func TestCrossPlatformCoverageToolCallerAdapter_ReadOnlyDryRunMarkerIsStrict(t *testing.T) {
	tests := []struct {
		name    string
		product string
		tool    string
		args    map[string]any
		want    bool
	}{
		{name: "exact plan", product: "pat", tool: "pat.batch_plan", args: map[string]any{"dryRun": true}, want: true},
		{name: "exact revoke", product: "pat", tool: "pat.scope_revoke", args: map[string]any{"dryRun": true}, want: true},
		{name: "request false", product: "pat", tool: "pat.batch_plan", args: map[string]any{"dryRun": false}},
		{name: "request missing", product: "pat", tool: "pat.batch_plan", args: map[string]any{}},
		{name: "string true rejected", product: "pat", tool: "pat.batch_plan", args: map[string]any{"dryRun": "true"}},
		{name: "revoke false", product: "pat", tool: "pat.scope_revoke", args: map[string]any{"dryRun": false}},
		{name: "revoke missing", product: "pat", tool: "pat.scope_revoke", args: map[string]any{}},
		{name: "revoke string true rejected", product: "pat", tool: "pat.scope_revoke", args: map[string]any{"dryRun": "true"}},
		{name: "other tool", product: "pat", tool: "pat.batch_grant", args: map[string]any{"dryRun": true}},
		{name: "other product", product: "calendar", tool: "pat.batch_plan", args: map[string]any{"dryRun": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &dryRunInvocationCaptureRunner{}
			caller := newToolCallerAdapter(runner, &GlobalFlags{DryRun: true})
			if _, err := caller.CallTool(context.Background(), tt.product, tt.tool, tt.args); err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if got := runner.invocation.AllowReadOnlyDuringDryRun; got != tt.want {
				t.Fatalf("AllowReadOnlyDuringDryRun = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageToolCallerAdapter_ReadOnlyDryRunPATChallengeHasNoAuthSideEffects(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
	t.Setenv(authpkg.AgentCodeEnv, "")

	var toolCalls atomic.Int32
	var pollCalls atomic.Int32
	var tokenExchangeCalls atomic.Int32
	var server *httptest.Server
	challenge := map[string]any{
		"success": false,
		"code":    "AGENT_CODE_NOT_EXISTS",
		"data": map[string]any{
			"flowId":              "flow-dry-run",
			"clientId":            "server-assigned-client",
			"pollIntervalSeconds": 1,
			"opaque":              "preserve-me",
		},
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/cli/oauth/device/poll"):
			pollCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"status": authpkg.StatusApproved, "authCode": "dry-run-code"},
			})
			return
		case strings.Contains(r.URL.Path, "token") || strings.Contains(r.URL.Path, "Token"):
			tokenExchangeCalls.Add(1)
			http.Error(w, "dry-run must not exchange tokens", http.StatusInternalServerError)
			return
		}

		toolCalls.Add(1)
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.Method != "tools/call" || request.Params.Name != "pat.batch_plan" || request.Params.Arguments["dryRun"] != true {
			t.Errorf("unexpected real request: %#v", request)
		}
		challenge["data"].(map[string]any)["uri"] = server.URL + "/authorize"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"content": challenge,
				"isError": true,
			},
		})
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(server.URL), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	_ = authpkg.EnsureExists(configDir)
	_ = resolveIdentityHeaders() // pre-seed any detected per-agent identity before the snapshot
	if _, err := patpkg.SetBrowserPolicy(configDir, "", true); err != nil {
		t.Fatalf("enable browser policy: %v", err)
	}

	var browserCalls atomic.Int32
	originalOpenBrowser := openBrowserFunc
	openBrowserFunc = func(string) error {
		browserCalls.Add(1)
		return nil
	}
	t.Cleanup(func() { openBrowserFunc = originalOpenBrowser })

	configBefore := snapshotTestDirectory(t, configDir)
	keychainDir := os.Getenv(keychain.StorageDirEnv)
	keychainBefore := snapshotTestDirectory(t, keychainDir)

	flags := &GlobalFlags{DryRun: true, Format: "table", Token: "test-token", Timeout: 4}
	runner := &runtimeRunner{
		loader:      cli.StaticLoader{},
		transport:   transport.NewClient(server.Client()),
		globalFlags: flags,
		fallback:    executor.EchoRunner{},
		auditSink:   audit.NopSink{},
	}
	caller := newToolCallerAdapter(runner, flags)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	result, err := caller.CallTool(ctx, "pat", "pat.batch_plan", map[string]any{"dryRun": true})
	if err != nil {
		t.Fatalf("read-only PAT dry-run challenge error = %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("challenge content = %#v, want one untouched block", result.Content)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatalf("decode returned challenge: %v; raw=%q", err, result.Content[0].Text)
	}
	wantJSON, _ := json.Marshal(challenge)
	var want map[string]any
	_ = json.Unmarshal(wantJSON, &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("returned challenge changed\ngot:  %#v\nwant: %#v", got, want)
	}

	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("tools/call count = %d, want exactly 1", got)
	}
	if got := browserCalls.Load(); got != 0 {
		t.Fatalf("browser open count = %d, want 0", got)
	}
	if got := pollCalls.Load(); got != 0 {
		t.Fatalf("PAT poll count = %d, want 0", got)
	}
	if got := tokenExchangeCalls.Load(); got != 0 {
		t.Fatalf("token exchange count = %d, want 0", got)
	}
	if after := snapshotTestDirectory(t, configDir); !reflect.DeepEqual(after, configBefore) {
		t.Fatalf("config directory changed during read-only dry-run\nbefore: %#v\nafter:  %#v", configBefore, after)
	}
	if after := snapshotTestDirectory(t, keychainDir); !reflect.DeepEqual(after, keychainBefore) {
		t.Fatalf("token keychain changed during read-only dry-run\nbefore: %#v\nafter:  %#v", keychainBefore, after)
	}
}

func TestCrossPlatformCoverageToolCallerAdapter_ReadOnlyDryRunSkipsCredentialAndClassifierHooks(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
	t.Setenv("DWS_AUDIT", "0")
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	var toolCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toolCalls.Add(1)
		if got := r.Header.Get("claw-type"); got != "strict-preview-claw" {
			t.Errorf("claw-type = %q, want strict-preview-claw", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"content": map[string]any{
					"success": false,
					"code":    "AUTH_TOKEN_EXPIRED",
					"opaque":  "preserve-without-classification",
				},
				"isError": true,
			},
		})
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

	var mergeCalls atomic.Int32
	var enterpriseCredentialCalls atomic.Int32
	var classifierCalls atomic.Int32
	var tokenProviderCalls atomic.Int32
	var loadTokenCalls atomic.Int32
	previousEdition := edition.Get()
	edition.Override(&edition.Hooks{
		Name:          "strict-preview-hostile-hooks",
		ClawTypeValue: "strict-preview-claw",
		AuthClientID:  "must-not-reach-strict-preview-env",
		MergeHeaders: func(headers map[string]string) map[string]string {
			mergeCalls.Add(1)
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			enterpriseCredentialCalls.Add(1)
			return headers
		},
		TokenProvider: func(context.Context, func() (string, error)) (string, error) {
			tokenProviderCalls.Add(1)
			return "hook-token", nil
		},
		LoadToken: func(string) ([]byte, error) {
			loadTokenCalls.Add(1)
			return nil, context.Canceled
		},
		ClassifyToolResult: func(map[string]any) error {
			classifierCalls.Add(1)
			return context.Canceled
		},
	})
	t.Cleanup(func() { edition.Override(previousEdition) })

	flags := &GlobalFlags{DryRun: true, Format: "json", Token: "explicit-preview-token", Timeout: 4}
	runner := newCommandRunnerWithFlags(cli.StaticLoader{}, flags)
	caller := newToolCallerAdapter(runner, flags)
	result, err := caller.CallTool(context.Background(), "pat", "pat.batch_plan", map[string]any{"dryRun": true})
	if err != nil {
		t.Fatalf("strict dry-run error = %v", err)
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "preserve-without-classification") {
		t.Fatalf("strict dry-run content = %#v, want untouched server response", result.Content)
	}
	if !result.IsError {
		t.Fatal("strict dry-run lost MCP isError=true")
	}
	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("PAT transport calls = %d, want exactly 1", got)
	}
	if got := mergeCalls.Load(); got != 0 {
		t.Fatalf("MergeHeaders calls = %d, want 0", got)
	}
	if got := enterpriseCredentialCalls.Load(); got != 0 {
		t.Fatalf("EnterpriseCredentialHeaders calls = %d, want 0", got)
	}
	if got := classifierCalls.Load(); got != 0 {
		t.Fatalf("ClassifyToolResult calls = %d, want 0", got)
	}
	if got := tokenProviderCalls.Load(); got != 0 {
		t.Fatalf("TokenProvider calls = %d, want 0", got)
	}
	if got := loadTokenCalls.Load(); got != 0 {
		t.Fatalf("LoadToken calls = %d, want 0 with explicit --token", got)
	}
	if got := os.Getenv("DWS_CLIENT_ID"); got != "" {
		t.Fatalf("DWS_CLIENT_ID = %q, want unchanged empty value", got)
	}
}

func TestCrossPlatformCoveragePreparseExplicitTokenFlag(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "separate value", args: []string{"pat", "chmod", "--token", "explicit"}, want: true},
		{name: "equals value", args: []string{"--token=explicit", "pat", "chmod"}, want: true},
		{name: "empty separate value", args: []string{"--token", ""}},
		{name: "empty equals value", args: []string{"--token="}},
		{name: "not supplied", args: []string{"pat", "chmod", "--dry-run"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := preparseExplicitTokenFlag(tt.args); got != tt.want {
				t.Fatalf("preparseExplicitTokenFlag(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageRootPATStrictDryRunPreservesRawErrorsWithoutCredentialSideEffects(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		tool    string
		raw     string
		isError bool
	}{
		{
			name:    "protocol isError batch challenge",
			command: []string{"pat", "chmod", "--all", "--dry-run", "--format", "json"},
			tool:    "pat.batch_plan",
			raw:     `{"success":false,"code":"AGENT_CODE_NOT_EXISTS","data":{"flowId":"flow-raw","uri":"https://example.test/authorize"}}`,
			isError: true,
		},
		{
			name:    "structured success false revoke challenge",
			command: []string{"pat", "chmod", "calendar.event:read", "--revoke", "--dry-run", "--format", "json"},
			tool:    "pat.scope_revoke",
			raw:     `{"success":false,"code":"PAT_REVOKE_REQUIRES_CONFIRMATION","data":{"scope":"calendar.event:read","opaque":"preserve-me"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
			t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
			t.Setenv("DWS_AUDIT", "0")
			t.Setenv("DWS_CLIENT_ID", "")
			t.Setenv("DWS_CLIENT_SECRET", "")
			t.Setenv(keychain.DisableKeychainEnv, "1")

			rootDir := t.TempDir()
			configDir := filepath.Join(rootDir, "config")
			keychainDir := filepath.Join(rootDir, "keychain")
			t.Setenv("DWS_CONFIG_DIR", configDir)
			t.Setenv(keychain.StorageDirEnv, keychainDir)

			var toolCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				toolCalls.Add(1)
				if got := r.Header.Get("claw-type"); got != "root-strict-preview" {
					t.Errorf("claw-type = %q, want root-strict-preview", got)
				}
				var request struct {
					ID     int `json:"id"`
					Params struct {
						Name string `json:"name"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if request.Params.Name != tt.tool {
					t.Errorf("tool = %q, want %q", request.Params.Name, tt.tool)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"result": map[string]any{
						"content": []map[string]any{{"type": "text", "text": tt.raw}},
						"isError": tt.isError,
					},
				})
			}))
			defer server.Close()
			t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

			var mergeCalls atomic.Int32
			var enterpriseCalls atomic.Int32
			var tokenProviderCalls atomic.Int32
			var loadTokenCalls atomic.Int32
			var classifierCalls atomic.Int32
			markHook := func(name string) {
				_ = os.MkdirAll(configDir, 0o700)
				_ = os.WriteFile(filepath.Join(configDir, "unexpected-hook-"+name), []byte("called"), 0o600)
			}
			previousEdition := edition.Get()
			edition.Override(&edition.Hooks{
				Name:          "root-strict-hostile",
				ClawTypeValue: "root-strict-preview",
				AuthClientID:  "must-not-reach-root-preview-env",
				MergeHeaders: func(headers map[string]string) map[string]string {
					mergeCalls.Add(1)
					markHook("merge")
					return headers
				},
				EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
					enterpriseCalls.Add(1)
					markHook("enterprise")
					return headers
				},
				TokenProvider: func(context.Context, func() (string, error)) (string, error) {
					tokenProviderCalls.Add(1)
					markHook("token-provider")
					return "unexpected-hook-token", nil
				},
				LoadToken: func(string) ([]byte, error) {
					loadTokenCalls.Add(1)
					markHook("load-token")
					return nil, context.Canceled
				},
				ClassifyToolResult: func(map[string]any) error {
					classifierCalls.Add(1)
					markHook("classifier")
					return context.Canceled
				},
			})
			t.Cleanup(func() { edition.Override(previousEdition) })

			explicitToken := "root-explicit-preview-token"
			commandArgs := append([]string{"--token", explicitToken}, tt.command...)
			previousArgs := os.Args
			os.Args = append([]string{"dws"}, commandArgs...)
			t.Cleanup(func() { os.Args = previousArgs })

			configBefore := snapshotTestDirectory(t, configDir)
			keychainBefore := snapshotTestDirectory(t, keychainDir)
			output, err := executeRootCaptureStdout(t, commandArgs)
			if err != nil {
				t.Fatalf("root strict dry-run error = %v; output=%q", err, output)
			}
			if output != tt.raw {
				t.Fatalf("raw challenge changed\ngot:  %q\nwant: %q", output, tt.raw)
			}
			if got := toolCalls.Load(); got != 1 {
				t.Fatalf("tools/call count = %d, want exactly 1", got)
			}
			for name, calls := range map[string]int32{
				"MergeHeaders":                mergeCalls.Load(),
				"EnterpriseCredentialHeaders": enterpriseCalls.Load(),
				"TokenProvider":               tokenProviderCalls.Load(),
				"LoadToken":                   loadTokenCalls.Load(),
				"ClassifyToolResult":          classifierCalls.Load(),
			} {
				if calls != 0 {
					t.Errorf("%s calls = %d, want 0", name, calls)
				}
			}
			if got := os.Getenv("DWS_CLIENT_ID"); got != "" {
				t.Errorf("DWS_CLIENT_ID = %q, want unchanged empty value", got)
			}
			if got := os.Getenv("DWS_CLIENT_SECRET"); got != "" {
				t.Errorf("DWS_CLIENT_SECRET = %q, want unchanged empty value", got)
			}
			for _, name := range []string{"identity.json", "profiles.json", "token.json"} {
				if _, err := os.Stat(filepath.Join(configDir, name)); !os.IsNotExist(err) {
					t.Errorf("strict root entry created %s: %v", name, err)
				}
			}
			if after := snapshotTestDirectory(t, keychainDir); !reflect.DeepEqual(after, keychainBefore) {
				t.Errorf("keychain changed during root strict dry-run\nbefore: %#v\nafter:  %#v", keychainBefore, after)
			}
			for path := range snapshotTestDirectory(t, configDir) {
				if strings.HasPrefix(filepath.Base(path), "unexpected-hook-") {
					t.Errorf("credential/header hook wrote %s; before=%#v", path, configBefore)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageToolCallerAdapter_NormalExecutionKeepsCredentialAndHeaderHooks(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
	t.Setenv("DWS_AUDIT", "0")
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	authpkg.SetClientID("")
	authpkg.SetClientSecret("")
	t.Cleanup(func() {
		authpkg.SetClientID("")
		authpkg.SetClientSecret("")
	})

	var toolCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toolCalls.Add(1)
		if got := r.Header.Get("x-test-merge"); got != "yes" {
			t.Errorf("x-test-merge = %q, want yes", got)
		}
		if got := r.Header.Get("x-test-enterprise"); got != "yes" {
			t.Errorf("x-test-enterprise = %q, want yes", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]any{"content": map[string]any{"success": true}},
		})
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

	var mergeCalls atomic.Int32
	var enterpriseCredentialCalls atomic.Int32
	var mergeSawClientID atomic.Bool
	previousEdition := edition.Get()
	edition.Override(&edition.Hooks{
		Name:         "normal-hook-regression",
		AuthClientID: "normal-client-id",
		MergeHeaders: func(headers map[string]string) map[string]string {
			mergeCalls.Add(1)
			mergeSawClientID.Store(os.Getenv("DWS_CLIENT_ID") == "normal-client-id")
			headers["x-test-merge"] = "yes"
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			enterpriseCredentialCalls.Add(1)
			headers["x-test-enterprise"] = "yes"
			return headers
		},
	})
	t.Cleanup(func() { edition.Override(previousEdition) })

	flags := &GlobalFlags{Format: "json", Token: "normal-execution-token", Timeout: 4}
	runner := newCommandRunnerWithFlags(cli.StaticLoader{}, flags)
	caller := newToolCallerAdapter(runner, flags)
	if _, err := caller.CallTool(context.Background(), "pat", "pat.batch_plan", map[string]any{"dryRun": true}); err != nil {
		t.Fatalf("normal execution error = %v", err)
	}
	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("PAT transport calls = %d, want exactly 1", got)
	}
	if got := mergeCalls.Load(); got != 1 {
		t.Fatalf("MergeHeaders calls = %d, want exactly 1", got)
	}
	if got := enterpriseCredentialCalls.Load(); got != 1 {
		t.Fatalf("EnterpriseCredentialHeaders calls = %d, want exactly 1", got)
	}
	if !mergeSawClientID.Load() {
		t.Fatal("MergeHeaders ran before DWS_CLIENT_ID was populated on the normal path")
	}
}

func TestCrossPlatformCoverageToolCallerAdapter_ReadOnlyDryRunExpiredStoredTokenDoesNotRefresh(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
	t.Setenv("DWS_AUDIT", "1")
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(authpkg.AgentCodeEnv, "strict-preview-test")

	var refreshCalls atomic.Int32
	var toolCalls atomic.Int32
	refreshAttempted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == authpkg.MCPRefreshTokenPath {
			refreshCalls.Add(1)
			select {
			case refreshAttempted <- struct{}{}:
			default:
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode refresh request: %v", err)
			}
			if request["refreshToken"] != "still-valid-refresh" {
				t.Errorf("refreshToken = %#v, want stored refresh token", request["refreshToken"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "refreshed-access",
				"refreshToken": "rotated-refresh",
				"expiresIn":    7200,
				"corpId":       "corp-expired",
			})
			return
		}

		toolCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]any{"content": map[string]any{"preview": true}},
		})
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	keychainDir := filepath.Join(root, "keychain")
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv(keychain.StorageDirEnv, keychainDir)
	t.Setenv("DWS_AUDIT_DIR", filepath.Join(root, "constructor-audit"))
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(server.URL), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	if err := authpkg.SaveTokenData(configDir, &authpkg.TokenData{
		AccessToken:  "expired-access",
		RefreshToken: "still-valid-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		RefreshExpAt: time.Now().Add(time.Hour),
		CorpID:       "corp-expired",
		ClientID:     "client-expired",
		Source:       "mcp",
	}); err != nil {
		t.Fatalf("save expired token fixture: %v", err)
	}
	ResetRuntimeTokenCache()
	t.Cleanup(ResetRuntimeTokenCache)
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })
	pluginAuthMu.Lock()
	previousPluginAuth, hadPluginAuth := pluginAuthRegistry[defaultPATProductID]
	pluginAuthRegistry[defaultPATProductID] = &PluginAuth{Token: "plugin-token-must-not-bypass-strict-auth"}
	pluginAuthMu.Unlock()
	t.Cleanup(func() {
		pluginAuthMu.Lock()
		defer pluginAuthMu.Unlock()
		if hadPluginAuth {
			pluginAuthRegistry[defaultPATProductID] = previousPluginAuth
		} else {
			delete(pluginAuthRegistry, defaultPATProductID)
		}
	})

	configBefore := snapshotTestDirectory(t, configDir)
	keychainBefore := snapshotTestDirectory(t, keychainDir)
	flags := &GlobalFlags{DryRun: true, Format: "json", Timeout: 4}
	runner := newCommandRunnerWithFlags(cli.StaticLoader{}, flags)
	auditDir := filepath.Join(configDir, "audit")
	attachStrictPreviewAuditSink(t, runner, auditDir)
	resetAuditIdentityCache()
	t.Cleanup(resetAuditIdentityCache)
	caller := newToolCallerAdapter(runner, flags)

	_, err := caller.CallTool(context.Background(), "pat", "pat.batch_plan", map[string]any{"dryRun": true})
	if err == nil {
		t.Fatal("expired-token strict dry-run error = nil, want authentication error")
	}
	if !strings.Contains(err.Error(), "过期") || !strings.Contains(err.Error(), "不会自动刷新") {
		t.Fatalf("expired-token error = %q, want clear no-refresh authentication guidance", err)
	}
	select {
	case <-refreshAttempted:
		t.Fatal("OAuth refresh was attempted asynchronously after strict dry-run returned")
	case <-time.After(250 * time.Millisecond):
		// The ordinary runner prefetch reaches the isolated file-backed token
		// immediately; this no-call window proves the strict path never started it.
	}
	if got := refreshCalls.Load(); got != 0 {
		t.Fatalf("OAuth refresh calls = %d, want 0", got)
	}
	if got := toolCalls.Load(); got != 0 {
		t.Fatalf("PAT transport calls = %d, want 0", got)
	}
	if after := snapshotWithoutAudit(snapshotTestDirectory(t, configDir)); !reflect.DeepEqual(after, snapshotWithoutAudit(configBefore)) {
		t.Fatalf("config/profile files changed during expired-token strict dry-run\nbefore: %#v\nafter:  %#v", configBefore, after)
	}
	if after := snapshotTestDirectory(t, keychainDir); !reflect.DeepEqual(after, keychainBefore) {
		t.Fatalf("keychain changed during expired-token strict dry-run\nbefore: %#v\nafter:  %#v", keychainBefore, after)
	}
	assertStrictPreviewAuditEvent(t, auditDir, "error")
	assertAuditIdentityCacheUnchanged(t)
}

func TestCrossPlatformCoverageToolCallerAdapter_ReadOnlyDryRunDoesNotCreateIdentity(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
	t.Setenv("DWS_AUDIT", "1")
	t.Setenv("DWS_CLIENT_ID", "")
	t.Setenv("DWS_CLIENT_SECRET", "")
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(authpkg.AgentCodeEnv, "strict-preview-test")

	var toolCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toolCalls.Add(1)
		if got := r.Header.Get("x-dingtalk-dws-agent-code"); got != "strict-preview-test" {
			t.Errorf("agent-code header = %q, want strict-preview-test", got)
		}
		if got := r.Header.Get("x-dws-agent-instance-id"); got != "" {
			t.Errorf("agent instance header = %q, want empty without persisted identity", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]any{"content": map[string]any{"preview": true}},
		})
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)

	root := t.TempDir()
	configDir := filepath.Join(root, "empty-config")
	keychainDir := filepath.Join(root, "empty-keychain")
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv(keychain.StorageDirEnv, keychainDir)
	t.Setenv("DWS_AUDIT_DIR", filepath.Join(root, "constructor-audit"))
	authpkg.SetClientID("")
	authpkg.SetClientSecret("")
	t.Cleanup(func() {
		authpkg.SetClientID("")
		authpkg.SetClientSecret("")
	})
	appCredentials := &authpkg.AppConfig{
		ClientID:     "persisted-preview-client",
		ClientSecret: authpkg.PlainSecret("persisted-preview-secret"),
	}
	if err := authpkg.SaveAppConfig(configDir, appCredentials); err != nil {
		t.Fatalf("save app credentials fixture: %v", err)
	}
	appCredentials = authpkg.GetCachedAppConfig(configDir)
	if appCredentials == nil {
		t.Fatal("cached app credentials fixture = nil")
	}
	t.Cleanup(func() { _ = authpkg.DeleteAppConfig(configDir) })
	if err := authpkg.SaveTokenData(configDir, &authpkg.TokenData{
		AccessToken: "persisted-profile-token",
		ExpiresAt:   time.Now().Add(time.Hour),
		UserID:      "persisted-profile-user",
		UserName:    "Persisted Profile User",
		CorpID:      "persisted-profile-corp",
		CorpName:    "Persisted Profile Corp",
	}); err != nil {
		t.Fatalf("save persisted profile fixture: %v", err)
	}
	configBefore := snapshotTestDirectory(t, configDir)
	keychainBefore := snapshotTestDirectory(t, keychainDir)

	flags := &GlobalFlags{DryRun: true, Format: "json", Token: "explicit-preview-token", Timeout: 4}
	runner := newCommandRunnerWithFlags(cli.StaticLoader{}, flags)
	auditDir := filepath.Join(configDir, "audit")
	attachStrictPreviewAuditSink(t, runner, auditDir)
	resetAuditIdentityCache()
	t.Cleanup(resetAuditIdentityCache)
	caller := newToolCallerAdapter(runner, flags)
	if _, err := caller.CallTool(context.Background(), "pat", "pat.batch_plan", map[string]any{"dryRun": true}); err != nil {
		t.Fatalf("strict dry-run with explicit token error = %v", err)
	}
	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("PAT transport calls = %d, want exactly 1", got)
	}
	if after := snapshotWithoutAudit(snapshotTestDirectory(t, configDir)); !reflect.DeepEqual(after, snapshotWithoutAudit(configBefore)) {
		t.Fatalf("empty config tree changed during strict dry-run\nbefore: %#v\nafter:  %#v", configBefore, after)
	}
	if after := snapshotTestDirectory(t, keychainDir); !reflect.DeepEqual(after, keychainBefore) {
		t.Fatalf("empty keychain tree changed during strict dry-run\nbefore: %#v\nafter:  %#v", keychainBefore, after)
	}
	if got := os.Getenv("DWS_CLIENT_ID"); got != "" {
		t.Fatalf("DWS_CLIENT_ID = %q after strict dry-run, want unchanged empty value", got)
	}
	// SaveAppConfig keeps this pointer in the app-config cache. Mutating it lets
	// the next resolver call distinguish an untouched resolved-credential cache
	// from one populated by root construction/strict preview.
	appCredentials.ClientID = "post-preview-client"
	appCredentials.ClientSecret = authpkg.PlainSecret("post-preview-secret")
	clientID, clientSecret := authpkg.ResolveAppCredentials(configDir)
	if clientID != "post-preview-client" || clientSecret != "post-preview-secret" {
		t.Fatalf("resolved app credential cache was populated during strict dry-run: id=%q secret=%q", clientID, clientSecret)
	}
	event := strictPreviewAuditEvent(t, auditDir, "success")
	if event.Actor != (audit.Actor{}) {
		t.Fatalf("explicit-token strict preview actor = %+v, want empty persisted-profile attribution", event.Actor)
	}
	assertAuditIdentityCacheUnchanged(t)
}

func TestCrossPlatformCoverageToolCallerAdapter_ReadOnlyDryRunMakesOneTransportAttempt(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "temporary failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("DINGTALK_PAT_MCP_URL", server.URL)
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	flags := &GlobalFlags{DryRun: true, Token: "test-token", Timeout: 4}
	runner := &runtimeRunner{
		loader:      cli.StaticLoader{},
		transport:   transport.NewClient(server.Client()),
		globalFlags: flags,
		fallback:    executor.EchoRunner{},
		auditSink:   audit.NopSink{},
	}
	caller := newToolCallerAdapter(runner, flags)
	if _, err := caller.CallTool(context.Background(), "pat", "pat.batch_plan", map[string]any{"dryRun": true}); err == nil {
		t.Fatal("read-only dry-run transport error = nil, want original failure")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport attempts = %d, want exactly 1", got)
	}
}

func TestCrossPlatformCoverageRuntimeRunnerStrictPATDryRunRejectsMultipleProfiles(t *testing.T) {
	previousProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("corp-a,corp-b")
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })

	invocation := executor.NewHelperInvocation(
		"overlay.pat.pat.batch_plan",
		defaultPATProductID,
		"pat.batch_plan",
		map[string]any{"dryRun": true},
	)
	invocation.DryRun = true
	invocation.AllowReadOnlyDuringDryRun = true
	_, err := (&runtimeRunner{}).Run(context.Background(), invocation)
	if err == nil || !strings.Contains(err.Error(), "exactly one --profile") {
		t.Fatalf("Run() error = %v, want strict single-profile rejection", err)
	}
}

func TestCrossPlatformCoverageRuntimeRunnerDryRunBoundaryHelpers(t *testing.T) {
	var nilRunner *runtimeRunner
	nilRunner.stampGlobalDryRun(nil)

	runner := &runtimeRunner{
		globalFlags: &GlobalFlags{DryRun: true},
		fallback:    executor.EchoRunner{},
	}
	invocation := executor.NewHelperInvocation("test", "missing-product", "missing.tool", map[string]any{"id": "1"})
	result, err := runner.handleCatalogMiss(context.Background(), invocation, "not found")
	if err != nil {
		t.Fatalf("handleCatalogMiss() error = %v", err)
	}
	if result.Response["dry_run"] != true {
		t.Fatalf("handleCatalogMiss() response = %#v, want dry-run echo", result.Response)
	}

	stdioResult, err := runner.executeStdioInvocation(context.Background(), invocation)
	if err != nil {
		t.Fatalf("executeStdioInvocation() error = %v", err)
	}
	if stdioResult.Response["transport"] != "stdio" || stdioResult.Response["dry_run"] != true {
		t.Fatalf("executeStdioInvocation() response = %#v", stdioResult.Response)
	}
}

func TestCrossPlatformCoverageRuntimeRunnerExecuteInvocationDryRunUsesPluginCredentialPath(t *testing.T) {
	const product = "coverage-plugin"
	pluginAuthMu.Lock()
	previous, existed := pluginAuthRegistry[product]
	pluginAuthRegistry[product] = &PluginAuth{Token: "plugin-token"}
	pluginAuthMu.Unlock()
	t.Cleanup(func() {
		pluginAuthMu.Lock()
		defer pluginAuthMu.Unlock()
		if existed {
			pluginAuthRegistry[product] = previous
		} else {
			delete(pluginAuthRegistry, product)
		}
	})

	runner := &runtimeRunner{
		transport:   transport.NewClient(http.DefaultClient),
		globalFlags: &GlobalFlags{DryRun: true},
	}
	invocation := executor.NewHelperInvocation("test", product, "coverage.tool", map[string]any{"id": "1"})
	result, err := runner.executeInvocation(context.Background(), "https://example.invalid/mcp", invocation)
	if err != nil {
		t.Fatalf("executeInvocation() error = %v", err)
	}
	if result.Response["dry_run"] != true {
		t.Fatalf("executeInvocation() response = %#v, want dry-run preview", result.Response)
	}
}

func TestCrossPlatformCoverageResolveReadOnlyIdentityHeadersUsesPersistedAgentEntry(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv(authpkg.AgentCodeEnv, "coverage-agent")
	identity := authpkg.Identity{
		AgentID:   "machine-id",
		MachineID: "machine-id",
		Source:    "dws",
		Agents: map[string]*authpkg.AgentEntry{
			"coverage-agent": {AgentID: "persisted-agent-id"},
		},
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("json.Marshal(identity) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "identity.json"), encoded, 0o600); err != nil {
		t.Fatalf("write identity.json: %v", err)
	}

	headers := resolveReadOnlyIdentityHeaders()
	if got := headers["x-dws-agent-instance-id"]; got != "persisted-agent-id" {
		t.Fatalf("x-dws-agent-instance-id = %q, want persisted-agent-id", got)
	}
}

func TestCrossPlatformCoverageEnsureRuntimeClientIDEnvCoversExistingAndResolvedValues(t *testing.T) {
	t.Setenv("DWS_CLIENT_ID", "existing-client")
	ensureRuntimeClientIDEnv()
	if got := os.Getenv("DWS_CLIENT_ID"); got != "existing-client" {
		t.Fatalf("existing DWS_CLIENT_ID = %q", got)
	}

	authpkg.SetClientID("resolved-client")
	t.Cleanup(func() { authpkg.SetClientID("") })
	t.Setenv("DWS_CLIENT_ID", "")
	ensureRuntimeClientIDEnv()
	if got := os.Getenv("DWS_CLIENT_ID"); got != "resolved-client" {
		t.Fatalf("resolved DWS_CLIENT_ID = %q, want resolved-client", got)
	}
}

func TestCrossPlatformCoverageConvertResultCoversSliceAndScalarContent(t *testing.T) {
	sliceResult := convertResult(executor.Result{Response: map[string]any{
		"is_error": true,
		"content": []any{
			map[string]any{"type": "text", "text": "server text"},
			"ignored",
		},
	}})
	if !sliceResult.IsError || len(sliceResult.Content) != 1 || sliceResult.Content[0].Text != "server text" {
		t.Fatalf("slice result = %#v", sliceResult)
	}

	scalarResult := convertResult(executor.Result{Response: map[string]any{"content": 42}})
	if len(scalarResult.Content) != 1 || scalarResult.Content[0].Text != "42" {
		t.Fatalf("scalar result = %#v", scalarResult)
	}
}

func TestCrossPlatformCoverageAuditIdentityReadOnlyReportsTokenErrorAndLoadsPersistedIdentity(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	previousProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("missing-profile")
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })

	identity := authpkg.Identity{AgentID: "audit-agent-id", MachineID: "audit-agent-id", Source: "dws"}
	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("json.Marshal(identity) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "identity.json"), encoded, 0o600); err != nil {
		t.Fatalf("write identity.json: %v", err)
	}

	actor, agentID := auditIdentityReadOnly(false)
	if actor != (audit.Actor{}) {
		t.Fatalf("actor = %+v, want empty after read-only token failure", actor)
	}
	if agentID != "audit-agent-id" {
		t.Fatalf("agentID = %q, want audit-agent-id", agentID)
	}
}

func attachStrictPreviewAuditSink(t *testing.T, runner executor.Runner, auditDir string) {
	t.Helper()
	runtime, ok := runner.(*runtimeRunner)
	if !ok {
		t.Fatalf("runner type = %T, want *runtimeRunner", runner)
	}
	writer, err := audit.NewDateRotatingWriter(auditDir, 0)
	if err != nil {
		t.Fatalf("create strict-preview audit writer: %v", err)
	}
	sink := audit.NewFileSink(writer, audit.NewChain(auditDir), nil)
	runtime.auditSink = sink
	t.Cleanup(func() {
		if err := sink.Close(); err != nil {
			t.Errorf("close strict-preview audit sink: %v", err)
		}
	})
}

func snapshotWithoutAudit(snapshot map[string]string) map[string]string {
	filtered := make(map[string]string, len(snapshot))
	for path, content := range snapshot {
		if strings.HasPrefix(filepath.ToSlash(path), "audit/") {
			continue
		}
		filtered[path] = content
	}
	return filtered
}

func assertStrictPreviewAuditEvent(t *testing.T, auditDir, result string) {
	t.Helper()
	_ = strictPreviewAuditEvent(t, auditDir, result)
}

func strictPreviewAuditEvent(t *testing.T, auditDir, result string) audit.Event {
	t.Helper()
	for _, content := range snapshotTestDirectory(t, auditDir) {
		for _, line := range strings.Split(content, "\n") {
			var event audit.Event
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			if event.Product == "pat" && event.Command == "pat.batch_plan" && event.Result == result {
				return event
			}
		}
	}
	t.Fatalf("strict-preview audit event with result %q was not emitted in %s", result, auditDir)
	return audit.Event{}
}

func assertAuditIdentityCacheUnchanged(t *testing.T) {
	t.Helper()
	auditIDMu.Lock()
	defer auditIDMu.Unlock()
	if identityLoaded || cachedProfile != "" || cachedAgentID != "" || cachedActor != (audit.Actor{}) {
		t.Fatalf("strict-preview audit populated normal actor cache: loaded=%t profile=%q agent=%q actor=%+v", identityLoaded, cachedProfile, cachedAgentID, cachedActor)
	}
}

func snapshotTestDirectory(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	if strings.TrimSpace(root) == "" {
		return snapshot
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[relative] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot directory %q: %v", root, err)
	}
	return snapshot
}
