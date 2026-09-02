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
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateInputSchemaAcceptsValidPayload(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"required": []any{
			"title",
		},
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
			"count": map[string]any{"type": "integer"},
			"mode":  map[string]any{"type": "string", "enum": []any{"auto", "manual"}},
		},
	}

	err := ValidateInputSchema(map[string]any{
		"title": "Quarterly Report",
		"count": float64(3),
		"mode":  "auto",
	}, schema)
	if err != nil {
		t.Fatalf("ValidateInputSchema() error = %v, want nil", err)
	}
}

func TestValidateInputSchemaRejectsUnknownProperty(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}

	err := ValidateInputSchema(map[string]any{
		"title":   "Quarterly Report",
		"unknown": "x",
	}, schema)
	if err == nil {
		t.Fatal("ValidateInputSchema() error = nil, want unknown-property validation error")
	}
	if !strings.Contains(err.Error(), "$.unknown is not allowed") {
		t.Fatalf("ValidateInputSchema() error = %v, want unknown-property message", err)
	}
}

func TestValidateMCPInputSchemaUsesJSONSchemaAdditionalPropertiesDefault(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}
	params := map[string]any{"title": "Quarterly Report", "extension": true}

	if err := ValidateMCPInputSchema(params, schema); err != nil {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want nil", err)
	}

	schema["additionalProperties"] = false
	err := ValidateMCPInputSchema(params, schema)
	if err == nil || !strings.Contains(err.Error(), "$.extension is not allowed") {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want explicit additionalProperties rejection", err)
	}
}

func TestValidateMCPInputSchemaRejectsUnsupportedConstraint(t *testing.T) {
	t.Parallel()

	err := ValidateMCPInputSchema(map[string]any{}, map[string]any{
		"oneOf": []any{
			map[string]any{"required": []any{"id"}},
			map[string]any{"required": []any{"name"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported JSON Schema keyword "oneOf"`) {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want unsupported oneOf error", err)
	}
}

func TestValidateMCPInputSchemaSupportShapes(t *testing.T) {
	t.Parallel()

	valid := []map[string]any{
		{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "id", "$anchor": "anchor", "$comment": "comment", "title": "title", "description": "description", "default": "x", "examples": []any{"x"}, "deprecated": true, "readOnly": true, "writeOnly": true},
		{"type": "object"},
		{"type": []string{"string", "null"}},
		{"type": []any{"number", "integer"}},
		{"enum": []string{"a"}},
		{"enum": []any{nil}},
		{"required": []string{"id"}},
		{"required": []any{"id"}},
		{"properties": map[string]any{"nested": map[string]any{"type": "string"}}},
		{"items": map[string]any{"type": "boolean"}},
		{"additionalProperties": false},
		{"additionalProperties": map[string]any{"type": "string"}},
	}
	for i, schema := range valid {
		if err := validateMCPInputSchemaSupport("$", schema); err != nil {
			t.Fatalf("valid schema %d rejected: %v", i, err)
		}
	}

	invalid := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "malformed type", schema: map[string]any{"type": true}, want: ".type is not"},
		{name: "malformed dialect", schema: map[string]any{"$schema": true}, want: ".$schema must"},
		{name: "unsupported dialect", schema: map[string]any{"$schema": "https://example.com/schema"}, want: "unsupported dialect"},
		{name: "empty type", schema: map[string]any{"type": []any{}}, want: ".type is not"},
		{name: "non-string union", schema: map[string]any{"type": []any{"string", true}}, want: ".type is not"},
		{name: "empty union member", schema: map[string]any{"type": []string{"string", ""}}, want: ".type is not"},
		{name: "duplicate union member", schema: map[string]any{"type": []string{"string", "string"}}, want: ".type is not"},
		{name: "unsupported type", schema: map[string]any{"type": "uuid"}, want: `unsupported type "uuid"`},
		{name: "malformed enum", schema: map[string]any{"enum": "a"}, want: ".enum must"},
		{name: "empty enum", schema: map[string]any{"enum": []any{}}, want: ".enum must"},
		{name: "duplicate enum", schema: map[string]any{"enum": []any{json.Number("1"), json.Number("1.0")}}, want: ".enum must"},
		{name: "malformed required", schema: map[string]any{"required": "id"}, want: ".required must"},
		{name: "non-string required", schema: map[string]any{"required": []any{1}}, want: ".required must"},
		{name: "empty required", schema: map[string]any{"required": []string{""}}, want: ".required must"},
		{name: "duplicate required", schema: map[string]any{"required": []string{"id", "id"}}, want: ".required must"},
		{name: "malformed properties", schema: map[string]any{"properties": []any{}}, want: ".properties must"},
		{name: "boolean property schema", schema: map[string]any{"properties": map[string]any{"id": true}}, want: "unsupported boolean"},
		{name: "nested property constraint", schema: map[string]any{"properties": map[string]any{"id": map[string]any{"pattern": "x"}}}, want: `keyword "pattern"`},
		{name: "malformed items", schema: map[string]any{"items": true}, want: ".items uses"},
		{name: "nested items constraint", schema: map[string]any{"items": map[string]any{"minimum": 1}}, want: `keyword "minimum"`},
		{name: "malformed additional properties", schema: map[string]any{"additionalProperties": 1}, want: ".additionalProperties must"},
		{name: "nested additional constraint", schema: map[string]any{"additionalProperties": map[string]any{"maxLength": 2}}, want: `keyword "maxLength"`},
		{name: "unsupported root constraint", schema: map[string]any{"$ref": "#/$defs/input"}, want: `keyword "$ref"`},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMCPInputSchemaSupport("$", tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateMCPInputSchemaSupport() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateMCPInputSchemaEnforcesExplicitAdditionalProperties(t *testing.T) {
	t.Parallel()

	err := ValidateMCPInputSchema(map[string]any{"unknown": true}, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	})
	if err == nil || !strings.Contains(err.Error(), "$.unknown is not allowed") {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want additionalProperties=false rejection", err)
	}

	err = ValidateMCPInputSchema(map[string]any{"count": "wrong"}, map[string]any{
		"additionalProperties": map[string]any{"type": "integer"},
	})
	if err == nil || !strings.Contains(err.Error(), "$.count must be integer") {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want additionalProperties schema rejection", err)
	}
}

func TestValidateMCPInputSchemaPreservesWhitespaceRequiredName(t *testing.T) {
	t.Parallel()

	schema := map[string]any{"required": []any{" "}}
	if err := ValidateMCPInputSchema(map[string]any{" ": true}, schema); err != nil {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want whitespace property accepted", err)
	}
	if err := ValidateMCPInputSchema(map[string]any{}, schema); err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("ValidateMCPInputSchema() error = %v, want whitespace property required", err)
	}
}

func TestValidateInputSchemaRejectsMissingRequired(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"required": []any{
			"title",
		},
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}

	err := ValidateInputSchema(map[string]any{}, schema)
	if err == nil {
		t.Fatal("ValidateInputSchema() error = nil, want required validation error")
	}
	if !strings.Contains(err.Error(), "$.title is required") {
		t.Fatalf("ValidateInputSchema() error = %v, want required-field message", err)
	}
}

func TestValidateInputSchemaRejectsEnumMismatch(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type": "string",
				"enum": []any{"auto", "manual"},
			},
		},
	}

	err := ValidateInputSchema(map[string]any{
		"mode": "semi",
	}, schema)
	if err == nil {
		t.Fatal("ValidateInputSchema() error = nil, want enum validation error")
	}
	if !strings.Contains(err.Error(), "$.mode must be one of [auto manual]") {
		t.Fatalf("ValidateInputSchema() error = %v, want enum message", err)
	}
}
