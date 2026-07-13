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
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const SchemaCatalogSnapshotVersion = 1

//go:embed schema_catalog.json
var embeddedSchemaCatalogJSON []byte

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

// SchemaCatalogBuildOptions limits generation to the reviewed public command
// surface. Unknown local/plugin commands are ignored and every allowed path
// must be present in the source Cobra tree.
type SchemaCatalogBuildOptions struct {
	AllowedCanonicalPaths map[string]bool
	SurfaceHash           string
}

// CatalogCommandDefinition is the endpoint-free executable fallback carried
// by the release catalog. Real Go helpers and CLIOverlay commands take
// precedence; these definitions only restore reviewed leaves that disappear
// from a runtime registry response.
type CatalogCommandDefinition struct {
	ProductID       string
	ProductName     string
	SourceProductID string
	Source          string
	ToolName        string
	RPCName         string
	CanonicalPath   string
	CLIPath         string
	Aliases         []string
	Title           string
	Description     string
	Parameters      []CatalogParameterDefinition
	Constraints     RuntimeSchemaConstraints
	Positionals     []RuntimeSchemaPositional
}

type CatalogParameterDefinition struct {
	Name          string
	Property      string
	Type          string
	InterfaceType string
	Description   string
	Default       string
	Format        string
	RequiredWhen  string
	Enum          []string
	Required      bool
}

type loadedSchemaCatalog struct {
	Snapshot SchemaCatalogSnapshot
	Lookup   map[string]string
	Products map[string]map[string]any
}

var (
	runtimeEmbeddedSchemaCatalogOnce sync.Once
	runtimeEmbeddedSchemaCatalog     loadedSchemaCatalog
)

func embeddedSchemaCatalog() loadedSchemaCatalog {
	runtimeEmbeddedSchemaCatalogOnce.Do(func() {
		runtimeEmbeddedSchemaCatalog = loadEmbeddedSchemaCatalog()
	})
	return runtimeEmbeddedSchemaCatalog
}

// BuildSchemaCatalogSnapshot renders a deterministic catalog from the
// executable release command tree after all build-time metadata is embedded.
func BuildSchemaCatalogSnapshot(root *cobra.Command, options SchemaCatalogBuildOptions) (SchemaCatalogSnapshot, error) {
	if root == nil {
		return SchemaCatalogSnapshot{}, fmt.Errorf("schema source root is nil")
	}
	catalog, err := runtimeSchemaPayload(root, nil)
	if err != nil {
		return SchemaCatalogSnapshot{}, err
	}
	catalog = cloneSchemaMap(catalog)
	appendMissingHelperProducts(catalog, helperProductSummaries(root))
	catalog["source"] = "embedded-command-catalog"

	allowed := options.AllowedCanonicalPaths
	seen := map[string]bool{}
	tools := map[string]map[string]any{}
	products := schemaMapSlice(catalog["products"])
	filteredProducts := make([]map[string]any, 0, len(products))
	for _, product := range products {
		filteredTools := make([]map[string]any, 0)
		for _, summary := range schemaMapSlice(product["tools"]) {
			canonical := strings.TrimSpace(schemaString(summary["canonical_path"]))
			if canonical == "" || (len(allowed) > 0 && !allowed[canonical]) {
				continue
			}
			detail, detailErr := schemaDetailFromRoot(root, canonical)
			if detailErr != nil {
				return SchemaCatalogSnapshot{}, fmt.Errorf("render %s: %w", canonical, detailErr)
			}
			filteredTools = append(filteredTools, cloneSchemaMap(summary))
			tools[canonical] = cloneSchemaMap(detail)
			seen[canonical] = true
		}
		if len(filteredTools) == 0 {
			continue
		}
		copyProduct := cloneSchemaMap(product)
		copyProduct["tools"] = filteredTools
		copyProduct["tool_count"] = len(filteredTools)
		filteredProducts = append(filteredProducts, copyProduct)
	}
	if len(allowed) > 0 {
		missing := make([]string, 0)
		for canonical := range allowed {
			if !seen[canonical] {
				missing = append(missing, canonical)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return SchemaCatalogSnapshot{}, fmt.Errorf("command surface contains %d paths absent from source tree: %s", len(missing), strings.Join(missing, ", "))
		}
	}
	catalog["products"] = filteredProducts
	catalog["count"] = len(filteredProducts)
	catalog["tool_count"] = schemaCatalogToolCount(filteredProducts)

	snapshot := SchemaCatalogSnapshot{
		Version:     SchemaCatalogSnapshotVersion,
		SurfaceHash: strings.TrimSpace(options.SurfaceHash),
		Catalog:     catalog,
		Tools:       tools,
	}
	snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)
	return snapshot, nil
}

func schemaDetailFromRoot(root *cobra.Command, canonical string) (map[string]any, error) {
	return runtimeSchemaPayload(root, []string{canonical})
}

func appendMissingHelperProducts(catalog map[string]any, helpers []map[string]any) {
	products := schemaMapSlice(catalog["products"])
	seen := map[string]bool{}
	for _, product := range products {
		seen[schemaString(product["id"])] = true
	}
	for _, product := range helpers {
		if id := schemaString(product["id"]); id != "" && !seen[id] {
			products = append(products, cloneSchemaMap(product))
			seen[id] = true
		}
	}
	catalog["products"] = products
	catalog["count"] = len(products)
	catalog["tool_count"] = schemaCatalogToolCount(products)
}

func loadEmbeddedSchemaCatalog() loadedSchemaCatalog {
	var snapshot SchemaCatalogSnapshot
	if err := json.Unmarshal(embeddedSchemaCatalogJSON, &snapshot); err != nil ||
		snapshot.Version != SchemaCatalogSnapshotVersion || len(snapshot.Catalog) == 0 || len(snapshot.Tools) == 0 {
		return loadedSchemaCatalog{}
	}
	if snapshot.SourceHash == "" || snapshot.SourceHash != schemaCatalogSnapshotHash(snapshot) {
		return loadedSchemaCatalog{}
	}
	loaded := loadedSchemaCatalog{
		Snapshot: snapshot,
		Lookup:   map[string]string{},
		Products: map[string]map[string]any{},
	}
	for _, product := range schemaMapSlice(snapshot.Catalog["products"]) {
		if id := strings.TrimSpace(schemaString(product["id"])); id != "" {
			loaded.Products[id] = product
		}
	}
	for canonical, detail := range snapshot.Tools {
		registerSchemaLookup(loaded.Lookup, canonical, canonical)
		registerSchemaLookup(loaded.Lookup, schemaString(detail["canonical_path"]), canonical)
		registerSchemaLookup(loaded.Lookup, schemaString(detail["path"]), canonical)
		registerSchemaLookup(loaded.Lookup, schemaString(detail["cli_path"]), canonical)
		registerSchemaLookup(loaded.Lookup, schemaString(detail["primary_cli_path"]), canonical)
		for _, alias := range schemaStringSlice(detail["aliases"]) {
			registerSchemaLookup(loaded.Lookup, alias, canonical)
		}
		if sourceProduct := strings.TrimSpace(schemaString(detail["source_product_id"])); sourceProduct != "" {
			registerSchemaLookup(loaded.Lookup, sourceProduct+"."+schemaString(detail["name"]), canonical)
		}
	}
	return loaded
}

func embeddedSchemaCatalogAvailable() bool {
	return len(embeddedSchemaCatalog().Snapshot.Tools) > 0
}

// EmbeddedSchemaCommandDefinitions returns a detached, deterministic view of
// the frozen CLI contract. It never exposes endpoints or Agent-only fields.
func EmbeddedSchemaCommandDefinitions() []CatalogCommandDefinition {
	loaded := embeddedSchemaCatalog()
	if len(loaded.Snapshot.Tools) == 0 {
		return nil
	}
	productNames := map[string]string{}
	for id, product := range loaded.Products {
		productNames[id] = firstNonEmptySchemaString(product["name"], product["description"], id)
	}
	keys := make([]string, 0, len(loaded.Snapshot.Tools))
	for canonical := range loaded.Snapshot.Tools {
		keys = append(keys, canonical)
	}
	sort.Strings(keys)
	out := make([]CatalogCommandDefinition, 0, len(keys))
	for _, canonical := range keys {
		detail := loaded.Snapshot.Tools[canonical]
		definition := CatalogCommandDefinition{
			ProductID:       strings.TrimSpace(schemaString(detail["product_id"])),
			SourceProductID: strings.TrimSpace(schemaString(detail["source_product_id"])),
			Source:          strings.TrimSpace(schemaString(detail["source"])),
			ToolName:        strings.TrimSpace(schemaString(detail["name"])),
			CanonicalPath:   canonical,
			CLIPath:         strings.TrimSpace(schemaString(detail["primary_cli_path"])),
			Aliases:         append([]string(nil), schemaStringSlice(detail["aliases"])...),
			Title:           strings.TrimSpace(schemaString(detail["title"])),
			Description:     strings.TrimSpace(schemaString(detail["description"])),
		}
		if definition.CLIPath == "" {
			definition.CLIPath = strings.TrimSpace(schemaString(detail["cli_path"]))
		}
		if definition.SourceProductID == "" {
			definition.SourceProductID = definition.ProductID
		}
		definition.RPCName = definition.ToolName
		if ref, ok := detail["interface_ref"].(map[string]any); ok {
			definition.SourceProductID = firstNonEmptySchemaString(ref["product_id"], definition.SourceProductID)
			definition.RPCName = firstNonEmptySchemaString(ref["rpc_name"], definition.RPCName)
		}
		definition.ProductName = productNames[definition.ProductID]
		for name, raw := range schemaMap(detail["parameters"]) {
			parameter := CatalogParameterDefinition{
				Name:          name,
				Property:      strings.TrimSpace(schemaString(raw["property"])),
				Type:          strings.TrimSpace(schemaString(raw["type"])),
				InterfaceType: strings.TrimSpace(schemaString(raw["interface_type"])),
				Description:   strings.TrimSpace(schemaString(raw["description"])),
				Default:       schemaScalarString(raw["default"]),
				Format:        strings.TrimSpace(schemaString(raw["format"])),
				RequiredWhen:  strings.TrimSpace(schemaString(raw["required_when"])),
				Enum:          append([]string(nil), schemaStringSlice(raw["enum"])...),
				Required:      schemaBool(raw["required"]),
			}
			if parameter.Property == "" {
				parameter.Property = name
			}
			if parameter.Type == "" {
				parameter.Type = "string"
			}
			definition.Parameters = append(definition.Parameters, parameter)
		}
		sort.Slice(definition.Parameters, func(i, j int) bool { return definition.Parameters[i].Name < definition.Parameters[j].Name })
		decodeSchemaValue(detail["constraints"], &definition.Constraints)
		decodeSchemaValue(detail["positionals"], &definition.Positionals)
		out = append(out, definition)
	}
	return out
}

// EmbeddedSchemaCLIPaths returns the reviewed executable primary and alias
// paths for the current release.
func EmbeddedSchemaCLIPaths() map[string]bool {
	paths := map[string]bool{}
	for _, definition := range EmbeddedSchemaCommandDefinitions() {
		for _, path := range append([]string{definition.CLIPath}, definition.Aliases...) {
			if path = strings.Join(strings.Fields(path), " "); path != "" {
				paths[path] = true
			}
		}
	}
	return paths
}

// EmbeddedSchemaAnnotationReport describes how the frozen release contract
// maps back to the executable Cobra tree. Missing paths indicate a release
// contract drift and are asserted by catalog quality gates.
type EmbeddedSchemaAnnotationReport struct {
	Matched int
	Missing []string
}

// AnnotateEmbeddedSchemaCommands attaches the frozen release contract to the
// executable Cobra leaves. It never creates commands or changes validation;
// catalog fallback commands are registered separately at low priority.
func AnnotateEmbeddedSchemaCommands(root *cobra.Command) EmbeddedSchemaAnnotationReport {
	report := EmbeddedSchemaAnnotationReport{}
	for _, definition := range EmbeddedSchemaCommandDefinitions() {
		cmd := compatibleSchemaCommand(root, definition.CLIPath, definition)
		if cmd == nil {
			for _, alias := range definition.Aliases {
				if cmd = compatibleSchemaCommand(root, alias, definition); cmd != nil {
					break
				}
			}
		}
		if cmd == nil {
			report.Missing = append(report.Missing, definition.CanonicalPath)
			continue
		}
		existingProduct, existingTool, existingSource := runtimeSchemaAnnotations(cmd)
		if existingProduct != "" && existingTool != "" && existingSource != "frozen-catalog" {
			report.Matched++
			continue
		}
		AttachRuntimeSchema(cmd, definition.ProductID, definition.ToolName, definition.Source)
		// Real Cobra commands own their current parameter contract. The embedded
		// Catalog may backfill stable identity only. Parameter bindings come from
		// their own versioned input; required and constraints come from current
		// Cobra metadata and strong annotations. Only generated fallback leaves
		// need the complete frozen contract.
		if existingSource != "frozen-catalog" {
			report.Matched++
			continue
		}
		AnnotateRuntimeToolMetadata(cmd, definition.Title, definition.Description, "embedded-command-catalog")
		AnnotateRuntimeConstraints(cmd, definition.Constraints)
		AnnotateRuntimePositionals(cmd, definition.Positionals...)
		for _, parameter := range definition.Parameters {
			parameterType := firstNonEmptySchemaString(parameter.InterfaceType, parameter.Type, "string")
			AnnotateRuntimeFlag(cmd, parameter.Name, parameter.Property, parameterType, parameter.Required, parameter.Default)
			AnnotateRuntimeFlagRequiredWhen(cmd, parameter.Name, parameter.RequiredWhen)
			AnnotateRuntimeFlagFormat(cmd, parameter.Name, parameter.Format)
			AnnotateRuntimeFlagEnum(cmd, parameter.Name, parameter.Enum...)
		}
		report.Matched++
	}
	sort.Strings(report.Missing)
	return report
}

func compatibleSchemaCommand(root *cobra.Command, path string, definition CatalogCommandDefinition) *cobra.Command {
	cmd := exactSchemaCommand(root, path)
	if cmd == nil {
		return nil
	}
	productID, toolName, _ := runtimeSchemaAnnotations(cmd)
	if productID == "" && toolName == "" {
		return cmd
	}
	if productID == definition.ProductID && toolName == definition.ToolName {
		return cmd
	}
	return nil
}

func exactSchemaCommand(root *cobra.Command, rawPath string) *cobra.Command {
	if root == nil {
		return nil
	}
	parts := strings.Fields(strings.TrimSpace(rawPath))
	if len(parts) > 0 && parts[0] == root.Name() {
		parts = parts[1:]
	}
	current := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	if current == root {
		return nil
	}
	return current
}

func embeddedSchemaPayload(args []string) (map[string]any, error) {
	loaded := embeddedSchemaCatalog()
	if len(args) == 0 {
		payload := cloneSchemaMap(loaded.Snapshot.Catalog)
		payload["catalog_hash"] = loaded.Snapshot.SourceHash
		if loaded.Snapshot.SurfaceHash != "" {
			payload["surface_hash"] = loaded.Snapshot.SurfaceHash
		}
		return payload, nil
	}
	raw := strings.TrimSpace(args[0])
	if canonical := resolveSchemaLookup(loaded.Lookup, raw); canonical != "" {
		detail := cloneSchemaMap(loaded.Snapshot.Tools[canonical])
		if alias := matchingSchemaAlias(detail, raw); alias != "" {
			detail["is_alias"] = true
			detail["cli_path"] = alias
		}
		return detail, nil
	}
	tokens := splitSchemaPathTokens(raw)
	if len(tokens) == 1 {
		if product := loaded.Products[tokens[0]]; product != nil {
			copyProduct := cloneSchemaMap(product)
			return map[string]any{
				"kind":    "schema",
				"level":   "product",
				"count":   schemaProductToolCount(copyProduct),
				"product": copyProduct,
				"source":  "embedded-command-catalog",
			}, nil
		}
	}
	if len(tokens) > 1 {
		path := strings.Join(tokens, " ")
		if product := loaded.Products[tokens[0]]; product != nil {
			matched := make([]map[string]any, 0)
			for _, summary := range schemaMapSlice(product["tools"]) {
				if schemaSummaryUnderGroup(summary, path) {
					matched = append(matched, cloneSchemaMap(summary))
				}
			}
			if len(matched) > 0 {
				return map[string]any{
					"kind":   "schema",
					"level":  "group",
					"path":   path,
					"count":  len(matched),
					"tools":  matched,
					"source": "embedded-command-catalog",
				}, nil
			}
		}
	}
	return nil, apperrors.NewValidation("unknown runtime schema path " + strconvQuote(raw))
}

func matchingSchemaAlias(detail map[string]any, raw string) string {
	normalized := strings.Join(splitSchemaPathTokens(raw), " ")
	for _, alias := range schemaStringSlice(detail["aliases"]) {
		if normalized == strings.Join(splitSchemaPathTokens(alias), " ") {
			return alias
		}
	}
	return ""
}

func schemaSummaryUnderGroup(summary map[string]any, path string) bool {
	prefix := path + " "
	paths := []string{
		schemaString(summary["cli_path"]),
		schemaString(summary["primary_cli_path"]),
	}
	paths = append(paths, schemaStringSlice(summary["aliases"])...)
	for _, candidate := range paths {
		if strings.HasPrefix(strings.Join(splitSchemaPathTokens(candidate), " "), prefix) {
			return true
		}
	}
	return false
}

func registerSchemaLookup(index map[string]string, raw, canonical string) {
	raw = strings.TrimSpace(raw)
	canonical = strings.TrimSpace(canonical)
	if raw == "" || canonical == "" {
		return
	}
	index[raw] = canonical
	index[strings.Join(splitSchemaPathTokens(raw), " ")] = canonical
}

func resolveSchemaLookup(index map[string]string, raw string) string {
	if canonical := index[strings.TrimSpace(raw)]; canonical != "" {
		return canonical
	}
	return index[strings.Join(splitSchemaPathTokens(raw), " ")]
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

func cloneSchemaMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	encoded, _ := json.Marshal(input)
	var output map[string]any
	_ = json.Unmarshal(encoded, &output)
	return output
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

func schemaMap(value any) map[string]map[string]any {
	input, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]map[string]any, len(input))
	for key, value := range input {
		if item, ok := value.(map[string]any); ok {
			out[key] = item
		}
	}
	return out
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

func schemaBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func schemaScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64, bool:
		return fmt.Sprint(typed)
	default:
		return ""
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

func decodeSchemaValue(value any, target any) {
	encoded, err := json.Marshal(value)
	if err == nil {
		_ = json.Unmarshal(encoded, target)
	}
}
