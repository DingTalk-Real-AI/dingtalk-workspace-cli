// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func runChatParity(t *testing.T, fake *larkAlignmentCaller, args ...string) (string, error) {
	t.Helper()
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"chat"}, args...))
	executed, err := root.ExecuteC()
	if err == nil && output.UsesUnifiedResult(executed) {
		code, _, emitErr := output.EmitStoredResult(executed)
		if emitErr != nil {
			err = emitErr
		} else if code != 0 {
			err = fmt.Errorf("command result exit %d", code)
		}
	}
	return out.String(), err
}

func TestCrossPlatformCoverageChatParityFixedFeedActions(t *testing.T) {
	for _, name := range []string{"+feed-shortcut-create", "+feed-shortcut-remove"} {
		t.Run(name, func(t *testing.T) {
			f := &larkAlignmentCaller{}
			_, err := runChatParity(t, f, name, "--chat-ids", "cid,cid,other", "--yes")
			if err != nil {
				t.Fatal(err)
			}
			if len(f.calls) != 2 {
				t.Fatalf("calls=%#v", f.calls)
			}
			for _, call := range f.calls {
				if call.tool != "set_top_conversation" || call.args["top"] != (name == "+feed-shortcut-create") {
					t.Fatalf("wrong action %#v", call)
				}
			}
			f = &larkAlignmentCaller{}
			_, err = runChatParity(t, f, name, "--conversation-id", "cid", "--off", "--yes")
			if err == nil || len(f.calls) != 0 {
				t.Fatalf("off must not reverse action: %v %#v", err, f.calls)
			}
			f = &larkAlignmentCaller{dryRun: true}
			out, err := runChatParity(t, f, name, "--conversation-id", "cid", "--dry-run")
			if err != nil || len(f.calls) != 0 || !strings.Contains(out, `"executed": false`) {
				t.Fatalf("preview: %v %s %#v", err, out, f.calls)
			}
		})
	}
	f := &larkAlignmentCaller{failProductToolAt: map[string]int{"im/set_top_conversation": 1}}
	out, err := runChatParity(t, f, "+feed-shortcut-remove", "--conversation-ids", "cid,other", "--yes")
	if err == nil || len(f.calls) != 2 || !strings.Contains(out, `"outcome": "partial_failure"`) || !strings.Contains(out, `"unknown"`) {
		t.Fatalf("partial failure lost: %v %s", err, out)
	}
}

func TestCrossPlatformCoverageChatParityBotImageAndShare(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		tool, field string
		value       any
	}{
		{"bot image", []string{"--as", "bot", "--robot-code", "robot", "--groups", "cid,other", "--image-url", "https://example.com/image.png"}, "send_robot_group_message", "msgKey", "sampleImageMsg"},
		{"bot direct image", []string{"--as", "bot", "--robot-code", "robot", "--users", "u1,u2", "--image-url", "https://example.com/image.png"}, "batch_send_robot_msg_to_users", "msgType", "sampleImageMsg"},
		{"user profile", []string{"--group", "cid", "--contact-id", "DAAAAAAAAAAAiE"}, "send_personal_message", "msgType", "profile"},
		{"user group invitation", []string{"--group", "cid", "--share-chat-id", "source", "--expires-seconds", "3600", "--uuid", "key"}, "share_group_invite_url", "sourceOpenConversationId", "source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{}
			args := append([]string{"+messages-send"}, tc.args...)
			args = append(args, "--yes")
			_, err := runChatParity(t, f, args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.calls) == 0 {
				t.Fatal("no send")
			}
			for _, call := range f.calls {
				if call.tool != tc.tool || call.args[tc.field] != tc.value {
					t.Fatalf("wrong route %#v", call)
				}
			}
		})
	}
	for _, args := range [][]string{
		{"--as", "webhook", "--webhook-token", "token", "--image-url", "https://example.com/i.png"},
		{"--as", "user", "--group", "cid", "--image-url", "https://example.com/i.png"},
		{"--as", "bot", "--robot-code", "r", "--group", "cid", "--media-id", "media"},
		{"--as", "bot", "--robot-code", "r", "--group", "cid", "--image-url", "file:///etc/passwd"},
		{"--as", "bot", "--robot-code", "r", "--group", "cid", "--image-url", "https://example.com/i.png", "--at-all"},
		{"--group", "cid", "--text", "text", "--expires-seconds", "5"},
	} {
		f := &larkAlignmentCaller{}
		argv := append([]string{"+messages-send"}, args...)
		argv = append(argv, "--yes")
		if _, err := runChatParity(t, f, argv...); err == nil || len(f.calls) != 0 {
			t.Fatalf("invalid mode wrote: %v %#v", err, f.calls)
		}
	}
}

func TestCrossPlatformCoverageChatParityBotLocalFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("file.txt", []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	uploads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploads++
		body, _ := io.ReadAll(r.Body)
		if string(body) != "payload" {
			t.Errorf("body %q", body)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	responses := map[string]string{"im/init_conversation_file_upload": `{"resourceUrl":"` + server.URL + `","uploadKey":"upload"}`, "im/commit_conversation_file_upload": `{"result":{"downloadUrl":"https://example.com/download"}}`}
	f := &larkAlignmentCaller{responses: responses}
	_, err := runChatParity(t, f, "+messages-send", "--as", "bot", "--robot-code", "r", "--group", "cid", "--file", "file.txt", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if uploads != 1 || len(f.calls) != 3 || f.calls[2].args["fileUrl"] != "https://example.com/download" || f.calls[2].args["msgKey"] != "sampleDingtalkDriveFile" {
		t.Fatalf("upload flow %#v", f.calls)
	}
	f = &larkAlignmentCaller{dryRun: true}
	out, err := runChatParity(t, f, "+messages-send", "--as", "bot", "--robot-code", "r", "--users", "u1", "--file", "file.txt", "--dry-run")
	if err != nil || len(f.calls) != 0 || uploads != 1 || !strings.Contains(out, `"executed": false`) {
		t.Fatalf("dry run uploaded: %v %s", err, out)
	}
	f = &larkAlignmentCaller{}
	_, err = runChatParity(t, f, "+messages-send", "--as", "bot", "--robot-code", "r", "--users", "u1,u2", "--file", "file.txt", "--yes")
	if err == nil || len(f.calls) != 0 {
		t.Fatal("multi-recipient local file must fail before upload")
	}
	f = &larkAlignmentCaller{responses: responses, failProductTool: "im/commit_conversation_file_upload"}
	_, err = runChatParity(t, f, "+messages-send", "--as", "bot", "--robot-code", "r", "--group", "cid", "--file", "file.txt", "--yes")
	if err == nil || len(f.calls) != 2 {
		t.Fatalf("must not send after failed commit %v %#v", err, f.calls)
	}
}

func TestCrossPlatformCoverageChatParityReplyRouting(t *testing.T) {
	base := []string{"+messages-reply", "--group", "cid", "--message-id", "msg", "--content", "reply", "--yes"}
	t.Run("bot quote", func(t *testing.T) {
		f := &larkAlignmentCaller{responses: map[string]string{"chat/get_conversation_info": `{"success":true,"result":{"conversationInfo":{"openConversationId":"cid","convThreadEnabled":false}}}`}}
		_, err := runChatParity(t, f, append(base, "--as", "bot", "--robot-code", "r")...)
		if err != nil {
			t.Fatal(err)
		}
		last := f.calls[len(f.calls)-1]
		if last.product != "bot" || last.args["srcMsgSendOpenDingTalkId"] != "D-inferred" || last.args["referenceOpenMessageId"] != "msg" {
			t.Fatalf("route %#v", last)
		}
	})
	t.Run("thread target from source", func(t *testing.T) {
		f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": `{"result":[{"openMessageId":"msg","openConversationId":"cid","openConvThreadId":"thread","senderOpenDingTalkId":"D-author"}]}`}}
		_, err := runChatParity(t, f, append(base, "--reply-in-thread")...)
		if err != nil {
			t.Fatal(err)
		}
		last := f.calls[len(f.calls)-1]
		if last.args["openConversationId"] != "thread" || last.args["msgType"] != "markdown" {
			t.Fatalf("must target child thread %#v", last)
		}
	})
	t.Run("direct recipient checked", func(t *testing.T) {
		f := &larkAlignmentCaller{responses: map[string]string{"chat/get_conversation_info": `{"result":{"openConversationId":"cid"}}`}}
		_, err := runChatParity(t, f, append(base, "--open-dingtalk-id", "DAAAAAAAAAAAiE")...)
		if err != nil {
			t.Fatal(err)
		}
		last := f.calls[len(f.calls)-1]
		if last.args["receiverOpenDingTalkId"] != "DAAAAAAAAAAAiE" || last.args["openConversationId"] != nil {
			t.Fatalf("direct payload %#v", last)
		}
	})
	for _, tc := range []struct {
		name, response string
		flags          []string
	}{
		{"wrong conversation", `{"result":[{"openMessageId":"msg","openConversationId":"other","senderOpenDingTalkId":"D-author"}]}`, []string{"--as", "bot", "--robot-code", "r"}},
		{"wrong message", `{"result":[{"openMessageId":"other","openConversationId":"cid","senderOpenDingTalkId":"D-author"}]}`, []string{"--as", "bot", "--robot-code", "r"}},
		{"wrong sender", `{"result":[{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"D-author"}]}`, []string{"--as", "bot", "--robot-code", "r", "--ref-sender", "D-other"}},
		{"missing thread", `{"result":[{"openMessageId":"msg","openConversationId":"cid"}]}`, []string{"--reply-in-thread"}},
		{"wrong thread", `{"result":[{"openMessageId":"msg","openConversationId":"cid","openConvThreadId":"thread"}]}`, []string{"--reply-in-thread", "--thread-id", "other"}},
		{"bot thread", `{"result":[{"openMessageId":"msg","openConversationId":"cid","openConvThreadId":"thread","senderOpenDingTalkId":"D-author"}]}`, []string{"--as", "bot", "--robot-code", "r"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: map[string]string{"im/list_messages_by_ids": tc.response}}
			_, err := runChatParity(t, f, append(base, tc.flags...)...)
			if err == nil {
				t.Fatal("expected failure")
			}
			for _, call := range f.calls {
				if strings.Contains(call.tool, "send") {
					t.Fatalf("invalid source wrote %#v", call)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageChatParityEditAndCreateBots(t *testing.T) {
	f := &larkAlignmentCaller{}
	out, err := runChatParity(t, f, "+messages-edit", "--chat-id", "cid", "--message-id", "msg", "--markdown", "new", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[1].tool != "edit_message" || !strings.Contains(out, "not_performed") {
		t.Fatalf("edit %#v %s", f.calls, out)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(f.calls[1].args["content"].(string)), &content); err != nil || content["text"] != "new" {
		t.Fatalf("content %#v %v", content, err)
	}
	for _, args := range [][]string{
		{"--as", "bot", "--text", "body"}, {"--content", `{"post":[]}`}, {"--content", `{"text":"x","unknown":"bad"}`},
	} {
		f = &larkAlignmentCaller{}
		argv := append([]string{"+messages-edit", "--group", "cid", "--message-id", "msg", "--yes"}, args...)
		if _, err := runChatParity(t, f, argv...); err == nil || len(f.calls) != 0 {
			t.Fatalf("invalid edit wrote %v %#v", err, f.calls)
		}
	}
	f = &larkAlignmentCaller{failProductToolAt: map[string]int{"bot/add_robot_to_group": 1}}
	out, err = runChatParity(t, f, "+chat-create", "--name", "test", "--users", "u1", "--bots", "r1,r2", "--yes")
	if err == nil || !strings.Contains(out, `"failedBotCount": 1`) || !strings.Contains(out, "open-cid") {
		t.Fatalf("partial create lost context %v %s", err, out)
	}
	added := 0
	for _, call := range f.calls {
		if call.tool == "add_robot_to_group" {
			added++
			if call.args["openConversationId"] != "open-cid" {
				t.Fatalf("wrong group %#v", call)
			}
		}
		if call.tool == "dismiss_group" {
			t.Fatal("must not rollback created group")
		}
	}
	if added != 2 {
		t.Fatalf("add calls %d", added)
	}
}

func TestCrossPlatformCoverageChatParityA2UICreate(t *testing.T) {
	f := &larkAlignmentCaller{}
	_, err := runChatParity(t, f, "+messages-send", "--group", "cid", "--a2ui-messages", `["{\"version\":\"v1.0\"}"]`, "--request-id", "request", "--biz-card-id", "card", "--card-summary", "summary", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0].tool != "create_and_send_a2ui_card" {
		t.Fatalf("calls %#v", f.calls)
	}
	args := f.calls[0].args
	if args["requestId"] != "request" || args["bizCardId"] != "card" || args["summary"] != "summary" || args["flowStatus"] != "PROCESSING" {
		t.Fatalf("card args %#v", args)
	}
	for _, extra := range [][]string{{"--uuid", "key"}, {"--as", "bot", "--robot-code", "r"}} {
		f = &larkAlignmentCaller{}
		argv := append([]string{"+messages-send", "--group", "cid", "--a2ui-messages", `["{}"]`, "--yes"}, extra...)
		if _, err := runChatParity(t, f, argv...); err == nil || len(f.calls) != 0 {
			t.Fatalf("unsupported card mode wrote: %v %#v", err, f.calls)
		}
	}
}

func TestCrossPlatformCoverageChatParityUnknownWritesNeverBecomeSuccess(t *testing.T) {
	for _, tc := range []struct {
		command      string
		args         []string
		responseTool string
	}{
		{"+feed-shortcut-remove", []string{"--conversation-id", "cid"}, "im/set_top_conversation"},
		{"+messages-edit", []string{"--group", "cid", "--message-id", "msg", "--text", "new"}, "im/edit_message"},
	} {
		f := &larkAlignmentCaller{responses: map[string]string{tc.responseTool: `{"result":{}}`}}
		argv := append([]string{tc.command}, tc.args...)
		argv = append(argv, "--yes")
		out, err := runChatParity(t, f, argv...)
		if err == nil || !strings.Contains(out, `"outcome": "failure"`) {
			t.Fatalf("empty write response advertised success: %v %s", err, out)
		}
		writes := 0
		for _, call := range f.calls {
			if call.product+"/"+call.tool == tc.responseTool {
				writes++
			}
		}
		if writes != 1 {
			t.Fatalf("unexpected retry: %#v", f.calls)
		}
	}
}

func TestCrossPlatformCoverageChatParityLarkRepeatedChatIDKeepsAllTargets(t *testing.T) {
	f := &larkAlignmentCaller{}
	_, err := runChatParity(t, f, "+feed-shortcut-remove", "--chat-id", "one,two", "--chat-id", "three", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("repeated/comma-separated Lark chat-id lost targets: %#v", f.calls)
	}
	for i, want := range []string{"one", "two", "three"} {
		if f.calls[i].args["openConversationId"] != want || f.calls[i].args["top"] != false {
			t.Fatalf("wrong target %#v", f.calls[i])
		}
	}
}
