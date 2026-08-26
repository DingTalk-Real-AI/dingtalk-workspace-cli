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
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
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
		TraverseChildren:  true,
		DisableAutoGenTag: true,
	}
	corecmd.ApplyGroupPolicy(cmd, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
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
	mergeGroupPolicy(dst, src)
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

func mergeGroupPolicy(dst, src *cobra.Command) {
	dstPolicy, dstOK, err := corecmd.GroupPolicyFor(dst)
	if err != nil {
		panic(fmt.Sprintf("destination command %q has invalid GroupPolicy: %v", dst.CommandPath(), err))
	}
	srcPolicy, srcOK, err := corecmd.GroupPolicyFor(src)
	if err != nil {
		panic(fmt.Sprintf("source command %q has invalid GroupPolicy: %v", src.CommandPath(), err))
	}
	if len(dst.Commands()) > 0 && !dstOK {
		panic(fmt.Sprintf("destination command %q has children but no GroupPolicy", dst.CommandPath()))
	}
	if len(src.Commands()) > 0 && !srcOK {
		panic(fmt.Sprintf("source command %q has children but no GroupPolicy", src.CommandPath()))
	}
	if dstOK && !srcOK {
		if !isNeutralMergePlaceholder(src) {
			panic(fmt.Sprintf("cannot merge runnable or behavior-bearing leaf command %q into typed group command %q",
				src.CommandPath(), dst.CommandPath()))
		}
		return
	}
	if !dstOK && srcOK {
		if !isNeutralMergePlaceholder(dst) {
			panic(fmt.Sprintf("cannot merge typed group command %q into runnable or behavior-bearing leaf command %q",
				src.CommandPath(), dst.CommandPath()))
		}
		corecmd.ApplyGroupPolicy(dst, srcPolicy)
		return
	}
	if dstOK && srcOK && dstPolicy != srcPolicy {
		// A NavigationOnly/Reject/Sibling source is the framework's neutral
		// service scaffold (shortcuts and plugin overlays use it before being
		// folded into an owning product root). The destination owns the merged
		// command's default action and recovery scope, so preserve its policy.
		// Any stronger source declaration would lose behavior during this
		// destination-oriented merge and therefore fails closed.
		if srcPolicy != (corecmd.GroupPolicy{
			Mode:        corecmd.GroupNavigationOnly,
			Positionals: corecmd.PositionalsReject,
			Recovery:    corecmd.RecoverySibling,
		}) {
			panic(fmt.Sprintf("cannot merge command %q with conflicting GroupPolicy declarations: %+v != %+v",
				dst.CommandPath(), dstPolicy, srcPolicy))
		}
		return
	}
}

// isNeutralMergePlaceholder reports whether cmd contributes metadata only.
// Such a shell may adopt (or be folded into) one typed group declaration.
// Anything executable, parse-affecting, or child-bearing must declare its own
// compatible GroupPolicy so tree assembly cannot silently discard behavior.
func isNeutralMergePlaceholder(cmd *cobra.Command) bool {
	if cmd == nil || len(cmd.Commands()) != 0 || cmd.Runnable() || cmd.Args != nil ||
		cmd.PreRun != nil || cmd.PreRunE != nil || cmd.PostRun != nil || cmd.PostRunE != nil ||
		cmd.PersistentPreRun != nil || cmd.PersistentPreRunE != nil ||
		cmd.PersistentPostRun != nil || cmd.PersistentPostRunE != nil ||
		cmd.TraverseChildren || cmd.DisableFlagParsing {
		return false
	}
	hasFlags := false
	cmd.LocalNonPersistentFlags().VisitAll(func(*pflag.Flag) { hasFlags = true })
	cmd.PersistentFlags().VisitAll(func(*pflag.Flag) { hasFlags = true })
	return !hasFlags
}

// ValidateGroupTree checks the final assembled Cobra tree rather than source
// syntax. Every command with children must carry one valid typed GroupPolicy;
// leaves must carry none. This catches dynamically assembled aliases, plugin
// parents, and constructors outside any one source directory.
func ValidateGroupTree(root *cobra.Command) error {
	if root == nil {
		return fmt.Errorf("cannot validate a nil command tree")
	}
	return validateGroupNode(root)
}

func validateGroupNode(cmd *cobra.Command) error {
	policy, declared, err := corecmd.GroupPolicyFor(cmd)
	if err != nil {
		return fmt.Errorf("command %q has invalid GroupPolicy metadata: %w", cmd.CommandPath(), err)
	}
	children := cmd.Commands()
	if len(children) == 0 {
		if declared {
			return fmt.Errorf("leaf command %q retains GroupPolicy %+v", cmd.CommandPath(), policy)
		}
		return nil
	}
	if !declared {
		return fmt.Errorf("command %q has children but no GroupPolicy", cmd.CommandPath())
	}
	if !cmd.Runnable() {
		return fmt.Errorf("group command %q with mode %q is not runnable", cmd.CommandPath(), policy.Mode)
	}
	if policy.Mode == corecmd.GroupNavigationOnly && (cmd.RunE == nil || cmd.Run != nil) {
		return fmt.Errorf("navigation-only group %q does not retain framework help execution", cmd.CommandPath())
	}
	if policy.Mode == corecmd.GroupHybrid && cmd.RunE == nil {
		return fmt.Errorf("hybrid group %q lost its business RunE", cmd.CommandPath())
	}
	if policy.Positionals == corecmd.PositionalsAllow && cmd.Args == nil {
		return fmt.Errorf("group command %q allows positionals without an explicit Args contract", cmd.CommandPath())
	}
	if policy.Positionals == corecmd.PositionalsReject {
		if cmd.Args == nil {
			return fmt.Errorf("group command %q rejects positionals without compiled Args behavior", cmd.CommandPath())
		}
	}
	if policy.Recovery == corecmd.RecoveryDeep && !hasAvailableDescendant(cmd) {
		return fmt.Errorf("group command %q declares deep recovery without an available descendant", cmd.CommandPath())
	}
	for _, child := range children {
		if err := validateGroupNode(child); err != nil {
			return err
		}
	}
	return nil
}

func hasAvailableDescendant(cmd *cobra.Command) bool {
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		return true
	}
	return false
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
