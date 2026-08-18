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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Book: create a calendar event AND (optionally) invite participants BY NAME,
// in one command, with automatic rollback if inviting fails.
//
// Steps: create the event (summary + start/end) → if --with is given, resolve
// each name to a unique userId and add them all as participants; if that add
// fails, delete the just-created event so we never leave a half-built event
// behind. Replaces `calendar event create` (copy eventId) →
// `contact +search-user` (copy each userId) → `calendar attendee add`.
//
//	dws calendar +book --title "Q1 复盘会" \
//	  --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
//	  --with 张三,李四
var Book = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+book",
	Product:     "calendar",
	Description: "创建日程，并可按姓名邀请参会人（自动解析 userId，失败自动回滚删除日程）",
	Intent: "当你想快速排一个会/日程、并顺手把几位同事按姓名拉进来时使用；" +
		"内部先建日程拿到 eventId，再把每个姓名解析成唯一 userId 批量加为参会人。" +
		"如果加参会人失败，会自动删除刚建好的日程回滚，避免留下一个没人的空日程。" +
		"会真实创建日程并发出参会邀请。",
	Risk: shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "calendar",
			Name:           "shortcut_book",
			CanonicalPath:  "calendar.shortcut_book",
			CLIPath:        "calendar +book",
			PrimaryCLIPath: "calendar +book",
		},
		Description: "创建日程，并可按姓名邀请参会人（自动解析 userId，失败自动回滚删除日程）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "创建日程，并可按姓名邀请参会人（自动解析 userId，失败自动回滚删除日程）",
			UseWhen:      []string{"当你想快速排一个会/日程、并顺手把几位同事按姓名拉进来时使用；内部先建日程拿到 eventId，再把每个姓名解析成唯一 userId 批量加为参会人。如果加参会人失败，会自动删除刚建好的日程回滚，避免留下一个没人的空日程。会真实创建日程并发出参会邀请。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws calendar +book --title \"周会\" --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T15:00:00+08:00\"",
				"dws calendar +book --title \"Q1 复盘会\" --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T15:00:00+08:00\" --with 张三,李四",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "title", Type: shortcut.FlagString, Desc: "日程标题", Required: true},
		{Name: "start", Type: shortcut.FlagString, Desc: "开始时间（ISO8601，如 2026-03-10T14:00:00+08:00）", Required: true},
		{Name: "end", Type: shortcut.FlagString, Desc: "结束时间（ISO8601，如 2026-03-10T15:00:00+08:00）", Required: true},
		{Name: "with", Type: shortcut.FlagString, Desc: "参会人姓名，逗号分隔（可选）"},
	},
	Tips: []string{
		`dws calendar +book --title "周会" --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"`,
		`dws calendar +book --title "Q1 复盘会" --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --with 张三,李四`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		start := rt.Str("start")
		end := rt.Str("end")
		if err := calendarSmartValidateRange(start, end); err != nil {
			return err
		}
		// create_calendar_event params copied verbatim from the helper's create
		// call site: summary + startDateTime/endDateTime (ISO strings, not millis).
		createArgs := map[string]any{
			"summary":       rt.Str("title"),
			"startDateTime": start,
			"endDateTime":   end,
		}

		// Step 1 — resolve every participant name to a unique userId BEFORE
		// creating the event, so an unknown/ambiguous name fails cheaply without
		// leaving a dangling event behind.
		var userIDs []string
		var userNames []string
		withPeople := rt.Changed("with") && strings.TrimSpace(rt.Str("with")) != ""
		if withPeople {
			for _, name := range strings.Split(rt.Str("with"), ",") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				user, err := resolveUser(rt, name)
				if err != nil {
					return err
				}
				userIDs = append(userIDs, user.userID)
				userNames = append(userNames, user.name)
			}
			if len(userIDs) == 0 {
				return apperrors.NewValidation("--with 需要至少一个有效的参会人姓名")
			}
		}

		// Under --dry-run we resolved names (reads) to validate them, but must not
		// create the event or send invites. Preview what would happen and return.
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"success":      true,
				"dryRun":       true,
				"executed":     false,
				"wouldCreate":  createArgs,
				"inviteeCount": len(userIDs),
			})
		}

		// Step 2 — create the event and require an explicit terminal receipt plus
		// a stable id. Empty/unknown write acknowledgements are never success.
		created, err := rt.CallMCPWriteDataStrict("calendar", "create_calendar_event", createArgs)
		if err != nil {
			return err
		}
		if err := calendarSmartWriteReceipt(created, "calendar/create_calendar_event"); err != nil {
			return err
		}
		eventID := calendarSmartEventID(created)
		if eventID == "" {
			return calendarSmartError("calendar/create_calendar_event", "missing_event_id", "创建回执缺少稳定日程 id，远端效果未知")
		}

		// Step 3 — add all participants in one batch. attendeesToAdd copied
		// verbatim from the helper's `attendee add` call site.
		if len(userIDs) > 0 {
			added, addErr := rt.CallMCPWriteDataStrict("calendar", "add_calendar_participant", map[string]any{
				"eventId":        eventID,
				"attendeesToAdd": userIDs,
			})
			if addErr == nil {
				addErr = calendarSmartWriteReceipt(added, "calendar/add_calendar_participant")
			}
			if addErr != nil {
				// Rollback itself is not enough: verify the newly-created event is
				// absent before saying rollback succeeded.
				if rollbackErr := calendarSmartDeleteAndVerify(rt, eventID); rollbackErr != nil {
					return apperrors.NewValidation(fmt.Sprintf(
						"添加参会人失败：%v；回滚删除或删除后验证失败：%v，请人工核查日程状态",
						addErr, rollbackErr))
				}
				return apperrors.NewValidation(fmt.Sprintf("添加参会人失败：%v；新建日程已回滚并验证不存在", addErr))
			}
		}

		// Step 4 — prove the final state before returning one composed result.
		readback, err := rt.CallMCPData("calendar", "get_calendar_detail", map[string]any{"eventId": eventID})
		if err != nil {
			return err
		}
		event, err := calendarSmartRequireEvent(readback, "calendar/get_calendar_detail", eventID)
		if err != nil {
			return err
		}
		if err := calendarSmartVerifyCreatedEvent(event, eventID, rt.Str("title"), start, end); err != nil {
			return err
		}
		if len(userIDs) > 0 {
			participants, err := rt.CallMCPData("calendar", "get_calendar_participants", map[string]any{"eventId": eventID})
			if err != nil {
				return err
			}
			present, err := calendarSmartAttendees(participants)
			if err != nil {
				return err
			}
			currentUserID, err := calendarSmartCurrentUserID(rt, present)
			if err != nil {
				return err
			}
			if err := calendarSmartVerifyAttendees(present, userIDs, userNames, currentUserID); err != nil {
				return err
			}
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
	finalizeCalendarSmart(&Book, "已创建并通过精确读回验证的日程")
	shortcut.Register(Book)
}
