// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// ── 共享 fixture：passthrough caller 与 CaptureOut helper ──

// setShareScopePassthroughCaller scripts the MCP response text so passthrough
// fidelity tests can assert false/null/absent are kept distinguishable.
type setShareScopePassthroughCaller struct {
	calls        []guardedMutationCall
	responseText string
}

func (c *setShareScopePassthroughCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, guardedMutationCall{productID: productID, toolName: toolName, args: args})
	text := c.responseText
	if text == "" {
		text = `{}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*setShareScopePassthroughCaller) Format() string { return "json" }
func (*setShareScopePassthroughCaller) DryRun() bool   { return false }
func (*setShareScopePassthroughCaller) Fields() string { return "" }
func (*setShareScopePassthroughCaller) JQ() string     { return "" }

// setShareScopeErrorCaller 强制 CallTool 返回指定错误，用于验证 ResultCall
// 错误路径能正确传播到框架统一信封。
type setShareScopeErrorCaller struct {
	calls []guardedMutationCall
	err   error
}

func (c *setShareScopeErrorCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, guardedMutationCall{productID: productID, toolName: toolName, args: args})
	return nil, c.err
}

func (*setShareScopeErrorCaller) Format() string { return "json" }
func (*setShareScopeErrorCaller) DryRun() bool   { return false }
func (*setShareScopeErrorCaller) Fields() string { return "" }
func (*setShareScopeErrorCaller) JQ() string     { return "" }

// executeGuardedMutationCommandCaptureOut mirrors executeGuardedMutationCommand
// but tees the Formatter data/diagnostic streams into out so a test can assert
// on the --dry-run preview text (e.g. password masking) or response passthrough.
func executeGuardedMutationCommandCaptureOut(t *testing.T, caller edition.ToolCaller, build func() *cobra.Command, out *bytes.Buffer, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	InitDeps(caller)
	deps.Out.w = out
	deps.Out.errW = out
	root := build()
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(out)
	root.SetErr(out)
	if root.InOrStdin() == os.Stdin {
		root.SetIn(strings.NewReader(""))
	}
	root.SetArgs(args)
	ctx, _ := output.WithResultStore(context.Background())
	// Execute through corecmd.ExecuteContextCForTest so the self-built tree runs
	// the same PrepareCommandTree adapters (FlagErrorFunc/ValidationErrorFunc)
	// as production, per the AGENTS.md test-execution helper discipline.
	executed, err := corecmd.ExecuteContextCForTest(root, ctx)
	if err != nil {
		return err
	}
	_, _, err = output.EmitStoredResult(executed)
	return err
}

// ── drive permission set-share-scope：路由 / 别名 / 参数三态装配 ──

func TestCrossPlatformCoverageDrivePermissionSetShareScopeRoutesToDrive(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "set-share-scope", "--node", "node-1", "--visibility", "PRIVATE", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want exactly one", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "drive" || call.toolName != "set_share_scope" {
		t.Fatalf("call = %#v, want drive/set_share_scope", call)
	}
	want := map[string]any{"nodeId": "node-1", "targetVisibility": "PRIVATE"}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("args = %#v, want %#v", call.args, want)
	}
}

func TestCrossPlatformCoverageDrivePermissionSetShareScopeHiddenAliases(t *testing.T) {
	for _, alias := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		caller := &guardedMutationCaller{}
		err := executeGuardedMutationCommand(t, caller, newDriveCommand,
			"permission", "set-share-scope", "--"+alias, "node-alias", "--visibility", "PRIVATE", "--yes")
		if err != nil {
			t.Fatalf("alias --%s: %v", alias, err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("alias --%s calls = %#v, want exactly one", alias, caller.calls)
		}
		call := caller.calls[0]
		if call.productID != "drive" || call.toolName != "set_share_scope" {
			t.Fatalf("alias --%s call = %#v", alias, call)
		}
		if call.args["nodeId"] != "node-alias" {
			t.Fatalf("alias --%s args = %#v, want nodeId=node-alias", alias, call.args)
		}
	}
}

func TestCrossPlatformCoverageDrivePermissionSetShareScopeArgAssembly(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "PRIVATE minimal sends only nodeId+visibility",
			args: []string{"--node", "n1", "--visibility", "PRIVATE"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "PRIVATE"},
		},
		{
			name: "ORGANIZATION role",
			args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--role", "READER"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "ORGANIZATION", "defaultRole": "READER"},
		},
		{
			name: "ORGANIZATION partner true",
			args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--partner"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "ORGANIZATION", "partnerIncluded": true},
		},
		{
			name: "ORGANIZATION partner explicit false is still sent",
			args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--partner=false"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "ORGANIZATION", "partnerIncluded": false},
		},
		{
			name: "ORGANIZATION can-search + can-recommend",
			args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--can-search", "--can-recommend"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "ORGANIZATION", "canSearch": true, "canRecommend": true},
		},
		{
			name: "ORGANIZATION can-search explicit false",
			args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--can-search=false"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "ORGANIZATION", "canSearch": false},
		},
		{
			name: "ORGANIZATION all optional flags",
			args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--role", "EDITOR", "--partner", "--can-search", "--can-recommend"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "ORGANIZATION", "defaultRole": "EDITOR", "partnerIncluded": true, "canSearch": true, "canRecommend": true},
		},
		{
			name: "PUBLIC expire-days 0 means permanent and is sent",
			args: []string{"--node", "n1", "--visibility", "PUBLIC", "--expire-days", "0"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "PUBLIC", "expireDays": 0},
		},
		{
			name: "PUBLIC expire-days positive",
			args: []string{"--node", "n1", "--visibility", "PUBLIC", "--expire-days", "7"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "PUBLIC", "expireDays": 7},
		},
		{
			name: "PUBLIC password non-empty sets requirePassword true",
			args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "ab12"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "PUBLIC", "requirePassword": true, "password": "ab12"},
		},
		{
			name: "PUBLIC password empty clears (requirePassword false, no password key)",
			args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", ""},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "PUBLIC", "requirePassword": false},
		},
		{
			name: "PUBLIC combined role+password+expire-days",
			args: []string{"--node", "n1", "--visibility", "PUBLIC", "--role", "DOWNLOADER", "--password", "AB12", "--expire-days", "30"},
			want: map[string]any{"nodeId": "n1", "targetVisibility": "PUBLIC", "defaultRole": "DOWNLOADER", "requirePassword": true, "password": "AB12", "expireDays": 30},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			full := append([]string{"permission", "set-share-scope"}, tt.args...)
			full = append(full, "--yes")
			if err := executeGuardedMutationCommand(t, caller, newDriveCommand, full...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v, want exactly one", caller.calls)
			}
			if !reflect.DeepEqual(caller.calls[0].args, tt.want) {
				t.Fatalf("args = %#v, want %#v", caller.calls[0].args, tt.want)
			}
		})
	}
}

// ── drive permission set-share-scope：本地校验 exit3 全部拒绝分支且零 RPC ──

func TestCrossPlatformCoverageDrivePermissionSetShareScopeValidationRejects(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{name: "missing node", args: []string{"--visibility", "PRIVATE"}, wantMsg: "missing required flag(s): --node"},
		{name: "missing visibility", args: []string{"--node", "n1"}, wantMsg: "missing required flag(s): --visibility"},
		{name: "invalid visibility enum", args: []string{"--node", "n1", "--visibility", "private"}, wantMsg: "不合法"},
		{name: "PRIVATE + role", args: []string{"--node", "n1", "--visibility", "PRIVATE", "--role", "READER"}, wantMsg: "PRIVATE 档位不适用 --role"},
		{name: "PRIVATE + partner", args: []string{"--node", "n1", "--visibility", "PRIVATE", "--partner"}, wantMsg: "PRIVATE 档位不适用 --partner"},
		{name: "PRIVATE + can-search", args: []string{"--node", "n1", "--visibility", "PRIVATE", "--can-search"}, wantMsg: "PRIVATE 档位不适用 --can-search"},
		{name: "PRIVATE + can-recommend", args: []string{"--node", "n1", "--visibility", "PRIVATE", "--can-recommend"}, wantMsg: "PRIVATE 档位不适用 --can-recommend"},
		{name: "PRIVATE + password", args: []string{"--node", "n1", "--visibility", "PRIVATE", "--password", "ab12"}, wantMsg: "PRIVATE 档位不适用 --password"},
		{name: "PRIVATE + expire-days", args: []string{"--node", "n1", "--visibility", "PRIVATE", "--expire-days", "7"}, wantMsg: "PRIVATE 档位不适用 --expire-days"},
		{name: "ORGANIZATION + password", args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--password", "ab12"}, wantMsg: "ORGANIZATION（企业内公开）档位不适用 --password"},
		{name: "ORGANIZATION + expire-days", args: []string{"--node", "n1", "--visibility", "ORGANIZATION", "--expire-days", "7"}, wantMsg: "ORGANIZATION（企业内公开）档位不适用 --expire-days"},
		{name: "PUBLIC + partner", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--partner"}, wantMsg: "PUBLIC（互联网公开）档位不适用 --partner"},
		{name: "PUBLIC + can-search", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--can-search"}, wantMsg: "PUBLIC（互联网公开）档位不适用 --can-search"},
		{name: "PUBLIC + can-recommend", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--can-recommend"}, wantMsg: "PUBLIC（互联网公开）档位不适用 --can-recommend"},
		{name: "PUBLIC expire-days negative", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--expire-days", "-1"}, wantMsg: "--expire-days 不能为负数"},
		{name: "PUBLIC password too short", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "abc"}, wantMsg: "必须为4位英文字母或数字"},
		{name: "PUBLIC password too long", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "abcde"}, wantMsg: "必须为4位英文字母或数字"},
		{name: "PUBLIC password with space", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "ab 1"}, wantMsg: "必须为4位英文字母或数字"},
		{name: "PUBLIC password leading space not trimmed", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", " ab1"}, wantMsg: "必须为4位英文字母或数字"},
		{name: "PUBLIC password non-alphanumeric", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "ab-1"}, wantMsg: "必须为4位英文字母或数字"},
		// 纯空白密码不等于"清除"：清除语义只认 --password ""，任何非空串（含全
		// 空白）都必须走格式校验被拒，否则会把误输入当成一次静默的密码摘除。
		{name: "PUBLIC password whitespace only not treated as clear", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "  "}, wantMsg: "必须为4位英文字母或数字"},
		// 尾随空格不 trim：与已有的 leading-space 用例对称，锁定"逐字节匹配、
		// 不做任何空白归一化"的判定语义。
		{name: "PUBLIC password trailing space not trimmed", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--password", "ab1 "}, wantMsg: "必须为4位英文字母或数字"},
		// 溢出：--expire-days 是 Int flag，超 int64 的值在 flag 解析阶段就被 pflag
		// 拒绝（早于 ValidateRequired / Validate）。框架 PrepareCommandTree 安装的
		// FlagErrorFunc 会把该解析错误归一化为 typed 校验错误（CategoryValidation /
		// exit 3，reason invalid_flag），与其余本地校验拒绝分支同码。固化实际文案，
		// 防止后续把 flag 改成 string 自行解析时无声改变外部契约。
		{name: "PUBLIC expire-days overflow", args: []string{"--node", "n1", "--visibility", "PUBLIC", "--expire-days", "99999999999999999999"}, wantMsg: `invalid argument "99999999999999999999" for "--expire-days" flag: strconv.ParseInt: parsing "99999999999999999999": value out of range`},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			full := append([]string{"permission", "set-share-scope"}, tt.args...)
			err := executeGuardedMutationCommand(t, caller, newDriveCommand, full...)
			if err == nil {
				t.Fatalf("args %v: expected rejection, got nil", tt.args)
			}
			if got := apperrors.ExitCode(err); got != apperrors.ExitCodeValidation {
				t.Fatalf("exit code = %d, want %d (err = %v)", got, apperrors.ExitCodeValidation, err)
			}
			var appErr *apperrors.Error
			if !stderrors.As(err, &appErr) || appErr.Category != apperrors.CategoryValidation {
				t.Fatalf("err = %#v, want validation category (exit 3)", err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %q, want contains %q", err.Error(), tt.wantMsg)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("calls = %#v, want zero RPC on local validation failure", caller.calls)
			}
		})
	}
}

// ── drive permission set-share-scope：ResultSpec 返回值契约 ──

func TestCrossPlatformCoverageDrivePermissionSetShareScopeResultContract(t *testing.T) {
	drive := newDriveCommand()
	leaf, _, err := drive.Find([]string{"permission", "set-share-scope"})
	if err != nil || leaf == nil {
		t.Fatalf("find drive permission set-share-scope: command=%v err=%v", leaf, err)
	}
	final, ok := contractfinal.RuntimeContractFinal(leaf)
	if !ok || final.Identity == nil || final.Identity.CanonicalPath != "drive.set_share_scope" {
		t.Fatalf("set-share-scope ContractFinal identity = %#v, found = %v", final.Identity, ok)
	}
	if final.Result == nil {
		t.Fatal("set-share-scope final Result is nil")
	}
	wantOutcomes := []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}
	if !reflect.DeepEqual(final.Result.Outcomes, wantOutcomes) {
		t.Fatalf("outcomes = %#v, want %#v", final.Result.Outcomes, wantOutcomes)
	}
	properties := resultSchemaProperties(t, final.Result.DataSchema)
	want := []string{"canRecommend", "canSearch", "defaultRole", "docUrl", "expireAt", "expireDays", "message", "nodeId", "partnerIncluded", "pendingApproval", "permissionBreakApplied", "requirePassword", "visibility"}
	if got := sortedContractSchemaKeys(properties); !reflect.DeepEqual(got, want) {
		t.Fatalf("result properties = %#v, want %#v", got, want)
	}
	var root map[string]any
	if err := json.Unmarshal(final.Result.DataSchema, &root); err != nil {
		t.Fatalf("result data_schema is not JSON: %v\n%s", err, final.Result.DataSchema)
	}
	assertSchemaRequired(t, root, "nodeId", "visibility")
	if got := schemaEnumValues(t, properties["visibility"].(map[string]any)); !reflect.DeepEqual(got, []string{"PRIVATE", "ORGANIZATION", "PUBLIC", "<null>"}) {
		t.Fatalf("visibility enum = %#v", got)
	}
	if got := schemaEnumValues(t, properties["defaultRole"].(map[string]any)); !reflect.DeepEqual(got, []string{"READER", "DOWNLOADER", "EDITOR", "<null>"}) {
		t.Fatalf("defaultRole enum = %#v", got)
	}
	// 可空口径：expireDays 与入参侧 InterfaceType:"number" 及服务端 metadata 统一
	// 为 number（integer 会让 Agent 以为不接受 0.5 之类的服务端实际口径）；
	// permissionBreakApplied / message 服务端可能整字段不回传，必须声明可空，
	// 否则 Agent 会把"未回传"读成 false / 空串（drive 节点存在继承关系，无
	// "恒 false"依据）。三态维度 partnerIncluded / canSearch / canRecommend /
	// requirePassword / pendingApproval 一并钉住，防回归。
	assertSchemaTypeUnion(t, properties, "nodeId", "string")
	assertSchemaTypeUnion(t, properties, "visibility", "string", "null")
	assertSchemaTypeUnion(t, properties, "defaultRole", "string", "null")
	assertSchemaTypeUnion(t, properties, "partnerIncluded", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "canSearch", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "canRecommend", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "requirePassword", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "expireDays", "number", "null")
	assertSchemaTypeUnion(t, properties, "expireAt", "number", "null")
	assertSchemaTypeUnion(t, properties, "permissionBreakApplied", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "pendingApproval", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "message", "string", "null")
	assertSchemaTypeUnion(t, properties, "docUrl", "string")
}

// ── drive permission set-share-scope：dry-run 密码掩码 ──

// TestCrossPlatformCoverageDriveSetShareScopeDryRunMasksPassword asserts the desensitization contract
// for the internet-public access password: the --dry-run preview must mask the
// password (no plaintext on stdout/stderr) while the real (--yes) request still
// carries the correct plaintext password and requirePassword=true.
func TestCrossPlatformCoverageDriveSetShareScopeDryRunMasksPassword(t *testing.T) {
	const plaintext = "ab12"
	baseArgs := []string{"permission", "set-share-scope", "--node", "n1", "--visibility", "PUBLIC", "--password", plaintext}

	// dry-run: preview masks password, zero business call.
	dryRun := &guardedMutationCaller{dryRun: true}
	var out bytes.Buffer
	if err := executeGuardedMutationCommandCaptureOut(t, dryRun, newDriveCommand, &out, append(append([]string(nil), baseArgs...), "--dry-run")...); err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if len(dryRun.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", dryRun.calls)
	}
	preview := out.String()
	if strings.Contains(preview, plaintext) {
		t.Fatalf("dry-run preview leaked plaintext password: %s", preview)
	}
	if !strings.Contains(preview, "***") {
		t.Fatalf("dry-run preview did not mask password: %s", preview)
	}

	// real (--yes): exact plaintext password + requirePassword=true dispatched.
	confirmed := &guardedMutationCaller{}
	if err := executeGuardedMutationCommand(t, confirmed, newDriveCommand, append(append([]string(nil), baseArgs...), "--yes")...); err != nil {
		t.Fatalf("confirmed set-share-scope returned error: %v", err)
	}
	if len(confirmed.calls) != 1 {
		t.Fatalf("tool calls = %d, want exactly 1: %+v", len(confirmed.calls), confirmed.calls)
	}
	want := map[string]any{"nodeId": "n1", "targetVisibility": "PUBLIC", "requirePassword": true, "password": plaintext}
	if !reflect.DeepEqual(confirmed.calls[0].args, want) {
		t.Fatalf("real args = %#v, want %#v (plaintext password must be sent)", confirmed.calls[0].args, want)
	}
}

// ── drive permission set-share-scope：响应透传保真 ──

// TestDriveSetShareScopeResponsePassthroughFidelity pins the response contract:
// the CLI is a pure passthrough of the server payload and must keep the three
// "negative" shapes distinguishable.
//
//	server sent false  -> stays false
//	server sent null   -> stays explicit JSON null
//	server sent nothing-> stays absent
//
// Collapsing any of the three into `false` / `""` would let an Agent read
// "unknown" as "permission inheritance was not broken" or "the operation
// produced no message", which is exactly the failure mode the nullable Result
// DataSchema declarations exist to prevent.
func TestDriveSetShareScopeResponsePassthroughFidelity(t *testing.T) {
	// Realistic ORGANIZATION payload: permission inheritance actually broken,
	// search really turned off, approval explicitly null, and the two PUBLIC /
	// ORGANIZATION dimensions the server never reports left out entirely.
	const serverMessage = "分享范围已更新：企业内公开（含合作伙伴（外包）），组织内搜索已关闭。"
	drivePayload := `{"nodeId":"n1","visibility":"ORGANIZATION","defaultRole":"READER",` +
		`"partnerIncluded":true,"canSearch":false,` +
		`"message":"` + serverMessage + `",` +
		`"permissionBreakApplied":true,"pendingApproval":null,` +
		`"docUrl":"https://alidocs.dingtalk.com/i/nodes/n1"}`

	caller := &setShareScopePassthroughCaller{responseText: drivePayload}
	var out bytes.Buffer
	args := []string{"permission", "set-share-scope", "--node", "n1", "--visibility", "ORGANIZATION", "--role", "READER", "--partner", "--yes"}
	if err := executeGuardedMutationCommandCaptureOut(t, caller, newDriveCommand, &out, args...); err != nil {
		t.Fatalf("set-share-scope returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want exactly 1: %+v", len(caller.calls), caller.calls)
	}
	raw := out.String()
	// 统一信封格式：业务数据在 data 字段内透传。
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, raw)
	}
	if envelope["outcome"] != "success" {
		t.Fatalf("envelope outcome = %#v, want success", envelope["outcome"])
	}
	body, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope data is not an object: %#v\n%s", envelope["data"], raw)
	}
	// message 逐字透传：先在原始字节上钉一次（避开重新序列化可能引入的
	// 转义/缩进差异），再在解析后的对象上钉一次。
	if !strings.Contains(raw, serverMessage) {
		t.Fatalf("output did not pass message through verbatim:\nwant substring %q\ngot:\n%s", serverMessage, raw)
	}
	if got, _ := body["message"].(string); got != serverMessage {
		t.Fatalf("message = %#v, want %#v", body["message"], serverMessage)
	}
	for _, name := range []string{"permissionBreakApplied", "partnerIncluded"} {
		if got, ok := body[name]; !ok || got != true {
			t.Fatalf("%s = %#v (present=%v), want true", name, got, ok)
		}
	}
	for _, name := range []string{"canSearch"} {
		if got, ok := body[name]; !ok || got != false {
			t.Fatalf("%s = %#v (present=%v), want false (server really sent false)", name, got, ok)
		}
	}
	for _, name := range []string{"pendingApproval"} {
		got, ok := body[name]
		if !ok {
			t.Fatalf("%s is absent, want explicit JSON null preserved", name)
		}
		if got != nil {
			t.Fatalf("%s = %#v, want explicit JSON null (must not fold into false)", name, got)
		}
	}
	for _, name := range []string{"canRecommend", "requirePassword", "expireDays"} {
		if got, ok := body[name]; ok {
			t.Fatalf("%s = %#v, want absent (server never sent it; must not be synthesized as false/%q)", name, got, "")
		}
	}
	for name, wantValue := range map[string]string{
		"nodeId":      "n1",
		"visibility":  "ORGANIZATION",
		"defaultRole": "READER",
		"docUrl":      "https://alidocs.dingtalk.com/i/nodes/n1",
	} {
		got, ok := body[name].(string)
		if !ok || got != wantValue {
			t.Fatalf("%s = %#v, want %q", name, body[name], wantValue)
		}
	}
}

// ── drive permission set-share-scope：MCP 错误传播 ──

func TestCrossPlatformCoverageDriveSetShareScopeErrorPropagation(t *testing.T) {
	caller := &setShareScopeErrorCaller{err: stderrors.New("upstream unavailable")}
	var out bytes.Buffer
	args := []string{"permission", "set-share-scope", "--node", "n1", "--visibility", "PUBLIC", "--yes"}
	err := executeGuardedMutationCommandCaptureOut(t, caller, newDriveCommand, &out, args...)
	if err == nil {
		t.Fatal("expected error from MCP failure, got nil")
	}
	if !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("error = %q, want contains upstream unavailable", err.Error())
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(caller.calls))
	}
}

// assertSchemaTypeUnion pins the JSON-Schema "type" of a result property. Both
// the scalar form ("boolean") and the union form (["boolean","null"]) are
// accepted, and the assertion is order-sensitive so a nullable widening cannot
// silently regress into a non-nullable type — or the other way round, which
// would let an Agent read a missing server field as false / "".
func assertSchemaTypeUnion(t *testing.T, properties map[string]any, name string, want ...string) {
	t.Helper()
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %s = %#v, want schema object", name, properties[name])
	}
	var got []string
	switch typed := property["type"].(type) {
	case string:
		got = []string{typed}
	case []any:
		got = make([]string, 0, len(typed))
		for _, value := range typed {
			entry, ok := value.(string)
			if !ok {
				t.Fatalf("schema property %s type entry = %#v, want string", name, value)
			}
			got = append(got, entry)
		}
	default:
		t.Fatalf("schema property %s type = %#v, want string or array", name, property["type"])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema property %s type = %#v, want %#v", name, got, want)
	}
}
