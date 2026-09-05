// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemaruntime

import (
	"strings"
)

const runtimeAssembledSource = "runtime-assembled"

// TrustedHashes are release-envelope values validated by the loading layer.
// The runtime boundary projects them verbatim and does not calculate or verify
// cache hashes.
type TrustedHashes struct {
	CatalogHash string
	SurfaceHash string
}

// QueryProjectors permits a caller's invariant tests to replace a pure summary
// projection. Production callers should use RenderQuery.
type QueryProjectors struct {
	ProductSummary func(ProductSpec) (map[string]any, error)
	ToolSummary    func(ToolSpec) (map[string]any, error)
}

// UnknownPathError identifies a query miss without coupling the runtime to the
// parent CLI's public error framework.
type UnknownPathError struct{ Path string }

func (e UnknownPathError) Error() string {
	return "unknown runtime schema path " + quote(e.Path)
}

// RenderAll projects the complete registry and stamps only trusted hashes.
func RenderAll(registry SchemaRegistry, hashes TrustedHashes) (map[string]any, error) {
	payload, err := registry.ToPayload()
	if err != nil {
		return nil, err
	}
	stampTrustedHashes(payload, hashes)
	return payload, nil
}

// RenderOverview projects the first-hop product view and stamps only trusted hashes.
func RenderOverview(registry SchemaRegistry, hashes TrustedHashes) (map[string]any, error) {
	payload, err := registry.ToOverviewPayload()
	if err != nil {
		return nil, err
	}
	stampTrustedHashes(payload, hashes)
	return payload, nil
}

// RenderCatalog projects the progressive full catalog and stamps only trusted hashes.
func RenderCatalog(registry SchemaRegistry, hashes TrustedHashes) (map[string]any, error) {
	snapshot, err := registry.ToSnapshotPayload()
	if err != nil {
		return nil, err
	}
	stampTrustedHashes(snapshot.Catalog, hashes)
	return snapshot.Catalog, nil
}

// RenderQuery resolves and projects one product, group, or leaf. Query payloads
// intentionally carry no catalog or surface hashes.
func RenderQuery(registry SchemaRegistry, index SchemaIndex, raw string) (map[string]any, error) {
	return RenderQueryWithProjectors(registry, index, raw, QueryProjectors{})
}

// RenderQueryWithProjectors is RenderQuery with optional pure summary seams.
func RenderQueryWithProjectors(registry SchemaRegistry, index SchemaIndex, raw string, projectors QueryProjectors) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if tool, ok := index.ResolveQuery(raw); ok {
		return AliasView(tool, raw).ToPayload()
	}
	tokens := SplitPathTokens(raw)
	if len(tokens) == 1 {
		if product, ok := index.Product(tokens[0]); ok {
			render := projectors.ProductSummary
			if render == nil {
				render = ProductSpec.ToSummaryPayload
			}
			payload, err := render(product)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"kind":    "schema",
				"level":   "product",
				"count":   len(product.Tools),
				"product": payload,
				"source":  sourceOrDefault(registry.Source),
			}, nil
		}
	}
	if len(tokens) > 1 {
		path := strings.Join(tokens, " ")
		if product, ok := index.Product(tokens[0]); ok {
			matched := make([]map[string]any, 0)
			for _, tool := range product.Tools {
				if !ToolUnderGroup(tool, path) {
					continue
				}
				render := projectors.ToolSummary
				if render == nil {
					render = ToolSpec.ToSummaryPayload
				}
				summary, err := render(tool)
				if err != nil {
					return nil, err
				}
				matched = append(matched, summary)
			}
			if len(matched) > 0 {
				return map[string]any{
					"kind":   "schema",
					"level":  "group",
					"path":   path,
					"count":  len(matched),
					"tools":  matched,
					"source": sourceOrDefault(registry.Source),
				}, nil
			}
		}
	}
	return nil, UnknownPathError{Path: raw}
}

// AliasView returns a detached leaf view for an alias query. Only cli_path and
// is_alias may differ from the canonical registry tool.
func AliasView(tool ToolSpec, raw string) ToolSpec {
	normalized := NormalizeQueryCLIPath(raw)
	if normalized == "" || normalized == tool.Identity.CLIPath || normalized == tool.Identity.PrimaryCLIPath {
		return tool
	}
	for _, alias := range tool.Identity.Aliases {
		if NormalizeCLIPath(alias) == normalized {
			tool.Identity.CLIPath = NormalizeCLIPath(alias)
			tool.Identity.IsAlias = true
			return tool
		}
	}
	return tool
}

// ToolUnderGroup reports whether any primary or alias CLI path is below group.
func ToolUnderGroup(tool ToolSpec, group string) bool {
	prefix := NormalizeCLIPath(group) + " "
	paths := append([]string{tool.Identity.CLIPath, tool.Identity.PrimaryCLIPath}, tool.Identity.Aliases...)
	for _, path := range paths {
		if strings.HasPrefix(NormalizeCLIPath(path), prefix) {
			return true
		}
	}
	return false
}

// SplitPathTokens splits a query on dots, slashes, and whitespace.
func SplitPathTokens(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '.' || r == '/' || r == ' ' || r == '\t'
	})
	out := fields[:0]
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			out = append(out, field)
		}
	}
	return out
}

// NormalizeCLIPath normalizes authored space-separated CLI paths.
func NormalizeCLIPath(path string) string {
	parts := strings.Fields(strings.TrimSpace(path))
	if len(parts) > 0 && parts[0] == "dws" {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}

// NormalizeQueryCLIPath accepts historical dot/slash/space query spellings.
func NormalizeQueryCLIPath(path string) string {
	parts := SplitPathTokens(strings.TrimSpace(path))
	if len(parts) > 0 && parts[0] == "dws" {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}

func normalizeSchemaCLIPath(path string) string      { return NormalizeCLIPath(path) }
func normalizeSchemaQueryCLIPath(path string) string { return NormalizeQueryCLIPath(path) }

func stampTrustedHashes(payload map[string]any, hashes TrustedHashes) {
	payload["catalog_hash"] = hashes.CatalogHash
	if hashes.SurfaceHash != "" {
		payload["surface_hash"] = hashes.SurfaceHash
	}
}

// StampTrustedHashes stamps trusted release-envelope hashes onto a payload.
func StampTrustedHashes(payload map[string]any, hashes TrustedHashes) {
	stampTrustedHashes(payload, hashes)
}

func sourceOrDefault(source string) string {
	if source = strings.TrimSpace(source); source != "" {
		return source
	}
	return runtimeAssembledSource
}

func quote(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}
