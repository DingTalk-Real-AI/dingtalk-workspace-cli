// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeApprovalCards(t *testing.T) {
	for _, scenario := range []string{"approve", "decline", "expired", "unsafe", "revoked", "create-failure", "missing-biz", "ledger-create", "ledger-pending", "ledger-decision", "update-failure"} {
		t.Run(scenario, func(t *testing.T) {
			r, _ := employeeLedgerFixture(t)
			r.cfg.Binding = digitalEmployeeBinding{SchemaVersion: 1, DWSProfile: "corp:employee", AgentUUID: "agent", Channel: "codex", OperatorOpenDingTalkID: "owner", BindingState: "bound", DesiredState: "running", BindingRevision: 1}
			testseam.Swap(t, &deapConnectConfigDir, func() string { return r.dir })
			if err := saveDigitalEmployeeBinding(r.dir, r.cfg.Binding); err != nil {
				t.Fatal(err)
			}
			runtimeCtx, stopRuntime := context.WithTimeout(context.Background(), 3*time.Second)
			defer stopRuntime()
			r.ctx = runtimeCtx
			// 回合结束不等于员工退出；超时后仍需使用员工上下文更新原卡。
			ctx, cancel := context.WithCancel(r.turnContext(employeeEvent{SenderName: "发起同事", SenderID: "coworker", ConversationID: "source-conversation", MessageID: "source-message", Type: "user_im_message_receive_at"}))
			defer cancel()
			if codexApprovalSink(r.turnContext()) == nil {
				t.Fatal("ask mode missing handler")
			}
			r.cfg.Options.Yolo = true
			if codexApprovalSink(r.turnContext()) == nil {
				t.Fatal("manual approval must remain available after changing conversation permissions")
			}
			r.cfg.Options.Yolo = false
			if scenario == "ledger-create" {
				if err := os.WriteFile(filepath.Join(r.dir, "approvals"), []byte("block"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var failPersistence atomic.Bool
			testseam.Swap(t, &atomicRename, func(src, dst string) error {
				if failPersistence.Load() {
					return errors.New("unavailable")
				}
				return os.Rename(src, dst)
			})
			created := make(chan string, 1)
			calls := 0
			testseam.Swap(t, &employeeA2UICall, func(callCtx context.Context, b digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if b.DWSProfile != "corp:employee" {
					t.Error("wrong identity")
				}
				if args[0] == "send-a2ui-card" {
					data, _ := json.Marshal(messages)
					for _, value := range []string{"发起同事", "coworker", "source-conversation", "source-message", "群聊"} {
						if !strings.Contains(string(data), value) {
							t.Errorf("审批卡缺少事件来源 %s", value)
						}
					}
					if strings.Join(args, " ") != "send-a2ui-card --open-dingtalk-id owner" {
						t.Error("approval must go to supervisor")
					}
					var id string
					components := messages[1]["updateComponents"].(map[string]any)["components"].([]map[string]any)
					for _, c := range components {
						if c["id"] == "decline" {
							id = c["action"].(map[string]any)["event"].(map[string]any)["context"].(map[string]any)["approvalId"].(string)
						}
					}
					if scenario == "approve" {
						values := map[string]map[string]any{}
						for _, c := range components {
							values[c["id"].(string)] = c
						}
						if values["field-command-value"]["content"] != "```text\necho harmless\n```" || values["field-cwd-value"]["text"] != "/workspace" || values["field-reason-value"]["text"] != "独立显示的申请理由" {
							t.Error("代码块应只包含原始指令，目录和理由必须单独展示")
						}
					}
					if id == "" {
						t.Error("missing callback correlation")
					}
					if scenario == "create-failure" {
						return nil, errors.New("unknown")
					}
					if scenario == "missing-biz" {
						return map[string]any{}, nil
					}
					if scenario == "ledger-pending" {
						failPersistence.Store(true)
					}
					created <- id
					return map[string]any{"bizId": "biz"}, nil
				}
				if scenario == "expired" && callCtx.Err() != nil {
					t.Error("审批超时不应取消仍在运行的员工更新原卡")
				}
				data, _ := json.Marshal(messages)
				if strings.Contains(string(data), `"dws_employee_approval"`) {
					t.Error("terminal card retains active button")
				}
				if scenario == "update-failure" {
					return nil, errors.New("unconfirmed")
				}
				return map[string]any{"flowStatus": "FINISH"}, nil
			})
			done := make(chan bool, 1)
			raw := json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"item","command":"echo harmless","cwd":"/workspace","reason":"独立显示的申请理由"}`)
			if scenario == "unsafe" {
				raw = json.RawMessage(`{"command":"tool --token secret"}`)
			}
			go func() { done <- r.requestApproval(ctx, "item/commandExecution/requestApproval", raw) }()
			if scenario != "ledger-create" && scenario != "create-failure" && scenario != "missing-biz" {
				id := <-created
				if scenario != "ledger-pending" {
					deadline := time.Now().Add(time.Second)
					for {
						data, _ := os.ReadFile(filepath.Join(r.dir, "approvals", id+".json"))
						var rec employeeApprovalRecord
						_ = json.Unmarshal(data, &rec)
						if rec.Status == "pending" {
							if rec.Origin == nil || rec.Origin.SenderID != "coworker" || rec.Origin.ConversationID != "source-conversation" {
								t.Fatal("审批记录没有保留实际发起人和会话")
							}
							break
						}
						if time.Now().After(deadline) {
							t.Fatal("pending not durable")
						}
						time.Sleep(time.Millisecond)
					}
					select {
					case <-done:
						t.Fatal("executed without decision")
					default:
					}
					if scenario == "expired" {
						cancel()
					} else {
						if scenario == "revoked" {
							b := r.cfg.Binding
							b.BindingRevision++
							if err := saveDigitalEmployeeBinding(r.dir, b); err != nil {
								t.Fatal(err)
							}
						}
						if scenario == "ledger-decision" {
							failPersistence.Store(true)
						}
						// 未授权操作者、错误卡片及无效选择不能放行。
						testseam.Swap(t, &employeeApprovalOperator, func(context.Context, digitalEmployeeBinding, string) (string, error) { return "outsider", nil })
						callback := func(biz, decision string) []byte {
							return []byte(fmt.Sprintf(`{"type":"user_card_action_triggered","payload":{"body":{"actionData":{"name":"dws_employee_approval","surfaceId":%q,"sourceComponentId":%q,"context":{"approvalId":%q,"decision":%q}},"bizInfoDTO":{"bizId":%q},"operatorDTO":{"uid":443102}}}}`, "approval-"+id, decision, id, decision, biz))
						}
						for _, line := range [][]byte{callback("wrong", "approve"), callback("biz", "invalid"), callback("biz", "approve")} {
							if !r.handleApprovalEvent(line) {
								t.Fatal("callback entered task queue")
							}
						}
						r.wg.Wait()
						select {
						case <-done:
							t.Fatal("unauthorized callback decided request")
						default:
						}
						employeeApprovalOperator = func(context.Context, digitalEmployeeBinding, string) (string, error) { return "owner", nil }
						decision := "approve"
						if scenario == "decline" {
							decision = "decline"
						}
						r.handleApprovalEvent(callback("biz", decision))
						r.wg.Wait()
					}
				}
			}
			allow := <-done
			want := scenario == "approve" || scenario == "update-failure"
			if allow != want {
				t.Fatalf("allow=%v want=%v", allow, want)
			}
			r.wg.Wait()
			if len(r.approvals) != 0 {
				t.Fatal("pending request leaked")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeApprovalBoundaries(t *testing.T) {
	for _, raw := range []string{`!`, `{}`, `{"command":"x --token=secret"}`, `{"reason":"Bearer secret"}`, `{"command":"` + strings.Repeat("x", 5000) + `"}`} {
		if _, safe := employeeApprovalSummary("item/commandExecution/requestApproval", json.RawMessage(raw)); safe {
			t.Fatal("unsafe preview approvable")
		}
	}
	if _, safe := employeeApprovalSummary("item/fileChange/requestApproval", json.RawMessage(`{"grantRoot":"/tmp"}`)); safe {
		t.Fatal("missing file diff approvable")
	}
	r, _ := employeeLedgerFixture(t)
	if r.handleApprovalEvent([]byte(`!`)) || r.handleApprovalEvent([]byte(`{"type":"message"}`)) {
		t.Fatal("not a callback")
	}
	// 原生操作者解析失败、空 token 均须保守拒绝。
	for _, token := range []*auth.TokenData{nil, {AccessToken: "employee-token"}} {
		testseam.Swap(t, &employeeSelfIdentityToken, func(context.Context, string) (*auth.TokenData, error) { return token, errors.New("unavailable") })
		if _, err := employeeApprovalOperator(context.Background(), digitalEmployeeBinding{}, "443102"); err == nil {
			t.Fatal("identity failure accepted")
		}
	}
}

func TestCrossPlatformCoverageEmployeeApprovalSchemaAndOperator(t *testing.T) {
	protocol, err := aicardProtocol()
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"等待主管审批（仅批准本次请求）", "主管已允许本次请求", "审批已失效，未放行"} {
		messages := []map[string]any{employeeCardMessage("createSurface", "surface", map[string]any{"catalogId": employeeCardCatalog, "dataModel": map[string]any{}})}
		messages = append(messages, employeeApprovalMessages("surface", "approval-id", "无害测试：echo hello", state, true)...)
		data, _ := json.Marshal(messages)
		report, err := protocol.Lint(data, false, true)
		if err != nil || !report.Valid {
			t.Fatalf("invalid approval card: %+v %v", report, err)
		}
		if _, err := protocol.Preflight(report.A2UIMessages, "new-card"); err != nil {
			t.Fatal(err)
		}
	}
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"contact/search_contact_by_key_word": {`{"result":[{"userId":"443102","openDingTalkId":"owner"}]}`}}}
	InitDepsForTest(t, caller)
	testseam.Swap(t, &employeeSelfIdentityToken, func(context.Context, string) (*auth.TokenData, error) {
		return &auth.TokenData{AccessToken: "employee-token"}, nil
	})
	got, err := employeeApprovalOperator(context.Background(), digitalEmployeeBinding{DWSProfile: "corp:employee"}, "443102")
	if err != nil || got != "owner" {
		t.Fatalf("wrong scoped operator: %q %v", got, err)
	}
}

func TestCrossPlatformCoverageEmployeeApprovalStaleCallback(t *testing.T) {
	r, _ := employeeLedgerFixture(t)
	r.cfg.Binding.OperatorOpenDingTalkID = "owner"
	r.approvals = map[string]*employeeApproval{"id": {BizID: "biz", decision: make(chan bool, 1)}}
	testseam.Swap(t, &employeeApprovalOperator, func(context.Context, digitalEmployeeBinding, string) (string, error) {
		r.mu.Lock()
		delete(r.approvals, "id")
		r.mu.Unlock()
		return "owner", nil
	})
	line := []byte(`{"type":"user_card_action_triggered","payload":{"body":{"actionData":{"name":"dws_employee_approval","surfaceId":"approval-id","sourceComponentId":"approve","context":{"approvalId":"id","decision":"approve"}},"bizInfoDTO":{"bizId":"biz"},"operatorDTO":{"uid":443102}}}}`)
	if !r.handleApprovalEvent(line) {
		t.Fatal("callback not consumed")
	}
	r.wg.Wait()
}

// 根据真实 A2UI 回调结构构造脱敏用例，操作者必须来自信封，不信任 context 的自报身份。
func TestCrossPlatformCoverageEmployeeApprovalA2UIEvent(t *testing.T) {
	for _, scenario := range []string{"approve", "decline", "wrong-actor", "context-spoof", "version", "name", "surface", "component", "unknown-id", "wrong-biz", "missing-actor"} {
		t.Run(scenario, func(t *testing.T) {
			r, _ := employeeLedgerFixture(t)
			r.cfg.Binding.OperatorOpenDingTalkID = "owner"
			decision := "approve"
			if scenario == "decline" {
				decision = "decline"
			}
			pending := &employeeApproval{BizID: "biz", decision: make(chan bool, 1)}
			r.approvals = map[string]*employeeApproval{"id": pending}
			actor := "owner"
			if scenario == "wrong-actor" || scenario == "context-spoof" {
				actor = "outsider"
			}
			if scenario == "missing-actor" {
				actor = ""
			}
			action := map[string]any{"name": "dws_employee_approval", "surfaceId": "approval-id", "sourceComponentId": decision, "context": map[string]any{"approvalId": "id", "decision": decision, "openDingTalkId": "employee-self", "corpId": "corp"}}
			version := "1.0"
			biz := "biz"
			switch scenario {
			case "version":
				version = "2.0"
			case "name":
				action["name"] = "other"
			case "surface":
				action["surfaceId"] = "other"
			case "component":
				action["sourceComponentId"] = "other"
			case "unknown-id":
				action["context"].(map[string]any)["approvalId"] = "other"
			case "wrong-biz":
				biz = "other"
			case "context-spoof":
				action["context"].(map[string]any)["openDingTalkId"] = "owner"
			}
			event := map[string]any{"type": "user_card_action_triggered", "payload": map[string]any{"body": map[string]any{"a2uiEvent": map[string]any{"version": version, "action": action}, "operatorDTO": map[string]any{"openDingTalkId": actor}, "bizInfoDTO": map[string]any{"bizId": biz}}}}
			testseam.Swap(t, &employeeApprovalOperator, func(context.Context, digitalEmployeeBinding, string) (string, error) {
				t.Error("A2UI should use envelope open ID")
				return "", errors.New("not used")
			})
			data, _ := json.Marshal(event)
			if !r.handleApprovalEvent(data) {
				t.Fatal("A2UI event became a model message")
			}
			r.wg.Wait()
			select {
			case got := <-pending.decision:
				if scenario != "approve" && scenario != "decline" {
					t.Fatal("invalid callback approved")
				}
				if got != (scenario == "approve") {
					t.Fatal("wrong decision")
				}
			default:
				if scenario == "approve" || scenario == "decline" {
					t.Fatal("real A2UI callback ignored")
				}
			}
		})
	}
	r, _ := employeeLedgerFixture(t)
	r.cfg.Binding.OperatorOpenDingTalkID = "owner"
	r.approvals = map[string]*employeeApproval{"id": {BizID: "biz", decision: make(chan bool, 1)}}
	testseam.Swap(t, &employeeApprovalOperator, func(context.Context, digitalEmployeeBinding, string) (string, error) {
		return "", errors.New("unavailable")
	})
	r.handleApprovalEvent([]byte(`{"type":"user_card_action_triggered","payload":{"body":{"actionData":{"name":"dws_employee_approval","surfaceId":"approval-id","sourceComponentId":"approve","context":{"approvalId":"id","decision":"approve"}},"bizInfoDTO":{"bizId":"biz"},"operatorDTO":{"uid":443102}}}}`))
	r.wg.Wait()
	summary, fields, safe := employeeApprovalDetails("item/commandExecution/requestApproval", json.RawMessage("{\"command\":\"echo ``` nested\"}"))
	if !safe {
		t.Fatal("无敏感信息的完整指令被拒绝")
	}
	data, _ := json.Marshal(employeeApprovalMessages("surface", "id", summary, "等待主管审批（仅批准本次请求）", true, employeeApprovalCardInfo{Fields: fields}))
	if !strings.Contains(string(data), "````text") {
		t.Fatal("command can close its preview fence")
	}
}

func TestCrossPlatformCoverageEmployeeApprovalStructuredDetails(t *testing.T) {
	for _, tc := range []struct{ method, body, contains string }{
		{"item/permissions/requestApproval", `{"permissions":{"network":{"enabled":true}}}`, "申请权限"},
		{"item/fileChange/requestApproval", `{"grantRoot":"/workspace","_dwsFileChanges":[{"path":"/workspace/a","diff":"+hello"}]}`, "文件变更"},
		{"item/commandExecution/requestApproval", `{"networkApprovalContext":{"host":"example.com","protocol":"https"}}`, "网络访问目标"},
	} {
		summary, safe := employeeApprovalSummary(tc.method, json.RawMessage(tc.body))
		if !safe || !strings.Contains(summary, tc.contains) {
			t.Fatalf("missing reviewable details: %q", summary)
		}
	}
	summary, safe := employeeApprovalSummary("item/commandExecution/requestApproval", json.RawMessage(`{"command":"printf hello > /workspace/a"}`))
	if !safe || strings.Contains(summary, `\u003e`) || !strings.Contains(summary, `hello > /workspace`) {
		t.Fatal("command preview must preserve shell syntax")
	}
}
