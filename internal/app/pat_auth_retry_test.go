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

	// Per docs/pat/contract.md §2 the payload is single-line; assert by parsing the JSON
	// rather than by matching pretty-printed substrings.
	output := buf.String()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("PrintPatAuthJSON must emit directly-parsable JSON: %v\nraw=%s", err, output)
	}
	if code, _ := parsed["code"].(string); code != "PAT_SCOPE_AUTH_REQUIRED" {
		t.Errorf("code = %q, want PAT_SCOPE_AUTH_REQUIRED", code)
	}
	data, _ := parsed["data"].(map[string]any)
	if data == nil {
		t.Fatalf("expected data object, got: %s", output)
	}
	if got, _ := data["missingScope"].(string); got != "mail:send" {
		t.Errorf("missingScope = %q, want mail:send", got)
	}
	if _, ok := data["hostControl"]; !ok {
		t.Errorf("expected data.hostControl, got: %s", output)
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
			URI:      "", // empty to avoid opening browser in test
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

func TestHandlePatAuthCheck_HostControlledFlowIDPassthrough(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", tmpDir)
	// Host-owned decision: driven ONLY by DINGTALK_DWS_AGENTCODE (see docs/pat/contract.md §7).
	// DINGTALK_AGENT is set to demonstrate it does NOT leak into
	// hostControl.clawType — the open-source build pins that to the
	// literal edition.DefaultOSSClawType value ("openClaw").
	t.Setenv(authpkg.AgentCodeEnv, "agt-sales")
	t.Setenv("DINGTALK_AGENT", "sales-copilot")

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner should not be called in host-controlled PAT mode")
			return executor.Result{}, nil
		},
	}

	runner := &runtimeRunner{fallback: mock}
	patErr := &apperrors.PATError{RawJSON: makePATErrorJSON("flow-host", "test-client-id")}

	ctx := context.Background()
	var buf bytes.Buffer
	_, err := handlePatAuthCheck(ctx, runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "test_tool",
	}, patErr, tmpDir, &buf)

	if err == nil {
		t.Fatal("expected PATError in host-controlled mode")
	}
	patOut, ok := err.(*apperrors.PATError)
	if !ok {
		t.Fatalf("expected *PATError, got %T: %v", err, err)
	}
	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("expected no human-readable output in host mode, got %q", got)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(patOut.RawJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal(host PAT payload) error = %v\nraw=%s", err, patOut.RawJSON)
	}

	data, _ := payload["data"].(map[string]any)
	if got, _ := data["flowId"].(string); got != "flow-host" {
		t.Fatalf("data.flowId = %q, want flow-host", got)
	}
	hostControl, _ := data["hostControl"].(map[string]any)
	if got, _ := hostControl["clawType"].(string); got != "openClaw" {
		t.Fatalf("hostControl.clawType = %q, want openClaw (hard-wired by open-source edition)", got)
	}
	if _, ok := data["callbacks"]; ok {
		t.Fatalf("unexpected callbacks contract in host-controlled PAT payload: %#v", data["callbacks"])
	}
	if _, ok := payload["_meta"]; ok {
		t.Fatalf("unexpected _meta contract in host-controlled PAT payload: %#v", payload["_meta"])
	}
	if strings.Contains(patOut.RawJSON, `"pat","callback"`) {
		t.Fatalf("host PAT payload should not advertise dws pat callback argv: %s", patOut.RawJSON)
	}
}

func TestHandlePatAuthCheck_HostControlledEmptyFlowID_StillReturnsContract(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", tmpDir)
	t.Setenv(authpkg.AgentCodeEnv, "agt-support")
	t.Setenv("DINGTALK_AGENT", "customer-support")

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner should not be called in host-controlled PAT mode")
			return executor.Result{}, nil
		},
	}

	runner := &runtimeRunner{fallback: mock}
	patErr := &apperrors.PATError{RawJSON: makePATErrorJSON("", "test-client-id")}

	var buf bytes.Buffer
	_, err := handlePatAuthCheck(context.Background(), runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "test_tool",
	}, patErr, tmpDir, &buf)

	if err == nil {
		t.Fatal("expected PATError in host-controlled mode")
	}
	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("expected no human-readable output in host mode, got %q", got)
	}
	patOut, ok := err.(*apperrors.PATError)
	if !ok {
		t.Fatalf("expected *PATError, got %T: %v", err, err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(patOut.RawJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal(host PAT payload) error = %v\nraw=%s", err, patOut.RawJSON)
	}
	data, _ := payload["data"].(map[string]any)
	if _, ok := data["callbacks"]; ok {
		t.Fatalf("unexpected callbacks contract when flowId is absent: %#v", data["callbacks"])
	}
	if _, ok := payload["_meta"]; ok {
		t.Fatalf("unexpected _meta contract when flowId is absent: %#v", payload["_meta"])
	}
	if strings.Contains(patOut.RawJSON, `"pat","callback"`) {
		t.Fatalf("host PAT payload should not advertise dws pat callback argv: %s", patOut.RawJSON)
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
	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("expected no human-readable output for raw PAT passthrough, got %q", got)
	}
}

// TestEnrichPATErrorForHostControl_SingleLineOutput locks in the docs/pat/contract.md §2 /
// docs/pat/contract.md §2 wire invariant: the enriched host-controlled
// PAT payload must be single-line (no embedded newlines, no indentation),
// so stderr-line-scanning hosts stay correct. Regression guard against
// accidental reintroduction of json.MarshalIndent.
func TestEnrichPATErrorForHostControl_SingleLineOutput(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "agt-sales")
	t.Setenv("DINGTALK_AGENT", "sales-copilot")

	raw := `{"success":false,"code":"PAT_LOW_RISK_NO_PERMISSION","data":{"flowId":"flow-1","desc":"授权"}}`
	out := enrichPATErrorForHostControl(raw, "flow-1")

	if strings.Contains(out, "\n") {
		t.Fatalf("enrichPATErrorForHostControl output must be single-line, got embedded newline:\n%s", out)
	}
	if strings.HasPrefix(out, " ") || strings.HasPrefix(out, "\t") {
		t.Fatalf("enrichPATErrorForHostControl output must not be indented, got leading whitespace: %q", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("single-line output must round-trip via json.Unmarshal: %v\nraw=%s", err, out)
	}
	data, _ := parsed["data"].(map[string]any)
	if _, ok := data["hostControl"]; !ok {
		t.Fatalf("expected data.hostControl injection, got: %s", out)
	}
}

// TestBuildPATScopeHostJSON_SingleLineOutput mirrors the above regression
// for the scope-error branch (PAT_SCOPE_AUTH_REQUIRED emission).
func TestBuildPATScopeHostJSON_SingleLineOutput(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "agt-support")
	t.Setenv("DINGTALK_AGENT", "customer-support")

	scopeErr := &PatScopeError{
		OriginalError: "missing required scope(s): mail:send",
		Identity:      "user",
		ErrorType:     "missing_scope",
		Message:       "missing required scope(s): mail:send",
		Hint:          "run `dws auth login --scope \"mail:send\"` to authorize",
		MissingScope:  "mail:send",
	}
	out := buildPATScopeHostJSON(scopeErr)

	if strings.Contains(out, "\n") {
		t.Fatalf("buildPATScopeHostJSON output must be single-line, got embedded newline:\n%s", out)
	}
	if strings.HasPrefix(out, " ") || strings.HasPrefix(out, "\t") {
		t.Fatalf("buildPATScopeHostJSON output must not be indented, got leading whitespace: %q", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("single-line output must round-trip via json.Unmarshal: %v\nraw=%s", err, out)
	}
	if code, _ := parsed["code"].(string); code != "PAT_SCOPE_AUTH_REQUIRED" {
		t.Errorf("code = %q, want PAT_SCOPE_AUTH_REQUIRED", code)
	}
}

func TestRetryWithPatAuthRetry_HostControlledReturnsJSON(t *testing.T) {
	t.Setenv(authpkg.AgentCodeEnv, "agt-support")
	t.Setenv("DINGTALK_AGENT", "customer-support")

	scopeErr := &PatScopeError{
		OriginalError: "missing required scope(s): mail:send",
		Identity:      "user",
		ErrorType:     "missing_scope",
		Message:       "missing required scope(s): mail:send",
		Hint:          "run `dws auth login --scope \"mail:send\"` to authorize the missing scope",
		MissingScope:  "mail:send",
	}

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner should not be called in host-controlled scope mode")
			return executor.Result{}, nil
		},
	}

	var buf bytes.Buffer
	_, err := retryWithPatAuthRetry(context.Background(), mock, executor.Invocation{}, scopeErr, t.TempDir(), &buf)
	if err == nil {
		t.Fatal("expected PATError")
	}
	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("expected no human-readable output, got %q", got)
	}
	patErr, ok := err.(*apperrors.PATError)
	if !ok {
		t.Fatalf("expected *PATError, got %T: %v", err, err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(patErr.RawJSON), &payload); err != nil {
		t.Fatalf("json.Unmarshal(scope host payload) error = %v\nraw=%s", err, patErr.RawJSON)
	}
	if got, _ := payload["code"].(string); got != "PAT_SCOPE_AUTH_REQUIRED" {
		t.Fatalf("code = %q, want PAT_SCOPE_AUTH_REQUIRED", got)
	}
	data, _ := payload["data"].(map[string]any)
	if got, _ := data["missingScope"].(string); got != "mail:send" {
		t.Fatalf("missingScope = %q, want mail:send", got)
	}
	if _, ok := data["callbacks"]; ok {
		t.Fatalf("unexpected callbacks contract in PAT scope host payload: %#v", data["callbacks"])
	}
	if strings.Contains(patErr.RawJSON, `"pat","callback"`) {
		t.Fatalf("scope host payload should not advertise dws pat callback argv: %s", patErr.RawJSON)
	}
}
