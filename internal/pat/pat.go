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

// Package pat implements the "dws pat" command group for PAT (Personal Action
// Token) authorization management.
package pat

import (
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var caller edition.ToolCaller

// RegisterCommands adds the pat command tree to rootCmd.
func RegisterCommands(root *cobra.Command, c edition.ToolCaller) {
	caller = c
	patCmd := &cobra.Command{
		Use:   "pat",
		Short: "行为授权管理",
		Long: `管理行为授权（PAT）。

命令结构:
  dws pat chmod     <scope>...        授予指定权限
  dws pat apply     <scope>...        主动申请 scope（orchestrator）
  dws pat status    [<authRequestId>] 查询异步 PAT 流程状态
  dws pat scopes                      列出当前已授权的 scope

Host-owned PAT 开关（SSOT §1 + §2）：
  当且仅当环境变量 DINGTALK_DWS_AGENTCODE 非空时，CLI 命中 PAT
  固定以 stderr JSON + exit=4 的 host-owned 形式返回，
  由宿主处理全部 UI / 交互 / 回调节奏 / 重试逻辑，
  CLI 侧不再拉起任何本地浏览器 / 轮询。

服务端路由标签 claw-type（开源构建硬编码）：
  开源构建在所有出站 MCP 请求上恒定注入 claw-type: openClaw，
  与 DINGTALK_AGENT / 宿主环境解耦，与历史 main 行为一致。
  hostControl.clawType 也会回填该值，便于宿主侧审计/路由。

DINGTALK_AGENT（可选，仅供 x-dingtalk-agent 使用）：
  如设置，将原样注入 HTTP 请求头 x-dingtalk-agent，
  便于上游按业务 Agent 名称区分流量。
  它不参与 claw-type 派生，也不参与 host-owned PAT 判定。

DWS_CHANNEL 只用于上游 channelCode。`,
		RunE: cmdutil.GroupRunE,
	}

	patCmd.AddCommand(chmodCmd)
	patCmd.AddCommand(applyCmd)
	patCmd.AddCommand(statusCmd)
	patCmd.AddCommand(scopesCmd)
	root.AddCommand(patCmd)
}
