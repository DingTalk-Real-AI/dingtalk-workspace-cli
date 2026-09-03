// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// validateUnresolvedCommand gives command identity precedence over flag
// parsing. Cobra otherwise falls back to the nearest parent when a child is
// unknown, so a later flag can incorrectly turn an invented command into an
// "unknown flag" error on that parent.
//
// Only groups whose typed framework policy rejects positionals and enables
// recovery are handled here. Commands that legitimately accept positionals
// remain Cobra's responsibility, without inference from tree shape or RunE.
func validateUnresolvedCommand(target *cobra.Command, remaining []string) error {
	if target == nil || len(remaining) == 0 {
		return nil
	}
	candidate := strings.TrimSpace(remaining[0])
	if candidate == "" || candidate == "--" || strings.HasPrefix(candidate, "-") {
		return nil
	}
	// Cobra registers its built-in `help [command]` lazily during ExecuteC,
	// after this pre-parse traversal runs. Preserve that reserved root entry so
	// `dws help auth` reaches Cobra instead of being classified as a typo.
	if target == target.Root() && candidate == "help" {
		return nil
	}
	policy, declared, err := corecmd.GroupPolicyFor(target)
	if err != nil {
		return err
	}
	if !declared || policy.Positionals != corecmd.PositionalsReject || policy.Recovery == corecmd.RecoveryDisabled {
		return nil
	}

	reason := cmdutil.ClassifyCommandResolution(target, candidate)
	suggestions := cmdutil.SuggestSubcommands(target, candidate)
	if reason == cmdutil.ResolutionUnknownSubcommand && policy.Recovery == corecmd.RecoveryDeep {
		if deep := cmdutil.SuggestDescendantSubcommands(target, candidate); len(deep) > 0 {
			suggestions = deep
		}
	}
	// ValidateGroupPolicy and the guard above leave only sibling/deep recovery.
	return cmdutil.NewCommandResolution(target, candidate, reason, suggestions, "").Err()
}
