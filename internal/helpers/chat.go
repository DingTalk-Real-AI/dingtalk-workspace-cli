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
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return chatHandler{}
	})
}

// chatHandler retains only the chat commands that carry real business logic
// (intelligent tool routing, current-user resolution, response normalization,
// or stdin/@file input support that dynamic commands do not yet provide).
// Thin wrappers — search, group rename, group members list/add/remove/add-bot,
// bot search — are now produced by the dynamic service-discovery envelope
// (envelope/pre-discovery.json) so the helper does not have to duplicate them.
type chatHandler struct{}

func (chatHandler) Name() string {
	return "chat"
}

func (chatHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "chat",
		Short:             "群聊 / 消息 / 机器人",
		Long:              "钉钉会话与群聊：发送消息（用户/机器人/Webhook）、撤回机器人消息、创建群。其余命令由服务发现 envelope 提供。",
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
		newChatMessageReplyCommand(runner),
	)

	group := &cobra.Command{
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
	members.AddCommand(
		newChatGroupMembersAddBotCommand(runner),
		newChatGroupMembersRemoveBotCommand(runner),
	)
	group.AddCommand(
		newChatGroupCreateCommand(runner),
		newChatGroupBotsCommand(runner),
		members,
	)

	bot := &cobra.Command{
		Use:               "bot",
		Short:             "机器人查询",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	bot.AddCommand(
		newChatBotFindCommand(runner),
		newChatBotSearchCommand(runner),
	)

	root.AddCommand(message, group, bot)
	return root
}

// botInvoke 把 bot 相关命令统一路由到 "bot" MCP server，与 wukong 的
// callMCPToolOnServer("bot", ...) 对齐。
func botInvoke(runner executor.Runner, cmd *cobra.Command, tool string, params map[string]any) error {
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
}

func newChatBotFindCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "find",
		Short:             "搜索全部可用机器人（含他人/官方，额外返回 openDingTalkId 可发单聊）",
		Long:              "按关键词搜索当前用户可用的全部机器人（含他人创建、官方），支持游标分页。find 返回 openDingTalkId（可给机器人发单聊）；只搜自己创建的用 dws chat bot search。",
		Example:           "  dws chat bot find --query \"日报\"\n  dws chat bot find --query \"日报\" --limit 20",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, _ := cmd.Flags().GetString("query")
			if strings.TrimSpace(query) == "" {
				query, _ = cmd.Flags().GetString("keyword")
			}
			if strings.TrimSpace(query) == "" {
				return apperrors.NewValidation("--query is required")
			}
			params := map[string]any{"keyword": query}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				params["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				params["cursor"] = v
			}
			return botInvoke(runner, cmd, "search_bots", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	cmd.Flags().String("keyword", "", "--query 的别名")
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().Int("limit", 20, "每页返回数量（默认 20）")
	cmd.Flags().String("cursor", "", "分页游标（首次不传，翻页传上次返回的 nextCursor）")
	return cmd
}

func newChatBotSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             "搜索我创建的机器人",
		Long:              "按名称搜索当前用户自己创建的机器人。搜全部（含他人/官方）用 dws chat bot find。",
		Example:           "  dws chat bot search --name \"日报\"",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				params["robotName"] = v
			}
			if v, _ := cmd.Flags().GetInt("page"); v > 0 {
				params["currentPage"] = v
			}
			if v, _ := cmd.Flags().GetInt("size"); v > 0 {
				params["pageSize"] = v
			}
			return botInvoke(runner, cmd, "search_my_robots", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", "机器人名称关键词（可选）")
	cmd.Flags().Int("page", 0, "页码（可选）")
	cmd.Flags().Int("size", 0, "每页数量（可选）")
	return cmd
}

func newChatGroupBotsCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "bots",
		Short:             "查看群内所有机器人",
		Example:           "  dws chat group bots --group <openConversationId>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			group, _ := cmd.Flags().GetString("group")
			if strings.TrimSpace(group) == "" {
				return apperrors.NewValidation("--group is required")
			}
			return botInvoke(runner, cmd, "list_group_bots", map[string]any{"openConversationId": group})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	return cmd
}

func newChatGroupMembersAddBotCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "add-bot",
		Short:             "将机器人添加到群中",
		Example:           "  dws chat group members add-bot --id <openConversationId> --robot-code <robotCode>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := cmd.Flags().GetString("id")
			robotCode, _ := cmd.Flags().GetString("robot-code")
			if strings.TrimSpace(id) == "" || strings.TrimSpace(robotCode) == "" {
				return apperrors.NewValidation("--id and --robot-code are required")
			}
			return botInvoke(runner, cmd, "add_robot_to_group", map[string]any{
				"openConversationId": id,
				"robotCode":          robotCode,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("id", "", "群聊 openConversationId (必填)")
	cmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	return cmd
}

func newChatGroupMembersRemoveBotCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove-bot",
		Short:             "从群内移除机器人",
		Example:           "  dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, _ := cmd.Flags().GetString("id")
			botID, _ := cmd.Flags().GetString("bot-id")
			if strings.TrimSpace(id) == "" || strings.TrimSpace(botID) == "" {
				return apperrors.NewValidation("--id and --bot-id are required")
			}
			return botInvoke(runner, cmd, "remove_robot_in_group", map[string]any{
				"openConversationId": id,
				"openBotId":          botID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("id", "", "群聊 openConversationId (必填)")
	cmd.Flags().String("bot-id", "", "机器人 openBotId (必填)")
	return cmd
}

func newChatMessageSendCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "以当前用户身份发送消息 (--group 群聊 / --user 或 --open-dingtalk-id 单聊)",
		Long: `以当前用户身份发送群消息或单聊消息。

--group 指定群聊 openConversationId 发群消息；--user 指定 userId 发单聊；
--open-dingtalk-id 指定 openDingTalkId 发单聊 (适用于无法获取 userId 的场景)。
三者只能选其一，不能同时指定。

消息内容通过 --text 传入，也可作为位置参数；支持 Markdown。
--title 是消息标题，群聊与单聊都必填（API 强制要求；缺失时返回误导性的 "发群服务窗会话消息失败"）。

群聊场景下可用 --at-all / --at-open-dingtalk-ids 进行 @ 提醒（仅 --group 时生效）。
富媒体：--msg-type image --media-id 发图片；--msg-type file --dentry-id --space-id --file-name 发钉盘文件。`,
		Example: `  dws chat message send --group <openconversation_id> --title "周报" --text "请提交本周日报"
  dws chat message send --user <userId> --title "提醒" --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --title "提醒" --text "请确认"
  dws chat message send --group <openconversation_id> --msg-type image --media-id <mediaId>`,
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
	cmd.Flags().String("title", "", "消息标题 (可选，未指定时从内容截取)")
	cmd.Flags().Bool("at-all", false, "@所有人 (仅 --group 群聊生效)")
	cmd.Flags().String("at-open-dingtalk-ids", "", "@指定成员 openDingTalkId 列表，逗号分隔 (仅 --group 群聊生效)")
	cmd.Flags().String("uuid", "", "幂等 UUID (可选，24h 内相同 uuid 不重复发送)")
	cmd.Flags().String("msg-type", "", "富媒体类型: image / file (纯文本/Markdown 留空)")
	cmd.Flags().String("media-id", "", "图片 mediaId (msg-type=image 时必填)")
	cmd.Flags().Int64("dentry-id", 0, "钉盘文件 dentryId (msg-type=file 时必填)")
	cmd.Flags().Int64("space-id", 0, "钉盘空间 ID (msg-type=file 时必填)")
	cmd.Flags().String("file-name", "", "文件名 (msg-type=file 时必填)")
	cmd.Flags().String("file-type", "", "文件类型/扩展名 (msg-type=file)")
	cmd.Flags().String("file-path", "", "文件展示路径 (msg-type=file)")
	cmd.Flags().Int64("file-size", 0, "文件大小，单位字节 (msg-type=file)")
	cmd.Flags().Bool("ai-tag", false, "标记为「通过AI发送」(默认不带；仅传 --ai-tag 时才在消息下方显示 AI 发送角标)")
	return cmd
}

// attachAITag 仅在用户显式传入 --ai-tag 时，给发送参数加上 clawType，
// 由 IM 服务端据此渲染「通过AI发送」角标 (悟空版渲染「悟空AI发送」)。
// 默认不带：是否标记 AI 发送交由用户自行选择，不强加。
func attachAITag(cmd *cobra.Command, params map[string]any) {
	if on, _ := cmd.Flags().GetBool("ai-tag"); on {
		params["clawType"] = edition.ClawType()
	}
}

// deriveTitleFromText 在未显式指定 --title 时，从正文截取一个标题
// (首行、最多 20 个字符)，与 wukong 行为对齐 (send_personal_message 的
// content 内携带 title)。
func deriveTitleFromText(text string) string {
	t := strings.TrimSpace(text)
	if i := strings.IndexAny(t, "\r\n"); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	r := []rune(t)
	if len(r) > 20 {
		r = r[:20]
	}
	if len(r) == 0 {
		return "消息"
	}
	return string(r)
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

	uuid, _ := cmd.Flags().GetString("uuid")

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

	// ── 富媒体消息 (image / file)：走 send_personal_message，后端 schema 支持
	// content + msgType (image 经 content 携带 mediaId；file 经 content 携带
	// dentryId/spaceId)。本地 --file-path 自动上传暂未移植，使用钉盘 dentry/space。
	msgType, _ := cmd.Flags().GetString("msg-type")
	if msgType == "text" || msgType == "markdown" {
		msgType = ""
	}
	if msgType != "" {
		var contentJSON string
		switch msgType {
		case "image":
			mediaID, _ := cmd.Flags().GetString("media-id")
			if strings.TrimSpace(mediaID) == "" {
				return nil, "", apperrors.NewValidation("--media-id is required for --msg-type image")
			}
			b, _ := json.Marshal(map[string]string{"mediaId": mediaID})
			contentJSON = string(b)
		case "file":
			dentryID, _ := cmd.Flags().GetInt64("dentry-id")
			spaceID, _ := cmd.Flags().GetInt64("space-id")
			fileName, _ := cmd.Flags().GetString("file-name")
			if dentryID == 0 || spaceID == 0 || strings.TrimSpace(fileName) == "" {
				return nil, "", apperrors.NewValidation("--msg-type file 需要 --dentry-id、--space-id、--file-name (本地 --file-path 自动上传暂未支持)")
			}
			fileType, _ := cmd.Flags().GetString("file-type")
			filePath, _ := cmd.Flags().GetString("file-path")
			fileSize, _ := cmd.Flags().GetInt64("file-size")
			b, _ := json.Marshal(map[string]any{
				"dentryId": dentryID, "spaceId": spaceID, "fileName": fileName,
				"fileType": fileType, "filePath": filePath, "fileSize": fileSize,
			})
			contentJSON = string(b)
		default:
			return nil, "", apperrors.NewValidation("unsupported --msg-type: " + msgType + " (supported: image, file)")
		}
		params := map[string]any{"msgType": msgType, "content": contentJSON}
		attachAITag(cmd, params)
		if strings.TrimSpace(uuid) != "" {
			params["uuid"] = uuid
		}
		switch {
		case hasGroup:
			params["openConversationId"] = group
		case hasOpenID:
			params["receiverOpenDingTalkId"] = openID
		default:
			return nil, "", apperrors.NewValidation("--msg-type image/file 需配合 --group 或 --open-dingtalk-id (--user 暂不支持富媒体)")
		}
		return params, "send_personal_message", nil
	}

	// ── 文本 / Markdown 消息 ──
	if strings.TrimSpace(text) == "" {
		return nil, "", apperrors.NewValidation("--text (or positional argument) is required")
	}
	if strings.TrimSpace(title) == "" {
		title = deriveTitleFromText(text)
	}

	atAll, _ := cmd.Flags().GetBool("at-all")
	atOpenIDs, _ := cmd.Flags().GetString("at-open-dingtalk-ids")
	hasAtOpenIDs := strings.TrimSpace(atOpenIDs) != ""
	if !hasGroup && (atAll || hasAtOpenIDs) {
		return nil, "", apperrors.NewValidation("--at-all / --at-open-dingtalk-ids only apply when --group is set")
	}

	switch {
	case hasGroup:
		if atAll && !strings.Contains(text, "<@all>") {
			text = "<@all> " + text
		}
		params := map[string]any{
			"openConversationId": group,
			"msgType":            "markdown",
			"content":            marshalMessageContent(title, text),
		}
		attachAITag(cmd, params)
		if atAll {
			params["atAll"] = true
		}
		if hasAtOpenIDs {
			params["atOpenDingTalkIds"] = splitCSVStrings(atOpenIDs)
		}
		if strings.TrimSpace(uuid) != "" {
			params["uuid"] = uuid
		}
		return params, "send_personal_message", nil
	case hasUser:
		params := map[string]any{"title": title, "text": text, "receiverUserId": user}
		attachAITag(cmd, params)
		return params, "send_direct_message_as_user", nil
	default:
		params := map[string]any{
			"receiverOpenDingTalkId": openID,
			"msgType":                "markdown",
			"content":                marshalMessageContent(title, text),
		}
		attachAITag(cmd, params)
		if strings.TrimSpace(uuid) != "" {
			params["uuid"] = uuid
		}
		return params, "send_personal_message", nil
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
	cmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	cmd.Flags().String("text", "", "消息内容 Markdown (必填)")
	cmd.Flags().String("title", "", "消息标题 (必填)")
	cmd.Flags().String("users", "", "接收者 userId 列表，逗号分隔，最多 20 个 (单聊必填)")
	return cmd
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

			groupType := strings.ToUpper(strings.TrimSpace(cmd.Flags().Lookup("type").Value.String()))
			if groupType == "" {
				groupType = "INTERNAL"
			}
			switch groupType {
			case "INTERNAL", "EXTERNAL", "NORMAL":
			default:
				return apperrors.NewValidation("--type must be one of INTERNAL, EXTERNAL, NORMAL")
			}
			threadEnabled, _ := cmd.Flags().GetBool("thread")

			currentUserID, err := getCurrentUserID(cmd.Context(), runner)
			if err != nil {
				return err
			}
			allMembers := prependOwner(currentUserID, memberUserIDs)

			// create_group_conversation (multi-type + thread support) and the
			// legacy create_internal_group live on two different MCP servers
			// ("im" and "group-chat") that both publish `dws chat ...`. Route
			// each tool to its owning server explicitly so direct-runtime
			// endpoint resolution does not collapse them onto the shared
			// cli.command endpoint (which would send create_group_conversation
			// to the group-chat server, where it is not registered).
			product := "im"
			tool := "create_group_conversation"
			params := map[string]any{
				"groupMembers":      stringSliceToAny(allMembers),
				"groupName":         name,
				"groupType":         groupType,
				"convThreadEnabled": threadEnabled,
			}

			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				product,
				tool,
				params,
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
	cmd.Flags().String("type", "INTERNAL", "群类型: INTERNAL(企业内部群) / EXTERNAL(外部群) / NORMAL(普通群)，默认 INTERNAL")
	cmd.Flags().Bool("thread", false, "开启话题圈 (convThreadEnabled)")
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
//
// Kept as a helper (rather than delegating to the dynamic envelope) because
// it needs --text @file / stdin pipe support via resolveStringFlag, which the
// dynamic-command layer does not yet provide.

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

// ── message reply ────────────────────────────────────────
//
// Kept as a helper because the underlying MCP tool send_personal_message
// requires the reply payload to be a JSON-encoded string assembled from
// --ref-msg-id / --ref-sender / --text. Envelope toolOverride does flat
// flag→param mapping only and cannot construct nested JSON, so this
// orchestration must live in CLI code.

func newChatMessageReplyCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "reply",
		Short:             "引用回复消息（支持单聊/群聊）",
		Long:              "以当前用户身份引用某条消息并回复。需 --conversation-id 会话 ID、--ref-msg-id 被引用消息 ID、--ref-sender 原发送者 openDingTalkId、--text 回复内容。",
		Example:           `  dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <openDingTalkId> --text "收到，马上处理"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID, _ := cmd.Flags().GetString("conversation-id")
			refMsgID, _ := cmd.Flags().GetString("ref-msg-id")
			refSender, _ := cmd.Flags().GetString("ref-sender")
			text, _ := cmd.Flags().GetString("text")
			if strings.TrimSpace(convID) == "" {
				return apperrors.NewValidation("--conversation-id is required")
			}
			if strings.TrimSpace(refMsgID) == "" {
				return apperrors.NewValidation("--ref-msg-id is required")
			}
			if strings.TrimSpace(refSender) == "" {
				return apperrors.NewValidation("--ref-sender is required")
			}
			if strings.TrimSpace(text) == "" {
				return apperrors.NewValidation("--text is required")
			}
			replyContent := map[string]any{
				"referenceOpenMessageId":   refMsgID,
				"srcMsgSendOpenDingTalkId": refSender,
				"replyMsgType":             "text",
				"content":                  text,
			}
			contentJSON, err := jsonMarshal(replyContent)
			if err != nil {
				return apperrors.NewInternal("marshal reply content: " + err.Error())
			}
			params := map[string]any{
				"openConversationId": convID,
				"msgType":            "reply",
				"content":            contentJSON,
			}
			// clawType 仅在 --ai-tag 时携带；默认不带，回复不强加 AI 角标。
			// edition 决定取值 (开源 openClaw / 悟空 wukong)。
			attachAITag(cmd, params)
			if uuid, _ := cmd.Flags().GetString("uuid"); strings.TrimSpace(uuid) != "" {
				params["uuid"] = uuid
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd),
				"group-chat",
				"send_personal_message",
				params,
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
	cmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	cmd.Flags().String("ref-msg-id", "", "被引用的消息 openMessageId (必填)")
	cmd.Flags().String("ref-sender", "", "被引用消息发送者 openDingTalkId (必填)")
	cmd.Flags().String("text", "", "回复正文 (必填)")
	cmd.Flags().String("uuid", "", "可选 uuid（幂等标识）")
	cmd.Flags().Bool("ai-tag", false, "标记为「通过AI发送」(默认不带；仅传 --ai-tag 时才显示 AI 发送角标)")
	return cmd
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalMessageContent builds the send_personal_message content payload
// ({"title","text"}) WITHOUT HTML-escaping < > &. DingTalk's client renders
// @-mentions by matching literal <@openDingTalkId> / <@all> tokens in the
// message text; the default json.Marshal escaping turns them into
// <@...>, which the client shows as plain text instead of a rendered
// mention. encoding/json offers no escape toggle on Marshal, so use an Encoder.
func marshalMessageContent(title, text string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	// Encoder errors are impossible for a map[string]string; ignore safely.
	_ = enc.Encode(map[string]string{"title": title, "text": text})
	// Encoder.Encode appends a trailing newline; strip it.
	return strings.TrimRight(buf.String(), "\n")
}
