// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateSchemaDeliveryInvariantsAcceptsOneTypedSource(t *testing.T) {
	fixture := schemaDeliveryTestTool{
		Canonical: "sample.run",
		CLIPath:   "sample category run",
		Aliases:   []string{"sample legacy execute"},
	}
	source := schemaDeliveryTestRegistry(fixture)
	snapshot := schemaDeliveryTestSnapshot(fixture)
	if err := ValidateSchemaDeliveryInvariants(source, snapshot); err != nil {
		t.Fatalf("ValidateSchemaDeliveryInvariants(): %v", err)
	}
}

func TestValidateSchemaDeliveryInvariantsRejectsConsistentSerializerMutation(t *testing.T) {
	fixture := schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"}
	source := schemaDeliveryTestRegistry(fixture)
	snapshot := schemaDeliveryTestSnapshot(fixture)

	// Mutate both wire projections together. Snapshot-only checks cannot detect
	// this because the summary, full tool, queries, and --all remain mutually
	// consistent; only the typed source proves that the value changed.
	snapshot.Tools[fixture.Canonical]["risk"] = "high"
	mutatedProvenance := schemaDeliveryTestProvenance(map[string]any{"risk": "high"})["risk"]
	fullProvenance := snapshot.Tools[fixture.Canonical]["field_provenance"].(map[string]any)
	encodedProvenance, err := typedJSONValue(mutatedProvenance)
	if err != nil {
		t.Fatal(err)
	}
	fullProvenance["risk"] = encodedProvenance
	products := snapshot.Catalog["products"].([]map[string]any)
	summaries := products[0]["tools"].([]map[string]any)
	summaries[0]["risk"] = "high"
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	if err := validateSchemaSnapshotDeliveryInvariants(snapshot); err != nil {
		t.Fatalf("snapshot should remain internally consistent: %v", err)
	}
	err = ValidateSchemaDeliveryInvariants(source, snapshot)
	if err == nil || !strings.Contains(err.Error(), "differs from complete source SchemaRegistry") {
		t.Fatalf("ValidateSchemaDeliveryInvariants() error = %v, want source mutation rejection", err)
	}
}

func TestValidateSchemaDeliveryInvariantsRejectsSelectionProvenanceOmission(t *testing.T) {
	fixture := schemaDeliveryTestTool{
		Canonical: "sample.run",
		CLIPath:   "sample run",
		Selection: SelectionSpec{Examples: []string{"dws sample run"}},
	}
	source := schemaDeliveryTestRegistry(fixture)
	snapshot := schemaDeliveryTestSnapshot(fixture)

	// Examples are leaf-only on the wire, but once selected they participate in
	// precedence. Dropping both the value and provenance must therefore fail at
	// the production snapshot boundary rather than surviving until source diff.
	full := snapshot.Tools[fixture.Canonical]
	delete(full, "examples")
	provenance := full["field_provenance"].(map[string]any)
	delete(provenance, "examples")
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	if err := validateSchemaSnapshotDeliveryInvariants(snapshot); err != nil {
		t.Fatalf("snapshot-only relationship validation should not apply production coverage: %v", err)
	}
	err := ValidateSchemaDeliveryInvariants(source, snapshot)
	if err == nil || !strings.Contains(err.Error(), "differs from complete source SchemaRegistry") {
		t.Fatalf("ValidateSchemaDeliveryInvariants() error = %v, want source selection omission rejection", err)
	}
}

func TestProductionSnapshotRejectsProductSelectionProvenanceOmission(t *testing.T) {
	registry := schemaDeliveryTestRegistry(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	product := &registry.Products[0]
	product.Selection.AgentSummary = "Sample operations"
	product.Selection.UseWhen = []string{"manage a sample"}
	product.Selection.AvoidWhen = []string{"manage another product"}
	product.FieldProvenance = schemaDeliveryTestProvenance(map[string]any{
		"agent_summary": product.Selection.AgentSummary,
		"use_when":      product.Selection.UseWhen,
		"avoid_when":    product.Selection.AvoidWhen,
	})
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := SchemaCatalogSnapshot{Version: SchemaCatalogSnapshotVersion, Catalog: payload.Catalog, Tools: payload.Tools}
	products := snapshot.Catalog["products"].([]map[string]any)
	provenance := products[0]["field_provenance"].(map[string]any)
	delete(provenance, "agent_summary")
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	_, err = loadSchemaCatalogSnapshot(snapshot)
	if err == nil || !strings.Contains(err.Error(), "Schema product sample has no provenance for agent_summary") {
		t.Fatalf("loadSchemaCatalogSnapshot() error = %v", err)
	}
}

func TestSelectionExplicitEmptyListSurvivesFinalDelivery(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.run",
		CLIPath:   "sample category run",
		Aliases:   []string{"sample legacy execute"},
		Selection: SelectionSpec{UseWhen: []string{}},
	})

	full, ok := snapshot.Tools["sample.run"]["use_when"].([]string)
	if !ok || full == nil || len(full) != 0 {
		t.Fatalf("generated ToolSpec use_when = %#v, want explicit []", snapshot.Tools["sample.run"]["use_when"])
	}
	if err := validateSchemaSnapshotDeliveryInvariants(snapshot); err != nil {
		t.Fatalf("validateSchemaSnapshotDeliveryInvariants(): %v", err)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := decodeSchemaCatalogSnapshot(encoded)
	if err != nil {
		t.Fatalf("decodeSchemaCatalogSnapshot(): %v", err)
	}
	canonical, err := schemaPayloadFromLoadedCatalog(loaded, []string{"sample.run"})
	if err != nil {
		t.Fatalf("canonical query: %v", err)
	}
	alias, err := schemaPayloadFromLoadedCatalog(loaded, []string{"sample legacy execute"})
	if err != nil {
		t.Fatalf("alias query: %v", err)
	}
	for name, payload := range map[string]map[string]any{"canonical": canonical, "alias": alias} {
		value, ok := payload["use_when"].([]string)
		if !ok || value == nil || len(value) != 0 {
			t.Fatalf("%s delivered use_when = %#v, want explicit []", name, payload["use_when"])
		}
		provenance := payload["field_provenance"].(map[string]any)["use_when"].(map[string]any)
		winner, ok := provenance["value"].([]any)
		if !ok || winner == nil || len(winner) != 0 {
			t.Fatalf("%s provenance winner = %#v, want JSON []", name, provenance["value"])
		}
	}
	if problem := schemaAliasViewProblem(canonical, alias, "sample legacy execute"); problem != "" {
		t.Fatalf("alias explicit-empty projection: %s", problem)
	}
}

func TestCompatibleSchemaAliasSeparatorsSurviveFinalDelivery(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.run",
		CLIPath:   "sample category run",
		Aliases:   []string{"sample legacy execute"},
	})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := decodeSchemaCatalogSnapshot(encoded)
	if err != nil {
		t.Fatalf("decodeSchemaCatalogSnapshot(): %v", err)
	}
	canonical, err := schemaPayloadFromLoadedCatalog(loaded, []string{"sample.run"})
	if err != nil {
		t.Fatalf("canonical query: %v", err)
	}
	for _, path := range []string{"sample legacy execute", "sample.legacy.execute", "sample/legacy/execute"} {
		alias, aliasErr := schemaPayloadFromLoadedCatalog(loaded, []string{path})
		if aliasErr != nil {
			t.Fatalf("alias query %q: %v", path, aliasErr)
		}
		if problem := schemaAliasViewProblem(canonical, alias, "sample legacy execute"); problem != "" {
			t.Fatalf("alias projection for %q: %s", path, problem)
		}
	}
}

func TestValidateSchemaDeliveryInvariantsAllowsOnlyEnvelopeHashes(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	snapshot.SurfaceHash = "sha256:reviewed-command-registry"
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	if err := validateSchemaSnapshotDeliveryInvariants(snapshot); err != nil {
		t.Fatalf("validateSchemaSnapshotDeliveryInvariants(): %v", err)
	}
}

func TestSchemaOverviewPayloadFromCatalogPreservesListSelectionPriority(t *testing.T) {
	catalog := map[string]any{
		"kind":   "schema",
		"source": "embedded-command-catalog",
		"products": []map[string]any{
			{
				"id":            "agent-summary",
				"agent_summary": "reviewed summary",
				"use_when":      []string{"lower-priority use_when"},
				"description":   "lower-priority description",
				"tools":         []map[string]any{{"canonical_path": "agent-summary.run"}},
			},
			{
				"id":          "use-when",
				"use_when":    []any{"first use", "second use"},
				"description": "lower-priority description",
				"tools":       []any{},
			},
			{
				"id":          "description",
				"description": "product description",
				"tools":       []map[string]any{{}, {}},
			},
		},
		"interface_metadata": map[string]any{"coverage": "complete"},
		"agent_metadata":     map[string]any{"coverage": "reviewed"},
	}

	got := schemaOverviewPayloadFromCatalog(catalog)
	wantProducts := []map[string]any{
		{"id": "agent-summary", "schema_path": "agent-summary", "tool_count": 1, "agent_summary": "reviewed summary"},
		{"id": "use-when", "schema_path": "use-when", "tool_count": 0, "use_when": []string{"first use"}},
		{"id": "description", "schema_path": "description", "tool_count": 2, "description": "product description"},
	}
	if !schemaJSONEqual(got["products"], wantProducts) {
		t.Fatalf("overview products = %#v, want %#v", got["products"], wantProducts)
	}
	if got["count"] != 3 || got["tool_count"] != 3 || got["source"] != "embedded-command-catalog" {
		t.Fatalf("overview envelope = %#v", got)
	}
	if !schemaJSONEqual(got["interface_metadata"], catalog["interface_metadata"]) || !schemaJSONEqual(got["agent_metadata"], catalog["agent_metadata"]) {
		t.Fatalf("overview metadata = %#v", got)
	}
}

func TestProductionSnapshotLoaderRejectsUnknownEnvelopeField(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded[:len(encoded)-1], []byte(`,"unexpected":true}`)...)

	if _, err := decodeSchemaCatalogSnapshot(encoded); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("decodeSchemaCatalogSnapshot() error = %v, want unknown envelope field", err)
	}
}

func TestProductionSnapshotLoaderRejectsTrailingJSON(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, []byte(` {}`)...)

	if _, err := decodeSchemaCatalogSnapshot(encoded); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("decodeSchemaCatalogSnapshot() error = %v, want trailing JSON rejection", err)
	}
}

func TestProductionSnapshotLoaderRejectsUnknownCatalogField(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	snapshot.Catalog["unexpected"] = true
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want unknown Catalog field", err)
	}
}

func TestProductionSnapshotLoaderRejectsCatalogTopLevelDrift(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	snapshot.Catalog["level"] = ""
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "changed complete Catalog content") {
		t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want complete Catalog drift rejection", err)
	}
}

func TestProductionSnapshotLoaderRejectsAliasViewAsCanonicalTool(t *testing.T) {
	for name, test := range map[string]struct {
		mutate func(map[string]any)
		want   string
	}{
		"alternate cli path": {
			mutate: func(tool map[string]any) { tool["cli_path"] = "sample execute" },
			want:   "must equal primary_cli_path",
		},
		"alias marker": {
			mutate: func(tool map[string]any) { tool["is_alias"] = true },
			want:   "must have is_alias=false",
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
				Canonical: "sample.run",
				CLIPath:   "sample run",
				Aliases:   []string{"sample execute"},
			})
			test.mutate(snapshot.Tools["sample.run"])
			snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

			_, err := loadSchemaCatalogSnapshot(snapshot)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateSchemaDeliveryInvariantsRejectsCatalogSummaryContentDrift(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(
		schemaDeliveryTestTool{Canonical: "sample.one", CLIPath: "sample one"},
		schemaDeliveryTestTool{Canonical: "sample.two", CLIPath: "sample two"},
	)
	products := snapshot.Catalog["products"].([]map[string]any)
	tools := products[0]["tools"].([]map[string]any)
	// Preserve every aggregate count and canonical set. A count gate would pass.
	tools[0]["title"] = "count-preserving summary drift"
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	err := validateSchemaSnapshotDeliveryInvariants(snapshot)
	if err == nil || (!strings.Contains(err.Error(), "changed product/tool summaries") && !strings.Contains(err.Error(), "differs from final ToolSpec summary")) {
		t.Fatalf("ValidateSchemaDeliveryInvariants() error = %v, want content drift", err)
	}
}

func TestValidateSchemaDeliveryInvariantsRejectsCatalogFullToolContentDrift(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	// Keep the summary, tool count and canonical set unchanged while changing the
	// full delivery contract.
	snapshot.Tools["sample.run"]["title"] = "full contract drift"
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	err := validateSchemaSnapshotDeliveryInvariants(snapshot)
	if err == nil || (!strings.Contains(err.Error(), "changed product/tool summaries") && !strings.Contains(err.Error(), "differs from final ToolSpec")) {
		t.Fatalf("ValidateSchemaDeliveryInvariants() error = %v, want full tool drift", err)
	}
}

func TestSchemaAliasViewProblemAllowsOnlyPathAndAliasMarker(t *testing.T) {
	canonical := map[string]any{
		"canonical_path": "sample.run",
		"cli_path":       "sample run",
		"is_alias":       false,
		"title":          "Run sample",
		"parameters": map[string]any{
			"name": map[string]any{"type": "string", "required": true},
		},
	}
	alias := map[string]any{
		"canonical_path": "sample.run",
		"cli_path":       "sample execute",
		"is_alias":       true,
		"title":          "Run sample",
		"parameters": map[string]any{
			"name": map[string]any{"type": "string", "required": true},
		},
	}
	if problem := schemaAliasViewProblem(canonical, alias, "sample execute"); problem != "" {
		t.Fatalf("valid alias projection: %s", problem)
	}

	alias["title"] = "alias-specific contract"
	if problem := schemaAliasViewProblem(canonical, alias, "sample execute"); !strings.Contains(problem, "other than cli_path/is_alias") {
		t.Fatalf("semantic alias drift problem = %q", problem)
	}
	alias["title"] = canonical["title"]
	alias["is_alias"] = false
	if problem := schemaAliasViewProblem(canonical, alias, "sample execute"); !strings.Contains(problem, "is_alias=true") {
		t.Fatalf("alias marker problem = %q", problem)
	}
}

func TestSchemaDeliveryToolsByCanonicalRejectsDuplicateWithoutRelyingOnCount(t *testing.T) {
	payload := map[string]any{
		"products": []map[string]any{{
			"tools": []map[string]any{
				{"canonical_path": "sample.same", "title": "first"},
				{"canonical_path": "sample.same", "title": "second"},
			},
		}},
	}
	tools, problems := schemaDeliveryToolsByCanonical(payload, "test view")
	if len(tools) != 1 || len(problems) != 1 || !strings.Contains(problems[0], "duplicate tool sample.same") {
		t.Fatalf("tools=%v problems=%v", tools, problems)
	}
}
