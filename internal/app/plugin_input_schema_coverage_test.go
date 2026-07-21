// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestPluginToolInputSchemaMatchesTrimmedName(t *testing.T) {
	want := map[string]any{"type": "object"}
	tools := transport.ToolsListResult{Tools: []transport.ToolDescriptor{
		{Name: "other", InputSchema: map[string]any{"type": "string"}},
		{Name: "  create_conference  ", InputSchema: want},
	}}

	got, ok := pluginToolInputSchema(tools, " create_conference ")
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("pluginToolInputSchema() = (%#v, %v), want (%#v, true)", got, ok, want)
	}
	if got, ok := pluginToolInputSchema(tools, "missing"); ok || got != nil {
		t.Fatalf("missing pluginToolInputSchema() = (%#v, %v), want (nil, false)", got, ok)
	}
}

func TestNormalizePluginInputParamsCoercesNestedValues(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []string{"enabled"},
		"properties": map[string]any{
			"enabled": map[string]any{"type": []any{"null", "bool"}},
			"count":   map[string]any{"type": "int"},
			"ratio":   map[string]any{"type": "float"},
			"settings": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"active": map[string]any{"type": "bool"},
				},
			},
			"ids": map[string]any{
				"type":  []string{"array", "null"},
				"items": map[string]any{"type": "int"},
			},
			"labels": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"booleans": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "bool"},
			},
			"ambiguous": map[string]any{"type": []string{"string", "int"}},
		},
	}
	params := map[string]any{
		"enabled":   " true ",
		"count":     " 7 ",
		"ratio":     " 2.5 ",
		"settings":  `{"active":"false"}`,
		"ids":       `["1", "2"]`,
		"labels":    "alpha, , beta",
		"booleans":  []string{"true", "false"},
		"ambiguous": "9",
	}

	got, err := normalizePluginInputParams(params, schema)
	if err != nil {
		t.Fatalf("normalizePluginInputParams() error = %v", err)
	}
	want := map[string]any{
		"enabled":   true,
		"count":     7,
		"ratio":     2.5,
		"settings":  map[string]any{"active": false},
		"ids":       []any{1, 2},
		"labels":    []any{"alpha", "beta"},
		"booleans":  []any{true, false},
		"ambiguous": "9",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizePluginInputParams() = %#v, want %#v", got, want)
	}

	properties := schema["properties"].(map[string]any)
	if gotType := properties["enabled"].(map[string]any)["type"].([]any)[1]; gotType != "bool" {
		t.Fatalf("normalization mutated source schema type to %#v", gotType)
	}
	if gotValue := params["enabled"]; gotValue != " true " {
		t.Fatalf("normalization mutated source params to %#v", gotValue)
	}
}

func TestNormalizePluginInputParamsReportsConversionPath(t *testing.T) {
	tests := []struct {
		name        string
		value       any
		fieldSchema map[string]any
		wantText    string
	}{
		{name: "boolean", value: "sometimes", fieldSchema: map[string]any{"type": "bool"}, wantText: "cannot convert"},
		{name: "integer", value: "1.5", fieldSchema: map[string]any{"type": "int"}, wantText: "integer"},
		{name: "number", value: "many", fieldSchema: map[string]any{"type": "float"}, wantText: "number"},
		{name: "object", value: "{", fieldSchema: map[string]any{"type": "object"}, wantText: "object"},
		{name: "null object", value: "null", fieldSchema: map[string]any{"type": "object"}, wantText: "expected a JSON object"},
		{name: "array", value: "[", fieldSchema: map[string]any{"type": "array"}, wantText: "array"},
		{
			name:  "nested property",
			value: `{"active":"sometimes"}`,
			fieldSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"active": map[string]any{"type": "bool"},
				},
			},
			wantText: "field: active:",
		},
		{
			name:  "array item",
			value: "1,not-an-int",
			fieldSchema: map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "int"},
			},
			wantText: "item 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := map[string]any{
				"type":       "object",
				"properties": map[string]any{"field": tt.fieldSchema},
			}
			_, err := normalizePluginInputParams(map[string]any{"field": tt.value}, schema)
			if err == nil {
				t.Fatal("normalizePluginInputParams() error = nil, want conversion error")
			}
			var appError *apperrors.Error
			if !errors.As(err, &appError) ||
				appError.Category != apperrors.CategoryValidation ||
				appError.Reason != "plugin_input_schema_invalid" {
				t.Fatalf("conversion error = %#v, want categorized plugin schema validation error", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("conversion error = %q, want text %q", err, tt.wantText)
			}
		})
	}
}

func TestNormalizePluginInputParamsRunsSchemaValidation(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	if _, err := normalizePluginInputParams(map[string]any{}, schema); err == nil ||
		!strings.Contains(err.Error(), "$.name is required") {
		t.Fatalf("required-field validation error = %v", err)
	}
}

func TestPluginInputSchemaHelperEdges(t *testing.T) {
	if got := canonicalPluginInputSchema(nil); got != nil {
		t.Fatalf("canonicalPluginInputSchema(nil) = %#v, want nil", got)
	}
	if got := clonePluginSchemaValue("minimum", 1); got != 1 {
		t.Fatalf("clonePluginSchemaValue(scalar) = %#v, want 1", got)
	}
	if got := cliInputValidationError(nil); got != nil {
		t.Fatalf("cliInputValidationError(nil) = %v, want nil", got)
	}

	if got, err := coercePluginSchemaValue("", map[string]any{"type": "array"}); err != nil || !reflect.DeepEqual(got, []any(nil)) {
		t.Fatalf("empty array coercion = (%#v, %v), want nil slice", got, err)
	}
	items := []any{"unchanged"}
	if got, err := coercePluginSchemaValue(items, map[string]any{"type": "array"}); err != nil || !reflect.DeepEqual(got, items) {
		t.Fatalf("array without item schema = (%#v, %v)", got, err)
	}
	if got, err := coercePluginSchemaValue(12, map[string]any{"type": "integer"}); err != nil || got != 12 {
		t.Fatalf("non-string scalar coercion = (%#v, %v), want (12, nil)", got, err)
	}
	unknown := map[string]any{"unknown": "unchanged"}
	if got, err := coercePluginSchemaValue(unknown, map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}); err != nil || !reflect.DeepEqual(got, unknown) {
		t.Fatalf("unknown property coercion = (%#v, %v), want unchanged map", got, err)
	}

	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "missing", schema: map[string]any{}, want: ""},
		{name: "single string", schema: map[string]any{"type": "integer"}, want: "integer"},
		{name: "single string slice", schema: map[string]any{"type": []string{"null", "number"}}, want: "number"},
		{name: "any slice", schema: map[string]any{"type": []any{nil, 3, "", "null", "boolean"}}, want: "boolean"},
		{name: "ambiguous", schema: map[string]any{"type": []any{"string", "integer"}}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := singlePluginSchemaType(tt.schema); got != tt.want {
				t.Fatalf("singlePluginSchemaType(%#v) = %q, want %q", tt.schema, got, tt.want)
			}
		})
	}

	for raw, want := range map[string]string{
		" BOOL ": "boolean",
		"Int":    "integer",
		"FLOAT":  "number",
		"custom": "custom",
	} {
		if got := canonicalPluginSchemaType(raw); got != want {
			t.Errorf("canonicalPluginSchemaType(%q) = %q, want %q", raw, got, want)
		}
	}
}
