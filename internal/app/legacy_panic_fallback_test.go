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

package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// captureStderr redirects os.Stderr for the duration of fn and returns what
// was written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = pipeW
	defer func() { os.Stderr = origStderr }()

	fn()

	_ = pipeW.Close()
	os.Stderr = origStderr
	captured, _ := io.ReadAll(pipeR)
	return string(captured)
}

// TestNewLegacyPublicCommandsPanicFallsBackToHelpers verifies the escape
// hatch for a poisoned discovery cache: when the dynamic command build
// panics (e.g. duplicate pflag registration, the pre-1.0.32 lock-out
// "flag redefined: params"), newLegacyPublicCommands must NOT propagate
// the panic. With no on-disk cache to quarantine there is nothing to
// self-heal from, so it degrades to the hardcoded helper commands and
// prints a stderr hint pointing at `dws cache refresh`.
func TestNewLegacyPublicCommandsPanicFallsBackToHelpers(t *testing.T) {
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	calls := 0
	orig := loadDynamicCommandsFn
	loadDynamicCommandsFn = func(context.Context, executor.Runner) []*cobra.Command {
		calls++
		panic("chat_permission_grant flag redefined: params")
	}
	t.Cleanup(func() { loadDynamicCommandsFn = orig })

	var cmds []*cobra.Command
	captured := captureStderr(t, func() {
		cmds = newLegacyPublicCommands(context.Background(), nil)
	})

	if len(cmds) == 0 {
		t.Fatalf("newLegacyPublicCommands() = 0 commands after build panic, want helper fallback set")
	}
	if !strings.Contains(captured, "dws cache refresh") {
		t.Errorf("stderr = %q, want a hint mentioning 'dws cache refresh'", captured)
	}
	if calls != 1 {
		t.Errorf("dynamic build attempts = %d, want 1 (no cache on disk, nothing to quarantine and retry)", calls)
	}
}

// TestNewLegacyPublicCommandsSelfHealsPoisonedCache verifies the self-heal
// path: when the build panics AND a discovery cache exists on disk, the
// partition is quarantined (moved aside, kept for inspection) and the build
// retried once. The retry succeeding means the user gets the full dynamic
// command tree with zero manual cache surgery.
func TestNewLegacyPublicCommandsSelfHealsPoisonedCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(cli.CacheDirEnv, tmp)

	store := cache.NewStore(tmp)
	partition := editionPartition()
	if err := store.SaveTools(partition, "poisoned-server", cache.ToolsSnapshot{ServerKey: "poisoned-server"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	calls := 0
	orig := loadDynamicCommandsFn
	loadDynamicCommandsFn = func(context.Context, executor.Runner) []*cobra.Command {
		calls++
		if calls == 1 {
			panic("chat_permission_grant flag redefined: params")
		}
		return []*cobra.Command{{Use: "dynamic-probe"}}
	}
	t.Cleanup(func() { loadDynamicCommandsFn = orig })

	var cmds []*cobra.Command
	captured := captureStderr(t, func() {
		cmds = newLegacyPublicCommands(context.Background(), nil)
	})

	if calls != 2 {
		t.Fatalf("dynamic build attempts = %d, want 2 (initial + retry after quarantine)", calls)
	}
	found := false
	for _, c := range cmds {
		if c.Name() == "dynamic-probe" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("newLegacyPublicCommands() did not return the rebuilt dynamic command tree; got %d commands without 'dynamic-probe'", len(cmds))
	}

	quarantines, _ := filepath.Glob(filepath.Join(tmp, "*.quarantined"))
	if len(quarantines) != 1 {
		t.Fatalf("quarantine dirs = %v, want exactly 1", quarantines)
	}
	if _, err := os.Stat(filepath.Join(quarantines[0], "tools", "poisoned-server.json")); err != nil {
		t.Errorf("poisoned snapshot not preserved in quarantine: %v", err)
	}
	if !strings.Contains(captured, "rebuilding from a fresh fetch") {
		t.Errorf("stderr = %q, want a note about rebuilding from a fresh fetch", captured)
	}
	if strings.Contains(captured, "dws cache refresh") {
		t.Errorf("stderr = %q, must not tell the user to run 'dws cache refresh' when the rebuild succeeded", captured)
	}
}

// TestNewLegacyPublicCommandsSecondPanicDegradesToHelpers verifies the final
// safety net: if the rebuild after quarantine panics again (remote envelope
// still poisoned, or offline), the CLI degrades to helper commands and keeps
// the `dws cache refresh` hint.
func TestNewLegacyPublicCommandsSecondPanicDegradesToHelpers(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(cli.CacheDirEnv, tmp)

	store := cache.NewStore(tmp)
	if err := store.SaveTools(editionPartition(), "poisoned-server", cache.ToolsSnapshot{ServerKey: "poisoned-server"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	calls := 0
	orig := loadDynamicCommandsFn
	loadDynamicCommandsFn = func(context.Context, executor.Runner) []*cobra.Command {
		calls++
		panic("chat_permission_grant flag redefined: params")
	}
	t.Cleanup(func() { loadDynamicCommandsFn = orig })

	var cmds []*cobra.Command
	captured := captureStderr(t, func() {
		cmds = newLegacyPublicCommands(context.Background(), nil)
	})

	if calls != 2 {
		t.Fatalf("dynamic build attempts = %d, want 2 (initial + retry after quarantine)", calls)
	}
	if len(cmds) == 0 {
		t.Fatalf("newLegacyPublicCommands() = 0 commands after repeated build panics, want helper fallback set")
	}
	if !strings.Contains(captured, "dws cache refresh") {
		t.Errorf("stderr = %q, want a hint mentioning 'dws cache refresh'", captured)
	}
}

// TestNewLegacyPublicCommandsNoPanicKeepsDynamicPath ensures the guard is
// transparent on the happy path: commands returned by the dynamic build
// still reach the caller unchanged.
func TestNewLegacyPublicCommandsNoPanicKeepsDynamicPath(t *testing.T) {
	orig := loadDynamicCommandsFn
	loadDynamicCommandsFn = func(context.Context, executor.Runner) []*cobra.Command {
		return []*cobra.Command{{Use: "dynamic-probe"}}
	}
	t.Cleanup(func() { loadDynamicCommandsFn = orig })

	cmds := newLegacyPublicCommands(context.Background(), nil)

	found := false
	for _, c := range cmds {
		if c.Name() == "dynamic-probe" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("newLegacyPublicCommands() lost the dynamic command; got %d commands without 'dynamic-probe'", len(cmds))
	}
}
