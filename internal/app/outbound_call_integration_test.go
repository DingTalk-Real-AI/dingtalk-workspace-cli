// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

const outboundCallEndpoint = "https://mcp-gw.dingtalk.com/server/1b382233a46b7208b9adeb2396bcaac5ffc344b97502f11dea0a3eaa13853e5f"

func TestCrossPlatformCoverageOutboundCallLongTailRegistration(t *testing.T) {
	root := NewRootCommand()
	product, remaining, err := root.Find([]string{"outbound-call"})
	if err != nil || len(remaining) != 0 || product == root || !product.Hidden {
		t.Fatalf("outbound-call must register as a hidden helper product: command=%v remaining=%v err=%v", product, remaining, err)
	}
	var stream bytes.Buffer
	root.SetOut(&stream)
	root.SetErr(&stream)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stream.String(), "outbound-call") {
		t.Fatalf("long-tail product leaked into root help: %s", &stream)
	}
}

func TestCrossPlatformCoverageOutboundCallSchema(t *testing.T) {
	root := NewRootCommand()
	for _, name := range []string{"create", "detail"} {
		var fullResult any
		for _, compact := range []bool{false, true} {
			var stream bytes.Buffer
			root.SetOut(&stream)
			root.SetErr(&stream)
			compactArg := "--compact=false"
			if compact {
				compactArg = "--compact=true"
			}
			root.SetArgs([]string{"schema", "--cli-path", "outbound-call " + name, "--format", "json", compactArg})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			var leaf map[string]any
			if err := json.Unmarshal(stream.Bytes(), &leaf); err != nil {
				t.Fatalf("schema: %v %s", err, &stream)
			}
			result, ok := leaf["result"].(map[string]any)
			if !ok || len(result["outcomes"].([]any)) == 0 {
				t.Fatalf("result declaration missing: %#v", leaf)
			}
			schema := result["data_schema"].(map[string]any)
			if !reflect.DeepEqual(schema["type"], []any{"object", "array", "string", "number", "boolean"}) {
				t.Fatalf("receipt schema lost the non-null JSON contract: %#v", schema)
			}
			if !compact {
				fullResult = result
			} else if !reflect.DeepEqual(result, fullResult) {
				t.Fatal("compact result differs from full leaf")
			}
			params := leaf["parameters"].(map[string]any)
			mappings := map[string]string{"task-id": "taskId"}
			confirmation := "not_required"
			if name == "create" {
				mappings = map[string]string{"callee": "calleeInfos[0].callee", "task": "calleeInfos[0].prompt", "opening": "calleeInfos[0].greeting"}
				confirmation = "user_required"
			}
			for flag, property := range mappings {
				param := params[flag].(map[string]any)
				if param["required"] != true || (!compact && param["property"] != property) {
					t.Fatalf("parameter %s lost required/mapping: %#v", flag, param)
				}
			}
			if leaf["confirmation"] != confirmation {
				t.Fatalf("confirmation = %#v, want %s", leaf["confirmation"], confirmation)
			}
		}
	}
}

func TestCrossPlatformCoverageOutboundCallRootDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		args          []string
		response      map[string]any
		wireErr       error
		calls, code   int
	}{
		{"create", "pending", []string{"create", "--callee", "fixture-user-id", "--task", " p ", "--opening", "g", "--yes"}, map[string]any{"taskId": "t"}, nil, 1, 0},
		{"detail", "success", []string{"detail", "--task-id", "t"}, map[string]any{"taskId": "t", "callStatus": 4}, nil, 1, 0},
		{"dry-run", "success", []string{"create", "--callee", "u", "--task", "p", "--opening", "g", "--dry-run"}, nil, nil, 0, 0},
		{"unconfirmed", "", []string{"create", "--callee", "u", "--task", "p", "--opening", "g"}, nil, nil, 0, 3},
		{"upstream-error", "", []string{"detail", "--task-id", "t"}, map[string]any{"success": false, "errorMsg": "private business content"}, nil, 1, 1},
		{"timeout", "failure", []string{"create", "--callee", "u", "--task", "p", "--opening", "g", "--yes"}, nil, context.DeadlineExceeded, 1, 1},
		{"detail-timeout", "", []string{"detail", "--task-id", "t"}, nil, context.DeadlineExceeded, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Setenv("DINGTALK_OUTBOUND_CALL_MCP_URL", "")
			testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
				return AccessTokenSnapshot{AccessToken: "fixture-token"}, nil
			})
			stubOutboundCallTokenPrefetch(t, tc.calls > 0)
			var logs bytes.Buffer
			client := transport.NewClient(nil)
			client.FileLogger = slog.New(slog.NewTextHandler(&logs, nil))
			client.SnapshotRecorder = outboundCallSnapshotTrap{t}
			calls := 0
			testseam.Swap(t, &runnerCallTool, func(invocationClient *transport.Client, _ context.Context, endpoint, tool string, args map[string]any) (transport.ToolCallResult, error) {
				calls++
				if invocationClient.MaxRetries != 0 || invocationClient.FileLogger != nil || invocationClient.SnapshotRecorder != nil {
					t.Fatal("raw diagnostics or transport retries enabled")
				}
				if endpoint != outboundCallEndpoint {
					t.Fatalf("wrong runtime endpoint: %s", endpoint)
				}
				if tc.name == "create" {
					raw, _ := json.Marshal(args)
					if tool != "create_dingtalk_audio_talk_call" || string(raw) != `{"calleeInfos":[{"callee":"fixture-user-id","greeting":"g","prompt":" p "}]}` {
						t.Fatalf("wrong create dispatch: %s %s", tool, raw)
					}
				} else if tc.args[0] == "detail" && (tool != "outbound_call_detail" || !reflect.DeepEqual(args, map[string]any{"taskId": "t"})) {
					t.Fatalf("wrong detail dispatch: %s %#v", tool, args)
				}
				return transport.ToolCallResult{Content: tc.response}, tc.wireErr
			})
			envelope, err, code := executeOutboundCallTestCommand(t, func(flags *GlobalFlags) executor.Runner {
				return &runtimeRunner{transport: client, globalFlags: flags, auditSink: audit.NopSink{}}
			}, append([]string{"outbound-call"}, tc.args...)...)
			if code != tc.code || calls != tc.calls || (tc.outcome != "" && string(envelope.Outcome) != tc.outcome) {
				t.Fatalf("code=%d calls=%d outcome=%s err=%v", code, calls, envelope.Outcome, err)
			}
			if client.FileLogger == nil || client.SnapshotRecorder == nil {
				t.Fatal("shared transport mutated")
			}
			if strings.Contains(logs.String(), "private business content") {
				t.Fatal("upstream content leaked into logs")
			}
			if tc.name == "upstream-error" {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != "business_error" || !strings.Contains(typed.Message, "private business content") {
					t.Fatal("privacy adapter discarded the structured error")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageOutboundCallTelemetryPrivacy(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "1")
	t.Cleanup(CloseFileLogger)
	testseam.Swap(t, &os.Args, []string{"dws", "outbound-call", "detail", "--task-id", "t", "--format", "json"})
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result { return runtimecontext.Result{} })
	testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
	testseam.Swap(t, &rootExecuteCommand, func(root *cobra.Command) (*cobra.Command, error) {
		leaf, _, err := root.Find([]string{"outbound-call", "detail"})
		if err != nil {
			t.Fatal(err)
		}
		return leaf, apperrors.NewAPI("private conversation text")
	})
	capture, err := os.CreateTemp(t.TempDir(), "output-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = capture.Close() })
	testseam.Swap(t, &os.Stdout, capture)
	testseam.Swap(t, &os.Stderr, capture)
	if code, path, summary := ExecuteWithTelemetry(); code != 1 || path != "outbound-call detail" || summary != "outbound call failed" {
		t.Fatalf("telemetry privacy changed: code=%d path=%q summary=%q", code, path, summary)
	}
}

type outboundCallSnapshotTrap struct{ t *testing.T }

func (s outboundCallSnapshotTrap) RecordJSONRPC(string, string, []byte, []byte) string {
	s.t.Fatal("outbound business content entered raw snapshot recorder")
	return ""
}

func TestCrossPlatformCoverageOutboundCallArgvRedaction(t *testing.T) {
	for _, flag := range []string{"--callee", "--task", "--opening"} {
		for _, args := range [][]string{{"dws", "outbound-call", "create", flag, "private-value"}, {"dws", "outbound-call", "create", flag + "=private-value"}} {
			if sanitized := SanitizeCommand(args); strings.Contains(sanitized, "private-value") || !strings.Contains(sanitized, "***") {
				t.Fatalf("sensitive argv leaked: %s", sanitized)
			}
		}
	}
}

func TestCrossPlatformCoverageOutboundCallHTTPFailures(t *testing.T) {
	const accepted = `{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"{\"code\":\"0\",\"data\":{\"taskId\":\"fixture-task\"}}"}],"structuredContent":{"code":"0","data":{"taskId":"fixture-task"}}}}`
	for _, tc := range []struct {
		name, verb, body, subtype, outcome string
		status, exitCode                   int
		retryable                          bool
	}{
		{name: "accepted", verb: "create", status: 200, body: accepted, outcome: "pending"},
		{name: "create_unavailable", verb: "create", status: 503, subtype: "submission_unknown", outcome: "failure", exitCode: 1},
		{name: "detail_unavailable", verb: "detail", status: 503, subtype: "http_503", exitCode: 1, retryable: true},
		{name: "forbidden", verb: "create", status: 403, subtype: "http_403", exitCode: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				var request struct{ Method string }
				if err := json.NewDecoder(req.Body).Decode(&request); err != nil || request.Method != "tools/call" {
					t.Errorf("unexpected request: method=%q err=%v", request.Method, err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			t.Setenv("DINGTALK_OUTBOUND_CALL_MCP_URL", server.URL)
			testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
				return AccessTokenSnapshot{AccessToken: "fixture-token"}, nil
			})
			stubOutboundCallTokenPrefetch(t, true)
			args := []string{"outbound-call", "detail", "--task-id", "fixture-task"}
			if tc.verb == "create" {
				args = []string{"outbound-call", "create", "--callee", "fixture-user", "--task", "fixture-task", "--opening", "fixture-opening", "--yes"}
			}
			envelope, err, code := executeOutboundCallTestCommand(t, func(flags *GlobalFlags) executor.Runner {
				return &runtimeRunner{transport: transport.NewClient(server.Client()), globalFlags: flags, auditSink: audit.NopSink{}}
			}, args...)
			if code != tc.exitCode || calls.Load() != 1 {
				t.Fatalf("code=%d calls=%d err=%v", code, calls.Load(), err)
			}
			if tc.outcome == "" {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != tc.subtype || typed.Retryable != tc.retryable {
					t.Fatalf("classification changed: %v", err)
				}
				return
			}
			if err != nil || string(envelope.Outcome) != tc.outcome {
				t.Fatalf("outcome=%s err=%v", envelope.Outcome, err)
			}
			if tc.subtype != "" && (envelope.Error == nil || envelope.Error.Subtype != tc.subtype || envelope.Error.Retryable || !strings.Contains(envelope.Error.Hint, "不要自动重试")) {
				t.Fatalf("unsafe failure: %#v", envelope.Error)
			}
			if tc.outcome == "pending" && (envelope.Meta.Operation == nil || envelope.Meta.Operation.ID != "fixture-task") {
				t.Fatalf("task identity lost: %#v", envelope.Meta.Operation)
			}
		})
	}
}

func TestCrossPlatformCoverageOutboundCallProfileBoundary(t *testing.T) {
	for _, tc := range []struct {
		verb, profile string
		dryRun        bool
	}{
		{"create", "corp_a,corp_b", false},
		{"detail", "corp_a,corp_b", false},
		{"create", "corp_a", false},
		{"create", "corp_a,corp_b", true},
	} {
		t.Run(tc.verb+"/"+tc.profile, func(t *testing.T) {
			setupAuthLogoutProfiles(t, authLogoutTestToken("corp_a"), authLogoutTestToken("corp_b"))
			runner := &outboundCallTestRunner{}
			args := []string{"outbound-call", "detail", "--task-id", "fixture-task", "--profile", tc.profile}
			if tc.verb == "create" {
				args = []string{"outbound-call", "create", "--callee", "fixture-a", "--task", "p", "--opening", "g", "--yes", "--profile", tc.profile}
			}
			if tc.dryRun {
				args = append(args, "--dry-run")
			}
			envelope, err, code := executeOutboundCallTestCommand(t, func(flags *GlobalFlags) executor.Runner {
				return &runtimeRunner{fallback: runner, globalFlags: flags}
			}, args...)
			if tc.dryRun {
				if err != nil || code != 0 || runner.calls != 0 || !envelope.DryRun {
					t.Fatalf("dry-run reached execution: calls=%d code=%d err=%v", runner.calls, code, err)
				}
				return
			}
			if strings.Contains(tc.profile, ",") {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || code != 3 || typed.Reason != "outbound_call_multi_profile_unsupported" || runner.calls != 0 || typed.ExecutionStarted == nil || *typed.ExecutionStarted {
					t.Fatalf("multi-profile reached execution: calls=%d code=%d err=%v", runner.calls, code, err)
				}
				return
			}
			if err != nil || code != 0 || runner.calls != 1 || runner.profile != "corp_a:user-corp_a" {
				t.Fatalf("single profile changed: calls=%d profile=%q code=%d err=%v", runner.calls, runner.profile, code, err)
			}
			if tc.verb == "create" && (envelope.Outcome != output.OutcomePending || runner.args != `{"calleeInfos":[{"callee":"fixture-a","greeting":"g","prompt":"p"}]}`) {
				t.Fatalf("single-profile call changed: outcome=%s args=%s", envelope.Outcome, runner.args)
			}
			if tc.verb == "create" {
				const next = "dws outbound-call detail --task-id fixture-task --profile corp_a:user-corp_a"
				if envelope.Meta.Operation == nil || envelope.Meta.Operation.NextCommand != next {
					t.Fatalf("query command lost the creation profile: %#v", envelope.Meta.Operation)
				}
			}
		})
	}
}

type outboundCallTestRunner struct {
	calls   int
	profile string
	args    string
}

func (r *outboundCallTestRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.calls++
	r.profile = authpkg.RuntimeProfile()
	raw, _ := json.Marshal(inv.Params)
	r.args = string(raw)
	return executor.Result{Invocation: inv, Response: map[string]any{"content": map[string]any{"taskId": "fixture-task"}}}, nil
}

func executeOutboundCallTestCommand(t *testing.T, factory func(*GlobalFlags) executor.Runner, args ...string) (output.Envelope, error, int) {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	previousProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })
	t.Cleanup(CloseFileLogger)
	helpers.InitDepsForTest(t, helpers.GetCaller())
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result { return runtimecontext.Result{} })
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, factory)
	args = append(args, "--format", "json")
	testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
	ctx, store := output.WithResultStore(context.Background())
	root := NewRootCommand(ctx)
	var stream bytes.Buffer
	root.SetOut(&stream)
	root.SetErr(&stream)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
		t.Fatal(err)
	}
	_, err := root.ExecuteC()
	if err != nil {
		return output.Envelope{}, err, apperrors.ExitCode(err)
	}
	code, _ := output.StoredExitCode(store)
	var envelope output.Envelope
	if err := json.Unmarshal(stream.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid command output: %v %s", err, &stream)
	}
	return envelope, nil, code
}

func stubOutboundCallTokenPrefetch(t *testing.T, expectPrefetch bool) {
	t.Helper()
	done := make(chan struct{})
	testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) {
		defer close(done)
		return "fixture-token", nil
	})
	if expectPrefetch {
		// Cleanup is LIFO: join prefetch before restoring its seam or removing config.
		t.Cleanup(func() {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("token prefetch did not finish before fixture cleanup")
			}
		})
	}
}
