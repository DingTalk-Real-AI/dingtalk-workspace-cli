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
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	todoshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/todo"
)

// Remind: create a personal todo for YOURSELF with an optional due time,
// in one command. It resolves the current user and explicitly passes executorIds;
// the real todo backend does not reliably default a missing executor to "me".
//
// Steps: take --task as the subject, and (optionally) parse --at into epoch
// milliseconds (mirroring the todo helper's parseISOTimeToMillis, which stores
// dueTime as int64 millis) → create_personal_todo. Replaces having to look up
// your own userId before `todo +create`.
//
//	dws todo +remind --task "交周报" --at 2026-03-10T18:00:00+08:00
var Remind = shortcut.Shortcut{
	OutputRollout: output.RolloutUnifiedActive,
	Service:       "todo",
	Command:       "+remind",
	Product:       "todo",
	Description:   "给自己创建一条带可选截止时间的待办",
	Intent: "当你想给自己记一件事、并（可选）设一个截止时间，又不想先查自己的 userId 时使用；" +
		"内部先解析当前登录用户的 userId，再显式设置 executorIds，--at 只会按 ISO8601 写入截止时间 dueTime，不会创建独立提醒规则。会真实创建待办。",
	Risk: shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "todo",
			Name:           "shortcut_remind",
			CanonicalPath:  "todo.shortcut_remind",
			CLIPath:        "todo +remind",
			PrimaryCLIPath: "todo +remind",
		},
		Description: "给自己创建一条带可选截止时间的待办",
		DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false},
		Result:      &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess}, DataSchema: json.RawMessage(`{"type":"object","description":"已验证的自用待办","properties":{"taskId":{"type":"string","description":"新待办稳定 taskId"},"subject":{"type":"string","description":"待办标题"},"verified":{"type":"boolean","description":"是否完成详情读回核验"}},"required":["taskId","subject","verified"],"additionalProperties":false}`)},
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "给自己创建一条带可选截止时间的待办",
			UseWhen:      []string{"当你想给自己记一件事、并（可选）设一个截止时间，又不想先查自己的 userId 时使用；内部先解析当前登录用户的 userId，再显式设置 executorIds，--at 只会按 ISO8601 写入截止时间 dueTime，不会创建独立提醒规则。会真实创建待办。"},
			AvoidWhen: []string{
				"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令",
				"用户需要独立提醒规则时改用 dws todo task add-reminder；不要把 --at 解释成提醒时间",
			},
			Examples: []string{"dws todo +remind --task \"交周报\" --at 2026-03-10T18:00:00+08:00"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "task", Type: shortcut.FlagString, Desc: "待办标题/内容", Required: true},
		{Name: "at", Type: shortcut.FlagString, Desc: "截止时间（ISO8601，可选，写入 dueTime，不是提醒时间）"},
	},
	Tips: []string{`dws todo +remind --task "交周报" --at 2026-03-10T18:00:00+08:00`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		task := strings.TrimSpace(rt.Str("task"))
		var dueMillis int64
		atProvided := rt.Changed("at")
		if atProvided {
			var err error
			dueMillis, err = shortcutRemindParseMillis("at", rt.Str("at"))
			if err != nil {
				return err
			}
		}
		if rt.DryRun() {
			preview := map[string]any{"dryRun": true, "executed": false, "preview_kind": "plan", "subject": task, "executor": "current_user"}
			if atProvided {
				preview["dueTime"] = dueMillis
			}
			return rt.Output(preview)
		}

		profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return err
		}
		userID := myAttendanceCurrentUserID(profile)
		if userID == "" {
			return apperrors.NewValidation("无法解析当前登录用户的 userId，无法给自己创建待办")
		}

		vo := map[string]any{
			"subject":     task,
			"executorIds": []string{userID},
		}

		// Optional due time. The todo helper feeds --due through
		// parseISOTimeToMillis and stores dueTime as epoch milliseconds (int64),
		// so we do the same here rather than passing a raw string.
		if atProvided {
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
		return rt.Output(map[string]any{"taskId": taskID, "subject": task, "verified": true})
	},
}

// shortcutRemindParseMillis parses an ISO8601 timestamp into epoch milliseconds,
// returning a clear validation error naming the offending flag. Mirrors the todo
// helper's parseISOTimeToMillis (which the CLI uses to build dueTime).
func shortcutRemindParseMillis(flag, value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf(
			"--%s 时间格式无效：%q，请使用 ISO8601（如 2026-03-10T18:00:00+08:00）", flag, value))
	}
	return t.UnixMilli(), nil
}

func init() {
	shortcut.Register(Remind)
}
