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
	"path/filepath"
	"reflect"
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
		// targetFolderId: import --folder 场景承载目标文件夹，优先级在 folderId
		// 之后、workspaceId 之前；copy/move args 同时含 nodeId+targetFolderId 时
		// nodeId 优先（源节点作为被校验对象），targetFolderId 仅进 options。
		{"targetFolderId only", map[string]any{"targetFolderId": "tf1"}, "tf1"},
		{"targetFolderId beats workspaceId", map[string]any{"workspaceId": "w1", "targetFolderId": "tf1"}, "tf1"},
		{"nodeId beats targetFolderId (copy/move)", map[string]any{"nodeId": "n1", "targetFolderId": "tf1"}, "n1"},
		{"folderId beats targetFolderId", map[string]any{"folderId": "f1", "targetFolderId": "tf1"}, "f1"},
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

// ---------------------------------------------------------------------------
// Phase 2 (options 补全) delegation-side coverage — merged from the former
// doc_delegation_auth_options_test.go (kept in this file per single-file rule).
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Phase 2 (options 补全) delegation-side coverage. These tests pin the pure
// builders (buildDelegationOptions and its sub-builders), the $corpId runtime
// default read path (resolveCurrentCorpID), the options injection into the
// check_capability call, backward compatibility (no options key when empty),
// and import dry-run delegation parity.
// ---------------------------------------------------------------------------

// setRuntimeCorpID installs a temporary $corpId runtime default resolver and
// restores the previous registry on cleanup, avoiding RegisterRuntimeDefault's
// duplicate-id panic across tests.
func setRuntimeCorpID(t *testing.T, corpID string, present bool) {
	t.Helper()
	runtimeDefaultsMu.Lock()
	previous := runtimeDefaults
	runtimeDefaults = make(map[string]edition.RuntimeDefaultFn, len(previous)+1)
	for k, v := range previous {
		runtimeDefaults[k] = v
	}
	runtimeDefaults[RuntimeDefaultCorpID] = func(context.Context) (string, bool) {
		return corpID, present
	}
	runtimeDefaultsMu.Unlock()
	t.Cleanup(func() {
		runtimeDefaultsMu.Lock()
		runtimeDefaults = previous
		runtimeDefaultsMu.Unlock()
	})
}

func TestCrossPlatformCoverageDelegationOptionsCreateActionParam(t *testing.T) {
	cases := []struct {
		name     string
		toolKey  string
		args     map[string]any
		wantNil  bool
		wantName any // string wanted, or nil for absent
		wantDir  bool
	}{
		{
			name:     "create_document rebuilds .adoc",
			toolKey:  "doc.create_document",
			args:     map[string]any{"name": "周报"},
			wantName: "周报.adoc",
			wantDir:  false,
		},
		{
			name:    "create_document missing name yields nil",
			toolKey: "doc.create_document",
			args:    map[string]any{},
			wantNil: true,
		},
		{
			name:     "create_file typed rebuilds extension",
			toolKey:  "drive.create_file",
			args:     map[string]any{"name": "data", "type": "axls"},
			wantName: "data.axls",
			wantDir:  false,
		},
		{
			name:     "create_file folder type is createFolder without name",
			toolKey:  "drive.create_file",
			args:     map[string]any{"name": "ignored", "type": "folder"},
			wantName: nil,
			wantDir:  true,
		},
		{
			name:     "create_folder is createFolder without name",
			toolKey:  "drive.create_folder",
			args:     map[string]any{},
			wantName: nil,
			wantDir:  true,
		},
		{
			name:     "create_workspace_sheet rebuilds .axls",
			toolKey:  "sheet.create_workspace_sheet",
			args:     map[string]any{"name": "预算"},
			wantName: "预算.axls",
			wantDir:  false,
		},
		{
			name:     "apply_sheet_template with name rebuilds .axls",
			toolKey:  "sheet.apply_sheet_template",
			args:     map[string]any{"name": "模板"},
			wantName: "模板.axls",
			wantDir:  false,
		},
		{
			name:    "apply_sheet_template without name yields nil (optional)",
			toolKey: "sheet.apply_sheet_template",
			args:    map[string]any{},
			wantNil: true,
		},
		{
			name:    "create_file typed without name yields nil",
			toolKey: "drive.create_file",
			args:    map[string]any{"type": "axls"},
			wantNil: true,
		},
		{
			name:     "create_file without type keeps bare name (no extension rebuild)",
			toolKey:  "drive.create_file",
			args:     map[string]any{"name": "raw"},
			wantName: "raw",
			wantDir:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := buildDelegationOptions(tc.toolKey, tc.args, "")
			if tc.wantNil {
				if opts != nil {
					t.Fatalf("options = %#v, want nil", opts)
				}
				return
			}
			if opts == nil {
				t.Fatal("options = nil, want createActionParam")
			}
			if _, hasAction := opts["action"]; hasAction {
				t.Fatalf("options must not carry an action field: %#v", opts)
			}
			if len(opts) != 1 {
				t.Fatalf("options = %#v, want exactly one sub-object", opts)
			}
			param, ok := opts["createActionParam"].(map[string]any)
			if !ok {
				t.Fatalf("options = %#v, want createActionParam", opts)
			}
			if param["createFolder"] != tc.wantDir {
				t.Fatalf("createFolder = %v, want %v", param["createFolder"], tc.wantDir)
			}
			got, hasName := param["name"]
			if tc.wantName == nil {
				if hasName {
					t.Fatalf("name = %v, want absent", got)
				}
				return
			}
			if got != tc.wantName {
				t.Fatalf("name = %v, want %v", got, tc.wantName)
			}
		})
	}

	// buildDelegationOptions only dispatches the five create_* tool names to
	// buildCreateActionParam, so its default return nil (unmatched toolName) is
	// unreachable through dispatch; exercise it directly.
	t.Run("unmatched toolName yields nil (direct)", func(t *testing.T) {
		if p := buildCreateActionParam("not_a_create_tool", map[string]any{"name": "x"}); p != nil {
			t.Fatalf("buildCreateActionParam(unmatched) = %#v, want nil", p)
		}
	})
}

func TestCrossPlatformCoverageDelegationOptionsUploadAndImport(t *testing.T) {
	cases := []struct {
		name     string
		toolKey  string
		args     map[string]any
		wantKey  string // options sub-object key
		wantNil  bool
		wantFile string
		wantSize any // int64 wanted, or nil for absent
	}{
		{
			name:     "drive get_upload_info uses fileName + fileSize",
			toolKey:  "drive.get_upload_info",
			args:     map[string]any{"fileName": "a.pdf", "fileSize": float64(1024)},
			wantKey:  "uploadActionParam",
			wantFile: "a.pdf",
			wantSize: int64(1024),
		},
		{
			name:     "drive get_upload_info fileSize as int normalizes to int64",
			toolKey:  "drive.get_upload_info",
			args:     map[string]any{"fileName": "int.pdf", "fileSize": int(2048)},
			wantKey:  "uploadActionParam",
			wantFile: "int.pdf",
			wantSize: int64(2048),
		},
		{
			name:     "drive commit_upload without size omits fileSize",
			toolKey:  "drive.commit_upload",
			args:     map[string]any{"fileName": "b.pdf"},
			wantKey:  "uploadActionParam",
			wantFile: "b.pdf",
			wantSize: nil,
		},
		{
			name:     "doc get_file_upload_info reads name, emits fileName",
			toolKey:  "doc.get_file_upload_info",
			args:     map[string]any{"name": "c.docx", "fileSize": float64(2048)},
			wantKey:  "uploadActionParam",
			wantFile: "c.docx",
			wantSize: int64(2048),
		},
		{
			name:     "doc commit_uploaded_file reads name",
			toolKey:  "doc.commit_uploaded_file",
			args:     map[string]any{"name": "d.docx"},
			wantKey:  "uploadActionParam",
			wantFile: "d.docx",
			wantSize: nil,
		},
		{
			name:     "create_import_session emits importActionParam",
			toolKey:  "doc.create_import_session",
			args:     map[string]any{"fileName": "e.md", "fileSize": int64(64)},
			wantKey:  "importActionParam",
			wantFile: "e.md",
			wantSize: int64(64),
		},
		{
			name:     "create_import_session joins suffix into base fileName",
			toolKey:  "doc.create_import_session",
			args:     map[string]any{"fileName": "imp", "suffix": "md", "fileSize": int64(12)},
			wantKey:  "importActionParam",
			wantFile: "imp.md",
			wantSize: int64(12),
		},
		{
			name:     "create_import_session keeps fileName that already has extension",
			toolKey:  "doc.create_import_session",
			args:     map[string]any{"fileName": "report.docx", "suffix": "pdf"},
			wantKey:  "importActionParam",
			wantFile: "report.docx",
			wantSize: nil,
		},
		{
			name:     "create_import_session without suffix keeps base fileName (degrade)",
			toolKey:  "doc.create_import_session",
			args:     map[string]any{"fileName": "imp"},
			wantKey:  "importActionParam",
			wantFile: "imp",
			wantSize: nil,
		},
		{
			name:    "create_import_session without fileName yields nil",
			toolKey: "doc.create_import_session",
			args:    map[string]any{"suffix": "md", "fileSize": int64(12)},
			wantNil: true,
		},
		{
			name:    "upload without file name yields nil",
			toolKey: "drive.get_upload_info",
			args:    map[string]any{"fileSize": float64(1)},
			wantNil: true,
		},
		{
			name:    "doc upload folder step without name yields nil",
			toolKey: "doc.get_file_upload_info",
			args:    map[string]any{"folderId": "f-1"},
			wantNil: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := buildDelegationOptions(tc.toolKey, tc.args, "")
			if tc.wantNil {
				if opts != nil {
					t.Fatalf("options = %#v, want nil", opts)
				}
				return
			}
			param, ok := opts[tc.wantKey].(map[string]any)
			if !ok || len(opts) != 1 {
				t.Fatalf("options = %#v, want single %s", opts, tc.wantKey)
			}
			if param["fileName"] != tc.wantFile {
				t.Fatalf("fileName = %v, want %v", param["fileName"], tc.wantFile)
			}
			got, hasSize := param["fileSize"]
			if tc.wantSize == nil {
				if hasSize {
					t.Fatalf("fileSize = %v, want absent", got)
				}
				return
			}
			if got != tc.wantSize {
				t.Fatalf("fileSize = %v (%T), want %v", got, got, tc.wantSize)
			}
		})
	}
}

func TestCrossPlatformCoverageDelegationOptionsCopyMove(t *testing.T) {
	t.Run("copy prefers targetFolderId", func(t *testing.T) {
		opts := buildDelegationOptions("doc.copy_document", map[string]any{
			"targetFolderId": "folder-9", "workspaceId": "ws-1",
		}, "")
		param, ok := opts["copyActionParam"].(map[string]any)
		if !ok || len(opts) != 1 || param["targetNodeId"] != "folder-9" {
			t.Fatalf("options = %#v, want copyActionParam.targetNodeId=folder-9", opts)
		}
	})
	t.Run("move falls back to workspaceId", func(t *testing.T) {
		opts := buildDelegationOptions("doc.move_document", map[string]any{
			"workspaceId": "ws-2",
		}, "")
		param, ok := opts["moveActionParam"].(map[string]any)
		if !ok || param["targetNodeId"] != "ws-2" {
			t.Fatalf("options = %#v, want moveActionParam.targetNodeId=ws-2", opts)
		}
	})
	t.Run("copy without any target yields nil", func(t *testing.T) {
		if opts := buildDelegationOptions("doc.copy_document", map[string]any{}, ""); opts != nil {
			t.Fatalf("options = %#v, want nil", opts)
		}
	})
}

func TestCrossPlatformCoverageDelegationOptionsSetFilePublish(t *testing.T) {
	t.Run("published true injects shareScopeSetParam WEB(9)", func(t *testing.T) {
		opts := buildDelegationOptions("drive.set_file_publish", map[string]any{"published": true}, "")
		param, ok := opts["shareScopeSetParam"].(map[string]any)
		if !ok || len(opts) != 1 {
			t.Fatalf("options = %#v, want single shareScopeSetParam", opts)
		}
		if param["targetScope"] != 9 {
			t.Fatalf("targetScope = %#v (%T), want int(9)", param["targetScope"], param["targetScope"])
		}
	})
	t.Run("published false yields nil", func(t *testing.T) {
		if opts := buildDelegationOptions("drive.set_file_publish", map[string]any{"published": false}, ""); opts != nil {
			t.Fatalf("options = %#v, want nil", opts)
		}
	})
	t.Run("missing published yields nil", func(t *testing.T) {
		if opts := buildDelegationOptions("drive.set_file_publish", map[string]any{"fileId": "f-1"}, ""); opts != nil {
			t.Fatalf("options = %#v, want nil", opts)
		}
	})
}

func TestCrossPlatformCoverageDelegationOptionsPermissionFormats(t *testing.T) {
	t.Run("new members format maps directly", func(t *testing.T) {
		opts := buildDelegationOptions("doc.add_permission", map[string]any{
			"members": []map[string]any{
				{"type": "USER", "id": "u1", "corpId": "corp-x", "roleId": "r1"},
				{"type": "DEPARTMENT", "id": "d1"},
			},
		}, "corp-current")
		members := permissionMembers(t, opts)
		if len(members) != 2 {
			t.Fatalf("members = %#v, want 2", members)
		}
		want0 := map[string]any{"memberType": "USER", "id": "u1", "corpId": "corp-x", "roleId": "r1"}
		if !reflect.DeepEqual(members[0], want0) {
			t.Fatalf("members[0] = %#v, want %#v", members[0], want0)
		}
		want1 := map[string]any{"memberType": "DEPARTMENT", "id": "d1"}
		if !reflect.DeepEqual(members[1], want1) {
			t.Fatalf("members[1] = %#v, want %#v", members[1], want1)
		}
	})

	t.Run("legacy userIds map to USER members with current corpID", func(t *testing.T) {
		opts := buildDelegationOptions("doc.update_permission", map[string]any{
			"userIds": []string{"u1", "u2"},
			"roleId":  "editor",
		}, "corp-current")
		members := permissionMembers(t, opts)
		if len(members) != 2 {
			t.Fatalf("members = %#v, want 2", members)
		}
		want := map[string]any{"memberType": "USER", "id": "u1", "corpId": "corp-current", "roleId": "editor"}
		if !reflect.DeepEqual(members[0], want) {
			t.Fatalf("members[0] = %#v, want %#v", members[0], want)
		}
	})

	t.Run("legacy remove omits roleId", func(t *testing.T) {
		opts := buildDelegationOptions("doc.remove_permission", map[string]any{
			"userIds": []string{"u1"},
		}, "corp-current")
		members := permissionMembers(t, opts)
		if _, has := members[0]["roleId"]; has {
			t.Fatalf("members[0] = %#v, want no roleId for remove", members[0])
		}
		if members[0]["corpId"] != "corp-current" || members[0]["memberType"] != "USER" {
			t.Fatalf("members[0] = %#v", members[0])
		}
	})

	t.Run("legacy with empty corpID yields nil options", func(t *testing.T) {
		opts := buildDelegationOptions("doc.add_permission", map[string]any{
			"userIds": []string{"u1"},
		}, "")
		if opts != nil {
			t.Fatalf("options = %#v, want nil when corpID empty", opts)
		}
	})

	t.Run("legacy userIds skip empty ids", func(t *testing.T) {
		opts := buildDelegationOptions("doc.add_permission", map[string]any{
			"userIds": []string{"", "u1"},
		}, "corp-current")
		members := permissionMembers(t, opts)
		if len(members) != 1 || members[0]["id"] != "u1" {
			t.Fatalf("members = %#v, want single member id=u1 (empty id skipped)", members)
		}
	})

	t.Run("legacy userIds all empty yields nil options", func(t *testing.T) {
		opts := buildDelegationOptions("doc.add_permission", map[string]any{
			"userIds": []string{"", ""},
		}, "corp-current")
		if opts != nil {
			t.Fatalf("options = %#v, want nil when all userIds empty", opts)
		}
	})

	t.Run("neither members nor userIds yields nil options", func(t *testing.T) {
		opts := buildDelegationOptions("doc.add_permission", map[string]any{
			"roleId": "editor",
		}, "corp-current")
		if opts != nil {
			t.Fatalf("options = %#v, want nil when no members/userIds", opts)
		}
	})
}

// permissionMembers unwraps options.permissionManageParam.targetMembers.
func permissionMembers(t *testing.T, opts map[string]any) []map[string]any {
	t.Helper()
	if opts == nil {
		t.Fatal("options = nil, want permissionManageParam")
	}
	if len(opts) != 1 {
		t.Fatalf("options = %#v, want single permissionManageParam", opts)
	}
	param, ok := opts["permissionManageParam"].(map[string]any)
	if !ok {
		t.Fatalf("options = %#v, want permissionManageParam", opts)
	}
	members, ok := param["targetMembers"].([]map[string]any)
	if !ok {
		t.Fatalf("permissionManageParam = %#v, want targetMembers slice", param)
	}
	return members
}

func TestCrossPlatformCoverageDelegationOptionsUnmappedToolYieldsNil(t *testing.T) {
	for _, toolKey := range []string{"doc.update_document", "drive.list_files", "sheet.read_range", "doc"} {
		if opts := buildDelegationOptions(toolKey, map[string]any{"name": "x", "fileName": "y"}, "corp"); opts != nil {
			t.Fatalf("buildDelegationOptions(%q) = %#v, want nil", toolKey, opts)
		}
	}
}

func TestCrossPlatformCoverageResolveCurrentCorpID(t *testing.T) {
	t.Run("registered resolver returns trimmed corpID", func(t *testing.T) {
		setRuntimeCorpID(t, "  corp-42 ", true)
		if got := resolveCurrentCorpID(context.Background()); got != "corp-42" {
			t.Fatalf("resolveCurrentCorpID() = %q, want corp-42", got)
		}
	})
	t.Run("resolver reporting not-ok returns empty", func(t *testing.T) {
		setRuntimeCorpID(t, "corp-x", false)
		if got := resolveCurrentCorpID(context.Background()); got != "" {
			t.Fatalf("resolveCurrentCorpID() = %q, want empty", got)
		}
	})
	t.Run("unregistered returns empty", func(t *testing.T) {
		runtimeDefaultsMu.Lock()
		previous := runtimeDefaults
		runtimeDefaults = make(map[string]edition.RuntimeDefaultFn)
		runtimeDefaultsMu.Unlock()
		t.Cleanup(func() {
			runtimeDefaultsMu.Lock()
			runtimeDefaults = previous
			runtimeDefaultsMu.Unlock()
		})
		if got := resolveCurrentCorpID(context.Background()); got != "" {
			t.Fatalf("resolveCurrentCorpID() = %q, want empty", got)
		}
	})
}

func TestCrossPlatformCoverageDelegationOptionsInjectedIntoCheckArgs(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "create_document", map[string]any{
		"nodeId": "node-1", "name": "季度总结",
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	check := inner.calls[0]
	opts, ok := check.args["options"].(map[string]any)
	if !ok {
		t.Fatalf("check args = %#v, want options key", check.args)
	}
	param, ok := opts["createActionParam"].(map[string]any)
	if !ok || param["name"] != "季度总结.adoc" {
		t.Fatalf("options = %#v, want createActionParam.name=季度总结.adoc", opts)
	}
}

func TestCrossPlatformCoverageDelegationUnmappedToolOmitsOptionsKey(t *testing.T) {
	// Backward compatibility: a tool key that yields no options must not add
	// the options key at all, keeping the check_capability payload byte-for-byte
	// identical to phase 1.
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	_, err := d.CallTool(context.Background(), "doc", "update_document", map[string]any{
		"nodeId": "node-1", "content": "x",
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if _, has := inner.calls[0].args["options"]; has {
		t.Fatalf("check args = %#v, want no options key", inner.calls[0].args)
	}
}

func TestCrossPlatformCoverageDelegationCachePersistsAcrossOptionsBuild(t *testing.T) {
	// The per-node dedup cache must be unaffected by options enrichment: a
	// second identical call for the same tool key + node skips the check.
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	args := map[string]any{"nodeId": "node-1", "name": "文档"}
	if _, err := d.CallTool(context.Background(), "doc", "create_document", args); err != nil {
		t.Fatalf("first CallTool() error = %v", err)
	}
	if _, err := d.CallTool(context.Background(), "doc", "create_document", args); err != nil {
		t.Fatalf("second CallTool() error = %v", err)
	}
	checks := 0
	for _, c := range inner.calls {
		if c.tool == checkCapTool {
			checks++
		}
	}
	if checks != 1 {
		t.Fatalf("check_capability calls = %d, want 1 (cache dedup)", checks)
	}
}

// optionsImportDryRunCaller is a dry-run ToolCaller with empty JQ()/Fields()
// so the import preview renders through deps.Out.PrintJSON without applying a
// sentinel jq/fields filter. It records every call and scripts the
// check_capability response.
type optionsImportDryRunCaller struct {
	calls    []docDelegationCall
	checkRes *edition.ToolResult
}

func (c *optionsImportDryRunCaller) CallTool(_ context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, docDelegationCall{server: serverID, tool: toolName, args: copied})
	if toolName == checkCapTool {
		return c.checkRes, nil
	}
	return textToolResult(`{}`), nil
}

func (*optionsImportDryRunCaller) Format() string { return "json" }
func (*optionsImportDryRunCaller) DryRun() bool   { return true }
func (*optionsImportDryRunCaller) Fields() string { return "" }
func (*optionsImportDryRunCaller) JQ() string     { return "" }

// importDryRunCommand builds a cobra command mirroring `doc import` flags for
// the dry-run delegation parity tests.
func importDryRunCommand(t *testing.T, filePath, workspace, principal string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder", "", "")
	cmd.Flags().String("folder-id", "", "")
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String(FlagPrincipalUserID, "", "")
	mustSet(t, cmd, "file", filePath)
	if workspace != "" {
		mustSet(t, cmd, "workspace", workspace)
	}
	if principal != "" {
		mustSet(t, cmd, FlagPrincipalUserID, principal)
	}
	return cmd
}

func mustSet(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s: %v", name, err)
	}
}

func TestCrossPlatformCoverageImportDryRunDelegationParity(t *testing.T) {
	t.Run("allowed principal previews after delegation check", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":true}`)}
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		cmd := importDryRunCommand(t, writeImportFixture(t, "docx"), "ws-1", "u-principal")
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		check := inner.calls[0]
		if check.tool != checkCapTool || check.args["mcpToolKey"] != "doc.create_import_session" {
			t.Fatalf("check call = %#v, want doc.create_import_session gate", check.args)
		}
		if check.args["nodeId"] != "ws-1" {
			t.Fatalf("check nodeId = %v, want ws-1", check.args["nodeId"])
		}
		opts, ok := check.args["options"].(map[string]any)
		if !ok {
			t.Fatalf("check args = %#v, want importActionParam options", check.args)
		}
		if _, ok := opts["importActionParam"].(map[string]any); !ok {
			t.Fatalf("options = %#v, want importActionParam", opts)
		}
		if !strings.Contains(out.String(), `"dry_run": true`) {
			t.Fatalf("preview = %q, want dry-run preview after allowed check", out.String())
		}
	})

	t.Run("denied principal blocks preview", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":true}`)}
		inner.checkRes = textToolResult(`{"allowed":false,"denialMessage":"未授权"}`)
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		cmd := importDryRunCommand(t, writeImportFixture(t, "docx"), "ws-1", "u-principal")
		err := runImportCommand(cmd, nil, docImportFlowConfig())
		if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
			t.Fatalf("runImportCommand() error = %v, want DELEGATION_AUTH_DENIED", err)
		}
		if strings.Contains(out.String(), "dry_run") {
			t.Fatalf("preview = %q, want no preview when denied", out.String())
		}
	})

	t.Run("no principal keeps preview without any check", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":true}`)}
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		cmd := importDryRunCommand(t, writeImportFixture(t, "docx"), "ws-1", "")
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		for _, c := range inner.calls {
			if c.tool == checkCapTool {
				t.Fatalf("unexpected check_capability call without principal: %#v", c)
			}
		}
		if !strings.Contains(out.String(), `"dry_run": true`) {
			t.Fatalf("preview = %q, want dry-run preview", out.String())
		}
	})
}

// importDryRunCommandWithFolder builds a cobra command with --folder set
// (targeting a specific folder rather than workspace).
func importDryRunCommandWithFolder(t *testing.T, filePath, folder, principal string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder", "", "")
	cmd.Flags().String("folder-id", "", "")
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String(FlagPrincipalUserID, "", "")
	mustSet(t, cmd, "file", filePath)
	if folder != "" {
		mustSet(t, cmd, "folder", folder)
	}
	if principal != "" {
		mustSet(t, cmd, FlagPrincipalUserID, principal)
	}
	return cmd
}

func TestCrossPlatformCoverageImportDryRunDelegationFolderTarget(t *testing.T) {
	t.Run("folder target triggers check with targetFolderId as nodeId", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":true}`)}
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		cmd := importDryRunCommandWithFolder(t, writeImportFixture(t, "docx"), "folder-abc", "u-principal")
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		check := inner.calls[0]
		if check.tool != checkCapTool || check.args["mcpToolKey"] != "doc.create_import_session" {
			t.Fatalf("check call = %#v, want doc.create_import_session gate", check.args)
		}
		// The nodeId for check must be the targetFolderId (folder-abc), proving
		// extractNodeId correctly resolves it.
		if check.args["nodeId"] != "folder-abc" {
			t.Fatalf("check nodeId = %v, want folder-abc (targetFolderId)", check.args["nodeId"])
		}
		opts, ok := check.args["options"].(map[string]any)
		if !ok {
			t.Fatalf("check args = %#v, want importActionParam options", check.args)
		}
		if _, ok := opts["importActionParam"].(map[string]any); !ok {
			t.Fatalf("options = %#v, want importActionParam", opts)
		}
		if !strings.Contains(out.String(), `"dry_run": true`) {
			t.Fatalf("preview = %q, want dry-run preview after allowed check", out.String())
		}
	})

	t.Run("folder denied blocks preview", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":false,"denialMessage":"no access"}`)}
		d := newDocDelegationAuthDecorator(inner)
		_, _ = installHelpersCoreDeps(t, d)

		cmd := importDryRunCommandWithFolder(t, writeImportFixture(t, "docx"), "folder-abc", "u-principal")
		err := runImportCommand(cmd, nil, docImportFlowConfig())
		if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
			t.Fatalf("runImportCommand() error = %v, want DELEGATION_AUTH_DENIED", err)
		}
	})

	t.Run("copy/move nodeId still wins over targetFolderId", func(t *testing.T) {
		// When both nodeId and targetFolderId are present (copy/move scenario),
		// nodeId (source node) must be the check target, not targetFolderId.
		inner := newDocDelegationTestCaller()
		d := newDocDelegationAuthDecorator(inner)
		_, err := d.CallTool(context.Background(), "doc", "copy_document", map[string]any{
			"nodeId": "source-node", "targetFolderId": "dest-folder",
		})
		if err != nil {
			t.Fatalf("CallTool() error = %v", err)
		}
		check := inner.calls[0]
		if check.args["nodeId"] != "source-node" {
			t.Fatalf("check nodeId = %v, want source-node (nodeId beats targetFolderId)", check.args["nodeId"])
		}
	})
}

// docUploadParityInner scripts the doc-space three-step upload backend
// (check_capability -> get_file_upload_info -> commit_uploaded_file) and
// records every call in order so tests can assert the FIRST delegated call
// carries the operation-level options built from the shared step1Args.
type docUploadParityInner struct {
	calls        []docDelegationCall
	checkResText string
}

func (c *docUploadParityInner) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}
	c.calls = append(c.calls, docDelegationCall{server: server, tool: tool, args: copied})
	switch tool {
	case checkCapTool:
		return textToolResult(c.checkResText), nil
	case "get_file_upload_info":
		return textToolResult(`{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`), nil
	case "commit_uploaded_file":
		return textToolResult(`{"dentryUuid":"node-1","name":"x.md"}`), nil
	}
	return textToolResult(`{"ok":true}`), nil
}

func (*docUploadParityInner) Format() string { return "json" }
func (*docUploadParityInner) DryRun() bool   { return false }
func (*docUploadParityInner) Fields() string { return "" }
func (*docUploadParityInner) JQ() string     { return "" }

// runDocUploadRealPath drives the non-dry `doc upload` command against the
// supplied caller and reports whether the HTTP PUT was reached. The real
// execution path is exercised end to end (get_file_upload_info -> PUT ->
// commit_uploaded_file) so the FIRST get_file_upload_info args can be asserted.
func runDocUploadRealPath(t *testing.T, caller edition.ToolCaller, folder, name string) (putReached bool, err error) {
	t.Helper()
	prevDeps := deps
	prevArgs := os.Args
	t.Cleanup(func() {
		deps = prevDeps
		os.Args = prevArgs
		SetHTTPPutFile(nil)
	})
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	os.Args = []string{"dws", "doc"}
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
		putReached = true
		return nil
	})

	file := filepath.Join(t.TempDir(), "src.md")
	if writeErr := os.WriteFile(file, []byte("hello-body"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	root := newDocCommand()
	cmd, _, findErr := root.Find([]string{"upload"})
	if findErr != nil {
		t.Fatalf("find upload command: %v", findErr)
	}
	_ = cmd.Flags().Set("file", file)
	if folder != "" {
		_ = cmd.Flags().Set("folder", folder)
	}
	if name != "" {
		_ = cmd.Flags().Set("name", name)
	}
	return putReached, cmd.RunE(cmd, nil)
}

// TestCrossPlatformCoverageDocUploadStep1ArgsSharedConstructor pins the single
// source of truth docFileUploadInfoArgs: fileSize is unconditional, name is set
// when non-empty, and overwriteNodeId (when present) supersedes folderId. Both
// the folder and overwrite shapes carry name+fileSize so the first capability
// check always has an operation-level uploadActionParam.
func TestCrossPlatformCoverageDocUploadStep1ArgsSharedConstructor(t *testing.T) {
	folder := docFileUploadInfoArgs("x.md", 12, "f1", "w1", "")
	if folder["name"] != "x.md" {
		t.Fatalf("folder mode name = %v, want x.md", folder["name"])
	}
	if folder["fileSize"] != float64(12) {
		t.Fatalf("folder mode fileSize = %v (%T), want float64(12)", folder["fileSize"], folder["fileSize"])
	}
	if folder["folderId"] != "f1" || folder["workspaceId"] != "w1" {
		t.Fatalf("folder mode target = %#v, want folderId=f1 workspaceId=w1", folder)
	}
	if _, has := folder["overwriteNodeId"]; has {
		t.Fatalf("folder mode must not carry overwriteNodeId: %#v", folder)
	}

	overwrite := docFileUploadInfoArgs("x.md", 34, "f1", "w1", "node-9")
	if overwrite["name"] != "x.md" || overwrite["fileSize"] != float64(34) {
		t.Fatalf("overwrite mode name/size = %#v, want name=x.md fileSize=34", overwrite)
	}
	if overwrite["overwriteNodeId"] != "node-9" {
		t.Fatalf("overwrite mode overwriteNodeId = %v, want node-9", overwrite["overwriteNodeId"])
	}
	if _, has := overwrite["folderId"]; has {
		t.Fatalf("overwrite mode must exclude folderId (mutually exclusive): %#v", overwrite)
	}

	if _, has := docFileUploadInfoArgs("", 0, "", "", "")["name"]; has {
		t.Fatal("empty name must be omitted")
	}
}

// TestCrossPlatformCoverageDocUploadRealFirstCallCarriesNameSize proves the
// standard `doc upload` real execution issues get_file_upload_info as its first
// call already carrying name+fileSize (previously only folderId/workspaceId).
func TestCrossPlatformCoverageDocUploadRealFirstCallCarriesNameSize(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`},
		{text: `{"ok":true}`},
	}}
	putReached, err := runDocUploadRealPath(t, caller, "f1", "renamed.md")
	if err != nil {
		t.Fatalf("doc upload real path error = %v", err)
	}
	if !putReached {
		t.Fatal("expected HTTP PUT to be reached on the allow/no-delegation path")
	}
	if len(caller.toolLog) == 0 || caller.toolLog[0] != "get_file_upload_info" {
		t.Fatalf("first tool = %#v, want get_file_upload_info", caller.toolLog)
	}
	first := caller.argsLog[0]
	if first["name"] != "renamed.md" {
		t.Fatalf("first get_file_upload_info name = %v, want renamed.md (regression: name dropped)", first["name"])
	}
	if first["fileSize"] != float64(len("hello-body")) {
		t.Fatalf("first get_file_upload_info fileSize = %v, want %d", first["fileSize"], len("hello-body"))
	}
	if first["folderId"] != "f1" {
		t.Fatalf("first get_file_upload_info folderId = %v, want f1", first["folderId"])
	}
}

// TestCrossPlatformCoverageDocImportUploadFallbackFirstCallCarriesNameSize
// covers the import upload fallback (docSpaceUploadCommitText): its first real
// get_file_upload_info must also carry name+fileSize so the fallback authorizes
// precisely before streaming bytes.
func TestCrossPlatformCoverageDocImportUploadFallbackFirstCallCarriesNameSize(t *testing.T) {
	caller := &docImportTargetCaller{responses: map[string][]scriptedToolStep{
		"get_file_upload_info": {{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`}},
		"commit_uploaded_file": {{text: `{"dentryUuid":"node-1","name":"page.html"}`}},
	}}
	if _, err := runDocImportTargetFlow(t, caller, "html", "folder-1", ""); err != nil {
		t.Fatalf("import upload fallback error = %v", err)
	}
	if len(caller.calls) == 0 || caller.calls[0].tool != "get_file_upload_info" {
		t.Fatalf("first fallback call = %#v, want get_file_upload_info", caller.calls)
	}
	first := caller.calls[0].args
	if name, ok := first["name"].(string); !ok || name == "" {
		t.Fatalf("fallback get_file_upload_info name = %#v, want non-empty (regression: name dropped)", first["name"])
	}
	if size, ok := first["fileSize"].(float64); !ok || size <= 0 {
		t.Fatalf("fallback get_file_upload_info fileSize = %#v, want positive float64", first["fileSize"])
	}
}

// TestCrossPlatformCoverageDocUploadDelegationDeniedBeforePut is the core
// auto-CR guard: when the principal is denied, the very first delegated call
// (check_capability for get_file_upload_info) already carries the
// uploadActionParam operation options and the command fails BEFORE any HTTP PUT
// or commit_uploaded_file — no orphaned object is left behind.
func TestCrossPlatformCoverageDocUploadDelegationDeniedBeforePut(t *testing.T) {
	inner := &docUploadParityInner{checkResText: `{"allowed":false,"denialReason":"NO_PERM","denialMessage":"没有该文档的委托权限"}`}
	decorator := newDocDelegationAuthDecorator(inner)

	putReached, err := runDocUploadRealPath(t, decorator, "f1", "x.md")
	if err == nil {
		t.Fatal("expected delegation denial error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
		t.Fatalf("Error() = %q, want [DELEGATION_AUTH_DENIED] prefix", err.Error())
	}
	if putReached {
		t.Fatal("HTTP PUT reached before authorization: rejection must happen before uploading bytes")
	}
	if len(inner.calls) != 1 {
		t.Fatalf("inner calls = %#v, want only the check_capability call (no upload/commit passthrough)", inner.calls)
	}
	check := inner.calls[0]
	if check.tool != checkCapTool || check.args["mcpToolKey"] != "doc.get_file_upload_info" {
		t.Fatalf("first call = %#v, want check_capability for doc.get_file_upload_info", check)
	}
	assertUploadActionParam(t, check.args, "x.md")
}

// TestCrossPlatformCoverageDocUploadDelegationAllowedFirstCallMatches verifies
// the allow path: the first precise tool call is exactly the shared step1Args
// (name+fileSize+folderId) and its capability check carried the matching
// uploadActionParam, i.e. precheck options == real first-call options.
func TestCrossPlatformCoverageDocUploadDelegationAllowedFirstCallMatches(t *testing.T) {
	inner := &docUploadParityInner{checkResText: `{"allowed":true}`}
	decorator := newDocDelegationAuthDecorator(inner)

	putReached, err := runDocUploadRealPath(t, decorator, "f1", "x.md")
	if err != nil {
		t.Fatalf("doc upload allow path error = %v", err)
	}
	if !putReached {
		t.Fatal("expected HTTP PUT to run after the capability check allowed the principal")
	}
	if len(inner.calls) < 2 {
		t.Fatalf("inner calls = %#v, want check followed by get_file_upload_info passthrough", inner.calls)
	}
	check := inner.calls[0]
	if check.tool != checkCapTool || check.args["mcpToolKey"] != "doc.get_file_upload_info" {
		t.Fatalf("call[0] = %#v, want check_capability for doc.get_file_upload_info", check)
	}
	assertUploadActionParam(t, check.args, "x.md")

	first := inner.calls[1]
	if first.tool != "get_file_upload_info" {
		t.Fatalf("call[1] = %#v, want get_file_upload_info passthrough", first)
	}
	if first.args["name"] != "x.md" || first.args["fileSize"] != float64(len("hello-body")) || first.args["folderId"] != "f1" {
		t.Fatalf("first precise call args = %#v, want name=x.md fileSize=%d folderId=f1", first.args, len("hello-body"))
	}
}

// assertUploadActionParam checks the capability check options carry the
// operation-level uploadActionParam with the given fileName and a fileSize.
func assertUploadActionParam(t *testing.T, checkArgs map[string]any, wantFileName string) {
	t.Helper()
	opts, ok := checkArgs["options"].(map[string]any)
	if !ok {
		t.Fatalf("check options = %#v, want operation-level options map", checkArgs["options"])
	}
	param, ok := opts["uploadActionParam"].(map[string]any)
	if !ok {
		t.Fatalf("options = %#v, want uploadActionParam", opts)
	}
	if param["fileName"] != wantFileName {
		t.Fatalf("uploadActionParam.fileName = %v, want %s", param["fileName"], wantFileName)
	}
	if _, has := param["fileSize"]; !has {
		t.Fatalf("uploadActionParam = %#v, want fileSize present", param)
	}
}

// countCheckCapabilityCalls 统计脚本化 caller 中 check_capability 的调用次数，
// 供续调放行测试断言「续调未触发额外远程鉴权」。
func countCheckCapabilityCalls(calls []docDelegationCall) int {
	n := 0
	for _, c := range calls {
		if c.tool == checkCapTool {
			n++
		}
	}
	return n
}

// TestCrossPlatformCoverageDocDelegationAuthImportContinuationAllowed drives the
// real (non-dry-run) import chain: create_import_session carries the target
// node and is gated by check_capability; the follow-up confirm_import and
// query_import_task continuations carry only sessionId/taskId (no node) yet must
// be allowed to pass through instead of being rejected as NOT_SUPPORTED.
func TestCrossPlatformCoverageDocDelegationAuthImportContinuationAllowed(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	ctx := context.Background()

	// Step 1: create_import_session — carries targetFolderId, gated by check.
	if _, err := d.CallTool(ctx, "doc", "create_import_session", map[string]any{
		"targetFolderId": "folder-1", "fileName": "f", "suffix": "md", "fileSize": int64(3),
	}); err != nil {
		t.Fatalf("create_import_session error = %v", err)
	}
	if got := countCheckCapabilityCalls(inner.calls); got != 1 {
		t.Fatalf("check_capability calls after session = %d, want 1", got)
	}

	// Step 2: confirm_import — only sessionId, no node. Must be allowed.
	res2, err := d.CallTool(ctx, "doc", "confirm_import", map[string]any{"sessionId": "s1"})
	if err != nil {
		t.Fatalf("confirm_import continuation error = %v, want allowed", err)
	}
	if res2 != inner.passRes {
		t.Fatalf("confirm_import result = %#v, want passthrough", res2)
	}

	// Step 3: query_import_task — only taskId, no node. Must be allowed.
	res3, err := d.CallTool(ctx, "doc", "query_import_task", map[string]any{"taskId": "t1"})
	if err != nil {
		t.Fatalf("query_import_task continuation error = %v, want allowed", err)
	}
	if res3 != inner.passRes {
		t.Fatalf("query_import_task result = %#v, want passthrough", res3)
	}

	// Continuations must NOT trigger any extra check_capability round-trip.
	if got := countCheckCapabilityCalls(inner.calls); got != 1 {
		t.Fatalf("total check_capability calls = %d, want 1 (only the session)", got)
	}
	// calls: check + create_import_session + confirm_import + query_import_task.
	if len(inner.calls) != 4 {
		t.Fatalf("inner calls = %d, want 4", len(inner.calls))
	}
	if inner.calls[2].tool != "confirm_import" || inner.calls[3].tool != "query_import_task" {
		t.Fatalf("continuation passthrough order = %q,%q", inner.calls[2].tool, inner.calls[3].tool)
	}
}

// TestCrossPlatformCoverageDocDelegationAuthUploadCommitContinuationAllowed
// drives the real upload chain: the first credential call carries the node and
// is gated by check_capability; the commit continuation carrying no node must be
// allowed. Covers both drive (get_upload_info→commit_upload) and doc
// (commit_uploaded_file) commit tools.
func TestCrossPlatformCoverageDocDelegationAuthUploadCommitContinuationAllowed(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	ctx := context.Background()

	// drive get_upload_info — carries parentId, gated by check.
	if _, err := d.CallTool(ctx, "drive", "get_upload_info", map[string]any{
		"parentId": "p1", "fileName": "f.txt", "fileSize": int64(5),
	}); err != nil {
		t.Fatalf("get_upload_info error = %v", err)
	}
	if got := countCheckCapabilityCalls(inner.calls); got != 1 {
		t.Fatalf("check_capability calls after get_upload_info = %d, want 1", got)
	}

	// drive commit_upload — no node (only uploadId). Must be allowed.
	if res, err := d.CallTool(ctx, "drive", "commit_upload", map[string]any{
		"uploadId": "u1", "fileName": "f.txt", "fileSize": int64(5),
	}); err != nil {
		t.Fatalf("commit_upload continuation error = %v, want allowed", err)
	} else if res != inner.passRes {
		t.Fatalf("commit_upload result = %#v, want passthrough", res)
	}

	// doc commit_uploaded_file — no node (only uploadKey/name). Must be allowed.
	if res, err := d.CallTool(ctx, "doc", "commit_uploaded_file", map[string]any{
		"uploadKey": "k1", "name": "f.txt",
	}); err != nil {
		t.Fatalf("commit_uploaded_file continuation error = %v, want allowed", err)
	} else if res != inner.passRes {
		t.Fatalf("commit_uploaded_file result = %#v, want passthrough", res)
	}

	// No commit continuation should trigger an extra check_capability round-trip.
	if got := countCheckCapabilityCalls(inner.calls); got != 1 {
		t.Fatalf("total check_capability calls = %d, want 1 (only get_upload_info)", got)
	}
}

// TestCrossPlatformCoverageDocDelegationAuthContinuationExemptionKeepsIndependentBlocked
// guards the exemption boundary: node-less INDEPENDENT commands (search / create)
// that are NOT flow continuations must still be rejected as NOT_SUPPORTED, so the
// exemption set never leaks blanket bypass to real independent write commands.
func TestCrossPlatformCoverageDocDelegationAuthContinuationExemptionKeepsIndependentBlocked(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		server string
		tool   string
		args   map[string]any
	}{
		{"doc", "search_documents", map[string]any{"query": "x"}},
		{"wiki", "search_nodes", map[string]any{"keyword": "y"}},
		{"doc", "create_document", map[string]any{"name": "no-parent"}},
	}
	for _, tc := range cases {
		inner := newDocDelegationTestCaller()
		d := newDocDelegationAuthDecorator(inner)
		_, err := d.CallTool(ctx, tc.server, tc.tool, tc.args)
		if err == nil {
			t.Fatalf("%s/%s error = nil, want DELEGATION_AUTH_NOT_SUPPORTED", tc.server, tc.tool)
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationNotSupported {
			t.Fatalf("%s/%s error = %v, want CLIError code %q", tc.server, tc.tool, err, codeDelegationNotSupported)
		}
		if len(inner.calls) != 0 {
			t.Fatalf("%s/%s inner calls = %d, want 0 (blocked before any remote call)", tc.server, tc.tool, len(inner.calls))
		}
	}
}

// TestCrossPlatformCoverageDocDelegationAuthIndependentContinuationToolBlocked
// guards the P2-3 security fix: the flow-continuation tool NAMES
// (confirm_import / query_import_task / commit_upload / commit_uploaded_file)
// can also be issued by INDEPENDENT commands with no preceding node-bearing
// check (e.g. `doc import get` / `sheet import get` fire query_import_task with
// only a taskId; a manual `drive commit` fires commit_upload). Without a prior
// sessionAuthorized first step, these node-less calls must fall back to
// DELEGATION_AUTH_NOT_SUPPORTED rather than be blanket-exempted — otherwise the
// delegation check is entirely bypassed and the tool runs as the current login
// identity (delegation bypass).
func TestCrossPlatformCoverageDocDelegationAuthIndependentContinuationToolBlocked(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		server string
		tool   string
		args   map[string]any
	}{
		{"independent import get (query_import_task, taskId only)", "doc", "query_import_task", map[string]any{"taskId": "t1"}},
		{"independent sheet import get (query_import_task, taskId only)", "doc", "query_import_task", map[string]any{"taskId": "t2"}},
		{"manual confirm_import (sessionId only)", "doc", "confirm_import", map[string]any{"sessionId": "s1"}},
		{"manual drive commit (commit_upload, uploadId only)", "drive", "commit_upload", map[string]any{"uploadId": "u1", "fileName": "f.txt", "fileSize": int64(5)}},
		{"manual commit_uploaded_file (uploadKey only)", "doc", "commit_uploaded_file", map[string]any{"uploadKey": "k1", "name": "f.txt"}},
	}
	for _, tc := range cases {
		inner := newDocDelegationTestCaller()
		d := newDocDelegationAuthDecorator(inner)
		// No preceding node-bearing check → sessionAuthorized stays false.
		_, err := d.CallTool(ctx, tc.server, tc.tool, tc.args)
		if err == nil {
			t.Fatalf("%s: error = nil, want DELEGATION_AUTH_NOT_SUPPORTED (must not bypass)", tc.name)
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationNotSupported {
			t.Fatalf("%s: error = %v, want CLIError code %q", tc.name, err, codeDelegationNotSupported)
		}
		if len(inner.calls) != 0 {
			t.Fatalf("%s: inner calls = %d, want 0 (blocked before any remote call)", tc.name, len(inner.calls))
		}
	}
}

// TestCrossPlatformCoverageDocDelegationAuthSessionAuthorizedIsPerDecorator
// verifies sessionAuthorized is scoped to a single decorator instance: a
// node-bearing allowed check on one decorator must NOT authorize node-less
// continuations on a DIFFERENT decorator (no cross-session leakage).
func TestCrossPlatformCoverageDocDelegationAuthSessionAuthorizedIsPerDecorator(t *testing.T) {
	ctx := context.Background()

	// Decorator A performs a node-bearing allowed check → sessionAuthorized.
	innerA := newDocDelegationTestCaller()
	dA := newDocDelegationAuthDecorator(innerA)
	if _, err := dA.CallTool(ctx, "doc", "create_import_session", map[string]any{
		"targetFolderId": "folder-1", "fileName": "f", "suffix": "md", "fileSize": int64(3),
	}); err != nil {
		t.Fatalf("decorator A create_import_session error = %v", err)
	}

	// Decorator B (fresh session) sees a node-less continuation. Must be blocked.
	innerB := newDocDelegationTestCaller()
	dB := newDocDelegationAuthDecorator(innerB)
	_, err := dB.CallTool(ctx, "doc", "query_import_task", map[string]any{"taskId": "t1"})
	if err == nil {
		t.Fatal("decorator B query_import_task error = nil, want NOT_SUPPORTED (no cross-session leakage)")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationNotSupported {
		t.Fatalf("decorator B error = %v, want CLIError code %q", err, codeDelegationNotSupported)
	}
	if len(innerB.calls) != 0 {
		t.Fatalf("decorator B inner calls = %d, want 0", len(innerB.calls))
	}
}

// TestCrossPlatformCoverageImportUploadFallbackDryRunDelegationParity guards the
// P2-1 fix: when the input format falls outside the import whitelist, doc import
// falls back to the doc-space file-upload链路 whose first real call is
// doc.get_file_upload_info. The dry-run branch of runImportUploadFallback must
// run markdownDryRunDelegationPrecheck on that first call BEFORE rendering any
// warning/preview, so a --principal-user-id run that would be rejected at the
// real get_file_upload_info is intercepted rather than falsely previewed as
// executable.
func TestCrossPlatformCoverageImportUploadFallbackDryRunDelegationParity(t *testing.T) {
	t.Run("allowed principal previews upload fallback after delegation check", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":true}`)}
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		// pdf is outside the import whitelist → uploadFallback path.
		cmd := importDryRunCommand(t, writeImportFixture(t, "pdf"), "ws-1", "u-principal")
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		check := inner.calls[0]
		if check.tool != checkCapTool || check.args["mcpToolKey"] != "doc.get_file_upload_info" {
			t.Fatalf("check call = %#v, want doc.get_file_upload_info gate", check.args)
		}
		if check.args["nodeId"] != "ws-1" {
			t.Fatalf("check nodeId = %v, want ws-1 (workspaceId)", check.args["nodeId"])
		}
		opts, ok := check.args["options"].(map[string]any)
		if !ok {
			t.Fatalf("check args = %#v, want uploadActionParam options", check.args)
		}
		if _, ok := opts["uploadActionParam"].(map[string]any); !ok {
			t.Fatalf("options = %#v, want uploadActionParam", opts)
		}
		if !strings.Contains(out.String(), `"dry_run": true`) || !strings.Contains(out.String(), `"fallback": "upload"`) {
			t.Fatalf("preview = %q, want upload-fallback dry-run preview after allowed check", out.String())
		}
	})

	t.Run("denied principal blocks upload fallback preview", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":false,"denialMessage":"未授权"}`)}
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		cmd := importDryRunCommand(t, writeImportFixture(t, "pdf"), "ws-1", "u-principal")
		err := runImportCommand(cmd, nil, docImportFlowConfig())
		if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
			t.Fatalf("runImportCommand() error = %v, want DELEGATION_AUTH_DENIED", err)
		}
		if strings.Contains(out.String(), "dry_run") || strings.Contains(out.String(), "fallback") {
			t.Fatalf("preview = %q, want no preview when upload fallback denied", out.String())
		}
	})

	t.Run("no principal keeps upload fallback preview without any check", func(t *testing.T) {
		inner := &optionsImportDryRunCaller{checkRes: textToolResult(`{"allowed":true}`)}
		d := newDocDelegationAuthDecorator(inner)
		out, _ := installHelpersCoreDeps(t, d)

		cmd := importDryRunCommand(t, writeImportFixture(t, "pdf"), "ws-1", "")
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		for _, c := range inner.calls {
			if c.tool == checkCapTool {
				t.Fatalf("unexpected check_capability call without principal: %#v", c)
			}
		}
		if !strings.Contains(out.String(), `"dry_run": true`) || !strings.Contains(out.String(), `"fallback": "upload"`) {
			t.Fatalf("preview = %q, want upload-fallback dry-run preview", out.String())
		}
	})
}

// TestCrossPlatformCoverageDocDelegationAuthDefaultTargetListSpacesBlocked
// documents the P2-2 finding: resolving the default import target on the REAL
// execution path calls drive.list_spaces with only {spaceType}, i.e. no node
// identifier. Under delegation (--principal-user-id) that node-less call is
// rejected as DELEGATION_AUTH_NOT_SUPPORTED by the same decorator, BEFORE the
// import ever starts. Hence a delegated import without an explicit
// --folder/--workspace is blocked at list_spaces on the real path exactly as
// the dry-run precheck blocks it at create_import_session — the dry-run
// rejection is faithful parity, not an over-strict dry-run limitation. Letting
// dry-run "resolve then allow" would require exempting node-less list_spaces,
// which would reopen the very delegation bypass the continuation-exemption
// hardening closes; therefore P2-2 is intentionally NOT changed.
func TestCrossPlatformCoverageDocDelegationAuthDefaultTargetListSpacesBlocked(t *testing.T) {
	inner := newDocDelegationTestCaller()
	d := newDocDelegationAuthDecorator(inner)
	// Mirror resolveDefaultDocImportTarget's real call: node-less list_spaces.
	_, err := d.CallTool(context.Background(), "drive", "list_spaces", map[string]any{"spaceType": "orgSpace"})
	if err == nil {
		t.Fatal("list_spaces error = nil, want DELEGATION_AUTH_NOT_SUPPORTED (default-target resolution is node-less under delegation)")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != codeDelegationNotSupported {
		t.Fatalf("list_spaces error = %v, want CLIError code %q", err, codeDelegationNotSupported)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("inner calls = %d, want 0 (blocked before any remote call)", len(inner.calls))
	}
}
