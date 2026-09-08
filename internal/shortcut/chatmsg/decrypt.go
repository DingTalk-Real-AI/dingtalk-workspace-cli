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

// Policy-driven decrypt orchestration shared by the atomic helpers read
// commands and the read-message shortcuts. Keep this file stdlib-only plus
// internal/msgcrypto/message: internal/msgcrypto (root) imports internal/auth,
// which imports internal/helpers, which imports this package — importing the
// root here would create an import cycle.
package chatmsg

import (
	"context"
	"fmt"
	"strings"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
)

// messageDecryptFailedOriginalContentKey marks a raw message whose decrypt
// failed so shortcut projections can restore the original ciphertext into
// text. Projections that copy raw maps verbatim (helpers projectChatMessageItem)
// must never receive this key; DecryptOptions.MarkFailedOriginal gates it.
const messageDecryptFailedOriginalContentKey = "_contentDecryptFailedOriginal"

// messageDecryptClient is the process-wide crypto client store. It starts as
// the inert DefaultClient (no SafeChat backend in default builds) and is wired
// once by app via helpers.SetChatCryptoClient.
var messageDecryptClient = messagecrypto.DefaultClient()

// SetMessageDecryptClient injects the process-wide crypto client; nil resets
// to the inert DefaultClient. It mutates package-global state: production
// calls it once on the main goroutine during app startup, and tests must not
// call it from t.Parallel tests.
func SetMessageDecryptClient(client *messagecrypto.Client) {
	if client == nil {
		messageDecryptClient = messagecrypto.DefaultClient()
		return
	}
	messageDecryptClient = client
}

// MessageDecryptClient returns the process-wide crypto client.
func MessageDecryptClient() *messagecrypto.Client {
	return messageDecryptClient
}

// DecryptOptions tunes the shared decrypt pipeline per caller class.
type DecryptOptions struct {
	// MarkFailedOriginal writes messageDecryptFailedOriginalContentKey onto
	// failed raw message maps so downstream shortcut projections can restore
	// the original ciphertext into text. Shortcut callers pass true; the
	// atomic helpers projection passes the zero value because it copies raw
	// maps verbatim and must keep its ciphertext-content + marker-text shape.
	MarkFailedOriginal bool
}

// DecryptMessagesByPolicy runs gate → collect → policy filter → batch decrypt
// → write-back → canonical ledger over raw message maps. It returns nil iff
// gated (dry-run or no ready backend): no policy query, no decrypt call, and
// no decrypt field may be emitted. A non-nil return is the canonical ledger
// shared with the atomic commands: decryptCandidateCount is the pre-filter
// count, policy_disabled is recorded as a per-message failure, and zero
// candidates still publish zero-value counters.
func DecryptMessagesByPolicy(
	ctx context.Context,
	rt messagecrypto.Runtime,
	client *messagecrypto.Client,
	messages []map[string]any,
	opts DecryptOptions,
) map[string]any {
	if rt == nil || rt.DryRun() || client == nil || client.BackendReady == nil || !client.BackendReady() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	items := collectEncryptedMessageItems(messages)
	ledger := map[string]any{
		"decryptCandidateCount": len(items),
		"decryptAllowedCount":   0,
		"decryptedCount":        0,
		"decryptFailedCount":    0,
	}
	failures := make([]map[string]any, 0)
	filtered := make([]messagecrypto.BatchDecryptItem, 0, len(items))
	for _, item := range items {
		decision, err := client.PolicyDecision(ctx, rt, messagecrypto.Options{
			Identity:           "user",
			MsgType:            "text",
			OpenConversationID: item.ConversationID,
		})
		if err != nil {
			failures = append(failures, messageDecryptFailure(item.MessageID, item.ConversationID, err.Error()))
			continue
		}
		if !decision.Enabled {
			failures = append(failures, messageDecryptFailure(item.MessageID, item.ConversationID, "policy_disabled"))
			continue
		}
		filtered = append(filtered, item)
	}
	ledger["decryptAllowedCount"] = len(filtered)
	if len(filtered) > 0 {
		result, err := client.BatchDecryptInbound(ctx, rt, messagecrypto.Options{}, filtered)
		if err != nil {
			for _, item := range filtered {
				failures = append(failures, messageDecryptFailure(item.MessageID, item.ConversationID, err.Error()))
			}
		} else {
			index := indexMessageMapsByID(messages)
			decryptedCount := 0
			for _, item := range result.Items {
				if item.Status != "" && item.Status != "success" {
					failures = append(failures, messageDecryptFailure(item.MessageID, item.ConversationID, item.Reason))
					continue
				}
				if strings.TrimSpace(item.PlaintextContent) == "" {
					failures = append(failures, messageDecryptFailure(item.MessageID, item.ConversationID, "empty_plaintext"))
					continue
				}
				for _, message := range index[item.MessageID] {
					// Restore into whichever key held the ciphertext: top-level
					// messages use content, forwardMessages children use text.
					target := "content"
					for _, key := range []string{"content", "text"} {
						if IsEncrypted(firstMessageDecryptString(message, key)) {
							target = key
							break
						}
					}
					message[target] = item.PlaintextContent
					message["contentDecrypted"] = true
					message["cryptoLayer"] = "ding+safechat"
					if item.KeyVersion > 0 {
						message["dingKeyVersion"] = item.KeyVersion
					}
				}
				decryptedCount++
			}
			for _, item := range result.Failures {
				failures = append(failures, messageDecryptFailure(item.MessageID, item.ConversationID, item.Reason))
			}
			ledger["decryptedCount"] = decryptedCount
		}
	}
	if opts.MarkFailedOriginal {
		markMessageDecryptFailures(messages, failures)
	}
	if len(failures) > 0 {
		ledger["decryptFailedCount"] = len(failures)
		ledger["decryptFailures"] = failures
		ledger["partial"] = true
	}
	return ledger
}

// ApplyDecryptLedger merges the canonical decrypt ledger into a projected
// payload. A nil ledger (gate skipped) writes nothing; a non-nil ledger always
// carries the four counters and, with failures, decryptFailures and partial.
func ApplyDecryptLedger(payload map[string]any, ledger map[string]any) {
	if payload == nil || ledger == nil {
		return
	}
	for key, value := range ledger {
		payload[key] = value
	}
}

// collectEncryptedMessageItems walks messages (recursing into forwardMessages)
// and returns decrypt candidates carrying a stable message ID and ciphertext.
func collectEncryptedMessageItems(messages []map[string]any) []messagecrypto.BatchDecryptItem {
	items := make([]messagecrypto.BatchDecryptItem, 0)
	var walk func(map[string]any)
	walk = func(message map[string]any) {
		messageID := strings.TrimSpace(fmt.Sprint(MessageID(message)))
		conversationID := strings.TrimSpace(fmt.Sprint(ConversationID(message)))
		if conversationID == "<nil>" {
			conversationID = ""
		}
		content := firstMessageDecryptString(message, "content", "text")
		if messageID != "" && messageID != "<nil>" && IsEncrypted(content) {
			items = append(items, messagecrypto.BatchDecryptItem{
				MessageID:      messageID,
				ConversationID: conversationID,
				Ciphertext:     content,
			})
		}
		if forwarded, ok := message["forwardMessages"].([]any); ok {
			for _, item := range forwarded {
				if child, ok := item.(map[string]any); ok {
					walk(child)
				}
			}
		}
	}
	for _, message := range messages {
		walk(message)
	}
	return items
}

func indexMessageMapsByID(messages []map[string]any) map[string][]map[string]any {
	index := map[string][]map[string]any{}
	var walk func(map[string]any)
	walk = func(message map[string]any) {
		messageID := strings.TrimSpace(fmt.Sprint(MessageID(message)))
		if messageID != "" && messageID != "<nil>" {
			index[messageID] = append(index[messageID], message)
		}
		if forwarded, ok := message["forwardMessages"].([]any); ok {
			for _, item := range forwarded {
				if child, ok := item.(map[string]any); ok {
					walk(child)
				}
			}
		}
	}
	for _, message := range messages {
		walk(message)
	}
	return index
}

func markMessageDecryptFailures(messages []map[string]any, failures []map[string]any) {
	if len(failures) == 0 {
		return
	}
	index := indexMessageMapsByID(messages)
	for _, failure := range failures {
		messageID := strings.TrimSpace(fmt.Sprint(failure["messageId"]))
		if messageID == "" {
			continue
		}
		for _, message := range index[messageID] {
			content := firstMessageDecryptString(message, "content", "text")
			if IsEncrypted(content) {
				message[messageDecryptFailedOriginalContentKey] = content
			}
		}
	}
}

func messageDecryptFailure(messageID, conversationID, reason string) map[string]any {
	failure := map[string]any{
		"stage":     "message-decrypt",
		"messageId": strings.TrimSpace(messageID),
		"reason":    strings.TrimSpace(reason),
	}
	if failure["reason"] == "" {
		failure["reason"] = "decrypt_failed"
	}
	if trimmed := strings.TrimSpace(conversationID); trimmed != "" {
		failure["conversationId"] = trimmed
	}
	return failure
}

func firstMessageDecryptString(message map[string]any, keys ...string) string {
	value := firstMessageDecryptValue(message, keys...)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstMessageDecryptValue(message map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := message[key]; ok {
			return value
		}
	}
	return nil
}
