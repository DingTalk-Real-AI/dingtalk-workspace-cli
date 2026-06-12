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

import "sync"

// convQueue serialises message handling per conversation: same-chat messages
// run in arrival order — a Q&A follow-up ("还是不行") must see the previous
// turn's session state, and two parallel agent CLIs racing on one --resume
// session corrupt the transcript. Different conversations stay parallel.
// Ported from the hermes gateway's per-session promise chain
// (gateway/run.py session queues).
type convQueue struct {
	mu    sync.Mutex
	tails map[string]chan struct{}
}

func newConvQueue() *convQueue {
	return &convQueue{tails: map[string]chan struct{}{}}
}

// run schedules fn after the conversation's previous task and returns
// immediately (the Stream callback must ack fast). The chain entry is removed
// once the queue for that conversation drains, so idle chats hold no memory.
func (q *convQueue) run(convID string, fn func()) {
	q.mu.Lock()
	prev := q.tails[convID]
	done := make(chan struct{})
	q.tails[convID] = done
	q.mu.Unlock()
	go func() {
		defer func() {
			close(done)
			q.mu.Lock()
			if q.tails[convID] == done {
				delete(q.tails, convID)
			}
			q.mu.Unlock()
		}()
		if prev != nil {
			<-prev
		}
		fn()
	}()
}
