// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSelectionListsUseWholeFieldPrecedenceOrderIndependently(t *testing.T) {
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		t.Run(field, func(t *testing.T) {
			low := toolMetadataWithList(field, []string{"low second", "low first"}, selectionRankImported, "imported.json")
			high := toolMetadataWithList(field, []string{"high second", "high first"}, selectionRankReviewedExplicit, "reviewed.json")

			forward, err := mergeToolMetadata(low, high, "calendar event get")
			if err != nil {
				t.Fatalf("mergeToolMetadata(low, high) error = %v", err)
			}
			reverse, err := mergeToolMetadata(high, low, "calendar event get")
			if err != nil {
				t.Fatalf("mergeToolMetadata(high, low) error = %v", err)
			}
			want := []string{"high second", "high first"}
			if got := toolMetadataList(forward, field); !reflect.DeepEqual(got, want) {
				t.Fatalf("forward %s = %#v, want authored winner %#v", field, got, want)
			}
			if got := toolMetadataList(reverse, field); !reflect.DeepEqual(got, want) {
				t.Fatalf("reverse %s = %#v, want authored winner %#v", field, got, want)
			}

			provenance, ok := forward.FieldProvenance[field]
			if !ok || provenance.Source != "reviewed.json" || provenance.Precedence != "reviewed_explicit" || provenance.Resolution != "highest_precedence" {
				t.Fatalf("%s provenance = %#v", field, provenance)
			}
			if got, ok := provenance.Value.([]string); !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s provenance winner = %#v", field, provenance.Value)
			}
			if !hasListCandidate(provenance.Candidates, want, "reviewed.json", true) ||
				!hasListCandidate(provenance.Candidates, []string{"low second", "low first"}, "imported.json", false) {
				t.Fatalf("%s provenance candidates = %#v", field, provenance.Candidates)
			}
		})
	}
}

func TestSelectionListsRejectSameRankContentOrOrderConflict(t *testing.T) {
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		for _, test := range []struct {
			name  string
			left  []string
			right []string
		}{
			{name: "content", left: []string{"first"}, right: []string{"second"}},
			{name: "order", left: []string{"first", "second"}, right: []string{"second", "first"}},
		} {
			t.Run(field+"/"+test.name, func(t *testing.T) {
				left := toolMetadataWithList(field, test.left, selectionRankExplicit, "a.json")
				right := toolMetadataWithList(field, test.right, selectionRankExplicit, "b.json")
				for _, pair := range [][2]ToolMetadata{{left, right}, {right, left}} {
					_, err := mergeToolMetadata(pair[0], pair[1], "calendar event get")
					if err == nil || !strings.Contains(err.Error(), "field "+field) || !strings.Contains(err.Error(), "a.json") || !strings.Contains(err.Error(), "b.json") {
						t.Fatalf("merge error = %v, want order-independent same-rank %s conflict", err, field)
					}
				}
			})
		}
	}
}

func TestSelectionListsAllowIdenticalSameRankArraysAndAuditRefsStillUnion(t *testing.T) {
	left := toolMetadataWithList("use_when", []string{"second", "first"}, selectionRankExplicit, "b.json")
	left.SourceRefs = []string{"b.json"}
	right := toolMetadataWithList("use_when", []string{"second", "first"}, selectionRankExplicit, "a.json")
	right.SourceRefs = []string{"a.json", "b.json"}

	merged, err := mergeToolMetadata(left, right, "calendar event get")
	if err != nil {
		t.Fatalf("mergeToolMetadata() error = %v", err)
	}
	if got, want := merged.UseWhen, []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("use_when = %#v, want authored order %#v", got, want)
	}
	if got, want := merged.SourceRefs, []string{"a.json", "b.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source_refs = %#v, want audit union %#v", got, want)
	}
}

func TestSelectionListsTreatExplicitEmptyAsAWinningValue(t *testing.T) {
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		t.Run(field, func(t *testing.T) {
			low := toolMetadataWithList(field, []string{"lower value"}, selectionRankImported, "imported.json")
			high := toolMetadataWithList(field, []string{}, selectionRankReviewedExplicit, "reviewed.json")

			for direction, pair := range map[string][2]ToolMetadata{
				"forward": {low, high},
				"reverse": {high, low},
			} {
				merged, err := mergeToolMetadata(pair[0], pair[1], "calendar event get")
				if err != nil {
					t.Fatalf("%s merge error = %v", direction, err)
				}
				got := toolMetadataList(merged, field)
				if got == nil || len(got) != 0 {
					t.Fatalf("%s %s = %#v, want selected explicit []", direction, field, got)
				}
				provenance := merged.FieldProvenance[field]
				winner, ok := provenance.Value.([]string)
				if !ok || winner == nil || len(winner) != 0 || provenance.Source != "reviewed.json" {
					t.Fatalf("%s %s provenance = %#v, want reviewed [] winner", direction, field, provenance)
				}
				if !hasListCandidate(provenance.Candidates, []string{}, "reviewed.json", true) ||
					!hasListCandidate(provenance.Candidates, []string{"lower value"}, "imported.json", false) {
					t.Fatalf("%s %s candidates = %#v", direction, field, provenance.Candidates)
				}
				encoded, err := json.Marshal(merged)
				if err != nil {
					t.Fatalf("json.Marshal() error = %v", err)
				}
				var object map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &object); err != nil {
					t.Fatalf("json.Unmarshal() error = %v", err)
				}
				if string(object[field]) != "[]" {
					t.Fatalf("%s JSON field = %s, full JSON = %s", field, object[field], encoded)
				}
				var wire struct {
					FieldProvenance map[string]struct {
						Value      []string `json:"value"`
						Candidates []struct {
							Value    []string `json:"value"`
							Source   string   `json:"source"`
							Selected bool     `json:"selected"`
						} `json:"candidates"`
					} `json:"field_provenance"`
				}
				if err := json.Unmarshal(encoded, &wire); err != nil {
					t.Fatalf("decode provenance JSON error = %v", err)
				}
				wireProvenance := wire.FieldProvenance[field]
				if wireProvenance.Value == nil || len(wireProvenance.Value) != 0 {
					t.Fatalf("%s wire provenance value = %#v, want JSON []", field, wireProvenance.Value)
				}
				selectedEmpty := false
				for _, candidate := range wireProvenance.Candidates {
					if candidate.Source == "reviewed.json" && candidate.Selected && candidate.Value != nil && len(candidate.Value) == 0 {
						selectedEmpty = true
					}
				}
				if !selectedEmpty {
					t.Fatalf("%s wire candidates = %#v, want selected reviewed JSON []", field, wireProvenance.Candidates)
				}
			}
		})
	}
}

func TestSelectionListsRejectSameRankEmptyVersusNonEmpty(t *testing.T) {
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		t.Run(field, func(t *testing.T) {
			empty := toolMetadataWithList(field, []string{}, selectionRankExplicit, "empty.json")
			nonempty := toolMetadataWithList(field, []string{"value"}, selectionRankExplicit, "nonempty.json")
			for _, pair := range [][2]ToolMetadata{{empty, nonempty}, {nonempty, empty}} {
				_, err := mergeToolMetadata(pair[0], pair[1], "calendar event get")
				if err == nil || !strings.Contains(err.Error(), "field "+field) || !strings.Contains(err.Error(), "empty.json") || !strings.Contains(err.Error(), "nonempty.json") {
					t.Fatalf("merge error = %v, want same-rank [] versus non-empty conflict", err)
				}
			}
		})
	}
}

func TestSelectionListsIgnoreOmittedHigherRankValue(t *testing.T) {
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		t.Run(field, func(t *testing.T) {
			low := toolMetadataWithList(field, []string{"lower value"}, selectionRankImported, "imported.json")
			omitted := toolMetadataWithList(field, nil, selectionRankReviewedExplicit, "reviewed.json")
			setToolMetadataListPresence(&omitted, field, false)

			for _, pair := range [][2]ToolMetadata{{low, omitted}, {omitted, low}} {
				merged, err := mergeToolMetadata(pair[0], pair[1], "calendar event get")
				if err != nil {
					t.Fatalf("merge error = %v", err)
				}
				if got, want := toolMetadataList(merged, field), []string{"lower value"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("%s = %#v, want lower authored value %#v", field, got, want)
				}
				provenance := merged.FieldProvenance[field]
				if provenance.Source != "imported.json" || hasListCandidate(provenance.Candidates, []string{}, "reviewed.json", false) {
					t.Fatalf("%s provenance = %#v, omitted source must not participate", field, provenance)
				}
			}
		})
	}
}

func TestHintExplicitEmptyOverridesImportedAndOmittedDoesNotParticipate(t *testing.T) {
	reviewed := true
	out := File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}
	stats := Stats{}
	origins := sourceTracker{}
	imported := parsedHintSource{
		file: sourceFile{display: "imported.json"},
		hint: HintFile{
			Source: HintSource{Kind: "imported", Name: "imported"},
			Tools: map[string]HintTool{
				"calendar event get": {UseWhen: []string{"lower value"}},
			},
		},
	}
	explicit := parsedHintSource{
		file: sourceFile{display: "reviewed.json"},
		hint: HintFile{
			Source: HintSource{Kind: "explicit", Name: "reviewed"},
			Tools: map[string]HintTool{
				"calendar event get":  {UseWhen: []string{}, Reviewed: &reviewed},
				"calendar event list": {Reviewed: &reviewed},
			},
		},
	}
	if err := applyHintSource(&out, imported, &stats, origins); err != nil {
		t.Fatalf("apply imported hint error = %v", err)
	}
	if err := applyHintSource(&out, explicit, &stats, origins); err != nil {
		t.Fatalf("apply explicit hint error = %v", err)
	}
	normalizeFile(&out, 0)

	got := out.Tools["calendar event get"]
	if got.UseWhen == nil || len(got.UseWhen) != 0 || !got.useWhenPresent {
		t.Fatalf("explicit use_when = %#v, present=%v; want [] selected", got.UseWhen, got.useWhenPresent)
	}
	if _, exists := out.Tools["calendar event list"].FieldProvenance["use_when"]; exists {
		t.Fatalf("omitted use_when unexpectedly participated: %#v", out.Tools["calendar event list"].FieldProvenance)
	}
}

func TestHintJSONDistinguishesOmittedAndExplicitEmptyList(t *testing.T) {
	var omitted, explicitEmpty HintTool
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("decode omitted hint error = %v", err)
	}
	if err := json.Unmarshal([]byte(`{"use_when":[]}`), &explicitEmpty); err != nil {
		t.Fatalf("decode explicit-empty hint error = %v", err)
	}
	if omitted.UseWhen != nil {
		t.Fatalf("omitted use_when = %#v, want nil", omitted.UseWhen)
	}
	if explicitEmpty.UseWhen == nil || len(explicitEmpty.UseWhen) != 0 {
		t.Fatalf("explicit use_when = %#v, want non-nil []", explicitEmpty.UseWhen)
	}
	if hasAgentHintFields(omitted) {
		t.Fatal("omitted list unexpectedly counts as an authored hint")
	}
	if !hasAgentHintFields(explicitEmpty) {
		t.Fatal("explicit [] must count as an authored hint")
	}
}

func TestProductSelectionListExplicitEmptyIsSerializedAndProvenanced(t *testing.T) {
	metadata := ProductMetadata{
		UseWhen:        []string{},
		useWhenPresent: true,
		useWhenRank:    selectionRankReviewedExplicit,
		useWhenOrigin:  "reviewed.json",
	}
	recordProductListCandidate(&metadata, "use_when", metadata.UseWhen, true, metadata.useWhenRank, metadata.useWhenOrigin)
	file := File{Products: map[string]ProductMetadata{"calendar": metadata}, Tools: map[string]ToolMetadata{}}
	normalizeFile(&file, 0)
	product := file.Products["calendar"]
	provenance := product.FieldProvenance["use_when"]
	if winner, ok := provenance.Value.([]string); !ok || winner == nil || len(winner) != 0 || !hasListCandidate(provenance.Candidates, []string{}, "reviewed.json", true) {
		t.Fatalf("product provenance = %#v", provenance)
	}
	encoded, err := json.Marshal(product)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"use_when":[]`) {
		t.Fatalf("product JSON = %s, want explicit use_when:[]", encoded)
	}
}

func toolMetadataWithList(field string, values []string, rank int, origin string) ToolMetadata {
	metadata := ToolMetadata{}
	values = cloneStringList(values)
	switch field {
	case "use_when":
		metadata.UseWhen, metadata.useWhenPresent, metadata.useWhenRank, metadata.useWhenOrigin = values, values != nil, rank, origin
	case "avoid_when":
		metadata.AvoidWhen, metadata.avoidWhenPresent, metadata.avoidWhenRank, metadata.avoidWhenOrigin = values, values != nil, rank, origin
	case "prerequisites":
		metadata.Prerequisites, metadata.prerequisitesPresent, metadata.prerequisitesRank, metadata.prerequisitesOrigin = values, values != nil, rank, origin
	case "tips":
		metadata.Tips, metadata.tipsPresent, metadata.tipsRank, metadata.tipsOrigin = values, values != nil, rank, origin
	case "workflow_refs":
		metadata.WorkflowRefs, metadata.workflowRefsPresent, metadata.workflowRefsRank, metadata.workflowRefsOrigin = values, values != nil, rank, origin
	case "examples":
		metadata.Examples, metadata.examplesPresent, metadata.examplesRank, metadata.examplesOrigin = values, values != nil, rank, origin
	}
	return metadata
}

func setToolMetadataListPresence(metadata *ToolMetadata, field string, present bool) {
	switch field {
	case "use_when":
		metadata.useWhenPresent = present
	case "avoid_when":
		metadata.avoidWhenPresent = present
	case "prerequisites":
		metadata.prerequisitesPresent = present
	case "tips":
		metadata.tipsPresent = present
	case "workflow_refs":
		metadata.workflowRefsPresent = present
	case "examples":
		metadata.examplesPresent = present
	}
}

func toolMetadataList(metadata ToolMetadata, field string) []string {
	switch field {
	case "use_when":
		return metadata.UseWhen
	case "avoid_when":
		return metadata.AvoidWhen
	case "prerequisites":
		return metadata.Prerequisites
	case "tips":
		return metadata.Tips
	case "workflow_refs":
		return metadata.WorkflowRefs
	case "examples":
		return metadata.Examples
	default:
		return nil
	}
}

func hasListCandidate(candidates []FieldCandidateProvenance, values []string, source string, selected bool) bool {
	for _, candidate := range candidates {
		actual, ok := candidate.Value.([]string)
		if ok && reflect.DeepEqual(actual, values) && candidate.Source == source && candidate.Selected == selected {
			return true
		}
	}
	return false
}
