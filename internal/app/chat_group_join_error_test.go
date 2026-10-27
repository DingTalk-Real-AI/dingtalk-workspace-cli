// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// Keep production transport-result classification, including the runner's
// newServerFailureAPIError and ServerDiagnostics extraction. Only endpoint
// resolution, credentials and the final network exchange are local fixtures.
type chatGroupJoinWireErrorRunner struct {
	runner    *runtimeRunner
	calls     []executor.Invocation
	runnerErr error
}

func (r *chatGroupJoinWireErrorRunner) Run(ctx context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, invocation)
	result, err := r.runner.executeInvocation(ctx, "https://group-join-test.invalid", invocation)
	r.runnerErr = err
	return result, err
}

func TestCrossPlatformCoverageChatGroupJoinSubmittedWireFailure(t *testing.T) {
	const (
		message   = "成员邀请已发送"
		traceID   = "fixture-group-join-trace"
		inviteURL = "https://qr.dingtalk.com/action/joingroup?code=fixture-invite"
	)
	for _, command := range []struct {
		name     string
		tool     string
		args     []string
		params   map[string]any
		readHint string
		actions  []string
	}{
		{
			name: "group-id", tool: "join_group_by_group_id",
			args:     []string{"join-by-group-id", "--group-id", "123456"},
			params:   map[string]any{"groupId": int64(123456)},
			readHint: "dws chat group get-by-group-id --group-id 123456 --format json",
			actions:  []string{"dws chat group get-by-group-id --group-id 123456 --format json"},
		},
		{
			name: "invite-url", tool: "join_group_by_invite_url",
			args:     []string{"join-by-invite-url", "--invite-url", inviteURL},
			params:   map[string]any{"inviteUrl": inviteURL},
			readHint: "chat group get-info-by-invite-url",
		},
	} {
		for _, isMCPError := range []bool{false, true} {
			reason := "business_error"
			if isMCPError {
				reason = "mcp_tool_error"
			}
			t.Run(command.name+"/"+reason, func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				// The root command opens <config>/logs/dws.log; on Windows the
				// held handle makes t.TempDir's RemoveAll fail unless closed
				// first (this cleanup runs before TempDir's, LIFO).
				t.Cleanup(CloseFileLogger)
				testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result { return runtimecontext.Result{} })
				testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
					return AccessTokenSnapshot{AccessToken: "synthetic-fixture-token"}, nil
				})
				wireCalls := 0
				testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, _ context.Context, _, tool string, params map[string]any) (transport.ToolCallResult, error) {
					wireCalls++
					if tool != command.tool || !reflect.DeepEqual(params, command.params) {
						t.Fatalf("unexpected tools/call: tool=%q params=%#v", tool, params)
					}
					return transport.ToolCallResult{IsError: isMCPError, Content: map[string]any{
						"success": false, "errorCode": "400001", "errorMsg": message,
						"message": message, "technical_detail": message, "traceId": traceID,
					}}, nil
				})
				runner := &chatGroupJoinWireErrorRunner{}
				testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
					runner.runner = &runtimeRunner{transport: transport.NewClient(nil), globalFlags: flags, auditSink: audit.NopSink{}}
					return runner
				})
				args := append([]string{"chat", "group"}, command.args...)
				args = append(args, "--format", "json")
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				helpers.InitDepsForTest(t, helpers.GetCaller())
				ctx, store := output.WithResultStore(context.Background())
				root := NewRootCommand(ctx)
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				root.SetArgs(args)
				if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
					t.Fatal(err)
				}
				leaf, err := root.ExecuteC()
				var original, submitted *apperrors.Error
				if err == nil || !errors.As(runner.runnerErr, &original) || !errors.As(err, &submitted) {
					t.Fatalf("typed runner/command failure missing: runner=%#v command=%#v", runner.runnerErr, err)
				}
				if original.Reason != reason || original.Category != apperrors.CategoryAPI || original.ServerDiag.ServerErrorCode != "400001" || original.ServerDiag.TraceID != traceID {
					t.Fatalf("runner classification/diagnostics changed: %#v", original)
				}
				if submitted == original || !errors.Is(err, runner.runnerErr) || submitted.Reason != "group_join_submitted" || !submitted.RetryableSet || submitted.Retryable {
					t.Fatalf("command lost submitted/non-retry classification: %#v", submitted)
				}
				if len(runner.calls) != 1 || wireCalls != 1 || runner.calls[0].CanonicalProduct != "im" || stdout.Len() != 0 || stderr.Len() != 0 {
					t.Fatalf("write replayed or emitted success: invocations=%#v wireCalls=%d stdout=%q stderr=%q", runner.calls, wireCalls, &stdout, &stderr)
				}
				if _, attempted, _, _ := output.StoredEmissionState(store); attempted {
					t.Fatal("failure must not publish a success or pending result before root rendering")
				}
				// This is the same projection/emitter used by the process root's
				// unified failure branch, applied to the actual chat leaf error.
				code, emitErr := output.EmitResult(leaf, output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err)))
				if emitErr != nil || code != apperrors.ExitCodeAPI || stderr.Len() != 0 {
					t.Fatalf("emit code=%d err=%v stderr=%q", code, emitErr, &stderr)
				}
				var envelope map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				info, _ := envelope["error"].(map[string]any)
				if envelope["ok"] != false || envelope["outcome"] != "failure" || envelope["data"] != nil || info["type"] != "api" || info["subtype"] != "group_join_submitted" || info["exit_code"] != float64(code) {
					t.Fatalf("submitted error must remain nonzero failure: %s", &stdout)
				}
				if info["message"] != message || info["upstream_code"] != "400001" || info["trace_id"] != traceID || info["technical_detail"] != message || info["server_key"] != "im" || info["operation"] != "tools/call" {
					t.Fatalf("upstream diagnostics lost in root projection: %s", &stdout)
				}
				hint, _ := info["hint"].(string)
				for _, text := range []string{"已提交", "等待", "审批", "尚未确认入群", "不要重复提交", "只读回查", command.readHint} {
					if !strings.Contains(hint, text) {
						t.Errorf("submitted hint missing %q: %q", text, hint)
					}
				}
				if _, retry := info["retryable"]; retry || info["retry_after_seconds"] != nil || info["next_retry_at"] != nil || info["cause"] != nil {
					t.Fatalf("failure advertises retry or internal error chain: %s", &stdout)
				}
				if !schemaContractJSONEqual(info["actions"], command.actions) {
					t.Fatalf("only reviewed read action may be offered: %#v", info["actions"])
				}
				meta, _ := envelope["meta"].(map[string]any)
				if meta["operation"] != nil || strings.Contains(stdout.String(), "fixture-invite") || len(runner.calls) != 1 || wireCalls != 1 {
					t.Fatalf("invented pending state, leaked invitation or extra tools/call: %s", &stdout)
				}
			})
		}
	}
}
