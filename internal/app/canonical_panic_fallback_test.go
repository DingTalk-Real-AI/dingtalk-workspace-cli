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
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/spf13/cobra"
)

// TestNewMCPCommandPanicDegradesToStub verifies the canonical-tree guard:
// the `dws mcp` build runs BEFORE the legacy build and used to sit outside
// every poisoned-cache guard, so a panic there (e.g. a tool schema property
// named after the reserved --params flag) aborted every invocation. With no
// on-disk cache to quarantine it must degrade to an inert stub instead.
func TestNewMCPCommandPanicDegradesToStub(t *testing.T) {
	t.Setenv(cli.CacheDirEnv, t.TempDir())

	calls := 0
	orig := buildMCPCommandFn
	buildMCPCommandFn = func(context.Context, cli.CatalogLoader, executor.Runner, *pipeline.Engine) *cobra.Command {
		calls++
		panic("chat_permission_grant flag redefined: params")
	}
	t.Cleanup(func() { buildMCPCommandFn = orig })

	var cmd *cobra.Command
	captured := captureStderr(t, func() {
		cmd = newMCPCommand(context.Background(), nil, nil, nil)
	})

	if cmd == nil || cmd.Name() != "mcp" {
		t.Fatalf("newMCPCommand() = %v after build panic, want an 'mcp' stub", cmd)
	}
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "dws cache refresh") {
		t.Errorf("stub RunE error = %v, want a 'dws cache refresh' hint", err)
	}
	if !strings.Contains(captured, "dws cache refresh") {
		t.Errorf("stderr = %q, want a hint mentioning 'dws cache refresh'", captured)
	}
	if calls != 1 {
		t.Errorf("canonical build attempts = %d, want 1 (no cache on disk, nothing to quarantine and retry)", calls)
	}
}

// TestNewMCPCommandSelfHealsPoisonedCache verifies the self-heal path: when
// the build panics AND a discovery cache exists on disk, the partition is
// quarantined and the build retried once, so a fixed binary escapes the
// lock-out with zero manual cache surgery.
func TestNewMCPCommandSelfHealsPoisonedCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(cli.CacheDirEnv, tmp)

	store := cache.NewStore(tmp)
	if err := store.SaveTools(editionPartition(), "poisoned-server", cache.ToolsSnapshot{ServerKey: "poisoned-server"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	calls := 0
	orig := buildMCPCommandFn
	buildMCPCommandFn = func(context.Context, cli.CatalogLoader, executor.Runner, *pipeline.Engine) *cobra.Command {
		calls++
		if calls == 1 {
			panic("chat_permission_grant flag redefined: params")
		}
		return &cobra.Command{Use: "mcp", Short: "rebuilt-probe"}
	}
	t.Cleanup(func() { buildMCPCommandFn = orig })

	var cmd *cobra.Command
	captured := captureStderr(t, func() {
		cmd = newMCPCommand(context.Background(), nil, nil, nil)
	})

	if calls != 2 {
		t.Fatalf("canonical build attempts = %d, want 2 (initial + retry after quarantine)", calls)
	}
	if cmd == nil || cmd.Short != "rebuilt-probe" {
		t.Errorf("newMCPCommand() did not return the rebuilt tree, got %v", cmd)
	}
	quarantines, _ := filepath.Glob(filepath.Join(tmp, "*.quarantined"))
	if len(quarantines) != 1 {
		t.Fatalf("quarantine dirs = %v, want exactly 1", quarantines)
	}
	if !strings.Contains(captured, "rebuilding from a fresh fetch") {
		t.Errorf("stderr = %q, want a note about rebuilding from a fresh fetch", captured)
	}
}

// TestNewMCPCommandSecondPanicDegradesToStub verifies the final safety net:
// if the rebuild after quarantine panics again, the stub is returned and the
// `dws cache refresh` hint kept.
func TestNewMCPCommandSecondPanicDegradesToStub(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(cli.CacheDirEnv, tmp)

	store := cache.NewStore(tmp)
	if err := store.SaveTools(editionPartition(), "poisoned-server", cache.ToolsSnapshot{ServerKey: "poisoned-server"}); err != nil {
		t.Fatalf("SaveTools() error = %v", err)
	}

	calls := 0
	orig := buildMCPCommandFn
	buildMCPCommandFn = func(context.Context, cli.CatalogLoader, executor.Runner, *pipeline.Engine) *cobra.Command {
		calls++
		panic("chat_permission_grant flag redefined: params")
	}
	t.Cleanup(func() { buildMCPCommandFn = orig })

	var cmd *cobra.Command
	captured := captureStderr(t, func() {
		cmd = newMCPCommand(context.Background(), nil, nil, nil)
	})

	if calls != 2 {
		t.Fatalf("canonical build attempts = %d, want 2 (initial + retry after quarantine)", calls)
	}
	if cmd == nil || cmd.Name() != "mcp" {
		t.Fatalf("newMCPCommand() = %v after repeated panics, want an 'mcp' stub", cmd)
	}
	if !strings.Contains(captured, "dws cache refresh") {
		t.Errorf("stderr = %q, want a hint mentioning 'dws cache refresh'", captured)
	}
}

// TestNewMCPCommandNoPanicKeepsCanonicalPath ensures the guard is transparent
// on the happy path.
func TestNewMCPCommandNoPanicKeepsCanonicalPath(t *testing.T) {
	orig := buildMCPCommandFn
	buildMCPCommandFn = func(context.Context, cli.CatalogLoader, executor.Runner, *pipeline.Engine) *cobra.Command {
		return &cobra.Command{Use: "mcp", Short: "canonical-probe"}
	}
	t.Cleanup(func() { buildMCPCommandFn = orig })

	cmd := newMCPCommand(context.Background(), nil, nil, nil)
	if cmd == nil || cmd.Short != "canonical-probe" {
		t.Errorf("newMCPCommand() lost the canonical command, got %v", cmd)
	}
}
