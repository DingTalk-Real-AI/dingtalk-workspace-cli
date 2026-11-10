// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"strings"
	"time"
)

type employeeApprovalCardInfo struct {
	Origin    *employeeApprovalOrigin
	Method    string
	ExpiresAt time.Time
	Fields    []employeeApprovalCardField
}

type employeeApprovalCardField struct {
	Key, Label, Value string
	Code              bool
}

// 沿用升级版示例的单条圆角面板与胶囊操作区；审批仍逐条关联真实请求。
func employeeApprovalMessages(surface, id, summary, status string, allow bool, metadata ...employeeApprovalCardInfo) []map[string]any {
	info := employeeApprovalCardInfo{}
	if len(metadata) != 0 {
		info = metadata[0]
	}
	name := "数字员工"
	if origin := info.Origin; origin != nil {
		if displayName := strings.TrimSpace(origin.SenderName); displayName != "" {
			name = displayName
			if origin.SenderID != "" {
				name += "（" + origin.SenderID + "）"
			}
		} else if origin.SenderID != "" {
			name = origin.SenderID
		}
	}
	kind := map[string]string{"item/commandExecution/requestApproval": "命令执行", "item/fileChange/requestApproval": "文件修改", "item/permissions/requestApproval": "权限申请"}[info.Method]
	if kind == "" {
		kind = "操作授权"
	}
	scope := "仅批准本次请求"
	approveLabel := "仅本次允许"
	if info.Method == "item/permissions/requestApproval" {
		scope = "本轮有效，不保存为永久权限"
		approveLabel = "仅本轮允许"
	}
	components := []map[string]any{
		{"id": "intro", "component": "Markdown", "content": "数字员工操作审批"},
		{"id": "single-title", "component": "Text", "text": name + " · " + kind, "maxLine": 20, "weight": 1, "bold": true, "colorToken": "common_level1_base_color", "sizeToken": "common_body_text_style__font_size"},
		{"id": "scope-value", "component": "Text", "text": scope, "maxLine": 30, "colorToken": "common_level2_base_color", "sizeToken": "common_body_text_style__font_size"},
	}
	header := []string{"single-title"}
	body := []string{"single-header", "scope-value"}
	if !info.ExpiresAt.IsZero() {
		components = append(components, map[string]any{"id": "deadline", "component": "Text", "text": "有效期至 " + info.ExpiresAt.Format("01-02 15:04:05"), "maxLine": 30, "colorToken": "common_level2_base_color", "sizeToken": "common_body_text_style__font_size"})
		body = append(body, "deadline")
	}
	details, detailIDs := employeeApprovalContentComponents(summary, info)
	components = append(components, details...)
	body = append(body, detailIDs...)
	if status == "等待主管审批（仅批准本次请求）" {
		components = append(components, map[string]any{"id": "single-recommendation", "component": "Text", "text": "待你确认", "maxLine": 1, "bold": true, "colorToken": "common_link_color", "sizeToken": "common_footnote_text_style__font_size"})
		header = append(header, "single-recommendation")
		for _, action := range []string{"decline", "approve"} {
			label, color := "拒绝", "common_level1_base_color"
			if action == "approve" {
				label, color = approveLabel, "common_link_color"
				if !allow {
					color = "common_level2_base_color"
				}
			}
			control := map[string]any{"id": action, "component": "Row", "children": []string{action + "-label"}, "weight": 1, "justify": "center", "align": "center", "padding": 8, "cornerRadius": 24, "backgroundColorToken": "common_bg_color"}
			if action == "decline" || allow {
				control["action"] = map[string]any{"event": map[string]any{"name": "dws_employee_approval", "context": map[string]any{"approvalId": id, "decision": action}}}
			}
			components = append(components,
				map[string]any{"id": action + "-label", "component": "Text", "text": label, "maxLine": 1, "colorToken": color, "sizeToken": "common_body_text_style__font_size"}, control)
		}
		components = append(components, map[string]any{"id": "single-actions", "component": "Row", "children": []string{"decline", "approve"}, "gap": 10, "align": "center"})
		body = append(body, "single-actions")
	} else {
		components = append(components,
			map[string]any{"id": "single-status", "component": "Text", "text": status, "maxLine": 30, "colorToken": "common_level1_base_color", "sizeToken": "common_body_text_style__font_size"},
			map[string]any{"id": "single-result", "component": "Row", "children": []string{"single-status"}, "weight": 1, "justify": "center", "align": "center", "padding": 8, "cornerRadius": 24, "backgroundColorToken": "common_bg_color"},
			map[string]any{"id": "single-result-row", "component": "Row", "children": []string{"single-result"}, "gap": 0, "align": "center"})
		body = append(body, "single-result-row")
	}
	components = append(components,
		map[string]any{"id": "single-header", "component": "Row", "children": header, "gap": 8, "align": "center"},
		map[string]any{"id": "single-item", "component": "Column", "children": body, "gap": 16, "align": "stretch"},
		map[string]any{"id": "single-panel", "component": "Column", "children": []string{"single-item"}, "padding": 12, "cornerRadius": 16, "borderWidth": 0.5, "borderColorToken": "common_line_light_color", "backgroundColorToken": "common_fg_z1_color", "gap": 0, "align": "stretch"},
		map[string]any{"id": "root", "component": "Column", "children": []string{"intro", "single-panel"}, "gap": 10, "align": "stretch"})
	return []map[string]any{employeeCardMessage("updateComponents", surface, map[string]any{"components": components})}
}

func employeeApprovalContentComponents(summary string, info employeeApprovalCardInfo) ([]map[string]any, []string) {
	var components []map[string]any
	var children []string
	// 来源和说明使用字面文本，避免被混进待审核指令或解释为 Markdown。
	if source := employeeApprovalSource(info.Origin); source != "" {
		components = append(components, map[string]any{"id": "source", "component": "Text", "text": strings.TrimSpace(source), "maxLine": 4096, "colorToken": "common_level2_base_color"})
		children = append(children, "source")
	}
	if len(info.Fields) == 0 {
		components = append(components, map[string]any{"id": "details", "component": "Text", "text": summary, "maxLine": 4096})
		return components, append(children, "details")
	}
	for _, field := range info.Fields {
		id := "field-" + field.Key
		value := map[string]any{"id": id + "-value", "component": "Text", "text": field.Value, "maxLine": 4096}
		if field.Code {
			fence := "```"
			for strings.Contains(field.Value, fence) {
				fence += "`"
			}
			value = map[string]any{"id": id + "-value", "component": "Markdown", "content": fence + "text\n" + field.Value + "\n" + fence}
		}
		components = append(components,
			map[string]any{"id": id + "-label", "component": "Text", "text": field.Label, "maxLine": 2, "colorToken": "common_level2_base_color"},
			value,
			map[string]any{"id": id, "component": "Column", "children": []string{id + "-label", id + "-value"}, "gap": 4, "align": "stretch"})
		children = append(children, id)
	}
	return components, children
}
