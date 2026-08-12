// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package smart

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"math"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageThreadRepliesResolvesRootMessageIDBeforeReadingReplies(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"messages":[{"openMessageId":"root-message","openConversationId":"cid","openConvThreadId":"thread","createTime":"2026-08-05 16:48:30"}]}}`,
		`{"result":{"hasMore":false,"messages":[{"openMessageId":"reply-1","createTime":"2026-08-05 19:46:00"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+thread-replies", "--message-id", "root-message"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.args) != 2 || !reflect.DeepEqual(caller.args[0]["openMsgIds"], []string{"root-message"}) ||
		caller.args[1]["openconversationId"] != "cid" || caller.args[1]["topicId"] != "thread" ||
		caller.args[1]["forward"] != false {
		t.Fatalf("resolution calls = %#v", caller.args)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if payload["conversationId"] != "cid" || payload["threadId"] != "thread" ||
		payload["resolvedFromMessageId"] != "root-message" || payload["order"] != "desc" ||
		payload["orderScope"] != "complete_result" || payload["count"] != float64(1) {
		t.Fatalf("resolved payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageThreadRepliesMessageResolutionFailsClosedWithoutThreadContext(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"messages":[{"openMessageId":"ordinary-message","openConversationId":"cid"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+thread-replies", "--message-id", "ordinary-message"})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Category != apperrors.CategoryAPI || typed.Reason != "thread_context_missing" {
		t.Fatalf("error = %#v", err)
	}
	if len(caller.args) != 1 || !reflect.DeepEqual(caller.args[0]["openMsgIds"], []string{"ordinary-message"}) {
		t.Fatalf("unexpected fallback calls = %#v", caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesMessageResolutionRejectsConversationMismatch(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"messages":[{"openMessageId":"root-message","openConversationId":"resolved-cid","openConvThreadId":"thread"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+thread-replies", "--message-id", "root-message", "--group", "different-cid",
	})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Category != apperrors.CategoryValidation {
		t.Fatalf("error = %#v", err)
	}
	if len(caller.args) != 1 {
		t.Fatalf("mismatch reached replies/contact fallback: %#v", caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllOrdersCompleteResultAscending(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m3","createTime":"2026-08-06 21:28:41"},{"openMessageId":"m2","createTime":"2026-08-06 21:28:40"}]}}`,
		`{"result":{"hasMore":false,"messages":[{"openMessageId":"m1","createTime":"2026-08-05 19:46:00"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread",
		"--page-all", "--sort", "asc",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	replies, _ := payload["replies"].([]any)
	if len(replies) != 3 || replies[0].(map[string]any)["messageId"] != "m1" ||
		replies[1].(map[string]any)["messageId"] != "m2" ||
		replies[2].(map[string]any)["messageId"] != "m3" || payload["order"] != "asc" ||
		payload["orderScope"] != "complete_result" || payload["complete"] != true {
		t.Fatalf("ascending payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageThreadRepliesAscendingRequiresPageAll(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{`{"result":{"hasMore":false,"messages":[]}}`}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread", "--order", "asc",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("single-page asc unexpectedly succeeded")
	}
	if len(caller.args) != 0 {
		t.Fatalf("invalid asc reached lower read: %#v", caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesThreadSelectorRequiresGroup(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{`{"result":{"hasMore":false,"messages":[]}}`}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+thread-replies", "--thread-id", "thread"})
	if err := root.Execute(); err == nil {
		t.Fatal("thread selector without group unexpectedly succeeded")
	}
	if len(caller.args) != 0 {
		t.Fatalf("invalid selector reached lower read: %#v", caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllUsesMillisecondCursorAndDeduplicates(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m2","createTime":"2026-08-06 21:28:40","content":"同秒第一条"},{"openMessageId":"m1","createTime":"2026-08-06 21:28:40","content":"同秒第二条"}]}}`,
		`{"result":{"hasMore":false,"messages":[{"openMessageId":"m1","createTime":"2026-08-06 21:28:40","content":"重复边界"},{"openMessageId":"m0","createTime":"2026-08-06 21:28:39","content":"下一页"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread",
		"--page-size", "2", "--page-all", "--page-limit", "5", "--no-reactions",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.args) != 2 || caller.args[0]["pageSize"] != 2 ||
		caller.args[0]["forward"] != false || caller.args[1]["startTime"] != "2026-08-06T13:28:39.361Z" {
		t.Fatalf("pagination calls = %#v", caller.args)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if payload["complete"] != true || payload["hasMore"] != false ||
		payload["count"] != float64(3) || payload["pagesFetched"] != float64(2) ||
		payload["stopReason"] != "source_complete" || payload["failedCount"] != float64(0) {
		t.Fatalf("all-page payload = %#v", payload)
	}
	replies, _ := payload["replies"].([]any)
	if len(replies) != 3 || replies[0].(map[string]any)["messageId"] != "m2" ||
		replies[1].(map[string]any)["messageId"] != "m1" ||
		replies[2].(map[string]any)["messageId"] != "m0" {
		t.Fatalf("deduplicated replies = %#v", replies)
	}
}

func TestCrossPlatformCoverageThreadRepliesSinglePagePublishesMillisecondContinuation(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m1","createTime":"2026-08-06 21:28:39"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread", "--page-size", "1",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	next, _ := payload["nextPage"].(map[string]any)
	if payload["complete"] != false || payload["hasMore"] != true ||
		payload["paginationKnown"] != true || payload["failedCount"] != float64(0) ||
		payload["stopReason"] != "single_page" || next["time"] != "2026-08-06T13:28:39.361Z" ||
		next["nextCursor"] != float64(1786022919361) {
		t.Fatalf("single-page payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageThreadRepliesSinglePageFailsClosedWithoutCursor(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1","createTime":"2026-08-06 21:28:39"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread", "--page-size", "1",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if payload["complete"] != false || payload["hasMore"] != true ||
		payload["paginationKnown"] != false || payload["failedCount"] != float64(1) ||
		payload["stopReason"] != "pagination_error" || payload["nextPage"] != nil {
		t.Fatalf("single-page missing-cursor payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllAcceptsEmptyTerminalPage(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m1","createTime":"2026-08-05 16:48:20"}]}}`,
		`{"result":{"hasMore":false,"messages":[]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread",
		"--limit", "1", "--page-all", "--page-limit", "5",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if len(caller.args) != 2 || payload["complete"] != true || payload["hasMore"] != false ||
		payload["count"] != float64(1) || payload["pagesFetched"] != float64(2) {
		t.Fatalf("empty terminal page payload = %#v calls=%#v", payload, caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllPublishesBoundedContinuation(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m1","createTime":"2026-08-05 16:48:20"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread",
		"--page-all", "--page-limit", "1",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	next, _ := payload["nextPage"].(map[string]any)
	if len(caller.args) != 1 || payload["complete"] != false || payload["hasMore"] != true ||
		payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" ||
		next["time"] != "2026-08-06T13:28:39.361Z" || next["direction"] != "older" ||
		next["nextCursor"] != float64(1786022919361) {
		t.Fatalf("bounded payload = %#v calls=%#v", payload, caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllReturnsPartialLedgerOnLaterFailure(t *testing.T) {
	caller := &chatMessagesPagingCaller{
		responses: []string{
			`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m1","createTime":"2026-08-05 16:48:20"}]}}`,
		},
		failAt: 2,
	}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread", "--page-all",
	})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Category != apperrors.CategoryAPI ||
		typed.Reason != "thread_replies_incomplete" || !typed.Retryable ||
		typed.ExecutionStarted == nil || !*typed.ExecutionStarted {
		t.Fatalf("error = %#v", err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if len(caller.args) != 2 || payload["partial"] != true || payload["complete"] != false ||
		payload["count"] != float64(1) || payload["pagesFetched"] != float64(1) ||
		payload["failedCount"] != float64(1) || payload["stopReason"] != "read_failure" {
		t.Fatalf("partial payload = %#v calls=%#v", payload, caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllFailsClosedWithoutCursor(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread", "--page-all",
	})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "thread_replies_incomplete" {
		t.Fatalf("error = %#v", err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if len(caller.args) != 1 || payload["complete"] != false ||
		payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
		t.Fatalf("stalled payload = %#v calls=%#v", payload, caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesPageAllFailsClosedOnStalledCursor(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m2","createTime":"2026-08-06 21:28:40"}]}}`,
		`{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[{"openMessageId":"m1","createTime":"2026-08-06 21:28:39"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread", "--page-all",
	})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "thread_replies_incomplete" {
		t.Fatalf("error = %#v", err)
	}
	payload := decodeThreadRepliesPayload(t, output.Bytes())
	if len(caller.args) != 2 || payload["complete"] != false ||
		payload["count"] != float64(2) || payload["failedCount"] != float64(1) ||
		payload["stopReason"] != "pagination_error" {
		t.Fatalf("stalled cursor payload = %#v calls=%#v", payload, caller.args)
	}
}

func TestCrossPlatformCoverageThreadRepliesNextCursorBoundaryValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		value     any
		wantKey   string
		wantTime  string
		wantError bool
	}{
		{name: "integer", value: int64(1786022919361), wantKey: "1786022919361", wantTime: "2026-08-06T13:28:39.361Z"},
		{name: "native integer", value: int(1786022919361), wantKey: "1786022919361", wantTime: "2026-08-06T13:28:39.361Z"},
		{name: "int32", value: int32(1234), wantKey: "1234", wantTime: "1970-01-01T00:00:01.234Z"},
		{name: "float32", value: float32(1024), wantKey: "1024", wantTime: "1970-01-01T00:00:01.024Z"},
		{name: "json number", value: float64(1786022919361), wantKey: "1786022919361", wantTime: "2026-08-06T13:28:39.361Z"},
		{name: "string", value: "1786022919361", wantKey: "1786022919361", wantTime: "2026-08-06T13:28:39.361Z"},
		{name: "missing", value: nil, wantError: true},
		{name: "fractional", value: 1786022919361.5, wantError: true},
		{name: "float32 nan", value: float32(math.NaN()), wantError: true},
		{name: "float32 infinity", value: float32(math.Inf(1)), wantError: true},
		{name: "zero", value: 0, wantError: true},
		{name: "negative", value: -1, wantError: true},
		{name: "opaque", value: "not-a-millisecond-cursor", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			key, boundary, err := threadRepliesNextCursorBoundary(test.value)
			if test.wantError {
				if err == nil {
					t.Fatalf("threadRepliesNextCursorBoundary(%#v) = %q, %q, nil", test.value, key, boundary)
				}
				return
			}
			if err != nil || key != test.wantKey || boundary != test.wantTime {
				t.Fatalf("threadRepliesNextCursorBoundary(%#v) = %q, %q, %v", test.value, key, boundary, err)
			}
		})
	}
}

func TestCrossPlatformCoverageThreadRepliesAdditionalEdges(t *testing.T) {
	run := func(t *testing.T, caller *chatMessagesPagingCaller, writer any, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(caller)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		if failWriter, ok := writer.(chatMessagesFailWriter); ok {
			root.SetOut(failWriter)
		} else {
			root.SetOut(&output)
		}
		root.SetArgs(append([]string{"chat", "+thread-replies"}, args...))
		err := root.Execute()
		if output.Len() == 0 {
			return nil, err
		}
		return decodeThreadRepliesPayload(t, output.Bytes()), err
	}

	t.Run("explicit time and single read failure", func(t *testing.T) {
		payload, err := run(t, &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":false,"messages":[]}}`,
		}}, nil, "--group", "cid", "--thread-id", "thread", "--time", "2026-08-06")
		if err != nil || payload["complete"] != true {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
		if _, err = run(t, &chatMessagesPagingCaller{failAt: 1}, nil,
			"--group", "cid", "--thread-id", "thread"); err == nil {
			t.Fatal("single-page read failure unexpectedly succeeded")
		}
	})

	t.Run("root-message resolution failures", func(t *testing.T) {
		if _, err := run(t, &chatMessagesPagingCaller{failAt: 1}, nil, "--message-id", "root"); err == nil {
			t.Fatal("message-detail read failure unexpectedly succeeded")
		}
		if _, err := run(t, &chatMessagesPagingCaller{responses: []string{
			`{"result":{"messages":[{"openMessageId":"other"}]}}`,
		}}, nil, "--message-id", "root"); err == nil {
			t.Fatal("missing root message unexpectedly succeeded")
		}
	})

	for _, tc := range []struct {
		name     string
		response string
		wantStop string
	}{
		{name: "unknown pagination", response: `{"result":{"messages":[]}}`, wantStop: "pagination_error"},
		{name: "empty continuation page", response: `{"result":{"hasMore":true,"nextCursor":1786022919361,"messages":[]}}`, wantStop: "pagination_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &chatMessagesPagingCaller{responses: []string{tc.response}}, nil,
				"--group", "cid", "--thread-id", "thread", "--page-all")
			if err == nil || payload["stopReason"] != tc.wantStop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("failure ledger output error", func(t *testing.T) {
		_, err := run(t, &chatMessagesPagingCaller{responses: []string{
			`{"result":{"messages":[]}}`,
		}}, chatMessagesFailWriter{}, "--group", "cid", "--thread-id", "thread", "--page-all")
		if err == nil || err.Error() != "fixture output failure" {
			t.Fatalf("error=%v", err)
		}
	})

	applyThreadRepliesResultContract(nil, nil, threadRepliesTarget{}, "desc")
	if got := threadRepliesString("<nil>"); got != "" {
		t.Fatalf("threadRepliesString(<nil>) = %q", got)
	}
}

func TestCrossPlatformCoverageThreadRepliesPaginationValidationStopsBeforeRead(t *testing.T) {
	for _, args := range [][]string{
		{"--limit", "0"},
		{"--page-size", "0"},
		{"--limit", "1", "--page-size", "1"},
		{"--page-limit", "2"},
		{"--page-all", "--page-limit", "0"},
		{"--page-all", "--page-limit", "501"},
	} {
		caller := &chatMessagesPagingCaller{responses: []string{`{"result":{"hasMore":false,"messages":[]}}`}}
		helpers.InitDeps(caller)
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{
			"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread",
		}, args...))
		if err := root.Execute(); err == nil {
			t.Errorf("invalid pagination succeeded: %v", args)
		}
		if len(caller.args) != 0 {
			t.Errorf("invalid pagination reached read: %v calls=%#v", args, caller.args)
		}
	}
}

func decodeThreadRepliesPayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode thread replies payload: %v\n%s", err, raw)
	}
	return payload
}
