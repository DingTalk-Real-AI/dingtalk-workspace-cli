// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func validParameterBindingSnapshotForCoverage(t *testing.T) schemaParameterBindingSnapshot {
	t.Helper()
	bindings := map[string]map[string]string{
		"sample.read":  {"id": "itemId", "limit": "pageSize"},
		"sample.write": {"name": "name"},
	}
	hash, err := schemaParameterBindingManifestHash(bindings)
	if err != nil {
		t.Fatal(err)
	}
	return schemaParameterBindingSnapshot{
		Version:  schemaParameterBindingsVersion,
		Baseline: schemaParameterBindingBaseline{Manifest: schemaParameterBindingsBaselineManifest, SHA256: hash, Reason: "reviewed", Reviewed: true},
		Bindings: bindings,
	}
}

func TestCrossPlatformCoverageSchemaParameterBindingSnapshotAuditEdges(t *testing.T) {
	valid := validParameterBindingSnapshotForCoverage(t)
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchemaParameterBindingsSource(encoded); err != nil {
		t.Fatalf("ValidateSchemaParameterBindingsSource() = %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*schemaParameterBindingSnapshot)
		want   string
	}{
		{name: "manifest", mutate: func(s *schemaParameterBindingSnapshot) { s.Baseline.Manifest = " wrong " }, want: "must declare manifest"},
		{name: "empty active", mutate: func(s *schemaParameterBindingSnapshot) { s.Bindings = nil }, want: "active manifest is empty"},
		{name: "correction key", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Corrections = map[string]schemaParameterBindingCorrection{"bad": {}}
		}, want: "correction: invalid exact"},
		{name: "correction properties", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Corrections = map[string]schemaParameterBindingCorrection{"sample.read --id": {OldProperty: "same", NewProperty: "same", Reason: "r", Reviewed: true}}
		}, want: "invalid old/new"},
		{name: "correction review", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Corrections = map[string]schemaParameterBindingCorrection{"sample.read --id": {OldProperty: "old", NewProperty: "itemId"}}
		}, want: "must be reviewed"},
		{name: "correction active", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Corrections = map[string]schemaParameterBindingCorrection{"sample.read --id": {OldProperty: "old", NewProperty: "other", Reason: "r", Reviewed: true}}
		}, want: "active manifest"},
		{name: "removal key", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"bad": {}}
		}, want: "removal: invalid exact"},
		{name: "removal active", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.read --id": {Reason: "r", Reviewed: true}}
		}, want: "still exists"},
		{name: "removal review", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {}}
		}, want: "must be reviewed"},
		{name: "removal replaced trim", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true, ReplacedBy: " sample.read --id"}}
		}, want: "non-canonical replaced_by"},
		{name: "removal replacement key", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true, ReplacedBy: "bad"}}
		}, want: "replaced_by: invalid exact"},
		{name: "removal replacement inactive", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true, ReplacedBy: "sample.other --id"}}
		}, want: "is not active"},
		{name: "exclusion key", mutate: func(s *schemaParameterBindingSnapshot) {
			s.MappingExclusions = map[string]string{"bad": "r"}
		}, want: "exclusion: invalid exact"},
		{name: "exclusion active", mutate: func(s *schemaParameterBindingSnapshot) {
			s.MappingExclusions = map[string]string{"sample.read --id": "r"}
		}, want: "conflicts with the active"},
		{name: "exclusion removal", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true}}
			s.MappingExclusions = map[string]string{"sample.old --id": "r"}
		}, want: "also recorded as a removal"},
		{name: "exclusion reason", mutate: func(s *schemaParameterBindingSnapshot) {
			s.MappingExclusions = map[string]string{"sample.old --id": " "}
		}, want: "exact non-empty reason"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validParameterBindingSnapshotForCoverage(t)
			tc.mutate(&snapshot)
			err := validateSchemaParameterBindingSnapshot(snapshot)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	valid.Corrections = map[string]schemaParameterBindingCorrection{
		"sample.read --id": {OldProperty: "oldId", NewProperty: "itemId", Reason: "reviewed", Reviewed: true},
	}
	valid.Removals = map[string]schemaParameterBindingRemoval{
		"sample.old --id": {Reason: "reviewed", ReplacedBy: "sample.read --id", Reviewed: true},
	}
	valid.MappingExclusions = map[string]string{"sample.read --local": "reviewed"}
	if err := validateSchemaParameterBindingSnapshot(valid); err != nil {
		t.Fatalf("valid audit records rejected: %v", err)
	}
}

func TestCrossPlatformCoverageSchemaParameterBindingDeliveryRemainingEdges(t *testing.T) {
	command := &cobra.Command{Use: "read"}
	command.Flags().String("id", "", "id")
	boundSpec := BoundCommandSpec{CommandSpec: CommandSpec{CanonicalPath: "sample.read", Visibility: SchemaVisibilityPublic}, PrimaryCommand: command}
	bound := BoundCommandRegistry{Commands: []BoundCommandSpec{boundSpec}, ByCanonical: map[string]BoundCommandSpec{"sample.read": boundSpec}}
	versioned := FieldProvenance{Value: json.RawMessage(`"itemId"`), Source: "versioned_parameter_binding", Resolution: "selected"}
	parameter := ParameterSpec{Name: "id", Property: "itemId", FieldProvenance: map[string]FieldProvenance{"property": versioned}}
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{Identity: ToolIdentitySpec{CanonicalPath: "sample.read"}, Parameters: []ParameterSpec{parameter}}}}}}

	cases := []struct {
		name     string
		snapshot schemaParameterBindingSnapshot
		bound    BoundCommandRegistry
		registry SchemaRegistry
		want     string
	}{
		{name: "empty canonical", snapshot: schemaParameterBindingSnapshot{}, bound: bound, registry: SchemaRegistry{Products: []ProductSpec{{Tools: []ToolSpec{{}}}}}, want: "empty canonical_path"},
		{
			name:     "duplicate tool",
			snapshot: schemaParameterBindingSnapshot{},
			bound:    bound,
			registry: SchemaRegistry{Products: []ProductSpec{{Tools: []ToolSpec{
				{Identity: ToolIdentitySpec{CanonicalPath: "sample.read"}},
				{Identity: ToolIdentitySpec{CanonicalPath: "sample.read"}},
			}}}},
			want: "duplicate canonical",
		},
		{name: "missing cobra flag", snapshot: schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{"sample.read": {"missing": "value"}}}, bound: bound, registry: registry, want: "exact bound Cobra flag"},
		{name: "missing parameter", snapshot: schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{"sample.read": {"other": "value"}}}, bound: bound, registry: registry, want: "exact final public Schema parameter"},
		{name: "property mismatch", snapshot: schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{"sample.read": {"id": "other"}}}, bound: bound, registry: registry, want: "final Schema"},
		{
			name:     "provenance mismatch",
			snapshot: schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{"sample.read": {"id": "itemId"}}},
			bound:    bound,
			registry: SchemaRegistry{Products: []ProductSpec{{Tools: []ToolSpec{{
				Identity:   ToolIdentitySpec{CanonicalPath: "sample.read"},
				Parameters: []ParameterSpec{{Name: "id", Property: "itemId"}},
			}}}}},
			want: "no exact versioned",
		},
		{name: "exclusion bound", snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{"sample.other --id": "r"}}, bound: bound, registry: registry, want: "exclusion \"sample.other --id\" does not reference a public"},
		{name: "exclusion flag", snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{"sample.read --missing": "r"}}, bound: bound, registry: registry, want: "exact bound Cobra flag"},
		{name: "exclusion property", snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{"sample.read --id": "r"}}, bound: bound, registry: registry, want: "want omitted"},
		{name: "removal delivered", snapshot: schemaParameterBindingSnapshot{Removals: map[string]schemaParameterBindingRemoval{"sample.read --id": {}}}, bound: bound, registry: registry, want: "still delivered"},
		{name: "reverse version claim", snapshot: schemaParameterBindingSnapshot{}, bound: bound, registry: registry, want: "claims versioned binding provenance"},
		{
			name:     "reverse exclusion claim",
			snapshot: schemaParameterBindingSnapshot{},
			bound:    bound,
			registry: SchemaRegistry{Products: []ProductSpec{{Tools: []ToolSpec{{
				Identity: ToolIdentitySpec{CanonicalPath: "sample.read"},
				Parameters: []ParameterSpec{{Name: "id", FieldProvenance: map[string]FieldProvenance{
					"property": {Source: "reviewed_mapping_exclusion"},
				}}},
			}}}}},
			want: "claims mapping exclusion provenance",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSchemaParameterBindingDelivery(tc.snapshot, tc.bound, tc.registry)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	noPropertyProvenance := registry
	noPropertyProvenance.Products[0].Tools[0].Parameters[0].FieldProvenance = map[string]FieldProvenance{}
	if err := validateSchemaParameterBindingDelivery(schemaParameterBindingSnapshot{}, bound, noPropertyProvenance); err != nil {
		t.Fatalf("absent optional property provenance rejected: %v", err)
	}
}

func TestCrossPlatformCoverageSchemaParameterBindingHelpersAndLoaderErrors(t *testing.T) {
	if _, ok := finalSchemaParameterByName(ToolSpec{}, "missing"); ok {
		t.Fatal("missing parameter found")
	}
	selected := FieldProvenance{Value: json.RawMessage(`"value"`), Source: "source"}
	if !schemaParameterProvenanceHasStringCandidate(selected, "source", "value") {
		t.Fatal("selected provenance not found")
	}
	if schemaParameterProvenanceHasStringCandidate(FieldProvenance{Value: json.RawMessage(`{`), Source: "source"}, "source", "value") {
		t.Fatal("invalid selected provenance matched")
	}
	candidates := FieldProvenance{Candidates: []FieldCandidateProvenance{{Source: "other", Value: json.RawMessage(`"value"`)}, {Source: "source", Value: json.RawMessage(`{`)}, {Source: "source", Value: json.RawMessage(`"value"`)}}}
	if !schemaParameterProvenanceHasStringCandidate(candidates, "source", "value") || schemaParameterProvenanceHasStringCandidate(candidates, "missing", "value") {
		t.Fatal("candidate provenance lookup failed")
	}

	manifestCases := []map[string]map[string]string{
		nil,
		{" bad": {"id": "value"}},
		{"sample.read": {}},
		{"sample.read": {" bad": "value"}},
		{"sample.read": {"id": " value "}},
	}
	for _, bindings := range manifestCases {
		if _, err := schemaParameterBindingManifestHash(bindings); err == nil {
			t.Fatalf("invalid bindings accepted: %#v", bindings)
		}
	}
	for _, key := range []string{"", " bad", "bad", "sample.read --id --other", "sample.read --", "sample.read --bad flag"} {
		if err := validateSchemaParameterBindingAuditKey(key); err == nil {
			t.Fatalf("invalid audit key %q accepted", key)
		}
	}

	command := &cobra.Command{Use: "read"}
	command.Flags().String("id", "", "id")
	applyRuntimeSchemaParameterBindingsFrom(command, " sample.read ", map[string]map[string]string{"sample.read": {"id": " itemId ", "missing": "ignored"}})
	if got := command.Flags().Lookup("id").Annotations[runtimeSchemaFlagBindingPropertyAnnotation]; len(got) != 1 || got[0] != "itemId" {
		t.Fatalf("binding annotation = %#v", got)
	}

	previous := schemaParameterBindingData
	schemaParameterBindingData = func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{}, errors.New("binding data failed")
	}
	t.Cleanup(func() { schemaParameterBindingData = previous })
	if err := ValidateEmbeddedSchemaParameterBindings(); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("ValidateEmbeddedSchemaParameterBindings() error = %v", err)
	}
	if err := ValidateSchemaParameterBindingDelivery(BoundCommandRegistry{}, SchemaRegistry{}); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("ValidateSchemaParameterBindingDelivery() error = %v", err)
	}
	if _, err := EmbeddedSchemaParameterBindings(); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("EmbeddedSchemaParameterBindings() error = %v", err)
	}
}
