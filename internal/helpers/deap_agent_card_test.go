// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type employeeProgressFixture struct {
	employeeBoundaryForwarder
	run func(func(agentProgress)) (string, error)
}

func (f *employeeProgressFixture) forwardProgress(_ context.Context, _, _ string, emit func(agentProgress), _ ...connectMediaAttachment) (string, error) {
	return f.run(emit)
}

func TestCrossPlatformCoverageEmployeeCardLifecycle(t *testing.T) {
	for _, scenario := range []string{"success", "group", "create-unknown", "missing-biz", "update-unknown", "agent-error", "empty", "text", "control"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.cfg.Binding.DWSProfile = "corp:employee"
			r.cfg.ReplyMode = "a2ui"
			if scenario == "group" {
				e.Type = "user_im_message_receive_group_all"
				r.cfg.Options.AllowedGroups = []string{e.ConversationID}
			}
			if scenario == "text" {
				r.cfg.ReplyMode = "text"
			}
			if scenario == "control" {
				e.Content = "/new"
			}
			calls, agentCalls := 0, 0
			var frames []string
			surface := ""
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, b digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if b.DWSProfile != "corp:employee" {
					t.Fatal("wrong sender profile")
				}
				body, _ := json.Marshal(messages)
				frames = append(frames, string(body))
				running := calls == 1 || args[4] != "FINISH"
				if strings.Contains(string(body), "[AIGenerating]") != running || strings.Contains(string(body), `"textEffect":"shimmer"`) != running {
					t.Fatalf("loading animation must follow running state: %s", body)
				}
				if strings.Contains(string(body), "defaultExpanded") {
					t.Fatal("updates must preserve client expansion state")
				}
				var ledger employeeTaskRecord
				data, err := os.ReadFile(r.recordPath(e))
				if err != nil || json.Unmarshal(data, &ledger) != nil {
					t.Fatal("write attempted without durable ledger")
				}
				if calls == 1 {
					wantTarget := "--open-dingtalk-id sender"
					if scenario == "group" {
						wantTarget = "--conversation-id conversation"
					}
					if strings.Join(args, " ") != "send-a2ui-card "+wantTarget || ledger.Status != "card_creating" || agentCalls != 0 {
						t.Fatalf("placeholder must precede model and use trusted target: %v %v", args, ledger)
					}
					surface = ledger.SurfaceID
					if scenario == "create-unknown" {
						return nil, errors.New("response lost")
					}
					if scenario == "missing-biz" {
						return map[string]any{"openTaskId": "must-not-use"}, nil
					}
					return map[string]any{"bizId": "server-real-biz", "openTaskId": "not-biz"}, nil
				}
				if args[0] != "update-a2ui-card" || args[2] != "server-real-biz" || ledger.Status != "card_updating" || ledger.SurfaceID != surface || strings.Contains(string(body), "createSurface") || !strings.Contains(string(body), surface) {
					t.Fatalf("invalid update identity or ledger: %v %v", args, ledger)
				}
				if strings.Contains(string(body), `"id":"answer"`) {
					if len(args) != 7 || args[5] != "--a2ui-annotations" || !strings.Contains(args[6], `"type":"artifact"`) || !strings.Contains(args[6], `"componentId":"answer"`) || !strings.Contains(args[6], surface) {
						t.Fatalf("answer component must be annotated as an artifact: %v", args)
					}
				}
				if scenario == "update-unknown" {
					return nil, errors.New("response lost")
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			fwd := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
				agentCalls++
				if calls != 1 {
					t.Fatal("model ran before placeholder acknowledgement")
				}
				emit(agentProgress{ID: "public-1", Kind: "progress", Text: "已完成数据核对", Done: true})
				emit(agentProgress{ID: "tool-1", Kind: "tool"})
				emit(agentProgress{ID: "tool-1", Kind: "tool", Done: true})
				emit(agentProgress{ID: "answer-1", Kind: "answer", Text: "已有结果"})
				if scenario == "agent-error" {
					return "", errors.New("private tool failure")
				}
				if scenario == "empty" {
					return "", nil
				}
				return "完整结果", nil
			}}
			fwd.answer = "text fallback"
			r.fwd = fwd
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, filepath.Join(r.dir, "no-remote-test"))
			})
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "event-stable"}); err != nil {
				t.Fatal(err)
			}
			if err := r.process(e); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(r.recordPath(e))
			var result employeeTaskRecord
			_ = json.Unmarshal(data, &result)
			want := "card_finished"
			switch scenario {
			case "create-unknown", "missing-biz", "update-unknown", "text", "control":
				want = "needs_review"
			case "agent-error", "empty":
				want = "card_incomplete"
			}
			if result.Status != want {
				t.Fatalf("status=%s want=%s", result.Status, want)
			}
			if scenario == "text" || scenario == "control" {
				if calls != 0 || agentCalls != 0 {
					t.Fatal("text/control invoked card path")
				}
				return
			}
			if strings.Contains(string(data), "完整结果") || strings.Contains(string(data), "private") {
				t.Fatal("ledger leaked content")
			}
			if want == "needs_review" {
				expected := 1
				if scenario == "update-unknown" {
					expected = 2
				}
				if calls != expected {
					t.Fatal("unknown write was retried or finalized")
				}
			} else if result.FlowStatus != "FINISH" || result.CardUpdates != 2 || result.CardCreatedAt == "" {
				t.Fatalf("missing terminal state: %+v", result)
			}
			if want == "card_incomplete" && (!strings.Contains(frames[len(frames)-1], "已有结果") || strings.Contains(frames[len(frames)-1], "处理完成")) {
				t.Fatal("failure erased results or claimed success")
			}
			before := calls
			if err := r.enqueue(e); err != nil || before != calls {
				t.Fatal("duplicate event repeated card")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardInterruptedAndUnauthorized(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.Options.AllowedUsers = nil
	f := &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { t.Fatal("unauthorized model"); return "", nil }}
	if err := r.processCard(e, &employeeTaskRecord{}, f); err == nil {
		t.Fatal("unauthorized reply allowed")
	}
	for _, state := range []string{"card_creating", "card_streaming", "card_updating"} {
		if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: state, BizID: "real", SurfaceID: "stable"}); err != nil {
			t.Fatal(err)
		}
		if err := r.recoverInterruptedTasks(); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(r.recordPath(e))
		var got employeeTaskRecord
		_ = json.Unmarshal(data, &got)
		if got.Status != "needs_review" || got.BizID != "real" || got.SurfaceID != "stable" {
			t.Fatal("interrupted task lost identity or became retryable")
		}
	}
}

func TestEmployeeCardOutputFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "employee-card-output" {
		return
	}
	fmt.Print(os.Getenv("DWS_EMPLOYEE_CARD_TEST_BODY"))
	if os.Getenv("DWS_EMPLOYEE_CARD_TEST_STDIN") == "1" {
		data, err := io.ReadAll(os.Stdin)
		var messages []string
		if err != nil || json.Unmarshal(data, &messages) != nil {
			os.Exit(12)
		}
	}
	os.Exit(0)
}

func TestCrossPlatformCoverageEmployeeCardCLITransport(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	t.Setenv("DWS_EMPLOYEE_CARD_TEST_STDIN", "1")
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, bin string, args ...string) *exec.Cmd {
		if len(args) < 9 || args[0] != "--profile" || args[1] != cfg.Binding.DWSProfile || args[2] != "chat" || args[4] != "send-a2ui-card" {
			t.Fatalf("wrong CLI boundary: %v", args)
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCardOutputFixture$", "--", "employee-card-output")
	})
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"success":true,"result":{"bizId":"real"}}`, true},
		{`{"success":false,"result":{"bizId":"fake"}}`, false},
		{`{"ok":true,"data":{"bizId":"wrong-envelope"}}`, false},
		{`{"success":true}`, false},
		{`not json`, false},
	} {
		t.Setenv("DWS_EMPLOYEE_CARD_TEST_BODY", tc.body)
		_, err := runEmployeeA2UICall(context.Background(), cfg.Binding, []map[string]any{{"version": "v1.0"}}, "send-a2ui-card", "--open-dingtalk-id", "owner")
		if (err == nil) != tc.ok {
			t.Fatalf("body %q: %v", tc.body, err)
		}
	}
	for _, message := range []map[string]any{{"bad": make(chan int)}, {"large": strings.Repeat("x", 70*1024)}} {
		if _, err := runEmployeeA2UICall(context.Background(), cfg.Binding, []map[string]any{message}, "send-a2ui-card"); err == nil {
			t.Fatal("invalid payload reached CLI")
		}
	}
	testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", errors.New("unavailable") })
	if _, err := runEmployeeA2UICall(context.Background(), cfg.Binding, nil, "send-a2ui-card"); err == nil || err.Error() != "dws_command_unavailable" {
		t.Fatalf("unavailable executable: %v", err)
	}
	cfg.Binding.BindingRevision++
	if _, err := runEmployeeA2UICall(context.Background(), cfg.Binding, nil, "send-a2ui-card"); err == nil {
		t.Fatal("stale binding sent a card")
	}
}

func TestCrossPlatformCoverageEmployeeCardPersistenceAndTerminalFailures(t *testing.T) {
	for _, scenario := range []string{"ledger-1", "ledger-2", "ledger-3", "ledger-4", "ledger-5", "ledger-6", "status", "finish"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			writes, calls := 0, 0
			testseam.Swap(t, &atomicRename, func(from, to string) error {
				writes++
				if scenario == fmt.Sprintf("ledger-%d", writes) {
					return errors.New("disk unavailable")
				}
				return os.Rename(from, to)
			})
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if calls == 1 {
					return map[string]any{"bizId": "real"}, nil
				}
				if scenario == "finish" && args[4] == "FINISH" {
					return nil, errors.New("lost response")
				}
				if scenario == "status" {
					return map[string]any{"flowStatus": "FINISH"}, nil
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			f := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
				emit(agentProgress{ID: "one", Kind: "progress", Text: "已核对"})
				emit(agentProgress{ID: "two", Kind: "progress", Text: "已分析"})
				return "最终结果", nil
			}}
			record := employeeTaskRecord{}
			err := r.processCard(e, &record, f)
			if scenario == "ledger-1" || scenario == "ledger-2" {
				if err == nil || err.Error() != "ledger_unavailable" || calls > 1 {
					t.Fatalf("unsafe create after ledger failure: calls=%d err=%v", calls, err)
				}
			} else if err != nil || record.Status != "needs_review" || record.CardError == "" || calls > 3 {
				t.Fatalf("unsafe completion/retry: %+v calls=%d err=%v", record, calls, err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardBoundedPublicView(t *testing.T) {
	v := employeeCardView{}
	v.accept(agentProgress{ID: "private", Kind: "reasoning", Text: "must not publish"})
	for i := 0; i < 40; i++ {
		v.accept(agentProgress{ID: fmt.Sprint(i), Kind: "tool"})
	}
	v.accept(agentProgress{ID: "39", Kind: "tool", Done: true})
	messages := v.messages("surface", "正在处理", true)
	body, err := json.Marshal(messages)
	if err != nil || strings.Contains(string(body), "must not publish") || strings.Contains(string(body), `"id":"step-0"`) || !strings.Contains(string(body), `"id":"step-39"`) || !strings.Contains(string(body), "工具调用已结束") {
		t.Fatalf("invalid bounded public view: %s %v", body, err)
	}
	components := messages[1]["updateComponents"].(map[string]any)["components"].([]map[string]any)
	if len(components) != 34 {
		t.Fatalf("expected 32 visible tools plus panel and root, got %d", len(components))
	}
}

func TestCrossPlatformCoverageEmployeeCardRuntimeFailurePropagation(t *testing.T) {
	for _, stage := range []string{"process-card-ledger", "finish-record-audit"} {
		t.Run(stage, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.cfg.ReplyMode = "a2ui"
			path := r.recordPath(e)
			if err := writeEmployeeJSON(path, employeeTaskRecord{Status: "accepted", IdempotencyKey: "event-stable"}); err != nil {
				t.Fatal(err)
			}
			var feedbackLabels []string
			testseam.Swap(t, &employeeFeedbackCall, func(_ context.Context, _, _ string, _ employeeEvent, emotion employeeEmotion) (map[string]any, error) {
				feedbackLabels = append(feedbackLabels, emotion.Label)
				return map[string]any{"emotionId": "fixture-" + emotion.Label}, nil
			})
			r.feedback = newEmployeeFeedback(r.ctx, "corp:employee", r.dir)
			defer r.feedback.wait()

			injected := false
			if stage == "process-card-ledger" {
				// 只阻断卡片创建前的账本提交，让运行态落盘与反馈记录保持真实成功。
				testseam.Swap(t, &atomicRename, func(from, to string) error {
					if to == path {
						raw, err := os.ReadFile(from)
						var pending employeeTaskRecord
						if err != nil {
							return err
						}
						if err := json.Unmarshal(raw, &pending); err != nil {
							return err
						}
						if pending.Status == "card_creating" {
							injected = true
							return errors.New("private card ledger fault")
						}
					}
					return os.Rename(from, to)
				})
			} else {
				// 卡片交付完成后，真实审计文件打开失败，不能把最终账本写成完成。
				if err := os.Mkdir(filepath.Join(r.dir, "audit.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cardCalls, agentCalls := 0, 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				cardCalls++
				if args[0] == "send-a2ui-card" {
					return map[string]any{"bizId": "server-biz"}, nil
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			r.fwd = &employeeProgressFixture{run: func(func(agentProgress)) (string, error) {
				agentCalls++
				return "final answer", nil
			}}

			err := r.process(e)
			r.feedback.wait()
			wantCode, wantStatus := "ledger_unavailable", "running"
			if stage == "finish-record-audit" {
				wantCode, wantStatus = "audit_unavailable", "card_streaming"
			}
			var classified *employeeRunError
			if !errors.As(err, &classified) || classified.Code != wantCode || classified.Retryable == nil || *classified.Retryable || err.Error() != wantCode {
				t.Fatalf("运行时未保留稳定的不可重试错误：%v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record employeeTaskRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			if record.Status != wantStatus || record.IdempotencyKey != "event-stable" {
				t.Fatalf("失败后丢失原账本或错误提交完成状态：%+v", record)
			}
			if stage == "process-card-ledger" {
				if !injected || cardCalls != 0 || agentCalls != 0 || record.BizID != "" || record.Execution != "running" {
					t.Fatalf("卡片账本失败后继续执行：injected=%v card=%d agent=%d record=%+v", injected, cardCalls, agentCalls, record)
				}
			} else if cardCalls != 2 || agentCalls != 1 || record.BizID != "server-biz" || record.FlowStatus != "FINISH" || record.Delivery != "accepted" {
				t.Fatalf("未在已确认交付后的审计阶段失败：card=%d agent=%d record=%+v", cardCalls, agentCalls, record)
			}
			for _, label := range feedbackLabels {
				if label == "已完成" {
					t.Fatal("运行时失败仍发布成功反馈")
				}
			}
			raw, err = os.ReadFile(r.feedback.path(e))
			if err != nil {
				t.Fatal(err)
			}
			var feedback employeeFeedbackRecord
			if err := json.Unmarshal(raw, &feedback); err != nil {
				t.Fatal(err)
			}
			if feedback.Desired != "已中断，待核查" || feedback.Applied != "已中断，待核查" || feedback.Status != "applied" {
				t.Fatalf("错误透传后未收敛到待核查反馈：%+v", feedback)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardInvalidConfigAndLedger(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	cfg.ReplyMode = "invalid"
	if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json"), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDigitalEmployeeConfig(cfg.Binding.DWSProfile); err == nil || !strings.Contains(err.Error(), "reply mode") {
		t.Fatalf("invalid config accepted: %v", err)
	}
	r, e := employeeLedgerFixture(t)
	r.fwd = &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { t.Fatal("model ran without ledger"); return "", nil }}
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted"}); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &atomicRename, func(string, string) error { return errors.New("disk unavailable") })
	if err := r.process(e); err == nil || err.Error() != "ledger_unavailable" {
		t.Fatalf("ledger failure not propagated: %v", err)
	}
}
