// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import "testing"

func TestCrossPlatformCoverageProjectStreamingCardReceipt(t *testing.T) {
	complete := ProjectStreamingCardReceipt(map[string]any{
		"result": map[string]any{
			"bizId":              "biz-1",
			"openTaskId":         "task-1",
			"openMessageId":      "msg-1",
			"openConversationId": "cid-1",
		},
	}, "biz-1")
	if complete["contractVersion"] != StreamingCardContractVersion || complete["referencePairAvailable"] != true ||
		complete["bizId"] != "biz-1" || complete["openTaskId"] != "task-1" {
		t.Fatalf("complete receipt = %#v", complete)
	}
	ref, _ := complete["cardRef"].(map[string]any)
	if ref["bizId"] != "biz-1" || ref["openMessageId"] != "msg-1" || ref["openConversationId"] != "cid-1" {
		t.Fatalf("cardRef = %#v", ref)
	}
	if _, exists := complete["capabilityGap"]; exists {
		t.Fatalf("complete receipt has capability gap: %#v", complete)
	}

	partial := ProjectStreamingCardReceipt(map[string]any{
		"result": map[string]any{"bizId": "biz-2", "openTaskId": "task-2"},
	}, "biz-2")
	if partial["referencePairAvailable"] != false || partial["openTaskId"] != "task-2" {
		t.Fatalf("partial receipt = %#v", partial)
	}
	if _, exists := partial["capabilityGap"]; exists {
		t.Fatalf("queryable partial receipt has capability gap: %#v", partial)
	}
	actions, _ := partial["nextActions"].([]map[string]any)
	if len(actions) != 2 || actions[1]["cliPath"] != "chat message query-send-status" {
		t.Fatalf("partial nextActions = %#v", actions)
	}

	missingTask := ProjectStreamingCardReceipt(map[string]any{"result": map[string]any{"bizId": "biz-3"}}, "biz-3")
	if missingTask["capabilityGap"] == "" {
		t.Fatalf("missing-task receipt = %#v", missingTask)
	}
}

func TestCrossPlatformCoverageProjectStreamingCardUpdate(t *testing.T) {
	payload := ProjectStreamingCardUpdate(map[string]any{"result": map[string]any{"updated": true}}, "biz-1", CardUpdateVerification{Accepted: true, Verified: true, Evidence: "updated=true"})
	if payload["contractVersion"] != StreamingCardContractVersion || payload["verified"] != true || payload["verificationEvidence"] != "updated=true" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, exists := payload["result"]; !exists {
		t.Fatal("lower response was not preserved")
	}
}

func TestCrossPlatformCoverageProjectAcceptedUnverifiedStreamingCardUpdate(t *testing.T) {
	payload := ProjectStreamingCardUpdate(map[string]any{"success": true}, "biz-1", CardUpdateVerification{Accepted: true, Verified: false, Evidence: "success=true"})
	if payload["accepted"] != true || payload["verified"] != false || payload["warning"] == "" {
		t.Fatalf("payload = %#v", payload)
	}
}
