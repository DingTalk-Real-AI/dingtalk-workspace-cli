// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const SchemaCatalogSnapshotVersion = 1

// Schema Catalog delivery is single-track: RegisterSchemaSourceRoot →
// ResolveSchemaBuild (see schema_source_root.go). There is no committed
// schema_catalog/ embed and no shard/envelope loading path left in the CLI;
// cmd_schema_catalog owns shard writing and re-merges its own dumps.

// SchemaCatalogSnapshot is the release-stable Agent contract. Catalog holds
// the progressive product/tool index; Tools holds full leaf parameter schemas.
// It intentionally contains no endpoint, credential, or runtime cache data.
type SchemaCatalogSnapshot struct {
	Version     int                       `json:"version"`
	SourceHash  string                    `json:"source_hash"`
	SurfaceHash string                    `json:"surface_hash,omitempty"`
	Catalog     map[string]any            `json:"catalog"`
	Tools       map[string]map[string]any `json:"tools"`
}

// SchemaCatalogBuildOptions carries release-envelope inputs which are checked
// against the effective reviewed CommandRegistry. The command set is not an
// option: visibility is resolved by EffectiveCommandRegistry before assembly,
// and every public command must be delivered.
type SchemaCatalogBuildOptions struct {
	RegistryHash string
}

type loadedSchemaCatalog struct {
	Snapshot SchemaCatalogSnapshot
	Registry SchemaRegistry
	Index    SchemaIndex
}

func registryToSnapshotPayload(registry SchemaRegistry) (SchemaCatalogSnapshot, error) {
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		return SchemaCatalogSnapshot{}, err
	}
	return SchemaCatalogSnapshot{Catalog: payload.Catalog, Tools: payload.Tools}, nil
}

var registryToSnapshotPayloadFn = registryToSnapshotPayload

var (
	buildCatalogValidateParameterBindings = ValidateSchemaParameterBindingDelivery
	buildCatalogValidateDryRun            = ValidateReviewedDryRunCapabilityDelivery
	buildCatalogValidateExamples          = ValidateAgentExampleDelivery
	buildCatalogValidateCompleteness      = validateResolvedRuntimeSchemaCompleteness
	buildCatalogValidateRegistry          = validateSchemaRegistryAgainstCommandRegistry
	buildCatalogValidateInterfaces        = validateSchemaRegistryInterfaces
	buildCatalogValidateAgentMetadata     = validateSchemaRegistryAgentMetadata
	buildCatalogValidateProvenance        = validateFinalSchemaProvenanceCoverage
	buildCatalogValidateDelivery          = ValidateSchemaDeliveryInvariants
	buildCatalogValidateFinalCompleteness = validateResolvedSchemaCatalogDeliveryCompleteness

	loadCatalogValidateInterfaces = validateSchemaRegistryInterfaces
	loadCatalogValidateProvenance = validateFinalSchemaProvenanceCoverage

	renderDeliverySchemaAll      = func(registry SchemaRegistry) (map[string]any, error) { return registry.ToPayload() }
	renderDeliverySchemaOverview = func(registry SchemaRegistry) (map[string]any, error) { return registry.ToOverviewPayload() }
	renderSchemaProductSummary   = func(product ProductSpec) (map[string]any, error) { return product.ToSummaryPayload() }
	renderSchemaToolSummary      = func(tool ToolSpec) (map[string]any, error) { return tool.ToSummaryPayload() }
)

// BuildSchemaCatalogSnapshot renders a deterministic Catalog from one
// resolved source-to-delivery hand-off. It deliberately accepts no Cobra root:
// rebuilding SchemaRegistry or re-deriving identity at this boundary would
// allow generation gates to validate one candidate while publishing another.
func BuildSchemaCatalogSnapshot(resolved ResolvedSchemaBuild, options SchemaCatalogBuildOptions) (SchemaCatalogSnapshot, error) {
	if resolved.root == nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("schema Catalog source was not created by ResolveSchemaBuild")
	}
	registry := resolved.registry
	effectiveCommands := resolved.effective
	registryHash := strings.TrimSpace(options.RegistryHash)
	if registryHash == "" {
		registryHash = effectiveCommands.SourceHash()
	} else if registryHash != effectiveCommands.SourceHash() {
		return SchemaCatalogSnapshot{}, fmt.Errorf("provided Registry hash %q disagrees with effective CommandRegistry %q", registryHash, effectiveCommands.SourceHash())
	}
	if err := buildCatalogValidateParameterBindings(resolved.bound, registry); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Schema parameter binding delivery: %w", err)
	}
	if err := buildCatalogValidateDryRun(registry); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate reviewed dry-run capability delivery: %w", err)
	}
	if _, err := buildCatalogValidateExamples(resolved.bound, registry); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Manual Agent example delivery: %w", err)
	}
	if err := buildCatalogValidateCompleteness(resolved.root, resolved.bound); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate reverse command-tree completeness: %w", err)
	}
	if err := buildCatalogValidateRegistry(registry, effectiveCommands); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate typed Schema registry against reviewed CommandRegistry: %w", err)
	}
	if err := buildCatalogValidateInterfaces(registry); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Schema interface disposition: %w", err)
	}
	if err := buildCatalogValidateAgentMetadata(registry); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Schema Agent metadata set: %w", err)
	}
	if err := buildCatalogValidateProvenance(registry); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Schema provenance: %w", err)
	}
	// Visibility has already selected the complete public set in
	// collectRuntimeSchemaEntriesFromBound. Do not apply a post-assembly
	// allowlist: doing so could silently erase an otherwise valid reviewed
	// manual-only command after the exact-set validation above has passed.
	registry.Source = SchemaSourceRuntimeAssembled
	payload, err := registry.ToSnapshotPayload()
	if err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("serialize typed Schema registry: %w", err)
	}

	snapshot := SchemaCatalogSnapshot{
		Version:     SchemaCatalogSnapshotVersion,
		SurfaceHash: registryHash,
		Catalog:     payload.Catalog,
		Tools:       payload.Tools,
	}
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	if err := buildCatalogValidateDelivery(registry, snapshot); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Schema delivery invariants: %w", err)
	}
	if err := buildCatalogValidateFinalCompleteness(resolved.root, resolved.bound, snapshot); err != nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("validate final Catalog delivery completeness: %w", err)
	}
	return snapshot, nil
}

// decodeSchemaCatalogSnapshot decodes a single-document snapshot and loads it
// through loadSchemaCatalogSnapshot. Delivery validation round-trips generated
// JSON through this function so it cannot pass with data that the shared
// loader would reject.
func decodeSchemaCatalogSnapshot(data []byte) (loadedSchemaCatalog, error) {
	var snapshot SchemaCatalogSnapshot
	if err := decodeStrictSchemaJSON(data, &snapshot); err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("decode Schema Catalog snapshot: %w", err)
	}
	return loadSchemaCatalogSnapshot(snapshot)
}

// loadSchemaCatalogSnapshot constructs the production lookup from an
// arbitrary decoded snapshot. It validates the progressive summaries against
// the full leaf store before publishing any lookup key.
func loadSchemaCatalogSnapshot(snapshot SchemaCatalogSnapshot) (loadedSchemaCatalog, error) {
	if snapshot.Version != SchemaCatalogSnapshotVersion {
		return loadedSchemaCatalog{}, fmt.Errorf("unsupported Schema Catalog snapshot version %d", snapshot.Version)
	}
	if len(snapshot.Catalog) == 0 || len(snapshot.Tools) == 0 {
		return loadedSchemaCatalog{}, fmt.Errorf("schema Catalog snapshot is empty")
	}
	if snapshot.SourceHash == "" {
		return loadedSchemaCatalog{}, fmt.Errorf("schema Catalog snapshot is missing source_hash")
	}
	if snapshot.SourceHash != schemaCatalogSnapshotHash(snapshot) {
		return loadedSchemaCatalog{}, fmt.Errorf("schema Catalog snapshot source_hash does not match its content")
	}
	// Production delivery assembles in memory via assembleSchemaCatalogFromRoot and
	// never round-trips through this decode loader. This hash check applies to
	// serialized snapshots only (CI round-trips, cmd_schema_catalog dumps, and
	// delivery-invariant gates) so tampered catalog_hash cannot be stamped from
	// a forged source_hash.
	registry, index, err := schemaRegistryFromSnapshot(snapshot)
	if err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("load typed Schema registry: %w", err)
	}
	if err := loadCatalogValidateInterfaces(registry); err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("validate final Schema interface disposition: %w", err)
	}
	// The production loader validates delivered provenance exactly as encoded.
	// ToolSpecFromRuntime deliberately does not synthesize candidates or
	// rewrite winners, and this coverage gate applies to every snapshot source.
	if err := loadCatalogValidateProvenance(registry); err != nil {
		return loadedSchemaCatalog{}, fmt.Errorf("validate final Schema provenance: %w", err)
	}
	// Agent metadata is no longer a shipped embed. CI/local cmd_schema_catalog
	// dumps may still inject Agent metadata for validation; production delivery
	// reassembles via ResolveSchemaBuild and must not reopen retired
	// schema_agent_metadata/ artifacts.
	return loadedSchemaCatalog{Snapshot: snapshot, Registry: registry, Index: index}, nil
}

func deliverySchemaAllPayload() (map[string]any, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return nil, err
	}
	return schemaAllPayloadFromLoaded(deliverySchemaCatalog())
}

func deliverySchemaOverviewPayload() (map[string]any, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return nil, err
	}
	return schemaOverviewPayloadFromLoaded(deliverySchemaCatalog())
}

// stampSnapshotHashes writes the protected catalog_hash / surface_hash pair
// onto a delivery payload. Every envelope that exposes snapshot hashes must
// go through this helper so the two keys never drift apart.
func stampSnapshotHashes(payload map[string]any, loaded loadedSchemaCatalog) {
	payload["catalog_hash"] = loaded.Snapshot.SourceHash
	if loaded.Snapshot.SurfaceHash != "" {
		payload["surface_hash"] = loaded.Snapshot.SurfaceHash
	}
}

func schemaAllPayloadFromLoaded(loaded loadedSchemaCatalog) (map[string]any, error) {
	payload, err := renderDeliverySchemaAll(loaded.Registry)
	if err != nil {
		return nil, err
	}
	stampSnapshotHashes(payload, loaded)
	return payload, nil
}

func schemaOverviewPayloadFromLoaded(loaded loadedSchemaCatalog) (map[string]any, error) {
	payload, err := renderDeliverySchemaOverview(loaded.Registry)
	if err != nil {
		return nil, err
	}
	stampSnapshotHashes(payload, loaded)
	return payload, nil
}

// queryDeliverySchemaPayload serves dws schema queries through the production
// delivery loader. Completeness / invariant gates must call
// schemaPayloadFromLoadedCatalog (or the deliverySchemaPayload var alias) with
// an explicit loaded catalog — never this helper — to avoid an init cycle
// through assembleSchemaCatalogFromRoot → BuildSchemaCatalogSnapshot.
func queryDeliverySchemaPayload(args []string) (map[string]any, error) {
	if err := deliverySchemaCatalogError(); err != nil {
		return nil, err
	}
	return schemaPayloadFromLoadedCatalog(deliverySchemaCatalog(), args)
}

// schemaPayloadFromLoadedCatalog is shared by the shipped schema command and
// the final-delivery gate. Keeping lookup and payload rendering on one path
// prevents generation-only validation from accepting an unqueryable snapshot.
func schemaPayloadFromLoadedCatalog(loaded loadedSchemaCatalog, args []string) (map[string]any, error) {
	if len(args) == 0 {
		snapshot, err := loaded.Registry.ToSnapshotPayload()
		if err != nil {
			return nil, err
		}
		payload := snapshot.Catalog
		stampSnapshotHashes(payload, loaded)
		return payload, nil
	}
	raw := strings.TrimSpace(args[0])
	if tool, ok := loaded.Index.ResolveQuery(raw); ok {
		return schemaToolForResolvedPath(tool, raw).ToPayload()
	}
	tokens := splitSchemaPathTokens(raw)
	if len(tokens) == 1 {
		if product, ok := loaded.Index.Product(tokens[0]); ok {
			payload, err := renderSchemaProductSummary(product)
			if err != nil {
				return nil, err
			}
			source := strings.TrimSpace(loaded.Registry.Source)
			if source == "" {
				source = SchemaSourceRuntimeAssembled
			}
			return map[string]any{
				"kind":    "schema",
				"level":   "product",
				"count":   len(product.Tools),
				"product": payload,
				"source":  source,
			}, nil
		}
	}
	if len(tokens) > 1 {
		path := strings.Join(tokens, " ")
		if product, ok := loaded.Index.Product(tokens[0]); ok {
			matched := make([]map[string]any, 0)
			for _, tool := range product.Tools {
				if schemaToolUnderGroup(tool, path) {
					summary, err := renderSchemaToolSummary(tool)
					if err != nil {
						return nil, err
					}
					matched = append(matched, summary)
				}
			}
			if len(matched) > 0 {
				source := strings.TrimSpace(loaded.Registry.Source)
				if source == "" {
					source = SchemaSourceRuntimeAssembled
				}
				return map[string]any{
					"kind":   "schema",
					"level":  "group",
					"path":   path,
					"count":  len(matched),
					"tools":  matched,
					"source": source,
				}, nil
			}
		}
	}
	return nil, apperrors.NewValidation("unknown runtime schema path " + strconvQuote(raw))
}

func schemaCatalogSnapshotHash(snapshot SchemaCatalogSnapshot) string {
	payload := struct {
		Version     int                       `json:"version"`
		SurfaceHash string                    `json:"surface_hash,omitempty"`
		Catalog     map[string]any            `json:"catalog"`
		Tools       map[string]map[string]any `json:"tools"`
	}{snapshot.Version, snapshot.SurfaceHash, snapshot.Catalog, snapshot.Tools}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func schemaMapSlice(value any) []map[string]any {
	switch values := value.(type) {
	case []map[string]any:
		return values
	case []any:
		out := make([]map[string]any, 0, len(values))
		for _, value := range values {
			if item, ok := value.(map[string]any); ok {
				out = append(out, item)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaString(value any) string {
	valueString, _ := value.(string)
	return valueString
}

func schemaStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if item, ok := value.(string); ok {
				out = append(out, item)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptySchemaString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(schemaString(value)); text != "" {
			return text
		}
	}
	return ""
}
