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
	"net/http"
	"net/http/httptest"
	"net/url"
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
	AllowedHosts["127.0.0.1"] = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(AuthHeader) != "test-token" {
			t.Errorf("expected auth header %q, got %q", "test-token", r.Header.Get(AuthHeader))
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"name": "test"})
	}))
	defer srv.Close()

	c := NewClient("test-token", srv.URL)
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
	AllowedHosts["127.0.0.1"] = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["key"] != "value" {
			t.Errorf("expected body key=value, got %v", body)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("tok", srv.URL)
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
	AllowedHosts["127.0.0.1"] = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageSize") != "10" {
			t.Errorf("expected pageSize=10, got %v", r.URL.Query())
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("tok", srv.URL)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Legacy API: token should be in query param.
		if r.URL.Query().Get(LegacyAuthParam) != "legacy-token" {
			t.Errorf("expected access_token=legacy-token in query, got %v", r.URL.Query())
		}
		// Should NOT have the new-style auth header.
		if r.Header.Get(AuthHeader) != "" {
			t.Errorf("expected no auth header for legacy API, got %q", r.Header.Get(AuthHeader))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"userid":"user1"}}`))
	}))
	defer srv.Close()

	// Use full URL with oapi.dingtalk.com in the path, but redirect to test server.
	// Since we can't DNS-resolve oapi.dingtalk.com, we use the test server URL
	// and pass the full oapi URL as Path so that NormalisePath preserves it.
	// Then we override the resolved URL in the client to point to our test server.
	//
	// Best approach: directly verify that buildURL + IsLegacyAPI routing works
	// by testing buildURL output and calling Do with a custom transport that
	// redirects oapi.dingtalk.com to our test server.
	c := NewClient("legacy-token", "")
	// Replace the transport to redirect oapi.dingtalk.com to test server.
	c.HTTPClient.Transport = &legacyTestTransport{targetURL: srv.URL}

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

// legacyTestTransport redirects requests from oapi.dingtalk.com to a local test server.
type legacyTestTransport struct {
	targetURL string
}

func (t *legacyTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the host to point to our test server, preserving path and query.
	newURL := t.targetURL + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	parsed, _ := url.Parse(newURL)
	req.URL = parsed
	req.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(req)
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
	AllowedHosts["127.0.0.1"] = true
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount >= 3 {
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"has_more": false, "items": []any{1, 2}},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"has_more":    true,
					"next_cursor": 100,
					"items":       []any{callCount},
				},
			})
		}
	}))
	defer srv.Close()

	c := NewClient("test-token", srv.URL)

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
