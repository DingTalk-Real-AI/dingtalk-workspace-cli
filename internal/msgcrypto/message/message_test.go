// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package message

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRuntime struct {
	dryRun   bool
	reads    []fakeCall
	writes   []fakeCall
	read     map[string]any
	write    map[string]any
	readErr  error
	writeErr error
}

type fakeCall struct {
	product string
	tool    string
	params  map[string]any
}

func (r *fakeRuntime) CallMCPReadData(product, tool string, params map[string]any) (map[string]any, error) {
	r.reads = append(r.reads, fakeCall{product: product, tool: tool, params: cloneMap(params)})
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.read, nil
}

func (r *fakeRuntime) CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error) {
	r.writes = append(r.writes, fakeCall{product: product, tool: tool, params: cloneMap(params)})
	if r.writeErr != nil {
		return nil, r.writeErr
	}
	return r.write, nil
}

func (r *fakeRuntime) DryRun() bool {
	return r.dryRun
}

type fakeCipher struct {
	encryptCorp   string
	encryptStaff  string
	encryptPlain  []byte
	decryptCorp   string
	decryptStaff  string
	decryptText   []byte
	decryptByText map[string]fakeDecrypt
	decryptErr    error
}

type fakeDecrypt struct {
	plain []byte
	err   error
}

func (c *fakeCipher) EncryptMessage(_ context.Context, corpID, staffID string, plaintext []byte) ([]byte, error) {
	c.encryptCorp = corpID
	c.encryptStaff = staffID
	c.encryptPlain = append([]byte(nil), plaintext...)
	return []byte("safe:" + string(plaintext)), nil
}

func (c *fakeCipher) DecryptMessage(_ context.Context, corpID, staffID string, ciphertext []byte) ([]byte, error) {
	c.decryptCorp = corpID
	c.decryptStaff = staffID
	c.decryptText = append([]byte(nil), ciphertext...)
	if c.decryptErr != nil {
		return nil, c.decryptErr
	}
	if c.decryptByText != nil {
		if got, ok := c.decryptByText[string(ciphertext)]; ok {
			return got.plain, got.err
		}
	}
	return []byte("ding-cipher"), nil
}

func TestDefaultClientShouldBePolicyCacheReadyAndBackendDisabled(t *testing.T) {
	client := DefaultClient()
	if client.BackendReady() {
		t.Fatal("default backend should be disabled")
	}
	if client.PolicyCache == nil {
		t.Fatal("default policy cache is nil")
	}
}

func TestPolicyDecisionShouldReusePolicyCacheWithinTTL(t *testing.T) {
	now := time.Unix(100, 0)
	rt := &fakeRuntime{
		read: map[string]any{"result": map[string]any{"mode": ModeOff, "ttlSeconds": 60}},
	}
	client, _ := newTestClient(t)
	client.PolicyCache = NewPolicyCache(func() time.Time { return now })

	if _, err := client.PolicyDecision(context.Background(), rt, testOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PolicyDecision(context.Background(), rt, testOptions()); err != nil {
		t.Fatal(err)
	}
	if len(rt.reads) != 1 {
		t.Fatalf("policy reads = %d, want 1", len(rt.reads))
	}
}

func TestPolicyDecisionShouldReturnDisabledWhenPolicyModeOff(t *testing.T) {
	rt := &fakeRuntime{read: map[string]any{"success": true}}
	client, _ := newTestClient(t)

	got, err := client.PolicyDecision(context.Background(), rt, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Policy.Mode != ModeOff {
		t.Fatalf("decision = %#v", got)
	}
}

func TestPolicyDecisionShouldReturnEnabledWhenPolicyRequiresCrypto(t *testing.T) {
	rt := &fakeRuntime{read: map[string]any{"result": map[string]any{
		"mode":                ModeRequired,
		"provider":            "safechat",
		"keyServer":           "https://keys.example.test",
		"allowedRedirectHost": "auth.example.test",
		"staffIdTransform":    "raw",
		"ttlSeconds":          int64(30),
		"reason":              "admin_enabled",
	}}}
	client, _ := newTestClient(t)

	got, err := client.PolicyDecision(context.Background(), rt, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Policy.TTL != 30*time.Second || got.Policy.Reason != "admin_enabled" {
		t.Fatalf("decision = %#v", got)
	}
}

func TestPolicyDecisionShouldReturnIdentityError(t *testing.T) {
	client, _ := newTestClient(t)
	client.Identity = func(context.Context, string) (Identity, error) {
		return Identity{}, errors.New("identity boom")
	}

	_, err := client.PolicyDecision(context.Background(), &fakeRuntime{}, testOptions())
	if err == nil || !strings.Contains(err.Error(), "identity boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicyDecisionShouldFailWithoutIdentityProvider(t *testing.T) {
	_, err := (&Client{}).PolicyDecision(context.Background(), &fakeRuntime{}, testOptions())
	if err == nil || !strings.Contains(err.Error(), "identity provider") {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicyDecisionShouldReturnPolicyReadError(t *testing.T) {
	client, _ := newTestClient(t)

	_, err := client.PolicyDecision(context.Background(), &fakeRuntime{readErr: errors.New("policy boom")}, testOptions())
	if err == nil || !strings.Contains(err.Error(), "policy boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicyDecisionShouldRejectUnknownPolicyMode(t *testing.T) {
	client, _ := newTestClient(t)

	_, err := client.PolicyDecision(context.Background(), &fakeRuntime{read: map[string]any{"mode": "mystery"}}, testOptions())
	if err == nil || !strings.Contains(err.Error(), "未知 mode") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldDecryptSafeChatThenDing(t *testing.T) {
	rt := &fakeRuntime{write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "manual", "status": "success", "plaintextContent": "hello", "keyVersion": 9},
	}}}}
	client, cipher := newTestClient(t)

	got, err := client.DecryptInbound(context.Background(), rt, Options{Layer: "full", Ciphertext: "safe-cipher"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plaintext != "hello" || got.DingCiphertext != "ding-cipher" || got.KeyVersion != 9 {
		t.Fatalf("result = %#v", got)
	}
	if string(cipher.decryptText) != "safe-cipher" || len(rt.writes) != 1 {
		t.Fatalf("decryptText=%q writes=%#v", cipher.decryptText, rt.writes)
	}
	if rt.writes[0].tool != "batch_ding_decrypt_messages" {
		t.Fatalf("ding decrypt tool = %q", rt.writes[0].tool)
	}
	items, _ := rt.writes[0].params["items"].([]map[string]any)
	if len(items) != 1 || !reflect.DeepEqual(items[0], map[string]any{"messageId": "manual", "dingCiphertext": "ding-cipher"}) {
		t.Fatalf("ding batch decrypt params = %#v", rt.writes[0].params)
	}
}

func TestDecryptInboundShouldDefaultToFullLayer(t *testing.T) {
	rt := &fakeRuntime{write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "manual", "status": "success", "plaintextContent": "hello"},
	}}}}
	client, _ := newTestClient(t)

	got, err := client.DecryptInbound(context.Background(), rt, Options{Ciphertext: "safe-cipher"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Layer != "full" || got.Plaintext != "hello" {
		t.Fatalf("result = %#v", got)
	}
}

func TestDecryptInboundShouldCallDingBatchDirectlyForDingLayer(t *testing.T) {
	rt := &fakeRuntime{write: map[string]any{"items": []any{
		map[string]any{"msgId": "manual", "plaintext": "hello-from-ding", "keyVersion": float64(7)},
	}}}
	client, cipher := newTestClient(t)

	got, err := client.DecryptInbound(context.Background(), rt, Options{Layer: "ding", Ciphertext: " ding-cipher-only "})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plaintext != "hello-from-ding" || got.DingCiphertext != "ding-cipher-only" || got.KeyVersion != 7 {
		t.Fatalf("result = %#v", got)
	}
	if cipher.decryptText != nil {
		t.Fatalf("safechat decrypt should not run, decryptText=%q", cipher.decryptText)
	}
	items, _ := rt.writes[0].params["items"].([]map[string]any)
	if len(items) != 1 || !reflect.DeepEqual(items[0], map[string]any{"messageId": "manual", "dingCiphertext": "ding-cipher-only"}) {
		t.Fatalf("ding batch decrypt params = %#v", rt.writes[0].params)
	}
}

func TestDecryptInboundShouldReturnSafeChatPlaintextWhenLayerSafeChat(t *testing.T) {
	rt := &fakeRuntime{}
	client, _ := newTestClient(t)

	got, err := client.DecryptInbound(context.Background(), rt, Options{Layer: "safechat", Ciphertext: "safe-cipher"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plaintext != "ding-cipher" || got.DingCiphertext != "ding-cipher" {
		t.Fatalf("result = %#v", got)
	}
	if len(rt.writes) != 0 {
		t.Fatalf("writes = %#v", rt.writes)
	}
}

func TestDecryptInboundShouldFailWhenSafeChatSessionMissing(t *testing.T) {
	client, _ := newTestClient(t)
	client.OpenSession = nil

	_, err := client.DecryptInbound(context.Background(), &fakeRuntime{}, Options{Layer: "full", Ciphertext: "safe-cipher"})
	if err == nil || !strings.Contains(err.Error(), "session provider") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldFailWhenSafeChatDecryptFails(t *testing.T) {
	client, cipher := newTestClient(t)
	cipher.decryptErr = errors.New("safechat boom")

	_, err := client.DecryptInbound(context.Background(), &fakeRuntime{}, Options{Layer: "full", Ciphertext: "safe-cipher"})
	if err == nil || !strings.Contains(err.Error(), "safechat boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldFailWhenDingBatchCallFails(t *testing.T) {
	client, _ := newTestClient(t)

	_, err := client.DecryptInbound(context.Background(), &fakeRuntime{writeErr: errors.New("ding boom")}, Options{Layer: "ding", Ciphertext: "ding-cipher"})
	if err == nil || !strings.Contains(err.Error(), "ding boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldFailWhenDingBatchReturnsNoItems(t *testing.T) {
	client, _ := newTestClient(t)

	_, err := client.DecryptInbound(context.Background(), &fakeRuntime{write: map[string]any{"result": map[string]any{}}}, Options{Layer: "ding", Ciphertext: "ding-cipher"})
	if err == nil || !strings.Contains(err.Error(), "未返回 plaintextContent") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldFailWhenDingBatchItemFails(t *testing.T) {
	client, _ := newTestClient(t)
	rt := &fakeRuntime{write: map[string]any{"result": map[string]any{"results": []any{
		map[string]any{"messageId": "manual", "status": "failed", "error": "bad_key"},
	}}}}

	_, err := client.DecryptInbound(context.Background(), rt, Options{Layer: "ding", Ciphertext: "ding-cipher"})
	if err == nil || !strings.Contains(err.Error(), "bad_key") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldFailWhenDingBatchPlaintextEmpty(t *testing.T) {
	client, _ := newTestClient(t)
	rt := &fakeRuntime{write: map[string]any{"result": map[string]any{"messages": []any{
		map[string]any{"messageId": "manual", "status": "success", "plaintextContent": "  "},
	}}}}

	_, err := client.DecryptInbound(context.Background(), rt, Options{Layer: "ding", Ciphertext: "ding-cipher"})
	if err == nil || !strings.Contains(err.Error(), "未返回 plaintextContent") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecryptInboundShouldRejectUnknownLayer(t *testing.T) {
	rt := &fakeRuntime{}
	client, _ := newTestClient(t)

	_, err := client.DecryptInbound(context.Background(), rt, Options{Layer: "unknown", Ciphertext: "safe-cipher"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBatchDecryptInboundShouldDecryptSafeChatAndCallDingBatchOnce(t *testing.T) {
	rt := &fakeRuntime{write: map[string]any{"result": map[string]any{"items": []any{
		"ignored",
		map[string]any{"messageId": "m1", "status": "success", "plaintextContent": "hello", "keyVersion": 4},
		map[string]any{"messageId": "m2", "status": "failed", "reason": "bad_key"},
	}}}}
	client, cipher := newTestClient(t)

	got, err := client.BatchDecryptInbound(context.Background(), rt, Options{}, []BatchDecryptItem{
		{MessageID: "m1", ConversationID: "cid", Ciphertext: "safe-1"},
		{MessageID: "m2", ConversationID: "cid", Ciphertext: "safe-2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.writes) != 1 || rt.writes[0].product != "im" || rt.writes[0].tool != "batch_ding_decrypt_messages" {
		t.Fatalf("writes = %#v", rt.writes)
	}
	if _, ok := rt.writes[0].params["corpId"]; ok {
		t.Fatalf("batch ding decrypt params should not expose corpId: %#v", rt.writes[0].params)
	}
	items, _ := rt.writes[0].params["items"].([]map[string]any)
	if len(items) != 2 || items[0]["dingCiphertext"] != "ding-cipher" || items[1]["messageId"] != "m2" {
		t.Fatalf("batch items = %#v", rt.writes[0].params["items"])
	}
	if len(got.Items) != 2 || got.Items[0].PlaintextContent != "hello" || got.Items[0].KeyVersion != 4 ||
		got.Items[1].Status != "failed" || got.Items[1].Reason != "bad_key" {
		t.Fatalf("result = %#v", got)
	}
	if string(cipher.decryptText) != "safe-2" {
		t.Fatalf("last safechat ciphertext = %q", cipher.decryptText)
	}
}

func TestBatchDecryptInboundShouldReturnEmptyForInvalidItems(t *testing.T) {
	rt := &fakeRuntime{}
	client, _ := newTestClient(t)

	got, err := client.BatchDecryptInbound(context.Background(), rt, Options{}, []BatchDecryptItem{
		{MessageID: " ", Ciphertext: "safe-1"},
		{MessageID: "m1", Ciphertext: " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 0 || len(got.Failures) != 0 || len(rt.writes) != 0 {
		t.Fatalf("result=%#v writes=%#v", got, rt.writes)
	}
}

func TestBatchDecryptInboundShouldDeduplicateAndTrimItems(t *testing.T) {
	rt := &fakeRuntime{write: map[string]any{"result": map[string]any{"items": []any{
		map[string]any{"messageId": "m1", "status": "success", "plaintextContent": "hello"},
	}}}}
	client, cipher := newTestClient(t)
	cipher.decryptByText = map[string]fakeDecrypt{
		"safe-1": {plain: []byte("ding-1")},
	}

	_, err := client.BatchDecryptInbound(context.Background(), rt, Options{}, []BatchDecryptItem{
		{MessageID: " m1 ", ConversationID: " cid ", Ciphertext: " safe-1 "},
		{MessageID: "m1", ConversationID: "cid", Ciphertext: "safe-duplicate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := rt.writes[0].params["items"].([]map[string]any)
	if len(items) != 1 || items[0]["messageId"] != "m1" || items[0]["conversationId"] != "cid" || items[0]["dingCiphertext"] != "ding-1" {
		t.Fatalf("batch items = %#v", items)
	}
}

func TestBatchDecryptInboundShouldRecordSafeChatFailuresAndSkipDingWhenAllFail(t *testing.T) {
	rt := &fakeRuntime{}
	client, cipher := newTestClient(t)
	cipher.decryptErr = errors.New("bad safechat")

	got, err := client.BatchDecryptInbound(context.Background(), rt, Options{}, []BatchDecryptItem{
		{MessageID: "m1", ConversationID: "cid", Ciphertext: "safe-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.writes) != 0 {
		t.Fatalf("writes = %#v", rt.writes)
	}
	if len(got.Failures) != 1 || got.Failures[0].MessageID != "m1" || got.Failures[0].Reason != "safechat_decrypt_failed" {
		t.Fatalf("result = %#v", got)
	}
}

func TestBatchDecryptInboundShouldReturnSessionError(t *testing.T) {
	client, _ := newTestClient(t)
	client.OpenSession = nil

	_, err := client.BatchDecryptInbound(context.Background(), &fakeRuntime{}, Options{}, []BatchDecryptItem{
		{MessageID: "m1", Ciphertext: "safe-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "session provider") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchDecryptInboundShouldReturnDingBatchCallError(t *testing.T) {
	client, _ := newTestClient(t)

	_, err := client.BatchDecryptInbound(context.Background(), &fakeRuntime{writeErr: errors.New("ding boom")}, Options{}, []BatchDecryptItem{
		{MessageID: "m1", Ciphertext: "safe-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "ding boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestPolicyCacheShouldDropExpiredAndSkipNonPositiveTTL(t *testing.T) {
	now := time.Unix(200, 0)
	cache := NewPolicyCache(func() time.Time { return now })
	cache.Set("off", Policy{Mode: ModeRequired})
	if _, ok := cache.Get("off"); ok {
		t.Fatal("non-positive ttl policy should not be cached")
	}
	cache.Set("short", Policy{Mode: ModeRequired, TTL: time.Second})
	now = now.Add(2 * time.Second)
	if _, ok := cache.Get("short"); ok {
		t.Fatal("expired policy should not be returned")
	}
}

func TestNewPolicyCacheShouldUseDefaultClockWhenNil(t *testing.T) {
	cache := NewPolicyCache(nil)
	cache.Set("key", Policy{Mode: ModeRequired, TTL: time.Second})
	if got, ok := cache.Get("key"); !ok || got.Mode != ModeRequired {
		t.Fatalf("cache get = %#v, %v", got, ok)
	}
}

func TestHelpersShouldParseIntegerFieldVariants(t *testing.T) {
	data := map[string]any{
		"i":   int(1),
		"l":   int64(2),
		"f":   float64(3),
		"s":   "4",
		"bad": "x",
	}
	for key, want := range map[string]int{"i": 1, "l": 2, "f": 3, "s": 4} {
		if got := intField(data, key); got != want {
			t.Fatalf("intField(%s) = %d, want %d", key, got, want)
		}
	}
	if got := intField(data, "bad", "missing"); got != 0 {
		t.Fatalf("bad intField = %d", got)
	}
}

func TestCrossPlatformCoverageMessageCryptoPackage(t *testing.T) {
	cases := []struct {
		name string
		run  func(*testing.T)
	}{
		{"default_client", TestDefaultClientShouldBePolicyCacheReadyAndBackendDisabled},
		{"policy_cache_reuse", TestPolicyDecisionShouldReusePolicyCacheWithinTTL},
		{"policy_off", TestPolicyDecisionShouldReturnDisabledWhenPolicyModeOff},
		{"policy_required", TestPolicyDecisionShouldReturnEnabledWhenPolicyRequiresCrypto},
		{"identity_error", TestPolicyDecisionShouldReturnIdentityError},
		{"missing_identity_provider", TestPolicyDecisionShouldFailWithoutIdentityProvider},
		{"policy_read_error", TestPolicyDecisionShouldReturnPolicyReadError},
		{"unknown_policy_mode", TestPolicyDecisionShouldRejectUnknownPolicyMode},
		{"decrypt_full", TestDecryptInboundShouldDecryptSafeChatThenDing},
		{"decrypt_default_layer", TestDecryptInboundShouldDefaultToFullLayer},
		{"decrypt_ding_layer", TestDecryptInboundShouldCallDingBatchDirectlyForDingLayer},
		{"decrypt_safechat_layer", TestDecryptInboundShouldReturnSafeChatPlaintextWhenLayerSafeChat},
		{"decrypt_missing_session", TestDecryptInboundShouldFailWhenSafeChatSessionMissing},
		{"decrypt_safechat_error", TestDecryptInboundShouldFailWhenSafeChatDecryptFails},
		{"decrypt_ding_call_error", TestDecryptInboundShouldFailWhenDingBatchCallFails},
		{"decrypt_no_items", TestDecryptInboundShouldFailWhenDingBatchReturnsNoItems},
		{"decrypt_failed_item", TestDecryptInboundShouldFailWhenDingBatchItemFails},
		{"decrypt_empty_plaintext", TestDecryptInboundShouldFailWhenDingBatchPlaintextEmpty},
		{"unknown_layer", TestDecryptInboundShouldRejectUnknownLayer},
		{"batch_success", TestBatchDecryptInboundShouldDecryptSafeChatAndCallDingBatchOnce},
		{"batch_invalid_items", TestBatchDecryptInboundShouldReturnEmptyForInvalidItems},
		{"batch_dedupe", TestBatchDecryptInboundShouldDeduplicateAndTrimItems},
		{"batch_safechat_failures", TestBatchDecryptInboundShouldRecordSafeChatFailuresAndSkipDingWhenAllFail},
		{"batch_session_error", TestBatchDecryptInboundShouldReturnSessionError},
		{"batch_ding_call_error", TestBatchDecryptInboundShouldReturnDingBatchCallError},
		{"cache_expired", TestPolicyCacheShouldDropExpiredAndSkipNonPositiveTTL},
		{"cache_default_clock", TestNewPolicyCacheShouldUseDefaultClockWhenNil},
		{"int_variants", TestHelpersShouldParseIntegerFieldVariants},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

func newTestClient(t *testing.T) (*Client, *fakeCipher) {
	t.Helper()
	cipher := &fakeCipher{}
	return &Client{
		Identity: func(context.Context, string) (Identity, error) {
			return Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, SessionOptions) (*Session, error) {
			return &Session{Cipher: cipher, CorpID: "corp-1", StaffID: "staff-1", Close: func() error { return nil }}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  NewPolicyCache(time.Now),
	}, cipher
}

func testOptions() Options {
	return Options{
		Identity:           "user",
		MsgType:            "text",
		OpenConversationID: "cid-1",
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
