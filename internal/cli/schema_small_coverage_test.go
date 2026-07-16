// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSchemaCommandRemainingBranches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"cli path conflict", []string{"sample", "--cli-path", "sample get"}, "mutually exclusive"},
		{"list alias", []string{"list"}, ""},
		{"all path conflict", []string{"sample", "--all"}, "--all cannot"},
		{"cli path", []string{"--cli-path", "aisearch person"}, ""},
		{"all", []string{"--all"}, ""},
		{"overview", nil, ""},
		{"missing", []string{"definitely-missing-schema-path"}, ""},
		{"compact", []string{"--compact"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewSchemaCommand(nil)
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.name == "missing" {
				if err == nil {
					t.Fatal("missing schema path succeeded")
				}
				return
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || output.Len() == 0 {
				t.Fatalf("schema output length = %d, error = %v", output.Len(), err)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaCommandCatalogFailure(t *testing.T) {
	original := schemaCommandCatalogError
	t.Cleanup(func() { schemaCommandCatalogError = original })
	schemaCommandCatalogError = func() error { return errors.New("catalog failed") }
	cmd := NewSchemaCommand(nil)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "catalog failed") {
		t.Fatalf("catalog failure error = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaSupportParameterAndIdentityEdges(t *testing.T) {
	if hasRuntimeSchemaCommand(nil) {
		t.Fatal("nil command has runtime schema")
	}
	root := &cobra.Command{Use: "root"}
	help := &cobra.Command{Use: "help", Run: func(*cobra.Command, []string) {}}
	hidden := &cobra.Command{Use: "hidden", Hidden: true}
	leaf := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(leaf, "sample", "get", "fixture")
	hidden.AddCommand(leaf)
	root.AddCommand(help, hidden)
	var visited []*cobra.Command
	walkLeafCommands(root, func(command *cobra.Command) { visited = append(visited, command) })
	if len(visited) != 1 || visited[0] != leaf {
		t.Fatalf("visited leaves = %#v", visited)
	}

	original := runtimeSchemaParameterMetadataByCanonical
	t.Cleanup(func() { runtimeSchemaParameterMetadataByCanonical = original })
	runtimeSchemaParameterMetadataByCanonical = map[string]RuntimeSchemaParameterMetadata{}
	applyRuntimeSchemaParameterMetadata(leaf, "missing")

	provenance := commandRegistryIdentityProvenance(BoundCommandSpec{
		CommandSpec:   CommandSpec{CanonicalPath: "sample.get", PrimaryCLIPath: "sample get", Source: "reviewed_manual_hint"},
		AliasCommands: []BoundAlias{{Path: "sample alias", Command: nil}},
	})
	if provenance.Precedence != "reviewed_manual" || len(provenance.Candidates) != 1 {
		t.Fatalf("identity provenance = %#v", provenance)
	}
}

func TestCrossPlatformCoverageAgentMetadataRemainingPureEdges(t *testing.T) {
	provenance := resolvedFieldProvenance(make(chan int), " source ", " ref ", " precedence ", " resolution ", " reason ")
	if string(provenance.Value) != "null" || len(provenance.Candidates) != 1 {
		t.Fatalf("fallback provenance = %#v", provenance)
	}
}

func TestCrossPlatformCoverageEmbeddedAgentMetadataLoaderEdges(t *testing.T) {
	for _, source := range []fs.FS{
		fstest.MapFS{},
		fstest.MapFS{"schema_agent_metadata/index.json": {Data: []byte("{")}},
		fstest.MapFS{"schema_agent_metadata/index.json": {Data: []byte(`{"domains":[" "]}`)}},
		fstest.MapFS{"schema_agent_metadata/index.json": {Data: []byte(`{"domains":["bad/path"]}`)}},
		fstest.MapFS{"schema_agent_metadata/index.json": {Data: []byte(`{"domains":["sample"]}`)}},
		fstest.MapFS{
			"schema_agent_metadata/index.json":  {Data: []byte(`{"domains":["sample"]}`)},
			"schema_agent_metadata/sample.json": {Data: []byte("{")},
		},
		fstest.MapFS{
			"schema_agent_metadata/index.json":  {Data: []byte(`{"domains":["sample"]}`)},
			"schema_agent_metadata/sample.json": {Data: []byte(`{"product_id":"other"}`)},
		},
	} {
		metadata := loadEmbeddedAgentMetadataFrom(source)
		if metadata.Products == nil || metadata.Tools == nil || len(metadata.Tools) != 0 {
			t.Fatalf("empty fallback metadata = %#v", metadata)
		}
	}
	valid := fstest.MapFS{
		"schema_agent_metadata/index.json":  {Data: []byte(`{"domains":["sample"]}`)},
		"schema_agent_metadata/sample.json": {Data: []byte(`{"product_id":"sample","tools":{"sample.get":{}}}`)},
	}
	metadata := loadEmbeddedAgentMetadataFrom(valid)
	if metadata.Products == nil || len(metadata.Tools) != 1 {
		t.Fatalf("loaded agent metadata = %#v", metadata)
	}
}

func TestCrossPlatformCoverageManualAgentSelectionRemainingEdges(t *testing.T) {
	baseBound := manualAgentSelectionBoundFixture()
	baseHints := manualAgentHintSetFixture()
	for _, tc := range []struct {
		name   string
		mutate func(*BoundCommandRegistry, *ManualAgentHintSet)
	}{
		{"invalid bound canonical", func(bound *BoundCommandRegistry, hints *ManualAgentHintSet) {
			item := bound.Commands[0]
			item.CanonicalPath = "invalid"
			bound.Commands[0] = item
		}},
		{"missing by canonical", func(bound *BoundCommandRegistry, hints *ManualAgentHintSet) {
			delete(bound.ByCanonical, "sample.search_items")
		}},
		{"missing use when", func(_ *BoundCommandRegistry, hints *ManualAgentHintSet) {
			item := hints.Tools["sample.search_items"]
			item.UseWhen = nil
			hints.Tools["sample.search_items"] = item
		}},
		{"missing avoid when", func(_ *BoundCommandRegistry, hints *ManualAgentHintSet) {
			item := hints.Tools["sample.search_items"]
			item.AvoidWhen = nil
			hints.Tools["sample.search_items"] = item
		}},
		{"blank use when", func(_ *BoundCommandRegistry, hints *ManualAgentHintSet) {
			item := hints.Tools["sample.search_items"]
			item.UseWhen = []string{" "}
			hints.Tools["sample.search_items"] = item
		}},
		{"blank avoid when", func(_ *BoundCommandRegistry, hints *ManualAgentHintSet) {
			item := hints.Tools["sample.search_items"]
			item.AvoidWhen = []string{" "}
			hints.Tools["sample.search_items"] = item
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bound, hints := baseBound, baseHints
			bound.Commands = append([]BoundCommandSpec(nil), baseBound.Commands...)
			bound.ByCanonical = cloneBoundByCanonical(baseBound.ByCanonical)
			bound.ByCLIPath = cloneBoundByCanonical(baseBound.ByCLIPath)
			hints.Tools = cloneManualAgentTools(baseHints.Tools)
			tc.mutate(&bound, &hints)
			if _, _, err := BuildManualAgentSelectionEvalFixture(bound, hints); err == nil {
				t.Fatal("invalid selection fixture succeeded")
			}
		})
	}

	canonical := "sample.search_items"
	boundItem := baseBound.ByCanonical[canonical]
	for _, tc := range []struct {
		name   string
		mutate func(*BoundCommandRegistry, *BoundCommandSpec)
	}{
		{"mismatch", func(_ *BoundCommandRegistry, item *BoundCommandSpec) { item.CanonicalPath = "other.tool" }},
		{"nil command", func(_ *BoundCommandRegistry, item *BoundCommandSpec) { item.PrimaryCommand = nil }},
		{"non runnable", func(_ *BoundCommandRegistry, item *BoundCommandSpec) {
			item.PrimaryCommand = &cobra.Command{Use: "leaf"}
		}},
		{"empty path", func(_ *BoundCommandRegistry, item *BoundCommandSpec) { item.PrimaryCLIPath = " " }},
		{"path mismatch", func(bound *BoundCommandRegistry, _ *BoundCommandSpec) { delete(bound.ByCLIPath, "sample item search") }},
	} {
		t.Run("binding "+tc.name, func(t *testing.T) {
			bound := baseBound
			bound.ByCLIPath = cloneBoundByCanonical(baseBound.ByCLIPath)
			item := boundItem
			tc.mutate(&bound, &item)
			if err := validateManualAgentSelectionBinding(bound, canonical, item); err == nil {
				t.Fatal("invalid binding succeeded")
			}
		})
	}

	originalMarshal := marshalManualAgentSelectionFixture
	t.Cleanup(func() { marshalManualAgentSelectionFixture = originalMarshal })
	marshalManualAgentSelectionFixture = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	if _, err := manualAgentSelectionFixtureDigest(ManualAgentSelectionFixture{}); err == nil {
		t.Fatal("selection digest marshal failure succeeded")
	}
	if _, _, err := BuildManualAgentSelectionEvalFixture(baseBound, baseHints); err == nil {
		t.Fatal("selection fixture ignored digest failure")
	}
	marshalManualAgentSelectionFixture = json.Marshal
}

func cloneBoundByCanonical(source map[string]BoundCommandSpec) map[string]BoundCommandSpec {
	out := make(map[string]BoundCommandSpec, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func cloneManualAgentTools(source map[string]ManualAgentToolHint) map[string]ManualAgentToolHint {
	out := make(map[string]ManualAgentToolHint, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func TestCrossPlatformCoverageHintDirectoryLoaderRemainingEdges(t *testing.T) {
	directoryJSON := fstest.MapFS{"bad.json": {Mode: fs.ModeDir}}
	invalidJSON := fstest.MapFS{"bad.json": {Data: []byte("{")}}
	if _, err := loadParameterCommandsFromMetadata(fstest.MapFS{}, "["); err == nil {
		t.Fatal("invalid metadata glob succeeded")
	}
	if _, err := loadParameterCommandsFromMetadata(directoryJSON, "*.json"); err == nil {
		t.Fatal("metadata directory read succeeded")
	}
	if _, err := loadParameterCommandsFromMetadata(invalidJSON, "*.json"); err == nil {
		t.Fatal("invalid metadata JSON succeeded")
	}
	missingPath := fstest.MapFS{"metadata.json": {Data: []byte(`{"version":1,"tools":{"sample.get":{"parameters":{"value":{"description":"value"}}}}}`)}}
	if _, err := loadParameterCommandsFromMetadata(missingPath, "*.json"); err == nil {
		t.Fatal("parameter metadata without cli_path succeeded")
	}
	validMetadata := fstest.MapFS{"metadata.json": {Data: []byte(`{"version":1,"tools":{"skip":{},"sample.get":{"cli_path":" sample get ","parameters":{"value":{"description":"value"}}}}}`)}}
	commands, err := loadParameterCommandsFromMetadata(validMetadata, "*.json")
	if err != nil || len(commands) != 1 || commands[0].Reason == "" || commands[0].CLIPath != "sample get" {
		t.Fatalf("metadata commands = %#v, %v", commands, err)
	}

	if _, err := loadAgentHintsFromSelection(fstest.MapFS{}, "["); err == nil {
		t.Fatal("invalid selection glob succeeded")
	}
	if _, err := loadAgentHintsFromSelection(directoryJSON, "*.json"); err == nil {
		t.Fatal("selection directory read succeeded")
	}
	if _, err := loadAgentHintsFromSelection(invalidJSON, "*.json"); err == nil {
		t.Fatal("invalid selection JSON succeeded")
	}
	validSelection := fstest.MapFS{"sample.json": {Data: []byte(`{
		"version":1,
		"products":{"sample":{"agent_summary":"summary","use_when":["use"],"avoid_when":["avoid"],"reviewed":true}},
		"tools":{"sample.get":{"agent_summary":"summary","use_when":["use"],"avoid_when":["avoid"],"examples":["dws sample get"],"reviewed":true}}
	}`)}}
	hints, err := loadAgentHintsFromSelection(validSelection, "*.json")
	if err != nil || hints.Products["sample"].Reason == "" || len(hints.Products["sample"].Evidence) != 1 || hints.Tools["sample.get"].Reason == "" || len(hints.Tools["sample.get"].Evidence) != 1 {
		t.Fatalf("selection hints = %#v, %v", hints, err)
	}

	if _, err := loadManualSchemaHintsFromHintDirs(invalidJSON, "*.json", validSelection, "*.json"); err == nil {
		t.Fatal("wrapper metadata failure succeeded")
	}
	if _, err := loadManualSchemaHintsFromHintDirs(validMetadata, "*.json", invalidJSON, "*.json"); err == nil {
		t.Fatal("wrapper selection failure succeeded")
	}
	invalidSelection := fstest.MapFS{"sample.json": {Data: []byte(`{"version":1,"tools":{"sample.get":{}}}`)}}
	if _, err := loadManualSchemaHintsFromHintDirs(validMetadata, "*.json", invalidSelection, "*.json"); err == nil {
		t.Fatal("wrapper selection validation failure succeeded")
	}
}

func TestCrossPlatformCoverageInterfaceRegistryRemainingEdges(t *testing.T) {
	ref := &embeddedMCPInterfaceRef{ProductID: "sample", RPCName: "get"}
	for _, tools := range []map[string]embeddedMCPToolMetadata{
		{" ": {InterfaceRef: ref}},
		{"sample.none": {}},
		{"sample.incomplete": {InterfaceRef: &embeddedMCPInterfaceRef{ProductID: "sample"}}},
	} {
		if _, err := buildInterfaceRegistry(tools); err == nil {
			t.Fatalf("invalid interface registry succeeded: %#v", tools)
		}
	}
	if err := validateSchemaRegistryInterfacesWithMetadata(SchemaRegistry{}, embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{"bad": {}}}); err == nil {
		t.Fatal("invalid embedded interface metadata succeeded")
	}

	interfaces, err := buildInterfaceRegistry(map[string]embeddedMCPToolMetadata{"sample.get": {InterfaceRef: ref}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []*InterfaceRefSpec{nil, {ProductID: "sample"}, {ProductID: "sample", RPCName: "missing"}} {
		if err := validateToolInterfaceRef("sample.get", "mcp", candidate, interfaces); err == nil {
			t.Fatalf("invalid interface ref succeeded: %#v", candidate)
		}
	}

	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{
		Identity:  ToolIdentitySpec{ProductID: "", Name: ""},
		Interface: InterfaceSpec{Mode: "remote", Availability: "available"},
	}}}}}
	if err := validateSchemaRegistryInterfacesWithMetadata(registry, embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{"sample.get": {InterfaceRef: ref}}}); err == nil || !strings.Contains(err.Error(), "<unknown>") {
		t.Fatalf("unknown canonical interface error = %v", err)
	}
}

func TestCrossPlatformCoverageReviewedDryRunCapabilityRemainingEdges(t *testing.T) {
	originalGroups := reviewedDryRunCapabilityGroups
	t.Cleanup(func() {
		reviewedDryRunCapabilityGroups = originalGroups
		resetReviewedDryRunCapabilitiesForTest()
	})
	for _, groups := range [][]dryRunCapabilityGroup{
		{{PreviewKind: "invalid", CanonicalPaths: []string{"sample.get"}}},
		{{PreviewKind: DryRunPreviewRequest, CanonicalPaths: []string{" "}}},
		{{PreviewKind: DryRunPreviewRequest, CanonicalPaths: []string{"sample.z", "sample.a"}}},
		{{PreviewKind: DryRunPreviewRequest, CanonicalPaths: []string{"sample.get"}}, {PreviewKind: DryRunPreviewPlan, CanonicalPaths: []string{"sample.get"}}},
	} {
		reviewedDryRunCapabilityGroups = groups
		resetReviewedDryRunCapabilitiesForTest()
		if _, err := loadReviewedDryRunCapabilities(); err == nil {
			t.Fatalf("invalid dry-run groups succeeded: %#v", groups)
		}
		if _, err := reviewedDryRunCapability("sample.get"); err == nil {
			t.Fatal("dry-run lookup ignored registry error")
		}
		if err := ValidateReviewedDryRunCapabilityDelivery(SchemaRegistry{}); err == nil {
			t.Fatal("dry-run delivery ignored registry error")
		}
	}

	reviewedDryRunCapabilityGroups = []dryRunCapabilityGroup{{PreviewKind: DryRunPreviewRequest, CanonicalPaths: []string{"sample.get"}}}
	resetReviewedDryRunCapabilitiesForTest()
	capabilities, err := ReviewedDryRunCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	delete(capabilities, "sample.get")
	if second, err := ReviewedDryRunCapabilities(); err != nil || len(second) != 1 {
		t.Fatalf("defensive dry-run copy = %#v, %v", second, err)
	}
	if spec, err := reviewedDryRunCapability("missing"); err != nil || spec != nil {
		t.Fatalf("missing dry-run capability = %#v, %v", spec, err)
	}
	if err := ValidateReviewedDryRunCapabilityDelivery(SchemaRegistry{}); err == nil {
		t.Fatal("missing final dry-run capability succeeded")
	}
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{
		{Identity: ToolIdentitySpec{CanonicalPath: "sample.get"}, DryRun: &DryRunSpec{PreviewKind: DryRunPreviewPlan}},
		{Identity: ToolIdentitySpec{CanonicalPath: "sample.other"}, DryRun: &DryRunSpec{PreviewKind: DryRunPreviewRequest}},
	}}}}
	if err := ValidateReviewedDryRunCapabilityDelivery(registry); err == nil {
		t.Fatal("mismatched and unreviewed dry-run capabilities succeeded")
	}
}

func resetReviewedDryRunCapabilitiesForTest() {
	reviewedDryRunCapabilitiesLazy.once = sync.Once{}
	reviewedDryRunCapabilitiesLazy.byCanonical = nil
	reviewedDryRunCapabilitiesLazy.err = nil
}

func TestCrossPlatformCoverageSchemaSnapshotAdapterRemainingEdges(t *testing.T) {
	if _, err := schemaToolSpecFromPayload(map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("unmarshalable tool payload succeeded")
	}
	if err := decodeStrictSchemaJSON([]byte(`{} {}`), &map[string]any{}); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("multiple JSON values error = %v", err)
	}
	if err := decodeStrictSchemaJSON([]byte(`{} trailing`), &map[string]any{}); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing JSON error = %v", err)
	}
	if schemaJSONEqual(make(chan int), map[string]any{}) {
		t.Fatal("unmarshalable JSON values compared equal")
	}

	registry, snapshot := validSnapshotAdapterFixture(t)
	if _, _, err := schemaRegistryFromSnapshot(SchemaCatalogSnapshot{Catalog: map[string]any{"bad": make(chan int)}}); err == nil {
		t.Fatal("unmarshalable catalog succeeded")
	}
	if _, _, err := schemaRegistryFromSnapshot(SchemaCatalogSnapshot{Catalog: map[string]any{"unknown": true}}); err == nil {
		t.Fatal("unknown catalog field succeeded")
	}

	canonical := registry.Products[0].Tools[0].Identity.CanonicalPath
	missing := cloneSnapshotForCoverage(t, snapshot)
	delete(missing.Tools, canonical)
	if _, _, err := schemaRegistryFromSnapshot(missing); err == nil || !strings.Contains(err.Error(), "no full ToolSpec") {
		t.Fatalf("missing detail error = %v", err)
	}
	badTool := cloneSnapshotForCoverage(t, snapshot)
	badTool.Tools[canonical]["unknown"] = true
	if _, _, err := schemaRegistryFromSnapshot(badTool); err == nil || !strings.Contains(err.Error(), "decode Schema ToolSpec") {
		t.Fatalf("invalid detail error = %v", err)
	}
	wrongProduct := cloneSnapshotForCoverage(t, snapshot)
	wrongProduct.Tools[canonical]["product_id"] = "other"
	wrongProduct.Tools[canonical]["canonical_path"] = "other.get"
	if _, _, err := schemaRegistryFromSnapshot(wrongProduct); err == nil || !strings.Contains(err.Error(), "belongs to product") {
		t.Fatalf("wrong product error = %v", err)
	}
	extra := cloneSnapshotForCoverage(t, snapshot)
	extra.Tools["sample.extra"] = cloneCoverageSchemaMap(extra.Tools[canonical])
	if _, _, err := schemaRegistryFromSnapshot(extra); err == nil || !strings.Contains(err.Error(), "absent from typed products") {
		t.Fatalf("extra detail error = %v", err)
	}

	roundTrip := cloneSnapshotForCoverage(t, snapshot)
	products := schemaMapSlice(roundTrip.Catalog["products"])
	tools := schemaMapSlice(products[0]["tools"])
	tools[0]["title"] = "changed summary"
	products[0]["tools"] = tools
	roundTrip.Catalog["products"] = products
	if _, _, err := schemaRegistryFromSnapshot(roundTrip); err == nil {
		t.Fatal("inconsistent summary round trip succeeded")
	}

	invalidRegistry := SchemaRegistry{Products: []ProductSpec{{ID: " "}}}
	if err := validateSnapshotTypedRoundTrip(snapshot, invalidRegistry); err == nil {
		t.Fatal("invalid registry snapshot round trip succeeded")
	}
	missingRendered := cloneSnapshotForCoverage(t, snapshot)
	missingRendered.Tools["sample.extra"] = map[string]any{}
	if err := validateSnapshotTypedRoundTrip(missingRendered, registry); err == nil || !strings.Contains(err.Error(), "dropped full tool") {
		t.Fatalf("dropped full tool error = %v", err)
	}
	changedTool := cloneSnapshotForCoverage(t, snapshot)
	changedTool.Tools[canonical]["title"] = "changed"
	if err := validateSnapshotTypedRoundTrip(changedTool, registry); err == nil || !strings.Contains(err.Error(), "changed full tool") {
		t.Fatalf("changed full tool error = %v", err)
	}
	changedProducts := cloneSnapshotForCoverage(t, snapshot)
	products = schemaMapSlice(changedProducts.Catalog["products"])
	products[0]["name"] = "changed"
	changedProducts.Catalog["products"] = products
	if err := validateSnapshotTypedRoundTrip(changedProducts, registry); err == nil || !strings.Contains(err.Error(), "product/tool summaries") {
		t.Fatalf("changed products error = %v", err)
	}
	changedCatalog := cloneSnapshotForCoverage(t, snapshot)
	changedCatalog.Catalog["source"] = "changed"
	if err := validateSnapshotTypedRoundTrip(changedCatalog, registry); err == nil || !strings.Contains(err.Error(), "complete Catalog") {
		t.Fatalf("changed catalog error = %v", err)
	}
}

func validSnapshotAdapterFixture(t *testing.T) (SchemaRegistry, SchemaCatalogSnapshot) {
	t.Helper()
	tool, err := ToolSpecFromRuntime(RuntimeToolSpecInput{Identity: ToolIdentitySpec{
		ProductID: "sample", Name: "get", CLIName: "get", CLIPath: "sample get",
	}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := SchemaRegistryFromRuntime("fixture", []ProductSpec{{ID: "sample", Name: "Sample", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatal(err)
	}
	return registry, SchemaCatalogSnapshot{Catalog: payload.Catalog, Tools: payload.Tools}
}

func cloneSnapshotForCoverage(t *testing.T, source SchemaCatalogSnapshot) SchemaCatalogSnapshot {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var out SchemaCatalogSnapshot
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func cloneCoverageSchemaMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
