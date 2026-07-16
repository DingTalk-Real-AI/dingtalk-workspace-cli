// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type qualityEvaluationCall struct {
	productID string
	toolName  string
	args      map[string]any
}

type qualityEvaluationCaller struct {
	calls  []qualityEvaluationCall
	dryRun bool
}

func (c *qualityEvaluationCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, qualityEvaluationCall{productID: productID, toolName: toolName, args: args})
	switch toolName {
	case "get_cell_infos":
		return qualityEvaluationTextResult("null"), nil
	case "get_todo_detail":
		taskID, _ := args["taskId"].(string)
		if taskID == "INVALID" {
			return nil, errors.New("task not found")
		}
		if taskID == "MISMATCH" {
			return qualityEvaluationTextResult(`{"success":true,"result":{"todoDetailModel":{"taskId":"OTHER"}}}`), nil
		}
		return qualityEvaluationTextResult(`{"success":true,"result":{"todoDetailModel":{"taskId":"` + taskID + `"}}}`), nil
	case "update_todo_done_status":
		return qualityEvaluationTextResult(`{"success":true}`), nil
	case "list_todo_attachment":
		return qualityEvaluationTextResult(`{"success":true,"attachments":[]}`), nil
	default:
		return qualityEvaluationTextResult(`{}`), nil
	}
}

func (*qualityEvaluationCaller) Format() string { return "json" }
func (c *qualityEvaluationCaller) DryRun() bool { return c.dryRun }
func (*qualityEvaluationCaller) Fields() string { return "" }
func (*qualityEvaluationCaller) JQ() string     { return "" }

func qualityEvaluationTextResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func executeQualityEvaluationCommand(t *testing.T, product string, caller *qualityEvaluationCaller, cmd *cobra.Command, args ...string) error {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	deps.Out.w = io.Discard
	os.Args = append([]string{"dws", product}, args...)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageSheetRangeReadRejectsNullToolResponse(t *testing.T) {
	for _, command := range []string{"read", "get"} {
		t.Run(command, func(t *testing.T) {
			caller := &qualityEvaluationCaller{}
			err := executeQualityEvaluationCommand(t, "sheet", caller, newSheetCommand(),
				"range", command, "--node", "INVALID")
			if err == nil {
				t.Fatal("sheet range command accepted a null MCP response")
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != "get_cell_infos" {
				t.Fatalf("calls = %#v, want one get_cell_infos call", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageTodoCommandsRejectMissingTaskBeforeTargetCall(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		targetTool string
	}{
		{name: "done", args: []string{"task", "done", "--task-id", "INVALID", "--status", "true"}, targetTool: "update_todo_done_status"},
		{name: "done with mismatched task", args: []string{"task", "done", "--task-id", "MISMATCH", "--status", "true"}, targetTool: "update_todo_done_status"},
		{name: "list attachment", args: []string{"task", "list-attachment", "--task-id", "INVALID"}, targetTool: "list_todo_attachment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &qualityEvaluationCaller{}
			err := executeQualityEvaluationCommand(t, "todo", caller, newTodoCommand(), tt.args...)
			if err == nil {
				t.Fatal("todo command accepted a missing task")
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != "get_todo_detail" {
				t.Fatalf("calls = %#v, want only get_todo_detail preflight before %s", caller.calls, tt.targetTool)
			}
		})
	}
}

func TestCrossPlatformCoverageTodoCommandsPreflightExistingTask(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		targetTool string
	}{
		{name: "done", args: []string{"task", "done", "--task-id", "12345", "--status", "true"}, targetTool: "update_todo_done_status"},
		{name: "list attachment", args: []string{"task", "list-attachment", "--task-id", "12345"}, targetTool: "list_todo_attachment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &qualityEvaluationCaller{}
			if err := executeQualityEvaluationCommand(t, "todo", caller, newTodoCommand(), tt.args...); err != nil {
				t.Fatalf("todo command returned error: %v", err)
			}
			if len(caller.calls) != 2 || caller.calls[0].toolName != "get_todo_detail" || caller.calls[1].toolName != tt.targetTool {
				t.Fatalf("calls = %#v, want get_todo_detail then %s", caller.calls, tt.targetTool)
			}
		})
	}
}

func TestCrossPlatformCoverageTodoTaskPreflightIsSkippedForDryRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "done", args: []string{"task", "done", "--task-id", "12345", "--status", "true"}},
		{name: "list attachment", args: []string{"task", "list-attachment", "--task-id", "12345"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &qualityEvaluationCaller{dryRun: true}
			if err := executeQualityEvaluationCommand(t, "todo", caller, newTodoCommand(), tt.args...); err != nil {
				t.Fatalf("todo dry-run returned error: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("dry-run made remote calls: %#v", caller.calls)
			}
		})
	}
}
