// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import "testing"

func TestCrossPlatformCoverageProjectMessageSendReceiptLinksStatusQuery(t *testing.T) {
	receipt := ProjectMessageSendReceipt(map[string]any{
		"result": map[string]any{"openTaskId": "task-1"},
	})
	if receipt["contractVersion"] != MessageSendReceiptContractVersion || receipt["openTaskId"] != "task-1" || receipt["readyForMessageActions"] != false {
		t.Fatalf("receipt = %#v", receipt)
	}
	actions, _ := receipt["nextActions"].([]map[string]any)
	if len(actions) != 1 || actions[0]["cliPath"] != "chat +messages-query-send-status" || actions[0]["ready"] != true {
		t.Fatalf("nextActions = %#v", actions)
	}
}

func TestCrossPlatformCoverageProjectMessageSendReceiptReadyWorkflow(t *testing.T) {
	receipt := ProjectMessageSendReceipt(map[string]any{
		"openTaskId":         "task-ready",
		"openMessageId":      "msg-ready",
		"openConversationId": "cid-ready",
	})
	if receipt["readyForMessageActions"] != true {
		t.Fatalf("receipt = %#v", receipt)
	}
	ref, _ := receipt["messageRef"].(map[string]any)
	if ref["openMessageId"] != "msg-ready" || ref["openConversationId"] != "cid-ready" {
		t.Fatalf("messageRef = %#v", ref)
	}
	actions, _ := receipt["nextActions"].([]map[string]any)
	if len(actions) != 3 || actions[0]["ready"] != true {
		t.Fatalf("nextActions = %#v", actions)
	}
}

func TestCrossPlatformCoverageProjectMessageSendStatusReadyWorkflow(t *testing.T) {
	raw := map[string]any{
		"result": map[string]any{
			"openTaskId":         "task-1",
			"openMessageId":      "msg-1",
			"openConversationId": "cid-1",
			"status":             "SUCCESS",
		},
	}
	payload := ProjectMessageSendStatus(raw, "ignored")
	if payload["contractVersion"] != MessageSendStatusContractVersion ||
		payload["openTaskId"] != "task-1" || payload["readyForMessageActions"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	ref, _ := payload["messageRef"].(map[string]any)
	if ref["openMessageId"] != "msg-1" || ref["openConversationId"] != "cid-1" {
		t.Fatalf("messageRef = %#v", ref)
	}
	actions, _ := payload["nextActions"].([]map[string]any)
	if len(actions) != 3 || actions[0]["cliPath"] != "chat message recall" || actions[2]["cliPath"] != "chat message read-status" {
		t.Fatalf("nextActions = %#v", actions)
	}
	if _, ok := payload["result"]; !ok {
		t.Fatal("raw response field was not preserved")
	}
}

func TestCrossPlatformCoverageProjectMessageSendStatusPendingDoesNotInventMessageRef(t *testing.T) {
	payload := ProjectMessageSendStatus(map[string]any{
		"result": map[string]any{"status": "PENDING"},
	}, "task-pending")
	if payload["openTaskId"] != "task-pending" || payload["readyForMessageActions"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	if _, exists := payload["messageRef"]; exists {
		t.Fatalf("pending payload invented messageRef: %#v", payload)
	}
	actions, _ := payload["nextActions"].([]map[string]any)
	if len(actions) != 1 || actions[0]["ready"] != false {
		t.Fatalf("nextActions = %#v", actions)
	}
}

func TestCrossPlatformCoverageFirstSendStatusStringTraversesArrays(t *testing.T) {
	value := []any{
		nil,
		map[string]any{"result": []any{
			map[string]any{"openTaskId": "  task-from-array  "},
		}},
	}
	if got := firstSendStatusString(value, "openTaskId"); got != "task-from-array" {
		t.Fatalf("firstSendStatusString() = %q", got)
	}
}
