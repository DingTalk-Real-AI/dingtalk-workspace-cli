// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func aitableBaseNotFoundFixture() map[string]any {
	return map[string]any{
		"data":     map[string]any{},
		"status":   "error",
		"success":  true,
		"summary":  "Failed to get base because base does not exist or is inaccessible",
		"trace_id": "trace-redacted",
		"error": map[string]any{
			"code":      "BASE_NOT_FOUND",
			"message":   "Specified base does not exist, has been deleted, or is inaccessible",
			"retryable": false,
			"type":      "INPUT_ERROR",
		},
	}
}

func TestCrossPlatformCoverageAITableBaseNotFoundSurvivesHelperPipeline(t *testing.T) {
	text := `{"data":{},"error":{"code":"BASE_NOT_FOUND","message":"Specified base does not exist, has been deleted, or is inaccessible","retryable":false,"type":"INPUT_ERROR"},"meta":{},"status":"error","success":true,"summary":"Failed to get base because base does not exist or is inaccessible","trace_id":"trace-redacted"}`
	gotText, err := parseMCPToolTextResult("aitable", "get_base", textToolResult(text), nil)
	if gotText != "" {
		t.Fatalf("text = %q, want empty on classified error", gotText)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryAPI || typed.Reason != "not_found" || typed.ExitCode() != apperrors.ExitCodeAPI {
		t.Fatalf("classification = category %q reason %q exit %d", typed.Category, typed.Reason, typed.ExitCode())
	}
	if typed.Operation != "aitable/get_base" || typed.ServerDiag.ServerErrorCode != "BASE_NOT_FOUND" {
		t.Fatalf("classification context = operation %q diagnostics %#v", typed.Operation, typed.ServerDiag)
	}
	if !typed.RetryableSet || typed.Retryable {
		t.Fatalf("retryability = (%v, %v), want explicit false", typed.RetryableSet, typed.Retryable)
	}
}

func TestCrossPlatformCoverageAITableBaseNotFoundSurvivesTerminalCallPipeline(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"data":{},"error":{"code":"BASE_NOT_FOUND","message":"Specified base does not exist","retryable":false,"type":"INPUT_ERROR"},"status":"error","success":true}`}}
	InitDepsForTest(t, caller)
	err := callMCPToolInternalOpts("aitable", "get_base", map[string]any{"baseId": "base-redacted"}, false)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "not_found" || typed.Operation != "aitable/get_base" || strings.TrimSpace(typed.Message) == "" {
		t.Fatalf("terminal pipeline error = %#v", err)
	}
}

func TestCrossPlatformCoverageAITableBaseNotFoundExactBoundaries(t *testing.T) {
	tests := []struct {
		name, serverID, toolName string
		mutate                   func(map[string]any)
	}{
		{name: "exact", serverID: "aitable", toolName: "get_base"},
		{name: "different service", serverID: "other", toolName: "get_base"},
		{name: "different tool", serverID: "aitable", toolName: "delete_base"},
		{name: "adjacent invalid base id", serverID: "aitable", toolName: "get_base", mutate: func(body map[string]any) {
			body["error"].(map[string]any)["code"] = "INVALID_BASE_ID"
		}},
		{name: "unstructured phrase", serverID: "aitable", toolName: "get_base", mutate: func(body map[string]any) {
			body["error"] = "Specified base does not exist"
		}},
		{name: "wrong status", serverID: "aitable", toolName: "get_base", mutate: func(body map[string]any) {
			body["status"] = "success"
		}},
		{name: "success false", serverID: "aitable", toolName: "get_base", mutate: func(body map[string]any) {
			body["success"] = false
		}},
		{name: "retryable", serverID: "aitable", toolName: "get_base", mutate: func(body map[string]any) {
			body["error"].(map[string]any)["retryable"] = true
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := aitableBaseNotFoundFixture()
			if test.mutate != nil {
				test.mutate(body)
			}
			err := classifyServiceBusinessError(body, test.serverID, test.toolName)
			if test.name == "exact" {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != "not_found" {
					t.Fatalf("exact fixture = %#v, want typed not_found", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("adjacent response was widened into not_found: %#v", err)
			}
		})
	}
}
