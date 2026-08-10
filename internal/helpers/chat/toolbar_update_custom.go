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

func newToolbarUpdateCustomCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-custom",
		Short: "更新自定义快捷栏入口",
		Long: `更新一个已有的自定义快捷栏入口。

必填参数：
  --conversation-id  会话 openConversationId
  --shortcut-id      自定义入口 ID
  --title            入口标题
  --url              入口跳转链接
  --icon-url         入口图标 URL
  --pc-url           PC 端跳转链接

可选参数：
  --extension        扩展信息，格式 key=value，可重复使用
  --desc             入口描述
  --tag              入口标签
  --sort-index       排序权重`,
		Example: `  dws chat toolbar update-custom --conversation-id <cid> --shortcut-id 123 --title "周报" --url "https://example.com" --icon-url "https://example.com/icon.png" --pc-url "https://example.com"
  # 查询入口 ID: dws chat toolbar list --conversation-id <cid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cid, err := toolbarConversationID(cmd)
			if err != nil {
				return err
			}
			shortcutId, _ := cmd.Flags().GetInt64("shortcut-id")
			if err := validateRequiredFlags(cmd, "title", "url", "icon-url", "pc-url"); err != nil {
				return err
			}

			toolArgs := map[string]any{
				"openConversationId": cid,
				"shortcutId":         shortcutId,
				"name":               toolbarI18nJSONString(mustGetFlag(cmd, "title")),
				"url":                mustGetFlag(cmd, "url"),
				"icons":              toolbarIconsJSONString(mustGetFlag(cmd, "icon-url")),
				"pcUrl":              mustGetFlag(cmd, "pc-url"),
			}

			descValue := mustGetFlag(cmd, "desc")
			if descValue == "" {
				descValue = mustGetFlag(cmd, "title")
			}
			toolArgs["desc"] = toolbarI18nJSONString(descValue)
			if tag, _ := cmd.Flags().GetString("tag"); tag != "" {
				toolArgs["tag"] = tag
			}
			if cmd.Flags().Changed("sort-index") {
				sortIndex, _ := cmd.Flags().GetInt("sort-index")
				toolArgs["sortIndex"] = sortIndex
			}

			ext, err := parseExtension(cmd)
			if err != nil {
				return err
			}
			if ext != nil {
				toolArgs["extension"] = ext
			}

			err = callMCPToolOnServer("im", "update_custom_shortcut", toolArgs)
			if isSystemBusy(err) {
				return toolbarNewSystemBusyError()
			}
			return err
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 openConversationId")
	cmd.Flags().Int64("shortcut-id", 0, "自定义入口 ID")
	cmd.Flags().String("title", "", "入口标题")
	cmd.Flags().String("url", "", "入口跳转链接")
	cmd.Flags().String("icon-url", "", "入口图标 URL")
	cmd.Flags().String("pc-url", "", "PC 端跳转链接")
	cmd.Flags().StringArray("extension", nil, "扩展信息，格式 key=value，可重复使用")
	cmd.Flags().String("desc", "", "入口描述")
	cmd.Flags().String("tag", "", "入口标签")
	cmd.Flags().Int("sort-index", 0, "排序权重")
	_ = cmd.MarkFlagRequired("conversation-id")
	_ = cmd.MarkFlagRequired("shortcut-id")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("icon-url")
	_ = cmd.MarkFlagRequired("pc-url")
	cmd.DisableAutoGenTag = true

	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "chat",
				Name:           "update_custom_shortcut",
				CanonicalPath:  "chat.update_custom_shortcut",
				CLIPath:        "chat toolbar update-custom",
				PrimaryCLIPath: "chat toolbar update-custom",
			},
			Description: "更新自定义快捷栏入口",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "im", RPCName: "update_custom_shortcut"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新自定义快捷栏入口",
				UseWhen:      []string{"需要修改已有自定义快捷栏入口的标题、链接、图标等属性"},
				AvoidWhen:    []string{"创建新入口使用 chat toolbar create-custom；删除入口使用 chat toolbar remove-custom"},
				Examples:     []string{"dws chat toolbar update-custom --conversation-id <cid> --shortcut-id 123 --title \"周报\" --url \"https://example.com\" --icon-url \"https://example.com/icon.png\" --pc-url \"https://example.com\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(true)},
				{Name: "shortcut-id", Property: "shortcutId", Required: boolPtr(true)},
				{Name: "title", Property: "name", Required: boolPtr(true)},
				{Name: "url", Property: "url", Required: boolPtr(true)},
				{Name: "icon-url", Property: "icons", Required: boolPtr(true)},
				{Name: "pc-url", Property: "pcUrl", Required: boolPtr(true)},
				{Name: "extension", Property: "extension", Required: boolPtr(false)},
				{Name: "desc", Property: "desc", Required: boolPtr(false)},
				{Name: "tag", Property: "tag", Required: boolPtr(false)},
				{Name: "sort-index", Property: "sortIndex", Required: boolPtr(false)},
			},
		},
	})

	return cmd
}
