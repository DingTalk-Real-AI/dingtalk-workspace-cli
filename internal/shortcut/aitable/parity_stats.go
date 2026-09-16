// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"strings"
)

var DataQuery = shortcut.Shortcut{
	Service: "aitable", Command: "+data-query", Product: serverMain,
	Description: "用 DWS JSON DSL 统一执行标量或分组聚合，不拉全表做本地统计",
	Intent:      "明确单表统计任务时使用；dsl.stats 必填，dsl.group 非空时走分组聚合。使用 DWS fieldId/statsType 协议，不直接接受 Lark dimensions/measures。",
	Risk:        shortcut.RiskRead, Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: aitableCompositeContractWithResult("+data-query", "用 DWS JSON DSL 统一执行标量或分组聚合", "需要对单表执行多个指标或分组统计时", "明细读取用 +record-query；多表 JOIN 或 SQL 用 aitable psql；Lark DSL 需显式转换", `dws aitable +data-query --base-id B --table-id T --dsl '{"stats":[{"fieldId":"fldAmount","statsType":"SUM"}]}'`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"baseId":{"type":"string","description":"目标 Base"},"tableId":{"type":"string","description":"目标数据表"},"mode":{"type":"string","description":"scalar 或 grouped"},"result":{"type":"object","description":"原生聚合数据，保留统计值精度与版本信息"}},"required":["baseId","tableId","mode","result"]}`)}),
	Flags:    []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}, {Name: "dsl", Type: shortcut.FlagString, Desc: "DWS DSL 对象：stats 为 1-20 项 fieldId/statsType；可选 group/filters/sort/dataVersion/keyword；不接受 limit 以免静默统计子集", Required: true}},
	Execute:  executeDataQuery,
}

func executeDataQuery(rt *shortcut.RuntimeContext) error {
	dsl, err := parseJSONObject("dsl", rt.Str("dsl"))
	if err != nil {
		return err
	}
	allowed := map[string]bool{"stats": true, "group": true, "filters": true, "sort": true, "dataVersion": true, "keyword": true}
	for k := range dsl {
		if !allowed[k] {
			return apperrors.NewValidation("不支持的 DSL 属性 " + k + "；仅接受明确的 DWS 聚合协议")
		}
	}
	stats, ok := dsl["stats"].([]any)
	if !ok || len(stats) < 1 || len(stats) > 20 {
		return apperrors.NewValidation("dsl.stats 必须包含 1-20 项")
	}
	seen := map[string]bool{}
	validTypes := map[string]bool{}
	for _, v := range strings.Fields("SUM AVG MAX MIN MEDIAN STANDARD_DEVIATION RANGE COUNT COUNT_COLUMN DISTINCT EXISTS UN_EXISTS EXIST_RATIO UN_EXIST_RATIO DISTINCT_RATIO CHECKED_RATIO UN_CHECKED_RATIO EARLIEST_DATE LATEST_DATE DATE_RANGE DATE_RANGE_MONTH CHECKED UN_CHECKED") {
		validTypes[v] = true
	}
	for i, raw := range stats {
		m, ok := raw.(map[string]any)
		if !ok {
			return apperrors.NewValidation("stats 子项必须是对象")
		}
		id, ok := m["fieldId"].(string)
		typ, tok := m["statsType"].(string)
		if !ok || strings.TrimSpace(id) == "" || seen[id] || !tok || !validTypes[typ] || len(m) != 2 {
			return apperrors.NewValidation(fmt.Sprintf("stats[%d] 需要唯一 fieldId 和大写 statsType", i))
		}
		seen[id] = true
	}
	params := map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id"), "stats": stats}
	tool, mode := "query_records_stats", "scalar"
	if raw, present := dsl["group"]; present {
		group, ok := raw.([]any)
		if !ok {
			return apperrors.NewValidation("dsl.group 必须是 JSON 数组")
		}
		if len(group) > 0 {
			for _, raw := range group {
				m, ok := raw.(map[string]any)
				if !ok || stringValue(m, "fieldId") == "" {
					return apperrors.NewValidation("分组项需要 fieldId")
				}
			}
			b, _ := json.Marshal(group)
			params["group"] = string(b)
			tool, mode = "query_stats", "grouped"
		}
	}
	if raw, present := dsl["filters"]; present {
		b, _ := json.Marshal(raw)
		f, e := parseRecordQueryFilters(string(b))
		if e != nil {
			return e
		}
		params["filters"] = f
	}
	if raw, present := dsl["sort"]; present {
		sort, ok := raw.([]any)
		if !ok {
			return apperrors.NewValidation("dsl.sort 必须是 JSON 数组")
		}
		for _, raw := range sort {
			m, ok := raw.(map[string]any)
			if !ok || stringValue(m, "fieldId") == "" {
				return apperrors.NewValidation("排序项需要 fieldId")
			}
			v := strings.ToUpper(stringValue(m, "direction"))
			if v != "ASC" && v != "DESC" {
				return apperrors.NewValidation("排序 direction 必须为 ASC/DESC")
			}
		}
		b, _ := json.Marshal(sort)
		key := "sort"
		if mode == "grouped" {
			key = "sortDsl"
		}
		params[key] = string(b)
	}
	for _, key := range []string{"keyword", "dataVersion"} {
		if raw, present := dsl[key]; present {
			v, ok := raw.(string)
			if !ok || strings.TrimSpace(v) == "" {
				return apperrors.NewValidation("dsl." + key + " 必须是非空字符串")
			}
			if key == "keyword" && mode == "grouped" {
				return apperrors.NewValidation("分组聚合不支持 keyword；请使用 filters")
			}
			params[key] = v
		}
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"dry_run": true, "executed": false, "tool": tool, "arguments": params})
	}
	result, err := rt.CallMCPData(serverMain, tool, params)
	if err != nil {
		return err
	}
	body := result
	for i := 0; i < 8; i++ {
		if _, ok := body["results"]; ok {
			break
		}
		v, ok := body["data"].(map[string]any)
		if !ok {
			break
		}
		body = v
	}
	if mode == "grouped" && explicitEmptyGroupedStats(result, body) {
		body = cloneAnyMap(body)
		body["results"] = []any{}
	}
	if err := validateParityStats(body, stats, mode); err != nil {
		return apperrors.NewAPI(err.Error(), apperrors.WithReason("aitable_stats_invalid_response"))
	}
	return rt.Output(map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id"), "mode": mode, "result": body})
}
func init() { shortcut.Register(withAITableParityAliases(DataQuery)) }

// Validate business values, not just a successful transport envelope.
func validateParityStats(body map[string]any, stats []any, mode string) error {
	rows, ok := body["results"].([]any)
	if !ok {
		return fmt.Errorf("统计响应缺少 results 数组")
	}
	expected := map[string]string{}
	for _, s := range stats {
		m := s.(map[string]any)
		expected[m["fieldId"].(string)] = m["statsType"].(string)
	}
	if mode == "grouped" {
		for _, r := range rows {
			m, ok := r.(map[string]any)
			if !ok {
				return fmt.Errorf("分组统计项不是对象")
			}
			keys, ok := m["groupKeys"].([]any)
			if !ok || len(keys) == 0 {
				return fmt.Errorf("分组统计缺少 groupKeys")
			}
			for _, k := range keys {
				g, ok := k.(map[string]any)
				if !ok || stringValue(g, "fieldId") == "" {
					return fmt.Errorf("分组键缺少字段 ID")
				}
				if _, ok = g["value"]; !ok {
					return fmt.Errorf("分组键缺少值")
				}
			}
			fields, ok := m["fieldStatsMap"].(map[string]any)
			if !ok || len(fields) != len(expected) {
				return fmt.Errorf("分组统计未返回所有请求指标")
			}
			for id, typ := range expected {
				v, ok := fields[id].(map[string]any)
				if !ok || v["action"] != typ {
					return fmt.Errorf("分组统计指标不匹配")
				}
				if _, ok = v["value"]; !ok {
					return fmt.Errorf("分组统计缺少值")
				}
			}
		}
		return nil
	}
	seen := map[string]bool{}
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			return fmt.Errorf("标量统计项不是对象")
		}
		list, ok := m["results"].([]any)
		if !ok {
			return fmt.Errorf("标量统计缺少业务 results")
		}
		for _, v := range list {
			stat, ok := v.(map[string]any)
			id := stringValue(stat, "fieldId")
			if !ok || id == "" || seen[id] || expected[id] == "" || stat["statsType"] != expected[id] {
				return fmt.Errorf("标量统计指标缺失、重复或与请求不符")
			}
			if _, ok = stat["value"]; !ok {
				return fmt.Errorf("标量统计缺少值")
			}
			seen[id] = true
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("标量统计未返回全部请求指标")
	}
	return nil
}

// The reviewed zero-match response contains one empty group sentinel. Only
// that exact successful wire shape becomes an empty group list; missing keys,
// missing metrics on a real group, and metadata-only bodies remain errors.
func explicitEmptyGroupedStats(raw, body map[string]any) bool {
	// CallMCPData has already rejected transport and business errors. The
	// live read envelope uses summary/data/meta, without success/status flags.
	// Reject contradictory explicit flags, but do not require invented flags.
	if v, present := raw["success"]; present && v != true {
		return false
	}
	if v, present := raw["status"]; present && v != "success" {
		return false
	}
	version, ok := numericInt64(body["dataVersion"])
	if !ok || version < 0 {
		return false
	}
	rows, ok := body["results"].([]any)
	if !ok || len(rows) != 1 {
		return false
	}
	row, ok := rows[0].(map[string]any)
	if !ok || len(row) != 2 {
		return false
	}
	keys, kok := row["groupKeys"].([]any)
	fields, fok := row["fieldStatsMap"].(map[string]any)
	return kok && len(keys) == 0 && fok && len(fields) == 0
}
