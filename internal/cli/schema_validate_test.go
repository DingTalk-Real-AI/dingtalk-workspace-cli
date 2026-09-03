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
