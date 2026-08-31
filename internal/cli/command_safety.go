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

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// CommandSafety holds the safety metadata for a CLI command, resolved at
// runtime via ResolveMeta. This is a read-only view — NOT a second safety
// source. Production Meta is projected from the runtime-assembled
// SchemaRegistry into a cached map[cli_path]CommandMeta during
// deliverySchemaCatalog's sync.Once; steady-state ResolveMeta / leaf --help
// are O(1) map lookups (同源 with assembly, not a separate gob authority).
type CommandSafety struct {
	Effect       string // read / write / destructive
	Risk         string // low / medium / high
	Confirmation string // not_required / user_required
	Idempotency  string // idempotent / retryable / non_idempotent / unknown
}

// ShouldRender returns true when any reviewed safety metadata is available.
// Agent-visible leaves render the full tuple even for read/low operations so
// absence is never mistaken for an unknown or unsafe command.
func (s CommandSafety) ShouldRender() bool {
	return strings.TrimSpace(s.Effect) != "" ||
		strings.TrimSpace(s.Risk) != "" ||
		strings.TrimSpace(s.Confirmation) != "" ||
		strings.TrimSpace(s.Idempotency) != ""
}

// SafetyForCLIPath returns the safety metadata for a command identified by its
// CLI path (e.g. "dev app delete"). Returns ok=false when the path is absent
// from the Schema surface (utility commands, hidden commands, shortcuts), or
// when no Schema source root is registered (synthetic help trees in unit
// tests). With a registered factory, assembly failure panics via ResolveMeta
// (fail-closed).
//
// Deprecated: use ResolveMeta(cliPath).Safety for the complete metadata view.
// Kept for backward compatibility with existing callers.
func SafetyForCLIPath(cliPath string) (CommandSafety, bool) {
	if !SchemaSourceRootRegistered() {
		return CommandSafety{}, false
	}
	meta, ok := ResolveMeta(cliPath)
	if !ok {
		return CommandSafety{}, false
	}
	return meta.Safety, true
}

// RenderSafetyAnnotation writes the reviewed Safety tuple to the command's
// stdout. Prefer RenderHelpAffordances for production Help; this focused entry
// point remains for compatibility and direct safety tests.
func RenderSafetyAnnotation(cmd *cobra.Command) {
	if !SchemaSourceRootRegistered() {
		return
	}
	cliPath := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "))
	safety, ok := SafetyForCLIPath(cliPath)
	if !ok || !safety.ShouldRender() {
		return
	}
	renderSafety(cmd, safety)
}

func renderSafety(cmd *cobra.Command, safety CommandSafety) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "\nSafety: effect=%s  risk=%s  confirmation=%s", safety.Effect, safety.Risk, safety.Confirmation)
	if safety.Idempotency != "" {
		fmt.Fprintf(w, "  idempotency=%s", safety.Idempotency)
	}
	if safety.Confirmation == "user_required" {
		fmt.Fprint(w, "\n  Do not use --yes until the user explicitly confirms this operation.")
	}
	fmt.Fprintln(w)
}
