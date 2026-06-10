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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// TestNewLegacyPublicCommandsPanicFallsBackToHelpers verifies the escape
// hatch for a poisoned discovery cache: when the dynamic command build
// panics (e.g. duplicate pflag registration, the pre-1.0.32 lock-out
// "flag redefined: params"), newLegacyPublicCommands must NOT propagate
// the panic. Instead it degrades to the hardcoded helper commands and
// prints a stderr hint pointing at `dws cache refresh`, so the CLI stays
// usable and the cache can be repaired without manual file deletion.
func TestNewLegacyPublicCommandsPanicFallsBackToHelpers(t *testing.T) {
	orig := loadDynamicCommandsFn
	loadDynamicCommandsFn = func(context.Context, executor.Runner) []*cobra.Command {
		panic("chat_permission_grant flag redefined: params")
	}
	t.Cleanup(func() { loadDynamicCommandsFn = orig })

	// Capture stderr to assert the self-heal hint.
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = pipeW
	t.Cleanup(func() { os.Stderr = origStderr })

	cmds := newLegacyPublicCommands(context.Background(), nil)

	_ = pipeW.Close()
	os.Stderr = origStderr
	captured, _ := io.ReadAll(pipeR)

	if len(cmds) == 0 {
		t.Fatalf("newLegacyPublicCommands() = 0 commands after build panic, want helper fallback set")
	}
	if !strings.Contains(string(captured), "dws cache refresh") {
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
