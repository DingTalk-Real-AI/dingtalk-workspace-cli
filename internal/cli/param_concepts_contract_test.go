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
	for _, field := range []string{"denotes", "canonical_hint", "members", "excludes", "risk"} {
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

func TestDecodeParamConceptsRejectsUnknownFieldsAtEveryLevel(t *testing.T) {
	valid := `{"$schema":"./param_concepts.schema.json","version":1,` +
		`"concepts":{"search_query":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}},` +
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
		return `"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":` + members + `,"excludes":` + excludes + `,"risk":"` + risk + `"}}`
	}
	tests := map[string]string{
		"missing schema ref":       `{"version":1,"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}}`,
		"wrong version":            `{"$schema":"./param_concepts.schema.json","version":2,"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}}`,
		"no concepts":              wrap(`"concepts":{}`),
		"invalid concept id":       wrap(`"concepts":{"BadID":{"denotes":"d","canonical_hint":"query","members":["query"],"risk":"green"}}`),
		"empty denotes":            wrap(`"concepts":{"c_one":{"denotes":"","canonical_hint":"query","members":["query"],"risk":"green"}}`),
		"invalid risk":             wrap(concept(`["query"]`, `[]`, "red")),
		"no members":               wrap(`"concepts":{"c_one":{"denotes":"d","canonical_hint":"query","members":[],"risk":"green"}}`),
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
