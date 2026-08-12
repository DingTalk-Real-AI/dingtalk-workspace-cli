// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageAtMePageAllUsesOpaqueCursorAndDeduplicates(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"conversationMessagesList":[{"openConversationId":"cid","title":"群","messages":[{"openMessageId":"m2"},{"openMessageId":"m1"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`,
		`{"result":{"conversationMessagesList":[{"openConversationId":"cid","title":"群","messages":[{"openMessageId":"m1"},{"openMessageId":"m0"}]}],"hasMore":false}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+at-me", "--limit", "2", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.args) != 2 || caller.args[0]["cursor"] != "0" || caller.args[1]["cursor"] != "cursor-2" {
		t.Fatalf("calls = %#v", caller.args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(3) || payload["pagesFetched"] != float64(2) ||
		payload["complete"] != true || payload["hasMore"] != false || payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload["items"].([]any)) != 3 {
		t.Fatalf("compatibility items = %#v", payload["items"])
	}
}

func TestCrossPlatformCoverageAtMePageAllFailsClosedWithoutContinuation(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"conversationMessagesList":[{"messages":[{"openMessageId":"m1"}]}],"hasMore":true}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+at-me", "--page-all"})
	if err := root.Execute(); err == nil {
		t.Fatal("missing @me continuation unexpectedly succeeded")
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageAtMePageAllContinuesAcrossEmptyIntermediatePage(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":"cursor-2"}}`,
		`{"result":{"conversationMessagesList":[{"messages":[{"openMessageId":"m1"}]}],"hasMore":false}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+at-me", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["pagesFetched"] != float64(2) || payload["complete"] != true {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageAtMeMaxItemsPublishesStableTruncation(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"conversationMessagesList":[{"messages":[{"openMessageId":"m1"},{"openMessageId":"m2"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+at-me", "--page-all", "--max-items", "1", "--page-delay", "0"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["truncated"] != true ||
		payload["truncatedByResultLimit"] != true || payload["stopReason"] != "result_limit" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMyGroupsPageAllUsesNumericCursorAndFiltersAfterMerge(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"groups":[{"openConversationId":"g1","title":"群一","groupType":"group"}],"hasMore":true,"nextCursor":88}}`,
		`{"result":{"groups":[{"openConversationId":"g1","title":"重复群","groupType":"group"},{"openConversationId":"g2","title":"单聊","groupType":"p2p"}],"hasMore":false}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+my-groups", "--type", "group", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.args) != 2 || caller.args[1]["cursor"] != float64(88) {
		t.Fatalf("calls = %#v", caller.args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["pagesFetched"] != float64(2) ||
		payload["complete"] != true || payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMyGroupsMaxItemsAppliesAfterTypeFilter(t *testing.T) {
	caller := &chatMessagesPagingCaller{responses: []string{
		`{"result":{"groups":[{"openConversationId":"p1","groupType":"p2p"},{"openConversationId":"g1","groupType":"group"},{"openConversationId":"g2","groupType":"group"}],"hasMore":false}}`,
	}}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+my-groups", "--type", "group", "--page-all", "--max-items", "1", "--page-delay", "0"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(1) || payload["truncated"] != true ||
		payload["truncatedByResultLimit"] != true || payload["stopReason"] != "result_limit" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageRemainingReadPaginationValidation(t *testing.T) {
	for _, args := range [][]string{
		{"chat", "+at-me", "--page-limit", "2"},
		{"chat", "+at-me", "--max-items", "1"},
		{"chat", "+at-me", "--page-delay", "1"},
		{"chat", "+at-me", "--page-all", "--page-limit", "501"},
		{"chat", "+at-me", "--page-all", "--max-items", "-1"},
		{"chat", "+my-groups", "--limit", "201"},
		{"chat", "+my-groups", "--page-limit", "2"},
		{"chat", "+my-groups", "--max-items", "1"},
		{"chat", "+my-groups", "--page-all", "--page-limit", "501"},
	} {
		helpers.InitDeps(&chatMessagesPagingCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid args succeeded: %v", args)
		}
	}
}

func TestCrossPlatformCoverageMyGroupsAdditionalEdges(t *testing.T) {
	run := func(t *testing.T, caller *chatMessagesPagingCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(caller)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{"chat", "+my-groups"}, args...))
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

	t.Run("single-page outcomes", func(t *testing.T) {
		payload, err := run(t, &chatMessagesPagingCaller{responses: []string{
			`{"result":{"groups":[],"hasMore":false}}`,
		}}, "--cursor", "cursor-1")
		if err != nil || payload["complete"] != true || payload["stopReason"] != "source_complete" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
		payload, err = run(t, &chatMessagesPagingCaller{responses: []string{
			`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":2}}`,
		}})
		if err != nil || payload["complete"] != false || payload["stopReason"] != "single_page" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
		if _, err = run(t, &chatMessagesPagingCaller{failAt: 1}); err == nil {
			t.Fatal("single-page read failure unexpectedly succeeded")
		}
	})

	t.Run("all-page failures", func(t *testing.T) {
		if _, err := run(t, &chatMessagesPagingCaller{failAt: 1}, "--page-all"); err == nil {
			t.Fatal("first-page failure unexpectedly succeeded")
		}
		payload, err := run(t, &chatMessagesPagingCaller{
			responses: []string{`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":2}}`},
			failAt:    2,
		}, "--page-all")
		if err == nil || payload["partial"] != true || payload["stopReason"] != "read_failure" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	for _, tc := range []struct {
		name      string
		responses []string
		args      []string
		stop      string
		wantErr   bool
	}{
		{name: "unknown pagination", responses: []string{`{"result":{"groups":[]}}`}, args: []string{"--page-all"}, stop: "pagination_error", wantErr: true},
		{name: "missing continuation", responses: []string{`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true}}`}, args: []string{"--page-all"}, stop: "pagination_error", wantErr: true},
		{name: "stalled continuation", responses: []string{
			`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":2}}`,
			`{"result":{"groups":[{"openConversationId":"g2"}],"hasMore":true,"nextCursor":2}}`,
		}, args: []string{"--page-all"}, stop: "pagination_error", wantErr: true},
		{name: "page limit", responses: []string{`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":2}}`}, args: []string{"--page-all", "--page-limit", "1"}, stop: "page_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &chatMessagesPagingCaller{responses: tc.responses}, tc.args...)
			if (err != nil) != tc.wantErr || payload["stopReason"] != tc.stop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("output failures", func(t *testing.T) {
		helpers.InitDeps(&chatMessagesPagingCaller{responses: []string{`{"result":{"groups":[],"hasMore":false}}`}})
		for _, args := range [][]string{{"chat", "+my-groups"}, {"chat", "+my-groups", "--page-all"}} {
			root := newPlatformCoverageRoot()
			root.SetOut(chatMessagesFailWriter{})
			root.SetArgs(args)
			if err := root.Execute(); err == nil {
				t.Fatalf("output failure swallowed for %v", args)
			}
		}
	})

	for _, value := range []any{nil, "", "0", "<nil>", 0} {
		if got := myGroupsCursorString(value); got != "" {
			t.Fatalf("cursor %v = %q", value, got)
		}
	}
}

func TestCrossPlatformCoverageAtMeAdditionalEdges(t *testing.T) {
	run := func(t *testing.T, caller *chatMessagesPagingCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(caller)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{"chat", "+at-me"}, args...))
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

	t.Run("single page terminal and read failure", func(t *testing.T) {
		payload, err := run(t, &chatMessagesPagingCaller{responses: []string{
			`{"result":{"conversationMessagesList":[],"hasMore":false}}`,
		}})
		if err != nil || payload["complete"] != true || payload["stopReason"] != "source_complete" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
		_, err = run(t, &chatMessagesPagingCaller{failAt: 1})
		if err == nil {
			t.Fatal("single-page read failure unexpectedly succeeded")
		}
	})

	t.Run("all-page read failures", func(t *testing.T) {
		_, err := run(t, &chatMessagesPagingCaller{failAt: 1}, "--page-all")
		if err == nil {
			t.Fatal("first all-page read failure unexpectedly succeeded")
		}
		payload, err := run(t, &chatMessagesPagingCaller{
			responses: []string{`{"result":{"conversationMessagesList":[{"messages":[{"openMessageId":"m1"}]}],"hasMore":true,"nextCursor":"next"}}`},
			failAt:    2,
		}, "--page-all")
		if err == nil || payload["partial"] != true || payload["stopReason"] != "read_failure" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	for _, tc := range []struct {
		name      string
		responses []string
		args      []string
		wantErr   bool
		stop      string
	}{
		{name: "unknown pagination", responses: []string{`{"result":{"conversationMessagesList":[]}}`}, args: []string{"--page-all"}, wantErr: true, stop: "pagination_error"},
		{name: "page limit", responses: []string{`{"result":{"conversationMessagesList":[{"messages":[{"openMessageId":"m1"}]}],"hasMore":true,"nextCursor":"next"}}`}, args: []string{"--page-all", "--page-limit", "1"}, stop: "page_limit"},
		{name: "empty cursor defaults", responses: []string{`{"result":{"conversationMessagesList":[],"hasMore":false}}`}, args: []string{"--cursor=", "--page-all"}, stop: "source_complete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &chatMessagesPagingCaller{responses: tc.responses}, tc.args...)
			if (err != nil) != tc.wantErr || payload["stopReason"] != tc.stop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			if tc.stop == "page_limit" && payload["nextCursor"] != "next" {
				t.Fatalf("continuation payload=%#v", payload)
			}
		})
	}

	t.Run("output errors propagate", func(t *testing.T) {
		helpers.InitDeps(&chatMessagesPagingCaller{responses: []string{
			`{"result":{"conversationMessagesList":[],"hasMore":false}}`,
		}})
		root := newPlatformCoverageRoot()
		root.SetOut(chatMessagesFailWriter{})
		root.SetArgs([]string{"chat", "+at-me"})
		if err := root.Execute(); err == nil {
			t.Fatal("output error was swallowed")
		}
	})

	for _, value := range []any{nil, "", "0", "<nil>"} {
		if got := atMeCursorString(value); got != "" {
			t.Fatalf("cursor %v = %q", value, got)
		}
	}
}
