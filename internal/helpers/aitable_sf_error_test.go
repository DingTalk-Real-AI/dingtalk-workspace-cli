// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAitableSFFailureStagesAcrossMCPPaths(t *testing.T) {
	for _, code := range []string{
		"FIELD_GROUP_CREDENTIAL_REJECTED", "FIELD_GROUP_SERVICE_UNAVAILABLE",
		"FIELD_GROUP_PERMISSION_REJECTED", "FIELD_GROUP_DATA_NOT_READY", "FIELD_GROUP_OUTCOME_UNKNOWN",
		"FIELD_GROUP_PRECHECK_FAILED", "FIELD_GROUP_INPUT_REJECTED",
	} {
		for _, retryable := range []any{true, false, "true", nil} {
			for _, path := range []string{"data", "direct"} {
				t.Run(fmt.Sprintf("%s/%v/%s", code, retryable, path), func(t *testing.T) {
					upstream := map[string]any{"code": code, "message": "Safe stage rejection",
						"details": map[string]any{"uid": "private"}}
					if retryable != nil {
						upstream["retryable"] = retryable
					}
					raw, _ := json.Marshal(map[string]any{"status": "error", "error": upstream,
						"data": map[string]any{"uid": "private", "token": "secret"}})
					caller := &aitableAppCaller{response: string(raw)}
					InitDepsForTest(t, caller)
					var err error
					switch path {
					case "data":
						_, err = CallMCPToolDataOnServer(t.Context(), "aitable", "ungroup_field_group", nil)
					case "direct":
						err = CallMCPToolOnServer("aitable", "ungroup_field_group", nil)
					}
					var typed *apperrors.Error
					if !errors.As(err, &typed) {
						t.Fatalf("stage was not projected: %T %v", err, err)
					}
					if typed.Category != apperrors.CategoryAPI || typed.ExitCode() != 1 || typed.Origin != "mcp" || typed.Operation != "tools/call" || typed.ServerKey != "aitable" ||
						typed.ServerDiag.ServerErrorCode != code || typed.Reason != code || typed.Message != "Safe stage rejection" {
						t.Fatalf("lost stage or changed classification: %#v", typed)
					}
					preSubmissionRetry := code == "FIELD_GROUP_DATA_NOT_READY" || code == "FIELD_GROUP_SERVICE_UNAVAILABLE" || code == "FIELD_GROUP_PRECHECK_FAILED"
					if !typed.RetryableSet || typed.Retryable != (preSubmissionRetry && retryable == true) {
						t.Fatalf("unsafe or missing retry hint: %#v", typed)
					}
					if typed.Details != nil || typed.Cause != nil || typed.ExecutionStarted != nil {
						t.Fatalf("invented execution evidence or copied unreviewed response: %#v", typed)
					}
					if len(caller.calls) != 1 {
						t.Fatalf("replayed mutation: %#v", caller.calls)
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageAitableSFProjectionBoundaries(t *testing.T) {
	for _, raw := range []string{
		`{"status":"success","error":{"code":"FIELD_GROUP_DATA_NOT_READY","retryable":true}}`,
		`{"error":{"code":"FIELD_GROUP_DATA_NOT_READY","retryable":true}}`,
		`{"status":"error","error":"FIELD_GROUP_DATA_NOT_READY"}`,
		`{"status":"error","error":{"code":123,"retryable":true}}`,
		`{"status":"error","error":{"code":"FIELD_GROUP_FUTURE_ERROR","retryable":true}}`,
	} {
		var body map[string]any
		_ = json.Unmarshal([]byte(raw), &body)
		if err := classifyAitableMutationError(body); err != nil {
			t.Fatalf("unreviewed shape entered typed stage projection: %s: %v", raw, err)
		}
	}
	const raw = `{"status":"error","error":{"code":"FIELD_GROUP_DATA_NOT_READY"},"data":{"token":"secret"}}`
	var typed *apperrors.Error
	fallback := &aitableAppCaller{response: raw}
	InitDepsForTest(t, fallback)
	_, fallbackErr := CallMCPToolDataOnServer(t.Context(), "aitable", "ungroup_field_group", nil)
	if err := fallbackErr; !errors.As(err, &typed) || typed.Retryable ||
		typed.Message != "AI table mutation failed; inspect the stage code before another write" {
		t.Fatalf("missing safe fallback: %#v %v", typed, err)
	}
	// Generic/legacy callers still consume the raw JSON for their own recovery.
	const legacyRaw = `{"status":"error","error":{"code":"FIELD_GROUP_FUTURE_ERROR","message":"Legacy failure","retryable":true}}`
	caller := &aitableAppCaller{response: legacyRaw}
	InitDepsForTest(t, caller)
	_, err := CallMCPToolTextOnServer("aitable", "ungroup_field_group", nil)
	var legacy *CLIError
	if !errors.As(err, &legacy) || legacy.Code != CodeMCPToolError || legacy.Message != legacyRaw || len(caller.calls) != 1 {
		t.Fatalf("legacy raw response contract changed: %#v %v", legacy, err)
	}
}

func TestCrossPlatformCoverageAitableSFReadFailureHints(t *testing.T) {
	for _, tool := range []string{"list_field_groups", "list_builtin_ai_field_plugins"} {
		for _, hint := range []any{true, false, "true", nil} {
			t.Run(fmt.Sprintf("%s/%v", tool, hint), func(t *testing.T) {
				upstream := map[string]any{"code": "InvalidRequest.DataNotReady"}
				if hint != nil {
					upstream["retryable"] = hint
				}
				raw, _ := json.Marshal(map[string]any{"status": "error", "error": upstream})
				caller := &aitableAppCaller{response: string(raw)}
				InitDepsForTest(t, caller)
				_, err := callAitableUnifiedDataContext(t.Context(), tool, nil)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || !typed.RetryableSet || typed.Retryable != (hint == true) || typed.ServerDiag.ServerErrorCode != upstream["code"] {
					t.Fatalf("invalid read hint projection: %#v %v", typed, err)
				}
				if typed.Message != "AI table read failed; inspect the error code before retrying" || len(caller.calls) != 1 || isAitableReadRetryTool(tool) {
					t.Fatalf("missing fallback or automatic retry added: %#v %#v", typed, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableSFReadProjectionBoundaries(t *testing.T) {
	for _, tc := range []struct{ product, tool, response string }{
		{"calendar", "list_field_groups", `{"status":"error","error":{"code":"InvalidRequest.DataNotReady","retryable":true}}`},
		{"aitable", "create_field_group", `{"status":"error","error":{"code":"InvalidRequest.DataNotReady","retryable":true}}`},
		{"aitable", "get_fields", `{"status":"error","error":{"code":"InvalidRequest.DataNotReady","retryable":true}}`},
		{"aitable", "list_field_groups", `null`},
		{"aitable", "list_field_groups", `{"status":"success","error":{"code":"InvalidRequest.DataNotReady","retryable":true}}`},
		{"aitable", "list_field_groups", `{"error":{"code":"InvalidRequest.DataNotReady","retryable":true}}`},
		{"aitable", "list_field_groups", `{"status":"error","error":"InvalidRequest.DataNotReady"}`},
		{"aitable", "list_field_groups", `{"status":"error","error":{"code":500,"retryable":true}}`},
		{"aitable", "list_field_groups", `{"status":"error","error":{"code":"  ","retryable":true}}`},
	} {
		t.Run(tc.product+"/"+tc.tool+"/"+tc.response, func(t *testing.T) {
			caller := &aitableAppCaller{response: tc.response}
			InitDepsForTest(t, caller)
			_, err := CallMCPToolDataOnServer(t.Context(), tc.product, tc.tool, nil)
			var typed *apperrors.Error
			if errors.As(err, &typed) || len(caller.calls) != 1 {
				t.Fatalf("unreviewed request entered read projection: %#v %v", typed, err)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableSFMutationContextBoundaries(t *testing.T) {
	const raw = `{"status":"error","error":{"code":"FIELD_GROUP_DATA_NOT_READY","retryable":true,"message":"Other tool failure"}}`
	for _, tc := range []struct{ product, tool string }{
		{"calendar", "ungroup_field_group"}, {"aitable", "get_fields"},
	} {
		for _, direct := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/direct=%t", tc.product, tc.tool, direct), func(t *testing.T) {
				caller := &aitableAppCaller{response: raw}
				InitDepsForTest(t, caller)
				var err error
				if direct {
					err = CallMCPToolOnServer(tc.product, tc.tool, nil)
				} else {
					_, err = CallMCPToolDataOnServer(t.Context(), tc.product, tc.tool, nil)
				}
				var typed *apperrors.Error
				if err == nil || errors.As(err, &typed) || len(caller.calls) != 1 {
					t.Fatalf("unrelated tool entered stage projection: err=%#v calls=%#v", err, caller.calls)
				}
			})
		}
	}
	var typed *apperrors.Error
	if err := ClassifyMCPResponseText(raw); err == nil || errors.As(err, &typed) {
		t.Fatalf("context-free classifier granted field-group semantics: %#v", err)
	}
}
