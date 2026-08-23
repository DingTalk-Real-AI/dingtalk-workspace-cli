// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

func TestValidateSchemaRegistryAgainstCommandRegistryChecksFullIdentity(t *testing.T) {
	tool := ToolSpec{Identity: contract.ToolIdentitySpec{
		ProductID:       "sample",
		SourceProductID: "implementation_a",
		Name:            "run",
		CanonicalPath:   "sample.run",
		Path:            "sample.run",
		CLIPath:         "sample run",
		PrimaryCLIPath:  "sample run",
		Source:          "contract_identity",
	}}
	registry, err := SchemaRegistryFromRuntime("test", []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatal(err)
	}

	base := CommandSpec{
		CanonicalPath:   "sample.run",
		SourceProductID: "implementation_a",
		PrimaryCLIPath:  "sample run",
		Visibility:      SchemaVisibilityPublic,
		Source:          "contract_identity",
	}
	for name, test := range map[string]struct {
		mutate func(*CommandSpec)
		want   string
	}{
		"source product": {
			mutate: func(spec *CommandSpec) { spec.SourceProductID = "implementation_b" },
			want:   "source product",
		},
		"identity source": {
			mutate: func(spec *CommandSpec) { spec.Source = "stale_identity_source" },
			want:   "identity source",
		},
	} {
		t.Run(name, func(t *testing.T) {
			expected := cloneCommandSpec(base)
			test.mutate(&expected)
			effective, err := newEffectiveCommandRegistry([]CommandSpec{expected})
			if err != nil {
				t.Fatal(err)
			}
			err = validateSchemaRegistryAgainstCommandRegistry(registry, effective)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateSchemaRegistryAgainstCommandRegistryRejectsAliasViewAsCanonical(t *testing.T) {
	baseTool := ToolSpec{Identity: contract.ToolIdentitySpec{
		ProductID:      "sample",
		Name:           "run",
		CanonicalPath:  "sample.run",
		Path:           "sample.run",
		CLIPath:        "sample run",
		PrimaryCLIPath: "sample run",
		Aliases:        []string{"sample execute"},
		Source:         "contract_identity",
	}}
	effective, err := newEffectiveCommandRegistry([]CommandSpec{{
		CanonicalPath:  "sample.run",
		PrimaryCLIPath: "sample run",
		Aliases:        []string{"sample execute"},
		Visibility:     SchemaVisibilityPublic,
		Source:         "contract_identity",
	}})
	if err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		mutate func(*contract.ToolIdentitySpec)
		want   string
	}{
		"alternate cli path": {
			mutate: func(identity *contract.ToolIdentitySpec) { identity.CLIPath = "sample execute" },
			want:   "must equal primary_cli_path",
		},
		"alias marker": {
			mutate: func(identity *contract.ToolIdentitySpec) { identity.IsAlias = true },
			want:   "must have is_alias=false",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tool := baseTool
			test.mutate(&tool.Identity)
			registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}}}
			err := validateSchemaRegistryAgainstCommandRegistry(registry, effective)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAssembleSchemaRegistryFailClosedMissingContractFinal(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "run", Short: "Run", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(leaf, "sample", "run", "test")
	product := &cobra.Command{Use: "sample"}
	product.AddCommand(leaf)
	root.AddCommand(product)

	t.Cleanup(func() { contract.ClearProductDeclForTest("sample") })
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sample",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Sample product",
			UseWhen:      []string{"sample routing"},
			AvoidWhen:    []string{"not sample"},
		},
	})

	_, err := schemaRegistryForTest(root)
	if err == nil || !strings.Contains(err.Error(), "missing RuntimeContractFinal") {
		t.Fatalf("production assembly error = %v, want missing RuntimeContractFinal", err)
	}
}

func TestCrossPlatformCoverageAssembleSchemaRegistryFailClosedMissingProductDecl(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "run", Short: "Run", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(leaf, "orphan", "run", "test")
	contractfinal.RegisterRuntimeContractFinal(leaf, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "orphan", Name: "run", CanonicalPath: "orphan.run",
			CLIPath: "orphan run", PrimaryCLIPath: "orphan run",
		},

		Title:       "Orphan run",
		Description: "Has ContractFinal but no ProductDecl",
		Selection:   &contract.SelectionSpec{AgentSummary: "orphan leaf"},
	})
	t.Cleanup(func() {
		contractfinal.ClearRuntimeContractFinalForTest(leaf)
		contract.ClearProductDeclForTest("orphan")
	})
	product := &cobra.Command{Use: "orphan"}
	product.AddCommand(leaf)
	root.AddCommand(product)

	_, err := schemaRegistryForTest(root)
	if err == nil || !strings.Contains(err.Error(), "missing ProductDecl") {
		t.Fatalf("production assembly error = %v, want missing ProductDecl", err)
	}
}

func TestAssembleSchemaRegistryRequiresContractFinalAndProductDecl(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "run", Short: "Run sample", Long: "Run the sample tool", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(leaf, "sample", "run", "test")
	contractfinal.RegisterRuntimeContractFinal(leaf, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run",
		},

		Title:       "Sample run",
		Description: "Declared sample tool",
		Safety: &contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "none", Idempotency: "idempotent",
		},
		Interface: &contract.InterfaceSpec{
			Mode: "local", Availability: "available", Reason: "test local leaf",
		},
		Selection: &contract.SelectionSpec{
			AgentSummary: "Run a sample tool",
			UseWhen:      []string{"need sample run"},
			AvoidWhen:    []string{"need other product"},
		},
	})
	t.Cleanup(func() {
		contractfinal.ClearRuntimeContractFinalForTest(leaf)
		contract.ClearProductDeclForTest("sample")
	})
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sample",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Sample product",
			UseWhen:      []string{"sample routing"},
			AvoidWhen:    []string{"not sample"},
		},
	})
	product := &cobra.Command{Use: "sample"}
	product.AddCommand(leaf)
	root.AddCommand(product)

	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatalf("declared production assembly: %v", err)
	}
	if len(registry.Products) != 1 || len(registry.Products[0].Tools) != 1 {
		t.Fatalf("registry = %#v", registry.Products)
	}
	tool := registry.Products[0].Tools[0]
	if tool.MetadataSource != "corecmd.contract" {
		t.Fatalf("metadata_source = %q, want corecmd.contract", tool.MetadataSource)
	}
	if registry.Products[0].Selection.AgentSummary != "Sample product" {
		t.Fatalf("product selection = %#v", registry.Products[0].Selection)
	}
}

func TestAssembleContractFinalDescriptionPrefersCobraLong(t *testing.T) {
	tool := assembleContractFinalTextTool(t, "Run sample", "Cobra Long description wins", "Declared description")
	if tool.Description != "Cobra Long description wins" {
		t.Fatalf("description = %q, want Cobra Long", tool.Description)
	}
	prov := tool.FieldProvenance["description"]
	if prov.Precedence != "cobra_help" {
		t.Fatalf("description precedence = %q, want cobra_help", prov.Precedence)
	}
	if prov.Resolution != "cobra_help_preferred" {
		t.Fatalf("description resolution = %q, want cobra_help_preferred", prov.Resolution)
	}
	if prov.Source != "cobra_help" {
		t.Fatalf("description source = %q, want cobra_help", prov.Source)
	}
	// Title stays declared-first even when Short differs.
	if tool.Title != "Declared title" {
		t.Fatalf("title = %q, want declared title", tool.Title)
	}
	titleProv := tool.FieldProvenance["title"]
	if titleProv.Precedence != "contract_final" {
		t.Fatalf("title precedence = %q, want contract_final", titleProv.Precedence)
	}
}

func TestAssembleContractFinalDescriptionUsesDeclaredWithoutLong(t *testing.T) {
	tool := assembleContractFinalTextTool(t, "Run sample", "", "Declared description without Long")
	if tool.Description != "Declared description without Long" {
		t.Fatalf("description = %q, want declared ContractDecl description", tool.Description)
	}
	prov := tool.FieldProvenance["description"]
	if prov.Precedence != "contract_final" {
		t.Fatalf("description precedence = %q, want contract_final", prov.Precedence)
	}
	if prov.Resolution != "contract_pass_through" {
		t.Fatalf("description resolution = %q, want contract_pass_through", prov.Resolution)
	}
	if prov.Source != "corecmd.contract" {
		t.Fatalf("description source = %q, want corecmd.contract", prov.Source)
	}
}

// Short-only leaves must not leak Cobra Short into delivered description.
// Description compares only against Long; Short stays a title fallback candidate.
func TestAssembleContractFinalDescriptionIgnoresShortWhenDeclared(t *testing.T) {
	const (
		short    = "Short must not become description"
		declared = "Declared Contract.Description for short-only leaf"
	)
	tool := assembleContractFinalTextTool(t, short, "", declared)
	if tool.Description != declared {
		t.Fatalf("description = %q, want declared %q", tool.Description, declared)
	}
	if tool.Description == short {
		t.Fatalf("description must not equal Short %q", short)
	}
	prov := tool.FieldProvenance["description"]
	if prov.Precedence == "cobra_help" || prov.Source == "cobra_help" {
		t.Fatalf("description provenance = %#v, must not be cobra_help when Long is empty", prov)
	}
	if prov.Precedence != "contract_final" {
		t.Fatalf("description precedence = %q, want contract_final", prov.Precedence)
	}
	if prov.Resolution != "contract_pass_through" {
		t.Fatalf("description resolution = %q, want contract_pass_through", prov.Resolution)
	}
	if prov.Source != "corecmd.contract" {
		t.Fatalf("description source = %q, want corecmd.contract", prov.Source)
	}
}

// assembleContractFinalTextTool builds a one-leaf tree, registers ContractFinal +
// ProductDecl, and runs the production assembly path (schemaRegistryForTest).
func assembleContractFinalTextTool(t *testing.T, short, long, declaredDescription string) ToolSpec {
	t.Helper()
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{
		Use:   "run",
		Short: short,
		Long:  long,
		Run:   func(*cobra.Command, []string) {},
	}
	AttachRuntimeSchema(leaf, "sample", "run", "test")
	contractfinal.RegisterRuntimeContractFinal(leaf, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run",
		},

		Title:       "Declared title",
		Description: declaredDescription,
		Safety: &contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "none", Idempotency: "idempotent",
		},
		Interface: &contract.InterfaceSpec{
			Mode: "local", Availability: "available",
		},
		Selection: &contract.SelectionSpec{
			AgentSummary: "Run a sample tool",
			UseWhen:      []string{"need sample run"},
			AvoidWhen:    []string{"need other product"},
		},
	})
	t.Cleanup(func() {
		contractfinal.ClearRuntimeContractFinalForTest(leaf)
		contract.ClearProductDeclForTest("sample")
	})
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sample",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Sample product",
			UseWhen:      []string{"sample routing"},
			AvoidWhen:    []string{"not sample"},
		},
	})
	product := &cobra.Command{Use: "sample"}
	product.AddCommand(leaf)
	root.AddCommand(product)

	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatalf("assemble schema registry: %v", err)
	}
	if len(registry.Products) != 1 || len(registry.Products[0].Tools) != 1 {
		t.Fatalf("registry = %#v", registry.Products)
	}
	return registry.Products[0].Tools[0]
}
