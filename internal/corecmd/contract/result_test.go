// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeResultSpecCanonicalizesAndCopies(t *testing.T) {
	in := &ResultSpec{
		Outcomes:       []ResultOutcome{ResultOutcomeFailure, ResultOutcomeSuccess},
		DataSchema:     json.RawMessage(`{ "properties": {"items":{"type":"array","description":"Result records","items":{"type":"object"}}}, "type":"object" }`),
		SensitivePaths: []string{"items.secret", "credential"},
	}

	got, err := NormalizeResultSpec(in, "dev.list")
	if err != nil {
		t.Fatalf("NormalizeResultSpec() error = %v", err)
	}
	if want := []ResultOutcome{ResultOutcomeSuccess, ResultOutcomeFailure}; !reflect.DeepEqual(got.Outcomes, want) {
		t.Fatalf("outcomes = %#v, want %#v", got.Outcomes, want)
	}
	if string(got.DataSchema) != `{"properties":{"items":{"type":"array","description":"Result records","items":{"type":"object"}}},"type":"object"}` {
		t.Fatalf("data_schema = %s", got.DataSchema)
	}
	if want := []string{"credential", "items.secret"}; !reflect.DeepEqual(got.SensitivePaths, want) {
		t.Fatalf("sensitive_paths = %#v, want %#v", got.SensitivePaths, want)
	}

	in.Outcomes[0] = ResultOutcomePending
	in.DataSchema[0] = '['
	in.SensitivePaths[0] = "changed"
	if got.Outcomes[0] != ResultOutcomeSuccess || got.DataSchema[0] != '{' || got.SensitivePaths[0] != "credential" {
		t.Fatalf("normalized result aliases input: %#v", got)
	}
}

func TestNormalizeResultSpecRejectsInvalidContractsDeterministically(t *testing.T) {
	valid := func() *ResultSpec {
		return &ResultSpec{Outcomes: []ResultOutcome{ResultOutcomeSuccess}, DataSchema: json.RawMessage(`{"type":"object"}`)}
	}
	tests := []struct {
		name string
		edit func(*ResultSpec)
		want string
	}{
		{"no outcomes", func(r *ResultSpec) { r.Outcomes = nil }, "no outcomes"},
		{"unknown outcome", func(r *ResultSpec) { r.Outcomes = []ResultOutcome{"ok"} }, "unknown outcome"},
		{"duplicate outcome", func(r *ResultSpec) { r.Outcomes = []ResultOutcome{ResultOutcomeSuccess, ResultOutcomeSuccess} }, "duplicate outcome"},
		{"schema array", func(r *ResultSpec) { r.DataSchema = json.RawMessage(`[]`) }, "data_schema: must be one JSON object"},
		{"multiple schemas", func(r *ResultSpec) { r.DataSchema = json.RawMessage(`{} {}`) }, "data_schema: must be one JSON object"},
		{"missing property description", func(r *ResultSpec) {
			r.DataSchema = json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`)
		}, "properties.id requires description"},
		{"unsafe path", func(r *ResultSpec) { r.SensitivePaths = []string{"$.token"} }, "unsafe segment"},
		{"duplicate path", func(r *ResultSpec) { r.SensitivePaths = []string{"token", " token "} }, "duplicate sensitive path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := valid()
			test.edit(spec)
			_, err := NormalizeResultSpec(spec, "dev.test")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNormalizePaginationSpecUsesFrameworkMetaPaths(t *testing.T) {
	got, err := NormalizePaginationSpec(&PaginationSpec{Kind: PaginationKindCursor, CursorParameter: "--cursor"}, "dev.list")
	if err != nil {
		t.Fatal(err)
	}
	if got.CursorParameter != "cursor" || got.MetaPath != PaginationMetaPath || got.EndpointExhaustedPath != PaginationExhaustedPath || got.NextTokenPath != PaginationNextTokenPath {
		t.Fatalf("pagination = %#v", got)
	}
	for _, spec := range []*PaginationSpec{
		{Kind: "offset", CursorParameter: "cursor"},
		{Kind: PaginationKindCursor},
		{Kind: PaginationKindCursor, CursorParameter: "cursor", MetaPath: "data.pagination"},
	} {
		if _, err := NormalizePaginationSpec(spec, "dev.list"); err == nil {
			t.Fatalf("invalid pagination accepted: %#v", spec)
		}
	}
}
