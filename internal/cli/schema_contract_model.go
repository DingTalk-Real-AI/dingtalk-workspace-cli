// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// Public aliases preserve the original cli API while ownership lives in the
// Cobra-free runtime package.
type SchemaRegistry = schemaruntime.SchemaRegistry
type ProductSpec = schemaruntime.ProductSpec
type ToolSpec = schemaruntime.ToolSpec
type ParameterSpec = schemaruntime.ParameterSpec
type RuntimeToolSpecInput = schemaruntime.RuntimeToolSpecInput
type SchemaIndex = schemaruntime.SchemaIndex
type SchemaSnapshotPayload = schemaruntime.SchemaSnapshotPayload

var (
	SchemaRegistryFromRuntime = schemaruntime.SchemaRegistryFromRuntime
	ToolSpecFromRuntime       = schemaruntime.ToolSpecFromRuntime
)

var requiredToolProvenanceFields = [...]string{
	"canonical_path", "effect", "risk", "confirmation", "idempotency",
	"interface_ref", "interface_mode", "availability", "agent_summary",
}

var conditionalSelectionProvenanceFields = [...]string{
	"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples",
}

var requiredParameterProvenanceFields = [...]string{
	"property", "type", "description", "required", "required_when",
}

var (
	snapshotToolSummary    = ToolSpec.ToSummaryPayload
	snapshotToolPayload    = ToolSpec.ToPayload
	snapshotProductSummary = ProductSpec.ToSummaryPayload
)

func schemaSnapshotPayload(registry SchemaRegistry) (SchemaSnapshotPayload, error) {
	return schemaruntime.ToSnapshotPayloadWithProjectors(registry, schemaruntime.SnapshotProjectors{
		ToolSummary:    snapshotToolSummary,
		ToolPayload:    snapshotToolPayload,
		ProductSummary: snapshotProductSummary,
	})
}

func stableUniqueStrings(values []string) []string { return schemaruntime.StableUniqueStrings(values) }
func sortedUniqueStrings(values []string) []string { return schemaruntime.SortedUniqueStrings(values) }
func cloneOptionalStrings(values []string) []string {
	return schemaruntime.CloneOptionalStrings(values)
}
func cloneFieldProvenance(source map[string]contract.FieldProvenance) map[string]contract.FieldProvenance {
	return schemaruntime.CloneFieldProvenance(source)
}
func rawJSONValue(raw json.RawMessage) (any, error) { return schemaruntime.RawJSONValue(raw) }
func typedJSONValue(value any) (any, error)         { return schemaruntime.TypedJSONValue(value) }
func validateFinalFieldProvenance(owner, field string, provenance contract.FieldProvenance, value any) error {
	return schemaruntime.ValidateFinalFieldProvenance(owner, field, provenance, value)
}
func equalJSONValues(left, right []byte) bool { return schemaruntime.EqualJSONValues(left, right) }
func defaultString(value, fallback string) string {
	return schemaruntime.DefaultString(value, fallback)
}
func toolProvenanceValue(tool ToolSpec, field string) (any, bool) {
	return schemaruntime.ToolProvenanceValue(tool, field)
}
func parameterProvenanceValue(parameter ParameterSpec, field string) (any, bool) {
	return schemaruntime.ParameterProvenanceValue(parameter, field)
}
func productProvenanceValue(product ProductSpec, field string) (any, bool) {
	return schemaruntime.ProductProvenanceValue(product, field)
}
func normalizeToolSpec(tool ToolSpec) ToolSpec { return schemaruntime.NormalizeToolSpec(tool) }

func normalizeSchemaCLIPath(path string) string  { return schemaruntime.NormalizeCLIPath(path) }
func splitSchemaPathTokens(path string) []string { return schemaruntime.SplitPathTokens(path) }
func normalizeSchemaQueryCLIPath(path string) string {
	return schemaruntime.NormalizeQueryCLIPath(path)
}
func schemaToolForResolvedPath(tool ToolSpec, raw string) ToolSpec {
	return schemaruntime.AliasView(tool, raw)
}
func schemaToolUnderGroup(tool ToolSpec, group string) bool {
	return schemaruntime.ToolUnderGroup(tool, group)
}
