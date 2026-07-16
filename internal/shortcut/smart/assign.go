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

// Assign: create a todo AND assign it to a person by NAME, in one command.
//
// Steps: resolve the assignee name → userId → create a personal todo with that
// user as executor. Replaces `contact +search-user` (copy userId) →
// `todo +create --executors <id>`.
//
//	dws todo +assign --to 张三 --task "整理周报"
var Assign = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+assign",
	Product:     "todo",
	Description: "按姓名给某人创建并指派一条待办（自动解析 userId）",
	Intent: "当你想把一件事指派给某位同事、但只知道对方姓名不想先查 userId 时使用；" +
		"内部先按姓名解析出唯一 userId，再创建待办并把 TA 设为执行人。会真实创建待办。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagString, Desc: "执行人姓名/花名", Required: true},
		{Name: "task", Type: shortcut.FlagString, Desc: "待办标题/内容", Required: true},
		{Name: "due", Type: shortcut.FlagString, Desc: "截止时间（ISO8601，可选）"},
	},
	Tips: []string{`dws todo +assign --to 张三 --task "整理周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the assignee name to a unique userId.
		user, err := resolveUser(rt, rt.Str("to"))
		if err != nil {
			return err
		}

		// Step 2 — create the todo with that user as executor. create_personal_todo
		// accepts executorIds directly, so assignment is part of creation.
		vo := map[string]any{
			"subject":     rt.Str("task"),
			"executorIds": []string{user.userID},
		}
		if rt.Changed("due") {
			// create_personal_todo stores dueTime as epoch millis (int64); the todo
			// helper feeds --due through parseISOTimeToMillis, so mirror that here
			// (shared with +remind's --at) rather than passing a raw ISO string.
			ms, err := shortcutRemindParseMillis("due", rt.Str("due"))
			if err != nil {
				return err
			}
			vo["dueTime"] = ms
		}
		return rt.CallMCP("create_personal_todo", map[string]any{
			"PersonalTodoCreateVO": vo,
		})
	},
}

func init() {
	shortcut.Register(Assign)
}
