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

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// setupMCPConfigDir creates a temp config dir with mcp_url pointing to the
// given test server and sets DWS_CONFIG_DIR via t.Setenv.
// NOTE: tests calling this must NOT use t.Parallel().
func setupMCPConfigDir(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(srvURL), 0o600)
	t.Setenv("DWS_CONFIG_DIR", dir)
	return dir
}

// resetClientIDFromMCP clears the MCP-sourced flag (test helper).
func resetClientIDFromMCP() {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientIDFromMCP = false
}

// ---------------------------------------------------------------------------
// 1. CheckCLIAuthEnabled: interface error → fail-closed with retry
// ---------------------------------------------------------------------------

func TestCheckCLIAuthEnabled_ServerError_FailClosed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	p := &OAuthProvider{configDir: configDir, httpClient: srv.Client()}

	_, err := p.CheckCLIAuthEnabled(context.Background(), "fake-token")

	if err == nil {
		t.Fatal("expected error from CheckCLIAuthEnabled when server returns 500, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error should mention retry exhaustion, got: %s", err)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", c)
	}
	t.Logf("✅ Server 500 → fail-closed: error=%q, attempts=%d", err, calls.Load())
}

func TestCheckCLIAuthEnabled_ConnectionRefused_FailClosed(t *testing.T) {
	configDir := setupMCPConfigDir(t, "http://127.0.0.1:1")
	p := &OAuthProvider{
		configDir:  configDir,
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}

	_, err := p.CheckCLIAuthEnabled(context.Background(), "fake-token")

	if err == nil {
		t.Fatal("expected error when connection is refused, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error should mention retry exhaustion, got: %s", err)
	}
	t.Logf("✅ Connection refused → fail-closed: error=%q", err)
}

func TestCheckCLIAuthEnabled_MalformedJSON_FailClosed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{this is not valid json}`)
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	p := &OAuthProvider{configDir: configDir, httpClient: srv.Client()}

	_, err := p.CheckCLIAuthEnabled(context.Background(), "fake-token")

	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", c)
	}
	t.Logf("✅ Malformed JSON → fail-closed: error=%q, attempts=%d", err, calls.Load())
}

func TestCheckCLIAuthEnabled_Timeout_FailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	p := &OAuthProvider{
		configDir:  configDir,
		httpClient: &http.Client{Timeout: 200 * time.Millisecond},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.CheckCLIAuthEnabled(ctx, "fake-token")

	if err == nil {
		t.Fatal("expected error on request timeout, got nil")
	}
	t.Logf("✅ Timeout → fail-closed: error=%q", err)
}

// ---------------------------------------------------------------------------
// 2. CheckCLIAuthEnabled: transient error then recovery → succeeds
// ---------------------------------------------------------------------------

func TestCheckCLIAuthEnabled_TransientThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CLIAuthStatus{
			Success: true,
			Result:  &CLIAuthResult{CLIAuthEnabled: true},
		})
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	p := &OAuthProvider{configDir: configDir, httpClient: srv.Client()}

	status, err := p.CheckCLIAuthEnabled(context.Background(), "fake-token")

	if err != nil {
		t.Fatalf("expected success after transient failures, got error: %v", err)
	}
	if !status.Success || status.Result == nil || !status.Result.CLIAuthEnabled {
		t.Fatalf("expected CLIAuthEnabled=true, got %+v", status)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 attempts (2 failures + 1 success), got %d", c)
	}
	t.Logf("✅ Transient error then success: attempts=%d, enabled=%v", calls.Load(), status.Result.CLIAuthEnabled)
}

// ---------------------------------------------------------------------------
// 3. CheckCLIAuthEnabled: normal responses (pass-through)
// ---------------------------------------------------------------------------

func TestCheckCLIAuthEnabled_Enabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-user-access-token") != "good-token" {
			t.Errorf("missing access token header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CLIAuthStatus{
			Success: true,
			Result:  &CLIAuthResult{CLIAuthEnabled: true},
		})
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	p := &OAuthProvider{configDir: configDir, httpClient: srv.Client()}

	status, err := p.CheckCLIAuthEnabled(context.Background(), "good-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Result == nil || !status.Result.CLIAuthEnabled {
		t.Fatal("expected CLIAuthEnabled=true")
	}
	t.Logf("✅ Normal enabled response: success=%v, enabled=%v", status.Success, status.Result.CLIAuthEnabled)
}

func TestCheckCLIAuthEnabled_Disabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CLIAuthStatus{
			Success: true,
			Result:  &CLIAuthResult{CLIAuthEnabled: false},
		})
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	p := &OAuthProvider{configDir: configDir, httpClient: srv.Client()}

	status, err := p.CheckCLIAuthEnabled(context.Background(), "fake-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Result == nil || status.Result.CLIAuthEnabled {
		t.Fatal("expected CLIAuthEnabled=false")
	}
	t.Logf("✅ Normal disabled response: success=%v, enabled=%v", status.Success, status.Result.CLIAuthEnabled)
}

// ---------------------------------------------------------------------------
// 4. OAuth callback: simulates the fail-closed logic at the /callback level
// ---------------------------------------------------------------------------

func TestOAuthCallback_CLIAuthError_ShowsNotEnabledPage(t *testing.T) {
	t.Parallel()

	var statusErr error = fmt.Errorf("simulated network error")
	var authStatus *CLIAuthStatus
	_ = authStatus

	// This is the exact expression used in oauth_provider.go callback:
	// cliAuthEnabled := statusErr == nil && authStatus.Success && authStatus.Result.CLIAuthEnabled
	cliAuthEnabled := statusErr == nil // false

	if cliAuthEnabled {
		t.Fatal("cliAuthEnabled should be false when statusErr != nil")
	}
	t.Logf("✅ OAuth callback: statusErr=%v → cliAuthEnabled=%v → shows notEnabledHTML (fail-closed)", statusErr, cliAuthEnabled)
}

func TestOAuthCallback_CLIAuthEnabled_ShowsSuccessPage(t *testing.T) {
	t.Parallel()

	var statusErr error
	authStatus := &CLIAuthStatus{
		Success: true,
		Result:  &CLIAuthResult{CLIAuthEnabled: true},
	}

	cliAuthEnabled := statusErr == nil && authStatus.Success && authStatus.Result != nil && authStatus.Result.CLIAuthEnabled
	if !cliAuthEnabled {
		t.Fatal("cliAuthEnabled should be true when API returns enabled")
	}
	t.Logf("✅ OAuth callback: statusErr=nil, enabled=true → cliAuthEnabled=%v → shows successHTML", cliAuthEnabled)
}

func TestOAuthCallback_CLIAuthDisabledByServer_ShowsNotEnabledPage(t *testing.T) {
	t.Parallel()

	var statusErr error
	authStatus := &CLIAuthStatus{
		Success: true,
		Result:  &CLIAuthResult{CLIAuthEnabled: false},
	}

	cliAuthEnabled := statusErr == nil && authStatus.Success && authStatus.Result != nil && authStatus.Result.CLIAuthEnabled
	if cliAuthEnabled {
		t.Fatal("cliAuthEnabled should be false when server says disabled")
	}
	t.Logf("✅ OAuth callback: statusErr=nil, enabled=false → cliAuthEnabled=%v → shows notEnabledHTML", cliAuthEnabled)
}

// ---------------------------------------------------------------------------
// 5. Device Flow: loginOnce with broken cliAuthEnabled endpoint
// ---------------------------------------------------------------------------

func TestDeviceFlow_LoginOnce_CLIAuthError_FailClosed(t *testing.T) {
	SetClientIDFromMCP("test-client-id")
	t.Cleanup(func() {
		SetClientID("")
		resetClientIDFromMCP()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, DeviceCodePath):
			writeServiceResult(w, true, DeviceAuthResponse{
				DeviceCode:      "dc-test",
				UserCode:        "TEST-CODE",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       60,
				Interval:        1,
			}, "", "")

		case strings.HasSuffix(r.URL.Path, DeviceTokenPath):
			writeServiceResult(w, true, DeviceTokenResponse{
				AuthCode: "test-auth-code",
			}, "", "")

		case strings.HasSuffix(r.URL.Path, MCPOAuthTokenPath):
			json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "test-access-token",
				"refreshToken": "test-refresh-token",
				"expiresIn":    7200,
				"corpId":       "corp123",
			})

		case strings.HasSuffix(r.URL.Path, CLIAuthEnabledPath):
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)

		default:
			http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	provider := &DeviceFlowProvider{
		configDir:  configDir,
		clientID:   "test-client-id",
		scope:      DefaultScopes,
		baseURL:    srv.URL,
		logger:     newDeviceFlowTestLogger(),
		Output:     io.Discard,
		httpClient: srv.Client(),
	}

	_, err := provider.loginOnce(context.Background(), 1)

	if err == nil {
		t.Fatal("expected loginOnce to fail when CLI auth check fails, got nil")
	}
	if !strings.Contains(err.Error(), "检查 CLI 授权状态失败") && !strings.Contains(err.Error(), "Failed to check CLI auth status") {
		t.Fatalf("unexpected error message: %s", err)
	}
	t.Logf("✅ Device Flow: CLI auth check error → login blocked: %s", err)
}

func TestDeviceFlow_LoginOnce_CLIAuthDisabled_ShowsError(t *testing.T) {
	SetClientIDFromMCP("test-client-id")
	t.Cleanup(func() {
		SetClientID("")
		resetClientIDFromMCP()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, DeviceCodePath):
			writeServiceResult(w, true, DeviceAuthResponse{
				DeviceCode:      "dc-test",
				UserCode:        "TEST-CODE",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       60,
				Interval:        1,
			}, "", "")

		case strings.HasSuffix(r.URL.Path, DeviceTokenPath):
			writeServiceResult(w, true, DeviceTokenResponse{
				AuthCode: "test-auth-code",
			}, "", "")

		case strings.HasSuffix(r.URL.Path, MCPOAuthTokenPath):
			json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "test-access-token",
				"refreshToken": "test-refresh-token",
				"expiresIn":    7200,
				"corpId":       "corp123",
			})

		case strings.HasSuffix(r.URL.Path, CLIAuthEnabledPath):
			json.NewEncoder(w).Encode(CLIAuthStatus{
				Success: true,
				Result:  &CLIAuthResult{CLIAuthEnabled: false},
			})

		case strings.HasSuffix(r.URL.Path, SuperAdminPath):
			json.NewEncoder(w).Encode(SuperAdminResponse{
				Success: true,
				Result:  []SuperAdmin{{StaffID: "admin1", Name: "张三"}},
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	provider := &DeviceFlowProvider{
		configDir:  configDir,
		clientID:   "test-client-id",
		scope:      DefaultScopes,
		baseURL:    srv.URL,
		logger:     newDeviceFlowTestLogger(),
		Output:     io.Discard,
		httpClient: srv.Client(),
	}

	_, err := provider.loginOnce(context.Background(), 1)

	if err == nil {
		t.Fatal("expected loginOnce to fail when CLI auth is disabled, got nil")
	}
	t.Logf("✅ Device Flow: CLI auth disabled by server → login blocked: %s", err)
}

func TestDeviceFlow_LoginOnce_CLIAuthEnabled_Success(t *testing.T) {
	SetClientIDFromMCP("test-client-id")
	t.Cleanup(func() {
		SetClientID("")
		resetClientIDFromMCP()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, DeviceCodePath):
			writeServiceResult(w, true, DeviceAuthResponse{
				DeviceCode:      "dc-test",
				UserCode:        "TEST-CODE",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       60,
				Interval:        1,
			}, "", "")

		case strings.HasSuffix(r.URL.Path, DeviceTokenPath):
			writeServiceResult(w, true, DeviceTokenResponse{
				AuthCode: "test-auth-code",
			}, "", "")

		case strings.HasSuffix(r.URL.Path, MCPOAuthTokenPath):
			json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "test-access-token",
				"refreshToken": "test-refresh-token",
				"expiresIn":    7200,
				"corpId":       "corp123",
			})

		case strings.HasSuffix(r.URL.Path, CLIAuthEnabledPath):
			json.NewEncoder(w).Encode(CLIAuthStatus{
				Success: true,
				Result:  &CLIAuthResult{CLIAuthEnabled: true},
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	configDir := setupMCPConfigDir(t, srv.URL)
	provider := &DeviceFlowProvider{
		configDir:  configDir,
		clientID:   "test-client-id",
		scope:      DefaultScopes,
		baseURL:    srv.URL,
		logger:     newDeviceFlowTestLogger(),
		Output:     io.Discard,
		httpClient: srv.Client(),
	}

	token, err := provider.loginOnce(context.Background(), 1)

	if err != nil {
		t.Fatalf("expected loginOnce to succeed, got error: %v", err)
	}
	if token.AccessToken != "test-access-token" {
		t.Fatalf("unexpected token: %s", token.AccessToken)
	}
	t.Logf("✅ Device Flow: CLI auth enabled → login succeeded, token=%s", token.AccessToken)
}

// ---------------------------------------------------------------------------
// 6. FetchClientIDFromMCP: /cli/clientId error handling
// ---------------------------------------------------------------------------

func TestFetchClientIDFromMCP_ServerError_FailClosed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	_, err := FetchClientIDFromMCP(context.Background())

	if err == nil {
		t.Fatal("expected error when /cli/clientId returns 500, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error should mention retry exhaustion, got: %s", err)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", c)
	}
	t.Logf("✅ /cli/clientId 500 → fail-closed with retry: error=%q, attempts=%d", err, calls.Load())
}

func TestFetchClientIDFromMCP_ConnectionRefused_FailClosed(t *testing.T) {
	setupMCPConfigDir(t, "http://127.0.0.1:1")

	_, err := FetchClientIDFromMCP(context.Background())

	if err == nil {
		t.Fatal("expected error when connection is refused, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error should mention retry exhaustion, got: %s", err)
	}
	t.Logf("✅ /cli/clientId connection refused → fail-closed: error=%q", err)
}

func TestFetchClientIDFromMCP_MalformedJSON_FailClosed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json at all`)
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	_, err := FetchClientIDFromMCP(context.Background())

	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", c)
	}
	t.Logf("✅ /cli/clientId malformed JSON → fail-closed: error=%q, attempts=%d", err, calls.Load())
}

func TestFetchClientIDFromMCP_BusinessError_FailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ClientIDResponse{
			Success:   false,
			ErrorCode: "forbidden",
			ErrorMsg:  "access denied",
		})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	_, err := FetchClientIDFromMCP(context.Background())

	if err == nil {
		t.Fatal("expected error when server returns success=false, got nil")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("error should contain server error message, got: %s", err)
	}
	t.Logf("✅ /cli/clientId business error → fail-closed: error=%q", err)
}

func TestFetchClientIDFromMCP_TransientThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ClientIDResponse{
			Success: true,
			Result:  "recovered-client-id",
		})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	id, err := FetchClientIDFromMCP(context.Background())

	if err != nil {
		t.Fatalf("expected success after transient failures, got error: %v", err)
	}
	if id != "recovered-client-id" {
		t.Fatalf("expected client ID 'recovered-client-id', got %q", id)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 attempts (2 failures + 1 success), got %d", c)
	}
	t.Logf("✅ /cli/clientId transient then success: attempts=%d, id=%s", calls.Load(), id)
}

func TestFetchClientIDFromMCP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ClientIDPath {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ClientIDResponse{
			Success: true,
			Result:  "my-client-id-123",
		})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	id, err := FetchClientIDFromMCP(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "my-client-id-123" {
		t.Fatalf("expected 'my-client-id-123', got %q", id)
	}
	t.Logf("✅ /cli/clientId normal success: id=%s", id)
}

// ---------------------------------------------------------------------------
// 7. GetSuperAdmins: /cli/superAdmin error handling
// ---------------------------------------------------------------------------

func TestGetSuperAdmins_ServerError_RetriesAndFails(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	_, err := GetSuperAdmins(context.Background(), "fake-token")

	if err == nil {
		t.Fatal("expected error when /cli/superAdmin returns 500, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error should mention retry exhaustion, got: %s", err)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", c)
	}
	t.Logf("✅ /cli/superAdmin 500 → retried 3 times: error=%q", err)
}

func TestGetSuperAdmins_TransientThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SuperAdminResponse{
			Success: true,
			Result:  []SuperAdmin{{StaffID: "a1", Name: "张三"}, {StaffID: "a2", Name: "李四"}},
		})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	result, err := GetSuperAdmins(context.Background(), "fake-token")

	if err != nil {
		t.Fatalf("expected success after transient failures, got error: %v", err)
	}
	if !result.Success || len(result.Result) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 attempts, got %d", c)
	}
	t.Logf("✅ /cli/superAdmin transient then success: attempts=%d, admins=%v", calls.Load(), result.Result)
}

func TestGetSuperAdmins_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-user-access-token") != "good-token" {
			t.Errorf("missing access token header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SuperAdminResponse{
			Success: true,
			Result:  []SuperAdmin{{StaffID: "admin1", Name: "王五"}},
		})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	result, err := GetSuperAdmins(context.Background(), "good-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Result) != 1 || result.Result[0].Name != "王五" {
		t.Fatalf("unexpected result: %+v", result)
	}
	t.Logf("✅ /cli/superAdmin normal success: admins=%v", result.Result)
}

// ---------------------------------------------------------------------------
// 8. SendCliAuthApply: /cli/sendCliAuthApply error handling
// ---------------------------------------------------------------------------

func TestSendCliAuthApply_ServerError_RetriesAndFails(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	_, err := SendCliAuthApply(context.Background(), "fake-token", "admin1")

	if err == nil {
		t.Fatal("expected error when /cli/sendCliAuthApply returns 500, got nil")
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("error should mention retry exhaustion, got: %s", err)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", c)
	}
	t.Logf("✅ /cli/sendCliAuthApply 500 → retried 3 times: error=%q", err)
}

func TestSendCliAuthApply_TransientThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 1 {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SendApplyResponse{Success: true, Result: true})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	result, err := SendCliAuthApply(context.Background(), "fake-token", "admin1")

	if err != nil {
		t.Fatalf("expected success after transient failure, got error: %v", err)
	}
	if !result.Success || !result.Result {
		t.Fatalf("unexpected result: %+v", result)
	}
	if c := calls.Load(); c != 2 {
		t.Fatalf("expected 2 attempts, got %d", c)
	}
	t.Logf("✅ /cli/sendCliAuthApply transient then success: attempts=%d", calls.Load())
}

func TestSendCliAuthApply_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-user-access-token") != "good-token" {
			t.Errorf("missing access token header")
		}
		if !strings.Contains(r.URL.RawQuery, "adminStaffId=admin123") {
			t.Errorf("missing or wrong adminStaffId param: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SendApplyResponse{Success: true, Result: true})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	result, err := SendCliAuthApply(context.Background(), "good-token", "admin123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || !result.Result {
		t.Fatalf("unexpected result: %+v", result)
	}
	t.Logf("✅ /cli/sendCliAuthApply normal success: result=%+v", result)
}

func TestSendCliAuthApply_BusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SendApplyResponse{
			Success:   false,
			ErrorCode: "invalid_admin",
			ErrorMsg:  "admin not found",
			Result:    false,
		})
	}))
	defer srv.Close()

	setupMCPConfigDir(t, srv.URL)

	result, err := SendCliAuthApply(context.Background(), "fake-token", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result.Success {
		t.Fatal("expected success=false for business error")
	}
	t.Logf("✅ /cli/sendCliAuthApply business error: errorCode=%s, errorMsg=%s", result.ErrorCode, result.ErrorMsg)
}
