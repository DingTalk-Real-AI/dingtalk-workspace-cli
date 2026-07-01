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

package personal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreateSubscriptionHeadersAndBody(t *testing.T) {
	var gotPath string
	var gotReq CreateSubscriptionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("x-user-access-token"); got != "token-1" {
			t.Fatalf("x-user-access-token = %q", got)
		}
		if got := r.Header.Get("X-DWS-Client-Id"); got != "client-1" {
			t.Fatalf("X-DWS-Client-Id = %q", got)
		}
		if got := r.Header.Get("X-DWS-Source-Id"); got != "open" {
			t.Fatalf("X-DWS-Source-Id = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"subscribe_id": "sub-1",
				"event_key":    gotReq.EventKey,
				"rule_type":    gotReq.RuleType,
				"status":       "active",
				"source_id":    "open",
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{
		AccessToken: "token-1",
		CorpID:      "corp-1",
		UserID:      "user-1",
		ClientID:    "client-1",
		SourceID:    "open",
	})
	sub, err := c.CreateSubscription(t.Context(), CreateSubscriptionRequest{
		EventKey:  EventMention,
		RuleType:  "at",
		RuleParam: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if gotPath != "/subscriptions" {
		t.Fatalf("path = %q, want /subscriptions", gotPath)
	}
	if gotReq.Delivery["mode"] != "stream" {
		t.Fatalf("delivery = %#v, want stream", gotReq.Delivery)
	}
	if sub.SubscribeID != "sub-1" {
		t.Fatalf("subscribe_id = %q", sub.SubscribeID)
	}
}

func TestClientDeleteSubscriptionTreatsNotFoundAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/subscriptions/sub-404" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "PERSONAL_EVENT_NOT_FOUND",
				"message": "not found",
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	if err := c.DeleteSubscription(t.Context(), "sub-404"); err != nil {
		t.Fatalf("DeleteSubscription() error = %v", err)
	}
}
