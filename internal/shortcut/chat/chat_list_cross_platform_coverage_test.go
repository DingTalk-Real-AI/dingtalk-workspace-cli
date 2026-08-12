// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageChatListDefaultsAndLarkAliases(t *testing.T) {
	fake := &larkAlignmentCaller{
		responses: map[string]string{
			"im/list_all_conversations": `{
				"result":{
					"hasMore":true,
					"nextCursor":"7",
					"list":[
						{"openConversationId":"cid-group","conversationName":"项目群","singleChat":false,"ownerUserId":"owner-1"},
						{"openConversationId":"cid-direct","title":"张三","singleChat":true},
						{"openConversationId":"cid-unknown","title":"未知"}
					]
				}
			}`,
		},
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list", "--exclude-muted", "--page-size", "20"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "list_all_conversations" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	if fake.calls[0].args["limit"] != 20 || fake.calls[0].args["excludeMuted"] != true {
		t.Fatalf("args = %#v", fake.calls[0].args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) {
		t.Fatalf("default group filter count = %#v", payload)
	}
	chats := payload["chats"].([]any)
	chat := chats[0].(map[string]any)
	if chat["openConversationId"] != "cid-group" || chat["conversationType"] != "group" || chat["chatMode"] != "group" {
		t.Fatalf("chat = %#v", chat)
	}
	if !reflect.DeepEqual(payload["requestedTypes"], []any{"group"}) {
		t.Fatalf("requestedTypes = %#v", payload["requestedTypes"])
	}
	if filter, _ := payload["filter"].(map[string]any); filter["excludeMuted"] != true {
		t.Fatalf("filter = %#v", payload["filter"])
	}
}

func TestCrossPlatformCoverageChatListTypesPaginationAndValidation(t *testing.T) {
	t.Run("group and p2p with page token", func(t *testing.T) {
		fake := &larkAlignmentCaller{
			responses: map[string]string{
				"im/list_all_conversations": `{
					"result":{"list":[
						{"openConversationId":"cid-group","name":"项目群","conversationType":"group"},
						{"openConversationId":"cid-direct","name":"李四","conversationType":"P2P"}
					]}
				}`,
			},
		}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{"chat", "+chat-list", "--types", "group,p2p", "--page-token", "3"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if fake.calls[0].args["cursor"] != 3 || fake.calls[0].args["limit"] != 20 {
			t.Fatalf("args = %#v", fake.calls[0].args)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["count"] != float64(2) {
			t.Fatalf("both types count = %#v", payload)
		}
		direct := payload["chats"].([]any)[1].(map[string]any)
		if direct["conversationType"] != "direct" || direct["chatMode"] != "p2p" {
			t.Fatalf("direct chat = %#v", direct)
		}
	})

	t.Run("p2p only via limit alias", func(t *testing.T) {
		fake := &larkAlignmentCaller{
			responses: map[string]string{
				"im/list_all_conversations": `{
					"result":{"list":[
						{"openConversationId":"cid-group","name":"项目群","singleChat":false},
						{"openConversationId":"cid-direct","name":"王五","singleChat":true}
					]}
				}`,
			},
		}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{"chat", "+chat-list", "--types", "p2p", "--limit", "5"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if fake.calls[0].args["limit"] != 5 {
			t.Fatalf("limit alias args = %#v", fake.calls[0].args)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["count"] != float64(1) {
			t.Fatalf("p2p-only count = %#v", payload)
		}
	})

	t.Run("cursor alias", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/list_all_conversations": `{"result":{"list":[]}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(&bytes.Buffer{})
		root.SetArgs([]string{"chat", "+chat-list", "--cursor", "2"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if fake.calls[0].args["cursor"] != 2 {
			t.Fatalf("cursor args = %#v", fake.calls[0].args)
		}
	})

	validationCases := []struct {
		name string
		args []string
	}{
		{name: "invalid types", args: []string{"chat", "+chat-list", "--types", "channel"}},
		{name: "page size too large", args: []string{"chat", "+chat-list", "--page-size", "101"}},
		{name: "page size zero", args: []string{"chat", "+chat-list", "--limit", "0"}},
		{name: "negative cursor", args: []string{"chat", "+chat-list", "--cursor", "-1"}},
		{name: "invalid page token", args: []string{"chat", "+chat-list", "--page-token", "bad"}},
		{name: "empty type token", args: []string{"chat", "+chat-list", "--types", "group,"}},
	}
	for _, tc := range validationCases {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(&larkAlignmentCaller{})
			root := newPlatformCoverageRoot()
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	t.Run("default page size from unchanged flag", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/list_all_conversations": `{"result":{"list":[]}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(&bytes.Buffer{})
		root.SetArgs([]string{"chat", "+chat-list", "--page-size", "15"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if fake.calls[0].args["limit"] != 15 {
			t.Fatalf("default page-size args = %#v", fake.calls[0].args)
		}
	})

	t.Run("mcp failure", func(t *testing.T) {
		helpers.InitDeps(&larkAlignmentCaller{failProductTool: "im/list_all_conversations"})
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+chat-list"})
		if err := root.Execute(); err == nil {
			t.Fatal("expected MCP error")
		}
	})
}

func TestCrossPlatformCoverageChatListProjectionHelpers(t *testing.T) {
	rows := chatListProject(map[string]any{"result": map[string]any{"list": []any{
		map[string]any{"openConversationId": "cid", "conversationName": "G", "singleChat": false},
		map[string]any{"openConversationId": "did", "title": "D", "singleChat": true},
	}}})
	if len(rows) != 2 || rows[0]["conversationType"] != "group" || rows[1]["conversationType"] != "direct" {
		t.Fatalf("projected rows = %#v", rows)
	}
	filtered := chatListFilterTypes(rows, []string{"p2p"})
	if len(filtered) != 1 || filtered[0]["openConversationId"] != "did" {
		t.Fatalf("filtered = %#v", filtered)
	}
	types, err := normalizeChatListTypes([]string{"GROUP", "p2p", "group"})
	if err != nil || len(types) != 2 {
		t.Fatalf("normalize types = %#v, err = %v", types, err)
	}
	if _, err := normalizeChatListTypes([]string{"bad"}); err == nil {
		t.Fatal("invalid type accepted")
	}
	var tuple map[string]any
	if err := json.Unmarshal([]byte(`{"result":[[{"openConversationId":"tuple"}],2,true]}`), &tuple); err != nil {
		t.Fatal(err)
	}
	page, err := chatListPagination(tuple)
	if err != nil || page["nextCursor"] != float64(2) || page["hasMore"] != true || len(chatListProject(tuple)) != 1 {
		t.Fatalf("tuple normalization rows=%#v page=%#v err=%v", chatListProject(tuple), page, err)
	}
	ordinary := map[string]any{"result": []any{map[string]any{"openConversationId": "ordinary"}}}
	page, err = chatListPagination(ordinary)
	if err != nil || page != nil || len(chatListProject(ordinary)) != 1 {
		t.Fatalf("ordinary list rows=%#v page=%#v err=%v", chatListProject(ordinary), page, err)
	}
}

func TestCrossPlatformCoverageChatListExecuteAndHelperEdges(t *testing.T) {
	root := newPlatformCoverageRoot()
	chatCmd, _, err := root.Find([]string{"chat", "+chat-list"})
	if err != nil {
		t.Fatal(err)
	}
	rt := shortcut.RuntimeContextForTest(chatCmd, ChatList)
	if got := chatListPageSize(rt); got != 20 {
		t.Fatalf("default page size = %d, want 20", got)
	}
	unsetPageSizeCmd, _, err := root.Find([]string{"chat", "+chat-list"})
	if err != nil {
		t.Fatal(err)
	}
	if err := unsetPageSizeCmd.Flags().Set("page-size", "15"); err != nil {
		t.Fatal(err)
	}
	if got := chatListPageSize(shortcut.RuntimeContextForTest(unsetPageSizeCmd, ChatList)); got != 15 {
		t.Fatalf("unchanged positive page-size = %d, want 15", got)
	}

	helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
		"im/list_all_conversations": `{"result":{"list":[{"openConversationId":"cid","conversationName":"G","singleChat":false}]}}`,
	}})
	var output bytes.Buffer
	chatCmd.SetOut(&output)
	if err := executeChatList(rt); err != nil {
		t.Fatalf("executeChatList success: %v", err)
	}

	helpers.InitDeps(&larkAlignmentCaller{failProductTool: "im/list_all_conversations"})
	if err := executeChatList(rt); err == nil {
		t.Fatal("expected MCP failure from executeChatList")
	}

	badTypesCmd, _, err := root.Find([]string{"chat", "+chat-list"})
	if err != nil {
		t.Fatal(err)
	}
	if err := badTypesCmd.Flags().Set("types", "bad-channel"); err != nil {
		t.Fatal(err)
	}
	_ = badTypesCmd.Flags().Lookup("types").Value.Set("bad-channel")
	badTypesCmd.Flags().Lookup("types").Changed = true
	if err := executeChatList(shortcut.RuntimeContextForTest(badTypesCmd, ChatList)); err == nil {
		t.Fatal("expected invalid types from executeChatList")
	}

	badCursorCmd, _, err := root.Find([]string{"chat", "+chat-list"})
	if err != nil {
		t.Fatal(err)
	}
	if err := badCursorCmd.Flags().Set("page-token", "bad"); err != nil {
		t.Fatal(err)
	}
	badCursorCmd.Flags().Lookup("page-token").Changed = true
	if err := executeChatList(shortcut.RuntimeContextForTest(badCursorCmd, ChatList)); err == nil {
		t.Fatal("expected invalid cursor from executeChatList")
	}

	rows := chatListProject(map[string]any{"result": map[string]any{"list": []any{
		"not-a-map",
		map[string]any{"openConversationId": "cid", "conversationName": "G", "singleChat": false},
	}}})
	if len(rows) != 1 {
		t.Fatalf("non-map skip rows = %#v", rows)
	}
	if got := chatListFilterTypes(rows, []string{"unknown"}); len(got) != 1 {
		t.Fatalf("noop filter = %#v", got)
	}
}

func TestCrossPlatformCoverageChatListCursorAndPageSizeDirect(t *testing.T) {
	root := newPlatformCoverageRoot()
	chatCmd, _, err := root.Find([]string{"chat", "+chat-list"})
	if err != nil {
		t.Fatal(err)
	}
	// Force page-size Int=0 and unchanged so chatListPageSize falls through to return 20.
	if err := chatCmd.Flags().Set("page-size", "0"); err != nil {
		t.Fatal(err)
	}
	chatCmd.Flags().Lookup("page-size").Changed = false
	if got := chatListPageSize(shortcut.RuntimeContextForTest(chatCmd, ChatList)); got != 20 {
		t.Fatalf("page size fallback = %d, want 20", got)
	}
	// Negative cursor alias hits chatListCursor error used by executeChatList.
	cursorCmd, _, err := root.Find([]string{"chat", "+chat-list"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cursorCmd.Flags().Set("cursor", "-1"); err != nil {
		t.Fatal(err)
	}
	cursorCmd.Flags().Lookup("cursor").Changed = true
	if _, err := chatListCursor(shortcut.RuntimeContextForTest(cursorCmd, ChatList)); err == nil {
		t.Fatal("expected negative cursor error")
	}
	if err := executeChatList(shortcut.RuntimeContextForTest(cursorCmd, ChatList)); err == nil {
		t.Fatal("expected executeChatList cursor error")
	}
}

func TestCrossPlatformCoverageChatListPageAllMergesBeforeTypeFilter(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/list_all_conversations": {
			`{"result":{"conversations":[{"openConversationId":"direct-1","title":"单聊","singleChat":true}],"hasMore":true,"nextCursor":2}}`,
			`{"result":{"conversations":[{"openConversationId":"direct-1","title":"重复单聊","singleChat":true},{"openConversationId":"group-1","title":"项目群","singleChat":false}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list", "--types", "group", "--page-size", "1", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[1].args["cursor"] != 2 {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["pagesFetched"] != float64(2) || payload["complete"] != true || payload["hasMore"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	chat := payload["chats"].([]any)[0].(map[string]any)
	if chat["openConversationId"] != "group-1" || chat["conversationType"] != "group" {
		t.Fatalf("chat = %#v", chat)
	}
}

func TestCrossPlatformCoverageChatListProbesMaximumWindowWhenBackendFalselyEndsFullFirstPage(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/list_all_conversations": {
			`{"result":{"conversations":[{"openConversationId":"group-1","title":"项目一群","singleChat":false}],"hasMore":false}}`,
			`{"result":{"conversations":[{"openConversationId":"group-1","title":"项目一群","singleChat":false},{"openConversationId":"group-2","title":"项目二群","singleChat":false}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list", "--page-size", "1", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].args["limit"] != 1 || fake.calls[1].args["limit"] != 100 {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(2) || payload["complete"] != true || payload["paginationKnown"] != false ||
		payload["completionEvidence"] != "max_window_short_page" || payload["stopReason"] != "max_window_short_page" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatListPageLimitPublishesContinuation(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_all_conversations": `{"result":{"conversations":[{"openConversationId":"group-1","title":"项目群","singleChat":false}],"hasMore":true,"nextCursor":9}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list", "--page-all", "--page-limit", "1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["hasMore"] != true || payload["nextCursor"] != float64(9) ||
		payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" || payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatListLaterPageFailureKeepsPartialLedger(t *testing.T) {
	fake := &larkAlignmentCaller{
		sequenceResponses: map[string][]string{
			"im/list_all_conversations": {`{"result":{"conversations":[{"openConversationId":"group-1","title":"项目群","singleChat":false}],"hasMore":true,"nextCursor":2}}`},
		},
		failProductToolAt: map[string]int{"im/list_all_conversations": 2},
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list", "--page-all"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected later-page error")
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["complete"] != false || payload["partial"] != true ||
		payload["failedCount"] != float64(1) || payload["stopReason"] != "read_failure" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatListPaginationValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--page-limit", "2"},
		{"--page-all", "--page-limit", "0"},
		{"--page-all", "--page-limit", "501"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+chat-list"}, args...))
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid args succeeded: %v", args)
		}
	}

	if _, err := chatListNextCursor(float64(1.5)); err == nil {
		t.Fatal("fractional cursor unexpectedly accepted")
	}
	if _, err := chatListNextCursor(struct{}{}); err == nil {
		t.Fatal("unsupported cursor unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageChatListAdditionalPaginationEdges(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{"chat", "+chat-list"}, args...))
		err := root.Execute()
		if output.Len() == 0 {
			return nil, err
		}
		var payload map[string]any
		if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return payload, err
	}

	t.Run("first read failure", func(t *testing.T) {
		_, err := run(t, &larkAlignmentCaller{failProductToolAt: map[string]int{"im/list_all_conversations": 1}})
		if err == nil {
			t.Fatal("first read failure unexpectedly succeeded")
		}
	})

	t.Run("cursor without hasMore continues", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
			"im/list_all_conversations": {
				`{"result":{"conversations":[{"openConversationId":"g1"}],"nextCursor":2}}`,
				`{"result":{"conversations":[],"hasMore":false}}`,
			},
		}}, "--page-all")
		if err != nil || payload["complete"] != true || payload["pagesFetched"] != float64(2) {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("gateway tuple preserves continuation metadata", func(t *testing.T) {
		fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
			"im/list_all_conversations": {
				`{"result":[[{"openConversationId":"g1","singleChat":false}],2,true]}`,
				`{"result":[[{"openConversationId":"g2","singleChat":false}],0,false]}`,
			},
		}}
		payload, err := run(t, fake, "--page-size", "1", "--page-all")
		if err != nil {
			t.Fatalf("tuple pagination payload=%#v calls=%#v err=%v", payload, fake.calls, err)
		}
		if len(fake.calls) != 2 || fake.calls[1].args["cursor"] != 2 {
			t.Fatalf("tuple pagination calls = %#v", fake.calls)
		}
		if payload["count"] != float64(2) || payload["pagesFetched"] != float64(2) ||
			payload["complete"] != true || payload["hasMore"] != false || payload["stopReason"] != "source_complete" {
			t.Fatalf("tuple pagination payload = %#v", payload)
		}
	})

	for name, response := range map[string]string{
		"missing metadata": `{"result":[[{"openConversationId":"g1"}]]}`,
		"invalid hasMore":  `{"result":[[{"openConversationId":"g1"}],2,"true"]}`,
	} {
		t.Run("malformed gateway tuple "+name, func(t *testing.T) {
			payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
				"im/list_all_conversations": response,
			}}, "--page-all")
			if err == nil || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
				t.Fatalf("malformed tuple payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("full legacy page without pagination fails closed", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
			"im/list_all_conversations": `{"result":{"conversations":[{"openConversationId":"g1"}]}}`,
		}}, "--page-size", "1")
		if err == nil || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("single full page is untrusted", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
			"im/list_all_conversations": `{"result":{"conversations":[{"openConversationId":"g1"}],"hasMore":false}}`,
		}}, "--page-size", "1")
		if err != nil || payload["complete"] != false || payload["stopReason"] != "single_page_full_untrusted" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("bounded probe reports page limit", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
			"im/list_all_conversations": `{"result":{"conversations":[{"openConversationId":"g1"}],"hasMore":false}}`,
		}}, "--page-size", "1", "--page-all", "--page-limit", "1")
		if err != nil || payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("full maximum probe fails closed", func(t *testing.T) {
		chats := make([]map[string]any, chatListMaxWindowSize)
		for i := range chats {
			chats[i] = map[string]any{"openConversationId": fmt.Sprintf("probe-%d", i)}
		}
		second, marshalErr := json.Marshal(map[string]any{"result": map[string]any{"conversations": chats, "hasMore": false}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		payload, err := run(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
			"im/list_all_conversations": {
				`{"result":{"conversations":[{"openConversationId":"first"}],"hasMore":false}}`,
				string(second),
			},
		}}, "--page-size", "1", "--page-all")
		if err == nil || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	for _, tc := range []struct {
		name     string
		response string
		wantErr  bool
		stop     string
	}{
		{name: "missing continuation", response: `{"result":{"conversations":[{"openConversationId":"g1"}],"hasMore":true}}`, wantErr: true, stop: "pagination_error"},
		{name: "single page continuation", response: `{"result":{"conversations":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":2}}`, stop: "single_page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{"im/list_all_conversations": tc.response}})
			if (err != nil) != tc.wantErr || payload["stopReason"] != tc.stop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("exclude muted and output errors", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/list_all_conversations": `{"result":{"conversations":[],"hasMore":false}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(chatOutputErrorWriter{err: errors.New("fixture output")})
		root.SetArgs([]string{"chat", "+chat-list", "--exclude-muted"})
		if err := root.Execute(); err == nil || fake.calls[0].args["excludeMuted"] != true {
			t.Fatalf("calls=%#v err=%v", fake.calls, err)
		}
	})
}

func TestCrossPlatformCoverageChatListCursorTypeEdges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		ok    bool
	}{
		{name: "nil", value: nil, ok: true},
		{name: "int", value: int(1), ok: true},
		{name: "negative int", value: int(-1)},
		{name: "int64", value: int64(2), ok: true},
		{name: "negative int64", value: int64(-1)},
		{name: "float64", value: float64(3), ok: true},
		{name: "negative float64", value: float64(-1)},
		{name: "json number", value: json.Number("4"), ok: true},
		{name: "invalid json number", value: json.Number("bad")},
		{name: "empty string", value: "", ok: true},
		{name: "string", value: "5", ok: true},
		{name: "invalid string", value: "bad"},
		{name: "unsupported", value: struct{}{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := chatListNextCursor(tc.value)
			if (err == nil) != tc.ok {
				t.Fatalf("value=%#v err=%v", tc.value, err)
			}
		})
	}
}
