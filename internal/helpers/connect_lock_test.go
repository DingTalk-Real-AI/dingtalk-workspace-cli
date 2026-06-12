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
	"strings"
	"testing"
)

func withLockDir(t *testing.T) string {
	t.Helper()
	old := connectLockDir
	dir := t.TempDir()
	connectLockDir = dir
	t.Cleanup(func() { connectLockDir = old })
	return dir
}

// TestConnectLockBlocksSecond: a live lock (this test process's pid) must
// reject a second acquire for the same robot, with the dual-connection
// explanation in the error.
func TestConnectLockBlocksSecond(t *testing.T) {
	withLockDir(t)
	release, err := acquireConnectLock("ding-bot-1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()

	if _, err := acquireConnectLock("ding-bot-1"); err == nil {
		t.Fatal("second acquire succeeded, want rejection")
	} else if !strings.Contains(err.Error(), "已有连接器在本机运行") {
		t.Fatalf("error = %v, want dual-connection explanation", err)
	}

	// A different robot is unaffected.
	release2, err := acquireConnectLock("ding-bot-2")
	if err != nil {
		t.Fatalf("other robot acquire: %v", err)
	}
	release2()
}

// TestConnectLockReleaseAndReacquire: release removes the pid file and the
// same robot can be locked again.
func TestConnectLockReleaseAndReacquire(t *testing.T) {
	dir := withLockDir(t)
	release, err := acquireConnectLock("ding-bot-1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("lock file not removed: %v", entries)
	}
	release2, err := acquireConnectLock("ding-bot-1")
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	release2()
}

// TestConnectLockStaleTakeover: a lock owned by a dead pid is taken over.
func TestConnectLockStaleTakeover(t *testing.T) {
	dir := withLockDir(t)
	// pid far beyond any OS pid range — guaranteed not alive.
	stale := filepath.Join(dir, "dws-connect-ding-bot-1.pid")
	if err := os.WriteFile(stale, []byte("999999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := acquireConnectLock("ding-bot-1")
	if err != nil {
		t.Fatalf("stale takeover failed: %v", err)
	}
	release()
}

// TestConnectLockGarbageFile: an unreadable/garbage lock is treated as stale.
func TestConnectLockGarbageFile(t *testing.T) {
	dir := withLockDir(t)
	if err := os.WriteFile(filepath.Join(dir, "dws-connect-ding-bot-1.pid"), []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := acquireConnectLock("ding-bot-1")
	if err != nil {
		t.Fatalf("garbage lock takeover failed: %v", err)
	}
	release()
}
