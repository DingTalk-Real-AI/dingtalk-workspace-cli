// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// realMap builds a per-leaf real-flag table keyed by the shared Morph so the
// tests exercise exactly the same intersection the generator performs.
func realMap(flags ...realFlag) map[string][]realFlag {
	m := make(map[string][]realFlag)
	for _, f := range flags {
		k := cmdutil.Morph(f.name)
		m[k] = appendRealFlag(m[k], f)
	}
	return m
}

// conceptFixture is a small synthetic concept set; reduceLeafParamAliases is
// deliberately pure so it can be tested without the whole Cobra tree.
func conceptFixture() []Concept {
	return []Concept{
		{ID: "pagination_size", CanonicalHint: "limit", Members: []string{"limit", "size", "page-size", "max-results"}},
		{ID: "base_id", CanonicalHint: "base-id", Members: []string{"base", "base-id", "base-token"}},
		{ID: "user_id", CanonicalHint: "user-id", Members: []string{"user", "users", "user-id", "uid"}},
	}
}

func useParamConceptLoader(t *testing.T, concepts ParamConcepts, err error) {
	t.Helper()
	testseam.Swap(t, &loadReviewedParamConcepts, func() (ParamConcepts, error) { return concepts, err })
}

func TestReduceParamAliasesLoadsAndValidatesSourceTree(t *testing.T) {
	t.Run("load failure", func(t *testing.T) {
		useParamConceptLoader(t, ParamConcepts{}, errors.New("fixture load"))
		if _, err := ReduceParamAliases(&cobra.Command{Use: "dws"}); err == nil || !strings.Contains(err.Error(), "fixture load") {
			t.Fatalf("ReduceParamAliases() error = %v", err)
		}
	})

	t.Run("nil root", func(t *testing.T) {
		useParamConceptLoader(t, ParamConcepts{Version: 1}, nil)
		if _, err := ReduceParamAliases(nil); err == nil || !strings.Contains(err.Error(), "root is nil") {
			t.Fatalf("ReduceParamAliases(nil) error = %v", err)
		}
	})

	t.Run("real tree", func(t *testing.T) {
		concepts := ParamConcepts{
			Version: 1,
			Concepts: []Concept{
				{ID: "query", Members: []string{"query", "keyword"}, Commands: []string{"demo run"}},
				{ID: "user_id", Members: []string{"user-id", "uid"}, Commands: []string{"demo run"}},
			},
			Overrides: []CommandOverride{
				{CommandPath: "alpha", Block: []string{"unsafe"}},
				{CommandPath: "demo run", Bind: map[string]string{"id": "user_id"}},
			},
		}
		useParamConceptLoader(t, concepts, nil)

		root := &cobra.Command{Use: "dws"}
		alpha := &cobra.Command{Use: "alpha", Run: func(*cobra.Command, []string) {}}
		alpha.Flags().String("name", "", "name")
		demo := &cobra.Command{Use: "demo", Run: func(*cobra.Command, []string) {}}
		run := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
		run.Flags().String("query", "", "query")
		run.Flags().String("id", "", "id")
		demo.AddCommand(run)
		root.AddCommand(alpha, demo)
		root.AddCommand(&cobra.Command{Use: "help", Run: func(*cobra.Command, []string) {}})
		root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}})

		entries, err := ReduceParamAliases(root)
		if err != nil {
			t.Fatalf("ReduceParamAliases() error = %v", err)
		}
		if len(entries) != 2 || entries[0].CLIPath != "alpha" || entries[1].CLIPath != "demo run" {
			t.Fatalf("entries = %#v", entries)
		}
		if entries[1].Aliases["keyword"] != "query" || entries[1].Aliases["uid"] != "id" {
			t.Fatalf("demo aliases = %#v", entries[1].Aliases)
		}
	})

	t.Run("stale and unbound review inputs", func(t *testing.T) {
		concepts := ParamConcepts{
			Version: 1,
			Concepts: []Concept{
				{ID: "missing", Members: []string{"missing"}, Commands: []string{"demo run"}},
				{ID: "stale", Members: []string{"stale"}, Commands: []string{"ghost run"}},
			},
			Overrides: []CommandOverride{{CommandPath: "ghost run", Block: []string{"unsafe"}}},
		}
		useParamConceptLoader(t, concepts, nil)
		root := &cobra.Command{Use: "dws"}
		demo := &cobra.Command{Use: "demo"}
		run := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
		run.Flags().String("query", "", "query")
		demo.AddCommand(run)
		root.AddCommand(demo)

		_, err := ReduceParamAliases(root)
		for _, want := range []string{"has no matching real flag", "does not match any runnable Cobra leaf", "does not match any runnable Cobra command"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("ReduceParamAliases() error = %v, want %q", err, want)
			}
		}
	})
}

func TestParamAliasHelperEdges(t *testing.T) {
	walkRunnableParamCommands(nil, func(*cobra.Command) { t.Fatal("nil root was visited") })
	helpOnly := &cobra.Command{Use: "demo"}
	helpOnly.Flags().String("help", "", "help")
	if got := realFlagsByMorph(helpOnly); len(got) != 0 {
		t.Fatalf("help flag entered the real parameter table: %#v", got)
	}

	flags := []realFlag{{name: "same", hidden: true}}
	flags = appendRealFlag(flags, realFlag{name: "same"})
	if len(flags) != 1 || flags[0].hidden {
		t.Fatalf("visible duplicate did not replace hidden registration: %#v", flags)
	}
	flags = appendRealFlag(flags, realFlag{name: "same", hidden: true})
	if len(flags) != 1 || flags[0].hidden {
		t.Fatalf("hidden duplicate replaced visible registration: %#v", flags)
	}
	flags = appendRealFlag(flags, realFlag{name: "alpha"})
	if !reflect.DeepEqual([]string{flags[0].name, flags[1].name}, []string{"alpha", "same"}) {
		t.Fatalf("new real flags are not sorted: %#v", flags)
	}

	candidates := []realFlag{{name: "hidden", hidden: true}, {name: "visible"}, {name: "visible"}}
	if got := distinctRealNames(candidates, true); !reflect.DeepEqual(got, []string{"visible"}) {
		t.Fatalf("visible names = %v", got)
	}
	if got := canonicalRealName([]realFlag{{name: "hidden", hidden: true}}); got != "hidden" {
		t.Fatalf("hidden-only canonical = %q", got)
	}
	if got := canonicalRealName(nil); got != "" {
		t.Fatalf("empty canonical = %q", got)
	}
	if got := sortedUnique(nil); got != nil {
		t.Fatalf("sortedUnique(nil) = %#v", got)
	}
	if got := sortedUnique([]string{"b", "a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("sortedUnique() = %v", got)
	}

	real := realMap(realFlag{name: "query"}, realFlag{name: "id"})
	if !conceptHasRealFlag(Concept{ID: "query", Members: []string{"query"}}, CommandOverride{}, real) {
		t.Fatal("concept member did not match a real flag")
	}
	if !conceptHasRealFlag(Concept{ID: "user", Members: []string{"user-id"}}, CommandOverride{Bind: map[string]string{"id": "user"}}, real) {
		t.Fatal("reviewed bind did not match a real flag")
	}
	if conceptHasRealFlag(Concept{ID: "user", Members: []string{"user-id"}}, CommandOverride{Bind: map[string]string{"missing": "user"}}, real) {
		t.Fatal("missing reviewed bind matched a real flag")
	}
}

func TestReduceLeafParamAliasesRemainingEdges(t *testing.T) {
	t.Run("pending reviewed bind blocks non-real members", func(t *testing.T) {
		entry, problems := reduceLeafParamAliases(
			"demo cmd",
			realMap(realFlag{name: "id"}),
			[]Concept{{ID: "user_id", Members: []string{"user-id", "uid"}}},
			CommandOverride{Bind: map[string]string{"id": "user_id"}, Investigate: true},
		)
		if len(problems) != 0 || entry == nil ||
			!containsParamAlias(entry.Blocked, "user-id") || !containsParamAlias(entry.Blocked, "uid") {
			t.Fatalf("pending bind entry = %#v, problems = %v", entry, problems)
		}
	})

	t.Run("hidden-only canonical", func(t *testing.T) {
		entry, problems := reduceLeafParamAliases(
			"demo cmd",
			realMap(realFlag{name: "query", hidden: true}),
			[]Concept{{ID: "query", Members: []string{"query", "keyword"}}},
			CommandOverride{},
		)
		if len(problems) != 0 || entry == nil || entry.Aliases["keyword"] != "query" {
			t.Fatalf("hidden-only entry = %#v, problems = %v", entry, problems)
		}
	})

	t.Run("multiple hidden candidates stay unresolved", func(t *testing.T) {
		entry, problems := reduceLeafParamAliases(
			"demo cmd",
			realMap(realFlag{name: "first", hidden: true}, realFlag{name: "second", hidden: true}),
			[]Concept{{ID: "choice", Members: []string{"first", "second", "choice"}}},
			CommandOverride{},
		)
		if len(problems) != 0 || entry != nil {
			t.Fatalf("multiple hidden candidates entry = %#v, problems = %v", entry, problems)
		}
	})

	t.Run("two concepts cannot claim one emitted spelling", func(t *testing.T) {
		_, problems := reduceLeafParamAliases(
			"demo cmd",
			realMap(realFlag{name: "first"}, realFlag{name: "second"}),
			[]Concept{
				{ID: "first", Members: []string{"first", "shared"}},
				{ID: "second", Members: []string{"second", "shared"}},
			},
			CommandOverride{},
		)
		if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "reduces to both") {
			t.Fatalf("alias collision problems = %v", problems)
		}
	})

	t.Run("unclaimed exclude becomes blocked", func(t *testing.T) {
		entry, problems := reduceLeafParamAliases(
			"demo cmd",
			realMap(realFlag{name: "query"}),
			[]Concept{{ID: "query", Members: []string{"query", "keyword"}, Excludes: []string{"name"}}},
			CommandOverride{},
		)
		if len(problems) != 0 || entry == nil || !containsParamAlias(entry.Blocked, "name") {
			t.Fatalf("exclude entry = %#v, problems = %v", entry, problems)
		}
	})
}

func TestParamAliasEntryLookupMethods(t *testing.T) {
	entry := ParamAliasEntry{
		Aliases:   map[string]string{"uid": "user"},
		Blocked:   []string{"count"},
		Ambiguous: []string{"user-id"},
	}
	if got, ok := entry.ResolveAlias("uid"); !ok || got != "user" {
		t.Fatalf("ResolveAlias(uid) = %q, %v", got, ok)
	}
	if _, ok := entry.ResolveAlias("missing"); ok {
		t.Fatal("ResolveAlias(missing) unexpectedly matched")
	}
	if !entry.IsBlocked("count") || entry.IsBlocked("missing") {
		t.Fatalf("blocked lookup mismatch: %#v", entry.Blocked)
	}
	if !entry.IsAmbiguous("user-id") || entry.IsAmbiguous("missing") {
		t.Fatalf("ambiguous lookup mismatch: %#v", entry.Ambiguous)
	}
}

func TestReduceLeafParamAliasesAutoReduction(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "limit"}), conceptFixture(), CommandOverride{})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry == nil {
		t.Fatal("expected a reduced entry")
	}
	for _, emitted := range []string{"size", "page-size", "max-results"} {
		if entry.Aliases[emitted] != "limit" {
			t.Fatalf("alias %q = %q, want limit", emitted, entry.Aliases[emitted])
		}
	}
	if _, ok := entry.Aliases["limit"]; ok {
		t.Fatal("the real flag limit must never be an alias key")
	}
}

func TestReduceLeafParamAliasesCoOccurrenceRequiresReview(t *testing.T) {
	_, problems := reduceLeafParamAliases("demo cmd",
		realMap(realFlag{name: "user"}, realFlag{name: "users"}), conceptFixture(), CommandOverride{})
	if len(problems) == 0 {
		t.Fatal("two visible real flags for one concept must fail without a reviewed ambiguous whitelist")
	}
}

func TestReduceLeafParamAliasesAmbiguousWhitelist(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd",
		realMap(realFlag{name: "user"}, realFlag{name: "users"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Ambiguous: []string{"user-id", "uid"}})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry == nil {
		t.Fatal("expected a reduced entry")
	}
	if _, ok := entry.Aliases["user-id"]; ok {
		t.Fatal("a reviewed co-occurrence must not auto-reduce its concept members")
	}
	if len(entry.Ambiguous) != 2 || entry.Ambiguous[0] != "uid" || entry.Ambiguous[1] != "user-id" {
		t.Fatalf("ambiguous = %v, want sorted [uid user-id]", entry.Ambiguous)
	}
}

// TestReduceLeafParamAliasesCoOccurrencePerConcept locks the per-concept guard:
// reviewing one concept's co-occurrence must not silently vouch for a second,
// unreviewed co-occurring concept on the same command. Here user_id (user +
// users) is whitelisted while base_id (base + base-id) is not, so base_id's
// unreviewed emittable member base-token must still fail generation.
func TestReduceLeafParamAliasesCoOccurrencePerConcept(t *testing.T) {
	real := realMap(
		realFlag{name: "user"}, realFlag{name: "users"},
		realFlag{name: "base"}, realFlag{name: "base-id"},
	)
	_, problems := reduceLeafParamAliases("demo cmd", real, conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Ambiguous: []string{"user-id", "uid"}})
	if len(problems) == 0 {
		t.Fatal("an unreviewed second co-occurring concept must fail even when another concept is whitelisted")
	}
	found := false
	for _, p := range problems {
		if strings.Contains(p, `concept "base_id"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a problem naming the unreviewed base_id concept, got: %v", problems)
	}
}

// TestReduceLeafParamAliasesCoOccurrenceBothReviewed confirms the per-concept
// guard passes once every co-occurring concept's emittable members are listed.
func TestReduceLeafParamAliasesCoOccurrenceBothReviewed(t *testing.T) {
	real := realMap(
		realFlag{name: "user"}, realFlag{name: "users"},
		realFlag{name: "base"}, realFlag{name: "base-id"},
	)
	_, problems := reduceLeafParamAliases("demo cmd", real, conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Ambiguous: []string{"user-id", "uid", "base-token"}})
	if len(problems) != 0 {
		t.Fatalf("both concepts reviewed should pass: %v", problems)
	}
}

// TestReduceLeafParamAliasesRejectsAliasAmbiguousOverlap locks the guard that a
// single name cannot be both auto-reduced and marked ambiguous. Here only
// --users is real, so user_id auto-reduces user-id to users; hand-listing
// user-id as ambiguous would produce a self-contradictory entry.
func TestReduceLeafParamAliasesRejectsAliasAmbiguousOverlap(t *testing.T) {
	_, problems := reduceLeafParamAliases("demo cmd",
		realMap(realFlag{name: "users"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Ambiguous: []string{"user-id"}})
	if len(problems) == 0 {
		t.Fatal("a name that both auto-reduces and is listed ambiguous must fail")
	}
}

func TestReduceLeafParamAliasesAbsorbsHiddenLegacyAlias(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd",
		realMap(realFlag{name: "base-id"}, realFlag{name: "base", hidden: true}), conceptFixture(), CommandOverride{})
	if len(problems) != 0 {
		t.Fatalf("a hidden legacy alias flag must not be a co-occurrence: %v", problems)
	}
	if entry == nil {
		t.Fatal("expected a reduced entry")
	}
	if entry.Aliases["base-token"] != "base-id" {
		t.Fatalf("base-token = %q, want base-id", entry.Aliases["base-token"])
	}
	if _, ok := entry.Aliases["base"]; ok {
		t.Fatal("a real (hidden) flag must never be an alias key")
	}
}

func TestReduceLeafParamAliasesBindGenericFlag(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "id"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Bind: map[string]string{"id": "base_id"}})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry.Aliases["base"] != "id" || entry.Aliases["base-id"] != "id" || entry.Aliases["base-token"] != "id" {
		t.Fatalf("bind reduction wrong: %#v", entry.Aliases)
	}
}

func TestReduceLeafParamAliasesBindRejectsNonRealFlag(t *testing.T) {
	_, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "id"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Bind: map[string]string{"missing": "base_id"}})
	if len(problems) == 0 {
		t.Fatal("binding a non-real flag must fail")
	}
}

func TestReduceLeafParamAliasesScopedAlias(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "id"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", ScopedAliases: map[string]string{"ding-id": "id"}})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry == nil || entry.Aliases[cmdutil.Morph("ding-id")] != "id" {
		t.Fatalf("scoped alias not applied: %#v", entry)
	}
}

func TestReduceLeafParamAliasesScopedAliasRejectsNonRealTarget(t *testing.T) {
	_, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "id"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", ScopedAliases: map[string]string{"foo": "nonexistent"}})
	if len(problems) == 0 {
		t.Fatal("a scoped alias onto a non-real flag must fail")
	}
}

func TestReduceLeafParamAliasesBlockRemovesAndRecords(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "limit"}), conceptFixture(),
		CommandOverride{CommandPath: "demo cmd", Block: []string{"size"}})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry == nil {
		t.Fatal("expected a reduced entry")
	}
	if _, ok := entry.Aliases["size"]; ok {
		t.Fatal("a blocked emitted name must be removed from the alias map")
	}
	found := false
	for _, b := range entry.Blocked {
		if b == "size" {
			found = true
		}
	}
	if !found {
		t.Fatalf("size not recorded in blocked: %v", entry.Blocked)
	}
}

func TestReduceLeafParamAliasesPendingReviewDoesNotEmit(t *testing.T) {
	entry, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "query"}), nil,
		CommandOverride{CommandPath: "demo cmd", ScopedAliases: map[string]string{"keyword": "query"}, Confirm: true})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry == nil || entry.Aliases["keyword"] != "" || !containsParamAlias(entry.Blocked, "keyword") {
		t.Fatalf("pending mapping entered automatic aliases: %#v", entry)
	}
}

func TestReduceLeafParamAliasesExcludesProtectFuzzyButDoNotOverrideAnotherConcept(t *testing.T) {
	concepts := []Concept{
		{ID: "page_number", Members: []string{"page", "page-no"}, Excludes: []string{"page-size"}},
		{ID: "page_size", Members: []string{"limit", "page-size"}},
	}
	entry, problems := reduceLeafParamAliases("demo cmd", realMap(realFlag{name: "page"}, realFlag{name: "limit"}), concepts, CommandOverride{})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if entry.Aliases["page-size"] != "limit" || containsParamAlias(entry.Blocked, "page-size") {
		t.Fatalf("another reviewed concept alias was overridden by an exclude: %#v", entry)
	}
}

func TestReduceLeafParamAliasesRejectsProtectionOrScopedAliasOnRealFlag(t *testing.T) {
	real := realMap(realFlag{name: "user-id"}, realFlag{name: "user"})
	for name, override := range map[string]CommandOverride{
		"block":     {CommandPath: "demo cmd", Block: []string{"user-id"}},
		"ambiguous": {CommandPath: "demo cmd", Ambiguous: []string{"user-id"}},
		"scoped":    {CommandPath: "demo cmd", ScopedAliases: map[string]string{"user-id": "user"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, problems := reduceLeafParamAliases("demo cmd", real, nil, override); len(problems) == 0 {
				t.Fatal("real native flag was allowed to be reclassified")
			}
		})
	}
}

// TestGeneratedParamAliasesAreWellFormed guards the committed generated table
// at the Go level, complementing the byte-identity drift gate.
func TestGeneratedParamAliasesAreWellFormed(t *testing.T) {
	if len(generatedParamAliases) == 0 {
		t.Fatal("generated parameter alias table is empty")
	}
	seen := make(map[string]bool, len(generatedParamAliases))
	for _, e := range generatedParamAliases {
		if e.CLIPath == "" {
			t.Fatal("generated entry has an empty CLIPath")
		}
		if seen[e.CLIPath] {
			t.Fatalf("duplicate CLIPath %q in generated table", e.CLIPath)
		}
		seen[e.CLIPath] = true
		for emitted, canon := range e.Aliases {
			if emitted != cmdutil.Morph(emitted) {
				t.Fatalf("%s: alias key %q is not morph-normalized", e.CLIPath, emitted)
			}
			if canon == "" {
				t.Fatalf("%s: alias %q has an empty target", e.CLIPath, emitted)
			}
			if emitted == canon {
				t.Fatalf("%s: alias %q maps to itself", e.CLIPath, emitted)
			}
		}
		classified := make(map[string]string, len(e.Aliases)+len(e.Blocked)+len(e.Ambiguous))
		for emitted := range e.Aliases {
			classified[emitted] = "alias"
		}
		for kind, values := range map[string][]string{"blocked": e.Blocked, "ambiguous": e.Ambiguous} {
			for _, name := range values {
				if name != cmdutil.Morph(name) {
					t.Fatalf("%s: %s name %q is not morph-normalized", e.CLIPath, kind, name)
				}
				if previous := classified[name]; previous != "" {
					t.Fatalf("%s: %q is classified as both %s and %s", e.CLIPath, name, previous, kind)
				}
				classified[name] = kind
			}
		}
	}
}
