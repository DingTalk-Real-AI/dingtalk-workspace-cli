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

package mcpschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageValidateInputSchemaCoreSubset(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
		"required": []any{
			"query",
		},
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "enum": []any{"one", "two"}},
			"items": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		},
		"additionalProperties": false,
	}
	if err := ValidateInputSchema(map[string]any{"query": "one", "items": []any{json.Number("2")}}, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v, want nil", err)
	}
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "required", params: map[string]any{}, want: "$.query is required"},
		{name: "enum", params: map[string]any{"query": "three"}, want: "$.query must be one of"},
		{name: "item type", params: map[string]any{"query": "one", "items": []any{"bad"}}, want: "$.items[0] must be integer"},
		{name: "additional", params: map[string]any{"query": "one", "extra": true}, want: "$.extra is not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputSchema(tt.params, schema)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInputSchema() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageValidateInputSchemaUsesAdditionalPropertiesDefault(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"title": map[string]any{"type": "string"}},
	}
	if err := ValidateInputSchema(map[string]any{"title": "Report", "extension": true}, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v, want default additionalProperties=true", err)
	}
	schema["additionalProperties"] = map[string]any{"type": "integer"}
	if err := ValidateInputSchema(map[string]any{"extension": "bad"}, schema); err == nil || !strings.Contains(err.Error(), "$.extension must be integer") {
		t.Fatalf("additionalProperties schema error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateInputSchemaFailsClosedOnUnsupportedOrMalformedKeywords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "unsupported assertion", schema: map[string]any{"type": "object", "oneOf": []any{}}, want: `keyword "oneOf"`},
		{name: "unsupported annotation", schema: map[string]any{"type": "object", "$vocabulary": map[string]any{}}, want: `keyword "$vocabulary"`},
		{name: "unsupported dialect keyword", schema: map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "patternProperties": map[string]any{}}, want: `keyword "patternProperties"`},
		{name: "unsupported dialect", schema: map[string]any{"$schema": "https://example.test/schema", "type": "object"}, want: "unsupported dialect"},
		{name: "malformed description", schema: map[string]any{"type": "object", "description": true}, want: "description must be a string"},
		{name: "boolean child", schema: map[string]any{"type": "object", "properties": map[string]any{"id": true}}, want: "unsupported boolean"},
		{name: "tuple items", schema: map[string]any{"type": "object", "properties": map[string]any{"ids": map[string]any{"type": "array", "items": []any{}}}}, want: "items uses"},
		{name: "non-object root", schema: map[string]any{"type": "array"}, want: `$.type must be exactly "object"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputSchema(map[string]any{}, tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInputSchema() error = %v, want %q", err, tt.want)
			}
		})
	}
	err := ValidateInputSchema(map[string]any{}, map[string]any{"type": "object", "minimum": 1})
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.Reason != "mcp_input_schema_validation_failed" {
		t.Fatalf("typed validation error = %#v", err)
	}
}

func TestCrossPlatformCoverageValidateInputSchemaExactNumbersAndEnumUniqueness(t *testing.T) {
	t.Parallel()
	duplicate := map[string]any{
		"type": "object",
		"properties": map[string]any{"value": map[string]any{"enum": []any{
			map[string]any{"nested": []any{json.Number("9007199254740993")}},
			map[string]any{"nested": []any{json.Number("90071992547409930e-1")}},
		}}},
	}
	if err := ValidateInputSchema(map[string]any{}, duplicate); err == nil || !strings.Contains(err.Error(), "duplicate at index 1") {
		t.Fatalf("duplicate enum error = %v", err)
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{"value": map[string]any{
			"type": "integer", "enum": []any{json.Number("9007199254740993")},
		}},
	}
	if err := ValidateInputSchema(map[string]any{"value": json.Number("90071992547409930e-1")}, schema); err != nil {
		t.Fatalf("mathematically equal exact number rejected: %v", err)
	}
}

func TestCrossPlatformCoverageValidateInputSchemaComplexityBudgets(t *testing.T) {
	t.Parallel()
	depthSchema := map[string]any{"type": "object"}
	current := depthSchema
	for range maxSchemaDepth {
		child := map[string]any{}
		current["properties"] = map[string]any{"next": child}
		current = child
	}
	properties := make(map[string]any, maxSchemaProperties+1)
	for index := 0; index <= maxSchemaProperties; index++ {
		properties[strconv.Itoa(index)] = map[string]any{}
	}
	enumMembers := make([]any, maxEnumMembers+1)
	for index := range enumMembers {
		enumMembers[index] = index
	}
	deepEnum := any(nil)
	for range maxEnumValueDepth {
		deepEnum = []any{deepEnum}
	}
	var schemaTree func(int) map[string]any
	schemaTree = func(depth int) map[string]any {
		if depth == 0 {
			return map[string]any{}
		}
		return map[string]any{
			"items":                schemaTree(depth - 1),
			"additionalProperties": schemaTree(depth - 1),
		}
	}
	nodeSchema := schemaTree(10)
	nodeSchema["type"] = "object"
	required := make([]any, maxRequiredProperties+1)
	for index := range required {
		required[index] = strconv.Itoa(index)
	}
	deepAnnotation := any(nil)
	for range maxAnnotationDepth {
		deepAnnotation = []any{deepAnnotation}
	}
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "depth", schema: depthSchema, want: "schema recursion depth limit"},
		{name: "nodes", schema: nodeSchema, want: "total schema node limit"},
		{name: "properties", schema: map[string]any{"type": "object", "properties": properties}, want: "object property limit"},
		{name: "required", schema: map[string]any{"type": "object", "required": required}, want: "required exceeds total member limit"},
		{name: "schema strings", schema: map[string]any{"type": "object", "description": strings.Repeat("x", maxSchemaStringBytes+1)}, want: "schema string limit"},
		{name: "annotation depth", schema: map[string]any{"type": "object", "default": deepAnnotation}, want: "annotation nesting limit"},
		{name: "annotation nodes", schema: map[string]any{"type": "object", "examples": []any{make([]any, maxAnnotationNodes)}}, want: "annotation node limit"},
		{name: "annotation bytes", schema: map[string]any{"type": "object", "default": strings.Repeat("x", maxAnnotationBytes+1)}, want: "annotation size limit"},
		{name: "enum members", schema: map[string]any{"type": "object", "enum": enumMembers}, want: "enum member limit"},
		{name: "enum depth", schema: map[string]any{"type": "object", "enum": []any{deepEnum}}, want: "enum value nesting limit"},
		{name: "enum nodes", schema: map[string]any{"type": "object", "enum": []any{make([]any, maxEnumValueNodes)}}, want: "enum value node limit"},
		{name: "enum bytes", schema: map[string]any{"type": "object", "enum": []any{strings.Repeat("x", maxEnumValueBytes+1)}}, want: "enum value size limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateInputSchema(map[string]any{}, tt.schema); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInputSchema() error = %v, want %q", err, tt.want)
			}
		})
	}
	arraySchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"values": map[string]any{"type": "array", "items": map[string]any{"type": "null"}}},
	}
	if err := ValidateInputSchema(map[string]any{"values": make([]any, maxArrayValidationItems+1)}, arraySchema); err == nil || !strings.Contains(err.Error(), "array validation item limit") {
		t.Fatalf("array budget error = %v", err)
	}
	deepValue := any("leaf")
	for range 20 {
		deepValue = []any{deepValue}
	}
	workSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{"values": map[string]any{
			"type": "array", "items": map[string]any{"enum": []any{deepValue}},
		}},
	}
	values := make([]any, maxArrayValidationItems)
	for index := range values {
		values[index] = deepValue
	}
	if err := ValidateInputSchema(map[string]any{"values": values}, workSchema); err == nil || !strings.Contains(err.Error(), "enum validation exceeds work limit") {
		t.Fatalf("enum work budget error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateInputSchemaBoundsExactNumbers(t *testing.T) {
	t.Parallel()
	ordinary := json.Number("1234567890123456789012345678901234567890")
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "integer", "enum": []any{ordinary}}},
	}
	if err := ValidateInputSchema(map[string]any{"value": ordinary}, schema); err != nil {
		t.Fatalf("ordinary exact integer rejected: %v", err)
	}
	tests := []struct {
		name   string
		value  json.Number
		params bool
		want   string
	}{
		{name: "literal bytes", value: json.Number(strings.Repeat("9", maxNumberLiteralBytes+1)), want: "numeric literal exceeds byte limit"},
		{name: "digits", value: json.Number(strings.Repeat("9", maxNumberDigits+1)), want: "numeric literal exceeds digit limit"},
		{name: "exponent", value: json.Number("1e" + strings.Repeat("9", 20)), params: true, want: "exponent exceeds magnitude limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateSchema := map[string]any{"type": "object", "enum": []any{tt.value}}
			params := map[string]any{}
			if tt.params {
				candidateSchema = map[string]any{
					"type":       "object",
					"properties": map[string]any{"value": map[string]any{"type": "number"}},
				}
				params["value"] = tt.value
			}
			if err := ValidateInputSchema(params, candidateSchema); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInputSchema() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageDigestIsDeterministicAndSnapshotSensitive(t *testing.T) {
	t.Parallel()
	first, err := Digest(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
			"name":  map[string]any{"type": "string"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Digest(map[string]any{
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"count": map[string]any{"type": "integer"},
		},
		"type": "object",
	})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := Digest(map[string]any{"type": "object"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || first != second || first == changed {
		t.Fatalf("digests first=%q second=%q changed=%q", first, second, changed)
	}
	lexicalOne, err := Digest(map[string]any{"type": "object", "default": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	lexicalOnePointZero, err := Digest(map[string]any{"type": "object", "default": json.Number("1.0")})
	if err != nil {
		t.Fatal(err)
	}
	if lexicalOne == lexicalOnePointZero {
		t.Fatal("digest did not bind numeric lexical representation")
	}
}

func TestCrossPlatformCoverageValidateInputSchemaAcceptedKeywordForms(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"$id":         "schema-id",
		"$anchor":     "schema-anchor",
		"$comment":    "schema-comment",
		"title":       "Schema title",
		"description": "Schema description",
		"deprecated":  true,
		"readOnly":    false,
		"writeOnly":   true,
		"examples":    []string{"first", "second"},
		"default": map[string]any{
			"enabled": true,
			"missing": nil,
			"nested":  []string{"value"},
			"number":  json.Number("1.25e+2"),
		},
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": []any{"string", "null"}},
		},
		"additionalProperties": true,
	}
	if err := ValidateInputSchema(nil, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v, want nil", err)
	}
}

func TestCrossPlatformCoverageValidateInputSchemaRejectsMalformedSupportedKeywords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "schema URI type", schema: map[string]any{"$schema": true, "type": "object"}, want: "$schema must be a supported dialect URI"},
		{name: "boolean annotation", schema: map[string]any{"deprecated": "true", "type": "object"}, want: "deprecated must be a boolean"},
		{name: "examples type", schema: map[string]any{"examples": "example", "type": "object"}, want: "examples must be an array"},
		{name: "examples invalid value", schema: map[string]any{"examples": []any{make(chan int)}, "type": "object"}, want: "is not a JSON value"},
		{name: "default invalid number", schema: map[string]any{"default": json.Number("01"), "type": "object"}, want: "invalid numeric literal"},
		{name: "default invalid value", schema: map[string]any{"default": make(chan int), "type": "object"}, want: "is not a JSON value"},
		{name: "empty type", schema: map[string]any{"type": ""}, want: "type is not a supported"},
		{name: "empty type array", schema: map[string]any{"type": []any{}}, want: "type is not a supported"},
		{name: "unsupported type", schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "date"}}}, want: `unsupported type "date"`},
		{name: "required type", schema: map[string]any{"required": "value", "type": "object"}, want: "required must be an array"},
		{name: "required non-string", schema: map[string]any{"required": []any{1}, "type": "object"}, want: "required must be an array"},
		{name: "required duplicate", schema: map[string]any{"required": []string{"value", "value"}, "type": "object"}, want: "required must be an array"},
		{name: "properties type", schema: map[string]any{"properties": []any{}, "type": "object"}, want: "properties must be an object"},
		{name: "additional properties type", schema: map[string]any{"additionalProperties": "false", "type": "object"}, want: "additionalProperties must be a boolean or object schema"},
		{name: "items child", schema: map[string]any{"items": map[string]any{"minimum": 1}, "type": "object"}, want: `keyword "minimum"`},
		{name: "additional properties child", schema: map[string]any{"additionalProperties": map[string]any{"minimum": 1}, "type": "object"}, want: `keyword "minimum"`},
		{name: "empty enum", schema: map[string]any{"enum": []any{}, "type": "object"}, want: "non-empty array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputSchema(map[string]any{}, tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInputSchema() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageValidateInputSchemaStringAccountingEdges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{
			name:   "keyword",
			schema: map[string]any{strings.Repeat("k", maxSchemaStringBytes+1): true, "type": "object"},
			want:   "schema string limit",
		},
		{
			name: "dialect",
			schema: map[string]any{
				"$anchor": strings.Repeat("a", maxSchemaStringBytes-len("$anchor")-len("$schema")),
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type":    "object",
			},
			want: "schema string limit",
		},
		{
			name: "type value",
			schema: map[string]any{
				"description": strings.Repeat("d", maxSchemaStringBytes-len("description")-len("type")),
				"type":        "object",
			},
			want: "schema string limit",
		},
		{
			name: "required member",
			schema: map[string]any{
				"description": strings.Repeat("d", maxSchemaStringBytes-len("description")-len("required")),
				"required":    []string{"value"},
				"type":        "object",
			},
			want: "schema string limit",
		},
		{
			name: "property name",
			schema: map[string]any{
				"properties": map[string]any{strings.Repeat("p", maxSchemaStringBytes): map[string]any{}},
				"type":       "object",
			},
			want: "schema string limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputSchema(map[string]any{}, tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInputSchema() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageAnnotationValueVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "boolean", value: true},
		{name: "string", value: "value"},
		{name: "number", value: json.Number("-1.5E-2")},
		{name: "any array", value: []any{"value", false}},
		{name: "string array", value: []string{"one", "two"}},
		{name: "object", value: map[string]any{"key": []any{nil}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := accountAnnotationValue("$", tt.value, 1, &schemaValidationState{}); err != nil {
				t.Fatalf("accountAnnotationValue() error = %v, want nil", err)
			}
		})
	}

	errorTests := []struct {
		name  string
		value any
		state schemaValidationState
		want  string
	}{
		{name: "invalid number", value: json.Number("1."), want: "invalid numeric literal"},
		{name: "numeric bytes", value: json.Number("1"), state: schemaValidationState{annotationBytes: maxAnnotationBytes}, want: "annotation size limit"},
		{name: "string array child bytes", value: []string{"x"}, state: schemaValidationState{annotationBytes: maxAnnotationBytes}, want: "annotation size limit"},
		{name: "map key bytes", value: map[string]any{"k": nil}, state: schemaValidationState{annotationBytes: maxAnnotationBytes}, want: "annotation size limit"},
		{name: "map child", value: map[string]any{"k": make(chan int)}, want: "not a JSON value"},
		{name: "unsupported", value: make(chan int), want: "not a JSON value"},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			err := accountAnnotationValue("$", tt.value, 1, &tt.state)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("accountAnnotationValue() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSchemaListAndArrayHelpers(t *testing.T) {
	t.Parallel()
	if values, valid := schemaStringArray([]string{"one", "two"}, false); !valid || len(values) != 2 {
		t.Fatalf("schemaStringArray() = %v, %v", values, valid)
	}
	for _, raw := range []any{[]any{""}, []any{"one", "one"}, []any{"one", 2}, true} {
		if _, valid := schemaStringArray(raw, false); valid {
			t.Fatalf("schemaStringArray(%#v) unexpectedly valid", raw)
		}
	}
	if values, valid := jsonArray([]string{"one", "two"}); !valid || len(values) != 2 || values[1] != "two" {
		t.Fatalf("jsonArray() = %v, %v", values, valid)
	}
	if _, valid := jsonArray("not an array"); valid {
		t.Fatal("jsonArray(string) unexpectedly valid")
	}
}

func TestCrossPlatformCoverageCanonicalEnumValueVariants(t *testing.T) {
	t.Parallel()
	values := []any{
		nil,
		true,
		false,
		"value",
		[]any{nil, true, "value"},
		[]string{"one", "two"},
		map[string]any{"b": false, "a": json.Number("1.0")},
	}
	for _, value := range values {
		if _, err := canonicalEnumValue("$", value, &schemaValidationState{}); err != nil {
			t.Fatalf("canonicalEnumValue(%#v) error = %v", value, err)
		}
	}

	if err := validateEnum("$.enum", []string{"one", "two"}, &schemaValidationState{enumIndexes: map[string]map[string]struct{}{}}); err != nil {
		t.Fatalf("validateEnum([]string) error = %v", err)
	}
	if err := validateEnum("$.enum", []any{make(chan int)}, &schemaValidationState{enumIndexes: map[string]map[string]struct{}{}}); err == nil || !strings.Contains(err.Error(), "not a JSON value") {
		t.Fatalf("validateEnum() error = %v, want invalid JSON value", err)
	}
	if _, err := canonicalEnumValue("$", make(chan int), &schemaValidationState{}); err == nil || !strings.Contains(err.Error(), "not a JSON value") {
		t.Fatalf("canonicalEnumValue() error = %v, want invalid JSON value", err)
	}

	errorTests := []struct {
		name  string
		value any
		state schemaValidationState
		want  string
	}{
		{name: "numeric bytes", value: json.Number("1"), state: schemaValidationState{enumValueBytes: maxEnumValueBytes}, want: "enum value size limit"},
		{name: "string bytes", value: "x", state: schemaValidationState{enumValueBytes: maxEnumValueBytes}, want: "enum value size limit"},
		{name: "any array child", value: []any{make(chan int)}, want: "not a JSON value"},
		{name: "string array child bytes", value: []string{"x"}, state: schemaValidationState{enumValueBytes: maxEnumValueBytes}, want: "enum value size limit"},
		{name: "map key bytes", value: map[string]any{"k": nil}, state: schemaValidationState{enumValueBytes: maxEnumValueBytes}, want: "enum value size limit"},
		{name: "map child", value: map[string]any{"k": make(chan int)}, want: "not a JSON value"},
		{name: "unsupported", value: make(chan int), want: "not a JSON value"},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			var buffer bytes.Buffer
			err := writeCanonicalEnumValue(&buffer, "$", tt.value, 1, &tt.state)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("writeCanonicalEnumValue() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageTypeAndNumberVariants(t *testing.T) {
	t.Parallel()
	typeTests := []struct {
		value    any
		expected string
		want     bool
	}{
		{value: map[string]any{}, expected: "object", want: true},
		{value: []any{}, expected: "array", want: true},
		{value: "value", expected: "string", want: true},
		{value: true, expected: "boolean", want: true},
		{value: 1.5, expected: "number", want: true},
		{value: 1.5, expected: "integer", want: false},
		{value: "1", expected: "integer", want: false},
		{value: nil, expected: "null", want: true},
		{value: nil, expected: "unknown", want: false},
	}
	for _, tt := range typeTests {
		got, err := matchesType(tt.value, tt.expected)
		if err != nil || got != tt.want {
			t.Fatalf("matchesType(%#v, %q) = %v, %v, want %v, nil", tt.value, tt.expected, got, err, tt.want)
		}
	}
	if got, err := matchesType(math.Inf(1), "number"); err == nil || got {
		t.Fatalf("matchesType(+Inf, number) = %v, %v, want false, error", got, err)
	}
	if got, err := matchesType(json.Number("01"), "integer"); err == nil || got {
		t.Fatalf("matchesType(01, integer) = %v, %v, want false, error", got, err)
	}
	if got, err := matchesEnum(make(chan int), nil, &schemaValidationState{}); err == nil || got {
		t.Fatalf("matchesEnum(invalid) = %v, %v, want false, error", got, err)
	}

	numbers := []any{
		float64(1),
		float32(1),
		int(1),
		int8(1),
		int16(1),
		int32(1),
		int64(1),
		uint(1),
		uint8(1),
		uint16(1),
		uint32(1),
		uint64(1),
		json.Number("1"),
	}
	for _, value := range numbers {
		text, numeric := numericLiteralText(value)
		if !numeric || text == "" {
			t.Fatalf("numericLiteralText(%T) = %q, %v", value, text, numeric)
		}
		if _, numeric, err := parseNumber(value); !numeric || err != nil {
			t.Fatalf("parseNumber(%T) numeric=%v error=%v", value, numeric, err)
		}
	}
	if parsed, numeric, err := parseNumber("1"); parsed != nil || numeric || err != nil {
		t.Fatalf("parseNumber(non-number) = %v, %v, %v", parsed, numeric, err)
	}
}

func TestCrossPlatformCoverageNumberLiteralGrammar(t *testing.T) {
	t.Parallel()
	valid := []string{"0", "-0", "10", "0.5", "-10.25", "1e2", "1E+2", "1e-2", "1e4096"}
	for _, text := range valid {
		if err := validateNumberLiteral(text); err != nil {
			t.Errorf("validateNumberLiteral(%q) error = %v, want nil", text, err)
		}
	}
	invalid := []string{"", "-", "01", "+1", ".1", "1.", "1e", "1e+", "1x"}
	for _, text := range invalid {
		if err := validateNumberLiteral(text); err == nil {
			t.Errorf("validateNumberLiteral(%q) error = nil, want error", text)
		}
	}
}

func TestCrossPlatformCoverageDigestRejectsUnencodableSnapshot(t *testing.T) {
	t.Parallel()
	if _, err := Digest(map[string]any{"invalid": make(chan int)}); err == nil || !strings.Contains(err.Error(), "cannot be encoded") {
		t.Fatalf("Digest() error = %v, want encoding error", err)
	}
}
