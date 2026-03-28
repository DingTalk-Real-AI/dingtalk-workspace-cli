package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	cli "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/output"
	"github.com/spf13/cobra"
)

func Add(root *cobra.Command, loader cli.CatalogLoader, runner executor.Runner) error {
	if root == nil || loader == nil {
		return nil
	}
	catalog, err := loader.Load(commandContext(root))
	if err != nil {
		return err
	}
	AddCatalog(root, catalog, runner)
	return nil
}

func AddCatalog(root *cobra.Command, catalog ir.Catalog, runner executor.Runner) {
	if root == nil {
		return
	}
	addChatMessageCompat(root, catalog, runner)
}

func addChatMessageCompat(root *cobra.Command, catalog ir.Catalog, runner executor.Runner) {
	chat := childByName(root, "chat")
	if chat == nil {
		return
	}

	message := childByName(chat, "message")
	if message == nil {
		message = newGroupCommand("message", "会话消息管理")
		chat.AddCommand(message)
	}

	hideChatMessageRawCommands(message)

	if cmd := newChatMessageListCommand(catalog, runner); cmd != nil {
		replaceChildCommand(message, cmd)
	}
	if cmd := newChatMessageSendCommand(catalog, runner); cmd != nil {
		replaceChildCommand(message, cmd)
	}
	if cmd := newChatMessageSendByBotCommand(catalog, runner); cmd != nil {
		replaceChildCommand(message, cmd)
	}
	if cmd := newChatMessageRecallByBotCommand(catalog, runner); cmd != nil {
		replaceChildCommand(message, cmd)
	}
	if cmd := newChatMessageSendByWebhookCommand(catalog, runner); cmd != nil {
		replaceChildCommand(message, cmd)
	}
	if cmd := newChatMessageListTopicRepliesCommand(catalog, runner); cmd != nil {
		replaceChildCommand(message, cmd)
	}
	pruneChatMessageRawCommands(message)
}

func newChatMessageListCommand(catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	groupProduct, groupTool, ok := catalog.FindTool("group-chat.list_conversation_message_v2")
	if !ok {
		return nil
	}
	userProduct, userTool, ok := catalog.FindTool("group-chat.list_individual_chat_message")
	if !ok {
		return nil
	}

	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"list_conversation_message_v2", "list_individual_chat_message"},
		Short:             "拉取会话消息内容",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := firstNonEmptyFlag(cmd, "group", "conversation-id", "id", "chat", "open-conversation-id")
			userID := firstNonEmptyFlag(cmd, "user")
			if groupID != "" && userID != "" {
				return fmt.Errorf("--group and --user are mutually exclusive")
			}
			if groupID == "" && userID == "" {
				return fmt.Errorf("--group or --user is required")
			}
			timeValue := firstNonEmptyFlag(cmd, "time")
			if timeValue == "" {
				return fmt.Errorf("--time is required")
			}
			forward, _ := cmd.Flags().GetBool("forward")
			if groupID != "" {
				params := map[string]any{
					"openconversation_id": groupID,
					"time":                timeValue,
					"forward":             forward,
				}
				if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
					params["limit"] = limit
				}
				return runCanonicalInvocation(cmd, groupProduct, groupTool, params, runner)
			}
			params := map[string]any{
				"userId":  userID,
				"time":    timeValue,
				"forward": forward,
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				params["limit"] = limit
			}
			return runCanonicalInvocation(cmd, userProduct, userTool, params, runner)
		},
	}
	cmd.Flags().String("group", "", "群聊 openconversation_id（群聊时必填）")
	cmd.Flags().String("user", "", "单聊用户 userId（单聊时必填）")
	cmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss (必填)")
	cmd.Flags().Bool("forward", true, "true=拉给定时间之后的消息，false=拉给定时间之前的消息")
	cmd.Flags().Int("limit", 0, "返回数量，不传则不限制")
	addGroupAliases(cmd)
	return cmd
}

func newChatMessageSendCommand(catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	groupProduct, groupTool, ok := catalog.FindTool("group-chat.send_message_as_user")
	if !ok {
		return nil
	}
	userProduct, userTool, ok := catalog.FindTool("group-chat.send_direct_message_as_user")
	if !ok {
		return nil
	}

	cmd := &cobra.Command{
		Use:               "send",
		Aliases:           []string{"send_message_as_user", "send_direct_message_as_user"},
		Short:             "以当前用户身份发送消息（--group 群聊 / --user 单聊）",
		DisableAutoGenTag: true,
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := firstNonEmptyFlag(cmd, "group", "conversation-id", "id", "chat", "open-conversation-id")
			userID := firstNonEmptyFlag(cmd, "user")
			if groupID != "" && userID != "" {
				return fmt.Errorf("--group and --user are mutually exclusive")
			}
			if groupID == "" && userID == "" {
				return fmt.Errorf("--group or --user is required")
			}

			text := firstNonEmptyFlag(cmd, "text", "content", "body", "message", "markdown")
			if text == "" && len(args) > 0 {
				text = args[0]
			}
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("message content required (use --text or positional arg)")
			}

			title, _ := cmd.Flags().GetString("title")
			if groupID != "" {
				params := map[string]any{
					"openConversation_id": groupID,
					"title":               title,
					"text":                text,
				}
				atAll, _ := cmd.Flags().GetBool("at-all")
				if atAll {
					params["atAll"] = true
				}
				if atUsers := splitCSV(firstNonEmptyFlag(cmd, "at-users")); len(atUsers) > 0 {
					params["atUserIds"] = atUsers
				}
				return runCanonicalInvocation(cmd, groupProduct, groupTool, params, runner)
			}
			params := map[string]any{
				"receiverUserId": userID,
				"title":          title,
				"text":           text,
			}
			return runCanonicalInvocation(cmd, userProduct, userTool, params, runner)
		},
	}
	cmd.Flags().String("group", "", "群聊 openconversation_id（群聊时必填）")
	cmd.Flags().String("user", "", "接收人 userId（单聊时必填）")
	cmd.Flags().String("title", "消息", "消息标题，显示在消息列表（可选，默认「消息」）")
	cmd.Flags().String("text", "", "消息内容（可替代位置参数）")
	cmd.Flags().String("content", "", "--text 的别名")
	cmd.Flags().String("body", "", "--text 的别名")
	cmd.Flags().String("message", "", "--text 的别名")
	cmd.Flags().String("markdown", "", "--text 的别名")
	_ = cmd.Flags().MarkHidden("text")
	_ = cmd.Flags().MarkHidden("content")
	_ = cmd.Flags().MarkHidden("body")
	_ = cmd.Flags().MarkHidden("message")
	_ = cmd.Flags().MarkHidden("markdown")
	cmd.Flags().Bool("at-all", false, "@所有人（仅群聊时生效，可选）,设置时，消息内容中一定要包含对应的占位符<@all>")
	cmd.Flags().String("at-users", "", "@指定成员的 userId 列表，逗号分隔（仅群聊时生效，可选）,设置--at-users userId1,userId2时，消息内容中一定要包含对应格式的占位符<@userId1> <@userId2>")
	addGroupAliases(cmd)
	return cmd
}

func newChatMessageSendByBotCommand(catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	groupProduct, groupTool, ok := catalog.FindTool("bot.send_robot_group_message")
	if !ok {
		return nil
	}
	userProduct, userTool, ok := catalog.FindTool("bot.batch_send_robot_msg_to_users")
	if !ok {
		return nil
	}

	cmd := &cobra.Command{
		Use:               "send-by-bot",
		Aliases:           []string{"send_robot_group_message", "batch_send_robot_msg_to_users"},
		Short:             "机器人发送消息（--group 群聊 / --users 单聊）",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonPayload := strings.TrimSpace(firstNonEmptyFlag(cmd, "json")); jsonPayload != "" {
				params, err := parseJSONObjectFlag("json", jsonPayload)
				if err != nil {
					return err
				}
				groupID := firstStringValue(params, "openConversationId", "group", "open-conversation-id")
				if groupID != "" {
					params["openConversationId"] = groupID
					return runCanonicalInvocation(cmd, groupProduct, groupTool, params, runner)
				}
				return runCanonicalInvocation(cmd, userProduct, userTool, params, runner)
			}
			robotCode := firstNonEmptyFlag(cmd, "robot-code")
			title := firstNonEmptyFlag(cmd, "title")
			text := firstNonEmptyFlag(cmd, "text", "markdown")
			if robotCode == "" || title == "" || text == "" {
				return fmt.Errorf("--robot-code, --title and --text are required")
			}
			groupID := firstNonEmptyFlag(cmd, "group", "conversation-id", "id", "chat", "open-conversation-id")
			users := splitCSV(firstNonEmptyFlag(cmd, "users"))
			if groupID != "" && len(users) > 0 {
				return fmt.Errorf("--group and --users are mutually exclusive")
			}
			if groupID == "" && len(users) == 0 {
				return fmt.Errorf("--group or --users is required")
			}
			if groupID != "" {
				params := map[string]any{
					"robotCode":          robotCode,
					"openConversationId": groupID,
					"title":              title,
					"markdown":           text,
				}
				return runCanonicalInvocation(cmd, groupProduct, groupTool, params, runner)
			}
			params := map[string]any{
				"robotCode": robotCode,
				"userIds":   users,
				"title":     title,
				"markdown":  text,
			}
			return runCanonicalInvocation(cmd, userProduct, userTool, params, runner)
		},
	}
	cmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	cmd.Flags().String("group", "", "群聊 openConversationId（群聊时必填）")
	cmd.Flags().String("users", "", "用户 userId 列表，逗号分隔，最多20个（单聊时必填）")
	cmd.Flags().String("title", "", "消息标题 (必填)")
	cmd.Flags().String("text", "", "消息内容 Markdown (必填)")
	cmd.Flags().String("markdown", "", "--text 的别名")
	_ = cmd.Flags().MarkHidden("markdown")
	cmd.Flags().String("json", "", "JSON object payload")
	_ = cmd.Flags().MarkHidden("json")
	addGroupAliases(cmd)
	return cmd
}

func newChatMessageRecallByBotCommand(catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	groupProduct, groupTool, ok := catalog.FindTool("bot.recall_robot_group_message")
	if !ok {
		return nil
	}
	userProduct, userTool, ok := catalog.FindTool("bot.batch_recall_robot_users_msg")
	if !ok {
		return nil
	}

	cmd := &cobra.Command{
		Use:               "recall-by-bot",
		Aliases:           []string{"recall_robot_group_message", "batch_recall_robot_users_msg"},
		Short:             "机器人撤回消息（--group 群聊 / 不传为单聊）",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonPayload := strings.TrimSpace(firstNonEmptyFlag(cmd, "json")); jsonPayload != "" {
				params, err := parseJSONObjectFlag("json", jsonPayload)
				if err != nil {
					return err
				}
				groupID := firstStringValue(params, "openConversationId", "group", "open-conversation-id")
				if groupID != "" {
					params["openConversationId"] = groupID
					return runCanonicalInvocation(cmd, groupProduct, groupTool, params, runner)
				}
				return runCanonicalInvocation(cmd, userProduct, userTool, params, runner)
			}
			robotCode := firstNonEmptyFlag(cmd, "robot-code")
			keys := splitCSV(firstNonEmptyFlag(cmd, "keys", "process-query-keys"))
			if robotCode == "" || len(keys) == 0 {
				return fmt.Errorf("--robot-code and --keys are required")
			}
			groupID := firstNonEmptyFlag(cmd, "group", "conversation-id", "id", "chat", "open-conversation-id")
			if groupID != "" {
				params := map[string]any{
					"robotCode":          robotCode,
					"openConversationId": groupID,
					"processQueryKeys":   keys,
				}
				return runCanonicalInvocation(cmd, groupProduct, groupTool, params, runner)
			}
			params := map[string]any{
				"robotCode":        robotCode,
				"processQueryKeys": keys,
			}
			return runCanonicalInvocation(cmd, userProduct, userTool, params, runner)
		},
	}
	cmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	cmd.Flags().String("group", "", "群聊 openConversationId（群聊撤回时必填）")
	cmd.Flags().String("keys", "", "消息 processQueryKey 列表，逗号分隔 (必填)")
	cmd.Flags().String("process-query-keys", "", "--keys 的别名")
	_ = cmd.Flags().MarkHidden("process-query-keys")
	cmd.Flags().String("json", "", "JSON object payload")
	_ = cmd.Flags().MarkHidden("json")
	addGroupAliases(cmd)
	return cmd
}

func newChatMessageSendByWebhookCommand(catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	product, tool, ok := catalog.FindTool("bot.send_message_by_custom_robot")
	if !ok {
		return nil
	}

	cmd := &cobra.Command{
		Use:               "send-by-webhook",
		Aliases:           []string{"send_message_by_custom_robot"},
		Short:             "自定义机器人 Webhook 发送群消息",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonPayload := strings.TrimSpace(firstNonEmptyFlag(cmd, "json")); jsonPayload != "" {
				params, err := parseJSONObjectFlag("json", jsonPayload)
				if err != nil {
					return err
				}
				return runCanonicalInvocation(cmd, product, tool, params, runner)
			}
			token := firstNonEmptyFlag(cmd, "token", "robot-token")
			title := firstNonEmptyFlag(cmd, "title")
			text := firstNonEmptyFlag(cmd, "text")
			if token == "" || title == "" || text == "" {
				return fmt.Errorf("--token, --title and --text are required")
			}
			params := map[string]any{
				"robotToken": token,
				"title":      title,
				"text":       text,
			}
			atAll := firstTrueBoolFlag(cmd, "at-all", "is-at-all")
			if atAll {
				params["isAtAll"] = true
			}
			if mobiles := splitCSV(firstNonEmptyFlag(cmd, "at-mobiles")); len(mobiles) > 0 {
				params["atMobiles"] = mobiles
			}
			if users := splitCSV(firstNonEmptyFlag(cmd, "at-users", "at-user-ids")); len(users) > 0 {
				params["atUserIds"] = users
			}
			return runCanonicalInvocation(cmd, product, tool, params, runner)
		},
	}
	cmd.Flags().String("token", "", "Webhook Token (必填)")
	cmd.Flags().String("title", "", "消息标题 (必填)")
	cmd.Flags().String("text", "", "消息内容 (必填)")
	cmd.Flags().String("robot-token", "", "--token 的别名")
	_ = cmd.Flags().MarkHidden("robot-token")
	cmd.Flags().String("json", "", "JSON object payload")
	_ = cmd.Flags().MarkHidden("json")
	cmd.Flags().Bool("at-all", false, "@ 所有人")
	cmd.Flags().Bool("is-at-all", false, "--at-all 的别名")
	_ = cmd.Flags().MarkHidden("is-at-all")
	cmd.Flags().String("at-mobiles", "", "@ 指定手机号，逗号分隔")
	cmd.Flags().String("at-users", "", "@ 指定用户，逗号分隔")
	cmd.Flags().String("at-user-ids", "", "--at-users 的别名")
	_ = cmd.Flags().MarkHidden("at-user-ids")
	return cmd
}

func newChatMessageListTopicRepliesCommand(catalog ir.Catalog, runner executor.Runner) *cobra.Command {
	product, tool, ok := catalog.FindTool("group-chat.list_topic_replies")
	if !ok {
		return nil
	}

	cmd := &cobra.Command{
		Use:               "list-topic-replies",
		Short:             "拉取群话题回复消息列表",
		Hidden:            true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := firstNonEmptyFlag(cmd, "group", "conversation-id", "id", "chat", "open-conversation-id")
			topicID := firstNonEmptyFlag(cmd, "topic-id")
			if groupID == "" || topicID == "" {
				return fmt.Errorf("--group and --topic-id are required")
			}
			params := map[string]any{
				"openconversationId": groupID,
				"topicId":            topicID,
			}
			if timeValue := firstNonEmptyFlag(cmd, "time"); timeValue != "" {
				params["startTime"] = timeValue
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				params["pageSize"] = limit
			}
			forward, _ := cmd.Flags().GetBool("forward")
			params["forward"] = forward
			return runCanonicalInvocation(cmd, product, tool, params, runner)
		},
	}
	cmd.Flags().String("group", "", "群会话 openconversationId (必填)")
	_ = cmd.MarkFlagRequired("group")
	cmd.Flags().String("topic-id", "", "话题 ID，由 dws chat message list 返回 (必填)")
	_ = cmd.MarkFlagRequired("topic-id")
	cmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）")
	cmd.Flags().Int("limit", 50, "返回数量（默认 50）")
	cmd.Flags().Bool("forward", false, "true=从老往新，false=从新往老（默认 false）")
	addGroupAliases(cmd)
	return cmd
}

func runCanonicalInvocation(cmd *cobra.Command, product ir.CanonicalProduct, tool ir.ToolDescriptor, params map[string]any, runner executor.Runner) error {
	if err := cli.ValidateInputSchema(params, tool.InputSchema); err != nil {
		return err
	}
	invocation := executor.NewInvocation(product, tool, params)
	if cmd.Flags().Lookup("dry-run") != nil {
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		invocation.DryRun = dryRun
	}
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return err
	}
	return output.WriteCommandPayload(cmd, result, output.FormatTable)
}

func addGroupAliases(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.Flags().String("conversation-id", "", "--group 的别名")
	_ = cmd.Flags().MarkHidden("conversation-id")
	if cmd.Flags().Lookup("id") == nil {
		cmd.Flags().String("id", "", "--group 的别名")
		_ = cmd.Flags().MarkHidden("id")
	}
	if cmd.Flags().Lookup("open-conversation-id") == nil {
		cmd.Flags().String("open-conversation-id", "", "--group 的别名")
		_ = cmd.Flags().MarkHidden("open-conversation-id")
	}
	cmd.Flags().String("chat", "", "--group 的别名")
	_ = cmd.Flags().MarkHidden("chat")
}

func splitCSV(raw string) []any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmptyFlag(cmd *cobra.Command, names ...string) string {
	for _, name := range names {
		if cmd == nil {
			return ""
		}
		if cmd.Flags().Lookup(name) == nil {
			continue
		}
		value, err := cmd.Flags().GetString(name)
		if err == nil && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstTrueBoolFlag(cmd *cobra.Command, names ...string) bool {
	for _, name := range names {
		if cmd == nil || cmd.Flags().Lookup(name) == nil {
			continue
		}
		value, err := cmd.Flags().GetBool(name)
		if err == nil && value {
			return true
		}
	}
	return false
}

func parseJSONObjectFlag(label string, raw string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON for --%s: %v", label, err)
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	return payload, nil
}

func firstStringValue(params map[string]any, keys ...string) string {
	for _, key := range keys {
		value := params[key]
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func hideChatMessageRawCommands(message *cobra.Command) {
	for _, name := range []string{
		"send_direct_message_as_user",
		"list_individual_chat_message",
		"send_robot_group_message",
		"batch_send_robot_msg_to_users",
		"recall_robot_group_message",
		"batch_recall_robot_users_msg",
		"send_message_by_custom_robot",
		"list_topic_replies",
	} {
		if child := childByName(message, name); child != nil {
			child.Hidden = true
		}
	}
}

func pruneChatMessageRawCommands(message *cobra.Command) {
	if message == nil {
		return
	}
	for _, name := range []string{
		"send_direct_message_as_user",
		"list_individual_chat_message",
		"send_robot_group_message",
		"batch_send_robot_msg_to_users",
		"recall_robot_group_message",
		"batch_recall_robot_users_msg",
		"send_message_by_custom_robot",
	} {
		if child := childByName(message, name); child != nil {
			message.RemoveCommand(child)
		}
	}
}

func replaceChildCommand(parent *cobra.Command, candidate *cobra.Command) {
	if parent == nil || candidate == nil {
		return
	}
	if existing := childByName(parent, candidate.Name()); existing != nil {
		parent.RemoveCommand(existing)
	}
	parent.AddCommand(candidate)
}

func newGroupCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:               use,
		Short:             short,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

func childByName(parent *cobra.Command, name string) *cobra.Command {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}
