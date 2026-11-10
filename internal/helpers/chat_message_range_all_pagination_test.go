package helpers

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// IMS-535: `chat message list-all --page-all` shares the generic paging layer,
// whose error path synthesizes HasMore=true to mean only "not finished".

func rangeAllPage(messages ...string) string {
	rows := make([]string, 0, len(messages))
	for _, id := range messages {
		rows = append(rows, `{"id":"`+id+`"}`)
	}
	return `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[` +
		strings.Join(rows, ",") + `]}]}}`
}

func rangeAllArgs(extra ...string) []string {
	args := []string{
		"message", "list-all",
		"--start", "2026-08-01 00:00:00", "--end", "2026-08-02 00:00:00",
		"--page-all", "--page-delay", "0",
	}
	return append(args, extra...)
}

// runRangeAllPageAll extracts the partial ledger this entry carries on its typed error.
func runRangeAllPageAll(t *testing.T, steps []scriptedToolStep, extra ...string) (map[string]any, int, *apperrors.Error, error) {
	t.Helper()
	caller := &chatMessagePaginationCaller{steps: steps}
	_, err := executeChatMessagePaginationCommand(t, caller, rangeAllArgs(extra...)...)
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		return nil, len(caller.calls), nil, err
	}
	partial, ok := typed.Details["partialResult"].(map[string]any)
	if !ok {
		t.Fatalf("details.partialResult = %#v, want the retained page result", typed.Details["partialResult"])
	}
	return partial, len(caller.calls), typed, nil
}

func assertRangeAllUnknownPagination(t *testing.T, partial map[string]any, calls, wantCalls, wantMessages int, wantFailure string) {
	t.Helper()
	if partial["paginationKnown"] != false {
		t.Fatalf("paginationKnown = %#v, want false: %#v", partial["paginationKnown"], partial)
	}
	if partial["complete"] != false {
		t.Fatalf("complete = %#v, want false", partial["complete"])
	}
	if partial["partial"] != true {
		t.Fatalf("partial = %#v, want true", partial["partial"])
	}
	if partial["stopReason"] != "pagination_error" {
		t.Fatalf("stopReason = %#v, want pagination_error", partial["stopReason"])
	}
	if calls != wantCalls {
		t.Fatalf("lower calls = %d, want %d (sweep must stop)", calls, wantCalls)
	}
	// The original downstream failure survives next to the reliability verdict.
	failures, _ := partial["failures"].([]map[string]any)
	if len(failures) == 0 {
		t.Fatalf("failures = %#v, want the retained downstream error", partial["failures"])
	}
	found := false
	for _, failure := range failures {
		if strings.Contains(fmt.Sprint(failure["error"]), wantFailure) {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %#v, want the original %q", failures, wantFailure)
	}
	if got := rangeAllMessageCount(partial); got != wantMessages {
		t.Fatalf("retained messages = %d, want %d", got, wantMessages)
	}
	// No continuation may be offered: the cursor cannot be trusted to resume.
	if actions, _ := partial["nextActions"].([]map[string]any); len(actions) != 0 {
		t.Fatalf("nextActions = %#v, want none without a reliable cursor", partial["nextActions"])
	}
}

func rangeAllMessageCount(partial map[string]any) int {
	count := 0
	result, _ := partial["result"].(map[string]any)
	groups, _ := result["conversationMessagesList"].([]any)
	for _, raw := range groups {
		group, _ := raw.(map[string]any)
		messages, _ := group["messages"].([]any)
		count += len(messages)
	}
	return count
}

func TestChatMessageRangeAllPageAllReportsUnknownPagination(t *testing.T) {
	for _, tc := range []struct {
		name         string
		steps        []scriptedToolStep
		wantCalls    int
		wantMessages int
		wantFailure  string
	}{
		{
			name: "later page missing hasMore",
			steps: []scriptedToolStep{
				{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`},
				{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m2"}]}]}}`},
			},
			wantCalls: 2, wantMessages: 1, wantFailure: "paged response missing result.hasMore",
		},
		{
			name: "later page hasMore of unreliable type",
			steps: []scriptedToolStep{
				{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`},
				{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m2"}]}],"hasMore":"yes","nextCursor":"c3"}}`},
			},
			wantCalls: 2, wantMessages: 1, wantFailure: "must be boolean",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			partial, calls, typed, err := runRangeAllPageAll(t, tc.steps)
			if err != nil {
				t.Fatalf("unexpected error shape: %v", err)
			}
			if typed.Reason != "pagination_partial_failure" {
				t.Fatalf("reason = %q, want pagination_partial_failure", typed.Reason)
			}
			assertRangeAllUnknownPagination(t, partial, calls, tc.wantCalls, tc.wantMessages, tc.wantFailure)
		})
	}

	// An unparseable first page leaves no envelope, so the command fails closed
	// rather than publishing a ledger that claims the range was read.
	for _, tc := range []struct {
		name        string
		step        string
		wantFailure string
	}{
		{name: "first page missing hasMore", step: rangeAllPage("m1"), wantFailure: "paged response missing result.hasMore"},
		{name: "first page hasMore of unreliable type", step: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":"yes","nextCursor":"c2"}}`, wantFailure: "must be boolean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			partial, calls, typed, err := runRangeAllPageAll(t, []scriptedToolStep{{text: tc.step}})
			if err == nil {
				t.Fatal("unparsable first page unexpectedly succeeded")
			}
			if calls != 1 {
				t.Fatalf("lower calls = %d, want 1 (sweep must stop)", calls)
			}
			if typed != nil || partial != nil {
				t.Fatalf("published a partial ledger without any retained page: %#v", partial)
			}
			if !strings.Contains(err.Error(), tc.wantFailure) {
				t.Fatalf("error = %v, want %q", err, tc.wantFailure)
			}
		})
	}
}

func TestChatMessageRangeAllPageAllKeepsKnownPaginationForOtherStops(t *testing.T) {
	const goodFirst = `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`

	t.Run("request failure after a good page is not unknown pagination", func(t *testing.T) {
		partial, calls, _, err := runRangeAllPageAll(t, []scriptedToolStep{
			{text: goodFirst},
			{err: errors.New("fixture transport failure")},
		})
		if err != nil {
			t.Fatalf("unexpected error shape: %v", err)
		}
		if calls != 2 {
			t.Fatalf("lower calls = %d, want 2", calls)
		}
		// The kept page did state hasMore, so reliability stays true and the
		// stop is reported as a read failure instead of unknown pagination.
		if partial["paginationKnown"] != true || partial["complete"] != false ||
			partial["partial"] != true || partial["stopReason"] != "pagination_error" {
			t.Fatalf("ledger = %#v", partial)
		}
		if got := rangeAllMessageCount(partial); got != 1 {
			t.Fatalf("retained messages = %d, want 1", got)
		}
		paging, _ := partial["paging"].(map[string]any)
		if paging["hasMoreReliable"] != true || paging["resumeCursorReliable"] != true {
			t.Fatalf("paging = %#v, want reliable hasMore and resume cursor", paging)
		}
		actions, _ := partial["nextActions"].([]map[string]any)
		if len(actions) != 1 {
			t.Fatalf("nextActions = %#v, want the safe resume hint", partial["nextActions"])
		}
	})

	t.Run("stalled cursor is not unknown pagination", func(t *testing.T) {
		partial, calls, _, err := runRangeAllPageAll(t, []scriptedToolStep{{text: goodFirst}, {text: goodFirst}})
		if err != nil {
			t.Fatalf("unexpected error shape: %v", err)
		}
		if calls != 2 {
			t.Fatalf("lower calls = %d, want 2", calls)
		}
		if partial["paginationKnown"] != true || partial["complete"] != false ||
			partial["stopReason"] != "pagination_error" {
			t.Fatalf("ledger = %#v", partial)
		}
		if got := rangeAllMessageCount(partial); got != 2 {
			t.Fatalf("retained messages = %d, want 2", got)
		}
		paging, _ := partial["paging"].(map[string]any)
		if paging["hasMoreReliable"] != true {
			t.Fatalf("paging = %#v, want reliable hasMore", paging)
		}
		// The cursor provably does not advance, so no resume hint may be offered.
		if paging["resumeCursorReliable"] != false {
			t.Fatalf("resumeCursorReliable = %#v, want false", paging["resumeCursorReliable"])
		}
		if actions, _ := partial["nextActions"].([]map[string]any); len(actions) != 0 {
			t.Fatalf("nextActions = %#v, want none for a stalled cursor", partial["nextActions"])
		}
	})
}

// TestChatMessageRangeAllPaginationProjection pins the contract the shared
// paging layer feeds: hasMoreReliable decides paginationKnown, and its absence
// on a non-partial envelope keeps the historical reading.
func TestChatMessageRangeAllPaginationProjection(t *testing.T) {
	requestArgs := map[string]any{"startTime": "2026-08-01 00:00:00", "endTime": "2026-08-02 00:00:00"}
	for _, tc := range []struct {
		name      string
		envelope  map[string]any
		wantKnown bool
		wantStop  string
	}{
		{
			name:      "no pagination evidence at all",
			envelope:  map[string]any{"result": map[string]any{}},
			wantKnown: false,
			wantStop:  "pagination_unknown",
		},
		{
			name: "partial envelope with source-backed hasMore",
			envelope: map[string]any{
				"result": map[string]any{"nextCursor": "c2"},
				"paging": map[string]any{
					"hasMore": true, "partial": true, "hasMoreReliable": true,
					"resumeCursorReliable": true, "lastCursor": "c2",
				},
			},
			wantKnown: true,
			wantStop:  "pagination_error",
		},
		{
			name: "partial envelope whose hasMore was synthesized",
			envelope: map[string]any{
				"result": map[string]any{},
				"paging": map[string]any{"hasMore": true, "partial": true, "hasMoreReliable": false, "resumeCursorReliable": false},
			},
			wantKnown: false,
			wantStop:  "pagination_error",
		},
		{
			name: "single page envelope keeps the source hasMore",
			envelope: map[string]any{
				"result": map[string]any{"hasMore": false},
				"paging": map[string]any{"hasMore": false},
			},
			wantKnown: true,
			wantStop:  "source_complete",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projected := projectChatMessageRangeAllPayload(tc.envelope, requestArgs, true)
			if projected["paginationKnown"] != tc.wantKnown {
				t.Fatalf("paginationKnown = %#v, want %v", projected["paginationKnown"], tc.wantKnown)
			}
			if projected["stopReason"] != tc.wantStop {
				t.Fatalf("stopReason = %#v, want %q", projected["stopReason"], tc.wantStop)
			}
			if wantComplete := tc.wantKnown && tc.wantStop == "source_complete"; projected["complete"] != wantComplete {
				t.Fatalf("complete = %#v, want %v", projected["complete"], wantComplete)
			}
			meta, metaErr := chatMessageRangeAllFrameworkMeta(projected)
			if tc.wantKnown {
				if metaErr != nil || meta == nil {
					t.Fatalf("framework meta = %#v, err = %v", meta, metaErr)
				}
				return
			}
			var typed *apperrors.Error
			if !errors.As(metaErr, &typed) || typed.Reason != "invalid_result_pagination" {
				t.Fatalf("framework meta error = %v, want invalid_result_pagination", metaErr)
			}
		})
	}
}

func TestChatMessageRangeAllPageAllKnownPaginationControls(t *testing.T) {
	run := func(t *testing.T, steps []scriptedToolStep, extra ...string) (map[string]any, int, error) {
		t.Helper()
		caller := &chatMessagePaginationCaller{steps: steps}
		got, err := executeChatMessagePaginationCommand(t, caller, rangeAllArgs(extra...)...)
		return got, len(caller.calls), err
	}

	t.Run("explicit hasMore=false on a single page", func(t *testing.T) {
		got, calls, err := run(t, []scriptedToolStep{{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":false,"nextCursor":""}}`}})
		if err != nil || calls != 1 {
			t.Fatalf("calls = %d, err = %v", calls, err)
		}
		paging, _ := got["paging"].(map[string]any)
		if paging["hasMore"] != false || paging["sourceExhausted"] != true ||
			paging["requestSatisfied"] != true || paging["truncated"] != false {
			t.Fatalf("paging = %#v", paging)
		}
		if _, exists := paging["partial"]; exists {
			t.Fatalf("paging = %#v, want no partial block on success", paging)
		}
		if got := rangeAllMessageCount(got); got != 1 {
			t.Fatalf("messages = %d, want 1", got)
		}
	})

	t.Run("normal multi-page read", func(t *testing.T) {
		got, calls, err := run(t, []scriptedToolStep{
			{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`},
			{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m2"}]}],"hasMore":false,"nextCursor":""}}`},
		}, "--page-limit", "5")
		if err != nil || calls != 2 {
			t.Fatalf("calls = %d, err = %v", calls, err)
		}
		paging, _ := got["paging"].(map[string]any)
		if paging["pages"] != float64(2) || paging["hasMore"] != false || paging["truncated"] != false {
			t.Fatalf("paging = %#v", paging)
		}
		if got := rangeAllMessageCount(got); got != 2 {
			t.Fatalf("messages = %d, want 2", got)
		}
	})

	t.Run("page limit truncates without claiming an unknown state", func(t *testing.T) {
		got, calls, err := run(t, []scriptedToolStep{{text: `{"result":{"conversationMessagesList":[{"openConversationId":"cid1","messages":[{"id":"m1"}]}],"hasMore":true,"nextCursor":"c2"}}`}}, "--page-limit", "1")
		if err != nil || calls != 1 {
			t.Fatalf("calls = %d, err = %v", calls, err)
		}
		paging, _ := got["paging"].(map[string]any)
		if paging["hasMore"] != true || paging["truncated"] != true {
			t.Fatalf("paging = %#v", paging)
		}
		if _, exists := paging["partial"]; exists {
			t.Fatalf("paging = %#v, want no partial block for a page limit", paging)
		}
		projected := projectChatMessageRangeAllPayload(got, map[string]any{
			"startTime": "2026-08-01 00:00:00", "endTime": "2026-08-02 00:00:00",
		}, true)
		if projected["paginationKnown"] != true || projected["complete"] != false ||
			projected["stopReason"] != "local_limit" {
			t.Fatalf("projected ledger = %#v", projected)
		}
	})
}
