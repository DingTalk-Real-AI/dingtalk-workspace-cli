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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type aitableWorkflowCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type aitableWorkflowCaller struct {
	calls []aitableWorkflowCall
}

func (c *aitableWorkflowCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableWorkflowCall{productID: productID, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text",
		Text: `{"status":"success","data":{"valid":true,"flowId":"flow-test","issues":[]}}`,
	}}}, nil
}

func (*aitableWorkflowCaller) Format() string { return "json" }
func (*aitableWorkflowCaller) DryRun() bool   { return false }
func (*aitableWorkflowCaller) Fields() string { return "" }
func (*aitableWorkflowCaller) JQ() string     { return "" }

func runAitableWorkflowCommand(t *testing.T, stdin io.Reader, args ...string) (*aitableWorkflowCaller, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	caller := &aitableWorkflowCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", "aitable", "workflow"}, args...)

	cmd := newAitableCommand()
	cmd.PersistentFlags().String("format", "json", "output format")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append([]string{"workflow"}, args...))
	if stdin != nil {
		cmd.SetIn(stdin)
	}
	return caller, cmd.Execute()
}

func TestAitableWorkflowCreateMapsDSLWithoutRetry(t *testing.T) {
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
		"dsl": map[string]any{
			"version": "workflow-dsl/v1",
			"name":    "updated",
		},
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
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
