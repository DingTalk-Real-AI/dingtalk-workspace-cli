// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeCardPreservesPublicCodexContext(t *testing.T) {
	for _, scenario := range []string{"new", "resume", "resume-failed"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			e.QuotedMessage = &personal.MessageEventContext{Content: "quoted fixture"}
			attachment := connectMediaAttachment{LocalPath: filepath.Join(r.dir, "image.png"), MediaType: "image"}
			f := &codexAppServerForwarder{sessions: newCodexThreadSessions(""), developerInstructions: codexEmployeeDeveloperInstructions}
			if scenario != "new" {
				f.sessions.setThreadID(codexManagedConversationKey(e.ConversationID), "thread")
			}
			w := &bufferWriteCloser{}
			thread := codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"thread"}}`)}
			if scenario == "resume-failed" {
				thread = codexRPCMessage{ID: codexIntPtr(2), Error: &codexRPCError{Code: -32600, Message: "thread busy"}}
			}
			client := unitCodexClient(w,
				codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)}, thread,
				codexRPCMessage{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"answer"}]}}`)},
			)
			testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return client, nil })
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				if args[0] == "send-a2ui-card" {
					return map[string]any{"bizId": "server-biz"}, nil
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			record := employeeTaskRecord{}
			if err := r.processCard(e, &record, f, attachment); err != nil {
				t.Fatal(err)
			}
			if scenario == "resume-failed" {
				if record.Execution != "failure" || record.Status != "card_incomplete" || f.sessions.threadID(codexManagedConversationKey(e.ConversationID)) != "thread" || strings.Contains(w.String(), `"method":"thread/start"`) {
					t.Fatal("卡片恢复失败不得重建会话或清除公共会话映射")
				}
				return
			}
			if record.Execution != "success" || record.Delivery != "accepted" || employeeTaskLabel(record) != "已完成" {
				t.Fatalf("卡片结果未衔接公共任务反馈: %+v", record)
			}
			var sawTurn bool
			for _, line := range strings.Split(strings.TrimSpace(w.String()), "\n") {
				var req struct {
					Method string `json:"method"`
					Params struct {
						Input                 []map[string]string          `json:"input"`
						DeveloperInstructions string                       `json:"developerInstructions"`
						AdditionalContext     map[string]map[string]string `json:"additionalContext"`
					} `json:"params"`
				}
				if err := json.Unmarshal([]byte(line), &req); err != nil {
					t.Fatal(err)
				}
				if req.Method == "thread/start" && (!strings.Contains(req.Params.DeveloperInstructions, codexEmployeeDeveloperInstructions) || !strings.Contains(req.Params.DeveloperInstructions, codexManagedReplyContext)) {
					t.Fatal("新卡片会话覆盖了员工工具授权策略或缺少投递归属")
				}
				if req.Method == "turn/start" {
					sawTurn = true
					policy := req.Params.AdditionalContext["dws.digital_employee_policy"]["value"]
					if !strings.Contains(policy, codexEmployeeDeveloperInstructions) || !strings.Contains(policy, codexManagedReplyContext) {
						t.Fatal("本轮应用上下文丢失公共员工策略或卡片投递归属")
					}
					if len(req.Params.Input) != 2 || req.Params.Input[0]["text"] != employeeImagePrompt(employeeTurnText(e), []connectMediaAttachment{attachment}) || req.Params.Input[1]["type"] != "localImage" || req.Params.Input[1]["path"] != attachment.LocalPath {
						t.Fatalf("卡片接入丢失引用上下文或图片附件: %+v", req.Params.Input)
					}
				}
			}
			if !sawTurn {
				t.Fatal("未执行真实 turn/start 协议路径")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardFeedbackStates(t *testing.T) {
	for _, test := range []struct{ status, delivery, label string }{
		{"card_finished", "accepted", "已完成"},
		{"card_incomplete", "accepted", "处理失败"},
		{"card_rejected", "rejected", "处理失败"},
		{"needs_review", "unknown", "发送待确认"},
	} {
		if got := employeeTaskLabel(employeeTaskRecord{Status: test.status, Delivery: test.delivery}); got != test.label {
			t.Fatalf("%s label=%s want=%s", test.status, got, test.label)
		}
	}
}
