// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime_test

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
)

// Count the authoritative assembly, so newly declared commands are covered
// without retaining a stale benchmark-era fixture size.
func realSchemaToolCount(registry schemaruntime.SchemaRegistry) int {
	count := 0
	for _, product := range registry.Products {
		count += len(product.Tools)
	}
	return count
}

func TestRealAssembledSchemaCacheRoundTripAllTools(t *testing.T) {
	registry := assembleRealRegistry(t)
	wantTools := realSchemaToolCount(registry)
	built, meta := buildRealCache(t, registry)
	second, _ := buildRealCache(t, registry)
	if !bytes.Equal(built.Meta, second.Meta) || !bytes.Equal(built.ProductShards, second.ProductShards) {
		t.Fatal("real Schema cache build is not byte deterministic")
	}
	if !reflect.DeepEqual(schemaruntime.BuildCommandMetaLookup(registry), meta.CommandMetaByPath) {
		t.Fatal("real CommandMeta lookup differs after cache round trip")
	}
	wantOverview, err := registry.ToOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	gotOverview, err := meta.Overview.ToPayload()
	if err != nil || !reflect.DeepEqual(wantOverview, gotOverview) {
		t.Fatalf("real overview differs after cache round trip: %v", err)
	}
	decodedTools := 0
	for _, descriptor := range meta.ProductDescriptors {
		offset, length, boundsErr := schemaruntime.ProductShardBounds(descriptor, uint64(len(built.ProductShards)))
		if boundsErr != nil {
			t.Fatal(boundsErr)
		}
		start := int(offset)
		decoded, decodeErr := schemaruntime.DecodeSchemaProductCache(built.ProductShards[start:start+length], descriptor, meta)
		if decodeErr != nil {
			t.Fatalf("decode product %s: %v", descriptor.ProductID, decodeErr)
		}
		decodedTools += len(decoded.Registry.Products[0].Tools)
	}
	if decodedTools != wantTools {
		t.Fatalf("decoded tools = %d, want %d", decodedTools, wantTools)
	}
	decoded, index, err := schemaruntime.DecodeAllSchemaProducts(built.ProductShards, meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.CanonicalPaths()) != wantTools {
		t.Fatalf("global index tools = %d, want %d", len(index.CanonicalPaths()), wantTools)
	}
	if !reflect.DeepEqual(registry, decoded) {
		t.Fatal("real typed Registry differs after cache round trip")
	}
	wantPayload, err := registry.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	gotPayload, err := decoded.ToPayload()
	if err != nil || !reflect.DeepEqual(wantPayload, gotPayload) {
		t.Fatalf("real public Registry wire differs after cache round trip: %v", err)
	}
}

func BenchmarkRealSchemaCache(b *testing.B) {
	registry := assembleRealRegistry(b)
	built, meta := buildRealCache(b, registry)
	descriptor := meta.ProductDescriptors[len(meta.ProductDescriptors)/2]
	for _, candidate := range meta.ProductDescriptors {
		if candidate.ProductID == "calendar" {
			descriptor = candidate
			break
		}
	}
	offset, length, err := schemaruntime.ProductShardBounds(descriptor, uint64(len(built.ProductShards)))
	if err != nil {
		b.Fatal(err)
	}
	start := int(offset)
	productPayload := built.ProductShards[start : start+length]
	b.Run("artifact-sizes", func(b *testing.B) {
		b.ReportMetric(float64(len(built.Meta)), "meta-bytes")
		b.ReportMetric(float64(len(built.ProductShards)), "all-shards-bytes")
		b.ReportMetric(float64(len(productPayload)), "selected-shard-bytes")
		b.ReportMetric(float64(len(meta.ProductDescriptors)), "products")
		b.ReportMetric(float64(len(schemaruntime.BuildCommandMetaLookup(registry))), "meta-lookups")
		b.ReportMetric(float64(realSchemaToolCount(registry)), "tools")
	})
	b.Run("meta-decode-validate-convert", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(built.Meta)))
		for b.Loop() {
			if _, decodeErr := schemaruntime.DecodeSchemaMetaCache(built.Meta); decodeErr != nil {
				b.Fatal(decodeErr)
			}
		}
	})
	b.Run("selected-product-verify-decode-convert-index", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(productPayload)))
		for b.Loop() {
			if _, decodeErr := schemaruntime.DecodeSchemaProductCache(productPayload, descriptor, meta); decodeErr != nil {
				b.Fatal(decodeErr)
			}
		}
	})
	b.Run("all-verify-decode-convert-index", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(built.ProductShards)))
		for b.Loop() {
			if _, _, decodeErr := schemaruntime.DecodeAllSchemaProducts(built.ProductShards, meta); decodeErr != nil {
				b.Fatal(decodeErr)
			}
		}
	})
}

type testFataler interface {
	Helper()
	Fatal(args ...any)
}

func assembleRealRegistry(t testFataler) cli.SchemaRegistry {
	t.Helper()
	registry, err := cli.AssembleSchemaRegistry(app.NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatal(err)
	}
	if realSchemaToolCount(registry) == 0 {
		t.Fatal("authoritative Schema assembly is empty")
	}
	return registry
}

func buildRealCache(t testFataler, registry cli.SchemaRegistry) (schemaruntime.BuiltSchemaCache, schemaruntime.DecodedSchemaMeta) {
	t.Helper()
	overview, err := schemaruntime.BuildSchemaOverview(registry)
	if err != nil {
		t.Fatal(err)
	}
	locators, err := schemaruntime.BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	hashes := schemaruntime.CacheHashes{SourceSHA256: sha256.Sum256([]byte("real-source")), SurfaceSHA256: sha256.Sum256([]byte("real-surface"))}
	built, err := schemaruntime.BuildSchemaCache(registry, schemaruntime.BuildCommandMetaLookup(registry), overview, locators, hashes)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := schemaruntime.DecodeSchemaMetaCache(built.Meta)
	if err != nil {
		t.Fatal(err)
	}
	return built, meta
}
