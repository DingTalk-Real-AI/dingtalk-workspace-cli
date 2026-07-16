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
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// DueToday: list MY todos whose planFinishDate falls within today.
//
// Unlike +overdue (which locally keeps cards whose dueTime is already in the
// past), this shortcut filters SERVER-SIDE: it passes the today window as
// planFinishDateStart / planFinishDateEnd (epoch millis) to
// get_user_todos_in_current_org, so the backend returns only todos due today.
//
// Steps:
//
//  1. compute the [today 00:00, tomorrow 00:00) window in the local timezone,
//     as epoch millis;
//
//  2. list my todos via get_user_todos_in_current_org with
//     planFinishDateStart = today 00:00 ms, planFinishDateEnd = tomorrow 00:00
//     ms, roleTypes=["executor"] (mirroring +overdue), pageNum/pageSize as
//     strings (mirroring helpers.todo list);
//
//  3. project each card to a clean {title, status, priority, creator,
//     planFinishDate, taskId} shape and print via rt.Output so it honours
//     --format/--jq/--fields.
//
// Read-only: it never mutates any todo, it only lists and projects.
//
//	dws todo +due-today
var DueToday = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+due-today",
	Product:     "todo",
	Description: "列出我今天到期的待办",
	Intent: "当你想快速看清自己今天（planFinishDate 落在今天 00:00 到次日 00:00 之间）到期的待办、方便安排一天的工作时使用；" +
		"内部按今天的本地时间窗，把 planFinishDateStart=今天0点、planFinishDateEnd=次日0点（毫秒时间戳）传给 get_user_todos_in_current_org 做服务端过滤，" +
		"默认拉取你作为执行人(executor)的待办，可用 --role-types 覆盖角色范围，最后只打印这些今天到期待办的标题、状态、优先级、创建人、到期时间和任务 ID。" +
		"这与 +overdue（已过期）不同：+overdue 看的是已经过了截止时间的待办，本命令看的是今天当天到期的待办。" +
		"这是纯只读操作，只做列表与投影，不会修改或完成任何待办；若今天没有到期的待办则返回错误提示。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{
			Name: "role-types",
			Type: shortcut.FlagString,
			Desc: "覆盖默认角色范围，逗号分隔，取值 creator/executor/participant；不传则默认 executor",
		},
	},
	Tips: []string{
		`dws todo +due-today`,
		`dws todo +due-today --role-types creator,executor`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — compute [today 00:00, tomorrow 00:00) in the local timezone.
		now := time.Now()
		loc := now.Location()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		startOfNextDay := startOfDay.AddDate(0, 0, 1)
		startMs := startOfDay.UnixMilli()
		endMs := startOfNextDay.UnixMilli()

		// Step 2 — build params. roleTypes defaults to executor (like +overdue);
		// --role-types overrides it (validated like helpers.parseRoleTypes). The
		// today window is passed as planFinishDateStart / planFinishDateEnd so the
		// backend filters server-side.
		roleTypes := []string{"executor"}
		if rt.Changed("role-types") {
			parsed, err := parseRelatedRoleTypes(rt.Str("role-types"))
			if err != nil {
				return err
			}
			if len(parsed) > 0 {
				roleTypes = parsed
			}
		}

		params := map[string]any{
			"roleTypes":           roleTypes,
			"planFinishDateStart": startMs,
			"planFinishDateEnd":   endMs,
		}

		// List ALL matching cards across pages so a todo due today beyond the
		// first page is not silently dropped.
		cards, err := shortcutListAllTodoCards(rt, params)
		if err != nil {
			return err
		}

		// Step 3 — project the cards. The server already filtered to today's
		// window; we keep a defensive local guard against any planFinishDate that
		// leaks outside [startMs, endMs).
		results := make([]map[string]any, 0, len(cards))
		for _, m := range cards {
			if due, ok := shortcutOverdueDueTime(m); ok { // reused from overdue.go
				if due < startMs || due >= endMs {
					continue
				}
			}
			taskID := shortcutRelatedTaskID(m)                           // reused from related_tasks.go
			results = append(results, shortcutRelatedProject(m, taskID)) // reused from related_tasks.go
		}

		if len(results) == 0 {
			return apperrors.NewValidation("今天没有到期的待办")
		}

		// Step 4 — print the projected list.
		return rt.Output(map[string]any{"count": len(results), "tasks": results})
	},
}

func init() {
	shortcut.Register(DueToday)
}
