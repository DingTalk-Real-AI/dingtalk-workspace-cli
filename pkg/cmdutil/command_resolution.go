// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package cmdutil

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ResolutionReason is the stable machine-readable classification for a
// command-token resolution failure.
type ResolutionReason string

const (
	// ResolutionUnknownSubcommand reports a token that does not resolve to a
	// child command of the selected parent.
	ResolutionUnknownSubcommand ResolutionReason = "unknown_subcommand"
	// ResolutionUnknownShortcut reports an explicit +shortcut token that does
	// not resolve under a top-level service.
	ResolutionUnknownShortcut ResolutionReason = "unknown_shortcut"
)

// CommandResolution is the immutable typed projection of a command-token
// resolution failure. Its constructors normalize suggestions once so the
// human hint and machine-readable details cannot diverge.
type CommandResolution struct {
	reason      ResolutionReason
	input       string
	suggestions []string
	message     string
	hint        string
	actions     []string
}

// ClassifyCommandResolution returns the stable reason for an unresolved token.
// A leading '+' is shortcut syntax only for a top-level service that actually
// exposes +shortcut children, and only when it does not already name a real
// child or alias. Keeping this classification beside CommandResolution
// prevents PreParse and direct Cobra execution from producing different
// machine contracts for the same input.
func ClassifyCommandResolution(parent *cobra.Command, input string) ResolutionReason {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "+") && isTopLevelShortcutService(parent) && !hasExactChildCommand(parent, input) {
		return ResolutionUnknownShortcut
	}
	return ResolutionUnknownSubcommand
}

// NewCommandResolution builds a bounded command-resolution result. Callers
// may pass sibling suggestions from SuggestSubcommands or reviewed deep-path
// candidates; both are normalized to the same three-item contract.
func NewCommandResolution(parent *cobra.Command, input string, reason ResolutionReason, suggestions []string, authoredHint string) CommandResolution {
	input = strings.TrimSpace(input)
	suggestions = normalizeCommandSuggestions(suggestions)
	parentPath := commandResolutionParentPath(parent)
	helpAction := fmt.Sprintf("Run '%s --help' for the full list", parentPath)

	resolution := CommandResolution{
		reason:      reason,
		input:       input,
		suggestions: suggestions,
		actions:     []string{helpAction},
	}

	switch reason {
	case ResolutionUnknownShortcut:
		resolution.message = fmt.Sprintf("unknown shortcut %q for %q", input, parentPath)
		if parent != nil {
			root := parent.Root()
			if root != nil {
				resolution.actions = append(resolution.actions,
					fmt.Sprintf("Run '%s shortcut list --service %s --format json'", root.Name(), parent.Name()))
			}
		}
	default:
		resolution.reason = ResolutionUnknownSubcommand
		resolution.message = fmt.Sprintf("unknown subcommand %q for %q", input, parentPath)
	}

	authoredHint = strings.TrimSpace(authoredHint)
	if authoredHint != "" {
		resolution.hint = authoredHint + " (" + helpAction + ")"
	} else {
		resolution.hint = FormatSubcommandSuggestionHint(parent, suggestions, helpAction)
	}
	return resolution
}

// NewInputCommandResolution classifies input and builds its one normalized
// human/machine projection. It is the shared entry point for PreParse and
// direct group execution.
func NewInputCommandResolution(parent *cobra.Command, input string, suggestions []string) CommandResolution {
	return NewCommandResolution(parent, input, ClassifyCommandResolution(parent, input), suggestions, "")
}

// Details returns a fresh machine-readable payload for the resolution. It
// always carries the exact bounded suggestions rendered in Hint.
func (r CommandResolution) Details() map[string]any {
	suggestions := make([]string, len(r.suggestions))
	copy(suggestions, r.suggestions)
	return map[string]any{
		"input":       r.input,
		"suggestions": suggestions,
	}
}

// Err projects the resolution through the repository's structured validation
// error contract.
func (r CommandResolution) Err() error {
	return apperrors.NewValidation(
		r.message,
		apperrors.WithReason(string(r.reason)),
		apperrors.WithHint(r.hint),
		apperrors.WithActions(r.actions...),
		apperrors.WithDetails(r.Details()),
	)
}

// GroupRunE is the reusable handler for navigation-only parent commands. It
// shows help without arguments and otherwise returns the same structured
// resolution contract used by pre-parse validation.
func GroupRunE(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	input := strings.TrimSpace(args[0])
	return NewInputCommandResolution(cmd, input, SuggestSubcommands(cmd, input)).Err()
}

// HintSubCmd creates a hidden compatibility command for a reviewed wrong path.
// It keeps hint-only identity while returning the unified typed resolution.
func HintSubCmd(use, authoredHint string) *cobra.Command {
	command := &cobra.Command{
		Use:         use,
		Hidden:      true,
		Annotations: map[string]string{hintOnlyCommandAnnotation: "true"},
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		parent := cmd.Parent()
		if parent == nil {
			parent = cmd.Root()
		}
		return NewCommandResolution(
			parent,
			cmd.Name(),
			ResolutionUnknownSubcommand,
			[]string{},
			authoredHint,
		).Err()
	}
	return command
}

func normalizeCommandSuggestions(suggestions []string) []string {
	out := make([]string, 0, min(len(suggestions), MaxCommandSuggestions))
	seen := make(map[string]struct{}, min(len(suggestions), MaxCommandSuggestions))
	for _, suggestion := range suggestions {
		suggestion = strings.TrimSpace(suggestion)
		if suggestion == "" {
			continue
		}
		if _, duplicate := seen[suggestion]; duplicate {
			continue
		}
		seen[suggestion] = struct{}{}
		out = append(out, suggestion)
		if len(out) == MaxCommandSuggestions {
			break
		}
	}
	return out
}

func commandResolutionParentPath(parent *cobra.Command) string {
	if parent == nil {
		return "dws"
	}
	if path := strings.TrimSpace(parent.CommandPath()); path != "" {
		return path
	}
	if name := strings.TrimSpace(parent.Name()); name != "" {
		return name
	}
	return "dws"
}

func hasExactChildCommand(parent *cobra.Command, candidate string) bool {
	if parent == nil {
		return false
	}
	for _, child := range parent.Commands() {
		if child.Name() == candidate {
			return true
		}
		for _, alias := range child.Aliases {
			if alias == candidate {
				return true
			}
		}
	}
	return false
}

func isTopLevelShortcutService(parent *cobra.Command) bool {
	if parent == nil {
		return false
	}
	root := parent.Root()
	if root == nil || parent == root || parent.Parent() != root {
		return false
	}
	for _, child := range parent.Commands() {
		if strings.HasPrefix(child.Name(), "+") {
			return true
		}
	}
	return false
}
