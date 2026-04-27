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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
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
		newChatMessageSendCommand(runner),
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

func newChatMessageSendCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "以当前用户身份发送消息 (--group 群聊 / --user 或 --open-dingtalk-id 单聊)",
		Long: `以当前用户身份发送群消息或单聊消息。

--group 指定群聊 openConversationId 发群消息；--user 指定 userId 发单聊；
--open-dingtalk-id 指定 openDingTalkId 发单聊 (适用于无法获取 userId 的场景)。
三者只能选其一，不能同时指定。

消息内容通过 --text 传入，也可作为位置参数；支持 Markdown。可选 --title 作为消息标题。

群聊场景下可用 --at-all / --at-users / --at-mobiles 进行 @ 提醒（仅 --group 时生效）。
注意 --text 中需包含对应的 <@userId> / <@all> 占位符才能在客户端渲染出 @ 效果。`,
		Example: `  dws chat message send --group <openconversation_id> --text "hello"
  dws chat message send --user <userId> --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --title "提醒" --text "请确认"
  dws chat message send --group <openconversation_id> --title "拉群通知" --text "<@uid> 你被 @ 了" --at-users uid`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params, tool, err := buildChatMessageSendInvocation(cmd, args)
			if err != nil {
				return err
			}
			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"chat",
				tool,
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)

	cmd.Flags().String("group", "", "群会话 openConversationId (群聊三选一)")
	cmd.Flags().String("user", "", "接收人 userId (单聊三选一)")
	cmd.Flags().String("open-dingtalk-id", "", "接收人 openDingTalkId (单聊三选一)")
	cmd.Flags().String("text", "", "消息内容，支持 Markdown (也可作位置参数)")
	cmd.Flags().String("title", "", "消息标题 (可选)")
	cmd.Flags().Bool("at-all", false, "@所有人 (仅 --group 群聊生效)")
	cmd.Flags().String("at-users", "", "按 userId @ 指定成员，逗号分隔 (仅 --group 群聊生效)")
	cmd.Flags().String("at-mobiles", "", "按手机号 @ 指定成员，逗号分隔 (仅 --group 群聊生效)")
	return cmd
}

func buildChatMessageSendInvocation(cmd *cobra.Command, args []string) (map[string]any, string, error) {
	guard := cli.NewStdinGuard()

	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --group")
	}
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --user")
	}
	openID, err := cmd.Flags().GetString("open-dingtalk-id")
	if err != nil {
		return nil, "", apperrors.NewInternal("failed to read --open-dingtalk-id")
	}
	title, err := resolveStringFlag(cmd, "title", guard, false)
	if err != nil {
		return nil, "", err
	}
	// --text is the primary content flag: receives stdin pipe and positional
	// fallback when empty.
	text, err := resolveStringFlag(cmd, "text", guard, true)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(text) == "" && len(args) > 0 {
		text = args[0]
	}

	hasGroup := strings.TrimSpace(group) != ""
	hasUser := strings.TrimSpace(user) != ""
	hasOpenID := strings.TrimSpace(openID) != ""
	specified := 0
	if hasGroup {
		specified++
	}
	if hasUser {
		specified++
	}
	if hasOpenID {
		specified++
	}
	switch specified {
	case 0:
		return nil, "", apperrors.NewValidation("one of --group, --user, or --open-dingtalk-id is required")
	case 1:
	default:
		return nil, "", apperrors.NewValidation("--group, --user, and --open-dingtalk-id are mutually exclusive")
	}
	if strings.TrimSpace(text) == "" {
		return nil, "", apperrors.NewValidation("--text (or positional argument) is required")
	}

	atAll, _ := cmd.Flags().GetBool("at-all")
	atUsers, _ := cmd.Flags().GetString("at-users")
	atMobiles, _ := cmd.Flags().GetString("at-mobiles")
	hasAtUsers := strings.TrimSpace(atUsers) != ""
	hasAtMobiles := strings.TrimSpace(atMobiles) != ""
	if !hasGroup && (atAll || hasAtUsers || hasAtMobiles) {
		return nil, "", apperrors.NewValidation("--at-all / --at-users / --at-mobiles only apply when --group is set")
	}

	params := map[string]any{"text": text}
	if strings.TrimSpace(title) != "" {
		params["title"] = title
	}

	switch {
	case hasGroup:
		params["openConversation_id"] = group
		if atAll {
			params["isAtAll"] = true
		}
		if hasAtUsers {
			params["atUserIds"] = splitCSV(atUsers)
		}
		if hasAtMobiles {
			params["atMobiles"] = splitCSV(atMobiles)
		}
		return params, "send_message_as_user", nil
	case hasUser:
		params["receiverUserId"] = user
		return params, "send_direct_message_as_user", nil
	default:
		params["receiverOpenDingTalkId"] = openID
		return params, "send_direct_message_as_user", nil
	}
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

			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"bot",
				tool,
				params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	// Keeps the helper-restructured group winning over the dynamic envelope's
	// `members` leaf (which only exposes `get_group_members`); without this
	// the merge layer treats the shape mismatch as "envelope is authority"
	// and drops the entire helper subtree (issue #164).
	preferLegacyLeaf(members)

	members.AddCommand(
		newChatGroupMembersListCommand(runner),
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

			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"chat",
				"create_internal_group",
				map[string]any{
					"groupMembers": stringSliceToAny(allMembers),
					"groupName":    name,
				},
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
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
	guard := cli.NewStdinGuard()

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
	title, err := resolveStringFlag(cmd, "title", guard, false)
	if err != nil {
		return nil, "", err
	}
	// --text is the primary content flag: receives stdin pipe when empty.
	text, err := resolveStringFlag(cmd, "text", guard, true)
	if err != nil {
		return nil, "", err
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
				inv := executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "bot", "recall_robot_group_message", params,
				)
				inv.DryRun = commandDryRun(cmd)
				result, err := runner.Run(cmd.Context(), inv)
				if err != nil {
					return err
				}
				return writeCommandPayload(cmd, result)
			}
			params := map[string]any{
				"robotCode":        robotCode,
				"processQueryKeys": processQueryKeys,
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "batch_recall_robot_users_msg", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
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
			guard := cli.NewStdinGuard()
			token, _ := cmd.Flags().GetString("token")
			title, err := resolveStringFlag(cmd, "title", guard, false)
			if err != nil {
				return err
			}
			// --text is the primary content flag: receives stdin pipe when empty.
			text, err := resolveStringFlag(cmd, "text", guard, true)
			if err != nil {
				return err
			}
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
			invocation := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "send_message_by_custom_robot", params,
			)
			invocation.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), invocation)
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

func newChatGroupMembersListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "查询群成员列表",
		Example:           `  dws chat group members list --id <openconversation_id>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
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
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	cmd.Flags().String("cursor", "", "分页游标 (首页留空)")
	return cmd
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
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "chat", "update_group_name", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
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
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "chat", "add_group_member", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
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
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "chat", "remove_group_member", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
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
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "bot", "add_robot_to_group", params,
			)
			inv.DryRun = commandDryRun(cmd)
			result, err := runner.Run(cmd.Context(), inv)
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
