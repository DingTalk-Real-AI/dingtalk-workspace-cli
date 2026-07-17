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
	"context"
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type platformCoverageCall struct {
	product string
	tool    string
	args    map[string]any
}

type platformCoverageCaller struct {
	calls []platformCoverageCall
}

func (f *platformCoverageCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, platformCoverageCall{product: product, tool: tool, args: args})
	text := `{"result":[]}`
	switch product + "/" + tool {
	case "contact/search_contact_by_key_word":
		text = `{"result":[{"userId":"u1","name":"张三","openDingTalkId":"open1"}]}`
	case "im/search_groups":
		text = `{"result":[{"openConversationId":"cid-1","title":"项目冲刺"}]}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (f *platformCoverageCaller) Format() string { return "json" }
func (f *platformCoverageCaller) DryRun() bool   { return false }
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
			if got := send.args["clawType"]; got != edition.ClawType() {
				t.Fatalf("clawType = %#v, want %q", got, edition.ClawType())
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

func TestCrossPlatformCoverageCompatibilityAliases(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		wantTool string
		wantArgs map[string]any
	}{
		{
			name:     "chat messages id and size",
			argv:     []string{"chat", "+chat-messages", "--id", "cid-1", "--size", "9", "--yes"},
			wantTool: "list_conversation_message_v2",
			wantArgs: map[string]any{"openCid": "cid-1", "limit": 9},
		},
		{
			name:     "search message id and keyword",
			argv:     []string{"chat", "+search-msg", "--id", "cid-1", "--keyword", "树莓派", "--yes"},
			wantTool: "search_messages_by_keyword",
			wantArgs: map[string]any{"openConversationId": "cid-1", "keyword": "树莓派"},
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
			if call.product != "chat" || call.tool != tc.wantTool {
				t.Fatalf("call = %s/%s, want chat/%s", call.product, call.tool, tc.wantTool)
			}
			for key, want := range tc.wantArgs {
				if got := call.args[key]; got != want {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
		})
	}
}
