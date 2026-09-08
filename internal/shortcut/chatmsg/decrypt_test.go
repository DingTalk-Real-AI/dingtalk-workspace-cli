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

package chatmsg

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
)

const decryptTestCipher = "SwzNkAraDE6lUHUNlVT3mjFdbxL6dWvmt77XtjACdpJx9VFibzTbW9KtDbkzGOYP||2||1||1"

type decryptTestCipherFake struct{}

func (decryptTestCipherFake) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (decryptTestCipherFake) DecryptMessage(_ context.Context, _, _ string, ciphertext []byte) ([]byte, error) {
	return []byte("ding:" + string(ciphertext)), nil
}

type decryptTestRuntime struct {
	dryRun           bool
	policyMode       string
	policyTTLSeconds int
	policyErr        error
	batchData        string
	batchErr         error
	readCalls        int
	writeCalls       int
}

func (r *decryptTestRuntime) CallMCPReadData(_ string, _ string, params map[string]any) (map[string]any, error) {
	r.readCalls++
	if r.policyErr != nil {
		return nil, r.policyErr
	}
	mode := r.policyMode
	if mode == "" {
		mode = "required"
	}
	result := map[string]any{"mode": mode, "openConversationId": params["openConversationId"]}
	if r.policyTTLSeconds > 0 {
		result["ttlSeconds"] = r.policyTTLSeconds
	}
	return map[string]any{"result": result}, nil
}

func (r *decryptTestRuntime) CallMCPWriteDataStrict(_ string, _ string, params map[string]any) (map[string]any, error) {
	r.writeCalls++
	if r.batchErr != nil {
		return nil, r.batchErr
	}
	if r.batchData != "" {
		var data map[string]any
		if err := json.Unmarshal([]byte(r.batchData), &data); err != nil {
			return nil, err
		}
		return data, nil
	}
	dingItems, _ := params["items"].([]map[string]any)
	echo := make([]any, 0, len(dingItems))
	for _, item := range dingItems {
		echo = append(echo, map[string]any{
			"messageId":        item["messageId"],
			"conversationId":   item["conversationId"],
			"status":           "success",
			"plaintextContent": item["dingCiphertext"],
		})
	}
	return map[string]any{"result": map[string]any{"items": echo}}, nil
}

func (r *decryptTestRuntime) DryRun() bool { return r.dryRun }

func decryptTestClient(backendReady bool) *messagecrypto.Client {
	return &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: decryptTestCipherFake{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		BackendReady: func() bool { return backendReady },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	}
}

func encryptedTestMessage(id string) map[string]any {
	return map[string]any{
		"openMessageId":      id,
		"openConversationId": "cid",
		"content":            decryptTestCipher,
	}
}

func TestDecryptMessagesByPolicySkipsWhenDryRun(t *testing.T) {
	rt := &decryptTestRuntime{dryRun: true}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true),
		[]map[string]any{encryptedTestMessage("m1")}, DecryptOptions{MarkFailedOriginal: true})
	if ledger != nil {
		t.Fatalf("dry-run ledger = %#v, want nil", ledger)
	}
	if rt.readCalls != 0 || rt.writeCalls != 0 {
		t.Fatalf("dry-run calls = %d/%d, want zero", rt.readCalls, rt.writeCalls)
	}
}

func TestDecryptMessagesByPolicySkipsWhenBackendUnavailable(t *testing.T) {
	rt := &decryptTestRuntime{}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(false),
		[]map[string]any{encryptedTestMessage("m1")}, DecryptOptions{})
	if ledger != nil {
		t.Fatalf("unavailable backend ledger = %#v, want nil", ledger)
	}
	if rt.readCalls != 0 || rt.writeCalls != 0 {
		t.Fatalf("unavailable backend calls = %d/%d, want zero", rt.readCalls, rt.writeCalls)
	}
}

func TestDecryptMessagesByPolicyZeroCandidatesPublishesZeroCounters(t *testing.T) {
	rt := &decryptTestRuntime{}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true),
		[]map[string]any{{"openMessageId": "m1", "openConversationId": "cid", "content": "plain text"}}, DecryptOptions{})
	if ledger == nil {
		t.Fatal("zero candidates must still publish the canonical counters")
	}
	if ledger["decryptCandidateCount"] != 0 || ledger["decryptAllowedCount"] != 0 ||
		ledger["decryptedCount"] != 0 || ledger["decryptFailedCount"] != 0 {
		t.Fatalf("zero-candidate ledger = %#v", ledger)
	}
	if _, ok := ledger["decryptFailures"]; ok {
		t.Fatalf("zero-candidate ledger must not carry failures: %#v", ledger)
	}
	if _, ok := ledger["partial"]; ok {
		t.Fatalf("zero-candidate ledger must not carry partial: %#v", ledger)
	}
	if rt.readCalls != 0 {
		t.Fatalf("zero candidates must not query policy, readCalls = %d", rt.readCalls)
	}
}

func TestDecryptMessagesByPolicyDecryptsEncryptedBatch(t *testing.T) {
	rt := &decryptTestRuntime{}
	messages := []map[string]any{encryptedTestMessage("m1"), encryptedTestMessage("m2")}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{})
	if ledger["decryptCandidateCount"] != 2 || ledger["decryptAllowedCount"] != 2 ||
		ledger["decryptedCount"] != 2 || ledger["decryptFailedCount"] != 0 {
		t.Fatalf("decrypt ledger = %#v", ledger)
	}
	if _, ok := ledger["partial"]; ok {
		t.Fatalf("success ledger must not carry partial: %#v", ledger)
	}
	if rt.writeCalls != 1 || rt.readCalls != 2 {
		t.Fatalf("calls = %d reads / %d writes, want 2/1", rt.readCalls, rt.writeCalls)
	}
	for _, message := range messages {
		if message["content"] != "ding:"+decryptTestCipher {
			t.Fatalf("decrypted content = %#v", message["content"])
		}
		if message["contentDecrypted"] != true || message["cryptoLayer"] != "ding+safechat" {
			t.Fatalf("decrypt markers = %#v", message)
		}
	}
}

func TestDecryptMessagesByPolicyWritesDingCiphertextAndKeyVersion(t *testing.T) {
	rt := &decryptTestRuntime{
		batchData: `{"result":{"items":[{"messageId":"m1","status":"success","plaintextContent":"hello","keyVersion":8}]}}`,
	}
	messages := []map[string]any{encryptedTestMessage("m1")}
	var batchParams map[string]any
	recording := &batchRecordingRuntime{decryptTestRuntime: rt, capture: &batchParams}
	DecryptMessagesByPolicy(context.Background(), recording, decryptTestClient(true), messages, DecryptOptions{})
	items, _ := batchParams["items"].([]map[string]any)
	if len(items) != 1 || items[0]["messageId"] != "m1" || items[0]["dingCiphertext"] != "ding:"+decryptTestCipher {
		t.Fatalf("batch args = %#v", batchParams)
	}
	if messages[0]["content"] != "hello" {
		t.Fatalf("plaintext content = %#v", messages[0]["content"])
	}
	if got, ok := messages[0]["dingKeyVersion"].(int); !ok || got != 8 {
		t.Fatalf("dingKeyVersion = %#v, want 8", messages[0]["dingKeyVersion"])
	}
}

type batchRecordingRuntime struct {
	*decryptTestRuntime
	capture *map[string]any
}

func (r *batchRecordingRuntime) CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error) {
	*r.capture = params
	return r.decryptTestRuntime.CallMCPWriteDataStrict(product, tool, params)
}

func TestDecryptMessagesByPolicyRecordsPolicyDisabled(t *testing.T) {
	rt := &decryptTestRuntime{policyMode: "off"}
	messages := []map[string]any{encryptedTestMessage("m1"), encryptedTestMessage("m2")}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{MarkFailedOriginal: true})
	if ledger["decryptCandidateCount"] != 2 || ledger["decryptAllowedCount"] != 0 || ledger["decryptFailedCount"] != 2 {
		t.Fatalf("policy-off ledger = %#v", ledger)
	}
	if ledger["partial"] != true {
		t.Fatalf("policy-off ledger must be partial: %#v", ledger)
	}
	if rt.writeCalls != 0 {
		t.Fatalf("policy-off must not decrypt, writeCalls = %d", rt.writeCalls)
	}
	failures, _ := ledger["decryptFailures"].([]map[string]any)
	if len(failures) != 2 {
		t.Fatalf("decryptFailures = %#v", ledger["decryptFailures"])
	}
	for _, failure := range failures {
		if failure["stage"] != "message-decrypt" || failure["reason"] != "policy_disabled" {
			t.Fatalf("failure = %#v", failure)
		}
		if failure["messageId"] == "" || failure["conversationId"] != "cid" {
			t.Fatalf("failure identity = %#v", failure)
		}
	}
	for _, message := range messages {
		if message["content"] != decryptTestCipher {
			t.Fatalf("policy-off message must keep ciphertext: %#v", message["content"])
		}
		if message[messageDecryptFailedOriginalContentKey] != decryptTestCipher {
			t.Fatalf("policy-off failure must mark original: %#v", message)
		}
	}
}

func TestDecryptMessagesByPolicyRecordsPolicyQueryFailure(t *testing.T) {
	rt := &decryptTestRuntime{policyErr: errors.New("policy unavailable")}
	messages := []map[string]any{encryptedTestMessage("m1")}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{MarkFailedOriginal: true})
	if ledger["decryptAllowedCount"] != 0 || ledger["decryptFailedCount"] != 1 {
		t.Fatalf("policy failure ledger = %#v", ledger)
	}
	failures, _ := ledger["decryptFailures"].([]map[string]any)
	if len(failures) != 1 || failures[0]["reason"] != "policy unavailable" {
		t.Fatalf("decryptFailures = %#v", ledger["decryptFailures"])
	}
	if rt.writeCalls != 0 {
		t.Fatalf("policy failure must not decrypt, writeCalls = %d", rt.writeCalls)
	}
}

func TestDecryptMessagesByPolicyRecordsPerItemBatchFailures(t *testing.T) {
	rt := &decryptTestRuntime{
		batchData: `{"result":{"items":[` +
			`{"messageId":"ok","status":"success","plaintextContent":"ok text","keyVersion":3},` +
			`{"messageId":"failed","status":"failed","reason":"bad_key"},` +
			`{"messageId":"empty","status":"success","plaintextContent":"  "},` +
			`{"messageId":"lost","status":"failed","reason":"gone"}` +
			`]}}`,
	}
	messages := []map[string]any{encryptedTestMessage("ok"), encryptedTestMessage("failed"), encryptedTestMessage("empty")}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{MarkFailedOriginal: true})
	if ledger["decryptedCount"] != 1 || ledger["decryptFailedCount"] != 3 || ledger["partial"] != true {
		t.Fatalf("item failure ledger = %#v", ledger)
	}
	reasons := map[string]bool{}
	for _, failure := range ledger["decryptFailures"].([]map[string]any) {
		reason, _ := failure["reason"].(string)
		reasons[reason] = true
	}
	if !reasons["bad_key"] || !reasons["empty_plaintext"] || !reasons["gone"] {
		t.Fatalf("failure reasons = %#v", reasons)
	}
	if messages[0]["content"] != "ok text" {
		t.Fatalf("successful message not decrypted: %#v", messages[0]["content"])
	}
	if messages[1]["content"] != decryptTestCipher || messages[2]["content"] != decryptTestCipher {
		t.Fatalf("failed messages must keep ciphertext: %#v / %#v", messages[1]["content"], messages[2]["content"])
	}
}

func TestDecryptMessagesByPolicyTransportErrorRecordsPerItemFailures(t *testing.T) {
	rt := &decryptTestRuntime{batchErr: errors.New("ding batch failed")}
	messages := []map[string]any{encryptedTestMessage("m1"), encryptedTestMessage("m2")}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{MarkFailedOriginal: true})
	if ledger["decryptFailedCount"] != 2 || ledger["partial"] != true {
		t.Fatalf("transport error ledger = %#v", ledger)
	}
	failures, _ := ledger["decryptFailures"].([]map[string]any)
	if len(failures) != 2 {
		t.Fatalf("decryptFailures = %#v, want one per filtered item", ledger["decryptFailures"])
	}
	ids := map[string]bool{}
	for _, failure := range failures {
		id, _ := failure["messageId"].(string)
		ids[id] = true
		if failure["reason"] != "ding batch failed" {
			t.Fatalf("failure = %#v", failure)
		}
	}
	if !ids["m1"] || !ids["m2"] {
		t.Fatalf("transport failures must carry message ids: %#v", ids)
	}
}

func TestDecryptMessagesByPolicyPolicyCacheTTLReusesConversationDecisions(t *testing.T) {
	rt := &decryptTestRuntime{policyTTLSeconds: 60}
	messages := []map[string]any{
		encryptedTestMessage("m1"),
		{"openMessageId": "m2", "openConversationId": "cid", "content": decryptTestCipher},
		{"openMessageId": "m3", "openConversationId": "cid-2", "content": decryptTestCipher},
	}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{})
	if ledger["decryptAllowedCount"] != 3 || ledger["decryptedCount"] != 3 {
		t.Fatalf("ttl ledger = %#v", ledger)
	}
	if rt.readCalls != 2 {
		t.Fatalf("readCalls = %d, want one per conversation (2), not per message (3)", rt.readCalls)
	}
	if rt.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want a single batch decrypt", rt.writeCalls)
	}
}

func TestDecryptMessagesByPolicyCollectsForwardedChildren(t *testing.T) {
	rt := &decryptTestRuntime{
		batchData: `{"result":{"items":[{"messageId":"child","status":"success","plaintextContent":"child text","keyVersion":2}]}}`,
	}
	parent := map[string]any{
		"openMessageId":      "root",
		"openConversationId": "cid",
		"content":            "plain root",
		"forwardMessages": []any{
			map[string]any{"openMessageId": "child", "openConversationId": "cid", "text": decryptTestCipher},
			"ignored",
		},
	}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), []map[string]any{parent}, DecryptOptions{})
	if ledger["decryptCandidateCount"] != 1 || ledger["decryptedCount"] != 1 {
		t.Fatalf("forwarded ledger = %#v", ledger)
	}
	child, _ := parent["forwardMessages"].([]any)[0].(map[string]any)
	if child["text"] != "child text" || child["contentDecrypted"] != true {
		t.Fatalf("forwarded child = %#v", child)
	}
}

func TestDecryptMessagesByPolicySkipsMessagesWithoutStableID(t *testing.T) {
	rt := &decryptTestRuntime{}
	messages := []map[string]any{
		{"openConversationId": "cid", "content": decryptTestCipher},
		{"messageId": "<nil>", "openConversationId": "cid", "content": decryptTestCipher},
		{"openMessageId": "", "openConversationId": "cid", "content": decryptTestCipher},
	}
	ledger := DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{})
	if ledger["decryptCandidateCount"] != 0 {
		t.Fatalf("unstable ids must not be collected: %#v", ledger)
	}
	if rt.readCalls != 0 || rt.writeCalls != 0 {
		t.Fatalf("unstable ids must not trigger calls: %d/%d", rt.readCalls, rt.writeCalls)
	}
}

func TestDecryptMessagesByPolicyMarkFailedOriginalGating(t *testing.T) {
	rt := &decryptTestRuntime{policyMode: "off"}
	messages := []map[string]any{encryptedTestMessage("m1"), {"openMessageId": "m2", "openConversationId": "cid", "content": "plain"}}
	DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{})
	if _, ok := messages[0][messageDecryptFailedOriginalContentKey]; ok {
		t.Fatalf("MarkFailedOriginal=false must not mark: %#v", messages[0])
	}

	rt = &decryptTestRuntime{policyMode: "off"}
	messages = []map[string]any{encryptedTestMessage("m1"), {"openMessageId": "m2", "openConversationId": "cid", "content": "plain"}}
	DecryptMessagesByPolicy(context.Background(), rt, decryptTestClient(true), messages, DecryptOptions{MarkFailedOriginal: true})
	if messages[0][messageDecryptFailedOriginalContentKey] != decryptTestCipher {
		t.Fatalf("MarkFailedOriginal=true must restore original: %#v", messages[0])
	}
	if _, ok := messages[1][messageDecryptFailedOriginalContentKey]; ok {
		t.Fatalf("plaintext message must not be marked: %#v", messages[1])
	}
}

func TestApplyDecryptLedger(t *testing.T) {
	payload := map[string]any{"count": 1}
	ApplyDecryptLedger(payload, nil)
	if len(payload) != 1 {
		t.Fatalf("nil ledger must not write: %#v", payload)
	}
	ledger := map[string]any{
		"decryptCandidateCount": 2,
		"decryptAllowedCount":   1,
		"decryptedCount":        1,
		"decryptFailedCount":    1,
		"decryptFailures":       []map[string]any{{"stage": "message-decrypt"}},
		"partial":               true,
	}
	ApplyDecryptLedger(payload, ledger)
	if payload["decryptCandidateCount"] != 2 || payload["decryptAllowedCount"] != 1 ||
		payload["decryptedCount"] != 1 || payload["decryptFailedCount"] != 1 ||
		payload["partial"] != true {
		t.Fatalf("merged payload = %#v", payload)
	}
	if _, ok := payload["decryptFailures"].([]map[string]any); !ok {
		t.Fatalf("decryptFailures = %#v", payload["decryptFailures"])
	}
	ApplyDecryptLedger(nil, ledger)
}

func TestMessageDecryptClientStore(t *testing.T) {
	t.Cleanup(func() { SetMessageDecryptClient(nil) })
	defaultClient := MessageDecryptClient()
	if defaultClient == nil || defaultClient.BackendReady == nil || defaultClient.BackendReady() {
		t.Fatalf("default client = %#v, want inert DefaultClient", defaultClient)
	}
	custom := decryptTestClient(true)
	SetMessageDecryptClient(custom)
	if MessageDecryptClient() != custom {
		t.Fatal("SetMessageDecryptClient must store the injected client")
	}
	SetMessageDecryptClient(nil)
	if MessageDecryptClient() == nil || MessageDecryptClient().BackendReady() {
		t.Fatal("nil reset must restore the inert DefaultClient")
	}
}
