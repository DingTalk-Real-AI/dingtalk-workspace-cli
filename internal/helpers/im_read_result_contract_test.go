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

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type imReadResultCall struct {
	productID string
	toolName  string
}

type imReadResultCaller struct {
	responses map[string]string
	results   map[string]*edition.ToolResult
	errors    map[string]error
	calls     []imReadResultCall
	args      map[string]any
	dryRun    bool
}

func (c *imReadResultCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, imReadResultCall{productID: productID, toolName: toolName})
	c.args = map[string]any{}
	for k, v := range args {
		c.args[k] = v
	}
	if err := c.errors[toolName]; err != nil {
		return nil, err
	}
	if result, ok := c.results[toolName]; ok {
		return result, nil
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.responses[toolName]}}}, nil
}

func TestCrossPlatformCoverageChatMessageReadsPreserveToolOperationOnCallerError(t *testing.T) {
	caller := &imReadResultCaller{errors: map[string]error{
		"list_conversation_message_v2": errors.New("dial tcp: connection refused"),
	}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00")
	if err == nil {
		t.Fatalf("chat message list unexpectedly succeeded with output %q", got)
	}
	if got != "" {
		t.Fatalf("chat message list output = %q, want no success payload", got)
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Operation != "chat/list_conversation_message_v2" {
		t.Fatalf("chat message list error = %#v", err)
	}

	existing := &CLIError{
		Code:      CodeNetworkTimeout,
		Message:   "request already classified",
		Operation: "chat/existing-operation",
	}
	caller = &imReadResultCaller{errors: map[string]error{
		"list_conversation_message_v2": existing,
	}}
	got, err = executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00")
	if err != existing {
		t.Fatalf("chat message list error = %#v, want original %#v", err, existing)
	}
	if got != "" {
		t.Fatalf("chat message list output = %q, want no success payload", got)
	}
}

func (*imReadResultCaller) Format() string { return "json" }
func (c *imReadResultCaller) DryRun() bool { return c.dryRun }
func (*imReadResultCaller) Fields() string { return "" }
func (*imReadResultCaller) JQ() string     { return "" }

func executeIMReadCommand(t *testing.T, caller *imReadResultCaller, processArgs []string, build func() *cobra.Command, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard
	os.Args = processArgs

	root := build()
	installExampleGlobalFlags(root)
	root.SetOut(&stdout)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), err
}

func requireSameJSON(t *testing.T, got, want string) {
	t.Helper()
	var gotValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("command output = %#v, want %#v", gotValue, wantValue)
	}
}

func TestCrossPlatformCoverageChatMessageListProjectsStableFieldsAndPreservesLegacy(t *testing.T) {
	payload := `{
		"result": {
			"messages": [
				{"openMessageId":"msg-1","content":{"text":"回复正文"},"createTime":101,"msgType":"reply","quotedMessage":{"msgType":"merged_forward","content":{"items":[{"text":"原消息"}]}}},
				{"openMessageId":"msg-2","content":{"text":"图片回复"},"createTime":102,"msgType":"reply","quotedMessage":{"msgType":"image","content":{"mediaId":"media-1"}}}
			],
			"hasMore": false
		}
	}`
	caller := &imReadResultCaller{responses: map[string]string{"list_conversation_message_v2": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00", "--limit", "50")
	if err != nil {
		t.Fatalf("chat message list returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0] != (imReadResultCall{productID: "chat", toolName: "list_conversation_message_v2"}) {
		t.Fatalf("calls = %#v, want chat/list_conversation_message_v2", caller.calls)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	if _, exists := result["contractVersion"]; exists {
		t.Fatalf("typed list added an out-of-scope contract envelope: %#v", result)
	}
	messages, ok := result["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", result["messages"])
	}
	first, ok := messages[0].(map[string]any)
	if !ok || first["messageId"] != "msg-1" || first["openMessageId"] != "msg-1" || first["text"] != "回复正文" {
		t.Fatalf("first projected message = %#v", messages[0])
	}
	if _, ok := first["content"].(map[string]any); !ok {
		t.Fatalf("legacy content not preserved: %#v", first)
	}
	quoted, ok := first["quotedMessage"].(map[string]any)
	if !ok || quoted["msgType"] != "merged_forward" {
		t.Fatalf("merged-forward quote = %#v", first["quotedMessage"])
	}
	quotedContent, ok := quoted["content"].(map[string]any)
	items, itemsOK := quotedContent["items"].([]any)
	if !ok || !itemsOK || len(items) != 1 {
		t.Fatalf("merged-forward quote content = %#v", quoted["content"])
	}
	second, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("second projected message = %#v", messages[1])
	}
	quoted, ok = second["quotedMessage"].(map[string]any)
	if !ok || quoted["msgType"] != "image" {
		t.Fatalf("image quote = %#v", second["quotedMessage"])
	}
	quotedContent, ok = quoted["content"].(map[string]any)
	if !ok || quotedContent["mediaId"] != "media-1" {
		t.Fatalf("image quote content = %#v", quoted["content"])
	}
	legacy, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("legacy result envelope missing: %#v", result["result"])
	}
	legacyMessages, ok := legacy["messages"].([]any)
	if !ok || len(legacyMessages) != 2 {
		t.Fatalf("legacy result.messages = %#v", legacy["messages"])
	}
}

func TestCrossPlatformCoverageChatMessageListDefaultsTimeAndOlderDirection(t *testing.T) {
	payload := `{"result":{"messages":[],"hasMore":false}}`
	caller := &imReadResultCaller{responses: map[string]string{"list_conversation_message_v2": payload}}
	loc := shanghaiLocation()

	before := time.Now().In(loc)
	_, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--limit", "50")
	after := time.Now().In(loc)
	if err != nil {
		t.Fatalf("chat message list returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0] != (imReadResultCall{productID: "chat", toolName: "list_conversation_message_v2"}) {
		t.Fatalf("calls = %#v, want chat/list_conversation_message_v2", caller.calls)
	}
	if got := caller.args["forward"]; got != false {
		t.Fatalf("forward = %#v, want false when --time is omitted", got)
	}
	timeRaw, ok := caller.args["time"].(string)
	if !ok || strings.TrimSpace(timeRaw) == "" {
		t.Fatalf("time arg = %#v, want generated current time", caller.args["time"])
	}
	gotTime, err := time.ParseInLocation("2006-01-02 15:04:05", timeRaw, loc)
	if err != nil {
		t.Fatalf("parse generated time %q: %v", timeRaw, err)
	}
	if gotTime.Before(before.Add(-time.Second)) || gotTime.After(after.Add(time.Second)) {
		t.Fatalf("time = %s, want between %s and %s", gotTime, before, after)
	}
}

func TestCrossPlatformCoverageChatMessageSearchProjectsStableFieldsAndPreservesLegacy(t *testing.T) {
	payload := `{
		"result": {
			"conversationMessagesList": [{
				"openConversationId": "cid-1",
				"title": "项目群",
				"messages": [{"openMessageId":"msg-search-1","messageId":"legacy-conflict","content":{"richText":"发布计划"},"createTime":201}]
			}],
			"nextCursor": "cursor-2"
		}
	}`
	caller := &imReadResultCaller{responses: map[string]string{"search_messages_by_keyword": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "search", "--query", "发布计划",
		"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-10T00:00:00+08:00")
	if err != nil {
		t.Fatalf("chat message search returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0] != (imReadResultCall{productID: "chat", toolName: "search_messages_by_keyword"}) {
		t.Fatalf("calls = %#v, want chat/search_messages_by_keyword", caller.calls)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	if _, exists := result["contractVersion"]; exists {
		t.Fatalf("typed search added an out-of-scope contract envelope: %#v", result)
	}
	messages, ok := result["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", result["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok || message["messageId"] != "msg-search-1" || message["openMessageId"] != "msg-search-1" ||
		message["conversationId"] != "cid-1" || message["text"] != "发布计划" {
		t.Fatalf("projected search message = %#v", messages[0])
	}
	if _, ok := message["content"].(map[string]any); !ok {
		t.Fatalf("legacy content not preserved: %#v", message)
	}
	legacy, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("legacy result envelope missing: %#v", result["result"])
	}
	legacyGroups, ok := legacy["conversationMessagesList"].([]any)
	if !ok || len(legacyGroups) != 1 {
		t.Fatalf("legacy result.conversationMessagesList = %#v", legacy["conversationMessagesList"])
	}
	if legacy["nextCursor"] != "cursor-2" {
		t.Fatalf("legacy result.nextCursor = %#v", legacy["nextCursor"])
	}
}

func TestCrossPlatformCoverageChatMessageScopedSearchProjectsStableFields(t *testing.T) {
	payload := `{
		"result": {
			"conversationMessagesList": [{
				"openConversationId": "cid-1",
				"messages": [{"openMessageId":"msg-scoped-1","content":{"text":"范围内消息"}}]
			}],
			"hasMore": false
		}
	}`
	caller := &imReadResultCaller{responses: map[string]string{
		"get_conversation_info":      `{}`,
		"search_messages_by_keyword": payload,
	}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "search", "--query", "范围内消息", "--group", "cid-1",
		"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-10T00:00:00+08:00")
	if err != nil {
		t.Fatalf("scoped chat message search returned error: %v", err)
	}
	wantCalls := []imReadResultCall{
		{productID: "chat", toolName: "get_conversation_info"},
		{productID: "chat", toolName: "search_messages_by_keyword"},
	}
	if !reflect.DeepEqual(caller.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", caller.calls, wantCalls)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	messages, ok := result["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", result["messages"])
	}
	message, _ := messages[0].(map[string]any)
	if message["messageId"] != "msg-scoped-1" || message["openMessageId"] != "msg-scoped-1" ||
		message["conversationId"] != "cid-1" || message["text"] != "范围内消息" {
		t.Fatalf("projected scoped message = %#v", message)
	}
	scope, _ := result["scope"].(map[string]any)
	if scope["targetsValidated"] != true || scope["resultsWithinScope"] != true || scope["sourceComplete"] != true {
		t.Fatalf("scope = %#v", scope)
	}
	legacy, _ := result["result"].(map[string]any)
	if _, ok := legacy["conversationMessagesList"].([]any); !ok {
		t.Fatalf("legacy result envelope missing: %#v", legacy)
	}
}

func TestCrossPlatformCoverageChatMessageListPreservesNonJSONResponse(t *testing.T) {
	const payload = "upstream temporarily returned plain text"
	caller := &imReadResultCaller{responses: map[string]string{"list_conversation_message_v2": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00")
	if err != nil {
		t.Fatalf("chat message list returned error: %v", err)
	}
	if got != payload+"\n" {
		t.Fatalf("command output = %q, want raw payload", got)
	}
}

func TestCrossPlatformCoverageChatMessageReadsRejectEmptyToolResults(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		caller   *imReadResultCaller
		args     []string
	}{
		{
			name:     "list empty text",
			toolName: "list_conversation_message_v2",
			caller: &imReadResultCaller{responses: map[string]string{
				"list_conversation_message_v2": "  \n ",
			}},
			args: []string{"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00"},
		},
		{
			name:     "search without text content",
			toolName: "search_messages_by_keyword",
			caller: &imReadResultCaller{results: map[string]*edition.ToolResult{
				"search_messages_by_keyword": {Content: []edition.ContentBlock{{Type: "image"}}},
			}},
			args: []string{"message", "search", "--query", "发布计划",
				"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-10T00:00:00+08:00"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := executeIMReadCommand(t, tc.caller, []string{"dws", "chat"}, newChatCommand, tc.args...)
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded with output %q", tc.toolName, got)
			}
			if got != "" {
				t.Fatalf("%s output = %q, want no success payload", tc.toolName, got)
			}
			var apiErr *apperrors.Error
			if !errors.As(err, &apiErr) || apiErr.Reason != "empty_tool_response" ||
				apiErr.Operation != "chat/"+tc.toolName || !apiErr.RetryableSet || !apiErr.Retryable {
				t.Fatalf("%s error = %#v", tc.toolName, err)
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessageListPreservesTopLevelMessageFields(t *testing.T) {
	payload := `{
		"messages": [{
			"openMessageId": "msg-top-1",
			"content": {"text": "顶层正文"},
			"msgType": "text",
			"sender": {"name": "张三", "department": "研发"},
			"senderName": "张三",
			"quotedMessage": {
				"msgType": "image",
				"content": {"mediaId": "media-1", "caption": "原图说明"},
				"extension": {"source": "legacy-quote"}
			},
			"forwarded": [{
				"openMessageId": "forward-1",
				"content": {"text": "转发正文"},
				"extension": {"source": "legacy-forward"}
			}],
			"reactions": [{
				"emoji": "like",
				"count": 2,
				"extension": {"source": "legacy-reaction"}
			}],
			"extensionField": {"source": "legacy"}
		}, {
			"messageId": "msg-top-2",
			"text": "稳定正文"
		}],
		"count": 99,
		"partial": true,
		"failedCount": 1,
		"failures": [{"stage":"legacy","error":"legacy failure"}],
		"hasMore": false
	}`
	caller := &imReadResultCaller{responses: map[string]string{"list_conversation_message_v2": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00")
	if err != nil {
		t.Fatalf("chat message list returned error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	messages, ok := result["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", result["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("message = %#v", messages[0])
	}
	if message["messageId"] != "msg-top-1" || message["text"] != "顶层正文" {
		t.Fatalf("stable fields = %#v", message)
	}
	if message["msgType"] != "text" || message["senderName"] != "张三" {
		t.Fatalf("legacy message fields = %#v", message)
	}
	if !reflect.DeepEqual(message["sender"], map[string]any{"name": "张三", "department": "研发"}) {
		t.Fatalf("legacy sender = %#v", message["sender"])
	}
	wantQuoted := map[string]any{
		"msgType":   "image",
		"content":   map[string]any{"mediaId": "media-1", "caption": "原图说明"},
		"extension": map[string]any{"source": "legacy-quote"},
	}
	if !reflect.DeepEqual(message["quotedMessage"], wantQuoted) {
		t.Fatalf("legacy quotedMessage = %#v", message["quotedMessage"])
	}
	wantForwarded := []any{map[string]any{
		"openMessageId": "forward-1",
		"content":       map[string]any{"text": "转发正文"},
		"extension":     map[string]any{"source": "legacy-forward"},
	}}
	if !reflect.DeepEqual(message["forwarded"], wantForwarded) {
		t.Fatalf("legacy forwarded = %#v", message["forwarded"])
	}
	wantReactions := []any{map[string]any{
		"emoji":     "like",
		"count":     float64(2),
		"extension": map[string]any{"source": "legacy-reaction"},
	}}
	if !reflect.DeepEqual(message["reactions"], wantReactions) {
		t.Fatalf("legacy reactions = %#v", message["reactions"])
	}
	if extension, ok := message["extensionField"].(map[string]any); !ok || extension["source"] != "legacy" {
		t.Fatalf("extensionField = %#v", message["extensionField"])
	}
	stableOnly, ok := messages[1].(map[string]any)
	if !ok || stableOnly["openMessageId"] != "msg-top-2" || stableOnly["content"] != "稳定正文" {
		t.Fatalf("legacy aliases from stable fields = %#v", messages[1])
	}
	if result["count"] != float64(99) || result["partial"] != true || result["failedCount"] != float64(1) {
		t.Fatalf("legacy top-level envelope fields = %#v", result)
	}
	if _, exists := result["contractVersion"]; exists {
		t.Fatalf("typed list added an out-of-scope contract envelope: %#v", result)
	}
	failures, ok := result["failures"].([]any)
	if !ok || len(failures) != 1 || failures[0].(map[string]any)["stage"] != "legacy" {
		t.Fatalf("legacy failures = %#v", result["failures"])
	}
}

func TestCrossPlatformCoverageChatMessageListDryRunKeepsPreviewPath(t *testing.T) {
	caller := &imReadResultCaller{dryRun: true}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00")
	if err != nil {
		t.Fatalf("chat message list dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run calls = %#v, want none", caller.calls)
	}
	var preview map[string]any
	if err := json.Unmarshal([]byte(got), &preview); err != nil {
		t.Fatalf("decode dry-run output: %v\noutput: %s", err, got)
	}
	if preview["dry_run"] != true || preview["executed"] != false || preview["tool"] != "list_conversation_message_v2" {
		t.Fatalf("dry-run preview = %#v", preview)
	}
}

func TestCrossPlatformCoverageChatMessageListRawPreservesLargeIntegers(t *testing.T) {
	const payload = `{"messages":[{"openMessageId":"msg-1","content":{"text":"正文"},"sequence":9007199254740993}],"hasMore":false}`
	caller := &imReadResultCaller{responses: map[string]string{"list_conversation_message_v2": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "list", "--group", "cid-1", "--time", "2026-07-14 00:00:00", "--format", "raw")
	if err != nil {
		t.Fatalf("chat message list returned error: %v", err)
	}
	if !bytes.Contains([]byte(got), []byte(`"sequence":9007199254740993`)) {
		t.Fatalf("raw output changed large integer: %s", got)
	}
}

func TestDingMessageListPreservesContent(t *testing.T) {
	payload := `{"result":{"dingMessages":[{"openDingId":"ding-1","status":"READ","content":"升级提醒"}]}}`
	caller := &imReadResultCaller{responses: map[string]string{"list_ding_messages": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "ding"}, newDingCommand,
		"message", "list", "--type", "ALL")
	if err != nil {
		t.Fatalf("ding message list returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0] != (imReadResultCall{productID: "im", toolName: "list_ding_messages"}) {
		t.Fatalf("calls = %#v, want im/list_ding_messages", caller.calls)
	}
	requireSameJSON(t, got, payload)
}
