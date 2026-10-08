// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ── wiki permission set-share-scope：2 级 permission 组注册 / 路由 / 参数三态 / 本地校验 ──

func TestCrossPlatformCoverageWikiPermissionGroupRegistered(t *testing.T) {
	wiki := newWikiCommand()
	for _, path := range [][]string{{"permission", "set-share-scope"}, {"perm", "set-share-scope"}} {
		leaf, _, err := wiki.Find(path)
		if err != nil || leaf == nil || leaf.Name() != "set-share-scope" {
			t.Fatalf("find wiki %v: command=%v err=%v", path, leaf, err)
		}
	}
	permCmd, _, err := wiki.Find([]string{"permission"})
	if err != nil || permCmd == nil || permCmd.Name() != "permission" {
		t.Fatalf("find wiki permission group: command=%v err=%v", permCmd, err)
	}
	if !reflect.DeepEqual(permCmd.Aliases, []string{"perm"}) {
		t.Fatalf("permission aliases = %#v, want [perm]", permCmd.Aliases)
	}
}

func TestCrossPlatformCoverageWikiPermissionSetShareScopeRoutesToWiki(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newWikiCommand,
		"permission", "set-share-scope", "--workspace", "ws1", "--visibility", "PRIVATE", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want exactly one", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "wiki" || call.toolName != "set_space_share_scope" {
		t.Fatalf("call = %#v, want wiki/set_space_share_scope", call)
	}
	want := map[string]any{"workspaceId": "ws1", "targetVisibility": "PRIVATE"}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("args = %#v, want %#v", call.args, want)
	}
}

func TestCrossPlatformCoverageWikiPermissionSetShareScopeHiddenAliases(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newWikiCommand,
		"permission", "set-share-scope", "--workspace-id", "ws-alias", "--visibility", "PRIVATE", "--yes")
	if err != nil {
		t.Fatalf("alias --workspace-id: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["workspaceId"] != "ws-alias" {
		t.Fatalf("calls = %#v, want workspaceId=ws-alias", caller.calls)
	}
}

func TestCrossPlatformCoverageWikiPermissionSetShareScopeArgAssembly(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "PRIVATE minimal",
			args: []string{"--workspace", "ws1", "--visibility", "PRIVATE"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "PRIVATE"},
		},
		{
			name: "ORGANIZATION role EDITOR allowed",
			args: []string{"--workspace", "ws1", "--visibility", "ORGANIZATION", "--role", "EDITOR"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "ORGANIZATION", "defaultRole": "EDITOR"},
		},
		{
			name: "ORGANIZATION partner true",
			args: []string{"--workspace", "ws1", "--visibility", "ORGANIZATION", "--partner"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "ORGANIZATION", "partnerIncluded": true},
		},
		{
			name: "ORGANIZATION partner explicit false is sent",
			args: []string{"--workspace", "ws1", "--visibility", "ORGANIZATION", "--partner=false"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "ORGANIZATION", "partnerIncluded": false},
		},
		{
			name: "ORGANIZATION can-search + can-recommend",
			args: []string{"--workspace", "ws1", "--visibility", "ORGANIZATION", "--can-search", "--can-recommend"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "ORGANIZATION", "canSearch": true, "canRecommend": true},
		},
		{
			name: "ORGANIZATION all optional flags",
			args: []string{"--workspace", "ws1", "--visibility", "ORGANIZATION", "--role", "READER", "--partner", "--can-search", "--can-recommend"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "ORGANIZATION", "defaultRole": "READER", "partnerIncluded": true, "canSearch": true, "canRecommend": true},
		},
		{
			name: "PUBLIC role DOWNLOADER allowed",
			args: []string{"--workspace", "ws1", "--visibility", "PUBLIC", "--role", "DOWNLOADER"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "PUBLIC", "defaultRole": "DOWNLOADER"},
		},
		{
			name: "PUBLIC minimal",
			args: []string{"--workspace", "ws1", "--visibility", "PUBLIC"},
			want: map[string]any{"workspaceId": "ws1", "targetVisibility": "PUBLIC"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			full := append([]string{"permission", "set-share-scope"}, tt.args...)
			full = append(full, "--yes")
			if err := executeGuardedMutationCommand(t, caller, newWikiCommand, full...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v, want exactly one", caller.calls)
			}
			if caller.calls[0].toolName != "set_space_share_scope" {
				t.Fatalf("tool = %#v, want set_space_share_scope", caller.calls[0])
			}
			if !reflect.DeepEqual(caller.calls[0].args, tt.want) {
				t.Fatalf("args = %#v, want %#v", caller.calls[0].args, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageWikiPermissionSetShareScopeValidationRejects(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{name: "missing workspace", args: []string{"--visibility", "PRIVATE"}, wantMsg: "missing required flag(s): --workspace"},
		{name: "missing visibility", args: []string{"--workspace", "ws1"}, wantMsg: "missing required flag(s): --visibility"},
		{name: "invalid visibility enum", args: []string{"--workspace", "ws1", "--visibility", "public"}, wantMsg: "不合法"},
		{name: "invalid role enum", args: []string{"--workspace", "ws1", "--visibility", "ORGANIZATION", "--role", "MANAGER"}, wantMsg: "不合法"},
		{name: "PRIVATE + role", args: []string{"--workspace", "ws1", "--visibility", "PRIVATE", "--role", "READER"}, wantMsg: "PRIVATE 档位不适用 --role"},
		{name: "PRIVATE + partner", args: []string{"--workspace", "ws1", "--visibility", "PRIVATE", "--partner"}, wantMsg: "PRIVATE 档位不适用 --partner"},
		{name: "PRIVATE + can-search", args: []string{"--workspace", "ws1", "--visibility", "PRIVATE", "--can-search"}, wantMsg: "PRIVATE 档位不适用 --can-search"},
		{name: "PRIVATE + can-recommend", args: []string{"--workspace", "ws1", "--visibility", "PRIVATE", "--can-recommend"}, wantMsg: "PRIVATE 档位不适用 --can-recommend"},
		{name: "PUBLIC + partner", args: []string{"--workspace", "ws1", "--visibility", "PUBLIC", "--partner"}, wantMsg: "PUBLIC（互联网公开）档位不适用 --partner"},
		{name: "PUBLIC + can-search", args: []string{"--workspace", "ws1", "--visibility", "PUBLIC", "--can-search"}, wantMsg: "PUBLIC（互联网公开）档位不适用 --can-search"},
		{name: "PUBLIC + can-recommend", args: []string{"--workspace", "ws1", "--visibility", "PUBLIC", "--can-recommend"}, wantMsg: "PUBLIC（互联网公开）档位不适用 --can-recommend"},
		{name: "PUBLIC + role EDITOR", args: []string{"--workspace", "ws1", "--visibility", "PUBLIC", "--role", "EDITOR"}, wantMsg: "--role 仅支持 READER 或 DOWNLOADER"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			caller := &guardedMutationCaller{}
			full := append([]string{"permission", "set-share-scope"}, tt.args...)
			err := executeGuardedMutationCommand(t, caller, newWikiCommand, full...)
			if err == nil {
				t.Fatalf("args %v: expected rejection, got nil", tt.args)
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

// ── wiki permission set-share-scope：ResultSpec 返回值契约 ──

func TestCrossPlatformCoverageWikiPermissionSetShareScopeResultContract(t *testing.T) {
	wiki := newWikiCommand()
	leaf, _, err := wiki.Find([]string{"permission", "set-share-scope"})
	if err != nil || leaf == nil {
		t.Fatalf("find wiki permission set-share-scope: command=%v err=%v", leaf, err)
	}
	final, ok := contractfinal.RuntimeContractFinal(leaf)
	if !ok || final.Identity == nil || final.Identity.CanonicalPath != "wiki.set_space_share_scope" {
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
	want := []string{"canRecommend", "canSearch", "defaultRole", "message", "partnerIncluded", "pendingApproval", "permissionBreakApplied", "spaceUrl", "visibility", "workspaceId"}
	if got := sortedContractSchemaKeys(properties); !reflect.DeepEqual(got, want) {
		t.Fatalf("result properties = %#v, want %#v", got, want)
	}
	var root map[string]any
	if err := json.Unmarshal(final.Result.DataSchema, &root); err != nil {
		t.Fatalf("result data_schema is not JSON: %v\n%s", err, final.Result.DataSchema)
	}
	assertSchemaRequired(t, root, "workspaceId", "visibility")
	if got := schemaEnumValues(t, properties["defaultRole"].(map[string]any)); !reflect.DeepEqual(got, []string{"READER", "DOWNLOADER", "EDITOR", "<null>"}) {
		t.Fatalf("defaultRole enum = %#v", got)
	}
	// 可空口径：message 服务端可能整字段不回传，声明可空避免 Agent 把 null 读成
	// 空串。permissionBreakApplied 与 drive 侧相反，保持非可空 boolean：知识库
	// 空间是权限继承根、恒独立，该字段恒为 false，有据不可空。wiki 侧无
	// expireDays（有效期仅 drive 互联网公开档涉及）。三态维度一并钉住防回归。
	assertSchemaTypeUnion(t, properties, "workspaceId", "string")
	assertSchemaTypeUnion(t, properties, "visibility", "string", "null")
	assertSchemaTypeUnion(t, properties, "defaultRole", "string", "null")
	assertSchemaTypeUnion(t, properties, "partnerIncluded", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "canSearch", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "canRecommend", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "permissionBreakApplied", "boolean")
	assertSchemaTypeUnion(t, properties, "pendingApproval", "boolean", "null")
	assertSchemaTypeUnion(t, properties, "message", "string", "null")
	assertSchemaTypeUnion(t, properties, "spaceUrl", "string")
}

// ── wiki permission set-share-scope：dry-run 预览与 MCP 错误传播 ──

func TestCrossPlatformCoverageWikiSetShareScopeDryRun(t *testing.T) {
	dryRun := &guardedMutationCaller{dryRun: true}
	var out bytes.Buffer
	args := []string{"permission", "set-share-scope", "--workspace", "ws1", "--visibility", "ORGANIZATION", "--dry-run"}
	if err := executeGuardedMutationCommandCaptureOut(t, dryRun, newWikiCommand, &out, args...); err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if len(dryRun.calls) != 0 {
		t.Fatalf("dry-run tool calls = %#v, want none", dryRun.calls)
	}
	preview := out.String()
	if !strings.Contains(preview, "\"executed\":false") && !strings.Contains(preview, "\"executed\": false") {
		t.Fatalf("dry-run preview missing executed=false: %s", preview)
	}
	if !strings.Contains(preview, "set_space_share_scope") {
		t.Fatalf("dry-run preview missing tool name: %s", preview)
	}
}

func TestCrossPlatformCoverageWikiSetShareScopeErrorPropagation(t *testing.T) {
	caller := &setShareScopeErrorCaller{err: stderrors.New("upstream unavailable")}
	var out bytes.Buffer
	args := []string{"permission", "set-share-scope", "--workspace", "ws1", "--visibility", "PUBLIC", "--yes"}
	err := executeGuardedMutationCommandCaptureOut(t, caller, newWikiCommand, &out, args...)
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

// ── wiki permission set-share-scope：响应透传保真 ──

// TestWikiSetShareScopeResponsePassthroughFidelity pins the wiki response
// contract: the CLI is a pure passthrough and must keep absent fields absent
// (not synthesized as false/"").
func TestWikiSetShareScopeResponsePassthroughFidelity(t *testing.T) {
	// Sparse wiki payload: the server only echoes the identity fields, so every
	// tri-state dimension plus message / permissionBreakApplied is missing.
	wikiPayload := `{"workspaceId":"ws1","visibility":"PUBLIC","defaultRole":"READER",` +
		`"spaceUrl":"https://alidocs.dingtalk.com/i/spaces/ws1"}`

	caller := &setShareScopePassthroughCaller{responseText: wikiPayload}
	var out bytes.Buffer
	args := []string{"permission", "set-share-scope", "--workspace", "ws1", "--visibility", "PUBLIC", "--role", "READER", "--yes"}
	if err := executeGuardedMutationCommandCaptureOut(t, caller, newWikiCommand, &out, args...); err != nil {
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
	for _, name := range []string{
		"message", "permissionBreakApplied", "pendingApproval",
		"partnerIncluded", "canSearch", "canRecommend",
	} {
		if got, ok := body[name]; ok {
			t.Fatalf("%s = %#v, want absent (server never sent it; must not be synthesized as false/%q)", name, got, "")
		}
	}
	for name, wantValue := range map[string]string{
		"workspaceId": "ws1",
		"visibility":  "PUBLIC",
		"defaultRole": "READER",
		"spaceUrl":    "https://alidocs.dingtalk.com/i/spaces/ws1",
	} {
		got, ok := body[name].(string)
		if !ok || got != wantValue {
			t.Fatalf("%s = %#v, want %q", name, body[name], wantValue)
		}
	}
}
