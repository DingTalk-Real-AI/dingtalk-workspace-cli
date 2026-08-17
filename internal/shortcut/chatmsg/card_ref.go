// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import "strings"

// StreamingCardContractVersion identifies the additive card receipt emitted by
// high-level card shortcuts.
const StreamingCardContractVersion = "im.streaming-card.v1"

// ProjectStreamingCardReceipt publishes the card update identifier and links
// the asynchronous send receipt to the existing status-query workflow.
// referencePairAvailable means this response contained both the update
// identifier and the visible message identifiers; it does not claim that older
// messages can be resolved without server-side mapping support.
func ProjectStreamingCardReceipt(created map[string]any, bizID string) map[string]any {
	bizID = strings.TrimSpace(bizID)
	messageID := firstSendStatusString(created, "openMessageId", "messageId", "msgId")
	conversationID := firstSendStatusString(created, "openConversationId", "conversationId", "openCid")
	sendReceipt := ProjectMessageSendReceipt(created)
	taskID, _ := sendReceipt["openTaskId"].(string)
	cardRef := map[string]any{}
	if bizID != "" {
		cardRef["bizId"] = bizID
	}
	if messageID != "" {
		cardRef["openMessageId"] = messageID
	}
	if conversationID != "" {
		cardRef["openConversationId"] = conversationID
	}
	pairAvailable := bizID != "" && messageID != "" && conversationID != ""
	payload := map[string]any{
		"contractVersion":        StreamingCardContractVersion,
		"ok":                     true,
		"bizId":                  bizID,
		"cardRef":                cardRef,
		"sendReceipt":            sendReceipt,
		"referencePairAvailable": pairAvailable,
		"created":                created,
		"nextActions":            []map[string]any{},
	}
	if taskID != "" {
		payload["openTaskId"] = taskID
	}
	actions := make([]map[string]any, 0, 2)
	if bizID != "" {
		actions = append(actions, map[string]any{
			"cliPath": "chat +messages-update-card",
			"arguments": map[string]any{
				"biz-id": bizID,
			},
			"requiredArguments": []string{"content", "flow-status"},
			"ready":             false,
		})
	}
	if !pairAvailable && taskID != "" {
		actions = append(actions, map[string]any{
			"cliPath": "chat message query-send-status",
			"arguments": map[string]any{
				"open-task-id": taskID,
			},
			"ready": true,
			"when":  "需要确认卡片消息投递结果或取得真实消息 ID 时",
		})
	}
	payload["nextActions"] = actions
	if !pairAvailable && taskID == "" {
		payload["capabilityGap"] = "服务端未返回 openTaskId，也未同时返回 openMessageId 和 openConversationId；CLI 无法衔接消息投递状态查询"
	}
	return payload
}

// ProjectStreamingCardUpdate preserves the lower response while making the
// verified target explicit for downstream consumers.
func ProjectStreamingCardUpdate(updated map[string]any, bizID string, verification CardUpdateVerification) map[string]any {
	payload := cloneSendStatusMap(updated)
	payload["contractVersion"] = StreamingCardContractVersion
	payload["cardRef"] = map[string]any{"bizId": strings.TrimSpace(bizID)}
	payload["accepted"] = verification.Accepted
	payload["verified"] = verification.Verified
	payload["verificationEvidence"] = verification.Evidence
	if verification.Accepted && !verification.Verified {
		payload["warning"] = "服务端已接受卡片更新请求，但未返回可独立证明可见内容已更新的字段；不要重复执行相同更新"
	}
	return payload
}
