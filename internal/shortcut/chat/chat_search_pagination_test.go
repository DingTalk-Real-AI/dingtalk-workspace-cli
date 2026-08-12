// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageChatSearchDryRunStopsBeforeRead(t *testing.T) {
	fake := &larkAlignmentCaller{dryRun: true}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-search", "--query", "项目", "--page-size", "20",
		"--cursor", "cursor-1", "--exclude-muted", "--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("chat-search dry-run made lower calls: %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageChatSearchPageAllUsesOpaqueCursorAndDeduplicates(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/search_groups": {
			`{"result":{"groups":[{"openConversationId":"cid-1","title":"项目一群"}],"hasMore":true,"nextCursor":"cursor-2"}}`,
			`{"result":{"groups":[{"openConversationId":"cid-1","title":"重复项目群"},{"openConversationId":"cid-2","title":"项目二群"}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--page-size", "1", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].args["cursor"] != "0" || fake.calls[1].args["cursor"] != "cursor-2" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(2) || payload["pagesFetched"] != float64(2) || payload["complete"] != true || payload["hasMore"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	chats := payload["chats"].([]any)
	if chats[0].(map[string]any)["name"] != "项目一群" || chats[1].(map[string]any)["openConversationId"] != "cid-2" {
		t.Fatalf("chats = %#v", chats)
	}
}

func TestCrossPlatformCoverageChatSearchProbesMaximumWindowWhenBackendFalselyEndsFullFirstPage(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/search_groups": {
			`{"result":{"groups":[{"openConversationId":"cid-1","title":"项目一群"}],"hasMore":false}}`,
			`{"result":{"groups":[{"openConversationId":"cid-1","title":"项目一群"},{"openConversationId":"cid-2","title":"项目二群"}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--page-size", "1", "--page-all", "--page-limit", "5"})
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

func TestCrossPlatformCoverageChatSearchPageTokenAndPageLimit(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/search_groups": `{"result":{"groups":[{"openConversationId":"cid-2","title":"项目二群"}],"hasMore":true,"nextCursor":"cursor-3"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--page-token", "cursor-2", "--page-all", "--page-limit", "1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].args["cursor"] != "cursor-2" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["hasMore"] != true || payload["nextCursor"] != "cursor-3" ||
		payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatSearchFailureModes(t *testing.T) {
	t.Run("later read failure keeps partial result", func(t *testing.T) {
		fake := &larkAlignmentCaller{
			sequenceResponses: map[string][]string{
				"im/search_groups": {`{"result":{"groups":[{"openConversationId":"cid-1","title":"项目群"}],"hasMore":true,"nextCursor":"cursor-2"}}`},
			},
			failProductToolAt: map[string]int{"im/search_groups": 2},
		}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--page-all"})
		if err := root.Execute(); err == nil {
			t.Fatal("expected later-page error")
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["count"] != float64(1) || payload["partial"] != true || payload["stopReason"] != "read_failure" {
			t.Fatalf("payload = %#v", payload)
		}
	})

	t.Run("full legacy page without pagination fails closed", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/search_groups": `{"result":{"groups":[{"openConversationId":"cid-1","title":"项目群"}]}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(&bytes.Buffer{})
		root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--page-size", "1", "--page-all"})
		if err := root.Execute(); err == nil {
			t.Fatal("missing pagination unexpectedly succeeded")
		}
	})

	t.Run("short legacy page is bounded complete", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{"im/search_groups": `{"result":[]}`}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{"chat", "+chat-search", "--query", "不存在"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["complete"] != true || payload["paginationKnown"] != false || payload["stopReason"] != "legacy_short_page" {
			t.Fatalf("payload = %#v", payload)
		}
	})
}

func TestCrossPlatformCoverageChatSearchPaginationValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--page-size", "0"},
		{"--page-limit", "2"},
		{"--page-all", "--page-limit", "0"},
		{"--page-all", "--page-limit", "501"},
		{"--cursor", "c1", "--page-token", "c2"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+chat-search", "--query", "项目"}, args...))
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid args succeeded: %v", args)
		}
	}
}

func TestCrossPlatformCoverageChatSearchAdditionalPaginationEdges(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{"chat", "+chat-search", "--query", "项目"}, args...))
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
		_, err := run(t, &larkAlignmentCaller{failProductToolAt: map[string]int{"im/search_groups": 1}})
		if err == nil {
			t.Fatal("first read failure unexpectedly succeeded")
		}
	})

	t.Run("cursor without hasMore continues", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
			"im/search_groups": {
				`{"result":{"groups":[{"openConversationId":"g1"}],"nextCursor":"next"}}`,
				`{"result":{"groups":[],"hasMore":false}}`,
			},
		}}, "--page-all")
		if err != nil || payload["complete"] != true || payload["pagesFetched"] != float64(2) {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("single full page is untrusted", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
			"im/search_groups": `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":false}}`,
		}}, "--page-size", "1")
		if err != nil || payload["complete"] != false || payload["stopReason"] != "single_page_full_untrusted" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("bounded probe reports page limit", func(t *testing.T) {
		payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
			"im/search_groups": `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":false}}`,
		}}, "--page-size", "1", "--page-all", "--page-limit", "1")
		if err != nil || payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	t.Run("full maximum probe fails closed", func(t *testing.T) {
		groups := make([]map[string]any, chatSearchMaxWindowSize)
		for i := range groups {
			groups[i] = map[string]any{"openConversationId": fmt.Sprintf("probe-%d", i)}
		}
		second, marshalErr := json.Marshal(map[string]any{"result": map[string]any{"groups": groups, "hasMore": false}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		payload, err := run(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
			"im/search_groups": {
				`{"result":{"groups":[{"openConversationId":"first"}],"hasMore":false}}`,
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
		args     []string
		wantErr  bool
		stop     string
	}{
		{name: "missing continuation", response: `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true}}`, wantErr: true, stop: "pagination_error"},
		{name: "single page continuation", response: `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":"next"}}`, stop: "single_page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{"im/search_groups": tc.response}, failProductToolAt: map[string]int{}}, tc.args...)
			if (err != nil) != tc.wantErr || payload["stopReason"] != tc.stop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("exclude muted reaches lower call and output errors propagate", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/search_groups": `{"result":{"groups":[],"hasMore":false}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(chatOutputErrorWriter{err: errors.New("fixture output")})
		root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--exclude-muted"})
		if err := root.Execute(); err == nil || fake.calls[0].args["excludeMuted"] != true {
			t.Fatalf("calls=%#v err=%v", fake.calls, err)
		}
	})
}

func TestCrossPlatformCoverageChatSearchProjectionEdges(t *testing.T) {
	if got := chatSearchItems(nil); len(got) != 0 {
		t.Fatalf("nil items = %#v", got)
	}
	if got := chatSearchItems(map[string]any{"result": map[string]any{"unknown": true}}); len(got) != 0 {
		t.Fatalf("unknown items = %#v", got)
	}
	got := projectChatSearchItems([]any{
		"invalid",
		map[string]any{"conversationId": "g1", "conversationName": "项目群"},
	})
	if len(got) != 1 || got[0]["openConversationId"] != "g1" || got[0]["name"] != "项目群" {
		t.Fatalf("projected = %#v", got)
	}
	for _, value := range []any{nil, "", "0", "<nil>"} {
		if cursor := chatSearchCursorString(value); cursor != "" {
			t.Fatalf("cursor %v = %q", value, cursor)
		}
	}

	helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
		"im/search_groups": `{"result":{"groups":[],"hasMore":false}}`,
	}})
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"chat", "+chat-search", "--query", "项目", "--cursor="})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
