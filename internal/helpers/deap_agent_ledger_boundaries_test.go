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
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeLedgerFixture(t *testing.T) (*employeeRuntime, employeeEvent) {
	t.Helper()
	r := &employeeRuntime{dir: t.TempDir(), ctx: context.Background(), fwd: &employeeBoundaryForwarder{}, queues: map[string]chan employeeEvent{}, fatal: make(chan error, 1)}
	r.cfg.Options.AllowedUsers = []string{"sender"}
	e := employeeEvent{Type: "user_im_message_receive_o2o_all", EventID: "event", MessageID: "message", ConversationID: "conversation", SenderID: "sender", Content: "private input"}
	return r, e
}

func TestCrossPlatformCoverageEmployeeQueueCapacityAndPersistence(t *testing.T) {
	for _, scenario := range []string{"rejected", "audit", "ledger", "conversation-capacity", "queue-capacity", "fatal", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() { cancel(); r.wg.Wait() }()
			r.ctx = ctx
			switch scenario {
			case "rejected":
				e.Type = "unknown"
			case "audit":
				if err := os.Mkdir(filepath.Join(r.dir, "audit.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			case "ledger":
				testseam.Swap(t, &atomicRename, func(string, string) error { return errors.New("ledger unavailable") })
			case "conversation-capacity":
				for i := 0; i < 128; i++ {
					r.queues[fmt.Sprint(i)] = make(chan employeeEvent, 1)
				}
			case "queue-capacity":
				q := make(chan employeeEvent, 1)
				q <- e
				r.queues[e.ConversationID] = q
			case "fatal":
				writes := 0
				testseam.Swap(t, &atomicRename, func(src, dst string) error {
					writes++
					if writes > 1 {
						return errors.New("ledger unavailable")
					}
					return os.Rename(src, dst)
				})
			case "cancelled":
				cancel()
			}
			err := r.enqueue(e)
			wantError := scenario != "rejected" && scenario != "fatal" && scenario != "cancelled"
			if (err != nil) != wantError {
				t.Fatalf("enqueue=%v", err)
			}
			if scenario == "fatal" {
				select {
				case err := <-r.fatal:
					if err == nil {
						t.Fatal("nil failure")
					}
				case <-time.After(time.Second):
					t.Fatal("worker did not report persistence failure")
				}
			}
			cancel()
			r.wg.Wait()
		})
	}
}

func TestCrossPlatformCoverageEmployeeTaskFailureStates(t *testing.T) {
	for _, scenario := range []string{"cancelled", "missing", "corrupt", "agent-failed", "empty", "sending-write", "send-failed", "audit", "final-write"} {
		t.Run(scenario, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			path := r.recordPath(e)
			if scenario != "missing" {
				if err := writeEmployeeJSON(path, employeeTaskRecord{Status: "accepted", IdempotencyKey: "stable"}); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				r.ctx = ctx
			case "corrupt":
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "agent-failed":
				r.fwd = &employeeBoundaryForwarder{err: errors.New("private model error")}
			case "sending-write", "send-failed":
				r.fwd = &employeeBoundaryForwarder{answer: "private answer"}
			case "audit":
				if err := os.Mkdir(filepath.Join(r.dir, "audit.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "sending-write" || scenario == "final-write" {
				testseam.Swap(t, &atomicRename, func(string, string) error { return errors.New("ledger write failed") })
			}
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, filepath.Join(r.dir, "missing-binary"))
			})
			err := r.process(e)
			wantError := scenario == "missing" || scenario == "corrupt" || scenario == "sending-write" || scenario == "audit" || scenario == "final-write"
			if (err != nil) != wantError {
				t.Fatalf("process=%v", err)
			}
			if err == nil && scenario != "cancelled" {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var record employeeTaskRecord
				if err := json.Unmarshal(raw, &record); err != nil {
					t.Fatal(err)
				}
				want := "empty_reply"
				if scenario == "agent-failed" {
					want = "agent_failed"
				}
				if scenario == "send-failed" {
					want = "needs_review"
				}
				if record.Status != want {
					t.Fatalf("record=%+v", record)
				}
				if strings.Contains(string(raw), "private") {
					t.Fatal("task record leaked content")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCapacityRejectionAllowsRedelivery(t *testing.T) {
	// A capacity rejection must not leave a dedup record behind; otherwise the
	// Event Bus redelivery of the same event would be short-circuited as
	// already handled and the user's message would be permanently lost.
	t.Run("queue-capacity", func(t *testing.T) {
		r, e := employeeLedgerFixture(t)
		r.ctx = context.Background()
		// Occupy the only queue slot with an unrelated event so the target
		// event is rejected purely on single-queue capacity.
		q := make(chan employeeEvent, 1)
		other := e
		other.MessageID = "other"
		q <- other
		r.queues[e.ConversationID] = q
		if err := r.enqueue(e); err == nil {
			t.Fatal("expected queue_capacity rejection")
		}
		if _, err := os.Stat(r.recordPath(e)); !os.IsNotExist(err) {
			t.Fatalf("rejected event must not leave a dedup record: %v", err)
		}
		// Drain to relieve backpressure, then redeliver: the event must now be
		// accepted and persisted rather than silently deduped away.
		<-q
		if err := r.enqueue(e); err != nil {
			t.Fatalf("redelivery after capacity relief must be accepted: %v", err)
		}
		if _, err := os.Stat(r.recordPath(e)); err != nil {
			t.Fatalf("redelivered event must persist a dedup record: %v", err)
		}
	})
	t.Run("conversation-capacity", func(t *testing.T) {
		r, e := employeeLedgerFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		r.ctx = ctx
		defer func() { cancel(); r.wg.Wait() }()
		for i := 0; i < 128; i++ {
			r.queues[fmt.Sprint(i)] = make(chan employeeEvent, 1)
		}
		if err := r.enqueue(e); err == nil {
			t.Fatal("expected conversation_capacity rejection")
		}
		if _, err := os.Stat(r.recordPath(e)); !os.IsNotExist(err) {
			t.Fatalf("rejected event must not leave a dedup record: %v", err)
		}
		// Free a conversation slot, then redeliver: the event must now be
		// accepted and persisted rather than silently deduped away.
		delete(r.queues, "0")
		if err := r.enqueue(e); err != nil {
			t.Fatalf("redelivery after capacity relief must be accepted: %v", err)
		}
		if _, err := os.Stat(r.recordPath(e)); err != nil {
			t.Fatalf("redelivered event must persist a dedup record: %v", err)
		}
	})
}

func TestCrossPlatformCoverageEmployeeMachinePayloadAndAuditEncoding(t *testing.T) {
	for _, payload := range []any{make(chan int), strings.Repeat("x", digitalEmployeeStdinLimit+1)} {
		if _, err := employeeMachineCall(context.Background(), "corp:employee", payload); err == nil || err.Error() != "invalid_payload" {
			t.Fatalf("payload error=%v", err)
		}
	}
	r, _ := employeeLedgerFixture(t)
	if err := r.audit(employeeTaskRecord{UpdatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("invalid audit timestamp accepted")
	}
}
