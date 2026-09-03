// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableWorkflowCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type aitableWorkflowCaller struct {
	calls    []aitableWorkflowCall
	response string
	err      error
	dryRun   bool
}

func (c *aitableWorkflowCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableWorkflowCall{productID: productID, toolName: toolName, args: args})
	if c.err != nil {
		return nil, c.err
	}
	response := c.response
	if response == "" {
		response = `{"status":"success","data":{"valid":true,"flowId":"flow-test","issues":[]}}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text",
		Text: response,
	}}}, nil
}

func (*aitableWorkflowCaller) Format() string { return "json" }
func (c *aitableWorkflowCaller) DryRun() bool { return c.dryRun }
func (*aitableWorkflowCaller) Fields() string { return "" }
func (*aitableWorkflowCaller) JQ() string     { return "" }

func runAitableWorkflowCommand(t *testing.T, stdin io.Reader, args ...string) (*aitableWorkflowCaller, error) {
	t.Helper()
	caller := &aitableWorkflowCaller{}
	return caller, runAitableWorkflowCommandWithCaller(t, caller, stdin, args...)
}

func runAitableWorkflowCommandWithCaller(t *testing.T, caller *aitableWorkflowCaller, stdin io.Reader, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)

	InitDepsForTest(t, caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", "aitable", "workflow"}, args...)

	cmd := newAitableCommand()
	cmd.PersistentFlags().String("format", "json", "output format")
	cmd.PersistentFlags().Bool("yes", false, "skip confirmation")
	cmd.PersistentFlags().Bool("dry-run", false, "preview only")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append([]string{"workflow"}, args...))
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	cmd.SetIn(stdin)
	return cmd.Execute()
}

func TestCrossPlatformCoverageAitableWorkflowCreateMapsDSLWithoutRetry(t *testing.T) {
	wantDSL := map[string]any{
		"version": "workflow-dsl/v1",
		"name":    "create test",
	}
	caller, err := runAitableWorkflowCommand(t, nil,
		"create",
		"--base-id", "base-create",
		"--dsl", `{"version":"workflow-dsl/v1","name":"create test"}`,
		"--locale", "zh-CN",
	)
	if err != nil {
		t.Fatalf("workflow create returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "aitable" || call.toolName != "create_workflow" {
		t.Fatalf("tool call = %s/%s, want aitable/create_workflow", call.productID, call.toolName)
	}
	wantArgs := map[string]any{
		"baseId": "base-create",
		"dsl":    wantDSL,
		"locale": "zh-CN",
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

func TestCrossPlatformCoverageAitableWorkflowPublishRejectsFalseSuccess(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{name: "valid false", response: `{"status":"success","data":{"valid":false,"issues":[{"message":"bad dsl"}]}}`, want: "valid=false"},
		{name: "missing valid", response: `{"status":"success","data":{"flowId":"flow-test"}}`, want: "missing valid"},
		{name: "missing flow id", response: `{"status":"success","data":{"valid":true}}`, want: "missing a non-empty flowId"},
		{name: "wrong valid type", response: `{"status":"success","data":{"valid":"true","flowId":"flow-test"}}`, want: "valid must be boolean"},
		{name: "malformed response", response: `{`, want: "not a JSON object"},
		{name: "null response", response: `null`, want: "not a JSON object"},
		{name: "conflicting valid", response: `{"valid":true,"data":{"valid":false,"flowId":"flow-test"}}`, want: "conflicting valid values"},
		{name: "invalid flow id", response: `{"valid":true,"flowId":1}`, want: "flowId must be a non-empty string"},
		{name: "conflicting workflow ids", response: `{"valid":true,"flowId":"one","data":{"workflowId":"two"}}`, want: "conflicting workflow IDs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableWorkflowCaller{response: tc.response}
			err := runAitableWorkflowCommandWithCaller(t, caller, nil,
				"create", "--base-id", "base", "--dsl", `{"version":"workflow-dsl/v1","name":"test"}`)
			if err == nil || !strings.Contains(err.Error(), tc.want) || len(caller.calls) != 1 {
				t.Fatalf("workflow false success = err:%v calls:%#v", err, caller.calls)
			}
		})
	}

	t.Run("update uses the same strict contract", func(t *testing.T) {
		caller := &aitableWorkflowCaller{response: `{"status":"success","data":{"valid":false}}`}
		err := runAitableWorkflowCommandWithCaller(t, caller, nil,
			"update", "--base-id", "base", "--workflow-id", "flow", "--dsl", `{"version":"workflow-dsl/v1","name":"test"}`)
		if err == nil || !strings.Contains(err.Error(), "valid=false") || len(caller.calls) != 1 || caller.calls[0].toolName != "update_workflow" {
			t.Fatalf("workflow update false success = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("dry-run does not call publish tool", func(t *testing.T) {
		caller := &aitableWorkflowCaller{dryRun: true}
		err := runAitableWorkflowCommandWithCaller(t, caller, nil,
			"create", "--base-id", "base", "--dsl", `{"version":"workflow-dsl/v1","name":"test"}`, "--dry-run")
		if err != nil || len(caller.calls) != 0 {
			t.Fatalf("workflow create dry-run = err:%v calls:%#v", err, caller.calls)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		caller := &aitableWorkflowCaller{err: context.Canceled}
		err := runAitableWorkflowCommandWithCaller(t, caller, nil,
			"create", "--base-id", "base", "--dsl", `{"version":"workflow-dsl/v1","name":"test"}`)
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) || len(caller.calls) != 1 {
			t.Fatalf("workflow transport error = err:%v calls:%#v", err, caller.calls)
		}
	})

	if _, _, _, err := strictAitableWorkflowPublishResult(nil); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("nil workflow envelope = %v", err)
	}
}

func TestAitableWorkflowEditExampleMapsEmptyArguments(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t, nil, "edit-example")
	if err != nil {
		t.Fatalf("workflow edit-example returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "aitable" || call.toolName != "edit_workflow_example" {
		t.Fatalf("tool call = %s/%s, want aitable/edit_workflow_example", call.productID, call.toolName)
	}
	if len(call.args) != 0 {
		t.Fatalf("tool args = %#v, want empty arguments", call.args)
	}
}

func TestAitableWorkflowUpdateReadsDSLFile(t *testing.T) {
	path := t.TempDir() + "/workflow.json"
	if err := os.WriteFile(path, []byte(`{"version":"workflow-dsl/v1","name":"updated"}`), 0o600); err != nil {
		t.Fatalf("write workflow fixture: %v", err)
	}
	caller, err := runAitableWorkflowCommand(t, nil,
		"update",
		"--base-id", "base-update",
		"--workflow-id", "flow-existing",
		"--dsl", "@"+path,
		"--locale", "en-US",
	)
	if err != nil {
		t.Fatalf("workflow update returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "aitable" || call.toolName != "update_workflow" {
		t.Fatalf("tool call = %s/%s, want aitable/update_workflow", call.productID, call.toolName)
	}
	wantArgs := map[string]any{
		"baseId":     "base-update",
		"workflowId": "flow-existing",
		"locale":     "en-US",
		"dsl": map[string]any{
			"version": "workflow-dsl/v1",
			"name":    "updated",
		},
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

func TestAitableWorkflowWriteReportsStdinReadError(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t, coverageFailingReader{},
		"create", "--base-id", "base-stdin", "--dsl", "-",
	)
	if err == nil || !strings.Contains(err.Error(), "--dsl stdin read failed: read failed") {
		t.Fatalf("error = %v, want wrapped stdin read error", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("failed stdin read reached MCP: %#v", caller.calls)
	}
}

func TestAitableWorkflowCreateReadsDSLFromStdin(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t,
		strings.NewReader(`{"version":"workflow-dsl/v1","name":"stdin"}`),
		"create", "--base-id", "base-stdin", "--dsl", "-",
	)
	if err != nil {
		t.Fatalf("workflow create from stdin returned error: %v", err)
	}
	if got := caller.calls[0].args["dsl"].(map[string]any)["name"]; got != "stdin" {
		t.Fatalf("dsl name = %#v, want stdin", got)
	}
}

func TestAitableWorkflowWriteRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "create missing dsl", args: []string{"create", "--base-id", "base"}, want: "dsl"},
		{name: "update missing workflow", args: []string{"update", "--base-id", "base", "--dsl", `{}`}, want: "workflow-id"},
		{name: "malformed json", args: []string{"create", "--base-id", "base", "--dsl", "{not-json"}, want: "JSON parse failed"},
		{name: "array", args: []string{"create", "--base-id", "base", "--dsl", `[]`}, want: "JSON parse failed"},
		{name: "null", args: []string{"create", "--base-id", "base", "--dsl", `null`}, want: "JSON object"},
		{name: "empty file path", args: []string{"create", "--base-id", "base", "--dsl", "@"}, want: "file path"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runAitableWorkflowCommand(t, nil, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid input reached MCP: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableWorkflowRunMapsRecordTrigger(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t, nil,
		"run",
		"--base-id", "base-run",
		"--workflow-id", "workflow-run",
		"--table-id", "table-run",
		"--record-ids", "record-1,record-2",
		"--yes",
	)
	if err != nil {
		t.Fatalf("workflow run returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "aitable" || call.toolName != "run_workflow" {
		t.Fatalf("tool call = %s/%s, want aitable/run_workflow", call.productID, call.toolName)
	}
	wantArgs := map[string]any{
		"baseId":     "base-run",
		"workflowId": "workflow-run",
		"tableId":    "table-run",
		"recordIds":  []string{"record-1", "record-2"},
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

func TestCrossPlatformCoverageAitableWorkflowRunMapsScheduledTrigger(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t, nil,
		"run", "--base", "base-scheduled", "--workflow-id", "workflow-scheduled", "--yes",
	)
	if err != nil {
		t.Fatalf("scheduled workflow run returned error: %v", err)
	}
	wantArgs := map[string]any{
		"baseId":     "base-scheduled",
		"workflowId": "workflow-scheduled",
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("calls = %#v, want one scheduled invocation %#v", caller.calls, wantArgs)
	}
}

func TestCrossPlatformCoverageAitableWorkflowRunRejectsUnsafeOrInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "confirmation", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow"}, want: "用户确认"},
		{name: "blank table", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow", "--table-id", "   ", "--yes"}, want: "--table-id 不能为空"},
		{name: "blank records", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow", "--record-ids", " , ", "--yes"}, want: "--record-ids 必须包含"},
		{name: "table without records", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow", "--table-id", "table", "--yes"}, want: "必须同时提供"},
		{name: "records without table", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow", "--record-ids", "record", "--yes"}, want: "必须同时提供"},
		{name: "duplicate records", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow", "--table-id", "table", "--record-ids", "record,record", "--yes"}, want: "不能包含重复值"},
		{name: "too many records", args: []string{"run", "--base-id", "base", "--workflow-id", "workflow", "--table-id", "table", "--record-ids", "r1,r2,r3,r4,r5,r6", "--yes"}, want: "最多支持 5 个"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runAitableWorkflowCommand(t, nil, tc.args...)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid run reached MCP: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableWorkflowHistoryMapsFilters(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t, nil,
		"history",
		"--base-id", "base-history",
		"--workflow-id", "workflow-history",
		"--status", "failed",
		"--after-time", "1786000000000",
		"--before-time", "1787000000000",
		"--page", "2",
		"--size", "50",
	)
	if err != nil {
		t.Fatalf("workflow history returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.productID != "aitable" || call.toolName != "get_flow_record_list" {
		t.Fatalf("tool call = %s/%s, want aitable/get_flow_record_list", call.productID, call.toolName)
	}
	wantArgs := map[string]any{
		"baseId":     "base-history",
		"flowId":     "workflow-history",
		"status":     "failed",
		"afterTime":  1786000000000,
		"beforeTime": 1787000000000,
		"page":       2,
		"size":       50,
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

func TestCrossPlatformCoverageAitableWorkflowHistoryMapsSingleTimeFilter(t *testing.T) {
	caller, err := runAitableWorkflowCommand(t, nil,
		"history",
		"--base-id", "base-history",
		"--workflow-id", "workflow-history",
		"--after-time", "1786000000000",
	)
	if err != nil {
		t.Fatalf("workflow history returned error: %v", err)
	}
	wantArgs := map[string]any{
		"baseId":    "base-history",
		"flowId":    "workflow-history",
		"afterTime": 1786000000000,
		"size":      20,
	}
	if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, wantArgs) {
		t.Fatalf("calls = %#v, want one history invocation %#v", caller.calls, wantArgs)
	}
}

func TestCrossPlatformCoverageAitableWorkflowHistoryRejectsInvalidFilters(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "status", args: []string{"history", "--base-id", "base", "--workflow-id", "workflow", "--status", "unknown"}, want: "允许值"},
		{name: "negative page", args: []string{"history", "--base-id", "base", "--workflow-id", "workflow", "--page", "-1"}, want: "--page 必须 >= 0"},
		{name: "zero size", args: []string{"history", "--base-id", "base", "--workflow-id", "workflow", "--size", "0"}, want: "--size 必须在"},
		{name: "large size", args: []string{"history", "--base-id", "base", "--workflow-id", "workflow", "--size", "101"}, want: "--size 必须在"},
		{name: "negative after", args: []string{"history", "--base-id", "base", "--workflow-id", "workflow", "--after-time", "-1"}, want: "Unix 毫秒时间戳"},
		{name: "reversed range", args: []string{"history", "--base-id", "base", "--workflow-id", "workflow", "--after-time", "200", "--before-time", "100"}, want: "必须小于"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runAitableWorkflowCommand(t, nil, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid history query reached MCP: %#v", caller.calls)
			}
		})
	}
}
