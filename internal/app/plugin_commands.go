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

package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type pluginFlagKind uint8

const (
	pluginFlagString pluginFlagKind = iota
	pluginFlagInt
	pluginFlagFloat
	pluginFlagBool
	pluginFlagStringSlice
	pluginFlagJSON
)

type pluginFlagBinding struct {
	property        string
	names           []string
	kind            pluginFlagKind
	required        bool
	defaultProvided bool
	defaultValue    string
	envDefault      string
	positional      bool
	positionalIndex int
	omitWhen        string
}

type pluginFlagReservations struct {
	names      map[string]bool
	shorthands map[string]bool
}

// buildPluginCommands compiles only manifest-authored CLI overlays. It never
// starts a transport or calls tools/list, so plugin help stays available when
// a local subprocess or remote MCP endpoint is offline.
func buildPluginCommands(
	servers []mcptypes.ServerDescriptor,
	runner executor.Runner,
	hostRoot *cobra.Command,
) []*cobra.Command {
	reservations := pluginReservedFlags(hostRoot)
	sorted := append([]mcptypes.ServerDescriptor(nil), servers...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := strings.TrimSpace(sorted[i].DisplayName) + "\x00" + strings.TrimSpace(sorted[i].Key)
		right := strings.TrimSpace(sorted[j].DisplayName) + "\x00" + strings.TrimSpace(sorted[j].Key)
		return left < right
	})

	roots := make(map[string]*cobra.Command)
	for _, server := range sorted {
		overlay := server.CLI
		if overlay.Skip || len(overlay.ToolOverrides) == 0 {
			continue
		}
		if reason := unsupportedPluginOverlay(overlay); reason != "" {
			slog.Warn("plugin: unsupported overlay semantics, skipping command tree",
				"plugin", server.DisplayName, "field", reason)
			continue
		}
		name := firstNonEmptyPluginString(overlay.Command, overlay.ID, server.Key)
		if !validPluginCommandName(name) {
			slog.Warn("plugin: invalid overlay command name, skipping",
				"plugin", server.DisplayName, "command", name)
			continue
		}

		root := buildPluginOverlayRoot(server, runner, reservations)
		if root == nil {
			continue
		}
		if existing := roots[name]; existing != nil {
			mergePluginRoot(existing, root)
			continue
		}
		roots[name] = root
	}

	names := make([]string, 0, len(roots))
	for name := range roots {
		names = append(names, name)
	}
	sort.Strings(names)
	commands := make([]*cobra.Command, 0, len(names))
	for _, name := range names {
		commands = append(commands, roots[name])
	}
	return commands
}

func buildPluginOverlayRoot(
	server mcptypes.ServerDescriptor,
	runner executor.Runner,
	reservations pluginFlagReservations,
) *cobra.Command {
	overlay := server.CLI
	name := firstNonEmptyPluginString(overlay.Command, overlay.ID, server.Key)
	short := firstNonEmptyPluginString(overlay.Description, server.Description, server.DisplayName, name)
	root := cobracmd.NewGroupCommand(name, short)
	root.Hidden = overlay.Hidden
	root.Aliases = safePluginAliases(overlay.Aliases, name)
	cmdutil.MarkPluginSource(root)

	groups := make(map[string]*cobra.Command)
	groupNames := make([]string, 0, len(overlay.Groups))
	for groupName := range overlay.Groups {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)
	for _, groupName := range groupNames {
		group := overlay.Groups[groupName]
		ensurePluginGroup(root, groupName, group.Description, groups)
	}

	toolNames := make([]string, 0, len(overlay.ToolOverrides))
	for toolName := range overlay.ToolOverrides {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)
	leafCount := 0
	for _, toolName := range toolNames {
		override := overlay.ToolOverrides[toolName]
		if override.Hidden || strings.TrimSpace(toolName) == "" {
			continue
		}
		leaf := buildPluginLeaf(server, toolName, override, runner, reservations)
		if leaf == nil {
			continue
		}
		parent := root
		if groupPath := strings.TrimSpace(override.Group); groupPath != "" {
			parent = ensurePluginGroup(root, groupPath, groupPath, groups)
			if parent == nil {
				slog.Warn("plugin: invalid overlay group path, skipping leaf",
					"plugin", server.DisplayName, "tool", toolName, "group", groupPath)
				continue
			}
		}
		if existing := cobracmd.ChildByName(parent, leaf.Name()); existing != nil {
			slog.Warn("plugin: duplicate overlay leaf, keeping first",
				"plugin", server.DisplayName,
				"command", strings.TrimSpace(parent.CommandPath()+" "+leaf.Name()))
			continue
		}
		parent.AddCommand(leaf)
		leafCount++
	}

	if leafCount == 0 {
		return nil
	}
	pruneEmptyPluginGroups(root)
	return root
}

func buildPluginLeaf(
	server mcptypes.ServerDescriptor,
	toolName string,
	override mcptypes.CLIToolOverride,
	runner executor.Runner,
	reservations pluginFlagReservations,
) *cobra.Command {
	overlay := server.CLI
	if reason := unsupportedPluginToolOverride(override); reason != "" {
		slog.Warn("plugin: unsupported tool override semantics, skipping leaf",
			"plugin", server.DisplayName, "tool", toolName, "field", reason)
		return nil
	}
	name := strings.TrimSpace(override.CLIName)
	if name == "" {
		name = derivePluginCommandName(toolName, overlay.Prefixes)
	}
	if !validPluginCommandName(name) {
		slog.Warn("plugin: invalid overlay leaf name, skipping",
			"plugin", server.DisplayName, "tool", toolName, "command", name)
		return nil
	}

	short := firstNonEmptyPluginString(override.Description, toolName)
	bindings, use, argsValidator, ok := registerPluginBindings(name, override, reservations)
	if !ok {
		return nil
	}
	if !validPluginFlagConstraints(bindings, override) {
		slog.Warn("plugin: invalid flag constraints, skipping leaf",
			"plugin", server.DisplayName, "tool", toolName)
		return nil
	}
	canonicalProduct := firstNonEmptyPluginString(overlay.ID, server.Key)
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Example:           strings.TrimRight(override.Example, " \t\r\n"),
		Args:              argsValidator,
		DisableAutoGenTag: true,
	}
	cmdutil.MarkPluginSource(cmd)

	registerPluginFlags(cmd, bindings, override, reservations)
	applyPluginFlagConstraints(cmd, override)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		jsonPayload, err := cmd.Flags().GetString("json")
		if err != nil {
			return apperrors.NewInternal("failed to read --json")
		}
		paramsPayload, err := cmd.Flags().GetString("params")
		if err != nil {
			return apperrors.NewInternal("failed to read --params")
		}
		params, err := executor.MergePayloads(jsonPayload, paramsPayload, nil)
		if err != nil {
			return err
		}
		if err := collectPluginBindings(cmd, args, bindings, params); err != nil {
			return err
		}
		if wrapper := strings.TrimSpace(override.BodyWrapper); wrapper != "" {
			wrapPluginParams(params, wrapper)
		}

		dryRun := pluginRootBoolFlag(cmd, "dry-run")
		if override.IsSensitive && !dryRun && !pluginRootBoolFlag(cmd, "yes") {
			return pluginConfirmationRequired(cmd.CommandPath())
		}
		if runner == nil {
			return apperrors.NewInternal("plugin command runner is not configured")
		}

		invocation := executor.NewCompatibilityInvocation(
			cobracmd.LegacyCommandPath(cmd),
			canonicalProduct,
			toolName,
			params,
		)
		invocation.DryRun = dryRun
		result, err := runner.Run(cmd.Context(), invocation)
		if err != nil {
			return err
		}
		return output.WriteCommandPayload(cmd, result, output.FormatJSON)
	}
	return cmd
}

func registerPluginBindings(
	name string,
	override mcptypes.CLIToolOverride,
	reservations pluginFlagReservations,
) ([]pluginFlagBinding, string, cobra.PositionalArgs, bool) {
	properties := make([]string, 0, len(override.Flags))
	for property := range override.Flags {
		properties = append(properties, property)
	}
	sort.Strings(properties)

	bindings := make([]pluginFlagBinding, 0, len(properties))
	usedNames := make(map[string]bool, len(reservations.names))
	for reserved := range reservations.names {
		usedNames[reserved] = true
	}
	usedPositions := make(map[int]bool)
	maxPositionals := 0
	minPositionals := 0
	type positionalLabel struct {
		index int
		name  string
	}
	var labels []positionalLabel

	for _, property := range properties {
		flagOverride := override.Flags[property]
		if strings.Contains(property, ".") {
			slog.Warn("plugin: dotted flag properties are unsupported, skipping leaf",
				"command", name, "property", property)
			return nil, "", nil, false
		}
		if reason := unsupportedPluginFlagOverride(flagOverride); reason != "" {
			slog.Warn("plugin: unsupported flag override semantics, skipping leaf",
				"command", name, "property", property, "field", reason)
			return nil, "", nil, false
		}
		primary := strings.TrimSpace(flagOverride.Alias)
		if primary == "" && !flagOverride.Positional {
			primary = pluginKebabName(property)
		}
		if !flagOverride.Positional &&
			(!validPluginFlagName(primary) || usedNames[primary]) {
			slog.Warn("plugin: primary overlay flag conflicts with the host CLI, skipping leaf",
				"command", name, "flag", primary, "property", property)
			return nil, "", nil, false
		}

		names := make([]string, 0, 1+len(flagOverride.Aliases))
		for _, candidate := range append([]string{primary}, flagOverride.Aliases...) {
			candidate = strings.TrimSpace(candidate)
			if !validPluginFlagName(candidate) || usedNames[candidate] {
				if candidate != "" {
					slog.Warn("plugin: duplicate or invalid overlay flag, skipping",
						"command", name, "flag", candidate, "property", property)
				}
				continue
			}
			usedNames[candidate] = true
			names = append(names, candidate)
		}

		if flagOverride.Positional {
			index := flagOverride.PositionalIndex
			if index < 0 || usedPositions[index] {
				slog.Warn("plugin: invalid or duplicate positional binding, skipping leaf",
					"command", name, "property", property, "index", index)
				return nil, "", nil, false
			}
			usedPositions[index] = true
			if index+1 > maxPositionals {
				maxPositionals = index + 1
			}
			if flagOverride.Required && len(names) == 0 && index+1 > minPositionals {
				minPositionals = index + 1
			}
			labels = append(labels, positionalLabel{index: index, name: property})
		}

		bindings = append(bindings, pluginFlagBinding{
			property:        property,
			names:           names,
			kind:            pluginFlagKindFromString(flagOverride.Type),
			required:        flagOverride.Required,
			defaultProvided: flagOverride.Default != "",
			defaultValue:    flagOverride.Default,
			envDefault:      strings.TrimSpace(flagOverride.EnvDefault),
			positional:      flagOverride.Positional,
			positionalIndex: flagOverride.PositionalIndex,
			omitWhen:        strings.ToLower(strings.TrimSpace(flagOverride.OmitWhen)),
		})
	}
	for index := 0; index < maxPositionals; index++ {
		if !usedPositions[index] {
			slog.Warn("plugin: positional bindings must use contiguous indexes, skipping leaf",
				"command", name, "missingIndex", index)
			return nil, "", nil, false
		}
	}

	use := name
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].index < labels[j].index })
	for _, label := range labels {
		use += " [" + label.name + "]"
	}

	var validator cobra.PositionalArgs = cobra.NoArgs
	switch {
	case maxPositionals == 0:
		validator = cobra.NoArgs
	case minPositionals == maxPositionals:
		validator = cobra.ExactArgs(maxPositionals)
	case minPositionals > 0:
		validator = cobra.RangeArgs(minPositionals, maxPositionals)
	default:
		validator = cobra.MaximumNArgs(maxPositionals)
	}
	return bindings, use, validator, true
}

func registerPluginFlags(
	cmd *cobra.Command,
	bindings []pluginFlagBinding,
	override mcptypes.CLIToolOverride,
	reservations pluginFlagReservations,
) {
	usedShorthands := make(map[string]bool)
	for _, binding := range bindings {
		flagOverride := override.Flags[binding.property]
		for index, name := range binding.names {
			shorthand := ""
			if index == 0 {
				shorthand = safePluginShorthand(
					flagOverride.Shorthand,
					usedShorthands,
					reservations.shorthands,
				)
			}
			usage := firstNonEmptyPluginString(flagOverride.Description, binding.property)
			registerPluginFlag(cmd.Flags(), name, shorthand, usage, binding.kind, flagOverride.Default)
			if index > 0 || flagOverride.Hidden {
				_ = cmd.Flags().MarkHidden(name)
			}
		}
	}

	cmd.Flags().String("json", "", "Base JSON object payload for this command")
	cmd.Flags().String("params", "", "Additional JSON object payload merged after --json")
	_ = cmd.Flags().MarkHidden("json")
	_ = cmd.Flags().MarkHidden("params")
}

func registerPluginFlag(flags *pflag.FlagSet, name, shorthand, usage string, kind pluginFlagKind, rawDefault string) {
	switch kind {
	case pluginFlagInt:
		value, _ := strconv.Atoi(strings.TrimSpace(rawDefault))
		flags.IntP(name, shorthand, value, usage)
	case pluginFlagFloat:
		value, _ := strconv.ParseFloat(strings.TrimSpace(rawDefault), 64)
		flags.Float64P(name, shorthand, value, usage)
	case pluginFlagBool:
		value, _ := strconv.ParseBool(strings.TrimSpace(rawDefault))
		flags.BoolP(name, shorthand, value, usage)
	case pluginFlagStringSlice:
		var value []string
		if strings.TrimSpace(rawDefault) != "" {
			for _, item := range strings.Split(rawDefault, ",") {
				if item = strings.TrimSpace(item); item != "" {
					value = append(value, item)
				}
			}
		}
		flags.StringSliceP(name, shorthand, value, usage)
	default:
		flags.StringP(name, shorthand, rawDefault, usage)
	}
}

func collectPluginBindings(
	cmd *cobra.Command,
	args []string,
	bindings []pluginFlagBinding,
	params map[string]any,
) error {
	for _, binding := range bindings {
		var (
			value any
			set   bool
			err   error
		)
		for _, name := range binding.names {
			if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
				value, err = readPluginFlag(cmd.Flags(), name, binding.kind)
				set = err == nil
				break
			}
		}
		if err != nil {
			return apperrors.NewValidation(fmt.Sprintf("invalid plugin flag for %s: %v", binding.property, err))
		}
		if !set {
			if existing, exists := params[binding.property]; exists {
				value = existing
				set = true
			}
		}
		if !set && binding.positional && binding.positionalIndex >= 0 && binding.positionalIndex < len(args) {
			value, err = parsePluginValue(args[binding.positionalIndex], binding.kind)
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf(
					"invalid positional value for %s: %v",
					binding.property,
					err,
				))
			}
			set = true
		}
		if !set && binding.defaultProvided {
			value, err = parsePluginValue(binding.defaultValue, binding.kind)
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("invalid default for %s: %v", binding.property, err))
			}
			set = true
		}
		if !set && binding.envDefault != "" {
			if raw := strings.TrimSpace(os.Getenv(binding.envDefault)); raw != "" {
				value, err = parsePluginValue(raw, binding.kind)
				if err != nil {
					return apperrors.NewValidation(fmt.Sprintf("invalid %s value from %s: %v", binding.property, binding.envDefault, err))
				}
				set = true
			}
		}
		if !set {
			if binding.required {
				label := binding.property
				if len(binding.names) > 0 {
					label = "--" + binding.names[0]
				}
				return apperrors.NewValidation("missing required plugin parameter: " + label)
			}
			continue
		}
		if shouldOmitPluginValue(value, binding.omitWhen) {
			delete(params, binding.property)
			if binding.required {
				return apperrors.NewValidation("required plugin parameter is empty: " + binding.property)
			}
			continue
		}
		params[binding.property] = value
	}
	return nil
}

func readPluginFlag(flags *pflag.FlagSet, name string, kind pluginFlagKind) (any, error) {
	switch kind {
	case pluginFlagInt:
		return flags.GetInt(name)
	case pluginFlagFloat:
		return flags.GetFloat64(name)
	case pluginFlagBool:
		return flags.GetBool(name)
	case pluginFlagStringSlice:
		return flags.GetStringSlice(name)
	case pluginFlagJSON:
		raw, err := flags.GetString(name)
		if err != nil {
			return nil, err
		}
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return flags.GetString(name)
	}
}

func parsePluginValue(raw string, kind pluginFlagKind) (any, error) {
	switch kind {
	case pluginFlagInt:
		return strconv.Atoi(strings.TrimSpace(raw))
	case pluginFlagFloat:
		return strconv.ParseFloat(strings.TrimSpace(raw), 64)
	case pluginFlagBool:
		return strconv.ParseBool(strings.TrimSpace(raw))
	case pluginFlagStringSlice:
		var values []string
		for _, item := range strings.Split(raw, ",") {
			if item = strings.TrimSpace(item); item != "" {
				values = append(values, item)
			}
		}
		return values, nil
	case pluginFlagJSON:
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return raw, nil
	}
}

func shouldOmitPluginValue(value any, mode string) bool {
	if mode == "never" {
		return false
	}
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []string:
		return len(typed) == 0
	}
	if mode != "zero" {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return !typed
	case int:
		return typed == 0
	case float64:
		return typed == 0
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func wrapPluginParams(params map[string]any, wrapper string) {
	if params == nil || strings.TrimSpace(wrapper) == "" {
		return
	}
	body := make(map[string]any)
	if existing, ok := params[wrapper].(map[string]any); ok {
		for key, value := range existing {
			body[key] = value
		}
		delete(params, wrapper)
	}
	for key, value := range params {
		if strings.HasPrefix(key, "_") {
			continue
		}
		body[key] = value
		delete(params, key)
	}
	params[wrapper] = body
}

func applyPluginFlagConstraints(cmd *cobra.Command, override mcptypes.CLIToolOverride) {
	validate := func(group []string) []string {
		names := make([]string, 0, len(group))
		for _, name := range group {
			name = strings.TrimSpace(name)
			if name == "" || cmd.Flags().Lookup(name) == nil {
				return nil
			}
			names = append(names, name)
		}
		return names
	}
	for _, group := range override.MutuallyExclusive {
		if names := validate(group); len(names) > 1 {
			cmd.MarkFlagsMutuallyExclusive(names...)
		}
	}
	for _, group := range override.RequireOneOf {
		if names := validate(group); len(names) > 0 {
			cmd.MarkFlagsOneRequired(names...)
		}
	}
	for _, group := range override.RequireTogether {
		if names := validate(group); len(names) > 1 {
			cmd.MarkFlagsRequiredTogether(names...)
		}
	}
}

func validPluginFlagConstraints(bindings []pluginFlagBinding, override mcptypes.CLIToolOverride) bool {
	known := make(map[string]bool)
	for _, binding := range bindings {
		for _, name := range binding.names {
			known[name] = true
		}
	}
	validate := func(groups [][]string, minimum int) bool {
		for _, group := range groups {
			if len(group) < minimum {
				return false
			}
			seen := make(map[string]bool, len(group))
			for _, rawName := range group {
				name := strings.TrimSpace(rawName)
				if name == "" || !known[name] || seen[name] {
					return false
				}
				seen[name] = true
			}
		}
		return true
	}
	return validate(override.MutuallyExclusive, 2) &&
		validate(override.RequireOneOf, 1) &&
		validate(override.RequireTogether, 2)
}

func ensurePluginGroup(
	root *cobra.Command,
	rawPath string,
	description string,
	groups map[string]*cobra.Command,
) *cobra.Command {
	if !validPluginGroupPath(rawPath) {
		return nil
	}
	parts := strings.Split(strings.TrimSpace(rawPath), ".")
	parent := root
	var pathParts []string
	for index, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		pathParts = append(pathParts, part)
		path := strings.Join(pathParts, ".")
		if existing := groups[path]; existing != nil {
			parent = existing
			continue
		}
		short := part
		if index == len(parts)-1 {
			short = firstNonEmptyPluginString(description, part)
		}
		group := cobracmd.NewGroupCommand(part, short)
		cmdutil.MarkPluginSource(group)
		parent.AddCommand(group)
		groups[path] = group
		parent = group
	}
	return parent
}

func validPluginGroupPath(rawPath string) bool {
	parts := strings.Split(strings.TrimSpace(rawPath), ".")
	for _, rawPart := range parts {
		if !validPluginCommandName(strings.TrimSpace(rawPart)) {
			return false
		}
	}
	return true
}

func mergePluginRoot(destination, source *cobra.Command) {
	if destination == nil || source == nil {
		return
	}
	aliases := append(append([]string(nil), destination.Aliases...), source.Aliases...)
	destination.Aliases = safePluginAliases(aliases, destination.Name())
	cobracmd.MergeCommandTree(destination, source)
}

func pruneEmptyPluginGroups(parent *cobra.Command) {
	if parent == nil {
		return
	}
	for _, child := range append([]*cobra.Command(nil), parent.Commands()...) {
		pruneEmptyPluginGroups(child)
		if cmdutil.IsGroup(child) && len(child.Commands()) == 0 {
			parent.RemoveCommand(child)
		}
	}
}

func pluginConfirmationRequired(commandPath string) error {
	return apperrors.NewValidation(
		commandPath+" is sensitive; preview with --dry-run, then obtain user confirmation and retry with --yes",
		apperrors.WithReason("confirmation_required"),
		apperrors.WithHint("run the same command with --dry-run first; after the user confirms, add --yes"),
		apperrors.WithActions("preview with --dry-run", "obtain user confirmation", "execute with --yes"),
	)
}

func pluginRootBoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil || cmd.Root() == nil {
		return false
	}
	flags := cmd.Root().PersistentFlags()
	if flags == nil || flags.Lookup(name) == nil {
		return false
	}
	value, err := flags.GetBool(name)
	return err == nil && value
}

func pluginFlagKindFromString(raw string) pluginFlagKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "int", "integer":
		return pluginFlagInt
	case "float", "float64", "number":
		return pluginFlagFloat
	case "bool", "boolean":
		return pluginFlagBool
	case "stringslice", "string_slice", "array", "[]string":
		return pluginFlagStringSlice
	case "json", "object":
		return pluginFlagJSON
	default:
		return pluginFlagString
	}
}

func safePluginShorthand(raw string, used, reserved map[string]bool) string {
	value := strings.TrimSpace(raw)
	if len(value) != 1 || used[value] || reserved[value] {
		return ""
	}
	used[value] = true
	return value
}

func pluginReservedFlags(root *cobra.Command) pluginFlagReservations {
	reservations := pluginFlagReservations{
		names: map[string]bool{
			"client-id": true, "client-secret": true, "debug": true,
			"dry-run": true, "fields": true, "format": true, "help": true,
			"jq": true, "json": true, "mock": true, "output": true,
			"params": true, "profile": true, "timeout": true, "token": true,
			"verbose": true, "yes": true,
		},
		shorthands: map[string]bool{
			"f": true, "h": true, "o": true, "v": true, "y": true,
		},
	}
	if root == nil {
		return reservations
	}
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		reservations.names[flag.Name] = true
		if shorthand := strings.TrimSpace(flag.Shorthand); shorthand != "" {
			reservations.shorthands[shorthand] = true
		}
	})
	return reservations
}

func unsupportedPluginOverlay(overlay mcptypes.CLIOverlay) string {
	switch {
	case strings.TrimSpace(overlay.Parent) != "":
		return "parent"
	case strings.TrimSpace(overlay.Group) != "":
		return "group"
	case len(overlay.ServerDeps) > 0:
		return "serverDeps"
	case len(overlay.Hints) > 0:
		return "hintCommands"
	case strings.TrimSpace(overlay.RedirectTo) != "":
		return "redirectTo"
	default:
		return ""
	}
}

func unsupportedPluginDescriptor(root *cobra.Command, descriptor mcptypes.ServerDescriptor) string {
	overlay := descriptor.CLI
	if reason := unsupportedPluginOverlay(overlay); reason != "" {
		return reason
	}
	if len(overlay.ToolOverrides) == 0 {
		return ""
	}
	if !validPluginCommandName(pluginDescriptorRootName(descriptor)) {
		return "command"
	}
	for groupPath := range overlay.Groups {
		if !validPluginGroupPath(groupPath) {
			return "groups"
		}
	}
	reservations := pluginReservedFlags(root)
	for toolName, override := range overlay.ToolOverrides {
		if override.Hidden {
			continue
		}
		if strings.TrimSpace(toolName) == "" {
			return "tool"
		}
		if reason := unsupportedPluginToolOverride(override); reason != "" {
			return reason
		}
		leafName := strings.TrimSpace(override.CLIName)
		if leafName == "" {
			leafName = derivePluginCommandName(toolName, overlay.Prefixes)
		}
		if !validPluginCommandName(leafName) {
			return "cliName"
		}
		if groupPath := strings.TrimSpace(override.Group); groupPath != "" &&
			!validPluginGroupPath(groupPath) {
			return "group"
		}
		bindings, _, _, ok := registerPluginBindings(leafName, override, reservations)
		if !ok {
			return "flags"
		}
		if !validPluginFlagConstraints(bindings, override) {
			return "constraints"
		}
	}
	return ""
}

func unsupportedPluginToolOverride(override mcptypes.CLIToolOverride) string {
	switch {
	case len(override.CLIAliases) > 0:
		return "cliAliases"
	case len(override.OutputFormat) > 0:
		return "outputFormat"
	case strings.TrimSpace(override.ServerOverride) != "":
		return "serverOverride"
	case strings.TrimSpace(override.RedirectTo) != "":
		return "redirectTo"
	case len(override.Pipeline) > 0:
		return "pipeline"
	default:
		return ""
	}
}

func unsupportedPluginFlagOverride(override mcptypes.CLIFlagOverride) string {
	switch {
	case strings.TrimSpace(override.MapsTo) != "":
		return "mapsTo"
	case strings.TrimSpace(override.Transform) != "":
		return "transform"
	case len(override.TransformArgs) > 0:
		return "transformArgs"
	case strings.TrimSpace(override.RuntimeDefault) != "":
		return "runtimeDefault"
	case override.PipelineLocal:
		return "pipelineLocal"
	case !supportedPluginFlagType(override.Type):
		return "type"
	case !supportedPluginOmitMode(override.OmitWhen):
		return "omitWhen"
	default:
		return ""
	}
}

func supportedPluginFlagType(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "string", "int", "integer", "float", "float64", "number",
		"bool", "boolean", "stringslice", "string_slice", "array", "[]string",
		"json", "object":
		return true
	default:
		return false
	}
}

func supportedPluginOmitMode(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "empty", "zero", "never":
		return true
	default:
		return false
	}
}

func safePluginAliases(rawAliases []string, commandName string) []string {
	seen := map[string]bool{commandName: true}
	aliases := make([]string, 0, len(rawAliases))
	for _, raw := range rawAliases {
		alias := strings.TrimSpace(raw)
		if !validPluginCommandName(alias) || reservedCommands[alias] || seen[alias] {
			continue
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	return aliases
}

func derivePluginCommandName(toolName string, prefixes []string) string {
	name := strings.TrimSpace(toolName)
	for _, rawPrefix := range prefixes {
		prefix := strings.TrimSpace(rawPrefix)
		if prefix != "" && strings.HasPrefix(name, prefix+"_") {
			name = strings.TrimPrefix(name, prefix+"_")
			break
		}
	}
	return pluginKebabName(name)
}

func pluginKebabName(raw string) string {
	var result strings.Builder
	lastDash := false
	var previous rune
	for index, current := range raw {
		separator := current == '_' || current == '.' || unicode.IsSpace(current)
		if separator {
			if result.Len() > 0 && !lastDash {
				result.WriteByte('-')
				lastDash = true
			}
			previous = current
			continue
		}
		if unicode.IsUpper(current) && index > 0 && !lastDash &&
			(unicode.IsLower(previous) || unicode.IsDigit(previous)) {
			result.WriteByte('-')
		}
		result.WriteRune(unicode.ToLower(current))
		lastDash = false
		previous = current
	}
	return strings.Trim(result.String(), "-")
}

func validPluginCommandName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "help" && validPluginKebabName(name)
}

func validPluginFlagName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "json" && name != "params" && validPluginKebabName(name)
}

func validPluginKebabName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	previousDash := false
	for _, current := range name {
		switch {
		case current >= 'a' && current <= 'z':
			previousDash = false
		case current >= '0' && current <= '9':
			previousDash = false
		case current == '-' && !previousDash:
			previousDash = true
		default:
			return false
		}
	}
	return !previousDash
}

func firstNonEmptyPluginString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
