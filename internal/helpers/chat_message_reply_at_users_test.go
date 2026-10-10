// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestChatMessageReplyAtUsersFlagAndPayload(t *testing.T) {
	root := newChatCommand()
	command, remaining, err := root.Find([]string{"message", "reply"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find message reply: remaining=%v err=%v", remaining, err)
	}
	flag := command.Flags().Lookup("at-users")
	if flag == nil || flag.Value.Type() != "string" {
		t.Fatalf("at-users flag = %#v, want string", flag)
	}

	tests := []struct {
		name          string
		atUsers       string
		text          string
		responses     []string
		wantOpenIDs   []string
		wantText      string
		wantCallCount int
	}{
		{
			name:          "open dingtalk ids",
			atUsers:       "DBAAAAAAAAAAiE, DCAAAAAAAAAAiE",
			text:          "@DBAAAAAAAAAAiE <@DCAAAAAAAAAAiE> 收到",
			wantOpenIDs:   []string{"DBAAAAAAAAAAiE", "DCAAAAAAAAAAiE"},
			wantText:      "<@DBAAAAAAAAAAiE> <@DCAAAAAAAAAAiE> 收到",
			wantCallCount: 1,
		},
		{
			name:    "user ids resolve to open dingtalk ids",
			atUsers: "u1, u2",
			text:    "<@u1> @u2 收到",
			responses: []string{
				`{"result":[{"userId":"u1","openDingTalkId":"DBAAAAAAAAAAiE"},{"userId":"u2","openDingTalkId":"DCAAAAAAAAAAiE"}]}`,
				`{}`,
			},
			wantOpenIDs:   []string{"DBAAAAAAAAAAiE", "DCAAAAAAAAAAiE"},
			wantText:      "<@DBAAAAAAAAAAiE> <@DCAAAAAAAAAAiE> 收到",
			wantCallCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{responses: tt.responses}
			_, _, err := executeWukongWeeklySyncCommand(
				t,
				"chat",
				caller,
				newChatCommand,
				"message", "reply",
				"--conversation-id", "cid",
				"--ref-msg-id", "mid",
				"--ref-sender", "DAAAAAAAAAAAiE",
				"--text", tt.text,
				"--at-users", tt.atUsers,
			)
			if err != nil {
				t.Fatalf("reply returned error: %v", err)
			}
			if len(caller.calls) != tt.wantCallCount {
				t.Fatalf("calls = %#v, want %d", caller.calls, tt.wantCallCount)
			}
			call := caller.calls[len(caller.calls)-1]
			if call.server != "chat" || call.tool != "send_personal_message" {
				t.Fatalf("target = %s/%s, want chat/send_personal_message", call.server, call.tool)
			}
			if got := call.args["atOpenDingTalkIds"]; !reflect.DeepEqual(got, tt.wantOpenIDs) {
				t.Fatalf("atOpenDingTalkIds = %#v, want %#v", got, tt.wantOpenIDs)
			}
			if _, exists := call.args["atUserIds"]; exists {
				t.Fatalf("unsupported atUserIds unexpectedly forwarded: %#v", call.args)
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(call.args["content"].(string)), &content); err != nil {
				t.Fatalf("reply content is invalid JSON: %v", err)
			}
			if content["content"] != tt.wantText {
				t.Fatalf("reply text = %q, want %q", content["content"], tt.wantText)
			}
		})
	}
}

func TestChatMessageReplyWithoutAtUsersOmitsMentionPayload(t *testing.T) {
	caller := &wukongWeeklySyncCaller{}
	_, _, err := executeWukongWeeklySyncCommand(
		t,
		"chat",
		caller,
		newChatCommand,
		"message", "reply",
		"--conversation-id", "cid",
		"--ref-msg-id", "mid",
		"--ref-sender", "DAAAAAAAAAAAiE",
		"--text", "收到",
	)
	if err != nil {
		t.Fatalf("reply returned error: %v", err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one send call", caller.calls)
	}
	if _, exists := caller.calls[0].args["atOpenDingTalkIds"]; exists {
		t.Fatalf("atOpenDingTalkIds unexpectedly present: %#v", caller.calls[0].args)
	}
}

func TestChatMessageReplyAtUsersResolutionFailureIsReported(t *testing.T) {
	// The directory lookup succeeds but returns no mapping for the userId, so
	// resolution fails and the reply must not be sent with an unresolved @.
	caller := &wukongWeeklySyncCaller{responses: []string{`{"result":[]}`}}
	_, _, err := executeWukongWeeklySyncCommand(
		t,
		"chat",
		caller,
		newChatCommand,
		"message", "reply",
		"--conversation-id", "cid",
		"--ref-msg-id", "mid",
		"--ref-sender", "DAAAAAAAAAAAiE",
		"--text", "@u1 收到",
		"--at-users", "u1",
	)
	if err == nil {
		t.Fatalf("expected an error when --at-users cannot be resolved")
	}
	if !strings.Contains(err.Error(), "cannot resolve --at-users") {
		t.Fatalf("error = %v, want it to mention cannot resolve --at-users", err)
	}
	for _, call := range caller.calls {
		if call.tool == "send_personal_message" {
			t.Fatalf("reply was sent despite unresolved --at-users: %#v", call.args)
		}
	}
}
