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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ChatMessages: fetch the message list of one conversation (group OR single
// chat) and print a clean projected list (speaker / text / time) instead of a
// raw dump.
//
// Steps:
//  1. depending on whether --group or --user is given, call either
//     list_conversation_message_v2 (group; param openCid) or
//     list_individual_chat_message (single chat; param userId) on the chat
//     server — tool names and param keys copied verbatim from chat.go's
//     `dws chat message list` call sites;
//  2. defensively unwrap the message list (multiple candidate container keys)
//     and project each message to {sender, text, createTime} tolerating field
//     aliases and one level of nesting;
//  3. print via rt.Output as {messages, count} so it honours --format/--jq/--fields.
//
// Read-only: it only reads a conversation's messages and reshapes them locally,
// never posts or mutates anything.
//
//	dws chat +chat-messages --group <openconversation_id> --time "2025-03-01 00:00:00"
//	dws chat +chat-messages --user <userId> --time "2025-03-01 00:00:00" --limit 50
var ChatMessages = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-messages",
	Product:     "chat",
	Description: "拉取某个会话（群聊或单聊）的消息列表并投影出发言人/文本/时间",
	Intent: "当你想快速看某个会话里的消息（谁在什么时间说了什么），而不想拿到一大坨原始消息字段时使用；" +
		"群聊传 --group（群会话 ID，openConversationId），单聊传 --user（对方 userId），两者互斥且必须二选一。" +
		"可选 --time 指定起始时间、--limit 指定每页条数、--direction newer/older 控制时间方向（newer 从给定时间往现在拉，older 往以前拉）。" +
		"内部据此调用群聊或单聊的消息列表接口，再在本地投影出每条消息的发言人、文本和时间。" +
		"这是纯只读操作，只做拉取与本地投影，不会发送或修改任何消息。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群会话 ID（openConversationId），与 --user 互斥"},
		{Name: "user", Type: shortcut.FlagString, Desc: "单聊对方的 userId，与 --group 互斥"},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"（可选）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页拉取的消息条数（可选）"},
		{Name: "direction", Type: shortcut.FlagString, Desc: "时间方向 newer/older（可选，newer 从给定时间往现在拉，older 往以前拉）"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "user"}},
	},
	Tips: []string{
		`dws chat +chat-messages --group <openconversation_id> --time "2025-03-01 00:00:00"`,
		`dws chat +chat-messages --user <userId> --time "2025-03-01 00:00:00" --limit 50`,
		`dws chat +chat-messages --group <openconversation_id> --direction older`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — build params and pick the right tool. Param keys
		// (openCid/cid / userId / time / forward / limit) match the MCP server
		// schema for group and direct message listing.
		var tool string
		params := map[string]any{}

		if rt.Changed("time") && rt.Str("time") != "" {
			params["time"] = rt.Str("time")
		}
		if rt.Changed("limit") && rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		// direction newer/older maps to the tools' boolean `forward` param
		// (newer -> forward=true, older -> forward=false), matching chat.go's
		// resolveMessageForward.
		if rt.Changed("direction") {
			switch strings.TrimSpace(strings.ToLower(rt.Str("direction"))) {
			case "newer":
				params["forward"] = true
			case "older":
				params["forward"] = false
			}
		}

		if rt.Str("group") != "" {
			tool = "list_conversation_message_v2"
			params["openCid"] = rt.Str("group")
			params["cid"] = rt.Str("group")
		} else {
			tool = "list_individual_chat_message"
			params["userId"] = rt.Str("user")
		}

		data, err := rt.CallMCPData("chat", tool, params)
		if err != nil {
			return err
		}

		// Step 2 — defensively unwrap and project. Response shape has no
		// contract, so probe multiple candidate container/field keys.
		items := chatMessageItems(data)
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"sender":     chatMessageSender(m),
				"text":       chatMessageText(m),
				"createTime": chatMessageCreateTime(m),
			})
		}

		return rt.Output(map[string]any{
			"messages": results,
			"count":    len(results),
		})
	},
}

// chatMessageItems defensively unwraps the message list from the response,
// tolerating the common container keys and one level of nesting under a
// "result"/"data" wrapper.
func chatMessageItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	scopes := []map[string]any{data}
	for _, wrap := range []string{"result", "data"} {
		if inner, ok := data[wrap].(map[string]any); ok {
			scopes = append(scopes, inner)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"messages", "list", "items", "records", "data", "result"} {
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

// chatMessageSender reads a message's speaker display name, tolerating common
// sender-name keys.
func chatMessageSender(m map[string]any) any {
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

// chatMessageText reads a message's textual content, tolerating common text
// keys and one level of nesting (e.g. {"content":{"text":"..."}}).
func chatMessageText(m map[string]any) any {
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
			for _, inner := range []string{"text", "content", "value"} {
				if s, ok := t[inner].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return nil
}

// chatMessageCreateTime reads a message's create/send time, returning the raw
// value under whichever candidate key is present.
func chatMessageCreateTime(m map[string]any) any {
	for _, key := range []string{"createTime", "sendTime", "gmtCreate", "createAt", "timestamp", "time"} {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func init() {
	shortcut.Register(ChatMessages)
}
