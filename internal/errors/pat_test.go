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

package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// PATError basic behaviour
// ---------------------------------------------------------------------------

func TestPATError_Implements(t *testing.T) {
	t.Parallel()
	raw := `{"success":false,"code":"PAT_NO_PERMISSION"}`
	pe := &PATError{RawJSON: raw}

	if pe.Error() != raw {
		t.Errorf("Error() = %q, want %q", pe.Error(), raw)
	}
	if pe.ExitCode() != ExitCodePermission {
		t.Errorf("ExitCode() = %d, want %d", pe.ExitCode(), ExitCodePermission)
	}
	if pe.RawStderr() != raw {
		t.Errorf("RawStderr() = %q, want %q", pe.RawStderr(), raw)
	}
}

func TestIsPATError_True(t *testing.T) {
	t.Parallel()
	err := &PATError{RawJSON: "{}"}
	if !IsPATError(err) {
		t.Fatal("expected IsPATError to return true for *PATError")
	}
}

func TestIsPATError_False(t *testing.T) {
	t.Parallel()
	err := stderrors.New("some other error")
	if IsPATError(err) {
		t.Fatal("expected IsPATError to return false for non-PATError")
	}
}

// ---------------------------------------------------------------------------
// IsPATNoPermissionCode
// ---------------------------------------------------------------------------

func TestIsPATNoPermissionCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		want bool
	}{
		{"PAT_NO_PERMISSION", true},
		{"PAT_LOW_RISK_NO_PERMISSION", true},
		{"PAT_MEDIUM_RISK_NO_PERMISSION", true},
		{"PAT_HIGH_RISK_NO_PERMISSION", true},
		{"AGENT_CODE_NOT_EXISTS", false},
		{"UNKNOWN_CODE", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsPATNoPermissionCode(tc.code); got != tc.want {
			t.Errorf("IsPATNoPermissionCode(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// getDWSGatewayErrorCode
// ---------------------------------------------------------------------------

func TestGetDWSGatewayErrorCode_ErrorCode(t *testing.T) {
	t.Parallel()
	body := map[string]any{"errorCode": "DWS_SERVICE_UNAUTHORIZED"}
	code, ok := getDWSGatewayErrorCode(body)
	if !ok || code != "DWS_SERVICE_UNAUTHORIZED" {
		t.Errorf("got (%q, %v), want (DWS_SERVICE_UNAUTHORIZED, true)", code, ok)
	}
}

func TestGetDWSGatewayErrorCode_ErrorCodeUnderscore(t *testing.T) {
	t.Parallel()
	body := map[string]any{"error_code": "DWS_AUTH_SERVICE_FAILED"}
	code, ok := getDWSGatewayErrorCode(body)
	if !ok || code != "DWS_AUTH_SERVICE_FAILED" {
		t.Errorf("got (%q, %v), want (DWS_AUTH_SERVICE_FAILED, true)", code, ok)
	}
}

func TestGetDWSGatewayErrorCode_Unknown(t *testing.T) {
	t.Parallel()
	body := map[string]any{"errorCode": "SOME_OTHER_ERROR"}
	_, ok := getDWSGatewayErrorCode(body)
	if ok {
		t.Fatal("expected ok=false for unknown error code")
	}
}

func TestGetDWSGatewayErrorCode_Empty(t *testing.T) {
	t.Parallel()
	body := map[string]any{}
	_, ok := getDWSGatewayErrorCode(body)
	if ok {
		t.Fatal("expected ok=false for empty body")
	}
}

// ---------------------------------------------------------------------------
// isNotLoggedInError
// ---------------------------------------------------------------------------

func TestIsNotLoggedInError_True(t *testing.T) {
	t.Parallel()
	body := map[string]any{"error": "Missing service_id or access_key in request headers"}
	if !isNotLoggedInError(body) {
		t.Fatal("expected true for Missing service_id message")
	}
}

func TestIsNotLoggedInError_False(t *testing.T) {
	t.Parallel()
	body := map[string]any{"error": "something else happened"}
	if isNotLoggedInError(body) {
		t.Fatal("expected false for unrelated error message")
	}
}

func TestIsNotLoggedInError_NoErrorField(t *testing.T) {
	t.Parallel()
	body := map[string]any{"message": "Missing service_id or access_key"}
	if isNotLoggedInError(body) {
		t.Fatal("expected false when error field is absent")
	}
}

// ---------------------------------------------------------------------------
// isBusinessError
// ---------------------------------------------------------------------------

func TestIsBusinessError_ErrorField(t *testing.T) {
	t.Parallel()
	body := map[string]any{"error": "some error message"}
	if !isBusinessError(body) {
		t.Fatal("expected true when 'error' field is present")
	}
}

func TestIsBusinessError_SuccessBoolFalse(t *testing.T) {
	t.Parallel()
	body := map[string]any{"success": false}
	if !isBusinessError(body) {
		t.Fatal("expected true when success=false (bool)")
	}
}

func TestIsBusinessError_SuccessStringFalse(t *testing.T) {
	t.Parallel()
	body := map[string]any{"success": "False"}
	if !isBusinessError(body) {
		t.Fatal("expected true when success=\"False\" (string)")
	}
}

func TestIsBusinessError_SuccessTrue(t *testing.T) {
	t.Parallel()
	body := map[string]any{"success": true, "data": "ok"}
	if isBusinessError(body) {
		t.Fatal("expected false when success=true")
	}
}

func TestIsBusinessError_EmptyBody(t *testing.T) {
	t.Parallel()
	body := map[string]any{"data": "hello"}
	if isBusinessError(body) {
		t.Fatal("expected false for body without error indicators")
	}
}

// ---------------------------------------------------------------------------
// ClassifyToolResultContent
// ---------------------------------------------------------------------------

func TestClassifyToolResultContent_GatewayAuth(t *testing.T) {
	t.Parallel()
	content := map[string]any{"errorCode": "DWS_SERVICE_UNAUTHORIZED", "message": "expired"}
	err := ClassifyToolResultContent(content)
	if err == nil {
		t.Fatal("expected non-nil error for gateway auth")
	}
	var typed *Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if typed.Category != CategoryAuth {
		t.Errorf("Category = %v, want %v", typed.Category, CategoryAuth)
	}
	if typed.Reason != "gateway_auth_expired" {
		t.Errorf("Reason = %q, want gateway_auth_expired", typed.Reason)
	}
}

func TestClassifyToolResultContent_PATPermission(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"code": "PAT_NO_PERMISSION",
		"data": map[string]any{"desc": "需要授权"},
	}
	err := ClassifyToolResultContent(content)
	if err == nil {
		t.Fatal("expected non-nil error for PAT permission")
	}
	var patErr *PATError
	if !stderrors.As(err, &patErr) {
		t.Fatalf("expected *PATError, got %T", err)
	}
	if !strings.Contains(patErr.RawJSON, "PAT_NO_PERMISSION") {
		t.Errorf("RawJSON should contain PAT_NO_PERMISSION, got: %s", patErr.RawJSON)
	}
}

func TestClassifyToolResultContent_NoError(t *testing.T) {
	t.Parallel()
	content := map[string]any{"success": true, "data": "ok"}
	if err := ClassifyToolResultContent(content); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ClassifyMCPResponseText
// ---------------------------------------------------------------------------

func TestClassifyMCPResponseText_GatewayAuth(t *testing.T) {
	t.Parallel()
	text := `{"errorCode":"DWS_SERVICE_UNAUTHORIZED","message":"token expired"}`
	err := ClassifyMCPResponseText(text)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	var typed *Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if typed.Reason != "gateway_auth_expired" {
		t.Errorf("Reason = %q, want gateway_auth_expired", typed.Reason)
	}
}

func TestClassifyMCPResponseText_NotLoggedIn(t *testing.T) {
	t.Parallel()
	text := `{"error":"Missing service_id or access_key in headers"}`
	err := ClassifyMCPResponseText(text)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	var typed *Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if typed.Reason != "not_configured" {
		t.Errorf("Reason = %q, want not_configured", typed.Reason)
	}
}

func TestClassifyMCPResponseText_PATPermission(t *testing.T) {
	t.Parallel()
	text := `{"code":"PAT_HIGH_RISK_NO_PERMISSION","data":{"desc":"high risk"}}`
	err := ClassifyMCPResponseText(text)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	var patErr *PATError
	if !stderrors.As(err, &patErr) {
		t.Fatalf("expected *PATError, got %T", err)
	}
	if !strings.Contains(patErr.RawJSON, "PAT_HIGH_RISK_NO_PERMISSION") {
		t.Errorf("RawJSON should contain code, got: %s", patErr.RawJSON)
	}
}

func TestClassifyMCPResponseText_BusinessError(t *testing.T) {
	t.Parallel()
	text := `{"success":false,"errorMsg":"搜索内容不能为空"}`
	err := ClassifyMCPResponseText(text)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	var typed *Error
	if !stderrors.As(err, &typed) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if typed.Reason != "business_error" {
		t.Errorf("Reason = %q, want business_error", typed.Reason)
	}
	if !strings.Contains(typed.Hint, "搜索关键词") {
		t.Errorf("Hint should contain search suggestion, got: %s", typed.Hint)
	}
}

func TestClassifyMCPResponseText_InvalidJSON(t *testing.T) {
	t.Parallel()
	text := "not json at all"
	if err := ClassifyMCPResponseText(text); err != nil {
		t.Fatalf("expected nil for invalid JSON, got %v", err)
	}
}

func TestClassifyMCPResponseText_NoError(t *testing.T) {
	t.Parallel()
	text := `{"success":true,"data":"hello"}`
	if err := ClassifyMCPResponseText(text); err != nil {
		t.Fatalf("expected nil for success response, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ClassifyPatAuthCheck
// ---------------------------------------------------------------------------

func TestClassifyPatAuthCheck_PATNoPermission(t *testing.T) {
	t.Parallel()
	content := map[string]any{"code": "PAT_NO_PERMISSION", "data": map[string]any{"flowId": "f1"}}
	patErr := ClassifyPatAuthCheck(content)
	if patErr == nil {
		t.Fatal("expected non-nil *PATError")
	}
	if !strings.Contains(patErr.RawJSON, "PAT_NO_PERMISSION") {
		t.Errorf("RawJSON should contain code, got: %s", patErr.RawJSON)
	}
}

func TestClassifyPatAuthCheck_AgentCodeNotExists(t *testing.T) {
	t.Parallel()
	content := map[string]any{"errorCode": "AGENT_CODE_NOT_EXISTS", "data": map[string]any{"clientId": "c1"}}
	patErr := ClassifyPatAuthCheck(content)
	if patErr == nil {
		t.Fatal("expected non-nil *PATError for AGENT_CODE_NOT_EXISTS")
	}
	if !strings.Contains(patErr.RawJSON, "AGENT_CODE_NOT_EXISTS") {
		t.Errorf("RawJSON should contain AGENT_CODE_NOT_EXISTS, got: %s", patErr.RawJSON)
	}
}

func TestClassifyPatAuthCheck_NoMatch(t *testing.T) {
	t.Parallel()
	content := map[string]any{"code": "SOME_BUSINESS_ERROR", "message": "oops"}
	if patErr := ClassifyPatAuthCheck(content); patErr != nil {
		t.Fatalf("expected nil, got %v", patErr)
	}
}

func TestClassifyPatAuthCheck_EmptyContent(t *testing.T) {
	t.Parallel()
	content := map[string]any{}
	if patErr := ClassifyPatAuthCheck(content); patErr != nil {
		t.Fatalf("expected nil for empty content, got %v", patErr)
	}
}

// ---------------------------------------------------------------------------
// AsPatAuthCheckError
// ---------------------------------------------------------------------------

func TestAsPatAuthCheckError_Wrapped(t *testing.T) {
	t.Parallel()
	inner := &PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
	wrapped := stderrors.Join(stderrors.New("context"), inner)
	got := AsPatAuthCheckError(wrapped)
	if got == nil {
		t.Fatal("expected non-nil *PATError from wrapped error")
	}
	if got.RawJSON != inner.RawJSON {
		t.Errorf("RawJSON = %q, want %q", got.RawJSON, inner.RawJSON)
	}
}

func TestAsPatAuthCheckError_NotPAT(t *testing.T) {
	t.Parallel()
	err := stderrors.New("just a plain error")
	if got := AsPatAuthCheckError(err); got != nil {
		t.Fatalf("expected nil for non-PAT error, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// cleanPATJSON
// ---------------------------------------------------------------------------

func TestCleanPATJSON_WithData(t *testing.T) {
	t.Parallel()
	body := map[string]any{
		"success": false,
		"code":    "PAT_NO_PERMISSION",
		"data": map[string]any{
			"desc":   "需要授权",
			"flowId": "f123",
			"class":  "com.foo.Bar",
		},
	}
	result := cleanPATJSON(body, "PAT_NO_PERMISSION")
	if !strings.Contains(result, "PAT_NO_PERMISSION") {
		t.Errorf("expected code in output, got: %s", result)
	}
	if !strings.Contains(result, "flowId") {
		t.Errorf("expected flowId in data, got: %s", result)
	}
	if strings.Contains(result, "class") {
		t.Errorf("expected class field to be stripped, got: %s", result)
	}
}

func TestCleanPATJSON_WithoutData(t *testing.T) {
	t.Parallel()
	body := map[string]any{
		"success": false,
		"code":    "PAT_NO_PERMISSION",
		"message": "no permission",
		"extra":   "value",
	}
	result := cleanPATJSON(body, "PAT_NO_PERMISSION")
	if !strings.Contains(result, "extra") {
		t.Errorf("expected extra field in fallback data, got: %s", result)
	}
	// Top-level stripped fields should not appear
	if strings.Contains(result, `"message"`) {
		t.Errorf("expected message to be stripped from top level, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// stripClassFields
// ---------------------------------------------------------------------------

func TestStripClassFields_Map(t *testing.T) {
	t.Parallel()
	input := map[string]any{
		"name":  "test",
		"class": "com.foo.Bar",
		"nested": map[string]any{
			"value": 42,
			"class": "com.baz.Qux",
		},
	}
	result := stripClassFields(input).(map[string]any)
	if _, ok := result["class"]; ok {
		t.Error("top-level class should be removed")
	}
	nested := result["nested"].(map[string]any)
	if _, ok := nested["class"]; ok {
		t.Error("nested class should be removed")
	}
	if nested["value"] != 42 {
		t.Errorf("nested value should be preserved, got %v", nested["value"])
	}
}

func TestStripClassFields_Array(t *testing.T) {
	t.Parallel()
	input := []any{
		map[string]any{"id": 1, "class": "Foo"},
		map[string]any{"id": 2},
	}
	result := stripClassFields(input).([]any)
	first := result[0].(map[string]any)
	if _, ok := first["class"]; ok {
		t.Error("class in array element should be removed")
	}
	if first["id"] != 1 {
		t.Error("other fields in array element should be preserved")
	}
}

func TestStripClassFields_Scalar(t *testing.T) {
	t.Parallel()
	if stripClassFields("hello") != "hello" {
		t.Error("scalar string should pass through unchanged")
	}
	if stripClassFields(42) != 42 {
		t.Error("scalar int should pass through unchanged")
	}
}

// ---------------------------------------------------------------------------
// suggestForBusinessErrorText
// ---------------------------------------------------------------------------

func TestSuggestForBusinessErrorText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		body     map[string]any
		contains string
	}{
		{map[string]any{"errorMsg": "搜索内容不能为空"}, "搜索关键词"},
		{map[string]any{"message": "User has no permission to access this email"}, "邮箱"},
		{map[string]any{"error": "频率超限"}, "rate limit"},
		{map[string]any{"errorMsg": "参数错误"}, "parameters"},
		{map[string]any{"error": "unknown"}, "business error"},
	}
	for _, tc := range cases {
		hint := suggestForBusinessErrorText(tc.body)
		if !strings.Contains(strings.ToLower(hint), strings.ToLower(tc.contains)) {
			t.Errorf("suggestForBusinessErrorText(%v) = %q, want to contain %q", tc.body, hint, tc.contains)
		}
	}
}
