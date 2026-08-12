// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import "strings"

// StreamingCardContractVersion identifies the additive card receipt emitted by
// high-level card shortcuts.
const StreamingCardContractVersion = "im.streaming-card.v1"

// ProjectStreamingCardReceipt publishes every server-returned identifier in a
// single cardRef. referencePairAvailable means this response contained both
// the update identifier and the visible message identifiers; it does not claim
// that older messages can be resolved without server-side mapping support.
func ProjectStreamingCardReceipt(created map[string]any, bizID string) map[string]any {
	bizID = strings.TrimSpace(bizID)
	messageID := firstSendStatusString(created, "openMessageId", "messageId", "msgId")
	conversationID := firstSendStatusString(created, "openConversationId", "conversationId", "openCid")
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
		"cardRef":                cardRef,
		"referencePairAvailable": pairAvailable,
		"created":                created,
		"nextActions":            []map[string]any{},
	}
	if bizID != "" {
		payload["nextActions"] = []map[string]any{{
			"cliPath": "chat +messages-update-card",
			"arguments": map[string]any{
				"biz-id": bizID,
			},
			"requiredArguments": []string{"content", "flow-status"},
			"ready":             false,
		}}
	}
	if !pairAvailable {
		payload["capabilityGap"] = "服务端尚未同时返回 bizId、openMessageId 和 openConversationId；CLI 只能保留本次响应，不能据此承诺从历史消息反向恢复 bizId"
	}
	return payload
}

// ProjectStreamingCardUpdate preserves the lower response while making the
// verified target explicit for downstream consumers.
func ProjectStreamingCardUpdate(updated map[string]any, bizID, proof string) map[string]any {
	payload := cloneSendStatusMap(updated)
	payload["contractVersion"] = StreamingCardContractVersion
	payload["cardRef"] = map[string]any{"bizId": strings.TrimSpace(bizID)}
	payload["verified"] = true
	payload["verificationEvidence"] = proof
	return payload
}
