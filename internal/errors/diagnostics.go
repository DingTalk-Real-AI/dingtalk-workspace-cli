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

package errors

import (
	stderrors "errors"
	"strings"
)

// ServerDiagnostics holds server-side diagnostic fields extracted from
// MCP response bodies or HTTP response headers. Fields are populated
// on a best-effort basis during error construction.
type ServerDiagnostics struct {
	TraceID         string `json:"trace_id,omitempty"`
	ServerErrorCode string `json:"server_error_code,omitempty"`
	TechnicalDetail string `json:"technical_detail,omitempty"`
	FriendlyHint    string `json:"friendly_hint,omitempty"`
	ActionURL       string `json:"action_url,omitempty"`
	ServerRetryable *bool  `json:"server_retryable,omitempty"`
}

// IsEmpty returns true when no diagnostic field has been populated.
func (d ServerDiagnostics) IsEmpty() bool {
	return d.TraceID == "" && d.ServerErrorCode == "" &&
		d.TechnicalDetail == "" && d.FriendlyHint == "" &&
		d.ActionURL == "" && d.ServerRetryable == nil
}

// WithServerDiag attaches server diagnostics to the error.
func WithServerDiag(diag ServerDiagnostics) Option {
	if diag.IsEmpty() {
		return func(*Error) {}
	}
	return func(e *Error) {
		e.ServerDiag = diag
		// Override retryable if server explicitly specified.
		if diag.ServerRetryable != nil {
			e.Retryable = *diag.ServerRetryable
			e.RetryableSet = true
		}
	}
}

// WithTraceID records the server-provided trace identifier.
// Used when only the trace ID is available (e.g. from HTTP headers)
// without a full ServerDiagnostics struct.
func WithTraceID(id string) Option {
	id = strings.TrimSpace(id)
	if id == "" {
		return func(*Error) {}
	}
	return func(e *Error) {
		e.ServerDiag.TraceID = id
	}
}

// IsMCPToolNotFound reports whether an MCP call failed because the selected
// server does not expose that tool name. Callers may use it to fall back
// between reviewed read-only aliases, never to replay a write.
func IsMCPToolNotFound(err error) bool {
	if err == nil {
		return false
	}
	parts := []string{err.Error()}
	var typed *Error
	if stderrors.As(err, &typed) && typed != nil {
		parts = append(parts,
			typed.Reason,
			typed.ServerDiag.ServerErrorCode,
			typed.ServerDiag.TechnicalDetail,
			typed.Hint,
		)
	}
	message := strings.ToLower(strings.Join(parts, " "))
	for _, marker := range []string{
		"tool_not_found",
		"mcp_tool_not_found",
		"tool not found",
		"tool not registered",
		"tool not exist",
		"tool does not exist",
		"unknown tool",
		"no such tool",
		"未找到指定工具",
		"未找到工具",
		"工具不存在",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
