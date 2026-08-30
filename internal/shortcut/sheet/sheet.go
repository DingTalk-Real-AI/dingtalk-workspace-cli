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

// Package sheet declares high-fidelity shortcuts for the DingTalk sheet MCP
// product. Tool names and parameter keys mirror the helper commands under
// internal/helpers/sheet_*.go verbatim.
package sheet

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/responsecheck"
)

// ── workbook & worksheet ──────────────────────────────────────────────

// Create creates a new DingTalk online spreadsheet document.
// ListSheets lists all worksheets in a spreadsheet document.
var ListSheets = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "sheet",
	Command:       "+list-sheets",
	Product:       "sheet",
	Description:   "严格列出在线电子表格的工作表，并可按完整标题精确筛选",
	Intent:        "需要发现一个在线电子表格中的工作表与稳定 sheetId，或验证某个完整工作表标题是否存在时",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "sheet",
			Name:           "shortcut_list_sheets",
			CanonicalPath:  "sheet.shortcut_list_sheets",
			CLIPath:        "sheet +list-sheets",
			PrimaryCLIPath: "sheet +list-sheets",
		},
		Description: "严格列出在线电子表格的工作表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "严格列出在线电子表格的工作表，并可按完整标题精确筛选",
			UseWhen:      []string{"需要发现一个在线电子表格中的工作表与稳定 sheetId，或验证某个完整工作表标题是否存在时"},
			AvoidWhen:    []string{"需要管理 AITable/Base 的数据表或记录时改用 aitable；需要原始 get_all_sheets 响应时改用 sheet list"},
			Examples:     []string{"dws sheet +list-sheets --node NODE_ID"},
		},
		Parameters: []contract.ParamDecl{
			{Name: "node", Property: "node"},
			{Name: "title", Property: "title"},
		},
		Result: &contract.ResultSpec{
			Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			DataSchema:     json.RawMessage(`{"type":"object","description":"经过严格校验的工作表列表","properties":{"count":{"type":"integer","description":"精确筛选后的工作表数量"},"sheets":{"type":"array","description":"工作表条目；显式空数组表示合法零命中","items":{"type":"object","description":"带稳定身份的工作表","properties":{"sheetId":{"type":"string","description":"稳定工作表 ID"},"title":{"type":"string","description":"工作表完整标题"},"index":{"type":"integer","description":"可选工作表顺序"},"visibility":{"description":"可选工作表可见性"},"rowCount":{"type":"integer","description":"可选行数"},"columnCount":{"type":"integer","description":"可选列数"}},"required":["sheetId","title"],"additionalProperties":false}}},"required":["count","sheets"],"additionalProperties":false}`),
			SensitivePaths: []string{"sheets.sheetId", "sheets.title"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "表格文档 ID 或 URL；--node 去除空白后不能为空", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "按完整工作表标题精确筛选（区分大小写）；显式传入时去除空白后不能为空"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node"}, Description: "--node 去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"title"}, Description: "--title 显式传入时去除空白后不能为空"},
	},
	Tips: []string{`dws sheet +list-sheets --node NODE_ID`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := validateSheetNode(rt); err != nil {
			return err
		}
		return validateOptionalSheetString(rt, "title")
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("sheet", "get_all_sheets", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		sheets, err := projectSheetList(data, rt.Str("title"))
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(sheets), "sheets": sheets})
	},
}

func projectSheetList(data map[string]any, exactTitle string) ([]map[string]any, error) {
	items, err := responsecheck.RequireObjectCollection(data, "sheet/get_all_sheets", "sheets")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		sheetID, err := requireSheetString(item, index, "sheetId")
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[sheetID]; duplicate {
			return nil, responsecheck.Error("sheet/get_all_sheets", "duplicate_sheet_id", fmt.Sprintf("响应 sheets[%d] 的 sheetId 重复", index))
		}
		seen[sheetID] = struct{}{}
		title, err := requireSheetStringAliases(item, index, "name", "title")
		if err != nil {
			return nil, err
		}
		if exactTitle != "" && title != exactTitle {
			continue
		}
		row := map[string]any{"sheetId": sheetID, "title": title}
		if err := copySheetOptionalNumber(row, item, index, "index", "index", "sheetIndex"); err != nil {
			return nil, err
		}
		if err := copySheetOptionalVisibility(row, item, index, "visibility", "visibility", "hidden", "visible"); err != nil {
			return nil, err
		}
		if err := copySheetOptionalNumber(row, item, index, "rowCount", "rowCount", "row_count", "rows"); err != nil {
			return nil, err
		}
		if err := copySheetOptionalNumber(row, item, index, "columnCount", "columnCount", "column_count", "columns", "colCount"); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// SheetInfo returns the detail of a single worksheet.
// SheetCreate adds a new worksheet to a spreadsheet.
// SheetUpdate updates worksheet properties (name/position/hidden/freeze/tab color).
// SheetCopy duplicates a worksheet within the same spreadsheet.
// SheetDelete removes a worksheet (irreversible).
// ── data read/write ───────────────────────────────────────────────────

// Read reads structured per-cell data from a worksheet range.
var Read = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "sheet",
	Command:       "+read",
	Product:       "sheet",
	Description:   "完整读取并严格校验在线电子表格范围；截断结果失败关闭",
	Intent:        "需要逐格读取在线电子表格的值、公式或格式化值，并要求结果未被服务端截断时",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "sheet",
			Name:           "shortcut_read",
			CanonicalPath:  "sheet.shortcut_read",
			CLIPath:        "sheet +read",
			PrimaryCLIPath: "sheet +read",
		},
		Description: "完整读取并严格校验工作表范围",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "完整读取并严格校验在线电子表格范围；截断结果失败关闭",
			UseWhen:      []string{"需要逐格读取在线电子表格的值、公式或格式化值，并要求结果未被服务端截断时"},
			AvoidWhen:    []string{"需要纯 CSV 时用 sheet csv-get；需要 AITable/Base 结构化记录时用 aitable；超大范围应拆小后重试，不能把截断结果当完整成功"},
			Examples:     []string{"dws sheet +read --node NODE_ID --sheet-id SHEET_ID --range \"A1:D10\""},
		},
		Parameters: []contract.ParamDecl{
			{Name: "node", Property: "node"},
			{Name: "sheet-id", Property: "sheetId"},
			{Name: "range", Property: "range"},
			{Name: "value-render-option", Property: "valueRenderOption"},
		},
		DryRun: &contract.DryRunSpec{
			PreviewKind: contract.DryRunPreviewRequest,
			RemoteReads: false,
		},
		Result: &contract.ResultSpec{
			Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			DataSchema:     json.RawMessage(`{"type":"object","description":"完整且未截断的单元格范围","properties":{"cells":{"type":"array","description":"与行列坐标对齐的二维单元格对象数组","items":{"type":"array","description":"一行单元格对象","items":{"type":"object","description":"单元格结构化数据","additionalProperties":true}}},"colIndices":{"type":"array","description":"与 cells 列对齐的 A1 列标","items":{"type":"string"}},"rowIndices":{"type":"array","description":"与 cells 行对齐的一基行号","items":{"type":"integer"}},"complete":{"type":"boolean","description":"结果已证明完整；成功结果恒为 true"},"hasMore":{"type":"boolean","description":"服务端截断标记；成功结果恒为 false"},"truncationReasons":{"type":"array","description":"截断原因；成功结果恒为空数组","items":{"type":"string"}},"resolvedRange":{"type":"string","description":"服务解析后的请求范围"},"returnedRange":{"type":"string","description":"服务实际返回的范围"},"message":{"type":"string","description":"可选服务说明"}},"required":["cells","colIndices","rowIndices","complete","hasMore","truncationReasons"],"additionalProperties":false}`),
			SensitivePaths: []string{"cells"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "表格文档 ID 或 URL；--node 去除空白后不能为空", Required: true},
		{Name: "sheet-id", Type: shortcut.FlagString, Desc: "工作表 ID 或名称 (不传则第一个工作表)；显式传入时去除空白后不能为空"},
		{Name: "range", Type: shortcut.FlagString, Desc: "读取范围，A1 表示法 (不传则全部数据)；显式传入时去除空白后不能为空"},
		{Name: "value-render-option", Type: shortcut.FlagString, Desc: "取值模式", Enum: []string{"formatted_value", "raw_value", "formula"}},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node"}, Description: "--node 去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"sheet-id"}, Description: "--sheet-id 显式传入时去除空白后不能为空"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"range"}, Description: "--range 显式传入时去除空白后不能为空"},
	},
	Tips: []string{`dws sheet +read --node NODE_ID --sheet-id SHEET_ID --range "A1:D10"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := validateSheetNode(rt); err != nil {
			return err
		}
		for _, name := range []string{"sheet-id", "range"} {
			if err := validateOptionalSheetString(rt, name); err != nil {
				return err
			}
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if rt.Changed("sheet-id") {
			params["sheetId"] = rt.Str("sheet-id")
		}
		if rt.Changed("range") {
			params["range"] = rt.Str("range")
		}
		if rt.Changed("value-render-option") {
			params["valueRenderOption"] = rt.Str("value-render-option")
		}
		if rt.DryRun() {
			return rt.CallMCP("get_cell_infos", params)
		}
		data, err := rt.CallMCPData("sheet", "get_cell_infos", params)
		if err != nil {
			return err
		}
		projected, err := projectSheetRead(data)
		if err != nil {
			return err
		}
		return rt.Output(projected)
	},
}

func validateSheetNode(rt *shortcut.RuntimeContext) error {
	if strings.TrimSpace(rt.Str("node")) == "" {
		return apperrors.NewValidation("--node 去除空白后不能为空")
	}
	return nil
}

func validateOptionalSheetString(rt *shortcut.RuntimeContext, name string) error {
	if rt.Changed(name) && strings.TrimSpace(rt.Str(name)) == "" {
		return apperrors.NewValidation(fmt.Sprintf("--%s 显式传入时去除空白后不能为空", name))
	}
	return nil
}

func requireSheetString(item map[string]any, index int, key string) (string, error) {
	value, present := item[key]
	if !present {
		return "", responsecheck.Error("sheet/get_all_sheets", "missing_sheet_identity", fmt.Sprintf("响应 sheets[%d] 缺少 %s", index, key))
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", responsecheck.Error("sheet/get_all_sheets", "malformed_sheet_identity", fmt.Sprintf("响应 sheets[%d].%s 必须是非空字符串", index, key))
	}
	return strings.TrimSpace(text), nil
}

func requireSheetStringAliases(item map[string]any, index int, keys ...string) (string, error) {
	for _, key := range keys {
		if _, present := item[key]; present {
			return requireSheetString(item, index, key)
		}
	}
	return "", responsecheck.Error("sheet/get_all_sheets", "missing_sheet_title", fmt.Sprintf("响应 sheets[%d] 缺少工作表标题", index))
}

func copySheetOptionalNumber(target, source map[string]any, itemIndex int, targetKey string, sourceKeys ...string) error {
	for _, key := range sourceKeys {
		if value, present := source[key]; present {
			number, ok := value.(float64)
			if !ok || number < 0 || math.Trunc(number) != number {
				return responsecheck.Error("sheet/get_all_sheets", "malformed_sheet_field", fmt.Sprintf("响应 sheets[%d].%s 必须是非负整数", itemIndex, key))
			}
			target[targetKey] = value
			return nil
		}
	}
	return nil
}

func copySheetOptionalVisibility(target, source map[string]any, itemIndex int, targetKey string, sourceKeys ...string) error {
	for _, key := range sourceKeys {
		value, present := source[key]
		if !present {
			continue
		}
		switch typed := value.(type) {
		case bool:
			target[targetKey] = typed
		case string:
			if strings.TrimSpace(typed) == "" {
				return responsecheck.Error("sheet/get_all_sheets", "malformed_sheet_field", fmt.Sprintf("响应 sheets[%d].%s 不能为空字符串", itemIndex, key))
			}
			target[targetKey] = strings.TrimSpace(typed)
		default:
			return responsecheck.Error("sheet/get_all_sheets", "malformed_sheet_field", fmt.Sprintf("响应 sheets[%d].%s 必须是布尔值或字符串", itemIndex, key))
		}
		return nil
	}
	return nil
}

func projectSheetRead(data map[string]any) (map[string]any, error) {
	envelope, err := responsecheck.RequireSuccess(data, "sheet/get_cell_infos")
	if err != nil {
		return nil, err
	}
	cellsRaw, err := requireSheetArray(envelope, "cells")
	if err != nil {
		return nil, err
	}
	columnsRaw, err := requireSheetArray(envelope, "colIndices")
	if err != nil {
		return nil, err
	}
	rowsRaw, err := requireSheetArray(envelope, "rowIndices")
	if err != nil {
		return nil, err
	}
	columns := make([]any, len(columnsRaw))
	for index, value := range columnsRaw {
		column, ok := value.(string)
		if !ok || strings.TrimSpace(column) == "" {
			return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_column_index", fmt.Sprintf("响应 colIndices[%d] 必须是非空字符串", index))
		}
		columns[index] = strings.TrimSpace(column)
	}
	rows := make([]any, len(rowsRaw))
	for index, value := range rowsRaw {
		number, ok := value.(float64)
		if !ok || number < 1 || math.Trunc(number) != number {
			return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_row_index", fmt.Sprintf("响应 rowIndices[%d] 必须是正整数", index))
		}
		rows[index] = int(number)
	}
	if len(cellsRaw) != len(rows) {
		return nil, responsecheck.Error("sheet/get_cell_infos", "row_count_mismatch", "响应 cells 行数与 rowIndices 数量不一致")
	}
	cells := make([]any, len(cellsRaw))
	for rowIndex, rowValue := range cellsRaw {
		row, ok := rowValue.([]any)
		if !ok {
			return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_cell_row", fmt.Sprintf("响应 cells[%d] 必须是数组", rowIndex))
		}
		if len(row) != len(columns) {
			return nil, responsecheck.Error("sheet/get_cell_infos", "column_count_mismatch", fmt.Sprintf("响应 cells[%d] 列数与 colIndices 数量不一致", rowIndex))
		}
		clean := make([]any, len(row))
		for columnIndex, cellValue := range row {
			cell, ok := cellValue.(map[string]any)
			if !ok {
				return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_cell", fmt.Sprintf("响应 cells[%d][%d] 必须是对象", rowIndex, columnIndex))
			}
			clean[columnIndex] = cell
		}
		cells[rowIndex] = clean
	}
	hasMoreValue, present := envelope["hasMore"]
	if !present {
		return nil, responsecheck.Error("sheet/get_cell_infos", "missing_completion_evidence", "响应缺少 hasMore，无法证明读取完整")
	}
	hasMore, ok := hasMoreValue.(bool)
	if !ok {
		return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_completion_evidence", "响应 hasMore 必须是布尔值")
	}
	reasonsRaw, err := requireSheetArray(envelope, "truncationReasons")
	if err != nil {
		return nil, err
	}
	reasons := make([]any, len(reasonsRaw))
	for index, value := range reasonsRaw {
		reason, ok := value.(string)
		if !ok || strings.TrimSpace(reason) == "" {
			return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_truncation_reason", fmt.Sprintf("响应 truncationReasons[%d] 必须是非空字符串", index))
		}
		reasons[index] = strings.TrimSpace(reason)
	}
	if hasMore {
		if len(reasons) == 0 {
			return nil, responsecheck.Error("sheet/get_cell_infos", "missing_truncation_reason", "响应 hasMore=true 但缺少截断原因")
		}
		return nil, responsecheck.Error("sheet/get_cell_infos", "incomplete_result", "工作表读取结果被服务端截断；请缩小 --range 后重试，不能把部分数据当作完整成功")
	}
	if len(reasons) != 0 {
		return nil, responsecheck.Error("sheet/get_cell_infos", "conflicting_completion_evidence", "响应 hasMore=false 但仍声明截断原因")
	}
	out := map[string]any{
		"cells": cells, "colIndices": columns, "rowIndices": rows,
		"complete": true, "hasMore": false, "truncationReasons": reasons,
	}
	for _, key := range []string{"resolvedRange", "returnedRange", "message"} {
		if value, present := envelope[key]; present {
			text, ok := value.(string)
			if !ok {
				return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_optional_field", fmt.Sprintf("响应 %s 必须是字符串", key))
			}
			out[key] = text
		}
	}
	return out, nil
}

func requireSheetArray(object map[string]any, key string) ([]any, error) {
	value, present := object[key]
	if !present {
		return nil, responsecheck.Error("sheet/get_cell_infos", "missing_"+key, fmt.Sprintf("响应缺少 %s 数组", key))
	}
	array, ok := value.([]any)
	if !ok {
		return nil, responsecheck.Error("sheet/get_cell_infos", "malformed_"+key, fmt.Sprintf("响应 %s 必须是数组", key))
	}
	return array, nil
}

// Write updates a worksheet range with a 2D JSON array of cell objects.
// Append appends rows to the end of a worksheet.
// CSVGet reads worksheet data as RFC 4180 CSV.
// CSVPut writes CSV text into a worksheet at a start cell (values only).
// CellsSearch finds cells matching a text query.
// CellsReplace finds and replaces text across a worksheet.
// CellsClear clears content/format of a worksheet range.
// ── range operations ──────────────────────────────────────────────────

// RangeSort sorts a worksheet range by one or more keys.
// RangeFill auto-fills a target range from a source range.
// RangeCopy copies a source range to a target location (supports cross-sheet).
// RangeMove moves a source range to a target location (source is cleared).
// ── dimensions & merge ────────────────────────────────────────────────

// InsertDimension inserts empty rows or columns before a position.
// AddDimension appends empty rows or columns at the end of a worksheet.
// MoveDimension moves rows or columns to a destination position.
// UpdateDimension updates hidden state and/or row height / column width.
// DeleteDimension deletes rows or columns from a position (irreversible).
// MergeCells merges a range of cells.
// UnmergeCells unmerges cells within a range.
// ── dropdown lists ────────────────────────────────────────────────────

// SetDropdown sets a dropdown list on a cell range.
// GetDropdown reads dropdown list configuration for a range.
// DeleteDropdown removes dropdown list configuration from a range.
func init() {
	shortcut.Register(
		ListSheets,
		Read,
	)
}
