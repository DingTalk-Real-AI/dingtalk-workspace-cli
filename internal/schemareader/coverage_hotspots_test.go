// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemareader

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
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
