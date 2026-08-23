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
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// runDevAppFamily 在 stdout/stderr 分流下执行 dev app 命令树（统一输出 dev 域
// 试点族验证，队列 B64~B106）。返回两路缓冲与执行错误，供信封/流纪律断言。
func runDevAppFamily(t *testing.T, runner executor.Runner, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	root := newDevAppTestRoot(runner)
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return &outBuf, &errBuf, err
}

// devAppFamilyContentRunner 返回一个把 content 载荷包进 ServiceResult 的 runner，
// 模拟 op-app 成功响应（normalizeDevAppServiceResult 会解包到 content）。
func devAppFamilyContentRunner(content map[string]any) *devAppResponseRunner {
	return &devAppResponseRunner{
		response: map[string]any{
			"content": map[string]any{
				"success": true,
				"result":  content,
			},
		},
	}
}

func devAppFamilyRawContentRunner(content map[string]any) *devAppResponseRunner {
	runner := devAppFamilyContentRunner(content)
	runner.preserveMissingPagination = true
	return runner
}

// TestDevAppFamilyReadLeavesDualFormat 是队列 B64/B68/B71/B80/B83/B87/B95/B102
// 的读叶子双格式验证（AC-28/M1.7）：默认 json 出完整信封（ok/outcome/data），
// -f table 只渲染 data（不含信封包装键）。业务载荷形状不变。
func TestDevAppFamilyReadLeavesDualFormat(t *testing.T) {
	content := map[string]any{
		"unifiedAppId": "u-1",
		"name":         "DemoApp",
		"appStatus":    "ENABLED",
	}

	cases := []struct {
		name string
		args []string
	}{
		{"event list", []string{"dev", "app", "event", "list", "--unified-app-id", "u-1"}},
		{"app list", []string{"dev", "app", "list", "--name", "DemoApp"}},
		{"app get", []string{"dev", "app", "get", "--unified-app-id", "u-1"}},
		{"credentials get", []string{"dev", "app", "credentials", "get", "--unified-app-id", "u-1"}},
		{"webapp get", []string{"dev", "app", "webapp", "get", "--unified-app-id", "u-1"}},
		{"permission list", []string{"dev", "app", "permission", "list", "--unified-app-id", "u-1"}},
		{"member list", []string{"dev", "app", "member", "list", "--unified-app-id", "u-1"}},
		{"robot config get", []string{"dev", "app", "robot", "get", "--unified-app-id", "u-1"}},
		{"robot result", []string{"dev", "app", "robot", "result", "--task-id", "t-1"}},
		{"version list", []string{"dev", "app", "version", "list", "--unified-app-id", "u-1"}},
		{"version get", []string{"dev", "app", "version", "get", "--unified-app-id", "u-1", "--version-id", "v-1"}},
		{"version check-approval", []string{"dev", "app", "version", "check-approval", "--unified-app-id", "u-1", "--version-id", "v-1"}},
		{"version status", []string{"dev", "app", "version", "status", "--unified-app-id", "u-1", "--version-id", "v-1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/json", func(t *testing.T) {
			out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(content), tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}
			env := decodePhaseFEnvelope(t, out.Bytes())
			if !env.OK || env.Outcome != "success" {
				t.Fatalf("envelope ok/outcome = %v/%q, want true/success: %s", env.OK, env.Outcome, out.String())
			}
			if env.DryRun {
				t.Fatalf("read leaf must not set dry_run: %s", out.String())
			}
			if env.Data == nil {
				t.Fatalf("envelope data is nil: %s", out.String())
			}
		})

		t.Run(tc.name+"/table", func(t *testing.T) {
			args := append([]string{}, tc.args...)
			args = append(args, "--format", "table")
			out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(content), args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}
			s := out.String()
			// table 只渲染 data：不得出现信封包装键，但要见业务载荷值。
			if strings.Contains(s, `"outcome"`) || strings.Contains(s, `"ok"`) {
				t.Fatalf("-f table must render data only, envelope leaked: %s", s)
			}
			if !strings.Contains(s, "DemoApp") {
				t.Fatalf("-f table output missing data payload value: %s", s)
			}
		})
	}
}

// TestDevAppFamilyWriteLeavesDryRunEnvelope 是队列 B65/B66/B73/B74/B81/B84/B85/
// B88/B89/B91/B93/B96/B100/B104 的写叶子 dry-run 预览断言（契约规范 §6 场景6）：
// --dry-run 出 ok:true/outcome:success + dry_run:true，无需 --yes，参数校验先于确认门。
func TestDevAppFamilyWriteLeavesDryRunEnvelope(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"event subscribe", []string{"dev", "app", "event", "subscribe", "--unified-app-id", "u-1", "--event-codes", "a,b"}},
		{"event unsubscribe", []string{"dev", "app", "event", "unsubscribe", "--unified-app-id", "u-1", "--event-codes", "a,b"}},
		{"app create", []string{"dev", "app", "create", "--name", "DemoApp"}},
		{"app update", []string{"dev", "app", "update", "--unified-app-id", "u-1", "--name", "NewName"}},
		{"app disable", []string{"dev", "app", "disable", "--unified-app-id", "u-1"}},
		{"app enable", []string{"dev", "app", "enable", "--unified-app-id", "u-1"}},
		{"webapp config", []string{"dev", "app", "webapp", "config", "--unified-app-id", "u-1", "--homepage-url", "https://example.com"}},
		{"permission add", []string{"dev", "app", "permission", "add", "--unified-app-id", "u-1", "--scope-values", "Contact.User.mobile"}},
		{"permission remove", []string{"dev", "app", "permission", "remove", "--unified-app-id", "u-1", "--scope-values", "Contact.User.mobile"}},
		{"member add", []string{"dev", "app", "member", "add", "--unified-app-id", "u-1", "--user-ids", "user-1", "--member-type", "DEVELOPER"}},
		{"member remove", []string{"dev", "app", "member", "remove", "--unified-app-id", "u-1", "--user-ids", "user-1", "--member-type", "DEVELOPER"}},
		{"security config", []string{"dev", "app", "security", "config", "--unified-app-id", "u-1", "--redirect-urls", "https://cb.example.invalid/cb"}},
		{"robot submit", []string{"dev", "app", "robot", "submit", "--name", "智能体", "--robot-name", "小助手", "--desc", "审批问答"}},
		{"robot config", []string{"dev", "app", "robot", "config", "--unified-app-id", "u-1", "--name", "小助手"}},
		{"robot enable", []string{"dev", "app", "robot", "enable", "--unified-app-id", "u-1"}},
		{"robot disable", []string{"dev", "app", "robot", "disable", "--unified-app-id", "u-1"}},
		{"version create", []string{"dev", "app", "version", "create", "--unified-app-id", "u-1", "--desc", "新增机器人"}},
		{"version publish", []string{"dev", "app", "version", "publish", "--unified-app-id", "u-1", "--version-id", "v-1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{}, tc.args...)
			args = append(args, "--dry-run")
			out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(map[string]any{"ok": true}), args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}
			env := decodePhaseFEnvelope(t, out.Bytes())
			// dry-run 是已完成的预演，不是尚未终态的异步操作。
			if !env.OK || env.Outcome != "success" || !env.DryRun {
				t.Fatalf("dry-run envelope ok/outcome/dry_run = %v/%q/%v, want true/success/true: %s",
					env.OK, env.Outcome, env.DryRun, out.String())
			}
			// dry-run 不投影分页 meta。
			if strings.Contains(out.String(), `"pagination"`) {
				t.Fatalf("dry-run must not carry pagination meta: %s", out.String())
			}
		})
	}
}

// TestDevAppListPaginationProjectsMeta 是队列 B69（契约规范 §3）：列表载荷的
// cursor 分页字段投影到 meta.pagination。hasMore=true+nextCursor →
// endpoint_exhausted:false + next_token（可续跑）；hasMore=false → exhausted:true；
// 已声明分页的工具缺少两类标记时 fail-closed，不得伪装为完整成功。
func TestDevAppListPaginationProjectsMeta(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		content   map[string]any
		wantNext  string
		wantExh   bool
		wantHasPg bool
		wantFail  bool
	}{
		{
			name:      "app list has more with cursor",
			args:      []string{"dev", "app", "list", "--name", "DemoApp"},
			content:   map[string]any{"items": []any{}, "hasMore": true, "nextCursor": "tok-9"},
			wantNext:  "tok-9",
			wantExh:   false,
			wantHasPg: true,
		},
		{
			name:      "app list exhausted",
			args:      []string{"dev", "app", "list", "--name", "DemoApp"},
			content:   map[string]any{"items": []any{}, "hasMore": false},
			wantExh:   true,
			wantHasPg: true,
		},
		{
			name:     "app list no pagination fields",
			args:     []string{"dev", "app", "list", "--name", "DemoApp"},
			content:  map[string]any{"items": []any{}},
			wantFail: true,
		},
		{
			name:      "version list has more with cursor",
			args:      []string{"dev", "app", "version", "list", "--unified-app-id", "u-1"},
			content:   map[string]any{"items": []any{}, "hasMore": true, "nextCursor": "v-tok-3"},
			wantNext:  "v-tok-3",
			wantExh:   false,
			wantHasPg: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := devAppFamilyContentRunner(tc.content)
			if tc.wantFail {
				runner = devAppFamilyRawContentRunner(tc.content)
			}
			out, errBuf, err := runDevAppFamily(t, runner, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errBuf.String())
			}
			var env struct {
				OK      bool   `json:"ok"`
				Outcome string `json:"outcome"`
				Error   *struct {
					Subtype string `json:"subtype"`
				} `json:"error"`
				Meta struct {
					Pagination *struct {
						EndpointExhausted bool   `json:"endpoint_exhausted"`
						NextToken         string `json:"next_token"`
					} `json:"pagination"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, out.String())
			}
			if tc.wantFail {
				if env.OK || env.Outcome != "failure" || env.Error == nil || env.Error.Subtype != "pagination_inconsistent" {
					t.Fatalf("missing pagination markers must fail closed: %s", out.String())
				}
				return
			}
			if !tc.wantHasPg {
				if env.Meta.Pagination != nil {
					t.Fatalf("pagination must be absent, got %+v: %s", env.Meta.Pagination, out.String())
				}
				return
			}
			if env.Meta.Pagination == nil {
				t.Fatalf("pagination missing: %s", out.String())
			}
			if env.Meta.Pagination.EndpointExhausted != tc.wantExh {
				t.Fatalf("endpoint_exhausted = %v, want %v: %s",
					env.Meta.Pagination.EndpointExhausted, tc.wantExh, out.String())
			}
			if env.Meta.Pagination.NextToken != tc.wantNext {
				t.Fatalf("next_token = %q, want %q: %s", env.Meta.Pagination.NextToken, tc.wantNext, out.String())
			}
		})
	}
}

// TestDevAppListNameFilterPassthrough 是队列 B70：信封迁移不改业务参数——
// --name 过滤仍原样透传到工具调用（只观察装配，不改变行为）。
func TestDevAppListNameFilterPassthrough(t *testing.T) {
	runner := &captureRunner{}
	out, errBuf, err := runDevAppFamily(t, runner, "dev", "app", "list", "--name", "Waker")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errBuf.String())
	}
	if runner.last.Tool != "list_dev_app" {
		t.Fatalf("Tool = %q, want list_dev_app", runner.last.Tool)
	}
	if got := runner.last.Params["name"]; got != "Waker" {
		t.Fatalf("name = %#v, want Waker (信封迁移不得改业务参数)", got)
	}
	_ = out
}

// TestDevAppGetLocatorBranches 是队列 B72：--unified-app-id 与 --app-key 两个
// 分支各自的信封断言（AC-28）——两者都能出成功信封，业务参数各自透传。
func TestDevAppGetLocatorBranches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantKey string
		wantVal string
	}{
		{"by unified-app-id", []string{"dev", "app", "get", "--unified-app-id", "u-1"}, "unifiedAppId", "u-1"},
		{"by app-key", []string{"dev", "app", "get", "--app-key", "dingxxx"}, "appKey", "dingxxx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			out, errBuf, err := runDevAppFamily(t, runner, tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errBuf.String())
			}
			env := decodePhaseFEnvelope(t, out.Bytes())
			if !env.OK || env.Outcome != "success" {
				t.Fatalf("envelope ok/outcome = %v/%q, want true/success: %s", env.OK, env.Outcome, out.String())
			}
			if got := runner.last.Params[tc.wantKey]; got != tc.wantVal {
				t.Fatalf("%s = %#v, want %q", tc.wantKey, got, tc.wantVal)
			}
		})
	}
}

// TestDevAppCredentialsSecretFieldsPreserved 是队列 B78：clientSecret 等敏感字段
// 经信封迁移后仍原样保留在 data 层（jsonutil 关闭 HTML 转义，无 \u003c 污染）。
func TestDevAppCredentialsSecretFieldsPreserved(t *testing.T) {
	content := map[string]any{
		"appKey":       "dingxxx",
		"appSecret":    "secret-app",
		"clientSecret": "secret-client<>&",
	}
	out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(content),
		"dev", "app", "credentials", "get", "--unified-app-id", "u-1")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errBuf.String())
	}
	s := out.String()
	for _, want := range []string{"secret-app", "secret-client<>&"} {
		if !strings.Contains(s, want) {
			t.Fatalf("credentials output missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\\u003c") {
		t.Fatalf("HTML escaping must be disabled, got \\u003c: %s", s)
	}
}

// TestDevAppDeleteDryRunPreviewsWithoutConfirmName 是队列 B75：delete 的
// dry-run 无需 --confirm-name 即出 success 预览信封（AC-28/AC-03——真实删除
// 才要求二次确认；dry-run 是读应用名的预览路径）。
func TestDevAppDeleteDryRunPreviewsWithoutConfirmName(t *testing.T) {
	out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(map[string]any{"name": "DemoApp"}),
		"dev", "app", "delete", "--unified-app-id", "u-1", "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	env := decodePhaseFEnvelope(t, out.Bytes())
	if !env.OK || env.Outcome != "success" || !env.DryRun {
		t.Fatalf("delete dry-run envelope ok/outcome/dry_run = %v/%q/%v, want true/success/true: %s",
			env.OK, env.Outcome, env.DryRun, out.String())
	}

	// 真实删除（无 --dry-run）缺 --confirm-name 时报 validation 错误（不进信封）。
	out2, _, err := runDevAppFamily(t, devAppFamilyContentRunner(map[string]any{"name": "DemoApp"}),
		"dev", "app", "delete", "--unified-app-id", "u-1", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--confirm-name") {
		t.Fatalf("real delete without confirm-name error = %v, want --confirm-name validation", err)
	}
	if strings.Contains(out2.String(), `"outcome": "success"`) {
		t.Fatalf("delete validation error must not emit success envelope: %s", out2.String())
	}
}

// TestDevAppFamilyValidationErrorsKeepEnvelopeOff 是队列 B67/B79/B82/B86/B90/
// B92/B99/B106 的错误路径回归：参数校验失败继续走 apperrors 通道（不进信封），
// stdout 不含信封（由 root 错误处理器承接），错误信息可定位。
func TestDevAppFamilyValidationErrorsKeepEnvelopeOff(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"get requires locator", []string{"dev", "app", "get"}, "请传入 --unified-app-id 或 --app-key"},
		{"event subscribe requires codes", []string{"dev", "app", "event", "subscribe", "--unified-app-id", "u-1", "--dry-run"}, "--event-codes 为必填"},
		{"member add requires users", []string{"dev", "app", "member", "add", "--unified-app-id", "u-1", "--member-type", "DEVELOPER", "--dry-run"}, "--user-ids 为必填"},
		{"update requires one field", []string{"dev", "app", "update", "--unified-app-id", "u-1", "--dry-run"}, "至少提供一项待更新字段"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runDevAppFamily(t, &captureRunner{}, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, tc.wantErr)
			}
			// 校验失败时 stdout 不得出现成功信封。
			if strings.Contains(out.String(), `"outcome": "success"`) {
				t.Fatalf("validation error must not emit success envelope: %s", out.String())
			}
		})
	}
}
