// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeCardRecoveryFixtureError() error {
	return employeeCardResponseError(nil, []byte(`{"error":{"category":"api","reason":"tls_certificate_untrusted","stage":"request","execution_started":false,"retryable":true,"message":"request failed","cause":"Post https://user:secret@mcp-gw.dingtalk.com/server/private?access_token=TOKEN_VALUE: tls: x509: certificate signed by unknown authority","execution_id":"test-request"}}`))
}

func TestCrossPlatformCoverageEmployeeCardRecoveryDiagnostics(t *testing.T) {
	for _, raw := range []string{
		`{"error":{"category":"api","reason":"request_failed","stage":"request","cause":"Post https://user:password@mcp-gw.dingtalk.com/private?token=SECRET: x509: unknown authority; Authorization: Bearer ABC; api_key=KEY; token=TOKEN","details":{"execution_id":"request-123"}}}`,
		`{"error":{"category":"auth","message":"private tool output","cause":"credential:SECRET"}}`,
		`{"error":{"category":"api","reason":"request_failed","cause":"private output"},`,
	} {
		err := employeeCardResponseError(nil, []byte(raw))
		var classified *employeeRunError
		if !errors.As(err, &classified) || classified.CardDiagnostic == nil {
			t.Fatal("诊断字段丢失")
		}
		d := classified.CardDiagnostic
		encoded, _ := json.Marshal(d)
		for _, secret := range []string{"SECRET", "ABC", "KEY", "TOKEN", "user:password", "private tool output", "private output"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("脱敏失败：%s", secret)
			}
		}
		if strings.Contains(raw, "request-123") && (d.ExecutionID != "request-123" || !strings.Contains(d.Cause, "unknown authority")) {
			t.Fatal("底层原因或关联请求 ID 丢失")
		}
	}
	long := employeeCardSafeDiagnosticText(strings.Repeat("字", 2000))
	if len(long) > 1024 || !json.Valid([]byte(`"`+long+`"`)) {
		t.Fatal("诊断没有按 UTF-8 限长")
	}
}

func TestCrossPlatformCoverageEmployeeCardRecoverySafetyBoundaries(t *testing.T) {
	text := employeeCardSafeDiagnosticText("bad https://[invalid\nline\tend\x7f")
	if !strings.Contains(text, "[redacted-url]") || strings.Contains(text, "invalid") || strings.ContainsAny(text, "\n\t\x7f") {
		t.Fatal("畸形 URL 或控制字符未安全清除")
	}
	if employeeCardTransportReason("unrecognized_business_failure") {
		t.Fatal("未知业务原因不能作为传输故障保留或恢复")
	}
	// 执行结果未知时不能向用户断言失败，更不能显示成功。
	if employeeTaskLabel(employeeTaskRecord{Status: "card_incomplete", Execution: "unknown"}) != "结果待确认" {
		t.Fatal("执行未知的卡片必须保留待确认状态")
	}
	r, e := employeeLedgerFixture(t)
	record := employeeTaskRecord{CardResultFile: "card-results/existing.json"}
	if err := r.preserveCardResult(e, &record, " \n\t"); err != nil || record.CardResultFile != "card-results/existing.json" {
		t.Fatal("空白结果不能覆盖既有答案引用")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "card-results")); !os.IsNotExist(err) {
		t.Fatal("空白结果不应创建结果目录")
	}
}

func TestCrossPlatformCoverageEmployeeCardRecoveryPartialAndStorageFailures(t *testing.T) {
	for _, scenario := range []string{"partial", "partial-unknown", "alert", "result-write"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			surface := newEmployeeCardTestSurface()
			failed := make(chan struct{})
			calls := 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if calls == 1 {
					return map[string]any{"bizId": "original"}, nil
				}
				if calls == 2 {
					if scenario == "alert" {
						if err := os.Mkdir(filepath.Join(r.dir, "runtime.log"), 0700); err != nil {
							t.Fatal(err)
						}
					}
					if scenario == "result-write" {
						if err := os.WriteFile(filepath.Join(r.dir, "card-results"), []byte("blocked"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					close(failed)
					if scenario == "partial-unknown" || scenario == "result-write" {
						return nil, employeeTerminal("card_write_unconfirmed")
					}
					return nil, employeeCardRecoveryFixtureError()
				}
				if calls != 3 || args[2] != "original" || args[4] != "FINISH" {
					t.Fatal("部分结果恢复必须只更新一次原卡终态")
				}
				surface.apply(messages)
				return map[string]any{"flowStatus": "FINISH"}, nil
			})
			record := employeeTaskRecord{}
			err := r.processCard(e, &record, &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
				emit(agentProgress{ID: "answer", Kind: "answer", Text: "已确认的部分答案"})
				select {
				case <-failed:
				case <-time.After(3 * time.Second):
					t.Fatal("未触发流式更新故障")
				}
				return "", errors.New("private agent failure")
			}})
			if record.Execution != "failure" {
				t.Fatal("部分答案不能把 Agent 失败改为成功")
			}
			switch scenario {
			case "partial":
				if err != nil || record.Status != "card_incomplete" || surface.text("result") != "已确认的部分答案" || employeeTaskLabel(record) != "执行失败" {
					t.Fatalf("部分结果未完整收尾：%v %+v", err, record)
				}
			case "partial-unknown":
				if err != nil || calls != 2 || record.CardResultFile == "" || employeeTaskLabel(record) != "送达待确认" {
					t.Fatalf("未知交付不能重发或丢弃部分结果：%v %+v", err, record)
				}
			default:
				want := "card_alert_unavailable"
				if scenario == "result-write" {
					want = "ledger_unavailable"
				}
				if err == nil || err.Error() != want || calls != 2 {
					t.Fatalf("持久化失败后不能继续交付：%v calls=%d", err, calls)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardRecoveryRejectedTask(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "a2ui"
	testseam.Swap(t, &employeeA2UICall, func(context.Context, digitalEmployeeBinding, []map[string]any, ...string) (map[string]any, error) {
		return nil, employeeTerminal("card_write_rejected")
	})
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(r.recordPath(e))
	var record employeeTaskRecord
	if err != nil || json.Unmarshal(raw, &record) != nil || record.Status != "card_rejected" || employeeTaskLabel(record) != "回复发送失败" {
		t.Fatal("明确拒绝的任务没有完成失败收尾")
	}
}

func TestCrossPlatformCoverageEmployeeCardRecoveryRestart(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	record := employeeTaskRecord{Status: "card_updating", Execution: "success", BizID: "original", CardResultFile: "card-results/result.json"}
	if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverInterruptedTasks(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(r.recordPath(e))
	_ = json.Unmarshal(raw, &record)
	if record.Execution != "success" || record.Delivery != "unknown" || record.Status != "needs_review" || record.CardResultFile == "" {
		t.Fatalf("恢复交付状态时覆盖了已确认的模型结果：%+v", record)
	}
}

func TestCrossPlatformCoverageEmployeeCardRecoveryPersistenceFailure(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	if err := os.WriteFile(filepath.Join(r.dir, "card-results"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	testseam.Swap(t, &employeeA2UICall, func(context.Context, digitalEmployeeBinding, []map[string]any, ...string) (map[string]any, error) {
		calls++
		if calls == 1 {
			return map[string]any{"bizId": "original"}, nil
		}
		return nil, employeeCardRecoveryFixtureError()
	})
	record := employeeTaskRecord{}
	err := r.processCard(e, &record, &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { return "完整结果", nil }})
	if err == nil || err.Error() != "ledger_unavailable" || calls != 2 || record.Status == "card_finished" {
		t.Fatalf("保存失败后仍然重试或假报成功：%v %+v calls=%d", err, record, calls)
	}
}

func TestCrossPlatformCoverageEmployeeCardRecoveryDelivery(t *testing.T) {
	for _, scenario := range []string{"transient", "persistent", "unknown", "rejected", "final-unknown", "final-only"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			surface := newEmployeeCardTestSurface()
			failed := make(chan struct{})
			calls, modelCalls := 0, 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if calls == 1 {
					surface.apply(messages)
					return map[string]any{"bizId": "original-card"}, nil
				}
				if args[2] != "original-card" {
					t.Fatal("恢复时创建了另一张卡片")
				}
				if calls == 2 {
					close(failed)
					switch scenario {
					case "unknown":
						return nil, employeeTerminal("card_write_unconfirmed")
					case "rejected":
						return nil, employeeTerminal("card_write_rejected")
					}
					return nil, employeeCardRecoveryFixtureError()
				}
				if args[4] != "FINISH" || calls > 3 {
					t.Fatal("只能补交一次本轮最终结果")
				}
				if scenario == "persistent" {
					return nil, employeeCardRecoveryFixtureError()
				}
				if scenario == "final-unknown" {
					return nil, employeeTerminal("card_write_unconfirmed")
				}
				surface.apply(messages)
				return map[string]any{"flowStatus": "FINISH"}, nil
			})
			record := employeeTaskRecord{}
			answer := "本轮完整答案，不应丢失，也不能写到运行日志"
			err := r.processCard(e, &record, &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
				modelCalls++
				if scenario != "final-only" {
					emit(agentProgress{ID: "progress", Kind: "progress", Text: "公开过程"})
					select {
					case <-failed:
					case <-time.After(3 * time.Second):
						t.Fatal("未触发过程更新故障")
					}
					emit(agentProgress{ID: "tool", Kind: "tool", Text: "文件读取", Status: "completed", Done: true})
				}
				return answer, nil
			}})
			if err != nil || modelCalls != 1 {
				t.Fatalf("恢复不能重跑 Agent：calls=%d err=%v", modelCalls, err)
			}
			wantStatus, wantCalls := "card_finished", 3
			if scenario == "persistent" || scenario == "rejected" {
				wantStatus = "card_delivery_failed"
			}
			if scenario == "unknown" || scenario == "final-unknown" {
				wantStatus = "needs_review"
			}
			if scenario == "unknown" || scenario == "rejected" {
				wantCalls = 2
			}
			if record.Status != wantStatus || calls != wantCalls || record.Execution != "success" {
				t.Fatalf("交付状态或恢复次数错误：%+v calls=%d", record, calls)
			}
			if wantStatus == "card_finished" {
				if surface.text("result") != answer || record.FlowStatus != "FINISH" || employeeTaskLabel(record) != "已完成" {
					t.Fatal("补交后正文未完整显示或未收尾")
				}
				if scenario != "final-only" && surface.components["tool-group-step-1"] == nil {
					t.Fatal("更新失败后丢弃了后续工具状态")
				}
			} else {
				if record.FlowStatus == "FINISH" {
					t.Fatal("交付失败不能假报 FINISH")
				}
				raw, _ := json.Marshal(record)
				var fields map[string]any
				_ = json.Unmarshal(raw, &fields)
				file, _ := fields["cardResultFile"].(string)
				if file == "" || filepath.IsAbs(file) {
					t.Fatal("未保留待交付答案引用")
				}
				path := filepath.Join(r.dir, file)
				data, err := os.ReadFile(path)
				var saved map[string]any
				_ = json.Unmarshal(data, &saved)
				info, statErr := os.Stat(path)
				if err != nil || statErr != nil || saved["answer"] != answer || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
					t.Fatal("答案没有安全保存")
				}
				if strings.Contains(string(raw), answer) {
					t.Fatal("任务账本不应内嵌业务正文")
				}
			}
			log, _ := os.ReadFile(filepath.Join(r.dir, "runtime.log"))
			if strings.Contains(string(log), answer) || strings.Contains(string(log), "TOKEN_VALUE") || strings.Contains(string(log), "user:secret") {
				t.Fatal("诊断日志泄露正文或凭据")
			}
			if scenario == "persistent" && (!strings.Contains(string(log), "unknown authority") || employeeTaskLabel(record) != "回复发送失败") {
				t.Fatal("没有保留底层原因或明确失败状态")
			}
		})
	}
}
