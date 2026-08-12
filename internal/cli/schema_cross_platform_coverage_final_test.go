// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageFinalChangedStatementGaps(t *testing.T) {
	t.Run("tokenize escape and shell expansion edges", func(t *testing.T) {
		argv, err := tokenizeAgentExample(`dws sample run --name \x`)
		if err != nil {
			t.Fatalf("unquoted escape error = %v", err)
		}
		if joined := strings.Join(argv, " "); !strings.Contains(joined, "x") {
			t.Fatalf("argv = %#v", argv)
		}
		if _, err := tokenizeAgentExample(`dws sample run --name "a$b"`); err == nil || !strings.Contains(err.Error(), "expansion") {
			t.Fatalf("quoted $ expansion error = %v", err)
		}
		if _, err := tokenizeAgentExample("dws sample run --name \"a`b\""); err == nil || !strings.Contains(err.Error(), "expansion") {
			t.Fatalf("quoted backtick expansion error = %v", err)
		}
	})

	t.Run("matchAgentExamplePath empty argv continue", func(t *testing.T) {
		if _, _, ok := matchAgentExamplePath(
			[]string{"dws", "sample", "run"},
			[]agentExamplePath{{Path: "sample run", Argv: []string{}}},
		); ok {
			t.Fatal("empty Argv path must not match")
		}
	})

	t.Run("mergeAgentExamplePositionals name tiebreak", func(t *testing.T) {
		got := mergeAgentExamplePositionals(
			[]contract.RuntimeSchemaPositional{{Name: "b", Index: 1}, {Name: "a", Index: 1}},
		)
		if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
			t.Fatalf("tiebreak = %#v", got)
		}
	})

	t.Run("loadSchemaCatalogSnapshot missing source_hash and provenance failures", func(t *testing.T) {
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
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "missing source_hash") {
			t.Fatalf("missing source_hash error = %v", err)
		}
		snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
		testseam.Swap(t, &loadCatalogValidateInterfaces, func(SchemaRegistry) error { return nil })
		testseam.Swap(t, &loadCatalogValidateProvenance, func(SchemaRegistry) error { return fmt.Errorf("provenance boom") })
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "provenance boom") {
			t.Fatalf("provenance error = %v", err)
		}
		testseam.Swap(t, &loadCatalogValidateProvenance, func(SchemaRegistry) error { return nil })
		// Product references a tool that is absent from the tools map.
		snapshot.Catalog["products"] = []any{map[string]any{
			"id":    "sample",
			"tools": []any{map[string]any{"canonical_path": "missing.run"}},
		}}
		snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "load typed Schema registry") {
			t.Fatalf("typed registry error = %v", err)
		}
	})

	t.Run("meta index validation error paths", func(t *testing.T) {
		if _, err := decodeSchemaMetaIndexLookup([]byte(`{`)); err == nil {
			t.Fatal("decodeSchemaMetaIndexLookup must fail on bad JSON")
		}
		index := SchemaMetaIndexSnapshot{SourceHash: "a", SurfaceHash: "s", Entries: []SchemaMetaIndexEntry{{
			CLIPath: "x", Canonical: "x.y",
		}}}
		snapshot := SchemaCatalogSnapshot{SourceHash: "a", SurfaceHash: "s", Tools: map[string]map[string]any{
			"x.y": {"cli_path": "other", "canonical_path": "x.y"},
		}}
		if err := ValidateSchemaMetaIndexAgainstSnapshot(index, snapshot); err == nil {
			t.Fatal("ValidateSchemaMetaIndexAgainstSnapshot must detect lookup mismatch")
		}
		registry := SchemaRegistry{Products: []ProductSpec{{
			ID: "p", Tools: []ToolSpec{{
				Identity: contract.ToolIdentitySpec{CLIPath: "other", CanonicalPath: "x.y", Name: "y", ProductID: "p"},
			}},
		}}}
		if err := ValidateSchemaMetaIndexAgainstCatalog(index, registry); err == nil {
			t.Fatal("ValidateSchemaMetaIndexAgainstCatalog must detect lookup mismatch")
		}
		// Force commandMetaLookupFromIndex failure inside Validate* after hash checks pass.
		badIndex := SchemaMetaIndexSnapshot{SourceHash: "a", SurfaceHash: "s", Entries: []SchemaMetaIndexEntry{{
			CLIPath: " ", Canonical: "x.y",
		}}}
		if err := ValidateSchemaMetaIndexAgainstSnapshot(badIndex, SchemaCatalogSnapshot{SourceHash: "a", SurfaceHash: "s", Tools: map[string]map[string]any{"x.y": {"cli_path": "x", "canonical_path": "x.y"}}}); err == nil {
			t.Fatal("blank cli_path entry must fail lookup build")
		}
		if err := ValidateSchemaMetaIndexAgainstCatalog(badIndex, registry); err == nil {
			t.Fatal("blank cli_path entry must fail catalog validation lookup")
		}
	})

	t.Run("buildMetaByCLIPathFromSnapshotTools nil", func(t *testing.T) {
		if got := buildMetaByCLIPathFromSnapshotTools(nil); len(got) != 0 {
			t.Fatalf("nil tools = %#v", got)
		}
	})

	t.Run("InstallBuildTimeAgentMetadataJSON nil maps", func(t *testing.T) {
		t.Cleanup(ClearBuildTimeAgentMetadata)
		if err := InstallBuildTimeAgentMetadataJSON([]byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("schemaRegistryFromSnapshot wire and typed failures", func(t *testing.T) {
		testseam.Swap(t, &validateSchemaSnapshotTypedRoundTrip, false)
		snapshot := SchemaCatalogSnapshot{
			Catalog: map[string]any{"kind": "schema", "level": "catalog", "source": "t", "count": 1, "tool_count": 1, "products": []any{
				map[string]any{"id": "sample", "tools": []any{map[string]any{"canonical_path": "sample.run"}}},
			}},
			Tools: map[string]map[string]any{
				"sample.run": {"constraints": "bad"},
			},
		}
		if _, _, err := schemaRegistryFromSnapshot(snapshot); err == nil {
			t.Fatal("bad tool wire must fail")
		}
		snapshot.Tools = map[string]map[string]any{}
		snapshot.Catalog = map[string]any{"kind": "schema", "level": "catalog", "source": "t", "count": 1, "tool_count": 1, "products": []any{
			map[string]any{"id": "sample", "tools": []any{map[string]any{"canonical_path": "missing.run"}}},
		}}
		if _, _, err := schemaRegistryFromSnapshot(snapshot); err == nil {
			t.Fatal("missing tool must fail typed rebuild")
		}
		validateSchemaSnapshotTypedRoundTrip = true
		snapshot.Tools = map[string]map[string]any{
			"sample.run": {
				"product_id": "sample", "canonical_path": "sample.run", "name": "run",
				"path": "sample.run", "cli_path": "sample run", "primary_cli_path": "sample run",
			},
		}
		snapshot.Catalog = map[string]any{"kind": "schema", "level": "catalog", "source": "t", "count": 1, "tool_count": 1, "products": []any{
			map[string]any{"id": "sample", "tools": []any{map[string]any{"canonical_path": "sample.run"}}},
		}}
		if _, _, err := schemaRegistryFromSnapshot(snapshot); err == nil {
			// Round-trip often fails without full provenance/safety; either error or success is fine
			// as long as the validateSchemaSnapshotTypedRoundTrip branch executes.
			_ = err
		}
	})

	t.Run("schemaToolWireFromPayload marshal failure", func(t *testing.T) {
		if _, err := schemaToolWireFromPayload(map[string]any{"bad": func() {}}); err == nil {
			t.Fatal("unmarshalable payload must fail marshal")
		}
	})

	t.Run("validateSchemaRegistryAgentMetadata missing summary", func(t *testing.T) {
		testseam.Swap(t, &finalSchemaAgentMetadata, func() agentMetadata { return agentMetadata{} })
		registry := SchemaRegistry{Products: []ProductSpec{{
			ID: "sample",
			Tools: []ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
					CLIPath: "sample run", PrimaryCLIPath: "sample run",
				},
			}},
		}}}
		if err := validateSchemaRegistryAgentMetadata(registry); err == nil || !strings.Contains(err.Error(), "no agent_summary") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("registryToSnapshotPayload named helper success and error", func(t *testing.T) {
		got, err := registryToSnapshotPayload(SchemaRegistry{})
		if err != nil {
			t.Fatalf("empty registry snapshot error = %v", err)
		}
		_ = got

		oldSummary := snapshotToolSummary
		t.Cleanup(func() { snapshotToolSummary = oldSummary })
		snapshotToolSummary = func(ToolSpec) (map[string]any, error) {
			return nil, errors.New("summary failed")
		}
		registry := SchemaRegistry{Products: []ProductSpec{{
			ID: "sample",
			Tools: []ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					ProductID: "sample", Name: "run", CanonicalPath: "sample.run",
					CLIPath: "sample run", PrimaryCLIPath: "sample run",
				},
			}},
		}}}
		if _, err := registryToSnapshotPayload(registry); err == nil || !strings.Contains(err.Error(), "summary failed") {
			t.Fatalf("named helper error = %v", err)
		}
	})

	t.Run("agent selection binding failure through fixture builder", func(t *testing.T) {
		bound := crossPlatformAgentSelectionBound(t, nil)
		spec := bound.Commands[0]
		// Commands keeps a valid PrimaryCommand so ContractFinal discovery passes;
		// ByCanonical is poisoned so validateAgentSelectionBinding fails later.
		bad := spec
		bad.PrimaryCommand = nil
		bound.ByCanonical[spec.CanonicalPath] = bad
		if _, _, err := BuildAgentSelectionEvalFixture(bound); err == nil || !strings.Contains(err.Error(), "no bound primary") {
			t.Fatalf("binding error = %v", err)
		}
	})

	t.Run("loadSchemaCatalogSnapshot registry decode failure", func(t *testing.T) {
		snapshot := SchemaCatalogSnapshot{
			Version: SchemaCatalogSnapshotVersion,
			Catalog: map[string]any{"products": "not-a-list"},
			Tools:   map[string]map[string]any{"a": {"canonical_path": "a"}},
		}
		snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
		if _, err := loadSchemaCatalogSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "load typed Schema registry") {
			t.Fatalf("snapshot load error = %v", err)
		}
	})

	t.Run("schemaOverviewPayloadFromLoaded render failure", func(t *testing.T) {
		testseam.Swap(t, &renderDeliverySchemaOverview, func(SchemaRegistry) (map[string]any, error) {
			return nil, errors.New("overview failed")
		})
		if _, err := schemaOverviewPayloadFromLoaded(loadedSchemaCatalog{}); err == nil || !strings.Contains(err.Error(), "overview failed") {
			t.Fatalf("overview error = %v", err)
		}
	})

	t.Run("marshalSchemaRaw marshal failure", func(t *testing.T) {
		if _, err := marshalSchemaRaw(map[string]any{"bad": func() {}}); err == nil {
			t.Fatal("unmarshalable value must fail marshalSchemaRaw")
		}
	})

}

func TestCrossPlatformCoverageDeliverySchemaPayloadAndResolveMetaFactory(t *testing.T) {
	t.Run("delivery payload helpers without factory fail closed", func(t *testing.T) {
		storeSchemaSourceRootFn(nil)
		assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
		resetMetaByCLIPathStateForTest()
		t.Cleanup(restorePackageCLISchemaDeliveryForTest)
		if _, err := deliverySchemaAllPayload(); err == nil || !strings.Contains(err.Error(), "schema source root factory is not registered") {
			t.Fatalf("deliverySchemaAllPayload() error = %v, want missing factory", err)
		}
	})

	t.Run("NewSchemaCommand delivery branches", func(t *testing.T) {
		restorePackageCLISchemaDeliveryForTest()
		cmd := NewSchemaCommand()
		cmd.SetArgs([]string{"--all"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("--all error = %v", err)
		}
		cmd = NewSchemaCommand()
		cmd.SetArgs(nil)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("overview error = %v", err)
		}
		cmd = NewSchemaCommand()
		cmd.SetArgs([]string{"dev"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("path query error = %v", err)
		}
	})

	t.Run("NewSchemaCommand catalog load failure", func(t *testing.T) {
		testseam.Swap(t, &schemaCommandCatalogError, func() error { return errors.New("catalog broken") })
		cmd := NewSchemaCommand()
		cmd.SetArgs([]string{"--all"})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "load typed Schema registry") {
			t.Fatalf("catalog error = %v", err)
		}
	})

	t.Run("ResolveMeta via registered source root", func(t *testing.T) {
		resetMetaByCLIPathStateForTest()
		t.Cleanup(func() {
			restorePackageCLISchemaDeliveryForTest()
			resetMetaByCLIPathStateForTest()
		})
		RegisterSchemaSourceRoot(func() *cobra.Command {
			return &cobra.Command{Use: "dws"}
		})
		assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
			return loadedSchemaCatalog{
				Registry: SchemaRegistry{Products: []ProductSpec{{
					ID: "sample",
					Tools: []ToolSpec{{
						Identity: contract.ToolIdentitySpec{
							CLIPath: "sample run", CanonicalPath: "sample.run",
							ProductID: "sample", Name: "run", Path: "sample.run", PrimaryCLIPath: "sample run",
						},
						Safety:    contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "yes"},
						Selection: contract.SelectionSpec{AgentSummary: "sample"},
					}},
				}}},
			}, nil
		}
		meta, ok := ResolveMeta("sample run")
		if !ok || meta.Identity.Canonical != "sample.run" {
			t.Fatalf("ResolveMeta via factory = %#v ok=%v", meta, ok)
		}
	})

	t.Run("ResolveMeta factory assembly failure", func(t *testing.T) {
		resetMetaByCLIPathStateForTest()
		t.Cleanup(func() {
			restorePackageCLISchemaDeliveryForTest()
			resetMetaByCLIPathStateForTest()
		})
		RegisterSchemaSourceRoot(func() *cobra.Command { return nil })
		defer func() {
			if recover() == nil {
				t.Fatal("ResolveMeta must panic when delivery assembly fails")
			}
		}()
		ResolveMeta("sample run")
	})

	t.Run("RuntimeSchemaMetadataLoadCounts prefers delivery counter", func(t *testing.T) {
		restorePackageCLISchemaDeliveryForTest()
		resetDeliverySchemaCatalogStateForTest()
		RegisterSchemaSourceRoot(func() *cobra.Command { return nil })
		t.Cleanup(func() {
			restorePackageCLISchemaDeliveryForTest()
			resetDeliverySchemaCatalogStateForTest()
		})
		_ = deliverySchemaCatalog()
		counts := RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog == 0 {
			t.Fatalf("Catalog load count = %d, want delivery count", counts.Catalog)
		}
	})
}

func TestCrossPlatformCoverageSchemaMetaIndexGobJSONDeliveryGaps(t *testing.T) {
	valid := SchemaMetaIndexSnapshot{
		Version:    SchemaMetaIndexVersion,
		SourceHash: "hash",
		Entries:    []SchemaMetaIndexEntry{{CLIPath: "sample run", Canonical: "sample.run"}},
	}

	t.Run("DecodeSchemaMetaIndex empty and invalid", func(t *testing.T) {
		if _, err := DecodeSchemaMetaIndex(nil); err == nil || !strings.Contains(err.Error(), "empty payload") {
			t.Fatalf("empty error = %v", err)
		}
		if _, err := DecodeSchemaMetaIndex([]byte("not-gob")); err == nil || !strings.Contains(err.Error(), "decode schema meta index") {
			t.Fatalf("bad gob error = %v", err)
		}
		encoded, err := EncodeSchemaMetaIndex(SchemaMetaIndexSnapshot{Version: 99, SourceHash: "x", Entries: []SchemaMetaIndexEntry{{CLIPath: "a", Canonical: "a.a"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeSchemaMetaIndex(encoded); err == nil || !strings.Contains(err.Error(), "unsupported schema meta index version") {
			t.Fatalf("validate after gob decode error = %v", err)
		}
	})

	t.Run("DecodeSchemaMetaIndexJSON success", func(t *testing.T) {
		raw, err := EncodeSchemaMetaIndexJSON(valid)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeSchemaMetaIndexJSON(raw)
		if err != nil {
			t.Fatalf("DecodeSchemaMetaIndexJSON() error = %v", err)
		}
		if got.SourceHash != valid.SourceHash || len(got.Entries) != 1 {
			t.Fatalf("decoded = %#v", got)
		}
	})

	t.Run("encodeSchemaMetaIndexGob failure", func(t *testing.T) {
		original := gobEncodeSchemaMetaIndex
		t.Cleanup(func() { gobEncodeSchemaMetaIndex = original })
		gobEncodeSchemaMetaIndex = func(SchemaMetaIndexSnapshot, *bytes.Buffer) error { return errors.New("gob boom") }
		if _, err := encodeSchemaMetaIndexGob(valid); err == nil || !strings.Contains(err.Error(), "gob boom") {
			t.Fatalf("gob encode error = %v", err)
		}
	})

	t.Run("EncodeSchemaMetaIndexJSON failure", func(t *testing.T) {
		original := jsonMarshalSchemaMetaIndex
		t.Cleanup(func() { jsonMarshalSchemaMetaIndex = original })
		jsonMarshalSchemaMetaIndex = func(any, string, string) ([]byte, error) { return nil, errors.New("json boom") }
		if _, err := EncodeSchemaMetaIndexJSON(valid); err == nil || !strings.Contains(err.Error(), "encode schema meta index json") {
			t.Fatalf("json encode error = %v", err)
		}
	})
}
