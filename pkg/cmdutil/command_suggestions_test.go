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

func TestCrossPlatformCoverageSuggestSubcommandsRanksNearestAndBoundsResults(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo", SuggestionsMinimumDistance: 100}
	root.AddCommand(group)
	for _, name := range []string{"aaa", "alpha", "alphi", "alphx"} {
		group.AddCommand(&cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}})
	}
	group.AddCommand(&cobra.Command{Use: "alpho-hidden", Hidden: true, Run: func(*cobra.Command, []string) {}})

	want := []string{"alpha", "alphi", "alphx"}
	if got := SuggestSubcommands(group, "alpho"); !slices.Equal(got, want) {
		t.Fatalf("SuggestSubcommands() = %#v, want nearest %#v", got, want)
	}
}

func TestCrossPlatformCoverageSuggestSubcommandsUsesAliasesAndReviewedSuggestions(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo", SuggestionsMinimumDistance: 1}
	root.AddCommand(group)
	group.AddCommand(
		&cobra.Command{Use: "canonical", Aliases: []string{"metas"}, Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "reviewed", SuggestFor: []string{"legacy-command"}, Run: func(*cobra.Command, []string) {}},
	)

	if got := SuggestSubcommands(group, "meta"); !slices.Equal(got, []string{"canonical"}) {
		t.Fatalf("alias suggestion = %#v, want canonical command", got)
	}
	if got := SuggestSubcommands(group, "legacy-command"); !slices.Equal(got, []string{"reviewed"}) {
		t.Fatalf("reviewed SuggestFor = %#v", got)
	}
}

func TestCrossPlatformCoverageGroupRunERemainsConciseWithoutSuggestions(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo"}
	root.AddCommand(group)
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		group.AddCommand(&cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}})
	}

	err := GroupRunE(group, []string{"zzzzz"})
	var structured *apperrors.Error
	if !errors.As(err, &structured) || structured.Reason != string(ResolutionUnknownSubcommand) ||
		strings.Contains(structured.Hint, "available:") || structured.Hint != "Run 'dws demo --help' for the full list" {
		t.Fatalf("GroupRunE() error = %v, want concise parent-help guidance", err)
	}
	assertResolutionDetails(t, structured, "zzzzz", []string{})
}

func TestCrossPlatformCoverageFormatSubcommandSuggestionHintDefensivelyBoundsInput(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	group := &cobra.Command{Use: "demo"}
	root.AddCommand(group)
	hint := FormatSubcommandSuggestionHint(group, []string{"one", "two", "three", "four"}, "fallback")
	if strings.Count(hint, `"dws demo `) != MaxCommandSuggestions || strings.Contains(hint, "four") {
		t.Fatalf("unbounded formatted hint = %q", hint)
	}
	if hint := FormatSubcommandSuggestionHint(group, []string{"one"}, "fallback"); hint != `Did you mean "dws demo one"? (fallback)` {
		t.Fatalf("single suggestion hint = %q", hint)
	}
}

func TestCrossPlatformCoverageSuggestSubcommandsDefensiveInputs(t *testing.T) {
	if got := SuggestSubcommands(nil, "candidate"); got != nil {
		t.Fatalf("SuggestSubcommands(nil) = %#v, want nil", got)
	}

	group := &cobra.Command{Use: "demo"}
	group.AddCommand(&cobra.Command{
		Use:     "target",
		Aliases: []string{""},
		Run:     func(*cobra.Command, []string) {},
	})
	if got := SuggestSubcommands(group, " \t "); len(got) != 0 {
		t.Fatalf("empty candidate suggestions = %#v", got)
	}
	if got := SuggestSubcommands(group, "target"); !slices.Equal(got, []string{"target"}) {
		t.Fatalf("empty alias changed canonical suggestion = %#v", got)
	}

	blankGroup := &cobra.Command{Use: "demo", SuggestionsMinimumDistance: 100}
	blankGroup.AddCommand(&cobra.Command{Use: "", Run: func(*cobra.Command, []string) {}})
	if got := SuggestSubcommands(blankGroup, "candidate"); len(got) != 0 {
		t.Fatalf("blank command identity suggestions = %#v", got)
	}
}

func TestCrossPlatformCoverageSuggestSubcommandsRankingTieBreakers(t *testing.T) {
	newGroup := func() *cobra.Command {
		return &cobra.Command{Use: "demo", SuggestionsMinimumDistance: 100}
	}

	explicit := newGroup()
	explicit.AddCommand(
		&cobra.Command{Use: "reviewed", SuggestFor: []string{"legacy"}, Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "ordinary", Run: func(*cobra.Command, []string) {}},
	)
	if got := SuggestSubcommands(explicit, "legacy"); len(got) < 2 || got[0] != "reviewed" {
		t.Fatalf("explicit ranking = %#v", got)
	}

	prefix := newGroup()
	prefix.AddCommand(
		&cobra.Command{Use: "alpha", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "xlp", Run: func(*cobra.Command, []string) {}},
	)
	if got := SuggestSubcommands(prefix, "alp"); len(got) < 2 || got[0] != "alpha" {
		t.Fatalf("prefix ranking = %#v", got)
	}

	lengthDelta := newGroup()
	lengthDelta.AddCommand(
		&cobra.Command{Use: "ab", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "axc", Run: func(*cobra.Command, []string) {}},
	)
	if got := SuggestSubcommands(lengthDelta, "abc"); !slices.Equal(got, []string{"axc", "ab"}) {
		t.Fatalf("length-delta ranking = %#v", got)
	}
}

func TestCrossPlatformCoverageSuggestDescendantSubcommandsExactCanonicalAliasSortedAndBounded(t *testing.T) {
	root := &cobra.Command{Use: "sheet"}
	addGroup := func(name string, hidden bool) *cobra.Command {
		group := &cobra.Command{Use: name, Hidden: hidden, Run: func(*cobra.Command, []string) {}}
		root.AddCommand(group)
		return group
	}
	addLeaf := func(group *cobra.Command, use string, aliases []string, hidden, runnable bool) {
		leaf := &cobra.Command{Use: use, Aliases: aliases, Hidden: hidden}
		if runnable {
			leaf.Run = func(*cobra.Command, []string) {}
		}
		group.AddCommand(leaf)
	}

	// Insert matching paths deliberately out of order. The visible alias match
	// must teach its canonical path, and the result must be sorted before it is
	// capped at the shared three-suggestion limit.
	addLeaf(addGroup("zeta", false), "read", nil, false, true)
	addLeaf(addGroup("gamma", false), "read", nil, false, true)
	addLeaf(addGroup("alpha", false), "fetch", []string{"read"}, false, true)
	addLeaf(addGroup("beta", false), "read", nil, false, true)

	// These paths sort before the expected results if they leak through, so the
	// assertion also proves hidden and unavailable descendants are excluded
	// before bounding rather than merely falling beyond the cap.
	addLeaf(addGroup("aaa-hidden-group", true), "read", nil, false, true)
	addLeaf(addGroup("aab-unavailable", false), "read", nil, false, false)
	addLeaf(addGroup("aac-hidden-leaf", false), "read", nil, true, true)

	want := []string{"alpha fetch", "beta read", "gamma read"}
	if got := SuggestDescendantSubcommands(root, " READ "); !slices.Equal(got, want) {
		t.Fatalf("SuggestDescendantSubcommands() = %#v, want %#v", got, want)
	}
	if got := SuggestDescendantSubcommands(root, "rea"); len(got) != 0 {
		t.Fatalf("fuzzy descendant suggestions = %#v, want exact matching only", got)
	}
	if got := SuggestDescendantSubcommands(nil, "read"); got != nil {
		t.Fatalf("nil parent suggestions = %#v", got)
	}
	if got := SuggestDescendantSubcommands(root, " \t "); len(got) != 0 {
		t.Fatalf("blank candidate suggestions = %#v", got)
	}
}
