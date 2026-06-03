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

package bus

import (
	"errors"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func mkEvent(typ, id string) *dwsevent.RawEvent {
	return &dwsevent.RawEvent{
		EventID:    id,
		EventType:  typ,
		Data:       `{}`,
		ReceivedAt: time.Now().UTC(),
	}
}

func drain(c *Consumer, n int, t *testing.T) []*transport.Event {
	t.Helper()
	out := make([]*transport.Event, 0, n)
	for i := 0; i < n; i++ {
		select {
		case f := <-c.SendCh:
			ev, ok := f.(transport.Event)
			if !ok {
				t.Fatalf("frame %d is not Event: %T", i, f)
			}
			out = append(out, &ev)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
	return out
}

func TestHub_RegisterAssignsMonotonicID(t *testing.T) {
	h := NewHub(10)
	c1, err := h.Register(transport.Hello{ConsumerPID: 1})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := h.Register(transport.Hello{ConsumerPID: 2})
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID >= c2.ID {
		t.Fatalf("IDs not monotonic: %d, %d", c1.ID, c2.ID)
	}
}

func TestHub_DeliverMatchesPrefix(t *testing.T) {
	h := NewHub(10)
	c, err := h.Register(transport.Hello{EventTypes: []string{"im.message.*"}})
	if err != nil {
		t.Fatal(err)
	}
	h.Deliver(mkEvent("im.message.receive_v1", "1"))
	h.Deliver(mkEvent("approval.task", "2")) // no match
	h.Deliver(mkEvent("im.message.at_v1", "3"))

	got := drain(c, 2, t)
	if got[0].EventID != "1" || got[1].EventID != "3" {
		t.Fatalf("expected 1,3 got %s,%s", got[0].EventID, got[1].EventID)
	}
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("seq mismatch: %d %d", got[0].Seq, got[1].Seq)
	}
}

func TestHub_DeliverCatchAll(t *testing.T) {
	h := NewHub(10)
	c, _ := h.Register(transport.Hello{}) // empty == catch-all
	h.Deliver(mkEvent("im.message.receive_v1", "1"))
	h.Deliver(mkEvent("approval.task", "2"))
	h.Deliver(mkEvent("foo.bar", "3"))
	got := drain(c, 3, t)
	if got[0].EventID != "1" || got[1].EventID != "2" || got[2].EventID != "3" {
		t.Fatalf("catch-all missed events: %+v", got)
	}
}

func TestHub_DeliverFilterRegex(t *testing.T) {
	h := NewHub(10)
	c, err := h.Register(transport.Hello{Filter: `^im\.`})
	if err != nil {
		t.Fatal(err)
	}
	h.Deliver(mkEvent("im.message.receive_v1", "1"))
	h.Deliver(mkEvent("approval.task", "2")) // filtered out
	h.Deliver(mkEvent("im.chat.member.bot.added_v1", "3"))
	got := drain(c, 2, t)
	if got[0].EventID != "1" || got[1].EventID != "3" {
		t.Fatalf("filter regex missed: %+v", got)
	}
}

func TestHub_RegisterRejectsBadFilterRegex(t *testing.T) {
	h := NewHub(10)
	_, err := h.Register(transport.Hello{Filter: `(unclosed`})
	if err == nil {
		t.Fatal("expected RegisterError for bad regex")
	}
	var re *RegisterError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v, want *RegisterError", err)
	}
}

func TestHub_DropOldestOnFullChannel(t *testing.T) {
	h := NewHub(2) // small buffer
	c, _ := h.Register(transport.Hello{})

	// Push 5 events without draining → 2 stay, 3 dropped
	for i := 0; i < 5; i++ {
		h.Deliver(mkEvent("foo", string(rune('0'+i))))
	}
	if c.received.Load() != 2 {
		t.Fatalf("received = %d, want 2", c.received.Load())
	}
	if c.dropped.Load() != 3 {
		t.Fatalf("dropped = %d, want 3", c.dropped.Load())
	}
	// Bus counters reflect it too
	snap := h.Counters().Snapshot()
	if snap["foo"].Dropped != 3 {
		t.Fatalf("hub dropped = %d, want 3", snap["foo"].Dropped)
	}
}

func TestHub_UnregisterClosesChannel(t *testing.T) {
	h := NewHub(10)
	c, _ := h.Register(transport.Hello{})
	h.Unregister(c.ID)

	// Channel should be closed; receive returns zero value with ok=false
	_, ok := <-c.SendCh
	if ok {
		t.Fatal("SendCh should be closed after Unregister")
	}

	// Further Deliver must not panic (closed flag prevents send)
	h.Deliver(mkEvent("foo", "x"))
	if h.Len() != 0 {
		t.Fatalf("Len after Unregister = %d, want 0", h.Len())
	}
}

func TestHub_UnregisterIdempotent(t *testing.T) {
	h := NewHub(10)
	c, _ := h.Register(transport.Hello{})
	h.Unregister(c.ID)
	h.Unregister(c.ID) // must not panic
	h.Unregister(9999) // unknown ID
}

func TestHub_BroadcastReachesAllConsumers(t *testing.T) {
	h := NewHub(10)
	a, _ := h.Register(transport.Hello{ConsumerPID: 1})
	b, _ := h.Register(transport.Hello{ConsumerPID: 2})

	bye := transport.Bye{Type: transport.FrameTypeBye, Reason: "shutdown"}
	if got := h.Broadcast(bye); got != 2 {
		t.Fatalf("Broadcast delivered to %d, want 2", got)
	}

	for _, c := range []*Consumer{a, b} {
		select {
		case f := <-c.SendCh:
			if _, ok := f.(transport.Bye); !ok {
				t.Errorf("PID %d got %T, want Bye", c.PID, f)
			}
		case <-time.After(time.Second):
			t.Errorf("PID %d did not receive broadcast", c.PID)
		}
	}
}

func TestHub_Snapshot_SortedByPID(t *testing.T) {
	h := NewHub(10)
	for _, pid := range []int{30, 10, 20} {
		_, _ = h.Register(transport.Hello{ConsumerPID: pid, EventTypes: []string{"a"}})
	}
	snap := h.Snapshot()
	if len(snap) != 3 || snap[0].PID != 10 || snap[1].PID != 20 || snap[2].PID != 30 {
		t.Fatalf("snapshot not sorted: %+v", snap)
	}
}

func TestHub_PerConsumerSeqRestartsAtOne(t *testing.T) {
	h := NewHub(10)
	a, _ := h.Register(transport.Hello{ConsumerPID: 1})
	b, _ := h.Register(transport.Hello{ConsumerPID: 2})

	h.Deliver(mkEvent("foo", "x"))
	h.Deliver(mkEvent("foo", "y"))

	ea := drain(a, 2, t)
	eb := drain(b, 2, t)

	for i, ev := range ea {
		if ev.Seq != uint64(i+1) {
			t.Errorf("consumer a seq[%d] = %d, want %d", i, ev.Seq, i+1)
		}
	}
	for i, ev := range eb {
		if ev.Seq != uint64(i+1) {
			t.Errorf("consumer b seq[%d] = %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func TestHub_NilEventNoOp(t *testing.T) {
	h := NewHub(10)
	c, _ := h.Register(transport.Hello{})
	h.Deliver(nil)
	if c.received.Load() != 0 {
		t.Fatal("nil event should not increment")
	}
}
