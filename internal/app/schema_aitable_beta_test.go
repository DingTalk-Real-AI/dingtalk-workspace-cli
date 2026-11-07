// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableBetaCreateFailureCaller struct {
	aitableOutputCompatibilityCaller
	nilResult bool
}

func (c *aitableBetaCreateFailureCaller) CallTool(_ context.Context, _ string, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.tools = append(c.tools, tool)
	if c.nilResult {
		return nil, nil
	}
	return nil, context.DeadlineExceeded
}

func TestCrossPlatformCoverageAitableCreateUnknownErrorDelivery(t *testing.T) {
	for _, nilResult := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "nil result"}[nilResult], func(t *testing.T) {
			args := []string{"aitable", "field", "create", "--base-id=b", "--table-id=t", "--name=N", "--type=text", "--wait", "--format=json"}
			testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
			root := NewRootCommand()
			caller := &aitableBetaCreateFailureCaller{nilResult: nilResult}
			helpers.InitDepsForTest(t, caller)
			root.SetArgs(args)
			err := root.Execute()
			if err == nil || len(caller.tools) != 1 || caller.tools[0] != "create_fields" {
				t.Fatalf("unexpected execution: %v / %v", err, caller.tools)
			}
			info := errorInfoFromExecutionError(err)
			if info.Subtype != "create_outcome_unknown" || info.Retryable || info.ExecutionStarted == nil || !*info.ExecutionStarted || info.Details["baseId"] != "b" || info.Details["tableId"] != "t" {
				t.Fatalf("unknown write lost final error contract: %#v", info)
			}
			encoded, marshalErr := json.Marshal(info)
			if marshalErr != nil || !strings.Contains(info.Hint, "只读") || strings.Contains(string(encoded), "Retry later") || strings.Contains(string(encoded), "请重试并") {
				t.Fatalf("unsafe recovery reached wire: %s / %v", encoded, marshalErr)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableBetaWaitAndSafetyDelivery(t *testing.T) {
	for _, path := range []string{
		"aitable base create", "aitable base copy", "aitable record create", "aitable record create-sub",
		"aitable view create", "aitable view duplicate", "aitable form submit", "aitable dashboard create",
		"aitable chart create", "aitable advperm role-create", "aitable section create", "aitable datasource create", "aitable create",
	} {
		tool := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if tool["idempotency"] != "non_idempotent" {
			t.Fatalf("creation may be replayed: %s: %#v", path, tool["idempotency"])
		}
	}
	for _, path := range []string{"aitable table create", "aitable field create"} {
		for _, compact := range []bool{false, true} {
			args := []string{"--cli-path", path}
			if compact {
				args = append(args, "--compact")
			}
			tool := executeShortcutSchemaQuery(t, args...)
			if tool["idempotency"] != "non_idempotent" {
				t.Fatalf("%s safety: %#v", path, tool["idempotency"])
			}
			params := tool["parameters"].(map[string]any)
			for _, name := range []string{"wait", "wait-timeout"} {
				p, ok := params[name].(map[string]any)
				if !ok || p["property"] != nil && p["property"] != "" {
					t.Fatalf("%s local flag %s: %#v", path, name, p)
				}
			}
		}
	}
	for _, path := range []string{"aitable +app-get", "aitable +app-page-list"} {
		tool := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if tool["effect"] != "write" || tool["confirmation"] != "user_required" || tool["idempotency"] != "idempotent" {
			t.Fatalf("conditional write delivery %s: %#v", path, tool)
		}
	}
}

func TestCrossPlatformCoverageAitableAllFlagDocumentation(t *testing.T) {
	body, err := os.ReadFile("../../skills/multi/dingtalk-aitable/references/aitable-record-ops.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "+record-query --all --max-records") || strings.Contains(string(body), "不提供 `--all`") {
		t.Fatal("shortcut --all documentation drift")
	}
	root := NewRootCommand()
	command, _, err := root.Find([]string{"aitable", "+record-query"})
	if err != nil || command.Flags().Lookup("all") == nil || command.Flags().Lookup("max-records") == nil {
		t.Fatalf("documented flags missing: %v", err)
	}
}
