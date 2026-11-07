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
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageA2UIStdinInput(t *testing.T) {
	for _, tc := range []struct {
		raw, input string
		ok         bool
	}{
		{`["{\"version\":\"v1.0\"}"]`, "", true},
		{"-", "\ufeff[\"{}\"]", true},
		{"-", "[]", false},
		{"-", strings.Repeat("x", 10*1024*1024+1), false},
	} {
		cmd := &cobra.Command{}
		cmd.Flags().String("content", tc.raw, "")
		cmd.SetIn(strings.NewReader(tc.input))
		_, err := readA2UIMessages(cmd)
		if (err == nil) != tc.ok {
			t.Fatalf("stdin validation: %v", err)
		}
	}
}

func TestCrossPlatformCoverageEmployeeCardTimeoutCancelsPendingWrite(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	started := make(chan struct{})
	calls := 0
	testseam.Swap(t, &employeeA2UICall, func(ctx context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
		calls++
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "real"}, nil
		}
		close(started)
		<-ctx.Done()
		return nil, employeeTerminal("card_write_unconfirmed")
	})
	f := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		emit(agentProgress{ID: "p", Kind: "progress", Text: "阶段"})
		<-started
		return "", context.DeadlineExceeded
	}}
	record := employeeTaskRecord{}
	if err := r.processCard(e, &record, f); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || record.Status != "needs_review" || record.CardErrorCode != "card_write_unconfirmed" {
		t.Fatalf("timeout retried/finalized unknown write: %+v %d", record, calls)
	}
}

func TestCrossPlatformCoverageEmployeeCardRuntimeCancellation(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	r.ctx = ctx
	calls := 0
	testseam.Swap(t, &employeeA2UICall, func(ctx context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
		calls++
		if ctx.Err() != nil {
			return nil, employeeTerminal("card_write_unconfirmed")
		}
		if args[0] != "send-a2ui-card" {
			t.Fatal("runtime cancellation submitted a card update")
		}
		return map[string]any{"bizId": "real"}, nil
	})
	f := &employeeProgressFixture{run: func(func(agentProgress)) (string, error) {
		cancel()
		// Simulate an adapter finishing cancellation after the idle consumer exits.
		<-time.After(100 * time.Millisecond)
		return "", context.Canceled
	}}
	record := employeeTaskRecord{}
	if err := r.processCard(e, &record, f); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || record.Status != "needs_review" || record.FlowStatus != "PROCESSING" || record.CardErrorCode != "card_write_unconfirmed" {
		t.Fatalf("cancelled runtime finalized or lost unknown status: %+v %d", record, calls)
	}
}

func TestCrossPlatformCoverageEmployeeCardAlertFailures(t *testing.T) {
	for _, mode := range []string{"marshal", "open", "write"} {
		t.Run(mode, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			switch mode {
			case "marshal":
				testseam.Swap(t, &employeeMarshalJSON, func(any) ([]byte, error) { return nil, errors.New("unavailable") })
			case "open":
				if err := os.Mkdir(filepath.Join(r.dir, "runtime.log"), 0700); err != nil {
					t.Fatal(err)
				}
			case "write":
				testseam.Swap(t, &employeeOpenFile, func(string, int, os.FileMode) (*os.File, error) {
					f, err := os.CreateTemp(r.dir, "closed")
					if err == nil {
						f.Close()
					}
					return f, err
				})
			}
			if err := r.cardFailure(e, &employeeTaskRecord{}, "create_unconfirmed", errors.New("private")); err == nil || err.Error() != "card_alert_unavailable" {
				t.Fatalf("missing alert failure: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardFailureClassificationAndAlerts(t *testing.T) {
	for _, tc := range []struct{ stdout, stderr, code string }{
		{"", `{"error":{"category":"auth","message":"private token"}}`, "card_write_rejected"},
		{"", `{"error":{"category":"validation"}}`, "card_write_rejected"},
		{"", `{"error":{"execution_started":false}}`, "card_write_rejected"},
		{`{"success":false}`, "", "card_response_failed"},
		{"", `{"error":{"category":"network"}}`, "card_write_unconfirmed"},
		{"", `{"error":{"category":"server"}}`, "card_response_failed"},
		{"", `{"success":false}`, "card_response_failed"},
		{"invalid", "private output", "card_write_unconfirmed"},
	} {
		err := employeeCardResponseError([]byte(tc.stdout), []byte(tc.stderr))
		if err.Error() != tc.code {
			t.Fatalf("classification=%v want=%s", err, tc.code)
		}
	}
	r, e := employeeLedgerFixture(t)
	for i := 0; i < 3; i++ {
		record := employeeTaskRecord{}
		if err := r.cardFailure(e, &record, "create_unconfirmed", employeeTerminal("card_write_rejected")); err != nil {
			t.Fatal(err)
		}
		if record.Status != "card_rejected" || record.CardErrorCode != "card_write_rejected" {
			t.Fatalf("lost rejection: %+v", record)
		}
	}
	data, err := os.ReadFile(filepath.Join(r.dir, "runtime.log"))
	if err != nil || !strings.Contains(string(data), "card_reply_continuous_failures") || strings.Count(string(data), "\n") != 2 {
		t.Fatalf("missing alert: %s %v", data, err)
	}
	if strings.Contains(string(data), e.ConversationID) || strings.Contains(string(data), "private") {
		t.Fatal("alert leaked private data")
	}
}

func TestCrossPlatformCoverageEmployeeCardCancelledCLIWrite(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCardOutputFixture$", "--", "employee-card-output")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runEmployeeA2UICall(ctx, cfg.Binding, []map[string]any{{"version": "v1.0"}}, "send-a2ui-card")
	if err == nil || err.Error() != "card_write_unconfirmed" {
		t.Fatalf("cancelled write guessed rejection: %v", err)
	}
}

func TestCrossPlatformCoverageEmployeeCardProgressBackpressure(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	final := ""
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "real"}, nil
		}
		if args[4] == "INPUTTING" {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
		} else {
			data, _ := json.Marshal(messages)
			final = string(data)
		}
		return map[string]any{"flowStatus": args[4]}, nil
	})
	f := &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		emit(agentProgress{ID: "first", Kind: "progress", Text: "开始"})
		<-started
		for i := 0; i < 256; i++ {
			emit(agentProgress{ID: "latest", Kind: "progress", Text: fmt.Sprintf("阶段%d", i)})
		}
		close(release)
		return "最终答案", nil
	}}
	record := employeeTaskRecord{}
	if err := r.processCard(e, &record, f); err != nil {
		t.Fatal(err)
	}
	if record.Status != "card_finished" || !strings.Contains(final, "阶段255") || !strings.Contains(final, "最终答案") {
		t.Fatalf("backpressure lost latest/final state: %+v", record)
	}
}

func TestCrossPlatformCoverageEmployeeCardLongPayloadUsesStdin(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	t.Setenv("DWS_EMPLOYEE_CARD_TEST_STDIN", "1")
	t.Setenv("DWS_EMPLOYEE_CARD_TEST_BODY", `{"success":true,"result":{"bizId":"real"}}`)
	testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(strings.Join(args, " ")) > 1000 {
			t.Fatal("long payload entered command line")
		}
		for i, arg := range args {
			if arg == "--content" && args[i+1] != "-" {
				t.Fatal("stdin content flag missing")
			}
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeCardOutputFixture$", "--", "employee-card-output")
	})
	_, err := runEmployeeA2UICall(context.Background(), cfg.Binding, []map[string]any{{"text": strings.Repeat("x", 35000)}}, "send-a2ui-card", "--open-dingtalk-id", "owner")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeCardSlowWriteDoesNotBlockAgent(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "a2ui"
	release := make(chan struct{})
	advanced := make(chan struct{})
	started := make(chan struct{})
	testseam.Swap(t, &employeeA2UICall, func(ctx context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
		if args[0] == "send-a2ui-card" {
			return map[string]any{"bizId": "real"}, nil
		}
		if args[4] == "INPUTTING" {
			select {
			case <-started:
			default:
				close(started)
			}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return map[string]any{"flowStatus": args[4]}, nil
	})
	r.fwd = &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
		emit(agentProgress{ID: "p", Kind: "progress", Text: "公开进度"})
		close(advanced)
		<-started
		return "最终答案", nil
	}}
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted"}); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- r.process(e) }()
	select {
	case <-advanced:
	case <-time.After(time.Second):
		t.Error("slow card update blocked the model progress callback")
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.recordPath(e))
	if err != nil {
		t.Fatal(err)
	}
	var record employeeTaskRecord
	if json.Unmarshal(data, &record) != nil || record.Status != "card_finished" {
		t.Fatalf("record: %s", data)
	}
}
