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

// RelatedTasks: list ALL todos related to me in one step — the union of the
// todos where I am the creator, the executor, or a participant — mirroring
// the creator+executor+participant roles-union view.
//
// Steps:
//
//  1. list my todos via get_user_todos_in_current_org (pageNum / pageSize as
//     strings, mirroring helpers.todo list) with roleTypes defaulting to all
//     three enum values ["creator","executor","participant"], so the server
//     returns todos where I hold ANY of those roles. creator / executor /
//     participant are exactly the values accepted by helpers.parseRoleTypes.
//     --role-types (CSV) can override the default; --status is passed through
//     as todoStatus, both mirroring helpers.buildListTodoTaskArgs;
//
//  2. dedupe the returned cards by taskId (one todo can appear more than once
//     when I hold several roles on it), then project each surviving card to a
//     clean {title, status, priority, creator, planFinishDate, taskId} shape
//     with defensive multi-key field probing;
//
//  3. print the deduped list via rt.Output so it honours --format/--jq/--fields.
//
// Read-only: it only lists, dedupes and projects, it never mutates any todo.
//
//	dws todo +related-tasks
var RelatedTasks = shortcut.Shortcut{
	Service:     "todo",
	Command:     "+related-tasks",
	Product:     "todo",
	Description: "一次性列出与我相关的全部待办（我作为创建人/执行人/参与人三种角色的并集，按 taskId 去重）",
	Intent: "当你想一次看清『所有和我有关的待办』——不管是我创建(creator)的、指派给我执行(executor)的、还是我作为参与人(participant)协作的——时使用；" +
		"内部默认拉取你当前组织下 roleTypes=[\"creator\",\"executor\",\"participant\"] 三种角色的待办并集（这三个值正是待办列表支持的角色枚举），" +
		"再在本地按任务 ID(taskId) 去重（同一条待办可能因多角色重复出现），把每条投影成标题、状态、优先级、创建人、计划完成时间和 taskId 打印出来。" +
		"可用 --role-types 以逗号分隔覆盖默认角色（取值 creator/executor/participant），可用 --status 透传 todoStatus 过滤状态。" +
		"这是纯只读操作，只做列表、去重与投影，不会创建或修改任何待办；若没有与你相关的待办则返回空列表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{
			Name: "role-types",
			Type: shortcut.FlagString,
			Desc: "覆盖默认角色范围，逗号分隔，取值 creator/executor/participant；不传则默认三者并集",
		},
		{
			Name: "status",
			Type: shortcut.FlagString,
			Desc: "按 todoStatus 过滤（透传给 get_user_todos_in_current_org）",
		},
	},
	Tips: []string{
		`dws todo +related-tasks`,
		`dws todo +related-tasks --role-types creator,executor`,
		`dws todo +related-tasks --status TODO`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — build params. roleTypes defaults to the three-role union;
		// --role-types overrides it (validated like helpers.parseRoleTypes).
		roleTypes := []string{"creator", "executor", "participant"}
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
			"roleTypes": roleTypes,
		}
		if rt.Changed("status") {
			if v := strings.TrimSpace(rt.Str("status")); v != "" {
				params["todoStatus"] = v
			}
		}

		// List ALL related cards across pages so a todo beyond the first page is
		// not silently dropped from the union/dedupe.
		cards, err := shortcutListAllTodoCards(rt, params)
		if err != nil {
			return err
		}

		// Step 2 — dedupe by taskId and project.
		seen := make(map[string]bool, len(cards))
		results := make([]map[string]any, 0, len(cards))
		for _, m := range cards {
			taskID := shortcutRelatedTaskID(m)
			if taskID != "" {
				if seen[taskID] {
					continue
				}
				seen[taskID] = true
			}
			results = append(results, shortcutRelatedProject(m, taskID))
		}

		if len(results) == 0 {
			return apperrors.NewValidation("没有与你相关的待办（creator/executor/participant 三种角色下均为空）")
		}

		// Step 3 — print the deduped, projected list.
		return rt.Output(map[string]any{"tasks": results, "count": len(results)})
	},
}

// parseRelatedRoleTypes parses a comma-separated role-types string and validates
// each value, mirroring helpers.parseRoleTypes. Only creator / executor /
// participant are allowed.
func parseRelatedRoleTypes(s string) ([]string, error) {
	allowed := map[string]bool{"creator": true, "executor": true, "participant": true}
	out := make([]string, 0, 3)
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if !allowed[p] {
			return nil, apperrors.NewValidation(fmt.Sprintf("非法的 role-type %q，合法取值：creator, executor, participant", p))
		}
		out = append(out, p)
	}
	return out, nil
}

// shortcutRelatedTaskID reads a card's task id, probing common aliases and
// tolerating both string and numeric JSON encodings.
func shortcutRelatedTaskID(m map[string]any) string {
	for _, key := range []string{"taskId", "todoId", "id"} {
		switch v := m[key].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			if v != 0 {
				return fmt.Sprintf("%.0f", v)
			}
		case int64:
			if v != 0 {
				return fmt.Sprintf("%d", v)
			}
		}
	}
	return ""
}

// shortcutRelatedProject projects a raw todo card to a clean output shape,
// probing multiple candidate keys per field defensively.
func shortcutRelatedProject(m map[string]any, taskID string) map[string]any {
	item := map[string]any{"taskId": taskID}
	if v := shortcutRelatedStr(m, "subject", "title", "name"); v != "" {
		item["title"] = v
	}
	if v := shortcutRelatedStr(m, "todoStatus", "status", "finalStatusStage", "statusStage"); v != "" {
		item["status"] = v
	}
	if v, ok := m["priority"]; ok {
		item["priority"] = v
	}
	if v := shortcutRelatedStr(m, "creatorName", "creator", "creatorId"); v != "" {
		item["creator"] = v
	}
	if due, ok := shortcutOverdueDueTime(m); ok {
		item["planFinishDate"] = due
	}
	return item
}

// shortcutRelatedStr returns the first non-empty string value among the given
// candidate keys.
func shortcutRelatedStr(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	}
	return ""
}

func init() {
	shortcut.Register(RelatedTasks)
}
