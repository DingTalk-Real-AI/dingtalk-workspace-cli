// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/spf13/cobra"
)

func runChatThreadCreate(cmd *cobra.Command, toolArgs map[string]any) error {
	requestedMembers, _ := toolArgs["groupMembers"].([]string)
	if deps.Caller.DryRun() {
		members := append([]string{"<current-user-id>"}, requestedMembers...)
		toolArgs["groupMembers"] = members
		return storeChatThreadDryRun(cmd, "im", "create_group_conversation", toolArgs)
	}

	currentUserID, err := getCurrentUserID(cmd.Context())
	if err != nil {
		return err
	}
	seen := map[string]bool{currentUserID: true}
	members := []string{currentUserID}
	for _, uid := range requestedMembers {
		if !seen[uid] {
			seen[uid] = true
			members = append(members, uid)
		}
	}

	toolArgs["groupMembers"] = members
	raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "im", "create_group_conversation", toolArgs)
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return chatThreadResponseValidationError("im/create_group_conversation", err)
	}
	normalizeChatGroupCreateResponse(resp)
	return output.StoreResult(cmd.Context(), output.Success(resp))
}

func storeChatThreadDryRun(cmd *cobra.Command, product, tool string, arguments map[string]any) error {
	return output.StoreResult(cmd.Context(), output.Success(map[string]any{
		"dry_run":   true,
		"executed":  false,
		"product":   product,
		"tool":      tool,
		"arguments": arguments,
	}, output.WithDryRun()))
}

func newChatThreadCommand(sendRunE func(*cobra.Command, []string) error) *cobra.Command {
	thread := newGroupCommand(&cobra.Command{
		Use:   "thread",
		Short: "群聊话题（Thread）管理",
		Long:  "Thread 表示群聊中的一条话题主消息及其回复。以下命令同时适用于普通群和话题圈中的 Thread；只有 create-group 用于新建话题圈。promote 将普通群中的已有消息升级为 Thread 根消息；发布新话题时传父群 openConversationId，直接回复时传该 Thread 的 openConvThreadId。list 浏览话题主消息，list-replies 读取某个话题的逐条回复。",
		RunE:  groupRunE,
	})

	create := newChatThreadCreateCommand()
	send := newChatThreadSendCommand("send", "conversation-id", "父会话 openConversationId", sendRunE)
	reply := newChatThreadSendCommand("reply", "conversation-id", "Thread 子会话 openConvThreadId", sendRunE)
	list := newChatThreadListCommand()
	listReplies := newChatThreadListRepliesCommand()
	forward := newChatThreadForwardCommand()
	thread.AddCommand(
		create, send, newChatThreadPromoteCommand(), list, reply, listReplies, forward,
		newChatThreadRecallMessageCommand(),
		newChatThreadAddEmojiCommand(),
		newChatThreadRemoveEmojiCommand(),
		newChatThreadListEmotionRepliesCommand(),
		newChatThreadAddTextEmotionCommand(),
		newChatThreadRemoveTextEmotionCommand(),
		newChatThreadUpdateTextEmotionCommand(),
	)
	return thread
}

func newChatThreadPromoteCommand() *cobra.Command {
	const tool = "convert_message_to_thread"
	return NewLeafCommand(LeafSpec{
		Use:           "promote",
		Short:         "将普通群中的已有消息升级为 Thread",
		Long:          "将普通群中的一条已有消息升级为 Thread 根消息。--conversation-id 与 --message-id 必须属于同一个普通群；单聊消息不支持。转换成功后返回 openConvThreadId，并在群内产生 Thread 转换事件；重复请求保持幂等。",
		Example:       `  dws chat thread promote --conversation-id <openConversationId> --message-id <openMessageId>`,
		OutputRollout: output.RolloutUnifiedActive,
		Server:        "im",
		Tool:          tool,
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "消息所属普通群的 openConversationId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "待升级消息的 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMessageId"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "promote_message_to_thread", CanonicalPath: "chat.promote_message_to_thread", CLIPath: "chat thread promote", PrimaryCLIPath: "chat thread promote"},
			Description: "将普通群中的已有消息升级为 Thread 根消息",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "im", RPCName: tool}},
			Selection: contract.SelectionSpec{
				AgentSummary: "将普通群中的已有消息升级为 Thread 根消息",
				UseWhen:      []string{"用户明确要求把普通群里一条已存在的消息转成 Thread 或群内话题时"},
				AvoidWhen:    []string{"发布全新的 Thread 使用 chat thread send；回复已有 Thread 使用 chat thread reply；单聊消息不能升级"},
				Examples:     []string{"dws chat thread promote --conversation-id <openConversationId> --message-id <openMessageId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(true), InterfaceType: "string"},
				{Name: "message-id", Property: "openMessageId", Required: boolPtr(true), InterfaceType: "string"},
			},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"普通群消息升级为 Thread 的结果","properties":{"openConversationId":{"type":"string","description":"Thread 所属普通群的 openConversationId"},"openMessageId":{"type":"string","description":"升级为 Thread 根消息的 openMessageId"},"openConvThreadId":{"type":"string","description":"转换后生成的 openConvThreadId"}},"required":["openConversationId","openMessageId","openConvThreadId"],"additionalProperties":false}`),
			},
		},
		ResultCall: callChatThreadPromoteResult,
	})
}

func callChatThreadPromoteResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if deps.Caller.DryRun() {
		return output.Success(map[string]any{
			"tool": tool, "arguments": args, "executed": false,
		}, output.WithDryRun()), nil
	}
	payload, err := CallMCPToolDataOnServer(cmd.Context(), "im", tool, args)
	if err != nil {
		return nil, err
	}
	body, ok := payload.(map[string]any)
	if !ok {
		return nil, chatThreadResponseValidationError("im/"+tool, fmt.Errorf("响应不是 JSON 对象"))
	}
	result, ok := body["result"].(map[string]any)
	if !ok {
		return nil, chatThreadResponseValidationError("im/"+tool, fmt.Errorf("响应缺少 result 对象"))
	}
	for _, field := range []string{"openConversationId", "openMessageId", "openConvThreadId"} {
		value, ok := result[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, chatThreadResponseValidationError("im/"+tool, fmt.Errorf("result.%s 不是非空字符串", field))
		}
		result[field] = strings.TrimSpace(value)
	}
	for _, field := range []string{"openConversationId", "openMessageId"} {
		if result[field] != strings.TrimSpace(fmt.Sprint(args[field])) {
			return nil, chatThreadResponseValidationError("im/"+tool, fmt.Errorf("result.%s 与请求不一致", field))
		}
	}
	return output.Success(result), nil
}

func newChatThreadCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "create-group",
		Short:         "创建话题圈",
		Long:          "创建一个开启 Thread 模式的群聊（话题圈）。当前登录用户会自动加入，--users 中的重复成员会去重。已有普通群或话题圈中的 Thread 可直接使用 send、list、reply 等命令。",
		Example:       `  dws chat thread create-group --name "项目话题圈" --users userId1,userId2`,
		OutputRollout: output.RolloutUnifiedActive,
		Tool:          "create_group_conversation",
		Flags: []LeafFlag{
			{Name: "name", Usage: "话题圈名称 (必填)", Required: true, MarkRequired: true, Bind: "groupName"},
			{Name: "users", Usage: "成员 userId 或 openDingTalkId，逗号分隔 (必填)", Required: true, MarkRequired: true, Bind: "groupMembers", Transform: func(raw string) (any, error) {
				return parseCSVValues(raw), nil
			}},
			{Name: "type", Usage: "话题圈类型: INTERNAL/EXTERNAL/NORMAL", Default: "INTERNAL", Bind: "groupType", Transform: func(raw string) (any, error) {
				groupType := strings.ToUpper(raw)
				switch groupType {
				case "INTERNAL", "EXTERNAL", "NORMAL":
					return groupType, nil
				default:
					return nil, fmt.Errorf("invalid --type %q, supported: INTERNAL, EXTERNAL, NORMAL", groupType)
				}
			}},
		},
		ConstParams: map[string]any{"convThreadEnabled": true},
		Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "create_thread_group", CanonicalPath: "chat.create_thread_group", CLIPath: "chat thread create-group", PrimaryCLIPath: "chat thread create-group"},
			Description: "创建开启 Thread 模式的群聊",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "im", RPCName: "create_group_conversation"}},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建开启话题模式的群聊容器",
				UseWhen:      []string{"用户明确要新建话题圈并已提供名称和成员时"},
				AvoidWhen:    []string{"创建普通群聊时使用 chat group create"},
				Examples:     []string{"dws chat thread create-group --name \"项目话题圈\" --users userId1,userId2"},
			},
			Parameters: []contract.ParamDecl{{Name: "name", Property: "groupName"}, {Name: "type", Property: "groupType"}, {Name: "users", Property: "groupMembers"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"话题圈创建响应","properties":{"result":{"type":"object","description":"创建结果","additionalProperties":true}},"additionalProperties":true}`),
			},
		},
		Call: func(cmd *cobra.Command, _ string, toolArgs map[string]any) error {
			return runChatThreadCreate(cmd, toolArgs)
		},
	})
}

func newChatThreadSendCommand(use, targetFlag, targetDescription string, sendRunE func(*cobra.Command, []string) error) *cobra.Command {
	description := "发布一条新话题"
	longDescription := "在指定父群会话中发布一条新话题，普通群和话题圈均可使用。--conversation-id 传父群 openConversationId。"
	identityName := "send_thread"
	canonical := "chat.send_thread"
	if use == "reply" {
		description = "向指定 openConvThreadId 直接追加回复（非引用回复）"
		longDescription = "向已有 Thread 直接追加回复（非引用回复），普通群和话题圈均可使用。--conversation-id 传该 Thread 的 openConvThreadId，而不是父群 ID。"
		identityName = "reply_thread"
		canonical = "chat.reply_thread"
	}
	cmd := &cobra.Command{
		Use:     use,
		Short:   description,
		Long:    longDescription + "支持文本或 Markdown、已有 mediaId 图片，以及本地 file/audio/video；发送后立即返回 openTaskId，不在命令内轮询状态。",
		Example: fmt.Sprintf("  dws chat thread %s --%s <%s> --content \"内容\"", use, targetFlag, targetFlag),
		Args:    cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Flags().Set("conversation-id", mustGetFlag(cmd, targetFlag))
		},
		RunE: sendRunE,
	}
	registerChatThreadSendFlags(cmd, targetFlag, targetDescription)
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: identityName, CanonicalPath: canonical, CLIPath: "chat thread " + use, PrimaryCLIPath: "chat thread " + use},
			Description: description,
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "chat", RPCName: "send_personal_message"}},
			Selection: contract.SelectionSpec{
				AgentSummary: description,
				UseWhen:      []string{description + "，并沿用异步 openTaskId 发送契约时"},
				AvoidWhen:    []string{"不创建 Thread 的普通群聊或单聊消息使用 chat message send；引用回复普通消息使用 chat message reply"},
				Examples:     []string{fmt.Sprintf("dws chat thread %s --%s <%s> --content \"内容\"", use, targetFlag, targetFlag)},
			},
			Parameters: []contract.ParamDecl{
				{Name: targetFlag, Property: "openConversationId", Required: boolPtr(true)},
				{Name: "ai-tag", Property: "clawType", InterfaceType: "string"},
				{Name: "at-all", Property: "atAll", InterfaceType: "boolean"},
				{Name: "at-open-dingtalk-ids", Property: "atOpenDingTalkIds", InterfaceType: "array"},
				{Name: "idempotency-key", Property: "uuid"},
			},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending},
				DataSchema: json.RawMessage(`{"type":"object","description":"话题发送受理结果","properties":{"result":{"type":"object","description":"异步发送任务","properties":{"openTaskId":{"type":"string","description":"用于查询发送状态的任务 ID"}},"additionalProperties":true}},"additionalProperties":true}`),
			},
		},
	})
	cli.AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{Name: "content", Type: "string", Description: "消息内容（也可使用 --content）", Required: false, Index: 0})
	cli.AnnotateRuntimeFlagEnum(cmd, "msg-type", "image", "file", "audio", "video")
	cli.AnnotateRuntimeFlagFormat(cmd, "file", "file-path")
	return cmd
}

func registerChatThreadSendFlags(cmd *cobra.Command, targetFlag, targetDescription string) {
	cmd.Flags().String(targetFlag, "", targetDescription+" (必填)")
	_ = cmd.MarkFlagRequired(targetFlag)
	if targetFlag != "conversation-id" {
		cmd.Flags().String("conversation-id", "", "内部目标映射")
		_ = cmd.Flags().MarkHidden("conversation-id")
	}
	corecmd.RegisterFlags(cmd, []corecmd.FlagSpec{{
		Name:    "content",
		Usage:   "消息内容（推荐方式，也可用位置参数传递。内容含换行/特殊字符时必须使用此 flag）",
		Aliases: []string{"text"},
	}})
	for _, alias := range []string{"body", "message", "markdown"} {
		cmd.Flags().String(alias, "", "--content 的兼容别名")
		_ = cmd.Flags().MarkHidden(alias)
	}
	cmd.Flags().String("title", "", "消息标题")
	cmd.Flags().Bool("at-all", false, "@所有人（仅群聊时生效，可选）,设置时，消息内容中一定要包含对应的占位符<@all>")
	cmd.Flags().String("at-open-dingtalk-ids", "", "@指定成员的 openDingTalkId 列表，逗号分隔（仅群聊时生效，可选）,设置--at-open-dingtalk-ids openDingTalkId1,openDingTalkId2时，消息内容中一定要包含对应格式的占位符<@openDingTalkId1> <@openDingTalkId2>")
	cmd.Flags().String("media-id", "", "已有图片 mediaId")
	cmd.Flags().String("msg-type", "", "内容类型: image/file/audio/video")
	corecmd.RegisterFlags(cmd, []corecmd.FlagSpec{{
		Name:    "file",
		Usage:   "本地文件路径（msgType=file/audio/video 时直接上传并按 file 消息发送）",
		Aliases: []string{"file-path"},
	}})
	cmd.Flags().Int64("dentry-id", 0, "文件 dentryId（与 --space-id 成对传入时跳过自动上传）")
	cmd.Flags().Int64("space-id", 0, "空间 ID（与 --dentry-id 成对传入时跳过自动上传）")
	cmd.Flags().String("file-name", "", "文件名")
	cmd.Flags().String("file-type", "", "文件类型/扩展名")
	cmd.Flags().Int64("file-size", 0, "文件大小，单位字节")
	for _, name := range []string{"dentry-id", "space-id", "file-name", "file-type", "file-size"} {
		_ = cmd.Flags().MarkHidden(name)
	}
	cmd.Flags().Bool("ai-tag", true, "消息是否带 AI 发送角标（默认 true）")
	corecmd.RegisterFlags(cmd, []corecmd.FlagSpec{{Name: "idempotency-key", Usage: "幂等键，相同 key 在 24h 内不会重复发送", Aliases: []string{"uuid"}}})
}

func newChatThreadListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "分页列出群聊中的话题主消息",
		Long:    "分页读取普通群或话题圈中的 Thread 主消息，用于浏览话题列表或概览，不返回某个 Thread 的逐条回复。--conversation-id 传父群 openConversationId；每次返回一页，续页状态通过统一结果的 meta.pagination 返回。如需读取具体回复，使用 chat thread list-replies。",
		Example: `  dws chat thread list --conversation-id <openConversationId> --limit 50`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			conversationID := mustGetFlag(cmd, "conversation-id")
			timeValue := mustGetFlag(cmd, "time")
			defaultForward := true
			if timeValue == "" {
				timeValue = defaultChatMessageListTime()
				defaultForward = false
			}
			forward, err := resolveMessageForward(cmd, defaultForward)
			if err != nil {
				return err
			}
			args := map[string]any{"openconversation_id": conversationID, "time": timeValue, "forward": forward}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				args["limit"] = limit
			}
			if deps.Caller.DryRun() {
				return storeChatThreadDryRun(cmd, "chat", "list_conversation_message_v2", args)
			}
			raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "chat", "list_conversation_message_v2", args)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if err := unmarshalJSONUseNumber(raw, &data); err != nil {
				return chatThreadResponseValidationError("chat/list_conversation_message_v2", err)
			}
			items := chatmsg.ListMessageItems(data)
			payload := projectChatThreadsPayload(items, conversationID)
			meta, err := chatThreadPaginationMeta("chat/list_conversation_message_v2", data, items, payload["count"].(int), messageDirection(forward))
			if err != nil {
				return err
			}
			return output.StoreResult(cmd.Context(), output.Success(payload, output.WithMeta(meta)))
		},
	}
	cmd.Flags().String("conversation-id", "", "会话 openConversationId (必填)")
	_ = cmd.MarkFlagRequired("conversation-id")
	cmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss（可选，默认上海时间当前时间）")
	cmd.Flags().Int("limit", 0, "返回数量，不传则不限制")
	cmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（未传 --time 时默认 older）")
	cmd.Flags().String("forward", "false", "兼容方向参数")
	_ = cmd.Flags().MarkHidden("forward")
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "list_threads", CanonicalPath: "chat.list_threads", CLIPath: "chat thread list", PrimaryCLIPath: "chat thread list"},
			Description: "分页读取会话中的话题主消息",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "读取 list_conversation_message_v2 后只投影包含 openConvThreadId 的话题主消息。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "分页读取会话中的话题主消息",
				UseWhen:      []string{"已知会话 ID 并需要浏览其中的话题主消息时"},
				AvoidWhen: []string{
					"读取不区分话题的会话消息时使用 chat message list",
					"需要逐条查看某个 Thread 的回复正文时使用 chat thread list-replies",
				},
				Examples: []string{"dws chat thread list --conversation-id <openConversationId> --limit 50"},
			},
			Parameters: []contract.ParamDecl{{Name: "conversation-id", Property: "openconversation_id"}, {Name: "time", Property: "time"}, {Name: "direction", Property: "forward"}, {Name: "limit", Property: "limit"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"群会话中的一页 Thread 主消息","properties":{"topics":{"type":"array","description":"包含 openConvThreadId 的 Thread 主消息","items":{"type":"object","description":"Thread 主消息","additionalProperties":true}},"count":{"type":"integer","description":"当前页 Thread 数量"}},"required":["topics","count"],"additionalProperties":true}`),
			},
			Pagination: chatThreadCursorPagination(),
		},
	})
	return cmd
}

func projectChatThreadsPayload(items []map[string]any, conversationID string) map[string]any {
	topics := make([]map[string]any, 0)
	for _, item := range items {
		threadID := strings.TrimSpace(fmt.Sprint(chatmsg.ThreadID(item)))
		if threadID == "" || threadID == "<nil>" {
			continue
		}
		row := chatmsg.ProjectMessageV1(item, true)
		row["conversationId"] = conversationID
		row["openConvThreadId"] = threadID
		delete(row, "threadId")
		topics = append(topics, row)
	}
	return map[string]any{"topics": topics, "count": len(topics)}
}

func newChatThreadListRepliesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-replies",
		Short:   "分页读取指定 Thread 的回复",
		Long:    "分页读取一个 Thread 的逐条回复，普通群和话题圈均可使用。--conversation-id 传父群 openConversationId，--topic-id 传该 Thread 的 openConvThreadId。每次返回一页；如需自动读取全部页面、排序或下载资源，可使用 chat +thread-replies --page-all。",
		Example: `  dws chat thread list-replies --conversation-id <openConversationId> --topic-id <openConvThreadId>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			forward, err := resolveMessageForward(cmd, false)
			if err != nil {
				return err
			}
			args := map[string]any{
				"openconversationId": mustGetFlag(cmd, "conversation-id"),
				"topicId":            mustGetFlag(cmd, "topic-id"),
				"forward":            forward,
			}
			if value := mustGetFlag(cmd, "time"); value != "" {
				args["startTime"] = value
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				args["pageSize"] = limit
			}
			if deps.Caller.DryRun() {
				return storeChatThreadDryRun(cmd, "chat", "list_topic_replies", args)
			}
			raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "chat", "list_topic_replies", args)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if err := unmarshalJSONUseNumber(raw, &data); err != nil {
				return chatThreadResponseValidationError("chat/list_topic_replies", err)
			}
			items := chatmsg.ListMessageItems(data)
			payload := projectAtomicThreadRepliesPayload(
				items,
				mustGetFlag(cmd, "conversation-id"),
				mustGetFlag(cmd, "topic-id"),
			)
			meta, err := chatThreadPaginationMeta("chat/list_topic_replies", data, items, len(items), messageDirection(forward))
			if err != nil {
				return err
			}
			return output.StoreResult(cmd.Context(), output.Success(payload, output.WithMeta(meta)))
		},
	}
	cmd.Flags().String("conversation-id", "", "父会话 openConversationId (必填)")
	_ = cmd.MarkFlagRequired("conversation-id")
	cmd.Flags().String("topic-id", "", "Thread openConvThreadId (必填)")
	_ = cmd.MarkFlagRequired("topic-id")
	cmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）")
	cmd.Flags().Int("limit", 50, "每页返回数量")
	cmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐，默认 older）")
	cmd.Flags().String("forward", "false", "兼容方向参数")
	_ = cmd.Flags().MarkHidden("forward")
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "list_topic_replies", CanonicalPath: "chat.list_topic_replies", CLIPath: "chat thread list-replies", PrimaryCLIPath: "chat thread list-replies"},
			Description: "分页读取指定 openConvThreadId 的回复",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "chat", RPCName: "list_topic_replies"}},
			Selection: contract.SelectionSpec{
				AgentSummary: "分页读取指定 Thread 的回复",
				UseWhen: []string{
					"已知父会话 ID 与 openConvThreadId 并需要查看回复内容时",
					"需要逐条列出某个话题当前存在的回复或核实具体回复是否仍存在时",
				},
				AvoidWhen: []string{"只浏览 Thread 主消息而不读取回复时使用 chat thread list"},
				Examples:  []string{"dws chat thread list-replies --conversation-id <openConversationId> --topic-id <openConvThreadId>"},
			},
			Parameters: []contract.ParamDecl{{Name: "conversation-id", Property: "openconversationId"}, {Name: "topic-id", Property: "topicId"}, {Name: "time", Property: "startTime"}, {Name: "direction", Property: "forward"}, {Name: "limit", Property: "pageSize"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"指定话题的一页回复","properties":{"openConversationId":{"type":"string","description":"父会话 openConversationId"},"openConvThreadId":{"type":"string","description":"话题 openConvThreadId"},"replies":{"type":"array","description":"当前页回复","items":{"type":"object","description":"话题回复","additionalProperties":true}},"count":{"type":"integer","description":"当前页回复数量"}},"required":["openConversationId","openConvThreadId","replies","count"],"additionalProperties":true}`),
			},
			Pagination: chatThreadCursorPagination(),
		},
	})
	return cmd
}

func projectAtomicThreadRepliesPayload(items []map[string]any, conversationID, openConvThreadID string) map[string]any {
	replies := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := chatmsg.ProjectMessageV1(item, true)
		row["conversationId"] = conversationID
		row["openConvThreadId"] = openConvThreadID
		delete(row, "threadId")
		replies = append(replies, row)
	}
	return map[string]any{
		"openConversationId": conversationID,
		"openConvThreadId":   openConvThreadID,
		"replies":            replies,
		"count":              len(replies),
	}
}

func validateThreadMessages(cmd *cobra.Command, threadConversationID string, messageIDs []string) (map[string]string, error) {
	if deps.Caller.DryRun() {
		return nil, nil
	}
	raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "im", "list_messages_by_ids", map[string]any{"openMsgIds": messageIDs})
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if err := unmarshalJSONUseNumber(raw, &data); err != nil {
		return nil, chatThreadResponseValidationError("im/list_messages_by_ids", err)
	}
	items := chatmsg.ListMessageItems(data)
	byID := make(map[string]map[string]any, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(fmt.Sprint(chatmsg.MessageID(item))); id != "" && id != "<nil>" {
			byID[id] = item
		}
	}
	parents := make(map[string]string, len(messageIDs))
	for _, messageID := range messageIDs {
		message := byID[messageID]
		if message == nil {
			return nil, apperrors.NewValidation(
				fmt.Sprintf("未找到消息 %s，无法确认 Thread 归属", messageID),
				apperrors.WithReason("thread_message_not_found"),
			)
		}
		parentConversationID := strings.TrimSpace(fmt.Sprint(chatmsg.ConversationID(message)))
		if parentConversationID != "" && parentConversationID != "<nil>" {
			parents[messageID] = parentConversationID
		}
		threadID := strings.TrimSpace(fmt.Sprint(chatmsg.ThreadID(message)))
		if threadID != "" && threadID != "<nil>" {
			continue
		}
		if threadConversationID != "" && parentConversationID != "" && parentConversationID != "<nil>" {
			found := false
			if threadConversationID != parentConversationID {
				found, err = threadReplyContainsMessage(cmd, parentConversationID, threadConversationID, messageID)
			} else {
				found, err = messageAppearsInConversationThread(cmd, parentConversationID, messageID)
			}
			if err != nil {
				return nil, err
			}
			if found {
				continue
			}
		}
		return nil, apperrors.NewValidation(
			fmt.Sprintf("消息 %s 不属于 Thread", messageID),
			apperrors.WithReason("message_not_in_thread"),
		)
	}
	return parents, nil
}

func messageAppearsInConversationThread(cmd *cobra.Command, parentConversationID, messageID string) (bool, error) {
	args := map[string]any{
		"openconversation_id": parentConversationID,
		"time":                defaultChatMessageListTime(),
		"forward":             false,
		"limit":               100,
	}
	seenBoundaries := map[string]bool{}
	checkedThreads := 0
	for page := 0; page < 100; page++ {
		raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "chat", "list_conversation_message_v2", args)
		if err != nil {
			return false, err
		}
		data := map[string]any{}
		if err := unmarshalJSONUseNumber(raw, &data); err != nil {
			return false, chatThreadResponseValidationError("chat/list_conversation_message_v2", err)
		}
		items := chatmsg.ListMessageItems(data)
		for _, item := range items {
			threadID := strings.TrimSpace(fmt.Sprint(chatmsg.ThreadID(item)))
			if threadID == "" || threadID == "<nil>" {
				continue
			}
			checkedThreads++
			if checkedThreads > 100 {
				return false, chatThreadResponseValidationError("chat/list_conversation_message_v2", fmt.Errorf("Thread 归属校验超过 100 个话题安全上限"))
			}
			found, err := threadReplyContainsMessage(cmd, parentConversationID, threadID, messageID)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		}
		pagination := map[string]any{}
		chatmsg.ApplyMessagePagination(pagination, data, items, "older")
		known, _ := pagination["paginationKnown"].(bool)
		hasMore, _ := pagination["hasMore"].(bool)
		if !known {
			return false, chatThreadResponseValidationError("chat/list_conversation_message_v2", fmt.Errorf("话题分页状态不完整"))
		}
		if !hasMore {
			return false, nil
		}
		nextPage, _ := pagination["nextPage"].(map[string]any)
		boundary := strings.TrimSpace(fmt.Sprint(nextPage["time"]))
		if boundary == "" || boundary == "<nil>" || seenBoundaries[boundary] {
			return false, chatThreadResponseValidationError("chat/list_conversation_message_v2", fmt.Errorf("话题分页游标未前进"))
		}
		seenBoundaries[boundary] = true
		args["time"] = boundary
	}
	return false, chatThreadResponseValidationError("chat/list_conversation_message_v2", fmt.Errorf("话题分页超过 100 页安全上限"))
}

func threadReplyContainsMessage(cmd *cobra.Command, parentConversationID, threadConversationID, messageID string) (bool, error) {
	args := map[string]any{
		"openconversationId": parentConversationID,
		"topicId":            threadConversationID,
		"forward":            false,
		"pageSize":           100,
	}
	seenBoundaries := map[string]bool{}
	for page := 0; page < 100; page++ {
		raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "chat", "list_topic_replies", args)
		if err != nil {
			return false, err
		}
		data := map[string]any{}
		if err := unmarshalJSONUseNumber(raw, &data); err != nil {
			return false, chatThreadResponseValidationError("chat/list_topic_replies", err)
		}
		items := chatmsg.ListMessageItems(data)
		for _, item := range items {
			if strings.TrimSpace(fmt.Sprint(chatmsg.MessageID(item))) == messageID {
				return true, nil
			}
		}
		pagination := map[string]any{}
		chatmsg.ApplyMessagePagination(pagination, data, items, "older")
		known, _ := pagination["paginationKnown"].(bool)
		hasMore, _ := pagination["hasMore"].(bool)
		if !known {
			return false, chatThreadResponseValidationError("chat/list_topic_replies", fmt.Errorf("回复分页状态不完整"))
		}
		if !hasMore {
			return false, nil
		}
		nextPage, _ := pagination["nextPage"].(map[string]any)
		boundary := strings.TrimSpace(fmt.Sprint(nextPage["time"]))
		if boundary == "" || boundary == "<nil>" || seenBoundaries[boundary] {
			return false, chatThreadResponseValidationError("chat/list_topic_replies", fmt.Errorf("回复分页游标未前进"))
		}
		seenBoundaries[boundary] = true
		args["startTime"] = boundary
	}
	return false, chatThreadResponseValidationError("chat/list_topic_replies", fmt.Errorf("回复分页超过 100 页安全上限"))
}

func newChatThreadRecallMessageCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "recall-message",
		Short:   "撤回 Thread 中的一条消息",
		Long:    "撤回当前用户在 Thread 中发送的一条主消息或回复。命令会先校验 Thread 归属；--conversation-id 可传父群 openConversationId，处理回复时也可传该 Thread 的 openConvThreadId。",
		Example: `  dws chat thread recall-message --conversation-id <openConversationId> --message-id <openMessageId>`,
		Server:  "im",
		Tool:    "recall_message",
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "父群 openConversationId；处理回复时也可传 Thread openConvThreadId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "消息 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMessageId"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_recall_message", CanonicalPath: "chat.thread_recall_message", CLIPath: "chat thread recall-message", PrimaryCLIPath: "chat thread recall-message"},
			Description: "撤回 Thread 中当前用户发送的一条消息",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先用 im/list_messages_by_ids 校验 Thread 归属，再调用 im/recall_message。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "撤回 Thread 中当前用户发送的一条消息",
				UseWhen:      []string{"已知 Thread 内消息及其会话 ID，需要只撤回这一条消息时"},
				AvoidWhen:    []string{"撤回普通消息使用 chat message recall；转发或关闭整条 Thread 时不要使用"},
				Examples:     []string{"dws chat thread recall-message --conversation-id <openConversationId> --message-id <openMessageId>"},
			},
			Parameters: []contract.ParamDecl{{Name: "conversation-id", Property: "openConversationId"}, {Name: "message-id", Property: "openMessageId"}},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			messageID := fmt.Sprint(args["openMessageId"])
			parents, err := validateThreadMessages(cmd, fmt.Sprint(args["openConversationId"]), []string{messageID})
			if err != nil {
				return err
			}
			if parent := parents[messageID]; parent != "" && parent != "<nil>" {
				args["openConversationId"] = parent
			}
			return callMCPToolOnServer("im", "recall_message", args)
		},
	})
}

func newChatThreadAddEmojiCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "add-emoji",
		Short:   "给 Thread 消息添加 emoji",
		Long:    "为 Thread 主消息或回复添加 emoji reaction，普通群和话题圈均可使用。命令会先校验消息的 Thread 归属；文字状态请使用 add-text-emotion。",
		Example: `  dws chat thread add-emoji --conversation-id <openConversationId> --message-id <openMessageId> --emoji "赞"`,
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "父群 openConversationId；处理回复时也可传 Thread openConvThreadId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "消息 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMsgId"},
			{Name: "emoji", Usage: "emoji 表情名称 (必填)", Required: true, MarkRequired: true, Bind: "emojiName"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_add_emoji", CanonicalPath: "chat.thread_add_emoji", CLIPath: "chat thread add-emoji", PrimaryCLIPath: "chat thread add-emoji"},
			Description: "给 Thread 主消息或回复添加 emoji reaction",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先校验 Thread 归属，再调用 im/add_emoji_reaction。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "给 Thread 主消息或回复添加 emoji reaction",
				UseWhen:      []string{"需要对 Thread 中一条已知消息点赞、赞同或确认时"},
				AvoidWhen:    []string{"普通消息 reaction 使用 chat message add-emoji；文字状态使用 chat thread add-text-emotion"},
				Examples:     []string{"dws chat thread add-emoji --conversation-id <openConversationId> --message-id <openMessageId> --emoji \"赞\""},
			},
			Parameters: []contract.ParamDecl{{Name: "conversation-id", Property: "openConversationId"}, {Name: "message-id", Property: "openMsgId"}, {Name: "emoji", Property: "emojiName"}},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			messageID := fmt.Sprint(args["openMsgId"])
			parents, err := validateThreadMessages(cmd, fmt.Sprint(args["openConversationId"]), []string{messageID})
			if err != nil {
				return err
			}
			if parent := parents[messageID]; parent != "" && parent != "<nil>" {
				args["openConversationId"] = parent
			}
			return callMCPToolOnServer("im", "add_emoji_reaction", args)
		},
	})
}

func newChatThreadRemoveEmojiCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "remove-emoji",
		Short:   "移除 Thread 消息上的 emoji",
		Long:    "移除当前用户在 Thread 主消息或回复上添加的 emoji reaction，普通群和话题圈均可使用。命令会先校验消息的 Thread 归属。",
		Example: `  dws chat thread remove-emoji --conversation-id <openConversationId> --message-id <openMessageId> --emoji "赞"`,
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "父群 openConversationId；处理回复时也可传 Thread openConvThreadId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "消息 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMsgId"},
			{Name: "emoji", Usage: "emoji 表情名称 (必填)", Required: true, MarkRequired: true, Bind: "emojiName"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_remove_emoji", CanonicalPath: "chat.thread_remove_emoji", CLIPath: "chat thread remove-emoji", PrimaryCLIPath: "chat thread remove-emoji"},
			Description: "移除 Thread 主消息或回复上的 emoji reaction",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先校验 Thread 归属，再调用 im/remove_emoji_reaction。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "移除 Thread 主消息或回复上的 emoji reaction",
				UseWhen:      []string{"需要取消自己在 Thread 消息上添加的 emoji 时"},
				AvoidWhen:    []string{"普通消息 reaction 使用 chat message remove-emoji；文字状态使用 chat thread remove-text-emotion"},
				Examples:     []string{"dws chat thread remove-emoji --conversation-id <openConversationId> --message-id <openMessageId> --emoji \"赞\""},
			},
			Parameters: []contract.ParamDecl{{Name: "conversation-id", Property: "openConversationId"}, {Name: "message-id", Property: "openMsgId"}, {Name: "emoji", Property: "emojiName"}},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			messageID := fmt.Sprint(args["openMsgId"])
			parents, err := validateThreadMessages(cmd, fmt.Sprint(args["openConversationId"]), []string{messageID})
			if err != nil {
				return err
			}
			if parent := parents[messageID]; parent != "" && parent != "<nil>" {
				args["openConversationId"] = parent
			}
			return callMCPToolOnServer("im", "remove_emoji_reaction", args)
		},
	})
}

func newChatThreadListEmotionRepliesCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "list-emotion-replies",
		Short:   "查询 Thread 消息的 emoji 与文字状态",
		Long:    "查询一批 Thread 主消息或回复上的 emoji reaction 与文字表情（状态）。命令会先逐条校验 Thread 归属；如需读取回复正文，使用 chat thread list-replies。",
		Example: `  dws chat thread list-emotion-replies --msg-ids msgId1,msgId2`,
		Flags: []LeafFlag{{Name: "msg-ids", Usage: "消息 ID 列表，逗号分隔 (必填)", Required: true, MarkRequired: true, Bind: "openMessageIds", Transform: func(raw string) (any, error) {
			return parseCSVValues(raw), nil
		}}},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_list_emotion_replies", CanonicalPath: "chat.thread_list_emotion_replies", CLIPath: "chat thread list-emotion-replies", PrimaryCLIPath: "chat thread list-emotion-replies"},
			Description: "批量查询 Thread 消息的 emoji 和文字表情回复",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先批量校验 Thread 归属，再调用 im/list_message_emotion_replies。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量查询 Thread 消息的 emoji 和文字表情回复",
				UseWhen:      []string{"已有一批 Thread 消息 ID，需要查看点赞人、表情统计或文字状态时"},
				AvoidWhen:    []string{"读取 Thread 正文与回复流时使用 chat thread list-replies"},
				Examples:     []string{"dws chat thread list-emotion-replies --msg-ids msgId1,msgId2"},
			},
			Parameters: []contract.ParamDecl{{Name: "msg-ids", Property: "openMessageIds", InterfaceType: "array"}},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			values, _ := args["openMessageIds"].([]string)
			if _, err := validateThreadMessages(cmd, "", values); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "list_message_emotion_replies", args)
		},
	})
}

func newChatThreadAddTextEmotionCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "add-text-emotion",
		Short:   "给 Thread 消息添加文字表情或状态",
		Long:    "给 Thread 主消息或回复添加已有的文字表情（可用作“处理中”等状态）。emotion-id、background-id、名称和文字使用 chat message create-text-emotion 返回的实际值；命令会先校验 Thread 归属。",
		Example: `  dws chat thread add-text-emotion --conversation-id <openConversationId> --message-id <openMessageId> --emotion-id <emotionId> --emotion-name "处理中" --text "处理中" --background-id im_bg_5`,
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "父群 openConversationId；处理回复时也可传 Thread openConvThreadId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "消息 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMsgId"},
			{Name: "emotion-id", Usage: "create-text-emotion 返回的 emotionId (必填)", Required: true, MarkRequired: true, Bind: "emotionId"},
			{Name: "emotion-name", Usage: "create-text-emotion 返回的表情名称 (必填)", Required: true, MarkRequired: true, Bind: "emotionName"},
			{Name: "text", Usage: "create-text-emotion 返回的文字内容 (必填)", Required: true, MarkRequired: true, Bind: "text"},
			{Name: "background-id", Usage: "create-text-emotion 返回的 backgroundId (必填)", Required: true, MarkRequired: true, Bind: "backgroundId"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_add_text_emotion", CanonicalPath: "chat.thread_add_text_emotion", CLIPath: "chat thread add-text-emotion", PrimaryCLIPath: "chat thread add-text-emotion"},
			Description: "给 Thread 主消息或回复添加文字表情",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先校验 Thread 归属，再调用 im/add_text_emotion。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "给 Thread 主消息或回复添加文字表情",
				UseWhen:      []string{"需要给 Thread 消息添加处理中、已解决或感谢等已有文字状态时"},
				AvoidWhen:    []string{"尚未创建文字表情资源时先使用 chat message create-text-emotion；普通 emoji 使用 chat thread add-emoji"},
				Examples:     []string{"dws chat thread add-text-emotion --conversation-id <openConversationId> --message-id <openMessageId> --emotion-id <emotionId> --emotion-name \"处理中\" --text \"处理中\" --background-id im_bg_5"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId"}, {Name: "message-id", Property: "openMsgId"},
				{Name: "emotion-id", Property: "emotionId"}, {Name: "emotion-name", Property: "emotionName"},
				{Name: "text", Property: "text"}, {Name: "background-id", Property: "backgroundId"},
			},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			messageID := fmt.Sprint(args["openMsgId"])
			parents, err := validateThreadMessages(cmd, fmt.Sprint(args["openConversationId"]), []string{messageID})
			if err != nil {
				return err
			}
			if parent := parents[messageID]; parent != "" && parent != "<nil>" {
				args["openConversationId"] = parent
			}
			return callMCPToolOnServer("im", "add_text_emotion", args)
		},
	})
}

func newChatThreadRemoveTextEmotionCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "remove-text-emotion",
		Short:   "移除 Thread 消息上的文字表情或状态",
		Long:    "移除 Thread 主消息或回复上已添加的文字表情（状态）。emotion-id、background-id、名称和文字须使用添加时的实际值；命令会先校验 Thread 归属。",
		Example: `  dws chat thread remove-text-emotion --conversation-id <openConversationId> --message-id <openMessageId> --emotion-id <emotionId> --emotion-name "处理中" --text "处理中" --background-id im_bg_5`,
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "父群 openConversationId；处理回复时也可传 Thread openConvThreadId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "消息 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMsgId"},
			{Name: "emotion-id", Usage: "已添加文字表情的 emotionId (必填)", Required: true, MarkRequired: true, Bind: "emotionId"},
			{Name: "emotion-name", Usage: "已添加文字表情的名称 (必填)", Required: true, MarkRequired: true, Bind: "emotionName"},
			{Name: "text", Usage: "已添加文字表情的文字内容 (必填)", Required: true, MarkRequired: true, Bind: "text"},
			{Name: "background-id", Usage: "已添加文字表情的 backgroundId (必填)", Required: true, MarkRequired: true, Bind: "backgroundId"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_remove_text_emotion", CanonicalPath: "chat.thread_remove_text_emotion", CLIPath: "chat thread remove-text-emotion", PrimaryCLIPath: "chat thread remove-text-emotion"},
			Description: "移除 Thread 主消息或回复上的文字表情",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先校验 Thread 归属，再调用 im/remove_text_emotion。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "移除 Thread 主消息或回复上的文字表情",
				UseWhen:      []string{"需要清除 Thread 消息上一个已知文字状态时"},
				AvoidWhen:    []string{"更新为另一个状态使用 chat thread update-text-emotion；普通 emoji 使用 chat thread remove-emoji"},
				Examples:     []string{"dws chat thread remove-text-emotion --conversation-id <openConversationId> --message-id <openMessageId> --emotion-id <emotionId> --emotion-name \"处理中\" --text \"处理中\" --background-id im_bg_5"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId"}, {Name: "message-id", Property: "openMsgId"},
				{Name: "emotion-id", Property: "emotionId"}, {Name: "emotion-name", Property: "emotionName"},
				{Name: "text", Property: "text"}, {Name: "background-id", Property: "backgroundId"},
			},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			messageID := fmt.Sprint(args["openMsgId"])
			parents, err := validateThreadMessages(cmd, fmt.Sprint(args["openConversationId"]), []string{messageID})
			if err != nil {
				return err
			}
			if parent := parents[messageID]; parent != "" && parent != "<nil>" {
				args["openConversationId"] = parent
			}
			return callMCPToolOnServer("im", "remove_text_emotion", args)
		},
	})
}

func newChatThreadUpdateTextEmotionCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:     "update-text-emotion",
		Short:   "更新 Thread 消息上的文字表情或状态",
		Long:    "把 Thread 主消息或回复上的已有文字表情（状态）原子替换为新值。old-emotion-id 传当前 emotionId，其余表情字段传 chat message create-text-emotion 返回的新值；命令会先校验 Thread 归属。",
		Example: `  dws chat thread update-text-emotion --conversation-id <openConversationId> --message-id <openMessageId> --old-emotion-id <oldEmotionId> --emotion-id <emotionId> --emotion-name "已完成" --text "已完成" --background-id im_bg_5`,
		Flags: []LeafFlag{
			{Name: "conversation-id", Usage: "父群 openConversationId；处理回复时也可传 Thread openConvThreadId (必填)", Required: true, MarkRequired: true, Bind: "openConversationId"},
			{Name: "message-id", Usage: "消息 openMessageId (必填)", Required: true, MarkRequired: true, Bind: "openMsgId"},
			{Name: "old-emotion-id", Usage: "当前文字表情的 emotionId (必填)", Required: true, MarkRequired: true, Bind: "oldEmotionId"},
			{Name: "emotion-id", Usage: "新文字表情的 emotionId (必填)", Required: true, MarkRequired: true, Bind: "emotionId"},
			{Name: "emotion-name", Usage: "新文字表情的名称 (必填)", Required: true, MarkRequired: true, Bind: "emotionName"},
			{Name: "text", Usage: "新文字表情的文字内容 (必填)", Required: true, MarkRequired: true, Bind: "text"},
			{Name: "background-id", Usage: "新文字表情的 backgroundId (必填)", Required: true, MarkRequired: true, Bind: "backgroundId"},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "thread_update_text_emotion", CanonicalPath: "chat.thread_update_text_emotion", CLIPath: "chat thread update-text-emotion", PrimaryCLIPath: "chat thread update-text-emotion"},
			Description: "原子替换 Thread 主消息或回复上的文字表情",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "先校验 Thread 归属，再调用 im/update_text_emotion。"},
			Selection: contract.SelectionSpec{
				AgentSummary: "原子替换 Thread 主消息或回复上的文字表情",
				UseWhen:      []string{"需要把 Thread 消息上的处理中等状态更新为已完成等新状态时"},
				AvoidWhen:    []string{"消息尚无文字表情时使用 chat thread add-text-emotion；只清除时使用 chat thread remove-text-emotion"},
				Examples:     []string{"dws chat thread update-text-emotion --conversation-id <openConversationId> --message-id <openMessageId> --old-emotion-id <oldEmotionId> --emotion-id <emotionId> --emotion-name \"已完成\" --text \"已完成\" --background-id im_bg_5"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "conversation-id", Property: "openConversationId"}, {Name: "message-id", Property: "openMsgId"},
				{Name: "old-emotion-id", Property: "oldEmotionId"}, {Name: "emotion-id", Property: "emotionId"},
				{Name: "emotion-name", Property: "emotionName"}, {Name: "text", Property: "text"},
				{Name: "background-id", Property: "backgroundId"},
			},
		},
		Call: func(cmd *cobra.Command, _ string, args map[string]any) error {
			messageID := fmt.Sprint(args["openMsgId"])
			parents, err := validateThreadMessages(cmd, fmt.Sprint(args["openConversationId"]), []string{messageID})
			if err != nil {
				return err
			}
			if parent := parents[messageID]; parent != "" && parent != "<nil>" {
				args["openConversationId"] = parent
			}
			return callMCPToolOnServer("im", "update_text_emotion", args)
		},
	})
}

func messageDirection(forward bool) string {
	if forward {
		return "newer"
	}
	return "older"
}

func chatThreadCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "time",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func chatThreadPaginationMeta(operation string, data map[string]any, sourceItems []map[string]any, businessCount int, direction string) (*output.Meta, error) {
	normalizeChatThreadPaginationNumbers(data)
	projection := map[string]any{}
	chatmsg.ApplyMessagePagination(projection, data, sourceItems, direction)
	known, _ := projection["paginationKnown"].(bool)
	hasMore, hasMoreKnown := projection["hasMore"].(bool)
	if !known || !hasMoreKnown {
		return nil, chatThreadPaginationError(operation, "下层响应缺少可靠的分页终态或续页游标")
	}
	nextToken := ""
	if hasMore {
		nextPage, _ := projection["nextPage"].(map[string]any)
		nextToken, _ = nextPage["time"].(string)
	}
	pagination, err := output.NewPagination(!hasMore, nextToken)
	if err != nil {
		return nil, chatThreadPaginationError(operation, "下层响应无法生成可执行的下一页时间边界")
	}
	pagination.Pages = 1
	pagination.Items = businessCount
	return &output.Meta{Count: output.NewCount(businessCount), Pagination: pagination}, nil
}

func normalizeChatThreadPaginationNumbers(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if number, ok := child.(json.Number); ok {
				if parsed, err := number.Int64(); err == nil {
					typed[key] = parsed
				}
				continue
			}
			normalizeChatThreadPaginationNumbers(child)
		}
	case []any:
		for _, child := range typed {
			normalizeChatThreadPaginationNumbers(child)
		}
	}
}

func chatThreadPaginationError(operation, message string) error {
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation(operation),
		apperrors.WithReason("invalid_pagination"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
	)
}

func chatThreadResponseValidationError(operation string, cause error) error {
	return apperrors.NewAPI(
		fmt.Sprintf("解析 %s 返回失败: %v", operation, cause),
		apperrors.WithOperation(operation),
		apperrors.WithReason("thread_response_invalid"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithCause(cause),
	)
}

func newChatThreadForwardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "forward",
		Short:   "转发 Thread 到目标会话",
		Long:    "使用源 Thread 主消息、父群 openConversationId、openConvThreadId 和目标会话转发整条 Thread 并保留上下文。当前不支持从话题圈向另一个话题圈转发整条 Thread；可转发到普通群。",
		Example: `  dws chat thread forward --src-msg-id <messageId> --src-conversation-id <openConversationId> --src-thread-id <openConvThreadId> --dest-conversation-id <openConversationId>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			args := map[string]any{
				"srcOpenMessageId":       mustGetFlag(cmd, "src-msg-id"),
				"srcOpenConversationId":  mustGetFlag(cmd, "src-conversation-id"),
				"srcOpenConvThreadId":    mustGetFlag(cmd, "src-thread-id"),
				"destOpenConversationId": mustGetFlag(cmd, "dest-conversation-id"),
			}
			if deps.Caller.DryRun() {
				return storeChatThreadDryRun(cmd, "im", "forward_topic", args)
			}
			raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "im", "forward_topic", args)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if err := unmarshalJSONUseNumber(raw, &data); err != nil {
				return chatThreadResponseValidationError("im/forward_topic", err)
			}
			data["source"] = map[string]any{
				"messageId":          mustGetFlag(cmd, "src-msg-id"),
				"openConversationId": mustGetFlag(cmd, "src-conversation-id"),
				"openConvThreadId":   mustGetFlag(cmd, "src-thread-id"),
			}
			data["destinationOpenConversationId"] = mustGetFlag(cmd, "dest-conversation-id")
			return output.StoreResult(cmd.Context(), output.Success(data))
		},
	}
	for _, flag := range []struct{ name, usage string }{
		{"src-msg-id", "源话题主消息 messageId"},
		{"src-conversation-id", "源会话 openConversationId"},
		{"src-thread-id", "源 Thread openConvThreadId"},
		{"dest-conversation-id", "目标会话 openConversationId"},
	} {
		cmd.Flags().String(flag.name, "", flag.usage+" (必填)")
		_ = cmd.MarkFlagRequired(flag.name)
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "forward_topic", CanonicalPath: "chat.forward_topic", CLIPath: "chat thread forward", PrimaryCLIPath: "chat thread forward"},
			Description: "把一条话题转发到目标会话",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "im", RPCName: "forward_topic"}},
			Selection:   contract.SelectionSpec{AgentSummary: "把一条 Thread 转发到目标会话", UseWhen: []string{"需要保留 Thread 上下文转发到另一个会话时"}, AvoidWhen: []string{"普通单条消息转发使用 chat message forward"}, Examples: []string{"dws chat thread forward --src-msg-id <messageId> --src-conversation-id <openConversationId> --src-thread-id <openConvThreadId> --dest-conversation-id <openConversationId>"}},
			Parameters:  []contract.ParamDecl{{Name: "src-msg-id", Property: "srcOpenMessageId"}, {Name: "src-conversation-id", Property: "srcOpenConversationId"}, {Name: "src-thread-id", Property: "srcOpenConvThreadId"}, {Name: "dest-conversation-id", Property: "destOpenConversationId"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"话题转发结果","properties":{"source":{"type":"object","description":"源话题标识","properties":{"messageId":{"type":"string","description":"源话题主消息 ID"},"openConversationId":{"type":"string","description":"源会话 openConversationId"},"openConvThreadId":{"type":"string","description":"源话题 openConvThreadId"}},"required":["messageId","openConversationId","openConvThreadId"],"additionalProperties":false},"destinationOpenConversationId":{"type":"string","description":"目标会话 openConversationId"}},"required":["source","destinationOpenConversationId"],"additionalProperties":true}`),
			},
		},
	})
	return cmd
}

func topicQuoteReplyDisabledError() error {
	return apperrors.NewValidation(
		"话题圈不支持引用消息回复；请使用 chat thread reply 向 openConvThreadId 直接追加回复",
		apperrors.WithReason("topic_quote_reply_disabled"),
		apperrors.WithHint("使用 dws chat thread reply --conversation-id <openConvThreadId> --content <content>"),
	)
}

func guardTopicQuoteReply(cmd *cobra.Command, openConversationID, openMessageID string) error {
	if deps.Caller.DryRun() {
		return nil
	}
	raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "im", "list_messages_by_ids", map[string]any{
		"openMsgIds": []string{openMessageID},
	})
	if err != nil {
		return topicQuoteGuardUnavailable("im/list_messages_by_ids", "读取被引用消息失败，无法确认其是否属于话题圈，已阻止发送")
	}
	messageData := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &messageData); err != nil {
		return topicQuoteGuardUnavailable("im/list_messages_by_ids", "被引用消息响应无法解析，已阻止发送")
	}
	if err := validateTopicQuoteMessage(messageData, openConversationID, openMessageID); err != nil {
		return topicQuoteGuardUnavailable("im/list_messages_by_ids", "无法确认被引用消息的会话与话题归属，已阻止发送")
	}
	raw, err = callMCPToolReturnTextOnServer(cmd.Context(), "chat", "get_conversation_info", map[string]any{
		"openConversationId": openConversationID,
	})
	if err != nil {
		return topicQuoteGuardUnavailable("chat/get_conversation_info", "无法确认引用回复目标是否属于话题圈，已阻止发送")
	}
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return topicQuoteGuardUnavailable("chat/get_conversation_info", "会话信息响应无法解析，已阻止发送")
	}
	inspection := inspectTopicContainerState(data, openConversationID)
	switch inspection.state {
	case topicContainerTopic:
		return topicQuoteReplyDisabledError()
	case topicContainerUnknown:
		if !inspection.needsChannelLookup {
			return topicQuoteGuardUnavailable("chat/get_conversation_info", "会话信息无法确认有效的话题圈标识，已阻止发送")
		}
		raw, err = callMCPToolReturnTextOnServer(cmd.Context(), "im", "search_groups", map[string]any{
			"keyword": inspection.conversationTitle,
			"limit":   100,
			"cursor":  "0",
		})
		if err != nil {
			return topicQuoteGuardUnavailable("im/search_groups", "无法读取会话类型，已阻止发送")
		}
		var searchData any
		if err := json.Unmarshal([]byte(raw), &searchData); err != nil {
			return topicQuoteGuardUnavailable("im/search_groups", "会话类型响应无法解析，已阻止发送")
		}
		switch detectTopicChannelState(searchData, openConversationID, inspection.conversationTitle) {
		case topicContainerTopic:
			return topicQuoteReplyDisabledError()
		case topicContainerUnknown:
			return topicQuoteGuardUnavailable("im/search_groups", "群搜索无法确认同一会话的 channel 标识，已阻止发送")
		}
	}
	return nil
}

func validateTopicQuoteMessage(data map[string]any, openConversationID, openMessageID string) error {
	wantConversationID := strings.TrimSpace(openConversationID)
	for _, message := range chatmsg.ListMessageItems(data) {
		if chatmsg.StableMessageID(message) != strings.TrimSpace(openMessageID) {
			continue
		}
		messageConversationID := strings.TrimSpace(fmt.Sprint(chatmsg.ConversationID(message)))
		if messageConversationID == "" || messageConversationID == "<nil>" {
			return fmt.Errorf("message %s did not include an openConversationId", openMessageID)
		}
		if messageConversationID != wantConversationID {
			return fmt.Errorf("message %s belongs to conversation %s, not %s", openMessageID, messageConversationID, wantConversationID)
		}
		return nil
	}
	return fmt.Errorf("message %s was not returned", openMessageID)
}

type topicContainerState uint8

const (
	topicContainerUnknown topicContainerState = iota
	topicContainerNonTopic
	topicContainerTopic
)

type topicContainerInspection struct {
	state              topicContainerState
	conversationTitle  string
	needsChannelLookup bool
}

func detectTopicContainerState(value any, openConversationID string) topicContainerState {
	return inspectTopicContainerState(value, openConversationID).state
}

func inspectTopicContainerState(value any, openConversationID string) topicContainerInspection {
	envelope, ok := value.(map[string]any)
	if !ok {
		return topicContainerInspection{state: topicContainerUnknown}
	}
	success, ok := envelope["success"].(bool)
	if !ok || !success {
		return topicContainerInspection{state: topicContainerUnknown}
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		return topicContainerInspection{state: topicContainerUnknown}
	}
	conversation, ok := result["conversationInfo"].(map[string]any)
	if !ok {
		return topicContainerInspection{state: topicContainerUnknown}
	}
	wantConversationID := strings.TrimSpace(openConversationID)
	conversationID, ok := conversation["openConversationId"].(string)
	if !ok || strings.TrimSpace(conversationID) != wantConversationID {
		return topicContainerInspection{state: topicContainerUnknown}
	}

	sawTrue := false
	sawFalse := false
	sawInvalid := false
	for _, key := range []string{"convThreadEnabled", "topicGroup", "isTopicGroup"} {
		raw, present := conversation[key]
		if !present {
			continue
		}
		switch enabled := raw.(type) {
		case bool:
			if enabled {
				sawTrue = true
			} else {
				sawFalse = true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(enabled)) {
			case "true", "1":
				sawTrue = true
			case "false", "0":
				sawFalse = true
			default:
				sawInvalid = true
			}
		default:
			sawInvalid = true
		}
	}
	if sawInvalid || (sawTrue && sawFalse) {
		return topicContainerInspection{state: topicContainerUnknown}
	}
	if sawTrue {
		return topicContainerInspection{state: topicContainerTopic}
	}
	if sawFalse {
		return topicContainerInspection{state: topicContainerNonTopic}
	}
	title, ok := conversation["title"].(string)
	title = strings.TrimSpace(title)
	if !ok || title == "" {
		return topicContainerInspection{state: topicContainerUnknown}
	}
	// Missing topic-only fields is not positive ordinary-group evidence. The
	// caller must bind this exact conversation to search_groups.channel before
	// allowing a write.
	return topicContainerInspection{
		state:              topicContainerUnknown,
		conversationTitle:  title,
		needsChannelLookup: true,
	}
}

func detectTopicChannelState(value any, openConversationID, conversationTitle string) topicContainerState {
	envelope, ok := value.(map[string]any)
	if !ok {
		return topicContainerUnknown
	}
	success, ok := envelope["success"].(bool)
	if !ok || !success {
		return topicContainerUnknown
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		return topicContainerUnknown
	}
	groups, ok := result["groups"].([]any)
	if !ok {
		return topicContainerUnknown
	}

	wantConversationID := strings.TrimSpace(openConversationID)
	wantTitle := strings.TrimSpace(conversationTitle)
	sawTopic := false
	sawNonTopic := false
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		conversationID, ok := group["openConversationId"].(string)
		if !ok || strings.TrimSpace(conversationID) != wantConversationID {
			continue
		}
		title, ok := group["title"].(string)
		if !ok || strings.TrimSpace(title) != wantTitle {
			return topicContainerUnknown
		}
		channel, ok := group["channel"].(bool)
		if !ok {
			return topicContainerUnknown
		}
		if channel {
			sawTopic = true
		} else {
			sawNonTopic = true
		}
	}
	if sawTopic && sawNonTopic {
		return topicContainerUnknown
	}
	if sawTopic {
		return topicContainerTopic
	}
	if sawNonTopic {
		return topicContainerNonTopic
	}
	return topicContainerUnknown
}

func topicQuoteGuardUnavailable(operation, message string) error {
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation(operation),
		apperrors.WithReason("topic_quote_guard_unavailable"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithRetryable(true),
		apperrors.WithHint("确认消息与会话信息可读取后重试；Thread 回复请使用 chat thread reply"),
	)
}
