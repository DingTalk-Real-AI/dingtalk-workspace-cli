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
	"log/slog"
	"net/http"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// authRetryingKey marks a context that has already attempted one
// AuthRefreshRequired-driven retry of the current invocation. The runner uses
// this to refuse a second refresh+retry pass and surface the original cause
// to the user instead.
type authRetryingKeyType struct{}

var authRetryingKey = authRetryingKeyType{}

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

var forceRefreshRejectedTokenFunc = ForceRefreshRejectedToken

func (r *runtimeRunner) maybeAuthRefreshRetry(
	ctx context.Context,
	endpoint string,
	invocation executor.Invocation,
	rejectedAccessToken string,
	sourceErr error,
	refresh *authretry.AuthRefreshRequired,
) (executor.Result, error) {
	originalErr := authRefreshCause(refresh, sourceErr)
	if refresh == nil || !authRefreshAllowed(sourceErr, originalErr) {
		return executor.Result{}, originalErr
	}
	if IsAuthRetrying(ctx) {
		credentialDeleted, cleanupFailed := purgeCurrentAuthCredentials(ctx, rejectedAccessToken)
		logAuthRefreshRecovery("retry_exhausted", invocation, credentialDeleted, cleanupFailed)
		return executor.Result{}, originalErr
	}

	if _, err := forceRefreshRejectedTokenFunc(ctx, defaultConfigDir(), rejectedAccessToken); err != nil {
		credentialDeleted, cleanupFailed := purgeCurrentAuthCredentials(ctx, rejectedAccessToken)
		logAuthRefreshRecovery("refresh_failed", invocation, credentialDeleted, cleanupFailed)
		return executor.Result{}, originalErr
	}
	invalidateAuthCaches()
	return r.executeInvocation(withAuthRetrying(ctx), endpoint, invocation)
}

func authRefreshCause(refresh *authretry.AuthRefreshRequired, fallback error) error {
	if refresh != nil && refresh.Cause != nil {
		return refresh.Cause
	}
	if fallback != nil {
		return fallback
	}
	return refresh
}

func authRefreshAllowed(sourceErr, originalErr error) bool {
	if apperrors.AsPatAuthCheckError(sourceErr) != nil || apperrors.AsPatAuthCheckError(originalErr) != nil {
		return false
	}
	var patScope *PatScopeError
	if errors.As(sourceErr, &patScope) || errors.As(originalErr, &patScope) {
		return false
	}
	if isPatScopeError(sourceErr) || isPatScopeError(originalErr) {
		return false
	}

	if authRefreshDeniedByTypedError(sourceErr) || authRefreshDeniedByTypedError(originalErr) {
		return false
	}
	return true
}

func authRefreshDeniedByTypedError(err error) bool {
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		return false
	}
	if typed.Reason == "http_403" || typed.RPCCode == http.StatusForbidden {
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(typed.ServerDiag.ServerErrorCode)) {
	case "CLI_ORG_NOT_AUTHORIZED", "AUTH_PERMISSION_DENIED", "PERMISSION_DENIED":
		return true
	default:
		return false
	}
}

func purgeCurrentAuthCredentials(ctx context.Context, expectedAccessToken string) (bool, bool) {
	deleted, err := authpkg.DeleteTokenDataIfAccessTokenMatches(ctx, defaultConfigDir(), expectedAccessToken)
	invalidateAuthCaches()
	return deleted, err != nil
}

func invalidateAuthCaches() {
	ResetRuntimeTokenCache()
	if fn := edition.Get().InvalidateAuthCaches; fn != nil {
		fn()
	}
}

func logAuthRefreshRecovery(reason string, invocation executor.Invocation, credentialDeleted, cleanupFailed bool) {
	slog.Warn("runtime.auth_refresh_recovery",
		"reason", reason,
		"product", invocation.CanonicalProduct,
		"tool", invocation.Tool,
		"profile_selected", strings.TrimSpace(authpkg.RuntimeProfile()) != "",
		"credential_deleted", credentialDeleted,
		"credential_cleanup_failed", cleanupFailed,
	)
}

func (r *runtimeRunner) canAutoRefreshAuth(hasPluginAuth bool) bool {
	if r == nil || hasPluginAuth {
		return false
	}
	return r.globalFlags == nil || strings.TrimSpace(r.globalFlags.Token) == ""
}
