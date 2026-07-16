// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommandRegistryJSONSchemaDocumentsClosedCommandSpec(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(embeddedSchemaCommandRegistrySchemaJSON, &schema); err != nil {
		t.Fatalf("decode schema_command_registry.schema.json: %v", err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["additionalProperties"] != false {
		t.Fatalf("registry root schema is not closed: %#v", schema)
	}
	definitions := schema["$defs"].(map[string]any)
	command := definitions["commandSpec"].(map[string]any)
	if command["additionalProperties"] != false {
		t.Fatalf("CommandSpec schema allows unknown fields: %#v", command)
	}
	properties := command["properties"].(map[string]any)
	for _, field := range []string{"canonical_path", "source_product_id", "cli_path", "aliases", "visibility"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("CommandSpec schema is missing %s", field)
		}
	}
	visibility := properties["visibility"].(map[string]any)
	if visibility["default"] != "public" || strings.Join(schemaStringSlice(visibility["enum"]), ",") != "public,compat,internal" {
		t.Fatalf("visibility contract = %#v", visibility)
	}

	var source map[string]any
	if err := json.Unmarshal(embeddedSchemaCommandRegistryJSON, &source); err != nil {
		t.Fatalf("decode schema_command_registry.json: %v", err)
	}
	if source["$schema"] != commandRegistrySchemaRef {
		t.Fatalf("registry source $schema = %#v, want %q", source["$schema"], commandRegistrySchemaRef)
	}
}

func TestDecodeCommandRegistryRejectsUnknownFieldsAtEveryLevel(t *testing.T) {
	valid := `{"$schema":"./schema_command_registry.schema.json","version":1,"products":[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample run"}]}]}`
	for name, input := range map[string]string{
		"root":    strings.Replace(valid, `"version":1`, `"version":1,"unknown":true`, 1),
		"product": strings.Replace(valid, `"id":"sample"`, `"id":"sample","unknown":true`, 1),
		"command": strings.Replace(valid, `"cli_path":"sample run"`, `"cli_path":"sample run","unknown":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCommandRegistry([]byte(input)); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("decodeCommandRegistry() error = %v, want unknown field", err)
			}
		})
	}
}

func TestDecodeCommandRegistryEnforcesReviewedSourceConstraints(t *testing.T) {
	wrap := func(products string) string {
		return `{"$schema":"./schema_command_registry.schema.json","version":1,"products":` + products + `}`
	}
	validTool := `{"canonical_path":"sample.run","cli_path":"sample run"}`
	tests := map[string]string{
		"missing schema ref":  `{"version":1,"products":[{"id":"sample","tools":[` + validTool + `]}]}`,
		"empty products":      wrap(`[]`),
		"invalid product":     wrap(`[{"id":"bad product","tools":[` + validTool + `]}]`),
		"duplicate product":   wrap(`[{"id":"sample","tools":[` + validTool + `]},{"id":"sample","tools":[{"canonical_path":"sample.other","cli_path":"sample other"}]}]`),
		"empty commands":      wrap(`[{"id":"sample","tools":[]}]`),
		"wrong product":       wrap(`[{"id":"sample","tools":[{"canonical_path":"other.run","cli_path":"sample run"}]}]`),
		"invalid source":      wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","source_product_id":"bad product","cli_path":"sample run"}]}]`),
		"empty source":        wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","source_product_id":"","cli_path":"sample run"}]}]`),
		"leading dws":         wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"dws sample run"}]}]`),
		"flag in path":        wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample run --yes"}]}]`),
		"repeated whitespace": wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample  run"}]}]`),
		"primary as alias":    wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample run","aliases":["sample run"]}]}]`),
		"duplicate alias":     wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample run","aliases":["sample execute","sample execute"]}]}]`),
		"invalid visibility":  wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample run","visibility":"hidden"}]}]`),
		"empty visibility":    wrap(`[{"id":"sample","tools":[{"canonical_path":"sample.run","cli_path":"sample run","visibility":""}]}]`),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCommandRegistry([]byte(input)); err == nil {
				t.Fatal("decodeCommandRegistry() unexpectedly accepted invalid reviewed source")
			}
		})
	}

	registry, err := decodeCommandRegistry([]byte(wrap(`[{"id":"sample","tools":[` + validTool + `]}]`)))
	if err != nil {
		t.Fatalf("decode valid registry: %v", err)
	}
	if got := registry.ByCanonical["sample.run"].Visibility; got != SchemaVisibilityPublic {
		t.Fatalf("default visibility = %q, want public", got)
	}
}

func TestCommandRegistryHashCoversEveryStableCommandField(t *testing.T) {
	baseline := CommandSpec{
		CanonicalPath:   "sample.run",
		SourceProductID: "implementation",
		PrimaryCLIPath:  "sample run",
		Aliases:         []string{"sample execute"},
		Visibility:      SchemaVisibilityPublic,
	}
	baselineHash := hashCommandSpecs([]CommandSpec{baseline})
	mutations := map[string]func(*CommandSpec){
		"canonical":      func(spec *CommandSpec) { spec.CanonicalPath = "sample.execute" },
		"source product": func(spec *CommandSpec) { spec.SourceProductID = "other_implementation" },
		"primary path":   func(spec *CommandSpec) { spec.PrimaryCLIPath = "sample start" },
		"aliases":        func(spec *CommandSpec) { spec.Aliases = []string{"sample legacy"} },
		"visibility":     func(spec *CommandSpec) { spec.Visibility = SchemaVisibilityCompat },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := cloneCommandSpec(baseline)
			mutate(&candidate)
			if got := hashCommandSpecs([]CommandSpec{candidate}); got == baselineHash {
				t.Fatalf("stable field mutation did not change CommandRegistry hash: %s", got)
			}
		})
	}
}

func TestNewCommandRegistryRejectsAliasAndCanonicalCollisions(t *testing.T) {
	tests := map[string][]CommandSpec{
		"duplicate alias": {{
			CanonicalPath:  "sample.run",
			PrimaryCLIPath: "sample run",
			Aliases:        []string{"sample execute", "sample execute"},
		}},
		"alias equals primary": {{
			CanonicalPath:  "sample.run",
			PrimaryCLIPath: "sample run",
			Aliases:        []string{"sample run"},
		}},
		"alias collides with canonical": {
			{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run", Aliases: []string{"other.execute"}},
			{CanonicalPath: "other.execute", PrimaryCLIPath: "other execute"},
		},
		"primary collides with canonical": {
			{CanonicalPath: "sample.run", PrimaryCLIPath: "other.execute"},
			{CanonicalPath: "other.execute", PrimaryCLIPath: "other execute"},
		},
	}
	for name, commands := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := newCommandRegistry(commands); err == nil {
				t.Fatal("newCommandRegistry() unexpectedly accepted a global identity/path collision")
			}
		})
	}
}

func TestMinutesFixedScopeLeavesRemainDistinctRegistryIdentities(t *testing.T) {
	registry, err := loadEmbeddedCommandRegistry()
	if err != nil {
		t.Fatalf("load embedded command registry: %v", err)
	}
	want := map[string]string{
		"minutes list all":    "minutes.list_accessible_minutes",
		"minutes list mine":   "minutes.list_by_keyword_and_time_range",
		"minutes list shared": "minutes.list_shared_minutes",
	}
	seen := map[string]bool{}
	for path, canonical := range want {
		spec, ok := registry.ByCLIPath[path]
		if !ok {
			t.Errorf("registry path %q is missing", path)
			continue
		}
		if spec.CanonicalPath != canonical {
			t.Errorf("registry path %q canonical = %q, want %q", path, spec.CanonicalPath, canonical)
		}
		if len(spec.Aliases) != 0 {
			t.Errorf("registry path %q aliases = %v, want none for fixed request scope", path, spec.Aliases)
		}
		seen[spec.CanonicalPath] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("minutes fixed-scope leaves collapsed to %d canonical identities, want %d", len(seen), len(want))
	}
}
