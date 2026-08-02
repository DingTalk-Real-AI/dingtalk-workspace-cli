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
	"reflect"
	"strings"
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

type muteMemberResolutionCaller struct {
	calls []platformCoverageCaller
}

func (f *muteMemberResolutionCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, platformCoverageCaller{product: product, tool: tool, args: args})
	var text string
	switch product + "/" + tool {
	case "contact/get_user_info_by_user_ids":
		text = `{"result":[{"orgEmployeeModel":{"orgUserId":"user-2","orgUserName":"测试成员"}}]}`
	case "chat/get_group_members":
		text = `{"result":{"hasMore":false,"list":[{"memberEmpName":"测试成员","openDingtalkId":"D-open-2"}]}}`
	case "im/set_group_member_mute_list":
		text = `{"success":true}`
	default:
		text = `{"result":[]}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (f *muteMemberResolutionCaller) Format() string { return "json" }
func (f *muteMemberResolutionCaller) DryRun() bool   { return false }
func (f *muteMemberResolutionCaller) Fields() string { return "" }
func (f *muteMemberResolutionCaller) JQ() string     { return "" }

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
		{
			name:        "at all mute matches live MCP schema",
			argv:        []string{"chat", "+conversation-mute-at-all", "--conversation-id", "cid-1", "--yes"},
			wantProduct: "im",
			wantTool:    "update_at_all_notification_off",
			wantArgs:    map[string]any{"openConversationId": "cid-1", "mute": true},
			wantAbsent:  []string{"cid"},
		},
		{
			name:        "red envelope mute matches live MCP schema",
			argv:        []string{"chat", "+conversation-mute-red-envelope", "--conversation-id", "cid-1", "--off", "--yes"},
			wantProduct: "im",
			wantTool:    "update_red_env_notification_off",
			wantArgs:    map[string]any{"openConversationId": "cid-1", "mute": false},
			wantAbsent:  []string{"cid"},
		},
		{
			name: "message resource url msg id alias",
			argv: []string{
				"chat", "+messages-resource-url", "--resource-id", "resource-1",
				"--msg-id", "msg-1", "--open-conversation-id", "cid-1",
			},
			wantProduct: "im",
			wantTool:    "get_resource_download_url",
			wantArgs: map[string]any{
				"resourceType":       "mediaId",
				"resourceId":         "resource-1",
				"openMessageId":      "msg-1",
				"openConversationId": "cid-1",
			},
		},
		{
			name: "message resource url open message id alias",
			argv: []string{
				"chat", "+messages-resource-url", "--resource-id", "resource-1",
				"--open-message-id", "msg-1", "--open-conversation-id", "cid-1",
			},
			wantProduct: "im",
			wantTool:    "get_resource_download_url",
			wantArgs: map[string]any{
				"resourceType":       "mediaId",
				"resourceId":         "resource-1",
				"openMessageId":      "msg-1",
				"openConversationId": "cid-1",
			},
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

func TestCrossPlatformCoverageChatIDHelpers(t *testing.T) {
	t.Run("recognize open DingTalk IDs", func(t *testing.T) {
		tests := []struct {
			value string
			want  bool
		}{
			{value: "DingTalk-open-id", want: true},
			{value: " dingtalk-open-id ", want: true},
			{value: "user-id", want: false},
			{value: "   ", want: false},
		}
		for _, tc := range tests {
			if got := isOpenID(tc.value); got != tc.want {
				t.Errorf("isOpenID(%q) = %v, want %v", tc.value, got, tc.want)
			}
		}
	})

	t.Run("split mixed IDs", func(t *testing.T) {
		userIDs, openIDs := splitIDs([]string{" user-1 ", "", "D-open-1", "d-open-2", "user-2"})
		if want := []string{"user-1", "user-2"}; !reflect.DeepEqual(userIDs, want) {
			t.Fatalf("user IDs = %#v, want %#v", userIDs, want)
		}
		if want := []string{"D-open-1", "d-open-2"}; !reflect.DeepEqual(openIDs, want) {
			t.Fatalf("open IDs = %#v, want %#v", openIDs, want)
		}
	})

	t.Run("parse numeric IDs", func(t *testing.T) {
		got, err := toInt64Slice([]string{" 1 ", "", "-2"})
		if err != nil {
			t.Fatal(err)
		}
		if want := []int64{1, -2}; !reflect.DeepEqual(got, want) {
			t.Fatalf("numeric IDs = %#v, want %#v", got, want)
		}
		if _, err := toInt64Slice([]string{"not-a-number"}); err == nil {
			t.Fatal("invalid integer unexpectedly succeeded")
		}
		if _, err := toInt64Slice([]string{"", "  "}); err == nil {
			t.Fatal("empty integer list unexpectedly succeeded")
		}
	})
}

func TestChatMuteMemberResolvesUserIDToOpenDingTalkID(t *testing.T) {
	fake := &muteMemberResolutionCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-mute-member",
		"--group", "cid-1",
		"--users", "user-2,D-open-2",
		"--mute-time", "300000",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("calls = %d, want 3: %#v", len(fake.calls), fake.calls)
	}
	final := fake.calls[2]
	if final.product != "im" || final.tool != "set_group_member_mute_list" {
		t.Fatalf("final call = %s/%s, want im/set_group_member_mute_list", final.product, final.tool)
	}
	if _, ok := final.args["uids"]; ok {
		t.Fatalf("known-broken uids argument leaked into final call: %#v", final.args)
	}
	if got, want := final.args["openDingTalkIds"], []string{"D-open-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("openDingTalkIds = %#v, want %#v", got, want)
	}
}

func TestConversationCategoryTitleValidation(t *testing.T) {
	tests := []struct {
		name      string
		argv      []string
		wantTool  string
		wantTitle string
		wantError string
	}{
		{
			name:      "create accepts fifteen characters and trims",
			argv:      []string{"chat", "+category-create", "--title", "  123456789012345  ", "--yes"},
			wantTool:  "create_conv_category",
			wantTitle: "123456789012345",
		},
		{
			name:      "rename accepts unicode title and trims",
			argv:      []string{"chat", "+category-rename", "--category-id", "7", "--title", "  工作群  ", "--yes"},
			wantTool:  "rename_conv_category",
			wantTitle: "工作群",
		},
		{
			name:      "create rejects more than fifteen characters",
			argv:      []string{"chat", "+category-create", "--title", "1234567890123456", "--yes"},
			wantError: "最多 15 个字符",
		},
		{
			name:      "rename rejects whitespace-only title",
			argv:      []string{"chat", "+category-rename", "--category-id", "7", "--title", "   ", "--yes"},
			wantError: "--title 不能为空",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.argv)
			err := root.Execute()
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantError)
				}
				if fake.tool != "" {
					t.Fatalf("validation failure unexpectedly called %q", fake.tool)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fake.tool != tc.wantTool {
				t.Fatalf("tool = %q, want %q", fake.tool, tc.wantTool)
			}
			if got := fake.args["title"]; got != tc.wantTitle {
				t.Fatalf("title = %#v, want %q", got, tc.wantTitle)
			}
		})
	}
}
