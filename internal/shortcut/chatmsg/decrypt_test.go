// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import (
	"context"
	"testing"
	"time"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

const decryptTestCiphertext = "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcQ==||2||1||196"

type decryptTestCipher struct {
	decryptByText map[string]string
	plain         string
}

func (c *decryptTestCipher) EncryptMessage(_ context.Context, _, _ string, plaintext []byte) ([]byte, error) {
	return plaintext, nil
}

func (c *decryptTestCipher) DecryptMessage(_ context.Context, _, _ string, ciphertext []byte) ([]byte, error) {
	if c.decryptByText != nil {
		if plain, ok := c.decryptByText[string(ciphertext)]; ok {
			return []byte(plain), nil
		}
	}
	return []byte(c.plain), nil
}

type decryptTestCall struct {
	product string
	tool    string
	params  map[string]any
}

type decryptTestRuntime struct {
	dryRun   bool
	reads    []decryptTestCall
	writes   []decryptTestCall
	read     map[string]any
	write    map[string]any
	readErr  error
	writeErr error
}

func (r *decryptTestRuntime) CallMCPReadData(product, tool string, params map[string]any) (map[string]any, error) {
	r.reads = append(r.reads, decryptTestCall{product: product, tool: tool, params: params})
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.read, nil
}

func (r *decryptTestRuntime) CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error) {
	r.writes = append(r.writes, decryptTestCall{product: product, tool: tool, params: params})
	if r.writeErr != nil {
		return nil, r.writeErr
	}
	return r.write, nil
}

func (r *decryptTestRuntime) DryRun() bool { return r.dryRun }

func newDecryptTestClient(cipher *decryptTestCipher, backendReady bool) *messagecrypto.Client {
	return &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: cipher, CorpID: "corp-1", StaffID: "staff-1", Close: func() error { return nil }}, nil
		},
		BackendReady: func() bool { return backendReady },
		PolicyCache:  messagecrypto.NewPolicyCache(time.Now),
	}
}

var decryptTestPolicyRead = map[string]any{"result": map[string]any{
	"mode":                messagecrypto.ModeRequired,
	"provider":            "safechat",
	"keyServer":           "https://keys.example.test",
	"allowedRedirectHost": "auth.example.test",
	"staffIdTransform":    "raw",
	"ttlSeconds":          int64(60),
	"reason":              "admin_enabled",
}}

func swapDecryptTestClient(t *testing.T, client *messagecrypto.Client) {
	t.Helper()
	testseam.Swap(t, &messageDecryptClient, client)
}

func TestDecryptChatMessageItemsShouldReturnNilWhenDecryptSkipped(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "plain"}, true))
	items := []map[string]any{{"openMessageId": "m1", "content": decryptTestCiphertext}}

	if got := DecryptChatMessageItems(context.Background(), &decryptTestRuntime{dryRun: true}, items); got != nil {
		t.Fatalf("dry-run ledger = %#v, want nil", got)
	}
	var nilRuntime messagecrypto.Runtime
	if got := DecryptChatMessageItems(context.Background(), nilRuntime, items); got != nil {
		t.Fatalf("nil runtime ledger = %#v, want nil", got)
	}
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "plain"}, false))
	if got := DecryptChatMessageItems(context.Background(), &decryptTestRuntime{}, items); got != nil {
		t.Fatalf("backend-not-ready ledger = %#v, want nil", got)
	}
}

func TestDecryptChatMessageItemsShouldReturnZeroLedgerWithoutCandidates(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "plain"}, true))
	rt := &decryptTestRuntime{}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{
		{"openMessageId": "m1", "openConversationId": "cid-1", "content": "plain hello"},
	})
	if got == nil {
		t.Fatal("ledger = nil, want zero-value ledger")
	}
	for key, want := range map[string]int{
		"decryptCandidateCount": 0,
		"decryptAllowedCount":   0,
		"decryptedCount":        0,
		"decryptFailedCount":    0,
	} {
		if got[key] != want {
			t.Fatalf("ledger[%s] = %v, want %d", key, got[key], want)
		}
	}
	if _, ok := got["decryptFailures"]; ok {
		t.Fatalf("ledger has decryptFailures: %#v", got)
	}
	if _, ok := got["partial"]; ok {
		t.Fatalf("ledger has partial: %#v", got)
	}
	if len(rt.reads) != 0 || len(rt.writes) != 0 {
		t.Fatalf("MCP calls = %d reads/%d writes, want none", len(rt.reads), len(rt.writes))
	}
}

func TestDecryptChatMessageItemsShouldRewriteDecryptedItems(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{read: decryptTestPolicyRead, write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "m1", "status": "success", "plaintextContent": "秘密内容", "keyVersion": 3},
	}}}}
	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": decryptTestCiphertext}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{item})
	if got == nil {
		t.Fatal("ledger = nil")
	}
	if item["content"] != "秘密内容" || item["contentDecrypted"] != true || item["cryptoLayer"] != "ding+safechat" {
		t.Fatalf("item = %#v", item)
	}
	if item["dingKeyVersion"] != 3 {
		t.Fatalf("dingKeyVersion = %v, want 3", item["dingKeyVersion"])
	}
	for key, want := range map[string]int{
		"decryptCandidateCount": 1,
		"decryptAllowedCount":   1,
		"decryptedCount":        1,
		"decryptFailedCount":    0,
	} {
		if got[key] != want {
			t.Fatalf("ledger[%s] = %v, want %d", key, got[key], want)
		}
	}
	if _, ok := got["partial"]; ok {
		t.Fatalf("ledger has partial: %#v", got)
	}
	if len(rt.writes) != 1 || rt.writes[0].product != "im" || rt.writes[0].tool != "batch_ding_decrypt_messages" {
		t.Fatalf("writes = %#v", rt.writes)
	}
}

func TestDecryptChatMessageItemsShouldRecordFailedItemsWithoutBreakingCommand(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{read: decryptTestPolicyRead, write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "m1", "conversationId": "cid-1", "status": "failed", "reason": "bad_key"},
		map[string]any{"messageId": "m2", "conversationId": "cid-1", "status": "success", "plaintextContent": " "},
	}}}}
	item1 := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": decryptTestCiphertext}
	item2 := map[string]any{"openMessageId": "m2", "openConversationId": "cid-1", "content": decryptTestCiphertext}
	item3 := map[string]any{"openMessageId": "m3", "openConversationId": "cid-1", "content": "明文"}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{item1, item2, item3})
	if got == nil {
		t.Fatal("ledger = nil")
	}
	if got["decryptedCount"] != 0 || got["decryptFailedCount"] != 2 || got["partial"] != true {
		t.Fatalf("ledger = %#v", got)
	}
	failures, ok := got["decryptFailures"].([]map[string]any)
	if !ok || len(failures) != 2 {
		t.Fatalf("decryptFailures = %#v", got["decryptFailures"])
	}
	if failures[0]["messageId"] != "m1" || failures[0]["reason"] != "bad_key" || failures[0]["stage"] != "message-decrypt" || failures[0]["conversationId"] != "cid-1" {
		t.Fatalf("failure[0] = %#v", failures[0])
	}
	if failures[1]["messageId"] != "m2" || failures[1]["reason"] != "empty_plaintext" {
		t.Fatalf("failure[1] = %#v", failures[1])
	}
	if item3["content"] != "明文" {
		t.Fatalf("plaintext item rewritten: %#v", item3)
	}
	if item2["contentDecrypted"] == true {
		t.Fatalf("failed item marked decrypted: %#v", item2)
	}
}

func TestDecryptChatMessageItemsShouldRecordPolicyDisabled(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{read: map[string]any{"result": map[string]any{
		"mode":       messagecrypto.ModeOff,
		"ttlSeconds": int64(60),
	}}}
	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": decryptTestCiphertext}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{item})
	if got == nil {
		t.Fatal("ledger = nil")
	}
	if got["decryptCandidateCount"] != 1 || got["decryptAllowedCount"] != 0 || got["decryptFailedCount"] != 1 || got["partial"] != true {
		t.Fatalf("ledger = %#v", got)
	}
	failures, ok := got["decryptFailures"].([]map[string]any)
	if !ok || len(failures) != 1 || failures[0]["reason"] != "policy_disabled" || failures[0]["messageId"] != "m1" {
		t.Fatalf("decryptFailures = %#v", got["decryptFailures"])
	}
	if item["contentDecrypted"] == true {
		t.Fatalf("ciphertext must be kept when policy disabled: %#v", item)
	}
	if len(rt.writes) != 0 {
		t.Fatalf("batch decrypt must not run when policy disabled: %#v", rt.writes)
	}
}

func TestDecryptChatMessageItemsShouldRecordPolicyError(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{readErr: context.DeadlineExceeded}
	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": decryptTestCiphertext}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{item})
	if got == nil {
		t.Fatal("ledger = nil")
	}
	if got["decryptAllowedCount"] != 0 || got["decryptFailedCount"] != 1 || got["partial"] != true {
		t.Fatalf("ledger = %#v", got)
	}
	failures, ok := got["decryptFailures"].([]map[string]any)
	if !ok || len(failures) != 1 || failures[0]["reason"] == "" || failures[0]["reason"] == "policy_disabled" {
		t.Fatalf("decryptFailures = %#v", got["decryptFailures"])
	}
}

func TestDecryptChatMessageItemsShouldRecordOverallBatchError(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{read: decryptTestPolicyRead, writeErr: context.Canceled}
	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": decryptTestCiphertext}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{item})
	if got == nil {
		t.Fatal("ledger = nil")
	}
	if got["decryptCandidateCount"] != 1 || got["decryptAllowedCount"] != 1 || got["decryptedCount"] != 0 || got["decryptFailedCount"] != 1 || got["partial"] != true {
		t.Fatalf("ledger = %#v", got)
	}
	failures, ok := got["decryptFailures"].([]map[string]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("decryptFailures = %#v", got["decryptFailures"])
	}
	if failures[0]["stage"] != "message-decrypt" || failures[0]["reason"] == "" {
		t.Fatalf("failure = %#v", failures[0])
	}
	if _, ok := failures[0]["messageId"]; ok {
		t.Fatalf("overall failure must not carry messageId: %#v", failures[0])
	}
	if item["contentDecrypted"] == true {
		t.Fatalf("ciphertext must be kept on batch error: %#v", item)
	}
}

func TestDecryptChatMessageItemsShouldReusePolicyCacheAcrossItems(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{read: decryptTestPolicyRead, write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "m1", "status": "success", "plaintextContent": "一"},
		map[string]any{"messageId": "m2", "status": "success", "plaintextContent": "二"},
	}}}}
	items := []map[string]any{
		{"openMessageId": "m1", "openConversationId": "cid-1", "content": decryptTestCiphertext},
		{"openMessageId": "m2", "openConversationId": "cid-1", "content": decryptTestCiphertext},
	}

	got := DecryptChatMessageItems(context.Background(), rt, items)
	if got == nil || got["decryptedCount"] != 2 {
		t.Fatalf("ledger = %#v", got)
	}
	if len(rt.reads) != 1 {
		t.Fatalf("policy reads = %d, want 1 (PolicyCache reuse)", len(rt.reads))
	}
	if len(rt.writes) != 1 {
		t.Fatalf("batch decrypt calls = %d, want 1", len(rt.writes))
	}
	if wrote, ok := rt.writes[0].params["items"].([]map[string]any); !ok || len(wrote) != 2 {
		t.Fatalf("batch items = %#v", rt.writes[0].params["items"])
	}
}

func TestDecryptChatMessageItemsShouldRewriteTextKeyWhenContentAbsent(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "ding-cipher"}, true))
	rt := &decryptTestRuntime{read: decryptTestPolicyRead, write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "m1", "status": "success", "plaintextContent": "明文内容", "keyVersion": 1},
	}}}}
	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "text": decryptTestCiphertext}

	got := DecryptChatMessageItems(context.Background(), rt, []map[string]any{item})
	if got == nil || got["decryptedCount"] != 1 {
		t.Fatalf("ledger = %#v", got)
	}
	if item["text"] != "明文内容" || item["contentDecrypted"] != true {
		t.Fatalf("item = %#v", item)
	}
	if _, ok := item["content"]; ok {
		t.Fatalf("rewrite must land on the ciphertext's own key: %#v", item)
	}
}

func TestSetMessageDecryptClientShouldFallbackToDefaultOnNil(t *testing.T) {
	swapDecryptTestClient(t, newDecryptTestClient(&decryptTestCipher{plain: "plain"}, true))
	SetMessageDecryptClient(nil)
	if messageDecryptClient.BackendReady() {
		t.Fatal("nil injection should fall back to default backend-less client")
	}
	if got := DecryptChatMessageItems(context.Background(), &decryptTestRuntime{}, []map[string]any{
		{"openMessageId": "m1", "content": decryptTestCiphertext},
	}); got != nil {
		t.Fatalf("ledger = %#v, want nil", got)
	}
}

func TestMergeDecryptLedgerShouldAccumulateCountersAndKeepPartial(t *testing.T) {
	payload := map[string]any{
		"decryptCandidateCount": 1,
		"decryptAllowedCount":   1,
		"decryptedCount":        0,
		"decryptFailedCount":    1,
		"decryptFailures":       []map[string]any{{"messageId": "f0"}},
		"partial":               true,
	}
	MergeDecryptLedger(payload, map[string]any{
		"decryptCandidateCount": 2,
		"decryptAllowedCount":   1,
		"decryptedCount":        2,
		"decryptFailedCount":    1,
		"decryptFailures":       []map[string]any{{"messageId": "f1"}},
		"partial":               false,
	})
	for key, want := range map[string]int{
		"decryptCandidateCount": 3,
		"decryptAllowedCount":   2,
		"decryptedCount":        2,
		"decryptFailedCount":    2,
	} {
		if payload[key] != want {
			t.Fatalf("payload[%s] = %v, want %d", key, payload[key], want)
		}
	}
	failures, ok := payload["decryptFailures"].([]map[string]any)
	if !ok || len(failures) != 2 || failures[0]["messageId"] != "f0" || failures[1]["messageId"] != "f1" {
		t.Fatalf("decryptFailures = %#v", payload["decryptFailures"])
	}
	if payload["partial"] != true {
		t.Fatalf("partial = %v, want true (or-merge)", payload["partial"])
	}

	untouched := map[string]any{"decryptedCount": 1}
	MergeDecryptLedger(untouched, nil)
	if untouched["decryptedCount"] != 1 || len(untouched) != 1 {
		t.Fatalf("nil ledger mutated payload: %#v", untouched)
	}
}
