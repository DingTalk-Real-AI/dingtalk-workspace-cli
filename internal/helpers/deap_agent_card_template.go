// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// 采用 agent-run-full.json 的时间线、工具分组和明暗主题加载图标。
// 模板示例里的指令、结果和附件由本轮公开进度替换，不读取工具原始输入输出。
const (
	employeeCardLoadingLight = "https://g.alicdn.com/dingding/card_a2ui/0.1.1/img/loading-circular/loading_circular_light.DlX94S3.gif"
	employeeCardLoadingDark  = "https://g.alicdn.com/dingding/card_a2ui/0.1.1/img/loading-circular/loading_circular_dark.D-pS11K.gif"
)

func employeeCardProgressText(id, text string) map[string]any {
	return map[string]any{"id": id, "component": "Text", "text": text, "colorToken": "common_level3_base_color", "sizeToken": "common_action_text_style__font_size", "maxLine": 256}
}

func employeeCardToolComponents(id string, p agentProgress, running bool) []map[string]any {
	label := p.Text
	if label == "" {
		label = "工具调用"
	}
	status := p.Status
	if status == "" {
		status = "running"
		if p.Done {
			status = "ended"
		}
	}
	// 回合终止时没有成功回执的工具只能标记结束，不能继续转圈或假报成功。
	if !running && status == "running" {
		status = "ended"
	}
	statusText := map[string]string{"running": "执行中…", "completed": "已完成", "failed": "执行失败", "cancelled": "已取消", "ended": "已结束"}[status]
	if statusText == "" {
		statusText = "已结束"
	}
	group := map[string]any{"id": "tool-group-" + id, "component": "CollapsiblePanel", "title": label + " · " + statusText, "children": []string{"tool-item-" + id}, "colorToken": "common_level2_base_color", "variant": "indented", "fallbackMarkdown": label + " · " + statusText, "defaultExpanded": false}
	switch status {
	case "running":
		group["icon"], group["darkIcon"] = employeeCardLoadingLight, employeeCardLoadingDark
	case "completed":
		group["icon"] = "Check_L_outlined"
	case "failed":
		group["icon"] = "Error_L_outlined"
	case "cancelled":
		group["icon"] = "Close_L_outlined"
	}
	icon := map[string]string{"文件读取": "Folder_L_outlined", "文件修改": "Edit_L_outlined", "文件搜索": "Search_L_outlined", "网页搜索": "Language_L_outlined", "网页检索": "Language_L_outlined", "命令执行": "CodeProgram_L_outlined"}[label]
	if icon == "" {
		icon = "CodeProgram_L_outlined"
	}
	item := map[string]any{"id": "tool-item-" + id, "component": "CollapsiblePanel", "title": label, "children": []string{id}, "colorToken": "common_level2_base_color", "variant": "indented", "fallbackMarkdown": label, "defaultExpanded": false, "icon": icon, "maxHeight": 240}
	return []map[string]any{group, item, employeeCardProgressText(id, label+" · "+statusText)}
}
