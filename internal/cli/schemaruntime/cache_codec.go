// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"crypto/sha256"
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
	SchemaCacheDTOVersion = 1
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

// BuiltSchemaCache is one deterministic Meta payload and its concatenated shards.
type BuiltSchemaCache struct {
	Meta             []byte
	ProductShards    []byte
	Descriptors      []ProductDescriptor
	RegistrySHA256   [sha256.Size]byte
	RegistryDataSize uint64
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
	RegistryDataLength    uint64
	RegistryDataSHA256    [sha256.Size]byte
	Hashes                CacheHashes
	commandCountByProduct map[string]int
	locatorCountByProduct map[string]int
}

// DecodedSchemaProduct contains the exact shard conversion and its typed index.
type DecodedSchemaProduct struct {
	Registry SchemaRegistry
	Index    SchemaIndex
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
// The supplied projections must exactly match the validated Registry.
func BuildSchemaCache(registry SchemaRegistry, lookup map[string]CommandMeta, overview SchemaOverview, locators map[string]string, hashes CacheHashes) (BuiltSchemaCache, error) {
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
	wantOverview, err := BuildSchemaOverview(registry)
	if err != nil {
		return BuiltSchemaCache{}, err
	}
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
			DtoVersion: schemacachepb.DTOVersion_DTO_VERSION_V1,
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
	meta := &schemacachepb.SchemaMetaCache{
		DtoVersion:         schemacachepb.DTOVersion_DTO_VERSION_V1,
		Registry:           registryFieldsToProto(registry),
		CommandEntries:     commandLookupToProto(lookup),
		Overview:           overviewToProto(overview),
		Locators:           locatorsToProto(locators),
		ProductDescriptors: descriptorsToProto(result.Descriptors),
		RegistryDataLength: result.RegistryDataSize,
		RegistryDataSha256: cloneBytes(result.RegistrySHA256[:]),
		SourceSha256:       cloneBytes(hashes.SourceSHA256[:]),
		SurfaceSha256:      cloneBytes(hashes.SurfaceSHA256[:]),
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

// MarshalSchemaCacheDeterministic is the only production protobuf encoder.
func MarshalSchemaCacheDeterministic(message proto.Message) ([]byte, error) {
	if message == nil || !message.ProtoReflect().IsValid() {
		return nil, fmt.Errorf("cannot marshal nil Schema cache message")
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(message)
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
	if root.GetDtoVersion() != schemacachepb.DTOVersion_DTO_VERSION_V1 {
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
	if !present || count != len(wantLookup) || !commandMetaSubsetEqual(meta.CommandMetaByPath, wantLookup) {
		return DecodedSchemaProduct{}, fmt.Errorf("product %q CommandMeta entries disagree with shard", descriptor.ProductID)
	}
	wantLocators, err := buildSchemaProductLocatorsUnchecked(registry)
	if err != nil {
		return DecodedSchemaProduct{}, fmt.Errorf("build product %q locators: %w", descriptor.ProductID, err)
	}
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
	index, err := registry.Index()
	if err != nil {
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("validate reconstructed Schema Registry: %w", err)
	}
	return registry, index, nil
}

func validateAndConvertMeta(root *schemacachepb.SchemaMetaCache) (DecodedSchemaMeta, error) {
	if root.GetDtoVersion() != schemacachepb.DTOVersion_DTO_VERSION_V1 {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta DTO version is %d, want %d", root.GetDtoVersion(), SchemaCacheDTOVersion)
	}
	if root.GetRegistry() == nil || root.GetOverview() == nil || root.GetCommandEntries() == nil || root.GetLocators() == nil || root.GetProductDescriptors() == nil {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta is missing a required presence wrapper")
	}
	if root.Overview.GetRegistry() == nil || root.Overview.GetProducts() == nil {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta overview is missing registry fields or products")
	}
	if len(root.CommandEntries.Items) > maxSchemaMetaEntries || len(root.Locators.Items) > maxSchemaMetaEntries || len(root.ProductDescriptors.Items) > maxSchemaProducts || len(root.Overview.Products.Items) > maxSchemaProducts {
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
		Kind:                 root.Registry.GetKind(),
		Level:                root.Registry.GetLevel(),
		Source:               root.Registry.GetSource(),
		AgentMetadata:        bytesFromProto(root.Registry.GetAgentMetadata()),
		CommandMetaByPath:    make(map[string]CommandMeta, len(root.CommandEntries.Items)),
		Overview:             overviewFromProto(root.Overview),
		LocatorProductByPath: make(map[string]string, len(root.Locators.Items)),
		ProductDescriptors:   descriptorsFromProto(root.ProductDescriptors),
		RegistryDataLength:   root.GetRegistryDataLength(),
	}
	copy(result.RegistryDataSHA256[:], root.GetRegistryDataSha256())
	copy(result.Hashes.SourceSHA256[:], root.GetSourceSha256())
	copy(result.Hashes.SurfaceSHA256[:], root.GetSurfaceSha256())

	last := ""
	for i, entry := range root.CommandEntries.Items {
		if entry == nil || entry.GetMeta() == nil || entry.Meta.GetIdentity() == nil || entry.Meta.GetSafety() == nil || entry.Meta.GetSelection() == nil {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta command entry %d is incomplete", i)
		}
		if entry.GetLookupPath() == "" || (i > 0 && entry.GetLookupPath() <= last) {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta command lookup keys are empty, duplicate, or unsorted at %q", entry.GetLookupPath())
		}
		last = entry.GetLookupPath()
		commandMeta := commandMetaFromProto(entry.Meta)
		if commandMeta.Identity.CLIPath == "" || commandMeta.Identity.Canonical == "" || commandMeta.Identity.ProductID == "" {
			return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta command entry %q has incomplete identity", entry.GetLookupPath())
		}
		result.CommandMetaByPath[entry.GetLookupPath()] = commandMeta
	}
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
	if !validMetaAliasExpansion(result.CommandMetaByPath) {
		return DecodedSchemaMeta{}, fmt.Errorf("Schema Meta CommandMeta entries are not an exact primary/alias expansion")
	}

	for path, meta := range result.CommandMetaByPath {
		if !products[meta.Identity.ProductID] || result.LocatorProductByPath[path] != meta.Identity.ProductID {
			return DecodedSchemaMeta{}, fmt.Errorf("CommandMeta %q has inconsistent product locator", path)
		}
		for _, identityPath := range append([]string{meta.Identity.CLIPath, meta.Identity.Canonical}, meta.Identity.Aliases...) {
			if result.LocatorProductByPath[strings.TrimSpace(identityPath)] != meta.Identity.ProductID {
				return DecodedSchemaMeta{}, fmt.Errorf("CommandMeta %q identity locator %q is missing or inconsistent", path, identityPath)
			}
		}
	}
	// Product verification needs an exact subset cardinality and global-key
	// lookup, not another heap copy of every large CommandMeta value.
	result.commandCountByProduct = make(map[string]int, len(products))
	for _, commandMeta := range result.CommandMetaByPath {
		result.commandCountByProduct[commandMeta.Identity.ProductID]++
	}
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
	last := ""
	for i, descriptor := range descriptors {
		if descriptor.ProductID == "" || (i > 0 && descriptor.ProductID <= last) {
			return fmt.Errorf("product descriptors are empty, duplicate, or unsorted at %q", descriptor.ProductID)
		}
		if descriptor.Offset != next {
			return fmt.Errorf("product %q offset %d leaves a gap or overlap after %d", descriptor.ProductID, descriptor.Offset, next)
		}
		if _, _, err := ProductShardBounds(descriptor, total); err != nil {
			return err
		}
		next += descriptor.Length
		last = descriptor.ProductID
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
		return check(value.Registry, value.CommandEntries, value.Overview, value.Locators, value.ProductDescriptors)
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
		return check(value.Meta)
	case *schemacachepb.CommandMeta:
		return check(value.Identity, value.Safety, value.Selection)
	case *schemacachepb.CommandIdentity:
		return check(value.Aliases)
	case *schemacachepb.CommandSelection:
		return check(value.UseWhen, value.AvoidWhen, value.Prerequisites, value.Tips, value.Examples)
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
		*schemacachepb.CommandSafety, *schemacachepb.LocatorEntry, *schemacachepb.Positional, *schemacachepb.DryRun,
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
				if err := validateRawValue(raw, "tool "+canonical+" parameter "+parameter.GetName()+" "+name); err != nil {
					return err
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
			if err := validateRawValue(result.GetDataSchema(), "tool "+canonical+" result data_schema"); err != nil {
				return err
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
		if err := validateRawValue(entry.Value.GetValue(), path+"."+entry.GetKey()+".value"); err != nil {
			return err
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
				if err := validateRawValue(candidate.GetValue(), fmt.Sprintf("%s.%s candidate %d value", path, entry.GetKey(), candidateIndex)); err != nil {
					return err
				}
			}
		}
		last = entry.GetKey()
	}
	return nil
}

func validateRawValue(in *schemacachepb.BytesValue, path string) error {
	if in != nil && len(in.Value) > 0 && !json.Valid(in.Value) {
		return fmt.Errorf("%s is invalid JSON", path)
	}
	return nil
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
