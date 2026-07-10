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

package cli

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:embed schema_mcp_metadata.json
var embeddedMCPMetadataJSON []byte

const (
	runtimeSchemaProductAnnotation = "dws.schema.product"
	runtimeSchemaToolAnnotation    = "dws.schema.tool"
	runtimeSchemaSourceAnnotation  = "dws.schema.source"
	runtimeSchemaTitleAnnotation   = "dws.schema.title"
	runtimeSchemaDescAnnotation    = "dws.schema.description"
	runtimeSchemaMetaAnnotation    = "dws.schema.metadata_source"
	runtimeSchemaExcludeAnnotation = "dws.schema.exclude"
	runtimeSchemaRulesAnnotation   = "dws.schema.constraints"
	runtimeSchemaArgsAnnotation    = "dws.schema.positionals"

	runtimeSchemaFlagPropertyAnnotation = "dws.schema.property"
	runtimeSchemaFlagTypeAnnotation     = "dws.schema.type"
	runtimeSchemaFlagRequiredAnnotation = "dws.schema.required"
	runtimeSchemaFlagDefaultAnnotation  = "dws.schema.default"
	runtimeSchemaFlagExampleAnnotation  = "dws.schema.example"
)

// RuntimeSchemaConstraints describes cross-parameter rules that cannot be
// represented by an individual parameter's required bit.
type RuntimeSchemaConstraints struct {
	MutuallyExclusive [][]string `json:"mutually_exclusive,omitempty"`
	RequireOneOf      [][]string `json:"require_one_of,omitempty"`
	RequireTogether   [][]string `json:"require_together,omitempty"`
}

// RuntimeSchemaPositional describes one ordered CLI argument. Name is also
// used by RuntimeSchemaConstraints when a one-of group mixes flags and args.
type RuntimeSchemaPositional struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Variadic    bool   `json:"variadic,omitempty"`
	Index       int    `json:"index"`
}

type embeddedMCPMetadata struct {
	Version        int                                `json:"version"`
	Source         string                             `json:"source"`
	SourceRevision string                             `json:"source_revision,omitempty"`
	SourceHash     string                             `json:"source_hash"`
	Coverage       embeddedMCPMetadataCoverage        `json:"coverage,omitempty"`
	Tools          map[string]embeddedMCPToolMetadata `json:"tools"`
}

type embeddedMCPMetadataCoverage struct {
	SourceServices   int      `json:"source_services,omitempty"`
	SnapshotServices int      `json:"snapshot_services,omitempty"`
	MissingServices  []string `json:"missing_services,omitempty"`
	SourceTools      int      `json:"source_tools,omitempty"`
	SurfaceTools     int      `json:"surface_tools,omitempty"`
	MatchedTools     int      `json:"matched_tools,omitempty"`
	AliasedTools     int      `json:"aliased_tools,omitempty"`
	UnmatchedTools   int      `json:"unmatched_tools,omitempty"`
}

type embeddedMCPInterfaceRef struct {
	ProductID string `json:"product_id"`
	RPCName   string `json:"rpc_name"`
}

type embeddedMCPToolMetadata struct {
	Title        string                          `json:"title,omitempty"`
	Description  string                          `json:"description,omitempty"`
	Parameters   map[string]embeddedMCPParamMeta `json:"parameters,omitempty"`
	InterfaceRef *embeddedMCPInterfaceRef        `json:"interface_ref,omitempty"`
}

type embeddedMCPParamMeta struct {
	Type         string   `json:"type,omitempty"`
	Description  string   `json:"description,omitempty"`
	Default      string   `json:"default,omitempty"`
	Format       string   `json:"format,omitempty"`
	Enum         []string `json:"enum,omitempty"`
	Required     *bool    `json:"required,omitempty"`
	RequiredWhen string   `json:"required_when,omitempty"`
}

var runtimeEmbeddedMCPMetadata = loadEmbeddedMCPMetadata()

func loadEmbeddedMCPMetadata() embeddedMCPMetadata {
	var metadata embeddedMCPMetadata
	if err := json.Unmarshal(embeddedMCPMetadataJSON, &metadata); err != nil {
		return embeddedMCPMetadata{Tools: map[string]embeddedMCPToolMetadata{}}
	}
	if metadata.Tools == nil {
		metadata.Tools = map[string]embeddedMCPToolMetadata{}
	}
	return metadata
}

func interfaceMetadataSummary() map[string]any {
	metadata := runtimeEmbeddedMCPMetadata
	summary := map[string]any{
		"source":      strings.TrimSpace(metadata.Source),
		"version":     metadata.Version,
		"source_hash": strings.TrimSpace(metadata.SourceHash),
		"tool_count":  len(metadata.Tools),
	}
	if revision := strings.TrimSpace(metadata.SourceRevision); revision != "" {
		summary["source_revision"] = revision
	}
	if metadata.Coverage.SurfaceTools > 0 {
		summary["coverage"] = metadata.Coverage
	}
	return summary
}

// AttachRuntimeSchema marks a runnable command as part of the runtime schema
// surface. `dws schema` scans only commands with these annotations, so the
// schema source is the actual command surface generated from overlays/helpers,
// not the MCP tools/list catalog.
func AttachRuntimeSchema(cmd *cobra.Command, productID, toolName, source string) {
	if cmd == nil {
		return
	}
	productID = strings.TrimSpace(productID)
	toolName = strings.TrimSpace(toolName)
	if productID == "" || toolName == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[runtimeSchemaProductAnnotation] = productID
	cmd.Annotations[runtimeSchemaToolAnnotation] = toolName
	if source = strings.TrimSpace(source); source != "" {
		cmd.Annotations[runtimeSchemaSourceAnnotation] = source
	}
}

// AnnotateRuntimeToolMetadata preserves MCP-provided tool metadata on a Cobra
// leaf so `dws schema` can render richer descriptions without refetching.
func AnnotateRuntimeToolMetadata(cmd *cobra.Command, title, description, source string) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	if title = strings.TrimSpace(title); title != "" {
		cmd.Annotations[runtimeSchemaTitleAnnotation] = title
	}
	if description = strings.TrimSpace(description); description != "" {
		cmd.Annotations[runtimeSchemaDescAnnotation] = description
	}
	if source = strings.TrimSpace(source); source != "" {
		cmd.Annotations[runtimeSchemaMetaAnnotation] = source
	}
}

// AnnotateRuntimeFlag adds parameter metadata to an already-registered flag.
// The metadata mirrors the runtime binding that produced the flag, allowing
// schema rendering to preserve MCP parameter names while displaying CLI flags.
func AnnotateRuntimeFlag(cmd *cobra.Command, flagName, propertyName, paramType string, required bool, defaultValue string) {
	if cmd == nil {
		return
	}
	flagName = strings.TrimSpace(flagName)
	if flagName == "" {
		return
	}
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		return
	}
	setFlagAnnotation(flag, runtimeSchemaFlagPropertyAnnotation, strings.TrimSpace(propertyName))
	setFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation, strings.TrimSpace(paramType))
	setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, strconv.FormatBool(required))
	if strings.TrimSpace(defaultValue) != "" {
		setFlagAnnotation(flag, runtimeSchemaFlagDefaultAnnotation, defaultValue)
	}
}

// AnnotateRuntimeRequiredFlags records schema-only required semantics. Unlike
// cobra.MarkFlagRequired, it does not require the primary flag itself to be
// changed, so helper commands can keep accepting hidden --url/--id aliases.
func AnnotateRuntimeRequiredFlags(cmd *cobra.Command, flagNames ...string) {
	if cmd == nil {
		return
	}
	for _, name := range flagNames {
		flag := cmd.Flags().Lookup(strings.TrimSpace(name))
		if flag != nil {
			setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "true")
		}
	}
}

// AnnotateRuntimeFlagFormat records a machine-readable value format without
// changing the Cobra flag type.
func AnnotateRuntimeFlagFormat(cmd *cobra.Command, flagName, format string) {
	if cmd == nil {
		return
	}
	if flag := cmd.Flags().Lookup(strings.TrimSpace(flagName)); flag != nil {
		setFlagAnnotation(flag, "x-cli-format", strings.TrimSpace(format))
	}
}

// AnnotateRuntimeFlagEnum records the accepted values for a flag.
func AnnotateRuntimeFlagEnum(cmd *cobra.Command, flagName string, values ...string) {
	if cmd == nil {
		return
	}
	flag := cmd.Flags().Lookup(strings.TrimSpace(flagName))
	if flag == nil {
		return
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) == 0 {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations["x-cli-enum"] = clean
}

// AnnotateRuntimeFlagExample records a valid representative CLI value.
func AnnotateRuntimeFlagExample(cmd *cobra.Command, flagName, example string) {
	if cmd == nil {
		return
	}
	if flag := cmd.Flags().Lookup(strings.TrimSpace(flagName)); flag != nil {
		setFlagAnnotation(flag, runtimeSchemaFlagExampleAnnotation, strings.TrimSpace(example))
	}
}

// AnnotateRuntimeConstraints records command-level parameter relationships.
func AnnotateRuntimeConstraints(cmd *cobra.Command, constraints RuntimeSchemaConstraints) {
	if cmd == nil {
		return
	}
	constraints = normalizeRuntimeSchemaConstraints(constraints)
	if runtimeSchemaConstraintsEmpty(constraints) {
		return
	}
	if existing := runtimeCommandConstraints(cmd); !runtimeSchemaConstraintsEmpty(existing) {
		constraints.MutuallyExclusive = append(existing.MutuallyExclusive, constraints.MutuallyExclusive...)
		constraints.RequireOneOf = append(existing.RequireOneOf, constraints.RequireOneOf...)
		constraints.RequireTogether = append(existing.RequireTogether, constraints.RequireTogether...)
		constraints = normalizeRuntimeSchemaConstraints(constraints)
	}
	encoded, _ := json.Marshal(constraints)
	setRuntimeCommandAnnotation(cmd, runtimeSchemaRulesAnnotation, string(encoded))
}

// AnnotateRuntimePositionals records ordered positional arguments for agents.
func AnnotateRuntimePositionals(cmd *cobra.Command, positionals ...RuntimeSchemaPositional) {
	if cmd == nil {
		return
	}
	clean := make([]RuntimeSchemaPositional, 0, len(positionals))
	for _, positional := range positionals {
		positional.Name = strings.TrimSpace(positional.Name)
		positional.Type = strings.TrimSpace(positional.Type)
		positional.Description = strings.TrimSpace(positional.Description)
		if positional.Name == "" || positional.Index < 0 {
			continue
		}
		if positional.Type == "" {
			positional.Type = "string"
		}
		clean = append(clean, positional)
	}
	if len(clean) == 0 {
		return
	}
	sort.SliceStable(clean, func(i, j int) bool { return clean[i].Index < clean[j].Index })
	encoded, _ := json.Marshal(clean)
	setRuntimeCommandAnnotation(cmd, runtimeSchemaArgsAnnotation, string(encoded))
}

// ExcludeFromRuntimeSchema keeps a human-facing hint or redirect in --help
// while preventing it from being advertised as an executable agent tool.
func ExcludeFromRuntimeSchema(cmd *cobra.Command) {
	setRuntimeCommandAnnotation(cmd, runtimeSchemaExcludeAnnotation, "true")
}

func setRuntimeCommandAnnotation(cmd *cobra.Command, key, value string) {
	if cmd == nil || strings.TrimSpace(value) == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[key] = value
}

func setFlagAnnotation(flag *pflag.Flag, key, value string) {
	if flag == nil || strings.TrimSpace(value) == "" {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[key] = []string{value}
}

func runtimeSchemaPayload(root *cobra.Command, args []string) (map[string]any, error) {
	entries := collectRuntimeSchemaEntries(root)
	if len(args) == 0 {
		return runtimeSchemaListPayload(entries), nil
	}

	entry, ok := resolveRuntimeSchemaEntry(entries, args[0])
	if ok {
		return runtimeToolPayload(entry), nil
	}
	if payload, ok := runtimeSchemaBrowsePayload(entries, args[0]); ok {
		return payload, nil
	}
	return nil, apperrors.NewValidation("unknown runtime schema path " + strconvQuote(args[0]))
}

type runtimeSchemaEntry struct {
	ProductID       string
	SourceProductID string
	ProductName     string
	ToolName        string
	CLIName         string
	Group           string
	CLIPath         string
	Title           string
	Description     string
	Source          string
	MetadataSource  string
	Command         *cobra.Command
	PrimaryCLIPath  string
	Aliases         []string
	IsAlias         bool
}

func collectRuntimeSchemaEntries(root *cobra.Command) []runtimeSchemaEntry {
	if root == nil {
		return nil
	}
	entries := []runtimeSchemaEntry{}
	seen := map[string]bool{}
	seenCLIPaths := map[string]bool{}
	walkLeafCommands(root, func(leaf *cobra.Command) {
		if runtimeSchemaExcluded(leaf) {
			return
		}
		productID, toolName, source := runtimeSchemaAnnotations(leaf)
		if productID == "" || toolName == "" {
			return
		}
		parts := commandPathParts(leaf)
		if len(parts) == 0 {
			return
		}
		group := ""
		if len(parts) > 2 {
			group = strings.Join(parts[1:len(parts)-1], ".")
		}
		displayProductID := productID
		productName := ""
		if top := topLevelCommand(leaf); top != nil {
			displayProductID = strings.TrimSpace(top.Name())
			productName = strings.TrimSpace(top.Short)
		}
		if defaultSchemaHintRegistry.ProductVisibility(displayProductID) != SchemaVisibilityPublic {
			return
		}
		entry := runtimeSchemaEntry{
			ProductID:       displayProductID,
			SourceProductID: productID,
			ProductName:     productName,
			ToolName:        toolName,
			CLIName:         leaf.Name(),
			Group:           group,
			CLIPath:         strings.Join(parts, " "),
			Title:           runtimeCommandTitle(leaf),
			Description:     runtimeCommandDescription(leaf),
			Source:          source,
			MetadataSource:  runtimeCommandMetadataSource(leaf),
			Command:         leaf,
		}
		entries = append(entries, entry)
		seen[entryKey(entry)] = true
		seenCLIPaths[entry.ProductID+"\x00"+entry.CLIPath] = true
	})
	for productID, hint := range defaultSchemaHintRegistry.RuntimeRoots() {
		if defaultSchemaHintRegistry.ProductVisibility(productID) != SchemaVisibilityPublic {
			continue
		}
		productRoot, _, err := root.Find([]string{productID})
		if err != nil || productRoot == nil || !productRoot.HasParent() {
			continue
		}
		productName := strings.TrimSpace(productRoot.Short)
		walkLeafCommands(productRoot, func(leaf *cobra.Command) {
			if runtimeSchemaExcluded(leaf) {
				return
			}
			parts := commandPathParts(leaf)
			if len(parts) == 0 {
				return
			}
			cliPath := strings.Join(parts, " ")
			if len(hint.IncludeCLIPaths) > 0 && !hint.IncludeCLIPaths[cliPath] {
				return
			}
			if seenCLIPaths[productID+"\x00"+cliPath] {
				return
			}
			toolName := strings.TrimSpace(hint.ToolNames[cliPath])
			if toolName == "" {
				toolName = derivedRuntimeToolName(parts)
			}
			group := ""
			if len(parts) > 2 {
				group = strings.Join(parts[1:len(parts)-1], ".")
			}
			source := strings.TrimSpace(hint.Source)
			if source == "" {
				source = "hardcoded:" + productID
			}
			entry := runtimeSchemaEntry{
				ProductID:       productID,
				SourceProductID: productID,
				ProductName:     productName,
				ToolName:        toolName,
				CLIName:         leaf.Name(),
				Group:           group,
				CLIPath:         cliPath,
				Title:           runtimeCommandTitle(leaf),
				Description:     runtimeCommandDescription(leaf),
				Source:          source,
				MetadataSource:  runtimeCommandMetadataSource(leaf),
				Command:         leaf,
			}
			if seen[entryKey(entry)] {
				return
			}
			entries = append(entries, entry)
			seen[entryKey(entry)] = true
			seenCLIPaths[entry.ProductID+"\x00"+entry.CLIPath] = true
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ProductID != entries[j].ProductID {
			return entries[i].ProductID < entries[j].ProductID
		}
		if entries[i].ToolName != entries[j].ToolName {
			return entries[i].ToolName < entries[j].ToolName
		}
		return entries[i].CLIPath < entries[j].CLIPath
	})
	annotateRuntimeSchemaAliases(entries)
	return entries
}

func entryKey(entry runtimeSchemaEntry) string {
	return entry.ProductID + "\x00" + entry.ToolName + "\x00" + entry.CLIPath
}

func canonicalEntryKey(entry runtimeSchemaEntry) string {
	return entry.ProductID + "\x00" + entry.ToolName
}

func annotateRuntimeSchemaAliases(entries []runtimeSchemaEntry) {
	groups := map[string][]int{}
	for idx, entry := range entries {
		groups[canonicalEntryKey(entry)] = append(groups[canonicalEntryKey(entry)], idx)
	}
	for _, indexes := range groups {
		if len(indexes) == 0 {
			continue
		}
		primary := choosePrimaryRuntimeEntry(entries, indexes)
		aliases := make([]string, 0, len(indexes)-1)
		for _, idx := range indexes {
			if idx == primary {
				continue
			}
			aliases = append(aliases, entries[idx].CLIPath)
		}
		sort.Strings(aliases)
		primaryPath := entries[primary].CLIPath
		for _, idx := range indexes {
			entries[idx].PrimaryCLIPath = primaryPath
			entries[idx].Aliases = append([]string(nil), aliases...)
			entries[idx].IsAlias = idx != primary
		}
	}
}

func choosePrimaryRuntimeEntry(entries []runtimeSchemaEntry, indexes []int) int {
	primaryHint := schemaPrimaryCLIPath(entries[indexes[0]].ProductID, entries[indexes[0]].ToolName)
	if primaryHint != "" {
		for _, idx := range indexes {
			if entries[idx].CLIPath == primaryHint {
				return idx
			}
		}
	}
	productID := entries[indexes[0]].ProductID
	for _, idx := range indexes {
		if strings.HasPrefix(entries[idx].CLIPath, productID+" ") {
			return idx
		}
	}
	return indexes[0]
}

func schemaPrimaryCLIPath(productID, toolName string) string {
	canonicalPath := productID + "." + toolName
	if hint := schemaHintForCanonicalPath(canonicalPath); strings.TrimSpace(hint.PrimaryCLIPath) != "" {
		return strings.Join(splitSchemaPathTokens(hint.PrimaryCLIPath), " ")
	}
	rootHint, ok := defaultSchemaHintRegistry.RuntimeRoots()[productID]
	if !ok {
		return ""
	}
	if cliPath := strings.TrimSpace(rootHint.PrimaryCLIPaths[toolName]); cliPath != "" {
		return strings.Join(splitSchemaPathTokens(cliPath), " ")
	}
	if cliPath := strings.TrimSpace(rootHint.PrimaryCLIPaths[canonicalPath]); cliPath != "" {
		return strings.Join(splitSchemaPathTokens(cliPath), " ")
	}
	return ""
}

func runtimeSchemaHintForEntry(entry runtimeSchemaEntry) ToolSchemaHint {
	if hint := schemaHintForCanonicalPath(entry.ProductID + "." + entry.ToolName); !isZeroToolSchemaHint(hint) {
		return hint
	}
	if entry.SourceProductID != "" && entry.SourceProductID != entry.ProductID {
		return schemaHintForCanonicalPath(entry.SourceProductID + "." + entry.ToolName)
	}
	return ToolSchemaHint{}
}

func embeddedMCPMetadataForEntry(entry runtimeSchemaEntry) (embeddedMCPToolMetadata, bool) {
	for _, key := range []string{
		entry.SourceProductID + "." + entry.ToolName,
		entry.ProductID + "." + entry.ToolName,
	} {
		key = strings.Trim(key, ".")
		if key == "" {
			continue
		}
		if meta, ok := runtimeEmbeddedMCPMetadata.Tools[key]; ok {
			return meta, true
		}
	}
	return embeddedMCPToolMetadata{}, false
}

func isZeroToolSchemaHint(hint ToolSchemaHint) bool {
	return strings.TrimSpace(hint.Title) == "" &&
		strings.TrimSpace(hint.Description) == "" &&
		strings.TrimSpace(hint.PrimaryCLIPath) == "" &&
		len(hint.Parameters) == 0
}

func derivedRuntimeToolName(parts []string) string {
	if len(parts) <= 1 {
		return "command"
	}
	return strings.ReplaceAll(strings.Join(parts[1:], "_"), "-", "_")
}

func runtimeSchemaAnnotations(cmd *cobra.Command) (productID, toolName, source string) {
	if cmd == nil || cmd.Annotations == nil {
		return "", "", ""
	}
	productID = strings.TrimSpace(cmd.Annotations[runtimeSchemaProductAnnotation])
	toolName = strings.TrimSpace(cmd.Annotations[runtimeSchemaToolAnnotation])
	source = strings.TrimSpace(cmd.Annotations[runtimeSchemaSourceAnnotation])
	if source == "" && productID != "" {
		source = "runtime:" + productID
	}
	return productID, toolName, source
}

func runtimeSchemaExcluded(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations != nil &&
		strings.EqualFold(strings.TrimSpace(cmd.Annotations[runtimeSchemaExcludeAnnotation]), "true")
}

func runtimeCommandTitle(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if cmd.Annotations != nil {
		if title := strings.TrimSpace(cmd.Annotations[runtimeSchemaTitleAnnotation]); title != "" {
			return title
		}
	}
	return strings.TrimSpace(cmd.Short)
}

func runtimeCommandDescription(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if cmd.Annotations != nil {
		if desc := strings.TrimSpace(cmd.Annotations[runtimeSchemaDescAnnotation]); desc != "" {
			return desc
		}
	}
	if desc := strings.TrimSpace(cmd.Long); desc != "" {
		return desc
	}
	return strings.TrimSpace(cmd.Short)
}

func runtimeCommandMetadataSource(cmd *cobra.Command) string {
	if cmd == nil || cmd.Annotations == nil {
		return ""
	}
	return strings.TrimSpace(cmd.Annotations[runtimeSchemaMetaAnnotation])
}

func commandPathParts(cmd *cobra.Command) []string {
	parts := []string{}
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return parts
}

func topLevelCommand(cmd *cobra.Command) *cobra.Command {
	var top *cobra.Command
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		top = c
	}
	return top
}

func runtimeSchemaListPayload(entries []runtimeSchemaEntry) map[string]any {
	byProduct := map[string][]runtimeSchemaEntry{}
	for _, entry := range entries {
		if entry.IsAlias {
			continue
		}
		byProduct[entry.ProductID] = append(byProduct[entry.ProductID], entry)
	}

	ids := make([]string, 0, len(byProduct))
	for id := range byProduct {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	products := make([]map[string]any, 0, len(ids))
	totalTools := 0
	for _, id := range ids {
		productEntries := byProduct[id]
		tools := make([]map[string]any, 0, len(productEntries))
		for _, entry := range productEntries {
			tools = append(tools, runtimeToolSummary(entry))
		}
		totalTools += len(tools)
		products = append(products, map[string]any{
			"id":          id,
			"name":        productEntries[0].ProductName,
			"description": productEntries[0].ProductName,
			"tool_count":  len(tools),
			"tools":       tools,
			"runtime":     true,
		})
		applyAgentProductMetadata(products[len(products)-1], id, productEntries[0].SourceProductID)
	}

	return map[string]any{
		"kind":               "schema",
		"level":              "catalog",
		"count":              len(products),
		"tool_count":         totalTools,
		"products":           products,
		"source":             "runtime-command",
		"interface_metadata": interfaceMetadataSummary(),
		"agent_metadata":     agentMetadataSummary(),
	}
}

// compactSchemaOverviewPayload projects the complete catalog into a small
// first-hop response. Agents can then query one product or group instead of
// loading every tool description into context up front.
func compactSchemaOverviewPayload(payload map[string]any) map[string]any {
	products := schemaMapSlice(payload["products"])
	compact := make([]map[string]any, 0, len(products))
	totalTools := 0
	for _, product := range products {
		id, _ := product["id"].(string)
		toolCount := schemaProductToolCount(product)
		totalTools += toolCount
		entry := map[string]any{
			"id":          id,
			"tool_count":  toolCount,
			"schema_path": id,
		}
		if helper, _ := product["helper"].(bool); helper {
			entry["helper"] = true
		}
		applyCompactAgentProductMetadata(entry, id)
		if _, hasSummary := entry["agent_summary"]; !hasSummary {
			if _, hasUseWhen := entry["use_when"]; !hasUseWhen {
				entry["description"] = product["description"]
			}
		}
		compact = append(compact, entry)
	}
	return map[string]any{
		"kind":               "schema",
		"level":              "products",
		"count":              len(compact),
		"tool_count":         totalTools,
		"products":           compact,
		"source":             payload["source"],
		"interface_metadata": payload["interface_metadata"],
		"agent_metadata":     payload["agent_metadata"],
	}
}

func schemaProductToolCount(product map[string]any) int {
	switch value := product["tool_count"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	if tools, ok := product["tools"].([]map[string]any); ok {
		return len(tools)
	}
	if tools, ok := product["tools"].([]any); ok {
		return len(tools)
	}
	return 0
}

func runtimeToolSummary(entry runtimeSchemaEntry) map[string]any {
	title, description, metadataSource := runtimeToolTextMetadata(entry)
	embeddedMeta, _ := embeddedMCPMetadataForEntry(entry)
	tool := map[string]any{
		"name":             entry.ToolName,
		"cli_name":         entry.CLIName,
		"canonical_path":   entry.ProductID + "." + entry.ToolName,
		"cli_path":         entry.CLIPath,
		"primary_cli_path": entry.PrimaryCLIPath,
		"description":      description,
	}
	if title != "" && title != description {
		tool["title"] = title
	}
	if entry.SourceProductID != "" && entry.SourceProductID != entry.ProductID {
		tool["source_product_id"] = entry.SourceProductID
	}
	if metadataSource != "" {
		tool["metadata_source"] = metadataSource
	}
	if len(entry.Aliases) > 0 {
		tool["aliases"] = entry.Aliases
	}
	applyRuntimeInterfaceRef(tool, entry, embeddedMeta, false)
	applyAgentToolMetadata(tool, false,
		entry.PrimaryCLIPath,
		entry.CLIPath,
		entry.ProductID+"."+entry.ToolName,
	)
	return tool
}

func runtimeToolTextMetadata(entry runtimeSchemaEntry) (title, description, metadataSource string) {
	title = entry.Title
	description = entry.Description
	metadataSource = entry.MetadataSource
	embeddedMeta, hasEmbeddedMeta := embeddedMCPMetadataForEntry(entry)
	if metadataSource == "" && hasEmbeddedMeta {
		if value := strings.TrimSpace(embeddedMeta.Title); value != "" {
			title = value
		}
		if value := strings.TrimSpace(embeddedMeta.Description); value != "" {
			description = value
		}
		metadataSource = "embedded-mcp-metadata"
	}
	hint := runtimeSchemaHintForEntry(entry)
	if value := strings.TrimSpace(hint.Title); value != "" {
		title = value
	}
	if value := strings.TrimSpace(hint.Description); value != "" {
		description = value
	}
	return strings.TrimSpace(title), strings.TrimSpace(description), strings.TrimSpace(metadataSource)
}

func runtimeSchemaBrowsePayload(entries []runtimeSchemaEntry, raw string) (map[string]any, bool) {
	tokens := splitSchemaPathTokens(raw)
	if len(tokens) == 0 {
		return nil, false
	}
	path := strings.Join(tokens, " ")
	if len(tokens) == 1 {
		productEntries := runtimeSchemaEntriesForProduct(entries, tokens[0])
		if len(productEntries) == 0 {
			return nil, false
		}
		product := runtimeSchemaProductObject(productEntries)
		return map[string]any{
			"kind":               "schema",
			"level":              "product",
			"count":              len(productEntries),
			"product":            product,
			"source":             "runtime-command",
			"interface_metadata": interfaceMetadataSummary(),
			"agent_metadata":     agentMetadataSummary(),
		}, true
	}

	groupEntries := make([]runtimeSchemaEntry, 0)
	prefix := path + " "
	for _, entry := range entries {
		if entry.IsAlias || !strings.HasPrefix(entry.CLIPath, prefix) {
			continue
		}
		groupEntries = append(groupEntries, entry)
	}
	if len(groupEntries) == 0 {
		return nil, false
	}
	tools := make([]map[string]any, 0, len(groupEntries))
	for _, entry := range groupEntries {
		tools = append(tools, runtimeToolSummary(entry))
	}
	return map[string]any{
		"kind":               "schema",
		"level":              "group",
		"path":               path,
		"count":              len(tools),
		"tools":              tools,
		"source":             "runtime-command",
		"interface_metadata": interfaceMetadataSummary(),
		"agent_metadata":     agentMetadataSummary(),
	}, true
}

func runtimeSchemaEntriesForProduct(entries []runtimeSchemaEntry, productID string) []runtimeSchemaEntry {
	productID = strings.TrimSpace(productID)
	out := make([]runtimeSchemaEntry, 0)
	for _, entry := range entries {
		if !entry.IsAlias && entry.ProductID == productID {
			out = append(out, entry)
		}
	}
	return out
}

func runtimeSchemaProductObject(entries []runtimeSchemaEntry) map[string]any {
	if len(entries) == 0 {
		return map[string]any{}
	}
	tools := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		tools = append(tools, runtimeToolSummary(entry))
	}
	product := map[string]any{
		"id":          entries[0].ProductID,
		"name":        entries[0].ProductName,
		"description": entries[0].ProductName,
		"tool_count":  len(tools),
		"tools":       tools,
		"runtime":     true,
	}
	applyAgentProductMetadata(product, entries[0].ProductID, entries[0].SourceProductID)
	return product
}

func resolveRuntimeSchemaEntry(entries []runtimeSchemaEntry, raw string) (runtimeSchemaEntry, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return runtimeSchemaEntry{}, false
	}
	tokens := splitSchemaPathTokens(raw)
	normalized := strings.Join(tokens, " ")
	for _, entry := range entries {
		if raw == entry.CLIPath || normalized == entry.CLIPath {
			return entry, true
		}
	}
	for _, entry := range entries {
		if raw == entry.ProductID+"."+entry.ToolName && !entry.IsAlias {
			return entry, true
		}
		if entry.SourceProductID != "" && entry.SourceProductID != entry.ProductID &&
			raw == entry.SourceProductID+"."+entry.ToolName && !entry.IsAlias {
			return entry, true
		}
	}
	return runtimeSchemaEntry{}, false
}

func runtimeToolPayload(entry runtimeSchemaEntry) map[string]any {
	canonicalPath := entry.ProductID + "." + entry.ToolName
	hint := runtimeSchemaHintForEntry(entry)
	embeddedMeta, hasEmbeddedMeta := embeddedMCPMetadataForEntry(entry)
	title, description, metadataSource := runtimeToolTextMetadata(entry)
	constraints := runtimeCommandConstraints(entry.Command)
	parameters := runtimeCommandParameters(entry.Command, hint.Parameters, embeddedMeta.Parameters, constraints)
	if parameters == nil {
		parameters = map[string]any{}
	}
	payload := map[string]any{
		"name":             entry.ToolName,
		"cli_name":         entry.CLIName,
		"canonical_path":   canonicalPath,
		"path":             canonicalPath,
		"cli_path":         entry.CLIPath,
		"primary_cli_path": entry.PrimaryCLIPath,
		"is_alias":         entry.IsAlias,
		"source":           entry.Source,
		"product_id":       entry.ProductID,
		"display":          entry.ProductName,
		"title":            title,
		"description":      description,
		"parameters":       parameters,
		"has_parameters":   len(parameters) > 0,
		"parameter_count":  len(parameters),
	}
	if entry.Group != "" {
		payload["group"] = entry.Group
	}
	if entry.SourceProductID != "" && entry.SourceProductID != entry.ProductID {
		payload["source_product_id"] = entry.SourceProductID
	}
	if metadataSource != "" {
		payload["metadata_source"] = metadataSource
	} else if hasEmbeddedMeta {
		payload["metadata_source"] = "embedded-mcp-metadata"
	}
	if len(entry.Aliases) > 0 {
		payload["aliases"] = entry.Aliases
	}
	applyRuntimeInterfaceRef(payload, entry, embeddedMeta, true)
	if rendered := runtimeConstraintsPayload(constraints); len(rendered) > 0 {
		payload["constraints"] = rendered
	}
	if positionals := runtimeCommandPositionals(entry.Command); len(positionals) > 0 {
		payload["positionals"] = positionals
	}
	paths := []string{entry.PrimaryCLIPath, entry.CLIPath, canonicalPath}
	paths = append(paths, entry.Aliases...)
	applyAgentToolMetadata(payload, true, paths...)
	return payload
}

func applyRuntimeInterfaceRef(target map[string]any, entry runtimeSchemaEntry, metadata embeddedMCPToolMetadata, always bool) {
	if target == nil || metadata.InterfaceRef == nil {
		return
	}
	productID := strings.TrimSpace(metadata.InterfaceRef.ProductID)
	rpcName := strings.TrimSpace(metadata.InterfaceRef.RPCName)
	if productID == "" || rpcName == "" {
		return
	}
	if !always && productID == entry.ProductID && rpcName == entry.ToolName {
		return
	}
	target["interface_ref"] = map[string]any{
		"product_id": productID,
		"rpc_name":   rpcName,
	}
}

func runtimeCommandParameters(cmd *cobra.Command, hints map[string]ParameterSchemaHint, embeddedParams map[string]embeddedMCPParamMeta, constraints RuntimeSchemaConstraints) map[string]any {
	if cmd == nil {
		return nil
	}
	params := map[string]any{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag == nil || flag.Hidden || flag.Name == "help" || isGenericPayloadFlag(flag) {
			return
		}
		property := firstFlagAnnotation(flag, runtimeSchemaFlagPropertyAnnotation)
		if property == "" {
			property = lowerCamelFlagName(flag.Name)
		}
		hint, _, hasHint := lookupParameterSchemaHint(hints, property, flag.Name)
		embeddedParam, hasEmbeddedParam := lookupEmbeddedMCPParam(embeddedParams, property, flag.Name)
		flagName := flag.Name
		if hasHint && strings.TrimSpace(hint.FlagName) != "" {
			flagName = strings.TrimSpace(hint.FlagName)
		}

		paramType := runtimeFlagCLIType(flag)
		interfaceType := firstFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation)
		if hasEmbeddedParam && strings.TrimSpace(embeddedParam.Type) != "" {
			interfaceType = strings.TrimSpace(embeddedParam.Type)
		}
		if hasHint && strings.TrimSpace(hint.Type) != "" {
			interfaceType = strings.TrimSpace(hint.Type)
		}
		description := strings.TrimSpace(flag.Usage)
		interfaceDescription := ""
		if hasEmbeddedParam {
			interfaceDescription = strings.TrimSpace(embeddedParam.Description)
		}
		if hasHint && strings.TrimSpace(hint.Description) != "" {
			description = strings.TrimSpace(hint.Description)
		}
		required, hasCLIRequired := runtimeFlagRequiredState(flag)
		if !hasCLIRequired && hasEmbeddedParam && embeddedParam.Required != nil {
			required = *embeddedParam.Required
		}
		if hasHint && hint.Required != nil {
			required = *hint.Required
		}

		entry := map[string]any{
			"type":        paramType,
			"description": description,
			"required":    required,
		}
		if property != "" {
			entry["property"] = property
		}
		if interfaceDescription != "" && interfaceDescription != description {
			entry["interface_description"] = interfaceDescription
		}
		if interfaceType != "" && interfaceType != paramType {
			entry["interface_type"] = interfaceType
		}
		requiredWhen := ""
		if hasEmbeddedParam {
			requiredWhen = strings.TrimSpace(embeddedParam.RequiredWhen)
		}
		if hasHint && strings.TrimSpace(hint.RequiredWhen) != "" {
			requiredWhen = strings.TrimSpace(hint.RequiredWhen)
		}
		if requiredWhen != "" {
			entry["required_when"] = requiredWhen
		}
		if hasHint && strings.TrimSpace(hint.Default) != "" {
			entry["default"] = strings.TrimSpace(hint.Default)
		} else if def := runtimeFlagDefault(flag); def != "" {
			entry["default"] = def
		} else if hasEmbeddedParam && strings.TrimSpace(embeddedParam.Default) != "" {
			entry["default"] = strings.TrimSpace(embeddedParam.Default)
		}
		if format := firstFlagAnnotation(flag, "x-cli-format"); format != "" {
			entry["format"] = format
		} else if format := inferredRuntimeFlagFormat(flag); format != "" {
			entry["format"] = format
		} else if hasEmbeddedParam && strings.TrimSpace(embeddedParam.Format) != "" {
			entry["format"] = strings.TrimSpace(embeddedParam.Format)
		}
		if enum := runtimeFlagEnum(flag); len(enum) > 0 {
			entry["enum"] = enum
		} else if hasEmbeddedParam && len(embeddedParam.Enum) > 0 {
			entry["enum"] = append([]string(nil), embeddedParam.Enum...)
		}
		if example := firstFlagAnnotation(flag, runtimeSchemaFlagExampleAnnotation); example != "" {
			entry["example"] = example
		}
		params[flagName] = entry
	})
	// A member of a require-one-of group is conditionally required, not
	// individually required. This also corrects usage text such as "群聊必填"
	// that would otherwise make every alternative look globally mandatory.
	for _, group := range constraints.RequireOneOf {
		for _, name := range group {
			if entry, ok := params[name].(map[string]any); ok {
				entry["required"] = false
			}
		}
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func runtimeCommandConstraints(cmd *cobra.Command) RuntimeSchemaConstraints {
	if cmd == nil || cmd.Annotations == nil {
		return RuntimeSchemaConstraints{}
	}
	raw := strings.TrimSpace(cmd.Annotations[runtimeSchemaRulesAnnotation])
	if raw == "" {
		return RuntimeSchemaConstraints{}
	}
	var constraints RuntimeSchemaConstraints
	if json.Unmarshal([]byte(raw), &constraints) != nil {
		return RuntimeSchemaConstraints{}
	}
	return normalizeRuntimeSchemaConstraints(constraints)
}

func runtimeCommandPositionals(cmd *cobra.Command) []RuntimeSchemaPositional {
	if cmd == nil || cmd.Annotations == nil {
		return nil
	}
	raw := strings.TrimSpace(cmd.Annotations[runtimeSchemaArgsAnnotation])
	if raw == "" {
		return nil
	}
	var positionals []RuntimeSchemaPositional
	if json.Unmarshal([]byte(raw), &positionals) != nil {
		return nil
	}
	sort.SliceStable(positionals, func(i, j int) bool { return positionals[i].Index < positionals[j].Index })
	return positionals
}

func normalizeRuntimeSchemaConstraints(constraints RuntimeSchemaConstraints) RuntimeSchemaConstraints {
	constraints.MutuallyExclusive = normalizeRuntimeSchemaGroups(constraints.MutuallyExclusive, 2)
	constraints.RequireOneOf = normalizeRuntimeSchemaGroups(constraints.RequireOneOf, 1)
	constraints.RequireTogether = normalizeRuntimeSchemaGroups(constraints.RequireTogether, 2)
	return constraints
}

func normalizeRuntimeSchemaGroups(groups [][]string, minimum int) [][]string {
	out := make([][]string, 0, len(groups))
	seenGroups := map[string]bool{}
	for _, group := range groups {
		clean := make([]string, 0, len(group))
		seenNames := map[string]bool{}
		for _, name := range group {
			name = strings.TrimSpace(name)
			if name == "" || seenNames[name] {
				continue
			}
			seenNames[name] = true
			clean = append(clean, name)
		}
		if len(clean) < minimum {
			continue
		}
		key := strings.Join(clean, "\x00")
		if seenGroups[key] {
			continue
		}
		seenGroups[key] = true
		out = append(out, clean)
	}
	return out
}

func runtimeSchemaConstraintsEmpty(constraints RuntimeSchemaConstraints) bool {
	return len(constraints.MutuallyExclusive) == 0 &&
		len(constraints.RequireOneOf) == 0 &&
		len(constraints.RequireTogether) == 0
}

func runtimeConstraintsPayload(constraints RuntimeSchemaConstraints) map[string]any {
	payload := map[string]any{}
	if len(constraints.MutuallyExclusive) > 0 {
		payload["mutually_exclusive"] = constraints.MutuallyExclusive
	}
	if len(constraints.RequireOneOf) > 0 {
		payload["require_one_of"] = constraints.RequireOneOf
	}
	if len(constraints.RequireTogether) > 0 {
		payload["require_together"] = constraints.RequireTogether
	}
	return payload
}

func lookupEmbeddedMCPParam(params map[string]embeddedMCPParamMeta, property, flagName string) (embeddedMCPParamMeta, bool) {
	if len(params) == 0 {
		return embeddedMCPParamMeta{}, false
	}
	if meta, ok := params[property]; ok {
		return meta, true
	}
	if meta, ok := params[flagName]; ok {
		return meta, true
	}
	return embeddedMCPParamMeta{}, false
}

func isGenericPayloadFlag(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	switch flag.Name {
	case "json":
		return strings.TrimSpace(flag.Usage) == "Base JSON object payload for this tool invocation"
	case "params":
		return strings.TrimSpace(flag.Usage) == "Additional JSON object payload merged after --json"
	default:
		return false
	}
}

func runtimeFlagType(flag *pflag.Flag) string {
	return runtimeFlagCLIType(flag)
}

func runtimeFlagCLIType(flag *pflag.Flag) string {
	switch flag.Value.Type() {
	case "int", "int8", "int16", "int32", "int64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "bool":
		return "boolean"
	case "stringSlice", "stringArray":
		return "array"
	default:
		return "string"
	}
}

func runtimeFlagRequired(flag *pflag.Flag) bool {
	required, _ := runtimeFlagRequiredState(flag)
	return required
}

func runtimeFlagRequiredState(flag *pflag.Flag) (bool, bool) {
	if raw := firstFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation); raw != "" {
		required, err := strconv.ParseBool(raw)
		if err == nil {
			return required, true
		}
	}
	if values := flag.Annotations[cobra.BashCompOneRequiredFlag]; len(values) > 0 {
		return true, true
	}
	usage := strings.ToLower(strings.TrimSpace(flag.Usage))
	if usageImpliesRequired(usage) {
		return true, true
	}
	return false, false
}

func usageImpliesRequired(usage string) bool {
	usage = strings.ToLower(strings.TrimSpace(usage))
	if usage == "" {
		return false
	}
	for _, conditional := range []string{
		"可选", "时必填", "下必填", "二选一", "至少传一个", "至少填一个", "至少提供一项",
		"at least one", "required when", "required if", "conditionally required",
	} {
		if strings.Contains(usage, conditional) {
			return false
		}
	}
	return strings.Contains(usage, "required") || strings.Contains(usage, "必填")
}

func lowerCamelFlagName(flagName string) string {
	flagName = strings.TrimSpace(flagName)
	parts := strings.FieldsFunc(flagName, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return flagName
	}
	if len(parts) == 1 {
		return parts[0]
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	out := strings.ToLower(parts[0])
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		out += strings.ToUpper(lower[:1]) + lower[1:]
	}
	return out
}

func runtimeFlagDefault(flag *pflag.Flag) string {
	if annotated := firstFlagAnnotation(flag, runtimeSchemaFlagDefaultAnnotation); annotated != "" {
		return annotated
	}
	def := strings.TrimSpace(flag.DefValue)
	switch flag.Value.Type() {
	case "bool":
		if def == "false" {
			return ""
		}
	case "int", "int8", "int16", "int32", "int64", "float32", "float64":
		if def == "0" {
			return ""
		}
	case "stringSlice", "stringArray":
		if def == "[]" {
			return ""
		}
	}
	return def
}

func firstFlagAnnotation(flag *pflag.Flag, key string) string {
	if flag == nil || flag.Annotations == nil {
		return ""
	}
	values := flag.Annotations[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func runtimeFlagEnum(flag *pflag.Flag) []string {
	if flag == nil || flag.Annotations == nil {
		return nil
	}
	values := flag.Annotations["x-cli-enum"]
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func inferredRuntimeFlagFormat(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}
	usage := strings.ToLower(strings.TrimSpace(flag.Usage))
	if strings.Contains(usage, "iso-8601") || strings.Contains(usage, "rfc3339") {
		return "date-time"
	}
	if strings.Contains(usage, "a1") {
		return "a1-range"
	}
	return ""
}

func strconvQuote(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}
