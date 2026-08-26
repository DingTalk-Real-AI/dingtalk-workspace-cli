// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package cmdutil

import (
	"errors"
	"slices"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCommandResolutionProjectsExactShortcutContract(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	chat := &cobra.Command{Use: "chat"}
	root.AddCommand(chat)

	resolution := NewCommandResolution(
		chat,
		" +chat-mesages ",
		ResolutionUnknownShortcut,
		[]string{"+chat-messages"},
		"",
	)
	structured := requireTypedCommandResolution(t, resolution.Err(), ResolutionUnknownShortcut)

	if structured.Message != `unknown shortcut "+chat-mesages" for "dws chat"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if structured.Hint != `Did you mean "dws chat +chat-messages"? (Run 'dws chat --help' for the full list)` {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	wantActions := []string{
		"Run 'dws chat --help' for the full list",
		"Run 'dws shortcut list --service chat --format json'",
	}
	if !slices.Equal(structured.Actions, wantActions) {
		t.Fatalf("Actions = %#v, want %#v", structured.Actions, wantActions)
	}
	assertResolutionDetails(t, structured, "+chat-mesages", []string{"+chat-messages"})
}

func TestCrossPlatformCoverageGroupRunEClassifiesShortcutLikePreParse(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	chat := &cobra.Command{Use: "chat"}
	chat.AddCommand(&cobra.Command{Use: "+chat-messages", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(chat)

	structured := requireTypedCommandResolution(t, GroupRunE(chat, []string{"+chat-mesages"}), ResolutionUnknownShortcut)
	if structured.Message != `unknown shortcut "+chat-mesages" for "dws chat"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if structured.Hint != `Did you mean "dws chat +chat-messages"? (Run 'dws chat --help' for the full list)` {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	if !slices.Equal(structured.Actions, []string{
		"Run 'dws chat --help' for the full list",
		"Run 'dws shortcut list --service chat --format json'",
	}) {
		t.Fatalf("Actions = %#v", structured.Actions)
	}
	assertResolutionDetails(t, structured, "+chat-mesages", []string{"+chat-messages"})
}

func TestCrossPlatformCoverageCommandResolutionBoundsOneSuggestionSetForHintAndDetails(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo"}
	root.AddCommand(group)

	resolution := NewCommandResolution(
		group,
		" alphx ",
		ResolutionUnknownSubcommand,
		[]string{" alpha ", "alpha", "beta", "", "gamma", "delta"},
		"",
	)
	if !slices.Equal(resolution.suggestions, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("Suggestions = %#v", resolution.suggestions)
	}
	if got, ok := resolution.Details()["suggestions"].([]string); !ok || !slices.Equal(got, resolution.suggestions) {
		t.Fatalf("Details.suggestions = %#v", resolution.Details()["suggestions"])
	}
	structured := requireTypedCommandResolution(t, resolution.Err(), ResolutionUnknownSubcommand)
	if structured.Message != `unknown subcommand "alphx" for "dws demo"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if strings.Count(structured.Hint, `"dws demo `) != MaxCommandSuggestions || strings.Contains(structured.Hint, "delta") {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	assertResolutionDetails(t, structured, "alphx", []string{"alpha", "beta", "gamma"})
}

func TestCrossPlatformCoverageHintSubCmdUsesSameTypedResolutionFactory(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	parent := &cobra.Command{Use: "contact"}
	hint := HintSubCmd("department", "use: dws contact dept")
	root.AddCommand(parent)
	parent.AddCommand(hint)

	structured := requireTypedCommandResolution(t, hint.RunE(hint, nil), ResolutionUnknownSubcommand)
	if structured.Hint != "use: dws contact dept (Run 'dws contact --help' for the full list)" {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	if !IsHintOnlyCommand(hint) {
		t.Fatal("hint command lost hint-only identity")
	}
	assertResolutionDetails(t, structured, "department", []string{})
}

func TestCrossPlatformCoverageCommandResolutionDefensiveFallbacks(t *testing.T) {
	resolution := NewCommandResolution(nil, "missing", ResolutionReason("unsupported"), nil, "")
	structured := requireTypedCommandResolution(t, resolution.Err(), ResolutionUnknownSubcommand)
	if structured.Message != `unknown subcommand "missing" for "dws"` || structured.Hint != "Run 'dws --help' for the full list" {
		t.Fatalf("fallback resolution = %#v", structured)
	}
	assertResolutionDetails(t, structured, "missing", []string{})
	if suggestions := structured.Details["suggestions"].([]string); suggestions == nil {
		t.Fatal("empty suggestions must remain a machine-readable array, not null")
	}
}

func TestCrossPlatformCoverageCommandResolutionHelperBranches(t *testing.T) {
	group := &cobra.Command{Use: "group"}
	group.AddCommand(&cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}})
	var help strings.Builder
	group.SetOut(&help)
	group.SetErr(&help)
	if err := GroupRunE(group, nil); err != nil {
		t.Fatalf("GroupRunE help error = %v", err)
	}
	if output := help.String(); !strings.Contains(output, "Usage:") {
		t.Fatalf("GroupRunE help output = %q", output)
	}

	hint := HintSubCmd("mistake", "use canonical")
	structured := requireTypedCommandResolution(t, hint.RunE(hint, nil), ResolutionUnknownSubcommand)
	if structured.Message != `unknown subcommand "mistake" for "mistake"` {
		t.Fatalf("detached hint Message = %q", structured.Message)
	}

	nameFallback := &cobra.Command{
		Use:         "fallback",
		Annotations: map[string]string{cobra.CommandDisplayNameAnnotation: "   "},
	}
	if got := commandResolutionParentPath(nameFallback); got != "fallback" {
		t.Fatalf("commandResolutionParentPath name fallback = %q", got)
	}
	emptyFallback := &cobra.Command{
		Annotations: map[string]string{cobra.CommandDisplayNameAnnotation: ""},
	}
	if got := commandResolutionParentPath(emptyFallback); got != "dws" {
		t.Fatalf("commandResolutionParentPath empty fallback = %q", got)
	}

	parent := &cobra.Command{Use: "parent"}
	child := &cobra.Command{Use: "canonical", Aliases: []string{"alias"}}
	parent.AddCommand(child)
	if hasExactChildCommand(nil, "canonical") {
		t.Fatal("nil parent reported an exact child")
	}
	for _, candidate := range []string{"canonical", "alias"} {
		if !hasExactChildCommand(parent, candidate) {
			t.Fatalf("hasExactChildCommand(%q) = false", candidate)
		}
	}
	if hasExactChildCommand(parent, "missing") {
		t.Fatal("missing child reported as exact")
	}

	root := &cobra.Command{Use: "dws"}
	service := &cobra.Command{Use: "service"}
	service.AddCommand(&cobra.Command{Use: "ordinary"})
	root.AddCommand(service)
	if isTopLevelShortcutService(nil) || isTopLevelShortcutService(root) || isTopLevelShortcutService(service) {
		t.Fatal("non-shortcut command classified as a top-level shortcut service")
	}
}

func requireTypedCommandResolution(t *testing.T, err error, reason ResolutionReason) *apperrors.Error {
	t.Helper()
	var structured *apperrors.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %v, want *errors.Error", err, err)
	}
	if structured.Category != apperrors.CategoryValidation || structured.Reason != string(reason) || structured.ExitCode() != 3 {
		t.Fatalf("structured error = %#v", structured)
	}
	return structured
}

func assertResolutionDetails(t *testing.T, structured *apperrors.Error, input string, suggestions []string) {
	t.Helper()
	if structured.Details["input"] != input {
		t.Fatalf("Details.input = %#v, want %q", structured.Details["input"], input)
	}
	got, ok := structured.Details["suggestions"].([]string)
	if !ok || !slices.Equal(got, suggestions) {
		t.Fatalf("Details.suggestions = %#v, want %#v", structured.Details["suggestions"], suggestions)
	}
}
