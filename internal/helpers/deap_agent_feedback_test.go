// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeFailureRepliesToUser(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.fwd = &employeeBoundaryForwarder{err: errors.New("private model error")}
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	testseam.Swap(t, &employeeExecCommand, func(context.Context, string, ...string) *exec.Cmd {
		calls++
		return exec.Command("missing-fixture-command")
	})
	if err := r.process(e); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("执行失败后没有向用户发送安全提示：调用次数 = %d", calls)
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackSerializesAndCoalesces(t *testing.T) {
	f := newEmployeeFeedback(context.Background(), "corp:employee", t.TempDir())
	e := employeeEvent{ConversationID: "conversation", MessageID: "message"}
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var calls []string
	testseam.Swap(t, &employeeFeedbackCall, func(_ context.Context, profile, operation string, _ employeeEvent, emotion employeeEmotion) (map[string]any, error) {
		if profile != "corp:employee" {
			return nil, errors.New("wrong employee")
		}
		mu.Lock()
		calls = append(calls, operation+":"+emotion.Label)
		mu.Unlock()
		if operation == "add-text-emotion" && emotion.Label == "排队中" {
			close(started)
			<-release
		}
		return map[string]any{"success": true, "emotionId": "id-" + emotion.Label}, nil
	})
	f.set(e, "排队中", false)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("没有入站反馈")
	}
	f.set(e, "思考中", false)
	f.set(e, "已完成", true)
	f.set(e, "思考中", false)
	close(release)
	f.wait()
	want := []string{"create-text-emotion:排队中", "add-text-emotion:排队中", "remove-text-emotion:排队中", "create-text-emotion:已完成", "add-text-emotion:已完成"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v", calls)
	}
	raw, err := os.ReadFile(f.path(e))
	if err != nil {
		t.Fatal(err)
	}
	var record employeeFeedbackRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record.Applied != "已完成" || record.Status != "applied" {
		t.Fatalf("record = %+v", record)
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackFailureRecoveryAndIsolation(t *testing.T) {
	for _, failure := range []string{"remove", "add", "create", "missing-id"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			f := newEmployeeFeedback(context.Background(), "corp:employee", dir)
			e := employeeEvent{ConversationID: "conversation", MessageID: "message"}
			old := employeeEmotion{ID: "owned-old", Label: "思考中"}
			record := employeeFeedbackRecord{ConversationID: e.ConversationID, MessageID: e.MessageID, Owned: []employeeEmotion{old}, Applied: old.Label}
			var removed []string
			fail := true
			testseam.Swap(t, &employeeFeedbackCall, func(_ context.Context, profile, operation string, got employeeEvent, emotion employeeEmotion) (map[string]any, error) {
				if got != e || profile != "corp:employee" {
					return nil, errors.New("wrong target")
				}
				if operation == "remove-text-emotion" {
					removed = append(removed, emotion.ID)
				}
				if fail && (failure == "remove" && operation == "remove-text-emotion" || failure == "add" && operation == "add-text-emotion" || failure == "create" && operation == "create-text-emotion") {
					return nil, errors.New("private error")
				}
				if fail && failure == "missing-id" && operation == "create-text-emotion" {
					return map[string]any{"success": true}, nil
				}
				return map[string]any{"success": true, "emotionId": "new-id"}, nil
			})
			if err := f.apply(context.Background(), e, &record, "已完成"); err == nil {
				t.Fatal("忽略了表情失败")
			}
			if record.Status != "failed" || record.Desired != "已完成" {
				t.Fatalf("record = %+v", record)
			}
			if failure == "remove" && record.Applied != "思考中" {
				t.Fatal("撤旧失败仍宣称已清理")
			}
			if failure == "add" && len(record.Owned) != 1 {
				t.Fatal("未保存结果未知的表情 ID")
			}
			fail = false
			restarted := newEmployeeFeedback(context.Background(), "corp:employee", dir)
			restarted.recover(e, "已完成")
			restarted.wait()
			raw, err := os.ReadFile(f.path(e))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			if record.Status != "applied" || record.Applied != "已完成" {
				t.Fatalf("recovery = %+v", record)
			}
			for _, id := range removed {
				if id != "owned-old" && id != "new-id" {
					t.Fatalf("撤除了不属于本轮的表情 %s", id)
				}
			}
			f.wait()
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackShutdownAndCache(t *testing.T) {
	dir := t.TempDir()
	f := newEmployeeFeedback(context.Background(), "corp:employee", dir)
	var created int
	var mu sync.Mutex
	testseam.Swap(t, &employeeFeedbackCall, func(ctx context.Context, _, operation string, _ employeeEvent, _ employeeEmotion) (map[string]any, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		mu.Lock()
		defer mu.Unlock()
		if operation == "create-text-emotion" {
			created++
		}
		return map[string]any{"success": true, "emotionId": "real-id"}, nil
	})
	e := employeeEvent{ConversationID: "c", MessageID: "m"}
	f.set(e, "思考中", false)
	f.wait()
	raw, err := os.ReadFile(f.path(e))
	if err != nil {
		t.Fatal(err)
	}
	var record employeeFeedbackRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record.Applied != "已中断，待核查" {
		t.Fatalf("stop = %+v", record)
	}
	before := created
	restarted := newEmployeeFeedback(context.Background(), "corp:employee", dir)
	if _, err := restarted.emotionID(context.Background(), e, "已中断，待核查"); err != nil {
		t.Fatal(err)
	}
	if created != before {
		t.Fatal("重启后重复创建状态模板")
	}
	restarted.wait()
}

func TestCrossPlatformCoverageEmployeeFeedbackWireAndCommands(t *testing.T) {
	largeID, err := decodeEmployeeFeedbackResult([]byte(`{"success":true,"emotionId":9223372036854775807}`))
	if err != nil || findJSONScalar(largeID, "emotionId") != "9223372036854775807" {
		t.Fatal("表情 ID 精度丢失", err)
	}
	for _, tc := range []struct {
		raw string
		ok  bool
	}{
		{`{"success":true,"result":{"emotionId":"real-id"}}`, true},
		{`{"ok":true,"data":{"emotionId":"real-id"}}`, true},
		{`{"ok":true,"data":{"success":false}}`, false},
		{`{"success":false}`, false}, {`{"ok":false}`, false},
		{`{}`, false}, {`null`, false}, {`[]`, false}, {`{"success":true} {}`, false},
	} {
		if _, err := decodeEmployeeFeedbackResult([]byte(tc.raw)); (err == nil) != tc.ok {
			t.Fatalf("raw=%s err=%v", tc.raw, err)
		}
	}
	for _, operation := range []string{"create-text-emotion", "add-text-emotion", "remove-text-emotion"} {
		t.Run(operation, func(t *testing.T) {
			t.Setenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE", `{"success":true,"result":{"emotionId":"real-id"}}`)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				root := newChatCommand()
				command, _, err := root.Find([]string{"message", operation})
				if err != nil {
					t.Fatal(err)
				}
				// 去掉全局 profile 与 format，再用真实 Cobra 验证参数。
				var flags []string
				for i := 5; i < len(args); i++ {
					if args[i] == "--format" {
						i++
						continue
					}
					flags = append(flags, args[i])
				}
				if err := command.ParseFlags(flags); err != nil {
					t.Fatal(err)
				}
				if err := command.ValidateRequiredFlags(); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.Join(args, " "), "--profile corp:employee chat message "+operation) {
					t.Fatalf("args=%v", args)
				}
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackSubprocessFixture$")
			})
			if _, err := runEmployeeFeedbackCall(context.Background(), "corp:employee", operation, employeeEvent{ConversationID: "c", MessageID: "m"}, employeeEmotion{ID: "real-id", Label: "思考中"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackRejectsCorruptCache(t *testing.T) {
	for _, body := range []string{`{"思考中":1}`, `{"思考中":""}`, `{"思考中":"real-id","已完成":1}`} {
		t.Run(body, func(t *testing.T) {
			f := newEmployeeFeedback(context.Background(), "corp:employee", t.TempDir())
			if err := os.WriteFile(filepath.Join(f.dir, "feedback-emotions.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &employeeFeedbackCall, func(context.Context, string, string, employeeEvent, employeeEmotion) (map[string]any, error) {
				t.Error("缓存损坏仍发起网络请求")
				return nil, nil
			})
			for i := 0; i < 2; i++ {
				if _, err := f.emotionID(context.Background(), employeeEvent{}, "思考中"); err == nil {
					t.Fatal("缓存损坏被忽略")
				}
			}
			f.wait()
		})
	}
}

func TestEmployeeFeedbackSubprocessFixture(t *testing.T) {
	if raw := os.Getenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE"); raw != "" {
		fmt.Print(raw)
		os.Exit(0)
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackCorruptOwnershipStops(t *testing.T) {
	for _, body := range []string{`{`, `{"conversationId":"someone-else","messageId":"m"}`} {
		t.Run(body, func(t *testing.T) {
			f := newEmployeeFeedback(context.Background(), "corp:employee", t.TempDir())
			e := employeeEvent{ConversationID: "c", MessageID: "m"}
			if err := os.MkdirAll(filepath.Dir(f.path(e)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.path(e), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &employeeFeedbackCall, func(context.Context, string, string, employeeEvent, employeeEmotion) (map[string]any, error) {
				t.Error("归属不明仍调用外部接口")
				return nil, nil
			})
			f.set(e, "已完成", true)
			f.wait()
		})
	}
}

type employeeFeedbackForwarder func(context.Context, string, string) (string, error)

func (f employeeFeedbackForwarder) label() string { return "fixture" }
func (f employeeFeedbackForwarder) forward(ctx context.Context, conversation, text string) (string, error) {
	return f(ctx, conversation, text)
}

func TestCrossPlatformCoverageEmployeeFeedbackFollowsExecutionAndReceipt(t *testing.T) {
	for _, scenario := range []struct {
		name, answer, receipt, label, execution, delivery string
		agentErr                                          error
	}{
		{"reply", "answer", `{"ok":true,"data":{"deliveryStatus":"delivered","openMessageId":"reply"}}`, "已完成", "success", "accepted", nil},
		{"unknown", "answer", `{"ok":true,"data":{"deliveryStatus":"unknown"}}`, "发送待确认", "success", "unknown", nil},
		{"timeout", "answer", `{"ok":false}`, "发送待确认", "success", "unknown", nil},
		{"agent-failure", "", `{"ok":true,"data":{"deliveryStatus":"delivered","openMessageId":"reply"}}`, "处理失败", "failure", "accepted", errors.New("private model failure")},
		{"empty", "", `{"ok":true,"data":{"deliveryStatus":"delivered","openMessageId":"reply"}}`, "收到", "success", "not_required", nil},
		{"cancelled", "", `{"ok":false}`, "已取消", "cancelled", "", context.Canceled},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			r.feedback = newEmployeeFeedback(context.Background(), "corp:employee", r.dir)
			var sent int
			t.Setenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE", scenario.receipt)
			testseam.Swap(t, &employeeFeedbackCall, func(context.Context, string, string, employeeEvent, employeeEmotion) (map[string]any, error) {
				return map[string]any{"success": true, "emotionId": "real-id"}, nil
			})
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				sent++
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackSubprocessFixture$")
			})
			r.fwd = employeeFeedbackForwarder(func(context.Context, string, string) (string, error) { return scenario.answer, scenario.agentErr })
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
				t.Fatal(err)
			}
			if err := r.process(e); err != nil {
				t.Fatal(err)
			}
			r.feedback.wait()
			raw, err := os.ReadFile(r.recordPath(e))
			if err != nil {
				t.Fatal(err)
			}
			var task employeeTaskRecord
			if err := json.Unmarshal(raw, &task); err != nil {
				t.Fatal(err)
			}
			if task.Execution != scenario.execution || task.Delivery != scenario.delivery {
				t.Fatalf("task=%+v", task)
			}
			raw, err = os.ReadFile(r.feedback.path(e))
			if err != nil {
				t.Fatal(err)
			}
			var feedback employeeFeedbackRecord
			if err := json.Unmarshal(raw, &feedback); err != nil {
				t.Fatal(err)
			}
			if feedback.Applied != scenario.label {
				t.Fatalf("feedback=%+v", feedback)
			}
			wantSent := 1
			if scenario.name == "cancelled" || scenario.name == "empty" {
				wantSent = 0
			}
			if sent != wantSent {
				t.Fatalf("reply calls=%d", sent)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackDoesNotBlockExecutionOrFinishBeforeReply(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.feedback = newEmployeeFeedback(context.Background(), "corp:employee", r.dir)
	feedbackStarted, releaseFeedback := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var labels []string
	testseam.Swap(t, &employeeFeedbackCall, func(_ context.Context, _, op string, _ employeeEvent, emotion employeeEmotion) (map[string]any, error) {
		if op == "add-text-emotion" {
			if emotion.Label == "排队中" {
				once.Do(func() { close(feedbackStarted) })
				<-releaseFeedback
			}
			mu.Lock()
			labels = append(labels, emotion.Label)
			mu.Unlock()
		}
		return map[string]any{"success": true, "emotionId": "real-id"}, nil
	})
	modelStarted, replyStarted, releaseReply, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan error, 1)
	r.fwd = employeeFeedbackForwarder(func(context.Context, string, string) (string, error) { close(modelStarted); return "answer", nil })
	t.Setenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE", `{"ok":true,"data":{"deliveryStatus":"delivered","openMessageId":"reply"}}`)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		close(replyStarted)
		<-releaseReply
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackSubprocessFixture$")
	})
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
		t.Fatal(err)
	}
	r.feedback.set(e, "排队中", false)
	select {
	case <-feedbackStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("no feedback")
	}
	go func() { done <- r.process(e) }()
	select {
	case <-modelStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("表情请求阻塞了执行")
	}
	<-replyStarted
	close(releaseFeedback)
	mu.Lock()
	for _, label := range labels {
		if label == "已完成" {
			t.Error("投递前标记已完成")
		}
	}
	mu.Unlock()
	close(releaseReply)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r.feedback.wait()
	mu.Lock()
	defer mu.Unlock()
	if len(labels) == 0 || labels[len(labels)-1] != "已完成" {
		t.Fatalf("labels=%v", labels)
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackRecoveryPreservesKnownFacts(t *testing.T) {
	for _, status := range []string{"accepted", "running", "sending", "delivered", "completed_without_reply"} {
		t.Run(status, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			record := employeeTaskRecord{Status: status, ConversationID: e.ConversationID, MessageID: e.MessageID, Execution: "running"}
			if status == "sending" {
				record.Execution, record.Delivery = "success", "pending"
			}
			if status == "delivered" {
				record.Execution, record.Delivery = "success", "accepted"
			}
			if status == "completed_without_reply" {
				record.Execution, record.Delivery = "success", "not_required"
			}
			if err := writeEmployeeJSON(r.recordPath(e), record); err != nil {
				t.Fatal(err)
			}
			if err := r.recoverInterruptedTasks(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(r.recordPath(e))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			wantLabel := "已中断，待核查"
			switch status {
			case "sending":
				wantLabel = "发送待确认"
				if record.Execution != "success" || record.Delivery != "unknown" {
					t.Fatalf("record=%+v", record)
				}
			case "completed_without_reply":
				wantLabel = "收到"
				if record.Execution != "success" || record.Delivery != "not_required" {
					t.Fatalf("record=%+v", record)
				}
			case "delivered":
				wantLabel = "已完成"
				if record.Status != "delivered" || record.Delivery != "accepted" {
					t.Fatalf("record=%+v", record)
				}
			default:
				if record.Execution != "unknown" {
					t.Fatalf("record=%+v", record)
				}
			}
			if employeeTaskLabel(record) != wantLabel {
				t.Fatalf("label=%s", employeeTaskLabel(record))
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeFeedbackQueuesOnlyBehindPendingTurn(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	r.ctx = ctx
	r.feedback = newEmployeeFeedback(ctx, "corp:employee", r.dir)
	defer func() { cancel(); r.wg.Wait(); r.feedback.wait() }()
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	r.fwd = employeeFeedbackForwarder(func(ctx context.Context, _, text string) (string, error) {
		entered <- struct{}{}
		if text == "third" {
			<-ctx.Done()
			return "", ctx.Err()
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return "answer", nil
		}
	})
	t.Setenv("DWS_EMPLOYEE_FEEDBACK_FIXTURE", `{"ok":true,"data":{"deliveryStatus":"delivered","openMessageId":"reply"}}`)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeFeedbackSubprocessFixture$")
	})
	labels := make(chan string, 32)
	testseam.Swap(t, &employeeFeedbackCall, func(_ context.Context, _, operation string, got employeeEvent, emotion employeeEmotion) (map[string]any, error) {
		if operation == "add-text-emotion" {
			labels <- got.MessageID + ":" + emotion.Label
		}
		return map[string]any{"success": true, "emotionId": "id-" + emotion.Label}, nil
	})
	if err := r.enqueue(e); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("首条消息未执行")
	}
	second := e
	second.EventID, second.MessageID = "event-2", "message-2"
	if err := r.enqueue(second); err != nil {
		t.Fatal(err)
	}
	seenFirst, seenSecond := false, false
	deadline := time.After(3 * time.Second)
	for !seenFirst || !seenSecond {
		select {
		case label := <-labels:
			if label == "message:排队中" {
				t.Fatal("空闲会话的首条消息被标为排队")
			}
			seenFirst = seenFirst || label == "message:思考中"
			seenSecond = seenSecond || label == "message-2:排队中"
		case <-deadline:
			t.Fatal("没有显示执行和等待的区别")
		}
	}
	close(release)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("队列未继续执行")
	}
	// 两轮都收尾后，同一会话的新消息也不应误标为排队。
	deadline = time.After(5 * time.Second)
	for {
		r.mu.Lock()
		pending := r.pending[e.ConversationID]
		r.mu.Unlock()
		if pending == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("收尾后仍计为在途任务")
		case <-time.After(5 * time.Millisecond):
		}
	}
	third := e
	third.EventID, third.MessageID, third.Content = "event-3", "message-3", "third"
	if err := r.enqueue(third); err != nil {
		t.Fatal(err)
	}
	deadline = time.After(3 * time.Second)
	for {
		select {
		case label := <-labels:
			if label == "message-3:排队中" {
				t.Fatal("已经空闲的会话仍显示排队")
			}
			if label == "message-3:思考中" {
				return
			}
		case <-deadline:
			t.Fatal("空闲会话未显示思考中")
		}
	}
}

func TestCrossPlatformCoverageEmployeeCodexEmptyCompletion(t *testing.T) {
	for _, scenario := range []struct {
		name, status, label, execution, delivery string
		sends                                    int
	}{
		{"completed", "completed", "收到", "success", "not_required", 0},
		{"failed", "failed", "处理失败", "failure", "unknown", 1},
		{"completed-with-error", "completed", "处理失败", "failure", "unknown", 1},
		{"interrupted", "interrupted", "已取消", "cancelled", "", 0},
		{"unconfirmed", "", "处理失败", "failure", "unknown", 1},
		{"disconnected", "eof", "处理失败", "failure", "unknown", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) {
				c := unitCodexClient(&bufferWriteCloser{},
					codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
					codexRPCMessage{ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"thread"}}`)},
				)
				if scenario.status == "eof" {
					close(c.msgs)
				} else {
					turn := map[string]any{"status": scenario.status}
					if scenario.name == "completed-with-error" {
						turn["error"] = map[string]any{"message": "private error"}
					}
					raw, _ := json.Marshal(map[string]any{"threadId": "thread", "turn": turn})
					c.msgs <- codexRPCMessage{Method: "turn/completed", Params: raw}
				}
				return c, nil
			})
			// 机器人仍按原契约拒绝无正文回合；仅数字员工将已确认完成转换为表情回应。
			f := &codexAppServerForwarder{bin: "fixture"}
			if _, err := f.forward(context.Background(), "conversation", "无需文字回复"); err == nil {
				t.Fatal("改变了机器人空回复契约")
			}
			r, e := employeeLedgerFixture(t)
			r.fwd = f
			r.feedback = newEmployeeFeedback(context.Background(), "corp:employee", r.dir)
			testseam.Swap(t, &employeeFeedbackCall, func(context.Context, string, string, employeeEvent, employeeEmotion) (map[string]any, error) {
				return map[string]any{"success": true, "emotionId": "real-id"}, nil
			})
			sent := 0
			testseam.Swap(t, &employeeExecCommand, func(context.Context, string, ...string) *exec.Cmd {
				sent++
				return exec.Command("missing-fixture-command")
			})
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
				t.Fatal(err)
			}
			if err := r.process(e); err != nil {
				t.Fatal(err)
			}
			r.feedback.wait()
			raw, err := os.ReadFile(r.recordPath(e))
			if err != nil {
				t.Fatal(err)
			}
			var record employeeTaskRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			if record.Execution != scenario.execution || record.Delivery != scenario.delivery || employeeTaskLabel(record) != scenario.label || sent != scenario.sends {
				t.Fatalf("record=%+v label=%s sent=%d", record, employeeTaskLabel(record), sent)
			}
			raw, err = os.ReadFile(r.feedback.path(e))
			if err != nil {
				t.Fatal(err)
			}
			var feedback employeeFeedbackRecord
			if err := json.Unmarshal(raw, &feedback); err != nil {
				t.Fatal(err)
			}
			if feedback.Applied != scenario.label {
				t.Fatalf("feedback=%+v", feedback)
			}
		})
	}
}
