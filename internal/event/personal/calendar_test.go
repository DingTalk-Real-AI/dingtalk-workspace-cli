// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package personal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestCrossPlatformCoverageCalendarRules(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts RuleOptions
		want string
	}{
		{"omitted dimensions", RuleOptions{}, `{}`},
		{"primary creation", RuleOptions{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventCreated"}}, `{"calendarIds":["primary"],"changeTypes":["eventCreated"]}`},
		{"primary update", RuleOptions{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventUpdated"}}, `{"calendarIds":["primary"],"changeTypes":["eventUpdated"]}`},
		{"primary cancellation", RuleOptions{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventCancelled"}}, `{"calendarIds":["primary"],"changeTypes":["eventCancelled"]}`},
		{"all changes", RuleOptions{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventUpdated", "eventCancelled", " eventCreated ", "eventUpdated"}}, `{"calendarIds":["primary"],"changeTypes":["eventCancelled","eventCreated","eventUpdated"]}`},
		{"changes only", RuleOptions{ChangeTypes: []string{"eventCancelled", "eventUpdated"}}, `{"changeTypes":["eventCancelled","eventUpdated"]}`},
		{"set semantics", RuleOptions{CalendarIDs: []string{"z", " primary ", "z", "a"}, ChangeTypes: []string{"eventCreated", "eventCreated"}}, `{"calendarIds":["a","primary","z"],"changeTypes":["eventCreated"]}`},
		{"empty calendar", RuleOptions{CalendarIDs: []string{}}, `{"calendarIds":[]}`},
		{"empty changes", RuleOptions{ChangeTypes: []string{}}, `{"changeTypes":[]}`},
		{"wildcards", RuleOptions{CalendarIDs: []string{"*", "*"}, ChangeTypes: []string{"*"}}, `{"calendarIds":["*"],"changeTypes":["*"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, params, err := BuildRuleParam(EventCalendarEventChanged, tc.opts)
			if err != nil || rule != "all" {
				t.Fatalf("rule = %q, error = %v", rule, err)
			}
			got, err := CanonicalJSON(params)
			if err != nil || got != tc.want {
				t.Fatalf("filterRule = %q, error = %v, want %s", got, err, tc.want)
			}
		})
	}
	for _, opts := range []RuleOptions{
		{UserID: "someone"}, {OpenDingTalkID: "someone"}, {GroupID: "group"}, {RoleTypes: []string{"creator"}},
		{RuleType: "group"}, {CalendarIDs: []string{"*", "primary"}},
		{ChangeTypes: []string{"*", "eventCreated"}}, {ChangeTypes: []string{"*", "eventUpdated"}},
		{ChangeTypes: []string{"eventCancelled", "*"}}, {ChangeTypes: []string{"eventDeleted"}},
		{ChangeTypes: []string{"eventUpdated", "unknown"}}, {ChangeTypes: []string{"eventCanceled"}},
		{CalendarIDs: []string{" "}}, {ChangeTypes: []string{""}},
	} {
		if _, _, err := BuildRuleParam(EventCalendarEventChanged, opts); err == nil {
			t.Fatalf("accepted invalid calendar options: %#v", opts)
		}
	}
	for _, key := range []string{EventMention, EventTodoTaskCreated, EventCardAction} {
		for _, opts := range []RuleOptions{{CalendarIDs: []string{}}, {ChangeTypes: []string{"eventCreated"}}} {
			if _, _, err := BuildRuleParam(key, opts); err == nil {
				t.Fatalf("%s accepted calendar filter %#v", key, opts)
			}
		}
	}
	_, first, _ := BuildRuleParam(EventCalendarEventChanged, RuleOptions{CalendarIDs: []string{"primary", "other"}})
	_, second, _ := BuildRuleParam(EventCalendarEventChanged, RuleOptions{CalendarIDs: []string{"other", "primary", "other"}})
	identity := Identity{CorpID: "corp", UserID: "self", ClientID: "client"}
	if IdempotencyKey(identity, EventCalendarEventChanged, "all", first, "") != IdempotencyKey(identity, EventCalendarEventChanged, "all", second, "") {
		t.Fatal("equivalent calendar sets produced different idempotency keys")
	}
	_, first, _ = BuildRuleParam(EventCalendarEventChanged, RuleOptions{ChangeTypes: []string{"eventUpdated", "eventCancelled", "eventUpdated"}})
	_, second, _ = BuildRuleParam(EventCalendarEventChanged, RuleOptions{ChangeTypes: []string{"eventCancelled", "eventUpdated"}})
	_, created, _ := BuildRuleParam(EventCalendarEventChanged, RuleOptions{ChangeTypes: []string{"eventCreated"}})
	key := IdempotencyKey(identity, EventCalendarEventChanged, "all", first, "")
	if key != IdempotencyKey(identity, EventCalendarEventChanged, "all", second, "") {
		t.Fatal("equivalent change type sets produced different idempotency keys")
	}
	if key == IdempotencyKey(identity, EventCalendarEventChanged, "all", created, "") {
		t.Fatal("creation-only subscription reused an update/cancellation idempotency key")
	}
}

func TestCrossPlatformCoverageCalendarSubscriptionWire(t *testing.T) {
	for _, opts := range []RuleOptions{
		{},
		{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventCreated"}},
		{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventUpdated"}},
		{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventCancelled"}},
		{CalendarIDs: []string{"primary"}, ChangeTypes: []string{"eventUpdated", "eventCreated", "eventCancelled"}},
		{CalendarIDs: []string{}, ChangeTypes: []string{}},
	} {
		t.Run("filter", func(t *testing.T) {
			rule, params, err := BuildRuleParam(EventCalendarEventChanged, opts)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := CanonicalJSON(params)
			cancelCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s", r.Method)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				switch r.URL.Path {
				case "/subscription/user":
					if body["eventKey"] != EventCalendarEventChanged || body["filterRule"] != want || body["sourceId"] != "example-agent" {
						t.Errorf("create body = %#v, want filterRule string %s", body, want)
					}
					_, _ = w.Write([]byte(`{"success":true,"result":["calendar-sub"]}`))
				case "/subscription/cancel":
					cancelCalls++
					if body["subId"] != "calendar-sub" {
						t.Errorf("cancel body = %#v", body)
					}
					_, _ = w.Write([]byte(`{"success":true}`))
				default:
					t.Errorf("unexpected path = %s", r.URL.Path)
				}
			}))
			defer server.Close()
			client := NewClient(server.URL, Identity{AccessToken: "test-token", ClientID: "test-client", SourceID: "example-agent"})
			sub, err := client.CreateSubscription(t.Context(), CreateSubscriptionRequest{EventKey: EventCalendarEventChanged, RuleType: rule, RuleParam: params})
			if err != nil || sub == nil || sub.SubscribeID != "calendar-sub" {
				t.Fatalf("subscription = %#v, error = %v", sub, err)
			}
			if err := client.DeleteSubscription(t.Context(), sub.SubscribeID); err != nil || cancelCalls != 1 {
				t.Fatalf("cancel calls = %d, error = %v", cancelCalls, err)
			}
		})
	}
}

func calendarTransport(payload string) transport.Event {
	data, _ := json.Marshal(map[string]any{
		"eventKey": EventCalendarEventChanged, "eventId": "transport-id", "subId": "inner-sub",
		"occurredAtMs": int64(1791763200001), "payload": json.RawMessage(payload),
	})
	return transport.Event{EventType: EventCalendarEventChanged, SubscribeID: "outer-sub", Data: string(data)}
}

func TestCrossPlatformCoverageCalendarOperatorUIDCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
		bad  bool
	}{
		{"string", `"9223372036854775807"`, "9223372036854775807", false},
		{"legacy integer", `9223372036854775807`, "9223372036854775807", false},
		{"zero", `0`, "0", false},
		{"absent", ``, "", false},
		{"null", `null`, "", false},
		{"empty string", `""`, "", false},
		{"fraction", `1.5`, "", true},
		{"overflow", `9223372036854775808`, "", true},
		{"boolean", `true`, "", true},
		{"object", `{}`, "", true},
		{"array", `[]`, "", true},
	} {
		for _, change := range []string{"eventCreated", "eventUpdated", "eventCancelled"} {
			t.Run(tc.name+"/"+change, func(t *testing.T) {
				operator := ""
				if tc.raw != "" {
					operator = `,"operatorUid":` + tc.raw
				}
				payload := `{"bizid":"delivery-id","event_time":1791763200000,"uid":9223372036854775807,"body":{"calendarId":"primary","eventId":"openapi-event-id","changeType":"` + change + `"` + operator + `}}`
				got, err := ProjectOutput(calendarTransport(payload))
				if tc.bad {
					if _, ok := got.(baseEventOutput); !ok || err == nil {
						t.Fatalf("invalid operator did not fall back safely: %#v, %v", got, err)
					}
					return
				}
				want := CalendarEventOutput{Type: EventCalendarEventChanged, EventID: "transport-id", Timestamp: 1791763200001, SubscribeID: "outer-sub", BizID: "delivery-id", EventTime: 1791763200000, ChangeType: change, CalendarID: "primary", CalendarEventID: "openapi-event-id", OperatorUID: tc.want}
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("operator compatibility lost business fields: %#v, %v; want %#v", got, err, want)
				}
				encoded, err := json.Marshal(got)
				if err != nil || strings.Contains(string(encoded), `"uid":`) || (tc.want == "" && strings.Contains(string(encoded), `"operator_uid":`)) {
					t.Fatalf("invalid flattened identifier output: %s, %v", encoded, err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageCalendarOutput(t *testing.T) {
	payload := `{"bizid":"delivery-id","event_time":1791763200000,"uid":9223372036854775807,"orgId":9223372036854775806,"filterSubId":"outer-sub","body":{"changeType":"eventCreated","calendarId":"primary","eventId":"openapi-event-id"}}`
	ev := calendarTransport(payload)
	projected, err := ProjectOutput(ev)
	if err != nil {
		t.Fatal(err)
	}
	want := CalendarEventOutput{Type: EventCalendarEventChanged, EventID: "transport-id", Timestamp: 1791763200001, SubscribeID: "outer-sub", BizID: "delivery-id", EventTime: 1791763200000, ChangeType: "eventCreated", CalendarID: "primary", CalendarEventID: "openapi-event-id"}
	if !reflect.DeepEqual(projected, want) {
		t.Fatalf("projected = %#v, want %#v", projected, want)
	}
	encoded, _ := json.Marshal(projected)
	for _, hidden := range []string{"operator_uid", "uid\"", "orgId", "filterSubId"} {
		if strings.Contains(string(encoded), hidden) {
			t.Fatalf("projection published absent/routing field %q: %s", hidden, encoded)
		}
	}
	transportOutput, err := ProjectTransportOutput(ev)
	if err != nil || !reflect.DeepEqual(transportOutput, ev) {
		t.Fatalf("default transport changed: %#v, %v", transportOutput, err)
	}
	withOperator := calendarTransport(strings.Replace(payload, `"eventId":"openapi-event-id"`, `"eventId":"openapi-event-id","operatorUid":"9223372036854775807"`, 1))
	projected, err = ProjectOutput(withOperator)
	if err != nil || projected.(CalendarEventOutput).OperatorUID != "9223372036854775807" {
		t.Fatalf("operator uid = %#v, error = %v", projected, err)
	}
	for _, changeType := range []string{"eventUpdated", "eventCancelled"} {
		t.Run(changeType, func(t *testing.T) {
			event := calendarTransport(strings.Replace(payload, "eventCreated", changeType, 1))
			got, err := ProjectOutput(event)
			expected := want
			expected.ChangeType = changeType
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Fatalf("projection = %#v, error = %v, want %#v", got, err, expected)
			}
			unchanged, err := ProjectTransportOutput(event)
			if err != nil || !reflect.DeepEqual(unchanged, event) {
				t.Fatalf("default transport changed: %#v, %v", unchanged, err)
			}
		})
	}
	// A later change type remains consumable without inventing an output enum.
	projected, err = ProjectOutput(calendarTransport(strings.Replace(payload, "eventCreated", "futureChange", 1)))
	if err != nil || projected.(CalendarEventOutput).ChangeType != "futureChange" {
		t.Fatalf("future change = %#v, error = %v", projected, err)
	}
	for _, bad := range []string{
		`null`, `{}`, `{"body":null}`, `{"body":{"calendarId":123}}`,
		strings.Replace(payload, `"bizid":"delivery-id"`, `"bizid":""`, 1),
		strings.Replace(payload, `"event_time":1791763200000,`, ``, 1),
		strings.Replace(payload, `"event_time":1791763200000`, `"event_time":null`, 1),
		strings.Replace(payload, `"changeType":"eventCreated"`, `"changeType":""`, 1),
		strings.Replace(payload, `"calendarId":"primary"`, `"calendarId":""`, 1),
		strings.Replace(payload, `"eventId":"openapi-event-id"`, `"eventId":""`, 1),
	} {
		got, err := ProjectOutput(calendarTransport(bad))
		if err == nil {
			t.Fatalf("accepted malformed calendar payload: %s", bad)
		}
		if _, ok := got.(baseEventOutput); !ok {
			t.Fatalf("malformed payload returned raw routing data: %T", got)
		}
	}
	got, err := ProjectOutput(transport.Event{EventType: EventCalendarEventChanged, Data: "not JSON"})
	if _, ok := got.(baseEventOutput); !ok || err == nil {
		t.Fatalf("invalid envelope = %#v, %v", got, err)
	}
	def, ok := Lookup(EventCalendarEventChanged)
	if !ok || len(Catalog("calendar", true, false)) != 1 || !def.Public {
		t.Fatalf("calendar definition = %#v", def)
	}
	flat := BuildSchemaDocumentForMode(def, true)
	properties := flat.Schema["properties"].(map[string]any)
	if flat.Category != "calendar" || flat.JQRootPath != "." || len(properties) != 10 {
		t.Fatalf("flatten schema = %#v", flat)
	}
	for _, name := range []string{"biz_id", "change_type", "calendar_id", "calendar_event_id", "operator_uid", "event_time"} {
		if properties[name] == nil {
			t.Fatalf("schema missing %s", name)
		}
	}
	if BuildSchemaDocument(def).JQRootPath != ".data | fromjson" {
		t.Fatal("default schema stopped describing the transport envelope")
	}
}
