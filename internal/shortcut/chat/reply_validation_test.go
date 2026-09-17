// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageReplyResolvesAndChecksSourceForEveryQuote(t *testing.T) {
	valid := `{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"` + fixtureCurrentDOpenID + `"}`
	for _, tc := range []struct {
		name, items string
		flags       []string
		fail        bool
	}{
		{"resolve without group", valid, nil, false},
		{"matching explicit sender", valid, []string{"--ref-sender", fixtureCurrentDOpenID}, false},
		{"wrong group", valid, []string{"--group", "other"}, true},
		{"explicit sender cannot bypass wrong group", valid, []string{"--group", "other", "--ref-sender", fixtureCurrentDOpenID}, true},
		{"wrong explicit sender", valid, []string{"--ref-sender", fixtureCurrentDOpenID2}, true},
		{"missing id", `{"openConversationId":"cid","senderOpenDingTalkId":"` + fixtureCurrentDOpenID + `"}`, nil, true},
		{"missing conversation", `{"openMessageId":"msg","senderOpenDingTalkId":"` + fixtureCurrentDOpenID + `"}`, nil, true},
		{"duplicate id", valid + "," + valid, nil, true},
		{"foreign row ignored", `{"openMessageId":"other","openConversationId":"other","senderOpenDingTalkId":"D-other"},` + valid, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":[` + tc.items + `]}`}}
			args := []string{"+messages-reply", "--message-id", "msg", "--text", "reply", "--dry-run"}
			args = append(args, tc.flags...)
			out, err := runChatParity(t, f, args...)
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v output=%s", err, out)
			}
			if len(f.calls) != 1 || f.calls[0].tool != "list_messages_by_ids" {
				t.Fatalf("preflight must read once and never write: %#v", f.calls)
			}
			if tc.fail {
				return
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result["conversationId"] != "cid" || result["willSend"] != false {
				t.Fatal(result)
			}
		})
	}
}

func TestCrossPlatformCoverageReplyMentionRoutingAndRejection(t *testing.T) {
	for _, mode := range []string{"group", "thread", "bot"} {
		t.Run(mode, func(t *testing.T) {
			source := map[string]any{"openMessageId": "msg", "openConversationId": "cid", "senderOpenDingTalkId": fixtureCurrentDOpenID}
			flags := []string{}
			if mode == "thread" {
				source["openConvThreadId"] = "thread"
				flags = append(flags, "--reply-in-thread")
			}
			if mode == "bot" {
				flags = append(flags, "--as", "bot", "--robot-code", "r")
			}
			data, _ := json.Marshal(map[string]any{"result": []any{source}})
			f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": string(data), "chat/get_conversation_info": `{"success":true,"result":{"conversationInfo":{"openConversationId":"cid","convThreadEnabled":false}}}`}}
			args := []string{"+messages-reply", "--message-id", "msg", "--content", "hello", "--at-all", "--at-open-dingtalk-ids", fixtureCurrentDOpenID + "," + fixtureCurrentDOpenID, "--dry-run"}
			args = append(args, flags...)
			out, err := runChatParity(t, f, args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range f.calls {
				if strings.Contains(call.tool, "send") {
					t.Fatal("dry-run wrote")
				}
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			a := result["arguments"].(map[string]any)
			if mode == "bot" {
				if a["isAtAll"] != "true" || len(a["atOpendingtalkIds"].([]any)) != 1 || !strings.Contains(a["markdown"].(string), "@"+fixtureCurrentDOpenID) {
					t.Fatal(a)
				}
			} else {
				if a["atAll"] != true || len(a["atOpenDingTalkIds"].([]any)) != 1 || !strings.Contains(a["content"].(string), fixtureCurrentDOpenID) {
					t.Fatal(a)
				}
				if mode == "thread" && a["openConversationId"] != "thread" {
					t.Fatal(a)
				}
			}
		})
	}
	for _, flags := range [][]string{
		{"--open-dingtalk-id", fixtureCurrentDOpenID, "--at-all"},
		{"--at-open-dingtalk-ids", "not-an-open-id"},
		{"--text", "<@" + fixtureCurrentDOpenID + ">"},
		{"--text", "<@all>"},
		{"--content", "   "},
	} {
		f := &larkAlignmentCaller{}
		args := append([]string{"+messages-reply", "--message-id", "msg", "--content", "hello", "--dry-run"}, flags...)
		if _, err := runChatParity(t, f, args...); err == nil || len(f.calls) > 0 {
			t.Fatalf("invalid mention input %v reached reads: %v %#v", flags, err, f.calls)
		}
	}
}
