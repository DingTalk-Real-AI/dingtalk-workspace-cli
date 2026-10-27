// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"maps"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageAITableSFCapabilitiesReachFinalSchema(t *testing.T) {
	for path, rpc := range map[string]string{
		"aitable field ai-plugins":        "list_builtin_ai_field_plugins",
		"aitable field-group list":        "list_field_groups",
		"aitable field-group create":      "create_field_group",
		"aitable field-group update":      "update_field_group",
		"aitable field-group ungroup":     "ungroup_field_group",
		"aitable field-group move-fields": "move_field_group_fields",
	} {
		t.Run(path, func(t *testing.T) {
			full := executeShortcutSchemaQuery(t, "--cli-path", path)
			compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
			if full["result"] == nil || !schemaContractJSONEqual(full["result"], compact["result"]) {
				t.Fatalf("Result drift: full=%#v compact=%#v", full["result"], compact["result"])
			}
			ref := schemaInterfaceObject(full["interface_ref"])
			if full["interface_mode"] != "mcp" || ref["product_id"] != "aitable" || ref["rpc_name"] != rpc {
				t.Fatalf("interface=%#v", full["interface_ref"])
			}
			read := rpc == "list_field_groups" || rpc == "list_builtin_ai_field_plugins"
			wantConfirmation := "user_required"
			if read {
				wantConfirmation = "not_required"
			}
			if full["confirmation"] != wantConfirmation || compact["confirmation"] != wantConfirmation {
				t.Fatalf("confirmation drift: full=%#v compact=%#v", full["confirmation"], compact["confirmation"])
			}
			if full["dry_run"] == nil || compact["dry_run"] == nil {
				t.Fatal("missing reviewed request preview")
			}
			params := schemaContractMap(full["parameters"])
			if params["base-id"]["required"] != true || params["base-id"]["property"] != "baseId" {
				t.Fatalf("base parameter=%#v", params["base-id"])
			}
			if rpc == "update_field_group" {
				assertSchemaContractConstraintGroup(t, full, "require_one_of", []string{"name", "description"})
				if !strings.Contains(params["name"]["description"].(string), "非空白") || !strings.Contains(full["description"].(string), "非空白") || !strings.Contains(compact["description"].(string), "非空白") {
					t.Fatalf("update name validation is missing from final Schema: %#v", full)
				}
			}
			if rpc == "move_field_group_fields" {
				if params["target-group-id"]["required"] == true || !strings.Contains(params["target-group-id"]["description"].(string), "非空白") {
					t.Fatalf("target group presence contract drift: %#v", params["target-group-id"])
				}
				if !strings.Contains(full["description"].(string), "非空白") || !strings.Contains(compact["description"].(string), "非空白") {
					t.Fatalf("move target validation is missing from final Schema: %#v", full)
				}
			}
			if (strings.HasPrefix(rpc, "move_") || rpc == "create_field_group") && params["confirm-empty-group"]["property"] != "confirmEmptyGroup" {
				t.Fatal("empty source-group confirmation mapping drift")
			}
			if rpc == "move_field_group_fields" || rpc == "create_field_group" {
				if !strings.Contains(params["field-ids"]["description"].(string), "不重复") {
					t.Fatalf("unique field IDs requirement is missing from final Schema: %#v", params["field-ids"])
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAITableSFMinimalPendingResultsMatchSchemas(t *testing.T) {
	groupPending := map[string]any{"baseId": "B", "tableId": "T", "verificationStatus": "pending", "warningMessage": "readback unavailable"}
	for _, tc := range []struct {
		path       string
		data       map[string]any
		collection string
		required   []string
	}{
		{"aitable field ai-plugins", map[string]any{"baseId": "B", "tableId": "T", "plugins": []any{map[string]any{"pluginId": "ai-agent", "name": "智能体", "fieldTypes": []any{"text"}, "configurationMode": "unsupported", "reason": "Use the existing ai-agent protocol"}}}, "plugins", []string{"baseId", "tableId", "plugins"}},
		{"aitable field-group list", map[string]any{"baseId": "B", "tableId": "T", "groups": []any{}}, "groups", []string{"baseId", "tableId", "groups"}},
		{"aitable field-group create", groupPending, "groups", []string{"baseId", "tableId", "verificationStatus"}},
		{"aitable field-group update", groupPending, "groups", []string{"baseId", "tableId", "verificationStatus"}},
		{"aitable field-group ungroup", groupPending, "groups", []string{"baseId", "tableId", "verificationStatus"}},
		{"aitable field-group move-fields", groupPending, "groups", []string{"baseId", "tableId", "verificationStatus"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				query := []string{"--cli-path", tc.path}
				if compact {
					query = append(query, "--compact")
				}
				leaf := executeShortcutSchemaQuery(t, query...)
				result := leaf["result"].(map[string]any)
				compiler := jsonschema.NewCompiler()
				compiler.DefaultDraft(jsonschema.Draft2020)
				const resource = "https://dws.test/sf-result.json"
				if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
					t.Fatal(err)
				}
				schema, err := compiler.Compile(resource)
				if err != nil {
					t.Fatal(err)
				}
				if err := schema.Validate(tc.data); err != nil {
					t.Fatalf("compact=%t minimal receipt rejected: %v", compact, err)
				}
				for _, field := range tc.required {
					invalid := maps.Clone(tc.data)
					delete(invalid, field)
					if err := schema.Validate(invalid); err == nil {
						t.Fatalf("compact=%t missing %s accepted", compact, field)
					}
				}
				if tc.collection == "" {
					continue
				}
				withCollection := maps.Clone(tc.data)
				withCollection[tc.collection] = []any{}
				if err := schema.Validate(withCollection); err != nil {
					t.Fatalf("compact=%t array receipt rejected: %v", compact, err)
				}
				for _, value := range []any{nil, "not-an-array", map[string]any{}} {
					invalid := maps.Clone(tc.data)
					invalid[tc.collection] = value
					if err := schema.Validate(invalid); err == nil {
						t.Fatalf("compact=%t invalid %s accepted: %#v", compact, tc.collection, value)
					}
				}
			}
		})
	}
}
