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

// ConversationInfo gets conversation info (get_conversation_info, chat server).
var ConversationInfo = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-info",
	Product:     "chat",
	Description: "获取会话信息（群聊传 --group，单聊传 --open-dingtalk-id）",
	Intent:      "当你已有群 openConversationId 或单聊对方 openDingTalkId、需要查看该会话的名称/类型/成员数等基础信息时使用；只读，群聊传 --group、单聊传 --open-dingtalk-id 二选一。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群聊 openConversationId"},
		{Name: "open-dingtalk-id", Type: shortcut.FlagString, Desc: "单聊对方 openDingTalkId"},
	},
	Tips: []string{`dws chat +conversation-info --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Str("group") != "" {
			params["openConversationId"] = rt.Str("group")
		}
		if rt.Str("open-dingtalk-id") != "" {
			params["openDingTalkId"] = rt.Str("open-dingtalk-id")
		}
		if len(params) == 0 {
			return fmt.Errorf("--group 或 --open-dingtalk-id 必填其一")
		}
		return rt.CallMCP("get_conversation_info", params)
	},
}

// ConversationSetTop sets/unsets a conversation top (set_top_conversation, im).
var ConversationSetTop = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-set-top",
	Product:     "im",
	Description: "会话置顶 / 取消置顶（支持单聊/群聊）",
	Intent:      "当你想把某个单聊或群聊置顶到会话列表顶部、或取消其置顶时使用；会实际修改该会话的置顶状态，需传 openConversationId，加 --off 取消置顶。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "取消置顶（不传则设置置顶）"},
	},
	Tips: []string{`dws chat +conversation-set-top --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_top_conversation", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
			"top":                !rt.Bool("off"),
		})
	},
}

// ConversationMute mutes/unmutes a conversation (update_notification_off, im).
var ConversationMute = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-mute",
	Product:     "im",
	Description: "会话消息免打扰（支持单聊/群聊）",
	Intent:      "当你想对某个会话开启或关闭消息免打扰时使用；会实际更改该会话的免打扰设置，需传 openConversationId，加 --off 表示关闭免打扰。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "关闭免打扰（不传则开启免打扰）"},
	},
	Tips: []string{`dws chat +conversation-mute --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_notification_off", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
			"mute":               !rt.Bool("off"),
		})
	},
}

// ConversationMuteAtAll toggles @all notification (update_at_all_notification_off, im).
var ConversationMuteAtAll = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-mute-at-all",
	Product:     "im",
	Description: "关闭/开启 @所有人消息提醒",
	Intent:      "当你在某个群里不想再被'@所有人'打扰、或想恢复该提醒时使用；会实际修改该会话的@所有人提醒开关，需传 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "恢复接收 @所有人通知（不传则关闭通知）"},
	},
	Tips: []string{`dws chat +conversation-mute-at-all --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_at_all_notification_off", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
			"mute":               !rt.Bool("off"),
		})
	},
}

// ConversationMuteRedEnvelope toggles red-envelope notification (update_red_env_notification_off, im).
var ConversationMuteRedEnvelope = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-mute-red-envelope",
	Product:     "im",
	Description: "关闭/开启红包消息提醒",
	Intent:      "当你想在某个会话里关闭或恢复红包消息提醒时使用；会实际修改该会话的红包提醒开关，需传 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "恢复接收红包通知（不传则关闭通知）"},
	},
	Tips: []string{`dws chat +conversation-mute-red-envelope --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_red_env_notification_off", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
			"mute":               !rt.Bool("off"),
		})
	},
}

// ConversationMarkUnread marks a conversation unread (mark_conversation_unread, im).
var ConversationMarkUnread = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-mark-unread",
	Product:     "im",
	Description: "标记会话为未读",
	Intent:      "当你想把某个已读会话重新标记为未读（提醒自己稍后再处理）时使用；会实际改变该会话的未读状态，需传 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +conversation-mark-unread --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("mark_conversation_unread", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
		})
	},
}

// ConversationClearRedPoint clears a conversation red point (clear_conversation_red_point, im).
var ConversationClearRedPoint = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-clear-red-point",
	Product:     "im",
	Description: "清除会话红点",
	Intent:      "当你想消除某个会话上的未读红点（小圆点）时使用；会实际清除该会话红点，需传 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +conversation-clear-red-point --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("clear_conversation_red_point", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
		})
	},
}

// ConversationClearAllRedPoint clears all red points (clear_all_red_point, im).
var ConversationClearAllRedPoint = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-clear-all-red-point",
	Product:     "im",
	Description: "清除所有会话红点（全部已读）",
	Intent:      "当你想一键把全部会话标记为已读、清空所有红点时使用；会实际清除当前用户所有会话的红点，无需任何参数。",
	Risk:        shortcut.RiskWrite,
	Tips:        []string{`dws chat +conversation-clear-all-red-point`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("clear_all_red_point", map[string]any{})
	},
}

// ConversationList paginates all conversations (list_all_conversations, im).
var ConversationList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-list",
	Product:     "im",
	Description: "分页获取当前用户的全部会话列表（单聊+群聊）",
	Intent:      "当你想遍历当前用户的所有会话（单聊+群聊）做统计、清理或批量处理时使用；只读分页返回，可用 --exclude-muted 排除已免打扰会话。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "每页数量（1-100）"},
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "分页游标（首次不传或 0）"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已免打扰会话"},
	},
	Tips: []string{`dws chat +conversation-list --limit 50`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Int("cursor") > 0 {
			params["cursor"] = rt.Int("cursor")
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("im", "list_all_conversations", params)
		if err != nil {
			return err
		}
		convs := conversationListProject(data)
		payload := map[string]any{"count": len(convs), "conversations": convs}
		// carry pagination hints when present so翻页仍可继续（字段防御式探测）。
		if v, ok := conversationListFirst(data, "nextCursor", "cursor"); ok {
			payload["nextCursor"] = v
		}
		if v, ok := conversationListFirst(data, "hasMore", "has_more"); ok {
			payload["hasMore"] = v
		}
		return rt.Output(payload)
	},
}

// conversationListProject reshapes the raw list_all_conversations response into a
// clean conversation list — clean output projection. Both the list
// container and the per-item field names are probed defensively across candidate
// keys, so an unknown/empty shape yields an empty list rather than a crash or
// fabricated data.
func conversationListProject(data map[string]any) []map[string]any {
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
			row["conversationName"] = v
		}
		if v, ok := conversationListFirst(m, "conversationType", "type"); ok {
			row["conversationType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// conversationListResolveList locates the conversation array inside the response,
// tolerating a bare top-level list or nesting one level under a common envelope.
func conversationListResolveList(data map[string]any) []any {
	for _, key := range []string{"conversationList", "conversations", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"conversationList", "conversations", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// conversationListFirst returns the first present candidate key's value.
func conversationListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ConversationListTop lists pinned conversations (list_top_conversations, chat server).
var ConversationListTop = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-list-top",
	Description: "拉取置顶会话列表",
	Intent:      "当你只想查看被置顶的那些会话时使用；只读分页返回置顶会话列表，可用 --exclude-muted 排除已免打扰会话。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页数量"},
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "分页游标（首次不传或 0）"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已免打扰会话"},
	},
	Tips: []string{`dws chat +conversation-list-top --limit 1000`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Int("cursor") > 0 {
			params["cursor"] = rt.Int("cursor")
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("chat", "list_top_conversations", params)
		if err != nil {
			return err
		}
		convs := conversationListTopProject(data)
		payload := map[string]any{"count": len(convs), "conversations": convs}
		if v, ok := conversationListTopFirst(data, "nextCursor", "cursor"); ok {
			payload["nextCursor"] = v
		}
		if v, ok := conversationListTopFirst(data, "hasMore", "has_more"); ok {
			payload["hasMore"] = v
		}
		return rt.Output(payload)
	},
}

// conversationListTopProject reshapes the raw list_top_conversations response
// into a clean pinned-conversation list — clean output projection.
// Both the list container and the per-item field names are probed defensively
// across candidate keys, so an unknown/empty shape yields an empty list rather
// than a crash or fabricated data.
func conversationListTopProject(data map[string]any) []map[string]any {
	raw := conversationListTopResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := conversationListTopFirst(m, "openConversationId", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := conversationListTopFirst(m, "conversationName", "name", "title"); ok {
			row["conversationName"] = v
		}
		if v, ok := conversationListTopFirst(m, "conversationType", "type"); ok {
			row["conversationType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// conversationListTopResolveList locates the conversation array inside the
// response, tolerating a bare top-level list or nesting one level under a
// common envelope.
func conversationListTopResolveList(data map[string]any) []any {
	for _, key := range []string{"conversationList", "conversations", "topConversations", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"conversationList", "conversations", "topConversations", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// conversationListTopFirst returns the first present candidate key's value.
func conversationListTopFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ConversationClearMessages clears a conversation's chat records (clear_conversation_messages, im).
var ConversationClearMessages = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-clear-messages",
	Product:     "im",
	Description: "清空当前用户指定会话的聊天记录（仅本人视角，不可逆）",
	Intent:      "当你要清空自己在某个会话里的聊天记录时使用；仅影响本人视角，但会实际删除且不可逆，需传 openConversationId，务必谨慎操作。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +conversation-clear-messages --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("clear_conversation_messages", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
		})
	},
}

// ConversationMarkRead marks a message read (mark_message_read, im).
var ConversationMarkRead = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-mark-read",
	Product:     "im",
	Description: "标记消息已读（该消息及之前的消息都标记为已读）",
	Intent:      "当你想把某会话中某条消息及其之前的所有消息都标记为已读时使用；会实际更新已读位置，需传 openConversationId 和该消息 openMessageId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "消息 openMessageId", Required: true},
	},
	Tips: []string{`dws chat +conversation-mark-read --conversation-id <openConversationId> --message-id <openMessageId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("mark_message_read", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"openMessageId":      rt.Str("message-id"),
		})
	},
}

// ConversationHide hides a conversation from the list (hide_conversation, im).
var ConversationHide = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+conversation-hide",
	Product:     "im",
	Description: "会话列表中隐藏会话（收到新消息会重新出现）",
	Intent:      "当你想把某个会话从会话列表中暂时隐藏、让列表更清爽时使用；会实际隐藏该会话（收到新消息会自动重新出现），需传 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +conversation-hide --conversation-id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("hide_conversation", map[string]any{
			"openConversationId": rt.Str("conversation-id"),
			"cid":                rt.Str("conversation-id"),
		})
	},
}

// ── category: 会话分组管理 (im) ──────────────────────────────

// CategoryList lists user-defined conversation categories (list_user_define_conv_categories, im).
var CategoryList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-list",
	Product:     "im",
	Description: "获取用户自定义会话分组",
	Intent:      "当你想查看当前用户自建了哪些会话分组（如'工作群''项目群'）时使用；只读返回分组列表及其 categoryId，供后续按分组拉会话或增删。",
	Risk:        shortcut.RiskRead,
	Tips:        []string{`dws chat +category-list`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("im", "list_user_define_conv_categories", map[string]any{})
		if err != nil {
			return err
		}
		categories := categoryListProject(data)
		return rt.Output(map[string]any{"count": len(categories), "categories": categories})
	},
}

// categoryListProject reshapes the raw list_user_define_conv_categories response
// into a clean {categoryId, title} list — clean output projection.
// Both the list container and the per-item field names are probed defensively
// across candidate keys, so an unknown/empty shape yields an empty list rather
// than a crash or fabricated data.
func categoryListProject(data map[string]any) []map[string]any {
	raw := categoryListResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := categoryListFirst(m, "categoryId", "category_id", "id"); ok {
			row["categoryId"] = v
		}
		if v, ok := categoryListFirst(m, "title", "categoryName", "name"); ok {
			row["title"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// categoryListResolveList locates the category array inside the response,
// tolerating a bare top-level list or nesting one level under a common envelope.
func categoryListResolveList(data map[string]any) []any {
	for _, key := range []string{"categoryList", "categories", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"categoryList", "categories", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// categoryListFirst returns the first present candidate key's value.
func categoryListFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// CategoryListConversations lists conversations in a category (list_conversations_by_category, im).
var CategoryListConversations = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-list-conversations",
	Product:     "im",
	Description: "拉取指定自定义会话分组下的会话",
	Intent:      "当你想查看某个自定义会话分组里都归入了哪些会话时使用；只读，需传 categoryId，可用 --exclude-muted 排除已免打扰会话。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "category-id", Type: shortcut.FlagInt, Desc: "会话分组 ID", Required: true},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已免打扰会话"},
	},
	Tips: []string{`dws chat +category-list-conversations --category-id <分组ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"categoryId": rt.Int("category-id")}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("im", "list_conversations_by_category", params)
		if err != nil {
			return err
		}
		convs := categoryConversationsProject(data)
		payload := map[string]any{"count": len(convs), "conversations": convs}
		if v, ok := categoryConversationsFirst(data, "nextCursor", "cursor"); ok {
			payload["nextCursor"] = v
		}
		if v, ok := categoryConversationsFirst(data, "hasMore", "has_more"); ok {
			payload["hasMore"] = v
		}
		return rt.Output(payload)
	},
}

// categoryConversationsProject reshapes the raw list_conversations_by_category
// response into a clean conversation list — clean output projection.
// Both the list container and the per-item field names are probed defensively
// across candidate keys, so an unknown/empty shape yields an empty list rather
// than a crash or fabricated data.
func categoryConversationsProject(data map[string]any) []map[string]any {
	raw := categoryConversationsResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := categoryConversationsFirst(m, "openConversationId", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := categoryConversationsFirst(m, "conversationName", "name", "title"); ok {
			row["conversationName"] = v
		}
		if v, ok := categoryConversationsFirst(m, "conversationType", "type"); ok {
			row["conversationType"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// categoryConversationsResolveList locates the conversation array inside the
// response, tolerating a bare top-level list or nesting one level under a
// common envelope.
func categoryConversationsResolveList(data map[string]any) []any {
	for _, key := range []string{"conversationList", "conversations", "result", "data", "list", "items"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"conversationList", "conversations", "list", "items", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// categoryConversationsFirst returns the first present candidate key's value.
func categoryConversationsFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// CategoryCreate creates a conversation category (create_conv_category, im).
var CategoryCreate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-create",
	Product:     "im",
	Description: "创建用户自定义会话分组",
	Intent:      "当你想新建一个会话分组来归类会话时使用；会实际创建分组并返回其 ID，需传分组名称 --title。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "title", Type: shortcut.FlagString, Desc: "分组名称", Required: true},
	},
	Tips: []string{`dws chat +category-create --title "工作群"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("create_conv_category", map[string]any{"title": rt.Str("title")})
	},
}

// CategoryDelete deletes a conversation category (delete_conv_category, im).
var CategoryDelete = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-delete",
	Product:     "im",
	Description: "删除用户自定义会话分组",
	Intent:      "当你想删除某个自定义会话分组时使用；会实际删除分组（不影响其中会话本身），不可逆，需传 categoryId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "category-id", Type: shortcut.FlagInt, Desc: "会话分组 ID", Required: true},
	},
	Tips: []string{`dws chat +category-delete --category-id <分组ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("delete_conv_category", map[string]any{"categoryId": rt.Int("category-id")})
	},
}

// CategoryRename renames a conversation category (rename_conv_category, im).
var CategoryRename = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-rename",
	Product:     "im",
	Description: "更新用户自定义会话分组的名称",
	Intent:      "当你想重命名已有的自定义会话分组时使用；会实际更新分组名称，需传 categoryId 和新名称 --title。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "category-id", Type: shortcut.FlagInt, Desc: "会话分组 ID", Required: true},
		{Name: "title", Type: shortcut.FlagString, Desc: "新的分组名称", Required: true},
	},
	Tips: []string{`dws chat +category-rename --category-id <分组ID> --title "新名称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("rename_conv_category", map[string]any{
			"categoryId": rt.Int("category-id"),
			"title":      rt.Str("title"),
		})
	},
}

// CategoryAddConversation adds a conversation to categories (add_conv_to_categories, im).
var CategoryAddConversation = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-add-conversation",
	Product:     "im",
	Description: "将会话移动到指定的自定义分组中",
	Intent:      "当你想把某个会话归入一个或多个自定义分组时使用；会实际把会话加入指定分组，需传会话 openConversationId 和目标分组 ID 列表。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "category-ids", Type: shortcut.FlagStringSlice, Desc: "目标分组 ID 列表", Required: true},
	},
	Tips: []string{`dws chat +category-add-conversation --group <openConversationId> --category-ids 123,456`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		ids, err := toInt64Slice(rt.StrSlice("category-ids"))
		if err != nil {
			return fmt.Errorf("--category-ids: %w", err)
		}
		return rt.CallMCP("add_conv_to_categories", map[string]any{
			"openConversationId": rt.Str("group"),
			"categoryIds":        ids,
		})
	},
}

// CategoryRemoveConversation removes a conversation from categories (remove_conv_from_categories, im).
var CategoryRemoveConversation = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+category-remove-conversation",
	Product:     "im",
	Description: "将会话从指定的自定义分组中移出",
	Intent:      "当你想把某个会话从指定自定义分组中移出时使用；会实际从分组移除该会话（不删除会话本身），需传会话 openConversationId 和分组 ID 列表。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "会话 openConversationId", Required: true},
		{Name: "category-ids", Type: shortcut.FlagStringSlice, Desc: "目标分组 ID 列表", Required: true},
	},
	Tips: []string{`dws chat +category-remove-conversation --group <openConversationId> --category-ids 123,456`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		ids, err := toInt64Slice(rt.StrSlice("category-ids"))
		if err != nil {
			return fmt.Errorf("--category-ids: %w", err)
		}
		return rt.CallMCP("remove_conv_from_categories", map[string]any{
			"openConversationId": rt.Str("group"),
			"categoryIds":        ids,
		})
	},
}

func init() {
	shortcut.Register(
		ConversationInfo,
		ConversationSetTop,
		ConversationMute,
		ConversationMuteAtAll,
		ConversationMuteRedEnvelope,
		ConversationMarkUnread,
		ConversationClearRedPoint,
		ConversationClearAllRedPoint,
		ConversationList,
		ConversationListTop,
		ConversationClearMessages,
		ConversationMarkRead,
		ConversationHide,
		CategoryList,
		CategoryListConversations,
		CategoryCreate,
		CategoryDelete,
		CategoryRename,
		CategoryAddConversation,
		CategoryRemoveConversation,
	)
}
