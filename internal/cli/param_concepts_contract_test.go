// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParamConceptsJSONSchemaDocumentsClosedShape(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(embeddedParamConceptsSchemaJSON, &schema); err != nil {
		t.Fatalf("decode param_concepts.schema.json: %v", err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["additionalProperties"] != false {
		t.Fatalf("param concepts root schema is not closed: %#v", schema)
	}
	definitions := schema["$defs"].(map[string]any)
	concept := definitions["concept"].(map[string]any)
	if concept["additionalProperties"] != false {
		t.Fatalf("concept schema allows unknown fields: %#v", concept)
	}
	properties := concept["properties"].(map[string]any)
	for _, field := range []string{"denotes", "canonical_hint", "members", "excludes", "commands", "risk"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("concept schema is missing %s", field)
		}
	}
	override := definitions["commandOverride"].(map[string]any)
	if override["additionalProperties"] != false {
		t.Fatalf("commandOverride schema allows unknown fields: %#v", override)
	}

	var source map[string]any
	if err := json.Unmarshal(embeddedParamConceptsJSON, &source); err != nil {
		t.Fatalf("decode param_concepts.json: %v", err)
	}
	if source["$schema"] != paramConceptsSchemaRef {
		t.Fatalf("param concepts source $schema = %#v, want %q", source["$schema"], paramConceptsSchemaRef)
	}
}

func TestEmbeddedParamConceptsLoadsAndSatisfiesInvariants(t *testing.T) {
	concepts, err := LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}
	if len(concepts.Concepts) == 0 {
		t.Fatal("embedded param concepts declares no concepts")
	}

	// Members must be globally unique and disjoint from their own excludes.
	memberOwner := make(map[string]string)
	for _, concept := range concepts.Concepts {
		if len(concept.Commands) == 0 {
			t.Fatalf("concept %s has no reviewed command scope", concept.ID)
		}
		excludeSet := make(map[string]bool, len(concept.Excludes))
		for _, exclude := range concept.Excludes {
			excludeSet[exclude] = true
		}
		for _, member := range concept.Members {
			if owner, exists := memberOwner[member]; exists {
				t.Fatalf("member %q belongs to both concept %s and %s", member, owner, concept.ID)
			}
			memberOwner[member] = concept.ID
			if excludeSet[member] {
				t.Fatalf("concept %s lists %q as both member and exclude", concept.ID, member)
			}
		}
	}

	// Every bind target must reference a declared concept.
	for _, override := range concepts.Overrides {
		if override.Confirm || override.Investigate {
			t.Fatalf("current override %q remains unresolved (confirm=%v investigate=%v)", override.CommandPath, override.Confirm, override.Investigate)
		}
		for flag, conceptID := range override.Bind {
			if _, ok := concepts.ByConcept[conceptID]; !ok {
				t.Fatalf("command_override %q binds %q to undeclared concept %q", override.CommandPath, flag, conceptID)
			}
		}
	}

	// Fixture sentinels are limited to the two known did-you-mean forms.
	for _, c := range concepts.Fixture {
		if strings.HasPrefix(c.Expect, "did-you-mean:") &&
			c.Expect != paramDidYouMeanAmbiguous && c.Expect != paramDidYouMeanBlocked {
			t.Fatalf("fixture %q/%q has unknown sentinel %q", c.Command, c.Emitted, c.Expect)
		}
	}
}

func TestParamConceptRiskAuditBoundaries(t *testing.T) {
	concepts, err := LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}
	assertMembers := func(id string, forbidden ...string) {
		t.Helper()
		concept, ok := concepts.ByConcept[id]
		if !ok {
			t.Fatalf("missing audited concept %q", id)
		}
		members := make(map[string]bool, len(concept.Members))
		for _, member := range concept.Members {
			members[member] = true
		}
		for _, name := range forbidden {
			if members[name] {
				t.Fatalf("audited concept %s still contains forbidden cross-semantics member %q", id, name)
			}
		}
	}
	assertMembers("user_id", "users", "user-ids")
	assertMembers("user_ids", "user", "user-id")
	assertMembers("dept_id", "depts", "dept-ids")
	assertMembers("dept_ids", "dept", "dept-id")
	assertMembers("group_id", "conversation-ids", "group-ids")
	assertMembers("page_number", "page-index")
	assertMembers("robot_code", "robot-id")
}

func TestDecodeParamConceptsRejectsUnknownFieldsAtEveryLevel(t *testing.T) {
	valid := `{"$schema":"./param_concepts.schema.json","version":1,` +
		`"concepts":{"search_query":{"denotes":"d","canonical_hint":"query","members":["query"],"commands":["demo cmd"],"risk":"green"}},` +
		`"command_overrides":{"chat group rename":{"bind":{"id":"search_query"}}}}`
	for name, input := range map[string]string{
		"root":     strings.Replace(valid, `"version":1`, `"version":1,"unknown":true`, 1),
		"concept":  strings.Replace(valid, `"risk":"green"`, `"risk":"green","unknown":true`, 1),
		"override": strings.Replace(valid, `"bind":{"id":"search_query"}`, `"bind":{"id":"search_query"},"unknown":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeParamConcepts([]byte(input)); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("decodeParamConcepts() error = %v, want unknown field", err)
			}
		})
	}
}

func TestDecodeParamConceptsEnforcesReviewedConstraints(t *testing.T) {
	wrap := func(body string) string {
		return `{"$schema":"./param_concepts.schema.json","version":1,` + body + `}`
	}
	concept := func(members, excludes, risk string) string {
		return `"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":` + members + `,"excludes":` + excludes + `,"commands":["demo cmd"],"risk":"` + risk + `"}}`
	}
	tests := map[string]string{
		"missing schema ref":       `{"version":1,"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}}`,
		"wrong version":            `{"$schema":"./param_concepts.schema.json","version":2,"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}}`,
		"no concepts":              wrap(`"concepts":{}`),
		"invalid concept id":       wrap(`"concepts":{"BadID":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}`),
		"empty denotes":            wrap(`"concepts":{"c_one":{"denotes":"","canonical_hint":"query","members":["query"],"risk":"green"}}`),
		"invalid risk":             wrap(concept(`["query"]`, `[]`, "red")),
		"no members":               wrap(`"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":[],"risk":"green"}}`),
		"no command scope":         wrap(`"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"commands":[],"risk":"green"}}`),
		"member equals exclude":    wrap(concept(`["query"]`, `["query"]`, "green")),
		"member overlaps concepts": wrap(`"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"},"c_two":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}`),
		"bind undeclared concept":  wrap(concept(`["query"]`, `[]`, "green") + `,"command_overrides":{"chat group rename":{"bind":{"id":"missing"}}}`),
		"empty override":           wrap(concept(`["query"]`, `[]`, "green") + `,"command_overrides":{"chat group rename":{}}`),
		"bad fixture sentinel":     wrap(concept(`["query"]`, `[]`, "green") + `,"validation_fixture":{"cases":[{"command":"chat group rename","emitted":"group","expect":"did-you-mean:oops"}]}`),
		"empty fixture cases":      wrap(concept(`["query"]`, `[]`, "green") + `,"validation_fixture":{"cases":[]}`),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeParamConcepts([]byte(input)); err == nil {
				t.Fatal("decodeParamConcepts() unexpectedly accepted invalid reviewed source")
			}
		})
	}

	got, err := decodeParamConcepts([]byte(wrap(concept(`["query","keyword"]`, `["name"]`, "green"))))
	if err != nil {
		t.Fatalf("decodeParamConcepts() valid source error = %v", err)
	}
	if len(got.Concepts) != 1 || got.Concepts[0].ID != "c_one" {
		t.Fatalf("decodeParamConcepts() concepts = %#v", got.Concepts)
	}
}

func validParamConceptSpecFixture() paramConceptSpec {
	return paramConceptSpec{
		Denotes:       "a reviewed value",
		CanonicalHint: "query",
		Members:       []string{"query"},
		Excludes:      []string{"name"},
		Commands:      []string{"demo run"},
		Risk:          "green",
	}
}

func TestDecodeParamConceptsRemainingSyntaxEdges(t *testing.T) {
	valid := `{"$schema":"./param_concepts.schema.json","version":1,` +
		`"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"commands":["demo run"],"risk":"green"}}}`
	if _, err := decodeParamConcepts([]byte(valid + ` {}`)); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("multiple JSON values error = %v", err)
	}

	withEmptyMorphDescription := strings.Replace(
		valid,
		`"concepts":`,
		`"morphological_rules":{"camel":{"desc":""}},"concepts":`,
		1,
	)
	if _, err := decodeParamConcepts([]byte(withEmptyMorphDescription)); err == nil || !strings.Contains(err.Error(), "empty desc") {
		t.Fatalf("empty morph description error = %v", err)
	}
}

func TestDecodeParamConceptSpecsRemainingValidationEdges(t *testing.T) {
	base := validParamConceptSpecFixture()
	invalidCanonical := base
	invalidCanonical.CanonicalHint = "bad token"
	invalidMember := base
	invalidMember.Members = []string{"bad token"}
	repeatedMember := base
	repeatedMember.Members = []string{"query", "query"}
	invalidExclude := base
	invalidExclude.Excludes = []string{"bad token"}
	repeatedExclude := base
	repeatedExclude.Excludes = []string{"name", "name"}
	invalidCommand := base
	invalidCommand.Commands = []string{"demo /bad"}
	repeatedCommand := base
	repeatedCommand.Commands = []string{"demo run", "demo run"}

	tests := map[string]map[string]paramConceptSpec{
		"invalid canonical hint": {"c_one": invalidCanonical},
		"invalid member":         {"c_one": invalidMember},
		"repeated member":        {"c_one": repeatedMember},
		"invalid exclude":        {"c_one": invalidExclude},
		"repeated exclude":       {"c_one": repeatedExclude},
		"invalid command":        {"c_one": invalidCommand},
		"repeated command":       {"c_one": repeatedCommand},
		"cross-concept member": {
			"c_one": base,
			"c_two": base,
		},
	}
	for name, specs := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeParamConceptSpecs(specs); err == nil {
				t.Fatal("decodeParamConceptSpecs() unexpectedly accepted invalid input")
			}
		})
	}
}

func TestDecodeParamCommandOverridesRemainingValidationEdges(t *testing.T) {
	byConcept := map[string]Concept{"c_one": {ID: "c_one"}}
	tests := map[string]map[string]paramCommandOverride{
		"invalid path": {
			" demo run": {Block: []string{"name"}},
		},
		"invalid bind flag": {
			"demo run": {Bind: map[string]string{"bad token": "c_one"}},
		},
		"invalid scoped emitted": {
			"demo run": {ScopedAliases: map[string]string{"bad token": "query"}},
		},
		"invalid scoped target": {
			"demo run": {ScopedAliases: map[string]string{"keyword": "bad token"}},
		},
		"invalid block token": {
			"demo run": {Block: []string{"bad token"}},
		},
		"repeated ambiguous token": {
			"demo run": {Ambiguous: []string{"uid", "uid"}},
		},
	}
	for name, specs := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeParamCommandOverrides(specs, byConcept); err == nil {
				t.Fatal("decodeParamCommandOverrides() unexpectedly accepted invalid input")
			}
		})
	}
}

func TestDecodeParamFixtureCasesRemainingValidationEdges(t *testing.T) {
	tests := map[string]paramFixtureCaseSpec{
		"invalid command": {Command: " demo run", Emitted: "query", Expect: "query"},
		"invalid emitted": {Command: "demo run", Emitted: "bad token", Expect: "query"},
		"empty expect":    {Command: "demo run", Emitted: "query", Expect: " "},
		"invalid expect":  {Command: "demo run", Emitted: "query", Expect: "bad token"},
		"negative occ":    {Command: "demo run", Emitted: "query", Expect: "query", Occ: -1},
	}
	for name, fixture := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeParamFixtureCases(&paramValidationFixtureSpec{Cases: []paramFixtureCaseSpec{fixture}}); err == nil {
				t.Fatal("decodeParamFixtureCases() unexpectedly accepted invalid input")
			}
		})
	}
}

func TestParamConceptPathAndTokenValidationEdges(t *testing.T) {
	if validParamCommandPath(" demo run") {
		t.Fatal("command path with surrounding whitespace was accepted")
	}
	if validParamCommandPath("demo /bad") {
		t.Fatal("command path with an invalid token was accepted")
	}
	if err := validParamTokenList("demo run", "block", []string{"bad token"}); err == nil {
		t.Fatal("invalid parameter token was accepted")
	}
	if err := validParamTokenList("demo run", "block", []string{"uid", "uid"}); err == nil {
		t.Fatal("repeated parameter token was accepted")
	}
}
