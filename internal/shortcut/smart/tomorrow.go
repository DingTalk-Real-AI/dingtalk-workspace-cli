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

// Tomorrow: list the current user's calendar events for tomorrow, with the day
// boundaries computed automatically — the mirror of +today shifted one day
// forward. Handy for "what's on my plate tomorrow" without hand-crafting a time
// range.
//
// It resolves "tomorrow" from the machine's local clock: startTime = tomorrow
// 00:00, endTime = day-after 00:00 (local timezone), both converted to epoch
// milliseconds — the int64 shape list_calendar_events expects. calendarId
// defaults to the primary calendar. Events are projected to
// {title,start,end,location,eventId}, identical to +today / +week.
//
//	dws calendar +tomorrow
var Tomorrow = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+tomorrow",
	Product:     "calendar",
	Description: "列出我明天的日程（自动计算明天的起止时间，无需手动填时间范围）",
	Intent: "当你想快速看看『我明天有哪些日程/会议安排』、提前准备时使用；" +
		"内部用本地时区自动把时间范围算成明天 00:00 到后天 00:00，转成毫秒时间戳，" +
		"查询主日历（primary）下明天的全部日程，并投影出标题、开始时间、结束时间、地点、eventId。只读，不会创建或修改任何日程。",
	Risk:  shortcut.RiskRead,
	Flags: []shortcut.Flag{
		// No required flags: "tomorrow" is fully derived from the local clock.
	},
	Tips: []string{
		`dws calendar +tomorrow`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Tomorrow's [00:00, day-after 00:00) window in local time, as epoch millis.
		startOfTomorrow, startOfDayafter := calendarDayRange(1)
		toolArgs := map[string]any{
			"startTime":  startOfTomorrow.UnixMilli(),
			"endTime":    startOfDayafter.UnixMilli(),
			"calendarId": "primary",
		}

		// Same {title,start,end,location,eventId} projection as +today/+week.
		data, err := rt.CallMCPData("calendar", "list_calendar_events", toolArgs)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"events": calendarProjectEvents(data)})
	},
}

func init() {
	shortcut.Register(Tomorrow)
}
