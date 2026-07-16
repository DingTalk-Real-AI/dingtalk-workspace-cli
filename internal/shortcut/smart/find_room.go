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
	"fmt"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// FindRoom: list meeting rooms that are AVAILABLE within a given time window.
//
// Steps:
//
//  1. parse the ISO8601 --start/--end into epoch millis, exactly as the
//     calendar room-search tool expects (startTime/endTime are int64 millis,
//     mirroring helpers.roomSearch Mode 2);
//
//  2. call query_available_meeting_room with {startTime, endTime} — the same
//     MCP tool + parameter names used by `calendar room search` availability
//     mode (see helpers/calendar.go callMeetingRoomSearchResult);
//
//  3. defensively project each returned room to {roomId, name, capacity} and
//     print the list via rt.Output so it honours --format/--jq/--fields.
//
// Read-only: it only queries availability, it never books or mutates anything.
//
//	dws calendar +find-room --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"
var FindRoom = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+find-room",
	Product:     "calendar",
	Description: "查询指定时间段内所有可用的会议室",
	Intent: "当你想在某个明确的时间段内找出所有当前可预定的空闲会议室（比如临时要约线下会、先看看哪些会议室有空）时使用；" +
		"内部把你给的 ISO8601 起止时间解析成毫秒时间戳，调用会议室可用性查询，只返回该时间范围内可预定的会议室，" +
		"并投影出每个会议室的 roomId、名称与容量，方便你随后用来预订。" +
		"这是纯只读操作，只做可用性查询，不会预订或改动任何会议室或日程；" +
		"注意大部分会议室仅在工作时间可用，非工作时间可能查不到结果，且 start 需为未来时间。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间（ISO8601，如 2026-03-10T14:00:00+08:00，需为未来时间）", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间（ISO8601，如 2026-03-10T15:00:00+08:00）", Required: true},
	},
	Tips: []string{
		`dws calendar +find-room --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — parse the ISO8601 range to epoch millis. The room-search
		// availability tool expects startTime/endTime as int64 millis, exactly
		// like helpers.roomSearch (parseISOTimeToMillis).
		startMillis, err := findRoomParseMillis("start", rt.Str("start"))
		if err != nil {
			return err
		}
		endMillis, err := findRoomParseMillis("end", rt.Str("end"))
		if err != nil {
			return err
		}
		if endMillis <= startMillis {
			return apperrors.NewValidation("--end 必须晚于 --start")
		}

		// Step 2 — query available meeting rooms over the range. tool name +
		// param names copied verbatim from helpers callMeetingRoomSearchResult
		// (query_available_meeting_room) / roomSearch Mode 2 toolArgs.
		data, err := rt.CallMCPData("calendar", "query_available_meeting_room", map[string]any{
			"startTime": startMillis,
			"endTime":   endMillis,
		})
		if err != nil {
			return err
		}

		// Step 3 — project each returned room to {roomId, name, capacity}.
		rooms := make([]map[string]any, 0)
		for _, m := range findRoomExtractRooms(data) {
			rooms = append(rooms, map[string]any{
				"roomId":   findRoomFirstString(m, "roomId", "roomID", "id", "room_id"),
				"name":     findRoomFirstString(m, "roomName", "name", "title", "displayName"),
				"capacity": findRoomCapacity(m),
			})
		}

		return rt.Output(map[string]any{"rooms": rooms})
	},
}

// findRoomParseMillis parses an ISO8601 timestamp into epoch milliseconds,
// returning a clear validation error naming the offending flag.
func findRoomParseMillis(flag, value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf(
			"--%s 时间格式无效：%q，请使用 ISO8601（如 2026-03-10T14:00:00+08:00）", flag, value))
	}
	return t.UnixMilli(), nil
}

// findRoomExtractRooms defensively pulls the room list out of a
// query_available_meeting_room response, tolerating several common shapes:
// the list may sit directly under result, or be nested under a rooms-like key
// inside a result object, or live at the top level.
func findRoomExtractRooms(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	// Candidate containers to probe, in priority order.
	containers := []any{data["result"], data["data"], data}
	listKeys := []string{"rooms", "roomList", "meetingRooms", "list", "items", "records"}

	for _, c := range containers {
		switch v := c.(type) {
		case []any:
			if out := findRoomToMaps(v); len(out) > 0 {
				return out
			}
		case map[string]any:
			for _, k := range listKeys {
				if arr, ok := v[k].([]any); ok {
					if out := findRoomToMaps(arr); len(out) > 0 {
						return out
					}
				}
			}
		}
	}
	return nil
}

// findRoomToMaps keeps only the map elements of a JSON array.
func findRoomToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// findRoomFirstString returns the first non-empty string value among the given
// candidate keys.
func findRoomFirstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// findRoomCapacity reads a room's seating capacity, tolerating numeric
// (float64/int) and string JSON encodings across a few common key names.
func findRoomCapacity(m map[string]any) any {
	for _, k := range []string{"capacity", "maxCapacity", "seatCount", "seats"} {
		switch v := m[k].(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		case string:
			if v != "" {
				return v
			}
		}
	}
	return nil
}

func init() {
	shortcut.Register(FindRoom)
}
