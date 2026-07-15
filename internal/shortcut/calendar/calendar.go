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
	"strings"
	"time"

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
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 ISO-8601 (例如 2026-03-10T00:00:00+08:00)，默认今天 00:00"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 ISO-8601，默认今天 23:59"},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID (默认 primary 主日历)"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标 (上一次返回的 nextCursor)"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回条数 (默认 100，最大 100)"},
	},
	Tips: []string{
		`dws calendar +agenda`,
		`dws calendar +agenda --start "2026-03-10T00:00:00+08:00" --end "2026-03-31T23:59:59+08:00" --limit 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		now := time.Now()
		if rt.Changed("start") {
			ms, err := parseMillis("start", rt.Str("start"))
			if err != nil {
				return err
			}
			params["startTime"] = ms
		} else {
			params["startTime"] = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
		}
		if rt.Changed("end") {
			ms, err := parseMillis("end", rt.Str("end"))
			if err != nil {
				return err
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
		data, err := rt.CallMCPData("calendar", "list_calendar_events", params)
		if err != nil {
			return err
		}
		events := eventListProject(data)
		return rt.Output(map[string]any{"count": len(events), "events": events})
	},
}

// eventListProject reshapes the raw list_calendar_events response into a clean,
// stable event list (eventId/summary/start/end/status/location) — the
// the clean output projection applied to every list command. The
// list container and each field are probed defensively across candidate keys,
// since event payloads may nest under result/data/list/items with aliases.
func eventListProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	raw := eventListContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := eventListFirst(m, "eventId", "event_id", "id"); ok {
			row["eventId"] = v
		}
		if v, ok := eventListFirst(m, "summary", "title", "name"); ok {
			row["summary"] = v
		}
		if v, ok := eventListFirst(m, "start", "startTime", "start_time", "startDateTime", "start_date_time"); ok {
			row["start"] = v
		}
		if v, ok := eventListFirst(m, "end", "endTime", "end_time", "endDateTime", "end_date_time"); ok {
			row["end"] = v
		}
		if v, ok := eventListFirst(m, "status", "eventStatus", "event_status", "responseStatus", "response_status"); ok {
			row["status"] = v
		}
		if v, ok := eventListFirst(m, "location", "locationName", "location_name"); ok {
			row["location"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
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
		attendees := attendeeListProject(data)
		return rt.Output(map[string]any{"count": len(attendees), "attendees": attendees})
	},
}

// attendeeListProject reshapes the raw get_calendar_participants response into a
// clean, stable attendee list (displayName/userId/responseStatus) — the
// the clean output projection applied to every list command.
// The list container and each field are probed defensively across candidate keys,
// since participant payloads may nest under result/data/list/items with aliases.
func attendeeListProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	raw := attendeeListContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := attendeeFirst(m, "displayName", "display_name", "name", "userName", "user_name", "nick", "nickName"); ok {
			row["displayName"] = v
		}
		if v, ok := attendeeFirst(m, "userId", "user_id", "id", "staffId", "staff_id", "unionId", "union_id"); ok {
			row["userId"] = v
		}
		if v, ok := attendeeFirst(m, "responseStatus", "response_status", "status", "attendeeStatus", "attendee_status", "responseType", "response"); ok {
			row["responseStatus"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
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
		rooms := roomSearchProject(data)
		return rt.Output(map[string]any{"count": len(rooms), "rooms": rooms})
	},
}

// roomSearchProject reshapes the raw search_rooms response into a clean, stable
// room list (roomId/roomName/capacity/location) — the output-projection fidelity
// the framework applies to every list command. The list container and each field
// are probed defensively across candidate keys, since room payloads may nest
// under result/data/list/items with aliases.
func roomSearchProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	raw := roomSearchContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := roomSearchFirst(m, "roomId", "room_id", "id"); ok {
			row["roomId"] = v
		}
		if v, ok := roomSearchFirst(m, "roomName", "room_name", "name", "summary"); ok {
			row["roomName"] = v
		}
		if v, ok := roomSearchFirst(m, "capacity", "seats", "seatCount", "seat_count"); ok {
			row["capacity"] = v
		}
		if v, ok := roomSearchFirst(m, "location", "floor", "building", "address"); ok {
			row["location"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
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
	Intent:      "当你要在某个时间段找一间空闲会议室开会时使用；传入 --start/--end 时间段（须为未来时间，缺省为当前起 1 小时），可加 --available 只看空闲、--group-id/--room-name 缩小范围，返回该时段的会议室及其可用性和 roomId，便于据此 +room-add 预定。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间 ISO-8601 (必须是未来时间)"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间 ISO-8601"},
		{Name: "available", Type: shortcut.FlagBool, Desc: "仅返回可用会议室"},
		{Name: "group-id", Type: shortcut.FlagString, Desc: "会议室分组 ID"},
		{Name: "room-name", Type: shortcut.FlagString, Desc: "会议室名称过滤"},
		{Name: "limit", Type: shortcut.FlagString, Desc: "每页条数 (pageSize)"},
		{Name: "page", Type: shortcut.FlagString, Desc: "页码 (pageIndex，从 0 开始)"},
	},
	Tips: []string{
		`dws calendar +room-find --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
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
		params := map[string]any{
			"startTime": startMs,
			"endTime":   endMs,
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
		if rt.Changed("limit") {
			params["pageSize"] = rt.Str("limit")
		}
		if rt.Changed("page") {
			params["pageIndex"] = rt.Str("page")
		}
		return rt.CallMCP("query_available_meeting_room", params)
	},
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
		groups := roomGroupsProject(data)
		return rt.Output(map[string]any{"count": len(groups), "groups": groups})
	},
}

// roomGroupsProject reshapes the raw list_meeting_room_groups response into a
// clean, stable group list (groupId/groupName) — the output-projection fidelity
// the framework applies to every list command. The list container and each field
// are probed defensively across candidate keys, since group payloads may nest
// under result/data/list/items with aliases.
func roomGroupsProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	raw := roomGroupsContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := roomGroupsFirst(m, "groupId", "group_id", "id"); ok {
			row["groupId"] = v
		}
		if v, ok := roomGroupsFirst(m, "groupName", "group_name", "name", "summary"); ok {
			row["groupName"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// roomGroupsContainer locates the group slice across candidate wrapper keys,
// unwrapping one nested object layer (e.g. result.list) when needed.
func roomGroupsContainer(data map[string]any) []any {
	keys := []string{"result", "data", "list", "items", "groups"}
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
		return rt.CallMCP("query_busy_status", params)
	},
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
	Tips:        []string{`dws calendar +book-list`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("calendar", "list_calendars", nil)
		if err != nil {
			return err
		}
		books := bookListProject(data)
		return rt.Output(map[string]any{"count": len(books), "calendars": books})
	},
}

// bookListProject reshapes list_calendars into a clean calendar-book list
// (calendarId/summary/privilege/type) — clean output projection.
func bookListProject(data map[string]any) []map[string]any {
	raw, ok := data["result"].([]any)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		for _, k := range []string{"calendarId", "summary", "privilege", "type", "description"} {
			if v, ok := m[k]; ok {
				row[k] = v
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
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
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "日历本名称关键词", Required: true},
	},
	Tips: []string{`dws calendar +book-search --query "项目"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("calendar", "search_calendar", map[string]any{"query": rt.Str("query")})
		if err != nil {
			return err
		}
		calendars := bookSearchProject(data)
		return rt.Output(map[string]any{"count": len(calendars), "calendars": calendars})
	},
}

// bookSearchProject reshapes the raw search_calendar response into a clean,
// stable calendar-book list (calendarId/summary/privilege/type) — output-projection
// clean output projection. The list container and each field are probed defensively
// across candidate keys, tolerating nesting under result/data/list/items.
func bookSearchProject(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	raw := bookSearchContainer(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := bookSearchFirst(m, "calendarId", "calendar_id", "id"); ok {
			row["calendarId"] = v
		}
		if v, ok := bookSearchFirst(m, "summary", "name", "title"); ok {
			row["summary"] = v
		}
		if v, ok := bookSearchFirst(m, "privilege", "role", "accessRole"); ok {
			row["privilege"] = v
		}
		if v, ok := bookSearchFirst(m, "type", "calendarType", "calendar_type"); ok {
			row["type"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
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
	shortcut.Register(
		EventList,
		AttendeeList,
		RoomSearch,
		RoomFind,
		RoomGroups,
		BusySearch,
		BookList,
		BookSearch,
	)
}
