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

package dedup

import (
	"strconv"
	"sync"
	"testing"
)

func TestLRU_FirstSeenIsFalse(t *testing.T) {
	l := New()
	if l.Seen("a") {
		t.Fatal("first occurrence should not be reported as seen")
	}
	if !l.Seen("a") {
		t.Fatal("second occurrence should be reported as seen")
	}
}

func TestLRU_EmptyKeyNeverSeen(t *testing.T) {
	l := New()
	if l.Seen("") {
		t.Fatal("empty key must never be reported as seen")
	}
	if l.Seen("") {
		t.Fatal("empty key must never be stored / reported as seen on second call either")
	}
	if l.Len() != 0 {
		t.Fatalf("empty key must not be stored, got Len=%d", l.Len())
	}
}

func TestLRU_EvictsOldestAtCapacity(t *testing.T) {
	l := NewWithCapacity(3)
	for _, k := range []string{"a", "b", "c"} {
		if l.Seen(k) {
			t.Fatalf("unexpected seen for %s", k)
		}
	}
	// d inserts → a should be evicted
	if l.Seen("d") {
		t.Fatal("d is new, should not be seen")
	}
	if l.Seen("a") {
		t.Fatal("a should have been evicted; second insert returns not-seen")
	}
	// Now b should be the oldest. After re-querying "c" (refreshes c),
	// inserting "e" should evict b not c.
	if !l.Seen("c") {
		t.Fatal("c is still in set, should be seen")
	}
	if l.Seen("e") {
		t.Fatal("e is new")
	}
	if l.Seen("b") {
		t.Fatal("b should have been evicted by e (c was just refreshed)")
	}
}

func TestLRU_LenAndCap(t *testing.T) {
	l := NewWithCapacity(5)
	if l.Cap() != 5 {
		t.Fatalf("Cap = %d, want 5", l.Cap())
	}
	if l.Len() != 0 {
		t.Fatalf("initial Len = %d, want 0", l.Len())
	}
	_ = l.Seen("x")
	_ = l.Seen("y")
	if l.Len() != 2 {
		t.Fatalf("after 2 inserts Len = %d, want 2", l.Len())
	}
}

func TestLRU_ZeroCapacityUsesDefault(t *testing.T) {
	l := NewWithCapacity(0)
	if l.Cap() != DefaultCapacity {
		t.Fatalf("zero cap should use DefaultCapacity, got %d", l.Cap())
	}
}

func TestLRU_ConcurrentSafety(t *testing.T) {
	l := NewWithCapacity(1000)
	const N = 200
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = l.Seen(strconv.Itoa(i))
			_ = l.Seen(strconv.Itoa(i)) // duplicate
		}()
	}
	wg.Wait()
	if l.Len() != N {
		t.Fatalf("Len after concurrent insert = %d, want %d", l.Len(), N)
	}
}
