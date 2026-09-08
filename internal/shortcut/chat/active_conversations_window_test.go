// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"encoding/json"
	"testing"
	"time"
)

func activeConversationWindowGroupForTest(id, name string, singleChat any, times ...any) map[string]any {
	messages := make([]map[string]any, 0, len(times))
	for _, timestamp := range times {
		messages = append(messages, map[string]any{"createTime": timestamp})
	}
	return map[string]any{"openConversationId": id, "title": name, "singleChat": singleChat, "messages": messages}
}

func activeConversationWindowPageForTest(t *testing.T, next string, groups ...map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"result": map[string]any{
		"conversationMessagesList": groups, "hasMore": next != "", "nextCursor": next,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCrossPlatformCoverageActiveConversationsFiltersMessageTimeWindow(t *testing.T) {
	end := time.Date(2026, 9, 7, 12, 0, 0, 0, activeConversationsLocation)
	for _, tc := range []struct {
		name       string
		times      []any
		wantLatest string
	}{
		{name: "before start", times: []any{"2026-09-07T09:59:59+08:00"}},
		{name: "before start by nanosecond", times: []any{"2026-09-07T09:59:59.999999999+08:00"}},
		{name: "equal start", times: []any{"2026-09-07T10:00:00+08:00"}, wantLatest: "2026-09-07T10:00:00+08:00"},
		{name: "inside with subsecond precision", times: []any{"2026-09-07T10:30:00.123456789+08:00"}, wantLatest: "2026-09-07T10:30:00.123456789+08:00"},
		{name: "before end by nanosecond", times: []any{"2026-09-07T11:59:59.999999999+08:00"}, wantLatest: "2026-09-07T11:59:59.999999999+08:00"},
		{name: "equal end", times: []any{"2026-09-07T12:00:00+08:00"}},
		{name: "after end by nanosecond", times: []any{"2026-09-07T12:00:00.000000001+08:00"}},
		{name: "after end", times: []any{"2026-09-07T12:00:01+08:00"}},
		{name: "mixed unsorted messages", times: []any{"2026-09-07T09:59:59+08:00", "2026-09-07T12:00:01+08:00", "2026-09-07T10:30:00.123+08:00", "2026-09-07T10:15:00+08:00"}, wantLatest: "2026-09-07T10:30:00.123+08:00"},
		{name: "equal start in UTC", times: []any{"2026-09-07T02:00:00Z"}, wantLatest: "2026-09-07T10:00:00+08:00"},
		{name: "equal end in UTC", times: []any{"2026-09-07T04:00:00Z"}},
		{name: "equal start unix seconds", times: []any{end.Add(-2 * time.Hour).Unix()}, wantLatest: "2026-09-07T10:00:00+08:00"},
		{name: "equal end unix milliseconds", times: []any{end.UnixMilli()}},
		{name: "before end unix milliseconds", times: []any{end.Add(-time.Millisecond).UnixMilli()}, wantLatest: "2026-09-07T11:59:59.999+08:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{responses: map[string]string{
				activeConversationsOperation: activeConversationWindowPageForTest(t, "", activeConversationWindowGroupForTest("cid-1", "群聊", false, tc.times...)),
			}}
			envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07 10:00:00", "end": "2026-09-07 12:00:00"})
			if err != nil {
				t.Fatal(err)
			}
			data := envelope["data"].(map[string]any)
			rows := data["conversations"].([]any)
			wantCount := 0
			if tc.wantLatest != "" {
				wantCount = 1
			}
			pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
			// Pagination omits zero-valued items; meta.count remains explicit.
			items, _ := pagination["items"].(float64)
			if envelope["outcome"] != "success" || len(rows) != wantCount || data["count"] != float64(wantCount) || envelope["meta"].(map[string]any)["count"] != float64(wantCount) || data["unknownTypeCount"] != float64(0) || data["complete"] != true || data["pagesFetched"] != float64(1) || items != float64(wantCount) || pagination["endpoint_exhausted"] != true {
				t.Fatalf("unexpected filtered result: %#v", envelope)
			}
			if wantCount > 0 && rows[0].(map[string]any)["latestMessageTime"] != tc.wantLatest {
				t.Fatalf("latest message must be the window's maximum %s: %#v", tc.wantLatest, rows[0])
			}
			if data["rangeSemantics"] != "[start,end)" || data["start"] != "2026-09-07T10:00:00+08:00" || data["end"] != "2026-09-07T12:00:00+08:00" || len(caller.calls) != 1 || caller.calls[0].args["startTime"] != "2026-09-07 10:00:00" || caller.calls[0].args["endTime"] != "2026-09-07 12:00:00" {
				t.Fatalf("query boundaries changed: data=%#v calls=%#v", data, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsFilteredPagesPreservePagination(t *testing.T) {
	pages := []string{
		activeConversationWindowPageForTest(t, "cursor-2", activeConversationWindowGroupForTest("outside-first", "", nil, "2026-09-07 09:59:59", "2026-09-07 12:00:00")),
		activeConversationWindowPageForTest(t, "cursor-3", activeConversationWindowGroupForTest("active", "有效群", false, "2026-09-07 11:00:00")),
		activeConversationWindowPageForTest(t, "", activeConversationWindowGroupForTest("active", "不得覆盖", true, "2026-09-07 12:00:01"), activeConversationWindowGroupForTest("outside-last", "", nil, "2026-09-07 09:59:59")),
	}
	for _, tc := range []struct {
		name, pageLimit, cursor, next string
		responseOffset, pages, count  int
		complete, exhausted           bool
	}{
		{name: "stop after filtered page", pageLimit: "1", pages: 1, next: "cursor-2"},
		{name: "stop after active page", pageLimit: "2", pages: 2, count: 1, next: "cursor-3"},
		{name: "all pages", pageLimit: "3", pages: 3, count: 1, complete: true, exhausted: true},
		{name: "resumed batch", pageLimit: "3", cursor: "cursor-2", responseOffset: 1, pages: 2, count: 1, exhausted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{activeConversationsOperation: pages[tc.responseOffset:]}}
			envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07 10:00:00", "end": "2026-09-07 12:00:00", "page-limit": tc.pageLimit, "cursor": tc.cursor})
			if err != nil {
				t.Fatal(err)
			}
			if envelope["outcome"] != "success" {
				t.Fatalf("valid filtered pages failed: %#v", envelope)
			}
			data := envelope["data"].(map[string]any)
			rows := data["conversations"].([]any)
			pagination := envelope["meta"].(map[string]any)["pagination"].(map[string]any)
			items, _ := pagination["items"].(float64)
			if len(rows) != tc.count || data["count"] != float64(tc.count) || envelope["meta"].(map[string]any)["count"] != float64(tc.count) || data["unknownTypeCount"] != float64(0) || data["pagesFetched"] != float64(tc.pages) || data["complete"] != tc.complete || items != float64(tc.count) || pagination["pages"] != float64(tc.pages) || pagination["endpoint_exhausted"] != tc.exhausted || len(caller.calls) != tc.pages {
				t.Fatalf("filtered page changed pagination: result=%#v calls=%#v", envelope, caller.calls)
			}
			if next, _ := pagination["next_token"].(string); next != tc.next {
				t.Fatalf("next token = %q, want %q", next, tc.next)
			}
			if len(rows) > 0 {
				row := rows[0].(map[string]any)
				if row["conversationId"] != "active" || row["latestMessageTime"] != "2026-09-07T11:00:00+08:00" || row["name"] != "有效群" || row["type"] != "group" {
					t.Fatalf("out-of-window page changed valid state: %#v", row)
				}
			}
			for index, call := range caller.calls {
				wantCursor := []string{"0", "cursor-2", "cursor-3"}[tc.responseOffset+index]
				if call.args["startTime"] != "2026-09-07 10:00:00" || call.args["endTime"] != "2026-09-07 12:00:00" || call.args["cursor"] != wantCursor {
					t.Fatalf("query range/cursor changed: %#v", call)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsWindowFilteringPreservesPageAtomicity(t *testing.T) {
	for _, firstTime := range []string{"2026-09-07 09:59:59", "2026-09-07 10:30:00"} {
		t.Run(firstTime, func(t *testing.T) {
			caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{activeConversationsOperation: {
				activeConversationWindowPageForTest(t, "cursor-2", activeConversationWindowGroupForTest("existing", "原名称", false, firstTime)),
				activeConversationWindowPageForTest(t, "",
					activeConversationWindowGroupForTest("existing", "不可提交", false, "2026-09-07 11:00:00"),
					activeConversationWindowGroupForTest("new-valid", "不可新增", false, "2026-09-07 10:45:00"),
					activeConversationWindowGroupForTest("invalid", "", nil, "2026-09-07 09:59:59", "bad-time")),
			}}}
			envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07 10:00:00", "end": "2026-09-07 12:00:00"})
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if firstTime == "2026-09-07 10:30:00" {
				wantCount = 1
			}
			batch := assertActiveConversationsPartialForTest(t, envelope, 1, wantCount, "cursor-2")
			rows := batch["conversations"].([]any)
			if len(rows) != wantCount || batch["unknownTypeCount"] != float64(0) || len(caller.calls) != 2 {
				t.Fatalf("failed page leaked entries: %#v", envelope)
			}
			if wantCount > 0 {
				row := rows[0].(map[string]any)
				if row["conversationId"] != "existing" || row["name"] != "原名称" || row["latestMessageTime"] != "2026-09-07T10:30:00+08:00" {
					t.Fatalf("failed page mutated prior state: %#v", row)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsIgnoresOutsideDuplicateGroups(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{activeConversationsOperation: activeConversationWindowPageForTest(t, "",
		activeConversationWindowGroupForTest("same", "窗外单聊", true, "2026-09-07 09:59:59"),
		activeConversationWindowGroupForTest("same", "窗内群聊", false, "2026-09-07 11:00:00"),
		activeConversationWindowGroupForTest("same", "窗外单聊", true, "2026-09-07 12:00:01"),
	)}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07 10:00:00", "end": "2026-09-07 12:00:00"})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	rows := data["conversations"].([]any)
	if len(rows) != 1 || data["count"] != float64(1) || data["complete"] != true {
		t.Fatalf("filtered duplicates = %#v", data)
	}
	row := rows[0].(map[string]any)
	if row["type"] != "group" || row["name"] != "窗内群聊" || row["latestMessageTime"] != "2026-09-07T11:00:00+08:00" {
		t.Fatalf("outside duplicate polluted state: %#v", row)
	}
}
