// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

// runPaginationKnownCase returns the published ledger and the lower-call count,
// so a stopped sweep can be proved to have issued no further reads.
func runPaginationKnownCase(t *testing.T, caller *chatMessagesPagingCaller, args ...string) (map[string]any, int, error) {
	t.Helper()
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs(args)
	err := corecmd.ExecuteForTest(root)
	if output.Len() == 0 {
		return nil, len(caller.args), err
	}
	var payload map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stdout JSON = %q, err = %v", output.String(), decodeErr)
	}
	return payload, len(caller.args), err
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
	for _, key := range []string{"nextPage", "nextCursor"} {
		published, exists := payload[key]
		if !exists || published == nil || published == "" {
			continue
		}
		if want.retryCursor == "" {
			t.Fatalf("%s = %#v, want no continuation without a reliable cursor", key, published)
		}
		if jsonCursorString(published) != want.retryCursor {
			t.Fatalf("%s = %#v, want retry point %q", key, published, want.retryCursor)
		}
	}
}

func jsonCursorString(value any) string {
	number, ok := value.(float64)
	if !ok {
		return fmt.Sprint(value)
	}
	return strconv.FormatInt(int64(number), 10)
}

func TestCrossPlatformCoverageAtMePageAllReportsUnknownPagination(t *testing.T) {
	const (
		missingFirst = `{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}]}}`
		missingLater = `{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m2"}]}]}}`
		typedWrong   = `{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}],"hasMore":"yes","nextCursor":"cursor-2"}}`
		goodFirst    = `{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`
	)
	base := []string{"chat", "+at-me", "--page-all", "--page-limit", "5", "--page-delay", "0"}
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
			want:      unknownPaginationLedger{calls: 2, itemKey: "messages", items: 2, retryCursor: "cursor-2"},
		},
		{
			name:      "hasMore of unreliable type",
			responses: []string{typedWrong},
			want:      unknownPaginationLedger{calls: 1, itemKey: "messages", items: 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &chatMessagesPagingCaller{responses: tc.responses}
			payload, calls, err := runPaginationKnownCase(t, caller, base...)
			if err == nil {
				t.Fatal("unknown pagination unexpectedly succeeded")
			}
			tc.want.failure = "@我消息下层未返回可靠的 hasMore，无法证明结果完整"
			assertPaginationUnknownLedger(t, payload, calls, tc.want)
		})
	}
}

func TestCrossPlatformCoverageAtMePageAllKeepsKnownPaginationForOtherStops(t *testing.T) {
	const goodFirst = `{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`
	base := []string{"chat", "+at-me", "--page-all", "--page-delay", "0"}
	for _, tc := range []struct {
		name       string
		caller     *chatMessagesPagingCaller
		args       []string
		wantStop   string
		wantCalls  int
		wantCursor bool
	}{
		{
			name:       "request failure after a good page is not unknown pagination",
			caller:     &chatMessagesPagingCaller{responses: []string{goodFirst}, failAt: 2},
			wantStop:   "read_failure",
			wantCalls:  2,
			wantCursor: true,
		},
		{
			name:      "missing cursor is not unknown pagination",
			caller:    &chatMessagesPagingCaller{responses: []string{`{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}],"hasMore":true}}`}},
			wantStop:  "pagination_error",
			wantCalls: 1,
		},
		{
			name:       "stalled cursor is not unknown pagination",
			caller:     &chatMessagesPagingCaller{responses: []string{goodFirst, goodFirst}},
			args:       []string{"--page-limit", "5"},
			wantStop:   "pagination_error",
			wantCalls:  2,
			wantCursor: true,
		},
		{
			name:       "page limit keeps a reliable continuation",
			caller:     &chatMessagesPagingCaller{responses: []string{goodFirst}},
			args:       []string{"--page-limit", "1"},
			wantStop:   "page_limit",
			wantCalls:  1,
			wantCursor: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, base...), tc.args...)
			payload, calls, _ := runPaginationKnownCase(t, tc.caller, args...)
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
			_, hasCursor := payload["nextCursor"]
			if hasCursor != tc.wantCursor {
				t.Fatalf("nextCursor present = %v, want %v: %#v", hasCursor, tc.wantCursor, payload["nextCursor"])
			}
		})
	}
}

func TestCrossPlatformCoverageAtMePageAllKnownPaginationControls(t *testing.T) {
	run := func(t *testing.T, caller *chatMessagesPagingCaller, extra ...string) (map[string]any, int) {
		t.Helper()
		args := append([]string{"chat", "+at-me", "--page-all", "--page-delay", "0"}, extra...)
		payload, calls, err := runPaginationKnownCase(t, caller, args...)
		if err != nil {
			t.Fatalf("control case unexpectedly failed: %v (%#v)", err, payload)
		}
		return payload, calls
	}

	t.Run("explicit hasMore=false on a single page", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}],"hasMore":false}}`,
		}}
		payload, calls := run(t, caller)
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["hasMore"] != false || payload["stopReason"] != "source_complete" || calls != 1 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})

	t.Run("normal multi-page read", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m2"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`,
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[{"openMessageId":"m1"}]}],"hasMore":false}}`,
		}}
		payload, calls := run(t, caller, "--page-limit", "5")
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["pagesFetched"] != float64(2) || payload["count"] != float64(2) || calls != 2 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})
}

func TestCrossPlatformCoverageMyGroupsPageAllReportsUnknownPagination(t *testing.T) {
	const (
		missingFirst = `{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}]}}`
		missingLater = `{"result":{"groups":[{"openConversationId":"g2","groupType":"group"}]}}`
		typedWrong   = `{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}],"hasMore":"yes","nextCursor":88}}`
		goodFirst    = `{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}],"hasMore":true,"nextCursor":88}}`
	)
	base := []string{"chat", "+my-groups", "--type", "group", "--page-all", "--page-limit", "5", "--page-delay", "0"}
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
			caller := &chatMessagesPagingCaller{responses: tc.responses}
			payload, calls, err := runPaginationKnownCase(t, caller, base...)
			if err == nil {
				t.Fatal("unknown pagination unexpectedly succeeded")
			}
			tc.want.failure = "我的群列表下层未返回可靠的 hasMore，无法证明结果完整"
			assertPaginationUnknownLedger(t, payload, calls, tc.want)
		})
	}
}

func TestCrossPlatformCoverageMyGroupsPageAllKeepsKnownPaginationForOtherStops(t *testing.T) {
	const goodFirst = `{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}],"hasMore":true,"nextCursor":88}}`
	base := []string{"chat", "+my-groups", "--type", "group", "--page-all", "--page-delay", "0"}
	for _, tc := range []struct {
		name       string
		caller     *chatMessagesPagingCaller
		args       []string
		wantStop   string
		wantCalls  int
		wantCursor bool
	}{
		{
			name:       "request failure after a good page is not unknown pagination",
			caller:     &chatMessagesPagingCaller{responses: []string{goodFirst}, failAt: 2},
			wantStop:   "read_failure",
			wantCalls:  2,
			wantCursor: true,
		},
		{
			name:      "missing cursor is not unknown pagination",
			caller:    &chatMessagesPagingCaller{responses: []string{`{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}],"hasMore":true}}`}},
			wantStop:  "pagination_error",
			wantCalls: 1,
		},
		{
			name:       "stalled cursor is not unknown pagination",
			caller:     &chatMessagesPagingCaller{responses: []string{goodFirst, goodFirst}},
			args:       []string{"--page-limit", "5"},
			wantStop:   "pagination_error",
			wantCalls:  2,
			wantCursor: true,
		},
		{
			name:       "page limit keeps a reliable continuation",
			caller:     &chatMessagesPagingCaller{responses: []string{goodFirst}},
			args:       []string{"--page-limit", "1"},
			wantStop:   "page_limit",
			wantCalls:  1,
			wantCursor: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, base...), tc.args...)
			payload, calls, _ := runPaginationKnownCase(t, tc.caller, args...)
			if payload["paginationKnown"] != true {
				t.Fatalf("paginationKnown = %#v, want true: %#v", payload["paginationKnown"], payload)
			}
			if payload["stopReason"] != tc.wantStop || payload["complete"] != false {
				t.Fatalf("ledger = %#v, want stopReason %q", payload, tc.wantStop)
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

func TestCrossPlatformCoverageMyGroupsPageAllKnownPaginationControls(t *testing.T) {
	run := func(t *testing.T, caller *chatMessagesPagingCaller, extra ...string) (map[string]any, int) {
		t.Helper()
		args := append([]string{"chat", "+my-groups", "--type", "group", "--page-all", "--page-delay", "0"}, extra...)
		payload, calls, err := runPaginationKnownCase(t, caller, args...)
		if err != nil {
			t.Fatalf("control case unexpectedly failed: %v (%#v)", err, payload)
		}
		return payload, calls
	}

	t.Run("explicit hasMore=false on a single page", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}],"hasMore":false}}`,
		}}
		payload, calls := run(t, caller)
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["hasMore"] != false || payload["stopReason"] != "source_complete" || calls != 1 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})

	t.Run("normal multi-page read", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"groups":[{"openConversationId":"g1","groupType":"group"}],"hasMore":true,"nextCursor":88}}`,
			`{"result":{"groups":[{"openConversationId":"g2","groupType":"group"}],"hasMore":false}}`,
		}}
		payload, calls := run(t, caller, "--page-limit", "5")
		if payload["paginationKnown"] != true || payload["complete"] != true ||
			payload["pagesFetched"] != float64(2) || payload["count"] != float64(2) || calls != 2 {
			t.Fatalf("payload = %#v, calls = %d", payload, calls)
		}
	})
}
