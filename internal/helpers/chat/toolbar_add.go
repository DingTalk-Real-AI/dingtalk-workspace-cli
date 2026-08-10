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
)

func newToolbarAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "将入口添加到快捷栏可见区",
		Long:  "将一个或多个快捷入口从隐藏区移至可见区。入口 ID 使用逗号分隔。",
		Example: `  dws chat toolbar add --conversation-id <cid> --shortcut-ids 101,102
  # 查询入口 ID: dws chat toolbar list --conversation-id <cid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cid, err := toolbarConversationID(cmd)
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "shortcut-ids"); err != nil {
				return err
			}
			ids, err := parseCSVInt64(mustGetFlag(cmd, "shortcut-ids"))
			if err != nil {
				return fmt.Errorf("--shortcut-ids: %w", err)
			}
			err = callMCPToolOnServer("im", "add_shortcut_to_bar", map[string]any{
				"openConversationId": cid,
				"shortcutIds":        ids,
			})
			if isSystemBusy(err) {
				return toolbarNewSystemBusyError()
			}
			return err
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 openConversationId")
	cmd.Flags().String("shortcut-ids", "", "入口 ID 列表（逗号分隔）")
	_ = cmd.MarkFlagRequired("conversation-id")
	_ = cmd.MarkFlagRequired("shortcut-ids")
	cmd.DisableAutoGenTag = true

	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "chat",
				Name:           "add_shortcut_to_bar",
				CanonicalPath:  "chat.add_shortcut_to_bar",
				CLIPath:        "chat toolbar add",
				PrimaryCLIPath: "chat toolbar add",
			},
			Description: "将快捷入口添加到会话快捷栏可见区",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "im", RPCName: "add_shortcut_to_bar"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将快捷入口添加到会话快捷栏可见区",
				UseWhen:      []string{"需要将隐藏的快捷入口重新显示在快捷栏中"},
				AvoidWhen:    []string{"创建新的自定义入口应使用 chat toolbar create-custom"},
				Examples:     []string{"dws chat toolbar add --conversation-id <cid> --shortcut-ids 101,102"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(true)},
				{Name: "shortcut-ids", Property: "shortcutIds", Required: boolPtr(true)},
			},
		},
	})

	return cmd
}
