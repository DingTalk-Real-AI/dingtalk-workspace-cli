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
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/process"
)

// DefaultStopTimeout is the wall-clock budget Stop waits for the bus to
// exit after the signal is sent. 5s covers the bus's own graceful tear-down
// (broadcast Bye → consumer goroutines drain → cleanup) with margin.
const DefaultStopTimeout = 5 * time.Second

// ErrNotRunning indicates bus.lock either does not exist or its recorded
// PID is not alive. Stop returns this as a sentinel so the caller can
// distinguish "nothing to stop" from "failed to stop".
var ErrNotRunning = errors.New("busctl: bus is not running")

// StopConfig identifies the target bus and tunes timing.
type StopConfig struct {
	// WorkDir holds bus.lock; Stop reads the PID from there.
	WorkDir string
	// Timeout is the total wall-clock budget for graceful exit. After this,
	// Stop returns an error; it does NOT escalate to SIGKILL — leave that
	// to the operator.
	Timeout time.Duration
}

// Stop signals the bus daemon for cfg.WorkDir to exit gracefully and waits
// for the process to actually die. Returns ErrNotRunning if no bus is
// running for that work dir.
//
// Implementation note: on Unix we send SIGTERM. The bus daemon's Run loop
// watches its parent ctx for cancellation; the cobra `event _bus` command
// wires signal.NotifyContext so SIGTERM triggers ctx.Done() → graceful
// shutdown path. On Windows we use os.Process.Signal(os.Interrupt) which
// the Go runtime maps to TerminateProcess for processes outside our
// console group; for v1 that's acceptable (Windows graceful shutdown is
// future work — plan §16 v2).
func Stop(cfg StopConfig) error {
	if cfg.WorkDir == "" {
		return errors.New("busctl: StopConfig.WorkDir is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultStopTimeout
	}

	pid := bus.ReadHolderPID(LockPath(cfg.WorkDir))
	if pid <= 0 {
		return ErrNotRunning
	}
	if !process.Alive(pid) {
		return ErrNotRunning
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("busctl: find process %d: %w", pid, err)
	}
	if err := proc.Signal(stopSignal()); err != nil {
		// On many Unix platforms Signal returns "process already finished"
		// when the bus has just exited on its own — treat that as success.
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("busctl: signal bus pid=%d: %w", pid, err)
	}

	// Poll for actual exit.
	deadline := time.Now().Add(cfg.Timeout)
	for time.Now().Before(deadline) {
		if !process.Alive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("busctl: bus pid=%d did not exit within %s", pid, cfg.Timeout)
}
