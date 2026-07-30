// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

// ChatCreate creates a DingTalk group as the current user. It intentionally
// does not advertise Lark-only owner, description, initial-bot, or visibility
// semantics.
var ChatCreate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-create",
	Product:     "im",
	Description: "以当前用户身份创建钉钉群聊",
	Intent:      "当你要创建一个基础钉钉群聊时使用；自动把当前用户加入成员列表并作为群主，支持 INTERNAL、EXTERNAL、NORMAL 和话题模式。它不支持指定其他 owner、群 description、初始机器人或 Lark public/private 语义。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "群名称", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "初始成员 userId 或 openDingTalkId 列表", Required: true},
		{Name: "type", Type: shortcut.FlagString, Default: "INTERNAL", Desc: "群类型", Enum: []string{"INTERNAL", "EXTERNAL", "NORMAL"}},
		{Name: "thread", Type: shortcut.FlagBool, Desc: "创建为话题群"},
	},
	Tips: []string{
		`dws chat +chat-create --name "项目冲刺群" --users userId1,userId2`,
		`dws chat +chat-create --name "合作群" --users userId1,userId2 --type EXTERNAL`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return fmt.Errorf("读取当前用户以设置群主失败: %w", err)
		}
		currentUserID := currentProfileUserID(profile)
		if currentUserID == "" {
			return apperrors.NewValidation("当前用户资料缺少 userId，无法保证群主属于初始成员列表")
		}
		members := []string{currentUserID}
		for _, member := range rt.StrSlice("users") {
			member = strings.TrimSpace(member)
			if member != "" {
				members = appendUniqueShortcutString(members, member)
			}
		}
		params := map[string]any{
			"groupName":    rt.Str("name"),
			"groupMembers": members,
			"groupType":    rt.Str("type"),
		}
		if rt.Bool("thread") {
			params["convThreadEnabled"] = true
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
	Product:     "chat",
	Description: "更新群名称（仅名称，不支持 description）",
	Intent:      "当你只需要修改群名称时使用；这是 lark-cli +chat-update 的诚实子集，只接受群 openConversationId 和新名称。修改群 description、个人备注、群昵称或其他群设置时不要使用。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "新的群名称", Required: true},
	},
	Tips: []string{`dws chat +chat-update --group <openConversationId> --name "新群名"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_name", map[string]any{
			"openconversation_id": rt.Str("group"),
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
	Description: "以当前用户身份引用回复消息（自动补全原发送者）",
	Intent:      "当你要以当前用户身份对已有消息发送纯文本引用回复时使用；提供会话和被引用消息即可，默认通过 mget 自动读取原发送者，也可显式传 openDingTalkId/userId；userId 会通过通讯录搜索精确匹配 openDingTalkId。它不支持 bot 身份、富媒体、卡片或 thread 内回复。",
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
		return rt.CallMCP("send_personal_message", params)
	},
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
			if candidate := strings.TrimSpace(fmt.Sprint(typed[key])); isOpenID(candidate) {
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
		if candidate := strings.TrimSpace(fmt.Sprint(message[key])); isOpenID(candidate) {
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
		{Name: "message-id", Type: shortcut.FlagString, Desc: "单条消息 openMessageId"},
		{Name: "message-ids", Type: shortcut.FlagStringSlice, Desc: "多条消息 openMessageId（最多 10 条）"},
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
		{Name: "message-id", Type: shortcut.FlagString, Desc: "单条消息 openMessageId"},
		{Name: "message-ids", Type: shortcut.FlagStringSlice, Desc: "多条消息 openMessageId（最多 10 条）"},
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
	Description: "分页查询当前用户收藏的消息",
	Intent:      "当你要查看当前用户的 DingTalk message favorite 列表时使用；返回下层分页结果，不把 message favorite 与 Pin、会话置顶或 Lark feed-layer thread flag 混为一谈。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Default: "0", Desc: "数字分页游标，首次传 0"},
		{Name: "size", Type: shortcut.FlagInt, Default: "20", Desc: "每页数量，范围 1-100"},
	},
	Constraints: []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"cursor", "size"},
		Description: "--cursor 必须大于等于 0，--size 必须在 1-100 之间",
	}},
	Tips: []string{`dws chat +flag-list --cursor 0 --size 20`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("cursor") < 0 {
			return apperrors.NewValidation("--cursor 必须大于等于 0")
		}
		if size := rt.Int("size"); size < 1 || size > 100 {
			return apperrors.NewValidation("--size 必须在 1-100 之间")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_message_favorites", map[string]any{
			"cursor": rt.Int("cursor"),
			"size":   strconv.Itoa(rt.Int("size")),
		})
	},
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

func init() {
	shortcut.Register(
		ChatCreate,
		ChatUpdate,
		MessagesReply,
		FlagCreate,
		FlagCancel,
		FlagList,
		FeedGroupQueryItem,
	)
}
