// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func coverageSchemaCacheHome(t *testing.T) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	testHome, err := os.MkdirTemp(home, ".dws-schema-cache-remaining-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(testHome) })
	t.Setenv("HOME", testHome)
	cacheBase := filepath.Join(testHome, ".cache")
	if runtime.GOOS == "darwin" {
		cacheBase = filepath.Join(testHome, "Library", "Caches")
	}
	if err := os.MkdirAll(cacheBase, 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", cacheBase)
	}
}

func publishCoverageSchemaRuntime(t *testing.T) (*schemaCacheRuntime, SchemaCacheIdentity, SchemaCacheArtifacts) {
	t.Helper()
	skipWithoutPersistentSchemaCache(t)
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	coverageSchemaCacheHome(t)
	restorePackageCLISchemaDeliveryForTest()
	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	identity := coverageIdentityFromArtifacts(t, artifacts)
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Publish(identity.ExpectedIdentity(), artifacts.RegistryArtifact(), artifacts.MetaArtifact(), artifacts.PayloadArtifact()); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RegisterSchemaCacheOptions(SchemaCacheOptions{}) })
	runtimeCache := activeSchemaCacheRuntime()
	if runtimeCache == nil {
		t.Fatal("runtime not registered")
	}
	return runtimeCache, identity, artifacts
}

func TestCrossPlatformCoverageSchemaCachePayloadInnerReadyReturn(t *testing.T) {
	runtimeCache, _, _ := publishCoverageSchemaRuntime(t)
	index, err := runtimeCache.readPayloadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.PayloadDescriptors) == 0 {
		t.Fatal("no payload products")
	}
	productID := index.PayloadDescriptors[0].ProductID

	var passed atomic.Int32
	firstBlocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	testseam.Swap(t, &schemaCachePayloadLoadBeforeInnerLock, func() {
		if passed.Add(1) == 1 {
			close(firstBlocked)
			<-releaseFirst
		}
	})

	done := make(chan error, 2)
	go func() {
		_, loadErr := runtimeCache.loadCommandPayload(index, productID)
		done <- loadErr
	}()
	<-firstBlocked
	go func() {
		_, loadErr := runtimeCache.loadCommandPayload(index, productID)
		done <- loadErr
	}()
	if err := <-done; err != nil {
		t.Fatalf("second payload load: %v", err)
	}
	close(releaseFirst)
	if err := <-done; err != nil {
		t.Fatalf("first payload load: %v", err)
	}
}

func TestCrossPlatformCoverageSchemaCacheRuntimeRemainingPayloadPaths(t *testing.T) {
	runtimeCache, identity, artifacts := publishCoverageSchemaRuntime(t)
	index, err := runtimeCache.readPayloadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.PayloadDescriptors) == 0 {
		t.Fatal("no payload products")
	}
	productID := index.PayloadDescriptors[0].ProductID

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := runtimeCache.loadCommandPayload(index, productID)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent loadCommandPayload: %v", err)
		}
	}

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
	if cliPath == "" {
		t.Fatal("no command in payload")
	}

	if _, err := runtimeCache.readCommandMetaFromPayloadFresh(cliPath); err != nil {
		t.Fatal(err)
	}

	ghost := index
	ghost.LocatorProductByPath = map[string]string{}
	for path, id := range index.LocatorProductByPath {
		ghost.LocatorProductByPath[path] = id
	}
	ghost.LocatorProductByPath["ghost.tool"] = "no-such-product"
	ghost.LocatorProductByPath["not-a-command"] = productID
	runtimeCache.seedPayloadIndex(ghost)
	if _, _, err := runtimeCache.resolveCommandMetaFromPayload("ghost.tool"); err == nil {
		t.Fatal("missing product resolve succeeded")
	}
	if _, ok := runtimeCache.renderedCompactLeaf("ghost.tool"); ok {
		t.Fatal("missing product compact leaf succeeded")
	}
	if _, ok := runtimeCache.renderedCompactLeaf("not-a-command"); ok {
		t.Fatal("unknown command compact leaf succeeded")
	}

	payloadPath := filepath.Join(mustOpenedDir(t, runtimeCache), "payloads.shards.cache")
	body, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	desc, ok := schemareader.PayloadDescriptor(index, productID)
	if !ok {
		t.Fatalf("missing payload descriptor for %q", productID)
	}
	headerOff := schemacache.HeaderSize + int(identity.PayloadIndexLength+desc.Offset)
	if headerOff >= len(body) {
		t.Fatalf("payload header offset %d outside file %d", headerOff, len(body))
	}
	body[headerOff] ^= 0xff
	if err := os.WriteFile(payloadPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeCache.readCommandMetaFromPayloadFresh(cliPath); err == nil {
		t.Fatal("corrupt payload fresh read succeeded")
	}

	runtimeCache.resetPayloadsHandle()
	runtimeCache.options.Identity.Payload.EncodedSHA256 = sha256.Sum256([]byte("wrong-payload-leaf"))
	if _, ok := runtimeCache.renderedCompactLeaf(canonical); ok {
		t.Fatal("poisoned identity compact leaf succeeded")
	}

	meta, err := runtimeCache.readMeta()
	if err != nil {
		t.Fatal(err)
	}
	runtimeCache.seedMeta(meta)
	if _, err := runtimeCache.loadMeta(); err != nil {
		t.Fatal(err)
	}
	ghostMeta := meta
	ghostMeta.LocatorProductByPath = map[string]string{}
	for path, id := range meta.LocatorProductByPath {
		ghostMeta.LocatorProductByPath[path] = id
	}
	ghostMeta.LocatorProductByPath["ghost-path-xyz"] = productID
	if _, err := runtimeCache.queryPayload(ghostMeta, "ghost-path-xyz", false); err == nil {
		t.Fatal("unknown located query succeeded")
	}
	testseam.Swap(t, &renderSchemaProductSummary, func(schemaruntime.ProductSpec) (map[string]any, error) {
		return nil, errors.New("forced product summary remaining")
	})
	testseam.Swap(t, &renderSchemaToolSummary, func(schemaruntime.ToolSpec) (map[string]any, error) {
		return nil, errors.New("forced tool summary remaining")
	})
	forced := false
	for _, product := range meta.Overview.Products {
		if _, err := runtimeCache.queryPayload(meta, product.ID, false); err != nil {
			forced = true
			break
		}
	}
	if !forced && len(strings.Fields(cliPath)) >= 2 {
		if _, err := runtimeCache.queryPayload(meta, strings.Join(strings.Fields(cliPath)[:2], " "), false); err != nil {
			forced = true
		}
	}
	if !forced {
		t.Fatal("forced summary projectors never triggered")
	}

	wrongSHA := meta
	wrongSHA.ProductDescriptors = append([]schemaruntime.ProductDescriptor(nil), meta.ProductDescriptors...)
	wrongSHA.ProductDescriptors[0].SHA256 = sha256.Sum256([]byte("x"))
	if _, err := runtimeCache.readAllPayload(wrongSHA, false); err == nil {
		t.Fatal("range digest mismatch readAll succeeded")
	}
	overviewDrift := meta
	overviewDrift.Overview.Products = append([]schemaruntime.OverviewProduct(nil), meta.Overview.Products...)
	if len(overviewDrift.Overview.Products) > 0 {
		overviewDrift.Overview.Products[0].ToolCount = 0
		if _, err := runtimeCache.readAllPayload(overviewDrift, false); err == nil {
			t.Fatal("decode product drift readAll succeeded")
		}
	}
	for _, descriptor := range meta.ProductDescriptors {
		runtimeCache.storeProduct(descriptor.ProductID, schemaruntime.DecodedSchemaProduct{
			Registry: schemaruntime.SchemaRegistry{Products: []ProductSpec{{
				ID: descriptor.ProductID,
				Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{
					ProductID: descriptor.ProductID, Name: "run", CanonicalPath: "nope.run",
					Path: "nope.run", CLIPath: descriptor.ProductID + " run", PrimaryCLIPath: descriptor.ProductID + " run",
				}}},
			}}},
		})
	}
	if _, err := runtimeCache.readAllPayload(meta, true); err == nil {
		t.Fatal("invalid reused products Index succeeded")
	}
	for _, descriptor := range meta.ProductDescriptors {
		product, err := runtimeCache.readProduct(meta, descriptor.ProductID)
		if err != nil {
			t.Fatal(err)
		}
		runtimeCache.storeProduct(descriptor.ProductID, product)
	}
	badMeta := meta
	badMeta.AgentMetadata = json.RawMessage("not-json")
	if _, err := runtimeCache.readAllPayload(badMeta, true); err == nil {
		t.Fatal("invalid agent metadata RenderAll succeeded")
	}

	opened, err := runtimeCache.opened()
	if err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(opened.Directory(), "registry.shards.cache")
	regBody, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(regBody) > schemacache.HeaderSize {
		regBody[schemacache.HeaderSize] ^= 0xff
		if err := os.WriteFile(regPath, regBody, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtimeCache.readAllPayload(meta, false); err == nil {
		t.Fatal("aggregate digest mismatch readAll succeeded")
	}

	_ = artifacts
}

func TestCrossPlatformCoverageSchemaCacheArtifactsRemainingFailures(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	loaded := deliverySchemaCatalog()
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(loaded)
	if err != nil {
		t.Fatal(err)
	}
	digest := artifacts.SourceHash
	surface := artifacts.SurfaceHash

	colliding := collidingLocatorRegistry()
	if _, err := colliding.Index(); err != nil {
		t.Fatalf("colliding locator registry must Index: %v", err)
	}
	if _, err := buildSchemaCacheArtifacts(colliding, digest, surface); err == nil {
		t.Fatal("locator collision artifacts accepted")
	}

	t.Run("compact-marshal-artifacts", func(t *testing.T) {
		testseam.Swap(t, &compactLeafMarshal, func(any, string, string) ([]byte, error) {
			return nil, errors.New("forced compact")
		})
		if _, err := buildSchemaCacheArtifacts(loaded.Registry, digest, surface); err == nil {
			t.Fatal("forced compact marshal artifacts accepted")
		}
	})

	bad := loaded.Registry
	if len(bad.Products) == 0 || len(bad.Products[0].Tools) == 0 {
		t.Fatal("empty delivery registry")
	}
	products := append([]ProductSpec(nil), bad.Products...)
	tools := append([]ToolSpec(nil), products[0].Tools...)
	tools[0].Selection.ExampleDispositions = []contract.ExampleDisposition{{
		Mode: "not-a-mode", ReasonCode: contract.ExampleDispositionReasonLocalState, Reason: "x", Reviewed: true,
	}}
	products[0].Tools = tools
	bad.Products = products
	if _, err := buildSchemaCacheArtifacts(bad, digest, surface); err == nil {
		t.Fatal("unsupported disposition artifacts accepted")
	}

	titleDrift := artifacts
	titleProducts := append([]ProductSpec(nil), artifacts.registry.Products...)
	titleTools := append([]ToolSpec(nil), titleProducts[0].Tools...)
	titleTools[0].Title = "drifted-title"
	titleProducts[0].Tools = titleTools
	titleDrift.registry.Products = titleProducts
	if err := titleDrift.ValidateRoundTrip(); err == nil {
		t.Fatal("identity title drift round trip succeeded")
	}
	overviewJSON := artifacts
	overviewJSON.registry.AgentMetadata = json.RawMessage("{")
	if err := overviewJSON.ValidateRoundTrip(); err == nil {
		t.Fatal("invalid overview JSON round trip succeeded")
	}
	kindDrift := artifacts
	kindDrift.registry.Kind = "other"
	if err := kindDrift.ValidateRoundTrip(); err == nil {
		t.Fatal("overview kind drift round trip succeeded")
	}
	garbageRegistry := artifacts
	garbageRegistry.Registry = []byte("nope")
	if err := garbageRegistry.ValidateRoundTrip(); err == nil {
		t.Fatal("garbage registry round trip succeeded")
	}
	descDrift := artifacts
	descProducts := append([]ProductSpec(nil), artifacts.registry.Products...)
	descTools := append([]ToolSpec(nil), descProducts[0].Tools...)
	descTools[0].Display = "drifted-display"
	descProducts[0].Tools = descTools
	descDrift.registry.Products = descProducts
	if err := descDrift.ValidateRoundTrip(); err == nil {
		t.Fatal("registry DeepEqual drift round trip succeeded")
	}
	queryDrift := artifacts
	queryDrift.index = SchemaIndex{}
	if err := queryDrift.ValidateRoundTrip(); err == nil {
		t.Fatal("query drift round trip succeeded")
	}
}

func TestCrossPlatformCoverageSchemaCatalogRepairRecheckFailures(t *testing.T) {
	runtimeCache, _, _ := publishCoverageSchemaRuntime(t)
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	runtimeCache.metaOnce.Do(func() { runtimeCache.metaErr = errors.New("poison meta") })
	runtimeCache.options.Identity.SourceSHA256 = sha256.Sum256([]byte("wrong-source-for-repair"))
	assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
		return loadedSchemaCatalog{}, errors.New("forced catalog err")
	}
	resetDeliverySchemaCatalogStateForTest()
	runtimeDeliveryLiveCatalog.Store(nil)
	if _, err := deliverySchemaOverviewPayload(); err == nil {
		t.Fatal("overview repair with failed live catalog succeeded")
	}
	if _, err := queryDeliverySchemaPayload([]string{"calendar event create"}); err == nil {
		t.Fatal("query repair with failed live catalog succeeded")
	}
}

func TestCrossPlatformCoverageSchemaReaderIdentityAndRangeFailures(t *testing.T) {
	runtimeCache, identity, _ := publishCoverageSchemaRuntime(t)
	cache, err := runtimeCache.opened()
	if err != nil {
		t.Fatal(err)
	}
	garbage := []byte("not-a-protobuf-meta")
	garbageArt := schemacache.Artifact{
		Payload: garbage,
		Expectation: schemacache.ArtifactExpectation{
			Kind: schemacache.KindMeta, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
			FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(garbage)), DecodedLength: uint64(len(garbage)),
			EncodedSHA256: sha256.Sum256(garbage),
		},
	}
	garbageIdentity := identity
	garbageIdentity.Meta = garbageArt.Expectation
	if err := cache.WriteArtifact(garbageIdentity.ExpectedIdentity(), garbageArt); err != nil {
		t.Fatal(err)
	}
	if _, err := schemareader.ReadMeta(cache, garbageIdentity); err == nil {
		t.Fatal("garbage Meta decode succeeded")
	}

	runtimeCache2, identity2, _ := publishCoverageSchemaRuntime(t)
	cache2, err := runtimeCache2.opened()
	if err != nil {
		t.Fatal(err)
	}
	mismatch := identity2
	mismatch.Registry.EncodedSHA256 = sha256.Sum256([]byte("wrong-registry-pin"))
	if _, err := schemareader.ReadMeta(cache2, mismatch); err == nil {
		t.Fatal("registry pin mismatch accepted")
	}
	meta, err := schemareader.ReadMeta(cache2, identity2)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ProductDescriptors) == 0 {
		t.Fatal("no products")
	}
	mut := meta
	mut.ProductDescriptors = append([]schemaruntime.ProductDescriptor(nil), meta.ProductDescriptors...)
	mut.ProductDescriptors[0].SHA256 = sha256.Sum256([]byte("wrong-range"))
	if _, err := schemareader.ReadProduct(cache2, identity2, mut, mut.ProductDescriptors[0].ProductID); err == nil {
		t.Fatal("ReadProduct range digest mismatch accepted")
	}
}

func collidingLocatorRegistry() SchemaRegistry {
	tool := func(product, name, cli string) ToolSpec {
		return ToolSpec{Identity: contract.ToolIdentitySpec{
			ProductID: product, Name: name, CLIName: name,
			CanonicalPath: product + "." + name, Path: product + "." + name,
			CLIPath: cli, PrimaryCLIPath: cli,
		}}
	}
	return SchemaRegistry{Kind: "schema", Level: "catalog", Source: SchemaSourceRuntimeAssembled, Products: []ProductSpec{
		{ID: "alpha", Tools: []ToolSpec{tool("alpha", "run", "beta run"), tool("alpha", "zzz", "alpha zzz")}},
		{ID: "beta", Tools: []ToolSpec{tool("beta", "other", "beta other"), tool("beta", "zzz", "beta zzz")}},
	}}
}

func mustOpenedDir(t *testing.T, runtimeCache *schemaCacheRuntime) string {
	t.Helper()
	cache, err := runtimeCache.opened()
	if err != nil {
		t.Fatal(err)
	}
	return cache.Directory()
}
