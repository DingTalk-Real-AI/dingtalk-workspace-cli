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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// CompactView is the JSON-encoded shape `dws event consume --compact`
// writes per event. Implementations populate the common header fields
// then layer event-type-specific semantic fields on top via "extra".
type CompactView struct {
	// Type echoes EventType for routing/filtering at the agent layer.
	Type string `json:"type"`
	// EventID is the SDK-assigned identifier (DataFrameHeader "eventId").
	EventID string `json:"event_id,omitempty"`
	// Timestamp is the event-born-time milliseconds value, surfaced under a
	// shorter name agents typically expect.
	Timestamp int64 `json:"timestamp,omitempty"`
	// CorpID propagates the SDK's eventCorpId when present.
	CorpID string `json:"corp_id,omitempty"`
	// AppID propagates eventUnifiedAppId when present.
	AppID string `json:"app_id,omitempty"`
	// Extra holds the event-type-specific flattened fields (e.g.
	// chat_id/sender_id for IM messages). Marshalled inline.
	Extra map[string]any `json:"-"`
}

// MarshalJSON serialises CompactView with the Extra fields lifted to the
// top level, dropping the wrapping "extra" key. This produces the flat
// map output that agents prefer.
func (v CompactView) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, 6+len(v.Extra))
	out["type"] = v.Type
	if v.EventID != "" {
		out["event_id"] = v.EventID
	}
	if v.Timestamp != 0 {
		out["timestamp"] = v.Timestamp
	}
	if v.CorpID != "" {
		out["corp_id"] = v.CorpID
	}
	if v.AppID != "" {
		out["app_id"] = v.AppID
	}
	for k, val := range v.Extra {
		// Header fields win over Extra on collision — we never let a
		// processor accidentally overwrite the documented top-level shape.
		if _, taken := out[k]; taken {
			continue
		}
		out[k] = val
	}
	return json.Marshal(out)
}

// Processor transforms one Event frame into a CompactView. Implementations
// MUST NOT modify the input. Returning a zero CompactView is allowed and
// falls back to the generic processor at the caller's discretion.
type Processor func(ev transport.Event) CompactView

// processors holds the registered specialised processors. Lookup is by
// exact EventType match; wildcard support is intentionally absent because
// compact rendering is identity-specific (per event_type schema).
//
// To register more event types, add entries here. Each processor receives
// the full transport.Event and returns a CompactView with the per-type
// semantic fields lifted into Extra. See GenericProcessor for the
// fall-through behaviour applied to unregistered types.
var processors = map[string]Processor{
	// IM
	"im.message.receive_v1": compactIMMessage,
	"im.message.read_v1":    compactIMMessageRead,
	// Approval
	"approval.instance.status_changed": compactApprovalInstance,
	"approval.task.created":            compactApprovalTask,
	// Contact
	"contact.user.created_v3": compactContactUser,
	"contact.user.updated_v3": compactContactUser,
	"contact.user.deleted_v3": compactContactUser,
	// Calendar
	"cal.event.created_v1": compactCalendarEvent,
	"cal.event.updated_v1": compactCalendarEvent,
	"cal.event.deleted_v1": compactCalendarEvent,
	// Attendance
	"attendance.check_v1": compactAttendanceCheck,
}

// LookupProcessor returns the registered processor for eventType, or
// GenericProcessor when no specialised one exists.
func LookupProcessor(eventType string) Processor {
	if p, ok := processors[eventType]; ok {
		return p
	}
	return GenericProcessor
}

// GenericProcessor is the fallback used when no specialised processor is
// registered for the event type. It surfaces the 5 SDK header fields and
// tries to parse the JSON payload — on parse failure it embeds the raw
// payload string under "data" so nothing is lost.
func GenericProcessor(ev transport.Event) CompactView {
	v := CompactView{
		Type:      ev.EventType,
		EventID:   ev.EventID,
		Timestamp: ev.EventBornTime,
		CorpID:    ev.EventCorpID,
		AppID:     ev.EventUnifiedAppID,
		Extra:     map[string]any{},
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(ev.Data), &parsed); err == nil {
		for k, val := range parsed {
			v.Extra[k] = val
		}
	} else if ev.Data != "" {
		v.Extra["data"] = ev.Data
	}
	return v
}

// liftFromNested copies named keys from a nested map at top level on the
// CompactView. No-op when nested is missing. Used by the per-type
// processors to lift "sender.sender_id.open_id" style nested fields.
func liftFromNested(v *CompactView, nestedKey string, keys ...string) {
	nested, ok := v.Extra[nestedKey].(map[string]any)
	if !ok {
		return
	}
	for _, k := range keys {
		if val, ok := nested[k]; ok {
			if _, taken := v.Extra[k]; !taken {
				v.Extra[k] = val
			}
		}
	}
}

// compactIMMessage is the specialised processor for IM message receipts
// (event_type: im.message.receive_v1).
//
// The SDK payload for IM message events is a nested JSON document. The
// canonical agent fields we expose are: message_id, chat_id, chat_type,
// message_type, content (already a string for text messages), sender_id.
// Any field DingTalk adds later flows through via the generic merge —
// we never drop unknown fields, we just promote the common ones.
func compactIMMessage(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "message", "message_id", "chat_id", "chat_type", "message_type", "content")
	// Sender open_id may be doubly nested: sender.sender_id.open_id.
	if sender, ok := v.Extra["sender"].(map[string]any); ok {
		if sid, ok := sender["sender_id"].(map[string]any); ok {
			if open, ok := sid["open_id"]; ok {
				if _, taken := v.Extra["sender_id"]; !taken {
					v.Extra["sender_id"] = open
				}
			}
		}
	}
	return v
}

// compactIMMessageRead lifts message read receipts (open_id of the reader
// + the message_ids they acknowledged).
func compactIMMessageRead(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "reader", "open_id", "read_time")
	liftFromNested(&v, "context", "open_message_ids")
	return v
}

// compactApprovalInstance promotes the approval-instance-level identity
// fields (instance_id / status / business_id / process_code) and the
// requestor name.
func compactApprovalInstance(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "instance", "instance_id", "process_code", "business_id", "title")
	liftFromNested(&v, "status_change", "from_status", "to_status", "operate_time")
	if op, ok := v.Extra["operator"].(map[string]any); ok {
		if id, ok := op["userid"]; ok {
			v.Extra["operator_userid"] = id
		}
	}
	return v
}

// compactApprovalTask lifts task assignment info (task_id, assignee user_id).
func compactApprovalTask(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "task", "task_id", "instance_id", "status", "action_type")
	if assignee, ok := v.Extra["assignee"].(map[string]any); ok {
		if id, ok := assignee["userid"]; ok {
			v.Extra["assignee_userid"] = id
		}
	}
	return v
}

// compactContactUser lifts the user identity for user lifecycle events.
func compactContactUser(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "user", "open_id", "userid", "name", "active")
	if v.Extra["open_id"] == nil {
		// older payload may put open_id at top level under "open_ids" array
		if arr, ok := v.Extra["open_ids"].([]any); ok && len(arr) > 0 {
			v.Extra["open_id"] = arr[0]
		}
	}
	return v
}

// compactCalendarEvent lifts calendar event identity + organiser.
func compactCalendarEvent(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "event", "event_id", "calendar_id", "title", "start_time", "end_time", "location")
	if org, ok := v.Extra["organizer"].(map[string]any); ok {
		if id, ok := org["userid"]; ok {
			v.Extra["organizer_userid"] = id
		}
	}
	return v
}

// compactAttendanceCheck lifts the punch info (userid, check_time, location).
func compactAttendanceCheck(ev transport.Event) CompactView {
	v := GenericProcessor(ev)
	liftFromNested(&v, "punch", "userid", "check_time", "check_type", "location_method")
	return v
}
