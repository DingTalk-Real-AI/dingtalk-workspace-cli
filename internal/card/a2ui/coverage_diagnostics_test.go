// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

func TestCrossPlatformCoverageAicardDiagnosticLookupBoundaries(t *testing.T) {
	p := &Protocol{docs: map[string]map[string]any{"one": {"$id": "urn:example:one", "properties": map[string]any{"a/b": map[string]any{"type": "string"}}}, "bad": {"$id": ":"}}}
	for _, tc := range []struct {
		value any
		path  []string
		want  any
	}{
		{[]any{"first"}, []string{"bad"}, nil},
		{[]any{"first"}, []string{"1"}, nil},
		{map[string]any{"child": 1}, []string{"child", "next"}, nil},
		{[]any{"first"}, []string{"0"}, "first"},
	} {
		if got := at(tc.value, tc.path); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("at(%v, %v) = %v", tc.value, tc.path, got)
		}
	}
	if p.schemaAt(":") != nil || p.schemaAt("urn:missing") != nil {
		t.Fatal("invalid and unknown schema URLs must not resolve")
	}
	if got := p.schemaAt("urn:example:one"); !reflect.DeepEqual(got, p.docs["one"]) {
		t.Fatal("root schema URL did not resolve")
	}
	if got := p.schemaAt("urn:example:one#/properties/a~1b"); got["type"] != "string" {
		t.Fatal("escaped JSON pointer did not resolve", got)
	}
	if resolve(":", "child") != "" || resolve("urn:example:one", ":") != "" {
		t.Fatal("invalid URL should not resolve")
	}
	if !p.wrongTag(map[string]any{"allOf": []any{map[string]any{"properties": map[string]any{"component": map[string]any{"const": "Text"}}}}}, map[string]any{"component": "Image"}, "", map[string]bool{}) {
		t.Fatal("allOf child tag mismatch was ignored")
	}
}

func TestCrossPlatformCoverageAicardDiagnosticRankingAndFallback(t *testing.T) {
	p := &Protocol{docs: map[string]map[string]any{"one": {"$id": "urn:example:one", "properties": map[string]any{"declared": true}}}}
	leaf := func(k jsonschema.ErrorKind, path ...string) *jsonschema.ValidationError {
		return &jsonschema.ValidationError{SchemaURL: "urn:example:one", InstanceLocation: path, ErrorKind: k}
	}
	branch := &jsonschema.ValidationError{ErrorKind: &kind.OneOf{}, Causes: []*jsonschema.ValidationError{
		leaf(&kind.Type{Want: []string{"object"}}),
		leaf(&kind.Enum{}, "component"),
		leaf(&kind.Required{Missing: []string{"component"}}),
	}}
	got := p.leaves(branch, map[string]any{"component": "Image"})
	if len(got) != 1 || got[0] != branch.Causes[2] {
		t.Fatalf("branch ranking picked %v", got)
	}
	branch.Causes = []*jsonschema.ValidationError{
		leaf(&kind.Enum{}, "component"), leaf(&kind.Required{Missing: []string{"component"}}),
	}
	got = p.leaves(branch, map[string]any{"component": "Image"})
	if len(got) != 1 || got[0] != branch.Causes[1] {
		t.Fatalf("tag mismatch ranking picked %v", got)
	}
	// A failed condition can leave a field unevaluated even when the owning
	// schema declares it. Preserve a generic failure rather than returning none.
	declared := &jsonschema.ValidationError{SchemaURL: "urn:example:one#/unevaluatedProperties", InstanceLocation: []string{"declared"}, ErrorKind: &kind.FalseSchema{}}
	ds := p.diagnostics(declared, map[string]any{"declared": true}, "/0")
	if len(ds) != 1 || ds[0].Code != "schema.invalid" || ds[0].Pointer != "/0" {
		t.Fatalf("declared field fallback: %+v", ds)
	}
	// Unknown properties duplicated by two failing alternatives should be
	// emitted once, and a more specific error at the same path wins.
	root := &jsonschema.ValidationError{ErrorKind: &kind.AllOf{}, Causes: []*jsonschema.ValidationError{
		leaf(&kind.AdditionalProperties{Properties: []string{"bad", "bad"}}),
		leaf(&kind.Type{Want: []string{"string"}}, "bad"),
		leaf(&kind.Required{Missing: []string{"z", "a"}}),
		leaf(&kind.FalseSchema{}, "other"),
	}}
	ds = p.diagnostics(root, map[string]any{"bad": 1}, "/0")
	if len(ds) != 3 || ds[0].Code != "schema.missing_required" || ds[1].Code != "schema.type_mismatch" || ds[2].Code != "schema.invalid" {
		t.Fatalf("sorted and filtered diagnostics: %+v", ds)
	}
	if !strings.Contains(ds[0].Message, "a, z") || !strings.Contains(ds[1].Message, "expected string") {
		t.Fatal(ds)
	}
	unknown := p.diagnostics(leaf(&kind.AdditionalProperties{Properties: []string{"x", "x"}}), map[string]any{}, "")
	if len(unknown) != 1 || unknown[0].Code != "schema.unknown_property" {
		t.Fatal(unknown)
	}
	// Different diagnostic codes on one path are ordered deterministically.
	same := &jsonschema.ValidationError{ErrorKind: &kind.AllOf{}, Causes: []*jsonschema.ValidationError{
		leaf(&kind.Type{Want: []string{"string"}}, "v"), leaf(&kind.Enum{}, "v"),
	}}
	ds = p.diagnostics(same, map[string]any{"v": 1}, "")
	if len(ds) != 2 || ds[0].Code != "schema.bad_enum" || ds[1].Code != "schema.type_mismatch" {
		t.Fatal(ds)
	}
}
