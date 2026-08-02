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
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
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
// The default path only reads and reshapes topic replies;
// --download-resources additionally writes resource files locally.
//
//	dws chat +thread-replies --group <openconversationId> --thread-id <threadId>
var ThreadReplies = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+thread-replies",
	Product:     "chat",
	Description: "拉取某条话题消息的全部回复并投影出发言人/文本/时间",
	Intent: "当你已经拿到某个群里一条「话题消息」的 threadId/topicId、想快速看这条话题下的全部回复（谁在什么时间回复了什么），" +
		"而不想拿到一大坨原始消息字段时使用；内部按 --group（群会话 ID）和 --thread-id（兼容 --topic-id）拉取该话题的回复列表，" +
		"可选 --time 指定起始时间、--limit 指定每页条数，再在本地投影出每条回复的发言人、文本和回复时间。" +
		"默认只读且不会发送或修改任何消息；--download-resources 使用工作目录内安全路径、默认不覆盖和原子落盘，按既有安全下载约定无需交互确认。",
	Risk: shortcut.RiskRead,
	Flags: append([]shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群会话 ID（openConversationId，必填）", Required: true},
		{Name: "thread-id", Type: shortcut.FlagString, Desc: "话题/线程 ID（可直接使用消息列表返回的 threadId）"},
		{Name: "topic-id", Type: shortcut.FlagString, Desc: "--thread-id 的兼容别名"},
		{Name: "time", Type: shortcut.FlagString, Desc: "起始时间，如 \"2025-03-01 00:00:00\"（可选）"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页拉取的回复条数（可选）"},
		{Name: "no-reactions", Type: shortcut.FlagBool, Desc: "不输出回复 reaction（默认输出）"},
	}, chatshortcut.MessageResourceDownloadFlags()...),
	Constraints: append([]shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintExactlyOne,
			Flags:       []string{"thread-id", "topic-id"},
			Description: "--thread-id 与兼容参数 --topic-id 必须且只能指定一个",
		},
	}, chatshortcut.MessageResourceDownloadConstraints()...),
	Tips: []string{
		`dws chat +thread-replies --group <openconversationId> --thread-id <threadId>`,
		`dws chat +thread-replies --group <openconversationId> --thread-id <threadId> --time "2025-03-01 00:00:00" --limit 20`,
	},
	Validate: chatshortcut.ValidateMessageResourceDownload,
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — fetch the topic replies. Param keys (openconversationId /
		// topicId / startTime / pageSize) are copied verbatim from chat.go's
		// list-topic-replies call site.
		params := map[string]any{
			"openconversationId": rt.Str("group"),
			"topicId":            rt.StrFirst("thread-id", "topic-id"),
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
			results = append(results, projectChatMessageWithReactions(m, !rt.Bool("no-reactions")))
		}

		payload := map[string]any{
			"replies": results,
			"count":   len(results),
		}
		chatmsg.ApplyMessagePagination(payload, data, items, "older")
		if rt.Bool("download-resources") {
			payload["resourceDownloads"] = chatshortcut.DownloadMessageResources(rt, items, rt.Str("group"))
		}
		return rt.Output(payload)
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

func init() {
	shortcut.Register(ThreadReplies)
}
