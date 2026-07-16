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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ── workbook & worksheet ──────────────────────────────────────────────

// Create creates a new DingTalk online spreadsheet document.
// ListSheets lists all worksheets in a spreadsheet document.
var ListSheets = shortcut.Shortcut{
	Service:     "sheet",
	Command:     "+list-sheets",
	Product:     "sheet",
	Description: "获取表格文档中全部工作表列表",
	Intent:      "当你拿到一个表格文档、想先了解它里面有哪些工作表（sheet）以及各自的 sheetId 时使用，通常作为读写具体数据前的第一步；传入表格文档 ID 或 URL，返回工作表清单。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "表格文档 ID 或 URL", Required: true},
	},
	Tips: []string{`dws sheet +list-sheets --node NODE_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("sheet", "get_all_sheets", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		sheets := listSheetsProject(data)
		return rt.Output(map[string]any{"count": len(sheets), "sheets": sheets})
	},
}

// listSheetsProject reshapes the raw get_all_sheets response into a clean
// worksheet list (sheetId/title/index/visibility/rowCount/columnCount) — the
// the clean output projection applied to every list command. Both
// the list container and the per-item field names are probed defensively across
// candidate keys, so an empty/unknown shape yields an empty list rather than a
// crash or fabricated data.
func listSheetsProject(data map[string]any) []map[string]any {
	raw := listSheetsResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := listSheetsFirst(m, "sheetId", "sheet_id", "id"); ok {
			row["sheetId"] = v
		}
		if v, ok := listSheetsFirst(m, "title", "name", "sheetName", "sheet_name"); ok {
			row["title"] = v
		}
		if v, ok := listSheetsFirst(m, "index", "sheetIndex"); ok {
			row["index"] = v
		}
		if v, ok := listSheetsFirst(m, "visibility", "hidden", "visible"); ok {
			row["visibility"] = v
		}
		if v, ok := listSheetsFirst(m, "rowCount", "row_count", "rows"); ok {
			row["rowCount"] = v
		}
		if v, ok := listSheetsFirst(m, "columnCount", "column_count", "columns", "colCount"); ok {
			row["columnCount"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listSheetsResolveList locates the worksheet array inside the response,
// tolerating a bare top-level array container or nesting one level deeper under
// a common envelope key.
func listSheetsResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	for _, key := range []string{"sheets", "result", "data", "list", "items", "worksheets"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"sheets", "list", "items", "worksheets", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// listSheetsFirst returns the first present candidate key's value.
func listSheetsFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// SheetInfo returns the detail of a single worksheet.
// SheetCreate adds a new worksheet to a spreadsheet.
// SheetUpdate updates worksheet properties (name/position/hidden/freeze/tab color).
// SheetCopy duplicates a worksheet within the same spreadsheet.
// SheetDelete removes a worksheet (irreversible).
// ── data read/write ───────────────────────────────────────────────────

// Read reads structured per-cell data from a worksheet range.
var Read = shortcut.Shortcut{
	Service:     "sheet",
	Command:     "+read",
	Product:     "sheet",
	Description: "读取工作表指定范围的结构化单元格数据",
	Intent:      "当你需要按单元格逐格获取数据（含类型、公式或格式化值等结构化信息）以便程序处理时使用；传入表格与可选范围（A1 表示法，不传则全部），可指定取格式化值/原始值/公式，返回结构化单元格数组。若只想要纯文本可改用 +csv-get。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "表格文档 ID 或 URL", Required: true},
		{Name: "sheet-id", Type: shortcut.FlagString, Desc: "工作表 ID 或名称 (不传则第一个工作表)"},
		{Name: "range", Type: shortcut.FlagString, Desc: "读取范围，A1 表示法 (不传则全部数据)"},
		{Name: "value-render-option", Type: shortcut.FlagString, Desc: "取值模式", Enum: []string{"formatted_value", "raw_value", "formula"}},
	},
	Tips: []string{`dws sheet +read --node NODE_ID --sheet-id SHEET_ID --range "A1:D10"`},
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
		return rt.CallMCP("get_cell_infos", params)
	},
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
