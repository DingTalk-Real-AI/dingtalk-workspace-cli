// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package messagecrypto

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"reflect"
	"testing"
	"time"
)

type fakeRuntime struct {
	dryRun bool
	reads  []fakeCall
	writes []fakeCall
	read   map[string]any
	write  map[string]any
}

type fakeCall struct {
	product string
	tool    string
	params  map[string]any
}

func (r *fakeRuntime) CallMCPReadData(product, tool string, params map[string]any) (map[string]any, error) {
	r.reads = append(r.reads, fakeCall{product: product, tool: tool, params: cloneMap(params)})
	return r.read, nil
}

func (r *fakeRuntime) CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error) {
	r.writes = append(r.writes, fakeCall{product: product, tool: tool, params: cloneMap(params)})
	return r.write, nil
}

func (r *fakeRuntime) DryRun() bool {
	return r.dryRun
}

type fakeCipher struct {
	encryptCorp  string
	encryptStaff string
	encryptPlain []byte
	decryptCorp  string
	decryptStaff string
	decryptText  []byte
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
	return []byte("ding-cipher"), nil
}

func TestEncryptOutboundShouldReturnPlaintextDecisionWhenPolicyOff(t *testing.T) {
	rt := &fakeRuntime{read: map[string]any{"result": map[string]any{"mode": ModeOff, "reason": "admin-off"}}}
	client, _ := newTestClient(t)

	got, err := client.EncryptOutbound(context.Background(), rt, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got.Encrypted || got.Policy.Mode != ModeOff || got.Policy.Reason != "admin-off" {
		t.Fatalf("result = %#v", got)
	}
	if len(rt.reads) != 1 || len(rt.writes) != 0 {
		t.Fatalf("calls reads=%#v writes=%#v", rt.reads, rt.writes)
	}
}

func TestEncryptOutboundShouldPlanEncryptionWithoutWritesWhenDryRun(t *testing.T) {
	rt := &fakeRuntime{
		dryRun: true,
		read:   map[string]any{"result": map[string]any{"mode": ModeRequired}},
	}
	client, _ := newTestClient(t)

	got, err := client.EncryptOutbound(context.Background(), rt, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Encrypted || got.Ciphertext != "" {
		t.Fatalf("result = %#v", got)
	}
	if len(rt.writes) != 0 {
		t.Fatalf("dry-run writes = %#v", rt.writes)
	}
}

func TestEncryptOutboundShouldEncryptDingThenSafeChatWhenPolicyRequired(t *testing.T) {
	rt := &fakeRuntime{
		read:  map[string]any{"result": map[string]any{"mode": ModeRequired, "staffIdTransform": "md5"}},
		write: map[string]any{"dingCiphertext": "ding-cipher", "algorithm": "AES-CBC", "keyVersion": 7},
	}
	client, cipher := newTestClient(t)

	got, err := client.EncryptOutbound(context.Background(), rt, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Encrypted || got.Ciphertext != "safe:ding-cipher" || got.DingAlgorithm != "AES-CBC" || got.DingKeyVersion != 7 {
		t.Fatalf("result = %#v", got)
	}
	if len(rt.writes) != 1 || rt.writes[0].product != "im" || rt.writes[0].tool != "ding_encrypt_message" {
		t.Fatalf("writes = %#v", rt.writes)
	}
	if rt.writes[0].params["plaintextContent"] != "hello" {
		t.Fatalf("write params = %#v", rt.writes[0].params)
	}
	sum := md5.Sum([]byte("staff-1"))
	if cipher.encryptStaff != hex.EncodeToString(sum[:]) || string(cipher.encryptPlain) != "ding-cipher" {
		t.Fatalf("safechat staff=%q plain=%q", cipher.encryptStaff, cipher.encryptPlain)
	}
}

func TestEncryptOutboundShouldFailWhenDingCiphertextMissing(t *testing.T) {
	rt := &fakeRuntime{
		read:  map[string]any{"result": map[string]any{"mode": ModeRequired}},
		write: map[string]any{"ok": true},
	}
	client, _ := newTestClient(t)

	_, err := client.EncryptOutbound(context.Background(), rt, testOptions())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEncryptOutboundShouldReusePolicyCacheWithinTTL(t *testing.T) {
	now := time.Unix(100, 0)
	rt := &fakeRuntime{
		read: map[string]any{"result": map[string]any{"mode": ModeOff, "ttlSeconds": 60}},
	}
	client, _ := newTestClient(t)
	client.PolicyCache = NewPolicyCache(func() time.Time { return now })

	if _, err := client.EncryptOutbound(context.Background(), rt, testOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.EncryptOutbound(context.Background(), rt, testOptions()); err != nil {
		t.Fatal(err)
	}
	if len(rt.reads) != 1 {
		t.Fatalf("policy reads = %d, want 1", len(rt.reads))
	}
}

func TestDecryptInboundShouldDecryptSafeChatThenDing(t *testing.T) {
	rt := &fakeRuntime{write: map[string]any{"plaintextContent": "hello", "keyVersion": 9}}
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
	if !reflect.DeepEqual(rt.writes[0].params, map[string]any{"corpId": "corp-1", "dingCiphertext": "ding-cipher"}) {
		t.Fatalf("ding decrypt params = %#v", rt.writes[0].params)
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
		PlaintextContent:   "hello",
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
