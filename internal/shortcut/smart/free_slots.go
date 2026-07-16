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
	"sort"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// FreeSlots: find the OPEN gaps in my day — "when can I fit a meeting?" — the
// complement of +conflicts. Not offered by the raw 1:1 layer.
//
// It lists list_calendar_events for the target day (default today, --in-days N
// ahead), merges the busy intervals, and reports the free windows inside a
// working-hours range (default 09:00–18:00, override with --from/--to hour).
// Read-only.
//
//	dws calendar +free-slots
//	dws calendar +free-slots --in-days 1 --from 9 --to 20
var FreeSlots = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+free-slots",
	Product:     "calendar",
	Description: "找我某天工作时段内的空闲时间段（默认今天 09:00-18:00）",
	Intent: "当你想知道『我今天（或某天）还有哪些时间是空的、可以安排会议/事情』时使用；" +
		"内部列出目标日期（默认今天，--in-days 指定几天后）的全部日程，合并忙碌时段，" +
		"再在工作时间范围内（默认 09:00-18:00，可用 --from/--to 指定起止小时）算出所有空闲窗口并给出每段的起止与时长。" +
		"只读操作，不修改任何日程；用于快速定位可预约的空档。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "in-days", Type: shortcut.FlagInt, Desc: "几天后（可选，0=今天默认）", Required: false},
		{Name: "from", Type: shortcut.FlagInt, Desc: "工作时段起始小时（可选，默认 9）", Required: false},
		{Name: "to", Type: shortcut.FlagInt, Desc: "工作时段结束小时（可选，默认 18）", Required: false},
	},
	Tips: []string{
		`dws calendar +free-slots`,
		`dws calendar +free-slots --in-days 1 --from 9 --to 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		now := time.Now()
		offset := rt.Int("in-days")
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, offset)

		fromHour, toHour := 9, 18
		if rt.Changed("from") {
			fromHour = rt.Int("from")
		}
		if rt.Changed("to") {
			toHour = rt.Int("to")
		}
		if fromHour < 0 || fromHour > 23 || toHour < 1 || toHour > 24 || toHour <= fromHour {
			return apperrors.NewValidation("--from/--to 需满足 0<=from<to<=24")
		}
		workStart := day.Add(time.Duration(fromHour) * time.Hour)
		workEnd := day.Add(time.Duration(toHour) * time.Hour)

		// List the day's events (a little wider than the work window so events
		// spanning the edges are captured).
		data, err := rt.CallMCPData("calendar", "list_calendar_events", map[string]any{
			"startTime":  day.UnixMilli(),
			"endTime":    day.AddDate(0, 0, 1).UnixMilli(),
			"calendarId": "primary",
		})
		if err != nil {
			return err
		}

		// Collect and clip busy intervals to the work window.
		type interval struct{ start, end time.Time }
		var busy []interval
		for _, e := range shortcutNextEventList(data) {
			s, ok := shortcutNextEventStart(e)
			if !ok {
				continue
			}
			en, ok := conflictsEndTime(e)
			if !ok {
				continue
			}
			if en.After(workStart) && s.Before(workEnd) {
				if s.Before(workStart) {
					s = workStart
				}
				if en.After(workEnd) {
					en = workEnd
				}
				busy = append(busy, interval{s, en})
			}
		}
		sort.Slice(busy, func(i, j int) bool { return busy[i].start.Before(busy[j].start) })

		// Sweep the work window, emitting the gaps between merged busy blocks.
		free := make([]map[string]any, 0)
		cursor := workStart
		for _, b := range busy {
			if b.start.After(cursor) {
				free = append(free, freeSlotEntry(cursor, b.start))
			}
			if b.end.After(cursor) {
				cursor = b.end
			}
		}
		if cursor.Before(workEnd) {
			free = append(free, freeSlotEntry(cursor, workEnd))
		}

		return rt.Output(map[string]any{
			"date":      day.Format("2006-01-02"),
			"window":    fmt.Sprintf("%02d:00-%02d:00", fromHour, toHour),
			"slotCount": len(free),
			"freeSlots": free,
			"allBusy":   len(free) == 0,
		})
	},
}

// freeSlotEntry renders a free window with its duration in minutes.
func freeSlotEntry(start, end time.Time) map[string]any {
	return map[string]any{
		"start":       start.Format("15:04"),
		"end":         end.Format("15:04"),
		"durationMin": int(end.Sub(start).Minutes()),
	}
}

func init() {
	shortcut.Register(FreeSlots)
}
