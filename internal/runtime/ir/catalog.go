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

package ir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

type Catalog struct {
	Products []CanonicalProduct `json:"products"`
}

type LifecycleInfo struct {
	DeprecatedBy        int    `json:"deprecated_by,omitempty"`
	DeprecationDate     string `json:"deprecation_date,omitempty"`
	MigrationURL        string `json:"migration_url,omitempty"`
	DeprecatedCandidate bool   `json:"deprecated_candidate,omitempty"`
}

// CLIGroupDef defines a sub-command group within a product.
type CLIGroupDef struct {
	Description string `json:"description"`
}

type ProductCLIMetadata struct {
	Command  string                 `json:"command,omitempty"`
	Aliases  []string               `json:"aliases,omitempty"`
	Prefixes []string               `json:"prefixes,omitempty"`
	Group    string                 `json:"group,omitempty"`
	Hidden   bool                   `json:"hidden,omitempty"`
	Skip     bool                   `json:"skip,omitempty"`
	Groups   map[string]CLIGroupDef `json:"groups,omitempty"`
}

type CLIFlagHint struct {
	Shorthand string `json:"shorthand,omitempty"`
	Alias     string `json:"alias,omitempty"`
	// Transform carries the data-transformation rule from _meta.cli.toolOverrides[].flags[].transform.
	// Known values: "csv_to_array" (comma-separated string → []string), "json_parse" (JSON string → object/array).
	Transform     string         `json:"transform,omitempty"`
	TransformArgs map[string]any `json:"transform_args,omitempty"`
	EnvDefault    string         `json:"env_default,omitempty"`
	Default       any            `json:"default,omitempty"`
	Hidden        bool           `json:"hidden,omitempty"`
	Required      bool           `json:"required,omitempty"`
}

type CanonicalProduct struct {
	ID                        string              `json:"id"`
	DisplayName               string              `json:"display_name"`
	Description               string              `json:"description,omitempty"`
	ServiceDescription        string              `json:"service_description,omitempty"`
	ServerKey                 string              `json:"server_key"`
	Endpoint                  string              `json:"endpoint"`
	SchemaURI                 string              `json:"schema_uri,omitempty"`
	NegotiatedProtocolVersion string              `json:"negotiated_protocol_version,omitempty"`
	Source                    string              `json:"source,omitempty"`
	Degraded                  bool                `json:"degraded"`
	Lifecycle                 *LifecycleInfo      `json:"lifecycle,omitempty"`
	CLI                       *ProductCLIMetadata `json:"cli,omitempty"`
	Tools                     []ToolDescriptor    `json:"tools"`
}

type ToolDescriptor struct {
	RPCName      string         `json:"rpc_name"`
	CLIName      string         `json:"cli_name,omitempty"`
	Aliases      []string       `json:"aliases,omitempty"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	Sensitive    bool           `json:"sensitive"`
	Hidden       bool           `json:"hidden,omitempty"`
	// Group is the sub-command group this tool belongs to within its product.
	// Populated from _meta.cli.toolOverrides[].group or _meta.cli.tools[].group.
	Group           string                 `json:"group,omitempty"`
	FlagHints       map[string]CLIFlagHint `json:"flag_hints,omitempty"`
	SourceServerKey string                 `json:"source_server_key"`
	CanonicalPath   string                 `json:"canonical_path"`
}

func BuildCatalog(runtimeServers []discovery.RuntimeServer) Catalog {
	sorted := append([]discovery.RuntimeServer(nil), runtimeServers...)
	sort.Slice(sorted, func(i, j int) bool {
		left := sorted[i].Server
		right := sorted[j].Server
		if left.DisplayName != right.DisplayName {
			return left.DisplayName < right.DisplayName
		}
		if left.Endpoint != right.Endpoint {
			return left.Endpoint < right.Endpoint
		}
		return left.Key < right.Key
	})

	usedIDs := make(map[string]struct{}, len(sorted))
	products := make([]CanonicalProduct, 0, len(sorted))
	for _, runtimeServer := range sorted {
		productID, idErr := nextCanonicalProductID(
			runtimeServer.Server.CLI.ID,
			runtimeServer.Server.Key,
			usedIDs,
		)
		if idErr != nil {
			// Strict mode: every server must declare _meta.cli.id.
			// Skip servers that violate this requirement and log the error.
			continue
		}
		toolMetaByName := collectToolCLIMetadata(runtimeServer.Server.CLI)
		declaredTools := declaredCLITools(runtimeServer.Server.CLI)

		runtimeTools := append([]transportTool(nil), toTransportTools(runtimeServer.Tools)...)
		if runtimeServer.Degraded {
			runtimeTools = appendSynthesizedTools(runtimeTools, toolMetaByName)
		}

		tools := make([]ToolDescriptor, 0, len(runtimeTools))
		for _, tool := range runtimeTools {
			meta := toolMetaByName[tool.Name]
			title := strings.TrimSpace(tool.Title)
			if override := strings.TrimSpace(meta.Title); override != "" {
				title = override
			}
			if title == "" {
				title = tool.Name
			}
			description := strings.TrimSpace(tool.Description)
			if override := strings.TrimSpace(meta.Description); override != "" {
				description = override
			}
			if description == "" {
				description = title
			}
			sensitive := tool.Sensitive
			if meta.HasSensitive {
				sensitive = meta.Sensitive
			}
			hidden := false
			if meta.HasHidden {
				hidden = meta.Hidden
			}
			if _, declared := declaredTools[tool.Name]; len(declaredTools) > 0 && !declared {
				hidden = true
			}
			cliName := strings.TrimSpace(meta.CLIName)
			if cliName == "" {
				cliName = tool.Name
			}
			group := strings.TrimSpace(meta.Group)
			if group == "" {
				group = inferMissingToolGroup(productID, tool.Name, cliName, runtimeServer.Server.CLI)
			}
			inputSchema := cloneMap(tool.InputSchema)
			rawFlagHints := cloneFlagHints(meta.FlagHints)
			flagHints := mergeFlagHints(cloneFlagHints(rawFlagHints), inputSchema)
			if meta.HasFlagContract {
				inputSchema, flagHints = projectToolInputSchema(inputSchema, rawFlagHints, flagHints)
			}
			flagHints = mergeFlagHints(flagHints, inputSchema)
			tools = append(tools, ToolDescriptor{
				RPCName:         tool.Name,
				CLIName:         cliName,
				Aliases:         routeAliases(cliName, tool.Name, meta.LegacyCLIName),
				Title:           title,
				Description:     description,
				InputSchema:     inputSchema,
				OutputSchema:    cloneMap(tool.OutputSchema),
				Sensitive:       sensitive,
				Hidden:          hidden,
				Group:           group,
				FlagHints:       flagHints,
				SourceServerKey: runtimeServer.Server.Key,
				CanonicalPath:   fmt.Sprintf("%s.%s", productID, tool.Name),
			})
		}
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].RPCName < tools[j].RPCName
		})

		lifecycle := LifecycleInfo{
			DeprecatedBy:        runtimeServer.Server.Lifecycle.DeprecatedBy,
			DeprecationDate:     strings.TrimSpace(runtimeServer.Server.Lifecycle.DeprecationDate),
			MigrationURL:        strings.TrimSpace(runtimeServer.Server.Lifecycle.MigrationURL),
			DeprecatedCandidate: runtimeServer.Server.Lifecycle.DeprecatedCandidate,
		}
		var lifecyclePtr *LifecycleInfo
		if lifecycle.DeprecatedBy > 0 || lifecycle.DeprecationDate != "" || lifecycle.MigrationURL != "" || lifecycle.DeprecatedCandidate {
			lifecycleCopy := lifecycle
			lifecyclePtr = &lifecycleCopy
		}
		cliMeta := ProductCLIMetadata{
			Command:  strings.TrimSpace(runtimeServer.Server.CLI.Command),
			Aliases:  productRouteAliases(strings.TrimSpace(runtimeServer.Server.CLI.Command), productID, runtimeServer.Server.CLI.Aliases),
			Prefixes: trimStrings(runtimeServer.Server.CLI.Prefixes),
			Group:    strings.TrimSpace(runtimeServer.Server.CLI.Group),
			Hidden:   runtimeServer.Server.CLI.Hidden,
			Skip:     runtimeServer.Server.CLI.Skip,
		}
		if len(runtimeServer.Server.CLI.Groups) > 0 {
			groups := make(map[string]CLIGroupDef, len(runtimeServer.Server.CLI.Groups))
			for name, def := range runtimeServer.Server.CLI.Groups {
				groups[name] = CLIGroupDef{Description: def.Description}
			}
			cliMeta.Groups = groups
		}
		var cliPtr *ProductCLIMetadata
		if cliMeta.Command != "" || len(cliMeta.Aliases) > 0 || len(cliMeta.Prefixes) > 0 || cliMeta.Group != "" || cliMeta.Hidden || cliMeta.Skip || len(cliMeta.Groups) > 0 {
			cliCopy := cliMeta
			cliPtr = &cliCopy
		}

		serviceDescription := strings.TrimSpace(runtimeServer.Server.Description)
		description := serviceDescription
		if cliDescription := strings.TrimSpace(runtimeServer.Server.CLI.Description); cliDescription != "" {
			description = cliDescription
		}

		products = append(products, CanonicalProduct{
			ID:                        productID,
			DisplayName:               runtimeServer.Server.DisplayName,
			Description:               description,
			ServiceDescription:        serviceDescription,
			ServerKey:                 runtimeServer.Server.Key,
			Endpoint:                  runtimeServer.Server.Endpoint,
			SchemaURI:                 runtimeServer.Server.SchemaURI,
			NegotiatedProtocolVersion: runtimeServer.NegotiatedProtocolVersion,
			Source:                    runtimeServer.Source,
			Degraded:                  runtimeServer.Degraded,
			Lifecycle:                 lifecyclePtr,
			CLI:                       cliPtr,
			Tools:                     tools,
		})
	}

	return Catalog{Products: products}
}

func inferMissingToolGroup(productID, rpcName, cliName string, cli market.CLIOverlay) string {
	if group := inferAITableToolGroup(productID, rpcName, cliName, cli); group != "" {
		return group
	}
	return ""
}

func inferAITableToolGroup(productID, rpcName, cliName string, cli market.CLIOverlay) string {
	if strings.TrimSpace(strings.ToLower(productID)) != "aitable" {
		return ""
	}
	if !cliHasGroupOrPrefix(cli, "table") {
		return ""
	}

	switch strings.TrimSpace(rpcName) {
	case "create_view", "delete_view", "get_views", "update_view", "export_data", "import_data", "prepare_import_upload":
		return "table"
	}
	switch strings.TrimSpace(cliName) {
	case "create_view", "delete_view", "get_views", "update_view", "export_data", "import_data", "prepare_import_upload":
		return "table"
	}
	return ""
}

func cliHasGroupOrPrefix(cli market.CLIOverlay, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if _, ok := cli.Groups[name]; ok {
		return true
	}
	for _, prefix := range cli.Prefixes {
		if strings.TrimSpace(prefix) == name {
			return true
		}
	}
	return false
}

type transportTool struct {
	Name         string
	Title        string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
	Sensitive    bool
}

type toolCLIMetadata struct {
	CLIName         string
	LegacyCLIName   string
	Title           string
	Description     string
	Group           string
	Declared        bool
	HasFlagContract bool
	Hidden          bool
	HasHidden       bool
	Sensitive       bool
	HasSensitive    bool
	FlagHints       map[string]CLIFlagHint
}

func toTransportTools(in []transport.ToolDescriptor) []transportTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]transportTool, 0, len(in))
	for _, tool := range in {
		out = append(out, transportTool{
			Name:         tool.Name,
			Title:        tool.Title,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
			Sensitive:    tool.Sensitive,
		})
	}
	return out
}

func collectToolCLIMetadata(cli market.CLIOverlay) map[string]toolCLIMetadata {
	metaByName := make(map[string]toolCLIMetadata)
	for toolName, override := range cli.ToolOverrides {
		name := strings.TrimSpace(toolName)
		if name == "" {
			continue
		}
		meta := metaByName[name]
		meta.Declared = true
		if cliName := strings.TrimSpace(override.CLIName); cliName != "" {
			meta.CLIName = cliName
			meta.LegacyCLIName = cliName
		}
		if group := strings.TrimSpace(override.Group); group != "" {
			meta.Group = group
		}
		if override.Hidden != nil {
			meta.Hidden = *override.Hidden
			meta.HasHidden = true
		}
		if override.IsSensitive != nil {
			meta.Sensitive = *override.IsSensitive
			meta.HasSensitive = true
		}
		if len(override.Flags) > 0 {
			meta.HasFlagContract = true
		}
		meta.FlagHints = mergeOverrideFlagHints(meta.FlagHints, override.Flags)
		metaByName[name] = meta
	}

	for _, cliTool := range cli.Tools {
		name := strings.TrimSpace(cliTool.Name)
		if name == "" {
			continue
		}
		meta := metaByName[name]
		meta.Declared = true
		if cliName := strings.TrimSpace(cliTool.CLIName); cliName != "" {
			if meta.LegacyCLIName == "" {
				meta.LegacyCLIName = meta.CLIName
			}
			meta.CLIName = cliName
		}
		if title := strings.TrimSpace(cliTool.Title); title != "" {
			meta.Title = title
		}
		if description := strings.TrimSpace(cliTool.Description); description != "" {
			meta.Description = description
		}
		if group := strings.TrimSpace(cliTool.Group); group != "" {
			meta.Group = group
		}
		if cliTool.Hidden != nil {
			meta.Hidden = *cliTool.Hidden
			meta.HasHidden = true
		}
		if cliTool.IsSensitive != nil {
			meta.Sensitive = *cliTool.IsSensitive
			meta.HasSensitive = true
		}
		meta.FlagHints = mergeToolFlagHints(meta.FlagHints, cliTool.Flags)
		metaByName[name] = meta
	}
	return metaByName
}

func declaredCLITools(cli market.CLIOverlay) map[string]struct{} {
	if len(cli.Tools) == 0 && len(cli.ToolOverrides) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(cli.Tools)+len(cli.ToolOverrides))
	for _, tool := range cli.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	for name := range cli.ToolOverrides {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func projectToolInputSchema(inputSchema map[string]any, rawHints map[string]CLIFlagHint, resolvedHints map[string]CLIFlagHint) (map[string]any, map[string]CLIFlagHint) {
	if len(inputSchema) == 0 || len(rawHints) == 0 {
		return inputSchema, resolvedHints
	}

	keys := make([]string, 0, len(rawHints))
	for key := range rawHints {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return inputSchema, resolvedHints
	}

	projected := newProjectedSchemaRoot(inputSchema)
	projectedHints := make(map[string]CLIFlagHint, len(keys))
	for _, key := range keys {
		rawHint := rawHints[key]
		resolvedPath, propertySchema := resolveProjectedProperty(inputSchema, key, rawHint, resolvedHints)
		if resolvedPath == "" || len(propertySchema) == 0 {
			continue
		}
		hint := mergeCLIFlagHint(rawHint, resolvedHints[resolvedPath])
		setProjectedProperty(projected, inputSchema, resolvedPath, propertySchema, hint.Required)
		projectedHints[resolvedPath] = mergeCLIFlagHint(projectedHints[resolvedPath], hint)
	}
	if len(projectedHints) == 0 {
		return inputSchema, resolvedHints
	}
	return projected, projectedHints
}

func newProjectedSchemaRoot(source map[string]any) map[string]any {
	root := cloneSchemaEnvelope(source)
	if len(root) == 0 {
		root = map[string]any{}
	}
	if _, ok := root["type"]; !ok {
		root["type"] = "object"
	}
	root["properties"] = make(map[string]any)
	return root
}

func cloneSchemaEnvelope(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		if key == "properties" || key == "required" {
			continue
		}
		out[key] = cloneSchemaValue(value)
	}
	return out
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneSchemaValue(item)
		}
		return out
	default:
		return typed
	}
}

func resolveProjectedProperty(schema map[string]any, key string, hint CLIFlagHint, resolvedHints map[string]CLIFlagHint) (string, map[string]any) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	if property := schemaAtPath(schema, key); len(property) > 0 {
		return key, property
	}
	if exact := caseInsensitiveSchemaPath(schema, key); exact != "" {
		return exact, schemaAtPath(schema, exact)
	}
	if rebound := siblingLeafSchemaPath(schema, key); rebound != "" {
		return rebound, schemaAtPath(schema, rebound)
	}
	if fallback := trustedResolvedHintPath(key, hint, resolvedHints); fallback != "" {
		return fallback, schemaAtPath(schema, fallback)
	}
	return key, synthesizeProjectedPropertySchema(key, hint)
}

func trustedResolvedHintPath(rawKey string, rawHint CLIFlagHint, resolvedHints map[string]CLIFlagHint) string {
	if len(resolvedHints) == 0 {
		return ""
	}
	var match string
	for candidate, hint := range resolvedHints {
		if candidate == rawKey {
			continue
		}
		if !sameContractHint(rawHint, hint) {
			continue
		}
		if !trustResolvedHintPath(rawKey, rawHint, candidate) {
			continue
		}
		if match != "" {
			return ""
		}
		match = candidate
	}
	return match
}

func sameContractHint(left, right CLIFlagHint) bool {
	return strings.TrimSpace(left.Alias) == strings.TrimSpace(right.Alias) &&
		strings.TrimSpace(left.Shorthand) == strings.TrimSpace(right.Shorthand) &&
		strings.TrimSpace(left.Transform) == strings.TrimSpace(right.Transform) &&
		strings.TrimSpace(left.EnvDefault) == strings.TrimSpace(right.EnvDefault) &&
		left.Hidden == right.Hidden &&
		reflect.DeepEqual(left.TransformArgs, right.TransformArgs) &&
		reflect.DeepEqual(left.Default, right.Default)
}

func trustResolvedHintPath(rawKey string, rawHint CLIFlagHint, candidate string) bool {
	if strings.Contains(rawKey, ".") {
		return true
	}
	candidateTail := semanticLeafToken(lastSchemaPathToken(candidate))
	if candidateTail == "" {
		return false
	}
	if rawTail := semanticLeafToken(lastSchemaPathToken(rawKey)); rawTail != "" && rawTail == candidateTail {
		return true
	}
	if aliasTail := semanticLeafToken(strings.TrimSpace(rawHint.Alias)); aliasTail != "" && aliasTail == candidateTail {
		return true
	}
	return false
}

func schemaAtPath(schema map[string]any, path string) map[string]any {
	current := schema
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		properties, ok := current["properties"].(map[string]any)
		if !ok {
			return nil
		}
		next, ok := properties[part].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return cloneMap(current)
}

func caseInsensitiveSchemaPath(schema map[string]any, key string) string {
	var match string
	for _, candidate := range collectSchemaPropertyPaths(schema, "") {
		if !strings.EqualFold(candidate, key) {
			continue
		}
		if match != "" {
			return ""
		}
		match = candidate
	}
	return match
}

func siblingLeafSchemaPath(schema map[string]any, key string) string {
	parent, leaf := splitSchemaPath(key)
	if leaf == "" {
		return ""
	}
	candidates := directSchemaChildPaths(schema, parent)
	if len(candidates) == 0 {
		return ""
	}
	tail := semanticLeafToken(leaf)
	if tail == "" {
		return ""
	}
	var match string
	for _, candidate := range candidates {
		if semanticLeafToken(lastSchemaPathToken(candidate)) != tail {
			continue
		}
		if match != "" {
			return ""
		}
		match = candidate
	}
	return match
}

func splitSchemaPath(path string) (string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ""
	}
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1]
}

func directSchemaChildPaths(schema map[string]any, parent string) []string {
	current := schema
	parent = strings.TrimSpace(parent)
	if parent != "" {
		current = schemaAtPath(schema, parent)
	}
	if len(current) == 0 {
		return nil
	}
	properties, ok := current["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return nil
	}
	paths := make([]string, 0, len(properties))
	for key := range properties {
		path := strings.TrimSpace(key)
		if parent != "" {
			path = parent + "." + path
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func collectSchemaPropertyPaths(schema map[string]any, prefix string) []string {
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	paths := make([]string, 0, len(keys))
	for _, key := range keys {
		current := strings.TrimSpace(key)
		if prefix != "" {
			current = prefix + "." + current
		}
		paths = append(paths, current)
		child, ok := properties[key].(map[string]any)
		if !ok {
			continue
		}
		paths = append(paths, collectSchemaPropertyPaths(child, current)...)
	}
	return paths
}

func semanticLeafToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	normalized := strings.ReplaceAll(kebabToken(value), "_", "-")
	parts := strings.Split(normalized, "-")
	return parts[len(parts)-1]
}

func kebabToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for index, char := range value {
		if char >= 'A' && char <= 'Z' {
			if index > 0 {
				builder.WriteByte('-')
			}
			builder.WriteByte(byte(char + 32))
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func lastSchemaPathToken(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

func synthesizeProjectedPropertySchema(path string, hint CLIFlagHint) map[string]any {
	switch strings.TrimSpace(hint.Transform) {
	case "json_parse":
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	case "csv_to_array":
		return map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
			},
		}
	case "iso8601_to_millis":
		return map[string]any{
			"type": "string",
		}
	}

	if inferred, ok := inferSchemaFromDefault(hint.Default); ok {
		return inferred
	}
	if looksBooleanProperty(lastSchemaPathToken(path)) {
		return map[string]any{
			"type": "boolean",
		}
	}
	return map[string]any{
		"type": "string",
	}
}

func inferSchemaFromDefault(value any) (map[string]any, bool) {
	switch value.(type) {
	case bool:
		return map[string]any{"type": "boolean"}, true
	case int, int32, int64:
		return map[string]any{"type": "integer"}, true
	case float32, float64:
		return map[string]any{"type": "number"}, true
	case []any, []string:
		return map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "string",
			},
		}, true
	case map[string]any:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}, true
	default:
		return nil, false
	}
}

func looksBooleanProperty(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "is") ||
		strings.HasPrefix(name, "has") ||
		strings.HasPrefix(name, "need") ||
		strings.HasPrefix(name, "enable")
}

func setProjectedProperty(projected map[string]any, source map[string]any, path string, propertySchema map[string]any, required bool) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return
	}

	currentProjected := projected
	currentSource := source
	for _, part := range parts[:len(parts)-1] {
		properties := ensureSchemaProperties(currentProjected)
		nextProjected, ok := properties[part].(map[string]any)
		if !ok {
			nextProjected = newProjectedObjectSchema(schemaChild(currentSource, part))
			properties[part] = nextProjected
		}
		if propertyRequired(currentSource, part) {
			appendRequiredProperty(currentProjected, part)
		}
		currentProjected = nextProjected
		currentSource = schemaChild(currentSource, part)
	}

	leaf := parts[len(parts)-1]
	properties := ensureSchemaProperties(currentProjected)
	properties[leaf] = cloneMap(propertySchema)
	if required || propertyRequired(currentSource, leaf) {
		appendRequiredProperty(currentProjected, leaf)
	}
}

func newProjectedObjectSchema(source map[string]any) map[string]any {
	out := cloneSchemaEnvelope(source)
	if len(out) == 0 {
		out = map[string]any{}
	}
	if _, ok := out["type"]; !ok {
		out["type"] = "object"
	}
	out["properties"] = make(map[string]any)
	return out
}

func ensureSchemaProperties(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if ok && properties != nil {
		return properties
	}
	properties = make(map[string]any)
	schema["properties"] = properties
	return properties
}

func schemaChild(schema map[string]any, name string) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	child, ok := properties[strings.TrimSpace(name)].(map[string]any)
	if !ok {
		return nil
	}
	return child
}

func propertyRequired(schema map[string]any, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(schema) == 0 {
		return false
	}
	required := requiredProperties(schema)
	_, ok := required[name]
	return ok
}

func appendRequiredProperty(schema map[string]any, name string) {
	name = strings.TrimSpace(name)
	if name == "" || schema == nil {
		return
	}
	required := requiredStrings(schema["required"])
	for _, existing := range required {
		if existing == name {
			return
		}
	}
	required = append(required, name)
	schema["required"] = stringsToAnySlice(required)
}

func requiredStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					out = append(out, text)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func stringsToAnySlice(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func mergeOverrideFlagHints(dst map[string]CLIFlagHint, overrides map[string]market.CLIFlagOverride) map[string]CLIFlagHint {
	if len(overrides) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]CLIFlagHint, len(overrides))
	}
	for key, override := range overrides {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		hint := dst[name]
		if alias := strings.TrimSpace(override.Alias); alias != "" {
			hint.Alias = alias
		}
		if transform := strings.TrimSpace(override.Transform); transform != "" {
			hint.Transform = transform
		}
		if len(override.TransformArgs) > 0 {
			hint.TransformArgs = cloneMap(override.TransformArgs)
		}
		if envDefault := strings.TrimSpace(override.EnvDefault); envDefault != "" {
			hint.EnvDefault = envDefault
		}
		if override.Default != nil {
			hint.Default = *override.Default
		}
		if override.Hidden != nil {
			hint.Hidden = *override.Hidden
		}
		dst[name] = hint
	}
	return dst
}

func mergeToolFlagHints(dst map[string]CLIFlagHint, flags map[string]market.CLIFlagHint) map[string]CLIFlagHint {
	if len(flags) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]CLIFlagHint, len(flags))
	}
	for key, flag := range flags {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		hint := dst[name]
		if alias := strings.TrimSpace(flag.Alias); alias != "" {
			hint.Alias = alias
		}
		if shorthand := strings.TrimSpace(flag.Shorthand); shorthand != "" {
			hint.Shorthand = shorthand
		}
		dst[name] = hint
	}
	return dst
}

func mergeFlagHints(base map[string]CLIFlagHint, inputSchema map[string]any) map[string]CLIFlagHint {
	if len(base) == 0 && len(inputSchema) == 0 {
		return nil
	}
	out := cloneFlagHints(base)
	out = reconcileTopLevelFlagHints(out, inputSchema)
	requiredSet := requiredProperties(inputSchema)
	if out == nil && (len(requiredSet) > 0 || len(propertyDefaults(inputSchema)) > 0) {
		out = make(map[string]CLIFlagHint)
	}
	for name := range requiredSet {
		hint := out[name]
		hint.Required = true
		out[name] = hint
	}
	for name, value := range propertyDefaults(inputSchema) {
		hint := out[name]
		if hint.Default == nil {
			hint.Default = value
		}
		out[name] = hint
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func reconcileTopLevelFlagHints(hints map[string]CLIFlagHint, inputSchema map[string]any) map[string]CLIFlagHint {
	if len(hints) == 0 || len(inputSchema) == 0 {
		return hints
	}

	properties := topLevelProperties(inputSchema)
	if len(properties) == 0 {
		return hints
	}

	propertyNames := make([]string, 0, len(properties))
	for name := range properties {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)

	resolved := make(map[string]struct{}, len(propertyNames))
	candidates := make([]flattenedFlagHintCandidate, 0, len(hints))
	candidatesByLeaf := make(map[string][]flattenedFlagHintCandidate)

	hintKeys := make([]string, 0, len(hints))
	for key := range hints {
		hintKeys = append(hintKeys, key)
	}
	sort.Strings(hintKeys)

	for _, key := range hintKeys {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		if _, ok := properties[name]; ok {
			resolved[name] = struct{}{}
			continue
		}
		candidate, ok := newFlattenedFlagHintCandidate(name, properties)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
		candidatesByLeaf[candidate.Leaf] = append(candidatesByLeaf[candidate.Leaf], candidate)
	}

	resolvedCandidateKeys := make(map[string]struct{}, len(candidates))
	for _, propertyName := range propertyNames {
		matches := candidatesByLeaf[propertyName]
		if len(matches) != 1 {
			continue
		}
		candidate := matches[0]
		hints = mergeResolvedFlagHint(hints, propertyName, candidate.Key)
		resolved[propertyName] = struct{}{}
		resolvedCandidateKeys[candidate.Key] = struct{}{}
	}

	var unresolvedCandidates []flattenedFlagHintCandidate
	for _, candidate := range candidates {
		if _, ok := resolvedCandidateKeys[candidate.Key]; ok {
			continue
		}
		unresolvedCandidates = append(unresolvedCandidates, candidate)
	}

	var unresolvedProperties []string
	for _, propertyName := range propertyNames {
		if _, ok := resolved[propertyName]; ok {
			continue
		}
		unresolvedProperties = append(unresolvedProperties, propertyName)
	}

	if len(unresolvedCandidates) == 1 && len(unresolvedProperties) == 1 {
		hints = mergeResolvedFlagHint(hints, unresolvedProperties[0], unresolvedCandidates[0].Key)
	}

	return hints
}

type flattenedFlagHintCandidate struct {
	Key    string
	Prefix string
	Leaf   string
}

func newFlattenedFlagHintCandidate(name string, properties map[string]struct{}) (flattenedFlagHintCandidate, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return flattenedFlagHintCandidate{}, false
	}
	if !strings.Contains(name, ".") {
		return flattenedFlagHintCandidate{Key: name, Leaf: name}, true
	}

	parts := strings.Split(name, ".")
	prefix := strings.TrimSpace(parts[0])
	if prefix == "" {
		return flattenedFlagHintCandidate{}, false
	}
	if _, exists := properties[prefix]; exists {
		return flattenedFlagHintCandidate{}, false
	}

	leaf := strings.TrimSpace(parts[len(parts)-1])
	if leaf == "" {
		return flattenedFlagHintCandidate{}, false
	}
	return flattenedFlagHintCandidate{
		Key:    name,
		Prefix: prefix,
		Leaf:   leaf,
	}, true
}

func mergeResolvedFlagHint(hints map[string]CLIFlagHint, targetKey string, sourceKey string) map[string]CLIFlagHint {
	targetKey = strings.TrimSpace(targetKey)
	sourceKey = strings.TrimSpace(sourceKey)
	if targetKey == "" || sourceKey == "" || targetKey == sourceKey {
		return hints
	}

	source, ok := hints[sourceKey]
	if !ok {
		return hints
	}
	target := hints[targetKey]
	hints[targetKey] = mergeCLIFlagHint(target, source)
	delete(hints, sourceKey)
	return hints
}

func mergeCLIFlagHint(target CLIFlagHint, source CLIFlagHint) CLIFlagHint {
	if target.Alias == "" {
		target.Alias = source.Alias
	}
	if target.Shorthand == "" {
		target.Shorthand = source.Shorthand
	}
	if target.Transform == "" {
		target.Transform = source.Transform
	}
	if len(target.TransformArgs) == 0 && len(source.TransformArgs) > 0 {
		target.TransformArgs = cloneMap(source.TransformArgs)
	}
	if target.EnvDefault == "" {
		target.EnvDefault = source.EnvDefault
	}
	if target.Default == nil {
		target.Default = source.Default
	}
	if !target.Hidden && source.Hidden {
		target.Hidden = true
	}
	if !target.Required && source.Required {
		target.Required = true
	}
	return target
}

func topLevelProperties(schema map[string]any) map[string]struct{} {
	rawProps, ok := schema["properties"].(map[string]any)
	if !ok || len(rawProps) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(rawProps))
	for key := range rawProps {
		name := strings.TrimSpace(key)
		if name == "" || strings.Contains(name, ".") {
			continue
		}
		out[name] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func requiredProperties(schema map[string]any) map[string]struct{} {
	switch raw := schema["required"].(type) {
	case []any:
		if len(raw) == 0 {
			return nil
		}
		out := make(map[string]struct{}, len(raw))
		for _, item := range raw {
			name, _ := item.(string)
			name = strings.TrimSpace(name)
			if name != "" {
				out[name] = struct{}{}
			}
		}
		return out
	case []string:
		if len(raw) == 0 {
			return nil
		}
		out := make(map[string]struct{}, len(raw))
		for _, name := range raw {
			name = strings.TrimSpace(name)
			if name != "" {
				out[name] = struct{}{}
			}
		}
		return out
	default:
		return nil
	}
}

func propertyDefaults(schema map[string]any) map[string]any {
	rawProps, ok := schema["properties"].(map[string]any)
	if !ok || len(rawProps) == 0 {
		return nil
	}
	out := make(map[string]any)
	for key, rawProp := range rawProps {
		prop, ok := rawProp.(map[string]any)
		if !ok {
			continue
		}
		if value, exists := prop["default"]; exists {
			out[strings.TrimSpace(key)] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendSynthesizedTools(existing []transportTool, metaByName map[string]toolCLIMetadata) []transportTool {
	if len(metaByName) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing))
	for _, tool := range existing {
		seen[tool.Name] = struct{}{}
	}
	names := make([]string, 0, len(metaByName))
	for name := range metaByName {
		if _, exists := seen[name]; exists {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := append([]transportTool(nil), existing...)
	for _, name := range names {
		meta := metaByName[name]
		title := meta.Title
		if strings.TrimSpace(title) == "" {
			title = name
		}
		description := meta.Description
		if strings.TrimSpace(description) == "" {
			description = title
		}
		out = append(out, transportTool{
			Name:        name,
			Title:       title,
			Description: description,
			Sensitive:   meta.Sensitive,
		})
	}
	return out
}

func routeAliases(public string, internal string, extras ...string) []string {
	values := make([]string, 0, len(extras)+1)
	if public != "" && internal != "" && public != internal {
		values = append(values, internal)
	}
	values = append(values, extras...)
	return uniqueTrimmedExcluding(public, values...)
}

func productRouteAliases(public string, canonicalID string, extras []string) []string {
	values := make([]string, 0, len(extras)+1)
	if public != "" && canonicalID != "" && public != canonicalID {
		values = append(values, canonicalID)
	}
	values = append(values, extras...)
	return uniqueTrimmedExcluding(public, values...)
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return uniqueTrimmed(values...)
}

func uniqueTrimmed(values ...string) []string {
	return uniqueTrimmedExcluding("", values...)
}

func uniqueTrimmedExcluding(exclude string, values ...string) []string {
	exclude = strings.TrimSpace(exclude)
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || (exclude != "" && trimmed == exclude) {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c Catalog) FindProduct(id string) (CanonicalProduct, bool) {
	for _, product := range c.Products {
		if product.ID == id {
			return product, true
		}
	}
	return CanonicalProduct{}, false
}

func (c Catalog) FindTool(path string) (CanonicalProduct, ToolDescriptor, bool) {
	productID, toolName, ok := strings.Cut(path, ".")
	if !ok || productID == "" || toolName == "" {
		return CanonicalProduct{}, ToolDescriptor{}, false
	}
	product, ok := c.FindProduct(productID)
	if !ok {
		return CanonicalProduct{}, ToolDescriptor{}, false
	}
	tool, ok := product.FindTool(toolName)
	if !ok {
		return CanonicalProduct{}, ToolDescriptor{}, false
	}
	return product, tool, true
}

func (p CanonicalProduct) FindTool(name string) (ToolDescriptor, bool) {
	for _, tool := range p.Tools {
		if tool.RPCName == name {
			return tool, true
		}
	}
	return ToolDescriptor{}, false
}

// nextCanonicalProductID returns the product ID derived strictly from
// _meta.cli.id. If the ID collides with an already-used ID, a short hash
// suffix is appended to disambiguate.
func nextCanonicalProductID(cliID, serverKey string, usedIDs map[string]struct{}) (string, error) {
	base := strings.TrimSpace(cliID)
	if base == "" {
		return "", fmt.Errorf("server %q is missing _meta.cli.id; every MCP server must declare this field", serverKey)
	}
	id := base
	if _, exists := usedIDs[id]; exists {
		id = fmt.Sprintf("%s-%s", base, shortHash(serverKey))
	}
	usedIDs[id] = struct{}{}
	return id, nil
}

// shortHash returns the first 8 hex characters of the SHA-256 hash of value.
func shortHash(value string) string {
	if strings.TrimSpace(value) == "" {
		sum := sha256.Sum256([]byte("dws"))
		return hex.EncodeToString(sum[:])[:8]
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func cloneMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneFlagHints(value map[string]CLIFlagHint) map[string]CLIFlagHint {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]CLIFlagHint, len(value))
	for key, hint := range value {
		hint.TransformArgs = cloneMap(hint.TransformArgs)
		out[key] = hint
	}
	return out
}
