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

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
)

// authRetryingKey marks a context that has already attempted one
// AuthRefreshRequired-driven retry of the current invocation. The runner uses
// this to refuse a second refresh+retry pass and surface the original cause
// to the user instead.
type authRetryingKeyType struct{}

type authRefreshFailureError struct {
	rejection error
	refresh   error
}

func (e *authRefreshFailureError) Error() string {
	return "automatic access token refresh failed"
}

func (e *authRefreshFailureError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return []error{e.rejection, e.refresh}
}

var authRetryingKey = authRetryingKeyType{}

var (
	runnerForceRefreshRejectedAccessToken = forceRefreshRejectedAccessToken
	runnerExecuteAuthRetry                func(*runtimeRunner, context.Context, string, executor.Invocation) (executor.Result, error)
)

func init() {
	runnerExecuteAuthRetry = func(r *runtimeRunner, ctx context.Context, endpoint string, invocation executor.Invocation) (executor.Result, error) {
		return r.executeInvocation(ctx, endpoint, invocation)
	}
}

// IsAuthRetrying reports whether the current context is already inside an
// AuthRefreshRequired retry. Mirrors IsPatRetrying.
func IsAuthRetrying(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(authRetryingKey).(bool)
	return v
}

func withAuthRetrying(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authRetryingKey, true)
}

func authRefreshLogger() *slog.Logger {
	if logger := FileLoggerInstance(); logger != nil {
		return logger
	}
	return slog.Default()
}

func (r *runtimeRunner) managesRuntimeOAuth(hasPluginAuth bool) bool {
	if r == nil || hasPluginAuth {
		return false
	}
	return r.globalFlags == nil || strings.TrimSpace(r.globalFlags.Token) == ""
}

// retryAuthRefreshRequired consumes only the explicit edition marker. It does
// not infer retryability from free text, generic auth categories, HTTP 403, or
// ordinary business errors.
func (r *runtimeRunner) retryAuthRefreshRequired(
	ctx context.Context,
	endpoint string,
	invocation executor.Invocation,
	rejectedAccessToken string,
	markerErr error,
	hasPluginAuth bool,
) (executor.Result, error, bool) {
	marker, marked := authretry.As(markerErr)
	if !marked {
		return executor.Result{}, nil, false
	}
	cause := marker.Cause
	if cause == nil {
		cause = markerErr
	}

	// Explicit --token and plugin credentials are not backed by the default
	// OAuth refresh store. Preserve the overlay cause without mutating an
	// unrelated persisted login.
	if !r.managesRuntimeOAuth(hasPluginAuth) {
		return executor.Result{}, cause, true
	}
	if IsAuthRetrying(ctx) {
		authRefreshLogger().Warn("auth.runtime.refresh.retry_exhausted",
			"product", invocation.CanonicalProduct,
			"tool", invocation.Tool,
		)
		return executor.Result{}, cause, true
	}

	if _, err := runnerForceRefreshRejectedAccessToken(ctx, defaultConfigDir(), rejectedAccessToken); err != nil {
		// Keep every log credential-safe. The returned error chain retains the
		// complete cause for in-process diagnosis; even DWS_DEBUG_AUTH must not
		// serialize an OAuth response body or other attacker-controlled text.
		authRefreshLogger().Warn("auth.runtime.refresh.failed",
			"product", invocation.CanonicalProduct,
			"tool", invocation.Tool,
			"stage", "force_refresh_rejected_token",
			"error_type", fmt.Sprintf("%T", err),
		)
		logging.AuthDebug("auth.runtime.refresh.failed.detail",
			"product", invocation.CanonicalProduct,
			"tool", invocation.Tool,
			"stage", "force_refresh_rejected_token",
			"error_type", fmt.Sprintf("%T", err),
		)
		combined := &authRefreshFailureError{rejection: cause, refresh: err}
		return executor.Result{}, apperrors.NewAuth(
			"automatic access token refresh failed",
			apperrors.WithOperation("auth/token/refresh"),
			apperrors.WithReason("auth_refresh_failed"),
			apperrors.WithHint("本地凭证已保留；可稍后重试，若持续失败请查看认证诊断日志。"),
			apperrors.WithCause(combined),
		), true
	}

	logging.AuthDebug("auth.runtime.refresh.succeeded",
		"product", invocation.CanonicalProduct,
		"tool", invocation.Tool,
	)
	result, err := runnerExecuteAuthRetry(r, withAuthRetrying(ctx), endpoint, invocation)
	return result, err, true
}

// isRefreshableTransportAuthError deliberately excludes HTTP/RPC 403 and
// generic CategoryAuth values. OnAuthError may request a refresh only for an
// exact transport-level unauthorized signal.
func isRefreshableTransportAuthError(err error) bool {
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAuth {
		return false
	}
	return typed.Reason == "http_401" || typed.RPCCode == 401
}
