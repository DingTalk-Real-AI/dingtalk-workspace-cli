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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestStop_NotRunningWhenLockMissing(t *testing.T) {
	dir := shortTempDir(t)
	err := Stop(StopConfig{WorkDir: dir})
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop on missing lock = %v, want ErrNotRunning", err)
	}
}

func TestCrossPlatformCoverageStopNotRunningWhenPIDDead(t *testing.T) {
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

func TestCrossPlatformCoverageStopPrefersGracefulIPC(t *testing.T) {
	const pid = 4242
	stopped := false
	signals := 0
	testseam.Swap(t, &stopReadHolderPID, func(string) int { return pid })
	testseam.Swap(t, &stopAlive, func(int) bool { return !stopped })
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &stopFindProcess, func(int) (*os.Process, error) { return proc, nil })
	testseam.Swap(t, &stopRequest, func(endpoint string) error {
		if endpoint != "test-endpoint" {
			t.Fatalf("stop endpoint = %q", endpoint)
		}
		stopped = true
		return nil
	})
	testseam.Swap(t, &stopSignalProcess, func(*os.Process, os.Signal) error {
		signals++
		return nil
	})

	if err := Stop(StopConfig{
		WorkDir:     "test-workdir",
		IPCEndpoint: "test-endpoint",
		Timeout:     time.Second,
	}); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if signals != 0 {
		t.Fatalf("graceful IPC stop used %d process signals", signals)
	}
}

func TestCrossPlatformCoverageStopDoesNotSignalUnverifiedReusedPID(t *testing.T) {
	const pid = 4242
	testseam.Swap(t, &stopReadHolderPID, func(string) int { return pid })
	testseam.Swap(t, &stopAlive, func(int) bool { return true })
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &stopFindProcess, func(int) (*os.Process, error) { return proc, nil })
	testseam.Swap(t, &stopRequest, func(string) error { return errors.New("stale endpoint") })
	testseam.Swap(t, &stopValidateHolderOwner, func(path string, gotPID int) (bool, error) {
		if path != LockPath("test-workdir") || gotPID != pid {
			t.Fatalf("ownership check path=%q pid=%d", path, gotPID)
		}
		return false, nil
	})
	signals := 0
	testseam.Swap(t, &stopSignalProcess, func(*os.Process, os.Signal) error {
		signals++
		return nil
	})

	err = Stop(StopConfig{
		WorkDir:     "test-workdir",
		IPCEndpoint: "stale-endpoint",
		Timeout:     time.Millisecond,
	})
	if !errors.Is(err, ErrOwnerUnverified) {
		t.Fatalf("Stop() error = %v, want ErrOwnerUnverified", err)
	}
	if signals != 0 {
		t.Fatalf("unverified reused PID received %d signals", signals)
	}
}

func TestCrossPlatformCoverageStopAcceptsExitAtGracefulTimeoutBoundary(t *testing.T) {
	const pid = 4242

	t.Run("before ownership validation", func(t *testing.T) {
		exited := false
		validated := false
		testseam.Swap(t, &stopReadHolderPID, func(string) int { return pid })
		testseam.Swap(t, &stopAlive, func(int) bool { return !exited })
		proc, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &stopFindProcess, func(int) (*os.Process, error) { return proc, nil })
		testseam.Swap(t, &stopRequest, func(string) error { return nil })
		testseam.Swap(t, &stopWaitForBusExit, func(int, time.Duration) bool {
			exited = true
			return false
		})
		testseam.Swap(t, &stopValidateHolderOwner, func(string, int) (bool, error) {
			validated = true
			return false, nil
		})

		if err := Stop(StopConfig{WorkDir: "test-workdir", IPCEndpoint: "test-endpoint"}); err != nil {
			t.Fatalf("Stop() = %v, want success after bus exit", err)
		}
		if validated {
			t.Fatal("ownership was validated after the bus had already exited")
		}
	})

	t.Run("during ownership validation", func(t *testing.T) {
		exited := false
		signals := 0
		testseam.Swap(t, &stopReadHolderPID, func(string) int { return pid })
		testseam.Swap(t, &stopAlive, func(int) bool { return !exited })
		proc, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &stopFindProcess, func(int) (*os.Process, error) { return proc, nil })
		testseam.Swap(t, &stopRequest, func(string) error { return nil })
		testseam.Swap(t, &stopWaitForBusExit, func(int, time.Duration) bool { return false })
		testseam.Swap(t, &stopValidateHolderOwner, func(string, int) (bool, error) {
			exited = true
			return false, nil
		})
		testseam.Swap(t, &stopSignalProcess, func(*os.Process, os.Signal) error {
			signals++
			return nil
		})

		if err := Stop(StopConfig{WorkDir: "test-workdir", IPCEndpoint: "test-endpoint"}); err != nil {
			t.Fatalf("Stop() = %v, want success after exit during ownership validation", err)
		}
		if signals != 0 {
			t.Fatalf("exited bus received %d fallback signals", signals)
		}
	})
}

func TestCrossPlatformCoverageStopReportsOwnershipValidationError(t *testing.T) {
	const pid = 4242
	errInjected := errors.New("ownership validation failed")
	testseam.Swap(t, &stopReadHolderPID, func(string) int { return pid })
	testseam.Swap(t, &stopAlive, func(int) bool { return true })
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &stopFindProcess, func(int) (*os.Process, error) { return proc, nil })
	testseam.Swap(t, &stopValidateHolderOwner, func(string, int) (bool, error) {
		return false, errInjected
	})
	signals := 0
	testseam.Swap(t, &stopSignalProcess, func(*os.Process, os.Signal) error {
		signals++
		return nil
	})

	err = Stop(StopConfig{WorkDir: "test-workdir"})
	if !errors.Is(err, errInjected) {
		t.Fatalf("Stop() error = %v, want injected ownership error", err)
	}
	if signals != 0 {
		t.Fatalf("ownership validation error sent %d signals", signals)
	}
}

func TestCrossPlatformCoverageRequestBusStopProtocol(t *testing.T) {
	dir := shortTempDir(t)
	endpoint := dwsevent.IPCEndpoint(
		dir,
		"open",
		dwsevent.SourceKindAppStream,
		dwsevent.IdentityHash(dir),
	)
	listener, err := transport.Listen(endpoint)
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		r := transport.NewReader(conn)
		w := transport.NewWriter(conn)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			serverDone <- err
			return
		}
		if hello.Type != transport.FrameTypeHello || hello.Role != transport.HelloRoleStop {
			serverDone <- errors.New("unexpected stop hello")
			return
		}
		serverDone <- w.WriteJSON(transport.Bye{
			Type:   transport.FrameTypeBye,
			Reason: "stop_request",
		})
	}()

	if err := requestBusStop(endpoint); err != nil {
		t.Fatalf("requestBusStop() = %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("stop protocol server = %v", err)
	}
}

func TestCrossPlatformCoverageRequestBusStopErrors(t *testing.T) {
	t.Run("dial", func(t *testing.T) {
		testseam.Swap(t, &stopDial, func(string) (net.Conn, error) {
			return nil, errors.New("dial failed")
		})
		if err := requestBusStop("test-endpoint"); err == nil {
			t.Fatal("requestBusStop() unexpectedly succeeded")
		}
	})

	t.Run("write", func(t *testing.T) {
		testseam.Swap(t, &stopDial, func(string) (net.Conn, error) {
			return &queryErrorConn{failAt: 1}, nil
		})
		if err := requestBusStop("test-endpoint"); err == nil {
			t.Fatal("requestBusStop() unexpectedly succeeded")
		}
	})

	t.Run("read", func(t *testing.T) {
		testseam.Swap(t, &stopDial, func(string) (net.Conn, error) {
			return &queryErrorConn{}, nil
		})
		if err := requestBusStop("test-endpoint"); err == nil {
			t.Fatal("requestBusStop() unexpectedly succeeded")
		}
	})

	t.Run("unexpected response", func(t *testing.T) {
		client, server := net.Pipe()
		t.Cleanup(func() {
			_ = client.Close()
			_ = server.Close()
		})
		testseam.Swap(t, &stopDial, func(string) (net.Conn, error) {
			return client, nil
		})
		serverDone := make(chan error, 1)
		go func() {
			var hello transport.Hello
			if err := transport.NewReader(server).ReadJSON(&hello); err != nil {
				serverDone <- err
				return
			}
			serverDone <- transport.NewWriter(server).WriteJSON(transport.Bye{
				Type:   transport.FrameTypeBye,
				Reason: "unexpected",
			})
		}()
		if err := requestBusStop("test-endpoint"); err == nil {
			t.Fatal("requestBusStop() unexpectedly succeeded")
		}
		if err := <-serverDone; err != nil {
			t.Fatalf("stop protocol server = %v", err)
		}
	})
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
	testseam.Swap(t, &stopValidateHolderOwner, func(string, int) (bool, error) { return true, nil })

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
	testseam.Swap(t, &stopValidateHolderOwner, func(string, int) (bool, error) { return true, nil })

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
