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
	"sync"
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

func assertNoEvent(c *Consumer, t *testing.T) {
	t.Helper()
	select {
	case f := <-c.SendCh:
		t.Fatalf("unexpected frame: %#v", f)
	case <-time.After(50 * time.Millisecond):
	}
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

func TestHub_DeliverFiltersBySubscribeID(t *testing.T) {
	h := NewHub(10)
	c, err := h.Register(transport.Hello{
		EventTypes:  []string{"user_im_message_receive_o2o"},
		SubscribeID: "sub-b",
	})
	if err != nil {
		t.Fatal(err)
	}

	wrong := mkEvent("user_im_message_receive_o2o", "1")
	wrong.SubscribeID = "sub-c"
	h.Deliver(wrong)
	assertNoEvent(c, t)

	right := mkEvent("user_im_message_receive_o2o", "2")
	right.SubscribeID = "sub-b"
	h.Deliver(right)

	got := drain(c, 1, t)
	if got[0].EventID != "2" || got[0].SubscribeID != "sub-b" {
		t.Fatalf("event = %#v, want sub-b event 2", got[0])
	}
}

func TestHub_DeliverSubscribeIDSeparatesSameEventType(t *testing.T) {
	h := NewHub(10)
	b, err := h.Register(transport.Hello{
		EventTypes:  []string{"user_im_message_receive_o2o"},
		SubscribeID: "sub-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.Register(transport.Hello{
		EventTypes:  []string{"user_im_message_receive_o2o"},
		SubscribeID: "sub-c",
	})
	if err != nil {
		t.Fatal(err)
	}

	evB := mkEvent("user_im_message_receive_o2o", "b-msg")
	evB.SubscribeID = "sub-b"
	evC := mkEvent("user_im_message_receive_o2o", "c-msg")
	evC.SubscribeID = "sub-c"
	h.Deliver(evB)
	h.Deliver(evC)

	gotB := drain(b, 1, t)
	gotC := drain(c, 1, t)
	if gotB[0].EventID != "b-msg" || gotB[0].SubscribeID != "sub-b" {
		t.Fatalf("B consumer got %#v", gotB[0])
	}
	if gotC[0].EventID != "c-msg" || gotC[0].SubscribeID != "sub-c" {
		t.Fatalf("C consumer got %#v", gotC[0])
	}
	assertNoEvent(b, t)
	assertNoEvent(c, t)
}

func TestHub_DeliverDropsMissingSubscribeIDForSpecificConsumer(t *testing.T) {
	h := NewHub(10)
	c, err := h.Register(transport.Hello{
		EventTypes:  []string{"user_im_message_receive_group"},
		SubscribeID: "sub-group",
	})
	if err != nil {
		t.Fatal(err)
	}

	h.Deliver(mkEvent("user_im_message_receive_group", "missing-sub"))

	assertNoEvent(c, t)
}

func TestHub_DeliverEmptyConsumerSubscribeIDReceivesAnySubscribeID(t *testing.T) {
	h := NewHub(10)
	c, err := h.Register(transport.Hello{
		EventTypes: []string{"user_im_message_receive_o2o"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := mkEvent("user_im_message_receive_o2o", "1")
	ev.SubscribeID = "sub-any"
	h.Deliver(ev)

	got := drain(c, 1, t)
	if got[0].SubscribeID != "sub-any" {
		t.Fatalf("event subscribe_id = %q, want sub-any", got[0].SubscribeID)
	}
}

func TestHub_DeliverEventTypeMismatchEvenWithSubscribeID(t *testing.T) {
	h := NewHub(10)
	c, err := h.Register(transport.Hello{
		EventTypes:  []string{"user_im_message_receive_o2o"},
		SubscribeID: "sub-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := mkEvent("user_im_message_receive_group", "1")
	ev.SubscribeID = "sub-1"

	h.Deliver(ev)

	assertNoEvent(c, t)
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

func TestHub_ConcurrentDeliverBroadcastUnregister(t *testing.T) {
	for iteration := 0; iteration < 200; iteration++ {
		h := NewHub(4)
		c, err := h.Register(transport.Hello{ConsumerPID: iteration + 1})
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		for producer := 0; producer < 4; producer++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 25; i++ {
					h.Deliver(mkEvent("foo", "event"))
					h.Broadcast(transport.SourceState{Type: transport.FrameTypeSourceState, State: "connected"})
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			h.Unregister(c.ID)
		}()

		close(start)
		wg.Wait()

		deadline := time.After(time.Second)
	drain:
		for {
			select {
			case _, ok := <-c.SendCh:
				if !ok {
					break drain
				}
			case <-deadline:
				t.Fatal("SendCh was not closed after concurrent unregister")
			}
		}

		seq := c.seq.Load()
		received := c.received.Load()
		dropped := c.dropped.Load()
		h.Deliver(mkEvent("foo", "after-close"))
		h.Broadcast(transport.Bye{Type: transport.FrameTypeBye})
		if c.seq.Load() != seq || c.received.Load() != received || c.dropped.Load() != dropped {
			t.Fatal("closed consumer counters changed after unregister")
		}
	}
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
