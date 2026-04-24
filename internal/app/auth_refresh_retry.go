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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
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

// withAuthRetrying returns a child context flagged as "already retried once"
// so the runner does not enter an infinite refresh loop if the second attempt
// also returns AuthRefreshRequired.
func withAuthRetrying(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authRetryingKey, true)
}

// handleAuthRefreshRequired performs a one-shot ForceRefresh using the active
// configDir and re-runs the invocation through the supplied runner. It must
// only be called when the runner has observed an *authretry.AuthRefreshRequired
// from an edition hook (ClassifyToolResult / OnAuthError).
//
// Behaviour rules — all three matter for safety:
//  1. If the context is already flagged via IsAuthRetrying, this returns
//     refresh.Cause unchanged. No further refresh attempts, no recursion.
//  2. If ForceRefresh fails (e.g. refresh_token also expired), this returns
//     refresh.Cause so the user sees the original auth diagnostic, not an
//     internal "force refresh failed" message.
//  3. On successful refresh, this resets the per-process token cache and
//     re-runs the invocation with withAuthRetrying applied so a second
//     refresh request from the overlay degrades gracefully to "show the
//     original error".
func handleAuthRefreshRequired(
	ctx context.Context,
	r executor.Runner,
	invocation executor.Invocation,
	refresh *authretry.AuthRefreshRequired,
) (executor.Result, error) {
	if refresh == nil {
		return executor.Result{}, nil
	}
	if IsAuthRetrying(ctx) {
		return executor.Result{}, refresh.Cause
	}
	if _, err := ForceRefreshAccessToken(ctx, defaultConfigDir()); err != nil {
		return executor.Result{}, refresh.Cause
	}
	ResetRuntimeTokenCache()
	return r.Run(withAuthRetrying(ctx), invocation)
}
