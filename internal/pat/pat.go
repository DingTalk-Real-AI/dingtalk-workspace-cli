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

// RegisterCommands adds the pat command tree to rootCmd.
func RegisterCommands(root *cobra.Command, c edition.ToolCaller) {
	patCmd := &cobra.Command{
		Use:   "pat",
		Short: "行为授权管理",
		Long: `管理行为授权（PAT）。

命令结构:
  dws pat chmod <scope>...  授予指定权限
  dws pat browser-policy    配置 PAT 浏览器打开策略

能力说明：
  pat chmod 默认输出轻量授权摘要；显式 --format json / --verbose 时，
  才返回服务端完整 JSON（含逐 scope 明细），便于机器校验。
  pat chmod 支持批量授权：可一次传多个 scope，也可通过
  --products / --product、--domains / --domain 或 --recommend
  让服务端按产品模板 / 推荐集合计算授权计划，再批量授予选中的 scope。
  批量计划会返回 selected / skipped / pending 明细；--dry-run 只预览计划，
  不写入授权。真正执行批量授权前必须由用户显式添加 --yes；未加 --yes
  时 CLI 会阻断并提示 agent 先确认。
  浏览器是否打开由本地 PAT 策略单独决定，与 json / non-json 独立。
  pat chmod 可传 --agentCode，或设置 DINGTALK_DWS_AGENTCODE；
  CLI 会把显式 agentCode 放入 batch 请求参数，
  并同步注入 gateway 兼容身份头。未传 agentCode 时由服务端默认兜底。
  浏览器策略生效时会优先按 DINGTALK_DWS_AGENTCODE 读取 agent 策略，再回退到默认策略。
  写入 agent 策略需显式传 --agentCode；不传则写入全局默认策略。

Host-owned PAT 开关：
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

	patCmd.AddCommand(newChmodCommand(c))
	patCmd.AddCommand(newBrowserPolicyCommand())
	root.AddCommand(patCmd)
}
