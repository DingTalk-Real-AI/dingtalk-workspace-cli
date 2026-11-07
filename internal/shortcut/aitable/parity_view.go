// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func executeFilteredViewUpdate(rt *shortcut.RuntimeContext, params map[string]any, config map[string]any) error {
	if err := helpers.NormalizeAITableViewConfig(rt.Command().Context(), rt.Str("base-id"), rt.Str("table-id"), config); err != nil {
		return err
	}
	params["config"] = config
	if rt.DryRun() {
		return rt.Output(map[string]any{"executed": false, "arguments": params})
	}
	id := rt.Str("view-id")
	if _, err := readExactParityView(rt, id); err != nil {
		return err
	}
	write, err := rt.CallMCPWriteDataStrict(serverMain, "update_view", params)
	if err != nil {
		return helpers.AnnotateViewUpdateError(err)
	}
	actual, err := readExactParityView(rt, id)
	if err != nil {
		return err
	}
	expectedConfig := cloneAnyMap(config)
	actualForCompare := cloneAnyMap(actual)
	if filter, ok := config["filter"]; ok {
		expectedConfig["filter"] = canonicalParityViewFilter(filter)
		rawFilter, exists := actual["filter"]
		if !exists {
			if nested, ok := actual["config"].(map[string]any); ok {
				rawFilter = nested["filter"]
			}
		}
		actualForCompare["filter"] = canonicalParityViewFilter(rawFilter)
	}
	if !presetViewMatches(actualForCompare, stringValue(actual, "viewType", "type"), expectedConfig) {
		return fmt.Errorf("view update readback does not match requested config; inspect the view before retrying")
	}
	if name, ok := params["newViewName"].(string); ok && stringValue(actual, "viewName", "name") != name {
		return fmt.Errorf("view update name readback mismatch")
	}
	if desc, ok := params["viewDescription"]; ok && !declaredValueMatches(actual["viewDescription"], desc) {
		return fmt.Errorf("view update description readback mismatch")
	}
	return rt.Output(write)
}
func readExactParityView(rt *shortcut.RuntimeContext, id string) (map[string]any, error) {
	raw, err := rt.CallMCPData(serverMain, "get_views", map[string]any{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id"), "viewIds": []string{id}})
	if err != nil {
		return nil, err
	}
	rows, ok := findNamedObjectList(raw, "views")
	if !ok {
		return nil, fmt.Errorf("view directory missing")
	}
	var selected map[string]any
	for _, r := range rows {
		if stringValue(r, "viewId", "id") == id {
			for key, wanted := range map[string]string{"baseId": rt.Str("base-id"), "tableId": rt.Str("table-id")} {
				if actual, present := r[key]; present && actual != wanted {
					return nil, fmt.Errorf("view response %s mismatch", key)
				}
			}
			if selected != nil {
				return nil, fmt.Errorf("view ID is ambiguous")
			}
			selected = r
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("requested view ID not found")
	}
	return selected, nil
}

// A single leaf and AND([leaf]) have the same meaning. Do not collapse a
// multi-term OR into AND, drop conditions, or reorder comparison operands.
func canonicalParityViewFilter(value any) any {
	if list, ok := value.([]any); ok {
		if len(list) == 1 {
			return canonicalParityViewFilter(list[0])
		}
		children := make([]any, len(list))
		for i, v := range list {
			children[i] = canonicalParityViewFilter(v)
		}
		return map[string]any{"operator": "and", "operands": children}
	}
	m, ok := value.(map[string]any)
	if !ok {
		return value
	}
	op := stringValue(m, "operator")
	if op != "and" && op != "or" {
		return value
	}
	list, ok := m["operands"].([]any)
	if !ok {
		return value
	}
	if len(list) == 1 {
		return canonicalParityViewFilter(list[0])
	}
	copy := cloneAnyMap(m)
	children := make([]any, len(list))
	for i, v := range list {
		children[i] = canonicalParityViewFilter(v)
	}
	copy["operands"] = children
	return copy
}
