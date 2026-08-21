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

package smart

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/calendarcompat"
)

const calendarSmartMaxPages = 20

// calendarDayRange returns the [00:00, next-day 00:00) window for "today plus
// offsetDays" in the local timezone. Shared by the day-scoped calendar
// shortcuts (+today offset 0, +tomorrow offset 1, +conflicts --in-days).
func calendarDayRange(offsetDays int) (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, offsetDays)
	return start, start.AddDate(0, 0, 1)
}

// calendarProjectEvents lists and projects the events in a list_calendar_events
// response into clean {title,start,end,location,eventId} items. The strict page
// parser has already required stable ids and valid time boundaries.
func calendarProjectEvents(raw []map[string]any) []map[string]any {
	events := make([]map[string]any, 0)
	for _, e := range raw {
		events = append(events, shortcutNextEventProject(e))
	}
	return events
}

func calendarSmartResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"additionalProperties":true}`,
			description,
		)),
	}
}

func finalizeCalendarSmart(item *shortcut.Shortcut, description string) {
	item.OutputRollout = output.RolloutUnifiedActive
	item.Contract.Result = calendarSmartResult(description)
}

func calendarSmartError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

func calendarSmartRequireSuccess(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, calendarSmartError(operation, "empty_tool_response", "服务返回空响应，无法证明成功或合法空结果")
	}
	raw, present := data["success"]
	if !present {
		return nil, calendarSmartError(operation, "missing_success", "响应缺少 success 终态字段")
	}
	success, ok := raw.(bool)
	if !ok {
		return nil, calendarSmartError(operation, "malformed_success", "响应 success 字段不是布尔值")
	}
	if !success {
		message := calendarSmartFirstString(data, "errorMsg", "errorMessage", "message", "error")
		if message == "" {
			message = "服务明确返回 success=false"
		}
		return nil, calendarSmartError(operation, "remote_failure", message)
	}
	return data, nil
}

func calendarSmartRequireEvent(data map[string]any, operation, expectedID string) (map[string]any, error) {
	data, err := calendarSmartRequireSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	event, ok := data["result"].(map[string]any)
	if !ok || len(event) == 0 {
		return nil, calendarSmartError(operation, "missing_event", "响应 result 不是非空日程对象")
	}
	id := calendarSmartFirstString(event, "id", "eventId", "event_id")
	if id == "" {
		return nil, calendarSmartError(operation, "missing_event_id", "日程对象缺少稳定 id")
	}
	if expectedID != "" && id != expectedID {
		return nil, calendarSmartError(operation, "event_id_mismatch", "读回日程 id 与请求 eventId 不一致")
	}
	return event, nil
}

func calendarSmartFirstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func calendarSmartEventID(data map[string]any) string {
	if value := calendarSmartFirstString(data, "id", "eventId", "event_id"); value != "" {
		return value
	}
	for _, key := range []string{"result", "event", "data"} {
		if nested, ok := data[key].(map[string]any); ok {
			if value := calendarSmartFirstString(nested, "id", "eventId", "event_id"); value != "" {
				return value
			}
		}
	}
	return ""
}

func calendarSmartListAll(rt *shortcut.RuntimeContext, params map[string]any) ([]map[string]any, error) {
	all := make([]map[string]any, 0)
	seenIDs := map[string]bool{}
	cursor := ""
	for page := 1; page <= calendarSmartMaxPages; page++ {
		request := make(map[string]any, len(params)+2)
		for key, value := range params {
			request[key] = value
		}
		request["limit"] = 100
		if cursor != "" {
			request["cursor"] = cursor
		}
		data, err := rt.CallMCPData("calendar", "list_calendar_events", request)
		if err != nil {
			return nil, err
		}
		items, hasMore, next, err := calendarSmartEventPage(data)
		if err != nil {
			return nil, err
		}
		for _, event := range items {
			id := calendarSmartFirstString(event, "id", "eventId", "event_id")
			if !seenIDs[id] {
				seenIDs[id] = true
				all = append(all, event)
			}
		}
		if !hasMore {
			return all, nil
		}
		if next == cursor {
			return nil, calendarSmartError("calendar/list_calendar_events", "stalled_cursor", "服务端 nextCursor 未前进，不能证明列表完整")
		}
		cursor = next
	}
	return nil, calendarSmartError("calendar/list_calendar_events", "page_limit", fmt.Sprintf("自动翻页达到 %d 页上限，拒绝基于不完整事件集给出结论", calendarSmartMaxPages))
}

func calendarSmartEventPage(data map[string]any) ([]map[string]any, bool, string, error) {
	data, err := calendarSmartRequireSuccess(data, "calendar/list_calendar_events")
	if err != nil {
		return nil, false, "", err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "missing_result", "响应 result 不是对象")
	}
	rawEvents, present := result["events"]
	if !present {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "missing_events", "响应缺少显式 events 数组")
	}
	list, ok := rawEvents.([]any)
	if !ok {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "malformed_events", "响应 events 字段不是数组")
	}
	rawHasMore, present := result["hasMore"]
	if !present {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "missing_pagination", "响应缺少 hasMore 分页终态")
	}
	hasMore, ok := rawHasMore.(bool)
	if !ok {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "malformed_pagination", "响应 hasMore 字段不是布尔值")
	}
	next := ""
	if raw, present := result["nextCursor"]; present && raw != nil {
		var ok bool
		next, ok = raw.(string)
		if !ok {
			return nil, false, "", calendarSmartError("calendar/list_calendar_events", "malformed_pagination", "响应 nextCursor 字段不是字符串")
		}
		next = strings.TrimSpace(next)
	}
	if hasMore && next == "" {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "missing_next_cursor", "服务端返回 hasMore=true 但 nextCursor 为空")
	}
	if !hasMore && next != "" {
		return nil, false, "", calendarSmartError("calendar/list_calendar_events", "inconsistent_pagination", "服务端返回 hasMore=false 但 nextCursor 非空")
	}
	list, _ = calendarcompat.NormalizeTerminalEmptyEvents(list, result)
	events := make([]map[string]any, 0, len(list))
	for index, item := range list {
		event, ok := item.(map[string]any)
		if !ok || len(event) == 0 {
			return nil, false, "", calendarSmartError("calendar/list_calendar_events", "malformed_event", fmt.Sprintf("events[%d] 不是非空对象", index))
		}
		if calendarSmartFirstString(event, "id", "eventId", "event_id") == "" {
			return nil, false, "", calendarSmartError("calendar/list_calendar_events", "missing_event_id", fmt.Sprintf("events[%d] 缺少稳定 id", index))
		}
		if _, ok := shortcutNextEventStart(event); !ok {
			return nil, false, "", calendarSmartError("calendar/list_calendar_events", "malformed_event_start", fmt.Sprintf("events[%d] 缺少合法开始时间", index))
		}
		if _, ok := conflictsEndTime(event); !ok {
			return nil, false, "", calendarSmartError("calendar/list_calendar_events", "malformed_event_end", fmt.Sprintf("events[%d] 缺少合法结束时间", index))
		}
		events = append(events, event)
	}
	return events, hasMore, next, nil
}

func calendarSmartBusySlots(data map[string]any) ([]map[string]any, error) {
	data, err := calendarSmartRequireSuccess(data, "calendar/query_busy_status")
	if err != nil {
		return nil, err
	}
	entries, ok := data["result"].([]any)
	if !ok {
		return nil, calendarSmartError("calendar/query_busy_status", "missing_result", "响应 result 不是显式忙闲数组")
	}
	out := make([]map[string]any, 0)
	for entryIndex, entry := range entries {
		object, ok := entry.(map[string]any)
		if !ok || len(object) == 0 {
			return nil, calendarSmartError("calendar/query_busy_status", "malformed_result_item", fmt.Sprintf("result[%d] 不是非空对象", entryIndex))
		}
		rawItems, present := object["scheduleItems"]
		if !present {
			return nil, calendarSmartError("calendar/query_busy_status", "missing_schedule_items", fmt.Sprintf("result[%d] 缺少 scheduleItems", entryIndex))
		}
		items, ok := rawItems.([]any)
		if !ok {
			return nil, calendarSmartError("calendar/query_busy_status", "malformed_schedule_items", fmt.Sprintf("result[%d].scheduleItems 不是数组", entryIndex))
		}
		for itemIndex, item := range items {
			schedule, ok := item.(map[string]any)
			if !ok || len(schedule) == 0 {
				return nil, calendarSmartError("calendar/query_busy_status", "malformed_schedule_item", fmt.Sprintf("scheduleItems[%d] 不是非空对象", itemIndex))
			}
			start := freebusyDateTime(schedule["start"])
			end := freebusyDateTime(schedule["end"])
			if start == nil || end == nil || strings.TrimSpace(fmt.Sprint(start)) == "" || strings.TrimSpace(fmt.Sprint(end)) == "" {
				return nil, calendarSmartError("calendar/query_busy_status", "malformed_schedule_time", "忙闲条目缺少开始或结束时间")
			}
			out = append(out, map[string]any{"start": start, "end": end})
		}
	}
	return out, nil
}

func calendarSmartSuggestedSlots(data map[string]any) ([]map[string]any, error) {
	data, err := calendarSmartRequireSuccess(data, "calendar/list_suggested_event_times")
	if err != nil {
		return nil, err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return nil, calendarSmartError("calendar/list_suggested_event_times", "missing_result", "响应 result 不是对象")
	}
	rawSlots, present := result["recommendEventTimes"]
	if !present {
		return nil, calendarSmartError("calendar/list_suggested_event_times", "missing_suggestions", "响应缺少显式 recommendEventTimes 数组")
	}
	slots, ok := rawSlots.([]any)
	if !ok {
		return nil, calendarSmartError("calendar/list_suggested_event_times", "malformed_suggestions", "recommendEventTimes 不是数组")
	}
	out := make([]map[string]any, 0, len(slots))
	for index, item := range slots {
		slot, ok := item.(map[string]any)
		if !ok || len(slot) == 0 {
			return nil, calendarSmartError("calendar/list_suggested_event_times", "malformed_suggestion", fmt.Sprintf("recommendEventTimes[%d] 不是非空对象", index))
		}
		start := slot["startTime"]
		end := slot["endTime"]
		if start == nil || end == nil {
			return nil, calendarSmartError("calendar/list_suggested_event_times", "malformed_suggestion_time", fmt.Sprintf("recommendEventTimes[%d] 缺少 startTime/endTime", index))
		}
		row := map[string]any{"start": start, "end": end}
		if raw, ok := slot["timeConflictAttendees"].([]any); ok {
			conflicts := make([]any, 0, len(raw))
			for _, value := range raw {
				if value != nil {
					conflicts = append(conflicts, value)
				}
			}
			if len(conflicts) > 0 {
				row["conflicts"] = conflicts
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func calendarSmartAttendees(data map[string]any) (map[string]bool, error) {
	data, err := calendarSmartRequireSuccess(data, "calendar/get_calendar_participants")
	if err != nil {
		return nil, err
	}
	var raw any
	result, present := data["result"]
	if !present {
		return nil, calendarSmartError("calendar/get_calendar_participants", "missing_result", "参会人响应缺少 result")
	}
	switch typed := result.(type) {
	case []any:
		raw = typed
	case map[string]any:
		found := false
		for _, key := range []string{"attendees", "participants", "items", "list"} {
			if raw, found = typed[key]; found {
				break
			}
		}
		if !found {
			return nil, calendarSmartError("calendar/get_calendar_participants", "missing_attendees", "响应缺少显式参会人数组")
		}
	default:
		return nil, calendarSmartError("calendar/get_calendar_participants", "malformed_result", "参会人响应 result 既不是数组也不是含参会人数组的对象")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, calendarSmartError("calendar/get_calendar_participants", "malformed_attendees", "参会人字段不是数组")
	}
	identities := map[string]bool{}
	for index, item := range items {
		attendee, ok := item.(map[string]any)
		if !ok || len(attendee) == 0 {
			return nil, calendarSmartError("calendar/get_calendar_participants", "malformed_attendee", fmt.Sprintf("参会人第 %d 项不是非空对象", index))
		}
		id := calendarSmartFirstString(attendee, "userId", "user_id", "id", "staffId")
		name := calendarSmartFirstString(attendee, "displayName", "display_name", "name", "userName")
		if id == "" && name == "" {
			return nil, calendarSmartError("calendar/get_calendar_participants", "missing_attendee_identity", fmt.Sprintf("参会人第 %d 项既无 userId 也无 displayName", index))
		}
		if id != "" {
			identities[id] = true
		}
		if name != "" {
			identities[name] = true
		}
		if rawSelf, present := attendee["self"]; present {
			isSelf, ok := rawSelf.(bool)
			if !ok {
				return nil, calendarSmartError("calendar/get_calendar_participants", "malformed_attendee_self", fmt.Sprintf("参会人第 %d 项 self 不是布尔值", index))
			}
			if isSelf {
				identities["__self__"] = true
			}
		}
	}
	return identities, nil
}

func calendarSmartVerifyEventTimes(event map[string]any, start, end string) error {
	startValue, startOK := calendarSmartEventTime(event["start"])
	if !startOK {
		startValue, startOK = calendarSmartEventTime(event["startDateTime"])
	}
	endValue, endOK := calendarSmartEventTime(event["end"])
	if !endOK {
		endValue, endOK = calendarSmartEventTime(event["endDateTime"])
	}
	wantStart, startErr := time.Parse(time.RFC3339, start)
	wantEnd, endErr := time.Parse(time.RFC3339, end)
	if startErr != nil || endErr != nil || !startOK || !endOK || !startValue.Equal(wantStart) || !endValue.Equal(wantEnd) {
		return calendarSmartError("calendar/get_calendar_detail", "readback_time_mismatch", "写后读回的开始/结束时间与请求不一致")
	}
	return nil
}

func calendarSmartEventTime(value any) (time.Time, bool) {
	if object, ok := value.(map[string]any); ok {
		value = object["dateTime"]
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	return parsed, err == nil
}

func calendarSmartNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"not found", "not_found", "不存在", "未找到", "404"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func calendarSmartValidateRange(start, end string) error {
	startTime, startErr := time.Parse(time.RFC3339, strings.TrimSpace(start))
	if startErr != nil {
		return apperrors.NewValidation("--start 时间格式无效，请使用 RFC3339/ISO8601")
	}
	endTime, endErr := time.Parse(time.RFC3339, strings.TrimSpace(end))
	if endErr != nil {
		return apperrors.NewValidation("--end 时间格式无效，请使用 RFC3339/ISO8601")
	}
	if !endTime.After(startTime) {
		return apperrors.NewValidation("--end 必须晚于 --start")
	}
	return nil
}

func calendarSmartWriteReceipt(data map[string]any, operation string) error {
	_, err := calendarSmartRequireSuccess(data, operation)
	return err
}

func calendarSmartVerifyCreatedEvent(event map[string]any, eventID, title, start, end string) error {
	if id := calendarSmartFirstString(event, "id", "eventId", "event_id"); id != eventID {
		return calendarSmartError("calendar/get_calendar_detail", "readback_event_id_mismatch", "创建后读回的日程 id 与创建回执不一致")
	}
	if got := calendarSmartFirstString(event, "summary", "title"); got != strings.TrimSpace(title) {
		return calendarSmartError("calendar/get_calendar_detail", "readback_title_mismatch", "创建后读回的日程标题与请求不一致")
	}
	return calendarSmartVerifyEventTimes(event, start, end)
}

func calendarSmartVerifyAttendees(present map[string]bool, expectedIDs, expectedNames []string, currentUserID string) error {
	for index, id := range expectedIDs {
		name := ""
		if index < len(expectedNames) {
			name = expectedNames[index]
		}
		isCurrentUser := currentUserID != "" && id == currentUserID && present["__self__"]
		if !present[id] && (name == "" || !present[name]) && !isCurrentUser {
			return calendarSmartError("calendar/get_calendar_participants", "readback_attendee_missing", "写后读回未找到请求添加的参会人")
		}
	}
	return nil
}

func calendarSmartCurrentUserID(rt *shortcut.RuntimeContext, attendees map[string]bool) (string, error) {
	if !attendees["__self__"] {
		return "", nil
	}
	profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
	if err != nil {
		return "", err
	}
	userID := myAttendanceCurrentUserID(profile)
	if userID == "" {
		return "", calendarSmartError("contact/get_current_user_profile", "missing_current_user_id", "参会人读回只标记 self，但无法解析当前用户 userId")
	}
	return userID, nil
}

func calendarSmartDeleteAndVerify(rt *shortcut.RuntimeContext, eventID string) error {
	written, err := rt.CallMCPWriteDataStrict("calendar", "delete_calendar_event", map[string]any{"eventId": eventID})
	if err != nil {
		return err
	}
	if err := calendarSmartWriteReceipt(written, "calendar/delete_calendar_event"); err != nil {
		return err
	}
	data, err := rt.CallMCPData("calendar", "get_calendar_detail", map[string]any{"eventId": eventID})
	if err != nil {
		if calendarSmartNotFound(err) {
			return nil
		}
		return calendarSmartError("calendar/get_calendar_detail", "delete_readback_unknown", "删除后读回失败，无法证明日程已不存在")
	}
	event, err := calendarSmartRequireEvent(data, "calendar/get_calendar_detail", eventID)
	if err != nil {
		return err
	}
	status := strings.ToLower(calendarSmartFirstString(event, "status", "eventStatus", "event_status"))
	if status == "cancelled" || status == "canceled" || status == "deleted" {
		return nil
	}
	return calendarSmartError("calendar/get_calendar_detail", "delete_readback_present", "删除后读回日程仍然存在")
}
