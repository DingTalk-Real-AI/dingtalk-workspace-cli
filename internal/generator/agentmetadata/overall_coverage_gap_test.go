// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverallCoverageGapAgentmetadataMergeAndDisposition(t *testing.T) {
	target := []string{"a"}
	present := true
	rank := selectionRankSkill
	origin := "old-skill"
	if err := mergeRankedStringList(&target, &present, &rank, &origin, []string{"b"}, true, selectionRankMCPFallback, "lower", "sample", "use_when"); err != nil {
		t.Fatal(err)
	}
	if len(target) != 1 || target[0] != "a" {
		t.Fatalf("lower rank must not overwrite: %#v", target)
	}
	if err := mergeRankedStringList(&target, &present, &rank, &origin, []string{"b"}, true, selectionRankSkill, "new-skill", "sample", "use_when"); err != nil {
		t.Fatal(err)
	}
	if len(target) != 2 {
		t.Fatalf("skill same-rank merge = %#v", target)
	}
	rank = selectionRankExplicit
	target = []string{"same"}
	origin = "left"
	if err := mergeRankedStringList(&target, &present, &rank, &origin, []string{"same"}, true, selectionRankExplicit, "aaaa", "sample", "use_when"); err != nil {
		t.Fatal(err)
	}
	if origin != "aaaa" {
		t.Fatalf("equal non-skill merge should stabilize origin, got %q", origin)
	}
	if err := mergeRankedStringList(&target, &present, &rank, &origin, []string{"other"}, true, selectionRankExplicit, "conflict", "sample", "use_when"); err == nil {
		t.Fatal("equal-rank list conflict must fail")
	}

	for _, r := range []int{
		selectionRankContractFinal, selectionRankReviewedManual, selectionRankReviewedExplicit,
		selectionRankExplicit, selectionRankImported, selectionRankUnreviewedExplicit,
		selectionRankSkill, selectionRankMCPFallback, selectionRankDefault,
	} {
		if precedenceRank(selectionPrecedence(r)) != r {
			t.Fatalf("selection precedence round-trip lost rank %d", r)
		}
	}
	if got := selectionPrecedence(999); got != selectionPrecedenceDefault {
		t.Fatalf("unknown rank precedence = %q, want %q", got, selectionPrecedenceDefault)
	}
	for _, p := range []string{
		selectionPrecedenceContractFinal, selectionPrecedenceReviewedManual, selectionPrecedenceReviewedExplicit,
		selectionPrecedenceExplicit, selectionPrecedenceImported, selectionPrecedenceUnreviewedExplicit,
		selectionPrecedenceSkill, selectionPrecedenceMCPFallback,
	} {
		if selectionPrecedence(precedenceRank(p)) != p {
			t.Fatalf("selection precedence round-trip lost %q", p)
		}
	}
	if got := precedenceRank("unknown"); got != selectionRankDefault {
		t.Fatalf("unknown precedence rank = %d, want %d", got, selectionRankDefault)
	}

	if cloneInterfaceRef(nil) != nil {
		t.Fatal("nil interface ref clone must stay nil")
	}
	srcRef := &InterfaceRef{ProductID: "a", RPCName: "b"}
	clonedRef := cloneInterfaceRef(srcRef)
	if clonedRef == nil || *clonedRef != *srcRef {
		t.Fatalf("cloneInterfaceRef = %#v, want copy of %#v", clonedRef, srcRef)
	}
	clonedRef.ProductID = "mutated"
	if srcRef.ProductID != "a" {
		t.Fatal("cloneInterfaceRef must not alias the source")
	}
	if cloneStringList(nil) != nil {
		t.Fatal("nil string list clone must stay nil")
	}

	left := ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "a", RPCName: "b"},
		interfaceRefPresent: true, interfaceRefRank: selectionRankExplicit, interfaceRefOrigin: "left",
	}
	if err := mergeRankedInterfaceRef(&left, ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "a", RPCName: "b"},
		interfaceRefPresent: true, interfaceRefRank: selectionRankMCPFallback, interfaceRefOrigin: "lower",
	}, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := mergeRankedInterfaceRef(&left, ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "a", RPCName: "b"},
		interfaceRefPresent: true, interfaceRefRank: selectionRankExplicit, interfaceRefOrigin: "right",
	}, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := mergeRankedInterfaceRef(&left, ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "a", RPCName: "other"},
		interfaceRefPresent: true, interfaceRefRank: selectionRankExplicit, interfaceRefOrigin: "conflict",
	}, "sample"); err == nil {
		t.Fatal("interface_ref conflict must fail")
	}

	product := ProductMetadata{}
	recordProductListCandidate(nil, "use_when", []string{"x"}, true, selectionRankSkill, "src")
	recordProductListCandidate(&product, "use_when", []string{"x"}, true, selectionRankSkill, "src")
	recordProductListCandidate(&product, "use_when", []string{"x"}, true, selectionRankSkill, "src")
	recordProductStringCandidate(&product, "summary", "s", true, selectionRankSkill, "src")
	recordProductStringCandidate(&product, "summary", "s", true, selectionRankSkill, "src")
	if got := product.fieldCandidates["use_when"]; len(got) != 1 || got[0].Precedence != selectionPrecedenceSkill || got[0].Source != "src" {
		t.Fatalf("use_when candidates after dedup = %#v", got)
	}
	if got := product.fieldCandidates["summary"]; len(got) != 1 || got[0].Value != "s" {
		t.Fatalf("summary candidates after dedup = %#v", got)
	}

	mergeFieldCandidateHistory(nil, ToolMetadata{})
	targetTool := ToolMetadata{}
	incoming := ToolMetadata{fieldCandidates: map[string][]FieldCandidateProvenance{
		"use_when": {{Value: []string{"a"}, Source: "a", Precedence: selectionPrecedenceSkill}},
	}}
	mergeFieldCandidateHistory(&targetTool, incoming)
	mergeFieldCandidateHistory(&targetTool, incoming)
	if got := targetTool.fieldCandidates["use_when"]; len(got) != 1 {
		t.Fatalf("mergeFieldCandidateHistory must merge once and dedup, got %#v", got)
	}

	if err := validateToolFieldCandidateConflicts(File{
		Products: map[string]ProductMetadata{"p": {fieldCandidates: map[string][]FieldCandidateProvenance{
			"use_when": {
				{Value: []string{"a"}, Source: "a", Precedence: selectionPrecedenceSkill},
				{Value: []string{"b"}, Source: "b", Precedence: selectionPrecedenceSkill},
			},
		}}},
	}); err == nil {
		t.Fatal("product field candidate conflict must fail")
	}
	if err := validateToolFieldCandidateConflicts(File{
		Tools: map[string]ToolMetadata{"sample": {fieldCandidates: map[string][]FieldCandidateProvenance{
			"summary": {
				{Value: "a", Source: "z", Precedence: selectionPrecedenceSkill},
				{Value: "b", Source: "a", Precedence: selectionPrecedenceSkill},
			},
		}}},
	}); err == nil {
		t.Fatal("tool field candidate conflict must fail")
	}

	invalid := File{Tools: map[string]ToolMetadata{
		"unavail-ref": {
			InterfaceMode: "mcp", Availability: "unavailable",
			InterfaceRef: &InterfaceRef{ProductID: "a", RPCName: "b"}, InterfaceReason: "offline",
		},
		"unavail-reason": {InterfaceMode: "mcp", Availability: "unavailable"},
		"local-ref": {
			InterfaceMode: "local", Availability: "available",
			InterfaceRef: &InterfaceRef{ProductID: "a", RPCName: "b"},
		},
		"composite-ref": {
			InterfaceMode: "composite", Availability: "available",
			InterfaceRef: &InterfaceRef{ProductID: "a", RPCName: "b"}, InterfaceReason: "multi",
		},
		"composite-reason": {InterfaceMode: "composite", Availability: "available"},
	}}
	if err := validateInterfaceDispositions(invalid); err == nil {
		t.Fatal("interface disposition matrix must fail")
	}
	if got := uniqueStringsInOrder([]string{"", " ", "a", "a"}); len(got) != 1 || got[0] != "a" {
		t.Fatalf("uniqueStringsInOrder = %#v", got)
	}
	if got := uniqueStringsInOrder([]string{"", " "}); got != nil {
		t.Fatalf("blank uniqueStringsInOrder = %#v", got)
	}
	if got := normalizeAuthoredStrings([]string{"", " "}); got == nil || len(got) != 0 {
		t.Fatalf("normalizeAuthoredStrings empty = %#v", got)
	}

	stats := &Stats{referenceReviews: map[string]ReferenceReview{
		"alias path":         {Status: "alias", Target: "live.path"},
		"reviewed unmatched": {Status: "keep", Reason: "reviewed"},
	}}
	file := &File{Tools: map[string]ToolMetadata{
		"alias path":         {UseWhen: []string{"from-alias"}, useWhenPresent: true, useWhenRank: selectionRankSkill, useWhenOrigin: "alias"},
		"reviewed unmatched": {UseWhen: []string{"keep"}, useWhenPresent: true},
		"plain unmatched":    {},
		"live path":          {UseWhen: []string{"live"}, useWhenPresent: true, useWhenRank: selectionRankSkill, useWhenOrigin: "live"},
	}}
	origins := sourceTracker{}
	origins.add("alias path", "skill.md", 1)
	origins.add("reviewed unmatched", "skill.md", 2)
	if err := reconcileSurface(file, Options{ToolPaths: map[string]string{
		"live path": "live path",
		"live.path": "live path",
	}}, stats, origins); err != nil {
		t.Fatal(err)
	}
	if _, ok := file.Tools["live path"]; !ok {
		t.Fatalf("reconciled tools = %#v", file.Tools)
	}

	if hasSurfaceAgentSummary(File{Tools: map[string]ToolMetadata{
		"old": {AgentSummary: "s", agentSummaryPresent: true},
	}}, map[string]string{"old": "live", "canonical": "live"}, "canonical") != true {
		t.Fatal("remapped surface summary must be detected")
	}
	if hasSurfaceAgentSummary(File{Tools: map[string]ToolMetadata{
		"other": {AgentSummary: "s", agentSummaryPresent: true},
	}}, nil, "canonical") {
		t.Fatal("unrelated summary must not match")
	}

	if _, _, err := Generate(Options{}); err == nil {
		t.Fatal("Generate without registry projection must fail")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "products"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skill.md"), []byte("# skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSources(Options{Root: root, SkillPath: "missing.md", IntentGuidePath: "skill.md", ProductsDir: "products"}); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("missing source error = %v", err)
	}

	conflictLeft := ToolMetadata{
		UseWhen: []string{"a"}, useWhenPresent: true, useWhenRank: selectionRankExplicit, useWhenOrigin: "left",
	}
	if _, err := mergeToolMetadata(conflictLeft, ToolMetadata{
		UseWhen: []string{"b"}, useWhenPresent: true, useWhenRank: selectionRankExplicit, useWhenOrigin: "right",
	}, "sample"); err == nil {
		t.Fatal("mergeToolMetadata list conflict must fail")
	}
	trueVal := true
	falseVal := false
	merged, err := mergeToolMetadata(ToolMetadata{
		Reviewed: &trueVal, reviewedRank: selectionRankExplicit, reviewedOrigin: "left",
		InterfaceMode: "mcp", interfaceModePresent: true, interfaceModeRank: selectionRankSkill, interfaceModeOrigin: "left",
	}, ToolMetadata{
		Reviewed: &falseVal, reviewedRank: selectionRankSkill, reviewedOrigin: "right",
		InterfaceMode: "local", interfaceModePresent: true, interfaceModeRank: selectionRankExplicit, interfaceModeOrigin: "right",
		InterfaceRef: &InterfaceRef{ProductID: "a", RPCName: "b"}, interfaceRefPresent: true, interfaceRefRank: selectionRankExplicit, interfaceRefOrigin: "right",
	}, "sample")
	if err != nil {
		t.Fatal(err)
	}
	if merged.InterfaceMode != "local" || merged.Reviewed == nil || !*merged.Reviewed {
		t.Fatalf("merged metadata = %#v", merged)
	}

	ifaceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ifaceRoot, "products"), 0o700); err != nil {
		t.Fatal(err)
	}
	ifacePath := filepath.Join(ifaceRoot, "interface.json")
	ifaceBody := `{
  "version": 1,
  "source": "mcp",
  "source_hash": "h",
  "tools": {
    "sample get": {
      "description": "Read a sample item for agents.",
      "interface_ref": {"product_id": "sample", "rpc_name": "get"}
    }
  }
}`
	if err := os.WriteFile(ifacePath, []byte(ifaceBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ifaceRoot, "skill.md"), []byte("# skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := loadSources(Options{
		Root: ifaceRoot, SkillPath: "skill.md", IntentGuidePath: "skill.md", ProductsDir: "products",
		InterfaceMetadataPath: "interface.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	byDisplay := map[string]sourceFile{}
	for _, file := range files {
		byDisplay[file.display] = file
	}
	out := &File{Tools: map[string]ToolMetadata{
		"sample get": {AgentSummary: "existing", agentSummaryPresent: true},
	}}
	ifaceStats := &Stats{}
	if err := applyInterfaceMetadataFallback(out, byDisplay, Options{
		Root: ifaceRoot, InterfaceMetadataPath: "interface.json",
		ToolPaths: map[string]string{"sample get": "sample get"},
	}, ifaceStats, sourceTracker{}); err != nil {
		t.Fatal(err)
	}
	if ifaceStats.InterfaceMetadata == nil || ifaceStats.InterfaceMetadata.PreservedSummaries != 1 {
		t.Fatalf("preserved interface summaries = %#v", ifaceStats.InterfaceMetadata)
	}

	if _, err := mergeToolMetadata(ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "a", RPCName: "b"},
		interfaceRefPresent: true, interfaceRefRank: selectionRankExplicit, interfaceRefOrigin: "left",
	}, ToolMetadata{
		InterfaceRef:        &InterfaceRef{ProductID: "a", RPCName: "c"},
		interfaceRefPresent: true, interfaceRefRank: selectionRankExplicit, interfaceRefOrigin: "right",
	}, "sample"); err == nil {
		t.Fatal("mergeToolMetadata interface_ref conflict must fail")
	}
	if _, err := mergeToolMetadata(ToolMetadata{
		InterfaceMode: "mcp", interfaceModePresent: true, interfaceModeRank: selectionRankExplicit, interfaceModeOrigin: "left",
	}, ToolMetadata{
		InterfaceMode: "local", interfaceModePresent: true, interfaceModeRank: selectionRankExplicit, interfaceModeOrigin: "right",
	}, "sample"); err == nil {
		t.Fatal("mergeToolMetadata interface_mode conflict must fail")
	}
	recordProductStringCandidate(&ProductMetadata{}, "summary", "x", false, selectionRankSkill, "src")
}
