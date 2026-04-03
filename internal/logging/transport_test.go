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

	LogResponse(logger, "tools/call", "https://mcp.dingtalk.com/api", "exec-1", 200, 1024, 150*time.Millisecond, nil)

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

	LogResponse(logger, "initialize", "https://mcp.dingtalk.com", "exec-2", 500, 0, 2*time.Second, errors.New("connection refused"))

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
	LogResponse(nil, "test", "http://localhost", "", 200, 0, 0, nil)
	LogRequestBody(nil, "tools/call", "exec-1", "tool", nil)
	LogResponseBody(nil, "tools/call", "exec-1", 200, nil, "")
	LogRetryAttempt(nil, "tools/call", "exec-1", 0, 2, 429, 0, nil)
	LogErrorClassified(nil, "tools/call", "exec-1", "api", "timeout", 0, 0, true, "")
	LogCommandStart(nil, "exec-1", "doc", "list", "https://mcp.example.com", "1.0.0", false, 0)
	LogCommandEnd(nil, "exec-1", "doc", "list", true, 0, "", "")
}

func TestLogRequestBody(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	args := map[string]any{"name": "test", "limit": 10}
	LogRequestBody(logger, "tools/call", "exec-1", "doc.list", args)

	out := buf.String()
	if !strings.Contains(out, "jsonrpc_request_body") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "doc.list") {
		t.Error("missing tool_name")
	}
	if !strings.Contains(out, "exec-1") {
		t.Error("missing execution_id")
	}
}

func TestLogRequestBody_SkipsNonToolsCall(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogRequestBody(logger, "initialize", "exec-1", "", nil)
	if buf.Len() != 0 {
		t.Error("should not log body for non-tools/call methods")
	}
}

func TestLogResponseBody(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogResponseBody(logger, "tools/call", "exec-1", 500, []byte(`{"error":"fail"}`), "trace-abc")

	out := buf.String()
	if !strings.Contains(out, "jsonrpc_response_body") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "trace-abc") {
		t.Error("missing trace_id")
	}
	if !strings.Contains(out, "WARN") {
		t.Error("expected WARN level for 500 status")
	}
}

func TestLogRetryAttempt(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogRetryAttempt(logger, "tools/call", "exec-1", 0, 2, 429, 10*time.Millisecond, errors.New("rate limited"))

	out := buf.String()
	if !strings.Contains(out, "jsonrpc_retry") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "rate limited") {
		t.Error("missing error")
	}
}

func TestLogErrorClassified(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	LogErrorClassified(logger, "tools/call", "exec-1", "auth", "http_401", 401, 0, false, "trace-xyz")

	out := buf.String()
	if !strings.Contains(out, "error_classified") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "trace-xyz") {
		t.Error("missing trace_id")
	}
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
