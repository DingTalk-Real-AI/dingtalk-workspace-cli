// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// boundTestCommandRegistry adapts the small synthetic Cobra trees used by unit
// tests to the same explicit registry input as production. Annotation discovery
// lives only in this _test.go helper; production never falls back to it.
func boundTestCommandRegistry(root *cobra.Command) (BoundCommandRegistry, error) {
	grouped := map[string][]CommandSpec{}
	walkLeafCommands(root, func(leaf *cobra.Command) {
		if runtimeSchemaExcluded(leaf) {
			return
		}
		path := normalizeSchemaCLIPath(strings.Join(commandPathParts(leaf), " "))
		productID, toolName, source := runtimeSchemaAnnotations(leaf)
		reason := ""
		if productID == "" || toolName == "" {
			var ok bool
			productID, toolName, reason, ok = runtimeManualSchemaIdentity(leaf)
			if !ok {
				return
			}
			source = "reviewed_manual_hint"
		}
		canonical := productID + "." + toolName
		grouped[canonical] = append(grouped[canonical], CommandSpec{
			CanonicalPath:   canonical,
			SourceProductID: productID,
			PrimaryCLIPath:  path,
			Source:          defaultString(strings.TrimSpace(source), "test_registry"),
			ReviewReason:    reason,
		})
	})

	canonicals := make([]string, 0, len(grouped))
	for canonical := range grouped {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)
	specs := make([]CommandSpec, 0, len(canonicals))
	for _, canonical := range canonicals {
		paths := grouped[canonical]
		sort.Slice(paths, func(i, j int) bool { return paths[i].PrimaryCLIPath < paths[j].PrimaryCLIPath })
		primary := paths[0]
		for index := range paths {
			if index != 0 {
				primary.Aliases = append(primary.Aliases, paths[index].PrimaryCLIPath)
			}
		}
		specs = append(specs, primary)
	}
	effective, err := newEffectiveCommandRegistry(specs)
	if err != nil {
		return BoundCommandRegistry{}, err
	}
	return BindEffectiveCommandRegistry(root, effective)
}

func schemaRegistryForTest(root *cobra.Command) (SchemaRegistry, error) {
	bound, err := boundTestCommandRegistry(root)
	if err != nil {
		return SchemaRegistry{}, err
	}
	return AssembleSchemaRegistryFromBound(bound)
}

func schemaRegistryForTestWithMetadata(root *cobra.Command, agent embeddedAgentMetadata, mcp embeddedMCPMetadata) (SchemaRegistry, error) {
	bound, err := boundTestCommandRegistry(root)
	if err != nil {
		return SchemaRegistry{}, err
	}
	return assembleSchemaRegistryFromBound(bound, runtimeSchemaMetadataSources{Agent: agent, MCP: mcp})
}

func runtimeSchemaPayloadForTest(root *cobra.Command, args []string) (map[string]any, error) {
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		return nil, err
	}
	return runtimeSchemaPayloadFromRegistry(registry, args)
}

func runtimeSchemaPayloadForTestWithMetadata(root *cobra.Command, args []string, agent embeddedAgentMetadata, mcp embeddedMCPMetadata) (map[string]any, error) {
	registry, err := schemaRegistryForTestWithMetadata(root, agent, mcp)
	if err != nil {
		return nil, err
	}
	return runtimeSchemaPayloadFromRegistry(registry, args)
}

func runtimeSchemaAllPayloadForTest(root *cobra.Command) (map[string]any, error) {
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		return nil, err
	}
	return runtimeSchemaAllPayloadFromRegistry(registry)
}

func runtimeSchemaCompletenessForTest(root *cobra.Command, exclusions []RuntimeSchemaExclusion) RuntimeSchemaCompletenessReport {
	bound, err := boundTestCommandRegistry(root)
	if err != nil {
		return RuntimeSchemaCompletenessReport{DeliveryErrors: []string{err.Error()}}
	}
	covered := map[string]bool{}
	for _, command := range bound.Commands {
		addSchemaCoveredPath(covered, command.PrimaryCLIPath)
		for _, alias := range command.Aliases {
			addSchemaCoveredPath(covered, alias)
		}
	}
	return runtimeSchemaCompletenessAgainstPaths(root, exclusions, covered)
}

func testRegistryIdentityByCLIPath(root *cobra.Command) (map[string]runtimeSchemaResolvedIdentity, []string) {
	bound, err := boundTestCommandRegistry(root)
	if err != nil {
		return map[string]runtimeSchemaResolvedIdentity{}, []string{err.Error()}
	}
	result := map[string]runtimeSchemaResolvedIdentity{}
	for _, command := range bound.Commands {
		for _, path := range append([]string{command.PrimaryCLIPath}, command.Aliases...) {
			result[normalizeSchemaCLIPath(path)] = runtimeSchemaResolvedIdentity{
				CanonicalPath: command.CanonicalPath,
				Source:        command.Source,
			}
		}
	}
	return result, nil
}

func schemaCatalogDeliveryCompletenessForTest(root *cobra.Command, snapshot SchemaCatalogSnapshot, exclusions []RuntimeSchemaExclusion) RuntimeSchemaCompletenessReport {
	loaded, err := loadSchemaCatalogSnapshot(snapshot)
	if err != nil {
		return RuntimeSchemaCompletenessReport{DeliveryErrors: []string{err.Error()}}
	}
	expected, mappingErrors := testRegistryIdentityByCLIPath(root)
	return schemaCatalogDeliveryCompletenessAgainstLoadedAndIdentity(root, loaded, exclusions, expected, mappingErrors)
}

func validateSchemaCatalogDeliveryCompletenessForTest(root *cobra.Command, snapshot SchemaCatalogSnapshot, exclusions []RuntimeSchemaExclusion) error {
	report := schemaCatalogDeliveryCompletenessForTest(root, snapshot, exclusions)
	if len(report.DeliveryErrors) > 0 {
		return fmt.Errorf("invalid final Schema Catalog delivery: %s", strings.Join(report.DeliveryErrors, "; "))
	}
	if len(report.Missing) > 0 {
		return fmt.Errorf("public Cobra leaves missing from final Schema Catalog or reviewed exclusions: %s", strings.Join(report.Missing, ", "))
	}
	return nil
}
