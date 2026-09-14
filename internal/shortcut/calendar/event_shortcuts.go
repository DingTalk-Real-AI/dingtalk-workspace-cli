// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func calendarReadShortcut(command, description, intent, collection string, flags []shortcut.Flag, params []contract.ParamDecl, execute func(*shortcut.RuntimeContext) error) shortcut.Shortcut {
	result := calendarObjectResult(description)
	if collection != "" {
		result = calendarCollectionResult(collection, description)
	}
	return shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       "calendar",
		Command:       command,
		Product:       "calendar",
		Description:   description,
		Intent:        intent,
		Risk:          shortcut.RiskRead,
		Safety:        calendarReadSafety(),
		Contract: calendarContract(command, description, intent,
			[]string{"需要未公开底层参数或原始 MCP 响应时改用对应原子命令；缺失业务对象或数组不是合法空结果"},
			[]string{calendarExample(command)}, result, nil, params...),
		Flags:   flags,
		Execute: execute,
	}
}

func calendarWriteShortcut(command, description, intent, example string, flags []shortcut.Flag, params []contract.ParamDecl, execute func(*shortcut.RuntimeContext) error) shortcut.Shortcut {
	return shortcut.Shortcut{
		OutputRollout: output.RolloutUnifiedActive,
		Service:       "calendar",
		Command:       command,
		Product:       "calendar",
		Description:   description,
		Intent:        intent,
		Risk:          shortcut.RiskWrite,
		Safety:        calendarWriteSafety(),
		Contract: calendarContract(command, description, intent,
			[]string{"只需查看日程或尚未确认目标 eventId/影响范围时不要执行写操作"},
			[]string{example}, calendarObjectResult(description, contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure), nil, params...),
		Flags:   flags,
		Execute: execute,
	}
}

func calendarExample(command string) string {
	switch command {
	case "+get":
		return "dws calendar +get --event <EVENT_ID> --format json"
	case "+search-event":
		return `dws calendar +search-event --query "评审" --format json`
	case "+suggestion":
		return "dws calendar +suggestion --users <USER_ID> --duration 30 --format json"
	default:
		return "dws calendar " + command + " --format json"
	}
}

// EventGet is the strict shortcut counterpart of calendar event get. A missing
// result object and a result object without an id are protocol failures.
var EventGet = calendarReadShortcut(
	"+get", "严格获取日程详情",
	"已知 eventId 时读取完整日程；把后端稳定 id 规范化为 eventId，并拒绝空响应、缺失对象或错误对象。",
	"",
	[]shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Required: true, Desc: "日程 eventId"},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID（默认 primary）"},
	},
	[]contract.ParamDecl{{Name: "event", Property: "eventId"}, {Name: "calendar-id", Property: "calendarId"}},
	func(rt *shortcut.RuntimeContext) error {
		params := calendarEventTarget(rt)
		data, err := rt.CallMCPData("calendar", "get_calendar_detail", params)
		if err != nil {
			return err
		}
		event, err := requireCalendarEvent(data, "calendar/get_calendar_detail", rt.Str("event"))
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"event": normalizeCalendarEvent(event)})
	},
)

// EventCreate creates one event and requires an event id plus a successful
// get_calendar_detail read-back before reporting success.
var EventCreate = calendarWriteShortcut(
	"+create", "创建日程并读回验证",
	"创建单次日程，可同时传 userId 参会人和 roomId；只有创建响应给出 eventId 且详情读回与标题、起止时间一致时才报告成功。",
	`dws calendar +create --title "项目评审" --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --format json`,
	[]shortcut.Flag{
		{Name: "title", Type: shortcut.FlagString, Required: true, Desc: "日程标题，最多 2048 字符"},
		{Name: "start", Type: shortcut.FlagString, Required: true, Desc: "开始时间 RFC3339/ISO-8601"},
		{Name: "end", Type: shortcut.FlagString, Required: true, Desc: "结束时间 RFC3339/ISO-8601"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "日程描述，最多 5000 字符"},
		{Name: "attendees", Type: shortcut.FlagStringSlice, Desc: "参会人 userId，最多 500 个"},
		{Name: "rooms", Type: shortcut.FlagStringSlice, Desc: "会议室 roomId"},
		{Name: "timezone", Type: shortcut.FlagString, Desc: "IANA 时区，如 Asia/Shanghai"},
		{Name: "location", Type: shortcut.FlagString, Desc: "地点文本"},
		{Name: "free-busy", Type: shortcut.FlagString, Desc: "忙闲状态", Enum: []string{"busy", "free"}},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "目标日历 ID（默认 primary）"},
	},
	[]contract.ParamDecl{
		{Name: "title", Property: "summary"}, {Name: "start", Property: "startDateTime"}, {Name: "end", Property: "endDateTime"},
		{Name: "desc", Property: "description"}, {Name: "attendees", Property: "attendees"}, {Name: "rooms", Property: "roomIds"},
		{Name: "timezone", Property: "timeZone"}, {Name: "location", Property: "location"}, {Name: "free-busy", Property: "freeBusy"},
		{Name: "calendar-id", Property: "calendarId"},
	},
	func(rt *shortcut.RuntimeContext) error {
		params, verify, err := calendarCreateParams(rt)
		if err != nil {
			return err
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "calendar/create_calendar_event", "arguments": params})
		}
		written, err := rt.CallMCPWriteDataStrict("calendar", "create_calendar_event", params)
		if err != nil {
			return err
		}
		written, err = requireCalendarWriteResponse(written, "calendar/create_calendar_event")
		if err != nil {
			return err
		}
		eventID := nestedCalendarString(written, "eventId", "event_id", "id")
		if eventID == "" {
			return calendarResponseError("calendar/create_calendar_event", "missing_created_id", "创建响应没有 eventId/id；远端效果未知")
		}
		readParams := map[string]any{"eventId": eventID}
		if calendarID := rt.Str("calendar-id"); calendarID != "" {
			readParams["calendarId"] = calendarID
		}
		readback, err := rt.CallMCPData("calendar", "get_calendar_detail", readParams)
		if err != nil {
			return err
		}
		event, err := requireCalendarEvent(readback, "calendar/get_calendar_detail", eventID)
		if err != nil {
			return err
		}
		if err := verifyCalendarEvent(event, eventID, verify); err != nil {
			return err
		}
		return rt.Output(map[string]any{"success": true, "eventId": eventID, "verified": true, "event": normalizeCalendarEvent(event)})
	},
)

// EventUpdate applies event fields and attendee changes as explicit stages.
// It never rolls back or hides a later-stage failure; errors name completed
// stages so callers can safely inspect state before retrying.
var EventUpdate = calendarWriteShortcut(
	"+update", "分阶段更新日程并读回验证",
	"已知 eventId 时更新标题、描述、完整起止时间或参会人；多阶段写不是事务，任一步失败都会列出已经完成的步骤，成功则读回字段和参会人集合。",
	`dws calendar +update --event <EVENT_ID> --title "新标题" --format json`,
	[]shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Required: true, Desc: "日程 eventId"},
		{Name: "title", Type: shortcut.FlagString, Desc: "新标题"},
		{Name: "desc", Type: shortcut.FlagString, Desc: "新描述"},
		{Name: "start", Type: shortcut.FlagString, Desc: "新开始时间；必须与 --end 同时传"},
		{Name: "end", Type: shortcut.FlagString, Desc: "新结束时间；必须与 --start 同时传"},
		{Name: "timezone", Type: shortcut.FlagString, Desc: "IANA 时区"},
		{Name: "location", Type: shortcut.FlagString, Desc: "新地点文本"},
		{Name: "free-busy", Type: shortcut.FlagString, Desc: "忙闲状态", Enum: []string{"busy", "free"}},
		{Name: "add-attendees", Type: shortcut.FlagStringSlice, Desc: "新增参会人 userId"},
		{Name: "remove-attendees", Type: shortcut.FlagStringSlice, Desc: "移除参会人 userId"},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID（默认 primary）"},
	},
	[]contract.ParamDecl{
		{Name: "event", Property: "eventId"}, {Name: "title", Property: "summary"}, {Name: "desc", Property: "description"},
		{Name: "start", Property: "startDateTime"}, {Name: "end", Property: "endDateTime"}, {Name: "timezone", Property: "timeZone"},
		{Name: "location", Property: "location"}, {Name: "free-busy", Property: "freeBusy"},
		{Name: "add-attendees", Property: "attendeesToAdd"}, {Name: "remove-attendees", Property: "attendeesToRemove"},
		{Name: "calendar-id", Property: "calendarId"},
	},
	func(rt *shortcut.RuntimeContext) error {
		fields, add, remove, err := calendarUpdateParams(rt)
		if err != nil {
			return err
		}
		target := calendarEventTarget(rt)
		preflight, err := rt.CallMCPData("calendar", "get_calendar_detail", target)
		if err != nil {
			return err
		}
		if _, err := requireCalendarEvent(preflight, "calendar/get_calendar_detail", rt.Str("event")); err != nil {
			return err
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "calendar/update", "eventId": rt.Str("event"), "fields": fields, "addAttendees": add, "removeAttendees": remove})
		}
		completed := make([]string, 0, 3)
		if len(fields) > 0 {
			params := calendarEventTarget(rt)
			for key, value := range fields {
				params[key] = value
			}
			written, callErr := rt.CallMCPWriteDataStrict("calendar", "update_calendar_event", params)
			if callErr != nil {
				return calendarStageError("update_event", completed, callErr)
			}
			if _, callErr = requireCalendarWriteResponse(written, "calendar/update_calendar_event"); callErr != nil {
				return calendarStageError("update_event", completed, callErr)
			}
			completed = append(completed, "update_event")
		}
		if len(remove) > 0 {
			params := calendarEventTarget(rt)
			params["attendeesToRemove"] = remove
			written, callErr := rt.CallMCPWriteDataStrict("calendar", "remove_calendar_participant", params)
			if callErr != nil {
				return calendarStageError("remove_attendees", completed, callErr)
			}
			if _, callErr = requireCalendarWriteResponse(written, "calendar/remove_calendar_participant"); callErr != nil {
				return calendarStageError("remove_attendees", completed, callErr)
			}
			completed = append(completed, "remove_attendees")
		}
		if len(add) > 0 {
			params := calendarEventTarget(rt)
			params["attendeesToAdd"] = add
			written, callErr := rt.CallMCPWriteDataStrict("calendar", "add_calendar_participant", params)
			if callErr != nil {
				return calendarStageError("add_attendees", completed, callErr)
			}
			if _, callErr = requireCalendarWriteResponse(written, "calendar/add_calendar_participant"); callErr != nil {
				return calendarStageError("add_attendees", completed, callErr)
			}
			completed = append(completed, "add_attendees")
		}
		readback, err := rt.CallMCPData("calendar", "get_calendar_detail", target)
		if err != nil {
			return calendarStageError("verify_event", completed, err)
		}
		event, err := requireCalendarEvent(readback, "calendar/get_calendar_detail", rt.Str("event"))
		if err != nil {
			return calendarStageError("verify_event", completed, err)
		}
		if err := verifyCalendarEvent(event, rt.Str("event"), fields); err != nil {
			return calendarStageError("verify_event", completed, err)
		}
		if len(add)+len(remove) > 0 {
			participants, callErr := rt.CallMCPData("calendar", "get_calendar_participants", target)
			if callErr != nil {
				return calendarStageError("verify_attendees", completed, callErr)
			}
			rows, callErr := attendeeListProject(participants)
			if callErr != nil {
				return calendarStageError("verify_attendees", completed, callErr)
			}
			if callErr := verifyCalendarAttendees(rows, add, remove); callErr != nil {
				return calendarStageError("verify_attendees", completed, callErr)
			}
		}
		return rt.Output(map[string]any{"success": true, "eventId": rt.Str("event"), "completedSteps": completed, "verified": true, "event": normalizeCalendarEvent(event)})
	},
)

// RSVP maps familiar verbs onto the DingTalk responseStatus values. It always
// reads the event after the write; responseStatus is verified when the backend
// exposes it, otherwise success is explicitly labelled terminal-response-only.
var RSVP = calendarWriteShortcut(
	"+rsvp", "响应日程邀请并读取终态",
	"作为当前参会人接受、拒绝或暂定一个日程邀请；写响应必须非空且非失败，并在写后读取日程，绝不把空 ack 当成功。",
	"dws calendar +rsvp --event <EVENT_ID> --status accept --format json",
	[]shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Required: true, Desc: "日程 eventId"},
		{Name: "status", Type: shortcut.FlagString, Required: true, Desc: "响应动作", Enum: []string{"accept", "decline", "tentative", "needs-action"}},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID（默认 primary）"},
	},
	[]contract.ParamDecl{{Name: "event", Property: "eventId"}, {Name: "status", Property: "responseStatus"}, {Name: "calendar-id", Property: "calendarId"}},
	func(rt *shortcut.RuntimeContext) error {
		status := map[string]string{"accept": "accepted", "decline": "declined", "tentative": "tentative", "needs-action": "needsAction"}[rt.Str("status")]
		target := calendarEventTarget(rt)
		preflight, err := rt.CallMCPData("calendar", "get_calendar_detail", target)
		if err != nil {
			return err
		}
		if _, err := requireCalendarEvent(preflight, "calendar/get_calendar_detail", rt.Str("event")); err != nil {
			return err
		}
		params := calendarEventTarget(rt)
		params["responseStatus"] = status
		if rt.DryRun() {
			return rt.Output(map[string]any{"dryRun": true, "executed": false, "operation": "calendar/respond", "arguments": params})
		}
		written, err := rt.CallMCPWriteDataStrict("calendar", "respond", params)
		if err != nil {
			return err
		}
		written, err = requireCalendarWriteResponse(written, "calendar/respond")
		if err != nil {
			return err
		}
		readback, err := rt.CallMCPData("calendar", "get_calendar_detail", target)
		if err != nil {
			return err
		}
		event, err := requireCalendarEvent(readback, "calendar/get_calendar_detail", rt.Str("event"))
		if err != nil {
			return err
		}
		readStatus := firstCalendarString(event, "responseStatus", "response_status")
		verified := readStatus == status
		if readStatus != "" && !verified {
			return calendarResponseError("calendar/respond", "readback_status_mismatch", "响应日程后读回的 responseStatus 与请求不一致")
		}
		if !verified {
			writeStatus := nestedCalendarString(written, "responseStatus", "response_status", "status")
			if writeStatus != "" && writeStatus != status {
				return calendarResponseError("calendar/respond", "terminal_status_mismatch", "响应接口返回的状态与请求不一致")
			}
			if writeStatus == status {
				verified = true
			}
		}
		return rt.Output(map[string]any{"success": true, "eventId": rt.Str("event"), "responseStatus": status, "verified": verified, "verification": map[string]any{"eventReadback": true, "statusReadback": readStatus != ""}})
	},
)

// EventSearch is an explicitly page-local client-side search. It returns the
// upstream cursor and complete=false whenever another page may exist.
var EventSearch = calendarReadShortcut(
	"+search-event", "在一个日程页内严格搜索事件",
	"DingTalk 没有专用日程全文搜索接口；该命令在 list_calendar_events 当前页按标题、描述和地点过滤，并原样保留 nextCursor/hasMore，绝不把单页零命中说成全局未找到。",
	"events",
	[]shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Required: true, Desc: "标题、描述或地点关键词"},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间，默认今天 00:00"},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间，默认今天 23:59:59"},
		{Name: "calendar-id", Type: shortcut.FlagString, Desc: "日历 ID（默认 primary）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "上一页 nextCursor"},
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "单页条数 1-100"},
	},
	[]contract.ParamDecl{{Name: "start", Property: "startTime"}, {Name: "end", Property: "endTime"}, {Name: "calendar-id", Property: "calendarId"}, {Name: "cursor", Property: "cursor"}, {Name: "limit", Property: "limit"}},
	func(rt *shortcut.RuntimeContext) error {
		params, err := calendarListParams(rt)
		if err != nil {
			return err
		}
		data, err := rt.CallMCPData("calendar", "list_calendar_events", params)
		if err != nil {
			return err
		}
		items, container, err := requireCalendarCollection(data, "calendar/list_calendar_events", "events")
		if err != nil {
			return err
		}
		query := strings.ToLower(rt.Str("query"))
		matched := make([]any, 0)
		for _, item := range items {
			object := item.(map[string]any)
			text := strings.ToLower(strings.Join([]string{
				firstCalendarString(object, "summary", "title", "name"),
				firstCalendarString(object, "description", "desc"),
				firstCalendarString(object, "location", "locationName", "location_name"),
			}, "\n"))
			if strings.Contains(text, query) {
				matched = append(matched, item)
			}
		}
		events, err := projectCalendarRows(matched, "calendar/list_calendar_events", map[string][]string{
			"eventId": {"eventId", "event_id", "id"}, "summary": {"summary", "title", "name"},
			"start": {"start", "startTime", "startDateTime"}, "end": {"end", "endTime", "endDateTime"},
			"status": {"status", "eventStatus"}, "location": {"location", "locationName"},
		}, "eventId")
		if err != nil {
			return err
		}
		page, err := calendarPagination(container, "calendar/list_calendar_events")
		if err != nil {
			return err
		}
		if !page.Known {
			return calendarResponseError("calendar/list_calendar_events", "missing_pagination", "日程搜索响应缺少 hasMore/nextCursor 分页证据，不能把当前页当作完整搜索结果")
		}
		out := map[string]any{"count": len(events), "events": events, "query": rt.Str("query")}
		return outputCalendarPage(rt, out, page)
	},
)

// Suggestion exposes the direct-user-id suggested-time route. Name resolution
// remains the distinct +suggest-time shortcut.
var Suggestion = calendarReadShortcut(
	"+suggestion", "按 userId 严格推荐共同空闲时间",
	"已经知道参与者 userId 时直接查询共同可用时间；显式 recommendEventTimes:[] 才表示没有建议，缺字段或坏元素会失败。按姓名解析请用 +suggest-time。",
	"suggestions",
	[]shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Required: true, Desc: "参与者 userId"},
		{Name: "duration", Type: shortcut.FlagInt, Default: "30", Desc: "会议时长分钟数 1-1440"},
		{Name: "start", Type: shortcut.FlagString, Desc: "推荐范围开始；与 --end 同时传"},
		{Name: "end", Type: shortcut.FlagString, Desc: "推荐范围结束；与 --start 同时传"},
		{Name: "timezone", Type: shortcut.FlagString, Desc: "IANA 时区"},
	},
	[]contract.ParamDecl{{Name: "users", Property: "attendeeUserIds"}, {Name: "duration", Property: "durationMinutes"}, {Name: "start", Property: "start"}, {Name: "end", Property: "end"}, {Name: "timezone", Property: "timeZone"}},
	func(rt *shortcut.RuntimeContext) error {
		users := calendarStringList(rt.StrSlice("users"))
		if len(users) == 0 {
			return apperrors.NewValidation("--users 至少包含一个非空 userId")
		}
		if rt.Int("duration") < 1 || rt.Int("duration") > 1440 {
			return apperrors.NewValidation("--duration 必须在 1-1440 分钟之间")
		}
		if rt.Changed("start") != rt.Changed("end") {
			return apperrors.NewValidation("--start 与 --end 必须同时传入")
		}
		params := map[string]any{"attendeeUserIds": users, "durationMinutes": fmt.Sprint(rt.Int("duration"))}
		if rt.Changed("start") {
			if _, _, err := validateCalendarRange("start", rt.Str("start"), "end", rt.Str("end")); err != nil {
				return err
			}
			params["start"] = rt.Str("start")
			params["end"] = rt.Str("end")
		}
		if rt.Changed("timezone") {
			params["timeZone"] = rt.Str("timezone")
		}
		data, err := rt.CallMCPData("calendar", "list_suggested_event_times", params)
		if err != nil {
			return err
		}
		items, _, err := requireCalendarCollection(data, "calendar/list_suggested_event_times", "recommendEventTimes", "suggestions")
		if err != nil {
			return err
		}
		suggestions, err := projectCalendarRows(items, "calendar/list_suggested_event_times", map[string][]string{
			"start": {"startTime", "start", "startDateTime"}, "end": {"endTime", "end", "endDateTime"},
			"conflicts": {"timeConflictAttendees", "conflicts"},
		}, "start")
		if err != nil {
			return err
		}
		for index, suggestion := range suggestions {
			if calendarEmptyValue(suggestion["end"]) {
				return calendarResponseError("calendar/list_suggested_event_times", "malformed_collection_item", fmt.Sprintf("建议时段第 %d 项缺少结束时间", index))
			}
		}
		return rt.Output(map[string]any{"count": len(suggestions), "suggestions": suggestions, "complete": true})
	},
)

func calendarEventTarget(rt *shortcut.RuntimeContext) map[string]any {
	params := map[string]any{"eventId": rt.Str("event")}
	if calendarID := rt.Str("calendar-id"); calendarID != "" {
		params["calendarId"] = calendarID
	}
	return params
}

func requireCalendarEvent(data map[string]any, operation, expectedID string) (map[string]any, error) {
	event, err := requireCalendarObject(data, operation)
	if err != nil {
		return nil, err
	}
	eventID := calendarEventID(event)
	if eventID == "" {
		return nil, calendarResponseError(operation, "missing_event_id", "日程对象缺少 id/eventId")
	}
	if expectedID != "" && eventID != expectedID {
		return nil, calendarResponseError(operation, "event_id_mismatch", "日程对象 id 与请求 eventId 不一致")
	}
	return event, nil
}

func normalizeCalendarEvent(event map[string]any) map[string]any {
	out := make(map[string]any, len(event)+1)
	for key, value := range event {
		out[key] = value
	}
	out["eventId"] = calendarEventID(event)
	return out
}

func calendarCreateParams(rt *shortcut.RuntimeContext) (map[string]any, map[string]any, error) {
	if len([]rune(rt.Str("title"))) > 2048 {
		return nil, nil, apperrors.NewValidation("--title 不能超过 2048 个字符")
	}
	if len([]rune(rt.Str("desc"))) > 5000 {
		return nil, nil, apperrors.NewValidation("--desc 不能超过 5000 个字符")
	}
	if _, _, err := validateCalendarRange("start", rt.Str("start"), "end", rt.Str("end")); err != nil {
		return nil, nil, err
	}
	attendees := calendarStringList(rt.StrSlice("attendees"))
	if len(attendees) > 500 {
		return nil, nil, apperrors.NewValidation("--attendees 最多 500 个 userId")
	}
	params := map[string]any{"summary": rt.Str("title"), "startDateTime": rt.Str("start"), "endDateTime": rt.Str("end")}
	verify := map[string]any{"summary": rt.Str("title"), "startDateTime": rt.Str("start"), "endDateTime": rt.Str("end")}
	for _, pair := range [][2]string{{"desc", "description"}, {"timezone", "timeZone"}, {"location", "location"}, {"free-busy", "freeBusy"}, {"calendar-id", "calendarId"}} {
		if rt.Changed(pair[0]) {
			params[pair[1]] = rt.Str(pair[0])
			if pair[1] != "calendarId" {
				verify[pair[1]] = rt.Str(pair[0])
			}
		}
	}
	if len(attendees) > 0 {
		params["attendees"] = attendees
	}
	if rooms := calendarStringList(rt.StrSlice("rooms")); len(rooms) > 0 {
		params["roomIds"] = rooms
	}
	return params, verify, nil
}

func calendarUpdateParams(rt *shortcut.RuntimeContext) (map[string]any, []string, []string, error) {
	if rt.Changed("start") != rt.Changed("end") {
		return nil, nil, nil, apperrors.NewValidation("--start 与 --end 必须同时传入")
	}
	if rt.Changed("start") {
		if _, _, err := validateCalendarRange("start", rt.Str("start"), "end", rt.Str("end")); err != nil {
			return nil, nil, nil, err
		}
	}
	fields := map[string]any{}
	for _, pair := range [][2]string{{"title", "summary"}, {"desc", "description"}, {"start", "startDateTime"}, {"end", "endDateTime"}, {"timezone", "timeZone"}, {"location", "location"}, {"free-busy", "freeBusy"}} {
		if rt.Changed(pair[0]) {
			fields[pair[1]] = rt.Str(pair[0])
		}
	}
	add := calendarStringList(rt.StrSlice("add-attendees"))
	remove := calendarStringList(rt.StrSlice("remove-attendees"))
	if len(fields)+len(add)+len(remove) == 0 {
		return nil, nil, nil, apperrors.NewValidation("至少提供一个要更新的字段或参会人变更")
	}
	removeSet := map[string]bool{}
	for _, id := range remove {
		removeSet[id] = true
	}
	for _, id := range add {
		if removeSet[id] {
			return nil, nil, nil, apperrors.NewValidation("同一 userId 不能同时出现在 --add-attendees 与 --remove-attendees")
		}
	}
	return fields, add, remove, nil
}

func verifyCalendarAttendees(rows []map[string]any, added, removed []string) error {
	present := map[string]bool{}
	for _, row := range rows {
		if id := strings.TrimSpace(fmt.Sprint(row["userId"])); id != "" {
			present[id] = true
		}
	}
	for _, id := range added {
		if !present[id] {
			return calendarResponseError("calendar/get_calendar_participants", "readback_attendee_missing", "写后读回未找到新增参会人")
		}
	}
	for _, id := range removed {
		if present[id] {
			return calendarResponseError("calendar/get_calendar_participants", "readback_attendee_present", "写后读回仍包含已移除参会人")
		}
	}
	return nil
}

func calendarStageError(stage string, completed []string, cause error) error {
	message := fmt.Sprintf("日程更新在步骤 %s 失败", stage)
	if len(completed) > 0 {
		message += "；此前已完成且不会自动回滚的步骤: " + strings.Join(completed, ",")
	}
	if cause != nil {
		message += ": " + cause.Error()
	}
	return calendarResponseError("calendar/update", "partial_or_unknown_effect", message)
}

func calendarListParams(rt *shortcut.RuntimeContext) (map[string]any, error) {
	if rt.Int("limit") < 1 || rt.Int("limit") > 100 {
		return nil, apperrors.NewValidation("--limit 必须在 1-100 之间")
	}
	now := time.Now()
	start := rt.Str("start")
	end := rt.Str("end")
	if start == "" {
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format(time.RFC3339)
	}
	if end == "" {
		end = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).Format(time.RFC3339)
	}
	startTime, endTime, err := validateCalendarRange("start", start, "end", end)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"startTime": startTime.UnixMilli(), "endTime": endTime.UnixMilli(), "limit": rt.Int("limit")}
	if value := rt.Str("calendar-id"); value != "" {
		params["calendarId"] = value
	}
	if value := rt.Str("cursor"); value != "" {
		params["cursor"] = value
	}
	return params, nil
}
