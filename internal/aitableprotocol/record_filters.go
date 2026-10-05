// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

const RecordFilterOperators = "eq, ne, gt, lt, gte, lte, contain, exclusive, exist, un_exist, any_of, all_of, none_of, date_eq, before, after, not_before, not_after"

var recordFilterOperators = []string{"eq", "ne", "gt", "lt", "gte", "lte", "contain", "exclusive", "exist", "un_exist", "any_of", "all_of", "none_of", "date_eq", "before", "after", "not_before", "not_after"}

const RecordFiltersHelp = `记录筛选 JSON：{"operator":"and|or","operands":[{"operator":"gt","operands":["<fieldId>",5]}]}；子条件也支持 {"fieldId":"<fieldId>","operator":"gt","value":5}。操作符：` + RecordFilterOperators + `；exist/un_exist 只传 fieldId；日期使用日期字符串或整数毫秒数，不接受 View relative/exact Scheme`

// ParseRecordFilters validates both public spellings before constructing the
// wire filter. An invalid selector must never widen a bulk write.
func ParseRecordFilters(raw string) (map[string]any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("--filters must be a valid JSON object: %w", err)
	}
	return normalizeRecordFilter(value, "filters", true)
}

func normalizeRecordFilter(value any, path string, root bool) (map[string]any, error) {
	filter, ok := value.(map[string]any)
	if !ok || filter == nil {
		return nil, fmt.Errorf("%s must be a JSON object", path)
	}
	for key := range filter {
		switch key {
		case "operator", "operands", "fieldId", "value":
		default:
			return nil, fmt.Errorf("%s.%s is not a supported filter property; refusing to discard a selector", path, key)
		}
	}
	operator, ok := filter["operator"].(string)
	if !ok || strings.TrimSpace(operator) == "" {
		return nil, fmt.Errorf("%s.operator must be a non-empty string", path)
	}
	operator = strings.ToLower(strings.TrimSpace(operator))
	if _, hasValue := filter["value"]; hasValue {
		_, hasField := filter["fieldId"]
		if !hasField || operator == "and" || operator == "or" {
			return nil, fmt.Errorf("%s.value is only valid with a comparison fieldId; do not mix the two filter formats", path)
		}
	}
	if root && operator != "and" && operator != "or" {
		return nil, fmt.Errorf("%s.operator at root must be and/or, got %q", path, operator)
	}
	operands, hasOperands := filter["operands"].([]any)
	if operator == "and" || operator == "or" {
		if _, shorthand := filter["fieldId"]; shorthand {
			return nil, fmt.Errorf("%s: logical operator %s cannot contain fieldId", path, operator)
		}
		if !hasOperands || len(operands) == 0 {
			return nil, fmt.Errorf("%s.operands requires a non-empty array of conditions", path)
		}
		children := make([]any, 0, len(operands))
		for i, operand := range operands {
			child, err := normalizeRecordFilter(operand, fmt.Sprintf("%s.operands[%d]", path, i), false)
			if err != nil {
				return nil, err
			}
			children = append(children, child)
		}
		return map[string]any{"operator": operator, "operands": children}, nil
	}
	if !slices.Contains(recordFilterOperators, operator) {
		return nil, fmt.Errorf("%s.operator: unsupported filter operator %q; supported operators: %s", path, operator, RecordFilterOperators)
	}
	if fieldID, shorthand := filter["fieldId"]; shorthand {
		if _, mixed := filter["operands"]; mixed {
			return nil, fmt.Errorf("%s: do not mix fieldId/value with operands", path)
		}
		operands = []any{fieldID}
		if comparison, exists := filter["value"]; exists {
			operands = append(operands, comparison)
		}
	}
	want := 2
	if operator == "exist" || operator == "un_exist" {
		want = 1
	}
	if len(operands) != want {
		return nil, fmt.Errorf("%s.operands: operator %s requires exactly %d operands, starting with fieldId", path, operator, want)
	}
	fieldID, ok := operands[0].(string)
	if !ok || strings.TrimSpace(fieldID) == "" {
		return nil, fmt.Errorf("%s.operands[0] must be a non-empty fieldId", path)
	}
	switch operator {
	case "date_eq", "before", "after", "not_before", "not_after":
		switch date := operands[1].(type) {
		case string:
			if strings.TrimSpace(date) != "" {
				return map[string]any{"operator": operator, "operands": operands}, nil
			}
		case float64:
			if !math.IsNaN(date) && !math.IsInf(date, 0) && date == math.Trunc(date) {
				return map[string]any{"operator": operator, "operands": operands}, nil
			}
		}
		return nil, fmt.Errorf("%s: record query operator %s requires a date/RFC3339 string or Unix-millisecond JSON number; relative/exact objects belong to view update filter", path, operator)
	}
	return map[string]any{"operator": operator, "operands": operands}, nil
}
