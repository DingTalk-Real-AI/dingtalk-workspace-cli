package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func callMCPToolSheetInfo(toolArgs map[string]any) error {
	if deps.Caller.DryRun() {
		return callMCPTool("get_sheet", toolArgs)
	}
	text, err := callMCPToolReturnText(context.Background(), "get_sheet", toolArgs)
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		deps.Out.PrintRaw(text)
		return nil
	}
	normalizeSheetInfoCoordinatesForAgent(parsed)
	return deps.Out.PrintJSON(parsed)
}

func normalizeSheetInfoCoordinatesForAgent(sheet map[string]any) {
	if nonEmptyRange := normalizeNonEmptyRangeObject(sheet["nonEmptyRange"]); nonEmptyRange != nil {
		sheet["nonEmptyRange"] = nonEmptyRange
	} else if nonEmptyRange := buildNonEmptyRangeFromLegacy(sheet); nonEmptyRange != nil {
		sheet["nonEmptyRange"] = nonEmptyRange
	} else {
		sheet["nonEmptyRange"] = nil
	}

	delete(sheet, "lastNonEmptyRow")
	delete(sheet, "lastNonEmptyColumn")
	delete(sheet, "lastNonEmptyIndexBase")
	delete(sheet, "lastNonEmptyRowNumber")
	delete(sheet, "lastNonEmptyColumnLetter")
	delete(sheet, "nonEmptyRangeA1")
}

func buildNonEmptyRangeFromLegacy(sheet map[string]any) map[string]any {
	row, hasRow := nonNegativeJSONInt(sheet["lastNonEmptyRow"])
	col, hasCol := nonNegativeJSONInt(sheet["lastNonEmptyColumn"])
	if !hasRow || !hasCol {
		return nil
	}

	lastColumnLetter := sheetColumnLetterFromZeroBased(col)
	lastRowNumber := row + 1
	lastCellA1 := fmt.Sprintf("%s%d", lastColumnLetter, lastRowNumber)
	return map[string]any{
		"range":      "A1:" + lastCellA1,
		"lastCell":   lastCellA1,
		"lastRow":    lastRowNumber,
		"lastColumn": lastColumnLetter,
	}
}

func normalizeNonEmptyRangeObject(v any) map[string]any {
	nonEmptyRange, ok := v.(map[string]any)
	if !ok {
		return nil
	}

	rangeValue, hasRange := nonEmptyRange["range"].(string)
	lastCell, hasLastCell := nonEmptyRange["lastCell"].(string)
	lastRow, hasLastRow := nonNegativeJSONInt(nonEmptyRange["lastRow"])
	lastColumn, hasLastColumn := nonEmptyRange["lastColumn"].(string)
	if hasRange && hasLastCell && hasLastRow && hasLastColumn {
		return map[string]any{
			"range":      rangeValue,
			"lastCell":   lastCell,
			"lastRow":    lastRow,
			"lastColumn": lastColumn,
		}
	}

	return nil
}

func nonNegativeJSONInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, n >= 0
	case int64:
		if n < 0 {
			return 0, false
		}
		return int(n), true
	case float64:
		if n < 0 {
			return 0, false
		}
		i := int(n)
		if float64(i) != n {
			return 0, false
		}
		return i, true
	case json.Number:
		i, err := n.Int64()
		if err != nil || i < 0 {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func sheetColumnLetterFromZeroBased(index int) string {
	if index < 0 {
		return ""
	}
	index++
	var b strings.Builder
	for index > 0 {
		index--
		b.WriteByte(byte('A' + index%26))
		index /= 26
	}
	letters := []byte(b.String())
	for i, j := 0, len(letters)-1; i < j; i, j = i+1, j-1 {
		letters[i], letters[j] = letters[j], letters[i]
	}
	return string(letters)
}

// callMCPToolCellInfos 调用 get_cell_infos 并清理 MCP 框架填充的空元数据壳。
func callMCPToolCellInfos(toolArgs map[string]any) error {
	text, err := callMCPToolReturnText(context.Background(), "get_cell_infos", toolArgs)
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// 非 JSON，原样输出
		deps.Out.PrintRaw(text)
		return nil
	}
	// 清理空 dataValidation / hyperlink
	if cells, ok := parsed["cells"].([]any); ok {
		for _, row := range cells {
			rowSlice, ok := row.([]any)
			if !ok {
				continue
			}
			for _, cell := range rowSlice {
				cellMap, ok := cell.(map[string]any)
				if !ok {
					continue
				}
				dv, exists := cellMap["dataValidation"]
				if !exists {
					continue
				}
				if isEmptyDataValidation(dv) {
					delete(cellMap, "dataValidation")
				}
				hyperlink, exists := cellMap["hyperlink"]
				if exists && isEmptyHyperlink(hyperlink) {
					delete(cellMap, "hyperlink")
				}
			}
		}
	}
	return deps.Out.PrintJSON(parsed)
}

// isEmptyDataValidation 判断 dataValidation 是否为全 null 的空壳（MCP 框架按 schema 填充的）。
func isEmptyDataValidation(dv any) bool {
	if dv == nil {
		return true
	}
	dvMap, ok := dv.(map[string]any)
	if !ok {
		return false
	}
	for _, v := range dvMap {
		if v != nil {
			return false
		}
	}
	return true
}

// isEmptyHyperlink 判断 hyperlink 是否为全 null 的空壳（MCP 框架按 schema 填充的）。
func isEmptyHyperlink(hyperlink any) bool {
	if hyperlink == nil {
		return true
	}
	hyperlinkMap, ok := hyperlink.(map[string]any)
	if !ok {
		return false
	}
	for _, v := range hyperlinkMap {
		if v != nil {
			return false
		}
	}
	return true
}
