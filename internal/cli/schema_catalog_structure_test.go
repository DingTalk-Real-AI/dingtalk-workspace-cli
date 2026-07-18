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

// TestEmbeddedSchemaCatalogStructure gates the delivered catalog: every tool
// entry must conform to the unified closed structure. If this fails after
// regeneration, either fix the generator inputs or deliberately extend the
// whitelist in schema_catalog_structure.go.
func TestEmbeddedSchemaCatalogStructure(t *testing.T) {
	// After PR #656 the catalog is embedded as per-product shards, not a single
	// JSON file. The loaded snapshot is the reassembled result — serialize it
	// back to JSON and validate the same closed structure.
	loaded := embeddedSchemaCatalog()
	data, err := json.Marshal(loaded.Snapshot)
	if err != nil {
		t.Fatalf("marshal embedded catalog snapshot: %v", err)
	}
	if err := ValidateCatalogStructure(data); err != nil {
		t.Fatalf("embedded schema catalog violates the unified command structure: %v", err)
	}
}

func validCatalogToolEntry() map[string]any {
	return map[string]any{
		"agent_metadata_source": "embedded-skill-metadata",
		"agent_source_refs":     []any{"skills/mono/references/products/aitable.md"},
		"agent_summary":         "创建多维表格记录",
		"agent_summary_source":  "dws-agent-selection/aitable",
		"availability":          "available",
		"avoid_when":            []any{"批量更新时请用 record update"},
		"canonical_path":        "aitable.record_create",
		"cli_name":              "create",
		"cli_path":              "aitable record create",
		"confirmation":          "not_required",
		"description":           "在数据表中创建一条或多条记录。",
		"display":               "多维表格",
		"effect":                "write",
		"effect_source":         "command-verb",
		"examples":              []any{"dws aitable record create --base-id x"},
		"field_provenance":      map[string]any{"effect": map[string]any{"precedence": "reviewed_explicit"}},
		"has_parameters":        true,
		"idempotency":           "non_idempotent",
		"interface_mode":        "mcp",
		"interface_ref":         map[string]any{"product_id": "aitable", "rpc_name": "create_records"},
		"is_alias":              false,
		"name":                  "record create",
		"parameter_count":       float64(1),
		"parameters": map[string]any{
			"base-id": map[string]any{
				"type":             "string",
				"description":      "目标多维表格 ID。",
				"required":         true,
				"field_provenance": map[string]any{"description": map[string]any{"precedence": "reviewed_explicit"}},
			},
		},
		"path":             "aitable.record_create",
		"primary_cli_path": "aitable record create",
		"product_id":       "aitable",
		"reviewed":         true,
		"risk":             "medium",
		"source":           "cobra+registry",
		"title":            "创建记录",
		"use_when":         []any{"需要向多维表格写入新记录时"},
	}
}

func catalogPayload(t *testing.T, entry map[string]any) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"version": SchemaCatalogSnapshotVersion,
		"tools":   map[string]any{"aitable.record_create": entry},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func TestValidateCatalogStructureAcceptsValidEntry(t *testing.T) {
	if err := ValidateCatalogStructure(catalogPayload(t, validCatalogToolEntry())); err != nil {
		t.Fatalf("ValidateCatalogStructure() error = %v", err)
	}
}

func TestValidateCatalogStructureRejectsViolations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(entry map[string]any)
		want   string
	}{
		{
			name:   "unknown tool field",
			mutate: func(e map[string]any) { e["surprise"] = "x" },
			want:   `unknown field "surprise"`,
		},
		{
			name:   "missing required core field",
			mutate: func(e map[string]any) { delete(e, "agent_summary") },
			want:   `"agent_summary" must be a non-empty string`,
		},
		{
			name:   "bad effect enum",
			mutate: func(e map[string]any) { e["effect"] = "mutate" },
			want:   `"effect" = "mutate"`,
		},
		{
			name:   "mcp without interface_ref",
			mutate: func(e map[string]any) { delete(e, "interface_ref") },
			want:   "interface_mode=mcp requires interface_ref",
		},
		{
			name: "composite without interface_reason",
			mutate: func(e map[string]any) {
				e["interface_mode"] = "composite"
				delete(e, "interface_ref")
			},
			want: "interface_mode=composite requires a non-empty interface_reason",
		},
		{
			name: "composite must not keep interface_ref",
			mutate: func(e map[string]any) {
				e["interface_mode"] = "composite"
				e["interface_reason"] = "本地组合多个 MCP 调用"
			},
			want: "interface_mode=composite must not set interface_ref",
		},
		{
			name:   "parameter_count mismatch",
			mutate: func(e map[string]any) { e["parameter_count"] = float64(2) },
			want:   "parameter_count = 2, want 1",
		},
		{
			name:   "has_parameters mismatch",
			mutate: func(e map[string]any) { e["has_parameters"] = false },
			want:   "has_parameters = false, want true",
		},
		{
			name: "param missing required",
			mutate: func(e map[string]any) {
				params := e["parameters"].(map[string]any)
				delete(params["base-id"].(map[string]any), "required")
			},
			want: `parameter "base-id": required must be a boolean`,
		},
		{
			name: "param unknown field",
			mutate: func(e map[string]any) {
				params := e["parameters"].(map[string]any)
				params["base-id"].(map[string]any)["mystery"] = true
			},
			want: `parameter "base-id": unknown field "mystery"`,
		},
		{
			name: "param bad type enum",
			mutate: func(e map[string]any) {
				params := e["parameters"].(map[string]any)
				params["base-id"].(map[string]any)["type"] = "text"
			},
			want: `parameter "base-id": type = "text"`,
		},
		{
			name: "param default must be string",
			mutate: func(e map[string]any) {
				params := e["parameters"].(map[string]any)
				params["base-id"].(map[string]any)["default"] = float64(3)
			},
			want: `parameter "base-id": default must be a string`,
		},
		{
			name:   "reviewed must be bool",
			mutate: func(e map[string]any) { e["reviewed"] = "yes" },
			want:   `"reviewed" must be a boolean`,
		},
		{
			name:   "examples must be string array",
			mutate: func(e map[string]any) { e["examples"] = []any{"ok", float64(1)} },
			want:   `"examples" must be an array of strings`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := validCatalogToolEntry()
			tc.mutate(entry)
			err := ValidateCatalogStructure(catalogPayload(t, entry))
			if err == nil {
				t.Fatalf("ValidateCatalogStructure() = nil, want violation containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateCatalogStructure() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateCatalogStructureRejectsBadEnvelope(t *testing.T) {
	if err := ValidateCatalogStructure([]byte(`{"version":99,"tools":{}}`)); err == nil {
		t.Fatal("ValidateCatalogStructure() = nil, want version error")
	}
	if err := ValidateCatalogStructure([]byte(`{"version":1,"tools":{}}`)); err == nil {
		t.Fatal("ValidateCatalogStructure() = nil, want empty-tools error")
	}
	if err := ValidateCatalogStructure([]byte(`not json`)); err == nil {
		t.Fatal("ValidateCatalogStructure() = nil, want decode error")
	}
}
