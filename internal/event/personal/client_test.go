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
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientCreateSubscriptionDWSRequestAndArrayResponse(t *testing.T) {
	var gotPath string
	var gotReq dwsCreateSubscriptionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
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
		if got := r.Header.Get("X-DWS-Corp-Id"); got != "corp-1" {
			t.Fatalf("X-DWS-Corp-Id = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []string{"sub-1"},
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
		EventKey: EventSingleChat,
		RuleType: "singleChat",
		Name:     "test-o2o",
		RuleParam: map[string]any{
			"targetUid":     "507971",
			"targetUidType": "staffId",
		},
		Filter:         map[string]any{"field": "message.text", "op": "contains", "value": "P0"},
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if gotPath != "/subscription/user" {
		t.Fatalf("path = %q, want /subscription/user", gotPath)
	}
	if gotReq.ClientID != "client-1" || gotReq.SourceID != "open" || gotReq.EventKey != EventSingleChat {
		t.Fatalf("request identity/event = %#v", gotReq)
	}
	if gotReq.DeliveryPref != "realtime" {
		t.Fatalf("deliveryPref = %q, want realtime", gotReq.DeliveryPref)
	}
	var filterRule map[string]any
	if err := json.Unmarshal([]byte(gotReq.FilterRule), &filterRule); err != nil {
		t.Fatalf("filterRule is not JSON: %q: %v", gotReq.FilterRule, err)
	}
	if filterRule["targetUid"] != "507971" || filterRule["targetUidType"] != "staffId" {
		t.Fatalf("filterRule = %#v", filterRule)
	}
	if gotReq.Ext["ruleType"] != "singleChat" || gotReq.Ext["name"] != "test-o2o" || gotReq.Ext["idempotencyKey"] != "idem-1" {
		t.Fatalf("ext = %#v", gotReq.Ext)
	}
	if sub.SubscribeID != "sub-1" {
		t.Fatalf("subscribe_id = %q", sub.SubscribeID)
	}
	if sub.EventKey != EventSingleChat || sub.RuleType != "singleChat" || sub.Status != "active" || sub.SourceID != "open" {
		t.Fatalf("subscription = %#v", sub)
	}
}

func TestClientCreateSubscriptionObjectResponses(t *testing.T) {
	cases := []map[string]any{
		{"subId": "sub-camel", "eventKey": EventMention, "sourceId": "open", "status": 1},
		{"subscribe_id": "sub-snake", "event_key": EventMention, "source_id": "open", "status": "active"},
	}
	for _, result := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  result,
			})
		}))
		c := NewClient(srv.URL, Identity{AccessToken: "token-1", ClientID: "client-1", SourceID: "open"})
		sub, err := c.CreateSubscription(t.Context(), CreateSubscriptionRequest{
			EventKey:  EventMention,
			RuleType:  "at",
			RuleParam: map[string]any{},
		})
		srv.Close()
		if err != nil {
			t.Fatalf("CreateSubscription() error = %v", err)
		}
		if sub.SubscribeID == "" || !strings.HasPrefix(sub.SubscribeID, "sub-") {
			t.Fatalf("subscription = %#v", sub)
		}
	}
}

func TestClientDebugLogCreateSubscriptionRequestResponse(t *testing.T) {
	logs := captureClientDebugLogs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   true,
			"requestId": "req-ok",
			"result":    []string{"sub-1"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "secret-token", ClientID: "client-1", SourceID: "pre_open_source"})
	if _, err := c.CreateSubscription(t.Context(), CreateSubscriptionRequest{
		EventKey: EventSingleChat,
		RuleType: "singleChat",
		RuleParam: map[string]any{
			"targetUid":     "507971",
			"targetUidType": "staffId",
		},
		IdempotencyKey: "idem-1",
	}); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	out := logs.String()
	for _, want := range []string{
		"personal event control request",
		"/subscription/user",
		"client-1",
		"pre_open_source",
		EventSingleChat,
		"filterRule",
		"targetUid",
		"507971",
		"sub-1",
		"req-ok",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug log missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "secret-token") {
		t.Fatalf("debug log leaked access token: %s", out)
	}
}

func TestClientBusinessErrorHTTP200(t *testing.T) {
	logs := captureClientDebugLogs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   false,
			"requestId": "req-1",
			"errorCode": "INVALID_PARAM",
			"errorMsg":  "clientId is empty",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token-1", ClientID: "client-1", SourceID: "open"})
	_, err := c.CreateSubscription(t.Context(), CreateSubscriptionRequest{
		EventKey:  EventMention,
		RuleType:  "at",
		RuleParam: map[string]any{},
	})
	if err == nil || !strings.Contains(err.Error(), "INVALID_PARAM") || !strings.Contains(err.Error(), "clientId is empty") {
		t.Fatalf("error = %v, want INVALID_PARAM business error", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Details["method"] != http.MethodPost || apiErr.Details["path"] != "/subscription/user" ||
		apiErr.Details["http_status"] != http.StatusOK || apiErr.Details["request_id"] != "req-1" {
		t.Fatalf("details = %#v", apiErr.Details)
	}
	out := logs.String()
	for _, want := range []string{"/subscription/user", "INVALID_PARAM", "clientId is empty", "req-1", "request", "response"} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug log missing %q: %s", want, out)
		}
	}
}

func TestClientOmitsCorpHeaderWhenUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-DWS-Corp-Id"); got != "" {
			t.Fatalf("X-DWS-Corp-Id = %q, want empty", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []string{"sub-1"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token-1", ClientID: "client-1", SourceID: "open"})
	if _, err := c.CreateSubscription(t.Context(), CreateSubscriptionRequest{
		EventKey:  EventMention,
		RuleType:  "at",
		RuleParam: map[string]any{},
	}); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
}

func TestIdentityKeyUsesLocalSubjectFallback(t *testing.T) {
	withCorpUser := Identity{CorpID: "corp-1", UserID: "user-1", ClientID: "client-1", SourceID: "open"}
	if got := withCorpUser.Key(); got != "corp_user\x00corp-1\x00user-1\x00client-1\x00open" {
		t.Fatalf("corp/user key = %q", got)
	}
	fallback := Identity{LocalSubject: "refresh:abc", ClientID: "client-1", SourceID: "open"}
	if got := fallback.Key(); got != "local_subject\x00refresh:abc\x00client-1\x00open" {
		t.Fatalf("fallback key = %q", got)
	}
}

func TestClientDeleteSubscriptionTreatsNotFoundAsSuccess(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/subscription/cancel" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": "PERSONAL_EVENT_NOT_FOUND", "message": "not found"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	if err := c.DeleteSubscription(t.Context(), "sub-404"); err != nil {
		t.Fatalf("DeleteSubscription() error = %v", err)
	}
	if gotBody["subId"] != "sub-404" {
		t.Fatalf("cancel body = %#v", gotBody)
	}
}

func TestClientDeleteSubscriptionBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   false,
			"requestId": "req-cancel",
			"errorCode": "INVALID_STATE",
			"errorMsg":  "subscription cannot be cancelled",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token-1", ClientID: "client-1", SourceID: "open"})
	err := c.DeleteSubscription(t.Context(), "sub-1")
	if err == nil || !strings.Contains(err.Error(), "INVALID_STATE") {
		t.Fatalf("DeleteSubscription() error = %v, want INVALID_STATE", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Details["path"] != "/subscription/cancel" || apiErr.Details["request_id"] != "req-cancel" {
		t.Fatalf("details = %#v", apiErr.Details)
	}
}

func TestClientDebugLogListAndDeleteSubscription(t *testing.T) {
	logs := captureClientDebugLogs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/event/sublist":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  map[string]any{"items": []map[string]any{}},
			})
		case "/subscription/cancel":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	if _, err := c.ListSubscriptions(t.Context(), ListOptions{Status: "active"}); err != nil {
		t.Fatalf("ListSubscriptions() error = %v", err)
	}
	if err := c.DeleteSubscription(t.Context(), "sub-1"); err != nil {
		t.Fatalf("DeleteSubscription() error = %v", err)
	}
	out := logs.String()
	for _, want := range []string{"/event/sublist", "clientId=client", "sourceId=open", "/subscription/cancel", "sub-1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug log missing %q: %s", want, out)
		}
	}
}

func TestClientDebugLogRedactsSensitivePayloadFields(t *testing.T) {
	logs := captureClientDebugLogs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   true,
			"requestId": "req-secret",
			"result": map[string]any{
				"access_token":  "resp-access-token",
				"client_secret": "resp-client-secret",
				"ticket":        "resp-ticket",
				"Authorization": "Bearer resp-auth",
				"safe":          "ok",
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "header-token-secret", ClientID: "client", SourceID: "open"})
	err := c.do(t.Context(), http.MethodPost, "/subscription/user", nil, map[string]any{
		"access_token":  "req-access-token",
		"client_secret": "req-client-secret",
		"ticket":        "req-ticket",
		"Authorization": "Bearer req-auth",
		"safe":          "ok",
	}, nil)
	if err != nil {
		t.Fatalf("do() error = %v", err)
	}
	out := logs.String()
	for _, leaked := range []string{
		"header-token-secret",
		"req-access-token",
		"req-client-secret",
		"req-ticket",
		"Bearer req-auth",
		"resp-access-token",
		"resp-client-secret",
		"resp-ticket",
		"Bearer resp-auth",
	} {
		if strings.Contains(out, leaked) {
			t.Fatalf("debug log leaked %q: %s", leaked, out)
		}
	}
	for _, want := range []string{"<redacted>", "safe", "ok", "req-secret"} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug log missing %q: %s", want, out)
		}
	}
}

func TestClientListSubscriptionsDWSSublist(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/event/sublist" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"total":    2,
				"pageNo":   1,
				"pageSize": 20,
				"items": []map[string]any{
					{
						"subId":        "sub-1",
						"eventKey":     EventSingleChat,
						"sourceId":     "open",
						"deliveryPref": "realtime",
						"status":       1,
						"gmtCreate":    "2026-06-29T10:00:00Z",
					},
					{
						"subId":    "sub-2",
						"eventKey": EventMention,
						"sourceId": "open",
						"status":   3,
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	subs, err := c.ListSubscriptions(t.Context(), ListOptions{Status: "active", EventKey: EventSingleChat})
	if err != nil {
		t.Fatalf("ListSubscriptions() error = %v", err)
	}
	if !strings.Contains(gotQuery, "clientId=client") || !strings.Contains(gotQuery, "sourceId=open") ||
		!strings.Contains(gotQuery, "pageNo=1") || !strings.Contains(gotQuery, "pageSize=100") {
		t.Fatalf("query = %q", gotQuery)
	}
	if len(subs) != 1 || subs[0].SubscribeID != "sub-1" || subs[0].Status != "active" || subs[0].CreatedAt == "" {
		t.Fatalf("subs = %#v", subs)
	}
}

func TestClientGetSubscriptionFiltersSublist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"items": []map[string]any{
					{"subId": "sub-1", "eventKey": EventMention, "sourceId": "open", "status": 1},
					{"subId": "sub-2", "eventKey": EventSingleChat, "sourceId": "open", "status": 1},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	sub, err := c.GetSubscription(t.Context(), "sub-2")
	if err != nil {
		t.Fatalf("GetSubscription() error = %v", err)
	}
	if sub.SubscribeID != "sub-2" || sub.EventKey != EventSingleChat {
		t.Fatalf("subscription = %#v", sub)
	}
}

func captureClientDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &buf
}
