// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var ChartTemplates = shortcut.Shortcut{
	Service: "aitable", Command: "+chart-templates", Product: serverMain,
	Description: "解析 JSONC 模板，只输出指定图表类型的结构化配置", Risk: shortcut.RiskRead,
	Intent:        "需要指定大写 chartType 的配置模板时使用；默认只读确认能力后优先服务端按类型读取；明确缺能力时读取全量 JSONC 并在 DWS 内投影。发现失败停止；--server-filter=false 可强制旧路径。模板中的示例 ID 必须绑定真实表和字段后再创建。",
	Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	OutputRollout: output.RolloutUnifiedActive,
	Contract: aitableCompositeContractWithResult("+chart-templates", "按类型获取结构化模板；默认发现能力后优先服务端筛选，缺能力时客户端投影", "缺少目标图表配置结构，且已知大写 chartType，需要仅获取这些类型的结构化模板时", "已有合法完整配置直接创建；发现失败先排查；模板示例 ID 不能直接用于写入", `dws aitable +chart-templates --chart-types LINE,PIE`, &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{"type":"object","description":"按请求类型投影的结构化图表模板；不代表图表已经创建或示例 ID 已绑定","properties":{"templates":{"type":"object","description":"按请求大写 chartType 索引的配置模板，创建前仍须绑定真实表和字段 ID","additionalProperties":true}},"required":["templates"]}`),
	}),
	Flags: []shortcut.Flag{{Name: "chart-types", Type: shortcut.FlagStringSlice, Desc: "需要的图表类型，使用大写 chartType，多个用逗号分隔", Required: true}, {Name: "server-filter", Type: shortcut.FlagBool, Desc: "默认发现能力后优先按类型读取；false 强制全量 JSONC 客户端投影，true 显式选择新接口"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		useServer, err := preferServerChartTool(rt, "server-filter", "get_dashboard_widget_templates")
		if err != nil {
			return err
		}
		if useServer {
			raw, err := rt.CallMCPData(serverMain, "get_dashboard_widget_templates", map[string]any{"chartTypes": rt.StrSlice("chart-types")})
			if err != nil {
				return err
			}
			body := parityResponseObject(raw)
			templates, err := aitableprotocol.ProjectStructuredChartTemplates(body, rt.StrSlice("chart-types"))
			if err != nil {
				return err
			}
			return rt.Output(map[string]any{"templates": templates})
		}
		raw, err := rt.CallMCPData(serverMain, "get_dashboard_widgets_example", map[string]any{})
		if err != nil {
			return err
		}
		value, ok := raw["data"].(string)
		if !ok {
			return fmt.Errorf("图表模板返回缺少 JSONC data 文本")
		}
		templates, err := aitableprotocol.ProjectChartTemplates(value, rt.StrSlice("chart-types"))
		if err != nil {
			return apperrors.NewValidation(err.Error())
		}
		return rt.Output(map[string]any{"templates": templates})
	},
}

func init() { shortcut.Register(ChartTemplates) }
