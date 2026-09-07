// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemareader

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

// ReadMeta only decodes bytes authenticated against the binary's expectation.
// It also binds the DTO's Registry descriptor to that same pinned identity.
func ReadMeta(cache *schemacache.Cache, identity Identity) (schemaruntime.DecodedSchemaMeta, error) {
	payload, err := cache.ReadMeta(identity.ExpectedIdentity(), identity.Meta)
	if err != nil {
		return schemaruntime.DecodedSchemaMeta{}, err
	}
	meta, err := schemaruntime.DecodeSchemaMetaCache(payload)
	if err != nil {
		return schemaruntime.DecodedSchemaMeta{}, err
	}
	if meta.Hashes.SourceSHA256 != identity.SourceSHA256 || meta.Hashes.SurfaceSHA256 != identity.SurfaceSHA256 ||
		meta.RegistryDataLength != identity.Registry.EncodedLength || meta.RegistryDataSHA256 != identity.Registry.EncodedSHA256 {
		return schemaruntime.DecodedSchemaMeta{}, schemacache.ErrIdentityMismatch
	}
	return meta, nil
}

// Descriptor selects only a range from previously authenticated Meta.
func Descriptor(meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.ProductDescriptor, bool) {
	i := sort.Search(len(meta.ProductDescriptors), func(i int) bool { return meta.ProductDescriptors[i].ProductID >= productID })
	if i == len(meta.ProductDescriptors) || meta.ProductDescriptors[i].ProductID != productID {
		return schemaruntime.ProductDescriptor{}, false
	}
	return meta.ProductDescriptors[i], true
}

// ReadProduct authenticates the selected range before any protobuf decoding.
// Meta must come from ReadMeta; callers must never supply disk-learned identity.
func ReadProduct(cache *schemacache.Cache, identity Identity, meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.DecodedSchemaProduct, error) {
	descriptor, ok := Descriptor(meta, productID)
	if !ok {
		return schemaruntime.DecodedSchemaProduct{}, fmt.Errorf("unknown Schema product %q", productID)
	}
	registry, err := cache.OpenRegistry(identity.ExpectedIdentity(), identity.Registry)
	if err != nil {
		return schemaruntime.DecodedSchemaProduct{}, err
	}
	defer registry.Close()
	payload, err := registry.ReadRange(schemacache.RangeDescriptor{Offset: descriptor.Offset, Length: descriptor.Length, SHA256: descriptor.SHA256})
	if err != nil {
		return schemaruntime.DecodedSchemaProduct{}, err
	}
	return schemaruntime.DecodeSchemaProductCache(payload, descriptor, meta)
}

// PayloadDescriptor selects only a command payload range from previously
// authenticated Meta.
func PayloadDescriptor(meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.CommandPayloadDescriptor, bool) {
	i := sort.Search(len(meta.PayloadDescriptors), func(i int) bool { return meta.PayloadDescriptors[i].ProductID >= productID })
	if i == len(meta.PayloadDescriptors) || meta.PayloadDescriptors[i].ProductID != productID {
		return schemaruntime.CommandPayloadDescriptor{}, false
	}
	return meta.PayloadDescriptors[i], true
}

// ReadCommandPayload authenticates the command payload range before any
// protobuf decoding. It reads the payload file, which is deliberately
// independent of the registry so a corrupted registry cannot affect it. The
// payload expectation is derived from the Meta, which is itself authenticated.
func ReadCommandPayload(cache *schemacache.Cache, identity Identity, meta schemaruntime.DecodedSchemaMeta, productID string) (schemaruntime.DecodedCommandPayloads, error) {
	descriptor, ok := PayloadDescriptor(meta, productID)
	if !ok {
		return schemaruntime.DecodedCommandPayloads{}, fmt.Errorf("unknown Schema command payload product %q", productID)
	}
	payloads, err := cache.OpenPayloads(identity.ExpectedIdentity(), payloadExpectation(meta))
	if err != nil {
		return schemaruntime.DecodedCommandPayloads{}, err
	}
	defer payloads.Close()
	payload, err := payloads.ReadRange(schemacache.RangeDescriptor{Offset: descriptor.Offset, Length: descriptor.Length, SHA256: descriptor.SHA256})
	if err != nil {
		return schemaruntime.DecodedCommandPayloads{}, err
	}
	return schemaruntime.DecodeSchemaCommandPayloadCache(payload, descriptor, meta)
}

// payloadExpectation derives the payload file expectation from the Meta, which
// is itself authenticated by the pinned identity.
func payloadExpectation(meta schemaruntime.DecodedSchemaMeta) schemacache.ArtifactExpectation {
	return schemacache.ArtifactExpectation{
		Kind: schemacache.KindPayloads, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		FormatVersion: schemacache.DTOFormatVersion, EncodedLength: meta.PayloadDataLength, DecodedLength: meta.PayloadDataLength,
		EncodedSHA256: meta.PayloadSHA256,
	}
}

func Locator(meta schemaruntime.DecodedSchemaMeta, raw string) (string, bool) {
	tokens := schemaruntime.SplitPathTokens(raw)
	candidates := []string{strings.TrimSpace(raw), schemaruntime.NormalizeQueryCLIPath(raw), strings.Join(tokens, ".")}
	for _, candidate := range candidates {
		if product, ok := meta.LocatorProductByPath[candidate]; ok {
			return product, true
		}
	}
	return "", false
}
