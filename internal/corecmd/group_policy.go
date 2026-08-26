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

package corecmd

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// GroupMode declares whether a command with children is navigation-only or
// also owns business execution. The zero value means the command is a leaf.
type GroupMode string

const (
	// GroupNavigationOnly is a parent whose own invocation only renders help.
	GroupNavigationOnly GroupMode = "navigation_only"
	// GroupHybrid is a runnable business command that also owns children.
	GroupHybrid GroupMode = "hybrid"
)

// PositionalsPolicy declares whether a group may consume positional values.
type PositionalsPolicy string

const (
	// PositionalsReject makes every unmatched positional token eligible for
	// command-resolution recovery rather than business execution.
	PositionalsReject PositionalsPolicy = "reject"
	// PositionalsAllow reserves positional values for the group's business
	// execution. Recovery must therefore be disabled to avoid ambiguity.
	PositionalsAllow PositionalsPolicy = "allow"
)

// RecoveryPolicy declares the search scope for unknown-command recovery.
type RecoveryPolicy string

const (
	// RecoverySibling suggests only direct children of the current group.
	RecoverySibling RecoveryPolicy = "sibling"
	// RecoveryDeep may search all descendants of the current group.
	RecoveryDeep RecoveryPolicy = "deep"
	// RecoveryDisabled leaves positional handling entirely to Cobra or the
	// command's business execution.
	RecoveryDisabled RecoveryPolicy = "disabled"
)

// GroupPolicy is the typed declaration for every non-leaf command.
//
// Its zero value deliberately means "leaf": callers must declare all three
// fields together for a group. ApplyGroupPolicy compiles the declaration to
// Cobra behavior and private framework metadata; command authors must not
// author parallel kind annotations themselves.
type GroupPolicy struct {
	Mode        GroupMode
	Positionals PositionalsPolicy
	Recovery    RecoveryPolicy
}

const groupPolicyAnnotation = "dws.internal.corecmd.group_policy.v1"

// IsZero reports whether p is the leaf declaration.
func (p GroupPolicy) IsZero() bool {
	return p.Mode == "" && p.Positionals == "" && p.Recovery == ""
}

// ValidateGroupPolicy rejects partial declarations, unknown enum values, and
// combinations whose parsing semantics would be ambiguous.
func ValidateGroupPolicy(p GroupPolicy) error {
	if p.IsZero() {
		return nil
	}
	switch p.Mode {
	case GroupNavigationOnly, GroupHybrid:
	default:
		return fmt.Errorf("invalid group mode %q", p.Mode)
	}
	switch p.Positionals {
	case PositionalsReject, PositionalsAllow:
	default:
		return fmt.Errorf("invalid group positionals policy %q", p.Positionals)
	}
	switch p.Recovery {
	case RecoverySibling, RecoveryDeep, RecoveryDisabled:
	default:
		return fmt.Errorf("invalid group recovery policy %q", p.Recovery)
	}
	if p.Mode == GroupNavigationOnly && p.Positionals != PositionalsReject {
		return fmt.Errorf("navigation-only group requires positionals=%q", PositionalsReject)
	}
	if p.Positionals == PositionalsAllow && p.Recovery != RecoveryDisabled {
		return fmt.Errorf("group with positionals=%q requires recovery=%q", PositionalsAllow, RecoveryDisabled)
	}
	return nil
}

// ApplyGroupPolicy is the sole declaration API for non-leaf command behavior.
// Invalid declarations panic because they are framework construction bugs,
// matching the fail-closed behavior of Spec flag/constraint declarations.
//
// Navigation-only groups receive the shared help/unknown-command RunE. Hybrid
// groups retain their existing RunE; when they reject positionals, a wrapper
// sends non-empty args to the same unknown-command resolver before invoking
// business execution. When recovery is enabled, rejecting positionals
// deliberately compiles to cobra.ArbitraryArgs: Cobra must not intercept the
// token with a generic error before command resolution can produce bounded
// guidance. RecoveryDisabled instead compiles rejection to cobra.NoArgs.
func ApplyGroupPolicy(cmd *cobra.Command, policy GroupPolicy) {
	if cmd == nil {
		panic("cannot apply GroupPolicy to a nil command")
	}
	if policy.IsZero() {
		panic(fmt.Sprintf("command %q cannot apply the zero GroupPolicy; zero means leaf", cmd.Name()))
	}
	if err := ValidateGroupPolicy(policy); err != nil {
		panic(fmt.Sprintf("command %q declares invalid GroupPolicy: %v", cmd.Name(), err))
	}
	if existing, ok, err := GroupPolicyFor(cmd); err != nil {
		panic(fmt.Sprintf("command %q carries invalid GroupPolicy metadata: %v", cmd.Name(), err))
	} else if ok && existing != policy {
		panic(fmt.Sprintf("command %q redeclares GroupPolicy from %+v to %+v", cmd.Name(), existing, policy))
	} else if ok {
		return
	}

	if policy.Mode == GroupNavigationOnly {
		cmd.Run = nil
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			return runGroupPolicy(cmd, args, policy)
		}
	} else if cmd.RunE == nil {
		panic(fmt.Sprintf("hybrid group %q must declare RunE before GroupPolicy is applied", cmd.Name()))
	} else if policy.Positionals == PositionalsReject && policy.Recovery != RecoveryDisabled {
		businessRunE := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return runGroupPolicy(cmd, args, policy)
			}
			return businessRunE(cmd, args)
		}
	}
	if policy.Positionals == PositionalsReject && policy.Recovery != RecoveryDisabled {
		cmd.Args = cobra.ArbitraryArgs
	} else if policy.Positionals == PositionalsReject {
		cmd.Args = cobra.NoArgs
	}

	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[groupPolicyAnnotation] = encodeGroupPolicy(policy)
}

// GroupPolicyFor reads the typed declaration compiled onto cmd. The boolean is
// false only for a leaf. Malformed private metadata is returned as an error so
// tree assembly can fail closed instead of silently treating a group as a leaf.
func GroupPolicyFor(cmd *cobra.Command) (GroupPolicy, bool, error) {
	if cmd == nil || cmd.Annotations == nil {
		return GroupPolicy{}, false, nil
	}
	raw, ok := cmd.Annotations[groupPolicyAnnotation]
	if !ok {
		return GroupPolicy{}, false, nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) != 3 {
		return GroupPolicy{}, false, fmt.Errorf("malformed encoded GroupPolicy %q", raw)
	}
	policy := GroupPolicy{
		Mode:        GroupMode(parts[0]),
		Positionals: PositionalsPolicy(parts[1]),
		Recovery:    RecoveryPolicy(parts[2]),
	}
	if err := ValidateGroupPolicy(policy); err != nil {
		return GroupPolicy{}, false, err
	}
	if policy.IsZero() {
		return GroupPolicy{}, false, fmt.Errorf("encoded GroupPolicy must not be zero")
	}
	return policy, true, nil
}

func encodeGroupPolicy(policy GroupPolicy) string {
	return string(policy.Mode) + "|" + string(policy.Positionals) + "|" + string(policy.Recovery)
}

func runGroupPolicy(cmd *cobra.Command, args []string, policy GroupPolicy) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	input := strings.TrimSpace(args[0])
	reason := cmdutil.ClassifyCommandResolution(cmd, input)
	suggestions := cmdutil.SuggestSubcommands(cmd, input)
	if reason == cmdutil.ResolutionUnknownSubcommand && policy.Recovery == RecoveryDeep {
		if deep := cmdutil.SuggestDescendantSubcommands(cmd, input); len(deep) > 0 {
			suggestions = deep
		}
	}
	return cmdutil.NewCommandResolution(
		cmd,
		input,
		reason,
		suggestions,
		"",
	).Err()
}
