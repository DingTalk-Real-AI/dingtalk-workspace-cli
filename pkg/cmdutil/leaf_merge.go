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
// conflicts, the dynamic side wins by default because the runtime envelope is
// authoritative; hardcoded commands are retained as fallback for paths the
// envelope does not declare.
//
// A hardcoded leaf or group can opt into replacement by carrying a strictly
// higher OverridePriority than the dynamic command at the same path.
//
// MergeHardcodedLeaves mutates dynamicRoot in place and returns it. Grafted
// commands are detached from hardcodedRoot so Cobra parent pointers remain
// correct.
func MergeHardcodedLeaves(dynamicRoot, hardcodedRoot *cobra.Command) *cobra.Command {
	if dynamicRoot == nil || hardcodedRoot == nil {
		return dynamicRoot
	}

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
		case !IsLeafCmd(hc) && !IsLeafCmd(dyn) && OverridePriority(hc) > OverridePriority(dyn):
			hardcodedRoot.RemoveCommand(hc)
			dynamicRoot.RemoveCommand(dyn)
			dynamicRoot.AddCommand(hc)
		case !IsLeafCmd(hc) && !IsLeafCmd(dyn):
			MergeHardcodedLeaves(dyn, hc)
		case IsLeafCmd(dyn) && !IsLeafCmd(hc) && OverridePriority(hc) > OverridePriority(dyn):
			hardcodedRoot.RemoveCommand(hc)
			dynamicRoot.RemoveCommand(dyn)
			dynamicRoot.AddCommand(hc)
		default:
			slog.Warn("overlay: shape mismatch, keeping dynamic",
				"name", hc.Name(),
				"dynamicIsLeaf", IsLeafCmd(dyn),
				"hardcodedIsLeaf", IsLeafCmd(hc))
		}
	}
	return dynamicRoot
}

// IsLeafCmd reports whether cmd has no subcommands.
func IsLeafCmd(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return !cmd.HasSubCommands()
}

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
