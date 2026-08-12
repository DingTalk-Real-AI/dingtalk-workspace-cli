// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type larkAlignmentCall struct {
	product string
	tool    string
	args    map[string]any
}

type larkAlignmentCaller struct {
	calls             []larkAlignmentCall
	dryRun            bool
	failTarget        string
	failProductTool   string
	failProductToolAt map[string]int
	callCounts        map[string]int
	category          string
	responses         map[string]string
	sequenceResponses map[string][]string
}

func (f *larkAlignmentCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	f.calls = append(f.calls, larkAlignmentCall{product: product, tool: tool, args: args})
	if f.failTarget != "" && args["openMessageId"] == f.failTarget {
		return nil, errors.New("fixture write failed")
	}
	key := product + "/" + tool
	if f.callCounts == nil {
		f.callCounts = map[string]int{}
	}
	f.callCounts[key]++
	if f.failProductTool == key {
		return nil, errors.New("fixture lower call failed")
	}
	if f.failProductToolAt[key] == f.callCounts[key] {
		return nil, errors.New("fixture sequenced lower call failed")
	}
	text := `{"success":true}`
	switch key {
	case "contact/get_current_user_profile":
		text = `{"result":[{"orgEmployeeModel":{"userId":"self-user"}}]}`
	case "contact/get_user_info_by_user_ids":
		text = `{"result":[{"orgEmployeeModel":{"orgUserId":"user-id","orgUserName":"Resolved User"}}]}`
	case "contact/search_contact_by_key_word":
		keyword, _ := args["keyword"].(string)
		payload, _ := json.Marshal(map[string]any{
			"result": []map[string]any{
				{
					"name":           "Fuzzy Neighbor",
					"userId":         keyword + "-other",
					"openDingTalkId": "D-other",
				},
				{
					"name":           "Resolved User",
					"userId":         keyword,
					"openDingTalkId": "D-resolved",
				},
			},
		})
		text = string(payload)
	case "im/create_group_conversation":
		text = `{"result":{"cid":"internal-cid","openCid":"open-cid"}}`
	case "im/create_and_send_card":
		text = `{"result":{"bizId":"biz-created"}}`
	case "im/list_messages_by_ids":
		text = `{"result":[{"openMessageId":"msg","openConversationId":"cid","senderOpenDingTalkId":"D-inferred","content":"{\"mediaId\":\"@image\"}"}]}`
	case "im/list_conversations_by_category":
		text = f.category
		if text == "" {
			text = `{"result":{"hasMore":false,"list":[{"openConversationId":"cid-a","conversationName":"A"},{"openConversationId":"cid-b","conversationName":"B"}]}}`
		}
	}
	if response, ok := f.responses[key]; ok {
		text = response
	}
	if responses := f.sequenceResponses[key]; len(responses) > 0 {
		text = responses[0]
		f.sequenceResponses[key] = responses[1:]
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (f *larkAlignmentCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	return f.CallTool(ctx, product, tool, args)
}

func (f *larkAlignmentCaller) Format() string { return "json" }
func (f *larkAlignmentCaller) DryRun() bool   { return f.dryRun }
func (f *larkAlignmentCaller) Fields() string { return "" }
func (f *larkAlignmentCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageEvaluationRegressionNaturalGroupTargetsAndRecallInference(t *testing.T) {
	t.Run("group name to bots", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/search_groups":    `{"result":[{"openConversationId":"cid-project","title":"项目群"}],"hasMore":false}`,
			"bot/list_group_bots": `{"result":{"bots":[]}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+chat-bots", "--group", "项目群"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 2 || fake.calls[1].tool != "list_group_bots" || fake.calls[1].args["openConversationId"] != "cid-project" {
			t.Fatalf("calls = %#v", fake.calls)
		}
	})

	t.Run("group query to invite url", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/search_groups": `{"result":[{"openConversationId":"cid-project","title":"项目群"}],"hasMore":false}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+chat-invite-url", "--chat-query", "项目群"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 2 || fake.calls[1].tool != "get_group_invite_url" || fake.calls[1].args["openConversationId"] != "cid-project" {
			t.Fatalf("calls = %#v", fake.calls)
		}
	})

	t.Run("stable id in group query bypasses search", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+chat-invite-url", "--chat-query", "cid-fixture-chat-0001"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 1 || fake.calls[0].tool != "get_group_invite_url" ||
			fake.calls[0].args["openConversationId"] != "cid-fixture-chat-0001" {
			t.Fatalf("calls = %#v", fake.calls)
		}
	})

	t.Run("message id fills conversation before recall", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+messages-recall", "--message-ids", "msg", "--yes"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 2 || fake.calls[0].tool != "list_messages_by_ids" || fake.calls[1].tool != "recall_message" || fake.calls[1].args["openConversationId"] != "cid" {
			t.Fatalf("calls = %#v", fake.calls)
		}
	})
}

func TestCrossPlatformCoverageChatCreateAddsCurrentUserAndNormalizesResult(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-create",
		"--name", "测试群",
		"--users", "other-user,self-user",
		"--type", "NORMAL",
		"--thread",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %#v, want profile + create", fake.calls)
	}
	create := fake.calls[1]
	if create.product != "im" || create.tool != "create_group_conversation" {
		t.Fatalf("create call = %s/%s", create.product, create.tool)
	}
	if got, want := create.args["groupMembers"], []string{"self-user", "other-user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groupMembers = %#v, want %#v", got, want)
	}
	if create.args["groupType"] != "NORMAL" || create.args["convThreadEnabled"] != true {
		t.Fatalf("create args = %#v", create.args)
	}

	payload := map[string]any{"result": map[string]any{"cid": "secret", "openCid": "open"}}
	normalizeCreatedConversation(payload)
	result := payload["result"].(map[string]any)
	if result["openConversationId"] != "open" {
		t.Fatalf("normalized result = %#v", result)
	}
	if _, ok := result["cid"]; ok {
		t.Fatalf("internal cid leaked: %#v", result)
	}
}

func TestCrossPlatformCoverageChatCreateResolvesEveryNaturalMemberBeforeCreating(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"name":"张三","userId":"resolved-user","openDingTalkId":"D-resolved"}]}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-create",
		"--name", "测试群",
		"--users", "explicit-user",
		"--member-query", "张三",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("calls = %#v, want member resolve + current profile + create", fake.calls)
	}
	if fake.calls[0].tool != "search_contact_by_key_word" ||
		fake.calls[1].tool != "get_current_user_profile" ||
		fake.calls[2].tool != "create_group_conversation" {
		t.Fatalf("call order = %#v", fake.calls)
	}
	if got, want := fake.calls[2].args["groupMembers"], []string{"self-user", "explicit-user", "resolved-user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groupMembers = %#v, want %#v", got, want)
	}
}

func TestCrossPlatformCoverageChatCreateNaturalMemberAmbiguityStopsBeforeProfileAndCreate(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"name":"张三","userId":"u1"},{"name":"张三","userId":"u2"}]}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-create",
		"--name", "测试群",
		"--member-query", "张三",
		"--yes",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("ambiguous member unexpectedly created a group")
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "search_contact_by_key_word" {
		t.Fatalf("ambiguous member reached profile/create: %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageChatCreateNaturalMemberDryRunUsesSameResolutionChain(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"name":"张三","userId":"resolved-user"}]}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+chat-create",
		"--name", "测试群",
		"--member-query", "张三",
		"--dry-run",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 ||
		fake.calls[0].tool != "search_contact_by_key_word" ||
		fake.calls[1].tool != "get_current_user_profile" ||
		fake.calls[2].tool != "create_group_conversation" {
		t.Fatalf("dry-run calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageMessagesSendRoutesIdentitySpecificTransports(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		product string
		tool    string
		want    map[string]any
		bodyKey string
		body    string
	}{
		{
			name:    "user",
			args:    []string{"chat", "+messages-send", "--as", "user", "--chat-id", "cid", "--markdown", "hello @D1", "--at-open-dingtalk-ids", "D1", "--at-all", "--idempotency-key", "u1", "--yes"},
			product: "chat",
			tool:    "send_personal_message",
			want:    map[string]any{"openConversationId": "cid", "msgType": "markdown", "uuid": "u1"},
			bodyKey: "content",
			body:    "<@all> hello <@D1>",
		},
		{
			name:    "bot",
			args:    []string{"chat", "+messages-send", "--identity", "bot", "--robot-code", "robot", "--group", "cid", "--text", "<@u1> hello", "--at-user-ids", "u1", "--at-all", "--yes"},
			product: "bot",
			tool:    "send_robot_group_message",
			want:    map[string]any{"robotCode": "robot", "openConversationId": "cid", "isAtAll": "true"},
			bodyKey: "markdown",
			body:    "@all @u1 hello",
		},
		{
			name:    "webhook",
			args:    []string{"chat", "+messages-send", "--identity", "webhook", "--webhook-token", "token", "--text", "<@13800000000> hello", "--at-mobiles", "13800000000", "--at-all", "--yes"},
			product: "bot",
			tool:    "send_message_by_custom_robot",
			want:    map[string]any{"robotToken": "token", "isAtAll": true},
			bodyKey: "text",
			body:    "@all @13800000000 hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v", fake.calls)
			}
			call := fake.calls[0]
			if call.product != tt.product || call.tool != tt.tool {
				t.Fatalf("call = %#v", call)
			}
			for key, want := range tt.want {
				if !reflect.DeepEqual(call.args[key], want) {
					t.Errorf("%s = %#v, want %#v", key, call.args[key], want)
				}
			}
			if tt.bodyKey == "content" {
				var content map[string]string
				if err := json.Unmarshal([]byte(call.args[tt.bodyKey].(string)), &content); err != nil {
					t.Fatal(err)
				}
				if content["text"] != tt.body {
					t.Errorf("content text = %q, want %q", content["text"], tt.body)
				}
			} else if call.args[tt.bodyKey] != tt.body {
				t.Errorf("%s = %#v, want %q", tt.bodyKey, call.args[tt.bodyKey], tt.body)
			}
		})
	}
}

func TestCrossPlatformCoverageMessagesSendRejectsUnsupportedIdentityCapability(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send",
		"--identity", "bot",
		"--robot-code", "robot",
		"--group", "cid",
		"--text", "hello",
		"--uuid", "unsupported",
		"--yes",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("bot uuid unexpectedly accepted")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("invalid capability reached lower service: %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageLarkAlignmentWriteMappings(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		product  string
		tool     string
		wantArgs map[string]any
	}{
		{
			name:    "chat-update-name-only",
			args:    []string{"chat", "+chat-update", "--group", "cid-fixture-chat-0001", "--name", "新群名", "--yes"},
			product: "chat",
			tool:    "update_group_name",
			wantArgs: map[string]any{
				"openconversation_id": "cid-fixture-chat-0001",
				"group_name":          "新群名",
			},
		},
		{
			name:    "flag-create",
			args:    []string{"chat", "+flag-create", "--message-id", "msg", "--conversation-id", "cid", "--yes"},
			product: "im",
			tool:    "add_message_favorite",
			wantArgs: map[string]any{
				"openMessageId":      "msg",
				"openConversationId": "cid",
			},
		},
		{
			name:    "flag-cancel",
			args:    []string{"chat", "+flag-cancel", "--message-id", "msg", "--conversation-id", "cid", "--yes"},
			product: "im",
			tool:    "remove_message_favorite",
			wantArgs: map[string]any{
				"openMessageId":      "msg",
				"openConversationId": "cid",
			},
		},
		{
			name:    "flag-list",
			args:    []string{"chat", "+flag-list", "--cursor", "3", "--size", "30"},
			product: "im",
			tool:    "list_message_favorites",
			wantArgs: map[string]any{
				"cursor": 3,
				"size":   "30",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v, want 1", fake.calls)
			}
			call := fake.calls[0]
			if call.product != tt.product || call.tool != tt.tool || !reflect.DeepEqual(call.args, tt.wantArgs) {
				t.Fatalf("call = %#v, want %s/%s %#v", call, tt.product, tt.tool, tt.wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageObservedChatRenameAliasResolvesNameBeforeWrite(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/search_groups": `{"result":[{"openConversationId":"cid-project","title":"项目评测群"}],"hasMore":false}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-rename", "--group", "项目评测群", "--name", "项目讨论群", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].tool != "search_groups" || fake.calls[1].tool != "update_group_name" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	if fake.calls[1].args["openconversation_id"] != "cid-project" || fake.calls[1].args["group_name"] != "项目讨论群" {
		t.Fatalf("write args = %#v", fake.calls[1].args)
	}
}

func TestCrossPlatformCoverageMessagesReplyPublishesPlainTextBoundary(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"chat/send_personal_message": `{"result":{"openMessageId":"new-msg","openConvThreadId":"thread-1","sendStatus":"accepted"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-reply",
		"--conversation-id", "cid",
		"--message-id", "msg",
		"--ref-sender", "D-sender",
		"--text", "收到",
		"--idempotency-key", "reply-uuid",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %#v, want 1", fake.calls)
	}
	call := fake.calls[0]
	if call.product != "chat" || call.tool != "send_personal_message" {
		t.Fatalf("reply call = %#v", call)
	}
	if call.args["openConversationId"] != "cid" || call.args["msgType"] != "reply" || call.args["uuid"] != "reply-uuid" {
		t.Fatalf("reply args = %#v", call.args)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(call.args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["referenceOpenMessageId"] != "msg" ||
		content["srcMsgSendOpenDingTalkId"] != "D-sender" ||
		content["replyMsgType"] != "text" ||
		content["content"] != "收到" {
		t.Fatalf("reply content = %#v", content)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["contractVersion"] != "im.message-reply.v1" ||
		payload["messageId"] != "new-msg" ||
		payload["conversationId"] != "cid" ||
		payload["threadId"] != "thread-1" ||
		payload["deliveryStatus"] != "accepted" ||
		payload["idempotencyKey"] != "reply-uuid" {
		t.Fatalf("reply result context = %#v", payload)
	}
	referenced, _ := payload["referencedMessage"].(map[string]any)
	if referenced["messageId"] != "msg" || referenced["resolutionSource"] != "explicit" {
		t.Fatalf("referenced message context = %#v", referenced)
	}
}

func TestCrossPlatformCoverageMessagesReplyDryRunStopsBeforeWrite(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-reply",
		"--conversation-id", "cid",
		"--message-id", "msg",
		"--ref-sender", "D-sender",
		"--text", "收到",
		"--dry-run",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("reply dry-run reached write transport: %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageFlagBatchContinuesAndPublishesFailureLedger(t *testing.T) {
	fake := &larkAlignmentCaller{failTarget: "m2"}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+flag-create",
		"--message-ids", "m1,m2",
		"--conversation-id", "cid",
		"--yes",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("partial batch failure returned success")
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != false || payload["partial"] != true ||
		payload["requestedCount"] != float64(2) ||
		payload["succeededCount"] != float64(1) ||
		payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageConversationSetTopBatchDryRunPublishesActionsWithoutWrites(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+conversation-set-top",
		"--conversation-ids", "cid-1,cid-2",
		"--off",
		"--dry-run",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("dry-run reached lower service: %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["dry_run"] != true || payload["executed"] != false ||
		payload["preview_kind"] != "plan" ||
		payload["actionCount"] != float64(2) ||
		payload["failedCount"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageMessagesMgetDryRunPublishesMultiResourceDownloadPlan(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-mget",
		"--msg-ids", "msg",
		"--download-resources",
		"--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 ||
		fake.calls[0].product != "im" ||
		fake.calls[0].tool != "list_messages_by_ids" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	ledger, _ := payload["resourceDownloads"].(map[string]any)
	if ledger["dryRun"] != true || ledger["requestedCount"] != float64(1) {
		t.Fatalf("ledger = %#v", ledger)
	}
}

func TestCrossPlatformCoverageMessagesReplyResolvesUserIDBeforeExecution(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-reply",
		"--conversation-id", "cid",
		"--ref-msg-id", "msg",
		"--ref-sender", "user-id",
		"--text", "收到",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 ||
		fake.calls[0].product != "contact" ||
		fake.calls[0].tool != "search_contact_by_key_word" ||
		fake.calls[1].tool != "send_personal_message" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(fake.calls[1].args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["srcMsgSendOpenDingTalkId"] != "D-resolved" {
		t.Fatalf("content = %#v", content)
	}
}

func TestCrossPlatformCoverageMessagesReplyInfersSenderFromReferencedMessage(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-reply",
		"--conversation-id", "cid",
		"--ref-msg-id", "msg",
		"--text", "收到",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 ||
		fake.calls[0].product != "im" ||
		fake.calls[0].tool != "list_messages_by_ids" ||
		fake.calls[1].tool != "send_personal_message" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(fake.calls[1].args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["srcMsgSendOpenDingTalkId"] != "D-inferred" {
		t.Fatalf("content = %#v", content)
	}
}

func TestCrossPlatformCoverageFindMessageSenderOpenDingTalkIDIgnoresUnrelatedNestedIdentity(t *testing.T) {
	message := map[string]any{
		"content": map[string]any{
			"mentions": []any{
				map[string]any{"openDingTalkId": "D-wrong-recipient"},
			},
		},
		"quotedMessage": map[string]any{
			"senderOpenDingTalkId": "D-wrong-quoted-sender",
		},
	}
	if got := findMessageSenderOpenDingTalkID(message); got != "" {
		t.Fatalf("sender = %q, want empty", got)
	}
	message["senderInfo"] = map[string]any{"openDingTalkId": "D-correct"}
	if got := findMessageSenderOpenDingTalkID(message); got != "D-correct" {
		t.Fatalf("sender = %q, want D-correct", got)
	}
}

func TestCrossPlatformCoverageChatListDefaultsToGroupsAndSupportsLarkAliases(t *testing.T) {
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

func TestCrossPlatformCoverageChatListIncludesP2PAndRejectsInvalidTypes(t *testing.T) {
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
	chats := payload["chats"].([]any)
	direct := chats[1].(map[string]any)
	if direct["conversationType"] != "direct" || direct["chatMode"] != "p2p" {
		t.Fatalf("direct chat = %#v", direct)
	}

	helpers.InitDeps(&larkAlignmentCaller{})
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+chat-list", "--types", "channel"})
	if err := root.Execute(); err == nil {
		t.Fatal("invalid --types unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageChatListP2POnlyDropsGroups(t *testing.T) {
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
	chat := payload["chats"].([]any)[0].(map[string]any)
	if chat["openConversationId"] != "cid-direct" {
		t.Fatalf("chat = %#v", chat)
	}
}

func TestCrossPlatformCoverageFeedGroupQueryProjectPreservesRequestOrderAndMissingLedger(t *testing.T) {
	conversations := []map[string]any{
		{"openConversationId": "cid-a", "conversationName": "A"},
		{"openConversationId": "cid-b", "conversationName": "B"},
	}
	got := feedGroupQueryProject(conversations, []string{"cid-b", "cid-missing", "cid-a", "cid-b"})
	if got["requestedCount"] != 3 || got["foundCount"] != 2 {
		t.Fatalf("counts = %#v", got)
	}
	items := got["items"].([]map[string]any)
	if items[0]["openConversationId"] != "cid-b" || items[1]["openConversationId"] != "cid-a" {
		t.Fatalf("items = %#v", items)
	}
	if missing := got["notFoundConversationIds"]; !reflect.DeepEqual(missing, []string{"cid-missing"}) {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestCrossPlatformCoverageFeedGroupQueryDoesNotMisreportMissingItemWhenSourceHasMore(t *testing.T) {
	fake := &larkAlignmentCaller{
		category: `{"result":{"hasMore":true,"list":[{"openConversationId":"cid-a","conversationName":"A"}]}}`,
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+feed-group-query-item",
		"--category-id", "1",
		"--conversation-ids", "cid-a,cid-later",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["complete"] != false ||
		payload["notFoundCount"] != float64(0) ||
		payload["unresolvedCount"] != float64(1) ||
		payload["failedCount"] != float64(1) {
		t.Fatalf("payload = %#v", payload)
	}
	unresolved, _ := payload["unresolvedConversationIds"].([]any)
	if len(unresolved) != 1 || unresolved[0] != "cid-later" {
		t.Fatalf("unresolved = %#v", unresolved)
	}
}
