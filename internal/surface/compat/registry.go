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

package compat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/convert"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/output"
	"github.com/spf13/cobra"
)

type ValueKind string

const (
	ValueString      ValueKind = "string"
	ValueInt         ValueKind = "int"
	ValueFloat       ValueKind = "float"
	ValueBool        ValueKind = "bool"
	ValueStringSlice ValueKind = "string_slice"
	ValueIntSlice    ValueKind = "int_slice"
	ValueFloatSlice  ValueKind = "float_slice"
	ValueBoolSlice   ValueKind = "bool_slice"
	ValueJSON        ValueKind = "json"
)

type Target struct {
	CanonicalProduct string
	Tool             string
}

type FlagBinding struct {
	FlagName string
	Alias    string
	Short    string
	Property string
	Kind     ValueKind
	Usage    string
	Required bool
}

type Normalizer func(cmd *cobra.Command, params map[string]any) error

type Route struct {
	Use        string
	Aliases    []string
	Short      string
	Long       string
	Example    string
	Hidden     bool
	Target     Target
	Bindings   []FlagBinding
	Normalizer Normalizer
}

type CommandFactory func(runner executor.Runner) *cobra.Command

var (
	registryMu        sync.Mutex
	publicFactories   []CommandFactory
	fallbackFactories []CommandFactory
)

func RegisterPublic(factory CommandFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	publicFactories = append(publicFactories, factory)
}

func RegisterFallback(factory CommandFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	fallbackFactories = append(fallbackFactories, factory)
}

func NewPublicCommands(runner executor.Runner) []*cobra.Command {
	return buildFactories(publicFactories, runner)
}

func NewFallbackCommands(runner executor.Runner) []*cobra.Command {
	return buildFactories(fallbackFactories, runner)
}

// NewGroupCommand delegates to cobracmd.NewGroupCommand for backward compatibility.
var NewGroupCommand = cobracmd.NewGroupCommand

func NewDirectCommand(route Route, runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               route.Use,
		Aliases:           append([]string(nil), route.Aliases...),
		Short:             route.Short,
		Long:              route.Long,
		Example:           route.Example,
		Hidden:            route.Hidden,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonPayload, err := cmd.Flags().GetString("json")
			if err != nil {
				return apperrors.NewInternal("failed to read --json")
			}
			paramsPayload, err := cmd.Flags().GetString("params")
			if err != nil {
				return apperrors.NewInternal("failed to read --params")
			}
			baseParams, err := executor.MergePayloads(jsonPayload, paramsPayload, nil)
			if err != nil {
				return err
			}

			bindingParams, err := CollectBindings(cmd, route.Bindings, baseParams)
			if err != nil {
				return err
			}
			params := baseParams
			for key, value := range bindingParams {
				params[key] = value
			}
			if route.Normalizer != nil {
				if err := route.Normalizer(cmd, params); err != nil {
					return err
				}
			}
			if blocked, _ := params["_blocked"].(bool); blocked {
				return nil
			}

			invocation := executor.NewCompatInvocation(
				cobracmd.LegacyCommandPath(cmd),
				route.Target.CanonicalProduct,
				route.Target.Tool,
				params,
			)
			if dryRun, _ := cmd.Root().PersistentFlags().GetBool("dry-run"); dryRun {
				invocation.DryRun = true
			}
			result, err := runner.Run(cmd.Context(), invocation)
			if err != nil {
				return err
			}
			return output.WriteCommandPayload(cmd, result, output.FormatJSON)
		},
	}

	ApplyBindings(cmd, route.Bindings)
	return cmd
}

// NewCuratedCommand creates a DirectCommand with override priority so it wins
// over auto-generated MCP overlay commands during command tree merging.
func NewCuratedCommand(route Route, runner executor.Runner) *cobra.Command {
	cmd := NewDirectCommand(route, runner)
	cli.SetOverridePriority(cmd, 100)
	return cmd
}

func ApplyBindings(cmd *cobra.Command, bindings []FlagBinding) {
	for _, binding := range bindings {
		primary := strings.TrimSpace(binding.FlagName)
		if primary == "" {
			continue
		}
		alias := strings.TrimSpace(binding.Alias)
		if alias == primary {
			alias = ""
		}

		switch binding.Kind {
		case ValueString:
			cmd.Flags().StringP(primary, binding.Short, "", binding.Usage)
			if alias != "" {
				cmd.Flags().String(alias, "", binding.Usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case ValueInt:
			cmd.Flags().IntP(primary, binding.Short, 0, binding.Usage)
			if alias != "" {
				cmd.Flags().Int(alias, 0, binding.Usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case ValueFloat:
			cmd.Flags().Float64P(primary, binding.Short, 0, binding.Usage)
			if alias != "" {
				cmd.Flags().Float64(alias, 0, binding.Usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case ValueBool:
			cmd.Flags().BoolP(primary, binding.Short, false, binding.Usage)
			if alias != "" {
				cmd.Flags().Bool(alias, false, binding.Usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case ValueStringSlice, ValueIntSlice, ValueFloatSlice, ValueBoolSlice:
			cmd.Flags().StringSliceP(primary, binding.Short, nil, binding.Usage)
			if alias != "" {
				cmd.Flags().StringSlice(alias, nil, binding.Usage+" (alias)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		case ValueJSON:
			cmd.Flags().StringP(primary, binding.Short, "", binding.Usage+" (JSON)")
			if alias != "" {
				cmd.Flags().String(alias, "", binding.Usage+" (alias, JSON)")
				_ = cmd.Flags().MarkHidden(alias)
			}
		}
	}
	cmd.Flags().String("json", "", "Base JSON object payload for this command")
	cmd.Flags().String("params", "", "Additional JSON object payload merged after --json")
	_ = cmd.Flags().MarkHidden("json")
	_ = cmd.Flags().MarkHidden("params")
}

func CollectBindings(cmd *cobra.Command, bindings []FlagBinding, existing map[string]any) (map[string]any, error) {
	if existing == nil {
		existing = map[string]any{}
	}
	params := make(map[string]any)
	for _, binding := range bindings {
		primaryName := strings.TrimSpace(binding.FlagName)
		if primaryName == "" {
			continue
		}
		aliasName := strings.TrimSpace(binding.Alias)
		primaryChanged := cobracmd.FlagChanged(cmd, primaryName)
		aliasChanged := aliasName != "" && cobracmd.FlagChanged(cmd, aliasName)

		flagName := primaryName
		if aliasChanged {
			flagName = aliasName
		}

		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			continue
		}
		if binding.Required && !primaryChanged && !aliasChanged {
			if _, ok := existing[binding.Property]; ok {
				continue
			}
			return nil, apperrors.NewValidation(fmt.Sprintf("--%s is required", primaryName))
		}
		if !primaryChanged && !aliasChanged {
			continue
		}

		switch binding.Kind {
		case ValueString:
			value, err := cmd.Flags().GetString(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			params[binding.Property] = value
		case ValueJSON:
			value, err := cmd.Flags().GetString(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			var parsed any
			if jsonErr := json.Unmarshal([]byte(value), &parsed); jsonErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid JSON for --%s: %v", flagName, jsonErr))
			}
			params[binding.Property] = parsed
		case ValueInt:
			value, err := cmd.Flags().GetInt(flagName)
			if err != nil {
				// Flag may be wrapped by typedValue (display-only override);
				// fall back to parsing the raw string representation.
				raw := flag.Value.String()
				parsed, parseErr := strconv.Atoi(strings.TrimSpace(raw))
				if parseErr != nil {
					return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
				}
				params[binding.Property] = parsed
				continue
			}
			params[binding.Property] = value
		case ValueFloat:
			value, err := cmd.Flags().GetFloat64(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			params[binding.Property] = value
		case ValueBool:
			value, err := cmd.Flags().GetBool(flagName)
			if err != nil {
				return nil, apperrors.NewInternal(fmt.Sprintf("failed to read --%s", flagName))
			}
			params[binding.Property] = value
		case ValueStringSlice:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				// Flag may be wrapped by typedValue (display-only override);
				// fall back to reading the raw string and splitting by comma.
				raw := strings.TrimSpace(flag.Value.String())
				// pflag StringSlice wraps values in [brackets]
				raw = strings.TrimPrefix(raw, "[")
				raw = strings.TrimSuffix(raw, "]")
				var parts []string
				for _, s := range strings.Split(raw, ",") {
					t := strings.TrimSpace(s)
					// pflag StringSlice internally quotes each element
					t = strings.Trim(t, "\"")
					t = strings.TrimSpace(t)
					if t != "" {
						parts = append(parts, t)
					}
				}
				params[binding.Property] = convert.StringsToAny(parts)
				continue
			}
			params[binding.Property] = convert.StringsToAny(value)
		case ValueIntSlice:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				// Fallback: parse raw string
				raw := strings.TrimSpace(flag.Value.String())
				raw = strings.TrimPrefix(raw, "[")
				raw = strings.TrimSuffix(raw, "]")
				value = nil
				for _, s := range strings.Split(raw, ",") {
					if t := strings.TrimSpace(s); t != "" {
						value = append(value, t)
					}
				}
			}
			parsed, parseErr := convert.ParseStringList(value, strconv.Atoi)
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
			}
			params[binding.Property] = convert.IntsToAny(parsed)
		case ValueFloatSlice:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				// Fallback: parse raw string
				raw := strings.TrimSpace(flag.Value.String())
				raw = strings.TrimPrefix(raw, "[")
				raw = strings.TrimSuffix(raw, "]")
				value = nil
				for _, s := range strings.Split(raw, ",") {
					if t := strings.TrimSpace(s); t != "" {
						value = append(value, t)
					}
				}
				_ = err // clear error after fallback
			}
			parsed, parseErr := convert.ParseStringList(value, func(raw string) (float64, error) {
				return strconv.ParseFloat(raw, 64)
			})
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
			}
			params[binding.Property] = convert.FloatsToAny(parsed)
		case ValueBoolSlice:
			value, err := cmd.Flags().GetStringSlice(flagName)
			if err != nil {
				// Fallback: parse raw string
				raw := strings.TrimSpace(flag.Value.String())
				raw = strings.TrimPrefix(raw, "[")
				raw = strings.TrimSuffix(raw, "]")
				value = nil
				for _, s := range strings.Split(raw, ",") {
					if t := strings.TrimSpace(s); t != "" {
						value = append(value, t)
					}
				}
			}
			parsed, parseErr := convert.ParseStringList(value, strconv.ParseBool)
			if parseErr != nil {
				return nil, apperrors.NewValidation(fmt.Sprintf("invalid values for --%s: %v", flagName, parseErr))
			}
			params[binding.Property] = convert.BoolsToAny(parsed)
		}
	}
	return params, nil
}

func MustString(params map[string]any, key string) (string, error) {
	value, ok := params[key]
	if !ok {
		return "", apperrors.NewValidation(fmt.Sprintf("%s is required", key))
	}
	raw, ok := value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", apperrors.NewValidation(fmt.Sprintf("%s must be a non-empty string", key))
	}
	return raw, nil
}

func MoveParam(params map[string]any, from, to string) {
	value, ok := params[from]
	if !ok {
		return
	}
	delete(params, from)
	params[to] = value
}

func buildFactories(factories []CommandFactory, runner executor.Runner) []*cobra.Command {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make([]*cobra.Command, 0, len(factories))
	for _, factory := range factories {
		out = append(out, factory(runner))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Use < out[j].Use
	})
	return mergeRootCommands(out)
}

func mergeRootCommands(commands []*cobra.Command) []*cobra.Command {
	byName := make(map[string]*cobra.Command, len(commands))
	for _, cmd := range commands {
		if cmd == nil || cmd.Name() == "" {
			continue
		}
		if existing, ok := byName[cmd.Name()]; ok {
			cobracmd.MergeCommandTree(existing, cmd)
			continue
		}
		byName[cmd.Name()] = cmd
	}

	out := make([]*cobra.Command, 0, len(byName))
	for _, cmd := range byName {
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Use < out[j].Use
	})
	return out
}

// requireYesForDelete enforces --yes confirmation for destructive operations.
// If the user has not passed --yes, the command is blocked (params["_blocked"] = true).
func requireYesForDelete(cmd *cobra.Command, params map[string]any) error {
	yes, _ := cmd.Flags().GetBool("yes")
	delete(params, "_yes")
	if !yes {
		params["_blocked"] = true
		return nil
	}
	return nil
}

// compatFlagName converts a camelCase or snake_case parameter name to
// kebab-case suitable for CLI flags.
func compatFlagName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for idx, r := range raw {
		switch {
		case r == '_' || r == ' ' || r == '-':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		case unicode.IsUpper(r):
			if idx > 0 && !lastDash {
				builder.WriteByte('-')
			}
			builder.WriteRune(unicode.ToLower(r))
			lastDash = false
		default:
			builder.WriteRune(unicode.ToLower(r))
			lastDash = false
		}
	}
	return strings.Trim(builder.String(), "-")
}
