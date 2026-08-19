// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatTopicCall struct {
	product string
	tool    string
	args    map[string]any
}

type chatTopicCaller struct {
	calls     []chatTopicCall
	responses map[string]string
	errors    map[string]error
	dryRun    bool
}

func (c *chatTopicCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, chatTopicCall{product: product, tool: tool, args: args})
	if err := c.errors[product+"/"+tool]; err != nil {
		return nil, err
	}
	text := c.responses[product+"/"+tool]
	if text == "" {
		text = `{"success":true}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*chatTopicCaller) Format() string { return "json" }
func (c *chatTopicCaller) DryRun() bool { return c.dryRun }
func (*chatTopicCaller) Fields() string { return "" }
func (*chatTopicCaller) JQ() string     { return "" }

func executeAtomicTopicCommand(t *testing.T, caller *chatTopicCaller, args ...string) error {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	ctx, _ := output.WithResultStore(context.Background())
	executed, err := root.ExecuteContextC(ctx)
	if err != nil {
		return err
	}
	_, _, err = output.EmitStoredResult(executed)
	return err
}

func executeAtomicTopicDryRun(t *testing.T, caller *chatTopicCaller, args ...string) ([]byte, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
	var stdout, stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr
	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	ctx, _ := output.WithResultStore(context.Background())
	executed, err := root.ExecuteContextC(ctx)
	if err != nil {
		return stdout.Bytes(), err
	}
	_, emitted, err := output.EmitStoredResult(executed)
	if err == nil && !emitted {
		err = errors.New("unified dry-run returned without a CommandResult")
	}
	return stdout.Bytes(), err
}

func TestCrossPlatformCoverageAtomicTopicDryRunStoresOneResult(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "create", args: []string{"topic", "create", "--name", "话题圈", "--users", "user-1"}},
		{name: "send", args: []string{"topic", "send", "--open-topic-id", "topic-1", "--text", "新话题"}},
		{name: "list", args: []string{"topic", "list", "--open-topic-id", "topic-1"}},
		{name: "reply", args: []string{"topic", "reply", "--open-conv-thread-id", "thread-1", "--text", "回复"}},
		{name: "list-replies", args: []string{"topic", "list-replies", "--open-topic-id", "topic-1", "--open-conv-thread-id", "thread-1"}},
		{name: "forward", args: []string{"topic", "forward", "--src-msg-id", "message-1", "--src-open-topic-id", "topic-1", "--src-open-conv-thread-id", "thread-1", "--dest-open-conversation-id", "conversation-2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &chatTopicCaller{dryRun: true}
			stdout, err := executeAtomicTopicDryRun(t, caller, test.args...)
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout, &envelope); err != nil {
				t.Fatalf("dry-run output is not one JSON result: %q: %v", stdout, err)
			}
			if envelope["dry_run"] != true || envelope["outcome"] != "success" {
				t.Fatalf("dry-run envelope = %#v", envelope)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("dry-run calls = %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageAtomicTopicListsPublishPaginationInMeta(t *testing.T) {
	for _, test := range []struct {
		name      string
		response  map[string]string
		args      []string
		wantItems float64
	}{
		{
			name: "topics",
			response: map[string]string{
				"chat/list_conversation_message_v2": `{"result":{"messages":[{"openMessageId":"root-1","openConvThreadId":"thread-1"}],"hasMore":true,"nextCursor":1787000000123}}`,
			},
			args:      []string{"topic", "list", "--open-topic-id", "topic-1"},
			wantItems: 1,
		},
		{
			name: "topics filtered empty page",
			response: map[string]string{
				"chat/list_conversation_message_v2": `{"result":{"messages":[{"openMessageId":"ordinary-1"}],"hasMore":true,"nextCursor":1787000000123}}`,
			},
			args:      []string{"topic", "list", "--open-topic-id", "topic-1"},
			wantItems: 0,
		},
		{
			name: "replies",
			response: map[string]string{
				"chat/list_topic_replies": `{"result":{"messages":[{"openMessageId":"reply-1"}],"hasMore":true,"nextCursor":1787000000123}}`,
			},
			args:      []string{"topic", "list-replies", "--open-topic-id", "topic-1", "--open-conv-thread-id", "thread-1"},
			wantItems: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, err := executeAtomicTopicDryRun(t, &chatTopicCaller{responses: test.response}, test.args...)
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout, &envelope); err != nil {
				t.Fatal(err)
			}
			data, _ := envelope["data"].(map[string]any)
			for _, key := range []string{"hasMore", "nextCursor", "cursor", "nextPage", "complete"} {
				if _, exists := data[key]; exists {
					t.Fatalf("pagination field %q leaked into data: %#v", key, data)
				}
			}
			meta, _ := envelope["meta"].(map[string]any)
			pagination, _ := meta["pagination"].(map[string]any)
			gotItems, _ := pagination["items"].(float64)
			if pagination["endpoint_exhausted"] != false || pagination["next_token"] == "" || pagination["pages"] != float64(1) || gotItems != test.wantItems {
				t.Fatalf("pagination = %#v", pagination)
			}
		})
	}
}

func TestCrossPlatformCoverageAtomicTopicRejectsNonJSONWithoutRawOutput(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses map[string]string
		args      []string
	}{
		{
			name: "create",
			responses: map[string]string{
				"contact/get_current_user_profile": `{"result":{"userId":"owner-1"}}`,
				"im/create_group_conversation":     `<html>bad gateway</html>`,
			},
			args: []string{"topic", "create", "--name", "话题圈", "--users", "user-1"},
		},
		{
			name:      "list",
			responses: map[string]string{"chat/list_conversation_message_v2": `<html>bad gateway</html>`},
			args:      []string{"topic", "list", "--open-topic-id", "topic-1"},
		},
		{
			name:      "list replies",
			responses: map[string]string{"chat/list_topic_replies": `<html>bad gateway</html>`},
			args:      []string{"topic", "list-replies", "--open-topic-id", "topic-1", "--open-conv-thread-id", "thread-1"},
		},
		{
			name:      "forward",
			responses: map[string]string{"im/forward_topic": `<html>bad gateway</html>`},
			args:      []string{"topic", "forward", "--src-msg-id", "message-1", "--src-open-topic-id", "topic-1", "--src-open-conv-thread-id", "thread-1", "--dest-open-conversation-id", "conversation-2"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, err := executeAtomicTopicDryRun(t, &chatTopicCaller{responses: test.responses}, test.args...)
			var typed *apperrors.Error
			if err == nil || !errors.As(err, &typed) {
				t.Fatalf("error = %v, want structured response validation error", err)
			}
			if typed.FailureStage != "response_validation" || typed.Reason != "topic_response_invalid" {
				t.Fatalf("error = %#v", typed)
			}
			if len(stdout) != 0 {
				t.Fatalf("stdout = %q, want no raw response", stdout)
			}
		})
	}
}

func TestCrossPlatformCoverageChatTopicSurfaceAndLegacyVisibility(t *testing.T) {
	root := newChatCommand()
	topic, remaining, err := root.Find([]string{"topic"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("find chat topic: command=%v remaining=%v error=%v", topic, remaining, err)
	}
	visible := map[string]bool{}
	for _, command := range topic.Commands() {
		if !command.Hidden {
			visible[command.Name()] = true
		}
	}
	want := map[string]bool{"create": true, "send": true, "list": true, "reply": true, "list-replies": true, "forward": true}
	if !reflect.DeepEqual(visible, want) {
		t.Fatalf("visible topic commands = %#v, want %#v", visible, want)
	}
	for _, path := range [][]string{{"message", "list-topic-replies"}, {"message", "forward-topic"}} {
		command, _, findErr := root.Find(path)
		if findErr != nil || !command.Hidden || !command.Runnable() {
			t.Fatalf("legacy path %v: command=%v hidden=%v runnable=%v error=%v", path, command, command != nil && command.Hidden, command != nil && command.Runnable(), findErr)
		}
	}
	create, _, err := root.Find([]string{"group", "create"})
	if err != nil || create.Flags().Lookup("thread") == nil || !create.Flags().Lookup("thread").Hidden {
		t.Fatalf("legacy --thread flag is not hidden: command=%v error=%v", create, err)
	}
	for _, paths := range []struct {
		legacy []string
		topic  []string
	}{
		{legacy: []string{"message", "list"}, topic: []string{"topic", "list"}},
		{legacy: []string{"message", "list-topic-replies"}, topic: []string{"topic", "list-replies"}},
	} {
		legacy, _, legacyErr := root.Find(paths.legacy)
		topicCommand, _, topicErr := root.Find(paths.topic)
		if legacyErr != nil || topicErr != nil {
			t.Fatalf("find time-compatible commands: legacy=%v topic=%v", legacyErr, topicErr)
		}
		for _, name := range []string{"time", "direction"} {
			if legacy.Flags().Lookup(name).Usage != topicCommand.Flags().Lookup(name).Usage {
				t.Fatalf("%v --%s help = %q, want legacy %q", paths.topic, name, topicCommand.Flags().Lookup(name).Usage, legacy.Flags().Lookup(name).Usage)
			}
		}
	}
	legacySend, _, legacySendErr := root.Find([]string{"message", "send"})
	if legacySendErr != nil {
		t.Fatalf("find legacy message send: %v", legacySendErr)
	}
	for _, path := range [][]string{{"topic", "send"}, {"topic", "reply"}} {
		topicSend, _, topicSendErr := root.Find(path)
		if topicSendErr != nil {
			t.Fatalf("find %v: %v", path, topicSendErr)
		}
		for _, name := range []string{"content", "file", "at-all", "at-open-dingtalk-ids"} {
			if legacySend.Flags().Lookup(name).Usage != topicSend.Flags().Lookup(name).Usage {
				t.Fatalf("%v --%s help = %q, want legacy %q", path, name, topicSend.Flags().Lookup(name).Usage, legacySend.Flags().Lookup(name).Usage)
			}
		}
		for _, alias := range []string{"text", "body", "message", "markdown", "file-path", "uuid"} {
			flag := topicSend.Flags().Lookup(alias)
			if flag == nil || !flag.Hidden {
				t.Fatalf("%v --%s compatibility alias = %#v, want hidden", path, alias, flag)
			}
		}
	}
}

func TestCrossPlatformCoverageAtomicTopicReplyUsesDirectThreadTarget(t *testing.T) {
	caller := &chatTopicCaller{}
	err := executeAtomicTopicCommand(t, caller,
		"topic", "reply", "--open-conv-thread-id", "thread-1", "--text", "收到")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %#v, want one write", caller.calls)
	}
	call := caller.calls[0]
	if call.product != "" || call.tool != "send_personal_message" || call.args["openConversationId"] != "thread-1" {
		t.Fatalf("reply call = %#v", call)
	}
	if call.args["referenceOpenMessageId"] != nil || call.args["quotedMessage"] != nil {
		t.Fatalf("topic reply carried quote fields: %#v", call.args)
	}
}

func TestCrossPlatformCoverageAtomicTopicCompatibilityMappings(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		responses := map[string]string{
			"contact/get_current_user_profile": `{"result":{"userId":"owner-1"}}`,
			"im/create_group_conversation":     `{"result":{"openCid":"topic-1"}}`,
		}
		legacy := &chatTopicCaller{responses: responses}
		if err := executeAtomicTopicCommand(t, legacy,
			"group", "create", "--name", "话题圈", "--users", "user-1", "--thread"); err != nil {
			t.Fatal(err)
		}
		topic := &chatTopicCaller{responses: responses}
		if err := executeAtomicTopicCommand(t, topic,
			"topic", "create", "--name", "话题圈", "--users", "user-1"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 2 || len(topic.calls) != 2 ||
			legacy.calls[1].product != topic.calls[1].product || legacy.calls[1].tool != topic.calls[1].tool ||
			!reflect.DeepEqual(legacy.calls[1].args, topic.calls[1].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
	})

	t.Run("send", func(t *testing.T) {
		legacy := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "send", "--conversation-id", "topic-1", "--text", "新话题", "--idempotency-key", "send-1"); err != nil {
			t.Fatal(err)
		}
		topic := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, topic,
			"topic", "send", "--open-topic-id", "topic-1", "--text", "新话题", "--idempotency-key", "send-1"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || len(topic.calls) != 1 ||
			legacy.calls[0].product != topic.calls[0].product || legacy.calls[0].tool != topic.calls[0].tool ||
			!reflect.DeepEqual(legacy.calls[0].args, topic.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
	})

	for _, test := range []struct {
		name       string
		topicPath  string
		targetFlag string
		target     string
	}{
		{name: "send mentions", topicPath: "send", targetFlag: "--open-topic-id", target: "topic-1"},
		{name: "reply mentions", topicPath: "reply", targetFlag: "--open-conv-thread-id", target: "thread-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			legacy := &chatTopicCaller{}
			if err := executeAtomicTopicCommand(t, legacy,
				"message", "send", "--conversation-id", test.target, "--content", "通知 <@user-open-id> <@all>",
				"--at-open-dingtalk-ids", "user-open-id", "--at-all"); err != nil {
				t.Fatal(err)
			}
			topic := &chatTopicCaller{}
			if err := executeAtomicTopicCommand(t, topic,
				"topic", test.topicPath, test.targetFlag, test.target, "--content", "通知 <@user-open-id> <@all>",
				"--at-open-dingtalk-ids", "user-open-id", "--at-all"); err != nil {
				t.Fatal(err)
			}
			if len(legacy.calls) != 1 || len(topic.calls) != 1 ||
				legacy.calls[0].product != topic.calls[0].product || legacy.calls[0].tool != topic.calls[0].tool ||
				!reflect.DeepEqual(legacy.calls[0].args, topic.calls[0].args) {
				t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
			}
		})
	}

	t.Run("list", func(t *testing.T) {
		responses := map[string]string{
			"chat/list_conversation_message_v2": `{"result":{"messages":[],"hasMore":false}}`,
		}
		legacy := &chatTopicCaller{responses: responses}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "list", "--conversation-id", "topic-1", "--time", "2026-08-18 10:00:00", "--direction", "older", "--limit", "20"); err != nil {
			t.Fatal(err)
		}
		topic := &chatTopicCaller{responses: responses}
		if err := executeAtomicTopicCommand(t, topic,
			"topic", "list", "--open-topic-id", "topic-1", "--time", "2026-08-18 10:00:00", "--direction", "older", "--limit", "20"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || len(topic.calls) != 1 ||
			legacy.calls[0].product != topic.calls[0].product || legacy.calls[0].tool != topic.calls[0].tool ||
			!reflect.DeepEqual(legacy.calls[0].args, topic.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
	})

	t.Run("list default time", func(t *testing.T) {
		responses := map[string]string{
			"chat/list_conversation_message_v2": `{"result":{"messages":[],"hasMore":false}}`,
		}
		legacy := &chatTopicCaller{responses: responses}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "list", "--conversation-id", "topic-1"); err != nil {
			t.Fatal(err)
		}
		topic := &chatTopicCaller{responses: responses}
		if err := executeAtomicTopicCommand(t, topic,
			"topic", "list", "--open-topic-id", "topic-1"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || len(topic.calls) != 1 {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
		legacyTime, legacyErr := parseISOTimeToMillis("time", legacy.calls[0].args["time"].(string))
		topicTime, topicErr := parseISOTimeToMillis("time", topic.calls[0].args["time"].(string))
		if legacyErr != nil || topicErr != nil || legacyTime-topicTime > 5000 || topicTime-legacyTime > 5000 {
			t.Fatalf("default times differ: legacy=%#v topic=%#v errors=(%v, %v)", legacy.calls[0].args["time"], topic.calls[0].args["time"], legacyErr, topicErr)
		}
		legacy.calls[0].args["time"] = "<default-time>"
		topic.calls[0].args["time"] = "<default-time>"
		if legacy.calls[0].product != topic.calls[0].product || legacy.calls[0].tool != topic.calls[0].tool ||
			!reflect.DeepEqual(legacy.calls[0].args, topic.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
	})

	t.Run("reply", func(t *testing.T) {
		legacy := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "send", "--conversation-id", "thread-1", "--text", "直接回复", "--idempotency-key", "reply-1"); err != nil {
			t.Fatal(err)
		}
		topic := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, topic,
			"topic", "reply", "--open-conv-thread-id", "thread-1", "--text", "直接回复", "--idempotency-key", "reply-1"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || len(topic.calls) != 1 ||
			legacy.calls[0].product != topic.calls[0].product || legacy.calls[0].tool != topic.calls[0].tool ||
			!reflect.DeepEqual(legacy.calls[0].args, topic.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
	})

	t.Run("reply preuploaded file", func(t *testing.T) {
		legacy := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "send", "--conversation-id", "thread-1", "--msg-type", "file",
			"--dentry-id", "101", "--space-id", "202", "--file-name", "fixture.txt",
			"--file-type", "txt", "--file-size", "12", "--file", "/fixture.txt"); err != nil {
			t.Fatal(err)
		}
		topic := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, topic,
			"topic", "reply", "--open-conv-thread-id", "thread-1", "--msg-type", "file",
			"--dentry-id", "101", "--space-id", "202", "--file-name", "fixture.txt",
			"--file-type", "txt", "--file-size", "12", "--file", "/fixture.txt"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || len(topic.calls) != 1 ||
			legacy.calls[0].product != topic.calls[0].product || legacy.calls[0].tool != topic.calls[0].tool ||
			!reflect.DeepEqual(legacy.calls[0].args, topic.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, topic.calls)
		}
	})

	t.Run("list replies", func(t *testing.T) {
		caller := &chatTopicCaller{responses: map[string]string{
			"chat/list_topic_replies": `{"result":{"messages":[],"hasMore":false}}`,
		}}
		err := executeAtomicTopicCommand(t, caller,
			"topic", "list-replies", "--open-topic-id", "topic-1", "--open-conv-thread-id", "thread-1", "--time", "2026-08-18 10:00:00", "--direction", "newer", "--limit", "20")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"openconversationId": "topic-1", "topicId": "thread-1", "startTime": "2026-08-18 10:00:00", "forward": true, "pageSize": 20}
		if len(caller.calls) != 1 || caller.calls[0].product != "chat" || caller.calls[0].tool != "list_topic_replies" || !reflect.DeepEqual(caller.calls[0].args, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
		legacy := &chatTopicCaller{responses: map[string]string{
			"chat/list_topic_replies": `{"result":{"messages":[],"hasMore":false}}`,
		}}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "list-topic-replies", "--conversation-id", "topic-1", "--topic-id", "thread-1", "--time", "2026-08-18 10:00:00", "--direction", "newer", "--limit", "20"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || !reflect.DeepEqual(legacy.calls[0].args, caller.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, caller.calls)
		}
	})

	t.Run("forward", func(t *testing.T) {
		caller := &chatTopicCaller{}
		err := executeAtomicTopicCommand(t, caller,
			"topic", "forward", "--src-msg-id", "message-1", "--src-open-topic-id", "topic-1", "--src-open-conv-thread-id", "thread-1", "--dest-open-conversation-id", "conversation-2")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"srcOpenMessageId": "message-1", "srcOpenConversationId": "topic-1",
			"srcOpenConvThreadId": "thread-1", "destOpenConversationId": "conversation-2",
		}
		if len(caller.calls) != 1 || caller.calls[0].product != "im" || caller.calls[0].tool != "forward_topic" || !reflect.DeepEqual(caller.calls[0].args, want) {
			t.Fatalf("calls = %#v, want %#v", caller.calls, want)
		}
		legacy := &chatTopicCaller{}
		if err := executeAtomicTopicCommand(t, legacy,
			"message", "forward-topic", "--src-msg-id", "message-1", "--src-conversation-id", "topic-1", "--src-thread-id", "thread-1", "--dest-conversation-id", "conversation-2"); err != nil {
			t.Fatal(err)
		}
		if len(legacy.calls) != 1 || !reflect.DeepEqual(legacy.calls[0].args, caller.calls[0].args) {
			t.Fatalf("legacy=%#v topic=%#v", legacy.calls, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageAtomicTopicQuoteReplyIsRejectedBeforeWrite(t *testing.T) {
	caller := &chatTopicCaller{responses: map[string]string{
		"im/list_messages_by_ids": `{"result":{"messages":[{"openMessageId":"root-1","openConvThreadId":"thread-1"}]}}`,
	}}
	err := executeAtomicTopicCommand(t, caller,
		"message", "reply", "--conversation-id", "topic-1", "--ref-msg-id", "root-1",
		"--ref-sender", "DAAAAAAAAAAAiE", "--text", "错误引用")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "topic_quote_reply_disabled" {
		t.Fatalf("error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_messages_by_ids" {
		t.Fatalf("quote guard reached write: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAtomicTopicBotQuoteReplyIsRejectedBeforeWrite(t *testing.T) {
	caller := &chatTopicCaller{responses: map[string]string{
		"im/list_messages_by_ids": `{"result":{"messages":[{"openMessageId":"root-1","openConvThreadId":"thread-1"}]}}`,
	}}
	err := executeAtomicTopicCommand(t, caller,
		"message", "send-by-bot", "--robot-code", "robot-1", "--conversation-id", "topic-1",
		"--reply", "root-1", "--ref-sender", "DAAAAAAAAAAAiE", "--text", "错误引用")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "topic_quote_reply_disabled" {
		t.Fatalf("error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_messages_by_ids" {
		t.Fatalf("bot quote guard reached write: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAtomicTopicQuoteGuardFailsClosedWhenConversationLookupFails(t *testing.T) {
	caller := &chatTopicCaller{errors: map[string]error{
		"im/list_messages_by_ids":    errors.New("message lookup unavailable"),
		"chat/get_conversation_info": errors.New("conversation lookup unavailable"),
	}}
	err := executeAtomicTopicCommand(t, caller,
		"message", "reply", "--conversation-id", "topic-1", "--ref-msg-id", "root-1",
		"--ref-sender", "DAAAAAAAAAAAiE", "--text", "错误引用")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "topic_quote_guard_unavailable" {
		t.Fatalf("error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "list_messages_by_ids" {
		t.Fatalf("quote guard reached write: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageAtomicTopicQuoteGuardFailsClosedWithoutConversationState(t *testing.T) {
	caller := &chatTopicCaller{responses: map[string]string{
		"im/list_messages_by_ids":    `{"result":{"messages":[{"openMessageId":"message-1","openConversationId":"cid"}]}}`,
		"chat/get_conversation_info": `{"result":{"openConversationId":"cid"}}`,
	}}
	err := executeAtomicTopicCommand(t, caller,
		"message", "reply", "--conversation-id", "cid", "--ref-msg-id", "message-1",
		"--ref-sender", "DAAAAAAAAAAAiE", "--text", "普通引用")
	var typed *apperrors.Error
	if err == nil || !errors.As(err, &typed) || typed.Reason != "topic_quote_guard_unavailable" {
		t.Fatalf("error = %v", err)
	}
	if len(caller.calls) != 2 || caller.calls[1].tool != "get_conversation_info" {
		t.Fatalf("quote guard reached write: %#v", caller.calls)
	}
}
