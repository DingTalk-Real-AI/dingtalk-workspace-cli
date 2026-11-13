// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageEmployeeApprovalOfficialCard(t *testing.T) {
	protocol, err := aicardProtocol()
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Date(2026, 10, 19, 20, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	origin := &employeeApprovalOrigin{SenderName: "测试同事", SenderID: "coworker", ConversationID: "source-conversation", ConversationType: "群聊", MessageID: "source-message"}
	for _, tc := range []struct {
		name, status, method string
		allow                bool
		origin               *employeeApprovalOrigin
	}{
		{"command", "等待主管审批（仅批准本次请求）", "item/commandExecution/requestApproval", true, origin},
		{"file", "等待主管审批（仅批准本次请求）", "item/fileChange/requestApproval", true, origin},
		{"permissions", "等待主管审批（仅批准本次请求）", "item/permissions/requestApproval", true, origin},
		{"unsafe", "等待主管审批（仅批准本次请求）", "", false, nil},
		{"no-name", "等待主管审批（仅批准本次请求）", "", true, &employeeApprovalOrigin{SenderID: "coworker"}},
		{"name-only", "等待主管审批（仅批准本次请求）", "", true, &employeeApprovalOrigin{SenderName: "测试同事"}},
		{"approved", "主管已允许本次请求", "", false, origin},
		{"declined", "主管已拒绝", "", false, origin},
		{"expired", "审批已失效，未放行", "", false, origin},
		{"record-failed", "审批记录未能保存，未放行", "", false, origin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 长命令包含换行和 Markdown 围栏时，展示不能被截断或提前结束。
			summary := "命令执行\necho ```\n" + strings.Repeat("完整操作详情", 80)
			info := employeeApprovalCardInfo{Origin: tc.origin, Method: tc.method, ExpiresAt: expires}
			messages := []map[string]any{employeeCardMessage("createSurface", "approval-card", map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{}})}
			messages = append(messages, employeeApprovalMessages("approval-card", "approval-id", summary, tc.status, tc.allow, info)...)
			raw, err := json.Marshal(messages)
			if err != nil {
				t.Fatal(err)
			}
			lint, err := protocol.Lint(raw, false, true)
			if err != nil || !lint.Valid {
				t.Fatalf("卡片不符合协议：%+v %v", lint, err)
			}
			if _, err := protocol.Preflight(lint.A2UIMessages, "new-card"); err != nil {
				t.Fatal(err)
			}
			if dir := os.Getenv("DWS_APPROVAL_CARD_CASE_DIR"); dir != "" {
				if err := writeEmployeeJSON(filepath.Join(dir, tc.name+".json"), messages); err != nil {
					t.Fatal(err)
				}
			}
			components := map[string]map[string]any{}
			for _, c := range messages[1]["updateComponents"].(map[string]any)["components"].([]map[string]any) {
				components[c["id"].(string)] = c
			}
			panel := components["single-panel"]
			if panel["component"] != "Column" || panel["padding"] != 12 || panel["cornerRadius"] != 16 || panel["borderWidth"] != 0.5 || panel["backgroundColorToken"] != "common_fg_z1_color" {
				t.Fatal("没有使用新版单条审批的圆角面板")
			}
			if !reflect.DeepEqual(components["root"]["children"], []string{"intro", "single-panel"}) || components["root"]["gap"] != 10 {
				t.Fatal("没有使用新版单条审批布局")
			}
			wantScope := "仅批准本次请求"
			if tc.method == "item/permissions/requestApproval" {
				wantScope = "本轮有效，不保存为永久权限"
			}
			if !strings.Contains(components["deadline"]["text"].(string), "10-19 20:30:00") || components["scope-value"]["text"] != wantScope {
				t.Fatal("有效期或单次授权范围丢失")
			}
			if components["details"]["component"] != "Text" || components["details"]["text"] != summary || components["details"]["maxLine"] != 4096 {
				t.Fatal("未结构化的说明必须完整展示为字面文本")
			}
			if tc.origin != nil && !strings.Contains(components["single-title"]["text"].(string), "测试同事") && !strings.Contains(components["single-title"]["text"].(string), "coworker") {
				t.Fatal("卡片缺少实际发起人")
			}
			pending := tc.status == "等待主管审批（仅批准本次请求）"
			for _, action := range []string{"approve", "decline"} {
				button, exists := components[action]
				if exists != pending {
					t.Fatal("终态卡片保留了审批按钮")
				}
				if !pending {
					continue
				}
				if button["component"] != "Row" || button["cornerRadius"] != 24 || button["padding"] != 8 || button["backgroundColorToken"] != "common_bg_color" {
					t.Fatal("没有使用新版圆角操作区")
				}
				if action == "approve" && !tc.allow {
					if _, clickable := button["action"]; clickable || components["approve-label"]["colorToken"] != "common_level2_base_color" {
						t.Fatal("不可安全展示的操作仍可批准")
					}
					continue
				}
				event := button["action"].(map[string]any)["event"].(map[string]any)
				want := map[string]any{"approvalId": "approval-id", "decision": action}
				if event["name"] != "dws_employee_approval" || !reflect.DeepEqual(event["context"], want) {
					t.Fatal("按钮没有接入实际授权请求")
				}
			}
			if pending && (components["single-actions"]["gap"] != 10 || !reflect.DeepEqual(components["single-actions"]["children"], []string{"decline", "approve"})) {
				t.Fatal("没有沿用新版底部并排操作区")
			}
			if pending && tc.method == "item/permissions/requestApproval" && components["approve-label"]["text"] != "仅本轮允许" {
				t.Fatal("权限申请按钮未明确本轮范围")
			}
			if !pending && (components["single-result"]["cornerRadius"] != 24 || components["single-status"]["text"] != tc.status) {
				t.Fatal("终态没有使用新版结果展示")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeApprovalSeparateFields(t *testing.T) {
	protocol, err := aicardProtocol()
	if err != nil {
		t.Fatal(err)
	}
	command := "printf '%s\\n' '申请理由：命令中的原文\n工作目录：仍属于指令\n```'"
	reason := "保留 <原文> 与 **标记**\n" + strings.Repeat("完整理由\n", 40)
	for _, tc := range []struct {
		name, method string
		params       map[string]any
		codeKey      string
	}{
		{"command", "item/commandExecution/requestApproval", map[string]any{"command": command, "cwd": "/workspace/有 空格", "reason": reason, "grantRoot": "/workspace", "networkApprovalContext": map[string]any{"host": "example.test"}}, "command"},
		{"file", "item/fileChange/requestApproval", map[string]any{"_dwsFileChanges": []any{map[string]any{"path": "file.txt", "diff": "+ 内容 ```"}}, "reason": reason, "grantRoot": "/workspace"}, "_dwsFileChanges"},
		{"permissions", "item/permissions/requestApproval", map[string]any{"permissions": map[string]any{"fileSystem": "read"}, "additionalPermissions": []string{"network"}, "cwd": "/workspace", "reason": reason}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.params)
			summary, fields, safe := employeeApprovalDetails(tc.method, raw)
			if !safe || len(fields) != len(tc.params) {
				t.Fatal("展示分组丢失了待授权字段")
			}
			for _, status := range []string{"等待主管审批（仅批准本次请求）", "主管已允许本次请求"} {
				info := employeeApprovalCardInfo{Method: tc.method, Fields: fields, Origin: &employeeApprovalOrigin{SenderID: "coworker", ConversationID: "source", MessageID: "message"}}
				messages := []map[string]any{employeeCardMessage("createSurface", "approval-fields", map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{}})}
				messages = append(messages, employeeApprovalMessages("approval-fields", "approval-id", summary, status, status != "主管已允许本次请求", info)...)
				encoded, _ := json.Marshal(messages)
				lint, err := protocol.Lint(encoded, false, true)
				if err != nil || !lint.Valid {
					t.Fatalf("字段布局不符合协议：%+v %v", lint, err)
				}
				if _, err := protocol.Preflight(lint.A2UIMessages, "new-card"); err != nil {
					t.Fatal(err)
				}
				components := map[string]map[string]any{}
				for _, component := range messages[1]["updateComponents"].(map[string]any)["components"].([]map[string]any) {
					components[component["id"].(string)] = component
				}
				codeCount := 0
				for key, original := range tc.params {
					want, ok := original.(string)
					if !ok {
						data, _ := json.MarshalIndent(original, "", "  ")
						want = string(data)
					}
					value := components["field-"+key+"-value"]
					if key == tc.codeKey {
						codeCount++
						fence := "```"
						if strings.Contains(want, fence) {
							fence = "````"
						}
						if value["component"] != "Markdown" || value["content"] != fence+"text\n"+want+"\n"+fence {
							t.Fatal("操作代码块混入其他字段、指令被改写或围栏提前结束")
						}
					} else if value["component"] != "Text" || value["text"] != want || value["maxLine"] != 4096 {
						t.Fatalf("字段 %s 没有独立、完整地按字面展示", key)
					}
				}
				for id, c := range components {
					if c["component"] == "Markdown" && id != "intro" {
						codeCount--
					}
				}
				if codeCount != 0 || components["source"]["component"] != "Text" || !strings.Contains(components["source"]["text"].(string), "source") {
					t.Fatal("说明或来源进入代码块")
				}
				if dir := os.Getenv("DWS_APPROVAL_CARD_CASE_DIR"); dir != "" {
					if err := writeEmployeeJSON(filepath.Join(dir, tc.name+"-fields-"+status+".json"), messages); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
	for _, raw := range []string{`{"command":"echo test","reason":"token=private-value"}`, `{"command":"echo test","cwd":"/workspace/password=private-value"}`} {
		summary, fields, safe := employeeApprovalDetails("item/commandExecution/requestApproval", json.RawMessage(raw))
		if safe || len(fields) != 0 || strings.Contains(summary, "private-value") {
			t.Fatal("布局拆分绕过敏感信息校验或保留了可泄露字段")
		}
	}
}
