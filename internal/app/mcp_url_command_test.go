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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type mcpURLTestCaller struct {
	productID string
	toolName  string
	args      map[string]any
	result    *edition.ToolResult
	err       error
}

func (c *mcpURLTestCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.productID = productID
	c.toolName = toolName
	c.args = args
	return c.result, c.err
}

func (*mcpURLTestCaller) Format() string { return "json" }
func (*mcpURLTestCaller) DryRun() bool   { return false }
func (*mcpURLTestCaller) Fields() string { return "" }
func (*mcpURLTestCaller) JQ() string     { return "" }

func executeMCPURLCommand(t *testing.T, caller edition.ToolCaller, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "mcp", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newMCPURLGroup(caller))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func TestMCPURLGetCallsMetaServerAndPreservesResponse(t *testing.T) {
	const response = `{"result":{"mcpURL":"https://example.test/mcp?key=one&token=two","mcpJSON":{"transport":"streamable-http"},"name":"Example"}}`
	caller := &mcpURLTestCaller{
		result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: response}}},
	}

	out, err := executeMCPURLCommand(t, caller, "url", "get", " 10043 ")
	if err != nil {
		t.Fatalf("execute mcp url get: %v", err)
	}
	if caller.productID != mcpMetaServerID {
		t.Fatalf("productID = %q, want %q", caller.productID, mcpMetaServerID)
	}
	if caller.toolName != mcpMetaURLTool {
		t.Fatalf("toolName = %q, want %q", caller.toolName, mcpMetaURLTool)
	}
	if got := caller.args["mcpId"]; got != "10043" {
		t.Fatalf("mcpId = %#v, want %q", got, "10043")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("output result = %#v", payload["result"])
	}
	if got := result["mcpURL"]; got != "https://example.test/mcp?key=one&token=two" {
		t.Fatalf("result.mcpURL = %#v", got)
	}
}

func TestMCPURLGetRejectsBlankID(t *testing.T) {
	_, err := executeMCPURLCommand(t, &mcpURLTestCaller{}, "url", "get", "   ")
	if err == nil || !strings.Contains(err.Error(), "mcpId 不能为空") {
		t.Fatalf("error = %v, want blank mcpId error", err)
	}
}

func TestMCPURLGroupShowsHelp(t *testing.T) {
	out, err := executeMCPURLCommand(t, nil, "url")
	if err != nil {
		t.Fatalf("execute mcp url: %v", err)
	}
	if !strings.Contains(out, "get") {
		t.Fatalf("help output does not list get command:\n%s", out)
	}
}

func TestMCPURLGetRejectsMissingCaller(t *testing.T) {
	_, err := executeMCPURLCommand(t, nil, "url", "get", "10043")
	if err == nil || !strings.Contains(err.Error(), "caller is not configured") {
		t.Fatalf("error = %v, want missing caller error", err)
	}
}

func TestMCPURLGetPropagatesCallError(t *testing.T) {
	caller := &mcpURLTestCaller{err: errors.New("permission denied")}
	_, err := executeMCPURLCommand(t, caller, "url", "get", "10043")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error = %v, want call error", err)
	}
}

func TestMCPURLGetRejectsInvalidJSON(t *testing.T) {
	caller := &mcpURLTestCaller{
		result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: "not-json"}}},
	}
	_, err := executeMCPURLCommand(t, caller, "url", "get", "10043")
	if err == nil || !strings.Contains(err.Error(), "无效 JSON") {
		t.Fatalf("error = %v, want invalid JSON error", err)
	}
}

func TestMCPURLGetRejectsEmptyResults(t *testing.T) {
	tests := []struct {
		name   string
		result *edition.ToolResult
	}{
		{name: "nil result"},
		{
			name: "no usable text content",
			result: &edition.ToolResult{Content: []edition.ContentBlock{
				{Type: "image", Text: "ignored"},
				{Type: "text", Text: "   "},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &mcpURLTestCaller{result: tt.result}
			_, err := executeMCPURLCommand(t, caller, "url", "get", "10043")
			if err == nil || !strings.Contains(err.Error(), "返回空结果") {
				t.Fatalf("error = %v, want empty result error", err)
			}
		})
	}
}

func TestMCPURLGetClassifiesBusinessError(t *testing.T) {
	caller := &mcpURLTestCaller{
		result: &edition.ToolResult{Content: []edition.ContentBlock{{
			Type: "text",
			Text: `{"success":false,"errorMsg":"搜索内容不能为空"}`,
		}}},
	}
	_, err := executeMCPURLCommand(t, caller, "url", "get", "10043")
	if err == nil {
		t.Fatal("expected classified business error")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "business_error" {
		t.Fatalf("error = %#v, want classified business error", err)
	}
}

func TestRootRegistersMCPURLGet(t *testing.T) {
	root := NewRootCommand(t.Context())
	mcp, _, err := root.Find([]string{"mcp"})
	if err != nil {
		t.Fatalf("find mcp: %v", err)
	}
	if mcp.Hidden {
		t.Fatal("mcp command must be public when it contains reviewed public helpers")
	}
	cmd, _, err := root.Find([]string{"mcp", "url", "get"})
	if err != nil {
		t.Fatalf("find mcp url get: %v", err)
	}
	if got := cmd.CommandPath(); got != "dws mcp url get" {
		t.Fatalf("command path = %q, want %q", got, "dws mcp url get")
	}
}
