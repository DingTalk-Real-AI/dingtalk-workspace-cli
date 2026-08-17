// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

const (
	flagListHardPageLimit = 500
	flagListMaxPageSize   = 30
	chatListHardPageLimit = 500
	chatListMaxWindowSize = 100
)

// ChatCreate creates a DingTalk group after resolving every natural member and
// the optional owner to stable DingTalk identities. Description, initial-bot,
// idempotency, and Lark visibility semantics remain deliberately unsupported.
var ChatCreate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-create",
	Product:     "im",
	Description: "按成员和可选群主全量预检后创建一个钉钉群聊",
	Intent:      "当你要创建钉钉群聊时使用；成员可传稳定 ID 或 --member-query 姓名，群主默认当前用户，也可用 --owner-open-dingtalk-id 或 --owner-query 明确指定。所有自然身份会在唯一解析并去重后才执行一次创建，任一零命中或多命中都会整体停止。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "群名称", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "初始成员 userId 或 openDingTalkId 列表"},
		{Name: "member-query", Type: shortcut.FlagStringSlice, Desc: "按姓名/花名唯一解析的初始成员，可逗号分隔或重复传入"},
		{Name: "owner-open-dingtalk-id", Type: shortcut.FlagString, Desc: "明确指定群主 openDingTalkId（与 --owner-query 互斥；省略时群主为当前用户）"},
		{Name: "owner-query", Type: shortcut.FlagString, Desc: "按姓名唯一解析群主 openDingTalkId（与 --owner-open-dingtalk-id 互斥）"},
		{Name: "type", Type: shortcut.FlagString, Default: "INTERNAL", Desc: "群类型", Enum: []string{"INTERNAL", "EXTERNAL", "NORMAL"}},
		{Name: "thread", Type: shortcut.FlagBool, Desc: "创建为话题群"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"users", "member-query"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"owner-open-dingtalk-id", "owner-query"}},
	},
	Tips: []string{
		`dws chat +chat-create --name "项目冲刺群" --users userId1,userId2`,
		`dws chat +chat-create --name "合作群" --member-query "张三,李四" --type EXTERNAL`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if ownerOpenID := rt.Str("owner-open-dingtalk-id"); ownerOpenID != "" {
			if err := targetresolver.ValidateExplicitOpenDingTalkID("--owner-open-dingtalk-id", ownerOpenID); err != nil {
				return err
			}
		}
		resolvedMembers, err := targetresolver.ResolveUsers(
			rt,
			rt.StrSlice("member-query"),
			targetresolver.IdentityAny,
		)
		if err != nil {
			return err
		}
		ownerOpenID := rt.Str("owner-open-dingtalk-id")
		if query := rt.Str("owner-query"); query != "" {
			resolvedOwner, resolveErr := targetresolver.ResolveUser(
				rt, query, targetresolver.IdentityOpenDingTalkID)
			if resolveErr != nil {
				return resolveErr
			}
			ownerOpenID = resolvedOwner.Selected.OpenDingTalkID
		}
		members := make([]string, 0, len(rt.StrSlice("users"))+len(resolvedMembers)+1)
		if ownerOpenID != "" {
			members = append(members, ownerOpenID)
		} else {
			profile, profileErr := rt.CallMCPData("contact", "get_current_user_profile", nil)
			if profileErr != nil {
				return fmt.Errorf("读取当前用户以设置群主失败: %w", profileErr)
			}
			currentUserID := currentProfileUserID(profile)
			if currentUserID == "" {
				return apperrors.NewValidation("当前用户资料缺少 userId，无法保证群主属于初始成员列表")
			}
			members = append(members, currentUserID)
		}
		for _, member := range rt.StrSlice("users") {
			member = strings.TrimSpace(member)
			if member != "" {
				members = appendUniqueShortcutString(members, member)
			}
		}
		for _, resolved := range resolvedMembers {
			member := resolved.Selected.UserID
			if member == "" {
				member = resolved.Selected.OpenDingTalkID
			}
			members = appendUniqueShortcutString(members, member)
		}
		params := map[string]any{
			"groupName":    rt.Str("name"),
			"groupMembers": members,
			"groupType":    rt.Str("type"),
		}
		if rt.Bool("thread") {
			params["convThreadEnabled"] = true
		}
		if ownerOpenID != "" {
			params["ownerOpenDingTalkId"] = ownerOpenID
		}
		if rt.DryRun() {
			return rt.CallMCP("create_group_conversation", params)
		}
		data, err := rt.CallMCPWriteData("im", "create_group_conversation", params)
		if err != nil {
			return err
		}
		normalizeCreatedConversation(data)
		return rt.Output(data)
	},
}

func currentProfileUserID(data map[string]any) string {
	candidates := []map[string]any{data}
	if result, ok := data["result"].(map[string]any); ok {
		candidates = append(candidates, result)
	}
	if result, ok := data["result"].([]any); ok {
		for _, item := range result {
			if mapped, ok := item.(map[string]any); ok {
				candidates = append(candidates, mapped)
			}
		}
	}
	for _, candidate := range candidates {
		if employee, ok := candidate["orgEmployeeModel"].(map[string]any); ok {
			if userID := shortcutString(employee, "userId", "userid", "orgUserId", "staffId"); userID != "" {
				return userID
			}
		}
		if userID := shortcutString(candidate, "userId", "userid", "orgUserId", "staffId"); userID != "" {
			return userID
		}
	}
	return ""
}

func normalizeCreatedConversation(data map[string]any) {
	result, ok := data["result"].(map[string]any)
	if !ok {
		return
	}
	if openCID, exists := result["openCid"]; exists {
		result["openConversationId"] = openCID
		delete(result, "openCid")
	}
	delete(result, "cid")
}

// ChatUpdate intentionally supports only the group-name subset shared by DWS
// and lark-cli. A description flag is omitted because DWS has no such lower
// capability.
var ChatUpdate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update",
	Aliases:     []string{"+chat-rename"},
	Product:     "chat",
	Description: "更新群名称（仅名称，不支持 description）",
	Intent:      "当你只需要修改群名称时使用；--group 可传群名或 openConversationId，群名必须唯一解析后才会写入。修改群 description、个人备注、群昵称或其他群设置时不要使用。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称或 openConversationId", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新的群名称", Required: true},
	},
	Tips: []string{`dws chat +chat-update --group <群名或openConversationId> --name "新群名"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		resolved, err := targetresolver.ResolveChatTarget(rt, rt.Str("group"), "")
		if err != nil {
			return err
		}
		return rt.CallMCP("update_group_name", map[string]any{
			"openconversation_id": resolved.Selected.OpenConversationID,
			"group_name":          rt.Str("name"),
		})
	},
}

// MessagesReply quote-replies with plain text as the current user. It can infer
// the original sender from the referenced message, so callers do not need to
// manually carry a second identity field from a previous list operation.
var MessagesReply = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-reply",
	Product:     "chat",
	Description: "引用回复一条已有消息，并返回可继续查询或撤回的发送上下文",
	Intent:      "当你要以当前用户身份对一条已有消息发送纯文本引用回复时使用；传会话和原消息 ID，CLI 会先读取原发送者，也可显式传 --ref-sender。成功结果在保留下层响应的同时增量返回 messageId（下层提供时）、conversationId、threadId（适用时）、deliveryStatus、idempotencyKey 和 referencedMessage 来源上下文。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "ref-msg-id", Type: shortcut.FlagString, Desc: "被引用消息 openMessageId"},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "--ref-msg-id 的 lark-cli 对齐别名"},
		{Name: "ref-sender", Type: shortcut.FlagString, Desc: "原消息发送者 openDingTalkId/userId（userId 通过通讯录搜索精确匹配；不传则自动读取）"},
		{Name: "text", Type: shortcut.FlagString, Desc: "纯文本回复内容", Required: true},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键（可选）"},
		{Name: "idempotency-key", Type: shortcut.FlagString, Desc: "--uuid 的 lark-cli 对齐别名"},
		shortcut.AIMessageTagFlag(),
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"ref-msg-id", "message-id"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"uuid", "idempotency-key"}},
	},
	Tips: []string{`dws chat +messages-reply --conversation-id <openConversationId> --message-id <openMessageId> --text "收到" --idempotency-key <key>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		refSender, err := resolveReplySender(rt)
		if err != nil {
			return err
		}
		content, _ := json.Marshal(map[string]string{
			"referenceOpenMessageId":   replyMessageID(rt),
			"srcMsgSendOpenDingTalkId": refSender,
			"replyMsgType":             "text",
			"content":                  rt.Str("text"),
		})
		params := rt.AddAIMessageTag(map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"msgType":            "reply",
			"content":            string(content),
		})
		if value := rt.StrFirst("idempotency-key", "uuid"); value != "" {
			params["uuid"] = value
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"contractVersion": "im.message-reply.v1",
				"dryRun":          true,
				"willSend":        false,
				"transport":       "chat/send_personal_message",
				"arguments":       params,
				"conversationId":  rt.Str("conversation-id"),
				"referencedMessage": map[string]any{
					"messageId":            replyMessageID(rt),
					"senderOpenDingTalkId": refSender,
				},
			})
		}
		data, err := rt.CallMCPWriteData("chat", "send_personal_message", params)
		if err != nil {
			return err
		}
		enrichReplyResult(data, rt, refSender)
		return rt.Output(data)
	},
}

func enrichReplyResult(data map[string]any, rt *shortcut.RuntimeContext, refSender string) {
	data["contractVersion"] = "im.message-reply.v1"
	data["conversationId"] = rt.Str("conversation-id")
	data["referencedMessage"] = map[string]any{
		"messageId":            replyMessageID(rt),
		"senderOpenDingTalkId": refSender,
		"resolutionSource": func() string {
			if rt.Str("ref-sender") != "" {
				return "explicit"
			}
			return "message_lookup"
		}(),
	}
	if key := rt.StrFirst("idempotency-key", "uuid"); key != "" {
		data["idempotencyKey"] = key
	}
	if value := replyResponseValue(data, "openMessageId", "openMsgId", "messageId", "msgId"); value != nil {
		data["messageId"] = value
	}
	if value := replyResponseValue(data, "openConvThreadId", "threadId", "topicId"); value != nil {
		data["threadId"] = value
	}
	if value := replyResponseValue(data, "deliveryStatus", "sendStatus", "status"); value != nil {
		data["deliveryStatus"] = value
		data["deliveryStatusKnown"] = true
	} else {
		data["deliveryStatus"] = "unknown"
		data["deliveryStatusKnown"] = false
	}
}

func replyResponseValue(data map[string]any, keys ...string) any {
	scopes := []map[string]any{data}
	for _, wrapper := range []string{"result", "data"} {
		if nested, ok := data[wrapper].(map[string]any); ok {
			scopes = append(scopes, nested)
		}
	}
	for _, scope := range scopes {
		for _, key := range keys {
			if value, ok := scope[key]; ok && value != nil {
				if text, isString := value.(string); !isString || strings.TrimSpace(text) != "" {
					return value
				}
			}
		}
	}
	return nil
}

func resolveReplySender(rt *shortcut.RuntimeContext) (string, error) {
	if value := rt.Str("ref-sender"); value != "" {
		if isOpenID(value) {
			return value, nil
		}
		openID, err := resolveUserOpenDingTalkID(rt, value)
		if err != nil {
			return "", fmt.Errorf("把 --ref-sender userId 解析为 openDingTalkId 失败: %w", err)
		}
		return openID, nil
	}

	messageID := replyMessageID(rt)
	data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{
		"openMsgIds": []string{messageID},
	})
	if err != nil {
		return "", fmt.Errorf("读取被引用消息以补全原发送者失败: %w", err)
	}
	for _, message := range shortcutMessageMaps(data) {
		if id := strings.TrimSpace(fmt.Sprint(chatmsg.MessageID(message))); id != "" && id != "<nil>" && id != messageID {
			continue
		}
		if openID := findMessageSenderOpenDingTalkID(message); openID != "" {
			return openID, nil
		}
	}
	return "", apperrors.NewValidation("被引用消息未返回 senderOpenDingTalkId；请显式传 --ref-sender")
}

func replyMessageID(rt *shortcut.RuntimeContext) string {
	return rt.StrFirst("message-id", "ref-msg-id")
}

func findOpenDingTalkID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"senderOpenDingTalkId", "senderOpenDingtalkId", "openDingTalkId", "openDingtalkId"} {
			if candidate := strings.TrimSpace(fmt.Sprint(typed[key])); candidate != "" && candidate != "<nil>" {
				return candidate
			}
		}
		for _, child := range typed {
			if candidate := findOpenDingTalkID(child); candidate != "" {
				return candidate
			}
		}
	case []any:
		for _, child := range typed {
			if candidate := findOpenDingTalkID(child); candidate != "" {
				return candidate
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if candidate := findOpenDingTalkID(child); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

// findMessageSenderOpenDingTalkID intentionally inspects only sender identity
// fields. A message may contain unrelated openDingTalkId values in mentions,
// quoted content, cards, or attachments; recursively taking the first identity
// from the whole payload could reply with the wrong source sender.
func findMessageSenderOpenDingTalkID(message map[string]any) string {
	for _, key := range []string{
		"senderOpenDingTalkId",
		"senderOpenDingtalkId",
		"senderOpenId",
	} {
		if candidate := strings.TrimSpace(fmt.Sprint(message[key])); candidate != "" && candidate != "<nil>" {
			return candidate
		}
	}
	for _, key := range []string{"sender", "senderInfo", "senderUser"} {
		if candidate := findOpenDingTalkID(message[key]); candidate != "" {
			return candidate
		}
	}
	return ""
}

func shortcutMessageMaps(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	scopes := []map[string]any{data}
	for _, key := range []string{"result", "data"} {
		if inner, ok := data[key].(map[string]any); ok {
			scopes = append(scopes, inner)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"result", "data", "messages", "list", "items", "records"} {
			switch values := scope[key].(type) {
			case []any:
				out := make([]map[string]any, 0, len(values))
				for _, value := range values {
					if mapped, ok := value.(map[string]any); ok {
						out = append(out, mapped)
					}
				}
				return out
			case []map[string]any:
				return values
			}
		}
	}
	return nil
}

// FlagCreate/FlagCancel/FlagList map Lark's message flag intent to DingTalk's
// message-favorite layer. They are explicitly not Pin or Feed Shortcut state.
var FlagCreate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+flag-create",
	Product:     "im",
	Description: "收藏一条或多条消息（最多 10 条）",
	Intent:      "当你要把同一会话中的一条或多条消息加入当前用户的个人收藏时使用；逐项返回成功/失败 ledger。这是消息 favorite，不是消息 Pin、会话置顶或 feed-layer thread flag。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "message-id", Type: shortcut.FlagString, Desc: "单条消息 openMessageId；消息 ID 去重后必须为 1-10 条"},
		{Name: "message-ids", Type: shortcut.FlagStringSlice, Desc: "多条消息 openMessageId；消息 ID 去重后必须为 1-10 条"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "消息所在会话 openConversationId", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"message-id", "message-ids"}},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"message-id", "message-ids"},
			Description: "消息 ID 去重后必须为 1-10 条",
		},
	},
	Tips:     []string{`dws chat +flag-create --message-id <openMessageId> --conversation-id <openConversationId>`},
	Validate: validateFlagMessageIDs,
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeFlagBatch(rt, "add_message_favorite")
	},
}

var FlagCancel = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+flag-cancel",
	Product:     "im",
	Description: "取消收藏一条或多条消息（最多 10 条）",
	Intent:      "当你要移除当前用户对同一会话中一条或多条消息的个人收藏标记时使用；逐项返回成功/失败 ledger，只影响 message favorite，不删除原消息，也不会修改 Pin 或会话置顶。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "message-id", Type: shortcut.FlagString, Desc: "单条消息 openMessageId；消息 ID 去重后必须为 1-10 条"},
		{Name: "message-ids", Type: shortcut.FlagStringSlice, Desc: "多条消息 openMessageId；消息 ID 去重后必须为 1-10 条"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "消息所在会话 openConversationId", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"message-id", "message-ids"}},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"message-id", "message-ids"},
			Description: "消息 ID 去重后必须为 1-10 条",
		},
	},
	Tips:     []string{`dws chat +flag-cancel --message-id <openMessageId> --conversation-id <openConversationId>`},
	Validate: validateFlagMessageIDs,
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeFlagBatch(rt, "remove_message_favorite")
	},
}

func flagMessageIDs(rt *shortcut.RuntimeContext) []string {
	values := append([]string{}, rt.StrSlice("message-ids")...)
	if value := rt.Str("message-id"); value != "" {
		values = append(values, value)
	}
	return uniqueShortcutStrings(values)
}

func validateFlagMessageIDs(rt *shortcut.RuntimeContext) error {
	ids := flagMessageIDs(rt)
	if len(ids) < 1 || len(ids) > 10 {
		return apperrors.NewValidation(fmt.Sprintf("消息 ID 去重后必须为 1-10 条，当前 %d 条", len(ids)))
	}
	return nil
}

func executeFlagBatch(rt *shortcut.RuntimeContext, tool string) error {
	conversationID := rt.Str("conversation-id")
	ids := flagMessageIDs(rt)
	items := make([]shortcutBatchWrite, 0, len(ids))
	for _, id := range ids {
		items = append(items, shortcutBatchWrite{
			target: id,
			arguments: map[string]any{
				"openMessageId":      id,
				"openConversationId": conversationID,
			},
		})
	}
	return executeShortcutBatchWrite(rt, "im", tool, items)
}

var FlagList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+flag-list",
	Product:     "im",
	Description: "分页查询当前用户收藏的消息，支持有界自动翻页",
	Intent:      "当你要查看当前用户的 DingTalk message favorite 列表时使用；默认读取一页，明确要求全部收藏时加 --page-all，并用 --page-limit 保持有界。底层实际使用数字 cursor，结果按 openMessageId 去重并公开 complete、hasMore、nextCursor、stopReason 和 failures；它不把 message favorite 与 Pin、会话置顶或 Lark feed-layer thread flag 混为一谈。",
	Risk:        shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量；下游真实上限为 30，显式页大小必须在 1-30 之间"},
		{Name: "size", Type: shortcut.FlagInt, Default: "20", Desc: "--page-size 的兼容别名；下游真实上限为 30，显式页大小必须在 1-30 之间"},
		{Name: "page-token", Type: shortcut.FlagString, Desc: "Lark 对齐的起始分页参数；起始 cursor 必须是非负整数"},
		{Name: "cursor", Type: shortcut.FlagInt, Default: "0", Desc: "钉钉数字分页游标；起始 cursor 必须是非负整数"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动读取全部收藏分页；--page-limit 仅与 --page-all 一起使用且范围 1-500；--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "20", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlFlags()...),
	Constraints: append([]shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"page-size", "size"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"page-token", "cursor"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-size", "size"}, Description: "显式页大小必须在 1-30 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-token", "cursor"}, Description: "起始 cursor 必须是非负整数"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlConstraints()...),
	Tips: []string{
		`dws chat +flag-list --cursor 0 --page-size 20`,
		`dws chat +flag-list --page-size 30 --page-all --page-limit 20`,
	},
	Validate: validateFlagList,
	Execute:  executeFlagList,
}

func validateFlagList(rt *shortcut.RuntimeContext) error {
	if size := flagListPageSize(rt); size < 1 || size > flagListMaxPageSize {
		return apperrors.NewValidation("--page-size/--size 必须在 1-30 之间")
	}
	if _, err := flagListStartCursor(rt); err != nil {
		return err
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > flagListHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	if err := shortcut.ValidateAutoPageControls(rt); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	return nil
}

func flagListPageSize(rt *shortcut.RuntimeContext) int {
	if rt.Changed("size") {
		return rt.Int("size")
	}
	return rt.Int("page-size")
}

func flagListStartCursor(rt *shortcut.RuntimeContext) (int, error) {
	if rt.Changed("page-token") {
		token := strings.TrimSpace(rt.Str("page-token"))
		if token == "" || token == "0" {
			return 0, nil
		}
		value, err := strconv.Atoi(token)
		if err != nil || value < 0 {
			return 0, apperrors.NewValidation("--page-token 必须是非负整数")
		}
		return value, nil
	}
	if value := rt.Int("cursor"); value < 0 {
		return 0, apperrors.NewValidation("--cursor 必须大于等于 0")
	}
	return rt.Int("cursor"), nil
}

func executeFlagList(rt *shortcut.RuntimeContext) error {
	pageSize := flagListPageSize(rt)
	cursor, _ := flagListStartCursor(rt) // Validate has already accepted the cursor.
	if rt.DryRun() {
		return rt.CallMCP("list_message_favorites", flagListRequestParams(cursor, pageSize))
	}
	pageLimit := 1
	if rt.Bool("page-all") {
		pageLimit = rt.Int("page-limit")
	}
	seenCursors := map[int]bool{cursor: true}
	seenMessages := map[string]bool{}
	items := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	paginationKnown := true
	complete := false
	hasMore := false
	nextCursor := 0
	var cursorErr error
	stopReason := "source_complete"
	truncatedByPageLimit := false
	truncatedByResultLimit := false

	for pagesFetched < pageLimit {
		if pagesFetched > 0 {
			if err := shortcut.WaitAutoPageDelay(rt); err != nil {
				failures = append(failures, map[string]any{
					"page": pagesFetched + 1, "stage": "delay", "cursor": cursor, "error": err.Error(),
				})
				stopReason = "delay_interrupted"
				break
			}
		}
		requestPageSize := shortcut.AutoPageRequestSize(rt, pageSize, len(items))
		data, callErr := rt.CallMCPData("im", "list_message_favorites", flagListRequestParams(cursor, requestPageSize))
		if callErr != nil {
			if pagesFetched == 0 {
				return callErr
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "cursor": cursor, "error": callErr.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		pageItems := flagListItems(data)
		overflowOnPage := false
		for _, item := range pageItems {
			messageID := firstNonEmptyMapString(item, "openMessageId", "messageId", "itemId", "id")
			if messageID != "" && seenMessages[messageID] {
				continue
			}
			if messageID != "" {
				seenMessages[messageID] = true
			}
			if maxItems := rt.Int("max-items"); maxItems > 0 && len(items) >= maxItems {
				truncatedByResultLimit = true
				overflowOnPage = true
				continue
			}
			items = append(items, item)
		}

		page := chatmsg.Pagination(data)
		pageHasMore, hasMoreKnown := page["hasMore"].(bool)
		nextCursor, cursorErr = flagListNextCursor(page["nextCursor"])
		if !hasMoreKnown {
			switch {
			case nextCursor > 0:
				pageHasMore = true
			case len(pageItems) < requestPageSize:
				paginationKnown = false
				complete = true
				hasMore = false
				stopReason = "legacy_short_page"
			default:
				paginationKnown = false
				failures = append(failures, map[string]any{
					"page": pagesFetched, "stage": "pagination",
					"error": "收藏列表返回满页结果但缺少 hasMore/nextCursor，无法证明结果完整",
				})
				stopReason = "pagination_error"
			}
			if complete || len(failures) > 0 {
				break
			}
		}
		hasMore = pageHasMore
		if overflowOnPage {
			hasMore = true
			nextCursor = 0
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "收藏列表下层返回条数超过请求的剩余额度，无法生成不跳项的安全续页游标",
			})
			stopReason = "pagination_error"
			break
		}
		if !hasMore {
			complete = true
			nextCursor = 0
			stopReason = "source_complete"
			break
		}
		if cursorErr != nil || nextCursor <= 0 || seenCursors[nextCursor] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "收藏列表返回 hasMore=true，但数字 nextCursor 缺失、无效或未前进",
			})
			stopReason = "pagination_error"
			break
		}
		if !rt.Bool("page-all") {
			stopReason = "single_page"
			break
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
		if maxItems := rt.Int("max-items"); maxItems > 0 && len(items) >= maxItems {
			truncatedByResultLimit = true
			stopReason = "result_limit"
			break
		}
	}

	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit && !truncatedByResultLimit {
		truncatedByPageLimit = rt.Bool("page-all")
		if truncatedByPageLimit {
			stopReason = "page_limit"
		}
	}
	payload := map[string]any{
		"count":                  len(items),
		"items":                  items,
		"pagesFetched":           pagesFetched,
		"paginationKnown":        paginationKnown,
		"complete":               complete && len(failures) == 0,
		"hasMore":                hasMore,
		"nextCursor":             nextCursor,
		"stopReason":             stopReason,
		"truncatedByPageLimit":   truncatedByPageLimit,
		"truncatedByResultLimit": truncatedByResultLimit,
		"failedCount":            len(failures),
		"failures":               failures,
		"partial":                len(failures) > 0 && len(items) > 0,
	}
	chatmsg.ApplyTruncation(payload)
	if outputErr := rt.Output(payload); outputErr != nil {
		return outputErr
	}
	if len(failures) > 0 {
		return apperrors.NewAPI(
			fmt.Sprintf("收藏消息分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
			apperrors.WithOperation("im/list_message_favorites"),
			apperrors.WithReason("flag_list_incomplete"),
			apperrors.WithOrigin("mcp_gateway"),
			apperrors.WithFailureStage("pagination"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(true),
			apperrors.WithHint("请根据 failures 和 nextCursor 重试"),
		)
	}
	return nil
}

func flagListRequestParams(cursor, pageSize int) map[string]any {
	return map[string]any{
		"cursor": cursor,
		"size":   strconv.Itoa(pageSize),
	}
}

func flagListItems(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	scopes := []map[string]any{data}
	for _, key := range []string{"result", "data"} {
		if nested, ok := data[key].(map[string]any); ok {
			scopes = append(scopes, nested)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"items", "favorites", "messageFavorites", "list"} {
			raw, ok := scope[key].([]any)
			if !ok {
				continue
			}
			out := make([]map[string]any, 0, len(raw))
			for _, item := range raw {
				if value, ok := item.(map[string]any); ok {
					out = append(out, value)
				}
			}
			return out
		}
	}
	return []map[string]any{}
}

func flagListNextCursor(value any) (int, error) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case int:
		if typed < 0 {
			return 0, fmt.Errorf("negative cursor")
		}
		return typed, nil
	case int64:
		if typed < 0 || int64(int(typed)) != typed {
			return 0, fmt.Errorf("cursor out of range")
		}
		return int(typed), nil
	case float64:
		if typed < 0 || math.Trunc(typed) != typed || typed > float64(int(^uint(0)>>1)) {
			return 0, fmt.Errorf("invalid numeric cursor")
		}
		return int(typed), nil
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		if err != nil || parsed < 0 || int64(int(parsed)) != parsed {
			return 0, fmt.Errorf("invalid json cursor")
		}
		return int(parsed), nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || text == "0" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("invalid string cursor")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported cursor type %T", value)
	}
}

func firstNonEmptyMapString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(item[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

// FeedGroupQueryItem is a client-side exact filter over a DingTalk conversation
// category. It does not claim Lark deleted-item or server-side multi-ID query
// semantics.
var FeedGroupQueryItem = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+feed-group-query-item",
	Product:     "im",
	Description: "在会话分组结果中按会话 ID 精确查询多项",
	Intent:      "当你已知一个钉钉会话分组 ID 和若干 openConversationId、想精确取回这些分组项时使用；先读取该分组，再按 ID 本地过滤并返回未找到清单。若下层表明结果仍有后续页但未提供可执行游标，本命令会把缺失项标为 unresolved 并返回失败 ledger，不会误报 notFound。它不提供 Lark deleted_items 或服务端多 ID 查询语义。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "category-id", Type: shortcut.FlagInt, Desc: "钉钉会话分组 ID", Required: true},
		{Name: "conversation-ids", Type: shortcut.FlagStringSlice, Desc: "要精确查询的 openConversationId 列表", Required: true},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "读取分组时排除已免打扰会话"},
	},
	Tips: []string{`dws chat +feed-group-query-item --category-id <分组ID> --conversation-ids <openConversationId1>,<openConversationId2>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"categoryId": rt.Int("category-id")}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("im", "list_conversations_by_category", params)
		if err != nil {
			return err
		}
		payload := feedGroupQueryProject(categoryConversationsProject(data), rt.StrSlice("conversation-ids"))
		chatmsg.ApplyPagination(payload, data)
		if page := chatmsg.Pagination(data); page["hasMore"] == true {
			unresolved, _ := payload["notFoundConversationIds"].([]string)
			payload["ok"] = false
			payload["complete"] = false
			payload["notFoundCount"] = 0
			payload["notFoundConversationIds"] = []string{}
			payload["unresolvedCount"] = len(unresolved)
			payload["unresolvedConversationIds"] = unresolved
			payload["failedCount"] = 1
			payload["failures"] = []map[string]any{{
				"stage": "source-pagination",
				"error": "下层返回 hasMore=true，但 list_conversations_by_category 未公开可继续游标；无法证明缺失会话不在后续页",
			}}
		} else {
			payload["complete"] = true
			payload["unresolvedCount"] = 0
			payload["unresolvedConversationIds"] = []string{}
		}
		return rt.Output(payload)
	},
}

func feedGroupQueryProject(conversations []map[string]any, requestedIDs []string) map[string]any {
	byID := make(map[string]map[string]any, len(conversations))
	for _, conversation := range conversations {
		id := strings.TrimSpace(fmt.Sprint(conversation["openConversationId"]))
		if id != "" {
			byID[id] = conversation
		}
	}
	items := make([]map[string]any, 0, len(requestedIDs))
	notFound := make([]string, 0)
	seen := map[string]bool{}
	for _, requestedID := range requestedIDs {
		requestedID = strings.TrimSpace(requestedID)
		if requestedID == "" || seen[requestedID] {
			continue
		}
		seen[requestedID] = true
		if item := byID[requestedID]; item != nil {
			items = append(items, item)
		} else {
			notFound = append(notFound, requestedID)
		}
	}
	return map[string]any{
		"ok":                      len(notFound) == 0,
		"requestedCount":          len(seen),
		"foundCount":              len(items),
		"notFoundCount":           len(notFound),
		"failedCount":             0,
		"items":                   items,
		"notFoundConversationIds": notFound,
		"failures":                []map[string]any{},
	}
}

// ChatList is the lark-cli +chat-list alignment for DingTalk. It lists the
// current user's conversations through list_all_conversations, defaults to
// groups only (lark omit-types behavior), and applies --types after all fetched
// pages have been merged. Sort order and bot
// identity p2p stripping are intentionally omitted: DingTalk has no matching
// sort_type parameter here, and DWS shortcuts run as the signed-in user.
var ChatList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list",
	Product:     "im",
	Description: "分页列出当前用户加入的会话（默认群聊；可选包含单聊）",
	Intent: "当你要像 lark-cli +chat-list 一样列出当前用户加入的会话时使用；默认只返回群聊，" +
		"--types 可追加 p2p（钉钉投影为 conversationType=direct）。" +
		"--exclude-muted 交给下层 excludeMuted；默认读取一页，明确要求全部时加 --page-all，合并并去重所有已读取页面后再执行 --types 过滤，结果公开完整性 ledger。" +
		"不支持 lark 的 sort/sort-type，也不模拟 bot 身份剥离 p2p。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "types", Type: shortcut.FlagStringSlice, Desc: "会话类型只能包含 group 和/或 p2p；省略时默认只返回群聊"},
		{Name: "page-size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量，必须在 1-100 之间"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "--page-size 的别名，必须在 1-100 之间"},
		{Name: "page-token", Type: shortcut.FlagString, Desc: "分页游标；若提供则必须是非负整数"},
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "--page-token 的整数别名"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动读取全部会话分页；--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已免打扰会话"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"page-size", "limit"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"page-token", "cursor"}},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"types"},
			Description: "只能包含 group 和/或 p2p",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"page-size", "limit"},
			Description: "必须在 1-100 之间",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"page-token"},
			Description: "若提供则必须是非负整数",
		},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	},
	Tips: []string{
		`dws chat +chat-list`,
		`dws chat +chat-list --types group,p2p --exclude-muted --page-size 50`,
		`dws chat +chat-list --types group --page-size 100 --page-all --page-limit 20`,
	},
	Validate: validateChatList,
	Execute:  executeChatList,
}

func validateChatList(rt *shortcut.RuntimeContext) error {
	if _, err := normalizeChatListTypes(rt.StrSlice("types")); err != nil {
		return err
	}
	pageSize := chatListPageSize(rt)
	if pageSize < 1 || pageSize > 100 {
		return apperrors.NewValidation("--page-size/--limit 必须在 1-100 之间")
	}
	if _, err := chatListCursor(rt); err != nil {
		return err
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > chatListHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	return nil
}

func executeChatList(rt *shortcut.RuntimeContext) error {
	types, err := normalizeChatListTypes(rt.StrSlice("types"))
	if err != nil {
		return err
	}
	if len(types) == 0 {
		types = []string{"group"}
	}
	cursor, err := chatListCursor(rt)
	if err != nil {
		return err
	}
	pageSize := chatListPageSize(rt)
	requestPageSize := pageSize
	initialCursor := cursor
	pageLimit := 1
	if rt.Bool("page-all") {
		pageLimit = rt.Int("page-limit")
	}
	seenCursors := map[int]bool{cursor: true}
	seenChats := map[string]bool{}
	allChats := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	paginationKnown := true
	complete := false
	hasMore := false
	nextCursor := 0
	stopReason := "source_complete"
	truncatedByPageLimit := false
	maxWindowProbeUsed := false
	completionEvidence := ""

	for pagesFetched < pageLimit {
		params := map[string]any{"limit": requestPageSize}
		if cursor > 0 {
			params["cursor"] = cursor
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, callErr := rt.CallMCPData("im", "list_all_conversations", params)
		if callErr != nil {
			if pagesFetched == 0 {
				return callErr
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "cursor": cursor, "error": callErr.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		pageItems := chatListProject(data)
		for _, chat := range pageItems {
			id := firstNonEmptyMapString(chat, "openConversationId")
			if id != "" && seenChats[id] {
				continue
			}
			if id != "" {
				seenChats[id] = true
			}
			allChats = append(allChats, chat)
		}

		page, paginationErr := chatListPagination(data)
		if paginationErr != nil {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination", "error": paginationErr.Error(),
			})
			stopReason = "pagination_error"
			break
		}
		pageHasMore, hasMoreKnown := page["hasMore"].(bool)
		nextCursor, err = chatListNextCursor(page["nextCursor"])
		if !hasMoreKnown {
			switch {
			case nextCursor > 0:
				pageHasMore = true
			case len(pageItems) < requestPageSize:
				paginationKnown = false
				complete = true
				hasMore = false
				stopReason = "legacy_short_page"
			default:
				paginationKnown = false
				failures = append(failures, map[string]any{
					"page": pagesFetched, "stage": "pagination",
					"error": "会话列表返回满页结果但缺少 hasMore/nextCursor，无法证明结果完整",
				})
				stopReason = "pagination_error"
			}
			if complete || len(failures) > 0 {
				break
			}
		}
		hasMore = pageHasMore
		if !hasMore {
			// A full first page with hasMore=false/no cursor is the live legacy
			// shape that can hide additional rows. Once a prior page supplied a
			// valid continuation cursor, its terminal hasMore=false is trustworthy.
			fullPageWithoutCursor := len(pageItems) >= requestPageSize && nextCursor <= 0 &&
				(pagesFetched == 1 || (maxWindowProbeUsed && pagesFetched == 2))
			if fullPageWithoutCursor {
				paginationKnown = false
				if rt.Bool("page-all") && initialCursor == 0 && !maxWindowProbeUsed && requestPageSize < chatListMaxWindowSize && pagesFetched < pageLimit {
					maxWindowProbeUsed = true
					requestPageSize = chatListMaxWindowSize
					cursor = initialCursor
					stopReason = "max_window_probe"
					continue
				}
				if !rt.Bool("page-all") {
					complete = false
					stopReason = "single_page_full_untrusted"
					break
				}
				if pagesFetched >= pageLimit && requestPageSize < chatListMaxWindowSize {
					truncatedByPageLimit = true
					complete = false
					stopReason = "page_limit"
					break
				}
				failures = append(failures, map[string]any{
					"page": pagesFetched, "stage": "pagination",
					"error": "会话列表在最大窗口仍返回满页且没有可用 nextCursor，无法证明结果完整",
				})
				complete = false
				stopReason = "pagination_error"
				break
			}
			complete = true
			nextCursor = 0
			if maxWindowProbeUsed {
				paginationKnown = false
				completionEvidence = "max_window_short_page"
				stopReason = "max_window_short_page"
			} else {
				completionEvidence = "backend_has_more_false_short_page"
				stopReason = "source_complete"
			}
			break
		}
		if err != nil || nextCursor <= 0 || seenCursors[nextCursor] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "会话列表返回 hasMore=true，但数字 nextCursor 缺失、无效或未前进",
			})
			stopReason = "pagination_error"
			break
		}
		if !rt.Bool("page-all") {
			stopReason = "single_page"
			break
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit {
		truncatedByPageLimit = rt.Bool("page-all")
		if truncatedByPageLimit {
			stopReason = "page_limit"
		}
	}
	chats := chatListFilterTypes(allChats, types)
	payload := map[string]any{
		"count":                len(chats),
		"chats":                chats,
		"requestedTypes":       types,
		"pagesFetched":         pagesFetched,
		"paginationKnown":      paginationKnown,
		"complete":             complete && len(failures) == 0,
		"hasMore":              hasMore,
		"nextCursor":           nextCursor,
		"stopReason":           stopReason,
		"truncatedByPageLimit": truncatedByPageLimit,
		"failedCount":          len(failures),
		"failures":             failures,
		"partial":              len(failures) > 0 && len(chats) > 0,
		"requestedPageSize":    pageSize,
		"effectivePageSize":    requestPageSize,
	}
	if completionEvidence != "" {
		payload["completionEvidence"] = completionEvidence
	}
	if rt.Bool("exclude-muted") {
		payload["filter"] = map[string]any{"excludeMuted": true}
	}
	if outputErr := rt.Output(payload); outputErr != nil {
		return outputErr
	}
	if len(failures) > 0 {
		return apperrors.NewAPI(
			fmt.Sprintf("会话列表分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
			apperrors.WithOperation("im/list_all_conversations"),
			apperrors.WithReason("chat_list_incomplete"),
			apperrors.WithOrigin("mcp_gateway"),
			apperrors.WithFailureStage("pagination"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(true),
			apperrors.WithHint("请根据 failures 和 nextCursor 重试"),
		)
	}
	return nil
}

// chatListPagination preserves the shared map/envelope pagination formats and
// supplements them with the list_all_conversations gateway tuple:
// result:[conversationList,nextCursor,hasMore]. Tuple positions are specific to
// this RPC, so they are normalized here instead of broadening chatmsg.Pagination
// for unrelated Chat shortcuts.
func chatListPagination(data map[string]any) (map[string]any, error) {
	page := chatmsg.Pagination(data)
	for _, key := range []string{"result", "data"} {
		values, ok := data[key].([]any)
		if !ok || len(values) == 0 {
			continue
		}
		if _, tuple := values[0].([]any); !tuple {
			continue
		}
		if len(values) < 3 {
			return nil, fmt.Errorf("会话列表 tuple 分页元数据不完整：期望 [conversationList,nextCursor,hasMore]")
		}
		hasMore, ok := values[2].(bool)
		if !ok {
			return nil, fmt.Errorf("会话列表 tuple 的 hasMore 必须是布尔值")
		}
		return map[string]any{
			"nextCursor": values[1],
			"hasMore":    hasMore,
			"complete":   !hasMore,
		}, nil
	}
	return page, nil
}

func chatListNextCursor(value any) (int, error) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case int:
		if typed < 0 {
			return 0, fmt.Errorf("negative cursor")
		}
		return typed, nil
	case int64:
		if typed < 0 || int64(int(typed)) != typed {
			return 0, fmt.Errorf("cursor out of range")
		}
		return int(typed), nil
	case float64:
		if typed < 0 || math.Trunc(typed) != typed || typed > float64(int(^uint(0)>>1)) {
			return 0, fmt.Errorf("invalid numeric cursor")
		}
		return int(typed), nil
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		if err != nil || parsed < 0 || int64(int(parsed)) != parsed {
			return 0, fmt.Errorf("invalid json cursor")
		}
		return int(parsed), nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || text == "0" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("invalid string cursor")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported cursor type %T", value)
	}
}

func chatListPageSize(rt *shortcut.RuntimeContext) int {
	if rt.Changed("limit") {
		return rt.Int("limit")
	}
	if rt.Changed("page-size") {
		return rt.Int("page-size")
	}
	if size := rt.Int("page-size"); size > 0 {
		return size
	}
	return 20
}

func chatListCursor(rt *shortcut.RuntimeContext) (int, error) {
	if rt.Changed("cursor") {
		if value := rt.Int("cursor"); value < 0 {
			return 0, apperrors.NewValidation("--cursor 必须大于等于 0")
		}
		return rt.Int("cursor"), nil
	}
	token := strings.TrimSpace(rt.Str("page-token"))
	if token == "" || token == "0" {
		return 0, nil
	}
	value, err := strconv.Atoi(token)
	if err != nil || value < 0 {
		return 0, apperrors.NewValidation("--page-token 必须是非负整数")
	}
	return value, nil
}

func normalizeChatListTypes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		normalized := strings.TrimSpace(strings.ToLower(value))
		if normalized == "" {
			return nil, apperrors.NewValidation("--types 不能包含空值")
		}
		if normalized != "group" && normalized != "p2p" {
			return nil, apperrors.NewValidation(fmt.Sprintf("--types 含有无效值 %q：期望 group 或 p2p", value))
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func chatListProject(data map[string]any) []map[string]any {
	raw := conversationListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := conversationListFirst(m, "openConversationId", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := conversationListFirst(m, "conversationName", "name", "title"); ok {
			row["name"] = v
		}
		if conversationType, ok := conversationListTopType(m); ok {
			row["conversationType"] = conversationType
			if conversationType == "direct" {
				row["chatMode"] = "p2p"
			} else {
				row["chatMode"] = "group"
			}
		}
		if v, ok := conversationListFirst(m, "ownerUserId", "ownerId", "owner"); ok {
			row["ownerUserId"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func chatListFilterTypes(chats []map[string]any, types []string) []map[string]any {
	wantDirect := false
	wantGroup := false
	for _, value := range types {
		switch value {
		case "p2p":
			wantDirect = true
		case "group":
			wantGroup = true
		}
	}
	if !wantDirect && !wantGroup {
		return chats
	}
	out := make([]map[string]any, 0, len(chats))
	for _, chat := range chats {
		switch chat["conversationType"] {
		case "direct":
			if wantDirect {
				out = append(out, chat)
			}
		case "group":
			if wantGroup {
				out = append(out, chat)
			}
		default:
			// Unknown types are dropped under an explicit type filter so agents
			// never treat untyped rows as an approved group/p2p match.
		}
	}
	return out
}

func init() {
	shortcut.Register(withReviewedChatShortcutContracts(
		ChatCreate,
		ChatList,
		ChatUpdate,
		MessagesReply,
		FlagCreate,
		FlagCancel,
		FlagList,
		FeedGroupQueryItem,
	)...)
}
