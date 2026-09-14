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

//go:build windows

package busctl

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// CREATE_NEW_PROCESS_GROUP (0x00000200) prevents the child from receiving
// the parent's Ctrl+C signal, similar in spirit to Setsid on Unix.
const createNewProcessGroup = 0x00000200

var (
	spawnWindowsDial         = transport.Dial
	spawnWindowsPollInterval = 25 * time.Millisecond
)

// Spawn starts a detached Windows bus without cmd.ExtraFiles (unsupported by
// Go on Windows). Readiness is confirmed by dialing the bus's existing named
// pipe. The child is reaped in the background after readiness, while an early
// process exit is surfaced as ErrSpawnFailed.
func Spawn(cfg SpawnConfig) (pid int, err error) {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return 0, fmt.Errorf("busctl: SpawnConfig.ClientID is required")
	}
	if strings.TrimSpace(cfg.IPCEndpoint) == "" {
		return 0, fmt.Errorf("busctl: SpawnConfig.IPCEndpoint is required on Windows")
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

	args := append([]string{"event", "_bus", "--client-id", cfg.ClientID}, cfg.ExtraArgs...)
	cmd := exec.Command(cfg.ExecPath, args...)
	cmd.Env = withoutReadyFDEnv(cfg.Env)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	applyDetach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("busctl: start %s: %w", cfg.ExecPath, err)
	}
	pid = cmd.Process.Pid

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	timer := time.NewTimer(ReadyTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(spawnWindowsPollInterval)
	defer ticker.Stop()

	for {
		if conn, dialErr := spawnWindowsDial(cfg.IPCEndpoint); dialErr == nil {
			_ = conn.Close()
			return pid, nil
		}
		select {
		case waitErr := <-waitDone:
			if waitErr == nil {
				return pid, fmt.Errorf("%w: child exited before binding named pipe", ErrSpawnFailed)
			}
			return pid, fmt.Errorf("%w: %v", ErrSpawnFailed, waitErr)
		case <-timer.C:
			_ = cmd.Process.Kill()
			select {
			case <-waitDone:
			case <-time.After(time.Second):
			}
			return pid, ErrSpawnTimeout
		case <-ticker.C:
		}
	}
}

func withoutReadyFDEnv(env []string) []string {
	prefix := strings.ToUpper(ReadyFDEnv) + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), prefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
		HideWindow:    true,
	}
}
