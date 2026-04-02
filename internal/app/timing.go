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

package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

// Environment variable to enable performance timing output.
const PerfTimingEnv = "DWS_PERF_TIMING"

// timingContextKey is the context key for TimingCollector.
type timingContextKey struct{}

// TimingEntry represents a single timing measurement.
type TimingEntry struct {
	Name      string
	Duration  time.Duration
	Timestamp time.Time
	Seq       int // insertion order
}

// TimingCollector collects timing measurements for a single command execution.
// It is safe for concurrent use.
type TimingCollector struct {
	mu      sync.Mutex
	start   time.Time
	entries []TimingEntry
	seq     int
}

// NewTimingCollector creates a new collector with the start time set to now.
func NewTimingCollector() *TimingCollector {
	return &TimingCollector{
		start:   time.Now(),
		entries: make([]TimingEntry, 0, 16),
	}
}

// Record adds a timing entry with the given name and duration.
func (tc *TimingCollector) Record(name string, d time.Duration) {
	if tc == nil {
		return
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.entries = append(tc.entries, TimingEntry{
		Name:      name,
		Duration:  d,
		Timestamp: time.Now(),
		Seq:       tc.seq,
	})
	tc.seq++
}

// StartTimer returns a function that, when called, records the elapsed time
// since StartTimer was called. This is convenient for defer usage:
//
//	defer tc.StartTimer("operation")()
func (tc *TimingCollector) StartTimer(name string) func() {
	if tc == nil {
		return func() {}
	}
	start := time.Now()
	return func() {
		tc.Record(name, time.Since(start))
	}
}

// Total returns the total elapsed time since the collector was created.
func (tc *TimingCollector) Total() time.Duration {
	if tc == nil {
		return 0
	}
	return time.Since(tc.start)
}

// Entries returns a copy of all recorded entries in insertion order.
func (tc *TimingCollector) Entries() []TimingEntry {
	if tc == nil {
		return nil
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make([]TimingEntry, len(tc.entries))
	copy(result, tc.entries)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Seq < result[j].Seq
	})
	return result
}

// Print writes a summary of all timing entries to the given writer.
func (tc *TimingCollector) Print(w io.Writer) {
	if tc == nil || w == nil {
		return
	}
	entries := tc.Entries()
	if len(entries) == 0 {
		fmt.Fprintf(w, "\n[Timing] Total: %v (no detailed entries)\n", tc.Total().Truncate(time.Millisecond))
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "[Timing] Execution breakdown:")
	for _, e := range entries {
		fmt.Fprintf(w, "  %-30s %v\n", e.Name, e.Duration.Truncate(time.Millisecond))
	}
	fmt.Fprintf(w, "  %-30s %v\n", "──────────────────────────────", "──────────")
	fmt.Fprintf(w, "  %-30s %v\n", "Total", tc.Total().Truncate(time.Millisecond))
}

// PrintIfEnabled prints timing info to stderr if DWS_PERF_TIMING is set.
func (tc *TimingCollector) PrintIfEnabled() {
	if tc == nil {
		return
	}
	if os.Getenv(PerfTimingEnv) == "" {
		return
	}
	tc.Print(os.Stderr)
}

// WithTimingCollector returns a new context with the TimingCollector attached.
func WithTimingCollector(ctx context.Context, tc *TimingCollector) context.Context {
	return context.WithValue(ctx, timingContextKey{}, tc)
}

// TimingCollectorFromContext extracts the TimingCollector from context, or nil.
func TimingCollectorFromContext(ctx context.Context) *TimingCollector {
	if ctx == nil {
		return nil
	}
	tc, _ := ctx.Value(timingContextKey{}).(*TimingCollector)
	return tc
}

// RecordTiming is a convenience function to record timing to the collector in context.
func RecordTiming(ctx context.Context, name string, d time.Duration) {
	if tc := TimingCollectorFromContext(ctx); tc != nil {
		tc.Record(name, d)
	}
}

// StartTiming is a convenience function that returns a stop function for defer usage.
// Example:
//
//	defer StartTiming(ctx, "operation")()
func StartTiming(ctx context.Context, name string) func() {
	tc := TimingCollectorFromContext(ctx)
	if tc == nil {
		return func() {}
	}
	return tc.StartTimer(name)
}

// IsPerfTimingEnabled returns true if performance timing output is enabled.
func IsPerfTimingEnabled() bool {
	return os.Getenv(PerfTimingEnv) != ""
}
