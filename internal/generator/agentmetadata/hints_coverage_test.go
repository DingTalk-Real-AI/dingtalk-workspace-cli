// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func parseCoverageHints(root string, bodies map[string]string) (bool, error) {
	files := make([]sourceFile, 0, len(bodies))
	for relative, body := range bodies {
		files = append(files, sourceFile{
			path:    filepath.Join(root, "hints", filepath.FromSlash(relative)),
			display: filepath.ToSlash(filepath.Join("hints", relative)),
			data:    []byte(body),
		})
	}
	out := &File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}
	stats := &Stats{referenceReviews: map[string]ReferenceReview{}}
	return parseHintSources(out, files, Options{Root: root, HintsDir: "hints"}, stats, sourceTracker{})
}

func validCoverageIndex(metadataPath, selectionPath string) string {
	return `{"version":1,"format":"dws-agent-hint-index","source":{"kind":"explicit"},"metadata":{"sample":"` + metadataPath + `"},"selection":{"sample":"` + selectionPath + `"}}`
}

func TestCrossPlatformCoverageHintUnmarshalAndIndexValidationEdges(t *testing.T) {
	if err := json.Unmarshal([]byte(`{`), &HintProduct{}); err == nil {
		t.Fatal("invalid HintProduct succeeded")
	}
	if err := json.Unmarshal([]byte(`{`), &HintTool{}); err == nil {
		t.Fatal("invalid HintTool succeeded")
	}
	if used, err := parseHintSources(&File{}, nil, Options{}, &Stats{}, sourceTracker{}); err != nil || used {
		t.Fatalf("empty hints = %v, %v", used, err)
	}

	root := t.TempDir()
	validHint := `{"version":1,"source":{"kind":"explicit"}}`
	tests := []struct {
		name  string
		index string
		files map[string]string
		want  string
	}{
		{"invalid json", `{`, nil, "decode Agent hint index"},
		{"version", `{"version":2}`, nil, "unsupported version"},
		{"format", `{"version":1,"format":"other"}`, nil, "unsupported format"},
		{"source kind", `{"version":1,"format":"dws-agent-hint-index","source":{"kind":"imported"}}`, nil, "unsupported source kind"},
		{"metadata required", `{"version":1,"format":"dws-agent-hint-index","selection":{"sample":"selection.json"}}`, nil, "metadata map is required"},
		{"selection required", `{"version":1,"format":"dws-agent-hint-index","metadata":{"sample":"metadata.json"}}`, nil, "selection map is required"},
		{"empty metadata path", validCoverageIndex("", "selection.json"), map[string]string{"selection.json": validHint}, "missing path"},
		{"escaping metadata path", validCoverageIndex("../outside.json", "selection.json"), map[string]string{"selection.json": validHint}, "escapes hints root"},
		{"missing metadata file", validCoverageIndex("metadata.json", "selection.json"), map[string]string{"selection.json": validHint}, "file missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodies := map[string]string{"index.json": tc.index}
			for path, body := range tc.files {
				bodies[path] = body
			}
			_, err := parseCoverageHints(root, bodies)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	indexWithProjection := `{"version":1,"format":"dws-agent-hint-index","source":{"kind":"explicit"},"coverage":{"source_tools":1},"metadata":{"sample":"metadata.json"},"selection":{"sample":"selection.json"},"reference_review":{"old path":{"status":"stale","reason":"removed"}}}`
	if used, err := parseCoverageHints(root, map[string]string{"index.json": indexWithProjection, "metadata.json": validHint, "selection.json": validHint}); err != nil || !used {
		t.Fatalf("index projection = %v, %v", used, err)
	}
}

func TestCrossPlatformCoverageHintFileAndRoleValidationEdges(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"invalid json", `{`, "decode Agent hint"},
		{"version", `{"version":2}`, "unsupported version"},
		{"source kind", `{"version":1,"source":{"kind":"other"}}`, "unsupported source kind"},
		{"legacy unavailable mode", `{"version":1,"source":{},"tools":{"sample.tool":{"interface_mode":"unavailable"}}}`, "legacy interface_mode"},
		{"invalid interface mode", `{"version":1,"source":{},"tools":{"sample.tool":{"interface_mode":"remote"}}}`, "unsupported interface_mode"},
		{"invalid availability", `{"version":1,"source":{},"tools":{"sample.tool":{"availability":"sometimes"}}}`, "unsupported availability"},
		{"incomplete interface ref", `{"version":1,"source":{},"tools":{"sample.tool":{"interface_ref":{"product_id":"sample"}}}}`, "incomplete interface_ref"},
		{"alias target", `{"version":1,"source":{},"reference_review":{"old":{"status":"alias","reason":"renamed"}}}`, "alias is missing target"},
		{"group target", `{"version":1,"source":{},"reference_review":{"old":{"status":"group","target":"new","reason":"group"}}}`, "cannot have target"},
		{"reference status", `{"version":1,"source":{},"reference_review":{"old":{"status":"other","reason":"bad"}}}`, "unsupported status"},
		{"reference reason", `{"version":1,"source":{},"reference_review":{"old":{"status":"stale"}}}`, "missing reason"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCoverageHints(root, map[string]string{"legacy.json": tc.body})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	trueValue := true
	roleTests := []struct {
		name string
		role hintFileRole
		hint HintFile
		want string
	}{
		{"metadata selection fields", hintRoleMetadata, HintFile{Tools: map[string]HintTool{"sample.tool": {UseWhen: []string{"x"}}}}, "selection fields"},
		{"metadata bad gate", hintRoleMetadata, HintFile{Tools: map[string]HintTool{"sample.tool": {RuntimeGate: "other"}}}, "unsupported runtime_gate"},
		{"metadata gate confirmation", hintRoleMetadata, HintFile{Tools: map[string]HintTool{"sample.tool": {RuntimeGate: "typed_yes", Confirmation: "not_required"}}}, "requires confirmation"},
		{"metadata confirmation gate", hintRoleMetadata, HintFile{Tools: map[string]HintTool{"sample.tool": {RuntimeGate: "none", Confirmation: "user_required"}}}, "requires runtime_gate"},
		{"metadata products", hintRoleMetadata, HintFile{Products: map[string]HintProduct{"sample": {}}}, "must not carry products"},
		{"selection metadata fields", hintRoleSelection, HintFile{Tools: map[string]HintTool{"sample.tool": {Reviewed: &trueValue, Effect: "read", effectPresent: true}}}, "metadata fields"},
	}
	for _, tc := range roleTests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHintFileRole("fixture", tc.hint, tc.role)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageHintHelperRemainingBranches(t *testing.T) {
	falseValue := false
	if got := hintSelectionRank(HintSource{}, &falseValue); got != selectionRankUnreviewedExplicit {
		t.Fatalf("unreviewed rank = %d", got)
	}
	if got := hintSelectionRank(HintSource{Reviewed: true}, nil); got != selectionRankReviewedExplicit {
		t.Fatalf("reviewed source rank = %d", got)
	}
	if reason := hintCandidateReviewReason(HintSource{Kind: "explicit", Name: "fixture"}, HintTool{}); !strings.Contains(reason, "explicit") {
		t.Fatalf("explicit reason = %q", reason)
	}
	if got := normalizeHintToolPath(" dws sample.tool "); got != "sample.tool" {
		t.Fatalf("canonical hint path = %q", got)
	}
	if got := normalizeHintToolPath("  "); got != "" {
		t.Fatalf("blank hint path = %q", got)
	}
	if got := hintSourceLabel(HintSource{Revision: "1234567890abcdef"}, "fallback"); got != "fallback@1234567890ab" {
		t.Fatalf("fallback label = %q", got)
	}
}

func TestCrossPlatformCoverageHintParsingAndApplicationConflictEdges(t *testing.T) {
	var product HintProduct
	if err := product.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("direct invalid HintProduct succeeded")
	}
	var tool HintTool
	if err := tool.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("direct invalid HintTool succeeded")
	}

	root := t.TempDir()
	badSelection := validCoverageIndex("metadata.json", "selection.json")
	if _, err := parseCoverageHints(root, map[string]string{
		"index.json":     badSelection,
		"metadata.json":  `{"version":1,"source":{"kind":"explicit"}}`,
		"selection.json": `{"version":1,"source":{"kind":"explicit"},"tools":{"sample.tool":{"effect":"read"}}}`,
	}); err == nil || !strings.Contains(err.Error(), "metadata fields") {
		t.Fatalf("selection role error = %v", err)
	}
	if _, err := parseCoverageHints(root, map[string]string{
		"index.json":    badSelection,
		"metadata.json": `{"version":1,"source":{"kind":"explicit"}}`,
	}); err == nil || !strings.Contains(err.Error(), "selection") {
		t.Fatalf("missing selection error = %v", err)
	}

	index := validCoverageIndex("metadata.json", "selection.json")
	if used, err := parseCoverageHints(root, map[string]string{
		"index.json":      index,
		"metadata.json":   `{"version":1,"source":{"kind":"explicit"}}`,
		"selection.json":  `{"version":1,"source":{"kind":"explicit"}}`,
		"imported/z.json": `{"version":1,"source":{"kind":"imported"}}`,
		"imported/a.json": `{"version":1,"source":{"kind":"imported"}}`,
	}); err != nil || !used {
		t.Fatalf("sorted imported hints = %v, %v", used, err)
	}

	trueValue := true
	apply := func(out File, incoming HintFile) error {
		stats := Stats{referenceReviews: map[string]ReferenceReview{}}
		return applyHintSource(&out, parsedHintSource{
			file: sourceFile{display: "hints/selection/sample.json"},
			hint: incoming,
		}, &stats, sourceTracker{})
	}
	reviewedSource := HintSource{Kind: "explicit", Reviewed: true}
	for _, tc := range []struct {
		name     string
		existing ProductMetadata
		incoming HintProduct
	}{
		{"summary", ProductMetadata{AgentSummary: "a", agentSummaryPresent: true, agentSummaryRank: selectionRankReviewedExplicit, agentSummaryOrigin: "existing"}, HintProduct{AgentSummary: "b", agentSummaryPresent: true}},
		{"use", ProductMetadata{UseWhen: []string{"a"}, useWhenPresent: true, useWhenRank: selectionRankReviewedExplicit, useWhenOrigin: "existing"}, HintProduct{UseWhen: []string{"b"}}},
		{"avoid", ProductMetadata{AvoidWhen: []string{"a"}, avoidWhenPresent: true, avoidWhenRank: selectionRankReviewedExplicit, avoidWhenOrigin: "existing"}, HintProduct{AvoidWhen: []string{"b"}}},
	} {
		t.Run("product "+tc.name, func(t *testing.T) {
			err := apply(File{Products: map[string]ProductMetadata{"sample": tc.existing}, Tools: map[string]ToolMetadata{}}, HintFile{
				Source: reviewedSource, Products: map[string]HintProduct{"sample": tc.incoming},
			})
			if err == nil {
				t.Fatal("same-rank product conflict succeeded")
			}
		})
	}

	listExisting := func(field string) ToolMetadata {
		metadata := ToolMetadata{}
		switch field {
		case "use_when":
			metadata.UseWhen, metadata.useWhenPresent, metadata.useWhenRank, metadata.useWhenOrigin = []string{"a"}, true, selectionRankReviewedExplicit, "existing"
		case "avoid_when":
			metadata.AvoidWhen, metadata.avoidWhenPresent, metadata.avoidWhenRank, metadata.avoidWhenOrigin = []string{"a"}, true, selectionRankReviewedExplicit, "existing"
		case "prerequisites":
			metadata.Prerequisites, metadata.prerequisitesPresent, metadata.prerequisitesRank, metadata.prerequisitesOrigin = []string{"a"}, true, selectionRankReviewedExplicit, "existing"
		case "tips":
			metadata.Tips, metadata.tipsPresent, metadata.tipsRank, metadata.tipsOrigin = []string{"a"}, true, selectionRankReviewedExplicit, "existing"
		case "workflow_refs":
			metadata.WorkflowRefs, metadata.workflowRefsPresent, metadata.workflowRefsRank, metadata.workflowRefsOrigin = []string{"a"}, true, selectionRankReviewedExplicit, "existing"
		case "examples":
			metadata.Examples, metadata.examplesPresent, metadata.examplesRank, metadata.examplesOrigin = []string{"a"}, true, selectionRankReviewedExplicit, "existing"
		}
		return metadata
	}
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		t.Run("tool "+field, func(t *testing.T) {
			incoming := HintTool{Reviewed: &trueValue}
			switch field {
			case "use_when":
				incoming.UseWhen = []string{"b"}
			case "avoid_when":
				incoming.AvoidWhen = []string{"b"}
			case "prerequisites":
				incoming.Prerequisites = []string{"b"}
			case "tips":
				incoming.Tips = []string{"b"}
			case "workflow_refs":
				incoming.WorkflowRefs = []string{"b"}
			case "examples":
				incoming.Examples = []string{"b"}
			}
			err := apply(File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{"sample get": listExisting(field)}}, HintFile{
				Source: reviewedSource, Tools: map[string]HintTool{"sample get": incoming},
			})
			if err == nil {
				t.Fatal("same-rank list conflict succeeded")
			}
		})
	}

	for _, tc := range []struct {
		name     string
		existing ToolMetadata
		incoming HintTool
	}{
		{"effect", ToolMetadata{Effect: "read", effectPresent: true, effectRank: selectionRankReviewedExplicit, effectOrigin: "existing"}, HintTool{Effect: "write", effectPresent: true, Reviewed: &trueValue}},
		{"risk", ToolMetadata{Risk: "low", riskPresent: true, riskRank: selectionRankReviewedExplicit, riskOrigin: "existing"}, HintTool{Risk: "high", riskPresent: true, Reviewed: &trueValue}},
		{"confirmation", ToolMetadata{Confirmation: "not_required", confirmationPresent: true, confirmationRank: selectionRankReviewedExplicit, confirmationOrigin: "existing"}, HintTool{Confirmation: "user_required", confirmationPresent: true, Reviewed: &trueValue}},
		{"idempotency", ToolMetadata{Idempotency: "idempotent", idempotencyPresent: true, idempotencyRank: selectionRankReviewedExplicit, idempotencyOrigin: "existing"}, HintTool{Idempotency: "unknown", idempotencyPresent: true, Reviewed: &trueValue}},
		{"interface ref", ToolMetadata{InterfaceRef: &InterfaceRef{ProductID: "sample", RPCName: "one"}, interfaceRefPresent: true, interfaceRefRank: selectionRankReviewedExplicit, interfaceRefOrigin: "existing"}, HintTool{InterfaceRef: &InterfaceRef{ProductID: "sample", RPCName: "two"}, interfaceRefPresent: true, Reviewed: &trueValue}},
		{"interface mode", ToolMetadata{InterfaceMode: "mcp", interfaceModePresent: true, interfaceModeRank: selectionRankReviewedExplicit, interfaceModeOrigin: "existing"}, HintTool{InterfaceMode: "local", interfaceModePresent: true, Reviewed: &trueValue}},
	} {
		t.Run("tool "+tc.name, func(t *testing.T) {
			err := apply(File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{"sample get": tc.existing}}, HintFile{
				Source: reviewedSource, Tools: map[string]HintTool{"sample get": tc.incoming},
			})
			if err == nil {
				t.Fatal("same-rank scalar conflict succeeded")
			}
		})
	}

	base := File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}
	stats := Stats{referenceReviews: map[string]ReferenceReview{}}
	if err := applyHintSource(&base, parsedHintSource{file: sourceFile{display: "fixture"}, hint: HintFile{
		Source:          reviewedSource,
		ReferenceReview: map[string]ReferenceReview{" ": {Status: "stale"}},
		Products:        map[string]HintProduct{" ": {}},
		Tools:           map[string]HintTool{" ": {AgentSummary: "ignored"}, "empty": {}, "sample get": {Risk: "high", riskPresent: true, Reviewed: &trueValue}},
	}}, &stats, sourceTracker{}); err != nil {
		t.Fatal(err)
	}
	if stats.RiskRules != 1 {
		t.Fatalf("risk rules = %d", stats.RiskRules)
	}
}
