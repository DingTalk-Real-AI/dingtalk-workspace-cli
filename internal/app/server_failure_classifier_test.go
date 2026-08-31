// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageServerFailureClassifierBackendMetadataUnavailable(t *testing.T) {
	retryable := true
	err := newServerFailureAPIError(
		"business error: success=false",
		"business_error",
		"check parameters",
		"im",
		"list_conversations",
		apperrors.ServerDiagnostics{
			TraceID:         "trace-local",
			ServerErrorCode: "NETWORK_ERROR",
			TechnicalDetail: "调用 McpService.queryToolMeta 失败: status = StatusCode.UNAVAILABLE; connect: Connection refused (111)",
			ServerRetryable: &retryable,
		},
	)

	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "backend_dependency_unavailable" || typed.Origin != "mcp_gateway" || typed.FailureStage != "tool_metadata_lookup" {
		t.Fatalf("classification = reason %q origin %q stage %q", typed.Reason, typed.Origin, typed.FailureStage)
	}
	if typed.ExecutionStarted != nil {
		t.Fatalf("execution_started = %v, want unknown until the backend publishes it", typed.ExecutionStarted)
	}
	if !typed.RetryableSet || !typed.Retryable {
		t.Fatalf("retryability = (%v, %v), want explicit true", typed.RetryableSet, typed.Retryable)
	}
	if strings.Contains(strings.ToLower(typed.Hint), "parameter") || strings.Contains(typed.Hint, "认证") {
		t.Fatalf("misleading hint = %q", typed.Hint)
	}
	if actions := apperrors.RecoveryActions(err); !strings.Contains(strings.Join(actions, "\n"), apperrors.DoctorCommand) {
		t.Fatalf("backend dependency error has no doctor entry: %#v", actions)
	}
}

func TestCrossPlatformCoverageServerFailureClassifierRequiredConversationID(t *testing.T) {
	err := newServerFailureAPIError(
		"openCid or cid is required",
		"business_error",
		"check parameters",
		"chat",
		"send_message",
		apperrors.ServerDiagnostics{ServerErrorCode: "1001"},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "invalid_request" || typed.FailureStage != "tool_validation" {
		t.Fatalf("classification = reason %q stage %q", typed.Reason, typed.FailureStage)
	}
	if typed.ExecutionStarted != nil {
		t.Fatalf("execution_started = %v, want unknown until the backend publishes it", typed.ExecutionStarted)
	}
}

func TestCrossPlatformCoverageServerFailureClassifierTodoCreateUpstreamInternalError(t *testing.T) {
	serverSaysRetryable := true
	err := newServerFailureAPIError(
		"[UNCLASSIFIED] system error: java.lang.NullPointerException (operation: todo/create_personal_todo)",
		"business_error",
		"The API returned a business-level error. Check required parameters and values.",
		"todo",
		"create_personal_todo",
		apperrors.ServerDiagnostics{
			TraceID:         "trace-todo-create",
			ServerErrorCode: "999",
			ServerRetryable: &serverSaysRetryable,
		},
	)

	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "upstream_internal_error" || typed.Origin != "dingtalk_api" || typed.FailureStage != "upstream_execution" {
		t.Fatalf("classification = reason %q origin %q stage %q", typed.Reason, typed.Origin, typed.FailureStage)
	}
	if typed.Operation != "todo/create_personal_todo" {
		t.Fatalf("operation = %q, want todo/create_personal_todo", typed.Operation)
	}
	if typed.ExecutionStarted != nil {
		t.Fatalf("execution_started = %v, want unknown", typed.ExecutionStarted)
	}
	if !typed.RetryableSet || typed.Retryable {
		t.Fatalf("retryability = (%v, %v), want explicit false", typed.RetryableSet, typed.Retryable)
	}
	if typed.ServerDiag.TraceID != "trace-todo-create" || typed.ServerDiag.ServerErrorCode != "999" {
		t.Fatalf("diagnostics = %#v", typed.ServerDiag)
	}
	if strings.Contains(strings.ToLower(typed.Hint), "parameter") || !strings.Contains(typed.Hint, "创建结果未知") {
		t.Fatalf("hint = %q", typed.Hint)
	}
	for _, action := range typed.Actions {
		if strings.Contains(action, "dws doctor") || strings.Contains(action, "登录") || strings.Contains(action, "网络") {
			t.Fatalf("misleading action = %q", action)
		}
	}

	payload := multiProfileErrorPayload(err)
	for key, want := range map[string]any{
		"reason":            "upstream_internal_error",
		"origin":            "dingtalk_api",
		"stage":             "upstream_execution",
		"retryable":         false,
		"trace_id":          "trace-todo-create",
		"server_error_code": "999",
	} {
		if got := payload[key]; got != want {
			t.Errorf("payload[%q] = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := payload["execution_started"]; ok {
		t.Fatalf("payload must keep execution_started unknown: %#v", payload)
	}
}

func TestCrossPlatformCoverageServerFailureClassifierUnknownFallsBack(t *testing.T) {
	err := newServerFailureAPIError(
		"business error: success=false",
		"business_error",
		"check parameters",
		"im",
		"list_conversations",
		apperrors.ServerDiagnostics{},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "business_error" || typed.Origin != "" || typed.FailureStage != "" || typed.ExecutionStarted != nil {
		t.Fatalf("unexpected fallback classification: %#v", typed)
	}
	if len(typed.Actions) == 0 || strings.Contains(strings.Join(apperrors.RecoveryActions(err), "\n"), "dws doctor") {
		t.Fatalf("fallback business error must keep service recovery without doctor: %#v", typed.Actions)
	}
}

func TestCrossPlatformCoverageServerFailureReasonUsesTypedClassification(t *testing.T) {
	err := newServerFailureAPIError(
		"business error: success=false",
		"business_error",
		"check parameters",
		"im",
		"list_conversations",
		apperrors.ServerDiagnostics{ServerErrorCode: "NETWORK_ERROR"},
	)
	if got := serverFailureReason(err, "business_error"); got != "backend_dependency_unavailable" {
		t.Fatalf("reason = %q", got)
	}
	if got := serverFailureReason(errors.New("plain"), "fallback"); got != "fallback" {
		t.Fatalf("fallback reason = %q", got)
	}
}

func TestCrossPlatformCoverageMultiProfileErrorPayloadPreservesFailureSemantics(t *testing.T) {
	retryable := true
	err := newServerFailureAPIError(
		"business error: success=false",
		"business_error",
		"check parameters",
		"im",
		"list_conversations",
		apperrors.ServerDiagnostics{
			TraceID:         "trace-multi",
			ServerErrorCode: "NETWORK_ERROR",
			TechnicalDetail: "McpService.queryToolMeta: StatusCode.UNAVAILABLE",
			ServerRetryable: &retryable,
		},
	)
	payload := multiProfileErrorPayload(err)
	for key, want := range map[string]any{
		"reason":            "backend_dependency_unavailable",
		"origin":            "mcp_gateway",
		"stage":             "tool_metadata_lookup",
		"retryable":         true,
		"trace_id":          "trace-multi",
		"server_error_code": "NETWORK_ERROR",
	} {
		if got := payload[key]; got != want {
			t.Errorf("payload[%q] = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := payload["execution_started"]; ok {
		t.Fatalf("payload must not invent execution_started: %#v", payload)
	}
	actions, _ := payload["actions"].([]string)
	if !strings.Contains(strings.Join(actions, "\n"), apperrors.DoctorCommand) {
		t.Fatalf("multi-profile backend failure has no doctor entry: %#v", payload["actions"])
	}
}

func TestCrossPlatformCoverageMultiProfileErrorPayloadPreservesResolutionDetails(t *testing.T) {
	err := apperrors.NewValidation(
		"群目标不唯一",
		apperrors.WithReason("resolution_ambiguous"),
		apperrors.WithOrigin("client"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithHint("请选择候选"),
		apperrors.WithActions("使用稳定 ID 重试"),
		apperrors.WithDetails(map[string]any{
			"type":       "resolution",
			"candidates": []string{"cid-1", "cid-2"},
		}),
	)
	payload := multiProfileErrorPayload(err)
	details, ok := payload["details"].(map[string]any)
	if !ok || details["type"] != "resolution" {
		t.Fatalf("details = %#v", payload["details"])
	}
	if payload["execution_started"] != false || payload["origin"] != "client" || payload["stage"] != "target_resolution" {
		t.Fatalf("payload = %#v", payload)
	}
	if actions, ok := payload["actions"].([]string); !ok || len(actions) != 1 {
		t.Fatalf("actions = %#v", payload["actions"])
	}
}

func TestCrossPlatformCoverageExecuteInvocationClassifiesObservedMCPMetadataFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"structuredContent": map[string]any{
					"success":          false,
					"code":             "NETWORK_ERROR",
					"trace_id":         "trace-replay",
					"technical_detail": "调用 McpService.queryToolMeta 失败: status = StatusCode.UNAVAILABLE; connect: Connection refused (111)",
					"retryable":        true,
				},
			},
		})
	}))
	defer server.Close()

	client := transport.NewClient(server.Client())
	client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
	runner := &runtimeRunner{
		transport:   client,
		globalFlags: &GlobalFlags{Token: "local-test-token"},
	}
	_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
		CanonicalProduct: "im",
		Tool:             "list_conversations",
		Params:           map[string]any{"pageSize": 100},
	})
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("executeInvocation() error = %T %v, want typed API error", err, err)
	}
	if typed.Reason != "backend_dependency_unavailable" || typed.Origin != "mcp_gateway" || typed.FailureStage != "tool_metadata_lookup" {
		t.Fatalf("classification = reason %q origin %q stage %q", typed.Reason, typed.Origin, typed.FailureStage)
	}
	if typed.ServerDiag.TraceID != "trace-replay" || !typed.RetryableSet || !typed.Retryable {
		t.Fatalf("diagnostics = %#v retryable=(%v,%v)", typed.ServerDiag, typed.RetryableSet, typed.Retryable)
	}
	if typed.ExecutionStarted != nil {
		t.Fatalf("execution_started must remain unknown: %v", typed.ExecutionStarted)
	}
}

func TestCrossPlatformCoverageExecuteInvocationClassifiesTodoCreateUpstreamInternalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"structuredContent": map[string]any{
					"success":  false,
					"code":     "999",
					"trace_id": "trace-todo-replay",
					"errorMsg": "[UNCLASSIFIED] system error: java.lang.NullPointerException (operation: todo/create_personal_todo)",
				},
			},
		})
	}))
	defer server.Close()

	client := transport.NewClient(server.Client())
	client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
	runner := &runtimeRunner{
		transport:   client,
		globalFlags: &GlobalFlags{Token: "local-test-token"},
	}
	_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
		CanonicalProduct: "todo",
		Tool:             "create_personal_todo",
		CanonicalPath:    "todo.create_personal_todo",
		Params: map[string]any{
			"PersonalTodoCreateVO": map[string]any{
				"subject":     "fixture",
				"executorIds": []string{"user-1"},
			},
		},
	})
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("executeInvocation() error = %T %v, want typed API error", err, err)
	}
	if typed.Reason != "upstream_internal_error" || typed.Operation != "todo/create_personal_todo" {
		t.Fatalf("classification = reason %q operation %q", typed.Reason, typed.Operation)
	}
	if !typed.RetryableSet || typed.Retryable || typed.ExecutionStarted != nil {
		t.Fatalf("failure semantics = retryable(%v,%v) execution_started=%v", typed.RetryableSet, typed.Retryable, typed.ExecutionStarted)
	}
	if typed.ServerDiag.TraceID != "trace-todo-replay" || typed.ServerDiag.ServerErrorCode != "999" {
		t.Fatalf("diagnostics = %#v", typed.ServerDiag)
	}
}
