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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

const directMessagesHardPageLimit = 500

// MessagesSend sends a text/markdown message as the current user
// (send_personal_message, chat server). Media/file variants are not covered.
// MessagesReply quote-replies a message (send_personal_message, chat server).
// MessagesSendByBot sends a group message via a robot (send_robot_group_message, bot).
var MessagesSendByBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-send-by-bot",
	Product:     "bot",
	Description: "机器人向群聊发送 Markdown 消息",
	Intent:      "当你要用机器人向某群推送 Markdown 消息（如日报、告警播报）时使用；会实际以机器人身份发群消息，需传 robotCode、群 openConversationId、标题与正文，可 @人或 @所有人。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code", Required: true},
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "消息标题", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "Markdown 正文", Required: true, Aliases: []string{"text"}},
		{Name: "at-user-ids", Type: shortcut.FlagStringSlice, Desc: "@ 的 userId 列表"},
		{Name: "at-open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "@ 的 openDingTalkId 列表"},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@ 所有人"},
	},
	Tips: []string{`dws chat +messages-send-by-bot --robot-code <robotCode> --group <openConversationId> --title "日报" --content "## 今日完成"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := validateExplicitOpenIDs("--at-open-dingtalk-ids", rt.StrSlice("at-open-dingtalk-ids")); err != nil {
			return err
		}
		params := map[string]any{
			"robotCode":          rt.Str("robot-code"),
			"openConversationId": rt.Str("group"),
			"title":              rt.Str("title"),
			"markdown":           rt.StrFirst("text", "content"),
		}
		if v := rt.StrSlice("at-user-ids"); len(v) > 0 {
			params["atUserIds"] = v
		}
		if v := rt.StrSlice("at-open-dingtalk-ids"); len(v) > 0 {
			params["atOpendingtalkIds"] = v
		}
		if rt.Bool("at-all") {
			params["isAtAll"] = "true"
		}
		return rt.CallMCP("send_robot_group_message", params)
	},
}

// MessagesBatchSendByBot sends single-chat messages via a robot to users
// (batch_send_robot_msg_to_users, bot).
var MessagesBatchSendByBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-batch-send-by-bot",
	Product:     "bot",
	Description: "机器人批量向用户发送单聊 Markdown 消息",
	Intent:      "当你要用机器人给多个人分别发单聊 Markdown 消息（如批量提醒交周报）时使用；会实际批量发送单聊消息，需传 robotCode、接收人列表（userId 或 openDingTalkId）及标题正文。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "消息标题", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "Markdown 正文", Required: true, Aliases: []string{"text"}},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "接收人 userId 列表"},
		{Name: "open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "接收人 openDingTalkId 列表"},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@ 所有人"},
	},
	Tips: []string{`dws chat +messages-batch-send-by-bot --robot-code <robotCode> --users userId1,userId2 --title "提醒" --content "请提交周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := validateExplicitOpenIDs("--open-dingtalk-ids", rt.StrSlice("open-dingtalk-ids")); err != nil {
			return err
		}
		params := map[string]any{
			"robotCode": rt.Str("robot-code"),
			"title":     rt.Str("title"),
			"markdown":  rt.StrFirst("text", "content"),
		}
		if v := rt.StrSlice("users"); len(v) > 0 {
			params["userIds"] = v
		}
		if v := rt.StrSlice("open-dingtalk-ids"); len(v) > 0 {
			params["openDingtalkIds"] = v
		}
		if rt.Bool("at-all") {
			params["isAtAll"] = "true"
		}
		return rt.CallMCP("batch_send_robot_msg_to_users", params)
	},
}

// MessagesSendByWebhook sends via a custom robot webhook (send_message_by_custom_robot, bot).
var MessagesSendByWebhook = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-send-by-webhook",
	Product:     "bot",
	Description: "兼容旧入口的自定义机器人 Webhook 群消息发送",
	Intent:      "只有既有自动化明确依赖 +messages-send-by-webhook 兼容路径、暂时不能迁移统一身份入口时使用",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_send_by_webhook",
			CanonicalPath:  "chat.shortcut_messages_send_by_webhook",
			CLIPath:        "chat +messages-send-by-webhook",
			PrimaryCLIPath: "chat +messages-send-by-webhook",
		},
		Description: "兼容旧入口的自定义机器人 Webhook 群消息发送",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "兼容旧入口的自定义机器人 Webhook 群消息发送",
			UseWhen:      []string{"只有既有自动化明确依赖 +messages-send-by-webhook 兼容路径、暂时不能迁移统一身份入口时使用"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +messages-send-by-webhook --token <token> --title \"告警\" --content \"CPU 超 90%\" --at-all"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "token", Type: shortcut.FlagString, Desc: "Webhook token", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "消息标题", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "消息正文", Required: true, Aliases: []string{"text"}},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@ 所有人"},
		{Name: "at-mobiles", Type: shortcut.FlagStringSlice, Desc: "@ 的手机号列表"},
		{Name: "at-users", Type: shortcut.FlagStringSlice, Desc: "@ 的 userId 列表"},
	},
	Tips: []string{`dws chat +messages-send-by-webhook --token <token> --title "告警" --content "CPU 超 90%" --at-all`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"robotToken": rt.Str("token"),
			"title":      rt.Str("title"),
			"text":       rt.StrFirst("text", "content"),
		}
		if rt.Bool("at-all") {
			params["isAtAll"] = true
		}
		if v := rt.StrSlice("at-mobiles"); len(v) > 0 {
			params["atMobiles"] = v
		}
		if v := rt.StrSlice("at-users"); len(v) > 0 {
			params["atUserIds"] = v
		}
		return rt.CallMCP("send_message_by_custom_robot", params)
	},
}

// MessagesRecall recalls a message sent by the current user (recall_message, im).
var MessagesRecall = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-recall",
	Product:     "im",
	Description: "撤回当前用户发送的消息",
	Intent:      "当你想撤回当前用户刚发出的某条消息时使用；会实际撤回消息。推荐同时传会话 openConversationId 和消息 openMessageId；若只传一个消息 ID，CLI 会先只读查询消息详情并补齐会话 ID。兼容 --message-id/--message-ids 的单值写法。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId；省略时从消息详情解析"},
		{Name: "group", Type: shortcut.FlagString, Desc: "--conversation-id 的兼容别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--conversation-id 的兼容别名", Hidden: true},
		{Name: "chat", Type: shortcut.FlagString, Desc: "--conversation-id 的兼容别名", Hidden: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMessageId；一次只能撤回一个消息 ID；--message-ids 仅接受单值"},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "--msg-id 的兼容别名", Hidden: true},
		{Name: "message-ids", Type: shortcut.FlagStringSlice, Desc: "--msg-id 的兼容单值别名；不支持批量撤回", Hidden: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"msg-id", "message-id", "message-ids"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"msg-id"}, Description: "一次只能撤回一个消息 ID；--message-ids 仅接受单值"},
	},
	Tips: []string{`dws chat +messages-recall --conversation-id <openConversationId> --msg-id <openMessageId>`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		messageIDs := uniqueShortcutStrings(append(
			[]string{rt.StrFirst("msg-id", "message-id")},
			rt.StrSlice("message-ids")...,
		))
		if len(messageIDs) != 1 {
			return apperrors.NewValidation("撤回一次只接受一个消息 ID；请通过 --msg-id 或单值 --message-ids 传入")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		messageIDs := uniqueShortcutStrings(append(
			[]string{rt.StrFirst("msg-id", "message-id")},
			rt.StrSlice("message-ids")...,
		))
		messageID := messageIDs[0]
		conversationID := strings.TrimSpace(rt.StrFirst("conversation-id", "group", "id", "chat"))
		if conversationID == "" {
			data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{"openMsgIds": []string{messageID}})
			if err != nil {
				return err
			}
			messages := listMessagesResolveMaps(data)
			if len(messages) == 0 {
				return apperrors.NewValidation("无法根据消息 ID 查询到会话；请补充 --conversation-id")
			}
			conversationID = strings.TrimSpace(fmt.Sprint(chatmsg.ConversationID(messages[0])))
			if conversationID == "" || conversationID == "<nil>" {
				return apperrors.NewValidation("消息详情未返回会话 ID；请补充 --conversation-id")
			}
		}
		return rt.CallMCP("recall_message", map[string]any{
			"openConversationId": conversationID,
			"openMessageId":      messageID,
		})
	},
}

// MessagesRecallByBot recalls a robot group message (recall_robot_group_message, bot).
var MessagesRecallByBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-recall-by-bot",
	Product:     "bot",
	Description: "机器人撤回群消息",
	Intent:      "当你要撤回机器人此前发到群里的消息时使用；会实际撤回机器人群消息，需传 robotCode、群 openConversationId 和发送时返回的 processQueryKey 列表。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code", Required: true},
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "keys", Type: shortcut.FlagStringSlice, Desc: "发送时返回的 processQueryKey 列表", Required: true},
	},
	Tips: []string{`dws chat +messages-recall-by-bot --robot-code <robotCode> --group <openConversationId> --keys key1,key2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("recall_robot_group_message", map[string]any{
			"robotCode":          rt.Str("robot-code"),
			"openConversationId": rt.Str("group"),
			"processQueryKeys":   rt.StrSlice("keys"),
		})
	},
}

// MessagesBatchRecallByBot recalls robot single-chat messages (batch_recall_robot_users_msg, bot).
var MessagesBatchRecallByBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-batch-recall-by-bot",
	Product:     "bot",
	Description: "机器人撤回单聊消息",
	Intent:      "当你要批量撤回机器人此前发出的单聊消息时使用；会实际撤回机器人单聊消息，需传 robotCode 和发送时返回的 processQueryKey 列表。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code", Required: true},
		{Name: "keys", Type: shortcut.FlagStringSlice, Desc: "发送时返回的 processQueryKey 列表", Required: true},
	},
	Tips: []string{`dws chat +messages-batch-recall-by-bot --robot-code <robotCode> --keys key1,key2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("batch_recall_robot_users_msg", map[string]any{
			"robotCode":        rt.Str("robot-code"),
			"processQueryKeys": rt.StrSlice("keys"),
		})
	},
}

// MessagesList pulls messages of a group (list_conversation_message_v2, chat server).
var MessagesList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-list",
	Description: "拉取群聊会话消息",
	Intent:      "当你想按时间拉取某个群的历史聊天记录（做回顾或分析）时使用；只读，需传群 openConversationId 和起始时间，--forward 控制往新还是往旧方向翻。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"", Required: true},
		{Name: "forward", Type: shortcut.FlagBool, Default: "true", Desc: "true=从给定时间往现在拉，false=往以前拉"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量；显式页大小必须大于 0"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "--limit 的旧版别名；显式页大小必须大于 0", Hidden: true},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出消息 reaction（默认输出）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "conversation-id", "id"}},
	},
	Tips: []string{`dws chat +messages-list --group <openConversationId> --time "2025-03-01 00:00:00"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		group := rt.StrFirst("group", "conversation-id", "id")
		params := map[string]any{
			"openconversation_id": group,
			"time":                rt.Str("time"),
			"forward":             rt.Bool("forward"),
		}
		if limit := rt.IntFirst("limit", "size"); limit > 0 {
			params["limit"] = limit
		}
		data, err := rt.CallMCPData("chat", "list_conversation_message_v2", params)
		if err != nil {
			return err
		}
		messages := listMessagesProjectWithReactions(data, !rt.Bool("no-reactions"))
		payload := map[string]any{"count": len(messages), "messages": messages}
		direction := "older"
		if rt.Bool("forward") {
			direction = "newer"
		}
		chatmsg.ApplyMessagePagination(payload, data, listMessagesResolveMaps(data), direction)
		return rt.Output(payload)
	},
}

// listMessagesProject reshapes a raw conversation-message list response into a
// clean {messageId, senderId, msgType, createTime, text} list — output-projection
// clean output projection. Both the list container and the per-item field names are
// probed defensively across candidate keys, so an empty or unexpected shape
// yields an empty list rather than a crash or fabricated data.
//
// Text goes through the shared chatmsg projection so card/auto-reply JSON is
// rendered readable and encrypted ciphertext is marked, and forwarded chat
// records ("聊天记录") expand their nested messages under "forwarded" instead of
// collapsing to a "[卡片]" summary.
func listMessagesProject(data map[string]any) []map[string]any {
	return listMessagesProjectWithReactions(data, true)
}

func listMessagesProjectWithReactions(data map[string]any, includeReactions bool) []map[string]any {
	raw := listMessagesResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row := listMessageProjectOneWithReactions(m, includeReactions); len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listMessageProjectOne projects a single message into the native
// {messageId, senderId, msgType, createTime, text(, forwarded)} shape, reused
// recursively for forwarded chat records.
func listMessageProjectOne(m map[string]any) map[string]any {
	return listMessageProjectOneWithReactions(m, true)
}

func listMessageProjectOneWithReactions(m map[string]any, includeReactions bool) map[string]any {
	row := chatmsg.ProjectMessageV1(m, includeReactions)
	// The established mget/list projection omits absent scalar fields; keep
	// that wire behavior even though the shared chat/search view retains them.
	for _, key := range []string{"sender", "text", "createTime"} {
		if row[key] == nil {
			delete(row, key)
		}
	}
	// Keep the historical mget/list msgType alias while adding the canonical
	// messageType field from MessageViewV1.
	if messageType := chatmsg.MessageType(m); messageType != nil {
		row["msgType"] = messageType
	}
	if forwarded := chatmsg.Forwarded(m, func(item map[string]any) map[string]any {
		return listMessageProjectOneWithReactions(item, includeReactions)
	}); len(forwarded) > 0 {
		row["forwarded"] = forwarded
	}
	return row
}

// listMessagesResolveList locates the list payload, tolerating a bare top-level
// array container or nesting one level deeper under a common envelope key.
func listMessagesResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "messages", "messageList", "records"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "messages", "messageList", "records", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

func listMessagesResolveMaps(data map[string]any) []map[string]any {
	raw := listMessagesResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if message, ok := item.(map[string]any); ok {
			out = append(out, message)
		}
	}
	return out
}

// MessagesListDirect pulls messages of a single chat (list_individual_chat_message, chat server).
var MessagesListDirect = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-list-direct",
	Description: "拉取单聊会话消息",
	Intent:      "当你想按时间拉取与某人单聊的历史消息时使用；只读，需传对方 userId 或 openDingTalkId 及起始时间，--forward 控制翻页方向。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_list_direct",
			CanonicalPath:  "chat.shortcut_messages_list_direct",
			CLIPath:        "chat +messages-list-direct",
			PrimaryCLIPath: "chat +messages-list-direct",
		},
		Description: "拉取单聊会话消息",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "拉取单聊会话消息",
			UseWhen:      []string{"当你想按时间拉取与某人单聊的历史消息时使用；只读，需传对方 userId 或 openDingTalkId 及起始时间，--forward 控制翻页方向。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +messages-list-direct --user <userId> --time \"2025-03-01 00:00:00\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "user", Type: shortcut.FlagString, Desc: "对方 userId（与 --open-dingtalk-id 二选一）"},
		{Name: "open-dingtalk-id", Type: shortcut.FlagString, Desc: "对方 openDingTalkId（与 --user 二选一）"},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"", Required: true},
		{Name: "forward", Type: shortcut.FlagBool, Default: "true", Desc: "true=往现在拉，false=往以前拉"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量；显式页大小必须大于 0"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "--limit 的旧版别名；显式页大小必须大于 0", Hidden: true},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "沿毫秒级 nextCursor 自动读取全部单聊消息；--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出消息 reaction（默认输出）"},
	},
	Tips: []string{
		`dws chat +messages-list-direct --user <userId> --time "2025-03-01 00:00:00"`,
		`dws chat +messages-list-direct --open-dingtalk-id <openDingTalkId> --time "2025-03-01 00:00:00" --forward=false --page-all`,
	},
	Execute: executeMessagesListDirect,
}

func validateMessagesListDirectPagination(rt *shortcut.RuntimeContext) error {
	// This existing command shipped without formal Schema constraints. Keep its
	// published constraint set stable and validate the new pagination options at
	// runtime so adding --page-all remains backwards compatible for consumers.
	for _, name := range []string{"limit", "size"} {
		if rt.Changed(name) && rt.Int(name) <= 0 {
			return apperrors.NewValidation("--" + name + " 必须大于 0")
		}
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > directMessagesHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	return nil
}

func executeMessagesListDirect(rt *shortcut.RuntimeContext) error {
	if err := validateMessagesListDirectPagination(rt); err != nil {
		return err
	}
	params := map[string]any{
		"time":    rt.Str("time"),
		"forward": rt.Bool("forward"),
	}
	switch {
	case rt.Str("open-dingtalk-id") != "":
		if err := targetresolver.ValidateExplicitOpenDingTalkID("--open-dingtalk-id", rt.Str("open-dingtalk-id")); err != nil {
			return err
		}
		params["openDingTalkId"] = rt.Str("open-dingtalk-id")
	case rt.Str("user") != "":
		params["userId"] = rt.Str("user")
	default:
		return fmt.Errorf("--user 或 --open-dingtalk-id 必填其一")
	}
	if limit := rt.IntFirst("limit", "size"); limit > 0 {
		params["limit"] = limit
	}
	if rt.Bool("page-all") {
		payload, err := readAllDirectMessages(rt, params)
		if outputErr := rt.Output(payload); outputErr != nil {
			return outputErr
		}
		return err
	}
	data, err := rt.CallMCPData("chat", "list_individual_chat_message", params)
	if err != nil {
		return err
	}
	messages := listMessagesProjectWithReactions(data, !rt.Bool("no-reactions"))
	payload := map[string]any{"count": len(messages), "messages": messages}
	direction := "older"
	if rt.Bool("forward") {
		direction = "newer"
	}
	chatmsg.ApplyMessagePagination(payload, data, listMessagesResolveMaps(data), direction)
	if payload["complete"] == true {
		payload["stopReason"] = "source_complete"
	} else {
		payload["stopReason"] = "single_page"
	}
	return rt.Output(payload)
}

func readAllDirectMessages(rt *shortcut.RuntimeContext, params map[string]any) (map[string]any, error) {
	pageLimit := rt.Int("page-limit")
	direction := "older"
	if rt.Bool("forward") {
		direction = "newer"
	}
	seenCursors := map[string]bool{}
	seenMessages := map[string]bool{}
	allItems := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	complete := false
	hasMore := false
	stopReason := "source_complete"
	truncatedByPageLimit := false
	var nextPage map[string]any

	for pagesFetched < pageLimit {
		data, err := rt.CallMCPData("chat", "list_individual_chat_message", params)
		if err != nil {
			if pagesFetched == 0 {
				return nil, err
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "error": err.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		pageItems := listMessagesResolveMaps(data)
		for _, item := range pageItems {
			id := chatmsg.StableMessageID(item)
			if id != "" && seenMessages[id] {
				continue
			}
			if id != "" {
				seenMessages[id] = true
			}
			allItems = append(allItems, item)
		}

		page := chatmsg.Pagination(data)
		pageHasMore, known := page["hasMore"].(bool)
		if !known {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "单聊消息下层未返回可靠的 hasMore，无法证明结果完整",
			})
			stopReason = "pagination_error"
			break
		}
		hasMore = pageHasMore
		if !hasMore {
			complete = true
			nextPage = nil
			stopReason = "source_complete"
			break
		}
		if len(pageItems) == 0 {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "单聊消息下层返回 hasMore=true 但当前页没有消息",
			})
			stopReason = "pagination_error"
			break
		}
		cursorKey, boundary, cursorErr := directMessageCursorBoundary(page["nextCursor"])
		if cursorErr != nil {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "单聊消息下层返回 hasMore=true，但 nextCursor 无效: " + cursorErr.Error(),
			})
			stopReason = "pagination_error"
			break
		}
		if seenCursors[cursorKey] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "单聊消息毫秒 nextCursor 停滞",
			})
			stopReason = "pagination_error"
			break
		}
		seenCursors[cursorKey] = true
		nextPage = map[string]any{
			"direction": direction, "time": boundary, "nextCursor": page["nextCursor"],
		}
		params["time"] = boundary
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit {
		truncatedByPageLimit = true
		stopReason = "page_limit"
	}

	messages := make([]map[string]any, 0, len(allItems))
	for _, item := range allItems {
		messages = append(messages, listMessageProjectOneWithReactions(item, !rt.Bool("no-reactions")))
	}
	payload := chatmsg.NewMessageListPayload(messages)
	payload["pagesFetched"] = pagesFetched
	payload["paginationKnown"] = true
	payload["complete"] = complete && len(failures) == 0
	payload["hasMore"] = hasMore
	payload["stopReason"] = stopReason
	payload["truncatedByPageLimit"] = truncatedByPageLimit
	chatmsg.ApplyTruncation(payload)
	payload["failedCount"] = len(failures)
	payload["failures"] = failures
	payload["partial"] = len(failures) > 0 && len(messages) > 0
	if hasMore && nextPage != nil {
		payload["nextPage"] = nextPage
	}
	if len(failures) == 0 {
		return payload, nil
	}
	return payload, apperrors.NewAPI(
		fmt.Sprintf("单聊消息分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
		apperrors.WithOperation("chat/list_individual_chat_message"),
		apperrors.WithReason("messages_list_direct_incomplete"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(true),
		apperrors.WithHint("请根据 failures 和 nextPage 重试"),
	)
}

func directMessageCursorBoundary(value any) (string, string, error) {
	var millis int64
	switch typed := value.(type) {
	case int:
		millis = int64(typed)
	case int32:
		millis = int64(typed)
	case int64:
		millis = typed
	case float32:
		asFloat := float64(typed)
		if math.IsNaN(asFloat) || math.IsInf(asFloat, 0) || asFloat <= 0 || math.Trunc(asFloat) != asFloat || asFloat > math.MaxInt64 {
			return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
		}
		millis = int64(asFloat)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed <= 0 || math.Trunc(typed) != typed || typed > math.MaxInt64 {
			return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
		}
		millis = int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
		}
		millis = parsed
	default:
		return "", "", fmt.Errorf("缺少毫秒级分页游标")
	}
	if millis <= 0 {
		return "", "", fmt.Errorf("必须是正整数毫秒时间戳")
	}
	key := strconv.FormatInt(millis, 10)
	boundary := time.UnixMilli(millis).UTC().Format(time.RFC3339Nano)
	return key, boundary, nil
}

// MessagesListTopicReplies pulls topic replies (list_topic_replies, chat server).
// MessagesListAll pulls all messages in a time range (search_messages_by_time_range, chat server).
// MessagesListFocused pulls messages from specially-followed people (list_special_focus_messages, chat server).
// MessagesListUnreadConversations lists conversations with unread messages
// (unread_message_conversation_list, chat server).
var MessagesListUnreadConversations = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-list-unread-conversations",
	Description: "获取有未读消息的会话列表",
	Intent:      "当你想快速定位哪些会话还有未读消息时使用；只读返回有未读的会话列表，可用 --exclude-muted 排除已免打扰会话。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_list_unread_conversations",
			CanonicalPath:  "chat.shortcut_messages_list_unread_conversations",
			CLIPath:        "chat +messages-list-unread-conversations",
			PrimaryCLIPath: "chat +messages-list-unread-conversations",
		},
		Description: "获取有未读消息的会话列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取有未读消息的会话列表",
			UseWhen:      []string{"当你想快速定位哪些会话还有未读消息时使用；只读返回有未读的会话列表，可用 --exclude-muted 排除已免打扰会话。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +messages-list-unread-conversations --count 20"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "count", Type: shortcut.FlagInt, Desc: "返回的会话条数"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已免打扰会话"},
	},
	Tips: []string{`dws chat +messages-list-unread-conversations --count 20`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Int("count") > 0 {
			params["count"] = rt.Int("count")
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("chat", "unread_message_conversation_list", params)
		if err != nil {
			return err
		}
		conversations := listUnreadConversationsProject(data)
		return rt.Output(map[string]any{"count": len(conversations), "conversations": conversations})
	},
}

// listUnreadConversationsProject reshapes the raw unread_message_conversation_list
// response into a clean {conversationId, title, unreadCount, lastMessageTime} list
// — clean output projection. Both the list container and per-item
// field names are probed defensively across candidate keys, so an empty or
// unexpected shape yields an empty list rather than a crash or fabricated data.
func listUnreadConversationsProject(data map[string]any) []map[string]any {
	raw := listUnreadConversationsResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := listUnreadConversationsFirst(m, "openConversationId", "conversationId", "openConvId", "cid"); ok {
			row["conversationId"] = v
		}
		if v, ok := listUnreadConversationsFirst(m, "title", "conversationTitle", "name"); ok {
			row["title"] = v
		}
		if v, ok := listUnreadConversationsFirst(m, "unreadCount", "unread", "unReadCount", "count"); ok {
			row["unreadCount"] = v
		}
		if v, ok := listUnreadConversationsFirst(m, "lastMessageTime", "lastMsgTime", "latestMessageTime", "updateTime"); ok {
			row["lastMessageTime"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listUnreadConversationsResolveList locates the list payload, tolerating a bare
// top-level array container or nesting one level deeper under a common envelope.
func listUnreadConversationsResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "conversations", "conversationList"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "conversations", "conversationList", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// listUnreadConversationsFirst returns the first present candidate key's value.
func listUnreadConversationsFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// MessagesMget batch-queries messages by id (list_messages_by_ids, im).
var MessagesMget = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-mget",
	Product:     "im",
	Description: "根据消息 ID 批量查询消息（最多 50 条）",
	Intent:      "当你已有一批消息 openMsgId、需要批量取回完整详情、reaction 和可执行资源引用时使用；一次最多 50 条。--download-resources 可把所有可识别 mediaId/fileId 安全下载到工作目录内，并逐资源返回成功/失败 ledger；本地下载路径受限于工作目录、默认不覆盖同名文件，按既有安全下载约定无需交互确认。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_mget",
			CanonicalPath:  "chat.shortcut_messages_mget",
			CLIPath:        "chat +messages-mget",
			PrimaryCLIPath: "chat +messages-mget",
		},
		Description: "根据消息 ID 批量查询消息（最多 50 条）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "根据消息 ID 批量查询消息（最多 50 条）",
			UseWhen:      []string{"当你已有一批消息 openMsgId、需要批量取回完整详情、reaction 和可执行资源引用时使用；一次最多 50 条。--download-resources 可把所有可识别 mediaId/fileId 安全下载到工作目录内，并逐资源返回成功/失败 ledger；本地下载路径受限于工作目录、默认不覆盖同名文件，按既有安全下载约定无需交互确认。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +messages-mget --msg-ids msgId1,msgId2"},
		},
	},
	Flags: append([]shortcut.Flag{
		{Name: "msg-ids", Type: shortcut.FlagStringSlice, Desc: "消息 openMsgId 列表；--msg-ids 去重后必须包含 1-50 条消息 ID", Required: true},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出消息 reaction（默认输出）"},
	}, MessageResourceDownloadFlags()...),
	Constraints: append([]shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"msg-ids"},
			Description: "--msg-ids 去重后必须包含 1-50 条消息 ID",
		},
	}, MessageResourceDownloadConstraints()...),
	Tips: []string{`dws chat +messages-mget --msg-ids msgId1,msgId2`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		ids := uniqueShortcutStrings(rt.StrSlice("msg-ids"))
		if len(ids) < 1 || len(ids) > 50 {
			return fmt.Errorf("--msg-ids 去重后必须包含 1-50 条消息 ID，当前 %d 条", len(ids))
		}
		return ValidateMessageResourceDownload(rt)
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		ids := uniqueShortcutStrings(rt.StrSlice("msg-ids"))
		data, err := rt.CallMCPData("im", "list_messages_by_ids", map[string]any{"openMsgIds": ids})
		if err != nil {
			return err
		}
		rawMessages := listMessagesResolveMaps(data)
		messages := listMessagesProjectWithReactions(data, !rt.Bool("no-reactions"))
		found := map[string]bool{}
		for _, message := range rawMessages {
			if id := strings.TrimSpace(fmt.Sprint(chatmsg.MessageID(message))); id != "" && id != "<nil>" {
				found[id] = true
			}
		}
		notFound := make([]string, 0)
		failures := make([]map[string]any, 0)
		for _, id := range ids {
			if !found[id] {
				notFound = append(notFound, id)
				failures = append(failures, map[string]any{
					"stage":     "mget",
					"messageId": id,
					"error":     "下层未返回该消息",
				})
			}
		}
		payload := map[string]any{
			"contractVersion":    chatmsg.MessageListContractVersion,
			"requestedCount":     len(ids),
			"foundCount":         len(ids) - len(notFound),
			"notFoundCount":      len(notFound),
			"notFoundMessageIds": notFound,
			"messages":           messages,
			"complete":           len(notFound) == 0,
			"hasMore":            false,
			"nextCursor":         "",
			"paginationKnown":    true,
			"pagesFetched":       1,
			"enrichedCount":      0,
			"failedCount":        len(failures),
			"failures":           failures,
		}
		if rt.Bool("download-resources") {
			AttachMessageResourceDownloads(payload, DownloadMessageResources(rt, rawMessages, ""))
		}
		return rt.Output(payload)
	},
}

// MessageResourceDownloadFlags returns the common opt-in resource workflow used
// by message list, search, mget, @me and thread-reading Shortcuts.
func MessageResourceDownloadFlags() []shortcut.Flag {
	return []shortcut.Flag{
		{Name: "download-resources", Type: shortcut.FlagBool, Desc: "自动下载消息中的全部可识别 mediaId/fileId 资源"},
		{Name: "output-dir", Type: shortcut.FlagString, Default: "./downloads", Desc: "资源输出目录；必须是工作目录内的相对路径，禁止绝对路径和 .. 逃逸"},
		{Name: "overwrite", Type: shortcut.FlagBool, Desc: "允许覆盖工作目录内已存在的本地输出文件（默认拒绝）"},
	}
}

// MessageResourceDownloadConstraints publishes the shared safe-output rule.
func MessageResourceDownloadConstraints() []shortcut.Constraint {
	return []shortcut.Constraint{{
		Kind:        shortcut.ConstraintCustom,
		Flags:       []string{"output-dir"},
		Description: "--output-dir 必须是工作目录内的相对路径，不允许绝对路径或 .. 逃逸",
	}}
}

// ValidateMessageResourceDownload validates the output path only when the
// caller opts into local writes.
func ValidateMessageResourceDownload(rt *shortcut.RuntimeContext) error {
	if !rt.Bool("download-resources") {
		return nil
	}
	return validateResourceDownloadOutputFlag(rt.Str("output-dir"), "--output-dir")
}

// DownloadMessageResources downloads every unique message resource reference
// and returns a per-resource success/failure ledger. A fallback conversation
// ID lets group/thread list commands supply context when a mediaId item's lower
// response omits it; fileId resources route through the existing drive leaf.
func DownloadMessageResources(
	rt *shortcut.RuntimeContext,
	messages []map[string]any,
	fallbackConversationID string,
) map[string]any {
	resources := make([]map[string]any, 0)
	for _, message := range messages {
		resources = append(resources, chatmsg.ResourcesDeep(message)...)
	}
	discoveredCount := len(resources)
	uniqueResources := make([]map[string]any, 0, len(resources))
	seen := map[string]bool{}
	for _, resource := range resources {
		resourceType := strings.TrimSpace(fmt.Sprint(resource["type"]))
		if canonicalType, ok := canonicalMessageResourceType(resourceType); ok {
			resourceType = canonicalType
		}
		resourceID := strings.TrimSpace(fmt.Sprint(resource["resourceId"]))
		download, _ := resource["download"].(map[string]any)
		arguments, _ := download["arguments"].(map[string]any)
		messageID := strings.TrimSpace(fmt.Sprint(arguments["message-id"]))
		if messageID == "<nil>" {
			messageID = ""
		}
		key := strings.ToLower(resourceType) + "\x00" + resourceID
		if strings.EqualFold(resourceType, "mediaId") {
			conversationID := strings.TrimSpace(fmt.Sprint(arguments["open-conversation-id"]))
			key += "\x00" + messageID + "\x00" + conversationID
		}
		if resourceID != "" && resourceID != "<nil>" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		uniqueResources = append(uniqueResources, resource)
	}
	resources = uniqueResources
	if rt.DryRun() {
		return map[string]any{
			"dryRun":            true,
			"discoveredCount":   discoveredCount,
			"requestedCount":    len(resources),
			"deduplicatedCount": discoveredCount - len(resources),
			"resources":         resources,
		}
	}
	if len(resources) == 0 {
		return map[string]any{
			"ok":                true,
			"partial":           false,
			"discoveredCount":   discoveredCount,
			"requestedCount":    0,
			"deduplicatedCount": discoveredCount,
			"downloadedCount":   0,
			"failedCount":       0,
			"downloads":         []map[string]any{},
			"failures":          []map[string]any{},
		}
	}

	cwd, err := resourceGetwd()
	if err != nil {
		return map[string]any{
			"ok":                false,
			"partial":           false,
			"discoveredCount":   discoveredCount,
			"requestedCount":    len(resources),
			"deduplicatedCount": discoveredCount - len(resources),
			"downloadedCount":   0,
			"failedCount":       len(resources),
			"downloads":         []map[string]any{},
			"failures": []map[string]any{{
				"stage":         "output-directory",
				"affectedCount": len(resources),
				"error":         fmt.Sprintf("读取工作目录失败: %v", err),
			}},
		}
	}
	outputDir := strings.TrimRight(rt.Str("output-dir"), `/\`)
	downloads := make([]map[string]any, 0, len(resources))
	failures := make([]map[string]any, 0)
	downloadedNames := map[string]bool{}
	for _, resource := range resources {
		resourceType := strings.TrimSpace(fmt.Sprint(resource["type"]))
		if canonicalType, ok := canonicalMessageResourceType(resourceType); ok {
			resourceType = canonicalType
		}
		resourceID := strings.TrimSpace(fmt.Sprint(resource["resourceId"]))
		download, _ := resource["download"].(map[string]any)
		arguments, _ := download["arguments"].(map[string]any)
		messageID := strings.TrimSpace(fmt.Sprint(arguments["message-id"]))
		conversationID := strings.TrimSpace(fmt.Sprint(arguments["open-conversation-id"]))
		if messageID == "<nil>" {
			messageID = ""
		}
		if conversationID == "" || conversationID == "<nil>" {
			conversationID = strings.TrimSpace(fallbackConversationID)
		}
		missingMediaContext := resourceType == "mediaId" &&
			(messageID == "" || messageID == "<nil>" ||
				conversationID == "" || conversationID == "<nil>")
		if resourceID == "" || resourceID == "<nil>" || missingMediaContext {
			failures = append(failures, map[string]any{
				"resourceType": resourceType,
				"resourceId":   resourceID,
				"messageId":    messageID,
				"error":        "资源引用缺少 resource-id，或 mediaId 缺少 message-id/open-conversation-id",
			})
			continue
		}

		data, callErr := resolveMessageResourceDownloadData(
			rt, resourceType, resourceID, messageID, conversationID)
		if callErr != nil {
			failures = append(failures, map[string]any{
				"resourceType": resourceType,
				"resourceId":   resourceID,
				"messageId":    messageID,
				"error":        callErr.Error(),
			})
			continue
		}
		resourceURL, headers, infoErr := resourceDownloadInfo(data)
		if infoErr != nil {
			failures = append(failures, map[string]any{
				"resourceType": resourceType,
				"resourceId":   resourceID,
				"messageId":    messageID,
				"error":        infoErr.Error(),
			})
			continue
		}
		preferredName := resourceDownloadPreferredName(data)
		if preferredName == "" {
			preferredName, _ = resource["name"].(string)
			preferredName = strings.TrimSpace(preferredName)
		}
		filename := resourceDownloadFilename(resourceURL, preferredName)
		filename = disambiguateResourceDownloadFilename(filename, downloadedNames)
		output := filepath.Join(outputDir, filename)
		destPath, relativePath, pathErr := resolveResourceDownloadPath(
			cwd,
			output,
			resourceURL,
			rt.Bool("overwrite"),
			preferredName,
		)
		if pathErr != nil {
			failures = append(failures, map[string]any{
				"resourceType": resourceType,
				"resourceId":   resourceID,
				"messageId":    messageID,
				"error":        pathErr.Error(),
			})
			continue
		}
		size, downloadErr := resourceDownload(
			rt.Command().Context(), nil, resourceURL, headers, destPath, rt.Bool("overwrite"))
		if downloadErr != nil {
			failures = append(failures, map[string]any{
				"resourceType": resourceType,
				"resourceId":   resourceID,
				"messageId":    messageID,
				"error":        downloadErr.Error(),
			})
			continue
		}
		downloadedNames[strings.ToLower(filepath.Base(relativePath))] = true
		downloads = append(downloads, map[string]any{
			"resourceType": resourceType,
			"resourceId":   resourceID,
			"messageId":    messageID,
			"localPath":    filepath.ToSlash(relativePath),
			"sizeBytes":    size,
		})
	}
	return map[string]any{
		"ok":                len(failures) == 0,
		"partial":           len(downloads) > 0 && len(failures) > 0,
		"discoveredCount":   discoveredCount,
		"requestedCount":    len(resources),
		"deduplicatedCount": discoveredCount - len(resources),
		"downloadedCount":   len(downloads),
		"failedCount":       len(failures),
		"downloads":         downloads,
		"failures":          failures,
	}
}

// AttachMessageResourceDownloads publishes the download ledger and folds any
// resource failure into the task-level completeness contract without dropping
// successfully read messages or downloaded files.
func AttachMessageResourceDownloads(payload, ledger map[string]any) {
	payload["resourceDownloads"] = ledger
	failed := messageLedgerInt(ledger["failedCount"])
	if failed == 0 {
		return
	}
	payload["complete"] = false
	payload["failedCount"] = messageLedgerInt(payload["failedCount"]) + failed
	taskFailures, _ := payload["failures"].([]map[string]any)
	resourceFailures, _ := ledger["failures"].([]map[string]any)
	for _, failure := range resourceFailures {
		row := make(map[string]any, len(failure)+1)
		row["stage"] = "resource-download"
		for key, value := range failure {
			row[key] = value
		}
		taskFailures = append(taskFailures, row)
	}
	payload["failures"] = taskFailures
}

func messageLedgerInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func disambiguateResourceDownloadFilename(filename string, used map[string]bool) string {
	if !used[strings.ToLower(filename)] {
		return filename
	}
	extension := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	for sequence := 2; ; sequence++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, sequence, extension)
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func uniqueShortcutStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = appendUniqueShortcutString(out, value)
		}
	}
	return out
}

// MessagesQuerySendStatus queries send status of a message (query_message_send_status, im).
var MessagesQuerySendStatus = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-query-send-status",
	Aliases:     []string{"+messages-send-status"},
	Product:     "im",
	Description: "查询消息投递状态并衔接后续消息操作",
	Intent:      "当你发消息后拿到 openTaskId、想确认投递结果，或后续 edit/recall/read-status 需要取得 openMessageId 和 openConversationId 时使用；openTaskId 不是消息 ID。结果会保留下层响应，并追加版本化 messageRef 与结构化 nextActions。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_query_send_status",
			CanonicalPath:  "chat.shortcut_messages_query_send_status",
			CLIPath:        "chat +messages-query-send-status",
			PrimaryCLIPath: "chat +messages-query-send-status",
			Aliases:        []string{"chat +messages-send-status"},
		},
		Description: "查询消息投递状态并衔接后续消息操作",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询消息投递状态并衔接后续消息操作",
			UseWhen:      []string{"当你发消息后拿到 openTaskId、想确认投递结果，或后续 edit/recall/read-status 需要取得 openMessageId 和 openConversationId 时使用；openTaskId 不是消息 ID。结果会保留下层响应，并追加版本化 messageRef 与结构化 nextActions。"},
			AvoidWhen:    []string{"没有 openTaskId、已经有消息 ID，或只需查历史消息内容时不要使用"},
			Examples:     []string{"dws chat +messages-query-send-status --open-task-id <openTaskId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "open-task-id", Type: shortcut.FlagString, Desc: "发送消息时返回的 openTaskId", Required: true},
	},
	Tips: []string{`dws chat +messages-query-send-status --open-task-id <openTaskId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		taskID := rt.Str("open-task-id")
		data, err := rt.CallMCPData("im", "query_message_send_status", map[string]any{"openTaskId": taskID})
		if err != nil {
			return err
		}
		return rt.Output(chatmsg.ProjectMessageSendStatus(data, taskID))
	},
}

// MessagesReadStatus queries read/unread status of a message (query_msg_read_status, im).
var MessagesReadStatus = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-read-status",
	Product:     "im",
	Description: "查询消息的已读/未读状态",
	Intent:      "当你想知道自己发出的某条消息有哪些人已读/未读时使用；只读，需传会话 openConversationId 和该消息 openMessageId，可指定目标成员列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_read_status",
			CanonicalPath:  "chat.shortcut_messages_read_status",
			CLIPath:        "chat +messages-read-status",
			PrimaryCLIPath: "chat +messages-read-status",
		},
		Description: "查询消息的已读/未读状态",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询消息的已读/未读状态",
			UseWhen:      []string{"当你想知道自己发出的某条消息有哪些人已读/未读时使用；只读，需传会话 openConversationId 和该消息 openMessageId，可指定目标成员列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +messages-read-status --conversation-id <openConversationId> --message-id <openMessageId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId"},
		{Name: "group", Type: shortcut.FlagString, Desc: "--conversation-id 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--conversation-id 的别名", Hidden: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "消息 openMessageId（当前用户发送的消息）", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "目标 userId 或 openDingTalkId 列表（不传返回全部接收者）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"conversation-id", "group", "id"}},
	},
	Tips: []string{`dws chat +messages-read-status --conversation-id <openConversationId> --message-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId": rt.StrFirst("conversation-id", "group", "id"),
			"openMessageId":      rt.Str("message-id"),
		}
		if v := rt.StrSlice("users"); len(v) > 0 {
			userIDs, openIDs := splitIDs(v)
			if len(userIDs) > 0 {
				params["targetUserIds"] = userIDs
			}
			if len(openIDs) > 0 {
				params["targetOpenDingTalkIds"] = openIDs
			}
		}
		return rt.CallMCP("query_msg_read_status", params)
	},
}

// MessagesAddEmoji adds an emoji reaction (add_emoji_reaction, im).
var MessagesAddEmoji = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-add-emoji",
	Product:     "im",
	Description: "对消息添加 emoji 表情回应",
	Intent:      "当你想给某条消息点一个 emoji 表情回应时使用；会实际添加表情回应，需传会话 openConversationId、消息 openMsgId 和表情名称。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMsgId", Required: true},
		{Name: "emoji", Type: shortcut.FlagString, Desc: "emoji 表情名称", Required: true},
	},
	Tips: []string{`dws chat +messages-add-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "赞"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("add_emoji_reaction", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"openMsgId":          rt.Str("msg-id"),
			"emojiName":          rt.Str("emoji"),
		})
	},
}

// MessagesRemoveEmoji removes an emoji reaction (remove_emoji_reaction, im).
var MessagesRemoveEmoji = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-remove-emoji",
	Product:     "im",
	Description: "移除消息的 emoji 表情回应",
	Intent:      "当你想取消此前给某条消息添加的 emoji 表情回应时使用；会实际移除表情回应，需传会话 openConversationId、消息 openMsgId 和表情名称。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMsgId", Required: true},
		{Name: "emoji", Type: shortcut.FlagString, Desc: "emoji 表情名称", Required: true},
	},
	Tips: []string{`dws chat +messages-remove-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "赞"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_emoji_reaction", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"openMsgId":          rt.Str("msg-id"),
			"emojiName":          rt.Str("emoji"),
		})
	},
}

// MessagesAddTextEmotion adds a text emotion reaction (add_text_emotion, im).
var MessagesAddTextEmotion = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-add-text-emotion",
	Product:     "im",
	Description: "对消息添加文字表情回应",
	Intent:      "当你想给某条消息添加自定义文字表情回应时使用；会实际添加文字表情，需传会话、消息 openMsgId 及由 create-text-emotion 得到的 emotionId 等参数。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMsgId", Required: true},
		{Name: "emotion-id", Type: shortcut.FlagString, Desc: "表情 ID（由 create-text-emotion 获取）", Required: true},
		{Name: "emotion-name", Type: shortcut.FlagString, Desc: "表情名称", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "文字内容", Required: true},
		{Name: "background-id", Type: shortcut.FlagString, Desc: "背景 ID", Required: true},
	},
	Tips: []string{`dws chat +messages-add-text-emotion --conversation-id <openConversationId> --msg-id <openMsgId> --emotion-id <id> --emotion-name "赞" --text "nice" --background-id im_bg_5`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("add_text_emotion", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"openMsgId":          rt.Str("msg-id"),
			"emotionId":          rt.Str("emotion-id"),
			"emotionName":        rt.Str("emotion-name"),
			"text":               rt.Str("text"),
			"backgroundId":       rt.Str("background-id"),
		})
	},
}

// MessagesRemoveTextEmotion removes a text emotion reaction (remove_text_emotion, im).
var MessagesRemoveTextEmotion = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-remove-text-emotion",
	Product:     "im",
	Description: "移除消息的文字表情回应",
	Intent:      "当你想移除此前给某条消息添加的文字表情回应时使用；会实际移除文字表情，需传会话、消息 openMsgId 及对应的表情参数。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMsgId", Required: true},
		{Name: "emotion-id", Type: shortcut.FlagString, Desc: "表情 ID", Required: true},
		{Name: "emotion-name", Type: shortcut.FlagString, Desc: "表情名称", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "文字内容", Required: true},
		{Name: "background-id", Type: shortcut.FlagString, Desc: "背景 ID", Required: true},
	},
	Tips: []string{`dws chat +messages-remove-text-emotion --conversation-id <openConversationId> --msg-id <openMsgId> --emotion-id <id> --emotion-name "赞" --text "nice" --background-id im_bg_5`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_text_emotion", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"openMsgId":          rt.Str("msg-id"),
			"emotionId":          rt.Str("emotion-id"),
			"emotionName":        rt.Str("emotion-name"),
			"text":               rt.Str("text"),
			"backgroundId":       rt.Str("background-id"),
		})
	},
}

// MessagesCreateTextEmotion creates a text emotion template (create_text_emotion, im).
var MessagesCreateTextEmotion = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-create-text-emotion",
	Product:     "im",
	Description: "创建文字表情（获取 emotionId）",
	Intent:      "当你要先创建一个文字表情模板（拿到 emotionId 供 add-text-emotion 使用）时使用；会实际创建文字表情，需传表情名称和文字内容。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "emotion-name", Type: shortcut.FlagString, Desc: "表情名称", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "文字内容", Required: true},
		{Name: "background-id", Type: shortcut.FlagString, Desc: "背景 ID（可选）"},
	},
	Tips: []string{`dws chat +messages-create-text-emotion --emotion-name "赞" --text "nice"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"emotionName": rt.Str("emotion-name"),
			"text":        rt.Str("text"),
		}
		if rt.Changed("background-id") {
			params["backgroundId"] = rt.Str("background-id")
		}
		return rt.CallMCP("create_text_emotion", params)
	},
}

// MessagesSendCard creates and optionally completes a streaming card by
// composing create_and_send_card with update_streaming_card.
var MessagesSendCard = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-send-card",
	Product:     "im",
	Description: "创建流式卡片，可在同一次调用中写入内容并结束；群聊创建时可 @成员或 @所有人",
	Intent:      "当你要发送一张流式文本卡片时使用；群 openConversationId、单聊 userId、单聊 openDingTalkId 严格三选一，分别使用 --group、--receiver、--receiver-open-dingtalk-id。群聊可用 --at-open-dingtalk-ids 或 --at-all 在创建卡片时设置 @对象；同时传 --content 时，Runtime 会把创建响应中的 atTag 自动加在正文前，后续更新不重复传递 @参数。--receiver 始终按 userId 通过通讯录关键词搜索做精确匹配，即使值以 D/d 开头也不会猜成 openDingTalkId；已有 openDingTalkId 时必须用显式参数直传。userId 包括在 --dry-run 时也会先解析。只传目标时创建卡片并返回 bizId，供后续 messages-update-card 流式更新；同时传 --content 时会自动串联创建和更新，默认以 flowStatus=3 完成。当前只支持 streaming text，不支持 Card JSON 组件或 action callback。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_send_card",
			CanonicalPath:  "chat.shortcut_messages_send_card",
			CLIPath:        "chat +messages-send-card",
			PrimaryCLIPath: "chat +messages-send-card",
		},
		Description: "创建流式卡片，可在同一次调用中写入内容并结束；群聊创建时可 @成员或 @所有人",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed card lifecycle adapter: it can resolve a userId through contact search with exact matching, call create_and_send_card alone, or compose creation with update_streaming_card after extracting the returned bizId and atTag.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "创建流式卡片，可在同一次调用中写入内容并结束；群聊创建时可 @成员或 @所有人",
			UseWhen:      []string{"当你要发送一张流式文本卡片时使用；群 openConversationId、单聊 userId、单聊 openDingTalkId 严格三选一，分别使用 --group、--receiver、--receiver-open-dingtalk-id。群聊可用 --at-open-dingtalk-ids 或 --at-all 在创建卡片时设置 @对象；同时传 --content 时，Runtime 会把创建响应中的 atTag 自动加在正文前，后续更新不重复传递 @参数。--receiver 始终按 userId 通过通讯录关键词搜索做精确匹配，即使值以 D/d 开头也不会猜成 openDingTalkId；已有 openDingTalkId 时必须用显式参数直传。userId 包括在 --dry-run 时也会先解析。只传目标时创建卡片并返回 bizId，供后续 messages-update-card 流式更新；同时传 --content 时会自动串联创建和更新，默认以 flowStatus=3 完成。当前只支持 streaming text，不支持 Card JSON 组件或 action callback。"},
			AvoidWhen:    []string{"已有 bizId、只需要追加或更新现有卡片内容时使用 +messages-update-card"},
			Examples: []string{
				"dws chat +messages-send-card --group <openConversationId> --at-open-dingtalk-ids <openDingTalkId> --content \"任务已完成\"",
				"dws chat +messages-send-card --receiver <userId>",
			},
		},
		Parameters: []contract.ParamDecl{
			{Name: "receiver-open-dingtalk-id", Property: "receiverOpenDingTalkId"},
			{Name: "at-open-dingtalk-ids", Property: "atOpenDingTalkIds"},
			{Name: "at-all", Property: "atAll"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId（与两个单聊接收者参数互斥）；艾特参数仅支持群聊 --group"},
		{Name: "receiver", Type: shortcut.FlagString, Desc: "单聊接收者 userId（与 --group/--receiver-open-dingtalk-id 互斥）；始终通过通讯录搜索精确匹配 openDingTalkId，包括 --dry-run 和 D/d 开头的 userId"},
		{Name: "receiver-open-dingtalk-id", Type: shortcut.FlagString, Desc: "单聊接收者 openDingTalkId（与 --group/--receiver 互斥）；显式直传且不做通讯录解析"},
		{Name: "at-open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "群聊创建卡片时 @ 的 openDingTalkId 列表；仅随 create_and_send_card 发送；艾特参数仅支持群聊 --group"},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "群聊创建卡片时 @ 所有人；仅随 create_and_send_card 发送；艾特参数仅支持群聊 --group"},
		{Name: "content", Type: shortcut.FlagString, Desc: "创建后立即写入的卡片正文；群聊 @ 时 Runtime 自动前置 create 返回的 atTag；省略时仅创建并返回 bizId"},
		{Name: "flow-status", Type: shortcut.FlagInt, Default: "3", Desc: "自动更新状态：1处理中/2输入中/3完成/4执行中/5错误；--flow-status 必须在 1-5 之间，且显式指定时必须同时提供 --content"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "receiver", "receiver-open-dingtalk-id"}},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"flow-status"},
			Description: "--flow-status 必须在 1-5 之间，且显式指定时必须同时提供 --content",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"group", "at-open-dingtalk-ids", "at-all"},
			Description: "艾特参数仅支持群聊 --group",
		},
	},
	Tips: []string{
		`dws chat +messages-send-card --group <openConversationId>`,
		`dws chat +messages-send-card --group <openConversationId> --at-open-dingtalk-ids <openDingTalkId> --content "任务已完成"`,
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if receiverOpenID := rt.Str("receiver-open-dingtalk-id"); receiverOpenID != "" {
			if err := targetresolver.ValidateExplicitOpenDingTalkID("--receiver-open-dingtalk-id", receiverOpenID); err != nil {
				return err
			}
		}
		if err := validateExplicitOpenIDs("--at-open-dingtalk-ids", rt.StrSlice("at-open-dingtalk-ids")); err != nil {
			return err
		}
		if status := rt.Int("flow-status"); !validCardFlowStatus(status) {
			return fmt.Errorf("--flow-status 必须在 1-5 之间")
		}
		if rt.Changed("flow-status") && rt.Str("content") == "" {
			return fmt.Errorf("--flow-status 只有与 --content 一起使用才有意义")
		}
		if rt.Str("group") == "" && (len(uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids"))) > 0 || rt.Bool("at-all")) {
			return fmt.Errorf("--at-open-dingtalk-ids 和 --at-all 仅支持群聊 --group")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		group := rt.Str("group")
		receiver := rt.Str("receiver")
		receiverOpenID := rt.Str("receiver-open-dingtalk-id")
		params := map[string]any{}
		mentionsRequested := false
		switch {
		case group != "":
			params["openConversationId"] = group
			if atOpenIDs := uniqueShortcutStrings(rt.StrSlice("at-open-dingtalk-ids")); len(atOpenIDs) > 0 {
				params["atOpenDingTalkIds"] = atOpenIDs
				mentionsRequested = true
			}
			if rt.Bool("at-all") {
				params["atAll"] = true
				mentionsRequested = true
			}
		case receiver != "":
			openID, err := resolveUserOpenDingTalkID(rt, receiver)
			if err != nil {
				return err
			}
			params["receiverOpenDingTalkId"] = openID
		default:
			params["receiverOpenDingTalkId"] = receiverOpenID
		}
		content := rt.Str("content")
		if content == "" {
			if rt.DryRun() {
				return rt.Output(map[string]any{
					"contractVersion": chatmsg.StreamingCardContractVersion,
					"dry_run":         true,
					"executed":        false,
					"preview_kind":    "plan",
					"actionCount":     1,
					"actions": []map[string]any{{
						"tool":      "create_and_send_card",
						"arguments": params,
					}},
				})
			}
			created, err := rt.CallMCPWriteData("im", "create_and_send_card", params)
			if err != nil {
				return err
			}
			bizID := findCardBizID(created)
			if bizID == "" {
				return cardCreateMissingBizIDError(created)
			}
			return rt.Output(chatmsg.ProjectStreamingCardReceipt(created, bizID))
		}
		status := rt.Int("flow-status")
		if rt.DryRun() {
			plannedContent := content
			if mentionsRequested {
				plannedContent = "<atTag from create_and_send_card>" + content
			}
			return rt.Output(map[string]any{
				"contractVersion": currentCardWorkflowContract.Version,
				"dry_run":         true,
				"executed":        false,
				"preview_kind":    "plan",
				"actionCount":     2,
				"failedCount":     0,
				"actions": []map[string]any{
					{
						"tool":      "create_and_send_card",
						"arguments": params,
					},
					{
						"tool": "update_streaming_card",
						"arguments": map[string]any{
							"bizId":      "<from create_and_send_card>",
							"msgContent": plannedContent,
							"flowStatus": status,
						},
					},
				},
			})
		}
		created, err := rt.CallMCPWriteData("im", "create_and_send_card", params)
		if err != nil {
			return err
		}
		bizID := findCardBizID(created)
		if bizID == "" {
			return fmt.Errorf("卡片已创建但下层未返回 bizId，无法自动更新；请检查 create_and_send_card 响应")
		}
		atTag := findCardAtTag(created)
		if mentionsRequested && atTag == "" {
			return fmt.Errorf("卡片已创建（bizId=%s），但下层未返回 atTag，无法保证请求的 @ 生效；未执行自动更新", bizID)
		}
		updated, err := rt.CallMCPWriteData("im", "update_streaming_card", map[string]any{
			"bizId":      bizID,
			"msgContent": atTag + content,
			"flowStatus": status,
		})
		if err != nil {
			return fmt.Errorf("卡片已创建（bizId=%s），但自动更新失败: %w", bizID, err)
		}
		verification, err := chatmsg.VerifyStreamingCardUpdate(bizID, updated)
		if err != nil {
			return fmt.Errorf("卡片已创建（bizId=%s），但自动更新结果不可信: %w", bizID, cardUpdateVerificationError(bizID, err))
		}
		payload := chatmsg.ProjectStreamingCardReceipt(created, bizID)
		payload["bizId"] = bizID
		payload["flowStatus"] = status
		payload["updated"] = updated
		payload["updateAccepted"] = verification.Accepted
		payload["updateVerified"] = verification.Verified
		payload["updateVerificationEvidence"] = verification.Evidence
		if verification.Accepted && !verification.Verified {
			payload["updateWarning"] = "服务端已接受卡片更新请求，但未返回可独立证明可见内容已更新的字段；不要重复执行相同更新"
		}
		return rt.Output(payload)
	},
}

func findCardBizID(value any) string {
	return strings.TrimSpace(findCardResponseString(value, []string{"bizId", "bizID", "biz_id"}))
}

func findCardAtTag(value any) string {
	return findCardResponseString(value, []string{"atTag"})
}

func findCardResponseString(value any, directKeys []string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range directKeys {
			if candidate, ok := typed[key].(string); ok && strings.TrimSpace(candidate) != "" {
				return candidate
			}
		}

		// Prefer documented response envelopes before scanning extension fields.
		// Map iteration order is deliberately randomized by Go, so an unordered
		// recursive walk could select a stale metadata bizId.
		envelopeKeys := []string{"result", "data", "card", "response"}
		visited := make(map[string]struct{}, len(directKeys)+len(envelopeKeys))
		for _, key := range directKeys {
			visited[key] = struct{}{}
		}
		for _, key := range envelopeKeys {
			visited[key] = struct{}{}
			if candidate := findCardResponseString(typed[key], directKeys); candidate != "" {
				return candidate
			}
		}

		remainingKeys := make([]string, 0, len(typed))
		for key := range typed {
			if _, ok := visited[key]; !ok {
				remainingKeys = append(remainingKeys, key)
			}
		}
		sort.Strings(remainingKeys)
		for _, key := range remainingKeys {
			if candidate := findCardResponseString(typed[key], directKeys); candidate != "" {
				return candidate
			}
		}
	case []any:
		for _, child := range typed {
			if candidate := findCardResponseString(child, directKeys); candidate != "" {
				return candidate
			}
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var nested any
			if json.Unmarshal([]byte(trimmed), &nested) == nil {
				return findCardResponseString(nested, directKeys)
			}
		}
	}
	return ""
}

// MessagesUpdateCard streams updated card content (update_streaming_card, im).
var MessagesUpdateCard = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-update-card",
	Product:     "im",
	Description: "流式更新卡片内容（最后一次 --flow-status 应为 3）",
	Intent:      "当你要向已发送的流式文本卡片持续追加/更新内容时使用；会实际更新卡片，需传 send-card 返回的 bizId、新内容及 flowStatus 1-5（最后一次应为 3 表示完成）。当前不支持 Card JSON 组件或 action callback。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_update_card",
			CanonicalPath:  "chat.shortcut_messages_update_card",
			CLIPath:        "chat +messages-update-card",
			PrimaryCLIPath: "chat +messages-update-card",
		},
		Description: "流式更新卡片内容（最后一次 --flow-status 应为 3）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "流式更新卡片内容（最后一次 --flow-status 应为 3）",
			UseWhen:      []string{"当你要向已发送的流式文本卡片持续追加/更新内容时使用；会实际更新卡片，需传 send-card 返回的 bizId、新内容及 flowStatus 1-5（最后一次应为 3 表示完成）。当前不支持 Card JSON 组件或 action callback。"},
			AvoidWhen:    []string{"需要底层原始响应、未公开参数，或由调用方自行管理确认与更新节奏时，改用 chat message update-card"},
			Examples:     []string{"dws chat +messages-update-card --biz-id <bizId> --content \"内容\" --flow-status 3"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "biz-id", Type: shortcut.FlagString, Desc: "send-card 返回的卡片业务 ID", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "卡片消息内容", Required: true},
		{Name: "flow-status", Type: shortcut.FlagInt, Desc: "流式状态 1处理中/2输入中/3完成/4执行中/5错误；--flow-status 必须在 1-5 之间", Required: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"flow-status"}, Description: "--flow-status 必须在 1-5 之间"},
	},
	Tips: []string{`dws chat +messages-update-card --biz-id <bizId> --content "内容" --flow-status 3`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if _, err := chatmsg.NormalizeCardBizID(rt.Str("biz-id")); err != nil {
			return err
		}
		if !validCardFlowStatus(rt.Int("flow-status")) {
			return fmt.Errorf("--flow-status 必须在 1-5 之间")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Validate has already normalized and rejected empty, placeholder, and
		// whitespace-containing values before Execute is entered.
		bizID, _ := chatmsg.NormalizeCardBizID(rt.Str("biz-id"))
		params := map[string]any{
			"bizId":      bizID,
			"msgContent": rt.Str("content"),
			"flowStatus": rt.Int("flow-status"),
		}
		if rt.DryRun() {
			return rt.Output(map[string]any{
				"dry_run":  true,
				"executed": false,
				"verified": false,
				"action": map[string]any{
					"product":   "im",
					"tool":      "update_streaming_card",
					"arguments": params,
				},
			})
		}
		updated, err := rt.CallMCPWriteData("im", "update_streaming_card", params)
		if err != nil {
			return err
		}
		verification, err := chatmsg.VerifyStreamingCardUpdate(bizID, updated)
		if err != nil {
			return cardUpdateVerificationError(bizID, err)
		}
		return rt.Output(chatmsg.ProjectStreamingCardUpdate(updated, bizID, verification))
	},
}

func cardCreateMissingBizIDError(created map[string]any) error {
	return apperrors.NewAPI(
		"卡片可能已经创建，但服务端未返回后续更新所需的 bizId；CLI 无法确认卡片工作流可继续",
		apperrors.WithOperation("create_and_send_card"),
		apperrors.WithServerKey("im"),
		apperrors.WithOrigin("client_postcondition"),
		apperrors.WithFailureStage("verify_card_reference"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason("streaming_card_reference_missing"),
		apperrors.WithHint("不要盲目重试创建；请保留 trace_id 并推动服务端返回 bizId、openMessageId 和 openConversationId"),
		apperrors.WithDetails(map[string]any{"created": created}),
	)
}

func cardUpdateVerificationError(bizID string, verifyErr error) error {
	reason := "streaming_card_update_unverified"
	message := "服务端未返回卡片实际更新的证据；为避免假成功，CLI 已将本次操作判为失败"
	hint := "请检查服务端是否返回 updated=true、affectedCount>0 或等价的明确更新结果"
	switch {
	case errors.Is(verifyErr, chatmsg.ErrCardUpdateNotApplied):
		reason = "streaming_card_update_not_applied"
		message = "服务端明确表示流式卡片没有被更新"
		hint = "请确认 bizId 来自 send-card、当前账号有权限且卡片仍允许该状态转换"
	case errors.Is(verifyErr, chatmsg.ErrCardUpdateBizIDDrift):
		reason = "streaming_card_update_biz_id_mismatch"
		message = "服务端返回的 bizId 与本次请求不一致；无法确认目标卡片已更新"
		hint = "请保留 trace_id 并检查 update_streaming_card 的响应映射"
	}
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation("update_streaming_card"),
		apperrors.WithServerKey("im"),
		apperrors.WithOrigin("client_postcondition"),
		apperrors.WithFailureStage("verify_update_result"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
		apperrors.WithHint(hint),
		apperrors.WithDetails(map[string]any{"bizId": bizID}),
		apperrors.WithCause(verifyErr),
	)
}

// MessagesResourceURL gets a message resource download url (get_resource_download_url, im).
var MessagesResourceURL = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-resource-url",
	Product:     "im",
	Description: "获取消息资源（图片/视频/语音）下载链接",
	Intent:      "当你想下载消息里的图片/视频/语音等资源时使用；只读换取临时下载链接，需传资源 mediaId、消息 openMessageId 和会话 openConversationId。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "type", Type: shortcut.FlagString, Default: "mediaId", Desc: "资源类型", Enum: []string{"mediaId"}},
		{Name: "resource-id", Type: shortcut.FlagString, Desc: "资源 ID（消息中的 mediaId）", Required: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "消息 openMessageId"},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "--message-id 的别名", Hidden: true},
		{Name: "open-message-id", Type: shortcut.FlagString, Desc: "--message-id 的别名", Hidden: true},
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
	},
	// message-id is required, but accept the natural aliases agents reach for
	// (the message-list output field is openMessageId/msgId). Declared via a
	// constraint rather than Required because a shortcut's Required check only
	// looks at the primary flag name, so a hidden alias could not satisfy it.
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"message-id", "msg-id", "open-message-id"}},
	},
	Tips: []string{`dws chat +messages-resource-url --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_resource_download_url", map[string]any{
			"resourceType":       rt.Str("type"),
			"resourceId":         rt.Str("resource-id"),
			"openMessageId":      rt.StrFirst("message-id", "msg-id", "open-message-id"),
			"openConversationId": rt.Str("open-conversation-id"),
		})
	},
}

// MessagesForward forwards one message (forward_message, im).
var MessagesForward = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-forward",
	Product:     "im",
	Description: "转发单条消息",
	Intent:      "当你想把一条消息转发到另一个会话时使用；会实际转发消息，需传源会话 openConversationId、源消息 openMessageId 和目标会话 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "src-conversation-id", Type: shortcut.FlagString, Desc: "源会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "源消息 openMessageId", Required: true},
		{Name: "dest-conversation-id", Type: shortcut.FlagString, Desc: "目标会话 openConversationId", Required: true},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键（可选）"},
	},
	Tips: []string{`dws chat +messages-forward --src-conversation-id <srcCid> --msg-id <msgId> --dest-conversation-id <destCid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"srcOpenCid":       rt.Str("src-conversation-id"),
			"srcOpenMessageId": rt.Str("msg-id"),
			"destOpenCid":      rt.Str("dest-conversation-id"),
		}
		if rt.Changed("uuid") {
			params["uuid"] = rt.Str("uuid")
		}
		return rt.CallMCP("forward_message", params)
	},
}

// MessagesCombineForward merge-forwards multiple messages (combine_forward_messages, im).
var MessagesCombineForward = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-combine-forward",
	Product:     "im",
	Description: "合并转发多条消息",
	Intent:      "当你想把多条消息合并成一条转发到目标会话时使用；会实际合并转发，需传源会话、源消息 openMessageId 列表和目标会话 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "src-conversation-id", Type: shortcut.FlagString, Desc: "源会话 openConversationId", Required: true},
		{Name: "msg-ids", Type: shortcut.FlagStringSlice, Desc: "源消息 openMessageId 列表", Required: true},
		{Name: "dest-conversation-id", Type: shortcut.FlagString, Desc: "目标会话 openConversationId", Required: true},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键（可选）"},
	},
	Tips: []string{`dws chat +messages-combine-forward --src-conversation-id <srcCid> --msg-ids id1,id2 --dest-conversation-id <destCid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"srcOpenCid":        rt.Str("src-conversation-id"),
			"srcOpenMessageIds": rt.StrSlice("msg-ids"),
			"destOpenCid":       rt.Str("dest-conversation-id"),
		}
		if rt.Changed("uuid") {
			params["uuid"] = rt.Str("uuid")
		}
		return rt.CallMCP("combine_forward_messages", params)
	},
}

// MessagesForwardTopic forwards a topic message (forward_topic, im).
var MessagesForwardTopic = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-forward-topic",
	Product:     "im",
	Description: "转发话题消息到目标会话",
	Intent:      "当你要把某条群话题消息转发到目标会话时使用；会实际转发话题消息，需传源消息、源会话、话题 threadId 和目标会话 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "src-msg-id", Type: shortcut.FlagString, Desc: "源消息 openMessageId", Required: true},
		{Name: "src-conversation-id", Type: shortcut.FlagString, Desc: "源会话 openConversationId", Required: true},
		{Name: "src-thread-id", Type: shortcut.FlagString, Desc: "话题 ID（convThread + 加密 convThreadId）", Required: true},
		{Name: "dest-conversation-id", Type: shortcut.FlagString, Desc: "目标会话 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +messages-forward-topic --src-msg-id <msgId> --src-conversation-id <srcCid> --src-thread-id <threadId> --dest-conversation-id <destCid>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("forward_topic", map[string]any{
			"srcOpenMessageId":       rt.Str("src-msg-id"),
			"srcOpenConversationId":  rt.Str("src-conversation-id"),
			"srcOpenConvThreadId":    rt.Str("src-thread-id"),
			"destOpenConversationId": rt.Str("dest-conversation-id"),
		})
	},
}

// MessagesSetPin pins a message (set_pin_message, im).
var MessagesSetPin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-set-pin",
	Product:     "im",
	Description: "钉住消息（Pin）",
	Intent:      "当你想把某条消息钉在会话中（Pin）以便成员随时查看时使用；会实际钉住消息，需传会话 openConversationId 和消息 openMessageId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
	},
	Tips: []string{`dws chat +messages-set-pin --open-conversation-id <openConversationId> --msg-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_pin_message", map[string]any{
			"openConversationId": rt.Str("open-conversation-id"),
			"cid":                rt.Str("open-conversation-id"),
			"openMessageId":      rt.Str("msg-id"),
		})
	},
}

// MessagesUnsetPin unpins a message (unset_pin_message, im).
var MessagesUnsetPin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-unset-pin",
	Product:     "im",
	Description: "取消钉住消息（Unpin）",
	Intent:      "当你想取消此前钉住的消息时使用；会实际取消 Pin，需传会话 openConversationId 和消息 openMessageId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
	},
	Tips: []string{`dws chat +messages-unset-pin --open-conversation-id <openConversationId> --msg-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("unset_pin_message", map[string]any{
			"openConversationId": rt.Str("open-conversation-id"),
			"cid":                rt.Str("open-conversation-id"),
			"openMessageId":      rt.Str("msg-id"),
		})
	},
}

// MessagesListPin lists pinned messages (list_pin_messages, im).
var MessagesListPin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-list-pin",
	Product:     "im",
	Description: "拉取会话中钉住的消息列表",
	Intent:      "当你想查看某会话里当前钉住的消息有哪些时使用；只读分页返回，需传会话 openConversationId。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_messages_list_pin",
			CanonicalPath:  "chat.shortcut_messages_list_pin",
			CLIPath:        "chat +messages-list-pin",
			PrimaryCLIPath: "chat +messages-list-pin",
		},
		Description: "拉取会话中钉住的消息列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "拉取会话中钉住的消息列表",
			UseWhen:      []string{"当你想查看某会话里当前钉住的消息有哪些时使用；只读分页返回，需传会话 openConversationId。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +messages-list-pin --open-conversation-id <openConversationId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传 nextCursor"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "一次拉取的消息数量（默认 20，最大 100）"},
	},
	Tips: []string{`dws chat +messages-list-pin --open-conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId": rt.Str("open-conversation-id"),
			"cid":                rt.Str("open-conversation-id"),
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		if rt.Int("size") > 0 {
			params["count"] = rt.Int("size")
		}
		data, err := rt.CallMCPData("im", "list_pin_messages", params)
		if err != nil {
			return err
		}
		pins := listPinProject(data)
		payload := map[string]any{"count": len(pins), "pins": pins}
		chatmsg.ApplyPagination(payload, data)
		return rt.Output(payload)
	},
}

// listPinProject reshapes the raw list_pin_messages response into a clean
// {messageId, senderId, pinTime, conversationId} list — output-projection
// clean output projection. Both the list container and per-item field names are probed
// defensively across candidate keys, so an empty or unexpected shape yields an
// empty list rather than a crash or fabricated data.
func listPinProject(data map[string]any) []map[string]any {
	raw := listPinResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := listPinFirst(m, "openMessageId", "openMsgId", "messageId", "msgId"); ok {
			row["messageId"] = v
		}
		if v, ok := listPinFirst(m, "senderOpenDingTalkId", "senderUserId", "operatorId", "senderId"); ok {
			row["senderId"] = v
		}
		if v, ok := listPinFirst(m, "pinTime", "createTime", "gmtCreate", "operateTime"); ok {
			row["pinTime"] = v
		}
		if v, ok := listPinFirst(m, "openConversationId", "conversationId", "openConvId"); ok {
			row["conversationId"] = v
		}
		if threadID := chatmsg.ThreadID(m); threadID != nil {
			row["threadId"] = threadID
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// listPinResolveList locates the list payload, tolerating a bare top-level array
// container or nesting one level deeper under a common envelope key.
func listPinResolveList(data map[string]any) []any {
	for _, key := range []string{"result", "data", "list", "items", "pinMessages", "pinnedMessages", "messages"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "pinMessages", "pinnedMessages", "messages", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// listPinFirst returns the first present candidate key's value.
func listPinFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// MessagesSetTop pins a message to the top (set_top_message, im).
var MessagesSetTop = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-set-top",
	Product:     "im",
	Description: "置顶消息",
	Intent:      "当你想把某条消息置顶到会话顶部时使用；会实际置顶消息，需传会话 openConversationId 和消息 openMessageId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
	},
	Tips: []string{`dws chat +messages-set-top --open-conversation-id <openConversationId> --msg-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_top_message", map[string]any{
			"openConversationId": rt.Str("open-conversation-id"),
			"openMessageId":      rt.Str("msg-id"),
		})
	},
}

// MessagesUnsetTop cancels a message top (unset_top_message, im).
var MessagesUnsetTop = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-unset-top",
	Product:     "im",
	Description: "取消置顶消息",
	Intent:      "当你想取消此前置顶的消息时使用；会实际取消置顶，需传会话 openConversationId 和消息 openMessageId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
	},
	Tips: []string{`dws chat +messages-unset-top --open-conversation-id <openConversationId> --msg-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("unset_top_message", map[string]any{
			"openConversationId": rt.Str("open-conversation-id"),
			"openMessageId":      rt.Str("msg-id"),
		})
	},
}

func init() {
	shortcut.Register(withReviewedChatShortcutContracts(
		MessagesSendByBot,
		MessagesBatchSendByBot,
		MessagesSendByWebhook,
		MessagesRecall,
		MessagesRecallByBot,
		MessagesBatchRecallByBot,
		MessagesList,
		MessagesListDirect,
		MessagesListUnreadConversations,
		MessagesMget,
		MessagesQuerySendStatus,
		MessagesReadStatus,
		MessagesAddEmoji,
		MessagesRemoveEmoji,
		MessagesAddTextEmotion,
		MessagesRemoveTextEmotion,
		MessagesCreateTextEmotion,
		MessagesSendCard,
		MessagesUpdateCard,
		MessagesResourceURL,
		MessagesForward,
		MessagesCombineForward,
		MessagesForwardTopic,
		MessagesSetPin,
		MessagesUnsetPin,
		MessagesListPin,
		MessagesSetTop,
		MessagesUnsetTop,
	)...)
}
