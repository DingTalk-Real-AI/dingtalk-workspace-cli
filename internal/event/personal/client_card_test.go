// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package personal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCardActionSubscriptionReusesIMCreateAndCancelEndpointsWithEmptyObjectFilterRule(t *testing.T) {
	var createBody map[string]any
	cancelCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/subscription/user":
			if r.Method != http.MethodPost {
				t.Fatalf("create method = %s, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []string{"card-sub"}})
		case "/subscription/cancel":
			cancelCalls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode cancel request: %v", err)
			}
			if body["subId"] != "card-sub" {
				t.Fatalf("cancel body = %#v, want card-sub", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	ruleType, ruleParam, err := BuildRuleParam(EventCardAction, RuleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(srv.URL, Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	sub, err := client.CreateSubscription(t.Context(), CreateSubscriptionRequest{
		EventKey:  EventCardAction,
		RuleType:  ruleType,
		RuleParam: ruleParam,
	})
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if createBody["eventKey"] != EventCardAction {
		t.Fatalf("eventKey = %#v, want %q", createBody["eventKey"], EventCardAction)
	}
	if createBody["filterRule"] != "{}" {
		t.Fatalf("filterRule = %#v, want empty JSON object", createBody["filterRule"])
	}
	ext, ok := createBody["ext"].(map[string]any)
	if !ok || ext["ruleType"] != "all" {
		t.Fatalf("ext = %#v, want ruleType all", createBody["ext"])
	}
	if sub.SubscribeID != "card-sub" {
		t.Fatalf("subscription = %#v", sub)
	}
	if err := client.DeleteSubscription(t.Context(), sub.SubscribeID); err != nil {
		t.Fatalf("DeleteSubscription() error = %v", err)
	}
	if cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancelCalls)
	}
}
