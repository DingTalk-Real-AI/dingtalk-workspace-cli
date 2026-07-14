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
		// create_calendar_event params copied verbatim from the helper's create
		// call site: summary + startDateTime/endDateTime (ISO strings, not millis).
		createArgs := map[string]any{
			"summary":       rt.Str("title"),
			"startDateTime": rt.Str("start"),
			"endDateTime":   rt.Str("end"),
		}

		// Fast path: no participants → create and print in one step.
		if !rt.Changed("with") || strings.TrimSpace(rt.Str("with")) == "" {
			return rt.CallMCP("create_calendar_event", createArgs)
		}

		// Step 1 — resolve every participant name to a unique userId BEFORE
		// creating the event, so an unknown/ambiguous name fails cheaply without
		// leaving a dangling event behind.
		var userIDs []string
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
		}
		if len(userIDs) == 0 {
			return apperrors.NewValidation("--with 需要至少一个有效的参会人姓名")
		}

		// Under --dry-run we resolved names (reads) to validate them, but must not
		// create the event or send invites. Preview what would happen and return.
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"dryRun":      true,
				"wouldCreate": createArgs,
				"wouldInvite": userIDs,
			})
		}

		// Step 2 — create the event and pull the eventId from the response.
		created, err := rt.CallMCPWriteData("calendar", "create_calendar_event", createArgs)
		if err != nil {
			return err
		}
		eventID := shortcutExtractEventID(created)
		if eventID == "" {
			return apperrors.NewValidation("日程创建成功但无法从返回结果中解析 eventId，已跳过邀请参会人")
		}

		// Step 3 — add all participants in one batch. attendeesToAdd copied
		// verbatim from the helper's `attendee add` call site.
		if _, err := rt.CallMCPWriteData("calendar", "add_calendar_participant", map[string]any{
			"eventId":        eventID,
			"attendeesToAdd": userIDs,
		}); err != nil {
			// Rollback: delete the just-created event so we never leave a
			// half-built event without its intended attendees.
			_, delErr := rt.CallMCPWriteData("calendar", "delete_calendar_event", map[string]any{
				"eventId": eventID,
			})
			if delErr != nil {
				return apperrors.NewValidation(fmt.Sprintf(
					"添加参会人失败：%v；且回滚删除日程 %s 也失败：%v（请手动删除该日程）",
					err, eventID, delErr))
			}
			return apperrors.NewValidation(fmt.Sprintf(
				"添加参会人失败：%v；已回滚删除日程 %s", err, eventID))
		}

		// Step 4 — success: print the final event detail (eventId + attendees).
		return rt.CallMCP("get_calendar_detail", map[string]any{"eventId": eventID})
	},
}

// shortcutExtractEventID digs the eventId out of a create_calendar_event
// response, tolerating the common shapes ({eventId}, {id}, {result:{...}},
// {event:{...}}) since the exact envelope is not guaranteed.
func shortcutExtractEventID(data map[string]any) string {
	if data == nil {
		return ""
	}
	if id := shortcutPickID(data); id != "" {
		return id
	}
	for _, key := range []string{"result", "event", "data"} {
		if nested, ok := data[key].(map[string]any); ok {
			if id := shortcutPickID(nested); id != "" {
				return id
			}
		}
	}
	return ""
}

func shortcutPickID(m map[string]any) string {
	for _, k := range []string{"eventId", "id"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func init() {
	shortcut.Register(Book)
}
