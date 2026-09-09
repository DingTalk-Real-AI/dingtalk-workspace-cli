// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSchemaCacheDeliveryRemainder(t *testing.T) {
	runtimeCache, identity, _ := publishCoverageSchemaRuntime(t)
	goos, goarch := coverageCacheGOOSARCH()
	counters := &schemacache.Counters{}
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: goos, GOARCH: goarch,
		Counters: counters, RuntimeEligible: func() bool { return true },
		LockTimeout: 50 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeCache = activeSchemaCacheRuntime()
	if runtimeCache == nil {
		t.Fatal("runtime not registered")
	}
	if _, err := runtimeCache.opened(); err != nil {
		t.Fatal(err)
	}

	resetDeliverySchemaCatalogStateForTest()
	if _, err := runtimeCache.loadOverviewPayload(); err != nil {
		t.Fatal(err)
	}
	if _, err := DeliverySchemaOverviewPayloadForTest(); err != nil {
		t.Fatal(err)
	}
	index, err := runtimeCache.readPayloadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.PayloadDescriptors) == 0 {
		t.Fatal("no payload products")
	}
	productID := index.PayloadDescriptors[0].ProductID
	first, err := runtimeCache.loadCommandPayload(index, productID)
	if err != nil {
		t.Fatal(err)
	}
	var cliPath, canonical string
	for path, command := range first.Commands {
		cliPath, canonical = path, command.Identity.Canonical
		if command.Identity.CLIPath == path && canonical != "" {
			break
		}
	}
	if cliPath == "" || canonical == "" {
		t.Fatal("no command in payload")
	}
	if _, err := runtimeCache.loadQueryPayload(cliPath); err != nil {
		t.Fatal(err)
	}
	if _, err := DeliverySchemaQueryPayloadForTest(cliPath); err != nil {
		t.Fatal(err)
	}
	if _, err := DeliverySchemaAllPayloadForTest(); err != nil {
		t.Fatal(err)
	}
	if _, ok := ResolveMeta(cliPath); !ok {
		t.Fatal("ResolveMeta cache hit missed")
	}

	cmd := NewSchemaCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{canonical, "--compact"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("compact leaf write was empty")
	}

	failOpen := &schemaCacheRuntime{
		options:  SchemaCacheOptions{Identity: SchemaCacheIdentity{Edition: "!!!invalid"}},
		products: make(map[string]*schemaCacheProductLoad),
		payloads: make(map[string]*schemaCachePayloadLoad),
	}
	meta, err := runtimeCache.readMeta()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failOpen.queryPayload(meta, productID, true); err == nil {
		t.Fatal("invalid edition queryPayload succeeded")
	}

	broken := &schemaCacheRuntime{
		options:  SchemaCacheOptions{Identity: identity},
		products: make(map[string]*schemaCacheProductLoad),
		payloads: make(map[string]*schemaCachePayloadLoad),
	}
	broken.options.Identity.Registry.EncodedSHA256 = sha256.Sum256([]byte("wrong-registry-remainder"))
	broken.seedMeta(meta)
	if _, err := broken.loadAllPayload(); err == nil {
		t.Fatal("broken registry loadAllPayload succeeded")
	}

	restorePackageCLISchemaDeliveryForTest()
	resetDeliverySchemaCatalogStateForTest()
	poisoned := activeSchemaCacheRuntime()
	if poisoned == nil {
		if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
			Enabled: true, Identity: identity, GOOS: goos, GOARCH: goarch,
			Counters: counters, RuntimeEligible: func() bool { return true },
			LockTimeout: 50 * time.Millisecond,
		}); err != nil {
			t.Fatal(err)
		}
		poisoned = activeSchemaCacheRuntime()
	}
	if poisoned == nil {
		t.Fatal("runtime missing before repair remainder")
	}
	poisoned.options.Identity.SourceSHA256 = sha256.Sum256([]byte("wrong-source-remainder"))
	if _, err := deliverySchemaAllPayload(); err != nil {
		t.Fatal(err)
	}
	resetDeliverySchemaCatalogStateForTest()
	if _, err := deliverySchemaOverviewPayload(); err != nil {
		t.Fatal(err)
	}
	resetDeliverySchemaCatalogStateForTest()
	if _, err := queryDeliverySchemaPayload([]string{cliPath}); err != nil {
		t.Fatal(err)
	}

	restorePackageCLISchemaDeliveryForTest()
	resetDeliverySchemaCatalogStateForTest()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: goos, GOARCH: goarch,
		Counters: counters, RuntimeEligible: func() bool { return true },
		LockTimeout: 50 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	locked := activeSchemaCacheRuntime()
	cache, err := locked.opened()
	if err != nil {
		t.Fatal(err)
	}
	held, err := cache.AcquireLock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repairSchemaCache(locked, func() (any, error) {
		return nil, errors.New("recheck while locked")
	}); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: goos, GOARCH: goarch,
		Counters: counters, RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	PrewarmSchemaCache()
	AwaitSchemaCachePrewarmForTest()
	if SchemaCachePrewarmPayloadsHandleForTest() == nil {
		t.Fatal("prewarm payloads handle missing")
	}
	prewarmed := activeSchemaCacheRuntime()
	if _, err := prewarmed.opened(); err != nil {
		t.Fatal(err)
	}
	if _, err := prewarmed.loadPayloadIndex(); err != nil {
		t.Fatal(err)
	}
	if _, err := prewarmed.payloadsHandle(); err != nil {
		t.Fatal(err)
	}
	prewarmed.resetPayloadsHandle()

	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: goos, GOARCH: goarch,
		RuntimeEligible: func() bool { return false },
	}); err != nil {
		t.Fatal(err)
	}
	if activeSchemaCacheRuntime() != nil {
		t.Fatal("ineligible runtime stayed active")
	}

	t.Cleanup(func() {
		buildCatalogValidateParameterBindings = ValidateSchemaParameterBindingDelivery
		buildCatalogValidateDryRun = ValidateReviewedDryRunCapabilityDelivery
		buildCatalogValidateExamples = ValidateAgentExampleDelivery
		buildCatalogValidateCompleteness = validateResolvedRuntimeSchemaCompleteness
		buildCatalogValidateRegistry = validateSchemaRegistryAgainstCommandRegistry
		buildCatalogValidateInterfaces = validateSchemaRegistryInterfaces
		buildCatalogValidateAgentMetadata = validateSchemaRegistryAgentMetadata
		buildCatalogValidateProvenance = validateFinalSchemaProvenanceCoverage
		buildCatalogValidateDelivery = ValidateSchemaDeliveryInvariants
		buildCatalogValidateFinalCompleteness = validateResolvedSchemaCatalogDeliveryCompleteness
	})
	buildCatalogValidateParameterBindings = func(BoundCommandRegistry, SchemaRegistry) error { return nil }
	buildCatalogValidateDryRun = func(SchemaRegistry) error { return nil }
	buildCatalogValidateExamples = func(BoundCommandRegistry, SchemaRegistry) (AgentExampleExecutionPlan, error) {
		return AgentExampleExecutionPlan{}, nil
	}
	buildCatalogValidateCompleteness = func(*cobra.Command, BoundCommandRegistry) error { return nil }
	buildCatalogValidateRegistry = func(SchemaRegistry, EffectiveCommandRegistry) error { return nil }
	buildCatalogValidateInterfaces = func(SchemaRegistry) error { return nil }
	buildCatalogValidateAgentMetadata = func(SchemaRegistry) error { return nil }
	buildCatalogValidateProvenance = func(SchemaRegistry) error { return nil }
	buildCatalogValidateDelivery = func(SchemaRegistry, SchemaCatalogSnapshot) error { return nil }
	buildCatalogValidateFinalCompleteness = func(*cobra.Command, BoundCommandRegistry, SchemaCatalogSnapshot) error { return nil }
	restorePackageCLISchemaDeliveryForTest()
	loaded := deliverySchemaCatalog()
	if _, err := BuildSchemaCacheArtifacts(ResolvedSchemaBuild{
		root:     &cobra.Command{Use: "dws"},
		registry: loaded.Registry,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageSchemaCacheRepairLiveFallbacks(t *testing.T) {
	_, identity, _ := publishCoverageSchemaRuntime(t)
	goos, goarch := coverageCacheGOOSARCH()
	register := func() *schemaCacheRuntime {
		t.Helper()
		restorePackageCLISchemaDeliveryForTest()
		resetDeliverySchemaCatalogStateForTest()
		if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
			Enabled: true, Identity: identity, GOOS: goos, GOARCH: goarch,
			RuntimeEligible: func() bool { return true },
		}); err != nil {
			t.Fatal(err)
		}
		runtimeCache := activeSchemaCacheRuntime()
		if runtimeCache == nil {
			t.Fatal("runtime not registered")
		}
		return runtimeCache
	}

	runtimeCache := register()
	runtimeCache.allOnce.Do(func() { runtimeCache.allErr = errors.New("poison all") })
	if _, err := deliverySchemaAllPayload(); err != nil {
		t.Fatal(err)
	}

	runtimeCache = register()
	runtimeCache.metaOnce.Do(func() { runtimeCache.metaErr = errors.New("poison meta") })
	if _, err := deliverySchemaOverviewPayload(); err != nil {
		t.Fatal(err)
	}

	runtimeCache = register()
	runtimeCache.metaOnce.Do(func() { runtimeCache.metaErr = errors.New("poison meta") })
	if _, err := queryDeliverySchemaPayload([]string{"calendar event create"}); err != nil {
		t.Fatal(err)
	}

	runtimeCache = register()
	runtimeCache.options.Identity.SourceSHA256 = sha256.Sum256([]byte("wrong-source-repair-hit"))
	if _, err := deliverySchemaAllPayload(); err != nil {
		t.Fatal(err)
	}

	runtimeCache = register()
	runtimeCache.options.Identity.SourceSHA256 = sha256.Sum256([]byte("wrong-source-repair-overview"))
	if _, err := deliverySchemaOverviewPayload(); err != nil {
		t.Fatal(err)
	}

	runtimeCache = register()
	runtimeCache.options.Identity.SourceSHA256 = sha256.Sum256([]byte("wrong-source-repair-query"))
	if _, err := queryDeliverySchemaPayload([]string{"calendar event create"}); err != nil {
		t.Fatal(err)
	}
}
