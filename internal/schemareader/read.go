// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package schemareader composes authenticated cache I/O with the shared typed
// Schema decoder. It owns no declarations, process globals, repair or rendering
// policy; the CLI consumes these immutable identities.
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

// PayloadDescriptor selects one product's command payload range from an
// authenticated payload index.
func PayloadDescriptor(index schemaruntime.DecodedSchemaPayloadIndex, productID string) (schemaruntime.CommandPayloadDescriptor, bool) {
	i := sort.Search(len(index.PayloadDescriptors), func(i int) bool { return index.PayloadDescriptors[i].ProductID >= productID })
	if i == len(index.PayloadDescriptors) || index.PayloadDescriptors[i].ProductID != productID {
		return schemaruntime.CommandPayloadDescriptor{}, false
	}
	return index.PayloadDescriptors[i], true
}

// ReadPayloadIndex reads and authenticates the payload file's self-describing
// index region using only the binary-pinned identity; Meta is never involved.
func ReadPayloadIndex(cache *schemacache.Cache, identity Identity) (schemaruntime.DecodedSchemaPayloadIndex, error) {
	payloads, err := cache.OpenPayloads(identity.ExpectedIdentity(), identity.Payload)
	if err != nil {
		return schemaruntime.DecodedSchemaPayloadIndex{}, err
	}
	defer payloads.Close()
	region, err := payloads.ReadRange(schemacache.RangeDescriptor{Offset: 0, Length: identity.PayloadIndexLength, SHA256: identity.PayloadIndexSHA256})
	if err != nil {
		return schemaruntime.DecodedSchemaPayloadIndex{}, err
	}
	return schemaruntime.DecodeSchemaPayloadIndex(region)
}

// ReadCommandPayload authenticates and decodes one product's command payload
// header (complete CommandMeta rows plus the rendered leaf index), located
// through the pinned payload index. The payload file is deliberately
// independent of the registry so a corrupted registry cannot affect it.
func ReadCommandPayload(cache *schemacache.Cache, identity Identity, index schemaruntime.DecodedSchemaPayloadIndex, productID string) (schemaruntime.DecodedCommandPayloads, error) {
	descriptor, ok := PayloadDescriptor(index, productID)
	if !ok {
		return schemaruntime.DecodedCommandPayloads{}, fmt.Errorf("unknown Schema command payload product %q", productID)
	}
	payloads, err := cache.OpenPayloads(identity.ExpectedIdentity(), identity.Payload)
	if err != nil {
		return schemaruntime.DecodedCommandPayloads{}, err
	}
	defer payloads.Close()
	payload, err := payloads.ReadRange(schemacache.RangeDescriptor{Offset: identity.PayloadIndexLength + descriptor.Offset, Length: descriptor.HeaderLength, SHA256: descriptor.HeaderSHA256})
	if err != nil {
		return schemaruntime.DecodedCommandPayloads{}, err
	}
	return schemaruntime.DecodeSchemaCommandPayloadHeader(payload, descriptor)
}

// ReadRenderedLeaf reads one pre-rendered leaf blob from the product's payload
// shard blob region. The ref comes from the already authenticated shard header.
func ReadRenderedLeaf(cache *schemacache.Cache, identity Identity, index schemaruntime.DecodedSchemaPayloadIndex, productID string, ref schemaruntime.RenderedLeafRef) ([]byte, error) {
	descriptor, ok := PayloadDescriptor(index, productID)
	if !ok {
		return nil, fmt.Errorf("unknown Schema command payload product %q", productID)
	}
	payloads, err := cache.OpenPayloads(identity.ExpectedIdentity(), identity.Payload)
	if err != nil {
		return nil, err
	}
	defer payloads.Close()
	return payloads.ReadRange(schemacache.RangeDescriptor{
		Offset: identity.PayloadIndexLength + descriptor.Offset + descriptor.HeaderLength + ref.Offset,
		Length: ref.Length,
		SHA256: ref.SHA256,
	})
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

// IndexLocator resolves a raw query path to its product through the payload
// index, accepting the same spellings as Locator.
func IndexLocator(index schemaruntime.DecodedSchemaPayloadIndex, raw string) (string, bool) {
	tokens := schemaruntime.SplitPathTokens(raw)
	candidates := []string{strings.TrimSpace(raw), schemaruntime.NormalizeQueryCLIPath(raw), strings.Join(tokens, ".")}
	for _, candidate := range candidates {
		if product, ok := index.LocatorProductByPath[candidate]; ok {
			return product, true
		}
	}
	return "", false
}
