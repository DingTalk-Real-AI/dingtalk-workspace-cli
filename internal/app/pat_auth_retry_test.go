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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

func TestIsPatScopeError_MissingScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("missing required scope(s): mail:user_mailbox.message:send")
	if !isPatScopeError(err) {
		t.Fatal("expected missing_scope error to be detected")
	}
}

func TestIsPatScopeError_PlainString(t *testing.T) {
	t.Parallel()
	err := &PatScopeError{
		OriginalError: "missing_scope: user lacks required scope",
		ErrorType:     "missing_scope",
		Message:       "user lacks required scope",
	}
	if !isPatScopeError(err) {
		t.Fatal("expected plain string with missing_scope to be detected")
	}
}

func TestIsPatScopeError_NotScopeError(t *testing.T) {
	t.Parallel()
	err := apperrors.NewValidation("invalid parameter")
	if isPatScopeError(err) {
		t.Fatal("expected validation error NOT to be detected as scope error")
	}
}

func TestIsPatScopeError_Nil(t *testing.T) {
	t.Parallel()
	if isPatScopeError(nil) {
		t.Fatal("nil error should not be detected as scope error")
	}
}

func TestIsPatScopeError_WithReason(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("API error",
		apperrors.WithReason("missing_scope"),
	)
	if !isPatScopeError(err) {
		t.Fatal("expected error with missing_scope reason to be detected")
	}
}

func TestIsPatScopeError_InsufficientScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("insufficient_scope for resource",
		apperrors.WithReason("insufficient_scope"),
	)
	if !isPatScopeError(err) {
		t.Fatal("expected insufficient_scope error to be detected")
	}
}

func TestExtractPatScopeError_MissingScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("missing required scope(s): mail:user_mailbox.message:send")
	scopeErr := extractPatScopeError(err)
	if scopeErr == nil {
		t.Fatal("expected non-nil PatScopeError")
	}
	if scopeErr.ErrorType != "missing_scope" {
		t.Errorf("expected error type 'missing_scope', got %q", scopeErr.ErrorType)
	}
	if !strings.Contains(scopeErr.Hint, "dws auth login") {
		t.Errorf("expected hint to contain 'dws auth login', got %q", scopeErr.Hint)
	}
}

func TestExtractPatScopeError_ExtractsScope(t *testing.T) {
	t.Parallel()
	err := &PatScopeError{
		OriginalError: "missing_scope: user needs calendar:read",
		ErrorType:     "missing_scope",
		Message:       "user needs calendar:read",
	}
	scopeErr := extractPatScopeError(err)
	if scopeErr == nil {
		t.Fatal("expected non-nil PatScopeError")
	}
	if scopeErr.MissingScope != "calendar:read" {
		t.Errorf("expected MissingScope 'calendar:read', got %q", scopeErr.MissingScope)
	}
}

func TestPrintPatAuthError_HumanReadable(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	scopeErr := &PatScopeError{
		Identity:     "user",
		ErrorType:    "missing_scope",
		Message:      "missing required scope(s): mail:user_mailbox.message:send",
		Hint:         "run `dws auth login --scope \"mail:user_mailbox.message:send\"` to authorize",
		MissingScope: "mail:user_mailbox.message:send",
	}
	PrintPatAuthError(&buf, scopeErr)

	output := buf.String()
	if !strings.Contains(output, "missing_scope") {
		t.Errorf("expected output to contain 'missing_scope', got: %s", output)
	}
	if !strings.Contains(output, "dws auth login") {
		t.Errorf("expected output to contain 'dws auth login', got: %s", output)
	}
	if !strings.Contains(output, "需要额外授权") {
		t.Errorf("expected output to contain Chinese auth prompt, got: %s", output)
	}
}

func TestPrintPatAuthJSON_MachineReadable(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	scopeErr := &PatScopeError{
		Identity:     "user",
		ErrorType:    "missing_scope",
		Message:      "missing required scope(s): mail:send",
		Hint:         "run dws auth login --scope mail:send",
		MissingScope: "mail:send",
	}
	PrintPatAuthJSON(&buf, scopeErr)

	output := buf.String()
	if !strings.Contains(output, `"ok": false`) {
		t.Errorf("expected JSON to contain ok: false, got: %s", output)
	}
	if !strings.Contains(output, `"missing_scope": "mail:send"`) {
		t.Errorf("expected JSON to contain missing_scope, got: %s", output)
	}
}

func TestIsPatScopeError_BusinessPermissionDenied(t *testing.T) {
	t.Parallel()
	// Generic business "permission denied" should NOT trigger PAT re-auth.
	err := apperrors.NewAuth("User has no permission to access this mailbox, permission denied")
	if isPatScopeError(err) {
		t.Fatal("generic 'permission denied' should not be detected as PAT scope error")
	}
}

func TestIsPatScopeError_GenericForbidden(t *testing.T) {
	t.Parallel()
	// HTTP 403 Forbidden should NOT trigger PAT re-auth.
	err := apperrors.NewAuth("403 Forbidden")
	if isPatScopeError(err) {
		t.Fatal("'403 Forbidden' should not be detected as PAT scope error")
	}
}

func TestExtractPatScopeError_ComplexScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("missing required scope(s): mail:user_mailbox.message:send")
	scopeErr := extractPatScopeError(err)
	if scopeErr == nil {
		t.Fatal("expected non-nil PatScopeError")
	}
	if scopeErr.MissingScope != "mail:user_mailbox.message:send" {
		t.Errorf("expected MissingScope 'mail:user_mailbox.message:send', got %q", scopeErr.MissingScope)
	}
}

func TestPatScopeError_Error(t *testing.T) {
	t.Parallel()
	err := &PatScopeError{
		OriginalError: "test error message",
	}
	if err.Error() != "test error message" {
		t.Errorf("expected Error() to return OriginalError, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// pollPatDeviceFlow integration tests — httptest mock covering four terminal
// states: APPROVED, REJECTED, EXPIRED, CANCELLED (ctx cancel).
// ---------------------------------------------------------------------------

// setupPollServer creates an httptest server that responds to
// /cli/oauth/device/poll?flowId=<fid> with the given status sequence.
// It also writes the server URL into a temp DWS_CONFIG_DIR/mcp_url so that
// GetMCPBaseURL() returns the test server address.
func setupPollServer(t *testing.T, statuses []authpkg.DevicePollResponse) (*httptest.Server, string) {
	t.Helper()
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(callCount.Add(1)) - 1
		if idx >= len(statuses) {
			idx = len(statuses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(statuses[idx])
	}))

	// Write mcp_url so GetMCPBaseURL picks up the test server.
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "mcp_url"), []byte(server.URL), 0644)
	t.Setenv("DWS_CONFIG_DIR", tmpDir)

	return server, tmpDir
}

func TestPollPatDeviceFlow_Approved(t *testing.T) {
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: true, Data: authpkg.DevicePollData{Status: "PENDING"}},
		{Success: true, Data: authpkg.DevicePollData{Status: "APPROVED", AuthCode: "code123"}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-1", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "APPROVED" {
		t.Errorf("expected APPROVED, got %q", status)
	}
	if authCode != "code123" {
		t.Errorf("expected authCode 'code123', got %q", authCode)
	}
}

func TestPollPatDeviceFlow_Rejected(t *testing.T) {
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: false, Data: authpkg.DevicePollData{Status: "REJECTED"}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-2", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "REJECTED" {
		t.Errorf("expected REJECTED, got %q", status)
	}
	if authCode != "" {
		t.Errorf("expected empty authCode for REJECTED, got %q", authCode)
	}
}

func TestPollPatDeviceFlow_Expired(t *testing.T) {
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: false, Data: authpkg.DevicePollData{Status: "EXPIRED"}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-3", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "EXPIRED" {
		t.Errorf("expected EXPIRED, got %q", status)
	}
	if authCode != "" {
		t.Errorf("expected empty authCode for EXPIRED, got %q", authCode)
	}
}

func TestPollPatDeviceFlow_Cancelled(t *testing.T) {
	// Server always returns PENDING so context cancellation is the only exit.
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: true, Data: authpkg.DevicePollData{Status: "PENDING"}},
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after first poll tick.
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-4", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %q", status)
	}
	if authCode != "" {
		t.Errorf("expected empty authCode for CANCELLED, got %q", authCode)
	}
}

// ---------------------------------------------------------------------------
// IsPatRetrying tests
// ---------------------------------------------------------------------------

func TestIsPatRetrying_Default(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if IsPatRetrying(ctx) {
		t.Fatal("expected false for plain context")
	}
}

func TestIsPatRetrying_WithValue(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), patRetryingKey, true)
	if !IsPatRetrying(ctx) {
		t.Fatal("expected true when pat retry key is set")
	}
}

// ---------------------------------------------------------------------------
// pollPatDeviceFlow edge cases
// ---------------------------------------------------------------------------

func TestPollPatDeviceFlow_ServerErrorFallback(t *testing.T) {
	// When server returns success=false with empty status, should treat as EXPIRED.
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: false, Data: authpkg.DevicePollData{Status: ""}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-err", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "EXPIRED" {
		t.Errorf("expected EXPIRED for server error fallback, got %q", status)
	}
	if authCode != "" {
		t.Errorf("expected empty authCode for server error, got %q", authCode)
	}
}

func TestPollPatDeviceFlow_RedirectSkipped(t *testing.T) {
	// When server returns 302 (SSO redirect), poll should continue until real response.
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 1 {
			// First call: simulate SSO redirect
			w.Header().Set("Location", "https://sso.example.com")
			w.WriteHeader(http.StatusFound)
			return
		}
		// Second call: return APPROVED
		w.Header().Set("Content-Type", "application/json")
		resp := authpkg.DevicePollResponse{
			Success: true,
			Data:    authpkg.DevicePollData{Status: "APPROVED"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "mcp_url"), []byte(server.URL), 0644)
	t.Setenv("DWS_CONFIG_DIR", tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, _, err := pollPatDeviceFlow(ctx, "flow-redirect", tmpDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "APPROVED" {
		t.Errorf("expected APPROVED after redirect, got %q", status)
	}
}

func TestPollPatDeviceFlow_UnknownStatusPrintsRawResponse(t *testing.T) {
	t.Setenv("DWS_DEBUG_PAT_POLL", "1")
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: true, Data: authpkg.DevicePollData{Status: ""}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-unknown", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "" {
		t.Fatalf("expected empty unknown status, got %q", status)
	}
	if authCode != "" {
		t.Fatalf("expected empty authCode for unknown status, got %q", authCode)
	}
	output := buf.String()
	if !strings.Contains(output, "PAT 轮询接口返回原文") {
		t.Fatalf("expected raw poll response to be printed, got %q", output)
	}
	if !strings.Contains(output, `"status":""`) {
		t.Fatalf("expected raw poll body in output, got %q", output)
	}
}

func TestPollPatDeviceFlow_UnknownStatusHidesRawResponseByDefault(t *testing.T) {
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: true, Data: authpkg.DevicePollData{Status: ""}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-unknown-default", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "" {
		t.Fatalf("expected empty unknown status, got %q", status)
	}
	if authCode != "" {
		t.Fatalf("expected empty authCode for unknown status, got %q", authCode)
	}
	output := buf.String()
	if strings.Contains(output, "PAT 轮询接口返回原文") {
		t.Fatalf("expected raw poll response to stay hidden by default, got %q", output)
	}
}

func TestPollPatDeviceFlow_ResultEnvelopeCompatibility(t *testing.T) {
	server, configDir := setupPollServer(t, []authpkg.DevicePollResponse{
		{Success: true, Result: authpkg.DevicePollData{Status: "PENDING"}},
		{Success: true, Result: authpkg.DevicePollData{Status: "APPROVED", AuthCode: "code-from-result"}},
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	status, authCode, err := pollPatDeviceFlow(ctx, "flow-result", configDir, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "APPROVED" {
		t.Fatalf("expected APPROVED, got %q", status)
	}
	if authCode != "code-from-result" {
		t.Fatalf("expected authCode from result envelope, got %q", authCode)
	}
}

// ---------------------------------------------------------------------------
// extractPatScopeError edge cases
// ---------------------------------------------------------------------------

func TestExtractPatScopeError_Nil(t *testing.T) {
	t.Parallel()
	if got := extractPatScopeError(nil); got != nil {
		t.Fatalf("expected nil for nil error, got %+v", got)
	}
}

func TestExtractPatScopeError_WithIdentity(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth(`insufficient_scope: identity "app_user" needs calendar:write`)
	scopeErr := extractPatScopeError(err)
	if scopeErr == nil {
		t.Fatal("expected non-nil PatScopeError")
	}
	if scopeErr.Identity != "app_user" {
		t.Errorf("expected Identity 'app_user', got %q", scopeErr.Identity)
	}
	if scopeErr.MissingScope != "calendar:write" {
		t.Errorf("expected MissingScope 'calendar:write', got %q", scopeErr.MissingScope)
	}
}

// ---------------------------------------------------------------------------
// handlePatAuthCheck integration tests — cover the main orchestrator with
// mock runner + httptest poll server for APPROVED, REJECTED, EmptyFlowID.
// ---------------------------------------------------------------------------

// mockRunner is a simple executor.Runner for testing handlePatAuthCheck.
type mockRunner struct {
	runFunc func(ctx context.Context, inv executor.Invocation) (executor.Result, error)
}

func (m *mockRunner) Run(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
	return m.runFunc(ctx, inv)
}

// setupHandlePATServer creates an httptest server for handlePatAuthCheck tests.
// It responds to device poll requests with the given status after the first poll.
func setupHandlePATServer(t *testing.T, terminalStatus string, authCode string) (*httptest.Server, string) {
	t.Helper()
	var pollCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cli/oauth/device/poll") {
			idx := int(pollCount.Add(1)) - 1
			var resp authpkg.DevicePollResponse
			if idx == 0 {
				resp = authpkg.DevicePollResponse{Success: true, Data: authpkg.DevicePollData{Status: "PENDING"}}
			} else {
				resp = authpkg.DevicePollResponse{
					Success: terminalStatus == "APPROVED",
					Data:    authpkg.DevicePollData{Status: terminalStatus, AuthCode: authCode},
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "mcp_url"), []byte(server.URL), 0644)
	t.Setenv("DWS_CONFIG_DIR", tmpDir)

	return server, tmpDir
}

func makePATErrorJSON(flowID, clientID string) string {
	return makePATErrorJSONWithURI(flowID, clientID, "")
}

func makePATErrorJSONWithURI(flowID, clientID, uri string) string {
	type patData struct {
		Desc     string `json:"desc"`
		FlowID   string `json:"flowId"`
		URI      string `json:"uri"`
		ClientID string `json:"clientId"`
	}
	payload := struct {
		Code string  `json:"code"`
		Data patData `json:"data"`
	}{
		Code: "AGENT_CODE_NOT_EXISTS",
		Data: patData{
			Desc:     "test auth",
			FlowID:   flowID,
			URI:      uri,
			ClientID: clientID,
		},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func TestHandlePatAuthCheck_Approved(t *testing.T) {
	server, configDir := setupHandlePATServer(t, "APPROVED", "test-auth-code")
	defer server.Close()

	var retryCalled bool
	var retryHasKey bool
	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			retryCalled = true
			retryHasKey = IsPatRetrying(ctx)
			return executor.Result{Response: map[string]any{"ok": true}}, nil
		},
	}

	runner := &runtimeRunner{fallback: mock}
	patErr := &apperrors.PATError{RawJSON: makePATErrorJSON("flow-approved", "test-client-id")}

	ctx := context.Background()
	var buf bytes.Buffer
	_, err := handlePatAuthCheck(ctx, runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "test_tool",
	}, patErr, configDir, &buf)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retryCalled {
		t.Fatal("expected mock runner to be called for retry")
	}
	if !retryHasKey {
		t.Fatal("expected retry context to have patRetryingKey")
	}
	// Verify SetClientIDFromMCP was called with the PAT response clientId.
	if cid := authpkg.ClientID(); cid != "test-client-id" {
		t.Errorf("expected ClientID 'test-client-id', got %q", cid)
	}
}

func TestHandlePatAuthCheck_Rejected(t *testing.T) {
	server, configDir := setupHandlePATServer(t, "REJECTED", "")
	defer server.Close()

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner should not be called on REJECTED")
			return executor.Result{}, nil
		},
	}

	runner := &runtimeRunner{fallback: mock}
	patErr := &apperrors.PATError{RawJSON: makePATErrorJSON("flow-rejected", "test-client-id")}

	ctx := context.Background()
	var buf bytes.Buffer
	_, err := handlePatAuthCheck(ctx, runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "test_tool",
	}, patErr, configDir, &buf)

	if err == nil {
		t.Fatal("expected error for REJECTED")
	}
	if !strings.Contains(err.Error(), "用户已拒绝授权") {
		t.Errorf("expected rejection error, got: %v", err)
	}
}

func TestHandlePatAuthCheck_EmptyFlowID_FallsBackToPATError(t *testing.T) {
	// No poll server needed — empty flowId means no polling, return PATError directly.
	tmpDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", tmpDir)

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner should not be called when flowId is empty")
			return executor.Result{}, nil
		},
	}

	runner := &runtimeRunner{fallback: mock}
	patErr := &apperrors.PATError{RawJSON: makePATErrorJSON("", "test-client-id")}

	ctx := context.Background()
	var buf bytes.Buffer
	_, err := handlePatAuthCheck(ctx, runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "test_tool",
	}, patErr, tmpDir, &buf)

	if err == nil {
		t.Fatal("expected PATError when flowId is empty")
	}
	// Should return the original PATError.
	if _, ok := err.(*apperrors.PATError); !ok {
		t.Errorf("expected *PATError, got %T: %v", err, err)
	}
}

func TestHandlePatAuthCheck_OpensOpaqueURIWithoutRebuild(t *testing.T) {
	server, configDir := setupHandlePATServer(t, "APPROVED", "test-auth-code")
	defer server.Close()

	rawURI := "https://open-dev.dingtalk.com/fe/old?hash=%23%2FpersonalAuthorization%3FflowId%3D50dff7654b7444e88ced7489b07cce8d%26userCode%3DQ8RY-X6E9#/personalAuthorization?flowId=50dff7654b7444e88ced7489b07cce8d&userCode=Q8RY-X6E9"
	var opened string
	origOpenBrowser := openBrowserFunc
	openBrowserFunc = func(rawURL string) error {
		opened = rawURL
		return nil
	}
	t.Cleanup(func() { openBrowserFunc = origOpenBrowser })

	var retryCalled bool
	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			retryCalled = true
			return executor.Result{Response: map[string]any{"ok": true}}, nil
		},
	}

	runner := &runtimeRunner{fallback: mock}
	patErr := &apperrors.PATError{RawJSON: makePATErrorJSONWithURI("flow-opaque", "test-client-id", rawURI)}

	var buf bytes.Buffer
	_, err := handlePatAuthCheck(context.Background(), runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "test_tool",
	}, patErr, configDir, &buf)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retryCalled {
		t.Fatal("expected retry to run after approved PAT flow")
	}
	if opened != rawURI {
		t.Fatalf("opened url = %q, want verbatim %q", opened, rawURI)
	}
}
