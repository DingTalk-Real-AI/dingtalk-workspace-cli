// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageRecordTypedScalarProjection(t *testing.T) {
	cases := []struct {
		name, kind string
		got, want  any
		equal      bool
	}{
		{"number", "number", "42", float64(42), true},
		{"decimal", "currency", "12.50", json.Number("12.5"), true},
		{"progress", "progress", "0.5", 0.5, true},
		{"rating", "rating", "4", 4, true},
		{"precision", "number", "9007199254740993", json.Number("9007199254740992"), false},
		{"text untouched", "text", "42", 42, false},
		{"no schema", "", "42", 42, false},
		{"bad number", "number", "42x", 42, false},
		{"bool", "number", "1", true, false},
		{"nan", "number", "NaN", math.NaN(), false},
		{"unbounded exponent", "number", "1e1000000000", 1, false},
		{"date", "date", "2026-09-03T00:00:00+08:00", "2026-09-03", true},
		{"instant", "date", "2026-09-03T00:00:00+08:00", "2026-09-02T16:00:00Z", true},
		{"different date", "date", "2026-09-04T00:00:00+08:00", "2026-09-03", false},
		{"not midnight", "date", "2026-09-03T12:00:00+08:00", "2026-09-03", false},
		{"different instant", "date", "2026-09-03T00:00:00+08:00", "2026-09-03T00:00:00Z", false},
		{"invalid date", "date", "bad", "2026-09-03", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := recordCellValueEqual(tc.got, tc.want, tc.kind); got != tc.equal {
				t.Fatalf("equal=%v, want %v", got, tc.equal)
			}
		})
	}
}

func TestCrossPlatformCoverageRecordUpdateTypedProjectionE2E(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"status":"success","data":{"recordIds":["r1"]}}`},
		{text: `{"records":[{"recordId":"r1","cells":{"n":"42","d":"2026-09-03T00:00:00+08:00"}}]}`},
		{text: `{"fields":[{"fieldId":"n","type":"number"},{"fieldId":"d","type":"date"}]}`},
	}}
	out, err := runRecordBatchCLI(t, caller, "+record-update", []map[string]any{{"recordId": "r1", "cells": map[string]any{"n": 42, "d": "2026-09-03"}}})
	if err != nil || !strings.Contains(out, `"status": "verified"`) || len(caller.calls) != 3 {
		t.Fatalf("out=%s err=%v calls=%v", out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordRejectDoesNotVerifyOrRetry(t *testing.T) {
	raw := `{"status":"error","error":{"type":"INPUT_ERROR","code":"MIXED_ATTACHMENT_UPDATE_MODES","retryable":false}}`
	for _, writeErr := range []error{&helpers.CLIError{Code: helpers.CodeMCPToolError, Message: raw}, apperrors.NewAPI(raw, apperrors.WithReason("business_error"))} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: writeErr}}}
		_, err := runRecordBatchCLI(t, caller, "+record-update", []map[string]any{{"recordId": "r1", "cells": map[string]any{"attachment": []any{}}}})
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable || typed.Reason != "aitable_composite_failed" || len(caller.calls) != 1 || !errors.Is(err, writeErr) {
			t.Fatalf("error=%#v calls=%v", err, caller.calls)
		}
	}
	for _, err := range []error{errors.New(raw), &helpers.CLIError{Code: helpers.CodeMCPToolError, Message: `{"status":"error","error":{"type":"SYSTEM_ERROR","retryable":false}}`}, &helpers.CLIError{Code: helpers.CodeMCPToolError, Message: `{"status":"error","error":{"type":"INPUT_ERROR"}}`}} {
		if isRecordWriteInputRejection(err) {
			t.Fatalf("ambiguous error treated as input rejection: %v", err)
		}
	}
}

func TestCrossPlatformCoverageRecordByKeyTypedProjectionAndRejection(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"records":[{"recordId":"r1","cells":{"fldKey":"TASK-1"}}]}`},
		{text: `{"status":"success","data":{"recordIds":["r1"]}}`},
		{text: `{"records":[{"recordId":"r1","cells":{"fldKey":"TASK-1","n":"42"}}]}`},
		{text: `{"fields":[{"fieldId":"n","type":"number"}]}`},
	}}
	if _, err := runUpsertByKeyCLI(t, caller, "--cells", `{"n":42}`); err != nil {
		t.Fatal(err)
	}
	caller = &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"records":[{"recordId":"r1","cells":{"fldKey":"TASK-1"}}]}`},
		{err: &helpers.CLIError{Code: helpers.CodeMCPToolError, Message: `{"status":"error","error":{"type":"INPUT_ERROR","retryable":false}}`}},
	}}
	_, err := runUpsertByKeyCLI(t, caller)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Retryable || typed.Reason != "aitable_composite_failed" || len(caller.calls) != 2 {
		t.Fatalf("error=%v calls=%v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageRecordRejectPreservesCompletedBatch(t *testing.T) {
	records := make([]map[string]any, recordBatchSize+1)
	for i := range records {
		records[i] = map[string]any{"recordId": "r" + strconv.Itoa(i), "cells": map[string]any{"f": "x"}}
	}
	readback, _ := json.Marshal(map[string]any{"records": records[:recordBatchSize]})
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"updatedCount":100}`}, {text: string(readback)},
		{err: &helpers.CLIError{Code: helpers.CodeMCPToolError, Message: `{"status":"error","error":{"type":"INPUT_ERROR","retryable":false}}`}},
	}}
	_, err := runRecordBatchCLI(t, caller, "+record-update", records)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Retryable || typed.Reason != "aitable_composite_partial_success" {
		t.Fatalf("error=%#v", err)
	}
	result := typed.Details["result"].(compositeResult)
	if result.CompletedCount != 100 || result.FailedCount != 1 || len(result.KnownEffects) != 1 || len(caller.calls) != 3 {
		t.Fatalf("result=%#v calls=%v", result, caller.calls)
	}
}
