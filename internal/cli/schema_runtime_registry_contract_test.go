// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProjectRuntimeSchemaConstraintsPublishesOnlyResolvedParameters(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("primary", "", "primary")
	cmd.Flags().String("secondary", "", "secondary")
	cmd.Flags().String("legacy-primary", "", "legacy primary")
	cmd.Flags().String("legacy-secondary", "", "legacy secondary")
	cmd.Flags().String("spoofed-secondary", "", "unreviewed alias")
	_ = cmd.Flags().MarkHidden("legacy-primary")
	_ = cmd.Flags().MarkHidden("legacy-secondary")
	_ = cmd.Flags().MarkHidden("spoofed-secondary")
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("legacy-primary"), runtimeannotate.AnnotationFlagAliasOf, "primary")
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("legacy-primary"), runtimeannotate.AnnotationFlagAliasOrigin, runtimeannotate.FlagAliasOriginCorecmdV1)
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("legacy-secondary"), runtimeannotate.AnnotationFlagAliasOf, "secondary")
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("legacy-secondary"), runtimeannotate.AnnotationFlagAliasOrigin, runtimeannotate.FlagAliasOriginCorecmdV1)
	// alias_of without the framework-owned origin is not sufficient evidence
	// to rewrite an executable-only spelling into the public contract.
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("spoofed-secondary"), runtimeannotate.AnnotationFlagAliasOf, "secondary")

	parameters := []ParameterSpec{{Name: "primary"}, {Name: "secondary"}}
	runtimeConstraints := RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{
			{"primary", "legacy-primary"},
			{"primary", "secondary", "legacy-primary", "spoofed-secondary"},
		},
		RequireOneOf: [][]string{
			{"primary", "legacy-primary", "spoofed-secondary"},
		},
		RequireTogether: [][]string{
			{"primary", "legacy-secondary"},
		},
	}

	got, err := projectRuntimeSchemaConstraints(cmd, parameters, nil, runtimeConstraints)
	if err != nil {
		t.Fatal(err)
	}
	want := RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"primary", "secondary"}},
		RequireOneOf:      [][]string{{"primary"}},
		RequireTogether:   [][]string{{"primary", "secondary"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public constraints = %#v, want %#v", got, want)
	}
	if got := runtimeConstraints.MutuallyExclusive[0]; !reflect.DeepEqual(got, []string{"primary", "legacy-primary"}) {
		t.Fatalf("projection mutated executable constraints: %#v", runtimeConstraints)
	}
}

func TestCrossPlatformCoverageProjectRuntimeSchemaConstraintsFailsClosedByConstraintKind(t *testing.T) {
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "run"}
		cmd.Flags().String("primary", "", "primary")
		cmd.Flags().String("secondary", "", "secondary")
		cmd.Flags().String("hidden-only", "", "unreviewed hidden alias")
		_ = cmd.Flags().MarkHidden("hidden-only")
		// alias_of without the framework-owned origin remains untrusted.
		runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("hidden-only"), runtimeannotate.AnnotationFlagAliasOf, "secondary")
		return cmd
	}
	parameters := []ParameterSpec{{Name: "primary"}, {Name: "secondary"}}

	t.Run("mutually exclusive filters unreviewed hidden members", func(t *testing.T) {
		got, err := projectRuntimeSchemaConstraints(
			newCommand(),
			parameters,
			nil,
			RuntimeSchemaConstraints{MutuallyExclusive: [][]string{{"primary", "hidden-only", "secondary"}}},
		)
		if err != nil {
			t.Fatal(err)
		}
		want := normalizeRuntimeSchemaConstraints(RuntimeSchemaConstraints{MutuallyExclusive: [][]string{{"primary", "secondary"}}})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("projected constraints = %#v, want %#v", got, want)
		}
	})

	t.Run("require one of keeps a public alternative", func(t *testing.T) {
		got, err := projectRuntimeSchemaConstraints(
			newCommand(),
			parameters,
			nil,
			RuntimeSchemaConstraints{RequireOneOf: [][]string{{"hidden-only", "primary"}}},
		)
		if err != nil {
			t.Fatal(err)
		}
		want := normalizeRuntimeSchemaConstraints(RuntimeSchemaConstraints{RequireOneOf: [][]string{{"primary"}}})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("projected constraints = %#v, want %#v", got, want)
		}
	})

	t.Run("require one of rejects an empty public projection", func(t *testing.T) {
		_, err := projectRuntimeSchemaConstraints(
			newCommand(),
			parameters,
			nil,
			RuntimeSchemaConstraints{RequireOneOf: [][]string{{"hidden-only"}}},
		)
		if err == nil || !strings.Contains(err.Error(), "has no published alternative after filtering unreviewed hidden inputs") {
			t.Fatalf("empty public require_one_of error = %v", err)
		}
	})

	t.Run("require together rejects mixed public and hidden members", func(t *testing.T) {
		_, err := projectRuntimeSchemaConstraints(
			newCommand(),
			parameters,
			nil,
			RuntimeSchemaConstraints{RequireTogether: [][]string{{"primary", "hidden-only"}}},
		)
		if err == nil || !strings.Contains(err.Error(), "mixes published inputs") {
			t.Fatalf("mixed public/hidden require_together error = %v", err)
		}
	})

	t.Run("require together may omit an executable-only relationship", func(t *testing.T) {
		cmd := newCommand()
		cmd.Flags().String("hidden-peer", "", "another unreviewed hidden input")
		_ = cmd.Flags().MarkHidden("hidden-peer")
		got, err := projectRuntimeSchemaConstraints(
			cmd,
			parameters,
			nil,
			RuntimeSchemaConstraints{RequireTogether: [][]string{{"hidden-only", "hidden-peer"}}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !runtimeSchemaConstraintsEmpty(got) {
			t.Fatalf("projected constraints = %#v, want empty", got)
		}
	})
}

func TestCrossPlatformCoverageProjectRuntimeSchemaConstraintsRetainsPublishedPositionals(t *testing.T) {
	got, err := projectRuntimeSchemaConstraints(
		&cobra.Command{Use: "consume"},
		[]ParameterSpec{{Name: "subscribe-id"}},
		[]contract.RuntimeSchemaPositional{{Name: "event_key", Index: 0}},
		RuntimeSchemaConstraints{RequireOneOf: [][]string{{"event_key", "subscribe-id"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := normalizeRuntimeSchemaConstraints(RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"event_key", "subscribe-id"}},
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positional constraint projection = %#v, want %#v", got, want)
	}
}

func TestCrossPlatformCoverageProjectRuntimeSchemaConstraintsRejectsUnknownInput(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("primary", "", "primary")

	_, err := projectRuntimeSchemaConstraints(
		cmd,
		[]ParameterSpec{{Name: "primary"}},
		nil,
		RuntimeSchemaConstraints{RequireOneOf: [][]string{{"primary", "primray"}}},
	)
	if err == nil || !strings.Contains(err.Error(), `constraint require_one_of[0] references unknown executable input "primray"`) {
		t.Fatalf("unknown constraint input error = %v", err)
	}
}

func TestCrossPlatformCoverageProjectRuntimeSchemaConstraintsRejectsVisibleUnpublishedInput(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("primary", "", "primary")
	cmd.Flags().String("secondary", "", "secondary")

	_, err := projectRuntimeSchemaConstraints(
		cmd,
		[]ParameterSpec{{Name: "primary"}},
		nil,
		RuntimeSchemaConstraints{RequireTogether: [][]string{{"primary", "secondary"}}},
	)
	if err == nil || !strings.Contains(err.Error(), `constraint require_together[0] references visible executable input "secondary" absent from public Schema`) {
		t.Fatalf("visible unpublished constraint input error = %v", err)
	}
}

func TestCrossPlatformCoverageProjectRuntimeSchemaConstraintsRejectsInvalidReviewedAlias(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		prepare func(*cobra.Command)
		want    string
	}{
		{
			name: "missing target",
			prepare: func(cmd *cobra.Command) {
				annotateReviewedAliasForTest(cmd, "legacy", "missing")
			},
			want: `hidden alias "legacy" targets unknown executable input "missing"`,
		},
		{
			name: "hidden target",
			prepare: func(cmd *cobra.Command) {
				_ = cmd.Flags().MarkHidden("primary")
				annotateReviewedAliasForTest(cmd, "legacy", "primary")
			},
			want: `hidden alias "legacy" targets hidden input "primary"`,
		},
		{
			name: "type mismatch",
			prepare: func(cmd *cobra.Command) {
				cmd.Flags().Int("legacy-int", 0, "legacy int")
				_ = cmd.Flags().MarkHidden("legacy-int")
				annotateReviewedAliasForTest(cmd, "legacy-int", "primary")
			},
			want: `hidden alias "legacy-int" and public input "primary" have incompatible types`,
		},
		{
			name: "malformed target annotation",
			prepare: func(cmd *cobra.Command) {
				runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("legacy"), runtimeannotate.AnnotationFlagAliasOf, " primary ")
				runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup("legacy"), runtimeannotate.AnnotationFlagAliasOrigin, runtimeannotate.FlagAliasOriginCorecmdV1)
			},
			want: `hidden input "legacy" has malformed reviewed alias target`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "run"}
			cmd.Flags().String("primary", "", "primary")
			cmd.Flags().String("legacy", "", "legacy")
			_ = cmd.Flags().MarkHidden("legacy")
			testCase.prepare(cmd)

			constraintName := "legacy"
			if testCase.name == "type mismatch" {
				constraintName = "legacy-int"
			}
			_, err := projectRuntimeSchemaConstraints(
				cmd,
				[]ParameterSpec{{Name: "primary"}},
				nil,
				RuntimeSchemaConstraints{RequireOneOf: [][]string{{constraintName}}},
			)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("invalid reviewed alias error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func annotateReviewedAliasForTest(cmd *cobra.Command, aliasName, targetName string) {
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup(aliasName), runtimeannotate.AnnotationFlagAliasOf, targetName)
	runtimeannotate.SetFlagAnnotation(cmd.Flags().Lookup(aliasName), runtimeannotate.AnnotationFlagAliasOrigin, runtimeannotate.FlagAliasOriginCorecmdV1)
}

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
