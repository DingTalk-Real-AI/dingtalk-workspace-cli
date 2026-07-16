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
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eventlock "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
)

type personalRoundTripFunc func(*http.Request) (*http.Response, error)

func (f personalRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type personalErrorReadCloser struct{}

func (personalErrorReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (personalErrorReadCloser) Close() error             { return nil }

func personalHTTPClient(status int, body string) *http.Client {
	return &http.Client{Transport: personalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
}

func TestCrossPlatformCoveragePersonalClientEdgeCases(t *testing.T) {
	if got := (Identity{}).Key(); !strings.HasPrefix(got, "unknown\x00") {
		t.Fatalf("unknown identity key = %q", got)
	}
	var nilAPIError *APIError
	for name, apiErr := range map[string]*APIError{
		"nil":      nilAPIError,
		"code_msg": {Code: "C", Message: "message"},
		"code":     {Code: "C"},
		"message":  {Message: "message"},
	} {
		if name != "nil" && apiErr.Error() == "" {
			t.Fatalf("%s APIError rendered empty", name)
		}
		if name == "nil" && apiErr.Error() != "" {
			t.Fatalf("nil APIError = %q", apiErr.Error())
		}
	}
	if got := NewClient("", Identity{}); got.BaseURL == "" || got.HTTPClient == nil {
		t.Fatalf("NewClient fallback = %#v", got)
	}

	var nilClient *Client
	if err := nilClient.do(t.Context(), http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("nil client did not fail")
	}
	c := &Client{BaseURL: "http://example.test", Identity: Identity{}}
	if err := c.do(t.Context(), http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("missing access token did not fail")
	}
	c.Identity.AccessToken = "token"
	if err := c.do(t.Context(), http.MethodPost, "/x", nil, make(chan int), nil); err == nil {
		t.Fatal("unencodable body did not fail")
	}
	c.BaseURL = ":"
	if err := c.do(t.Context(), http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("invalid URL did not fail")
	}
	c.BaseURL = "http://example.test"
	c.HTTPClient = &http.Client{Transport: personalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("send failed")
	})}
	if err := c.do(t.Context(), http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("transport error did not propagate")
	}
	c.HTTPClient = &http.Client{Transport: personalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: personalErrorReadCloser{}, Header: make(http.Header)}, nil
	})}
	if err := c.do(t.Context(), http.MethodGet, "/x", nil, nil, nil); err == nil {
		t.Fatal("response read error did not propagate")
	}

	oldDefaultClient := http.DefaultClient
	http.DefaultClient = personalHTTPClient(http.StatusOK, "")
	t.Cleanup(func() { http.DefaultClient = oldDefaultClient })
	c.HTTPClient = nil
	if err := c.do(t.Context(), http.MethodGet, "/x", nil, nil, nil); err != nil {
		t.Fatalf("default HTTP client: %v", err)
	}

	cases := []struct {
		name    string
		status  int
		body    string
		out     any
		wantErr bool
	}{
		{"http envelope error", 400, `{"error":{"code":"BAD","message":"bad"},"requestId":"r1"}`, nil, true},
		{"http legacy error", 400, `{"errorCode":"OLD","errorMsg":"bad"}`, nil, true},
		{"http direct error", 400, `{"code":"DIRECT","message":"bad"}`, nil, true},
		{"http plain error", 500, `plain`, nil, true},
		{"empty", 204, ``, nil, false},
		{"implicit envelope error", 200, `{"error":{"code":"BAD","message":"bad"}}`, nil, true},
		{"false envelope error", 200, `{"success":false,"errorCode":"BAD","errorMsg":"bad"}`, nil, true},
		{"false envelope", 200, `{"success":false}`, nil, true},
		{"nil result", 200, `{"success":true}`, &Subscription{}, false},
		{"ignored result", 200, `{"success":true,"result":{"subId":"s"}}`, nil, false},
		{"decoded result", 200, `{"success":true,"result":{"subId":"s"}}`, &Subscription{}, false},
		{"bad result", 200, `{"success":true,"result":"bad"}`, &dwsSubListResult{}, true},
		{"ignored plain", 200, `{"plain":true}`, nil, false},
		{"decoded plain", 200, `{"total":2}`, &dwsSubListResult{}, false},
		{"bad plain", 200, `{`, &dwsSubListResult{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.HTTPClient = personalHTTPClient(tc.status, tc.body)
			err := c.do(t.Context(), http.MethodPost, "/x", url.Values{"token": {"secret"}}, map[string]string{"name": "x"}, tc.out)
			if (err != nil) != tc.wantErr {
				t.Fatalf("do() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoveragePersonalClientHelpersAndOperations(t *testing.T) {
	trueValue := true
	if (responseEnvelope{}).apiError() != nil {
		t.Fatal("empty response envelope returned an error")
	}
	if got := (responseEnvelope{ErrorCode: "E"}).apiError(); got == nil || got.Code != "E" {
		t.Fatalf("legacy envelope error = %#v", got)
	}
	if got := (responseEnvelope{Success: &trueValue, RequestID2: "r2"}).requestID(); got != "r2" {
		t.Fatalf("request id = %q", got)
	}
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`)} {
		if err := decodeResult(raw, &Subscription{}); err != nil {
			t.Fatalf("decodeResult(%s): %v", raw, err)
		}
	}
	var sub Subscription
	if err := decodeResult(json.RawMessage(`[]`), &sub); err != nil {
		t.Fatal(err)
	}
	if err := decodeResult(json.RawMessage(`{"subscribe_id":"s","event_key":"e"}`), &sub); err != nil || sub.SubscribeID != "s" {
		t.Fatalf("object subscription = %#v, %v", sub, err)
	}
	if err := decodeResult(json.RawMessage(`{"value":1}`), &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := decodeResult(json.RawMessage(`{`), &map[string]any{}); err == nil {
		t.Fatal("invalid result decoded")
	}
	if got, ok := decodeSubscriptionResult(json.RawMessage(`{"other":true}`)); ok || got.SubscribeID != "" {
		t.Fatalf("unexpected subscription result = %#v, %v", got, ok)
	}

	for _, raw := range [][]byte{
		[]byte(`{"error":{"code":"E"}}`),
		[]byte(`{"errorCode":"E2"}`),
		[]byte(`{"code":"E3"}`),
	} {
		if decodeAPIError(raw) == nil {
			t.Fatalf("decodeAPIError(%s) = nil", raw)
		}
	}
	if decodeAPIError([]byte(`{}`)) != nil || decodeAPIError([]byte(`{`)) != nil {
		t.Fatal("non-error payload decoded as API error")
	}
	if responseRequestID([]byte(`{"request_id":"r"}`)) != "r" || responseRequestID([]byte(`{`)) != "" {
		t.Fatal("response request ID decoding failed")
	}
	if withRequestDetails(nil, "GET", "/", 200, "") != nil {
		t.Fatal("nil API error was changed")
	}
	detailed := withRequestDetails(&APIError{Details: map[string]any{}}, "GET", "/", 400, "")
	if detailed.Details["http_status"] != 400 {
		t.Fatalf("details = %#v", detailed.Details)
	}
	if sanitizeLogPayload(nil) != "" || sanitizeLogPayload([]byte(`not-json`)) != "not-json" {
		t.Fatal("payload sanitization failed")
	}
	if _, err := marshalLogJSON(make(chan int)); err == nil {
		t.Fatal("marshalLogJSON accepted channel")
	}
	if got := redactedQueryString(url.Values{"authorization": {"secret"}, "q": {"ok"}}); !strings.Contains(got, "%3Credacted%3E") {
		t.Fatalf("redacted query = %q", got)
	}
	for raw, want := range map[string]string{"": "", "null": "", `"paused"`: "paused", "1": "active", "2": "paused", "3": "deleted", "4": "4", "true": "true"} {
		if got := dwsStatusString(json.RawMessage(raw)); got != want {
			t.Fatalf("status %q = %q, want %q", raw, got, want)
		}
	}

	base := &Client{BaseURL: "http://example.test", Identity: Identity{AccessToken: "token", ClientID: " c ", SourceID: " s "}}
	if _, err := base.CreateSubscription(t.Context(), CreateSubscriptionRequest{}); err == nil {
		t.Fatal("invalid create request accepted")
	}
	base.HTTPClient = personalHTTPClient(400, `{"error":{"code":"DUP","details":{"subscribe_id":"existing"}}}`)
	created, err := base.CreateSubscription(t.Context(), CreateSubscriptionRequest{EventKey: "e", RuleType: "r"})
	if err != nil || created.SubscribeID != "existing" {
		t.Fatalf("duplicate create = %#v, %v", created, err)
	}
	request := base.buildCreateRequest(CreateSubscriptionRequest{
		EventKey: "e", RuleType: "r", Name: "n", RuleParam: map[string]any{"bad": make(chan int)},
		Filter: true, IdempotencyKey: "i", TTLSeconds: 1,
	})
	if request.ExpiresAt == "" || request.Ext["name"] != "n" || request.FilterRule != "" {
		t.Fatalf("create request = %#v", request)
	}
	if _, err := base.GetSubscription(t.Context(), " "); err == nil {
		t.Fatal("empty get accepted")
	}
	base.HTTPClient = personalHTTPClient(200, `{"success":true,"result":{"items":[]}}`)
	if _, err := base.GetSubscription(t.Context(), "missing"); err == nil {
		t.Fatal("missing subscription did not fail")
	}
	base.HTTPClient = &http.Client{Transport: personalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("list failed")
	})}
	if _, err := base.GetSubscription(t.Context(), "id"); err == nil {
		t.Fatal("list error did not propagate through get")
	}
	if err := base.DeleteSubscription(t.Context(), " "); err == nil {
		t.Fatal("empty delete accepted")
	}
}

func TestCrossPlatformCoveragePersonalPaginationFiltersAndGuard(t *testing.T) {
	client := &Client{BaseURL: "http://example.test", Identity: Identity{AccessToken: "token"}}
	items := `[{"subId":"id1","status":2,"eventKey":"target"},{"subId":"id2","status":1,"eventKey":"other"},{"subId":"idx","status":1,"eventKey":"target"},{"subId":"id3","status":1,"eventKey":"target"}]`
	client.HTTPClient = personalHTTPClient(200, `{"success":true,"result":{"items":`+items+`}}`)
	got, err := client.ListSubscriptions(t.Context(), ListOptions{Status: "active", EventKey: "target", SubscribeID: "id3"})
	if err != nil || len(got) != 1 || got[0].SubscribeID != "id3" {
		t.Fatalf("filtered list = %#v, %v", got, err)
	}

	oldMax := subscriptionListMaxPages
	subscriptionListMaxPages = 1
	t.Cleanup(func() { subscriptionListMaxPages = oldMax })
	client.HTTPClient = personalHTTPClient(200, `{"success":true,"result":{"total":2,"pageSize":1,"items":[{"eventKey":"target"}]}}`)
	if _, err := client.ListSubscriptions(t.Context(), ListOptions{}); err == nil || !strings.Contains(err.Error(), "exceeded 1 pages") {
		t.Fatalf("pagination guard error = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalRegistryEdgeCases(t *testing.T) {
	if got := (&SchemaPendingError{EventKey: "pending"}).Error(); !strings.Contains(got, "pending") {
		t.Fatalf("pending error = %q", got)
	}
	if got := PublicAvailabilityError("private"); got == nil || !strings.Contains(got.Error(), "private") {
		t.Fatalf("availability error = %v", got)
	}
	if _, _, err := BuildRuleParam("unknown", RuleOptions{}); err == nil {
		t.Fatal("unknown event accepted")
	}
	if _, _, err := BuildRuleParam(EventMention, RuleOptions{RuleType: "group"}); err == nil {
		t.Fatal("wrong rule accepted")
	}
	if _, _, err := BuildRuleParam(EventMention, RuleOptions{GroupID: "g"}); err == nil {
		t.Fatal("mention group accepted")
	}
	if _, _, err := BuildRuleParam(EventFromUser, RuleOptions{UserID: "u", GroupID: "g"}); err == nil {
		t.Fatal("sender group accepted")
	}
	if _, _, err := BuildFilter(`{`, ""); err == nil {
		t.Fatal("invalid filter accepted")
	}
	if value, canonical, err := BuildFilter("", ""); err != nil || value != nil || canonical != "" {
		t.Fatalf("empty filter = %#v, %q, %v", value, canonical, err)
	}
	if value, canonical, err := BuildFilter("", "one"); err != nil || value == nil || canonical == "" {
		t.Fatalf("query filter = %#v, %q, %v", value, canonical, err)
	}
	if value, canonical, err := BuildFilter(`{"field":"content"}`, ""); err != nil || value == nil || canonical == "" {
		t.Fatalf("JSON filter = %#v, %q, %v", value, canonical, err)
	}
	if got, err := CanonicalJSON(nil); err != nil || got != "" {
		t.Fatalf("CanonicalJSON(nil) = %q, %v", got, err)
	}
	if _, err := CanonicalJSON(make(chan int)); err == nil {
		t.Fatal("CanonicalJSON accepted channel")
	}
	normalized := normalizeFilterAliases(map[string]any{
		"field":  12,
		"nested": []any{map[string]any{"field": "unmapped"}, "value"},
	}).(map[string]any)
	if normalized["field"] != 12 {
		t.Fatalf("normalized filter = %#v", normalized)
	}
	if IsSchemaPending(errors.New("other")) || !IsSchemaPending(&SchemaPendingError{EventKey: "x"}) {
		t.Fatal("schema pending detection failed")
	}

	original := append([]Definition(nil), definitions...)
	definitions = append(definitions,
		Definition{EventKey: "pending", Category: "other", RuleType: "at", Status: StatusPending, Public: true},
		Definition{EventKey: "unknown-rule", Category: "im", RuleType: "future", Status: StatusEnabled, Public: true},
	)
	t.Cleanup(func() { definitions = original })
	if got := Catalog("im", true, false); len(got) == 0 {
		t.Fatal("enabled catalog is empty")
	}
	if got := Catalog("other", true, true); len(got) != 0 {
		t.Fatalf("enabled pending catalog = %#v", got)
	}
	if got := Catalog("other", false, false); len(got) != 0 {
		t.Fatalf("excluded pending catalog = %#v", got)
	}
	if got := Catalog("other", false, true); len(got) != 1 {
		t.Fatalf("included pending catalog = %#v", got)
	}
	if _, _, err := BuildRuleParam("pending", RuleOptions{}); !IsSchemaPending(err) {
		t.Fatalf("pending rule error = %v", err)
	}
	if _, _, err := BuildRuleParam("unknown-rule", RuleOptions{}); !IsSchemaPending(err) {
		t.Fatalf("future rule error = %v", err)
	}
}

func TestCrossPlatformCoverageRunStateEdgeCases(t *testing.T) {
	workDir := t.TempDir()
	if err := UpsertRunState(workDir, RunState{}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRunStates(workDir, nil); err != nil {
		t.Fatal(err)
	}
	if err := UpsertRunState(workDir, RunState{SubscribeID: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertRunState(workDir, RunState{SubscribeID: "b", EventKey: "updated", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRunStates(workDir, []string{"", "missing"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, StateFileName), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunStates(workDir); err == nil {
		t.Fatal("invalid state JSON decoded")
	}
	if err := UpsertRunState(workDir, RunState{SubscribeID: "x"}); err == nil {
		t.Fatal("upsert ignored load error")
	}
	if err := RemoveRunStates(workDir, []string{"x"}); err == nil {
		t.Fatal("remove ignored load error")
	}
}

func TestCrossPlatformCoverageRunStateInjectedFailures(t *testing.T) {
	originalRead := runStateReadFile
	originalMkdir := runStateMkdirAll
	originalAcquire := runStateTryAcquire
	originalRemove := runStateRemove
	originalMarshal := runStateMarshalIndent
	originalWrite := runStateWriteFile
	originalRename := runStateRename
	originalSleep := runStateSleep
	t.Cleanup(func() {
		runStateReadFile = originalRead
		runStateMkdirAll = originalMkdir
		runStateTryAcquire = originalAcquire
		runStateRemove = originalRemove
		runStateMarshalIndent = originalMarshal
		runStateWriteFile = originalWrite
		runStateRename = originalRename
		runStateSleep = originalSleep
	})
	failure := errors.New("injected failure")
	workDir := t.TempDir()

	runStateReadFile = func(string) ([]byte, error) { return nil, failure }
	if _, err := LoadRunStates(workDir); !errors.Is(err, failure) {
		t.Fatalf("read error = %v", err)
	}
	runStateReadFile = originalRead
	runStateMkdirAll = func(string, os.FileMode) error { return failure }
	if err := withRunStateLock(workDir, time.Second, func() error { return nil }); !errors.Is(err, failure) {
		t.Fatalf("lock mkdir error = %v", err)
	}
	if err := writeRunStates(workDir, []RunState{{SubscribeID: "x"}}); !errors.Is(err, failure) {
		t.Fatalf("write mkdir error = %v", err)
	}
	runStateMkdirAll = originalMkdir
	runStateTryAcquire = func(string) (*eventlock.File, error) { return nil, failure }
	if err := withRunStateLock(workDir, time.Second, func() error { return nil }); !errors.Is(err, failure) {
		t.Fatalf("lock acquire error = %v", err)
	}
	runStateTryAcquire = originalAcquire

	runStateRemove = func(string) error { return failure }
	if err := writeRunStates(workDir, nil); !errors.Is(err, failure) {
		t.Fatalf("remove error = %v", err)
	}
	runStateRemove = originalRemove
	if err := writeRunStates(workDir, nil); err != nil {
		t.Fatalf("remove empty state file: %v", err)
	}
	runStateMarshalIndent = func(any, string, string) ([]byte, error) { return nil, failure }
	if err := writeRunStates(workDir, []RunState{{SubscribeID: "x"}}); !errors.Is(err, failure) {
		t.Fatalf("marshal error = %v", err)
	}
	runStateMarshalIndent = originalMarshal
	runStateWriteFile = func(string, []byte, os.FileMode) error { return failure }
	if err := writeRunStates(workDir, []RunState{{SubscribeID: "x"}}); !errors.Is(err, failure) {
		t.Fatalf("write error = %v", err)
	}
	runStateWriteFile = originalWrite
	runStateRename = func(string, string) error { return failure }
	if err := writeRunStates(workDir, []RunState{{SubscribeID: "x"}}); !errors.Is(err, failure) {
		t.Fatalf("rename error = %v", err)
	}
	runStateRename = originalRename

	attempts := 0
	runStateTryAcquire = func(path string) (*eventlock.File, error) {
		attempts++
		if attempts == 1 {
			return nil, eventlock.ErrBusy
		}
		return originalAcquire(path)
	}
	runStateSleep = func(time.Duration) {}
	if err := withRunStateLock(workDir, time.Millisecond, func() error { return nil }); err != nil {
		t.Fatalf("busy retry error = %v", err)
	}
}
