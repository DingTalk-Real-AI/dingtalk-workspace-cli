// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// TestCrossPlatformCoverageDevEventRunnerErrorPassthrough 离线验证透传契约：
// nil、纯文本、取消/截止、已分类但无信封事实、message 已自述 errorCode 的
// 已分类错误，都必须原样返回同一对象，不得重建或改写类别。
func TestCrossPlatformCoverageDevEventRunnerErrorPassthrough(t *testing.T) {
	plain := stderrors.New("business error: success=false")
	deadline := fmt.Errorf("call dev event tool: %w", context.DeadlineExceeded)
	canceled := fmt.Errorf("call dev event tool: %w", context.Canceled)
	validation := apperrors.NewValidation("--file 为必填")
	selfDescribing := apperrors.NewAPI("business error: code CONFIG_REVISION_CONFLICT",
		apperrors.WithReason("business_error"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "CONFIG_REVISION_CONFLICT"}))
	cancelledCall := &transport.CallError{
		Stage: transport.CallStageHTTP,
		Cause: context.DeadlineExceeded,
	}

	for name, tc := range map[string]error{
		"nil":                   nil,
		"纯文本":                   plain,
		"截止错误":                  deadline,
		"取消错误":                  canceled,
		"无信封事实的已分类错误":           validation,
		"message 已自述 errorCode": selfDescribing,
		"CallError 包装截止错误":      cancelledCall,
	} {
		if got := devEventRunnerError(tc); got != tc {
			t.Fatalf("%s: devEventRunnerError 重建了错误 %v，want 原样透传 %#v", name, got, tc)
		}
	}
}

// TestCrossPlatformCoverageDevEventRunnerErrorSurfacesEnvelope 离线验证提取契约：
// 已分类错误携带 message 未自述的 ServerDiag 事实时，重建的错误必须把 errorCode
// 补进 message 与 Details，类别/reason/hint/retryable/退出码语义原样保留，
// ServerDiag 不丢，且 errors.Is/As/Unwrap 链仍然可达原错误。
func TestCrossPlatformCoverageDevEventRunnerErrorSurfacesEnvelope(t *testing.T) {
	original := apperrors.NewAPI("business error: success=false",
		apperrors.WithReason("business_error"),
		apperrors.WithHint("The API returned a business-level error. Check required parameters and values."),
		apperrors.WithOperation("tools/call"),
		apperrors.WithServerKey("devapp"),
		apperrors.WithRetryable(false),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			TraceID:         "trace-1",
			ServerErrorCode: "INVALID_EVENT_KEY",
			TechnicalDetail: "eventKey must match ^app_user_",
		}),
	)
	rebuilt := devEventRunnerError(original)
	if rebuilt == original {
		t.Fatal("devEventRunnerError 返回了原错误，want 重建以透出 errorCode")
	}
	message := rebuilt.Error()
	if !strings.Contains(message, "business error: success=false") {
		t.Fatalf("message = %q, want 保留原始 message", message)
	}
	if !strings.Contains(message, "errorCode=INVALID_EVENT_KEY") {
		t.Fatalf("message = %q, want errorCode 进 message", message)
	}

	var typed *apperrors.Error
	if !stderrors.As(rebuilt, &typed) || typed == nil {
		t.Fatalf("errors.As(*apperrors.Error) 失败：rebuilt = %#v", rebuilt)
	}
	if typed.Category != apperrors.CategoryAPI {
		t.Fatalf("category = %q, want api（类别原样保留，不猜测）", typed.Category)
	}
	if code := apperrors.ExitCode(rebuilt); code != apperrors.ExitCodeAPI {
		t.Fatalf("exit code = %d, want %d", code, apperrors.ExitCodeAPI)
	}
	if typed.Reason != "business_error" {
		t.Fatalf("reason = %q, want business_error", typed.Reason)
	}
	if typed.Hint != "The API returned a business-level error. Check required parameters and values." {
		t.Fatalf("hint = %q, want 原 hint 保留", typed.Hint)
	}
	if typed.Operation != "tools/call" || typed.ServerKey != "devapp" {
		t.Fatalf("operation/server_key = %q/%q, want 原值保留", typed.Operation, typed.ServerKey)
	}
	if !typed.RetryableSet || typed.Retryable {
		t.Fatalf("retryable = %v(set=%v), want 显式 false", typed.Retryable, typed.RetryableSet)
	}
	if typed.ServerDiag.ServerErrorCode != "INVALID_EVENT_KEY" ||
		typed.ServerDiag.TechnicalDetail != "eventKey must match ^app_user_" ||
		typed.ServerDiag.TraceID != "trace-1" {
		t.Fatalf("server_diag = %+v, want 原样保留", typed.ServerDiag)
	}
	for key, want := range map[string]any{
		"errorCode":       "INVALID_EVENT_KEY",
		"technicalDetail": "eventKey must match ^app_user_",
		"traceId":         "trace-1",
	} {
		if got := typed.Details[key]; got != want {
			t.Fatalf("details[%q] = %v, want %v", key, got, want)
		}
	}
	if !stderrors.Is(rebuilt, original) {
		t.Fatal("errors.Is(rebuilt, original) = false, want Cause 链可达原错误")
	}
	if unwrapped := stderrors.Unwrap(rebuilt); unwrapped != original {
		t.Fatalf("Unwrap(rebuilt) = %v, want 原错误", unwrapped)
	}
}

// TestCrossPlatformCoverageDevEventRunnerErrorKeepsCategory 验证重建按原类别：
// 携带信封事实的 validation 类错误重建后仍是 validation（退出码 3），不得被
// 归一为 api。
func TestCrossPlatformCoverageDevEventRunnerErrorKeepsCategory(t *testing.T) {
	original := apperrors.NewValidation("事件定义未通过服务端校验",
		apperrors.WithReason("schema_incompatible"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{
			ServerErrorCode: "SCHEMA_INCOMPATIBLE",
		}),
	)
	rebuilt := devEventRunnerError(original)
	var typed *apperrors.Error
	if !stderrors.As(rebuilt, &typed) {
		t.Fatalf("errors.As(*apperrors.Error) 失败：rebuilt = %#v", rebuilt)
	}
	if typed.Category != apperrors.CategoryValidation {
		t.Fatalf("category = %q, want validation", typed.Category)
	}
	if code := apperrors.ExitCode(rebuilt); code != apperrors.ExitCodeValidation {
		t.Fatalf("exit code = %d, want %d", code, apperrors.ExitCodeValidation)
	}
	if typed.Reason != "schema_incompatible" {
		t.Fatalf("reason = %q, want schema_incompatible", typed.Reason)
	}
	if !strings.Contains(rebuilt.Error(), "errorCode=SCHEMA_INCOMPATIBLE") {
		t.Fatalf("message = %q, want errorCode 进 message", rebuilt.Error())
	}
}

// TestCrossPlatformCoverageDevEventRunnerErrorCallError 验证未分类
// transport.CallError 的归一：API 类别（退出码 1）而非 internal(5)，
// trace 取 TraceID（缺失时回退 RequestID），CallError 本体仍在链上可 As。
func TestCrossPlatformCoverageDevEventRunnerErrorCallError(t *testing.T) {
	cause := stderrors.New("bad gateway")
	original := &transport.CallError{
		Stage:      transport.CallStageHTTP,
		HTTPStatus: 502,
		TraceID:    "trace-9",
		RPCCode:    -32000,
		Cause:      cause,
	}
	rebuilt := devEventRunnerError(original)
	var typed *apperrors.Error
	if !stderrors.As(rebuilt, &typed) {
		t.Fatalf("errors.As(*apperrors.Error) 失败：rebuilt = %#v", rebuilt)
	}
	if typed.Category != apperrors.CategoryAPI {
		t.Fatalf("category = %q, want api", typed.Category)
	}
	if code := apperrors.ExitCode(rebuilt); code != apperrors.ExitCodeAPI {
		t.Fatalf("exit code = %d, want %d", code, apperrors.ExitCodeAPI)
	}
	if typed.ServerDiag.TraceID != "trace-9" {
		t.Fatalf("trace_id = %q, want trace-9", typed.ServerDiag.TraceID)
	}
	if typed.RPCCode != -32000 {
		t.Fatalf("rpc_code = %d, want -32000", typed.RPCCode)
	}
	if typed.FailureStage != "http" {
		t.Fatalf("stage = %q, want http", typed.FailureStage)
	}
	var callErr *transport.CallError
	if !stderrors.As(rebuilt, &callErr) || callErr.HTTPStatus != 502 {
		t.Fatalf("errors.As(*transport.CallError) 失败或状态丢失：callErr = %#v", callErr)
	}
	if !stderrors.Is(rebuilt, cause) {
		t.Fatal("errors.Is(rebuilt, cause) = false, want 链可达")
	}
	if rebuilt.Error() != "bad gateway" {
		t.Fatalf("message = %q, want bad gateway", rebuilt.Error())
	}

	requestIDOnly := devEventRunnerError(&transport.CallError{
		Stage:     transport.CallStageJSONRPC,
		RequestID: "req-7",
	})
	var requestTyped *apperrors.Error
	if !stderrors.As(requestIDOnly, &requestTyped) || requestTyped.ServerDiag.TraceID != "req-7" {
		t.Fatalf("request id 回退失败：server_diag = %+v", requestTyped.ServerDiag)
	}
	if requestTyped.FailureStage != "jsonrpc" {
		t.Fatalf("stage = %q, want jsonrpc", requestTyped.FailureStage)
	}
}

func TestCrossPlatformCoverageDevEventRunnerErrorPreservesFullRetryContract(t *testing.T) {
	next := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	original := apperrors.NewAPI("business error: success=false",
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: "E_FULL_CONTRACT", TechnicalDetail: "tech-detail"}),
		apperrors.WithDetails(map[string]any{"origin": "test"}),
		apperrors.WithRetryAfterSeconds(30),
		apperrors.WithNextRetryAt(next),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRPCCode(42),
		apperrors.WithAvailableFlags("event-key", "file"),
		apperrors.WithSnapshot("/tmp/snap.json"),
		apperrors.WithRPCData(json.RawMessage(`{"k":"v"}`)),
	)
	wrapped := devEventRunnerError(original)
	var typed *apperrors.Error
	if !stderrors.As(wrapped, &typed) || typed == nil {
		t.Fatalf("wrapped error is not *apperrors.Error: %v", wrapped)
	}
	if got := typed.Snapshot; got != "/tmp/snap.json" {
		t.Fatalf("Snapshot = %q", got)
	}
	if typed.RetryAfterSeconds == nil || *typed.RetryAfterSeconds != 30 {
		t.Fatalf("RetryAfterSeconds = %v", typed.RetryAfterSeconds)
	}
	if typed.NextRetryAt == nil || !typed.NextRetryAt.Equal(next) {
		t.Fatalf("NextRetryAt = %v", typed.NextRetryAt)
	}
	if typed.ExecutionStarted == nil || !*typed.ExecutionStarted {
		t.Fatalf("ExecutionStarted = %v", typed.ExecutionStarted)
	}
	if typed.RPCCode != 42 {
		t.Fatalf("RPCCode = %d", typed.RPCCode)
	}
	if len(typed.AvailableFlags) != 2 {
		t.Fatalf("AvailableFlags = %v", typed.AvailableFlags)
	}
	if len(typed.RPCData) == 0 {
		t.Fatal("RPCData lost")
	}
	if got := typed.Details["origin"]; got != "test" {
		t.Fatalf("Details[origin] = %v", got)
	}
	if got := typed.Details["errorCode"]; got != "E_FULL_CONTRACT" {
		t.Fatalf("Details[errorCode] = %v", got)
	}
}

func TestCrossPlatformCoverageDevEventCategoryErrorConstructors(t *testing.T) {
	for _, tc := range []struct {
		category apperrors.Category
		check    func(*apperrors.Error) bool
	}{
		{apperrors.CategoryAuth, func(e *apperrors.Error) bool { return e.ExitCode() == 2 }},
		{apperrors.CategoryDiscovery, func(e *apperrors.Error) bool { return e.ExitCode() == 6 }},
		{apperrors.CategoryInternal, func(e *apperrors.Error) bool { return e.ExitCode() == 5 }},
	} {
		err := devEventCategoryError(tc.category, "boom", nil)
		var typed *apperrors.Error
		if !stderrors.As(err, &typed) {
			t.Fatalf("%s: not typed: %v", tc.category, err)
		}
		if !tc.check(typed) {
			t.Fatalf("%s: exit code = %d", tc.category, typed.ExitCode())
		}
	}
}
