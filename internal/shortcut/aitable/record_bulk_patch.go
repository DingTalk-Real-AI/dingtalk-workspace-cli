// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var RecordBulkPatch = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-bulk-patch",
	Product:     serverMain,
	Description: "完整查询目标记录后批量合并同一组 cells，自动分片并逐条读回验证",
	Intent:      "当你要按 filters/关键词/recordIds 对一批已有记录应用相同字段补丁时使用；必须显式选择范围或 --all，并受 max-matches 写前上限保护。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: aitableCompositeContract(
		"+record-bulk-patch",
		"完整查询目标记录后批量合并同一组 cells，自动分片并逐条读回验证",
		"当你要按 filters/关键词/recordIds 对一批已有记录应用相同字段补丁时使用；必须显式选择范围或 --all，并受 max-matches 写前上限保护。",
		"单条已知 recordId 用 record update；每条记录写不同值用 record update/upsert；未明确范围时不要执行",
		`dws aitable +record-bulk-patch --base-id B --table-id T --query "待处理" --patch '{"fldStatus":"完成"}'`,
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "patch", Type: shortcut.FlagString, Desc: "要合并到每条记录的非空 cells JSON 对象", Required: true},
		{Name: "filters", Type: shortcut.FlagString, Desc: "query_records filters JSON（选择条件之一）"},
		{Name: "query", Type: shortcut.FlagString, Desc: "全文关键词（选择条件之一）"},
		{Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "明确的 recordId 列表（选择条件之一）"},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "可选视图上下文"},
		{Name: "all", Type: shortcut.FlagBool, Desc: "明确允许匹配整张表"},
		{Name: "max-matches", Type: shortcut.FlagInt, Default: "1000", Desc: "写入前允许匹配的最大记录数，1-10000"},
	},
	Tips: []string{`dws aitable +record-bulk-patch --base-id B --table-id T --query "待处理" --patch '{"fldStatus":"完成"}'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeRecordBulkPatch(rt)
	},
}

func executeRecordBulkPatch(rt *shortcut.RuntimeContext) error {
	patch, err := parseJSONObject("patch", rt.Str("patch"))
	if err != nil {
		return err
	}
	if len(patch) == 0 {
		return apperrors.NewValidation("--patch 必须是非空 JSON 对象")
	}
	params := map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id")}
	selectorCount := 0
	if rt.Changed("filters") {
		filters, err := parseBulkPatchFilters(rt.Str("filters"))
		if err != nil {
			return err
		}
		params["filters"] = filters
		selectorCount++
	}
	if rt.Changed("query") {
		query := strings.TrimSpace(rt.Str("query"))
		if query == "" {
			return apperrors.NewValidation("--query 必须包含非空白关键词")
		}
		params["keyword"] = query
		selectorCount++
	}
	if rt.Changed("record-ids") {
		ids, err := parseRecordIDs(rt.StrSlice("record-ids"))
		if err != nil {
			return err
		}
		params["recordIds"] = ids
		selectorCount++
	}
	if rt.Changed("all") {
		if !rt.Bool("all") {
			return apperrors.NewValidation("--all=false 不构成全表授权")
		}
		selectorCount++
	}
	if selectorCount == 0 {
		return apperrors.NewValidation("必须显式提供 --filters、--query、--record-ids 或 --all")
	}
	maxMatches := rt.Int("max-matches")
	if maxMatches < 1 || maxMatches > maxCompositeRecordRun {
		return apperrors.NewValidation(fmt.Sprintf("--max-matches 必须在 1..%d", maxCompositeRecordRun))
	}
	if rt.Changed("view-id") {
		params["viewId"] = rt.Str("view-id")
	}
	records, err := queryAllRecords(rt, params, maxMatches)
	if err != nil {
		return err
	}
	updates := make([]map[string]any, 0, len(records))
	for _, record := range records {
		id := recordID(record)
		if id == "" {
			return fmt.Errorf("query_records returned a matched record without recordId")
		}
		updates = append(updates, map[string]any{"recordId": id, "cells": cloneAnyMap(patch)})
	}
	if len(updates) == 0 {
		result := newCompositeResult("record_bulk_patch")
		result.RequestedCount = 0
		result.Verification = map[string]any{"status": "verified_no_matches"}
		result.Result = map[string]any{"matchedCount": 0, "updatedCount": 0}
		if rt.DryRun() {
			result.Status = "planned"
			result.Executed = false
		}
		return rt.Output(result)
	}
	return executeRecordBatches(rt, "record_bulk_patch", "update_records", serverMain, updates, verifyUpdateBatch)
}

func parseBulkPatchFilters(raw string) (map[string]any, error) {
	filters, err := parseJSONObject("filters", raw)
	if err != nil {
		return nil, err
	}
	if !bulkPatchFilterConstrictsScope(filters, true) {
		return nil, apperrors.NewValidation("--filters 必须是包含非空筛选条件的 JSON 对象，根 operator 必须为 and/or")
	}
	return filters, nil
}

func bulkPatchFilterConstrictsScope(value any, root bool) bool {
	filter, ok := value.(map[string]any)
	if !ok {
		return false
	}
	operator, ok := filter["operator"].(string)
	if !ok {
		return false
	}
	operator = strings.ToLower(strings.TrimSpace(operator))
	if operator == "" {
		return false
	}
	operands, ok := filter["operands"].([]any)
	if !ok || len(operands) == 0 {
		return false
	}
	if root && operator != "and" && operator != "or" {
		return false
	}
	if operator == "and" || operator == "or" {
		for _, operand := range operands {
			if !bulkPatchFilterConstrictsScope(operand, false) {
				return false
			}
		}
		return true
	}
	switch operator {
	case "eq", "ne", "gt", "lt", "gte", "lte", "contain", "exclusive",
		"exist", "un_exist", "any_of", "all_of", "none_of", "date_eq",
		"before", "after", "not_before", "not_after":
	default:
		return false
	}
	fieldID, ok := operands[0].(string)
	if !ok || strings.TrimSpace(fieldID) == "" {
		return false
	}
	return (operator == "exist" || operator == "un_exist") || len(operands) >= 2
}
