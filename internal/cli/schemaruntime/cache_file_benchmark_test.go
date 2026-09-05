// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime_test

import (
	"crypto/sha256"
	"os"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

// BenchmarkRealSchemaFileHit includes secure directory traversal, envelope
// authentication, payload reads/hashes, protobuf conversion and lookup/index.
// Files stay in the OS page cache; no assembled products are reused per sample.
func BenchmarkRealSchemaFileHit(b *testing.B) {
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		b.Skip("persistent cache backend is disabled on this target")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		b.Fatal(err)
	}
	testHome, err := os.MkdirTemp(home, ".dws-schema-file-bench-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(testHome) })
	b.Setenv("HOME", testHome)
	b.Setenv("XDG_CACHE_HOME", testHome+"/.cache")
	registry := assembleRealRegistry(b)
	built, meta := buildRealCache(b, registry)
	editionDigest, err := schemacache.EditionSHA256("benchmark")
	if err != nil {
		b.Fatal(err)
	}
	identity := schemacache.ExpectedIdentity{
		CatalogSnapshotVersion: 1, EditionSHA256: editionDigest,
		SourceSHA256: meta.Hashes.SourceSHA256, SurfaceSHA256: meta.Hashes.SurfaceSHA256,
		BuildID: sha256.Sum256([]byte("file-benchmark")),
	}
	artifact := func(kind schemacache.ArtifactKind, payload []byte) schemacache.Artifact {
		return schemacache.Artifact{Payload: payload, Expectation: schemacache.ArtifactExpectation{
			Kind: kind, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
			FormatVersion: schemacache.DTOFormatVersion,
			EncodedLength: uint64(len(payload)), DecodedLength: uint64(len(payload)), EncodedSHA256: sha256.Sum256(payload),
		}}
	}
	metaArtifact := artifact(schemacache.KindMeta, built.Meta)
	registryArtifact := artifact(schemacache.KindRegistry, built.ProductShards)
	cache, err := schemacache.Open("benchmark")
	if err != nil {
		b.Fatal(err)
	}
	if err := cache.Publish(identity, registryArtifact, metaArtifact); err != nil {
		cache.Close()
		b.Fatal(err)
	}
	cache.Close()
	b.Run("meta-open-authenticate-decode-lookup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cache, err := schemacache.Open("benchmark")
			if err != nil {
				b.Fatal(err)
			}
			payload, err := cache.ReadMeta(identity, metaArtifact.Expectation)
			cache.Close()
			if err != nil {
				b.Fatal(err)
			}
			decoded, err := schemaruntime.DecodeSchemaMetaCache(payload)
			if err != nil {
				b.Fatal(err)
			}
			if decoded.Hashes != meta.Hashes || decoded.RegistryDataSHA256 != registryArtifact.Expectation.EncodedSHA256 || decoded.RegistryDataLength != registryArtifact.Expectation.EncodedLength {
				b.Fatal("Meta identity mismatch")
			}
			if _, ok := decoded.CommandMetaByPath["calendar event create"]; !ok {
				b.Fatal("missing command metadata")
			}
		}
	})
	b.Run("selected-open-locator-authenticate-decode-index", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			// This stage begins with already authenticated Meta, matching
			// production. Add the Meta stage for a process's first query.
			productID, ok := meta.LocatorProductByPath["calendar event create"]
			if !ok {
				b.Fatal("missing product locator")
			}
			var descriptor schemaruntime.ProductDescriptor
			for _, candidate := range meta.ProductDescriptors {
				if candidate.ProductID == productID {
					descriptor = candidate
					break
				}
			}
			cache, err := schemacache.Open("benchmark")
			if err != nil {
				b.Fatal(err)
			}
			file, err := cache.OpenRegistry(identity, registryArtifact.Expectation)
			if err != nil {
				cache.Close()
				b.Fatal(err)
			}
			payload, err := file.ReadRange(schemacache.RangeDescriptor{Offset: descriptor.Offset, Length: descriptor.Length, SHA256: descriptor.SHA256})
			file.Close()
			cache.Close()
			if err != nil {
				b.Fatal(err)
			}
			product, err := schemaruntime.DecodeSchemaProductCache(payload, descriptor, meta)
			if err != nil {
				b.Fatal(err)
			}
			if _, ok := product.Index.ResolveQuery("calendar event create"); !ok {
				b.Fatal("missing indexed tool")
			}
		}
	})
}
