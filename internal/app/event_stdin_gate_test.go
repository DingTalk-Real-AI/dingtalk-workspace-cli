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
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
)

// A bounded run never arms the stdin-EOF watcher, regardless of stdin
// shape: --max-events / --duration are the lifecycle control.
func TestShouldWatchStdinEOF_BoundedIsNeverArmed(t *testing.T) {
	if shouldWatchStdinEOF(1, 0) {
		t.Error("--max-events set should not arm stdin watcher")
	}
	if shouldWatchStdinEOF(0, 5*time.Second) {
		t.Error("--duration set should not arm stdin watcher")
	}
	if shouldWatchStdinEOF(3, 2*time.Second) {
		t.Error("both bounds set should not arm stdin watcher")
	}
}

// Regression: the detached _bus child must receive --profile so it resolves
// credentials for the same organization as the parent. Missing it made a
// non-default `--profile` consume fail with "bus child reported startup
// failure on ready pipe" (no bus.log).
func TestPersonalBusSpawnArgs_ForwardsProfile(t *testing.T) {
	args := personalBusSpawnArgs(personal.Identity{
		CorpID:   "dinga626d60c1128d449",
		UserID:   "user_123",
		SourceID: "open",
	}, "", "")
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--profile" && args[i+1] == "dinga626d60c1128d449:user_123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("spawn args must forward --profile <corpId>:<userId>; got %v", args)
	}

	// No CorpID → no --profile appended (avoid an empty flag value).
	bare := personalBusSpawnArgs(personal.Identity{SourceID: "open"}, "", "")
	for _, a := range bare {
		if a == "--profile" {
			t.Errorf("must not append --profile when CorpID is empty; got %v", bare)
		}
	}
}
