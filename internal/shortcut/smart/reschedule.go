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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
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
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_reschedule",
			CanonicalPath:  "calendar.shortcut_reschedule",
			CLIPath:        "calendar +reschedule",
			PrimaryCLIPath: "calendar +reschedule",
		},
		Description: "改一个已有日程的时间（只动开始/结束时间，其他字段不变）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "改一个已有日程的时间（只动开始/结束时间，其他字段不变）",
			UseWhen:      []string{"当你想把一个已经存在的日程改到新的时间段、又不想动标题/描述/参会人等其他内容时使用；内部先用 eventId 拉一次日程详情确认它真实存在，再只更新开始和结束时间。如果 eventId 查不到会直接报错，不会误改别的日程。会真实修改该日程的时间。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +reschedule --event EVENT_ID --start \"2026-03-10T15:00:00+08:00\" --end \"2026-03-10T16:00:00+08:00\""},
		},
	},
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
		if err := calendarSmartValidateRange(start, end); err != nil {
			return err
		}

		// Step 1 — confirm the event exists. eventId param copied verbatim from the
		// helper's `event get` call site (get_calendar_detail).
		preflight, err := rt.CallMCPData("calendar", "get_calendar_detail", map[string]any{
			"eventId": eventID,
		})
		if err != nil {
			return err
		}
		if _, err := calendarSmartRequireEvent(preflight, "calendar/get_calendar_detail", eventID); err != nil {
			return err
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"success":  true,
				"dryRun":   true,
				"executed": false,
				"eventId":  eventID,
				"start":    start,
				"end":      end,
			})
		}

		// Step 2 — update ONLY the time. eventId + startDateTime/endDateTime (ISO
		// strings, not millis) copied verbatim from the helper's `event update`
		// call site (update_calendar_event). No other field is passed, so nothing
		// else on the event is changed.
		written, err := rt.CallMCPWriteDataStrict("calendar", "update_calendar_event", map[string]any{
			"eventId":       eventID,
			"startDateTime": start,
			"endDateTime":   end,
		})
		if err != nil {
			return err
		}
		if err := calendarSmartWriteReceipt(written, "calendar/update_calendar_event"); err != nil {
			return err
		}
		readback, err := rt.CallMCPData("calendar", "get_calendar_detail", map[string]any{"eventId": eventID})
		if err != nil {
			return err
		}
		event, err := calendarSmartRequireEvent(readback, "calendar/get_calendar_detail", eventID)
		if err != nil {
			return err
		}
		if err := calendarSmartVerifyEventTimes(event, start, end); err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"success":  true,
			"eventId":  eventID,
			"verified": true,
			"event":    event,
		})
	},
}

func init() {
	finalizeCalendarSmart(&Reschedule, "已改期并通过精确时间读回验证的日程")
	shortcut.Register(Reschedule)
}
