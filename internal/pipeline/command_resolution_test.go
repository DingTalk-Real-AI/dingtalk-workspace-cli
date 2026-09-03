// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	stderrors "errors"
	"slices"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type reviewedProtectionTestHandler struct{}

func (reviewedProtectionTestHandler) Name() string { return "reviewed-protection-test" }

func (reviewedProtectionTestHandler) Phase() Phase { return PreParse }

func (reviewedProtectionTestHandler) ResolveFlagProtection(command, flag string) (FlagProtection, bool) {
	if (command == "dws aisearch" || command == "dws chat") && flag == "types" {
		return FlagProtectionBlocked, true
	}
	return "", false
}

func (reviewedProtectionTestHandler) Handle(ctx *Context) error {
	if ctx.Command == "dws aisearch" || ctx.Command == "dws chat" {
		ctx.ProtectFlag("types", FlagProtectionBlocked)
	}
	if ctx.Command == "dws aisearch enterprise" {
		for index, argument := range ctx.Args {
			if argument != "--leaf-typz" {
				continue
			}
			ctx.Args[index] = "--leaf-type"
			ctx.AddCorrection("reviewed-protection-test", PreParse, "leaf-type", argument, "--leaf-type", "test")
		}
	}
	return nil
}

func TestCrossPlatformCoverageValidateUnresolvedCommandClassifiesShortcutBeforeFlags(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	chat := &cobra.Command{Use: "chat"}
	chat.AddCommand(
		&cobra.Command{Use: "+chat-messages", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "+messages-send", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "chat-mesages", Run: func(*cobra.Command, []string) {}},
	)
	applyTestNavigationGroup(chat)
	root.AddCommand(chat)

	err := validateUnresolvedCommand(chat, []string{"+chat-mesages", "--keyword", "x"})
	structured := requireCommandResolutionError(t, err, "unknown_shortcut")
	if structured.Message != `unknown shortcut "+chat-mesages" for "dws chat"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if structured.Hint != `Did you mean "dws chat +chat-messages"? (Run 'dws chat --help' for the full list)` {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	if len(structured.Actions) != 2 || structured.Actions[0] != "Run 'dws chat --help' for the full list" ||
		structured.Actions[1] != "Run 'dws shortcut list --service chat --format json'" {
		t.Fatalf("Actions = %#v", structured.Actions)
	}
	if len(structured.AvailableFlags) != 0 {
		t.Fatalf("AvailableFlags = %#v, want none for command error", structured.AvailableFlags)
	}
	if structured.Details["input"] != "+chat-mesages" {
		t.Fatalf("Details.input = %#v", structured.Details["input"])
	}
	if suggestions, ok := structured.Details["suggestions"].([]string); !ok || len(suggestions) != 1 || suggestions[0] != "+chat-messages" {
		t.Fatalf("Details.suggestions = %#v", structured.Details["suggestions"])
	}
}

func TestCrossPlatformCoverageValidateUnresolvedCommandClassifiesOnlyExplicitContainers(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	dev := &cobra.Command{Use: "dev"}
	app := &cobra.Command{Use: "app"}
	applyTestNavigationGroup(app)
	app.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	dev.AddCommand(app)
	root.AddCommand(dev)

	err := validateUnresolvedCommand(app, []string{"search", "--keyword", "x"})
	structured := requireCommandResolutionError(t, err, "unknown_subcommand")
	if structured.Message != `unknown subcommand "search" for "dws dev app"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if len(structured.Actions) != 1 || structured.Actions[0] != "Run 'dws dev app --help' for the full list" {
		t.Fatalf("Actions = %#v", structured.Actions)
	}
	if structured.Details["input"] != "search" {
		t.Fatalf("Details.input = %#v", structured.Details["input"])
	}
	if suggestions, ok := structured.Details["suggestions"].([]string); !ok || len(suggestions) != 0 {
		t.Fatalf("Details.suggestions = %#v", structured.Details["suggestions"])
	}
	legacy := &cobra.Command{Use: "legacy", RunE: cmdutil.GroupRunE}
	applyTestNavigationGroup(legacy)
	legacy.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(legacy)
	requireCommandResolutionError(t, validateUnresolvedCommand(legacy, []string{"lisst", "--query", "x"}), "unknown_subcommand")

	positional := &cobra.Command{Use: "schema [path]", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(positional)
	positionalWithChild := &cobra.Command{Use: "query [term]", Args: cobra.ExactArgs(1), Run: func(*cobra.Command, []string) {}}
	positionalWithChild.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(positionalWithChild)
	for name, test := range map[string]struct {
		target    *cobra.Command
		remaining []string
	}{
		"cobra help command":                {target: root, remaining: []string{"help", "chat"}},
		"legitimate positional":             {target: positional, remaining: []string{"+chat-messages"}},
		"positional close to child command": {target: positionalWithChild, remaining: []string{"lis"}},
		"flag remains a flag":               {target: app, remaining: []string{"--keyword", "x"}},
		"dash terminator":                   {target: app, remaining: []string{"--", "search"}},
		"nil target":                        {target: nil, remaining: []string{"search"}},
		"empty remaining":                   {target: app},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateUnresolvedCommand(test.target, test.remaining); err != nil {
				t.Fatalf("validateUnresolvedCommand() error = %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageValidateUnresolvedCommandRejectsInvalidGroupPolicyMetadata(t *testing.T) {
	group := &cobra.Command{Use: "broken"}
	applyTestNavigationGroup(group)
	for key := range group.Annotations {
		group.Annotations[key] = "navigation_only|reject|unexpected"
	}

	err := validateUnresolvedCommand(group, []string{"typo"})
	if err == nil || !strings.Contains(err.Error(), "invalid group recovery policy") {
		t.Fatalf("validateUnresolvedCommand() error = %v", err)
	}
}

func TestCrossPlatformCoverageRunPreParseArgsValidatesCommandsWithoutHandlersAndPrimesPresentation(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "table", "")
	chat := &cobra.Command{Use: "chat"}
	chat.AddCommand(&cobra.Command{Use: "+chat-messages", Run: func(*cobra.Command, []string) {}})
	applyTestNavigationGroup(chat)
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
	applyTestNavigationGroup(validChat)
	validRoot.AddCommand(validChat)
	if ctx, err := RunPreParseArgs(validRoot, nil, []string{"chat", "+chat-messages"}); ctx != nil || err != nil {
		t.Fatalf("valid shortcut = %#v, %v", ctx, err)
	}
}

func TestCrossPlatformCoverageRunPreParseArgsKeepsUnknownFlagValuesOutOfCommandResolution(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().Bool("yes", false, "")
	aisearch := &cobra.Command{Use: "aisearch", RunE: func(*cobra.Command, []string) error { return nil }}
	aisearch.Flags().String("query", "", "")
	aisearch.AddCommand(&cobra.Command{Use: "enterprise", Run: func(*cobra.Command, []string) {}})
	corecmd.ApplyGroupPolicy(aisearch, corecmd.GroupPolicy{
		Mode:        corecmd.GroupHybrid,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	root.AddCommand(aisearch)
	engine := NewEngine()
	engine.Register(reviewedProtectionTestHandler{})
	frozenParentTraversal := []string{"aisearch", "--"}
	if got := argsForCommandTraversal(root, []string{"aisearch", "enterprise"}); !slices.Equal(got, []string{"aisearch", "enterprise"}) {
		t.Fatalf("default command traversal = %v", got)
	}

	guarded := []string{"aisearch", "--types", "FIXTURE_VALUE"}
	if got := argsForCommandTraversalForEngine(root, engine, guarded); !slices.Equal(got, frozenParentTraversal) {
		t.Fatalf("guarded flag traversal = %v, want %v", got, frozenParentTraversal)
	}
	if ctx, err := RunPreParseArgs(root, engine, guarded); err != nil || ctx == nil || ctx.Command != "dws aisearch" || !ctx.IsFlagProtected("types") {
		t.Fatalf("guarded flag value entered command resolution: context=%#v error=%v", ctx, err)
	}

	// A protected flag value can also be spelled exactly like a child. Command
	// traversal must not guess which meaning the user intended.
	guardedChildValue := []string{"aisearch", "--types", "enterprise"}
	if got := argsForCommandTraversalForEngine(root, engine, guardedChildValue); !slices.Equal(got, frozenParentTraversal) {
		t.Fatalf("guarded child-name value traversal = %v, want %v", got, frozenParentTraversal)
	}
	if ctx, err := RunPreParseArgs(root, engine, guardedChildValue); err != nil || ctx == nil || ctx.Command != "dws aisearch" {
		t.Fatalf("guarded child-name value entered command resolution: context=%#v error=%v", ctx, err)
	}
	for _, guardedBooleanValue := range [][]string{
		{"aisearch", "--types", "false", "enterprise"},
		{"aisearch", "--types=false", "enterprise"},
	} {
		if got := argsForCommandTraversalForEngine(root, engine, guardedBooleanValue); !slices.Equal(got, frozenParentTraversal) {
			t.Fatalf("guarded boolean-like value traversal = %v, want %v", got, frozenParentTraversal)
		}
		if ctx, err := RunPreParseArgs(root, engine, guardedBooleanValue); err != nil || ctx == nil || ctx.Command != "dws aisearch" {
			t.Fatalf("guarded boolean-like value selected wrong command: context=%#v error=%v", ctx, err)
		}
	}
	guardedAfterLocalValue := []string{"aisearch", "--unknown-query", "Alice", "--types", "enterprise"}
	if got := argsForCommandTraversalForEngine(root, engine, guardedAfterLocalValue); !slices.Equal(got, frozenParentTraversal) {
		t.Fatalf("guarded flag after local value traversal = %v, want %v", got, frozenParentTraversal)
	}
	if ctx, err := RunPreParseArgs(root, engine, guardedAfterLocalValue); err != nil || ctx == nil || ctx.Command != "dws aisearch" {
		t.Fatalf("guarded flag after local value selected wrong command: context=%#v error=%v", ctx, err)
	}
	legitimateParentFlag := []string{"aisearch", "--query", "Alice", "enterprise", "--types", "document", "--leaf-typz", "value"}
	if ctx, err := RunPreParseArgs(root, engine, legitimateParentFlag); err != nil || ctx == nil || ctx.Command != "dws aisearch enterprise" ||
		!slices.Contains(ctx.Args, "--leaf-type") {
		t.Fatalf("parent flag/value prevented leaf resolution: context=%#v error=%v", ctx, err)
	}

	// Leading fuzzy root flags remain useful because no command path is
	// ambiguous yet. Once a group has resolved, the same fuzzy spelling is
	// preserved for the semantic flag pipeline instead of being consumed here.
	if got := argsForCommandTraversalForEngine(root, engine, []string{"--yess", "aisearch", "enterprise"}); !slices.Equal(got, []string{"aisearch", "enterprise"}) {
		t.Fatalf("leading fuzzy boolean traversal = %v", got)
	}
	if got := argsForCommandTraversalForEngine(root, engine, []string{"aisearch", "--yess", "enterprise"}); !slices.Equal(got, []string{"aisearch", "enterprise"}) {
		t.Fatalf("fuzzy boolean after group traversal = %v", got)
	}
	if got := argsForCommandTraversalForEngine(root, engine, []string{"aisearch", "--yess", "false", "enterprise"}); !slices.Equal(got, []string{"aisearch", "enterprise"}) {
		t.Fatalf("fuzzy boolean value before child traversal = %v", got)
	}
	leafArgs := []string{"aisearch", "--yess", "enterprise", "--leaf-typz", "value"}
	if ctx, err := RunPreParseArgs(root, engine, leafArgs); err != nil || ctx == nil || ctx.Command != "dws aisearch enterprise" ||
		!slices.Contains(ctx.Args, "--leaf-type") {
		t.Fatalf("fuzzy root flag skipped leaf handler: context=%#v error=%v", ctx, err)
	}

	_, err := RunPreParseArgs(root, engine, []string{"aisearch", "--yes", "enterprize"})
	requireCommandResolutionError(t, err, string(cmdutil.ResolutionUnknownSubcommand))
	_, err = RunPreParseArgs(root, engine, []string{"aisearch", "enterprize", "--types", "enterprise"})
	structured := requireCommandResolutionError(t, err, string(cmdutil.ResolutionUnknownSubcommand))
	if suggestions, ok := structured.Details["suggestions"].([]string); !ok || !slices.Equal(suggestions, []string{"enterprise"}) {
		t.Fatalf("typo before protected flag suggestions = %#v", structured.Details["suggestions"])
	}
	_, err = RunPreParseArgs(root, engine, []string{"aisearch", "--query", "Alice", "enterprize", "--types", "enterprise"})
	structured = requireCommandResolutionError(t, err, string(cmdutil.ResolutionUnknownSubcommand))
	if suggestions, ok := structured.Details["suggestions"].([]string); !ok || !slices.Equal(suggestions, []string{"enterprise"}) {
		t.Fatalf("typo after parent flag suggestions = %#v", structured.Details["suggestions"])
	}
}

func TestCrossPlatformCoverageRunPreParseArgsUsesTypedDeepRecoveryPolicy(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	sheet := &cobra.Command{Use: "sheet"}
	rangeGroup := &cobra.Command{Use: "range"}
	rangeGroup.AddCommand(&cobra.Command{Use: "read", Run: func(*cobra.Command, []string) {}})
	corecmd.ApplyGroupPolicy(rangeGroup, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	sheet.AddCommand(
		rangeGroup,
		&cobra.Command{Use: "ready", Run: func(*cobra.Command, []string) {}},
	)
	corecmd.ApplyGroupPolicy(sheet, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoveryDeep,
	})
	root.AddCommand(sheet)

	ctx, err := RunPreParseArgs(root, nil, []string{"sheet", "read", "--sheet-id", "sheet-1"})
	if ctx == nil || ctx.Command != "dws sheet" {
		t.Fatalf("Context = %#v, want unresolved target dws sheet", ctx)
	}
	structured := requireCommandResolutionError(t, err, string(cmdutil.ResolutionUnknownSubcommand))
	if structured.Message != `unknown subcommand "read" for "dws sheet"` {
		t.Fatalf("Message = %q", structured.Message)
	}
	if structured.Hint != `Did you mean "dws sheet range read"? (Run 'dws sheet --help' for the full list)` {
		t.Fatalf("Hint = %q", structured.Hint)
	}
	if !slices.Equal(structured.Actions, []string{"Run 'dws sheet --help' for the full list"}) {
		t.Fatalf("Actions = %#v", structured.Actions)
	}
	if structured.Details["input"] != "read" {
		t.Fatalf("Details.input = %#v", structured.Details["input"])
	}
	suggestions, ok := structured.Details["suggestions"].([]string)
	if !ok || !slices.Equal(suggestions, []string{"range read"}) {
		t.Fatalf("Details.suggestions = %#v", structured.Details["suggestions"])
	}
}

func TestCrossPlatformCoverageCommandSuggestionHintIsBoundedAndFallsBack(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo", SuggestionsMinimumDistance: 100}
	root.AddCommand(group)
	for _, name := range []string{"aaa", "alpha", "alphi", "alphx"} {
		group.AddCommand(&cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}})
	}
	hint := cmdutil.FormatSubcommandSuggestionHint(group, cmdutil.SuggestSubcommands(group, "alpho"), "fallback")
	if strings.Count(hint, `"dws demo `) != cmdutil.MaxCommandSuggestions || strings.Contains(hint, `"dws demo aaa"`) || !strings.Contains(hint, `"dws demo alphx"`) {
		t.Fatalf("nearest bounded hint = %q", hint)
	}

	empty := &cobra.Command{Use: "empty"}
	root.AddCommand(empty)
	if got := cmdutil.FormatSubcommandSuggestionHint(empty, cmdutil.SuggestSubcommands(empty, "missing"), "fallback"); got != "fallback" {
		t.Fatalf("fallback hint = %q", got)
	}
}

func TestCrossPlatformCoverageCommandResolutionDefensiveAndExplicitSuggestionPaths(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	service := &cobra.Command{Use: "demo"}
	group := &cobra.Command{Use: "nested"}
	service.AddCommand(group)
	root.AddCommand(service)

	if got := cmdutil.ClassifyCommandResolution(root, "+missing"); got != cmdutil.ResolutionUnknownSubcommand {
		t.Fatal("root command was treated as a shortcut service")
	}
	if got := cmdutil.ClassifyCommandResolution(group, "+missing"); got != cmdutil.ResolutionUnknownSubcommand {
		t.Fatal("nested command was treated as a top-level shortcut service")
	}
	service.AddCommand(&cobra.Command{Use: "+known", Aliases: []string{"+alias"}, Run: func(*cobra.Command, []string) {}})
	if got := cmdutil.ClassifyCommandResolution(service, "+missing"); got != cmdutil.ResolutionUnknownShortcut {
		t.Fatalf("top-level shortcut typo reason = %q", got)
	}
	if got := cmdutil.ClassifyCommandResolution(service, "+known"); got != cmdutil.ResolutionUnknownSubcommand {
		t.Fatalf("exact shortcut child was reclassified as unresolved shortcut: %q", got)
	}
	if got := cmdutil.SuggestSubcommands(nil, "missing"); got != nil {
		t.Fatalf("SuggestSubcommands(nil) = %#v", got)
	}

	service.SuggestionsMinimumDistance = 1
	service.AddCommand(&cobra.Command{
		Use:        "canonical",
		SuggestFor: []string{"invented"},
		Run:        func(*cobra.Command, []string) {},
	})
	if got := cmdutil.SuggestSubcommands(service, "invented"); len(got) != 1 || got[0] != "canonical" {
		t.Fatalf("explicit SuggestFor suggestions = %#v", got)
	}
}

func TestCrossPlatformCoverageHintSubCmdReturnsTypedRecovery(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	parent := &cobra.Command{Use: "chat"}
	hint := cmdutil.HintSubCmd("send", "use: dws chat message send")
	root.AddCommand(parent)
	parent.AddCommand(hint)

	err := hint.RunE(hint, nil)
	var structured *apperrors.Error
	if !stderrors.As(err, &structured) {
		t.Fatalf("HintSubCmd error = %T %v", err, err)
	}
	if structured.Category != apperrors.CategoryValidation || structured.Reason != "unknown_subcommand" {
		t.Fatalf("HintSubCmd error = %#v", structured)
	}
	if !strings.Contains(structured.Hint, "dws chat message send") || !strings.Contains(structured.Hint, "dws chat --help") {
		t.Fatalf("HintSubCmd hint = %q", structured.Hint)
	}
	if !cmdutil.IsHintOnlyCommand(hint) {
		t.Fatal("HintSubCmd lost hint-only identity")
	}
}

func TestCrossPlatformCoverageHintSubCmdAndGroupRunEDefensiveBranches(t *testing.T) {
	standaloneHint := cmdutil.HintSubCmd("send", " \t ")
	structured := requireCommandResolutionError(t, standaloneHint.RunE(standaloneHint, nil), "unknown_subcommand")
	if structured.Hint != "Run 'send --help' for the full list" {
		t.Fatalf("standalone empty hint = %q", structured.Hint)
	}

	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo"}
	group.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	structured = requireCommandResolutionError(t, cmdutil.GroupRunE(group, []string{"lisst"}), "unknown_subcommand")
	if structured.Details["input"] != "lisst" {
		t.Fatalf("GroupRunE() details.input = %#v", structured.Details["input"])
	}
	suggestions, ok := structured.Details["suggestions"].([]string)
	if !ok || len(suggestions) != 1 || suggestions[0] != "list" {
		t.Fatalf("GroupRunE() details.suggestions = %#v", structured.Details["suggestions"])
	}

	var help strings.Builder
	group.SetOut(&help)
	if err := cmdutil.GroupRunE(group, nil); err != nil {
		t.Fatalf("GroupRunE() help error = %v", err)
	}
	if output := help.String(); !strings.Contains(output, "Usage:") || !strings.Contains(output, "list") {
		t.Fatalf("GroupRunE() help output = %q", output)
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

func applyTestNavigationGroup(command *cobra.Command) {
	corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
}
