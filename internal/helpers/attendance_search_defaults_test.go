// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type attendanceSearchCall struct {
	server   string
	toolName string
	args     map[string]any
}

type attendanceSearchCaller struct {
	calls []attendanceSearchCall
}

func (c *attendanceSearchCaller) CallTool(_ context.Context, server string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, attendanceSearchCall{server: server, toolName: toolName, args: args})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

func (*attendanceSearchCaller) Format() string { return "json" }
func (*attendanceSearchCaller) DryRun() bool   { return false }
func (*attendanceSearchCaller) Fields() string { return "" }
func (*attendanceSearchCaller) JQ() string     { return "" }

func runAttendanceSearchCommand(t *testing.T, args ...string) (*attendanceSearchCaller, error) {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	caller := &attendanceSearchCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard

	cmd := newAttendanceCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return caller, cmd.Execute()
}

func TestAttendanceSearchPaginationFlagsAreOptionalRuntimeDefaults(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		toolName string
		wantArgs map[string]any
	}{
		{
			name:     "adjustment",
			args:     []string{"adjustment", "search"},
			toolName: "get_adjustment_rule",
			wantArgs: map[string]any{"ATRuleQueryParam": map[string]any{"currentPage": 1, "pageSize": 20}},
		},
		{
			name:     "overtime",
			args:     []string{"overtime", "search"},
			toolName: "get_overtime_rule",
			wantArgs: map[string]any{"ATRuleQueryParam": map[string]any{"currentPage": 1, "pageSize": 20}},
		},
		{
			name:     "group",
			args:     []string{"group", "search"},
			toolName: "get_simple_groups",
			wantArgs: map[string]any{
				"param": map[string]any{
					"queryPositionAndWifiNames": false,
					"queryBleDeviceList":        false,
				},
				"pageQuery": map[string]any{"pageIndex": 1, "pageSize": 20},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := runAttendanceSearchCommand(t, test.args...)
			if err != nil {
				t.Fatalf("attendance search returned error: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].toolName != test.toolName {
				t.Fatalf("tool calls = %#v, want one %s call", caller.calls, test.toolName)
			}
			if !reflect.DeepEqual(caller.calls[0].args, test.wantArgs) {
				t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, test.wantArgs)
			}
		})
	}
}

func TestAttendanceApproveListAcceptsCSVTypesExample(t *testing.T) {
	caller, err := runAttendanceSearchCommand(t,
		"approve", "list",
		"--users", "user-smoke",
		"--types", "overtime,leave",
		"--start", "2026-04-01",
		"--end", "2026-04-30",
	)
	if err != nil {
		t.Fatalf("attendance approve list returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].toolName != "query_user_approve" {
		t.Fatalf("tool calls = %#v, want one query_user_approve call", caller.calls)
	}
	want := map[string]any{
		"QueryUserApproveRequest": map[string]any{
			"userIds":  []string{"user-smoke"},
			"bizTypes": []int{1, 3},
			"fromDate": "2026-04-01 00:00:00",
			"toDate":   "2026-04-30 00:00:00",
		},
	}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("tool args = %#v, want %#v", caller.calls[0].args, want)
	}
}

func TestAttendanceSummaryKeepsHiddenDeprecatedTagNameCompatibility(t *testing.T) {
	root := newAttendanceCommand()
	summary, _, err := root.Find([]string{"summary"})
	if err != nil {
		t.Fatalf("find attendance summary: %v", err)
	}
	for _, flagName := range []string{"user", "date", "stats-type"} {
		if summary.Flags().Lookup(flagName) == nil {
			t.Errorf("attendance summary does not expose --%s", flagName)
		}
	}
	legacyTagName := summary.Flags().Lookup("tag-name")
	if legacyTagName == nil {
		t.Fatal("attendance summary no longer accepts legacy --tag-name")
	}
	if !legacyTagName.Hidden || legacyTagName.Deprecated == "" {
		t.Fatalf("legacy --tag-name hidden=%v deprecated=%q, want hidden and deprecated", legacyTagName.Hidden, legacyTagName.Deprecated)
	}

	caller, err := runAttendanceSearchCommand(t,
		"summary",
		"--user", "user-smoke",
		"--date", "2026-03-12",
		"--stats-type", "week",
		"--tag-name", "legacy-script-value",
	)
	if err != nil {
		t.Fatalf("attendance summary returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %#v, want exactly one call", caller.calls)
	}
	call := caller.calls[0]
	if call.server != "attendance-wukong" || call.toolName != "get_user_attendance_summary" {
		t.Fatalf("tool call = %s/%s, want attendance-wukong/get_user_attendance_summary", call.server, call.toolName)
	}
	queryDate, err := parseDateToTimestamp("2026-03-12", "date")
	if err != nil {
		t.Fatalf("parse fixture date: %v", err)
	}
	want := map[string]any{
		"userId":    "user-smoke",
		"queryDate": queryDate,
		"statsType": "week",
	}
	if !reflect.DeepEqual(call.args, want) {
		t.Fatalf("tool args = %#v, want %#v", call.args, want)
	}
}
