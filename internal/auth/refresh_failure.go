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

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

// RefreshFailureClass separates refresh failures that may recover after a
// delay from failures that require new credentials or local intervention.
type RefreshFailureClass string

const (
	RefreshFailureUnknown   RefreshFailureClass = "unknown"
	RefreshFailureTransient RefreshFailureClass = "transient"
	RefreshFailureTerminal  RefreshFailureClass = "terminal"
)

// HTTPStatusError preserves an OAuth endpoint status for structured retry
// decisions without copying an untrusted response body into logs.
type HTTPStatusError struct {
	StatusCode   int
	responseBody string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "OAuth endpoint request failed"
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func httpStatusResponseBody(err error) string {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr == nil {
		return ""
	}
	return statusErr.responseBody
}

// ClassifyRefreshFailure uses only structured transport and HTTP signals.
// Unknown errors, including parse, keychain and persistence failures, remain
// fatal so a long-running source cannot retry an error that needs user action.
func ClassifyRefreshFailure(err error) RefreshFailureClass {
	if err == nil {
		return RefreshFailureUnknown
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return RefreshFailureTransient
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return RefreshFailureTransient
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return RefreshFailureTransient
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr == nil {
		return RefreshFailureUnknown
	}
	if statusErr.StatusCode == http.StatusRequestTimeout ||
		statusErr.StatusCode == http.StatusTooManyRequests ||
		statusErr.StatusCode >= http.StatusInternalServerError {
		return RefreshFailureTransient
	}
	if statusErr.StatusCode == http.StatusBadRequest ||
		statusErr.StatusCode == http.StatusUnauthorized ||
		statusErr.StatusCode == http.StatusForbidden {
		return RefreshFailureTerminal
	}
	return RefreshFailureUnknown
}
