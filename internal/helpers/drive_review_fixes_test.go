// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	if err != nil {
		t.Fatal(err)
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
