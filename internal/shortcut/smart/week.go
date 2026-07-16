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
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Week: list the current user's calendar events for the current week (Monday as
// the first day of the week), with the week boundaries computed automatically
// so the caller never has to hand-craft ISO/millisecond time ranges.
//
// It resolves "this week" from the machine's local clock: startTime = this
// Monday 00:00, endTime = next Monday 00:00 (local timezone), both converted to
// epoch milliseconds — exactly the int64 shape the `list_calendar_events` tool
// expects at its helper call site (parseISOTimeToMillis -> millis). calendarId
// defaults to the primary calendar. The response events are defensively parsed
// and projected (title / start / end / eventId) via rt.Output so they honour
// --format/--jq/--fields. Replaces the manual
// `calendar event list --start ... --end ...` where you must format two
// timezone-aware timestamps by hand. Read-only.
//
//	dws calendar +week
var Week = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+week",
	Product:     "calendar",
	Description: "列出我本周的日程（自动按周一为周首计算本周起止时间，无需手动填时间范围）",
	Intent: "当你想快速看看『我本周有哪些日程/会议安排』时使用；" +
		"内部用本地时区、以周一为一周开始，自动把时间范围算成本周一 00:00 到下周一 00:00，转成毫秒时间戳，" +
		"查询主日历（primary）下本周的全部日程，并投影出标题、开始时间、结束时间、eventId。只读，不会创建或修改任何日程。",
	Risk:  shortcut.RiskRead,
	Flags: []shortcut.Flag{
		// No required flags: "this week" is fully derived from the local clock.
	},
	Tips: []string{
		`dws calendar +week`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Compute this week's [Monday 00:00, next-Monday 00:00) window in the
		// local timezone, as epoch milliseconds. list_calendar_events expects
		// startTime/endTime as int64 millis (helper builds them via UnixMilli /
		// parseISOTimeToMillis). Monday is treated as the first day of the week.
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		// Go's Weekday(): Sunday=0..Saturday=6. Offset to the most recent Monday:
		// Monday->0, Tuesday->1, ... Sunday->6.
		offset := (int(startOfDay.Weekday()) + 6) % 7
		startOfWeek := startOfDay.AddDate(0, 0, -offset)
		startOfNextWeek := startOfWeek.AddDate(0, 0, 7)

		// Params copied verbatim from the helper's list_calendar_events call site:
		// startTime/endTime (int64 millis) + calendarId (defaults to primary).
		params := map[string]any{
			"calendarId": "primary",
			"startTime":  startOfWeek.UnixMilli(),
			"endTime":    startOfNextWeek.UnixMilli(),
		}

		data, err := rt.CallMCPData("calendar", "list_calendar_events", params)
		if err != nil {
			return err
		}

		// Same {title,start,end,location,eventId} projection as +today/+tomorrow.
		return rt.Output(map[string]any{"events": calendarProjectEvents(data)})
	},
}

func init() {
	shortcut.Register(Week)
}
