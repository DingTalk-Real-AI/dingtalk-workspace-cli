// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var DashboardList = shortcut.Shortcut{Service: "aitable", Command: "+dashboard-list", Product: serverMain, Description: "从完整 Base 目录读取仪表盘列表并验证每项 ID", Intent: "要发现 Base 中全部 dashboardId 时使用；原始 Base 目录使用 +base-get。", Risk: shortcut.RiskRead, Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, Contract: aitableCompositeContractWithResult("+dashboard-list", "列出 Base 中全部仪表盘", "发现 Base 内仪表盘 ID 时", "已知 dashboardId 用 +dashboard-get", `dws aitable +dashboard-list --base-id B`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"baseId":{"type":"string","description":"所属 Base"},"count":{"type":"integer","description":"仪表盘数量"},"dashboards":{"type":"array","description":"服务端目录中的仪表盘","items":{"type":"object","properties":{"dashboardId":{"type":"string","description":"稳定 ID"}},"required":["dashboardId"]}}},"required":["baseId","count","dashboards"]}`)}), Flags: []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}}, Execute: func(rt *shortcut.RuntimeContext) error {
	r, err := rt.CallMCPData(serverMain, "get_base", map[string]any{"baseId": rt.Str("base-id")})
	if err != nil {
		return err
	}
	body := parityResponseObject(r)
	if body["baseId"] != rt.Str("base-id") {
		return fmt.Errorf("get_base identity mismatch")
	}
	list, ok := body["dashboards"].([]any)
	if !ok {
		return fmt.Errorf("get_base lacks dashboards collection")
	}
	seen := map[string]bool{}
	for _, v := range list {
		m, ok := v.(map[string]any)
		id := stringValue(m, "dashboardId")
		if !ok || id == "" || seen[id] {
			return fmt.Errorf("dashboard collection contains invalid identity")
		}
		seen[id] = true
	}
	return rt.Output(map[string]any{"baseId": rt.Str("base-id"), "count": len(list), "dashboards": list})
}}

func dashboardCreateShortcut(chart bool) shortcut.Shortcut {
	command, desc, example := "+dashboard-create", "创建仪表盘并独立核对新 ID 与名称", `dws aitable +dashboard-create --base-id B --name 经营看板`
	flags := []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "name", Type: shortcut.FlagString, Desc: "仪表盘名称", Required: true}, {Name: "config", Type: shortcut.FlagString, Desc: "可选完整 Dashboard config；name 参数覆盖同名属性"}}
	if chart {
		command, desc, example = "+chart-create", "创建图表并独立核对配置和布局", `dws aitable +chart-create --base-id B --dashboard-id D --config '{"name":"分析","chartType":"AI_ANALYZE"}' --layout '{"x":0,"y":0,"w":6,"h":4}'`
		flags = []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true}, {Name: "config", Type: shortcut.FlagString, Desc: "含 chartType 的完整图表配置", Required: true}, {Name: "layout", Type: shortcut.FlagString, Desc: "根布局 JSON，自动从真实 Dashboard 选择 12/48 列", Required: true}, {Name: "is-app-mode", Type: shortcut.FlagBool, Desc: "已知应用模式时按 48 列校验，只读上下文"}}
	}
	return shortcut.Shortcut{Service: "aitable", Command: command, Product: serverMain, Description: desc, Intent: desc + "；不把组件配置当作计算数据。", Risk: shortcut.RiskWrite, Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}, Contract: aitableCompositeContractWithResult(command, desc, desc+"时", "需要图表计算数据时不能用配置返回代替；非根容器布局需正式协议", example, parityCompositeResultSpec()), Flags: flags, Execute: func(rt *shortcut.RuntimeContext) error { return executeDashboardCreate(rt, chart) }}
}
func executeDashboardCreate(rt *shortcut.RuntimeContext, chart bool) error {
	cfg := map[string]any{}
	var err error
	if rt.Changed("config") {
		cfg, err = parseJSONObject("config", rt.Str("config"))
		if err != nil {
			return err
		}
	}
	tool, idKey, readTool := "create_dashboard", "dashboardId", "get_dashboard"
	args := map[string]any{"baseId": rt.Str("base-id"), "config": cfg}
	if chart {
		tool, idKey, readTool = "create_chart", "chartId", "get_chart"
		args["dashboardId"] = rt.Str("dashboard-id")
		if stringValue(cfg, "chartType") == "" {
			return apperrors.NewValidation("config.chartType 必填")
		}
		layout, e := parseJSONObject("layout", rt.Str("layout"))
		if e != nil {
			return e
		}
		if e = aitableprotocol.ValidateDashboardPersistentMetadata("layout", layout); e != nil {
			return e
		}
		if e = aitableprotocol.ValidateRootChartLayout(layout, 48); e != nil {
			return e
		}
		args["layout"] = layout
	} else {
		cfg["name"] = rt.Str("name")
		if rt.Str("name") == "" {
			return apperrors.NewValidation("--name 不能为空")
		}
	}
	if err = aitableprotocol.ValidateDashboardPersistentMetadata("config", cfg); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	result := newCompositeResult(tool)
	result.Resolved = map[string]any{"baseId": rt.Str("base-id")}
	result.Plan = []compositeStep{{Index: 1, Name: tool, Tool: tool, Status: "planned", Arguments: args}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	if chart {
		snap, e := rt.CallMCPData(serverMain, "get_dashboard", map[string]any{"baseId": args["baseId"], "dashboardId": args["dashboardId"]})
		if e != nil {
			return e
		}
		columns, e := aitableprotocol.ResolveDashboardRootColumns(snap, rt.Str("base-id"), rt.Str("dashboard-id"), rt.Bool("is-app-mode"))
		if e != nil {
			return e
		}
		if e = aitableprotocol.ValidateRootChartLayout(args["layout"].(map[string]any), columns); e != nil {
			return apperrors.NewValidation(e.Error())
		}
	}
	raw, err := rt.CallMCPWriteDataStrict(serverMain, tool, args)
	id := findStringByKeys(raw, idKey)
	if id != "" {
		result.Resolved[idKey] = id
		result.KnownEffects = []map[string]any{{"tool": tool, idKey: id}}
	}
	if err != nil {
		return compositeError(result, err, false)
	}
	if id == "" {
		return compositeError(result, fmt.Errorf("creation response lacks %s", idKey), false)
	}
	readArgs := map[string]any{"baseId": args["baseId"], idKey: id}
	if chart {
		readArgs["dashboardId"] = args["dashboardId"]
	}
	read, err := rt.CallMCPData(serverMain, readTool, readArgs)
	if err != nil {
		return compositeError(result, err, false)
	}
	body := parityResponseObject(read)
	if stringValue(body, idKey) != id {
		return compositeError(result, fmt.Errorf("creation readback ID mismatch"), false)
	}
	if chart {
		if body["dashboardId"] != args["dashboardId"] {
			return compositeError(result, fmt.Errorf("chart parent mismatch"), false)
		}
		widget, _ := body["widget"].(map[string]any)
		actual, _ := widget["config"].(map[string]any)
		if actual == nil {
			actual, _ = body["config"].(map[string]any)
		}
		layout, _ := body["layout"].(map[string]any)
		if !mapContains(actual, cfg) || !mapContains(layout, args["layout"].(map[string]any)) {
			return compositeError(result, fmt.Errorf("chart configuration/layout readback mismatch"), false)
		}
	} else {
		if stringValue(body, "dashboardName", "name") != rt.Str("name") {
			return compositeError(result, fmt.Errorf("dashboard name readback mismatch"), false)
		}
		for key, value := range cfg {
			if key == "name" {
				continue
			}
			if !declaredValueMatches(body[key], value) {
				return compositeError(result, fmt.Errorf("dashboard %s readback mismatch", key), false)
			}
		}
	}
	result.CompletedCount = 1
	result.Result = body
	result.Verification = map[string]any{"status": "verified", idKey: id}
	return rt.Output(result)
}
func init() {
	shortcut.Register(withAITableParityAliases(DashboardList), withAITableParityAliases(dashboardCreateShortcut(false)), withAITableParityAliases(dashboardCreateShortcut(true)))
}
