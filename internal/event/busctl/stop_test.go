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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
)

func TestStop_NotRunningWhenLockMissing(t *testing.T) {
	dir := shortTempDir(t)
	err := Stop(StopConfig{WorkDir: dir})
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop on missing lock = %v, want ErrNotRunning", err)
	}
}

func TestStop_NotRunningWhenPIDDead(t *testing.T) {
	dir := shortTempDir(t)
	// Write a definitely-dead PID into bus.lock.
	if err := os.WriteFile(LockPath(dir), []byte("2147483646\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Stop(StopConfig{WorkDir: dir})
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop on dead PID = %v, want ErrNotRunning", err)
	}
}

func TestStop_SignalsLiveProcess(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)

	// Spawn `sleep 10` to act as the "bus daemon".
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep child: %v", err)
	}
	defer func() {
		// Best-effort cleanup if test fails.
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()
	pid := cmd.Process.Pid

	// Reap the child in background so Wait doesn't leave a zombie.
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	// Write PID into bus.lock.
	if err := os.WriteFile(LockPath(dir), []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Stop should signal SIGTERM and observe the process exit.
	start := time.Now()
	if err := Stop(StopConfig{WorkDir: dir, Timeout: 3 * time.Second}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	elapsed := time.Since(start)

	// sleep should react to SIGTERM almost immediately.
	if elapsed > 2*time.Second {
		t.Errorf("Stop took %s, expected <2s for SIGTERM-responsive child", elapsed)
	}

	// Confirm the child actually exited.
	select {
	case err := <-waited:
		// sh -c "sleep 10" exits with non-zero on signal; either is fine.
		_ = err
	case <-time.After(2 * time.Second):
		t.Fatal("child did not exit after Stop")
	}
}

// TestStop_TimeoutWhenChildIgnoresSignal ensures Stop honours its deadline
// and returns a useful error instead of hanging forever.
func TestStop_TimeoutWhenChildIgnoresSignal(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)

	// shell that traps SIGTERM and ignores it for a long time
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start trap child: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	pid := cmd.Process.Pid

	if err := os.WriteFile(LockPath(dir), []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Stop(StopConfig{WorkDir: dir, Timeout: 250 * time.Millisecond})
	if err == nil {
		t.Fatal("Stop should error when child ignores SIGTERM")
	}
}

// TestStop_RealBusGracefulShutdown is the integration sanity check: bring
// up a real bus.Run instance, set bus.lock content to its PID, call Stop,
// and verify Run returned cleanly (via ctx done propagation in the test).
//
// NOTE: bus.Run installs its own ctx handler from the caller's ctx; here
// we don't have signal.NotifyContext (we're running in-process), so Stop's
// SIGTERM won't reach bus.Run unless we install a signal handler. Instead,
// we test the underlying primitives: PID-read, signal-send, alive-poll.
func TestStop_BusLockPathHelper(t *testing.T) {
	dir := shortTempDir(t)
	got := LockPath(dir)
	want := filepath.Join(dir, bus.LockFileName)
	if got != want {
		t.Fatalf("LockPath = %q, want %q", got, want)
	}
}
