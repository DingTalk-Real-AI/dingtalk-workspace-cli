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

// Package calendar declares the declarative shortcut (+command) layer for the
// DingTalk calendar MCP product. Tool names and parameter keys are copied
// verbatim from internal/helpers/calendar.go, the single source of truth for
// the real DingTalk MCP tools.
package calendar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// parseMillis converts an ISO-8601 / RFC3339 timestamp (e.g.
// "2026-03-10T14:00:00+08:00") into Unix epoch milliseconds, matching the
// helper's parseISOTimeToMillis behaviour for the millis-based MCP tools.
func parseMillis(field, v string) (int64, error) {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return 0, fmt.Errorf("invalid --%s time %q: expected ISO-8601 like 2026-03-10T14:00:00+08:00", field, v)
	}
	return t.UnixMilli(), nil
}

// ── event: 日程 ──────────────────────────────────────────────

// EventList → list_calendar_events
var EventList = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+agenda",
	Product:     "calendar",
	Description: "查询日程列表（不传时间默认查询今天）",
	Intent:      "当你想了解某人（默认自己）在某段时间内的日程安排、看看今天/本周有哪些会时使用；可传 --start/--end 圈定时间范围、--calendar-id 指定日历，返回该区间内的日程列表（含日程 ID，可配合 +get 看详情）。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_agenda",
			CanonicalPath:  "calendar.shortcut_agenda",
			CLIPath:        "calendar +agenda",
			PrimaryCLIPath: "calendar +agenda",
		},
		Description: "查询日程列表（不传时间默认查询今天）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询日程列表（不传时间默认查询今天）",
			UseWhen:      []string{"当你想了解某人（默认自己）在某段时间内的日程安排、看看今天/本周有哪些会时使用；可传 --start/--end 圈定时间范围、--calendar-id 指定日历，返回该区间内的日程列表（含日程 ID，可配合 +get 看详情）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws calendar +agenda",
				"dws calendar +agenda --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-31T23:59:59+08:00\" --limit 50",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 ISO-8601；起止时间必须是 RFC3339/ISO-8601，且 end 晚于 start；默认今天 00:00"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 ISO-8601；起止时间必须是 RFC3339/ISO-8601，且 end 晚于 start；默认今天 23:59"},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID (默认 primary 主日历)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上一次返回的 nextCursor)"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回条数（服务端默认 100）；limit 必须在 1-100 之间"},
	},
	Tips: []string{
		`dws calendar +agenda`,
		`dws calendar +agenda --start "2026-03-10T00:00:00+08:00" --end "2026-03-31T23:59:59+08:00" --limit 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params, err := calendarAgendaParams(rt, time.Now())
		if err != nil {
			return err
		}
		data, err := rt.CallMCPData("calendar", "list_calendar_events", params)
		if err != nil {
			return err
		}
		events, page, err := eventListProject(data)
		if err != nil {
			return err
		}
		out := map[string]any{"count": len(events), "events": events}
		return outputCalendarPage(rt, out, page)
	},
}

// calendarAgendaParams is the explicit adapter from the published composite
// Shortcut properties (start/end) to list_calendar_events RPC properties
// (startTime/endTime). The public Schema property names predate this adapter
// and remain stable for backwards compatibility.
func calendarAgendaParams(rt *shortcut.RuntimeContext, now time.Time) (map[string]any, error) {
	params := map[string]any{}
	if rt.Changed("start") {
		ms, err := parseMillis("start", rt.Str("start"))
		if err != nil {
			return nil, err
		}
		params["startTime"] = ms
	} else {
		params["startTime"] = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
	}
	if rt.Changed("end") {
		ms, err := parseMillis("end", rt.Str("end"))
		if err != nil {
			return nil, err
		}
		params["endTime"] = ms
	} else {
		params["endTime"] = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).UnixMilli()
	}
	if rt.Changed("calendar-id") {
		params["calendarId"] = rt.Str("calendar-id")
	}
	if rt.Changed("cursor") {
		params["cursor"] = rt.Str("cursor")
	}
	if rt.Changed("limit") {
		params["limit"] = rt.Int("limit")
	}
	if params["endTime"].(int64) <= params["startTime"].(int64) {
		return nil, fmt.Errorf("--end 必须晚于 --start")
	}
	return params, nil
}

// eventListProject reshapes the raw list_calendar_events response into a clean,
// stable event list (eventId/summary/start/end/status/location) — the
// the clean output projection applied to every list command. The
// list container and each field are probed defensively across candidate keys,
// since event payloads may nest under result/data/list/items with aliases.
func eventListProject(data map[string]any) ([]map[string]any, calendarPageEvidence, error) {
	items, container, err := requireCalendarCollection(data, "calendar/list_calendar_events", "events")
	if err != nil {
		return nil, calendarPageEvidence{}, err
	}
	rows, err := projectCalendarRows(items, "calendar/list_calendar_events", map[string][]string{
		"eventId":  {"eventId", "event_id", "id"},
		"summary":  {"summary", "title", "name"},
		"start":    {"start", "startTime", "start_time", "startDateTime", "start_date_time"},
		"end":      {"end", "endTime", "end_time", "endDateTime", "end_date_time"},
		"status":   {"status", "eventStatus", "event_status", "responseStatus", "response_status"},
		"location": {"location", "locationName", "location_name"},
	}, "eventId")
	if err != nil {
		return nil, calendarPageEvidence{}, err
	}
	page, err := calendarPagination(container, "calendar/list_calendar_events")
	if err == nil && !page.Known {
		err = calendarResponseError("calendar/list_calendar_events", "missing_pagination", "日程列表响应缺少 hasMore/nextCursor 分页证据，不能判断结果是否完整")
	}
	return rows, page, err
}

// eventListContainer locates the event slice across candidate wrapper keys,
// unwrapping one nested object layer (e.g. result.list) when needed.
func eventListContainer(data map[string]any) []any {
	keys := []string{"result", "data", "list", "items", "events"}
	for _, k := range keys {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if nested, ok := v.(map[string]any); ok {
			for _, nk := range keys {
				if arr, ok := nested[nk].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// eventListFirst returns the first present, non-nil value among candidate keys.
func eventListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// EventGet → get_calendar_detail
// EventCreate → create_calendar_event
// EventUpdate → update_calendar_event
// EventDelete → delete_calendar_event
// EventSuggest → list_suggested_event_times
// EventRespond → respond
// ── attendee: 参会人 ──────────────────────────────────────────

// AttendeeList → get_calendar_participants
var AttendeeList = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+attendee-list",
	Product:     "calendar",
	Description: "查看日程参会人",
	Intent:      "当你想知道某个日程都有谁参加、各人的出席响应状态时使用；输入 --event 日程 ID，返回参会人列表（userId 及其接受/拒绝等状态）。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_attendee_list",
			CanonicalPath:  "calendar.shortcut_attendee_list",
			CLIPath:        "calendar +attendee-list",
			PrimaryCLIPath: "calendar +attendee-list",
		},
		Description: "查看日程参会人",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查看日程参会人",
			UseWhen:      []string{"当你想知道某个日程都有谁参加、各人的出席响应状态时使用；输入 --event 日程 ID，返回参会人列表（userId 及其接受/拒绝等状态）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +attendee-list --event EVENT_ID"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Desc: "日程 ID", Required: true},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID (默认 primary 主日历)"},
	},
	Tips: []string{`dws calendar +attendee-list --event EVENT_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"eventId": rt.Str("event")}
		if rt.Changed("calendar-id") {
			params["calendarId"] = rt.Str("calendar-id")
		}
		data, err := rt.CallMCPData("calendar", "get_calendar_participants", params)
		if err != nil {
			return err
		}
		attendees, err := attendeeListProject(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(attendees), "attendees": attendees})
	},
}

// attendeeListProject reshapes the raw get_calendar_participants response into a
// clean, stable attendee list (displayName/userId/responseStatus) — the
// the clean output projection applied to every list command.
// The list container and each field are probed defensively across candidate keys,
// since participant payloads may nest under result/data/list/items with aliases.
func attendeeListProject(data map[string]any) ([]map[string]any, error) {
	items, _, err := requireCalendarCollection(data, "calendar/get_calendar_participants", "attendees", "participants", "items", "list")
	if err != nil {
		return nil, err
	}
	return projectCalendarRows(items, "calendar/get_calendar_participants", map[string][]string{
		"displayName":    {"displayName", "display_name", "name", "userName", "user_name", "nick", "nickName"},
		"userId":         {"userId", "user_id", "id", "staffId", "staff_id", "unionId", "union_id"},
		"responseStatus": {"responseStatus", "response_status", "status", "attendeeStatus", "attendee_status", "responseType", "response"},
		"self":           {"self"},
	}, "displayName", "userId")
}

// attendeeListContainer locates the participant slice across candidate wrapper
// keys, unwrapping one nested object layer (e.g. result.list) when needed.
func attendeeListContainer(data map[string]any) []any {
	keys := []string{"result", "data", "list", "items", "attendees", "participants"}
	for _, k := range keys {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if nested, ok := v.(map[string]any); ok {
			for _, nk := range keys {
				if arr, ok := nested[nk].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// attendeeFirst returns the first present, non-nil value among candidate keys.
func attendeeFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// AttendeeAdd → add_calendar_participant
// AttendeeRemove → remove_calendar_participant
// ── room: 会议室 ──────────────────────────────────────────────

// RoomSearch → search_rooms (按名称模糊搜索，不检查可用性)
var RoomSearch = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+room-search",
	Product:     "calendar",
	Description: "按名称模糊搜索会议室（不检查可用性）",
	Intent:      "当你只知道会议室名字、想拿到它的 roomId 以便后续预定时使用；输入 --room-name 名称关键词（建议只填核心专名，去掉“会议室”等后缀），返回名称匹配的会议室列表。它只按名字找、不判断该时段是否空闲，查可用性请用 +room-find。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_room_search",
			CanonicalPath:  "calendar.shortcut_room_search",
			CLIPath:        "calendar +room-search",
			PrimaryCLIPath: "calendar +room-search",
		},
		Description: "按名称模糊搜索会议室（不检查可用性）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按名称模糊搜索会议室（不检查可用性）",
			UseWhen:      []string{"当你只知道会议室名字、想拿到它的 roomId 以便后续预定时使用；输入 --room-name 名称关键词（建议只填核心专名，去掉“会议室”等后缀），返回名称匹配的会议室列表。它只按名字找、不判断该时段是否空闲，查可用性请用 +room-find。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +room-search --room-name 永澄亭"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "room-name", Type: shortcut.FlagString, Desc: "会议室名称（精简核心专名，剔除“会议室”等后缀）", Required: true},
	},
	Tips: []string{`dws calendar +room-search --room-name 永澄亭`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"roomName": strings.TrimSpace(rt.Str("room-name"))}
		data, err := rt.CallMCPData("calendar", "search_rooms", params)
		if err != nil {
			return err
		}
		rooms, err := roomSearchProject(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(rooms), "rooms": rooms})
	},
}

// roomSearchProject reshapes the raw search_rooms response into a clean, stable
// room list (roomId/roomName/capacity/location) — the output-projection fidelity
// the framework applies to every list command. The list container and each field
// are probed defensively across candidate keys, since room payloads may nest
// under result/data/list/items with aliases.
func roomSearchProject(data map[string]any) ([]map[string]any, error) {
	items, _, err := requireCalendarCollection(data, "calendar/search_rooms", "rooms", "roomList", "meetingRooms", "items", "list")
	if err != nil {
		return nil, err
	}
	return projectCalendarRows(items, "calendar/search_rooms", map[string][]string{
		"roomId":   {"roomId", "room_id", "id"},
		"roomName": {"roomName", "room_name", "name", "summary"},
		"capacity": {"capacity", "seats", "seatCount", "seat_count"},
		"location": {"location", "floor", "building", "address"},
	}, "roomId")
}

// roomSearchContainer locates the room slice across candidate wrapper keys,
// unwrapping one nested object layer (e.g. result.list) when needed.
func roomSearchContainer(data map[string]any) []any {
	keys := []string{"result", "data", "list", "items", "rooms"}
	for _, k := range keys {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if nested, ok := v.(map[string]any); ok {
			for _, nk := range keys {
				if arr, ok := nested[nk].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// roomSearchFirst returns the first present, non-nil value among candidate keys.
func roomSearchFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// RoomFind → query_available_meeting_room (按时间段查可用会议室)
var RoomFind = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+room-find",
	Product:     "calendar",
	Description: "按时间段搜索可用会议室（不传时间默认当前起 1 小时）",
	Intent:      "当你要在一个未来时间段寻找当前可预定的会议室时使用；--group-id/--room-name 可缩小范围，显式返回当前页和页码信息。该命令只查询单个时间段，后端不支持 Lark 风格多 slot、城市/楼宇/容量联合筛选。",
	Risk:        shortcut.RiskRead,
	Safety:      calendarReadSafety(),
	Contract: calendarContract(
		"+room-find", "按时间段严格搜索可用会议室",
		"需要按一个明确的未来时间段查询可用会议室，并保留真实页码信息时使用。",
		[]string{"只按名称查会议室而不检查可用性时改用 calendar +room-search", "要预订会议室时先取得 roomId，再使用原子 room add"},
		[]string{`dws calendar +room-find --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"`},
		calendarCollectionResult("rooms", "严格校验的可用会议室当前页"), nil,
		contract.ParamDecl{Name: "start", Property: "startTime"},
		contract.ParamDecl{Name: "end", Property: "endTime"},
		contract.ParamDecl{Name: "group-id", Property: "groupId"},
		contract.ParamDecl{Name: "room-name", Property: "roomName"},
		contract.ParamDecl{Name: "limit", Property: "pageSize"},
		contract.ParamDecl{Name: "page", Property: "pageIndex"},
	),
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 ISO-8601；起止时间必须是 RFC3339/ISO-8601，且 end 晚于 start"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 ISO-8601；起止时间必须是 RFC3339/ISO-8601，且 end 晚于 start"},
		{Name: "available", Type: shortcut.FlagBool, Desc: "仅返回可用会议室；保留已发布参数兼容性"},
		{Name: "group-id", Type: shortcut.FlagString, Desc: "会议室分组 ID"},
		{Name: "room-name", Type: shortcut.FlagString, Desc: "会议室名称过滤"},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页条数 (pageSize)；保留 string 类型兼容性；limit 必须在 1-100 之间"},
		{Name: "page", Type: shortcut.FlagString, Desc: "页码 (pageIndex，从 0 开始)；保留 string 类型兼容性；page 不能小于 0"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		_, _, err := roomFindPageValues(rt)
		return err
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "起止时间必须是 RFC3339/ISO-8601，且 end 晚于 start"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "limit 必须在 1-100 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page"}, Description: "page 不能小于 0"},
	},
	Tips: []string{
		`dws calendar +room-find --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		pageSize, pageIndex, err := roomFindPageValues(rt)
		if err != nil {
			return err
		}
		now := time.Now()
		startStr := rt.Str("start")
		endStr := rt.Str("end")
		if startStr == "" {
			startStr = now.Add(1 * time.Minute).Format(time.RFC3339)
		}
		if endStr == "" {
			endStr = now.Add(1 * time.Hour).Format(time.RFC3339)
		}
		startMs, err := parseMillis("start", startStr)
		if err != nil {
			return err
		}
		endMs, err := parseMillis("end", endStr)
		if err != nil {
			return err
		}
		if endMs <= startMs {
			return fmt.Errorf("--end 必须晚于 --start")
		}
		params := map[string]any{
			"startTime": startMs,
			"endTime":   endMs,
			"pageSize":  strconv.Itoa(pageSize),
			"pageIndex": strconv.Itoa(pageIndex),
		}
		if rt.Bool("available") {
			params["needAvailable"] = true
		}
		if rt.Changed("group-id") {
			params["groupId"] = rt.Str("group-id")
		}
		if rt.Changed("room-name") {
			params["roomName"] = strings.TrimSpace(rt.Str("room-name"))
		}
		data, err := rt.CallMCPData("calendar", "query_available_meeting_room", params)
		if err != nil {
			return err
		}
		items, container, err := requireCalendarCollection(data, "calendar/query_available_meeting_room", "rooms", "roomList", "meetingRoomList", "meetingRooms", "items", "list")
		if err != nil {
			return err
		}
		rooms, err := projectCalendarRows(items, "calendar/query_available_meeting_room", map[string][]string{
			"roomId":           {"roomId", "room_id", "id"},
			"roomName":         {"roomName", "room_name", "name", "summary"},
			"capacity":         {"capacity", "maxCapacity", "seatCount", "seats"},
			"groupId":          {"groupId", "group_id"},
			"supportRecurring": {"supportRecurring"},
		}, "roomId")
		if err != nil {
			return err
		}
		out := map[string]any{"count": len(rooms), "rooms": rooms, "page": pageIndex, "pageSize": pageSize, "complete": false}
		for _, key := range []string{"hasMore", "totalCount", "total", "pageIndex", "pageSize"} {
			if value, ok := container[key]; ok && value != nil {
				out[key] = value
			}
		}
		if hasMore, ok := out["hasMore"].(bool); ok {
			out["complete"] = !hasMore
		}
		return rt.Output(out)
	},
}

func roomFindPageValues(rt *shortcut.RuntimeContext) (pageSize int, pageIndex int, err error) {
	pageSize = 20
	if rt.Changed("limit") {
		pageSize, err = strconv.Atoi(strings.TrimSpace(rt.Str("limit")))
		if err != nil || pageSize < 1 || pageSize > 100 {
			return 0, 0, fmt.Errorf("--limit 必须是 1-100 之间的整数")
		}
	}
	if rt.Changed("page") {
		pageIndex, err = strconv.Atoi(strings.TrimSpace(rt.Str("page")))
		if err != nil || pageIndex < 0 {
			return 0, 0, fmt.Errorf("--page 必须是大于或等于 0 的整数")
		}
	}
	return pageSize, pageIndex, nil
}

// RoomAdd → add_meeting_room
// RoomRemove → delete_meeting_room
// RoomGroups → list_meeting_room_groups
var RoomGroups = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+room-groups",
	Product:     "calendar",
	Description: "会议室分组列表",
	Intent:      "当你想按楼层/园区等分组浏览会议室、或需要拿到 groupId 以便在 +room-find 里按分组过滤时使用；返回会议室分组列表，支持 --limit/--page 分页。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_room_groups",
			CanonicalPath:  "calendar.shortcut_room_groups",
			CLIPath:        "calendar +room-groups",
			PrimaryCLIPath: "calendar +room-groups",
		},
		Description: "会议室分组列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "会议室分组列表",
			UseWhen:      []string{"当你想按楼层/园区等分组浏览会议室、或需要拿到 groupId 以便在 +room-find 里按分组过滤时使用；返回会议室分组列表，支持 --limit/--page 分页。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +room-groups"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页条数 (pageSize)"},
		{Name: "page", Type: shortcut.FlagString, Desc: "页码 (pageIndex，从 0 开始)"},
	},
	Tips: []string{`dws calendar +room-groups`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("limit") {
			params["pageSize"] = rt.Str("limit")
		}
		if rt.Changed("page") {
			params["pageIndex"] = rt.Str("page")
		}
		data, err := rt.CallMCPData("calendar", "list_meeting_room_groups", params)
		if err != nil {
			return err
		}
		groups, err := roomGroupsProject(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(groups), "groups": groups})
	},
}

// roomGroupsProject reshapes the raw list_meeting_room_groups response into a
// clean, stable group list (groupId/groupName) — the output-projection fidelity
// the framework applies to every list command. The list container and each field
// are probed defensively across candidate keys, since group payloads may nest
// under result/data/list/items with aliases.
func roomGroupsProject(data map[string]any) ([]map[string]any, error) {
	items, _, err := requireCalendarCollection(data, "calendar/list_meeting_room_groups", "groupList", "groups", "items", "list")
	if err != nil {
		return nil, err
	}
	return projectCalendarRows(items, "calendar/list_meeting_room_groups", map[string][]string{
		"groupId":   {"groupId", "group_id", "id"},
		"groupName": {"groupName", "group_name", "name", "summary"},
	}, "groupId")
}

// roomGroupsContainer locates the group slice across candidate wrapper keys,
// unwrapping one nested object layer (e.g. result.list) when needed.
func roomGroupsContainer(data map[string]any) []any {
	// list_meeting_room_groups nests the groups under result.groupList;
	// "groupList" MUST be probed or +room-groups silently returns empty despite
	// the backend returning meeting-room groups.
	keys := []string{"result", "data", "list", "items", "groupList", "groups"}
	for _, k := range keys {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if nested, ok := v.(map[string]any); ok {
			for _, nk := range keys {
				if arr, ok := nested[nk].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// roomGroupsFirst returns the first present, non-nil value among candidate keys.
func roomGroupsFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// ── busy: 闲忙 ────────────────────────────────────────────────

// BusySearch → query_busy_status
var BusySearch = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+freebusy",
	Product:     "calendar",
	Description: "查询用户 / 会议室闲忙状态（--users 与 --rooms 至少其一）",
	Intent:      "当你要在约会前确认某些人或会议室在指定时间段是否有空、避免冲突时使用；传入 --start/--end 时间段并至少给出 --users 或 --rooms 其一，返回各对象在该区间的忙/闲时段。只看忙闲结果、不看具体日程内容，需要系统给出建议时段可用 +suggestion。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_freebusy",
			CanonicalPath:  "calendar.shortcut_freebusy",
			CLIPath:        "calendar +freebusy",
			PrimaryCLIPath: "calendar +freebusy",
		},
		Description: "查询用户 / 会议室闲忙状态（--users 与 --rooms 至少其一）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询用户 / 会议室闲忙状态（--users 与 --rooms 至少其一）",
			UseWhen:      []string{"当你要在约会前确认某些人或会议室在指定时间段是否有空、避免冲突时使用；传入 --start/--end 时间段并至少给出 --users 或 --rooms 其一，返回各对象在该区间的忙/闲时段。只看忙闲结果、不看具体日程内容，需要系统给出建议时段可用 +suggestion。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +freebusy --users userId1,userId2 --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "用户 userId 列表 (逗号分隔)"},
		{Name: "rooms", Type: shortcut.FlagStringSlice, Desc: "会议室 roomId 列表 (逗号分隔)"},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 ISO-8601", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 ISO-8601", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"users", "rooms"}},
	},
	Tips: []string{
		`dws calendar +freebusy --users userId1,userId2 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		startMs, err := parseMillis("start", rt.Str("start"))
		if err != nil {
			return err
		}
		endMs, err := parseMillis("end", rt.Str("end"))
		if err != nil {
			return err
		}
		params := map[string]any{
			"startTime": startMs,
			"endTime":   endMs,
		}
		if len(rt.StrSlice("users")) > 0 {
			params["userIds"] = rt.StrSlice("users")
		}
		if len(rt.StrSlice("rooms")) > 0 {
			params["roomIds"] = rt.StrSlice("rooms")
		}
		data, err := rt.CallMCPData("calendar", "query_busy_status", params)
		if err != nil {
			return err
		}
		busy, err := busySearchProject(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(busy), "busy": busy, "free": len(busy) == 0, "complete": true})
	},
}

func busySearchProject(data map[string]any) ([]map[string]any, error) {
	entries, _, err := requireCalendarCollection(data, "calendar/query_busy_status", "busy", "entries")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for entryIndex, rawEntry := range entries {
		entry := rawEntry.(map[string]any)
		rawItems, present := entry["scheduleItems"]
		if !present {
			return nil, calendarResponseError("calendar/query_busy_status", "missing_schedule_items", fmt.Sprintf("result[%d] 缺少 scheduleItems", entryIndex))
		}
		items, ok := rawItems.([]any)
		if !ok {
			return nil, calendarResponseError("calendar/query_busy_status", "malformed_schedule_items", fmt.Sprintf("result[%d].scheduleItems 不是数组", entryIndex))
		}
		if err := validateCalendarCollectionItems(items, "calendar/query_busy_status", "scheduleItems"); err != nil {
			return nil, err
		}
		for itemIndex, rawItem := range items {
			item := rawItem.(map[string]any)
			start := calendarBusyDateTime(item["start"])
			end := calendarBusyDateTime(item["end"])
			if calendarEmptyValue(start) || calendarEmptyValue(end) {
				return nil, calendarResponseError("calendar/query_busy_status", "malformed_schedule_time", fmt.Sprintf("scheduleItems[%d] 缺少开始或结束时间", itemIndex))
			}
			out = append(out, map[string]any{"start": start, "end": end})
		}
	}
	return out, nil
}

func calendarBusyDateTime(value any) any {
	if object, ok := value.(map[string]any); ok {
		if dateTime, present := object["dateTime"]; present {
			return dateTime
		}
		if date, present := object["date"]; present {
			return date
		}
	}
	return value
}

// ── attachment: 附件 ──────────────────────────────────────────

// AttachmentAdd → add_attachments
// ── acl: 日历访问权限 ─────────────────────────────────────────

// AclList → list_acls
// AclAdd → add_acl
// AclDelete → delete_acl
// ── book: 日历本 ──────────────────────────────────────────────

// BookList → list_calendars
var BookList = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+book-list",
	Product:     "calendar",
	Description: "查询用户的日历本列表",
	Intent:      "当你想知道自己有哪些日历本（主日历、项目日历、订阅日历等）、或需要拿到某个日历的 calendarId 以便在 +agenda/+create 中指定时使用；无需参数，返回全部日历本列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_book_list",
			CanonicalPath:  "calendar.shortcut_book_list",
			CLIPath:        "calendar +book-list",
			PrimaryCLIPath: "calendar +book-list",
		},
		Description: "查询用户的日历本列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询用户的日历本列表",
			UseWhen:      []string{"当你想知道自己有哪些日历本（主日历、项目日历、订阅日历等）、或需要拿到某个日历的 calendarId 以便在 +agenda/+create 中指定时使用；无需参数，返回全部日历本列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +book-list"},
		},
	},
	Tips: []string{`dws calendar +book-list`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("calendar", "list_calendars", nil)
		if err != nil {
			return err
		}
		books, err := bookListProject(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(books), "calendars": books})
	},
}

// bookListProject reshapes list_calendars into a clean calendar-book list
// (calendarId/summary/privilege/type) — clean output projection.
func bookListProject(data map[string]any) ([]map[string]any, error) {
	items, _, err := requireCalendarCollection(data, "calendar/list_calendars", "calendars", "calendarList", "items", "list")
	if err != nil {
		return nil, err
	}
	return projectCalendarRows(items, "calendar/list_calendars", map[string][]string{
		"calendarId":  {"calendarId", "calendar_id", "id"},
		"summary":     {"summary", "name", "title"},
		"privilege":   {"privilege", "role", "accessRole"},
		"type":        {"type", "calendarType", "calendar_type"},
		"description": {"description", "desc"},
	}, "calendarId")
}

// BookGet → get_calendar
// BookSearch → search_calendar
var BookSearch = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+book-search",
	Product:     "calendar",
	Description: "按名称模糊搜索日历本",
	Intent:      "当你只记得日历本名字的一部分、想据此找到对应的 calendarId 时使用；输入 --query 名称关键词，返回名称匹配的日历本列表，便于后续在其它命令里指定 --calendar-id。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_book_search",
			CanonicalPath:  "calendar.shortcut_book_search",
			CLIPath:        "calendar +book-search",
			PrimaryCLIPath: "calendar +book-search",
		},
		Description: "按名称模糊搜索日历本",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按名称模糊搜索日历本",
			UseWhen:      []string{"当你只记得日历本名字的一部分、想据此找到对应的 calendarId 时使用；输入 --query 名称关键词，返回名称匹配的日历本列表，便于后续在其它命令里指定 --calendar-id。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +book-search --query \"项目\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "日历本名称关键词", Required: true},
	},
	Tips: []string{`dws calendar +book-search --query "项目"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("calendar", "search_calendar", map[string]any{"query": rt.Str("query")})
		if err != nil {
			return err
		}
		calendars, err := bookSearchProject(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(calendars), "calendars": calendars})
	},
}

// bookSearchProject reshapes the raw search_calendar response into a clean,
// stable calendar-book list (calendarId/summary/privilege/type) — output-projection
// clean output projection. The list container and each field are probed defensively
// across candidate keys, tolerating nesting under result/data/list/items.
func bookSearchProject(data map[string]any) ([]map[string]any, error) {
	items, _, err := requireCalendarCollection(data, "calendar/search_calendar", "calendars", "calendarList", "items", "list")
	if err != nil {
		return nil, err
	}
	return projectCalendarRows(items, "calendar/search_calendar", map[string][]string{
		"calendarId": {"calendarId", "calendar_id", "id"},
		"summary":    {"summary", "name", "title"},
		"privilege":  {"privilege", "role", "accessRole"},
		"type":       {"type", "calendarType", "calendar_type"},
	}, "calendarId")
}

// bookSearchContainer locates the calendar slice across candidate wrapper keys,
// unwrapping one nested object layer (e.g. result.list) when needed.
func bookSearchContainer(data map[string]any) []any {
	keys := []string{"result", "data", "list", "items", "calendars"}
	for _, k := range keys {
		v, ok := data[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if nested, ok := v.(map[string]any); ok {
			for _, nk := range keys {
				if arr, ok := nested[nk].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// bookSearchFirst returns the first present, non-nil value among candidate keys.
func bookSearchFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// BookUpdate → update_calendar
func init() {
	EventList.Contract.Parameters = []contract.ParamDecl{
		{Name: "start", Property: "start"}, {Name: "end", Property: "end"},
		{Name: "calendar-id", Property: "calendarId"}, {Name: "cursor", Property: "cursor"}, {Name: "limit", Property: "limit"},
	}
	EventList.Validate = func(rt *shortcut.RuntimeContext) error {
		if !rt.Changed("limit") {
			return nil
		}
		return rt.RangeInt("limit", 1, 100)
	}
	EventList.Constraints = []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"start", "end"}, Description: "起止时间必须是 RFC3339/ISO-8601，且 end 晚于 start"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "limit 必须在 1-100 之间"},
	}
	finalizeCalendarShortcut(&EventList, calendarCollectionResult("events", "严格校验并保留分页证据的日程当前页"), calendarCursorPagination())
	finalizeCalendarShortcut(&AttendeeList, calendarObjectResult("严格校验的日程参会人列表"), nil)
	finalizeCalendarShortcut(&RoomSearch, calendarObjectResult("严格校验的按名会议室列表"), nil)
	finalizeCalendarShortcut(&RoomFind, RoomFind.Contract.Result, nil)
	finalizeCalendarShortcut(&RoomGroups, calendarObjectResult("严格校验的会议室分组当前页"), nil)
	finalizeCalendarShortcut(&BusySearch, calendarObjectResult("用户或会议室忙闲结果"), nil)
	finalizeCalendarShortcut(&BookList, calendarObjectResult("严格校验的日历本列表"), nil)
	finalizeCalendarShortcut(&BookSearch, calendarObjectResult("严格校验的日历本搜索结果"), nil)
	EventSearch.Contract.Pagination = calendarCursorPagination()
	shortcut.Register(
		EventList,
		EventGet,
		EventCreate,
		EventUpdate,
		RSVP,
		EventSearch,
		Suggestion,
		AttendeeList,
		RoomSearch,
		RoomFind,
		RoomGroups,
		BusySearch,
		BookList,
		BookSearch,
	)
}
