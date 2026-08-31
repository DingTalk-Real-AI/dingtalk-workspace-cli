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

package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewClient_DefaultBaseURL(t *testing.T) {
	c := NewClient("tok", "")
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("expected %q, got %q", DefaultBaseURL, c.BaseURL)
	}
}

func TestNewClient_CustomBaseURL(t *testing.T) {
	c := NewClient("tok", "https://custom.api.com/")
	if c.BaseURL != "https://custom.api.com" {
		t.Errorf("expected trailing slash stripped, got %q", c.BaseURL)
	}
}

func TestNormalisePath(t *testing.T) {
	tests := []struct {
		path, base, want string
	}{
		{"/v1.0/users", "", "https://api.dingtalk.com/v1.0/users"},
		{"v1.0/users", "", "https://api.dingtalk.com/v1.0/users"},
		{"https://api.dingtalk.com/v1.0/users", "", "https://api.dingtalk.com/v1.0/users"},
		{"/v1.0/users?foo=bar#frag", "", "https://api.dingtalk.com/v1.0/users"},
		{"/v1.0/users", "https://custom.example.com", "https://custom.example.com/v1.0/users"},
	}
	for _, tt := range tests {
		got := NormalisePath(tt.path, tt.base)
		if got != tt.want {
			t.Errorf("NormalisePath(%q, %q) = %q, want %q", tt.path, tt.base, got, tt.want)
		}
	}
}

func TestDo_Success(t *testing.T) {
	c := NewClient("test-token", "")
	c.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get(AuthHeader) != "test-token" {
			t.Errorf("expected auth header %q, got %q", "test-token", r.Header.Get(AuthHeader))
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		return jsonHTTPResponse(`{"name":"test"}`), nil
	})
	resp, err := c.Do(context.Background(), RawAPIRequest{
		Method: "GET",
		Path:   "/v1.0/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDo_PostWithBody(t *testing.T) {
	c := NewClient("tok", "")
	c.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type")
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["key"] != "value" {
			t.Errorf("expected body key=value, got %v", body)
		}
		return jsonHTTPResponse(`{"ok":true}`), nil
	})
	resp, err := c.Do(context.Background(), RawAPIRequest{
		Method: "POST",
		Path:   "/v1.0/test",
		Data:   map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDo_InvalidMethod(t *testing.T) {
	c := NewClient("tok", "")
	_, err := c.Do(context.Background(), RawAPIRequest{
		Method: "INVALID",
		Path:   "/test",
	})
	if err == nil {
		t.Error("expected error for invalid method")
	}
}

func TestDo_QueryParams(t *testing.T) {
	c := NewClient("tok", "")
	c.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("pageSize") != "10" {
			t.Errorf("expected pageSize=10, got %v", r.URL.Query())
		}
		return jsonHTTPResponse(`{}`), nil
	})
	_, err := c.Do(context.Background(), RawAPIRequest{
		Method: "GET",
		Path:   "/v1.0/test",
		Params: map[string]any{"pageSize": 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsLegacyAPI(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.dingtalk.com/v1.0/users", false},
		{"https://oapi.dingtalk.com/topapi/v2/user/get", true},
		{"https://OAPI.DINGTALK.COM/topapi/v2/user/get", true},
		{"https://custom.example.com/api", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsLegacyAPI(tt.url)
		if got != tt.want {
			t.Errorf("IsLegacyAPI(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestDo_LegacyAPI_TokenInQueryParam(t *testing.T) {
	c := NewClient("legacy-token", "")
	c.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// Legacy API: token should be in query param.
		if r.URL.Query().Get(LegacyAuthParam) != "legacy-token" {
			t.Errorf("expected access_token=legacy-token in query, got %v", r.URL.Query())
		}
		// Should NOT have the new-style auth header.
		if r.Header.Get(AuthHeader) != "" {
			t.Errorf("expected no auth header for legacy API, got %q", r.Header.Get(AuthHeader))
		}
		return jsonHTTPResponse(`{"errcode":0,"errmsg":"ok","result":{"userid":"user1"}}`), nil
	})

	resp, err := c.Do(context.Background(), RawAPIRequest{
		Method: "POST",
		Path:   "https://oapi.dingtalk.com/topapi/v2/user/get",
		Data:   map[string]string{"userid": "user1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNormalisePath_Legacy(t *testing.T) {
	tests := []struct {
		path, base, want string
	}{
		// Legacy full URL preserved.
		{"https://oapi.dingtalk.com/topapi/v2/user/get", "", "https://oapi.dingtalk.com/topapi/v2/user/get"},
		// Relative path with legacy base URL.
		{"/topapi/v2/user/get", LegacyBaseURL, "https://oapi.dingtalk.com/topapi/v2/user/get"},
		// Strip query from legacy URL.
		{"https://oapi.dingtalk.com/topapi/v2/user/get?access_token=xxx", "", "https://oapi.dingtalk.com/topapi/v2/user/get"},
	}
	for _, tt := range tests {
		got := NormalisePath(tt.path, tt.base)
		if got != tt.want {
			t.Errorf("NormalisePath(%q, %q) = %q, want %q", tt.path, tt.base, got, tt.want)
		}
	}
}

func TestResolvePageLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw, want int
	}{
		// 0 → unlimited → safety cap
		{0, MaxPageLimit},
		// normal usage
		{3, 3},
		// default
		{10, 10},
		// within cap
		{100, 100},
		// exactly cap
		{MaxPageLimit, MaxPageLimit},
		// exceeds cap
		{MaxPageLimit + 100, MaxPageLimit},
		// negative → default
		{-1, DefaultPageLimit},
		{-100, DefaultPageLimit},
	}
	for _, tt := range tests {
		got := resolvePageLimit(tt.raw)
		if got != tt.want {
			t.Errorf("resolvePageLimit(%d) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestPaginateAll_ProgressLog(t *testing.T) {
	callCount := 0
	c := NewClient("test-token", "")
	c.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		var body string
		if callCount >= 3 {
			body = `{"result":{"has_more":false,"items":[1,2]}}`
		} else {
			body = `{"result":{"has_more":true,"next_cursor":100,"items":[1]}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	var logBuf bytes.Buffer
	pages, err := c.PaginateAll(context.Background(), RawAPIRequest{
		Method: "GET",
		Path:   "/v1.0/test",
	}, PaginationOptions{
		PageLimit: 5,
		PageDelay: 0,
		LogWriter: &logBuf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 3 {
		t.Errorf("expected 3 pages, got %d", len(pages))
	}

	log := logBuf.String()
	if !strings.Contains(log, "第 1 页") || !strings.Contains(log, "第 2 页") || !strings.Contains(log, "第 3 页") {
		t.Errorf("expected progress log for each page, got: %s", log)
	}
	if !strings.Contains(log, "数据获取完成") {
		t.Errorf("expected completion message, got: %s", log)
	}
}
