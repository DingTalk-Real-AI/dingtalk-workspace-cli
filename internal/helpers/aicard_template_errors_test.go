// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageAicardTemplatePreservesServiceDiagnostics(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	for _, result := range []transport.ToolCallResult{
		{StructuredContent: map[string]any{"code": "NOT_CREATOR", "message": "Only the creator can update"}},
		{StructuredContent: map[string]any{"success": false, "errorCode": "NOT_CREATOR", "errorMsg": "Only the creator can update", "file": "private-file-canary"}},
		{IsError: true, StructuredContent: map[string]any{"error": map[string]any{"code": "NOT_CREATOR", "message": "Only the creator can update"}}},
		{IsError: true, Content: map[string]any{"code": "NOT_CREATOR", "message": "Only the creator can update"}},
		{IsError: true, Blocks: []transport.ContentBlock{{Type: "text", Text: `{"errorCode":"NOT_CREATOR","errorMessage":"Only the creator can update"}`}}},
	} {
		caller := &aicardTemplateTestCaller{result: result}
		_, err := executeAicardTemplate(t, caller, "update", "--template-id", "t", "--file", aicardTestFile(t, aicardTestSnapshot))
		var classified *apperrors.Error
		if !errors.As(err, &classified) || classified.Details["serviceErrorCode"] != "NOT_CREATOR" || classified.Details["serviceMessage"] != "Only the creator can update" || caller.invocations != 1 || classified.Retryable {
			t.Fatalf("service diagnostics were lost: %v", err)
		}
		encoded, marshalErr := json.Marshal(classified.Details)
		if marshalErr != nil || strings.Contains(string(encoded), "private-file-canary") {
			t.Fatal("remote template leaked into error diagnostics")
		}
	}
}

func TestCrossPlatformCoverageAicardTemplateDiagnosticBounds(t *testing.T) {
	message := aicardTemplateDiagnostic(map[string]any{"message": "  " + strings.Repeat("a", 2048)}, "message")
	if len(message) > 1024 || message == "" {
		t.Fatal("service diagnostic is not bounded")
	}
	if got := aicardTemplateDiagnostic(map[string]any{"empty": " ", "message": "a\r\nb\u202ec"}, "empty", "message"); got != "a  b c" {
		t.Fatalf("unsafe diagnostic characters: %q", got)
	}
	if got := aicardTemplateDiagnostic(nil, "message"); got != "" {
		t.Fatal(got)
	}
	if got := aicardTemplateDiagnostic(map[string]any{"code": json.Number("404")}, "code"); got != "404" {
		t.Fatal("numeric service error code was lost")
	}
	// Internationalized diagnostics must stay valid UTF-8 at the byte boundary.
	if got := aicardTemplateDiagnostic(map[string]any{"message": strings.Repeat("a", 1023) + "界"}, "message"); len(got) != 1023 {
		t.Fatal("multibyte diagnostic exceeded the byte budget")
	}
}

func TestCrossPlatformCoverageAicardTemplateEscapedBudgetStaysLocal(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	// This is an intentional Unicode transport-boundary fixture.
	file := aicardTestFile(t, strings.Replace(aicardTestSnapshot, "中文", strings.Repeat("\u200d", 140000), 1))
	for _, flags := range [][]string{nil, {"--dry-run"}} {
		caller := &aicardTemplateTestCaller{}
		args := append([]string{"create", "--file", file}, flags...)
		_, err := executeAicardTemplate(t, caller, args...)
		if err == nil || caller.invocations != 0 || !strings.Contains(err.Error(), "after JSON escaping") {
			t.Fatalf("encoded budget crossed the service boundary: calls=%d err=%v", caller.invocations, err)
		}
	}
}
