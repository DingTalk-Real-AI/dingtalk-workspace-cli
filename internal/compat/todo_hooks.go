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

// todo_hooks.go — CLI-side validators for the `todo` product. The envelope
// describes PersonalTodoCreateVO.parentId as a plain string flag (`--parent-id`)
// and the upstream MCP tool create_personal_sub_todo silently accepts any
// non-empty value: when a non-numeric string slips through, the server treats
// the missing numeric parent as "no parent" and creates an *orphan* root-level
// todo instead of failing. The auto-test
// todo/test_03_todo_create_sub.py::test_create_sub_todo_invalid_parent_id
// expects the CLI to reject the invalid value before it ever reaches MCP.
//
// The wukong reference implementation already does the same check inside its
// hand-written cobra RunE (see dws-wukong/wukong/products/todo.go ~line 90:
// strconv.ParseInt + CLIError with "父待办 ID 必须是纯数字, 当前值: ..."). The
// open-source CLI is envelope-driven, so we attach the equivalent guard as a
// PreRunE hook here. Empty parent-id is intentionally NOT validated here —
// envelope already marks it required, so cobra's MarkFlagRequired handles the
// missing case with the standard "required flag(s) ... not set" message.

package compat

import (
	"fmt"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// todoToolsWithNumericParentId lists every todo toolName whose --parent-id
// must be coerced to a pure-numeric long. Today only create_personal_sub_todo
// needs this; if future tools (e.g. add_sub_todo) join, append here.
var todoToolsWithNumericParentId = map[string]bool{
	"create_personal_sub_todo": true,
}

// installTodoHook wires todo-specific PreRunE validators onto leaf commands
// emitted by BuildDynamicCommands. It is a no-op for non-todo products and
// for todo tools that do not need extra client-side checks.
//
// The hook chain preserves the cmd.PreRunE that NewDirectCommand already
// installed (currently validateRequireTogether) by invoking it first.
func installTodoHook(cmd *cobra.Command, canonicalProduct, toolName string) {
	if cmd == nil {
		return
	}
	if strings.TrimSpace(canonicalProduct) != "todo" {
		return
	}
	if !todoToolsWithNumericParentId[toolName] {
		return
	}

	original := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if original != nil {
			if err := original(c, args); err != nil {
				return err
			}
		}
		return validateTodoParentIdNumeric(c)
	}
}

// validateTodoParentIdNumeric inspects --parent-id; if non-empty it must
// parse as int64. Empty values are passed through so cobra's MarkFlagRequired
// (driven by the envelope's `"required": true`) still owns the missing-flag
// error message, matching the existing UX for other required flags.
//
// Error wording mirrors wukong (dws-wukong/wukong/products/todo.go ~L97) so
// agents and humans see a stable message across both editions. The
// apperrors.NewValidation wrapper guarantees stderr renders as
// "Error: [VALIDATION] ..." (PrintHumanAt) or `{"error":{...}}` (PrintJSON),
// both of which satisfy the auto-test substring assertion
// `"error" in result.stderr.lower()`.
func validateTodoParentIdNumeric(cmd *cobra.Command) error {
	if cmd == nil {
		return nil
	}
	flag := cmd.Flags().Lookup("parent-id")
	if flag == nil {
		return nil
	}
	raw, err := cmd.Flags().GetString("parent-id")
	if err != nil {
		// Flag exists but type is not string — defensive no-op, do not block.
		return nil
	}
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	if _, parseErr := strconv.ParseInt(v, 10, 64); parseErr != nil {
		return apperrors.NewValidation(
			fmt.Sprintf("父待办 ID 必须是纯数字, 当前值: %s", v),
			apperrors.WithReason("invalid_parent_id"),
			apperrors.WithHint("请通过 'dws todo task list' 获取正确的父待办任务 ID。"),
			apperrors.WithOperation("todo.task.create-sub.parent-id"),
		)
	}
	return nil
}
