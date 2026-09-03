// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

const (
	commandPathFallbackModeRewrite   = "rewrite"
	commandPathFallbackModeAmbiguous = "ambiguous"
)

// normalizeCommandPathFallback applies only an exact reviewed path match. It
// runs before Cobra resolves a leaf, then the canonical argv continues through
// the ordinary parameter alias, validation, safety, and execution pipeline.
func normalizeCommandPathFallback(root *cobra.Command, engine *Engine, rawArgs []string) ([]string, *Correction, error) {
	if root == nil || engine == nil || engine.commandPathFallbackLookup == nil || len(rawArgs) == 0 {
		return rawArgs, nil, nil
	}
	traversalArgs, positions := argsForCommandTraversalWithPositionsForEngine(root, engine, rawArgs)
	entry, tokenCount, ok := longestCommandPathFallback(engine, traversalArgs)
	if !ok {
		return rawArgs, nil, nil
	}
	// Runtime extensions are mounted on the root before PreParse runs. An exact
	// executable command (or exact Cobra alias) is user/runtime authority and
	// must win over the reviewed recovery dictionary. Hint-only compatibility
	// nodes are not executable business commands, so fallback may supersede
	// them as it does for the distribution-owned declaration tree.
	if exactRunnableCommandPath(root, traversalArgs[:tokenCount]) {
		return rawArgs, nil, nil
	}

	switch entry.Mode {
	case commandPathFallbackModeRewrite:
		if strings.TrimSpace(entry.To) == "" {
			return rawArgs, nil, apperrors.NewInternal(fmt.Sprintf("reviewed command path fallback %q has no rewrite target", entry.From))
		}
		rewritten := rewriteCommandPathTokens(rawArgs, positions[:tokenCount], strings.Fields(entry.To))
		correction := &Correction{
			Handler:   "command-path-fallback",
			Phase:     PreParse,
			Field:     "command_path",
			Original:  entry.From,
			Corrected: entry.To,
			Kind:      "reviewed-fallback",
		}
		return rewritten, correction, nil
	case commandPathFallbackModeAmbiguous:
		return rawArgs, nil, ambiguousCommandPathFallbackError(root, entry)
	default:
		return rawArgs, nil, apperrors.NewInternal(fmt.Sprintf("reviewed command path fallback %q has invalid mode %q", entry.From, entry.Mode))
	}
}

func exactRunnableCommandPath(root *cobra.Command, path []string) bool {
	if root == nil || len(path) == 0 {
		return false
	}
	current := root
	if path[0] == root.Name() {
		path = path[1:]
	}
	if len(path) == 0 {
		return false
	}
	for _, token := range path {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == token || containsExactCommandAlias(child.Aliases, token) {
				next = child
				break
			}
		}
		if next == nil {
			return false
		}
		current = next
	}
	return current.Runnable() && !cmdutil.IsHintOnlyCommand(current)
}

func containsExactCommandAlias(aliases []string, token string) bool {
	for _, alias := range aliases {
		if alias == token {
			return true
		}
	}
	return false
}

func longestCommandPathFallback(engine *Engine, traversalArgs []string) (CommandPathFallback, int, bool) {
	var selected CommandPathFallback
	selectedCount := 0
	parts := make([]string, 0, len(traversalArgs))
	for index, argument := range traversalArgs {
		argument = strings.TrimSpace(argument)
		if argument == "" || argument == "--" || strings.HasPrefix(argument, "-") {
			break
		}
		parts = append(parts, argument)
		if candidate, ok := engine.lookupCommandPathFallback(strings.Join(parts, " ")); ok {
			selected = candidate
			selectedCount = index + 1
		}
	}
	return selected, selectedCount, selectedCount > 0
}

func rewriteCommandPathTokens(rawArgs []string, commandPositions []int, canonicalTokens []string) []string {
	if len(commandPositions) == 0 || len(canonicalTokens) == 0 {
		return append([]string(nil), rawArgs...)
	}
	remove := make(map[int]bool, len(commandPositions))
	for _, position := range commandPositions {
		remove[position] = true
	}
	insertAt := commandPositions[0]
	out := make([]string, 0, len(rawArgs)-len(commandPositions)+len(canonicalTokens))
	for index, argument := range rawArgs {
		if index == insertAt {
			out = append(out, canonicalTokens...)
		}
		if remove[index] {
			continue
		}
		out = append(out, argument)
	}
	return out
}

func ambiguousCommandPathFallbackError(root *cobra.Command, entry CommandPathFallback) error {
	rootName := "dws"
	if root != nil && strings.TrimSpace(root.Name()) != "" {
		rootName = root.Name()
	}
	paths := make([]string, 0, len(entry.Candidates))
	actions := make([]string, 0, len(entry.Candidates))
	for _, candidate := range entry.Candidates {
		path := strings.TrimSpace(rootName + " " + candidate)
		paths = append(paths, fmt.Sprintf("%q", path))
		actions = append(actions, fmt.Sprintf("Run '%s --help' to inspect this candidate", path))
	}
	hint := "Inspect the reviewed candidates and choose by the original user intent: " + strings.Join(paths, ", ")
	return apperrors.NewValidation(
		fmt.Sprintf("command path %q is ambiguous", strings.TrimSpace(rootName+" "+entry.From)),
		apperrors.WithReason("ambiguous_command_fallback"),
		apperrors.WithHint(hint),
		apperrors.WithActions(actions...),
	)
}
