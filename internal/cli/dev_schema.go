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
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// helperSchemaRoots are top-level command names whose subtrees are helper-only
// hard-coded cobra commands. `dws schema` still answers for them, but the schema
// CONTENT is fetched LIVE from the helper's pinned MCP server (op-app) and rendered in the flat
// gws-aligned shape — never synthesized from local cobra flags, never
// hardcoded. The mapping from a leaf command to its MCP tool comes from the
// `mcp-tool` cobra annotation set in internal/helpers/devapp.go.
var helperSchemaRoots = map[string]bool{"dev": true}

// HelperToolSchema is the live op-app tool schema the renderer needs: the raw
// MCP description plus the inputSchema's properties/required, exactly as the
// server returned them (no local transformation of CONTENT).
type HelperToolSchema struct {
	Name        string
	Description string
	Properties  map[string]any // MCP param name → property object {type,description,default?,...}
	Required    []string       // MCP param names that are required
}

// HelperToolFetcher loads a helper MCP server's tools/list LIVE and returns
// toolName→schema for the given source (e.g. "op-app" for dev app commands,
// "devdoc" for dev doc commands). The schema command injects this so
// dev_schema.go can resolve a command's MCP tool and render its real schema
// without the cli package importing app/transport. Implementations should
// cache per-source per-process so repeated `dws schema dev.*` only hit the
// network once per source.
type HelperToolFetcher func(ctx context.Context, source string) (map[string]HelperToolSchema, error)

// renderHelperSchema builds the `dws schema` payload for helper-only command
// subtrees. Returns (payload, true) when the path targets a helper subtree;
// (nil, false) otherwise so the caller falls back to runtime command schema.
//
// Leaf commands render the same gws-flat object as runtime schema leaves, with
// CONTENT pulled live from the MCP tool named by the command's `mcp-tool`
// annotation.
// Group/root paths render a browse listing {path, commands:[...]} from the
// cobra tree (no MCP needed).
func renderHelperSchema(ctx context.Context, root *cobra.Command, rawPath string, fetch HelperToolFetcher) (map[string]any, bool, error) {
	if root == nil {
		return nil, false, nil
	}
	tokens := splitSchemaPathTokens(rawPath)
	if len(tokens) == 0 || !helperSchemaRoots[tokens[0]] {
		return nil, false, nil
	}
	helperRoot, _, err := root.Find([]string{tokens[0]})
	if err != nil || helperRoot == nil || !helperRoot.HasParent() {
		return nil, false, nil
	}
	if strings.Contains(rawPath, ".") && len(tokens) == 2 {
		if leaf, ok := helperLeafByToolName(helperRoot, tokens[1]); ok {
			payload, err := helperLeafSchema(ctx, leaf, fetch)
			return payload, true, err
		}
		return nil, false, nil
	}

	target, rest, err := root.Find(tokens)
	if err != nil || target == nil {
		target = root
		rest = tokens[1:]
	}
	// Find resolves to the deepest matching command and returns trailing tokens
	// it couldn't match as (sub)commands. Any non-flag leftover means a typo'd
	// or unknown subcommand — surface it with the closest group's children.
	if unknown := firstNonFlag(rest); unknown != "" {
		return map[string]any{
			"path":      rawPath,
			"error":     "unknown subcommand \"" + unknown + "\" under \"" + helperCommandPath(target) + "\"",
			"available": helperSubcommands(target),
		}, true, nil
	}

	// A runnable leaf → emit its live MCP schema in gws-flat shape.
	// A group → browse its subcommands.
	if target.Runnable() && !target.HasAvailableSubCommands() {
		if !helperLeafHasMCPTool(target) {
			return nil, false, nil
		}
		payload, err := helperLeafSchema(ctx, target, fetch)
		return payload, true, err
	}

	return map[string]any{
		"path":     helperCommandPath(target),
		"commands": helperSubcommands(target),
	}, true, nil
}

// helperLeafSchema renders a single leaf command as the gws-flat object,
// fetching its MCP tool schema live. The command must carry an `mcp-tool`
// annotation; local helper commands without one are not part of `dws schema`.
func helperLeafSchema(ctx context.Context, cmd *cobra.Command, fetch HelperToolFetcher) (map[string]any, error) {
	toolName, source := helperLeafToolBinding(cmd)
	// Default source is op-app (dev app commands); dev doc commands annotate
	// mcp-source=devdoc to pull from the devdoc MCP server instead.
	if source == "" {
		source = "op-app"
	}
	path := helperCommandPath(cmd)
	if toolName == "" {
		return map[string]any{
			"path":  path,
			"error": "no MCP tool bound to this command; schema is unavailable",
		}, nil
	}
	if fetch == nil {
		return map[string]any{
			"path":  path,
			"error": "live MCP schema fetcher not configured",
		}, nil
	}

	tools, err := fetch(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("fetch %s tool schemas: %w", source, err)
	}
	tool, ok := tools[toolName]
	if !ok {
		return map[string]any{
			"path":  path,
			"error": fmt.Sprintf("MCP tool %q not found in %s tools/list", toolName, source),
		}, nil
	}

	meta := helperSchemaLeafForCommand(cmd)
	productID := meta.ProductID
	if productID == "" {
		productID = helperProductID(cmd)
	}
	canonicalPath := meta.CanonicalPath
	if canonicalPath == "" {
		canonicalPath = productID + "." + toolName
	}
	hint := schemaHintForCanonicalPath(canonicalPath)
	parameters := helperFlatParameters(tool, cmd, hint.Parameters)
	if parameters == nil {
		parameters = map[string]any{}
	}
	primaryCLIPath := meta.PrimaryCLIPath
	if primaryCLIPath == "" {
		primaryCLIPath = path
	}
	payload := map[string]any{
		"name":             toolName,
		"cli_name":         cmd.Name(),
		"canonical_path":   canonicalPath,
		"path":             canonicalPath,
		"cli_path":         path,
		"primary_cli_path": primaryCLIPath,
		"is_alias":         meta.IsAlias,
		"source":           "mcp:" + source,
		"product_id":       productID,
		"display":          helperProductDisplay(cmd),
		"title":            strings.TrimSpace(cmd.Short),
		"description":      tool.Description,
		"parameters":       parameters,
		"has_parameters":   len(parameters) > 0,
		"parameter_count":  len(parameters),
	}
	if len(meta.Aliases) > 0 {
		payload["aliases"] = meta.Aliases
	}
	return payload, nil
}

// helperFlatParameters projects an MCP tool's inputSchema into the gws-flat
// per-parameter object. Keys are kebab-case of the MCP param name when that key
// is an actual flag on the current helper leaf; live MCP-only parameters that
// are fixed internally by the helper command are intentionally omitted so schema
// output remains executable as CLI.
func helperFlatParameters(tool HelperToolSchema, cmd *cobra.Command, hints map[string]ParameterSchemaHint) map[string]any {
	required := make(map[string]bool, len(tool.Required))
	for _, r := range tool.Required {
		required[r] = true
	}

	params := make(map[string]any, len(tool.Properties))
	for name, raw := range tool.Properties {
		flagName := kebabCase(name)
		if !helperCommandHasFlag(cmd, flagName) {
			continue
		}
		hint, _, hasHint := lookupParameterSchemaHint(hints, name, flagName)
		if hasHint && strings.TrimSpace(hint.FlagName) != "" {
			flagName = strings.TrimSpace(hint.FlagName)
			if !helperCommandHasFlag(cmd, flagName) {
				continue
			}
		}
		prop, _ := raw.(map[string]any)
		paramType := mcpJSONType(prop)
		if hasHint && strings.TrimSpace(hint.Type) != "" {
			paramType = strings.TrimSpace(hint.Type)
		}
		description := mcpString(prop, "description")
		if hasHint && strings.TrimSpace(hint.Description) != "" {
			description = strings.TrimSpace(hint.Description)
		}
		paramRequired := required[name] || helperFlagRequired(cmd, flagName)
		if hasHint && hint.Required != nil {
			paramRequired = *hint.Required
		}
		entry := map[string]any{
			"type":        paramType,
			"description": description,
			"required":    paramRequired,
			"property":    name,
		}
		if hasHint && strings.TrimSpace(hint.Default) != "" {
			entry["default"] = strings.TrimSpace(hint.Default)
		} else if def, ok := mcpDefault(prop); ok {
			entry["default"] = def
		}
		params[flagName] = entry
	}
	return params
}

func helperFlagRequired(cmd *cobra.Command, flagName string) bool {
	if cmd == nil || strings.TrimSpace(flagName) == "" {
		return false
	}
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(flagName)
	}
	if flag == nil {
		return false
	}
	return usageImpliesRequired(flag.Usage)
}

func helperCommandHasFlag(cmd *cobra.Command, flagName string) bool {
	if cmd == nil || strings.TrimSpace(flagName) == "" {
		return false
	}
	for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		if flags != nil && flags.Lookup(flagName) != nil {
			return true
		}
	}
	return false
}

// mcpJSONType normalizes the MCP property "type" to a JSON-type string. MCP
// reports standard JSON Schema types; pass them through, defaulting to "string"
// when absent/unknown so the contract is always populated.
func mcpJSONType(prop map[string]any) string {
	t, _ := prop["type"].(string)
	switch t {
	case "string", "integer", "number", "boolean", "array", "object":
		return t
	default:
		return "string"
	}
}

// mcpString reads a string field from an MCP property object.
func mcpString(prop map[string]any, key string) string {
	if prop == nil {
		return ""
	}
	v, _ := prop[key].(string)
	return v
}

// mcpDefault returns the MCP-provided default, stringified, only when present.
// gws renders default as a string; mirror that. Non-string JSON defaults
// (numbers/bools) are formatted with %v so e.g. 0 → "0", true → "true".
func mcpDefault(prop map[string]any) (string, bool) {
	if prop == nil {
		return "", false
	}
	v, ok := prop["default"]
	if !ok || v == nil {
		return "", false
	}
	switch tv := v.(type) {
	case string:
		return tv, true
	case float64:
		// JSON numbers decode to float64; render integers without a fraction.
		if tv == float64(int64(tv)) {
			return fmt.Sprintf("%d", int64(tv)), true
		}
		return fmt.Sprintf("%v", tv), true
	default:
		return fmt.Sprintf("%v", tv), true
	}
}

// kebabCase converts an MCP camelCase param name to the CLI flag's kebab form,
// matching how flags are registered in internal/helpers/devapp.go:
//
//	eventCallbackUrl → event-callback-url
//	unifiedAppId     → unified-app-id
//	disableSSLVerify → disable-ssl-verify
//
// A boundary is inserted before an uppercase letter that follows a lowercase
// letter or digit, and before the final uppercase of a run that starts a new
// lowercase word (so SSLVerify → ssl-verify, not s-s-l-verify).
func kebabCase(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prevLowerOrDigit := i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]))
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if i > 0 && (prevLowerOrDigit || nextLower) {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	// Collapse any accidental double dashes and trim, just in case the source
	// already contained separators.
	out := strings.ReplaceAll(b.String(), "_", "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}

type helperSchemaLeaf struct {
	Command        *cobra.Command
	ProductID      string
	ToolName       string
	Source         string
	CanonicalPath  string
	CLIPath        string
	PrimaryCLIPath string
	Aliases        []string
	IsAlias        bool
}

func collectHelperSchemaLeaves(top *cobra.Command) []helperSchemaLeaf {
	if top == nil {
		return nil
	}
	productID := helperProductID(top)
	leaves := []helperSchemaLeaf{}
	walkLeafCommands(top, func(leaf *cobra.Command) {
		toolName, source := helperLeafToolBinding(leaf)
		if toolName == "" {
			return
		}
		if source == "" {
			source = "op-app"
		}
		leaves = append(leaves, helperSchemaLeaf{
			Command:       leaf,
			ProductID:     productID,
			ToolName:      toolName,
			Source:        source,
			CanonicalPath: productID + "." + toolName,
			CLIPath:       helperCommandPath(leaf),
		})
	})
	sort.Slice(leaves, func(i, j int) bool {
		if leaves[i].CanonicalPath != leaves[j].CanonicalPath {
			return leaves[i].CanonicalPath < leaves[j].CanonicalPath
		}
		return leaves[i].CLIPath < leaves[j].CLIPath
	})
	annotateHelperSchemaAliases(leaves)
	return leaves
}

func annotateHelperSchemaAliases(leaves []helperSchemaLeaf) {
	groups := map[string][]int{}
	for idx, leaf := range leaves {
		groups[leaf.CanonicalPath] = append(groups[leaf.CanonicalPath], idx)
	}
	for _, indexes := range groups {
		primary := choosePrimaryHelperLeaf(leaves, indexes)
		aliases := make([]string, 0, len(indexes)-1)
		for _, idx := range indexes {
			if idx == primary {
				continue
			}
			aliases = append(aliases, leaves[idx].CLIPath)
		}
		sort.Strings(aliases)
		primaryPath := leaves[primary].CLIPath
		for _, idx := range indexes {
			leaves[idx].PrimaryCLIPath = primaryPath
			leaves[idx].Aliases = append([]string(nil), aliases...)
			leaves[idx].IsAlias = idx != primary
		}
	}
}

func choosePrimaryHelperLeaf(leaves []helperSchemaLeaf, indexes []int) int {
	if len(indexes) == 0 {
		return 0
	}
	if primaryHint := schemaPrimaryCLIPath(leaves[indexes[0]].ProductID, leaves[indexes[0]].ToolName); primaryHint != "" {
		for _, idx := range indexes {
			if leaves[idx].CLIPath == primaryHint {
				return idx
			}
		}
	}
	toolParts := map[string]bool{}
	for _, part := range strings.FieldsFunc(leaves[indexes[0]].ToolName, func(r rune) bool { return r == '_' || r == '-' }) {
		if part != "" {
			toolParts[part] = true
		}
	}
	for _, idx := range indexes {
		if toolParts[leaves[idx].Command.Name()] {
			return idx
		}
	}
	return indexes[0]
}

func helperSchemaLeafForCommand(cmd *cobra.Command) helperSchemaLeaf {
	top := topLevelCommand(cmd)
	for _, leaf := range collectHelperSchemaLeaves(top) {
		if leaf.Command == cmd {
			return leaf
		}
	}
	return helperSchemaLeaf{}
}

func helperLeafByToolName(top *cobra.Command, toolName string) (*cobra.Command, bool) {
	canonicalPath := helperProductID(top) + "." + strings.TrimSpace(toolName)
	for _, leaf := range collectHelperSchemaLeaves(top) {
		if leaf.CanonicalPath == canonicalPath && !leaf.IsAlias {
			return leaf.Command, true
		}
	}
	return nil, false
}

// helperProductSummaries returns light product entries for every helper-only
// subtree, appended to the no-arg `dws schema` product listing so agents
// browsing all products also see helper commands. Tools are listed by path +
// summary only; drill in with `dws schema "<path>"` for full parameter schema.
func helperProductSummaries(root *cobra.Command) []map[string]any {
	if root == nil {
		return nil
	}
	out := []map[string]any{}
	for name := range helperSchemaRoots {
		top, _, err := root.Find([]string{name})
		if err != nil || top == nil || !top.HasParent() {
			continue
		}
		leafEntries := collectHelperSchemaLeaves(top)
		tools := []map[string]any{}
		for _, leaf := range leafEntries {
			if leaf.IsAlias {
				continue
			}
			tool := map[string]any{
				"name":             leaf.ToolName,
				"cli_name":         leaf.Command.Name(),
				"canonical_path":   leaf.CanonicalPath,
				"cli_path":         leaf.CLIPath,
				"primary_cli_path": leaf.PrimaryCLIPath,
				"source":           "mcp:" + leaf.Source,
				"description":      strings.TrimSpace(leaf.Command.Short),
			}
			if len(leaf.Aliases) > 0 {
				tool["aliases"] = leaf.Aliases
			}
			tools = append(tools, tool)
		}
		if len(tools) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"id":          name,
			"name":        strings.TrimSpace(top.Short),
			"description": "helper-only 命令组；schema 从 op-app MCP 实时拉取，用 `dws schema \"" + helperCommandPath(top) + " ...\"` 查具体参数",
			"helper":      true,
			"tools":       tools,
		})
	}
	return out
}

// walkLeafCommands invokes fn for every runnable leaf under cmd (depth-first).
func walkLeafCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd.Runnable() && !cmd.HasAvailableSubCommands() {
		fn(cmd)
		return
	}
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() || sub.Name() == "help" {
			continue
		}
		walkLeafCommands(sub, fn)
	}
}

// helperSubcommands lists a group's runnable children for browse mode, sorted
// by name for deterministic output.
func helperSubcommands(cmd *cobra.Command) []map[string]any {
	out := []map[string]any{}
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() || sub.Name() == "help" {
			continue
		}
		if !helperCommandHasSchemaLeaf(sub) {
			continue
		}
		out = append(out, map[string]any{
			"cli_path":    helperCommandPath(sub),
			"description": strings.TrimSpace(sub.Short),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["cli_path"].(string) < out[j]["cli_path"].(string)
	})
	return out
}

func helperLeafToolBinding(cmd *cobra.Command) (toolName, source string) {
	if cmd == nil || cmd.Annotations == nil {
		return "", ""
	}
	return strings.TrimSpace(cmd.Annotations["mcp-tool"]), strings.TrimSpace(cmd.Annotations["mcp-source"])
}

func helperLeafHasMCPTool(cmd *cobra.Command) bool {
	toolName, _ := helperLeafToolBinding(cmd)
	return toolName != ""
}

func helperCommandHasSchemaLeaf(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Runnable() && !cmd.HasAvailableSubCommands() {
		return helperLeafHasMCPTool(cmd)
	}
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() || sub.Name() == "help" {
			continue
		}
		if helperCommandHasSchemaLeaf(sub) {
			return true
		}
	}
	return false
}

func helperProductID(cmd *cobra.Command) string {
	parts := splitSchemaPathTokens(helperCommandPath(cmd))
	if len(parts) > 0 {
		return parts[0]
	}
	return "helper"
}

func helperProductDisplay(cmd *cobra.Command) string {
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		if !c.Parent().HasParent() {
			return strings.TrimSpace(c.Short)
		}
	}
	return ""
}

// helperCommandPath returns the space-joined path from root to cmd, e.g.
// "dev app robot config".
func helperCommandPath(cmd *cobra.Command) string {
	parts := []string{}
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}

// firstNonFlag returns the first token that is not a flag (does not start with
// "-"), or "" if there is none.
func firstNonFlag(tokens []string) string {
	for _, t := range tokens {
		if t != "" && !strings.HasPrefix(t, "-") {
			return t
		}
	}
	return ""
}
