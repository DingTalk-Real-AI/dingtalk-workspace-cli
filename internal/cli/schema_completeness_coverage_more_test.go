// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageEmbeddedRuntimeSchemaExclusionDecodeEdges(t *testing.T) {
	previous := embeddedRuntimeSchemaExclusionsJSON
	t.Cleanup(func() { embeddedRuntimeSchemaExclusionsJSON = previous })
	cases := []struct {
		data string
		want string
	}{
		{data: "{", want: "decode"},
		{data: `{"version":2,"groups":[]}`, want: "unsupported"},
		{data: `{"version":1,"groups":[{"id":"","reason":"r","reviewed":true}]}`, want: "not reviewed"},
		{data: `{"version":1,"groups":[{"id":"g","reason":"r","reviewed":true,"commands":[" "]}]}`, want: "empty command"},
		{data: `{"version":1,"groups":[{"id":"g","reason":"r","reviewed":true,"commands":["a b","a  b"]}]}`, want: "duplicate"},
	}
	for _, tc := range cases {
		embeddedRuntimeSchemaExclusionsJSON = []byte(tc.data)
		if _, err := EmbeddedRuntimeSchemaExclusions(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("EmbeddedRuntimeSchemaExclusions(%s) error = %v, want %q", tc.data, err, tc.want)
		}
	}
	embeddedRuntimeSchemaExclusionsJSON = previous
}

func TestCrossPlatformCoverageCompletenessWrapperErrorBoundaries(t *testing.T) {
	oldLoad := completenessLoadExclusions
	oldApply := completenessApplyManual
	oldBuild := completenessBuildEffective
	oldBind := completenessBindEffective
	oldRuntime := completenessRuntimeReport
	oldDelivery := completenessDeliveryReport
	oldCollect := completenessCollectEntries
	t.Cleanup(func() {
		completenessLoadExclusions = oldLoad
		completenessApplyManual = oldApply
		completenessBuildEffective = oldBuild
		completenessBindEffective = oldBind
		completenessRuntimeReport = oldRuntime
		completenessDeliveryReport = oldDelivery
		completenessCollectEntries = oldCollect
	})
	root := &cobra.Command{Use: "dws"}
	completenessApplyManual = func(*cobra.Command) (ManualSchemaHintReport, error) {
		return ManualSchemaHintReport{}, errors.New("apply failed")
	}
	if err := ValidateEmbeddedRuntimeSchemaCompleteness(root); err == nil || !strings.Contains(err.Error(), "apply failed") {
		t.Fatalf("apply error = %v", err)
	}
	completenessApplyManual = func(*cobra.Command) (ManualSchemaHintReport, error) { return ManualSchemaHintReport{}, nil }
	completenessBuildEffective = func(*cobra.Command) (EffectiveCommandRegistry, error) {
		return EffectiveCommandRegistry{}, errors.New("build failed")
	}
	if err := ValidateEmbeddedRuntimeSchemaCompleteness(root); err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("build error = %v", err)
	}
	completenessBuildEffective = func(*cobra.Command) (EffectiveCommandRegistry, error) { return EffectiveCommandRegistry{}, nil }
	completenessBindEffective = func(*cobra.Command, EffectiveCommandRegistry) (BoundCommandRegistry, error) {
		return BoundCommandRegistry{}, errors.New("bind failed")
	}
	if err := ValidateEmbeddedRuntimeSchemaCompleteness(root); err == nil || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("bind error = %v", err)
	}

	completenessLoadExclusions = func() ([]RuntimeSchemaExclusion, error) { return nil, errors.New("exclusions failed") }
	if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), "exclusions failed") {
		t.Fatalf("resolved exclusions error = %v", err)
	}
	if err := validateResolvedSchemaCatalogDeliveryCompleteness(root, BoundCommandRegistry{}, SchemaCatalogSnapshot{}); err == nil || !strings.Contains(err.Error(), "exclusions failed") {
		t.Fatalf("delivery exclusions error = %v", err)
	}
	completenessLoadExclusions = func() ([]RuntimeSchemaExclusion, error) { return nil, nil }

	reports := []struct {
		report RuntimeSchemaCompletenessReport
		want   string
	}{
		{report: RuntimeSchemaCompletenessReport{DeliveryErrors: []string{"delivery"}}, want: "invalid effective"},
		{report: RuntimeSchemaCompletenessReport{Missing: []string{"missing"}}, want: "missing from Schema"},
		{report: RuntimeSchemaCompletenessReport{InvalidExclusions: []string{"invalid"}}, want: "invalid runtime"},
		{report: RuntimeSchemaCompletenessReport{StaleExclusions: []string{"stale"}}, want: "stale runtime"},
	}
	for _, tc := range reports {
		completenessRuntimeReport = func(*cobra.Command, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
			return tc.report
		}
		if err := validateResolvedRuntimeSchemaCompleteness(root, BoundCommandRegistry{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("runtime report %#v error = %v, want %q", tc.report, err, tc.want)
		}
	}

	badSnapshot := SchemaCatalogSnapshot{Catalog: map[string]any{"bad": func() {}}}
	if err := validateSchemaCatalogDeliveryCompletenessFromBound(root, BoundCommandRegistry{}, badSnapshot, nil); err == nil || !strings.Contains(err.Error(), "encode final") {
		t.Fatalf("delivery marshal error = %v", err)
	}
	invalidSnapshot := SchemaCatalogSnapshot{Version: 999, Catalog: map[string]any{}, Tools: map[string]map[string]any{}}
	if err := validateSchemaCatalogDeliveryCompletenessFromBound(root, BoundCommandRegistry{}, invalidSnapshot, nil); err == nil || !strings.Contains(err.Error(), "production loader") {
		t.Fatalf("delivery loader error = %v", err)
	}
	validLoaded := deliveryCoverageLoaded(t)
	for _, tc := range reports {
		completenessDeliveryReport = func(*cobra.Command, loadedSchemaCatalog, []RuntimeSchemaExclusion, BoundCommandRegistry) RuntimeSchemaCompletenessReport {
			return tc.report
		}
		err := validateSchemaCatalogDeliveryCompletenessFromBound(root, BoundCommandRegistry{}, validLoaded.Snapshot, nil)
		if err == nil {
			t.Errorf("delivery report %#v unexpectedly succeeded", tc.report)
		}
	}

	completenessCollectEntries = func(*cobra.Command) ([]runtimeSchemaEntry, error) { return nil, errors.New("collect failed") }
	report := RuntimeSchemaCompleteness(root, nil)
	if len(report.DeliveryErrors) != 1 || !strings.Contains(report.DeliveryErrors[0], "collect failed") {
		t.Fatalf("collect report = %#v", report)
	}
}

func TestCrossPlatformCoverageCatalogDeliveryCompletenessLoadedIdentityEdges(t *testing.T) {
	oldQuery := deliverySchemaPayload
	oldResolve := deliveryIndexResolve
	oldTool := deliveryToolPayload
	t.Cleanup(func() {
		deliverySchemaPayload = oldQuery
		deliveryIndexResolve = oldResolve
		deliveryToolPayload = oldTool
	})
	reset := func() {
		deliverySchemaPayload = oldQuery
		deliveryIndexResolve = oldResolve
		deliveryToolPayload = oldTool
	}
	root := commandRegistryTestRoot("sample category run", "extra leaf")
	loaded := deliveryCoverageLoaded(t)
	identities := map[string]runtimeSchemaResolvedIdentity{
		"sample category run":   {CanonicalPath: "sample.run", Source: "expected-source"},
		"sample legacy execute": {CanonicalPath: "sample.run", Source: "expected-source"},
	}

	// An unreviewed public leaf has no expected identity and is left to the
	// reverse-completeness report.
	schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample category run" {
			return nil, errors.New("query failed")
		}
		return oldQuery(loaded, args)
	}
	schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample category run" {
			return map[string]any{"canonical_path": "other.run"}, nil
		}
		return oldQuery(loaded, args)
	}
	report := schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "public CLI path") {
		t.Fatalf("wrong public canonical report = %#v", report)
	}
	reset()
	deliveryIndexResolve = func(SchemaIndex, string) (ToolSpec, bool) { return ToolSpec{}, false }
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "lost canonical") {
		t.Fatalf("lost canonical report = %#v", report)
	}
	reset()
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "identity source") {
		t.Fatalf("identity source report = %#v", report)
	}

	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample.run" {
			return nil, errors.New("canonical failed")
		}
		return oldQuery(loaded, args)
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "canonical \"sample.run\" is not queryable") {
		t.Fatalf("canonical query report = %#v", report)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample.run" {
			return map[string]any{"canonical_path": "other.run"}, nil
		}
		return oldQuery(loaded, args)
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "resolves to \"other.run\"") {
		t.Fatalf("canonical mismatch report = %#v", report)
	}
	reset()
	deliveryToolPayload = func(ToolSpec) (map[string]any, error) { return nil, errors.New("render failed") }
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "cannot render typed payload") {
		t.Fatalf("canonical render report = %#v", report)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		payload, err := oldQuery(loaded, args)
		if len(args) > 0 && args[0] == "sample.run" && err == nil {
			payload["title"] = "changed"
		}
		return payload, err
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "query differs from final ToolSpec") {
		t.Fatalf("canonical drift report = %#v", report)
	}
	reset()

	missing := map[string]runtimeSchemaResolvedIdentity{}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, missing, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "non-executable Schema path") {
		t.Fatalf("missing path report = %#v", report)
	}
	wrongOwner := map[string]runtimeSchemaResolvedIdentity{
		"sample category run":   {CanonicalPath: "sample.other"},
		"sample legacy execute": {CanonicalPath: "sample.other"},
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, wrongOwner, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "owned by sample.other") {
		t.Fatalf("wrong owner report = %#v", report)
	}

	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample legacy execute" {
			return nil, errors.New("path failed")
		}
		return oldQuery(loaded, args)
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "path \"sample legacy execute\" is not queryable") {
		t.Fatalf("path query report = %#v", report)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample legacy execute" {
			return map[string]any{"canonical_path": "other.run"}, nil
		}
		return oldQuery(loaded, args)
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "path \"sample legacy execute\" resolves") {
		t.Fatalf("path canonical report = %#v", report)
	}
	reset()
	deliveryToolPayload = func(tool ToolSpec) (map[string]any, error) {
		if tool.Identity.IsAlias {
			return nil, errors.New("alias render failed")
		}
		return oldTool(tool)
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "path \"sample legacy execute\" cannot render") {
		t.Fatalf("path render report = %#v", report)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		payload, err := oldQuery(loaded, args)
		if len(args) > 0 && args[0] == "sample legacy execute" && err == nil {
			payload["title"] = "changed"
		}
		return payload, err
	}
	report = schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
	if !strings.Contains(strings.Join(report.DeliveryErrors, ";"), "path \"sample legacy execute\" query differs") {
		t.Fatalf("path drift report = %#v", report)
	}
	reset()

	loaded.Index.registry.Products[0].Tools[0].Identity.Aliases = append(loaded.Index.registry.Products[0].Tools[0].Identity.Aliases, " ", "sample category run")
	schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, nil, identities, nil)
}

func TestCrossPlatformCoverageSchemaRegistryProjectionRemainingEdges(t *testing.T) {
	oldRegistry := deliveryRegistryPayload
	oldProduct := renderRegistryProductSummary
	oldQuery := deliverySchemaPayload
	oldTool := deliveryToolPayload
	oldSummary := deliveryToolSummary
	t.Cleanup(func() {
		deliveryRegistryPayload = oldRegistry
		renderRegistryProductSummary = oldProduct
		deliverySchemaPayload = oldQuery
		deliveryToolPayload = oldTool
		deliveryToolSummary = oldSummary
	})
	reset := func() {
		deliveryRegistryPayload = oldRegistry
		renderRegistryProductSummary = oldProduct
		deliverySchemaPayload = oldQuery
		deliveryToolPayload = oldTool
		deliveryToolSummary = oldSummary
	}
	loaded := deliveryCoverageLoaded(t)
	deliveryRegistryPayload = func(SchemaRegistry) (map[string]any, error) { return nil, errors.New("all failed") }
	if got := schemaRegistryProjectionErrors(loaded); len(got) == 0 || !strings.Contains(got[0], "render final Schema --all") {
		t.Fatalf("all render problems = %v", got)
	}
	reset()
	deliveryRegistryPayload = func(SchemaRegistry) (map[string]any, error) {
		return map[string]any{"products": []map[string]any{{"tools": []map[string]any{{}, {"canonical_path": "sample.run"}, {"canonical_path": "sample.run"}}}}}, nil
	}
	problems := schemaRegistryProjectionErrors(loaded)
	if joined := strings.Join(problems, ";"); !strings.Contains(joined, "without canonical_path") || !strings.Contains(joined, "duplicate tool") {
		t.Fatalf("all identity problems = %v", problems)
	}
	reset()
	renderRegistryProductSummary = func(ProductSpec) (map[string]any, error) { return nil, errors.New("product failed") }
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "render product") {
		t.Fatalf("product render problems = %s", joined)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample" {
			return nil, errors.New("product query failed")
		}
		return oldQuery(loaded, args)
	}
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "product sample is not queryable") {
		t.Fatalf("product query problems = %s", joined)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample" {
			return map[string]any{"product": map[string]any{}}, nil
		}
		return oldQuery(loaded, args)
	}
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "query differs from final ProductSpec") {
		t.Fatalf("product drift problems = %s", joined)
	}
	reset()
	deliveryToolPayload = func(ToolSpec) (map[string]any, error) { return nil, errors.New("tool failed") }
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "render final ToolSpec") {
		t.Fatalf("tool render problems = %s", joined)
	}
	reset()
	deliveryRegistryPayload = func(SchemaRegistry) (map[string]any, error) {
		return map[string]any{"products": []map[string]any{}}, nil
	}
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "Schema --all is missing") {
		t.Fatalf("missing all tool problems = %s", joined)
	}
	reset()
	deliveryRegistryPayload = func(registry SchemaRegistry) (map[string]any, error) {
		payload, err := oldRegistry(registry)
		products := schemaMapSlice(payload["products"])
		tools := schemaMapSlice(products[0]["tools"])
		tools[0]["title"] = "changed"
		products[0]["tools"] = tools
		payload["products"] = products
		return payload, err
	}
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "differs from final ToolSpec") {
		t.Fatalf("all tool drift problems = %s", joined)
	}
	reset()

	loaded.Registry.Products[0].Tools[0].Identity.CLIPath = "other group run"
	loaded.Registry.Products[0].Tools[0].Identity.PrimaryCLIPath = "other group run"
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "has no typed product") {
		t.Fatalf("missing group product problems = %s", joined)
	}
	loaded = deliveryCoverageLoaded(t)
	deliveryToolSummary = func(ToolSpec) (map[string]any, error) { return nil, errors.New("summary failed") }
	if joined := strings.Join(schemaRegistryProjectionErrors(loaded), ";"); !strings.Contains(joined, "render group") {
		t.Fatalf("group summary problems = %s", joined)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample category" {
			return nil, errors.New("group failed")
		}
		return oldQuery(loaded, args)
	}
	if joined := strings.Join(schemaRegistryProjectionErrors(deliveryCoverageLoaded(t)), ";"); !strings.Contains(joined, "group sample category is not queryable") {
		t.Fatalf("group query problems = %s", joined)
	}
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample category" {
			return map[string]any{"tools": []map[string]any{}}, nil
		}
		return oldQuery(loaded, args)
	}
	if joined := strings.Join(schemaRegistryProjectionErrors(deliveryCoverageLoaded(t)), ";"); !strings.Contains(joined, "query differs from final ToolSpec summaries") {
		t.Fatalf("group drift problems = %s", joined)
	}
}

func TestCrossPlatformCoverageCompletenessIdentityAndPathHelpersRemainingEdges(t *testing.T) {
	bound := BoundCommandRegistry{Commands: []BoundCommandSpec{
		{CommandSpec: CommandSpec{CanonicalPath: "sample.one", PrimaryCLIPath: "sample run", Visibility: SchemaVisibilityPublic}},
		{CommandSpec: CommandSpec{CanonicalPath: "sample.two", PrimaryCLIPath: "sample run", Visibility: SchemaVisibilityPublic}},
		{CommandSpec: CommandSpec{CanonicalPath: "hidden.tool", PrimaryCLIPath: "hidden run", Visibility: SchemaVisibilityInternal}},
		{CommandSpec: CommandSpec{CanonicalPath: "", PrimaryCLIPath: "empty run", Visibility: SchemaVisibilityPublic}},
	}}
	identities, conflicts := runtimeSchemaIdentityByBound(bound)
	if len(identities) != 1 || len(conflicts) != 1 || !strings.Contains(conflicts[0], "belongs to both") {
		t.Fatalf("identities=%#v conflicts=%v", identities, conflicts)
	}
	if got := sortedUniqueSchemaStrings([]string{" b ", "", "a", "b"}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("sorted unique = %v", got)
	}
	covered := map[string]bool{}
	addSchemaCoveredPath(covered, " dws sample run ")
	addSchemaCoveredPath(covered, " ")
	if !covered["sample run"] || len(covered) != 1 {
		t.Fatalf("covered paths = %#v", covered)
	}
	runtimeSchemaCompletenessFromBound(&cobra.Command{Use: "dws"}, nil, bound)
	emptyRoot := &cobra.Command{Use: "dws", Run: func(*cobra.Command, []string) {}}
	runtimeSchemaCompletenessAgainstPaths(emptyRoot, nil, nil)
	hiddenRoot := &cobra.Command{Use: "dws"}
	hiddenRoot.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, Run: func(*cobra.Command, []string) {}})
	walkPublicRunnableLeaves(hiddenRoot, func(*cobra.Command) { t.Fatal("hidden leaf visited") })
	walkPublicRunnableLeaves(nil, func(*cobra.Command) { t.Fatal("nil root visited") })
}
