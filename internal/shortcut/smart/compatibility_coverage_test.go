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
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type platformCoverageCall struct {
	product string
	tool    string
	args    map[string]any
}

type platformCoverageCaller struct {
	calls               []platformCoverageCall
	dry                 bool
	contactSearchResult string
}

func (f *platformCoverageCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, platformCoverageCall{product: product, tool: tool, args: args})
	text := `{"result":[]}`
	switch product + "/" + tool {
	case "contact/search_contact_by_key_word":
		text = f.contactSearchResult
		if text == "" {
			text = `{"result":[{"userId":"u1","name":"张三","openDingTalkId":"open1"}]}`
		}
	case "contact/get_current_user_profile":
		text = `{"result":{"userId":"u1"}}`
	case "im/search_groups":
		text = `{"result":[{"openConversationId":"cid-1","title":"项目冲刺"}]}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (f *platformCoverageCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return f.CallTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageExternalContactAmbiguity(t *testing.T) {
	fake := &platformCoverageCaller{
		contactSearchResult: `{"result":[
			{"userId":"u1","name":"张三","openDingTalkId":"open1"},
			{"openDingtalkId":"open-external","nick":"外部张三"}
		]}`,
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+dm", "--to", "张三", "--text", "你好", "--yes"})
	err := root.Execute()
	if err == nil {
		t.Fatal("ambiguous internal and external contacts unexpectedly resolved")
	}
	for _, want := range []string{"张三(u1)", "外部张三(open-external)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ambiguity error %q does not contain %q", err, want)
		}
	}
}

func (f *platformCoverageCaller) Format() string { return "json" }
func (f *platformCoverageCaller) DryRun() bool   { return f.dry }
func (f *platformCoverageCaller) Fields() string { return "" }
func (f *platformCoverageCaller) JQ() string     { return "" }

func newPlatformCoverageRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(shortcut.Commands()...)
	return root
}

func TestCrossPlatformCoverageAIMessageTag(t *testing.T) {
	t.Setenv(agentproduct.EnvName, "qwenwork")

	tests := []struct {
		name string
		argv []string
	}{
		{name: "dm", argv: []string{"chat", "+dm", "--to", "张三", "--text", "你好", "--yes"}},
		{name: "send to group", argv: []string{"chat", "+send-to-group", "--group", "项目冲刺", "--text", "你好", "--yes"}},
		{name: "broadcast", argv: []string{"chat", "+broadcast", "--to", "张三", "--text", "你好", "--yes"}},
		{name: "share doc", argv: []string{"doc", "+share-doc", "--to", "张三", "--url", "https://example.com/doc", "--yes"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.argv)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) == 0 {
				t.Fatal("shortcut made no MCP calls")
			}
			send := fake.calls[len(fake.calls)-1]
			if send.product != "chat" || send.tool != "send_personal_message" {
				t.Fatalf("last call = %s/%s, want chat/send_personal_message", send.product, send.tool)
			}
			if got := send.args["clawType"]; got != "qwenwork" {
				t.Fatalf("clawType = %#v, want qwenwork", got)
			}
		})
	}

	t.Run("opt out", func(t *testing.T) {
		fake := &platformCoverageCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+dm", "--to", "张三", "--text", "你好", "--ai-tag=false", "--yes"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		send := fake.calls[len(fake.calls)-1]
		if _, ok := send.args["clawType"]; ok {
			t.Fatalf("clawType unexpectedly present with --ai-tag=false: %#v", send.args)
		}
	})
}

func TestCrossPlatformCoverageBroadcastDryRunPublishesExecutablePlan(t *testing.T) {
	fake := &platformCoverageCaller{dry: true}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+broadcast",
		"--to", "张三",
		"--text", "你好",
		"--dry-run",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 ||
		fake.calls[0].product != "contact" ||
		fake.calls[0].tool != "search_contact_by_key_word" {
		t.Fatalf("dry-run calls = %#v, want one read-only contact lookup", fake.calls)
	}

	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, output.String())
	}
	if payload["dry_run"] != true ||
		payload["executed"] != false ||
		payload["preview_kind"] != "plan" ||
		payload["tool"] != "send_personal_message" {
		t.Fatalf("dry-run payload = %#v", payload)
	}
	actions, _ := payload["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("dry-run actions = %#v", payload["actions"])
	}
	action, _ := actions[0].(map[string]any)
	arguments, _ := action["arguments"].(map[string]any)
	for _, key := range []string{"receiverOpenDingTalkId", "msgType", "content", "clawType"} {
		if _, ok := arguments[key]; !ok {
			t.Errorf("dry-run action arguments missing %q: %#v", key, arguments)
		}
	}
}

func TestCrossPlatformCoverageCompatibilityAliases(t *testing.T) {
	tests := []struct {
		name        string
		argv        []string
		wantProduct string
		wantTool    string
		wantArgs    map[string]any
		wantAbsent  []string
	}{
		{
			name:        "chat messages id and size",
			argv:        []string{"chat", "+chat-messages", "--id", "cid-1", "--size", "9", "--yes"},
			wantProduct: "chat",
			wantTool:    "list_conversation_message_v2",
			wantArgs:    map[string]any{"openconversation_id": "cid-1", "limit": 9},
			wantAbsent:  []string{"openCid", "cid"},
		},
		{
			name:        "search message id and keyword",
			argv:        []string{"chat", "+search-msg", "--id", "cid-1", "--keyword", "树莓派", "--no-enrich", "--yes"},
			wantProduct: "im",
			wantTool:    "search_messages",
			wantArgs:    map[string]any{"openConversationIds": []string{"cid-1"}, "keyword": "树莓派"},
		},
		{
			name:        "dm name alias",
			argv:        []string{"chat", "+dm", "--name", "张三", "--text", "你好", "--yes"},
			wantProduct: "chat",
			wantTool:    "send_personal_message",
			wantArgs:    map[string]any{"receiverOpenDingTalkId": "open1"},
		},
		{
			name:        "chat members list id alias",
			argv:        []string{"chat", "+chat-members-list", "--chat-id", "cid-1", "--member-types", "user"},
			wantProduct: "chat",
			wantTool:    "get_group_members",
			wantArgs:    map[string]any{"openconversation_id": "cid-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.argv)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			call := fake.calls[len(fake.calls)-1]
			if call.product != tc.wantProduct || call.tool != tc.wantTool {
				t.Fatalf("call = %s/%s, want %s/%s", call.product, call.tool, tc.wantProduct, tc.wantTool)
			}
			for key, want := range tc.wantArgs {
				if got := call.args[key]; !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
			for _, key := range tc.wantAbsent {
				if _, ok := call.args[key]; ok {
					t.Errorf("unexpected legacy argument %q in %#v", key, call.args)
				}
			}
		})
	}
}

func TestSendToGroupRejectsNonNameInputsBeforeMCP(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "conversation id used as group name",
			argv: []string{"chat", "+send-to-group", "--group", "cidayZx5r0T+UiMi6NrO1048A==", "--text", "你好", "--yes"},
			want: "只接受群名关键词",
		},
		{
			name: "at all belongs to messages send",
			argv: []string{"chat", "+send-to-group", "--group", "项目冲刺", "--text", "你好", "--at-all", "--yes"},
			want: "--at-all",
		},
		{
			name: "idempotency belongs to messages send",
			argv: []string{"chat", "+send-to-group", "--group", "项目冲刺", "--text", "你好", "--idempotency-key", "case-1", "--yes"},
			want: "--idempotency-key",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.argv)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute() error = %v, want %q", err, tc.want)
			}
			if len(fake.calls) != 0 {
				t.Fatalf("MCP calls = %#v, want none", fake.calls)
			}
		})
	}
}

func TestChatMessagesBeforeAliasAndPageAllGuard(t *testing.T) {
	t.Run("before maps to time and older", func(t *testing.T) {
		fake := &platformCoverageCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+chat-messages", "--group", "cid-1", "--before", "2026-07-30 16:51:39"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		call := fake.calls[len(fake.calls)-1]
		if call.args["time"] != "2026-07-30 16:51:39" || call.args["forward"] != false {
			t.Fatalf("args = %#v", call.args)
		}
	})

	t.Run("page all is blocked before MCP", func(t *testing.T) {
		fake := &platformCoverageCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+chat-messages", "--group", "cid-1", "--page-all"})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "暂不支持 --page-all") {
			t.Fatalf("Execute() error = %v", err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("MCP calls = %#v, want none", fake.calls)
		}
	})
}

func TestRelatedChatOptionsRejectBeforeMCP(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"group members rejects cid as name", []string{"chat", "+group-members", "--group", "cidayZx5r0T+UiMi6NrO1048A=="}, "--group"},
		{"members list rejects cid as name", []string{"chat", "+chat-members-list", "--group", "cidayZx5r0T+UiMi6NrO1048A=="}, "--group"},
		{"members list rejects name as cid", []string{"chat", "+chat-members-list", "--conversation-id", "测试群"}, "--conversation-id"},
		{"members list reports id alias", []string{"chat", "+chat-members-list", "--id", "测试群"}, "--id"},
		{"members list rejects unknown member type", []string{"chat", "+chat-members-list", "--group", "测试群", "--member-types", "admin"}, "--member-types"},
		{"unread chats rejects nonpositive count", []string{"chat", "+unread-chats", "--count", "0"}, "--count"},
		{"messages rejects time and before", []string{"chat", "+chat-messages", "--group", "cid", "--time", "2026-08-01", "--before", "2026-08-02"}, "--time"},
		{"messages rejects before newer", []string{"chat", "+chat-messages", "--group", "cid", "--before", "2026-08-02", "--direction", "newer"}, "--direction"},
		{"messages rejects group name as cid", []string{"chat", "+chat-messages", "--group", "测试群"}, "--group"},
		{"messages rejects nonpositive limit", []string{"chat", "+chat-messages", "--group", "cid", "--limit", "0"}, "--limit"},
		{"messages reports size alias", []string{"chat", "+chat-messages", "--group", "cid", "--size", "0"}, "--size"},
		{"messages rejects invalid time", []string{"chat", "+chat-messages", "--group", "cid", "--time", "yesterday-ish"}, "--time"},
		{"messages reports before alias", []string{"chat", "+chat-messages", "--group", "cid", "--before", "yesterday-ish"}, "--before"},
		{"messages reports conversation id alias", []string{"chat", "+chat-messages", "--conversation-id", "测试群"}, "--conversation-id"},
		{"thread replies rejects group name as cid", []string{"chat", "+thread-replies", "--group", "测试群", "--thread-id", "thread-1"}, "--group"},
		{"thread replies rejects nonpositive limit", []string{"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread-1", "--limit", "0"}, "--limit"},
		{"thread replies rejects invalid time", []string{"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread-1", "--time", "last-week"}, "--time"},
		{"at me rejects nonpositive days", []string{"chat", "+at-me", "--days", "0"}, "--days"},
		{"at me rejects oversized days", []string{"chat", "+at-me", "--days", "3651"}, "--days"},
		{"at me rejects nonpositive limit", []string{"chat", "+at-me", "--limit", "0"}, "--limit"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.argv)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute() error = %v, want %q", err, tc.want)
			}
			if len(fake.calls) != 0 {
				t.Fatalf("MCP calls = %#v, want none", fake.calls)
			}
		})
	}
}
