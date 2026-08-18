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

// Package todo declares high-fidelity shortcuts for the DingTalk "todo"
// (personal todo) MCP service. Tool names and parameter keys are copied verbatim
// from internal/helpers/todo.go; do not invent tools or params here.
package todo

import (
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Create maps helper `create_personal_todo`.
// CreateSub maps helper `create_personal_sub_todo`.
// GetMyTasks maps helper `get_user_todos_in_current_org`.
var GetMyTasks = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+get-my-tasks",
	Product:       "todo",
	Description:   "查询当前组织下我的待办列表",
	Intent:        "当你想查看自己在当前组织下的待办清单、盘点未完成事项或按条件筛选任务时使用；可按完成状态、优先级、角色（创建者/执行者/参与者）和截止时间范围过滤并分页，返回匹配的待办列表。",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_get_my_tasks",
			CanonicalPath:  "todo.shortcut_get_my_tasks",
			CLIPath:        "todo +get-my-tasks",
			PrimaryCLIPath: "todo +get-my-tasks",
		},
		Description: "查询当前组织下我的待办列表",
		Result:      todoPagedCollectionResult("todos", "当前组织下我的待办列表"),
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询当前组织下我的待办列表",
			UseWhen:      []string{"当你想查看自己在当前组织下的待办清单、盘点未完成事项或按条件筛选任务时使用；可按完成状态、优先级、角色（创建者/执行者/参与者）和截止时间范围过滤并分页，返回匹配的待办列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws todo +get-my-tasks --status false --priority 40,30"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "page", Type: shortcut.FlagString, Default: "1", Desc: "页码"},
		{Name: "size", Type: shortcut.FlagString, Default: "20", Desc: "每页数量"},
		{Name: "status", Type: shortcut.FlagString, Enum: []string{"true", "false"}, Desc: "true=已完成, false=未完成"},
		{Name: "priority", Type: shortcut.FlagStringSlice, Desc: "优先级过滤: 10/20/30/40"},
		{Name: "role-types", Type: shortcut.FlagStringSlice, Default: "executor", Desc: "角色类型: creator/executor/participant"},
		{Name: "plan-finish-start", Type: shortcut.FlagInt, Desc: "截止时间范围开始（Unix 毫秒时间戳）"},
		{Name: "plan-finish-end", Type: shortcut.FlagInt, Desc: "截止时间范围结束（Unix 毫秒时间戳）"},
		{Name: "all", Type: shortcut.FlagBool, Desc: "遍历全部分页；达到安全页数上限仍有下一页时失败"},
		{Name: "max-pages", Type: shortcut.FlagInt, Default: "40", Desc: "--all 的最大页数（1-40）"},
	},
	Tips: []string{`dws todo +get-my-tasks --status false --priority 40,30`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		page, err := strconv.Atoi(rt.Str("page"))
		if err != nil || page < 1 {
			return todoResponseError("todo/+get-my-tasks", "invalid_page", "--page 必须是大于 0 的整数")
		}
		size, err := strconv.Atoi(rt.Str("size"))
		if err != nil || size < 1 || size > 20 {
			return todoResponseError("todo/+get-my-tasks", "invalid_page_size", "--size 必须是 1 到 20 的整数；后端已知不支持更大值")
		}
		params := map[string]any{
			"pageNum":  strconv.Itoa(page),
			"pageSize": strconv.Itoa(size),
		}
		if rt.Changed("status") {
			params["todoStatus"] = rt.Str("status")
		}
		if rt.Changed("priority") {
			var ps []int
			for _, p := range rt.StrSlice("priority") {
				n, err := strconv.Atoi(p)
				if err != nil || (n != 10 && n != 20 && n != 30 && n != 40) {
					return todoResponseError("todo/+get-my-tasks", "invalid_priority", "--priority 仅接受 10/20/30/40")
				}
				ps = append(ps, n)
			}
			if ps != nil {
				params["priorityList"] = ps
			}
		}
		roles := rt.StrSlice("role-types")
		if len(roles) == 0 {
			roles = []string{"executor"}
		}
		params["roleTypes"] = roles
		for _, role := range roles {
			if role != "creator" && role != "executor" && role != "participant" {
				return todoResponseError("todo/+get-my-tasks", "invalid_role_type", "--role-types 仅接受 creator/executor/participant")
			}
		}
		if rt.Changed("plan-finish-start") {
			params["planFinishDateStart"] = rt.Int("plan-finish-start")
		}
		if rt.Changed("plan-finish-end") {
			params["planFinishDateEnd"] = rt.Int("plan-finish-end")
		}
		if rt.Bool("all") {
			maxPages := rt.Int("max-pages")
			if maxPages < 1 || maxPages > todoMaxPages {
				return todoResponseError("todo/+get-my-tasks", "invalid_page_limit", "--max-pages 必须是 1 到 40")
			}
			delete(params, "pageNum")
			delete(params, "pageSize")
			cards, err := listAllTodoCards(rt, params, maxPages)
			if err != nil {
				return err
			}
			return rt.Output(map[string]any{"count": len(cards), "todos": cards, "complete": true})
		}
		data, err := rt.CallMCPReadData("todo", "get_user_todos_in_current_org", params)
		if err != nil {
			return err
		}
		cards, err := getMyTasksProjectStrict(data)
		if err != nil {
			return err
		}
		hasMore, err := todoHasMore(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(cards), "todos": cards, "page": page, "size": size, "hasMore": hasMore})
	},
}

func getMyTasksProjectStrict(data map[string]any) ([]map[string]any, error) {
	raw, err := requireTodoCollection(data, "todo/get_user_todos_in_current_org", "todoCards")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for i, m := range raw {
		if todoStableString(m, "taskId", "todoId", "id") == "" {
			return nil, todoResponseError("todo/get_user_todos_in_current_org", "missing_stable_id", "待办列表第 "+strconv.Itoa(i)+" 项缺少稳定 taskId")
		}
		out = append(out, projectTodoCard(m))
	}
	return out, nil
}

func projectTodoCard(m map[string]any) map[string]any {
	row := map[string]any{"taskId": todoStableString(m, "taskId", "todoId", "id")}
	for _, k := range []string{"subject", "dueTime", "priority", "finalStatusStage", "creatorId"} {
		if v, ok := m[k]; ok {
			row[k] = v
		}
	}
	return row
}

// getMyTasksProject reshapes get_user_todos_in_current_org into a clean todo
// list (subject/taskId/dueTime/priority/done) — clean output projection.
// The card list lives under result.todoCards; fields are probed defensively.
func getMyTasksProject(data map[string]any) []map[string]any {
	container := data
	if r, ok := data["result"].(map[string]any); ok {
		container = r
	}
	raw, ok := container["todoCards"].([]any)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		for _, k := range []string{"subject", "taskId", "dueTime", "priority", "finalStatusStage", "creatorId"} {
			if v, ok := m[k]; ok {
				row[k] = v
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ListSub maps helper `list_sub_tasks`.
var ListSub = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+list-sub",
	Product:       "todo",
	Description:   "查询子待办列表",
	Intent:        "当你已知某个待办任务 ID、想了解它被拆解出的所有子任务时使用；输入父任务 ID，返回其下的子待办列表。",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_list_sub",
			CanonicalPath:  "todo.shortcut_list_sub",
			CLIPath:        "todo +list-sub",
			PrimaryCLIPath: "todo +list-sub",
		},
		Description: "查询子待办列表",
		Result:      todoCollectionResult("subTasks", "指定父待办的子待办列表"),
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询子待办列表",
			UseWhen:      []string{"当你已知某个待办任务 ID、想了解它被拆解出的所有子任务时使用；输入父任务 ID，返回其下的子待办列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws todo +list-sub --task-id <taskId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办任务 ID", Required: true},
	},
	Tips: []string{`dws todo +list-sub --task-id <taskId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("todo", "list_sub_tasks", map[string]any{
			"todoSubTaskListRequest": map[string]any{
				"taskId": rt.Str("task-id"),
			},
		})
		if err != nil {
			return err
		}
		subs, err := listSubProjectStrict(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(subs), "subTasks": subs})
	},
}

func listSubProjectStrict(data map[string]any) ([]map[string]any, error) {
	raw, err := requireTodoCollection(data, "todo/list_sub_tasks", "list", "items", "subTasks", "subTaskList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for i, m := range raw {
		id := todoStableString(m, "taskId", "id", "subTaskId")
		if id == "" {
			return nil, todoResponseError("todo/list_sub_tasks", "missing_stable_id", "子待办列表第 "+strconv.Itoa(i)+" 项缺少稳定 taskId")
		}
		row := map[string]any{"taskId": id}
		if v := listSubFirst(m, "subject", "title", "name", "content"); v != nil {
			row["subject"] = v
		}
		if v := listSubFirst(m, "dueTime", "dueDate", "deadline"); v != nil {
			row["dueTime"] = v
		}
		out = append(out, row)
	}
	return out, nil
}

// listSubProject reshapes list_sub_tasks into a clean sub-todo list
// (subject/taskId/dueTime) — clean output projection. The list
// container and field names are probed defensively across candidate keys.
func listSubProject(data map[string]any) []map[string]any {
	raw := listSubExtractList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := listSubFirst(m, "subject", "title", "name", "content"); v != nil {
			row["subject"] = v
		}
		if v := listSubFirst(m, "taskId", "id", "subTaskId"); v != nil {
			row["taskId"] = v
		}
		if v := listSubFirst(m, "dueTime", "dueDate", "deadline"); v != nil {
			row["dueTime"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listSubExtractList unwraps common container shapes (a bare slice, or a
// slice nested under result/data/list/items, optionally one level deeper) and
// returns the sub-task slice, or nil when none is found.
func listSubExtractList(data map[string]any) []any {
	containers := []map[string]any{data}
	for _, k := range []string{"result", "data"} {
		if m, ok := data[k].(map[string]any); ok {
			containers = append(containers, m)
		}
	}
	for _, c := range containers {
		for _, k := range []string{"list", "items", "subTasks", "subTaskList", "result", "data"} {
			if arr, ok := c[k].([]any); ok {
				return arr
			}
		}
	}
	// data itself may be a bare list under result/data.
	for _, k := range []string{"result", "data"} {
		if arr, ok := data[k].([]any); ok {
			return arr
		}
	}
	return nil
}

// listSubFirst returns the first present value among the candidate keys.
func listSubFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// Update maps helper `update_todo_task`.
// Complete maps helper `update_todo_done_status`.
// Get maps helper `get_todo_detail`.
var Get = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+get",
	Product:       "todo",
	Description:   "查询待办详情",
	Intent:        "当你已知某条待办的任务 ID、想查看它的完整信息（标题、执行者、截止时间、优先级、状态等）时使用；输入任务 ID，返回该待办的详细内容。",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_get",
			CanonicalPath:  "todo.shortcut_get",
			CLIPath:        "todo +get",
			PrimaryCLIPath: "todo +get",
		},
		Description: "查询待办详情",
		Result:      todoObjectResult("待办详情"),
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询待办详情",
			UseWhen:      []string{"当你已知某条待办的任务 ID、想查看它的完整信息（标题、执行者、截止时间、优先级、状态等）时使用；输入任务 ID，返回该待办的详细内容。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws todo +get --task-id <taskId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办任务 ID", Required: true},
	},
	Tips: []string{`dws todo +get --task-id <taskId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		detail, err := readTodoDetail(rt, rt.Str("task-id"))
		if err != nil {
			return err
		}
		return rt.Output(detail)
	},
}

// Delete maps helper `delete_todo`.
// AddExecutor maps helper `add_task_executors`.
// RemoveExecutor maps helper `remove_task_executors`.
// AddParticipant maps helper `add_task_participants`.
// RemoveParticipant maps helper `remove_task_participants`.
// AddReminder maps helper `add_todo_reminder`.
// ListAttachment maps helper `list_todo_attachment`.
var ListAttachment = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+list-attachment",
	Product:       "todo",
	Description:   "查询待办任务的附件列表",
	Intent:        "当你想查看某条待办上挂了哪些附件、或需要拿到附件 ID 以便后续删除时使用；输入任务 ID，返回该待办的附件列表。",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_list_attachment",
			CanonicalPath:  "todo.shortcut_list_attachment",
			CLIPath:        "todo +list-attachment",
			PrimaryCLIPath: "todo +list-attachment",
		},
		Description: "查询待办任务的附件列表",
		Result:      todoCollectionResult("attachments", "待办附件列表"),
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询待办任务的附件列表",
			UseWhen:      []string{"当你想查看某条待办上挂了哪些附件、或需要拿到附件 ID 以便后续删除时使用；输入任务 ID，返回该待办的附件列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws todo +list-attachment --task-id <taskId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办任务 ID", Required: true},
	},
	Tips: []string{`dws todo +list-attachment --task-id <taskId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("todo", "list_todo_attachment", map[string]any{
			"todoAttachmentListRequest": map[string]any{
				"taskId": rt.Str("task-id"),
			},
		})
		if err != nil {
			return err
		}
		atts, err := listAttachmentProjectStrict(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(atts), "attachments": atts})
	},
}

func listAttachmentProjectStrict(data map[string]any) ([]map[string]any, error) {
	raw, err := requireTodoCollection(data, "todo/list_todo_attachment", "list", "items", "attachments", "attachmentList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for i, m := range raw {
		id := todoStableString(m, "attachmentId", "id", "fileId", "dentryId")
		if id == "" {
			return nil, todoResponseError("todo/list_todo_attachment", "missing_stable_id", "附件列表第 "+strconv.Itoa(i)+" 项缺少稳定 ID")
		}
		row := map[string]any{"attachmentId": id}
		for _, pair := range [][2]string{{"fileName", "name"}, {"fileSize", "size"}, {"fileType", "type"}} {
			if v := listAttachmentFirst(m, pair[0], pair[1]); v != nil {
				row[pair[0]] = v
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// listAttachmentProject reshapes list_todo_attachment into a clean attachment
// list (attachmentId/fileName/fileSize/fileType) — output-projection fidelity
// for clean output. The list container and field names are probed defensively across
// candidate keys so response-shape drift yields an empty list, not a crash.
func listAttachmentProject(data map[string]any) []map[string]any {
	raw := listAttachmentExtractList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := listAttachmentFirst(m, "attachmentId", "id", "fileId"); v != nil {
			row["attachmentId"] = v
		}
		if v := listAttachmentFirst(m, "fileName", "name", "spaceFileName"); v != nil {
			row["fileName"] = v
		}
		if v := listAttachmentFirst(m, "fileSize", "size"); v != nil {
			row["fileSize"] = v
		}
		if v := listAttachmentFirst(m, "fileType", "type"); v != nil {
			row["fileType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listAttachmentExtractList unwraps a bare slice or one nested under common
// envelope keys (optionally one level deeper), returning nil when none found.
func listAttachmentExtractList(data map[string]any) []any {
	containers := []map[string]any{data}
	for _, k := range []string{"result", "data"} {
		if m, ok := data[k].(map[string]any); ok {
			containers = append(containers, m)
		}
	}
	for _, c := range containers {
		for _, k := range []string{"list", "items", "attachments", "attachmentList", "result", "data"} {
			if arr, ok := c[k].([]any); ok {
				return arr
			}
		}
	}
	for _, k := range []string{"result", "data"} {
		if arr, ok := data[k].([]any); ok {
			return arr
		}
	}
	return nil
}

// listAttachmentFirst returns the first present value among the candidate keys.
func listAttachmentFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// RemoveAttachment maps helper `remove_todo_attachment`.
// AddComment maps helper `add_todo_comment`.
// ListComment maps helper `list_todo_comment`.
var ListComment = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+list-comment",
	Product:       "todo",
	Description:   "查询待办评论列表",
	Intent:        "当你想查看某条待办下的讨论记录、了解协作沟通历史或获取评论 ID 以便删除时使用；输入任务 ID 并可分页，返回该待办的评论列表。",
	Risk:          shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_list_comment",
			CanonicalPath:  "todo.shortcut_list_comment",
			CLIPath:        "todo +list-comment",
			PrimaryCLIPath: "todo +list-comment",
		},
		Description: "查询待办评论列表",
		Result:      todoCollectionResult("comments", "待办评论列表"),
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询待办评论列表",
			UseWhen:      []string{"当你想查看某条待办下的讨论记录、了解协作沟通历史或获取评论 ID 以便删除时使用；输入任务 ID 并可分页，返回该待办的评论列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws todo +list-comment --task-id <taskId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "task-id", Type: shortcut.FlagString, Desc: "待办任务 ID", Required: true},
		{Name: "page", Type: shortcut.FlagString, Default: "1", Desc: "页码"},
		{Name: "size", Type: shortcut.FlagString, Default: "20", Desc: "每页数量"},
	},
	Tips: []string{`dws todo +list-comment --task-id <taskId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		page, err := strconv.Atoi(rt.Str("page"))
		if err != nil || page < 1 {
			return todoResponseError("todo/+list-comment", "invalid_page", "--page 必须是大于 0 的整数")
		}
		size, err := strconv.Atoi(rt.Str("size"))
		if err != nil || size < 1 || size > todoPageSize {
			return todoResponseError("todo/+list-comment", "invalid_page_size", "--size 必须是 1 到 20 的整数")
		}
		data, err := rt.CallMCPData("todo", "list_todo_comment", map[string]any{
			"taskId":   rt.Str("task-id"),
			"page":     strconv.Itoa(page),
			"pageSize": strconv.Itoa(size),
		})
		if err != nil {
			return err
		}
		comments, err := listCommentProjectStrict(data)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"count": len(comments), "comments": comments})
	},
}

func listCommentProjectStrict(data map[string]any) ([]map[string]any, error) {
	raw, err := requireTodoCollection(data, "todo/list_todo_comment", "list", "items", "comments", "commentList")
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for i, m := range raw {
		id := todoStableString(m, "commentId", "id")
		if id == "" {
			return nil, todoResponseError("todo/list_todo_comment", "missing_stable_id", "评论列表第 "+strconv.Itoa(i)+" 项缺少稳定 commentId")
		}
		row := map[string]any{"commentId": id}
		for _, pair := range [][2]string{{"content", "text"}, {"creatorId", "userId"}, {"createTime", "createdTime"}} {
			if v := listCommentFirst(m, pair[0], pair[1]); v != nil {
				row[pair[0]] = v
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// listCommentProject reshapes list_todo_comment into a clean comment list
// (commentId/content/creatorId/createTime) — clean output projection.
// The list container and field names are probed defensively across
// candidate keys so response-shape drift yields an empty list, not a crash.
func listCommentProject(data map[string]any) []map[string]any {
	raw := listCommentExtractList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v := listCommentFirst(m, "commentId", "id"); v != nil {
			row["commentId"] = v
		}
		if v := listCommentFirst(m, "content", "text", "comment"); v != nil {
			row["content"] = v
		}
		if v := listCommentFirst(m, "creatorId", "creator", "userId"); v != nil {
			row["creatorId"] = v
		}
		if v := listCommentFirst(m, "createTime", "createdTime", "gmtCreate"); v != nil {
			row["createTime"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listCommentExtractList unwraps a bare slice or one nested under common
// envelope keys (optionally one level deeper), returning nil when none found.
func listCommentExtractList(data map[string]any) []any {
	containers := []map[string]any{data}
	for _, k := range []string{"result", "data"} {
		if m, ok := data[k].(map[string]any); ok {
			containers = append(containers, m)
		}
	}
	for _, c := range containers {
		for _, k := range []string{"list", "items", "comments", "commentList", "result", "data"} {
			if arr, ok := c[k].([]any); ok {
				return arr
			}
		}
	}
	for _, k := range []string{"result", "data"} {
		if arr, ok := data[k].([]any); ok {
			return arr
		}
	}
	return nil
}

// listCommentFirst returns the first present value among the candidate keys.
func listCommentFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// DeleteComment maps helper `delete_todo_comment`.
func init() {
	shortcut.Register(
		GetMyTasks,
		ListSub,
		Get,
		ListAttachment,
		ListComment,
	)
}
