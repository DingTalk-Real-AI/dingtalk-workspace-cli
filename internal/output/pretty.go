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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
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
	if tool, ok := payload["tool"].(map[string]any); ok {
		return writeSchemaToolPretty(w, payload, tool)
	}
	if products, ok := payload["products"].([]any); ok {
		return writeSchemaListPretty(w, payload, products)
	}

	if degraded, _ := payload["degraded"].(bool); degraded {
		reason, _ := payload["reason"].(string)
		hint, _ := payload["hint"].(string)
		fmt.Fprintf(w, "%s discovery degraded: %s\n", tui.StateMark("warning"), tui.Warning(reason))
		if hint != "" {
			fmt.Fprintf(w, "  %s %s\n", tui.Key("hint"), hint)
		}
		return nil
	}
	return writeTableish(w, payload)
}

func writeSchemaListPretty(
	w io.Writer,
	payload map[string]any,
	products []any,
) error {
	count, _ := payload["count"].(float64)
	if count == 0 {
		count = float64(len(products))
	}
	fmt.Fprintf(w, "%s\n", tui.Header("Catalog", fmt.Sprintf("%d products discovered", int(count))))
	fmt.Fprintf(w, "%s\n", tui.Rule(64))

	for _, raw := range products {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := p["id"].(string)
		name, _ := p["name"].(string)
		desc, _ := p["description"].(string)
		tools, _ := p["tools"].([]any)
		fmt.Fprintf(w, "\n%s %s  %s\n", tui.StateMark("ok"), tui.Bold(id), tui.Dim(name))
		if desc != "" && desc != name {
			fmt.Fprintf(w, "  %s\n", tui.Dim(desc))
		}
		fmt.Fprintf(w, "  %s %s\n", tui.Key("tools"), tui.Cyan(fmt.Sprintf("%d", len(tools))))
		shown := 0
		for _, t := range tools {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			rpc, _ := tm["name"].(string)
			cli, _ := tm["cli_name"].(string)
			if cli == "" || cli == rpc {
				fmt.Fprintf(w, "    %s %s\n", tui.Bullet(), rpc)
			} else {
				fmt.Fprintf(w, "    %s %s %s %s\n", tui.Bullet(), rpc, tui.Arrow(), tui.Dim(cli))
			}
			shown++
			if shown >= 6 && len(tools) > 8 {
				fmt.Fprintf(w, "    %s\n", tui.Dim(fmt.Sprintf("… %d more", len(tools)-shown)))
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
) error {
	rpc, _ := tool["name"].(string)
	cli, _ := tool["cli_name"].(string)
	title, _ := tool["title"].(string)
	desc, _ := tool["description"].(string)
	group, _ := tool["group"].(string)
	canonical, _ := tool["canonical_path"].(string)

	header := rpc
	if title != "" && title != rpc {
		header = fmt.Sprintf("%s  %s", rpc, tui.Dim(title))
	}
	fmt.Fprintf(w, "%s\n", tui.Header("Tool "+header, "schema"))
	fmt.Fprintf(w, "%s\n", tui.Rule(72))

	var productID string
	if product, ok := payload["product"].(map[string]any); ok {
		pid, _ := product["id"].(string)
		pname, _ := product["name"].(string)
		productID = pid
		fmt.Fprintf(w, "  %s %s  %s\n", tui.Key("product"), tui.Cyan(pid), tui.Dim(pname))
	}
	if canonical != "" {
		fmt.Fprintf(w, "  %s %s\n", tui.Key("canonical"), canonical)
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
		fmt.Fprintf(w, "  %s %s\n", tui.Key("cli path"), tui.Cyan(strings.Join(parts, " ")))
	}

	// Sensitivity + annotations in one line.
	sensitive, _ := tool["sensitive"].(bool)
	if sensitive {
		fmt.Fprintf(w, "  %s %s\n", tui.Key("sensitive"), tui.Danger("yes (needs --yes)"))
	}
	if ann, ok := tool["annotations"].(map[string]any); ok && len(ann) > 0 {
		var parts []string
		for _, key := range sortedMapKeys(ann) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, ann[key]))
		}
		fmt.Fprintf(w, "  %s %s\n", tui.Key("annotations"), tui.Warning(strings.Join(parts, " ")))
	}

	if desc != "" && desc != title {
		fmt.Fprintln(w)
		for _, line := range strings.Split(strings.TrimSpace(desc), "\n") {
			fmt.Fprintf(w, "  %s\n", tui.Dim(line))
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
		fmt.Fprintf(w, "%s\n", tui.Section("Parameters"))
		for _, name := range sortedMapKeys(params) {
			prop, _ := params[name].(map[string]any)
			writeParamPretty(w, name, prop, required[name], overlay[name])
		}
	}

	// Output schema, if any.
	if out, ok := tool["output_schema"].(map[string]any); ok && len(out) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s\n", tui.Section("Output schema"))
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
) {
	typeStr := describeType(prop)
	marker := " "
	if required {
		marker = tui.Danger("*")
	}

	alias, _ := overlay["alias"].(string)
	line := fmt.Sprintf(" %s %s", marker, tui.Bold(name))
	if alias != "" && alias != name {
		line += fmt.Sprintf(" %s", tui.Cyan("--"+alias))
	}
	line += fmt.Sprintf("  %s", tui.Dim(typeStr))
	fmt.Fprintln(w, line)

	if d, _ := prop["description"].(string); d != "" {
		for _, ln := range strings.Split(strings.TrimSpace(d), "\n") {
			fmt.Fprintf(w, "     %s\n", tui.Dim(ln))
		}
	}

	// enum values — render inline.
	if enum, ok := prop["enum"].([]any); ok && len(enum) > 0 {
		vals := make([]string, 0, len(enum))
		for _, v := range enum {
			vals = append(vals, fmt.Sprintf("%v", v))
		}
		fmt.Fprintf(w, "     %s %s\n", tui.Key("enum"), tui.Success(strings.Join(vals, ", ")))
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
		fmt.Fprintf(w, "     %s %s\n", tui.Key("transform"), tui.Warning(extras))
	}
	if env, _ := overlay["env_default"].(string); env != "" {
		fmt.Fprintf(w, "     %s %s\n", tui.Key("env default"), tui.Success("$"+env))
	}
	if def, _ := overlay["default"].(string); def != "" {
		fmt.Fprintf(w, "     %s %s\n", tui.Key("default"), tui.Success(def))
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
