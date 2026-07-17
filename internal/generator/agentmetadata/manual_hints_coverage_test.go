// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageReviewedSelectionRetentionAndDeliveryFailures(t *testing.T) {
	retainReviewedSelectionCandidates(nil)
	file := File{
		Products: map[string]ProductMetadata{"sample": {}},
		Tools:    map[string]ToolMetadata{"sample tool": {}},
	}
	retainReviewedSelectionCandidates(&file)
	if file.Products["sample"].fieldCandidates == nil || file.Tools["sample tool"].fieldCandidates == nil {
		t.Fatal("candidate maps were not initialized")
	}

	opts := Options{ProductIDs: map[string]bool{"sample": true}, CanonicalToolPaths: map[string]string{"sample.tool": "sample tool"}}
	if err := validateReviewedSelectionDelivery(File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}, opts); err == nil || !strings.Contains(err.Error(), "missing product") || !strings.Contains(err.Error(), "missing tool") {
		t.Fatalf("missing delivery error = %v", err)
	}

	selected := []FieldCandidateProvenance{{Precedence: selectionPrecedenceReviewedExplicit, Selected: true}}
	wrongProvenance := FieldProvenance{Precedence: selectionPrecedenceReviewedExplicit, Source: "metadata/sample.json", Candidates: selected}
	falseValue := false
	trueValue := true
	file = File{
		Products: map[string]ProductMetadata{"sample": {
			FieldProvenance: map[string]FieldProvenance{
				"agent_summary": wrongProvenance,
				"use_when":      wrongProvenance,
				"avoid_when":    wrongProvenance,
			},
		}},
		Tools: map[string]ToolMetadata{"sample tool": {
			Reviewed: &falseValue,
			FieldProvenance: map[string]FieldProvenance{
				"agent_summary": wrongProvenance,
				"use_when":      wrongProvenance,
				"avoid_when":    wrongProvenance,
				"examples":      wrongProvenance,
			},
		}},
	}
	err := validateReviewedSelectionDelivery(file, opts)
	if err == nil || !strings.Contains(err.Error(), "source is not selection") || !strings.Contains(err.Error(), "is not reviewed_explicit") {
		t.Fatalf("invalid delivery error = %v", err)
	}

	file = File{
		Products: map[string]ProductMetadata{"sample": {
			AgentSummary: "summary", agentSummaryRank: selectionRankReviewedExplicit,
			UseWhen: []string{"use"}, useWhenRank: selectionRankReviewedExplicit,
			AvoidWhen: []string{"avoid"}, avoidWhenRank: selectionRankReviewedExplicit,
			FieldProvenance: map[string]FieldProvenance{},
		}},
		Tools: map[string]ToolMetadata{"sample tool": {
			AgentSummary: "summary", agentSummaryRank: selectionRankReviewedExplicit,
			UseWhen: []string{"use"}, useWhenRank: selectionRankReviewedExplicit,
			AvoidWhen: []string{"avoid"}, avoidWhenRank: selectionRankReviewedExplicit,
			Examples: []string{"dws sample tool"}, examplesRank: selectionRankReviewedExplicit,
			Reviewed: &trueValue, reviewedRank: selectionRankReviewedExplicit,
			FieldProvenance: map[string]FieldProvenance{},
		}},
	}
	if err := validateReviewedSelectionDelivery(file, opts); err == nil || !strings.Contains(err.Error(), "provenance is not one") {
		t.Fatalf("missing provenance error = %v", err)
	}
}

func TestCrossPlatformCoverageExpectedCanonicalToolHelpersRemainingEdges(t *testing.T) {
	paths := expectedCanonicalToolPaths(Options{CanonicalToolPaths: map[string]string{
		" sample.tool ": " sample item get ",
		"":              "blank",
		"blank.tool":    " ",
	}})
	if len(paths) != 1 || paths["sample.tool"] != "sample item get" {
		t.Fatalf("canonical paths = %#v", paths)
	}
	paths = expectedCanonicalToolPaths(Options{ToolPaths: map[string]string{
		"sample.tool":     "sample item get",
		"sample item get": "sample item get",
	}})
	if len(paths) != 1 || paths["sample.tool"] != "sample item get" {
		t.Fatalf("fallback canonical paths = %#v", paths)
	}

	set := expectedCanonicalToolSet(Options{CanonicalToolPaths: map[string]string{" sample.tool ": "path", " ": "blank"}})
	if len(set) != 1 || !set["sample.tool"] {
		t.Fatalf("canonical set = %#v", set)
	}
	if set := expectedCanonicalToolSet(Options{}); set != nil {
		t.Fatalf("empty canonical set = %#v", set)
	}
	set = expectedCanonicalToolSet(Options{ToolPaths: map[string]string{"sample.tool": "path", "sample path": "path"}})
	if len(set) != 1 || !set["sample.tool"] {
		t.Fatalf("fallback canonical set = %#v", set)
	}
}

func TestCrossPlatformCoverageSelectionAuthoringLoadFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hints"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "hints", "selection"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hints", "selection", "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateSelectionAuthoringContracts(Options{Root: root, HintsDir: "hints"})
	if err == nil || !strings.Contains(err.Error(), "load selection Agent hints") {
		t.Fatalf("selection load error = %v", err)
	}
}

func TestCrossPlatformCoverageSelectionAuthoringSelectionContractFailure(t *testing.T) {
	root := t.TempDir()
	writeSelectionFixture(t, root, true, `["dws sample item search --query value"]`)
	path := filepath.Join(root, "internal/cli/schema_hints/selection/sample.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), "A new item must be created", "An existing item must be found"))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	err = validateSelectionAuthoringContracts(selectionFixtureOptions(root))
	if err == nil || !strings.Contains(err.Error(), "same literal positive and negative") {
		t.Fatalf("selection contract error = %v", err)
	}
}
