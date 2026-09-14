// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageWhiteboardCommittedReadbackFailuresNeverReplay(t *testing.T) {
	for _, mode := range []string{"append", "overwrite"} {
		for _, tc := range []struct {
			name, response, reason string
		}{
			{"coordinate drift", validWhiteboardQueryResponse(`[{"id":"real-1","type":"group","x":40.5001}]`), "readback_field_mismatch"},
			{"missing coordinate", validWhiteboardQueryResponse(`[{"id":"real-1","type":"group"}]`), "readback_field_missing"},
			{"wrong type", validWhiteboardQueryResponse(`[{"id":"real-1","type":"frame","x":40}]`), "readback_type_mismatch"},
			{"wrong identity", validWhiteboardQueryResponse(`[{"id":"other","type":"group","x":40}]`), "readback_identity_mismatch"},
			{"invalid readback", `{"success":true}`, ""},
			{"read failure", "", "readback_failed"},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				deleted := 0
				if mode == "overwrite" {
					deleted = 2
				}
				caller := &whiteboardCoverageCaller{responses: map[string][]string{
					toolUpdate: {validWhiteboardUpdateResponse(mode, []string{"n1"}, []string{"real-1"}, deleted)},
				}}
				if tc.response != "" {
					caller.responses[toolQuery] = []string{tc.response}
				}
				source := fmt.Sprintf(`{"overwrite":%t,"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"group","x":40}]}}`, mode == "overwrite")
				stdout, err := runWhiteboardCoverageOutput(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes")
				var failure *apperrors.Error
				if !errors.As(err, &failure) {
					t.Fatalf("expected structured failure, got %v; output=%s", err, stdout)
				}
				if len(stdout) != 0 || failure.ExecutionStarted == nil || !*failure.ExecutionStarted || !failure.RetryableSet || failure.Retryable {
					t.Fatalf("committed failure lost no-replay state: %#v; output=%s", failure, stdout)
				}
				if tc.reason != "" && failure.Reason != tc.reason {
					t.Fatalf("reason=%q, want %q", failure.Reason, tc.reason)
				}
				wantDetails := map[string]any{
					"nodeId": "doc", "partId": "part", "mode": mode, "commitState": "committed", "verified": false,
					"receipt": map[string]any{
						"message": "completed", "createdNodeIds": []string{"real-1"},
						"idMap": map[string]string{"n1": "real-1"}, "deletedNodeCount": deleted,
					},
				}
				if !reflect.DeepEqual(failure.Details, wantDetails) {
					t.Fatalf("lost reconciliation evidence or leaked snapshot: %#v", failure.Details)
				}
				if !strings.Contains(failure.Hint, "停止重提并只读对账") || !strings.Contains(strings.Join(failure.Actions, " "), "不得自动重发") || failure.Cause == nil {
					t.Fatalf("unsafe recovery advice: %#v", failure)
				}
				if len(caller.calls) != 2 || caller.calls[0].tool != toolUpdate || caller.calls[1].tool != toolQuery {
					t.Fatalf("must write once then read once, got %#v", caller.calls)
				}
				for _, call := range caller.calls {
					if call.args["nodeId"] != "doc" || call.args["partId"] != "part" {
						t.Fatalf("reconciliation changed target: %#v", call)
					}
				}
				if caller.calls[0].args["mode"] != mode {
					t.Fatalf("recovery changed write mode: %#v", caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageWhiteboardCommittedFailurePreservesCauseWithoutMutatingIt(t *testing.T) {
	retryAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	original := apperrors.NewAPI("read temporarily unavailable",
		apperrors.WithOperation(serverWhiteboard+"/"+toolQuery),
		apperrors.WithReason("read_unavailable"), apperrors.WithFailureStage("request"),
		apperrors.WithExecutionStarted(false), apperrors.WithRetryable(true),
		apperrors.WithRetryAfterSeconds(5), apperrors.WithNextRetryAt(retryAt),
		apperrors.WithHint("retry read"), apperrors.WithActions("retry read once"),
		apperrors.WithTraceID("trace-read"), apperrors.WithRPCCode(-32001),
		apperrors.WithDetails(map[string]any{"readDiagnostic": "kept"}),
	).(*apperrors.Error)
	receipt := &verifiedUpdateReceipt{Message: "completed", CreatedNodeIDs: []string{"real-1"}, IDMap: map[string]string{"n1": "real-1"}}
	failure := whiteboardCommittedVerificationError(original, map[string]any{"nodeId": "doc", "partId": "part"}, "append", receipt).(*apperrors.Error)
	if !errors.Is(failure, original) || failure.Reason != original.Reason || failure.Operation != original.Operation || failure.ServerDiag.TraceID != original.ServerDiag.TraceID || failure.RPCCode != original.RPCCode || failure.Details["readDiagnostic"] != "kept" {
		t.Fatalf("lost underlying diagnostics: %#v", failure)
	}
	if failure.RetryAfterSeconds != nil || failure.NextRetryAt != nil || failure.Retryable || !*failure.ExecutionStarted {
		t.Fatalf("read-side retry advice leaked into committed write: %#v", failure)
	}
	if !original.Retryable || *original.ExecutionStarted || original.RetryAfterSeconds == nil || original.NextRetryAt == nil || original.Hint != "retry read" || original.Actions[0] != "retry read once" || len(original.Details) != 1 {
		t.Fatalf("mutated original error: %#v", original)
	}
	evidence := failure.Details["receipt"].(map[string]any)
	receipt.IDMap["n1"] = "changed"
	receipt.CreatedNodeIDs[0] = "changed"
	if evidence["idMap"].(map[string]string)["n1"] != "real-1" || evidence["createdNodeIds"].([]string)[0] != "real-1" {
		t.Fatalf("receipt evidence aliases mutable state: %#v", evidence)
	}
}

func TestCrossPlatformCoverageWhiteboardNoReceiptDoesNotClaimCommitted(t *testing.T) {
	validSource := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n1","type":"group","x":40}]}}`
	for _, tc := range []struct {
		name, source, response string
		wantCalls              int
	}{
		{"local validation", `{}`, "", 0},
		{"write transport failure", validSource, "", 1},
		{"unproven receipt", validSource, `{"success":true}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
			if tc.response != "" {
				caller.responses[toolUpdate] = []string{tc.response}
			}
			err := runWhiteboardCoverage(t, Update, caller, "", "--node", "doc", "--part-id", "part", "--source", tc.source, "--yes")
			if err == nil || len(caller.calls) != tc.wantCalls {
				t.Fatalf("err=%v calls=%#v", err, caller.calls)
			}
			var typed *apperrors.Error
			if errors.As(err, &typed) && typed.Details["commitState"] == "committed" {
				t.Fatalf("claimed committed without valid receipt: %#v", typed)
			}
		})
	}
}
