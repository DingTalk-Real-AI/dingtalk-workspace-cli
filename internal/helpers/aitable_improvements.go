// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// These leaves expose provider receipts. The corresponding Shortcuts perform
// independent readback; native success means the query or submission returned.
func newAitableImprovementLeaves(field, dashboard, chart *cobra.Command) {
	base := aitableAppBaseIDFlag()
	dash := LeafFlag{Name: "dashboard-id", Bind: "dashboardId", Required: true, Trim: true, Usage: "目标仪表盘 ID"}
	appMode := LeafFlag{Name: "is-app-mode", Bind: "isAppMode", Kind: corecmd.KindBool, Usage: "已确认的应用模式只读上下文"}
	chart.AddCommand(NewLeafCommand(LeafSpec{
		Use: "templates", Short: "按类型读取图表模板", Long: "按类型读取结构化图表模板；仅在新 MCP 入口已部署时使用，示例 ID 必须绑定真实资源。", Tool: "get_dashboard_widget_templates", Server: "aitable", Safety: aitableSafetyRead(),
		Flags: []LeafFlag{{Name: "chart-types", Bind: "chartTypes", Required: true, Trim: true, Usage: "1～20 个不同的大写图表类型，逗号分隔", Transform: func(raw string) (any, error) {
			values := strings.Split(raw, ",")
			seen := map[string]bool{}
			if len(values) > 20 {
				return nil, apperrors.NewValidation("chart-types 上限为 20")
			}
			for i, value := range values {
				value = strings.TrimSpace(value)
				if value == "" || value != strings.ToUpper(value) || seen[value] {
					return nil, apperrors.NewValidation("chart-types 必须为不同的非空大写类型")
				}
				seen[value], values[i] = true, value
			}
			return values, nil
		}}}, OutputRollout: output.RolloutUnifiedActive, ResultCall: callAitableImprovementResult,
		Contract: aitableImprovementContract("chart_templates", "aitable chart templates", "get_dashboard_widget_templates", "aitable", "按类型读取结构化图表模板；仅在新 MCP 入口已部署时使用，示例 ID 必须绑定真实资源。", "仅需指定类型的结构化模板且已确认新入口部署时", "旧 MCP 未部署此入口时用 +chart-templates 默认兼容路径；已有配置不必重复读模板", `dws aitable chart templates --chart-types LINE,PIE`, []contract.ParamDecl{aitableAppParam("chart-types", "chartTypes", "array", true)}, `{"type":"object","description":"指定类型的完整配置模板；内部示例 ID 不代表用户资源","properties":{"templates":{"type":"object","description":"按 chartType 键控的完整配置对象","additionalProperties":{"type":"object","additionalProperties":true}}},"required":["templates"]}`),
	}))
	chart.AddCommand(NewLeafCommand(LeafSpec{
		Use: "batch-create", Short: "批量提交图表，保留逐项状态", Long: "在同一仪表盘提交 1～20 张图表。保留逐项 chartId 和 failed/accepted/verified/unknown；accepted 不等于持久化，未知项先只读核验，禁止直接重放。入口须已部署；不自动重排。", Tool: "create_charts", Server: "aitable", Safety: aitableImprovementCreateSafety(),
		Flags: []LeafFlag{base, dash, appMode, aitableImprovementArrayFlag("charts", "charts", "1～20 项 config/layout 数组", 20, func(items []map[string]any) error {
			for i, item := range items {
				config, _ := item["config"].(map[string]any)
				kind, _ := config["chartType"].(string)
				layout, _ := item["layout"].(map[string]any)
				if kind == "" || kind != strings.ToUpper(kind) {
					return fmt.Errorf("charts[%d].config.chartType 必须为大写类型", i)
				}
				if err := aitableprotocol.ValidateDashboardPersistentMetadata("config", config); err != nil {
					return err
				}
				if err := aitableprotocol.ValidateDashboardPersistentMetadata("layout", layout); err != nil {
					return err
				}
				if err := aitableprotocol.ValidateRootChartLayout(layout, 48); err != nil {
					return fmt.Errorf("charts[%d]: %w", i, err)
				}
			}
			return nil
		})}, OutputRollout: output.RolloutUnifiedActive, ResultCall: callAitableImprovementResult,
		Contract: aitableImprovementContract("chart_batch_create", "aitable chart batch-create", "create_charts", "aitable", "在同一仪表盘提交 1～20 张图表。保留逐项 chartId 和 failed/accepted/verified/unknown；accepted 不等于持久化，未知项先只读核验，禁止直接重放。入口须已部署；不自动重排。", "新批量 MCP 已部署，需要一次提交同一仪表盘的完整图表配置和显式布局时", "需要 DWS 统一独立回读时用 +charts-create；缺能力时自动用串行兼容路径", `dws aitable chart batch-create --base-id B --dashboard-id D --charts '[{"config":{"chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]'`, []contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true), aitableAppParam("dashboard-id", "dashboardId", "string", true), aitableAppParam("charts", "charts", "array", true), aitableAppParam("is-app-mode", "isAppMode", "boolean", false)}, `{"type":"object","description":"批量图表逐项提交与独立核验回执；不保证整批原子性","properties":{"baseId":{"type":"string","description":"所属 Base"},"dashboardId":{"type":"string","description":"目标仪表盘"},"totalRequested":{"type":"integer","description":"请求数量"},"totalAccepted":{"type":"integer","description":"已受理数量"},"totalCreated":{"type":"integer","description":"旧协议的提交受理数量，不代表完整回读"},"totalVerified":{"type":"integer","description":"独立完整回读验证数量"},"results":{"type":"array","description":"顺序对应请求的逐项状态和候选 chartId；unknown 不得重放","items":{"type":"object","additionalProperties":true}}},"required":["baseId","dashboardId","totalRequested","totalCreated","totalAccepted","totalVerified","results"]}`),
	}))
	dashboard.AddCommand(NewLeafCommand(LeafSpec{
		Use: "preview", Short: "只读预览按行重排的真实差异", Long: "读取当前真实布局并用与按行重排同源的算法计算逐卡前后差异。此命令只读；fingerprint 只用于提交前变化检查，不是原子 CAS，也不证明页面可读。入口须已部署。", Tool: "preview_dashboard_layout", Server: "aitable-helper", Safety: aitableSafetyRead(),
		Flags: []LeafFlag{base, dash, appMode}, OutputRollout: output.RolloutUnifiedActive, ResultCall: callAitableImprovementResult,
		Contract: aitableImprovementContract("dashboard_preview", "aitable dashboard preview", "preview_dashboard_layout", "aitable-helper", "读取当前真实布局并用与按行重排同源的算法计算逐卡前后差异。此命令只读；fingerprint 只用于提交前变化检查，不是原子 CAS，也不证明页面可读。入口须已部署。", "明确考虑统一按行重排且需先检查实际逐卡变化时", "已有明确混排或叠放设计时保留显式布局，不把预览当创建后的默认收尾", `dws aitable dashboard preview --base-id B --dashboard-id D`, []contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true), aitableAppParam("dashboard-id", "dashboardId", "string", true), aitableAppParam("is-app-mode", "isAppMode", "boolean", false)}, `{"type":"object","properties":{"baseId":{"type":"string","description":"所属 Base"},"dashboardId":{"type":"string","description":"目标仪表盘"},"layoutFingerprint":{"type":"string","description":"真实源布局摘要"},"beforeLayout":{"type":"array","description":"真实逐卡旧布局","items":{"type":"object","additionalProperties":true}},"layout":{"type":"array","description":"同源算法逐卡新布局","items":{"type":"object","additionalProperties":true}},"preview":{"type":"boolean","const":true,"description":"只读预览标记"}},"required":["baseId","dashboardId","layoutFingerprint","beforeLayout","layout","preview"]}`),
	}))
	field.AddCommand(NewLeafCommand(LeafSpec{
		Use: "task-status", Short: "只读查询本轮 AI 固定目标状态", Long: "按本轮真实 taskId、字段和固定记录集合查询已有行状态，不重新触发 AI。按提供方确认的能力读取本轮状态：successTerminalVerifiable=false 或省略时不推断成功历史能力，旧窗口查询的缺失行保持 unknown；确认历史能力后可返回固定 taskId 终态及写回版本。resultVersionAvailable 不等于结果可读，当前 resultReadableVerified 仍为 false。入口须已部署。", Tool: "get_ai_field_task_status", Server: "aitable", Safety: aitableSafetyRead(),
		Flags: []LeafFlag{base, {Name: "table-id", Bind: "tableId", Required: true, Trim: true, Usage: "任务所属数据表 ID"}, aitableImprovementArrayFlag("tasks", "tasks", "1～10 项 taskId/fieldId/recordIds，字段不同，每项 1～500 个记录 ID", 10, validateAitableTaskTargets)}, OutputRollout: output.RolloutUnifiedActive, ResultCall: callAitableImprovementResult,
		Contract: aitableImprovementContract("field_task_status", "aitable field task-status", "get_ai_field_task_status", "aitable", "按本轮真实 taskId、字段和固定记录集合查询已有行状态，不重新触发 AI。按提供方确认的能力读取本轮状态：successTerminalVerifiable=false 或省略时不推断成功历史能力，旧窗口查询的缺失行保持 unknown；确认历史能力后可返回固定 taskId 终态及写回版本。resultVersionAvailable 不等于结果可读，当前 resultReadableVerified 仍为 false。入口须已部署。", "已有真实 taskId 和固定 recordIds，需要一次只读查询本轮状态时", "需要有总预算的自动退避等待用 +field-wait-ai；缺少 taskId 或目标不固定时不能凭单元格非空证明本轮完成", `dws aitable field task-status --base-id B --table-id T --tasks '[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]'`, []contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true), aitableAppParam("table-id", "tableId", "string", true), aitableAppParam("tasks", "tasks", "array", true)}, `{"type":"object","description":"本轮任务的精确行状态，查询成功不表示结果可读","properties":{"baseId":{"type":"string","description":"所属 Base"},"tableId":{"type":"string","description":"所属数据表"},"tasks":{"type":"array","description":"精确绑定 taskId/fieldId/recordIds 的逐行状态","items":{"type":"object","additionalProperties":true}},"queryComplete":{"type":"boolean","description":"本次目标查询是否完整，不代表任务完成"},"queryRetryable":{"type":"boolean","description":"查询不完整时允许按原 taskId 只读续查，不能重新运行 AI"},"queryBudgetExhausted":{"type":"boolean","description":"服务端状态查询预算是否耗尽"},"terminalVerified":{"type":"boolean","description":"实际返回的完整固定目标是否具有终态证据；缺失行不能反推成功"},"successTerminalVerifiable":{"type":"boolean","description":"提供方是否能够验证成功终态；旧窗口提供方为 false，旧服务可能省略，缺失不代表支持历史查询"},"resultReadableVerified":{"type":"boolean","const":false,"description":"兼容接口未提供本轮结果可读证据"},"resultVersionAvailable":{"type":"boolean","description":"是否返回本轮行的写回版本证据；有版本不等于 Record/WAL 或结果可读"}},"required":["baseId","tableId","tasks","terminalVerified","resultReadableVerified","resultVersionAvailable"]}`),
	}))
}

func aitableImprovementCreateSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}
}

func aitableImprovementContract(name, path, tool, server, description, positive, negative, example string, params []contract.ParamDecl, dataSchema string) LeafContract {
	declaration := aitableAppContract(name, path, tool, description, description, []string{positive}, []string{negative}, []string{example}, params,
		&contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(dataSchema)})
	if server == "aitable-helper" {
		declaration.Interface = aitableHelperMCPInterface(tool)
	}
	return declaration
}

func callAitableImprovementResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if preview, ok := aitableUnifiedDryRunResult(tool, args); ok {
		return preview, nil
	}
	server := "aitable"
	if tool == "preview_dashboard_layout" {
		server = "aitable-helper"
	}
	data, err := callAitableUnifiedDataOnServerContext(cmd.Context(), server, tool, args)
	if err != nil {
		return nil, err
	}
	if tool == "get_dashboard_widget_templates" {
		types, _ := args["chartTypes"].([]string)
		templates, err := aitableprotocol.ProjectStructuredChartTemplates(data, types)
		if err != nil {
			return nil, apperrors.NewInternal(err.Error())
		}
		return output.Success(map[string]any{"templates": templates}), nil
	}
	return output.Success(data), nil
}

func aitableImprovementArrayFlag(name, bind, usage string, maximum int, validate func([]map[string]any) error) LeafFlag {
	return LeafFlag{Name: name, Bind: bind, Usage: usage, Required: true, Trim: true, Transform: func(raw string) (any, error) {
		var items []map[string]any
		if err := json.Unmarshal([]byte(raw), &items); err != nil || len(items) == 0 || len(items) > maximum {
			return nil, apperrors.NewValidation(fmt.Sprintf("--%s 必须为 1～%d 项对象数组", name, maximum))
		}
		for _, item := range items {
			if item == nil {
				return nil, apperrors.NewValidation("数组项不能为 null")
			}
		}
		if err := validate(items); err != nil {
			return nil, apperrors.NewValidation(err.Error())
		}
		return items, nil
	}}
}

func validateAitableTaskTargets(items []map[string]any) error {
	fields := map[string]bool{}
	for _, item := range items {
		task, _ := item["taskId"].(string)
		field, _ := item["fieldId"].(string)
		ids, _ := item["recordIds"].([]any)
		if strings.TrimSpace(task) == "" || strings.TrimSpace(field) == "" || fields[field] || len(ids) == 0 || len(ids) > 500 {
			return fmt.Errorf("taskId/fieldId 必须非空、字段不同，recordIds 为 1～500 个不同 ID")
		}
		fields[field] = true
		seen := map[string]bool{}
		for _, raw := range ids {
			id, _ := raw.(string)
			if strings.TrimSpace(id) == "" || seen[id] {
				return fmt.Errorf("recordIds 必须为不同的非空字符串")
			}
			seen[id] = true
		}
	}
	return nil
}
