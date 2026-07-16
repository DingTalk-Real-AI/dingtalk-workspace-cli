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
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// AtMe: pull the messages that recently @-mentioned ME across chats in one step.
//
// Steps:
//  1. compute the look-back window [now-Nd, now] in local time and express both
//     bounds as epoch millis. N defaults to 7 days and is overridable via
//     --days; mirroring `dws chat message list-mentions`, which feeds startTime/
//     endTime as epoch millis to search_at_me_message.
//  2. call search_at_me_message on the chat server with startTime/endTime/limit/
//     cursor — the exact parameter names + first-page defaults (limit 50,
//     cursor "0") used by helpers.chatMessageListMentionsCmd.
//  3. defensively project each returned message down to {sender, time, text,
//     conversation} (multiple candidate keys per field) and print via rt.Output
//     so it honours --format/--jq/--fields. When the response carries no
//     recognisable message list we fall back to printing the raw payload.
//
// This replaces manually working out the millisecond time window and copying the
// list-mentions incantation. Read-only: it only searches and reshapes, never
// sends, recalls or marks anything.
//
//	dws chat +at-me
//	dws chat +at-me --days 3
var AtMe = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+at-me",
	Product:     "chat",
	Description: "查最近 @我 的消息（自动算时间窗，投影发送人/时间/内容/会话）",
	Intent: "当你想快速看回最近谁在群里或单聊里 @了你、但不想手动把起止时间换算成毫秒、也不想记 list-mentions 的一堆参数时使用；" +
		"内部按本地时区算出「最近 N 天」（默认 7 天，可用 --days 调整回溯天数）的时间窗，搜索这段时间内 @我 的消息，" +
		"再在本地把每条消息投影成发送人、时间、内容、所在会话四个关键字段。" +
		"这是纯只读操作，只做搜索与本地投影，不会发送、撤回或标记任何消息。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "days", Type: shortcut.FlagInt, Desc: "回溯天数（可选，默认 7）", Default: "7", Required: false},
	},
	Tips: []string{
		`dws chat +at-me`,
		`dws chat +at-me --days 3`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — look-back window [now-Nd, now] in epoch millis. days defaults
		// to 7; guard against non-positive overrides so the window stays sane.
		days := rt.Int("days")
		if days <= 0 {
			days = 7
		}
		now := time.Now()
		startMs := now.AddDate(0, 0, -days).UnixMilli()
		endMs := now.UnixMilli()

		// Step 2 — search @me messages. startTime/endTime/limit/cursor and the
		// first-page defaults (limit 50, cursor "0") mirror
		// helpers.chatMessageListMentionsCmd's search_at_me_message call.
		data, err := rt.CallMCPData("chat", "search_at_me_message", map[string]any{
			"startTime": startMs,
			"endTime":   endMs,
			"limit":     50,
			"cursor":    "0",
		})
		if err != nil {
			return err
		}

		// Step 3 — project matched messages; fall back to the raw payload when we
		// cannot locate a recognisable message list.
		items := atMeMessageItems(data)
		if len(items) == 0 {
			return rt.Output(data)
		}
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"sender":       atMeSender(m),
				"time":         atMeTime(m),
				"text":         atMeText(m),
				"conversation": atMeConversation(m),
			})
		}
		return rt.Output(map[string]any{"messages": results})
	},
}

// atMeMessageItems locates the message list inside a search_at_me_message
// response, probing common container keys at the top level and nested under
// "result". Returns nil when no list is found.
func atMeMessageItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	// Preferred real shape (verified against the live gateway):
	//   {result:{conversationMessagesList:[{title,openConversationId,messages:[…]}]}}
	// The messages are nested two levels deep and grouped per conversation, so
	// flatten every group into a single message list while carrying the group's
	// conversation title/id down onto each message.
	for _, root := range []map[string]any{data, atMeChildMap(data, "result")} {
		if root == nil {
			continue
		}
		if groups, ok := root["conversationMessagesList"].([]any); ok {
			return atMeFlattenGroups(groups)
		}
	}
	keys := []string{"list", "messages", "messageList", "items", "data", "records", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return atMeToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "messages", "messageList", "items", "data", "records"} {
				if arr, ok := inner[k2].([]any); ok {
					return atMeToMaps(arr)
				}
			}
		}
	}
	return nil
}

// atMeChildMap returns data[key] as a map, or nil.
func atMeChildMap(data map[string]any, key string) map[string]any {
	if m, ok := data[key].(map[string]any); ok {
		return m
	}
	return nil
}

// atMeFlattenGroups flattens conversationMessagesList groups into a single
// message list, injecting the group's conversation title / id onto each message
// (when the message itself lacks them) so the projection can show a readable
// conversation.
func atMeFlattenGroups(groups []any) []map[string]any {
	var out []map[string]any
	for _, g := range groups {
		grp, ok := g.(map[string]any)
		if !ok {
			continue
		}
		title := atMeString(grp["title"])
		cid := atMeString(grp["openConversationId"])
		msgs, ok := grp["messages"].([]any)
		if !ok {
			continue
		}
		for _, mm := range msgs {
			m, ok := mm.(map[string]any)
			if !ok {
				continue
			}
			if title != "" {
				if _, has := m["conversationTitle"]; !has {
					m["conversationTitle"] = title
				}
			}
			if cid != "" {
				if _, has := m["openConversationId"]; !has {
					m["openConversationId"] = cid
				}
			}
			out = append(out, m)
		}
	}
	return out
}

func atMeToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// atMeSender reads a message's sender display name/id, tolerating the common
// sender keys the gateway may use (including a nested sender object).
func atMeSender(m map[string]any) any {
	for _, key := range []string{"senderName", "sender_name", "senderNick", "fromName", "senderStaffName"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"sender", "from", "senderUser"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"name", "nick", "userName", "staffName", "displayName"} {
				if v := atMeString(nested[k2]); v != "" {
					return v
				}
			}
		}
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"senderId", "sender_id", "senderUserId", "senderStaffId", "openDingTalkId"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// atMeTime reads a message's send time, returning the raw value (usually epoch
// millis) under whichever candidate key is present.
func atMeTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "time", "msgTime", "createAt"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// atMeText reads a message's textual content, tolerating flat text keys and a
// nested content/text object.
func atMeText(m map[string]any) any {
	for _, key := range []string{"text", "content", "msgContent", "message", "body"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"content", "text", "msg"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"text", "content", "richText", "title"} {
				if v := atMeString(nested[k2]); v != "" {
					return v
				}
			}
		}
	}
	return nil
}

// atMeConversation reads the conversation (chat) this message belongs to,
// preferring a readable title and falling back to the conversation id.
func atMeConversation(m map[string]any) any {
	for _, key := range []string{"conversationTitle", "chatTitle", "groupName", "conversationName", "title"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"openConversationId", "conversationId", "conversation_id", "cid", "chatId"} {
		if v := atMeString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// atMeString coerces a scalar JSON value to a trimmed string, returning "" for
// nil / non-scalar / empty values.
func atMeString(v any) string {
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
	shortcut.Register(AtMe)
}
