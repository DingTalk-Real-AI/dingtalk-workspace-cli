// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/spf13/cobra"
)

const persistentCacheRealToolCount = 1357

func TestPersistentSchemaCacheRealDeliveryParityAndLazyIO(t *testing.T) {
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	configureSchemaCacheTestHome(t)
	resolved, err := cli.ResolveSchemaBuild(app.NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifacts.ValidateRoundTrip(); err != nil {
		t.Fatal(err)
	}
	identity := testSchemaCacheIdentity(t, artifacts)
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Publish(testExpectedIdentity(t, identity), artifacts.RegistryArtifact(), artifacts.MetaArtifact()); err != nil {
		t.Fatal(err)
	}
	cacheDirectory := cache.Directory()
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}

	var factoryCalls atomic.Int64
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return app.NewSchemaSourceRootCommand()
	})
	counters := &schemacache.Counters{}
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: counters,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{})
		cli.RestorePackageCLISchemaDeliveryForTest()
	})

	meta, ok := cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("ResolveMeta cache hit = %#v, %v", meta, ok)
	}
	assertNoRegistryIO(t, counters.Snapshot(), "ResolveMeta")
	if factoryCalls.Load() != 0 {
		t.Fatalf("cache-hit ResolveMeta invoked Cobra factory %d times", factoryCalls.Load())
	}

	wantOverview, err := artifacts.RenderOverview()
	if err != nil {
		t.Fatal(err)
	}
	gotOverview, err := cli.DeliverySchemaOverviewPayloadForTest()
	if err != nil || !reflect.DeepEqual(gotOverview, wantOverview) {
		t.Fatalf("Meta overview parity: err=%v equal=%v", err, reflect.DeepEqual(gotOverview, wantOverview))
	}
	assertNoRegistryIO(t, counters.Snapshot(), "overview")

	wantLeaf, err := artifacts.RenderQuery("calendar event create")
	if err != nil {
		t.Fatal(err)
	}
	gotLeaf, err := cli.DeliverySchemaQueryPayloadForTest("calendar event create")
	if err != nil || !reflect.DeepEqual(gotLeaf, wantLeaf) {
		t.Fatalf("leaf parity: err=%v equal=%v", err, reflect.DeepEqual(gotLeaf, wantLeaf))
	}
	afterLeaf := counters.Snapshot()
	calendarBytes := descriptorLength(t, artifacts, "calendar")
	if afterLeaf.RegistryReadOps != 1 || afterLeaf.RegistryReadBytes != calendarBytes {
		t.Fatalf("leaf Registry I/O = ops:%d bytes:%d, want one range/%d", afterLeaf.RegistryReadOps, afterLeaf.RegistryReadBytes, calendarBytes)
	}
	if _, err := cli.DeliverySchemaQueryPayloadForTest("calendar event create"); err != nil {
		t.Fatal(err)
	}
	if repeated := counters.Snapshot(); repeated.RegistryReadOps != afterLeaf.RegistryReadOps || repeated.RegistryReadBytes != afterLeaf.RegistryReadBytes {
		t.Fatalf("repeated leaf performed Registry I/O: before=%#v after=%#v", afterLeaf, repeated)
	}

	for _, path := range []string{"calendar", "calendar event"} {
		want, renderErr := artifacts.RenderQuery(path)
		got, deliveryErr := cli.DeliverySchemaQueryPayloadForTest(path)
		if renderErr != nil || deliveryErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("query parity for %q: render=%v delivery=%v equal=%v", path, renderErr, deliveryErr, reflect.DeepEqual(got, want))
		}
	}

	wantAll, err := artifacts.RenderAll()
	if err != nil {
		t.Fatal(err)
	}
	gotAll, err := cli.DeliverySchemaAllPayloadForTest()
	if err != nil || !reflect.DeepEqual(gotAll, wantAll) {
		t.Fatalf("--all parity: err=%v equal=%v", err, reflect.DeepEqual(gotAll, wantAll))
	}
	if got, _ := gotAll["tool_count"].(int); got != persistentCacheRealToolCount {
		t.Fatalf("--all tool_count = %d, want %d", got, persistentCacheRealToolCount)
	}
	if factoryCalls.Load() != 0 {
		t.Fatalf("cache-hit delivery invoked Cobra factory %d times", factoryCalls.Load())
	}

	for _, path := range artifacts.LocatorPaths() {
		want, renderErr := artifacts.RenderQuery(path)
		got, deliveryErr := cli.DeliverySchemaQueryPayloadForTest(path)
		if renderErr != nil || deliveryErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("locator parity for %q: render=%v delivery=%v equal=%v", path, renderErr, deliveryErr, reflect.DeepEqual(got, want))
		}
	}

	// Corrupt Registry after a clean hit, then race callers through the one
	// repair coordinator. Exactly one authoritative assembly republishes the
	// exact generation; every caller receives the complete live leaf.
	if err := os.WriteFile(filepath.Join(cacheDirectory, "registry.shards.cache"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	factoryCalls.Store(0)
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return app.NewSchemaSourceRootCommand()
	})
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: counters,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsByCaller := make(chan error, 12)
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			payload, queryErr := cli.DeliverySchemaQueryPayloadForTest("calendar event create")
			if queryErr != nil {
				errorsByCaller <- queryErr
				return
			}
			if !reflect.DeepEqual(payload, wantLeaf) {
				errorsByCaller <- &parityError{path: "calendar event create"}
			}
		}()
	}
	wait.Wait()
	close(errorsByCaller)
	for callerErr := range errorsByCaller {
		t.Fatal(callerErr)
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("concurrent corrupt-cache repair assembled %d times, want 1", factoryCalls.Load())
	}
	liveArtifacts, liveArtifactsErr := cli.DeliverySchemaCacheArtifactsForTest()
	if liveArtifactsErr != nil {
		t.Fatal(liveArtifactsErr)
	}
	if liveArtifacts.SourceHash != artifacts.SourceHash || liveArtifacts.SurfaceHash != artifacts.SurfaceHash || liveArtifacts.MetaSHA256 != artifacts.MetaSHA256 || liveArtifacts.RegistrySHA256 != artifacts.RegistrySHA256 {
		t.Fatalf("repair artifacts differ: source=%s/%s surface=%s/%s meta=%x/%x registry=%x/%x", liveArtifacts.SourceHash, artifacts.SourceHash, liveArtifacts.SurfaceHash, artifacts.SurfaceHash, liveArtifacts.MetaSHA256, artifacts.MetaSHA256, liveArtifacts.RegistrySHA256, artifacts.RegistrySHA256)
	}
	verifyCacheGeneration(t, identity, counters.Snapshot())

	command := cli.NewSchemaCommand()
	command.SilenceUsage = true
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"definitely.unknown.schema.path"})
	if err := command.Execute(); err == nil {
		t.Fatal("unknown path unexpectedly succeeded")
	}
	if output.Len() != 0 {
		t.Fatalf("unknown path emitted partial output: %q", output.String())
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("unknown-path authoritative fallback factory calls = %d, want 1", factoryCalls.Load())
	}

	// A lock timeout must return authoritative data and leave the missing Meta
	// unpublished. This also proves lock acquisition is bounded.
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return app.NewSchemaSourceRootCommand()
	})
	factoryCalls.Store(0)
	lockCache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockCache.AcquireLock(nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cacheDirectory, "meta.cache")); err != nil {
		t.Fatal(err)
	}
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LockTimeout: time.Millisecond,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	if got, overviewErr := cli.DeliverySchemaOverviewPayloadForTest(); overviewErr != nil || !reflect.DeepEqual(got, wantOverview) {
		t.Fatalf("lock-timeout fallback: err=%v equal=%v", overviewErr, reflect.DeepEqual(got, wantOverview))
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("lock-timeout fallback assembled %d times, want 1", factoryCalls.Load())
	}
	if _, err := os.Stat(filepath.Join(cacheDirectory, "meta.cache")); !os.IsNotExist(err) {
		t.Fatalf("lock-timeout fallback published Meta: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := lockCache.Close(); err != nil {
		t.Fatal(err)
	}

	// A publication failure after successful live assembly cannot replace the
	// in-memory result or leak a partial payload.
	publishCacheGeneration(t, identity, artifacts)
	factoryCalls.Store(0)
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return app.NewSchemaSourceRootCommand()
	})
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cli.ResolveMeta("calendar event create"); !ok {
		t.Fatal("publication-failure setup did not open valid Meta")
	}
	registryPath := filepath.Join(cacheDirectory, "registry.shards.cache")
	if err := os.WriteFile(registryPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cacheDirectory, 0o500); err != nil {
		t.Fatal(err)
	}
	gotAfterWriteFailure, writeFailureErr := cli.DeliverySchemaQueryPayloadForTest("calendar event create")
	if err := os.Chmod(cacheDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if writeFailureErr != nil || !reflect.DeepEqual(gotAfterWriteFailure, wantLeaf) {
		t.Fatalf("failed-publication fallback: err=%v equal=%v", writeFailureErr, reflect.DeepEqual(gotAfterWriteFailure, wantLeaf))
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("failed-publication fallback assembled %d times, want 1", factoryCalls.Load())
	}
	if info, err := os.Stat(registryPath); err != nil || info.Size() != int64(len("corrupt")) {
		t.Fatalf("failed publication unexpectedly repaired Registry: info=%v err=%v", info, err)
	}

	// Runtime ineligibility is decided before Open, producing zero persistent
	// I/O while preserving the same authoritative overview.
	factoryCalls.Store(0)
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return app.NewSchemaSourceRootCommand()
	})
	disabledCounters := &schemacache.Counters{}
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: disabledCounters,
		RuntimeEligible: func() bool { return false },
	}); err != nil {
		t.Fatal(err)
	}
	if got, overviewErr := cli.DeliverySchemaOverviewPayloadForTest(); overviewErr != nil || !reflect.DeepEqual(got, wantOverview) {
		t.Fatalf("disabled fallback: err=%v equal=%v", overviewErr, reflect.DeepEqual(got, wantOverview))
	}
	if disabledCounters.Snapshot() != (schemacache.IOSnapshot{}) {
		t.Fatalf("disabled cache performed persistent I/O: %#v", disabledCounters.Snapshot())
	}
}

type parityError struct{ path string }

func (e *parityError) Error() string { return "Schema cache parity mismatch for " + e.path }

func configureSchemaCacheTestHome(t *testing.T) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	testHome, err := os.MkdirTemp(home, ".dws-schema-cache-test-")
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

func testSchemaCacheIdentity(t *testing.T, artifacts cli.SchemaCacheArtifacts) cli.SchemaCacheIdentity {
	t.Helper()
	return cli.SchemaCacheIdentity{
		Edition: "open", CatalogSnapshotVersion: uint32(artifacts.Version),
		SourceSHA256: decodeTestSchemaDigest(t, artifacts.SourceHash), SurfaceSHA256: decodeTestSchemaDigest(t, artifacts.SurfaceHash),
		BuildID: sha256.Sum256([]byte("persistent-cache-real-delivery-test")),
		Meta:    artifacts.MetaArtifact().Expectation, Registry: artifacts.RegistryArtifact().Expectation,
	}
}

func testExpectedIdentity(t *testing.T, identity cli.SchemaCacheIdentity) schemacache.ExpectedIdentity {
	t.Helper()
	editionDigest, err := schemacache.EditionSHA256(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	return schemacache.ExpectedIdentity{
		CatalogSnapshotVersion: identity.CatalogSnapshotVersion, EditionSHA256: editionDigest,
		SourceSHA256: identity.SourceSHA256, SurfaceSHA256: identity.SurfaceSHA256, BuildID: identity.BuildID,
	}
}

func decodeTestSchemaDigest(t *testing.T, value string) [sha256.Size]byte {
	t.Helper()
	var digest [sha256.Size]byte
	decoded, err := hex.DecodeString(value[len("sha256:"):])
	if err != nil {
		t.Fatal(err)
	}
	copy(digest[:], decoded)
	return digest
}

func descriptorLength(t *testing.T, artifacts cli.SchemaCacheArtifacts, product string) uint64 {
	t.Helper()
	for _, descriptor := range artifacts.ProductDescriptors {
		if descriptor.ProductID == product {
			return descriptor.Length
		}
	}
	t.Fatalf("missing descriptor for %s", product)
	return 0
}

func assertNoRegistryIO(t *testing.T, snapshot schemacache.IOSnapshot, stage string) {
	t.Helper()
	if snapshot.RegistryReadOps != 0 || snapshot.RegistryReadBytes != 0 {
		encoded, _ := json.Marshal(snapshot)
		t.Fatalf("%s performed Registry I/O: %s", stage, encoded)
	}
}

func verifyCacheGeneration(t *testing.T, identity cli.SchemaCacheIdentity, operations schemacache.IOSnapshot) {
	t.Helper()
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if _, err := cache.ReadMeta(testExpectedIdentity(t, identity), identity.Meta); err != nil {
		t.Fatalf("read repaired Meta: %v", err)
	}
	registry, err := cache.OpenRegistry(testExpectedIdentity(t, identity), identity.Registry)
	if err != nil {
		t.Fatalf("open repaired Registry: %v (repair operations: %#v)", err, operations)
	}
	defer registry.Close()
	if err := registry.ValidateAggregate(); err != nil {
		t.Fatalf("validate repaired Registry: %v", err)
	}
}

func publishCacheGeneration(t *testing.T, identity cli.SchemaCacheIdentity, artifacts cli.SchemaCacheArtifacts) {
	t.Helper()
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if err := cache.Publish(testExpectedIdentity(t, identity), artifacts.RegistryArtifact(), artifacts.MetaArtifact()); err != nil {
		t.Fatal(err)
	}
}
