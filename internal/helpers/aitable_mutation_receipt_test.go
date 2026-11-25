// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAitablePreSubmissionErrorsPreserveRetryAndIdentity(t *testing.T) {
	for _, code := range []string{"BATCH_CREATE_CHARTS_OPERATION_ERROR", "BATCH_CREATE_CHARTS_ERROR", "DASHBOARD_STORAGE_NOT_READY", "LAYOUT_CONFLICT", "ATOMIC_LAYOUT_PRECONDITION_UNAVAILABLE", "DASHBOARD_WRITE_PERMISSION_UNAVAILABLE", "CHART_LAYOUT_LICENSE_UNAVAILABLE", "DASHBOARD_WRITE_MANAGER_REQUIRED", "CHART_LAYOUT_LICENSE_REQUIRED", "CHART_LAYOUT_PREPARATION_ERROR", "FORBIDDEN"} {
		for _, retry := range []bool{false, true} {
			body := map[string]any{"status": "error", "error": map[string]any{"code": code, "message": "No write submitted", "retryable": retry, "details": map[string]any{"mutationSubmitted": false, "stage": "license", "uid": "private"}}}
			raw, _ := json.Marshal(body)
			var typed *apperrors.Error
			if err := ClassifyMCPResponseText(string(raw)); !errors.As(err, &typed) || typed.ExecutionStarted == nil || *typed.ExecutionStarted || typed.Retryable != retry || typed.ServerDiag.ServerErrorCode != code {
				t.Fatalf("lost pre-submission evidence: %#v %v", typed, err)
			}
			encoded, _ := json.Marshal(typed.Details)
			if strings.Contains(string(encoded), "private") {
				t.Fatal("unreviewed details leaked")
			}
		}
	}
}

func TestCrossPlatformCoverageAitableUnknownBatchKeepsSafeCandidatesOnBothErrorPaths(t *testing.T) {
	const receipt = `{"status":"error","data":{"baseId":"B","dashboardId":"D","uid":"private","results":[{"index":0,"chartId":"C","status":"unknown","success":false,"config":{"uid":"private"},"error":"uid=private"}]},"error":{"code":"DOWNSTREAM_REJECTED","message":"Delivery unknown","retryable":true,"details":{"deliveryStatus":"unknown"}}}`
	for _, classify := range []bool{false, true} {
		caller := &aitableAppCaller{response: receipt}
		if classify {
			caller.err = ClassifyMCPResponseText(receipt)
		}
		err := executeAitableImprovementForTest(t, caller, "chart", "batch-create", "--base-id", "B", "--dashboard-id", "D", "--charts", `[{"config":{"chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`, "--yes")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || len(caller.calls) != 1 {
			t.Fatalf("lost unknown semantics or replayed: %#v %v", typed, err)
		}
		encoded, _ := json.Marshal(typed.Details)
		if !strings.Contains(string(encoded), `"chartId":"C"`) || strings.Contains(string(encoded), "private") {
			t.Fatalf("unsafe/lost candidates: %s", encoded)
		}
	}
}

func TestCrossPlatformCoverageAitableLegacyErrorsWithoutMutationEvidenceKeepClassification(t *testing.T) {
	for _, marker := range []string{"", `,"details":{"mutationSubmitted":"false"}`} {
		err := ClassifyMCPResponseText(`{"status":"error","error":{"code":"CREATE_CHART_OPERATION_ERROR","message":"Legacy failure"` + marker + `}}`)
		var legacy *CLIError
		if !errors.As(err, &legacy) || legacy.Code != CodeMCPToolError || legacy.ServerCode != "CREATE_CHART_OPERATION_ERROR" {
			t.Fatalf("legacy error changed classification: %v", err)
		}
	}
}

func TestCrossPlatformCoverageSeatAttemptIsIndependentFromLayoutSubmission(t *testing.T) {
	for _, attempted := range []bool{true, false} {
		raw, _ := json.Marshal(map[string]any{"status": "error", "error": map[string]any{"code": "CHART_LAYOUT_LICENSE_REQUIRED", "message": "Seat allocation unconfirmed", "retryable": true, "details": map[string]any{"mutationSubmitted": false, "licenseAllocationAttempted": attempted, "licenseAllocationConfirmed": false, "uid": "private", "unreviewed": true}}})
		var typed *apperrors.Error
		err := ClassifyMCPResponseText(string(raw))
		if !errors.As(err, &typed) || typed.ExecutionStarted == nil || *typed.ExecutionStarted != attempted || typed.Retryable == attempted {
			t.Fatalf("lost independent side effect: %#v %v", typed, err)
		}
		if typed.Details["mutationSubmitted"] != false || typed.Details["licenseAllocationAttempted"] != attempted || typed.Details["licenseAllocationConfirmed"] != false || len(typed.Details) != 3 {
			t.Fatal(typed.Details)
		}
	}
}

func TestCrossPlatformCoverageUnknownSingleMutationPreservesCandidateAndNoReplay(t *testing.T) {
	const raw = `{"status":"error","error":{"code":"CREATE_CHART_OPERATION_ERROR","message":"Delivery unknown","retryable":true,"details":{"mutationSubmitted":true,"deliveryStatus":"unknown","baseId":"B","dashboardId":"D","chartId":"C","uid":"private"}}}`
	var typed *apperrors.Error
	if err := ClassifyMCPResponseText(raw); !errors.As(err, &typed) || typed.Retryable || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Details["chartId"] != "C" || typed.Details["deliveryStatus"] != "unknown" {
		t.Fatalf("lost unknown receipt: %#v %v", typed, err)
	}
	if _, ok := typed.Details["uid"]; ok {
		t.Fatal("private identity leaked")
	}
}

func TestCrossPlatformCoverageLayoutConflictKeepsIndependentSeatEffect(t *testing.T) {
	raw := `{"status":"error","error":{"code":"LAYOUT_CONFLICT","message":"No layout committed; seat allocated","retryable":false,"details":{"mutationSubmitted":false,"licenseAllocationAttempted":true,"licenseAllocationConfirmed":true}}}`
	var typed *apperrors.Error
	if err := ClassifyMCPResponseText(raw); !errors.As(err, &typed) || typed.ExecutionStarted == nil || !*typed.ExecutionStarted || typed.Retryable || typed.Details["mutationSubmitted"] != false || typed.Details["licenseAllocationConfirmed"] != true {
		t.Fatalf("lost seat conflict evidence: %#v %v", typed, err)
	}
}

func TestCrossPlatformCoverageAitableRecordRecoveryStageBoundaries(t *testing.T) {
	if err := ClassifyAitableRecordMutationError("aitable", "record_upsert", map[string]any{"status": "success", "data": map[string]any{"updatedRecordIds": []any{"r"}}}); err != nil {
		t.Fatal("successful record write was classified as a recovery error")
	}
	for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "CREATE_RECORDS_OUTCOME_UNKNOWN", "UPDATE_RECORDS_RESULT_UNKNOWN", "DUPLICATE_CLIENT_TOKEN", "UNREVIEWED_ERROR"} {
		for _, stage := range []string{"PARTIALLY_COMPLETED", "OUTCOME_UNKNOWN", "COMMITTED_RESULT_UNAVAILABLE", "REJECTED", "NOT_ATTEMPTED", ""} {
			for _, withToken := range []bool{false, true} {
				token, tool := "", ""
				if withToken {
					token, tool = "original-token", "get_record_write_result"
				}
				want := code != "UNREVIEWED_ERROR" && (withToken || stage == "PARTIALLY_COMPLETED" || stage == "OUTCOME_UNKNOWN" || stage == "COMMITTED_RESULT_UNAVAILABLE") &&
					(code == "DUPLICATE_CLIENT_TOKEN" || stage != "NOT_ATTEMPTED" && stage != "REJECTED")
				if got := AitableRecordWriteNeedsReconciliation(code, stage, token, tool); got != want {
					t.Fatalf("code=%s stage=%s token=%v: got=%v want=%v", code, stage, withToken, got, want)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageAitableRecordRecoveryDropsSensitiveMessages(t *testing.T) {
	for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "CREATE_RECORDS_OUTCOME_UNKNOWN", "UPDATE_RECORDS_RESULT_UNKNOWN", "DUPLICATE_CLIENT_TOKEN", "ATTACHMENT_WRITE_UNCONFIRMED", "DOWNSTREAM_UNAVAILABLE"} {
		for _, summaryOnly := range []bool{false, true} {
			details := map[string]any{"writeOutcome": "PARTIALLY_COMPLETED", "mutationSubmitted": true}
			if code == "ATTACHMENT_WRITE_UNCONFIRMED" {
				details["attachmentVerification"] = "unconfirmed"
			}
			if code == "DOWNSTREAM_UNAVAILABLE" {
				details["groupOutcomes"] = map[string]any{"update": map[string]any{"requested": true, "error": map[string]any{"code": "UPDATE_RECORDS_RESULT_UNKNOWN", "details": map[string]any{"writeOutcome": "OUTCOME_UNKNOWN"}}}}
			}
			secret := "fileToken=private-file-canary https://upload.example.test/object?Signature=private-signature-canary"
			upstream := map[string]any{"code": code, "retryable": false, "details": details}
			body := map[string]any{"status": "error", "error": upstream, "summary": secret, "logId": "private-log-canary"}
			if !summaryOnly {
				upstream["message"] = secret
			}
			for _, classify := range []func(map[string]any) error{func(content map[string]any) error {
				return ClassifyAitableRecordMutationError("aitable", "record_upsert", content)
			}, func(content map[string]any) error {
				raw, _ := json.Marshal(content)
				return classifyAitableRecordTextForTest("aitable", "record_upsert", string(raw))
			}} {
				err := classify(body)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Retryable || typed.Reason != code {
					t.Fatalf("recovery classification changed: code=%s summaryOnly=%v err=%v", code, summaryOnly, err)
				}
				encoded, _ := json.Marshal(typed)
				if strings.Contains(string(encoded), "private-") || strings.Contains(err.Error(), "private-") {
					t.Errorf("recovery diagnostic retained an upstream message: code=%s summaryOnly=%v", code, summaryOnly)
				}
			}
		}
	}
}
