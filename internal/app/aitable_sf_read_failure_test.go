// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type fieldCapabilityRunner struct{ runtime *runtimeRunner }

func (r fieldCapabilityRunner) Run(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
	return r.runtime.executeInvocation(ctx, "https://fixture.test", inv)
}

type fieldCapabilityReadCaller struct {
	aitableOutputCompatibilityCaller
	response string
	calls    int
}

func (c *fieldCapabilityReadCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}

// Only the remote response is replaced: both runner isError forms and the
// legacy helper path must reach the final CLI envelope with the same contract.
func TestCrossPlatformCoverageAITableSFReadFailureDelivery(t *testing.T) {
	for path, tool := range map[string]string{
		"aitable field-group list": "list_field_groups",
		"aitable field ai-plugins": "list_builtin_ai_field_plugins",
	} {
		for _, code := range []string{"InvalidArgument", "InvalidRequest.ResourceNotFound", "InvalidRequest.Forbidden", "InvalidRequest.DataNotReady", "500"} {
			for _, delivery := range []string{"runner", "runner_is_error", "helper"} {
				t.Run(path+"/"+code+"/"+delivery, func(t *testing.T) {
					t.Setenv("DWS_CONFIG_DIR", t.TempDir())
					t.Cleanup(CloseFileLogger)
					retryable := code == "InvalidRequest.DataNotReady" || code == "500"
					errorType := "USER_ERROR"
					if retryable {
						errorType = "SYSTEM_ERROR"
					}
					const safeMessage = "Safe read rejection"
					body := fmt.Sprintf(`{"status":"error","error":{"type":%q,"code":%q,"message":%q,"retryable":%t,"details":{"token":"private-token"}},"data":{"uid":"private-uid"}}`, errorType, code, safeMessage, retryable)
					calls := 0
					testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, _ context.Context, _, calledTool string, args map[string]any) (transport.ToolCallResult, error) {
						calls++
						if calledTool != tool || args["baseId"] != "B" || args["tableId"] != "T" {
							t.Fatalf("changed read request: %s %#v", calledTool, args)
						}
						wire, err := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": body}}, "isError": delivery == "runner_is_error"})
						if err != nil {
							t.Fatal(err)
						}
						var result transport.ToolCallResult
						if err := json.Unmarshal(wire, &result); err != nil {
							t.Fatal(err)
						}
						return result, nil
					})
					args := append(strings.Fields(path), "--base-id=B", "--table-id=T", "--format=json")
					testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
					root := NewRootCommand()
					caller := &fieldCapabilityReadCaller{response: body}
					if delivery == "helper" {
						helpers.InitDepsForTest(t, caller)
					} else {
						flags := &GlobalFlags{Token: "fixture-only", Format: "json"}
						runner := fieldCapabilityRunner{&runtimeRunner{transport: transport.NewClient(nil), globalFlags: flags, auditSink: audit.NopSink{}}}
						helpers.InitDepsForTest(t, newToolCallerAdapter(runner, flags))
					}
					var buf bytes.Buffer
					root.SetOut(&buf)
					root.SetErr(&buf)
					root.SetArgs(args)
					cmd, err := root.ExecuteC()
					if err == nil || calls+caller.calls != 1 {
						t.Fatalf("expected one failed read: err=%v runner=%d helper=%d", err, calls, caller.calls)
					}
					info := errorInfoFromExecutionError(err)
					if info.Type != "api" || info.ExitCode != 1 || info.UpstreamCode != code || info.Subtype != code || info.Retryable != retryable || info.Origin != "mcp" || info.Message != safeMessage {
						t.Fatalf("read recovery contract lost: %#v", info)
					}
					if info.Details != nil || info.ExecutionStarted != nil || info.Cause != "" {
						t.Fatalf("copied unreviewed diagnostics or invented execution evidence: %#v", info)
					}
					status, emitErr := output.EmitResult(cmd, output.FailureWithExitCode(info, apperrors.ExitCode(err)))
					var envelope map[string]any
					if emitErr != nil || status != 1 || json.Unmarshal(buf.Bytes(), &envelope) != nil || envelope["outcome"] != "failure" || envelope["ok"] != false {
						t.Fatalf("invalid failure emission: %d %v %s", status, emitErr, buf.String())
					}
					failure := envelope["error"].(map[string]any)
					if failure["upstream_code"] != code || failure["message"] != safeMessage || (failure["retryable"] == true) != retryable {
						t.Fatalf("wire contract lost: %#v", failure)
					}
					if _, present := failure["retryable"]; present != retryable {
						t.Fatalf("retryable=false must remain omitted: %#v", failure)
					}
					if strings.Contains(buf.String(), "private-") || envelope["data"] != nil {
						t.Fatalf("read failure leaked data: %s", buf.String())
					}
				})
			}
		}
	}
}
