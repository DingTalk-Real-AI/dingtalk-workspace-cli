// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// These wire structs are used only at the JSON boundary. Once decoded, the
// release command keeps and queries SchemaRegistry/SchemaIndex exclusively.
type schemaCatalogWire struct {
	Kind          string              `json:"kind"`
	Level         string              `json:"level"`
	Source        string              `json:"source"`
	Count         int                 `json:"count"`
	ToolCount     int                 `json:"tool_count"`
	Products      []schemaProductWire `json:"products"`
	AgentMetadata json.RawMessage     `json:"agent_metadata"`
}

type schemaProductWire struct {
	ID                 string                              `json:"id"`
	Name               string                              `json:"name"`
	Description        string                              `json:"description"`
	Runtime            bool                                `json:"runtime"`
	ToolCount          int                                 `json:"tool_count"`
	Tools              []schemaToolWire                    `json:"tools"`
	AgentSummary       string                              `json:"agent_summary"`
	AgentSummarySource string                              `json:"agent_summary_source"`
	UseWhen            []string                            `json:"use_when"`
	AvoidWhen          []string                            `json:"avoid_when"`
	SourceRefs         []string                            `json:"agent_source_refs"`
	MetadataSource     string                              `json:"agent_metadata_source"`
	FieldProvenance    map[string]contract.FieldProvenance `json:"field_provenance"`
}

type schemaToolWire struct {
	Name                string                              `json:"name"`
	CLIName             string                              `json:"cli_name"`
	CanonicalPath       string                              `json:"canonical_path"`
	Path                string                              `json:"path"`
	CLIPath             string                              `json:"cli_path"`
	PrimaryCLIPath      string                              `json:"primary_cli_path"`
	Aliases             []string                            `json:"aliases"`
	IsAlias             bool                                `json:"is_alias"`
	Source              string                              `json:"source"`
	ProductID           string                              `json:"product_id"`
	SourceProductID     string                              `json:"source_product_id"`
	Group               string                              `json:"group"`
	Display             string                              `json:"display"`
	Title               string                              `json:"title"`
	Description         string                              `json:"description"`
	MetadataSource      string                              `json:"metadata_source"`
	Parameters          map[string]schemaParamWire          `json:"parameters"`
	HasParameters       bool                                `json:"has_parameters"`
	ParameterCount      int                                 `json:"parameter_count"`
	Constraints         RuntimeSchemaConstraints            `json:"constraints"`
	Positionals         []contract.RuntimeSchemaPositional  `json:"positionals"`
	DryRun              *contract.DryRunSpec                `json:"dry_run"`
	Result              *contract.ResultSpec                `json:"result"`
	Pagination          *contract.PaginationSpec            `json:"pagination"`
	Effect              string                              `json:"effect"`
	EffectSource        string                              `json:"effect_source"`
	Risk                string                              `json:"risk"`
	Confirmation        string                              `json:"confirmation"`
	Idempotency         string                              `json:"idempotency"`
	InterfaceRef        *contract.InterfaceRefSpec          `json:"interface_ref"`
	InterfaceMode       string                              `json:"interface_mode"`
	Availability        string                              `json:"availability"`
	InterfaceReason     string                              `json:"interface_reason"`
	AgentSummary        string                              `json:"agent_summary"`
	AgentSummarySource  string                              `json:"agent_summary_source"`
	UseWhen             []string                            `json:"use_when"`
	AvoidWhen           []string                            `json:"avoid_when"`
	Prerequisites       []string                            `json:"prerequisites"`
	Tips                []string                            `json:"tips"`
	WorkflowRefs        []string                            `json:"workflow_refs"`
	Examples            []string                            `json:"examples"`
	Reviewed            *bool                               `json:"reviewed"`
	SourceRefs          []string                            `json:"agent_source_refs"`
	AgentMetadataSource string                              `json:"agent_metadata_source"`
	FieldProvenance     map[string]contract.FieldProvenance `json:"field_provenance"`
}

type schemaParamWire struct {
	Type                 string                              `json:"type"`
	Description          string                              `json:"description"`
	Property             string                              `json:"property"`
	Required             bool                                `json:"required"`
	CLIRequired          bool                                `json:"cli_required"`
	RequiredWhen         string                              `json:"required_when"`
	Default              json.RawMessage                     `json:"default"`
	InterfaceDefault     json.RawMessage                     `json:"interface_default"`
	Example              json.RawMessage                     `json:"example"`
	Format               string                              `json:"format"`
	Enum                 []string                            `json:"enum"`
	InterfaceDescription string                              `json:"interface_description"`
	InterfaceType        string                              `json:"interface_type"`
	FieldProvenance      map[string]contract.FieldProvenance `json:"field_provenance"`
}

// validateSchemaSnapshotTypedRoundTrip gates the expensive Catalog↔typed
// content equality pass. Production cold start leaves it false: generation
// already proves delivery invariants, and the loader still enforces structure,
// unknown fields, Index(), interface, and provenance. Tests enable it via
// init in schema_snapshot_roundtrip_test.go so loader drift cases stay covered.
var validateSchemaSnapshotTypedRoundTrip = false

func schemaRegistryFromSnapshot(snapshot SchemaCatalogSnapshot) (SchemaRegistry, SchemaIndex, error) {
	catalogData, err := json.Marshal(snapshot.Catalog)
	if err != nil {
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("encode Schema Catalog index: %w", err)
	}
	var catalog schemaCatalogWire
	if err := decodeStrictSchemaJSON(catalogData, &catalog); err != nil {
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("decode typed Schema Catalog index: %w", err)
	}
	tools := make(map[string]schemaToolWire, len(snapshot.Tools))
	for canonical, detail := range snapshot.Tools {
		wire, err := schemaToolWireFromPayload(detail)
		if err != nil {
			return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("decode Schema ToolSpec %s: %w", canonical, err)
		}
		tools[canonical] = wire
	}
	registry, index, err := schemaRegistryFromTyped(catalog, tools)
	if err != nil {
		return SchemaRegistry{}, SchemaIndex{}, err
	}
	if validateSchemaSnapshotTypedRoundTrip {
		if err := validateSnapshotTypedRoundTrip(snapshot, registry); err != nil {
			return SchemaRegistry{}, SchemaIndex{}, err
		}
	}
	return registry, index, nil
}

// schemaRegistryFromTyped builds SchemaRegistry/SchemaIndex from already-decoded
// wire structs. Snapshot decoding uses it after decoding catalog/tools into
// wire types, keeping map[string]any off the typed construction path.
func schemaRegistryFromTyped(catalog schemaCatalogWire, tools map[string]schemaToolWire) (SchemaRegistry, SchemaIndex, error) {
	products := make([]ProductSpec, 0, len(catalog.Products))
	seen := make(map[string]bool, len(tools))
	for _, productWire := range catalog.Products {
		product := ProductSpec{
			ID:              strings.TrimSpace(productWire.ID),
			Name:            productWire.Name,
			Description:     productWire.Description,
			Runtime:         productWire.Runtime,
			FieldProvenance: cloneFieldProvenance(productWire.FieldProvenance),
			Selection: contract.SelectionSpec{
				AgentSummary:       productWire.AgentSummary,
				AgentSummarySource: productWire.AgentSummarySource,
				UseWhen:            productWire.UseWhen,
				AvoidWhen:          productWire.AvoidWhen,
				SourceRefs:         productWire.SourceRefs,
				MetadataSource:     productWire.MetadataSource,
			},
		}
		for _, summary := range productWire.Tools {
			canonical := strings.TrimSpace(summary.CanonicalPath)
			wire, ok := tools[canonical]
			if !ok {
				return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("schema Catalog summary %s has no full ToolSpec", canonical)
			}
			tool, err := schemaToolSpecFromWire(wire)
			if err != nil {
				return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("decode Schema ToolSpec %s: %w", canonical, err)
			}
			if tool.Identity.ProductID != product.ID {
				return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("schema ToolSpec %s belongs to product %s, not %s", canonical, tool.Identity.ProductID, product.ID)
			}
			product.Tools = append(product.Tools, tool)
			seen[canonical] = true
		}
		products = append(products, product)
	}
	if len(seen) != len(tools) {
		missing := make([]string, 0)
		for canonical := range tools {
			if !seen[canonical] {
				missing = append(missing, canonical)
			}
		}
		sort.Strings(missing)
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("schema Catalog full tools absent from typed products: %s", strings.Join(missing, ", "))
	}
	registry := SchemaRegistry{
		Kind:          catalog.Kind,
		Level:         catalog.Level,
		Source:        catalog.Source,
		Products:      products,
		AgentMetadata: catalog.AgentMetadata,
	}
	index, err := registry.Index()
	if err != nil {
		return SchemaRegistry{}, SchemaIndex{}, err
	}
	return index.Registry(), index, nil
}

func schemaToolWireFromPayload(payload map[string]any) (schemaToolWire, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return schemaToolWire{}, err
	}
	var wire schemaToolWire
	if err := decodeStrictSchemaJSON(data, &wire); err != nil {
		return schemaToolWire{}, err
	}
	return wire, nil
}

func schemaToolSpecFromWire(wire schemaToolWire) (ToolSpec, error) {
	parameterNames := make([]string, 0, len(wire.Parameters))
	for name := range wire.Parameters {
		parameterNames = append(parameterNames, name)
	}
	sort.Strings(parameterNames)
	parameters := make([]ParameterSpec, 0, len(parameterNames))
	for _, name := range parameterNames {
		parameter := wire.Parameters[name]
		parameters = append(parameters, ParameterSpec{
			Name:                 name,
			Type:                 parameter.Type,
			Description:          parameter.Description,
			Property:             parameter.Property,
			Required:             parameter.Required,
			CLIRequired:          parameter.CLIRequired,
			RequiredWhen:         parameter.RequiredWhen,
			Default:              parameter.Default,
			InterfaceDefault:     parameter.InterfaceDefault,
			Example:              parameter.Example,
			Format:               parameter.Format,
			Enum:                 parameter.Enum,
			InterfaceDescription: parameter.InterfaceDescription,
			InterfaceType:        parameter.InterfaceType,
			FieldProvenance:      parameter.FieldProvenance,
		})
	}
	return ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: contract.ToolIdentitySpec{
			ProductID:       wire.ProductID,
			SourceProductID: wire.SourceProductID,
			Name:            wire.Name,
			CLIName:         wire.CLIName,
			CanonicalPath:   wire.CanonicalPath,
			Path:            wire.Path,
			CLIPath:         wire.CLIPath,
			PrimaryCLIPath:  wire.PrimaryCLIPath,
			Group:           wire.Group,
			Aliases:         cloneOptionalStrings(wire.Aliases),
			IsAlias:         wire.IsAlias,
			Source:          wire.Source,
		},
		Display:        wire.Display,
		Title:          wire.Title,
		Description:    wire.Description,
		MetadataSource: wire.MetadataSource,
		Parameters:     parameters,
		Constraints:    wire.Constraints,
		Positionals:    wire.Positionals,
		DryRun:         wire.DryRun,
		Result:         wire.Result,
		Pagination:     wire.Pagination,
		Safety: contract.SafetySpec{
			Effect:       wire.Effect,
			EffectSource: wire.EffectSource,
			Risk:         wire.Risk,
			Confirmation: wire.Confirmation,
			Idempotency:  wire.Idempotency,
		},
		Interface: contract.InterfaceSpec{
			Ref:          wire.InterfaceRef,
			Mode:         wire.InterfaceMode,
			Availability: wire.Availability,
			Reason:       wire.InterfaceReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary:       wire.AgentSummary,
			AgentSummarySource: wire.AgentSummarySource,
			UseWhen:            cloneOptionalStrings(wire.UseWhen),
			AvoidWhen:          cloneOptionalStrings(wire.AvoidWhen),
			Prerequisites:      cloneOptionalStrings(wire.Prerequisites),
			Tips:               cloneOptionalStrings(wire.Tips),
			WorkflowRefs:       cloneOptionalStrings(wire.WorkflowRefs),
			Examples:           cloneOptionalStrings(wire.Examples),
			Reviewed:           wire.Reviewed,
			SourceRefs:         cloneOptionalStrings(wire.SourceRefs),
			MetadataSource:     wire.AgentMetadataSource,
		},
		FieldProvenance: wire.FieldProvenance,
	})
}

func decodeStrictSchemaJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func validateSnapshotTypedRoundTrip(snapshot SchemaCatalogSnapshot, registry SchemaRegistry) error {
	rendered, err := registry.ToSnapshotPayload()
	if err != nil {
		return err
	}
	for canonical, expected := range snapshot.Tools {
		actual, ok := rendered.Tools[canonical]
		if !ok {
			return fmt.Errorf("typed Schema snapshot dropped full tool %s", canonical)
		}
		if !schemaJSONEqual(expected, actual) {
			return fmt.Errorf("typed Schema snapshot changed full tool %s", canonical)
		}
	}
	expectedProducts := schemaMapSlice(snapshot.Catalog["products"])
	actualProducts := schemaMapSlice(rendered.Catalog["products"])
	if !schemaJSONEqual(expectedProducts, actualProducts) {
		return fmt.Errorf("typed Schema snapshot changed product/tool summaries")
	}
	if !schemaJSONEqual(snapshot.Catalog, rendered.Catalog) {
		return fmt.Errorf("typed Schema snapshot changed complete Catalog content")
	}
	return nil
}

func schemaJSONEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	// json.Marshal guarantees valid JSON. Equal canonical bytes therefore need
	// no second validation/decode pass; unequal encodings still use the semantic
	// fallback so equivalent RawMessage formatting remains accepted.
	return bytes.Equal(leftJSON, rightJSON) || equalJSONValues(leftJSON, rightJSON)
}
