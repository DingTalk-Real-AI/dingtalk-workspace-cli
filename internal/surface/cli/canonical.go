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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/output"
	"github.com/spf13/cobra"
)

type FlagKind string

const (
	flagString      FlagKind = "string"
	flagInteger     FlagKind = "integer"
	flagNumber      FlagKind = "number"
	flagBoolean     FlagKind = "boolean"
	flagStringArray FlagKind = "string_array"
	flagIntegerList FlagKind = "integer_array"
	flagNumberList  FlagKind = "number_array"
	flagBooleanList FlagKind = "boolean_array"
	flagJSON        FlagKind = "json"
)

type FlagSpec struct {
	PropertyName string   `json:"property_name"`
	FlagName     string   `json:"flag_name"`
	Alias        string   `json:"alias,omitempty"`
	Shorthand    string   `json:"shorthand,omitempty"`
	Kind         FlagKind `json:"kind"`
	Description  string   `json:"description,omitempty"`
	Hidden       bool     `json:"hidden,omitempty"`
}

// AddCanonicalProducts loads the catalog and registers each product command
// directly onto root, removing the intermediate "mcp" sub-command layer.
// Grouped aliases are also attached to root.
func AddCanonicalProducts(root *cobra.Command, loader CatalogLoader, runner executor.Runner) error {
	catalog, loadErr := loader.Load(commandContext(root))
	if loadErr != nil {
		return loadErr
	}

	for _, product := range catalog.Products {
		if product.CLI != nil && product.CLI.Skip {
			continue
		}
		upsertProductCommand(root, product, runner)
	}
	for _, product := range catalog.Products {
		if product.CLI != nil && product.CLI.Skip {
			continue
		}
		addGroupedProductAlias(root, product, runner)
	}
	return nil
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

// NewMCPCommand creates a standalone command tree rooted at "mcp" for
// unit-testing product/tool command construction. The "mcp" subcommand layer
// has been removed from production paths. This function is retained only for
// test compatibility. Production code should use AddCanonicalProducts instead.
func NewMCPCommand(loader CatalogLoader, runner executor.Runner) *cobra.Command {
	catalog, loadErr := loader.Load(context.Background())

	longDescription := "Reserved canonical runtime surface. Tools are generated from the shared Tool IR."
	if loadErr != nil {
		longDescription += fmt.Sprintf("\n\nDiscovery note: %v", loadErr)
	}
	if len(catalog.Products) == 0 {
		longDescription += "\n\nNo canonical products are currently loaded. Set DWS_CATALOG_FIXTURE to populate the surface."
	}

	cmd := &cobra.Command{
		Use:               "mcp",
		Short:             "Canonical MCP-derived CLI surface",
		Long:              longDescription,
		Hidden:            false,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringP("output-format", "f", "table", "Output format: json|table|raw")
	cmd.PersistentFlags().String("format", "table", "Output format: json|table|raw")
	_ = cmd.PersistentFlags().MarkHidden("format")

	if loadErr != nil {
		cmd.Args = cobra.ArbitraryArgs
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return loadErr
		}
		return cmd
	}

	for _, product := range catalog.Products {
		if product.CLI != nil && product.CLI.Skip {
			continue
		}
		upsertProductCommand(cmd, product, runner)
	}
	for _, product := range catalog.Products {
		if product.CLI != nil && product.CLI.Skip {
			continue
		}
		addGroupedProductAlias(cmd, product, runner)
	}
	return cmd
}

func NewSchemaCommand(loader CatalogLoader) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "schema [product[.tool]]",
		Short:             "Inspect product and tool schema metadata",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := loader.Load(cmd.Context())
			if err != nil {
				return err
			}

			payload, err := schemaPayload(catalog, args)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().Bool("json", false, "Emit schema metadata as JSON")
	return cmd
}

func BuildFlagSpecs(schema map[string]any, hints map[string]ir.CLIFlagHint) []FlagSpec {
	fields := buildFlagFields(schema, hints)
	if len(fields) == 0 {
		return nil
	}

	specs := make([]FlagSpec, 0, len(fields))
	for _, field := range fields {
		specs = append(specs, field.Spec)
	}
	return specs
}

func BuildEffectiveFlagHints(schema map[string]any, hints map[string]ir.CLIFlagHint) map[string]ir.CLIFlagHint {
	return buildEffectiveFlagHints(schema, hints)
}

func RequiredFlagProperties(hints map[string]ir.CLIFlagHint) []string {
	return requiredFlagProperties(hints)
}

func VisibleFlagSpecs(specs []FlagSpec) []FlagSpec {
	if len(specs) == 0 {
		return nil
	}
	out := make([]FlagSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Hidden {
			continue
		}
		out = append(out, spec)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type flagField struct {
	Spec FlagSpec
	Hint ir.CLIFlagHint
}

type flagFieldCandidate struct {
	PropertyName    string
	LeafName        string
	Kind            FlagKind
	Description     string
	Hint            ir.CLIFlagHint
	IsNested        bool
	PrimaryWanted   string
	FallbackPrimary string
	SecondaryAlias  string
}

func buildFlagFields(schema map[string]any, hints map[string]ir.CLIFlagHint) []flagField {
	candidates := preferPublicAliases(collectFlagFieldCandidates(schema, hints, ""))
	if len(candidates) == 0 {
		return nil
	}

	reservedPrimary := make(map[string]struct{}, len(candidates))
	reservedAliases := make(map[string]int)
	for _, candidate := range candidates {
		if candidate.IsNested {
			continue
		}
		reservedPrimary[candidate.PrimaryWanted] = struct{}{}
		if alias := kebabFlagSegment(strings.TrimSpace(candidate.SecondaryAlias)); alias != "" && alias != candidate.PrimaryWanted {
			reservedAliases[alias]++
		}
	}

	desiredNestedCounts := make(map[string]int)
	for _, candidate := range candidates {
		if !candidate.IsNested {
			continue
		}
		if desired := strings.TrimSpace(candidate.PrimaryWanted); desired != "" {
			desiredNestedCounts[desired]++
		}
	}

	usedPrimary := make(map[string]struct{}, len(candidates))
	usedAliases := make(map[string]struct{}, len(candidates))
	fields := make([]flagField, 0, len(candidates))

	for _, candidate := range candidates {
		spec := FlagSpec{
			PropertyName: candidate.PropertyName,
			Kind:         candidate.Kind,
			Description:  candidate.Description,
			Shorthand:    strings.TrimSpace(candidate.Hint.Shorthand),
			Hidden:       candidate.Hint.Hidden,
		}

		if candidate.IsNested {
			flagName := strings.TrimSpace(candidate.PrimaryWanted)
			if flagName == "" {
				flagName = candidate.FallbackPrimary
			}
			if desiredNestedCounts[strings.TrimSpace(candidate.PrimaryWanted)] > 1 {
				flagName = candidate.FallbackPrimary
			}
			if _, exists := reservedPrimary[flagName]; exists {
				flagName = candidate.FallbackPrimary
			}
			if reservedAliases[flagName] > 0 {
				flagName = candidate.FallbackPrimary
			}
			flagName = uniqueFlagName(flagName, usedPrimary)
			spec.FlagName = flagName
		} else {
			spec.FlagName = candidate.PrimaryWanted
			usedPrimary[spec.FlagName] = struct{}{}
		}

		fields = append(fields, flagField{
			Spec: spec,
			Hint: candidate.Hint,
		})
	}

	for index, candidate := range candidates {
		if candidate.IsNested {
			continue
		}
		alias := kebabFlagSegment(strings.TrimSpace(candidate.SecondaryAlias))
		if alias == "" || alias == fields[index].Spec.FlagName {
			continue
		}
		if reservedAliases[alias] > 1 {
			continue
		}
		if _, exists := usedPrimary[alias]; exists {
			continue
		}
		if _, exists := usedAliases[alias]; exists {
			continue
		}
		fields[index].Spec.Alias = alias
		usedAliases[alias] = struct{}{}
	}

	return fields
}

func preferPublicAliases(candidates []flagFieldCandidate) []flagFieldCandidate {
	if len(candidates) == 0 {
		return nil
	}

	out := append([]flagFieldCandidate(nil), candidates...)
	reservedPrimary := make(map[string]struct{}, len(out))
	reservedAliases := make(map[string]int)
	for _, candidate := range out {
		if candidate.IsNested {
			continue
		}
		if primary := strings.TrimSpace(candidate.PrimaryWanted); primary != "" {
			reservedPrimary[primary] = struct{}{}
		}
		if alias := strings.TrimSpace(candidate.SecondaryAlias); alias != "" && alias != candidate.PrimaryWanted {
			reservedAliases[alias]++
		}
	}

	for index, candidate := range out {
		if candidate.IsNested {
			continue
		}
		alias := strings.TrimSpace(candidate.SecondaryAlias)
		primary := strings.TrimSpace(candidate.PrimaryWanted)
		if alias == "" || alias == primary {
			continue
		}
		if reservedAliases[alias] > 1 {
			continue
		}
		if _, exists := reservedPrimary[alias]; exists {
			continue
		}
		out[index].PrimaryWanted = alias
		out[index].SecondaryAlias = primary
	}

	return out
}

func buildEffectiveFlagHints(schema map[string]any, hints map[string]ir.CLIFlagHint) map[string]ir.CLIFlagHint {
	fields := buildFlagFields(schema, hints)
	if len(fields) == 0 {
		return nil
	}

	out := make(map[string]ir.CLIFlagHint)
	for _, field := range fields {
		if !hasCLIFlagHintData(field.Hint) {
			continue
		}
		out[field.Spec.PropertyName] = field.Hint
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectFlagFieldCandidates(schema map[string]any, hints map[string]ir.CLIFlagHint, prefix string) []flagFieldCandidate {
	properties, ok := nestedMap(schema, "properties")
	if !ok {
		return nil
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sortStrings(keys)

	requiredSet := requiredFieldLookup(schema)
	candidates := make([]flagFieldCandidate, 0, len(keys))
	for _, key := range keys {
		propertySchema, ok := properties[key].(map[string]any)
		if !ok {
			continue
		}

		propertyName := joinFlagPropertyPath(prefix, key)
		hint := hints[propertyName]
		if requiredSet[key] {
			hint.Required = true
		}
		if hint.Default == nil {
			if value, exists := propertySchema["default"]; exists {
				hint.Default = value
			}
		}

		if shouldExpandFlagObject(propertySchema, hint) {
			children := collectFlagFieldCandidates(propertySchema, hints, propertyName)
			if len(children) > 0 {
				candidates = append(candidates, children...)
				continue
			}
		}

		kind, ok := flagKindForSchema(propertySchema)
		if !ok {
			continue
		}
		if transformedKind, hasTransform := flagKindForTransform(hint.Transform); hasTransform {
			kind = transformedKind
		}

		leafName := lastFlagPathToken(propertyName)
		isNested := strings.Contains(propertyName, ".")
		candidate := flagFieldCandidate{
			PropertyName:    propertyName,
			LeafName:        leafName,
			Kind:            kind,
			Description:     schemaDescription(propertySchema),
			Hint:            hint,
			IsNested:        isNested,
			PrimaryWanted:   publicLeafFlagName(leafName),
			FallbackPrimary: canonicalPathFallbackFlagName(propertyName),
		}
		if !isNested {
			candidate.PrimaryWanted = propertyFlagName(propertyName)
			candidate.SecondaryAlias = strings.TrimSpace(hint.Alias)
		} else if alias := strings.TrimSpace(hint.Alias); alias != "" {
			candidate.PrimaryWanted = alias
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func shouldExpandFlagObject(schema map[string]any, hint ir.CLIFlagHint) bool {
	if hint.Hidden {
		return false
	}
	if strings.TrimSpace(hint.Transform) == "json_parse" {
		return false
	}
	if schema == nil {
		return false
	}
	if rawType, _ := schema["type"].(string); strings.TrimSpace(rawType) != "object" {
		return false
	}
	properties, ok := nestedMap(schema, "properties")
	if !ok || len(properties) == 0 {
		return false
	}
	if hasComplexSchemaComposition(schema) {
		return false
	}
	if raw, exists := schema["additionalProperties"]; exists {
		switch value := raw.(type) {
		case bool:
			if value {
				return false
			}
		case map[string]any:
			return false
		default:
			return false
		}
	}
	return true
}

func hasComplexSchemaComposition(schema map[string]any) bool {
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if _, exists := schema[key]; exists {
			return true
		}
	}
	return false
}

func hasCLIFlagHintData(hint ir.CLIFlagHint) bool {
	return strings.TrimSpace(hint.Shorthand) != "" ||
		strings.TrimSpace(hint.Alias) != "" ||
		strings.TrimSpace(hint.Transform) != "" ||
		len(hint.TransformArgs) > 0 ||
		strings.TrimSpace(hint.EnvDefault) != "" ||
		hint.Default != nil ||
		hint.Hidden ||
		hint.Required
}

func uniqueFlagName(name string, used map[string]struct{}) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "field"
	}
	if _, exists := used[name]; !exists {
		used[name] = struct{}{}
		return name
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", name, index)
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func propertyFlagName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", "-")
}

func publicLeafFlagName(name string) string {
	return propertyFlagName(strings.TrimSpace(name))
}

func canonicalPathFallbackFlagName(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, kebabFlagSegment(part))
	}
	return strings.Join(out, "-")
}

func kebabFlagSegment(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "_", "-")
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

func joinFlagPropertyPath(prefix, name string) string {
	prefix = strings.TrimSpace(prefix)
	name = strings.TrimSpace(name)
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

func lastFlagPathToken(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

func requiredFieldLookup(schema map[string]any) map[string]bool {
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make(map[string]bool, len(raw))
	for _, entry := range raw {
		value, ok := entry.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

// flagKindForTransform maps a _meta.cli transform name to the corresponding
// FlagKind. Returns (kind, true) when the transform is recognised, or
// ("", false) when it is unknown or empty (caller keeps the schema-derived kind).
func flagKindForTransform(transform string) (FlagKind, bool) {
	switch strings.TrimSpace(transform) {
	case "csv_to_array":
		// Comma-separated string input that the executor splits into []string.
		return flagStringArray, true
	case "json_parse":
		// Raw JSON string input that the executor parses into an object/array.
		return flagJSON, true
	case "iso8601_to_millis":
		// Timestamp strings are normalized into milliseconds before invocation.
		return flagString, true
	case "enum_map":
		// Enum values are accepted as strings and mapped during normalization.
		return flagString, true
	default:
		return "", false
	}
}

func newBaseProductCommand(product ir.CanonicalProduct) *cobra.Command {
	shortDescription := product.DisplayName
	if strings.TrimSpace(product.Description) != "" {
		shortDescription = product.Description
	}
	use := product.ID
	if preferred := preferredProductRouteToken(product); preferred != "" {
		use = preferred
	}
	if shortDescription == "" {
		shortDescription = use
	}
	if shortDescription == "" {
		shortDescription = product.ID
	}
	aliases := commandAliases(use, product.ID)
	if product.CLI != nil {
		aliases = append(aliases, product.CLI.Aliases...)
		aliases = uniqueCommandAliases(use, aliases)
	}

	cmd := &cobra.Command{
		Use:               use,
		Aliases:           aliases,
		Short:             shortDescription,
		Hidden:            product.CLI != nil && product.CLI.Hidden,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	suppressAliasHelp(cmd)
	mergeProductCommandMetadata(cmd, product)
	return cmd
}

func mergeProductIntoCommand(cmd *cobra.Command, product ir.CanonicalProduct, runner executor.Runner) {
	if cmd == nil {
		return
	}
	mergeProductCommandMetadata(cmd, product)

	groupCmds := make(map[string]*cobra.Command)
	resolvedRoutes := resolveToolRoutes(product)
	for _, tool := range product.Tools {
		toolCmd := newToolCommand(product, tool, resolvedRoutes[toolRouteKey(tool)], runner)
		if tool.Group == "" {
			addOrMergeChildCommand(cmd, toolCmd)
		} else {
			parent := ensureNestedGroupCmd(cmd, tool.Group, product, groupCmds)
			merged := addOrMergeChildCommand(parent, toolCmd)
			registerGroupCommand(groupCmds, tool.Group, merged.Name(), merged)
		}
	}
}

// ensureNestedGroupCmd creates intermediate group commands for a potentially nested
// group path (e.g. "chat.group" → "chat" → "group"). It mirrors the logic in
// compat/dynamic_commands.go ensureNestedGroup so both paths behave identically.
func ensureNestedGroupCmd(root *cobra.Command, groupPath string, product ir.CanonicalProduct, registry map[string]*cobra.Command) *cobra.Command {
	if existing, ok := registry[groupPath]; ok {
		return existing
	}

	parts := strings.Split(groupPath, ".")
	parent := root
	builtPath := ""
	for i, part := range parts {
		if builtPath == "" {
			builtPath = part
		} else {
			builtPath = builtPath + "." + part
		}

		if existing, ok := registry[builtPath]; ok {
			parent = existing
			continue
		}

		if existing := commandByName(parent, part); existing != nil {
			if i == len(parts)-1 {
				applyGroupDescription(existing, product, builtPath, part)
			}
			registry[builtPath] = existing
			parent = existing
			continue
		}

		description := part
		if i == len(parts)-1 {
			description = groupDescription(product, groupPath, part)
		}
		groupCmd := &cobra.Command{
			Use:               part,
			Short:             description,
			Args:              cobra.NoArgs,
			DisableAutoGenTag: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return cmd.Help()
			},
		}
		suppressAliasHelp(groupCmd)
		parent.AddCommand(groupCmd)
		registry[builtPath] = groupCmd
		parent = groupCmd
	}
	return parent
}

func registerGroupCommand(registry map[string]*cobra.Command, groupPath string, leaf string, cmd *cobra.Command) {
	if len(registry) == 0 || cmd == nil {
		return
	}
	parts := splitCommandGroupPath(groupPath)
	leaf = strings.TrimSpace(leaf)
	if leaf != "" {
		parts = append(parts, leaf)
	}
	if len(parts) == 0 {
		return
	}
	registry[strings.Join(parts, ".")] = cmd
}

func upsertProductCommand(root *cobra.Command, product ir.CanonicalProduct, runner executor.Runner) *cobra.Command {
	if root == nil {
		return nil
	}
	use := product.ID
	if preferred := preferredProductRouteToken(product); preferred != "" {
		use = preferred
	}
	if existing := commandByName(root, use); existing != nil {
		mergeProductIntoCommand(existing, product, runner)
		return existing
	}
	cmd := newBaseProductCommand(product)
	root.AddCommand(cmd)
	mergeProductIntoCommand(cmd, product, runner)
	return cmd
}

func addGroupedProductAlias(root *cobra.Command, product ir.CanonicalProduct, runner executor.Runner) {
	if root == nil || product.CLI == nil {
		return
	}

	groupPath := splitRouteTokens(product.CLI.Group)
	if len(groupPath) == 0 {
		return
	}

	commandPath := splitRouteTokens(product.CLI.Command)
	if len(commandPath) == 0 {
		commandPath = []string{product.ID}
	}
	fullPath := append(append([]string{}, groupPath...), commandPath...)
	if len(fullPath) == 0 {
		return
	}

	parent := root
	if len(fullPath) > 1 {
		parent = ensureNestedGroupCmd(root, strings.Join(fullPath[:len(fullPath)-1], "."), product, make(map[string]*cobra.Command))
	}

	leaf := fullPath[len(fullPath)-1]
	if existing := commandByName(parent, leaf); existing != nil {
		aliasProduct := product
		if aliasProduct.CLI != nil {
			cliCopy := *aliasProduct.CLI
			cliCopy.Command = ""
			cliCopy.Group = ""
			aliasProduct.CLI = &cliCopy
		}
		mergeProductIntoCommand(existing, aliasProduct, runner)
		return
	}

	aliasProduct := product
	if aliasProduct.CLI != nil {
		cliCopy := *aliasProduct.CLI
		cliCopy.Command = ""
		cliCopy.Group = ""
		aliasProduct.CLI = &cliCopy
	}
	productCommand := newBaseProductCommand(aliasProduct)
	productCommand.Use = leaf
	productCommand.Aliases = nil
	if leaf != aliasProduct.ID {
		productCommand.Aliases = append(productCommand.Aliases, aliasProduct.ID)
	}
	parent.AddCommand(productCommand)
	mergeProductIntoCommand(productCommand, aliasProduct, runner)
}

func mergeProductCommandMetadata(cmd *cobra.Command, product ir.CanonicalProduct) {
	if cmd == nil {
		return
	}
	shortDescription := product.DisplayName
	if strings.TrimSpace(product.Description) != "" {
		shortDescription = product.Description
	}
	if strings.TrimSpace(cmd.Short) == "" || isPlaceholderGroupDescription(cmd.Short, cmd.Name()) {
		cmd.Short = shortDescription
	}
	cmd.Hidden = cmd.Hidden && product.CLI != nil && product.CLI.Hidden
	if !cmd.Hidden && product.CLI == nil {
		cmd.Hidden = false
	}
	if product.CLI != nil {
		cmd.Aliases = uniqueCommandAliases(cmd.Name(), append(cmd.Aliases, product.CLI.Aliases...))
		if strings.TrimSpace(product.CLI.Group) != "" {
			cmd.Long = fmt.Sprintf("%s\n\nGroup: %s", shortDescription, product.CLI.Group)
		}
	}
	if warning := lifecycleWarning(product); warning != "" {
		longText := cmd.Long
		if strings.TrimSpace(longText) == "" {
			longText = shortDescription
		}
		cmd.Long = strings.TrimSpace(longText + "\n\nLifecycle: " + warning)
	}
}

func addOrMergeChildCommand(parent *cobra.Command, candidate *cobra.Command) *cobra.Command {
	if parent == nil || candidate == nil {
		return candidate
	}
	if existing := commandByName(parent, candidate.Name()); existing != nil {
		subcommands := append([]*cobra.Command(nil), existing.Commands()...)
		for _, child := range subcommands {
			existing.RemoveCommand(child)
		}
		parent.RemoveCommand(existing)
		candidate.Aliases = uniqueCommandAliases(candidate.Name(), append(candidate.Aliases, existing.Aliases...))
		for _, child := range subcommands {
			candidate.AddCommand(child)
		}
		parent.AddCommand(candidate)
		return candidate
	}
	parent.AddCommand(candidate)
	return candidate
}

func groupDescription(product ir.CanonicalProduct, groupPath string, fallback string) string {
	if product.CLI != nil && product.CLI.Groups != nil {
		if def, ok := product.CLI.Groups[groupPath]; ok && def.Description != "" {
			return def.Description
		}
	}
	return fallback
}

func applyGroupDescription(cmd *cobra.Command, product ir.CanonicalProduct, groupPath string, fallback string) {
	if cmd == nil {
		return
	}
	description := groupDescription(product, groupPath, fallback)
	if description == fallback && !isPlaceholderGroupDescription(cmd.Short, cmd.Name()) && strings.TrimSpace(cmd.Short) != "" {
		return
	}
	cmd.Short = description
}

func isPlaceholderGroupDescription(value string, fallback string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == strings.TrimSpace(fallback) || strings.HasPrefix(value, "Canonical group ")
}

func newToolCommand(product ir.CanonicalProduct, tool ir.ToolDescriptor, route resolvedToolRoute, runner executor.Runner) *cobra.Command {
	shortDescription := tool.Title
	if strings.TrimSpace(tool.Description) != "" {
		shortDescription = tool.Description
	}
	specs := BuildFlagSpecs(tool.InputSchema, tool.FlagHints)
	use := route.Use
	if strings.TrimSpace(use) == "" {
		use = strings.TrimSpace(tool.CLIName)
		if use == "" {
			use = tool.RPCName
		}
	}
	if shortDescription == "" {
		shortDescription = use
	}
	aliases := route.Aliases
	if aliases == nil {
		aliases = uniqueCommandAliases(use, append(commandAliases(use, tool.RPCName), tool.Aliases...))
	}

	cmd := &cobra.Command{
		Use:               use,
		Aliases:           aliases,
		Short:             shortDescription,
		Hidden:            tool.Hidden,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if warning := lifecycleWarning(product); warning != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning)
			}
			dryRun := false
			if cmd.Flags().Lookup("dry-run") != nil {
				value, err := cmd.Flags().GetBool("dry-run")
				if err != nil {
					return apperrors.NewInternal("failed to read --dry-run")
				}
				dryRun = value
			}
			params, err := normalizeCanonicalParams(cmd, tool.InputSchema, specs, tool.FlagHints)
			if err != nil {
				return err
			}
			if err := ValidateInputSchema(params, tool.InputSchema); err != nil {
				return err
			}
			if !dryRun {
				if err := confirmSensitiveTool(cmd, tool); err != nil {
					return err
				}
			}
			invocation := executor.NewInvocation(product, tool, params)
			invocation.DryRun = dryRun
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			if warning := lifecycleWarning(product); warning != "" {
				if result.Response == nil {
					result.Response = map[string]any{}
				}
				result.Response["warning"] = warning
			}
			return output.WriteCommandPayload(cmd, result, output.FormatTable)
		},
	}
	suppressAliasHelp(cmd)

	cmd.Flags().String("json", "", "Base JSON object payload for this tool invocation")
	cmd.Flags().String("params", "", "Additional JSON object payload merged after --json")
	_ = cmd.Flags().MarkHidden("json")
	_ = cmd.Flags().MarkHidden("params")
	applyFlagSpecs(cmd, specs)

	// Canonical tool commands use priority 50 so they win over compat/dynamic
	// overlay commands (priority 0) during command tree merging, but still
	// yield to hand-written curated commands (priority 100).
	SetOverridePriority(cmd, 50)

	return cmd
}

func commandAliases(use string, fallback string) []string {
	if fallback == "" || fallback == use {
		return nil
	}
	return []string{fallback}
}

func uniqueCommandAliases(use string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	canonicalUse := normalizeRouteToken(use)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if normalizeRouteToken(value) == canonicalUse {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func applyFlagSpecs(cmd *cobra.Command, specs []FlagSpec) {
	for _, spec := range specs {
		primary := strings.TrimSpace(spec.FlagName)
		if primary == "" {
			continue
		}
		alias := effectiveFlagAlias(spec, specs)
		primaryUsage := primaryFlagUsage(spec, specs)
		aliasUsage := baseFlagUsage(spec)

		switch spec.Kind {
		case flagString, flagJSON:
			cmd.Flags().StringP(primary, spec.Shorthand, "", primaryUsage)
			if spec.Hidden {
				_ = cmd.Flags().MarkHidden(primary)
			}
			if alias != "" {
				cmd.Flags().String(alias, "", aliasUsage)
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagInteger:
			cmd.Flags().IntP(primary, spec.Shorthand, 0, primaryUsage)
			if spec.Hidden {
				_ = cmd.Flags().MarkHidden(primary)
			}
			if alias != "" {
				cmd.Flags().Int(alias, 0, aliasUsage)
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagNumber:
			cmd.Flags().Float64P(primary, spec.Shorthand, 0, primaryUsage)
			if spec.Hidden {
				_ = cmd.Flags().MarkHidden(primary)
			}
			if alias != "" {
				cmd.Flags().Float64(alias, 0, aliasUsage)
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagBoolean:
			cmd.Flags().BoolP(primary, spec.Shorthand, false, primaryUsage)
			if spec.Hidden {
				_ = cmd.Flags().MarkHidden(primary)
			}
			if alias != "" {
				cmd.Flags().Bool(alias, false, aliasUsage)
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagStringArray, flagIntegerList, flagNumberList, flagBooleanList:
			cmd.Flags().StringSliceP(primary, spec.Shorthand, nil, primaryUsage)
			if spec.Hidden {
				_ = cmd.Flags().MarkHidden(primary)
			}
			if alias != "" {
				cmd.Flags().StringSlice(alias, nil, aliasUsage)
				_ = cmd.Flags().MarkHidden(alias)
			}
		}
	}
}

func baseFlagUsage(spec FlagSpec) string {
	usage := spec.Description
	if usage == "" {
		usage = fmt.Sprintf("Override %s", spec.PropertyName)
	}
	return usage
}

func primaryFlagUsage(spec FlagSpec, specs []FlagSpec) string {
	usage := baseFlagUsage(spec)
	if alias := effectiveFlagAlias(spec, specs); alias != "" {
		return usage
	}
	return usage
}

func effectiveFlagAlias(spec FlagSpec, specs []FlagSpec) string {
	alias := strings.TrimSpace(spec.Alias)
	if alias == "" || alias == strings.TrimSpace(spec.FlagName) {
		return ""
	}
	for _, candidate := range specs {
		if candidate.PropertyName == spec.PropertyName {
			continue
		}
		if strings.TrimSpace(candidate.FlagName) == alias {
			return ""
		}
		if strings.TrimSpace(candidate.Alias) == alias {
			return ""
		}
	}
	return alias
}

func suppressAliasHelp(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	baseHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(current *cobra.Command, args []string) {
		aliases := current.Aliases
		if len(aliases) == 0 {
			baseHelp(current, args)
			return
		}
		current.Aliases = nil
		defer func() {
			current.Aliases = aliases
		}()
		baseHelp(current, args)
	})
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

func schemaPayload(catalog ir.Catalog, args []string) (map[string]any, error) {
	if len(args) == 0 {
		return map[string]any{
			"kind":     "schema",
			"scope":    "catalog",
			"products": schemaCatalogProducts(catalog),
			"count":    len(catalog.Products),
		}, nil
	}

	query := strings.TrimSpace(args[0])
	if query == "" {
		return nil, apperrors.NewValidation("schema path cannot be empty")
	}

	if !strings.Contains(query, ".") {
		product, ok := findSchemaProduct(catalog, query)
		if !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("unknown schema product %q", query))
		}
		tools := schemaProductTools(product)
		return map[string]any{
			"kind":    "schema",
			"scope":   "product",
			"path":    product.ID,
			"product": schemaProductSummary(product),
			"tools":   tools,
			"count":   len(tools),
		}, nil
	}

	product, tool, ok := findSchemaTool(catalog, query)
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("unknown schema path %q", query))
	}
	flags := VisibleFlagSpecs(BuildFlagSpecs(tool.InputSchema, tool.FlagHints))
	flagHints := visibleFlagHints(buildEffectiveFlagHints(tool.InputSchema, tool.FlagHints), flags)
	payload := map[string]any{
		"kind":         "schema",
		"scope":        "tool",
		"path":         tool.CanonicalPath,
		"product":      schemaProductSummary(product),
		"tool":         schemaToolSummary(tool),
		"flag_hints":   flagHints,
		"cli_path":     schemaToolCLIPath(product, tool),
		"input_schema": tool.InputSchema,
		"required":     requiredFlagProperties(flagHints),
		"flags":        flags,
	}
	if len(flagHints) > 0 {
		payload["flag_hints"] = flagHints
	}
	if len(tool.OutputSchema) > 0 {
		payload["output_schema"] = tool.OutputSchema
	}
	return payload, nil
}

func schemaCatalogProducts(catalog ir.Catalog) []map[string]any {
	products := make([]map[string]any, 0, len(catalog.Products))
	for _, product := range catalog.Products {
		products = append(products, schemaProductSummary(product))
	}
	return products
}

func schemaProductSummary(product ir.CanonicalProduct) map[string]any {
	summary := map[string]any{
		"id":           product.ID,
		"display_name": product.DisplayName,
		"description":  product.Description,
		"tool_count":   len(product.Tools),
	}
	if command := schemaProductCommand(product); command != "" {
		summary["command"] = command
	}
	if strings.TrimSpace(product.Endpoint) != "" {
		summary["endpoint"] = product.Endpoint
	}
	if strings.TrimSpace(product.NegotiatedProtocolVersion) != "" {
		summary["negotiated_protocol_version"] = product.NegotiatedProtocolVersion
	}
	return summary
}

func schemaProductTools(product ir.CanonicalProduct) []map[string]any {
	tools := make([]map[string]any, 0, len(product.Tools))
	for _, tool := range product.Tools {
		toolSummary := schemaToolSummary(tool)
		flags := VisibleFlagSpecs(BuildFlagSpecs(tool.InputSchema, tool.FlagHints))
		flagHints := visibleFlagHints(buildEffectiveFlagHints(tool.InputSchema, tool.FlagHints), flags)
		toolSummary["cli_path"] = schemaToolCLIPath(product, tool)
		toolSummary["required"] = requiredFlagProperties(flagHints)
		toolSummary["flags"] = flags
		if len(flagHints) > 0 {
			toolSummary["flag_hints"] = flagHints
		}
		tools = append(tools, toolSummary)
	}
	return tools
}

func schemaToolSummary(tool ir.ToolDescriptor) map[string]any {
	summary := map[string]any{
		"rpc_name":       tool.RPCName,
		"cli_name":       tool.CLIName,
		"title":          tool.Title,
		"description":    tool.Description,
		"canonical_path": tool.CanonicalPath,
		"sensitive":      tool.Sensitive,
	}
	if tool.Hidden {
		summary["hidden"] = true
	}
	return summary
}

func schemaToolCLIPath(product ir.CanonicalProduct, tool ir.ToolDescriptor) []string {
	path := []string{schemaProductCommand(product)}
	if len(path) == 1 && path[0] == "" {
		path[0] = product.ID
	}
	path = append(path, splitCommandGroupPath(tool.Group)...)
	use := resolveToolRoutes(product)[toolRouteKey(tool)].Use
	if strings.TrimSpace(use) == "" {
		use = strings.TrimSpace(tool.CLIName)
		if use == "" {
			use = tool.RPCName
		}
	}
	if use != "" {
		path = append(path, use)
	}

	out := make([]string, 0, len(path))
	for _, token := range path {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		out = append(out, token)
	}
	return out
}

// ResolveToolCLIPath returns the schema-derived CLI path for a tool, including
// the preferred product command token and any route disambiguation applied by
// the canonical CLI builder.
func ResolveToolCLIPath(product ir.CanonicalProduct, tool ir.ToolDescriptor) []string {
	return schemaToolCLIPath(product, tool)
}

func splitCommandGroupPath(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

type resolvedToolRoute struct {
	Use     string
	Aliases []string
}

func resolveToolRoutes(product ir.CanonicalProduct) map[string]resolvedToolRoute {
	resolved := make(map[string]resolvedToolRoute, len(product.Tools))
	occupiedByParent := make(map[string]map[string]struct{})

	for _, tool := range product.Tools {
		parentKey := toolParentRouteKey(product, tool)
		occupied := occupiedByParent[parentKey]
		if occupied == nil {
			occupied = make(map[string]struct{})
			occupiedByParent[parentKey] = occupied
		}

		use := strings.TrimSpace(tool.CLIName)
		if use == "" {
			use = tool.RPCName
		}
		if routeTokenOccupied(occupied, use) {
			fallback := strings.TrimSpace(tool.RPCName)
			if fallback != "" && !routeTokenOccupied(occupied, fallback) {
				use = fallback
			}
		}
		markRouteToken(occupied, use)

		aliases := uniqueCommandAliases(use, append(commandAliases(use, tool.RPCName), tool.Aliases...))
		filteredAliases := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			if routeTokenOccupied(occupied, alias) {
				continue
			}
			filteredAliases = append(filteredAliases, alias)
			markRouteToken(occupied, alias)
		}

		resolved[toolRouteKey(tool)] = resolvedToolRoute{
			Use:     use,
			Aliases: filteredAliases,
		}
	}

	return resolved
}

func toolParentRouteKey(product ir.CanonicalProduct, tool ir.ToolDescriptor) string {
	parts := []string{schemaProductCommand(product)}
	parts = append(parts, splitCommandGroupPath(tool.Group)...)
	return strings.Join(parts, "/")
}

func toolRouteKey(tool ir.ToolDescriptor) string {
	if strings.TrimSpace(tool.CanonicalPath) != "" {
		return strings.TrimSpace(tool.CanonicalPath)
	}
	return strings.TrimSpace(tool.RPCName)
}

func routeTokenOccupied(occupied map[string]struct{}, value string) bool {
	if occupied == nil {
		return false
	}
	_, exists := occupied[normalizeRouteToken(value)]
	return exists
}

func markRouteToken(occupied map[string]struct{}, value string) {
	if occupied == nil {
		return
	}
	normalized := normalizeRouteToken(value)
	if normalized == "" {
		return
	}
	occupied[normalized] = struct{}{}
}

func findSchemaProduct(catalog ir.Catalog, query string) (ir.CanonicalProduct, bool) {
	query = normalizeSchemaToken(query)
	if query == "" {
		return ir.CanonicalProduct{}, false
	}
	for _, product := range catalog.Products {
		for _, candidate := range schemaProductCandidates(product) {
			if candidate == query {
				return product, true
			}
		}
	}
	return ir.CanonicalProduct{}, false
}

func findSchemaTool(catalog ir.Catalog, query string) (ir.CanonicalProduct, ir.ToolDescriptor, bool) {
	query = normalizeSchemaToken(query)
	if query == "" {
		return ir.CanonicalProduct{}, ir.ToolDescriptor{}, false
	}
	for _, product := range catalog.Products {
		for _, tool := range product.Tools {
			for _, candidate := range schemaToolCandidates(product, tool) {
				if candidate == query {
					return product, tool, true
				}
			}
		}
	}
	return ir.CanonicalProduct{}, ir.ToolDescriptor{}, false
}

func schemaToolCandidates(product ir.CanonicalProduct, tool ir.ToolDescriptor) []string {
	candidates := []string{normalizeSchemaToken(tool.CanonicalPath)}
	route := resolveToolRoutes(product)[toolRouteKey(tool)]
	routeNames := []string{route.Use}
	routeNames = append(routeNames, route.Aliases...)

	for _, productCandidate := range schemaProductCandidates(product) {
		for _, routeName := range routeNames {
			candidate := schemaPath(productCandidate, routeName)
			if candidate != "" {
				candidates = append(candidates, candidate)
			}

			groupedCandidate := schemaPath(productCandidate, tool.Group, routeName)
			if groupedCandidate != "" && groupedCandidate != candidate {
				candidates = append(candidates, groupedCandidate)
			}
		}
	}

	return uniqueSchemaCandidates(candidates)
}

func schemaPath(parts ...string) string {
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		tokens = append(tokens, splitRouteTokens(part)...)
	}
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, ".")
}

func schemaProductCandidates(product ir.CanonicalProduct) []string {
	candidates := []string{normalizeSchemaToken(product.ID)}
	if product.CLI != nil {
		if command := normalizeSchemaToken(product.CLI.Command); command != "" {
			candidates = append(candidates, command)
		}
		if group := strings.TrimSpace(product.CLI.Group); group != "" {
			groupPath := normalizeSchemaToken(group)
			commandPath := normalizeSchemaToken(product.CLI.Command)
			switch {
			case groupPath != "" && commandPath != "":
				candidates = append(candidates, groupPath+"."+commandPath)
			case groupPath != "":
				candidates = append(candidates, groupPath)
			}
		}
	}
	return uniqueSchemaCandidates(candidates)
}

func uniqueSchemaCandidates(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeSchemaToken(raw string) string {
	parts := splitRouteTokens(raw)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ".")
}

func schemaProductCommand(product ir.CanonicalProduct) string {
	if preferred := preferredProductRouteToken(product); preferred != "" {
		return preferred
	}
	return product.ID
}

func writeJSON(w io.Writer, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return apperrors.NewInternal("failed to encode canonical CLI JSON output")
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func confirmSensitiveTool(cmd *cobra.Command, tool ir.ToolDescriptor) error {
	if !tool.Sensitive {
		return nil
	}

	yes := false
	if cmd.Flags().Lookup("yes") != nil {
		value, err := cmd.Flags().GetBool("yes")
		if err != nil {
			return apperrors.NewInternal("failed to read --yes")
		}
		yes = value
	}
	if yes {
		return nil
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "tool %s is sensitive, continue? [y/N]: ", tool.CanonicalPath)
	confirmed, err := readYesNo(cmd.InOrStdin())
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to read confirmation input: %v", err))
	}
	if !confirmed {
		return apperrors.NewValidation("sensitive operation cancelled; use --yes to skip confirmation")
	}
	return nil
}

func readYesNo(r io.Reader) (bool, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func lifecycleWarning(product ir.CanonicalProduct) string {
	if product.Lifecycle == nil {
		return ""
	}
	if product.Lifecycle.DeprecatedBy <= 0 && strings.TrimSpace(product.Lifecycle.DeprecationDate) == "" && !product.Lifecycle.DeprecatedCandidate {
		return ""
	}
	parts := make([]string, 0, 3)
	if product.Lifecycle.DeprecatedCandidate && product.Lifecycle.DeprecatedBy <= 0 && strings.TrimSpace(product.Lifecycle.DeprecationDate) == "" {
		parts = append(parts, fmt.Sprintf("product %s is marked as legacy candidate", product.ID))
	} else {
		parts = append(parts, fmt.Sprintf("product %s is deprecated", product.ID))
	}
	if product.Lifecycle.DeprecatedBy > 0 {
		parts = append(parts, fmt.Sprintf("deprecated_by_mcpId=%d", product.Lifecycle.DeprecatedBy))
	}
	if strings.TrimSpace(product.Lifecycle.DeprecationDate) != "" {
		parts = append(parts, "deprecation_date="+strings.TrimSpace(product.Lifecycle.DeprecationDate))
	}
	if strings.TrimSpace(product.Lifecycle.MigrationURL) != "" {
		parts = append(parts, "migration="+strings.TrimSpace(product.Lifecycle.MigrationURL))
	}
	return strings.Join(parts, "; ")
}

func nestedMap(root map[string]any, key string) (map[string]any, bool) {
	if root == nil {
		return nil, false
	}
	value, ok := root[key]
	if !ok {
		return nil, false
	}
	out, ok := value.(map[string]any)
	return out, ok
}

func flagKindForSchema(schema map[string]any) (FlagKind, bool) {
	if _, ok := schema["enum"].([]any); ok {
		return flagString, true
	}
	switch schema["type"] {
	case "string":
		return flagString, true
	case "integer":
		return flagInteger, true
	case "number":
		return flagNumber, true
	case "boolean":
		return flagBoolean, true
	case "object":
		return flagJSON, true
	case "array":
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return flagJSON, true
		}
		if _, ok := items["enum"].([]any); ok {
			return flagStringArray, true
		}
		switch items["type"] {
		case "string":
			return flagStringArray, true
		case "integer":
			return flagIntegerList, true
		case "number":
			return flagNumberList, true
		case "boolean":
			return flagBooleanList, true
		case "object":
			return flagJSON, true
		}
	}
	return "", false
}

func schemaDescription(schema map[string]any) string {
	value, _ := schema["description"].(string)
	return strings.TrimSpace(value)
}

func requiredFlagProperties(hints map[string]ir.CLIFlagHint) []string {
	if len(hints) == 0 {
		return nil
	}
	required := make([]string, 0, len(hints))
	for property, hint := range hints {
		if hint.Required {
			required = append(required, property)
		}
	}
	if len(required) == 0 {
		return nil
	}
	sortStrings(required)
	return required
}

func visibleFlagHints(hints map[string]ir.CLIFlagHint, specs []FlagSpec) map[string]ir.CLIFlagHint {
	if len(hints) == 0 || len(specs) == 0 {
		return nil
	}
	visibleProps := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Hidden {
			continue
		}
		visibleProps[spec.PropertyName] = struct{}{}
	}
	if len(visibleProps) == 0 {
		return nil
	}
	out := make(map[string]ir.CLIFlagHint)
	for property, hint := range hints {
		if _, ok := visibleProps[property]; !ok {
			continue
		}
		if hint.Hidden {
			continue
		}
		out[property] = hint
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringsToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func intsToAny(values []int) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func floatsToAny(values []float64) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func boolsToAny(values []bool) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func parseStringList[T any](values []string, parse func(string) (T, error)) ([]T, error) {
	out := make([]T, 0, len(values))
	for _, value := range values {
		parsed, err := parse(value)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

func sortStrings(values []string) {
	if len(values) < 2 {
		return
	}
	for idx := 1; idx < len(values); idx++ {
		current := values[idx]
		cursor := idx - 1
		for cursor >= 0 && values[cursor] > current {
			values[cursor+1] = values[cursor]
			cursor--
		}
		values[cursor+1] = current
	}
}

func preferredProductRouteToken(product ir.CanonicalProduct) string {
	if product.CLI == nil {
		return ""
	}
	parts := splitRouteTokens(product.CLI.Command)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func splitRouteTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	segments := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '/' || r == '\\' || r == '.'
	})
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		normalized := normalizeRouteToken(segment)
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func normalizeRouteToken(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func commandByName(parent *cobra.Command, name string) *cobra.Command {
	if parent == nil {
		return nil
	}
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}
