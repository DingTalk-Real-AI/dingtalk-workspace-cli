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

package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

// Each frame type is round-tripped through encoding/json then PeekType
// to guard against accidental wire-format changes (the JSON field tags
// double as part of the cross-version protocol contract).

func TestFrameType_StableWireValues(t *testing.T) {
	// If any of these strings change we've made a protocol-breaking
	// change. The test value list is duplicated here on purpose so a
	// reviewer renaming a constant is forced to also update the test.
	wants := map[FrameType]string{
		FrameTypeHello:       "hello",
		FrameTypeHelloAck:    "hello_ack",
		FrameTypeEvent:       "event",
		FrameTypeHeartbeat:   "heartbeat",
		FrameTypeSourceState: "source_state",
		FrameTypeBye:         "bye",
		FrameTypeStatusReq:   "status_req",
		FrameTypeStatusResp:  "status_resp",
	}
	for ft, want := range wants {
		if string(ft) != want {
			t.Errorf("FrameType wire value drift: %v != %q", ft, want)
		}
	}
}

func TestHelloRole_StableWireValues(t *testing.T) {
	wants := map[HelloRole]string{
		HelloRoleConsumer: "",
		HelloRoleStatus:   "status",
		HelloRoleStop:     "stop",
	}
	for r, want := range wants {
		if string(r) != want {
			t.Errorf("HelloRole wire value drift: %v != %q", r, want)
		}
	}
}

func roundTrip(t *testing.T, in any, dst any) {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal %T: %v", in, err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("unmarshal %T: %v", in, err)
	}
}

func TestHello_Roundtrip(t *testing.T) {
	in := Hello{
		Type:        FrameTypeHello,
		ConsumerPID: 42,
		EventTypes:  []string{"im.*", "approval.task"},
		Filter:      `^im\.`,
		Compact:     true,
		Role:        HelloRoleStatus,
	}
	var out Hello
	roundTrip(t, in, &out)
	if out.Type != in.Type || out.ConsumerPID != in.ConsumerPID || out.Filter != in.Filter ||
		out.Compact != in.Compact || out.Role != in.Role || len(out.EventTypes) != len(in.EventTypes) {
		t.Fatalf("roundtrip mismatch: %+v != %+v", out, in)
	}
}

func TestHello_OmitemptyForDefaults(t *testing.T) {
	// Default values should NOT appear in the wire form so old/new readers
	// stay tolerant of each other (each new field comes in with its
	// zero value by default).
	in := Hello{Type: FrameTypeHello, ConsumerPID: 1}
	b, _ := json.Marshal(in)
	s := string(b)
	for _, k := range []string{`"event_types"`, `"filter"`, `"compact"`, `"role"`} {
		if strings.Contains(s, k) {
			t.Errorf("zero-value field %s leaked into wire form: %s", k, s)
		}
	}
}

func TestHelloAck_Roundtrip(t *testing.T) {
	in := HelloAck{
		Type:               FrameTypeHelloAck,
		BusPID:             12345,
		SourceState:        "connected",
		StateSource:        "inferred",
		ClientIDSource:     "env",
		ClientSecretSource: "env",
		IdleTimeoutSecs:    300,
	}
	var out HelloAck
	roundTrip(t, in, &out)
	if out != in {
		t.Fatalf("HelloAck roundtrip: %+v != %+v", out, in)
	}
}

func TestEvent_Roundtrip(t *testing.T) {
	in := Event{
		Type:              FrameTypeEvent,
		Seq:               7,
		EventID:           "ev_x",
		EventBornTime:     1700000000123,
		EventCorpID:       "corp_x",
		EventType:         "im.message.receive_v1",
		EventUnifiedAppID: "app_y",
		Data:              `{"text":"hi"}`,
		Headers:           map[string]string{"extra": "v"},
		ReceivedAtUnixMS:  1700000000999,
	}
	var out Event
	roundTrip(t, in, &out)
	if out.EventID != in.EventID || out.Seq != in.Seq || out.Data != in.Data ||
		out.Headers["extra"] != "v" {
		t.Fatalf("Event roundtrip: %+v != %+v", out, in)
	}
}

func TestEvent_OmitsZeroOptionalHeader(t *testing.T) {
	in := Event{Type: FrameTypeEvent, EventID: "ev_1", EventType: "x", Data: "{}"}
	b, _ := json.Marshal(in)
	if strings.Contains(string(b), `"headers"`) {
		t.Errorf("nil Headers should not appear: %s", b)
	}
}

func TestHeartbeat_Roundtrip(t *testing.T) {
	in := Heartbeat{Type: FrameTypeHeartbeat}
	var out Heartbeat
	roundTrip(t, in, &out)
	if out.Type != FrameTypeHeartbeat {
		t.Fatalf("Heartbeat type lost: %v", out)
	}
}

func TestSourceState_Roundtrip(t *testing.T) {
	in := SourceState{
		Type:        FrameTypeSourceState,
		State:       "reconnecting",
		StateSource: "inferred",
		Attempt:     3,
	}
	var out SourceState
	roundTrip(t, in, &out)
	if out != in {
		t.Fatalf("SourceState mismatch: %+v != %+v", out, in)
	}
}

func TestBye_Roundtrip(t *testing.T) {
	for _, reason := range []string{"client_done", "shutdown", "idle_timeout", "stop_request"} {
		in := Bye{Type: FrameTypeBye, Reason: reason}
		var out Bye
		roundTrip(t, in, &out)
		if out.Reason != reason {
			t.Errorf("Bye reason %q lost: %+v", reason, out)
		}
	}
}

func TestStatusResp_Roundtrip(t *testing.T) {
	in := StatusResp{
		Type: FrameTypeStatusResp,
		Bus: StatusBus{
			PID: 99, UptimeSecs: 600, IdleTimeoutSec: 300,
			ClientID: "ding_x", Edition: "open",
		},
		SourceState: StatusSource{
			State: "connected", Source: "inferred", ReconnectCount: 1,
			LastEventAtMS: 1700000000000, LastReconnectMS: 1700000001000,
		},
		Consumers: []StatusConsumer{
			{PID: 12350, EventTypes: []string{"im.*"}, Filter: `^im\.`,
				SubscribedAtMS: 1700000000000, Received: 100, Dropped: 2},
		},
		PerEventTypeCounters: map[string]Counters{
			"im.message.receive_v1": {Received: 100, Dropped: 2},
		},
	}
	var out StatusResp
	roundTrip(t, in, &out)
	if out.Bus.ClientID != in.Bus.ClientID || len(out.Consumers) != 1 ||
		out.PerEventTypeCounters["im.message.receive_v1"].Received != 100 {
		t.Fatalf("StatusResp roundtrip mismatch: %+v", out)
	}
}

// PeekType is used by the daemon to dispatch incoming frames before
// fully decoding into the typed struct. Behaviour at boundaries matters.

func TestPeekType_EachFrameVariant(t *testing.T) {
	cases := []struct {
		v   any
		typ FrameType
	}{
		{Hello{Type: FrameTypeHello}, FrameTypeHello},
		{HelloAck{Type: FrameTypeHelloAck}, FrameTypeHelloAck},
		{Event{Type: FrameTypeEvent, EventID: "x"}, FrameTypeEvent},
		{Heartbeat{Type: FrameTypeHeartbeat}, FrameTypeHeartbeat},
		{SourceState{Type: FrameTypeSourceState, State: "connected"}, FrameTypeSourceState},
		{Bye{Type: FrameTypeBye, Reason: "x"}, FrameTypeBye},
		{StatusReq{Type: FrameTypeStatusReq}, FrameTypeStatusReq},
		{StatusResp{Type: FrameTypeStatusResp}, FrameTypeStatusResp},
	}
	for _, c := range cases {
		b, _ := json.Marshal(c.v)
		got, err := PeekType(b)
		if err != nil {
			t.Errorf("PeekType(%T): %v", c.v, err)
			continue
		}
		if got != c.typ {
			t.Errorf("PeekType(%T) = %q, want %q", c.v, got, c.typ)
		}
	}
}
