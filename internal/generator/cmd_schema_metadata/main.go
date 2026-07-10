// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

type catalog struct {
	Products []product `json:"products"`
}

type product struct {
	ID    string `json:"id"`
	Tools []tool `json:"tools"`
}

type tool struct {
	RPCName     string         `json:"rpc_name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type metadataFile struct {
	Version        int                     `json:"version"`
	Source         string                  `json:"source"`
	SourceRevision string                  `json:"source_revision,omitempty"`
	SourceHash     string                  `json:"source_hash"`
	Coverage       metadataCoverage        `json:"coverage,omitempty"`
	Tools          map[string]toolMetadata `json:"tools"`
}

type metadataCoverage struct {
	SourceServices   int      `json:"source_services,omitempty"`
	SnapshotServices int      `json:"snapshot_services,omitempty"`
	MissingServices  []string `json:"missing_services,omitempty"`
	SourceTools      int      `json:"source_tools,omitempty"`
	SurfaceTools     int      `json:"surface_tools,omitempty"`
	MatchedTools     int      `json:"matched_tools,omitempty"`
	AliasedTools     int      `json:"aliased_tools,omitempty"`
	UnmatchedTools   int      `json:"unmatched_tools,omitempty"`
}

type interfaceRef struct {
	ProductID string `json:"product_id"`
	RPCName   string `json:"rpc_name"`
}

type toolMetadata struct {
	Title        string                   `json:"title,omitempty"`
	Description  string                   `json:"description,omitempty"`
	Parameters   map[string]paramMetadata `json:"parameters,omitempty"`
	InterfaceRef *interfaceRef            `json:"interface_ref,omitempty"`
}

type paramMetadata struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Default     string   `json:"default,omitempty"`
	Format      string   `json:"format,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Required    *bool    `json:"required,omitempty"`
}

func main() {
	var catalogPath string
	var registryPath string
	var toolsDir string
	var surfacePath string
	var hintsDir string
	var sourceRevision string
	var outputPath string
	flag.StringVar(&catalogPath, "catalog", "docs/generated/schema/catalog.json", "Input catalog snapshot path")
	flag.StringVar(&registryPath, "registry", "", "Input cached CLI registry snapshot (takes precedence over --catalog)")
	flag.StringVar(&toolsDir, "tools-dir", "", "Optional cached MCP tools/list snapshot directory (requires --registry)")
	flag.StringVar(&surfacePath, "surface", "", "Optional public command-surface snapshot used to project interface metadata")
	flag.StringVar(&hintsDir, "hints", "", "Optional Agent hint directory containing canonical-to-MCP interface_ref mappings")
	flag.StringVar(&sourceRevision, "source-revision", "", "Pinned source revision recorded in generated metadata")
	flag.StringVar(&outputPath, "output", "internal/cli/schema_mcp_metadata.json", "Output embedded MCP metadata JSON")
	flag.Parse()

	inputPath := catalogPath
	source := "mcp-catalog"
	if strings.TrimSpace(registryPath) != "" {
		inputPath = registryPath
		source = "cli-registry"
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		fail(fmt.Errorf("read %s: %w", source, err))
	}

	var out metadataFile
	if source == "cli-registry" {
		var snapshot cache.RegistrySnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			fail(fmt.Errorf("decode CLI registry: %w", err))
		}
		if strings.TrimSpace(toolsDir) != "" {
			toolSnapshots, err := loadToolSnapshots(toolsDir)
			if err != nil {
				fail(err)
			}
			out = metadataFromRegistryAndTools(snapshot, toolSnapshots)
			out.Coverage = registryCoverage(snapshot, toolSnapshots)
			source = "mcp-tools-list+cli-registry"
		} else {
			out = metadataFromRegistry(snapshot)
			out.Coverage.SourceServices = len(snapshot.Servers)
		}
	} else {
		var cat catalog
		if err := json.Unmarshal(data, &cat); err != nil {
			fail(fmt.Errorf("decode MCP catalog: %w", err))
		}
		out = metadataFromCatalog(cat)
	}
	out.Coverage.SourceTools = len(out.Tools)
	if strings.TrimSpace(surfacePath) != "" {
		surface, err := loadInterfaceSurface(surfacePath)
		if err != nil {
			fail(err)
		}
		refs, err := loadHintInterfaceRefs(hintsDir)
		if err != nil {
			fail(err)
		}
		out = projectMetadataToSurface(out, surface, refs)
	}
	out.Version = 1
	out.Source = source
	out.SourceRevision = strings.TrimSpace(sourceRevision)
	out.SourceHash = metadataHash(out.Tools)

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fail(fmt.Errorf("encode metadata: %w", err))
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fail(fmt.Errorf("create output directory: %w", err))
	}
	if err := os.WriteFile(outputPath, encoded, 0o644); err != nil {
		fail(fmt.Errorf("write metadata: %w", err))
	}
	_, _ = fmt.Fprintf(
		os.Stderr,
		"generated schema interface metadata: output=%s source=%s tools=%d source_tools=%d surface_tools=%d unmatched=%d hash=%s\n",
		outputPath,
		source,
		len(out.Tools),
		out.Coverage.SourceTools,
		out.Coverage.SurfaceTools,
		out.Coverage.UnmatchedTools,
		out.SourceHash,
	)
}

func metadataFromCatalog(cat catalog) metadataFile {
	out := metadataFile{Tools: map[string]toolMetadata{}}
	for _, product := range cat.Products {
		productID := strings.TrimSpace(product.ID)
		if productID == "" {
			continue
		}
		for _, tool := range product.Tools {
			toolName := strings.TrimSpace(tool.RPCName)
			if toolName == "" {
				continue
			}
			meta := toolMetadata{
				Title:       strings.TrimSpace(tool.Title),
				Description: strings.TrimSpace(tool.Description),
				Parameters:  parameterMetadata(tool.InputSchema),
			}
			if meta.Title == "" && meta.Description == "" && len(meta.Parameters) == 0 {
				continue
			}
			out.Tools[productID+"."+toolName] = meta
		}
	}
	return out
}

func metadataFromRegistry(snapshot cache.RegistrySnapshot) metadataFile {
	out := metadataFile{Tools: map[string]toolMetadata{}}
	for _, server := range snapshot.Servers {
		productID := firstNonEmpty(server.CLI.ID, server.CLI.Command)
		for _, tool := range server.CLI.Tools {
			toolName := strings.TrimSpace(tool.Name)
			if productID == "" || toolName == "" || tool.Hidden {
				continue
			}
			mergeMetadata(out.Tools, productID+"."+toolName, toolMetadata{
				Title:       strings.TrimSpace(tool.Title),
				Description: strings.TrimSpace(tool.Description),
			})
		}
		for toolName, override := range server.CLI.ToolOverrides {
			toolName = strings.TrimSpace(toolName)
			canonicalProduct := firstNonEmpty(override.ServerOverride, productID)
			if canonicalProduct == "" || toolName == "" || override.Hidden || strings.TrimSpace(override.RedirectTo) != "" {
				continue
			}
			mergeMetadata(out.Tools, canonicalProduct+"."+toolName, toolMetadata{
				Description: strings.TrimSpace(override.Description),
				Parameters:  registryParameterMetadata(override.Flags),
			})
		}
	}
	return out
}

func metadataFromRegistryAndTools(snapshot cache.RegistrySnapshot, toolSnapshots []cache.ToolsSnapshot) metadataFile {
	out := metadataFromToolSnapshots(snapshot, toolSnapshots)
	registry := metadataFromRegistry(snapshot)
	for key, metadata := range registry.Tools {
		mergeMetadataFallback(out.Tools, key, metadata)
	}
	return out
}

func metadataFromToolSnapshots(snapshot cache.RegistrySnapshot, toolSnapshots []cache.ToolsSnapshot) metadataFile {
	out := metadataFile{Tools: map[string]toolMetadata{}}
	servers := make(map[string]market.ServerDescriptor, len(snapshot.Servers))
	for _, server := range snapshot.Servers {
		if key := strings.TrimSpace(server.Key); key != "" {
			servers[key] = server
		}
	}
	for _, toolSnapshot := range toolSnapshots {
		server, ok := servers[strings.TrimSpace(toolSnapshot.ServerKey)]
		if !ok {
			continue
		}
		productID := firstNonEmpty(server.CLI.ID, server.CLI.Command)
		if productID == "" {
			continue
		}
		for _, tool := range toolSnapshot.Tools {
			toolName := strings.TrimSpace(tool.Name)
			if toolName == "" {
				continue
			}
			mergeMetadata(out.Tools, productID+"."+toolName, toolMetadata{
				Title:       strings.TrimSpace(tool.Title),
				Description: strings.TrimSpace(tool.Description),
				Parameters:  parameterMetadata(tool.InputSchema),
			})
		}
	}
	return out
}

func loadToolSnapshots(dir string) ([]cache.ToolsSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read MCP tools/list snapshot directory: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	snapshots := make([]cache.ToolsSnapshot, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read MCP tools/list snapshot %s: %w", path, err)
		}
		var snapshot cache.ToolsSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, fmt.Errorf("decode MCP tools/list snapshot %s: %w", path, err)
		}
		if strings.TrimSpace(snapshot.ServerKey) == "" {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

type interfaceSurfaceSnapshot struct {
	Version  int                       `json:"version"`
	Products []interfaceSurfaceProduct `json:"products"`
}

type interfaceSurfaceProduct struct {
	ID    string                 `json:"id"`
	Tools []interfaceSurfaceTool `json:"tools"`
}

type interfaceSurfaceTool struct {
	CanonicalPath   string `json:"canonical_path"`
	SourceProductID string `json:"source_product_id,omitempty"`
}

type hintRefSource struct {
	path     string
	priority int
	hint     agentmetadata.HintFile
}

func loadInterfaceSurface(path string) (interfaceSurfaceSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return interfaceSurfaceSnapshot{}, fmt.Errorf("read command surface %s: %w", path, err)
	}
	var snapshot interfaceSurfaceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return interfaceSurfaceSnapshot{}, fmt.Errorf("decode command surface %s: %w", path, err)
	}
	if snapshot.Version != 1 {
		return interfaceSurfaceSnapshot{}, fmt.Errorf("decode command surface %s: unsupported version %d", path, snapshot.Version)
	}
	return snapshot, nil
}

func loadHintInterfaceRefs(root string) (map[string]interfaceRef, error) {
	refs := map[string]interfaceRef{}
	root = strings.TrimSpace(root)
	if root == "" {
		return refs, nil
	}
	paths := []string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk Agent hints %s: %w", root, err)
	}
	sort.Strings(paths)
	sources := make([]hintRefSource, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Agent hint %s: %w", path, err)
		}
		var hint agentmetadata.HintFile
		if err := json.Unmarshal(data, &hint); err != nil {
			return nil, fmt.Errorf("decode Agent hint %s: %w", path, err)
		}
		if hint.Version != agentmetadata.HintFileVersion {
			return nil, fmt.Errorf("decode Agent hint %s: unsupported version %d", path, hint.Version)
		}
		priority := 1
		if strings.EqualFold(strings.TrimSpace(hint.Source.Kind), "imported") {
			priority = 0
		}
		sources = append(sources, hintRefSource{path: path, priority: priority, hint: hint})
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].priority != sources[j].priority {
			return sources[i].priority < sources[j].priority
		}
		return sources[i].path < sources[j].path
	})
	priorities := map[string]int{}
	for _, source := range sources {
		toolPaths := make([]string, 0, len(source.hint.Tools))
		for path := range source.hint.Tools {
			toolPaths = append(toolPaths, path)
		}
		sort.Strings(toolPaths)
		for _, rawPath := range toolPaths {
			canonicalPath := strings.TrimSpace(rawPath)
			incoming := source.hint.Tools[rawPath].InterfaceRef
			if canonicalPath == "" || incoming == nil {
				continue
			}
			ref := interfaceRef{
				ProductID: strings.TrimSpace(incoming.ProductID),
				RPCName:   strings.TrimSpace(incoming.RPCName),
			}
			if ref.ProductID == "" || ref.RPCName == "" {
				return nil, fmt.Errorf("decode Agent hint %s: incomplete interface_ref for %s", source.path, canonicalPath)
			}
			if existing, ok := refs[canonicalPath]; ok && existing != ref && priorities[canonicalPath] == source.priority {
				return nil, fmt.Errorf("conflicting interface_ref for %s in %s", canonicalPath, source.path)
			}
			refs[canonicalPath] = ref
			priorities[canonicalPath] = source.priority
		}
	}
	return refs, nil
}

func registryCoverage(snapshot cache.RegistrySnapshot, toolSnapshots []cache.ToolsSnapshot) metadataCoverage {
	coverage := metadataCoverage{SourceServices: len(snapshot.Servers)}
	snapshots := map[string]bool{}
	for _, toolSnapshot := range toolSnapshots {
		if key := strings.TrimSpace(toolSnapshot.ServerKey); key != "" {
			snapshots[key] = true
		}
	}
	coverage.SnapshotServices = len(snapshots)
	missing := []string{}
	for _, server := range snapshot.Servers {
		key := strings.TrimSpace(server.Key)
		if key == "" {
			key = firstNonEmpty(server.CLI.ID, server.CLI.Command)
		}
		if key != "" && !snapshots[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	coverage.MissingServices = missing
	return coverage
}

func projectMetadataToSurface(source metadataFile, surface interfaceSurfaceSnapshot, refs map[string]interfaceRef) metadataFile {
	projected := make(map[string]toolMetadata)
	seen := map[string]bool{}
	aliased := 0
	for _, product := range surface.Products {
		productID := strings.TrimSpace(product.ID)
		for _, tool := range product.Tools {
			canonicalPath := strings.TrimSpace(tool.CanonicalPath)
			if canonicalPath == "" || seen[canonicalPath] {
				continue
			}
			seen[canonicalPath] = true
			rpcName := canonicalToolName(canonicalPath)
			candidates := []interfaceRef{}
			if ref, ok := refs[canonicalPath]; ok {
				candidates = append(candidates, ref)
			}
			if sourceProductID := strings.TrimSpace(tool.SourceProductID); sourceProductID != "" && rpcName != "" {
				candidates = append(candidates, interfaceRef{ProductID: sourceProductID, RPCName: rpcName})
			}
			if productID != "" && rpcName != "" {
				candidates = append(candidates, interfaceRef{ProductID: productID, RPCName: rpcName})
			}
			for _, ref := range uniqueInterfaceRefs(candidates) {
				sourceKey := ref.ProductID + "." + ref.RPCName
				metadata, ok := source.Tools[sourceKey]
				if !ok {
					continue
				}
				refCopy := ref
				metadata.InterfaceRef = &refCopy
				projected[canonicalPath] = metadata
				if sourceKey != canonicalPath {
					aliased++
				}
				break
			}
		}
	}
	source.Tools = projected
	source.Coverage.SurfaceTools = len(seen)
	source.Coverage.MatchedTools = len(projected)
	source.Coverage.AliasedTools = aliased
	source.Coverage.UnmatchedTools = len(seen) - len(projected)
	return source
}

func canonicalToolName(canonicalPath string) string {
	canonicalPath = strings.TrimSpace(canonicalPath)
	index := strings.IndexByte(canonicalPath, '.')
	if index < 0 || index == len(canonicalPath)-1 {
		return ""
	}
	return strings.TrimSpace(canonicalPath[index+1:])
}

func uniqueInterfaceRefs(values []interfaceRef) []interfaceRef {
	out := make([]interfaceRef, 0, len(values))
	seen := map[interfaceRef]bool{}
	for _, value := range values {
		value.ProductID = strings.TrimSpace(value.ProductID)
		value.RPCName = strings.TrimSpace(value.RPCName)
		if value.ProductID == "" || value.RPCName == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func registryParameterMetadata(flags map[string]market.CLIFlagOverride) map[string]paramMetadata {
	out := map[string]paramMetadata{}
	for property, flag := range flags {
		property = strings.TrimSpace(property)
		if property == "" || flag.Hidden || flag.PipelineLocal {
			continue
		}
		meta := paramMetadata{
			Type:        registryFlagType(flag.Type),
			Description: strings.TrimSpace(flag.Description),
			Default:     strings.TrimSpace(flag.Default),
		}
		if flag.Required {
			required := true
			meta.Required = &required
		}
		if meta.Type == "" && meta.Description == "" && meta.Default == "" && meta.Required == nil {
			continue
		}
		out[property] = meta
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func registryFlagType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "int", "int8", "int16", "int32", "int64", "integer":
		return "integer"
	case "bool", "boolean":
		return "boolean"
	case "stringslice", "stringarray", "array":
		return "array"
	case "string":
		return "string"
	default:
		return ""
	}
}

func mergeMetadata(tools map[string]toolMetadata, key string, incoming toolMetadata) {
	if incoming.Title == "" && incoming.Description == "" && len(incoming.Parameters) == 0 {
		return
	}
	existing := tools[key]
	if incoming.Title != "" {
		existing.Title = incoming.Title
	}
	if incoming.Description != "" {
		existing.Description = incoming.Description
	}
	if len(incoming.Parameters) > 0 {
		if existing.Parameters == nil {
			existing.Parameters = map[string]paramMetadata{}
		}
		for property, metadata := range incoming.Parameters {
			existing.Parameters[property] = metadata
		}
	}
	tools[key] = existing
}

func mergeMetadataFallback(tools map[string]toolMetadata, key string, incoming toolMetadata) {
	if incoming.Title == "" && incoming.Description == "" && len(incoming.Parameters) == 0 {
		return
	}
	existing, exists := tools[key]
	if !exists {
		tools[key] = incoming
		return
	}
	if existing.Title == "" {
		existing.Title = incoming.Title
	}
	if existing.Description == "" {
		existing.Description = incoming.Description
	}
	if len(incoming.Parameters) > 0 {
		if existing.Parameters == nil {
			existing.Parameters = map[string]paramMetadata{}
		}
		for property, fallback := range incoming.Parameters {
			current, ok := existing.Parameters[property]
			if !ok {
				existing.Parameters[property] = fallback
				continue
			}
			if current.Type == "" {
				current.Type = fallback.Type
			}
			if current.Description == "" {
				current.Description = fallback.Description
			}
			if current.Default == "" {
				current.Default = fallback.Default
			}
			if current.Format == "" {
				current.Format = fallback.Format
			}
			if len(current.Enum) == 0 {
				current.Enum = fallback.Enum
			}
			if current.Required == nil {
				current.Required = fallback.Required
			}
			existing.Parameters[property] = current
		}
	}
	tools[key] = existing
}

func metadataHash(tools map[string]toolMetadata) string {
	data, _ := json.Marshal(tools)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func parameterMetadata(schema map[string]any) map[string]paramMetadata {
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	required := map[string]bool{}
	for _, raw := range anySlice(schema["required"]) {
		if name, ok := raw.(string); ok && strings.TrimSpace(name) != "" {
			required[strings.TrimSpace(name)] = true
		}
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := map[string]paramMetadata{}
	for _, key := range keys {
		prop, _ := properties[key].(map[string]any)
		if len(prop) == 0 {
			continue
		}
		meta := paramMetadata{
			Type:        stringField(prop, "type"),
			Description: firstStringField(prop, "description", "title"),
			Default:     defaultString(prop["default"]),
			Format:      stringField(prop, "format"),
			Enum:        stringEnum(prop["enum"]),
		}
		if required[key] {
			v := true
			meta.Required = &v
		}
		out[key] = meta
	}
	return out
}

func anySlice(raw any) []any {
	values, _ := raw.([]any)
	return values
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func firstStringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(values, key); value != "" {
			return value
		}
	}
	return ""
}

func defaultString(raw any) string {
	if raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return fmt.Sprint(value)
	}
}

func stringEnum(raw any) []string {
	values := anySlice(raw)
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate-schema-mcp-metadata: %v\n", err)
	os.Exit(1)
}
