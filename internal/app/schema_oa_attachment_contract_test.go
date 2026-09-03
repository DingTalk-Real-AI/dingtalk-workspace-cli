// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOAAttachmentDeliveredSchemaMatchesExecutableHelp(t *testing.T) {
	tests := []struct {
		cliPath        string
		canonical      string
		rpc            string
		description    string
		effect         string
		resultType     string
		resultFields   map[string]string
		sensitivePaths []string
	}{
		{
			cliPath:     "oa approval attachment download-url",
			canonical:   "oa.get_attachment_download_url",
			rpc:         "get_attachment_download_url",
			description: "获取审批附件下载授权并生成临时下载链接",
			effect:      "read",
			resultType:  "object",
			resultFields: map[string]string{
				"spaceId": "integer", "agentId": "integer", "downloadUri": "string",
				"class": "string", "fileId": "string",
			},
			sensitivePaths: []string{"downloadUri"},
		},
		{
			cliPath:     "oa approval attachment authorize-download",
			canonical:   "oa.auth_download_file",
			rpc:         "auth_download_file",
			description: "批量授权当前用户下载指定的审批钉盘文件",
			effect:      "write",
			resultType:  "boolean",
		},
		{
			cliPath:     "oa approval attachment authorize-preview",
			canonical:   "oa.auth_preview_attachment",
			rpc:         "auth_preview_attachment",
			description: "批量授权当前用户预览审批单中的附件",
			effect:      "write",
			resultType:  "object",
			resultFields: map[string]string{
				"spaceId": "integer", "agentId": "integer", "class": "string",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.rpc, func(t *testing.T) {
			root := NewRootCommand()
			command := exactCommandForTest(root, test.cliPath)
			if command == nil {
				t.Fatalf("executable command %q is missing", test.cliPath)
			}

			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"schema", test.cliPath, "--format", "json"})
			if err := root.Execute(); err != nil {
				t.Fatalf("execute delivery schema leaf: %v; stderr=%s", err, stderr.String())
			}

			var tool map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &tool); err != nil {
				t.Fatalf("decode delivery schema leaf: %v", err)
			}
			if got := schemaContractString(tool["canonical_path"]); got != test.canonical {
				t.Fatalf("canonical_path = %q, want %q", got, test.canonical)
			}
			if got := schemaContractString(tool["primary_cli_path"]); got != test.cliPath {
				t.Fatalf("primary_cli_path = %q, want %q", got, test.cliPath)
			}
			if got := schemaContractString(tool["description"]); !strings.HasPrefix(got, test.description) {
				t.Fatalf("description = %q, want prefix %q", got, test.description)
			}
			if got := schemaContractString(tool["interface_mode"]); got != "mcp" {
				t.Fatalf("interface_mode = %q, want mcp", got)
			}
			if got := schemaContractString(tool["availability"]); got != "available" {
				t.Fatalf("availability = %q, want available", got)
			}
			interfaceRef := schemaInterfaceObject(tool["interface_ref"])
			if got := schemaContractString(interfaceRef["product_id"]); got != "oa" {
				t.Fatalf("interface_ref.product_id = %q, want oa", got)
			}
			if got := schemaContractString(interfaceRef["rpc_name"]); got != test.rpc {
				t.Fatalf("interface_ref.rpc_name = %q, want %q", got, test.rpc)
			}
			if got := schemaContractString(tool["effect"]); got != test.effect {
				t.Fatalf("effect = %q, want %q", got, test.effect)
			}
			if got := schemaContractString(tool["risk"]); got != "low" {
				t.Fatalf("risk = %q, want low", got)
			}
			if got := schemaContractString(tool["confirmation"]); got != "not_required" {
				t.Fatalf("confirmation = %q, want not_required", got)
			}
			if got := schemaContractString(tool["idempotency"]); got != "idempotent" {
				t.Fatalf("idempotency = %q, want idempotent", got)
			}
			fullResult := oaAttachmentResultContract(t, tool, test.resultType, test.resultFields, test.sensitivePaths)

			stdout.Reset()
			stderr.Reset()
			root.SetArgs([]string{"schema", test.cliPath, "--compact", "--format", "json"})
			if err := root.Execute(); err != nil {
				t.Fatalf("execute compact delivery schema leaf: %v; stderr=%s", err, stderr.String())
			}
			var compactTool map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &compactTool); err != nil {
				t.Fatalf("decode compact delivery schema leaf: %v", err)
			}
			compactResult, ok := compactTool["result"].(map[string]any)
			if !ok {
				t.Fatalf("compact result = %#v, want object", compactTool["result"])
			}
			if !reflect.DeepEqual(compactResult, fullResult) {
				t.Fatalf("compact/full result projection differs\ncompact: %#v\nfull: %#v", compactResult, fullResult)
			}
			if problem := schemaHelpFlagCompletenessProblem(test.canonical, test.cliPath, command, tool); problem != "" {
				t.Fatal(problem)
			}
		})
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadDeliversCompositeSchema 验证合并后的
// upload 命令以 composite 接口模式交付：它内部串联 init/commit 两个 RPC 与本地 HTTP PUT，
// 无法绑定单一 interface_ref，因此不进入上面按 mcp 模式断言的表驱动用例。
func TestCrossPlatformCoverageOAAttachmentUploadDeliversCompositeSchema(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	tool := snapshot.Tools["oa.attachment_upload"]
	if tool == nil {
		t.Fatal("oa.attachment_upload is missing from final Schema")
	}
	if got := schemaContractString(tool["primary_cli_path"]); got != "oa approval attachment upload" {
		t.Fatalf("primary_cli_path = %q, want oa approval attachment upload", got)
	}
	if got := schemaContractString(tool["interface_mode"]); got != "composite" {
		t.Fatalf("interface_mode = %q, want composite", got)
	}
	if got := schemaContractString(tool["availability"]); got != "available" {
		t.Fatalf("availability = %q, want available", got)
	}
	if got := schemaContractString(tool["interface_reason"]); got == "" {
		t.Fatal("composite upload command must document an interface reason")
	}
	if got := schemaContractString(tool["effect"]); got != "write" {
		t.Fatalf("effect = %q, want write", got)
	}
	if got := schemaContractString(tool["risk"]); got != "low" {
		t.Fatalf("risk = %q, want low", got)
	}
	if got := schemaContractString(tool["confirmation"]); got != "not_required" {
		t.Fatalf("confirmation = %q, want not_required", got)
	}
	parameters := schemaContractMap(tool["parameters"])
	for _, flag := range []string{"file", "file-name", "md5"} {
		if parameters[flag] == nil {
			t.Fatalf("upload --%s is missing from final Schema", flag)
		}
	}
	if required, _ := parameters["file"]["required"].(bool); !required {
		t.Fatalf("upload --file required = %#v, want true", parameters["file"]["required"])
	}
	result := schemaContractMap(tool["result"])
	dataSchema := schemaContractMap(result["data_schema"])
	properties := schemaContractMap(dataSchema["properties"])
	if properties["fileId"] == nil {
		t.Fatal("upload Result data_schema is missing fileId")
	}
}

func oaAttachmentResultContract(t *testing.T, tool map[string]any, resultType string, fields map[string]string, sensitivePaths []string) map[string]any {
	t.Helper()
	result, ok := tool["result"].(map[string]any)
	if !ok {
		t.Fatalf("full result = %#v, want object", tool["result"])
	}
	if got, want := schemaContractStringSlice(result["outcomes"]), []string{"success", "failure"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("result.outcomes = %#v, want %#v", got, want)
	}
	if got := schemaContractStringSlice(result["sensitive_paths"]); !reflect.DeepEqual(got, sensitivePaths) {
		t.Fatalf("result.sensitive_paths = %#v, want %#v", got, sensitivePaths)
	}
	dataSchema, ok := result["data_schema"].(map[string]any)
	if !ok || schemaContractString(dataSchema["type"]) != resultType {
		t.Fatalf("result.data_schema = %#v, want type %q", result["data_schema"], resultType)
	}
	properties, _ := dataSchema["properties"].(map[string]any)
	if len(properties) != len(fields) {
		t.Fatalf("result.data_schema.properties = %#v, want fields %#v", properties, fields)
	}
	for name, fieldType := range fields {
		property, ok := properties[name].(map[string]any)
		if !ok || schemaContractString(property["type"]) != fieldType {
			t.Fatalf("result.data_schema.properties.%s = %#v, want type %q", name, properties[name], fieldType)
		}
	}
	return result
}
