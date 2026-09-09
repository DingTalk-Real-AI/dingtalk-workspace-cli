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

func TestPersistentSchemaCacheRealDeliveryParityAndLazyIO(t *testing.T) {
	testPersistentSchemaCacheRealDelivery(t, true)
}

// This runs the same real-data loader/repair/lock assertions under -race without
// repeating exhaustive, serial build-time projection checks for every locator.
// Native CI runs the exhaustive test separately, without race instrumentation.
func TestPersistentSchemaCacheRealConcurrentRepair(t *testing.T) {
	testPersistentSchemaCacheRealDelivery(t, false)
}

func testPersistentSchemaCacheRealDelivery(t *testing.T, exhaustive bool) {
	t.Helper()
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
	if exhaustive {
		if err := artifacts.ValidateRoundTrip(); err != nil {
			t.Fatal(err)
		}
	}
	identity := testSchemaCacheIdentity(t, artifacts)
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Publish(testExpectedIdentity(t, identity), artifacts.RegistryArtifact(), artifacts.MetaArtifact(), artifacts.PayloadArtifact()); err != nil {
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
	if got, _ := gotAll["tool_count"].(int); got != resolved.CommandCount() {
		t.Fatalf("--all tool_count = %d, want %d", got, resolved.CommandCount())
	}
	if factoryCalls.Load() != 0 {
		t.Fatalf("cache-hit delivery invoked Cobra factory %d times", factoryCalls.Load())
	}

	if exhaustive {
		for _, path := range artifacts.LocatorPaths() {
			want, renderErr := artifacts.RenderQuery(path)
			got, deliveryErr := cli.DeliverySchemaQueryPayloadForTest(path)
			if renderErr != nil || deliveryErr != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("locator parity for %q: render=%v delivery=%v equal=%v", path, renderErr, deliveryErr, reflect.DeepEqual(got, want))
			}
		}
	}

	// Corrupt Registry after a clean hit, then race callers through the one
	// repair coordinator. Exactly one authoritative assembly republishes the
	// exact generation. Mix both loader families with a complete Registry read
	// so an in-flight product load cannot be consumed by --all memoization.
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
	start := make(chan struct{})
	for i := range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var payload, want map[string]any
			var queryErr error
			var path string
			switch i % 4 {
			case 0:
				path, want = "calendar event create", wantLeaf
				payload, queryErr = cli.DeliverySchemaQueryPayloadForTest(path)
			case 1:
				path, want = "overview", wantOverview
				payload, queryErr = cli.DeliverySchemaOverviewPayloadForTest()
			case 2:
				path, want = "--all", wantAll
				payload, queryErr = cli.DeliverySchemaAllPayloadForTest()
			case 3:
				gotMeta, found := cli.ResolveMeta("calendar event create")
				if !found || !reflect.DeepEqual(gotMeta, meta) {
					errorsByCaller <- &parityError{path: "ResolveMeta(calendar event create)"}
				}
				return
			}
			if queryErr != nil {
				errorsByCaller <- queryErr
				return
			}
			if !reflect.DeepEqual(payload, want) {
				errorsByCaller <- &parityError{path: path}
			}
		}()
	}
	close(start)
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

// TestPersistentSchemaCacheRenderedLeafFastPath covers the compact leaf fast
// path: pre-rendered payload bytes served without registry I/O, byte-identical
// to the live render, with alias and non-compact queries falling through to the
// registry-backed path.
func TestPersistentSchemaCacheRenderedLeafFastPath(t *testing.T) {
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
	identity := testSchemaCacheIdentity(t, artifacts)
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Publish(testExpectedIdentity(t, identity), artifacts.RegistryArtifact(), artifacts.MetaArtifact(), artifacts.PayloadArtifact()); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	counters := &schemacache.Counters{}
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: counters,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}) })

	fastOut := executeSchemaLeafCommand(t, "calendar.list_calendars", "--compact")
	snap := counters.Snapshot()
	if snap.RegistryReadOps != 0 || snap.RegistryReadBytes != 0 {
		t.Fatalf("compact leaf fast path touched the registry: %#v", snap)
	}
	if snap.PayloadReadOps != 3 {
		t.Fatalf("compact leaf fast path payload reads = %d, want 3 (index + shard header + leaf blob)", snap.PayloadReadOps)
	}

	// The primary CLI-path spelling renders the same canonical bytes.
	primaryOut := executeSchemaLeafCommand(t, "calendar book list", "--compact")
	if !bytes.Equal(fastOut, primaryOut) {
		t.Fatal("primary CLI path spelling changed the compact leaf output")
	}
	if after := counters.Snapshot(); after.RegistryReadOps != 0 {
		t.Fatal("primary CLI path spelling touched the registry")
	}

	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}); err != nil {
		t.Fatal(err)
	}
	liveOut := executeSchemaLeafCommand(t, "calendar.list_calendars", "--compact")
	if !bytes.Equal(fastOut, liveOut) {
		t.Fatal("compact leaf fast path output differs from the live render")
	}
}

// TestPersistentSchemaCachePrewarm covers the speculative payload probe: it is
// read-only when the cache is missing, and when it succeeds the compact leaf
// fast path serves identical bytes through the same three authenticated range
// reads (index during the probe, then shard header and leaf blob).
func TestPersistentSchemaCachePrewarm(t *testing.T) {
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	configureSchemaCacheTestHome(t)
	// A prior test's live render populates the live catalog, which disables
	// the persistent fast path; reset so this test is order-independent.
	cli.RestorePackageCLISchemaDeliveryForTest()
	resolved, err := cli.ResolveSchemaBuild(app.NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
	if err != nil {
		t.Fatal(err)
	}
	identity := testSchemaCacheIdentity(t, artifacts)

	// A missing cache must leave the filesystem untouched.
	probeCounters := &schemacache.Counters{}
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: probeCounters,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	cli.PrewarmSchemaCache()
	cli.AwaitSchemaCachePrewarmForTest()
	if snap := probeCounters.Snapshot(); snap.MkdirOps != 0 || snap.WriteOps != 0 {
		t.Fatalf("speculative probe mutated the cache: %#v", snap)
	}
	cacheBase := filepath.Join(os.Getenv("HOME"), ".cache")
	if runtime.GOOS == "darwin" {
		cacheBase = filepath.Join(os.Getenv("HOME"), "Library", "Caches")
	}
	if _, err := os.Stat(filepath.Join(cacheBase, "dws")); !os.IsNotExist(err) {
		t.Fatalf("speculative probe created cache ancestry: %v", err)
	}

	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Publish(testExpectedIdentity(t, identity), artifacts.RegistryArtifact(), artifacts.MetaArtifact(), artifacts.PayloadArtifact()); err != nil {
		t.Fatal(err)
	}
	cacheDirectory := cache.Directory()
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	counters := &schemacache.Counters{}
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: counters,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}) })
	cli.PrewarmSchemaCache()

	fastOut := executeSchemaLeafCommand(t, "calendar.list_calendars", "--compact")
	snap := counters.Snapshot()
	if snap.RegistryReadOps != 0 || snap.RegistryReadBytes != 0 {
		t.Fatalf("prewarmed compact leaf fast path touched the registry: %#v", snap)
	}
	if snap.PayloadReadOps != 3 {
		t.Fatalf("prewarmed compact leaf payload reads = %d, want 3 (index + shard header + leaf blob)", snap.PayloadReadOps)
	}

	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{}); err != nil {
		t.Fatal(err)
	}
	liveOut := executeSchemaLeafCommand(t, "calendar.list_calendars", "--compact")
	if !bytes.Equal(fastOut, liveOut) {
		t.Fatal("prewarmed compact leaf fast path output differs from the live render")
	}

	// A repair entered before any payload read must release the prewarmed
	// handle that was never adopted by the hot path. Reset the live catalog
	// populated by the parity check above, or --all answers from memory and
	// never touches the corrupted Meta.
	cli.RestorePackageCLISchemaDeliveryForTest()
	if err := cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Counters: &schemacache.Counters{},
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	cli.PrewarmSchemaCache()
	cli.AwaitSchemaCachePrewarmForTest()
	metaPath := filepath.Join(cacheDirectory, "meta.cache")
	if err := os.WriteFile(metaPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	gotAll, err := cli.DeliverySchemaAllPayloadForTest()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := gotAll["tool_count"].(int); got != resolved.CommandCount() {
		t.Fatalf("repaired --all tool_count = %d, want %d", got, resolved.CommandCount())
	}
	info, err := os.Stat(metaPath)
	if err != nil || info.Size() != int64(identity.Meta.EncodedLength+schemacache.HeaderSize) {
		t.Fatalf("Meta was not republished to its pinned size: info=%v err=%v", info, err)
	}
	if meta, ok := cli.ResolveMeta("calendar book list"); !ok || meta.Identity.Canonical != "calendar.list_calendars" {
		t.Fatalf("ResolveMeta after repair with a fresh handle = %#v, %v", meta, ok)
	}
}

func executeSchemaLeafCommand(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := cli.NewSchemaCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("schema %v: %v", args, err)
	}
	return out.Bytes()
}

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
	indexLength, indexDigest, err := artifacts.PayloadIndexPins()
	if err != nil {
		t.Fatal(err)
	}
	return cli.SchemaCacheIdentity{
		Edition: "open", CatalogSnapshotVersion: uint32(artifacts.Version),
		SourceSHA256: decodeTestSchemaDigest(t, artifacts.SourceHash), SurfaceSHA256: decodeTestSchemaDigest(t, artifacts.SurfaceHash),
		BuildID: sha256.Sum256([]byte("persistent-cache-real-delivery-test")),
		Meta:    artifacts.MetaArtifact().Expectation, Registry: artifacts.RegistryArtifact().Expectation,
		Payload:            artifacts.PayloadArtifact().Expectation,
		PayloadIndexLength: indexLength, PayloadIndexSHA256: indexDigest,
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
	if err := cache.Publish(testExpectedIdentity(t, identity), artifacts.RegistryArtifact(), artifacts.MetaArtifact(), artifacts.PayloadArtifact()); err != nil {
		t.Fatal(err)
	}
}
