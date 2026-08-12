// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chatmsg

import "testing"

func TestCrossPlatformCoverageProjectStreamingCardReceipt(t *testing.T) {
	complete := ProjectStreamingCardReceipt(map[string]any{
		"result": map[string]any{
			"bizId":              "biz-1",
			"openMessageId":      "msg-1",
			"openConversationId": "cid-1",
		},
	}, "biz-1")
	if complete["contractVersion"] != StreamingCardContractVersion || complete["referencePairAvailable"] != true {
		t.Fatalf("complete receipt = %#v", complete)
	}
	ref, _ := complete["cardRef"].(map[string]any)
	if ref["bizId"] != "biz-1" || ref["openMessageId"] != "msg-1" || ref["openConversationId"] != "cid-1" {
		t.Fatalf("cardRef = %#v", ref)
	}
	if _, exists := complete["capabilityGap"]; exists {
		t.Fatalf("complete receipt has capability gap: %#v", complete)
	}

	partial := ProjectStreamingCardReceipt(map[string]any{"result": map[string]any{"bizId": "biz-2"}}, "biz-2")
	if partial["referencePairAvailable"] != false || partial["capabilityGap"] == "" {
		t.Fatalf("partial receipt = %#v", partial)
	}
}

func TestCrossPlatformCoverageProjectStreamingCardUpdate(t *testing.T) {
	payload := ProjectStreamingCardUpdate(map[string]any{"result": map[string]any{"updated": true}}, "biz-1", "updated=true")
	if payload["contractVersion"] != StreamingCardContractVersion || payload["verified"] != true || payload["verificationEvidence"] != "updated=true" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, exists := payload["result"]; !exists {
		t.Fatal("lower response was not preserved")
	}
}
