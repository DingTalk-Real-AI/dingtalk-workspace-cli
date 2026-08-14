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
	"context"
	"encoding/json"
	stderrors "errors"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// TestProjectChatMessageExpandsForwarded guards that a forwarded chat record
// ("聊天记录") exposes its nested messages under "forwarded" instead of
// collapsing to the lossy top-level "[卡片]" summary, recursing through nested
// forwards, and that the string-"null" sender is nulled out. The per-field
// behaviour (sender/text/encryption) is covered in the chatmsg package tests.
func TestCrossPlatformCoverageProjectChatMessageExpandsForwarded(t *testing.T) {
	row := projectChatMessage(map[string]any{
		"sender":             "hugozhu",
		"openMessageId":      "msg-top",
		"openConversationId": "cid-top",
		"openConvThreadId":   "thread-top",
		"msgType":            "text",
		"content":            "hugozhu与opencode-agent的聊天记录\nopencode-agent:[卡片]",
		"createTime":         "2026-07-20 21:41:21",
		"emotionReplyList": []any{
			map[string]any{"emoji": "赞", "replyUsers": []any{"D1"}},
		},
		"forwardMessages": []any{
			map[string]any{"sender": "null", "content": "读下冬翔发给我的最近两条消息", "createTime": "2026-07-20 09:30:33"},
			map[string]any{"sender": "冬翔", "content": "W29 工作总结", "createTime": "2026-07-19 23:35:40",
				// nested forward inside a forward — must expand recursively.
				"forwardMessages": []any{
					map[string]any{"sender": "念晨", "content": "收到", "createTime": "2026-07-19 23:36:00"},
				},
			},
		},
	})

	if row["sender"] != "hugozhu" {
		t.Fatalf("top sender = %v, want hugozhu", row["sender"])
	}
	if row["messageId"] != "msg-top" || row["conversationId"] != "cid-top" || row["messageType"] != "text" {
		t.Errorf("stable identity = %#v", row)
	}
	if row["threadId"] != "thread-top" {
		t.Errorf("thread identity = %#v", row)
	}
	if reactions, ok := row["reactions"].(map[string]any); !ok || len(reactions) == 0 {
		t.Errorf("reactions = %#v", row["reactions"])
	}
	forwarded, ok := row["forwarded"].([]map[string]any)
	if !ok || len(forwarded) != 2 {
		t.Fatalf("forwarded = %#v, want 2 entries", row["forwarded"])
	}
	if forwarded[0]["sender"] != nil {
		t.Errorf("forwarded[0].sender = %v, want nil (string \"null\")", forwarded[0]["sender"])
	}
	if forwarded[0]["text"] != "读下冬翔发给我的最近两条消息" {
		t.Errorf("forwarded[0].text = %v", forwarded[0]["text"])
	}
	nested, ok := forwarded[1]["forwarded"].([]map[string]any)
	if !ok || len(nested) != 1 || nested[0]["sender"] != "念晨" {
		t.Errorf("nested forwarded = %#v, want 1 entry from 念晨", forwarded[1]["forwarded"])
	}

	// A plain message must not grow a "forwarded" key.
	plain := projectChatMessage(map[string]any{"sender": "念晨", "content": "hi", "createTime": "t"})
	if _, has := plain["forwarded"]; has {
		t.Errorf("plain message unexpectedly has forwarded key: %#v", plain)
	}

	withoutReactions := projectChatMessageWithReactions(map[string]any{
		"content": "hi",
		"emotionReplyList": []any{
			map[string]any{"emoji": "赞", "replyUsers": []any{"D1"}},
		},
	}, false)
	if _, has := withoutReactions["reactions"]; has {
		t.Errorf("no-reactions projection leaked reactions: %#v", withoutReactions)
	}
}

type chatMessagesPagingCaller struct {
	responses []string
	args      []map[string]any
	failAt    int
}

type chatMessagesFailWriter struct{}

func (chatMessagesFailWriter) Write([]byte) (int, error) {
	return 0, stderrors.New("fixture output failure")
}

func (c *chatMessagesPagingCaller) CallTool(
	_ context.Context,
	_, _ string,
	args map[string]any,
) (*edition.ToolResult, error) {
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.args = append(c.args, copied)
	index := len(c.args) - 1
	if c.failAt > 0 && len(c.args) == c.failAt {
		return nil, stderrors.New("fixture read failure")
	}
	if index >= len(c.responses) {
		index = len(c.responses) - 1
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{
		Type: "text",
		Text: c.responses[index],
	}}}, nil
}

func (c *chatMessagesPagingCaller) CallReadTool(
	ctx context.Context,
	product, tool string,
	args map[string]any,
) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, args)
}

func (*chatMessagesPagingCaller) Format() string { return "json" }
func (*chatMessagesPagingCaller) DryRun() bool   { return false }
func (*chatMessagesPagingCaller) Fields() string { return "" }
func (*chatMessagesPagingCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageChatMessagesPageAllUsesTypedBoundaryAndDeduplicates(t *testing.T) {
	const cursorMillis int64 = 1767225600123
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1767225600123,"messages":[{"openMessageId":"m2","createTime":"2026-01-02 00:00:00"},{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"}]}}`,
		`{"result":{"hasMore":false,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"},{"openMessageId":"m0","createTime":"2025-12-31 00:00:00"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--time", "2026-01-03 00:00:00", "--page-all", "--page-limit", "5",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	wantBoundary := time.UnixMilli(cursorMillis).UTC().Format(time.RFC3339Nano)
	if len(caller.args) != 2 || caller.args[1]["time"] != wantBoundary {
		t.Fatalf("pagination calls = %#v", caller.args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != true || payload["hasMore"] != false ||
		payload["count"] != float64(3) || payload["pagesFetched"] != float64(2) ||
		payload["stopReason"] != "source_complete" {
		t.Fatalf("all-page payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatMessagesMillisecondCursorDoesNotSkipSameSecondRows(t *testing.T) {
	const cursorMillis int64 = 1785919699136
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1785919699136,"messages":[{"openMessageId":"m4","createTime":"2026-08-05 16:48:19"},{"openMessageId":"m3","createTime":"2026-08-05 16:48:19"},{"openMessageId":"m2","createTime":"2026-08-05 16:48:19"}]}}`,
		`{"result":{"hasMore":false,"messages":[{"openMessageId":"m1","createTime":"2026-08-05 16:48:19"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid", "--time", "2026-08-05 16:49:00",
		"--page-size", "3", "--page-all", "--page-limit", "5",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	wantBoundary := time.UnixMilli(cursorMillis).UTC().Format(time.RFC3339Nano)
	if len(caller.args) != 2 || caller.args[1]["time"] != wantBoundary {
		t.Fatalf("pagination calls = %#v", caller.args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(4) || payload["complete"] != true || payload["pagesFetched"] != float64(2) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatMessagesDescendingRangeStopsAtInclusiveStart(t *testing.T) {
	const cursorMillis int64 = 1767312000456
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1767312000456,"messages":[{"openMessageId":"m3","createTime":"2026-01-03 00:00:00"},{"openMessageId":"m2","createTime":"2026-01-02 00:00:00"}]}}`,
		`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 12:00:00"},{"openMessageId":"m0","createTime":"2026-01-01 11:59:59"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--start", "2026-01-01 12:00:00", "--end", "2026-01-04 00:00:00",
		"--order", "desc", "--page-all", "--page-limit", "5",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	wantBoundary := time.UnixMilli(cursorMillis).UTC().Format(time.RFC3339Nano)
	if len(caller.args) != 2 || caller.args[0]["time"] != "2026-01-04 00:00:00" ||
		caller.args[0]["forward"] != false || caller.args[1]["time"] != wantBoundary {
		t.Fatalf("range calls = %#v", caller.args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != true || payload["hasMore"] != false ||
		payload["count"] != float64(3) || payload["stopReason"] != "range_start" {
		t.Fatalf("range payload = %#v", payload)
	}
	messages := payload["messages"].([]any)
	if messages[0].(map[string]any)["messageId"] != "m3" ||
		messages[2].(map[string]any)["messageId"] != "m1" {
		t.Fatalf("descending messages = %#v", messages)
	}
	rangeMeta := payload["queryRange"].(map[string]any)
	if rangeMeta["semantics"] != "[start,end)" || rangeMeta["order"] != "desc" {
		t.Fatalf("queryRange = %#v", rangeMeta)
	}
}

func TestCrossPlatformCoverageChatMessagesAscendingRangeStopsAtExclusiveEnd(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 12:00:00"},{"openMessageId":"m2","createTime":"2026-01-02 00:00:00"},{"openMessageId":"m-end","createTime":"2026-01-03 00:00:00"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--start-time", "2026-01-01 12:00:00", "--end-time", "2026-01-03 00:00:00",
		"--sort", "asc", "--page-all",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.args) != 1 || caller.args[0]["time"] != "2026-01-01 12:00:00" ||
		caller.args[0]["forward"] != true {
		t.Fatalf("ascending call = %#v", caller.args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != true || payload["hasMore"] != false ||
		payload["count"] != float64(2) || payload["stopReason"] != "range_end" {
		t.Fatalf("ascending payload = %#v", payload)
	}
	messages := payload["messages"].([]any)
	if messages[0].(map[string]any)["messageId"] != "m1" ||
		messages[1].(map[string]any)["messageId"] != "m2" {
		t.Fatalf("ascending messages = %#v", messages)
	}
}

func TestCrossPlatformCoverageChatMessagesFirstReadFailureSkipsOptionalSenderResolution(t *testing.T) {
	caller := &chatMessagesPagingCaller{
		responses: []string{`{"result":[]}`},
		failAt:    1,
	}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid", "--page-all",
		"--sender-query", "张三",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("first-page failure unexpectedly succeeded")
	}
	if len(caller.args) != 1 {
		t.Fatalf("calls after first read failure = %#v, want only the message read", caller.args)
	}
}

func TestCrossPlatformCoverageChatMessagesRangeValidationStopsBeforeRead(t *testing.T) {
	for _, args := range [][]string{
		{"--start", "2026-01-02", "--end", "2026-01-01"},
		{"--end", "2026-01-02", "--order", "asc"},
		{"--time", "2026-01-02", "--start", "2026-01-01"},
		{"--direction", "newer", "--start", "2026-01-01"},
		{"--start", "2026-01-01", "--start-time", "2026-01-01"},
	} {
		caller := &chatMessagesPagingCaller{responses: []string{`{"result":{"hasMore":false,"messages":[]}}`}}
		helpers.InitDeps(caller)
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+chat-messages", "--conversation-id", "cid"}, args...))
		if err := root.Execute(); err == nil {
			t.Errorf("invalid range succeeded: %v", args)
		}
		if len(caller.args) != 0 {
			t.Errorf("invalid range reached message read: %v calls=%#v", args, caller.args)
		}
	}
}

func TestCrossPlatformCoverageChatMessagesPageAllPublishesBoundedContinuation(t *testing.T) {
	const cursorMillis int64 = 1767225600123
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"nextCursor":1767225600123,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--page-all", "--page-limit", "1",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	next, _ := payload["nextPage"].(map[string]any)
	wantBoundary := time.UnixMilli(cursorMillis).UTC().Format(time.RFC3339Nano)
	if payload["complete"] != false || payload["hasMore"] != true ||
		payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" ||
		next["time"] != wantBoundary || next["nextCursor"] != float64(cursorMillis) {
		t.Fatalf("bounded payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatMessagesPageAllFailsClosedOnStalledBoundary(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--page-all"})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Category != apperrors.CategoryAPI ||
		typed.Reason != "chat_messages_incomplete" || !typed.Retryable ||
		typed.ExecutionStarted == nil || !*typed.ExecutionStarted {
		t.Fatalf("error = %#v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["failedCount"] != float64(1) ||
		payload["stopReason"] != "pagination_error" {
		t.Fatalf("stalled payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatMessagesFailedPageDoesNotExportPartialLedger(t *testing.T) {
	t.Chdir(t.TempDir())
	caller := &chatMessagesPagingCaller{
		responses: []string{
			`{"result":{"hasMore":true,"nextCursor":1767225600123,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"}]}}`,
		},
		failAt: 2,
	}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid", "--page-all",
		"--output", "exports/partial.json",
	})
	err := root.Execute()
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "chat_messages_incomplete" {
		t.Fatalf("error = %#v", err)
	}
	if _, statErr := os.Lstat("exports/partial.json"); !os.IsNotExist(statErr) {
		t.Fatalf("partial export exists: %v", statErr)
	}
	var ledger map[string]any
	if err := json.Unmarshal(output.Bytes(), &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger["partial"] != true || ledger["failedCount"] != float64(1) || ledger["count"] != float64(1) {
		t.Fatalf("failure ledger = %#v", ledger)
	}
}

func TestCrossPlatformCoverageChatMessagesFailureLedgerOutputErrorIsNonZero(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1"}]}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetOut(chatMessagesFailWriter{})
	root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--page-all"})
	if err := root.Execute(); err == nil || err.Error() != "fixture output failure" {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossPlatformCoverageChatMessagesExportIsAtomicAndNoClobber(t *testing.T) {
	t.Chdir(t.TempDir())
	newCaller := func() *chatMessagesPagingCaller {
		return &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":false,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"}]}}`,
		}}
	}
	run := func(overwrite bool) error {
		helpers.InitDeps(newCaller())
		root := newPlatformCoverageRoot()
		root.SetOut(&bytes.Buffer{})
		args := []string{"chat", "+chat-messages", "--conversation-id", "cid", "--page-all", "--output", "exports/messages.json"}
		if overwrite {
			args = append(args, "--overwrite")
		}
		root.SetArgs(args)
		return root.Execute()
	}
	if err := run(false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("exports/messages.json")
	if err != nil {
		t.Fatal(err)
	}
	var exported map[string]any
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatal(err)
	}
	if exported["complete"] != true || exported["count"] != float64(1) {
		t.Fatalf("exported ledger = %#v", exported)
	}
	if err := run(false); err == nil {
		t.Fatal("existing export was overwritten without --overwrite")
	}
	if err := run(true); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageChatMessagesExportRejectsNonJSONPlaceholder(t *testing.T) {
	t.Chdir(t.TempDir())
	helpers.InitDeps(&chatMessagesPagingCaller{responses: []string{
		`{"result":{"hasMore":false,"messages":[]}}`,
	}})
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{
		"chat", "+chat-messages", "--conversation-id", "cid", "--output", "{}",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("non-JSON placeholder output unexpectedly succeeded")
	}
	if _, err := os.Lstat("{}"); !os.IsNotExist(err) {
		t.Fatalf("placeholder output was created: %v", err)
	}
}

func chatMessagesRuntimeForTest(t *testing.T, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	root := newPlatformCoverageRoot()
	cmd, _, err := root.Find([]string{"chat", "+chat-messages"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%q: %v", name, value, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, ChatMessages)
}

func TestCrossPlatformCoverageChatMessagesKeepsMaxResultsPublic(t *testing.T) {
	root := newPlatformCoverageRoot()
	cmd, _, err := root.Find([]string{"chat", "+chat-messages"})
	if err != nil {
		t.Fatal(err)
	}
	flag := cmd.Flags().Lookup("max-results")
	if flag == nil || flag.Hidden {
		t.Fatalf("--max-results must remain a visible compatibility flag: %#v", flag)
	}
}

func TestCrossPlatformCoverageChatMessagesAdditionalValidationAndHelpers(t *testing.T) {
	for _, values := range []map[string]string{
		{"max-results": "1"},
		{"max-items": "1"},
		{"page-all": "true", "max-results": "-1"},
		{"page-all": "true", "max-items": "1", "max-results": "1"},
	} {
		if err := validateChatMessages(chatMessagesRuntimeForTest(t, values)); err == nil {
			t.Fatalf("pagination validation unexpectedly accepted %#v", values)
		}
	}
	for _, values := range []map[string]string{
		{"time": "2026-01-01", "start": "2026-01-01"},
		{"direction": "older", "start": "2026-01-01"},
	} {
		if err := validateChatMessages(chatMessagesRuntimeForTest(t, values)); err == nil {
			t.Fatalf("direct validation unexpectedly accepted %#v", values)
		}
	}
	if _, err := resolveChatMessagesRequest(chatMessagesRuntimeForTest(t, map[string]string{
		"conversation-id": "cid", "start": "not-a-time",
	})); err == nil {
		t.Fatal("invalid direct request range unexpectedly succeeded")
	}

	helpers.InitDeps(&platformCoverageCaller{
		contactSearchResult: `{"result":[{"name":"测试用户甲"}]}`,
	})
	filter := resolveOptionalChatMessagesSenderFilter(chatMessagesRuntimeForTest(t, map[string]string{
		"sender-query": "测试用户甲",
	}))
	if filter.applied || filter.resolutionFailure == nil {
		t.Fatalf("identity-free sender resolution = %#v", filter)
	}

	for _, tc := range []struct {
		name      string
		value     any
		wantError bool
	}{
		{name: "int", value: int(1234)},
		{name: "int32", value: int32(1234)},
		{name: "int64", value: int64(1234)},
		{name: "float32", value: float32(1024)},
		{name: "float64", value: float64(1234)},
		{name: "string", value: "1234"},
		{name: "float32 nan", value: float32(math.NaN()), wantError: true},
		{name: "float32 infinity", value: float32(math.Inf(1)), wantError: true},
		{name: "float64 fractional", value: 1.5, wantError: true},
		{name: "bad string", value: "opaque", wantError: true},
		{name: "missing", value: nil, wantError: true},
		{name: "zero", value: 0, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := chatMessagesNextCursorBoundary(tc.value)
			if (err != nil) != tc.wantError {
				t.Fatalf("value=%#v err=%v wantError=%v", tc.value, err, tc.wantError)
			}
		})
	}
}

func TestChatMessagesSenderScopeIgnoresNonMatchingIdentityFamily(t *testing.T) {
	filter := chatMessagesSenderFilter{
		requested: true,
		applied:   true,
		inputs:    []string{"DAAAAAAAAAAAiE"},
		inputMode: "sender",
		stableIDs: map[string]bool{"DAAAAAAAAAAAiE": true},
	}
	payload := map[string]any{
		"complete": true,
		"failures": []map[string]any{},
	}
	filtered := applyOptionalChatMessagesSenderFilter(
		chatMessagesRuntimeForTest(t, nil),
		payload,
		[]map[string]any{
			{"openMessageId": "wanted", "senderOpenDingTalkId": "DAAAAAAAAAAAiE"},
			{"openMessageId": "other-family", "senderUserId": "other-user"},
		},
		&filter,
	)
	if len(filtered) != 1 || filter.scopeErr != nil || payload["complete"] != true || payload["count"] != 1 {
		t.Fatalf("filtered=%#v filter=%#v payload=%#v", filtered, filter, payload)
	}
}

func TestCrossPlatformCoverageChatMessagesSenderFilterFailureEdges(t *testing.T) {
	t.Run("unrelated unique directory candidate never replaces the supplied user id", func(t *testing.T) {
		fake := &platformCoverageCaller{
			contactSearchResult: `{"result":[{"userId":"other-user","name":"其他用户"}],"hasMore":false}`,
		}
		helpers.InitDeps(fake)
		filter := resolveOptionalChatMessagesSenderFilter(chatMessagesRuntimeForTest(t, map[string]string{
			"sender": "fixture-user-id",
		}))
		if !filter.applied || !filter.stableIDs["fixture-user-id"] || filter.stableIDs["other-user"] {
			t.Fatalf("filter=%#v", filter)
		}
	})

	t.Run("direct user ids continue when directory is unavailable", func(t *testing.T) {
		fake := &platformCoverageCaller{failTool: "contact/search_contact_by_key_word"}
		helpers.InitDeps(fake)
		filter := resolveOptionalChatMessagesSenderFilter(chatMessagesRuntimeForTest(t, map[string]string{
			"sender": testCurrentDOpenID + ",fixture-name,ignored-name",
		}))
		if !filter.applied || filter.resolutionErr != nil || len(fake.calls) != 2 ||
			!filter.stableIDs["fixture-name"] || !filter.stableIDs["ignored-name"] {
			t.Fatalf("filter=%#v calls=%#v", filter, fake.calls)
		}
	})

	t.Run("unverified mixed sender becomes verified by an exact sender id match", func(t *testing.T) {
		payload := map[string]any{"complete": true, "failures": []map[string]any{}}
		resolution := targetresolver.UserResolution{
			Status:     targetresolver.StatusResolved,
			EntityType: "user",
			Query:      "stable-user-id",
			MatchType:  "unverified_user_id",
			Selected:   targetresolver.User{UserID: "stable-user-id"},
		}
		filter := chatMessagesSenderFilter{
			requested:   true,
			applied:     true,
			inputs:      []string{"stable-user-id"},
			inputMode:   "sender",
			stableIDs:   map[string]bool{"stable-user-id": true},
			resolutions: []targetresolver.UserResolution{resolution},
		}
		filtered := applyOptionalChatMessagesSenderFilter(
			chatMessagesRuntimeForTest(t, nil), payload,
			[]map[string]any{
				{"openMessageId": "wanted", "senderUserId": "stable-user-id"},
				{"openMessageId": "other", "senderUserId": "other-user"},
			}, &filter,
		)
		if len(filtered) != 1 || payload["complete"] != true || len(payload["failures"].([]map[string]any)) != 0 {
			t.Fatalf("filtered=%#v payload=%#v", filtered, payload)
		}
		identity := payload["identityResult"].(map[string]any)
		if identity["status"] != "evaluated" || identity["negativeConclusionAllowed"] != true {
			t.Fatalf("identityResult=%#v", identity)
		}
	})

	t.Run("unverified mixed sender without a match blocks complete negative conclusion", func(t *testing.T) {
		payload := map[string]any{"complete": true, "failures": []map[string]any{}}
		resolution := targetresolver.UserResolution{
			Status:     targetresolver.StatusResolved,
			EntityType: "user",
			Query:      "possibly-a-name",
			MatchType:  "unverified_user_id",
			Selected:   targetresolver.User{UserID: "possibly-a-name"},
		}
		filter := chatMessagesSenderFilter{
			requested:   true,
			applied:     true,
			inputs:      []string{"possibly-a-name"},
			inputMode:   "sender",
			stableIDs:   map[string]bool{"possibly-a-name": true},
			resolutions: []targetresolver.UserResolution{resolution},
		}
		filtered := applyOptionalChatMessagesSenderFilter(
			chatMessagesRuntimeForTest(t, nil), payload,
			[]map[string]any{{"openMessageId": "other", "senderUserId": "other-user"}}, &filter,
		)
		if len(filtered) != 0 || payload["complete"] != false || payload["failedCount"] != 1 {
			t.Fatalf("filtered=%#v payload=%#v", filtered, payload)
		}
		identity := payload["identityResult"].(map[string]any)
		if identity["status"] != "identity_unverified" || identity["negativeConclusionAllowed"] != false {
			t.Fatalf("identityResult=%#v", identity)
		}
	})

	t.Run("dedupes blanks and repeated values", func(t *testing.T) {
		got := uniqueChatMessageTargets([]string{"", "  ", "one", "one", " two "})
		if want := []string{"one", "two"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("targets=%#v want=%#v", got, want)
		}
	})

	t.Run("synthesizes failure when none was supplied", func(t *testing.T) {
		payload := map[string]any{"failures": []map[string]any{{"stage": "existing"}}}
		filter := chatMessagesSenderFilter{requested: true, inputs: []string{"fixture"}, inputMode: "sender"}
		if got := applyOptionalChatMessagesSenderFilter(
			chatMessagesRuntimeForTest(t, nil), payload, []map[string]any{{"openMessageId": "hidden"}}, &filter,
		); got != nil {
			t.Fatalf("filtered=%#v", got)
		}
		if _, exists := payload["stopReason"]; exists {
			t.Fatalf("existing failure should retain stop reason ownership: %#v", payload)
		}
	})

	t.Run("rejects only missing identities", func(t *testing.T) {
		payload := map[string]any{"complete": true, "failures": []map[string]any{}}
		filter := chatMessagesSenderFilter{
			requested: true,
			applied:   true,
			inputs:    []string{"fixture"},
			inputMode: "sender",
			stableIDs: map[string]bool{"wanted-user": true},
		}
		filtered := applyOptionalChatMessagesSenderFilter(
			chatMessagesRuntimeForTest(t, nil),
			payload,
			[]map[string]any{
				{"content": "missing sender and id"},
				{"openMessageId": "wanted", "senderUserId": "wanted-user"},
				{"openMessageId": "open-family", "senderOpenDingTalkId": testCurrentDOpenID},
			},
			&filter,
		)
		if len(filtered) != 1 || filter.scopeErr == nil {
			t.Fatalf("filtered=%#v filter=%#v payload=%#v", filtered, filter, payload)
		}
	})

	t.Run("ignores user identity when only open ids were resolved", func(t *testing.T) {
		payload := map[string]any{"complete": true, "failures": []map[string]any{}}
		filter := chatMessagesSenderFilter{
			requested: true,
			applied:   true,
			inputs:    []string{testCurrentDOpenID},
			inputMode: "sender",
			stableIDs: map[string]bool{testCurrentDOpenID: true},
		}
		applyOptionalChatMessagesSenderFilter(
			chatMessagesRuntimeForTest(t, nil), payload,
			[]map[string]any{{"senderUserId": "other-user"}}, &filter,
		)
		if filter.scopeErr != nil || payload["complete"] != true {
			t.Fatalf("filter=%#v", filter)
		}
	})

	t.Run("ignores open identity when only user ids were resolved", func(t *testing.T) {
		payload := map[string]any{"complete": true, "failures": []map[string]any{}}
		filter := chatMessagesSenderFilter{
			requested: true,
			applied:   true,
			inputs:    []string{"wanted-user"},
			inputMode: "sender",
			stableIDs: map[string]bool{"wanted-user": true},
		}
		applyOptionalChatMessagesSenderFilter(
			chatMessagesRuntimeForTest(t, nil), payload,
			[]map[string]any{{"senderOpenDingTalkId": testCurrentDOpenID}}, &filter,
		)
		if filter.scopeErr != nil || payload["complete"] != true {
			t.Fatalf("filter=%#v", filter)
		}
	})

	if _, err := resolveChatMessagesRequest(chatMessagesRuntimeForTest(t, map[string]string{
		"open-dingtalk-id": "not-an-open-id",
	})); err == nil {
		t.Fatal("invalid explicit open ID unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageChatMessagesSenderFailureOutputEdges(t *testing.T) {
	t.Run("ambiguous direct sender prioritizes output error", func(t *testing.T) {
		caller := &platformCoverageCaller{contactSearchResult: `{"result":[
			{"userId":"fixture-user-1","name":"测试同名发送者"},
			{"userId":"fixture-user-2","name":"测试同名发送者"}
		],"hasMore":false}`}
		helpers.InitDeps(caller)
		root := newPlatformCoverageRoot()
		root.SetOut(chatMessagesFailWriter{})
		root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--sender", "测试同名发送者"})
		if err := root.Execute(); err == nil || err.Error() != "fixture output failure" {
			t.Fatalf("error=%v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		sender string
	}{
		{name: "resolution", sender: "fixture-name"},
		{name: "scope", sender: testCurrentDOpenID},
	} {
		t.Run(tc.name+" returns domain error", func(t *testing.T) {
			caller := &chatMessagesPagingCaller{responses: []string{
				`{"result":{"hasMore":false,"messages":[{"openMessageId":"m-without-sender"}]}}`,
			}}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--sender", tc.sender})
			if err := root.Execute(); err == nil {
				t.Fatal("sender failure unexpectedly succeeded")
			}
		})

		t.Run(tc.name+" prioritizes output error", func(t *testing.T) {
			caller := &chatMessagesPagingCaller{responses: []string{
				`{"result":{"hasMore":false,"messages":[{"openMessageId":"m-without-sender"}]}}`,
			}}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			root.SetOut(chatMessagesFailWriter{})
			root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--sender", tc.sender})
			if err := root.Execute(); err == nil || err.Error() != "fixture output failure" {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessagesAdditionalCollectionEdges(t *testing.T) {
	runtimeWith := func(t *testing.T, caller *chatMessagesPagingCaller, values map[string]string) *shortcut.RuntimeContext {
		t.Helper()
		helpers.InitDeps(caller)
		return chatMessagesRuntimeForTest(t, values)
	}
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, dingTalkMessageLocation)
	configuredRange := chatMessageTimeRange{configured: true, start: &start, order: "desc"}

	t.Run("single-page range failures and terminal", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":true,"nextCursor":1234,"messages":[{"openMessageId":"bad","createTime":"invalid"}]}}`,
		}}
		payload, _, err := collectOneChatMessagesPage(
			runtimeWith(t, caller, nil),
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older", timeRange: configuredRange},
		)
		if err != nil || payload["stopReason"] != "time_filter_error" || payload["queryRange"] == nil {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}

		caller = &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":true,"nextCursor":1234,"messages":[{"openMessageId":"old","createTime":"2026-01-01 00:00:00"}]}}`,
		}}
		payload, _, err = collectOneChatMessagesPage(
			runtimeWith(t, caller, nil),
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older", timeRange: configuredRange},
		)
		if err != nil || payload["complete"] != true || payload["stopReason"] != "range_start" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("default page size and time-filter failure", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":false,"messages":[]}}`,
		}}
		payload, _, err := collectAllChatMessages(
			runtimeWith(t, caller, nil),
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older"},
		)
		if err != nil || payload["complete"] != true || caller.args[0]["limit"] != chatMessagesAllPageSize {
			t.Fatalf("payload=%#v calls=%#v err=%v", payload, caller.args, err)
		}

		caller = &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":false,"messages":[{"openMessageId":"bad","createTime":"invalid"}]}}`,
		}}
		payload, _, err = collectAllChatMessages(
			runtimeWith(t, caller, nil),
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older", timeRange: configuredRange},
		)
		if err == nil || payload["stopReason"] != "time_filter_error" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("terminal result limit and unsafe continuation", func(t *testing.T) {
		for _, flag := range []string{"max-items", "max-results"} {
			caller := &chatMessagesPagingCaller{responses: []string{
				`{"result":{"hasMore":true,"messages":[{"openMessageId":"m2","createTime":"2026-01-03 00:00:00"},{"openMessageId":"m1","createTime":"2026-01-02 00:00:00"},{"openMessageId":"old","createTime":"2026-01-01 00:00:00"}]}}`,
			}}
			payload, _, err := collectAllChatMessages(
				runtimeWith(t, caller, map[string]string{flag: "1"}),
				chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older", timeRange: configuredRange},
			)
			if err != nil || payload["truncated"] != true || payload["truncatedByResultLimit"] != true || payload["stopReason"] != "result_limit" {
				t.Fatalf("%s payload=%#v err=%v", flag, payload, err)
			}
		}

		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":true,"messages":[{"openMessageId":"m1","createTime":"2026-01-03 00:00:00"}]}}`,
		}}
		payload, _, err := collectAllChatMessages(
			runtimeWith(t, caller, map[string]string{"max-results": "1"}),
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older"},
		)
		if err == nil || payload["stopReason"] != "pagination_error" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("stalled cursor", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":true,"nextCursor":1234,"messages":[{"openMessageId":"m2"}]}}`,
			`{"result":{"hasMore":true,"nextCursor":1234,"messages":[{"openMessageId":"m1"}]}}`,
		}}
		payload, _, err := collectAllChatMessages(
			runtimeWith(t, caller, nil),
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older"},
		)
		if err == nil || payload["stopReason"] != "pagination_error" || len(caller.args) != 2 {
			t.Fatalf("payload=%#v calls=%#v err=%v", payload, caller.args, err)
		}
	})

	t.Run("canceled delay", func(t *testing.T) {
		caller := &chatMessagesPagingCaller{responses: []string{
			`{"result":{"hasMore":true,"nextCursor":1234,"messages":[{"openMessageId":"m1"}]}}`,
		}}
		rt := runtimeWith(t, caller, map[string]string{"page-delay": "1"})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rt.Command().SetContext(ctx)
		payload, _, err := collectAllChatMessages(
			rt,
			chatMessagesRequest{tool: "list_conversation_message_v2", params: map[string]any{}, direction: "older"},
		)
		if err == nil || payload["stopReason"] != "delay_interrupted" || payload["failedCount"] != 1 {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("first failure ledger output error", func(t *testing.T) {
		helpers.InitDeps(&chatMessagesPagingCaller{failAt: 1})
		root := newPlatformCoverageRoot()
		root.SetOut(chatMessagesFailWriter{})
		root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--page-all"})
		if err := root.Execute(); err == nil || err.Error() != "fixture output failure" {
			t.Fatalf("error=%v", err)
		}
	})
}
