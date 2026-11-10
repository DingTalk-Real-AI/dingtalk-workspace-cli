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
	Description: "按类型读取结构化图表模板及服务端提供的原文说明", Risk: shortcut.RiskRead,
	Intent:        "需要指定大写 chartType 的配置模板和说明时使用；默认确认能力后优先服务端按类型读取，documentation 保留公共说明和所选类型 JSONC。明确缺能力时全量投影仅输出配置；documentation 为增强说明，缺失时依据随包指南与模板继续语义明确的配置，仅具体必需信息无法确认时停止；异常或不完整说明整体丢弃，保留合法 templates，并向 stderr 写入非阻断诊断，不补发全量请求。发现失败停止；--server-filter=false 可强制旧路径。说明不得提交为 config，示例 ID 须绑定真实资源。",
	Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	OutputRollout: output.RolloutUnifiedActive,
	Contract: aitableCompositeContractWithResult("+chart-templates", "按类型获取结构化模板及服务端原文说明；缺能力时客户端投影仅保留配置", "已知大写 chartType，需要仅获取这些类型的配置模板及可用说明时", "已有合法完整配置直接创建；documentation 为增强说明，缺失时依据随包指南与模板继续语义明确的配置，仅具体必需信息无法确认时停止；异常或不完整说明整体丢弃，保留合法 templates，并向 stderr 写入非阻断诊断，不补发全量请求；说明和模板占位 ID 不得直接用于写入", `dws aitable +chart-templates --chart-types LINE,PIE`, &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(aitableprotocol.ChartTemplateDataSchema),
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
			result, diagnostic, err := aitableprotocol.ProjectChartTemplateResponse(body, rt.StrSlice("chart-types"))
			if err != nil {
				return apperrors.NewInternal(err.Error())
			}
			if diagnostic != "" {
				rt.Command().PrintErrln("警告: 图表模板说明已整体丢弃，已保留合法 templates:", diagnostic)
			}
			return rt.Output(result)
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
