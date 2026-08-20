// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const viewPresetReadbackAttempts = 8

var viewPresetSleep = time.Sleep

var ViewPresetApply = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+view-preset-apply",
	Product:     serverMain,
	Description: "按视图精确名称幂等创建或更新预设，并读回校验类型和 config",
	Intent:      "当你要把 Grid/Kanban/Gantt/Calendar/Gallery 的固定筛选、排序、分组或可见列预设部署到一张表时使用；同名唯一则更新，无同名则创建。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: aitableCompositeContract(
		"+view-preset-apply",
		"按视图精确名称幂等创建或更新预设，并读回校验类型和 config",
		"当你要把 Grid/Kanban/Gantt/Calendar/Gallery 的固定筛选、排序、分组或可见列预设部署到一张表时使用；同名唯一则更新，无同名则创建。",
		"只做一次性新建可用 view create；同名视图不唯一或现有视图类型不同必须人工处理",
		`dws aitable +view-preset-apply --base-id B --table-id T --name "待处理" --view-type Grid --config '{"visibleFieldIds":["fld1"]}'`,
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "预设视图精确名称", Required: true},
		{Name: "view-type", Type: shortcut.FlagString, Desc: "视图类型", Required: true, Enum: []string{"Grid", "Kanban", "Gantt", "Calendar", "Gallery"}},
		{Name: "config", Type: shortcut.FlagString, Desc: "目标 config JSON 对象", Required: true},
	},
	Tips: []string{`dws aitable +view-preset-apply --base-id B --table-id T --name "待处理" --view-type Grid --config '{"visibleFieldIds":["fld1"]}'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeViewPresetApply(rt)
	},
}

func executeViewPresetApply(rt *shortcut.RuntimeContext) error {
	config, err := parseJSONObject("config", rt.Str("config"))
	if err != nil {
		return err
	}
	if len(config) == 0 {
		return apperrors.NewValidation("--config 必须是非空 JSON 对象")
	}
	baseID, tableID := rt.Str("base-id"), rt.Str("table-id")
	name, viewType := strings.TrimSpace(rt.Str("name")), rt.Str("view-type")
	preflight, err := rt.CallMCPData(serverMain, "get_views", map[string]any{"baseId": baseID, "tableId": tableID})
	if err != nil {
		return err
	}
	views, found := findNamedObjectList(preflight, "views", "viewList")
	if !found {
		return fmt.Errorf("get_views preflight is missing the views collection")
	}
	matches := viewsByExactName(views, name)
	if len(matches) > 1 {
		return apperrors.NewValidation(fmt.Sprintf("精确名称 %q 匹配到 %d 个视图，拒绝选择", name, len(matches)), apperrors.WithReason("target_ambiguous"), apperrors.WithExecutionStarted(false))
	}
	action, tool := "create", "create_view"
	params := map[string]any{"baseId": baseID, "tableId": tableID, "viewName": name, "viewType": viewType, "config": config}
	viewID := ""
	if len(matches) == 1 {
		action, tool = "update", "update_view"
		viewID = stringValue(matches[0], "viewId", "id")
		if viewID == "" {
			return fmt.Errorf("matched view is missing viewId")
		}
		actualType := stringValue(matches[0], "viewType", "type")
		if actualType != "" && actualType != viewType {
			return apperrors.NewValidation(fmt.Sprintf("同名视图类型为 %s，不能原地改为 %s", actualType, viewType), apperrors.WithReason("target_type_conflict"), apperrors.WithExecutionStarted(false))
		}
		params = map[string]any{"baseId": baseID, "tableId": tableID, "viewId": viewID, "newViewName": name, "config": config}
		if presetViewMatches(matches[0], viewType, config) {
			result := newCompositeResult("view_preset_apply")
			result.Status = "unchanged"
			result.Executed = false
			result.Resolved = map[string]any{"action": "unchanged", "viewId": viewID, "name": name}
			result.Verification = map[string]any{"status": "verified", "viewId": viewID}
			return rt.Output(result)
		}
	}
	result := newCompositeResult("view_preset_apply")
	result.Resolved = map[string]any{"action": action, "name": name, "viewId": viewID}
	result.Plan = []compositeStep{{Index: 1, Name: action + " view preset", Tool: tool, Status: "planned", Arguments: params}}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, tool, params)
	if action == "create" {
		viewID = findStringByKeys(writeData, "viewId")
	}
	var verifiedMatches []map[string]any
	var verifyErr error
	for attempt := 0; attempt < viewPresetReadbackAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			if backoff > 12*time.Second {
				backoff = 12 * time.Second
			}
			viewPresetSleep(backoff)
		}
		readBack, readErr := rt.CallMCPData(serverMain, "get_views", map[string]any{"baseId": baseID, "tableId": tableID})
		verifyErr = readErr
		verifiedViews, found := findNamedObjectList(readBack, "views", "viewList")
		if verifyErr == nil && !found {
			verifyErr = fmt.Errorf("get_views read-back is missing the views collection")
		}
		verifiedMatches = viewsByExactName(verifiedViews, name)
		if verifyErr == nil && len(verifiedMatches) != 1 {
			verifyErr = fmt.Errorf("view read-back matched %d exact-name views, want 1", len(verifiedMatches))
			if len(verifiedMatches) > 1 {
				break
			}
		}
		if verifyErr == nil {
			actualID := stringValue(verifiedMatches[0], "viewId", "id")
			if actualID == "" || (viewID != "" && actualID != viewID) {
				verifyErr = fmt.Errorf("view read-back identity mismatch: got %q, response %q", actualID, viewID)
			} else if !presetViewMatches(verifiedMatches[0], viewType, config) {
				verifyErr = fmt.Errorf("view read-back does not contain the declared type/config")
			} else {
				viewID = actualID
				break
			}
		}
	}
	if verifyErr != nil {
		result.Status = "unknown"
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": tool, "viewId": viewID, "name": name})
		if writeErr != nil {
			result.Warnings = append(result.Warnings, "write response error: "+writeErr.Error())
		}
		return compositeError(result, verifyErr, action == "update")
	}
	result.Resolved["viewId"] = viewID
	result.CompletedCount = 1
	result.Verification = map[string]any{"status": "verified", "viewId": viewID, "viewType": viewType}
	result.Result = map[string]any{"action": action, "viewId": viewID, "view": verifiedMatches[0]}
	if writeErr != nil {
		result.Status = "recovered"
		result.Warnings = append(result.Warnings, "write response was an error, but the exact view preset was proven by read-back")
	}
	return rt.Output(result)
}

func viewsByExactName(views []map[string]any, name string) []map[string]any {
	out := make([]map[string]any, 0)
	for _, view := range views {
		if stringValue(view, "viewName", "name", "title") == name {
			out = append(out, view)
		}
	}
	return out
}

func presetViewMatches(view map[string]any, viewType string, config map[string]any) bool {
	actualType := stringValue(view, "viewType", "type")
	if actualType != "" && actualType != viewType {
		return false
	}
	actualConfig := make(map[string]any, len(config))
	if nested, ok := view["config"].(map[string]any); ok {
		for key, value := range nested {
			actualConfig[key] = value
		}
	}
	for key := range config {
		if value, ok := view[key]; ok {
			actualConfig[key] = value
		}
	}
	if _, wanted := config["visibleFieldIds"]; wanted {
		if visible, ok := projectedVisibleFieldIDs(view); ok {
			actualConfig["visibleFieldIds"] = visible
		}
	}
	return mapContains(actualConfig, config)
}

// get_views projects visible fields as columns plus hiddenFields instead of
// echoing create_view's config.visibleFieldIds input. Different deployments
// return hiddenFields either as a fieldId-keyed object or a parallel array.
func projectedVisibleFieldIDs(view map[string]any) ([]any, bool) {
	columns, columnsOK := view["columns"].([]any)
	custom, customOK := view["custom"].(map[string]any)
	if !columnsOK || !customOK {
		return nil, false
	}
	if hidden, ok := custom["hiddenFields"].(map[string]any); ok {
		visible := make([]any, 0, len(columns))
		for _, column := range columns {
			fieldID, fieldOK := column.(string)
			if !fieldOK {
				return nil, false
			}
			isHidden, exists := hidden[fieldID]
			if !exists {
				return nil, false
			}
			hiddenFlag, hiddenTypeOK := isHidden.(bool)
			if !hiddenTypeOK {
				return nil, false
			}
			if !hiddenFlag {
				visible = append(visible, fieldID)
			}
		}
		return visible, true
	}
	hidden, hiddenOK := custom["hiddenFields"].([]any)
	if !hiddenOK || len(columns) != len(hidden) {
		return nil, false
	}
	visible := make([]any, 0, len(columns))
	for index, column := range columns {
		fieldID, fieldOK := column.(string)
		isHidden, hiddenTypeOK := hidden[index].(bool)
		if !fieldOK || !hiddenTypeOK {
			return nil, false
		}
		if !isHidden {
			visible = append(visible, fieldID)
		}
	}
	return visible, true
}

func mapContains(actual, expected map[string]any) bool {
	for key, want := range expected {
		got, exists := actual[key]
		if !exists {
			return false
		}
		wantMap, wantIsMap := want.(map[string]any)
		gotMap, gotIsMap := got.(map[string]any)
		if wantIsMap {
			if !gotIsMap || !mapContains(gotMap, wantMap) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			return false
		}
	}
	return true
}
