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
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/spf13/cobra"
)

// BuildDynamicCommands generates cobra commands from servers.json CLIOverlay metadata.
// Each server with non-skip CLIOverlay gets a top-level command with groups and
// tool overrides translated into subcommands with proper flag bindings and transforms.
//
// detailsByID maps CLI server ID → []DetailTool from the MCP Detail API.
// When provided, tool Short/Long descriptions and typed flags are enriched from Detail API data.
//
// Conversion rules reference: docs/mcp-to-cli-conversion.md
func BuildDynamicCommands(servers []market.ServerDescriptor, runner executor.Runner, detailsByID map[string][]market.DetailTool) []*cobra.Command {
	type builtCmd struct {
		cmd    *cobra.Command
		parent string // cli.Parent: attach as sub-command of this top-level command
	}

	var built []builtCmd
	for _, server := range servers {
		cli := server.CLI
		// §1.5: cli.skip → skip entire service
		if cli.Skip {
			continue
		}
		if len(cli.ToolOverrides) == 0 {
			continue
		}

		// §1.1: cli.command → top-level command name
		cmdName := strings.TrimSpace(cli.Command)
		if cmdName == "" {
			cmdName = strings.TrimSpace(cli.ID)
		}
		if cmdName == "" {
			continue
		}

		rootCmd := NewGroupCommand(cmdName, cli.Description)
		// §1.5: cli.hidden → entire service hidden
		if cli.Hidden {
			rootCmd.Hidden = true
		}

		// Build detail index for this server: toolName → DetailTool
		detailIndex := buildDetailIndex(detailsByID[strings.TrimSpace(cli.ID)])

		// §1.2: Build group sub-commands (including nested groups via "." separator)
		groupCmds := make(map[string]*cobra.Command)
		if len(cli.Groups) > 0 {
			groupNames := sortedKeys(cli.Groups)
			for _, groupName := range groupNames {
				groupDef := cli.Groups[groupName]
				ensureNestedGroup(rootCmd, groupName, groupDef.Description, groupCmds)
			}
		}

		// §1.3: Build tool override leaf commands
		toolNames := sortedToolNames(cli.ToolOverrides)
		for _, toolName := range toolNames {
			override := cli.ToolOverrides[toolName]
			// §1.5: toolOverrides[tool].hidden = true → skip
			if override.Hidden {
				continue
			}

			cliName := strings.TrimSpace(override.CLIName)
			if cliName == "" {
				cliName = deriveCommandName(toolName, cli.Prefixes)
			}

			bindings, normalizer := buildOverrideBindings(override)

			// Upgrade binding.Kind from JSON schema: array/object params must
			// be parsed as JSON, not transmitted as raw strings (otherwise
			// MCP backends silently accept stringified payloads and drop data).
			if dt, ok := detailIndex[toolName]; ok && dt.ToolRequest != "" {
				bindings = upgradeBindingsFromSchema(bindings, dt.ToolRequest)
			}

			// Resolve Short/Long from Detail API toolTitle/toolDesc;
			// fallback to overlay description; then to generic cmdName/cliName.
			short := fmt.Sprintf("%s/%s", cmdName, cliName)
			long := ""
			if desc := strings.TrimSpace(override.Description); desc != "" {
				short = desc
			}
			if dt, ok := detailIndex[toolName]; ok {
				if title := strings.TrimSpace(dt.ToolTitle); title != "" {
					short = title
				}
				if desc := strings.TrimSpace(dt.ToolDesc); desc != "" {
					long = desc
				}
			}

			route := Route{
				Use:   cliName,
				Short: short,
				Long:  long,
				Target: Target{
					CanonicalProduct: strings.TrimSpace(cli.ID),
					Tool:             toolName,
				},
				Bindings:   bindings,
				Normalizer: normalizer,
			}

			// §5.1: isSensitive → need --yes confirmation
			if override.IsSensitive {
				route.Normalizer = chainSensitiveNormalizer(normalizer)
			}

			cmd := NewDirectCommand(route, runner)

			// Enrich flags with typed parameters from Detail API toolRequest JSON Schema.
			if dt, ok := detailIndex[toolName]; ok && dt.ToolRequest != "" {
				buildFlagsFromDetailSchema(cmd, dt.ToolRequest, override.Flags)
			}

			// §1.4: Add to the right parent group
			groupName := strings.TrimSpace(override.Group)
			if groupName != "" {
				parent := resolveNestedGroup(rootCmd, groupName, groupCmds)
				parent.AddCommand(cmd)
			} else {
				rootCmd.AddCommand(cmd)
			}
		}

		built = append(built, builtCmd{cmd: rootCmd, parent: strings.TrimSpace(cli.Parent)})
	}

	// Collect top-level commands first, then attach child commands via cli.Parent.
	topLevel := make(map[string]*cobra.Command)
	var topOrder []string
	var children []builtCmd

	for _, b := range built {
		if b.parent == "" {
			name := b.cmd.Name()
			if _, exists := topLevel[name]; !exists {
				topOrder = append(topOrder, name)
			}
			topLevel[name] = b.cmd
		} else {
			children = append(children, b)
		}
	}
	for _, child := range children {
		if parent, ok := topLevel[child.parent]; ok {
			parent.AddCommand(child.cmd)
		} else {
			// Parent not found among dynamic commands; emit as top-level.
			name := child.cmd.Name()
			if _, exists := topLevel[name]; !exists {
				topOrder = append(topOrder, name)
			}
			topLevel[name] = child.cmd
		}
	}

	commands := make([]*cobra.Command, 0, len(topLevel))
	for _, name := range topOrder {
		commands = append(commands, topLevel[name])
	}
	return commands
}

// buildDetailIndex creates a map from toolName → DetailTool for fast lookup.
func buildDetailIndex(tools []market.DetailTool) map[string]market.DetailTool {
	idx := make(map[string]market.DetailTool, len(tools))
	for _, dt := range tools {
		name := strings.TrimSpace(dt.ToolName)
		if name != "" {
			idx[name] = dt
		}
	}
	return idx
}

// toolRequestSchema is the minimal JSON Schema structure we need from toolRequest.
type toolRequestSchema struct {
	Properties map[string]toolRequestProp `json:"properties"`
	Required   []string                   `json:"required"`
}

type toolRequestProp struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// buildFlagsFromDetailSchema adds properly-typed cobra flags to cmd based on
// the MCP toolRequest JSON Schema. The flagOverrides map (from CLIToolOverride.Flags)
// provides the display-layer alias so the flag name matches what users see.
//
// Already-registered flags (from buildOverrideBindings) are skipped to avoid duplicates.
func buildFlagsFromDetailSchema(cmd *cobra.Command, schemaJSON string, flagOverrides map[string]market.CLIFlagOverride) {
	var schema toolRequestSchema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return
	}
	if len(schema.Properties) == 0 {
		return
	}

	requiredSet := make(map[string]bool, len(schema.Required))
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	// Process properties in sorted order for determinism.
	keys := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		prop := schema.Properties[key]

		// Determine flag name: prefer alias from CLIFlagOverride, else kebab-case.
		flagName := toKebabCase(key)
		if ov, ok := flagOverrides[key]; ok && strings.TrimSpace(ov.Alias) != "" {
			flagName = strings.TrimSpace(ov.Alias)
		}

		// Skip reserved names and already-registered flags.
		if flagName == "json" || flagName == "params" {
			continue
		}
		if cmd.Flags().Lookup(flagName) != nil {
			// Already registered by buildOverrideBindings; just update usage text if empty.
			if f := cmd.Flags().Lookup(flagName); f != nil && f.Usage == key {
				help := strings.TrimSpace(prop.Description)
				if help == "" {
					help = strings.TrimSpace(prop.Title)
				}
				if help != "" {
					f.Usage = help
				}
			}
			continue
		}

		help := strings.TrimSpace(prop.Description)
		if help == "" {
			help = strings.TrimSpace(prop.Title)
		}
		if help == "" {
			help = key
		}

		switch prop.Type {
		case "integer", "number":
			cmd.Flags().Int(flagName, 0, help)
		case "boolean":
			cmd.Flags().Bool(flagName, false, help)
		case "array":
			cmd.Flags().StringSlice(flagName, nil, help+" (comma-separated)")
		default: // "string", "object", or unknown → string
			defaultVal := prop.Default
			cmd.Flags().String(flagName, defaultVal, help)
		}

		if requiredSet[key] {
			_ = cmd.MarkFlagRequired(flagName)
		}
	}
}

// toKebabCase converts camelCase or snake_case identifiers to kebab-case.
// Examples: "parentDentryUuid" → "parent-dentry-uuid", "report_id" → "report-id"
func toKebabCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r == '_' {
			b.WriteByte('-')
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// ServerEndpoints extracts product ID → endpoint URL mapping from servers.
func ServerEndpoints(servers []market.ServerDescriptor) map[string]string {
	endpoints := make(map[string]string)
	for _, server := range servers {
		if server.CLI.Skip {
			continue
		}
		id := strings.TrimSpace(server.CLI.ID)
		endpoint := strings.TrimSpace(server.Endpoint)
		if id != "" && endpoint != "" {
			endpoints[id] = endpoint
		}
	}
	return endpoints
}

// ServerProductIDs extracts the set of product IDs from servers with CLI metadata.
func ServerProductIDs(servers []market.ServerDescriptor) map[string]bool {
	ids := make(map[string]bool)
	for _, server := range servers {
		if server.CLI.Skip {
			continue
		}
		id := strings.TrimSpace(server.CLI.ID)
		if id != "" {
			ids[id] = true
		}
		cmd := strings.TrimSpace(server.CLI.Command)
		if cmd != "" && cmd != id {
			ids[cmd] = true
		}
		for _, alias := range server.CLI.Aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				ids[alias] = true
			}
		}
	}
	return ids
}

// ensureNestedGroup creates group commands for potentially nested group names.
// §1.4: "group.members" with "." means parent-child relationship → "dws chat group members"
func ensureNestedGroup(root *cobra.Command, groupPath, description string, registry map[string]*cobra.Command) *cobra.Command {
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

		desc := part
		if i == len(parts)-1 {
			desc = description
		}
		gc := NewGroupCommand(part, desc)
		parent.AddCommand(gc)
		registry[builtPath] = gc
		parent = gc
	}
	return parent
}

// resolveNestedGroup finds or creates the group command for a potentially nested group path.
func resolveNestedGroup(root *cobra.Command, groupPath string, registry map[string]*cobra.Command) *cobra.Command {
	if existing, ok := registry[groupPath]; ok {
		return existing
	}
	// Auto-create if not defined in groups
	return ensureNestedGroup(root, groupPath, groupPath, registry)
}

// upgradeBindingsFromSchema inspects the MCP toolRequest JSON Schema and
// promotes ValueString bindings whose schema type is "array" or "object" to
// ValueJSON. This ensures complex parameters (e.g. sheet update_range.values
// 2D array) are parsed and forwarded as proper JSON values rather than raw
// strings — backends that accept stringified payloads silently drop data.
func upgradeBindingsFromSchema(bindings []FlagBinding, schemaJSON string) []FlagBinding {
	if len(bindings) == 0 || schemaJSON == "" {
		return bindings
	}
	var schema toolRequestSchema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return bindings
	}
	if len(schema.Properties) == 0 {
		return bindings
	}
	for i := range bindings {
		if bindings[i].Kind != ValueString {
			continue
		}
		prop, ok := schema.Properties[bindings[i].Property]
		if !ok {
			continue
		}
		switch prop.Type {
		case "array", "object":
			bindings[i].Kind = ValueJSON
		}
	}
	return bindings
}

// buildOverrideBindings converts CLIToolOverride flags into FlagBindings and
// constructs a Normalizer that applies transform rules.
// Implements §2.1-§2.5 of the conversion rules.
func buildOverrideBindings(override market.CLIToolOverride) ([]FlagBinding, Normalizer) {
	if len(override.Flags) == 0 {
		return nil, nil
	}

	paramNames := make([]string, 0, len(override.Flags))
	for paramName := range override.Flags {
		paramNames = append(paramNames, paramName)
	}
	sort.Strings(paramNames)

	var bindings []FlagBinding
	type transformEntry struct {
		paramName     string
		transform     string
		transformArgs map[string]any
	}
	var transforms []transformEntry
	type envDefaultEntry struct {
		paramName string
		envVar    string
	}
	var envDefaults []envDefaultEntry
	type hiddenDefaultEntry struct {
		paramName    string
		defaultValue string
	}
	var hiddenDefaults []hiddenDefaultEntry

	for _, paramName := range paramNames {
		flagOverride := override.Flags[paramName]

		// §2.2: flag name from alias, fallback to kebab-case of param name
		flagName := strings.TrimSpace(flagOverride.Alias)
		if flagName == "" {
			flagName = compatFlagName(paramName)
		}
		if flagName == "" {
			flagName = paramName
		}

		// Skip reserved internal flag names (--json, --params are added by ApplyBindings)
		if flagName == "json" || flagName == "params" {
			continue
		}

		binding := FlagBinding{
			FlagName: flagName,
			Property: paramName,
			Kind:     ValueString,
			Usage:    paramName,
		}

		// §2.5: hidden flag with default
		if flagOverride.Hidden {
			// Hidden flags are still added but marked hidden.
			// They are auto-populated with their default value via the normalizer.
			if flagOverride.Default != "" {
				hiddenDefaults = append(hiddenDefaults, hiddenDefaultEntry{
					paramName:    paramName,
					defaultValue: flagOverride.Default,
				})
			}
		}

		bindings = append(bindings, binding)

		if flagOverride.Transform != "" {
			transforms = append(transforms, transformEntry{
				paramName:     paramName,
				transform:     flagOverride.Transform,
				transformArgs: flagOverride.TransformArgs,
			})
		}
		if flagOverride.EnvDefault != "" {
			envDefaults = append(envDefaults, envDefaultEntry{
				paramName: paramName,
				envVar:    flagOverride.EnvDefault,
			})
		}
	}

	// Check if we need a normalizer: transforms, env defaults, hidden defaults,
	// or dotted property paths that need nesting.
	needsDottedNesting := false
	for _, b := range bindings {
		if strings.Contains(b.Property, ".") {
			needsDottedNesting = true
			break
		}
	}
	if len(transforms) == 0 && len(envDefaults) == 0 && len(hiddenDefaults) == 0 && !needsDottedNesting {
		return bindings, nil
	}

	// Build a normalizer that applies hidden defaults + env defaults + transforms + nesting
	normalizer := func(cmd *cobra.Command, params map[string]any) error {
		// §2.5: Apply hidden flag defaults for parameters not explicitly set
		for _, hd := range hiddenDefaults {
			if _, exists := params[hd.paramName]; !exists {
				params[hd.paramName] = hd.defaultValue
			}
		}

		// Apply environment variable defaults for parameters not explicitly set
		for _, ed := range envDefaults {
			if _, exists := params[ed.paramName]; !exists {
				if envVal := strings.TrimSpace(os.Getenv(ed.envVar)); envVal != "" {
					params[ed.paramName] = envVal
				}
			}
		}

		// §3: Apply transforms
		for _, t := range transforms {
			val, exists := params[t.paramName]
			if !exists {
				// For enum_map with _default, apply default even when flag is omitted
				if t.transform == "enum_map" && t.transformArgs != nil {
					if defaultVal, hasDefault := t.transformArgs["_default"]; hasDefault {
						params[t.paramName] = defaultVal
					}
				}
				continue
			}
			transformed, err := ApplyTransform(val, t.transform, t.transformArgs)
			if err != nil {
				return err
			}
			params[t.paramName] = transformed
		}

		// Nest dotted property paths: "Body.query" → params["Body"]["query"]
		nestDottedPaths(params)

		return nil
	}

	return bindings, normalizer
}

// chainSensitiveNormalizer wraps a normalizer with --yes confirmation for sensitive operations (§5.1).
func chainSensitiveNormalizer(inner Normalizer) Normalizer {
	return func(cmd *cobra.Command, params map[string]any) error {
		if inner != nil {
			if err := inner(cmd, params); err != nil {
				return err
			}
		}
		return requireYesForDelete(cmd, params)
	}
}

// deriveCommandName converts an MCP tool name to a CLI command name
// by stripping known prefixes and converting to kebab-case.
func deriveCommandName(toolName string, prefixes []string) string {
	name := toolName
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		stripped := strings.TrimPrefix(name, prefix+"_")
		if stripped != name {
			name = stripped
			break
		}
	}
	return compatFlagName(name)
}

func sortedKeys(m map[string]market.CLIGroupDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedToolNames(m map[string]market.CLIToolOverride) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// nestDottedPaths converts flat dotted keys in params into nested maps.
// Example: params["Body.query"] = "test" → params["Body"] = map{"query": "test"}
// If multiple dotted keys share a prefix, they are merged into the same nested map.
func nestDottedPaths(params map[string]any) {
	var dottedKeys []string
	for key := range params {
		if strings.Contains(key, ".") {
			dottedKeys = append(dottedKeys, key)
		}
	}
	if len(dottedKeys) == 0 {
		return
	}
	sort.Strings(dottedKeys)
	for _, key := range dottedKeys {
		val := params[key]
		delete(params, key)

		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			params[key] = val // shouldn't happen, but be safe
			continue
		}
		parent, child := parts[0], parts[1]

		// Get or create the nested map
		nested, ok := params[parent].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			params[parent] = nested
		}
		nested[child] = val
	}
}
