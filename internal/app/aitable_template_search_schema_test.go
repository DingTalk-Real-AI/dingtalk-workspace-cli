// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAITableTemplateSearchFinalSchemaRequiresQuery(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	for _, test := range []struct {
		canonical string
		cliPath   string
	}{
		{canonical: "aitable.shortcut_template_search", cliPath: "aitable +template-search"},
		{canonical: "aitable.template_search", cliPath: "aitable template search"},
	} {
		canonical := test.canonical
		tool := snapshot.Tools[canonical]
		if tool == nil {
			t.Fatalf("final Schema missing %s", canonical)
		}
		parameters := schemaContractMap(tool["parameters"])
		query := parameters["query"]
		if query == nil {
			t.Fatalf("%s final Schema missing query parameter", canonical)
		}
		if required, _ := query["required"].(bool); !required {
			t.Fatalf("%s query required=%#v", canonical, query["required"])
		}
		if canonical == "aitable.template_search" {
			if required, _ := query["cli_required"].(bool); !required {
				t.Fatalf("%s query cli_required=%#v", canonical, query["cli_required"])
			}
		}
		if canonical == "aitable.shortcut_template_search" {
			if tool["result"] == nil {
				t.Fatal("shortcut template search final Schema missing Result")
			}
			result, _ := tool["result"].(map[string]any)
			dataSchema, _ := result["data_schema"].(map[string]any)
			properties, _ := dataSchema["properties"].(map[string]any)
			templates, _ := properties["templates"].(map[string]any)
			items, _ := templates["items"].(map[string]any)
			itemProperties, _ := items["properties"].(map[string]any)
			templateName, _ := itemProperties["templateName"].(map[string]any)
			if schemaContractString(templateName["type"]) != "string" {
				t.Fatalf("templateName type = %#v", templateName["type"])
			}
			pagination, _ := tool["pagination"].(map[string]any)
			if schemaContractString(pagination["cursor_parameter"]) != "cursor" {
				t.Fatalf("shortcut template search pagination = %#v", pagination)
			}
		}
		texts := []string{schemaContractString(tool["description"])}
		if useWhen, ok := tool["use_when"].([]any); ok {
			for _, item := range useWhen {
				texts = append(texts, schemaContractString(item))
			}
		}
		for _, text := range texts {
			if strings.Contains(text, "返回热门") || strings.Contains(text, "不传关键词") {
				t.Fatalf("%s final Schema still claims unsupported fallback: %q", canonical, text)
			}
		}

		root := NewRootCommand()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs([]string{"schema", "--cli-path", test.cliPath, "--compact", "--format", "json"})
		if err := root.Execute(); err != nil {
			t.Fatalf("compact Schema %s: %v; %s", test.cliPath, err, stderr.String())
		}
		var compact map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &compact); err != nil {
			t.Fatalf("decode compact Schema %s: %v", test.cliPath, err)
		}
		compactQuery := schemaContractMap(compact["parameters"])["query"]
		if required, _ := compactQuery["required"].(bool); !required {
			t.Fatalf("compact %s query required=%#v", test.cliPath, compactQuery["required"])
		}
	}
}
