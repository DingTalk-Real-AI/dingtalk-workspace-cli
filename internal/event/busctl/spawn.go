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
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var busctlReadFull = io.ReadFull

// ReadyFDEnv is the env var the spawned `event _bus` child inspects to find
// the ready-pipe write end. The parent passes the FD number; child opens it
// via os.NewFile(fd, "ready") and writes 'R' on success or 'E' on failure.
// 3 is the first FD slot beyond stdio in cmd.ExtraFiles.
const ReadyFDEnv = "DWS_EVENT_BUS_READY_FD"

// ReadyTimeout caps how long Spawn waits for the child to signal readiness.
// 10s is generous — bus startup is local-only work (file I/O + socket bind),
// so 1s would normally suffice; the extra headroom covers cold-start
// keychain prompts and slow CI machines.
var ReadyTimeout = 10 * time.Second

var (
	spawnExecutable = os.Executable
	spawnPipe       = os.Pipe
)

// ErrSpawnFailed is returned when the child reports startup failure via
// the ready pipe ('E' byte). The child's exit error / log file holds the
// actual cause; this sentinel just lets the caller distinguish "ready
// pipe said no" from "ready pipe timed out / closed early".
var ErrSpawnFailed = errors.New("busctl: bus child reported startup failure on ready pipe")

// ErrSpawnTimeout is returned when ReadyTimeout elapses without any signal.
var ErrSpawnTimeout = errors.New("busctl: bus child did not signal readiness within deadline")

// SpawnConfig describes one spawn attempt. ClientID is the only field
// inspected by the child; the rest govern process attributes the parent
// applies before exec.
type SpawnConfig struct {
	// ExecPath is the dws binary to exec. Default os.Executable().
	ExecPath string

	// ClientID is passed as `--client-id` to `dws event _bus`. Required.
	ClientID string

	// ExtraArgs are appended after `--client-id`. Empty for normal use; tests
	// pass `--extra-flag-for-test` etc.
	ExtraArgs []string

	// Env to pass to the child. Defaults to os.Environ(). The ReadyFDEnv
	// entry is appended automatically.
	Env []string
}

// Spawn forks a detached `dws event _bus --client-id <id>` child process and
// waits for it to signal readiness via the ready pipe. Returns the child's
// PID on success — the caller can then dial the bus IPC endpoint.
//
// stdio detach (plan invariant #7):
//   - cmd.Stdout / cmd.Stderr set to nil so the child's own writes don't
//     pollute the parent's NDJSON stream
//   - Setsid on Unix so the child survives parent SIGHUP / parent exit
//   - CREATE_NEW_PROCESS_GROUP on Windows (set in spawn_windows.go)
//
// Child startup (handled by the eventcmd._bus handler, P6):
//   - Opens os.NewFile(<DWS_EVENT_BUS_READY_FD>, "ready")
//   - On startup success → writes 'R' and closes
//   - On startup failure  → writes 'E' and closes (child exits)
//
// Parent (this function):
//   - Holds the read end open until either 1 byte is read or ReadyTimeout
//   - Returns ErrSpawnFailed for 'E', ErrSpawnTimeout otherwise
func Spawn(cfg SpawnConfig) (pid int, err error) {
	if cfg.ClientID == "" {
		return 0, errors.New("busctl: SpawnConfig.ClientID is required")
	}
	if cfg.ExecPath == "" {
		execPath, err := spawnExecutable()
		if err != nil {
			return 0, fmt.Errorf("busctl: locate executable: %w", err)
		}
		cfg.ExecPath = execPath
	}
	if cfg.Env == nil {
		cfg.Env = os.Environ()
	}

	pr, pw, err := spawnPipe()
	if err != nil {
		return 0, fmt.Errorf("busctl: pipe: %w", err)
	}
	defer pr.Close()
	// pw is passed to the child; close in parent after Start so only the
	// child holds the write end (so reads return EOF if child dies before
	// signalling, helping us distinguish death from slow startup).

	args := append([]string{"event", "_bus", "--client-id", cfg.ClientID}, cfg.ExtraArgs...)
	cmd := exec.Command(cfg.ExecPath, args...)
	cmd.Env = append(cfg.Env, ReadyFDEnv+"=3")
	cmd.ExtraFiles = []*os.File{pw} // child sees fd 3 = pw
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	applyDetach(cmd) // platform-specific Setsid / new process group

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return 0, fmt.Errorf("busctl: start %s: %w", cfg.ExecPath, err)
	}
	pid = cmd.Process.Pid

	// Close parent's copy of the write end immediately. Now only the child
	// holds it; reading on pr will return EOF when the child exits without
	// signalling, instead of blocking forever.
	_ = pw.Close()

	// The detached child owns its own process group. Wait in the background to
	// release its process resources; the caller only retains the numeric PID.
	go func() { _ = cmd.Wait() }()

	// Wait for ready byte.
	if err := waitReady(pr); err != nil {
		return pid, err
	}
	return pid, nil
}

// waitReady reads the child's ready signal within ReadyTimeout. The first
// byte is 'R' (ready) or 'E' (error); on 'E' the child may write its real
// error text after the byte and close the pipe, which we surface so the
// caller sees WHY the bus failed to start (instead of an opaque
// ErrSpawnFailed). pr is closed by the caller on return.
func waitReady(pr *os.File) error {
	type result struct {
		b   byte
		msg string
		err error
	}
	done := make(chan result, 1)
	go func() {
		buf := make([]byte, 1)
		n, err := busctlReadFull(pr, buf)
		if err != nil {
			done <- result{err: err}
			return
		}
		if n != 1 {
			done <- result{err: io.ErrUnexpectedEOF}
			return
		}
		if buf[0] == 'E' {
			// Failure: read the trailing error text (bounded), which the
			// child writes right after 'E' before closing.
			rest, _ := io.ReadAll(io.LimitReader(pr, 4096))
			done <- result{b: 'E', msg: strings.TrimSpace(string(rest))}
			return
		}
		done <- result{b: buf[0]}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			if errors.Is(res.err, io.EOF) || errors.Is(res.err, io.ErrUnexpectedEOF) {
				return ErrSpawnFailed // child closed pipe without writing
			}
			return fmt.Errorf("busctl: read ready pipe: %w", res.err)
		}
		switch res.b {
		case 'R':
			return nil
		case 'E':
			if res.msg != "" {
				return fmt.Errorf("%w: %s", ErrSpawnFailed, res.msg)
			}
			return ErrSpawnFailed
		default:
			return fmt.Errorf("busctl: unexpected ready byte %q", res.b)
		}
	case <-time.After(ReadyTimeout):
		return ErrSpawnTimeout
	}
}

// ReadyFDFromEnv returns the inherited ready pipe (or nil if not set). The
// `event _bus` command handler calls this at startup, passes the returned
// *os.File to bus.Run as Config.ReadyPipe, and the bus signals readiness
// through it.
func ReadyFDFromEnv() *os.File {
	v := os.Getenv(ReadyFDEnv)
	if v == "" {
		return nil
	}
	fd, err := strconv.Atoi(v)
	if err != nil || fd < 3 {
		return nil
	}
	return os.NewFile(uintptr(fd), "dws-bus-ready")
}
