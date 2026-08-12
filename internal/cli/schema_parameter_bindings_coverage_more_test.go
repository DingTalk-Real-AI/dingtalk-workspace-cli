// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func validParameterBindingSnapshotForCoverage() schemaParameterBindingSnapshot {
	return schemaParameterBindingSnapshot{
		Bindings: map[string]map[string]string{},
		MappingExclusions: map[string]string{
			"sample.read --local": "reviewed local selector",
		},
		Removals: map[string]schemaParameterBindingRemoval{
			"sample.old --id": {Reason: "reviewed", Reviewed: true},
		},
	}
}

func TestCrossPlatformCoverageSchemaParameterBindingSnapshotAuditEdges(t *testing.T) {
	valid := validParameterBindingSnapshotForCoverage()
	if err := validateSchemaParameterBindingSnapshot(valid); err != nil {
		t.Fatalf("valid audit records rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*schemaParameterBindingSnapshot)
		want   string
	}{
		{name: "active bindings retired", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Bindings = map[string]map[string]string{"sample.read": {"id": "itemId"}}
		}, want: "active bindings must remain empty"},
		{name: "removal key", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"bad": {}}
		}, want: "removal: invalid exact"},
		{name: "removal review", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {}}
		}, want: "must be reviewed"},
		{name: "removal replaced trim", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true, ReplacedBy: " sample.read --id"}}
		}, want: "non-canonical replaced_by"},
		{name: "removal replacement retired", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true, ReplacedBy: "sample.read --id"}}
		}, want: "is not active"},
		{name: "exclusion key", mutate: func(s *schemaParameterBindingSnapshot) {
			s.MappingExclusions = map[string]string{"bad": "r"}
		}, want: "exclusion: invalid exact"},
		{name: "exclusion removal", mutate: func(s *schemaParameterBindingSnapshot) {
			s.Removals = map[string]schemaParameterBindingRemoval{"sample.old --id": {Reason: "r", Reviewed: true}}
			s.MappingExclusions = map[string]string{"sample.old --id": "r"}
		}, want: "also recorded as a removal"},
		{name: "exclusion reason", mutate: func(s *schemaParameterBindingSnapshot) {
			s.MappingExclusions = map[string]string{"sample.old --local": " "}
		}, want: "exact non-empty reason"},
		{name: "empty exclusions", mutate: func(s *schemaParameterBindingSnapshot) {
			s.MappingExclusions = map[string]string{}
		}, want: "mapping exclusions ledger must remain non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validParameterBindingSnapshotForCoverage()
			tc.mutate(&snapshot)
			err := validateSchemaParameterBindingSnapshot(snapshot)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaParameterBindingDeliveryRemainingEdges(t *testing.T) {
	command := &cobra.Command{Use: "read"}
	command.Flags().String("id", "", "id")
	boundSpec := BoundCommandSpec{CommandSpec: CommandSpec{CanonicalPath: "sample.read", Visibility: SchemaVisibilityPublic}, PrimaryCommand: command}
	bound := BoundCommandRegistry{Commands: []BoundCommandSpec{boundSpec}, ByCanonical: map[string]BoundCommandSpec{"sample.read": boundSpec}}
	versioned := contract.FieldProvenance{Value: json.RawMessage(`"itemId"`), Source: "versioned_parameter_binding", Resolution: "selected"}
	parameter := ParameterSpec{Name: "id", Property: "itemId", FieldProvenance: map[string]contract.FieldProvenance{"property": versioned}}
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.read"}, Parameters: []ParameterSpec{parameter}}}}}}

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
				{Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.read"}},
				{Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.read"}},
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
				Identity:   contract.ToolIdentitySpec{CanonicalPath: "sample.read"},
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
				Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.read"},
				Parameters: []ParameterSpec{{Name: "id", FieldProvenance: map[string]contract.FieldProvenance{
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
	noPropertyProvenance.Products[0].Tools[0].Parameters[0].FieldProvenance = map[string]contract.FieldProvenance{}
	if err := validateSchemaParameterBindingDelivery(schemaParameterBindingSnapshot{}, bound, noPropertyProvenance); err != nil {
		t.Fatalf("absent optional property provenance rejected: %v", err)
	}
}

func TestCrossPlatformCoverageSchemaParameterBindingHelpersAndLoaderErrors(t *testing.T) {
	if _, ok := finalSchemaParameterByName(ToolSpec{}, "missing"); ok {
		t.Fatal("missing parameter found")
	}
	selected := contract.FieldProvenance{Value: json.RawMessage(`"value"`), Source: "source"}
	if !schemaParameterProvenanceHasStringCandidate(selected, "source", "value") {
		t.Fatal("selected provenance not found")
	}
	if schemaParameterProvenanceHasStringCandidate(contract.FieldProvenance{Value: json.RawMessage(`{`), Source: "source"}, "source", "value") {
		t.Fatal("invalid selected provenance matched")
	}
	candidates := contract.FieldProvenance{Candidates: []contract.FieldCandidateProvenance{{Source: "other", Value: json.RawMessage(`"value"`)}, {Source: "source", Value: json.RawMessage(`{`)}, {Source: "source", Value: json.RawMessage(`"value"`)}}}
	if !schemaParameterProvenanceHasStringCandidate(candidates, "source", "value") || schemaParameterProvenanceHasStringCandidate(candidates, "missing", "value") {
		t.Fatal("candidate provenance lookup failed")
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

	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		return schemaParameterBindingSnapshot{}, errors.New("binding data failed")
	})
	if err := ValidateSchemaParameterBindings(); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("ValidateSchemaParameterBindings() error = %v", err)
	}
	if err := ValidateSchemaParameterBindingDelivery(BoundCommandRegistry{}, SchemaRegistry{}); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("ValidateSchemaParameterBindingDelivery() error = %v", err)
	}
	if _, err := LoadSchemaParameterBindings(); err == nil || !strings.Contains(err.Error(), "binding data failed") {
		t.Fatalf("LoadSchemaParameterBindings() error = %v", err)
	}
}
