// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAitableUpsertGroupRecoverySurvivesMCPWire(t *testing.T) {
	for _, code := range []string{"UPDATE_RECORDS_RESULT_UNKNOWN", "UPDATE_RECORDS_ERROR", "500"} {
		for _, mixed := range []bool{false, true} {
			t.Run(code+"/mixed="+map[bool]string{true: "true", false: "false"}[mixed], func(t *testing.T) {
				groups := map[string]any{
					"create": map[string]any{"requested": false},
					"update": map[string]any{"requested": true, "requestedRecordIds": []any{"original-update"},
						"acceptedRecordIds": []any{}, "cells": "private-cell", "error": map[string]any{
							"code": code, "retryable": true, "message": "private-message", "details": map[string]any{
								"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true, "uid": "private-uid",
								"baseId": "base", "tableId": "table", "response": "private-response",
							}}},
				}
				details := map[string]any{"groupOutcomes": groups, "baseId": "base", "tableId": "table",
					"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true}
				rootCode := code
				if mixed {
					rootCode = "ATTACHMENT_WRITE_UNCONFIRMED"
					details["attachmentVerification"] = "unconfirmed"
					details["clientToken"] = "original-token"
					details["createdRecordIds"] = []any{"created"}
					groups["create"] = map[string]any{"requested": true, "acceptedRecordIds": []any{"created"},
						"error": map[string]any{"code": rootCode, "details": map[string]any{
							"attachmentVerification": "unconfirmed", "clientToken": "original-token", "recordIds": []any{"created"},
							"fieldIds": []any{"attachment"}, "reconcileTool": "get_record_write_result",
						}}}
				}
				body := map[string]any{"status": "error", "summary": "Read original groups without replay",
					"error": map[string]any{"code": rootCode, "details": details}}
				wire, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				var typed *apperrors.Error
				if !errors.As(classifyAitableRecordTextForTest("aitable", "record_upsert", string(wire)), &typed) || typed.ExecutionStarted == nil ||
					!*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
					t.Fatalf("uncertain group lost write protection: %#v", typed)
				}
				got := typed.Details["groupOutcomes"].(map[string]any)
				update := got["update"].(map[string]any)
				if !reflect.DeepEqual(update["requestedRecordIds"], []string{"original-update"}) ||
					!reflect.DeepEqual(update["acceptedRecordIds"], []string{}) {
					t.Fatalf("requested target was lost or reported as accepted: %#v", update)
				}
				groupError := update["error"].(map[string]any)
				stage := groupError["details"].(map[string]any)
				if groupError["code"] != code || groupError["retryable"] != false || stage["writeOutcome"] != "OUTCOME_UNKNOWN" || stage["mutationSubmitted"] != true {
					t.Fatalf("update stage lost: %#v", groupError)
				}
				if mixed {
					createError := got["create"].(map[string]any)["error"].(map[string]any)
					if createError["details"].(map[string]any)["clientToken"] != "original-token" {
						t.Fatalf("create reconciliation lost: %#v", got)
					}
				} else if typed.Details["attachmentVerification"] != nil || got["create"].(map[string]any)["requested"] != false {
					t.Fatalf("unrequested attachment/create recovery invented: %#v", typed.Details)
				}
				projected, _ := json.Marshal(typed.Details)
				if strings.Contains(string(projected), "private-") {
					t.Fatalf("unreviewed diagnostics leaked: %s", projected)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableSingleRecordRecoverySurvivesMCPWire(t *testing.T) {
	for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "CREATE_RECORDS_OUTCOME_UNKNOWN", "UPDATE_RECORDS_RESULT_UNKNOWN", "DUPLICATE_CLIENT_TOKEN"} {
		t.Run(code, func(t *testing.T) {
			wire, err := json.Marshal(map[string]any{"status": "error", "error": map[string]any{
				"type": "SYSTEM_ERROR", "code": code, "message": "Query original targets without replay", "retryable": false,
				"details": map[string]any{"baseId": "B", "tableId": "T", "clientToken": "original-token",
					"reconcileTool": "get_record_write_result", "writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true,
					"uid": "private-uid", "cells": "private-cells"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			var typed *apperrors.Error
			if !errors.As(classifyAitableRecordTextForTest("aitable", "record_upsert", string(wire)), &typed) || typed.ExecutionStarted == nil ||
				!*typed.ExecutionStarted || !typed.RetryableSet || typed.Retryable {
				t.Fatalf("single record recovery lost its write protection: %#v", typed)
			}
			if typed.Details["clientToken"] != "original-token" || typed.Details["reconcileTool"] != "get_record_write_result" ||
				typed.Details["writeOutcome"] != "OUTCOME_UNKNOWN" || typed.Details["mutationSubmitted"] != true ||
				typed.Details["uid"] != nil || typed.Details["cells"] != nil {
				t.Fatalf("record recovery details were lost or leaked: %#v", typed.Details)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableSingleRecordRecoveryRequiresReviewedEvidence(t *testing.T) {
	for _, tc := range []struct {
		code    string
		details map[string]any
	}{
		{"ATTACHMENT_PREPARATION_UNAVAILABLE", nil},
		{"ATTACHMENT_PREPARATION_UNAVAILABLE", map[string]any{"writeOutcome": "REJECTED", "mutationSubmitted": true}},
		{"UPDATE_RECORDS_RESULT_UNKNOWN", map[string]any{"writeOutcome": "NOT_ATTEMPTED"}},
		{"UNREVIEWED_WRITE_ERROR", map[string]any{"writeOutcome": "OUTCOME_UNKNOWN", "mutationSubmitted": true}},
		{"DUPLICATE_CLIENT_TOKEN", map[string]any{"clientToken": "token", "reconcileTool": "unreviewed_tool"}},
	} {
		body := map[string]any{"status": "error", "error": map[string]any{"code": tc.code, "details": tc.details}}
		if err := ClassifyAitableRecordMutationError("aitable", "record_upsert", body); err != nil {
			t.Fatalf("invented single record recovery for %#v: %v", body, err)
		}
	}
}

func TestCrossPlatformCoverageAitableUpsertGroupRecoveryRejectsMalformedTargets(t *testing.T) {
	for _, ids := range []any{[]any{"target", 1}, make([]any, 101), []any{"target\nprivate"}, []any{strings.Repeat("x", 257)}} {
		groups, recovery := ProjectAitableRecordGroupOutcomes(map[string]any{
			"ignored": map[string]any{"requested": true},
			"create":  map[string]any{"requested": false, "error": map[string]any{"code": "ATTACHMENT_WRITE_UNCONFIRMED"}},
			"update": map[string]any{"requested": true, "requestedRecordIds": ids, "acceptedRecordIds": ids,
				"error": map[string]any{"code": "REJECTED", "details": map[string]any{
					"writeOutcome": "REJECTED", "mutationSubmitted": true, "reconcileTool": "unknown_tool", "recordIds": ids,
				}}},
		})
		update := groups["update"].(map[string]any)
		details := update["error"].(map[string]any)["details"].(map[string]any)
		if recovery || len(groups) != 2 || update["requestedRecordIds"] != nil || update["acceptedRecordIds"] != nil || details["recordIds"] != nil || details["reconcileTool"] != nil || len(groups["create"].(map[string]any)) != 1 {
			t.Fatalf("malformed target or unrequested error accepted: %#v", groups)
		}
	}
}
