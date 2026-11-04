// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import "encoding/json"

// 确定交付失败时，单独通知原请求，避免卡片和文字状态两个投影同时失败后一直显示处理中。
// 通知只包含固定状态，不回放答案、不重跑 Agent；回执未知时也不自动重发。
func (r *employeeRuntime) notifyCardFailure(e employeeEvent, record *employeeTaskRecord) error {
	if record.Delivery != "failure" || record.CardNoticeDelivery != "" || r.ctx.Err() != nil {
		return nil
	}
	if !r.accept(e) {
		return employeeTerminal("reply_not_authorized")
	}
	record.CardNoticeDelivery = "unknown"
	if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
		return employeeTerminal("ledger_unavailable")
	}
	metadata, _ := json.Marshal(EmployeeChatContext{AgentUUID: r.cfg.Binding.AgentUUID, Channel: bindingChannel(r.cfg.Binding), BindingRevision: r.cfg.Binding.BindingRevision})
	text := "本次回复卡片发送失败，已停止更新。请联系数字员工主管核查本轮结果，避免重复执行操作。"
	result, err := employeeMachineCall(r.ctx, r.cfg.Binding.DWSProfile, employeeChatBody(text),
		"chat", "+messages-reply", "--group", e.ConversationID, "--message-id", e.MessageID,
		"--content", "-", "--body-stdin", "--wait-delivery", "--employee-context", string(metadata),
		"--idempotency-key", record.IdempotencyKey+"-card-failure", "--yes", "--format", "json")
	if err == nil && jsonScalar(result["deliveryStatus"]) == "delivered" && jsonScalar(result["openMessageId"]) != "" {
		record.CardNoticeDelivery = "delivered"
		record.CardNoticeReplyID = jsonScalar(result["openMessageId"])
	}
	return nil
}
