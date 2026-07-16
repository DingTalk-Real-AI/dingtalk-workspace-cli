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
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// NextEvent: show the single upcoming calendar event that starts soonest,
// looking at the next 7 days from now.
//
// Steps: list the current user's primary-calendar events over
// [now, now+7d] (list_calendar_events, times in epoch millis, params copied
// verbatim from the helper's `event list` call site) → defensively parse the
// event list → pick the one with the earliest start time that is still in the
// future → print a one-line summary. Replaces eyeballing a full
// `calendar event list` dump just to find "what's next". Read-only.
//
//	dws calendar +next-event
var NextEvent = shortcut.Shortcut{
	Service:     "calendar",
	Command:     "+next-event",
	Product:     "calendar",
	Description: "查看接下来最近的一个日程（默认扫描未来 7 天）",
	Intent: "当你只想知道『我下一个日程是什么、什么时候开始』、而不想翻一整份日程列表时使用；" +
		"内部以当前时间为起点、往后 7 天为范围，拉取主日历下的日程，" +
		"按开始时间升序挑出最近的那一个并打印摘要（标题、开始/结束时间、地点）。" +
		"若这 7 天内没有任何日程，会明确提示『近 7 天无日程』。只读，不做任何修改。",
	Risk:  shortcut.RiskRead,
	Flags: []shortcut.Flag{},
	Tips:  []string{`dws calendar +next-event`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Window: [now, now+7d]. list_calendar_events expects epoch millis for
		// startTime/endTime and calendarId (all copied from the helper's
		// `event list` call site).
		now := time.Now()
		params := map[string]any{
			"calendarId": "primary",
			"startTime":  now.UnixMilli(),
			"endTime":    now.Add(7 * 24 * time.Hour).UnixMilli(),
		}

		data, err := rt.CallMCPData("calendar", "list_calendar_events", params)
		if err != nil {
			return err
		}

		event := shortcutNextEventPick(data, now)
		if event == nil {
			return rt.Output(map[string]any{"event": nil, "message": "近 7 天无日程"})
		}

		// Emit a structured event via rt.Output so it honours
		// --format/--jq/--fields (previously it printed a fixed text line and
		// ignored the output flags, unlike +today/+week).
		return rt.Output(map[string]any{"event": shortcutNextEventProject(event)})
	},
}

// shortcutNextEventPick walks a list_calendar_events response, keeps only valid
// events (must carry an id) whose start time is at or after `now`, and returns
// the one that starts soonest. Field access is fully defensive because the
// response envelope and per-event shape are not guaranteed.
func shortcutNextEventPick(data map[string]any, now time.Time) map[string]any {
	var best map[string]any
	var bestStart time.Time
	for _, e := range shortcutNextEventList(data) {
		id, _ := e["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		start, ok := shortcutNextEventStart(e)
		if !ok {
			continue
		}
		if start.Before(now) {
			continue
		}
		if best == nil || start.Before(bestStart) {
			best = e
			bestStart = start
		}
	}
	return best
}

// shortcutNextEventList flattens the events slice out of the common response
// shapes ({result:{events:[...]}}, {events:[...]}, or a bare [...] under
// result/data).
func shortcutNextEventList(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	var raw []any
	if result, ok := data["result"].(map[string]any); ok {
		if ev, ok := result["events"].([]any); ok {
			raw = ev
		}
	}
	if raw == nil {
		if ev, ok := data["events"].([]any); ok {
			raw = ev
		}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// shortcutNextEventStart pulls an event's start time as a time.Time, tolerating
// either the nested {start:{dateTime: RFC3339}} shape or a flat string field.
func shortcutNextEventStart(event map[string]any) (time.Time, bool) {
	if start, ok := event["start"].(map[string]any); ok {
		if dt, ok := start["dateTime"].(string); ok && dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				return t, true
			}
		}
	}
	for _, k := range []string{"startTime", "startDateTime"} {
		if dt, ok := event[k].(string); ok && dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// shortcutNextEventProject reshapes the chosen event into a structured
// {title,start,end,location,eventId} map, degrading gracefully when optional
// fields are missing. Mirrors the projection used by +today / +week.
func shortcutNextEventProject(event map[string]any) map[string]any {
	title, _ := event["summary"].(string)
	if strings.TrimSpace(title) == "" {
		title = "(无标题)"
	}
	item := map[string]any{"title": title}
	if id, ok := event["id"].(string); ok && strings.TrimSpace(id) != "" {
		item["eventId"] = id
	}
	if start, ok := shortcutNextEventStart(event); ok {
		item["start"] = start.Format("2006-01-02 15:04")
	}
	if end, ok := event["end"].(map[string]any); ok {
		if dt, ok := end["dateTime"].(string); ok && dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				item["end"] = t.Format("2006-01-02 15:04")
			}
		}
	}
	if loc, ok := event["location"].(string); ok && strings.TrimSpace(loc) != "" {
		item["location"] = loc
	}
	return item
}

func init() {
	shortcut.Register(NextEvent)
}
