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
	"sync"
	"testing"
	"time"
)

// TestConvQueueSerialSameConversation: tasks of one conversation run in
// arrival order even when earlier tasks are slow.
func TestConvQueueSerialSameConversation(t *testing.T) {
	q := newConvQueue()
	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		i := i
		wg.Add(1)
		q.run("conv-1", func() {
			defer wg.Done()
			if i == 1 {
				time.Sleep(50 * time.Millisecond) // slow head must not be overtaken
			}
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
		})
	}
	wg.Wait()
	for i, v := range order {
		if v != i+1 {
			t.Fatalf("order = %v, want 1..5 in sequence", order)
		}
	}
}

// TestConvQueueParallelAcrossConversations: a slow conversation must not
// block another conversation.
func TestConvQueueParallelAcrossConversations(t *testing.T) {
	q := newConvQueue()
	slowRelease := make(chan struct{})
	slowStarted := make(chan struct{})
	q.run("conv-slow", func() {
		close(slowStarted)
		<-slowRelease
	})
	<-slowStarted

	fastDone := make(chan struct{})
	q.run("conv-fast", func() { close(fastDone) })
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
	q.run("conv-1", func() { wg.Done() })
	wg.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for {
		q.mu.Lock()
		n := len(q.tails)
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
