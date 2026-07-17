// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package cli

import (
	"errors"
	"strings"
	"testing"
)

func deliveryCoverageLoaded(t *testing.T) loadedSchemaCatalog {
	t.Helper()
	snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
		Canonical: "sample.run",
		CLIPath:   "sample category run",
		Aliases:   []string{"sample legacy execute"},
	})
	loaded, err := loadSchemaCatalogSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func requireDeliveryProblem(t *testing.T, loaded loadedSchemaCatalog, want string) {
	t.Helper()
	problems := schemaDeliveryInvariantErrors(loaded)
	if !strings.Contains(strings.Join(problems, "; "), want) {
		t.Fatalf("problems = %v, want containing %q", problems, want)
	}
}

func TestCrossPlatformCoverageSchemaDeliveryInvariantBoundaryErrors(t *testing.T) {
	badSnapshot := SchemaCatalogSnapshot{Catalog: map[string]any{"bad": func() {}}}
	if err := ValidateSchemaDeliveryInvariants(SchemaRegistry{}, badSnapshot); err == nil || !strings.Contains(err.Error(), "encode final") {
		t.Fatalf("ValidateSchemaDeliveryInvariants() marshal error = %v", err)
	}
	if err := validateSchemaSnapshotDeliveryInvariants(badSnapshot); err == nil || !strings.Contains(err.Error(), "encode final") {
		t.Fatalf("snapshot invariant marshal error = %v", err)
	}
	invalidSnapshot := SchemaCatalogSnapshot{Version: 999, Catalog: map[string]any{}, Tools: map[string]map[string]any{}}
	if err := ValidateSchemaDeliveryInvariants(SchemaRegistry{}, invalidSnapshot); err == nil || !strings.Contains(err.Error(), "production loader") {
		t.Fatalf("ValidateSchemaDeliveryInvariants() loader error = %v", err)
	}
	if err := validateSchemaSnapshotDeliveryInvariants(invalidSnapshot); err == nil || !strings.Contains(err.Error(), "production loader") {
		t.Fatalf("snapshot invariant loader error = %v", err)
	}
	oldRegistryPayload := deliveryRegistryPayload
	t.Cleanup(func() { deliveryRegistryPayload = oldRegistryPayload })
	deliveryRegistryPayload = func(SchemaRegistry) (map[string]any, error) { return nil, errors.New("projection failed") }
	if err := validateSchemaSnapshotDeliveryInvariants(deliveryCoverageLoaded(t).Snapshot); err == nil || !strings.Contains(err.Error(), "invalid final Schema delivery invariants") {
		t.Fatalf("snapshot projection error = %v", err)
	}
	deliveryRegistryPayload = oldRegistryPayload
	if problems := schemaSourceSnapshotInvariantErrors(SchemaRegistry{Products: []ProductSpec{{ID: " "}}}, SchemaCatalogSnapshot{}); len(problems) == 0 || !strings.Contains(problems[0], "render source") {
		t.Fatalf("source snapshot problems = %v", problems)
	}
	if problems := schemaSourceDeliveryInvariantErrors(SchemaRegistry{Products: []ProductSpec{{ID: " "}}}, SchemaRegistry{}); len(problems) == 0 || !strings.Contains(problems[0], "normalize source") {
		t.Fatalf("source normalization problems = %v", problems)
	}
	if problems := schemaSourceDeliveryInvariantErrors(SchemaRegistry{}, SchemaRegistry{Products: []ProductSpec{{ID: " "}}}); len(problems) == 0 || !strings.Contains(problems[0], "production-decoded") {
		t.Fatalf("delivered normalization problems = %v", problems)
	}
	left, _ := validSnapshotAdapterFixture(t)
	right := left
	right.Source = "other"
	if problems := schemaSourceDeliveryInvariantErrors(left, right); len(problems) == 0 || !strings.Contains(problems[0], "differs from complete source") {
		t.Fatalf("typed difference problems = %v", problems)
	}
}

func TestCrossPlatformCoverageSchemaJSONDifferenceRemainingShapes(t *testing.T) {
	path, left, _ := firstSchemaJSONDifference(func() {}, map[string]any{})
	if path != "$" || !strings.Contains(left, "encode error") {
		t.Fatalf("encode difference = %q %q", path, left)
	}
	cases := []struct {
		left  any
		right any
		path  string
		diff  bool
	}{
		{left: map[string]any{"a": 1}, right: []any{1}, path: "$", diff: true},
		{left: map[string]any{"a": 1}, right: map[string]any{"b": 1}, path: "$.a", diff: true},
		{left: map[string]any{"a": map[string]any{"b": 1}}, right: map[string]any{"a": map[string]any{"b": 2}}, path: "$.a.b", diff: true},
		{left: map[string]any{"a": 1}, right: map[string]any{"a": 1}, path: "$", diff: false},
		{left: []any{1}, right: map[string]any{}, path: "$", diff: true},
		{left: []any{1}, right: []any{1, 2}, path: "$", diff: true},
		{left: []any{map[string]any{"a": 1}}, right: []any{map[string]any{"a": 2}}, path: "$[0].a", diff: true},
		{left: []any{1}, right: []any{1}, path: "$", diff: false},
		{left: 1, right: 2, path: "$", diff: true},
	}
	for _, tc := range cases {
		path, _, _, different := firstSchemaJSONDifferenceAt("$", tc.left, tc.right)
		if path != tc.path || different != tc.diff {
			t.Errorf("difference(%#v, %#v) = %q, %v; want %q, %v", tc.left, tc.right, path, different, tc.path, tc.diff)
		}
	}
	if got := compactSchemaDiagnosticValue(func() {}); !strings.Contains(got, "0x") {
		t.Fatalf("compact unsupported value = %q", got)
	}
	if got := compactSchemaDiagnosticValue(strings.Repeat("x", 300)); len(got) != 243 || !strings.HasSuffix(got, "...") {
		t.Fatalf("compact long value length = %d, value = %q", len(got), got)
	}
}

func TestCrossPlatformCoverageSchemaDeliveryInvariantProjectionEdges(t *testing.T) {
	oldRegistry := deliveryRegistryPayload
	oldQuery := deliverySchemaPayload
	oldOverview := deliveryOverviewPayload
	oldResolve := deliveryIndexResolve
	oldTool := deliveryToolPayload
	oldSummary := deliveryToolSummary
	t.Cleanup(func() {
		deliveryRegistryPayload = oldRegistry
		deliverySchemaPayload = oldQuery
		deliveryOverviewPayload = oldOverview
		deliveryIndexResolve = oldResolve
		deliveryToolPayload = oldTool
		deliveryToolSummary = oldSummary
	})
	reset := func() {
		deliveryRegistryPayload = oldRegistry
		deliverySchemaPayload = oldQuery
		deliveryOverviewPayload = oldOverview
		deliveryIndexResolve = oldResolve
		deliveryToolPayload = oldTool
		deliveryToolSummary = oldSummary
	}

	deliveryRegistryPayload = func(SchemaRegistry) (map[string]any, error) { return nil, errors.New("all failed") }
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "render final Schema --all")
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) == 0 {
			return nil, errors.New("list failed")
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "Schema list is not queryable")
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) == 0 {
			return map[string]any{"catalog_hash": "wrong", "surface_hash": "unexpected"}, nil
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "Schema list differs from complete Catalog")
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "catalog_hash")
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "has surface_hash")
	reset()
	loaded := deliveryCoverageLoaded(t)
	loaded.Snapshot.SurfaceHash = "expected"
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) == 0 {
			payload, err := oldQuery(loaded, args)
			delete(payload, "surface_hash")
			return payload, err
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, loaded, "differs from snapshot surface_hash")
	reset()
	deliveryOverviewPayload = func(SchemaRegistry) (map[string]any, error) { return nil, errors.New("overview failed") }
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "render final Schema product overview")
	reset()
	deliveryOverviewPayload = func(SchemaRegistry) (map[string]any, error) { return map[string]any{}, nil }
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "overview differs")
	reset()
	deliveryIndexResolve = func(SchemaIndex, string) (ToolSpec, bool) { return ToolSpec{}, false }
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "typed Schema index lost canonical")
	reset()
	deliveryToolPayload = func(ToolSpec) (map[string]any, error) { return nil, errors.New("tool failed") }
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "render final ToolSpec sample.run")
	reset()

	loaded = deliveryCoverageLoaded(t)
	delete(loaded.Snapshot.Tools, "sample.run")
	requireDeliveryProblem(t, loaded, "Catalog full tools are missing")
	loaded = deliveryCoverageLoaded(t)
	loaded.Snapshot.Tools["sample.run"]["title"] = "changed"
	requireDeliveryProblem(t, loaded, "Catalog full tool sample.run differs")

	deliveryRegistryPayload = func(registry SchemaRegistry) (map[string]any, error) {
		payload, err := oldRegistry(registry)
		products := schemaMapSlice(payload["products"])
		products[0]["tools"] = []map[string]any{}
		payload["products"] = products
		return payload, err
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "Schema --all is missing")
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
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "Schema --all tool sample.run differs")
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample.run" {
			return nil, errors.New("leaf failed")
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "canonical leaf sample.run is not queryable")
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample.run" {
			return map[string]any{"canonical_path": "sample.run"}, nil
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "canonical leaf sample.run differs")
	reset()
	deliveryToolSummary = func(ToolSpec) (map[string]any, error) { return nil, errors.New("summary failed") }
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "render final ToolSpec summary")
	reset()
	loaded = deliveryCoverageLoaded(t)
	products := schemaMapSlice(loaded.Snapshot.Catalog["products"])
	products[0]["tools"] = []map[string]any{}
	loaded.Snapshot.Catalog["products"] = products
	requireDeliveryProblem(t, loaded, "Catalog summary is missing")
	loaded = deliveryCoverageLoaded(t)
	products = schemaMapSlice(loaded.Snapshot.Catalog["products"])
	tools := schemaMapSlice(products[0]["tools"])
	tools[0]["title"] = "changed"
	products[0]["tools"] = tools
	loaded.Snapshot.Catalog["products"] = products
	requireDeliveryProblem(t, loaded, "Catalog summary sample.run differs")

	loaded = deliveryCoverageLoaded(t)
	loaded.Index.registry.Products[0].Tools[0].Identity.Aliases = append(loaded.Index.registry.Products[0].Tools[0].Identity.Aliases, " ")
	schemaDeliveryInvariantErrors(loaded)
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample legacy execute" {
			return nil, errors.New("alias failed")
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "alias \"sample legacy execute\" for sample.run is not queryable")
	reset()
	deliverySchemaPayload = func(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
		if len(args) > 0 && args[0] == "sample legacy execute" {
			return map[string]any{"cli_path": "wrong", "is_alias": true}, nil
		}
		return oldQuery(loaded, args)
	}
	requireDeliveryProblem(t, deliveryCoverageLoaded(t), "has cli_path")
	reset()

	loaded = deliveryCoverageLoaded(t)
	loaded.Snapshot.Tools["extra"] = map[string]any{}
	requireDeliveryProblem(t, loaded, "Catalog full tools contain 2 tools")
}

func TestCrossPlatformCoverageSchemaDeliveryToolExtractionAndAliasPathEdge(t *testing.T) {
	payload := map[string]any{"products": []map[string]any{{"tools": []map[string]any{{"title": "missing"}}}}}
	_, problems := schemaDeliveryToolsByCanonical(payload, "view")
	if len(problems) != 1 || !strings.Contains(problems[0], "without canonical_path") {
		t.Fatalf("problems = %v", problems)
	}
	canonical := map[string]any{"cli_path": "sample run", "is_alias": false, "title": "Run"}
	alias := map[string]any{"cli_path": "wrong", "is_alias": true, "title": "Run"}
	if problem := schemaAliasViewProblem(canonical, alias, "sample execute"); !strings.Contains(problem, "has cli_path") {
		t.Fatalf("alias path problem = %q", problem)
	}
}
