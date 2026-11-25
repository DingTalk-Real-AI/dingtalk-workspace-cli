// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAitableRecordLegacyBoundaryRedactsOnlyRecovery(t *testing.T) {
	for _, tc := range []struct {
		product, tool, code, status, outcome string
		wantRecovery                         bool
	}{
		{"aitable", "create_records", "CREATE_RECORDS_OUTCOME_UNKNOWN", "error", "OUTCOME_UNKNOWN", true},
		{"aitable-helper", "record_upsert", "DUPLICATE_CLIENT_TOKEN", "error", "OUTCOME_UNKNOWN", true},
		{"drive", "create_records", "CREATE_RECORDS_OUTCOME_UNKNOWN", "error", "OUTCOME_UNKNOWN", false},
		{"aitable", "query_records", "CREATE_RECORDS_OUTCOME_UNKNOWN", "error", "OUTCOME_UNKNOWN", false},
		{"aitable", "update_records", "INPUT_REJECTED", "error", "OUTCOME_UNKNOWN", false},
		{"aitable", "create_records", "CREATE_RECORDS_OUTCOME_UNKNOWN", "success", "OUTCOME_UNKNOWN", false},
		// Explicit pre-submission evidence wins over a token/reconcile fallback.
		{"aitable", "update_records", "ATTACHMENT_PREPARATION_UNAVAILABLE", "error", "REJECTED", false},
		{"aitable-helper", "record_upsert", "UPDATE_RECORDS_RESULT_UNKNOWN", "error", "NOT_ATTEMPTED", false},
		// Duplicate rejection concerns this attempt; the original token still needs reconciliation.
		{"aitable-helper", "record_upsert", "DUPLICATE_CLIENT_TOKEN", "error", "REJECTED", true},
	} {
		t.Run(tc.product+"/"+tc.tool+"/"+tc.code+"/"+tc.status+"/"+tc.outcome, func(t *testing.T) {
			body := map[string]any{"status": tc.status, "logId": "private-log", "summary": "private-summary",
				"error": map[string]any{"code": tc.code, "message": "private-message", "retryable": true,
					"details": map[string]any{"writeOutcome": tc.outcome, "clientToken": "original",
						"recordIds": []any{"r"}, "reconcileTool": "get_record_write_result", "cells": "private-cell"}}}
			before, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			projected := ClassifyAitableRecordMutationError(tc.product, tc.tool, body)
			if (projected != nil) != tc.wantRecovery {
				t.Fatalf("wrong recovery obligation for %s: %v", tc.outcome, projected)
			}
			afterPure, _ := json.Marshal(body)
			if !bytes.Equal(before, afterPure) {
				t.Fatal("pure record projection changed its input")
			}
			legacy := FieldGroupMutationError(tc.product, tc.tool, body)
			after, _ := json.Marshal(body)
			if projected == nil {
				if legacy != nil || !bytes.Equal(before, after) {
					t.Fatal("legacy boundary changed an unrelated receipt")
				}
				return
			}
			var typed *apperrors.Error
			if !errors.As(legacy, &typed) || typed.Reason != tc.code || typed.Retryable ||
				typed.ExecutionStarted == nil || !*typed.ExecutionStarted ||
				typed.Details["clientToken"] != "original" || !reflect.DeepEqual(typed.Details["recordIds"], []string{"r"}) {
				t.Fatal("legacy boundary lost the original recovery contract")
			}
			if bytes.Contains(after, []byte("private-")) || body["status"] != "error" || len(body) != 2 ||
				!reflect.DeepEqual(body["error"].(map[string]any)["details"], typed.Details) {
				t.Fatal("legacy caller retained raw diagnostics or lost safe recovery targets")
			}
		})
	}
}

func TestCrossPlatformCoverageAitableRecoveryRejectsDiagnosticControls(t *testing.T) {
	for _, code := range []string{"", "BAD\x1b[31m", "BAD\x00", "private-code-canary", strings.Repeat("X", 257)} {
		details := map[string]any{"groupOutcomes": map[string]any{"update": map[string]any{
			"requested": true, "requestedRecordIds": []any{"r"}, "error": map[string]any{
				"code": code, "details": map[string]any{"writeOutcome": "OUTCOME_UNKNOWN"},
			},
		}}}
		body := map[string]any{"status": "error", "error": map[string]any{"code": code, "details": details}}
		var typed *apperrors.Error
		if !errors.As(ClassifyAitableRecordMutationError("aitable", "record_upsert", body), &typed) ||
			typed.Reason != "RECORD_WRITE_REQUIRES_RECONCILIATION" || typed.ServerDiag.ServerErrorCode != typed.Reason || typed.Retryable {
			t.Fatalf("unsafe code must retain non-replay protection under a safe reason: %#v", typed)
		}
		group := typed.Details["groupOutcomes"].(map[string]any)["update"].(map[string]any)
		if group["error"].(map[string]any)["code"] != typed.Reason {
			t.Fatal("group code escaped the same diagnostic boundary")
		}
	}
	for _, unsafe := range []string{" ", "r\x1b", "r\x00", "r\u0085", "r\n", strings.Repeat("r", 257)} {
		if safeAitableRecoveryString(unsafe) || safeAitableRecordIDs([]any{unsafe}) != nil ||
			AitableRecordWriteNeedsReconciliation("CREATE_RECORDS_OUTCOME_UNKNOWN", "", unsafe, "get_record_write_result") {
			t.Fatal("unsafe recovery identity accepted")
		}
	}
}

func TestCrossPlatformCoverageAitableMissingRequestedKeepsRecoveryEvidence(t *testing.T) {
	for _, requested := range []any{nil, "true"} {
		for _, evidence := range []string{"error", "requestedRecordIds", "acceptedRecordIds"} {
			group := map[string]any{}
			if requested != nil {
				group["requested"] = requested
			}
			if evidence == "error" {
				group[evidence] = map[string]any{"code": "DOWNSTREAM_UNAVAILABLE", "message": "private-canary"}
			} else {
				group[evidence] = []any{"r"}
			}
			body := map[string]any{"status": "error", "error": map[string]any{"code": "DOWNSTREAM_UNAVAILABLE",
				"details": map[string]any{"groupOutcomes": map[string]any{"update": group}}}}
			var typed *apperrors.Error
			if !errors.As(ClassifyAitableRecordMutationError("aitable", "record_upsert", body), &typed) || typed.Retryable {
				t.Fatal("missing optional requested must not discard uncertain group evidence")
			}
			got := typed.Details["groupOutcomes"].(map[string]any)["update"].(map[string]any)
			if _, invented := got["requested"]; invented || got[evidence] == nil {
				t.Fatalf("optional requested was invented or evidence lost: %#v", got)
			}
		}
	}
	for _, stage := range []string{"NOT_ATTEMPTED", "REJECTED"} {
		for _, requested := range []any{nil, true} {
			for _, code := range []string{"ATTACHMENT_PREPARATION_UNAVAILABLE", "DUPLICATE_CLIENT_TOKEN"} {
				group := map[string]any{"error": map[string]any{"code": code, "details": map[string]any{
					"writeOutcome": stage, "mutationSubmitted": false, "clientToken": "original", "reconcileTool": "get_record_write_result",
				}}}
				if requested != nil {
					group["requested"] = requested
				}
				groups, recovery := ProjectAitableRecordGroupOutcomes(map[string]any{"update": group})
				if len(groups) != 1 || recovery != (code == "DUPLICATE_CLIENT_TOKEN") {
					t.Fatalf("explicit stage/original token semantics lost: %#v, recovery=%v", groups, recovery)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageAitableMixedAttachmentKeepsWriteStage(t *testing.T) {
	for _, token := range []string{"original", "   ", "private\x1b"} {
		details := map[string]any{"attachmentVerification": "unconfirmed", "writeOutcome": "PARTIALLY_COMPLETED",
			"mutationSubmitted": true, "clientToken": token, "reconcileTool": "get_record_write_result",
			"recordIds": []any{"r"}, "groupOutcomes": map[string]any{"update": map[string]any{
				"requestedRecordIds": []any{"r"},
			}}, "private": "private-canary"}
		body := map[string]any{"status": "error", "error": map[string]any{"code": "CREATE_RECORDS_OUTCOME_UNKNOWN", "details": details}}
		var typed *apperrors.Error
		if !errors.As(ClassifyAitableRecordMutationError("aitable", "record_upsert", body), &typed) || typed.Retryable ||
			typed.Details["writeOutcome"] != "PARTIALLY_COMPLETED" || typed.Details["mutationSubmitted"] != true ||
			!reflect.DeepEqual(typed.Details["recordIds"], []string{"r"}) || typed.Details["groupOutcomes"] == nil {
			t.Fatalf("mixed attachment stage/targets lost: %#v", typed)
		}
		if (typed.Details["clientToken"] != nil) != (token == "original") {
			t.Fatal("attachment identities used a different validation boundary")
		}
		raw, err := json.Marshal(typed.Details)
		if err != nil || strings.Contains(string(raw), "private") {
			t.Fatal("mixed recovery retained unreviewed diagnostics")
		}
	}
}
