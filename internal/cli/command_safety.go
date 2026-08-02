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
// runtime from the embedded schema catalog. This is a read-only view over the
// catalog — NOT a second safety source. The catalog remains the single
// authoritative reviewed source; this struct merely provides typed access for
// consumers (help rendering, skill generation).
type CommandSafety struct {
	Effect       string // read / write / destructive
	Risk         string // low / medium / high
	Confirmation string // not_required / user_required
	Idempotency  string // idempotent / non_idempotent
}

// ShouldRender returns true when the safety metadata warrants a visible
// annotation in --help. Only commands that need confirmation or carry
// above-low risk are annotated; read-only low-risk commands stay clean.
func (s CommandSafety) ShouldRender() bool {
	return s.Confirmation == "user_required" ||
		(s.Risk != "" && s.Risk != "low")
}

// SafetyForCLIPath returns the safety metadata for a command identified by its
// CLI path (e.g. "dev app delete"). Returns ok=false when the command is not
// in the embedded catalog (utility commands, hidden commands, shortcuts).
//
// Deprecated: use ResolveMeta(cliPath).Safety for the complete metadata view.
// Kept for backward compatibility with existing callers.
func SafetyForCLIPath(cliPath string) (CommandSafety, bool) {
	meta, ok := ResolveMeta(cliPath)
	if !ok {
		return CommandSafety{}, false
	}
	return meta.Safety, true
}

// RenderSafetyAnnotation writes a "Safety:" line to the command's stdout when
// the command carries above-low risk or requires user confirmation. This is
// the shared entry point for ALL help rendering paths (root HelpFunc, product
// group custom HelpFuncs like calendar's). It avoids the timing issue where a
// group captures origHelp before configureRootHelp sets the root's custom func.
func RenderSafetyAnnotation(cmd *cobra.Command) {
	cliPath := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "))
	safety, ok := SafetyForCLIPath(cliPath)
	if !ok || !safety.ShouldRender() {
		return
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "\nSafety: effect=%s  risk=%s  confirmation=%s", safety.Effect, safety.Risk, safety.Confirmation)
	if safety.Idempotency != "" {
		fmt.Fprintf(w, "  idempotency=%s", safety.Idempotency)
	}
	if safety.Confirmation == "user_required" {
		fmt.Fprint(w, "  (requires --yes)")
	}
	fmt.Fprintln(w)
}
