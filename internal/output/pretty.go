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

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fatih/color"
)

// writePretty renders the payload as ANSI-colored, human-readable text.
// If the payload looks like a `dws schema` response (has `kind: "schema"`),
// a schema-specific hierarchical renderer runs. Anything else falls back
// to the table renderer so `--format pretty` is safe on any command.
func writePretty(w io.Writer, payload any) error {
	normalized, err := normalizePayload(payload)
	if err != nil {
		return err
	}

	if m, ok := normalized.(map[string]any); ok {
		if kind, _ := m["kind"].(string); kind == "schema" {
			return writeSchemaPretty(w, m)
		}
	}
	return writeTableish(w, normalized)
}

// writeSchemaPretty handles both the list-all shape (has `products`) and
// the single-tool shape (has `tool`). Colours are on by default via fatih/color,
// auto-disabled when stdout is not a TTY or when NO_COLOR is set.
func writeSchemaPretty(w io.Writer, payload map[string]any) error {
	bold := color.New(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed, color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	if tool, ok := payload["tool"].(map[string]any); ok {
		return writeSchemaToolPretty(w, payload, tool, bold, cyan, green, yellow, red, dim)
	}
	if products, ok := payload["products"].([]any); ok {
		return writeSchemaListPretty(w, payload, products, bold, cyan, dim)
	}

	if degraded, _ := payload["degraded"].(bool); degraded {
		reason, _ := payload["reason"].(string)
		hint, _ := payload["hint"].(string)
		fmt.Fprintf(w, "%s discovery degraded: %s\n", red("!"), yellow(reason))
		if hint != "" {
			fmt.Fprintf(w, "  %s %s\n", dim("hint:"), hint)
		}
		return nil
	}
	return writeTableish(w, payload)
}

func writeSchemaListPretty(
	w io.Writer,
	payload map[string]any,
	products []any,
	bold, cyan, dim func(...any) string,
) error {
	count, _ := payload["count"].(float64)
	if count == 0 {
		count = float64(len(products))
	}
	fmt.Fprintf(w, "%s  %s products discovered\n", bold("Catalog"), cyan(fmt.Sprintf("%d", int(count))))

	for _, raw := range products {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := p["id"].(string)
		name, _ := p["name"].(string)
		desc, _ := p["description"].(string)
		tools, _ := p["tools"].([]any)
		fmt.Fprintf(w, "\n%s %s  %s\n", bold("▸"), bold(id), dim(name))
		if desc != "" && desc != name {
			fmt.Fprintf(w, "  %s\n", dim(desc))
		}
		fmt.Fprintf(w, "  %s %s\n", dim("tools:"), cyan(fmt.Sprintf("%d", len(tools))))
		shown := 0
		for _, t := range tools {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			rpc, _ := tm["name"].(string)
			cli, _ := tm["cli_name"].(string)
			if cli == "" || cli == rpc {
				fmt.Fprintf(w, "    - %s\n", rpc)
			} else {
				fmt.Fprintf(w, "    - %s %s\n", rpc, dim("→ "+cli))
			}
			shown++
			if shown >= 6 && len(tools) > 8 {
				fmt.Fprintf(w, "    %s\n", dim(fmt.Sprintf("… %d more", len(tools)-shown)))
				break
			}
		}
	}
	return nil
}

func writeSchemaToolPretty(
	w io.Writer,
	payload map[string]any,
	tool map[string]any,
	bold, cyan, green, yellow, red, dim func(...any) string,
) error {
	rpc, _ := tool["name"].(string)
	cli, _ := tool["cli_name"].(string)
	title, _ := tool["title"].(string)
	desc, _ := tool["description"].(string)
	group, _ := tool["group"].(string)
	canonical, _ := tool["canonical_path"].(string)

	header := rpc
	if title != "" && title != rpc {
		header = fmt.Sprintf("%s  %s", rpc, dim(title))
	}
	fmt.Fprintf(w, "%s %s\n", bold("Tool"), header)

	var productID string
	if product, ok := payload["product"].(map[string]any); ok {
		pid, _ := product["id"].(string)
		pname, _ := product["name"].(string)
		productID = pid
		fmt.Fprintf(w, "  %s %s  %s\n", dim("product:"), pid, dim(pname))
	}
	if canonical != "" {
		fmt.Fprintf(w, "  %s %s\n", dim("canonical:"), canonical)
	}
	if cli != "" {
		parts := []string{}
		if productID != "" {
			parts = append(parts, productID)
		}
		if group != "" {
			parts = append(parts, strings.Split(group, ".")...)
		}
		parts = append(parts, cli)
		fmt.Fprintf(w, "  %s %s\n", dim("cli path:"), cyan(strings.Join(parts, " ")))
	}

	// Sensitivity + annotations in one line.
	sensitive, _ := tool["sensitive"].(bool)
	if sensitive {
		fmt.Fprintf(w, "  %s %s\n", dim("sensitive:"), red("yes (needs --yes)"))
	}
	if ann, ok := tool["annotations"].(map[string]any); ok && len(ann) > 0 {
		var parts []string
		for _, key := range sortedMapKeys(ann) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, ann[key]))
		}
		fmt.Fprintf(w, "  %s %s\n", dim("annotations:"), yellow(strings.Join(parts, " ")))
	}

	if desc != "" && desc != title {
		fmt.Fprintln(w)
		for _, line := range strings.Split(strings.TrimSpace(desc), "\n") {
			fmt.Fprintf(w, "  %s\n", dim(line))
		}
	}

	// Parameters section.
	if params, ok := tool["parameters"].(map[string]any); ok && len(params) > 0 {
		required := map[string]bool{}
		if req, ok := tool["required"].([]any); ok {
			for _, r := range req {
				if s, ok := r.(string); ok {
					required[s] = true
				}
			}
		}
		overlay := map[string]map[string]any{}
		if ov, ok := tool["flag_overlay"].(map[string]any); ok {
			for name, v := range ov {
				if m, ok := v.(map[string]any); ok {
					overlay[name] = m
				}
			}
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s\n", bold("Parameters"))
		for _, name := range sortedMapKeys(params) {
			prop, _ := params[name].(map[string]any)
			writeParamPretty(w, name, prop, required[name], overlay[name],
				bold, cyan, green, yellow, red, dim)
		}
	}

	// Output schema, if any.
	if out, ok := tool["output_schema"].(map[string]any); ok && len(out) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s\n", bold("Output schema"))
		b, _ := json.MarshalIndent(out, "  ", "  ")
		fmt.Fprintf(w, "  %s\n", string(b))
	}

	return nil
}

func writeParamPretty(
	w io.Writer,
	name string,
	prop map[string]any,
	required bool,
	overlay map[string]any,
	bold, cyan, green, yellow, red, dim func(...any) string,
) {
	typeStr := describeType(prop)
	marker := " "
	if required {
		marker = red("*")
	}

	alias, _ := overlay["alias"].(string)
	line := fmt.Sprintf(" %s %s", marker, bold(name))
	if alias != "" && alias != name {
		line += fmt.Sprintf(" %s", cyan("--"+alias))
	}
	line += fmt.Sprintf("  %s", dim(typeStr))
	fmt.Fprintln(w, line)

	if d, _ := prop["description"].(string); d != "" {
		for _, ln := range strings.Split(strings.TrimSpace(d), "\n") {
			fmt.Fprintf(w, "     %s\n", dim(ln))
		}
	}

	// enum values — render inline.
	if enum, ok := prop["enum"].([]any); ok && len(enum) > 0 {
		vals := make([]string, 0, len(enum))
		for _, v := range enum {
			vals = append(vals, fmt.Sprintf("%v", v))
		}
		fmt.Fprintf(w, "     %s %s\n", dim("enum:"), green(strings.Join(vals, ", ")))
	}

	// overlay — transform / env default / default.
	if transform, _ := overlay["transform"].(string); transform != "" {
		extras := transform
		if args, ok := overlay["transform_args"].(map[string]any); ok && len(args) > 0 {
			kvs := make([]string, 0, len(args))
			for _, k := range sortedMapKeys(args) {
				kvs = append(kvs, fmt.Sprintf("%s=%v", k, args[k]))
			}
			extras += "(" + strings.Join(kvs, ", ") + ")"
		}
		fmt.Fprintf(w, "     %s %s\n", dim("transform:"), yellow(extras))
	}
	if env, _ := overlay["env_default"].(string); env != "" {
		fmt.Fprintf(w, "     %s %s\n", dim("env default:"), green("$"+env))
	}
	if def, _ := overlay["default"].(string); def != "" {
		fmt.Fprintf(w, "     %s %s\n", dim("default:"), green(def))
	}
}

func describeType(prop map[string]any) string {
	t, _ := prop["type"].(string)
	if t == "array" {
		if items, ok := prop["items"].(map[string]any); ok {
			if inner, _ := items["type"].(string); inner != "" {
				return inner + "[]"
			}
		}
		return "array"
	}
	if t == "" {
		return "any"
	}
	return t
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
