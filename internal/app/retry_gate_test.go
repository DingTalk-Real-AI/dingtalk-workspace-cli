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
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func retryGateTestRunner() *runtimeRunner {
	client := transport.NewClient(nil)
	return &runtimeRunner{
		transport:   client,
		globalFlags: &GlobalFlags{Token: "test-token"},
		auditSink:   audit.NopSink{},
	}
}

func TestCrossPlatformCoverageRuntimeRetryGateControlsTransportAndAPIHint(t *testing.T) {
	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
	testseam.Swap(t, &runnerCaptureRuntimeFailure, func(executor.Invocation, error, error) {})

	tests := []struct {
		name          string
		decision      *contract.RetryDecision
		wantRetries   int
		wantRetryable bool
	}{
		{name: "missing contract fails closed", wantRetries: 0, wantRetryable: false},
		{
			name: "conditional contract without key fails closed",
			decision: &contract.RetryDecision{
				EffectiveIdempotency: "non_idempotent",
				SafeToRetry:          false,
				Reason:               "deduplication_key_missing",
			},
			wantRetries:   0,
			wantRetryable: false,
		},
		{
			name: "resolved idempotent contract preserves retry",
			decision: &contract.RetryDecision{
				EffectiveIdempotency: "idempotent",
				SafeToRetry:          true,
				Reason:               "deduplication_key_present",
			},
			wantRetries:   1,
			wantRetryable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRetries := -1
			testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
				gotRetries = client.MaxRetries
				return transport.ToolCallResult{}, apperrors.NewAPI(
					"server unavailable",
					apperrors.WithReason("http_503"),
					apperrors.WithRetryable(true),
					apperrors.WithRetryAfterSeconds(3),
				)
			})

			runner := retryGateTestRunner()
			_, err := runner.executeInvocation(context.Background(), "https://example.test", executor.Invocation{
				CanonicalProduct: "chat",
				Tool:             "send_message",
				Params:           map[string]any{"uuid": "stable-key"},
				Retry:            tt.decision,
			})
			if gotRetries != tt.wantRetries {
				t.Fatalf("transport MaxRetries = %d, want %d", gotRetries, tt.wantRetries)
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T %v, want typed API error", err, err)
			}
			if !typed.RetryableSet || typed.Retryable != tt.wantRetryable {
				t.Fatalf("retryable = (%v, %v), want (true, %v)", typed.RetryableSet, typed.Retryable, tt.wantRetryable)
			}
			if tt.wantRetryable {
				if typed.RetryAfterSeconds == nil || *typed.RetryAfterSeconds != 3 {
					t.Fatalf("safe retry_after_seconds = %v, want 3", typed.RetryAfterSeconds)
				}
			} else if typed.RetryAfterSeconds != nil || typed.NextRetryAt != nil {
				t.Fatalf("unsafe retry kept backoff hints: after=%v at=%v", typed.RetryAfterSeconds, typed.NextRetryAt)
			}
			if runner.transport.MaxRetries != 1 {
				t.Fatalf("shared transport MaxRetries = %d, want unchanged 1", runner.transport.MaxRetries)
			}
		})
	}
}

func TestCrossPlatformCoverageRuntimeRetryGateDoesNotClampAuthErrors(t *testing.T) {
	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
	testseam.Swap(t, &runnerCaptureRuntimeFailure, func(executor.Invocation, error, error) {})
	authErr := apperrors.NewAuth("expired", apperrors.WithRetryable(true))
	testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
		return transport.ToolCallResult{}, authErr
	})

	_, err := retryGateTestRunner().executeInvocation(context.Background(), "https://example.test", executor.Invocation{
		CanonicalProduct: "chat",
		Tool:             "send_message",
		Params:           map[string]any{},
	})
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T %v, want typed auth error", err, err)
	}
	if typed.Category != apperrors.CategoryAuth || !typed.RetryableSet || !typed.Retryable {
		t.Fatalf("auth retryability was changed: %#v", typed)
	}
}

func TestCrossPlatformCoverageRuntimeRetryDecisionPrecedesDryRun(t *testing.T) {
	resolverCalls := 0
	runner := retryGateTestRunner()
	runner.globalFlags.DryRun = true
	runner.resolveToolCallRetry = func(productID, rpcName string, args map[string]any) contract.RetryDecision {
		resolverCalls++
		if productID != "chat" || rpcName != "send_message" || args["uuid"] != "stable-key" {
			t.Fatalf("resolver input = %q/%q %#v", productID, rpcName, args)
		}
		return contract.RetryDecision{
			EffectiveIdempotency: "idempotent",
			SafeToRetry:          true,
			Reason:               "deduplication_key_present",
		}
	}

	result, err := runner.Run(context.Background(), executor.NewHelperInvocation(
		"chat send",
		"chat",
		"send_message",
		map[string]any{"uuid": "stable-key"},
	))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resolverCalls != 1 || result.Invocation.Retry == nil || !result.Invocation.Retry.SafeToRetry {
		t.Fatalf("dry-run retry decision = %#v, resolver calls = %d", result.Invocation.Retry, resolverCalls)
	}
	if got, ok := result.Response["retry"].(*contract.RetryDecision); !ok || !got.SafeToRetry || got.EffectiveIdempotency != "idempotent" {
		t.Fatalf("dry-run response retry = %#v", result.Response["retry"])
	}

	explicit := contract.RetryDecision{EffectiveIdempotency: "unknown", SafeToRetry: false, Reason: "explicit"}
	invocation := executor.NewHelperInvocation("chat send", "chat", "send_message", nil)
	invocation.Retry = &explicit
	result, err = runner.Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("Run(explicit) error = %v", err)
	}
	if resolverCalls != 1 || result.Invocation.Retry == nil || result.Invocation.Retry.Reason != "explicit" {
		t.Fatalf("explicit retry decision was overwritten: %#v, resolver calls = %d", result.Invocation.Retry, resolverCalls)
	}
}

func TestCrossPlatformCoverageExecuteInvocationDryRunIncludesRetryDecision(t *testing.T) {
	decision := contract.RetryDecision{
		EffectiveIdempotency: "idempotent",
		SafeToRetry:          true,
		Reason:               "deduplication_key_present",
	}
	result, err := retryGateTestRunner().executeInvocation(context.Background(), "https://example.test", executor.Invocation{
		CanonicalProduct: "chat",
		Tool:             "send_message",
		DryRun:           true,
		Retry:            &decision,
	})
	if err != nil {
		t.Fatalf("executeInvocation() error = %v", err)
	}
	got, ok := result.Response["retry"].(*contract.RetryDecision)
	if !ok || got != &decision || !got.SafeToRetry {
		t.Fatalf("dry-run response retry = %#v, want original decision", result.Response["retry"])
	}
}

func TestCrossPlatformCoverageToolCallerDryRunIncludesRetryDecision(t *testing.T) {
	runner := retryGateTestRunner()
	runner.resolveToolCallRetry = func(_ string, _ string, args map[string]any) contract.RetryDecision {
		if key, _ := args["uuid"].(string); key == "" {
			return contract.RetryDecision{
				EffectiveIdempotency: "non_idempotent",
				Reason:               "deduplication_key_missing",
			}
		}
		return contract.RetryDecision{
			EffectiveIdempotency: "idempotent",
			SafeToRetry:          true,
			Reason:               "deduplication_key_present",
		}
	}
	var captured executor.Invocation
	testseam.Swap(t, &toolCallerDryRun, func(ctx context.Context, invocation executor.Invocation) (executor.Result, error) {
		captured = invocation
		return (executor.EchoRunner{}).Run(ctx, invocation)
	})

	caller := newToolCallerAdapter(runner, &GlobalFlags{DryRun: true})
	result, err := caller.CallTool(context.Background(), "chat", "send_message", map[string]any{"uuid": "stable-key"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if captured.Retry == nil || !captured.Retry.SafeToRetry || captured.Retry.EffectiveIdempotency != "idempotent" {
		t.Fatalf("captured retry decision = %#v", captured.Retry)
	}
	if len(result.Content) != 1 || !bytes.Contains([]byte(result.Content[0].Text), []byte(`"effective_idempotency":"idempotent"`)) {
		t.Fatalf("ToolResult does not expose effective idempotency: %#v", result)
	}

	result, err = caller.CallTool(context.Background(), "chat", "send_message", map[string]any{"content": "hello"})
	if err != nil {
		t.Fatalf("CallTool(without uuid) error = %v", err)
	}
	if captured.Retry == nil || captured.Retry.SafeToRetry || captured.Retry.EffectiveIdempotency != "non_idempotent" {
		t.Fatalf("missing-key retry decision = %#v", captured.Retry)
	}
	if len(result.Content) != 1 || !bytes.Contains([]byte(result.Content[0].Text), []byte(`"effective_idempotency":"non_idempotent"`)) {
		t.Fatalf("ToolResult does not expose missing-key idempotency: %#v", result)
	}
}

func TestCrossPlatformCoverageRuntimeSafeRetryReusesRequestBody(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll(request) error = %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), body...))
		attempt := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":{"content":{"messageId":"m1"}}}`)
	}))
	defer server.Close()

	runner := retryGateTestRunner()
	runner.transport = transport.NewClient(server.Client())
	runner.transport.RetryDelay = 0
	runner.transport.RetryMaxDelay = 0
	runner.auditSink = audit.NopSink{}
	decision := contract.RetryDecision{
		EffectiveIdempotency: "idempotent",
		SafeToRetry:          true,
		Reason:               "deduplication_key_present",
	}
	result, err := runner.executeInvocation(context.Background(), server.URL, executor.Invocation{
		CanonicalProduct: "chat",
		Tool:             "send_message",
		Params:           map[string]any{"uuid": "stable-key", "content": "hello"},
		Retry:            &decision,
	})
	if err != nil {
		t.Fatalf("executeInvocation() error = %v", err)
	}
	if !result.Invocation.Implemented {
		t.Fatal("successful retry was not marked implemented")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("request attempts = %d, want 2", len(bodies))
	}
	if !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("retry body changed:\nfirst:  %s\nsecond: %s", bodies[0], bodies[1])
	}
}
