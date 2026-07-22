// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli_test

import (
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
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
	if err := cli.ValidateEmbeddedRuntimeSchemaCompleteness(app.NewSchemaSourceRootCommand()); err != nil {
		t.Fatalf("ValidateEmbeddedRuntimeSchemaCompleteness() error = %v", err)
	}
	root = app.NewSchemaSourceRootCommand()
	if _, err := cli.ApplyEmbeddedManualSchemaHints(root); err != nil {
		t.Fatal(err)
	}
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatal(err)
	}
	hints, err := cli.LoadAgentHintsFromSelectionForValidation(os.DirFS("schema_hints/selection"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ValidateManualAgentSelectionContract(bound, hints); err != nil {
		t.Fatalf("ValidateManualAgentSelectionContract() error = %v", err)
	}
	if _, _, err := cli.BuildManualAgentSelectionEvalFixture(bound, hints); err != nil {
		t.Fatalf("BuildManualAgentSelectionEvalFixture() error = %v", err)
	}
	if _, err := cli.ValidateEmbeddedManualAgentExampleDelivery(bound, registry); err != nil {
		t.Fatalf("ValidateEmbeddedManualAgentExampleDelivery() error = %v", err)
	}
	if err := cli.ValidateSchemaParameterBindingDelivery(bound, registry); err != nil {
		t.Fatalf("ValidateSchemaParameterBindingDelivery() error = %v", err)
	}
	exclusions, err := cli.EmbeddedRuntimeSchemaExclusions()
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
	if bindings, err := cli.EmbeddedSchemaParameterBindings(); err != nil || len(bindings) == 0 {
		t.Fatalf("EmbeddedSchemaParameterBindings() = %d, %v", len(bindings), err)
	}
	if counts := cli.RuntimeSchemaMetadataLoadCounts(); counts.AgentMetadata == 0 || counts.MCPMetadata == 0 {
		t.Fatalf("RuntimeSchemaMetadataLoadCounts() = %#v", counts)
	}
}
