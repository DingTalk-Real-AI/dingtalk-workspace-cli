// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageEmployeeCardTemplateTimeline(t *testing.T) {
	v := employeeCardView{}
	v.accept(agentProgress{ID: "progress", Kind: "progress", Text: "正在核对资料"})
	v.accept(agentProgress{ID: "read", Kind: "tool", Text: "文件读取", Status: "running"})
	surface := newEmployeeCardTestSurface()
	surface.apply(v.messages("template", "处理中…", true))
	timeline := surface.components["executionTimeline"]
	if timeline["component"] != "Column" || len(timeline["children"].([]string)) != 2 {
		t.Fatal("模板时间线未保留公开进度和工具分组")
	}
	group := surface.components["tool-group-step-1"]
	item := surface.components["tool-item-step-1"]
	if group["variant"] != "indented" || item["variant"] != "indented" || item["maxHeight"] != 240 || item["icon"] != "Folder_L_outlined" {
		t.Fatal("工具没有使用模板的分组、可展开详情和类型图标")
	}
	if !strings.Contains(group["icon"].(string), "loading_circular_light") || !strings.Contains(group["darkIcon"].(string), "loading_circular_dark") {
		t.Fatal("执行中的工具缺少明暗主题加载图标")
	}
	if !strings.Contains(surface.components["step-1"]["text"].(string), "执行中") {
		t.Fatal("展开工具后看不到实际状态")
	}
	v.accept(agentProgress{ID: "read", Kind: "tool", Text: "文件读取", Status: "completed", Done: true})
	v.answer = "真实结果"
	surface.apply(v.messages("template", "处理完成", false))
	group = surface.components["tool-group-step-1"]
	if group["icon"] != "Check_L_outlined" || group["darkIcon"] != nil {
		t.Fatal("完成后没有清除加载图标并替换为成功标记")
	}
	if surface.components["execution-complete"]["defaultExpanded"] != false || surface.components["execution-complete"]["textEffect"] != nil {
		t.Fatal("完成面板没有收起或仍显示生成效果")
	}
	if surface.text("result") != "真实结果" || strings.Contains(surface.text("execution-complete"), "真实结果") {
		t.Fatal("最终正文未放入模板的独立结果区域")
	}
}

func TestCrossPlatformCoverageEmployeeCardTemplateTerminalStates(t *testing.T) {
	for _, status := range []string{"running", "completed", "failed", "cancelled", "ended", "unknown", ""} {
		t.Run(status, func(t *testing.T) {
			v := employeeCardView{}
			v.accept(agentProgress{ID: "tool", Kind: "tool", Text: "网页检索", Status: status, Done: status == ""})
			surface := newEmployeeCardTestSurface()
			messages := v.messages("template", "任务已结束", false)
			surface.apply(messages)
			group := surface.components["tool-group-step-0"]
			if group == nil {
				t.Fatal("缺少工具分组")
			}
			if strings.Contains(jsonScalar(group["icon"]), "loading") || group["darkIcon"] != nil {
				t.Fatal("终态仍显示工具加载动画")
			}
			if (group["icon"] == "Check_L_outlined") != (status == "completed") {
				t.Fatal("失败、取消或缺少结果的工具不能显示成功标记")
			}
			protocol, err := aicardProtocol()
			if err != nil {
				t.Fatal(err)
			}
			initial := []map[string]any{employeeCardMessage("createSurface", "template", map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{"answer": ""}})}
			initial = append(initial, messages...)
			encoded, _ := json.Marshal(initial)
			report, err := protocol.Lint(encoded, false, false)
			if err != nil || !report.Valid {
				t.Fatalf("模板协议无效：%+v %v", report, err)
			}
			var history []string
			for _, message := range initial {
				b, _ := json.Marshal(message)
				history = append(history, string(b))
			}
			preflight, err := protocol.Preflight(history, "new-card")
			if err != nil || preflight["valid"] != true {
				t.Fatalf("模板组件引用无效：%+v %v", preflight, err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardTemplateToolIcons(t *testing.T) {
	for label, want := range map[string]string{
		"文件读取": "Folder_L_outlined", "文件修改": "Edit_L_outlined", "文件搜索": "Search_L_outlined",
		"网页搜索": "Language_L_outlined", "网页检索": "Language_L_outlined", "命令执行": "CodeProgram_L_outlined",
		"MCP 工具调用": "CodeProgram_L_outlined", "": "CodeProgram_L_outlined",
	} {
		t.Run(label, func(t *testing.T) {
			components := employeeCardToolComponents("tool", agentProgress{Text: label}, true)
			if components[1]["icon"] != want || components[1]["title"] == "" || components[2]["text"] == "" {
				t.Fatal("工具类型图标或可展开状态丢失")
			}
		})
	}
}
