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
	"encoding/json"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	todoshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/todo"
)

// Assign: create a todo AND assign it to a person by NAME, in one command.
//
// Steps: resolve the assignee name → userId → create a personal todo with that
// user as executor. Replaces `contact +search-user` (copy userId) →
// `todo +create --executors <id>`.
//
//	dws todo +assign --to 张三 --task "整理周报"
var Assign = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+assign",
	Product:       "todo",
	Description:   "按姓名给某人创建并指派一条待办（自动解析 userId）",
	Intent: "当你想把一件事指派给某位同事、但只知道对方姓名不想先查 userId 时使用；" +
		"内部先按姓名解析出唯一 userId，再创建待办并把 TA 设为执行人。会真实创建待办。",
	Risk: shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_assign",
			CanonicalPath:  "todo.shortcut_assign",
			CLIPath:        "todo +assign",
			PrimaryCLIPath: "todo +assign",
		},
		Description: "按姓名给某人创建并指派一条待办（自动解析 userId）",
		DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false},
		Result:      &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(`{"type":"object","description":"已验证的单人指派待办","properties":{"taskId":{"type":"string","description":"新待办稳定 taskId"},"subject":{"type":"string","description":"待办标题"},"executorId":{"type":"string","description":"解析出的执行人 userId"},"verified":{"type":"boolean","description":"是否完成详情读回核验"}},"required":["taskId","subject","executorId","verified"],"additionalProperties":false}`)},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按姓名给某人创建并指派一条待办（自动解析 userId）",
			UseWhen:      []string{"当你想把一件事指派给某位同事、但只知道对方姓名不想先查 userId 时使用；内部先按姓名解析出唯一 userId，再创建待办并把 TA 设为执行人。会真实创建待办。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws todo +assign --to 张三 --task \"整理周报\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagString, Desc: "执行人姓名/花名", Required: true},
		{Name: "task", Type: shortcut.FlagString, Desc: "待办标题/内容", Required: true},
		{Name: "due", Type: shortcut.FlagString, Desc: "截止时间（ISO8601，可选）"},
	},
	Tips: []string{`dws todo +assign --to 张三 --task "整理周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		name := strings.TrimSpace(rt.Str("to"))
		task := strings.TrimSpace(rt.Str("task"))
		var dueMillis int64
		if rt.Changed("due") {
			var err error
			dueMillis, err = shortcutRemindParseMillis("due", rt.Str("due"))
			if err != nil {
				return err
			}
		}
		if rt.DryRun() {
			preview := map[string]any{"dryRun": true, "executed": false, "preview_kind": "plan", "subject": task, "assigneeQuery": name}
			if dueMillis != 0 {
				preview["dueTime"] = dueMillis
			}
			return rt.Output(preview)
		}

		// Step 1 — resolve the assignee name to a unique userId.
		user, err := resolveUser(rt, name)
		if err != nil {
			return err
		}

		// Step 2 — create the todo with that user as executor. create_personal_todo
		// accepts executorIds directly, so assignment is part of creation.
		vo := map[string]any{
			"subject":     task,
			"executorIds": []string{user.userID},
		}
		if dueMillis != 0 {
			vo["dueTime"] = dueMillis
		}
		params := map[string]any{
			"PersonalTodoCreateVO": vo,
		}
		data, err := rt.CallMCPWriteDataStrict("todo", "create_personal_todo", params)
		if err != nil {
			return err
		}
		taskID, _, err := todoshortcut.VerifyCreatedTodo(rt, data, "todo/create_personal_todo", task)
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{"taskId": taskID, "subject": task, "executorId": user.userID, "verified": true})
	},
}

func init() {
	shortcut.Register(Assign)
}
