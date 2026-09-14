// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
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

func TestCrossPlatformCoverageAttendanceCapabilityBoundaries(t *testing.T) {
	root := newAttendanceCommand()
	for _, tc := range []struct {
		name      string
		path      []string
		wantLong  string
		wantUse   string
		wantAvoid string
		flag      string
		wantFlag  string
	}{
		{
			name:      "selfsetting get current user only",
			path:      []string{"selfsetting", "get"},
			wantLong:  "只支持当前登录用户本人",
			wantUse:   "当前登录用户本人",
			wantAvoid: "目标是其他员工时不可用",
			flag:      "user",
			wantFlag:  "当前登录用户本人",
		},
		{
			name:      "selfsetting save current user only",
			path:      []string{"selfsetting", "save"},
			wantLong:  "只支持当前登录用户本人",
			wantUse:   "当前登录用户本人",
			wantAvoid: "目标是其他员工时不可用",
			flag:      "user",
			wantFlag:  "当前登录用户本人",
		},
		{
			name:      "vacation update freedom only",
			path:      []string{"vacation", "update-type"},
			wantLong:  "leaveStatisticType=freedom",
			wantUse:   "leaveStatisticType 严格等于 freedom",
			wantAvoid: "leaveStatisticType 不是 freedom",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leaf, _, err := root.Find(tc.path)
			if err != nil {
				t.Fatalf("find %v: %v", tc.path, err)
			}
			if !strings.Contains(leaf.Long, tc.wantLong) {
				t.Fatalf("%v Long does not contain %q: %q", tc.path, tc.wantLong, leaf.Long)
			}
			if tc.flag != "" {
				flag := leaf.Flags().Lookup(tc.flag)
				if flag == nil || !strings.Contains(flag.Usage, tc.wantFlag) {
					t.Fatalf("%v --%s usage = %#v, want %q", tc.path, tc.flag, flag, tc.wantFlag)
				}
			}
			final, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || final.Selection == nil {
				t.Fatalf("%v missing ContractFinal selection", tc.path)
			}
			if got := strings.Join(final.Selection.UseWhen, "\n"); !strings.Contains(got, tc.wantUse) {
				t.Fatalf("%v use_when = %q, want %q", tc.path, got, tc.wantUse)
			}
			if got := strings.Join(final.Selection.AvoidWhen, "\n"); !strings.Contains(got, tc.wantAvoid) {
				t.Fatalf("%v avoid_when = %q, want %q", tc.path, got, tc.wantAvoid)
			}
		})
	}
}

func TestCrossPlatformCoverageAttendanceScheduleGetSerializesDateStrings(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		wantUsers []string
		wantStart string
		wantEnd   string
	}{
		{
			name:      "canonical date-only flags cover whole days",
			args:      []string{"schedule", "get", "--users", "u1, u2", "--start", "2026-04-01", "--end", "2026-04-30"},
			wantUsers: []string{"u1", "u2"},
			wantStart: "2026-04-01 00:00:00",
			wantEnd:   "2026-04-30 23:59:59",
		},
		{
			name:      "legacy aliases preserve explicit datetimes",
			args:      []string{"schedule", "get", "--userIdList", "u1", "--workDateBegin", "2026-04-01 09:00:00", "--workDateEnd", "2026-04-01 18:00:00"},
			wantUsers: []string{"u1"},
			wantStart: "2026-04-01 09:00:00",
			wantEnd:   "2026-04-01 18:00:00",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := runAttendanceSearchCommand(t, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %#v, want one", caller.calls)
			}
			call := caller.calls[0]
			if call.server != "attendance-wukong" || call.toolName != "getScheduleByRange" {
				t.Fatalf("tool = %s/%s, want attendance-wukong/getScheduleByRange", call.server, call.toolName)
			}
			request, ok := call.args["GetScheduleByRangeRequest"].(map[string]any)
			if !ok {
				t.Fatalf("request = %#v, want map", call.args["GetScheduleByRangeRequest"])
			}
			if got, ok := request["workDateBegin"].(string); !ok || got != tc.wantStart {
				t.Fatalf("workDateBegin = %#v (%T), want string %q", request["workDateBegin"], request["workDateBegin"], tc.wantStart)
			}
			if got, ok := request["workDateEnd"].(string); !ok || got != tc.wantEnd {
				t.Fatalf("workDateEnd = %#v (%T), want string %q", request["workDateEnd"], request["workDateEnd"], tc.wantEnd)
			}
			if got := request["userIdList"]; !reflect.DeepEqual(got, tc.wantUsers) {
				t.Fatalf("userIdList = %#v, want %#v", got, tc.wantUsers)
			}
		})
	}

	caller, err := runAttendanceSearchCommand(t,
		"schedule", "get", "--users", "u1", "--start", "2026-04-02", "--end", "2026-04-01",
	)
	if err == nil || !strings.Contains(err.Error(), "--end must not be earlier than --start") {
		t.Fatalf("reversed range error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("reversed range calls = %#v, want none", caller.calls)
	}
}

func TestCrossPlatformCoverageAttendanceScheduleDateOnlyEndPreservesDSTCalendarDay(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load DST timezone: %v", err)
	}

	for _, date := range []string{
		"2026-03-08", // spring forward: 23-hour day
		"2026-11-01", // fall back: 25-hour day
	} {
		t.Run(date, func(t *testing.T) {
			want := date + " 23:59:59"
			got, parsed, err := normalizeScheduleRangeDateInLocation(date, "end", loc)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("normalized end = %q, want %q", got, want)
			}
			if gotParsed := parsed.In(loc).Format("2006-01-02 15:04:05"); gotParsed != want {
				t.Fatalf("parsed end = %q, want %q", gotParsed, want)
			}
		})
	}
}
