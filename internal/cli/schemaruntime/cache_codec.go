// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// SchemaCacheDTOVersion is the independently validated private DTO version.
	SchemaCacheDTOVersion = 5
	MaxSchemaMetaBytes    = 4 << 20
	MaxSchemaProductBytes = 8 << 20
	MaxSchemaShardData    = 64<<20 - 208
	maxSchemaProducts     = 4096
	maxSchemaMetaEntries  = 200000
	maxSchemaTools        = 100000
	maxSchemaParameters   = 10000
	maxSchemaProvenance   = 10000
	maxSchemaCandidates   = 100000
)

// CacheHashes binds generated DTOs to the public release Schema identities.
type CacheHashes struct {
	SourceSHA256  [sha256.Size]byte
	SurfaceSHA256 [sha256.Size]byte
}

// OverviewSummaryKind identifies the exact ToOverviewPayload fallback used.
type OverviewSummaryKind string

const (
	OverviewSummaryNone         OverviewSummaryKind = ""
	OverviewSummaryAgentSummary OverviewSummaryKind = "agent_summary"
	OverviewSummaryUseWhen      OverviewSummaryKind = "use_when"
	OverviewSummaryDescription  OverviewSummaryKind = "description"
)

// OverviewProduct is one typed no-argument Schema overview entry.
type OverviewProduct struct {
	ID          string
	ToolCount   uint64
	SchemaPath  string
	SummaryKind OverviewSummaryKind
	Summary     string
}

// SchemaOverview is the typed equivalent of SchemaRegistry.ToOverviewPayload.
type SchemaOverview struct {
	Kind          string
	Level         string
	Source        string
	AgentMetadata json.RawMessage
	Products      []OverviewProduct
	ToolCount     uint64
}

// ToPayload renders the exact no-argument overview wire shape.
func (overview SchemaOverview) ToPayload() (map[string]any, error) {
	products := make([]map[string]any, len(overview.Products))
	for i, product := range overview.Products {
		entry := map[string]any{"id": product.ID, "tool_count": int(product.ToolCount), "schema_path": product.SchemaPath}
		switch product.SummaryKind {
		case OverviewSummaryNone:
			if product.Summary != "" {
				return nil, fmt.Errorf("overview product %q has a summary without a kind", product.ID)
			}
		case OverviewSummaryAgentSummary, OverviewSummaryDescription:
			entry[string(product.SummaryKind)] = product.Summary
		case OverviewSummaryUseWhen:
			entry[string(product.SummaryKind)] = []string{product.Summary}
		default:
			return nil, fmt.Errorf("overview product %q has unknown summary kind %q", product.ID, product.SummaryKind)
		}
		products[i] = entry
	}
	payload := map[string]any{
		"kind": defaultString(overview.Kind, "schema"), "level": "products", "count": len(products),
		"tool_count": int(overview.ToolCount), "products": products,
	}
	if overview.Source != "" {
		payload["source"] = overview.Source
	}
	if err := putRawJSON(payload, "agent_metadata", overview.AgentMetadata); err != nil {
		return nil, fmt.Errorf("agent_metadata: %w", err)
	}
	return payload, nil
}

// ProductDescriptor authenticates one range in the concatenated shard payload.
type ProductDescriptor struct {
	ProductID string
	Offset    uint64
	Length    uint64
	SHA256    [sha256.Size]byte
}

// CommandPayloadDescriptor locates one product's Safety and Selection shard in
// the separate payload file, deliberately independent of the registry.
// HeaderLength and HeaderSHA256 cover the shard's 4-byte header length prefix
// plus the header proto, so header-only readers authenticate exactly that
// prefix; rendered leaf blobs carry their own digests inside the header.
type CommandPayloadDescriptor struct {
	ProductID    string
	Offset       uint64
	Length       uint64
	SHA256       [sha256.Size]byte
	HeaderLength uint64
	HeaderSHA256 [sha256.Size]byte
}

// BuiltSchemaCache is one deterministic Meta payload and its concatenated shards.
type BuiltSchemaCache struct {
	Meta               []byte
	ProductShards      []byte
	PayloadShards      []byte
	Descriptors        []ProductDescriptor
	PayloadDescriptors []CommandPayloadDescriptor
	RegistrySHA256     [sha256.Size]byte
	RegistryDataSize   uint64
	PayloadSHA256      [sha256.Size]byte
	PayloadDataSize    uint64
	// The payload file opens with the length-prefixed SchemaPayloadIndex
	// region; these pins let the binary authenticate that region without
	// reading Meta first. Payload descriptor offsets are relative to the first
	// byte after the index region.
	PayloadIndexLength uint64
	PayloadIndexSHA256 [sha256.Size]byte
}

// DecodedSchemaMeta is a fully validated runtime Meta cache.
type DecodedSchemaMeta struct {
	Kind                  string
	Level                 string
	Source                string
	AgentMetadata         json.RawMessage
	CommandMetaByPath     map[string]CommandMeta
	Overview              SchemaOverview
	LocatorProductByPath  map[string]string
	ProductDescriptors    []ProductDescriptor
	PayloadDescriptors    []CommandPayloadDescriptor
	RegistryDataLength    uint64
	PayloadDataLength     uint64
	PayloadSHA256         [sha256.Size]byte
	RegistryDataSHA256    [sha256.Size]byte
	Hashes                CacheHashes
	commandCountByProduct map[string]int
	locatorCountByProduct map[string]int
	// commandEntryShards stays sorted by product id with each shard's entries
	// blob still serialized, so meta decode never parses the command rows.
	// Immutable after decode; safe to share between value copies.
	commandEntryShards []*schemacachepb.CommandMetaEntryShard
}

// CommandMeta resolves one command row. While CommandMetaByPath is fully
// populated (verification paths) this is a plain lookup; ordinary single-path
// resolution decodes only the row's own product shard.
func (m DecodedSchemaMeta) CommandMeta(path string) (CommandMeta, bool) {
	if meta, ok := m.CommandMetaByPath[path]; ok {
		return meta, true
	}
	entries, ok := m.commandEntriesForPath(path)
	if !ok {
		return CommandMeta{}, false
	}
	position := sort.Search(len(entries), func(i int) bool { return entries[i].GetLookupPath() >= path })
	if position == len(entries) || entries[position].GetLookupPath() != path {
		return CommandMeta{}, false
	}
	return commandMetaFromProto(entries[position]), true
}

// commandEntriesForPath locates the path's product through the locator table
// and decodes only that product's entry shard.
func (m DecodedSchemaMeta) commandEntriesForPath(path string) ([]*schemacachepb.CommandMetaEntry, bool) {
	productID := m.LocatorProductByPath[path]
	if productID == "" {
		return nil, false
	}
	return m.commandEntriesForProduct(productID)
}

func (m DecodedSchemaMeta) commandEntriesForProduct(productID string) ([]*schemacachepb.CommandMetaEntry, bool) {
	i := sort.Search(len(m.commandEntryShards), func(i int) bool { return m.commandEntryShards[i].GetProductId() >= productID })
	if i == len(m.commandEntryShards) || m.commandEntryShards[i].GetProductId() != productID {
		return nil, false
	}
	shard := m.commandEntryShards[i]
	var list schemacachepb.CommandMetaEntryList
	if err := proto.Unmarshal(shard.GetEntries(), &list); err != nil {
		return nil, false
	}
	if err := rejectUnknownFieldsAndEnums(&list); err != nil {
		return nil, false
	}
	if len(list.Items) > maxSchemaMetaEntries || uint64(len(list.Items)) != shard.GetEntryCount() {
		return nil, false
	}
	last := ""
	for j, entry := range list.Items {
		if entry == nil || entry.GetLookupPath() == "" || (j > 0 && entry.GetLookupPath() <= last) {
			return nil, false
		}
		last = entry.GetLookupPath()
		if err := validateCommandMetaListPresence(entry); err != nil {
			return nil, false
		}
		identity := commandIdentityFromProto(entry)
		if identity.CLIPath == "" || identity.Canonical == "" || identity.ProductID != productID {
			return nil, false
		}
		if m.LocatorProductByPath[entry.GetLookupPath()] != productID {
			return nil, false
		}
		for _, identityPath := range append([]string{identity.CLIPath, identity.Canonical}, identity.Aliases...) {
			if m.LocatorProductByPath[strings.TrimSpace(identityPath)] != productID {
				return nil, false
			}
		}
	}
	return list.Items, true
}

// commandMetaMapForProduct decodes one product's entry shard into a lookup.
func (m DecodedSchemaMeta) commandMetaMapForProduct(productID string) (map[string]CommandMeta, bool) {
	entries, ok := m.commandEntriesForProduct(productID)
	if !ok {
		return nil, false
	}
	out := make(map[string]CommandMeta, len(entries))
	for _, entry := range entries {
		out[entry.GetLookupPath()] = commandMetaFromProto(entry)
	}
	return out, true
}

// MaterializeCommandMeta decodes every shard into CommandMetaByPath. Only
// verification paths that compare the complete lookup need this; ordinary
// single-path resolution should use CommandMeta.
func (m DecodedSchemaMeta) MaterializeCommandMeta() {
	if m.CommandMetaByPath == nil {
		return
	}
	for _, shard := range m.commandEntryShards {
		entries, ok := m.commandEntriesForProduct(shard.GetProductId())
		if !ok {
			continue
		}
		for _, entry := range entries {
			path := entry.GetLookupPath()
			if _, ok := m.CommandMetaByPath[path]; !ok {
				m.CommandMetaByPath[path] = commandMetaFromProto(entry)
			}
		}
	}
}

// DecodedSchemaProduct contains the exact shard conversion and its typed index.
type DecodedSchemaProduct struct {
	Registry SchemaRegistry
	Index    SchemaIndex
}

// validateRenderedSchemaLeaves requires the pre-rendered compact leaf payloads
// to exactly cover the distinct canonical paths of the lookup's primary tools,
// so a cached leaf query never partially falls back to the registry shard.
func validateRenderedSchemaLeaves(lookup map[string]CommandMeta, rendered map[string][]byte) error {
	if len(rendered) > maxSchemaMetaEntries {
		return fmt.Errorf("rendered Schema leaf count %d exceeds semantic collection limits", len(rendered))
	}
	canonical := make(map[string]bool, len(lookup))
	for path, meta := range lookup {
		if path == meta.Identity.CLIPath && meta.Identity.Canonical != "" {
			canonical[meta.Identity.Canonical] = true
		}
	}
	for path := range canonical {
		blob, ok := rendered[path]
		if !ok {
			return fmt.Errorf("rendered Schema leaves are missing canonical path %q", path)
		}
		if len(blob) < 2 || blob[len(blob)-1] != '\n' || !json.Valid(blob[:len(blob)-1]) {
			return fmt.Errorf("rendered Schema leaf %q is not newline-terminated JSON", path)
		}
	}
	for path := range rendered {
		if !canonical[path] {
			return fmt.Errorf("rendered Schema leaf %q is not a canonical Schema path", path)
		}
	}
	return nil
}

// renderedLeaf is one canonical path's exact pre-rendered compact leaf output
// bytes before it is laid out in the payload shard's blob region.
type renderedLeaf struct {
	path string
	blob []byte
}

// assembleCommandPayloadShard lays out one product's payload shard: a 4-byte
// big-endian header length, the header proto, then the raw leaf blob region.
func assembleCommandPayloadShard(header, blobs []byte) ([]byte, error) {
	if len(header) == 0 || len(header) > math.MaxUint32-4 {
		return nil, fmt.Errorf("command payload header length %d is not representable", len(header))
	}
	payload := make([]byte, 0, 4+len(header)+len(blobs))
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(header)))
	payload = append(payload, prefix[:]...)
	payload = append(payload, header...)
	payload = append(payload, blobs...)
	return payload, nil
}

// BuildSchemaOverview creates the typed no-argument projection without maps.
func BuildSchemaOverview(registry SchemaRegistry) (SchemaOverview, error) {
	if _, err := registry.Index(); err != nil {
		return SchemaOverview{}, fmt.Errorf("validate Schema Registry: %w", err)
	}
	registry = sortRegistryExact(registry)
	overview := SchemaOverview{
		Kind:          defaultString(registry.Kind, "schema"),
		Level:         "products",
		Source:        registry.Source,
		AgentMetadata: cloneBytes(registry.AgentMetadata),
		Products:      make([]OverviewProduct, len(registry.Products)),
	}
	for i, product := range registry.Products {
		entry := OverviewProduct{ID: product.ID, ToolCount: uint64(len(product.Tools)), SchemaPath: product.ID}
		switch {
		case strings.TrimSpace(product.Selection.AgentSummary) != "":
			entry.SummaryKind, entry.Summary = OverviewSummaryAgentSummary, strings.TrimSpace(product.Selection.AgentSummary)
		case len(product.Selection.UseWhen) > 0:
			entry.SummaryKind, entry.Summary = OverviewSummaryUseWhen, product.Selection.UseWhen[0]
		case product.Description != "":
			entry.SummaryKind, entry.Summary = OverviewSummaryDescription, product.Description
		}
		overview.Products[i] = entry
		overview.ToolCount += entry.ToolCount
	}
	return overview, nil
}

// BuildSchemaProductLocators returns every explicit Registry index locator.
func BuildSchemaProductLocators(registry SchemaRegistry) (map[string]string, error) {
	if _, err := registry.Index(); err != nil {
		return nil, fmt.Errorf("validate Schema Registry: %w", err)
	}
	return buildSchemaProductLocatorsUnchecked(registry)
}

func buildSchemaProductLocatorsUnchecked(registry SchemaRegistry) (map[string]string, error) {
	locators := make(map[string]string)
	add := func(path, productID string) error {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil
		}
		if old, ok := locators[path]; ok && old != productID {
			return fmt.Errorf("Schema locator %q resolves to both %q and %q", path, old, productID)
		}
		locators[path] = productID
		return nil
	}
	for _, product := range registry.Products {
		if err := add(product.ID, product.ID); err != nil {
			return nil, err
		}
		for _, tool := range product.Tools {
			paths := []string{tool.Identity.CanonicalPath, tool.Identity.Path, tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}
			if tool.Identity.SourceProductID != "" && tool.Identity.SourceProductID != tool.Identity.ProductID {
				paths = append(paths, tool.Identity.SourceProductID+"."+tool.Identity.Name)
			}
			paths = append(paths, tool.Identity.Aliases...)
			for _, path := range paths {
				if err := add(path, product.ID); err != nil {
					return nil, err
				}
				tokens := SplitPathTokens(path)
				for end := 1; end < len(tokens); end++ {
					if err := add(strings.Join(tokens[:end], " "), product.ID); err != nil {
						return nil, err
					}
					if err := add(strings.Join(tokens[:end], "."), product.ID); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return locators, nil
}

// BuildSchemaCache builds stable product-sorted shards and the authenticating Meta.
// The supplied projections must exactly match the validated Registry. rendered
// carries each canonical Schema path's exact pre-rendered compact leaf payload
// (indented JSON plus trailing newline) for the payload shard's rendered leaves.
func BuildSchemaCache(registry SchemaRegistry, lookup map[string]CommandMeta, overview SchemaOverview, locators map[string]string, hashes CacheHashes, rendered map[string][]byte) (BuiltSchemaCache, error) {
	if _, err := registry.Index(); err != nil {
		return BuiltSchemaCache{}, fmt.Errorf("validate Schema Registry: %w", err)
	}
	if len(registry.Products) == 0 || len(registry.Products) > maxSchemaProducts {
		return BuiltSchemaCache{}, fmt.Errorf("Schema Registry product count %d is outside 1..%d", len(registry.Products), maxSchemaProducts)
	}
	if len(lookup) > maxSchemaMetaEntries || len(locators) > maxSchemaMetaEntries {
		return BuiltSchemaCache{}, fmt.Errorf("Schema Registry projections exceed semantic collection limits")
	}
	wantLookup := BuildCommandMetaLookup(registry)
	if !reflect.DeepEqual(lookup, wantLookup) {
		return BuiltSchemaCache{}, fmt.Errorf("CommandMeta lookup does not exactly match Schema Registry")
	}
	if err := validateRenderedSchemaLeaves(lookup, rendered); err != nil {
		return BuiltSchemaCache{}, err
	}
	// Index already succeeded above; BuildSchemaOverview only fails on Index.
	wantOverview, _ := BuildSchemaOverview(registry)
	if !reflect.DeepEqual(overview, wantOverview) {
		return BuiltSchemaCache{}, fmt.Errorf("Schema overview does not exactly match Schema Registry")
	}
	wantLocators, err := buildSchemaProductLocatorsUnchecked(registry)
	if err != nil {
		return BuiltSchemaCache{}, err
	}
	if !reflect.DeepEqual(locators, wantLocators) {
		return BuiltSchemaCache{}, fmt.Errorf("Schema locator lookup does not exactly match Schema Registry")
	}

	registry = sortRegistryExact(registry)
	result := BuiltSchemaCache{Descriptors: make([]ProductDescriptor, 0, len(registry.Products))}
	for i := range registry.Products {
		product, conversionErr := productToProto(registry.Products[i])
		if conversionErr != nil {
			return BuiltSchemaCache{}, fmt.Errorf("convert product %q: %w", registry.Products[i].ID, conversionErr)
		}
		root := &schemacachepb.SchemaProductCache{
			DtoVersion: schemacachepb.DTOVersion_DTO_VERSION_V5,
			Registry:   registryFieldsToProto(registry),
			Product:    product,
		}
		payload, marshalErr := MarshalSchemaCacheDeterministic(root)
		if marshalErr != nil {
			return BuiltSchemaCache{}, fmt.Errorf("marshal product %q: %w", registry.Products[i].ID, marshalErr)
		}
		if len(payload) == 0 || len(payload) > MaxSchemaProductBytes {
			return BuiltSchemaCache{}, fmt.Errorf("product %q shard length %d is outside 1..%d", registry.Products[i].ID, len(payload), MaxSchemaProductBytes)
		}
		digest := sha256.Sum256(payload)
		result.Descriptors = append(result.Descriptors, ProductDescriptor{
			ProductID: registry.Products[i].ID,
			Offset:    uint64(len(result.ProductShards)),
			Length:    uint64(len(payload)),
			SHA256:    digest,
		})
		result.ProductShards = append(result.ProductShards, payload...)
	}
	if len(result.ProductShards) > MaxSchemaShardData {
		return BuiltSchemaCache{}, fmt.Errorf("registry shard data length %d exceeds %d", len(result.ProductShards), MaxSchemaShardData)
	}
	result.RegistryDataSize = uint64(len(result.ProductShards))
	result.RegistrySHA256 = sha256.Sum256(result.ProductShards)
	commandEntryShards, err := commandLookupToShards(lookup)
	if err != nil {
		return BuiltSchemaCache{}, err
	}
	payloadByProduct := make(map[string][]*schemacachepb.CommandPayloadEntry, len(registry.Products))
	renderedByProduct := make(map[string][]renderedLeaf, len(registry.Products))
	canonicalProduct := make(map[string]string, len(lookup))
	for path, meta := range lookup {
		payloadByProduct[meta.Identity.ProductID] = append(payloadByProduct[meta.Identity.ProductID],
			commandPayloadToProto(path, meta))
		if path == meta.Identity.CLIPath && meta.Identity.Canonical != "" {
			canonicalProduct[meta.Identity.Canonical] = meta.Identity.ProductID
		}
	}
	for path, blob := range rendered {
		renderedByProduct[canonicalProduct[path]] = append(renderedByProduct[canonicalProduct[path]],
			renderedLeaf{path: path, blob: blob})
	}
	result.PayloadDescriptors = make([]CommandPayloadDescriptor, 0, len(payloadByProduct))
	for _, productID := range sortedMapKeys(payloadByProduct) {
		entries := payloadByProduct[productID]
		sort.Slice(entries, func(i, j int) bool { return entries[i].LookupPath < entries[j].LookupPath })
		leaves := renderedByProduct[productID]
		sort.Slice(leaves, func(i, j int) bool { return leaves[i].path < leaves[j].path })
		var blobs []byte
		refs := make([]*schemacachepb.RenderedSchemaLeafRef, len(leaves))
		for i, leaf := range leaves {
			sum := sha256.Sum256(leaf.blob)
			refs[i] = &schemacachepb.RenderedSchemaLeafRef{
				CanonicalPath: leaf.path, Offset: uint64(len(blobs)), Length: uint64(len(leaf.blob)), Sha256: cloneBytes(sum[:]),
			}
			blobs = append(blobs, leaf.blob...)
		}
		header, marshalErr := MarshalSchemaCacheDeterministic(&schemacachepb.SchemaCommandPayloadCache{
			DtoVersion:        schemacachepb.DTOVersion_DTO_VERSION_V5,
			ProductId:         productID,
			Entries:           &schemacachepb.CommandPayloadEntryList{Items: entries},
			RenderedLeafIndex: &schemacachepb.RenderedSchemaLeafRefList{Items: refs},
		})
		if marshalErr != nil {
			return BuiltSchemaCache{}, fmt.Errorf("marshal command payloads for product %q: %w", productID, marshalErr)
		}
		payload, marshalErr := assembleCommandPayloadShard(header, blobs)
		if marshalErr != nil {
			return BuiltSchemaCache{}, fmt.Errorf("marshal command payloads for product %q: %w", productID, marshalErr)
		}
		if len(payload) == 0 || len(payload) > MaxSchemaProductBytes {
			return BuiltSchemaCache{}, fmt.Errorf("command payload shard %q length %d is outside 1..%d", productID, len(payload), MaxSchemaProductBytes)
		}
		digest := sha256.Sum256(payload)
		headerDigest := sha256.Sum256(payload[:4+len(header)])
		result.PayloadDescriptors = append(result.PayloadDescriptors, CommandPayloadDescriptor{
			ProductID:    productID,
			Offset:       uint64(len(result.PayloadShards)),
			Length:       uint64(len(payload)),
			SHA256:       digest,
			HeaderLength: uint64(4 + len(header)),
			HeaderSHA256: headerDigest,
		})
		result.PayloadShards = append(result.PayloadShards, payload...)
	}
	indexRoot := &schemacachepb.SchemaPayloadIndex{
		DtoVersion: schemacachepb.DTOVersion_DTO_VERSION_V5,
		Locators:   locatorsToProto(locators),
		Products:   payloadDescriptorsToProto(result.PayloadDescriptors),
	}
	indexBytes, marshalErr := MarshalSchemaCacheDeterministic(indexRoot)
	if marshalErr != nil {
		return BuiltSchemaCache{}, fmt.Errorf("marshal Schema payload index: %w", marshalErr)
	}
	indexRegion, marshalErr := assembleCommandPayloadShard(indexBytes, nil)
	if marshalErr != nil {
		return BuiltSchemaCache{}, fmt.Errorf("marshal Schema payload index: %w", marshalErr)
	}
	result.PayloadShards = append(indexRegion, result.PayloadShards...)
	result.PayloadIndexLength = uint64(len(indexRegion))
	result.PayloadIndexSHA256 = sha256.Sum256(indexRegion)
	result.PayloadDataSize = uint64(len(result.PayloadShards))
	result.PayloadSHA256 = sha256.Sum256(result.PayloadShards)
	meta := &schemacachepb.SchemaMetaCache{
		DtoVersion:                schemacachepb.DTOVersion_DTO_VERSION_V5,
		Registry:                  registryFieldsToProto(registry),
		CommandEntryShards:        commandEntryShards,
		Overview:                  overviewToProto(overview),
		Locators:                  locatorsToProto(locators),
		ProductDescriptors:        descriptorsToProto(result.Descriptors),
		CommandPayloadDescriptors: payloadDescriptorsToProto(result.PayloadDescriptors),
		RegistryDataLength:        result.RegistryDataSize,
		RegistryDataSha256:        cloneBytes(result.RegistrySHA256[:]),
		SourceSha256:              cloneBytes(hashes.SourceSHA256[:]),
		SurfaceSha256:             cloneBytes(hashes.SurfaceSHA256[:]),
		PayloadDataLength:         result.PayloadDataSize,
		PayloadSha256:             cloneBytes(result.PayloadSHA256[:]),
	}
	result.Meta, err = MarshalSchemaCacheDeterministic(meta)
	if err != nil {
		return BuiltSchemaCache{}, fmt.Errorf("marshal Schema Meta: %w", err)
	}
	if len(result.Meta) == 0 || len(result.Meta) > MaxSchemaMetaBytes {
		return BuiltSchemaCache{}, fmt.Errorf("Schema Meta length %d is outside 1..%d", len(result.Meta), MaxSchemaMetaBytes)
	}
	return result, nil
}

// marshalSchemaCacheMessage is the protobuf encoder used by cache builds. Tests
// swap it to force marshal and length-limit failures that a well-formed DTO
// cannot otherwise produce.
var marshalSchemaCacheMessage = func(message proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
}

// MarshalSchemaCacheDeterministic is the only production protobuf encoder.
func MarshalSchemaCacheDeterministic(message proto.Message) ([]byte, error) {
	if message == nil || !message.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("cannot marshal nil Schema cache message")
	}
	return marshalSchemaCacheMessage(message)
}

// DecodeSchemaMetaCache rejects unbounded, unknown, unordered, or inconsistent DTOs.
func DecodeSchemaMetaCache(payload []byte) (DecodedSchemaMeta, error) {
	if len(payload) == 0 || len(payload) > MaxSchemaMetaBytes {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta length %d is outside 1..%d", len(payload), MaxSchemaMetaBytes)
	}
	var root schemacachepb.SchemaMetaCache
	if err := (proto.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, &root); err != nil {
		return DecodedSchemaMeta{}, fmt.Errorf("decode Schema Meta protobuf: %w", err)
	}
	if err := rejectUnknownFieldsAndEnums(&root); err != nil {
		return DecodedSchemaMeta{}, err
	}
	return validateAndConvertMeta(&root)
}

// ProductShardBounds validates a descriptor before an offset or allocation is used.
func ProductShardBounds(descriptor ProductDescriptor, total uint64) (int64, int, error) {
	if descriptor.Length == 0 || descriptor.Length > MaxSchemaProductBytes {
		return 0, 0, fmt.Errorf("product %q has invalid shard length %d", descriptor.ProductID, descriptor.Length)
	}
	if descriptor.Offset > total || descriptor.Length > total-descriptor.Offset {
		return 0, 0, fmt.Errorf("product %q range %d:%d exceeds shard data length %d", descriptor.ProductID, descriptor.Offset, descriptor.Length, total)
	}
	if descriptor.Offset > math.MaxInt64 || descriptor.Length > uint64(math.MaxInt) {
		return 0, 0, fmt.Errorf("product %q range is not representable", descriptor.ProductID)
	}
	return int64(descriptor.Offset), int(descriptor.Length), nil
}

// DecodeSchemaProductCache verifies an already bounded shard before protobuf decode.
func DecodeSchemaProductCache(payload []byte, descriptor ProductDescriptor, meta DecodedSchemaMeta) (DecodedSchemaProduct, error) {
	return decodeSchemaProductCache(payload, descriptor, meta, true)
}

func decodeSchemaProductCache(payload []byte, descriptor ProductDescriptor, meta DecodedSchemaMeta, buildIndex bool) (DecodedSchemaProduct, error) {
	authenticated, ok := metaDescriptor(meta, descriptor.ProductID)
	if !ok || authenticated != descriptor {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q descriptor is not authenticated by Schema Meta", descriptor.ProductID)
	}
	if uint64(len(payload)) != descriptor.Length {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q shard length %d, want %d", descriptor.ProductID, len(payload), descriptor.Length)
	}
	if sha256.Sum256(payload) != descriptor.SHA256 {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q shard SHA-256 mismatch", descriptor.ProductID)
	}
	var root schemacachepb.SchemaProductCache
	if err := (proto.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, &root); err != nil {
		return DecodedSchemaProduct{}, fmt.Errorf("decode product %q protobuf: %w", descriptor.ProductID, err)
	}
	if err := rejectUnknownFieldsAndEnums(&root); err != nil {
		return DecodedSchemaProduct{}, err
	}
	if root.GetDtoVersion() != schemacachepb.DTOVersion_DTO_VERSION_V5 {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q DTO version is %d, want %d", descriptor.ProductID, root.GetDtoVersion(), SchemaCacheDTOVersion)
	}
	if root.GetRegistry() == nil || root.GetProduct() == nil {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q DTO is missing registry fields or product", descriptor.ProductID)
	}
	if raw := root.Registry.GetAgentMetadata(); raw != nil && len(raw.Value) > 0 && !json.Valid(raw.Value) {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q registry agent_metadata is invalid JSON", descriptor.ProductID)
	}
	if err := validateProductProto(root.GetProduct()); err != nil {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q DTO: %w", descriptor.ProductID, err)
	}
	registry := registryFromProductProto(&root)
	if registry.Kind != meta.Kind || registry.Level != meta.Level || registry.Source != meta.Source || !reflect.DeepEqual(registry.AgentMetadata, meta.AgentMetadata) {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q registry fields disagree with Schema Meta", descriptor.ProductID)
	}
	if len(registry.Products) != 1 || registry.Products[0].ID != descriptor.ProductID {
		return DecodedSchemaProduct{}, fmt.Errorf("product shard identity %q does not match descriptor %q", registry.Products[0].ID, descriptor.ProductID)
	}
	var index SchemaIndex
	var err error
	if buildIndex {
		index, err = registry.Index()
		if err != nil {
			return DecodedSchemaProduct{}, fmt.Errorf("validate product %q: %w", descriptor.ProductID, err)
		}
	}
	wantLookup := BuildCommandMetaLookup(registry)
	count, present := meta.commandCountByProduct[descriptor.ProductID]
	productEntries, entriesOK := meta.commandMetaMapForProduct(descriptor.ProductID)
	if !present || !entriesOK || count != len(wantLookup) || !commandIdentitySubsetEqual(productEntries, wantLookup) {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q CommandMeta entries disagree with shard", descriptor.ProductID)
	}
	// A single-product shard always locates to that product ID, so add() cannot collide.
	wantLocators, _ := buildSchemaProductLocatorsUnchecked(registry)
	locatorCount, locatorsPresent := meta.locatorCountByProduct[descriptor.ProductID]
	if !locatorsPresent || locatorCount != len(wantLocators) || !locatorSubsetEqual(meta.LocatorProductByPath, wantLocators) {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q locator entries disagree with shard", descriptor.ProductID)
	}
	position := sort.Search(len(meta.Overview.Products), func(i int) bool { return meta.Overview.Products[i].ID >= descriptor.ProductID })
	if position == len(meta.Overview.Products) || meta.Overview.Products[position].ID != descriptor.ProductID || meta.Overview.Products[position].ToolCount != uint64(len(registry.Products[0].Tools)) {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q overview entry disagrees with shard", descriptor.ProductID)
	}
	return DecodedSchemaProduct{Registry: registry, Index: index}, nil
}

// DecodedCommandPayloads holds the complete CommandMeta rows for one
// product's commands, decoded from its payload shard header.
type DecodedCommandPayloads struct {
	ProductID string
	// Commands maps each lookup path (primary and alias) to the complete
	// CommandMeta: identity, Safety, and Selection.
	Commands map[string]CommandMeta
	// LeafIndex locates each canonical path's pre-rendered compact leaf bytes
	// inside the shard's blob region, sorted by canonical path.
	LeafIndex []RenderedLeafRef
	// RenderedLeaves carries the verified leaf bytes only for the whole-shard
	// decode used by round-trip verification; header reads leave it nil.
	RenderedLeaves map[string][]byte
}

// metaPayloadDescriptor finds the payload descriptor authenticated by the Meta.
func metaPayloadDescriptor(meta DecodedSchemaMeta, productID string) (CommandPayloadDescriptor, bool) {
	i := sort.Search(len(meta.PayloadDescriptors), func(i int) bool { return meta.PayloadDescriptors[i].ProductID >= productID })
	if i == len(meta.PayloadDescriptors) || meta.PayloadDescriptors[i].ProductID != productID {
		return CommandPayloadDescriptor{}, false
	}
	return meta.PayloadDescriptors[i], true
}

// RenderedLeafRef locates one canonical path's pre-rendered compact leaf bytes
// inside the shard's blob region (offset 0 = first byte after the header).
type RenderedLeafRef struct {
	CanonicalPath string
	Offset        uint64
	Length        uint64
	SHA256        [sha256.Size]byte
}

// RenderedLeaf finds the leaf ref for one canonical path by binary search.
func (d DecodedCommandPayloads) RenderedLeaf(canonical string) (RenderedLeafRef, bool) {
	i := sort.Search(len(d.LeafIndex), func(i int) bool { return d.LeafIndex[i].CanonicalPath >= canonical })
	if i == len(d.LeafIndex) || d.LeafIndex[i].CanonicalPath != canonical {
		return RenderedLeafRef{}, false
	}
	return d.LeafIndex[i], true
}

// splitCommandPayloadShard separates the length-prefixed header from the raw
// leaf blob region without copying either.
func splitCommandPayloadShard(payload []byte) (header []byte, blobs []byte, err error) {
	if len(payload) < 4 {
		return nil, nil, fmt.Errorf("command payload shard is shorter than its header prefix")
	}
	headerLength := int(binary.BigEndian.Uint32(payload[:4]))
	if headerLength <= 0 || headerLength > len(payload)-4 {
		return nil, nil, fmt.Errorf("command payload shard header length %d is outside 1..%d", headerLength, len(payload)-4)
	}
	return payload[4 : 4+headerLength], payload[4+headerLength:], nil
}

// decodeCommandPayloadHeader validates the header message and converts its
// Safety/Selection entries plus the rendered leaf index. Blob bytes are not
// touched here; they carry their own digests and are verified per leaf read.
func decodeCommandPayloadHeader(header []byte, descriptor CommandPayloadDescriptor) (DecodedCommandPayloads, error) {
	var root schemacachepb.SchemaCommandPayloadCache
	if err := (proto.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(header, &root); err != nil {
		return DecodedCommandPayloads{}, fmt.Errorf("decode product %q command payload protobuf: %w", descriptor.ProductID, err)
	}
	if err := rejectUnknownFieldsAndEnums(&root); err != nil {
		return DecodedCommandPayloads{}, err
	}
	if root.GetDtoVersion() != schemacachepb.DTOVersion_DTO_VERSION_V5 {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload DTO version is %d, want %d", descriptor.ProductID, root.GetDtoVersion(), SchemaCacheDTOVersion)
	}
	if root.GetProductId() != descriptor.ProductID || root.GetEntries() == nil || root.GetRenderedLeafIndex() == nil {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload identity does not match descriptor", descriptor.ProductID)
	}
	result := DecodedCommandPayloads{
		ProductID: descriptor.ProductID,
		Commands:  make(map[string]CommandMeta, len(root.Entries.Items)),
		LeafIndex: make([]RenderedLeafRef, len(root.RenderedLeafIndex.Items)),
	}
	for _, entry := range root.Entries.Items {
		if entry.GetIdentity() == nil {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload entry %q is missing its identity", descriptor.ProductID, entry.GetLookupPath())
		}
		if err := validateCommandMetaListPresence(entry.GetIdentity()); err != nil {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload entry %q: %w", descriptor.ProductID, entry.GetLookupPath(), err)
		}
		meta := commandPayloadFromProto(entry)
		if meta.Identity.CLIPath == "" || meta.Identity.Canonical == "" || meta.Identity.ProductID != descriptor.ProductID {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload entry %q has an incomplete identity", descriptor.ProductID, entry.GetLookupPath())
		}
		result.Commands[entry.GetLookupPath()] = meta
	}
	last := ""
	for i, leaf := range root.RenderedLeafIndex.Items {
		if leaf == nil || leaf.GetCanonicalPath() == "" || (i > 0 && leaf.GetCanonicalPath() <= last) {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q rendered Schema leaf index is empty, duplicate, or unsorted at %q", descriptor.ProductID, leaf.GetCanonicalPath())
		}
		last = leaf.GetCanonicalPath()
		if len(leaf.GetSha256()) != sha256.Size {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q rendered Schema leaf %q has a non-SHA-256 digest", descriptor.ProductID, leaf.GetCanonicalPath())
		}
		result.LeafIndex[i] = RenderedLeafRef{CanonicalPath: leaf.GetCanonicalPath(), Offset: leaf.GetOffset(), Length: leaf.GetLength()}
		copy(result.LeafIndex[i].SHA256[:], leaf.GetSha256())
	}
	return result, nil
}

// DecodedSchemaPayloadIndex is the payload file's self-describing global
// header: the locator table plus the per-product shard descriptors.
type DecodedSchemaPayloadIndex struct {
	LocatorProductByPath map[string]string
	PayloadDescriptors   []CommandPayloadDescriptor
}

// DecodeSchemaPayloadIndex decodes and validates the payload index region,
// including the 4-byte length prefix the pinned digest covers. The region
// bytes must already be authenticated by the caller against the binary-pinned
// payload index digest.
func DecodeSchemaPayloadIndex(region []byte) (DecodedSchemaPayloadIndex, error) {
	if len(region) == 0 || len(region) > MaxSchemaMetaBytes {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index length %d is outside 1..%d", len(region), MaxSchemaMetaBytes)
	}
	payload, blobs, err := splitCommandPayloadShard(region)
	if err != nil {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index: %w", err)
	}
	if len(blobs) != 0 {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index carries %d unexpected trailing bytes", len(blobs))
	}
	var root schemacachepb.SchemaPayloadIndex
	if err := (proto.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, &root); err != nil {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("decode Schema payload index protobuf: %w", err)
	}
	if err := rejectUnknownFieldsAndEnums(&root); err != nil {
		return DecodedSchemaPayloadIndex{}, err
	}
	if root.GetDtoVersion() != schemacachepb.DTOVersion_DTO_VERSION_V5 {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index DTO version is %d, want %d", root.GetDtoVersion(), SchemaCacheDTOVersion)
	}
	if root.GetLocators() == nil || root.GetProducts() == nil {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index is missing a required presence wrapper")
	}
	if len(root.Locators.Items) > maxSchemaMetaEntries || len(root.Products.Items) > maxSchemaProducts {
		return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index exceeds semantic collection limits")
	}
	result := DecodedSchemaPayloadIndex{
		LocatorProductByPath: make(map[string]string, len(root.Locators.Items)),
		PayloadDescriptors:   payloadDescriptorsFromProto(root.Products),
	}
	last := ""
	for i, entry := range root.Locators.Items {
		if entry == nil || entry.GetLookupPath() == "" || entry.GetProductId() == "" || (i > 0 && entry.GetLookupPath() <= last) {
			return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index locator keys are incomplete, duplicate, or unsorted at %q", entry.GetLookupPath())
		}
		last = entry.GetLookupPath()
		result.LocatorProductByPath[entry.GetLookupPath()] = entry.GetProductId()
	}
	for i, descriptor := range root.Products.Items {
		if descriptor == nil || len(descriptor.GetSha256()) != sha256.Size || len(descriptor.GetHeaderSha256()) != sha256.Size || descriptor.GetHeaderLength() == 0 {
			return DecodedSchemaPayloadIndex{}, fmt.Errorf("Schema payload index descriptor %d is incomplete", i)
		}
	}
	return result, nil
}

// AuthenticateCommandPayloadDescriptor binds a command payload descriptor to
// the Meta's authenticated copy.
func AuthenticateCommandPayloadDescriptor(meta DecodedSchemaMeta, descriptor CommandPayloadDescriptor) error {
	authenticated, ok := metaPayloadDescriptor(meta, descriptor.ProductID)
	if !ok || authenticated != descriptor {
		return fmt.Errorf("product %q command payload descriptor is not authenticated by Schema Meta", descriptor.ProductID)
	}
	return nil
}

// DecodeSchemaCommandPayloadHeader authenticates only the shard's header
// prefix against the descriptor, so command reads never pull the rendered leaf
// blob region. The descriptor must come from an already authenticated source
// (Meta or the pinned payload index).
func DecodeSchemaCommandPayloadHeader(payload []byte, descriptor CommandPayloadDescriptor) (DecodedCommandPayloads, error) {
	if uint64(len(payload)) != descriptor.HeaderLength {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload header length %d, want %d", descriptor.ProductID, len(payload), descriptor.HeaderLength)
	}
	if sha256.Sum256(payload) != descriptor.HeaderSHA256 {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload header SHA-256 mismatch", descriptor.ProductID)
	}
	header, _, err := splitCommandPayloadShard(payload)
	if err != nil || len(payload) != 4+len(header) {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload header prefix is inconsistent", descriptor.ProductID)
	}
	return decodeCommandPayloadHeader(header, descriptor)
}

// DecodeSchemaCommandPayloadCache authenticates a whole command payload shard
// against the Meta's descriptor, decodes its Safety and Selection rows, and
// verifies every rendered leaf blob against the header's index digests. The
// header-only variant covers production reads; this full decode serves
// round-trip verification.
func DecodeSchemaCommandPayloadCache(payload []byte, descriptor CommandPayloadDescriptor, meta DecodedSchemaMeta) (DecodedCommandPayloads, error) {
	authenticated, ok := metaPayloadDescriptor(meta, descriptor.ProductID)
	if !ok || authenticated != descriptor {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload descriptor is not authenticated by Schema Meta", descriptor.ProductID)
	}
	if uint64(len(payload)) != descriptor.Length {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload length %d, want %d", descriptor.ProductID, len(payload), descriptor.Length)
	}
	if sha256.Sum256(payload) != descriptor.SHA256 {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload SHA-256 mismatch", descriptor.ProductID)
	}
	header, blobs, err := splitCommandPayloadShard(payload)
	if err != nil {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload: %w", descriptor.ProductID, err)
	}
	if descriptor.HeaderLength != uint64(4+len(header)) || sha256.Sum256(payload[:4+len(header)]) != descriptor.HeaderSHA256 {
		return DecodedCommandPayloads{}, fmt.Errorf("product %q command payload header prefix disagrees with the descriptor", descriptor.ProductID)
	}
	result, err := decodeCommandPayloadHeader(header, descriptor)
	if err != nil {
		return DecodedCommandPayloads{}, err
	}
	result.RenderedLeaves = make(map[string][]byte, len(result.LeafIndex))
	for _, ref := range result.LeafIndex {
		if ref.Offset > uint64(len(blobs)) || ref.Length > uint64(len(blobs))-ref.Offset {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q rendered Schema leaf %q range exceeds the blob region", descriptor.ProductID, ref.CanonicalPath)
		}
		blob := blobs[ref.Offset : ref.Offset+ref.Length]
		if sha256.Sum256(blob) != ref.SHA256 {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q rendered Schema leaf %q SHA-256 mismatch", descriptor.ProductID, ref.CanonicalPath)
		}
		if len(blob) < 2 || blob[len(blob)-1] != '\n' || !json.Valid(blob[:len(blob)-1]) {
			return DecodedCommandPayloads{}, fmt.Errorf("product %q rendered Schema leaf %q is not newline-terminated JSON", descriptor.ProductID, ref.CanonicalPath)
		}
		result.RenderedLeaves[ref.CanonicalPath] = append([]byte(nil), blob...)
	}
	return result, nil
}

// DecodeSchemaProductFromShards checks aggregate identity and a descriptor range.
func DecodeSchemaProductFromShards(shards []byte, meta DecodedSchemaMeta, productID string) (DecodedSchemaProduct, error) {
	if uint64(len(shards)) != meta.RegistryDataLength || len(shards) > MaxSchemaShardData {
		return DecodedSchemaProduct{}, fmt.Errorf("registry shard data length %d, want %d", len(shards), meta.RegistryDataLength)
	}
	if sha256.Sum256(shards) != meta.RegistryDataSHA256 {
		return DecodedSchemaProduct{}, fmt.Errorf("registry shard data SHA-256 mismatch")
	}
	descriptor, ok := metaDescriptor(meta, productID)
	if !ok {
		return DecodedSchemaProduct{}, fmt.Errorf("unknown Schema product %q", productID)
	}
	offset, length, err := ProductShardBounds(descriptor, uint64(len(shards)))
	if err != nil {
		return DecodedSchemaProduct{}, err
	}
	start := int(offset)
	return DecodeSchemaProductCache(shards[start:start+length], descriptor, meta)
}

// DecodeAllSchemaProducts reconstructs and globally indexes the full Registry.
func DecodeAllSchemaProducts(shards []byte, meta DecodedSchemaMeta) (SchemaRegistry, SchemaIndex, error) {
	if uint64(len(shards)) != meta.RegistryDataLength || sha256.Sum256(shards) != meta.RegistryDataSHA256 {
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("registry shard data identity mismatch")
	}
	registry := SchemaRegistry{Kind: meta.Kind, Level: meta.Level, Source: meta.Source, AgentMetadata: cloneBytes(meta.AgentMetadata)}
	registry.Products = make([]ProductSpec, 0, len(meta.ProductDescriptors))
	for _, descriptor := range meta.ProductDescriptors {
		offset, length, boundsErr := ProductShardBounds(descriptor, uint64(len(shards)))
		if boundsErr != nil {
			return SchemaRegistry{}, SchemaIndex{}, boundsErr
		}
		start := int(offset)
		decoded, err := decodeSchemaProductCache(shards[start:start+length], descriptor, meta, false)
		if err != nil {
			return SchemaRegistry{}, SchemaIndex{}, err
		}
		registry.Products = append(registry.Products, decoded.Registry.Products[0])
	}
	index, _ := registry.Index()
	return registry, index, nil
}

func validateAndConvertMeta(root *schemacachepb.SchemaMetaCache) (DecodedSchemaMeta, error) {
	if root.GetDtoVersion() != schemacachepb.DTOVersion_DTO_VERSION_V5 {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta DTO version is %d, want %d", root.GetDtoVersion(), SchemaCacheDTOVersion)
	}
	if root.GetRegistry() == nil || root.GetCommandEntryShards() == nil || root.GetOverview() == nil || root.GetLocators() == nil || root.GetProductDescriptors() == nil {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta is missing a required presence wrapper")
	}
	if root.Overview.GetRegistry() == nil || root.Overview.GetProducts() == nil {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta overview is missing registry fields or products")
	}
	if len(root.CommandEntryShards.Items) > maxSchemaProducts || len(root.Locators.Items) > maxSchemaMetaEntries || len(root.ProductDescriptors.Items) > maxSchemaProducts || len(root.Overview.Products.Items) > maxSchemaProducts {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta exceeds semantic collection limits")
	}
	if root.Registry.AgentMetadata != nil && len(root.Registry.AgentMetadata.Value) > 0 && !json.Valid(root.Registry.AgentMetadata.Value) {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta registry agent_metadata is invalid JSON")
	}
	if len(root.GetRegistryDataSha256()) != sha256.Size || len(root.GetSourceSha256()) != sha256.Size || len(root.GetSurfaceSha256()) != sha256.Size {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta contains a non-SHA-256 digest")
	}
	if root.GetRegistryDataLength() > MaxSchemaShardData {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta registry data length %d exceeds %d", root.GetRegistryDataLength(), MaxSchemaShardData)
	}
	for i, descriptor := range root.ProductDescriptors.Items {
		if descriptor == nil || len(descriptor.GetSha256()) != sha256.Size {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta product descriptor %d has a non-SHA-256 digest", i)
		}
	}
	result := DecodedSchemaMeta{
		Kind:                  root.Registry.GetKind(),
		Level:                 root.Registry.GetLevel(),
		Source:                root.Registry.GetSource(),
		AgentMetadata:         bytesFromProto(root.Registry.GetAgentMetadata()),
		CommandMetaByPath:     make(map[string]CommandMeta),
		Overview:              overviewFromProto(root.Overview),
		LocatorProductByPath:  make(map[string]string, len(root.Locators.Items)),
		ProductDescriptors:    descriptorsFromProto(root.ProductDescriptors),
		PayloadDescriptors:    payloadDescriptorsFromProto(root.CommandPayloadDescriptors),
		RegistryDataLength:    root.GetRegistryDataLength(),
		PayloadDataLength:     root.GetPayloadDataLength(),
		commandCountByProduct: make(map[string]int, len(root.CommandEntryShards.Items)),
	}
	copy(result.PayloadSHA256[:], root.GetPayloadSha256())
	copy(result.RegistryDataSHA256[:], root.GetRegistryDataSha256())
	copy(result.Hashes.SourceSHA256[:], root.GetSourceSha256())
	copy(result.Hashes.SurfaceSHA256[:], root.GetSurfaceSha256())

	last := ""
	for i, shard := range root.CommandEntryShards.Items {
		if shard == nil || shard.GetProductId() == "" || (i > 0 && shard.GetProductId() <= last) {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta command entry shards are empty, duplicate, or unsorted at index %d", i)
		}
		last = shard.GetProductId()
		if len(shard.GetEntries()) == 0 || uint64(len(shard.GetEntries())) > uint64(MaxSchemaMetaBytes) || shard.GetEntryCount() > uint64(maxSchemaMetaEntries) || shard.GetEntryCount() == 0 {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta command entry shard %q is empty or exceeds semantic limits", shard.GetProductId())
		}
		result.commandCountByProduct[shard.GetProductId()] = int(shard.GetEntryCount())
	}
	result.commandEntryShards = root.CommandEntryShards.Items
	last = ""
	for i, entry := range root.Locators.Items {
		if entry == nil || entry.GetLookupPath() == "" || entry.GetProductId() == "" || (i > 0 && entry.GetLookupPath() <= last) {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta locator keys are incomplete, duplicate, or unsorted at %q", entry.GetLookupPath())
		}
		last = entry.GetLookupPath()
		result.LocatorProductByPath[entry.GetLookupPath()] = entry.GetProductId()
	}
	if err := validateOverview(result.Overview, result); err != nil {
		return DecodedSchemaMeta{}, err
	}
	if err := validateDescriptors(result.ProductDescriptors, result.RegistryDataLength); err != nil {
		return DecodedSchemaMeta{}, err
	}
	products := make(map[string]bool, len(result.ProductDescriptors))
	for _, descriptor := range result.ProductDescriptors {
		products[descriptor.ProductID] = true
	}
	for path, productID := range result.LocatorProductByPath {
		if !products[productID] {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema locator %q names unknown product %q", path, productID)
		}
	}
	for productID := range result.commandCountByProduct {
		if !products[productID] {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema command entry shard %q has no product descriptor", productID)
		}
	}
	// Per-entry identity/locator consistency now runs when a product's shard is
	// decoded on access; the complete cross-check also runs in the round-trip
	// tests and at cache build time.
	result.locatorCountByProduct = make(map[string]int, len(products))
	for _, productID := range result.LocatorProductByPath {
		result.locatorCountByProduct[productID]++
	}

	return result, nil
}

func validateOverview(overview SchemaOverview, meta DecodedSchemaMeta) error {
	if overview.Kind != defaultString(meta.Kind, "schema") || overview.Level != "products" || overview.Source != meta.Source || !reflect.DeepEqual(overview.AgentMetadata, meta.AgentMetadata) {
		return fmt.Errorf("Schema overview registry fields disagree with Schema Meta")
	}
	last := ""
	var total uint64
	for i, product := range overview.Products {
		if product.ID == "" || product.SchemaPath != product.ID || (i > 0 && product.ID <= last) {
			return fmt.Errorf("Schema overview products are invalid, duplicate, or unsorted at %q", product.ID)
		}
		if (product.SummaryKind == OverviewSummaryNone) != (product.Summary == "") {
			return fmt.Errorf("Schema overview product %q has inconsistent summary presence", product.ID)
		}
		last = product.ID
		if product.ToolCount > math.MaxUint64-total {
			return fmt.Errorf("Schema overview tool count overflows")
		}
		total += product.ToolCount
	}
	if total != overview.ToolCount {
		return fmt.Errorf("Schema overview tool count %d, want sum %d", overview.ToolCount, total)
	}
	if len(overview.Products) != len(meta.ProductDescriptors) {
		return fmt.Errorf("Schema overview has %d products, want %d descriptors", len(overview.Products), len(meta.ProductDescriptors))
	}
	for i, product := range overview.Products {
		if product.ID != meta.ProductDescriptors[i].ProductID {
			return fmt.Errorf("Schema overview product %q disagrees with descriptor %q", product.ID, meta.ProductDescriptors[i].ProductID)
		}
	}
	return nil
}

func validateDescriptors(descriptors []ProductDescriptor, total uint64) error {
	if len(descriptors) == 0 || total == 0 {
		return fmt.Errorf("Schema Meta has no product descriptors or shard data")
	}
	var next uint64
	for _, descriptor := range descriptors {
		if descriptor.Offset != next {
			return fmt.Errorf("product %q offset %d leaves a gap or overlap after %d", descriptor.ProductID, descriptor.Offset, next)
		}
		if _, _, err := ProductShardBounds(descriptor, total); err != nil {
			return err
		}
		next += descriptor.Length
	}
	if next != total {
		return fmt.Errorf("product descriptors cover %d bytes, want %d", next, total)
	}
	return nil
}

func metaDescriptor(meta DecodedSchemaMeta, productID string) (ProductDescriptor, bool) {
	i := sort.Search(len(meta.ProductDescriptors), func(i int) bool { return meta.ProductDescriptors[i].ProductID >= productID })
	if i == len(meta.ProductDescriptors) || meta.ProductDescriptors[i].ProductID != productID {
		return ProductDescriptor{}, false
	}
	return meta.ProductDescriptors[i], true
}

func rejectUnknownFieldsAndEnums(message proto.Message) error {
	if message == nil || !message.ProtoReflect().IsValid() {
		return nil
	}
	if len(message.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("%s contains unknown protobuf fields", message.ProtoReflect().Descriptor().FullName())
	}
	check := func(messages ...proto.Message) error {
		for _, child := range messages {
			if err := rejectUnknownFieldsAndEnums(child); err != nil {
				return err
			}
		}
		return nil
	}
	switch value := message.(type) {
	case *schemacachepb.SchemaMetaCache:
		if _, ok := schemacachepb.DTOVersion_name[int32(value.GetDtoVersion())]; !ok {
			return fmt.Errorf("SchemaMetaCache contains unknown DTO version %d", value.GetDtoVersion())
		}
		return check(value.Registry, value.CommandEntryShards, value.Overview, value.Locators, value.ProductDescriptors)
	case *schemacachepb.SchemaProductCache:
		if _, ok := schemacachepb.DTOVersion_name[int32(value.GetDtoVersion())]; !ok {
			return fmt.Errorf("SchemaProductCache contains unknown DTO version %d", value.GetDtoVersion())
		}
		return check(value.Registry, value.Product)
	case *schemacachepb.RegistryFields:
		return check(value.AgentMetadata)
	case *schemacachepb.CommandMetaEntryList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.CommandMetaEntry:
		return nil
	case *schemacachepb.SchemaOverviewCache:
		return check(value.Registry, value.Products)
	case *schemacachepb.OverviewProductList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.OverviewProduct:
		if _, ok := schemacachepb.OverviewSummaryKind_name[int32(value.GetSummaryKind())]; !ok {
			return fmt.Errorf("OverviewProduct contains unknown summary kind %d", value.GetSummaryKind())
		}
	case *schemacachepb.LocatorEntryList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.ProductDescriptorList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.ProductDescriptor:
		return nil
	case *schemacachepb.ProductSpec:
		return check(value.Tools, value.Selection, value.FieldProvenance)
	case *schemacachepb.ToolList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.ToolSpec:
		return check(value.Identity, value.Parameters, value.Constraints, value.Positionals, value.DryRun, value.Result, value.Pagination, value.Safety, value.Interface, value.Selection, value.FieldProvenance)
	case *schemacachepb.ParameterList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.ParameterSpec:
		return check(value.DefaultValue, value.InterfaceDefault, value.Example, value.Enum, value.FieldProvenance)
	case *schemacachepb.ToolIdentity:
		return check(value.Aliases)
	case *schemacachepb.Constraints:
		return check(value.MutuallyExclusive, value.RequireOneOf, value.RequireTogether)
	case *schemacachepb.StringListList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.PositionalList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.Result:
		return check(value.Outcomes, value.DataSchema, value.SensitivePaths)
	case *schemacachepb.ResultOutcomeList:
		for _, item := range value.Items {
			if _, ok := schemacachepb.ResultOutcome_name[int32(item)]; !ok {
				return fmt.Errorf("ResultOutcomeList contains unknown outcome %d", item)
			}
		}
	case *schemacachepb.Interface:
		return check(value.Ref)
	case *schemacachepb.Selection:
		return check(value.UseWhen, value.AvoidWhen, value.Prerequisites, value.Tips, value.WorkflowRefs, value.Examples, value.ExampleDispositions, value.Reviewed, value.SourceRefs)
	case *schemacachepb.ExampleDispositionList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.ExampleDisposition:
		if _, ok := schemacachepb.ExampleDispositionMode_name[int32(value.GetMode())]; !ok {
			return fmt.Errorf("ExampleDisposition contains unknown mode %d", value.GetMode())
		}
		if _, ok := schemacachepb.ExampleDispositionReasonCode_name[int32(value.GetReasonCode())]; !ok {
			return fmt.Errorf("ExampleDisposition contains unknown reason code %d", value.GetReasonCode())
		}
		return check(value.Index)
	case *schemacachepb.ProvenanceList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.ProvenanceEntry:
		return check(value.Value)
	case *schemacachepb.FieldProvenance:
		return check(value.Value, value.Candidates, value.OverriddenCandidates)
	case *schemacachepb.CandidateList:
		for _, item := range value.Items {
			if err := rejectUnknownFieldsAndEnums(item); err != nil {
				return err
			}
		}
	case *schemacachepb.FieldCandidate:
		return check(value.Value, value.Selected)
	case *schemacachepb.StringList, *schemacachepb.BytesValue, *schemacachepb.BoolValue, *schemacachepb.IntValue,
		*schemacachepb.LocatorEntry, *schemacachepb.Positional, *schemacachepb.DryRun,
		*schemacachepb.Pagination, *schemacachepb.Safety, *schemacachepb.InterfaceRef:
		return nil
	default:
		return rejectUnknownFieldsAndEnumsReflect(message.ProtoReflect())
	}
	return nil
}

func rejectUnknownFieldsAndEnumsReflect(message protoreflect.Message) error {
	if len(message.GetUnknown()) != 0 {
		return fmt.Errorf("%s contains unknown protobuf fields", message.Descriptor().FullName())
	}
	fields := message.Descriptor().Fields()
	for fieldIndex := 0; fieldIndex < fields.Len(); fieldIndex++ {
		field := fields.Get(fieldIndex)
		if field.Kind() != protoreflect.MessageKind && field.Kind() != protoreflect.EnumKind {
			continue
		}
		if field.IsMap() {
			return fmt.Errorf("%s uses forbidden protobuf map encoding", field.FullName())
		}
		if field.IsList() {
			list := message.Get(field).List()
			for i := 0; i < list.Len(); i++ {
				item := list.Get(i)
				if field.Kind() == protoreflect.EnumKind && field.Enum().Values().ByNumber(item.Enum()) == nil {
					return fmt.Errorf("%s[%d] contains unknown enum value %d", field.FullName(), i, item.Enum())
				}
				if field.Kind() == protoreflect.MessageKind {
					if err := rejectUnknownFieldsAndEnumsReflect(item.Message()); err != nil {
						return err
					}
				}
			}
			continue
		}
		if !message.Has(field) {
			continue
		}
		value := message.Get(field)
		switch field.Kind() {
		case protoreflect.EnumKind:
			if field.Enum().Values().ByNumber(value.Enum()) == nil {
				return fmt.Errorf("%s contains unknown enum value %d", field.FullName(), value.Enum())
			}
		case protoreflect.MessageKind:
			if err := rejectUnknownFieldsAndEnumsReflect(value.Message()); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProductProto(product *schemacachepb.ProductSpec) error {
	if product.GetId() == "" || product.GetSelection() == nil {
		return fmt.Errorf("product is missing identity or selection")
	}
	if err := validateProvenanceProto(product.GetFieldProvenance(), "field_provenance"); err != nil {
		return err
	}
	if err := validateSelectionEnums(product.GetSelection(), "product "+product.GetId()); err != nil {
		return err
	}
	lastTool := ""
	var tools []*schemacachepb.ToolSpec
	if product.GetTools() != nil {
		tools = product.Tools.Items
	}
	if len(tools) > maxSchemaTools {
		return fmt.Errorf("product has %d tools, limit is %d", len(tools), maxSchemaTools)
	}
	for i, tool := range tools {
		if tool == nil || tool.GetIdentity() == nil || tool.GetConstraints() == nil || tool.GetSafety() == nil || tool.GetInterface() == nil || tool.GetSelection() == nil {
			return fmt.Errorf("tool %d is missing a required typed message", i)
		}
		canonical := tool.Identity.GetCanonicalPath()
		if i > 0 && canonical <= lastTool {
			return fmt.Errorf("tools are duplicate or unsorted at %q", canonical)
		}
		lastTool = canonical
		lastParameter := ""
		var parameters []*schemacachepb.ParameterSpec
		if tool.GetParameters() != nil {
			parameters = tool.Parameters.Items
		}
		if len(parameters) > maxSchemaParameters {
			return fmt.Errorf("tool %q has %d parameters, limit is %d", canonical, len(parameters), maxSchemaParameters)
		}
		for j, parameter := range parameters {
			if parameter == nil {
				return fmt.Errorf("tool %q parameter %d is nil", canonical, j)
			}
			if j > 0 && parameter.GetName() <= lastParameter {
				return fmt.Errorf("tool %q parameters are duplicate or unsorted at %q", canonical, parameter.GetName())
			}
			lastParameter = parameter.GetName()
			for name, raw := range map[string]*schemacachepb.BytesValue{
				"default": parameter.GetDefaultValue(), "interface_default": parameter.GetInterfaceDefault(), "example": parameter.GetExample(),
			} {
				if !validRawValue(raw) {
					return fmt.Errorf("tool %s parameter %s %s is invalid JSON", canonical, parameter.GetName(), name)
				}
			}
			if err := validateProvenanceProto(parameter.GetFieldProvenance(), "tool "+canonical+" parameter "+parameter.GetName()+" provenance"); err != nil {
				return err
			}
		}
		if result := tool.GetResult(); result != nil {
			if result.GetOutcomes() == nil || result.GetDataSchema() == nil {
				return fmt.Errorf("tool %q result is missing outcomes or data_schema", canonical)
			}
			for _, outcome := range result.Outcomes.Items {
				if outcome == schemacachepb.ResultOutcome_RESULT_OUTCOME_UNSPECIFIED {
					return fmt.Errorf("tool %q result contains unspecified outcome", canonical)
				}
			}
			if !validRawValue(result.GetDataSchema()) {
				return fmt.Errorf("tool %s result data_schema is invalid JSON", canonical)
			}
		}
		if err := validateSelectionEnums(tool.GetSelection(), "tool "+canonical); err != nil {
			return err
		}
		if err := validateProvenanceProto(tool.GetFieldProvenance(), "tool "+canonical+" provenance"); err != nil {
			return err
		}
	}
	return nil
}

func validateSelectionEnums(selection *schemacachepb.Selection, path string) error {
	if selection.GetExampleDispositions() == nil {
		return nil
	}
	for _, disposition := range selection.ExampleDispositions.Items {
		if disposition.GetMode() == schemacachepb.ExampleDispositionMode_EXAMPLE_DISPOSITION_MODE_UNSPECIFIED || disposition.GetReasonCode() == schemacachepb.ExampleDispositionReasonCode_EXAMPLE_DISPOSITION_REASON_CODE_UNSPECIFIED {
			return fmt.Errorf("%s contains unspecified example disposition enum", path)
		}
	}
	return nil
}

func validateProvenanceProto(in *schemacachepb.ProvenanceList, path string) error {
	if in == nil {
		return nil
	}
	if len(in.Items) > maxSchemaProvenance {
		return fmt.Errorf("%s has %d entries, limit is %d", path, len(in.Items), maxSchemaProvenance)
	}
	last := ""
	for i, entry := range in.Items {
		if entry == nil || entry.GetValue() == nil || entry.GetKey() == "" || (i > 0 && entry.GetKey() <= last) {
			return fmt.Errorf("%s keys are empty, duplicate, or unsorted at %q", path, entry.GetKey())
		}
		if !validRawValue(entry.Value.GetValue()) {
			return fmt.Errorf("%s.%s.value is invalid JSON", path, entry.GetKey())
		}
		for _, candidates := range []*schemacachepb.CandidateList{entry.Value.GetCandidates(), entry.Value.GetOverriddenCandidates()} {
			if candidates == nil {
				continue
			}
			if len(candidates.Items) > maxSchemaCandidates {
				return fmt.Errorf("%s.%s has too many candidates", path, entry.GetKey())
			}
			for candidateIndex, candidate := range candidates.Items {
				if candidate == nil {
					return fmt.Errorf("%s.%s candidate %d is nil", path, entry.GetKey(), candidateIndex)
				}
				if !validRawValue(candidate.GetValue()) {
					return fmt.Errorf("%s.%s candidate %d value is invalid JSON", path, entry.GetKey(), candidateIndex)
				}
			}
		}
		last = entry.GetKey()
	}
	return nil
}

// Keep diagnostics on the failure path: a valid shard contains thousands of
// provenance values, each of which previously allocated its error location.
func validRawValue(in *schemacachepb.BytesValue) bool {
	return in == nil || len(in.Value) == 0 || json.Valid(in.Value)
}

func sortRegistryExact(in SchemaRegistry) SchemaRegistry {
	out := in
	out.AgentMetadata = cloneBytes(in.AgentMetadata)
	out.Products = cloneSlice(in.Products)
	for i := range out.Products {
		out.Products[i] = cloneProductExact(out.Products[i])
	}
	sort.SliceStable(out.Products, func(i, j int) bool { return out.Products[i].ID < out.Products[j].ID })
	return out
}

func cloneSlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func cloneBytes[T ~[]byte](in T) T {
	if in == nil {
		return nil
	}
	out := make(T, len(in))
	copy(out, in)
	return out
}
