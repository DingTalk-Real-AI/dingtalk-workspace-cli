// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageResolveAndAssembleSchemaRegistryErrorBoundaries(t *testing.T) {
	oldApply := resolveApplyManualSchemaHints
	oldEffective := resolveEffectiveCommandRegistry
	oldBind := resolveBoundCommandRegistry
	oldAssemble := resolveAssembleSchemaRegistry
	oldDelivery := resolveValidateParameterDelivery
	oldValidateBindings := assembleValidateBindings
	oldCollect := assembleCollectEntries
	oldTool := assembleRuntimeToolSpec
	oldTyped := assembleTypedRegistry
	oldMarshal := assembleMarshalRaw
	t.Cleanup(func() {
		resolveApplyManualSchemaHints = oldApply
		resolveEffectiveCommandRegistry = oldEffective
		resolveBoundCommandRegistry = oldBind
		resolveAssembleSchemaRegistry = oldAssemble
		resolveValidateParameterDelivery = oldDelivery
		assembleValidateBindings = oldValidateBindings
		assembleCollectEntries = oldCollect
		assembleRuntimeToolSpec = oldTool
		assembleTypedRegistry = oldTyped
		assembleMarshalRaw = oldMarshal
	})

	if _, err := ResolveSchemaBuild(nil); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("nil root error = %v", err)
	}
	if _, err := AssembleSchemaRegistry(nil); err == nil || !strings.Contains(err.Error(), "root is nil") {
		t.Fatalf("AssembleSchemaRegistry(nil) error = %v", err)
	}
	root := &cobra.Command{Use: "dws"}
	resolveApplyManualSchemaHints = func(*cobra.Command) (ManualSchemaHintReport, error) {
		return ManualSchemaHintReport{}, errors.New("apply failed")
	}
	if _, err := ResolveSchemaBuild(root); err == nil || !strings.Contains(err.Error(), "apply failed") {
		t.Fatalf("apply error = %v", err)
	}
	resolveApplyManualSchemaHints = func(*cobra.Command) (ManualSchemaHintReport, error) { return ManualSchemaHintReport{}, nil }
	resolveEffectiveCommandRegistry = func(*cobra.Command) (EffectiveCommandRegistry, error) {
		return EffectiveCommandRegistry{}, errors.New("effective failed")
	}
	if _, err := ResolveSchemaBuild(root); err == nil || !strings.Contains(err.Error(), "effective failed") {
		t.Fatalf("effective error = %v", err)
	}
	resolveEffectiveCommandRegistry = func(*cobra.Command) (EffectiveCommandRegistry, error) { return EffectiveCommandRegistry{}, nil }
	resolveBoundCommandRegistry = func(*cobra.Command, EffectiveCommandRegistry) (BoundCommandRegistry, error) {
		return BoundCommandRegistry{}, errors.New("bind failed")
	}
	if _, err := ResolveSchemaBuild(root); err == nil || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("bind error = %v", err)
	}
	resolveBoundCommandRegistry = func(*cobra.Command, EffectiveCommandRegistry) (BoundCommandRegistry, error) {
		return BoundCommandRegistry{}, nil
	}
	resolveAssembleSchemaRegistry = func(BoundCommandRegistry) (SchemaRegistry, error) {
		return SchemaRegistry{}, errors.New("assemble failed")
	}
	if _, err := ResolveSchemaBuild(root); err == nil || !strings.Contains(err.Error(), "assemble failed") {
		t.Fatalf("assemble error = %v", err)
	}
	resolveAssembleSchemaRegistry = func(BoundCommandRegistry) (SchemaRegistry, error) { return SchemaRegistry{}, nil }
	resolveValidateParameterDelivery = func(BoundCommandRegistry, SchemaRegistry) error { return errors.New("delivery failed") }
	if _, err := AssembleSchemaRegistry(root); err == nil || !strings.Contains(err.Error(), "delivery failed") {
		t.Fatalf("delivery error = %v", err)
	}

	assembleValidateBindings = func() error { return errors.New("bindings failed") }
	if _, err := AssembleSchemaRegistryFromBound(BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "bindings failed") {
		t.Fatalf("bindings error = %v", err)
	}
	assembleValidateBindings = func() error { return nil }
	assembleCollectEntries = func(BoundCommandRegistry) ([]runtimeSchemaEntry, error) { return nil, errors.New("collect failed") }
	if _, err := assembleSchemaRegistryFromBound(BoundCommandRegistry{}, runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "collect failed") {
		t.Fatalf("collect error = %v", err)
	}
	assembleCollectEntries = func(BoundCommandRegistry) ([]runtimeSchemaEntry, error) {
		return []runtimeSchemaEntry{{ProductID: "sample", ToolName: "run"}}, nil
	}
	assembleRuntimeToolSpec = func(runtimeSchemaEntry, runtimeSchemaMetadataSources) (ToolSpec, error) {
		return ToolSpec{}, errors.New("tool failed")
	}
	if _, err := assembleSchemaRegistryFromBound(BoundCommandRegistry{}, runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "tool failed") {
		t.Fatalf("tool error = %v", err)
	}

	assembleCollectEntries = func(BoundCommandRegistry) ([]runtimeSchemaEntry, error) { return nil, nil }
	assembleTypedRegistry = func(string, []ProductSpec) (SchemaRegistry, error) {
		return SchemaRegistry{}, errors.New("typed failed")
	}
	if _, err := assembleSchemaRegistryFromBound(BoundCommandRegistry{}, runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "typed failed") {
		t.Fatalf("typed registry error = %v", err)
	}
	assembleTypedRegistry = func(string, []ProductSpec) (SchemaRegistry, error) { return SchemaRegistry{}, nil }
	assembleMarshalRaw = func(any) (json.RawMessage, error) { return nil, errors.New("marshal failed") }
	if _, err := assembleSchemaRegistryFromBound(BoundCommandRegistry{}, runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "interface metadata") {
		t.Fatalf("interface marshal error = %v", err)
	}
	calls := 0
	assembleMarshalRaw = func(any) (json.RawMessage, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("marshal failed")
		}
		return json.RawMessage(`{}`), nil
	}
	if _, err := assembleSchemaRegistryFromBound(BoundCommandRegistry{}, runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "Agent metadata") {
		t.Fatalf("agent marshal error = %v", err)
	}
}

func TestCrossPlatformCoverageRuntimeToolSpecDependencyErrors(t *testing.T) {
	oldDryRun := resolveReviewedDryRun
	oldText := resolveRuntimeToolText
	oldParameters := resolveRuntimeParameters
	t.Cleanup(func() {
		resolveReviewedDryRun = oldDryRun
		resolveRuntimeToolText = oldText
		resolveRuntimeParameters = oldParameters
	})
	entry := runtimeSchemaEntry{ProductID: "sample", ToolName: "run", Command: &cobra.Command{Use: "run"}}
	metadata := runtimeSchemaMetadataSources{}
	resolveReviewedDryRun = func(string) (*DryRunSpec, error) { return nil, errors.New("dry run failed") }
	if _, err := runtimeToolSpecFromMetadata(entry, metadata); err == nil || !strings.Contains(err.Error(), "dry run failed") {
		t.Fatalf("dry-run error = %v", err)
	}
	resolveReviewedDryRun = func(string) (*DryRunSpec, error) { return nil, nil }
	resolveRuntimeToolText = func(runtimeSchemaEntry, runtimeSchemaMetadataSources) (string, string, string, map[string]FieldProvenance, error) {
		return "", "", "", nil, errors.New("text failed")
	}
	if _, err := runtimeToolSpecFromMetadata(entry, metadata); err == nil || !strings.Contains(err.Error(), "text failed") {
		t.Fatalf("text error = %v", err)
	}
	resolveRuntimeToolText = func(runtimeSchemaEntry, runtimeSchemaMetadataSources) (string, string, string, map[string]FieldProvenance, error) {
		return "title", "description", "source", map[string]FieldProvenance{}, nil
	}
	resolveRuntimeParameters = func(*cobra.Command, string, map[string]ParameterSchemaHint, map[string]embeddedMCPParamMeta, RuntimeSchemaConstraints) ([]ParameterSpec, error) {
		return nil, errors.New("parameters failed")
	}
	if _, err := runtimeToolSpecFromMetadata(entry, metadata); err == nil || !strings.Contains(err.Error(), "parameters failed") {
		t.Fatalf("parameter error = %v", err)
	}
	if _, err := marshalSchemaRaw(func() {}); err == nil {
		t.Fatal("marshalSchemaRaw accepted an unsupported value")
	}
}

func TestCrossPlatformCoverageRuntimeSchemaRegistryPayloadBranches(t *testing.T) {
	oldSnapshot := renderRegistrySnapshot
	oldPayload := renderRegistryPayload
	oldProduct := renderRegistryProductSummary
	oldToolPayload := renderRegistryToolPayload
	oldToolSummary := renderRegistryToolSummary
	t.Cleanup(func() {
		renderRegistrySnapshot = oldSnapshot
		renderRegistryPayload = oldPayload
		renderRegistryProductSummary = oldProduct
		renderRegistryToolPayload = oldToolPayload
		renderRegistryToolSummary = oldToolSummary
	})

	tool, err := ToolSpecFromRuntime(RuntimeToolSpecInput{Identity: ToolIdentitySpec{
		ProductID: "sample", Name: "get", CLIName: "get", CanonicalPath: "sample.get", Path: "sample.get",
		CLIPath: "sample item get", PrimaryCLIPath: "sample item get", Aliases: []string{"sample item fetch"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := SchemaRegistryFromRuntime("fixture", []ProductSpec{{ID: "sample", Name: "Sample", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeSchemaPayloadFromRegistry(SchemaRegistry{Products: []ProductSpec{{ID: " "}}}, nil); err == nil {
		t.Fatal("invalid registry payload succeeded")
	}

	renderRegistrySnapshot = func(SchemaRegistry) (SchemaSnapshotPayload, error) {
		return SchemaSnapshotPayload{}, errors.New("snapshot failed")
	}
	if _, err := runtimeSchemaPayloadFromRegistry(registry, nil); err == nil || !strings.Contains(err.Error(), "snapshot failed") {
		t.Fatalf("snapshot error = %v", err)
	}
	renderRegistrySnapshot = oldSnapshot
	renderRegistryToolPayload = func(ToolSpec) (map[string]any, error) { return nil, errors.New("tool payload failed") }
	if _, err := runtimeSchemaPayloadFromRegistry(registry, []string{"sample.get"}); err == nil || !strings.Contains(err.Error(), "tool payload failed") {
		t.Fatalf("tool payload error = %v", err)
	}
	renderRegistryToolPayload = oldToolPayload
	renderRegistryProductSummary = func(ProductSpec) (map[string]any, error) { return nil, errors.New("product failed") }
	if _, err := runtimeSchemaPayloadFromRegistry(registry, []string{"sample"}); err == nil || !strings.Contains(err.Error(), "product failed") {
		t.Fatalf("product summary error = %v", err)
	}
	renderRegistryProductSummary = oldProduct
	renderRegistryToolSummary = func(ToolSpec) (map[string]any, error) { return nil, errors.New("summary failed") }
	if _, err := runtimeSchemaPayloadFromRegistry(registry, []string{"sample item"}); err == nil || !strings.Contains(err.Error(), "summary failed") {
		t.Fatalf("group summary error = %v", err)
	}
	renderRegistryToolSummary = oldToolSummary

	for _, path := range []string{"sample", "sample item", "sample.get", "sample item fetch"} {
		if _, err := runtimeSchemaPayloadFromRegistry(registry, []string{path}); err != nil {
			t.Errorf("payload %q error = %v", path, err)
		}
	}
	if _, err := runtimeSchemaPayloadFromRegistry(registry, []string{"sample other"}); err == nil || !strings.Contains(err.Error(), "unknown runtime schema path") {
		t.Fatalf("unknown path error = %v", err)
	}
	if got := schemaToolForResolvedPath(tool, "sample item fetch"); !got.Identity.IsAlias || got.Identity.CLIPath != "sample item fetch" {
		t.Fatalf("alias view = %#v", got.Identity)
	}
	if got := schemaToolForResolvedPath(tool, "other"); got.Identity.IsAlias {
		t.Fatalf("unknown path became alias = %#v", got.Identity)
	}
	if !schemaToolUnderGroup(tool, "sample item") || schemaToolUnderGroup(tool, "sample other") {
		t.Fatal("group containment mismatch")
	}

	renderRegistryPayload = func(SchemaRegistry) (map[string]any, error) { return nil, errors.New("all failed") }
	if _, err := runtimeSchemaAllPayloadFromRegistry(registry); err == nil || !strings.Contains(err.Error(), "all failed") {
		t.Fatalf("all payload error = %v", err)
	}
}

func TestCrossPlatformCoverageRuntimeRegistryIdentityAndAgentMetadataRemainingEdges(t *testing.T) {
	baseTool := ToolSpec{Identity: ToolIdentitySpec{
		ProductID: "sample", Name: "run", CanonicalPath: "sample.run", Path: "sample.run",
		CLIPath: "sample run", PrimaryCLIPath: "sample run", Aliases: []string{"sample execute"}, Source: "registry",
	}}
	baseCommand := CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run", Aliases: []string{"sample execute"}, Visibility: SchemaVisibilityPublic, Source: "registry"}
	makeEffective := func(command CommandSpec) EffectiveCommandRegistry {
		effective, err := newEffectiveCommandRegistry([]CommandSpec{command})
		if err != nil {
			t.Fatal(err)
		}
		return effective
	}

	if err := validateSchemaRegistryAgainstCommandRegistry(SchemaRegistry{Products: []ProductSpec{{ID: " "}}}, makeEffective(baseCommand)); err == nil {
		t.Fatal("invalid registry passed command registry validation")
	}
	if err := validateSchemaRegistryAgainstCommandRegistry(SchemaRegistry{}, makeEffective(baseCommand)); err == nil || !strings.Contains(err.Error(), "contains 0 canonical") {
		t.Fatalf("count mismatch = %v", err)
	}
	if err := validateSchemaRegistryAgainstCommandRegistry(SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{baseTool}}}}, makeEffective(baseCommand)); err != nil {
		t.Fatalf("valid registry identity rejected: %v", err)
	}
	emptySourceProductEffective := EffectiveCommandRegistry{ByCanonical: map[string]CommandSpec{
		"sample.run": baseCommand,
	}}
	if err := validateSchemaRegistryAgainstCommandRegistry(SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{baseTool}}}}, emptySourceProductEffective); err != nil {
		t.Fatalf("empty expected source product fallback rejected: %v", err)
	}
	other := baseTool
	other.Identity.ProductID, other.Identity.Name, other.Identity.CanonicalPath, other.Identity.Path = "other", "run", "other.run", "other.run"
	if err := validateSchemaRegistryAgainstCommandRegistry(SchemaRegistry{Products: []ProductSpec{{ID: "other", Tools: []ToolSpec{other}}}}, makeEffective(baseCommand)); err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Fatalf("missing canonical = %v", err)
	}

	mutations := []struct {
		name string
		tool func(ToolSpec) ToolSpec
		want string
	}{
		{name: "canonical cli", tool: func(v ToolSpec) ToolSpec { v.Identity.CLIPath = "sample execute"; return v }, want: "must equal primary"},
		{name: "alias marker", tool: func(v ToolSpec) ToolSpec { v.Identity.IsAlias = true; return v }, want: "is_alias=false"},
		{name: "primary", tool: func(v ToolSpec) ToolSpec {
			v.Identity.CLIPath, v.Identity.PrimaryCLIPath = "sample other", "sample other"
			return v
		}, want: "primary path"},
		{name: "aliases", tool: func(v ToolSpec) ToolSpec { v.Identity.Aliases = []string{"sample alternate"}; return v }, want: "aliases"},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			tool := tc.tool(baseTool)
			err := validateSchemaRegistryAgainstCommandRegistry(SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}}}, makeEffective(baseCommand))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	oldMetadata := finalSchemaAgentMetadata
	t.Cleanup(func() { finalSchemaAgentMetadata = oldMetadata })
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{baseTool}}}}
	finalSchemaAgentMetadata = func() embeddedAgentMetadata {
		return embeddedAgentMetadata{Tools: map[string]agentToolMetadata{"missing": {}}}
	}
	if err := validateSchemaRegistryAgentMetadata(registry); err == nil || !strings.Contains(err.Error(), "does not resolve") || !strings.Contains(err.Error(), "has no generated") {
		t.Fatalf("unresolved metadata error = %v", err)
	}
	finalSchemaAgentMetadata = func() embeddedAgentMetadata {
		return embeddedAgentMetadata{Tools: map[string]agentToolMetadata{"sample.run": {}, "sample execute": {}}}
	}
	if err := validateSchemaRegistryAgentMetadata(registry); err == nil || !strings.Contains(err.Error(), "both resolve") {
		t.Fatalf("duplicate metadata error = %v", err)
	}
	finalSchemaAgentMetadata = func() embeddedAgentMetadata {
		return embeddedAgentMetadata{Tools: map[string]agentToolMetadata{"sample.run": {}}}
	}
	if err := validateSchemaRegistryAgentMetadata(registry); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
	if err := validateSchemaRegistryAgentMetadata(SchemaRegistry{Products: []ProductSpec{{ID: " "}}}); err == nil {
		t.Fatal("invalid registry metadata validation succeeded")
	}
}

func TestCrossPlatformCoverageFinalSchemaProvenanceRejectsInvalidTool(t *testing.T) {
	registry := SchemaRegistry{Products: []ProductSpec{{
		ID:              "sample",
		Selection:       SelectionSpec{AgentSummary: "summary", UseWhen: []string{}, AvoidWhen: []string{}},
		FieldProvenance: map[string]FieldProvenance{},
		Tools:           []ToolSpec{{Identity: ToolIdentitySpec{CanonicalPath: "invalid"}}},
	}}}
	err := validateFinalSchemaProvenanceCoverage(registry)
	if err == nil || !strings.Contains(err.Error(), "no provenance") {
		t.Fatalf("provenance error = %v", err)
	}
}
