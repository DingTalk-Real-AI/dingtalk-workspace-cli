// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// executeDriveCommandCapture 执行 drive 命令并捕获 deps.Out 的 stdout 输出，
// 供需要断言 JSON 输出形态的测试使用（等价于 installDepthCaller + Execute）。
func executeDriveCommandCapture(t *testing.T, caller edition.ToolCaller, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	buf := &bytes.Buffer{}
	deps.Out.w = buf
	deps.Out.errW = io.Discard
	root := newDriveCommand()
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "confirm high-risk operation")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	err := root.Execute()
	return buf, err
}

// ── drive permission apply：确认门禁与帮助文案契约一致 ──

func TestCrossPlatformCoverageDrivePermissionApplyDeclined(t *testing.T) {
	caller := &guardedMutationCaller{}
	root := newDriveCommand()
	root.SetIn(strings.NewReader("no\n"))
	err := executeGuardedMutationCommand(t, caller, func() *cobra.Command { return root },
		"permission", "apply", "--node", "node-1", "--role", "READER", "--users", "u1")
	if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("declined apply error = %v, want 用户取消了操作", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("declined apply calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageDrivePermissionApplyYesProceeds(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "apply", "--node", "node-1", "--role", "reader", "--users", "u1,u2", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want exactly one", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "drive" || call.toolName != "apply_permission" {
		t.Fatalf("call = %#v", call)
	}
	if call.args["nodeId"] != "node-1" || call.args["roleId"] != "READER" {
		t.Fatalf("args = %#v", call.args)
	}
	if !reflect.DeepEqual(call.args["receivers"], []string{"u1", "u2"}) {
		t.Fatalf("receivers = %#v", call.args["receivers"])
	}
}

func TestCrossPlatformCoverageDrivePermissionApplyDryRunNoPromptNoCall(t *testing.T) {
	caller := &guardedMutationCaller{dryRun: true}
	root := newDriveCommand()
	promptOut := &bytes.Buffer{}
	root.SetErr(promptOut)
	root.SetIn(strings.NewReader("no\n"))
	err := executeGuardedMutationCommand(t, caller, func() *cobra.Command { return root },
		"permission", "apply", "--node", "node-1", "--role", "READER", "--users", "u1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run apply calls = %#v, want none", caller.calls)
	}
	if strings.Contains(promptOut.String(), "Confirm action") {
		t.Fatalf("dry-run prompted for confirmation: %q", promptOut.String())
	}
}

// ── drive permission transfer-owner：dry-run JSON 输出且校验先于 dry-run ──

func TestCrossPlatformCoverageDriveTransferOwnerDryRunJSONNode(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", dry: true}
	out, err := executeDriveCommandCapture(t, caller,
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run tool calls = %d, want none", caller.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, out.String())
	}
	if payload["dry_run"] != true || payload["executed"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["operation"] != "转交所有者" || payload["newOwnerId"] != "user-1" || payload["nodeId"] != "node-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["workspaceId"]; ok {
		t.Fatalf("unexpected workspaceId in %#v", payload)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerDryRunJSONWorkspace(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", dry: true}
	out, err := executeDriveCommandCapture(t, caller,
		"permission", "transfer-owner", "--workspace", "ws-1", "--new-owner", "user-1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, out.String())
	}
	if payload["workspaceId"] != "ws-1" || payload["newOwnerId"] != "user-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["nodeId"]; ok {
		t.Fatalf("unexpected nodeId in %#v", payload)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerDryRunYesValidatesFirst(t *testing.T) {
	caller := &guardedMutationCaller{dryRun: true}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-1", "--yes", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "--reserve-role is required") {
		t.Fatalf("err = %v, want --reserve-role required even under --dry-run", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveTransferOwnerDryRunNonJSON(t *testing.T) {
	caller := &scriptedToolCaller{format: "table", dry: true}
	_, err := executeDriveCommandCapture(t, caller,
		"permission", "transfer-owner", "--node", "node-1", "--new-owner", "user-1", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run tool calls = %d, want none", caller.calls)
	}
}

// ── drive list：--versions 与 --depth/--pattern 的交互 ──

func TestCrossPlatformCoverageDriveListVersionsRejectsDepth(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--depth", "2", "--limit", "10")
	if err == nil || !strings.Contains(err.Error(), "--depth") {
		t.Fatalf("err = %v, want explicit --versions/--depth conflict", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListVersionsRejectsPattern(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--pattern", "x")
	if err == nil || !strings.Contains(err.Error(), "--pattern") {
		t.Fatalf("err = %v, want explicit --versions/--pattern conflict", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListVersionsAllowsDepthOne(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--depth", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "list_file_versions" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveListVersionsWithLimit(t *testing.T) {
	caller := &guardedMutationCaller{}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"list", "--versions", "--node", "node-1", "--limit", "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want exactly one", caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "drive" || call.toolName != "list_file_versions" {
		t.Fatalf("call = %#v", call)
	}
	if call.args["nodeId"] != "node-1" || call.args["maxResults"] != 10 {
		t.Fatalf("args = %#v", call.args)
	}
}

func TestCrossPlatformCoverageDriveDownloadVersionDryRun(t *testing.T) {
	caller := &guardedMutationCaller{dryRun: true}
	err := executeGuardedMutationCommand(t, caller, newDriveCommand,
		"download-version", "--node", "node-1", "--version", "3", "--output", "./x.pdf", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageDriveDownloadVersionDirectoryOutput(t *testing.T) {
	SetHTTPGetFile(func(context.Context, string, map[string]string, string) error { return nil })
	t.Cleanup(func() { SetHTTPGetFile(nil) })

	dir := t.TempDir()
	for _, resp := range []string{
		`{"downloadUrl":"https://oss.test/get/report_v3.pdf","fileName":"报告v3.pdf"}`,
		`{"downloadUrl":"https://oss.test/get/inferred_v3.pdf"}`,
	} {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: resp}}}
		installScriptedCaller(t, caller)
		root := newDriveCommand()
		root.PersistentFlags().Bool("dry-run", false, "")
		root.SilenceErrors = true
		root.SilenceUsage = true
		root.SetArgs([]string{"download-version", "--node", "node-1", "--version", "3", "--output", dir})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 {
			t.Fatalf("calls = %d, want 1", caller.calls)
		}
	}
}

// ── drive publish set：CR 修复后的密码/有效期参数契约 ──

func TestCrossPlatformCoverageDrivePublishSetValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing node", []string{"publish", "set"}, "--node"},
		{"invalid permission", []string{"publish", "set", "--node", "n1", "--permission", "ADMIN"}, "--permission 值无效"},
		{"short password", []string{"publish", "set", "--node", "n1", "--password", "abc"}, "密码必须为 4 位"},
		{"non alphanumeric password", []string{"publish", "set", "--node", "n1", "--password", "a#c1"}, "密码必须为 4 位"},
		{"negative expire days", []string{"publish", "set", "--node", "n1", "--expire-days", "-1"}, "--expire-days 不能为负数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 校验发生在 LeafSpec.Validate（确认门之前）：无需 --yes、不触发
			// 交互确认，0 次工具调用即返回错误。
			caller := &scriptedToolCaller{}
			err := executeDriveEdge(t, caller, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("tool calls = %d, want 0 (validation must precede any call)", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDrivePublishSetPasswordAndExpiryArgs(t *testing.T) {
	t.Run("set password and expire days", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "publish", "set", "--node", "n1",
			"--password", "Ab12", "--expire-days", "7", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.argsLog) != 1 || caller.tool != "set_file_publish" {
			t.Fatalf("calls = %v", caller.toolLog)
		}
		args := caller.argsLog[0]
		if args["fileId"] != "n1" || args["published"] != true {
			t.Fatalf("args = %#v", args)
		}
		if args["requirePassword"] != true || args["password"] != "Ab12" {
			t.Fatalf("password args = %#v", args)
		}
		if args["expireDays"] != 7 {
			t.Fatalf("expireDays = %#v, want 7", args["expireDays"])
		}
	})

	t.Run("empty password clears protection", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "publish", "set", "--node", "n1",
			"--password", "", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		args := caller.argsLog[0]
		if args["requirePassword"] != false {
			t.Fatalf("requirePassword = %#v, want false", args["requirePassword"])
		}
		if _, present := args["password"]; present {
			t.Fatalf("password key should be absent when clearing: %#v", args)
		}
		if _, present := args["expireDays"]; present {
			t.Fatalf("expireDays key should be absent when unset: %#v", args)
		}
	})

	t.Run("permanent expiry zero", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "publish", "set", "--node", "n1",
			"--expire-days", "0", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		if args := caller.argsLog[0]; args["expireDays"] != 0 {
			t.Fatalf("expireDays = %#v, want 0", args["expireDays"])
		}
	})

	t.Run("permission passed through", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "publish", "set", "--node", "n1",
			"--permission", "READER", "--yes")
		if err != nil {
			t.Fatal(err)
		}
		if args := caller.argsLog[0]; args["publishPermission"] != "READER" {
			t.Fatalf("publishPermission = %#v", args["publishPermission"])
		}
	})
}

// ── drive quota / quota apps：参数组装 ──

func TestCrossPlatformCoverageDriveQuotaCommand(t *testing.T) {
	t.Run("enterprise level without args", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		if err := executeDriveEdge(t, caller, "quota"); err != nil {
			t.Fatal(err)
		}
		if caller.tool != "get_storage_quota" || len(caller.args) != 0 {
			t.Fatalf("call = %s %v", caller.tool, caller.args)
		}
	})

	t.Run("app level", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		if err := executeDriveEdge(t, caller, "quota", "--app", "app-1"); err != nil {
			t.Fatal(err)
		}
		if caller.args["appId"] != "app-1" {
			t.Fatalf("args = %#v", caller.args)
		}
	})

	t.Run("space level", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		if err := executeDriveEdge(t, caller, "quota", "--space", "space-1"); err != nil {
			t.Fatal(err)
		}
		if caller.args["spaceId"] != "space-1" {
			t.Fatalf("args = %#v", caller.args)
		}
	})

	t.Run("app and space are mutually exclusive", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{}, "quota", "--app", "a", "--space", "s")
		if err == nil || !strings.Contains(err.Error(), "none of the others can be") {
			t.Fatalf("error = %v, want flag group mutual exclusion failure", err)
		}
	})
}

func TestCrossPlatformCoverageDriveQuotaAppsCommand(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		if err := executeDriveEdge(t, caller, "quota", "apps"); err != nil {
			t.Fatal(err)
		}
		if caller.tool != "list_storage_apps" || caller.args["maxResults"] != float64(20) {
			t.Fatalf("call = %s %#v", caller.tool, caller.args)
		}
	})

	t.Run("pagination and ordering", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "quota", "apps",
			"--limit", "50", "--cursor", "next-1",
			"--order-by", "used-quota", "--order", "desc")
		if err != nil {
			t.Fatal(err)
		}
		args := caller.argsLog[0]
		if args["maxResults"] != float64(50) || args["nextToken"] != "next-1" {
			t.Fatalf("args = %#v", args)
		}
		if args["orderBy"] != "usedQuota" || args["order"] != "desc" {
			t.Fatalf("ordering args = %#v", args)
		}
	})

	t.Run("unknown order by rejected", func(t *testing.T) {
		// 无效排序字段直接报错（fail-fast），不再静默省略 orderBy。
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "quota", "apps", "--order-by", "nope")
		if err == nil || !strings.Contains(err.Error(), "--order-by 值无效") {
			t.Fatalf("error = %v, want --order-by rejection", err)
		}
		if caller.calls != 0 {
			t.Fatalf("tool calls = %d, want 0", caller.calls)
		}
	})

	t.Run("invalid limit rejected", func(t *testing.T) {
		// 显式传值越界（0 / 负数 / 超过 50）直接报错；未传时保持默认 20 语义。
		for _, limit := range []string{"0", "-1", "51"} {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
			err := executeDriveEdge(t, caller, "quota", "apps", "--limit", limit)
			if err == nil || !strings.Contains(err.Error(), "--limit 值无效") {
				t.Fatalf("limit %s: error = %v, want --limit rejection", limit, err)
			}
			if caller.calls != 0 {
				t.Fatalf("limit %s: tool calls = %d, want 0", limit, caller.calls)
			}
		}
	})

	t.Run("unknown order rejected", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
		err := executeDriveEdge(t, caller, "quota", "apps", "--order", "up")
		if err == nil || !strings.Contains(err.Error(), "--order 值无效") {
			t.Fatalf("error = %v, want --order rejection", err)
		}
		if caller.calls != 0 {
			t.Fatalf("tool calls = %d, want 0", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageMapOrderByToCamelCase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"used-quota", "usedQuota"},
		{"standard-used-quota", "standardUsedQuota"},
		{"exclusive-used-quota", "exclusiveUsedQuota"},
		{"", ""},
		{"unknown", ""},
	}
	for _, tc := range cases {
		if got := mapOrderByToCamelCase(tc.in); got != tc.want {
			t.Errorf("mapOrderByToCamelCase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── download：space-id 透传与非 JSON dry-run 预览（补齐门禁覆盖缺口） ──

func TestCrossPlatformCoverageDriveDownloadSpaceIDArg(t *testing.T) {
	SetHTTPGetFile(func(context.Context, string, map[string]string, string) error { return nil })
	t.Cleanup(func() { SetHTTPGetFile(nil) })

	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"resourceUrl":"https://x.test/files/report.pdf"}`}}}
	target := filepath.Join(t.TempDir(), "report.pdf")
	err := executeDriveEdge(t, caller, "download", "--node", "node-1", "--output", target, "--space-id", "space-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.argsLog) == 0 || caller.argsLog[0]["spaceId"] != "space-1" {
		t.Fatalf("download args = %#v", caller.argsLog)
	}
}

func TestCrossPlatformCoverageDriveDownloadVersionDryRunPlainText(t *testing.T) {
	caller := &scriptedToolCaller{dry: true}
	err := executeDriveEdge(t, caller, "download-version",
		"--node", "node-1", "--version", "3", "--output", "./x.pdf", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run calls = %d, want 0", caller.calls)
	}
}
