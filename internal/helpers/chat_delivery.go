// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ChatQuoteContent 统一普通聊天和兼容 Channel 入口的文本引用格式。
func ChatQuoteContent(messageID, sender, text string) string {
	content, _ := json.Marshal(map[string]string{
		"referenceOpenMessageId": messageID, "srcMsgSendOpenDingTalkId": sender,
		"replyMsgType": "text", "content": text,
	})
	return string(content)
}

func chatDeliveryResult(result map[string]any, conversationID, idempotencyKey string) map[string]any {
	openMessageID := firstJSONScalar(result, "openMessageId", "openMsgId", "messageId", "msgId")
	if conversationID == "" {
		conversationID = firstJSONScalar(result, "conversationId", "openConversationId", "openConvThreadId")
	}
	delivery := strings.ToLower(firstJSONScalar(result, "deliveryStatus", "sendStatus", "status"))
	if delivery != "delivered" && delivery != "success" && delivery != "accepted" {
		delivery = "unknown"
	} else {
		delivery = "delivered"
	}
	return map[string]any{
		"openMessageId": openMessageID, "conversationId": conversationID,
		"deliveryStatus": delivery, "idempotencyKey": idempotencyKey,
	}
}

// ResolveChatDelivery 查询已有发送任务；任何失败都不能触发重新发送。
func ResolveChatDelivery(ctx context.Context, sendResult map[string]any, conversationID, idempotencyKey string) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	delivery := chatDeliveryResult(sendResult, conversationID, idempotencyKey)
	if strings.TrimSpace(jsonScalar(delivery["openMessageId"])) != "" {
		return delivery, nil
	}
	taskID := firstJSONScalar(sendResult, "openTaskId", "taskId")
	if taskID == "" {
		return nil, apperrors.NewInternal("数字员工发送响应缺少 openMessageId 和 openTaskId，无法确认投递结果")
	}
	for attempt := 0; attempt < digitalEmployeeReceiptAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			if err := deapChannelReceiptWait(ctx, digitalEmployeeReceiptInterval); err != nil {
				return nil, err
			}
		}
		status, err := callMachineMCPJSON(ctx, "im", "query_message_send_status", map[string]any{"openTaskId": taskID})
		if errors.Is(err, errEmployeeReceiptNotVisible) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("query digital employee message status: %w", err)
		}
		delivery = chatDeliveryResult(status, conversationID, idempotencyKey)
		if strings.TrimSpace(jsonScalar(delivery["openMessageId"])) != "" {
			return delivery, nil
		}
	}
	return nil, apperrors.NewInternal("数字员工发送任务未在有限等待时间内返回 openMessageId，投递状态未知，请勿重新发送",
		apperrors.WithReason("delivery_unknown"), apperrors.WithRetryable(false),
		apperrors.WithHint("消息可能已经送达，请先核对原会话；不要重跑发送或 Agent 任务。"))
}

func waitForDigitalEmployeeReceipt(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
