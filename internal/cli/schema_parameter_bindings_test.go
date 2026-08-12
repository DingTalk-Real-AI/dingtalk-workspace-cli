// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestSchemaParameterBindingsMatchReviewedBaselineAndDeliveryCatalog(t *testing.T) {
	if err := ValidateSchemaParameterBindings(); err != nil {
		t.Fatalf("ValidateSchemaParameterBindings() error = %v", err)
	}
	snapshot, err := runtimeSchemaParameterBindingData()
	if err != nil {
		t.Fatalf("runtimeSchemaParameterBindingData() error = %v", err)
	}
	if len(snapshot.Bindings) != 0 {
		t.Fatalf("active bindings = %d groups, want empty after Phase 2 retirement", len(snapshot.Bindings))
	}
	if len(snapshot.MappingExclusions) == 0 {
		t.Fatal("mapping exclusions ledger is empty")
	}
	if len(snapshot.Removals) == 0 {
		t.Fatal("removals ledger is empty")
	}

	loaded := mustDeliverySchemaCatalogMaps(t)
	for key := range snapshot.MappingExclusions {
		canonical, flagName, _ := strings.Cut(key, " --")
		detail, ok := loaded.Snapshot.Tools[canonical]
		if !ok {
			t.Errorf("mapping exclusion references unknown canonical path %q", canonical)
			continue
		}
		parameters, _ := detail["parameters"].(map[string]any)
		if parameters[flagName] == nil {
			t.Errorf("mapping exclusion %s references an unknown flag", key)
		}
	}
	for key := range snapshot.Removals {
		canonical, flagName, _ := strings.Cut(key, " --")
		if got := snapshot.Bindings[canonical][flagName]; got != "" {
			t.Errorf("reviewed removal %q remains active as %q", key, got)
		}
	}
}

func TestBindEffectiveCommandRegistryFailsClosedOnInvalidParameterBindingSource(t *testing.T) {
	testseam.Swap(t, &schemaParameterBindingData, func() (schemaParameterBindingSnapshot, error) {
		snapshot := schemaParameterBindingSnapshot{}
		return snapshot, validateSchemaParameterBindingSnapshot(snapshot)
	})
	effective, err := BuildEffectiveCommandRegistry(&cobra.Command{Use: "dws"})
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v, want identity build without binding audit", err)
	}
	_, err = BindEffectiveCommandRegistry(&cobra.Command{Use: "dws"}, effective)
	if err == nil || !strings.Contains(err.Error(), "validate reviewed Schema parameter bindings") || !strings.Contains(err.Error(), "mapping exclusions ledger must remain non-empty") {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v, want strict mapping-ledger validation", err)
	}
}

func TestValidateSchemaParameterBindingDeliveryRejectsStaleReviewedKeys(t *testing.T) {
	command := &cobra.Command{Use: "read"}
	command.Flags().String("item-id", "", "item id")
	boundSpec := BoundCommandSpec{
		CommandSpec: CommandSpec{
			CanonicalPath:  "sample.read",
			PrimaryCLIPath: "sample read",
			Visibility:     SchemaVisibilityPublic,
		},
		PrimaryCommand: command,
	}
	bound := BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundSpec},
		ByCanonical: map[string]BoundCommandSpec{"sample.read": boundSpec},
	}
	parameter := ParameterSpec{
		Name:     "item-id",
		Property: "itemId",
		FieldProvenance: map[string]contract.FieldProvenance{
			"property": {
				Value:      json.RawMessage(`"itemId"`),
				Source:     "versioned_parameter_binding",
				Resolution: "highest_precedence",
			},
		},
	}
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{
		Identity:   contract.ToolIdentitySpec{CanonicalPath: "sample.read"},
		Parameters: []ParameterSpec{parameter},
	}}}}}
	valid := schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{
		"sample.read": {"item-id": "itemId"},
	}}
	if err := validateSchemaParameterBindingDelivery(valid, bound, registry); err != nil {
		t.Fatalf("valid delivery rejected: %v", err)
	}

	tests := []struct {
		name     string
		snapshot schemaParameterBindingSnapshot
		want     string
	}{
		{
			name: "unknown active canonical",
			snapshot: schemaParameterBindingSnapshot{Bindings: map[string]map[string]string{
				"sample.missing": {"item-id": "itemId"},
			}},
			want: "does not reference a public bound command",
		},
		{
			name: "unknown mapping exclusion flag",
			snapshot: schemaParameterBindingSnapshot{MappingExclusions: map[string]string{
				"sample.read --missing": "reviewed local selector",
			}},
			want: "does not reference an exact final public Schema parameter",
		},
		{
			name: "unknown removal canonical",
			snapshot: schemaParameterBindingSnapshot{Removals: map[string]schemaParameterBindingRemoval{
				"sample.missing --item-id": {Reason: "reviewed removal", Reviewed: true},
			}},
			want: "has a stale canonical path",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSchemaParameterBindingDelivery(test.snapshot, bound, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateSchemaParameterBindingDelivery() error = %v, want %q", err, test.want)
			}
		})
	}
}
