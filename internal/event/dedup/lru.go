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

// Package dedup implements a fixed-capacity LRU set used by the bus to
// suppress duplicate events that the DingTalk Stream SDK redelivers on
// reconnect (see plan invariant #2). The set stores only keys, never values,
// and is safe for concurrent use by multiple goroutines.
package dedup

import (
	"container/list"
	"sync"
)

// DefaultCapacity is the default LRU size when LRU is constructed without an
// explicit capacity. 8192 is sized to absorb a typical reconnect-storm window
// (~5 min × 30 events/s) while staying memory-cheap (~256 KB at 32 bytes/key).
const DefaultCapacity = 8192

// LRU is a fixed-capacity LRU set of string keys. Zero value is not usable;
// call New or NewWithCapacity.
type LRU struct {
	mu       sync.Mutex
	cap      int
	keys     map[string]*list.Element
	eviction *list.List // back = newest, front = oldest
}

// New returns an LRU with DefaultCapacity.
func New() *LRU { return NewWithCapacity(DefaultCapacity) }

// NewWithCapacity returns an LRU sized to hold up to cap keys. cap must be > 0.
func NewWithCapacity(cap int) *LRU {
	if cap <= 0 {
		cap = DefaultCapacity
	}
	return &LRU{
		cap:      cap,
		keys:     make(map[string]*list.Element, cap),
		eviction: list.New(),
	}
}

// Seen reports whether key was already present and inserts it if not. The
// return value is true when the caller should treat the event as a duplicate
// (drop it) and false when this is the first occurrence.
//
// Empty keys are never considered duplicates and are not stored — callers
// without a stable identifier should use RawEvent.DedupKey() which falls back
// to a content hash.
func (l *LRU) Seen(key string) bool {
	if key == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if el, ok := l.keys[key]; ok {
		l.eviction.MoveToBack(el)
		return true
	}

	if len(l.keys) >= l.cap {
		oldest := l.eviction.Front()
		if oldest != nil {
			delete(l.keys, oldest.Value.(string))
			l.eviction.Remove(oldest)
		}
	}

	el := l.eviction.PushBack(key)
	l.keys[key] = el
	return false
}

// Len returns the current number of stored keys.
func (l *LRU) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.keys)
}

// Cap returns the configured capacity.
func (l *LRU) Cap() int { return l.cap }
