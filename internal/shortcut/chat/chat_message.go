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
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

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
		{Name: "text", Type: shortcut.FlagString, Desc: "Markdown 正文", Required: true},
		{Name: "at-user-ids", Type: shortcut.FlagStringSlice, Desc: "@ 的 userId 列表"},
		{Name: "at-open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "@ 的 openDingTalkId 列表"},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@ 所有人"},
	},
	Tips: []string{`dws chat +messages-send-by-bot --robot-code <robotCode> --group <openConversationId> --title "日报" --text "## 今日完成"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"robotCode":          rt.Str("robot-code"),
			"openConversationId": rt.Str("group"),
			"title":              rt.Str("title"),
			"markdown":           rt.Str("text"),
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
		{Name: "text", Type: shortcut.FlagString, Desc: "Markdown 正文", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "接收人 userId 列表"},
		{Name: "open-dingtalk-ids", Type: shortcut.FlagStringSlice, Desc: "接收人 openDingTalkId 列表"},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@ 所有人"},
	},
	Tips: []string{`dws chat +messages-batch-send-by-bot --robot-code <robotCode> --users userId1,userId2 --title "提醒" --text "请提交周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"robotCode": rt.Str("robot-code"),
			"title":     rt.Str("title"),
			"markdown":  rt.Str("text"),
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
	Description: "自定义机器人 Webhook 发送群消息",
	Intent:      "当你只有自定义机器人的 Webhook token、想往其所在群推送消息时使用；会实际通过 Webhook 发群消息，需传 token、标题、正文，可 @手机号/userId 或 @所有人。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "token", Type: shortcut.FlagString, Desc: "Webhook token", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "消息标题", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "消息正文", Required: true},
		{Name: "at-all", Type: shortcut.FlagBool, Desc: "@ 所有人"},
		{Name: "at-mobiles", Type: shortcut.FlagStringSlice, Desc: "@ 的手机号列表"},
		{Name: "at-users", Type: shortcut.FlagStringSlice, Desc: "@ 的 userId 列表"},
	},
	Tips: []string{`dws chat +messages-send-by-webhook --token <token> --title "告警" --text "CPU 超 90%" --at-all`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"robotToken": rt.Str("token"),
			"title":      rt.Str("title"),
			"text":       rt.Str("text"),
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
	Intent:      "当你想撤回当前用户刚发出的某条消息时使用；会实际撤回消息，需传会话 openConversationId 和消息 openMessageId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "msg-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
	},
	Tips: []string{`dws chat +messages-recall --conversation-id <openConversationId> --msg-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("recall_message", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"openMessageId":      rt.Str("msg-id"),
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
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"", Required: true},
		{Name: "forward", Type: shortcut.FlagBool, Default: "true", Desc: "true=从给定时间往现在拉，false=往以前拉"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量"},
	},
	Tips: []string{`dws chat +messages-list --group <openConversationId> --time "2025-03-01 00:00:00"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openCid": rt.Str("group"),
			"cid":     rt.Str("group"),
			"time":    rt.Str("time"),
			"forward": rt.Bool("forward"),
		}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		data, err := rt.CallMCPData("chat", "list_conversation_message_v2", params)
		if err != nil {
			return err
		}
		messages := listMessagesProject(data)
		return rt.Output(map[string]any{"count": len(messages), "messages": messages})
	},
}

// listMessagesProject reshapes a raw conversation-message list response into a
// clean {messageId, senderId, msgType, createTime, text} list — output-projection
// clean output projection. Both the list container and the per-item field names are
// probed defensively across candidate keys, so an empty or unexpected shape
// yields an empty list rather than a crash or fabricated data.
func listMessagesProject(data map[string]any) []map[string]any {
	raw := listMessagesResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := listMessagesFirst(m, "openMessageId", "openMsgId", "messageId", "msgId"); ok {
			row["messageId"] = v
		}
		if v, ok := listMessagesFirst(m, "senderOpenDingTalkId", "senderUserId", "senderId", "senderStaffId"); ok {
			row["senderId"] = v
		}
		if v, ok := listMessagesFirst(m, "msgType", "messageType", "type"); ok {
			row["msgType"] = v
		}
		if v, ok := listMessagesFirst(m, "createTime", "sendTime", "gmtCreate", "messageTime"); ok {
			row["createTime"] = v
		}
		if v, ok := listMessagesFirst(m, "text", "content", "plainText"); ok {
			row["text"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
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

// listMessagesFirst returns the first present candidate key's value.
func listMessagesFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// MessagesListDirect pulls messages of a single chat (list_individual_chat_message, chat server).
var MessagesListDirect = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-list-direct",
	Description: "拉取单聊会话消息",
	Intent:      "当你想按时间拉取与某人单聊的历史消息时使用；只读，需传对方 userId 或 openDingTalkId 及起始时间，--forward 控制翻页方向。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "user", Type: shortcut.FlagString, Desc: "对方 userId（与 --open-dingtalk-id 二选一）"},
		{Name: "open-dingtalk-id", Type: shortcut.FlagString, Desc: "对方 openDingTalkId（与 --user 二选一）"},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"", Required: true},
		{Name: "forward", Type: shortcut.FlagBool, Default: "true", Desc: "true=往现在拉，false=往以前拉"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量"},
	},
	Tips: []string{`dws chat +messages-list-direct --user <userId> --time "2025-03-01 00:00:00"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"time":    rt.Str("time"),
			"forward": rt.Bool("forward"),
		}
		switch {
		case rt.Str("open-dingtalk-id") != "":
			params["openDingTalkId"] = rt.Str("open-dingtalk-id")
		case rt.Str("user") != "":
			params["userId"] = rt.Str("user")
		default:
			return fmt.Errorf("--user 或 --open-dingtalk-id 必填其一")
		}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		data, err := rt.CallMCPData("chat", "list_individual_chat_message", params)
		if err != nil {
			return err
		}
		messages := listMessagesProject(data)
		return rt.Output(map[string]any{"count": len(messages), "messages": messages})
	},
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
	Intent:      "当你已有一批消息 openMsgId、需要批量取回它们的详细内容时使用；只读，一次最多查询 50 条。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "msg-ids", Type: shortcut.FlagStringSlice, Desc: "消息 openMsgId 列表", Required: true},
	},
	Tips: []string{`dws chat +messages-mget --msg-ids msgId1,msgId2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_messages_by_ids", map[string]any{"openMsgIds": rt.StrSlice("msg-ids")})
	},
}

// MessagesQuerySendStatus queries send status of a message (query_message_send_status, im).
var MessagesQuerySendStatus = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-query-send-status",
	Product:     "im",
	Description: "查询消息发送状态",
	Intent:      "当你发消息后拿到 openTaskId、想确认这条消息是否发送成功时使用；只读返回发送状态，需传 --open-task-id。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "open-task-id", Type: shortcut.FlagString, Desc: "发送消息时返回的 openTaskId", Required: true},
	},
	Tips: []string{`dws chat +messages-query-send-status --open-task-id <openTaskId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("query_message_send_status", map[string]any{"openTaskId": rt.Str("open-task-id")})
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
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "消息 openMessageId（当前用户发送的消息）", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "目标 userId 或 openDingTalkId 列表（不传返回全部接收者）"},
	},
	Tips: []string{`dws chat +messages-read-status --conversation-id <openConversationId> --message-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId": rt.Str("conversation-id"),
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

// MessagesSendCard creates and pushes a streaming card (create_and_send_card, im).
var MessagesSendCard = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-send-card",
	Product:     "im",
	Description: "创建并推送流式卡片（需配合 messages-update-card）",
	Intent:      "当你要发送一张可后续流式更新的卡片消息（如 AI 逐字输出）时使用；会实际推送卡片并返回 bizId，群 openConversationId 或单聊接收者 userId 二选一，配合 messages-update-card 更新。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId（与 --receiver 互斥）"},
		{Name: "receiver", Type: shortcut.FlagString, Desc: "单聊接收者 userId（与 --group 互斥）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "receiver"}},
	},
	Tips: []string{`dws chat +messages-send-card --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		group := rt.Str("group")
		receiver := rt.Str("receiver")
		if group == "" && receiver == "" {
			return fmt.Errorf("--group 或 --receiver 必填其一")
		}
		if group != "" && receiver != "" {
			return fmt.Errorf("--group 与 --receiver 互斥")
		}
		params := map[string]any{}
		if group != "" {
			params["openConversationId"] = group
		} else {
			params["receiverUid"] = receiver
		}
		return rt.CallMCP("create_and_send_card", params)
	},
}

// MessagesUpdateCard streams updated card content (update_streaming_card, im).
var MessagesUpdateCard = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-update-card",
	Product:     "im",
	Description: "流式更新卡片内容（最后一次 --flow-status 应为 3）",
	Intent:      "当你要向已发送的流式卡片持续追加/更新内容时使用；会实际更新卡片，需传 send-card 返回的 bizId、新内容及 flowStatus（最后一次应为 3 表示完成）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "biz-id", Type: shortcut.FlagString, Desc: "send-card 返回的卡片业务 ID", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "卡片消息内容", Required: true},
		{Name: "flow-status", Type: shortcut.FlagInt, Desc: "流式状态 1处理中/2输入中/3完成/4执行中/5错误", Required: true},
	},
	Tips: []string{`dws chat +messages-update-card --biz-id <bizId> --content "内容" --flow-status 3`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_streaming_card", map[string]any{
			"bizId":      rt.Str("biz-id"),
			"msgContent": rt.Str("content"),
			"flowStatus": rt.Int("flow-status"),
		})
	},
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
		{Name: "message-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +messages-resource-url --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_resource_download_url", map[string]any{
			"resourceType":       rt.Str("type"),
			"resourceId":         rt.Str("resource-id"),
			"openMessageId":      rt.Str("message-id"),
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
		return rt.Output(map[string]any{"count": len(pins), "pins": pins})
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
	shortcut.Register(
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
	)
}
