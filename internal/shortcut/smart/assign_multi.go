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

// AssignMulti: create ONE personal todo and assign it to SEVERAL people at once,
// addressing every executor by NAME instead of by userId.
//
// Steps: split the --to name CSV, resolve each name to a unique userId
// (resolveUser, which never guesses — it errors on unknown/ambiguous names),
// collect ALL resolution errors first and abort if any name failed, so we never
// create a half-assigned todo. Then create the todo once via create_personal_todo
// with every resolved userId in executorIds. Replaces the manual dance of
// `contact user search` per name → copy each userId →
// `todo task create --executors id1,id2,... --title ...`.
//
// tool name + params (create_personal_todo, PersonalTodoCreateVO.subject /
// .executorIds) are copied verbatim from the todo helper's `task create` call
// site.
//
//	dws todo +assign-multi --to "张三,李四,王五" --task "周五前提交排期"
var AssignMulti = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+assign-multi",
	Product:     "todo",
	Description: "把一条待办按姓名一次性指派给多个人（自动把每个姓名解析成 userId）",
	Intent: "当你想把同一条待办同时指派给好几个同事、但手上只有他们的姓名而不是 userId 时使用；" +
		"内部会把 --to 里的每个姓名逐个解析成唯一 userId，只要有任何一个姓名查不到或者重名有歧义，" +
		"就把这些问题一次性汇总报错、并且完全不创建待办（不会建出只指派了一半人的残缺待办）。" +
		"全部姓名都解析成功后，才用这些 userId 一次性创建这条待办并指派给所有人。会真实创建一条新的待办。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagStringSlice, Desc: "执行人姓名/花名，逗号分隔（如 张三,李四）", Required: true},
		{Name: "task", Type: shortcut.FlagString, Desc: "待办标题", Required: true},
	},
	Tips: []string{
		`dws todo +assign-multi --to "张三,李四" --task "周五前提交排期"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		task := strings.TrimSpace(rt.Str("task"))
		if task == "" {
			return apperrors.NewValidation("--task 不能为空（待办标题）")
		}

		// Normalize the --to name list, dropping blanks.
		var names []string
		for _, n := range rt.StrSlice("to") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			return apperrors.NewValidation("--to 不能为空，请至少提供一个执行人姓名")
		}

		// Resolve every name up front. Collect all failures and abort before any
		// write, so we never create a todo assigned to only some of the people.
		var (
			executorIDs []string
			resolved    []string
			failures    []string
		)
		for _, name := range names {
			user, err := resolveUser(rt, name)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s", name, err.Error()))
				continue
			}
			executorIDs = append(executorIDs, user.userID)
			resolved = append(resolved, fmt.Sprintf("%s(%s)", user.name, user.userID))
		}
		if len(failures) > 0 {
			return apperrors.NewValidation(fmt.Sprintf(
				"以下执行人姓名无法唯一解析，已中止、未创建任何待办：\n%s",
				strings.Join(failures, "\n")))
		}
		if len(executorIDs) == 0 {
			return apperrors.NewValidation("没有解析出任何有效的执行人 userId，已中止")
		}

		// Create the todo once, assigning all resolved executors. Params mirror the
		// todo helper's `task create` call site verbatim.
		if err := rt.CallMCP("create_personal_todo", map[string]any{
			"PersonalTodoCreateVO": map[string]any{
				"subject":     task,
				"executorIds": executorIDs,
			},
		}); err != nil {
			return err
		}

		return rt.Output(map[string]any{
			"subject":   task,
			"executors": resolved,
			"count":     len(executorIDs),
		})
	},
}

func init() {
	shortcut.Register(AssignMulti)
}
