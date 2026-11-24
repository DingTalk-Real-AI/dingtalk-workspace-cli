// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"
)

// TestSportOrgAdminLeafSchemaPublishesTriStateResult 验证 sport.is_org_admin
// 的最终 embedded Schema 投影：full 与 compact 都必须发布布尔三态 result（true=
// 管理员 / false=确认非管理员 / success=false 时报错），无业务入参，且两个投影
// 一致。按 AGENTS.md 要求，full leaf、compact leaf 与 runtime delivery 必须通
// 过最终 embedded Schema 路径覆盖；runtime 侧用例见 helpers 包。
func TestSportOrgAdminLeafSchemaPublishesTriStateResult(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "sport org admin")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "sport org admin", "--compact")
	for name, leaf := range map[string]map[string]any{"full": full, "compact": compact} {
		if len(leaf) == 0 {
			t.Fatalf("%s 查询返回空 Schema 投影", name)
		}
		description, _ := leaf["description"].(string)
		if !strings.Contains(description, "管理员") {
			t.Errorf("%s 投影缺少管理员判定语义：%q", name, description)
		}
		parameters, _ := leaf["parameters"].(map[string]any)
		if len(parameters) != 0 {
			t.Errorf("%s 无业务入参工具投影出参数：%#v", name, parameters)
		}
		result, ok := leaf["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s 缺少 result 投影", name)
		}
		schema, _ := result["data_schema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		boolResult, _ := properties["result"].(map[string]any)
		if boolResult["type"] != "boolean" {
			t.Errorf("%s result 应为布尔：%#v", name, boolResult)
		}
		if text, _ := boolResult["description"].(string); !strings.Contains(text, "管理员") || !strings.Contains(text, "success=false") {
			t.Errorf("%s result 描述缺少三态语义：%q", name, text)
		}
		if _, exists := properties["success"]; !exists {
			t.Errorf("%s 缺少 success 字段投影", name)
		}
	}
	if full["description"] != compact["description"] || full["cli_path"] != compact["cli_path"] {
		t.Errorf("full 与 compact 投影不一致：\nfull=%#v\ncompact=%#v", full["description"], compact["description"])
	}
}
