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

// Package cobracmd provides shared Cobra command utilities used across
// multiple internal packages (app, cli, compat, helpers).
package cobracmd

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ChildByName returns the child command with the given name, or nil.
func ChildByName(parent *cobra.Command, name string) *cobra.Command {
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

// FlagChanged reports whether the named flag was explicitly set by the user.
func FlagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

// NewGroupCommand creates a non-leaf parent command that shows help when invoked.
func NewGroupCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	// Tag as a group container: its RunE only prints help, so cobra's
	// Runnable() can't distinguish it from a real leaf — callers that need to
	// collapse empty groups rely on this annotation.
	cmdutil.MarkGroup(cmd)
	return cmd
}

// NewHiddenGroupCommand creates a hidden non-leaf parent command.
func NewHiddenGroupCommand(use, short string) *cobra.Command {
	cmd := NewGroupCommand(use, short)
	cmd.Hidden = true
	return cmd
}

// NewPlaceholderParent creates a non-leaf parent command with pre-attached children.
func NewPlaceholderParent(use, short string, children ...*cobra.Command) *cobra.Command {
	cmd := NewGroupCommand(use, short)
	cmd.AddCommand(children...)
	return cmd
}

// IsGenericOverlayShort returns true if the description is an auto-generated
// overlay placeholder that should be overwritten by richer metadata.
func IsGenericOverlayShort(s string) bool {
	return strings.HasPrefix(s, "Generated compatibility overlay") ||
		strings.HasPrefix(s, "Generated raw tool overlay") ||
		strings.HasPrefix(s, "Fallback-only ")
}

// MergeCommandTree recursively merges src's children into dst. If a child
// exists in both trees, the one with higher override priority (or more local
// flags) wins for leaf commands; groups are merged recursively.
func MergeCommandTree(dst, src *cobra.Command) {
	if dst == nil || src == nil {
		return
	}
	if dst.Short == "" || (IsGenericOverlayShort(dst.Short) && src.Short != "" && !IsGenericOverlayShort(src.Short)) {
		dst.Short = src.Short
	}
	if dst.Long == "" {
		dst.Long = src.Long
	}
	if dst.Hidden && !src.Hidden && OverridePriority(src) >= OverridePriority(dst) {
		dst.Hidden = false
	}

	for _, child := range src.Commands() {
		if existing := ChildByName(dst, child.Name()); existing != nil {
			if ShouldReplaceLeaf(existing, child) {
				ReplaceChild(dst, existing, child)
				continue
			}
			MergeCommandTree(existing, child)
			continue
		}
		dst.AddCommand(child)
	}
}

// ShouldReplaceLeaf decides whether src should replace dst as a leaf command
// based on override priority and local flag count.
func ShouldReplaceLeaf(dst, src *cobra.Command) bool {
	if dst == nil || src == nil {
		return false
	}
	if len(dst.Commands()) != 0 || len(src.Commands()) != 0 {
		return false
	}
	if srcPriority, dstPriority := OverridePriority(src), OverridePriority(dst); srcPriority != dstPriority {
		return srcPriority > dstPriority
	}
	return LocalFlagCount(src) > LocalFlagCount(dst)
}

// ReplaceChild removes oldChild from parent and adds newChild.
func ReplaceChild(parent, oldChild, newChild *cobra.Command) {
	if parent == nil || oldChild == nil || newChild == nil {
		return
	}
	parent.RemoveCommand(oldChild)
	parent.AddCommand(newChild)
}

// LocalFlagCount returns the number of visible local flags on cmd.
func LocalFlagCount(cmd *cobra.Command) int {
	if cmd == nil {
		return 0
	}
	count := 0
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			count++
		}
	})
	return count
}

// LegacyCommandPath returns the command path with the root "dws " prefix stripped.
func LegacyCommandPath(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "dws ")
}
