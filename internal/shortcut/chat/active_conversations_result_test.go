// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageActiveConversationsPageErrorDiagnostics(t *testing.T) {
	callErr := &transport.CallError{
		Stage: transport.CallStageHTTP, HTTPStatus: 429, RPCCode: -32000,
		RetryAfter: "23", RequestID: "request-original", TraceID: "transport-trace",
		Cause: errors.New("upstream throttled the second page"),
	}
	cliErr := &helpers.CLIError{
		Code: helpers.CodeMCPServerError, Message: "message search failed",
		Suggestion: "transport hint", Operation: activeConversationsOperation,
		Cause: fmt.Errorf("transport wrapper: %w", callErr),
	}
	nextRetryAt := time.Date(2026, 9, 7, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	originalDetails := map[string]any{"upstreamContext": "keep-me", "failedPage": "upstream-value"}
	typed := apperrors.NewAPI("service rate limit",
		apperrors.WithCause(cliErr),
		apperrors.WithReason("rate_limited"),
		apperrors.WithHint("honor the server retry delay"),
		apperrors.WithRetryable(true),
		apperrors.WithRetryAfterSeconds(37),
		apperrors.WithNextRetryAt(nextRetryAt),
		apperrors.WithOperation(activeConversationsOperation),
		apperrors.WithServerKey("chat"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("response"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRPCCode(-32001),
		apperrors.WithRPCData(json.RawMessage(`{"resource":"chat-messages"}`)),
		apperrors.WithDetails(originalDetails),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			TraceID: "service-trace", ServerErrorCode: "SERVER_BUSY", TechnicalDetail: "message-search rate limit",
		}),
	).(*apperrors.Error)
	wrapped := fmt.Errorf("page read: %w", typed)
	info := activeConversationPageErrorInfo(wrapped, 2, "cursor-page-2")
	if err := info.Validate(); err != nil {
		t.Fatalf("invalid page error: %v", err)
	}
	if info.Type != "api" || info.Subtype != "rate_limited" || info.UpstreamCode != "SERVER_BUSY" ||
		info.HTTPStatus != 429 || info.RPCCode != -32001 || info.RequestID != "request-original" || info.TraceID != "service-trace" {
		t.Fatalf("lost or misclassified upstream diagnostics: %+v", info)
	}
	if info.Operation != activeConversationsOperation || info.ServerKey != "chat" || info.Origin != "mcp_gateway" ||
		info.Stage != "response" || info.TechnicalDetail != "message-search rate limit" ||
		info.ExecutionStarted == nil || !*info.ExecutionStarted {
		t.Fatalf("lost execution diagnostics: %+v", info)
	}
	if !info.Retryable || info.RetryAfterSeconds == nil || *info.RetryAfterSeconds != 37 || info.NextRetryAt != "2026-09-07T02:00:00Z" {
		t.Fatalf("lost reviewed retry policy: %+v", info)
	}
	if info.Message != wrapped.Error() || info.Cause != cliErr.Error() {
		t.Fatalf("lost original message/cause: %+v", info)
	}
	for _, fragment := range []string{"honor the server retry delay", "先处理", "profile", "--start", "--end", "--limit", "meta.pagination.next_token"} {
		if !strings.Contains(info.Hint, fragment) {
			t.Fatalf("hint %q lacks original/recovery guidance %q", info.Hint, fragment)
		}
	}
	wantDetails := map[string]any{
		"upstreamContext": "keep-me", "retryAfter": "23", "failedPage": 2, "failedCursor": "cursor-page-2",
	}
	if !reflect.DeepEqual(info.Details, wantDetails) || !reflect.DeepEqual(info.RPCData, map[string]any{"resource": "chat-messages"}) {
		t.Fatalf("lost structured context: details=%#v rpc=%#v", info.Details, info.RPCData)
	}
	if !reflect.DeepEqual(typed.Details, originalDetails) || typed.Hint != "honor the server retry delay" || *typed.RetryAfterSeconds != 37 {
		t.Fatalf("projection mutated source error: %+v", typed)
	}
	info.Details["upstreamContext"] = "changed-projection"
	*info.RetryAfterSeconds = 99
	*info.ExecutionStarted = false
	if typed.Details["upstreamContext"] != "keep-me" || *typed.RetryAfterSeconds != 37 || !*typed.ExecutionStarted {
		t.Fatalf("projection retained mutable source fields: %+v", typed)
	}
}

func TestCrossPlatformCoverageActiveConversationsPageErrorClassification(t *testing.T) {
	permissionPayload := `{"success":false,"code":"PAT_ORG_POLICY_DENIED","data":{"reason":"organization policy"}}`
	cases := []struct {
		name     string
		err      error
		wantType string
		wantCode any
		wantHint string
	}{
		{
			name: "validation", err: apperrors.NewValidation("invalid cursor", apperrors.WithRetryable(false)),
			wantType: "validation",
		},
		{
			name: "authentication", err: apperrors.NewAuth("token expired", apperrors.WithHint("authenticate first")),
			wantType: "auth", wantHint: "authenticate first",
		},
		{
			name: "PAT permission", err: fmt.Errorf("permission wrapper: %w", &apperrors.PATError{RawJSON: permissionPayload}),
			wantType: "permission", wantCode: "PAT_ORG_POLICY_DENIED",
		},
		{
			name: "CLI permission", err: &helpers.CLIError{Code: helpers.CodeAuthPermission, Message: "access denied", Suggestion: "request document permission"},
			wantType: "permission", wantCode: helpers.CodeAuthPermission, wantHint: "request document permission",
		},
		{
			name: "API not retryable", err: apperrors.NewAPI("business rule denied", apperrors.WithRetryable(false)),
			wantType: "api",
		},
		{
			name: "discovery", err: fmt.Errorf("resolve message search: %w", apperrors.NewDiscovery(
				"message search tool unavailable", apperrors.WithHint("refresh MCP discovery before resuming"),
			)),
			wantType: "discovery", wantHint: "refresh MCP discovery before resuming",
		},
		{
			name: "deadline", err: fmt.Errorf("read timeout: %w", context.DeadlineExceeded), wantType: "internal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := activeConversationPageErrorInfo(tc.err, 3, "cursor-3")
			if err := info.Validate(); err != nil {
				t.Fatal(err)
			}
			if info.Type != tc.wantType || info.UpstreamCode != tc.wantCode || info.Retryable {
				t.Fatalf("classification/retry policy changed: %+v", info)
			}
			if tc.wantHint != "" && !strings.Contains(info.Hint, tc.wantHint) {
				t.Fatalf("original recovery hint lost: %q", info.Hint)
			}
			if tc.name == "PAT permission" {
				var original map[string]any
				if err := json.Unmarshal([]byte(permissionPayload), &original); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(info.Details["permission"], original) {
					t.Fatalf("permission context lost: %#v", info.Details)
				}
			}
			if tc.name == "deadline" && info.Subtype != "deadline_exceeded" {
				t.Fatalf("deadline lost its stable subtype: %+v", info)
			}
		})
	}
	// A server's zero-second delay is meaningful, but it does not alone make
	// the enclosing API error retryable.
	info := activeConversationPageErrorInfo(apperrors.NewAPI("upstream failure", apperrors.WithCause(&transport.CallError{
		Stage: transport.CallStageHTTP, HTTPStatus: 503, RetryAfter: "0", TraceID: "trace-without-request",
	})), 2, "cursor-2")
	if info.RetryAfterSeconds == nil || *info.RetryAfterSeconds != 0 || info.Retryable || info.TraceID != "trace-without-request" || info.RequestID != "trace-without-request" {
		t.Fatalf("transport guidance/fallback lost or retryability invented: %+v", info)
	}
}

func TestCrossPlatformCoverageActiveConversationsResultSchemaBranches(t *testing.T) {
	normalized, err := contract.NormalizeResultSpec(ActiveConversations.Contract.Result, "chat.shortcut_active_conversations")
	if err != nil {
		t.Fatalf("result contract does not normalize: %v", err)
	}
	if !reflect.DeepEqual(normalized.Outcomes, []contract.ResultOutcome{
		contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure,
	}) {
		t.Fatalf("result outcomes do not expose partial failure: %v", normalized.Outcomes)
	}
	var schema map[string]any
	if err := json.Unmarshal(normalized.DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	var declaredSchema map[string]any
	if err := json.Unmarshal(activeConversationsResultSchema(), &declaredSchema); err != nil {
		t.Fatalf("static result declaration is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(schema, declaredSchema) {
		t.Fatal("result normalization changed or discarded part of the static schema")
	}
	branches, ok := schema["oneOf"].([]any)
	if schema["type"] != "object" || !ok || len(branches) != 2 {
		t.Fatalf("success/partial data must have distinct object branches: %#v", schema)
	}
	success := branches[0].(map[string]any)
	partial := branches[1].(map[string]any)
	aggregateFields := []string{"start", "end", "rangeSemantics", "count", "complete", "pagesFetched", "pageSize", "unknownTypeCount", "conversations"}
	assertActiveConversationSchemaRequiredForTest(t, success, aggregateFields...)
	assertActiveConversationSchemaRequiredForTest(t, partial, "total", "succeeded", "failed", "unknown")
	properties := partial["properties"].(map[string]any)
	if properties["total"].(map[string]any)["const"] != float64(2) || properties["unknown"].(map[string]any)["maxItems"] != float64(0) {
		t.Fatalf("partial ledger processing-unit count changed: %#v", properties)
	}
	succeeded := properties["succeeded"].(map[string]any)
	failed := properties["failed"].(map[string]any)
	for name, channel := range map[string]map[string]any{"succeeded": succeeded, "failed": failed} {
		if channel["minItems"] != float64(1) || channel["maxItems"] != float64(1) {
			t.Fatalf("%s must contain exactly one processing unit: %#v", name, channel)
		}
	}
	batch := succeeded["items"].(map[string]any)
	assertActiveConversationSchemaRequiredForTest(t, batch, append(aggregateFields, "id")...)
	batchProperties := batch["properties"].(map[string]any)
	if batchProperties["id"].(map[string]any)["const"] != "completed-pages" || batchProperties["complete"].(map[string]any)["const"] != false {
		t.Fatalf("partial batch must identify incomplete preserved pages: %#v", batchProperties)
	}
	for name, aggregate := range map[string]map[string]any{"success": success, "partial batch": batch} {
		aggregateProperties := aggregate["properties"].(map[string]any)
		for _, field := range []string{"start", "end"} {
			if !strings.Contains(aggregateProperties[field].(map[string]any)["description"].(string), "整秒") {
				t.Fatalf("%s %s must explain the query's whole-second precision", name, field)
			}
		}
		if !strings.Contains(aggregateProperties["end"].(map[string]any)["description"].(string), "向下取整") {
			t.Fatalf("%s end must explain how the default upper bound is fixed", name)
		}
		row := aggregateProperties["conversations"].(map[string]any)["items"].(map[string]any)
		assertActiveConversationSchemaRequiredForTest(t, row, "conversationId", "name", "nameKnown", "type", "latestMessageTime")
		if row["additionalProperties"] != false {
			t.Fatalf("%s row lacks strict return shape: %#v", name, row)
		}
		latest := row["properties"].(map[string]any)["latestMessageTime"].(map[string]any)
		if !strings.Contains(latest["description"].(string), "原始精度") {
			t.Fatalf("%s latest message time must retain message precision", name)
		}
	}
	failedItem := failed["items"].(map[string]any)
	assertActiveConversationSchemaRequiredForTest(t, failedItem, "id", "error")
	errorSchema := failedItem["properties"].(map[string]any)["error"].(map[string]any)
	details := errorSchema["properties"].(map[string]any)["details"].(map[string]any)
	assertActiveConversationSchemaRequiredForTest(t, details, "failedPage", "failedCursor")
}

func assertActiveConversationSchemaRequiredForTest(t *testing.T, schema map[string]any, fields ...string) {
	t.Helper()
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema lacks required fields: %#v", schema)
	}
	declared := make(map[string]bool, len(required))
	for _, field := range required {
		declared[field.(string)] = true
	}
	for _, field := range fields {
		if !declared[field] {
			t.Errorf("required return field %q missing: %#v", field, schema)
		}
	}
}
