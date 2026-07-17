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

// SearchMsg: search messages inside a single group chat by keyword in one step.
//
// Steps:
//  1. compute the look-back window [now-Nd, now] in local time and express both
//     bounds as epoch millis. N defaults to 7 days and is overridable via
//     --days; mirroring `dws chat message search`, which feeds startTime/endTime
//     as epoch millis to search_messages_by_keyword.
//  2. call search_messages_by_keyword on the chat server with keyword/startTime/
//     endTime/limit/cursor plus openConversationId — the exact parameter names
//     and first-page defaults (limit 100, cursor "0") used by
//     helpers.chatMessageSearchCmd. --query maps to keyword, --group maps to
//     openConversationId.
//  3. defensively project each returned message down to {sender, time, text,
//     messageId} (multiple candidate keys per field) and print via rt.Output so
//     it honours --format/--jq/--fields. When the response carries no
//     recognisable message list we fall back to printing the raw payload.
//
// This replaces manually working out the millisecond time window and copying the
// search incantation. Read-only: it only searches and reshapes, never sends,
// recalls or marks anything.
//
//	dws chat +search-msg --group <openConversationId> --query "changefree"
//	dws chat +search-msg --group <openConversationId> --query "周报" --days 3
var SearchMsg = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+search-msg",
	Product:     "chat",
	Description: "在群内按关键词搜消息（默认近 7 天，投影发送人/时间/内容/messageId）",
	Intent: "当你想在某个群里按关键词翻最近的聊天记录、但不想手动把起止时间换算成毫秒、也不想记 message search 的一堆参数时使用；" +
		"用 --group 指定群的 openConversationId、--query 指定搜索关键词，内部按本地时区算出「最近 N 天」（默认 7 天，可用 --days 调整回溯天数）的时间窗，" +
		"搜索这段时间内该群里命中关键词的消息，再在本地把每条消息投影成发送人、时间、内容、messageId 四个关键字段。" +
		"这是纯只读操作，只做搜索与本地投影，不会发送、撤回或标记任何消息。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群会话的 openConversationId（必填）"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "query", Type: shortcut.FlagString, Desc: "搜索关键词（必填）"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "--query 的别名", Hidden: true},
		{Name: "days", Type: shortcut.FlagInt, Desc: "回溯天数（可选，默认 7）", Default: "7", Required: false},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "conversation-id", "id"}},
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"query", "keyword"}},
	},
	Tips: []string{
		`dws chat +search-msg --group <openConversationId> --query "changefree"`,
		`dws chat +search-msg --group <openConversationId> --query "周报" --days 3`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		group := rt.StrFirst("group", "conversation-id", "id")
		query := rt.StrFirst("query", "keyword")

		// Step 1 — look-back window [now-Nd, now] in epoch millis. days defaults
		// to 7; guard against non-positive overrides so the window stays sane.
		days := rt.Int("days")
		if days <= 0 {
			days = 7
		}
		now := time.Now()
		startMs := now.AddDate(0, 0, -days).UnixMilli()
		endMs := now.UnixMilli()

		// Step 2 — search messages by keyword within the group. keyword/startTime/
		// endTime/limit/cursor + openConversationId and the first-page defaults
		// (limit 100, cursor "0") mirror helpers.chatMessageSearchCmd's
		// search_messages_by_keyword call.
		data, err := rt.CallMCPData("chat", "search_messages_by_keyword", map[string]any{
			"keyword":            query,
			"startTime":          startMs,
			"endTime":            endMs,
			"limit":              100,
			"cursor":             "0",
			"openConversationId": group,
		})
		if err != nil {
			return err
		}

		// Step 3 — project matched messages; fall back to the raw payload when we
		// cannot locate a recognisable message list.
		items := searchMsgItems(data)
		if len(items) == 0 {
			return rt.Output(data)
		}
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"sender":    searchMsgSender(m),
				"time":      searchMsgTime(m),
				"text":      searchMsgText(m),
				"messageId": searchMsgMessageID(m),
			})
		}
		return rt.Output(map[string]any{"messages": results})
	},
}

// searchMsgItems locates the message list inside a search_messages_by_keyword
// response, probing common container keys at the top level and nested under
// "result". Returns nil when no list is found.
func searchMsgItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	keys := []string{"list", "messages", "messageList", "items", "data", "records", "result"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return searchMsgToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "messages", "messageList", "items", "data", "records"} {
				if arr, ok := inner[k2].([]any); ok {
					return searchMsgToMaps(arr)
				}
			}
		}
	}
	return nil
}

func searchMsgToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// searchMsgSender reads a message's sender display name/id, tolerating the
// common sender keys the gateway may use (including a nested sender object).
func searchMsgSender(m map[string]any) any {
	for _, key := range []string{"senderName", "sender_name", "senderNick", "fromName", "senderStaffName"} {
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"sender", "from", "senderUser"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"name", "nick", "userName", "staffName", "displayName"} {
				if v := searchMsgString(nested[k2]); v != "" {
					return v
				}
			}
		}
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"senderId", "sender_id", "senderUserId", "senderStaffId", "openDingTalkId"} {
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// searchMsgTime reads a message's send time, returning the raw value (usually
// epoch millis) under whichever candidate key is present.
func searchMsgTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "time", "msgTime", "createAt"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// searchMsgText reads a message's textual content, tolerating flat text keys and
// a nested content/text object.
func searchMsgText(m map[string]any) any {
	for _, key := range []string{"text", "content", "msgContent", "message", "body"} {
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	for _, key := range []string{"content", "text", "msg"} {
		if nested, ok := m[key].(map[string]any); ok {
			for _, k2 := range []string{"text", "content", "richText", "title"} {
				if v := searchMsgString(nested[k2]); v != "" {
					return v
				}
			}
		}
	}
	return nil
}

// searchMsgMessageID reads a message's identifier, tolerating the common id keys
// the gateway may use.
func searchMsgMessageID(m map[string]any) any {
	for _, key := range []string{"messageId", "message_id", "msgId", "msg_id", "openMessageId", "id"} {
		if v := searchMsgString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// searchMsgString coerces a scalar JSON value to a trimmed string, returning ""
// for nil / non-scalar / empty values.
func searchMsgString(v any) string {
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
	shortcut.Register(SearchMsg)
}
