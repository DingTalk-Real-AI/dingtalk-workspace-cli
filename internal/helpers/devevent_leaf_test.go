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
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// fakeDevEventRunner 捕获 dev event 叶子派发的 invocation，并可注入上游响应或
// 传输错误。Implemented 置为 true 对齐真实 runner（internal/app/runner.go）：
// 否则 devAppEnvelopeData 不解包 response.content，事件定义域的结果归一、
// 失败投影与分页 fail-closed 都不会被执行，测试就会假通过。
type fakeDevEventRunner struct {
	got      executor.Invocation
	calls    int
	response map[string]any
	err      error
}

func (f *fakeDevEventRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	f.calls++
	inv.Implemented = true
	f.got = inv
	if f.err != nil {
		return executor.Result{}, f.err
	}
	response := f.response
	if response == nil {
		response = map[string]any{}
	}
	return executor.Result{Invocation: inv, Response: response}, nil
}

// prepareDevEventLeaf 装好统一结果存储与 stdout 缓冲，并补上根命令才注册的
// --yes / --dry-run：写守卫与 dry-run 分支都从 cmd.Flags() 读这两个开关。
func prepareDevEventLeaf(t *testing.T, cmd *cobra.Command) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	for _, name := range []string{"yes", "dry-run"} {
		if cmd.Flags().Lookup(name) == nil {
			cmd.Flags().Bool(name, false, "")
		}
	}
	return cmd, stdout
}

// devEventTestDefinition 是一份通过全部本地预检的事件定义。
const devEventTestDefinition = `{
  "eventKey": "app_user_hr_employee_joined",
  "name": "员工入职",
  "description": "HR 系统在员工入职完成后上报",
  "payloadSchema": {"type": "object", "properties": {"employeeId": {"type": "string", "description": "企业内部员工唯一标识"}}},
  "samplePayload": {"employeeId": "E-1"}
}`

func writeDevEventFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return path
}

// TestDevEventLeafDispatchTable 逐条断言九个叶子的远端工具名、派发产品与
// toolArgs 装配：Trim、OmitEmpty、LeafInt、游标注入、--file 预检展开与
// --payload-file 覆盖都不允许在这一层漂移。
func TestDevEventLeafDispatchTable(t *testing.T) {
	cases := []struct {
		name       string
		build      func(executor.Runner) *cobra.Command
		tool       string
		confirm    bool
		flags      func(t *testing.T) map[string]string
		wantParams map[string]any
		wantAbsent []string
	}{
		{
			name:  "list",
			build: newDevEventListCommand,
			tool:  devEventListTool,
			flags: func(*testing.T) map[string]string {
				return map[string]string{"unified-app-id": "  uapp_self_hr_A  ", "keyword": " 入职 ", "status": "DRAFT"}
			},
			wantParams: map[string]any{"unifiedAppId": "uapp_self_hr_A", "keyword": "入职", "status": "DRAFT", "pageSize": 20},
			wantAbsent: []string{"cursor"},
		},
		{
			name:  "get",
			build: newDevEventGetCommand,
			tool:  devEventGetTool,
			flags: func(*testing.T) map[string]string {
				return map[string]string{"unified-app-id": "uapp_self_hr_A", "event-key": " app_user_hr_employee_joined ", "view": "published"}
			},
			wantParams: map[string]any{"unifiedAppId": "uapp_self_hr_A", "eventKey": "app_user_hr_employee_joined", "view": "published"},
			wantAbsent: []string{"pageSize", "cursor"},
		},
		{
			name:    "create",
			build:   newDevEventCreateCommand,
			tool:    devEventCreateTool,
			confirm: true,
			flags: func(t *testing.T) map[string]string {
				return map[string]string{
					"unified-app-id": "uapp_self_hr_A",
					"file":           writeDevEventFile(t, "hr-employee-joined.json", devEventTestDefinition),
				}
			},
			wantParams: map[string]any{
				"unifiedAppId": "uapp_self_hr_A",
				"eventKey":     "app_user_hr_employee_joined",
				"name":         "员工入职",
				"description":  "HR 系统在员工入职完成后上报",
				// 定义文件里的对象在网关契约下以 JSON 文本透传（见
				// devEventApplyDefinitionFile 的 payloadSchema/samplePayload 序列化）。
				"payloadSchema": `{"properties":{"employeeId":{"description":"企业内部员工唯一标识","type":"string"}},"type":"object"}`,
				"samplePayload": `{"employeeId":"E-1"}`,
			},
			wantAbsent: []string{"file", "expectedConfigRevision"},
		},
		{
			name:    "update",
			build:   newDevEventUpdateCommand,
			tool:    devEventUpdateTool,
			confirm: true,
			flags: func(t *testing.T) map[string]string {
				return map[string]string{
					"unified-app-id":    "uapp_self_hr_A",
					"event-key":         "app_user_hr_employee_joined",
					"file":              writeDevEventFile(t, "hr-employee-joined.json", devEventTestDefinition),
					"expected-revision": "2",
				}
			},
			wantParams: map[string]any{
				"unifiedAppId":           "uapp_self_hr_A",
				"eventKey":               "app_user_hr_employee_joined",
				"name":                   "员工入职",
				"expectedConfigRevision": 2,
			},
			wantAbsent: []string{"file"},
		},
		{
			name:  "validate",
			build: newDevEventValidateCommand,
			tool:  devEventValidateTool,
			flags: func(*testing.T) map[string]string {
				return map[string]string{"unified-app-id": "uapp_self_hr_A", "event-key": "app_user_hr_employee_joined"}
			},
			wantParams: map[string]any{"unifiedAppId": "uapp_self_hr_A", "eventKey": "app_user_hr_employee_joined"},
			wantAbsent: []string{"expectedConfigRevision"},
		},
		{
			name:    "debug",
			build:   newDevEventDebugCommand,
			tool:    devEventDebugTool,
			confirm: true,
			flags: func(t *testing.T) map[string]string {
				return map[string]string{
					"unified-app-id":    "uapp_self_hr_A",
					"event-key":         "app_user_hr_employee_joined",
					"expected-revision": "1",
					"payload-file":      writeDevEventFile(t, "payload.json", `{"employeeId":"E-9"}`),
					"timeout":           " 120s ",
				}
			},
			wantParams: map[string]any{
				"unifiedAppId":           "uapp_self_hr_A",
				"eventKey":               "app_user_hr_employee_joined",
				"expectedConfigRevision": 1,
				"samplePayload":          map[string]any{"employeeId": "E-9"},
				// 规范化后仍回传用户原始写法，服务端与日志才对得上。
				"timeout": "120s",
			},
			wantAbsent: []string{"payloadFile"},
		},
		{
			name:    "publish",
			build:   newDevEventPublishCommand,
			tool:    devEventPublishTool,
			confirm: true,
			flags: func(*testing.T) map[string]string {
				return map[string]string{"unified-app-id": "uapp_self_hr_A", "event-key": "app_user_hr_employee_joined", "expected-revision": "1"}
			},
			wantParams: map[string]any{
				"unifiedAppId":           "uapp_self_hr_A",
				"eventKey":               "app_user_hr_employee_joined",
				"expectedConfigRevision": 1,
			},
			wantAbsent: []string{"timeout", "samplePayload"},
		},
		{
			name:    "offline",
			build:   newDevEventOfflineCommand,
			tool:    devEventOfflineTool,
			confirm: true,
			flags: func(*testing.T) map[string]string {
				return map[string]string{"unified-app-id": "uapp_self_hr_A", "event-key": "app_user_hr_employee_joined", "expected-revision": "3"}
			},
			wantParams: map[string]any{
				"unifiedAppId":           "uapp_self_hr_A",
				"eventKey":               "app_user_hr_employee_joined",
				"expectedConfigRevision": 3,
			},
			wantAbsent: []string{"file"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runner := &fakeDevEventRunner{}
			cmd, _ := prepareDevEventLeaf(t, c.build(runner))
			if c.confirm {
				if err := cmd.Flags().Set("yes", "true"); err != nil {
					t.Fatal(err)
				}
			}
			for name, value := range c.flags(t) {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if runner.calls != 1 {
				t.Fatalf("runner calls = %d, want exactly 1", runner.calls)
			}
			if runner.got.Tool != c.tool {
				t.Fatalf("tool = %q, want %q", runner.got.Tool, c.tool)
			}
			// 派发复用 devapp 的 op-app 网关：不新增产品端点。
			if runner.got.CanonicalProduct != devEventProduct || devEventProduct != "devapp" {
				t.Fatalf("canonical product = %q, want %q", runner.got.CanonicalProduct, "devapp")
			}
			if runner.got.Kind != "helper_invocation" {
				t.Fatalf("invocation kind = %q, want helper_invocation", runner.got.Kind)
			}
			for key, want := range c.wantParams {
				if got := runner.got.Params[key]; !reflect.DeepEqual(got, want) {
					t.Fatalf("param %s = %#v (%T), want %#v (%T)", key, got, got, want, want)
				}
			}
			for _, key := range c.wantAbsent {
				if got, present := runner.got.Params[key]; present {
					t.Fatalf("param %s present = %#v, want absent", key, got)
				}
			}
			// PostMount 必须挂上工具注解与 NoArgs，否则 schema 旁路与位置参数
			// 拒绝都会失效。
			if cmd.Annotations["mcp-tool"] != c.tool {
				t.Fatalf("mcp-tool annotation = %q, want %q", cmd.Annotations["mcp-tool"], c.tool)
			}
			if cmd.Args == nil {
				t.Fatal("Args == nil, want NoArgs via PostMount")
			}
		})
	}
}

// TestDevEventWriteLeavesRequireConfirmation 验证四条写叶子（含 debug）在没有
// --yes / --dry-run 时被确认门拦下，且拦截发生在任何后端调用之前。
func TestDevEventWriteLeavesRequireConfirmation(t *testing.T) {
	builders := map[string]func(executor.Runner) *cobra.Command{
		"create":  newDevEventCreateCommand,
		"update":  newDevEventUpdateCommand,
		"debug":   newDevEventDebugCommand,
		"publish": newDevEventPublishCommand,
		"offline": newDevEventOfflineCommand,
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			runner := &fakeDevEventRunner{}
			cmd, _ := prepareDevEventLeaf(t, build(runner))
			_ = cmd.Flags().Set("unified-app-id", "uapp_self_hr_A")
			err := cmd.RunE(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), "需要用户确认") {
				t.Fatalf("RunE() without --yes error = %v, want 需要用户确认", err)
			}
			if runner.calls != 0 {
				t.Fatalf("backend called before confirmation: %#v", runner.got)
			}
		})
	}
}

// TestDevEventReadLeavesSkipConfirmation 验证只读叶子不设确认门：list/get/
// validate 在没有 --yes 时直接派发。
func TestDevEventReadLeavesSkipConfirmation(t *testing.T) {
	builders := map[string]func(executor.Runner) *cobra.Command{
		"list":     newDevEventListCommand,
		"get":      newDevEventGetCommand,
		"validate": newDevEventValidateCommand,
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			runner := &fakeDevEventRunner{}
			cmd, _ := prepareDevEventLeaf(t, build(runner))
			_ = cmd.Flags().Set("unified-app-id", "uapp_self_hr_A")
			_ = cmd.Flags().Set("event-key", "app_user_hr_employee_joined")
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("RunE() error = %v", err)
			}
			if runner.calls != 1 {
				t.Fatalf("runner calls = %d, want 1", runner.calls)
			}
		})
	}
}

// TestDevEventRequiredFlagHints 验证每个必填参数缺失时逐字回传自己的
// RequiredHint，而不是框架的通用措辞：Agent 只能靠这句话恢复。
func TestDevEventRequiredFlagHints(t *testing.T) {
	cases := []struct {
		name    string
		build   func(executor.Runner) *cobra.Command
		set     map[string]string
		confirm bool
		want    string
	}{
		{"list missing app id", newDevEventListCommand, nil, false, "--unified-app-id 为必填"},
		{"get missing event key", newDevEventGetCommand, map[string]string{"unified-app-id": "A"}, false, "--event-key 为必填"},
		{"create missing file", newDevEventCreateCommand, map[string]string{"unified-app-id": "A"}, true, "--file 为必填"},
		{
			"update missing revision", newDevEventUpdateCommand,
			map[string]string{"unified-app-id": "A", "event-key": "app_user_a_b_c", "file": "unused.json"},
			true, "--expected-revision 为必填",
		},
		{
			"zero revision is treated as missing", newDevEventPublishCommand,
			map[string]string{"unified-app-id": "A", "event-key": "app_user_a_b_c", "expected-revision": "0"},
			true, "--expected-revision 为必填",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runner := &fakeDevEventRunner{}
			cmd, _ := prepareDevEventLeaf(t, c.build(runner))
			if c.confirm {
				if err := cmd.Flags().Set("yes", "true"); err != nil {
					t.Fatal(err)
				}
			}
			for name, value := range c.set {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}
			err := cmd.RunE(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("RunE() error = %v, want containing %q", err, c.want)
			}
			if runner.calls != 0 {
				t.Fatalf("backend called with missing required flag: %#v", runner.got)
			}
		})
	}
}

// TestDevEventValidateRevisionRejectsNegative 覆盖 Validate 钩子：0 由必填拦下，
// 负值必须由 devEventValidateRevision 拦下，避免「未传被当作 0/1 提交」。
func TestDevEventValidateRevisionRejectsNegative(t *testing.T) {
	for _, name := range []string{"update", "debug", "publish", "offline"} {
		t.Run(name, func(t *testing.T) {
			runner := &fakeDevEventRunner{}
			var cmd *cobra.Command
			switch name {
			case "update":
				cmd = newDevEventUpdateCommand(runner)
			case "debug":
				cmd = newDevEventDebugCommand(runner)
			case "publish":
				cmd = newDevEventPublishCommand(runner)
			default:
				cmd = newDevEventOfflineCommand(runner)
			}
			cmd, _ = prepareDevEventLeaf(t, cmd)
			_ = cmd.Flags().Set("yes", "true")
			_ = cmd.Flags().Set("unified-app-id", "uapp_self_hr_A")
			_ = cmd.Flags().Set("event-key", "app_user_hr_employee_joined")
			_ = cmd.Flags().Set("expected-revision", "-1")
			if name == "update" {
				_ = cmd.Flags().Set("file", writeDevEventFile(t, "d.json", devEventTestDefinition))
			}
			err := cmd.RunE(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), "--expected-revision 必须 ≥ 1") {
				t.Fatalf("RunE() error = %v, want --expected-revision 必须 ≥ 1", err)
			}
			if runner.calls != 0 {
				t.Fatal("negative revision reached the backend")
			}
		})
	}

	// 钩子本身也要能独立通过。
	cmd, _ := prepareDevEventLeaf(t, newDevEventPublishCommand(&fakeDevEventRunner{}))
	_ = cmd.Flags().Set("expected-revision", "7")
	if err := devEventValidateRevision(cmd, nil); err != nil {
		t.Fatalf("devEventValidateRevision() error = %v, want nil", err)
	}
}

// TestDevEventEnumRejection 验证 --status / --view 的取值域在运行时被强制，
// 不是只写在 --help 里的装饰。
func TestDevEventEnumRejection(t *testing.T) {
	t.Run("list status", func(t *testing.T) {
		runner := &fakeDevEventRunner{}
		cmd, _ := prepareDevEventLeaf(t, newDevEventListCommand(runner))
		_ = cmd.Flags().Set("unified-app-id", "A")
		_ = cmd.Flags().Set("status", "draft")
		err := cmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), `参数 --status 取值 "draft" 不合法，允许值：DRAFT, ONLINE, OFFLINE`) {
			t.Fatalf("RunE() error = %v, want enum rejection", err)
		}
		if runner.calls != 0 {
			t.Fatal("invalid status reached the backend")
		}
	})
	t.Run("get view", func(t *testing.T) {
		runner := &fakeDevEventRunner{}
		cmd, _ := prepareDevEventLeaf(t, newDevEventGetCommand(runner))
		_ = cmd.Flags().Set("unified-app-id", "A")
		_ = cmd.Flags().Set("event-key", "app_user_a_b_c")
		_ = cmd.Flags().Set("view", "DRAFT")
		err := cmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), `参数 --view 取值 "DRAFT" 不合法，允许值：draft, published`) {
			t.Fatalf("RunE() error = %v, want enum rejection", err)
		}
	})
	t.Run("accepted values pass", func(t *testing.T) {
		for _, status := range []string{"DRAFT", "ONLINE", "OFFLINE"} {
			runner := &fakeDevEventRunner{}
			cmd, _ := prepareDevEventLeaf(t, newDevEventListCommand(runner))
			_ = cmd.Flags().Set("unified-app-id", "A")
			_ = cmd.Flags().Set("status", status)
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("status %s error = %v", status, err)
			}
		}
	})
}

// TestTransformDevEventTimeout 覆盖 --timeout 的三类结论：合法时长原样回传、
// 不可解析、超出有界等待范围。
func TestTransformDevEventTimeout(t *testing.T) {
	t.Run("valid keeps the user spelling", func(t *testing.T) {
		for raw, want := range map[string]string{"120s": "120s", " 2m ": "2m", "1s": "1s", "5m": "5m"} {
			got, err := transformDevEventTimeout(raw)
			if err != nil {
				t.Fatalf("transformDevEventTimeout(%q) error = %v", raw, err)
			}
			if got != want {
				t.Fatalf("transformDevEventTimeout(%q) = %#v, want %q", raw, got, want)
			}
		}
	})
	t.Run("unparsable", func(t *testing.T) {
		_, err := transformDevEventTimeout("soon")
		if err == nil || !strings.Contains(err.Error(), "--timeout 不是合法的时长") {
			t.Fatalf("error = %v, want --timeout 不是合法的时长", err)
		}
	})
	t.Run("out of bounds", func(t *testing.T) {
		for _, raw := range []string{"500ms", "0s", "6m", "1h"} {
			_, err := transformDevEventTimeout(raw)
			if err == nil || !strings.Contains(err.Error(), "--timeout 必须在 1s 到 5m0s 之间") {
				t.Fatalf("transformDevEventTimeout(%q) error = %v, want bounds rejection", raw, err)
			}
		}
	})
	t.Run("rejects at the leaf", func(t *testing.T) {
		runner := &fakeDevEventRunner{}
		cmd, _ := prepareDevEventLeaf(t, newDevEventDebugCommand(runner))
		_ = cmd.Flags().Set("yes", "true")
		_ = cmd.Flags().Set("unified-app-id", "A")
		_ = cmd.Flags().Set("event-key", "app_user_a_b_c")
		_ = cmd.Flags().Set("expected-revision", "1")
		_ = cmd.Flags().Set("timeout", "30m")
		err := cmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "--timeout 必须在") {
			t.Fatalf("RunE() error = %v, want bounds rejection", err)
		}
		if runner.calls != 0 {
			t.Fatal("unbounded timeout reached the backend")
		}
	})
}

// TestDevEventRunnerErrorPropagates 覆盖传输错误分支：runner 报错时原样上抛，
// 不写信封、不伪装成业务失败。
func TestDevEventRunnerErrorPropagates(t *testing.T) {
	runner := &fakeDevEventRunner{err: errors.New("transport dial failed")}
	cmd, stdout := prepareDevEventLeaf(t, newDevEventGetCommand(runner))
	_ = cmd.Flags().Set("unified-app-id", "A")
	_ = cmd.Flags().Set("event-key", "app_user_a_b_c")
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "transport dial failed") {
		t.Fatalf("RunE() error = %v, want transport dial failed", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on transport error", stdout.String())
	}
}

// TestDevEventEnvelopeExit 验证成功路径逐字走统一信封出口，并且 --dry-run 只
// 追加 dry_run:true，不改 outcome：dry-run 是已完成的本地预演，不是未终态。
func TestDevEventEnvelopeExit(t *testing.T) {
	content := map[string]any{
		"unifiedAppId":   "uapp_self_hr_A",
		"eventKey":       "app_user_hr_employee_joined",
		"status":         "DRAFT",
		"configRevision": float64(2),
	}
	for _, dryRun := range []bool{false, true} {
		runner := &fakeDevEventRunner{response: map[string]any{"content": content}}
		cmd, stdout := prepareDevEventLeaf(t, newDevEventGetCommand(runner))
		_ = cmd.Flags().Set("unified-app-id", "uapp_self_hr_A")
		_ = cmd.Flags().Set("event-key", "app_user_hr_employee_joined")
		if dryRun {
			_ = cmd.Flags().Set("dry-run", "true")
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("dryRun=%v RunE() error = %v", dryRun, err)
		}
		if runner.got.DryRun != dryRun {
			t.Fatalf("invocation dry-run = %v, want %v", runner.got.DryRun, dryRun)
		}
		if !output.UsesUnifiedResult(cmd) {
			// legacy 渲染路径：stdout 必须恰为一个 JSON 信封。
			var env map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not a single JSON envelope: %v (%q)", err, stdout.String())
			}
			if env["ok"] != true || env["outcome"] != "success" {
				t.Fatalf("envelope = %#v, want ok:true outcome:success", env)
			}
			if got, present := env["dry_run"]; present != dryRun || (dryRun && got != true) {
				t.Fatalf("dry_run = %#v (present=%v), want present=%v true", got, present, dryRun)
			}
		}
	}
}
