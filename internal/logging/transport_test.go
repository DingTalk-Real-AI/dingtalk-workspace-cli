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
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogRequestWritesStructuredEntry(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogRequest(logger, "tools/call", "https://mcp.dingtalk.com/api?token=secret", "exec-123", 256)

	out := buf.String()
	if !strings.Contains(out, "jsonrpc_request") {
		t.Error("missing message 'jsonrpc_request'")
	}
	if !strings.Contains(out, "tools/call") {
		t.Error("missing method")
	}
	if !strings.Contains(out, "exec-123") {
		t.Error("missing execution_id")
	}
	// Query should be stripped
	if strings.Contains(out, "token=secret") {
		t.Error("endpoint query params should be redacted")
	}
}

func TestLogResponseSuccess(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogResponse(logger, "tools/call", "https://mcp.dingtalk.com/api", 200, 1024, 150*time.Millisecond, nil)

	out := buf.String()
	if !strings.Contains(out, "jsonrpc_response") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "DEBUG") {
		t.Error("success should be DEBUG level")
	}
	if strings.Contains(out, "error") {
		t.Error("success response should not contain error field")
	}
}

func TestLogResponseError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogResponse(logger, "initialize", "https://mcp.dingtalk.com", 500, 0, 2*time.Second, errors.New("connection refused"))

	out := buf.String()
	if !strings.Contains(out, "WARN") {
		t.Error("error response should be WARN level")
	}
	if !strings.Contains(out, "connection refused") {
		t.Error("error message missing")
	}
}

func TestLogRequestNilLogger(t *testing.T) {
	t.Parallel()
	// Should not panic
	LogRequest(nil, "test", "http://localhost", "", 0)
	LogResponse(nil, "test", "http://localhost", 200, 0, 0, nil)
}

func TestRedactEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"https://mcp.dingtalk.com/api", "https://mcp.dingtalk.com/api"},
		{"https://mcp.dingtalk.com/api?token=abc", "https://mcp.dingtalk.com/api"},
		{"https://mcp.dingtalk.com/api#section", "https://mcp.dingtalk.com/api"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := redactEndpoint(tt.input); got != tt.want {
				t.Errorf("redactEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
