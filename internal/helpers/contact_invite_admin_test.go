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
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// runContactInviteAdminCommand 在 helpers 包内直接执行 contact 子树并捕获
// MCP 调用。CI coverage gate 只统计 helpers 包内测试对 helpers 代码的覆盖
// （app 包测试的跨包覆盖不进入 profile），invite/apply/exclusive-account
// 新命令的分支因此需要包内用例兜底。--yes 是 root 的 persistent flag，
// 脱离完整 root 单测时补注册以等价用户显式确认。
func runContactInviteAdminCommand(t *testing.T, args ...string) (*contactEnterpriseCaller, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	caller := &contactEnterpriseCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", "contact"}, args...)

	cmd := newContactCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.SetArgs(append([]string{"--yes"}, args...))
	return caller, cmd.Execute()
}

// TestContactInviteAdminFlagEdges 覆盖新增 invite/apply/exclusive-account 命令
// 的 flag 边缘分支：可选布尔开关的空白值/非法值/合法值、布尔 flag 位置参数
// 归并、必填项传空白值、以及 no-audit 缺失与类型错误。
func TestContactInviteAdminFlagEdges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		toolName string
		wantArgs map[string]any
	}{
		{
			name:     "invite switch blank optional bool is ignored",
			args:     []string{"org", "invite-switch", "--open", "true", "--search-invite", " "},
			toolName: "set_org_invite_switch",
			wantArgs: map[string]any{"open": true},
		},
		{
			name:     "invite switch apply code invite true",
			args:     []string{"org", "invite-switch", "--open", "true", "--apply-code-invite", "true"},
			toolName: "set_org_invite_switch",
			wantArgs: map[string]any{"open": true, "orgApplyCodeInviteSwitch": true},
		},
		{
			name:     "org invite audit positional false merges into flag",
			args:     []string{"org", "invite-audit", "--no-audit", "false"},
			toolName: "set_org_apply_audit",
			wantArgs: map[string]any{"auditType": int64(1)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runContactInviteAdminCommand(t, tc.args...)
			if err != nil {
				t.Fatalf("%s execute: %v", tc.name, err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("%s want exactly 1 MCP call, got %d: %+v", tc.name, len(caller.calls), caller.calls)
			}
			call := caller.calls[0]
			if call.productID != "contact" || call.toolName != tc.toolName {
				t.Fatalf("%s call = %s/%s, want contact/%s", tc.name, call.productID, call.toolName, tc.toolName)
			}
			if !reflect.DeepEqual(call.args, tc.wantArgs) {
				t.Fatalf("%s args = %#v, want %#v", tc.name, call.args, tc.wantArgs)
			}
		})
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invite switch apply code invite rejects non boolean",
			args: []string{"org", "invite-switch", "--open", "true", "--apply-code-invite", "yes"},
			want: "--apply-code-invite 必须是 boolean",
		},
		{
			name: "exclusive account disable rejects blank staff id",
			args: []string{"exclusive-account", "disable", "--staff-id", " "},
			want: "--staff-id 不能为空",
		},
		{
			name: "org apply block rejects blank reason",
			args: []string{"org", "apply-block", "--id", "123", "--reason", " "},
			want: "--reason 不能为空",
		},
		{
			name: "dept invite audit requires explicit no audit",
			args: []string{"dept", "invite-audit", "--dept", "12345"},
			want: "必须显式传 --no-audit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runContactInviteAdminCommand(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s err = %v, want contains %q", tc.name, err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("%s must not reach MCP, got calls: %+v", tc.name, caller.calls)
			}
		})
	}

	t.Run("no audit get bool type error", func(t *testing.T) {
		cmd := &cobra.Command{Use: "flags"}
		cmd.Flags().String("no-audit", "", "")
		_ = cmd.Flags().Set("no-audit", "unexpected")
		if _, err := contactRequireNoAuditFlag(cmd); err == nil || !strings.Contains(err.Error(), "--no-audit 解析失败") {
			t.Fatalf("contactRequireNoAuditFlag err = %v, want --no-audit 解析失败", err)
		}
	})
}

// TestContactApplyListSafetyProjectionMatchesBehavior 是 apply-list 安全语义
// 的包内回归（CI coverage gate 只统计包内覆盖）：查询会把服务端未读申请
// 标记为已读，最终投影必须声明 Effect=write 而非把有副作用的查询伪装成
// 纯读取；副作用仅清除未读标记，Confirmation 保持 not_required，运行侧
// 不带 --yes 也直接调用 query_org_apply_list、不出现确认 gate。
func TestContactApplyListSafetyProjectionMatchesBehavior(t *testing.T) {
	root := newContactCommand()
	applyList := requireWukongSyncCommand(t, root, "org", "apply-list")
	payload, ok := contractfinal.RuntimeContractFinal(applyList)
	if !ok {
		t.Fatal("apply-list has no runtime contract final payload")
	}
	if payload.Safety == nil {
		t.Fatal("apply-list payload has no safety declaration")
	}
	if got := payload.Safety; got.Effect != "write" || got.Risk != "low" ||
		got.Confirmation != "not_required" || got.Idempotency != "idempotent" {
		t.Fatalf("apply-list safety = %+v, want write/low/not_required/idempotent", got)
	}
	if !strings.Contains(payload.Description, "标记为已读") {
		t.Fatalf("apply-list description %q must disclose the read-marking side effect", payload.Description)
	}
	if payload.Selection == nil || !strings.Contains(payload.Selection.AgentSummary, "标记为已读") {
		t.Fatalf("apply-list selection must disclose the read-marking side effect: %+v", payload.Selection)
	}

	// 运行行为与声明一致：not_required 不设确认 gate，不带 --yes 直接执行。
	caller, err := runContactEnterpriseCommand(t, "org", "apply-list")
	if err != nil {
		t.Fatalf("apply-list without --yes must not hit the confirmation gate: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("apply-list want exactly 1 MCP call, got %d: %+v", len(caller.calls), caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "contact" || call.toolName != "query_org_apply_list" {
		t.Fatalf("apply-list call = %s/%s, want contact/query_org_apply_list", call.productID, call.toolName)
	}
	if want := map[string]any{"status": int64(1), "size": int64(20)}; !reflect.DeepEqual(call.args, want) {
		t.Fatalf("apply-list args = %#v, want %#v", call.args, want)
	}
}

// TestContactApplyRemoveSafetyProjectionMatchesBehavior 是 apply-remove 安全语义的
// 包内回归（CI coverage gate 只统计包内覆盖）：删除申请记录不可恢复，最终
// 投影必须声明 Effect=destructive、Confirmation=user_required；运行侧未确认
// 必须被门禁拦截，确认后精确调用 remove_org_apply。
func TestContactApplyRemoveSafetyProjectionMatchesBehavior(t *testing.T) {
	root := newContactCommand()
	applyRemove := requireWukongSyncCommand(t, root, "org", "apply-remove")
	payload, ok := contractfinal.RuntimeContractFinal(applyRemove)
	if !ok {
		t.Fatal("apply-remove has no runtime contract final payload")
	}
	if payload.Safety == nil {
		t.Fatal("apply-remove payload has no safety declaration")
	}
	if got := payload.Safety; got.Effect != "destructive" || got.Risk != "high" ||
		got.Confirmation != "user_required" || got.Idempotency != "non_idempotent" {
		t.Fatalf("apply-remove safety = %+v, want destructive/high/user_required/non_idempotent", got)
	}
	if !strings.Contains(payload.Description, "不可恢复") {
		t.Fatalf("apply-remove description %q must disclose non-recoverable deletion", payload.Description)
	}
	if payload.Selection == nil || !strings.Contains(payload.Selection.AgentSummary, "不可恢复") {
		t.Fatalf("apply-remove selection must disclose non-recoverable deletion: %+v", payload.Selection)
	}

	// 运行行为与声明一致：user_required 必须显式 --yes 才执行。
	if _, err := runContactEnterpriseCommand(t, "org", "apply-remove", "--id", "123"); err == nil {
		t.Fatal("apply-remove without --yes must be rejected by confirmation gate")
	}

	caller, err := runContactEnterpriseCommand(t, "org", "apply-remove", "--id", "123", "--yes")
	if err != nil {
		t.Fatalf("apply-remove with --yes failed: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("apply-remove want exactly 1 MCP call, got %d: %+v", len(caller.calls), caller.calls)
	}
	call := caller.calls[0]
	if call.productID != "contact" || call.toolName != "remove_org_apply" {
		t.Fatalf("apply-remove call = %s/%s, want contact/remove_org_apply", call.productID, call.toolName)
	}
	if want := map[string]any{"id": int64(123)}; !reflect.DeepEqual(call.args, want) {
		t.Fatalf("apply-remove args = %#v, want %#v", call.args, want)
	}
}

// runContactOrgListCommandCapture 执行 invite-list / apply-list 并捕获 stdout，
// 同时注入自定义 MCP 文本响应。用于验证统一分页映射与 DataSchema 剥离。
func runContactOrgListCommandCapture(t *testing.T, responseText string, args ...string) (*contactEnterpriseCaller, string, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	caller := &contactEnterpriseCaller{responseText: responseText}
	InitDeps(caller)
	os.Args = append([]string{"dws", "contact"}, args...)

	var out bytes.Buffer
	cmd := newContactCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return caller, out.String(), err
}

// TestContactOrgListPaginationProjection 验证 invite-list / apply-list 的最终
// Schema 声明了统一 Pagination 契约、DataSchema 不再包含分页字段，且运行侧
// 把服务端返回的 hasMore/nextCursor 正确投影到 meta.pagination。
func TestContactOrgListPaginationProjection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		path          []string
		toolName      string
		response      string
		wantExhausted bool
		wantToken     string
	}{
		{
			name:          "invite-list declares cursor pagination and maps hasMore/nextCursor",
			path:          []string{"org", "invite-list"},
			toolName:      "list_team_invite",
			response:      `{"result":{"values":[{"id":1,"status":1,"empName":"张三"}],"hasMore":true,"nextCursor":42},"success":true}`,
			wantExhausted: false,
			wantToken:     "42",
		},
		{
			name:          "apply-list declares cursor pagination and strips terminal cursor",
			path:          []string{"org", "apply-list"},
			toolName:      "query_org_apply_list",
			response:      `{"result":{"values":[{"id":2,"status":1,"content":"李四"}],"hasMore":false,"nextCursor":99},"success":true}`,
			wantExhausted: true,
			wantToken:     "",
		},
		{
			name:          "large integer nextCursor preserves precision beyond float53",
			path:          []string{"org", "invite-list"},
			toolName:      "list_team_invite",
			response:      `{"result":{"values":[{"id":3,"status":1,"empName":"王五"}],"hasMore":true,"nextCursor":9007199254740993},"success":true}`,
			wantExhausted: false,
			wantToken:     "9007199254740993",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newContactCommand()
			cmd := requireWukongSyncCommand(t, root, tc.path...)
			payload, ok := contractfinal.RuntimeContractFinal(cmd)
			if !ok {
				t.Fatalf("%s has no runtime contract final payload", strings.Join(tc.path, " "))
			}
			if payload.Pagination == nil {
				t.Fatalf("%s must declare pagination", strings.Join(tc.path, " "))
			}
			if payload.Pagination.Kind != contract.PaginationKindCursor || payload.Pagination.CursorParameter != "cursor" {
				t.Fatalf("%s pagination = %+v, want cursor/cursor", strings.Join(tc.path, " "), payload.Pagination)
			}
			schema := string(payload.Result.DataSchema)
			if strings.Contains(schema, "hasMore") || strings.Contains(schema, "nextCursor") {
				t.Fatalf("%s data_schema must not contain pagination fields: %s", strings.Join(tc.path, " "), schema)
			}

			caller, out, err := runContactOrgListCommandCapture(t, tc.response, tc.path...)
			if err != nil {
				t.Fatalf("%s execute: %v\n%s", strings.Join(tc.path, " "), err, out)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("%s want 1 MCP call, got %d: %+v", strings.Join(tc.path, " "), len(caller.calls), caller.calls)
			}
			call := caller.calls[0]
			if call.productID != "contact" || call.toolName != tc.toolName {
				t.Fatalf("%s call = %s/%s, want contact/%s", strings.Join(tc.path, " "), call.productID, call.toolName, tc.toolName)
			}

			var envelope map[string]any
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatalf("%s output is not valid JSON: %v\n%s", strings.Join(tc.path, " "), err, out)
			}
			meta, ok := envelope["meta"].(map[string]any)
			if !ok {
				t.Fatalf("%s output missing meta: %s", strings.Join(tc.path, " "), out)
			}
			pg, ok := meta["pagination"].(map[string]any)
			if !ok {
				t.Fatalf("%s output missing meta.pagination: %s", strings.Join(tc.path, " "), out)
			}
			if pg["endpoint_exhausted"] != tc.wantExhausted {
				t.Fatalf("%s endpoint_exhausted = %v, want %v", strings.Join(tc.path, " "), pg["endpoint_exhausted"], tc.wantExhausted)
			}
			gotToken, hasToken := pg["next_token"]
			if tc.wantExhausted {
				if hasToken && gotToken != "" {
					t.Fatalf("%s exhausted pagination must not carry next_token, got %v", strings.Join(tc.path, " "), gotToken)
				}
			} else if gotToken != tc.wantToken {
				t.Fatalf("%s next_token = %v, want %q", strings.Join(tc.path, " "), gotToken, tc.wantToken)
			}
			data, ok := envelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("%s output missing data: %s", strings.Join(tc.path, " "), out)
			}
			result, ok := data["result"].(map[string]any)
			if !ok {
				t.Fatalf("%s output data.result missing or not object: %s", strings.Join(tc.path, " "), out)
			}
			if _, exists := result["hasMore"]; exists {
				t.Fatalf("%s data.result must not leak hasMore", strings.Join(tc.path, " "))
			}
			if _, exists := result["nextCursor"]; exists {
				t.Fatalf("%s data.result must not leak nextCursor", strings.Join(tc.path, " "))
			}
		})
	}
}

// TestContactOrgListResultEdgeCases 覆盖 contactOrgListResult 与分页映射的
// 异常分支：非对象 result、非布尔 hasMore、字符串/非法类型 nextCursor、
// hasMore=true 缺少 nextCursor、服务端返回非 JSON/分页不一致、以及结果
// 成功写入 result store 的分支。
func TestContactOrgListResultEdgeCases(t *testing.T) {
	cases := []struct {
		name       string
		response   string
		wantErr    bool
		wantErrMsg string
		wantCalls  int
	}{
		{
			name:       "non-object result is rejected",
			response:   `{"result":"unexpected","success":true}`,
			wantErr:    true,
			wantErrMsg: "服务端返回的 result 必须是对象",
			wantCalls:  1,
		},
		{
			name:       "non-boolean hasMore is rejected",
			response:   `{"result":{"hasMore":"yes"},"success":true}`,
			wantErr:    true,
			wantErrMsg: "hasMore must be a JSON boolean",
			wantCalls:  1,
		},
		{
			name:      "string nextCursor is accepted",
			response:  `{"result":{"values":[],"hasMore":true,"nextCursor":"42"},"success":true}`,
			wantCalls: 1,
		},
		{
			name:      "large integer nextCursor preserves precision",
			response:  `{"result":{"values":[],"hasMore":true,"nextCursor":9007199254740993},"success":true}`,
			wantCalls: 1,
		},
		{
			name:       "decimal nextCursor is rejected",
			response:   `{"result":{"values":[],"hasMore":true,"nextCursor":42.5},"success":true}`,
			wantErr:    true,
			wantErrMsg: "nextCursor must be an integer",
			wantCalls:  1,
		},
		{
			name:       "out-of-range nextCursor is rejected",
			response:   `{"result":{"values":[],"hasMore":true,"nextCursor":9223372036854775808},"success":true}`,
			wantErr:    true,
			wantErrMsg: "nextCursor must be an integer",
			wantCalls:  1,
		},
		{
			name:       "invalid nextCursor type is rejected",
			response:   `{"result":{"values":[],"hasMore":true,"nextCursor":true},"success":true}`,
			wantErr:    true,
			wantErrMsg: "nextCursor must be a JSON string or integer",
			wantCalls:  1,
		},
		{
			name:       "hasMore=true without nextCursor is rejected",
			response:   `{"result":{"values":[],"hasMore":true},"success":true}`,
			wantErr:    true,
			wantErrMsg: "hasMore=true is missing nextCursor",
			wantCalls:  1,
		},
		{
			name:       "non-JSON response is rejected",
			response:   `not json`,
			wantErr:    true,
			wantErrMsg: "非 JSON 文本或 null",
			wantCalls:  1,
		},
		{
			name:       "null response is rejected",
			response:   `null`,
			wantErr:    true,
			wantErrMsg: "非 JSON 文本或 null",
			wantCalls:  1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, out, err := runContactOrgListCommandCapture(t, tc.response, "org", "invite-list")
			if tc.wantErr {
				// 分页不一致通过 output.Failure 返回，因此 err 可能为 nil 但输出含错误信息；
				// 真正的调用/解析错误才会在 err 中返回。
				if err != nil {
					if !strings.Contains(err.Error(), tc.wantErrMsg) {
						t.Fatalf("error does not contain %q: err=%v", tc.wantErrMsg, err)
					}
					return
				}
				if !strings.Contains(out, tc.wantErrMsg) {
					t.Fatalf("output does not contain %q: out=%s", tc.wantErrMsg, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v\n%s", err, out)
			}
		})
	}
}

// TestContactOrgListStoreEmission 验证命令在带有 result store 的上下文里
// 成功通过 StoreResult 返回，覆盖 invite-list / apply-list RunE 中 StoreResult
// 成功后的 return nil 分支。
func TestContactOrgListStoreEmission(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		toolName string
	}{
		{name: "invite-list", args: []string{"org", "invite-list"}, toolName: "list_team_invite"},
		{name: "apply-list", args: []string{"org", "apply-list"}, toolName: "query_org_apply_list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousDeps := deps
			previousArgs := os.Args
			t.Cleanup(func() {
				deps = previousDeps
				os.Args = previousArgs
			})

			caller := &contactEnterpriseCaller{responseText: `{"result":{"values":[]},"success":true}`}
			InitDeps(caller)
			os.Args = append([]string{"dws", "contact"}, tc.args...)

			ctx, _ := output.WithResultStore(context.Background())
			cmd := newContactCommand()
			output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.PersistentFlags().Bool("yes", false, "")
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			cmd.SetContext(ctx)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute with store: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != tc.toolName {
				t.Fatalf("want 1 %s call, got %+v", tc.toolName, caller.calls)
			}
			_, emitted, err := output.EmitStoredResult(cmd)
			if err != nil || !emitted {
				t.Fatalf("EmitStoredResult emitted=%t err=%v out=%s", emitted, err, out.String())
			}
		})
	}
}
