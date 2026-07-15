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

// TodoDone: mark one of MY todos complete by matching a keyword in its title.
//
// Steps:
//
//  1. list my todos via get_user_todos_in_current_org (pageNum / pageSize as
//     strings, mirroring helpers.todo list);
//
//  2. scan result.todoCards[] and keep the ones whose subject contains --task;
//     none → "没找到匹配待办", many → list candidates (subject + taskId) so the
//     caller can be more specific, exactly one → take its taskId;
//
//  3. mark it done via update_todo_done_status (taskId + isDone="true",
//     mirroring `dws todo task done`).
//
//     dws todo +todo-done --task 周报
var TodoDone = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+todo-done",
	Product:     "todo",
	Description: "按标题关键词把我的某条待办标记完成（自动定位 taskId）",
	Intent: "当你只记得某条待办的标题关键词、想直接把它标记完成，却不想先翻列表复制 taskId 时使用；" +
		"内部先拉取你当前组织下作为执行人的待办列表，按标题(subject)包含关键词匹配：没匹配到会提示「没找到匹配待办」，" +
		"匹配到多条会列出候选(标题+taskId)让你写得更精确，唯一命中时才把它标记为已完成。这会真实修改待办完成状态。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "task", Type: shortcut.FlagString, Desc: "待办标题关键词", Required: true},
	},
	Tips: []string{`dws todo +todo-done --task 周报`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		keyword := strings.TrimSpace(rt.Str("task"))
		if keyword == "" {
			return apperrors.NewValidation("请用 --task 提供待办标题关键词")
		}

		// Step 1 — list ALL my todos across pages (roleTypes defaults to executor,
		// mirroring helpers.buildListTodoTaskArgs) so a match beyond the first page
		// is still found instead of yielding a false "没找到匹配待办".
		cards, err := shortcutListAllTodoCards(rt, map[string]any{
			"roleTypes": []string{"executor"},
		})
		if err != nil {
			return err
		}

		// Step 2 — match by subject substring.
		matches := shortcutTodoMatch(cards, keyword)
		switch {
		case len(matches) == 0:
			return apperrors.NewValidation(fmt.Sprintf("没找到匹配待办：标题里没有 %q 的待办。", keyword))
		case len(matches) > 1:
			return apperrors.NewValidation(fmt.Sprintf(
				"%q 匹配到 %d 条待办，请用更精确的关键词，或用 `dws todo task done --task-id` 指定：%s",
				keyword, len(matches), strings.Join(shortcutTodoLabels(matches), "；")))
		}

		// Step 3 — mark it done. taskId + isDone mirror helpers `todo task done`
		// (update_todo_done_status, isDone passed as a string).
		return rt.CallMCP("update_todo_done_status", map[string]any{
			"taskId": matches[0].taskID,
			"isDone": "true",
		})
	},
}

// shortcutTodoCard is the minimal identity we need from a todoCards entry.
type shortcutTodoCard struct {
	taskID  string
	subject string
}

// shortcutTodoMatch walks the todoCards ({subject, taskId, ...}) and returns the
// cards whose subject contains keyword (case-insensitive).
func shortcutTodoMatch(cards []map[string]any, keyword string) []shortcutTodoCard {
	kw := strings.ToLower(keyword)
	var out []shortcutTodoCard
	for _, m := range cards {
		subject, _ := m["subject"].(string)
		if subject == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(subject), kw) {
			continue
		}
		id := shortcutTodoTaskID(m)
		if id == "" {
			continue
		}
		out = append(out, shortcutTodoCard{taskID: id, subject: subject})
	}
	return out
}

// shortcutTodoCards pulls the todoCards list out of the response, probing
// wrapper layers defensively.  Real MCP responses may arrive as either
// {"result":{"todoCards":[...]}} or {"result":{"result":{"todoCards":[...]}}}
// depending on the call path, so do not assume a single fixed wrapper.
func shortcutTodoCards(data map[string]any) []map[string]any {
	if arr := shortcutTodoFindCards(data, 0); arr != nil {
		return shortcutTodoToMaps(arr)
	}
	return nil
}

func shortcutTodoFindCards(m map[string]any, depth int) []any {
	if depth > 4 {
		return nil
	}
	if arr, ok := m["todoCards"].([]any); ok {
		return arr
	}
	for _, k := range []string{"result", "data"} {
		if child, ok := m[k].(map[string]any); ok {
			if arr := shortcutTodoFindCards(child, depth+1); arr != nil {
				return arr
			}
		}
	}
	return nil
}

func shortcutTodoToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// shortcutTodoTaskID reads a card's taskId, tolerating both string and numeric
// JSON encodings.
func shortcutTodoTaskID(m map[string]any) string {
	switch v := m["taskId"].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	}
	return ""
}

func shortcutTodoLabels(cards []shortcutTodoCard) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, fmt.Sprintf("%s(taskId=%s)", c.subject, c.taskID))
	}
	return out
}

func init() {
	shortcut.Register(TodoDone)
}
