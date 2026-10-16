// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// These tests exercise the real Cobra leaf with injected MCP responses. They
// perform no live requests and do not require credentials or a DingTalk account.
func runDirectMessageIntegrityRegression(t *testing.T, caller *larkAlignmentCaller, args ...string) (map[string]any, error) {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs(append([]string{
		"chat", "+messages-list-direct", "--user", "fixture-user", "--time", "2026-09-01 00:00:00",
	}, args...))
	err := root.Execute()
	var payload map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode output: %v; command error: %v; output: %s", decodeErr, err, output.String())
	}
	return payload, err
}

func TestCrossPlatformCoverageDirectMessageIntegrityDecryptFailure(t *testing.T) {
	for _, tc := range []struct {
		name              string
		allPages          bool
		unknownPagination bool
	}{
		{name: "single_page_control"},
		{name: "all_pages_regression", allPages: true},
		{name: "decrypt_and_pagination_failure", allPages: true, unknownPagination: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			if tc.allPages {
				args = []string{"--page-all"}
			}
			testseam.Swap(t, &messageReadCryptoClient, &messagecrypto.Client{
				Identity: func(context.Context, string) (messagecrypto.Identity, error) {
					return messagecrypto.Identity{CorpID: "fixture-corp", StaffID: "fixture-user"}, nil
				},
				OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
					return &messagecrypto.Session{Cipher: messageReadFakeCipher{}, CorpID: "fixture-corp", StaffID: "fixture-user"}, nil
				},
				BackendReady: func() bool { return true },
				PolicyCache:  messagecrypto.NewPolicyCache(nil),
			})
			caller := &larkAlignmentCaller{
				responses: map[string]string{
					"chat/list_individual_chat_message": `{"result":{"messages":[{"openMessageId":"fixture-message","openConversationId":"fixture-conversation","content":"` + testCipher + `"}],"hasMore":false}}`,
				},
				failProductTool: "im/get_message_crypto_policy",
			}
			if tc.unknownPagination {
				caller.responses["chat/list_individual_chat_message"] = `{"result":{"messages":[{"openMessageId":"fixture-message","openConversationId":"fixture-conversation","content":"` + testCipher + `"}]}}`
			}
			payload, err := runDirectMessageIntegrityRegression(t, caller, args...)
			if tc.unknownPagination {
				if err == nil || payload["complete"] != false || payload["paginationKnown"] != false || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
					t.Fatalf("combined failure must retain pagination evidence: payload=%#v error=%v", payload, err)
				}
			} else if err != nil {
				t.Fatalf("existing decrypt fallback should return its ledger: %v", err)
			}
			if caller.callCounts["chat/list_individual_chat_message"] != 1 || caller.callCounts["im/get_message_crypto_policy"] != 1 {
				t.Fatalf("fault injection did not reach read and policy paths: %#v", caller.calls)
			}
			if payload["decryptFailedCount"] != float64(1) || payload["decryptedCount"] != float64(0) {
				t.Fatalf("expected one actual decrypt failure in ledger: %#v", payload)
			}
			messages := payload["messages"].([]any)
			if len(messages) != 1 || messages[0].(map[string]any)["text"] != testCipher {
				t.Fatalf("fallback must preserve original ciphertext: %#v", messages)
			}
			failures := payload["decryptFailures"].([]any)
			if len(failures) != 1 || failures[0].(map[string]any)["stage"] != "message-decrypt" {
				t.Fatalf("decryption failure details must be retained: %#v", failures)
			}
			if tc.allPages && !tc.unknownPagination && (payload["complete"] != true || payload["paginationKnown"] != true || payload["failedCount"] != float64(0)) {
				t.Fatalf("decryption failure must not alter pagination completion: %#v", payload)
			}
			if payload["partial"] != true {
				t.Fatalf("decryptFailedCount=1 requires partial=true; got partial=%v complete=%v stopReason=%v", payload["partial"], payload["complete"], payload["stopReason"])
			}
		})
	}
}

func TestCrossPlatformCoverageDirectMessageIntegrityUnknownPagination(t *testing.T) {
	for _, tc := range []struct {
		name      string
		responses []string
	}{
		{"first_page_missing_hasMore", []string{`{"result":{"messages":[{"openMessageId":"fixture-1","content":"plain"}]}}`}},
		{"later_page_missing_hasMore", []string{
			`{"result":{"messages":[{"openMessageId":"fixture-1","content":"plain"}],"hasMore":true,"nextCursor":1785919699136}}`,
			`{"result":{"messages":[{"openMessageId":"fixture-2","content":"plain"}]}}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{"chat/list_individual_chat_message": tc.responses}}
			payload, err := runDirectMessageIntegrityRegression(t, caller, "--page-all")
			if err == nil || payload["stopReason"] != "pagination_error" || payload["complete"] != false || payload["failedCount"] != float64(1) {
				t.Fatalf("unknown pagination must retain failure evidence: payload=%#v error=%v", payload, err)
			}
			if payload["count"] != float64(len(tc.responses)) || payload["partial"] != true {
				t.Fatalf("already read messages must be retained: %#v", payload)
			}
			if caller.callCounts["chat/list_individual_chat_message"] != len(tc.responses) {
				t.Fatalf("must stop at unknown pagination: %#v", caller.calls)
			}
			if payload["paginationKnown"] != false {
				t.Fatalf("missing hasMore requires paginationKnown=false; got paginationKnown=%v stopReason=%v pagesFetched=%v", payload["paginationKnown"], payload["stopReason"], payload["pagesFetched"])
			}
		})
	}
}

func TestCrossPlatformCoverageDirectMessageIntegrityCompleteControl(t *testing.T) {
	caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{"chat/list_individual_chat_message": {
		`{"result":{"messages":[{"openMessageId":"fixture-1","content":"plain"}],"hasMore":true,"nextCursor":1785919699136}}`,
		`{"result":{"messages":[{"openMessageId":"fixture-2","content":"plain"}],"hasMore":false}}`,
	}}}
	payload, err := runDirectMessageIntegrityRegression(t, caller, "--page-all")
	if err != nil || payload["count"] != float64(2) || payload["complete"] != true || payload["paginationKnown"] != true || payload["partial"] != false {
		t.Fatalf("normal completed pagination changed: payload=%#v error=%v", payload, err)
	}
}

func TestCrossPlatformCoverageDirectMessageIntegrityKnownPagination(t *testing.T) {
	const morePage = `{"result":{"messages":[{"openMessageId":"fixture-1","content":"plain"}],"hasMore":true,"nextCursor":1785919699136}}`
	for _, tc := range []struct {
		name       string
		response   string
		failAt     int
		stopReason string
		complete   bool
		wantError  bool
		calls      int
	}{
		{name: "explicit_complete", response: `{"result":{"messages":[{"openMessageId":"fixture-1","content":"plain"}],"hasMore":false}}`, stopReason: "source_complete", complete: true, calls: 1},
		{name: "later_read_failure", response: morePage, failAt: 2, stopReason: "read_failure", wantError: true, calls: 2},
		{name: "invalid_cursor", response: `{"result":{"messages":[{"openMessageId":"fixture-1","content":"plain"}],"hasMore":true}}`, stopReason: "pagination_error", wantError: true, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{
				responses:         map[string]string{"chat/list_individual_chat_message": tc.response},
				failProductToolAt: map[string]int{"chat/list_individual_chat_message": tc.failAt},
			}
			payload, err := runDirectMessageIntegrityRegression(t, caller, "--page-all")
			if (err != nil) != tc.wantError || payload["complete"] != tc.complete || payload["paginationKnown"] != true || payload["stopReason"] != tc.stopReason || payload["partial"] != tc.wantError {
				t.Fatalf("known pagination evidence changed: payload=%#v error=%v", payload, err)
			}
			if payload["count"] != float64(1) || caller.callCounts["chat/list_individual_chat_message"] != tc.calls {
				t.Fatalf("unexpected retained messages or requests: payload=%#v calls=%#v", payload, caller.calls)
			}
		})
	}
}
