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

package smart

import (
	"context"
	"strings"
	"testing"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
)

const testCipher = "SwzNkAraDE6lUHUNlVT3mjFdbxL6dWvmt77XtjACdpJx9VFibzTbW9KtDbkzGOYP||2||1||1"

func TestCrossPlatformCoverageAtMeProject(t *testing.T) {
	// nested sender object + plain text
	row := atMeProject(map[string]any{
		"sender":             map[string]any{"name": "念晨"},
		"createTime":         "2026-07-19 13:37:03",
		"content":            "普通消息",
		"openMessageId":      "msg-1",
		"msgType":            "text",
		"messageAiSendFlag":  "DWS",
		"conversationTitle":  "群A",
		"openConversationId": "cid1",
		"emotionReplyList": []any{
			map[string]any{"emoji": "赞", "replyUsers": []any{"D1"}},
		},
	})
	if row["sender"] != "念晨" || row["text"] != "普通消息" || row["conversation"] != "群A" {
		t.Fatalf("atMeProject nested = %#v", row)
	}
	if row["messageId"] != "msg-1" || row["conversationId"] != "cid1" || row["messageType"] != "text" {
		t.Errorf("atMeProject stable identity = %#v", row)
	}
	if row["messageAiSendFlag"] != "DWS" {
		t.Errorf("atMeProject AI send flag = %#v", row)
	}
	if reactions, ok := row["reactions"].(map[string]any); !ok || len(reactions) == 0 {
		t.Errorf("atMeProject reactions = %#v", row["reactions"])
	}

	// encrypted content → marked (never leaked); id-only sender fallback; a
	// forwarded sub-message whose sender is the literal "null" must be nulled.
	row = atMeProject(map[string]any{
		"senderId":      "DXYZ",
		"openMessageId": "m1",
		"content":       testCipher,
		"forwardMessages": []any{
			map[string]any{"sender": "null", "content": "子消息", "createTime": "t"},
		},
	})
	if row["sender"] != "DXYZ" {
		t.Errorf("atMeProject id-fallback sender = %v", row["sender"])
	}
	if s, _ := row["text"].(string); !strings.Contains(s, "加密消息") {
		t.Errorf("atMeProject encrypted text = %v, want marker", row["text"])
	}
	fwd, ok := row["forwarded"].([]map[string]any)
	if !ok || len(fwd) != 1 {
		t.Fatalf("atMeProject forwarded = %#v", row["forwarded"])
	}
	if fwd[0]["sender"] != nil {
		t.Errorf("forwarded sub sender = %v, want nil (literal \"null\")", fwd[0]["sender"])
	}

	// no sender / no text at all → nils, no forwarded key
	row = atMeProject(map[string]any{"createTime": "t"})
	if row["sender"] != nil || row["text"] != nil {
		t.Errorf("atMeProject empty = %#v", row)
	}
	if _, has := row["forwarded"]; has {
		t.Errorf("atMeProject plain unexpectedly has forwarded")
	}

	row = atMeProjectWithReactions(map[string]any{
		"emotionReplyList": []any{
			map[string]any{"emoji": "赞", "replyUsers": []any{"D1"}},
		},
	}, false)
	if _, has := row["reactions"]; has {
		t.Errorf("atMeProject no-reactions leaked reactions: %#v", row)
	}
}

func TestCrossPlatformCoverageSearchMsgProject(t *testing.T) {
	// nested sender + plain text + messageId
	row := searchMsgProject(map[string]any{
		"sender":            map[string]any{"nick": "千启"},
		"createTime":        "2026-07-19 13:37:03",
		"content":           "命中关键词的消息",
		"msgId":             "mid1",
		"messageAiSendFlag": "DWS",
	})
	if row["sender"] != "千启" || row["text"] != "命中关键词的消息" {
		t.Fatalf("searchMsgProject = %#v", row)
	}
	if row["messageAiSendFlag"] != "DWS" {
		t.Fatalf("searchMsgProject AI send flag = %#v", row)
	}

	// Canonical ID precedence and rich-text extraction must match typed search.
	row = searchMsgProject(map[string]any{
		"openMessageId": "open-id",
		"messageId":     "legacy-conflict",
		"content":       map[string]any{"richText": "富文本消息"},
	})
	if row["messageId"] != "open-id" || row["text"] != "富文本消息" {
		t.Fatalf("searchMsgProject canonical fields = %#v", row)
	}

	// encrypted → marker; id-only sender; forwarded "null" sender nulled.
	row = searchMsgProject(map[string]any{
		"senderId":      "DAAA",
		"openMessageId": "m2",
		"content":       testCipher,
		"forwardMessages": []any{
			map[string]any{"sender": "null", "content": "转发子消息", "createTime": "t"},
		},
	})
	if row["sender"] != "DAAA" {
		t.Errorf("searchMsgProject sender = %v", row["sender"])
	}
	if s, _ := row["text"].(string); !strings.Contains(s, "加密消息") {
		t.Errorf("searchMsgProject encrypted text = %v, want marker", row["text"])
	}
	fwd, ok := row["forwarded"].([]map[string]any)
	if !ok || len(fwd) != 1 || fwd[0]["sender"] != nil || fwd[0]["time"] != "t" {
		t.Errorf("searchMsgProject forwarded = %#v", row["forwarded"])
	}

	// no sender / no text
	row = searchMsgProject(map[string]any{"createTime": "t"})
	if row["sender"] != nil || row["text"] != nil {
		t.Errorf("searchMsgProject empty = %#v", row)
	}
}

// TestSenderHelpers exercises the atMe/searchMsg sender key families directly:
// a senderName-family key (first probe loop), a flat string under "sender"
// (second loop), and the "null" sentinel normalisation.
func TestCrossPlatformCoverageSenderHelpers(t *testing.T) {
	cases := []struct {
		fn   func(map[string]any) any
		name string
	}{
		{atMeSender, "atMeSender"},
		{searchMsgSender, "searchMsgSender"},
	}
	for _, c := range cases {
		if got := c.fn(map[string]any{"senderName": "张三"}); got != "张三" {
			t.Errorf("%s senderName = %v, want 张三", c.name, got)
		}
		if got := c.fn(map[string]any{"sender": "李四"}); got != "李四" {
			t.Errorf("%s flat sender = %v, want 李四", c.name, got)
		}
		if got := c.fn(map[string]any{"senderName": "null"}); got != nil {
			t.Errorf("%s \"null\" = %v, want nil", c.name, got)
		}
	}
}

func atMeDecryptResponse(content string) string {
	return `{"result":{"hasMore":false,"conversationMessagesList":[{"openConversationId":"cid","title":"群A","messages":[{"openMessageId":"m1","openConversationId":"cid","sender":"张三","content":"` + content + `","createTime":"2026-01-02 00:00:00","msgType":"text"}]}]}}`
}

func TestCrossPlatformCoverageAtMeDecryptSinglePage(t *testing.T) {
	swapChatMessagesDecryptClient(t, chatMessagesDecryptReadyClient())
	caller := &chatMessagesDecryptCaller{responses: map[string]string{
		"chat/search_at_me_message":      atMeDecryptResponse(chatMessagesDecryptCipherText),
		"im/get_message_crypto_policy":   `{"result":{"mode":"required"}}`,
		"im/batch_ding_decrypt_messages": `{"result":{"items":[{"messageId":"m1","status":"success","plaintextContent":"hello decrypted","keyVersion":8}]}}`,
	}}
	payload := runChatMessagesDecrypt(t, caller, "chat", "+at-me")
	if len(caller.calls) != 3 ||
		caller.calls[0].tool != "search_at_me_message" ||
		caller.calls[1].tool != "get_message_crypto_policy" ||
		caller.calls[2].tool != "batch_ding_decrypt_messages" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if payload["decryptCandidateCount"] != float64(1) ||
		payload["decryptAllowedCount"] != float64(1) ||
		payload["decryptedCount"] != float64(1) ||
		payload["decryptFailedCount"] != float64(0) {
		t.Fatalf("decrypt ledger = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != "hello decrypted" || first["contentDecrypted"] != true ||
		first["cryptoLayer"] != "ding+safechat" || first["dingKeyVersion"] != float64(8) {
		t.Fatalf("decrypted message = %#v", first)
	}
}

func TestCrossPlatformCoverageAtMeDecryptPolicyOffRestoresOriginalText(t *testing.T) {
	swapChatMessagesDecryptClient(t, chatMessagesDecryptReadyClient())
	caller := &chatMessagesDecryptCaller{responses: map[string]string{
		"chat/search_at_me_message":    atMeDecryptResponse(chatMessagesDecryptCipherText),
		"im/get_message_crypto_policy": `{"result":{"mode":"off"}}`,
	}}
	payload := runChatMessagesDecrypt(t, caller, "chat", "+at-me")
	if payload["decryptCandidateCount"] != float64(1) ||
		payload["decryptAllowedCount"] != float64(0) ||
		payload["decryptFailedCount"] != float64(1) ||
		payload["partial"] != true {
		t.Fatalf("policy-off ledger = %#v", payload)
	}
	failures, _ := payload["decryptFailures"].([]any)
	failure, _ := failures[0].(map[string]any)
	if failure["reason"] != "policy_disabled" || failure["stage"] != "message-decrypt" || failure["messageId"] != "m1" {
		t.Fatalf("policy-off failure = %#v", failure)
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != chatMessagesDecryptCipherText || first["contentDecrypted"] == true {
		t.Fatalf("policy-off message = %#v", first)
	}
}

func TestCrossPlatformCoverageAtMeDecryptBatchFailureFallsBackToOriginal(t *testing.T) {
	swapChatMessagesDecryptClient(t, chatMessagesDecryptReadyClient())
	caller := &chatMessagesDecryptCaller{
		responses: map[string]string{
			"chat/search_at_me_message":    atMeDecryptResponse(chatMessagesDecryptCipherText),
			"im/get_message_crypto_policy": `{"result":{"mode":"required"}}`,
		},
		failTool: "im/batch_ding_decrypt_messages",
	}
	payload := runChatMessagesDecrypt(t, caller, "chat", "+at-me")
	if payload["decryptFailedCount"] != float64(1) || payload["partial"] != true {
		t.Fatalf("batch-failure ledger = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != chatMessagesDecryptCipherText || first["contentDecrypted"] == true {
		t.Fatalf("batch-failure message = %#v", first)
	}
}

func TestCrossPlatformCoverageAtMeDecryptSkipsWhenBackendUnavailable(t *testing.T) {
	swapChatMessagesDecryptClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			t.Fatal("identity lookup must not run without the SafeChat backend")
			return messagecrypto.Identity{}, nil
		},
		BackendReady: func() bool { return false },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	caller := &chatMessagesDecryptCaller{responses: map[string]string{
		"chat/search_at_me_message": atMeDecryptResponse(chatMessagesDecryptCipherText),
	}}
	payload := runChatMessagesDecrypt(t, caller, "chat", "+at-me")
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want search only", caller.calls)
	}
	for _, key := range []string{"decryptCandidateCount", "decryptAllowedCount", "decryptedCount", "decryptFailedCount", "decryptFailures"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("unavailable backend emitted %q: %#v", key, payload)
		}
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if text, _ := first["text"].(string); !strings.Contains(text, "加密消息") {
		t.Fatalf("message changed without backend: %#v", first)
	}
}

func TestCrossPlatformCoverageAtMeDecryptPageAllSinglePolicyAndBatch(t *testing.T) {
	swapChatMessagesDecryptClient(t, chatMessagesDecryptReadyClient())
	caller := &chatMessagesDecryptCaller{
		pageTool: "chat/search_at_me_message",
		listPages: []string{
			`{"result":{"hasMore":true,"nextCursor":"cursor-2","conversationMessagesList":[{"openConversationId":"cid","title":"群A","messages":[{"openMessageId":"m1","openConversationId":"cid","content":"` + chatMessagesDecryptCipherText + `","createTime":"2026-01-02 00:00:00"}]}]}}`,
			`{"result":{"hasMore":false,"conversationMessagesList":[{"openConversationId":"cid","title":"群A","messages":[{"openMessageId":"m2","openConversationId":"cid","content":"` + chatMessagesDecryptCipherText + `","createTime":"2026-01-01 00:00:00"}]}]}}`,
		},
		responses: map[string]string{
			"im/get_message_crypto_policy":   `{"result":{"mode":"required","ttlSeconds":60}}`,
			"im/batch_ding_decrypt_messages": `{"result":{"items":[{"messageId":"m1","status":"success","plaintextContent":"明文一"},{"messageId":"m2","status":"success","plaintextContent":"明文二"}]}}`,
		},
	}
	payload := runChatMessagesDecrypt(t, caller, "chat", "+at-me", "--page-all", "--page-limit", "5")
	listCalls, policyCalls, batchCalls := 0, 0, 0
	for _, call := range caller.calls {
		switch call.tool {
		case "search_at_me_message":
			listCalls++
		case "get_message_crypto_policy":
			policyCalls++
		case "batch_ding_decrypt_messages":
			batchCalls++
		}
	}
	if listCalls != 2 || policyCalls != 1 || batchCalls != 1 {
		t.Fatalf("calls = %d list / %d policy / %d batch, want 2/1/1", listCalls, policyCalls, batchCalls)
	}
	if payload["decryptCandidateCount"] != float64(2) ||
		payload["decryptAllowedCount"] != float64(2) ||
		payload["decryptedCount"] != float64(2) {
		t.Fatalf("page-all decrypt ledger = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	first, _ := messages[0].(map[string]any)
	second, _ := messages[1].(map[string]any)
	if first["text"] != "明文一" || second["text"] != "明文二" {
		t.Fatalf("page-all decrypted messages = %#v/%#v", first, second)
	}
}

func TestCrossPlatformCoverageAtMeDecryptDryRunSkipsDecrypt(t *testing.T) {
	swapChatMessagesDecryptClient(t, chatMessagesDecryptReadyClient())
	caller := &chatMessagesDecryptCaller{responses: map[string]string{
		"chat/search_at_me_message": atMeDecryptResponse(chatMessagesDecryptCipherText),
	}}
	payload := runChatMessagesDecrypt(t, caller, "chat", "+at-me", "--dry-run")
	for _, call := range caller.calls {
		if call.tool == "get_message_crypto_policy" || call.tool == "batch_ding_decrypt_messages" {
			t.Fatalf("dry-run must not call decrypt tools: %#v", caller.calls)
		}
	}
	for _, key := range []string{"decryptCandidateCount", "decryptAllowedCount", "decryptedCount", "decryptFailedCount"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("dry-run emitted %q: %#v", key, payload)
		}
	}
}
