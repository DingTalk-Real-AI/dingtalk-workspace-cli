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
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// Apply replacement semantics, including parent references, rather than merely
// concatenating every value ever received (which hides lost-visible-text bugs).
type employeeCardTestSurface struct {
	data       map[string]string
	components map[string]map[string]any
}

func newEmployeeCardTestSurface() *employeeCardTestSurface {
	return &employeeCardTestSurface{data: map[string]string{}, components: map[string]map[string]any{}}
}
func (s *employeeCardTestSurface) apply(messages []map[string]any) {
	for _, m := range messages {
		if d, ok := m["updateDataModel"].(map[string]any); ok {
			s.data[d["path"].(string)] = d["value"].(string)
		}
		if u, ok := m["updateComponents"].(map[string]any); ok {
			for _, c := range u["components"].([]map[string]any) {
				s.components[c["id"].(string)] = c
			}
		}
	}
}
func (s *employeeCardTestSurface) text(id string) string {
	c := s.components[id]
	if content, ok := c["content"].(map[string]any); ok {
		return s.data[content["path"].(string)]
	}
	var text strings.Builder
	if children, ok := c["children"].([]string); ok {
		for _, child := range children {
			text.WriteString(s.text(child))
		}
	}
	return text.String()
}

func TestCrossPlatformCoverageEmployeeCardRevisionPublication(t *testing.T) {
	for _, old := range []string{"已展示的短正文", strings.Repeat(`"`, 17000)} {
		for _, failure := range []string{"success", "card_write_rejected", "card_write_unconfirmed", "card_payload_too_large"} {
			t.Run(failure+"_"+old[:3], func(t *testing.T) {
				r, e := employeeLedgerFixture(t)
				surface := newEmployeeCardTestSurface()
				previewDone := make(chan struct{})
				previewPublished, finalCalls := false, 0
				// Deliberately replace the prefix, too: staging must neither shorten nor
				// mix new text into a previous complete revision if a later batch fails.
				answer := strings.Repeat("新正文", 12000)
				testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
					if args[0] == "send-a2ui-card" {
						surface.apply(messages)
						return map[string]any{"bizId": "real"}, nil
					}
					if previewPublished {
						finalCalls++
						if finalCalls == 2 && failure != "success" {
							return nil, employeeTerminal(failure)
						}
						if surface.text("root") != old && args[4] != "FINISH" {
							t.Fatal("staging changed the visible previous answer")
						}
					}
					surface.apply(messages)
					if !previewPublished && surface.text("root") == old {
						previewPublished = true
						close(previewDone)
					}
					return map[string]any{"flowStatus": args[4]}, nil
				})
				rec := employeeTaskRecord{}
				err := r.processCard(e, &rec, &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
					emit(agentProgress{ID: "a", Kind: "answer", Text: old})
					select {
					case <-previewDone:
					case <-time.After(5 * time.Second):
						t.Error("preview did not publish")
					}
					return answer, nil
				}})
				if err != nil {
					t.Fatal(err)
				}
				want := old
				if failure == "success" {
					want = answer
					if rec.Status != "card_finished" {
						t.Fatal(rec)
					}
				} else if failure == "card_payload_too_large" {
					if rec.Status != "card_incomplete" || rec.FlowStatus != "FINISH" {
						t.Fatal(rec)
					}
				} else if rec.Status != "needs_review" || finalCalls != 2 {
					t.Fatalf("unexpected retry/status: %d %+v", finalCalls, rec)
				}
				if surface.text("root") != want {
					t.Fatalf("visible answer lost or mixed: got %d bytes, want %d", len(surface.text("root")), len(want))
				}
			})
		}
	}
}

func TestCrossPlatformCoverageEmployeeCardFailedPreviewIsNotGenerating(t *testing.T) {
	for _, agentErr := range []error{errors.New("agent failed"), context.Canceled, context.DeadlineExceeded, nil} {
		t.Run(fmt.Sprint(agentErr), func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			surface := newEmployeeCardTestSurface()
			previewDone := make(chan struct{})
			previewPublished := false
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
				surface.apply(messages)
				if args[0] == "send-a2ui-card" {
					return map[string]any{"bizId": "real"}, nil
				}
				if !previewPublished && strings.Contains(surface.text("answer"), "当前仅展示部分正文") {
					previewPublished = true
					close(previewDone)
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			rec := employeeTaskRecord{}
			err := r.processCard(e, &rec, &employeeProgressFixture{run: func(emit func(agentProgress)) (string, error) {
				emit(agentProgress{ID: "a", Kind: "answer", Text: strings.Repeat("x", 40000)})
				select {
				case <-previewDone:
				case <-time.After(5 * time.Second):
					t.Error("preview did not publish")
				}
				return "", agentErr
			}})
			if err != nil {
				t.Fatal(err)
			}
			if rec.Status != "card_incomplete" || rec.FlowStatus != "FINISH" {
				t.Fatal(rec)
			}
			if !strings.HasPrefix(surface.text("answer"), strings.Repeat("x", agentProgressPreviewBytes)) {
				t.Fatal("terminal erased preview")
			}
			if strings.Contains(surface.text("answer"), "生成中") || strings.Contains(surface.text("answer"), "展示全文") {
				t.Fatal("terminal promises future output")
			}
			if strings.Contains(surface.components["execution"]["title"].(string), "AIGenerating") {
				t.Fatal("terminal still loading")
			}
		})
	}
}
func TestCrossPlatformCoverageEmployeeCardPayloadBoundaries(t *testing.T) {
	// Force a boundary inside a multibyte rune; reconstruct the original exactly.
	answer := "x" + strings.Repeat("字", 20000)
	parts := employeeCardAnswerParts(answer)
	if strings.Join(parts, "") != answer || len(parts) < 2 {
		t.Fatal("text loss")
	}
	for _, part := range parts {
		if !utf8.ValidString(part) {
			t.Fatal("invalid UTF-8")
		}
	}
	// Invalid envelope size must fail preflight before any batch can be sent.
	v := employeeCardView{answer: "answer"}
	if batches, err := v.batches(strings.Repeat("s", employeeCardPayloadLimit), "done", false); err == nil || batches != nil {
		t.Fatal("oversized envelope accepted")
	}
	// Retain the defensive bound for a reconstructed/legacy view as well.
	v = employeeCardView{items: make([]agentProgress, 40)}
	for i := range v.items {
		v.items[i] = agentProgress{Kind: "progress", Text: fmt.Sprint(i)}
	}
	surface := newEmployeeCardTestSurface()
	surface.apply(v.messages("s", "done", false))
	if _, ok := surface.components["step-7"]; ok {
		t.Fatal("old progress retained")
	}
	if _, ok := surface.components["step-39"]; !ok {
		t.Fatal("recent progress lost")
	}
}

func TestCrossPlatformCoverageEmployeeCardCompactTerminalPersistence(t *testing.T) {
	for _, mode := range []string{"ledger", "alert"} {
		t.Run(mode, func(t *testing.T) {
			r, e := employeeLedgerFixture(t)
			calls := 0
			testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, _ []map[string]any, args ...string) (map[string]any, error) {
				calls++
				if calls == 1 {
					return map[string]any{"bizId": "real"}, nil
				}
				if calls == 2 {
					if mode == "alert" {
						if err := os.Mkdir(filepath.Join(r.dir, "runtime.log"), 0700); err != nil {
							t.Fatal(err)
						}
					}
					return nil, employeeTerminal("card_payload_too_large")
				}
				return map[string]any{"flowStatus": args[4]}, nil
			})
			testseam.Swap(t, &atomicRename, func(from, to string) error {
				if mode == "ledger" && calls == 2 {
					return errors.New("disk unavailable")
				}
				return os.Rename(from, to)
			})
			rec := employeeTaskRecord{}
			err := r.processCard(e, &rec, &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { return "answer", nil }})
			want := "ledger_unavailable"
			wantCalls := 2
			if mode == "alert" {
				want = "card_alert_unavailable"
				wantCalls = 3
			}
			if err == nil || err.Error() != want || calls != wantCalls {
				t.Fatalf("err=%v calls=%d record=%+v", err, calls, rec)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCardLargeFinalPreflightFailure(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	calls := 0
	testseam.Swap(t, &employeeA2UICall, func(_ context.Context, _ digitalEmployeeBinding, messages []map[string]any, args ...string) (map[string]any, error) {
		calls++
		if calls == 1 {
			return map[string]any{"bizId": "real"}, nil
		}
		if args[4] != "FINISH" {
			t.Fatal("partially delivered a locally rejected batch sequence")
		}
		raw, _ := json.Marshal(messages)
		if strings.Contains(string(raw), "updateDataModel") {
			t.Fatal("compact terminal overwrote existing answer")
		}
		return map[string]any{"flowStatus": "FINISH"}, nil
	})
	rec := employeeTaskRecord{}
	err := r.processCard(e, &rec, &employeeProgressFixture{run: func(func(agentProgress)) (string, error) { return strings.Repeat(`"`, 24*1024*1024), nil }})
	if err != nil || calls != 2 || rec.Status != "card_incomplete" || rec.CardErrorCode != "card_payload_too_large" {
		t.Fatalf("%v calls=%d %+v", err, calls, rec)
	}
}

func TestCrossPlatformCoverageEmployeeCardProcessPropagatesFailure(t *testing.T) {
	r, e := employeeLedgerFixture(t)
	r.cfg.ReplyMode = "a2ui"
	r.fwd = &employeeProgressFixture{run: func(func(agentProgress)) (string, error) {
		t.Fatal("model started after ledger failure")
		return "", nil
	}}
	if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "accepted"}); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &atomicRename, func(string, string) error { return errors.New("disk unavailable") })
	if err := r.process(e); err == nil || err.Error() != "ledger_unavailable" {
		t.Fatalf("lost failure: %v", err)
	}
}

func TestCrossPlatformCoverageCodexRepeatedStartKeepsDelta(t *testing.T) {
	p := codexPublicProgress{}
	p.handle("item/started", json.RawMessage(`{}`), "thread")
	var got []agentProgress
	p.emit = func(v agentProgress) { got = append(got, v) }
	start := json.RawMessage(`{"threadId":"thread","item":{"id":"a","type":"agentMessage","phase":"final_answer"}}`)
	p.handle("item/started", start, "thread")
	p.handle("item/agentMessage/delta", json.RawMessage(`{"threadId":"thread","itemId":"a","delta":"完整"}`), "thread")
	p.handle("item/started", start, "thread")
	p.handle("item/agentMessage/delta", json.RawMessage(`{"threadId":"thread","itemId":"a","delta":"段落\n\n"}`), "thread")
	if len(p.items) != 1 || len(p.order) != 1 || len(got) != 1 || got[0].Text != "完整段落" {
		t.Fatalf("duplicate start discarded or duplicated text: %+v", got)
	}
}
