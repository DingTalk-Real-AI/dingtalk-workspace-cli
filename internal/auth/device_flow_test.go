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
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
