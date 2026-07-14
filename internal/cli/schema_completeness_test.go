// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRuntimeSchemaCompletenessRequiresCoverageOrReviewedExclusion(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	missing := &cobra.Command{Use: "missing", Run: func(*cobra.Command, []string) {}}
	excluded := &cobra.Command{Use: "excluded", Run: func(*cobra.Command, []string) {}}
	hidden := &cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	root.AddCommand(covered, missing, excluded, hidden)

	report := runtimeSchemaCompletenessForTest(root, []RuntimeSchemaExclusion{{
		CLIPath: "excluded", Reason: "reviewed test exclusion", Reviewed: true,
	}})
	if !reflect.DeepEqual(report.Covered, []string{"covered"}) {
		t.Fatalf("covered = %v", report.Covered)
	}
	if !reflect.DeepEqual(report.Excluded, []string{"excluded"}) {
		t.Fatalf("excluded = %v", report.Excluded)
	}
	if !reflect.DeepEqual(report.Missing, []string{"missing"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
	if len(report.InvalidExclusions) != 0 || len(report.StaleExclusions) != 0 {
		t.Fatalf("invalid=%v stale=%v", report.InvalidExclusions, report.StaleExclusions)
	}
}

func TestRuntimeSchemaCompletenessRejectsInvalidAndStaleExclusions(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	current := &cobra.Command{Use: "current", Run: func(*cobra.Command, []string) {}}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	root.AddCommand(current, covered)
	report := runtimeSchemaCompletenessForTest(root, []RuntimeSchemaExclusion{
		{CLIPath: "current", Reason: "", Reviewed: true},
		{CLIPath: "stale", Reason: "reviewed but obsolete", Reviewed: true},
		{CLIPath: "covered", Reason: "no longer needed", Reviewed: true},
	})
	if !reflect.DeepEqual(report.InvalidExclusions, []string{"current"}) {
		t.Fatalf("invalid = %v", report.InvalidExclusions)
	}
	if !reflect.DeepEqual(report.StaleExclusions, []string{"covered", "stale"}) {
		t.Fatalf("stale = %v", report.StaleExclusions)
	}
	if !reflect.DeepEqual(report.Missing, []string{"current"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
}

func TestReviewedCommandRegistryOwnsIdentityWithoutNativeAnnotation(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "aisearch"}
	leaf := &cobra.Command{Use: "person", Run: func(*cobra.Command, []string) {}}
	product.AddCommand(leaf)
	root.AddCommand(product)

	reviewed, err := loadEmbeddedCommandRegistry()
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := reviewed.ByCLIPath["aisearch person"]
	if !ok || spec.CanonicalPath != "aisearch.enterprise_person_search" {
		t.Fatalf("reviewed registry fixture = %#v, %v", spec, ok)
	}
	effective, err := newEffectiveCommandRegistry([]CommandSpec{spec})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := collectRuntimeSchemaEntriesFromBound(bound)
	if err != nil || len(entries) != 1 || entries[0].ProductID+"."+entries[0].ToolName != spec.CanonicalPath {
		t.Fatalf("registry-owned entries = %#v, err=%v", entries, err)
	}

	AttachRuntimeSchema(leaf, "aisearch", "wrong_identity", "test")
	if _, err := BindEffectiveCommandRegistry(root, effective); err == nil || !strings.Contains(err.Error(), "conflicts with native annotation") {
		t.Fatalf("native mismatch error = %v", err)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsAnnotatedLeafOutsideSurface(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	omitted := &cobra.Command{Use: "omitted", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	AttachRuntimeSchema(omitted, "sample", "omitted", "test")
	product.AddCommand(covered, omitted)
	root.AddCommand(product)

	annotationReport := runtimeSchemaCompletenessForTest(root, nil)
	if !reflect.DeepEqual(annotationReport.Covered, []string{"sample covered", "sample omitted"}) || len(annotationReport.Missing) != 0 {
		t.Fatalf("annotation report = %#v", annotationReport)
	}

	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.covered", CLIPath: "sample covered",
	})
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, nil)
	if !reflect.DeepEqual(report.Covered, []string{"sample covered"}) {
		t.Fatalf("covered = %v", report.Covered)
	}
	if !reflect.DeepEqual(report.Missing, []string{"sample omitted"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
}

func TestSchemaCatalogDeliveryCompletenessAcceptsExactReviewedExclusion(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	covered := &cobra.Command{Use: "covered", Run: func(*cobra.Command, []string) {}}
	excluded := &cobra.Command{Use: "excluded", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(covered, "sample", "covered", "test")
	product.AddCommand(covered, excluded)
	root.AddCommand(product)

	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.covered", CLIPath: "sample covered",
	})
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, []RuntimeSchemaExclusion{{
		CLIPath: "sample excluded", Reason: "reviewed test exclusion", Reviewed: true,
	}})
	if !reflect.DeepEqual(report.Covered, []string{"sample covered"}) ||
		!reflect.DeepEqual(report.Excluded, []string{"sample excluded"}) ||
		len(report.Missing) != 0 || len(report.StaleExclusions) != 0 {
		t.Fatalf("delivery report = %#v", report)
	}
}

func TestSchemaCatalogDeliveryCompletenessAcceptsDeliveredAlias(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	current := &cobra.Command{Use: "current", Hidden: true, Run: func(*cobra.Command, []string) {}}
	legacy := &cobra.Command{Use: "legacy", Run: func(*cobra.Command, []string) {}}
	annotateTestCompatibilityPair(current, legacy)
	AttachRuntimeSchema(current, "sample", "current", "test")
	AttachRuntimeSchema(legacy, "sample", "current", "test")
	product.AddCommand(current, legacy)
	root.AddCommand(product)

	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.current", CLIPath: "sample current", Aliases: []string{"sample legacy"},
	})
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, nil)
	if !reflect.DeepEqual(report.Covered, []string{"sample legacy"}) || len(report.Missing) != 0 {
		t.Fatalf("delivery report = %#v", report)
	}
}

func TestSchemaRegistryRejectsIdentityConflictOnAliasLeaf(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "sample"}
	current := &cobra.Command{Use: "current", Run: func(*cobra.Command, []string) {}}
	legacy := &cobra.Command{Use: "legacy", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(current, "sample", "current", "test")
	AttachRuntimeSchema(legacy, "sample", "current", "test")
	annotateManualSchemaIdentity(legacy, "sample.wrong", "reviewed conflict fixture")
	product.AddCommand(current, legacy)
	root.AddCommand(product)

	if _, err := schemaRegistryForTest(root); err == nil || !strings.Contains(err.Error(), "sample legacy") || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("schemaRegistryForTest() error = %v, want alias identity conflict", err)
	}
	report := schemaCatalogDeliveryCompletenessForTest(root, schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.current", CLIPath: "sample current", Aliases: []string{"sample legacy"},
	}), nil)
	if len(report.DeliveryErrors) == 0 || !strings.Contains(strings.Join(report.DeliveryErrors, " "), "sample legacy") {
		t.Fatalf("delivery errors = %v, want alias identity conflict", report.DeliveryErrors)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsWrongCanonical(t *testing.T) {
	root := schemaDeliveryTestRoot(
		schemaDeliveryTestTool{Canonical: "sample.expected", CLIPath: "sample run"},
	)
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.wrong", CLIPath: "sample run",
	})
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, nil)
	if !reflect.DeepEqual(report.Missing, []string{"sample run"}) {
		t.Fatalf("missing = %v", report.Missing)
	}
	if len(report.DeliveryErrors) == 0 {
		t.Fatal("wrong canonical passed final delivery validation")
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsMissingFinalLookup(t *testing.T) {
	root := schemaDeliveryTestRoot(
		schemaDeliveryTestTool{Canonical: "sample.delivered", CLIPath: "sample delivered"},
		schemaDeliveryTestTool{Canonical: "sample.missing", CLIPath: "sample missing"},
	)
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.delivered", CLIPath: "sample delivered",
	})
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, nil)
	if !reflect.DeepEqual(report.Covered, []string{"sample delivered"}) || !reflect.DeepEqual(report.Missing, []string{"sample missing"}) {
		t.Fatalf("delivery report = %#v", report)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsAliasCollision(t *testing.T) {
	root := schemaDeliveryTestRoot(
		schemaDeliveryTestTool{Canonical: "sample.one", CLIPath: "sample one"},
		schemaDeliveryTestTool{Canonical: "sample.two", CLIPath: "sample two"},
	)
	snapshot := schemaDeliveryTestSnapshot(
		schemaDeliveryTestTool{Canonical: "sample.one", CLIPath: "sample one", Aliases: []string{"sample shared"}},
		schemaDeliveryTestTool{Canonical: "sample.two", CLIPath: "sample two"},
	)
	// Corrupt the serialized delivery boundary after building a valid typed
	// registry. SchemaRegistry itself rejects this state at construction time;
	// the production loader must reject the same collision after decoding.
	snapshot.Tools["sample.two"]["aliases"] = []string{"sample shared"}
	for _, product := range snapshot.Catalog["products"].([]map[string]any) {
		for _, tool := range product["tools"].([]map[string]any) {
			if schemaString(tool["canonical_path"]) == "sample.two" {
				tool["aliases"] = []string{"sample shared"}
			}
		}
	}
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, nil)
	if len(report.DeliveryErrors) != 1 || (!strings.Contains(report.DeliveryErrors[0], "resolves to both") && !strings.Contains(report.DeliveryErrors[0], "claimed by both")) {
		t.Fatalf("delivery errors = %v", report.DeliveryErrors)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsPhantomAlias(t *testing.T) {
	tool := schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run", Aliases: []string{"sample ghost"}}
	root := schemaDeliveryTestRoot(schemaDeliveryTestTool{Canonical: tool.Canonical, CLIPath: tool.CLIPath})
	report := schemaCatalogDeliveryCompletenessForTest(root, schemaDeliveryTestSnapshot(tool), nil)
	if len(report.DeliveryErrors) != 1 || !strings.Contains(report.DeliveryErrors[0], "non-executable Schema path") {
		t.Fatalf("delivery errors = %v", report.DeliveryErrors)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsSummaryLeafDrift(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	snapshot.Catalog["products"].([]map[string]any)[0]["tools"].([]map[string]any)[0]["primary_cli_path"] = "sample drift"
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	report := schemaCatalogDeliveryCompletenessForTest(schemaDeliveryTestRoot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"}), snapshot, nil)
	if len(report.DeliveryErrors) != 1 || (!strings.Contains(report.DeliveryErrors[0], "disagrees with full leaf") && !strings.Contains(report.DeliveryErrors[0], "changed product/tool summaries")) {
		t.Fatalf("delivery errors = %v", report.DeliveryErrors)
	}
}

func TestSchemaCatalogDeliveryCompletenessRejectsProvenanceDrift(t *testing.T) {
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	provenance := snapshot.Tools["sample.run"]["field_provenance"].(map[string]any)
	canonical := provenance["canonical_path"].(map[string]any)
	canonical["value"] = "sample.wrong"
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

	report := schemaCatalogDeliveryCompletenessForTest(schemaDeliveryTestRoot(schemaDeliveryTestTool{
		Canonical: "sample.run", CLIPath: "sample run",
	}), snapshot, nil)
	if len(report.DeliveryErrors) != 1 || !strings.Contains(report.DeliveryErrors[0], "provenance winner does not equal final value") {
		t.Fatalf("delivery errors = %v", report.DeliveryErrors)
	}
}

func TestSchemaCatalogDeliveryCompletenessRoundTripsThroughProductionLoader(t *testing.T) {
	tool := schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"}
	if err := validateSchemaCatalogDeliveryCompletenessForTest(schemaDeliveryTestRoot(tool), schemaDeliveryTestSnapshot(tool), nil); err != nil {
		t.Fatalf("valid final delivery: %v", err)
	}
}

type schemaDeliveryTestTool struct {
	Canonical  string
	CLIPath    string
	Aliases    []string
	Parameters []ParameterSpec
	Selection  SelectionSpec
}

func schemaDeliveryTestRoot(tools ...schemaDeliveryTestTool) *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	products := map[string]*cobra.Command{}
	for _, tool := range tools {
		parts := strings.SplitN(tool.Canonical, ".", 2)
		product := products[parts[0]]
		if product == nil {
			product = &cobra.Command{Use: parts[0]}
			products[parts[0]] = product
			root.AddCommand(product)
		}
		pathParts := strings.Fields(tool.CLIPath)
		leaf := &cobra.Command{Use: pathParts[len(pathParts)-1], Run: func(*cobra.Command, []string) {}}
		AttachRuntimeSchema(leaf, parts[0], parts[1], "test")
		product.AddCommand(leaf)
	}
	return root
}

func schemaDeliveryTestSnapshot(tools ...schemaDeliveryTestTool) SchemaCatalogSnapshot {
	registry := schemaDeliveryTestRegistry(tools...)
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		panic(err)
	}
	snapshot := SchemaCatalogSnapshot{
		Version: SchemaCatalogSnapshotVersion,
		Catalog: payload.Catalog,
		Tools:   payload.Tools,
	}
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	return snapshot
}

func schemaDeliveryTestRegistry(tools ...schemaDeliveryTestTool) SchemaRegistry {
	products := map[string][]ToolSpec{}
	for _, tool := range tools {
		parts := strings.SplitN(tool.Canonical, ".", 2)
		interfaceReason := "synthetic delivery fixture"
		provenanceValues := map[string]any{
			"canonical_path":   tool.Canonical,
			"effect":           "",
			"risk":             "",
			"confirmation":     "",
			"idempotency":      "",
			"interface_ref":    nil,
			"interface_mode":   "local",
			"availability":     "available",
			"interface_reason": interfaceReason,
			"agent_summary":    tool.Selection.AgentSummary,
		}
		for field, values := range map[string][]string{
			"use_when":      tool.Selection.UseWhen,
			"avoid_when":    tool.Selection.AvoidWhen,
			"prerequisites": tool.Selection.Prerequisites,
			"tips":          tool.Selection.Tips,
			"workflow_refs": tool.Selection.WorkflowRefs,
			"examples":      tool.Selection.Examples,
		} {
			if values != nil {
				provenanceValues[field] = values
			}
		}
		if tool.Selection.Reviewed != nil {
			provenanceValues["reviewed"] = *tool.Selection.Reviewed
		}
		spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
			Identity: ToolIdentitySpec{
				ProductID:      parts[0],
				Name:           parts[1],
				CLIName:        strings.Fields(tool.CLIPath)[len(strings.Fields(tool.CLIPath))-1],
				CanonicalPath:  tool.Canonical,
				CLIPath:        tool.CLIPath,
				PrimaryCLIPath: tool.CLIPath,
				Aliases:        append([]string(nil), tool.Aliases...),
				Source:         "test",
			},
			Parameters:      append([]ParameterSpec(nil), tool.Parameters...),
			Interface:       InterfaceSpec{Mode: "local", Availability: "available", Reason: interfaceReason},
			Selection:       tool.Selection,
			FieldProvenance: schemaDeliveryTestProvenance(provenanceValues),
		})
		if err != nil {
			panic(err)
		}
		products[parts[0]] = append(products[parts[0]], spec)
	}
	productIDs := make([]string, 0, len(products))
	for productID := range products {
		productIDs = append(productIDs, productID)
	}
	sort.Strings(productIDs)
	typedProducts := make([]ProductSpec, 0, len(productIDs))
	for _, productID := range productIDs {
		typedProducts = append(typedProducts, ProductSpec{ID: productID, Tools: products[productID]})
	}
	registry, err := SchemaRegistryFromRuntime("", typedProducts)
	if err != nil {
		panic(err)
	}
	return registry
}

func schemaDeliveryTestProvenance(fields map[string]any) map[string]FieldProvenance {
	provenance := make(map[string]FieldProvenance, len(fields))
	for field, value := range fields {
		encoded, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		selected := true
		provenance[field] = FieldProvenance{
			Value:      encoded,
			Source:     "test",
			Precedence: "test",
			Resolution: "test",
			Candidates: []FieldCandidateProvenance{{Value: append(json.RawMessage(nil), encoded...), Source: "test", Precedence: "test", Selected: &selected}},
		}
	}
	return provenance
}
