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

package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// serverDiagFields maps the various JSON field names the server may use
// for diagnostic information. Both snake_case and camelCase are supported.
type serverDiagFields struct {
	TraceID         string `json:"trace_id"`
	TraceIDCamel    string `json:"traceId"`
	Code            string `json:"code"`
	ErrorCode       string `json:"errorCode"`
	TechnicalDetail string `json:"technical_detail"`
	Retryable       *bool  `json:"retryable"`
}

// ExtractServerDiagnostics parses server diagnostic fields from a JSON
// payload (typically from RPCError.Data). Returns an empty struct if
// the payload is empty or unparseable.
func ExtractServerDiagnostics(data json.RawMessage) apperrors.ServerDiagnostics {
	if len(data) == 0 {
		return apperrors.ServerDiagnostics{}
	}
	var content map[string]any
	if json.Unmarshal(data, &content) != nil {
		return apperrors.ServerDiagnostics{}
	}
	return ExtractServerDiagnosticsFromMap(content)
}

// ExtractServerDiagnosticsFromMap parses server diagnostic fields from a
// map[string]any (typically from ToolCallResult.Content for business errors).
func ExtractServerDiagnosticsFromMap(content map[string]any) apperrors.ServerDiagnostics {
	if len(content) == 0 {
		return apperrors.ServerDiagnostics{}
	}
	diag := apperrors.ServerDiagnostics{
		TraceID:         stringFromMap(content, "trace_id", "traceId"),
		ServerErrorCode: serverErrorCodeFromMap(content, 0),
		TechnicalDetail: stringFromMap(content, "technical_detail"),
	}
	if v, ok := content["retryable"].(bool); ok {
		diag.ServerRetryable = &v
	}
	return diag
}

// ExtractTraceIDFromHeaders reads a trace ID from standard HTTP response
// headers. Returns empty string if none found.
func ExtractTraceIDFromHeaders(headers http.Header) string {
	for _, key := range []string{
		"X-Trace-Id",
		"X-Request-Id",
		"x-dingtalk-trace-id",
	} {
		if v := headers.Get(key); v != "" {
			return v
		}
	}
	return ""
}

// coalesceStr returns the first non-empty string.
func coalesceStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// stringFromMap returns the first non-empty string value found for any of
// the given keys in the map.
func stringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func serverErrorCodeFromMap(content map[string]any, depth int) string {
	if content == nil || depth > 8 {
		return ""
	}
	direct := stringFromMap(content, "errorCode", "code", "server_error_code")
	if isWrapperServerCode(direct) {
		if nested := nestedServerErrorCode(content, depth); nested != "" {
			return nested
		}
	}
	if direct != "" {
		return direct
	}
	return nestedServerErrorCode(content, depth)
}

func nestedServerErrorCode(content map[string]any, depth int) string {
	for _, key := range []string{"content", "result", "data"} {
		switch child := content[key].(type) {
		case map[string]any:
			if code := serverErrorCodeFromMap(child, depth+1); code != "" {
				return code
			}
		case []any:
			for _, item := range child {
				childMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if code := serverErrorCodeFromMap(childMap, depth+1); code != "" {
					return code
				}
			}
		}
	}
	return ""
}

func isWrapperServerCode(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "ERROR", "BUSINESS_ERROR", "-1":
		return true
	default:
		return false
	}
}
