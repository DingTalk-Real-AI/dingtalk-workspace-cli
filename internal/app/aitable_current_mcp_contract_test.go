// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAitableChartCreateLayoutGuidanceReachesHelpAndSchema(t *testing.T) {
	root := NewRootCommand()
	var help bytes.Buffer
	root.SetOut(&help)
	root.SetErr(&help)
	root.SetArgs([]string{"aitable", "chart", "create", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	full := executeShortcutSchemaQuery(t, "--cli-path", "aitable chart create")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "aitable chart create", "--compact")
	fullDescription, _ := full["description"].(string)
	compactDescription, _ := compact["description"].(string)
	if fullDescription == "" || fullDescription != compactDescription {
		t.Fatalf("chart create full/compact descriptions differ: %q / %q", fullDescription, compactDescription)
	}
	for label, text := range map[string]string{"help": help.String(), "full schema": fullDescription, "compact schema": compactDescription} {
		for _, required := range []string{"允许大小卡混排和纵向叠放", "不强制同行等高或每行铺满", "前端默认纵向压缩", "回读 config/layout", "固定视口验收", "不默认追加 arrange"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing layout guidance %q: %s", label, required, text)
			}
		}
		for _, obsolete := range []string{"同一根布局行的图表保持高度一致", "每行图表宽度之和应正好填满"} {
			if strings.Contains(text, obsolete) {
				t.Fatalf("%s still forces a uniform filled row: %s", label, text)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableDiagnoseGridRulesReachHelpAndSchema(t *testing.T) {
	root := NewRootCommand()
	var help bytes.Buffer
	root.SetOut(&help)
	root.SetErr(&help)
	root.SetArgs([]string{"aitable", "+dashboard-diagnose", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	full := executeShortcutSchemaQuery(t, "--cli-path", "aitable +dashboard-diagnose")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "aitable +dashboard-diagnose", "--compact")
	if !schemaContractJSONEqual(full["result"], compact["result"]) {
		t.Fatal("diagnose grid rules lost in compact result")
	}
	for label, text := range map[string]string{
		"help": help.String(), "full schema": full["description"].(string), "compact schema": compact["description"].(string),
	} {
		for _, required := range []string{"schemaVersionTypeVerified=true", "JSON number 2", "JSON string 2 不转换", "48 列", "12 列", "子容器使用独立坐标系"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing root grid rule %q", label, required)
			}
		}
	}
	result := compact["result"].(map[string]any)
	properties := result["data_schema"].(map[string]any)["properties"].(map[string]any)
	description := properties["totalColumns"].(map[string]any)["description"].(string)
	for _, required := range []string{"Base/Dashboard", "schemaVersionTypeVerified=true", "V2=48", "V1=12", "JSON string 2 不转为 number 2", "子容器独立"} {
		if !strings.Contains(description, required) {
			t.Fatalf("compact totalColumns missing rule %q", required)
		}
	}
}

func TestCrossPlatformCoverageAitableAccelerationScopeOmitsDeferredWrites(t *testing.T) {
	root := NewRootCommand()
	hasLeaf := func(path string) bool {
		current := root
		for _, name := range strings.Fields(path) {
			found := false
			for _, child := range current.Commands() {
				if child.Name() == name {
					current, found = child, true
					break
				}
			}
			if !found {
				return false
			}
		}
		return current.Runnable()
	}
	for _, path := range []string{"aitable chart layout-update", "aitable +chart-layout-update"} {
		if hasLeaf(path) {
			t.Fatalf("deferred write remains executable: %s", path)
		}
	}
	for _, path := range []string{"aitable chart batch-create", "aitable +charts-create", "aitable chart templates", "aitable +dashboard-preview", "aitable +dashboard-diagnose", "aitable +field-wait-ai", "aitable +task-report"} {
		if !hasLeaf(path) {
			t.Fatalf("acceleration command was removed: %s", path)
		}
	}
}

func TestCrossPlatformCoverageAitableCurrentMCPQueryEvidenceReachesCompactSchema(t *testing.T) {
	for _, path := range []string{"aitable field task-status", "aitable +field-wait-ai"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if !schemaContractJSONEqual(full["result"], compact["result"]) {
			t.Fatalf("%s full/compact result differ", path)
		}
		result := compact["result"].(map[string]any)
		schema := result["data_schema"].(map[string]any)
		if branches, ok := schema["oneOf"].([]any); ok {
			schema = branches[0].(map[string]any)
		}
		properties := schema["properties"].(map[string]any)
		for _, name := range []string{"queryComplete", "queryRetryable", "queryBudgetExhausted"} {
			property, ok := properties[name].(map[string]any)
			if !ok || property["type"] != "boolean" || property["description"] == "" {
				t.Fatalf("%s missing published query evidence %s: %#v", path, name, properties)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableTemplateDocumentationReachesCompactSchema(t *testing.T) {
	for _, path := range []string{"aitable chart templates", "aitable +chart-templates"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if !schemaContractJSONEqual(full["result"], compact["result"]) {
			t.Fatalf("%s full/compact result differ", path)
		}
		for label, payload := range map[string]map[string]any{"full": full, "compact": compact} {
			avoid := strings.Join(schemaContractStringSlice(payload["avoid_when"]), " ")
			for _, required := range []string{"documentation 为增强说明", "依据随包指南与模板继续语义明确的配置", "仅具体必需信息无法确认时停止", "异常或不完整说明整体丢弃", "保留合法 templates", "向 stderr 写入非阻断诊断", "不补发全量请求"} {
				if !strings.Contains(avoid, required) {
					t.Fatalf("%s %s missing selection guidance %q: %s", path, label, required, avoid)
				}
			}
			if strings.Contains(avoid, "复杂配置停止并报告能力缺口") {
				t.Fatalf("%s %s still blocks old templates-only results: %s", path, label, avoid)
			}
		}
		result := compact["result"].(map[string]any)
		schema := result["data_schema"].(map[string]any)
		if branches, ok := schema["oneOf"].([]any); ok {
			schema = branches[0].(map[string]any)
		}
		if !schemaContractJSONEqual(schema["required"], []string{"templates"}) {
			t.Fatalf("%s no longer allows old templates-only results", path)
		}
		properties := schema["properties"].(map[string]any)
		docs, ok := properties["documentation"].(map[string]any)
		if !ok || docs["type"] != "object" || docs["description"] == "" || !schemaContractJSONEqual(docs["required"], []string{"common", "byType"}) {
			t.Fatalf("%s missing documentation contract: %#v", path, properties)
		}
		for _, required := range []string{"增强说明", "依据随包指南与模板继续语义明确的配置", "仅具体必需信息无法确认时停止", "异常或不完整说明整体丢弃", "保留合法 templates", "向 stderr 写入非阻断诊断", "不伪造原文"} {
			if !strings.Contains(docs["description"].(string), required) {
				t.Fatalf("%s missing compatibility guidance %q: %#v", path, required, docs)
			}
		}
		fields := docs["properties"].(map[string]any)
		common := fields["common"].(map[string]any)
		byType := fields["byType"].(map[string]any)
		entry := byType["additionalProperties"].(map[string]any)
		if common["type"] != "string" || common["description"] == "" || byType["type"] != "object" || byType["description"] == "" || entry["type"] != "string" {
			t.Fatalf("%s lost preserved-string documentation schema: %#v", path, fields)
		}
	}
}

func TestCrossPlatformCoverageAitableGoalClosureCommandsReachCompactSchema(t *testing.T) {
	for path, flags := range map[string][]string{
		"aitable +task-report":        {"checkpoint", "baseline", "model-metrics", "baseline-model-metrics"},
		"aitable +field-wait-ai":      {"checkpoint", "resume"},
		"aitable +dashboard-diagnose": {"expected-layout", "regions"},
	} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if !schemaContractJSONEqual(full["result"], compact["result"]) {
			t.Fatalf("%s result differs between projections", path)
		}
		parameters := compact["parameters"].(map[string]any)
		for _, flag := range flags {
			if parameters[flag] == nil {
				t.Fatalf("%s missing %s", path, flag)
			}
		}
	}
}
