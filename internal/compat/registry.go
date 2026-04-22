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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/convert"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	// Aliases are additional hidden flag names that map to the same MCP
	// parameter. Any of them being set satisfies Required, and the value
	// is resolved via firstChangedFlag(FlagName, Alias, Aliases...).
	// Mirrors cmdutil.ValidateRequiredFlagWithAliases / FlagOrFallback.
	Aliases  []string
	Short    string
	Property string
	Kind     ValueKind
	Usage    string
	Required bool
	// Positional binds this parameter to a positional CLI argument rather
	// than a --flag. PositionalIndex is the 0-based slot.
	Positional      bool
	PositionalIndex int
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
	// OutputTransform, when non-nil, post-processes the MCP response payload
	// (rename / drop / columns) before the formatter emits it. Wired up from
	// CLIToolOverride.OutputFormat. See discovery-schema-v3 §2.5.
	OutputTransform func(map[string]any) map[string]any
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
	// Compute positional arity: cobra.Args requires knowing how many positional
	// slots are needed up-front. We use MinimumNArgs so trailing positionals
	// can still be captured via Use hints without breaking existing behaviour.
	positionalCount := 0
	hasPositional := false
	for _, b := range route.Bindings {
		if b.Positional && b.PositionalIndex+1 > positionalCount {
			positionalCount = b.PositionalIndex + 1
			hasPositional = true
		}
	}
	var argsValidator cobra.PositionalArgs = cobra.NoArgs
	if hasPositional {
		argsValidator = cobra.MinimumNArgs(positionalCount)
	}

	cmd := &cobra.Command{
		Use:               route.Use,
		Aliases:           append([]string(nil), route.Aliases...),
		Short:             route.Short,
		Long:              route.Long,
		Example:           route.Example,
		Hidden:            route.Hidden,
		Args:              argsValidator,
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

			// Inject positional args into params according to each binding's
			// PositionalIndex. Positional bindings are not registered as flags,
			// so CollectBindings skips them.
			if err := collectPositionalBindings(args, route.Bindings, params); err != nil {
				return err
			}

			// Collect schema-derived flags (from buildFlagsFromDetailSchema)
			// that are not covered by explicit bindings.
			collectSchemaFlags(cmd, route.Bindings, params)

			if route.Normalizer != nil {
				if err := route.Normalizer(cmd, params); err != nil {
					return err
				}
			}
			if blocked, _ := params["_blocked"].(bool); blocked {
				// Interactive confirmation for destructive operations (consistent with Helper commands)
				fmt.Fprintln(cmd.ErrOrStderr(), "⚠️  This is a destructive operation.")
				fmt.Fprint(cmd.ErrOrStderr(), "Confirm? (yes/no): ")

				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))

				if answer != "yes" && answer != "y" {
					fmt.Fprintln(cmd.ErrOrStderr(), "Operation cancelled")
					return nil
				}
				// User confirmed, continue execution
				delete(params, "_blocked")
			}

			invocation := executor.NewCompatibilityInvocation(
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
			if route.OutputTransform != nil && result.Response != nil {
				result.Response = route.OutputTransform(result.Response)
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
		// Positional bindings are collected from cobra args rather than flags.
		if binding.Positional {
			continue
		}
		primary := strings.TrimSpace(binding.FlagName)
		if primary == "" {
			continue
		}
		alias := strings.TrimSpace(binding.Alias)
		if alias == primary {
			alias = ""
		}
		// Dedupe extra aliases against primary + single alias and each other.
		var extras []string
		if len(binding.Aliases) > 0 {
			seen := map[string]bool{primary: true, "json": true, "params": true}
			if alias != "" {
				seen[alias] = true
			}
			extras = make([]string, 0, len(binding.Aliases))
			for _, a := range binding.Aliases {
				a = strings.TrimSpace(a)
				if a == "" || seen[a] {
					continue
				}
				seen[a] = true
				extras = append(extras, a)
			}
		}

		registerHidden := func(name string, suffix string) {
			if name == "" {
				return
			}
			switch binding.Kind {
			case ValueString:
				cmd.Flags().String(name, "", binding.Usage+suffix)
			case ValueInt:
				cmd.Flags().Int(name, 0, binding.Usage+suffix)
			case ValueFloat:
				cmd.Flags().Float64(name, 0, binding.Usage+suffix)
			case ValueBool:
				cmd.Flags().Bool(name, false, binding.Usage+suffix)
			case ValueStringSlice, ValueIntSlice, ValueFloatSlice, ValueBoolSlice:
				cmd.Flags().StringSlice(name, nil, binding.Usage+suffix)
			case ValueJSON:
				cmd.Flags().String(name, "", binding.Usage+suffix)
			}
			_ = cmd.Flags().MarkHidden(name)
		}

		switch binding.Kind {
		case ValueString:
			cmd.Flags().StringP(primary, binding.Short, "", binding.Usage)
		case ValueInt:
			cmd.Flags().IntP(primary, binding.Short, 0, binding.Usage)
		case ValueFloat:
			cmd.Flags().Float64P(primary, binding.Short, 0, binding.Usage)
		case ValueBool:
			cmd.Flags().BoolP(primary, binding.Short, false, binding.Usage)
		case ValueStringSlice, ValueIntSlice, ValueFloatSlice, ValueBoolSlice:
			cmd.Flags().StringSliceP(primary, binding.Short, nil, binding.Usage)
		case ValueJSON:
			cmd.Flags().StringP(primary, binding.Short, "", binding.Usage+" (JSON)")
		}
		registerHidden(alias, " (alias)")
		for _, extra := range extras {
			registerHidden(extra, " (alias)")
		}
		if binding.Required {
			// When no hidden aliases exist, lean on cobra's native required
			// validation for the best UX (colored error, shown in --help).
			// When aliases exist, CollectBindings does its own "any-of-these
			// is set" check so users who type the hidden alias do not hit
			// cobra yelling about the primary being missing.
			if alias == "" && len(extras) == 0 {
				_ = cmd.MarkFlagRequired(primary)
			}
		}
	}
	cmd.Flags().String("json", "", "Base JSON object payload for this command")
	cmd.Flags().String("params", "", "Additional JSON object payload merged after --json")
	_ = cmd.Flags().MarkHidden("json")
	_ = cmd.Flags().MarkHidden("params")
}

// collectPositionalBindings pulls positional args according to the bindings
// and injects them into params[property]. Missing slots are skipped (cobra
// arity validation already ran before RunE).
func collectPositionalBindings(args []string, bindings []FlagBinding, params map[string]any) error {
	for _, binding := range bindings {
		if !binding.Positional {
			continue
		}
		property := strings.TrimSpace(binding.Property)
		if property == "" {
			continue
		}
		if binding.PositionalIndex < 0 || binding.PositionalIndex >= len(args) {
			continue
		}
		raw := args[binding.PositionalIndex]
		switch binding.Kind {
		case ValueInt:
			v, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("positional argument %d (%s) must be int", binding.PositionalIndex, property))
			}
			params[property] = v
		case ValueFloat:
			v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("positional argument %d (%s) must be float", binding.PositionalIndex, property))
			}
			params[property] = v
		case ValueBool:
			v, err := strconv.ParseBool(strings.TrimSpace(raw))
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("positional argument %d (%s) must be bool", binding.PositionalIndex, property))
			}
			params[property] = v
		default:
			params[property] = raw
		}
	}
	return nil
}

// collectSchemaFlags picks up flags created by buildFlagsFromDetailSchema that
// have no explicit FlagBinding. This bridges the gap for plugin-defined tools
// whose parameters come from the MCP inputSchema rather than CLIToolOverride.Flags.
func collectSchemaFlags(cmd *cobra.Command, bindings []FlagBinding, params map[string]any) {
	// Build a set of flag names already covered by bindings.
	bound := make(map[string]bool, len(bindings)*2)
	for _, b := range bindings {
		if n := strings.TrimSpace(b.FlagName); n != "" {
			bound[n] = true
		}
		if a := strings.TrimSpace(b.Alias); a != "" {
			bound[a] = true
		}
		for _, extra := range b.Aliases {
			if e := strings.TrimSpace(extra); e != "" {
				bound[e] = true
			}
		}
	}

	// Reserved/internal flags that should never be forwarded as tool params.
	skip := map[string]bool{
		"json": true, "params": true, "help": true,
		"format": true, "fields": true, "jq": true,
		"debug": true, "verbose": true, "dry-run": true,
		"yes": true, "mock": true, "timeout": true,
		"client-id": true, "client-secret": true,
	}

	cmd.Flags().Visit(func(f *pflag.Flag) {
		if bound[f.Name] || skip[f.Name] {
			return
		}
		// Convert flag name back to the original parameter name (kebab → snake/camel)
		// For simplicity, use the flag name as-is since MCP tools typically
		// use snake_case which maps to kebab-case flags.
		paramName := toOriginalParamName(f.Name)
		if _, exists := params[paramName]; exists {
			return // already set by --json/--params
		}

		switch f.Value.Type() {
		case "int":
			if v, err := cmd.Flags().GetInt(f.Name); err == nil {
				params[paramName] = v
			}
		case "bool":
			if v, err := cmd.Flags().GetBool(f.Name); err == nil {
				params[paramName] = v
			}
		case "stringSlice":
			if v, err := cmd.Flags().GetStringSlice(f.Name); err == nil {
				params[paramName] = v
			}
		default:
			if v, err := cmd.Flags().GetString(f.Name); err == nil {
				params[paramName] = v
			}
		}
	})
}

// toOriginalParamName converts a kebab-case flag name back to the original
// MCP parameter name. Since toKebabCase converts both camelCase and snake_case
// to kebab-case, we default to snake_case (the MCP convention).
func toOriginalParamName(flagName string) string {
	return strings.ReplaceAll(flagName, "-", "_")
}

// firstChangedFlag returns the first name (in order) whose cobra flag has
// been set by the user. Whitespace-only or empty entries are skipped.
// Mirrors wukong cmdutil.FlagOrFallback precedence: primary > alias >
// extraAliases in declaration order.
func firstChangedFlag(cmd *cobra.Command, names ...string) (name string, changed bool) {
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if cobracmd.FlagChanged(cmd, n) {
			return n, true
		}
	}
	return "", false
}

func CollectBindings(cmd *cobra.Command, bindings []FlagBinding, existing map[string]any) (map[string]any, error) {
	if existing == nil {
		existing = map[string]any{}
	}
	params := make(map[string]any)
	for _, binding := range bindings {
		if binding.Positional {
			continue
		}
		primaryName := strings.TrimSpace(binding.FlagName)
		if primaryName == "" {
			continue
		}
		aliasName := strings.TrimSpace(binding.Alias)

		// Candidate flag names in precedence order: primary, single alias,
		// then extra aliases. Whichever is set first wins; mirrors the
		// semantics of cmdutil.FlagOrFallback.
		candidates := make([]string, 0, 2+len(binding.Aliases))
		candidates = append(candidates, primaryName)
		if aliasName != "" && aliasName != primaryName {
			candidates = append(candidates, aliasName)
		}
		for _, extra := range binding.Aliases {
			e := strings.TrimSpace(extra)
			if e == "" || e == primaryName || e == aliasName {
				continue
			}
			candidates = append(candidates, e)
		}

		flagName, anyChanged := firstChangedFlag(cmd, candidates...)
		if !anyChanged {
			flagName = primaryName
		}

		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			continue
		}
		if binding.Required && !anyChanged {
			if _, ok := existing[binding.Property]; ok {
				continue
			}
			return nil, apperrors.NewValidation(fmt.Sprintf("--%s is required", primaryName))
		}
		if !anyChanged {
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
