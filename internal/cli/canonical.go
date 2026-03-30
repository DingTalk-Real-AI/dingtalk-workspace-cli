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
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/convert"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
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
	PropertyName string
	FlagName     string
	Alias        string
	Shorthand    string
	Kind         FlagKind
	Description  string
}

func NewMCPCommand(ctx context.Context, loader CatalogLoader, runner executor.Runner, engine *pipeline.Engine) *cobra.Command {
	catalog, loadErr := loader.Load(ctx)

	longDescription := "Reserved canonical runtime surface. Tools are generated from the shared Tool IR under dws mcp."
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
		productCommand := newProductCommand(product, runner, engine)
		cmd.AddCommand(productCommand)
		addGroupedProductAlias(cmd, product, runner, engine)
	}
	return cmd
}

func NewSchemaCommand(loader CatalogLoader) *cobra.Command {
	return &cobra.Command{
		Use:   "schema [product.tool]",
		Short: "查看 MCP 工具 Schema (产品列表 / 工具参数)",
		Long: `查看已发现的 MCP 产品和工具的 Schema 元数据。

不带参数时列出所有产品及其工具数量；带 product.tool 路径时
输出该工具的完整输入 Schema（JSON Schema 格式）。

示例:
  dws schema                         # 列出所有产品
  dws schema aitable.query_records   # 查看 aitable query_records 的参数 Schema
  dws schema --fields id,tools       # 只显示 id 和 tools 字段
  dws schema --jq '.products[].id'   # 用 jq 提取所有产品 ID`,
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

			return output.WriteFiltered(
				cmd.OutOrStdout(),
				output.ResolveFormat(cmd, output.FormatJSON),
				payload,
				output.ResolveFields(cmd),
				output.ResolveJQ(cmd),
			)
		},
	}
}

func BuildFlagSpecs(schema map[string]any, hints map[string]ir.CLIFlagHint) []FlagSpec {
	properties, ok := nestedMap(schema, "properties")
	if !ok {
		return nil
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	specs := make([]FlagSpec, 0, len(keys))
	for _, key := range keys {
		propertySchema, ok := properties[key].(map[string]any)
		if !ok {
			continue
		}

		kind, ok := flagKindForSchema(propertySchema)
		if !ok {
			continue
		}

		specs = append(specs, FlagSpec{
			PropertyName: key,
			FlagName:     strings.ReplaceAll(key, "_", "-"),
			Alias:        strings.TrimSpace(hints[key].Alias),
			Shorthand:    strings.TrimSpace(hints[key].Shorthand),
			Kind:         kind,
			Description:  schemaDescription(propertySchema),
		})
	}
	return specs
}

func newProductCommand(product ir.CanonicalProduct, runner executor.Runner, engine *pipeline.Engine) *cobra.Command {
	shortDescription := product.DisplayName
	if strings.TrimSpace(product.Description) != "" {
		shortDescription = product.Description
	}
	if shortDescription == "" {
		shortDescription = product.ID
	}
	aliases := make([]string, 0, 1)
	if preferred := preferredProductRouteToken(product); preferred != "" && preferred != product.ID {
		aliases = append(aliases, preferred)
	}

	cmd := &cobra.Command{
		Use:               product.ID,
		Aliases:           aliases,
		Short:             shortDescription,
		Hidden:            product.CLI != nil && product.CLI.Hidden,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	if product.CLI != nil && strings.TrimSpace(product.CLI.Group) != "" {
		cmd.Long = fmt.Sprintf("%s\n\nGroup: %s", shortDescription, product.CLI.Group)
	}
	if warning := lifecycleWarning(product); warning != "" {
		if strings.TrimSpace(cmd.Long) == "" {
			cmd.Long = shortDescription
		}
		cmd.Long = strings.TrimSpace(cmd.Long + "\n\nLifecycle: " + warning)
	}

	for _, tool := range product.Tools {
		cmd.AddCommand(newToolCommand(product, tool, runner, engine))
	}
	return cmd
}

func addGroupedProductAlias(root *cobra.Command, product ir.CanonicalProduct, runner executor.Runner, engine *pipeline.Engine) {
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
	for _, token := range fullPath[:len(fullPath)-1] {
		existing := cobracmd.ChildByName(parent, token)
		if existing != nil {
			parent = existing
			continue
		}
		groupCommand := &cobra.Command{
			Use:               token,
			Short:             fmt.Sprintf("Canonical group %s", token),
			Args:              cobra.NoArgs,
			DisableAutoGenTag: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return cmd.Help()
			},
		}
		parent.AddCommand(groupCommand)
		parent = groupCommand
	}

	leaf := fullPath[len(fullPath)-1]
	if cobracmd.ChildByName(parent, leaf) != nil {
		return
	}

	aliasProduct := product
	if aliasProduct.CLI != nil {
		cliCopy := *aliasProduct.CLI
		cliCopy.Command = ""
		cliCopy.Group = ""
		aliasProduct.CLI = &cliCopy
	}
	productCommand := newProductCommand(aliasProduct, runner, engine)
	productCommand.Use = leaf
	productCommand.Aliases = nil
	if leaf != aliasProduct.ID {
		productCommand.Aliases = append(productCommand.Aliases, aliasProduct.ID)
	}
	parent.AddCommand(productCommand)
}

func newToolCommand(product ir.CanonicalProduct, tool ir.ToolDescriptor, runner executor.Runner, engine *pipeline.Engine) *cobra.Command {
	shortDescription := tool.Title
	if strings.TrimSpace(tool.Description) != "" {
		shortDescription = tool.Description
	}
	specs := BuildFlagSpecs(tool.InputSchema, tool.FlagHints)
	use := strings.TrimSpace(tool.CLIName)
	if use == "" {
		use = tool.RPCName
	}
	aliases := make([]string, 0, 1)
	if use != tool.RPCName {
		aliases = append(aliases, tool.RPCName)
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

			// One guard per invocation ensures stdin is read at most once.
			guard := NewStdinGuard()

			jsonPayload, err := cmd.Flags().GetString("json")
			if err != nil {
				return apperrors.NewInternal("failed to read --json")
			}

			// Resolve @file / @- for --json flag.
			jsonPayload, err = ResolveInputSource(jsonPayload, "json", guard)
			if err != nil {
				return err
			}

			paramsPayload, err := cmd.Flags().GetString("params")
			if err != nil {
				return apperrors.NewInternal("failed to read --params")
			}

			// Resolve @file / @- for all string-typed override flags BEFORE
			// the implicit stdin fallback, so explicit @- in any flag takes
			// priority over the implicit pipe read.
			overrides, err := collectOverrides(cmd, specs, guard)
			if err != nil {
				return err
			}

			// Implicit stdin fallback (lowest priority): if no --json was
			// given and no flag claimed stdin via @-, read from pipe.
			if jsonPayload == "" && !guard.Claimed() && StdinIsPipe() {
				if claimErr := guard.Claim("implicit stdin (pipe)"); claimErr != nil {
					return claimErr
				}
				stdinData, stdinErr := ReadStdin()
				if stdinErr != nil {
					return stdinErr
				}
				jsonPayload = stdinData
			}

			params, err := executor.MergePayloads(jsonPayload, paramsPayload, overrides)
			if err != nil {
				return err
			}

			// PostParse: normalise parameter values (date formats,
			// booleans, enums) using the tool's input schema.
			if engine != nil && engine.HasHandlers(pipeline.PostParse) {
				pctx := &pipeline.Context{
					Command: tool.CanonicalPath,
					Params:  params,
					Schema:  tool.InputSchema,
				}
				if pipeErr := engine.RunPhase(pipeline.PostParse, pctx); pipeErr != nil {
					return pipeErr
				}
				params = pctx.Params
			}

			if err := ValidateInputSchema(params, tool.InputSchema); err != nil {
				return err
			}
			if !dryRun {
				if err := confirmSensitiveTool(cmd, tool, guard); err != nil {
					return err
				}
			}

			// PreRequest: last chance to inspect/mutate payload before
			// the JSON-RPC call is dispatched.
			if engine != nil && engine.HasHandlers(pipeline.PreRequest) {
				pctx := &pipeline.Context{
					Command: tool.CanonicalPath,
					Params:  params,
					Schema:  tool.InputSchema,
					Payload: params,
				}
				if pipeErr := engine.RunPhase(pipeline.PreRequest, pctx); pipeErr != nil {
					return pipeErr
				}
				params = pctx.Params
			}

			invocation := executor.NewInvocation(product, tool, params)
			invocation.DryRun = dryRun
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}

			// PostResponse: transform or enrich the response before
			// writing it to stdout.
			if engine != nil && engine.HasHandlers(pipeline.PostResponse) {
				pctx := &pipeline.Context{
					Command:  tool.CanonicalPath,
					Params:   params,
					Schema:   tool.InputSchema,
					Response: result.Response,
				}
				if pipeErr := engine.RunPhase(pipeline.PostResponse, pctx); pipeErr != nil {
					return pipeErr
				}
				result.Response = pctx.Response
			}

			if warning := lifecycleWarning(product); warning != "" {
				if result.Response == nil {
					result.Response = map[string]any{}
				}
				result.Response["warning"] = warning
			}
			return output.WriteFiltered(
				cmd.OutOrStdout(),
				output.ResolveFormat(cmd, output.FormatJSON),
				result,
				output.ResolveFields(cmd),
				output.ResolveJQ(cmd),
			)
		},
	}

	cmd.Flags().String("json", "", "Base JSON object payload for this tool invocation")
	cmd.Flags().String("params", "", "Additional JSON object payload merged after --json")
	applyFlagSpecs(cmd, specs)
	return cmd
}

func applyFlagSpecs(cmd *cobra.Command, specs []FlagSpec) {
	for _, spec := range specs {
		usage := spec.Description
		if usage == "" {
			usage = fmt.Sprintf("Override %s", spec.PropertyName)
		}
		primary := strings.TrimSpace(spec.FlagName)
		if primary == "" {
			continue
		}
		alias := strings.TrimSpace(spec.Alias)
		if alias == primary {
			alias = ""
		}

		switch spec.Kind {
		case flagString, flagJSON:
			cmd.Flags().StringP(primary, spec.Shorthand, "", usage)
			if alias != "" {
				cmd.Flags().String(alias, "", usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagInteger:
			cmd.Flags().IntP(primary, spec.Shorthand, 0, usage)
			if alias != "" {
				cmd.Flags().Int(alias, 0, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagNumber:
			cmd.Flags().Float64P(primary, spec.Shorthand, 0, usage)
			if alias != "" {
				cmd.Flags().Float64(alias, 0, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagBoolean:
			cmd.Flags().BoolP(primary, spec.Shorthand, false, usage)
			if alias != "" {
				cmd.Flags().Bool(alias, false, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagStringArray, flagIntegerList, flagNumberList, flagBooleanList:
			cmd.Flags().StringSliceP(primary, spec.Shorthand, nil, usage)
			if alias != "" {
				cmd.Flags().StringSlice(alias, nil, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		}
	}
}

func collectOverrides(cmd *cobra.Command, specs []FlagSpec, guard *StdinGuard) (map[string]any, error) {
	overrides := make(map[string]any)
	for _, spec := range specs {
		flagName := strings.TrimSpace(spec.FlagName)
		if alias := strings.TrimSpace(spec.Alias); alias != "" && cobracmd.FlagChanged(cmd, alias) {
			flagName = alias
		}
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil || !flag.Changed {
			continue
		}

		switch spec.Kind {
		case flagString:
			value, err := cmd.Flags().GetString(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			// Resolve @file / @- for all string-typed flags.
			resolved, resolveErr := ResolveInputSource(value, flagName, guard)
			if resolveErr != nil {
				return nil, resolveErr
			}
			overrides[spec.PropertyName] = resolved
		case flagJSON:
			value, err := cmd.Flags().GetString(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			var parsed any
			if jsonErr := json.Unmarshal([]byte(value), &parsed); jsonErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid JSON for --%s: %v", flagName, jsonErr))
			}
			overrides[spec.PropertyName] = parsed
		case flagInteger:
			value, err := cmd.Flags().GetInt(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			overrides[spec.PropertyName] = value
		case flagNumber:
			value, err := cmd.Flags().GetFloat64(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			overrides[spec.PropertyName] = value
		case flagBoolean:
			value, err := cmd.Flags().GetBool(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			overrides[spec.PropertyName] = value
		case flagStringArray:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			overrides[spec.PropertyName] = convert.StringsToAny(value)
		case flagIntegerList:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			parsed, parseErr := convert.ParseStringList(value, strconv.Atoi)
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
			}
			overrides[spec.PropertyName] = convert.IntsToAny(parsed)
		case flagNumberList:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			parsed, parseErr := convert.ParseStringList(value, func(raw string) (float64, error) {
				return strconv.ParseFloat(raw, 64)
			})
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
			}
			overrides[spec.PropertyName] = convert.FloatsToAny(parsed)
		case flagBooleanList:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			parsed, parseErr := convert.ParseStringList(value, strconv.ParseBool)
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
			}
			overrides[spec.PropertyName] = convert.BoolsToAny(parsed)
		}
	}
	return overrides, nil
}

func schemaPayload(catalog ir.Catalog, args []string) (map[string]any, error) {
	if len(args) == 0 {
		products := make([]map[string]any, 0, len(catalog.Products))
		for _, p := range catalog.Products {
			tools := make([]map[string]any, 0, len(p.Tools))
			for _, t := range p.Tools {
				tools = append(tools, compactTool(t))
			}
			products = append(products, map[string]any{
				"id":          p.ID,
				"name":        p.DisplayName,
				"description": p.Description,
				"tools":       tools,
			})
		}
		return map[string]any{
			"kind":     "schema",
			"count":    len(products),
			"products": products,
		}, nil
	}

	product, tool, ok := catalog.FindTool(args[0])
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("unknown canonical schema path %q", args[0]))
	}
	return map[string]any{
		"kind":    "schema",
		"path":    args[0],
		"product": map[string]any{"id": product.ID, "name": product.DisplayName},
		"tool":    compactTool(tool),
	}, nil
}

// compactTool returns a lean representation of a tool for schema
// output, keeping only the fields that AI agents and developers
// need: name, description, parameters, and sensitivity flag.
func compactTool(t ir.ToolDescriptor) map[string]any {
	tool := map[string]any{
		"name":        t.RPCName,
		"title":       t.Title,
		"description": t.Description,
		"sensitive":   t.Sensitive,
	}

	if props, ok := t.InputSchema["properties"]; ok {
		tool["parameters"] = props
	}
	if req := requiredFields(t.InputSchema); len(req) > 0 {
		tool["required"] = req
	}

	return tool
}

func confirmSensitiveTool(cmd *cobra.Command, tool ir.ToolDescriptor, guard *StdinGuard) error {
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

	// Stdin was consumed for data input — interactive confirmation is impossible.
	if guard != nil && guard.Claimed() {
		return apperrors.NewValidation(
			"stdin used for data input; pass --yes to confirm sensitive operation",
		)
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

func requiredFields(schema map[string]any) []string {
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	fields := make([]string, 0, len(raw))
	for _, entry := range raw {
		value, ok := entry.(string)
		if ok && value != "" {
			fields = append(fields, value)
		}
	}
	return fields
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
