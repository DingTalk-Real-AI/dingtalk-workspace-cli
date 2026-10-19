// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeCardRealErrorEnvelope(t *testing.T) {
	for _, category := range []string{"auth", "validation", "api"} {
		for _, started := range []bool{false, true} {
			var err error
			switch category {
			case "auth":
				err = apperrors.NewAuth("fixture", apperrors.WithExecutionStarted(started))
			case "validation":
				err = apperrors.NewValidation("fixture", apperrors.WithExecutionStarted(started))
			default:
				err = apperrors.NewAPI("fixture", apperrors.WithExecutionStarted(started))
			}
			var stderr bytes.Buffer
			if err := apperrors.PrintJSON(&stderr, err); err != nil {
				t.Fatal(err)
			}
			want := "card_write_rejected"
			if started {
				want = "card_write_unconfirmed"
			}
			if got := employeeCardResponseError(nil, stderr.Bytes()); got.Error() != want {
				t.Fatalf("%s started=%v: %v", category, started, got)
			}
		}
	}
}

func TestCrossPlatformCoverageEmployeeLegacyReplyMode(t *testing.T) {
	for _, mode := range []string{"", "text", "a2ui"} {
		t.Run("saved_"+mode, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			cfg.ReplyMode = mode
			path := filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json")
			if err := writeEmployeeJSON(path, cfg); err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(path)
			if mode == "" && bytes.Contains(raw, []byte("replyMode")) {
				t.Fatal("not a legacy fixture")
			}
			loaded, err := loadDigitalEmployeeConfig(cfg.Binding.DWSProfile)
			if err != nil {
				t.Fatal(err)
			}
			want := mode
			if want == "" {
				want = "text"
			}
			if loaded.ReplyMode != want {
				t.Fatalf("mode=%q", loaded.ReplyMode)
			}
			r, e := employeeLedgerFixture(t)
			r.cfg = loaded
			r.cfg.Options.AllowedUsers = []string{"sender"}
			cardCalls := 0
			testseam.Swap(t, &employeeA2UICall, func(context.Context, digitalEmployeeBinding, []map[string]any, ...string) (map[string]any, error) {
				cardCalls++
				return map[string]any{"bizId": "real"}, nil
			})
			r.fwd = &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { return "answer", nil }}
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{}); err != nil {
				t.Fatal(err)
			}
			if err := r.process(e); err != nil {
				t.Fatal(err)
			}
			if (cardCalls > 0) != (mode == "a2ui") {
				t.Fatalf("old configuration switched mode: %d", cardCalls)
			}
		})
	}
	if got := newDeapConnectCommand().Flags().Lookup("reply-mode").DefValue; got != "a2ui" {
		t.Fatal(got)
	}
}

func TestCrossPlatformCoverageEmployeeCardReceiptIdentity(t *testing.T) {
	for _, raw := range []any{nil, float64(42), "<bizId>", "a b", "a\tb", "", " valid-id "} {
		t.Run(fmt.Sprint(raw), func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			calls, modelCalls := 0, 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if calls > 1 && args[2] != "valid-id" {
					t.Fatalf("not normalized: %v", args)
				}
				return map[string]any{"bizId": raw}, nil
			})
			rec := employeeTaskRecord{}
			if err := r.processCard(e, &rec, &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { modelCalls++; return "answer", nil }}); err != nil {
				t.Fatal(err)
			}
			if raw == " valid-id " {
				if modelCalls != 1 || rec.Status != "card_finished" {
					t.Fatalf("%+v", rec)
				}
			} else if calls != 1 || modelCalls != 0 || rec.Status != "needs_review" {
				t.Fatalf("invalid identity started model: %+v", rec)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardLongAnswerDelivery(t *testing.T) {
	protocol, err := aicardProtocol()
	if err != nil {
		t.Fatal(err)
	}
	for _, answer := range []string{strings.Repeat(`"`, 17000), strings.Repeat("字", 22000), strings.Repeat("段落 <>& \\\n\n", 10000)} {
		t.Run(fmt.Sprint(len(answer)), func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			calls := 0
			var history []string
			surface := newEmployeeCardTestSurface()
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				encoded, err := encodeEmployeeCardMessages(messages)
				if err != nil || len(encoded) > employeeCardPayloadLimit {
					t.Fatalf("oversized batch: %v", err)
				}
				surface.apply(messages)
				for _, message := range messages {
					raw, _ := json.Marshal(message)
					history = append(history, string(raw))
				}
				snapshot := []byte("[" + strings.Join(history, ",") + "]")
				report, err := protocol.Lint(snapshot, false, false)
				if err != nil || !report.Valid {
					t.Fatalf("invalid protocol: %+v %v", report, err)
				}
				preflight, err := protocol.Preflight(history, "new-card")
				if err != nil || preflight["valid"] != true {
					t.Fatalf("broken references: %+v %v", preflight, err)
				}
				if args[0] == "send-a2ui-card" {
					return map[string]any{"bizId": "real"}, nil
				}
				if args[4] == "FINISH" && surface.text("answer") != answer {
					t.Fatal("FINISH preceded complete delivery")
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			rec := employeeTaskRecord{}
			if err := r.processCard(e, &rec, &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { return answer, nil }}); err != nil {
				t.Fatal(err)
			}
			if calls < 3 || rec.FlowStatus != "FINISH" || rec.Status != "card_finished" {
				t.Fatalf("missing final delivery: %+v", rec)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardChunkFailureStops(t *testing.T) {
	for _, failure := range []string{"unknown", "local", "terminal-unknown", "terminal-status"} {
		t.Run(failure, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			calls := 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if calls == 1 {
					return map[string]any{"bizId": "real"}, nil
				}
				if calls == 3 {
					if failure == "unknown" {
						return nil, employeeTerminal("card_write_unconfirmed")
					}
					return nil, employeeTerminal("card_payload_too_large")
				}
				if calls == 4 {
					if args[4] != "FINISH" {
						t.Fatal("retried oversized content")
					}
					raw, _ := json.Marshal(messages)
					if strings.Contains(string(raw), "updateDataModel") || strings.Contains(string(raw), "AIGenerating") || !strings.Contains(string(raw), "完整结果未交付") {
						t.Fatalf("unsafe terminal: %s", raw)
					}
					if failure == "terminal-unknown" {
						return nil, employeeTerminal("card_write_unconfirmed")
					}
					if failure == "terminal-status" {
						return map[string]any{"flowStatus": "INPUTTING"}, nil
					}
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			rec := employeeTaskRecord{}
			if err := r.processCard(e, &rec, &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { return strings.Repeat("x", 100000), nil }}); err != nil {
				t.Fatal(err)
			}
			if failure == "unknown" {
				if calls != 3 || rec.Status != "needs_review" {
					t.Fatalf("replayed unknown write: %+v", rec)
				}
			} else if calls != 4 {
				t.Fatal(calls)
			}
			if failure == "local" && (rec.Status != "card_incomplete" || rec.FlowStatus != "FINISH" || rec.CardErrorCode != "card_payload_too_large") {
				t.Fatalf("local rejection failed to close: %+v", rec)
			}
			if strings.HasPrefix(failure, "terminal-") && rec.Status != "needs_review" {
				t.Fatalf("unknown terminal marked complete: %+v", rec)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardBoundedState(t *testing.T) {
	v := employeeCardView{}
	for i := 0; i < 4096; i++ {
		v.accept(agentProgress{ID: fmt.Sprint(i), Kind: "progress", Text: strings.Repeat("字", 2000)})
	}
	if len(v.items) != 32 || cap(v.items) > 32 {
		t.Fatalf("unbounded view %d/%d", len(v.items), cap(v.items))
	}
	v.accept(agentProgress{ID: "4095", Kind: "progress", Text: "updated"})
	if v.items[31].Text != "updated" {
		t.Fatal("visible item update lost")
	}
	for _, p := range v.items {
		if len(p.Text) > 259 || !utf8.ValidString(p.Text) {
			t.Fatal("unbounded or invalid text")
		}
	}
	raw, _ := json.Marshal(v.messages("s", "running", true))
	if !strings.Contains(string(raw), `"id":"step-4095"`) {
		t.Fatal("component identity changed")
	}
	v.accept(agentProgress{ID: "a", Kind: "answer", Text: strings.Repeat("字", 20000)})
	if len(v.answer) > agentProgressPreviewBytes+100 || !strings.Contains(v.answer, "当前仅展示部分正文") {
		t.Fatal("unbounded or silent preview truncation")
	}
}

func TestCrossPlatformCoverageA2UIInputErrorClassification(t *testing.T) {
	for _, command := range []string{"send-a2ui-card", "update-a2ui-card"} {
		for _, raw := range []string{"[]", "{", "null", "123", ""} {
			for _, stdin := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s_%s_stdin%v", command, raw, stdin), func(t *testing.T) {
					caller := &scriptedToolCaller{}
					installScriptedCaller(t, caller)
					root := newChatCommand()
					root.SilenceErrors = true
					root.SilenceUsage = true
					content := raw
					if stdin {
						content = "-"
						root.SetIn(strings.NewReader(raw))
					}
					args := []string{"message", command, "--content", content}
					if command == "send-a2ui-card" {
						args = append(args, "--conversation-id", "fixture")
					} else {
						args = append(args, "--biz-id", "real", "--flow-status", "FINISH")
					}
					root.SetArgs(args)
					err := corecmd.ExecuteForTest(root)
					var classified *apperrors.Error
					if !errors.As(err, &classified) || apperrors.ExitCode(err) != 3 {
						t.Fatalf("wrong validation classification: %T %v", err, err)
					}
					if caller.calls != 0 {
						t.Fatal("invalid input reached transport")
					}
				})
			}
		}
	}
}
