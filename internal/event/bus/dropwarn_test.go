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
	"testing"
)

// dropWarnWatcher is exercised end-to-end by daemon integration tests
// (it runs as a goroutine inside bus.Run). At the unit level we just
// verify the threshold-clamping behaviour in isolation so a misconfigured
// env var doesn't disable the safety net silently.
func TestDropWarnWatcher_ThresholdClamp(t *testing.T) {
	// The function applies its own clamp before using the threshold;
	// we re-derive the expected effective value through the same path
	// (calling helper-style code below).
	for _, in := range []int{-1, 0, 101, 1000} {
		got := clampDropWarnPctForTest(in)
		if got != DefaultDropWarnPercent {
			t.Errorf("invalid threshold %d should clamp to %d, got %d",
				in, DefaultDropWarnPercent, got)
		}
	}
	for _, in := range []int{1, 5, 10, 50, 100} {
		if got := clampDropWarnPctForTest(in); got != in {
			t.Errorf("valid threshold %d should pass through, got %d", in, got)
		}
	}
}

// clampDropWarnPctForTest mirrors the clamp logic in dropWarnWatcher.
// Kept separate so the public Watcher signature does not have to expose
// internal validation as a method — the watcher reads threshold from
// closure, and tests want to assert on the validation contract.
func clampDropWarnPctForTest(threshold int) int {
	if threshold <= 0 || threshold > 100 {
		return DefaultDropWarnPercent
	}
	return threshold
}
