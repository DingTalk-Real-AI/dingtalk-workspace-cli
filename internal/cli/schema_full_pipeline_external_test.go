// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli_test

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

// Exercise the complete production source-to-snapshot path from an external
// test package. Keeping this here (rather than in a generator package) means
// Go's normal per-package coverage accounting attributes the exercised Schema
// assembly code to internal/cli.
func TestCrossPlatformCoverageProductionSchemaSourcePipeline(t *testing.T) {
	root := app.NewSchemaSourceRootCommand()
	resolved, err := cli.ResolveSchemaBuild(root)
	if err != nil {
		t.Fatalf("ResolveSchemaBuild() error = %v", err)
	}
	if resolved.CommandCount() == 0 || resolved.RegistryHash() == "" {
		t.Fatalf("resolved build is empty: commands=%d hash=%q", resolved.CommandCount(), resolved.RegistryHash())
	}
	snapshot, err := cli.BuildSchemaCatalogSnapshot(resolved, cli.SchemaCatalogBuildOptions{
		RegistryHash: resolved.RegistryHash(),
	})
	if err != nil {
		t.Fatalf("BuildSchemaCatalogSnapshot() error = %v", err)
	}
	if len(snapshot.Tools) == 0 {
		t.Fatal("production Schema snapshot contains no tools")
	}
	registry, err := cli.AssembleSchemaRegistry(app.NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatalf("AssembleSchemaRegistry() error = %v", err)
	}
	if len(registry.Products) == 0 {
		t.Fatal("assembled production Schema registry contains no products")
	}
	if err := cli.ValidateRuntimeSchemaCompleteness(app.NewSchemaSourceRootCommand()); err != nil {
		t.Fatalf("ValidateRuntimeSchemaCompleteness() error = %v", err)
	}
	root = app.NewSchemaSourceRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ValidateAgentSelectionContract(bound); err != nil {
		t.Fatalf("ValidateAgentSelectionContract() error = %v", err)
	}
	if _, _, err := cli.BuildAgentSelectionEvalFixture(bound); err != nil {
		t.Fatalf("BuildAgentSelectionEvalFixture() error = %v", err)
	}
	if _, err := cli.ValidateAgentExampleDelivery(bound, registry); err != nil {
		t.Fatalf("ValidateAgentExampleDelivery() error = %v", err)
	}
	if err := cli.ValidateSchemaParameterBindingDelivery(bound, registry); err != nil {
		t.Fatalf("ValidateSchemaParameterBindingDelivery() error = %v", err)
	}
	exclusions, err := cli.ReviewedRuntimeSchemaExclusions()
	if err != nil {
		t.Fatal(err)
	}
	report := cli.RuntimeSchemaCompleteness(root, exclusions)
	if len(report.Missing)+len(report.InvalidExclusions)+len(report.StaleExclusions)+len(report.DeliveryErrors) != 0 {
		t.Fatalf("RuntimeSchemaCompleteness() = %#v", report)
	}
	if capabilities, err := cli.ReviewedDryRunCapabilities(); err != nil || len(capabilities) == 0 {
		t.Fatalf("ReviewedDryRunCapabilities() = %d, %v", len(capabilities), err)
	}
	// Phase 2: active binding tuples are empty; property delivery is ParamDecl.
	if bindings, err := cli.LoadSchemaParameterBindings(); err != nil || len(bindings) != 0 {
		t.Fatalf("LoadSchemaParameterBindings() = %d, %v; want empty active map after Phase 2", len(bindings), err)
	}
	if counts := cli.RuntimeSchemaMetadataLoadCounts(); counts.AgentMetadata == 0 {
		t.Fatalf("RuntimeSchemaMetadataLoadCounts() = %#v", counts)
	}
}

// TestCrossPlatformCoverageAssembleSchemaCatalogFromRootAndMaterialize covers
// the production RegisterSchemaSourceRoot → assembleSchemaCatalogFromRoot
// success path, content source_hash / catalog_hash, lazy materialize, and the
// ParamDecl-declared interface_type remaps that used to come from the retired
// MCP pin.
func TestCrossPlatformCoverageAssembleSchemaCatalogFromRootAndMaterialize(t *testing.T) {
	cli.InstallProductionSchemaAssemblyForTest(func() *cobra.Command {
		return app.NewSchemaSourceRootCommand()
	})
	t.Cleanup(cli.RestorePackageCLISchemaDeliveryForTest)

	all, err := cli.DeliverySchemaAllPayloadForTest()
	if err != nil {
		t.Fatalf("DeliverySchemaAllPayloadForTest() error = %v", err)
	}
	catalogHash, _ := all["catalog_hash"].(string)
	surfaceHash, _ := all["surface_hash"].(string)
	if catalogHash == "" || surfaceHash == "" {
		t.Fatalf("missing hashes: catalog_hash=%q surface_hash=%q", catalogHash, surfaceHash)
	}
	if catalogHash == surfaceHash {
		t.Fatalf("catalog_hash must be content hash, not registry/surface hash (%s)", catalogHash)
	}
	resolved, err := cli.ResolveSchemaBuild(app.NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatalf("ResolveSchemaBuild() error = %v", err)
	}
	if surfaceHash != resolved.RegistryHash() {
		t.Fatalf("surface_hash=%q, want EffectiveCommandRegistry hash %q", surfaceHash, resolved.RegistryHash())
	}

	wantTypes := map[string]map[string]string{
		"chat.reply_personal_message": {
			"ai-tag": "string",
		},
		"dev.search_open_platform_docs_rag": {
			"page": "number",
			"size": "number",
		},
		"minutes.list_accessible_minutes": {
			"start": "number",
			"end":   "number",
		},
		"minutes.list_shared_minutes": {
			"start": "number",
			"end":   "number",
		},
	}
	tools := map[string]map[string]any{}
	for _, tool := range schemaAnyMaps(all["products"]) {
		for _, leaf := range schemaAnyMaps(tool["tools"]) {
			canonical, _ := leaf["canonical_path"].(string)
			if canonical != "" {
				tools[canonical] = leaf
			}
		}
	}
	if len(tools) == 0 {
		t.Fatalf("schema --all contained no tools; top-level keys=%v", mapKeys(all))
	}
	for canonical, flags := range wantTypes {
		tool := tools[canonical]
		if tool == nil {
			t.Fatalf("missing tool %s in schema --all", canonical)
		}
		params, _ := tool["parameters"].(map[string]any)
		for flag, want := range flags {
			param, _ := params[flag].(map[string]any)
			if got, _ := param["interface_type"].(string); got != want {
				t.Fatalf("%s --%s interface_type = %#v, want %q", canonical, flag, param["interface_type"], want)
			}
		}
	}
}

func schemaAnyMaps(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}
