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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mustJSONBody returns a *bytes.Buffer containing the JSON encoding of v, or fails the test.
func mustJSONBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("json encode: %v", err)
	}
	return &buf
}

func TestAppTokenData_IsTokenValid(t *testing.T) {
	tests := []struct {
		name string
		data *AppTokenData
		want bool
	}{
		{"nil data", nil, false},
		{"empty token", &AppTokenData{}, false},
		{"expired", &AppTokenData{
			AccessToken: "tok",
			ExpiresAt:   time.Now().Add(-1 * time.Minute),
		}, false},
		{"within buffer", &AppTokenData{
			AccessToken: "tok",
			ExpiresAt:   time.Now().Add(3 * time.Minute), // 3 min < 5 min buffer
		}, false},
		{"valid", &AppTokenData{
			AccessToken: "tok",
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.IsTokenValid(); got != tt.want {
				t.Errorf("IsTokenValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppTokenData_JSONRoundTrip(t *testing.T) {
	original := &AppTokenData{
		AccessToken: "app-tok-abc",
		ExpiresAt:   time.Now().Add(2 * time.Hour).Truncate(time.Second),
		ClientID:    "my-app-key",
		UpdatedAt:   time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AppTokenData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.AccessToken != original.AccessToken {
		t.Errorf("AccessToken = %q, want %q", decoded.AccessToken, original.AccessToken)
	}
	if decoded.ClientID != original.ClientID {
		t.Errorf("ClientID = %q, want %q", decoded.ClientID, original.ClientID)
	}
}

func TestFetchAppToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["appKey"] != "mykey" || body["appSecret"] != "mysecret" {
			t.Errorf("got body %v, want appKey=mykey, appSecret=mysecret", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{
			"accessToken": "app-tok-123",
			"expireIn":    7200,
		})
	}))
	defer srv.Close()

	body := mustJSONBody(t, map[string]string{
		"appKey":    "mykey",
		"appSecret": "mysecret",
	})
	resp, err := srv.Client().Post(srv.URL, "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken != "app-tok-123" {
		t.Errorf("got token %q, want app-tok-123", result.AccessToken)
	}
	if result.ExpireIn != 7200 {
		t.Errorf("got expireIn %d, want 7200", result.ExpireIn)
	}
}

func TestFetchAppToken_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{
			"accessToken": "",
			"expireIn":    7200,
		})
	}))
	defer srv.Close()

	body := mustJSONBody(t, map[string]string{
		"appKey":    "badkey",
		"appSecret": "badsecret",
	})
	resp, err := srv.Client().Post(srv.URL, "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken != "" {
		t.Errorf("expected empty accessToken, got %q", result.AccessToken)
	}
}

func TestAppTokenProvider_GetToken_MissingCredentials(t *testing.T) {
	provider := &AppTokenProvider{
		ConfigDir: t.TempDir(),
		AppKey:    "",
		AppSecret: "",
	}
	_, err := provider.GetToken(context.Background())
	if err == nil {
		t.Error("expected error for missing credentials")
	}
}

func TestTruncateStr(t *testing.T) {
	if got := truncateStr("hello", 10); got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
	if got := truncateStr("hello world", 5); got != "hello..." {
		t.Errorf("got %q, want hello...", got)
	}
}
