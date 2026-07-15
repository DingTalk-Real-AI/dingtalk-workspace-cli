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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ThreadReplies: fetch every reply under one topic ("话题") message and print a
// clean projected list (speaker / text / time) instead of a raw dump.
//
// Steps:
//  1. call list_topic_replies (chat server) with openconversationId=--group and
//     topicId=--topic-id, optionally startTime=--time and pageSize=--limit —
//     param keys copied verbatim from chat.go's list-topic-replies call site;
//  2. defensively unwrap the reply list (multiple candidate container keys) and
//     project each reply to {sender, text, createTime} tolerating field aliases;
//  3. print via rt.Output as {replies, count} so it honours --format/--jq/--fields.
//
// Read-only: it only reads a topic's replies and reshapes them locally, never
// posts or mutates anything.
//
//	dws chat +thread-replies --group <openconversationId> --topic-id <topicId>
var ThreadReplies = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+thread-replies",
	Product:     "chat",
	Description: "拉取某条话题消息的全部回复并投影出发言人/文本/时间",
	Intent: "当你已经拿到某个群里一条「话题消息」的 topicId、想快速看这条话题下的全部回复（谁在什么时间回复了什么），" +
		"而不想拿到一大坨原始消息字段时使用；内部按 --group（群会话 ID）和 --topic-id（话题 ID）拉取该话题的回复列表，" +
		"可选 --time 指定起始时间、--limit 指定每页条数，再在本地投影出每条回复的发言人、文本和回复时间。" +
		"这是纯只读操作，只做拉取与本地投影，不会发送或修改任何消息。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群会话 ID（openConversationId，必填）", Required: true},
		{Name: "topic-id", Type: shortcut.FlagString, Desc: "话题 ID（由 dws chat message list 返回，必填）", Required: true},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"（可选）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页拉取的回复条数（可选）"},
	},
	Tips: []string{
		`dws chat +thread-replies --group <openconversationId> --topic-id <topicId>`,
		`dws chat +thread-replies --group <openconversationId> --topic-id <topicId> --time "2025-03-01 00:00:00" --limit 20`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — fetch the topic replies. Param keys (openconversationId /
		// topicId / startTime / pageSize) are copied verbatim from chat.go's
		// list-topic-replies call site.
		params := map[string]any{
			"openconversationId": rt.Str("group"),
			"topicId":            rt.Str("topic-id"),
			"forward":            false,
		}
		if rt.Changed("time") && rt.Str("time") != "" {
			params["startTime"] = rt.Str("time")
		}
		if rt.Changed("limit") && rt.Int("limit") > 0 {
			params["pageSize"] = rt.Int("limit")
		}

		data, err := rt.CallMCPData("chat", "list_topic_replies", params)
		if err != nil {
			return err
		}

		// Step 2 — defensively unwrap and project. Response shape has no
		// contract, so probe multiple candidate container/field keys.
		items := threadReplyItems(data)
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"sender":     threadReplySender(m),
				"text":       threadReplyText(m),
				"createTime": threadReplyCreateTime(m),
			})
		}

		return rt.Output(map[string]any{
			"replies": results,
			"count":   len(results),
		})
	},
}

// threadReplyItems defensively unwraps the reply list from the response,
// tolerating the common container keys and one level of nesting under a
// "result"/"data" wrapper.
func threadReplyItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	// Try the list keys directly on the response, then inside a wrapper.
	scopes := []map[string]any{data}
	for _, wrap := range []string{"result", "data"} {
		if inner, ok := data[wrap].(map[string]any); ok {
			scopes = append(scopes, inner)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"replies", "list", "items", "messages", "records", "data", "result"} {
			if raw, ok := scope[key].([]any); ok {
				out := make([]map[string]any, 0, len(raw))
				for _, e := range raw {
					if m, ok := e.(map[string]any); ok {
						out = append(out, m)
					}
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}
	return nil
}

// threadReplySender reads a reply's speaker display name, tolerating common
// sender-name keys.
func threadReplySender(m map[string]any) any {
	for _, key := range []string{"senderName", "senderNick", "nick", "senderStaffName", "userName", "name", "senderId", "senderStaffId"} {
		if v, ok := m[key]; ok && v != nil {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			return v
		}
	}
	return nil
}

// threadReplyText reads a reply's textual content, tolerating common text keys
// and one level of nesting (e.g. {"text":{"content":"..."}}).
func threadReplyText(m map[string]any) any {
	for _, key := range []string{"text", "content", "msgContent", "message", "body"} {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		case map[string]any:
			for _, inner := range []string{"content", "text", "value"} {
				if s, ok := t[inner].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return nil
}

// threadReplyCreateTime reads a reply's create/send time, returning the raw
// value under whichever candidate key is present.
func threadReplyCreateTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "createAt", "timestamp", "time"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func init() {
	shortcut.Register(ThreadReplies)
}
