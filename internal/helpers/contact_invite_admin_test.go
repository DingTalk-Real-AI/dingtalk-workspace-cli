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
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
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
