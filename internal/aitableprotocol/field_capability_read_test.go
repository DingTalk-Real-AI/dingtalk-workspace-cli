// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"errors"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageFieldCapabilityReadBoundaries(t *testing.T) {
	receipt := map[string]any{"status": "error", "error": map[string]any{"code": "500", "retryable": true}}
	for _, tc := range []struct {
		product, tool string
		body          map[string]any
	}{
		{"calendar", "list_field_groups", receipt},
		{"", "list_field_groups", receipt},
		{"aitable", "update_field_group", receipt},
		{"aitable", "", receipt},
		{"aitable", "list_field_groups", nil},
		{"aitable", "list_field_groups", map[string]any{"status": "success", "error": receipt["error"]}},
		{"aitable", "list_field_groups", map[string]any{"status": "error"}},
		{"aitable", "list_field_groups", map[string]any{"status": "error", "error": "bad"}},
		{"aitable", "list_field_groups", map[string]any{"status": "error", "error": map[string]any{"code": 500}}},
		{"aitable", "list_field_groups", map[string]any{"status": "error", "error": map[string]any{"code": " \t"}}},
	} {
		if err := FieldCapabilityReadError(tc.product, tc.tool, tc.body); err != nil {
			t.Fatalf("out-of-scope or malformed error projected: %#v: %v", tc, err)
		}
	}
}

func TestCrossPlatformCoverageFieldCapabilityReadProjection(t *testing.T) {
	for _, tool := range []string{"list_field_groups", "list_builtin_ai_field_plugins"} {
		for _, hint := range []any{true, false, "true", 1, nil} {
			for _, message := range []any{"Safe read rejection", " \t", nil, 42} {
				body := map[string]any{"status": "error", "traceId": "trace-read", "retryable": true,
					"technicalDetail": "private-detail", "actionUrl": "private-url", "friendlyHint": "private-hint",
					"error": map[string]any{"code": " InvalidRequest.DataNotReady \t", "message": message, "retryable": hint,
						"details": map[string]any{"token": "private-token"}}, "data": map[string]any{"uid": "private-uid"}}
				var typed *apperrors.Error
				if err := FieldCapabilityReadError("aitable", tool, body); !errors.As(err, &typed) {
					t.Fatalf("missing projection: %v", err)
				}
				if typed.Category != apperrors.CategoryAPI || typed.ExitCode() != 1 || typed.Origin != "mcp" || typed.Operation != "tools/call" || typed.ServerKey != "aitable" || typed.Reason != "InvalidRequest.DataNotReady" || !typed.RetryableSet || typed.Retryable != (hint == true) {
					t.Fatalf("read contract lost: %#v", typed)
				}
				if !reflect.DeepEqual(typed.ServerDiag, apperrors.ServerDiagnostics{ServerErrorCode: "InvalidRequest.DataNotReady", TraceID: "trace-read"}) || typed.Details != nil || typed.Cause != nil || typed.ExecutionStarted != nil {
					t.Fatalf("unreviewed diagnostics copied: %#v", typed)
				}
				want := "AI table read failed; inspect the error code before retrying"
				if message == "Safe read rejection" {
					want = message.(string)
				}
				if typed.Message != want {
					t.Fatalf("message = %q, want %q", typed.Message, want)
				}
			}
		}
	}
}
