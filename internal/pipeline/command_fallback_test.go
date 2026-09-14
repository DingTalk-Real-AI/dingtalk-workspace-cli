// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRunPreParseArgsRewritesReviewedCommandBeforeFlags(t *testing.T) {
	root, executed := commandFallbackPipelineRoot()
	engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat +bad": {From: "chat +bad", Mode: "rewrite", To: "chat +good"},
	})
	raw := []string{"--format", "json", "chat", "+bad", "--query", "project"}
	ctx, err := RunPreParseArgs(root, engine, raw)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"--format", "json", "chat", "+good", "--query", "project"}
	if ctx == nil || !reflect.DeepEqual(ctx.Args, wantArgs) || ctx.Command != "dws chat +good" {
		t.Fatalf("context = %#v, want args=%v command=dws chat +good", ctx, wantArgs)
	}
	if len(ctx.Corrections) != 1 || ctx.Corrections[0].Handler != "command-path-fallback" ||
		ctx.Corrections[0].Original != "chat +bad" || ctx.Corrections[0].Corrected != "chat +good" {
		t.Fatalf("corrections = %#v", ctx.Corrections)
	}
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if *executed != "dws chat +good:project" {
		t.Fatalf("executed = %q", *executed)
	}
}

func TestCrossPlatformCoverageCommandFallbackPrecedesLaterProtectedFlag(t *testing.T) {
	root, _ := commandFallbackPipelineRoot()
	root.PersistentFlags().Bool("yes", false, "")
	engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat +bad": {From: "chat +bad", Mode: "rewrite", To: "chat +good"},
	})
	engine.Register(reviewedProtectionTestHandler{})
	raw := []string{"chat", "+bad", "--types", "value"}
	ctx, err := RunPreParseArgs(root, engine, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"chat", "+good", "--types", "value"}
	if ctx == nil || ctx.Command != "dws chat +good" || !reflect.DeepEqual(ctx.Args, want) {
		t.Fatalf("fallback before protected flag = %#v, want args=%v", ctx, want)
	}
}

func TestCrossPlatformCoverageRunPreParseArgsRewritesMultiTokenPathAroundPersistentFlags(t *testing.T) {
	root, executed := commandFallbackPipelineRoot()
	engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat group search": {From: "chat group search", Mode: "rewrite", To: "chat search"},
	})
	raw := []string{"chat", "--format", "json", "group", "search", "--query", "project"}
	ctx, err := RunPreParseArgs(root, engine, raw)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"chat", "search", "--format", "json", "--query", "project"}
	if ctx == nil || !reflect.DeepEqual(ctx.Args, wantArgs) {
		t.Fatalf("rewritten args = %#v, want %v", ctx, wantArgs)
	}
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if *executed != "dws chat search:project" {
		t.Fatalf("executed = %q", *executed)
	}
}

func TestCrossPlatformCoverageCommandPathFallbackDefersToExactRunnableCommandAndAlias(t *testing.T) {
	if exactRunnableCommandPath(nil, []string{"dws"}) {
		t.Fatal("nil root unexpectedly resolved an executable path")
	}
	emptyRoot := &cobra.Command{Use: "dws"}
	if exactRunnableCommandPath(emptyRoot, []string{"dws"}) {
		t.Fatal("root-only path unexpectedly resolved as an executable leaf")
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "primary", path: "+members"},
		{name: "alias", path: "+runtime-members"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executed := ""
			root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			chat := &cobra.Command{Use: "chat"}
			members := &cobra.Command{
				Use:     "+members",
				Aliases: []string{"+runtime-members"},
				RunE: func(cmd *cobra.Command, _ []string) error {
					sentinel, _ := cmd.Flags().GetString("sentinel")
					executed = cmd.CommandPath() + ":" + sentinel
					return nil
				},
			}
			members.Flags().String("sentinel", "", "")
			chat.AddCommand(members)
			root.AddCommand(chat)
			engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
				"chat " + test.path: {From: "chat " + test.path, Mode: "rewrite", To: "chat +good"},
			})
			args := []string{"chat", test.path, "--sentinel", "fixture"}
			root.SetArgs(args)
			ctx, err := RunPreParseArgs(root, engine, args)
			if err != nil {
				t.Fatal(err)
			}
			if ctx != nil {
				t.Fatalf("runtime command unexpectedly entered correction pipeline: %#v", ctx)
			}
			if _, err := root.ExecuteC(); err != nil {
				t.Fatal(err)
			}
			if executed != "dws chat +members:fixture" {
				t.Fatalf("runtime command execution = %q", executed)
			}
		})
	}
}

func TestCrossPlatformCoverageCommandPathFallbackMaySupersedeHintOnlyCommand(t *testing.T) {
	root, executed := commandFallbackPipelineRoot()
	chat, _, err := root.Find([]string{"chat"})
	if err != nil {
		t.Fatal(err)
	}
	chat.AddCommand(cmdutil.HintSubCmd("+bad", "use the reviewed recovery"))
	engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat +bad": {From: "chat +bad", Mode: "rewrite", To: "chat +good"},
	})
	args := []string{"chat", "+bad", "--query", "project"}
	root.SetArgs(args)
	ctx, err := RunPreParseArgs(root, engine, args)
	if err != nil {
		t.Fatal(err)
	}
	if ctx == nil || ctx.Command != "dws chat +good" || len(ctx.Corrections) != 1 {
		t.Fatalf("hint-only fallback context = %#v", ctx)
	}
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if *executed != "dws chat +good:project" {
		t.Fatalf("hint-only fallback execution = %q", *executed)
	}
}

func TestCrossPlatformCoverageRunPreParseArgsRejectsAmbiguousCommandFallbackWithoutDispatch(t *testing.T) {
	root, executed := commandFallbackPipelineRoot()
	engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat +choose": {
			From:       "chat +choose",
			Mode:       "ambiguous",
			Candidates: []string{"chat +good", "chat +other"},
		},
	})
	ctx, err := RunPreParseArgs(root, engine, []string{"--format", "json", "chat", "+choose", "--query", "project"})
	if ctx == nil {
		t.Fatal("ambiguous fallback returned nil context")
	}
	structured := requireCommandResolutionError(t, err, "ambiguous_command_fallback")
	if len(structured.Actions) != 2 || !strings.Contains(structured.Hint, "dws chat +good") || !strings.Contains(structured.Hint, "dws chat +other") {
		t.Fatalf("ambiguous recovery = %#v", structured)
	}
	if *executed != "" {
		t.Fatalf("ambiguous fallback dispatched %q", *executed)
	}
	if format, getErr := root.PersistentFlags().GetString("format"); getErr != nil || format != "json" {
		t.Fatalf("format = %q, %v; want primed json", format, getErr)
	}
}

func TestCrossPlatformCoverageCommandPathFallbackLeavesUnknownAndCanonicalFlagErrorsDistinct(t *testing.T) {
	root, _ := commandFallbackPipelineRoot()
	engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat +bad": {From: "chat +bad", Mode: "rewrite", To: "chat +good"},
	})
	_, err := RunPreParseArgs(root, engine, []string{"chat", "+missing", "--query", "project"})
	requireCommandResolutionError(t, err, "unknown_shortcut")

	validRoot, _ := commandFallbackPipelineRoot()
	validRaw := []string{"chat", "+good", "--not-a-flag", "value"}
	if ctx, err := RunPreParseArgs(validRoot, engine, validRaw); ctx != nil || err != nil {
		t.Fatalf("valid command preparse = %#v, %v", ctx, err)
	}
	validRoot.SetArgs(validRaw)
	_, executeErr := validRoot.ExecuteC()
	if executeErr == nil || !strings.Contains(executeErr.Error(), "unknown flag") {
		t.Fatalf("canonical bad flag error = %v", executeErr)
	}
}

func TestCrossPlatformCoverageCommandPathFallbackCannotBypassCanonicalConfirmation(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         []string
		wantDispatch int
		wantErr      bool
	}{
		{name: "confirmation missing", args: []string{"chat", "+danger"}, wantErr: true},
		{name: "confirmed", args: []string{"chat", "+danger", "--yes"}, wantDispatch: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			dispatches := 0
			root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			chat := &cobra.Command{Use: "chat"}
			write := &cobra.Command{
				Use: "+write",
				RunE: func(cmd *cobra.Command, _ []string) error {
					confirmed, _ := cmd.Flags().GetBool("yes")
					if !confirmed {
						return apperrors.NewValidation("confirmation required", apperrors.WithReason("confirmation_required"))
					}
					dispatches++
					return nil
				},
			}
			write.Flags().Bool("yes", false, "")
			chat.AddCommand(write)
			root.AddCommand(chat)
			engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
				"chat +danger": {From: "chat +danger", Mode: "rewrite", To: "chat +write"},
			})
			if _, err := RunPreParseArgs(root, engine, test.args); err != nil {
				t.Fatal(err)
			}
			_, err := root.ExecuteC()
			if (err != nil) != test.wantErr || dispatches != test.wantDispatch {
				t.Fatalf("execute error=%v dispatches=%d, want error=%v dispatches=%d", err, dispatches, test.wantErr, test.wantDispatch)
			}
		})
	}
}

func TestCrossPlatformCoverageCommandPathFallbackRejectsInvalidGeneratedMode(t *testing.T) {
	root, _ := commandFallbackPipelineRoot()
	for _, entry := range []CommandPathFallback{
		{From: "chat +bad", Mode: "rewrite"},
		{From: "chat +bad", Mode: "invalid", To: "chat +good"},
	} {
		engine := commandFallbackPipelineEngine(map[string]CommandPathFallback{"chat +bad": entry})
		_, err := RunPreParseArgs(root, engine, []string{"chat", "+bad"})
		var structured *apperrors.Error
		if !errors.As(err, &structured) || structured.Category != apperrors.CategoryInternal {
			t.Fatalf("invalid entry error = %T %v", err, err)
		}
	}
}

func TestCrossPlatformCoverageRunPreParseArgsKeepsCommandErrorWhenPresentationPrimingFails(t *testing.T) {
	fallbackRoot, _ := commandFallbackPipelineRoot()
	fallbackEngine := commandFallbackPipelineEngine(map[string]CommandPathFallback{
		"chat +choose": {
			From:       "chat +choose",
			Mode:       "ambiguous",
			Candidates: []string{"chat +good", "chat +other"},
		},
	})
	_, err := RunPreParseArgs(fallbackRoot, fallbackEngine, []string{"chat", "+choose", "--format"})
	requireCommandResolutionError(t, err, "ambiguous_command_fallback")

	resolutionRoot, _ := commandFallbackPipelineRoot()
	_, err = RunPreParseArgs(resolutionRoot, nil, []string{"chat", "+missing", "--format"})
	requireCommandResolutionError(t, err, "unknown_shortcut")

	handlerRoot, _ := commandFallbackPipelineRoot()
	handlerEngine := NewEngine()
	handlerEngine.Register(newStub("fail-before-presentation", PreParse, func(*Context) error {
		return errors.New("handler failure")
	}))
	ctx, err := RunPreParseArgs(handlerRoot, handlerEngine, []string{"chat", "+good", "--query", "project", "--format"})
	if ctx == nil || err == nil || !strings.Contains(err.Error(), "handler failure") {
		t.Fatalf("handler error after presentation failure = context %#v, error %v", ctx, err)
	}
}

func TestCrossPlatformCoverageCommandFallbackTraversalRemainingBranches(t *testing.T) {
	args, positions := argsForCommandTraversalWithPositions(nil, []string{"chat", "+good"})
	if !reflect.DeepEqual(args, []string{"chat", "+good"}) || !reflect.DeepEqual(positions, []int{0, 1}) {
		t.Fatalf("nil-root traversal = args %v, positions %v", args, positions)
	}

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "table", "")
	raw := []string{"chat", "+good", "--", "--format", "json"}
	args, positions = argsForCommandTraversalWithPositions(root, raw)
	if !reflect.DeepEqual(args, raw) || !reflect.DeepEqual(positions, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("terminator traversal = args %v, positions %v", args, positions)
	}
}

func TestCrossPlatformCoverageRewriteCommandPathTokensLeavesArgsWhenRewriteIsEmpty(t *testing.T) {
	raw := []string{"chat", "+bad", "--query", "project"}
	for name, test := range map[string]struct {
		positions []int
		tokens    []string
	}{
		"no command positions": {tokens: []string{"chat", "+good"}},
		"no canonical tokens":  {positions: []int{0, 1}},
	} {
		t.Run(name, func(t *testing.T) {
			got := rewriteCommandPathTokens(raw, test.positions, test.tokens)
			if !reflect.DeepEqual(got, raw) {
				t.Fatalf("rewriteCommandPathTokens() = %v, want %v", got, raw)
			}
			got[0] = "mutated"
			if raw[0] != "chat" {
				t.Fatal("empty rewrite returned aliased argv")
			}
		})
	}
}

func commandFallbackPipelineEngine(entries map[string]CommandPathFallback) *Engine {
	engine := NewEngine()
	engine.SetCommandPathFallbackLookup(func(path string) (CommandPathFallback, bool) {
		entry, ok := entries[path]
		return entry, ok
	})
	return engine
}

func commandFallbackPipelineRoot() (*cobra.Command, *string) {
	executed := new(string)
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "table", "")
	chat := &cobra.Command{Use: "chat"}
	good := &cobra.Command{
		Use: "+good",
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, _ := cmd.Flags().GetString("query")
			*executed = cmd.CommandPath() + ":" + query
			return nil
		},
	}
	good.Flags().String("query", "", "")
	good.Flags().String("keyword", "", "")
	other := &cobra.Command{Use: "+other", Run: func(cmd *cobra.Command, _ []string) { *executed = cmd.CommandPath() }}
	search := &cobra.Command{
		Use: "search",
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, _ := cmd.Flags().GetString("query")
			*executed = cmd.CommandPath() + ":" + query
			return nil
		},
	}
	search.Flags().String("query", "", "")
	group := &cobra.Command{Use: "group"}
	group.AddCommand(cmdutil.HintSubCmd("search", "use dws chat search"))
	chat.AddCommand(good, other, search, group)
	corecmd.ApplyGroupPolicy(group, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	corecmd.ApplyGroupPolicy(chat, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	root.AddCommand(chat)
	return root, executed
}
