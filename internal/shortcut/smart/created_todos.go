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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// CreatedTodos: list the todos I created (rather than the ones assigned to me
// to execute) in one step.
//
// Steps:
//
//  1. list my todos via get_user_todos_in_current_org (pageNum / pageSize as
//     strings, mirroring helpers.todo list) but with roleTypes=["creator"] so
//     the server returns todos where I am the creator. "creator" is one of the
//     three enum values accepted by helpers.parseRoleTypes
//     (creator / executor / participant), so this is a supported role value.
//
//  2. project each todoCards[] entry to {subject, taskId, dueTime} with
//     defensive field parsing (reusing shortcutTodoCards / shortcutTodoTaskID /
//     shortcutOverdueDueTime), and print via rt.Output so it honours
//     --format/--jq/--fields.
//
// Read-only: it only lists and projects, it never creates or mutates any todo.
//
//	dws todo +created-todos
var CreatedTodos = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+created-todos",
	Product:     "todo",
	Description: "列出我创建的待办（我作为创建人 creator 发起的待办，而非分配给我执行的）",
	Intent: "当你想快速看清『哪些待办是我自己创建/发起的』，而不是别人指派给我执行的待办时使用；" +
		"内部拉取你当前组织下角色为创建人(creator)的待办列表（roleTypes=[\"creator\"]，" +
		"creator 是待办列表支持的角色枚举之一），再在本地把每条待办投影成标题(subject)、" +
		"任务 ID(taskId) 和截止时间(dueTime) 打印出来。这是纯只读操作，只做列表与投影，" +
		"不会创建或修改任何待办；若没有你创建的待办则返回空列表。",
	Risk:  shortcut.RiskRead,
	Flags: []shortcut.Flag{
		// No required flags: "my created todos" is fully derived from the
		// current identity plus roleTypes=["creator"].
	},
	Tips: []string{
		`dws todo +created-todos`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my todos as creator. pageNum/pageSize are strings and
		// roleTypes=["creator"] mirrors helpers.buildListTodoTaskArgs, where
		// "creator" is an accepted role-type enum value.
		data, err := rt.CallMCPData("todo", "get_user_todos_in_current_org", map[string]any{
			"pageNum":   "1",
			"pageSize":  "50",
			"roleTypes": []string{"creator"},
		})
		if err != nil {
			return err
		}

		// Step 2 — project each card to {subject, taskId, dueTime}, parsing
		// fields defensively (helpers reused from todo_done.go / overdue.go).
		cards := shortcutTodoCards(data)
		created := make([]map[string]any, 0, len(cards))
		for _, m := range cards {
			subject, _ := m["subject"].(string)
			item := map[string]any{
				"subject": subject,
				"taskId":  shortcutTodoTaskID(m),
			}
			if due, ok := shortcutOverdueDueTime(m); ok {
				item["dueTime"] = due
			}
			created = append(created, item)
		}

		// Step 3 — print the projected list of todos I created.
		return rt.Output(map[string]any{"created": created})
	},
}

func init() {
	shortcut.Register(CreatedTodos)
}
