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
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

var dingTalkMessageLocation = time.FixedZone("CST", 8*60*60)

func formatDingTalkMessageBoundary(now time.Time) string {
	return now.In(dingTalkMessageLocation).Format("2006-01-02 15:04:05")
}

// ChatMessages: fetch the message list of one conversation (group OR single
// chat) and print a clean projected list (speaker / text / time) instead of a
// raw dump.
//
// Steps:
//  1. depending on whether --group or --user is given, call either
//     list_conversation_message_v2 (group; param openconversation_id) or
//     list_individual_chat_message (single chat; param userId) on the chat
//     server — tool names and param keys copied verbatim from chat.go's
//     `dws chat message list` call sites;
//  2. defensively unwrap the message list (multiple candidate container keys)
//     and project each message to {sender, text, createTime} tolerating field
//     aliases and one level of nesting;
//  3. print via rt.Output as {messages, count} so it honours --format/--jq/--fields.
//
// The default path only reads and reshapes conversation messages;
// --download-resources additionally writes resource files locally.
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
		"省略 --time 时默认从当前时间向前读取最近消息；也可指定时间边界并用 --direction newer/older 控制方向。" +
		"内部据此调用群聊或单聊的消息列表接口，再在本地投影出每条消息的发言人、文本和时间。" +
		"默认只读且不会发送或修改任何消息；--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘，按既有安全下载约定无需交互确认。",
	Risk: shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群会话 ID（openConversationId），与 --user 互斥"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "--group 的别名", Hidden: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "单聊对方的 userId，与 --group 互斥"},
		{Name: "open-dingtalk-id", Type: shortcut.FlagString, Desc: "单聊对方的 openDingTalkId，与 --group/--user 互斥"},
		{Name: "time", Type: shortcut.FlagString, Desc: "时间边界，如 \"2025-03-01 00:00:00\"；省略时从当前时间向前读取最近消息"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页拉取的消息条数（可选）"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "--limit 的旧版别名", Hidden: true},
		{Name: "direction", Type: shortcut.FlagString, Enum: []string{"newer", "older"}, Desc: "时间方向 newer/older；省略时为 older，从时间边界向前读取"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出消息 reaction（默认输出）"},
	}, chatshortcut.MessageResourceDownloadFlags()...),
	Constraints: append([]shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "conversation-id", "id", "user", "open-dingtalk-id"}},
	}, chatshortcut.MessageResourceDownloadConstraints()...),
	Tips: []string{
		`dws chat +chat-messages --group <openconversation_id> --time "2025-03-01 00:00:00"`,
		`dws chat +chat-messages --user <userId> --time "2025-03-01 00:00:00" --limit 50`,
		`dws chat +chat-messages --group <openconversation_id> --direction older`,
	},
	Validate: chatshortcut.ValidateMessageResourceDownload,
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — build params and pick the right tool. Param keys
		// (openconversation_id / userId / time / forward / limit) match the MCP server
		// schema for group and direct message listing.
		var tool string
		params := map[string]any{}
		fallbackConversationID := ""

		if rt.Changed("time") && rt.Str("time") != "" {
			params["time"] = rt.Str("time")
		} else {
			params["time"] = formatDingTalkMessageBoundary(time.Now())
		}
		if limit := rt.IntFirst("limit", "size"); limit > 0 {
			params["limit"] = limit
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
		} else {
			params["forward"] = false
		}

		if group := rt.StrFirst("group", "conversation-id", "id"); group != "" {
			tool = "list_conversation_message_v2"
			params["openconversation_id"] = group
			fallbackConversationID = group
		} else if openID := strings.TrimSpace(rt.Str("open-dingtalk-id")); openID != "" {
			tool = "list_individual_chat_message"
			params["openDingTalkId"] = openID
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
			results = append(results, projectChatMessageWithReactions(m, !rt.Bool("no-reactions")))
		}

		payload := map[string]any{
			"messages": results,
			"count":    len(results),
		}
		direction := strings.TrimSpace(strings.ToLower(rt.Str("direction")))
		chatmsg.ApplyMessagePagination(payload, data, items, direction)
		if rt.Bool("download-resources") {
			payload["resourceDownloads"] = chatshortcut.DownloadMessageResources(rt, items, fallbackConversationID)
		}
		return rt.Output(payload)
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

// projectChatMessage reshapes one raw message into the clean
// {sender, text, createTime} projection, rendering card/auto-reply JSON and
// marking encrypted messages via chatmsg, and recursively expanding forwarded
// chat records under "forwarded".
func projectChatMessage(m map[string]any) map[string]any {
	return projectChatMessageWithReactions(m, true)
}

func projectChatMessageWithReactions(m map[string]any, includeReactions bool) map[string]any {
	row := map[string]any{
		"sender":     chatmsg.Sender(m),
		"text":       chatmsg.Text(m),
		"createTime": chatmsg.CreateTime(m),
	}
	if messageID := chatmsg.MessageID(m); messageID != nil {
		row["messageId"] = messageID
	}
	if conversationID := chatmsg.ConversationID(m); conversationID != nil {
		row["conversationId"] = conversationID
	}
	if threadID := chatmsg.ThreadID(m); threadID != nil {
		row["threadId"] = threadID
	}
	if messageType := chatmsg.MessageType(m); messageType != nil {
		row["messageType"] = messageType
	}
	if updateTime := chatmsg.UpdateTime(m); updateTime != nil {
		row["updateTime"] = updateTime
	}
	if includeReactions {
		if reactions := chatmsg.Reactions(m); len(reactions) > 0 {
			row["reactions"] = reactions
		}
	}
	if quoted := chatmsg.QuotedMessage(m); len(quoted) > 0 {
		row["quotedMessage"] = quoted
	}
	if resources := chatmsg.ResourcesDeep(m); len(resources) > 0 {
		row["resourceRefs"] = resources
	}
	projectForwarded := func(item map[string]any) map[string]any {
		return projectChatMessageWithReactions(item, includeReactions)
	}
	if forwarded := chatmsg.Forwarded(m, projectForwarded); len(forwarded) > 0 {
		row["forwarded"] = forwarded
	}
	return row
}

func init() {
	shortcut.Register(ChatMessages)
}
