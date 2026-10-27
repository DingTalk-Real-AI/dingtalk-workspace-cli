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
	"fmt"
	"net/http"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

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
		TraceID:         stringFromMapRecursive(content, 0, "trace_id", "traceId", "request_id", "requestId"),
		ServerErrorCode: serverErrorCodeFromMap(content, 0),
		ErrorStage:      stringFromMapRecursive(content, 0, "errorStage", "error_stage"),
		TechnicalDetail: stringFromMapRecursive(content, 0, "technical_detail", "technicalDetail"),
		FriendlyHint:    stringFromMapRecursive(content, 0, "friendly_hint", "friendlyHint"),
		ActionURL:       stringFromMapRecursive(content, 0, "action_url", "actionUrl"),
	}
	if diag.TraceID == "" {
		// Preserve the existing cross-product lookup, including wrapper traversal.
		// Product-specific log/request semantics belong in the product decoder.
		diag.TraceID = stringFromMapRecursive(content, 0, "logId", "log_id")
	}
	// Business classification is diagnostic only; do not derive retryability
	// or replace the CLI's authentication/network error category from it.
	switch category := stringFromMapRecursive(content, 0, "errorCategory", "error_category"); category {
	case "TIMEOUT", "VALIDATION", "DOWNSTREAM":
		diag.ErrorCategory = category
	}
	if v, ok := boolFromMapRecursive(content, 0, "retryable", "server_retryable", "serverRetryable"); ok {
		diag.ServerRetryable = &v
	}
	errorContent := explicitErrorDiagnosticsContainer(content, 0)
	if errorContent == nil {
		return diag
	}
	diag.ErrorType = serverErrorTypeFromMap(errorContent)
	diag.FriendlyHint = coalesceStr(
		stringFromMapRecursive(errorContent, 0, "friendly_hint", "friendlyHint", "hint"),
		diag.FriendlyHint,
	)
	diag.ActionURL = coalesceStr(stringFromMapRecursive(errorContent, 0, "action_url", "actionUrl"), diag.ActionURL)
	diag.Reason = sanitizedServerReason(stringFromMapRecursive(errorContent, 0, "reason", "subtype"))
	diag.Operation = stringFromMapRecursive(errorContent, 0, "operation")
	diag.Origin = stringFromMapRecursive(errorContent, 0, "origin")
	diag.FailureStage = stringFromMapRecursive(errorContent, 0, "failure_stage", "failureStage", "stage")
	diag.NextCommand = stringFromMapRecursive(errorContent, 0, "next_command", "nextCommand", "nextStep")
	diag.Outcome = stringFromMapRecursive(errorContent, 0, "outcome")
	diag.ContractVersion = stringFromMapRecursive(errorContent, 0, "contractVersion", "contract_version")
	diag.Actions = stringSliceFromMapRecursive(errorContent, 0, "actions")
	diag.Details = mapFromMapRecursive(errorContent, 0, "details")
	if v, ok := boolFromMapRecursive(errorContent, 0, "retryable", "server_retryable", "serverRetryable"); ok {
		diag.ServerRetryable = &v
	}
	if v, ok := boolFromMapRecursive(errorContent, 0, "unknown"); ok {
		diag.Unknown = &v
	}
	if v, ok := boolFromMapRecursive(errorContent, 0, "execution_started", "executionStarted", "executed"); ok {
		diag.ExecutionStarted = &v
	}
	return diag
}

func explicitErrorDiagnosticsContainer(content map[string]any, depth int) map[string]any {
	if content == nil || depth > 8 {
		return nil
	}
	for _, key := range []string{"errorDetail", "error_detail", "error"} {
		switch value := content[key].(type) {
		case map[string]any:
			if nested := explicitErrorDiagnosticsContainer(value, depth+1); nested != nil {
				return nested
			}
			return value
		case []any:
			var fallback map[string]any
			for _, item := range value {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if fallback == nil {
					fallback = itemMap
				}
				if nested := explicitErrorDiagnosticsContainer(itemMap, depth+1); nested != nil {
					return nested
				}
			}
			if fallback != nil {
				return fallback
			}
		}
	}
	for _, key := range []string{"content", "result", "data"} {
		switch child := content[key].(type) {
		case map[string]any:
			if value := explicitErrorDiagnosticsContainer(child, depth+1); value != nil {
				return value
			}
		case []any:
			for _, item := range child {
				childMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if value := explicitErrorDiagnosticsContainer(childMap, depth+1); value != nil {
					return value
				}
			}
		}
	}
	return nil
}

// IsFrameworkReservedReason reports whether a reason is owned by the CLI framework.
func IsFrameworkReservedReason(reason string) bool {
	return strings.EqualFold(strings.TrimSpace(reason), "confirmation_required")
}

func serverErrorTypeFromMap(content map[string]any) string {
	if value := stringFromMap(content, "error_type", "errorType"); value != "" {
		return value
	}
	value := strings.ToLower(strings.TrimSpace(stringFromMap(content, "type")))
	switch value {
	case "api", "auth", "authorization", "permission", "forbidden", "validation", "discovery", "internal":
		return value
	default:
		return ""
	}
}

func sanitizedServerReason(reason string) string {
	if IsFrameworkReservedReason(reason) {
		return ""
	}
	return reason
}

func boolFromMapRecursive(content map[string]any, depth int, keys ...string) (bool, bool) {
	if content == nil || depth > 8 {
		return false, false
	}
	for _, key := range keys {
		if value, ok := content[key].(bool); ok {
			return value, true
		}
	}
	for _, key := range []string{"errorDetail", "error_detail", "content", "result", "data", "error", "details"} {
		switch child := content[key].(type) {
		case map[string]any:
			if value, ok := boolFromMapRecursive(child, depth+1, keys...); ok {
				return value, true
			}
		case []any:
			for _, item := range child {
				childMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if value, ok := boolFromMapRecursive(childMap, depth+1, keys...); ok {
					return value, true
				}
			}
		}
	}
	return false, false
}

func stringFromMapRecursive(content map[string]any, depth int, keys ...string) string {
	if content == nil || depth > 8 {
		return ""
	}
	if value := stringFromMap(content, keys...); value != "" {
		return value
	}
	for _, key := range []string{"errorDetail", "error_detail", "content", "result", "data", "error", "details"} {
		switch child := content[key].(type) {
		case map[string]any:
			if value := stringFromMapRecursive(child, depth+1, keys...); value != "" {
				return value
			}
		case []any:
			for _, item := range child {
				childMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if value := stringFromMapRecursive(childMap, depth+1, keys...); value != "" {
					return value
				}
			}
		}
	}
	return ""
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

func scalarStringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := m[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			return text
		}
		switch value.(type) {
		case bool, map[string]any, []any:
			continue
		default:
			return fmt.Sprint(value)
		}
	}
	return ""
}

func stringSliceFromMapRecursive(content map[string]any, depth int, keys ...string) []string {
	if content == nil || depth > 8 {
		return nil
	}
	for _, key := range keys {
		switch values := content[key].(type) {
		case []any:
			out := make([]string, 0, len(values))
			for _, value := range values {
				if item, ok := value.(string); ok && strings.TrimSpace(item) != "" {
					out = append(out, item)
				}
			}
			if len(out) > 0 {
				return out
			}
		case []string:
			return append([]string(nil), values...)
		}
	}
	for _, key := range []string{"errorDetail", "error_detail", "content", "result", "data", "error", "details"} {
		switch child := content[key].(type) {
		case map[string]any:
			if values := stringSliceFromMapRecursive(child, depth+1, keys...); len(values) > 0 {
				return values
			}
		case []any:
			for _, item := range child {
				childMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if values := stringSliceFromMapRecursive(childMap, depth+1, keys...); len(values) > 0 {
					return values
				}
			}
		}
	}
	return nil
}

func mapFromMapRecursive(content map[string]any, depth int, keys ...string) map[string]any {
	if content == nil || depth > 8 {
		return nil
	}
	for _, key := range keys {
		if value, ok := content[key].(map[string]any); ok {
			return value
		}
	}
	for _, key := range []string{"errorDetail", "error_detail", "content", "result", "data", "error", "details"} {
		switch child := content[key].(type) {
		case map[string]any:
			if value := mapFromMapRecursive(child, depth+1, keys...); value != nil {
				return value
			}
		case []any:
			for _, item := range child {
				childMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if value := mapFromMapRecursive(childMap, depth+1, keys...); value != nil {
					return value
				}
			}
		}
	}
	return nil
}

func serverErrorCodeFromMap(content map[string]any, depth int) string {
	if content == nil || depth > 8 {
		return ""
	}
	direct := scalarStringFromMap(content, "errorCode", "code", "server_error_code", "serverErrorCode")
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
	for _, key := range []string{"errorDetail", "error_detail", "content", "result", "data", "error", "details"} {
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
