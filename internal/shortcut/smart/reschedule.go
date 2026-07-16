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
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Reschedule: change the time of an EXISTING calendar event in one step, leaving
// every other field (title, description, attendees, rooms, ...) untouched.
//
// Steps: confirm the event exists via get_calendar_detail (so a bad eventId
// fails clearly before any write), then update only its start/end time via
// update_calendar_event. Replaces `calendar event get --id` (verify) →
// `calendar event update --id --start --end` where you must remember not to
// touch anything else.
//
//	dws calendar +reschedule --event EVENT_ID \
//	  --start "2026-03-10T15:00:00+08:00" --end "2026-03-10T16:00:00+08:00"
var Reschedule = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+reschedule",
	Product:     "calendar",
	Description: "改一个已有日程的时间（只动开始/结束时间，其他字段不变）",
	Intent: "当你想把一个已经存在的日程改到新的时间段、又不想动标题/描述/参会人等其他内容时使用；" +
		"内部先用 eventId 拉一次日程详情确认它真实存在，再只更新开始和结束时间。" +
		"如果 eventId 查不到会直接报错，不会误改别的日程。" +
		"会真实修改该日程的时间。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Desc: "要改期的日程 eventId（可用 dws calendar event list 查询）", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "新的开始时间（ISO8601，如 2026-03-10T15:00:00+08:00）", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "新的结束时间（ISO8601，如 2026-03-10T16:00:00+08:00）", Required: true},
	},
	Tips: []string{
		`dws calendar +reschedule --event EVENT_ID --start "2026-03-10T15:00:00+08:00" --end "2026-03-10T16:00:00+08:00"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		eventID := strings.TrimSpace(rt.Str("event"))
		if eventID == "" {
			return apperrors.NewValidation("--event 不能为空")
		}
		start := strings.TrimSpace(rt.Str("start"))
		end := strings.TrimSpace(rt.Str("end"))
		if start == "" || end == "" {
			return apperrors.NewValidation("--start 与 --end 都必须提供（ISO8601 时间字符串）")
		}

		// Step 1 — confirm the event exists. eventId param copied verbatim from the
		// helper's `event get` call site (get_calendar_detail).
		if _, err := rt.CallMCPData("calendar", "get_calendar_detail", map[string]any{
			"eventId": eventID,
		}); err != nil {
			return err
		}

		// Step 2 — update ONLY the time. eventId + startDateTime/endDateTime (ISO
		// strings, not millis) copied verbatim from the helper's `event update`
		// call site (update_calendar_event). No other field is passed, so nothing
		// else on the event is changed.
		return rt.CallMCP("update_calendar_event", map[string]any{
			"eventId":       eventID,
			"startDateTime": start,
			"endDateTime":   end,
		})
	},
}

func init() {
	shortcut.Register(Reschedule)
}
