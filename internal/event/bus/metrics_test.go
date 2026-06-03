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
	"sync"
	"testing"
)

func TestPerTypeCounters_BasicAdd(t *testing.T) {
	c := NewPerTypeCounters()
	c.AddReceived("im.message.receive_v1")
	c.AddReceived("im.message.receive_v1")
	c.AddDropped("im.message.receive_v1")
	c.AddReceived("approval.task")

	snap := c.Snapshot()
	if snap["im.message.receive_v1"].Received != 2 {
		t.Errorf("im received = %d, want 2", snap["im.message.receive_v1"].Received)
	}
	if snap["im.message.receive_v1"].Dropped != 1 {
		t.Errorf("im dropped = %d, want 1", snap["im.message.receive_v1"].Dropped)
	}
	if snap["approval.task"].Received != 1 {
		t.Errorf("approval received = %d, want 1", snap["approval.task"].Received)
	}
}

func TestPerTypeCounters_SortedTypes(t *testing.T) {
	c := NewPerTypeCounters()
	for _, k := range []string{"z", "a", "m", "b"} {
		c.AddReceived(k)
	}
	got := c.SortedTypes()
	want := []string{"a", "b", "m", "z"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPerTypeCounters_DropRatePercent(t *testing.T) {
	c := NewPerTypeCounters()
	for i := 0; i < 95; i++ {
		c.AddReceived("a")
	}
	for i := 0; i < 5; i++ {
		c.AddDropped("a")
	}
	if got := c.DropRatePercent("a"); got != 5 {
		t.Errorf("DropRatePercent = %d, want 5", got)
	}
	if got := c.DropRatePercent("never-seen"); got != -1 {
		t.Errorf("unseen DropRatePercent = %d, want -1", got)
	}
}

func TestPerTypeCounters_ConcurrentAddSameType(t *testing.T) {
	c := NewPerTypeCounters()
	const N = 1000
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func() { defer wg.Done(); c.AddReceived("hot") }()
		go func() { defer wg.Done(); c.AddDropped("hot") }()
	}
	wg.Wait()
	snap := c.Snapshot()
	if snap["hot"].Received != N || snap["hot"].Dropped != N {
		t.Fatalf("got %+v, want received=%d dropped=%d", snap["hot"], N, N)
	}
}

func TestPerTypeCounters_ConcurrentNewTypes(t *testing.T) {
	c := NewPerTypeCounters()
	const N = 500
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			c.AddReceived(string(rune('a'+(i%26))) + "_" + string(rune('a'+((i/26)%26))))
		}()
	}
	wg.Wait()
	// Each goroutine adds exactly 1; total received across all types must be N.
	var total uint64
	for _, v := range c.Snapshot() {
		total += v.Received
	}
	if total != N {
		t.Fatalf("total received = %d, want %d", total, N)
	}
}
