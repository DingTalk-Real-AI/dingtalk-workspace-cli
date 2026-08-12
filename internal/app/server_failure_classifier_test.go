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
}

func TestCrossPlatformCoverageServerFailureClassifierRequiredConversationID(t *testing.T) {
	err := newServerFailureAPIError(
		"openCid or cid is required",
		"business_error",
		"check parameters",
		"chat",
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

func TestCrossPlatformCoverageServerFailureClassifierUnknownFallsBack(t *testing.T) {
	err := newServerFailureAPIError(
		"business error: success=false",
		"business_error",
		"check parameters",
		"im",
		apperrors.ServerDiagnostics{},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "business_error" || typed.Origin != "" || typed.FailureStage != "" || typed.ExecutionStarted != nil {
		t.Fatalf("unexpected fallback classification: %#v", typed)
	}
	if len(typed.Actions) == 0 || !strings.Contains(typed.Actions[0], "dws doctor") {
		t.Fatalf("fallback error has no stable troubleshooting entry: %#v", typed.Actions)
	}
}

func TestCrossPlatformCoverageServerFailureReasonUsesTypedClassification(t *testing.T) {
	err := newServerFailureAPIError(
		"business error: success=false",
		"business_error",
		"check parameters",
		"im",
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
