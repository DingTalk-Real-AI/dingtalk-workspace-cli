// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var ChartsCreate = shortcut.Shortcut{
	Service: "aitable", Command: "+charts-create", Product: serverMain, Description: "一次编排 1～20 张图表并按真实 ID 验证完整配置与显式布局", Risk: shortcut.RiskWrite,
	Intent: "同一 Dashboard 已准备 1～20 项完整 config/layout 时使用；默认在写前只读发现能力，优先 create_charts，明确缺能力才串行创建；发现失败停止。独立按真实 chartId 回读，不自动 arrange。--server-batch=false 强制串行；一次写提交不代表整批原子性。结果未知保留逐项回执和 ID，先只读核验，不重放创建。",
	Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}, OutputRollout: output.RolloutUnifiedActive,
	Contract: aitableCompositeContractWithResult("+charts-create", "创建整组图表并独立核验完整配置及显式布局；写前确认能力后优先服务端批量，缺能力才串行", "同一 Dashboard 已准备 1～20 项完整图表配置和显式布局，需要集中编排并保留逐项真实 ID 与核验状态时", "仅单图可用 +chart-create；能力发现失败时停止；结果未知不重放，先按原 chartId 只读核验；图表摘要不能代替完整配置", `dws aitable +charts-create --base-id B --dashboard-id D --charts '[{"config":{"name":"分析","chartType":"AI_ANALYZE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]'`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"图表编排与独立回读结果；异常时保留的同结构进度位于 error.details.result，不自动重建","properties":{"baseId":{"type":"string","description":"所属 Base"},"dashboardId":{"type":"string","description":"所属仪表盘"},"items":{"type":"array","description":"按输入顺序保留的逐图 ID、原始回执和验证状态；accepted 不等于 verified","items":{"type":"object","properties":{"index":{"type":"integer","description":"对应原输入的零基索引"},"chartId":{"type":"string","description":"真实创建回执返回的候选图表 ID；缺失时不能核实或重新创建"},"status":{"type":"string","description":"planned=未提交；accepted=受理但完整配置未核实；verified=独立完整回读匹配；unknown=写入或回执状态未知"},"receipt":{"type":"object","description":"该项原始提交回执，不代替独立完整回读","additionalProperties":true}},"required":["index","status"],"additionalProperties":true}},"verifiedCount":{"type":"integer","description":"独立完整配置与布局回读匹配的数量，不是受理数量"},"writeSubmissions":{"type":"integer","description":"远端写工具实际调用次数，不含前置或回读请求；一次提交不证明整批原子性"}},"required":["baseId","dashboardId","items","verifiedCount","writeSubmissions"]}`)}),
	Flags:    []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "dashboard-id", Type: shortcut.FlagString, Desc: "Dashboard ID", Required: true}, {Name: "charts", Type: shortcut.FlagString, Desc: "1～20 个完整 config/layout 的 JSON 数组；chartType 大写，布局显式声明", Required: true}, {Name: "server-batch", Type: shortcut.FlagBool, Desc: "默认发现能力后优先一次批建；false 强制串行，true 显式选择新接口；均不保证整批原子性"}, {Name: "is-app-mode", Type: shortcut.FlagBool, Desc: "已确认目标为应用模式时开启，用于核准根网格列数"}},
	Execute:  executeChartsCreate,
}

func parseChartBatch(raw string) ([]map[string]any, error) {
	var charts []map[string]any
	if err := json.Unmarshal([]byte(raw), &charts); err != nil {
		return nil, apperrors.NewValidation("charts 必须是 JSON 数组")
	}
	if len(charts) == 0 || len(charts) > 20 {
		return nil, apperrors.NewValidation("charts 必须包含 1～20 项")
	}
	for i, chart := range charts {
		cfg, ok := chart["config"].(map[string]any)
		kind := stringValue(cfg, "chartType")
		if !ok || kind == "" || kind != strings.ToUpper(kind) {
			return nil, apperrors.NewValidation(fmt.Sprintf("charts[%d].config.chartType 必须为大写类型", i))
		}
		layout, ok := chart["layout"].(map[string]any)
		if !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("charts[%d].layout 必须为完整对象", i))
		}
		if err := aitableprotocol.ValidateDashboardPersistentMetadata("config", cfg); err != nil {
			return nil, apperrors.NewValidation(err.Error())
		}
		if err := aitableprotocol.ValidateRootChartLayout(layout, 48); err != nil {
			return nil, apperrors.NewValidation(err.Error())
		}
	}
	return charts, nil
}

func verifyChartConfiguration(body, cfg, layout map[string]any) bool {
	widget, _ := body["widget"].(map[string]any)
	actual, _ := body["config"].(map[string]any)
	if actual == nil {
		actual, _ = widget["config"].(map[string]any)
	}
	for key, value := range cfg {
		if key == "name" {
			name := stringValue(body, "chartName", "name")
			if name == "" {
				name = stringValue(widget, "name")
			}
			if name == "" {
				name = stringValue(actual, "name")
			}
			if name != value {
				return false
			}
			continue
		}
		if !declaredValueMatches(actual[key], value) {
			return false
		}
	}
	position, _ := body["layout"].(map[string]any)
	return mapContains(position, layout)
}

func executeChartsCreate(rt *shortcut.RuntimeContext) error {
	charts, err := parseChartBatch(rt.Str("charts"))
	if err != nil {
		return err
	}
	b, d := rt.Str("base-id"), rt.Str("dashboard-id")
	items := []map[string]any{}
	result := map[string]any{"baseId": b, "dashboardId": d, "items": items, "verifiedCount": 0, "writeSubmissions": 0}
	if rt.DryRun() {
		for i := range charts {
			items = append(items, map[string]any{"index": i, "status": "planned"})
		}
		result["items"] = items
		return rt.Output(result)
	}
	useServer, err := preferServerChartTool(rt, "server-batch", "create_charts")
	if err != nil {
		return err
	}
	snapshot, err := rt.CallMCPData(serverMain, "get_dashboard", map[string]any{"baseId": b, "dashboardId": d})
	if err != nil {
		return err
	}
	columns, err := aitableprotocol.ResolveDashboardRootColumns(snapshot, b, d, rt.Bool("is-app-mode"))
	if err != nil {
		return err
	}
	for _, chart := range charts {
		if err = aitableprotocol.ValidateRootChartLayout(chart["layout"].(map[string]any), columns); err != nil {
			return apperrors.NewValidation(err.Error())
		}
	}
	for i := range charts {
		items = append(items, map[string]any{"index": i, "status": "planned"})
	}
	result["items"] = items
	if useServer {
		result["writeSubmissions"] = 1
		params := map[string]any{"baseId": b, "dashboardId": d, "charts": charts}
		params["isAppMode"] = rt.Bool("is-app-mode")
		raw, callErr := rt.CallMCPWriteDataStrict(serverMain, "create_charts", params)
		// The transport classifier keeps only safe candidates in a typed error.
		// Never replay the batch or discard IDs merely because status=error.
		var typed *apperrors.Error
		if callErr != nil && errors.As(callErr, &typed) {
			if typed.ExecutionStarted != nil && !*typed.ExecutionStarted {
				return callErr
			}
			if candidates, ok := typed.Details["result"].(map[string]any); ok {
				if candidates["baseId"] == b && candidates["dashboardId"] == d {
					raw = candidates
				}
			}
		}
		// Once dispatch may have started, missing/malformed receipts cannot leave
		// any item looking unsubmitted. Valid per-item evidence below can refine it.
		for _, item := range items {
			item["status"] = "unknown"
		}
		result["receipt"] = raw
		body := parityResponseObject(raw)
		if body["baseId"] != b || body["dashboardId"] != d {
			if callErr == nil {
				callErr = fmt.Errorf("批量回执目标不匹配；只读核验原 Dashboard，不重建")
			}
			return chartBatchError(result, callErr)
		}
		entries, ok := findNamedObjectList(raw, "results")
		if !ok || len(entries) != len(charts) {
			for _, item := range items {
				item["status"] = "unknown"
			}
			return chartBatchError(result, fmt.Errorf("批量回执缺少完整逐项结果；禁止重新创建"))
		}
		seen := map[int]bool{}
		invalidItems := false
		for _, entry := range entries {
			n, ok := entry["index"].(float64)
			index := int(n)
			if !ok || n != float64(index) || index < 0 || index >= len(charts) || seen[index] {
				return chartBatchError(result, fmt.Errorf("批量回执 index 不完整或重复"))
			}
			seen[index] = true
			// Preserve the provider receipt. Client status becomes verified only
			// after this invocation independently reads the complete chart.
			item := cloneAnyMap(entry)
			if callErr == nil && item["success"] == true && (item["status"] == "accepted" || item["status"] == "verified") {
				item["status"] = "accepted"
			} else {
				invalidItems = true
				if item["status"] != "failed" {
					item["status"] = "unknown"
				}
				item["success"] = false
			}
			items[index] = item
		}
		result["items"] = items
		if callErr != nil {
			return chartBatchError(result, callErr)
		}
		if invalidItems {
			return chartBatchError(result, fmt.Errorf("批量存在拒绝、缺少受理证据或投递未知项；保留候选 ID，只读核验，不重放创建"))
		}
	} else {
		for i, chart := range charts {
			result["writeSubmissions"] = i + 1
			params := map[string]any{"baseId": b, "dashboardId": d, "config": chart["config"], "layout": chart["layout"]}
			params["isAppMode"] = rt.Bool("is-app-mode")
			raw, callErr := rt.CallMCPWriteDataStrict(serverMain, "create_chart", params)
			items[i]["receipt"] = raw
			items[i]["chartId"] = findStringByKeys(raw, "chartId")
			items[i]["status"] = "accepted"
			if callErr != nil || items[i]["chartId"] == "" {
				var typed *apperrors.Error
				if errors.As(callErr, &typed) {
					if typed.ExecutionStarted != nil && !*typed.ExecutionStarted && i == 0 {
						// No earlier item was sent: keep the server's safe pre-write refusal.
						return callErr
					}
					if typed.Details["baseId"] == b && typed.Details["dashboardId"] == d {
						if id, ok := typed.Details["chartId"].(string); ok && id != "" {
							items[i]["chartId"] = id
						}
					}
				}
				items[i]["status"] = "unknown"
				if callErr == nil {
					callErr = fmt.Errorf("创建回执没有 chartId")
				}
				return chartBatchError(result, callErr)
			}
		}
	}
	// Separate inputs must resolve to separate created objects, even when their
	// configurations are identical. A duplicate ID cannot count twice as verified.
	seenIDs := map[string]bool{}
	for i, item := range items {
		id := stringValue(item, "chartId")
		if id == "" || seenIDs[id] {
			return chartBatchError(result, fmt.Errorf("图表 %d 的候选 ID 缺失或重复；保留回执，不重建", i))
		}
		seenIDs[id] = true
	}
	verified := 0
	for i, item := range items {
		id := stringValue(item, "chartId")
		raw, readErr := rt.CallMCPData(serverMain, "get_chart", map[string]any{"baseId": b, "dashboardId": d, "chartId": id})
		body := parityResponseObject(raw)
		if readErr != nil || body["baseId"] != b || stringValue(body, "chartId") != id || body["dashboardId"] != d || !verifyChartConfiguration(body, charts[i]["config"].(map[string]any), charts[i]["layout"].(map[string]any)) {
			item["status"] = "accepted"
			if readErr == nil {
				readErr = fmt.Errorf("图表 %s 的配置或布局未核实", id)
			}
			return chartBatchError(result, readErr)
		}
		item["status"] = "verified"
		verified++
		result["verifiedCount"] = verified
	}
	return rt.Output(result)
}

func chartBatchError(result map[string]any, cause error) error {
	return apperrors.NewAPI("整组图表未完整核实；保留逐项 ID，只读核验后继续", apperrors.WithCause(cause), apperrors.WithRetryable(false), apperrors.WithExecutionStarted(true), apperrors.WithDetails(map[string]any{"result": result}))
}
func init() { shortcut.Register(ChartsCreate) }
