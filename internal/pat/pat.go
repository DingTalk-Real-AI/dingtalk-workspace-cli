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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// RegisterCommands adds the pat command tree to rootCmd.
func RegisterCommands(root *cobra.Command, c edition.ToolCaller) {
	// Product-level Agent routing Decl (migrated from selection/pat.json
	// products.pat). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "pat",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("PAT 行为授权指南", "dingtalk-misc", "references/pat.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理 Agent 的 PAT 行为授权与本地浏览器策略",
			UseWhen: []string{
				"缺少行为授权需要预览/授予 scope，或配置授权是否打开浏览器",
			},
			AvoidWhen: []string{
				"普通登录用 auth；不要与开放平台应用权限或具体业务写操作混用",
			},
		},
	})
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

服务端 PAT / 路由标签 claw-type：
  开源构建固定在出站 MCP 请求中注入 claw-type: openClaw，
  hostControl.clawType 会回填相同值。DWS_AGENT_PRODUCT 不会修改它。

Agent 产品标识 DWS_AGENT_PRODUCT：
  合法非空值作为 x-dws-agent-product 请求头发送，供下游日志 / BI 使用；
  本客户端不使用该值派生或改变 PAT、鉴权或路由，下游使用契约由对应
  服务自行定义。同时该值作为启用 --ai-tag 时 IM 消息小尾巴的 clawType
  参数。未设置或为空时请求头省略，小尾巴回退发行版默认值。

DINGTALK_AGENT（可选，仅供 x-dingtalk-agent 使用）：
  如设置，将原样注入 HTTP 请求头 x-dingtalk-agent，
  便于上游按业务 Agent 名称区分流量。
  它不参与 claw-type 派生，也不参与 host-owned PAT 判定。

DWS_CHANNEL 只用于上游 channelCode。`,
	}
	corecmd.ApplyGroupPolicy(patCmd, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})

	patCmd.AddCommand(newChmodCommand(c))
	patCmd.AddCommand(newBrowserPolicyCommand())
	// Shortcuts are mounted before this open-source PAT tree in the app. Fold
	// that pre-existing service group into the native PAT parent so Cobra and
	// Schema both see exactly one top-level path while preserving PAT's group
	// behavior and adding every +leaf alongside chmod/browser-policy.
	for _, existing := range root.Commands() {
		if existing.Name() != patCmd.Name() {
			continue
		}
		cobracmd.MergeCommandTree(patCmd, existing)
		root.RemoveCommand(existing)
		break
	}
	root.AddCommand(patCmd)
}
