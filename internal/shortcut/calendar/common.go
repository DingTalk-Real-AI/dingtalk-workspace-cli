// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package calendar

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const calendarCompositeReason = "Reviewed Calendar Shortcut composite: the executable CLI owns strict response validation, truthful pagination, optional multi-step writes, read-back verification, and confirmation; no single MCP interface represents the complete command contract."

func calendarContract(command, description, useWhen string, avoidWhen, examples []string, result *contract.ResultSpec, pagination *contract.PaginationSpec, params ...contract.ParamDecl) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "calendar " + command
	return corecmd.ContractDecl{
		Description: description,
		Parameters:  params,
		Result:      result,
		Pagination:  pagination,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       calendarCompositeReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    avoidWhen,
			Examples:     examples,
		},
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           name,
			CanonicalPath:  "calendar." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
	}
}

func calendarObjectResult(description string, outcomes ...contract.ResultOutcome) *contract.ResultSpec {
	if len(outcomes) == 0 {
		outcomes = []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}
	}
	return &contract.ResultSpec{
		Outcomes: outcomes,
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"additionalProperties":true}`,
			description,
		)),
	}
}

func calendarCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"本页有效业务记录数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"Calendar 业务条目","additionalProperties":true}},"complete":{"type":"boolean","description":"服务端分页证据是否证明结果已完整"}},"required":["count",%q,"complete"],"additionalProperties":true}`,
			description, collection, description, collection,
		)),
	}
}

func calendarCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func calendarReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func calendarWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"}
}

func calendarResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func requireCalendarResponse(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, calendarResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明操作成功或结果确实为空")
	}
	for _, candidate := range calendarContainers(data) {
		value, present := candidate["success"]
		if !present {
			continue
		}
		success, ok := value.(bool)
		if !ok {
			return nil, calendarResponseError(operation, "malformed_success", "响应 success 字段不是布尔值")
		}
		if !success {
			message := firstCalendarString(candidate, "errorMsg", "errorMessage", "message", "error")
			if message == "" {
				message = "服务明确返回 success=false"
			}
			return nil, calendarResponseError(operation, "remote_failure", message)
		}
	}
	return data, nil
}

func requireCalendarWriteResponse(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireCalendarResponse(data, operation)
	if err != nil {
		return nil, err
	}
	for _, candidate := range calendarContainers(data) {
		if success, ok := candidate["success"].(bool); ok && success {
			return data, nil
		}
	}
	return nil, calendarResponseError(operation, "missing_terminal_success", "写操作响应没有 success=true 终态证据；远端效果未知")
}

func calendarContainers(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	containers := []map[string]any{data}
	seen := map[mapIdentity]bool{}
	for depth, index := 0, 0; index < len(containers) && depth < 12; index, depth = index+1, depth+1 {
		candidate := containers[index]
		for _, key := range []string{"content", "result", "data", "event"} {
			if nested, ok := candidate[key].(map[string]any); ok {
				identity := mapIdentity{parent: index, key: key}
				if !seen[identity] {
					seen[identity] = true
					containers = append(containers, nested)
				}
			}
		}
	}
	return containers
}

type mapIdentity struct {
	parent int
	key    string
}

func requireCalendarObject(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireCalendarResponse(data, operation)
	if err != nil {
		return nil, err
	}
	containers := calendarContainers(data)
	for index := len(containers) - 1; index >= 0; index-- {
		candidate := containers[index]
		if len(candidate) == 0 || calendarOnlyEnvelopeFields(candidate) {
			continue
		}
		return candidate, nil
	}
	return nil, calendarResponseError(operation, "missing_business_result", "响应没有可验证的业务对象")
}

func calendarOnlyEnvelopeFields(candidate map[string]any) bool {
	for key := range candidate {
		switch key {
		case "success", "result", "data", "content", "message", "errorMsg", "errorMessage":
		default:
			return false
		}
	}
	return true
}

func requireCalendarCollection(data map[string]any, operation string, keys ...string) ([]any, map[string]any, error) {
	data, err := requireCalendarResponse(data, operation)
	if err != nil {
		return nil, nil, err
	}
	for _, container := range calendarContainers(data) {
		for _, key := range keys {
			value, present := container[key]
			if !present {
				continue
			}
			items, ok := value.([]any)
			if !ok {
				return nil, nil, calendarResponseError(operation, "malformed_collection", fmt.Sprintf("响应 %s 字段不是数组", key))
			}
			if err := validateCalendarCollectionItems(items, operation, key); err != nil {
				return nil, nil, err
			}
			return items, container, nil
		}
	}
	for _, wrapper := range []string{"result", "data"} {
		if value, present := data[wrapper]; present {
			if items, ok := value.([]any); ok {
				if err := validateCalendarCollectionItems(items, operation, wrapper); err != nil {
					return nil, nil, err
				}
				return items, data, nil
			}
		}
	}
	return nil, nil, calendarResponseError(operation, "missing_collection", "响应缺少声明的业务数组；不能把缺字段、内部错误或协议漂移投影成空结果")
}

func validateCalendarCollectionItems(items []any, operation, key string) error {
	for index, item := range items {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			return calendarResponseError(operation, "malformed_collection_item", fmt.Sprintf("响应 %s[%d] 不是非空对象", key, index))
		}
	}
	return nil
}

func projectCalendarRows(items []any, operation string, aliases map[string][]string, requiredAny ...string) ([]map[string]any, error) {
	rows := make([]map[string]any, 0, len(items))
	for index, item := range items {
		source := item.(map[string]any)
		row := make(map[string]any)
		for canonical, candidates := range aliases {
			for _, candidate := range candidates {
				if value, ok := source[candidate]; ok && value != nil {
					row[canonical] = value
					break
				}
			}
		}
		valid := len(requiredAny) == 0 && len(row) > 0
		for _, required := range requiredAny {
			if value, ok := row[required]; ok && !calendarEmptyValue(value) {
				valid = true
				break
			}
		}
		if !valid {
			return nil, calendarResponseError(operation, "malformed_collection_item", fmt.Sprintf("业务数组第 %d 项缺少可识别的标识或时间字段", index))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func calendarEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

type calendarPageEvidence struct {
	Known      bool
	HasMore    bool
	NextCursor string
}

func calendarPagination(container map[string]any, operation string) (calendarPageEvidence, error) {
	page := calendarPageEvidence{}
	if container == nil {
		return page, nil
	}
	if raw, present := container["hasMore"]; present {
		value, ok := raw.(bool)
		if !ok {
			return page, calendarResponseError(operation, "malformed_pagination", "响应 hasMore 字段不是布尔值")
		}
		page.Known = true
		page.HasMore = value
	}
	for _, key := range []string{"nextCursor", "nextToken", "pageToken"} {
		raw, present := container[key]
		if !present || raw == nil {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return page, calendarResponseError(operation, "malformed_pagination", fmt.Sprintf("响应 %s 字段不是字符串", key))
		}
		page.Known = true
		page.NextCursor = strings.TrimSpace(value)
		break
	}
	if page.HasMore && page.NextCursor == "" {
		return page, calendarResponseError(operation, "missing_next_cursor", "服务端返回 hasMore=true 但没有可续用的 nextCursor，结果不完整")
	}
	if page.Known && !page.HasMore && page.NextCursor != "" {
		return page, calendarResponseError(operation, "inconsistent_pagination", "服务端返回 hasMore=false 但仍携带非空 nextCursor")
	}
	if page.NextCursor != "" {
		page.HasMore = true
	}
	return page, nil
}

func addCalendarPagination(out map[string]any, page calendarPageEvidence) {
	out["complete"] = page.Known && !page.HasMore
	if page.Known {
		out["hasMore"] = page.HasMore
	}
	if page.NextCursor != "" {
		out["nextCursor"] = page.NextCursor
	}
}

// outputCalendarPage keeps legacy JSON compatibility while honoring the
// unified Result contract: cursor controls move to meta.pagination and are not
// duplicated into business data.
func outputCalendarPage(rt *shortcut.RuntimeContext, payload map[string]any, page calendarPageEvidence) error {
	if !page.Known {
		return calendarResponseError("calendar/pagination", "missing_pagination", "响应缺少分页终态证据")
	}
	addCalendarPagination(payload, page)
	if !output.UsesUnifiedResult(rt.Command()) {
		return rt.Output(payload)
	}
	pagination, err := output.NewPagination(!page.HasMore, page.NextCursor)
	if err != nil {
		return calendarResponseError("calendar/pagination", "invalid_pagination", err.Error())
	}
	business := make(map[string]any, len(payload))
	for key, value := range payload {
		switch key {
		case "hasMore", "nextCursor":
		default:
			business[key] = value
		}
	}
	meta := &output.Meta{Pagination: pagination}
	if count, ok := payload["count"].(int); ok {
		meta.Count = output.NewCount(count)
	}
	return output.StoreResult(rt.Command().Context(), output.Success(business, output.WithMeta(meta)))
}

func firstCalendarString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nestedCalendarString(data map[string]any, keys ...string) string {
	for _, candidate := range calendarContainers(data) {
		if value := firstCalendarString(candidate, keys...); value != "" {
			return value
		}
	}
	return ""
}

func validateCalendarRange(startName, startValue, endName, endValue string) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(startValue))
	if err != nil {
		return time.Time{}, time.Time{}, apperrors.NewValidation(fmt.Sprintf("--%s 时间格式无效，请使用 RFC3339/ISO-8601", startName))
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(endValue))
	if err != nil {
		return time.Time{}, time.Time{}, apperrors.NewValidation(fmt.Sprintf("--%s 时间格式无效，请使用 RFC3339/ISO-8601", endName))
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, apperrors.NewValidation(fmt.Sprintf("--%s 必须晚于 --%s", endName, startName))
	}
	return start, end, nil
}

func calendarStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func calendarEventID(event map[string]any) string {
	return firstCalendarString(event, "eventId", "event_id", "id")
}

func calendarTimeEquivalent(actual any, expected string) bool {
	want, err := time.Parse(time.RFC3339, expected)
	if err != nil {
		return false
	}
	if object, ok := actual.(map[string]any); ok {
		for _, key := range []string{"dateTime", "date_time", "value", "timestamp"} {
			if nested, exists := object[key]; exists {
				return calendarTimeEquivalent(nested, expected)
			}
		}
	}
	switch value := actual.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
			return parsed.Equal(want)
		}
		millis, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return err == nil && time.UnixMilli(millis).Equal(want)
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0) && value == math.Trunc(value) && time.UnixMilli(int64(value)).Equal(want)
	case int64:
		return time.UnixMilli(value).Equal(want)
	case int:
		return time.UnixMilli(int64(value)).Equal(want)
	default:
		return false
	}
}

func verifyCalendarEvent(event map[string]any, eventID string, requested map[string]any) error {
	if got := calendarEventID(event); got == "" || got != eventID {
		return calendarResponseError("calendar/get_calendar_detail", "readback_id_mismatch", "写后读回的 eventId 缺失或不一致")
	}
	aliases := map[string][]string{
		"summary":       {"summary", "title", "name"},
		"description":   {"description", "desc"},
		"startDateTime": {"startDateTime", "start_time", "startTime", "start"},
		"endDateTime":   {"endDateTime", "end_time", "endTime", "end"},
		"timeZone":      {"timeZone", "timezone", "time_zone"},
		"location":      {"location", "locationName", "location_name"},
		"freeBusy":      {"freeBusy", "free_busy"},
	}
	for property, expected := range requested {
		candidates := aliases[property]
		if len(candidates) == 0 {
			continue
		}
		var actual any
		var present bool
		for _, candidate := range candidates {
			actual, present = event[candidate]
			if present {
				break
			}
		}
		if !present {
			return calendarResponseError("calendar/get_calendar_detail", "readback_field_missing", fmt.Sprintf("写后读回缺少字段 %s", property))
		}
		if property == "startDateTime" || property == "endDateTime" {
			if !calendarTimeEquivalent(actual, fmt.Sprint(expected)) {
				return calendarResponseError("calendar/get_calendar_detail", "readback_field_mismatch", fmt.Sprintf("写后读回字段 %s 与请求不一致", property))
			}
			continue
		}
		if strings.TrimSpace(fmt.Sprint(actual)) != strings.TrimSpace(fmt.Sprint(expected)) {
			return calendarResponseError("calendar/get_calendar_detail", "readback_field_mismatch", fmt.Sprintf("写后读回字段 %s 与请求不一致", property))
		}
	}
	return nil
}

func finalizeCalendarShortcut(item *shortcut.Shortcut, result *contract.ResultSpec, pagination *contract.PaginationSpec) {
	item.OutputRollout = output.RolloutUnifiedActive
	item.Contract.Result = result
	item.Contract.Pagination = pagination
	item.Contract.Selection.AgentSummary = item.Description
	item.Contract.Selection.UseWhen = []string{item.Intent}
	if item.Contract.Interface == nil {
		item.Contract.Interface = &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: calendarCompositeReason}
	}
}
