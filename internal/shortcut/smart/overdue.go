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

// Overdue: list MY overdue-and-unfinished todos in one step.
//
// Steps:
//
//  1. list my todos via get_user_todos_in_current_org (pageNum / pageSize as
//     strings, roleTypes=["executor"], mirroring helpers.todo list);
//
//  2. in Go, keep only the cards whose dueTime exists and is strictly before
//     time.Now().UnixMilli() AND that are not yet done (isDone not true and
//     finalStatusStage not a completed marker) — field parsing is defensive;
//
//  3. project each surviving card to {subject, dueTime, taskId} and print the
//     list via rt.Output so it honours --format/--jq/--fields.
//
// Read-only: it never mutates any todo, it only lists and filters locally.
//
//	dws todo +overdue
var Overdue = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+overdue",
	Product:     "todo",
	Description: "列出我已过期未完成的待办",
	Intent: "当你想快速看清自己有哪些待办已经过了截止时间却还没做完、方便优先处理时使用；" +
		"内部先拉取你当前组织下作为执行人(executor)的待办列表，再在本地按「有截止时间(dueTime) 且早于当前时刻 且尚未完成」的条件筛选，" +
		"最后只打印这些逾期待办的标题(subject)、截止时间(dueTime) 和任务 ID(taskId)。" +
		"这是纯只读操作，只做列表与本地过滤，不会修改或完成任何待办；若没有逾期待办则返回空列表。",
	Risk:  shortcut.RiskRead,
	Flags: []shortcut.Flag{},
	Tips: []string{
		`dws todo +overdue`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list ALL my todos across pages (roleTypes defaults to executor,
		// mirroring helpers.buildListTodoTaskArgs) so an overdue todo beyond the
		// first page is not silently undercounted.
		cards, err := shortcutListAllTodoCards(rt, map[string]any{
			"roleTypes": []string{"executor"},
		})
		if err != nil {
			return err
		}

		// Step 2 — filter locally: dueTime present, in the past, not done.
		now := time.Now().UnixMilli()
		overdue := make([]map[string]any, 0, len(cards))
		for _, m := range cards {
			due, ok := shortcutOverdueDueTime(m)
			if !ok || due >= now {
				continue
			}
			if shortcutOverdueIsDone(m) {
				continue
			}
			taskID := shortcutTodoTaskID(m) // reused from todo_done.go
			subject, _ := m["subject"].(string)
			overdue = append(overdue, map[string]any{
				"subject": subject,
				"dueTime": due,
				"taskId":  taskID,
			})
		}

		// Step 3 — print the projected overdue list.
		return rt.Output(map[string]any{"overdue": overdue})
	},
}

// shortcutOverdueDueTime reads a card's dueTime as epoch millis, tolerating both
// numeric (float64) and string JSON encodings. Returns ok=false when the field
// is absent, zero, or unparseable.
func shortcutOverdueDueTime(m map[string]any) (int64, bool) {
	for _, key := range []string{"dueTime", "planFinishDate", "gmtDue"} {
		switch v := m[key].(type) {
		case float64:
			if v > 0 {
				return int64(v), true
			}
		case int64:
			if v > 0 {
				return v, true
			}
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				continue
			}
			// dueTime is normally epoch millis; parse leniently.
			var n int64
			var neg bool
			ok := len(s) > 0
			for i, c := range s {
				if i == 0 && c == '-' {
					neg = true
					continue
				}
				if c < '0' || c > '9' {
					ok = false
					break
				}
				n = n*10 + int64(c-'0')
			}
			if ok && n > 0 && !neg {
				return n, true
			}
		}
	}
	return 0, false
}

// shortcutOverdueIsDone reports whether a card is already completed, probing the
// common completion markers defensively: a boolean isDone, or a
// finalStatusStage/statusStage string that reads as finished/done.
func shortcutOverdueIsDone(m map[string]any) bool {
	switch v := m["isDone"].(type) {
	case bool:
		if v {
			return true
		}
	case string:
		if strings.EqualFold(strings.TrimSpace(v), "true") {
			return true
		}
	}
	if v, ok := m["done"].(bool); ok && v {
		return true
	}
	for _, key := range []string{"finalStatusStage", "statusStage", "status"} {
		if s, ok := m[key].(string); ok {
			s = strings.ToUpper(strings.TrimSpace(s))
			switch s {
			case "DONE", "FINISHED", "COMPLETED", "COMPLETE":
				return true
			}
		}
	}
	return false
}

func init() {
	shortcut.Register(Overdue)
}
