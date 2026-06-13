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
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestConvSessionsPersistAndReload verifies a minted session survives a
// "restart": a fresh convSessions pointed at the same store recovers the map
// and resumes (--resume) instead of minting a new --session-id.
func TestConvSessionsPersistAndReload(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	path := connectSessionStorePath("robot-abc")
	if path == "" {
		t.Fatal("store path should be non-empty for a real clientId")
	}

	s := newConvSessions(path)
	first := s.args("conv-1")
	if first[0] != "--session-id" {
		t.Fatalf("first call should mint: %v", first)
	}
	id := first[1]

	// The mint must have hit disk already (persist happens inside args).
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session store not written: %v", err)
	}

	// Simulate a connector restart: a brand-new map from the same path.
	reloaded := newConvSessions(path)
	got := reloaded.args("conv-1")
	if got[0] != "--resume" || got[1] != id {
		t.Fatalf("after reload want [--resume %s], got %v", id, got)
	}

	// reset must persist the removal: a further restart re-mints.
	reloaded.reset("conv-1")
	afterReset := newConvSessions(path)
	fresh := afterReset.args("conv-1")
	if fresh[0] != "--session-id" || fresh[1] == id {
		t.Fatalf("after reset+reload want a fresh --session-id != %s, got %v", id, fresh)
	}
}

// TestConvSessionsCorruptFileDegrades verifies a corrupt store file degrades to
// an empty map (fresh start) without panicking or erroring.
func TestConvSessionsCorruptFileDegrades(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connect", "robot", "sessions.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{ this is not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := newConvSessions(path) // must not panic
	got := s.args("conv-1")
	if got[0] != "--session-id" {
		t.Fatalf("corrupt store should start empty (mint), got %v", got)
	}
	// And it should self-heal: the next load reads the now-valid file.
	reloaded := newConvSessions(path)
	if r := reloaded.args("conv-1"); r[0] != "--resume" || r[1] != got[1] {
		t.Fatalf("after rewrite want [--resume %s], got %v", got[1], r)
	}
}

// TestConvSessionsEmptyPathInMemory verifies an empty path keeps everything in
// memory: nothing is written and behaviour matches the pre-persistence map.
func TestConvSessionsEmptyPathInMemory(t *testing.T) {
	if p := connectSessionStorePath(""); p != "" {
		t.Fatalf("empty clientId must yield empty path, got %q", p)
	}
	s := newConvSessions("")
	if first := s.args("c"); first[0] != "--session-id" {
		t.Fatalf("mint expected, got %v", first)
	}
	if again := s.args("c"); again[0] != "--resume" {
		t.Fatalf("resume expected, got %v", again)
	}
}

// TestConvSessionsConcurrentAccess exercises the lock under concurrent
// args/reset across many conversations, persisting to a real file, to catch
// data races (run with -race). It must not panic and must leave a parseable
// store.
func TestConvSessionsConcurrentAccess(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	s := newConvSessions(connectSessionStorePath("robot-race"))

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			conv := "conv-" + string(rune('a'+n%8))
			for j := 0; j < 50; j++ {
				_ = s.args(conv)
				if j%7 == 0 {
					s.reset(conv)
				}
			}
		}(i)
	}
	wg.Wait()

	// The store must still be valid JSON after the storm.
	reloaded := newConvSessions(connectSessionStorePath("robot-race"))
	_ = reloaded.args("conv-z") // must not panic
}
