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
	return runTodoTaskCommandForReminderTests(t, "add-reminder", args...)
}

func runTodoTaskCommandForReminderTests(t *testing.T, leaf string, args ...string) (*todoReminderCaller, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	caller := &todoReminderCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard

	cmd := newTodoCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append([]string{"task", leaf}, args...))
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

func TestTodoResetReminderRejectsInvalidNonEmptyRulesBeforeCallingTool(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "explicit empty", value: "", wantErr: "不能为空"},
		{name: "whitespace", value: "  ", wantErr: "不能为空"},
		{name: "malformed JSON", value: `[`, wantErr: "合法 JSON 数组"},
		{name: "object instead of array", value: `{}`, wantErr: "由对象组成的 JSON 数组"},
		{name: "scalar item", value: `[1]`, wantErr: "由对象组成的 JSON 数组"},
		{name: "null array", value: `null`, wantErr: "不能是 null"},
		{name: "null item", value: `[null]`, wantErr: "第 1 条必须是对象"},
		{name: "missing base time", value: `[{}]`, wantErr: "缺少字符串 baseTime"},
		{name: "non-string base time", value: `[{"baseTime":1}]`, wantErr: "缺少字符串 baseTime"},
		{name: "due time missing offset", value: `[{"baseTime":"dueTime"}]`, wantErr: "必须提供整数 dueDateOffset"},
		{name: "due time string offset", value: `[{"baseTime":"dueTime","dueDateOffset":"-30"}]`, wantErr: "必须提供整数 dueDateOffset"},
		{name: "due time fractional offset", value: `[{"baseTime":"dueTime","dueDateOffset":-1.5}]`, wantErr: "dueDateOffset 必须是整数"},
		{name: "custom time missing timestamp", value: `[{"baseTime":"customTime"}]`, wantErr: "必须提供 ISO8601 字符串"},
		{name: "custom time numeric timestamp", value: `[{"baseTime":"customTime","reminderTimeStamp":1}]`, wantErr: "必须提供 ISO8601 字符串"},
		{name: "custom time invalid timestamp", value: `[{"baseTime":"customTime","reminderTimeStamp":"tomorrow"}]`, wantErr: "reminderTimeStamp 无效"},
		{name: "unknown base time", value: `[{"baseTime":"deadline"}]`, wantErr: "baseTime 必须是 dueTime 或 customTime"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := runTodoTaskCommandForReminderTests(t, "reset-reminder",
				"--task-id", "task-smoke", "--reminder-rules", test.value)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %#v, want none", caller.calls)
			}
		})
	}
}

func TestTodoResetReminderMapsValidatedRulesAndExplicitClears(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantRules []map[string]any
	}{
		{
			name:      "omitted rules clear with null",
			args:      []string{"--task-id", "task-smoke"},
			wantRules: nil,
		},
		{
			name:      "explicit empty array clears with array",
			args:      []string{"--task-id", "task-smoke", "--reminder-rules", `[]`},
			wantRules: []map[string]any{},
		},
		{
			name: "valid rules are normalized",
			args: []string{
				"--task-id", "task-smoke",
				"--reminder-rules",
				`[{"baseTime":"dueTime","dueDateOffset":-30},{"baseTime":"customTime","reminderTimeStamp":"2026-03-10T18:00:00+08:00","label":"keep"}]`,
			},
			wantRules: []map[string]any{
				{"baseTime": "dueTime", "dueDateOffset": int64(-30)},
				{"baseTime": "customTime", "reminderTimeStamp": int64(1773136800000), "label": "keep"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := runTodoTaskCommandForReminderTests(t, "reset-reminder", test.args...)
			if err != nil {
				t.Fatalf("todo reset-reminder returned error: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != "reset_todo_reminder" {
				t.Fatalf("tool calls = %#v, want one reset_todo_reminder call", caller.calls)
			}
			request, ok := caller.calls[0].args["todoReminderUpdateRequest"].(map[string]any)
			if !ok {
				t.Fatalf("request = %#v, want object", caller.calls[0].args["todoReminderUpdateRequest"])
			}
			if request["taskId"] != "task-smoke" {
				t.Fatalf("taskId = %#v, want task-smoke", request["taskId"])
			}
			rules, ok := request["reminderRules"].([]map[string]any)
			if !ok {
				t.Fatalf("reminderRules = %#v, want []map[string]any", request["reminderRules"])
			}
			if !reflect.DeepEqual(rules, test.wantRules) {
				t.Fatalf("reminderRules = %#v, want %#v", rules, test.wantRules)
			}
		})
	}
}

func TestUnsupportedRemindAtGuidanceDoesNotRequireDueForCustomTime(t *testing.T) {
	caller, err := runTodoTaskCommandForReminderTests(t, "create",
		"--title", "x", "--executors", "user-1", "--remind-at", "2026-03-10T18:00:00+08:00")
	if err == nil {
		t.Fatal("todo create --remind-at returned nil error")
	}
	if !strings.Contains(err.Error(), "customTime") ||
		!strings.Contains(err.Error(), "不必先设置 --due") {
		t.Fatalf("error = %v, want customTime guidance without due requirement", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("tool calls = %#v, want none", caller.calls)
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
