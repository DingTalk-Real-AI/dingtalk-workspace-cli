// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageBuildSchemaCatalogSnapshotGateFailures(t *testing.T) {
	registry, _ := validSnapshotAdapterFixture(t)
	resolved := ResolvedSchemaBuild{root: &cobra.Command{Use: "root"}, registry: registry}
	original := captureCatalogHooks()
	t.Cleanup(original.restore)
	installCatalogBuildNoops()

	if _, err := BuildSchemaCatalogSnapshot(resolved, SchemaCatalogBuildOptions{RegistryHash: "wrong"}); err == nil {
		t.Fatal("wrong registry hash succeeded")
	}

	failure := errors.New("gate failed")
	tests := []struct {
		name string
		set  func()
	}{
		{"parameter bindings", func() {
			buildCatalogValidateParameterBindings = func(BoundCommandRegistry, SchemaRegistry) error { return failure }
		}},
		{"dry run", func() { buildCatalogValidateDryRun = func(SchemaRegistry) error { return failure } }},
		{"examples", func() {
			buildCatalogValidateExamples = func(BoundCommandRegistry, SchemaRegistry) (ManualAgentExampleExecutionPlan, error) {
				return ManualAgentExampleExecutionPlan{}, failure
			}
		}},
		{"completeness", func() {
			buildCatalogValidateCompleteness = func(*cobra.Command, BoundCommandRegistry) error { return failure }
		}},
		{"registry", func() {
			buildCatalogValidateRegistry = func(SchemaRegistry, EffectiveCommandRegistry) error { return failure }
		}},
		{"interfaces", func() { buildCatalogValidateInterfaces = func(SchemaRegistry) error { return failure } }},
		{"agent metadata", func() { buildCatalogValidateAgentMetadata = func(SchemaRegistry) error { return failure } }},
		{"provenance", func() { buildCatalogValidateProvenance = func(SchemaRegistry) error { return failure } }},
		{"delivery", func() {
			buildCatalogValidateDelivery = func(SchemaRegistry, SchemaCatalogSnapshot) error { return failure }
		}},
		{"final completeness", func() {
			buildCatalogValidateFinalCompleteness = func(*cobra.Command, BoundCommandRegistry, SchemaCatalogSnapshot) error { return failure }
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			installCatalogBuildNoops()
			tc.set()
			if _, err := BuildSchemaCatalogSnapshot(resolved, SchemaCatalogBuildOptions{}); err == nil || !strings.Contains(err.Error(), "gate failed") {
				t.Fatalf("gate error = %v", err)
			}
		})
	}

	installCatalogBuildNoops()
	invalid := resolved
	invalid.registry = SchemaRegistry{Products: []ProductSpec{{ID: " "}}}
	if _, err := BuildSchemaCatalogSnapshot(invalid, SchemaCatalogBuildOptions{}); err == nil || !strings.Contains(err.Error(), "serialize typed") {
		t.Fatalf("serialization error = %v", err)
	}
}

func TestCrossPlatformCoverageLoadSchemaCatalogSnapshotGateFailures(t *testing.T) {
	_, base := validSnapshotAdapterFixture(t)
	base.Version = SchemaCatalogSnapshotVersion
	base.SourceHash = schemaCatalogSnapshotHash(base)
	original := captureCatalogHooks()
	t.Cleanup(original.restore)
	installCatalogLoadNoops()

	badVersion := cloneSnapshotForCoverage(t, base)
	badVersion.Version = 2
	if _, err := loadSchemaCatalogSnapshot(badVersion); err == nil {
		t.Fatal("bad snapshot version succeeded")
	}
	if _, err := loadSchemaCatalogSnapshot(SchemaCatalogSnapshot{Version: SchemaCatalogSnapshotVersion}); err == nil {
		t.Fatal("empty snapshot succeeded")
	}
	badHash := cloneSnapshotForCoverage(t, base)
	badHash.SourceHash = "wrong"
	if _, err := loadSchemaCatalogSnapshot(badHash); err == nil {
		t.Fatal("bad snapshot hash succeeded")
	}
	badTyped := cloneSnapshotForCoverage(t, base)
	badTyped.Catalog["unknown"] = true
	badTyped.SourceHash = schemaCatalogSnapshotHash(badTyped)
	if _, err := loadSchemaCatalogSnapshot(badTyped); err == nil || !strings.Contains(err.Error(), "load typed") {
		t.Fatalf("typed load error = %v", err)
	}

	failure := errors.New("load gate failed")
	installCatalogLoadNoops()
	loadCatalogValidateInterfaces = func(SchemaRegistry) error { return failure }
	if _, err := loadSchemaCatalogSnapshot(base); err == nil || !strings.Contains(err.Error(), "load gate failed") {
		t.Fatalf("interface load gate = %v", err)
	}
	installCatalogLoadNoops()
	loadCatalogValidateProvenance = func(SchemaRegistry) error { return failure }
	if _, err := loadSchemaCatalogSnapshot(base); err == nil || !strings.Contains(err.Error(), "load gate failed") {
		t.Fatalf("provenance load gate = %v", err)
	}

	embedded := cloneSnapshotForCoverage(t, base)
	embedded.Catalog["source"] = "embedded-command-catalog"
	embedded.SourceHash = schemaCatalogSnapshotHash(embedded)
	installCatalogLoadNoops()
	loadCatalogValidateAgentMetadata = func(SchemaRegistry) error { return failure }
	if _, err := loadSchemaCatalogSnapshot(embedded); err == nil || !strings.Contains(err.Error(), "load gate failed") {
		t.Fatalf("agent metadata load gate = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaCatalogLookupAndConversionEdges(t *testing.T) {
	if exactSchemaCommand(nil, "sample") != nil {
		t.Fatal("nil root resolved a command")
	}
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(child)
	if exactSchemaCommand(root, "root missing") != nil || exactSchemaCommand(root, "root") != nil || exactSchemaCommand(root, "root child") != child {
		t.Fatal("exact schema command resolution mismatch")
	}

	if schemaMap("bad") != nil {
		t.Fatal("non-map schema value converted")
	}
	converted := schemaMap(map[string]any{"valid": map[string]any{"x": true}, "bad": "x"})
	if len(converted) != 1 {
		t.Fatalf("converted schema map = %#v", converted)
	}
	if got := firstNonEmptySchemaString(nil, " "); got != "" {
		t.Fatalf("empty schema string = %q", got)
	}

	invalidLoaded := loadedSchemaCatalog{Registry: SchemaRegistry{Products: []ProductSpec{{ID: " "}}}}
	if _, err := schemaPayloadFromLoadedCatalog(invalidLoaded, nil); err == nil {
		t.Fatal("invalid loaded overview succeeded")
	}

	originalAll, originalOverview := renderEmbeddedSchemaAll, renderEmbeddedSchemaOverview
	originalProduct, originalTool := renderSchemaProductSummary, renderSchemaToolSummary
	t.Cleanup(func() {
		renderEmbeddedSchemaAll, renderEmbeddedSchemaOverview = originalAll, originalOverview
		renderSchemaProductSummary, renderSchemaToolSummary = originalProduct, originalTool
	})
	failure := errors.New("render failed")
	renderEmbeddedSchemaAll = func(SchemaRegistry) (map[string]any, error) { return nil, failure }
	if _, err := embeddedSchemaAllPayload(); err == nil {
		t.Fatal("embedded all render failure succeeded")
	}
	renderEmbeddedSchemaAll = originalAll
	renderEmbeddedSchemaOverview = func(SchemaRegistry) (map[string]any, error) { return nil, failure }
	if _, err := embeddedSchemaOverviewPayload(); err == nil {
		t.Fatal("embedded overview render failure succeeded")
	}
	renderEmbeddedSchemaOverview = originalOverview

	loaded := embeddedSchemaCatalog()
	productID := loaded.Registry.Products[0].ID
	renderSchemaProductSummary = func(ProductSpec) (map[string]any, error) { return nil, failure }
	if _, err := schemaPayloadFromLoadedCatalog(loaded, []string{productID}); err == nil {
		t.Fatal("product summary render failure succeeded")
	}
	renderSchemaProductSummary = originalProduct
	group := ""
	for _, product := range loaded.Registry.Products {
		for _, tool := range product.Tools {
			parts := strings.Fields(tool.Identity.CLIPath)
			if len(parts) > 2 {
				group = strings.Join(parts[:len(parts)-1], " ")
				break
			}
		}
		if group != "" {
			break
		}
	}
	if group == "" {
		t.Fatal("production Catalog has no group path fixture")
	}
	renderSchemaToolSummary = func(ToolSpec) (map[string]any, error) { return nil, failure }
	if _, err := schemaPayloadFromLoadedCatalog(loaded, []string{group}); err == nil {
		t.Fatal("group tool render failure succeeded")
	}
}

type catalogHookSnapshot struct {
	parameterBindings func(BoundCommandRegistry, SchemaRegistry) error
	dryRun            func(SchemaRegistry) error
	examples          func(BoundCommandRegistry, SchemaRegistry) (ManualAgentExampleExecutionPlan, error)
	completeness      func(*cobra.Command, BoundCommandRegistry) error
	registry          func(SchemaRegistry, EffectiveCommandRegistry) error
	interfaces        func(SchemaRegistry) error
	agentMetadata     func(SchemaRegistry) error
	provenance        func(SchemaRegistry) error
	delivery          func(SchemaRegistry, SchemaCatalogSnapshot) error
	finalCompleteness func(*cobra.Command, BoundCommandRegistry, SchemaCatalogSnapshot) error
	loadInterfaces    func(SchemaRegistry) error
	loadProvenance    func(SchemaRegistry) error
	loadAgentMetadata func(SchemaRegistry) error
}

func captureCatalogHooks() catalogHookSnapshot {
	return catalogHookSnapshot{
		buildCatalogValidateParameterBindings, buildCatalogValidateDryRun, buildCatalogValidateExamples,
		buildCatalogValidateCompleteness, buildCatalogValidateRegistry, buildCatalogValidateInterfaces,
		buildCatalogValidateAgentMetadata, buildCatalogValidateProvenance, buildCatalogValidateDelivery,
		buildCatalogValidateFinalCompleteness, loadCatalogValidateInterfaces, loadCatalogValidateProvenance,
		loadCatalogValidateAgentMetadata,
	}
}

func (hooks catalogHookSnapshot) restore() {
	buildCatalogValidateParameterBindings = hooks.parameterBindings
	buildCatalogValidateDryRun = hooks.dryRun
	buildCatalogValidateExamples = hooks.examples
	buildCatalogValidateCompleteness = hooks.completeness
	buildCatalogValidateRegistry = hooks.registry
	buildCatalogValidateInterfaces = hooks.interfaces
	buildCatalogValidateAgentMetadata = hooks.agentMetadata
	buildCatalogValidateProvenance = hooks.provenance
	buildCatalogValidateDelivery = hooks.delivery
	buildCatalogValidateFinalCompleteness = hooks.finalCompleteness
	loadCatalogValidateInterfaces = hooks.loadInterfaces
	loadCatalogValidateProvenance = hooks.loadProvenance
	loadCatalogValidateAgentMetadata = hooks.loadAgentMetadata
}

func installCatalogBuildNoops() {
	buildCatalogValidateParameterBindings = func(BoundCommandRegistry, SchemaRegistry) error { return nil }
	buildCatalogValidateDryRun = func(SchemaRegistry) error { return nil }
	buildCatalogValidateExamples = func(BoundCommandRegistry, SchemaRegistry) (ManualAgentExampleExecutionPlan, error) {
		return ManualAgentExampleExecutionPlan{}, nil
	}
	buildCatalogValidateCompleteness = func(*cobra.Command, BoundCommandRegistry) error { return nil }
	buildCatalogValidateRegistry = func(SchemaRegistry, EffectiveCommandRegistry) error { return nil }
	buildCatalogValidateInterfaces = func(SchemaRegistry) error { return nil }
	buildCatalogValidateAgentMetadata = func(SchemaRegistry) error { return nil }
	buildCatalogValidateProvenance = func(SchemaRegistry) error { return nil }
	buildCatalogValidateDelivery = func(SchemaRegistry, SchemaCatalogSnapshot) error { return nil }
	buildCatalogValidateFinalCompleteness = func(*cobra.Command, BoundCommandRegistry, SchemaCatalogSnapshot) error { return nil }
}

func installCatalogLoadNoops() {
	loadCatalogValidateInterfaces = func(SchemaRegistry) error { return nil }
	loadCatalogValidateProvenance = func(SchemaRegistry) error { return nil }
	loadCatalogValidateAgentMetadata = func(SchemaRegistry) error { return nil }
}
