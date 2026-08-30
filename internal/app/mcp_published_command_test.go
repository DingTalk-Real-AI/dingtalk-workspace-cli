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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mcpPublishedTestTransport struct {
	listEndpoint string
	callEndpoint string
	callTool     string
	callArgs     map[string]any
	listResult   transport.ToolsListResult
	callResult   transport.ToolCallResult
	listErr      error
	callErr      error
}

func (c *mcpPublishedTestTransport) Tools(_ context.Context, endpoint string) (transport.ToolsListResult, error) {
	c.listEndpoint = endpoint
	return c.listResult, c.listErr
}

func (c *mcpPublishedTestTransport) Invoke(_ context.Context, endpoint, tool string, args map[string]any) (transport.ToolCallResult, error) {
	c.callEndpoint = endpoint
	c.callTool = tool
	c.callArgs = args
	return c.callResult, c.callErr
}

func executeMCPPublishedCommand(
	t *testing.T,
	caller edition.ToolCaller,
	client *mcpPublishedTestTransport,
	args ...string,
) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(newMCPPublishedGroup(caller, func(context.Context) (mcpPublishedTransport, error) {
		return client, nil
	}))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func publishedURLCaller() *mcpURLTestCaller {
	return &mcpURLTestCaller{
		result: &edition.ToolResult{Content: []edition.ContentBlock{{
			Type: "text",
			Text: `{"result":{"mcpURL":"https://example.test/mcp?key=secret&token=private"}}`,
		}}},
	}
}

func TestMCPPublishedToolsResolvesIdentityEndpointAndRedactsOutput(t *testing.T) {
	client := &mcpPublishedTestTransport{
		listResult: transport.ToolsListResult{Tools: []transport.ToolDescriptor{{
			Name: "search", Description: "Search records",
		}}},
	}
	caller := publishedURLCaller()
	out, err := executeMCPPublishedCommand(t, caller, client, "published", "tools", "2480")
	if err != nil {
		t.Fatalf("execute published tools: %v", err)
	}
	if caller.args["mcpId"] != "2480" {
		t.Fatalf("meta mcpId = %#v", caller.args["mcpId"])
	}
	if client.listEndpoint != "https://example.test/mcp?key=secret&token=private" {
		t.Fatalf("list endpoint = %q", client.listEndpoint)
	}
	if strings.Contains(out, "secret") || strings.Contains(out, "private") {
		t.Fatalf("output leaked endpoint credentials: %s", out)
	}
	if !strings.Contains(out, `"toolCount": 1`) {
		t.Fatalf("output missing tool count: %s", out)
	}
}

func TestMCPPublishedInvokeDryRunDoesNotResolveOrCall(t *testing.T) {
	client := &mcpPublishedTestTransport{}
	out, err := executeMCPPublishedCommand(
		t,
		nil,
		client,
		"--dry-run", "published", "invoke", "2480", "search", "--params", `{"query":"example"}`,
	)
	if err != nil {
		t.Fatalf("execute published invoke dry-run: %v", err)
	}
	if client.callEndpoint != "" {
		t.Fatalf("dry-run called endpoint %q", client.callEndpoint)
	}
	if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, `"executed": false`) {
		t.Fatalf("dry-run output missing evidence: %s", out)
	}
}

func TestMCPPublishedInvokeRequiresConfirmationBeforeResolution(t *testing.T) {
	client := &mcpPublishedTestTransport{}
	_, err := executeMCPPublishedCommand(
		t,
		nil,
		client,
		"published", "invoke", "2480", "search", "--params", `{"query":"example"}`,
	)
	if err == nil || !strings.Contains(err.Error(), "需要用户确认") {
		t.Fatalf("error = %v, want confirmation_required", err)
	}
	if client.callEndpoint != "" {
		t.Fatalf("unconfirmed invocation called endpoint %q", client.callEndpoint)
	}
}

func TestMCPPublishedInvokeConfirmedCallsSelectedTool(t *testing.T) {
	client := &mcpPublishedTestTransport{
		callResult: transport.ToolCallResult{
			StructuredContent: map[string]any{"items": []any{"one"}},
		},
	}
	out, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		client,
		"--yes", "published", "invoke", "2480", "search", "--params", `{"query":"example"}`,
	)
	if err != nil {
		t.Fatalf("execute confirmed published invoke: %v", err)
	}
	if client.callTool != "search" || client.callArgs["query"] != "example" {
		t.Fatalf("call = tool %q args %#v", client.callTool, client.callArgs)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if payload["tool"] != "search" {
		t.Fatalf("output tool = %#v", payload["tool"])
	}
}

func TestMCPPublishedInvokeRejectsNonObjectParamsBeforeConfirmation(t *testing.T) {
	_, err := executeMCPPublishedCommand(
		t,
		publishedURLCaller(),
		&mcpPublishedTestTransport{},
		"published", "invoke", "2480", "search", "--params", `["bad"]`,
	)
	if err == nil || !strings.Contains(err.Error(), "--params 必须是 JSON 对象") {
		t.Fatalf("error = %v, want params validation", err)
	}
}

func TestMCPPublishedClientConfigInheritsRuntimeTokenAndTransport(t *testing.T) {
	base := transport.NewClient(nil)
	runner := &runtimeRunner{transport: base}
	flags := &GlobalFlags{Token: " explicit-token "}

	gotBase, token, _, err := resolveMCPPublishedClientConfig(t.Context(), runner, flags)
	if err != nil {
		t.Fatalf("resolve published client config: %v", err)
	}
	if gotBase != base {
		t.Fatal("published client did not inherit the runtime transport")
	}
	if token != "explicit-token" {
		t.Fatalf("published client token = %q, want explicit-token", token)
	}
}

func TestMCPPublishedGroupHelpAndDefaultFactory(t *testing.T) {
	group := newMCPPublishedGroup(nil, nil)
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(group)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"published"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("execute group help: %v", err)
	}
	if !strings.Contains(out.String(), "tools") || !strings.Contains(out.String(), "invoke") {
		t.Fatalf("group help missing commands:\n%s", out.String())
	}
}

func TestMCPPublishedToolsErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		caller  edition.ToolCaller
		factory mcpPublishedTransportFactory
		wantErr string
	}{
		{
			name:    "endpoint resolution",
			wantErr: "caller is not configured",
		},
		{
			name:   "factory",
			caller: publishedURLCaller(),
			factory: func(context.Context) (mcpPublishedTransport, error) {
				return nil, errors.New("factory failed")
			},
			wantErr: "factory failed",
		},
		{
			name:   "transport",
			caller: publishedURLCaller(),
			factory: func(context.Context) (mcpPublishedTransport, error) {
				return &mcpPublishedTestTransport{listErr: errors.New("list failed")}, nil
			},
			wantErr: "列出已发布 MCP 工具: list failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().String("format", "json", "")
			factory := tt.factory
			if factory == nil {
				factory = func(context.Context) (mcpPublishedTransport, error) {
					return &mcpPublishedTestTransport{}, nil
				}
			}
			root.AddCommand(newMCPPublishedGroup(tt.caller, factory))
			root.SetArgs([]string{"published", "tools", "2480"})
			err := root.ExecuteContext(t.Context())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestMCPPublishedInvokeErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		caller     edition.ToolCaller
		factory    mcpPublishedTransportFactory
		callResult transport.ToolCallResult
		callErr    error
		wantErr    string
	}{
		{
			name:    "endpoint resolution",
			wantErr: "caller is not configured",
		},
		{
			name:   "factory",
			caller: publishedURLCaller(),
			factory: func(context.Context) (mcpPublishedTransport, error) {
				return nil, errors.New("factory failed")
			},
			wantErr: "factory failed",
		},
		{
			name:    "transport",
			caller:  publishedURLCaller(),
			callErr: errors.New("call failed"),
			wantErr: "调用已发布 MCP 工具: call failed",
		},
		{
			name:   "tool result",
			caller: publishedURLCaller(),
			callResult: transport.ToolCallResult{
				IsError: true,
				Blocks:  []transport.ContentBlock{{Type: "text", Text: "remote rejected"}},
			},
			wantErr: "remote rejected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := tt.factory
			if factory == nil {
				factory = func(context.Context) (mcpPublishedTransport, error) {
					return &mcpPublishedTestTransport{callResult: tt.callResult, callErr: tt.callErr}, nil
				}
			}
			root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().Bool("dry-run", false, "")
			root.PersistentFlags().Bool("yes", false, "")
			root.PersistentFlags().String("format", "json", "")
			root.AddCommand(newMCPPublishedGroup(tt.caller, factory))
			root.SetArgs([]string{"--yes", "published", "invoke", "2480", "search"})
			err := root.ExecuteContext(t.Context())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseMCPPublishedInvokeValidationEdges(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("params", "{}", "")
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "arity", args: []string{"only-one"}, wantErr: "需要提供 mcpId 和工具名"},
		{name: "blank mcp id", args: []string{" ", "search"}, wantErr: "mcpId 不能为空"},
		{name: "blank tool", args: []string{"2480", " "}, wantErr: "工具名不能为空"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseMCPPublishedInvoke(cmd, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}

	_, _, _, err := parseMCPPublishedInvoke(&cobra.Command{}, []string{"2480", "search"})
	if err == nil {
		t.Fatal("missing params flag should fail")
	}

	run := runMCPPublishedInvoke(nil, nil)
	if err := run(cmd, []string{"only-one"}); err == nil {
		t.Fatal("invoke body should propagate parse errors")
	}
}

func TestResolvePublishedMCPEndpointEdges(t *testing.T) {
	if _, err := resolvePublishedMCPEndpoint(t.Context(), nil, "2480"); err == nil {
		t.Fatal("nil caller should fail")
	}
	if _, err := resolvePublishedMCPEndpoint(t.Context(), &mcpURLTestCaller{}, " "); err == nil {
		t.Fatal("blank mcpId should fail")
	}

	tests := []struct {
		name    string
		caller  *mcpURLTestCaller
		want    string
		wantErr string
	}{
		{name: "call error", caller: &mcpURLTestCaller{err: errors.New("denied")}, wantErr: "获取 MCP 服务地址: denied"},
		{name: "nil result", caller: &mcpURLTestCaller{}, wantErr: "返回空结果"},
		{
			name: "skip unusable blocks",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{
				{Type: "image", Text: "ignored"},
				{Type: "text", Text: " "},
			}}},
			wantErr: "返回空结果",
		},
		{
			name: "business error",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"success":false,"errorMsg":"denied"}`,
			}}}},
			wantErr: "denied",
		},
		{
			name: "invalid json",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{`,
			}}}},
			wantErr: "无效 JSON",
		},
		{
			name: "missing url",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"result":{"name":"missing"}}`,
			}}}},
			wantErr: "缺少 mcpURL",
		},
		{
			name: "flat url",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"mcpURL":" https://flat.example/mcp "}`,
			}}}},
			want: "https://flat.example/mcp",
		},
		{
			name: "nested blank falls back to flat",
			caller: &mcpURLTestCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{
				Type: "text", Text: `{"mcpURL":"https://flat.example/mcp","result":{"mcpURL":" "}}`,
			}}}},
			want: "https://flat.example/mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePublishedMCPEndpoint(t.Context(), tt.caller, "2480")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("endpoint = %q, error = %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestMCPPublishedAuthenticatedFactory(t *testing.T) {
	factory := newAuthenticatedMCPPublishedTransportFactory(
		&runtimeRunner{transport: transport.NewClient(nil)},
		&GlobalFlags{Token: "factory-token"},
	)
	client, err := factory(t.Context())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if client == nil {
		t.Fatal("factory returned nil client")
	}

	if got := firstNonEmpty(" ", "\t"); got != "" {
		t.Fatalf("firstNonEmpty blanks = %q", got)
	}
}

func TestMCPPublishedAuthenticatedFactoryPropagatesTokenError(t *testing.T) {
	oldManager := runtimeTokenManager
	oldProvider := newAccessTokenProvider
	oldLegacy := newLegacyTokenManager
	runtimeTokenManager = NewTokenManager()
	newAccessTokenProvider = func(string) accessTokenGetter {
		return fakeAccessTokenGetter{err: errors.New("oauth load failed")}
	}
	newLegacyTokenManager = func(string) legacyTokenGetter {
		return fakeLegacyTokenGetter{err: errors.New("legacy load failed")}
	}
	t.Cleanup(func() {
		runtimeTokenManager = oldManager
		newAccessTokenProvider = oldProvider
		newLegacyTokenManager = oldLegacy
	})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	factory := newAuthenticatedMCPPublishedTransportFactory(nil, nil)
	if _, err := factory(t.Context()); err == nil {
		t.Fatal("factory should propagate token resolution errors")
	}
}
