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
	Description: "按视图精确名称幂等创建或更新预设；Gantt 可用独立 timebar 完成专用两步写入",
	Intent:      "当你要部署 Grid/Kanban/Gantt/Calendar/Gallery 预设时使用；同名唯一则更新，无同名则创建，Gantt 传 --timebar 时另行写入并回读时间条。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent",
	},
	Contract: aitableCompositeContract(
		"+view-preset-apply",
		"按视图精确名称幂等创建或更新预设；Gantt 可用独立 timebar 完成专用两步写入",
		"当你要部署 Grid/Kanban/Gantt/Calendar/Gallery 预设时使用；同名唯一则更新，无同名则创建，Gantt 传 --timebar 时另行写入并回读时间条。",
		"只做一次性新建可用 view create；同名视图不唯一或现有视图类型不同必须人工处理",
		`dws aitable +view-preset-apply --base-id B --table-id T --name "待处理" --view-type Grid --config '{"visibleFieldIds":["fld1"]}'`,
	),
	Flags: []shortcut.Flag{
		{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true},
		{Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "预设视图精确名称", Required: true},
		{Name: "view-type", Type: shortcut.FlagString, Desc: "视图类型", Required: true, Enum: []string{"Grid", "Kanban", "Gantt", "Calendar", "Gallery"}},
		{Name: "config", Type: shortcut.FlagString, Desc: "目标 config JSON 对象", Required: true},
		{Name: "timebar", Type: shortcut.FlagString, Desc: "Gantt 专用 ganttTimebar JSON 对象；与通用 config 分两步写入"},
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
	if _, exists := config["ganttTimebar"]; exists {
		return apperrors.NewValidation("ganttTimebar 不能放入通用 --config；Gantt 请使用独立 --timebar")
	}
	baseID, tableID := rt.Str("base-id"), rt.Str("table-id")
	name, viewType := strings.TrimSpace(rt.Str("name")), rt.Str("view-type")
	if name == "" {
		return apperrors.NewValidation("--name 不能为空")
	}
	var timebar map[string]any
	if rt.Changed("timebar") {
		if viewType != "Gantt" {
			return apperrors.NewValidation("--timebar 仅适用于 --view-type Gantt")
		}
		timebar, err = parseJSONObject("timebar", rt.Str("timebar"))
		if err != nil {
			return err
		}
		if len(timebar) == 0 || strings.TrimSpace(stringValue(timebar, "startField")) == "" {
			return apperrors.NewValidation("--timebar 必须是包含非空 startField 的 JSON 对象")
		}
	}
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
		if presetViewMatches(matches[0], viewType, config) && (timebar == nil || viewTimebarMatches(matches[0], timebar)) {
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
	if timebar != nil {
		result.Plan = append(result.Plan, compositeStep{Index: 2, Name: "update and verify Gantt timebar", Tool: "update_view", Status: "planned", Arguments: map[string]any{"ganttTimebar": timebar}})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	writeData, writeErr := rt.CallMCPWriteDataStrict(serverMain, tool, params)
	if action == "create" {
		viewID = findStringByKeys(writeData, "viewId")
		if viewID != "" {
			result.Resolved["viewId"] = viewID
		}
	}
	var verifiedMatches []map[string]any
	var verifyErr error
	for attempt := 0; attempt < viewPresetReadbackAttempts; attempt++ {
		waitViewPresetReadback(attempt)
		verifiedMatches, verifyErr = readBackViewPreset(rt, baseID, tableID, name)
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
			} else {
				viewID = actualID
				result.Resolved["viewId"] = viewID
			}
			if verifyErr == nil && !presetViewMatches(verifiedMatches[0], viewType, config) {
				verifyErr = fmt.Errorf("view read-back does not contain the declared type/config")
			} else if verifyErr == nil {
				break
			}
		}
	}
	if verifyErr != nil {
		effectConfirmed := (action == "create" && viewID != "") || (action == "update" && writeErr == nil)
		if effectConfirmed {
			result.Status = "partial_success"
			result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": tool, "viewId": viewID, "name": name})
		} else {
			result.Status = "unknown"
		}
		if writeErr != nil {
			result.Warnings = append(result.Warnings, "write response error: "+writeErr.Error())
		}
		return compositeError(result, verifyErr, action == "update")
	}
	result.Resolved["viewId"] = viewID
	result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 1, Name: action + " view preset", Tool: tool, Status: "completed", Result: verifiedMatches[0]})
	if timebar != nil {
		// ganttTimebar is a separate live MCP mutation. Once it is called, an
		// unresolved read-back must not make the whole preset blindly retryable.
		_, timebarWriteErr := rt.CallMCPWriteDataStrict(serverMain, "update_view", map[string]any{
			"baseId": baseID, "tableId": tableID, "viewId": viewID,
			"config": map[string]any{"ganttTimebar": timebar},
		})
		var timebarVerifyErr error
		for attempt := 0; attempt < viewPresetReadbackAttempts; attempt++ {
			waitViewPresetReadback(attempt)
			verifiedMatches, timebarVerifyErr = readBackViewPreset(rt, baseID, tableID, name)
			if timebarVerifyErr == nil && len(verifiedMatches) == 1 && stringValue(verifiedMatches[0], "viewId", "id") == viewID && viewTimebarMatches(verifiedMatches[0], timebar) {
				timebarVerifyErr = nil
				break
			}
			if timebarVerifyErr == nil {
				timebarVerifyErr = fmt.Errorf("gantt timebar read-back does not match the requested configuration")
			}
		}
		if timebarVerifyErr != nil {
			result.Status = "partial_success"
			if timebarWriteErr == nil {
				result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "update_view", "viewId": viewID, "configKey": "ganttTimebar"})
			}
			if timebarWriteErr != nil {
				result.Warnings = append(result.Warnings, "timebar write response error: "+timebarWriteErr.Error())
			}
			result.Checkpoint = map[string]any{"viewId": viewID, "nextStep": "read and verify Gantt timebar before retrying"}
			return compositeError(result, timebarVerifyErr, false)
		}
		result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 2, Name: "update and verify Gantt timebar", Tool: "update_view", Status: "completed", Result: timebar})
		if timebarWriteErr != nil {
			result.Status = "recovered"
			result.Warnings = append(result.Warnings, "timebar write response was an error, but the requested timebar was proven by read-back")
		}
	}
	result.CompletedCount = len(result.CompletedSteps)
	result.Verification = map[string]any{"status": "verified", "viewId": viewID, "viewType": viewType, "timebarVerified": timebar != nil}
	result.Result = map[string]any{"action": action, "viewId": viewID, "view": verifiedMatches[0]}
	if writeErr != nil {
		result.Status = "recovered"
		result.Warnings = append(result.Warnings, "write response was an error, but the exact view preset was proven by read-back")
	}
	return rt.Output(result)
}

// waitViewPresetReadback applies the bounded delay shared by both verification phases.
func waitViewPresetReadback(attempt int) {
	if attempt <= 0 {
		return
	}
	backoff := time.Duration(1<<(attempt-1)) * time.Second
	if backoff > 12*time.Second {
		backoff = 12 * time.Second
	}
	viewPresetSleep(backoff)
}

// readBackViewPreset loads the current views and returns exact-name matches.
func readBackViewPreset(rt *shortcut.RuntimeContext, baseID, tableID, name string) ([]map[string]any, error) {
	readBack, err := rt.CallMCPData(serverMain, "get_views", map[string]any{"baseId": baseID, "tableId": tableID})
	if err != nil {
		return nil, err
	}
	views, found := findNamedObjectList(readBack, "views", "viewList")
	if !found {
		return nil, fmt.Errorf("get_views read-back is missing the views collection")
	}
	return viewsByExactName(views, name), nil
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
	expected := normalizePresetViewConfig(config)
	actual := normalizePresetViewConfig(actualConfig)
	for _, key := range []string{"filter", "sort", "group"} {
		want, wanted := expected[key]
		if !wanted {
			continue
		}
		if _, exists := actual[key]; !exists && emptyPresetViewConfigValue(key, want) {
			actual[key] = want
		}
	}
	return mapContains(actual, expected)
}

func normalizePresetViewConfig(config map[string]any) map[string]any {
	out := make(map[string]any, len(config))
	for key, value := range config {
		switch key {
		case "filter":
			out[key] = normalizePresetViewFilter(value)
		case "sort", "group":
			if value == nil {
				out[key] = []any{}
			} else {
				out[key] = value
			}
		default:
			out[key] = value
		}
	}
	return out
}

func normalizePresetViewFilter(value any) any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{"operator": "and", "operands": []any{}}
	case []any:
		if len(typed) == 0 {
			return map[string]any{"operator": "and", "operands": []any{}}
		}
		if len(typed) == 1 {
			if group, ok := typed[0].(map[string]any); ok {
				return group
			}
		}
	}
	return value
}

func emptyPresetViewConfigValue(key string, value any) bool {
	switch key {
	case "sort", "group":
		items, ok := value.([]any)
		return ok && len(items) == 0
	case "filter":
		group, ok := value.(map[string]any)
		if !ok || !strings.EqualFold(stringValue(group, "operator"), "and") {
			return false
		}
		operands, ok := group["operands"].([]any)
		return ok && len(operands) == 0
	default:
		return false
	}
}

func viewTimebarMatches(view map[string]any, expected map[string]any) bool {
	for _, container := range []map[string]any{view, objectValue(view, "config"), objectValue(view, "custom")} {
		if actual, ok := container["ganttTimebar"].(map[string]any); ok && mapContains(actual, expected) {
			return true
		}
	}
	return false
}

func objectValue(value map[string]any, key string) map[string]any {
	object, _ := value[key].(map[string]any)
	return object
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
