// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package oa

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageOAEmptyPageCompatibilityEnvelope(t *testing.T) {
	tests := []struct {
		declaration shortcut.Shortcut
		tool        string
		collection  string
		args        []string
	}{
		{ListPending, "get_todo_tasks", "instances", []string{"--start", "1785513600000", "--end", "1788191999000", "--page", "2"}},
		{ListExecuted, "get_done_tasks", "instances", []string{"--page", "2"}},
		{ListSubmitted, "get_submitted_instances", "instances", []string{"--page", "2"}},
		{PendingApprovals, "get_todo_tasks", "pending", nil},
		{DoneApprovals, "get_done_tasks", "done", nil},
		{MyInitiated, "get_submitted_instances", "initiated", []string{"--page", "2"}},
	}
	for _, test := range tests {
		for _, success := range []string{`true`, `"true"`} {
			t.Run(test.declaration.Command+"/success="+success, func(t *testing.T) {
				caller := &oaCoverageCaller{responses: map[string][]string{
					test.tool: {`{"success":` + success + `,"result":{"values":[]}}`},
				}}
				cmd, err := runOACoverage(t, test.declaration, caller, test.args...)
				if err != nil {
					t.Fatalf("empty page failed: %v", err)
				}
				if !reflect.DeepEqual(caller.history, []string{test.tool}) {
					t.Fatalf("calls=%v; empty-page compatibility must not make another request", caller.history)
				}
				var stdout bytes.Buffer
				cmd.SetOut(&stdout)
				code, emitted, err := output.EmitStoredResult(cmd)
				if err != nil || !emitted || code != 0 {
					t.Fatalf("emit code=%d emitted=%v err=%v", code, emitted, err)
				}
				var envelope map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope["ok"] != true || envelope["outcome"] != "success" {
					t.Fatalf("unexpected success envelope: %s", stdout.String())
				}
				data, ok := envelope["data"].(map[string]any)
				if !ok || data["count"] != float64(0) || data["complete"] != true {
					t.Fatalf("unexpected empty-page data: %s", stdout.String())
				}
				items, ok := data[test.collection].([]any)
				if !ok || items == nil || len(items) != 0 {
					t.Fatalf("%s must be an explicit empty array: %s", test.collection, stdout.String())
				}
				if _, present := data["nextPage"]; present {
					t.Fatalf("empty terminal page has nextPage: %s", stdout.String())
				}
				meta, ok := envelope["meta"].(map[string]any)
				if !ok {
					t.Fatalf("missing result metadata: %s", stdout.String())
				}
				pagination, present := meta["pagination"]
				if test.declaration.Contract.Pagination == nil {
					if present {
						t.Fatalf("non-paginated compatibility command gained pagination: %s", stdout.String())
					}
					return
				}
				page, ok := pagination.(map[string]any)
				if !ok || page["endpoint_exhausted"] != true {
					t.Fatalf("terminal pagination missing: %s", stdout.String())
				}
				if _, present := page["next_token"]; present {
					t.Fatalf("empty terminal page has next_token: %s", stdout.String())
				}
			})
		}
	}
}

func TestCrossPlatformCoverageOAEmptyPageCompatibilityPreservesValidation(t *testing.T) {
	for _, test := range []struct {
		declaration shortcut.Shortcut
		tool        string
		args        []string
	}{
		{ListPending, "get_todo_tasks", []string{"--start", "1785513600000", "--end", "1788191999000"}},
		{ListExecuted, "get_done_tasks", nil},
		{ListSubmitted, "get_submitted_instances", nil},
	} {
		for name, response := range map[string]string{
			"nonempty missing hasMore": `{"success":true,"result":{"values":[{"processInstanceId":"i"}]}}`,
			"missing success":          `{"result":{"values":[]}}`,
			"business failure":         `{"success":false,"result":{"values":[]}}`,
			"missing values":           `{"success":true,"result":{}}`,
			"null values":              `{"success":true,"result":{"values":null}}`,
			"object values":            `{"success":true,"result":{"values":{}}}`,
			"bad item":                 `{"success":true,"result":{"values":[{}]}}`,
			"null hasMore":             `{"success":true,"result":{"values":[],"hasMore":null}}`,
			"string hasMore":           `{"success":true,"result":{"values":[],"hasMore":"false"}}`,
			"continuation conflict":    `{"success":true,"result":{"values":[],"nextCursor":"2"}}`,
			"malformed continuation":   `{"success":true,"result":{"values":[],"nextCursor":false}}`,
		} {
			t.Run(test.declaration.Command+"/"+name, func(t *testing.T) {
				caller := &oaCoverageCaller{responses: map[string][]string{test.tool: {response}}}
				if _, err := runOACoverage(t, test.declaration, caller, test.args...); err == nil {
					t.Fatalf("malformed or incomplete response succeeded: %s", response)
				}
				if !reflect.DeepEqual(caller.history, []string{test.tool}) {
					t.Fatalf("calls=%v; expected one failed read", caller.history)
				}
			})
		}
	}
	for _, test := range []struct {
		declaration shortcut.Shortcut
		tool        string
		response    string
	}{
		{ListCc, "get_noticed_instances", `{"success":true,"result":{"values":[]}}`},
		{ListForms, "list_user_visible_process", `{"success":true,"result":{"processCodeList":[]}}`},
	} {
		t.Run("unconfirmed endpoint/"+test.tool, func(t *testing.T) {
			caller := &oaCoverageCaller{responses: map[string][]string{test.tool: {test.response}}}
			if _, err := runOACoverage(t, test.declaration, caller); err == nil {
				t.Fatalf("unconfirmed endpoint %s accepted missing hasMore", test.tool)
			}
		})
	}
}

func TestCrossPlatformCoverageOAEmptyPageCompatibilityPaginationSignals(t *testing.T) {
	const operation = "oa/get_submitted_instances"
	for name, result := range map[string]map[string]any{
		"missing nextCursor": {"values": []any{}},
		"null nextCursor":    {"values": []any{}, "nextCursor": nil},
		"blank nextCursor":   {"values": []any{}, "nextCursor": " \t"},
	} {
		t.Run(name, func(t *testing.T) {
			page, err := oaHasMorePage(result, operation, 2)
			if err != nil || !page.Known || page.HasMore || page.Next != "" {
				t.Fatalf("empty terminal page=%+v err=%v", page, err)
			}
		})
	}
	for name, result := range map[string]map[string]any{
		"nil slice":           {"values": []any(nil)},
		"numeric nextCursor":  {"values": []any{}, "nextCursor": 2},
		"zero nextCursor":     {"values": []any{}, "nextCursor": 0},
		"object nextCursor":   {"values": []any{}, "nextCursor": map[string]any{}},
		"array nextCursor":    {"values": []any{}, "nextCursor": []any{}},
		"boolean nextCursor":  {"values": []any{}, "nextCursor": true},
		"nonempty nextCursor": {"values": []any{}, "nextCursor": "3"},
	} {
		t.Run(name, func(t *testing.T) {
			if page, err := oaHasMorePage(result, operation, 2); err == nil {
				t.Fatalf("unexpected terminal-page inference: %+v", page)
			}
		})
	}
	for _, pageNumber := range []int{0, -1} {
		if page, err := oaHasMorePage(map[string]any{"values": []any{}}, operation, pageNumber); err == nil {
			t.Fatalf("invalid current page %d accepted: %+v", pageNumber, page)
		}
	}
	for _, hasMore := range []bool{false, true} {
		page, err := oaHasMorePage(map[string]any{"values": []any{}, "hasMore": hasMore}, operation, 2)
		if err != nil || !page.Known || page.HasMore != hasMore {
			t.Fatalf("explicit hasMore=%v was changed: page=%+v err=%v", hasMore, page, err)
		}
		if hasMore && page.Next != "3" {
			t.Fatalf("explicit continuation was lost: %+v", page)
		}
	}
}

func TestCrossPlatformCoverageOAEmptyPendingCannotApprove(t *testing.T) {
	caller := &oaCoverageCaller{responses: map[string][]string{
		"get_todo_tasks": {`{"success":true,"result":{"values":[]}}`},
	}}
	err := runOAConfirmedCoverage(t, caller, "--keyword", "fixture", "--yes")
	if err == nil || !strings.Contains(err.Error(), "当前匹配 0 条") {
		t.Fatalf("empty pending list must produce zero matches: %v", err)
	}
	if !reflect.DeepEqual(caller.history, []string{"get_todo_tasks"}) {
		t.Fatalf("empty pending search made further calls: %v", caller.history)
	}
}
