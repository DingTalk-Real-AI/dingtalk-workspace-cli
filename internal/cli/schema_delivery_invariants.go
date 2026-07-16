// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var (
	deliveryRegistryPayload = SchemaRegistry.ToPayload
	deliverySchemaPayload   = schemaPayloadFromLoadedCatalog
	deliveryOverviewPayload = SchemaRegistry.ToOverviewPayload
	deliveryIndexResolve    = SchemaIndex.Resolve
	deliveryToolPayload     = ToolSpec.ToPayload
	deliveryToolSummary     = ToolSpec.ToSummaryPayload
)

// ValidateSchemaDeliveryInvariants proves that the serialized snapshot which
// will be embedded in the release binary is an exact delivery of source and
// has one content-identical ToolSpec behind every public Schema view. The
// snapshot is deliberately encoded and decoded through the production loader
// before any comparison is made.
//
// This is a content gate, not a count gate: replacing one tool with another,
// consistently dropping a source field from every snapshot view, drifting a
// summary, or changing an alias payload while preserving all aggregate counts
// still fails.
func ValidateSchemaDeliveryInvariants(source SchemaRegistry, snapshot SchemaCatalogSnapshot) error {
	problems := schemaSourceSnapshotInvariantErrors(source, snapshot)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode final Schema Catalog for invariant validation: %w", err)
	}
	loaded, err := decodeSchemaCatalogSnapshot(encoded)
	if err != nil {
		return fmt.Errorf("load final Schema Catalog through production loader: %w", err)
	}
	problems = append(problems, schemaSourceDeliveryInvariantErrors(source, loaded.Registry)...)
	problems = append(problems, schemaDeliveryInvariantErrors(loaded)...)
	if problems = sortedUniqueSchemaStrings(problems); len(problems) > 0 {
		return fmt.Errorf("invalid final Schema delivery invariants: %s", strings.Join(problems, "; "))
	}
	return nil
}

// schemaSourceSnapshotInvariantErrors makes the pre-serialization -> raw
// snapshot boundary explicit. This catches mutation after rendering and gives
// a precise Catalog/full-tool diagnostic. The separate source -> decoded
// typed comparison below is still required because it catches a serializer
// that consistently omits the same field from every raw view.
func schemaSourceSnapshotInvariantErrors(source SchemaRegistry, snapshot SchemaCatalogSnapshot) []string {
	expected, err := source.ToSnapshotPayload()
	if err != nil {
		return []string{fmt.Sprintf("render source SchemaRegistry snapshot: %v", err)}
	}
	var problems []string
	if !schemaJSONEqual(expected.Catalog, snapshot.Catalog) {
		problems = append(problems, "raw Catalog differs from source SchemaRegistry projection")
	}
	if !schemaJSONEqual(expected.Tools, snapshot.Tools) {
		problems = append(problems, "raw Catalog full tools differ from source SchemaRegistry projection")
	}
	return problems
}

// validateSchemaSnapshotDeliveryInvariants validates only relationships among
// delivered snapshot views. It is useful for loader-focused tests, but is not
// a generation gate because an internally consistent snapshot can still have
// consistently lost or changed source fields.
func validateSchemaSnapshotDeliveryInvariants(snapshot SchemaCatalogSnapshot) error {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode final Schema Catalog for invariant validation: %w", err)
	}
	loaded, err := decodeSchemaCatalogSnapshot(encoded)
	if err != nil {
		return fmt.Errorf("load final Schema Catalog through production loader: %w", err)
	}
	if problems := schemaDeliveryInvariantErrors(loaded); len(problems) > 0 {
		return fmt.Errorf("invalid final Schema delivery invariants: %s", strings.Join(problems, "; "))
	}
	return nil
}

// schemaSourceDeliveryInvariantErrors compares the complete normalized typed
// source with the complete typed registry reconstructed by the production
// decoder. Comparing typed registries (rather than re-rendering source through
// ToSnapshotPayload) is what detects a serializer that consistently omits or
// mutates a field in both Catalog summaries and full tools.
func schemaSourceDeliveryInvariantErrors(source, delivered SchemaRegistry) []string {
	sourceIndex, err := source.Index()
	if err != nil {
		return []string{fmt.Sprintf("normalize source SchemaRegistry: %v", err)}
	}
	deliveredIndex, err := delivered.Index()
	if err != nil {
		return []string{fmt.Sprintf("normalize production-decoded SchemaRegistry: %v", err)}
	}
	if !schemaJSONEqual(sourceIndex.Registry(), deliveredIndex.Registry()) {
		path, left, right := firstSchemaJSONDifference(sourceIndex.Registry(), deliveredIndex.Registry())
		return []string{fmt.Sprintf("production-decoded snapshot differs from complete source SchemaRegistry at %s: source=%s delivered=%s", path, left, right)}
	}
	return nil
}

func firstSchemaJSONDifference(left, right any) (path, leftValue, rightValue string) {
	decode := func(value any) any {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("<encode error: %v>", err)
		}
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(string(encoded)))
		decoder.UseNumber()
		_ = decoder.Decode(&decoded)
		return decoded
	}
	path, leftDecoded, rightDecoded, _ := firstSchemaJSONDifferenceAt("$", decode(left), decode(right))
	return path, compactSchemaDiagnosticValue(leftDecoded), compactSchemaDiagnosticValue(rightDecoded)
}

func firstSchemaJSONDifferenceAt(path string, left, right any) (string, any, any, bool) {
	leftMap, leftIsMap := left.(map[string]any)
	rightMap, rightIsMap := right.(map[string]any)
	if leftIsMap || rightIsMap {
		if !leftIsMap || !rightIsMap {
			return path, left, right, true
		}
		keys := make([]string, 0, len(leftMap)+len(rightMap))
		seen := map[string]bool{}
		for key := range leftMap {
			seen[key] = true
			keys = append(keys, key)
		}
		for key := range rightMap {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			leftChild, leftOK := leftMap[key]
			rightChild, rightOK := rightMap[key]
			if !leftOK || !rightOK {
				return path + "." + key, leftChild, rightChild, true
			}
			if childPath, childLeft, childRight, different := firstSchemaJSONDifferenceAt(path+"."+key, leftChild, rightChild); different {
				return childPath, childLeft, childRight, true
			}
		}
		return path, left, right, false
	}
	leftList, leftIsList := left.([]any)
	rightList, rightIsList := right.([]any)
	if leftIsList || rightIsList {
		if !leftIsList || !rightIsList || len(leftList) != len(rightList) {
			return path, left, right, true
		}
		for index := range leftList {
			if childPath, childLeft, childRight, different := firstSchemaJSONDifferenceAt(fmt.Sprintf("%s[%d]", path, index), leftList[index], rightList[index]); different {
				return childPath, childLeft, childRight, true
			}
		}
		return path, left, right, false
	}
	if !reflect.DeepEqual(left, right) {
		return path, left, right, true
	}
	return path, left, right, false
}

func compactSchemaDiagnosticValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	const limit = 240
	if len(encoded) > limit {
		return string(encoded[:limit]) + "..."
	}
	return string(encoded)
}

// schemaDeliveryInvariantErrors checks all projections available after the
// production loader boundary. Callers that already performed the round trip
// can use it without decoding the snapshot a second time.
func schemaDeliveryInvariantErrors(loaded loadedSchemaCatalog) []string {
	problems := append([]string(nil), schemaRegistryProjectionErrors(loaded)...)

	all, err := deliveryRegistryPayload(loaded.Registry)
	if err != nil {
		return sortedUniqueSchemaStrings(append(problems, fmt.Sprintf("render final Schema --all projection: %v", err)))
	}
	allTools, allProblems := schemaDeliveryToolsByCanonical(all, "Schema --all")
	problems = append(problems, allProblems...)
	catalogSummaries, summaryProblems := schemaDeliveryToolsByCanonical(loaded.Snapshot.Catalog, "Catalog summary")
	problems = append(problems, summaryProblems...)

	// The queryable Catalog must preserve the complete serialized Catalog,
	// including source and metadata summaries. catalog_hash and surface_hash
	// are the only allowed view-only fields because they belong to the release
	// envelope rather than SchemaRegistry itself.
	if list, queryErr := deliverySchemaPayload(loaded, nil); queryErr != nil {
		problems = append(problems, fmt.Sprintf("Schema list is not queryable: %v", queryErr))
	} else {
		if !schemaJSONEqual(schemaCatalogWithoutEnvelopeHashes(list), loaded.Snapshot.Catalog) {
			problems = append(problems, "Schema list differs from complete Catalog content")
		}
		if got := strings.TrimSpace(schemaString(list["catalog_hash"])); got != loaded.Snapshot.SourceHash {
			problems = append(problems, fmt.Sprintf("Schema list catalog_hash %q differs from snapshot source_hash %q", got, loaded.Snapshot.SourceHash))
		}
		gotSurface, hasSurface := list["surface_hash"]
		if expected := strings.TrimSpace(loaded.Snapshot.SurfaceHash); expected == "" {
			if hasSurface {
				problems = append(problems, "Schema list has surface_hash but snapshot envelope does not")
			}
		} else if got := strings.TrimSpace(schemaString(gotSurface)); !hasSurface || got != expected {
			problems = append(problems, fmt.Sprintf("Schema list surface_hash %q differs from snapshot surface_hash %q", got, expected))
		}
	}
	if overview, overviewErr := deliveryOverviewPayload(loaded.Registry); overviewErr != nil {
		problems = append(problems, fmt.Sprintf("render final Schema product overview: %v", overviewErr))
	} else if expectedOverview := schemaOverviewPayloadFromCatalog(loaded.Snapshot.Catalog); !schemaJSONEqual(overview, expectedOverview) {
		problems = append(problems, "Schema list overview differs from complete Catalog content")
	}

	canonicals := loaded.Index.CanonicalPaths()
	for _, canonical := range canonicals {
		tool, ok := deliveryIndexResolve(loaded.Index, canonical)
		if !ok {
			problems = append(problems, fmt.Sprintf("typed Schema index lost canonical %q", canonical))
			continue
		}
		expectedFull, renderErr := deliveryToolPayload(tool)
		if renderErr != nil {
			problems = append(problems, fmt.Sprintf("render final ToolSpec %s: %v", canonical, renderErr))
			continue
		}
		storedFull, stored := loaded.Snapshot.Tools[canonical]
		if !stored {
			problems = append(problems, fmt.Sprintf("Catalog full tools are missing %s", canonical))
		} else if !schemaJSONEqual(storedFull, expectedFull) {
			problems = append(problems, fmt.Sprintf("Catalog full tool %s differs from final ToolSpec", canonical))
		}
		if allFull, exists := allTools[canonical]; !exists {
			problems = append(problems, fmt.Sprintf("Schema --all is missing final ToolSpec %s", canonical))
		} else if !schemaJSONEqual(allFull, expectedFull) {
			problems = append(problems, fmt.Sprintf("Schema --all tool %s differs from final ToolSpec", canonical))
		}
		if leaf, queryErr := deliverySchemaPayload(loaded, []string{canonical}); queryErr != nil {
			problems = append(problems, fmt.Sprintf("canonical leaf %s is not queryable: %v", canonical, queryErr))
		} else if !schemaJSONEqual(leaf, expectedFull) {
			problems = append(problems, fmt.Sprintf("canonical leaf %s differs from final ToolSpec", canonical))
		}

		expectedSummary, summaryErr := deliveryToolSummary(tool)
		if summaryErr != nil {
			problems = append(problems, fmt.Sprintf("render final ToolSpec summary %s: %v", canonical, summaryErr))
		} else if catalogSummary, exists := catalogSummaries[canonical]; !exists {
			problems = append(problems, fmt.Sprintf("Catalog summary is missing final ToolSpec %s", canonical))
		} else if !schemaJSONEqual(catalogSummary, expectedSummary) {
			problems = append(problems, fmt.Sprintf("Catalog summary %s differs from final ToolSpec summary", canonical))
		}

		for _, rawAlias := range tool.Identity.Aliases {
			alias := normalizeSchemaCLIPath(rawAlias)
			if alias == "" {
				continue
			}
			aliasPayload, queryErr := deliverySchemaPayload(loaded, []string{alias})
			if queryErr != nil {
				problems = append(problems, fmt.Sprintf("alias %q for %s is not queryable: %v", alias, canonical, queryErr))
				continue
			}
			if problem := schemaAliasViewProblem(expectedFull, aliasPayload, alias); problem != "" {
				problems = append(problems, fmt.Sprintf("alias %q for %s %s", alias, canonical, problem))
			}
		}
	}

	if len(allTools) != len(canonicals) {
		problems = append(problems, fmt.Sprintf("Schema --all contains %d tools, typed index contains %d", len(allTools), len(canonicals)))
	}
	if len(catalogSummaries) != len(canonicals) {
		problems = append(problems, fmt.Sprintf("Catalog summary contains %d tools, typed index contains %d", len(catalogSummaries), len(canonicals)))
	}
	if len(loaded.Snapshot.Tools) != len(canonicals) {
		problems = append(problems, fmt.Sprintf("Catalog full tools contain %d tools, typed index contains %d", len(loaded.Snapshot.Tools), len(canonicals)))
	}
	return sortedUniqueSchemaStrings(problems)
}

// schemaOverviewPayloadFromCatalog independently projects the compact
// `schema list` view from the raw Catalog boundary. The production command
// renders its overview from the decoded SchemaRegistry, so comparing the two
// directions proves that list did not silently lose or re-resolve content.
func schemaOverviewPayloadFromCatalog(catalog map[string]any) map[string]any {
	products := schemaMapSlice(catalog["products"])
	overviewProducts := make([]map[string]any, 0, len(products))
	toolCount := 0
	for _, product := range products {
		productToolCount := len(schemaMapSlice(product["tools"]))
		entry := map[string]any{
			"id":          schemaString(product["id"]),
			"tool_count":  productToolCount,
			"schema_path": schemaString(product["id"]),
		}
		switch {
		case strings.TrimSpace(schemaString(product["agent_summary"])) != "":
			entry["agent_summary"] = schemaString(product["agent_summary"])
		case len(schemaStringSlice(product["use_when"])) > 0:
			entry["use_when"] = []string{schemaStringSlice(product["use_when"])[0]}
		case schemaString(product["description"]) != "":
			entry["description"] = schemaString(product["description"])
		}
		overviewProducts = append(overviewProducts, entry)
		toolCount += productToolCount
	}
	payload := map[string]any{
		"kind":       defaultString(schemaString(catalog["kind"]), "schema"),
		"level":      "products",
		"count":      len(overviewProducts),
		"tool_count": toolCount,
		"products":   overviewProducts,
	}
	if source := schemaString(catalog["source"]); source != "" {
		payload["source"] = source
	}
	for _, key := range []string{"interface_metadata", "agent_metadata"} {
		if value, exists := catalog[key]; exists {
			payload[key] = value
		}
	}
	return payload
}

func schemaCatalogWithoutEnvelopeHashes(payload map[string]any) map[string]any {
	content := make(map[string]any, len(payload))
	for key, value := range payload {
		switch key {
		case "catalog_hash", "surface_hash":
			continue
		default:
			content[key] = value
		}
	}
	return content
}

// schemaDeliveryToolsByCanonical extracts either full or summary tool payloads
// from a products/tools projection and reports duplicate or unkeyed entries.
func schemaDeliveryToolsByCanonical(payload map[string]any, view string) (map[string]map[string]any, []string) {
	tools := map[string]map[string]any{}
	var problems []string
	for _, product := range schemaMapSlice(payload["products"]) {
		for _, tool := range schemaMapSlice(product["tools"]) {
			canonical := strings.TrimSpace(schemaString(tool["canonical_path"]))
			if canonical == "" {
				problems = append(problems, fmt.Sprintf("%s contains a tool without canonical_path", view))
				continue
			}
			if _, exists := tools[canonical]; exists {
				problems = append(problems, fmt.Sprintf("%s contains duplicate tool %s", view, canonical))
				continue
			}
			tools[canonical] = tool
		}
	}
	return tools, problems
}

// schemaAliasViewProblem enforces that resolving an alias is a view change,
// not a second ToolSpec. Only cli_path and is_alias may differ.
func schemaAliasViewProblem(canonical, alias map[string]any, expectedPath string) string {
	if got := normalizeSchemaCLIPath(schemaString(alias["cli_path"])); got != normalizeSchemaCLIPath(expectedPath) {
		return fmt.Sprintf("has cli_path %q, want %q", got, normalizeSchemaCLIPath(expectedPath))
	}
	if isAlias, ok := alias["is_alias"].(bool); !ok || !isAlias {
		return "does not set is_alias=true"
	}
	canonicalContent := schemaPayloadWithoutAliasView(canonical)
	aliasContent := schemaPayloadWithoutAliasView(alias)
	if !schemaJSONEqual(canonicalContent, aliasContent) {
		return "changes fields other than cli_path/is_alias"
	}
	return ""
}

func schemaPayloadWithoutAliasView(payload map[string]any) map[string]any {
	copy := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "cli_path" || key == "is_alias" {
			continue
		}
		copy[key] = value
	}
	return copy
}
