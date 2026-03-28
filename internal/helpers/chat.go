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

package helpers

import (
	"context"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return chatHandler{}
	})
}

type chatHandler struct{}

func (chatHandler) Name() string {
	return "chat"
}

func (chatHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "chat",
		Short:             "群聊 / 消息 / 机器人",
		Long:              "管理钉钉会话与群聊：创建群、搜索群、查看群成员、添加机器人到群、修改群名称、拉取会话消息、发送群消息、机器人消息与 Webhook。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	message := &cobra.Command{
		Use:               "message",
		Short:             "会话消息管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	message.AddCommand(
		newChatMessageSendByBotCommand(runner),
		newChatMessageRecallByBotCommand(runner),
		newChatMessageSendByWebhookCommand(runner),
	)

	bot := &cobra.Command{
		Use:               "bot",
		Short:             "机器人管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	bot.AddCommand(newChatBotSearchCommand(runner))

	root.AddCommand(message, newChatSearchCommand(runner), newChatGroupCommand(runner), bot)
	return root
}

func newChatMessageSendByBotCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "send-by-bot",
		Short:             "机器人发送消息（--group 群聊 / --users 单聊）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, tool, err := buildChatMessageSendByBotInvocation(cmd)
			if err != nil {
				return err
			}

			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"chat",
				tool,
				params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("group", "", "群会话 openConversationId (群聊必填)")
	cmd.Flags().String("robot-code", "", "机器人 Code")
	cmd.Flags().String("text", "", "消息内容 (Markdown)")
	cmd.Flags().String("title", "", "消息标题")
	cmd.Flags().String("users", "", "接收者 userId 列表，逗号分隔，最多 20 个 (单聊必填)")
	return cmd
}

func newChatSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             "根据名称搜索会话列表",
		Example:           `  dws chat search --query "项目冲刺"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := cmd.Flags().GetString("query")
			if err != nil {
				return apperrors.NewInternal("failed to read --query")
			}
			query = strings.TrimSpace(query)
			if query == "" {
				return apperrors.NewValidation("--query is required")
			}

			searchReq := map[string]any{"query": query}
			cursor, err := cmd.Flags().GetString("cursor")
			if err != nil {
				return apperrors.NewInternal("failed to read --cursor")
			}
			if strings.TrimSpace(cursor) != "" {
				searchReq["cursor"] = cursor
			}

			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"chat",
				"search_groups_by_keyword",
				map[string]any{"OpenSearchRequest": searchReq},
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	cmd.Flags().String("cursor", "", "分页游标 (首页留空)")
	return cmd
}

func newChatGroupCommand(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "group",
		Short:             "群组管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	members := &cobra.Command{
		Use:               "members",
		Short:             "群成员管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              newChatGroupMembersListRunE(runner),
	}
	members.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	members.Flags().String("cursor", "", "分页游标")
	preferLegacyLeaf(members)

	members.AddCommand(
		newChatGroupMemberAddCommand(runner),
		newChatGroupMemberRemoveCommand(runner),
		newChatGroupMembersAddBotCommand(runner),
	)

	root.AddCommand(
		newChatGroupCreateCommand(runner),
		members,
		newChatGroupRenameCommand(runner),
	)
	return root
}

func newChatGroupCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "创建群",
		Example:           `  dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := cmd.Flags().GetString("name")
			if err != nil {
				return apperrors.NewInternal("failed to read --name")
			}
			name = strings.TrimSpace(name)
			if name == "" {
				return apperrors.NewValidation("--name is required")
			}

			users, err := cmd.Flags().GetString("users")
			if err != nil {
				return apperrors.NewInternal("failed to read --users")
			}
			memberUserIDs := splitCSVStrings(users)
			if len(memberUserIDs) == 0 {
				return apperrors.NewValidation("--users is required")
			}

			currentUserID, err := getCurrentUserID(cmd.Context(), runner)
			if err != nil {
				return err
			}
			allMembers := prependOwner(currentUserID, memberUserIDs)

			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"chat",
				"create_internal_group",
				map[string]any{
					"groupMembers": stringSliceToAny(allMembers),
					"groupName":    name,
				},
			))
			if err != nil {
				return err
			}
			normalizeChatGroupCreateResponse(&result)
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("name", "", "群名称 (必填)")
	cmd.Flags().String("users", "", "群成员 userId 列表，逗号分隔 (必填)")
	return cmd
}

func buildChatMessageSendByBotInvocation(cmd *cobra.Command) (map[string]any, string, error) {
	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --group")
	}
	users, err := cmd.Flags().GetString("users")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --users")
	}
	robotCode, err := cmd.Flags().GetString("robot-code")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --robot-code")
	}
	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --title")
	}
	text, err := cmd.Flags().GetString("text")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --text")
	}

	switch {
	case strings.TrimSpace(group) == "" && strings.TrimSpace(users) == "":
		return nil, "", apperrors.NewValidation("either --group or --users is required")
	case strings.TrimSpace(group) != "" && strings.TrimSpace(users) != "":
		return nil, "", apperrors.NewValidation("--group and --users are mutually exclusive")
	}
	if strings.TrimSpace(robotCode) == "" {
		return nil, "", apperrors.NewValidation("--robot-code is required")
	}
	if strings.TrimSpace(title) == "" {
		return nil, "", apperrors.NewValidation("--title is required")
	}
	if strings.TrimSpace(text) == "" {
		return nil, "", apperrors.NewValidation("--text is required")
	}

	params := map[string]any{
		"markdown":  text,
		"robotCode": robotCode,
		"title":     title,
	}
	if strings.TrimSpace(group) != "" {
		params["openConversationId"] = group
		return params, "send_robot_group_message", nil
	}

	params["userIds"] = splitCSV(users)
	return params, "batch_send_robot_msg_to_users", nil
}

func splitCSV(raw string) []any {
	parts := strings.Split(raw, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func splitCSVStrings(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func getCurrentUserID(ctx context.Context, runner executor.Runner) (string, error) {
	result, err := runner.Run(ctx, executor.NewHelperInvocation(
		"contact raw get_current_user_profile",
		"contact",
		"get_current_user_profile",
		nil,
	))
	if err != nil {
		return "", err
	}
	content := helperResponseContent(result)
	if len(content) == 0 {
		// EchoRunner and dry-run previews do not have runtime content, so fall back
		// to the explicitly provided members instead of failing the preview path.
		if !result.Invocation.Implemented {
			return "", nil
		}
		return "", apperrors.NewInternal("contact.get_current_user_profile returned no content")
	}

	if arr, ok := content["result"].([]any); ok && len(arr) > 0 {
		if first, ok := arr[0].(map[string]any); ok {
			if employee, ok := first["orgEmployeeModel"].(map[string]any); ok {
				if userID, ok := employee["userId"].(string); ok && strings.TrimSpace(userID) != "" {
					return userID, nil
				}
			}
		}
	}
	if object, ok := content["result"].(map[string]any); ok {
		if userID, ok := object["userId"].(string); ok && strings.TrimSpace(userID) != "" {
			return userID, nil
		}
	}
	return "", apperrors.NewInternal("unable to parse userId from contact.get_current_user_profile")
}

func prependOwner(owner string, memberUserIDs []string) []string {
	seen := map[string]bool{}
	allMembers := make([]string, 0, len(memberUserIDs)+1)
	if trimmedOwner := strings.TrimSpace(owner); trimmedOwner != "" {
		seen[trimmedOwner] = true
		allMembers = append(allMembers, trimmedOwner)
	}
	for _, userID := range memberUserIDs {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		allMembers = append(allMembers, trimmed)
	}
	return allMembers
}

func normalizeChatGroupCreateResponse(result *executor.Result) {
	if result == nil {
		return
	}
	content := helperResponseContent(*result)
	if len(content) == 0 {
		return
	}
	payload, ok := content["result"].(map[string]any)
	if !ok {
		return
	}
	if value, exists := payload["openCid"]; exists {
		payload["openConversationId"] = value
		delete(payload, "openCid")
	}
	delete(payload, "cid")
}

func helperResponseContent(result executor.Result) map[string]any {
	if len(result.Response) == 0 {
		return nil
	}
	content, _ := result.Response["content"].(map[string]any)
	return content
}

// ── message recall-by-bot ──────────────────────────────────

func newChatMessageRecallByBotCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recall-by-bot",
		Short: "机器人撤回消息（--group 群聊 / 不传为单聊）",
		Long:  "群聊撤回：传 --group 和 --keys；单聊撤回：只传 --keys。--keys 是逗号分隔的 processQueryKey 列表。",
		Example: `  dws chat message recall-by-bot --robot-code <robot-code> --group <id> --keys <key>
  dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			robotCode, _ := cmd.Flags().GetString("robot-code")
			keysStr, _ := cmd.Flags().GetString("keys")
			groupID, _ := cmd.Flags().GetString("group")
			if strings.TrimSpace(robotCode) == "" {
				return apperrors.NewValidation("--robot-code is required")
			}
			if strings.TrimSpace(keysStr) == "" {
				return apperrors.NewValidation("--keys is required")
			}
			processQueryKeys := splitCSV(keysStr)
			if strings.TrimSpace(groupID) != "" {
				params := map[string]any{
					"robotCode":          robotCode,
					"openConversationId": groupID,
					"processQueryKeys":   processQueryKeys,
				}
				result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "bot", "recall_robot_group_message", params,
				))
				if err != nil {
					return err
				}
				return writeCommandPayload(cmd, result)
			}
			params := map[string]any{
				"robotCode":        robotCode,
				"processQueryKeys": processQueryKeys,
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "batch_recall_robot_users_msg", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	cmd.Flags().String("group", "", "群会话 openConversationId (群聊撤回必填)")
	cmd.Flags().String("keys", "", "逗号分隔的消息 processQueryKey 列表 (必填)")
	return cmd
}

// ── message send-by-webhook ────────────────────────────────

func newChatMessageSendByWebhookCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-by-webhook",
		Short: "自定义机器人 Webhook 发送群消息",
		Long:  "自定义机器人 Webhook 发送群消息。如需 @指定人，在 --text 中包含 @userId 或 @手机号。",
		Example: `  dws chat message send-by-webhook --token <token> --title "告警" --text "CPU 超 90%" --at-all
  dws chat message send-by-webhook --token <token> --title "test" --text "hi" --at-users 034766`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, _ := cmd.Flags().GetString("token")
			title, _ := cmd.Flags().GetString("title")
			text, _ := cmd.Flags().GetString("text")
			if strings.TrimSpace(token) == "" {
				return apperrors.NewValidation("--token is required")
			}
			if strings.TrimSpace(title) == "" {
				return apperrors.NewValidation("--title is required")
			}
			if strings.TrimSpace(text) == "" {
				return apperrors.NewValidation("--text is required")
			}
			params := map[string]any{
				"robotToken": token,
				"title":      title,
				"text":       text,
			}
			if v, _ := cmd.Flags().GetBool("at-all"); v {
				params["isAtAll"] = true
			}
			if v, _ := cmd.Flags().GetString("at-mobiles"); v != "" {
				params["atMobiles"] = splitCSV(v)
			}
			if v, _ := cmd.Flags().GetString("at-users"); v != "" {
				params["atUserIds"] = splitCSV(v)
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "send_message_by_custom_robot", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("token", "", "Webhook token (必填)")
	cmd.Flags().String("title", "", "消息标题 (必填)")
	cmd.Flags().String("text", "", "消息内容 (必填)")
	cmd.Flags().Bool("at-all", false, "@所有人")
	cmd.Flags().String("at-mobiles", "", "按手机号 @，逗号分隔")
	cmd.Flags().String("at-users", "", "按 userId @，逗号分隔")
	return cmd
}

// ── group members list ─────────────────────────────────────

func newChatGroupMembersListRunE(runner executor.Runner) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		groupID, _ := cmd.Flags().GetString("id")
		if strings.TrimSpace(groupID) == "" {
			return apperrors.NewValidation("--id is required")
		}
		params := map[string]any{
			"openconversation_id": groupID,
		}
		if v, _ := cmd.Flags().GetString("cursor"); v != "" {
			params["cursor"] = v
		}
		result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "chat", "get_group_members", params,
		))
		if err != nil {
			return err
		}
		return writeCommandPayload(cmd, result)
	}
}

// ── group rename ───────────────────────────────────────────

func newChatGroupRenameCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rename",
		Short:             "更新群名称",
		Example:           `  dws chat group rename --id <openconversation_id> --name "新群名"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID, _ := cmd.Flags().GetString("id")
			name, _ := cmd.Flags().GetString("name")
			if strings.TrimSpace(groupID) == "" {
				return apperrors.NewValidation("--id is required")
			}
			if strings.TrimSpace(name) == "" {
				return apperrors.NewValidation("--name is required")
			}
			params := map[string]any{
				"openconversation_id": groupID,
				"group_name":          name,
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "chat", "update_group_name", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	cmd.Flags().String("name", "", "新群名称 (必填)")
	return cmd
}

// ── group members add ──────────────────────────────────────

func newChatGroupMemberAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add",
		Short:             "添加群成员",
		Example:           `  dws chat group members add --id <openconversation_id> --users userId1,userId2`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID, _ := cmd.Flags().GetString("id")
			usersStr, _ := cmd.Flags().GetString("users")
			if strings.TrimSpace(groupID) == "" {
				return apperrors.NewValidation("--id is required")
			}
			if strings.TrimSpace(usersStr) == "" {
				return apperrors.NewValidation("--users is required")
			}
			params := map[string]any{
				"openconversation_id": groupID,
				"userId":              splitCSV(usersStr),
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "chat", "add_group_member", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	cmd.Flags().String("users", "", "要添加的 userId 列表，逗号分隔 (必填)")
	return cmd
}

// ── group members remove ───────────────────────────────────

func newChatGroupMemberRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             "移除群成员",
		Example:           `  dws chat group members remove --id <openconversation_id> --users userId1,userId2`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID, _ := cmd.Flags().GetString("id")
			usersStr, _ := cmd.Flags().GetString("users")
			if strings.TrimSpace(groupID) == "" {
				return apperrors.NewValidation("--id is required")
			}
			if strings.TrimSpace(usersStr) == "" {
				return apperrors.NewValidation("--users is required")
			}
			params := map[string]any{
				"openconversationId": groupID,
				"userIdList":         splitCSV(usersStr),
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "chat", "remove_group_member", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("id", "", "Group ID / openconversation_id (required)")
	cmd.Flags().String("users", "", "Comma-separated userId list to remove (required)")
	return cmd
}

// ── group members add-bot ──────────────────────────────────

func newChatGroupMembersAddBotCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add-bot",
		Short:             "Add bot to group",
		Example:           `  dws chat group members add-bot --robot-code <robot-code> --id <openconversation_id>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			robotCode, _ := cmd.Flags().GetString("robot-code")
			groupID, _ := cmd.Flags().GetString("id")
			if strings.TrimSpace(robotCode) == "" {
				return apperrors.NewValidation("--robot-code is required")
			}
			if strings.TrimSpace(groupID) == "" {
				return apperrors.NewValidation("--id is required")
			}
			params := map[string]any{
				"robotCode":          robotCode,
				"openConversationId": groupID,
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "add_robot_to_group", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("robot-code", "", "Bot code (required)")
	cmd.Flags().String("id", "", "Group openConversationId (required)")
	return cmd
}

// ── bot search ─────────────────────────────────────────────

func newChatBotSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             "Search my bots",
		Example:           "  dws chat bot search --page 1\n  dws chat bot search --page 1 --size 10 --name \"日报\"",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			params := map[string]any{
				"currentPage": page,
			}
			if v, _ := cmd.Flags().GetInt("size"); v > 0 {
				params["pageSize"] = v
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				params["robotName"] = v
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "search_my_robots", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Int("page", 1, "Page number, starting from 1")
	cmd.Flags().Int("size", 0, "Items per page (default 50)")
	cmd.Flags().String("name", "", "Search by name")
	return cmd
}
