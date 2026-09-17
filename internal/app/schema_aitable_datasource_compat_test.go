// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDatasourceUpdateOptionalConfigFinalSchema(t *testing.T) {
	for _, canonical := range []string{"aitable.datasource_update", "aitable.shortcut_datasource_update"} {
		t.Run(canonical, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				args := []string{canonical}
				if compact {
					args = append(args, "--compact")
				}
				leaf := executeShortcutSchemaQuery(t, args...)
				param := schemaContractMap(leaf["parameters"])["source-config"]
				if param == nil || param["type"] != "string" || param["required"] == true || param["cli_required"] == true {
					t.Fatalf("compact=%v source-config = %#v, want optional string", compact, param)
				}
				if description := schemaContractString(param["description"]); !strings.Contains(description, "省略时先读取当前") || !strings.Contains(description, "读取失败则不更新") {
					t.Fatalf("missing preservation semantics: %q", description)
				}
				if !compact {
					if param["property"] != "sourceConfig" {
						t.Fatalf("sourceConfig binding = %v", param)
					}
					if canonical == "aitable.datasource_update" {
						ref := schemaInterfaceObject(leaf["interface_ref"])
						if leaf["interface_mode"] != "mcp" || ref["product_id"] != "aitable" || ref["rpc_name"] != "update_datasource_config" {
							t.Fatalf("historical update interface binding changed: mode=%v ref=%v", leaf["interface_mode"], ref)
						}
					} else if leaf["interface_mode"] != "composite" || leaf["interface_ref"] != nil {
						t.Fatalf("shortcut interface binding changed: mode=%v ref=%v", leaf["interface_mode"], leaf["interface_ref"])
					}
				}
			}
		})
	}
	// Creation still requires a caller-supplied configuration.
	for _, canonical := range []string{"aitable.datasource_create", "aitable.shortcut_datasource_create"} {
		leaf := executeShortcutSchemaQuery(t, canonical, "--compact")
		if param := schemaContractMap(leaf["parameters"])["source-config"]; param["required"] != true {
			t.Fatalf("%s source-config lost requiredness: %v", canonical, param)
		}
	}
}
