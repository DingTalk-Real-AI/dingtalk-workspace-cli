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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Today: list the current user's calendar events for today, with the day
// boundaries computed automatically so the caller never has to hand-craft
// ISO/millisecond time ranges.
//
// It resolves "today" from the machine's local clock: startTime = today 00:00,
// endTime = tomorrow 00:00 (local timezone), both converted to epoch
// milliseconds — exactly the int64 shape the `list_calendar_events` tool
// expects at its helper call site (parseISOTimeToMillis -> millis). calendarId
// defaults to the primary calendar. Replaces the manual
// `calendar event list --start ... --end ...` where you must format two
// timezone-aware timestamps by hand.
//
//	dws calendar +today
var Today = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+today",
	Product:     "calendar",
	Description: "列出我今天的日程（自动计算今天的起止时间，无需手动填时间范围）",
	Intent: "当你想快速看看『我今天有哪些日程/会议安排』时使用；" +
		"内部用本地时区自动把时间范围算成今天 00:00 到次日 00:00，转成毫秒时间戳，" +
		"查询主日历（primary）下今天的全部日程。只读，不会创建或修改任何日程。",
	Risk:  shortcut.RiskRead,
	Flags: []shortcut.Flag{
		// No required flags: "today" is fully derived from the local clock.
	},
	Tips: []string{
		`dws calendar +today`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Today's [00:00, next-day 00:00) window in local time, as epoch millis.
		startOfToday, startOfTomorrow := calendarDayRange(0)
		toolArgs := map[string]any{
			"startTime":  startOfToday.UnixMilli(),
			"endTime":    startOfTomorrow.UnixMilli(),
			"calendarId": "primary",
		}

		// Project each event to {title, start, end, location, eventId} via the
		// shared calendarProjectEvents (same output as +tomorrow/+week) instead
		// of dumping the raw 17-field event objects.
		data, err := rt.CallMCPData("calendar", "list_calendar_events", toolArgs)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"events": calendarProjectEvents(data)})
	},
}

func init() {
	shortcut.Register(Today)
}
