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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
)

func newDeviceFlowTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func writeServiceResult(w http.ResponseWriter, success bool, result any, errCode, errMsg string) {
	payload := map[string]any{
		"success":   success,
		"errorCode": errCode,
		"errorMsg":  errMsg,
	}
	if result != nil {
		raw, _ := json.Marshal(result)
		payload["result"] = json.RawMessage(raw)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func TestRequestDeviceCodeSuccess(t *testing.T) {
	t.Parallel()

	// Set a test client ID
	SetClientID("test-client-id")
	t.Cleanup(func() { SetClientID("") })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if r.FormValue("client_id") == "" {
			t.Fatal("client_id should not be empty")
		}
		writeServiceResult(w, true, DeviceAuthResponse{
			DeviceCode:              "device-code-123",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://example.com/device/verify",
			VerificationURIComplete: "https://example.com/device/verify?user_code=ABCD-EFGH",
			ExpiresIn:               900,
			Interval:                1,
		}, "", "")
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
	provider.Output = io.Discard
	provider.SetBaseURL(server.URL)

	resp, err := provider.requestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("requestDeviceCode() error = %v", err)
	}
	if resp.DeviceCode != "device-code-123" || resp.UserCode != "ABCD-EFGH" {
		t.Fatalf("device auth response = %#v, want populated device/user code", resp)
	}
}

func TestWaitForAuthorizationSucceedsAfterPending(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// New terminal API uses GET method
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "flowId=") {
			t.Fatal("flowId query parameter should be present")
		}
		if calls.Add(1) <= 2 {
			// Return PENDING status
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    map[string]string{"status": "PENDING"},
			})
			return
		}
		// Return APPROVED status with authCode
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]string{
				"status":   "APPROVED",
				"authCode": "final-auth-code",
			},
		})
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
	provider.Output = io.Discard
	provider.SetTerminalBaseURL(server.URL)

	resp, err := provider.waitForAuthorization(context.Background(), &DeviceAuthResponse{
		FlowID:    "test-flow-id",
		ExpiresIn: 10,
		Interval:  1,
	})
	if err != nil {
		t.Fatalf("waitForAuthorization() error = %v", err)
	}
	if resp.AuthCode != "final-auth-code" {
		t.Fatalf("auth code = %q, want final-auth-code", resp.AuthCode)
	}
	if calls.Load() != 3 {
		t.Fatalf("poll calls = %d, want 3", calls.Load())
	}
}

func TestWaitForAuthorizationAcceptsResultEnvelope(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if calls.Add(1) <= 2 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  map[string]string{"status": "PENDING"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]string{
				"status":   "APPROVED",
				"authCode": "final-auth-code",
			},
		})
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
	provider.Output = io.Discard
	provider.SetTerminalBaseURL(server.URL)

	resp, err := provider.waitForAuthorization(context.Background(), &DeviceAuthResponse{
		FlowID:    "test-flow-id-result",
		ExpiresIn: 10,
		Interval:  1,
	})
	if err != nil {
		t.Fatalf("waitForAuthorization() error = %v", err)
	}
	if resp.AuthCode != "final-auth-code" {
		t.Fatalf("auth code = %q, want final-auth-code", resp.AuthCode)
	}
	if calls.Load() != 3 {
		t.Fatalf("poll calls = %d, want 3", calls.Load())
	}
}

func TestDevicePollResponseEffectiveData_FallsBackToResultEnvelope(t *testing.T) {
	t.Parallel()

	resp := DevicePollResponse{
		Success: true,
		Result: DevicePollData{
			Status:   "APPROVED",
			AuthCode: "auth-from-result",
			FlowID:   "flow-from-result",
		},
	}

	effective := resp.EffectiveData()
	if effective.Status != "APPROVED" {
		t.Fatalf("effective.Status = %q, want APPROVED", effective.Status)
	}
	if effective.AuthCode != "auth-from-result" {
		t.Fatalf("effective.AuthCode = %q, want auth-from-result", effective.AuthCode)
	}
	if effective.FlowID != "flow-from-result" {
		t.Fatalf("effective.FlowID = %q, want flow-from-result", effective.FlowID)
	}
}

func TestDevicePollResponseEffectiveData_DataEnvelopeWinsAsWholePayload(t *testing.T) {
	t.Parallel()

	resp := DevicePollResponse{
		Success: true,
		Data: DevicePollData{
			Status: "PENDING",
		},
		Result: DevicePollData{
			Status:   "APPROVED",
			AuthCode: "auth-from-result",
			FlowID:   "flow-from-result",
		},
	}

	effective := resp.EffectiveData()
	if effective.Status != "PENDING" {
		t.Fatalf("effective.Status = %q, want PENDING", effective.Status)
	}
	if effective.AuthCode != "" {
		t.Fatalf("effective.AuthCode = %q, want empty because Data envelope wins as a whole", effective.AuthCode)
	}
	if effective.FlowID != "" {
		t.Fatalf("effective.FlowID = %q, want empty because Data envelope wins as a whole", effective.FlowID)
	}
}
func TestWaitForAuthorizationFallsBackToDeviceCodeWhenFlowIDMissing(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.FormValue("device_code"); got != "legacy-device-code" {
			t.Fatalf("device_code = %q, want legacy-device-code", got)
		}
		if calls.Add(1) <= 2 {
			writeServiceResult(w, true, DeviceTokenResponse{Error: "authorization_pending"}, "", "")
			return
		}
		writeServiceResult(w, true, DeviceTokenResponse{AuthCode: "legacy-auth-code"}, "", "")
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
	var output bytes.Buffer
	provider.Output = &output
	provider.SetBaseURL(server.URL)

	resp, err := provider.waitForAuthorization(context.Background(), &DeviceAuthResponse{
		DeviceCode: "legacy-device-code",
		ExpiresIn:  10,
		Interval:   1,
	})
	if err != nil {
		t.Fatalf("waitForAuthorization() error = %v", err)
	}
	if resp.AuthCode != "legacy-auth-code" {
		t.Fatalf("auth code = %q, want legacy-auth-code", resp.AuthCode)
	}
	if calls.Load() != 3 {
		t.Fatalf("poll calls = %d, want 3", calls.Load())
	}
	if !strings.Contains(output.String(), i18n.T("等待用户授权...")) {
		t.Fatalf("expected device-code path to emit pending output, got %q", output.String())
	}
	if !strings.Contains(output.String(), i18n.T("授权成功!")) {
		t.Fatalf("expected device-code path to emit success output, got %q", output.String())
	}
}

func TestWaitForAuthorizationHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// New terminal API uses GET method
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    map[string]string{"status": "PENDING"},
		})
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
	provider.Output = io.Discard
	provider.SetTerminalBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	if _, err := provider.waitForAuthorization(ctx, &DeviceAuthResponse{
		FlowID:    "test-flow-id-2",
		ExpiresIn: 60,
		Interval:  1,
	}); err == nil {
		t.Fatal("waitForAuthorization() error = nil, want context cancellation")
	}
}

func TestWaitForAuthorizationByDeviceCodeHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeServiceResult(w, true, DeviceTokenResponse{Error: "authorization_pending"}, "", "")
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
	provider.Output = io.Discard
	provider.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	_, err := provider.waitForAuthorization(ctx, &DeviceAuthResponse{
		DeviceCode: "legacy-device-code",
		ExpiresIn:  60,
		Interval:   1,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitForAuthorization() error = %v, want context deadline exceeded", err)
	}
}

func TestWaitForAuthorizationByDeviceCodeErrorStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeout    time.Duration
		responses  []DeviceTokenResponse
		wantErr    string
		wantErrIs  error
		wantOutput string
	}{
		{
			name:       "slow_down_then_context_cancelled",
			timeout:    1500 * time.Millisecond,
			responses:  []DeviceTokenResponse{{Error: "slow_down"}},
			wantErrIs:  context.DeadlineExceeded,
			wantOutput: fmt.Sprintf(i18n.T("轮询过快，间隔增加至 %ds"), 6),
		},
		{
			name:       "access_denied",
			timeout:    5 * time.Second,
			responses:  []DeviceTokenResponse{{Error: "access_denied"}},
			wantErr:    i18n.T("用户拒绝了授权请求"),
			wantOutput: fmt.Sprintf(i18n.T("[%d] 轮询中... (%ds)"), 1, 1),
		},
		{
			name:       "expired_token",
			timeout:    5 * time.Second,
			responses:  []DeviceTokenResponse{{Error: "expired_token"}},
			wantErr:    i18n.T("设备授权码已过期"),
			wantOutput: fmt.Sprintf(i18n.T("[%d] 轮询中... (%ds)"), 1, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := int(calls.Add(1)) - 1
				if idx >= len(tt.responses) {
					idx = len(tt.responses) - 1
				}
				writeServiceResult(w, true, tt.responses[idx], "", "")
			}))
			defer server.Close()

			provider := NewDeviceFlowProvider(t.TempDir(), newDeviceFlowTestLogger())
			var output bytes.Buffer
			provider.Output = &output
			provider.SetBaseURL(server.URL)

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			_, err := provider.waitForAuthorization(ctx, &DeviceAuthResponse{
				DeviceCode: "legacy-device-code",
				ExpiresIn:  60,
				Interval:   1,
			})

			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("waitForAuthorization() error = %v, want %v", err, tt.wantErrIs)
				}
			} else if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("waitForAuthorization() error = %v, want %q", err, tt.wantErr)
			}

			if !strings.Contains(output.String(), tt.wantOutput) {
				t.Fatalf("expected output to contain %q, got %q", tt.wantOutput, output.String())
			}
		})
	}
}
