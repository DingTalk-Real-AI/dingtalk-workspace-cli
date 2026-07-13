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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestToolCallerAdapter_GlobalDryRunAllowsOnlyExactPATReadOnlyPreviews(t *testing.T) {
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
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

func TestToolCallerAdapter_ReadOnlyDryRunMarkerIsStrict(t *testing.T) {
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
