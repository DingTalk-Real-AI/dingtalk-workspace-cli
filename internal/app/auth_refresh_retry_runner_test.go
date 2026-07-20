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
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func installAuthRefreshRunnerSeams(t *testing.T) {
	t.Helper()
	previousHooks := edition.Get()
	previousCall := runnerCallTool
	previousPreflight := runnerPreflightDocDownload
	previousRefresh := runnerForceRefreshRejectedAccessToken
	previousRetry := runnerExecuteAuthRetry
	previousCapture := runnerCaptureRuntimeFailure
	previousProfile := authpkg.RuntimeProfile()

	pluginAuthMu.Lock()
	previousPlugins := pluginAuthRegistry
	pluginAuthRegistry = make(map[string]*PluginAuth)
	pluginAuthMu.Unlock()

	runnerPreflightDocDownload = func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	}
	runnerCaptureRuntimeFailure = func(executor.Invocation, error, error) {}
	authpkg.SetRuntimeProfile("")
	runtimeTokenManager.Invalidate()
	t.Setenv("DWS_CONFIG_DIR", "")
	t.Setenv("DWS_DEBUG_AUTH", "0")

	t.Cleanup(func() {
		edition.Override(previousHooks)
		runnerCallTool = previousCall
		runnerPreflightDocDownload = previousPreflight
		runnerForceRefreshRejectedAccessToken = previousRefresh
		runnerExecuteAuthRetry = previousRetry
		runnerCaptureRuntimeFailure = previousCapture
		authpkg.SetRuntimeProfile(previousProfile)
		runtimeTokenManager.Invalidate()
		pluginAuthMu.Lock()
		pluginAuthRegistry = previousPlugins
		pluginAuthMu.Unlock()
	})
}

func authRefreshTestRunner(flags *GlobalFlags) *runtimeRunner {
	return &runtimeRunner{
		transport:   transport.NewClient(nil),
		globalFlags: flags,
		auditSink:   audit.NopSink{},
	}
}

func authRefreshTestInvocation() executor.Invocation {
	return executor.Invocation{
		CanonicalProduct: "auth-retry-test-product",
		Tool:             "test_tool",
		Params:           map[string]any{"value": "safe"},
	}
}

func authRefreshTokenHooks(configDir string, token *string, classify func(map[string]any) error) *edition.Hooks {
	return &edition.Hooks{
		ConfigDir: func() string { return configDir },
		TokenProvider: func(context.Context, func() (string, error)) (string, error) {
			return *token, nil
		},
		ClassifyToolResult: classify,
	}
}

func TestCrossPlatformCoverageRunnerRetriesEditionAuthMarkerOnce(t *testing.T) {
	installAuthRefreshRunnerSeams(t)
	configDir := t.TempDir()
	token := "old-access"
	rejection := apperrors.NewAuth("server rejected access token", apperrors.WithReason("access_token_rejected"))
	edition.Override(authRefreshTokenHooks(configDir, &token, func(content map[string]any) error {
		if expired, _ := content["expired"].(bool); expired {
			return &authretry.AuthRefreshRequired{Cause: rejection}
		}
		return nil
	}))

	var callTokens []string
	runnerCallTool = func(client *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		callTokens = append(callTokens, client.AuthToken)
		if len(callTokens) == 1 {
			return transport.ToolCallResult{Content: map[string]any{"expired": true}}, nil
		}
		return transport.ToolCallResult{Content: map[string]any{"value": "ok"}}, nil
	}
	refreshCalls := 0
	runnerForceRefreshRejectedAccessToken = func(_ context.Context, gotDir, rejected string) (string, error) {
		refreshCalls++
		if gotDir != configDir || rejected != "old-access" {
			t.Fatalf("refresh input = dir %q token %q", gotDir, rejected)
		}
		token = "new-access"
		return token, nil
	}

	result, err := authRefreshTestRunner(nil).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation())
	if err != nil {
		t.Fatal(err)
	}
	if refreshCalls != 1 || len(callTokens) != 2 || callTokens[0] != "old-access" || callTokens[1] != "new-access" {
		t.Fatalf("refreshes=%d call tokens=%v", refreshCalls, callTokens)
	}
	content, _ := result.Response["content"].(map[string]any)
	if content["value"] != "ok" || content["success"] != true {
		t.Fatalf("result content = %#v", content)
	}
}

func TestCrossPlatformCoverageRunnerRefreshFailurePreservesBothCausesAndSafeLog(t *testing.T) {
	installAuthRefreshRunnerSeams(t)
	t.Setenv("DWS_DEBUG_AUTH", "1")
	configDir := t.TempDir()
	token := "old-access"
	rejection := apperrors.NewAuth("server rejected access token", apperrors.WithReason("access_token_rejected"))
	edition.Override(authRefreshTokenHooks(configDir, &token, func(map[string]any) error {
		return &authretry.AuthRefreshRequired{Cause: rejection}
	}))
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{Content: map[string]any{"expired": true}}, nil
	}
	refreshErr := errors.New(`oauth refresh response parse failed: body={"access_token":"access-token-secret","refresh_token":"refresh-token-secret","uid":"uid-secret-value"}`)
	runnerForceRefreshRejectedAccessToken = func(context.Context, string, string) (string, error) {
		return "", refreshErr
	}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	_, err := authRefreshTestRunner(nil).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation())
	if !errors.Is(err, rejection) || !errors.Is(err, refreshErr) {
		t.Fatalf("error = %v, want rejection and refresh causes", err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAuth || typed.Reason != "auth_refresh_failed" || typed.Operation != "auth/token/refresh" {
		t.Fatalf("refresh envelope = %#v", typed)
	}
	var rendered bytes.Buffer
	if printErr := apperrors.PrintJSON(&rendered, err); printErr != nil {
		t.Fatal(printErr)
	}
	for _, want := range []string{`"category": "auth"`, `"reason": "auth_refresh_failed"`, `"operation": "auth/token/refresh"`} {
		if !strings.Contains(rendered.String(), want) {
			t.Fatalf("structured stderr missing %s: %s", want, rendered.String())
		}
	}
	for _, secret := range []string{"access-token-secret", "refresh-token-secret", "uid-secret-value"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(logs.String(), secret) || strings.Contains(rendered.String(), secret) {
			t.Fatalf("auth output leaked %q: error=%q logs=%s stderr=%s", secret, err, logs.String(), rendered.String())
		}
	}
	for _, want := range []string{"auth.runtime.refresh.failed", "auth.runtime.refresh.failed.detail", "force_refresh_rejected_token", "error_type"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("safe refresh log missing %q: %s", want, logs.String())
		}
	}
}

func TestCrossPlatformCoverageRunnerSecondEditionMarkerReturnsSecondCause(t *testing.T) {
	installAuthRefreshRunnerSeams(t)
	configDir := t.TempDir()
	token := "old-access"
	firstCause := errors.New("first rejection")
	secondCause := errors.New("second rejection")
	edition.Override(authRefreshTokenHooks(configDir, &token, func(content map[string]any) error {
		attempt, _ := content["attempt"].(int)
		if attempt == 1 {
			return &authretry.AuthRefreshRequired{Cause: firstCause}
		}
		return &authretry.AuthRefreshRequired{Cause: secondCause}
	}))
	calls := 0
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		calls++
		return transport.ToolCallResult{Content: map[string]any{"attempt": calls}}, nil
	}
	refreshCalls := 0
	runnerForceRefreshRejectedAccessToken = func(context.Context, string, string) (string, error) {
		refreshCalls++
		token = "new-access"
		return token, nil
	}

	_, err := authRefreshTestRunner(nil).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation())
	if !errors.Is(err, secondCause) || errors.Is(err, firstCause) {
		t.Fatalf("error = %v, want only second rejection cause", err)
	}
	if calls != 2 || refreshCalls != 1 {
		t.Fatalf("calls=%d refreshes=%d", calls, refreshCalls)
	}
}

func TestCrossPlatformCoverageRunnerOnAuthErrorOnlyRetriesExactUnauthorized(t *testing.T) {
	t.Run("http 401 marker retries once", func(t *testing.T) {
		installAuthRefreshRunnerSeams(t)
		configDir := t.TempDir()
		token := "old-access"
		rejection := errors.New("transport rejected token")
		hookCalls := 0
		hooks := authRefreshTokenHooks(configDir, &token, nil)
		hooks.OnAuthError = func(string, error) error {
			hookCalls++
			return &authretry.AuthRefreshRequired{Cause: rejection}
		}
		edition.Override(hooks)
		calls := 0
		var callTokens []string
		runnerCallTool = func(client *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
			calls++
			callTokens = append(callTokens, client.AuthToken)
			if calls == 1 {
				return transport.ToolCallResult{}, apperrors.NewAuth("unauthorized", apperrors.WithReason("http_401"))
			}
			return transport.ToolCallResult{Content: map[string]any{"value": "ok"}}, nil
		}
		refreshCalls := 0
		runnerForceRefreshRejectedAccessToken = func(context.Context, string, string) (string, error) {
			refreshCalls++
			token = "new-access"
			return token, nil
		}

		if _, err := authRefreshTestRunner(nil).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation()); err != nil {
			t.Fatal(err)
		}
		if hookCalls != 1 || refreshCalls != 1 || calls != 2 || strings.Join(callTokens, ",") != "old-access,new-access" {
			t.Fatalf("hook=%d refresh=%d calls=%d tokens=%v", hookCalls, refreshCalls, calls, callTokens)
		}
	})

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "http 403", err: apperrors.NewAuth("forbidden", apperrors.WithReason("http_403"))},
		{name: "ordinary auth", err: apperrors.NewAuth("load failed", apperrors.WithReason("auth_load_failed"))},
	} {
		t.Run(tc.name+" does not enter hook", func(t *testing.T) {
			installAuthRefreshRunnerSeams(t)
			configDir := t.TempDir()
			token := "old-access"
			hookCalls := 0
			hooks := authRefreshTokenHooks(configDir, &token, nil)
			hooks.OnAuthError = func(string, error) error {
				hookCalls++
				return &authretry.AuthRefreshRequired{Cause: errors.New("must not run")}
			}
			edition.Override(hooks)
			runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
				return transport.ToolCallResult{}, tc.err
			}
			refreshCalls := 0
			runnerForceRefreshRejectedAccessToken = func(context.Context, string, string) (string, error) {
				refreshCalls++
				return "", nil
			}

			_, err := authRefreshTestRunner(nil).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation())
			if !errors.Is(err, tc.err) || hookCalls != 0 || refreshCalls != 0 {
				t.Fatalf("error=%v hook=%d refresh=%d", err, hookCalls, refreshCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageRunnerDoesNotRefreshExplicitTokenMarker(t *testing.T) {
	installAuthRefreshRunnerSeams(t)
	rejection := errors.New("explicit token rejected")
	edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error {
		return &authretry.AuthRefreshRequired{Cause: rejection}
	}})
	calls := 0
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		calls++
		return transport.ToolCallResult{Content: map[string]any{"expired": true}}, nil
	}
	refreshCalls := 0
	runnerForceRefreshRejectedAccessToken = func(context.Context, string, string) (string, error) {
		refreshCalls++
		return "", nil
	}

	_, err := authRefreshTestRunner(&GlobalFlags{Token: "explicit-token"}).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation())
	if !errors.Is(err, rejection) || calls != 1 || refreshCalls != 0 {
		t.Fatalf("error=%v calls=%d refresh=%d", err, calls, refreshCalls)
	}
}

func TestCrossPlatformCoverageRunnerRetriesPreflightEditionMarkerOnce(t *testing.T) {
	installAuthRefreshRunnerSeams(t)
	configDir := t.TempDir()
	token := "old-access"
	rejection := errors.New("preflight token rejected")
	edition.Override(authRefreshTokenHooks(configDir, &token, nil))
	preflightCalls := 0
	runnerPreflightDocDownload = func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		preflightCalls++
		if preflightCalls == 1 {
			return &authretry.AuthRefreshRequired{Cause: rejection}
		}
		return nil
	}
	toolCalls := 0
	runnerCallTool = func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		toolCalls++
		return transport.ToolCallResult{Content: map[string]any{"value": "ok"}}, nil
	}
	refreshCalls := 0
	runnerForceRefreshRejectedAccessToken = func(context.Context, string, string) (string, error) {
		refreshCalls++
		token = "new-access"
		return token, nil
	}

	if _, err := authRefreshTestRunner(nil).executeInvocation(context.Background(), "https://example.test", authRefreshTestInvocation()); err != nil {
		t.Fatal(err)
	}
	if preflightCalls != 2 || toolCalls != 1 || refreshCalls != 1 {
		t.Fatalf("preflights=%d tools=%d refreshes=%d", preflightCalls, toolCalls, refreshCalls)
	}
}
