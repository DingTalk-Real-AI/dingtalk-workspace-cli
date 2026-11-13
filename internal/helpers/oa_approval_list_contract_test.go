// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOAApprovalListFormsDocumentsPaginationFields(t *testing.T) {
	root := newOaCommand()
	leaf, _, err := root.Find([]string{"approval", "list-forms"})
	if err != nil {
		t.Fatalf("find list-forms: %v", err)
	}
	for _, want := range []string{"单页", "result.hasMore", "result.nextCursor"} {
		if !strings.Contains(leaf.Long, want) {
			t.Errorf("list-forms Long = %q, want %q", leaf.Long, want)
		}
	}
}

func TestCrossPlatformCoverageOAApprovalListFormsPreservesPaginationFields(t *testing.T) {
	const response = `{"success":true,"result":{"processCodeList":[{"processCode":"PROC-1","processName":"报销","processIconUrl":"https://example.test/icon.png","dirName":"财务"}],"totalCount":101,"nextCursor":200,"hasMore":true},"errorCode":0,"errorMessage":""}`
	caller := &scriptedToolCaller{
		format: "json",
		steps:  []scriptedToolStep{{text: response}},
	}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "list-forms", "--cursor", "100", "--limit", "100", "--format", "json")
	if err != nil {
		t.Fatalf("execute list-forms: %v", err)
	}
	if caller.calls != 1 || caller.server != "oa" || caller.tool != "list_user_visible_process" {
		t.Fatalf("call = %d %s/%s, want one oa/list_user_visible_process call", caller.calls, caller.server, caller.tool)
	}
	wantArgs := map[string]any{"cursor": float64(100), "pageSize": float64(100)}
	if !reflect.DeepEqual(caller.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", caller.args, wantArgs)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode stdout: %v; stdout=%q", err, stdout)
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want object", payload["result"])
	}
	if result["hasMore"] != true || result["nextCursor"] != float64(200) {
		t.Fatalf("pagination fields = %#v", result)
	}
}

func TestCrossPlatformCoverageOAApprovalListCommandsForwardCurrentMCPFilters(t *testing.T) {
	commonArgs := []string{
		"--page", "2",
		"--limit", "3",
		"--query", "fixture",
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

	tests := []struct {
		name      string
		command   string
		tool      string
		extraArgs []string
		extraWant map[string]any
	}{
		{name: "pending", command: "list-pending", tool: "get_todo_tasks", extraArgs: []string{"--create-before", "2026-08-28"}, extraWant: map[string]any{"createBefore": "2026-08-28"}},
		{name: "executed", command: "list-executed", tool: "get_done_tasks", extraArgs: []string{"--process-instance-status", "COMPLETED"}, extraWant: map[string]any{"processInstanceStatus": "COMPLETED"}},
		{name: "submitted", command: "list-submitted", tool: "get_submitted_instances", extraArgs: []string{"--process-instance-status", "COMPLETED"}, extraWant: map[string]any{"processInstanceStatus": "COMPLETED"}},
		{name: "cc explicit false", command: "list-cc", tool: "get_noticed_instances", extraArgs: []string{"--unread-only=false"}, extraWant: map[string]any{"unreadOnly": false}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			args := append([]string{"approval", test.command}, commonArgs...)
			args = append(args, test.extraArgs...)
			if err := executeOACommand(t, caller, args...); err != nil {
				t.Fatalf("execute %s: %v", test.command, err)
			}
			if caller.server != "oa" || caller.tool != test.tool {
				t.Fatalf("called %s/%s, want oa/%s", caller.server, caller.tool, test.tool)
			}
			want := make(map[string]any, len(commonWant)+len(test.extraWant))
			for key, value := range commonWant {
				want[key] = value
			}
			for key, value := range test.extraWant {
				want[key] = value
			}
			if !reflect.DeepEqual(caller.args, want) {
				t.Fatalf("arguments = %#v, want %#v", caller.args, want)
			}
		})
	}
}

func TestCrossPlatformCoverageOAApprovalListPendingPreservesHistoricalCLIInputs(t *testing.T) {
	root := newOaCommand()
	leaf, _, err := root.Find([]string{"approval", "list-pending"})
	if err != nil {
		t.Fatalf("find list-pending: %v", err)
	}
	for _, name := range []string{"page", "limit", "size"} {
		flag := leaf.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("missing historical --%s", name)
		}
		if flag.Value.Type() != "string" {
			t.Errorf("--%s type = %q, want string", name, flag.Value.Type())
		}
	}
	for _, name := range []string{"start", "end"} {
		flag := leaf.Flags().Lookup(name)
		if flag == nil || flag.Hidden {
			t.Errorf("historical --%s must remain visible and supported", name)
		}
	}

	caller := &scriptedToolCaller{}
	if err := executeOACommand(t, caller,
		"approval", "list-pending",
		"--start", "2026-08-01T00:00:00+08:00",
		"--end", "2026-08-31T23:59:59+08:00",
		"--page", "2", "--size", "3",
	); err != nil {
		t.Fatalf("execute historical list-pending argv: %v", err)
	}
	want := map[string]any{
		"pageNumber":     2,
		"pageSize":       3,
		"createTimeFrom": "2026-08-01",
		"createTimeTo":   "2026-08-31",
	}
	if caller.tool != "get_todo_tasks" || !reflect.DeepEqual(caller.args, want) {
		t.Fatalf("call = %s %#v, want get_todo_tasks %#v", caller.tool, caller.args, want)
	}
}

func TestCrossPlatformCoverageOAApprovalListPendingRejectsMixedLegacyAndDateRanges(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeOACommand(t, caller,
		"approval", "list-pending",
		"--start", "2026-08-01T00:00:00+08:00",
		"--end", "2026-08-31T23:59:59+08:00",
		"--create-time-from", "2026-08-01",
	)
	if err == nil || !strings.Contains(err.Error(), "--start 不能与对应的 yyyy-MM-dd 日期参数同时使用") {
		t.Fatalf("mixed legacy/date range error = %v, want start conflict", err)
	}
	if caller.calls != 0 {
		t.Fatalf("mixed legacy/date range made %d MCP calls", caller.calls)
	}
}

func TestCrossPlatformCoverageOAApprovalListCommandsRejectRetiredCurrentUserAndStatusResultFlags(t *testing.T) {
	retired := map[string][]string{
		"list-pending":   {"user-id", "process-instance-status", "process-instance-result"},
		"list-executed":  {"user-id", "process-instance-result"},
		"list-submitted": {"user-id", "process-instance-result"},
		"list-cc":        {"user-id", "process-instance-status", "process-instance-result"},
	}

	for command, flags := range retired {
		for _, flag := range flags {
			t.Run(command+"/"+flag, func(t *testing.T) {
				root := newOaCommand()
				leaf, _, err := root.Find([]string{"approval", command})
				if err != nil {
					t.Fatalf("find %s: %v", command, err)
				}
				if leaf.Flags().Lookup(flag) != nil {
					t.Fatalf("%s still exposes --%s", command, flag)
				}

				caller := &scriptedToolCaller{}
				err = executeOACommand(t, caller, "approval", command, "--"+flag, "fixture")
				if err == nil || !strings.Contains(err.Error(), "unknown flag: --"+flag) {
					t.Fatalf("%s --%s error = %v, want unknown flag", command, flag, err)
				}
				if caller.calls != 0 {
					t.Fatalf("%s --%s made %d MCP calls", command, flag, caller.calls)
				}
			})
		}
	}
}
