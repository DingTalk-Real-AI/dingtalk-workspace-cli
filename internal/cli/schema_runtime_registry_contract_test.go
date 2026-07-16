// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"strings"
	"testing"
)

func TestValidateSchemaRegistryAgainstCommandRegistryChecksFullIdentity(t *testing.T) {
	tool := ToolSpec{Identity: ToolIdentitySpec{
		ProductID:       "sample",
		SourceProductID: "implementation_a",
		Name:            "run",
		CanonicalPath:   "sample.run",
		Path:            "sample.run",
		CLIPath:         "sample run",
		PrimaryCLIPath:  "sample run",
		Source:          "reviewed_command_registry",
	}}
	registry, err := SchemaRegistryFromRuntime("test", []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatal(err)
	}

	base := CommandSpec{
		CanonicalPath:   "sample.run",
		SourceProductID: "implementation_a",
		PrimaryCLIPath:  "sample run",
		Visibility:      SchemaVisibilityPublic,
		Source:          "reviewed_command_registry",
	}
	for name, test := range map[string]struct {
		mutate func(*CommandSpec)
		want   string
	}{
		"source product": {
			mutate: func(spec *CommandSpec) { spec.SourceProductID = "implementation_b" },
			want:   "source product",
		},
		"identity source": {
			mutate: func(spec *CommandSpec) { spec.Source = "reviewed_manual_hint" },
			want:   "identity source",
		},
	} {
		t.Run(name, func(t *testing.T) {
			expected := cloneCommandSpec(base)
			test.mutate(&expected)
			effective, err := newEffectiveCommandRegistry([]CommandSpec{expected})
			if err != nil {
				t.Fatal(err)
			}
			err = validateSchemaRegistryAgainstCommandRegistry(registry, effective)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateSchemaRegistryAgainstCommandRegistryRejectsAliasViewAsCanonical(t *testing.T) {
	baseTool := ToolSpec{Identity: ToolIdentitySpec{
		ProductID:      "sample",
		Name:           "run",
		CanonicalPath:  "sample.run",
		Path:           "sample.run",
		CLIPath:        "sample run",
		PrimaryCLIPath: "sample run",
		Aliases:        []string{"sample execute"},
		Source:         "reviewed_command_registry",
	}}
	effective, err := newEffectiveCommandRegistry([]CommandSpec{{
		CanonicalPath:  "sample.run",
		PrimaryCLIPath: "sample run",
		Aliases:        []string{"sample execute"},
		Visibility:     SchemaVisibilityPublic,
		Source:         "reviewed_command_registry",
	}})
	if err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		mutate func(*ToolIdentitySpec)
		want   string
	}{
		"alternate cli path": {
			mutate: func(identity *ToolIdentitySpec) { identity.CLIPath = "sample execute" },
			want:   "must equal primary_cli_path",
		},
		"alias marker": {
			mutate: func(identity *ToolIdentitySpec) { identity.IsAlias = true },
			want:   "must have is_alias=false",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tool := baseTool
			test.mutate(&tool.Identity)
			registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}}}
			err := validateSchemaRegistryAgainstCommandRegistry(registry, effective)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}
