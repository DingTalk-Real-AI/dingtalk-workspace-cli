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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func newToolbarSortCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sort",
		Short: "排序快捷栏入口",
		Long:  "对会话快捷栏入口进行排序。sorted-ids 指定排序后的可见区入口 ID 列表，unsorted-ids 指定不参与排序放在末尾的入口 ID 列表。两个列表不能有交集。",
		Example: `  dws chat toolbar sort --conversation-id <cid> --sorted-ids 101,102,103
  dws chat toolbar sort --conversation-id <cid> --sorted-ids 101,102 --unsorted-ids 103,104
  # 查询入口 ID: dws chat toolbar list --conversation-id <cid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cid, err := toolbarConversationID(cmd)
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "sorted-ids"); err != nil {
				return err
			}
			sortedIds, err := parseCSVInt64(mustGetFlag(cmd, "sorted-ids"))
			if err != nil {
				return fmt.Errorf("--sorted-ids: %w", err)
			}

			toolArgs := map[string]any{
				"openConversationId": cid,
				"sortedShortcutIds":  sortedIds,
			}

			if unsortedRaw, _ := cmd.Flags().GetString("unsorted-ids"); unsortedRaw != "" {
				unsortedIds, err := parseCSVInt64(unsortedRaw)
				if err != nil {
					return fmt.Errorf("--unsorted-ids: %w", err)
				}
				if hasIntersection(sortedIds, unsortedIds) {
					return apperrors.NewValidation(
						"--sorted-ids 与 --unsorted-ids 不能有交集",
						apperrors.WithReason("id_intersection"),
						apperrors.WithHint("检查两个 ID 列表，确保同一个 ID 不同时出现在两个列表中"),
						apperrors.WithActions("修正 ID 列表后重试"),
					)
				}
				toolArgs["unsortedShortcutIds"] = unsortedIds
			}

			err = callMCPToolOnServer("im", "sort_shortcut_bar", toolArgs)
			if isSystemBusy(err) {
				return toolbarNewSystemBusyError()
			}
			return err
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 openConversationId")
	cmd.Flags().String("sorted-ids", "", "排序后的入口 ID 列表（逗号分隔）")
	cmd.Flags().String("unsorted-ids", "", "不参与排序放在末尾的入口 ID 列表（逗号分隔）")
	_ = cmd.MarkFlagRequired("conversation-id")
	_ = cmd.MarkFlagRequired("sorted-ids")
	cmd.DisableAutoGenTag = true

	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "chat",
				Name:           "sort_shortcut_bar",
				CanonicalPath:  "chat.sort_shortcut_bar",
				CLIPath:        "chat toolbar sort",
				PrimaryCLIPath: "chat toolbar sort",
			},
			Description: "对会话快捷栏入口进行排序",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "im", RPCName: "sort_shortcut_bar"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "对会话快捷栏入口进行排序",
				UseWhen:      []string{"需要调整快捷栏入口的显示顺序"},
				AvoidWhen:    []string{"仅添加或隐藏入口使用 chat toolbar add / hide"},
				Examples:     []string{"dws chat toolbar sort --conversation-id <cid> --sorted-ids 101,102,103"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(true)},
				{Name: "sorted-ids", Property: "sortedShortcutIds", Required: boolPtr(true)},
				{Name: "unsorted-ids", Property: "unsortedShortcutIds", Required: boolPtr(false)},
			},
		},
	})

	return cmd
}
