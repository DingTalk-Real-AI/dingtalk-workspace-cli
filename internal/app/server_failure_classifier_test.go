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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageServerFailureClassifierFillsServerMetadata(t *testing.T) {
	serverRetryable := true
	serverStarted := false
	unknown := true
	err := newServerFailureAPIError(
		"upstream failure",
		"business_error",
		"fallback hint",
		"chat",
		"send_message",
		apperrors.ServerDiagnostics{
			Reason:           "server_reason",
			Operation:        "server/operation",
			Origin:           "server_origin",
			FailureStage:     "server_stage",
			FriendlyHint:     "server hint",
			Actions:          []string{"check status", "check status"},
			NextCommand:      "dws check status",
			ServerRetryable:  &serverRetryable,
			ExecutionStarted: &serverStarted,
			Outcome:          "unknown",
			ContractVersion:  "v1",
			Unknown:          &unknown,
			Details:          map[string]any{"checkpoint": "v"},
		},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "server_reason" || typed.Operation != "server/operation" || typed.Origin != "server_origin" || typed.FailureStage != "server_stage" || typed.Hint != "server hint" {
		t.Fatalf("server metadata = %#v", typed)
	}
	if !typed.RetryableSet || !typed.Retryable || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
		t.Fatalf("server execution metadata = %#v", typed)
	}
	if !strings.Contains(strings.Join(typed.Actions, "\n"), "dws check status") {
		t.Fatalf("actions = %#v", typed.Actions)
	}
	if typed.Details["outcome"] != "unknown" || typed.Details["contractVersion"] != "v1" || typed.Details["unknown"] != true || typed.Details["checkpoint"] != "v" {
		t.Fatalf("details = %#v", typed.Details)
	}
}

func TestCrossPlatformCoverageServerFailureClassifierRateLimitGuidance(t *testing.T) {
	for _, tc := range []struct {
		name      string
		code      string
		message   string
		detail    string
		retryable *bool
		wantMatch bool
	}{
		{
			name:      "exact code",
			code:      "RATE_LIMIT_ERROR",
			message:   "business error: success=false",
			wantMatch: true,
		},
		{
			name:      "normalized code takes precedence over incidental text",
			code:      " rate_limit_error ",
			message:   "openConversationId is required",
			detail:    "connection refused",
			retryable: boolPointer(false),
			wantMatch: true,
		},
		{
			name:      "similar code does not match",
			code:      "RATE_LIMITED",
			message:   "当前请求频率过高，本次请求被限流",
			wantMatch: false,
		},
		{
			name:      "message alone does not match",
			message:   "RATE_LIMIT_ERROR: 当前请求频率过高",
			wantMatch: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := newServerFailureAPIError(
				tc.message,
				"business_error",
				"check parameters",
				"im",
				"send_personal_message",
				apperrors.ServerDiagnostics{
					ServerErrorCode: tc.code,
					TechnicalDetail: tc.detail,
					ServerRetryable: tc.retryable,
				},
			)
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T %v, want structured error", err, err)
			}
			if got := typed.Reason == "rate_limited"; got != tc.wantMatch {
				t.Fatalf("reason = %q, rate-limit match = %v", typed.Reason, got)
			}
			if !tc.wantMatch {
				if typed.Message != tc.message {
					t.Fatalf("non-matching message = %q, want original %q", typed.Message, tc.message)
				}
				return
			}
			if typed.Message != "由于当前请求频率过高，本次请求被限流，请等待 10 秒后重试。" {
				t.Fatalf("message = %q", typed.Message)
			}
			if !strings.Contains(typed.Hint, "降低请求频率") || !strings.Contains(typed.Hint, "等待 10 秒后重试") {
				t.Fatalf("hint = %q", typed.Hint)
			}
			if typed.Origin != "" || typed.FailureStage != "" || typed.ExecutionStarted != nil {
				t.Fatalf("invented failure semantics: %#v", typed)
			}
			if tc.retryable == nil {
				if typed.RetryableSet {
					t.Fatalf("client invented retryability: %#v", typed)
				}
			} else if !typed.RetryableSet || typed.Retryable != *tc.retryable {
				t.Fatalf("retryability = (%v, %v), want %v", typed.RetryableSet, typed.Retryable, *tc.retryable)
			}
			if typed.RetryAfterSeconds != nil {
				t.Fatalf("client invented retry_after_seconds: %v", typed.RetryAfterSeconds)
			}
		})
	}
}

func TestCrossPlatformCoverageExecuteInvocationRateLimitGuidance(t *testing.T) {
	for _, tc := range []struct {
		name      string
		isError   bool
		code      string
		retryable bool
		detail    string
	}{
		{name: "business failure", code: "RATE_LIMIT_ERROR", retryable: false},
		{name: "tool failure", isError: true, code: " rate_limit_error ", retryable: true, detail: "connection refused; openConversationId is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct {
					ID int `json:"id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"result": map[string]any{
						"isError": tc.isError,
						"structuredContent": map[string]any{
							"success":          false,
							"code":             tc.code,
							"message":          "请求被限流。当前请求过于频繁，请稍后重试",
							"trace_id":         "trace-rate-limit",
							"retryable":        tc.retryable,
							"technical_detail": tc.detail,
						},
					},
				})
			}))
			defer server.Close()

			client := transport.NewClient(server.Client())
			client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
			runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "local-test-token"}}
			_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
				CanonicalProduct: "chat",
				Tool:             "send_personal_message",
				Params:           map[string]any{"content": "fixture"},
			})
			if got := calls.Load(); got != 1 {
				t.Fatalf("business requests = %d, want 1 (no automatic replay)", got)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T %v, want structured error", err, err)
			}
			if typed.Retryable != tc.retryable || !typed.RetryableSet || typed.ExecutionStarted != nil || typed.RetryAfterSeconds != nil {
				t.Fatalf("retry semantics = retryable(%v,%v) execution_started=%v retry_after=%v", typed.RetryableSet, typed.Retryable, typed.ExecutionStarted, typed.RetryAfterSeconds)
			}

			var legacy bytes.Buffer
			if printErr := apperrors.PrintJSON(&legacy, err); printErr != nil {
				t.Fatal(printErr)
			}
			var legacyWire struct {
				Error map[string]any `json:"error"`
			}
			if decodeErr := json.Unmarshal(legacy.Bytes(), &legacyWire); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			code, unifiedWire := emitChatResultWireForTest(t, output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err)))
			unifiedError, ok := unifiedWire["error"].(map[string]any)
			if !ok {
				t.Fatalf("unified error = %#v", unifiedWire["error"])
			}
			if code != apperrors.ExitCodeAPI || unifiedWire["ok"] != false || unifiedWire["outcome"] != "failure" {
				t.Fatalf("exit = %d, unified = %#v", code, unifiedWire)
			}
			for _, wire := range []map[string]any{legacyWire.Error, unifiedError} {
				if wire["message"] != "由于当前请求频率过高，本次请求被限流，请等待 10 秒后重试。" || wire["trace_id"] != "trace-rate-limit" {
					t.Errorf("guidance wire = %#v", wire)
				}
				if _, exists := wire["retry_after_seconds"]; exists {
					t.Errorf("client invented retry delay: %#v", wire)
				}
				if _, exists := wire["execution_started"]; exists {
					t.Errorf("client invented execution state: %#v", wire)
				}
			}
			if legacyWire.Error["retryable"] != tc.retryable {
				t.Errorf("legacy retryable = %#v, want %v", legacyWire.Error["retryable"], tc.retryable)
			}
			if tc.retryable {
				if unifiedError["retryable"] != true {
					t.Errorf("unified retryable = %#v, want true", unifiedError["retryable"])
				}
			} else if _, exists := unifiedError["retryable"]; exists {
				t.Errorf("unified retryable=false should follow the existing omitempty contract: %#v", unifiedError)
			}
			if legacyWire.Error["reason"] != "rate_limited" || legacyWire.Error["server_error_code"] != tc.code || unifiedError["subtype"] != "rate_limited" || unifiedError["upstream_code"] != tc.code {
				t.Fatalf("classification/diagnostics: legacy=%#v unified=%#v", legacyWire.Error, unifiedError)
			}
		})
	}
}

func boolPointer(value bool) *bool {
	return &value
}

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

func TestCrossPlatformCoverageServerFailureClassifierDingRobotNotInOrg(t *testing.T) {
	for _, tool := range []string{"send_ding_message", "recall_ding_message"} {
		t.Run(tool, func(t *testing.T) {
			err := newServerFailureAPIError(
				"[UNCLASSIFIED] robotCode is in not valid or not in the org; hint: Use --verbose",
				"business_error",
				"check parameters",
				"ding",
				tool,
				apperrors.ServerDiagnostics{ServerErrorCode: "400"},
			)

			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T, want *errors.Error", err)
			}
			if typed.Reason != "robot_not_in_org" || typed.Origin != "dingtalk_api" || typed.FailureStage != "precondition_rejected" {
				t.Fatalf("classification = reason %q origin %q stage %q", typed.Reason, typed.Origin, typed.FailureStage)
			}
			if typed.Operation != "ding/"+tool {
				t.Fatalf("operation = %q", typed.Operation)
			}
			if !typed.RetryableSet || typed.Retryable {
				t.Fatalf("retryability = (%v, %v), want explicit false", typed.RetryableSet, typed.Retryable)
			}
			if typed.ExecutionStarted == nil || *typed.ExecutionStarted {
				t.Fatalf("execution_started = %v, want false", typed.ExecutionStarted)
			}
			if strings.Contains(strings.ToLower(typed.Message), "verbose") || strings.Contains(strings.ToLower(typed.Hint), "verbose") {
				t.Fatalf("verbose retry guidance leaked: message=%q hint=%q", typed.Message, typed.Hint)
			}
			for _, action := range typed.Actions {
				if strings.Contains(action, "doctor") {
					t.Fatalf("generic diagnosis action leaked: %q", action)
				}
			}
			actions := strings.Join(typed.Actions, " ")
			if !strings.Contains(actions, "禁止尝试或替换为其他机器人") || !strings.Contains(actions, "不要搜索 dev/devapp") {
				t.Fatalf("terminal robot isolation actions = %#v", typed.Actions)
			}
		})
	}
}

func TestCrossPlatformCoverageServerFailureClassifierPreservesLocalGuidance(t *testing.T) {
	serverRetryable := true
	serverStarted := true
	err := newServerFailureAPIError(
		"robotCode is invalid",
		"business_error",
		"generic fallback hint",
		"ding",
		"send_ding_message",
		apperrors.ServerDiagnostics{
			Reason:           "server_reason",
			Operation:        "server/operation",
			Origin:           "server_origin",
			FailureStage:     "server_stage",
			FriendlyHint:     "server retry hint",
			Actions:          []string{"retry with another robot"},
			ServerRetryable:  &serverRetryable,
			ExecutionStarted: &serverStarted,
		},
	)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T, want *errors.Error", err)
	}
	if typed.Reason != "robot_not_in_org" || typed.Operation != "ding/send_ding_message" ||
		typed.Origin != "dingtalk_api" || typed.FailureStage != "precondition_rejected" ||
		typed.Hint != "请提供当前组织内有效且已发布的 robot-code 后重新发起请求。" {
		t.Fatalf("local classification overwritten: %#v", typed)
	}
	if !typed.RetryableSet || typed.Retryable || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
		t.Fatalf("local safety state overwritten: retryable=(%v,%v) execution_started=%v", typed.RetryableSet, typed.Retryable, typed.ExecutionStarted)
	}
	actions := strings.Join(typed.Actions, " ")
	if !strings.Contains(actions, "禁止尝试或替换为其他机器人") || strings.Contains(actions, "retry with another robot") {
		t.Fatalf("local safety actions were replaced or conflicted: %#v", typed.Actions)
	}
}

func TestCrossPlatformCoverageServerFailureClassifierDingRobotParameterBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, message, detail string
		robotFailure          bool
	}{
		{"direct invalid", "robotCode is invalid", "", true},
		{"prefix invalid", "Invalid robotCode", "", true},
		{"colon invalid", "robotCode: invalid", "", true},
		{"organization", "robotCode is not in the organization", "", true},
		{"technical rejection", "business error", "robotCode is not valid", true},
		{"invalid recipient", "invalid userId; robotCode=robot-1", "", false},
		{"valid robot invalid receiver", "robotCode is valid; receiver is invalid", "", false},
		{"recipient outside org", "robotCode=robot-1; userId not in the org", "", false},
		{"unrelated technical failure", "robotCode=robot-1", "content is not valid", false},
		{"split phrase", "robotCode", "is invalid", false},
		{"negative predicate", "robotCode is not invalid; userId is invalid", "", false},
		{"different field", "robotCodeStatus is invalid", "", false},
		{"different word", "robotCode is invalidated after send; receiver invalid", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retryable := true
			diag := apperrors.ServerDiagnostics{
				ServerErrorCode: "PARAM_ERROR", TechnicalDetail: tc.detail, ServerRetryable: &retryable,
			}
			err := newServerFailureAPIError(tc.message, "business_error", "check parameters", "ding", "send_ding_message", diag)
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("expected typed error: %v", err)
			}
			if got := typed.Reason == "robot_not_in_org"; got != tc.robotFailure {
				t.Fatalf("reason=%q, robot failure=%v", typed.Reason, tc.robotFailure)
			}
			if tc.robotFailure {
				if !typed.RetryableSet || typed.Retryable || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
					t.Fatalf("robot rejection must be terminal before execution: %#v", typed)
				}
			} else if typed.Reason != "invalid_request" || !typed.RetryableSet || !typed.Retryable || typed.ExecutionStarted != nil {
				t.Fatalf("unrelated parameter failure lost upstream retry/execution semantics: %#v", typed)
			}
			if typed.ServerDiag.TechnicalDetail != tc.detail {
				t.Fatal("classification discarded original diagnostics")
			}
		})
	}
	for _, target := range [][2]string{{"chat", "send_ding_message"}, {"ding", "send_personal_ding"}} {
		got, classified := classifyServerFailure("robotCode is invalid", target[0], target[1], apperrors.ServerDiagnostics{})
		if classified || got.reason == "robot_not_in_org" {
			t.Errorf("robot rejection leaked to unrelated operation %v: %+v", target, got)
		}
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

func TestCrossPlatformCoverageExecuteInvocationPreservesExplicitErrorCategory(t *testing.T) {
	for _, tc := range []struct {
		category apperrors.Category
		wire     string
		exitCode int
	}{
		{category: apperrors.CategoryAuth, wire: "auth", exitCode: apperrors.ExitCodeAuth},
		{category: apperrors.CategoryAuth, wire: "permission", exitCode: apperrors.ExitCodeAuth},
		{category: apperrors.CategoryAuth, wire: "forbidden", exitCode: apperrors.ExitCodeAuth},
		{category: apperrors.CategoryValidation, wire: "validation", exitCode: apperrors.ExitCodeValidation},
		{category: apperrors.CategoryDiscovery, wire: "discovery", exitCode: apperrors.ExitCodeDiscovery},
		{category: apperrors.CategoryInternal, wire: "internal", exitCode: apperrors.ExitCodeInternal},
	} {
		t.Run(tc.wire, func(t *testing.T) {
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
							"success": false,
							"error": map[string]any{
								"errorDetail": map[string]any{
									"errorType": tc.wire,
									"message":   "access denied",
									"reason":    "permission_denied",
								},
							},
							"data": map[string]any{
								"reason": "confirmation_required",
							},
						},
					},
				})
			}))
			defer server.Close()

			client := transport.NewClient(server.Client())
			client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
			runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "local-test-token"}}
			_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
				CanonicalProduct: "other",
				Tool:             "test_tool",
			})
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("executeInvocation() error = %T %v, want typed error", err, err)
			}
			if typed.Category != tc.category || typed.ExitCode() != tc.exitCode {
				t.Fatalf("category/exit = %q/%d, want %q/%d", typed.Category, typed.ExitCode(), tc.category, tc.exitCode)
			}
			if tc.wire == "auth" && typed.Reason != "permission_denied" {
				t.Fatalf("reason = %q, want explicit errorDetail reason", typed.Reason)
			}
		})
	}
}

func TestCrossPlatformCoverageExecuteInvocationIgnoresBusinessCategory(t *testing.T) {
	t.Parallel()
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
					"success": false,
					"errorDetail": map[string]any{
						"category": "application_scope",
						"message":  "business failure",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := transport.NewClient(server.Client())
	client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
	runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "local-test-token"}}
	_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
		CanonicalProduct: "other",
		Tool:             "test_tool",
	})
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("executeInvocation() error = %T %v, want typed error", err, err)
	}
	if typed.Category != apperrors.CategoryAPI || typed.ExitCode() != apperrors.ExitCodeAPI {
		t.Fatalf("business category = %q/%d, want api/%d", typed.Category, typed.ExitCode(), apperrors.ExitCodeAPI)
	}
}

func TestCrossPlatformCoverageExecuteInvocationBusinessScopeHintSkipsPatRetry(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"isError": true,
				"structuredContent": map[string]any{
					"success": false,
					"message": "business denied",
					"error": map[string]any{
						"errorDetail": map[string]any{
							"errorType":     "permission",
							"message":       "business denied",
							"friendly_hint": "application scope approval required",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := transport.NewClient(server.Client())
	client.TrustedDomains = []string{strings.TrimPrefix(server.URL, "http://")}
	runner := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "local-test-token"}}
	_, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
		CanonicalProduct: "devapp",
		Tool:             "publish_dev_app_version",
	})
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("executeInvocation() error = %T %v, want typed error", err, err)
	}
	if typed.Category != apperrors.CategoryAuth || typed.ExitCode() != apperrors.ExitCodeAuth {
		t.Fatalf("business authorization = %q/%d, want auth/%d", typed.Category, typed.ExitCode(), apperrors.ExitCodeAuth)
	}
	if isPatScopeError(err) {
		t.Fatal("business scope hint must not be classified as a PAT scope error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("business requests = %d, want 1 (PAT retry must not replay the call)", got)
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
