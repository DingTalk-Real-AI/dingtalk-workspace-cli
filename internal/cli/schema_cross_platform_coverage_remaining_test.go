// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCrossPlatformCoverageAgentExampleRemainingBranches(t *testing.T) {
	idx := func(v int) *int { return &v }
	restoreSelection := func() { agentExampleSelectionFn = contractFinalToolSelection }

	t.Run("disposition validation and map build", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		t.Cleanup(restoreSelection)
		agentExampleSelectionFn = func(cmd *cobra.Command) AgentToolSelection {
			selection := contractFinalToolSelection(cmd)
			selection.ExampleDispositions = []AgentExampleDisposition{{
				Index: idx(0), Mode: AgentExampleModeDryRun, Reviewed: true,
				Reason: "r", ReasonCode: AgentExampleReasonLocalState,
			}}
			return selection
		}
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "invalid mode") {
			t.Fatalf("disposition validation error = %v", err)
		}
	})

	t.Run("invalid argv syntax in declared example", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{`dws sample run "unclosed`}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "invalid argv syntax") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("matched path with nil Cobra command", func(t *testing.T) {
		root := &cobra.Command{Use: "dws"}
		leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
		leaf.Flags().String("name", "", "name")
		AttachRuntimeSchema(leaf, "sample", "run", "test")
		contractfinal.RegisterRuntimeContractFinal(leaf, contract.ContractFinalPayload{
			Identity: &contract.ToolIdentitySpec{
				ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
				CLIPath: "sample run", PrimaryCLIPath: "sample run",
			},

			Selection: &contract.SelectionSpec{Examples: []string{"dws sample alt --name x"}},
		})
		t.Cleanup(func() { contractfinal.ClearRuntimeContractFinalForTest(leaf) })
		product := &cobra.Command{Use: "sample"}
		product.AddCommand(leaf)
		root.AddCommand(product)
		bound := BoundCommandRegistry{
			Commands: []BoundCommandSpec{{
				CommandSpec:    CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run", Visibility: SchemaVisibilityPublic},
				PrimaryCommand: leaf,
				AliasCommands:  []BoundAlias{{Path: "sample alt", Command: nil}},
			}},
			ByCanonical: map[string]BoundCommandSpec{"sample.run": {
				CommandSpec:    CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run"},
				PrimaryCommand: leaf,
				AliasCommands:  []BoundAlias{{Path: "sample alt", Command: nil}},
			}},
		}
		tool := ToolSpec{
			Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.run"},
		}
		_, err := buildAgentExampleExecutionPlan(bound, map[string]ToolSpec{"sample.run": tool})
		if err == nil || !strings.Contains(err.Error(), "no bound Cobra command") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid compatibility constraints and positionals", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(cmd *cobra.Command, _ *contract.ContractFinalPayload) {
			if cmd.Annotations == nil {
				cmd.Annotations = map[string]string{}
			}
			cmd.Annotations[runtimeSchemaRulesAnnotation] = "{"
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "invalid executable constraints") {
			t.Fatalf("constraints error = %v", err)
		}
		bound, registry = crossPlatformAgentExampleFixture(t, func(cmd *cobra.Command, _ *contract.ContractFinalPayload) {
			if cmd.Annotations == nil {
				cmd.Annotations = map[string]string{}
			}
			cmd.Annotations[runtimeSchemaArgsAnnotation] = "{"
		})
		_, err = BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "invalid executable positionals") {
			t.Fatalf("positionals error = %v", err)
		}
	})

	t.Run("typed constraints merge and cobra contract failure", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"dws sample run"}
		})
		registry.Products[0].Tools[0].Constraints = RuntimeSchemaConstraints{
			RequireOneOf: [][]string{{"missing-flag"}},
		}
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "require_one_of") {
			t.Fatalf("cobra contract error = %v", err)
		}
	})

	t.Run("disposition narrows dry_run capability", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.DryRun = &contract.DryRunSpec{PreviewKind: "plan"}
			payload.Selection.ExampleDispositions = []contract.ExampleDisposition{{
				Index: idx(0), Mode: contract.ExampleDispositionModeContractOnly, Reviewed: true,
				Reason: "cannot dry-run safely", ReasonCode: contract.ExampleDispositionReasonStatefulPreflight,
			}}
		})
		plan, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err != nil {
			t.Fatalf("plan error = %v", err)
		}
		if plan.ContractOnly != 1 || plan.ReviewedContractOnly != 1 || plan.ContractOnlyByReason[AgentExampleReasonStatefulPreflight] != 1 {
			t.Fatalf("plan = %#v", plan)
		}
		if len(plan.Examples) != 1 || plan.Examples[0].Mode != AgentExampleModeContractOnly {
			t.Fatalf("execution = %#v", plan.Examples)
		}
	})

	t.Run("disposition without dry_run capability fails", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.ExampleDispositions = []contract.ExampleDisposition{{
				Index: idx(0), Mode: contract.ExampleDispositionModeContractOnly, Reviewed: true,
				Reason: "no dry run", ReasonCode: contract.ExampleDispositionReasonLocalState,
			}}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "narrows no explicit dry_run") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("matchAgentExamplePath lazy argv init", func(t *testing.T) {
		cmd := &cobra.Command{Use: "run"}
		_, matched, ok := matchAgentExamplePath(
			[]string{"dws", "sample", "run"},
			[]agentExamplePath{{Path: "sample run", Command: cmd}},
		)
		if !ok || matched.Command != cmd {
			t.Fatalf("match = %#v ok=%v", matched, ok)
		}
	})

	t.Run("tokenize and placeholder edges", func(t *testing.T) {
		if _, err := tokenizeAgentExample(`dws sample run \`); err == nil || !strings.Contains(err.Error(), "trailing escape") {
			t.Fatalf("trailing escape error = %v", err)
		}
		if _, err := tokenizeAgentExample(`dws sample run 'x`); err == nil {
			t.Fatal("single-quoted token must fail when unclosed")
		}
		if _, err := tokenizeAgentExample(`dws sample run $HOME`); err == nil || !strings.Contains(err.Error(), "expansion") {
			t.Fatalf("expansion error = %v", err)
		}
		if _, err := tokenizeAgentExample(`dws sample run <`); err == nil || !strings.Contains(err.Error(), "redirection") {
			t.Fatalf("redirection error = %v", err)
		}
		if _, _, ok := agentExamplePlaceholderAt("x<>", 1); ok {
			t.Fatal("placeholder must reject empty body")
		}
	})

	t.Run("validateAgentExampleCobraContract shorthand and args", func(t *testing.T) {
		cmd := &cobra.Command{Use: "run"}
		cmd.Flags().StringP("mode", "m", "", "mode")
		parent := &cobra.Command{Use: "parent"}
		parent.PersistentFlags().StringP("token", "t", "", "token")
		parent.AddCommand(cmd)
		emptyConstraints := RuntimeSchemaConstraints{}
		if err := validateAgentExampleCobraContract(cmd, []string{"--"}, emptyConstraints, nil); err == nil {
			t.Fatal("-- terminator must fail")
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"--="}, emptyConstraints, nil); err == nil || !strings.Contains(err.Error(), "empty long flag") {
			t.Fatalf("empty long flag error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"-🙂"}, emptyConstraints, nil); err == nil || !strings.Contains(err.Error(), "non-ASCII") {
			t.Fatalf("non-ASCII shorthand error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"-z"}, emptyConstraints, nil); err == nil || !strings.Contains(err.Error(), "unknown shorthand") {
			t.Fatalf("unknown shorthand error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"-m"}, emptyConstraints, nil); err == nil || !strings.Contains(err.Error(), "requires a value") {
			t.Fatalf("shorthand value error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"-mt"}, emptyConstraints, nil); err != nil {
			t.Fatalf("combined shorthand persistent lookup error = %v", err)
		}
		cmd.Args = func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("need arg")
			}
			return nil
		}
		if err := validateAgentExampleCobraContract(cmd, nil, emptyConstraints, nil); err == nil || !strings.Contains(err.Error(), "invalid positional arguments") {
			t.Fatalf("args validation error = %v", err)
		}
		if err := validateAgentExampleConstraints(map[string]bool{}, RuntimeSchemaConstraints{
			RequireOneOf: [][]string{{"a"}},
		}); err == nil {
			t.Fatal("constraint propagation must fail")
		}
	})

	t.Run("mergeAgentExamplePositionals sort branches", func(t *testing.T) {
		merged := mergeAgentExamplePositionals(
			[]contract.RuntimeSchemaPositional{{Index: 1, Name: "b"}},
			[]contract.RuntimeSchemaPositional{{Index: 0, Name: "a"}},
		)
		if len(merged) != 2 || merged[0].Index != 0 {
			t.Fatalf("merged = %#v", merged)
		}
	})

	t.Run("runtimeCommandFlagByShorthand guards", func(t *testing.T) {
		if runtimeCommandFlagByShorthand(nil, "m") != nil {
			t.Fatal("nil command must return nil flag")
		}
		if runtimeCommandFlagByShorthand(&cobra.Command{}, "xy") != nil {
			t.Fatal("multi-char shorthand must return nil")
		}
	})
}

func TestCrossPlatformCoverageSchemaMetaIndexRemainingBranches(t *testing.T) {
	t.Run("BuildSchemaMetaIndex empty canonical_path uses map key", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Version: SchemaCatalogSnapshotVersion, SourceHash: "hash",
			Tools: map[string]map[string]any{
				"sample.run": {"cli_path": "sample run", "canonical_path": ""},
			},
		}
		index, err := BuildSchemaMetaIndex(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if index.Entries[0].Canonical != "sample.run" {
			t.Fatalf("canonical = %q", index.Entries[0].Canonical)
		}
	})

	t.Run("EncodeSchemaMetaIndex failure", func(t *testing.T) {
		encodeSchemaMetaIndexFn = func(SchemaMetaIndexSnapshot) ([]byte, error) { return nil, fmt.Errorf("boom") }
		t.Cleanup(func() { encodeSchemaMetaIndexFn = encodeSchemaMetaIndexGob })
		_, err := EncodeSchemaMetaIndex(SchemaMetaIndexSnapshot{Version: 1, SourceHash: "x", Entries: []SchemaMetaIndexEntry{{CLIPath: "a"}}})
		if err == nil || !strings.Contains(err.Error(), "encode schema meta index") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("DecodeSchemaMetaIndexJSON failures", func(t *testing.T) {
		if _, err := DecodeSchemaMetaIndexJSON([]byte(`{"version":2,"source_hash":"x","entries":[{"cli_path":"a"}]}`)); err == nil || !strings.Contains(err.Error(), "unsupported schema meta index version") {
			t.Fatalf("version error = %v", err)
		}
		if _, err := DecodeSchemaMetaIndexJSON([]byte(`{"version":1,"entries":[{"cli_path":"a"}]}`)); err == nil || !strings.Contains(err.Error(), "source_hash") {
			t.Fatalf("source_hash error = %v", err)
		}
	})

	t.Run("commandMetaLookupFromIndex duplicate and missing cli_path", func(t *testing.T) {
		dup := SchemaMetaIndexSnapshot{
			Version: SchemaMetaIndexVersion, SourceHash: "x",
			Entries: []SchemaMetaIndexEntry{
				{CLIPath: "same", Canonical: "a.a"},
				{CLIPath: "same", Canonical: "b.b"},
			},
		}
		if _, err := commandMetaLookupFromIndex(dup); err == nil || !strings.Contains(err.Error(), "duplicate cli_path") {
			t.Fatalf("duplicate error = %v", err)
		}
		if _, err := commandMetaLookupFromIndex(SchemaMetaIndexSnapshot{
			Version: SchemaMetaIndexVersion, SourceHash: "x",
			Entries: []SchemaMetaIndexEntry{{CLIPath: " ", Canonical: "a.a"}},
		}); err == nil || !strings.Contains(err.Error(), "missing cli_path") {
			t.Fatalf("missing cli_path error = %v", err)
		}
	})

	t.Run("ValidateSchemaMetaIndexAgainstSnapshot branches", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			SourceHash: "hash-a",
			Tools: map[string]map[string]any{
				"a.run": {"cli_path": "a run", "canonical_path": "a.run"},
				"b.run": {"cli_path": "b run", "canonical_path": "b.run"},
				"c.run": {"cli_path": "", "canonical_path": "c.run"},
			},
		}
		index := SchemaMetaIndexSnapshot{
			Version: SchemaMetaIndexVersion, SourceHash: "hash-a",
			Entries: []SchemaMetaIndexEntry{
				{CLIPath: "a run", Canonical: "a.run"},
				{CLIPath: "b run", Canonical: "b.run"},
			},
		}
		badSurface := index
		badSurface.SurfaceHash = "mismatch"
		if err := ValidateSchemaMetaIndexAgainstSnapshot(badSurface, snapshot); err == nil || !strings.Contains(err.Error(), "surface_hash") {
			t.Fatalf("surface_hash error = %v", err)
		}
		if err := ValidateSchemaMetaIndexAgainstSnapshot(index, snapshot); err == nil || !strings.Contains(err.Error(), "entry count") {
			t.Fatalf("entry count error = %v", err)
		}
	})

	t.Run("ValidateSchemaMetaIndexAgainstCatalog entry count mismatch", func(t *testing.T) {
		registry := SchemaRegistry{Products: []ProductSpec{{
			ID: "sample",
			Tools: []ToolSpec{
				{Identity: contract.ToolIdentitySpec{CLIPath: "sample run", CanonicalPath: "sample.run", ProductID: "sample", Name: "run", Path: "sample.run", PrimaryCLIPath: "sample run"}},
				{Identity: contract.ToolIdentitySpec{CLIPath: "sample alt", CanonicalPath: "sample.alt", ProductID: "sample", Name: "alt", Path: "sample.alt", PrimaryCLIPath: "sample alt"}},
				{Identity: contract.ToolIdentitySpec{CLIPath: " ", CanonicalPath: "sample.blank", ProductID: "sample", Name: "blank", Path: "sample.blank", PrimaryCLIPath: " "}},
			},
		}}}
		index := SchemaMetaIndexSnapshot{
			Version: SchemaMetaIndexVersion, SourceHash: "x",
			Entries: []SchemaMetaIndexEntry{
				{CLIPath: "sample run", Canonical: "sample.run", ProductID: "sample"},
				{CLIPath: "sample alt", Canonical: "sample.alt", ProductID: "sample"},
			},
		}
		if err := ValidateSchemaMetaIndexAgainstCatalog(index, registry); err == nil || !strings.Contains(err.Error(), "entry count") {
			t.Fatalf("catalog entry count error = %v", err)
		}
	})

	t.Run("compareCommandMetaLookups size and unexpected paths", func(t *testing.T) {
		got := map[string]CommandMeta{"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a"}}}
		want := map[string]CommandMeta{
			"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a"}},
			"b": {Identity: CommandIdentity{CLIPath: "b", Canonical: "p.b"}},
		}
		if err := compareCommandMetaLookups(got, want); err == nil || !strings.Contains(err.Error(), "lookup size") {
			t.Fatalf("size error = %v", err)
		}
		gotExtra := map[string]CommandMeta{
			"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a"}},
			"c": {Identity: CommandIdentity{CLIPath: "c", Canonical: "p.c"}},
		}
		wantExtra := map[string]CommandMeta{
			"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a"}},
			"b": {Identity: CommandIdentity{CLIPath: "b", Canonical: "p.b"}},
		}
		if err := compareCommandMetaLookups(gotExtra, wantExtra); err == nil || !strings.Contains(err.Error(), "missing path") {
			t.Fatalf("missing path error = %v", err)
		}
		if err := compareCommandMetaLookups(
			map[string]CommandMeta{
				"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a"}},
				"b": {Identity: CommandIdentity{CLIPath: "b", Canonical: "p.b"}},
			},
			map[string]CommandMeta{
				"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a"}},
				"c": {Identity: CommandIdentity{CLIPath: "c", Canonical: "p.c"}},
			},
		); err == nil || !strings.Contains(err.Error(), "missing path") {
			t.Fatalf("missing path error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSchemaAgentSelectionRemainingBranches(t *testing.T) {
	t.Run("missing ContractFinal and invalid canonical", func(t *testing.T) {
		leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
		bound := BoundCommandRegistry{Commands: []BoundCommandSpec{{
			CommandSpec:    CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run"},
			PrimaryCommand: leaf,
		}}}
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "no ContractFinal") {
			t.Fatalf("missing ContractFinal error = %v", err)
		}
		bound = crossPlatformAgentSelectionBound(t, nil)
		bound.Commands[0].CanonicalPath = "invalid"
		bound.ByCanonical["invalid"] = bound.Commands[0]
		delete(bound.ByCanonical, "sample.run")
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "invalid bound canonical path") {
			t.Fatalf("invalid canonical error = %v", err)
		}
	})

	t.Run("missing ByCanonical and selection requirements", func(t *testing.T) {
		bound := crossPlatformAgentSelectionBound(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.UseWhen = nil
			payload.Selection.AvoidWhen = nil
		})
		broken := bound
		broken.ByCanonical = map[string]BoundCommandSpec{}
		if _, _, err := BuildAgentSelectionEvalFixture(broken); err == nil || !strings.Contains(err.Error(), "missing from BoundCommandRegistry.ByCanonical") {
			t.Fatalf("ByCanonical error = %v", err)
		}
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "use_when") {
			t.Fatalf("use_when error = %v", err)
		}
		bound = crossPlatformAgentSelectionBound(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.UseWhen = []string{"run sample"}
			payload.Selection.AvoidWhen = nil
		})
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "avoid_when") {
			t.Fatalf("avoid_when error = %v", err)
		}
	})

	t.Run("empty normalized scenarios and conflicting literals", func(t *testing.T) {
		bound := crossPlatformAgentSelectionBound(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.UseWhen = []string{"   "}
			payload.Selection.AvoidWhen = []string{"stop"}
		})
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "empty normalized use_when") {
			t.Fatalf("empty use_when error = %v", err)
		}
		bound = crossPlatformAgentSelectionBound(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.UseWhen = []string{"same literal", "same literal"}
			payload.Selection.AvoidWhen = []string{"stop"}
		})
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "conflicting literal expectations") {
			t.Fatalf("conflicting use_when error = %v", err)
		}
		bound = crossPlatformAgentSelectionBound(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.UseWhen = []string{"same literal"}
			payload.Selection.AvoidWhen = []string{"   "}
		})
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "empty normalized avoid_when") {
			t.Fatalf("empty avoid_when error = %v", err)
		}
		bound = crossPlatformAgentSelectionBound(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.UseWhen = []string{"same literal"}
			payload.Selection.AvoidWhen = []string{"same literal"}
		})
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "same literal positive and negative") {
			t.Fatalf("contradictory scenario error = %v", err)
		}
	})

	t.Run("contractFinalToolSelection without selection", func(t *testing.T) {
		cmd := &cobra.Command{Use: "bare"}
		selection := contractFinalToolSelection(cmd)
		if !selection.Reviewed || selection.AgentSummary != "" {
			t.Fatalf("selection = %#v", selection)
		}
	})

	t.Run("validateAgentSelectionBinding remaining errors", func(t *testing.T) {
		bound := crossPlatformAgentSelectionBound(t, nil)
		bad := bound.Commands[0]
		bad.PrimaryCLIPath = "   "
		if err := validateAgentSelectionBinding(bound, "sample.run", bad); err == nil || !strings.Contains(err.Error(), "empty primary CLI path") {
			t.Fatalf("empty path error = %v", err)
		}
		bad = bound.Commands[0]
		bad.PrimaryCLIPath = "missing path"
		if err := validateAgentSelectionBinding(bound, "sample.run", bad); err == nil || !strings.Contains(err.Error(), "not bound back") {
			t.Fatalf("binding error = %v", err)
		}
		parent := &cobra.Command{Use: "parent", Run: func(*cobra.Command, []string) {}}
		parent.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
		nonLeafSpec := bound.Commands[0]
		nonLeafSpec.PrimaryCommand = parent
		if err := validateAgentSelectionBinding(bound, "sample.run", nonLeafSpec); err == nil || !strings.Contains(err.Error(), "not a runnable Cobra leaf") {
			t.Fatalf("non-leaf error = %v", err)
		}
	})
}

func crossPlatformAgentSelectionBound(t *testing.T, mutate func(*cobra.Command, *contract.ContractFinalPayload)) BoundCommandRegistry {
	t.Helper()
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(leaf, "sample", "run", "test")
	payload := contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
			CLIPath: "sample run", PrimaryCLIPath: "sample run",
		},

		Title:       "Run",
		Description: "Run sample",
		Safety:      &contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Interface:   &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "test"},
		Selection: &contract.SelectionSpec{
			AgentSummary: "Run it",
			UseWhen:      []string{"run"},
			AvoidWhen:    []string{"stop"},
			Examples:     []string{"dws sample run --name x"},
		},
	}
	if mutate != nil {
		mutate(leaf, &payload)
	}
	contractfinal.RegisterRuntimeContractFinal(leaf, payload)
	t.Cleanup(func() {
		contractfinal.ClearRuntimeContractFinalForTest(leaf)
		contract.ClearProductDeclForTest("sample")
	})
	product := &cobra.Command{Use: "sample"}
	product.AddCommand(leaf)
	root.AddCommand(product)
	bound, err := boundTestCommandRegistry(root)
	if err != nil {
		t.Fatalf("boundTestCommandRegistry() error = %v", err)
	}
	return bound
}

func TestCrossPlatformCoverageSchemaCatalogRemainingBranches(t *testing.T) {
	t.Run("delivery catalog assemble error propagation", func(t *testing.T) {
		RegisterSchemaSourceRoot(func() *cobra.Command { return &cobra.Command{Use: "dws"} })
		assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
			return loadedSchemaCatalog{}, fmt.Errorf("catalog load failed")
		}
		t.Cleanup(restorePackageCLISchemaDeliveryForTest)
		if err := deliverySchemaCatalogError(); err == nil || !strings.Contains(err.Error(), "catalog load failed") {
			t.Fatalf("catalog err = %v", err)
		}
	})

	t.Run("loadSchemaCatalogSnapshot interface validation failure", func(t *testing.T) {
		testseam.Swap(t, &validateSchemaSnapshotTypedRoundTrip, false)
		snapshot := SchemaCatalogSnapshot{
			Version: SchemaCatalogSnapshotVersion,
			Catalog: map[string]any{
				"kind": "schema", "level": "catalog", "source": "t",
				"count": float64(1), "tool_count": float64(1),
				"products": []any{map[string]any{
					"id":    "sample",
					"tools": []any{map[string]any{"canonical_path": "sample.run"}},
				}},
			},
			Tools: map[string]map[string]any{
				"sample.run": {
					"product_id": "sample", "canonical_path": "sample.run", "name": "run",
					"path": "sample.run", "cli_path": "sample run", "primary_cli_path": "sample run",
				},
			},
		}
		snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
		testseam.Swap(t, &loadCatalogValidateInterfaces, func(SchemaRegistry) error { return fmt.Errorf("interface boom") })
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "validate final Schema interface disposition") {
			t.Fatalf("interface validation error = %v", err)
		}
	})

	t.Run("loadSchemaCatalogSnapshot missing source_hash", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Version: SchemaCatalogSnapshotVersion,
			Catalog: map[string]any{"kind": "schema"},
			Tools:   map[string]map[string]any{"a": {}},
		}
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "missing source_hash") {
			t.Fatalf("source_hash error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSchemaDryRunCapabilitiesRemainingBranches(t *testing.T) {
	t.Cleanup(clearDeclaredDryRunCapabilitiesForTest)
	t.Cleanup(func() { setReviewedDryRunCapabilityGroupsForTest(nil) })

	restore := setReviewedDryRunCapabilityGroupsForTest([]dryRunCapabilityGroup{{
		PreviewKind:    "not-a-real-kind",
		CanonicalPaths: []string{"sample.run"},
	}})
	t.Cleanup(restore)
	if _, err := loadReviewedDryRunCapabilities(); err == nil {
		t.Fatal("invalid preview kind must fail manual registry load")
	}
	resetReviewedDryRunCapabilitiesLazyForTest()

	restore = setReviewedDryRunCapabilityGroupsForTest([]dryRunCapabilityGroup{{
		PreviewKind:    "plan",
		CanonicalPaths: []string{"sample.run"},
	}})
	t.Cleanup(restore)
	declaredDryRunCapabilities.Store(1, contract.DryRunSpec{PreviewKind: "plan"})
	if _, err := loadReviewedDryRunCapabilities(); err == nil || !strings.Contains(err.Error(), "non-string key") {
		t.Fatalf("non-string key error = %v", err)
	}
	clearDeclaredDryRunCapabilitiesForTest()
	declaredDryRunCapabilities.Store("sample.run", "not-a-spec")
	if _, err := loadReviewedDryRunCapabilities(); err == nil || !strings.Contains(err.Error(), "non-contract.DryRunSpec") {
		t.Fatalf("non-spec value error = %v", err)
	}
	clearDeclaredDryRunCapabilitiesForTest()
	declaredDryRunCapabilities.Store("sample.run", contract.DryRunSpec{PreviewKind: "request"})
	if _, err := loadReviewedDryRunCapabilities(); err == nil || !strings.Contains(err.Error(), "conflicts with manual") {
		t.Fatalf("manual conflict error = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaSnapshotAdapterRemainingBranches(t *testing.T) {
	t.Run("schemaRegistryFromSnapshot decode failures", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Catalog: map[string]any{"products": "not-a-list"},
			Tools:   map[string]map[string]any{"a": {"canonical_path": "a"}},
		}
		if _, _, err := schemaRegistryFromSnapshot(snapshot); err == nil {
			t.Fatal("invalid catalog decode must fail")
		}
		snapshot = SchemaCatalogSnapshot{
			Catalog: map[string]any{"kind": "schema", "level": "catalog", "source": "t", "count": 1, "tool_count": 1, "products": []any{}},
			Tools:   map[string]map[string]any{"a": {"constraints": "bad"}},
		}
		if _, _, err := schemaRegistryFromSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "decode Schema ToolSpec a") {
			t.Fatalf("tool decode error = %v", err)
		}
	})

	t.Run("schemaRegistryFromTyped product/tool mismatches", func(t *testing.T) {
		catalog := schemaCatalogWire{
			Kind: "schema", Level: "catalog", Source: "t", Count: 1, ToolCount: 1,
			Products: []schemaProductWire{{
				ID:    "sample",
				Tools: []schemaToolWire{{CanonicalPath: "missing.tool"}},
			}},
		}
		if _, _, err := schemaRegistryFromTyped(catalog, map[string]schemaToolWire{}); err == nil || !strings.Contains(err.Error(), "has no full ToolSpec") {
			t.Fatalf("missing tool error = %v", err)
		}
		wire := schemaToolWire{
			ProductID: "other", Name: "run", CanonicalPath: "other.run", Path: "other.run",
			CLIPath: "other run", PrimaryCLIPath: "other run",
		}
		catalogWithTool := schemaCatalogWire{
			Kind: "schema", Level: "catalog", Source: "t", Count: 1, ToolCount: 1,
			Products: []schemaProductWire{{
				ID:    "sample",
				Tools: []schemaToolWire{{CanonicalPath: "other.run"}},
			}},
		}
		if _, _, err := schemaRegistryFromTyped(catalogWithTool, map[string]schemaToolWire{"other.run": wire}); err == nil || !strings.Contains(err.Error(), "belongs to product") {
			t.Fatalf("product mismatch error = %v", err)
		}
		if _, _, err := schemaRegistryFromTyped(schemaCatalogWire{
			Kind: "schema", Level: "catalog", Source: "t", Count: 1, ToolCount: 1,
			Products: []schemaProductWire{{ID: "sample"}},
		}, map[string]schemaToolWire{"orphan.run": wire}); err == nil || !strings.Contains(err.Error(), "absent from typed products") {
			t.Fatalf("orphan tool error = %v", err)
		}
	})

	t.Run("schemaToolWireFromPayload and schemaToolSpecFromPayload errors", func(t *testing.T) {
		if _, err := schemaToolWireFromPayload(map[string]any{"constraints": "bad"}); err == nil {
			t.Fatal("invalid tool payload must fail wire decode")
		}
		if _, err := schemaToolSpecFromPayload(map[string]any{"constraints": "bad"}); err == nil {
			t.Fatal("invalid tool payload must fail spec decode")
		}
	})
}

func TestCrossPlatformCoverageRuntimeSchemaNormalizeGroups(t *testing.T) {
	got := normalizeRuntimeSchemaGroups([][]string{{"b", "a"}}, 1)
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != "b" {
		t.Fatalf("require_one_of normalize = %#v", got)
	}
	got = normalizeRuntimeSchemaGroups([][]string{{"z", "y"}}, 0)
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != "z" {
		t.Fatalf("mutually_exclusive normalize = %#v", got)
	}
}

func TestCrossPlatformCoverageSchemaRuntimeRegistryRemainingBranches(t *testing.T) {
	t.Run("missing ProductDecl selection and ContractFinal param decl failure", func(t *testing.T) {
		entry := runtimeSchemaEntry{ProductID: "orphan", ToolName: "run", Command: &cobra.Command{Use: "run"}}
		if _, _, err := assembleProductSelection(entry); err == nil || !strings.Contains(err.Error(), "missing ProductDecl") {
			t.Fatalf("product selection error = %v", err)
		}
		root := buildRuntimeSchemaTestRoot()
		contract.RegisterProductDecl(contract.ProductDecl{ID: "doc", Selection: contract.ProductSelectionDecl{
			AgentSummary: "docs", UseWhen: []string{"doc"}, AvoidWhen: []string{"not doc"},
		}})
		t.Cleanup(func() { contract.ClearProductDeclForTest("doc") })
		create := root.Commands()[0].Commands()[0]
		contractfinal.RegisterRuntimeContractFinal(create, contract.ContractFinalPayload{
			Identity: &contract.ToolIdentitySpec{
				ProductID: "doc", Name: "create_document", CanonicalPath: "doc.create_document",
				CLIPath: "doc create", PrimaryCLIPath: "doc create",
			},

			Title: "Create", Description: "Create",
			Safety:     &contract.SafetySpec{Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "non_idempotent"},
			Interface:  &contract.InterfaceSpec{Mode: "local", Availability: "available", Reason: "test"},
			Selection:  &contract.SelectionSpec{AgentSummary: "create", UseWhen: []string{"create"}, AvoidWhen: []string{"not create"}, Examples: []string{"dws doc create --title x"}},
			Parameters: []contract.ParamDecl{{Name: "missing-flag", Property: "missing"}},
		})
		t.Cleanup(func() { contractfinal.ClearRuntimeContractFinalForTest(create) })
		bound, err := boundTestCommandRegistry(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtimeToolSpecFromMetadata(collectFirstEntry(t, bound), runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "ParamDecl") {
			t.Fatalf("param decl error = %v", err)
		}
	})

	t.Run("validateSchemaRegistryAgentMetadata problems", func(t *testing.T) {
		tool := ToolSpec{
			Identity:  contract.ToolIdentitySpec{CanonicalPath: "sample.run", Path: "sample.run", CLIPath: "sample run", PrimaryCLIPath: "sample run", ProductID: "sample", Name: "run"},
			Selection: contract.SelectionSpec{},
		}
		registry, err := SchemaRegistryFromRuntime("test", []ProductSpec{{ID: "sample", Tools: []ToolSpec{tool}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSchemaRegistryAgentMetadata(registry); err == nil || !strings.Contains(err.Error(), "no agent_summary") {
			t.Fatalf("empty agent summary error = %v", err)
		}
		t.Cleanup(ClearBuildTimeAgentMetadata)
		if err := InstallBuildTimeAgentMetadataJSON([]byte(`{"tools":{"missing tool":{"agent_summary":"x"}}}`)); err != nil {
			t.Fatal(err)
		}
		if err := validateSchemaRegistryAgentMetadata(registry); err == nil || !strings.Contains(err.Error(), "does not resolve") {
			t.Fatalf("missing metadata key error = %v", err)
		}
	})
}

func collectFirstEntry(t *testing.T, bound BoundCommandRegistry) runtimeSchemaEntry {
	t.Helper()
	entries, err := collectRuntimeSchemaEntriesFromBound(bound)
	if err != nil || len(entries) == 0 {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
	return entries[0]
}

func TestCrossPlatformCoverageCommandMetaRemainingBranches(t *testing.T) {
	t.Run("missing factory fail-closed panic guard", func(t *testing.T) {
		storeSchemaSourceRootFn(nil)
		assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
		resetMetaByCLIPathStateForTest()
		t.Cleanup(restorePackageCLISchemaDeliveryForTest)
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("ResolveMeta must panic when source root is missing")
			}
		}()
		ResolveMeta("dev app delete")
	})

	t.Run("buildMetaByCLIPath snapshot-only and skip blank cli paths", func(t *testing.T) {
		lookup := buildMetaByCLIPath(loadedSchemaCatalog{Snapshot: SchemaCatalogSnapshot{
			Tools: map[string]map[string]any{"a": {"cli_path": ""}},
		}})
		if len(lookup) != 0 {
			t.Fatalf("lookup = %#v", lookup)
		}
		registry := SchemaRegistry{Products: []ProductSpec{{ID: "sample", Tools: []ToolSpec{{
			Identity: contract.ToolIdentitySpec{
				CLIPath: " ", CanonicalPath: "sample.run", ProductID: "sample", Name: "run",
				Path: "sample.run", PrimaryCLIPath: " ",
			},
		}}}}}
		if len(buildMetaByCLIPathFromRegistry(registry)) != 0 {
			t.Fatal("blank cli_path tools must be skipped")
		}
	})

	t.Run("registerCommandMetaAliases skips blank and duplicate alias paths", func(t *testing.T) {
		meta := CommandMeta{Identity: CommandIdentity{CLIPath: "primary", Canonical: "p.a", Aliases: []string{" ", "primary", "alias"}}}
		lookup := registerCommandMetaAliases(map[string]CommandMeta{"primary": meta}, []CommandMeta{meta, {
			Identity: CommandIdentity{CLIPath: "other", Canonical: "p.b", Aliases: []string{"alias"}},
		}})
		if _, ok := lookup["alias"]; !ok {
			t.Fatal("first alias owner must win")
		}
	})
}

func TestCrossPlatformCoverageSchemaAgentMetadataRemainingBranches(t *testing.T) {
	if err := InstallBuildTimeAgentMetadataJSON([]byte(`{"products":{},"tools":{}}`)); err != nil {
		t.Fatalf("install empty maps error = %v", err)
	}
	t.Cleanup(ClearBuildTimeAgentMetadata)
	meta := runtimeAgentMetadata()
	if meta.Products == nil || meta.Tools == nil {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestCrossPlatformCoveragePublicRunnableSchemaLeafSubcommands(t *testing.T) {
	parent := &cobra.Command{Use: "parent", Run: func(*cobra.Command, []string) {}}
	parent.AddCommand(&cobra.Command{Use: "child"})
	if publicRunnableSchemaLeaf(parent) {
		t.Fatal("parent with subcommands must not be a runnable schema leaf")
	}
}

func TestCrossPlatformCoverageAgentExamplePlanAndTokenizerRemaining(t *testing.T) {
	idx := func(v int) *int { return &v }

	t.Run("empty and excess examples", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = nil
		})
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err == nil || !strings.Contains(err.Error(), "non-empty Selection.Examples") {
			t.Fatalf("empty examples error = %v", err)
		}
		bound, registry = crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{
				"dws sample run --name a",
				"dws sample run --name b",
				"dws sample run --name c",
			}
		})
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err == nil || !strings.Contains(err.Error(), "maximum is 2") {
			t.Fatalf("excess examples error = %v", err)
		}
	})

	t.Run("help tokens and path ordering", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"dws sample run -h"}
		})
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err == nil || !strings.Contains(err.Error(), "not only --help") {
			t.Fatalf("-h example error = %v", err)
		}
		root := &cobra.Command{Use: "dws"}
		leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
		leaf.Flags().String("name", "", "name")
		AttachRuntimeSchema(leaf, "sample", "run", "test")
		contractfinal.RegisterRuntimeContractFinal(leaf, contract.ContractFinalPayload{
			Identity: &contract.ToolIdentitySpec{
				ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
				CLIPath: "sample run", PrimaryCLIPath: "sample run",
			},

			Selection: &contract.SelectionSpec{Examples: []string{"dws sample alt run --name x"}},
		})
		t.Cleanup(func() { contractfinal.ClearRuntimeContractFinalForTest(leaf) })
		product := &cobra.Command{Use: "sample"}
		alt := &cobra.Command{Use: "alt", Run: func(*cobra.Command, []string) {}}
		alt.AddCommand(&cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}})
		product.AddCommand(leaf, alt)
		root.AddCommand(product)
		bound, err := boundTestCommandRegistry(root)
		if err != nil {
			t.Fatal(err)
		}
		registry, err = schemaRegistryForTest(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err == nil || !strings.Contains(err.Error(), "does not use its reviewed primary/alias path") {
			t.Fatalf("path mismatch error = %v", err)
		}
	})

	t.Run("typed registry tool missing from bound", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		bound.Commands = nil
		bound.ByCanonical = map[string]BoundCommandSpec{}
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err == nil || !strings.Contains(err.Error(), "missing from BoundCommandRegistry") {
			t.Fatalf("missing bound error = %v", err)
		}
	})

	t.Run("typed tool missing from registry map", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		registry.Products[0].Tools = nil
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err == nil || !strings.Contains(err.Error(), "missing from final typed SchemaRegistry") {
			t.Fatalf("missing typed tool error = %v", err)
		}
	})

	t.Run("tokenize tab backslash and quoted escape edges", func(t *testing.T) {
		if argv, err := tokenizeAgentExample("dws\tsample\trun"); err != nil || len(argv) != 3 {
			t.Fatalf("tab argv = %#v, %v", argv, err)
		}
		if _, err := tokenizeAgentExample(`dws sample run "trailing\`); err == nil || !strings.Contains(err.Error(), "trailing escape in double-quoted") {
			t.Fatalf("quoted trailing escape error = %v", err)
		}
		if _, err := tokenizeAgentExample(`dws sample run \`); err == nil || !strings.Contains(err.Error(), "trailing escape") {
			t.Fatalf("backslash escape error = %v", err)
		}
		if _, err := tokenizeAgentExample("dws sample run\r\n"); err == nil || !strings.Contains(err.Error(), "newline") {
			t.Fatalf("cr/lf error = %v", err)
		}
	})

	t.Run("validate cobra contract shorthand help and variadic positional", func(t *testing.T) {
		cmd := &cobra.Command{Use: "run"}
		cmd.Flags().String("name", "", "name")
		if err := validateAgentExampleCobraContract(cmd, []string{"-h"}, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "-h") {
			t.Fatalf("-h shorthand error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"--name=x"}, RuntimeSchemaConstraints{}, nil); err != nil {
			t.Fatalf("explicit long flag value error = %v", err)
		}
		positionalSpecs := []contract.RuntimeSchemaPositional{{Index: 0, Name: "files", Variadic: true, Required: true}}
		if err := validateAgentExampleCobraContract(cmd, []string{"a", "b"}, RuntimeSchemaConstraints{}, positionalSpecs); err != nil {
			t.Fatalf("variadic positional error = %v", err)
		}
	})

	t.Run("visit flags and parent shorthand lookup", func(t *testing.T) {
		parent := &cobra.Command{Use: "parent"}
		parent.PersistentFlags().StringP("token", "t", "", "token")
		child := &cobra.Command{Use: "child"}
		parent.AddCommand(child)
		count := 0
		visitAgentExampleCommandFlags(child, func(*pflag.Flag) { count++ })
		if count == 0 {
			t.Fatal("visitAgentExampleCommandFlags must visit persistent flags")
		}
		if flag := runtimeCommandFlagByShorthand(child, "t"); flag == nil || flag.Name != "token" {
			t.Fatalf("parent shorthand lookup = %#v", flag)
		}
	})

	t.Run("disposition dry_run copy path", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.DryRun = &contract.DryRunSpec{PreviewKind: "plan"}
		})
		t.Cleanup(func() { agentExampleSelectionFn = contractFinalToolSelection })
		agentExampleSelectionFn = func(cmd *cobra.Command) AgentToolSelection {
			selection := contractFinalToolSelection(cmd)
			selection.ExampleDispositions = []AgentExampleDisposition{{
				Index: idx(0), Mode: AgentExampleModeContractOnly, Reviewed: true,
				Reason: "stateful", ReasonCode: AgentExampleReasonStatefulPreflight,
			}}
			return selection
		}
		plan, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err != nil {
			t.Fatalf("plan error = %v", err)
		}
		if plan.Examples[0].DryRun == nil || plan.Examples[0].DryRun.PreviewKind != "plan" {
			t.Fatalf("execution dry_run = %#v", plan.Examples[0])
		}
	})
}

func TestCrossPlatformCoverageSchemaMetaIndexMoreBranches(t *testing.T) {
	t.Run("BuildSchemaMetaIndex duplicate cli_path", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Version:    SchemaCatalogSnapshotVersion,
			SourceHash: "hash",
			Tools: map[string]map[string]any{
				"a.run": {"cli_path": "same path", "canonical_path": "a.run"},
				"b.run": {"cli_path": "same path", "canonical_path": "b.run"},
			},
		}
		if _, err := BuildSchemaMetaIndex(snapshot); err == nil || !strings.Contains(err.Error(), "duplicate cli_path") {
			t.Fatalf("duplicate cli_path error = %v", err)
		}
	})

	t.Run("DecodeSchemaMetaIndexJSON decode and empty entries", func(t *testing.T) {
		if _, err := DecodeSchemaMetaIndexJSON([]byte("{")); err == nil || !strings.Contains(err.Error(), "decode schema meta index") {
			t.Fatalf("decode error = %v", err)
		}
		if _, err := DecodeSchemaMetaIndexJSON([]byte(`{"version":1,"source_hash":"x","entries":[]}`)); err == nil || !strings.Contains(err.Error(), "no entries") {
			t.Fatalf("empty entries error = %v", err)
		}
	})

	t.Run("ValidateSchemaMetaIndexAgainstSnapshot source_hash mismatch", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{SourceHash: "want"}
		index := SchemaMetaIndexSnapshot{Version: SchemaMetaIndexVersion, SourceHash: "got", Entries: []SchemaMetaIndexEntry{{CLIPath: "a"}}}
		if err := ValidateSchemaMetaIndexAgainstSnapshot(index, snapshot); err == nil || !strings.Contains(err.Error(), "source_hash") {
			t.Fatalf("source_hash error = %v", err)
		}
	})

	t.Run("compareCommandMetaLookups identity mismatch", func(t *testing.T) {
		got := map[string]CommandMeta{"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a", ProductID: "p"}}}
		want := map[string]CommandMeta{"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "p.a", ProductID: "other"}}}
		if err := compareCommandMetaLookups(got, want); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
			t.Fatalf("identity mismatch error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSchemaDryRunManualRegistryEdges(t *testing.T) {
	t.Cleanup(clearDeclaredDryRunCapabilitiesForTest)
	restore := setReviewedDryRunCapabilityGroupsForTest([]dryRunCapabilityGroup{{
		PreviewKind: "plan", CanonicalPaths: []string{" "},
	}})
	t.Cleanup(restore)
	if _, err := loadReviewedDryRunCapabilities(); err == nil || !strings.Contains(err.Error(), "empty canonical path") {
		t.Fatalf("empty canonical error = %v", err)
	}
	resetReviewedDryRunCapabilitiesLazyForTest()

	restore = setReviewedDryRunCapabilityGroupsForTest([]dryRunCapabilityGroup{{
		PreviewKind: "plan", CanonicalPaths: []string{"b.run", "a.run"},
	}})
	t.Cleanup(restore)
	if _, err := loadReviewedDryRunCapabilities(); err == nil || !strings.Contains(err.Error(), "not strictly sorted") {
		t.Fatalf("unsorted paths error = %v", err)
	}
	resetReviewedDryRunCapabilitiesLazyForTest()

	restore = setReviewedDryRunCapabilityGroupsForTest([]dryRunCapabilityGroup{{
		PreviewKind: "plan", CanonicalPaths: []string{"sample.run", "sample.run"},
	}})
	t.Cleanup(restore)
	if _, err := loadReviewedDryRunCapabilities(); err == nil || !strings.Contains(err.Error(), "not strictly sorted") {
		t.Fatalf("duplicate canonical error = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaCatalogAndSnapshotMoreBranches(t *testing.T) {
	t.Run("loadSchemaCatalogSnapshot unsupported version", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Version:    99,
			SourceHash: "x",
			Catalog:    map[string]any{"kind": "schema"},
			Tools:      map[string]map[string]any{"a": {"cli_path": "a"}},
		}
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("version error = %v", err)
		}
	})

	t.Run("schemaRegistryFromSnapshot minimal decode", func(t *testing.T) {
		testseam.Swap(t, &validateSchemaSnapshotTypedRoundTrip, false)
		snapshot := SchemaCatalogSnapshot{
			Catalog: map[string]any{"kind": "schema", "level": "catalog", "source": "t", "count": 1, "tool_count": 1, "products": []any{}},
			Tools:   map[string]map[string]any{},
		}
		if _, _, err := schemaRegistryFromSnapshot(snapshot); err != nil {
			t.Fatalf("minimal snapshot should decode: %v", err)
		}
	})

	t.Run("decodeStrictSchemaJSON multiple values", func(t *testing.T) {
		if err := decodeStrictSchemaJSON([]byte(`{} {}`), &struct{}{}); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
			t.Fatalf("multiple JSON error = %v", err)
		}
	})

	t.Run("buildMetaByCLIPathFromSnapshotTools aliases", func(t *testing.T) {
		lookup := buildMetaByCLIPathFromSnapshotTools(map[string]map[string]any{
			"sample.run": {"cli_path": "sample run", "canonical_path": "sample.run", "aliases": []any{"sample alias"}},
		})
		if _, ok := lookup["sample alias"]; !ok {
			t.Fatalf("lookup = %#v", lookup)
		}
	})
}
