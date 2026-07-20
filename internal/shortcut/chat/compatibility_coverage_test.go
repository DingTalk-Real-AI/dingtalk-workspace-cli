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

package chat

import (
	"context"
	"io"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type platformCoverageCaller struct {
	product string
	tool    string
	args    map[string]any
}

func (f *platformCoverageCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.product, f.tool, f.args = product, tool, args
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"result":[]}`}}}, nil
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
			name:        "chat search keyword and size",
			argv:        []string{"chat", "+chat-search", "--keyword", "树莓派", "--size", "5", "--yes"},
			wantProduct: "im",
			wantTool:    "search_groups",
			wantArgs:    map[string]any{"keyword": "树莓派", "limit": 5},
		},
		{
			name:        "bot find keyword",
			argv:        []string{"chat", "+bot-find", "--keyword", "日报", "--yes"},
			wantProduct: "bot",
			wantTool:    "search_bots",
			wantArgs:    map[string]any{"keyword": "日报"},
		},
		{
			name: "conversation messages conversation id and size",
			argv: []string{
				"chat", "+messages-list", "--conversation-id", "cid-1",
				"--time", "2026-07-17 10:00:00", "--size", "7", "--yes",
			},
			wantProduct: "chat",
			wantTool:    "list_conversation_message_v2",
			wantArgs:    map[string]any{"openconversation_id": "cid-1", "limit": 7},
			wantAbsent:  []string{"openCid", "cid"},
		},
		{
			name: "direct messages size",
			argv: []string{
				"chat", "+messages-list-direct", "--user", "u1",
				"--time", "2026-07-17 10:00:00", "--size", "8", "--yes",
			},
			wantProduct: "chat",
			wantTool:    "list_individual_chat_message",
			wantArgs:    map[string]any{"userId": "u1", "limit": 8},
		},
		{
			name:        "read status id",
			argv:        []string{"chat", "+messages-read-status", "--id", "cid-1", "--message-id", "msg-1", "--yes"},
			wantProduct: "im",
			wantTool:    "query_msg_read_status",
			wantArgs:    map[string]any{"openConversationId": "cid-1"},
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
			if fake.product != tc.wantProduct || fake.tool != tc.wantTool {
				t.Fatalf("call = %s/%s, want %s/%s", fake.product, fake.tool, tc.wantProduct, tc.wantTool)
			}
			for key, want := range tc.wantArgs {
				if got := fake.args[key]; got != want {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
			for _, key := range tc.wantAbsent {
				if _, ok := fake.args[key]; ok {
					t.Errorf("unexpected legacy argument %q in %#v", key, fake.args)
				}
			}
		})
	}
}
