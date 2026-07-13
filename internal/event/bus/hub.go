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
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// DefaultSendBuffer is the per-consumer channel capacity used when the Hub
// is constructed via NewHub. Sized to absorb a short event burst (~100ms at
// ~1k evt/s) without backpressure. Overridable via DWS_EVENT_CONSUMER_BUFFER
// at daemon start (plan §15 已决但暴露方式).
const DefaultSendBuffer = 100

// Consumer represents one registered IPC connection to the bus. Wire-side
// reader/writer goroutines are owned by daemon.go; the Hub holds the
// metadata + sendCh.
type Consumer struct {
	ID           int      // monotonic, assigned by Hub
	PID          int      // from Hello.ConsumerPID
	EventTypes   []string // raw wildcard patterns from Hello
	Filter       string   // raw regex from Hello (for status display)
	SubscribeID  string   // optional personal subscription label and local isolation key
	SubscribedAt time.Time
	SendCh       chan any // bus → consume frames (Event/SourceState/Heartbeat/Bye)
	matcher      consumerMatcher
	sendMu       sync.Mutex    // serialises Deliver/Broadcast with SendCh close
	closed       bool          // guarded by sendMu
	seq          atomic.Uint64 // monotonic per-consumer sequence, starts at 1
	received     atomic.Uint64
	dropped      atomic.Uint64
}

// consumerMatcher pre-compiles EventTypes wildcard patterns and the optional
// Filter regex into a fast checker invoked once per delivered event per
// consumer. Empty EventTypes means catch-all (everything matches except
// what Filter excludes). Non-empty SubscribeID is an additional exact-match
// constraint used by personal_stream consumers so same event_type subscriptions
// do not fan out to each other.
type consumerMatcher struct {
	catchAll    bool
	exact       map[string]struct{} // patterns without '*'
	prefixes    []string            // patterns ending in ".*" or "*" — store the prefix only
	filter      *regexp.Regexp      // nil if no filter
	subscribeID string              // empty = no subscribe_id filtering
}

func compileMatcher(eventTypes []string, filter string, subscribeID string) (consumerMatcher, error) {
	m := consumerMatcher{
		exact:       make(map[string]struct{}),
		subscribeID: strings.TrimSpace(subscribeID),
	}
	if len(eventTypes) == 0 {
		m.catchAll = true
	}
	for _, raw := range eventTypes {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// Treat trailing '*' or '.*' as a prefix wildcard (e.g. "im.*",
		// "im.message.*"). Middle/leading wildcards are rare for event_type
		// strings — if a real use case appears we can switch to regex
		// compilation here without breaking the wire format.
		switch {
		case raw == "*":
			m.catchAll = true
		case strings.HasSuffix(raw, ".*"):
			m.prefixes = append(m.prefixes, raw[:len(raw)-1]) // keep the dot, drop the *
		case strings.HasSuffix(raw, "*"):
			m.prefixes = append(m.prefixes, raw[:len(raw)-1])
		default:
			m.exact[raw] = struct{}{}
		}
	}
	if filter != "" {
		re, err := regexp.Compile(filter)
		if err != nil {
			return consumerMatcher{}, err
		}
		m.filter = re
	}
	return m, nil
}

func (m *consumerMatcher) matches(raw *dwsevent.RawEvent) bool {
	if raw == nil {
		return false
	}
	if m.subscribeID != "" && raw.SubscribeID != m.subscribeID {
		return false
	}
	eventType := raw.EventType
	// First the include rules: catchAll OR exact-list OR prefix-list.
	included := m.catchAll
	if !included {
		if _, ok := m.exact[eventType]; ok {
			included = true
		}
	}
	if !included {
		for _, p := range m.prefixes {
			if strings.HasPrefix(eventType, p) {
				included = true
				break
			}
		}
	}
	if !included {
		return false
	}
	// Filter is an additional AND constraint (regex on the event_type
	// string). Empty filter is no-op.
	if m.filter != nil && !m.filter.MatchString(eventType) {
		return false
	}
	return true
}

// Hub is the bus's fan-out engine. It owns the set of registered consumers
// and the bus-wide per-event-type counters. The Hub is concurrency-safe
// across Register/Unregister/Deliver/Snapshot; Deliver is the hot path and
// is RLock-only.
type Hub struct {
	mu         sync.RWMutex
	consumers  map[int]*Consumer
	nextID     int
	bufferSize int
	counters   *PerTypeCounters
}

// NewHub returns a Hub with the given per-consumer channel buffer size.
// Zero or negative uses DefaultSendBuffer.
func NewHub(bufferSize int) *Hub {
	if bufferSize <= 0 {
		bufferSize = DefaultSendBuffer
	}
	return &Hub{
		consumers:  make(map[int]*Consumer),
		bufferSize: bufferSize,
		counters:   NewPerTypeCounters(),
	}
}

// Counters exposes the bus-wide per-event-type counter set for daemon-side
// rendering (status RPC, drop-rate warning).
func (h *Hub) Counters() *PerTypeCounters { return h.counters }

// RegisterError wraps the matcher compile error so the daemon can refuse
// the Hello and return a clean error to the consume client (instead of
// silently accepting a bad filter regex).
type RegisterError struct{ Err error }

func (e *RegisterError) Error() string { return "bus: register consumer: " + e.Err.Error() }
func (e *RegisterError) Unwrap() error { return e.Err }

// Register adds a consumer derived from a Hello frame. Returns a new
// Consumer with the populated ID + sendCh ready to use, or a RegisterError
// if the Hello's Filter regex is invalid.
func (h *Hub) Register(hello transport.Hello) (*Consumer, error) {
	m, err := compileMatcher(hello.EventTypes, hello.Filter, hello.SubscribeID)
	if err != nil {
		return nil, &RegisterError{Err: err}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	c := &Consumer{
		ID:           h.nextID,
		PID:          hello.ConsumerPID,
		EventTypes:   append([]string(nil), hello.EventTypes...),
		Filter:       hello.Filter,
		SubscribeID:  strings.TrimSpace(hello.SubscribeID),
		SubscribedAt: time.Now().UTC(),
		SendCh:       make(chan any, h.bufferSize),
		matcher:      m,
	}
	h.consumers[c.ID] = c
	return c, nil
}

// Unregister removes a consumer by ID and closes its sendCh. Idempotent —
// calling twice or on an unknown ID is a no-op. closeSend shares the same
// per-consumer lock as Deliver/Broadcast, so a stale Hub snapshot cannot send
// to the channel after it has been closed.
func (h *Hub) Unregister(id int) {
	h.mu.Lock()
	c, ok := h.consumers[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.consumers, id)
	h.mu.Unlock()
	c.closeSend()
}

// Deliver fans the raw event out to every matching consumer. Updates
// bus-wide and per-consumer counters. Always non-blocking — drops the
// oldest entry in any full sendCh (plan invariant #1).
//
// Called from the bus daemon's main loop after dedup; safe for concurrent
// callers (Hub uses an RLock, Consumer drop-oldest is single-producer-safe
// because the daemon serialises Deliver per event).
func (h *Hub) Deliver(raw *dwsevent.RawEvent) {
	if raw == nil {
		return
	}
	h.counters.AddReceived(raw.EventType)

	h.mu.RLock()
	matched := make([]*Consumer, 0, len(h.consumers))
	for _, c := range h.consumers {
		if c.matcher.matches(raw) {
			matched = append(matched, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range matched {
		c.deliver(raw, h.counters)
	}
}

// deliver builds the per-consumer Event frame (assigning seq) and pushes
// it onto sendCh with drop-oldest semantics. Updates per-consumer and
// bus-wide drop counters.
func (c *Consumer) deliver(raw *dwsevent.RawEvent, hubCounters *PerTypeCounters) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return
	}

	seq := c.seq.Add(1)
	frame := transport.Event{
		Type:              transport.FrameTypeEvent,
		Seq:               seq,
		EventID:           raw.EventID,
		EventBornTime:     raw.EventBornTime,
		EventCorpID:       raw.EventCorpID,
		EventType:         raw.EventType,
		EventUnifiedAppID: raw.EventUnifiedAppID,
		EventScope:        raw.EventScope,
		SubscribeID:       raw.SubscribeID,
		SourceID:          raw.SourceID,
		RuleType:          raw.RuleType,
		Data:              raw.Data,
		Headers:           raw.Headers,
		ReceivedAtUnixMS:  raw.ReceivedAt.UnixMilli(),
	}
	success, evicted := c.tryPushOrDropOldestLocked(frame)
	if evicted {
		// An older event we had previously enqueued is gone.
		c.dropped.Add(1)
		c.received.Add(^uint64(0)) // -1
		hubCounters.AddDropped(raw.EventType)
	}
	if success {
		c.received.Add(1)
	} else {
		// New event also didn't make it (rare: lost the race after eviction)
		c.dropped.Add(1)
		if !evicted {
			// No eviction happened but push still failed → unexpected; only
			// reached via concurrent reader-then-something. Count the bus-wide
			// drop too so the metric matches per-consumer drops.
			hubCounters.AddDropped(raw.EventType)
		}
	}
}

// tryPushOrDropOldestLocked tries to push frame to SendCh non-blockingly. If
// full, it pops the oldest entry to make room and tries once more. Caller must
// hold c.sendMu, which makes drop-oldest a true single-producer operation even
// when Deliver and Broadcast run concurrently.
//
// Returns:
//
//	success: true if the new frame is now in the channel
//	evicted: true if we removed an older queued frame to make room
//
// "received" semantics in deliver() interpret these:
//
//	success=true,  evicted=false → +1 received (normal push)
//	success=true,  evicted=true  → net 0 (lost 1, gained 1); +1 dropped
//	success=false, evicted=true  → -1 received, +2 dropped (both lost; rare)
//	success=false, evicted=false → only possible if reader drained between
//	  try and we still missed (extremely rare); +1 dropped only.
//
// The per-consumer send lock makes this block the sole SendCh producer, so the
// second push cannot race another producer. The transport writer may receive
// between our two operations, which only makes more room — never less.
func (c *Consumer) tryPushOrDropOldestLocked(frame any) (success bool, evicted bool) {
	select {
	case c.SendCh <- frame:
		return true, false
	default:
	}
	// Drop oldest, then retry.
	select {
	case <-c.SendCh:
		evicted = true
	default:
		// Reader took it, room is back.
	}
	select {
	case c.SendCh <- frame:
		return true, evicted
	default:
		return false, evicted
	}
}

func (c *Consumer) enqueue(frame any) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return false
	}
	success, _ := c.tryPushOrDropOldestLocked(frame)
	return success
}

func (c *Consumer) closeSend() {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.SendCh)
}

// Broadcast sends the same frame (e.g. SourceState change, Bye on shutdown)
// to every consumer using drop-oldest semantics. Returns the number of
// consumers the frame was successfully enqueued for.
func (h *Hub) Broadcast(frame any) int {
	h.mu.RLock()
	cs := make([]*Consumer, 0, len(h.consumers))
	for _, c := range h.consumers {
		cs = append(cs, c)
	}
	h.mu.RUnlock()
	ok := 0
	for _, c := range cs {
		if c.enqueue(frame) {
			ok++
		}
	}
	return ok
}

// Snapshot returns a deterministic StatusConsumer slice (sorted by PID)
// for status RPC encoding. Caller MUST NOT mutate the returned slice.
func (h *Hub) Snapshot() []transport.StatusConsumer {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]transport.StatusConsumer, 0, len(h.consumers))
	for _, c := range h.consumers {
		out = append(out, transport.StatusConsumer{
			PID:            c.PID,
			EventTypes:     append([]string(nil), c.EventTypes...),
			Filter:         c.Filter,
			SubscribeID:    c.SubscribeID,
			SubscribedAtMS: c.SubscribedAt.UnixMilli(),
			Received:       c.received.Load(),
			Dropped:        c.dropped.Load(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// Len returns the current number of registered consumers.
func (h *Hub) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.consumers)
}
