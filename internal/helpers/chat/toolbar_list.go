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
)

func newToolbarListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "查询会话快捷栏入口列表",
		Long:  "查询指定会话的快捷栏入口列表，包括可见区和隐藏区的所有入口。",
		Example: `  dws chat toolbar list --conversation-id <cid>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cid, err := toolbarConversationID(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("im", "list_chat_toolbar_shortcuts", map[string]any{
				"openConversationId": cid,
			})
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 openConversationId")
	_ = cmd.MarkFlagRequired("conversation-id")
	cmd.DisableAutoGenTag = true

	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "chat",
				Name:           "list_chat_toolbar_shortcuts",
				CanonicalPath:  "chat.list_chat_toolbar_shortcuts",
				CLIPath:        "chat toolbar list",
				PrimaryCLIPath: "chat toolbar list",
			},
			Description: "查询会话快捷栏入口列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "im", RPCName: "list_chat_toolbar_shortcuts"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询会话快捷栏入口列表",
				UseWhen:      []string{"需要查看会话快捷栏中有哪些入口、获取入口 ID 时"},
				AvoidWhen:    []string{"修改快捷栏排序使用 chat toolbar sort；创建自定义入口使用 chat toolbar create-custom"},
				Examples:     []string{"dws chat toolbar list --conversation-id <cid>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(true)},
			},
		},
	})

	return cmd
}
