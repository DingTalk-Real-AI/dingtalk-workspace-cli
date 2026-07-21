// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package cli

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
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
	}
}
