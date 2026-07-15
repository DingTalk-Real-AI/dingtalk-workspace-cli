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

func TestChatMessageListPreservesQuotedRichMessageContext(t *testing.T) {
	payload := `{
		"result": {
			"messages": [
				{"msgType":"reply","quotedMessage":{"msgType":"merged_forward","content":{"items":[{"text":"原消息"}]}}},
				{"msgType":"reply","quotedMessage":{"msgType":"image","content":{"mediaId":"media-1"}}}
			]
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
	requireSameJSON(t, got, payload)
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
