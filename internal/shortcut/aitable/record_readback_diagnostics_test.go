// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageRecordReadbackDiagnosticsExcludeUnsafeProse(t *testing.T) {
	for _, tc := range []struct {
		err   error
		kind  string
		field string
	}{
		{&recordReadbackMismatchError{fieldID: "F"}, "mismatch", "F"},
		{pendingRecordReadback(&recordReadbackMismatchError{fieldID: "F"}), "mismatch", "F"},
		{pendingRecordReadback(errors.New("private-pending-canary")), "pending", ""},
		{errors.New("private-file-token-canary"), "unavailable", ""},
	} {
		metadata := map[string]any{}
		addRecordReadbackDiagnostics(metadata, tc.err)
		if metadata["readback"] != tc.kind || tc.field != "" && metadata["mismatchedFieldId"] != tc.field ||
			tc.field == "" && metadata["mismatchedFieldId"] != nil || strings.Contains(mustJSONText(t, metadata), "private-") {
			t.Fatalf("unsafe or incomplete readback diagnostics: %#v", metadata)
		}
	}
	for _, field := range []string{"", "  ", "private\x1b", strings.Repeat("F", 257)} {
		metadata := map[string]any{}
		copyRecordReadbackDiagnostics(metadata, "mismatch", field)
		if metadata["readback"] != "mismatch" || metadata["mismatchedFieldId"] != nil {
			t.Fatal("unsafe mismatch identifier accepted")
		}
	}
	metadata := map[string]any{}
	copyRecordReadbackDiagnostics(metadata, "private-unreviewed-kind", "F")
	if len(metadata) != 0 {
		t.Fatal("unreviewed classification accepted")
	}
	if message := (&recordReadbackMismatchError{fieldID: "F"}).Error(); message != "read-back mismatch for field F: requested value is unconfirmed" {
		t.Fatal("mismatch message changed", message)
	}
}

func TestCrossPlatformCoverageRecordAttachmentPendingReadCanRecover(t *testing.T) {
	for _, tool := range []string{"record_upsert", "create_records"} {
		testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error { return nil })
		reads := 0
		batch := []map[string]any{{"cells": map[string]any{"A": []any{map[string]any{"fileToken": "fixture"}}}}}
		result, err := verifyRecordBatchEventually(nil, tool, "B", "T", batch, nil,
			func(_ *shortcut.RuntimeContext, _, _ string, _ []map[string]any, _ map[string]any) (map[string]any, error) {
				reads++
				if reads == 1 {
					return nil, pendingRecordReadback(errors.New("record not visible yet"))
				}
				return map[string]any{"status": "verified"}, nil
			})
		if err != nil || reads != 2 || result["status"] != "verified" {
			t.Fatalf("attachment pending read lost the existing retry budget: reads=%d err=%v", reads, err)
		}
	}
}

func TestCrossPlatformCoverageRecordMalformedRecoveryStopsAllFollowingCalls(t *testing.T) {
	for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "CREATE_RECORDS_OUTCOME_UNKNOWN", "UPDATE_RECORDS_RESULT_UNKNOWN", "DUPLICATE_CLIENT_TOKEN", "ATTACHMENT_WRITE_UNCONFIRMED", "GROUP_FAILURE"} {
		for _, invalid := range []any{[]any{"untrusted-id", 1}, make(chan int)} {
			details := map[string]any{"recordIds": invalid, "attachmentVerification": "unconfirmed"}
			if code == "GROUP_FAILURE" {
				details["groupOutcomes"] = invalid
			}
			cause := apperrors.NewAPI("unconfirmed write", apperrors.WithReason(code), apperrors.WithDetails(details))
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{err: cause}}}
			records := make([]map[string]any, 101)
			for index := range records {
				records[index] = map[string]any{"recordId": fmt.Sprintf("R%d", index), "cells": map[string]any{"F": "changed"}}
			}
			_, err := runRecordBatchCLI(t, caller, "+record-update", records)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != 1 {
				t.Fatalf("malformed recovery allowed later calls: code=%s err=%v calls=%d", code, err, len(caller.calls))
			}
			result := typed.Details["result"].(compositeResult)
			if result.Status != "unknown" || result.FailedCount != 0 || result.CompletedCount != 0 ||
				!strings.Contains(result.NextCommand, "+record-query") || strings.Contains(mustJSONText(t, result), "untrusted-id") {
				t.Fatalf("malformed receipt invented targets or lost safe recovery: %#v", result)
			}
		}
	}
}

func TestCrossPlatformCoverageRecordAttachmentRetainsActualReadbackFailure(t *testing.T) {
	for _, command := range []string{"+record-update", "+record-upsert", "+record-upsert-by-key"} {
		for _, mismatch := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mismatch=%v", command, mismatch), func(t *testing.T) {
				testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error { return nil })
				writes, reads := 0, 0
				caller := &upsertByKeyCaller{callFn: func(_ int, _, tool string, _ map[string]any) (string, error) {
					switch tool {
					case "update_records", "record_upsert":
						writes++
						return `{"data":{"updatedRecordIds":["R"]}}`, nil
					case "query_records":
						if command == "+record-upsert-by-key" && writes == 0 {
							return `{"data":{"records":[{"recordId":"R","cells":{"fldKey":"TASK-1"}}]}}`, nil
						}
						reads++
						if !mismatch {
							return "", errors.New("private-read-token-canary")
						}
						return `{"data":{"records":[{"recordId":"R","cells":{"A":[{"fileToken":"private-file-canary"}],"F":"old","fldKey":"TASK-1"}}]}}`, nil
					case "get_fields":
						return `{"data":{"fields":[{"fieldId":"A","type":"attachment"},{"fieldId":"F","type":"text"},{"fieldId":"fldKey","type":"text"}]}}`, nil
					}
					return "", fmt.Errorf("unexpected tool %s", tool)
				}}
				var err error
				if command == "+record-upsert-by-key" {
					_, err = runUpsertByKeyCLI(t, caller, "--cells", `{"A":[{"fileToken":"private-file-canary"}],"F":"new"}`)
				} else {
					_, err = runAITableCompositeCLI(t, caller, command, "--base-id", "B", "--table-id", "T", "--records", `[{"recordId":"R","cells":{"A":[{"fileToken":"private-file-canary"}],"F":"new"}}]`, "--yes")
				}
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Retryable || writes != 1 {
					t.Fatalf("attachment failure was replayed or lost: %v", err)
				}
				result := typed.Details["result"].(compositeResult)
				wantKind, wantReads := "unavailable", 1
				if mismatch {
					wantKind = "mismatch"
					if command == "+record-upsert" {
						wantReads += len(recordUpsertReadbackDelays)
					}
					if result.Verification["mismatchedFieldId"] != "F" {
						t.Fatal("ordinary field mismatch was hidden behind attachment diagnostics")
					}
				}
				if result.Verification["readback"] != wantKind || result.Status != "unknown" || result.FailedCount != 0 || reads != wantReads ||
					strings.Contains(mustJSONText(t, result), "private-") {
					t.Fatalf("unsafe or incomplete verification: reads=%d result=%#v", reads, result)
				}
			})
		}
	}
}
