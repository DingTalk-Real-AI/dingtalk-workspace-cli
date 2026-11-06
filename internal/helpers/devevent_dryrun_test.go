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

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// prepareDevEventDryRunLeaf 与 prepareDevEventLeaf 相同，但保留 ResultStore：
// 九条叶子都走 RolloutUnifiedActive，结果不落 stdout 而进 store，退出码只能
// 从 store 读回。
func prepareDevEventDryRunLeaf(t *testing.T, cmd *cobra.Command) (*cobra.Command, *bytes.Buffer, *output.ResultStore) {
	t.Helper()
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	for _, name := range []string{"yes", "dry-run"} {
		if cmd.Flags().Lookup(name) == nil {
			cmd.Flags().Bool(name, false, "")
		}
	}
	return cmd, stdout, store
}

// devEventDryRunFlags 是每条叶子在 --dry-run 下跑通所需的最小参数集，与
// devEventLeafExpectations 的 requiredFlags 同源。
func devEventDryRunFlags(use, definitionPath string) map[string]string {
	flags := map[string]string{"unified-app-id": "uapp_self_hr_A"}
	switch use {
	case "list":
		return flags
	case "create":
		flags["file"] = definitionPath
		return flags
	}
	flags["event-key"] = "app_user_hr_employee_joined"
	switch use {
	case "update":
		flags["file"] = definitionPath
		flags["expected-revision"] = "3"
	case "debug", "publish", "offline":
		flags["expected-revision"] = "3"
	}
	return flags
}

// TestDevEventDryRunReachesInvocationForEveryLeaf 验证 --dry-run 对九条叶子都
// 真正落到 invocation.DryRun：这是「预览不产生副作用」的唯一机械保证。声明面
// 的 DryRunSpec 只是承诺，派发面必须可观察。
func TestDevEventDryRunReachesInvocationForEveryLeaf(t *testing.T) {
	definitionPath := writeDevEventFile(t, "definition.json", devEventTestDefinition)
	for _, want := range devEventLeafExpectations() {
		if want.local {
			// lint 本地执行：--dry-run 没有远端预览可派发，由本地专用用例覆盖。
			continue
		}
		t.Run(want.use, func(t *testing.T) {
			runner := &fakeDevEventRunner{response: map[string]any{"content": map[string]any{
				"eventKey": "app_user_hr_employee_joined",
			}}}
			cmd, stdout, store := prepareDevEventDryRunLeaf(t, devEventBuildLeaf(want.use, runner))
			for name, value := range devEventDryRunFlags(want.use, definitionPath) {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}
			if err := cmd.Flags().Set("dry-run", "true"); err != nil {
				t.Fatal(err)
			}
			// --yes 一并给上，把确认门从本用例的变量里排除；确认门与 dry-run 的
			// 关系由 TestDevEventDryRunIsTheNonInteractivePreviewPath 单独断言。
			if err := cmd.Flags().Set("yes", "true"); err != nil {
				t.Fatal(err)
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if runner.calls != 1 {
				t.Fatalf("backend calls = %d, want exactly 1 preview dispatch", runner.calls)
			}
			if !runner.got.DryRun {
				t.Fatal("invocation.DryRun = false, want true: --dry-run 必须传到派发层")
			}
			if !output.UsesUnifiedResult(cmd) {
				t.Fatal("leaf is not on the unified result contract; failure projection would be dead code")
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty before emission: 统一结果契约下信封由框架发射", stdout.String())
			}
			// 退出码只在发射时落定，直接调 RunE 不会经过根命令的
			// PersistentPostRunE，这里显式补上那一步再读回。
			if _, _, err := output.EmitStoredResult(cmd); err != nil {
				t.Fatalf("EmitStoredResult() error = %v", err)
			}
			if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
				t.Fatalf("stored exit code = %d (emitted=%v), want 0: dry-run 是已完成的预演，不是失败", code, emitted)
			}
			var env map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("emitted stdout is not a single JSON envelope: %v (%q)", err, stdout.String())
			}
			if env["ok"] != true || env["outcome"] != "success" {
				t.Fatalf("envelope = %#v, want ok:true outcome:success", env)
			}
			if env["dry_run"] != true {
				t.Fatalf("envelope dry_run = %#v, want true", env["dry_run"])
			}
		})
	}
}

// devEventBuildLeaf 按 use 构造叶子，避免本文件重复一张与 dispatch 表平行的表。
func devEventBuildLeaf(use string, runner executor.Runner) *cobra.Command {
	switch use {
	case "list":
		return newDevEventListCommand(runner)
	case "get":
		return newDevEventGetCommand(runner)
	case "create":
		return newDevEventCreateCommand(runner)
	case "update":
		return newDevEventUpdateCommand(runner)
	case "validate":
		return newDevEventValidateCommand(runner)
	case "lint":
		return newDevEventLintCommand(runner)
	case "debug":
		return newDevEventDebugCommand(runner)
	case "publish":
		return newDevEventPublishCommand(runner)
	case "offline":
		return newDevEventOfflineCommand(runner)
	default:
		panic("unknown dev event leaf: " + use)
	}
}

// TestDevEventDryRunIsTheNonInteractivePreviewPath 验证写命令的确认门语义：
// 非交互环境下 --dry-run 与 --yes 是仅有的两条出路，--dry-run 单独就能预览，
// 且预览仍然是 DryRun 调用；两者都不给时必须拦下且零后端调用。
func TestDevEventDryRunIsTheNonInteractivePreviewPath(t *testing.T) {
	definitionPath := writeDevEventFile(t, "definition.json", devEventTestDefinition)
	for _, want := range devEventLeafExpectations() {
		if want.confirm != "user_required" {
			continue
		}
		t.Run(want.use, func(t *testing.T) {
			blocked := &fakeDevEventRunner{}
			blockedCmd, _ := prepareDevEventLeaf(t, devEventBuildLeaf(want.use, blocked))
			for name, value := range devEventDryRunFlags(want.use, definitionPath) {
				_ = blockedCmd.Flags().Set(name, value)
			}
			err := blockedCmd.RunE(blockedCmd, nil)
			if err == nil {
				t.Fatal("RunE() without --yes/--dry-run: want confirmation_required, got nil")
			}
			if blocked.calls != 0 {
				t.Fatalf("backend calls = %d, want 0: 确认门必须在派发之前", blocked.calls)
			}

			preview := &fakeDevEventRunner{response: map[string]any{"content": map[string]any{}}}
			previewCmd, _ := prepareDevEventLeaf(t, devEventBuildLeaf(want.use, preview))
			for name, value := range devEventDryRunFlags(want.use, definitionPath) {
				_ = previewCmd.Flags().Set(name, value)
			}
			_ = previewCmd.Flags().Set("dry-run", "true")
			if err := previewCmd.RunE(previewCmd, nil); err != nil {
				t.Fatalf("RunE() with --dry-run only: error = %v", err)
			}
			if !preview.got.DryRun {
				t.Fatal("invocation.DryRun = false, want true for a --dry-run preview")
			}
			if preview.calls != 1 {
				t.Fatalf("backend calls = %d, want 1", preview.calls)
			}
		})
	}
}

// TestDevEventDryRunDoesNotSkipLocalPrecheck 是本模块最关键的一条 dry-run 断言：
// 预览可以跳过写操作，但不能跳过本地预检。否则 --dry-run 会变成绕过校验的通道，
// 让一份含保留键或非法 eventKey 的定义文件「预览通过」再被原样提交。
func TestDevEventDryRunDoesNotSkipLocalPrecheck(t *testing.T) {
	cases := []struct {
		name    string
		use     string
		content string
		want    string
	}{
		{"create 拒绝保留键", "create", `{"eventKey":"app_user_a_b_c","name":"n","payloadSchema":{"type":"object"},"status":"ENABLED"}`, "不接受开发者自行指定的键"},
		{"update 拒绝非法 eventKey", "update", `{"eventKey":"User_A","name":"n","payloadSchema":{"type":"object"}}`, "不符合命名规则"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeDevEventFile(t, "definition.json", c.content)
			runner := &fakeDevEventRunner{}
			cmd, _ := prepareDevEventLeaf(t, devEventBuildLeaf(c.use, runner))
			for name, value := range devEventDryRunFlags(c.use, path) {
				_ = cmd.Flags().Set(name, value)
			}
			_ = cmd.Flags().Set("dry-run", "true")
			_ = cmd.Flags().Set("yes", "true")
			err := cmd.RunE(cmd, nil)
			devEventPrecheckError(t, err, c.want)
			if runner.calls != 0 {
				t.Fatalf("backend calls = %d, want 0: 本地预检失败不得派发", runner.calls)
			}
		})
	}
}

// TestDevEventDryRunDoesNotSkipPayloadFilePrecheck 覆盖 debug 的 --payload-file：
// 预览同样要读并校验这份文件，路径本身始终不下发。
func TestDevEventDryRunDoesNotSkipPayloadFilePrecheck(t *testing.T) {
	path := writeDevEventFile(t, "payload.json", `["not","an","object"]`)
	runner := &fakeDevEventRunner{}
	cmd, _ := prepareDevEventLeaf(t, newDevEventDebugCommand(runner))
	for name, value := range devEventDryRunFlags("debug", "") {
		_ = cmd.Flags().Set(name, value)
	}
	_ = cmd.Flags().Set("payload-file", path)
	_ = cmd.Flags().Set("dry-run", "true")
	_ = cmd.Flags().Set("yes", "true")
	devEventPrecheckError(t, cmd.RunE(cmd, nil), "不是合法 JSON 对象")
	if runner.calls != 0 {
		t.Fatalf("backend calls = %d, want 0", runner.calls)
	}
}

// TestDevEventDryRunDeclarationIsUniform 验证九条叶子的 DryRunSpec 完全一致：
// invocation 预览、不为预览发起远端读取。声明面漂移会让 Agent 误判预览成本。
func TestDevEventDryRunDeclarationIsUniform(t *testing.T) {
	want := devEventDryRun()
	for _, expectation := range devEventLeafExpectations() {
		final := devEventContractFinal(t, devEventBuildLeaf(expectation.use, &fakeDevEventRunner{}))
		if expectation.local {
			// lint 是纯本地执行：没有远端可预览，DryRun 必须缺席而不是伪装。
			if final.DryRun != nil {
				t.Fatalf("%s: local leaf DryRun = %+v, want nil", expectation.use, final.DryRun)
			}
			continue
		}
		if final.DryRun == nil {
			t.Fatalf("%s: DryRun == nil", expectation.use)
		}
		if final.DryRun.PreviewKind != want.PreviewKind || final.DryRun.RemoteReads != want.RemoteReads {
			t.Fatalf("%s: DryRun = %+v, want %+v", expectation.use, final.DryRun, want)
		}
	}
}
