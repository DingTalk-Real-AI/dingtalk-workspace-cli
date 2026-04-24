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

// Package authretry exposes the AuthRefreshRequired marker error used by
// edition overlays (in separate Go modules) to signal "the current access
// token was rejected by the server; please run a force-refresh and retry".
//
// The package has no dependencies beyond the standard library so it can be
// imported by both the runner (internal/app) and external overlays without
// creating import cycles with packages such as pkg/runtimetoken.
package authretry

import (
	stderrors "errors"
)

// AuthRefreshRequired is a marker error returned by overlays (typically from
// the edition's ClassifyToolResult or OnAuthError hook) to ask the runner to
// perform a one-shot ForceRefresh of the access token and retry the current
// invocation.
//
// Open-source code paths never produce this type, so the runner's retry
// branch is a no-op for editions that don't opt in.
type AuthRefreshRequired struct {
	// Cause is the underlying user-facing error. The runner returns Cause
	// (not the wrapper) when refresh fails or the retry budget is exhausted,
	// so end users always see the original diagnostic instead of an internal
	// "auth refresh required" message.
	Cause error
}

// Error returns the underlying cause's message, or a generic fallback when
// Cause is nil.
func (e *AuthRefreshRequired) Error() string {
	if e == nil || e.Cause == nil {
		return "authentication refresh required"
	}
	return e.Cause.Error()
}

// Unwrap exposes the underlying cause to errors.Is / errors.As.
func (e *AuthRefreshRequired) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// As walks the error chain and returns the first *AuthRefreshRequired it
// finds, along with true. Returns nil, false otherwise.
//
// Use this instead of a plain errors.As when callers care about acting on
// the marker (force-refresh + retry) rather than just identifying the
// underlying error class.
func As(err error) (*AuthRefreshRequired, bool) {
	if err == nil {
		return nil, false
	}
	var target *AuthRefreshRequired
	if stderrors.As(err, &target) {
		return target, true
	}
	return nil, false
}
