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

package helpers

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

// TestConvQueueCoalescesSameConversationBurst: while one conversation is
// running, later messages are batched into one follow-up turn instead of forming
// a long FIFO of stale prompts.
func TestConvQueueCoalescesSameConversationBurst(t *testing.T) {
	q := newConvQueue()
	started := make(chan struct{})
	release := make(chan struct{})
	batches := make(chan []string, 2)
	var once sync.Once
	process := func(batch []connectQueuedTurn) {
		var got []string
		for _, turn := range batch {
			got = append(got, turn.text)
		}
		batches <- got
		once.Do(func() {
			close(started)
			<-release
		})
	}

	q.submit(connectQueuedTurn{convID: "conv-1", text: "first"}, process)
	<-started
	q.submit(connectQueuedTurn{convID: "conv-1", text: "second"}, process)
	q.submit(connectQueuedTurn{convID: "conv-1", text: "third"}, process)
	close(release)

	if got := <-batches; !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("first batch = %v, want [first]", got)
	}
	if got := <-batches; !reflect.DeepEqual(got, []string{"second", "third"}) {
		t.Fatalf("pending batch = %v, want [second third]", got)
	}
}

// TestConvQueueParallelAcrossConversations: a slow conversation must not
// block another conversation.
func TestConvQueueParallelAcrossConversations(t *testing.T) {
	q := newConvQueue()
	slowRelease := make(chan struct{})
	slowStarted := make(chan struct{})
	q.submit(connectQueuedTurn{convID: "conv-slow", text: "slow"}, func([]connectQueuedTurn) {
		close(slowStarted)
		<-slowRelease
	})
	<-slowStarted

	fastDone := make(chan struct{})
	q.submit(connectQueuedTurn{convID: "conv-fast", text: "fast"}, func([]connectQueuedTurn) { close(fastDone) })
	select {
	case <-fastDone:
	case <-time.After(2 * time.Second):
		t.Fatal("fast conversation blocked behind slow one")
	}
	close(slowRelease)
}

// TestConvQueueDrainsEntries: once a conversation's queue drains, its chain
// entry is dropped (idle chats hold no memory).
func TestConvQueueDrainsEntries(t *testing.T) {
	q := newConvQueue()
	var wg sync.WaitGroup
	wg.Add(1)
	q.submit(connectQueuedTurn{convID: "conv-1", text: "done"}, func([]connectQueuedTurn) { wg.Done() })
	wg.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for {
		q.mu.Lock()
		n := len(q.states)
		q.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue entries not cleaned up: %d left", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
