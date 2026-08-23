// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageSafeResourceQueryDownloadsStayReadOnly(t *testing.T) {
	for _, command := range []shortcut.Shortcut{AtMe, ChatMessages, SearchMsg, ThreadReplies} {
		if command.Risk != shortcut.RiskRead {
			t.Errorf("%s risk contract = %q", command.Command, command.Risk)
		}
	}
}

func TestCrossPlatformCoverageAtMeEmptyResultKeepsMessagesAndItemsIterable(t *testing.T) {
	caller := &platformCoverageCaller{}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+at-me", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	for _, key := range []string{"messages", "items"} {
		rows, ok := payload[key].([]any)
		if !ok || len(rows) != 0 {
			t.Fatalf("%s = %#v, want empty array", key, payload[key])
		}
	}
}

func TestCrossPlatformCoverageMessageReadShortcutsPublishResourceDownloadPlans(t *testing.T) {
	message := `{"openMessageId":"msg","openConversationId":"cid","content":"{\"mediaId\":\"@image\"}","quotedMessage":{"openMessageId":"quoted","content":"{\"fileId\":\"@quoted-file\"}"}}`
	tests := []struct {
		name      string
		tool      string
		response  string
		args      []string
		resultKey string
	}{
		{
			name:      "chat messages",
			tool:      "chat/list_conversation_message_v2",
			response:  `{"result":{"messages":[` + message + `]}}`,
			args:      []string{"chat", "+chat-messages", "--conversation-id", "cid"},
			resultKey: "messages",
		},
		{
			name:      "search",
			tool:      "im/search_messages",
			response:  `{"result":{"messages":[` + message + `],"hasMore":false}}`,
			args:      []string{"chat", "+search-msg", "--query", "x", "--no-enrich"},
			resultKey: "messages",
		},
		{
			name:      "at me",
			tool:      "chat/search_at_me_message",
			response:  `{"result":{"conversationMessagesList":[{"openConversationId":"cid","messages":[` + message + `]}]}}`,
			args:      []string{"chat", "+at-me"},
			resultKey: "messages",
		},
		{
			name:      "thread replies",
			tool:      "chat/list_topic_replies",
			response:  `{"result":{"messages":[` + message + `]}}`,
			args:      []string{"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread"},
			resultKey: "replies",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartCoverageCaller{responses: map[string][]string{
				tc.tool: {tc.response},
			}}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			var output bytes.Buffer
			root.SetOut(&output)
			args := append([]string{}, tc.args...)
			args = append(args, "--download-resources", "--output-dir", "./downloads", "--dry-run")
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if caller.counts[tc.tool] != 1 {
				t.Fatalf("lower calls = %#v", caller.counts)
			}
			var payload map[string]any
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("decode output: %v\n%s", err, output.String())
			}
			rows, ok := payload[tc.resultKey].([]any)
			if !ok || len(rows) != 1 {
				t.Fatalf("payload missing %s: %#v", tc.resultKey, payload)
			}
			row, _ := rows[0].(map[string]any)
			resources, _ := row["resourceRefs"].([]any)
			if len(resources) != 2 {
				t.Fatalf("visible resources = %#v", resources)
			}
			ledger, _ := payload["resourceDownloads"].(map[string]any)
			if ledger["dryRun"] != true ||
				ledger["discoveredCount"] != float64(2) ||
				ledger["requestedCount"] != float64(2) {
				t.Fatalf("resource plan = %#v", ledger)
			}
		})
	}
}

func TestCrossPlatformCoverageMessageReadShortcutResourceOutputValidation(t *testing.T) {
	for _, args := range [][]string{
		{"chat", "+chat-messages", "--conversation-id", "cid"},
		{"chat", "+search-msg", "--query", "x", "--no-enrich"},
		{"chat", "+at-me"},
		{"chat", "+thread-replies", "--group", "cid", "--thread-id", "thread"},
	} {
		helpers.InitDeps(&smartCoverageCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append(args, "--download-resources", "--output-dir", "../outside", "--yes"))
		if err := root.Execute(); err == nil {
			t.Fatalf("unsafe output accepted: %v", args)
		}
	}
}

func TestCrossPlatformCoverageChatMessagesDefaultsToRecentHistory(t *testing.T) {
	caller := &platformCoverageCaller{}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	before := time.Now().Add(-2 * time.Second)
	root.SetArgs([]string{"chat", "+chat-messages", "--conversation-id", "cid", "--limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	after := time.Now().Add(2 * time.Second)
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v", caller.calls)
	}
	call := caller.calls[0]
	if call.args["forward"] != false {
		t.Fatalf("default forward = %#v, want false", call.args["forward"])
	}
	boundary, err := time.ParseInLocation(
		"2006-01-02 15:04:05",
		call.args["time"].(string),
		dingTalkMessageLocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if boundary.Before(before) || boundary.After(after) {
		t.Fatalf("default time = %s, want current boundary", boundary)
	}
}

func TestCrossPlatformCoverageFormatDingTalkMessageBoundaryDoesNotDependOnProcessTimezone(t *testing.T) {
	now := time.Date(2026, time.July, 29, 1, 2, 3, 0, time.UTC)
	if got := formatDingTalkMessageBoundary(now); got != "2026-07-29 09:02:03" {
		t.Fatalf("UTC process boundary = %q, want DingTalk UTC+8 wall time", got)
	}
}

func TestCrossPlatformCoverageChatMessagesPreservesExplicitTime(t *testing.T) {
	caller := &platformCoverageCaller{}
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-messages",
		"--conversation-id", "cid",
		"--time", "2026-07-01 12:34:56",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 ||
		caller.calls[0].args["time"] != "2026-07-01 12:34:56" {
		t.Fatalf("explicit time call = %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageSearchMsgFlattensRealGroupedResponse(t *testing.T) {
	original := map[string]any{"openMessageId": "m1"}
	items := searchMsgItems(map[string]any{
		"result": map[string]any{
			"conversationMessagesList": []any{
				"invalid-group",
				map[string]any{"messages": "invalid"},
				map[string]any{
					"openConversationId": "cid",
					"title":              "会话",
					"singleChat":         true,
					"messages": []any{
						"invalid-message",
						original,
						map[string]any{
							"openMessageId":      "m2",
							"openConversationId": "own-cid",
							"conversationTitle":  "own-title",
							"singleChat":         false,
						},
					},
				},
			},
		},
	})
	if len(items) != 2 {
		t.Fatalf("grouped items = %#v", items)
	}
	if items[0]["openConversationId"] != "cid" || items[0]["conversationTitle"] != "会话" ||
		items[0]["singleChat"] != true {
		t.Fatalf("injected group context = %#v", items[0])
	}
	if items[1]["openConversationId"] != "own-cid" || items[1]["conversationTitle"] != "own-title" ||
		items[1]["singleChat"] != false {
		t.Fatalf("message context was overwritten: %#v", items[1])
	}
	if _, mutated := original["openConversationId"]; mutated {
		t.Fatalf("source message mutated: %#v", original)
	}
	if searchMsgItems(map[string]any{"result": "invalid"}) != nil {
		t.Fatal("non-map child was accepted")
	}
}
