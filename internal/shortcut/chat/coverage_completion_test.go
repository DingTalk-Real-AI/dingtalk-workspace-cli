// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageConversationValidationAndTypeVariants(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("cid-%d", i)
	}
	root.SetArgs([]string{
		"chat", "+conversation-set-top",
		"--conversation-ids", strings.Join(tooMany, ","),
		"--yes",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("more than ten conversation IDs were accepted")
	}

	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+category-create", "--title", "   ", "--yes"})
	if err := root.Execute(); err == nil {
		t.Fatal("blank category title was accepted")
	}
	shortcut.Register(shortcut.Shortcut{
		Service:  "chat",
		Command:  "+coverage-category-title",
		Flags:    []shortcut.Flag{{Name: "title", Type: shortcut.FlagString}},
		Validate: validateConversationCategoryTitle,
		Execute:  func(*shortcut.RuntimeContext) error { return nil },
	})
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+coverage-category-title", "--title", "   "})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("direct blank category validation error = %v", err)
	}

	tests := []struct {
		value map[string]any
		want  string
	}{
		{map[string]any{"singleChat": "true"}, "direct"},
		{map[string]any{"singleChat": "false"}, "group"},
		{map[string]any{"singleChat": float64(1)}, "direct"},
		{map[string]any{"singleChat": float64(0)}, "group"},
		{map[string]any{"singleChat": 1}, "direct"},
		{map[string]any{"singleChat": 0}, "group"},
		{map[string]any{"conversationType": "group_chat"}, "group"},
	}
	for _, tc := range tests {
		if got, ok := conversationListTopType(tc.value); !ok || got != tc.want {
			t.Errorf("conversationListTopType(%#v) = %q, %v; want %q", tc.value, got, ok, tc.want)
		}
	}
}

func TestCrossPlatformCoverageConversationAndGroupListExecution(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	for _, args := range [][]string{
		{"chat", "+conversation-set-top", "--conversation-id", "cid", "--yes"},
		{"chat", "+conversation-list", "--limit", "1", "--cursor", "1", "--exclude-muted"},
		{"chat", "+conversation-list-top", "--limit", "1", "--cursor", "1", "--exclude-muted", "--type", "group"},
		{"chat", "+category-list-conversations", "--category-id", "1", "--exclude-muted"},
		{"chat", "+chat-list-mine", "--role", "OWNER", "--limit", "1", "--exclude-muted"},
		{"chat", "+chat-list-all", "--limit", "1", "--cursor", "next"},
		{"chat", "+messages-list-pin", "--open-conversation-id", "cid", "--cursor", "next", "--size", "1"},
	} {
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestCrossPlatformCoverageConversationListFailureBoundaries(t *testing.T) {
	for _, tail := range [][]string{
		{"--limit", "0"},
		{"--limit", "101"},
		{"--page-limit", "2"},
		{"--page-all", "--page-limit", "0"},
		{"--page-all", "--page-limit", "501"},
	} {
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+conversation-list"}, tail...))
		if err := root.Execute(); err == nil {
			t.Errorf("invalid conversation args succeeded: %v", tail)
		}
	}

	for _, tc := range []struct {
		name      string
		caller    *larkAlignmentCaller
		args      []string
		wantError bool
	}{
		{name: "first read failure", caller: &larkAlignmentCaller{failProductTool: "im/list_all_conversations"}, wantError: true},
		{name: "later read failure", caller: &larkAlignmentCaller{
			sequenceResponses: map[string][]string{"im/list_all_conversations": {`{"result":{"conversationList":[],"hasMore":true,"nextCursor":2}}`}},
			failProductToolAt: map[string]int{"im/list_all_conversations": 2},
		}, args: []string{"--page-all"}},
		{name: "missing pagination", caller: &larkAlignmentCaller{responses: map[string]string{"im/list_all_conversations": `{"result":{"conversationList":[]}}`}}},
		{name: "invalid cursor", caller: &larkAlignmentCaller{responses: map[string]string{"im/list_all_conversations": `{"result":{"conversationList":[],"hasMore":true,"nextCursor":"bad"}}`}}},
		{name: "stalled cursor", caller: &larkAlignmentCaller{responses: map[string]string{"im/list_all_conversations": `{"result":{"conversationList":[],"hasMore":true,"nextCursor":2}}`}}, args: []string{"--page-all", "--cursor", "2"}},
		{name: "page limit", caller: &larkAlignmentCaller{responses: map[string]string{"im/list_all_conversations": `{"result":{"conversationList":[],"hasMore":true,"nextCursor":2}}`}}, args: []string{"--page-all", "--page-limit", "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.caller)
			root := newPlatformCoverageRoot()
			root.SetArgs(append([]string{"chat", "+conversation-list"}, tc.args...))
			err := root.Execute()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
		})
	}

	for value, want := range map[any]int64{int(1): 1, int64(2): 2, float64(3): 3, "4": 4} {
		got, err := conversationPaginationCursor(value)
		if err != nil || got != want {
			t.Fatalf("cursor %#v = %d, %v; want %d", value, got, err, want)
		}
	}
	if _, err := conversationPaginationCursor(struct{}{}); err == nil {
		t.Fatal("unsupported cursor unexpectedly accepted")
	}
	if got := unwrapConversationTuple(nil); got != nil {
		t.Fatalf("empty tuple = %#v", got)
	}
}

func TestCrossPlatformCoverageChatCreateAndReplyFailures(t *testing.T) {
	tests := []struct {
		name      string
		caller    *larkAlignmentCaller
		args      []string
		wantError string
	}{
		{
			name:      "owner query resolution",
			caller:    &larkAlignmentCaller{failProductTool: "contact/search_contact_by_key_word"},
			args:      []string{"chat", "+chat-create", "--name", "群", "--users", "u1", "--owner-query", "群主", "--yes"},
			wantError: "fixture lower call failed",
		},
		{
			name:      "current profile call",
			caller:    &larkAlignmentCaller{failProductTool: "contact/get_current_user_profile"},
			args:      []string{"chat", "+chat-create", "--name", "群", "--users", "u1", "--yes"},
			wantError: "读取当前用户",
		},
		{
			name: "missing current user",
			caller: &larkAlignmentCaller{responses: map[string]string{
				"contact/get_current_user_profile": `{"result":[]}`,
			}},
			args:      []string{"chat", "+chat-create", "--name", "群", "--users", "u1", "--yes"},
			wantError: "缺少 userId",
		},
		{
			name:      "create write",
			caller:    &larkAlignmentCaller{failProductTool: "im/create_group_conversation"},
			args:      []string{"chat", "+chat-create", "--name", "群", "--users", "u1", "--yes"},
			wantError: "fixture lower call failed",
		},
		{
			name:      "explicit sender lookup",
			caller:    &larkAlignmentCaller{failProductTool: "contact/search_contact_by_key_word"},
			args:      []string{"chat", "+messages-reply", "--conversation-id", "cid", "--message-id", "msg", "--ref-sender", "user-id", "--text", "收到", "--yes"},
			wantError: "解析为 openDingTalkId",
		},
		{
			name: "explicit sender unresolved",
			caller: &larkAlignmentCaller{responses: map[string]string{
				"contact/search_contact_by_key_word": `{"result":[]}`,
			}},
			args:      []string{"chat", "+messages-reply", "--conversation-id", "cid", "--message-id", "msg", "--ref-sender", "user-id", "--text", "收到", "--yes"},
			wantError: "没有精确匹配",
		},
		{
			name:      "referenced message lookup",
			caller:    &larkAlignmentCaller{failProductTool: "im/list_messages_by_ids"},
			args:      []string{"chat", "+messages-reply", "--conversation-id", "cid", "--message-id", "msg", "--text", "收到", "--yes"},
			wantError: "读取被引用消息",
		},
		{
			name: "referenced message missing sender",
			caller: &larkAlignmentCaller{responses: map[string]string{
				"im/list_messages_by_ids": `{"result":[{"openMessageId":"other","senderOpenDingTalkId":"D-other"},{"openMessageId":"msg"}]}`,
			}},
			args:      []string{"chat", "+messages-reply", "--conversation-id", "cid", "--message-id", "msg", "--text", "收到", "--yes"},
			wantError: "未返回 senderOpenDingTalkId",
		},
		{
			name:      "feed source",
			caller:    &larkAlignmentCaller{failProductTool: "im/list_conversations_by_category"},
			args:      []string{"chat", "+feed-group-query-item", "--category-id", "1", "--conversation-ids", "cid"},
			wantError: "fixture lower call failed",
		},
		{
			name:      "chat update resolution",
			caller:    &larkAlignmentCaller{responses: map[string]string{"im/search_groups": `{"result":[]}`}},
			args:      []string{"chat", "+chat-update", "--group", "missing", "--name", "新群", "--yes"},
			wantError: "没有找到",
		},
		{
			name:      "reply write",
			caller:    &larkAlignmentCaller{failProductTool: "chat/send_personal_message"},
			args:      []string{"chat", "+messages-reply", "--conversation-id", "cid", "--message-id", "msg", "--ref-sender", fixtureCurrentDOpenID, "--text", "收到", "--yes"},
			wantError: "fixture lower call failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.caller)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantError)
			}
		})
	}

	dry := &larkAlignmentCaller{}
	helpers.InitDeps(dry)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-create", "--name", "群", "--users", "u1", "--dry-run", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(dry.calls) != 2 ||
		dry.calls[0].tool != "get_current_user_profile" ||
		dry.calls[1].tool != "create_group_conversation" {
		t.Fatalf("chat-create dry-run calls = %#v", dry.calls)
	}

	external := &larkAlignmentCaller{responses: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"name":"外部联系人","openDingTalkId":"D-external"}]}`,
	}}
	helpers.InitDeps(external)
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-create", "--name", "群", "--owner-open-dingtalk-id", fixtureCurrentDOpenID, "--member-query", "外部联系人", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageNaturalGroupReadFailures(t *testing.T) {
	for _, command := range []string{"+chat-invite-url", "+chat-bots"} {
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{"im/search_groups": `{"result":[]}`}})
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", command, "--group", "missing"})
		if err := root.Execute(); err == nil {
			t.Errorf("%s missing natural group unexpectedly resolved", command)
		}
	}
}

func TestCrossPlatformCoverageReplyShapeHelpers(t *testing.T) {
	if got := findOpenDingTalkID([]map[string]any{{"openDingTalkId": "D-map"}}); got != "D-map" {
		t.Fatalf("findOpenDingTalkID([]map) = %q", got)
	}
	if got := shortcutMessageMaps(nil); got != nil {
		t.Fatalf("nil message maps = %#v", got)
	}
	if got := shortcutMessageMaps(map[string]any{}); got != nil {
		t.Fatalf("empty message maps = %#v", got)
	}
	maps := shortcutMessageMaps(map[string]any{
		"data": map[string]any{
			"items": []map[string]any{{"openMessageId": "msg"}},
		},
	})
	if len(maps) != 1 || maps[0]["openMessageId"] != "msg" {
		t.Fatalf("nested []map message maps = %#v", maps)
	}

	if got := currentProfileUserID(map[string]any{"result": map[string]any{"userId": "nested"}}); got != "nested" {
		t.Fatalf("nested current profile = %q", got)
	}
	if got := currentProfileUserID(map[string]any{"userId": "direct"}); got != "direct" {
		t.Fatalf("direct current profile = %q", got)
	}
	data := map[string]any{"result": "not-a-map"}
	normalizeCreatedConversation(data)
	if data["result"] != "not-a-map" {
		t.Fatalf("normalization changed non-map result: %#v", data)
	}
}

func TestCrossPlatformCoverageFlagAndMgetValidation(t *testing.T) {
	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("msg-%d", i)
	}
	cases := [][]string{
		{"chat", "+flag-create", "--message-ids", strings.Join(tooMany, ","), "--conversation-id", "cid", "--yes"},
		{"chat", "+flag-list", "--cursor", "-1"},
		{"chat", "+flag-list", "--size", "31"},
		{"chat", "+messages-mget", "--msg-ids", strings.Join(makeIDs(51), ",")},
		{"chat", "+messages-mget", "--msg-ids", "msg", "--download-resources", "--output-dir", "../escape"},
	}
	for _, args := range cases {
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("invalid args unexpectedly succeeded: %v", args)
		}
	}

	fake := &larkAlignmentCaller{failProductTool: "im/list_messages_by_ids"}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+messages-mget", "--msg-ids", "msg", "--yes"})
	if err := root.Execute(); err == nil {
		t.Fatal("mget lower error was swallowed")
	}
}

func TestCrossPlatformCoverageRecallCardAndLedgerBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caller *larkAlignmentCaller
		args   []string
	}{
		{name: "multiple recall ids", caller: &larkAlignmentCaller{}, args: []string{"chat", "+messages-recall", "--message-ids", "m1,m2", "--yes"}},
		{name: "recall lookup failure", caller: &larkAlignmentCaller{failProductTool: "im/list_messages_by_ids"}, args: []string{"chat", "+messages-recall", "--msg-id", "m1", "--yes"}},
		{name: "recall lookup empty", caller: &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":[]}`}}, args: []string{"chat", "+messages-recall", "--msg-id", "m1", "--yes"}},
		{name: "recall lookup missing conversation", caller: &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":[{"openMessageId":"m1"}]}`}}, args: []string{"chat", "+messages-recall", "--msg-id", "m1", "--yes"}},
		{name: "card status low", caller: &larkAlignmentCaller{}, args: []string{"chat", "+messages-update-card", "--biz-id", "b", "--content", "x", "--flow-status", "0", "--yes"}},
		{name: "card status high", caller: &larkAlignmentCaller{}, args: []string{"chat", "+messages-update-card", "--biz-id", "b", "--content", "x", "--flow-status", "6", "--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.caller)
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil {
				t.Fatal("failure boundary unexpectedly succeeded")
			}
		})
	}

	helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
		"im/update_streaming_card": `{"result":{"updated":true}}`,
	}})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+messages-update-card", "--biz-id", "b", "--content", "x", "--flow-status", "3", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for value, want := range map[any]int{
		int(1): 1, int32(2): 2, int64(3): 3, float32(4): 4, float64(5): 5, "bad": 0,
	} {
		if got := messageLedgerInt(value); got != want {
			t.Fatalf("messageLedgerInt(%#v) = %d, want %d", value, got, want)
		}
	}
}

func TestCrossPlatformCoverageFeedCompleteAndExcludeMuted(t *testing.T) {
	fake := &larkAlignmentCaller{category: `{"result":{"hasMore":false,"list":[{"openConversationId":"cid"}]}}`}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+feed-group-query-item",
		"--category-id", "1",
		"--conversation-ids", "cid",
		"--exclude-muted",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].args["excludeMuted"] != true {
		t.Fatalf("feed calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageUnifiedSendValidationMatrix(t *testing.T) {
	cases := [][]string{
		{"--identity", "user", "--group", "cid", "--users", "u1", "--text", "x"},
		{"--identity", "user", "--text", "x"},
		{"--identity", "user", "--group", "cid", "--open-dingtalk-id", fixtureCurrentDOpenID, "--text", "x"},
		{"--identity", "user", "--group", "cid", "--at-user-ids", "u1", "--text", "x"},
		{"--identity", "user", "--open-dingtalk-id", fixtureCurrentDOpenID, "--at-all", "--text", "x"},
		{"--identity", "bot", "--group", "cid", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--group", "cid", "--users", "u1", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--group", "cid", "--open-dingtalk-id", fixtureCurrentDOpenID, "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--group", "cid", "--at-mobiles", "13800000000", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--users", "u1", "--at-user-ids", "u2", "--text", "x"},
		{"--identity", "webhook", "--text", "x"},
		{"--identity", "webhook", "--webhook-token", "token", "--group", "cid", "--text", "x"},
		{"--identity", "webhook", "--webhook-token", "token", "--at-open-dingtalk-ids", fixtureCurrentDOpenID, "--text", "x"},
		{"--identity", "webhook", "--webhook-token", "token", "--uuid", "key", "--text", "x"},
	}
	for _, tail := range cases {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		args := append([]string{"chat", "+messages-send"}, tail...)
		args = append(args, "--yes")
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("invalid unified send unexpectedly succeeded: %v", tail)
		}
		if len(fake.calls) != 0 {
			t.Errorf("invalid unified send reached lower service: %v => %#v", tail, fake.calls)
		}
	}
}

func TestCrossPlatformCoverageUnifiedSendOptionalArgumentsAndErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "user group mentions",
			args: []string{"--identity", "user", "--group", "cid", "--text", "x", "--at-open-dingtalk-ids", fixtureCurrentDOpenID + "," + fixtureCurrentDOpenID2, "--at-all"},
			want: map[string]any{"atOpenDingTalkIds": []string{fixtureCurrentDOpenID, fixtureCurrentDOpenID2}, "atAll": true},
		},
		{
			name: "user direct",
			args: []string{"--identity", "user", "--open-dingtalk-id", fixtureCurrentDOpenID, "--text", "x"},
			want: map[string]any{"receiverOpenDingTalkId": fixtureCurrentDOpenID},
		},
		{
			name: "bot group mentions",
			args: []string{"--identity", "bot", "--robot-code", "r", "--group", "cid", "--text", "x", "--at-user-ids", "u1", "--at-open-dingtalk-ids", fixtureCurrentDOpenID},
			want: map[string]any{"atUserIds": []string{"u1"}, "atOpendingtalkIds": []string{fixtureCurrentDOpenID}},
		},
		{
			name: "bot direct targets",
			args: []string{"--identity", "bot", "--robot-code", "r", "--users", "u1", "--open-dingtalk-ids", fixtureCurrentDOpenID, "--text", "x", "--at-all"},
			want: map[string]any{"userIds": []string{"u1"}, "openDingtalkIds": []string{fixtureCurrentDOpenID}, "isAtAll": "true"},
		},
		{
			name: "webhook mentions",
			args: []string{"--identity", "webhook", "--webhook-token", "token", "--text", "x", "--at-user-ids", "u1", "--at-mobiles", "13800000000"},
			want: map[string]any{"atUserIds": []string{"u1"}, "atMobiles": []string{"13800000000"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			args := append([]string{"chat", "+messages-send"}, tc.args...)
			args = append(args, "--yes")
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			call := fake.calls[len(fake.calls)-1]
			for key, want := range tc.want {
				if !reflect.DeepEqual(call.args[key], want) {
					t.Errorf("%s = %#v, want %#v", key, call.args[key], want)
				}
			}
		})
	}

	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+messages-send", "--identity", "user", "--group", "cid", "--text", "x", "--dry-run", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("unified send dry-run reached lower service: %#v", fake.calls)
	}

	fake = &larkAlignmentCaller{failProductTool: "chat/send_personal_message"}
	helpers.InitDeps(fake)
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+messages-send", "--identity", "user", "--group", "cid", "--text", "x", "--yes"})
	if err := root.Execute(); err == nil {
		t.Fatal("unified send write error was swallowed")
	}

	if got := shortcutMessageTitle(strings.Repeat("界", 45)); len([]rune(got)) != 40 {
		t.Fatalf("long generated title has %d runes", len([]rune(got)))
	}
}

func TestCrossPlatformCoverageUnifiedSendUnsupportedIdentityGuard(t *testing.T) {
	flags := append([]shortcut.Flag(nil), MessagesSend.Flags...)
	for i := range flags {
		if flags[i].Name == "identity" || flags[i].Name == "as" {
			flags[i].Enum = nil
		}
	}
	shortcut.Register(shortcut.Shortcut{
		Service: "chat",
		Command: "+coverage-unified-send",
		Flags:   flags,
		Execute: executeMessagesSend,
	})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+coverage-unified-send", "--identity", "unsupported", "--text", "x"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported identity") {
		t.Fatalf("unsupported identity error = %v", err)
	}
}

func TestCrossPlatformCoverageUnifiedSendGroupFileAndBatchBoundaries(t *testing.T) {
	readGroupFile := messagesSendReadGroupFile
	t.Cleanup(func() { messagesSendReadGroupFile = readGroupFile })

	for _, tail := range [][]string{
		{"--identity", "bot", "--robot-code", "r", "--group", "cid", "--groups", "c1", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--groups", "c1", "--groups-file", "groups.txt", "--text", "x"},
		{"--identity", "webhook", "--webhook-token", "token", "--chat-query", "群", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--groups", "", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--groups", strings.Join(makeIDs(101), ","), "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--groups-file", "/absolute.txt", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--groups-file", "../escape.txt", "--text", "x"},
		{"--identity", "bot", "--robot-code", "r", "--groups-file", "missing.txt", "--text", "x"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+messages-send"}, append(tail, "--yes")...))
		if err := root.Execute(); err == nil {
			t.Errorf("invalid unified send args succeeded: %v", tail)
		}
	}

	temp := t.TempDir()
	if err := os.Mkdir(filepath.Join(temp, "groups-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "large.txt"), make([]byte, messagesSendMaxGroupFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "unreadable.txt"), []byte("cid"), 0o600); err != nil {
		t.Fatal(err)
	}
	messagesSendReadGroupFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == "unreadable.txt" {
			return nil, os.ErrPermission
		}
		return readGroupFile(path)
	}
	t.Chdir(temp)
	for _, path := range []string{"groups-dir", "large.txt", "unreadable.txt"} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+messages-send", "--identity", "bot", "--robot-code", "r", "--groups-file", path, "--text", "x", "--yes"})
		if err := root.Execute(); err == nil {
			t.Errorf("invalid group file %q succeeded", path)
		}
	}

	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send", "--identity", "bot", "--robot-code", "r",
		"--groups", "c1,c2", "--text", "x", "--at-user-ids", "u1",
		"--at-open-dingtalk-ids", fixtureCurrentDOpenID, "--at-all", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("batch calls = %#v", fake.calls)
	}
	for _, call := range fake.calls {
		if !reflect.DeepEqual(call.args["atUserIds"], []string{"u1"}) ||
			!reflect.DeepEqual(call.args["atOpendingtalkIds"], []string{fixtureCurrentDOpenID}) ||
			call.args["isAtAll"] != "true" {
			t.Fatalf("batch mention args = %#v", call.args)
		}
	}

	shortcut.Register(shortcut.Shortcut{
		Service: "chat", Command: "+coverage-unified-execute-error",
		Flags: MessagesSend.Flags, Execute: executeMessagesSend,
	})
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+coverage-unified-execute-error", "--identity", "bot", "--robot-code", "r",
		"--groups-file", "missing-again.txt", "--text", "x",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("execute-time group file error was swallowed")
	}
}

func TestCrossPlatformCoverageMuteMemberResolutionFailures(t *testing.T) {
	tests := []struct {
		name   string
		caller *muteMemberScenarioCaller
	}{
		{"contact error", &muteMemberScenarioCaller{contactErr: errors.New("contact failed")}},
		{"contact missing name", &muteMemberScenarioCaller{contactText: `{"result":[]}`}},
		{"member error", &muteMemberScenarioCaller{memberErr: errors.New("members failed")}},
		{"member missing fields", &muteMemberScenarioCaller{memberMode: "missing-fields"}},
		{"member cursor missing", &muteMemberScenarioCaller{memberMode: "missing-cursor"}},
		{"member ambiguous", &muteMemberScenarioCaller{memberMode: "ambiguous"}},
		{"member page limit", &muteMemberScenarioCaller{memberMode: "page-limit"}},
		{"unrelated directory result", &muteMemberScenarioCaller{
			contactText: `{"result":[{"orgEmployeeModel":{"orgUserId":"other","orgUserName":"其他"}},{"orgEmployeeModel":{"orgUserId":"user-1","orgUserName":"测试成员"}}]}`,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.caller)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{"chat", "+chat-mute-member", "--group", "cid", "--users", "user-1", "--off", "--yes"})
			if err := root.Execute(); err == nil {
				t.Fatal("resolution failure scenario unexpectedly succeeded")
			}
		})
	}

	for _, tc := range []struct {
		value any
		want  string
	}{
		{float64(1.5), "1.5"},
		{int(2), "2"},
		{int64(3), "3"},
	} {
		if got := shortcutString(map[string]any{"value": tc.value}, "value"); got != tc.want {
			t.Errorf("shortcutString(%T) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

type muteMemberScenarioCaller struct {
	contactErr  error
	memberErr   error
	contactText string
	memberMode  string
}

func (c *muteMemberScenarioCaller) CallTool(
	_ context.Context,
	product, tool string,
	args map[string]any,
) (*edition.ToolResult, error) {
	switch product + "/" + tool {
	case "contact/get_user_info_by_user_ids":
		if c.contactErr != nil {
			return nil, c.contactErr
		}
		text := c.contactText
		if text == "" {
			text = `{"result":[{"orgEmployeeModel":{"orgUserId":"user-1","orgUserName":"测试成员"}}]}`
		}
		return textResult(text), nil
	case "chat/get_group_members":
		if c.memberErr != nil {
			return nil, c.memberErr
		}
		switch c.memberMode {
		case "missing-fields":
			return textResult(`{"result":{"hasMore":false,"list":[{"memberEmpName":"","openDingtalkId":""}]}}`), nil
		case "missing-cursor":
			return textResult(`{"result":{"hasMore":true,"list":[{"memberEmpName":"测试成员","openDingtalkId":"D1"}]}}`), nil
		case "ambiguous":
			return textResult(`{"result":{"hasMore":false,"list":[{"memberEmpName":"测试成员","openDingtalkId":"D1"},{"memberEmpName":"测试成员","openDingtalkId":"D2"}]}}`), nil
		case "page-limit":
			cursor, _ := strconv.Atoi(fmt.Sprint(args["cursor"]))
			return textResult(fmt.Sprintf(
				`{"result":{"hasMore":true,"nextCursor":"%d","list":[{"memberEmpName":"测试成员","openDingtalkId":"D1"}]}}`,
				cursor+1,
			)), nil
		default:
			return textResult(`{"result":{"hasMore":false,"list":[]}}`), nil
		}
	default:
		return textResult(`{"success":true}`), nil
	}
}

func (*muteMemberScenarioCaller) Format() string { return "json" }
func (*muteMemberScenarioCaller) DryRun() bool   { return false }
func (*muteMemberScenarioCaller) Fields() string { return "" }
func (*muteMemberScenarioCaller) JQ() string     { return "" }

func textResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func makeIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	return ids
}
