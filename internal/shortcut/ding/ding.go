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

// Package ding holds the built-in DING (钉) shortcuts. Tool names and params are
// copied verbatim from internal/helpers/ding.go. Some tools route to the "im"
// MCP server (helper uses callMCPToolOnServer("im", ...)), reflected in Product.
package ding

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"

var List = shortcut.Shortcut{
	Service: "ding", Command: "+list", Product: "im",
	Description: "查询 DING 消息列表", Intent: "当你想查看当前身份收到或发出的 DING 消息、回顾有哪些强提醒或获取某条 DING 的 openDingId 以便后续查已读或撤回时使用；可选按类型过滤并用 cursor 翻页，只读不产生副作用。", Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "分页游标 (可选)"},
		{Name: "type", Type: shortcut.FlagString, Default: "ALL", Desc: "类型 (可选，默认 ALL)"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// type 是服务端必填，空值会报错；不传或传空时兜底为 ALL（对齐 helper）。
		msgType := rt.Str("type")
		if msgType == "" {
			msgType = "ALL"
		}
		params := map[string]any{"type": msgType}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Int("cursor")
		}
		return rt.CallMCP("list_ding_messages", params)
	},
}

var ReceiverStatus = shortcut.Shortcut{
	Service: "ding", Command: "+receiver-status", Product: "im",
	Description: "查询 DING 消息接收人已读状态", Intent: "当你发出一条 DING 后想确认每位接收人是否已读、追踪谁还没看到以便催办时使用；需提供该 DING 的 openDingId，返回各接收人的已读/未读状态，只读操作。", Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "ding-id", Type: shortcut.FlagString, Desc: "openDingId", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_ding_receiver_status", map[string]any{
			"openDingId": rt.Str("ding-id"),
		})
	},
}

var SendPersonal = shortcut.Shortcut{
	Service: "ding", Command: "+send-personal", Product: "im",
	Description: "以本人身份发送 DING 给指定人", Intent: "当你想以自己（而非机器人）的身份直接给某些同事发 DING 强提醒，让对方看到是本人发起时使用；需提供接收人的 openDingTalkId 列表和内容，可选提醒方式与幂等 uuid，会真实向这些人发出 DING。", Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "接收人 openDingTalkId 列表 (CSV)", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "消息内容", Required: true},
		{Name: "type", Type: shortcut.FlagString, Default: "app", Desc: "提醒方式 remindType"},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键 (可选)"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"receiverOpenDingTalkIds": rt.StrSlice("users"),
			"content":                 rt.Str("content"),
			"remindType":              rt.Str("type"),
		}
		if rt.Changed("uuid") {
			params["uuid"] = rt.Str("uuid")
		}
		return rt.CallMCP("send_personal_ding", params)
	},
}

var SendByMessage = shortcut.Shortcut{
	Service: "ding", Command: "+send-by-message", Product: "im",
	Description: "针对某条消息发起 DING 提醒", Intent: "当群里已有一条聊天消息需要被重点跟进，你想直接把它变成 DING 去强提醒相关人（而不是另写新内容）时使用；需提供群 openConversationId、该消息 openMessageId 和接收人 openDingTalkId 列表，会真实基于这条消息发出 DING。", Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "openConversationId", Required: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "openMessageId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "接收人 openDingTalkId 列表 (CSV)", Required: true},
		{Name: "type", Type: shortcut.FlagString, Default: "app", Desc: "提醒方式 remindType"},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键 (可选)"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId":      rt.Str("group"),
			"openMessageId":           rt.Str("message-id"),
			"receiverOpenDingTalkIds": rt.StrSlice("users"),
			"remindType":              rt.Str("type"),
		}
		if rt.Changed("uuid") {
			params["uuid"] = rt.Str("uuid")
		}
		return rt.CallMCP("send_ding_by_message", params)
	},
}

var RecallPersonal = shortcut.Shortcut{
	Service: "ding", Command: "+recall-personal", Product: "im",
	Description: "撤回本人发起的 DING", Intent: "当你以本人身份发出的某条 DING 发错人或内容有误、想收回时使用（对应 send-personal/send-by-message 发出的 DING）；需提供该 DING 的 openDingId，会真实撤回它，接收人将不再看到该提醒。", Risk: shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "openDingId", Required: true},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("recall_personal_ding", map[string]any{
			"openDingId": rt.Str("id"),
		})
	},
}

func init() {
	shortcut.Register(
		List,
		ReceiverStatus,
		SendPersonal,
		SendByMessage,
		RecallPersonal,
	)
}
