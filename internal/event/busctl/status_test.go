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

package busctl

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
)

func TestEnumerateBuses_EmptyConfigDir(t *testing.T) {
	dir := shortTempDir(t)
	got, err := EnumerateBuses(dir, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty configDir, got %v", got)
	}
}

// makeBusDir creates events/<edition>/<hash>/ with optional meta + lock
// content. Returns the path.
func makeBusDir(t *testing.T, configDir, ed, hash string, withMeta bool, lockPID int) string {
	t.Helper()
	workDir := filepath.Join(configDir, "events", ed, hash)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if withMeta {
		shortHash := hash
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}
		if err := bus.WriteMeta(workDir, bus.Meta{
			ClientID:  "ding_" + shortHash,
			Edition:   ed,
			StartedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if lockPID != 0 {
		if err := os.WriteFile(filepath.Join(workDir, bus.LockFileName),
			[]byte(strconv.Itoa(lockPID)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return workDir
}

func TestEnumerateBuses_DetectsRunningOrphanNotRunning(t *testing.T) {
	dir := shortTempDir(t)
	makeBusDir(t, dir, "open", "aaaa1111", true, os.Getpid())   // running (self pid)
	makeBusDir(t, dir, "open", "bbbb2222", true, 2147483646)    // orphan (dead pid)
	makeBusDir(t, dir, "open", "cccc3333", true, 0)             // not_running (meta only, no lock content)
	makeBusDir(t, dir, "wukong", "dddd4444", true, os.Getpid()) // running, different edition

	all, err := EnumerateBuses(dir, "")
	if err != nil {
		t.Fatalf("EnumerateBuses all: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 entries, got %d: %+v", len(all), all)
	}

	// Filter by edition.
	openOnly, err := EnumerateBuses(dir, "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(openOnly) != 3 {
		t.Fatalf("expected 3 open-edition entries, got %d", len(openOnly))
	}
	for _, e := range openOnly {
		if e.Edition != "open" {
			t.Errorf("editionFilter leak: %+v", e)
		}
	}

	// State classification (in the all-editions slice the entries are
	// sorted by edition,hash so we can index deterministically).
	byHash := map[string]BusEntry{}
	for _, e := range all {
		byHash[e.ClientIDHash] = e
	}
	if got := byHash["aaaa1111"].State; got != BusStateRunning {
		t.Errorf("aaaa1111 state = %s, want running", got)
	}
	if got := byHash["bbbb2222"].State; got != BusStateOrphan {
		t.Errorf("bbbb2222 state = %s, want orphan", got)
	}
	if got := byHash["cccc3333"].State; got != BusStateNotRunning {
		t.Errorf("cccc3333 state = %s, want not_running", got)
	}
	if got := byHash["dddd4444"].State; got != BusStateRunning {
		t.Errorf("dddd4444 state = %s, want running", got)
	}
}

func TestEnumerateBuses_SortedDeterministic(t *testing.T) {
	dir := shortTempDir(t)
	makeBusDir(t, dir, "open", "zzzz", true, 0)
	makeBusDir(t, dir, "open", "aaaa", true, 0)
	makeBusDir(t, dir, "wukong", "bbbb", true, 0)
	got, _ := EnumerateBuses(dir, "")
	if len(got) != 3 {
		t.Fatalf("got %d entries", len(got))
	}
	// expected order: open/aaaa, open/zzzz, wukong/bbbb
	if got[0].ClientIDHash != "aaaa" || got[1].ClientIDHash != "zzzz" || got[2].ClientIDHash != "bbbb" {
		t.Fatalf("sort order wrong:\n  %+v\n  %+v\n  %+v", got[0], got[1], got[2])
	}
}

func TestFindBusByClientID(t *testing.T) {
	dir := shortTempDir(t)
	makeBusDir(t, dir, "open", "aaaa", true, os.Getpid())
	if e := FindBusByClientID(dir, "open", "aaaa"); e == nil {
		t.Fatal("FindBusByClientID returned nil for existing entry")
	} else if e.State != BusStateRunning {
		t.Errorf("State = %s, want running", e.State)
	}
	if e := FindBusByClientID(dir, "open", "missing"); e != nil {
		t.Errorf("missing entry should return nil, got %+v", e)
	}
}

func TestQueryStatus_RealBusE2E(t *testing.T) {
	skipOnWindows(t)
	// Bring up a real bus daemon, then query it.
	workDir := shortTempDir(t)
	sock := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		runDone <- bus.Run(ctx, bus.Config{
			WorkDir:     workDir,
			IPCEndpoint: sock,
			ClientID:    "ding_test_query",
			Edition:     "open",
			Source:      &fakeSrc{},
		})
	}()
	defer func() { cancel(); <-runDone }()
	waitForSocket(t, sock, 2*time.Second)

	resp, err := QueryStatus(sock)
	if err != nil {
		t.Fatalf("QueryStatus: %v", err)
	}
	if resp.Bus.ClientID != "ding_test_query" {
		t.Errorf("ClientID round-trip = %q", resp.Bus.ClientID)
	}
	if resp.Bus.Edition != "open" {
		t.Errorf("Edition = %q", resp.Bus.Edition)
	}
	if resp.Bus.PID != os.Getpid() {
		t.Errorf("Bus.PID = %d, want %d", resp.Bus.PID, os.Getpid())
	}
}

// fakeSrc is a no-op SourceAdapter used by QueryStatus E2E. It just
// blocks on ctx so the bus daemon stays up long enough for the test to
// dial it.
type fakeSrc struct{}

func (fakeSrc) Start(ctx context.Context, _ dwsevent.EmitFn) error {
	<-ctx.Done()
	return ctx.Err()
}

// waitForSocket polls for the unix socket file. Reused by tests that
// need to dial a freshly-spawned bus.
func waitForSocket(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %q did not appear within %s", path, timeout)
}
