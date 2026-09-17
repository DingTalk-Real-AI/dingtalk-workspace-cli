// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package unit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMultiOASkillProgressiveDisclosurePolicy(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	miscRoot := filepath.Join(root, "skills", "multi", "dingtalk-misc")

	read := func(relative string) string {
		t.Helper()
		path := filepath.Join(miscRoot, relative)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(content)
	}

	skill := read("SKILL.md")
	core := read(filepath.Join("references", "oa.md"))
	create := read(filepath.Join("references", "oa-create.md"))
	attachments := read(filepath.Join("references", "oa-attachments.md"))
	components := read(filepath.Join("references", "oa", "oa-form-components.md"))

	for _, required := range []string{"oa.md", "oa-create.md", "oa-attachments.md"} {
		if !strings.Contains(skill, required) {
			t.Errorf("misc SKILL missing OA progressive route %q", required)
		}
	}
	if len(core) > 16_000 {
		t.Fatalf("OA core reference is %d bytes; keep query/role routing under 16000 bytes", len(core))
	}
	for _, required := range []string{
		"角色来源是对象身份的一部分",
		"不得静默切换来源",
		"不得改查 `list-submitted` 或 `list-initiated`",
		"字段定义就是交付结果",
		"没有固定“最多两次查询”的机械限制",
		"[oa-create.md](oa-create.md)",
		"[oa-attachments.md](oa-attachments.md)",
	} {
		if !strings.Contains(core, required) {
			t.Errorf("OA core missing design guard %q", required)
		}
	}
	if strings.Contains(core, "## SKILL 摘要（原 dingtalk-oa/SKILL.md 正文）") {
		t.Error("OA core must not retain the duplicated legacy Skill summary")
	}
	for _, required := range []string{
		"不用真实写接口试探",
		"DDHolidayField",
		"不能套用 `DDDateRangeField`",
		"写后验证",
		"不可降级与单次写入",
		"对同一份确认摘要只调用一次 `create-instance`",
		"不得自行撤销重建",
		"不得删除整张明细或子字段",
		"简单模式的 `--approvers` / `--cc-list` 不能替代模板自选节点",
		"首次即停止",
		"默认在本地投影控件摘要",
		"不得用一次缺参调用来发现约束",
		"任一值变化后重新预测",
		"不能将失败归因于 API",
		"oa_create_preflight.py form-schema",
		"禁止临时编写 `jq`/Python 解析器",
		"常见 `targetSelect: true` 直接使用紧凑输出",
		"只有 `needsNodeReference=true`",
		"禁止按模板名称、`processCode`",
	} {
		if !strings.Contains(create, required) {
			t.Errorf("OA create reference missing safety rule %q", required)
		}
	}
	if !strings.Contains(components, "未定义稳定格式的控件") || !strings.Contains(components, "DDHolidayField") {
		t.Error("OA component reference must preserve the DDHolidayField unsupported-format boundary")
	}
	if !strings.Contains(attachments, "不要因为相邻能力成功就宣称目标能力成功") {
		t.Error("OA attachment reference must distinguish links, download authorization, and preview authorization")
	}
}
