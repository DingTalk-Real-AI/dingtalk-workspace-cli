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

package registry

import (
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func mustJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestGenericProcessor_HeaderFieldsPromoted(t *testing.T) {
	ev := transport.Event{
		EventID:           "ev_1",
		EventBornTime:     1700000000123,
		EventCorpID:       "corp_x",
		EventType:         "approval.task",
		EventUnifiedAppID: "app_y",
		Data:              `{"task_id":"t1","status":"approved"}`,
	}
	cv := GenericProcessor(ev)
	out := mustJSON(t, cv)
	wants := map[string]any{
		"type":      "approval.task",
		"event_id":  "ev_1",
		"corp_id":   "corp_x",
		"app_id":    "app_y",
		"task_id":   "t1",
		"status":    "approved",
		"timestamp": float64(1700000000123),
	}
	for k, want := range wants {
		if got := out[k]; got != want {
			t.Errorf("compact[%q] = %v, want %v", k, got, want)
		}
	}
}

func TestGenericProcessor_PayloadParseFailEmbedsRaw(t *testing.T) {
	ev := transport.Event{
		EventType: "x",
		Data:      "not json",
	}
	out := mustJSON(t, GenericProcessor(ev))
	if out["data"] != "not json" {
		t.Fatalf("non-JSON Data should land under 'data', got: %v", out["data"])
	}
}

func TestGenericProcessor_EmptyDataNoExtraData(t *testing.T) {
	ev := transport.Event{EventType: "x", Data: ""}
	out := mustJSON(t, GenericProcessor(ev))
	if _, ok := out["data"]; ok {
		t.Errorf("empty Data should not produce a 'data' key; got: %v", out)
	}
}

func TestCompactView_HeaderFieldsWinOverExtra(t *testing.T) {
	// A malicious or buggy processor that tries to overwrite "type" must
	// be ignored — header fields are the documented contract.
	cv := CompactView{
		Type:  "the_real_type",
		Extra: map[string]any{"type": "fake"},
	}
	out := mustJSON(t, cv)
	if out["type"] != "the_real_type" {
		t.Fatalf("Extra 'type' must not override header; got %v", out["type"])
	}
}

func TestLookupProcessor_FallsBackToGeneric(t *testing.T) {
	p := LookupProcessor("never.registered.event")
	if p == nil {
		t.Fatal("LookupProcessor should never return nil")
	}
	cv := p(transport.Event{EventType: "never.registered.event", Data: "{}"})
	if cv.Type != "never.registered.event" {
		t.Fatalf("fallback processor type = %q", cv.Type)
	}
}

func TestCompactIMMessage_LiftsNestedFields(t *testing.T) {
	// Simulate the nested SDK shape: {"message": {...}, "sender": {...}}.
	ev := transport.Event{
		EventType: "im.message.receive_v1",
		EventID:   "ev_abc",
		Data: `{
			"message": {
				"message_id": "om_x",
				"chat_id": "oc_y",
				"chat_type": "p2p",
				"message_type": "text",
				"content": "{\"text\":\"hello\"}"
			},
			"sender": {
				"sender_id": {"open_id": "ou_z"},
				"sender_type": "user"
			}
		}`,
	}
	out := mustJSON(t, compactIMMessage(ev))
	if out["message_id"] != "om_x" {
		t.Errorf("message_id = %v", out["message_id"])
	}
	if out["chat_id"] != "oc_y" {
		t.Errorf("chat_id = %v", out["chat_id"])
	}
	if out["chat_type"] != "p2p" {
		t.Errorf("chat_type = %v", out["chat_type"])
	}
	if out["sender_id"] != "ou_z" {
		t.Errorf("sender_id = %v", out["sender_id"])
	}
	// Original nested fields should still be present too (generic merge).
	if _, ok := out["message"]; !ok {
		t.Error("nested 'message' should also be retained for downstream consumers")
	}
}

func TestCompactIMMessage_FlatPayloadAlsoWorks(t *testing.T) {
	// If the SDK ever flattens these to the top level, we should still
	// surface them — generic merge already does this.
	ev := transport.Event{
		EventType: "im.message.receive_v1",
		Data:      `{"message_id":"om_x","chat_id":"oc_y","content":"hi"}`,
	}
	out := mustJSON(t, compactIMMessage(ev))
	if out["message_id"] != "om_x" || out["chat_id"] != "oc_y" || out["content"] != "hi" {
		t.Fatalf("flat fields lost: %+v", out)
	}
}

func TestCatchAllEventTypes_EmptyForV1(t *testing.T) {
	if got := CatchAllEventTypes(); got != nil {
		t.Fatalf("CatchAllEventTypes v1 should be nil, got %v", got)
	}
}

func TestSpecialisedProcessorsRegistered(t *testing.T) {
	// Sentinel: every specialised event type should resolve to a non-
	// GenericProcessor function. If a P7 entry was accidentally dropped
	// from the registry map this catches it before runtime users do.
	wantRegistered := []string{
		"im.message.receive_v1",
		"im.message.read_v1",
		"approval.instance.status_changed",
		"approval.task.created",
		"contact.user.created_v3",
		"contact.user.updated_v3",
		"contact.user.deleted_v3",
		"cal.event.created_v1",
		"cal.event.updated_v1",
		"cal.event.deleted_v1",
		"attendance.check_v1",
	}
	for _, et := range wantRegistered {
		if _, ok := processors[et]; !ok {
			t.Errorf("event type %q should have a registered processor", et)
		}
	}
}

func TestCompactApprovalInstance(t *testing.T) {
	ev := transport.Event{
		EventType: "approval.instance.status_changed",
		Data: `{
			"instance": {
				"instance_id": "inst_123",
				"process_code": "proc_abc",
				"title": "请假申请"
			},
			"status_change": {
				"from_status": "PENDING",
				"to_status": "APPROVED"
			},
			"operator": {"userid": "user_xyz"}
		}`,
	}
	out := mustJSON(t, compactApprovalInstance(ev))
	for k, want := range map[string]any{
		"instance_id":     "inst_123",
		"process_code":    "proc_abc",
		"title":           "请假申请",
		"from_status":     "PENDING",
		"to_status":       "APPROVED",
		"operator_userid": "user_xyz",
	} {
		if out[k] != want {
			t.Errorf("approval[%q] = %v, want %v", k, out[k], want)
		}
	}
}

func TestCompactContactUser(t *testing.T) {
	ev := transport.Event{
		EventType: "contact.user.created_v3",
		Data:      `{"user":{"open_id":"ou_x","userid":"u_x","name":"Alice","active":true}}`,
	}
	out := mustJSON(t, compactContactUser(ev))
	if out["open_id"] != "ou_x" || out["userid"] != "u_x" || out["name"] != "Alice" {
		t.Errorf("contact user fields missing: %+v", out)
	}
}

func TestCompactCalendarEvent(t *testing.T) {
	ev := transport.Event{
		EventType: "cal.event.created_v1",
		Data:      `{"event":{"event_id":"e1","title":"Standup","start_time":"2026-01-01T09:00:00Z"},"organizer":{"userid":"u_x"}}`,
	}
	out := mustJSON(t, compactCalendarEvent(ev))
	if out["event_id"] != "e1" || out["title"] != "Standup" || out["organizer_userid"] != "u_x" {
		t.Errorf("calendar event fields missing: %+v", out)
	}
}
