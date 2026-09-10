// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemareader

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func TestCrossPlatformCoverageParseIdentityIncompleteAndDecimal(t *testing.T) {
	digest := strings.Repeat("a", 64)
	upper := strings.Repeat("A", 64)
	raw := RawIdentity{Edition: "open", SourceSHA256: upper, SurfaceSHA256: digest, BuildID: digest,
		MetaLength: "1", MetaSHA256: digest, RegistryLength: "1", RegistrySHA256: digest,
		PayloadLength: "1", PayloadSHA256: digest, PayloadIndexLength: "1", PayloadIndexSHA256: digest}
	if identity, err := ParseOptionalIdentity(raw); err == nil || identity != nil {
		t.Fatalf("uppercase hex = %#v, %v", identity, err)
	}
	if _, ok := parseSchemaCacheLowerHex("abc"); ok {
		t.Fatal("short hex accepted")
	}
	if _, ok := parseSchemaCacheLowerHex(strings.Repeat("g", 64)); ok {
		t.Fatal("non-hex accepted")
	}
	if _, ok := parseSchemaCachePositiveDecimal("0"); ok {
		t.Fatal("zero accepted")
	}
	if _, ok := parseSchemaCachePositiveDecimal("1a"); ok {
		t.Fatal("non-decimal accepted")
	}
	if _, ok := parseSchemaCachePositiveDecimal("18446744073709551616"); ok {
		t.Fatal("overflow accepted")
	}
}

func TestCrossPlatformCoverageReaderErrorPathsWithoutCache(t *testing.T) {
	identity := Identity{Edition: "open", CatalogSnapshotVersion: CatalogSnapshotVersion}
	if _, err := ReadMeta((*schemacache.Cache)(nil), identity); err == nil {
		t.Fatal("nil cache ReadMeta succeeded")
	}
	if _, err := ReadProduct((*schemacache.Cache)(nil), identity, schemaruntime.DecodedSchemaMeta{}, "missing"); err == nil {
		t.Fatal("unknown product succeeded")
	}
	meta := schemaruntime.DecodedSchemaMeta{ProductDescriptors: []schemaruntime.ProductDescriptor{{ProductID: "sample"}}}
	if _, err := ReadProduct((*schemacache.Cache)(nil), identity, meta, "sample"); err == nil {
		t.Fatal("nil cache ReadProduct succeeded")
	}
	if _, err := ReadPayloadIndex((*schemacache.Cache)(nil), identity); err == nil {
		t.Fatal("nil cache ReadPayloadIndex succeeded")
	}
	if _, err := ReadPayloadIndexRange((*schemacache.Registry)(nil), identity); err == nil {
		t.Fatal("nil registry ReadPayloadIndexRange succeeded")
	}
	if _, err := ReadCommandPayload((*schemacache.Cache)(nil), identity, schemaruntime.DecodedSchemaPayloadIndex{}, "missing"); err == nil {
		t.Fatal("nil cache ReadCommandPayload succeeded")
	}
	if _, err := ReadCommandPayloadRange((*schemacache.Registry)(nil), identity, schemaruntime.DecodedSchemaPayloadIndex{}, "missing"); err == nil {
		t.Fatal("unknown payload product succeeded")
	}
	index := schemaruntime.DecodedSchemaPayloadIndex{PayloadDescriptors: []schemaruntime.CommandPayloadDescriptor{{ProductID: "sample", HeaderLength: 4}}}
	if _, err := ReadCommandPayloadRange((*schemacache.Registry)(nil), identity, index, "sample"); err == nil {
		t.Fatal("nil registry ReadCommandPayloadRange succeeded")
	}
	if _, err := ReadRenderedLeaf((*schemacache.Cache)(nil), identity, schemaruntime.DecodedSchemaPayloadIndex{}, "missing", schemaruntime.RenderedLeafRef{}); err == nil {
		t.Fatal("nil cache ReadRenderedLeaf succeeded")
	}
	if _, err := ReadRenderedLeafRange((*schemacache.Registry)(nil), identity, schemaruntime.DecodedSchemaPayloadIndex{}, "missing", schemaruntime.RenderedLeafRef{}); err == nil {
		t.Fatal("unknown rendered leaf product succeeded")
	}
}

func TestCrossPlatformCoverageParseIdentityEmptyEdition(t *testing.T) {
	digest := strings.Repeat("a", 64)
	raw := RawIdentity{Edition: "", SourceSHA256: digest, SurfaceSHA256: digest, BuildID: digest,
		MetaLength: "1", MetaSHA256: digest, RegistryLength: "1", RegistrySHA256: digest,
		PayloadLength: "1", PayloadSHA256: digest, PayloadIndexLength: "1", PayloadIndexSHA256: digest}
	if _, err := ParseIdentity(raw); err == nil {
		t.Fatal("empty edition accepted")
	}
}

func TestCrossPlatformCoverageLocatorAndIndexLocator(t *testing.T) {
	meta := schemaruntime.DecodedSchemaMeta{LocatorProductByPath: map[string]string{"sample.run": "sample", "sample run": "sample"}}
	if product, ok := Locator(meta, "sample.run"); !ok || product != "sample" {
		t.Fatalf("locator = %q %v", product, ok)
	}
	if _, ok := Locator(meta, "missing"); ok {
		t.Fatal("missing locator")
	}
	index := schemaruntime.DecodedSchemaPayloadIndex{LocatorProductByPath: map[string]string{"sample.run": "sample"}}
	if product, ok := IndexLocator(index, "sample.run"); !ok || product != "sample" {
		t.Fatalf("index locator = %q %v", product, ok)
	}
	if _, ok := IndexLocator(index, "missing"); ok {
		t.Fatal("missing index locator")
	}
}

func TestCrossPlatformCoverageReaderAuthenticatedDecodeAndRangeFaults(t *testing.T) {
	if !schemacache.PersistentBackendEnabled(runtime.GOOS, runtime.GOARCH) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	cache, identity := publishReaderFixture(t)
	t.Cleanup(func() { _ = cache.Close() })

	garbage := identity
	if _, err := ReadMeta(cache, garbage); err == nil {
		t.Fatal("garbage meta decoded")
	}

	validCache, validIdentity, meta, index := publishValidReaderFixture(t)
	t.Cleanup(func() { _ = validCache.Close() })
	got, err := ReadMeta(validCache, validIdentity)
	if err != nil {
		t.Fatal(err)
	}
	_ = got
	if _, err := ReadProduct(validCache, validIdentity, meta, "sample"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPayloadIndex(validCache, validIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCommandPayload(validCache, validIdentity, index, "sample"); err != nil {
		t.Fatal(err)
	}
	mismatch := validIdentity
	mismatch.Registry.EncodedSHA256 = sha256.Sum256([]byte("other-reg"))
	if _, err := ReadMeta(validCache, mismatch); err == nil {
		t.Fatal("registry hash mismatch accepted")
	}

	badMeta := meta
	badMeta.ProductDescriptors = append([]schemaruntime.ProductDescriptor(nil), meta.ProductDescriptors...)
	badMeta.ProductDescriptors[0].SHA256 = sha256.Sum256([]byte("nope"))
	if _, err := ReadProduct(validCache, validIdentity, badMeta, "sample"); err == nil {
		t.Fatal("product range mismatch accepted")
	}

	badIndex := validIdentity
	badIndex.PayloadIndexSHA256 = sha256.Sum256([]byte("nope"))
	if _, err := ReadPayloadIndex(validCache, badIndex); err == nil {
		t.Fatal("payload index mismatch accepted")
	}
	if _, err := ReadCommandPayload(validCache, validIdentity, schemaruntime.DecodedSchemaPayloadIndex{}, "missing"); err == nil {
		t.Fatal("missing command payload accepted")
	}
	if _, err := ReadRenderedLeaf(validCache, validIdentity, schemaruntime.DecodedSchemaPayloadIndex{}, "missing", schemaruntime.RenderedLeafRef{}); err == nil {
		t.Fatal("missing rendered leaf accepted")
	}
	if _, err := ReadRenderedLeafRange(mustOpenPayloads(t, validCache, validIdentity), validIdentity, index, "sample", schemaruntime.RenderedLeafRef{
		Offset: 0, Length: 4, SHA256: sha256.Sum256([]byte("leaf")),
	}); err == nil {
		t.Fatal("rendered leaf range mismatch accepted")
	}
}

func mustOpenPayloads(t *testing.T, cache *schemacache.Cache, identity Identity) *schemacache.Registry {
	t.Helper()
	payloads, err := cache.OpenPayloads(identity.ExpectedIdentity(), identity.Payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = payloads.Close() })
	return payloads
}

func readerArtifact(kind schemacache.ArtifactKind, payload []byte) schemacache.Artifact {
	return schemacache.Artifact{
		Expectation: schemacache.ArtifactExpectation{
			Kind: kind, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
			FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(payload)),
			DecodedLength: uint64(len(payload)), EncodedSHA256: sha256.Sum256(payload),
		},
		Payload: append([]byte(nil), payload...),
	}
}

func readerIdentity(edition string, source, surface, build [32]byte, meta, registry, payload schemacache.Artifact, indexLength uint64, indexSHA [32]byte) Identity {
	return Identity{
		Edition: edition, CatalogSnapshotVersion: CatalogSnapshotVersion,
		SourceSHA256: source, SurfaceSHA256: surface, BuildID: build,
		Meta: meta.Expectation, Registry: registry.Expectation, Payload: payload.Expectation,
		PayloadIndexLength: indexLength, PayloadIndexSHA256: indexSHA,
	}
}

func openReaderCache(t *testing.T, edition string) *schemacache.Cache {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	base, err := os.MkdirTemp(home, ".dws-schemareader-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_SCHEMA_CACHE_DIR", filepath.Clean(resolved))
	cache, err := schemacache.Open(edition)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func publishReaderFixture(t *testing.T) (*schemacache.Cache, Identity) {
	t.Helper()
	meta := readerArtifact(schemacache.KindMeta, []byte("not-a-schema-meta"))
	reg := readerArtifact(schemacache.KindRegistry, []byte("not-a-registry"))
	payload := readerArtifact(schemacache.KindPayloads, []byte("not-a-payload"))
	source, surface, build := sha256.Sum256([]byte("source")), sha256.Sum256([]byte("surface")), sha256.Sum256([]byte("build"))
	identity := readerIdentity("covread", source, surface, build, meta, reg, payload, 1, sha256.Sum256([]byte("idx")))
	cache := openReaderCache(t, identity.Edition)
	if err := cache.Publish(identity.ExpectedIdentity(), reg, meta, payload); err != nil {
		t.Fatal(err)
	}
	return cache, identity
}

func publishValidReaderFixture(t *testing.T) (*schemacache.Cache, Identity, schemaruntime.DecodedSchemaMeta, schemaruntime.DecodedSchemaPayloadIndex) {
	t.Helper()
	registry := schemaruntime.SchemaRegistry{
		Kind: "schema", Level: "catalog", Source: "test",
		Products: []schemaruntime.ProductSpec{{
			ID: "sample", Name: "Sample", Description: "Sample",
			Tools: []schemaruntime.ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					ProductID: "sample", Name: "run", CLIName: "run",
					CanonicalPath: "sample.run", Path: "sample.run",
					CLIPath: "sample run", PrimaryCLIPath: "sample run", Source: "runtime",
				},
				Title: "Run", Description: "Run it",
			}},
		}},
	}
	overview, err := schemaruntime.BuildSchemaOverview(registry)
	if err != nil {
		t.Fatal(err)
	}
	locators, err := schemaruntime.BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	lookup := schemaruntime.BuildCommandMetaLookup(registry)
	hashes := schemaruntime.CacheHashes{SourceSHA256: sha256.Sum256([]byte("source")), SurfaceSHA256: sha256.Sum256([]byte("surface"))}
	built, err := schemaruntime.BuildSchemaCache(registry, lookup, overview, locators, hashes, map[string][]byte{
		"sample.run": []byte("{\"ok\":true}\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	metaArt := readerArtifact(schemacache.KindMeta, built.Meta)
	regArt := readerArtifact(schemacache.KindRegistry, built.ProductShards)
	payloadArt := readerArtifact(schemacache.KindPayloads, built.PayloadShards)
	identity := readerIdentity("covvalid", hashes.SourceSHA256, hashes.SurfaceSHA256, sha256.Sum256([]byte("build")), metaArt, regArt, payloadArt, built.PayloadIndexLength, built.PayloadIndexSHA256)
	cache := openReaderCache(t, identity.Edition)
	if err := cache.Publish(identity.ExpectedIdentity(), regArt, metaArt, payloadArt); err != nil {
		t.Fatal(err)
	}
	decoded, err := schemaruntime.DecodeSchemaMetaCache(built.Meta)
	if err != nil {
		t.Fatal(err)
	}
	index, err := schemaruntime.DecodeSchemaPayloadIndex(built.PayloadShards[:built.PayloadIndexLength])
	if err != nil {
		t.Fatal(err)
	}
	return cache, identity, decoded, index
}
