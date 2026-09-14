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
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageWindowsSpawnUsesNamedPipeReadiness(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\dws-event-spawn-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	pid, err := spawnWithMarker(t, "windows-ready", func(cfg *SpawnConfig) {
		cfg.IPCEndpoint = endpoint
		cfg.Env = append(cfg.Env, childEndpointEnv+"="+endpoint)
	})
	if err != nil {
		t.Fatalf("Spawn() = %v", err)
	}
	if pid <= 0 {
		t.Fatalf("Spawn() pid = %d", pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Kill()
}

func TestCrossPlatformCoverageWindowsSpawnReportsEarlyExit(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\dws-event-spawn-fail-%d-%d`, os.Getpid(), time.Now().UnixNano())
	_, err := spawnWithMarker(t, "windows-fail", func(cfg *SpawnConfig) {
		cfg.IPCEndpoint = endpoint
	})
	if !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("Spawn() error = %v, want ErrSpawnFailed", err)
	}
}

func TestCrossPlatformCoverageWindowsSpawnReportsCleanEarlyExit(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\dws-event-spawn-exit-%d-%d`, os.Getpid(), time.Now().UnixNano())
	_, err := spawnWithMarker(t, "windows-exit", func(cfg *SpawnConfig) {
		cfg.IPCEndpoint = endpoint
	})
	if !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("Spawn() error = %v, want ErrSpawnFailed", err)
	}
}

func TestCrossPlatformCoverageWindowsSpawnResolvesExecutable(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\dws-event-spawn-resolve-%d-%d`, os.Getpid(), time.Now().UnixNano())
	testseam.Swap(t, &spawnExecutable, func() (string, error) { return os.Args[0], nil })
	pid, err := spawnWithMarker(t, "windows-ready", func(cfg *SpawnConfig) {
		cfg.ExecPath = ""
		cfg.IPCEndpoint = endpoint
		cfg.Env = append(cfg.Env, childEndpointEnv+"="+endpoint)
	})
	if err != nil {
		t.Fatalf("Spawn() = %v", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	_ = proc.Kill()
}

func TestCrossPlatformCoverageWindowsSpawnValidationAndStartErrors(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\dws-event-spawn-errors-%d-%d`, os.Getpid(), time.Now().UnixNano())
	if _, err := Spawn(SpawnConfig{ClientID: "client"}); err == nil {
		t.Fatal("missing endpoint unexpectedly succeeded")
	}
	testseam.Swap(t, &spawnExecutable, func() (string, error) {
		return "", errors.New("executable unavailable")
	})
	if _, err := Spawn(SpawnConfig{ClientID: "client", IPCEndpoint: endpoint}); err == nil {
		t.Fatal("executable lookup unexpectedly succeeded")
	}
	if _, err := Spawn(SpawnConfig{
		ClientID:    "client",
		IPCEndpoint: endpoint,
		ExecPath:    `C:\\definitely-missing-dws.exe`,
	}); err == nil {
		t.Fatal("missing executable unexpectedly started")
	}
}

func TestCrossPlatformCoverageWindowsSpawnReadyTimeoutTerminatesChild(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\dws-event-spawn-stall-%d-%d`, os.Getpid(), time.Now().UnixNano())
	testseam.Swap(t, &ReadyTimeout, 100*time.Millisecond)
	_, err := spawnWithMarker(t, "windows-stall", func(cfg *SpawnConfig) {
		cfg.IPCEndpoint = endpoint
	})
	if !errors.Is(err, ErrSpawnTimeout) {
		t.Fatalf("Spawn() error = %v, want ErrSpawnTimeout", err)
	}
}

func TestCrossPlatformCoverageWindowsSpawnRemovesInheritedReadyFD(t *testing.T) {
	env := withoutReadyFDEnv([]string{
		"PATH=C:\\Windows",
		ReadyFDEnv + "=3",
		strings.ToLower(ReadyFDEnv) + "=4",
	})
	if len(env) != 1 || !strings.HasPrefix(env[0], "PATH=") {
		t.Fatalf("withoutReadyFDEnv() = %#v", env)
	}
}

func TestCrossPlatformCoverageWindowsUnixReadyPipeValidation(t *testing.T) {
	if _, err := spawnWithReadyPipe(SpawnConfig{}); err == nil {
		t.Fatal("spawnWithReadyPipe() unexpectedly accepted an empty client ID")
	}
}
