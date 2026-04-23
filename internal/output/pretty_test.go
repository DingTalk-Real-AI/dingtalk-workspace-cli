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
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// forceNoColor disables ANSI codes for assertion simplicity. fatih/color
// would already do this when writing to a non-TTY, but tests use bytes.Buffer
// so we flip the flag explicitly.
func forceNoColor(t *testing.T) {
	t.Helper()
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })
}

func TestPretty_SchemaListRendersProducts(t *testing.T) {
	forceNoColor(t)

	payload := map[string]any{
		"kind":  "schema",
		"count": 2,
		"products": []any{
			map[string]any{
				"id":          "ding",
				"name":        "DING消息",
				"description": "DING 消息 / 发送 / 撤回",
				"tools": []any{
					map[string]any{"name": "send_ding_message", "cli_name": "send"},
					map[string]any{"name": "recall_ding_message", "cli_name": "recall"},
				},
			},
			map[string]any{
				"id":    "doc",
				"name":  "钉钉文档",
				"tools": []any{map[string]any{"name": "create_document", "cli_name": "create"}},
			},
		},
	}

	var buf bytes.Buffer
	if err := writePretty(&buf, payload); err != nil {
		t.Fatalf("writePretty() error = %v", err)
	}
	out := buf.String()

	wants := []string{
		"Catalog",
		"2 products discovered",
		"ding",
		"send_ding_message",
		"→ send",
		"recall_ding_message",
		"doc",
		"create_document",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("pretty list missing %q; got:\n%s", want, out)
		}
	}
}

func TestPretty_SchemaToolRendersAllSections(t *testing.T) {
	forceNoColor(t)

	payload := map[string]any{
		"kind": "schema",
		"path": "ding.send_ding_message",
		"product": map[string]any{
			"id":   "ding",
			"name": "DING消息",
		},
		"tool": map[string]any{
			"name":           "send_ding_message",
			"cli_name":       "send",
			"canonical_path": "ding.send_ding_message",
			"group":          "message",
			"title":          "发送DING消息",
			"description":    "使用企业内机器人发送DING消息",
			"sensitive":      true,
			"annotations": map[string]any{
				"destructive_hint": true,
			},
			"parameters": map[string]any{
				"robotCode": map[string]any{
					"type":        "string",
					"description": "机器人Code",
				},
				"receiverUserIdList": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "接收者用户ID列表",
				},
				"remindType": map[string]any{
					"type":        "number",
					"description": "提醒类型",
				},
			},
			"required": []any{"robotCode", "receiverUserIdList", "remindType"},
			"flag_overlay": map[string]any{
				"receiverUserIdList": map[string]any{
					"alias":     "users",
					"transform": "csv_to_array",
				},
				"robotCode": map[string]any{
					"alias":       "robot-code",
					"env_default": "DINGTALK_DING_ROBOT_CODE",
				},
				"remindType": map[string]any{
					"alias":     "type",
					"transform": "enum_map",
					"transform_args": map[string]any{
						"app":      1,
						"sms":      2,
						"call":     3,
						"_default": 1,
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := writePretty(&buf, payload); err != nil {
		t.Fatalf("writePretty() error = %v", err)
	}
	out := buf.String()

	wants := []string{
		"Tool send_ding_message", // header
		"canonical:",             // meta section
		"ding.send_ding_message",
		"cli path:",
		"ding message send",
		"sensitive:",
		"yes (needs --yes)",
		"annotations:",
		"destructive_hint=true",
		"Parameters",
		"robotCode",
		"--robot-code", // overlay alias rendered
		"env default:",
		"$DINGTALK_DING_ROBOT_CODE",
		"receiverUserIdList",
		"--users",
		"transform:",
		"csv_to_array",
		"string[]", // array<string> shorthand
		"remindType",
		"--type",
		"enum_map(", // with args inlined
		"app=1",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("pretty tool missing %q; full output:\n%s", want, out)
		}
	}

	// required params must show the red asterisk marker (even with color disabled
	// the literal '*' still appears).
	for _, req := range []string{"robotCode", "receiverUserIdList", "remindType"} {
		// format: " * <name>"
		if !strings.Contains(out, "* "+req) {
			t.Errorf("required marker missing for %q", req)
		}
	}
}

func TestPretty_NonSchemaFallsBackToTableish(t *testing.T) {
	forceNoColor(t)

	// Payload that doesn't have kind="schema" should fall through to
	// the tableish renderer, not error or render a schema header.
	payload := map[string]any{"items": []any{
		map[string]any{"id": "a", "name": "Alice"},
		map[string]any{"id": "b", "name": "Bob"},
	}}

	var buf bytes.Buffer
	if err := writePretty(&buf, payload); err != nil {
		t.Fatalf("writePretty() error = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Catalog") || strings.Contains(out, "Tool ") {
		t.Errorf("pretty wrongly ran schema renderer on non-schema payload:\n%s", out)
	}
	// tableish should produce header + rows
	for _, want := range []string{"id", "name", "Alice", "Bob"} {
		if !strings.Contains(out, want) {
			t.Errorf("tableish fallback missing %q:\n%s", want, out)
		}
	}
}

func TestNormalizeFormat_RecognisesPretty(t *testing.T) {
	if got := normalizeFormat("pretty", FormatJSON); got != FormatPretty {
		t.Errorf("normalizeFormat(pretty) = %q, want %q", got, FormatPretty)
	}
	if got := normalizeFormat("PRETTY", FormatJSON); got != FormatPretty {
		t.Errorf("normalizeFormat(PRETTY) = %q, want %q", got, FormatPretty)
	}
}
