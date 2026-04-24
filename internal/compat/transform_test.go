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
	"reflect"
	"testing"
)

// TestJSONParse_StrictJSON covers the primary path: callers passing
// canonical JSON (as generated programmatically or by agents).
func TestJSONParse_StrictJSON(t *testing.T) {
	t.Parallel()

	input := `[{"fieldName":"title","type":"text"},{"fieldName":"count","type":"number"}]`
	got, err := ApplyTransform(input, "json_parse", nil)
	if err != nil {
		t.Fatalf("strict JSON should parse, got err: %v", err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected []any of length 2, got %T %v", got, got)
	}
}

// TestJSONParse_YAMLFlowFallback is the motivating case: a user types an
// ad-hoc JSON-shaped array without quoting every key and value. YAML flow
// syntax accepts it and the parsed output is indistinguishable from the
// strict-JSON equivalent.
func TestJSONParse_YAMLFlowFallback(t *testing.T) {
	t.Parallel()

	// Intentionally unquoted keys, unquoted string values, and Chinese
	// identifiers — typical of what humans type at a shell.
	input := `[{fieldName: 标题, type: text}, {fieldName: 数量, type: number, config: {formatter: INT}}, {fieldName: 状态, type: singleSelect, config: {options: [{name: 待办}, {name: 进行中}, {name: 已完成}]}}, {fieldName: 已确认, type: checkbox}]`

	got, err := ApplyTransform(input, "json_parse", nil)
	if err != nil {
		t.Fatalf("YAML-flow input should parse, got err: %v", err)
	}
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(arr) != 4 {
		t.Fatalf("expected 4 field definitions, got %d", len(arr))
	}

	// Spot-check the third entry, which is the most deeply nested.
	third, ok := arr[2].(map[string]any)
	if !ok {
		t.Fatalf("arr[2] expected map[string]any, got %T", arr[2])
	}
	if third["fieldName"] != "状态" {
		t.Errorf("arr[2].fieldName: want 状态, got %v", third["fieldName"])
	}
	config, ok := third["config"].(map[string]any)
	if !ok {
		t.Fatalf("arr[2].config expected map, got %T", third["config"])
	}
	options, ok := config["options"].([]any)
	if !ok || len(options) != 3 {
		t.Fatalf("arr[2].config.options: want 3 items, got %v", config["options"])
	}
}

// TestJSONParse_EmptyString preserves the legacy behaviour of returning the
// original value untouched when the caller passes an empty / whitespace-only
// string, matching how other transforms treat empty input.
func TestJSONParse_EmptyString(t *testing.T) {
	t.Parallel()

	cases := []string{"", "   ", "\n\t"}
	for _, in := range cases {
		got, err := ApplyTransform(in, "json_parse", nil)
		if err != nil {
			t.Errorf("empty input %q should not error: %v", in, err)
			continue
		}
		if !reflect.DeepEqual(got, in) {
			t.Errorf("empty input %q should pass through, got %v", in, got)
		}
	}
}

// TestJSONParse_NonString passes through non-string inputs (already-parsed
// values flowing through the pipeline).
func TestJSONParse_NonString(t *testing.T) {
	t.Parallel()

	preParsed := []any{map[string]any{"k": "v"}}
	got, err := ApplyTransform(preParsed, "json_parse", nil)
	if err != nil {
		t.Fatalf("non-string should pass through: %v", err)
	}
	if !reflect.DeepEqual(got, preParsed) {
		t.Errorf("non-string should pass through unchanged, got %v", got)
	}
}

// TestJSONParse_InvalidInput verifies that genuine garbage is still rejected
// with a user-facing validation error that nudges towards `@file` syntax.
func TestJSONParse_InvalidInput(t *testing.T) {
	t.Parallel()

	// Unterminated bracket — neither valid JSON nor valid YAML flow.
	_, err := ApplyTransform("[{fieldName:", "json_parse", nil)
	if err == nil {
		t.Fatal("expected error for malformed input")
	}
	if msg := err.Error(); msg == "" {
		t.Fatal("error message should be non-empty")
	}
}
