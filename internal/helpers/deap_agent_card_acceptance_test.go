// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeCardAcceptanceExpansion(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	surface := newEmployeeCardTestSurface()
	expanded := map[string]bool{}
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		surface.apply(messages)
		for id, component := range surface.components {
			if component["component"] == "CollapsiblePanel" {
				if _, exists := expanded[id]; !exists {
					expanded[id], _ = component["defaultExpanded"].(bool)
				}
			}
		}
		children := surface.components["root"]["children"].([]string)
		panelID := children[0]
		if args[0] == "send-a2ui-card" {
			if !expanded[panelID] {
				t.Error("C2: 思考中面板首次显示没有展开")
			}
			// 渲染端把用户手动展开保存在组件 ID 上，更新 defaultExpanded 不会重置它。
			expanded[panelID] = true
			return map[string]any{"bizId": "acceptance-card"}, nil
		}
		if args[4] == "FINISH" && expanded[panelID] {
			t.Error("C2: 完成后仍保留用户手动展开的状态")
		}
		return map[string]any{"flowStatus": args[4]}, nil
	})
	record := employeeTaskRecord{}
	fwd := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		emit(agentProgress{ID: "read", Kind: "tool", Text: "文件读取", Status: "completed"})
		return "验收完成", nil
	}}
	if err := r.processCard(e, &record, fwd); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeCardAcceptanceProgressColor(t *testing.T) {
	v := employeeCardView{}
	v.accept(agentProgress{ID: "public", Kind: "progress", Text: "正在核对数据"})
	v.accept(agentProgress{ID: "read", Kind: "tool", Text: "文件读取", Status: "running"})
	v.answer = "最终答案"
	components := v.messages("surface", "处理中…", true)[1]["updateComponents"].(map[string]any)["components"].([]map[string]any)
	for _, component := range components {
		if strings.HasPrefix(component["id"].(string), "step-") {
			if component["component"] != "Text" || component["colorToken"] != "common_level3_base_color" {
				t.Errorf("C2: 过程内容未使用浅色文本: %v", component)
			}
		}
	}
}
