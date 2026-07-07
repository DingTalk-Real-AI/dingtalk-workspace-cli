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
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/convert"
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

// NewMCPCommand returns a stub command since the canonical discovery
// surface has been removed. The command tree is now built from plugins
// and static endpoint registration only.
func NewMCPCommand(_ context.Context, _ CatalogLoader, _ executor.Runner, _ *pipeline.Engine) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "mcp",
		Short:             "Canonical MCP-derived CLI surface (static mode)",
		Long:              "The canonical MCP command surface is disabled. Commands are now registered via plugins and static endpoints.",
		Hidden:            true,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	return cmd
}

// NewSchemaCommand returns a stub schema command since the canonical
// catalog discovery has been removed.
func NewSchemaCommand(_ CatalogLoader, helperTools HelperToolFetcher) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema [path]",
		Short: "查看有限的本地 Schema（静态端点模式）",
		Long: `查看有限的本地 Schema 元数据。

服务发现和动态 schema 已下线。静态端点模式下，仅支持 helper-only 子树的 schema 查询；普通产品命令和 flag 以当前二进制的 --help 为准。`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliPath, _ := cmd.Flags().GetString("cli-path")
			cliPath = strings.TrimSpace(cliPath)
			if cliPath != "" {
				if len(args) > 0 {
					return apperrors.NewValidation("--cli-path and positional argument are mutually exclusive")
				}
				args = []string{cliPath}
			}

			// Helper-only subtrees support.
			if len(args) > 0 && helperTools != nil {
				payload, ok, err := renderHelperSchema(cmd.Context(), cmd.Root(), args[0], helperTools)
				if err != nil {
					return err
				}
				if ok {
					data, _ := json.MarshalIndent(payload, "", "  ")
					fmt.Fprintln(cmd.OutOrStdout(), string(data))
					return nil
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), `{"kind":"schema","count":0,"products":[],"note":"static endpoint mode"}`)
			return nil
		},
	}
	cmd.Flags().String("cli-path", "", "按 CLI 命令路径查询 (等同于位置参数，便于脚本使用无需转义)")
	return cmd
}

func BuildFlagSpecs(schema map[string]any, hints map[string]CLIFlagHint) []FlagSpec {
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

// canRegisterToolFlag reports whether a long flag named name can be
// registered on cmd without panicking pflag ("flag redefined").
func canRegisterToolFlag(cmd *cobra.Command, name string) bool {
	if name == "" || name == "json" || name == "params" {
		return false
	}
	return cmd.Flags().Lookup(name) == nil
}

// safeToolShorthand returns short when it is a single-character shorthand not
// yet bound on cmd; otherwise "" (drop the shorthand, keep the long flag).
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

// splitSchemaPathTokens splits a CLI path on dots, slashes, and
// whitespace, returning only non-empty tokens.
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
