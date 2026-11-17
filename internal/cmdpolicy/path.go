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

package cmdpolicy

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/cobra"
)

// Policy paths are slash-separated command paths without the "dws" root,
// lower case. This file is the single place that converts Cobra nodes and
// authored space-separated CLI paths into that form.

// CommandPath returns the policy path of a live Cobra node. The root itself
// has the empty path.
func CommandPath(cmd *cobra.Command) string {
	if cmd == nil || !cmd.HasParent() {
		return ""
	}
	parts := make([]string, 0, 4)
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		parts = append(parts, c.Name())
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

// DeclaredPath converts an authored space-separated CLI path (as written in
// ContractFinal.Identity) into a policy path.
func DeclaredPath(cliPath string) string {
	normalized := schemaruntime.NormalizeCLIPath(cliPath)
	if normalized == "" {
		return ""
	}
	return strings.ReplaceAll(normalized, " ", "/")
}

// DisplayPath renders a policy path as the command a user would type.
func DisplayPath(path string) string {
	if path == "" {
		return "dws"
	}
	return "dws " + strings.ReplaceAll(path, "/", " ")
}

// validateRulePath enforces the path contract on one authored rule entry.
// Globs are validated up front so matching never sees a malformed pattern.
func validateRulePath(value string, glob bool) error {
	if value == "" {
		return fmt.Errorf("path is empty")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("path %q contains whitespace; use \"/\" between command names", value)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("path %q must be lower case", value)
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("path %q has an empty segment", value)
		}
	}
	if segments[0] == "dws" {
		return fmt.Errorf("path %q must not include the \"dws\" root", value)
	}
	if glob && !doublestar.ValidatePattern(value) {
		return fmt.Errorf("path %q is not a valid glob", value)
	}
	if !glob && strings.ContainsAny(value, "*?[]{}\\") {
		return fmt.Errorf("path %q must be an exact command path, not a glob", value)
	}
	return nil
}

// matchAny reports the first glob matching path. Patterns were validated at
// load time, so a match error cannot occur here; it is treated as no match.
func matchAny(globs []string, path string) (string, bool) {
	for _, glob := range globs {
		if ok, err := doublestar.Match(glob, path); err == nil && ok {
			return glob, true
		}
	}
	return "", false
}
