// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageNaturalSearchAndSenderFlagsStayPublicOptional(t *testing.T) {
	tests := []struct {
		name  string
		flags []shortcut.Flag
		want  []string
	}{
		{name: "search natural adapters", flags: SearchMsg.Flags, want: []string{"chat-query", "sender-query"}},
		{name: "chat messages natural adapters", flags: ChatMessages.Flags, want: []string{"chat-query", "sender-query"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range tc.want {
				found := false
				for _, flag := range tc.flags {
					if flag.Name != name {
						continue
					}
					found = true
					if flag.Hidden {
						t.Fatalf("--%s unexpectedly became hidden", name)
					}
					if flag.Required {
						t.Fatalf("--%s unexpectedly became required", name)
					}
					break
				}
				if !found {
					t.Fatalf("public optional flag --%s is missing", name)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMessageTimeAlignmentFlagsStayPublicOptional(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []shortcut.Flag
	}{
		{name: "search message", flags: SearchMsg.Flags},
		{name: "conversation message", flags: ChatMessages.Flags},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{"start", "start-time", "end", "end-time", "order", "sort"} {
				found := false
				for _, flag := range tc.flags {
					if flag.Name != name {
						continue
					}
					found = true
					if flag.Hidden || flag.Required {
						t.Fatalf("--%s must stay public and optional: %#v", name, flag)
					}
					if (name == "order" || name == "sort") && !reflect.DeepEqual(flag.Enum, []string{"asc", "desc"}) {
						t.Fatalf("--%s enum = %#v", name, flag.Enum)
					}
					break
				}
				if !found {
					t.Fatalf("public optional flag --%s is missing", name)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessagesResolvesNaturalChatAndUserTargets(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantTool  string
		wantKey   string
		wantValue string
	}{
		{
			name:      "chat query",
			args:      []string{"chat", "+chat-messages", "--chat-query", "项目冲刺"},
			wantTool:  "list_conversation_message_v2",
			wantKey:   "openconversation_id",
			wantValue: "cid-1",
		},
		{
			name:      "natural group through group flag",
			args:      []string{"chat", "+chat-messages", "--group", "项目冲刺"},
			wantTool:  "list_conversation_message_v2",
			wantKey:   "openconversation_id",
			wantValue: "cid-1",
		},
		{
			name:      "user query",
			args:      []string{"chat", "+chat-messages", "--user-query", "张三"},
			wantTool:  "list_individual_chat_message",
			wantKey:   "openDingTalkId",
			wantValue: "open1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 2 {
				t.Fatalf("calls = %#v, want resolve + read", fake.calls)
			}
			read := fake.calls[1]
			if read.tool != tt.wantTool || read.args[tt.wantKey] != tt.wantValue {
				t.Fatalf("read = %#v", read)
			}
		})
	}
}

func TestCrossPlatformCoverageChatMessagesStableGroupBypassesNaturalResolution(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-messages", "--group", "cid-fixture-chat-0001"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "list_conversation_message_v2" ||
		fake.calls[0].args["openconversation_id"] != "cid-fixture-chat-0001" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageChatMessagesOptionallyFiltersResolvedSenderByEitherStableID(t *testing.T) {
	fake := &platformCoverageCaller{
		contactSearchResult: `{"result":[{"name":"测试用户甲","userId":"uid-fixture-sender-1","openDingTalkId":"D1"}]}`,
		chatMessagesResult: `{"result":{"hasMore":false,"messages":[
			{"openMessageId":"m-open","sender":"群昵称甲","senderOpenDingTalkId":"D1","content":"open id match"},
			{"openMessageId":"m-user","sender":"群昵称甲","senderUserId":"uid-fixture-sender-1","content":"user id match"},
			{"openMessageId":"m-other","sender":"其他人","senderOpenDingTalkId":"D2","content":"other"}
		]}}`,
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--group", "cid-fixture-chat-0001",
		"--sender-query", "测试用户甲", "--page-all",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].tool != "list_conversation_message_v2" ||
		fake.calls[1].tool != "search_contact_by_key_word" {
		t.Fatalf("calls = %#v, want message read followed by optional sender resolve", fake.calls)
	}

	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	if payload["complete"] != true || payload["count"] != float64(2) {
		t.Fatalf("filtered payload = %#v", payload)
	}
	filters, ok := payload["resolvedFilters"].(map[string]any)
	if !ok {
		t.Fatalf("resolvedFilters missing: %#v", payload)
	}
	senders, ok := filters["senders"].([]any)
	if !ok || len(senders) != 1 {
		t.Fatalf("resolved senders = %#v", filters["senders"])
	}
	selected := senders[0].(map[string]any)["selected"].(map[string]any)
	if selected["userId"] != "uid-fixture-sender-1" || selected["openDingTalkId"] != "D1" {
		t.Fatalf("selected identity = %#v", selected)
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", payload["messages"])
	}
	for _, raw := range messages {
		message := raw.(map[string]any)
		if message["sender"] != "群昵称甲" || message["senderId"] == "D2" {
			t.Fatalf("unexpected filtered message = %#v", message)
		}
	}
}

func TestCrossPlatformCoverageChatMessagesSenderResolutionFailureKeepsUnfilteredMessages(t *testing.T) {
	fake := &platformCoverageCaller{
		contactSearchResult: `{"result":[]}`,
		chatMessagesResult: `{"result":{"hasMore":false,"messages":[
			{"openMessageId":"m1","sender":"群昵称甲","senderOpenDingTalkId":"D1","content":"仍然返回"}
		]}}`,
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+chat-messages", "--group", "cid-fixture-chat-0001",
		"--sender-query", "不存在的人", "--page-all",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("optional sender failure stopped message read: %v", err)
	}
	if len(fake.calls) != 2 || fake.calls[0].tool != "list_conversation_message_v2" ||
		fake.calls[1].tool != "search_contact_by_key_word" {
		t.Fatalf("calls = %#v, want message read followed by non-blocking failed resolve", fake.calls)
	}

	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	if payload["count"] != float64(1) || payload["complete"] != false ||
		payload["partial"] != true || payload["failedCount"] != float64(1) ||
		payload["stopReason"] != "sender_resolution_failed" {
		t.Fatalf("unfiltered fallback payload = %#v", payload)
	}
	if _, exists := payload["resolvedFilters"]; exists {
		t.Fatalf("failed resolution unexpectedly published resolvedFilters: %#v", payload)
	}
	failures, ok := payload["failures"].([]any)
	if !ok || len(failures) != 1 || failures[0].(map[string]any)["stage"] != "sender_resolution" {
		t.Fatalf("resolution failures = %#v", payload["failures"])
	}
}

func TestCrossPlatformCoverageChatMessagesNaturalUserAmbiguityStopsBeforeMessageRead(t *testing.T) {
	fake := &platformCoverageCaller{contactSearchResult: `{"result":[{"name":"张三","userId":"u1","openDingTalkId":"D1"},{"name":"张三","userId":"u2","openDingTalkId":"D2"}]}`}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-messages", "--user-query", "张三"})
	if err := root.Execute(); err == nil {
		t.Fatal("ambiguous user unexpectedly reached message read")
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "search_contact_by_key_word" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageChatMessagesRejectsConversationIDInPeerIdentityFlag(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-messages", "--open-dingtalk-id", "cid-fixture-chat-0001"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--group") {
		t.Fatalf("error = %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("invalid identity reached lower API: %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageAtMeResolvesNaturalGroupBeforeSearch(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+at-me", "--chat-query", "项目冲刺"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[1].tool != "search_at_me_message" || fake.calls[1].args["openConversationId"] != "cid-1" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageAtMeStableIDInQueryBypassesNaturalResolution(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+at-me", "--chat-query", "cid-fixture-chat-0001"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "search_at_me_message" ||
		fake.calls[0].args["openConversationId"] != "cid-fixture-chat-0001" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageSendToGroupStableIDBypassesNaturalResolution(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+send-to-group", "--group", "cid-fixture-chat-0001", "--text", "评测", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "send_personal_message" ||
		fake.calls[0].args["openConversationId"] != "cid-fixture-chat-0001" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageSearchMsgResolvesNaturalChatAndSenderBeforeSearch(t *testing.T) {
	fake := &platformCoverageCaller{contactSearchResult: `{"result":[{"name":"张三","userId":"u1","openDingTalkId":"D1"}]}`}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+search-msg",
		"--chat-query", "项目冲刺",
		"--sender-query", "张三",
		"--no-enrich",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("calls = %#v, want chat resolve + user resolve + search", fake.calls)
	}
	search := fake.calls[2]
	if search.product != "im" || search.tool != "search_messages" {
		t.Fatalf("search = %#v", search)
	}
	if got, want := search.args["openConversationIds"], []string{"cid-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("openConversationIds = %#v, want %#v", got, want)
	}
	if got, want := search.args["senderOpenDingTakIds"], []string{"D1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("senderOpenDingTakIds = %#v, want %#v", got, want)
	}
}

func TestCrossPlatformCoverageSearchMsgPreservesResolvedSenderWhenDisplayNameDiffers(t *testing.T) {
	fake := &platformCoverageCaller{
		contactSearchResult:  `{"result":[{"name":"测试用户甲","userId":"uid-fixture-sender-1","openDingTalkId":"D1"}]}`,
		searchMessagesResult: `{"result":{"messages":[{"openMessageId":"m1","openConversationId":"cid-1","sender":"群昵称甲","senderOpenDingTalkId":"D1","content":"评测消息"}],"hasMore":false}}`,
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+search-msg", "--sender-query", "测试用户甲", "--no-enrich"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	filters, ok := payload["resolvedFilters"].(map[string]any)
	if !ok {
		t.Fatalf("resolvedFilters missing: %#v", payload)
	}
	senders, ok := filters["senders"].([]any)
	if !ok || len(senders) != 1 {
		t.Fatalf("resolved senders = %#v", filters["senders"])
	}
	resolution := senders[0].(map[string]any)
	selected := resolution["selected"].(map[string]any)
	if resolution["query"] != "测试用户甲" || selected["openDingTalkId"] != "D1" {
		t.Fatalf("sender resolution = %#v", resolution)
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", payload["messages"])
	}
	message := messages[0].(map[string]any)
	if message["sender"] != "群昵称甲" || message["senderId"] != "D1" {
		t.Fatalf("projected message = %#v", message)
	}
}

func TestCrossPlatformCoverageSearchMsgAcceptsStableIDInChatQuery(t *testing.T) {
	fake := &platformCoverageCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+search-msg",
		"--chat-query", "cid-fixture-chat-0002",
		"--text", "评测",
		"--no-enrich",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "search_messages" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	if got, want := fake.calls[0].args["openConversationIds"], []string{"cid-fixture-chat-0002"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("openConversationIds = %#v, want %#v", got, want)
	}
	if fake.calls[0].args["keyword"] != "评测" {
		t.Fatalf("keyword = %#v", fake.calls[0].args["keyword"])
	}
}

func TestCrossPlatformCoverageChatMembersListGroupAcceptsNameAndStableID(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		wantCalls int
	}{
		{name: "name", args: []string{"chat", "+chat-members-list", "--group", "项目冲刺", "--member-types", "user"}, wantCalls: 2},
		{name: "stable id", args: []string{"chat", "+chat-members-list", "--group", "cid-fixture-chat-0001", "--member-types", "user"}, wantCalls: 1},
		{name: "compat alias", args: []string{"chat", "+chat-members-list", "--open-conversation-id", "cid-short-placeholder", "--member-types", "user"}, wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := &platformCoverageCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != tt.wantCalls || fake.calls[len(fake.calls)-1].tool != "get_group_members" {
				t.Fatalf("calls = %#v", fake.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageSearchMsgNaturalSenderAmbiguityStopsBeforeSearch(t *testing.T) {
	fake := &platformCoverageCaller{contactSearchResult: `{"result":[{"name":"张三","userId":"u1","openDingTalkId":"D1"},{"name":"张三","userId":"u2","openDingTalkId":"D2"}]}`}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+search-msg", "--sender-query", "张三", "--no-enrich"})
	if err := root.Execute(); err == nil {
		t.Fatal("ambiguous sender unexpectedly reached search")
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "search_contact_by_key_word" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}
