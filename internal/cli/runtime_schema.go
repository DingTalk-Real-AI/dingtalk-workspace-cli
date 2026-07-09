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
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	runtimeSchemaProductAnnotation = "dws.schema.product"
	runtimeSchemaToolAnnotation    = "dws.schema.tool"
	runtimeSchemaSourceAnnotation  = "dws.schema.source"

	runtimeSchemaFlagPropertyAnnotation = "dws.schema.property"
	runtimeSchemaFlagTypeAnnotation     = "dws.schema.type"
	runtimeSchemaFlagRequiredAnnotation = "dws.schema.required"
	runtimeSchemaFlagDefaultAnnotation  = "dws.schema.default"
)

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
	if required {
		setFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation, "true")
	}
	if strings.TrimSpace(defaultValue) != "" {
		setFlagAnnotation(flag, runtimeSchemaFlagDefaultAnnotation, defaultValue)
	}
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
	if !ok {
		return nil, apperrors.NewValidation("unknown runtime schema path " + strconvQuote(args[0]))
	}
	return runtimeToolPayload(entry), nil
}

type runtimeSchemaEntry struct {
	ProductID      string
	ProductName    string
	ToolName       string
	CLIName        string
	Group          string
	CLIPath        string
	Title          string
	Description    string
	Source         string
	Command        *cobra.Command
	PrimaryCLIPath string
	Aliases        []string
	IsAlias        bool
}

func collectRuntimeSchemaEntries(root *cobra.Command) []runtimeSchemaEntry {
	if root == nil {
		return nil
	}
	entries := []runtimeSchemaEntry{}
	seen := map[string]bool{}
	walkLeafCommands(root, func(leaf *cobra.Command) {
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
		productName := ""
		if top := topLevelCommand(leaf); top != nil {
			productName = strings.TrimSpace(top.Short)
		}
		entry := runtimeSchemaEntry{
			ProductID:   productID,
			ProductName: productName,
			ToolName:    toolName,
			CLIName:     leaf.Name(),
			Group:       group,
			CLIPath:     strings.Join(parts, " "),
			Title:       strings.TrimSpace(leaf.Short),
			Description: runtimeCommandDescription(leaf),
			Source:      source,
			Command:     leaf,
		}
		entries = append(entries, entry)
		seen[entryKey(entry)] = true
	})
	for productID, hint := range defaultSchemaHintRegistry.RuntimeRoots() {
		productRoot, _, err := root.Find([]string{productID})
		if err != nil || productRoot == nil || !productRoot.HasParent() {
			continue
		}
		productName := strings.TrimSpace(productRoot.Short)
		walkLeafCommands(productRoot, func(leaf *cobra.Command) {
			parts := commandPathParts(leaf)
			if len(parts) == 0 {
				return
			}
			cliPath := strings.Join(parts, " ")
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
				ProductID:   productID,
				ProductName: productName,
				ToolName:    toolName,
				CLIName:     leaf.Name(),
				Group:       group,
				CLIPath:     cliPath,
				Title:       strings.TrimSpace(leaf.Short),
				Description: runtimeCommandDescription(leaf),
				Source:      source,
				Command:     leaf,
			}
			if seen[entryKey(entry)] {
				return
			}
			entries = append(entries, entry)
			seen[entryKey(entry)] = true
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

func runtimeCommandDescription(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if desc := strings.TrimSpace(cmd.Long); desc != "" {
		return desc
	}
	return strings.TrimSpace(cmd.Short)
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
	for _, id := range ids {
		productEntries := byProduct[id]
		tools := make([]map[string]any, 0, len(productEntries))
		for _, entry := range productEntries {
			tool := map[string]any{
				"name":             entry.ToolName,
				"cli_name":         entry.CLIName,
				"canonical_path":   entry.ProductID + "." + entry.ToolName,
				"cli_path":         entry.CLIPath,
				"primary_cli_path": entry.PrimaryCLIPath,
				"description":      entry.Description,
			}
			if len(entry.Aliases) > 0 {
				tool["aliases"] = entry.Aliases
			}
			tools = append(tools, tool)
		}
		products = append(products, map[string]any{
			"id":          id,
			"name":        productEntries[0].ProductName,
			"description": productEntries[0].ProductName,
			"tools":       tools,
			"runtime":     true,
		})
	}

	return map[string]any{
		"kind":     "schema",
		"count":    len(products),
		"products": products,
		"source":   "runtime-command",
	}
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
	}
	return runtimeSchemaEntry{}, false
}

func runtimeToolPayload(entry runtimeSchemaEntry) map[string]any {
	canonicalPath := entry.ProductID + "." + entry.ToolName
	hint := schemaHintForCanonicalPath(canonicalPath)
	title := entry.Title
	if strings.TrimSpace(hint.Title) != "" {
		title = strings.TrimSpace(hint.Title)
	}
	description := entry.Description
	if strings.TrimSpace(hint.Description) != "" {
		description = strings.TrimSpace(hint.Description)
	}
	parameters := runtimeCommandParameters(entry.Command, hint.Parameters)
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
	if len(entry.Aliases) > 0 {
		payload["aliases"] = entry.Aliases
	}
	return payload
}

func runtimeCommandParameters(cmd *cobra.Command, hints map[string]ParameterSchemaHint) map[string]any {
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
		flagName := flag.Name
		if hasHint && strings.TrimSpace(hint.FlagName) != "" {
			flagName = strings.TrimSpace(hint.FlagName)
		}

		paramType := runtimeFlagType(flag)
		if hasHint && strings.TrimSpace(hint.Type) != "" {
			paramType = strings.TrimSpace(hint.Type)
		}
		description := strings.TrimSpace(flag.Usage)
		if hasHint && strings.TrimSpace(hint.Description) != "" {
			description = strings.TrimSpace(hint.Description)
		}
		required := runtimeFlagRequired(flag)
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
		if hasHint && strings.TrimSpace(hint.Default) != "" {
			entry["default"] = strings.TrimSpace(hint.Default)
		} else if def := runtimeFlagDefault(flag); def != "" {
			entry["default"] = def
		}
		params[flagName] = entry
	})
	if len(params) == 0 {
		return nil
	}
	return params
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
	if annotated := firstFlagAnnotation(flag, runtimeSchemaFlagTypeAnnotation); annotated != "" {
		return annotated
	}
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
	if strings.EqualFold(firstFlagAnnotation(flag, runtimeSchemaFlagRequiredAnnotation), "true") {
		return true
	}
	if values := flag.Annotations[cobra.BashCompOneRequiredFlag]; len(values) > 0 {
		return true
	}
	usage := strings.ToLower(strings.TrimSpace(flag.Usage))
	if usageImpliesRequired(usage) {
		return true
	}
	return false
}

func usageImpliesRequired(usage string) bool {
	usage = strings.ToLower(strings.TrimSpace(usage))
	if usage == "" {
		return false
	}
	for _, conditional := range []string{"可选", "时必填", "二选一", "至少传一个", "至少填一个", "至少提供一项", "at least one"} {
		if strings.Contains(usage, conditional) {
			return false
		}
	}
	return strings.Contains(usage, "required") || strings.Contains(usage, "必填")
}

func lowerCamelFlagName(flagName string) string {
	parts := strings.FieldsFunc(flagName, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return strings.TrimSpace(flagName)
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

func strconvQuote(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}
