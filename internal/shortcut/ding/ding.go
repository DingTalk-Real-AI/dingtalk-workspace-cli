// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package ding registers strict declarative shortcuts for DingTalk DING.
package ding

import (
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var List = shortcut.Shortcut{
	Service: "ding", Command: "+list", Product: "im",
	Description:   "查询 DING 消息列表",
	Intent:        "需要查看 DING 历史、取得稳定 openDingId，或按 ALL、UNREAD、SEND、NEW_COMMENT、DELETED 类型读取一页时使用。",
	Risk:          shortcut.RiskRead,
	Safety:        dingReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: dingContract(
		"+list", "查询 DING 消息列表",
		"需要查看 DING 历史、取得稳定 openDingId，或按 ALL、UNREAD、SEND、NEW_COMMENT、DELETED 类型读取一页时使用。",
		true, dingListResult(), dingPagination(),
		[]contract.ParamDecl{{Name: "cursor", Property: "cursor"}, {Name: "type", Property: "type"}},
		"dws ding +list --type ALL", "dws ding +list --type DELETED",
	),
	Flags: []shortcut.Flag{
		{Name: "cursor", Type: shortcut.FlagInt, Desc: "分页游标，首次不传或传 0"},
		{Name: "type", Type: shortcut.FlagString, Default: "ALL", Desc: "DING 类型：ALL、UNREAD、SEND、NEW_COMMENT 或 DELETED"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"cursor"}, Description: "--cursor 不能小于 0，续页响应必须返回严格前进的正整数 nextCursor"}},
	Tips:        []string{"dws ding +list --type ALL", "dws ding +list --type DELETED"},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Int("cursor") < 0 {
			return apperrors.NewValidation("--cursor 不能小于 0")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"type": rt.Str("type")}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Int("cursor")
		}
		data, err := rt.CallMCPData("im", "list_ding_messages", params)
		if err != nil {
			return err
		}
		messages, page, err := dingProjectMessages(data, "im/list_ding_messages")
		if err != nil {
			return err
		}
		if page.HasMore {
			current := int64(rt.Int("cursor"))
			next, parseErr := strconv.ParseInt(page.Next, 10, 64)
			if parseErr != nil || next <= current {
				return dingResponseError("im/list_ding_messages", "stalled_cursor", "nextCursor 没有严格前进")
			}
		}
		return outputDingPage(rt, messages, page)
	},
}

var ReceiverStatus = shortcut.Shortcut{
	Service: "ding", Command: "+receiver-status", Product: "im",
	Description:   "查询 DING 消息接收人已读状态",
	Intent:        "已经从 +list 取得稳定 openDingId，需要确认该 DING 的接收状态时使用；这是精确 ID 查询，不是消息搜索。",
	Risk:          shortcut.RiskRead,
	Safety:        dingReadSafety(),
	OutputRollout: output.RolloutUnifiedActive,
	Contract: dingContract(
		"+receiver-status", "查询 DING 消息接收人已读状态",
		"已经从 +list 取得稳定 openDingId，需要确认该 DING 的接收状态时使用；这是精确 ID 查询，不是消息搜索。",
		true, dingReceiverResult(), nil,
		[]contract.ParamDecl{{Name: "ding-id", Property: "dingId"}},
		"dws ding +receiver-status --ding-id <DING_ID>",
	),
	Flags: []shortcut.Flag{{Name: "ding-id", Type: shortcut.FlagString, Desc: "openDingId", Required: true}},
	Tips:  []string{"dws ding +receiver-status --ding-id <DING_ID>"},
	Execute: func(rt *shortcut.RuntimeContext) error {
		const operation = "im/list_ding_receiver_status"
		data, err := rt.CallMCPData("im", "list_ding_receiver_status", map[string]any{"openDingId": rt.Str("ding-id")})
		if err != nil {
			return err
		}
		receivers, err := dingProjectReceivers(data, operation, rt.Str("ding-id"))
		if err != nil {
			return err
		}
		payload := map[string]any{"count": len(receivers), "receivers": receivers}
		if !output.UsesUnifiedResult(rt.Command()) {
			return rt.Output(payload)
		}
		meta := &output.Meta{Count: output.NewCount(len(receivers))}
		return output.StoreResult(rt.Command().Context(), output.Success(payload, output.WithMeta(meta)))
	},
}

var SendPersonal = compatibilityDingWrite(
	"+send-personal", "以本人身份发送 DING 给指定人",
	"仅为历史 CLI 兼容保留：明确确认后按原有参数发送；因接收人稳定身份与撤回终态尚不可验证，不进入 Agent 公共发现。",
	shortcut.RiskWrite, false,
	[]shortcut.Flag{
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "接收人 openDingTalkId 列表 (CSV)", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "消息内容", Required: true},
		{Name: "type", Type: shortcut.FlagString, Default: "app", Desc: "提醒方式：app、sms 或 call"},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键"},
	},
	[]contract.ParamDecl{
		{Name: "users", Property: "users", InterfaceType: "array"},
		{Name: "content", Property: "content"}, {Name: "type", Property: "type"}, {Name: "uuid", Property: "uuid"},
	},
	"dws ding +send-personal --users <VALUES> --content <CONTENT>",
	func(rt *shortcut.RuntimeContext) error {
		remindType, err := dingPersonalRemindType(rt.Str("type"))
		if err != nil {
			return err
		}
		params := map[string]any{
			"receiverOpenDingTalkIds": rt.StrSlice("users"),
			"content":                 rt.Str("content"),
			"remindType":              remindType,
		}
		if rt.Changed("uuid") {
			params["uuid"] = rt.Str("uuid")
		}
		return rt.CallMCP("send_personal_ding", params)
	},
)

var SendByMessage = unavailableDingWrite(
	"+send-by-message", "针对某条消息发起 DING 提醒",
	"需要把指定聊天消息转成应用内、短信或电话 DING 时使用；对应 lark-cli 的三种 urgent 任务，但接收人稳定身份与精确撤回终态仍缺失。",
	shortcut.RiskWrite, false,
	[]shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "openConversationId", Required: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "openMessageId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "接收人 openDingTalkId 列表 (CSV)", Required: true},
		{Name: "type", Type: shortcut.FlagString, Default: "app", Desc: "提醒方式", Enum: []string{"app", "sms", "call"}},
		{Name: "uuid", Type: shortcut.FlagString, Desc: "幂等键"},
	},
	[]contract.ParamDecl{
		{Name: "group", Property: "openConversationId"}, {Name: "message-id", Property: "openMessageId"},
		{Name: "users", Property: "receiverOpenDingTalkIds", InterfaceType: "array"},
		{Name: "type", Property: "remindType"}, {Name: "uuid", Property: "uuid"},
	},
	"dws ding +send-by-message --group <GROUP_ID> --message-id <MESSAGE_ID> --users <VALUES>",
)

var RecallPersonal = compatibilityDingWrite(
	"+recall-personal", "撤回本人发起的 DING",
	"仅为历史 CLI 兼容保留：明确确认后按稳定 openDingId 撤回；因终态与残留通知尚不可查询验证，不进入 Agent 公共发现。",
	shortcut.RiskHighWrite, true,
	[]shortcut.Flag{{Name: "id", Type: shortcut.FlagString, Desc: "openDingId", Required: true}},
	[]contract.ParamDecl{{Name: "id", Property: "id"}},
	"dws ding +recall-personal --id <DING_ID>",
	func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("recall_personal_ding", map[string]any{"openDingId": rt.Str("id")})
	},
)

func compatibilityDingWrite(command, description, intent string, risk shortcut.Risk, destructive bool, flags []shortcut.Flag, params []contract.ParamDecl, example string, execute func(*shortcut.RuntimeContext) error) shortcut.Shortcut {
	declaration := dingContract(command, description, intent, true, nil, nil, params, example)
	declaration.Interface.Reason = dingCompatibilityWriteReason
	return shortcut.Shortcut{
		Service: "ding", Command: command, Product: "im",
		Description: description, Intent: intent, Risk: risk,
		Safety: dingWriteSafety(destructive), OutputRollout: output.RolloutUnifiedActive,
		Contract: declaration,
		Flags:    flags, Tips: []string{example}, Execute: execute,
	}
}

func unavailableDingWrite(command, description, intent string, risk shortcut.Risk, destructive bool, flags []shortcut.Flag, params []contract.ParamDecl, example string) shortcut.Shortcut {
	return shortcut.Shortcut{
		Service: "ding", Command: command, Product: "im",
		Description: description, Intent: intent, Risk: risk,
		Safety: dingWriteSafety(destructive), OutputRollout: output.RolloutUnifiedActive,
		Contract: dingContract(command, description, intent, false, nil, nil, params, example),
		Flags:    flags, Tips: []string{example},
		Execute: func(*shortcut.RuntimeContext) error {
			return dingUnavailable("ding/" + command)
		},
	}
}

func dingPersonalRemindType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "app":
		return "APP", nil
	case "sms":
		return "SMS", nil
	case "call":
		return "PHONE", nil
	default:
		return "", apperrors.NewValidation("--type 必须是 app、sms 或 call")
	}
}

func init() {
	shortcut.Register(List, ReceiverStatus, SendPersonal, SendByMessage, RecallPersonal)
}
