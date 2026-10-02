package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOATemplateDeliveredContract(t *testing.T) {
	for _, tc := range []struct{ command, tool string }{{"list", "list_manage_templates"}, {"detail", "get_template_detail"}} {
		t.Run(tc.command, func(t *testing.T) {
			root := NewRootCommand()
			path := "oa approval template " + tc.command
			command := exactCommandForTest(root, path)
			if command == nil {
				t.Fatal("missing executable")
			}
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetArgs([]string{"schema", path, "--format", "json"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			var leaf map[string]any
			if err := json.Unmarshal(buf.Bytes(), &leaf); err != nil {
				t.Fatal(err)
			}
			if leaf["canonical_path"] != "oa."+tc.tool || leaf["effect"] != "read" || leaf["confirmation"] != "not_required" {
				t.Fatalf("identity/safety: %#v", leaf)
			}
			if schemaInterfaceObject(leaf["interface_ref"])["rpc_name"] != tc.tool {
				t.Fatal("wrong MCP interface")
			}
			if leaf["primary_cli_path"] != path {
				t.Fatalf("primary path = %v, want %s", leaf["primary_cli_path"], path)
			}
			if leaf["result"] == nil {
				t.Fatal("missing result contract")
			}
			params := schemaContractMap(leaf["parameters"])
			if tc.command == "list" {
				if len(params) != 0 {
					t.Fatalf("unexpected parameters: %#v", params)
				}
			} else {
				if command.Flags().Lookup("process-code") == nil || command.Flags().Lookup("template-code") != nil {
					t.Fatal("template detail help flags do not match the new contract")
				}
				p := params["process-code"]
				if len(params) != 1 || p["type"] != "string" || p["property"] != "processCodes" || p["interface_type"] != "array" || p["required"] != true {
					t.Fatalf("parameter contract: %#v", params)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageOATemplateResultContractsAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		command  string
		fields   []string
		excluded []string
	}{
		{"list", []string{"processCode", "flowTitle"}, []string{"name", "schemaContent", "processConfig"}},
		{"detail", []string{"processCode", "name", "schemaContent", "processConfig"}, []string{"flowTitle"}},
	} {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact=%t", tc.command, compact), func(t *testing.T) {
				root := NewRootCommand()
				var stdout bytes.Buffer
				root.SetOut(&stdout)
				args := []string{"schema", "oa approval template " + tc.command, "--format", "json"}
				if compact {
					args = append(args, "--compact")
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				var leaf struct {
					Result struct {
						DataSchema struct {
							Properties map[string]struct {
								Items struct {
									Properties map[string]json.RawMessage `json:"properties"`
								} `json:"items"`
							} `json:"properties"`
						} `json:"data_schema"`
					} `json:"result"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &leaf); err != nil {
					t.Fatal(err)
				}
				fields := leaf.Result.DataSchema.Properties["templates"].Items.Properties
				for _, name := range tc.fields {
					if _, ok := fields[name]; !ok {
						t.Errorf("missing %s result field %s", tc.command, name)
					}
				}
				for _, name := range tc.excluded {
					if _, ok := fields[name]; ok {
						t.Errorf("%s declares field owned by the other API: %s", tc.command, name)
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOATemplateWriteDeliveredContract(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		for _, compact := range []bool{false, true} {
			t.Run(command+map[bool]string{true: "/compact", false: "/full"}[compact], func(t *testing.T) {
				root := NewRootCommand()
				path := "oa approval template " + command
				leafCmd := exactCommandForTest(root, path)
				if leafCmd == nil {
					t.Fatal("missing command")
				}
				var buf bytes.Buffer
				root.SetOut(&buf)
				args := []string{"schema", path, "--format", "json"}
				if compact {
					args = append(args, "--compact")
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				var leaf map[string]any
				if err := json.Unmarshal(buf.Bytes(), &leaf); err != nil {
					t.Fatal(err)
				}
				if leaf["canonical_path"] != "oa."+command+"_process_template" || leaf["effect"] != "write" || leaf["risk"] != "medium" || leaf["confirmation"] != "not_required" || leaf["idempotency"] != "unknown" {
					t.Fatalf("identity/safety: %#v", leaf)
				}
				params := schemaContractMap(leaf["parameters"])
				if params["from-document"] == nil || leafCmd.Flags().Lookup("from-document") == nil {
					t.Fatal("missing document input")
				}
				if params["from-document"]["property"] != nil && params["from-document"]["property"] != "" {
					t.Fatal("local file path exposes an RPC property")
				}
				constraints, _ := leaf["constraints"].(map[string]any)
				requiredGroups, _ := constraints["require_one_of"].([]any)
				wantGroups := 2
				if command == "update" {
					wantGroups = 4
				}
				if len(requiredGroups) != wantGroups {
					t.Fatalf("document alternative constraints: %#v", constraints)
				}

				for _, name := range []string{"name", "schema-content", "process-config", "description", "icon-url", "form-config", "node-config", "condition-rule", "plugin-configs", "visible-range", "manager-user-ids", "static-workflow", "append-enable", "duplicate-removal"} {
					if params[name] == nil || leafCmd.Flags().Lookup(name) == nil {
						t.Fatalf("missing parameter %s", name)
					}
					required := name == "name" || name == "schema-content" || command == "update" && name == "process-config"
					if params[name]["required"] == true || (required && params[name]["required_when"] != "未提供 --from-document 时必填") {
						t.Fatalf("wrong requiredness %s: %#v", name, params[name])
					}
				}
				if command == "update" {
					if params["expected-version"] != nil || leafCmd.Flags().Lookup("expected-version") != nil {
						t.Fatal("update exposes expected-version")
					}
					if params["process-code"]["required"] == true || params["process-code"]["required_when"] != "未提供 --from-document 时必填" {
						t.Fatal("process-code not required")
					}
				}
				if !compact {
					if schemaInterfaceObject(leaf["interface_ref"])["rpc_name"] != command+"_process_template" {
						t.Fatal("wrong interface")
					}
					for name, property := range map[string]string{"schema-content": "schemaContent", "process-config": "processConfig", "plugin-configs": "pluginConfigs", "manager-user-ids": "managerUserIds", "visible-range": "visibleRange"} {
						kind := "string"
						if strings.Contains(name, "configs") || name == "manager-user-ids" || name == "visible-range" {
							kind = "array"
						}
						interfaceType := params[name]["interface_type"]
						if interfaceType == nil {
							interfaceType = params[name]["type"]
						}
						if params[name]["property"] != property || interfaceType != kind {
							t.Fatalf("mapping: %#v", params[name])
						}
					}
					if command == "create" && (params["expected-version"]["interface_type"] != "number" || params["expected-version"]["required"] == true) {
						t.Fatal("create version contract")
					}
				}
				result := leaf["result"].(map[string]any)["data_schema"].(map[string]any)
				if result["properties"].(map[string]any)["processCode"] == nil {
					t.Fatal("missing result processCode")
				}
			})
		}
	}
}
