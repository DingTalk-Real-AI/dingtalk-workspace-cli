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

// CancelEvent: cancel (delete) an EXISTING calendar event in one step, with a
// confirm-before-delete safety net so you never wipe the wrong eventId.
//
// Steps: first pull the event's detail via get_calendar_detail (so a bad or
// stale eventId fails clearly before any destructive write). Then delete it via
// delete_calendar_event and verify the event is absent.
// Replaces `calendar event get --id` (verify it's the right one) →
// `calendar event delete --id` (destroy it).
//
// The eventId param is copied verbatim from the helper's `event get`
// (get_calendar_detail) and `event delete` (delete_calendar_event) call sites.
// This is a high-risk write; the framework asks for a second confirmation.
//
//	dws calendar +cancel-event --event EVENT_ID
var CancelEvent = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+cancel-event",
	Product:     "calendar",
	Description: "取消（删除）一个已有日程（删除前先确认它真实存在）",
	Intent: "当你想取消/删除一个已经存在的日程时使用；" +
		"内部先用 eventId 拉一次日程详情确认它真实存在，再执行删除并验证它已不存在，" +
		"避免因 eventId 写错而误删别的日程。" +
		"如果 eventId 查不到会直接报错，不会盲目删除。" +
		"这是高危写操作，会真实删除该日程，框架会二次确认。",
	Risk: shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_cancel_event",
			CanonicalPath:  "calendar.shortcut_cancel_event",
			CLIPath:        "calendar +cancel-event",
			PrimaryCLIPath: "calendar +cancel-event",
		},
		Description: "取消（删除）一个已有日程（删除前先确认它真实存在）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "取消（删除）一个已有日程（删除前先确认它真实存在）",
			UseWhen:      []string{"当你想取消/删除一个已经存在的日程时使用；内部先用 eventId 拉一次日程详情确认它真实存在，再执行删除并验证它已不存在，避免因 eventId 写错而误删别的日程。如果 eventId 查不到会直接报错，不会盲目删除。这是高危写操作，会真实删除该日程，框架会二次确认。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws calendar +cancel-event --event EVENT_ID"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "event", Type: shortcut.FlagString, Desc: "要取消的日程 eventId（可用 dws calendar event list 查询）", Required: true},
	},
	Tips: []string{`dws calendar +cancel-event --event EVENT_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		eventID := strings.TrimSpace(rt.Str("event"))
		if eventID == "" {
			return apperrors.NewValidation("--event 不能为空")
		}

		// Step 1 — confirm the event exists before deleting. eventId param copied
		// verbatim from the helper's `event get` call site (get_calendar_detail).
		// If the eventId is bad, this errors out and we never reach the delete.
		detail, err := rt.CallMCPData("calendar", "get_calendar_detail", map[string]any{
			"eventId": eventID,
		})
		if err != nil {
			return err
		}
		if _, err := calendarSmartRequireEvent(detail, "calendar/get_calendar_detail", eventID); err != nil {
			return err
		}

		// Never print the pre-delete event detail: it may carry title, attendee,
		// location, or meeting-room PII and would also create a second JSON value.
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"success":  true,
				"dryRun":   true,
				"executed": false,
				"eventId":  eventID,
			})
		}

		// Step 2 — require a terminal delete receipt and then prove absence.
		if err := calendarSmartDeleteAndVerify(rt, eventID); err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"success":  true,
			"eventId":  eventID,
			"deleted":  true,
			"verified": true,
		})
	},
}

func init() {
	finalizeCalendarSmart(&CancelEvent, "已删除并通过缺席读回验证的日程")
	shortcut.Register(CancelEvent)
}
