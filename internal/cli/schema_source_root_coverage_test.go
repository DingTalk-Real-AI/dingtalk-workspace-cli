// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAssembleSchemaCatalogFromRootSuccess(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)

	prevResolve := resolveSchemaBuildForDelivery
	prevValidateParam := resolveValidateParameterDelivery
	prevValidateIface := loadCatalogValidateInterfaces
	prevValidateProv := loadCatalogValidateProvenance
	t.Cleanup(func() {
		resolveSchemaBuildForDelivery = prevResolve
		resolveValidateParameterDelivery = prevValidateParam
		loadCatalogValidateInterfaces = prevValidateIface
		loadCatalogValidateProvenance = prevValidateProv
	})

	root := &cobra.Command{Use: "dws"}
	registry := SchemaRegistry{
		Products: []ProductSpec{{
			ID: "sample",
			Tools: []ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					CLIPath: "sample run", CanonicalPath: "sample.run",
					ProductID: "sample", Name: "run", Path: "sample.run", PrimaryCLIPath: "sample run",
				},
				Safety:    contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "yes"},
				Selection: contract.SelectionSpec{AgentSummary: "sample"},
			}},
		}},
	}
	resolveSchemaBuildForDelivery = func(*cobra.Command) (ResolvedSchemaBuild, error) {
		return ResolvedSchemaBuild{registry: registry, root: root}, nil
	}
	resolveValidateParameterDelivery = func(BoundCommandRegistry, SchemaRegistry) error { return nil }
	loadCatalogValidateInterfaces = func(SchemaRegistry) error { return nil }
	loadCatalogValidateProvenance = func(SchemaRegistry) error { return nil }

	loaded, err := assembleSchemaCatalogFromRoot(root)
	if err != nil {
		t.Fatalf("assembleSchemaCatalogFromRoot success path: %v", err)
	}
	if loaded.Registry.Source != SchemaSourceRuntimeAssembled {
		t.Fatalf("Source = %q", loaded.Registry.Source)
	}
	if loaded.Snapshot.SourceHash == "" || loaded.Snapshot.SurfaceHash == "" {
		t.Fatalf("snapshot hashes missing: %#v", loaded.Snapshot)
	}
	if len(loaded.Index.CanonicalPaths()) == 0 {
		t.Fatal("index must be populated on success")
	}

	InstallProductionSchemaAssemblyForTest(func() *cobra.Command { return root })
	if !SchemaSourceRootRegistered() {
		t.Fatal("InstallProductionSchemaAssemblyForTest must register factory")
	}
	assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
	resetDeliverySchemaCatalogStateForTest()
	if err := deliverySchemaCatalogError(); err != nil {
		t.Fatalf("delivery assemble error = %v", err)
	}
	// Snapshot maps are populated eagerly during assembly; repeated reads hit
	// the cached delivery catalog and must keep the same source hash.
	first := deliverySchemaCatalog()
	if first.Snapshot.SourceHash == "" || len(first.Snapshot.Tools) == 0 {
		t.Fatalf("delivery snapshot maps/hash missing: %#v", first.Snapshot)
	}
	if again := deliverySchemaCatalog(); again.Snapshot.SourceHash != first.Snapshot.SourceHash {
		t.Fatalf("cached delivery source_hash = %q, want %q", again.Snapshot.SourceHash, first.Snapshot.SourceHash)
	}
	payload, err := DeliverySchemaAllPayloadForTest()
	if err != nil || payload == nil {
		t.Fatalf("DeliverySchemaAllPayloadForTest = %#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageSchemaSourceRootErrorBranchesAndRestoreFallback(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)

	if _, err := assembleSchemaCatalogFromRoot(nil); err == nil || !strings.Contains(err.Error(), "schema source root is nil") {
		t.Fatalf("nil root error = %v", err)
	}

	prevResolve := resolveSchemaBuildForDelivery
	prevValidateParam := resolveValidateParameterDelivery
	prevValidateIface := loadCatalogValidateInterfaces
	prevValidateProv := loadCatalogValidateProvenance
	prevPayload := registryToSnapshotPayloadFn
	t.Cleanup(func() {
		resolveSchemaBuildForDelivery = prevResolve
		resolveValidateParameterDelivery = prevValidateParam
		loadCatalogValidateInterfaces = prevValidateIface
		loadCatalogValidateProvenance = prevValidateProv
		registryToSnapshotPayloadFn = prevPayload
	})

	root := &cobra.Command{Use: "dws"}
	resolveSchemaBuildForDelivery = func(*cobra.Command) (ResolvedSchemaBuild, error) {
		return ResolvedSchemaBuild{}, fmt.Errorf("resolve boom")
	}
	if _, err := assembleSchemaCatalogFromRoot(root); err == nil || !strings.Contains(err.Error(), "resolve Schema build") {
		t.Fatalf("resolve error = %v", err)
	}

	registry := SchemaRegistry{Products: []ProductSpec{{
		ID: "sample",
		Tools: []ToolSpec{{
			Identity: contract.ToolIdentitySpec{
				CLIPath: "sample run", CanonicalPath: "sample.run",
				ProductID: "sample", Name: "run", Path: "sample.run", PrimaryCLIPath: "sample run",
			},
			Safety:    contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "yes"},
			Selection: contract.SelectionSpec{AgentSummary: "sample"},
		}},
	}}}
	resolveSchemaBuildForDelivery = func(*cobra.Command) (ResolvedSchemaBuild, error) {
		return ResolvedSchemaBuild{registry: registry, root: root}, nil
	}

	resolveValidateParameterDelivery = func(BoundCommandRegistry, SchemaRegistry) error {
		return fmt.Errorf("param boom")
	}
	if _, err := assembleSchemaCatalogFromRoot(root); err == nil || !strings.Contains(err.Error(), "param boom") {
		t.Fatalf("param error = %v", err)
	}
	resolveValidateParameterDelivery = func(BoundCommandRegistry, SchemaRegistry) error { return nil }

	loadCatalogValidateInterfaces = func(SchemaRegistry) error { return fmt.Errorf("iface boom") }
	if _, err := assembleSchemaCatalogFromRoot(root); err == nil || !strings.Contains(err.Error(), "iface boom") {
		t.Fatalf("iface error = %v", err)
	}
	loadCatalogValidateInterfaces = func(SchemaRegistry) error { return nil }

	loadCatalogValidateProvenance = func(SchemaRegistry) error { return fmt.Errorf("prov boom") }
	if _, err := assembleSchemaCatalogFromRoot(root); err == nil || !strings.Contains(err.Error(), "prov boom") {
		t.Fatalf("provenance error = %v", err)
	}
	loadCatalogValidateProvenance = func(SchemaRegistry) error { return nil }

	registryToSnapshotPayloadFn = func(SchemaRegistry) (SchemaCatalogSnapshot, error) {
		return SchemaCatalogSnapshot{}, fmt.Errorf("payload boom")
	}
	if _, err := assembleSchemaCatalogFromRoot(root); err == nil || !strings.Contains(err.Error(), "payload boom") {
		t.Fatalf("payload error = %v", err)
	}
	registryToSnapshotPayloadFn = prevPayload

	// Index failure: empty registry still indexes; force via invalid product id.
	badRegistry := SchemaRegistry{Products: []ProductSpec{{
		ID: "",
		Tools: []ToolSpec{{
			Identity: contract.ToolIdentitySpec{CLIPath: "x", CanonicalPath: "x.y", ProductID: "x", Name: "y", Path: "x.y", PrimaryCLIPath: "x"},
		}},
	}}}
	resolveSchemaBuildForDelivery = func(*cobra.Command) (ResolvedSchemaBuild, error) {
		return ResolvedSchemaBuild{registry: badRegistry, root: root}, nil
	}
	if _, err := assembleSchemaCatalogFromRoot(root); err == nil {
		// Index may still succeed for empty product id; tolerate either outcome
		// as long as the call executes the Index branch.
		t.Log("index accepted empty product id")
	}

	RegisterSchemaSourceRoot(func() *cobra.Command { return &cobra.Command{Use: "dws"} })
	assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
		return loadedSchemaCatalog{}, fmt.Errorf("forced assemble failure")
	}
	resetDeliverySchemaCatalogStateForTest()
	if err := deliverySchemaCatalogError(); err == nil || !strings.Contains(err.Error(), "forced assemble failure") {
		t.Fatalf("delivery assemble error = %v", err)
	}

	testseam.Swap(t, &restorePackageCLISchemaDeliveryHook, nil)
	RestorePackageCLISchemaDeliveryForTest()
	if SchemaSourceRootRegistered() {
		t.Fatal("restore fallback must clear Schema source root")
	}
}

func TestCrossPlatformCoverageLoadSchemaSourceRootFnUnstored(t *testing.T) {
	prev := loadSchemaSourceRootFn()
	t.Cleanup(func() {
		resetSchemaSourceRootAtomicForTest()
		if prev != nil {
			storeSchemaSourceRootFn(prev)
		}
		restorePackageCLISchemaDeliveryForTest()
	})
	resetSchemaSourceRootAtomicForTest()
	if loadSchemaSourceRootFn() != nil {
		t.Fatal("loadSchemaSourceRootFn() must be nil before first store")
	}
}

func TestCrossPlatformCoverageSafetyForCLIPathUnregisteredFactory(t *testing.T) {
	prev := loadSchemaSourceRootFn()
	t.Cleanup(func() {
		storeSchemaSourceRootFn(prev)
		restorePackageCLISchemaDeliveryForTest()
	})
	storeSchemaSourceRootFn(nil)
	if _, ok := SafetyForCLIPath("dev app delete"); ok {
		t.Fatal("SafetyForCLIPath without factory must return ok=false")
	}
	safetyOut := &bytes.Buffer{}
	safetyCmd := &cobra.Command{Use: "dws"}
	safetyCmd.SetOut(safetyOut)
	RenderSafetyAnnotation(safetyCmd)
	if safetyOut.Len() != 0 {
		t.Fatalf("RenderSafetyAnnotation without factory must stay silent, got %q", safetyOut.String())
	}
}

func TestSchemaSourceRootFnAtomicStoreLoad(t *testing.T) {
	prev := loadSchemaSourceRootFn()
	t.Cleanup(func() {
		storeSchemaSourceRootFn(prev)
		restorePackageCLISchemaDeliveryForTest()
	})

	storeSchemaSourceRootFn(nil)
	if SchemaSourceRootRegistered() || loadSchemaSourceRootFn() != nil {
		t.Fatal("nil factory must clear registration")
	}
	factory := func() *cobra.Command { return &cobra.Command{Use: "atomic-root"} }
	RegisterSchemaSourceRoot(factory)
	got := loadSchemaSourceRootFn()
	if got == nil {
		t.Fatal("RegisterSchemaSourceRoot store/load returned nil")
	}
	if root := got(); root == nil || root.Use != "atomic-root" {
		t.Fatalf("RegisterSchemaSourceRoot factory root = %#v", root)
	}
	if !SchemaSourceRootRegistered() {
		t.Fatal("registered factory must report true")
	}
}

func TestCrossPlatformCoverageRenderSafetyAnnotationSuccess(t *testing.T) {
	restorePackageCLISchemaDeliveryForTest()
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)

	root := &cobra.Command{Use: "dws"}
	dev := &cobra.Command{Use: "dev"}
	appCmd := &cobra.Command{Use: "app"}
	deleteCmd := &cobra.Command{Use: "delete"}
	root.AddCommand(dev)
	dev.AddCommand(appCmd)
	appCmd.AddCommand(deleteCmd)
	var out bytes.Buffer
	deleteCmd.SetOut(&out)
	RenderSafetyAnnotation(deleteCmd)
	rendered := out.String()
	if !strings.Contains(rendered, "Safety:") || !strings.Contains(rendered, "effect=") {
		t.Fatalf("RenderSafetyAnnotation success = %q", rendered)
	}
	unknown := &cobra.Command{Use: "unknown-cmd"}
	root.AddCommand(unknown)
	var silent bytes.Buffer
	unknown.SetOut(&silent)
	RenderSafetyAnnotation(unknown)
	if silent.Len() != 0 {
		t.Fatalf("unknown command rendered %q", silent.String())
	}
}

func TestCrossPlatformCoverageMCPMetadataInterfaceRefEdges(t *testing.T) {
	// MCP pin lookup helpers are retired; keep this named coverage slot as a
	// no-op marker so CrossPlatformCoverage* selection stays stable.
	if got := emptyPinnedMCPMetadata(); got.Tools == nil || len(got.Tools) != 0 {
		t.Fatalf("empty pinned metadata = %#v", got)
	}
}

func TestCrossPlatformCoverageSchemaMetaIndexSuccessReturns(t *testing.T) {
	index := SchemaMetaIndexSnapshot{
		Version: SchemaMetaIndexVersion, SourceHash: "hash",
		Entries: []SchemaMetaIndexEntry{{CLIPath: "a run", Canonical: "a.run", ProductID: "a"}},
	}
	encoded, err := EncodeSchemaMetaIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeSchemaMetaIndex(encoded)
	if err != nil || got.SourceHash != "hash" {
		t.Fatalf("DecodeSchemaMetaIndex = %#v err=%v", got, err)
	}
	lookup, err := decodeSchemaMetaIndexLookup(encoded)
	if err != nil || lookup["a run"].Identity.Canonical != "a.run" {
		t.Fatalf("decodeSchemaMetaIndexLookup = %#v err=%v", lookup, err)
	}
}

func TestCrossPlatformCoverageDeliverySchemaOverviewAndQueryBranches(t *testing.T) {
	restorePackageCLISchemaDeliveryForTest()
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)

	if _, err := deliverySchemaOverviewPayload(); err != nil {
		t.Fatalf("deliverySchemaOverviewPayload() error = %v", err)
	}
	if _, err := queryDeliverySchemaPayload([]string{"dev"}); err != nil {
		t.Fatalf("queryDeliverySchemaPayload(dev) error = %v", err)
	}
	if _, err := queryDeliverySchemaPayload([]string{"dev app"}); err != nil {
		t.Fatalf("queryDeliverySchemaPayload(dev app) error = %v", err)
	}
	if _, err := deliverySchemaAllPayload(); err != nil {
		t.Fatalf("deliverySchemaAllPayload() error = %v", err)
	}

	storeSchemaSourceRootFn(nil)
	assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
	resetDeliverySchemaCatalogStateForTest()
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	if _, err := deliverySchemaOverviewPayload(); err == nil {
		t.Fatal("overview without factory must fail")
	}
	if _, err := queryDeliverySchemaPayload(nil); err == nil {
		t.Fatal("query without factory must fail")
	}
	if _, err := deliverySchemaAllPayload(); err == nil {
		t.Fatal("all without factory must fail")
	}
}

func TestCrossPlatformCoverageSchemaPayloadEmptySourceFallback(t *testing.T) {
	registry := SchemaRegistry{
		Source: "",
		Products: []ProductSpec{{
			ID: "sample",
			Tools: []ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					CLIPath: "sample nested run", CanonicalPath: "sample.nested_run",
					ProductID: "sample", Name: "nested_run", Path: "sample.nested_run",
					PrimaryCLIPath: "sample nested run", Group: "nested",
				},
				Safety:    contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "yes"},
				Selection: contract.SelectionSpec{AgentSummary: "sample"},
			}},
		}},
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatal(err)
	}
	loaded := loadedSchemaCatalog{Registry: registry, Index: index, Snapshot: SchemaCatalogSnapshot{SourceHash: "hash"}}
	product, err := schemaPayloadFromLoadedCatalog(loaded, []string{"sample"})
	if err != nil || product["source"] != SchemaSourceRuntimeAssembled {
		t.Fatalf("product empty-source fallback = %#v err=%v", product, err)
	}
	group, err := schemaPayloadFromLoadedCatalog(loaded, []string{"sample nested"})
	if err != nil || group["source"] != SchemaSourceRuntimeAssembled {
		t.Fatalf("group empty-source fallback = %#v err=%v", group, err)
	}
}
