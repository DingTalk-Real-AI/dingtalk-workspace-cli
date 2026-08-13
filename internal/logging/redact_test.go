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

package logging

import (
	"net/http"
	"strings"
	"testing"
)

func TestIsSensitiveKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want bool
	}{
		{"Authorization", true},
		{"authorization", true},
		{"x-user-access-token", true},
		{"X-User-Access-Token", true},
		{"DWS_AGENT_EXT", true},
		{"dws_agent_ext", true},
		{"x-dws-agent-ext", true},
		{"X-Dws-Agent-Ext", true},
		{"DWS_AGENT_VER", false},
		{"x-dws-agent-ver", false},
		{"client_secret", true},
		{"client-secret", true},
		{"token", true},
		{"password", true},
		{"cookie", true},
		{"Content-Type", false},
		{"X-Cli-Source", false},
		{"method", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			if got := IsSensitiveKey(tt.key); got != tt.want {
				t.Errorf("IsSensitiveKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestRedactValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"short", "***"},
		{"12345678", "***"},
		{"123456789", "1234***"},
		{"a-very-long-token-value", "a-ve***"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := RedactValue(tt.input); got != tt.want {
				t.Errorf("RedactValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	t.Parallel()
	short := []byte("hello")
	if got := TruncateBody(short, 100); got != "hello" {
		t.Fatalf("expected no truncation, got %q", got)
	}
	long := []byte(strings.Repeat("a", 200))
	got := TruncateBody(long, 50)
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	if !strings.Contains(got, "total=200") {
		t.Fatalf("expected total size, got %q", got)
	}
}

func TestTruncateBody_Empty(t *testing.T) {
	t.Parallel()
	if got := TruncateBody(nil, 100); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestSanitizeArguments(t *testing.T) {
	t.Parallel()
	args := map[string]any{
		"name":     "test",
		"password": "secret123",
		"nested": map[string]any{
			"api_key": "key-value",
			"safe":    "ok",
		},
	}
	got := SanitizeArguments(args, 4096)
	if strings.Contains(got, "secret123") {
		t.Fatalf("password should be redacted: %s", got)
	}
	if strings.Contains(got, "key-value") {
		t.Fatalf("api_key should be redacted: %s", got)
	}
	if !strings.Contains(got, "test") {
		t.Fatalf("non-sensitive value should remain: %s", got)
	}
}

func TestSanitizeArguments_Empty(t *testing.T) {
	t.Parallel()
	if got := SanitizeArguments(nil, 100); got != "{}" {
		t.Fatalf("expected {}, got %q", got)
	}
}

func TestRedactHeaders(t *testing.T) {
	t.Parallel()
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer test-credential-value")
	headers.Set("Content-Type", "application/json")
	headers.Set("DWS_AGENT_EXT", `{"umt":"test-umt-value"}`)
	headers.Set("x-dws-agent-ext", `{"ua":"test-agent-value"}`)
	attrs := RedactHeaders(headers)
	if len(attrs) != 4 {
		t.Fatalf("expected 4 attrs, got %d", len(attrs))
	}
	for _, attr := range attrs {
		switch attr.Key {
		case "header.authorization", "header.dws_agent_ext", "header.x-dws-agent-ext":
			if !strings.Contains(attr.Value.String(), "***") {
				t.Fatalf("%s should be redacted: %s", attr.Key, attr.Value.String())
			}
			if strings.Contains(attr.Value.String(), "test-") {
				t.Fatalf("%s leaked its original value: %s", attr.Key, attr.Value.String())
			}
		}
		if attr.Key == "header.content-type" && attr.Value.String() != "application/json" {
			t.Fatalf("content-type should not be redacted: %s", attr.Value.String())
		}
	}
}

func TestIsSensitiveKey_Substrings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key  string
		want bool
	}{
		{"x-api-token", true},
		{"user_password_hash", true},
		{"my_secret_key", true},
		{"x-credential-id", true},
		{"safe-header", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			if got := IsSensitiveKey(tt.key); got != tt.want {
				t.Errorf("IsSensitiveKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
