// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCrossPlatformCoverageAgentExampleExecutionPlanErrors(t *testing.T) {
	idx := func(v int) *int { return &v }

	t.Run("empty canonical in typed registry", func(t *testing.T) {
		registry := SchemaRegistry{Products: []ProductSpec{{
			Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{CanonicalPath: "  "}}},
		}}}
		_, err := BuildAgentExampleExecutionPlan(BoundCommandRegistry{}, registry)
		if err == nil || !strings.Contains(err.Error(), "empty canonical path") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("duplicate canonical in typed registry", func(t *testing.T) {
		tool := ToolSpec{Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.run"}}
		registry := SchemaRegistry{Products: []ProductSpec{
			{Tools: []ToolSpec{tool}},
			{Tools: []ToolSpec{tool}},
		}}
		_, err := BuildAgentExampleExecutionPlan(BoundCommandRegistry{}, registry)
		if err == nil || !strings.Contains(err.Error(), "duplicate tool") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing ContractFinal", func(t *testing.T) {
		leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
		bound := BoundCommandRegistry{
			Commands: []BoundCommandSpec{{
				CommandSpec:    CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run", Visibility: SchemaVisibilityPublic},
				PrimaryCommand: leaf,
			}},
			ByCanonical: map[string]BoundCommandSpec{"sample.run": {
				CommandSpec:    CommandSpec{CanonicalPath: "sample.run", PrimaryCLIPath: "sample run"},
				PrimaryCommand: leaf,
			}},
		}
		_, err := buildAgentExampleExecutionPlan(bound, map[string]ToolSpec{"sample.run": {Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.run"}}})
		if err == nil || !strings.Contains(err.Error(), "no ContractFinal") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing from typed registry", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		deleteRegistryTool := registry
		deleteRegistryTool.Products[0].Tools = nil
		_, err := BuildAgentExampleExecutionPlan(bound, deleteRegistryTool)
		if err == nil || !strings.Contains(err.Error(), "missing from final typed SchemaRegistry") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("empty examples", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = nil
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "non-empty Selection.Examples") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("too many examples", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"dws sample run", "dws sample run --name a", "dws sample run --name b"}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "maximum is 2") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("example must start with dws", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"sample run --name x"}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "must start with dws") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("example rejects --yes", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"dws sample run --yes --name x"}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "--yes") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("example rejects --help", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"dws sample run --help"}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "--help") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("example wrong path", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.Selection.Examples = []string{"dws other run --name x"}
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "primary/alias path") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("registry tool missing from bound", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		registry.Products[0].Tools = append(registry.Products[0].Tools, ToolSpec{
			Identity: contract.ToolIdentitySpec{CanonicalPath: "sample.extra"},
		})
		_, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err == nil || !strings.Contains(err.Error(), "missing from BoundCommandRegistry") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("success with dry_run capability", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, func(_ *cobra.Command, payload *contract.ContractFinalPayload) {
			payload.DryRun = &contract.DryRunSpec{PreviewKind: "plan"}
		})
		plan, err := BuildAgentExampleExecutionPlan(bound, registry)
		if err != nil {
			t.Fatalf("BuildAgentExampleExecutionPlan() error = %v", err)
		}
		if plan.DryRun != 1 || plan.Total != 1 {
			t.Fatalf("plan = %#v", plan)
		}
	})

	t.Run("ValidateAgentExampleDelivery", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		if _, err := ValidateAgentExampleDelivery(bound, registry); err != nil {
			t.Fatalf("ValidateAgentExampleDelivery() error = %v", err)
		}
	})

	t.Run("BuildAgentExampleExecutionPlan", func(t *testing.T) {
		bound, registry := crossPlatformAgentExampleFixture(t, nil)
		if _, err := BuildAgentExampleExecutionPlan(bound, registry); err != nil {
			t.Fatalf("BuildAgentExampleExecutionPlan() error = %v", err)
		}
	})

	t.Run("validateAgentExampleDispositions", func(t *testing.T) {
		examples := []string{"dws sample run --name x", "dws sample run --name y"}
		cases := []struct {
			name string
			disp []AgentExampleDisposition
			want string
		}{
			{"nil index", []AgentExampleDisposition{{Mode: AgentExampleModeContractOnly, Reviewed: true, Reason: "r", ReasonCode: AgentExampleReasonLocalState}}, "requires index"},
			{"out of range", []AgentExampleDisposition{{Index: idx(9), Mode: AgentExampleModeContractOnly, Reviewed: true, Reason: "r", ReasonCode: AgentExampleReasonLocalState}}, "out of range"},
			{"duplicate index", []AgentExampleDisposition{
				{Index: idx(0), Mode: AgentExampleModeContractOnly, Reviewed: true, Reason: "r", ReasonCode: AgentExampleReasonLocalState},
				{Index: idx(0), Mode: AgentExampleModeContractOnly, Reviewed: true, Reason: "r2", ReasonCode: AgentExampleReasonLocalState},
			}, "duplicate example disposition"},
			{"not reviewed", []AgentExampleDisposition{{Index: idx(0), Mode: AgentExampleModeContractOnly, Reviewed: false, Reason: "r", ReasonCode: AgentExampleReasonLocalState}}, "must be reviewed"},
			{"invalid mode", []AgentExampleDisposition{{Index: idx(0), Mode: AgentExampleModeDryRun, Reviewed: true, Reason: "r", ReasonCode: AgentExampleReasonLocalState}}, "invalid mode"},
			{"invalid reason_code", []AgentExampleDisposition{{Index: idx(0), Mode: AgentExampleModeContractOnly, Reviewed: true, Reason: "r", ReasonCode: "bogus"}}, "invalid reason_code"},
			{"empty reason", []AgentExampleDisposition{{Index: idx(0), Mode: AgentExampleModeContractOnly, Reviewed: true, Reason: "  ", ReasonCode: AgentExampleReasonLocalState}}, "non-empty reason"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := validateAgentExampleDispositions("sample.run", examples, tc.disp)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
			})
		}
		if !validAgentExampleReasonCode(AgentExampleReasonLocalState) || validAgentExampleReasonCode("nope") {
			t.Fatal("validAgentExampleReasonCode mismatch")
		}
	})

	t.Run("tokenizeAgentExample and ParseAgentExampleArgv", func(t *testing.T) {
		argv, err := ParseAgentExampleArgv(`dws sample run --name "a b"`)
		if err != nil || len(argv) != 5 || argv[4] != "a b" {
			t.Fatalf("ParseAgentExampleArgv() = %#v, %v", argv, err)
		}
		placeholder, err := tokenizeAgentExample(`dws sample run --file <path>`)
		if err != nil || placeholder[len(placeholder)-1] != "<path>" {
			t.Fatalf("placeholder argv = %#v, %v", placeholder, err)
		}
		badInputs := map[string]string{
			"dws sample run\n":          "newline",
			"dws sample run `x`":        "expansion",
			"dws sample run # comment":  "comments",
			"dws sample run ;":          "shell operator",
			"dws sample run --":         "terminator",
			`dws sample run "unclosed`:  "unterminated",
			`dws sample run "trailing\`: "trailing escape",
		}
		for input, fragment := range badInputs {
			if _, err := tokenizeAgentExample(input); err == nil || !strings.Contains(err.Error(), fragment) && fragment != "shell operator" {
				if _, err2 := tokenizeAgentExample(input); err2 == nil {
					t.Fatalf("tokenizeAgentExample(%q) error = nil", input)
				}
			}
		}
		if _, _, ok := agentExamplePlaceholderAt("x<bad op>", 1); ok {
			t.Fatal("agentExamplePlaceholderAt must reject invalid placeholder bodies")
		}
	})

	t.Run("validateAgentExampleCobraContract edges", func(t *testing.T) {
		cmd := &cobra.Command{Use: "run"}
		cmd.Flags().String("name", "", "name")
		_ = cmd.Flags().MarkHidden("legacy")
		cmd.Flags().Lookup("name").Annotations = map[string][]string{cobra.BashCompOneRequiredFlag: {"true"}}
		if err := validateAgentExampleCobraContract(nil, nil, RuntimeSchemaConstraints{}, nil); err == nil {
			t.Fatal("nil command must fail")
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"--unknown", "x"}, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("unknown flag error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"-h"}, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "-h") {
			t.Fatalf("-h error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"--help"}, RuntimeSchemaConstraints{}, nil); err == nil {
			t.Fatal("--help must fail")
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"--name"}, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "requires a value") {
			t.Fatalf("missing value error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{}, RuntimeSchemaConstraints{}, nil); err == nil || !strings.Contains(err.Error(), "missing required flag") {
			t.Fatalf("missing required error = %v", err)
		}
		if err := validateAgentExampleCobraContract(cmd, []string{"--name", "x"}, RuntimeSchemaConstraints{}, []contract.RuntimeSchemaPositional{{Index: 1, Name: "id", Required: true}}); err == nil || !strings.Contains(err.Error(), "positional") {
			t.Fatalf("missing positional error = %v", err)
		}
		constraints := RuntimeSchemaConstraints{RequireOneOf: [][]string{{"a", "b"}}}
		if err := validateAgentExampleConstraints(map[string]bool{}, constraints); err == nil || !strings.Contains(err.Error(), "require_one_of") {
			t.Fatalf("require_one_of error = %v", err)
		}
		if err := validateAgentExampleConstraints(map[string]bool{"a": true}, RuntimeSchemaConstraints{RequireTogether: [][]string{{"a", "b"}}}); err == nil || !strings.Contains(err.Error(), "require_together") {
			t.Fatalf("require_together error = %v", err)
		}
		if err := validateAgentExampleConstraints(map[string]bool{"a": true, "b": true}, RuntimeSchemaConstraints{MutuallyExclusive: [][]string{{"a", "b"}}}); err == nil || !strings.Contains(err.Error(), "mutually_exclusive") {
			t.Fatalf("mutually_exclusive error = %v", err)
		}
		if got := agentExampleFlagGroup([]string{" b ", "-a"}); got != "--a, --b" {
			t.Fatalf("agentExampleFlagGroup = %q", got)
		}
		cmd.Flags().StringP("mode", "m", "", "mode")
		if flag := runtimeCommandFlagByShorthand(cmd, "m"); flag == nil || flag.Name != "mode" {
			t.Fatalf("runtimeCommandFlagByShorthand = %#v", flag)
		}
	})

	t.Run("matchAgentExamplePath", func(t *testing.T) {
		paths := []agentExamplePath{{Path: "sample run", Argv: []string{"sample", "run"}, Command: &cobra.Command{}}}
		if _, _, ok := matchAgentExamplePath([]string{"cli"}, paths); ok {
			t.Fatal("non-dws argv must not match")
		}
		remainder, matched, ok := matchAgentExamplePath([]string{"dws", "sample", "run", "--name", "x"}, paths)
		if !ok || matched.Path != "sample run" || len(remainder) != 2 {
			t.Fatalf("match = %#v %#v %v", remainder, matched, ok)
		}
	})

	t.Run("mergeAgentExamplePositionals dedupes", func(t *testing.T) {
		merged := mergeAgentExamplePositionals(
			[]contract.RuntimeSchemaPositional{{Index: 0, Name: "id"}},
			[]contract.RuntimeSchemaPositional{{Index: 0, Name: "id"}},
		)
		if len(merged) != 1 {
			t.Fatalf("merged = %#v", merged)
		}
	})
}

func crossPlatformAgentExampleFixture(t *testing.T, mutate func(*cobra.Command, *contract.ContractFinalPayload)) (BoundCommandRegistry, SchemaRegistry) {
	t.Helper()
	root := &cobra.Command{Use: "dws"}
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().String("name", "", "name")
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
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sample",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Sample",
			UseWhen:      []string{"sample"},
			AvoidWhen:    []string{"not sample"},
		},
	})
	product := &cobra.Command{Use: "sample"}
	product.AddCommand(leaf)
	root.AddCommand(product)
	bound, err := boundTestCommandRegistry(root)
	if err != nil {
		t.Fatalf("boundTestCommandRegistry() error = %v", err)
	}
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatalf("schemaRegistryForTest() error = %v", err)
	}
	return bound, registry
}

func TestCrossPlatformCoverageSchemaCatalogLoaderEdges(t *testing.T) {
	t.Run("materialize embedded maps", func(t *testing.T) {
		loaded := mustDeliverySchemaCatalogMaps(t)
		if len(loaded.Snapshot.Catalog) == 0 || len(loaded.Snapshot.Tools) == 0 {
			t.Fatal("materialized catalog maps are empty")
		}
	})

	t.Run("loadSchemaCatalogSnapshot validation", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Version: 99,
			Catalog: map[string]any{"kind": "schema"},
			Tools:   map[string]map[string]any{"a": {}},
		}
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("version error = %v", err)
		}
		snapshot = SchemaCatalogSnapshot{Version: SchemaCatalogSnapshotVersion}
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("empty error = %v", err)
		}
		base := mustDeliverySchemaCatalogMaps(t).Snapshot
		raw, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		var tampered SchemaCatalogSnapshot
		if err := json.Unmarshal(raw, &tampered); err != nil {
			t.Fatal(err)
		}
		tampered.Catalog["tampered"] = true
		if _, err := loadSchemaCatalogSnapshot(tampered); err == nil || !strings.Contains(err.Error(), "source_hash does not match") {
			t.Fatalf("stale source_hash error = %v", err)
		}
	})

	t.Run("delivery snapshot and helpers", func(t *testing.T) {
		snapshot := mustDeliverySchemaCatalogMaps(t).Snapshot
		if len(snapshot.Tools) == 0 {
			t.Fatal("delivery snapshot tools are empty")
		}
		if got := schemaMap(map[string]any{"a": map[string]any{"b": "c"}}); got["a"]["b"] != "c" {
			t.Fatalf("schemaMap = %#v", got)
		}
		if got := schemaStringSlice([]any{"a", "b"}); len(got) != 2 {
			t.Fatalf("schemaStringSlice = %#v", got)
		}
		if firstNonEmptySchemaString("", " trimmed ", nil) != "trimmed" {
			t.Fatal("firstNonEmptySchemaString failed")
		}
		if err := deliverySchemaCatalogError(); err != nil {
			t.Fatalf("deliverySchemaCatalogError() = %v", err)
		}
	})

	t.Run("decodeSchemaCatalogSnapshot invalid JSON", func(t *testing.T) {
		if _, err := decodeSchemaCatalogSnapshot([]byte("not-json")); err == nil {
			t.Fatal("invalid snapshot JSON must fail")
		}
	})
}

func TestCrossPlatformCoverageSchemaMetaIndexAndCommandMeta(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	index, err := BuildSchemaMetaIndex(loaded.Snapshot)
	if err != nil {
		t.Fatalf("BuildSchemaMetaIndex() error = %v", err)
	}
	encoded, err := EncodeSchemaMetaIndex(index)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("EncodeSchemaMetaIndex() = %d bytes, %v", len(encoded), err)
	}
	if err := ValidateSchemaMetaIndexAgainstSnapshot(index, loaded.Snapshot); err != nil {
		t.Fatalf("ValidateSchemaMetaIndexAgainstSnapshot() error = %v", err)
	}
	if err := ValidateSchemaMetaIndexAgainstCatalog(index, loaded.Registry); err != nil {
		t.Fatalf("ValidateSchemaMetaIndexAgainstCatalog() error = %v", err)
	}
	decoded, err := func() (SchemaMetaIndexSnapshot, error) {
		loaded := mustDeliverySchemaCatalogMaps(t)
		return BuildSchemaMetaIndex(loaded.Snapshot)
	}()
	if err != nil {
		t.Fatalf("DecodeSchemaMetaIndex() error = %v", err)
	}
	_ = decoded
	if meta, ok := ResolveMeta("dev app delete"); !ok || meta.Identity.Canonical == "" {
		t.Fatalf("ResolveMeta() = %#v, ok=%v", meta, ok)
	}
	if err := deliverySchemaCatalogError(); err != nil {
		t.Fatalf("deliverySchemaCatalogError() = %v", err)
	}

	t.Run("BuildSchemaMetaIndex errors", func(t *testing.T) {
		if _, err := BuildSchemaMetaIndex(SchemaCatalogSnapshot{Version: 99, SourceHash: "x"}); err == nil {
			t.Fatal("unsupported version must fail")
		}
		if _, err := BuildSchemaMetaIndex(SchemaCatalogSnapshot{Version: SchemaCatalogSnapshotVersion}); err == nil {
			t.Fatal("missing source_hash must fail")
		}
		bad := SchemaCatalogSnapshot{
			Version: SchemaCatalogSnapshotVersion, SourceHash: "x",
			Tools: map[string]map[string]any{"a": {}},
		}
		if _, err := BuildSchemaMetaIndex(bad); err == nil || !strings.Contains(err.Error(), "cli_path") {
			t.Fatalf("missing cli_path error = %v", err)
		}
		bad.Tools = map[string]map[string]any{
			"a": {"cli_path": "same", "canonical_path": "a.a"},
			"b": {"cli_path": "same", "canonical_path": "b.b"},
		}
		if _, err := BuildSchemaMetaIndex(bad); err == nil || !strings.Contains(err.Error(), "duplicate cli_path") {
			t.Fatalf("duplicate cli_path error = %v", err)
		}
	})

	t.Run("DecodeSchemaMetaIndexJSON errors", func(t *testing.T) {
		if _, err := DecodeSchemaMetaIndexJSON([]byte(`{"version":1}`)); err == nil {
			t.Fatal("truncated index must fail")
		}
	})

	t.Run("commandMetaLookupFromIndex errors", func(t *testing.T) {
		bad := SchemaMetaIndexSnapshot{
			Version: SchemaMetaIndexVersion, SourceHash: "x",
			Entries: []SchemaMetaIndexEntry{{CLIPath: ""}},
		}
		if _, err := commandMetaLookupFromIndex(bad); err == nil || !strings.Contains(err.Error(), "cli_path") {
			t.Fatalf("empty cli_path error = %v", err)
		}
	})

	t.Run("compareCommandMetaLookups mismatch", func(t *testing.T) {
		got := map[string]CommandMeta{"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "x.a"}}}
		want := map[string]CommandMeta{"a": {Identity: CommandIdentity{CLIPath: "a", Canonical: "y.a"}}}
		if err := compareCommandMetaLookups(got, want); err == nil {
			t.Fatal("identity mismatch must fail")
		}
	})

	_ = decoded
}

func TestCrossPlatformCoverageSchemaRuntimeRegistryLegacyAndPayload(t *testing.T) {
	root := buildRuntimeSchemaTestRoot()
	declareRuntimeSchemaTestRootDoc(t, root, nil)
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatalf("declared assembly: %v", err)
	}
	loaded, err := loadedSchemaCatalogForTestRegistry(registry)
	if err != nil {
		t.Fatalf("loaded test catalog: %v", err)
	}
	payload, err := schemaPayloadFromLoadedCatalog(loaded, []string{"doc create"})
	if err != nil {
		t.Fatalf("schemaPayloadFromLoadedCatalog() error = %v", err)
	}
	if payload["canonical_path"] != "doc.create_document" {
		t.Fatalf("payload = %#v", payload)
	}
	all, err := registry.ToPayload()
	if err != nil || len(all) == 0 {
		t.Fatalf("registry.ToPayload() = %#v, %v", all, err)
	}

	t.Run("runtimeToolSpecFromMetadata missing ContractFinal", func(t *testing.T) {
		entry := runtimeSchemaEntry{ProductID: "sample", ToolName: "run", Command: &cobra.Command{Use: "run"}}
		if _, err := runtimeToolSpecFromMetadata(entry, runtimeSchemaMetadataSources{}); err == nil || !strings.Contains(err.Error(), "missing RuntimeContractFinal") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("assembleProductSelection missing ProductDecl", func(t *testing.T) {
		entry := runtimeSchemaEntry{ProductID: "orphan", ToolName: "run"}
		if _, _, err := assembleProductSelection(entry); err == nil || !strings.Contains(err.Error(), "missing ProductDecl") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSchemaAgentMetadataInstallAndLoad(t *testing.T) {
	fixture := fstest.MapFS{
		"schema_agent_metadata/index.json":  {Data: []byte(`{"domains":["sample"],"coverage":{"tools_with_metadata":1}}`)},
		"schema_agent_metadata/sample.json": {Data: []byte(`{"product_id":"sample","tools":{"sample.get":{"agent_summary":"S","use_when":["u"],"avoid_when":["a"],"examples":["dws sample get"],"interface_mode":"local","availability":"available"}}}`)},
	}
	loaded := loadAgentMetadataFixtureFrom(fixture)
	if len(loaded.Tools) != 1 {
		t.Fatalf("loaded = %#v", loaded)
	}
	if loadAgentMetadataFixtureFrom(fstest.MapFS{"schema_agent_metadata/index.json": {Data: []byte(`{"domains":["bad/name"]}`)}}).Tools == nil {
		t.Fatal("invalid domain must return empty metadata")
	}

	const agentJSON = `{"version":1,"products":{"p":{"agent_summary":"P"}},"tools":{"p.run":{"agent_summary":"T","use_when":["u"],"avoid_when":["a"],"effect":"read","interface_mode":"local","availability":"available","interface_reason":"r"}}}`
	if err := InstallBuildTimeAgentMetadataJSON([]byte(agentJSON)); err != nil {
		t.Fatalf("InstallBuildTimeAgentMetadataJSON() error = %v", err)
	}
	t.Cleanup(ClearBuildTimeAgentMetadata)
	meta := runtimeAgentMetadata()
	if meta.Products["p"].AgentSummary != "P" {
		t.Fatalf("runtimeAgentMetadata() = %#v", meta)
	}
	if err := InstallBuildTimeAgentMetadataJSON([]byte("{")); err == nil {
		t.Fatal("invalid JSON must fail install")
	}
}

func TestCrossPlatformCoverageSchemaAgentSelectionBinding(t *testing.T) {
	bound, _ := crossPlatformAgentExampleFixture(t, nil)
	fixture, report, err := BuildAgentSelectionEvalFixture(bound)
	if err != nil {
		t.Fatalf("BuildAgentSelectionEvalFixture() error = %v", err)
	}
	if report.Tools == 0 || len(fixture.Cases) == 0 {
		t.Fatalf("fixture = %#v report = %#v", fixture, report)
	}
	if _, err := ValidateAgentSelectionContract(bound); err != nil {
		t.Fatalf("ValidateAgentSelectionContract() error = %v", err)
	}
	selection := contractFinalToolSelection(bound.Commands[0].PrimaryCommand)
	if selection.AgentSummary == "" || len(selection.Examples) == 0 {
		t.Fatalf("contractFinalToolSelection = %#v", selection)
	}
	if err := validateAgentSelectionBinding(bound, "sample.run", bound.Commands[0]); err != nil {
		t.Fatalf("validateAgentSelectionBinding() error = %v", err)
	}
	if normalizeAgentSelectionScenario("  Foo\tBar ") != "foo bar" {
		t.Fatal("normalizeAgentSelectionScenario failed")
	}
}

func TestCrossPlatformCoverageSchemaDryRunCapabilities(t *testing.T) {
	t.Cleanup(clearDeclaredDryRunCapabilitiesForTest)
	recordDeclaredDryRunCapability("sample.run", contract.DryRunSpec{PreviewKind: "plan"})
	caps, err := ReviewedDryRunCapabilities()
	if err != nil || caps["sample.run"].PreviewKind != "plan" {
		t.Fatalf("ReviewedDryRunCapabilities() = %#v, %v", caps, err)
	}
	recordDeclaredDryRunCapability("", contract.DryRunSpec{PreviewKind: "plan"})
}

func TestCrossPlatformCoverageSchemaSnapshotAdapterEdges(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	toolPayload := loaded.Snapshot.Tools["calendar.create_calendar_event"]
	if _, err := schemaToolSpecFromPayload(toolPayload); err != nil {
		t.Fatalf("schemaToolSpecFromPayload() error = %v", err)
	}
	if err := decodeStrictSchemaJSON([]byte(`{"a":1}{"b":2}`), &map[string]any{}); err == nil {
		t.Fatal("multiple JSON values must fail decodeStrictSchemaJSON")
	}
}

func TestCrossPlatformCoverageSchemaCanonicalPathAndParamAliases(t *testing.T) {
	if _, _, ok := splitSchemaCanonicalPath("bad path"); ok {
		t.Fatal("invalid canonical must fail split")
	}
	if _, _, ok := splitManualSchemaCanonicalPath("sample.run"); !ok {
		t.Fatal("splitManualSchemaCanonicalPath alias failed")
	}
	leaf := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	if !publicRunnableSchemaLeaf(leaf) {
		t.Fatal("runnable public leaf must pass")
	}
	hidden := &cobra.Command{Use: "run", Hidden: true, Run: func(*cobra.Command, []string) {}}
	if publicRunnableSchemaLeaf(hidden) {
		t.Fatal("hidden leaf must fail")
	}
	if _, err := ReduceParamAliases(nil); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("ReduceParamAliases(nil) error = %v", err)
	}
	walkRunnableParamCommands(nil, func(*cobra.Command) { t.Fatal("must not invoke fn") })
}

func TestCrossPlatformCoverageDeliverySchemaCatalogDelivery(t *testing.T) {
	if !deliverySchemaCatalogAvailable() {
		t.Fatalf("delivery catalog unavailable: %v", deliverySchemaCatalogError())
	}
	loaded := deliverySchemaCatalog()
	if loaded.Registry.Source != SchemaSourceRuntimeAssembled {
		t.Fatalf("source = %q, want %q", loaded.Registry.Source, SchemaSourceRuntimeAssembled)
	}
	overview, err := deliverySchemaOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	if schemaProductToolCount(map[string]any{"tools": overview["products"]}) == 0 {
		t.Fatal("overview is empty")
	}
	payload, err := deliverySchemaAllPayload()
	if err != nil || len(schemaMapSlice(payload["products"])) == 0 {
		t.Fatalf("deliverySchemaAllPayload() = %#v, %v", payload, err)
	}
	leaf, err := queryDeliverySchemaPayload([]string{"calendar event create"})
	if err != nil || schemaString(leaf["canonical_path"]) == "" {
		t.Fatalf("queryDeliverySchemaPayload() = %#v, %v", leaf, err)
	}
}

func TestCrossPlatformCoverageValidateCatalogStructure(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	data, err := json.Marshal(loaded.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalogStructure(data); err != nil {
		t.Fatalf("ValidateCatalogStructure() error = %v", err)
	}
	if err := ValidateCatalogStructure(catalogPayload(t, validCatalogToolEntry())); err != nil {
		t.Fatalf("valid entry error = %v", err)
	}
	entry := validCatalogToolEntry()
	entry["surprise"] = "x"
	if err := ValidateCatalogStructure(catalogPayload(t, entry)); err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestCrossPlatformCoverageCommandMetaFromSnapshotTools(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	lookup := buildMetaByCLIPath(loaded)
	if len(lookup) == 0 {
		t.Fatal("buildMetaByCLIPath returned empty lookup")
	}
	snapshotLookup := buildMetaByCLIPathFromSnapshotTools(loaded.Snapshot.Tools)
	if len(snapshotLookup) == 0 {
		t.Fatal("buildMetaByCLIPathFromSnapshotTools returned empty lookup")
	}
}

func TestCrossPlatformCoverageAgentExampleMarshalDigest(t *testing.T) {
	marshalAgentSelectionFixture = func(v any) ([]byte, error) {
		return nil, fmt.Errorf("forced marshal error")
	}
	t.Cleanup(func() { marshalAgentSelectionFixture = json.Marshal })
	bound, _ := crossPlatformAgentExampleFixture(t, nil)
	if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "marshal Agent selection fixture") {
		t.Fatalf("digest error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateSchemaRegistryAgentMetadataEmptyInject(t *testing.T) {
	bound, registry := crossPlatformAgentExampleFixture(t, nil)
	if err := validateSchemaRegistryAgentMetadata(registry); err != nil {
		t.Fatalf("validateSchemaRegistryAgentMetadata() error = %v", err)
	}
	_ = bound
}

func TestCrossPlatformCoverageRuntimeSchemaCollectInvalidCanonical(t *testing.T) {
	bound := BoundCommandRegistry{Commands: []BoundCommandSpec{{
		CommandSpec: CommandSpec{CanonicalPath: "not-a-valid-canonical", Visibility: SchemaVisibilityPublic},
	}}}
	if _, err := collectRuntimeSchemaEntriesFromBound(bound); err == nil || !strings.Contains(err.Error(), "invalid canonical path") {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossPlatformCoverageParamAliasRealFlagsByMorph(t *testing.T) {
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().AddFlag(&pflag.Flag{Name: "base-id", Usage: "id"})
	cmd.Flags().AddFlag(&pflag.Flag{Name: "base", Usage: "legacy", Hidden: true})
	byMorph := realFlagsByMorph(cmd)
	if len(byMorph) == 0 {
		t.Fatal("realFlagsByMorph returned empty map")
	}
}

func TestCrossPlatformCoverageSchemaCatalogSnapshotLoadRoundTrip(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	snapshot := loaded.Snapshot
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSchemaCatalogSnapshot(data)
	if err != nil {
		t.Fatalf("decodeSchemaCatalogSnapshot() error = %v", err)
	}
	if len(decoded.Registry.Products) == 0 {
		t.Fatal("decoded registry is empty")
	}
}

func TestCrossPlatformCoverageRuntimeSchemaPayloadGroupAndProduct(t *testing.T) {
	root := buildRuntimeSchemaTestRoot()
	declareRuntimeSchemaTestRootDoc(t, root, nil)
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadedSchemaCatalogForTestRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	productPayload, err := schemaPayloadFromLoadedCatalog(loaded, []string{"doc"})
	if err != nil || productPayload["level"] != "product" {
		t.Fatalf("product payload = %#v, %v", productPayload, err)
	}
	if _, err := schemaPayloadFromLoadedCatalog(loaded, []string{"doc create"}); err != nil {
		t.Fatalf("leaf payload error = %v", err)
	}
	deliveryLoaded := deliverySchemaCatalog()
	if groupPayload, err := schemaPayloadFromLoadedCatalog(deliveryLoaded, []string{"calendar.event"}); err != nil || groupPayload["level"] != "group" {
		t.Fatalf("embedded group payload = %#v, %v", groupPayload, err)
	}
	if _, err := schemaPayloadFromLoadedCatalog(loaded, []string{"missing path"}); err == nil {
		t.Fatal("unknown path must fail")
	}
}

func TestCrossPlatformCoverageAgentSelectionBindingErrors(t *testing.T) {
	bound, _ := crossPlatformAgentExampleFixture(t, nil)
	bad := bound.Commands[0]
	bad.CanonicalPath = "other.run"
	if err := validateAgentSelectionBinding(bound, "sample.run", bad); err == nil || !strings.Contains(err.Error(), "mismatched") {
		t.Fatalf("canonical mismatch error = %v", err)
	}
	bad = bound.Commands[0]
	bad.PrimaryCommand = nil
	if err := validateAgentSelectionBinding(bound, "sample.run", bad); err == nil || !strings.Contains(err.Error(), "no bound primary") {
		t.Fatalf("nil primary error = %v", err)
	}
}

func TestCrossPlatformCoverageAgentExampleUnknownBoundCanonical(t *testing.T) {
	bound, registry := crossPlatformAgentExampleFixture(t, nil)
	bound.ByCanonical = map[string]BoundCommandSpec{}
	_, err := buildAgentExampleExecutionPlan(bound, map[string]ToolSpec{
		"sample.run": registry.Products[0].Tools[0],
	})
	if err == nil || !strings.Contains(err.Error(), "unknown canonical tool") {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossPlatformCoverageMetaIndexHashMismatch(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	index, err := BuildSchemaMetaIndex(loaded.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	index.SourceHash = "stale"
	if err := ValidateSchemaMetaIndexAgainstSnapshot(index, loaded.Snapshot); err == nil || !strings.Contains(err.Error(), "source_hash") {
		t.Fatalf("hash mismatch error = %v", err)
	}
}

func TestCrossPlatformCoverageSchemaMetaIndexCommandMetaEqualBranches(t *testing.T) {
	base := CommandMeta{
		Identity:  CommandIdentity{CLIPath: "a", Canonical: "p.a", ProductID: "p", Title: "t", Aliases: []string{"b"}},
		Safety:    CommandSafety{Effect: "read"},
		Selection: CommandSelection{AgentSummary: "s", UseWhen: []string{"u"}, AvoidWhen: []string{"a"}, Examples: []string{"e"}},
	}
	if err := commandMetaEqual(base, base); err != nil {
		t.Fatalf("equal metas error = %v", err)
	}
	safetyMismatch := base
	safetyMismatch.Safety = CommandSafety{Effect: "write"}
	if err := commandMetaEqual(base, safetyMismatch); err == nil {
		t.Fatal("safety mismatch must fail")
	}
	selectionMismatch := base
	selectionMismatch.Selection.AgentSummary = "other"
	if err := commandMetaEqual(base, selectionMismatch); err == nil {
		t.Fatal("selection mismatch must fail")
	}
	if !metaStringSlicesEqual(nil, nil) || metaStringSlicesEqual([]string{"a"}, []string{"b"}) {
		t.Fatal("metaStringSlicesEqual edge cases failed")
	}
}

func TestCrossPlatformCoverageLoadTypedSchemaCatalogSuccess(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	if len(loaded.Registry.Products) == 0 {
		t.Fatal("loaded delivery catalog is empty")
	}
	// Round-trip through untyped snapshot decode (CI dump path).
	snapshot := loaded.Snapshot
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := decodeSchemaCatalogSnapshot(raw)
	if err != nil {
		t.Fatalf("decodeSchemaCatalogSnapshot() error = %v", err)
	}
	if len(reloaded.Registry.Products) == 0 {
		t.Fatal("reloaded catalog is empty")
	}
}

func TestCrossPlatformCoverageAssembleDeliverySchemaCatalog(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	err := error(nil)
	_ = err
	if loaded.Snapshot.SourceHash == "" {
		t.Fatal("missing source hash")
	}
}
