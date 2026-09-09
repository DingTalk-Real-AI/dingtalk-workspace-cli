// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageActiveConversationsDefaultStartUsesEffectiveEnd(t *testing.T) {
	now := time.Date(2026, 9, 8, 15, 30, 45, 987_000_000, activeConversationsLocation)
	for _, tc := range []struct{ name, start, end, wantStart, wantEnd string }{
		{"no boundaries", "", "", "2026-09-07T15:30:45+08:00", "2026-09-08T15:30:45+08:00"},
		{"historical local end", "", "2026-09-01 15:30:00", "2026-08-31T15:30:00+08:00", "2026-09-01T15:30:00+08:00"},
		{"date end", "", "2026-09-08", "2026-09-07T00:00:00+08:00", "2026-09-08T00:00:00+08:00"},
		{"UTC end", "", "2026-09-08T07:30:00Z", "2026-09-07T15:30:00+08:00", "2026-09-08T15:30:00+08:00"},
		{"offset end", "", "2026-09-08T00:30:00-07:00", "2026-09-07T15:30:00+08:00", "2026-09-08T15:30:00+08:00"},
		{"zero fraction end", "", "2026-09-08T15:30:00.000+08:00", "2026-09-07T15:30:00+08:00", "2026-09-08T15:30:00+08:00"},
		{"leap day", "", "2024-03-01", "2024-02-29T00:00:00+08:00", "2024-03-01T00:00:00+08:00"},
		{"year boundary", "", "2026-01-01", "2025-12-31T00:00:00+08:00", "2026-01-01T00:00:00+08:00"},
		{"explicit start default end", "2026-09-01", "", "2026-09-01T00:00:00+08:00", "2026-09-08T15:30:45+08:00"},
		{"explicit range unchanged", "2026-09-08T02:00:00Z", "2026-09-08 12:00:00", "2026-09-08T10:00:00+08:00", "2026-09-08T12:00:00+08:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, err := resolveActiveConversationTimeRange(tc.start, tc.end, now)
			if err != nil {
				t.Fatal(err)
			}
			if got := query.start.In(activeConversationsLocation).Format(time.RFC3339); got != tc.wantStart {
				t.Fatalf("start = %s, want %s", got, tc.wantStart)
			}
			if got := query.end.In(activeConversationsLocation).Format(time.RFC3339); got != tc.wantEnd {
				t.Fatalf("end = %s, want %s", got, tc.wantEnd)
			}
			if tc.start == "" && query.end.Sub(query.start) != 24*time.Hour {
				t.Fatalf("default range is not exactly 24 hours: %#v", query)
			}
			if query.start.Nanosecond() != 0 || query.end.Nanosecond() != 0 {
				t.Fatalf("fractional default boundary: %#v", query)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsNoStartPagesShareReturnedWindow(t *testing.T) {
	for _, cursor := range []string{"", "0", "   "} {
		t.Run("cursor="+cursor, func(t *testing.T) {
			caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{
				activeConversationsOperation: {
					`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":"next"}}`,
					`{"result":{"conversationMessagesList":[],"hasMore":false}}`,
				},
			}}
			values := map[string]string{}
			if cursor != "" {
				values["cursor"] = cursor
			}
			before := time.Now().Truncate(time.Second)
			envelope, err := executeActiveConversationsForTest(t, caller, values)
			after := time.Now().Truncate(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			data := envelope["data"].(map[string]any)
			start, err := time.Parse(time.RFC3339, data["start"].(string))
			if err != nil {
				t.Fatal(err)
			}
			end, err := time.Parse(time.RFC3339, data["end"].(string))
			if err != nil || end.Before(before) || end.After(after) || end.Sub(start) != 24*time.Hour {
				t.Fatalf("unexpected default window: data=%#v err=%v", data, err)
			}
			if data["complete"] != true || data["count"] != float64(0) || len(caller.calls) != 2 {
				t.Fatalf("default paging result = %#v, calls=%#v", data, caller.calls)
			}
			for _, call := range caller.calls {
				if call.args["startTime"] != start.Format("2006-01-02 15:04:05") || call.args["endTime"] != end.Format("2006-01-02 15:04:05") {
					t.Fatalf("request boundaries differ from returned window: %#v", call)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsEndOnlyFiltersAndResumes(t *testing.T) {
	caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		activeConversationsOperation: {
			`{"result":{"conversationMessagesList":[{"openConversationId":"outside-before","messages":[{"createTime":"2026-09-07T15:29:59.999+08:00"}]},{"openConversationId":"inside","messages":[{"createTime":"2026-09-07T15:30:00+08:00"}]}],"hasMore":true,"nextCursor":"next"}}`,
			`{"result":{"conversationMessagesList":[{"openConversationId":"inside","messages":[{"createTime":"2026-09-08T15:29:59.999+08:00"},{"createTime":"2026-09-08T15:30:00+08:00"}]},{"openConversationId":"outside-after","messages":[{"createTime":"2026-09-08T15:30:01+08:00"}]}],"hasMore":false}}`,
		},
	}}
	first, err := executeActiveConversationsForTest(t, caller, map[string]string{"end": "2026-09-08 15:30:00", "page-limit": "1"})
	if err != nil {
		t.Fatal(err)
	}
	data := first["data"].(map[string]any)
	if data["start"] != "2026-09-07T15:30:00+08:00" || data["end"] != "2026-09-08T15:30:00+08:00" || data["complete"] != false || data["count"] != float64(1) {
		t.Fatalf("first batch = %#v", data)
	}
	firstRow := data["conversations"].([]any)[0].(map[string]any)
	if firstRow["conversationId"] != "inside" || firstRow["latestMessageTime"] != data["start"] {
		t.Fatalf("start boundary was not included: %#v", firstRow)
	}
	paging := first["meta"].(map[string]any)["pagination"].(map[string]any)
	second, err := executeActiveConversationsForTest(t, caller, map[string]string{
		"start": data["start"].(string), "end": data["end"].(string), "cursor": paging["next_token"].(string),
	})
	if err != nil {
		t.Fatal(err)
	}
	resumed := second["data"].(map[string]any)
	if resumed["start"] != data["start"] || resumed["end"] != data["end"] || resumed["complete"] != false || resumed["count"] != float64(1) {
		t.Fatalf("resumed batch = %#v", resumed)
	}
	row := resumed["conversations"].([]any)[0].(map[string]any)
	if row["conversationId"] != "inside" || row["latestMessageTime"] != "2026-09-08T15:29:59.999+08:00" {
		t.Fatalf("out-of-window message entered the resumed result: %#v", row)
	}
	for _, call := range caller.calls {
		if call.args["startTime"] != "2026-09-07 15:30:00" || call.args["endTime"] != "2026-09-08 15:30:00" {
			t.Fatalf("resumed request changed default window: %#v", call)
		}
	}
}

func TestCrossPlatformCoverageActiveConversationsDefaultDoesNotMaskInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		values     map[string]string
	}{
		{"empty start", "不能为空白", map[string]string{"start": ""}},
		{"whitespace start", "不能为空白", map[string]string{"start": " \t "}},
		{"bad start", "RFC3339", map[string]string{"start": "yesterday"}},
		{"fractional start", "整秒", map[string]string{"start": "2026-09-07T10:00:00.001+08:00"}},
		{"end-only fractional", "整秒", map[string]string{"end": "2026-09-08T15:30:00.001+08:00"}},
		{"end-only invalid", "RFC3339", map[string]string{"end": "yesterday"}},
		{"resume without boundaries", "同一 --end", map[string]string{"cursor": "next"}},
		{"resume without start", "同一 --start", map[string]string{"end": "2026-09-08", "cursor": "next"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{}
			_, err := executeActiveConversationsForTest(t, caller, tc.values)
			if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || !strings.Contains(err.Error(), tc.want) || len(caller.calls) != 0 {
				t.Fatalf("error=%v, calls=%#v; want validation containing %q before RPC", err, caller.calls, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsDefaultWindowIsCachedAndRefreshed(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":false}}`,
	}}
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	rt := shortcut.RuntimeContextForTest(cmd, ActiveConversations)
	if err := cmd.Flags().Set("end", "2026-09-08 15:30:00"); err != nil {
		t.Fatal(err)
	}
	if err := ActiveConversations.Validate(rt); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("end", "2026-09-09 15:30:00"); err != nil {
		t.Fatal(err)
	}
	// Execution must use the validated pair, not recompute either boundary.
	if err := ActiveConversations.Execute(rt); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["startTime"] != "2026-09-07 15:30:00" || caller.calls[0].args["endTime"] != "2026-09-08 15:30:00" {
		t.Fatalf("validated default window changed: %#v", caller.calls)
	}
	// Revalidation of a reused command must derive a new start from its new end.
	if err := ActiveConversations.Validate(rt); err != nil {
		t.Fatal(err)
	}
	query, err := activeConversationExecutionRange(rt)
	if err != nil || query.start.Format("2006-01-02 15:04:05") != "2026-09-08 15:30:00" || query.end.Format("2006-01-02 15:04:05") != "2026-09-09 15:30:00" {
		t.Fatalf("stale default window = %#v, err=%v", query, err)
	}
}

func TestCrossPlatformCoverageActiveConversationsCobraAcceptsNoStart(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":false}}`,
	}}
	helpers.InitDepsForTest(t, caller)
	root := newPlatformCoverageRoot()
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetArgs([]string{"chat", "+recent-conversations"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["startTime"] == "" || caller.calls[0].args["endTime"] == "" {
		t.Fatalf("no-argument invocation did not supply a concrete range: %#v", caller.calls)
	}
}
