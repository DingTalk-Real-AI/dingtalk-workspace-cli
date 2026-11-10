// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

var recordFieldCommands = []string{"+record-update", "+record-upsert", "+record-batch-create", "+record-bulk-patch", "+record-upsert-by-key"}

func runRecordFieldCLI(t *testing.T, caller *upsertByKeyCaller, command string, cells map[string]any, extra ...string) (string, error) {
	t.Helper()
	switch command {
	case "+record-upsert-by-key":
		return runUpsertByKeyCLI(t, caller, append([]string{"--cells", mustJSONText(t, cells)}, extra...)...)
	case "+record-bulk-patch":
		return runAITableCompositeCLI(t, caller, command, append([]string{"--base-id", "base", "--table-id", "table", "--record-ids", "r1,r2", "--patch", mustJSONText(t, cells), "--yes"}, extra...)...)
	default:
		records := []map[string]any{{"cells": cells}, {"cells": cells}}
		if command == "+record-update" {
			for i := range records {
				records[i]["recordId"] = []string{"r1", "r2"}[i]
			}
		}
		return runRecordBatchCLI(t, caller, command, records, extra...)
	}
}

func TestCrossPlatformCoverageRecordFieldsOnlyLoadAfterReadback(t *testing.T) {
	for _, command := range recordFieldCommands {
		for _, mode := range []string{"names", "ids", "aliases", "typed ids"} {
			t.Run(command+"/"+mode, func(t *testing.T) {
				cells := map[string]any{"标题": "任务", "金额": float64(42)}
				if mode == "ids" || mode == "typed ids" {
					cells = map[string]any{"F": "任务", "A": float64(42)}
				} else if mode == "aliases" {
					cells["F"] = "任务"
				}
				before := mustJSONText(t, cells)
				writes, reads, queries := 0, 0, 0
				caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
					switch tool {
					case "get_fields":
						reads++
						if writes != 1 || queries == 0 || mode == "ids" {
							t.Fatal("unnecessary or prewrite metadata lookup", tool, writes, queries, mode)
						}
						return `{"fields":[{"fieldId":"F","fieldName":"标题"},{"fieldId":"A","fieldName":"金额","type":"number"},{"fieldId":"fldKey","fieldName":"业务键","type":"text"}]}`, nil
					case "query_records":
						queries++
						if command == "+record-upsert-by-key" {
							if !strings.Contains(mustJSONText(t, args["filters"]), `["fldKey","TASK-1"]`) {
								t.Fatal("changed unique-key preflight", args)
							}
							if writes == 0 {
								return `{"records":[]}`, nil
							}
							return `{"records":[{"recordId":"r1","cells":{"F":"任务","A":"42","fldKey":"TASK-1"}}]}`, nil
						}
						amount := any("42")
						if mode == "ids" {
							amount = float64(42)
						}
						return mustJSONText(t, map[string]any{"records": []any{
							map[string]any{"recordId": "r2", "cells": map[string]any{"F": "任务", "A": amount}},
							map[string]any{"recordId": "r1", "cells": map[string]any{"F": "任务", "A": amount}},
						}}), nil
					default:
						writes++
						for _, record := range args["records"].([]any) {
							want := cloneAnyMap(cells)
							if command == "+record-upsert-by-key" {
								want["fldKey"] = "TASK-1"
							}
							if !reflect.DeepEqual(record.(map[string]any)["cells"], want) {
								t.Fatal("write payload was rewritten", args)
							}
						}
						if command == "+record-upsert-by-key" {
							return `{"newRecordIds":["r1"]}`, nil
						}
						return `{"newRecordIds":["r1","r2"],"createdRecordIds":["r1","r2"],"updatedCount":2}`, nil
					}
				}}
				// Exact ID values also take the zero-metadata path for by-key writes.
				if command == "+record-upsert-by-key" && mode == "ids" {
					cells = map[string]any{"F": "任务"}
					before = mustJSONText(t, cells)
				}
				out, err := runRecordFieldCLI(t, caller, command, cells)
				wantReads := 1
				if mode == "ids" {
					wantReads = 0
				}
				if err != nil || !strings.Contains(out, `"status": "verified"`) || writes != 1 || reads != wantReads || mustJSONText(t, cells) != before {
					t.Fatalf("output=%s err=%v writes=%d fields=%d input=%s", out, err, writes, reads, cells)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordFieldReadFailureKeepsReceipt(t *testing.T) {
	testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error { return nil })
	for _, command := range recordFieldCommands {
		for _, existing := range []bool{false, true} {
			t.Run(command+"/existing="+map[bool]string{true: "true", false: "false"}[existing], func(t *testing.T) {
				writes, reads, queries := 0, 0, 0
				cause := errors.New("fields temporarily unavailable")
				caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, _ map[string]any) (string, error) {
					switch tool {
					case "get_fields":
						reads++
						if writes != 1 || queries == 0 {
							t.Fatal("metadata blocked write")
						}
						return "", cause
					case "query_records":
						queries++
						if command == "+record-upsert-by-key" && writes == 0 && !existing {
							return `{"records":[]}`, nil
						}
						if command == "+record-upsert-by-key" {
							return `{"records":[{"recordId":"r1","cells":{"F":"任务","fldKey":"TASK-1"}}]}`, nil
						}
						return `{"records":[{"recordId":"r1","cells":{"F":"任务"}},{"recordId":"r2","cells":{"F":"任务"}}]}`, nil
					default:
						writes++
						if command == "+record-upsert-by-key" {
							return `{"acknowledged":true}`, nil
						}
						return `{"newRecordIds":["r1","r2"],"createdRecordIds":["r1","r2"],"updatedCount":2}`, nil
					}
				}}
				out, err := runRecordFieldCLI(t, caller, command, map[string]any{"标题": "任务"})
				var typed *apperrors.Error
				if out != "" || !errors.As(err, &typed) || !errors.Is(err, cause) || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable {
					t.Fatalf("missing write/error facts: output=%s err=%#v", out, err)
				}
				result := typed.Details["result"].(compositeResult)
				if result.Status != "unknown" || result.Verification["status"] != "pending" || len(result.CompletedSteps) != 1 || result.CompletedSteps[0].Result == nil || !strings.Contains(mustJSONText(t, result), "r1") || !strings.Contains(result.Verification["error"].(string), cause.Error()) {
					t.Fatal("lost receipt or falsely reported value mismatch", result)
				}
				if command == "+record-upsert-by-key" && result.Checkpoint["recordId"] != "r1" {
					t.Fatal("lost the ID obtained during read-back", result.Checkpoint)
				}
				wantReads := 1
				if command == "+record-upsert" || command == "+record-batch-create" || (command == "+record-upsert-by-key" && !existing) {
					wantReads = 3
				}
				if writes != 1 || reads != wantReads {
					t.Fatalf("write replay or unbounded metadata reads: writes=%d fields=%d", writes, reads)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordCreateDelayedReadback(t *testing.T) {
	for _, command := range []string{"+record-batch-create", "+record-upsert-by-key"} {
		for _, mode := range []string{"visible", "exhausted", "cancelled", "metadata cancelled", "read error"} {
			t.Run(command+"/"+mode, func(t *testing.T) {
				writes, reads, waits := 0, 0, 0
				cause := errors.New("read unavailable")
				testseam.Swap(t, &recordReadbackWait, func(_ context.Context, delay time.Duration) error {
					if delay != recordUpsertReadbackDelays[waits] {
						t.Fatal("changed read-only retry budget", delay)
					}
					waits++
					if strings.Contains(mode, "cancelled") {
						return context.Canceled
					}
					return nil
				})
				caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, _ map[string]any) (string, error) {
					switch tool {
					case "create_records":
						writes++
						if command == "+record-upsert-by-key" {
							return `{"newRecordIds":["r1"]}`, nil
						}
						return `{"newRecordIds":["r1","r2"]}`, nil
					case "query_records":
						if writes == 0 {
							return `{"records":[]}`, nil
						}
						reads++
						if mode == "read error" {
							return "", cause
						}
						if mode != "metadata cancelled" && (mode != "visible" || reads == 1) {
							return `{"records":[]}`, nil
						}
						if command == "+record-upsert-by-key" {
							return `{"records":[{"recordId":"r1","cells":{"F":"任务","fldKey":"TASK-1"}}]}`, nil
						}
						return `{"records":[{"recordId":"r1","cells":{"F":"任务"}},{"recordId":"r2","cells":{"F":"任务"}}]}`, nil
					case "get_fields":
						if mode == "metadata cancelled" {
							return "", cause
						}
						return `{"fields":[{"fieldId":"F","fieldName":"标题"}]}`, nil
					default:
						t.Fatal("unexpected RPC", tool)
						return "", nil
					}
				}}
				out, err := runRecordFieldCLI(t, caller, command, map[string]any{"标题": "任务"})
				if writes != 1 {
					t.Fatal("replayed create", writes)
				}
				if mode == "visible" {
					if err != nil || !strings.Contains(out, `"status": "verified"`) || reads != 2 || waits != 1 {
						t.Fatalf("out=%s err=%v reads=%d waits=%d", out, err, reads, waits)
					}
					return
				}
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Retryable {
					t.Fatalf("unsafe failed create: %v", err)
				}
				result := typed.Details["result"].(compositeResult)
				if result.Status != "unknown" || !strings.Contains(mustJSONText(t, result), "r1") {
					t.Fatal("lost acknowledged write", result)
				}
				switch mode {
				case "exhausted":
					if reads != 3 || waits != 2 || result.Verification["status"] != "pending" {
						t.Fatal("unbounded retry or false success", reads, waits, result)
					}
				case "cancelled", "metadata cancelled":
					if !errors.Is(err, context.Canceled) || reads != 1 || waits != 1 {
						t.Fatal("cancellation ignored", err, reads, waits)
					}
					if mode == "metadata cancelled" && command == "+record-upsert-by-key" && result.Checkpoint["recordId"] != "r1" {
						t.Fatal("cancellation lost observed record ID", result.Checkpoint)
					}
				case "read error":
					if !errors.Is(err, cause) || reads != 1 || waits != 0 {
						t.Fatal("unexpected retry of read error", err, reads, waits)
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageRecordVerificationNameResolution(t *testing.T) {
	fields := []map[string]any{
		{"fieldId": "F", "fieldName": "标题"},
		{"fieldId": "A", "fieldName": "金额"},
		{"fieldId": "D1", "fieldName": "重复"},
		{"fieldId": "D2", "fieldName": "重复"},
	}
	for _, tc := range []struct {
		name   string
		cells  map[string]any
		fields []map[string]any
		want   map[string]any
	}{
		{"name", map[string]any{"标题": "任务"}, fields, map[string]any{"F": "任务"}},
		{"ID", map[string]any{"F": "任务"}, fields, map[string]any{"F": "任务"}},
		{"equal aliases", map[string]any{"标题": nil, "F": nil}, fields, map[string]any{"F": nil}},
		{"conflict", map[string]any{"标题": "甲", "F": "乙"}, fields, nil},
		{"duplicate name", map[string]any{"重复": "任务"}, fields, nil},
		{"unknown", map[string]any{"aB12cD3": "任务"}, fields, nil},
		{"exact name only", map[string]any{" 标题 ": "任务"}, fields, nil},
		{"missing name", map[string]any{"标题": "任务"}, append(append([]map[string]any{}, fields...), map[string]any{"fieldId": "other"}), nil},
		{"missing ID", map[string]any{"标题": "任务"}, []map[string]any{{"fieldName": "标题"}}, nil},
		{"duplicate ID", map[string]any{"F": "任务"}, []map[string]any{{"id": "F"}, {"fieldId": "F"}}, nil},
		{"legacy metadata", map[string]any{"Title": "任务"}, []map[string]any{{"id": "F", "name": "Title"}}, map[string]any{"F": "任务"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := mustJSONText(t, tc.cells)
			got, err := recordVerificationCells(tc.cells, nil, tc.fields)
			if (err != nil) != (tc.want == nil) || !reflect.DeepEqual(got, tc.want) || before != mustJSONText(t, tc.cells) {
				t.Fatalf("got=%v err=%v want=%v", got, err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordVerificationDirectoryAndRecovery(t *testing.T) {
	for _, raw := range []string{`{}`, `{"fields":[]}`, `{"fields":[false]}`, `{"fields":false}`, `{"audit":{"fields":[]}}`, `{"fields":[{"id":"F","name":"标题"}],"hasMore":true}`, `{"fields":[{"id":"F","name":"标题"}],"nextCursor":"next"}`} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"updatedCount":1}`},
			{text: `{"records":[{"recordId":"r1","cells":{"F":"任务"}}]}`},
			{text: raw},
		}}
		_, err := runRecordBatchCLI(t, caller, "+record-update", []map[string]any{{"recordId": "r1", "cells": map[string]any{"标题": "任务"}}})
		if !isPendingRecordReadback(err) || len(caller.calls) != 3 || caller.calls[0].tool != "update_records" || caller.calls[2].tool != "get_fields" {
			t.Fatalf("directory=%s err=%v calls=%v", raw, err, caller.calls)
		}
	}
	testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error { return nil })
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"createdRecordIds":["r1","r2"]}`},
		{text: `{"records":[{"recordId":"r2","cells":{"F":"42"}},{"recordId":"r1","cells":{"F":"42"}}]}`},
		{err: errors.New("temporary metadata failure")},
		{text: `{"records":[{"recordId":"r2","cells":{"F":"42"}},{"recordId":"r1","cells":{"F":"42"}}]}`},
		{text: `{"data":{"fieldList":[{"id":"F","name":"金额","type":"number"}]},"hasMore":false,"nextCursor":"echoed"}`},
	}}
	out, err := runRecordBatchCLI(t, caller, "+record-upsert", []map[string]any{{"cells": map[string]any{"金额": 42}}, {"cells": map[string]any{"金额": 42}}})
	if err != nil || !strings.Contains(out, `"status": "verified"`) {
		t.Fatalf("output=%s err=%v", out, err)
	}
	for i, tool := range []string{"record_upsert", "query_records", "get_fields", "query_records", "get_fields"} {
		if len(caller.calls) != 5 || caller.calls[i].tool != tool {
			t.Fatal("unexpected retry sequence", caller.calls)
		}
	}
}

func TestCrossPlatformCoverageRecordVerificationCannotGuessOrHideMismatch(t *testing.T) {
	for _, tc := range []struct {
		cells   map[string]any
		fields  string
		pending bool
	}{
		{map[string]any{"标题": "任务"}, `[{"id":"F","name":"标题"},{"id":"G","name":"标题"}]`, true},
		{map[string]any{"标题": "甲", "F": "乙"}, `[{"id":"F","name":"标题"}]`, true},
		{map[string]any{"标题": "wrong"}, `[{"id":"F","name":"标题"}]`, false},
	} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"updatedCount":1}`},
			{text: `{"records":[{"recordId":"r1","cells":{"F":"任务"}}]}`},
			{text: `{"fields":` + tc.fields + `}`},
		}}
		out, err := runRecordBatchCLI(t, caller, "+record-update", []map[string]any{{"recordId": "r1", "cells": tc.cells}})
		if err == nil || out != "" || isPendingRecordReadback(err) != tc.pending || len(caller.calls) != 3 {
			t.Fatalf("output=%s err=%v pending=%v calls=%v", out, err, tc.pending, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageRecordFieldDryRunKeepsOriginalReads(t *testing.T) {
	for _, command := range recordFieldCommands {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":[]}`}}}
		out, err := runRecordFieldCLI(t, caller, command, map[string]any{"标题": "任务"}, "--dry-run")
		var result compositeResult
		if err != nil || json.Unmarshal([]byte(out), &result) != nil || result.Executed || result.Status != "planned" {
			t.Fatalf("command=%s output=%s err=%v", command, out, err)
		}
		for _, call := range caller.calls {
			if call.tool != "query_records" {
				t.Fatal("dry-run introduced metadata or write call", caller.calls)
			}
		}
	}
}
