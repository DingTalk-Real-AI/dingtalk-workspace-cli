// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	"fmt"
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

const maxCommandSuggestions = 3

// validateUnresolvedCommand gives command identity precedence over flag
// parsing. Cobra otherwise falls back to the nearest parent when a child is
// unknown, so a later flag can incorrectly turn an invented command into an
// "unknown flag" error on that parent.
//
// Only two unambiguous command containers are handled here:
//   - a top-level service followed by an explicit +shortcut token; and
//   - a command explicitly annotated as a group container.
//
// Other unresolved positionals remain Cobra's responsibility so commands that
// legitimately accept positional arguments are not reclassified.
func validateUnresolvedCommand(target *cobra.Command, remaining []string) error {
	if target == nil || len(remaining) == 0 {
		return nil
	}
	candidate := strings.TrimSpace(remaining[0])
	if candidate == "" || candidate == "--" || strings.HasPrefix(candidate, "-") {
		return nil
	}

	if isExplicitShortcutCandidate(target, candidate) {
		return unknownShortcutError(target, candidate)
	}
	if cmdutil.IsGroup(target) {
		return unknownSubcommandError(target, candidate)
	}
	return nil
}

func isExplicitShortcutCandidate(target *cobra.Command, candidate string) bool {
	if target == nil || !strings.HasPrefix(candidate, "+") {
		return false
	}
	root := target.Root()
	if root == nil || target == root || target.Parent() != root {
		return false
	}
	for _, child := range target.Commands() {
		if strings.HasPrefix(child.Name(), "+") {
			return true
		}
	}
	return false
}

func unknownShortcutError(parent *cobra.Command, candidate string) error {
	action := fmt.Sprintf("Run '%s shortcut list --service %s --format json'", parent.Root().Name(), parent.Name())
	return apperrors.NewValidation(
		fmt.Sprintf("unknown shortcut %q for %q", candidate, parent.CommandPath()),
		apperrors.WithReason("unknown_shortcut"),
		apperrors.WithHint(commandSuggestionHint(parent, candidate, action)),
		apperrors.WithActions(action),
	)
}

func unknownSubcommandError(parent *cobra.Command, candidate string) error {
	action := fmt.Sprintf("Run '%s --help' to see available subcommands", parent.CommandPath())
	return apperrors.NewValidation(
		fmt.Sprintf("unknown subcommand %q for %q", candidate, parent.CommandPath()),
		apperrors.WithReason("unknown_subcommand"),
		apperrors.WithHint(commandSuggestionHint(parent, candidate, action)),
		apperrors.WithActions(action),
	)
}

func commandSuggestionHint(parent *cobra.Command, candidate, fallback string) string {
	suggestions := commandSuggestions(parent, candidate)
	if len(suggestions) > maxCommandSuggestions {
		suggestions = suggestions[:maxCommandSuggestions]
	}
	paths := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		paths = append(paths, fmt.Sprintf("%q", parent.CommandPath()+" "+suggestion))
	}
	switch len(paths) {
	case 0:
		return fallback
	case 1:
		return "Did you mean " + paths[0] + "?"
	default:
		return "Did you mean one of: " + strings.Join(paths, ", ") + "?"
	}
}

// commandSuggestions mirrors Cobra's suggestion rules without mutating
// SuggestionsMinimumDistance. Cobra only installs its default distance inside
// the final error renderer; this guard runs earlier and must remain safe for
// command trees shared by concurrent tests and callers.
func commandSuggestions(parent *cobra.Command, candidate string) []string {
	if parent == nil {
		return nil
	}
	minimumDistance := parent.SuggestionsMinimumDistance
	if minimumDistance <= 0 {
		minimumDistance = 2
	}
	lowerCandidate := strings.ToLower(candidate)
	shortcutOnly := strings.HasPrefix(candidate, "+")
	seen := make(map[string]bool)
	var suggestions []string
	for _, child := range parent.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		name := child.Name()
		if shortcutOnly && !strings.HasPrefix(name, "+") {
			continue
		}
		matched := cmdutil.LevenshteinDist(lowerCandidate, strings.ToLower(name)) <= minimumDistance ||
			strings.HasPrefix(strings.ToLower(name), lowerCandidate)
		if !matched {
			for _, explicit := range child.SuggestFor {
				if strings.EqualFold(candidate, explicit) {
					matched = true
					break
				}
			}
		}
		if matched && !seen[name] {
			seen[name] = true
			suggestions = append(suggestions, name)
		}
	}
	sort.Strings(suggestions)
	return suggestions
}
