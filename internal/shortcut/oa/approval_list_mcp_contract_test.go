// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package oa

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageOAApprovalListsForwardCurrentMCPFilters(t *testing.T) {
	commonArgs := []string{
		"--page", "2",
		"--limit", "3",
		"--query", " fixture ",
		"--process-code", "PROC",
		"--originator-user-id", "originator-1",
		"--create-time-from", "2026-08-01",
		"--create-time-to", "2026-08-31",
		"--finish-time-from", "2026-08-02",
		"--finish-time-to", "2026-08-30",
	}
	commonWant := map[string]any{
		"pageNumber":       2,
		"pageSize":         3,
		"query":            "fixture",
		"processCode":      "PROC",
		"originatorUserId": "originator-1",
		"createTimeFrom":   "2026-08-01",
		"createTimeTo":     "2026-08-31",
		"finishTimeFrom":   "2026-08-02",
		"finishTimeTo":     "2026-08-30",
	}
	validPage := `{"success":"true","result":{"hasMore":false,"values":[]}}`

	tests := []struct {
		name        string
		declaration shortcut.Shortcut
		tool        string
		extraArgs   []string
		extraWant   map[string]any
	}{
		{
			name:        "pending",
			declaration: ListPending,
			tool:        "get_todo_tasks",
			extraArgs:   []string{"--create-before", "2026-08-28"},
			extraWant:   map[string]any{"createBefore": "2026-08-28"},
		},
		{name: "executed", declaration: ListExecuted, tool: "get_done_tasks", extraArgs: []string{"--process-instance-status", "COMPLETED"}, extraWant: map[string]any{"processInstanceStatus": "COMPLETED"}},
		{name: "submitted", declaration: ListSubmitted, tool: "get_submitted_instances", extraArgs: []string{"--process-instance-status", "COMPLETED"}, extraWant: map[string]any{"processInstanceStatus": "COMPLETED"}},
		{
			name:        "cc explicit false",
			declaration: ListCc,
			tool:        "get_noticed_instances",
			extraArgs:   []string{"--unread-only=false"},
			extraWant:   map[string]any{"unreadOnly": false},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &oaCoverageCaller{responses: map[string][]string{test.tool: {validPage}}}
			args := append(append([]string{}, commonArgs...), test.extraArgs...)
			if _, err := runOACoverage(t, test.declaration, caller, args...); err != nil {
				t.Fatalf("execute %s: %v", test.declaration.Command, err)
			}
			if !reflect.DeepEqual(caller.history, []string{test.tool}) {
				t.Fatalf("tools = %#v, want [%s]", caller.history, test.tool)
			}
			want := make(map[string]any, len(commonWant)+len(test.extraWant))
			for key, value := range commonWant {
				want[key] = value
			}
			for key, value := range test.extraWant {
				want[key] = value
			}
			if !reflect.DeepEqual(caller.arguments[0], want) {
				t.Fatalf("arguments = %#v, want %#v", caller.arguments[0], want)
			}
		})
	}
}

func TestCrossPlatformCoverageOAApprovalListsRejectRetiredCurrentUserAndStatusResultFlags(t *testing.T) {
	tests := []struct {
		declaration shortcut.Shortcut
		retired     []string
	}{
		{declaration: ListPending, retired: []string{"user-id", "process-instance-status", "process-instance-result"}},
		{declaration: ListExecuted, retired: []string{"user-id", "process-instance-result"}},
		{declaration: ListSubmitted, retired: []string{"user-id", "process-instance-result"}},
		{declaration: ListCc, retired: []string{"user-id", "process-instance-status", "process-instance-result"}},
	}

	for _, test := range tests {
		for _, flag := range test.retired {
			t.Run(test.declaration.Command+"/"+flag, func(t *testing.T) {
				for _, declaration := range test.declaration.Flags {
					if declaration.Name == flag {
						t.Fatalf("%s still exposes --%s", test.declaration.Command, flag)
					}
				}
				caller := &oaCoverageCaller{responses: map[string][]string{}}
				_, err := runOACoverage(t, test.declaration, caller, "--"+flag, "fixture")
				if err == nil || !strings.Contains(err.Error(), "unknown flag: --"+flag) {
					t.Fatalf("%s --%s error = %v, want unknown flag", test.declaration.Command, flag, err)
				}
				if len(caller.history) != 0 {
					t.Fatalf("%s --%s made MCP calls: %#v", test.declaration.Command, flag, caller.history)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOATodoTasksFailureEnvelopeFailsClosed(t *testing.T) {
	caller := &oaCoverageCaller{responses: map[string][]string{
		"get_todo_tasks": {`{"error_message":"参数错误","result":{},"success":"false","error_code":"400002"}`},
	}}
	if _, err := runOACoverage(t, ListPending, caller); err == nil {
		t.Fatal("get_todo_tasks business failure unexpectedly succeeded")
	}
	if !reflect.DeepEqual(caller.history, []string{"get_todo_tasks"}) {
		t.Fatalf("tools = %#v, want [get_todo_tasks]", caller.history)
	}
}

func TestCrossPlatformCoverageOACompatibilityListSchemaUsesMCPProperties(t *testing.T) {
	tests := []struct {
		declaration shortcut.Shortcut
		want        map[string]string
	}{
		{declaration: PendingApprovals, want: map[string]string{"limit": "pageSize"}},
		{declaration: DoneApprovals, want: map[string]string{"limit": "pageSize"}},
		{declaration: MyInitiated, want: map[string]string{"query": "query", "page": "pageNumber", "limit": "pageSize"}},
	}
	for _, test := range tests {
		got := make(map[string]string, len(test.declaration.Contract.Parameters))
		for _, parameter := range test.declaration.Contract.Parameters {
			got[parameter.Name] = parameter.Property
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s parameter properties = %#v, want %#v", test.declaration.Command, got, test.want)
		}
	}
}
