// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageAITableQueryPublishesCursorInvalidation(t *testing.T) {
	var description string
	for _, flag := range RecordQuery.Flags {
		if flag.Name == "cursor" {
			description = flag.Desc
		}
	}
	for _, code := range []string{"INVALID_CURSOR", "CURSOR_SNAPSHOT_CHANGED", "CURSOR_SNAPSHOT_UNAVAILABLE"} {
		if !strings.Contains(description, code) {
			t.Errorf("cursor discovery omits runtime recovery code %s", code)
		}
	}
}

// 窗口查询和全量查询共用同一恢复要求：失败时不能输出先前页，也不能自动重启。
func TestCrossPlatformCoverageAITableQueryRejectsStalePages(t *testing.T) {
	for _, all := range []bool{false, true} {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"records":[{"recordId":"old"}],"nextCursor":"stale"}`},
			{text: `{"status":"error","error":{"code":"CURSOR_SNAPSHOT_CHANGED","message":"changed"}}`},
		}}
		args := []string{"--base-id", "b", "--table-id", "t", "--limit", "40"}
		if all {
			args = append(args, "--all")
		}
		out, err := runAITableCompositeCLI(t, caller, "+record-query", args...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Retryable || typed.Details["discard_previous_results"] != true || out != "" || len(caller.calls) != 2 {
			t.Fatalf("all=%t unsafe query: out=%q err=%#v calls=%d", all, out, err, len(caller.calls))
		}
	}
}

func TestCrossPlatformCoverageAITableBaseListRetainsEmptyPageContinuation(t *testing.T) {
	for _, command := range []string{"+base-list", "+base-search"} {
		for _, tc := range []struct {
			raw, cursor string
			fail        bool
		}{
			{`{"data":{"bases":[],"nextCursor":"next"}}`, "", false},
			{`{"bases":[],"hasMore":true}`, "", true},
			{`{"bases":[],"hasMore":true,"nextCursor":"next"}`, "next", true},
			{`{"bases":[],"hasMore":false,"nextCursor":"next"}`, "next", false},
		} {
			c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: tc.raw}}}
			args := []string{}
			if command == "+base-search" {
				args = append(args, "--query", "name")
			}
			if tc.cursor != "" {
				args = append(args, "--cursor", tc.cursor)
			}
			out, err := runAITableCompositeCLI(t, c, command, args...)
			if (err != nil) != tc.fail || len(c.calls) != 1 {
				t.Fatalf("command=%s out=%s err=%v calls=%d", command, out, err, len(c.calls))
			}
			if tc.fail {
				if out != "" {
					t.Fatal("invalid page emitted success", out)
				}
				continue
			}
			if tc.cursor == "" && (!strings.Contains(out, `"nextCursor": "next"`) || !strings.Contains(out, `"hasMore": true`)) {
				t.Fatal(out)
			}
		}
	}
}

func TestCrossPlatformCoverageAITableFieldDescriptionPreservesEmpty(t *testing.T) {
	for _, value := range []string{"meaning", " spaced ", ""} {
		c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true}`}}}
		_, err := runAITableCompositeCLI(t, c, "+field-update", "--base-id", "b", "--table-id", "t", "--field-id", "f", "--description", value, "--yes")
		if err != nil || len(c.calls) != 1 || c.calls[0].args["description"] != value {
			t.Fatal(err, c.calls)
		}
	}
}

func TestCrossPlatformCoverageAITableParityAliasesReachSameRequest(t *testing.T) {
	c := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"fields":[{"fieldId":"f1"},{"fieldId":"f2"}]}`}}}
	_, err := runAITableCompositeCLI(t, c, "+field-list", "--base-id", "b", "--table-id", "t", "--field-ids", "f1,f2")
	if err != nil || len(c.calls) != 1 || c.calls[0].args["baseId"] != "b" {
		t.Fatal(err, c.calls)
	}
	ids := c.calls[0].args["fieldIds"].([]string)
	if len(ids) != 2 || ids[0] != "f1" || ids[1] != "f2" {
		t.Fatal(ids)
	}
	c = &upsertByKeyCaller{}
	_, err = runAITableCompositeCLI(t, c, "+record-query", "--base-id", "b", "--table-id", "t", "--page-size", "2", "--limit", "1")
	if err == nil || len(c.calls) != 0 {
		t.Fatal("conflicting alias reached network", err, c.calls)
	}
	c = &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"bases":[]}`}}}
	_, err = runAITableCompositeCLI(t, c, "+title-resolve", "--keyword", "missing")
	if err != nil || c.calls[0].args["query"] != "missing" {
		t.Fatal(err, c.calls)
	}
}
