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
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/convert"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func NewSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema [path]",
		Short: "渐进查看 MCP 工具 Schema (产品 / 分组 / 工具参数)",
		Long: `查看当前可运行命令的 Schema 元数据。

	不带参数时只列出产品和工具数量；传产品或分组路径时逐层展开工具摘要；
	传具体工具路径时输出扁平参数 Schema。使用 --all 可一次输出完整目录。
	schema 来自实际运行命令的 Cobra flags、hardcoded metadata 和版本内
	内嵌 JSON；查询 schema 不执行 MCP tools/list 服务发现。

路径支持三种写法：
  product                    产品路径 (e.g. calendar)
  product.group              分组路径 (e.g. calendar.event)
  product.rpc_name           规范路径 (e.g. ding.send_ding_message)
  product.group.cli_name     CLI 点路径 (e.g. ding.message.send)
  "product group cli_name"   CLI 空格/斜杠路径 (e.g. "ding message send")

	示例:
	  dws schema                                # 紧凑产品概览
	  dws schema list                           # 紧凑产品概览（兼容旧入口）
	  dws schema calendar                       # 展开一个产品
	  dws schema calendar.event                 # 展开一个分组
	  dws schema ding.send_ding_message         # 规范路径
	  dws schema "ding message send"            # CLI 路径（空格）
	  dws schema --all                          # 完整产品 + 工具目录
	  dws schema --cli-path "ding message send" # 同上，显式 flag（脚本友好）
	  dws schema ding.send_ding_message --jq '.parameters'
	  dws schema -f pretty ding.send_ding_message  # ANSI 彩色分区展示
	  dws schema --jq '.products[] | {id, tool_count}'

helper-only 命令组（如 dev）也支持查询，输出对齐 gws 的扁平格式
（parameters 内联 required，键为 CLI flag）：
  dws schema "dev app robot config"         # 版本内 helper 参数 schema
  dws schema "dev app"                      # 列出该分组下的子命令`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			cliPath, _ := cmd.Flags().GetString("cli-path")
			cliPath = strings.TrimSpace(cliPath)
			if cliPath != "" && len(args) > 0 {
				return apperrors.NewValidation("--cli-path and positional argument are mutually exclusive")
			}

			// `schema list` was used by an earlier schema browser. Keep it as
			// a stable compatibility alias for the root overview, rather than
			// treating "list" as a product path.
			if len(args) == 1 && strings.EqualFold(strings.TrimSpace(args[0]), "list") {
				args = nil
			}
			if all && (cliPath != "" || len(args) > 0) {
				return apperrors.NewValidation("--all cannot be combined with a schema path")
			}
			if cliPath != "" {
				args = []string{cliPath}
			}

			// Helper-only subtrees (e.g. `dws dev ...`) are rendered from the
			// executable Cobra surface and embedded metadata. Schema inspection must
			// never trigger MCP initialize/tools-list discovery.
			if len(args) > 0 {
				payload, ok, err := renderHelperSchema(cmd.Root(), args[0])
				if err != nil {
					return err
				}
				if ok {
					return output.WriteFiltered(
						cmd.OutOrStdout(),
						output.ResolveFormat(cmd, output.FormatJSON),
						payload,
						output.ResolveFields(cmd),
						output.ResolveJQ(cmd),
					)
				}
			}

			payload, err := runtimeSchemaPayload(cmd.Root(), args)
			if err != nil {
				return err
			}

			// Append helper-only subtrees (e.g. `dev`) to the no-arg product
			// listing so browsing all products also surfaces helper commands.
			if len(args) == 0 {
				if helpers := helperProductSummaries(cmd.Root()); len(helpers) > 0 {
					if products, ok := payload["products"].([]map[string]any); ok {
						payload["products"] = append(products, helpers...)
						payload["count"] = len(payload["products"].([]map[string]any))
						payload["tool_count"] = schemaCatalogToolCount(payload["products"].([]map[string]any))
					}
				}
				if !all {
					payload = compactSchemaOverviewPayload(payload)
				}
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
	cmd.Flags().String("cli-path", "", "按 CLI 命令路径查询 (等同于位置参数，便于脚本使用无需转义)")
	cmd.Flags().Bool("all", false, "输出全部产品和工具摘要（用于审计/CI，内容较大）")
	return cmd
}

func schemaCatalogToolCount(products []map[string]any) int {
	total := 0
	for _, product := range products {
		total += schemaProductToolCount(product)
	}
	return total
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
	aliases := make([]string, 0, 2)
	seenAlias := map[string]bool{product.ID: true}
	addAlias := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seenAlias[s] {
			return
		}
		seenAlias[s] = true
		aliases = append(aliases, s)
	}
	if preferred := preferredProductRouteToken(product); preferred != "" {
		addAlias(preferred)
	}
	// Consume only cli.Aliases (canonical alternate-name field).
	// cli.Prefixes is the tool-name-prefix pool consumed by deriveCommandName;
	// treating prefixes[1:] as aliases over-registers names the wukong edition
	// does not expose, breaking cross-edition parity.
	if product.CLI != nil {
		for _, a := range product.CLI.Aliases {
			addAlias(a)
		}
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

	// Register phase: notify the pipeline that a product and its
	// tools have been added to the command tree. This runs once at
	// startup (not per-request) and enables handlers to inspect or
	// enrich the registered command surface.
	if engine != nil && engine.HasHandlers(pipeline.Register) {
		pctx := &pipeline.Context{
			Command: product.ID,
		}
		// Best-effort — registration errors are logged but do not
		// prevent the CLI from starting.
		if pipeErr := engine.RunPhase(pipeline.Register, pctx); pipeErr != nil {
			slog.Debug("pipeline register phase", "product", product.ID, "error", pipeErr)
		} else {
			slog.Debug("pipeline register",
				"product", product.ID,
				"tool_count", len(product.Tools),
			)
		}
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
			dryRun := commandBoolFlag(cmd, "dry-run")

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
				for _, c := range pctx.Corrections {
					slog.Debug("pipeline correction",
						"phase", "post-parse",
						"handler", c.Handler,
						"kind", c.Kind,
						"field", c.Field,
						"original", c.Original,
						"corrected", c.Corrected,
					)
				}
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
				slog.Debug("pipeline pre-request",
					"command", tool.CanonicalPath,
					"param_count", len(params),
				)
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
				slog.Debug("pipeline post-response",
					"command", tool.CanonicalPath,
					"has_response", result.Response != nil,
				)
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

func commandBoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil || strings.TrimSpace(name) == "" {
		return false
	}
	var rootFlags *pflag.FlagSet
	if root := cmd.Root(); root != nil {
		rootFlags = root.PersistentFlags()
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags(), rootFlags} {
		if flags == nil || flags.Lookup(name) == nil {
			continue
		}
		value, err := flags.GetBool(name)
		return err == nil && value
	}
	return false
}

// canRegisterToolFlag reports whether a long flag named name can be
// registered on cmd without panicking pflag ("flag redefined"). The reserved
// payload names are excluded too: newToolCommand unconditionally registers
// --json/--params before the spec loop. Tool schemas are remote data — a
// property named after a reserved or already-registered flag must degrade to
// "flag unavailable" (the value stays reachable through --json/--params),
// never abort the process. Mirrors internal/compat's canRegisterFlag.
func canRegisterToolFlag(cmd *cobra.Command, name string) bool {
	if name == "" || name == "json" || name == "params" {
		return false
	}
	return cmd.Flags().Lookup(name) == nil
}

// safeToolShorthand returns short when it is a single-character shorthand not
// yet bound on cmd; otherwise "" (drop the shorthand, keep the long flag).
// pflag panics on both multi-character and duplicate shorthands.
func safeToolShorthand(cmd *cobra.Command, short string) string {
	short = strings.TrimSpace(short)
	if len(short) != 1 {
		return ""
	}
	if cmd.Flags().ShorthandLookup(short) != nil {
		return ""
	}
	return short
}

func applyFlagSpecs(cmd *cobra.Command, specs []FlagSpec) {
	for _, spec := range specs {
		usage := spec.Description
		if usage == "" {
			usage = fmt.Sprintf("Override %s", spec.PropertyName)
		}
		primary := strings.TrimSpace(spec.FlagName)
		if !canRegisterToolFlag(cmd, primary) {
			continue
		}
		shorthand := safeToolShorthand(cmd, spec.Shorthand)
		alias := strings.TrimSpace(spec.Alias)
		if alias == primary || !canRegisterToolFlag(cmd, alias) {
			alias = ""
		}

		switch spec.Kind {
		case flagString, flagJSON:
			cmd.Flags().StringP(primary, shorthand, "", usage)
			if alias != "" {
				cmd.Flags().String(alias, "", usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagInteger:
			cmd.Flags().IntP(primary, shorthand, 0, usage)
			if alias != "" {
				cmd.Flags().Int(alias, 0, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagNumber:
			cmd.Flags().Float64P(primary, shorthand, 0, usage)
			if alias != "" {
				cmd.Flags().Float64(alias, 0, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagBoolean:
			cmd.Flags().BoolP(primary, shorthand, false, usage)
			if alias != "" {
				cmd.Flags().Bool(alias, false, usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case flagStringArray, flagIntegerList, flagNumberList, flagBooleanList:
			cmd.Flags().StringSliceP(primary, shorthand, nil, usage)
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

// FlatToolSchemaPayload renders a single Tool IR descriptor using the same
// gws-flat leaf shape as runtime schema commands. It is kept for offline
// generated artifacts; the interactive `dws schema` command uses runtime command
// annotations instead of enumerating Tool IR descriptors.
func FlatToolSchemaPayload(product ir.CanonicalProduct, tool ir.ToolDescriptor) map[string]any {
	payload := compactTool(tool)
	payload["path"] = tool.CanonicalPath
	payload["source"] = "mcp:" + product.ID
	payload["product_id"] = product.ID
	payload["display"] = product.DisplayName
	return payload
}

// splitSchemaPathTokens splits a CLI path on dots, slashes, and
// whitespace, returning only non-empty tokens. "ding message send",
// "ding.message.send", and "ding/message/send" all yield the same
// three tokens.
func splitSchemaPathTokens(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '.' || r == '/' || r == ' ' || r == '\t'
	})
	out := fields[:0]
	for _, f := range fields {
		if s := strings.TrimSpace(f); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// compactTool returns a lean representation of a tool for schema
// output, keeping the fields AI agents and scripts need: RPC + CLI
// identity, input/output schema, sensitivity, MCP annotations, and the
// CLI flag overlay (alias/transform/envDefault/default) that shapes
// how raw MCP parameters appear on the command line.
func compactTool(t ir.ToolDescriptor) map[string]any {
	hint := schemaHintForTool(t)
	title := t.Title
	if strings.TrimSpace(hint.Title) != "" {
		title = strings.TrimSpace(hint.Title)
	}
	description := t.Description
	if strings.TrimSpace(hint.Description) != "" {
		description = strings.TrimSpace(hint.Description)
	}
	tool := map[string]any{
		"name":           t.RPCName,
		"cli_name":       t.CLIName,
		"canonical_path": t.CanonicalPath,
		"title":          title,
		"description":    description,
		"sensitive":      t.Sensitive,
	}

	if strings.TrimSpace(t.Group) != "" {
		tool["group"] = t.Group
	}
	params := buildFlatSchemaParameters(t.InputSchema, t.FlagOverlay, hint.Parameters)
	if params == nil {
		params = map[string]any{}
	}
	tool["parameters"] = params
	tool["has_parameters"] = len(params) > 0
	tool["parameter_count"] = len(params)
	if req := requiredFields(t.InputSchema); len(req) > 0 {
		tool["required"] = req
	}
	if len(t.OutputSchema) > 0 {
		tool["output_schema"] = t.OutputSchema
	}
	if t.Annotations != nil {
		tool["annotations"] = t.Annotations
	}
	if t.Auth != nil {
		tool["auth"] = t.Auth
	}
	if len(t.FlagOverlay) > 0 {
		tool["flag_overlay"] = t.FlagOverlay
	}

	return tool
}

// BuildFlatSchemaParameters projects an MCP input JSON Schema into the same
// gws-flat parameter map used by helper schema rendering. Keys are the effective
// CLI flag names: explicit flag_overlay alias wins, otherwise the MCP parameter
// name is converted to kebab-case.
func BuildFlatSchemaParameters(schema map[string]any, overlay map[string]ir.FlagOverlay) map[string]any {
	return buildFlatSchemaParameters(schema, overlay, nil)
}

func buildFlatSchemaParameters(schema map[string]any, overlay map[string]ir.FlagOverlay, hints map[string]ParameterSchemaHint) map[string]any {
	properties, ok := nestedMap(schema, "properties")
	if !ok || len(properties) == 0 {
		return nil
	}

	required := map[string]bool{}
	for _, name := range requiredFields(schema) {
		required[name] = true
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make(map[string]any, len(keys))
	for _, name := range keys {
		prop, _ := properties[name].(map[string]any)
		flagName := kebabCase(name)
		if ov, ok := overlay[name]; ok && strings.TrimSpace(ov.Alias) != "" {
			flagName = strings.TrimSpace(ov.Alias)
		}

		hint, _, hasHint := lookupParameterSchemaHint(hints, name, flagName)
		if hasHint && strings.TrimSpace(hint.FlagName) != "" {
			flagName = strings.TrimSpace(hint.FlagName)
		}

		paramType := mcpJSONType(prop)
		if hasHint && strings.TrimSpace(hint.Type) != "" {
			paramType = strings.TrimSpace(hint.Type)
		}
		description := schemaDescription(prop)
		if ov, ok := overlay[name]; ok && strings.TrimSpace(ov.Description) != "" {
			description = strings.TrimSpace(ov.Description)
		}
		if hasHint && strings.TrimSpace(hint.Description) != "" {
			description = strings.TrimSpace(hint.Description)
		}
		isRequired := required[name]
		if hasHint && hint.Required != nil {
			isRequired = *hint.Required
		}

		entry := map[string]any{
			"type":        paramType,
			"description": description,
			"required":    isRequired,
		}
		if hasHint && strings.TrimSpace(hint.Default) != "" {
			entry["default"] = strings.TrimSpace(hint.Default)
		} else if ov, ok := overlay[name]; ok && strings.TrimSpace(ov.Default) != "" {
			entry["default"] = strings.TrimSpace(ov.Default)
		} else if def, ok := mcpDefault(prop); ok {
			entry["default"] = def
		}
		params[flagName] = entry
	}
	return params
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
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	title, _ := schema["title"].(string)
	return strings.TrimSpace(title)
}

func requiredFields(schema map[string]any) []string {
	switch raw := schema["required"].(type) {
	case []any:
		fields := make([]string, 0, len(raw))
		for _, entry := range raw {
			value, ok := entry.(string)
			if ok && value != "" {
				fields = append(fields, value)
			}
		}
		return fields
	case []string:
		fields := make([]string, 0, len(raw))
		for _, value := range raw {
			if value != "" {
				fields = append(fields, value)
			}
		}
		return fields
	default:
		return nil
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
