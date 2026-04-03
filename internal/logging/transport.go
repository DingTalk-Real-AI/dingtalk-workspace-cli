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
	"context"
	"log/slog"
	"runtime"
	"time"
)

const (
	// maxBodyLogSize is the maximum bytes of request/response body to log.
	maxBodyLogSize = 4096
	// maxArgLogSize is the maximum bytes for sanitized argument summaries.
	maxArgLogSize = 1024
)

// LogRequest logs a JSON-RPC request at Debug level.
func LogRequest(logger *slog.Logger, method, endpoint, executionId string, bodySize int) {
	if logger == nil {
		return
	}
	logger.Debug("jsonrpc_request",
		slog.String("method", method),
		slog.String("endpoint", redactEndpoint(endpoint)),
		slog.String("execution_id", executionId),
		slog.Int("body_size", bodySize),
	)
}

// LogRequestBody logs a truncated, redacted request body for tools/call.
func LogRequestBody(logger *slog.Logger, method, executionId string, toolName string, arguments map[string]any) {
	if logger == nil || method != "tools/call" {
		return
	}
	logger.Debug("jsonrpc_request_body",
		slog.String("method", method),
		slog.String("execution_id", executionId),
		slog.String("tool_name", toolName),
		slog.String("arguments_summary", SanitizeArguments(arguments, maxArgLogSize)),
	)
}

// LogResponse logs a JSON-RPC response at Debug level (Warn on error).
func LogResponse(logger *slog.Logger, method, endpoint, executionId string, statusCode int, respSize int, duration time.Duration, err error) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("endpoint", redactEndpoint(endpoint)),
		slog.String("execution_id", executionId),
		slog.Int("status", statusCode),
		slog.Int("resp_size", respSize),
		slog.String("duration", duration.Truncate(time.Millisecond).String()),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		logger.LogAttrs(context.TODO(), slog.LevelWarn, "jsonrpc_response", attrs...)
		return
	}
	logger.LogAttrs(context.TODO(), slog.LevelDebug, "jsonrpc_response", attrs...)
}

// LogResponseBody logs a truncated response body on error paths.
func LogResponseBody(logger *slog.Logger, method, executionId string, statusCode int, body []byte, traceID string) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("execution_id", executionId),
		slog.Int("status", statusCode),
		slog.String("body", TruncateBody(body, maxBodyLogSize)),
	}
	if traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	level := slog.LevelDebug
	if statusCode >= 400 {
		level = slog.LevelWarn
	}
	logger.LogAttrs(context.TODO(), level, "jsonrpc_response_body", attrs...)
}

// LogRetryAttempt logs a retry attempt at Warn level.
func LogRetryAttempt(logger *slog.Logger, method, executionId string, attempt, maxRetries int, statusCode int, delay time.Duration, lastErr error) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("execution_id", executionId),
		slog.Int("attempt", attempt+1),
		slog.Int("max_attempts", maxRetries+1),
		slog.Int("status", statusCode),
		slog.String("delay", delay.String()),
	}
	if lastErr != nil {
		attrs = append(attrs, slog.String("error", lastErr.Error()))
	}
	logger.LogAttrs(context.TODO(), slog.LevelWarn, "jsonrpc_retry", attrs...)
}

// LogErrorClassified logs the final error classification at Warn level.
func LogErrorClassified(logger *slog.Logger, method, executionId, category, reason string, httpStatus, rpcCode int, retryable bool, traceID string) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("execution_id", executionId),
		slog.String("category", category),
		slog.String("reason", reason),
		slog.Bool("retryable", retryable),
	}
	if httpStatus != 0 {
		attrs = append(attrs, slog.Int("http_status", httpStatus))
	}
	if rpcCode != 0 {
		attrs = append(attrs, slog.Int("rpc_code", rpcCode))
	}
	if traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	logger.LogAttrs(context.TODO(), slog.LevelWarn, "error_classified", attrs...)
}

// LogCommandStart logs the beginning of a command execution.
func LogCommandStart(logger *slog.Logger, executionId, product, tool, endpoint, version string, authPresent bool, timeoutSec int) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("execution_id", executionId),
		slog.String("product", product),
		slog.String("tool", tool),
		slog.String("endpoint", redactEndpoint(endpoint)),
		slog.String("cli_version", version),
		slog.String("os", runtime.GOOS),
		slog.String("arch", runtime.GOARCH),
		slog.Bool("auth_token_present", authPresent),
	}
	if timeoutSec > 0 {
		attrs = append(attrs, slog.Int("timeout_sec", timeoutSec))
	}
	logger.LogAttrs(context.TODO(), slog.LevelInfo, "command_start", attrs...)
}

// LogCommandEnd logs the end of a command execution.
func LogCommandEnd(logger *slog.Logger, executionId, product, tool string, success bool, duration time.Duration, errCategory, errReason string) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("execution_id", executionId),
		slog.String("product", product),
		slog.String("tool", tool),
		slog.Bool("success", success),
		slog.String("duration", duration.Truncate(time.Millisecond).String()),
	}
	if !success {
		attrs = append(attrs, slog.String("error_category", errCategory))
		attrs = append(attrs, slog.String("error_reason", errReason))
	}
	logger.LogAttrs(context.TODO(), slog.LevelInfo, "command_end", attrs...)
}

// redactEndpoint removes query parameters from endpoint URLs in logs.
func redactEndpoint(endpoint string) string {
	for i := 0; i < len(endpoint); i++ {
		if endpoint[i] == '?' || endpoint[i] == '#' {
			return endpoint[:i]
		}
	}
	return endpoint
}
