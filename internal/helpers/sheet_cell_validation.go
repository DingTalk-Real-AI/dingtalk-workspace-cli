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

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// sheet range update 单元格结构的客户端校验，与 wukong 产品层 validateComplexValueCell
// 系列保持一致：拦住缺 type / richText 与整格 hyperlink 共存 / hyperlink.text 与
// cell.text 不一致 / 非法 dataValidation 等非法输入，避免把脏 payload 发到服务端。

var sheetComplexValueStyleFields = map[string]string{
	"bold":      "bool",
	"italic":    "bool",
	"underline": "bool",
	"strike":    "bool",
	"color":     "string",
	"size":      "number",
}

func sheetValidateComplexValueStyle(style map[string]any, path string) error {
	for k, v := range style {
		kind, ok := sheetComplexValueStyleFields[k]
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s 含未知字段 %q（合法字段: bold/italic/underline/strike/color/size）", path, k))
		}
		switch kind {
		case "bool":
			if _, ok := v.(bool); !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.%s 必须为 boolean", path, k))
			}
		case "string":
			if _, ok := v.(string); !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.%s 必须为字符串（如 \"#FF0000\"）", path, k))
			}
		case "number":
			if _, ok := v.(float64); !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.%s 必须为数字", path, k))
			}
		}
	}
	return nil
}

func sheetValidateRichTextItem(item map[string]any, path string) error {
	typeRaw, ok := item["type"]
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s: 缺少 type 字段（合法值: text/link/attachment/image）", path))
	}
	typeVal, ok := typeRaw.(string)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s.type 必须为字符串", path))
	}
	switch typeVal {
	case "text":
		if _, ok := item["text"].(string); !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=text 必须包含 text 字符串字段", path))
		}
	case "link":
		if _, ok := item["text"].(string); !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=link 必须包含 text 字符串字段（显示文字）", path))
		}
		if s, ok := item["link"].(string); !ok || s == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=link 必须包含非空 link 字符串字段（超链接 URL）", path))
		}
	case "attachment":
		if _, ok := item["text"].(string); !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=attachment 必须包含 text 字符串字段（显示文件名）", path))
		}
		if s, ok := item["resourceId"].(string); !ok || s == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=attachment 必须包含非空 resourceId 字符串字段（通过 dws sheet media-upload 获取）", path))
		}
		if s, ok := item["mimeType"].(string); !ok || s == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=attachment 必须包含非空 mimeType 字符串字段", path))
		}
	case "image":
		if s, ok := item["resourceId"].(string); !ok || s == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=image 必须包含非空 resourceId 字符串字段（通过 dws sheet media-upload 获取）", path))
		}
		if s, ok := item["resourceUrl"].(string); !ok || s == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=image 必须包含非空 resourceUrl 字符串字段", path))
		}
	default:
		return apperrors.NewValidation(fmt.Sprintf("%s.type 非法值 %q（合法值: text/link/attachment/image）", path, typeVal))
	}
	if styleRaw, exists := item["style"]; exists {
		if typeVal != "text" && typeVal != "link" {
			return apperrors.NewValidation(fmt.Sprintf("%s: style 字段仅 type=text / link 子项支持", path))
		}
		style, ok := styleRaw.(map[string]any)
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s.style 必须为 object", path))
		}
		if err := sheetValidateComplexValueStyle(style, path+".style"); err != nil {
			return err
		}
	}
	return nil
}

func sheetValidateCellHyperlink(raw any, path string) error {
	if raw == nil {
		return nil
	}
	hyperlink, ok := raw.(map[string]any)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s 必须为 object 或 null", path))
	}
	typeRaw, ok := hyperlink["type"]
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s: 缺少 type 字段（合法值: path / sheet / range / none）", path))
	}
	typeVal, ok := typeRaw.(string)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s.type 必须为字符串", path))
	}
	switch typeVal {
	case "path", "sheet", "range":
		linkRaw, ok := hyperlink["link"]
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s: 缺少 link 字段", path))
		}
		link, ok := linkRaw.(string)
		if !ok || strings.TrimSpace(link) == "" {
			return apperrors.NewValidation(fmt.Sprintf("%s.link 必须为非空字符串", path))
		}
		if textRaw, exists := hyperlink["text"]; exists {
			if _, ok := textRaw.(string); !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.text 必须为字符串", path))
			}
		}
	case "none":
		// type=none 表示显式清除单元格级超链接，不需其他字段
	default:
		return apperrors.NewValidation(fmt.Sprintf("%s.type 非法值 %q（合法值: path / sheet / range / none）", path, typeVal))
	}
	return nil
}

func sheetValidateDataValidation(dvRaw any, path string) error {
	dv, ok := dvRaw.(map[string]any)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s 必须为 object", path))
	}
	dvType, ok := dv["type"].(string)
	if !ok || dvType == "" {
		return apperrors.NewValidation(fmt.Sprintf("%s.type 必须为非空字符串（合法值: dropdown / checkbox / none）", path))
	}
	switch dvType {
	case "dropdown":
		optionsRaw, ok := dv["options"]
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=dropdown 必须包含 options 数组", path))
		}
		options, ok := optionsRaw.([]any)
		if !ok || len(options) == 0 {
			return apperrors.NewValidation(fmt.Sprintf("%s.options 必须为非空数组", path))
		}
		for i, opt := range options {
			optMap, ok := opt.(map[string]any)
			if !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.options[%d] 必须为 object（如 {\"value\":\"选项\"}）", path, i))
			}
			val, ok := optMap["value"].(string)
			if !ok || val == "" {
				return apperrors.NewValidation(fmt.Sprintf("%s.options[%d].value 必须为非空字符串", path, i))
			}
		}
	case "checkbox":
		if c, exists := dv["checked"]; exists {
			if _, ok := c.(bool); !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.checked 必须为 boolean", path))
			}
		}
	case "none":
		// type=none 表示显式清除单元格 DV，不需其他字段
	default:
		return apperrors.NewValidation(fmt.Sprintf("%s.type 非法值 %q（合法值: dropdown / checkbox / none）", path, dvType))
	}
	return nil
}

// sheet range read 回读投影：MCP 框架会按 schema 把 dataValidation / hyperlink
// 填成"全字段 null"的空壳，导致清除后回读仍带着这些 key。与 wukong
// callMCPToolCellInfos 一致：递归找到 cells，把全 null 的空壳 delete 掉，
// 让"清除后回读不应再有该 key"的语义成立。
func sheetCleanCellInfos(v any) {
	switch t := v.(type) {
	case map[string]any:
		if cellsRaw, ok := t["cells"]; ok {
			sheetStripCellsMeta(cellsRaw)
		}
		for _, child := range t {
			sheetCleanCellInfos(child)
		}
	case []any:
		for _, child := range t {
			sheetCleanCellInfos(child)
		}
	}
}

// sheetProjectFlatValues walks an MCP get_cell_infos response and, next to any
// rich `cells` array, adds a flat `values` 2D array of each cell's scalar
// `value`. This gives consumers the simple wukong get_range shape
// (values: [["姓名","部门"]]) without dropping the rich `cells` payload.
// Idempotent: skips when `values` already exists or there are no cells.
func sheetProjectFlatValues(v any) {
	switch t := v.(type) {
	case map[string]any:
		if cellsRaw, ok := t["cells"]; ok {
			if _, exists := t["values"]; !exists {
				if flat := sheetFlatValuesFromCells(cellsRaw); flat != nil {
					t["values"] = flat
				}
			}
		}
		for _, child := range t {
			sheetProjectFlatValues(child)
		}
	case []any:
		for _, child := range t {
			sheetProjectFlatValues(child)
		}
	}
}

func sheetFlatValuesFromCells(cellsRaw any) []any {
	rows, ok := cellsRaw.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		rowSlice, ok := row.([]any)
		if !ok {
			return nil
		}
		flatRow := make([]any, 0, len(rowSlice))
		for _, cell := range rowSlice {
			if cellMap, ok := cell.(map[string]any); ok {
				flatRow = append(flatRow, cellMap["value"])
			} else {
				flatRow = append(flatRow, cell)
			}
		}
		out = append(out, flatRow)
	}
	return out
}

func sheetStripCellsMeta(cellsRaw any) {
	rows, ok := cellsRaw.([]any)
	if !ok {
		return
	}
	for _, row := range rows {
		rowSlice, ok := row.([]any)
		if !ok {
			continue
		}
		for _, cell := range rowSlice {
			cellMap, ok := cell.(map[string]any)
			if !ok {
				continue
			}
			if dv, ok := cellMap["dataValidation"]; ok && sheetIsEmptyMetaShell(dv) {
				delete(cellMap, "dataValidation")
			}
			if hl, ok := cellMap["hyperlink"]; ok && sheetIsEmptyMetaShell(hl) {
				delete(cellMap, "hyperlink")
			}
		}
	}
}

// sheet info 坐标投影：只向 agent 暴露 A1/UI 语义的 nonEmptyRange
// （range / lastCell / lastRow / lastColumn），并清掉服务端的 legacy 0-based
// 字段。与 wukong normalizeSheetInfoCoordinatesForAgent 一致。
func sheetNormalizeInfoCoordinates(v any) {
	switch t := v.(type) {
	case map[string]any:
		_, hasNonEmpty := t["nonEmptyRange"]
		_, hasLegacyRow := t["lastNonEmptyRow"]
		if hasNonEmpty || hasLegacyRow {
			sheetApplyNonEmptyRange(t)
		}
		for _, child := range t {
			sheetNormalizeInfoCoordinates(child)
		}
	case []any:
		for _, child := range t {
			sheetNormalizeInfoCoordinates(child)
		}
	}
}

func sheetApplyNonEmptyRange(sheet map[string]any) {
	if ner := sheetNormalizeNonEmptyRangeObject(sheet["nonEmptyRange"]); ner != nil {
		sheet["nonEmptyRange"] = ner
	} else if ner := sheetBuildNonEmptyRangeFromLegacy(sheet); ner != nil {
		sheet["nonEmptyRange"] = ner
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

func sheetNormalizeNonEmptyRangeObject(v any) map[string]any {
	ner, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	rangeValue, hasRange := ner["range"].(string)
	lastCell, hasLastCell := ner["lastCell"].(string)
	lastRow, hasLastRow := sheetNonNegativeJSONInt(ner["lastRow"])
	lastColumn, hasLastColumn := ner["lastColumn"].(string)
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

func sheetBuildNonEmptyRangeFromLegacy(sheet map[string]any) map[string]any {
	row, hasRow := sheetNonNegativeJSONInt(sheet["lastNonEmptyRow"])
	col, hasCol := sheetNonNegativeJSONInt(sheet["lastNonEmptyColumn"])
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

func sheetNonNegativeJSONInt(v any) (int, bool) {
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

// sheetIsEmptyMetaShell 判断 metadata 块是否为全 null 空壳（MCP 框架按 schema 填充的）。
func sheetIsEmptyMetaShell(v any) bool {
	if v == nil {
		return true
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for _, vv := range m {
		if vv != nil {
			return false
		}
	}
	return true
}

func sheetValidateComplexValueCell(cell map[string]any, path string) error {
	if dvRaw, exists := cell["dataValidation"]; exists {
		if err := sheetValidateDataValidation(dvRaw, path+".dataValidation"); err != nil {
			return err
		}
	}

	if hyperlinkRaw, exists := cell["hyperlink"]; exists {
		if err := sheetValidateCellHyperlink(hyperlinkRaw, path+".hyperlink"); err != nil {
			return err
		}
	}

	typeRaw, hasType := cell["type"]
	// 没有 type 时，必须有 metadata 字段（不写值，只更新元数据）
	if !hasType {
		_, hasDV := cell["dataValidation"]
		_, hasCS := cell["cellStyles"]
		_, hasHyperlink := cell["hyperlink"]
		if hasDV || hasCS || hasHyperlink {
			return nil
		}
		return apperrors.NewValidation(fmt.Sprintf("%s: 缺少 type 字段（合法值: text/richText）", path))
	}
	typeVal, ok := typeRaw.(string)
	if !ok {
		return apperrors.NewValidation(fmt.Sprintf("%s.type 必须为字符串", path))
	}
	switch typeVal {
	case "text":
		var cellText string
		hasCellText := false
		if t, exists := cell["text"]; exists {
			textValue, ok := t.(string)
			if !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.text 必须为字符串（text=\"\" 表示清空 cell）", path))
			}
			cellText = textValue
			hasCellText = true
		}
		if styleRaw, exists := cell["style"]; exists {
			style, ok := styleRaw.(map[string]any)
			if !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.style 必须为 object", path))
			}
			if err := sheetValidateComplexValueStyle(style, path+".style"); err != nil {
				return err
			}
		}
		if hyperlinkRaw, exists := cell["hyperlink"]; exists && hyperlinkRaw != nil {
			hyperlink, _ := hyperlinkRaw.(map[string]any)
			if hyperlinkText, ok := hyperlink["text"].(string); ok && hasCellText && hyperlinkText != cellText {
				return apperrors.NewValidation(fmt.Sprintf("%s.hyperlink.text 与 %s.text 不一致，请只传 cell.text 或保持两者相同", path, path))
			}
		}
	case "richText":
		if _, hasHyperlink := cell["hyperlink"]; hasHyperlink {
			return apperrors.NewValidation(fmt.Sprintf("%s: cell-level hyperlink 不能与 type=richText 同时使用；整格链接用 hyperlink，片段链接用 richText.texts[].type=link", path))
		}
		textsRaw, ok := cell["texts"]
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s: type=richText 必须包含 texts 数组", path))
		}
		texts, ok := textsRaw.([]any)
		if !ok {
			return apperrors.NewValidation(fmt.Sprintf("%s.texts 必须为数组", path))
		}
		if len(texts) == 0 {
			return apperrors.NewValidation(fmt.Sprintf("%s.texts 不能为空数组", path))
		}
		for i, item := range texts {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return apperrors.NewValidation(fmt.Sprintf("%s.texts[%d] 必须为 object", path, i))
			}
			if err := sheetValidateRichTextItem(itemMap, fmt.Sprintf("%s.texts[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		return apperrors.NewValidation(fmt.Sprintf("%s.type 非法值 %q（合法值: text / richText；不再支持 number/boolean/null，数字布尔请用 {type:text,text:\"...\"} 字符串形式）", path, typeVal))
	}
	return nil
}
