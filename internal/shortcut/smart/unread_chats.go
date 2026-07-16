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

package smart

import (
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// UnreadChats: list MY conversations that currently have unread messages in one
// step.
//
// It calls unread_message_conversation_list on the chat server — the exact tool
// and parameter names used by helpers.chatMessageListUnreadConversationsCmd. The
// helper passes count (int) only when > 0 and excludeMuted (bool) only when
// true, so both are optional; this shortcut mirrors that: --count controls how
// many conversations to return (0 / unset uses the server default) and
// --exclude-muted drops chats you have muted.
//
// The returned conversations are then defensively projected down to
// {name, unread, conversationId} (multiple candidate keys per field). When the
// response carries no recognisable conversation list we fall back to printing
// the raw payload via rt.Output so it still honours --format/--jq/--fields.
//
// Read-only: it only lists and reshapes, it never marks anything read/unread.
//
//	dws chat +unread-chats
//	dws chat +unread-chats --count 20
//	dws chat +unread-chats --exclude-muted
var UnreadChats = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+unread-chats",
	Product:     "chat",
	Description: "列出我有未读消息的会话（投影会话名/未读数/会话ID）",
	Intent: "当你想快速看清自己当前有哪些会话还有未读消息、方便逐个处理时使用；" +
		"内部调用未读会话列表接口，可用 --count 控制返回的会话条数（不传则用服务端默认值），" +
		"用 --exclude-muted 排除你已设置免打扰的会话；再在本地把每个会话投影成会话名、未读数和会话 ID 三个关键字段。" +
		"这是纯只读操作，只做列表与本地投影，不会把任何会话标记为已读或未读；若没有未读会话则返回空列表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "count", Type: shortcut.FlagInt, Desc: "返回未读会话条数（可选，不传则使用服务端默认值）", Required: false},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "是否排除已设置免打扰的会话（可选，默认 false）", Required: false},
	},
	Tips: []string{
		`dws chat +unread-chats`,
		`dws chat +unread-chats --count 20`,
		`dws chat +unread-chats --exclude-muted`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Build params exactly like chatMessageListUnreadConversationsCmd: count is
		// only sent when > 0, excludeMuted only when true.
		toolArgs := map[string]any{}
		if count := rt.Int("count"); count > 0 {
			toolArgs["count"] = count
		}
		if rt.Bool("exclude-muted") {
			toolArgs["excludeMuted"] = true
		}

		data, err := rt.CallMCPData("chat", "unread_message_conversation_list", toolArgs)
		if err != nil {
			return err
		}

		// Project each conversation; fall back to the raw payload when we cannot
		// locate a recognisable conversation list.
		items := unreadChatItems(data)
		if len(items) == 0 {
			return rt.Output(data)
		}
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			row := map[string]any{
				"name":           unreadChatName(m),
				"conversationId": unreadChatConversationID(m),
			}
			// unread_message_conversation_list does not return a per-conversation
			// unread count (presence in the list already implies unread>0), so
			// only surface "unread" when the gateway actually provides it — avoid
			// emitting a noisy null on every row.
			if u := unreadChatUnread(m); u != nil {
				row["unread"] = u
			}
			results = append(results, row)
		}
		return rt.Output(map[string]any{"conversations": results})
	},
}

// unreadChatItems locates the conversation list inside an
// unread_message_conversation_list response, probing common container keys at
// the top level and nested under "result"/"data". Returns nil when no list is
// found.
func unreadChatItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	keys := []string{"list", "conversations", "conversationList", "unreadConversations", "items", "data", "records", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return unreadChatToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "conversations", "conversationList", "unreadConversations", "items", "data", "records"} {
				if arr, ok := inner[k2].([]any); ok {
					return unreadChatToMaps(arr)
				}
			}
		}
	}
	return nil
}

func unreadChatToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// unreadChatName reads a conversation's display title, preferring a readable
// name and falling back to a nested conversation object.
func unreadChatName(m map[string]any) any {
	for _, key := range []string{"conversationTitle", "title", "conversationName", "groupName", "chatTitle", "name", "showName"} {
		if v := unreadChatString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"conversation", "chat", "group"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"title", "name", "conversationTitle", "groupName", "showName"} {
				if v := unreadChatString(nested[k2]); v != "" {
					return v
				}
			}
		}
	}
	return nil
}

// unreadChatUnread reads a conversation's unread message count, returning the
// raw value under whichever candidate key is present.
func unreadChatUnread(m map[string]any) any {
	for _, key := range []string{"unreadCount", "unread", "unReadCount", "unreadNum", "redPoint", "count"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// unreadChatConversationID reads the conversation id, tolerating the common id
// keys the gateway may use.
func unreadChatConversationID(m map[string]any) any {
	for _, key := range []string{"openConversationId", "conversationId", "conversation_id", "cid", "openCid", "chatId"} {
		if v := unreadChatString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"conversation", "chat"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"openConversationId", "conversationId", "cid", "openCid"} {
				if v := unreadChatString(nested[k2]); v != "" {
					return v
				}
			}
		}
	}
	return nil
}

// unreadChatString coerces a scalar JSON value to a trimmed string, returning ""
// for nil / non-scalar / empty values.
func unreadChatString(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func init() {
	shortcut.Register(UnreadChats)
}
