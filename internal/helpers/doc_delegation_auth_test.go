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
	"os"
	"strings"
	"sync"
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

func TestCrossPlatformCoverageDocDelegationAuthNoNodeIDRejectsLocally(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "drive", "list_files", map[string]any{"limit": 20})
	if err == nil {
		t.Fatal("CallTool() error = nil, want DELEGATION_AUTH_NOT_SUPPORTED")
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_NOT_SUPPORTED]") {
		t.Fatalf("Error() = %q, want [DELEGATION_AUTH_NOT_SUPPORTED] prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "缺少节点标识参数") {
		t.Fatalf("Error() = %q, want message about missing node identifier", err.Error())
	}
	if !strings.Contains(err.Error(), "u-principal") {
		t.Fatalf("Error() = %q, want principal ID in message", err.Error())
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
		t.Fatalf("error = %v, want structured validation-category error", err)
	}
	if typed.Reason != "delegation_not_supported" {
		t.Fatalf("Reason = %q, want delegation_not_supported", typed.Reason)
	}
	if code := apperrors.ExitCode(err); code != apperrors.ExitCodeValidation {
		t.Fatalf("ExitCode() = %d, want %d", code, apperrors.ExitCodeValidation)
	}
	// Must not call any remote service
	if len(inner.calls) != 0 {
		t.Fatalf("calls = %d, want 0 (no remote call on missing node)", len(inner.calls))
	}
	// CLIError shell must pass through WrapErrorWithOperation unchanged
	if passthrough := WrapErrorWithOperation(err, "drive/list_files"); passthrough != err {
		t.Fatalf("WrapErrorWithOperation() = %v, want the not-supported error passed through unchanged", passthrough)
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
	// 退出码契约：渲染侧 apperrors.ExitCode 经 errors.As 穿透 CLIError.Cause
	// 命中 CategoryAPI，恢复退出码 1（缺失 Cause 时未知码会退化为 rc=5）。
	if code := apperrors.ExitCode(err); code != apperrors.ExitCodeAPI {
		t.Fatalf("ExitCode() = %d, want %d", code, apperrors.ExitCodeAPI)
	}
	// 守卫：CLIError 外壳必须在 WrapErrorWithOperation 直通分支原样返回，
	// 防止未来有人移除直通分支时拒绝错误被模式分类重包装成 MCP_TOOL_ERROR。
	if passthrough := WrapErrorWithOperation(err, "doc/update_document"); passthrough != err {
		t.Fatalf("WrapErrorWithOperation() = %v, want the denial error passed through unchanged", passthrough)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (original tool must not run)", len(inner.calls))
	}
	if d.checked["doc.update_document.n1"] {
		t.Fatal("denied toolKey must not be marked checked")
	}
}

func TestCrossPlatformCoverageDocDelegationAuthDeniedFallsBackToReason(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.checkRes = textToolResult(`{"allowed":false,"denialReason":"NO_PERM","denialMessage":"  "}`)
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
	if err == nil || !strings.Contains(err.Error(), "NO_PERM") {
		t.Fatalf("error = %v, want fallback to denialReason", err)
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDeniedSurvivesRealPipeline 把拒绝
// 外壳推经 helpers 层真实出口漏斗 parseMCPToolTextResult（helpers.go 工具调用
// 统一的 err 出口形态：先 reclassifyPATFromError、再 WrapError），断言返回的
// 仍是同一个 *CLIError 实例且 Code 未被改写。这是无需 stub 框架 runner 的
// 最窄真实接缝：PAT 重分类对非 PAT 文案返回 nil，随后 WrapError 命中
// CLIError 直通分支，两层均不得改写拒绝外壳。
func TestCrossPlatformCoverageDocDelegationAuthDeniedSurvivesRealPipeline(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.checkRes = textToolResult(`{"allowed":false,"denialReason":"NO_PERM","denialMessage":"没有该文档的委托权限"}`)
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want denial error")
	}
	text, pipeErr := parseMCPToolTextResult("doc", "update_document", nil, err)
	if text != "" {
		t.Fatalf("parseMCPToolTextResult() text = %q, want empty on error", text)
	}
	if pipeErr != err {
		t.Fatalf("parseMCPToolTextResult() error = %v (%T), want the same denial instance (%T)", pipeErr, pipeErr, err)
	}
	var cliErr *CLIError
	if !errors.As(pipeErr, &cliErr) || cliErr.Code != codeDelegationDenied {
		t.Fatalf("pipeline error = %v, want unchanged Code %q", pipeErr, codeDelegationDenied)
	}
	if strings.Contains(pipeErr.Error(), "MCP_TOOL_ERROR") {
		t.Fatalf("pipeline error = %q, must not carry MCP_TOOL_ERROR", pipeErr.Error())
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
		{"overwriteFileId", map[string]any{"overwriteFileId": "of1"}, "of1"},
		{"overwriteNodeId", map[string]any{"overwriteNodeId": "on1"}, "on1"},
		{"workspaceId", map[string]any{"workspaceId": "w1"}, "w1"},
		{"spaceId", map[string]any{"spaceId": "s1"}, "s1"},
		{"workspace_id", map[string]any{"workspace_id": "w2"}, "w2"},
		{"space_id", map[string]any{"space_id": "s2"}, "s2"},
		{"node beats workspace", map[string]any{"workspaceId": "w1", "fileId": "f1"}, "f1"},
		{"nodeId beats fileId", map[string]any{"fileId": "f1", "nodeId": "n1"}, "n1"},
		// 覆盖上传场景：step1 入参排他地携带 overwrite 键，且优先级 1 组
		// 整体优先于优先级 2 组（否则 check 误抓 spaceId/workspaceId 作为
		// nodeId，导致服务端 52600007 误拒）。
		{"nodeId beats overwrite keys", map[string]any{"overwriteFileId": "of1", "overwriteNodeId": "on1", "nodeId": "n1"}, "n1"},
		{"overwriteFileId beats overwriteNodeId", map[string]any{"overwriteNodeId": "on1", "overwriteFileId": "of1"}, "of1"},
		{"overwriteFileId beats space keys", map[string]any{"spaceId": "s1", "overwriteFileId": "of1"}, "of1"},
		{"overwriteNodeId beats workspace keys", map[string]any{"workspaceId": "w1", "overwriteNodeId": "on1"}, "on1"},
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
	if _, err := d.CallTool(ctx, "doc", "update_document", map[string]any{"nodeId": "n1"}); err != nil {
		t.Fatalf("CallTool(doc) error = %v", err)
	}
	if _, err := d.CallTool(ctx, "wiki", "create_wikiSpace", map[string]any{"nodeId": "n2"}); err != nil {
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

// TestCrossPlatformCoverageDocDelegationAuthMarkdownOverwriteRealSeam 以
// markdown overwrite 实际发出的工具调用形态验证真实接缝：markdown 子命令的
// 数据面调用全部复用 drive/doc 域函数（markdown.go → uploadToDrive/
// uploadToDocSpace），工具键形如 drive.get_upload_info / doc.
// get_file_upload_info，自功能初始提交起即经 drive/doc 白名单条目拦截，
// 全仓无以 "markdown" 为 serverID 的调用点。本测试钉住该真实形态：
// check 以 drive.get_upload_info 先行发起、nodeId 从 overwriteFileId 提升，
// 拒绝时阻断原调用、错误形态与 doc 域一致。
func TestCrossPlatformCoverageDocDelegationAuthMarkdownOverwriteRealSeam(t *testing.T) {
	// uploadToDrive 覆盖模式 step1 入参的真实形态（drive.go）：fileName/
	// fileSize/mimeType/spaceId/overwriteFileId，排他地不携带 parentId。
	args := map[string]any{
		"fileName":        "notes.md",
		"fileSize":        float64(128),
		"mimeType":        "text/markdown",
		"spaceId":         "sp-1",
		"overwriteFileId": "node-42",
	}

	// 场景 1：check 先行发起（toolKey 形如 drive.get_upload_info）、nodeId
	// 从 overwriteFileId 提升、通过后原调用透传。
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	if _, err := d.CallTool(context.Background(), "drive", "get_upload_info", args); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(inner.calls) != 2 || inner.calls[0].tool != checkCapTool || inner.calls[1].tool != "get_upload_info" {
		t.Fatalf("calls = %#v, want check followed by the original drive call", inner.calls)
	}
	if inner.calls[0].args["mcpToolKey"] != "drive.get_upload_info" {
		t.Fatalf("check args = %#v, want mcpToolKey drive.get_upload_info", inner.calls[0].args)
	}
	if inner.calls[0].args["nodeId"] != "node-42" {
		t.Fatalf("check args = %#v, want nodeId promoted from overwriteFileId", inner.calls[0].args)
	}
	if !d.checked["drive.get_upload_info.node-42"] {
		t.Fatal("drive.get_upload_info must be marked checked after the passing check")
	}

	// 场景 2：同一真实形态下拒绝时阻断原调用，错误形态与 doc 域一致。
	inner2 := newDocDelegationTestCaller()
	inner2.checkRes = textToolResult(`{"allowed":false,"denialReason":"NO_PERM","denialMessage":"没有该文档的委托权限"}`)
	d2 := newDocDelegationAuthDecorator(inner2)
	_, err := d2.CallTool(context.Background(), "drive", "get_upload_info", args)
	if err == nil {
		t.Fatal("CallTool() error = nil, want markdown-overwrite denial error")
	}
	if len(inner2.calls) != 1 || inner2.calls[0].tool != checkCapTool {
		t.Fatalf("calls = %#v, want only the check call (original blocked)", inner2.calls)
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
		t.Fatalf("Error() = %q, want [DELEGATION_AUTH_DENIED] prefix", err.Error())
	}
	if strings.Contains(err.Error(), "MCP_TOOL_ERROR") {
		t.Fatalf("Error() = %q, must not carry MCP_TOOL_ERROR", err.Error())
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI || typed.Reason != "delegation_denied" {
		t.Fatalf("error = %v, want API-category delegation_denied like the doc domain", err)
	}
	if d2.checked["drive.get_upload_info.node-42"] {
		t.Fatal("denied toolKey must not be marked checked")
	}

	// 场景 3（文档性断言）：markdown 子命令经 drive/doc 条目拦截，白名单
	// 不含 "markdown" 键——全仓无以 "markdown" 为 serverID 的调用点，该条目
	// 永不命中（曾存在的条目及"markdown 域工具以 ProductID markdown 发起
	// 调用"的注释均为错误认知，已回退）。
	if docBusinessServers["markdown"] {
		t.Fatal(`docBusinessServers must not contain a "markdown" entry: no call site uses serverID "markdown"; markdown subcommands ride the drive/doc entries`)
	}
}

func TestCrossPlatformCoverageDocDelegationAuthCheckCallFails(t *testing.T) {
	inner := newDocDelegationTestCaller()
	// 底层错误文本故意包含 "tool"：裸 fmt.Errorf 会被 WrapErrorWithOperation
	// 的 "tool" 模式重分类成 MCP_TOOL_ERROR，外壳必须阻止这种重包装。
	inner.checkErr = errors.New("tool check boom")
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want wrapped check failure")
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_CHECK_FAILED]") {
		t.Fatalf("Error() = %q, want [DELEGATION_AUTH_CHECK_FAILED] prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "委托鉴权校验失败: tool check boom") {
		t.Fatalf("Error() = %q, want underlying error text preserved", err.Error())
	}
	if strings.Contains(err.Error(), "MCP_TOOL_ERROR") {
		t.Fatalf("Error() = %q, must not carry MCP_TOOL_ERROR", err.Error())
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI {
		t.Fatalf("error = %v, want structured API-category error", err)
	}
	if typed.Reason != "delegation_check_failed" {
		t.Fatalf("Reason = %q, want delegation_check_failed", typed.Reason)
	}
	if code := apperrors.ExitCode(err); code != apperrors.ExitCodeAPI {
		t.Fatalf("ExitCode() = %d, want %d", code, apperrors.ExitCodeAPI)
	}
	// WithCause 保留底层错误链：errors.Is 仍能命中原始错误。
	if !errors.Is(err, inner.checkErr) {
		t.Fatalf("error = %v, want underlying checkErr in the chain", err)
	}
	// 守卫：外壳必须在 WrapErrorWithOperation 直通分支原样返回，防止底层
	// 文本命中 "tool" 模式时被重包装成 MCP_TOOL_ERROR。
	if passthrough := WrapErrorWithOperation(err, "doc/update_document"); passthrough != err {
		t.Fatalf("WrapErrorWithOperation() = %v, want the check-failure shell passed through unchanged", passthrough)
	}
	// 真实漏斗守卫（与 DeniedSurvivesRealPipeline 同法）：把 CHECK_FAILED
	// 外壳推经 helpers 层工具调用统一错误出口 parseMCPToolTextResult，断言
	// 返回同一实例且 Code 未被改写，防止未来被 reclassify/WrapError 二次
	// 包装。
	text, pipeErr := parseMCPToolTextResult("doc", "update_document", nil, err)
	if text != "" {
		t.Fatalf("parseMCPToolTextResult() text = %q, want empty on error", text)
	}
	if pipeErr != err {
		t.Fatalf("parseMCPToolTextResult() error = %v (%T), want the same check-failure instance (%T)", pipeErr, pipeErr, err)
	}
	var pipeCLI *CLIError
	if !errors.As(pipeErr, &pipeCLI) || pipeCLI.Code != codeDelegationCheckFailed {
		t.Fatalf("pipeline error = %v, want unchanged Code %q", pipeErr, codeDelegationCheckFailed)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (stop after check failure)", len(inner.calls))
	}
}

// TestCrossPlatformCoverageDocDelegationAuthCheckResponseInvalid 覆盖
// check_capability 响应异常三分支（nil result、空响应、JSON 解析失败）：
// 三分支统一 CLIError 外壳，断言前缀 DELEGATION_AUTH_CHECK_FAILED、无
// MCP_TOOL_ERROR、category=api、reason=delegation_check_bad_response、
// 退出码 1（裸 fmt.Errorf 会经模式分类致退出码分裂：解析失败 →
// INPUT_INVALID_JSON→3、其余 → UNCLASSIFIED→5）。
func TestCrossPlatformCoverageDocDelegationAuthCheckResponseInvalid(t *testing.T) {
	cases := []struct {
		name    string
		result  *edition.ToolResult
		wantSub string
	}{
		{"nil result", nil, "返回空结果"},
		{"empty content", &edition.ToolResult{}, "返回空响应"},
		{"whitespace text", &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "img"}, {Type: "text", Text: "   "}}}, "返回空响应"},
		{"invalid JSON", textToolResult("not-json"), "响应解析失败"},
	}
	for _, tc := range cases {
		inner := newDocDelegationTestCaller()
		inner.checkRes = tc.result
		d := newDocDelegationAuthDecorator(inner)
		_, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
		if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("%s: error = %v, want message containing %q", tc.name, err, tc.wantSub)
		}
		if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_CHECK_FAILED]") {
			t.Fatalf("%s: Error() = %q, want [DELEGATION_AUTH_CHECK_FAILED] prefix", tc.name, err.Error())
		}
		if strings.Contains(err.Error(), "MCP_TOOL_ERROR") {
			t.Fatalf("%s: Error() = %q, must not carry MCP_TOOL_ERROR", tc.name, err.Error())
		}
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI {
			t.Fatalf("%s: error = %v, want structured API-category error", tc.name, err)
		}
		if typed.Reason != "delegation_check_bad_response" {
			t.Fatalf("%s: Reason = %q, want delegation_check_bad_response", tc.name, typed.Reason)
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationCheckFailed {
			t.Fatalf("%s: error = %v, want CLIError code %q", tc.name, err, codeDelegationCheckFailed)
		}
		if code := apperrors.ExitCode(err); code != apperrors.ExitCodeAPI {
			t.Fatalf("%s: ExitCode() = %d, want %d", tc.name, code, apperrors.ExitCodeAPI)
		}
		if len(inner.calls) != 1 {
			t.Fatalf("%s: calls = %d, want 1 (original blocked on bad response)", tc.name, len(inner.calls))
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
	_, err := wrapped.CallReadTool(context.Background(), "wiki", "list_nodes", map[string]any{"nodeId": "n1"})
	if err == nil {
		t.Fatal("CallReadTool() error = nil, want denial")
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
		t.Fatalf("Error() = %q, want [DELEGATION_AUTH_DENIED] prefix", err.Error())
	}
	if strings.Contains(err.Error(), "MCP_TOOL_ERROR") {
		t.Fatalf("Error() = %q, must not carry MCP_TOOL_ERROR", err.Error())
	}
	// 读通道拒绝同样依赖 WrapError 的 CLIError 直通分支，不得被模式分类改写。
	if passthrough := WrapError(err); passthrough != err {
		t.Fatalf("WrapError() = %v, want the denial shell passed through unchanged", passthrough)
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

func TestCrossPlatformCoverageDocDelegationAuthInstallDryRunStillWraps(t *testing.T) {
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
	// After removing the dry-run early return, the decorator is always installed.
	decorated, ok := seen.(*docDelegationAuthCaller)
	if !ok {
		t.Fatalf("deps.Caller during dry-run RunE = %T, want *docDelegationAuthCaller (decorator installed even in dry-run)", seen)
	}
	if decorated.principalID != "u1" {
		t.Fatalf("principalID = %q, want %q", decorated.principalID, "u1")
	}
	if !decorated.inner.DryRun() {
		t.Fatal("inner.DryRun() = false, want true (decorator wraps a dry-run caller)")
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

// TestCrossPlatformCoverageDocDelegationAuthChainsRootPersistentPreRunE is a
// guard test verifying that installDocDelegationAuth's PersistentPreRunE
// explicitly chains the root command's PersistentPreRunE. Without chaining,
// cobra's nearest-ancestor semantics would shadow the root hook (which handles
// --output/--debug/--profile/agent metadata validation/diagnostics) for every
// leaf under the five doc-business product groups.
func TestCrossPlatformCoverageDocDelegationAuthChainsRootPersistentPreRunE(t *testing.T) {
	var rootHookCallCount int
	rootCmd := &cobra.Command{
		Use: "dws",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			rootHookCallCount++
			return nil
		},
	}
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)

	groups := []string{"doc", "drive", "markdown", "sheet", "wiki"}
	for _, name := range groups {
		group := &cobra.Command{Use: name}
		installDocDelegationAuth(group)
		leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
		group.AddCommand(leaf)
		rootCmd.AddCommand(group)
	}

	for _, name := range groups {
		rootHookCallCount = 0
		rootCmd.SetArgs([]string{name, "leaf"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("%s/leaf: Execute() error = %v", name, err)
		}
		if rootHookCallCount != 1 {
			t.Fatalf("%s/leaf: root PersistentPreRunE call count = %d, want 1 (must not be shadowed by installDocDelegationAuth)", name, rootHookCallCount)
		}
	}
}

// TestCrossPlatformCoverageDocDelegationAuthRootHookErrorPropagates verifies
// that when the root's PersistentPreRunE returns an error, it propagates and
// blocks the delegation auth and the command execution.
func TestCrossPlatformCoverageDocDelegationAuthRootHookErrorPropagates(t *testing.T) {
	rootErr := errors.New("root hook validation failed")
	rootCmd := &cobra.Command{
		Use: "dws",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return rootErr
		},
	}
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)

	var leafRan bool
	group := &cobra.Command{Use: "doc"}
	installDocDelegationAuth(group)
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error {
		leafRan = true
		return nil
	}}
	group.AddCommand(leaf)
	rootCmd.AddCommand(group)

	rootCmd.SetArgs([]string{"doc", "leaf"})
	err := rootCmd.Execute()
	if !errors.Is(err, rootErr) {
		t.Fatalf("Execute() error = %v, want root hook error propagated", err)
	}
	if leafRan {
		t.Fatal("leaf RunE must not execute when root hook fails")
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDedupDifferentNodeIds verifies
// that calls to the same tool with different nodeIds each trigger a separate
// check_capability verification (node-scoped cache granularity).
func TestCrossPlatformCoverageDocDelegationAuthDedupDifferentNodeIds(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	ctx := context.Background()

	// First call: doc.update_document with nodeId "n1"
	if _, err := d.CallTool(ctx, "doc", "update_document", map[string]any{"nodeId": "n1"}); err != nil {
		t.Fatalf("CallTool(n1) error = %v", err)
	}
	// Second call: same tool, different nodeId "n2" → must trigger new check
	if _, err := d.CallTool(ctx, "doc", "update_document", map[string]any{"nodeId": "n2"}); err != nil {
		t.Fatalf("CallTool(n2) error = %v", err)
	}
	// Third call: same tool, same nodeId "n1" → deduplicated (no new check)
	if _, err := d.CallTool(ctx, "doc", "update_document", map[string]any{"nodeId": "n1"}); err != nil {
		t.Fatalf("CallTool(n1 repeat) error = %v", err)
	}

	var checks int
	var checkNodeIds []string
	for _, call := range inner.calls {
		if call.tool == checkCapTool {
			checks++
			checkNodeIds = append(checkNodeIds, call.args["nodeId"].(string))
		}
	}
	if checks != 2 {
		t.Fatalf("check calls = %d, want 2 (one per distinct nodeId)", checks)
	}
	if checkNodeIds[0] != "n1" || checkNodeIds[1] != "n2" {
		t.Fatalf("check nodeIds = %v, want [n1, n2]", checkNodeIds)
	}
}

// TestCrossPlatformCoverageDocDelegationAuthExtractNodeIDParentAndFolder
// verifies that extractNodeId recognizes parentId and folderId as node-level
// identifiers used by drive.list_files and doc.list_nodes respectively.
func TestCrossPlatformCoverageDocDelegationAuthExtractNodeIDParentAndFolder(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"parentId only", map[string]any{"parentId": "p1"}, "p1"},
		{"folderId only", map[string]any{"folderId": "f1"}, "f1"},
		{"parentId beats workspace keys", map[string]any{"workspaceId": "w1", "parentId": "p1"}, "p1"},
		{"folderId beats workspace keys", map[string]any{"spaceId": "s1", "folderId": "f1"}, "f1"},
		{"nodeId beats parentId", map[string]any{"parentId": "p1", "nodeId": "n1"}, "n1"},
		{"fileId beats folderId", map[string]any{"folderId": "f1", "fileId": "ff"}, "ff"},
		{"parentId before folderId", map[string]any{"folderId": "f1", "parentId": "p1"}, "p1"},
		// walkRemoteDir 真实入参：list_files 带 spaceId+parentId，parentId 优先
		{"drive list_files real seam", map[string]any{"spaceId": "sp-1", "parentId": "folder-uuid", "maxResults": float64(200)}, "folder-uuid"},
		// doc.list_nodes 真实入参：带 workspaceId+folderId
		{"doc list_nodes real seam", map[string]any{"workspaceId": "ws-1", "folderId": "folder-node", "pageSize": float64(50)}, "folder-node"},
	}
	for _, tc := range cases {
		if got := extractNodeId(tc.args); got != tc.want {
			t.Fatalf("%s: extractNodeId(%#v) = %q, want %q", tc.name, tc.args, got, tc.want)
		}
	}
}

// TestCrossPlatformCoverageDocDelegationAuthParentIdTriggersNodeScopedCheck
// verifies that a drive.list_files call with parentId triggers a node-scoped
// check_capability with the parentId promoted to nodeId, and that subsequent
// calls with a different parentId trigger a new check.
func TestCrossPlatformCoverageDocDelegationAuthParentIdTriggersNodeScopedCheck(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	ctx := context.Background()

	// First list_files with parentId "folder-A"
	args1 := map[string]any{"spaceId": "sp-1", "parentId": "folder-A", "maxResults": float64(200)}
	if _, err := d.CallTool(ctx, "drive", "list_files", args1); err != nil {
		t.Fatalf("CallTool(folder-A) error = %v", err)
	}
	// Second list_files with parentId "folder-B" → new check
	args2 := map[string]any{"spaceId": "sp-1", "parentId": "folder-B", "maxResults": float64(200)}
	if _, err := d.CallTool(ctx, "drive", "list_files", args2); err != nil {
		t.Fatalf("CallTool(folder-B) error = %v", err)
	}
	// Third list_files with parentId "folder-A" → deduplicated
	if _, err := d.CallTool(ctx, "drive", "list_files", args1); err != nil {
		t.Fatalf("CallTool(folder-A repeat) error = %v", err)
	}

	var checks []string
	for _, call := range inner.calls {
		if call.tool == checkCapTool {
			checks = append(checks, call.args["nodeId"].(string))
		}
	}
	if len(checks) != 2 || checks[0] != "folder-A" || checks[1] != "folder-B" {
		t.Fatalf("check nodeIds = %v, want [folder-A, folder-B]", checks)
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDryRunStillChecks verifies that
// when the inner caller reports DryRun()=true, the decorator is still installed
// and ensureDelegationAuth triggers a check_capability call. In dry-run mode,
// performDelegationAuth routes check_capability through CallReadTool (not
// CallTool) because the inner implements ReadToolCaller — this avoids the
// EchoRunner returning {"dry_run":true} which would always deny.
func TestCrossPlatformCoverageDocDelegationAuthDryRunStillChecks(t *testing.T) {
	// readRes serves both check_capability (via ReadTool in dry-run) and the
	// actual get_document passthrough: the mock returns the same result for
	// all CallReadTool invocations regardless of tool name.
	readInner := &docDelegationReadTestCaller{docDelegationTestCaller: newDocDelegationTestCaller(), readRes: textToolResult(`{"allowed":true}`)}
	readInner.dry = true
	d := newDocDelegationAuthDecorator(readInner)
	wrapped := wrapDocDelegationAuthCaller(d, readInner).(*docDelegationAuthReadCaller)
	result, err := wrapped.CallReadTool(context.Background(), "doc", "get_document", map[string]any{"nodeId": "n1"})
	if err != nil {
		t.Fatalf("CallReadTool() error = %v", err)
	}
	if result != readInner.readRes {
		t.Fatalf("CallReadTool() result = %#v, want read passthrough", result)
	}
	// In dry-run with ReadToolCaller, check_capability goes through the read
	// channel (not CallTool). readCalls should have 2 entries: check +
	// passthrough; regular calls should have 0.
	if len(readInner.calls) != 0 {
		t.Fatalf("calls = %#v, want 0 (check_capability routes via ReadTool in dry-run)", readInner.calls)
	}
	if len(readInner.readCalls) != 2 {
		t.Fatalf("readCalls = %d, want 2 (check_capability + get_document)", len(readInner.readCalls))
	}
	if readInner.readCalls[0].tool != checkCapTool {
		t.Fatalf("readCalls[0] = %#v, want check_capability", readInner.readCalls[0])
	}
	if readInner.readCalls[0].args["nodeId"] != "n1" {
		t.Fatalf("check args = %#v, want nodeId n1", readInner.readCalls[0].args)
	}
	if readInner.readCalls[1].tool != "get_document" {
		t.Fatalf("readCalls[1] = %#v, want get_document passthrough", readInner.readCalls[1])
	}
}

// TestCrossPlatformCoverageDocDelegationAuthNoNodeRejectsLocally verifies
// that when args contain no recognizable node identifier, the decorator
// returns DELEGATION_AUTH_NOT_SUPPORTED immediately without any remote call,
// including via the read channel.
func TestCrossPlatformCoverageDocDelegationAuthNoNodeRejectsLocally(t *testing.T) {
	// Write channel: CallTool
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "search_documents", map[string]any{"query": "hello"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want DELEGATION_AUTH_NOT_SUPPORTED")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationNotSupported {
		t.Fatalf("error = %v, want CLIError code %q", err, codeDelegationNotSupported)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("calls = %d, want 0 (no remote call)", len(inner.calls))
	}

	// Read channel: CallReadTool
	readInner := &docDelegationReadTestCaller{docDelegationTestCaller: newDocDelegationTestCaller(), readRes: textToolResult(`{"ok":true}`)}
	d2 := newDocDelegationAuthDecorator(readInner)
	wrapped := wrapDocDelegationAuthCaller(d2, readInner).(*docDelegationAuthReadCaller)
	_, err = wrapped.CallReadTool(context.Background(), "wiki", "search_nodes", map[string]any{"keyword": "foo"})
	if err == nil {
		t.Fatal("CallReadTool() error = nil, want DELEGATION_AUTH_NOT_SUPPORTED")
	}
	if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationNotSupported {
		t.Fatalf("read error = %v, want CLIError code %q", err, codeDelegationNotSupported)
	}
	if len(readInner.calls) != 0 {
		t.Fatalf("read inner calls = %d, want 0 (no remote call)", len(readInner.calls))
	}
	if len(readInner.readCalls) != 0 {
		t.Fatalf("read readCalls = %d, want 0 (read blocked on not-supported)", len(readInner.readCalls))
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDryRunCallToolAlsoChecks verifies
// the full dry-run pre-check path: when helpers' dry-run branch calls
// deps.Caller.CallTool, the delegation-auth decorator intercepts and routes
// the check_capability call through the ReadTool channel (because inner
// reports DryRun()=true and implements ReadToolCaller).
func TestCrossPlatformCoverageDocDelegationAuthDryRunCallToolAlsoChecks(t *testing.T) {
	readInner := &docDelegationReadTestCaller{
		docDelegationTestCaller: newDocDelegationTestCaller(),
		readRes:                 textToolResult(`{"allowed":true}`),
	}
	readInner.dry = true
	d := newDocDelegationAuthDecorator(readInner)
	wrapped := wrapDocDelegationAuthCaller(d, readInner)

	// Simulate the dry-run pre-check path: helpers calls deps.Caller.CallTool
	// which hits the decorator's CallTool → ensureDelegationAuth →
	// performDelegationAuth → inner.DryRun()=true → CallReadTool.
	result, err := wrapped.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	// The passthrough result comes from CallTool on the base caller (not read channel).
	if result != readInner.passRes {
		t.Fatalf("CallTool() result = %#v, want passthrough result", result)
	}

	// Verify check_capability was routed to ReadTool channel.
	if len(readInner.readCalls) != 1 {
		t.Fatalf("readCalls = %d, want 1 (check_capability via ReadTool)", len(readInner.readCalls))
	}
	rc := readInner.readCalls[0]
	if rc.server != capabilityServerID || rc.tool != checkCapTool {
		t.Fatalf("readCall[0] = %s/%s, want %s/%s", rc.server, rc.tool, capabilityServerID, checkCapTool)
	}
	if rc.args["userId"] != "u-principal" || rc.args["mcpToolKey"] != "doc.update_document" || rc.args["nodeId"] != "n1" {
		t.Fatalf("readCall[0] args = %#v, want correct check_capability params", rc.args)
	}

	// The base CallTool still gets the passthrough call.
	var passthroughCalls int
	for _, c := range readInner.calls {
		if c.tool != checkCapTool {
			passthroughCalls++
		}
	}
	if passthroughCalls != 1 {
		t.Fatalf("passthrough calls = %d, want 1", passthroughCalls)
	}

	// check_capability must NOT appear on the regular CallTool channel
	// (it should only go through ReadTool in dry-run).
	for _, c := range readInner.calls {
		if c.tool == checkCapTool {
			t.Fatalf("check_capability must not go through regular CallTool in dry-run, but found: %#v", c)
		}
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDryRunFallbackNoReadCaller verifies
// that when the inner caller does NOT implement ReadToolCaller but is in
// dry-run mode, performDelegationAuth falls back to CallTool for the check.
func TestCrossPlatformCoverageDocDelegationAuthDryRunFallbackNoReadCaller(t *testing.T) {
	inner := newDocDelegationTestCaller()
	inner.dry = true
	d := newDocDelegationAuthDecorator(inner)

	result, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": "n1"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result != inner.passRes {
		t.Fatalf("CallTool() result = %#v, want passthrough", result)
	}
	// check_capability goes through regular CallTool (fallback path).
	if len(inner.calls) != 2 || inner.calls[0].tool != checkCapTool {
		t.Fatalf("calls = %#v, want [check_capability, update_document]", inner.calls)
	}
}

// concurrentSafeTestCaller wraps docDelegationTestCaller with a mutex to make
// CallTool safe for concurrent use in the race test (the production decorator's
// checked map is the real subject under test, not the mock).
type concurrentSafeTestCaller struct {
	mu    sync.Mutex
	inner *docDelegationTestCaller
}

func (c *concurrentSafeTestCaller) CallTool(ctx context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inner.CallTool(ctx, serverID, toolName, args)
}
func (c *concurrentSafeTestCaller) Format() string { return c.inner.Format() }
func (c *concurrentSafeTestCaller) DryRun() bool   { return c.inner.DryRun() }
func (c *concurrentSafeTestCaller) Fields() string { return c.inner.Fields() }
func (c *concurrentSafeTestCaller) JQ() string     { return c.inner.JQ() }

// TestCrossPlatformCoverageDocDelegationAuthConcurrentSafe uses -race detection
// (go test -race) to verify that concurrent CallTool invocations on the same
// decorator do not race on the checked map.
func TestCrossPlatformCoverageDocDelegationAuthConcurrentSafe(t *testing.T) {
	safeInner := &concurrentSafeTestCaller{inner: newDocDelegationTestCaller()}
	d := &docDelegationAuthCaller{inner: safeInner, principalID: "u-principal", checked: map[string]bool{}}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			nodeID := "n1" // same node → exercises both read and write on checked map
			if idx%2 == 0 {
				nodeID = "n2" // different node → exercises write path
			}
			_, _ = d.CallTool(context.Background(), "doc", "update_document", map[string]any{"nodeId": nodeID})
		}(i)
	}
	wg.Wait()

	// Basic sanity: both nodes must be checked.
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.checked["doc.update_document.n1"] || !d.checked["doc.update_document.n2"] {
		t.Fatalf("checked = %#v, want both n1 and n2 marked", d.checked)
	}
}

// docDelegationHelpersTestCaller is a minimal ToolCaller+ReadToolCaller for
// testing the helpers.go dryRunValidator integration path. Unlike the main
// mock, it returns valid empty values for JQ/Fields to avoid triggering jq
// evaluation errors in PrintJSON.
type docDelegationHelpersTestCaller struct {
	calls     []docDelegationCall
	readCalls []docDelegationCall
	checkRes  *edition.ToolResult
	readRes   *edition.ToolResult
}

func (c *docDelegationHelpersTestCaller) CallTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, docDelegationCall{server: serverID, tool: toolName, args: args})
	if toolName == checkCapTool {
		return c.checkRes, nil
	}
	return textToolResult(`{"ok":true}`), nil
}
func (c *docDelegationHelpersTestCaller) CallReadTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.readCalls = append(c.readCalls, docDelegationCall{server: serverID, tool: toolName, args: args})
	return c.readRes, nil
}
func (*docDelegationHelpersTestCaller) Format() string { return "json" }
func (*docDelegationHelpersTestCaller) DryRun() bool   { return true }
func (*docDelegationHelpersTestCaller) Fields() string { return "" }
func (*docDelegationHelpersTestCaller) JQ() string     { return "" }

// TestCrossPlatformCoverageDocDelegationAuthDryRunHelpersPreCheck verifies the
// helpers.go integration: when deps.Caller is a delegation auth decorator in
// dry-run mode, callMCPToolInternalOptsContext's dry-run branch triggers
// ensureDelegationAuth via the dryRunValidator interface BEFORE rendering the
// preview. This covers the full end-to-end path from helpers → decorator →
// check_capability (via ReadTool in dry-run).
func TestCrossPlatformCoverageDocDelegationAuthDryRunHelpersPreCheck(t *testing.T) {
	inner := &docDelegationHelpersTestCaller{
		checkRes: textToolResult(`{"allowed":true}`),
		readRes:  textToolResult(`{"allowed":true}`),
	}
	d := &docDelegationAuthCaller{inner: inner, principalID: "u-principal", checked: map[string]bool{}}
	wrapped := wrapDocDelegationAuthCaller(d, inner)
	out, _ := installHelpersCoreDeps(t, wrapped)

	// Call a doc-business tool through the helpers layer in dry-run mode.
	err := callMCPToolOnServer("doc", "update_document", map[string]any{"nodeId": "n1"})
	if err != nil {
		t.Fatalf("callMCPToolOnServer() error = %v", err)
	}

	// Verify check_capability went through ReadTool channel.
	if len(inner.readCalls) != 1 {
		t.Fatalf("readCalls = %d, want 1 (check_capability via ReadTool)", len(inner.readCalls))
	}
	if inner.readCalls[0].tool != checkCapTool {
		t.Fatalf("readCalls[0] = %#v, want check_capability", inner.readCalls[0])
	}
	if inner.readCalls[0].args["nodeId"] != "n1" {
		t.Fatalf("check args = %#v, want nodeId n1", inner.readCalls[0].args)
	}

	// Verify no actual MCP CallTool calls were made (dry-run returns early
	// after the pre-check, no inner.CallTool passthrough).
	if len(inner.calls) != 0 {
		t.Fatalf("calls = %d, want 0 (dry-run must not call inner.CallTool)", len(inner.calls))
	}

	// Verify dry-run preview was rendered.
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("output = %q, want dry-run JSON preview", out.String())
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDryRunHelpersPreCheckDenied verifies
// that when the delegation auth check denies in dry-run mode, the helpers layer
// returns the error and does NOT render the preview.
func TestCrossPlatformCoverageDocDelegationAuthDryRunHelpersPreCheckDenied(t *testing.T) {
	inner := &docDelegationHelpersTestCaller{
		checkRes: textToolResult(`{"allowed":true}`),
		readRes:  textToolResult(`{"allowed":false,"denialReason":"NO_PERM","denialMessage":"denied"}`),
	}
	d := &docDelegationAuthCaller{inner: inner, principalID: "u-principal", checked: map[string]bool{}}
	wrapped := wrapDocDelegationAuthCaller(d, inner)
	out, _ := installHelpersCoreDeps(t, wrapped)

	err := callMCPToolOnServer("doc", "update_document", map[string]any{"nodeId": "n1"})
	if err == nil {
		t.Fatal("callMCPToolOnServer() error = nil, want denial")
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
		t.Fatalf("error = %q, want [DELEGATION_AUTH_DENIED] prefix", err.Error())
	}

	// No preview should be rendered on denial.
	if strings.Contains(out.String(), "dry_run") || strings.Contains(out.String(), "DRY-RUN") {
		t.Fatalf("output = %q, must not render preview on denial", out.String())
	}
}

// TestCrossPlatformCoverageDocDelegationAuthDryRunHelpersPreCheckResolvesProduct
// verifies the dry-run pre-check path when NO explicit serverID is passed:
// callMCPToolContext forwards an empty explicitServerID, so the pre-check must
// fall back to resolveProductID() (reading the product name from os.Args) to
// determine the server ID before invoking the delegation auth validator. This
// covers the resolveProductID() branch inside the dry-run pre-check.
func TestCrossPlatformCoverageDocDelegationAuthDryRunHelpersPreCheckResolvesProduct(t *testing.T) {
	inner := &docDelegationHelpersTestCaller{
		checkRes: textToolResult(`{"allowed":true}`),
		readRes:  textToolResult(`{"allowed":true}`),
	}
	d := &docDelegationAuthCaller{inner: inner, principalID: "u-principal", checked: map[string]bool{}}
	wrapped := wrapDocDelegationAuthCaller(d, inner)
	out, _ := installHelpersCoreDeps(t, wrapped)

	// resolveProductID scans os.Args for a known product command name; "doc"
	// maps to server ID "doc" in cmdToProduct.
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"dws", "doc", "update-document"}

	// callMCPToolContext passes "" as explicitServerID, forcing the pre-check
	// to call resolveProductID().
	err := callMCPToolContext(context.Background(), "update_document", map[string]any{"nodeId": "n1"})
	if err != nil {
		t.Fatalf("callMCPToolContext() error = %v", err)
	}

	// The check must have been routed with the resolved server ID "doc".
	if len(inner.readCalls) != 1 {
		t.Fatalf("readCalls = %d, want 1 (check_capability via ReadTool)", len(inner.readCalls))
	}
	if inner.readCalls[0].tool != checkCapTool {
		t.Fatalf("readCalls[0] = %#v, want check_capability", inner.readCalls[0])
	}
	// mcpToolKey must be "doc.update_document" proving resolveProductID
	// returned "doc" as the server ID.
	if inner.readCalls[0].args["mcpToolKey"] != "doc.update_document" {
		t.Fatalf("check mcpToolKey = %#v, want doc.update_document (resolveProductID resolved doc)", inner.readCalls[0].args["mcpToolKey"])
	}
	if len(inner.calls) != 0 {
		t.Fatalf("calls = %d, want 0 (dry-run must not call inner.CallTool)", len(inner.calls))
	}
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("output = %q, want dry-run JSON preview", out.String())
	}
}
