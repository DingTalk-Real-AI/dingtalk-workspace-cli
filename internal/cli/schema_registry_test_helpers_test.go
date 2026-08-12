// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
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
		if productID == "" || toolName == "" {
			return
		}
		canonical := productID + "." + toolName
		grouped[canonical] = append(grouped[canonical], CommandSpec{
			CanonicalPath:   canonical,
			SourceProductID: productID,
			PrimaryCLIPath:  path,
			Source:          defaultString(strings.TrimSpace(source), "test_registry"),
			ReviewReason:    "test fixture native identity",
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

// declareRuntimeSchemaTestRootDoc registers the ContractFinal / ProductDecl
// declarations for the synthetic doc.create_document tree built by
// buildRuntimeSchemaTestRoot, so production-shaped assembly can resolve it.
// Declarations are removed again through t.Cleanup.
func declareRuntimeSchemaTestRootDoc(t *testing.T, root *cobra.Command, mutate func(*contract.ContractFinalPayload)) {
	t.Helper()
	create, _, err := root.Find([]string{"doc", "create"})
	if err != nil {
		t.Fatalf("locate doc create leaf: %v", err)
	}
	payload := contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "doc", Name: "create_document", CanonicalPath: "doc.create_document",
			CLIPath: "doc create", PrimaryCLIPath: "doc create",
		},
		Title:       "Create document",
		Description: "Create a DingTalk document",
		Safety: &contract.SafetySpec{
			Effect: "write", EffectSource: "command-verb", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Interface: &contract.InterfaceSpec{
			Mode: "local", Availability: "available", Reason: "test local implementation",
		},
		Selection: &contract.SelectionSpec{
			AgentSummary: "新建钉钉文档",
			UseWhen:      []string{"新建文档"},
			AvoidWhen:    []string{"只需读取文档时"},
			Examples:     []string{"dws doc create --title test"},
		},
	}
	if mutate != nil {
		mutate(&payload)
	}
	contractfinal.RegisterRuntimeContractFinal(create, payload)
	t.Cleanup(func() { contractfinal.ClearRuntimeContractFinalForTest(create) })
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "doc",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "创建、读取和维护钉钉文档",
			UseWhen:      []string{"需要创建或读取文档"},
			AvoidWhen:    []string{"管理表格"},
		},
	})
	t.Cleanup(func() { contract.ClearProductDeclForTest("doc") })
}

// loadedSchemaCatalogForTestRegistry wraps an assembled test registry in the
// production loadedSchemaCatalog shape so payload helpers exercise the same
// query path as delivery (schemaPayloadFromLoadedCatalog).
func loadedSchemaCatalogForTestRegistry(registry SchemaRegistry) (loadedSchemaCatalog, error) {
	index, err := registry.Index()
	if err != nil {
		return loadedSchemaCatalog{}, err
	}
	return loadedSchemaCatalog{
		Snapshot: SchemaCatalogSnapshot{Version: SchemaCatalogSnapshotVersion},
		Registry: registry,
		Index:    index,
	}, nil
}

func runtimeSchemaPayloadForTest(root *cobra.Command, args []string) (map[string]any, error) {
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		return nil, err
	}
	loaded, err := loadedSchemaCatalogForTestRegistry(registry)
	if err != nil {
		return nil, err
	}
	return schemaPayloadFromLoadedCatalog(loaded, args)
}

func runtimeSchemaAllPayloadForTest(root *cobra.Command) (map[string]any, error) {
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		return nil, err
	}
	return registry.ToPayload()
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
