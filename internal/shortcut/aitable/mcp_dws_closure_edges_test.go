// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageClosurePendingReceiptBoundaries(t *testing.T) {
	for _, raw := range []map[string]any{
		nil,
		{"results": []any{42}},
		{"results": []any{map[string]any{"success": false, "errorCode": "OTHER"}}},
		{"results": []any{map[string]any{"success": true}}},
	} {
		if fieldReadbackPendingReceipt(raw) {
			t.Fatalf("unverified receipt allowed continuation: %#v", raw)
		}
	}
	if isRecordWriteInputRejection(&helpers.CLIError{Code: helpers.CodeMCPToolError, Message: "not JSON"}) {
		t.Fatal("malformed receipt is not an input rejection")
	}
}

func TestCrossPlatformCoverageClosureReconciliationReadFailures(t *testing.T) {
	const token = "123e4567-e89b-42d3-a456-426614174000"
	for _, tc := range []struct {
		name, token string
		step        upsertByKeyStep
		calls       int
	}{
		{"invalid token", "invalid", upsertByKeyStep{}, 0},
		{"transport error", token, upsertByKeyStep{err: errors.New("read unavailable")}, 1},
		{"unknown", token, upsertByKeyStep{text: `{"state":"unknown"}`}, 1},
		{"missing IDs", token, upsertByKeyStep{text: `{"baseId":"b","tableId":"t","clientToken":"` + token + `","state":"applied"}`}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{tc.step}}
			out, err := runAITableCompositeCLI(t, caller, "+record-write-result", "--base-id", "b", "--table-id", "t", "--client-token", tc.token)
			if err == nil || out != "" || len(caller.calls) != tc.calls {
				t.Fatalf("out=%s err=%v calls=%#v", out, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageClosurePartialOrRejectedCreateStops(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{true: "input rejected", false: "partial applied"}[rejected], func(t *testing.T) {
			writes, reads, token := 0, 0, ""
			caller := &upsertByKeyCaller{callFn: func(_ int, _ string, tool string, args map[string]any) (string, error) {
				switch tool {
				case "create_records":
					writes++
					token = args["clientToken"].(string)
					if rejected {
						return `{"status":"error","error":{"type":"INPUT_ERROR","code":"INVALID_RECORDS","retryable":false}}`, nil
					}
					return "", errors.New("write response lost")
				case "get_record_write_result":
					reads++
					if args["clientToken"] != token {
						t.Fatal("changed reconciliation token")
					}
					return mustJSONText(t, map[string]any{"baseId": "b", "tableId": "t", "clientToken": token, "state": "applied", "recordIds": []any{"r1"}}), nil
				default:
					t.Fatalf("unexpected call after incomplete create: %s", tool)
					return "", nil
				}
			}}
			out, err := runAITableCompositeCLI(t, caller, "+record-batch-create", "--base-id", "b", "--table-id", "t", "--records", `[{"cells":{"title":"a"}},{"cells":{"title":"b"}}]`, "--yes")
			var failure *apperrors.Error
			if out != "" || !errors.As(err, &failure) || failure.Retryable || writes != 1 {
				t.Fatalf("out=%s err=%v writes=%d", out, err, writes)
			}
			if rejected {
				if reads != 0 {
					t.Fatal("input rejection must not reconcile")
				}
			} else {
				result := failure.Details["result"].(compositeResult)
				if reads != 1 || result.Checkpoint["clientToken"] != token || !strings.Contains(result.NextCommand, token) || result.CompletedCount != 0 {
					t.Fatalf("lost partial-write recovery: %#v", result)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageClosureAIWriteErrorStops(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"fields":[{"fieldId":"f","aiConfig":{"outputType":"text"}}]}`},
		{err: errors.New("AI submission unavailable")},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+field-run-ai", "--base-id", "b", "--table-id", "t", "--field-ids", "f", "--yes")
	if err == nil || out != "" || len(caller.calls) != 2 || caller.calls[1].tool != "run_ai_field" {
		t.Fatal(out, err, caller.calls)
	}
}
