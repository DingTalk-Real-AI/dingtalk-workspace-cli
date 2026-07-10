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

// convQueue serialises agent turns per conversation while coalescing bursts.
// Same-chat agent calls must never run in parallel — two turns racing on one
// session corrupt the transcript. But a pure FIFO is also bad UX: if a user
// keeps sending clarifications while a long turn is running, the bot should not
// spend the next several minutes answering stale intermediate prompts. Instead,
// messages arriving during the active turn accumulate in one pending batch; when
// the active turn finishes, that whole batch is processed as a single follow-up.
// Different conversations still run in parallel.
type convQueue struct {
	mu     sync.Mutex
	states map[string]*convQueueState
}

type convQueueState struct {
	running bool
	pending []connectQueuedTurn
	process func([]connectQueuedTurn)
}

func newConvQueue() *convQueue {
	return &convQueue{states: map[string]*convQueueState{}}
}

// submit schedules turn handling and returns immediately (the Stream callback
// must ack fast). If the conversation is already running, the turn is appended
// to that conversation's pending batch instead of creating another queued worker.
func (q *convQueue) submit(turn connectQueuedTurn, process func([]connectQueuedTurn)) {
	convID := turn.convID
	q.mu.Lock()
	st := q.states[convID]
	if st == nil {
		st = &convQueueState{}
		q.states[convID] = st
	}
	if st.running {
		st.pending = append(st.pending, turn)
		if process != nil {
			st.process = process
		}
		q.mu.Unlock()
		return
	}
	st.running = true
	st.process = process
	q.mu.Unlock()
	go q.drain(convID, []connectQueuedTurn{turn})
}

func (q *convQueue) drain(convID string, batch []connectQueuedTurn) {
	for {
		process := q.processFor(convID)
		if process != nil {
			process(batch)
		}

		q.mu.Lock()
		st := q.states[convID]
		if st == nil || len(st.pending) == 0 {
			delete(q.states, convID)
			q.mu.Unlock()
			return
		}
		batch = append([]connectQueuedTurn(nil), st.pending...)
		st.pending = nil
		q.mu.Unlock()
	}
}

func (q *convQueue) processFor(convID string) func([]connectQueuedTurn) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if st := q.states[convID]; st != nil {
		return st.process
	}
	return nil
}
