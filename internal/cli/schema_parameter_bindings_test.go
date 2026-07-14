// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const schemaParameterBindingInvalidBuildChildEnv = "DWS_SCHEMA_PARAMETER_BINDING_INVALID_BUILD_CHILD"

func TestSchemaParameterBindingsMatchReviewedBaselineAndEmbeddedCatalog(t *testing.T) {
	if err := ValidateEmbeddedSchemaParameterBindings(); err != nil {
		t.Fatalf("ValidateEmbeddedSchemaParameterBindings() error = %v", err)
	}
	snapshot, err := runtimeSchemaParameterBindingData()
	if err != nil {
		t.Fatalf("runtimeSchemaParameterBindingData() error = %v", err)
	}
	manifestHash, err := schemaParameterBindingManifestHash(snapshot.Bindings)
	if err != nil {
		t.Fatalf("schemaParameterBindingManifestHash() error = %v", err)
	}
	if manifestHash != snapshot.Baseline.SHA256 {
		t.Fatalf("active binding manifest hash = %q, reviewed baseline = %q", manifestHash, snapshot.Baseline.SHA256)
	}

	loaded := embeddedSchemaCatalog()
	for canonical, bindings := range snapshot.Bindings {
		detail, ok := loaded.Snapshot.Tools[canonical]
		if !ok {
			t.Errorf("binding references unknown canonical path %q", canonical)
			continue
		}
		parameters, _ := detail["parameters"].(map[string]any)
		for flagName, propertyName := range bindings {
			parameter, _ := parameters[flagName].(map[string]any)
			if parameter == nil {
				t.Errorf("binding %s --%s references an unknown flag", canonical, flagName)
				continue
			}
			if got := schemaString(parameter["property"]); got != propertyName {
				t.Errorf("binding %s --%s property = %q, want %q", canonical, flagName, got, propertyName)
			}
		}
	}

	for key, correction := range snapshot.Corrections {
		canonical, flagName, _ := strings.Cut(key, " --")
		if got := snapshot.Bindings[canonical][flagName]; got != correction.NewProperty {
			t.Errorf("reviewed correction %q delivers %q, want %q", key, got, correction.NewProperty)
		}
	}
	for key, removal := range snapshot.Removals {
		canonical, flagName, _ := strings.Cut(key, " --")
		if got := snapshot.Bindings[canonical][flagName]; got != "" {
			t.Errorf("reviewed removal %q remains active as %q", key, got)
		}
		if removal.ReplacedBy != "" {
			replacementCanonical, replacementFlag, _ := strings.Cut(removal.ReplacedBy, " --")
			if got := snapshot.Bindings[replacementCanonical][replacementFlag]; got == "" {
				t.Errorf("reviewed removal %q replacement %q is not active", key, removal.ReplacedBy)
			}
		}
	}

	if got := snapshot.Bindings["calendar.get_calendar"]["id"]; got != "calendarId" {
		t.Fatalf("calendar.get_calendar --id property = %q, want calendarId", got)
	}
	if got := snapshot.Bindings["aitable.field_update"]["name"]; got != "newFieldName" {
		t.Fatalf("aitable.field_update --name property = %q, want newFieldName", got)
	}
}

func TestDecodeSchemaParameterBindingsFailsClosed(t *testing.T) {
	valid := append([]byte(nil), embeddedSchemaParameterBindingsJSON...)
	replaceOnce := func(old, replacement string) []byte {
		t.Helper()
		updated := bytes.Replace(valid, []byte(old), []byte(replacement), 1)
		if bytes.Equal(updated, valid) {
			t.Fatalf("fixture does not contain %q", old)
		}
		return updated
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "unknown top-level field",
			data: replaceOnce(`"version": 3,`, `"version": 3, "unexpected": true,`),
			want: "unknown field",
		},
		{
			name: "unknown baseline field",
			data: replaceOnce(`"manifest": "schema-parameter-bindings-v3",`, `"manifest": "schema-parameter-bindings-v3", "count": 653,`),
			want: "unknown field",
		},
		{
			name: "multiple JSON values",
			data: append(append([]byte(nil), valid...), []byte("\n{}")...),
			want: "multiple JSON values",
		},
		{
			name: "unsupported version",
			data: replaceOnce(`"version": 3`, `"version": 999`),
			want: "unsupported schema parameter bindings version",
		},
		{
			name: "unreviewed baseline",
			data: replaceOnce(`"reviewed": true`, `"reviewed": false`),
			want: "baseline must be reviewed",
		},
		{
			name: "active binding drift",
			data: replaceOnce(`"types": "searchTypes"`, `"types": "changedSearchTypes"`),
			want: "want exact active manifest",
		},
		{
			name: "legacy count ledger",
			data: replaceOnce(`"version": 3,`, `"version": 3, "historical_binding_count": 653,`),
			want: "unknown field",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := decodeSchemaParameterBindings(test.data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decodeSchemaParameterBindings() snapshot=%#v error=%v, want %q", snapshot, err, test.want)
			}
			if len(snapshot.Bindings) != 0 {
				t.Fatalf("failed decode returned %d active binding groups; want no inference-capable fallback", len(snapshot.Bindings))
			}
		})
	}
}

func TestSchemaParameterBindingManifestHashIsExactContentNotCount(t *testing.T) {
	left := map[string]map[string]string{
		"sample.read": {"item-id": "itemId", "limit": "pageSize"},
	}
	right := map[string]map[string]string{
		"sample.read": {"limit": "pageSize", "item-id": "itemId"},
	}
	changed := map[string]map[string]string{
		"sample.read": {"item-id": "id", "limit": "pageSize"},
	}
	leftHash, err := schemaParameterBindingManifestHash(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := schemaParameterBindingManifestHash(right)
	if err != nil {
		t.Fatal(err)
	}
	changedHash, err := schemaParameterBindingManifestHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash != rightHash {
		t.Fatalf("manifest hash depends on map iteration order: %q != %q", leftHash, rightHash)
	}
	if leftHash == changedHash {
		t.Fatalf("same-size manifest property change did not change hash: %q", leftHash)
	}
}

func TestBuildEffectiveCommandRegistryFailsClosedOnInvalidParameterBindingSource(t *testing.T) {
	if os.Getenv(schemaParameterBindingInvalidBuildChildEnv) == "1" {
		if got := runtimeSchemaParameterBindingsLazyLoadCount.Load(); got != 0 {
			t.Fatalf("parameter bindings loaded before child test: %d", got)
		}
		embeddedSchemaParameterBindingsJSON = []byte(`{"version":3,"unexpected":true}`)
		_, err := BuildEffectiveCommandRegistry(&cobra.Command{Use: "dws"})
		if err == nil || !strings.Contains(err.Error(), "validate reviewed Schema parameter bindings") || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("BuildEffectiveCommandRegistry() error = %v, want strict binding validation", err)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestBuildEffectiveCommandRegistryFailsClosedOnInvalidParameterBindingSource$", "-test.count=1")
	command.Env = append(os.Environ(), schemaParameterBindingInvalidBuildChildEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("invalid binding Build child failed: %v\n%s", err, strings.TrimSpace(string(output)))
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
		FieldProvenance: map[string]FieldProvenance{
			"property": {
				Value:      json.RawMessage(`"itemId"`),
				Source:     "versioned_parameter_binding",
				Resolution: "highest_precedence",
			},
		},
	}
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{
		Identity:   ToolIdentitySpec{CanonicalPath: "sample.read"},
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
