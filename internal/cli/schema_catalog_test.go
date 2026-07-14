// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package cli

import (
	"sort"
	"strings"
	"testing"
)

func TestBuildSchemaCatalogSnapshotRejectsUnresolvedSource(t *testing.T) {
	_, err := BuildSchemaCatalogSnapshot(ResolvedSchemaBuild{}, SchemaCatalogBuildOptions{})
	if err == nil || !strings.Contains(err.Error(), "ResolveSchemaBuild") {
		t.Fatalf("BuildSchemaCatalogSnapshot() error = %v, want resolved-source requirement", err)
	}
}

func TestEmbeddedSchemaCatalogIntegrity(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	if !embeddedSchemaCatalogAvailable() {
		t.Fatal("embedded schema catalog is unavailable or failed integrity validation")
	}
	if got := schemaString(loaded.Snapshot.Catalog["source"]); got != "embedded-command-catalog" {
		t.Fatalf("catalog source = %q", got)
	}
}

func TestEmbeddedSchemaCatalogProgressiveQueries(t *testing.T) {
	overview, err := embeddedSchemaOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := schemaProductToolCount(map[string]any{"tools": overview["products"]}), len(embeddedSchemaCatalog().Registry.Products); got != want {
		t.Fatalf("compact product count = %d, want %d", got, want)
	}

	leaf, err := embeddedSchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["canonical_path"]); got != "calendar.create_calendar_event" {
		t.Fatalf("canonical path = %q", got)
	}
	if len(schemaMapSlice(leaf["parameters"])) != 0 {
		t.Fatal("parameters unexpectedly decoded as a list")
	}
	if parameters, ok := leaf["parameters"].(map[string]any); !ok || len(parameters) == 0 {
		t.Fatal("calendar.create_event parameters are empty")
	}

	group, err := embeddedSchemaPayload([]string{"calendar.event"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaProductToolCount(map[string]any{"tools": group["tools"]}) == 0 {
		t.Fatal("calendar.event group is empty")
	}

	alias, err := embeddedSchemaPayload([]string{"aitable record list"})
	if err != nil {
		t.Fatal(err)
	}
	if alias["is_alias"] != true || schemaString(alias["cli_path"]) != "aitable record list" {
		t.Fatalf("alias query did not preserve compatibility path: %#v", alias)
	}
	if schemaString(alias["canonical_path"]) != "aitable.query_records" {
		t.Fatalf("alias canonical path = %q", schemaString(alias["canonical_path"]))
	}
}

func TestEmbeddedSchemaAllPayloadContainsEveryFullLeaf(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	payload, err := embeddedSchemaAllPayload()
	if err != nil {
		t.Fatal(err)
	}

	expanded := 0
	parameterized := 0
	for _, product := range schemaMapSlice(payload["products"]) {
		for _, tool := range schemaMapSlice(product["tools"]) {
			canonical := schemaString(tool["canonical_path"])
			expected, ok := loaded.Snapshot.Tools[canonical]
			if !ok {
				t.Fatalf("full export contains unknown tool %q", canonical)
			}
			parameters, ok := tool["parameters"].(map[string]any)
			if !ok {
				t.Fatalf("full export tool %s has no parameters object", canonical)
			}
			if len(parameters) > 0 {
				parameterized++
			}
			if !schemaJSONEqual(tool, expected) {
				t.Fatalf("full export tool %s differs from stored leaf Schema", canonical)
			}
			expanded++
		}
	}
	if got, want := expanded, len(loaded.Snapshot.Tools); got != want {
		t.Fatalf("full export tools = %d, want %d", got, want)
	}
	if parameterized == 0 {
		t.Fatal("full export contains no parameterized tools")
	}
}

func TestEmbeddedCatalogPreservesRegistryIdentityAndManualParameterContract(t *testing.T) {
	leaf, err := embeddedSchemaPayload([]string{"chat category create-smart"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["source"]); got != "reviewed_command_registry" {
		t.Fatalf("source = %q, want reviewed_command_registry", got)
	}
	identity := schemaMap(leaf["field_provenance"])["canonical_path"]
	if identity["source"] != "reviewed_command_registry" || identity["precedence"] != "command_registry" {
		t.Fatalf("canonical identity provenance = %#v", identity)
	}
	parameters := schemaMap(leaf["parameters"])
	assertReviewedManual := func(flagName, field string) {
		t.Helper()
		provenance := schemaMap(parameters[flagName]["field_provenance"])
		winner := provenance[field]
		if winner["source"] != "reviewed_manual_hint" || winner["precedence"] != "reviewed_manual" {
			t.Fatalf("%s.%s provenance = %#v", flagName, field, winner)
		}
	}
	name := parameters["name"]
	if name["property"] != "categoryName" || name["required"] != true {
		t.Fatalf("name parameter = %#v", name)
	}
	assertReviewedManual("name", "property")
	assertReviewedManual("name", "required")
	for flagName, property := range map[string]string{
		"keywords": "groupNameKeywords",
		"members":  "memberOpenDingTalkIds",
	} {
		parameter := parameters[flagName]
		if parameter["property"] != property || parameter["interface_type"] != "array" || parameter["required"] != false {
			t.Fatalf("%s parameter = %#v", flagName, parameter)
		}
		for _, field := range []string{"property", "interface_type", "required"} {
			assertReviewedManual(flagName, field)
		}
	}
}

func TestEmbeddedCatalogModelsAitableExportBranches(t *testing.T) {
	leaf, err := embeddedSchemaPayload([]string{"aitable export data"})
	if err != nil {
		t.Fatal(err)
	}
	parameters := schemaMap(leaf["parameters"])
	if _, exists := parameters["format"]; exists {
		t.Fatal("business export format still shadows the global --format output flag")
	}
	exportFormat := parameters["export-format"]
	if exportFormat["property"] != "format" || exportFormat["required"] != false {
		t.Fatalf("export-format parameter = %#v", exportFormat)
	}
	if got, want := schemaStringSlice(exportFormat["enum"]), []string{"excel", "attachment", "excel_and_attachment", "excel_with_inline_images"}; !equalStringSlices(got, want) {
		t.Fatalf("export-format enum = %v, want %v", got, want)
	}
	if parameters["scope"]["required"] != false {
		t.Fatalf("scope must be conditional, got %#v", parameters["scope"])
	}
	if got := parameters["table-id"]["required_when"]; got != "scope is table or view" {
		t.Fatalf("table-id required_when = %#v", got)
	}
	if got := parameters["view-id"]["required_when"]; got != "scope is view" {
		t.Fatalf("view-id required_when = %#v", got)
	}

	constraints, ok := leaf["constraints"].(map[string]any)
	if !ok {
		t.Fatalf("constraints = %#v", leaf["constraints"])
	}
	hasGroup := func(field string, want ...string) bool {
		groups, _ := constraints[field].([]any)
		for _, raw := range groups {
			if equalStringSlices(schemaStringSlice(raw), want) {
				return true
			}
		}
		return false
	}
	if !hasGroup("require_one_of", "scope", "task-id") ||
		!hasGroup("require_one_of", "export-format", "task-id") ||
		!hasGroup("require_together", "scope", "export-format") {
		t.Fatalf("branch constraints = %#v", constraints)
	}
}

func TestEmbeddedCatalogKeepsSharedFlagSemanticsCommandScoped(t *testing.T) {
	queryLeaf, err := embeddedSchemaPayload([]string{"aitable record query"})
	if err != nil {
		t.Fatal(err)
	}
	getLeaf, err := embeddedSchemaPayload([]string{"aitable record get"})
	if err != nil {
		t.Fatal(err)
	}
	queryRecordIDs := schemaMap(queryLeaf["parameters"])["record-ids"]
	getRecordIDs := schemaMap(getLeaf["parameters"])["record-ids"]
	if queryRecordIDs["required"] != false {
		t.Fatalf("record query --record-ids = %#v, want optional", queryRecordIDs)
	}
	if getRecordIDs["required"] != true {
		t.Fatalf("record get --record-ids = %#v, want required", getRecordIDs)
	}
	getProvenance := schemaMap(getRecordIDs["field_provenance"])["required"]
	if getProvenance["source"] != "typed_parameter_metadata" {
		t.Fatalf("record get required provenance = %#v", getProvenance)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestStripSchemaPayloadCompactLeaf(t *testing.T) {
	leaf, err := embeddedSchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(leaf)

	// Must keep agent-essential fields.
	for _, key := range []string{"cli_path", "canonical_path", "description", "effect", "risk", "confirmation", "parameters", "constraints"} {
		if _, ok := stripped[key]; !ok {
			t.Fatalf("compact leaf missing essential key %q", key)
		}
	}

	// Must strip provenance / redundant fields.
	for _, key := range []string{"agent_metadata_source", "agent_source_refs", "agent_summary_source", "effect_source", "metadata_source", "primary_cli_path", "parameter_count", "has_parameters", "interface_ref", "source", "title", "display"} {
		if _, ok := stripped[key]; ok {
			t.Fatalf("compact leaf still contains stripped key %q", key)
		}
	}

	// Parameters must not contain interface_description / property.
	if params, ok := stripped["parameters"].(map[string]any); ok {
		for name, p := range params {
			if pm, ok := p.(map[string]any); ok {
				for _, stripped := range []string{"interface_description", "interface_type", "property"} {
					if _, present := pm[stripped]; present {
						t.Fatalf("compact param %q still contains %q", name, stripped)
					}
				}
				// Must keep type and required.
				if _, present := pm["type"]; !present {
					t.Fatalf("compact param %q missing type", name)
				}
			}
		}
	}
}

func TestStripSchemaPayloadCompactPreservesParameterIdentity(t *testing.T) {
	leaf, err := embeddedSchemaPayload([]string{"chat category create-smart"})
	if err != nil {
		t.Fatal(err)
	}
	full := schemaMap(leaf["parameters"])
	compact := schemaMap(stripSchemaPayloadCompact(leaf)["parameters"])
	if len(compact) != len(full) {
		t.Fatalf("compact parameter count = %d, want %d: full=%v compact=%v", len(compact), len(full), sortedSchemaKeys(full), sortedSchemaKeys(compact))
	}
	name := compact["name"]
	if name["required"] != true || name["type"] != "string" {
		t.Fatalf("compact --name parameter = %#v", name)
	}

	synthetic := map[string]any{"parameters": map[string]any{}}
	parameters := synthetic["parameters"].(map[string]any)
	for _, parameterName := range []string{"name", "path", "source", "title", "group", "aliases"} {
		parameters[parameterName] = map[string]any{"type": "string", "required": false, "field_provenance": map[string]any{"source": "test"}}
	}
	stripped := schemaMap(stripSchemaPayloadCompact(synthetic)["parameters"])
	for parameterName := range parameters {
		if _, ok := stripped[parameterName]; !ok {
			t.Errorf("compact projection dropped parameter identity %q", parameterName)
		}
	}
}

func sortedSchemaKeys(values map[string]map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestStripSchemaPayloadCompactOverview(t *testing.T) {
	overview, err := embeddedSchemaOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(overview)

	// Overview must keep kind/level/count/products.
	for _, key := range []string{"kind", "level", "count", "products"} {
		if _, ok := stripped[key]; !ok {
			t.Fatalf("compact overview missing key %q", key)
		}
	}
	// Must strip agent_metadata / interface_metadata at top level.
	for _, key := range []string{"agent_metadata", "interface_metadata", "source"} {
		if _, ok := stripped[key]; ok {
			t.Fatalf("compact overview still contains stripped key %q", key)
		}
	}
}

func TestStripSchemaPayloadCompactProduct(t *testing.T) {
	product, err := embeddedSchemaPayload([]string{"calendar"})
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(product)

	if _, ok := stripped["product"]; !ok {
		t.Fatal("compact product missing 'product' key")
	}
	prod := stripped["product"].(map[string]any)
	for _, key := range []string{"agent_metadata_source", "agent_source_refs", "source"} {
		if _, ok := prod[key]; ok {
			t.Fatalf("compact product still contains stripped key %q", key)
		}
	}
}
