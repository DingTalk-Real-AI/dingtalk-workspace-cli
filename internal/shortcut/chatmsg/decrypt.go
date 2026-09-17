// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import (
	"context"
	"fmt"
	"strings"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
)

// messageDecryptClient mirrors helpers.chatCryptoClient: the app wiring builds
// one crypto client and injects it into both consumers.
var messageDecryptClient = messagecrypto.DefaultClient()

type decryptItemKey struct {
	conversationID string
	messageID      string
}

type decryptTarget struct {
	message    map[string]any
	contentKey string
}

// SetMessageDecryptClient injects the app-owned SafeChat/Ding crypto client
// for the smart read shortcuts. nil falls back to the default (backend-less)
// client.
func SetMessageDecryptClient(client *messagecrypto.Client) {
	if client == nil {
		messageDecryptClient = messagecrypto.DefaultClient()
		return
	}
	messageDecryptClient = client
}

// DecryptChatMessageItems decrypts encrypted message items in place before
// projection, mirroring the atomic helpers chat read path. It returns the
// decrypt ledger payload fields, or nil when decryption must not run
// (dry-run, missing runtime, or a stub build without the SafeChat backend).
// It never returns an error: per-message failures land in the ledger and the
// command exit code stays unchanged.
func DecryptChatMessageItems(ctx context.Context, rt messagecrypto.Runtime, items []map[string]any) map[string]any {
	if ctx == nil || rt == nil || rt.DryRun() ||
		messageDecryptClient == nil || messageDecryptClient.BackendReady == nil || !messageDecryptClient.BackendReady() {
		return nil
	}
	batchItems := make([]messagecrypto.BatchDecryptItem, 0)
	targets := map[decryptItemKey][]decryptTarget{}
	for _, item := range items {
		messageID := strings.TrimSpace(fmt.Sprint(MessageID(item)))
		if messageID == "" || messageID == "<nil>" {
			continue
		}
		contentKey, ciphertext := encryptedItemContent(item)
		if contentKey == "" {
			continue
		}
		conversationID := strings.TrimSpace(fmt.Sprint(ConversationID(item)))
		if conversationID == "<nil>" {
			conversationID = ""
		}
		key := newDecryptItemKey(messageID, conversationID)
		targets[key] = append(targets[key], decryptTarget{message: item, contentKey: contentKey})
		batchItems = append(batchItems, messagecrypto.BatchDecryptItem{
			MessageID:      messageID,
			ConversationID: conversationID,
			Ciphertext:     ciphertext,
		})
	}
	decryptCandidateCount := len(batchItems)
	batchItems, policyFailures := filterDecryptItemsByPolicy(ctx, rt, batchItems)
	ledger := map[string]any{
		"decryptCandidateCount": decryptCandidateCount,
		"decryptAllowedCount":   len(batchItems),
		"decryptedCount":        0,
		"decryptFailedCount":    len(policyFailures),
	}
	if len(policyFailures) > 0 {
		ledger["decryptFailures"] = policyFailures
		ledger["partial"] = true
	}
	if len(batchItems) == 0 {
		return ledger
	}
	result, err := messageDecryptClient.BatchDecryptInbound(ctx, rt, messagecrypto.Options{}, batchItems)
	if err != nil {
		failures := append([]map[string]any{}, policyFailures...)
		failures = append(failures, map[string]any{"stage": "message-decrypt", "reason": err.Error()})
		ledger["decryptFailedCount"] = len(failures)
		ledger["decryptFailures"] = failures
		ledger["partial"] = true
		return ledger
	}
	decryptedCount := 0
	failures := make([]map[string]any, 0)
	for _, item := range result.Items {
		if item.Status != "" && item.Status != "success" {
			failures = append(failures, decryptFailure(item.MessageID, item.ConversationID, item.Reason))
			continue
		}
		if strings.TrimSpace(item.PlaintextContent) == "" {
			failures = append(failures, decryptFailure(item.MessageID, item.ConversationID, "empty_plaintext"))
			continue
		}
		matched := targets[newDecryptItemKey(item.MessageID, item.ConversationID)]
		if len(matched) == 0 {
			continue
		}
		for _, target := range matched {
			target.message[target.contentKey] = item.PlaintextContent
			target.message["contentDecrypted"] = true
			target.message["cryptoLayer"] = "ding+safechat"
			if item.KeyVersion > 0 {
				target.message["dingKeyVersion"] = item.KeyVersion
			}
		}
		decryptedCount++
	}
	for _, item := range result.Failures {
		failures = append(failures, decryptFailure(item.MessageID, item.ConversationID, item.Reason))
	}
	failures = append(policyFailures, failures...)
	ledger["decryptedCount"] = decryptedCount
	ledger["decryptFailedCount"] = len(failures)
	if len(failures) > 0 {
		ledger["decryptFailures"] = failures
		ledger["partial"] = true
	}
	return ledger
}

func newDecryptItemKey(messageID, conversationID string) decryptItemKey {
	return decryptItemKey{
		conversationID: strings.TrimSpace(conversationID),
		messageID:      strings.TrimSpace(messageID),
	}
}

// MergeDecryptLedger adds the decrypt ledger fields into an output payload.
// Counters accumulate, decryptFailures append, and partial is kept as an or so
// an earlier aggregation failure is never overwritten. A nil ledger (decrypt
// did not run) is a no-op so payloads stay byte-identical to the pre-decrypt
// output.
func MergeDecryptLedger(payload map[string]any, ledger map[string]any) {
	if payload == nil || ledger == nil {
		return
	}
	for _, key := range []string{"decryptCandidateCount", "decryptAllowedCount", "decryptedCount", "decryptFailedCount"} {
		value, ok := ledger[key].(int)
		if !ok {
			continue
		}
		existing, _ := payload[key].(int)
		payload[key] = existing + value
	}
	if failures, ok := ledger["decryptFailures"].([]map[string]any); ok && len(failures) > 0 {
		existing, _ := payload["decryptFailures"].([]map[string]any)
		merged := make([]map[string]any, 0, len(existing)+len(failures))
		merged = append(merged, existing...)
		merged = append(merged, failures...)
		payload["decryptFailures"] = merged
	}
	if partial, _ := ledger["partial"].(bool); partial {
		payload["partial"] = true
	}
}

func filterDecryptItemsByPolicy(ctx context.Context, rt messagecrypto.Runtime, items []messagecrypto.BatchDecryptItem) ([]messagecrypto.BatchDecryptItem, []map[string]any) {
	filtered := make([]messagecrypto.BatchDecryptItem, 0, len(items))
	failures := make([]map[string]any, 0)
	for _, item := range items {
		decision, err := messageDecryptClient.PolicyDecision(ctx, rt, messagecrypto.Options{
			Identity:           "user",
			MsgType:            "text",
			OpenConversationID: item.ConversationID,
		})
		if err != nil {
			failures = append(failures, decryptFailure(item.MessageID, item.ConversationID, err.Error()))
			continue
		}
		if !decision.Enabled {
			failures = append(failures, decryptFailure(item.MessageID, item.ConversationID, "policy_disabled"))
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, failures
}

func decryptFailure(messageID, conversationID, reason string) map[string]any {
	failure := map[string]any{
		"stage":     "message-decrypt",
		"messageId": strings.TrimSpace(messageID),
		"reason":    firstNonEmptyDecryptReason(reason, "decrypt_failed"),
	}
	if strings.TrimSpace(conversationID) != "" {
		failure["conversationId"] = strings.TrimSpace(conversationID)
	}
	return failure
}

// encryptedItemContent mirrors the atomic path's content probe: the first
// present key of content/text wins, and only a ciphertext-bearing value is a
// decrypt candidate. The key is returned so the plaintext rewrite lands where
// the smart projections read it back (they prefer text over content).
func encryptedItemContent(item map[string]any) (string, string) {
	for _, key := range []string{"content", "text"} {
		if value, ok := item[key]; ok {
			content := strings.TrimSpace(fmt.Sprint(value))
			if IsEncrypted(content) {
				return key, content
			}
			return "", ""
		}
	}
	return "", ""
}

func firstNonEmptyDecryptReason(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
