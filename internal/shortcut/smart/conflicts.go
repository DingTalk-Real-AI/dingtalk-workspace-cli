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
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Conflicts: detect time-overlapping events on my calendar for a day — the
// scheduling sanity check ("do I have any double-bookings?") that neither the
// 1:1 layer offers. dws-native value.
//
// It lists list_calendar_events for the target day (default today, or N days
// ahead via --in-days), then locally finds every pair of events whose [start,end)
// intervals overlap and reports them. Read-only.
//
//	dws calendar +conflicts
//	dws calendar +conflicts --in-days 1
var Conflicts = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+conflicts",
	Product:     "calendar",
	Description: "检测我某天日程的时间冲突（重叠/双重预订，默认今天）",
	Intent: "当你想快速知道『我今天（或某天）的日程有没有时间冲突、撞车的会议』时使用；" +
		"内部自动算出目标日期的时间范围（默认今天，可用 --in-days 指定几天后），列出当天全部日程，" +
		"再在本地两两比对开始/结束时间，找出所有时间段重叠的日程对并报告。" +
		"只读操作，不修改任何日程；没有冲突时明确告诉你「无冲突」。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_conflicts",
			CanonicalPath:  "calendar.shortcut_conflicts",
			CLIPath:        "calendar +conflicts",
			PrimaryCLIPath: "calendar +conflicts",
		},
		Description: "检测我某天日程的时间冲突（重叠/双重预订，默认今天）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "检测我某天日程的时间冲突（重叠/双重预订，默认今天）",
			UseWhen:      []string{"当你想快速知道『我今天（或某天）的日程有没有时间冲突、撞车的会议』时使用；内部自动算出目标日期的时间范围（默认今天，可用 --in-days 指定几天后），列出当天全部日程，再在本地两两比对开始/结束时间，找出所有时间段重叠的日程对并报告。只读操作，不修改任何日程；没有冲突时明确告诉你「无冲突」。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws calendar +conflicts",
				"dws calendar +conflicts --in-days 1",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "in-days", Type: shortcut.FlagInt, Desc: "几天后（可选，0=今天默认，1=明天…）", Required: false},
	},
	Tips: []string{
		`dws calendar +conflicts`,
		`dws calendar +conflicts --in-days 1`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Target day window in local time.
		dayStart, dayEnd := calendarDayRange(rt.Int("in-days"))

		events, err := calendarSmartListAll(rt, map[string]any{
			"startTime":  dayStart.UnixMilli(),
			"endTime":    dayEnd.UnixMilli(),
			"calendarId": "primary",
		})
		if err != nil {
			return err
		}

		// Collect events that carry both a start and an end, keeping the parsed
		// times for overlap comparison.
		type ev struct {
			title string
			start time.Time
			end   time.Time
		}
		var evs []ev
		for _, e := range events {
			start, ok := shortcutNextEventStart(e)
			if !ok {
				continue
			}
			end, ok := conflictsEndTime(e)
			if !ok {
				continue
			}
			title, _ := e["summary"].(string)
			if strings.TrimSpace(title) == "" {
				title = "(无标题)"
			}
			evs = append(evs, ev{title: title, start: start, end: end})
		}
		sort.Slice(evs, func(i, j int) bool { return evs[i].start.Before(evs[j].start) })

		// Find every overlapping pair. Two intervals [s1,e1),[s2,e2) overlap iff
		// s1 < e2 && s2 < e1.
		conflicts := make([]map[string]any, 0)
		for i := 0; i < len(evs); i++ {
			for j := i + 1; j < len(evs); j++ {
				if evs[i].start.Before(evs[j].end) && evs[j].start.Before(evs[i].end) {
					conflicts = append(conflicts, map[string]any{
						"a":            conflictsLabel(evs[i].title, evs[i].start, evs[i].end),
						"b":            conflictsLabel(evs[j].title, evs[j].start, evs[j].end),
						"overlapStart": conflictsMax(evs[i].start, evs[j].start).Format("15:04"),
						"overlapEnd":   conflictsMin(evs[i].end, evs[j].end).Format("15:04"),
					})
				}
			}
		}

		return rt.Output(map[string]any{
			"date":          dayStart.Format("2006-01-02"),
			"eventCount":    len(evs),
			"conflictCount": len(conflicts),
			"hasConflict":   len(conflicts) > 0,
			"conflicts":     conflicts,
			"complete":      true,
		})
	},
}

// conflictsEndTime parses a timed event or a legitimate all-day event.
func conflictsEndTime(e map[string]any) (time.Time, bool) {
	end, ok := e["end"].(map[string]any)
	if ok {
		if dt, ok := end["dateTime"].(string); ok && dt != "" {
			t, err := time.Parse(time.RFC3339, dt)
			return t, err == nil
		}
		if date, ok := end["date"].(string); ok && date != "" {
			t, err := time.ParseInLocation("2006-01-02", date, time.Local)
			return t, err == nil
		}
	}
	for _, key := range []string{"endTime", "endDateTime"} {
		if dt, ok := e[key].(string); ok && dt != "" {
			t, err := time.Parse(time.RFC3339, dt)
			if err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func conflictsLabel(title string, start, end time.Time) string {
	return title + "（" + start.Format("15:04") + "-" + end.Format("15:04") + "）"
}

func conflictsMax(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func conflictsMin(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func init() {
	finalizeCalendarSmart(&Conflicts, "完整事件集上的时间冲突分析")
	shortcut.Register(Conflicts)
}
