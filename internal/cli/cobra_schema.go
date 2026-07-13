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
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cobraSchemaRoots are top-level command names whose subtrees answer `dws schema`
// by SYNTHESIZING the machine-readable input schema from their cobra flags. This
// is the local-command counterpart to helperSchemaRoots: helper subtrees (dev)
// fetch CONTENT live from an MCP server, whereas these commands have no MCP
// backing so the schema is built from the flags the binary actually registered.
//
// The output shape is the same flat object helper leaves emit — {description,
// path, source, parameters{<flag>:{type,description,required,default?}}} — so an
// agent gets one consistent schema format no matter the source; `source` is
// "cobra" to mark it synthesized from flags (vs "mcp:<server>"). event is the
// first consumer; register more command trees here as they adopt the contract.
var cobraSchemaRoots = map[string]bool{"event": true}

// renderCobraSchema builds the `dws schema` payload for command subtrees listed
// in cobraSchemaRoots. Mirrors renderHelperSchema's routing: returns
// (payload, true) when the path targets a registered subtree so the caller
// skips the static-mode fallback; (nil, false) otherwise.
//
// A runnable leaf renders the flat parameter object synthesized from its flags
// (plus positional arguments parsed from its Use line). A group/root renders the
// same browse listing helper groups use.
func renderCobraSchema(root *cobra.Command, rawPath string) (map[string]any, bool, error) {
	if root == nil {
		return nil, false, nil
	}
	tokens := splitSchemaPathTokens(rawPath)
	if len(tokens) == 0 || !cobraSchemaRoots[tokens[0]] {
		return nil, false, nil
	}

	target, rest, err := root.Find(tokens)
	if err != nil || target == nil {
		target = root
		rest = tokens[1:]
	}
	// Any non-flag leftover token means an unknown subcommand — surface it with
	// the closest group's children, same as renderHelperSchema.
	if unknown := firstNonFlag(rest); unknown != "" {
		return map[string]any{
			"path":      rawPath,
			"error":     "unknown subcommand \"" + unknown + "\" under \"" + helperCommandPath(target) + "\"",
			"available": helperSubcommands(target),
		}, true, nil
	}

	if target.Runnable() && !target.HasAvailableSubCommands() {
		return cobraLeafSchema(target), true, nil
	}
	return map[string]any{
		"path":     helperCommandPath(target),
		"commands": helperSubcommands(target),
	}, true, nil
}

// cobraLeafSchema renders one runnable command as the flat schema object,
// synthesizing parameters from its flags and (when present) positional
// arguments from its Use line.
func cobraLeafSchema(cmd *cobra.Command) map[string]any {
	out := map[string]any{
		"description": strings.TrimSpace(cmd.Short),
		"path":        helperCommandPath(cmd),
		"source":      "cobra",
		"parameters":  cobraFlatParameters(cmd),
	}
	if args := cobraPositionalArgs(cmd); len(args) > 0 {
		out["arguments"] = args
	}
	return out
}

// cobraFlatParameters projects a command's LOCAL flags into the flat
// per-parameter object. Local (non-inherited) flags are the command-specific
// inputs; global persistent flags inherited from the root (--profile, --verbose,
// --jq, …) are intentionally excluded so the schema describes THIS command, not
// the whole CLI. Hidden internal flags are skipped. Each entry is
// {type, description, required, default?} with type mapped to a JSON-type
// string, required read from cobra's required-flag annotation, and default only
// when the flag has a meaningful (non-zero) default.
func cobraFlatParameters(cmd *cobra.Command) map[string]any {
	params := map[string]any{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		entry := map[string]any{
			"type":        pflagJSONType(f),
			"description": strings.TrimSpace(f.Usage),
			"required":    flagIsRequired(f),
		}
		if def, ok := meaningfulDefault(f); ok {
			entry["default"] = def
		}
		params[f.Name] = entry
	})
	return params
}

// cobraPositionalArgs parses a command's Use line into structured positional
// arguments. Cobra has no typed metadata for positionals, so the Use string
// ("consume [event_key]", "stop [subscribe_id]") is the source of truth:
// tokens after the command name are positional slots. <name> is required,
// [name] is optional, a trailing "..." marks it variadic. Returns nil when the
// command declares no positionals (e.g. flag-only commands), so the leaf object
// simply omits the "arguments" field.
func cobraPositionalArgs(cmd *cobra.Command) []map[string]any {
	fields := strings.Fields(cmd.Use)
	if len(fields) <= 1 {
		return nil
	}
	out := []map[string]any{}
	for _, tok := range fields[1:] {
		variadic := strings.Contains(tok, "...")
		required := strings.HasPrefix(tok, "<")
		name := strings.Trim(tok, "[]<>.")
		if name == "" {
			continue
		}
		arg := map[string]any{
			"name":     name,
			"required": required,
		}
		if variadic {
			arg["variadic"] = true
		}
		out = append(out, arg)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// pflagJSONType maps a pflag value type to a JSON-type string. Duration is
// expressed as "string" because it is entered as a CLI string ("10m"); slice
// and array types collapse to "array"; the numeric families collapse to
// "integer"/"number". Unknown types default to "string" so the contract is
// always populated.
func pflagJSONType(f *pflag.Flag) string {
	switch f.Value.Type() {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "count":
		return "integer"
	case "float32", "float64":
		return "number"
	case "stringSlice", "stringArray", "intSlice", "int32Slice", "int64Slice",
		"uintSlice", "float32Slice", "float64Slice", "boolSlice", "durationSlice":
		return "array"
	default:
		// string, duration, ip, and any custom Value type read as a string.
		return "string"
	}
}

// flagIsRequired reports whether cobra.MarkFlagRequired was applied to the flag
// (it records the requirement in the flag's annotations under
// cobra.BashCompOneRequiredFlag). Event's conditionally-required inputs
// (--user / --group depend on the event key) are enforced at runtime, not via
// this annotation, so they read as required:false here — the dependency is
// documented in the command help, not the flag metadata.
func flagIsRequired(f *pflag.Flag) bool {
	if f.Annotations == nil {
		return false
	}
	vals, ok := f.Annotations[cobra.BashCompOneRequiredFlag]
	return ok && len(vals) == 1 && vals[0] == "true"
}

// meaningfulDefault returns a flag's default only when it is a real value, not
// the zero/unset sentinel ("", "0", "0s", "false", "[]"). A zero default means
// "no default — the value comes from you", so omitting it matches how the helper
// renderer omits absent MCP defaults and keeps the schema free of noise.
func meaningfulDefault(f *pflag.Flag) (string, bool) {
	def := strings.TrimSpace(f.DefValue)
	switch def {
	case "", "0", "0s", "false", "[]", "{}":
		return "", false
	default:
		return def, true
	}
}
