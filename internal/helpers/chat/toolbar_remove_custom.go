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

package chat

import (
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// removeChatToolbarCustomShortcutFn is the package-level injection seam for
// the remove-custom command's remote call. The default implementation routes
// through callMCPToolOnServer against the im server, so production behavior is
// unchanged. Tests swap it with testseam.Swap(t, &removeChatToolbarCustomShortcutFn, stub)
// to capture the call without hitting a real remote. Like every package-var
// seam in this repo it is not safe for t.Parallel tests.
var removeChatToolbarCustomShortcutFn = func(openConversationId string, shortcutId int64) error {
	return callMCPToolOnServer("im", "remove_custom_shortcut", map[string]any{
		"openConversationId": openConversationId,
		"shortcutId":         shortcutId,
	})
}

func newToolbarRemoveCustomCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove-custom",
		Short:   "删除自定义快捷栏入口",
		Long:    "删除一个自定义快捷栏入口。该操作不可逆；必须先获得用户确认，再追加 --yes 执行。",
		Example: `  dws chat toolbar remove-custom --conversation-id <cid> --shortcut-id 123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cid, err := toolbarConversationID(cmd)
			if err != nil {
				return err
			}
			shortcutId, _ := cmd.Flags().GetInt64("shortcut-id")
			if !commandBoolFlag(cmd, "yes") {
				return apperrors.NewValidation(
					"删除自定义快捷入口不可逆；获得用户确认后加 --yes 执行",
					apperrors.WithReason("confirmation_required"),
					apperrors.WithHint("先确认目标入口及影响范围；用户明确同意后以相同参数追加 --yes"),
					apperrors.WithActions("确认目标自定义入口", "获得用户确认后使用 --yes 执行"),
				)
			}
			err = removeChatToolbarCustomShortcutFn(cid, shortcutId)
			if isSystemBusy(err) {
				return toolbarNewSystemBusyError()
			}
			return err
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 openConversationId")
	cmd.Flags().Int64("shortcut-id", 0, "自定义入口 ID")
	cmd.Flags().Bool("yes", false, "确认执行删除操作")
	_ = cmd.MarkFlagRequired("conversation-id")
	_ = cmd.MarkFlagRequired("shortcut-id")
	cmd.DisableAutoGenTag = true

	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "chat",
				Name:           "remove_custom_shortcut",
				CanonicalPath:  "chat.remove_custom_shortcut",
				CLIPath:        "chat toolbar remove-custom",
				PrimaryCLIPath: "chat toolbar remove-custom",
			},
			Description: "删除自定义快捷栏入口",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "im", RPCName: "remove_custom_shortcut"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除自定义快捷栏入口",
				UseWhen:      []string{"需要永久删除某个自定义快捷栏入口"},
				AvoidWhen:    []string{"仅隐藏入口使用 chat toolbar hide；更新入口使用 chat toolbar update-custom"},
				Examples:     []string{"dws chat toolbar remove-custom --conversation-id <cid> --shortcut-id 123"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(true)},
				{Name: "shortcut-id", Property: "shortcutId", Required: boolPtr(true)},
			},
		},
	})

	return cmd
}
