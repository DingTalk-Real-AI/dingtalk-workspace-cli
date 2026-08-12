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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
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
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_today",
			CanonicalPath:  "calendar.shortcut_today",
			CLIPath:        "calendar +today",
			PrimaryCLIPath: "calendar +today",
		},
		Description: "列出我今天的日程（自动计算今天的起止时间，无需手动填时间范围）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出我今天的日程（自动计算今天的起止时间，无需手动填时间范围）",
			UseWhen:      []string{"当你想快速看看『我今天有哪些日程/会议安排』时使用；内部用本地时区自动把时间范围算成今天 00:00 到次日 00:00，转成毫秒时间戳，查询主日历（primary）下今天的全部日程。只读，不会创建或修改任何日程。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +today"},
		},
	},
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
