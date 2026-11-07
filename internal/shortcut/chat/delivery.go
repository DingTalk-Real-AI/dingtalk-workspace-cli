// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"fmt"
	"io"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func chatDeliveryFlags() []shortcut.Flag {
	return []shortcut.Flag{
		{Name: "body-stdin", Type: shortcut.FlagBool, Desc: "从 stdin 读取最多 256 KiB 正文；选中的正文参数必须为 -，不改变默认字面量行为"},
		{Name: "wait-delivery", Type: shortcut.FlagBool, Desc: "仅个人文本/Markdown 发送或普通引用回复：查询发送回执并输出标准回执字段；未知状态不得重发"},
		{Name: "employee-context", Type: shortcut.FlagString, Desc: "已绑定宿主的 JSON 元数据（agentUuid/channel/bindingRevision）；发送前校验绑定，私聊仅限固定主管；必须配合 body-stdin、wait-delivery 和幂等键，仍须用户确认"},
	}
}

// 只有显式启用 stdin 的调用才解释 -；旧调用仍可发送字面量 -。
func prepareChatBody(rt *shortcut.RuntimeContext, names ...string) error {
	if !rt.Bool("body-stdin") {
		return nil
	}
	var selected string
	for _, name := range names {
		if rt.Command().Flags().Changed(name) {
			if selected != "" {
				return fmt.Errorf("--body-stdin requires exactly one body flag")
			}
			selected = name
		}
	}
	if selected == "" || rt.Str(selected) != "-" {
		return fmt.Errorf("--body-stdin requires a body flag with value -")
	}
	body, err := io.ReadAll(io.LimitReader(rt.Command().InOrStdin(), (256<<10)+1))
	if err != nil {
		return fmt.Errorf("cannot read chat body from stdin")
	}
	if len(body) > 256<<10 || strings.TrimSpace(string(body)) == "" {
		return fmt.Errorf("chat stdin body must be non-empty and at most 256 KiB")
	}
	return rt.Command().Flags().Set(selected, string(body))
}

func validateEmployeeReply(rt *shortcut.RuntimeContext) error {
	return helpers.ValidateEmployeeChatContext(rt.Command(), "reply", replyConversationID(rt), replyMessageID(rt), "", rt.StrFirst("idempotency-key", "uuid"))
}

func validateEmployeeSend(rt *shortcut.RuntimeContext) error {
	return helpers.ValidateEmployeeChatContext(rt.Command(), "operator-private", "", "", rt.StrFirst("open-dingtalk-id", "user-id"), rt.StrFirst("idempotency-key", "uuid"))
}
