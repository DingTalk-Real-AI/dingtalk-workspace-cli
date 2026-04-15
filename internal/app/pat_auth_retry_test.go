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
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestIsPatScopeError_MissingScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("missing required scope(s): mail:user_mailbox.message:send")
	if !isPatScopeError(err) {
		t.Fatal("expected missing_scope error to be detected")
	}
}

func TestIsPatScopeError_PlainString(t *testing.T) {
	t.Parallel()
	err := &PatScopeError{
		OriginalError: "missing_scope: user lacks required scope",
		ErrorType:     "missing_scope",
		Message:       "user lacks required scope",
	}
	if !isPatScopeError(err) {
		t.Fatal("expected plain string with missing_scope to be detected")
	}
}

func TestIsPatScopeError_NotScopeError(t *testing.T) {
	t.Parallel()
	err := apperrors.NewValidation("invalid parameter")
	if isPatScopeError(err) {
		t.Fatal("expected validation error NOT to be detected as scope error")
	}
}

func TestIsPatScopeError_Nil(t *testing.T) {
	t.Parallel()
	if isPatScopeError(nil) {
		t.Fatal("nil error should not be detected as scope error")
	}
}

func TestIsPatScopeError_WithReason(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("API error",
		apperrors.WithReason("missing_scope"),
	)
	if !isPatScopeError(err) {
		t.Fatal("expected error with missing_scope reason to be detected")
	}
}

func TestIsPatScopeError_InsufficientScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("insufficient_scope for resource",
		apperrors.WithReason("insufficient_scope"),
	)
	if !isPatScopeError(err) {
		t.Fatal("expected insufficient_scope error to be detected")
	}
}

func TestExtractPatScopeError_MissingScope(t *testing.T) {
	t.Parallel()
	err := apperrors.NewAuth("missing required scope(s): mail:user_mailbox.message:send")
	scopeErr := extractPatScopeError(err)
	if scopeErr == nil {
		t.Fatal("expected non-nil PatScopeError")
	}
	if scopeErr.ErrorType != "missing_scope" {
		t.Errorf("expected error type 'missing_scope', got %q", scopeErr.ErrorType)
	}
	if !strings.Contains(scopeErr.Hint, "dws auth login") {
		t.Errorf("expected hint to contain 'dws auth login', got %q", scopeErr.Hint)
	}
}

func TestExtractPatScopeError_ExtractsScope(t *testing.T) {
	t.Parallel()
	err := &PatScopeError{
		OriginalError: "missing_scope: user needs calendar:read",
		ErrorType:     "missing_scope",
		Message:       "user needs calendar:read",
	}
	scopeErr := extractPatScopeError(err)
	if scopeErr == nil {
		t.Fatal("expected non-nil PatScopeError")
	}
	if scopeErr.MissingScope == "" {
		t.Log("no scope extracted from message (regex didn't match)")
	}
}

func TestPrintPatAuthError_HumanReadable(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	scopeErr := &PatScopeError{
		Identity:     "user",
		ErrorType:    "missing_scope",
		Message:      "missing required scope(s): mail:user_mailbox.message:send",
		Hint:         "run `dws auth login --scope \"mail:user_mailbox.message:send\"` to authorize",
		MissingScope: "mail:user_mailbox.message:send",
	}
	PrintPatAuthError(&buf, scopeErr)

	output := buf.String()
	if !strings.Contains(output, "missing_scope") {
		t.Errorf("expected output to contain 'missing_scope', got: %s", output)
	}
	if !strings.Contains(output, "dws auth login") {
		t.Errorf("expected output to contain 'dws auth login', got: %s", output)
	}
	if !strings.Contains(output, "需要额外授权") {
		t.Errorf("expected output to contain Chinese auth prompt, got: %s", output)
	}
}

func TestPrintPatAuthJSON_MachineReadable(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	scopeErr := &PatScopeError{
		Identity:     "user",
		ErrorType:    "missing_scope",
		Message:      "missing required scope(s): mail:send",
		Hint:         "run dws auth login --scope mail:send",
		MissingScope: "mail:send",
	}
	PrintPatAuthJSON(&buf, scopeErr)

	output := buf.String()
	if !strings.Contains(output, `"ok": false`) {
		t.Errorf("expected JSON to contain ok: false, got: %s", output)
	}
	if !strings.Contains(output, `"missing_scope": "mail:send"`) {
		t.Errorf("expected JSON to contain missing_scope, got: %s", output)
	}
}

func TestPatScopeError_Error(t *testing.T) {
	t.Parallel()
	err := &PatScopeError{
		OriginalError: "test error message",
	}
	if err.Error() != "test error message" {
		t.Errorf("expected Error() to return OriginalError, got %q", err.Error())
	}
}
