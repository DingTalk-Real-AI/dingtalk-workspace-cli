// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageAITableSFMutationFailureDelivery(t *testing.T) {
	for _, tc := range []struct {
		args []string
		tool string
	}{
		{[]string{"create", "--name=group", "--field-ids=F"}, "create_field_group"},
		{[]string{"update", "--group-id=G", "--description="}, "update_field_group"},
		{[]string{"ungroup", "--group-id=G"}, "ungroup_field_group"},
		{[]string{"move-fields", "--field-ids=F"}, "move_field_group_fields"},
	} {
		for _, code := range []string{"FIELD_GROUP_CREDENTIAL_REJECTED", "FIELD_GROUP_PERMISSION_REJECTED", "FIELD_GROUP_INPUT_REJECTED", "FIELD_GROUP_DATA_NOT_READY", "FIELD_GROUP_SERVICE_UNAVAILABLE", "FIELD_GROUP_PRECHECK_FAILED", "FIELD_GROUP_OUTCOME_UNKNOWN"} {
			for _, delivery := range []string{"runner", "runner_is_error", "helper"} {
				t.Run(tc.tool+"/"+code+"/"+delivery, func(t *testing.T) {
					t.Setenv("DWS_CONFIG_DIR", t.TempDir())
					t.Cleanup(CloseFileLogger)
					// Even an erroneous true hint cannot make an unknown outcome retryable.
					body := fmt.Sprintf(`{"status":"error","error":{"type":"SYSTEM_ERROR","code":%q,"message":"Safe stage rejection","retryable":true,"details":{"token":"private-token"}},"data":{"uid":"private-uid"}}`, code)
					calls := 0
					testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, _ context.Context, _, tool string, args map[string]any) (transport.ToolCallResult, error) {
						calls++
						if tool != tc.tool || args["baseId"] != "B" || args["tableId"] != "T" {
							t.Fatalf("changed mutation: %s %#v", tool, args)
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
					args := append([]string{"aitable", "field-group"}, tc.args...)
					args = append(args, "--base-id=B", "--table-id=T", "--yes", "--format=json")
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
						t.Fatalf("expected one failed mutation: %v runner=%d helper=%d", err, calls, caller.calls)
					}
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Operation != "tools/call" || typed.ServerKey != "aitable" {
						t.Fatalf("lost mutation context: %#v %v", typed, err)
					}
					info := errorInfoFromExecutionError(err)
					retryable := code == "FIELD_GROUP_DATA_NOT_READY" || code == "FIELD_GROUP_SERVICE_UNAVAILABLE" || code == "FIELD_GROUP_PRECHECK_FAILED"
					if info.Type != "api" || info.UpstreamCode != code || info.Subtype != code || info.Retryable != retryable || info.Message != "Safe stage rejection" || info.Origin != "mcp" {
						t.Fatalf("mutation stage lost: %#v", info)
					}
					if info.Details != nil || info.ExecutionStarted != nil || info.Cause != "" {
						t.Fatalf("copied unreviewed diagnostics: %#v", info)
					}
					status, emitErr := output.EmitResult(cmd, output.FailureWithExitCode(info, apperrors.ExitCode(err)))
					var envelope map[string]any
					if emitErr != nil || status != 1 || json.Unmarshal(buf.Bytes(), &envelope) != nil || envelope["ok"] != false || envelope["outcome"] != "failure" || envelope["data"] != nil || strings.Contains(buf.String(), "private-") {
						t.Fatalf("invalid failure envelope: %d %v %s", status, emitErr, buf.String())
					}
				})
			}
		}
	}
}
