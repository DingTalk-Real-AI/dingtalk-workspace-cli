// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

const (
	directMessageTool = "chat/list_individual_chat_message"
	chatListAllTool   = "im/list_my_groups_pagination"
)

// runPaginationKnownCase returns the published ledger and the paged-tool call
// count, so a stopped sweep can be proved to have issued no further reads.
func runPaginationKnownCase(t *testing.T, fake *larkAlignmentCaller, tool string, args ...string) (map[string]any, int, error) {
	t.Helper()
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs(args)
	err := corecmd.ExecuteForTest(root)
	if output.Len() == 0 {
		return nil, toolCallCount(fake, tool), err
	}
	var payload map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stdout JSON = %q, err = %v", output.String(), decodeErr)
	}
	return payload, toolCallCount(fake, tool), err
}

func toolCallCount(fake *larkAlignmentCaller, tool string) int {
	count := 0
	for _, call := range fake.calls {
		if call.product+"/"+call.tool == tool {
			count++
		}
	}
	return count
}

// unknownPaginationLedger is what an entry must publish once the downstream
// stopped returning a usable hasMore.
type unknownPaginationLedger struct {
	calls   int
	itemKey string
	items   int
	failure string
	// retryCursor must point back at the failed page, never past unread
	// records, so it is empty when no page succeeded.
	retryCursor string
}

// assertPaginationUnknownLedger is the shared IMS-535 assertion: unknown
// pagination must publish paginationKnown=false and complete=false, keep the
// records and original failure already read, and offer no skipping continuation.
func assertPaginationUnknownLedger(t *testing.T, payload map[string]any, calls int, want unknownPaginationLedger) {
	t.Helper()
	if payload == nil {
		t.Fatal("no ledger was published")
	}
	if payload["paginationKnown"] != false {
		t.Fatalf("paginationKnown = %#v, want false: %#v", payload["paginationKnown"], payload)
	}
	if payload["complete"] != false {
		t.Fatalf("complete = %#v, want false", payload["complete"])
	}
	if payload["stopReason"] != "pagination_error" {
		t.Fatalf("stopReason = %#v, want pagination_error", payload["stopReason"])
	}
	if payload["failedCount"] != float64(1) {
		t.Fatalf("failedCount = %#v, want 1", payload["failedCount"])
	}
	if calls != want.calls {
		t.Fatalf("lower calls = %d, want %d (sweep must stop)", calls, want.calls)
	}
	failures, ok := payload["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures = %#v, want exactly one entry", payload["failures"])
	}
	failure, _ := failures[0].(map[string]any)
	if failure["stage"] != "pagination" || failure["error"] != want.failure {
		t.Fatalf("failure = %#v, want the original pagination error", failure)
	}
	items, _ := payload[want.itemKey].([]any)
	if len(items) != want.items {
		t.Fatalf("%s = %#v, want %d retained records", want.itemKey, items, want.items)
	}
	if want.items > 0 && payload["partial"] != true {
		t.Fatalf("partial = %#v, want true with retained records", payload["partial"])
	}
	assertContinuationCursor(t, payload, want.retryCursor)
}

// assertContinuationCursor proves the published continuation matches cursor
// reliability: absent when nothing trustworthy was read, else the retry point.
func assertContinuationCursor(t *testing.T, payload map[string]any, want string) {
	t.Helper()
	for _, key := range []string{"nextPage", "nextCursor"} {
		published, exists := payload[key]
		if !exists || published == nil {
			continue
		}
		if want == "" {
			t.Fatalf("%s = %#v, want no continuation without a reliable cursor", key, published)
		}
		got := published
		if page, ok := published.(map[string]any); ok {
			got = page["nextCursor"]
		}
		if continuationCursorKey(got) != want {
			t.Fatalf("%s cursor = %#v, want retry point %q", key, got, want)
		}
	}
}

func continuationCursorKey(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func TestCrossPlatformCoverageDirectMessagesPageAllReportsUnknownPagination(t *testing.T) {
	const (
		missingFirst = `{"result":{"messages":[{"openMessageId":"m1"}]}}`
		missingLater = `{"result":{"messages":[{"openMessageId":"m2"}]}}`
		typedWrong   = `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":"yes","nextCursor":1785919699136}}`
		goodFirst    = `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`
	)
	base := []string{
		"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-08-05 16:49:00",
		"--page-all", "--page-limit", "5",
	}
	for _, tc := range []struct {
		name      string
		responses []string
		want      unknownPaginationLedger
	}{
		{
			name:      "first page missing hasMore",
			responses: []string{missingFirst},
			want:      unknownPaginationLedger{calls: 1, itemKey: "messages", items: 1},
		},
		{
			name:      "later page missing hasMore",
			responses: []string{goodFirst, missingLater},
			want:      unknownPaginationLedger{calls: 2, itemKey: "messages", items: 2, retryCursor: "1785919699136"},
		},
		{
			name:      "hasMore of unreliable type",
			responses: []string{typedWrong},
			want:      unknownPaginationLedger{calls: 1, itemKey: "messages", items: 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{directMessageTool: tc.responses}}
			payload, calls, err := runPaginationKnownCase(t, fake, directMessageTool, base...)
			if err == nil {
				t.Fatal("unknown pagination unexpectedly succeeded")
			}
			tc.want.failure = "单聊消息下层未返回可靠的 hasMore，无法证明结果完整"
			assertPaginationUnknownLedger(t, payload, calls, tc.want)
		})
	}
}

func TestCrossPlatformCoverageDirectMessagesPageAllKeepsKnownPaginationForOtherStops(t *testing.T) {
	const goodFirst = `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`
	base := []string{
		"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-08-05 16:49:00",
		"--page-all",
	}
	for _, tc := range []struct {
		name      string
		fake      *larkAlignmentCaller
		args      []string
		wantStop  string
		wantCalls int
		wantNext  bool
	}{
		{
			name:      "request failure after a good page is not unknown pagination",
			fake:      &larkAlignmentCaller{sequenceResponses: map[string][]string{directMessageTool: {goodFirst}}, failProductToolAt: map[string]int{directMessageTool: 2}},
			wantStop:  "read_failure",
			wantCalls: 2,
			wantNext:  true,
		},
		{
			name:      "missing cursor is not unknown pagination",
			fake:      &larkAlignmentCaller{responses: map[string]string{directMessageTool: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true}}`}},
			wantStop:  "pagination_error",
			wantCalls: 1,
		},
		{
			name:      "stalled cursor is not unknown pagination",
			fake:      &larkAlignmentCaller{sequenceResponses: map[string][]string{directMessageTool: {goodFirst, goodFirst}}},
			args:      []string{"--page-limit", "5"},
			wantStop:  "pagination_error",
			wantCalls: 2,
			wantNext:  true,
		},
		{
			name:      "page limit keeps a reliable continuation",
			fake:      &larkAlignmentCaller{responses: map[string]string{directMessageTool: goodFirst}},
			args:      []string{"--page-limit", "1"},
			wantStop:  "page_limit",
			wantCalls: 1,
			wantNext:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, base...), tc.args...)
			payload, calls, _ := runPaginationKnownCase(t, tc.fake, directMessageTool, args...)
			// The downstream did return an explicit hasMore here, so the ledger
			// keeps paginationKnown=true and distinguishes the stop by reason.
			if payload["paginationKnown"] != true {
				t.Fatalf("paginationKnown = %#v, want true: %#v", payload["paginationKnown"], payload)
			}
			if payload["stopReason"] != tc.wantStop || payload["complete"] != false {
				t.Fatalf("ledger = %#v, want stopReason %q", payload, tc.wantStop)
			}
			if calls != tc.wantCalls {
				t.Fatalf("lower calls = %d, want %d", calls, tc.wantCalls)
			}
			_, hasNext := payload["nextPage"]
			if hasNext != tc.wantNext {
				t.Fatalf("nextPage present = %v, want %v: %#v", hasNext, tc.wantNext, payload["nextPage"])
			}
		})
	}
}

func TestCrossPlatformCoverageDirectMessagesPageAllKnownPaginationControls(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, extra ...string) (map[string]any, int) {
		t.Helper()
		args := append([]string{
			"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-08-05 16:49:00",
			"--page-all",
		}, extra...)
		payload, calls, err := runPaginationKnownCase(t, fake, directMessageTool, args...)
		if err != nil {
			t.Fatalf("control case unexpectedly failed: %v (%#v)", err, payload)
		}
		return payload, calls
	}

	t.Run("explicit hasMore=false on a single page", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			directMessageTool: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`,
		}}
		payload, calls := run(t, fake)
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["hasMore"] != false || payload["stopReason"] != "source_complete" || calls != 1 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})

	t.Run("normal multi-page read", func(t *testing.T) {
		fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{directMessageTool: {
			`{"result":{"messages":[{"openMessageId":"m2"}],"hasMore":true,"nextCursor":1785919699136}}`,
			`{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":false}}`,
		}}}
		payload, calls := run(t, fake, "--page-limit", "5")
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["pagesFetched"] != float64(2) || payload["count"] != float64(2) || calls != 2 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})
}

func TestCrossPlatformCoverageChatListAllPageAllReportsUnknownPagination(t *testing.T) {
	const (
		missingFirst = `{"result":{"groups":[{"openConversationId":"g1"}]}}`
		missingLater = `{"result":{"groups":[{"openConversationId":"g2"}]}}`
		typedWrong   = `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":"yes","nextCursor":88}}`
		goodFirst    = `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":88}}`
	)
	base := []string{"chat", "+chat-list-all", "--page-all", "--page-limit", "5", "--page-delay", "0"}
	for _, tc := range []struct {
		name      string
		responses []string
		want      unknownPaginationLedger
	}{
		{
			name:      "first page missing hasMore",
			responses: []string{missingFirst},
			want:      unknownPaginationLedger{calls: 1, itemKey: "groups", items: 1},
		},
		{
			name:      "later page missing hasMore",
			responses: []string{goodFirst, missingLater},
			want:      unknownPaginationLedger{calls: 2, itemKey: "groups", items: 2, retryCursor: "88"},
		},
		{
			name:      "hasMore of unreliable type",
			responses: []string{typedWrong},
			want:      unknownPaginationLedger{calls: 1, itemKey: "groups", items: 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{chatListAllTool: tc.responses}}
			payload, calls, err := runPaginationKnownCase(t, fake, chatListAllTool, base...)
			if err == nil {
				t.Fatal("unknown pagination unexpectedly succeeded")
			}
			tc.want.failure = "已加入群列表下层未返回可靠的 hasMore，无法证明结果完整"
			assertPaginationUnknownLedger(t, payload, calls, tc.want)
		})
	}
}

func TestCrossPlatformCoverageChatListAllPageAllKeepsKnownPaginationForOtherStops(t *testing.T) {
	const goodFirst = `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":88}}`
	base := []string{"chat", "+chat-list-all", "--page-all", "--page-delay", "0"}
	for _, tc := range []struct {
		name        string
		fake        *larkAlignmentCaller
		args        []string
		wantStop    string
		wantCalls   int
		wantCursor  bool
		wantPartial bool
	}{
		{
			name:        "request failure after a good page is not unknown pagination",
			fake:        &larkAlignmentCaller{sequenceResponses: map[string][]string{chatListAllTool: {goodFirst}}, failProductToolAt: map[string]int{chatListAllTool: 2}},
			wantStop:    "read_failure",
			wantCalls:   2,
			wantCursor:  true,
			wantPartial: true,
		},
		{
			name:        "missing cursor is not unknown pagination",
			fake:        &larkAlignmentCaller{responses: map[string]string{chatListAllTool: `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true}}`}},
			wantStop:    "pagination_error",
			wantCalls:   1,
			wantPartial: true,
		},
		{
			name:        "stalled cursor is not unknown pagination",
			fake:        &larkAlignmentCaller{sequenceResponses: map[string][]string{chatListAllTool: {goodFirst, goodFirst}}},
			args:        []string{"--page-limit", "5"},
			wantStop:    "pagination_error",
			wantCalls:   2,
			wantCursor:  true,
			wantPartial: true,
		},
		{
			name:       "page limit keeps a reliable continuation",
			fake:       &larkAlignmentCaller{responses: map[string]string{chatListAllTool: goodFirst}},
			args:       []string{"--page-limit", "1"},
			wantStop:   "page_limit",
			wantCalls:  1,
			wantCursor: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, base...), tc.args...)
			payload, calls, _ := runPaginationKnownCase(t, tc.fake, chatListAllTool, args...)
			if payload["paginationKnown"] != true {
				t.Fatalf("paginationKnown = %#v, want true: %#v", payload["paginationKnown"], payload)
			}
			if payload["stopReason"] != tc.wantStop || payload["complete"] != false {
				t.Fatalf("ledger = %#v, want stopReason %q", payload, tc.wantStop)
			}
			if payload["partial"] != tc.wantPartial {
				t.Fatalf("partial = %#v, want %v", payload["partial"], tc.wantPartial)
			}
			if calls != tc.wantCalls {
				t.Fatalf("lower calls = %d, want %d", calls, tc.wantCalls)
			}
			_, hasCursor := payload["nextCursor"]
			if hasCursor != tc.wantCursor {
				t.Fatalf("nextCursor present = %v, want %v: %#v", hasCursor, tc.wantCursor, payload["nextCursor"])
			}
		})
	}
}

func TestCrossPlatformCoverageChatListAllPageAllKnownPaginationControls(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, extra ...string) (map[string]any, int) {
		t.Helper()
		args := append([]string{"chat", "+chat-list-all", "--page-all", "--page-delay", "0"}, extra...)
		payload, calls, err := runPaginationKnownCase(t, fake, chatListAllTool, args...)
		if err != nil {
			t.Fatalf("control case unexpectedly failed: %v (%#v)", err, payload)
		}
		return payload, calls
	}

	t.Run("explicit hasMore=false on a single page", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			chatListAllTool: `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":false}}`,
		}}
		payload, calls := run(t, fake)
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["hasMore"] != false || payload["stopReason"] != "source_complete" || calls != 1 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})

	t.Run("normal multi-page read", func(t *testing.T) {
		fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{chatListAllTool: {
			`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":88}}`,
			`{"result":{"groups":[{"openConversationId":"g2"}],"hasMore":false}}`,
		}}}
		payload, calls := run(t, fake, "--page-limit", "5")
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["pagesFetched"] != float64(2) || payload["count"] != float64(2) || calls != 2 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})
}
