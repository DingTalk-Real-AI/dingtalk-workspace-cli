// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemareader

import (
	"crypto/sha256"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func TestCrossPlatformCoverageReaderAuthenticatedRoundTrip(t *testing.T) {
	schemacache.UseMemoryOpenForTest(t)
	registry := schemaruntime.SchemaRegistry{
		Kind: "schema", Level: "catalog", Source: "runtime-assembled",
		Products: []schemaruntime.ProductSpec{{
			ID: "sample", Name: "Sample", Description: "Sample product", Runtime: true,
			Tools: []schemaruntime.ToolSpec{{
				Identity: contract.ToolIdentitySpec{
					ProductID: "sample", Name: "run", CLIName: "run", CanonicalPath: "sample.run",
					CLIPath: "sample run", PrimaryCLIPath: "sample run", Source: "runtime",
				},
				Title: "Run", Description: "Runs",
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
	rendered := map[string][]byte{"sample.run": []byte("{\"canonical\":\"sample.run\"}\n")}
	hashes := schemaruntime.CacheHashes{SourceSHA256: sha256.Sum256([]byte("src")), SurfaceSHA256: sha256.Sum256([]byte("surf"))}
	built, err := schemaruntime.BuildSchemaCache(registry, lookup, overview, locators, hashes, rendered, nil)
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{
		Edition: "open", CatalogSnapshotVersion: CatalogSnapshotVersion,
		SourceSHA256: hashes.SourceSHA256, SurfaceSHA256: hashes.SurfaceSHA256, BuildID: sha256.Sum256([]byte("build")),
		Meta:               artifact(schemacache.KindMeta, built.Meta),
		Registry:           artifact(schemacache.KindRegistry, built.ProductShards),
		Payload:            artifact(schemacache.KindPayloads, built.PayloadShards),
		PayloadIndexLength: built.PayloadIndexLength,
		PayloadIndexSHA256: built.PayloadIndexSHA256,
	}
	if err := identity.Validate(); err != nil {
		t.Fatal(err)
	}
	cache, err := schemacache.Open("open")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	if err := cache.Publish(identity.ExpectedIdentity(),
		schemacache.Artifact{Expectation: identity.Registry, Payload: built.ProductShards},
		schemacache.Artifact{Expectation: identity.Meta, Payload: built.Meta},
		schemacache.Artifact{Expectation: identity.Payload, Payload: built.PayloadShards},
	); err != nil {
		t.Fatal(err)
	}

	meta, err := ReadMeta(cache, identity)
	if err != nil {
		t.Fatal(err)
	}
	wrong := identity
	wrong.Registry.EncodedLength++
	if _, err := ReadMeta(cache, wrong); err == nil {
		t.Fatal("registry pin mismatch accepted")
	}
	badRange := meta
	badRange.ProductDescriptors = append([]schemaruntime.ProductDescriptor(nil), meta.ProductDescriptors...)
	if len(badRange.ProductDescriptors) > 0 {
		badRange.ProductDescriptors[0].SHA256 = sha256.Sum256([]byte("wrong-range"))
	}
	if _, err := ReadProduct(cache, identity, badRange, "sample"); err == nil {
		t.Fatal("range digest mismatch accepted")
	}
	product, err := ReadProduct(cache, identity, meta, "sample")
	if err != nil || len(product.Registry.Products) == 0 || product.Registry.Products[0].ID != "sample" {
		t.Fatalf("product = %#v %v", product, err)
	}
	index, err := ReadPayloadIndex(cache, identity)
	if err != nil {
		t.Fatal(err)
	}
	payloads, err := ReadCommandPayload(cache, identity, index, "sample")
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := payloads.RenderedLeaf("sample.run")
	if !ok {
		t.Fatal("missing rendered leaf")
	}
	leaf, err := ReadRenderedLeaf(cache, identity, index, "sample", ref)
	if err != nil || string(leaf) != string(rendered["sample.run"]) {
		t.Fatalf("leaf = %q %v", leaf, err)
	}

	// The full-catalog fast path round-trips the pre-rendered wire bytes and
	// authenticates them through the pinned payload index.
	catalogWire := []byte("{\n  \"kind\": \"catalog\"\n}\n")
	builtWithCatalog, err := schemaruntime.BuildSchemaCache(registry, lookup, overview, locators, hashes, rendered, catalogWire)
	if err != nil {
		t.Fatal(err)
	}
	identityWithCatalog := identity
	identityWithCatalog.Meta = artifact(schemacache.KindMeta, builtWithCatalog.Meta)
	identityWithCatalog.Payload = artifact(schemacache.KindPayloads, builtWithCatalog.PayloadShards)
	identityWithCatalog.PayloadIndexLength = builtWithCatalog.PayloadIndexLength
	identityWithCatalog.PayloadIndexSHA256 = builtWithCatalog.PayloadIndexSHA256
	if err := cache.Publish(identityWithCatalog.ExpectedIdentity(),
		schemacache.Artifact{Expectation: identityWithCatalog.Registry, Payload: builtWithCatalog.ProductShards},
		schemacache.Artifact{Expectation: identityWithCatalog.Meta, Payload: builtWithCatalog.Meta},
		schemacache.Artifact{Expectation: identityWithCatalog.Payload, Payload: builtWithCatalog.PayloadShards},
	); err != nil {
		t.Fatal(err)
	}
	indexWithCatalog, err := ReadPayloadIndex(cache, identityWithCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if indexWithCatalog.RenderedCatalog == nil {
		t.Fatal("rendered catalog ref missing after publish")
	}
	payloadsFile, err := cache.OpenPayloads(identityWithCatalog.ExpectedIdentity(), identityWithCatalog.Payload)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadRenderedCatalogRange(payloadsFile, identityWithCatalog, *indexWithCatalog.RenderedCatalog)
	_ = payloadsFile.Close()
	if err != nil || string(catalog) != string(catalogWire) {
		t.Fatalf("rendered catalog = %q %v", catalog, err)
	}
	tamperedRef := *indexWithCatalog.RenderedCatalog
	tamperedRef.SHA256 = sha256.Sum256([]byte("tampered"))
	payloadsFile2, err := cache.OpenPayloads(identityWithCatalog.ExpectedIdentity(), identityWithCatalog.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRenderedCatalogRange(payloadsFile2, identityWithCatalog, tamperedRef); err == nil {
		_ = payloadsFile2.Close()
		t.Fatal("tampered catalog digest accepted")
	}
	_ = payloadsFile2.Close()
	// A generation published without the blob decodes with a nil ref so the
	// caller falls back to the registry path.
	if index.RenderedCatalog != nil {
		t.Fatal("legacy generation unexpectedly carries a rendered catalog ref")
	}

	garbage := identity
	garbage.Meta = artifact(schemacache.KindMeta, []byte("nope"))
	if err := cache.WriteArtifact(garbage.ExpectedIdentity(), schemacache.Artifact{Expectation: garbage.Meta, Payload: []byte("nope")}); err == nil {
		if _, err := ReadMeta(cache, garbage); err == nil {
			t.Fatal("garbage meta decoded")
		}
	}
}

func artifact(kind schemacache.ArtifactKind, payload []byte) schemacache.ArtifactExpectation {
	return schemacache.ArtifactExpectation{
		Kind: kind, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(payload)),
		DecodedLength: uint64(len(payload)), EncodedSHA256: sha256.Sum256(payload),
	}
}
