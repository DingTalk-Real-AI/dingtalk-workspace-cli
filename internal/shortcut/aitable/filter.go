// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package aitable

import (
	"fmt"
	"strings"
)

var recordFilterOperators = map[string]bool{
	"eq": true, "ne": true, "gt": true, "lt": true, "gte": true, "lte": true,
	"contain": true, "exclusive": true, "exist": true, "un_exist": true,
	"any_of": true, "all_of": true, "none_of": true,
	"date_eq": true, "before": true, "after": true, "not_before": true, "not_after": true,
}

// parseRecordFilters enforces the same canonical wire grammar published by
// the AITable filter reference before a Shortcut reaches query_records.  The
// service may silently ignore malformed filters, so remote validation is not
// sufficient evidence that the requested predicate was applied.
func parseRecordFilters(raw string) (any, error) {
	parsed, err := parseJSONAny("filters", raw)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeRecordFilterNode(parsed, true)
	if err != nil {
		return nil, fmt.Errorf("invalid --filters: %w", err)
	}
	return normalized, nil
}

func normalizeRecordFilterNode(value any, root bool) (map[string]any, error) {
	filter, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(`root must be {"operator":"and|or","operands":[...]}`)
	}
	op, ok := filter["operator"].(string)
	if !ok || strings.TrimSpace(op) == "" {
		return nil, fmt.Errorf("operator must be a non-empty string")
	}
	op = strings.ToLower(strings.TrimSpace(op))
	operands, ok := filter["operands"].([]any)
	if !ok {
		if _, shorthand := filter["fieldId"]; shorthand && !root {
			return normalizeRecordFilterShorthand(filter, op)
		}
		return nil, fmt.Errorf("operator %q requires an operands array", op)
	}

	if op == "and" || op == "or" {
		normalized := make([]any, 0, len(operands))
		for index, operand := range operands {
			child, err := normalizeRecordFilterNode(operand, false)
			if err != nil {
				return nil, fmt.Errorf("operands[%d]: %w", index, err)
			}
			normalized = append(normalized, child)
		}
		return map[string]any{"operator": op, "operands": normalized}, nil
	}
	if root {
		return nil, fmt.Errorf("root operator must be and or or, got %q", op)
	}
	if !recordFilterOperators[op] {
		return nil, fmt.Errorf("unsupported operator %q; use canonical operators such as eq or contain", op)
	}
	if len(operands) == 0 {
		return nil, fmt.Errorf("operator %q requires a fieldId operand", op)
	}
	if (op == "exist" || op == "un_exist") && len(operands) != 1 {
		return nil, fmt.Errorf("operator %q requires exactly one fieldId operand", op)
	}
	if op != "exist" && op != "un_exist" && len(operands) < 2 {
		return nil, fmt.Errorf("operator %q requires fieldId and value operands", op)
	}
	return map[string]any{"operator": op, "operands": operands}, nil
}

func normalizeRecordFilterShorthand(filter map[string]any, op string) (map[string]any, error) {
	if !recordFilterOperators[op] {
		return nil, fmt.Errorf("unsupported operator %q; use canonical operators such as eq or contain", op)
	}
	fieldID, ok := filter["fieldId"].(string)
	if !ok || strings.TrimSpace(fieldID) == "" {
		return nil, fmt.Errorf("fieldId must be a non-empty string")
	}
	operands := []any{strings.TrimSpace(fieldID)}
	if op != "exist" && op != "un_exist" {
		value, exists := filter["value"]
		if !exists {
			return nil, fmt.Errorf("operator %q requires value", op)
		}
		operands = append(operands, value)
	}
	return map[string]any{"operator": op, "operands": operands}, nil
}
