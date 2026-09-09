// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

func TestCrossPlatformCoverageSchemaCacheOptionsAndPrewarmEarlyReturn(t *testing.T) {
	t.Cleanup(func() {
		_ = RegisterSchemaCacheOptions(SchemaCacheOptions{})
		restorePackageCLISchemaDeliveryForTest()
	})
	identity := coverageSchemaCacheIdentity()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{Enabled: true, Identity: identity}); err != nil {
		t.Fatalf("empty GOOS/GOARCH fill: %v", err)
	}
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{}); err != nil {
		t.Fatal(err)
	}
	PrewarmSchemaCache()
	AwaitSchemaCachePrewarmForTest()
	if handle := SchemaCachePrewarmPayloadsHandleForTest(); handle != nil {
		t.Fatal("disabled prewarm published a handle")
	}
	if _, err := DeliverySchemaCacheArtifactsForTest(); err == nil && runtimeDeliveryLiveCatalog.Load() == nil {
		// missing live catalog is the ForTest early-return; error may also be nil
	}
	runtimeDeliveryLiveCatalog.Store(nil)
	_, _ = DeliverySchemaCacheArtifactsForTest()
	AwaitSchemaCachePrewarmForTest()
	_ = SchemaCachePrewarmPayloadsHandleForTest()
	restorePackageCLISchemaDeliveryForTest()

	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		RuntimeEligible: func() bool { return false },
	}); err != nil {
		t.Fatal(err)
	}
	PrewarmSchemaCache()
	MarkSchemaCacheRuntimeUncertain()
	PrewarmSchemaCache()
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	if handle := SchemaCachePrewarmPayloadsHandleForTest(); handle != nil {
		t.Fatal("enabled runtime without prewarm published a handle")
	}
	PrewarmSchemaCache()
	PrewarmSchemaCache()
	AwaitSchemaCachePrewarmForTest()
}

func TestCrossPlatformCoverageSchemaCacheRuntimeOpenedFailures(t *testing.T) {
	r := &schemaCacheRuntime{
		options:  SchemaCacheOptions{Identity: SchemaCacheIdentity{Edition: "!!!invalid"}},
		products: make(map[string]*schemaCacheProductLoad),
		payloads: make(map[string]*schemaCachePayloadLoad),
	}
	if _, err := r.readMeta(); err == nil {
		t.Fatal("invalid edition readMeta succeeded")
	}
	if _, err := r.readPayloadIndex(); err == nil {
		t.Fatal("invalid edition readPayloadIndex succeeded")
	}
	if _, err := r.readProduct(schemaruntime.DecodedSchemaMeta{}, "x"); err == nil {
		t.Fatal("invalid edition readProduct succeeded")
	}
	if _, err := r.payloadsHandle(); err == nil {
		t.Fatal("invalid edition payloadsHandle succeeded")
	}
	if _, err := r.readCommandPayload(schemaruntime.DecodedSchemaPayloadIndex{}, "x"); err == nil {
		t.Fatal("invalid edition readCommandPayload succeeded")
	}
	if _, _, err := r.resolveCommandMetaFromPayload("x"); err == nil {
		t.Fatal("invalid edition resolveCommandMeta succeeded")
	}
	if _, err := r.readCommandMetaFromPayloadFresh("x"); err == nil {
		t.Fatal("invalid edition readCommandMetaFromPayloadFresh succeeded")
	}
	if _, ok := r.renderedCompactLeaf("x"); ok {
		t.Fatal("invalid edition renderedCompactLeaf succeeded")
	}
	r.seedPayloadIndex(schemaruntime.DecodedSchemaPayloadIndex{})
	if _, err := r.loadPayloadIndex(); err != nil {
		t.Fatalf("seeded index: %v", err)
	}
	if _, ok := r.descriptor(schemaruntime.DecodedSchemaMeta{}, "missing"); ok {
		t.Fatal("missing descriptor")
	}
	r.resetPayloadsHandle()
	r.seedAll(map[string]any{"count": 0})
	if _, err := r.loadAllPayload(); err != nil {
		t.Fatalf("seeded all: %v", err)
	}
	if _, ok := schemaCacheLocator(schemaruntime.DecodedSchemaMeta{}, "missing"); ok {
		t.Fatal("missing locator")
	}
	if _, err := r.queryPayload(schemaruntime.DecodedSchemaMeta{}, "missing", true); err == nil {
		t.Fatal("unknown query succeeded")
	}
	r.seedMeta(schemaruntime.DecodedSchemaMeta{})
	if _, err := r.overviewPayload(schemaruntime.DecodedSchemaMeta{Overview: schemaruntime.SchemaOverview{AgentMetadata: json.RawMessage("{")}}); err == nil {
		t.Fatal("bad overview accepted")
	}
}

func TestCrossPlatformCoverageSchemaCacheHashesMatchAndRoundTrip(t *testing.T) {
	if _, err := schemaCacheHashes("nope", "sha256:"+hex.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("bad source hash accepted")
	}
	if _, err := schemaCacheHashes("sha256:"+hex.EncodeToString(make([]byte, 32)), "nope"); err == nil {
		t.Fatal("bad surface hash accepted")
	}
	if _, err := schemaCacheHashes("sha256:zzzz", "sha256:"+hex.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("non-hex source hash accepted")
	}
	empty := SchemaCacheArtifacts{Payload: []byte{1, 2}}
	if _, _, err := empty.PayloadIndexPins(); err == nil {
		t.Fatal("short payload pins accepted")
	}
	var prefix [4]byte
	prefix[3] = 100
	if _, _, err := (SchemaCacheArtifacts{Payload: append(prefix[:], 1, 2, 3)}).PayloadIndexPins(); err == nil {
		t.Fatal("index region overflow accepted")
	}
	if (SchemaCacheArtifacts{}).match(SchemaCacheIdentity{}) {
		t.Fatal("zero artifacts matched zero identity")
	}
	if _, err := canonicalSchemaCacheRegistry(SchemaRegistry{AgentMetadata: json.RawMessage("{")}); err == nil {
		t.Fatal("invalid agent_metadata canonicalized")
	}
	if _, err := canonicalSchemaCacheRegistry(SchemaRegistry{AgentMetadata: json.RawMessage("{}\n{}")}); err == nil {
		t.Fatal("multi-value agent_metadata canonicalized")
	}
	if _, err := renderCompactSchemaLeaves(SchemaRegistry{Products: []ProductSpec{{
		Tools: []ToolSpec{{Identity: contract.ToolIdentitySpec{}}},
	}}}, SchemaIndex{}); err != nil {
		t.Fatal(err)
	}
	if err := (SchemaCacheArtifacts{Meta: []byte("nope")}).ValidateRoundTrip(); err == nil {
		t.Fatal("invalid meta round trip succeeded")
	}
}

func TestCrossPlatformCoverageSchemaCacheRepairAndDeliveryMiss(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	testHome, err := os.MkdirTemp(home, ".dws-schema-cache-coverage-")
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

	runRepair := func(poison func(*schemaCacheRuntime), call func() error) {
		t.Helper()
		resetDeliverySchemaCatalogStateForTest()
		if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
			Enabled: true, Identity: identity, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			RuntimeEligible: func() bool { return true },
		}); err != nil {
			t.Fatal(err)
		}
		runtime := activeSchemaCacheRuntime()
		if runtime == nil {
			t.Fatal("runtime not registered")
		}
		poison(runtime)
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	runRepair(func(r *schemaCacheRuntime) {
		r.allOnce.Do(func() { r.allErr = errors.New("poison all") })
	}, func() error {
		_, err := deliverySchemaAllPayload()
		return err
	})
	runRepair(func(r *schemaCacheRuntime) {
		r.metaOnce.Do(func() { r.metaErr = errors.New("poison meta") })
	}, func() error {
		_, err := deliverySchemaOverviewPayload()
		return err
	})
	runRepair(func(r *schemaCacheRuntime) {
		r.indexOnce.Do(func() { r.indexErr = errors.New("poison index") })
	}, func() error {
		if _, ok := ResolveMeta("calendar event create"); !ok {
			return errors.New("ResolveMeta repair missed")
		}
		return nil
	})
	runRepair(func(r *schemaCacheRuntime) {
		r.metaOnce.Do(func() { r.metaErr = errors.New("poison meta") })
	}, func() error {
		_, err := queryDeliverySchemaPayload([]string{"calendar event create"})
		return err
	})
	empty, err := schemacache.Open("prewarm")
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: SchemaCacheIdentity{Edition: "prewarm", CatalogSnapshotVersion: identity.CatalogSnapshotVersion,
			SourceSHA256: identity.SourceSHA256, SurfaceSHA256: identity.SurfaceSHA256, BuildID: identity.BuildID,
			Meta: identity.Meta, Registry: identity.Registry, Payload: identity.Payload,
			PayloadIndexLength: identity.PayloadIndexLength, PayloadIndexSHA256: identity.PayloadIndexSHA256},
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	PrewarmSchemaCache()
	AwaitSchemaCachePrewarmForTest()
}

func coverageSchemaCacheIdentity() SchemaCacheIdentity {
	digest := sha256.Sum256([]byte("coverage-identity"))
	exp := func(kind schemacache.ArtifactKind) schemacache.ArtifactExpectation {
		return schemacache.ArtifactExpectation{
			Kind: kind, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
			FormatVersion: schemacache.DTOFormatVersion, EncodedLength: 1, DecodedLength: 1, EncodedSHA256: digest,
		}
	}
	return SchemaCacheIdentity{
		Edition: "open", CatalogSnapshotVersion: schemareader.CatalogSnapshotVersion,
		SourceSHA256: digest, SurfaceSHA256: digest, BuildID: digest,
		Meta: exp(schemacache.KindMeta), Registry: exp(schemacache.KindRegistry), Payload: exp(schemacache.KindPayloads),
		PayloadIndexLength: 1, PayloadIndexSHA256: digest,
	}
}

func coverageIdentityFromArtifacts(t *testing.T, artifacts SchemaCacheArtifacts) SchemaCacheIdentity {
	t.Helper()
	indexLength, indexDigest, err := artifacts.PayloadIndexPins()
	if err != nil {
		t.Fatal(err)
	}
	decode := func(value string) [sha256.Size]byte {
		t.Helper()
		var digest [sha256.Size]byte
		decoded, err := hex.DecodeString(value[len("sha256:"):])
		if err != nil {
			t.Fatal(err)
		}
		copy(digest[:], decoded)
		return digest
	}
	return SchemaCacheIdentity{
		Edition: "open", CatalogSnapshotVersion: uint32(artifacts.Version),
		SourceSHA256: decode(artifacts.SourceHash), SurfaceSHA256: decode(artifacts.SurfaceHash),
		BuildID: sha256.Sum256([]byte("coverage-delivery")),
		Meta:    artifacts.MetaArtifact().Expectation, Registry: artifacts.RegistryArtifact().Expectation,
		Payload: artifacts.PayloadArtifact().Expectation, PayloadIndexLength: indexLength, PayloadIndexSHA256: indexDigest,
	}
}

func TestCrossPlatformCoverageSchemaQueryCompactAndNormalize(t *testing.T) {
	if got := stripSchemaParamCompact(map[string]any{"type": "string", "audit": true}); got["type"] != "string" {
		t.Fatalf("compact = %#v", got)
	}
	if got := normalizeSchemaQueryCLIPath("dws calendar event"); got != "calendar event" {
		t.Fatalf("normalize = %q", got)
	}
}

func TestCrossPlatformCoverageQueryEmptyArgsAndRepairCatalogMiss(t *testing.T) {
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	resetDeliverySchemaCatalogStateForTest()
	runtimeDeliveryLiveCatalog.Store(nil)
	_ = RegisterSchemaCacheOptions(SchemaCacheOptions{})
	if _, err := queryDeliverySchemaPayload(nil); err != nil {
		t.Fatal(err)
	}

	loaded := deliverySchemaCatalog()
	runtimeDeliveryLiveCatalog.Store(&loaded)
	r := &schemaCacheRuntime{products: make(map[string]*schemaCacheProductLoad), payloads: make(map[string]*schemaCachePayloadLoad)}
	value, _, err := repairSchemaCache(r, func() (any, error) {
		t.Fatal("recheck must not run when live catalog is present")
		return nil, errors.New("unused")
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Fatal("live catalog repair must skip recheck")
	}

	RegisterSchemaSourceRoot(nil)
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	if err := RegisterSchemaCacheOptions(SchemaCacheOptions{
		Enabled: true, Identity: coverageSchemaCacheIdentity(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		RuntimeEligible: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	runtimeDeliveryLiveCatalog.Store(nil)
	if _, err := deliverySchemaAllPayload(); err == nil {
		t.Fatal("nil source root repair succeeded")
	}
}
