// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageFieldGroupMutationStages(t *testing.T) {
	for code, preSubmission := range map[string]bool{
		"FIELD_GROUP_SERVICE_UNAVAILABLE": true, "FIELD_GROUP_PRECHECK_FAILED": true,
		"FIELD_GROUP_DATA_NOT_READY": true, "FIELD_GROUP_CREDENTIAL_REJECTED": false,
		"FIELD_GROUP_PERMISSION_REJECTED": false, "FIELD_GROUP_INPUT_REJECTED": false,
		"FIELD_GROUP_OUTCOME_UNKNOWN": false,
	} {
		for _, hint := range []any{true, false, "true", nil} {
			for _, message := range []string{"Safe stage rejection", " \t"} {
				body := map[string]any{"status": "error", "requestId": "trace-write", "retryable": true, "technicalDetail": "private-detail", "error": map[string]any{
					"code": code, "retryable": hint, "message": message,
					"details": map[string]any{"token": "private"},
				}, "data": map[string]any{"uid": "private"}}
				var typed *apperrors.Error
				if err := FieldGroupMutationError("aitable", "ungroup_field_group", body); !errors.As(err, &typed) {
					t.Fatalf("stage was not projected: %s %v", code, err)
				}
				if typed.Category != apperrors.CategoryAPI || typed.ExitCode() != 1 || typed.Origin != "mcp" || typed.Operation != "tools/call" || typed.ServerKey != "aitable" || typed.Reason != code || typed.ServerDiag.ServerErrorCode != code || !typed.RetryableSet || typed.Retryable != (preSubmission && hint == true) {
					t.Fatalf("stage contract lost: %#v", typed)
				}
				wantMessage := message
				if message == " \t" {
					wantMessage = "AI table mutation failed; inspect the stage code before another write"
				}
				if typed.Message != wantMessage || typed.Details != nil || typed.Cause != nil || typed.ExecutionStarted != nil {
					t.Fatalf("copied unreviewed diagnostics: %#v", typed)
				}
				if !reflect.DeepEqual(typed.ServerDiag, apperrors.ServerDiagnostics{ServerErrorCode: code, TraceID: "trace-write"}) {
					t.Fatalf("trace lost or unreviewed diagnostics copied: %#v", typed.ServerDiag)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageFieldGroupMutationBoundaries(t *testing.T) {
	for _, body := range []map[string]any{
		nil,
		{"status": "success", "error": map[string]any{"code": "FIELD_GROUP_DATA_NOT_READY"}},
		{"status": "error", "error": "FIELD_GROUP_DATA_NOT_READY"},
		{"status": "error", "error": map[string]any{"code": 500}},
		{"status": "error", "error": map[string]any{"code": "FIELD_GROUP_FUTURE_ERROR", "retryable": true}},
	} {
		if err := FieldGroupMutationError("aitable", "ungroup_field_group", body); err != nil {
			t.Fatalf("unreviewed receipt entered projection: %#v %v", body, err)
		}
	}
}

func TestCrossPlatformCoverageFieldGroupMutationScope(t *testing.T) {
	body := map[string]any{"status": "error", "error": map[string]any{"code": "FIELD_GROUP_DATA_NOT_READY", "retryable": true}}
	for _, tc := range []struct {
		product, tool string
		want          bool
	}{
		{"aitable", "create_field_group", true}, {"aitable", "update_field_group", true},
		{"aitable", "ungroup_field_group", true}, {"aitable", "move_field_group_fields", true},
		{"calendar", "ungroup_field_group", false}, {"", "ungroup_field_group", false},
		{"aitable", "get_fields", false}, {"aitable", "list_field_groups", false}, {"aitable", "", false},
	} {
		err := FieldGroupMutationError(tc.product, tc.tool, body)
		if (err != nil) != tc.want {
			t.Fatalf("scope %s/%s projected=%v want=%t", tc.product, tc.tool, err, tc.want)
		}
	}
}
