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
	"sort"
	"sync"
	"sync/atomic"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// PerTypeCounters is bus-wide and tracks how many events of each event_type
// the bus delivered (or dropped due to backpressure) since startup. Surfaced
// via `dws event status`. Concurrent-safe.
//
// Implementation note: a map of *atomic uint64 pairs lets us avoid taking
// the mu in the hot Add() path; mu is only held when inserting a new
// event_type key.
type PerTypeCounters struct {
	mu sync.RWMutex
	m  map[string]*typeRow
}

type typeRow struct {
	received atomic.Uint64
	dropped  atomic.Uint64
}

// NewPerTypeCounters returns an empty counter set.
func NewPerTypeCounters() *PerTypeCounters {
	return &PerTypeCounters{m: make(map[string]*typeRow)}
}

// AddReceived increments the received counter for eventType (allocating the
// row on first sight of a new type).
func (c *PerTypeCounters) AddReceived(eventType string) {
	c.row(eventType).received.Add(1)
}

// AddDropped increments the dropped counter for eventType.
func (c *PerTypeCounters) AddDropped(eventType string) {
	c.row(eventType).dropped.Add(1)
}

// Snapshot returns a deterministic point-in-time view, sorted by
// event_type. Used by status RPC encoding.
func (c *PerTypeCounters) Snapshot() map[string]transport.Counters {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]transport.Counters, len(c.m))
	for k, row := range c.m {
		out[k] = transport.Counters{
			Received: row.received.Load(),
			Dropped:  row.dropped.Load(),
		}
	}
	return out
}

// SortedTypes returns the known event_types in deterministic order. Useful
// for human-readable formatting (status table).
func (c *PerTypeCounters) SortedTypes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.m))
	for k := range c.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DropRatePercent returns rounded (dropped / (received + dropped)) * 100
// for the given event_type, or -1 if the type was never observed. Used by
// the drop-rate stderr warning when crossing a configurable threshold.
func (c *PerTypeCounters) DropRatePercent(eventType string) int {
	c.mu.RLock()
	row, ok := c.m[eventType]
	c.mu.RUnlock()
	if !ok {
		return -1
	}
	r := row.received.Load()
	d := row.dropped.Load()
	total := r + d
	if total == 0 {
		return -1
	}
	return int((d * 100) / total)
}

func (c *PerTypeCounters) row(eventType string) *typeRow {
	c.mu.RLock()
	row, ok := c.m[eventType]
	c.mu.RUnlock()
	if ok {
		return row
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// double-check after acquiring write lock
	if row, ok = c.m[eventType]; ok {
		return row
	}
	row = &typeRow{}
	c.m[eventType] = row
	return row
}
