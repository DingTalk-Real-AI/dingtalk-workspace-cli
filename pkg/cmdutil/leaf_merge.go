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

package cmdutil

import (
	"log/slog"

	"github.com/spf13/cobra"
)

// MergeHardcodedLeaves grafts leaves from hardcodedRoot onto dynamicRoot when
// the same-named path does not already exist. Groups recurse. On leaf
// conflicts the dynamic side wins by default, because the discovery envelope
// is the runtime authority — hardcoded commands are retained only as a
// leaf-level fallback for behaviour the envelope explicitly does not
// declare. See _docs/discovery-overlay-authority.md.
//
// Explicit opt-in override: a hardcoded leaf may promote itself over a
// same-named dynamic leaf by carrying a strictly higher OverridePriority
// (see SetOverridePriority / OverridePriority). This exists for the narrow
// case where the envelope exposes one dispatch path but the hardcoded leaf
// needs richer flag-based routing (e.g. `chat message send` fanning out to
// multiple MCP tools depending on --group vs --user), which the envelope
// cannot currently express. Helpers without the annotation still lose to
// the envelope, so the default authority contract is preserved.
//
// PRECONDITION: dynamicRoot must be envelope-sourced (carry the
// SourceAnnotation=SourceEnvelope marker set by BuildDynamicCommands via
// MarkEnvelopeSource). Callers that might otherwise pass a helper-fallback
// root with the same name are responsible for evicting it upstream —
// otherwise the "envelope is authority" rule silently promotes helper leaves
// over same-named hardcoded leaves and the overlay loses its ability to
// override routing. The wukong overlay's RegisterProducts gates this call on
// IsEnvelopeSourced(dynamicRoot); new callers must do the same.
//
// Conflict resolution table:
//
//	dynamic  hardcoded                          →  action
//	-------  ---------------------------------  -----------------------------
//	absent   anything                              graft hardcoded subtree
//	leaf     leaf (hc priority ≤ dyn)              dynamic wins (no-op)
//	leaf     leaf (hc priority > dyn)              hardcoded replaces dynamic
//	group    group                                 recurse
//	leaf     group                                 dynamic wins, warn
//	group    leaf                                  dynamic wins, warn
//
// MergeHardcodedLeaves mutates dynamicRoot in place and returns it so callers
// can chain. hardcodedRoot is treated as a donor: grafted children are
// detached from it so their cobra parent pointer points at the new parent.
func MergeHardcodedLeaves(dynamicRoot, hardcodedRoot *cobra.Command) *cobra.Command {
	if dynamicRoot == nil || hardcodedRoot == nil {
		return dynamicRoot
	}
	// Snapshot children before mutating hardcodedRoot — RemoveCommand during
	// iteration over hardcodedRoot.Commands() is unsafe because cobra returns
	// a slice backed by an internal field that is re-sliced on removal.
	children := append([]*cobra.Command(nil), hardcodedRoot.Commands()...)
	for _, hc := range children {
		dyn := findChildByName(dynamicRoot, hc.Name())
		switch {
		case dyn == nil:
			hardcodedRoot.RemoveCommand(hc)
			dynamicRoot.AddCommand(hc)
		case IsLeafCmd(hc) && IsLeafCmd(dyn):
			if OverridePriority(hc) > OverridePriority(dyn) {
				hardcodedRoot.RemoveCommand(hc)
				dynamicRoot.RemoveCommand(dyn)
				dynamicRoot.AddCommand(hc)
			}
			// else: envelope is authority; hardcoded leaf is ignored.
		case !IsLeafCmd(hc) && !IsLeafCmd(dyn):
			MergeHardcodedLeaves(dyn, hc)
		default:
			slog.Warn("overlay: shape mismatch, keeping dynamic",
				"name", hc.Name(),
				"dynamicIsLeaf", IsLeafCmd(dyn),
				"hardcodedIsLeaf", IsLeafCmd(hc))
		}
	}
	return dynamicRoot
}

// IsLeafCmd reports whether a command has no subcommands. Leaves carry a RunE
// and are invocation targets; groups merely organise subcommands.
func IsLeafCmd(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return !cmd.HasSubCommands()
}

// findChildByName scans parent's direct children for a matching Name(). A
// local helper so pkg/cmdutil stays independent of internal/cobracmd.
func findChildByName(parent *cobra.Command, name string) *cobra.Command {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}
