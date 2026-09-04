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
