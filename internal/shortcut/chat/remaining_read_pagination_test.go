// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageChatListAllPageAllUsesNumericCursorAndDeduplicates(t *testing.T) {
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"im/list_my_groups_pagination": {
			`{"result":{"groups":[{"openConversationId":"g1","title":"群一"}],"hasMore":true,"nextCursor":88}}`,
			`{"result":{"groups":[{"openConversationId":"g1","title":"重复群"},{"openConversationId":"g2","title":"群二"}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list-all", "--limit", "1", "--page-all", "--page-limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[1].args["cursor"] != float64(88) {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(2) || payload["pagesFetched"] != float64(2) ||
		payload["complete"] != true || payload["hasMore"] != false || payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageDirectMessagesPageAllUsesMillisecondCursor(t *testing.T) {
	const cursorMillis int64 = 1785919699136
	fake := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		"chat/list_individual_chat_message": {
			`{"result":{"messages":[{"openMessageId":"m2","createTime":"2026-08-05 16:48:19"}],"hasMore":true,"nextCursor":1785919699136}}`,
			`{"result":{"messages":[{"openMessageId":"m1","createTime":"2026-08-05 16:48:19"}],"hasMore":false}}`,
		},
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-list-direct", "--open-dingtalk-id", "D-user",
		"--time", "2026-08-05 16:49:00", "--forward=false", "--limit", "1",
		"--page-all", "--page-limit", "5",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	wantBoundary := time.UnixMilli(cursorMillis).UTC().Format(time.RFC3339Nano)
	if len(fake.calls) != 2 || fake.calls[1].args["time"] != wantBoundary {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["count"] != float64(2) || payload["pagesFetched"] != float64(2) ||
		payload["complete"] != true || payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageDirectMessagesPageLimitPublishesExecutableContinuation(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"chat/list_individual_chat_message": `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-08-05 16:49:00",
		"--page-all", "--page-limit", "1",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["hasMore"] != true ||
		payload["truncatedByPageLimit"] != true || payload["stopReason"] != "page_limit" || payload["nextPage"] == nil {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageDirectMessagesPageAllFailsClosedWithoutMillisecondCursor(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"chat/list_individual_chat_message": `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-08-05 16:49:00", "--page-all",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("missing direct-message cursor unexpectedly succeeded")
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageChatListAllLaterReadFailureKeepsPartialLedger(t *testing.T) {
	fake := &larkAlignmentCaller{
		sequenceResponses: map[string][]string{
			"im/list_my_groups_pagination": {`{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":88}}`},
		},
		failProductToolAt: map[string]int{"im/list_my_groups_pagination": 2},
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+chat-list-all", "--page-all"})
	if err := root.Execute(); err == nil {
		t.Fatal("later group-list failure unexpectedly succeeded")
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

func TestCrossPlatformCoverageChatRemainingPaginationValidation(t *testing.T) {
	for _, args := range [][]string{
		{"chat", "+chat-list-all", "--limit", "201"},
		{"chat", "+chat-list-all", "--page-limit", "2"},
		{"chat", "+chat-list-all", "--page-all", "--page-limit", "501"},
		{"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-01-01", "--page-limit", "2"},
		{"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-01-01", "--page-all", "--page-limit", "501"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid args succeeded: %v", args)
		}
	}
}

func TestCrossPlatformCoverageChatListAllAdditionalEdges(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{"chat", "+chat-list-all"}, args...))
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

	t.Run("single page terminal and continuing", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			response string
			stop     string
		}{
			{name: "terminal", response: `{"result":{"groups":[],"hasMore":false}}`, stop: "source_complete"},
			{name: "continuing", response: `{"result":{"groups":[],"hasMore":true,"nextCursor":2}}`, stop: "single_page"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{"im/list_my_groups_pagination": tc.response}})
				if err != nil || payload["stopReason"] != tc.stop || payload["pagesFetched"] != float64(1) {
					t.Fatalf("payload=%#v err=%v", payload, err)
				}
			})
		}
	})

	t.Run("first read failure", func(t *testing.T) {
		for _, args := range [][]string{nil, {"--page-all"}} {
			_, err := run(t, &larkAlignmentCaller{failProductToolAt: map[string]int{"im/list_my_groups_pagination": 1}}, args...)
			if err == nil {
				t.Fatalf("first read failure unexpectedly succeeded for %v", args)
			}
		}
	})

	for _, tc := range []struct {
		name     string
		response string
		args     []string
		wantErr  bool
		stop     string
	}{
		{name: "unknown pagination", response: `{"result":{"groups":[]}}`, args: []string{"--page-all"}, wantErr: true, stop: "pagination_error"},
		{name: "missing continuation", response: `{"result":{"groups":[],"hasMore":true}}`, args: []string{"--page-all"}, wantErr: true, stop: "pagination_error"},
		{name: "page limit", response: `{"result":{"groups":[{"openConversationId":"g1"}],"hasMore":true,"nextCursor":2}}`, args: []string{"--page-all", "--page-limit", "1"}, stop: "page_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{"im/list_my_groups_pagination": tc.response}}, tc.args...)
			if (err != nil) != tc.wantErr || payload["stopReason"] != tc.stop {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("output errors propagate", func(t *testing.T) {
		for _, args := range [][]string{nil, {"--page-all"}} {
			fake := &larkAlignmentCaller{responses: map[string]string{
				"im/list_my_groups_pagination": `{"result":{"groups":[],"hasMore":false}}`,
			}}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetOut(chatOutputErrorWriter{err: errors.New("fixture output")})
			root.SetArgs(append([]string{"chat", "+chat-list-all"}, args...))
			if err := root.Execute(); err == nil {
				t.Fatalf("output error swallowed for args %v", args)
			}
		}
	})

	for _, value := range []any{nil, "", "0", "<nil>"} {
		if got := chatListAllCursorString(value); got != "" {
			t.Fatalf("cursor %v = %q", value, got)
		}
	}
	payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{
		"im/list_my_groups_pagination": `{"result":{"groups":[{"title":"无 ID 群"}],"hasMore":false}}`,
	}}, "--page-all")
	if err != nil || payload["count"] != float64(1) {
		t.Fatalf("missing-id payload=%#v err=%v", payload, err)
	}
}

func TestCrossPlatformCoverageDirectMessagesAdditionalEdges(t *testing.T) {
	run := func(t *testing.T, fake *larkAlignmentCaller, args ...string) (map[string]any, error) {
		t.Helper()
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(append([]string{
			"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-08-05 16:49:00",
		}, args...))
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

	for _, args := range [][]string{
		{"chat", "+messages-list-direct", "--time", "2026-01-01"},
		{"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-01-01", "--limit", "0"},
		{"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-01-01", "--size", "0"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid args succeeded: %v", args)
		}
	}

	t.Run("single page terminal and continuing", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			response string
			stop     string
		}{
			{name: "terminal", response: `{"result":{"messages":[],"hasMore":false}}`, stop: "source_complete"},
			{name: "continuing", response: `{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`, stop: "single_page"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				payload, err := run(t, &larkAlignmentCaller{responses: map[string]string{"chat/list_individual_chat_message": tc.response}})
				if err != nil || payload["stopReason"] != tc.stop {
					t.Fatalf("payload=%#v err=%v", payload, err)
				}
			})
		}
	})

	t.Run("read failures", func(t *testing.T) {
		_, err := run(t, &larkAlignmentCaller{failProductToolAt: map[string]int{"chat/list_individual_chat_message": 1}})
		if err == nil {
			t.Fatal("single-page read failure unexpectedly succeeded")
		}
		_, err = run(t, &larkAlignmentCaller{failProductToolAt: map[string]int{"chat/list_individual_chat_message": 1}}, "--page-all")
		if err == nil {
			t.Fatal("first all-page read failure unexpectedly succeeded")
		}
		payload, err := run(t, &larkAlignmentCaller{
			sequenceResponses: map[string][]string{
				"chat/list_individual_chat_message": {`{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`},
			},
			failProductToolAt: map[string]int{"chat/list_individual_chat_message": 2},
		}, "--page-all")
		if err == nil || payload["partial"] != true || payload["stopReason"] != "read_failure" {
			t.Fatalf("payload=%#v err=%v", payload, err)
		}
	})

	for _, tc := range []struct {
		name      string
		responses []string
	}{
		{name: "unknown pagination", responses: []string{`{"result":{"messages":[{"openMessageId":"m1"}]}}`}},
		{name: "empty continuing page", responses: []string{`{"result":{"messages":[],"hasMore":true,"nextCursor":1785919699136}}`}},
		{name: "stalled cursor and duplicate", responses: []string{
			`{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`,
			`{"result":{"messages":[{"openMessageId":"m1"}],"hasMore":true,"nextCursor":1785919699136}}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := run(t, &larkAlignmentCaller{sequenceResponses: map[string][]string{
				"chat/list_individual_chat_message": tc.responses,
			}}, "--page-all")
			if err == nil || payload["failedCount"] != float64(1) || payload["stopReason"] != "pagination_error" {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}

	t.Run("all-page output errors propagate", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"chat/list_individual_chat_message": `{"result":{"messages":[],"hasMore":false}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetOut(chatOutputErrorWriter{err: errors.New("fixture output")})
		root.SetArgs([]string{"chat", "+messages-list-direct", "--user", "u1", "--time", "2026-01-01", "--page-all"})
		if err := root.Execute(); err == nil {
			t.Fatal("output error was swallowed")
		}
	})

	for _, tc := range []struct {
		name  string
		value any
		ok    bool
	}{
		{name: "int", value: int(1), ok: true},
		{name: "int32", value: int32(2), ok: true},
		{name: "float32", value: float32(3), ok: true},
		{name: "float64", value: float64(4), ok: true},
		{name: "string", value: "5", ok: true},
		{name: "fractional float32", value: float32(1.5)},
		{name: "negative float64", value: float64(-1)},
		{name: "invalid string", value: "invalid"},
		{name: "unsupported", value: true},
		{name: "zero", value: int64(0)},
	} {
		t.Run("cursor "+tc.name, func(t *testing.T) {
			key, boundary, err := directMessageCursorBoundary(tc.value)
			if tc.ok {
				if err != nil || key == "" || boundary == "" {
					t.Fatalf("cursor=(%q,%q,%v)", key, boundary, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("invalid cursor succeeded: (%q,%q)", key, boundary)
			}
		})
	}
}
