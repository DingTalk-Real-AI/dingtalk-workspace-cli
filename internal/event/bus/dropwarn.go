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
	"context"
	"log/slog"
	"time"
)

// DefaultDropWarnPercent is the threshold above which per-event-type drop
// rate triggers a slog WARN line in bus.log. 5% is the plan default
// (§15 已决项); overridable via DWS_EVENT_DROP_WARN_PCT.
//
// We use whole-percentage granularity (int) because the counter math is
// integer; sub-percent precision would just add noise.
const DefaultDropWarnPercent = 5

// dropWarnTickInterval is how often the watcher samples counters. 30s
// balances responsiveness ("see the warning while the burst is still
// happening") with log noise (one warning per scan, not one per drop).
var dropWarnTickInterval = 30 * time.Second

// dropWarnState memoises the last warned drop rate per event type so we
// only emit a fresh WARN when the situation worsens by at least
// dropWarnHysteresis percentage points. Without hysteresis a steady-state
// burst would re-warn every tick.
const dropWarnHysteresis = 5

// dropWarnWatcher periodically samples per-event-type counters; for any
// type whose drop rate crosses the threshold (and hasn't recently been
// warned at the same level), it emits a slog WARN. Runs as a daemon
// goroutine spawned from bus.Run.
//
// Lifecycle: returns when ctx is cancelled (bus shutdown). Never holds
// any external lock — uses the counters' own concurrency-safe Snapshot.
func dropWarnWatcher(ctx context.Context, counters *PerTypeCounters, log *slog.Logger, threshold int) {
	if threshold <= 0 || threshold > 100 {
		threshold = DefaultDropWarnPercent
	}
	tick := time.NewTicker(dropWarnTickInterval)
	defer tick.Stop()
	lastWarnedPct := make(map[string]int)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			scanDropWarnings(counters, log, threshold, lastWarnedPct)
		}
	}
}

func scanDropWarnings(counters *PerTypeCounters, log *slog.Logger, threshold int, lastWarnedPct map[string]int) {
	for _, et := range counters.SortedTypes() {
		pct := counters.DropRatePercent(et)
		if pct < threshold {
			// drop rate is healthy → forget any prior warning so
			// a future spike re-triggers a fresh WARN
			delete(lastWarnedPct, et)
			continue
		}
		prev, warned := lastWarnedPct[et]
		if warned && pct < prev+dropWarnHysteresis {
			continue // not significantly worse; suppress
		}
		log.Warn("bus: event type backpressure",
			"event_type", et,
			"drop_pct", pct,
			"threshold_pct", threshold,
		)
		lastWarnedPct[et] = pct
	}
}
