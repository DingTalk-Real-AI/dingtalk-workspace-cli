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

import (
	"path/filepath"
	"testing"
)

// TestCodexThreadSessionsPersist verifies that a codex conversation's thread
// survives a connector restart: a second store opened on the same path restores
// the convID→threadID mapping, and a reset is persisted too.
func TestCodexThreadSessionsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex-threads.json")

	s := newCodexThreadSessions(path)
	if got := s.threadID("conv-1"); got != "" {
		t.Fatalf("fresh store threadID = %q, want empty", got)
	}
	s.setThreadID("conv-1", "thr_a")
	s.setThreadID("conv-2", "thr_b")

	// Simulate a restart: a new store on the same path must restore both.
	restarted := newCodexThreadSessions(path)
	if got := restarted.threadID("conv-1"); got != "thr_a" {
		t.Errorf("after restart conv-1 = %q, want thr_a", got)
	}
	if got := restarted.threadID("conv-2"); got != "thr_b" {
		t.Errorf("after restart conv-2 = %q, want thr_b", got)
	}

	// Reset is persisted: the dropped thread must not resurrect on restart.
	restarted.reset("conv-1")
	again := newCodexThreadSessions(path)
	if got := again.threadID("conv-1"); got != "" {
		t.Errorf("after reset+restart conv-1 = %q, want empty", got)
	}
	if got := again.threadID("conv-2"); got != "thr_b" {
		t.Errorf("after reset+restart conv-2 = %q, want thr_b (untouched)", got)
	}
}

// TestCodexThreadSessionsInMemory verifies that an empty path keeps the store
// purely in memory: it still works within the process but writes nothing to
// disk, preserving the pre-persistence behaviour.
func TestCodexThreadSessionsInMemory(t *testing.T) {
	s := newCodexThreadSessions("")
	s.setThreadID("conv-1", "thr_a")
	if got := s.threadID("conv-1"); got != "thr_a" {
		t.Errorf("in-memory conv-1 = %q, want thr_a", got)
	}
	// A separate in-memory store shares no state (nothing was persisted).
	other := newCodexThreadSessions("")
	if got := other.threadID("conv-1"); got != "" {
		t.Errorf("separate in-memory store conv-1 = %q, want empty", got)
	}
}
