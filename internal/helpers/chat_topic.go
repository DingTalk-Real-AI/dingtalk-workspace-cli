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

func runChatTopicCreate(cmd *cobra.Command, toolArgs map[string]any) error {
	requestedMembers, _ := toolArgs["groupMembers"].([]string)
	if deps.Caller.DryRun() {
		members := append([]string{"<current-user-id>"}, requestedMembers...)
		toolArgs["groupMembers"] = members
		return storeChatTopicDryRun(cmd, "im", "create_group_conversation", toolArgs)
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
		return chatTopicResponseValidationError("im/create_group_conversation", err)
	}
	if result, ok := resp["result"].(map[string]any); ok {
		if value, exists := result["openCid"]; exists {
			result["openTopicId"] = value
			delete(result, "openCid")
		} else {
			if value, exists := result["openConversationId"]; exists {
				result["openTopicId"] = value
				delete(result, "openConversationId")
			}
		}
		delete(result, "cid")
	}
	return output.StoreResult(cmd.Context(), output.Success(resp))
}

func storeChatTopicDryRun(cmd *cobra.Command, product, tool string, arguments map[string]any) error {
	return output.StoreResult(cmd.Context(), output.Success(map[string]any{
		"dry_run":   true,
		"executed":  false,
		"product":   product,
		"tool":      tool,
		"arguments": arguments,
	}, output.WithDryRun()))
}

func newChatTopicCommand(sendRunE func(*cobra.Command, []string) error) *cobra.Command {
	topic := &cobra.Command{
		Use:   "topic",
		Short: "话题圈与话题管理",
		Long:  "创建话题圈，发布、分页读取、直接回复和转发话题。话题圈使用 openTopicId，圈内一条具体话题使用 openConvThreadId。",
		RunE:  groupRunE,
	}

	create := newChatTopicCreateCommand()
	send := newChatTopicSendCommand("send", "open-topic-id", "话题圈 openTopicId", sendRunE)
	reply := newChatTopicSendCommand("reply", "open-conv-thread-id", "话题 openConvThreadId", sendRunE)
	list := newChatTopicListCommand()
	listReplies := newChatTopicListRepliesCommand()
	forward := newChatTopicForwardCommand()
	topic.AddCommand(create, send, list, reply, listReplies, forward)
	return topic
}

func newChatTopicCreateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "create",
		Short:         "创建话题圈",
		Long:          "创建一个话题圈，固定开启话题模式。返回结果使用 openTopicId。",
		Example:       `  dws chat topic create --name "项目话题圈" --users userId1,userId2`,
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
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "create_topic_conversation", CanonicalPath: "chat.create_topic_conversation", CLIPath: "chat topic create", PrimaryCLIPath: "chat topic create"},
			Description: "创建话题圈并返回 openTopicId",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "im", RPCName: "create_group_conversation"}},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建开启话题模式的群聊容器",
				UseWhen:      []string{"用户明确要新建话题圈并已提供名称和成员时"},
				AvoidWhen:    []string{"创建普通群聊时使用 chat group create"},
				Examples:     []string{"dws chat topic create --name \"项目话题圈\" --users userId1,userId2"},
			},
			Parameters: []contract.ParamDecl{{Name: "name", Property: "groupName"}, {Name: "type", Property: "groupType"}, {Name: "users", Property: "groupMembers"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"话题圈创建响应","properties":{"result":{"type":"object","description":"创建结果","properties":{"openTopicId":{"type":"string","description":"新话题圈 openTopicId"}},"additionalProperties":true}},"additionalProperties":true}`),
			},
		},
		Call: func(cmd *cobra.Command, _ string, toolArgs map[string]any) error {
			return runChatTopicCreate(cmd, toolArgs)
		},
	})
}

func newChatTopicSendCommand(use, targetFlag, targetDescription string, sendRunE func(*cobra.Command, []string) error) *cobra.Command {
	description := "在话题圈发布一条话题"
	identityName := "send_topic_message"
	canonical := "chat.send_topic_message"
	if use == "reply" {
		description = "向指定 openConvThreadId 直接追加回复（非引用回复）"
		identityName = "reply_topic"
		canonical = "chat.reply_topic"
	}
	cmd := &cobra.Command{
		Use:     use,
		Short:   description,
		Long:    description + "。支持文本或 Markdown、已有 mediaId 图片，以及本地 file/audio/video；发送后立即返回 openTaskId，不在命令内轮询状态。",
		Example: fmt.Sprintf("  dws chat topic %s --%s <%s> --content \"内容\"", use, targetFlag, targetFlag),
		Args:    cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Flags().Set("conversation-id", mustGetFlag(cmd, targetFlag))
		},
		RunE: sendRunE,
	}
	registerChatTopicSendFlags(cmd, targetFlag, targetDescription)
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: identityName, CanonicalPath: canonical, CLIPath: "chat topic " + use, PrimaryCLIPath: "chat topic " + use},
			Description: description,
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "chat", RPCName: "send_personal_message"}},
			Selection: contract.SelectionSpec{
				AgentSummary: description,
				UseWhen:      []string{description + "，并沿用异步 openTaskId 发送契约时"},
				AvoidWhen:    []string{"普通群聊或单聊消息使用 chat message send；引用回复普通消息使用 chat message reply"},
				Examples:     []string{fmt.Sprintf("dws chat topic %s --%s <%s> --content \"内容\"", use, targetFlag, targetFlag)},
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

func registerChatTopicSendFlags(cmd *cobra.Command, targetFlag, targetDescription string) {
	cmd.Flags().String(targetFlag, "", targetDescription+" (必填)")
	_ = cmd.MarkFlagRequired(targetFlag)
	cmd.Flags().String("conversation-id", "", "内部目标映射")
	_ = cmd.Flags().MarkHidden("conversation-id")
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

func newChatTopicListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "列出话题圈中的话题",
		Long:    "分页读取指定 openTopicId 的会话消息，每次返回一页，并只保留包含 openConvThreadId 的话题主消息。续页状态通过统一结果的 meta.pagination 返回。",
		Example: `  dws chat topic list --open-topic-id <openTopicId> --limit 50`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			openTopicID := mustGetFlag(cmd, "open-topic-id")
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
			args := map[string]any{"openconversation_id": openTopicID, "time": timeValue, "forward": forward}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				args["limit"] = limit
			}
			if deps.Caller.DryRun() {
				return storeChatTopicDryRun(cmd, "chat", "list_conversation_message_v2", args)
			}
			raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "chat", "list_conversation_message_v2", args)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if err := unmarshalJSONUseNumber(raw, &data); err != nil {
				return chatTopicResponseValidationError("chat/list_conversation_message_v2", err)
			}
			items := chatmsg.ListMessageItems(data)
			payload := projectChatTopicsPayload(items, openTopicID)
			meta, err := chatTopicPaginationMeta("chat/list_conversation_message_v2", data, items, payload["count"].(int), messageDirection(forward))
			if err != nil {
				return err
			}
			return output.StoreResult(cmd.Context(), output.Success(payload, output.WithMeta(meta)))
		},
	}
	cmd.Flags().String("open-topic-id", "", "话题圈 openTopicId (必填)")
	_ = cmd.MarkFlagRequired("open-topic-id")
	cmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss（可选，默认上海时间当前时间）")
	cmd.Flags().Int("limit", 0, "返回数量，不传则不限制")
	cmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（未传 --time 时默认 older）")
	cmd.Flags().String("forward", "false", "兼容方向参数")
	_ = cmd.Flags().MarkHidden("forward")
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "list_topics", CanonicalPath: "chat.list_topics", CLIPath: "chat topic list", PrimaryCLIPath: "chat topic list"},
			Description: "分页读取话题圈中的话题主消息",
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "读取 list_conversation_message_v2 后只投影包含 openConvThreadId 的话题主消息。"},
			Selection:   contract.SelectionSpec{AgentSummary: "分页读取话题圈中的话题主消息", UseWhen: []string{"已知 openTopicId 并需要浏览其中的话题时"}, AvoidWhen: []string{"读取普通群聊消息时使用 chat message list"}, Examples: []string{"dws chat topic list --open-topic-id <openTopicId> --limit 50"}},
			Parameters:  []contract.ParamDecl{{Name: "open-topic-id", Property: "openconversation_id"}, {Name: "time", Property: "time"}, {Name: "direction", Property: "forward"}, {Name: "limit", Property: "limit"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"话题圈中的一页话题主消息","properties":{"topics":{"type":"array","description":"包含 openConvThreadId 的话题主消息","items":{"type":"object","description":"话题主消息","additionalProperties":true}},"count":{"type":"integer","description":"当前页话题数量"}},"required":["topics","count"],"additionalProperties":true}`),
			},
			Pagination: chatTopicCursorPagination(),
		},
	})
	return cmd
}

func projectChatTopicsPayload(items []map[string]any, openTopicID string) map[string]any {
	topics := make([]map[string]any, 0)
	for _, item := range items {
		threadID := strings.TrimSpace(fmt.Sprint(chatmsg.ThreadID(item)))
		if threadID == "" || threadID == "<nil>" {
			continue
		}
		row := chatmsg.ProjectMessageV1(item, true)
		row["openTopicId"] = openTopicID
		row["openConvThreadId"] = threadID
		delete(row, "conversationId")
		delete(row, "threadId")
		topics = append(topics, row)
	}
	return map[string]any{"topics": topics, "count": len(topics)}
}

func newChatTopicListRepliesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-replies",
		Short:   "分页读取指定话题的回复",
		Long:    "分页读取指定 openConvThreadId 的回复，每次返回一页。--open-topic-id 指定话题圈，--open-conv-thread-id 指定圈内具体话题；需要自动读取全部页面时使用现有的 chat +thread-replies --page-all。",
		Example: `  dws chat topic list-replies --open-topic-id <openTopicId> --open-conv-thread-id <openConvThreadId>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			forward, err := resolveMessageForward(cmd, false)
			if err != nil {
				return err
			}
			args := map[string]any{
				"openconversationId": mustGetFlag(cmd, "open-topic-id"),
				"topicId":            mustGetFlag(cmd, "open-conv-thread-id"),
				"forward":            forward,
			}
			if value := mustGetFlag(cmd, "time"); value != "" {
				args["startTime"] = value
			}
			if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
				args["pageSize"] = limit
			}
			if deps.Caller.DryRun() {
				return storeChatTopicDryRun(cmd, "chat", "list_topic_replies", args)
			}
			raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "chat", "list_topic_replies", args)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if err := unmarshalJSONUseNumber(raw, &data); err != nil {
				return chatTopicResponseValidationError("chat/list_topic_replies", err)
			}
			items := chatmsg.ListMessageItems(data)
			payload := projectAtomicTopicRepliesPayload(
				items,
				mustGetFlag(cmd, "open-topic-id"),
				mustGetFlag(cmd, "open-conv-thread-id"),
			)
			meta, err := chatTopicPaginationMeta("chat/list_topic_replies", data, items, len(items), messageDirection(forward))
			if err != nil {
				return err
			}
			return output.StoreResult(cmd.Context(), output.Success(payload, output.WithMeta(meta)))
		},
	}
	cmd.Flags().String("open-topic-id", "", "话题圈 openTopicId (必填)")
	_ = cmd.MarkFlagRequired("open-topic-id")
	cmd.Flags().String("open-conv-thread-id", "", "话题 openConvThreadId (必填)")
	_ = cmd.MarkFlagRequired("open-conv-thread-id")
	cmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）")
	cmd.Flags().Int("limit", 50, "每页返回数量")
	cmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐，默认 older）")
	cmd.Flags().String("forward", "false", "兼容方向参数")
	_ = cmd.Flags().MarkHidden("forward")
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "list_topic_replies", CanonicalPath: "chat.list_topic_replies", CLIPath: "chat topic list-replies", PrimaryCLIPath: "chat topic list-replies"},
			Description: "分页读取指定 openConvThreadId 的回复",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "chat", RPCName: "list_topic_replies"}},
			Selection:   contract.SelectionSpec{AgentSummary: "分页读取指定话题的回复", UseWhen: []string{"已知 openTopicId 与 openConvThreadId 并需要查看回复时"}, AvoidWhen: []string{"浏览话题主消息时使用 chat topic list"}, Examples: []string{"dws chat topic list-replies --open-topic-id <openTopicId> --open-conv-thread-id <openConvThreadId>"}},
			Parameters:  []contract.ParamDecl{{Name: "open-topic-id", Property: "openconversationId"}, {Name: "open-conv-thread-id", Property: "topicId"}, {Name: "time", Property: "startTime"}, {Name: "direction", Property: "forward"}, {Name: "limit", Property: "pageSize"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"指定话题的一页回复","properties":{"openTopicId":{"type":"string","description":"话题圈 openTopicId"},"openConvThreadId":{"type":"string","description":"圈内话题 openConvThreadId"},"replies":{"type":"array","description":"当前页回复","items":{"type":"object","description":"话题回复","additionalProperties":true}},"count":{"type":"integer","description":"当前页回复数量"}},"required":["openTopicId","openConvThreadId","replies","count"],"additionalProperties":true}`),
			},
			Pagination: chatTopicCursorPagination(),
		},
	})
	return cmd
}

func projectAtomicTopicRepliesPayload(items []map[string]any, openTopicID, openConvThreadID string) map[string]any {
	replies := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := chatmsg.ProjectMessageV1(item, true)
		row["openTopicId"] = openTopicID
		row["openConvThreadId"] = openConvThreadID
		delete(row, "conversationId")
		delete(row, "threadId")
		replies = append(replies, row)
	}
	return map[string]any{
		"openTopicId":      openTopicID,
		"openConvThreadId": openConvThreadID,
		"replies":          replies,
		"count":            len(replies),
	}
}

func messageDirection(forward bool) string {
	if forward {
		return "newer"
	}
	return "older"
}

func chatTopicCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "time",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func chatTopicPaginationMeta(operation string, data map[string]any, sourceItems []map[string]any, businessCount int, direction string) (*output.Meta, error) {
	normalizeChatTopicPaginationNumbers(data)
	projection := map[string]any{}
	chatmsg.ApplyMessagePagination(projection, data, sourceItems, direction)
	known, _ := projection["paginationKnown"].(bool)
	hasMore, hasMoreKnown := projection["hasMore"].(bool)
	failedCount, _ := projection["failedCount"].(int)
	if !known || !hasMoreKnown || failedCount != 0 {
		return nil, chatTopicPaginationError(operation, "下层响应缺少可靠的分页终态或续页游标")
	}
	nextToken := ""
	if hasMore {
		nextPage, _ := projection["nextPage"].(map[string]any)
		nextToken, _ = nextPage["time"].(string)
		if strings.TrimSpace(nextToken) == "" {
			return nil, chatTopicPaginationError(operation, "下层响应无法生成可执行的下一页时间边界")
		}
	}
	pagination, err := output.NewPagination(!hasMore, nextToken)
	if err != nil {
		return nil, chatTopicPaginationError(operation, err.Error())
	}
	pagination.Pages = 1
	pagination.Items = businessCount
	return &output.Meta{Count: output.NewCount(businessCount), Pagination: pagination}, nil
}

func normalizeChatTopicPaginationNumbers(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if number, ok := child.(json.Number); ok {
				if parsed, err := number.Int64(); err == nil {
					typed[key] = parsed
				}
				continue
			}
			normalizeChatTopicPaginationNumbers(child)
		}
	case []any:
		for _, child := range typed {
			normalizeChatTopicPaginationNumbers(child)
		}
	}
}

func chatTopicPaginationError(operation, message string) error {
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation(operation),
		apperrors.WithReason("invalid_pagination"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
	)
}

func chatTopicResponseValidationError(operation string, cause error) error {
	return apperrors.NewAPI(
		fmt.Sprintf("解析 %s 返回失败: %v", operation, cause),
		apperrors.WithOperation(operation),
		apperrors.WithReason("topic_response_invalid"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithCause(cause),
	)
}

func newChatTopicForwardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "forward",
		Short:   "转发话题到目标会话",
		Long:    "使用源话题主消息 messageId、源 openTopicId/openConvThreadId 和目标会话 openConversationId 转发整条话题并保留话题上下文。",
		Example: `  dws chat topic forward --src-msg-id <messageId> --src-open-topic-id <openTopicId> --src-open-conv-thread-id <openConvThreadId> --dest-open-conversation-id <openConversationId>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			args := map[string]any{
				"srcOpenMessageId":       mustGetFlag(cmd, "src-msg-id"),
				"srcOpenConversationId":  mustGetFlag(cmd, "src-open-topic-id"),
				"srcOpenConvThreadId":    mustGetFlag(cmd, "src-open-conv-thread-id"),
				"destOpenConversationId": mustGetFlag(cmd, "dest-open-conversation-id"),
			}
			if deps.Caller.DryRun() {
				return storeChatTopicDryRun(cmd, "im", "forward_topic", args)
			}
			raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "im", "forward_topic", args)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if err := unmarshalJSONUseNumber(raw, &data); err != nil {
				return chatTopicResponseValidationError("im/forward_topic", err)
			}
			data["source"] = map[string]any{
				"messageId":        mustGetFlag(cmd, "src-msg-id"),
				"openTopicId":      mustGetFlag(cmd, "src-open-topic-id"),
				"openConvThreadId": mustGetFlag(cmd, "src-open-conv-thread-id"),
			}
			data["destinationOpenConversationId"] = mustGetFlag(cmd, "dest-open-conversation-id")
			return output.StoreResult(cmd.Context(), output.Success(data))
		},
	}
	for _, flag := range []struct{ name, usage string }{
		{"src-msg-id", "源话题主消息 messageId"},
		{"src-open-topic-id", "源话题圈 openTopicId"},
		{"src-open-conv-thread-id", "源话题 openConvThreadId"},
		{"dest-open-conversation-id", "目标会话 openConversationId"},
	} {
		cmd.Flags().String(flag.name, "", flag.usage+" (必填)")
		_ = cmd.MarkFlagRequired(flag.name)
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "chat", Name: "forward_topic", CanonicalPath: "chat.forward_topic", CLIPath: "chat topic forward", PrimaryCLIPath: "chat topic forward"},
			Description: "把一条话题转发到目标会话",
			Interface:   &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "im", RPCName: "forward_topic"}},
			Selection:   contract.SelectionSpec{AgentSummary: "把一条话题转发到目标会话", UseWhen: []string{"需要保留话题上下文转发到另一个会话时"}, AvoidWhen: []string{"普通单条消息转发使用 chat message forward"}, Examples: []string{"dws chat topic forward --src-msg-id <messageId> --src-open-topic-id <openTopicId> --src-open-conv-thread-id <openConvThreadId> --dest-open-conversation-id <openConversationId>"}},
			Parameters:  []contract.ParamDecl{{Name: "src-msg-id", Property: "srcOpenMessageId"}, {Name: "src-open-topic-id", Property: "srcOpenConversationId"}, {Name: "src-open-conv-thread-id", Property: "srcOpenConvThreadId"}, {Name: "dest-open-conversation-id", Property: "destOpenConversationId"}},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
				DataSchema: json.RawMessage(`{"type":"object","description":"话题转发结果","properties":{"source":{"type":"object","description":"源话题标识","properties":{"messageId":{"type":"string","description":"源话题主消息 ID"},"openTopicId":{"type":"string","description":"源话题圈 openTopicId"},"openConvThreadId":{"type":"string","description":"源话题 openConvThreadId"}},"required":["messageId","openTopicId","openConvThreadId"],"additionalProperties":false},"destinationOpenConversationId":{"type":"string","description":"目标会话 openConversationId"}},"required":["source","destinationOpenConversationId"],"additionalProperties":true}`),
			},
		},
	})
	return cmd
}

func topicQuoteReplyDisabledError() error {
	return apperrors.NewValidation(
		"话题圈不支持引用消息回复；请使用 chat topic reply 向 openConvThreadId 直接追加回复",
		apperrors.WithReason("topic_quote_reply_disabled"),
		apperrors.WithHint("使用 dws chat topic reply --open-conv-thread-id <openConvThreadId> --content <content>"),
	)
}

func guardTopicQuoteReply(cmd *cobra.Command, openConversationID, openMessageID string) error {
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
	isTopicMessage, err := topicQuoteMessageState(messageData, openMessageID)
	if err != nil {
		return topicQuoteGuardUnavailable("im/list_messages_by_ids", "无法确认被引用消息的会话与话题归属，已阻止发送")
	}
	if isTopicMessage {
		return topicQuoteReplyDisabledError()
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
	switch detectTopicContainerState(data) {
	case topicContainerTopic:
		return topicQuoteReplyDisabledError()
	case topicContainerUnknown:
		return topicQuoteGuardUnavailable("chat/get_conversation_info", "会话信息未明确返回 convThreadEnabled，无法确认引用回复目标是否属于话题圈，已阻止发送")
	}
	return nil
}

func topicQuoteMessageState(data map[string]any, openMessageID string) (bool, error) {
	for _, message := range chatmsg.ListMessageItems(data) {
		if chatmsg.StableMessageID(message) != strings.TrimSpace(openMessageID) {
			continue
		}
		threadID := strings.TrimSpace(fmt.Sprint(chatmsg.ThreadID(message)))
		return threadID != "" && threadID != "<nil>", nil
	}
	return false, fmt.Errorf("message %s was not returned", openMessageID)
}

type topicContainerState uint8

const (
	topicContainerUnknown topicContainerState = iota
	topicContainerNonTopic
	topicContainerTopic
)

func detectTopicContainerState(value any) topicContainerState {
	sawFalse := false
	sawInvalid := false
	var visit func(any) bool
	visit = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if key != "convThreadEnabled" {
					if visit(child) {
						return true
					}
					continue
				}
				switch enabled := child.(type) {
				case bool:
					if enabled {
						return true
					}
					sawFalse = true
				case string:
					switch strings.ToLower(strings.TrimSpace(enabled)) {
					case "true", "1":
						return true
					case "false", "0":
						sawFalse = true
					default:
						sawInvalid = true
					}
				default:
					sawInvalid = true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	if visit(value) {
		return topicContainerTopic
	}
	if sawFalse && !sawInvalid {
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
		apperrors.WithHint("确认消息与会话信息可读取后重试；话题回复请使用 chat topic reply"),
	)
}
