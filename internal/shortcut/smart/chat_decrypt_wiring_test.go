// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// The wiring tests drive the four smart read shortcuts end-to-end through
// declaration.Execute with a fake ToolCaller, proving the shared decrypt
// pipeline (chatmsg.DecryptChatMessageItems) is wired at the right point of
// each command: after the raw items land, before projection, after search-msg
// enrichment, and with the failure ledger merged without changing the exit
// code or the pre-decrypt byte shape.

const chatDecryptWirePolicy = `{"result":{"mode":"required","provider":"safechat","keyServer":"https://keys.example.test","allowedRedirectHost":"auth.example.test","staffIdTransform":"raw","ttlSeconds":60,"reason":"admin_enabled"}}`

const chatDecryptWireBatchOK = `{"result":{"items":[{"messageId":"m-enc","conversationId":"cid-wire","status":"success","plaintextContent":"秘密内容","keyVersion":3}]}}`

const chatDecryptWireMarker = "加密消息，无法解码"

type chatDecryptWireCaller struct {
	calls []string
	stubs map[string]string
	// listPages optionally feeds sequential pages of list_conversation_message_v2
	// for collect-all paths; once exhausted, listErr (if set) surfaces as a read
	// failure and the empty stub response otherwise.
	listPages []string
	listErr   error
	listCalls int
}

func (c *chatDecryptWireCaller) CallTool(_ context.Context, product, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, product+"/"+tool)
	if product == "chat" && tool == "list_conversation_message_v2" && (len(c.listPages) > 0 || c.listErr != nil) {
		index := c.listCalls
		c.listCalls++
		if index < len(c.listPages) {
			return chatDecryptGateResult(c.listPages[index]), nil
		}
		if c.listErr != nil {
			return nil, c.listErr
		}
	}
	payload, ok := c.stubs[tool]
	if !ok {
		return chatDecryptGateResult(`{}`), nil
	}
	return chatDecryptGateResult(payload), nil
}

func (c *chatDecryptWireCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, args)
}

func (*chatDecryptWireCaller) Format() string { return "json" }
func (*chatDecryptWireCaller) DryRun() bool   { return false }
func (*chatDecryptWireCaller) Fields() string { return "" }
func (*chatDecryptWireCaller) JQ() string     { return "" }

type chatDecryptWireRun struct {
	raw     string
	payload map[string]any
	err     error
	calls   []string
}

func runChatDecryptWireShortcut(t *testing.T, declaration shortcut.Shortcut, caller *chatDecryptWireCaller, stringFlags map[string]string, boolFlags ...string) chatDecryptWireRun {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	declaration.OutputRollout = output.RolloutLegacyOnly
	cmd := &cobra.Command{Use: declaration.Command}
	raw := &bytes.Buffer{}
	cmd.SetOut(raw)
	cmd.SetContext(context.Background())
	for name, value := range stringFlags {
		cmd.Flags().String(name, "", "")
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
		if err := cmd.Flags().Set(name, "true"); err != nil {
			t.Fatal(err)
		}
	}
	rt := shortcut.RuntimeContextForTest(cmd, declaration)
	err := declaration.Execute(rt)
	run := chatDecryptWireRun{raw: raw.String(), err: err, calls: append([]string(nil), caller.calls...)}
	if trimmed := strings.TrimSpace(run.raw); trimmed != "" {
		if unmarshalErr := json.Unmarshal([]byte(trimmed), &run.payload); unmarshalErr != nil {
			t.Fatalf("output is not JSON: %v; raw=%s", unmarshalErr, run.raw)
		}
	}
	return run
}

func chatDecryptWireCounter(t *testing.T, run chatDecryptWireRun, key string, want int) {
	t.Helper()
	value, ok := run.payload[key].(float64)
	if !ok || int(value) != want {
		t.Fatalf("%s = %#v, want %d; raw=%s", key, run.payload[key], want, run.raw)
	}
}

func chatDecryptWireFirstFailure(t *testing.T, run chatDecryptWireRun) map[string]any {
	t.Helper()
	failures, ok := run.payload["decryptFailures"].([]any)
	if !ok || len(failures) == 0 {
		t.Fatalf("decryptFailures missing; raw=%s", run.raw)
	}
	first, ok := failures[0].(map[string]any)
	if !ok {
		t.Fatalf("decryptFailures[0] = %#v", failures[0])
	}
	return first
}

func chatDecryptWireAssertDecrypted(t *testing.T, run chatDecryptWireRun) {
	t.Helper()
	if !strings.Contains(run.raw, "秘密内容") {
		t.Fatalf("plaintext missing from output: %s", run.raw)
	}
	if strings.Contains(run.raw, "QUJDREVGR0hJ") {
		t.Fatalf("ciphertext leaked into output: %s", run.raw)
	}
	if strings.Contains(run.raw, chatDecryptWireMarker) {
		t.Fatalf("decrypted message still rendered as undecodable: %s", run.raw)
	}
	if !strings.Contains(run.raw, `"contentDecrypted": true`) ||
		!strings.Contains(run.raw, `"cryptoLayer": "ding+safechat"`) ||
		!strings.Contains(run.raw, `"dingKeyVersion": 3`) {
		t.Fatalf("message-level decrypt markers missing: %s", run.raw)
	}
	chatDecryptWireCounter(t, run, "decryptCandidateCount", 1)
	chatDecryptWireCounter(t, run, "decryptAllowedCount", 1)
	chatDecryptWireCounter(t, run, "decryptedCount", 1)
	chatDecryptWireCounter(t, run, "decryptFailedCount", 0)
}

func chatDecryptWireAssertCallOrder(t *testing.T, run chatDecryptWireRun, want []string) {
	t.Helper()
	if strings.Join(run.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", run.calls, want)
	}
}

func TestCrossPlatformCoverageChatDecryptWiringChatMessages(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"list_conversation_message_v2": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"},
			{"openMessageId":"m-plain","openConversationId":"cid-wire","sender":"小红","createTime":1757900100000,"msgType":"text","content":"今天天气不错"}],
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, ChatMessages, caller,
		map[string]string{"conversation-id": "cid-wire"}, "no-reactions")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	if !strings.Contains(run.raw, "今天天气不错") {
		t.Fatalf("plaintext neighbour message lost: %s", run.raw)
	}
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/list_conversation_message_v2",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringAtMe(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"search_at_me_message": `{"result":{"conversationMessagesList":[{"title":"机密群","openConversationId":"cid-wire","messages":[
			{"openMessageId":"m-enc","sender":"小明","createTime":1757900000000,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"}]}]},
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, AtMe, caller, nil, "no-reactions")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	if !strings.Contains(run.raw, "机密群") {
		t.Fatalf("conversation title missing: %s", run.raw)
	}
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/search_at_me_message",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringSearchMsgAfterEnrichment(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		// The search hit and the enrichment re-fetch both carry ciphertext. If
		// decrypt ran before enrichment, the re-fetched ciphertext would win;
		// the plaintext output below proves decrypt runs after the mget merge.
		"search_messages": `{"list":[
			{"openMsgId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"list_messages_by_ids": `{"list":[
			{"openMsgId":"m-enc","content":"` + chatDecryptGateCiphertext + `","senderId":"u1"}]}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, SearchMsg, caller,
		map[string]string{"query": "机密"}, "no-reactions")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	batch, mget := -1, -1
	for index, call := range run.calls {
		switch call {
		case "im/batch_ding_decrypt_messages":
			batch = index
		case "im/list_messages_by_ids":
			mget = index
		}
	}
	if mget < 0 || batch < 0 || batch < mget {
		t.Fatalf("decrypt must run after enrichment: calls=%v", run.calls)
	}
}

func TestCrossPlatformCoverageChatDecryptWiringThreadReplies(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"list_topic_replies": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, ThreadReplies, caller,
		map[string]string{"group": "cid-wire", "thread-id": "th-wire"}, "no-reactions")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/list_topic_replies",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringDecryptFailureTolerated(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"list_topic_replies": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": `{"result":{"items":[{"messageId":"m-enc","conversationId":"cid-wire","status":"failed","reason":"bad_key"}]}}`,
	}}
	run := runChatDecryptWireShortcut(t, ThreadReplies, caller,
		map[string]string{"group": "cid-wire", "thread-id": "th-wire"}, "no-reactions")
	if run.err != nil {
		t.Fatalf("single decrypt failure must not break the command: %v; raw=%s", run.err, run.raw)
	}
	chatDecryptWireCounter(t, run, "decryptCandidateCount", 1)
	chatDecryptWireCounter(t, run, "decryptAllowedCount", 1)
	chatDecryptWireCounter(t, run, "decryptedCount", 0)
	chatDecryptWireCounter(t, run, "decryptFailedCount", 1)
	failure := chatDecryptWireFirstFailure(t, run)
	if failure["stage"] != "message-decrypt" || failure["messageId"] != "m-enc" ||
		failure["reason"] != "bad_key" || failure["conversationId"] != "cid-wire" {
		t.Fatalf("decrypt failure shape = %#v", failure)
	}
	if partial, _ := run.payload["partial"].(bool); !partial {
		t.Fatalf("partial must be true; raw=%s", run.raw)
	}
	if !strings.Contains(run.raw, chatDecryptWireMarker) {
		t.Fatalf("undecrypted message must render the encrypted marker: %s", run.raw)
	}
	if strings.Contains(run.raw, "秘密内容") {
		t.Fatalf("failed ciphertext must not be replaced: %s", run.raw)
	}
}

func TestCrossPlatformCoverageChatDecryptWiringPolicyDisabled(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"list_conversation_message_v2": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"get_message_crypto_policy":   `{"result":{"mode":"off"}}`,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, ChatMessages, caller,
		map[string]string{"conversation-id": "cid-wire"}, "no-reactions")
	if run.err != nil {
		t.Fatalf("policy-off must not break the command: %v; raw=%s", run.err, run.raw)
	}
	chatDecryptWireCounter(t, run, "decryptCandidateCount", 1)
	chatDecryptWireCounter(t, run, "decryptAllowedCount", 0)
	chatDecryptWireCounter(t, run, "decryptedCount", 0)
	chatDecryptWireCounter(t, run, "decryptFailedCount", 1)
	failure := chatDecryptWireFirstFailure(t, run)
	if failure["reason"] != "policy_disabled" || failure["messageId"] != "m-enc" {
		t.Fatalf("policy failure shape = %#v", failure)
	}
	for _, call := range run.calls {
		if call == "im/batch_ding_decrypt_messages" {
			t.Fatalf("policy-off must skip batch decrypt: %v", run.calls)
		}
	}
	if !strings.Contains(run.raw, chatDecryptWireMarker) {
		t.Fatalf("ciphertext must render the encrypted marker: %s", run.raw)
	}
}

func TestCrossPlatformCoverageChatDecryptWiringStubAndDryRunByteIdentical(t *testing.T) {
	stubs := func(caller *chatDecryptWireCaller) {
		caller.stubs = map[string]string{
			"list_conversation_message_v2": `{"list":[
				{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"content":"` + chatDecryptGateCiphertext + `"}],
				"hasMore":false}`,
		}
	}

	stubClient := chatDecryptGateClient()
	stubClient.BackendReady = func() bool { return false }
	chatmsg.SwapMessageDecryptClientForTest(t, stubClient)
	stubCaller := &chatDecryptWireCaller{}
	stubs(stubCaller)
	stubRun := runChatDecryptWireShortcut(t, ChatMessages, stubCaller,
		map[string]string{"conversation-id": "cid-wire"}, "no-reactions")
	if stubRun.err != nil {
		t.Fatalf("stub run: %v; raw=%s", stubRun.err, stubRun.raw)
	}

	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	dryRunCaller := &chatDecryptWireCaller{}
	stubs(dryRunCaller)
	dryRun := runChatDecryptWireShortcut(t, ChatMessages, dryRunCaller,
		map[string]string{"conversation-id": "cid-wire"}, "no-reactions", "dry-run")
	if dryRun.err != nil {
		t.Fatalf("dry-run: %v; raw=%s", dryRun.err, dryRun.raw)
	}

	if stubRun.raw != dryRun.raw {
		t.Fatalf("stub and dry-run outputs diverge:\nstub   = %s\ndryrun = %s", stubRun.raw, dryRun.raw)
	}
	if strings.Contains(stubRun.raw, "decryptCandidateCount") {
		t.Fatalf("decrypt short-circuit must stay byte-identical to the pre-decrypt payload: %s", stubRun.raw)
	}
	if strings.Contains(stubRun.raw, "秘密内容") || !strings.Contains(stubRun.raw, chatDecryptWireMarker) {
		t.Fatalf("ciphertext must be untouched and render the marker: %s", stubRun.raw)
	}
	chatDecryptWireAssertCallOrder(t, stubRun, []string{"chat/list_conversation_message_v2"})
	chatDecryptWireAssertCallOrder(t, dryRun, []string{"chat/list_conversation_message_v2"})
}

func TestCrossPlatformCoverageChatDecryptWiringChatMessagesPageAll(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"list_conversation_message_v2": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, ChatMessages, caller,
		map[string]string{"conversation-id": "cid-wire"}, "no-reactions", "page-all")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/list_conversation_message_v2",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringAtMePageAll(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"search_at_me_message": `{"result":{"conversationMessagesList":[{"title":"机密群","openConversationId":"cid-wire","messages":[
			{"openMessageId":"m-enc","sender":"小明","createTime":1757900000000,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"}]}]},
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, AtMe, caller, nil, "no-reactions", "page-all")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/search_at_me_message",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringThreadRepliesPageAll(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"list_topic_replies": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, ThreadReplies, caller,
		map[string]string{"group": "cid-wire", "thread-id": "th-wire"}, "no-reactions", "page-all")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/list_topic_replies",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringScopedReactionSingleDecrypt(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	// The scoped stream applies its own explicit time window, so the stub hit
	// must carry a fresh createTime to survive the in-collector range filter.
	freshCreateTime := time.Now().Add(-time.Minute).UnixMilli()
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		// The conversation stream and the mget enrichment both carry
		// ciphertext. The scoped stream must defer decryption past its own
		// enrichment and run the pipeline exactly once per Execute — a second
		// batch call here would mean the in-collector decrypt was not deferred.
		"list_conversation_message_v2": `{"list":[
			{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":` + strconv.FormatInt(freshCreateTime, 10) + `,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"}],
			"hasMore":false}`,
		"list_messages_by_ids": `{"list":[
			{"openMsgId":"m-enc","openConversationId":"cid-wire","content":"` + chatDecryptGateCiphertext + `","emotionReplyList":[{"emoji":"👍","user":"u1"}]}]}`,
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	run := runChatDecryptWireShortcut(t, SearchMsg, caller,
		map[string]string{
			"conversation-id": "cid-wire",
			// --start/--end keep the scoped stream eligible (startSet==endSet)
			// and give the in-collector time filter a real window; the helper
			// registers string flags, so --days (an int flag) would read as 0.
			"start": time.Now().Add(-time.Hour).Format(time.RFC3339),
			"end":   time.Now().Format(time.RFC3339),
		}, "has-reactions", "page-all")
	if run.err != nil {
		t.Fatalf("execute: %v; calls=%v raw=%s", run.err, run.calls, run.raw)
	}
	chatDecryptWireAssertDecrypted(t, run)
	batchCount, batchAt, mgetAt := 0, -1, -1
	for index, call := range run.calls {
		switch call {
		case "im/batch_ding_decrypt_messages":
			batchCount++
			batchAt = index
		case "im/list_messages_by_ids":
			mgetAt = index
		}
	}
	if batchCount != 1 {
		t.Fatalf("scoped stream must decrypt exactly once per Execute: calls=%v", run.calls)
	}
	if mgetAt < 0 || batchAt < mgetAt {
		t.Fatalf("single decrypt must run after the mget enrichment: calls=%v", run.calls)
	}
	chatDecryptWireAssertCallOrder(t, run, []string{
		"chat/get_conversation_info",
		"chat/list_conversation_message_v2",
		"im/list_messages_by_ids",
		"im/get_message_crypto_policy",
		"im/batch_ding_decrypt_messages",
	})
}

func TestCrossPlatformCoverageChatDecryptWiringAggregationPartialPreserved(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptWireCaller{stubs: map[string]string{
		"get_message_crypto_policy":   chatDecryptWirePolicy,
		"batch_ding_decrypt_messages": chatDecryptWireBatchOK,
	}}
	caller.listPages = []string{`{"list":[
		{"openMessageId":"m-enc","openConversationId":"cid-wire","sender":"小明","createTime":1757900000000,"msgType":"text","content":"` + chatDecryptGateCiphertext + `"}],
		"hasMore":true,"nextCursor":"1757900100000"}`}
	caller.listErr = errors.New("page 2 read failed")
	run := runChatDecryptWireShortcut(t, ChatMessages, caller,
		map[string]string{"conversation-id": "cid-wire", "page-limit": "5"}, "no-reactions", "page-all")
	if run.err == nil || !strings.Contains(run.err.Error(), "全量消息读取未完成") {
		t.Fatalf("expected the aggregation incomplete error, got %v; raw=%s", run.err, run.raw)
	}
	// #1297 semantics: the aggregation failure decides partial=true before the
	// decrypt ledger merges; the merge must keep it true (or-only), never reset.
	chatDecryptWireCounter(t, run, "decryptCandidateCount", 1)
	chatDecryptWireCounter(t, run, "decryptAllowedCount", 1)
	chatDecryptWireCounter(t, run, "decryptedCount", 1)
	chatDecryptWireCounter(t, run, "decryptFailedCount", 0)
	if partial, _ := run.payload["partial"].(bool); !partial {
		t.Fatalf("aggregation partial must survive the ledger merge: raw=%s", run.raw)
	}
	if !strings.Contains(run.raw, "秘密内容") {
		t.Fatalf("page-1 plaintext must ride along in the partial payload: %s", run.raw)
	}
}
