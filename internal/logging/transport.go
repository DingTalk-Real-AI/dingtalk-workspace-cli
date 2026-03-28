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
	"time"
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

// LogResponse logs a JSON-RPC response at Debug level.
func LogResponse(logger *slog.Logger, method, endpoint string, statusCode int, respSize int, duration time.Duration, err error) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("endpoint", redactEndpoint(endpoint)),
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

// redactEndpoint removes query parameters from endpoint URLs in logs.
func redactEndpoint(endpoint string) string {
	for i := 0; i < len(endpoint); i++ {
		if endpoint[i] == '?' || endpoint[i] == '#' {
			return endpoint[:i]
		}
	}
	return endpoint
}
