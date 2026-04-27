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
	"encoding/json"
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
	if !isNotLoggedInError(body) {
		t.Fatal("expected true when equivalent auth message is present in message")
	}
}

func TestGetDWSGatewayErrorCode_CodeField(t *testing.T) {
	t.Parallel()
	body := map[string]any{"code": "DWS_SERVICE_UNAUTHORIZED"}
	code, ok := getDWSGatewayErrorCode(body)
	if !ok || code != "DWS_SERVICE_UNAUTHORIZED" {
		t.Fatalf("getDWSGatewayErrorCode() = (%q, %t), want DWS_SERVICE_UNAUTHORIZED, true", code, ok)
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

func TestClassifyToolResultContent_PATPermissionLegacyErrorCode(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"error_code": "PAT_LOW_RISK_NO_PERMISSION",
		"data":       map[string]any{"desc": "需要授权"},
	}
	err := ClassifyToolResultContent(content)
	if err == nil {
		t.Fatal("expected non-nil error for legacy error_code PAT permission")
	}
	var patErr *PATError
	if !stderrors.As(err, &patErr) {
		t.Fatalf("expected *PATError, got %T", err)
	}
	if !strings.Contains(patErr.RawJSON, "PAT_LOW_RISK_NO_PERMISSION") {
		t.Errorf("RawJSON should contain PAT_LOW_RISK_NO_PERMISSION, got: %s", patErr.RawJSON)
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

func TestClassifyMCPResponseText_PATPermissionLegacyErrorCode(t *testing.T) {
	t.Parallel()
	text := `{"error_code":"PAT_MEDIUM_RISK_NO_PERMISSION","data":{"desc":"legacy"}}`
	err := ClassifyMCPResponseText(text)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	var patErr *PATError
	if !stderrors.As(err, &patErr) {
		t.Fatalf("expected *PATError, got %T", err)
	}
	if !strings.Contains(patErr.RawJSON, "PAT_MEDIUM_RISK_NO_PERMISSION") {
		t.Errorf("RawJSON should contain legacy code, got: %s", patErr.RawJSON)
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

func TestClassifyPatAuthCheck_LegacyErrorCode(t *testing.T) {
	t.Parallel()
	content := map[string]any{"error_code": "PAT_HIGH_RISK_NO_PERMISSION", "data": map[string]any{"flowId": "f1"}}
	patErr := ClassifyPatAuthCheck(content)
	if patErr == nil {
		t.Fatal("expected non-nil *PATError for legacy error_code")
	}
	if !strings.Contains(patErr.RawJSON, "PAT_HIGH_RISK_NO_PERMISSION") {
		t.Errorf("RawJSON should contain PAT_HIGH_RISK_NO_PERMISSION, got: %s", patErr.RawJSON)
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

// TestClassifyPatAuthCheck_scope_auth_required pins the PAT_SCOPE_AUTH_REQUIRED
// selector (Frozen; see docs/pat/contract.md §2.3 + error-catalog.md
// §PAT_SCOPE_AUTH_REQUIRED) as a PATError with exit=4 so hosts can kick the
// `dws auth login --scope <data.missingScope>` branch.
func TestClassifyPatAuthCheck_scope_auth_required(t *testing.T) {
	t.Parallel()
	content := map[string]any{
		"success": false,
		"code":    "PAT_SCOPE_AUTH_REQUIRED",
		"data":    map[string]any{"missingScope": "mail:send"},
	}
	patErr := ClassifyPatAuthCheck(content)
	if patErr == nil {
		t.Fatal("expected non-nil *PATError for PAT_SCOPE_AUTH_REQUIRED")
	}

	// Error value MUST satisfy the ExitCoder contract (exit=4) so the
	// process exits with the PAT Frozen code regardless of wrapping.
	var ec interface{ ExitCode() int } = patErr
	if ec.ExitCode() != ExitCodePermission {
		t.Errorf("ExitCode() = %d, want %d", ec.ExitCode(), ExitCodePermission)
	}

	// Host-visible RawStderr must carry the selector and, crucially, the
	// missingScope field that drives `dws auth login --scope <x>`.
	raw := patErr.RawStderr()
	if !strings.Contains(raw, "PAT_SCOPE_AUTH_REQUIRED") {
		t.Errorf("RawStderr missing selector, got: %s", raw)
	}
	if !strings.Contains(raw, "missingScope") {
		t.Errorf("RawStderr missing missingScope field, got: %s", raw)
	}
	if !strings.Contains(raw, "mail:send") {
		t.Errorf("RawStderr missing missingScope value, got: %s", raw)
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

// TestCleanPATJSON_InjectsHostControlWhenClawSet verifies the contract
// invariant from docs/pat/contract.md §5.2: when the bootstrap wires a
// non-empty clawType provider, cleanPATJSON MUST emit data.hostControl.
func TestCleanPATJSON_InjectsHostControlWhenClawSet(t *testing.T) {
	// Not parallel: mutates the package-level provider.
	t.Cleanup(func() { SetHostControlProvider(nil) })
	SetHostControlProvider(func() string { return "my-copilot" })

	body := map[string]any{
		"success": false,
		"code":    "PAT_NO_PERMISSION",
		"data": map[string]any{
			"desc":      "需要授权",
			"flowId":    "f-1",
			"callbacks": []any{"cb1", "cb2"},
		},
	}
	raw := cleanPATJSON(body, "PAT_NO_PERMISSION")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal cleanPATJSON output: %v\nraw=%s", err, raw)
	}
	data, ok := parsed["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", parsed["data"])
	}
	hc, ok := data["hostControl"].(map[string]any)
	if !ok {
		t.Fatalf("expected data.hostControl to be a map, got %T\nraw=%s", data["hostControl"], raw)
	}
	if got, _ := hc["clawType"].(string); got != "my-copilot" {
		t.Errorf("hostControl.clawType = %q, want %q", got, "my-copilot")
	}
	if got, _ := hc["callbackOwner"].(string); got != "host" {
		t.Errorf("hostControl.callbackOwner = %q, want %q", got, "host")
	}
	if got, _ := hc["mode"].(string); got != "host" {
		t.Errorf("hostControl.mode = %q, want %q", got, "host")
	}
	if _, ok := data["callbacks"]; ok {
		t.Fatalf("cleanPATJSON should strip callbacks in host-owned mode, got: %v", data["callbacks"])
	}
}

// TestCleanPATJSON_OmitsHostControlByDefault verifies that cleanPATJSON
// does NOT include a hostControl block when the provider is unset or
// returns empty (default CLI-owned mode).
func TestCleanPATJSON_OmitsHostControlByDefault(t *testing.T) {
	// Not parallel: reads the package-level provider.
	t.Cleanup(func() { SetHostControlProvider(nil) })
	SetHostControlProvider(nil)

	body := map[string]any{
		"success": false,
		"code":    "PAT_NO_PERMISSION",
		"data": map[string]any{
			"desc": "need auth",
		},
	}
	raw := cleanPATJSON(body, "PAT_NO_PERMISSION")
	if strings.Contains(raw, `"hostControl"`) {
		t.Fatalf("cleanPATJSON should omit hostControl in default mode, got: %s", raw)
	}

	SetHostControlProvider(func() string { return "" })
	raw = cleanPATJSON(body, "PAT_NO_PERMISSION")
	if strings.Contains(raw, `"hostControl"`) {
		t.Fatalf("cleanPATJSON should omit hostControl when provider returns empty, got: %s", raw)
	}
}

// TestCleanPATJSON_SingleLineOutput pins down the docs/pat/contract.md §2 /
// docs/pat/contract.md §2 wire invariant: stderr JSON MUST be emitted
// as a single line (no embedded \n, no pretty-print indentation) so
// that naïve host parsers reading stderr line-by-line stay correct.
// Regression guard against accidental reintroduction of
// json.MarshalIndent.
func TestCleanPATJSON_SingleLineOutput(t *testing.T) {
	t.Parallel()
	body := map[string]any{
		"success": false,
		"code":    "PAT_LOW_RISK_NO_PERMISSION",
		"data": map[string]any{
			"requiredScopes": []any{"aitable.record:read"},
			"grantOptions":   []any{"session", "permanent"},
			"displayName":    "读取记录",
			"productName":    "AI 表格",
		},
	}
	raw := cleanPATJSON(body, "PAT_LOW_RISK_NO_PERMISSION")

	if strings.Contains(raw, "\n") {
		t.Fatalf("cleanPATJSON output must be single-line, got embedded newline:\n%s", raw)
	}
	if strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t") {
		t.Fatalf("cleanPATJSON output must not be indented, got leading whitespace: %q", raw)
	}

	// Contract: the payload must remain a directly json.Unmarshal-able
	// object, even after the single-line constraint is enforced.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("single-line output must round-trip via json.Unmarshal: %v\nraw=%s", err, raw)
	}
	if code, _ := parsed["code"].(string); code != "PAT_LOW_RISK_NO_PERMISSION" {
		t.Errorf("code = %q, want %q", code, "PAT_LOW_RISK_NO_PERMISSION")
	}
}

func TestCleanPATJSON_PreservesOpaqueURIVerbatim(t *testing.T) {
	t.Parallel()
	rawURI := "https://open-dev.dingtalk.com/fe/old?hash=%23%2FpersonalAuthorization%3FflowId%3D50dff7654b7444e88ced7489b07cce8d%26userCode%3DQ8RY-X6E9#/personalAuthorization?flowId=50dff7654b7444e88ced7489b07cce8d&userCode=Q8RY-X6E9"
	body := map[string]any{
		"success": false,
		"code":    "PAT_MEDIUM_RISK_NO_PERMISSION",
		"data": map[string]any{
			"desc":   "在浏览器中打开以下链接进行认证",
			"flowId": "50dff7654b7444e88ced7489b07cce8d",
			"uri":    rawURI,
		},
	}

	result := cleanPATJSON(body, "PAT_MEDIUM_RISK_NO_PERMISSION")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal cleanPATJSON output: %v\nraw=%s", err, result)
	}
	data, _ := parsed["data"].(map[string]any)
	if got, _ := data["uri"].(string); got != rawURI {
		t.Fatalf("data.uri = %q, want verbatim %q", got, rawURI)
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
