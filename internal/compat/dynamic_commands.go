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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// runtimeDefaultWhitelist is the closed set of placeholders v3 supports for
// CLIFlagOverride.RuntimeDefault. Placeholders outside this set emit a
// warning at command-build time and are ignored at invocation time. See
// discovery-schema-v3 §2.3.
var runtimeDefaultWhitelist = map[string]bool{
	"$currentUserId": true,
	"$unionId":       true,
	"$corpId":        true,
	"$now":           true,
	"$today":         true,
}

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
		// §1.1: cli.command → top-level command name
		cmdName := strings.TrimSpace(cli.Command)
		if cmdName == "" {
			cmdName = strings.TrimSpace(cli.ID)
		}
		if cmdName == "" {
			continue
		}

		// §v3.2.6: CLIOverlay.RedirectTo turns the entire top-level product
		// into a stub that prints "Please use: dws <target>".
		if target := strings.TrimSpace(cli.RedirectTo); target != "" {
			stub := buildOverlayRedirect(cmdName, cli.Description, target)
			if cli.Hidden {
				stub.Hidden = true
			}
			// Mark envelope provenance so overlay registrants can tell this
			// redirect stub apart from a helper fallback carrying the same
			// name; see cmdutil.SourceAnnotation.
			cmdutil.MarkEnvelopeSource(stub)
			built = append(built, builtCmd{cmd: stub, parent: strings.TrimSpace(cli.Parent)})
			continue
		}

		if len(cli.ToolOverrides) == 0 {
			continue
		}

		rootCmd := NewGroupCommand(cmdName, cli.Description)
		// §1.5: cli.hidden → entire service hidden
		if cli.Hidden {
			rootCmd.Hidden = true
		}
		// Mark envelope provenance so edition overlays can distinguish this
		// dynamic root from a same-named helper fallback when deciding
		// whether to merge hardcoded leaves or evict and replace. See
		// cmdutil.SourceAnnotation and the wukong overlay's RegisterProducts.
		cmdutil.MarkEnvelopeSource(rootCmd)

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

			// §P2.redirect: redirectTo turns this entry into a stub.
			if target := strings.TrimSpace(override.RedirectTo); target != "" {
				redirect := buildRedirectCommand(cliName, override.Description, target)
				attachToGroup(rootCmd, override.Group, groupCmds, redirect)
				continue
			}

			bindings, normalizer := buildOverrideBindings(override)

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

			// ServerOverride routes this leaf's tool invocation to a different
			// product's MCP server; fall back to the enclosing overlay's ID.
			canonicalProduct := strings.TrimSpace(override.ServerOverride)
			if canonicalProduct == "" {
				canonicalProduct = strings.TrimSpace(cli.ID)
			}

			route := Route{
				Use: cliName,
				// CLIAliases register additional cobra command aliases for the
				// same MCP tool. Empty / nil means no extra names.
				Aliases: append([]string(nil), override.CLIAliases...),
				Short:   short,
				Long:    long,
				// Preserve left-side indentation: cobra's Examples template
				// renders {{.Example}} verbatim, and hardcoded helper commands
				// rely on a 2-space prefix to look indented under "Examples:".
				// Only trim trailing whitespace/newlines so envelope JSON can
				// safely carry a closing "\n" without doubling the blank line.
				Example: strings.TrimRight(override.Example, " \t\r\n"),
				Target: Target{
					CanonicalProduct: canonicalProduct,
					Tool:             toolName,
				},
				Bindings: bindings,
				// §pipeline: when the envelope declares a multi-step
				// orchestration, NewDirectCommand reroutes RunE into the
				// pipeline executor instead of the single-tool flow. The
				// CLIName / Group / Flags surface above still applies.
				Pipeline:   append([]market.PipelineStep(nil), override.Pipeline...),
				Normalizer: normalizer,
			}

			// §5.1: isSensitive → need --yes confirmation
			if override.IsSensitive {
				route.Normalizer = chainSensitiveNormalizer(normalizer)
			}

			// §v3.2.5: outputFormat.rename/drop/columns post-processing.
			route.OutputTransform = buildOutputTransform(override.OutputFormat)

			cmd := NewDirectCommand(route, runner)

			// Enrich flags with typed parameters from Detail API toolRequest JSON Schema.
			if dt, ok := detailIndex[toolName]; ok && dt.ToolRequest != "" {
				buildFlagsFromDetailSchema(cmd, dt.ToolRequest, override.Flags)
			}

			// §P2.flagconstraints: must run AFTER schema enrichment because the
			// target flags may be registered lazily by buildFlagsFromDetailSchema.
			applyFlagConstraints(cmd, override)

			// §1.4: Add to the right parent group
			attachToGroup(rootCmd, override.Group, groupCmds, cmd)
		}

		// §P2.hints: attach hint stub commands registered on the overlay.
		if len(cli.Hints) > 0 {
			hintNames := make([]string, 0, len(cli.Hints))
			for name := range cli.Hints {
				hintNames = append(hintNames, name)
			}
			sort.Strings(hintNames)
			for _, name := range hintNames {
				def := cli.Hints[name]
				hint := buildHintCommand(name, def)
				attachToGroup(rootCmd, def.Group, groupCmds, hint)
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
			attachOrMerge(parent, child.cmd)
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
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Default     string   `json:"default,omitempty"`
	Format      string   `json:"format,omitempty"`
	Enum        []string `json:"enum,omitempty"`
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

		// Skip properties that are bound as positional arguments; they are
		// collected from cobra args rather than flags. For dual-mode positional
		// bindings (envelope: positional + alias/aliases), ApplyBindings has
		// already registered the alias flags, so we should not re-register
		// them here from the MCP detail schema.
		if ov, ok := flagOverrides[key]; ok && ov.Positional {
			continue
		}

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

		// Carry schema "format" / "enum" hints onto the cobra flag via
		// pflag annotations so PreParse handlers (e.g. StickyHandler)
		// can reason about whether a glued suffix looks like a real
		// value. The annotation keys are read by FlagInfoFromCommand.
		if prop.Format != "" {
			_ = cmd.Flags().SetAnnotation(flagName, "x-cli-format", []string{prop.Format})
		}
		if len(prop.Enum) > 0 {
			_ = cmd.Flags().SetAnnotation(flagName, "x-cli-enum", append([]string{}, prop.Enum...))
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

// attachOrMerge adds child as a sub-command of parent. If parent already has a
// sub-command with the same Name(), the two are merged recursively: child's
// sub-commands are moved onto the existing one and child itself is discarded.
// Leaf collisions (two commands with the same Name and no further children)
// are resolved first-wins — the incoming one is dropped.
//
// This lets multiple server entries share a cli.command under the same parent,
// e.g. bot-message (command="message", parent="chat") can contribute leaves
// into the same "message" subtree already built from chat's own toolOverrides,
// without creating a duplicate "message" sibling in chat's help output.
func attachOrMerge(parent, child *cobra.Command) {
	existing := findSubcommand(parent, child.Name())
	if existing == nil {
		parent.AddCommand(child)
		return
	}
	// Snapshot child's sub-commands before we start moving them (RemoveCommand
	// mutates the slice we'd be iterating).
	subs := make([]*cobra.Command, len(child.Commands()))
	copy(subs, child.Commands())
	for _, sub := range subs {
		child.RemoveCommand(sub)
		attachOrMerge(existing, sub)
	}
}

// findSubcommand returns the first sub-command of parent with the given name,
// or nil if none match.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
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
	// defaultInjectEntry captures envelope flag.default values that must be
	// injected into the MCP body when the user omits the flag. v3.2 widened
	// this from hidden-only to all flags so that visible flags carrying a
	// default (e.g. oa list-forms cursor=0 / pageSize=100) match the
	// hardcoded helper command behavior of `mustGetFlag(cobra default) →
	// body`. The kind drives typed coercion at injection time so a
	// `type: int` envelope default reaches MCP as `int(0)`, not string `"0"`.
	type defaultInjectEntry struct {
		paramName    string
		defaultValue string
		kind         ValueKind
	}
	var defaultInjects []defaultInjectEntry
	type runtimeDefaultEntry struct {
		paramName   string
		placeholder string
	}
	var runtimeDefaults []runtimeDefaultEntry
	type omitEntry struct {
		paramName string
		mode      string // "empty" (default) | "zero" | "never"
	}
	omits := make(map[string]omitEntry, len(paramNames))

	for _, paramName := range paramNames {
		flagOverride := override.Flags[paramName]

		// §2.2: flag name from alias, fallback to kebab-case of param name.
		// For pure positional bindings (no alias declared) we deliberately
		// leave FlagName empty so ApplyBindings / NewDirectCommand can
		// distinguish "envelope wants flag-or-positional dual entry" from
		// "envelope only wants positional". Auto-deriving a flag name here
		// would otherwise leak a redundant `--<paramName>` flag and confuse
		// the dual-mode detection.
		explicitAlias := strings.TrimSpace(flagOverride.Alias)
		flagName := explicitAlias
		if flagName == "" && !flagOverride.Positional {
			flagName = compatFlagName(paramName)
		}
		if flagName == "" && !flagOverride.Positional {
			flagName = paramName
		}

		// Skip reserved internal flag names (--json, --params are added by ApplyBindings)
		if flagName == "json" || flagName == "params" {
			continue
		}

		// §2.2 (aliases): deduplicate additional hidden aliases against the
		// primary name + reserved names. Preserves envelope declaration order
		// so CLI precedence (primary > Alias > Aliases[0..n]) is deterministic.
		var extraAliases []string
		if len(flagOverride.Aliases) > 0 {
			seen := map[string]bool{"json": true, "params": true}
			if flagName != "" {
				seen[flagName] = true
			}
			extraAliases = make([]string, 0, len(flagOverride.Aliases))
			for _, a := range flagOverride.Aliases {
				a = strings.TrimSpace(a)
				if a == "" || seen[a] {
					continue
				}
				seen[a] = true
				extraAliases = append(extraAliases, a)
			}
		}

		// Usage defaults to paramName but an explicit Description on the
		// overlay wins (it also beats the Detail API's toolDesc during flag
		// enrichment because buildFlagsFromDetailSchema preserves overlay usage).
		usage := paramName
		if desc := strings.TrimSpace(flagOverride.Description); desc != "" {
			usage = desc
		}

		binding := FlagBinding{
			FlagName: flagName,
			Aliases:  extraAliases,
			// §pipeline: PipelineLocal flags (e.g. `--output` in the
			// sheet export pipeline) are CLI-side only — they appear in
			// --help and are bindable, but CollectBindings skips them so
			// the value never reaches MCP params. The pipeline executor
			// reads them via extractFlagValuesByAlias.
			PipelineLocal: flagOverride.PipelineLocal,
			Short:         strings.TrimSpace(flagOverride.Shorthand),
			Property:      paramName,
			Kind:          kindFromTypeName(flagOverride.Type),
			Usage:         usage,
			// §P1: Required is preserved for positional bindings too. For
			// pure positional, cobra arity (MinimumNArgs) enforces presence
			// at parse time. For dual-mode positional (positional + alias),
			// validateRequiredPositionalBindings closes the loop in RunE
			// after both flag and positional injection, so MarkFlagRequired
			// is intentionally avoided.
			Required: flagOverride.Required,
			// §2.4: Default drives both cobra's --help "(default ...)"
			// rendering and (since v3.2) MCP body injection when the user
			// omits the flag. CollectBindings still gates writes by
			// user-changed flags, so user-provided values always win; the
			// normalizer's defaultInjects loop only fills missing keys.
			Default:         flagOverride.Default,
			Positional:      flagOverride.Positional,
			PositionalIndex: flagOverride.PositionalIndex,
		}

		// §v3.2: any non-empty default — hidden or visible — gets injected
		// when the user omits the flag. Earlier versions gated this on
		// flagOverride.Hidden which left visible flags with `"default": "0"`
		// (e.g. oa list-forms cursor) silently absent from the MCP body.
		if flagOverride.Default != "" {
			defaultInjects = append(defaultInjects, defaultInjectEntry{
				paramName:    paramName,
				defaultValue: flagOverride.Default,
				kind:         binding.Kind,
			})
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
		if rd := strings.TrimSpace(flagOverride.RuntimeDefault); rd != "" {
			if !runtimeDefaultWhitelist[rd] {
				fmt.Fprintf(os.Stderr, "[discovery] runtimeDefault: unknown placeholder %q on %s; ignoring\n", rd, paramName)
			} else {
				runtimeDefaults = append(runtimeDefaults, runtimeDefaultEntry{
					paramName:   paramName,
					placeholder: rd,
				})
			}
		}
		if mode := strings.TrimSpace(flagOverride.OmitWhen); mode != "" && mode != "empty" {
			omits[paramName] = omitEntry{paramName: paramName, mode: mode}
		}
	}

	// Check if we need a normalizer: transforms, env defaults, hidden defaults,
	// runtime defaults, omit-when overrides, dotted property paths, or body wrapper.
	needsDottedNesting := false
	for _, b := range bindings {
		if strings.Contains(b.Property, ".") {
			needsDottedNesting = true
			break
		}
	}
	bodyWrapper := strings.TrimSpace(override.BodyWrapper)
	if len(transforms) == 0 && len(envDefaults) == 0 && len(defaultInjects) == 0 && len(runtimeDefaults) == 0 && len(omits) == 0 && !needsDottedNesting && bodyWrapper == "" {
		return bindings, nil
	}

	// Build a normalizer that applies default injections + env defaults + runtime defaults
	// + transforms + omitWhen + nesting + body wrap.
	normalizer := func(cmd *cobra.Command, params map[string]any) error {
		// §v3.2: Apply envelope flag.default for parameters not explicitly set.
		// Coerce by Kind so number-typed schemas don't reject string defaults.
		for _, di := range defaultInjects {
			if _, exists := params[di.paramName]; exists {
				continue
			}
			defStr, defInt, defFloat, defBool, defSlice := parseFlagDefault(di.kind, di.defaultValue)
			switch di.kind {
			case ValueInt:
				params[di.paramName] = defInt
			case ValueFloat:
				params[di.paramName] = defFloat
			case ValueBool:
				params[di.paramName] = defBool
			case ValueStringSlice, ValueIntSlice, ValueFloatSlice, ValueBoolSlice:
				params[di.paramName] = defSlice
			default: // ValueString, ValueJSON, and any unknown kind
				params[di.paramName] = defStr
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

		// §v3.2.3: Apply runtime defaults (lowest priority, last default fill).
		if len(runtimeDefaults) > 0 {
			resolvers := runtimeDefaultResolvers()
			for _, rd := range runtimeDefaults {
				if _, exists := params[rd.paramName]; exists {
					continue
				}
				resolver, ok := resolvers[rd.placeholder]
				if !ok {
					// Whitelisted but no provider registered (common on
					// open-source core). Emit a single warning and move on.
					fmt.Fprintf(os.Stderr, "[discovery] runtimeDefault: no resolver registered for %s; skipping %s\n", rd.placeholder, rd.paramName)
					continue
				}
				if val, ok := resolver(cmd.Context()); ok && val != "" {
					params[rd.paramName] = val
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

		// §v3.2.2: Apply omitWhen — drop keys whose value meets the omit
		// condition for the declared mode. Default mode "empty" is already
		// handled implicitly by CollectBindings (empty string / empty slice
		// never enters params), so we only deal with "zero" and "never".
		for _, o := range omits {
			applyOmitWhen(params, o.paramName, o.mode)
		}

		// Nest dotted property paths: "Body.query" → params["Body"]["query"]
		nestDottedPaths(params)

		// §P2.bodyWrapper: wrap user-facing params under a single named key.
		// Internal control keys (prefixed with '_' e.g. _blocked, _yes) stay
		// at the top level so downstream confirmation logic keeps working.
		if bodyWrapper != "" {
			wrapParamsIntoBody(params, bodyWrapper)
		}

		return nil
	}

	return bindings, normalizer
}

// wrapParamsIntoBody moves every non-internal key from params into a new
// map stored under params[wrapper]. Internal keys (leading underscore) are
// preserved at the top level so the dispatcher / --yes logic still sees
// them. If params already contains params[wrapper] it is merged in first.
func wrapParamsIntoBody(params map[string]any, wrapper string) {
	if wrapper == "" {
		return
	}
	body := map[string]any{}
	if existing, ok := params[wrapper].(map[string]any); ok {
		for k, v := range existing {
			body[k] = v
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

// attachToGroup places cmd under the right parent based on the dotted group
// path. Empty group means attach directly to the overlay root.
func attachToGroup(root *cobra.Command, groupPath string, groupCmds map[string]*cobra.Command, cmd *cobra.Command) {
	gp := strings.TrimSpace(groupPath)
	if gp == "" {
		root.AddCommand(cmd)
		return
	}
	parent := resolveNestedGroup(root, gp, groupCmds)
	parent.AddCommand(cmd)
}

// buildRedirectCommand returns a stub leaf that prints "use: <target>" and
// performs no tool invocation. Accepts unknown flags/args so users hitting
// the old path get the redirect message instead of a parse error.
func buildRedirectCommand(name, description, target string) *cobra.Command {
	short := strings.TrimSpace(description)
	if short == "" {
		short = fmt.Sprintf("moved → %s", target)
	}
	cmd := &cobra.Command{
		Use:                name,
		Short:              short,
		Long:               fmt.Sprintf("This command has moved. Please use: %s", target),
		DisableFlagParsing: true,
		DisableAutoGenTag:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "This command has moved. Please use: %s\n", target)
			return nil
		},
	}
	return cmd
}

// buildHintCommand returns a stub sub-command that prints a redirect hint
// to the canonical command path declared by the overlay's hintCommands entry.
func buildHintCommand(name string, def market.CLIHintDef) *cobra.Command {
	target := strings.TrimSpace(def.Target)
	short := strings.TrimSpace(def.Description)
	if short == "" {
		if target != "" {
			short = fmt.Sprintf("hint: use %s", target)
		} else {
			short = "hint: see --help for the canonical command"
		}
	}
	cmd := &cobra.Command{
		Use:                name,
		Short:              short,
		Long:               short,
		DisableFlagParsing: true,
		DisableAutoGenTag:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if target != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Please use: %s\n", target)
			} else {
				_ = cmd.Help()
			}
			return nil
		},
	}
	return cmd
}

// applyFlagConstraints wires mutuallyExclusive / requireOneOf declarations
// onto the cobra command. Unknown flag names are logged and skipped so a
// stale/malformed overlay never blocks the entire command tree from building.
func applyFlagConstraints(cmd *cobra.Command, override market.CLIToolOverride) {
	validate := func(names []string) []string {
		valid := make([]string, 0, len(names))
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if cmd.Flags().Lookup(n) == nil {
				fmt.Fprintf(os.Stderr, "[discovery] flag constraint references unknown flag --%s on %q; skipping\n", n, cmd.Name())
				return nil
			}
			valid = append(valid, n)
		}
		return valid
	}

	for _, group := range override.MutuallyExclusive {
		names := validate(group)
		if len(names) < 2 {
			continue
		}
		cmd.MarkFlagsMutuallyExclusive(names...)
	}
	for _, group := range override.RequireOneOf {
		names := validate(group)
		if len(names) < 1 {
			continue
		}
		cmd.MarkFlagsOneRequired(names...)
	}
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

// kindFromTypeName maps the v3 CLIFlagOverride.Type declaration to the
// internal FlagBinding.Kind enum. Empty / unknown → ValueString (which keeps
// the v2 behaviour where every overlay flag was a plain string).
func kindFromTypeName(typeName string) ValueKind {
	switch strings.TrimSpace(strings.ToLower(typeName)) {
	case "int", "integer", "number":
		return ValueInt
	case "bool", "boolean":
		return ValueBool
	case "stringslice", "string_slice", "[]string":
		return ValueStringSlice
	case "string", "":
		return ValueString
	default:
		fmt.Fprintf(os.Stderr, "[discovery] flag type %q not recognised; defaulting to string\n", typeName)
		return ValueString
	}
}

// runtimeDefaultResolvers returns the edition-provided resolver map plus
// built-in fallbacks for $now / $today (which are trivially local). Overlays
// are expected to register the user-identity placeholders; $now / $today are
// always available.
func runtimeDefaultResolvers() map[string]edition.RuntimeDefaultFn {
	resolvers := make(map[string]edition.RuntimeDefaultFn, len(runtimeDefaultWhitelist))
	resolvers["$now"] = func(ctx context.Context) (string, bool) {
		return fmt.Sprintf("%d", time.Now().UnixMilli()), true
	}
	resolvers["$today"] = func(ctx context.Context) (string, bool) {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.FixedZone("CST", 8*3600)
		}
		return time.Now().In(loc).Format("2006-01-02"), true
	}
	if hooks := edition.Get(); hooks != nil && hooks.RuntimeDefaults != nil {
		for id, fn := range hooks.RuntimeDefaults() {
			if fn != nil {
				resolvers[id] = fn
			}
		}
	}
	return resolvers
}

// applyOmitWhen drops a key from params when its value meets the omit
// condition for the declared mode. "empty" (the default) is handled by
// CollectBindings upstream, so this function only processes "zero" and
// "never" — "never" is a marker that keeps the zero value explicit, so we
// do nothing for it.
func applyOmitWhen(params map[string]any, key, mode string) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "never":
		return
	case "zero":
		val, exists := params[key]
		if !exists {
			return
		}
		if isZeroValue(val) {
			delete(params, key)
		}
	default:
		// "empty" is the default, no-op.
	}
	// Emit a trace for anyone debugging envelope behaviour; kept at Debug so
	// it never leaks into the default CLI output.
	slog.Debug("applyOmitWhen", "key", key, "mode", mode)
}

func isZeroValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case bool:
		return !val
	case int:
		return val == 0
	case int64:
		return val == 0
	case float64:
		return val == 0
	case []any:
		return len(val) == 0
	case []string:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
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
