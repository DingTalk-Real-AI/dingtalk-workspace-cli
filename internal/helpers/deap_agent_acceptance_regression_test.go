// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestEmployeeFeedbackErrorSubprocessFixture(t *testing.T) {
	mode := os.Getenv("DWS_FEEDBACK_ERROR_FIXTURE")
	if mode == "" {
		return
	}
	if mode == "overflow" {
		_, _ = os.Stderr.WriteString(strings.Repeat("x", digitalEmployeeStdinLimit+1))
		os.Exit(0)
	}
	if mode == "rejected" {
		_, _ = os.Stdout.WriteString(`{"success":false}`)
		os.Exit(0)
	}
	failure := `{"error":{"category":"api","reason":"tls_certificate_untrusted","stage":"request","execution_started":false,"retryable":true,"execution_id":"feedback-request","cause":"Post https://user:secret@example.com/private?token=PRIVATE: x509: unknown authority"}}`
	_, _ = os.Stderr.WriteString(failure)
	os.Exit(1)
}

func TestCrossPlatformCoverageEmployeeFeedbackWireFailures(t *testing.T) {
	for _, mode := range []string{"overflow", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("DWS_FEEDBACK_ERROR_FIXTURE", mode)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackErrorSubprocessFixture$")
			})
			_, err := runEmployeeFeedbackCall(context.Background(), "corp:employee", "add-text-emotion", employeeEvent{}, employeeEmotion{})
			want := "feedback_command_failed"
			if mode == "overflow" {
				want = "feedback_output_overflow"
			}
			if err == nil || err.Error() != want {
				t.Fatalf("反馈错误分类不正确：got=%v want=%s", err, want)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardNoticeLedgerFailureStopsSend(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "a2ui"
	r.fwd = &employeeBoundaryForwarder{answer: "保留结果"}
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "single"}, nil
		}
		return nil, employeeCardRecoveryFixtureError()
	})
	testseam.Swap(t, &atomicRename, func(src, dst string) error {
		raw, _ := os.ReadFile(src)
		var record employeeTaskRecord
		if json.Unmarshal(raw, &record) == nil && record.CardNoticeDelivery == "unknown" {
			return errors.New("notice ledger unavailable")
		}
		return os.Rename(src, dst)
	})
	testseam.Swap(t, &employeeExecCommand, func(context.Context, string, ...string) *exec.Cmd {
		t.Error("通知账本失败后仍然发起发送")
		return exec.Command("missing-fixture")
	})
	if err := r.process(e); err == nil || err.Error() != "ledger_unavailable" {
		t.Fatalf("通知持久化失败未停止：%v", err)
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackKeepsSafeDiagnostic(t *testing.T) {
	t.Setenv("DWS_FEEDBACK_ERROR_FIXTURE", "1")
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackErrorSubprocessFixture$")
	})
	_, err := runEmployeeFeedbackCall(context.Background(), "corp:employee", "add-text-emotion", employeeEvent{ConversationID: "c", MessageID: "m"}, employeeEmotion{ID: "id", Label: "回复发送失败"})
	var classified *employeeRunError
	if !errors.As(err, &classified) || classified.CardDiagnostic == nil || classified.CardDiagnostic.ExecutionID != "feedback-request" || !strings.Contains(classified.CardDiagnostic.Cause, "unknown authority") {
		t.Fatalf("底层反馈错误及关联请求丢失：%v", err)
	}
	raw, _ := json.Marshal(classified.CardDiagnostic)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "user:secret") {
		t.Fatal("反馈诊断泄露凭据")
	}
}

func TestCrossPlatformCoverageEmployeeLifecycleDryRunDoesNotControlHost(t *testing.T) {
	for _, action := range []string{"stop", "restart"} {
		t.Run(action, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			calls := 0
			testseam.Swap(t, &employeeDSHControl, func(context.Context, digitalEmployeeBinding, string) (employeeDSHState, error) {
				calls++
				return employeeDSHState{}, errors.New("预览不允许控制宿主")
			})
			cmd := newDigitalEmployeeStopCommand()
			if action == "restart" {
				cmd = newDigitalEmployeeRestartCommand()
			}
			cmd.Flags().Bool("dry-run", true, "test")
			_ = cmd.Flags().Set("agent-uuid", b.AgentUUID)
			cmd.SetContext(context.Background())
			var out bytes.Buffer
			cmd.SetOut(&out)
			err := runDigitalEmployeeLifecycle(cmd, action)
			current, loadErr := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
			if err != nil || loadErr != nil || calls != 0 || current.DesiredState != b.DesiredState || !strings.Contains(out.String(), "planned") {
				t.Fatalf("dry-run 发生副作用：calls=%d desired=%s err=%v output=%s", calls, current.DesiredState, err, out.String())
			}
		})
	}
}

// 慢请求期间仍会到达新快照；恢复后应立刻合并最新内容，不能再等完整节流周期。
func TestCrossPlatformCoverageEmployeeStreamingSlowWriteCoalescesLatest(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.ctx = ctx
	started, release := make(chan struct{}), make(chan struct{})
	frames := make(chan string, 8)
	surface := newEmployeeCardTestSurface()
	updates := 0
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "single"}, nil
		}
		surface.apply(messages)
		if args[4] == "INPUTTING" {
			updates++
			if updates == 1 {
				close(started)
				<-release
			} else {
				frames <- surface.text("answer")
			}
		}
		return map[string]any{"flowStatus": args[4]}, nil
	})
	f := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		emit(agentProgress{ID: "answer", Kind: "answer", Text: "开头"})
		<-started
		for i := 1; i <= 100; i++ {
			emit(agentProgress{ID: "answer", Kind: "answer", Text: strings.Repeat("字", i)})
		}
		time.Sleep(2 * employeeCardFlushInterval)
		close(release)
		select {
		case text := <-frames:
			if text != strings.Repeat("字", 100) {
				t.Errorf("慢发送后发出了陈旧快照：%d", len([]rune(text)))
			}
		case <-time.After(employeeCardFlushInterval / 2):
			t.Error("慢请求完成后额外等待了整个节流周期")
		}
		return "完整结果", nil
	}}
	record := employeeTaskRecord{}
	if err := r.processCard(e, &record, f); err != nil || record.Status != "card_finished" || surface.text("answer") != "完整结果" {
		t.Fatalf("最终结果未收尾：%+v %v", record, err)
	}
}

func TestCrossPlatformCoverageEmployeeCardFailureHasIndependentNotice(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "a2ui"
	r.fwd = &employeeBoundaryForwarder{answer: "已保存的完整结果"}
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "one-card"}, nil
		}
		return nil, employeeCardRecoveryFixtureError()
	})
	calls := 0
	t.Setenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE", `{"openMessageId":"notice-id","deliveryStatus":"delivered"}`)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		calls++
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "+messages-reply") || !strings.Contains(joined, "--employee-context") || !strings.Contains(joined, "--idempotency-key stable-card-failure") || !strings.Contains(joined, e.MessageID) {
			t.Errorf("失败通知没有绑定原请求及独立去重键：%v", args)
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackSubprocessFixture$")
	})
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("卡片持续失败后用户没有独立终态通知：calls=%d", calls)
	}
	var record employeeTaskRecord
	raw, _ := os.ReadFile(r.recordPath(e))
	_ = json.Unmarshal(raw, &record)
	if record.CardNoticeDelivery != "delivered" || record.CardNoticeReplyID != "notice-id" || record.Status != "card_delivery_failed" || record.Delivery != "failure" {
		t.Fatalf("通知送达不能冒充原卡片成功：%+v", record)
	}
}

func TestCrossPlatformCoverageEmployeeCardNoticeSafety(t *testing.T) {
	for _, scenario := range []string{"delivered", "unknown", "already-attempted", "cancelled", "forbidden", "ledger", "send-error", "unconfirmed", "missing-id"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			record := employeeTaskRecord{Delivery: "failure", IdempotencyKey: "stable"}
			switch scenario {
			case "delivered", "unknown":
				record.Delivery = scenario
			case "already-attempted":
				record.CardNoticeDelivery = "unknown"
			case "cancelled":
				ctx, cancel := context.WithCancel(r.ctx)
				cancel()
				r.ctx = ctx
			case "forbidden":
				e.SenderID = "other"
			case "ledger":
				testseam.Swap(t, &atomicRename, func(string, string) error { return errors.New("disk") })
			}
			response := `{"deliveryStatus":"unknown","openMessageId":"maybe-id"}`
			if scenario == "unconfirmed" {
				response = `{"deliveryStatus":"delivered","openMessageId":""}`
			}
			if scenario == "missing-id" {
				response = `{"ok":true,"data":{"deliveryStatus":"delivered"}}`
			}
			t.Setenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE", response)
			calls := 0
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				calls++
				raw, _ := os.ReadFile(r.recordPath(e))
				var saved employeeTaskRecord
				if json.Unmarshal(raw, &saved) != nil || saved.CardNoticeDelivery != "unknown" {
					t.Error("通知发送前没有持久化未知状态")
				}
				if scenario == "send-error" {
					return exec.CommandContext(ctx, "missing-notice-fixture")
				}
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackSubprocessFixture$")
			})
			err := r.notifyCardFailure(e, &record)
			wantError := scenario == "forbidden" || scenario == "ledger"
			if (err != nil) != wantError {
				t.Fatalf("通知错误边界：%v", err)
			}
			wantCalls := 0
			if scenario == "send-error" || scenario == "unconfirmed" || scenario == "missing-id" {
				wantCalls = 1
				if record.CardNoticeDelivery != "unknown" || record.CardNoticeReplyID != "" {
					t.Fatal("未知回执错误声明为已送达")
				}
				_ = r.notifyCardFailure(e, &record)
			}
			if calls != wantCalls {
				t.Fatalf("通知不能重复发送：%d", calls)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackDiagnosticPersistence(t *testing.T) {
	f := newEmployeeFeedback(context.Background(), "corp:employee", t.TempDir())
	e := employeeEvent{ConversationID: "c", MessageID: "m"}
	testseam.Swap(t, &employeeFeedbackCall, func(context.Context, string, string, employeeEvent, employeeEmotion) (map[string]any, error) {
		return nil, employeeFeedbackResponseError(nil, []byte(`{"error":{"category":"api","reason":"connection_refused","stage":"request","execution_id":"feedback-request","cause":"connection refused"}}`))
	})
	f.set(e, "回复发送失败", true)
	f.wait()
	var record employeeFeedbackRecord
	raw, _ := os.ReadFile(f.path(e))
	if json.Unmarshal(raw, &record) != nil || record.Status != "failed" || record.LastError == nil || record.LastError.ExecutionID != "feedback-request" {
		t.Fatal("反馈账本没有保留诊断")
	}
	if _, err := os.Stat(filepath.Join(f.dir, "feedback")); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageCodexDesktopActivePollCadence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	if err := codexDesktopPause(ctx); err != nil {
		t.Fatalf("活跃流式回合仍按一秒轮询：%v", err)
	}
}
