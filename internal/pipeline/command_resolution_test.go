// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	stderrors "errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageValidateUnresolvedCommandClassifiesShortcutBeforeFlags(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	chat := &cobra.Command{Use: "chat"}
	chat.AddCommand(
		&cobra.Command{Use: "+chat-messages", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "+messages-send", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "chat-mesages", Run: func(*cobra.Command, []string) {}},
	)
	root.AddCommand(chat)

	err := validateUnresolvedCommand(chat, []string{"+chat-mesages", "--keyword", "x"})
	structured := requireCommandResolutionError(t, err, "unknown_shortcut")
	if structured.Message != `unknown shortcut "+chat-mesages" for "dws chat"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if structured.Hint != `Did you mean "dws chat +chat-messages"?` {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	if len(structured.Actions) != 1 || structured.Actions[0] != "Run 'dws shortcut list --service chat --format json'" {
		t.Fatalf("Actions = %#v", structured.Actions)
	}
	if len(structured.AvailableFlags) != 0 {
		t.Fatalf("AvailableFlags = %#v, want none for command error", structured.AvailableFlags)
	}
}

func TestCrossPlatformCoverageValidateUnresolvedCommandClassifiesOnlyExplicitContainers(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	dev := &cobra.Command{Use: "dev"}
	app := &cobra.Command{Use: "app"}
	cmdutil.MarkGroup(app)
	app.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	dev.AddCommand(app)
	root.AddCommand(dev)

	err := validateUnresolvedCommand(app, []string{"search", "--keyword", "x"})
	structured := requireCommandResolutionError(t, err, "unknown_subcommand")
	if structured.Message != `unknown subcommand "search" for "dws dev app"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if len(structured.Actions) != 1 || structured.Actions[0] != "Run 'dws dev app --help' to see available subcommands" {
		t.Fatalf("Actions = %#v", structured.Actions)
	}

	positional := &cobra.Command{Use: "schema [path]", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(positional)
	for name, test := range map[string]struct {
		target    *cobra.Command
		remaining []string
	}{
		"legitimate positional": {target: positional, remaining: []string{"+chat-messages"}},
		"flag remains a flag":   {target: app, remaining: []string{"--keyword", "x"}},
		"dash terminator":       {target: app, remaining: []string{"--", "search"}},
		"nil target":            {target: nil, remaining: []string{"search"}},
		"empty remaining":       {target: app},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateUnresolvedCommand(test.target, test.remaining); err != nil {
				t.Fatalf("validateUnresolvedCommand() error = %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageRunPreParseArgsValidatesCommandsWithoutHandlersAndPrimesPresentation(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "table", "")
	chat := &cobra.Command{Use: "chat"}
	chat.AddCommand(&cobra.Command{Use: "+chat-messages", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(chat)

	ctx, err := RunPreParseArgs(root, nil, []string{
		"--format", "json", "chat", "+chat-mesages", "--keyword", "x",
	})
	if ctx == nil || ctx.Command != "dws chat" {
		t.Fatalf("Context = %#v", ctx)
	}
	requireCommandResolutionError(t, err, "unknown_shortcut")
	if format, getErr := root.PersistentFlags().GetString("format"); getErr != nil || format != "json" {
		t.Fatalf("format = %q, %v; want primed json presentation", format, getErr)
	}

	validRoot := &cobra.Command{Use: "dws"}
	validChat := &cobra.Command{Use: "chat"}
	validChat.AddCommand(&cobra.Command{Use: "+chat-messages", Run: func(*cobra.Command, []string) {}})
	validRoot.AddCommand(validChat)
	if ctx, err := RunPreParseArgs(validRoot, nil, []string{"chat", "+chat-messages"}); ctx != nil || err != nil {
		t.Fatalf("valid shortcut = %#v, %v", ctx, err)
	}
}

func TestCrossPlatformCoverageCommandSuggestionHintIsBoundedAndFallsBack(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo", SuggestionsMinimumDistance: 100}
	root.AddCommand(group)
	for _, name := range []string{"alpha", "beta", "delta", "gamma"} {
		group.AddCommand(&cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}})
	}
	hint := commandSuggestionHint(group, "missing", "fallback")
	if strings.Count(hint, `"dws demo `) != maxCommandSuggestions || strings.Contains(hint, "gamma") {
		t.Fatalf("bounded hint = %q", hint)
	}

	empty := &cobra.Command{Use: "empty"}
	root.AddCommand(empty)
	if got := commandSuggestionHint(empty, "missing", "fallback"); got != "fallback" {
		t.Fatalf("fallback hint = %q", got)
	}
}

func TestCrossPlatformCoverageCommandResolutionDefensiveAndExplicitSuggestionPaths(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	service := &cobra.Command{Use: "demo"}
	group := &cobra.Command{Use: "nested"}
	service.AddCommand(group)
	root.AddCommand(service)

	if isExplicitShortcutCandidate(root, "+missing") {
		t.Fatal("root command was treated as a shortcut service")
	}
	if isExplicitShortcutCandidate(group, "+missing") {
		t.Fatal("nested command was treated as a top-level shortcut service")
	}
	if got := commandSuggestions(nil, "missing"); got != nil {
		t.Fatalf("commandSuggestions(nil) = %#v", got)
	}

	service.SuggestionsMinimumDistance = 1
	service.AddCommand(&cobra.Command{
		Use:        "canonical",
		SuggestFor: []string{"invented"},
		Run:        func(*cobra.Command, []string) {},
	})
	if got := commandSuggestions(service, "invented"); len(got) != 1 || got[0] != "canonical" {
		t.Fatalf("explicit SuggestFor suggestions = %#v", got)
	}
}

func requireCommandResolutionError(t *testing.T, err error, reason string) *apperrors.Error {
	t.Helper()
	var structured *apperrors.Error
	if !stderrors.As(err, &structured) {
		t.Fatalf("error = %T %v, want *errors.Error", err, err)
	}
	if structured.Category != apperrors.CategoryValidation || structured.Reason != reason || structured.ExitCode() != 3 {
		t.Fatalf("structured error = %#v", structured)
	}
	return structured
}
