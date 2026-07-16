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
)

// These wire structs are used only at the JSON boundary. Once decoded, the
// release command keeps and queries SchemaRegistry/SchemaIndex exclusively.
type schemaCatalogWire struct {
	Kind              string              `json:"kind"`
	Level             string              `json:"level"`
	Source            string              `json:"source"`
	Count             int                 `json:"count"`
	ToolCount         int                 `json:"tool_count"`
	Products          []schemaProductWire `json:"products"`
	InterfaceMetadata json.RawMessage     `json:"interface_metadata"`
	AgentMetadata     json.RawMessage     `json:"agent_metadata"`
}

type schemaProductWire struct {
	ID                 string                     `json:"id"`
	Name               string                     `json:"name"`
	Description        string                     `json:"description"`
	Runtime            bool                       `json:"runtime"`
	ToolCount          int                        `json:"tool_count"`
	Tools              []schemaToolWire           `json:"tools"`
	AgentSummary       string                     `json:"agent_summary"`
	AgentSummarySource string                     `json:"agent_summary_source"`
	UseWhen            []string                   `json:"use_when"`
	AvoidWhen          []string                   `json:"avoid_when"`
	SourceRefs         []string                   `json:"agent_source_refs"`
	MetadataSource     string                     `json:"agent_metadata_source"`
	FieldProvenance    map[string]FieldProvenance `json:"field_provenance"`
}

type schemaToolWire struct {
	Name                string                     `json:"name"`
	CLIName             string                     `json:"cli_name"`
	CanonicalPath       string                     `json:"canonical_path"`
	Path                string                     `json:"path"`
	CLIPath             string                     `json:"cli_path"`
	PrimaryCLIPath      string                     `json:"primary_cli_path"`
	Aliases             []string                   `json:"aliases"`
	IsAlias             bool                       `json:"is_alias"`
	Source              string                     `json:"source"`
	ProductID           string                     `json:"product_id"`
	SourceProductID     string                     `json:"source_product_id"`
	Group               string                     `json:"group"`
	Display             string                     `json:"display"`
	Title               string                     `json:"title"`
	Description         string                     `json:"description"`
	MetadataSource      string                     `json:"metadata_source"`
	Parameters          map[string]schemaParamWire `json:"parameters"`
	HasParameters       bool                       `json:"has_parameters"`
	ParameterCount      int                        `json:"parameter_count"`
	Constraints         RuntimeSchemaConstraints   `json:"constraints"`
	Positionals         []RuntimeSchemaPositional  `json:"positionals"`
	DryRun              *DryRunSpec                `json:"dry_run"`
	Effect              string                     `json:"effect"`
	EffectSource        string                     `json:"effect_source"`
	Risk                string                     `json:"risk"`
	Confirmation        string                     `json:"confirmation"`
	Idempotency         string                     `json:"idempotency"`
	InterfaceRef        *InterfaceRefSpec          `json:"interface_ref"`
	InterfaceMode       string                     `json:"interface_mode"`
	Availability        string                     `json:"availability"`
	InterfaceReason     string                     `json:"interface_reason"`
	AgentSummary        string                     `json:"agent_summary"`
	AgentSummarySource  string                     `json:"agent_summary_source"`
	UseWhen             []string                   `json:"use_when"`
	AvoidWhen           []string                   `json:"avoid_when"`
	Prerequisites       []string                   `json:"prerequisites"`
	Tips                []string                   `json:"tips"`
	WorkflowRefs        []string                   `json:"workflow_refs"`
	Examples            []string                   `json:"examples"`
	Reviewed            *bool                      `json:"reviewed"`
	SourceRefs          []string                   `json:"agent_source_refs"`
	AgentMetadataSource string                     `json:"agent_metadata_source"`
	FieldProvenance     map[string]FieldProvenance `json:"field_provenance"`
}

type schemaParamWire struct {
	Type                 string                     `json:"type"`
	Description          string                     `json:"description"`
	Property             string                     `json:"property"`
	Required             bool                       `json:"required"`
	CLIRequired          bool                       `json:"cli_required"`
	RequiredWhen         string                     `json:"required_when"`
	Default              json.RawMessage            `json:"default"`
	InterfaceDefault     json.RawMessage            `json:"interface_default"`
	Example              json.RawMessage            `json:"example"`
	Format               string                     `json:"format"`
	Enum                 []string                   `json:"enum"`
	InterfaceDescription string                     `json:"interface_description"`
	InterfaceType        string                     `json:"interface_type"`
	FieldProvenance      map[string]FieldProvenance `json:"field_provenance"`
}

func schemaRegistryFromSnapshot(snapshot SchemaCatalogSnapshot) (SchemaRegistry, SchemaIndex, error) {
	catalogData, err := json.Marshal(snapshot.Catalog)
	if err != nil {
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("encode Schema Catalog index: %w", err)
	}
	var catalog schemaCatalogWire
	if err := decodeStrictSchemaJSON(catalogData, &catalog); err != nil {
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("decode typed Schema Catalog index: %w", err)
	}
	products := make([]ProductSpec, 0, len(catalog.Products))
	seen := make(map[string]bool, len(snapshot.Tools))
	for _, productWire := range catalog.Products {
		product := ProductSpec{
			ID:              strings.TrimSpace(productWire.ID),
			Name:            productWire.Name,
			Description:     productWire.Description,
			Runtime:         productWire.Runtime,
			FieldProvenance: cloneFieldProvenance(productWire.FieldProvenance),
			Selection: SelectionSpec{
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
			detail, ok := snapshot.Tools[canonical]
			if !ok {
				return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("schema Catalog summary %s has no full ToolSpec", canonical)
			}
			tool, err := schemaToolSpecFromPayload(detail)
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
	if len(seen) != len(snapshot.Tools) {
		missing := make([]string, 0)
		for canonical := range snapshot.Tools {
			if !seen[canonical] {
				missing = append(missing, canonical)
			}
		}
		sort.Strings(missing)
		return SchemaRegistry{}, SchemaIndex{}, fmt.Errorf("schema Catalog full tools absent from typed products: %s", strings.Join(missing, ", "))
	}
	registry := SchemaRegistry{
		Kind:              catalog.Kind,
		Level:             catalog.Level,
		Source:            catalog.Source,
		Products:          products,
		InterfaceMetadata: catalog.InterfaceMetadata,
		AgentMetadata:     catalog.AgentMetadata,
	}
	index, err := registry.Index()
	if err != nil {
		return SchemaRegistry{}, SchemaIndex{}, err
	}
	registry = index.Registry()
	if err := validateSnapshotTypedRoundTrip(snapshot, registry); err != nil {
		return SchemaRegistry{}, SchemaIndex{}, err
	}
	return registry, index, nil
}

func schemaToolSpecFromPayload(payload map[string]any) (ToolSpec, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return ToolSpec{}, err
	}
	var wire schemaToolWire
	if err := decodeStrictSchemaJSON(data, &wire); err != nil {
		return ToolSpec{}, err
	}
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
	return toolSpecFromSnapshot(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{
			ProductID:       wire.ProductID,
			SourceProductID: wire.SourceProductID,
			Name:            wire.Name,
			CLIName:         wire.CLIName,
			CanonicalPath:   wire.CanonicalPath,
			Path:            wire.Path,
			CLIPath:         wire.CLIPath,
			PrimaryCLIPath:  wire.PrimaryCLIPath,
			Group:           wire.Group,
			Aliases:         wire.Aliases,
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
		Safety: SafetySpec{
			Effect:       wire.Effect,
			EffectSource: wire.EffectSource,
			Risk:         wire.Risk,
			Confirmation: wire.Confirmation,
			Idempotency:  wire.Idempotency,
		},
		Interface: InterfaceSpec{
			Ref:          wire.InterfaceRef,
			Mode:         wire.InterfaceMode,
			Availability: wire.Availability,
			Reason:       wire.InterfaceReason,
		},
		Selection: SelectionSpec{
			AgentSummary:       wire.AgentSummary,
			AgentSummarySource: wire.AgentSummarySource,
			UseWhen:            wire.UseWhen,
			AvoidWhen:          wire.AvoidWhen,
			Prerequisites:      wire.Prerequisites,
			Tips:               wire.Tips,
			WorkflowRefs:       wire.WorkflowRefs,
			Examples:           wire.Examples,
			Reviewed:           wire.Reviewed,
			SourceRefs:         wire.SourceRefs,
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
