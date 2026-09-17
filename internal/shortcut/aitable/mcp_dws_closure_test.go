// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageClosureCopy105RecoversCommittedReceipt(t *testing.T) {
	stored := map[string]map[string]any{}
	tokens := map[string][]string{}
	writes, reconciliations := 0, 0
	caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
		switch tool {
		case "get_fields":
			id := "sourceField"
			if args["baseId"] == "target" {
				id = "targetField"
			}
			return mustJSONText(t, map[string]any{"fields": []any{map[string]any{"fieldId": id, "fieldName": "Title", "type": "text"}}}), nil
		case "create_table":
			return `{"tableId":"targetTable"}`, nil
		case "query_records":
			rows := []any{}
			if args["baseId"] == "source" {
				offset, _ := strconv.Atoi(fmt.Sprint(args["cursor"]))
				end := minInt(offset+args["limit"].(int), 105)
				for i := offset; i < end; i++ {
					rows = append(rows, map[string]any{"recordId": fmt.Sprintf("s%d", i), "cells": map[string]any{"sourceField": fmt.Sprintf("value%d", i)}})
				}
				next := ""
				if end < 105 {
					next = strconv.Itoa(end)
				}
				return mustJSONText(t, map[string]any{"records": rows, "hasMore": end < 105, "nextCursor": next}), nil
			}
			for _, id := range args["recordIds"].([]string) {
				rows = append(rows, stored[id])
			}
			return mustJSONText(t, map[string]any{"records": rows}), nil
		case "create_records":
			writes++
			token := args["clientToken"].(string)
			if token == "" || aitableprotocol.ValidateClientToken(token) != nil || tokens[token] != nil {
				t.Fatal("missing/reused batch token", token)
			}
			ids := []string{}
			for _, raw := range args["records"].([]any) {
				id := fmt.Sprintf("t%d", len(stored))
				stored[id] = map[string]any{"recordId": id, "cells": raw.(map[string]any)["cells"]}
				ids = append(ids, id)
			}
			tokens[token] = ids
			if writes == 1 {
				return "", fmt.Errorf("response lost after 100 committed records")
			}
			return mustJSONText(t, map[string]any{"newRecordIds": ids}), nil
		case "get_record_write_result":
			reconciliations++
			token := args["clientToken"].(string)
			ids := append([]string{}, tokens[token]...)
			// The server explicitly does not promise input order.
			for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
				ids[i], ids[j] = ids[j], ids[i]
			}
			return mustJSONText(t, map[string]any{"baseId": "target", "tableId": "targetTable", "clientToken": token, "state": "applied", "recordIds": ids}), nil
		default:
			t.Fatalf("unexpected tool %s", tool)
			return "", nil
		}
	}}
	out, err := runAITableCompositeCLI(t, caller, "+table-copy", "--source-base-id", "source", "--source-table-id", "sourceTable", "--target-base-id", "target", "--new-name", "Copy", "--include-records", "--yes")
	if err != nil || writes != 2 || reconciliations != 1 || len(stored) != 105 || !strings.Contains(out, `"recordCount": 105`) {
		t.Fatalf("writes=%d reconcile=%d stored=%d output=%s error=%v", writes, reconciliations, len(stored), out, err)
	}
}

func TestCrossPlatformCoverageClosureAIRequiresConfigAndVerifiedTasks(t *testing.T) {
	for _, configured := range []bool{false, true} {
		field := map[string]any{"fieldId": "f", "type": "text"}
		if configured {
			field["aiConfig"] = map[string]any{"outputType": "text"}
		}
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			parityStep(map[string]any{"fields": []any{field}}),
			parityStep(map[string]any{"tasks": []any{map[string]any{"fieldId": "f", "taskId": "task1", "status": "submitted"}}}),
		}}
		out, err := runAITableCompositeCLI(t, caller, "+field-run-ai", "--base-id", "b", "--table-id", "t", "--field-ids", "f", "--record-ids", "r", "--yes")
		if configured {
			if err != nil || !strings.Contains(out, `"status": "submitted"`) || !strings.Contains(out, `"completed": false`) {
				t.Fatal(out, err)
			}
		} else if err == nil || len(caller.calls) != 1 {
			t.Fatal("executed unconfigured AI field", out, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageClosurePendingFieldReadsSameIDWithoutRecreating(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"fields":[]}`},
		{text: `{"status":"success","data":{"successCount":0,"failedCount":1,"results":[{"fieldName":"Title","fieldId":"f","success":false,"errorCode":"CREATE_FIELD_READBACK_PENDING"}]}}`},
		{text: `{"fields":[{"fieldName":"Title","fieldId":"f","type":"text"}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", `[{"fieldName":"Title","type":"text"}]`, "--yes")
	if err != nil || len(caller.calls) != 3 || caller.calls[2].tool != "get_fields" || !strings.Contains(out, `"verified"`) {
		t.Fatal(out, err, caller.calls)
	}
}

func TestCrossPlatformCoverageClosurePendingFieldBudgetPreservesKnownID(t *testing.T) {
	testseam.Swap(t, &recordReadbackWait, func(context.Context, time.Duration) error { return nil })
	for _, code := range []string{"CREATE_FIELD_READBACK_PENDING", "CREATE_FIELD_REJECTED"} {
		t.Run(code, func(t *testing.T) {
			writes, reads := 0, 0
			caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
				switch tool {
				case "create_fields":
					writes++
					return fmt.Sprintf(`{"results":[{"fieldName":"Title","fieldId":"f","success":false,"errorCode":%q}]}`, code), nil
				case "get_fields":
					if writes > 0 {
						reads++
						ids, _ := args["fieldIds"].([]string)
						if len(ids) != 1 || ids[0] != "f" {
							t.Fatalf("readback changed the acknowledged ID: %#v", args)
						}
					}
					return `{"fields":[]}`, nil
				default:
					t.Fatalf("unexpected recovery tool: %s", tool)
					return "", nil
				}
			}}
			out, err := runAITableCompositeCLI(t, caller, "+field-create", "--base-id", "b", "--table-id", "t", "--fields", `[{"fieldName":"Title","type":"text"}]`, "--yes")
			var failure *apperrors.Error
			if out != "" || !errors.As(err, &failure) || failure.Retryable || writes != 1 {
				t.Fatal("replayed creation or claimed completion", out, err, writes)
			}
			result := failure.Details["result"].(compositeResult)
			if code == "CREATE_FIELD_READBACK_PENDING" {
				if reads != 6 || result.CompletedCount != 0 || len(result.KnownEffects) != 1 || result.Status != "unknown" {
					t.Fatal("lost the bounded pending state", reads, result)
				}
				ids, _ := result.KnownEffects[0]["fieldIds"].([]string)
				if len(ids) != 1 || ids[0] != "f" {
					t.Fatal("lost acknowledged field ID", result.KnownEffects)
				}
			} else if reads != 0 || len(result.KnownEffects) != 0 {
				t.Fatal("treated an arbitrary failed receipt as acknowledged", reads, result)
			}
		})
	}
}

func TestCrossPlatformCoverageClosureReconciliationRejectsUnknownAndWrongIdentity(t *testing.T) {
	for _, state := range []string{"unknown", "applied"} {
		body := map[string]any{"baseId": "b", "tableId": "wrong", "clientToken": "token", "state": state, "recordIds": []any{"r"}}
		if _, err := reconciledRecordIDs(body, "b", "t", "token"); err == nil {
			t.Fatal("accepted uncertain receipt")
		}
	}
	for _, ids := range [][]any{{"r", "r"}, {""}, {42}} {
		body := map[string]any{"baseId": "b", "tableId": "t", "clientToken": "token", "state": "applied", "recordIds": ids}
		if _, err := reconciledRecordIDs(body, "b", "t", "token"); err == nil {
			t.Fatal("accepted bad IDs", ids)
		}
	}
}

func TestCrossPlatformCoverageClosureUnknownBatchPreservesTokenAndStops(t *testing.T) {
	writes, reads, token := 0, 0, ""
	caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
		switch tool {
		case "create_records":
			writes++
			token = args["clientToken"].(string)
			return "", fmt.Errorf("response lost")
		case "get_record_write_result":
			reads++
			if args["clientToken"] != token {
				t.Fatal("changed token during reconciliation")
			}
			return `{"state":"unknown"}`, nil
		default:
			t.Fatalf("unexpected follow-up tool: %s", tool)
			return "", nil
		}
	}}
	rows := make([]map[string]any, 105)
	for i := range rows {
		rows[i] = map[string]any{"cells": map[string]any{"title": fmt.Sprint(i)}}
	}
	out, err := runAITableCompositeCLI(t, caller, "+record-batch-create", "--base-id", "b", "--table-id", "t", "--records", mustJSONText(t, rows), "--yes")
	var failure *apperrors.Error
	if out != "" || !errors.As(err, &failure) || failure.Retryable || writes != 1 || reads != 1 {
		t.Fatal(out, err, writes, reads)
	}
	result := failure.Details["result"].(compositeResult)
	if result.Checkpoint["clientToken"] != token || result.CompletedCount != 0 || len(result.KnownEffects) != 0 || !strings.Contains(result.NextCommand, "+record-write-result") {
		t.Fatal(result)
	}
}

func TestCrossPlatformCoverageClosurePartialBatchKeepsIDsWithoutBackfill(t *testing.T) {
	writes, reads, token := 0, 0, ""
	caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
		switch tool {
		case "create_records":
			writes++
			token = args["clientToken"].(string)
			return "", fmt.Errorf("response lost")
		case "get_record_write_result":
			reads++
			if args["baseId"] != "b" || args["tableId"] != "t" || args["clientToken"] != token {
				t.Fatal("changed the original reconciliation identity", args)
			}
			return mustJSONText(t, map[string]any{"baseId": "b", "tableId": "t", "clientToken": token, "state": "applied", "recordIds": []string{"r1"}}), nil
		default:
			t.Fatalf("partial ID set must stop before further operations: %s", tool)
			return "", nil
		}
	}}
	rows := make([]map[string]any, 105)
	for i := range rows {
		rows[i] = map[string]any{"cells": map[string]any{"title": fmt.Sprint(i)}}
	}
	out, err := runAITableCompositeCLI(t, caller, "+record-batch-create", "--base-id", "b", "--table-id", "t", "--records", mustJSONText(t, rows), "--yes")
	var failure *apperrors.Error
	if out != "" || !errors.As(err, &failure) || failure.Retryable || writes != 1 || reads != 1 {
		t.Fatal(out, err, writes, reads)
	}
	result := failure.Details["result"].(compositeResult)
	if result.Status != "unknown" || result.CompletedCount != 0 || len(result.KnownEffects) != 1 || result.Checkpoint["clientToken"] != token {
		t.Fatal("partial receipt advanced or lost its reconciliation identity", result)
	}
	ids, _ := result.KnownEffects[0]["recordIds"].([]string)
	wantNext := aitableRecoveryCommand("dws", "aitable", "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", token)
	if len(ids) != 1 || ids[0] != "r1" || result.NextCommand != wantNext {
		t.Fatal("lost verified IDs or suggested something other than original-token reconciliation", result)
	}
}

// A duplicate token is evidence of an earlier accepted request, not proof that
// no rows were written. Recover by token and verify cells without another write.
func TestCrossPlatformCoverageClosureDuplicateTokenReconcilesCommittedBatch(t *testing.T) {
	writes, reconciliations, verifications := 0, 0, 0
	token := ""
	caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
		switch tool {
		case "create_records":
			writes++
			token = args["clientToken"].(string)
			return mustJSONText(t, map[string]any{
				"status": "error",
				"error": map[string]any{
					"type": "USER_ERROR", "code": "DUPLICATE_CLIENT_TOKEN", "retryable": false,
					"details": map[string]any{"baseId": "b", "tableId": "t", "clientToken": token, "reconcileTool": "get_record_write_result"},
				},
			}), nil
		case "get_record_write_result":
			reconciliations++
			if args["clientToken"] != token {
				t.Fatal("reconciliation changed the original token")
			}
			return mustJSONText(t, map[string]any{"baseId": "b", "tableId": "t", "clientToken": token, "state": "applied", "recordIds": []string{"r1"}}), nil
		case "query_records":
			verifications++
			return `{"records":[{"recordId":"r1","cells":{"title":"value"}}]}`, nil
		default:
			t.Fatalf("unexpected tool: %s", tool)
			return "", nil
		}
	}}
	out, err := runAITableCompositeCLI(t, caller, "+record-batch-create", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"title":"value"}}]`, "--yes")
	if err != nil || writes != 1 || reconciliations != 1 || verifications != 1 || !strings.Contains(out, `"completedCount": 1`) {
		t.Fatalf("writes=%d reconciliations=%d verifications=%d output=%s error=%v", writes, reconciliations, verifications, out, err)
	}
}

// Both error wrappers must preserve the distinction between an input rejection
// and an uncertain write, including older duplicate-token receipts without details.
func TestCrossPlatformCoverageClosureWriteRejectionRespectsRecoveryEvidence(t *testing.T) {
	cases := []struct {
		name, raw string
		rejected  bool
	}{
		{"duplicate", `{"status":"error","error":{"type":"USER_ERROR","code":"DUPLICATE_CLIENT_TOKEN","retryable":false}}`, false},
		{"downstream duplicate", `{"status":"error","error":{"type":"INPUT_ERROR","code":"REQUEST_ID_CONFLICT","retryable":false}}`, false},
		{"recovery details", `{"status":"error","error":{"type":"INPUT_ERROR","code":"AFTER_WRITE_ERROR","retryable":false,"details":{"reconcileTool":"get_record_write_result"}}}`, false},
		{"ordinary input", `{"status":"error","error":{"type":"INPUT_ERROR","code":"INVALID_RECORDS","retryable":false}}`, true},
		{"ordinary user", `{"status":"error","error":{"type":"USER_ERROR","code":"VALUE_TOO_LONG","retryable":false}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, err := range []error{
				&helpers.CLIError{Code: helpers.CodeMCPToolError, Message: tc.raw},
				apperrors.NewAPI(tc.raw, apperrors.WithReason("business_error")),
			} {
				if got := isRecordWriteInputRejection(err); got != tc.rejected {
					t.Fatalf("rejected=%v, want %v for %T", got, tc.rejected, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageClosureReconciliationReadCommand(t *testing.T) {
	token := "123e4567-e89b-42d3-a456-426614174000"
	args := []string{"--base-id", "b", "--table-id", "t", "--client-token", token}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{parityStep(map[string]any{
		"baseId": "b", "tableId": "t", "clientToken": token, "state": "applied", "recordIds": []any{"r"},
	})}}
	out, err := runAITableCompositeCLI(t, caller, "+record-write-result", args...)
	if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_record_write_result" || !strings.Contains(out, `"applied"`) {
		t.Fatal(out, err, caller.calls)
	}
	caller = &upsertByKeyCaller{}
	out, err = runAITableCompositeCLI(t, caller, "+record-write-result", append(args, "--dry-run")...)
	if err != nil || len(caller.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatal(out, err)
	}
	if RecordWriteResult.Risk != "read" || RecordWriteResult.Safety.Effect != "read" || RecordWriteResult.Contract.Result == nil {
		t.Fatal("reconciliation must be a declared read-only command")
	}
}

func TestCrossPlatformCoverageClosureAITasksRejectMissingOrDuplicateReceipts(t *testing.T) {
	for _, tasks := range [][]any{
		{map[string]any{"fieldId": "f", "status": "submitted"}},
		{map[string]any{"fieldId": "f", "taskId": "id", "status": "completed"}},
		{map[string]any{"fieldId": "other", "taskId": "id", "status": "submitted"}},
	} {
		if aiTasksSubmitted(map[string]any{"tasks": tasks}, []string{"f"}) {
			t.Fatal("accepted unverified task", tasks)
		}
	}
	item := map[string]any{"fieldId": "f", "taskId": "id", "status": "submitted"}
	if aiTasksSubmitted(map[string]any{"tasks": []any{item, item}}, []string{"f", "g"}) {
		t.Fatal("accepted duplicate task")
	}
}
