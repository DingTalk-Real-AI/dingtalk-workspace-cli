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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// CancelEvent: cancel (delete) an EXISTING calendar event in one step, with a
// confirm-before-delete safety net so you never wipe the wrong eventId.
//
// Steps: first pull the event's detail via get_calendar_detail (so a bad or
// stale eventId fails clearly before any destructive write, and the title is
// surfaced back to the user). Then delete it via delete_calendar_event.
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
		"内部先用 eventId 拉一次日程详情确认它真实存在并回显标题，再执行删除，" +
		"避免因 eventId 写错而误删别的日程。" +
		"如果 eventId 查不到会直接报错，不会盲目删除。" +
		"这是高危写操作，会真实删除该日程，框架会二次确认。",
	Risk: shortcut.RiskHighWrite,
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

		// Defensively surface the event title so the user can see what is about to
		// be cancelled. Structure is unknown, so probe several candidate fields at
		// both the top level and under a nested detail/event/data wrapper; if none
		// match we simply skip the projection and proceed to delete.
		title := cancelEventExtractTitle(detail)
		if title != "" {
			_ = rt.Output(map[string]any{
				"eventId": eventID,
				"title":   title,
				"action":  "about_to_cancel",
			})
		}

		// Step 2 — delete the event. eventId param copied verbatim from the
		// helper's `event delete` call site (delete_calendar_event).
		return rt.CallMCP("delete_calendar_event", map[string]any{
			"eventId": eventID,
		})
	},
}

// cancelEventExtractTitle defensively digs a human-readable event title out of
// a get_calendar_detail response. It checks common title-ish keys at the top
// level and inside a single nested wrapper (detail/event/data/result), and
// returns "" when nothing usable is found.
func cancelEventExtractTitle(data map[string]any) string {
	if data == nil {
		return ""
	}
	if t := cancelEventTitleFromMap(data); t != "" {
		return t
	}
	for _, k := range []string{"detail", "event", "data", "result"} {
		if nested, ok := data[k].(map[string]any); ok {
			if t := cancelEventTitleFromMap(nested); t != "" {
				return t
			}
		}
	}
	return ""
}

func cancelEventTitleFromMap(m map[string]any) string {
	for _, k := range []string{"summary", "title", "subject", "name"} {
		if v, ok := m[k].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func init() {
	shortcut.Register(CancelEvent)
}
