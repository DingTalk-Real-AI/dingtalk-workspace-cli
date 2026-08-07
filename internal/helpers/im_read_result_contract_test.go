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
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type imReadResultCall struct {
	productID string
	toolName  string
}

type imReadResultCaller struct {
	responses map[string]string
	calls     []imReadResultCall
}

func (c *imReadResultCaller) CallTool(_ context.Context, productID, toolName string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, imReadResultCall{productID: productID, toolName: toolName})
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.responses[toolName]}}}, nil
}

func (*imReadResultCaller) Format() string { return "json" }
func (*imReadResultCaller) DryRun() bool   { return false }
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
	if result["contractVersion"] != "im.message-list.v1" || result["count"] != float64(2) {
		t.Fatalf("message envelope = %#v", result)
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
	if !ok || quoted["messageType"] != "merged_forward" {
		t.Fatalf("merged-forward quote = %#v", first["quotedMessage"])
	}
	second, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("second projected message = %#v", messages[1])
	}
	quoted, ok = second["quotedMessage"].(map[string]any)
	if !ok || quoted["messageType"] != "image" {
		t.Fatalf("image quote = %#v", second["quotedMessage"])
	}
	resources, ok := quoted["resourceRefs"].([]any)
	if !ok || len(resources) != 1 {
		t.Fatalf("image quote resources = %#v", quoted["resourceRefs"])
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

func TestCrossPlatformCoverageChatMessageSearchProjectsStableFieldsAndPreservesLegacy(t *testing.T) {
	payload := `{
		"result": {
			"conversationMessagesList": [{
				"openConversationId": "cid-1",
				"title": "项目群",
				"messages": [{"openMessageId":"msg-search-1","messageId":"legacy-conflict","content":{"richText":"发布计划"},"createTime":201}]
			}],
			"hasMore": true,
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
	if result["contractVersion"] != "im.message-list.v1" || result["count"] != float64(1) || result["nextCursor"] != "cursor-2" {
		t.Fatalf("search envelope = %#v", result)
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
}

func TestCrossPlatformCoverageChatMessageSearchTreatsNumericZeroCursorAsComplete(t *testing.T) {
	payload := `{
		"result": {
			"messages": [{"openMessageId":"msg-search-1","content":{"text":"发布计划"}}],
			"hasMore": false,
			"nextCursor": 0
		}
	}`
	caller := &imReadResultCaller{responses: map[string]string{"search_messages_by_keyword": payload}}

	got, err := executeIMReadCommand(t, caller, []string{"dws", "chat"}, newChatCommand,
		"message", "search", "--query", "发布计划",
		"--start", "2026-07-01T00:00:00+08:00", "--end", "2026-07-10T00:00:00+08:00")
	if err != nil {
		t.Fatalf("chat message search returned error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("decode command output: %v\noutput: %s", err, got)
	}
	if result["hasMore"] != false || result["complete"] != true || result["paginationKnown"] != true {
		t.Fatalf("numeric zero cursor pagination = %#v", result)
	}
	if _, exists := result["nextCursor"]; exists {
		t.Fatalf("numeric zero cursor exposed as next page: %#v", result["nextCursor"])
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

func TestCrossPlatformCoverageChatMessageListPreservesTopLevelMessageFields(t *testing.T) {
	payload := `{
		"messages": [{
			"openMessageId": "msg-top-1",
			"content": {"text": "顶层正文"},
			"msgType": "text",
			"senderName": "张三",
			"extensionField": {"source": "legacy"}
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
	if !ok || len(messages) != 1 {
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
	if extension, ok := message["extensionField"].(map[string]any); !ok || extension["source"] != "legacy" {
		t.Fatalf("extensionField = %#v", message["extensionField"])
	}
	if result["count"] != float64(99) || result["partial"] != true || result["failedCount"] != float64(1) {
		t.Fatalf("legacy top-level envelope fields = %#v", result)
	}
	failures, ok := result["failures"].([]any)
	if !ok || len(failures) != 1 || failures[0].(map[string]any)["stage"] != "legacy" {
		t.Fatalf("legacy failures = %#v", result["failures"])
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
