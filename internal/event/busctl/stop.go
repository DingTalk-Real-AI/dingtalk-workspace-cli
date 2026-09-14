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
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/process"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// DefaultStopTimeout is the wall-clock budget Stop waits for each graceful or
// fallback termination phase. 5s covers the bus's own tear-down (broadcast
// Bye → consumer goroutines drain → cleanup) with margin.
const DefaultStopTimeout = 5 * time.Second

// ErrNotRunning indicates bus.lock either does not exist or its recorded
// PID is not alive. Stop returns this as a sentinel so the caller can
// distinguish "nothing to stop" from "failed to stop".
var ErrNotRunning = errors.New("busctl: bus is not running")

// ErrOwnerUnverified indicates the PID from bus.lock is alive but no longer
// owns that lock. Stop refuses to send a process-level signal in this state
// because the operating system may have reused the stale PID.
var ErrOwnerUnverified = errors.New("busctl: bus process ownership could not be verified")

var (
	stopReadHolderPID       = bus.ReadHolderPID
	stopValidateHolderOwner = bus.ValidateHolderOwnership
	stopAlive               = process.Alive
	stopFindProcess         = os.FindProcess
	stopSignalProcess       = func(proc *os.Process, signal os.Signal) error { return proc.Signal(signal) }
	stopDial                = transport.Dial
	stopRequest             = requestBusStop
	stopWaitForBusExit      = waitForBusExit
)

// StopConfig identifies the target bus and tunes timing.
type StopConfig struct {
	// WorkDir holds bus.lock; Stop reads the PID from there.
	WorkDir string
	// IPCEndpoint enables the cross-platform graceful stop RPC. Callers that
	// know the bus identity should always provide it. Empty preserves the
	// legacy signal-only fallback for compatibility and focused tests.
	IPCEndpoint string
	// Timeout is the wall-clock budget for graceful exit and, if required,
	// the subsequent platform termination fallback.
	Timeout time.Duration
}

// Stop signals the bus daemon for cfg.WorkDir to exit gracefully and waits
// for the process to actually die. Returns ErrNotRunning if no bus is
// running for that work dir.
//
// Stop first asks the bus to shut down through its owner-only IPC endpoint.
// This is the normal path on every platform and lets the daemon cancel its
// cloud source, notify consumers, and release its lock. If the endpoint is
// unavailable (for example an older bus) or graceful shutdown times out, the
// platform signal is used as a compatibility fallback: SIGTERM on Unix and
// TerminateProcess via os.Kill on Windows.
func Stop(cfg StopConfig) error {
	if cfg.WorkDir == "" {
		return errors.New("busctl: StopConfig.WorkDir is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultStopTimeout
	}

	pid := stopReadHolderPID(LockPath(cfg.WorkDir))
	if pid <= 0 {
		return ErrNotRunning
	}
	if !stopAlive(pid) {
		return ErrNotRunning
	}

	proc, err := stopFindProcess(pid)
	if err != nil {
		return fmt.Errorf("busctl: find process %d: %w", pid, err)
	}
	if strings.TrimSpace(cfg.IPCEndpoint) != "" {
		if err := stopRequest(cfg.IPCEndpoint); err == nil && stopWaitForBusExit(pid, cfg.Timeout) {
			return nil
		}
	}

	// The bus may finish shutting down immediately after the graceful wait
	// reaches its deadline (or after a failed IPC request). Recheck before
	// inspecting ownership so a completed stop is not reported as stale.
	if !stopAlive(pid) {
		return nil
	}
	owner, err := stopValidateHolderOwner(LockPath(cfg.WorkDir), pid)
	if err != nil {
		return fmt.Errorf("busctl: verify bus pid=%d ownership: %w", pid, err)
	}
	if !owner {
		// ValidateHolderOwnership observes a released lock both for a stale,
		// reused PID and for a bus that exits during the ownership probe. Only
		// the still-alive case is unsafe to signal.
		if !stopAlive(pid) {
			return nil
		}
		return fmt.Errorf("%w: pid=%d does not own %s; stale PID was not signalled", ErrOwnerUnverified, pid, LockPath(cfg.WorkDir))
	}

	if err := stopSignalProcess(proc, stopSignal()); err != nil {
		// On many Unix platforms Signal returns "process already finished"
		// when the bus has just exited on its own — treat that as success.
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("busctl: signal bus pid=%d: %w", pid, err)
	}

	if stopWaitForBusExit(pid, cfg.Timeout) {
		return nil
	}
	return fmt.Errorf("busctl: bus pid=%d did not exit within %s", pid, cfg.Timeout)
}

func requestBusStop(endpoint string) error {
	conn, err := stopDial(endpoint)
	if err != nil {
		return fmt.Errorf("busctl: dial bus for stop: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultStatusRPCTimeout))
	w := transport.NewWriter(conn)
	r := transport.NewReader(conn)
	if err := w.WriteJSON(transport.Hello{
		Type:        transport.FrameTypeHello,
		ConsumerPID: os.Getpid(),
		Role:        transport.HelloRoleStop,
	}); err != nil {
		return fmt.Errorf("busctl: write stop hello: %w", err)
	}
	var bye transport.Bye
	if err := r.ReadJSON(&bye); err != nil {
		return fmt.Errorf("busctl: read stop response: %w", err)
	}
	if bye.Type != transport.FrameTypeBye || bye.Reason != "stop_request" {
		return fmt.Errorf("busctl: unexpected stop response type=%q reason=%q", bye.Type, bye.Reason)
	}
	return nil
}

func waitForBusExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !stopAlive(pid) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !stopAlive(pid)
}
