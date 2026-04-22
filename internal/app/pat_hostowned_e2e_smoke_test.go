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
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

// cliOwnedBannerBlacklist is the union of human-readable banner substrings
// that belong exclusively to the CLI-owned PAT authorization path. When
// the host claims PAT ownership (DINGTALK_DWS_AGENTCODE non-empty per
// docs/pat/contract.md §7), NONE of these strings may leak
// onto stderr — the host renders its own card and consumes the
// single-line JSON payload carried by *apperrors.PATError.
//
// Regression guard: several previous drafts accidentally fell through to
// PrintPatAuthError / the browser-open banner / the WaitForPatAuthorization
// ticker on the host-owned branch.
var cliOwnedBannerBlacklist = []string{
	"需要 PAT 授权",
	"在浏览器中打开",
	"⟳",
	"等待用户授权",
	"需要额外授权",
}

func assertNoCLIOwnedBannerLeaks(t *testing.T, got string) {
	t.Helper()
	for _, needle := range cliOwnedBannerBlacklist {
		if strings.Contains(got, needle) {
			t.Fatalf("host-owned stderr must not leak CLI-owned banner %q, got:\n%s",
				needle, got)
		}
	}
}

// TestHostOwnedPATPath_HandlePatAuthCheck_NoBannerLeaks locks in the
// contract that, once DINGTALK_DWS_AGENTCODE declares host ownership,
// handlePatAuthCheck returns a *apperrors.PATError with an
// openClaw-hardwired hostControl block AND writes nothing user-visible
// to stderr. The mock runner must not be invoked either — the host, not
// the CLI, is responsible for retrying after the card completes.
func TestHostOwnedPATPath_HandlePatAuthCheck_NoBannerLeaks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", tmpDir)
	t.Setenv(authpkg.AgentCodeEnv, "cursor")

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner must not be called in host-owned mode")
			return executor.Result{}, nil
		},
	}
	runner := &runtimeRunner{fallback: mock}

	patErr := &apperrors.PATError{RawJSON: makePATErrorJSON("flow-123", "cid-xyz")}

	var buf bytes.Buffer
	_, err := handlePatAuthCheck(context.Background(), runner, executor.Invocation{
		CanonicalProduct: "test",
		Tool:             "t",
	}, patErr, tmpDir, &buf)

	if err == nil {
		t.Fatal("expected *apperrors.PATError in host-owned mode")
	}
	patOut, ok := err.(*apperrors.PATError)
	if !ok {
		t.Fatalf("expected *apperrors.PATError, got %T: %v", err, err)
	}

	assertNoCLIOwnedBannerLeaks(t, buf.String())

	var payload map[string]any
	if err := json.Unmarshal([]byte(patOut.RawJSON), &payload); err != nil {
		t.Fatalf("RawJSON must be valid JSON: %v\nraw=%s", err, patOut.RawJSON)
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		t.Fatalf("expected data object in RawJSON, got %s", patOut.RawJSON)
	}
	if got, _ := data["flowId"].(string); got != "flow-123" {
		t.Fatalf("data.flowId = %q, want flow-123", got)
	}
	hostControl, _ := data["hostControl"].(map[string]any)
	if hostControl == nil {
		t.Fatalf("expected data.hostControl block, got %s", patOut.RawJSON)
	}
	if got, _ := hostControl["clawType"].(string); got != "openClaw" {
		t.Fatalf("hostControl.clawType = %q, want openClaw (open-source build hardwired)", got)
	}
	if _, ok := data["callbacks"]; ok {
		t.Fatalf("host-owned PAT payload must not carry data.callbacks: %#v", data["callbacks"])
	}
	if _, ok := payload["_meta"]; ok {
		t.Fatalf("host-owned PAT payload must not carry top-level _meta: %#v", payload["_meta"])
	}
}

// TestHostOwnedPATPath_RetryWithPatAuthRetry_NoBannerLeaks locks the
// scope-error branch: on host-owned decision, retryWithPatAuthRetry
// MUST NOT call PrintPatAuthError / WaitForPatAuthorization at all.
// Stderr must be empty (byte-for-byte after trim), and the returned
// error is *apperrors.PATError with PAT_SCOPE_AUTH_REQUIRED +
// hostControl.clawType=="openClaw" + missingScope round-tripped.
func TestHostOwnedPATPath_RetryWithPatAuthRetry_NoBannerLeaks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", tmpDir)
	t.Setenv(authpkg.AgentCodeEnv, "cursor")

	scopeErr := &PatScopeError{
		OriginalError: "missing required scope(s): calendar:read",
		Identity:      "user",
		ErrorType:     "missing_scope",
		Message:       "missing required scope(s): calendar:read",
		Hint:          "run `dws auth login --scope \"calendar:read\"` to authorize the missing scope",
		MissingScope:  "calendar:read",
	}

	mock := &mockRunner{
		runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
			t.Fatal("runner must not be called in host-owned mode")
			return executor.Result{}, nil
		},
	}
	runner := &runtimeRunner{fallback: mock}

	var buf bytes.Buffer
	_, err := retryWithPatAuthRetry(context.Background(), runner, executor.Invocation{
		CanonicalProduct: "x",
		Tool:             "y",
	}, scopeErr, tmpDir, &buf)

	if err == nil {
		t.Fatal("expected *apperrors.PATError in host-owned scope mode")
	}
	patOut, ok := err.(*apperrors.PATError)
	if !ok {
		t.Fatalf("expected *apperrors.PATError, got %T: %v", err, err)
	}

	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("host-owned retryWithPatAuthRetry must emit zero human-readable output, got %q", got)
	}
	// Belt-and-suspenders: the blacklist check also guards against any
	// whitespace-only banner that TrimSpace would flatten.
	assertNoCLIOwnedBannerLeaks(t, buf.String())

	var payload map[string]any
	if err := json.Unmarshal([]byte(patOut.RawJSON), &payload); err != nil {
		t.Fatalf("RawJSON must be valid JSON: %v\nraw=%s", err, patOut.RawJSON)
	}
	if code, _ := payload["code"].(string); code != "PAT_SCOPE_AUTH_REQUIRED" {
		t.Fatalf("code = %q, want PAT_SCOPE_AUTH_REQUIRED", code)
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		t.Fatalf("expected data object, got %s", patOut.RawJSON)
	}
	if got, _ := data["missingScope"].(string); got != "calendar:read" {
		t.Fatalf("data.missingScope = %q, want calendar:read", got)
	}
	hostControl, _ := data["hostControl"].(map[string]any)
	if hostControl == nil {
		t.Fatalf("expected data.hostControl block, got %s", patOut.RawJSON)
	}
	if got, _ := hostControl["clawType"].(string); got != "openClaw" {
		t.Fatalf("hostControl.clawType = %q, want openClaw", got)
	}
}

// TestHostOwnedPATPath_EnvEmptyOrWhitespace_FallsBackToCLIBanner is the
// reverse assertion: when DINGTALK_DWS_AGENTCODE is unset / whitespace /
// tab+newline, HostOwnsPATFlow() is false and retryWithPatAuthRetry
// MUST take the CLI-owned branch (PrintPatAuthError →
// WaitForPatAuthorization → timeout error). We pre-cancel ctx so
// WaitForPatAuthorization exits via its <-ctx.Done() case immediately
// without burning the 10-minute poll budget.
//
// The expected terminal error is a non-PATError *apperrors.Error with
// reason=pat_auth_timeout; the stderr buffer MUST contain the CLI-owned
// "需要额外授权" banner that test 1+2 forbid.
func TestHostOwnedPATPath_EnvEmptyOrWhitespace_FallsBackToCLIBanner(t *testing.T) {
	cases := []struct {
		name      string
		agentCode string
	}{
		{name: "env unset", agentCode: ""},
		{name: "env whitespace", agentCode: "   "},
		{name: "env tab_newline", agentCode: "\t\n "},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", tmpDir)
			t.Setenv(authpkg.AgentCodeEnv, tc.agentCode)

			if authpkg.HostOwnsPATFlow() {
				t.Fatalf("precondition: HostOwnsPATFlow() must be false for agentCode=%q", tc.agentCode)
			}

			scopeErr := &PatScopeError{
				OriginalError: "missing required scope(s): calendar:read",
				Identity:      "user",
				ErrorType:     "missing_scope",
				Message:       "missing required scope(s): calendar:read",
				Hint:          "run `dws auth login --scope \"calendar:read\"` to authorize the missing scope",
				MissingScope:  "calendar:read",
			}

			mock := &mockRunner{
				runFunc: func(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
					t.Fatal("runner must not be called when authorization times out")
					return executor.Result{}, nil
				},
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			var buf bytes.Buffer
			_, err := retryWithPatAuthRetry(ctx, mock, executor.Invocation{
				CanonicalProduct: "x",
				Tool:             "y",
			}, scopeErr, tmpDir, &buf)

			if err == nil {
				t.Fatal("expected non-nil error for cancelled CLI-owned path")
			}
			if _, ok := err.(*apperrors.PATError); ok {
				t.Fatalf("CLI-owned fallback must NOT return *apperrors.PATError, got: %v", err)
			}

			var typed *apperrors.Error
			if !errorsAs(err, &typed) {
				t.Fatalf("expected *apperrors.Error, got %T: %v", err, err)
			}
			if typed.Reason != "pat_auth_timeout" {
				t.Errorf("reason = %q, want pat_auth_timeout", typed.Reason)
			}

			if !strings.Contains(buf.String(), "需要额外授权") {
				t.Fatalf("CLI-owned fallback must emit the 需要额外授权 banner, got:\n%s", buf.String())
			}
		})
	}
}

// errorsAs is a thin local shim over errors.As to keep the import
// surface minimal and avoid shadowing the apperrors alias.
func errorsAs(err error, target **apperrors.Error) bool {
	for cur := err; cur != nil; {
		if t, ok := cur.(*apperrors.Error); ok {
			*target = t
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := cur.(unwrapper)
		if !ok {
			return false
		}
		cur = u.Unwrap()
	}
	return false
}
