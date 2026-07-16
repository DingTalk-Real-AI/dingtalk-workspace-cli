// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type todoReminderCall struct {
	toolName string
	args     map[string]any
}

type todoReminderCaller struct {
	calls []todoReminderCall
}

func (c *todoReminderCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, todoReminderCall{toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*todoReminderCaller) Format() string { return "json" }
func (*todoReminderCaller) DryRun() bool   { return false }
func (*todoReminderCaller) Fields() string { return "" }
func (*todoReminderCaller) JQ() string     { return "" }

func runTodoReminderCommand(t *testing.T, args ...string) (*todoReminderCaller, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	caller := &todoReminderCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard

	cmd := newTodoCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append([]string{"task", "add-reminder"}, args...))
	return caller, cmd.Execute()
}

func TestTodoAddReminderValidatesModeSpecificInputsBeforeCallingTool(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "due time requires offset",
			args:    []string{"--task-id", "task-smoke", "--base-time", "dueTime"},
			wantErr: "--due-date-offset is required",
		},
		{
			name:    "custom time requires timestamp",
			args:    []string{"--task-id", "task-smoke", "--base-time", "customTime"},
			wantErr: "--reminder-time-stamp is required",
		},
		{
			name:    "unknown base time",
			args:    []string{"--task-id", "task-smoke", "--base-time", "deadline", "--due-date-offset", "-30"},
			wantErr: "--base-time must be one of dueTime or customTime",
		},
		{
			name:    "custom time validates timestamp",
			args:    []string{"--task-id", "task-smoke", "--base-time", "customTime", "--reminder-time-stamp", "tomorrow"},
			wantErr: "reminder-time-stamp",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := runTodoReminderCommand(t, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool call count = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestTodoAddReminderMapsReviewedModes(t *testing.T) {
	tests := []struct {
		name                  string
		args                  []string
		wantBaseTime          string
		wantDueDateOffset     any
		wantReminderTimestamp any
	}{
		{
			name:              "due time",
			args:              []string{"--task-id", "task-smoke", "--base-time", "dueTime", "--due-date-offset", "-30"},
			wantBaseTime:      "dueTime",
			wantDueDateOffset: "-30",
		},
		{
			name:                  "custom time",
			args:                  []string{"--task-id", "task-smoke", "--base-time", "customTime", "--reminder-time-stamp", "2026-03-10T18:00:00+08:00"},
			wantBaseTime:          "customTime",
			wantReminderTimestamp: int64(1773136800000),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := runTodoReminderCommand(t, test.args...)
			if err != nil {
				t.Fatalf("todo add-reminder returned error: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != "add_todo_reminder" {
				t.Fatalf("tool calls = %#v, want one add_todo_reminder call", caller.calls)
			}
			request, ok := caller.calls[0].args["todoReminderAddRequest"].(map[string]any)
			if !ok {
				t.Fatalf("request = %#v, want object", caller.calls[0].args["todoReminderAddRequest"])
			}
			if request["taskId"] != "task-smoke" || request["baseTime"] != test.wantBaseTime {
				t.Fatalf("request identity = %#v", request)
			}
			if request["dueDateOffset"] != test.wantDueDateOffset {
				t.Fatalf("dueDateOffset = %#v, want %#v", request["dueDateOffset"], test.wantDueDateOffset)
			}
			if request["reminderTimeStamp"] != test.wantReminderTimestamp {
				t.Fatalf("reminderTimeStamp = %#v, want %#v", request["reminderTimeStamp"], test.wantReminderTimestamp)
			}
		})
	}
}

func TestTodoRoleTypesCSVExampleUsesRuntimeParser(t *testing.T) {
	got, err := parseRoleTypes("creator,executor")
	if err != nil {
		t.Fatalf("parse role-types example: %v", err)
	}
	want := []string{"creator", "executor"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("role types = %#v, want %#v", got, want)
	}
}
