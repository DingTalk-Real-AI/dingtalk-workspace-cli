// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type docDelegationCall struct {
	server string
	tool   string
	args   map[string]any
}

// docDelegationTestCaller scripts check/passthrough responses per tool name
// and records every CallTool in order.
type docDelegationTestCaller struct {
	calls    []docDelegationCall
	checkRes *edition.ToolResult
	checkErr error
	passRes  *edition.ToolResult
	passErr  error
	dry      bool
}

func (c *docDelegationTestCaller) CallTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, docDelegationCall{server: serverID, tool: toolName, args: copied})
	if toolName == checkCapTool {
		return c.checkRes, c.checkErr
	}
	return c.passRes, c.passErr
}

func (c *docDelegationTestCaller) Format() string { return "json" }
func (c *docDelegationTestCaller) DryRun() bool   { return c.dry }
func (*docDelegationTestCaller) Fields() string   { return "fields-x" }
func (*docDelegationTestCaller) JQ() string       { return "jq-x" }

// docDelegationReadTestCaller adds the optional ReadToolCaller capability.
type docDelegationReadTestCaller struct {
	*docDelegationTestCaller
	readCalls []docDelegationCall
	readRes   *edition.ToolResult
	readErr   error
}

func (c *docDelegationReadTestCaller) CallReadTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.readCalls = append(c.readCalls, docDelegationCall{server: serverID, tool: toolName, args: args})
	return c.readRes, c.readErr
}

func newDocDelegationTestCaller() *docDelegationTestCaller {
	return &docDelegationTestCaller{
		checkRes: textToolResult(`{"allowed":true}`),
		passRes:  textToolResult(`{"result":"ok"}`),
	}
}

func newDocDelegationAuthDecorator(inner edition.ToolCaller) *docDelegationAuthCaller {
	return &docDelegationAuthCaller{inner: inner, principalID: "u-principal", checked: map[string]bool{}}
}

func TestCrossPlatformCoverageDocDelegationAuthCheckSuccessFlow(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	args := map[string]any{"nodeId": "node-1", "content": "x"}
	result, err := d.CallTool(context.Background(), "doc", "update_document", args)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result != inner.passRes {
		t.Fatalf("CallTool() result = %#v, want passthrough result", result)
	}
	if len(inner.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (check, original)", len(inner.calls))
	}
	check := inner.calls[0]
	if check.server != capabilityServerID || check.tool != checkCapTool {
		t.Fatalf("call[0] = %s/%s, want %s/%s", check.server, check.tool, capabilityServerID, checkCapTool)
	}
	if check.args["userId"] != "u-principal" || check.args["mcpToolKey"] != "doc.update_document" || check.args["nodeId"] != "node-1" {
		t.Fatalf("check args = %#v", check.args)
	}
	orig := inner.calls[1]
	if orig.server != "doc" || orig.tool != "update_document" || orig.args["content"] != "x" {
		t.Fatalf("call[1] = %#v, want original passthrough", orig)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthNoNodeIDStillCallsCheck(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	if _, err := d.CallTool(context.Background(), "drive", "list_files", map[string]any{"limit": 20}); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(inner.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (check, original)", len(inner.calls))
	}
	if _, exists := inner.calls[0].args["nodeId"]; exists {
		t.Fatalf("check args should omit nodeId: %#v", inner.calls[0].args)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthDeniedWithMessage(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.checkRes = textToolResult(`{"allowed":false,"denialReason":"NO_PERM","denialMessage":"没有该文档的委托权限"}`)
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want denial error")
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI {
		t.Fatalf("error = %v, want structured API-category error", err)
	}
	if typed.Reason != "delegation_denied" {
		t.Fatalf("Reason = %q, want delegation_denied", typed.Reason)
	}
	if !strings.Contains(typed.Message, "委托鉴权未通过（委托人 u-principal）") || !strings.Contains(typed.Message, "没有该文档的委托权限") {
		t.Fatalf("Message = %q, want principal ID and denialMessage surfaced", typed.Message)
	}
	if strings.Contains(typed.Message, "doc.update_document") {
		t.Fatalf("Message = %q, must not leak internal toolKey", typed.Message)
	}
	if strings.Contains(err.Error(), "MCP_TOOL_ERROR") {
		t.Fatalf("Error() = %q, must not carry MCP_TOOL_ERROR", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
		t.Fatalf("Error() = %q, want [DELEGATION_AUTH_DENIED] prefix", err.Error())
	}
	// 守卫：CLIError 外壳必须在 WrapErrorWithOperation 直通分支原样返回，
	// 防止未来有人移除直通分支时拒绝错误被模式分类重包装成 MCP_TOOL_ERROR。
	if passthrough := WrapErrorWithOperation(err, "doc/update_document"); passthrough != err {
		t.Fatalf("WrapErrorWithOperation() = %v, want the denial error passed through unchanged", passthrough)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (original tool must not run)", len(inner.calls))
	}
	if d.checked["doc.update_document"] {
		t.Fatal("denied toolKey must not be marked checked")
	}
}

func TestCrossPlatformCoverageDocDelegationAuthDeniedFallsBackToReason(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.checkRes = textToolResult(`{"allowed":false,"denialReason":"NO_PERM","denialMessage":"  "}`)
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", nil)
	if err == nil || !strings.Contains(err.Error(), "NO_PERM") {
		t.Fatalf("error = %v, want fallback to denialReason", err)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthExtractNodeID(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"nodeId", map[string]any{"nodeId": "n1"}, "n1"},
		{"fileId", map[string]any{"fileId": "f1"}, "f1"},
		{"node_id", map[string]any{"node_id": "n2"}, "n2"},
		{"workspaceId", map[string]any{"workspaceId": "w1"}, "w1"},
		{"spaceId", map[string]any{"spaceId": "s1"}, "s1"},
		{"workspace_id", map[string]any{"workspace_id": "w2"}, "w2"},
		{"space_id", map[string]any{"space_id": "s2"}, "s2"},
		{"node beats workspace", map[string]any{"workspaceId": "w1", "fileId": "f1"}, "f1"},
		{"nodeId beats fileId", map[string]any{"fileId": "f1", "nodeId": "n1"}, "n1"},
		{"empty string skipped", map[string]any{"nodeId": "", "spaceId": "s1"}, "s1"},
		{"non-string skipped", map[string]any{"nodeId": 42, "spaceId": "s1"}, "s1"},
		{"none found", map[string]any{"other": "x"}, ""},
		{"nil args", nil, ""},
	}
	for _, tc := range cases {
		if got := extractNodeId(tc.args); got != tc.want {
			t.Fatalf("%s: extractNodeId(%#v) = %q, want %q", tc.name, tc.args, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageDocDelegationAuthDedupSameToolKey(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := d.CallTool(ctx, "doc", "update_document", map[string]any{"nodeId": "n1"}); err != nil {
			t.Fatalf("CallTool(#%d) error = %v", i, err)
		}
	}
	var checks, originals int
	for _, call := range inner.calls {
		if call.tool == checkCapTool {
			checks++
		} else {
			originals++
		}
	}
	if checks != 1 || originals != 2 {
		t.Fatalf("checks/originals = %d/%d, want 1/2", checks, originals)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthDifferentToolKeysEachChecked(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	ctx := context.Background()
	if _, err := d.CallTool(ctx, "doc", "update_document", nil); err != nil {
		t.Fatalf("CallTool(doc) error = %v", err)
	}
	if _, err := d.CallTool(ctx, "wiki", "create_wikiSpace", nil); err != nil {
		t.Fatalf("CallTool(wiki) error = %v", err)
	}
	var checkKeys []string
	for _, call := range inner.calls {
		if call.tool == checkCapTool {
			checkKeys = append(checkKeys, call.args["mcpToolKey"].(string))
		}
	}
	if len(checkKeys) != 2 || checkKeys[0] != "doc.update_document" || checkKeys[1] != "wiki.create_wikiSpace" {
		t.Fatalf("check toolKeys = %#v, want both keys checked separately", checkKeys)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthNonDocServerPassthrough(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	if _, err := d.CallTool(context.Background(), "chat", "send_message", map[string]any{"nodeId": "n1"}); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(inner.calls) != 1 || inner.calls[0].tool != "send_message" {
		t.Fatalf("calls = %#v, want direct passthrough without check", inner.calls)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthCheckCallFails(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.checkErr = errors.New("check boom")
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", nil)
	if err == nil || !strings.Contains(err.Error(), "委托鉴权校验失败") || !errors.Is(err, inner.checkErr) {
		t.Fatalf("error = %v, want wrapped check failure", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (stop after check failure)", len(inner.calls))
	}
}

func TestCrossPlatformCoverageDocDelegationAuthCheckResponseInvalid(t *testing.T) {
	cases := []struct {
		name    string
		result  *edition.ToolResult
		wantSub string
	}{
		{"nil result", nil, "nil result"},
		{"empty content", &edition.ToolResult{}, "空响应"},
		{"whitespace text", &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "img"}, {Type: "text", Text: "   "}}}, "空响应"},
		{"invalid JSON", textToolResult("not-json"), "解析"},
	}
	for _, tc := range cases {
		inner := newDocDelegationTestCaller()
		inner.checkRes = tc.result
		d := newDocDelegationAuthDecorator(inner)
		_, err := d.CallTool(context.Background(), "doc", "update_document", nil)
		if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("%s: error = %v, want message containing %q", tc.name, err, tc.wantSub)
		}
	}
}

func TestCrossPlatformCoverageDocDelegationAuthAccessorPassthrough(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.dry = true
	d := newDocDelegationAuthDecorator(inner)
	if d.Format() != "json" || !d.DryRun() || d.Fields() != "fields-x" || d.JQ() != "jq-x" {
		t.Fatalf("accessor passthrough mismatch: %q/%v/%q/%q", d.Format(), d.DryRun(), d.Fields(), d.JQ())
	}
}

func TestCrossPlatformCoverageDocDelegationAuthWrapKeepsReadCapability(t *testing.T) {
	plain := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(plain)
	if wrapped := wrapDocDelegationAuthCaller(d, plain); wrapped != edition.ToolCaller(d) {
		t.Fatalf("wrap(plain) = %T, want the decorator itself", wrapped)
	}
	readInner := &docDelegationReadTestCaller{docDelegationTestCaller: newDocDelegationTestCaller(), readRes: textToolResult(`{"ok":true}`)}
	d2 := newDocDelegationAuthDecorator(readInner)
	wrapped := wrapDocDelegationAuthCaller(d2, readInner)
	if _, ok := wrapped.(*docDelegationAuthReadCaller); !ok {
		t.Fatalf("wrap(read-capable) = %T, want *docDelegationAuthReadCaller", wrapped)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthReadCallIntercepted(t *testing.T) {
	readInner := &docDelegationReadTestCaller{docDelegationTestCaller: newDocDelegationTestCaller(), readRes: textToolResult(`{"ok":true}`)}
	d := newDocDelegationAuthDecorator(readInner)
	wrapped := wrapDocDelegationAuthCaller(d, readInner).(*docDelegationAuthReadCaller)
	result, err := wrapped.CallReadTool(context.Background(), "wiki", "list_nodes", map[string]any{"workspaceId": "w1"})
	if err != nil {
		t.Fatalf("CallReadTool() error = %v", err)
	}
	if result != readInner.readRes {
		t.Fatalf("CallReadTool() result = %#v, want read passthrough", result)
	}
	if len(readInner.calls) != 1 || readInner.calls[0].tool != checkCapTool {
		t.Fatalf("calls = %#v, want check on the write channel", readInner.calls)
	}
	if len(readInner.readCalls) != 1 || readInner.readCalls[0].tool != "list_nodes" {
		t.Fatalf("readCalls = %#v, want one read passthrough", readInner.readCalls)
	}
	if readInner.calls[0].args["nodeId"] != "w1" {
		t.Fatalf("check args = %#v, want workspaceId promoted to nodeId", readInner.calls[0].args)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthReadCallDenied(t *testing.T) {
	readInner := &docDelegationReadTestCaller{docDelegationTestCaller: newDocDelegationTestCaller()}
	readInner.checkRes = textToolResult(`{"allowed":false,"denialReason":"NO_PERM"}`)
	d := newDocDelegationAuthDecorator(readInner)
	wrapped := wrapDocDelegationAuthCaller(d, readInner).(*docDelegationAuthReadCaller)
	if _, err := wrapped.CallReadTool(context.Background(), "wiki", "list_nodes", nil); err == nil {
		t.Fatal("CallReadTool() error = nil, want denial")
	}
	if len(readInner.readCalls) != 0 {
		t.Fatalf("readCalls = %#v, want read blocked on denial", readInner.readCalls)
	}
}

func newDocDelegationTestRoot(runE func(*cobra.Command, []string) error) *cobra.Command {
	root := &cobra.Command{Use: "drive"}
	installDocDelegationAuth(root)
	if runE == nil {
		runE = func(*cobra.Command, []string) error { return nil }
	}
	sub := &cobra.Command{Use: "sub", RunE: runE}
	root.AddCommand(sub)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

func TestCrossPlatformCoverageDocDelegationAuthInstallNoFlagKeepsCaller(t *testing.T) {
	inner := newDocDelegationTestCaller()
	installHelpersCoreDeps(t, inner)
	var seen edition.ToolCaller
	root := newDocDelegationTestRoot(func(*cobra.Command, []string) error {
		seen = deps.Caller
		return nil
	})
	root.SetArgs([]string{"sub"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if seen != edition.ToolCaller(inner) {
		t.Fatalf("deps.Caller during RunE = %T, want the raw inner caller", seen)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthInstallDepsNotInitialized(t *testing.T) {
	old := deps
	t.Cleanup(func() { deps = old })
	for _, state := range []*Deps{nil, {Caller: nil}} {
		deps = state
		root := newDocDelegationTestRoot(nil)
		root.SetArgs([]string{"sub", "--principal-user-id", "u1"})
		err := root.Execute()
		var cliErr *CLIError
		if err == nil || !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError {
			t.Fatalf("deps=%#v: Execute() error = %v, want CLIError CodeMCPToolError", state, err)
		}
	}
}

func TestCrossPlatformCoverageDocDelegationAuthInstallDryRunSkipsDecorator(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.dry = true
	installHelpersCoreDeps(t, inner)
	var seen edition.ToolCaller
	root := newDocDelegationTestRoot(func(*cobra.Command, []string) error {
		seen = deps.Caller
		return nil
	})
	root.SetArgs([]string{"sub", "--principal-user-id", "u1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if seen != edition.ToolCaller(inner) {
		t.Fatalf("deps.Caller during dry-run RunE = %T, want undecorated caller", seen)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthInstallWrapsAndRestores(t *testing.T) {
	inner := newDocDelegationTestCaller()
	installHelpersCoreDeps(t, inner)
	var seen edition.ToolCaller
	root := newDocDelegationTestRoot(func(*cobra.Command, []string) error {
		seen = deps.Caller
		return nil
	})
	root.SetArgs([]string{"sub", "--principal-user-id", " u1 "})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	decorated, ok := seen.(*docDelegationAuthCaller)
	if !ok {
		t.Fatalf("deps.Caller during RunE = %T, want *docDelegationAuthCaller", seen)
	}
	if decorated.principalID != "u1" {
		t.Fatalf("principalID = %q, want trimmed %q", decorated.principalID, "u1")
	}
	if decorated.inner != edition.ToolCaller(inner) {
		t.Fatalf("decorator inner = %T, want the previous caller", decorated.inner)
	}
	if deps.Caller != edition.ToolCaller(inner) {
		t.Fatalf("deps.Caller after Execute = %T, want restored inner caller", deps.Caller)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthInstallKeepsReadCapability(t *testing.T) {
	readInner := &docDelegationReadTestCaller{docDelegationTestCaller: newDocDelegationTestCaller()}
	installHelpersCoreDeps(t, readInner)
	var seen edition.ToolCaller
	root := newDocDelegationTestRoot(func(*cobra.Command, []string) error {
		seen = deps.Caller
		return nil
	})
	root.SetArgs([]string{"sub", "--principal-user-id", "u1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, ok := seen.(*docDelegationAuthReadCaller); !ok {
		t.Fatalf("deps.Caller during RunE = %T, want *docDelegationAuthReadCaller", seen)
	}
	if deps.Caller != edition.ToolCaller(readInner) {
		t.Fatalf("deps.Caller after Execute = %T, want restored inner caller", deps.Caller)
	}
}
