// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

const aitableBuiltinAIConfigDescription = `AI 配置 JSON，原样交给 MCP 校验和转换。通用 AI 使用 outputType/prompt；内置插件使用 type="builtin"、pluginId、formData、referFieldIds，可选 autoRecompute，不要求 prompt/outputType。用 field ai-plugins 查询真实插件 ID 和配置结构；verificationStatus=pending 时继续只读核对，不重复写入。`

func newAitableBuiltinAIPluginsCommand() *cobra.Command {
	declaration := aitableAppContract("field_ai_plugins", "aitable field ai-plugins", "list_builtin_ai_field_plugins", "只读发现当前数据表可配置的内置 AI 插件；返回真实 pluginId、适用字段类型、配置结构和模式限制；工作流凭据须通过现有 UI 绑定，不由 CLI 传递。", "发现内置 AI 字段插件与配置结构。", []string{"准备创建或更新内置 AI 字段，需要确认真实 pluginId、配置结构、可用字段类型或 UI 绑定限制时"}, []string{"运行已配置 AI 字段用 field run-ai；通用 agent 提示词配置无需选择内置插件"}, []string{"dws aitable field ai-plugins --base-id B --table-id T"}, []contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true), aitableAppParam("table-id", "tableId", "string", true)}, &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: aitableResultSchemaWithDryRun("内置 AI 插件目录或请求预览", json.RawMessage(`{"type":"object","properties":{"baseId":{"type":"string","description":"所属 Base ID"},"tableId":{"type":"string","description":"所属数据表 ID"},"plugins":{"type":"array","description":"目标环境实际发布的内置插件；不能从目录推断插件凭据已绑定","items":{"type":"object","properties":{"pluginId":{"type":"string","description":"真实内置插件 ID，用于 aiConfig.pluginId"},"name":{"type":"string","description":"插件显示名"},"fieldTypes":{"type":"array","description":"适用字段类型","items":{"type":"string"}},"configSchema":{"type":"object","description":"支持配置的插件才返回表单结构；unsupported 模式可省略，交给 MCP 校验","additionalProperties":true},"configurationMode":{"type":"string","description":"配置模式，包含直接配置或需要既有 UI 绑定等限制"},"reason":{"type":"string","description":"配置限制或需要 UI 绑定的原因"}},"required":["pluginId","name","fieldTypes","configurationMode"],"additionalProperties":true}}},"required":["baseId","tableId","plugins"],"additionalProperties":true}`)),
	})
	declaration.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false}
	return NewLeafCommand(LeafSpec{Use: "ai-plugins", Short: "发现内置 AI 字段插件与配置结构", Long: declaration.Description, Server: "aitable", Tool: "list_builtin_ai_field_plugins", Flags: []LeafFlag{aitableAppBaseIDFlag(), {Name: "table-id", Bind: "tableId", Required: true, Trim: true, Usage: "目标数据表 ID"}}, Safety: aitableSafetyRead(), OutputRollout: output.RolloutUnifiedActive, ResultCall: callAitableAdvpermResult, Contract: declaration})
}
