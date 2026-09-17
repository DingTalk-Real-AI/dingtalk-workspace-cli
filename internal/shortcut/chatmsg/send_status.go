// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import (
	"strings"
)

// MessageSendStatusContractVersion identifies the additive workflow fields
// projected by the high-level send-status shortcut. The lower response fields
// remain at their original locations for compatibility.
const MessageSendStatusContractVersion = "im.message-send-status.v1"

// MessageSendReceiptContractVersion identifies the additive receipt attached
// to high-level current-user send results.
const MessageSendReceiptContractVersion = "im.message-send-receipt.v1"

// ProjectMessageSendReceipt connects a send result to its asynchronous status
// query without treating openTaskId as a message identifier.
func ProjectMessageSendReceipt(raw map[string]any) map[string]any {
	taskID := firstSendStatusString(raw, "openTaskId", "taskId")
	messageID := firstSendStatusString(raw, "openMessageId", "messageId", "msgId")
	conversationID := firstSendStatusString(raw, "openConversationId", "conversationId", "openCid")
	ready := messageID != "" && conversationID != ""
	receipt := map[string]any{
		"contractVersion":        MessageSendReceiptContractVersion,
		"openTaskId":             taskID,
		"readyForMessageActions": ready,
		"nextActions":            []map[string]any{},
	}
	if messageID != "" || conversationID != "" {
		messageRef := map[string]any{}
		if messageID != "" {
			messageRef["openMessageId"] = messageID
		}
		if conversationID != "" {
			messageRef["openConversationId"] = conversationID
		}
		receipt["messageRef"] = messageRef
	}
	switch {
	case ready:
		receipt["nextActions"] = sendStatusNextActions(taskID, messageID, conversationID, true)
	case taskID != "":
		receipt["nextActions"] = []map[string]any{{
			"cliPath": "chat +messages-query-send-status",
			"arguments": map[string]any{
				"open-task-id": taskID,
			},
			"ready": true,
			"when":  "需要确认投递结果或取得真实消息 ID 时",
		}}
	default:
		receipt["capabilityGap"] = "下层发送响应未返回 openTaskId 或完整 messageRef，CLI 无法生成后续状态查询"
	}
	return receipt
}

// ProjectMessageSendStatus preserves the lower response and adds a stable
// receipt that connects openTaskId to the message identifiers required by
// edit, recall, and read-status. It never manufactures a message reference:
// downstream actions are marked ready only when both IDs are actually present.
func ProjectMessageSendStatus(raw map[string]any, requestedTaskID string) map[string]any {
	payload := cloneSendStatusMap(raw)
	taskID := firstSendStatusString(payload, "openTaskId", "taskId")
	if taskID == "" {
		taskID = strings.TrimSpace(requestedTaskID)
	}
	messageID := firstSendStatusString(payload, "openMessageId", "messageId", "msgId")
	conversationID := firstSendStatusString(payload, "openConversationId", "conversationId", "openCid")

	payload["contractVersion"] = MessageSendStatusContractVersion
	payload["openTaskId"] = taskID
	messageRef := map[string]any{}
	if messageID != "" {
		messageRef["openMessageId"] = messageID
	}
	if conversationID != "" {
		messageRef["openConversationId"] = conversationID
	}
	if len(messageRef) > 0 {
		payload["messageRef"] = messageRef
	}
	ready := messageID != "" && conversationID != ""
	payload["readyForMessageActions"] = ready
	payload["nextActions"] = sendStatusNextActions(taskID, messageID, conversationID, ready)
	return payload
}

func sendStatusNextActions(taskID, messageID, conversationID string, ready bool) []map[string]any {
	if !ready {
		return []map[string]any{{
			"cliPath": "chat message query-send-status",
			"arguments": map[string]any{
				"open-task-id": taskID,
			},
			"ready": false,
			"when":  "投递任务尚未返回 openMessageId 和 openConversationId 时稍后重查",
		}}
	}
	messageArgs := map[string]any{
		"conversation-id": conversationID,
		"msg-id":          messageID,
	}
	return []map[string]any{
		{
			"cliPath":   "chat message recall",
			"arguments": cloneSendStatusMap(messageArgs),
			"ready":     true,
		},
		{
			"cliPath": "chat message edit",
			"arguments": map[string]any{
				"conversation-id": conversationID,
				"msg-id":          messageID,
			},
			"requiredArguments": []string{"text 或 content"},
			"ready":             false,
		},
		{
			"cliPath": "chat message read-status",
			"arguments": map[string]any{
				"conversation-id": conversationID,
				"message-id":      messageID,
			},
			"ready": true,
		},
	}
}

func firstSendStatusString(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if candidate, ok := typed[key].(string); ok && strings.TrimSpace(candidate) != "" {
				return strings.TrimSpace(candidate)
			}
		}
		for _, key := range []string{"result", "data", "response", "content", "message"} {
			if candidate := firstSendStatusString(typed[key], keys...); candidate != "" {
				return candidate
			}
		}
	case []any:
		for _, item := range typed {
			if candidate := firstSendStatusString(item, keys...); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func cloneSendStatusMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source)+5)
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
