// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

type chatOutputErrorWriter struct {
	err error
}

func (w chatOutputErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestCrossPlatformCoverageMessagesSendPublishesCompleteIdentityConstraintInputs(t *testing.T) {
	want := []string{
		"identity", "as", "group", "chat-id", "groups", "groups-file", "chat-query", "user", "user-query", "open-dingtalk-id",
		"users", "open-dingtalk-ids", "robot-code", "webhook-token",
		"uuid", "idempotency-key",
	}
	var got []string
	for _, constraint := range MessagesSend.Constraints {
		if constraint.Kind == shortcut.ConstraintCustom {
			got = constraint.Flags
			break
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity constraint flags = %#v, want %#v", got, want)
	}
	wantSet := make(map[string]bool, len(want))
	for _, name := range want {
		wantSet[name] = true
	}
	for _, flag := range MessagesSend.Flags {
		if !wantSet[flag.Name] {
			continue
		}
		if !strings.Contains(flag.Desc, "能力矩阵") {
			t.Errorf("--%s does not publish identity matrix semantics: %q", flag.Name, flag.Desc)
		}
		if flag.Name == "user" && !strings.Contains(flag.Desc, "--dry-run") {
			t.Errorf("--user does not publish dry-run contact resolution: %q", flag.Desc)
		}
	}
}

func TestCrossPlatformCoverageMessagesSendIdentityDescriptorMatchesRuntimeSurface(t *testing.T) {
	capabilities := MessageIdentityCapabilities()
	if len(capabilities) != 3 {
		t.Fatalf("identity capabilities = %#v", capabilities)
	}
	byIdentity := make(map[string]MessageIdentityCapability, len(capabilities))
	for _, capability := range capabilities {
		byIdentity[capability.Identity] = capability
	}
	if !byIdentity["user"].IdempotencyKeys || byIdentity["user"].BatchLedger {
		t.Fatalf("user capability = %#v", byIdentity["user"])
	}
	if !byIdentity["bot"].BatchLedger || byIdentity["bot"].IdempotencyKeys ||
		!reflect.DeepEqual(byIdentity["bot"].ContentTypes, []string{"text", "markdown"}) {
		t.Fatalf("bot capability = %#v", byIdentity["bot"])
	}
	if byIdentity["webhook"].BatchLedger || byIdentity["webhook"].IdempotencyKeys {
		t.Fatalf("webhook capability = %#v", byIdentity["webhook"])
	}
	capabilities[0].ContentTypes[0] = "mutated"
	if MessageIdentityCapabilities()[0].ContentTypes[0] == "mutated" {
		t.Fatal("identity capability descriptor leaked mutable storage")
	}
	if !messageIdentitySupportsContent("user", "audio") ||
		!messageIdentitySupportsContent("user", "video") ||
		messageIdentitySupportsContent("missing", "text") {
		t.Fatal("identity content normalization or unknown-identity guard drifted")
	}
}

func TestCrossPlatformCoverageIMWorkflowContractsPublishRealPositiveAndNegativeBoundaries(t *testing.T) {
	card := CurrentCardWorkflowContract()
	if card.Version != "im.streaming-card.v1" || card.CallbackSupported ||
		!reflect.DeepEqual(card.ContentTypes, []string{"streaming-text"}) || len(card.FlowStatuses) != 5 {
		t.Fatalf("card contract = %#v", card)
	}
	card.Targets[0] = "mutated"
	if CurrentCardWorkflowContract().Targets[0] == "mutated" {
		t.Fatal("card contract leaked mutable storage")
	}

	boundaries := CurrentIMCapabilityBoundaries()
	byName := make(map[string]bool, len(boundaries))
	for _, boundary := range boundaries {
		byName[boundary.Capability] = boundary.Supported
		if boundary.Alternative == "" {
			t.Errorf("boundary %s lacks alternative", boundary.Capability)
		}
	}
	for _, unsupported := range []string{"thread-write", "bot-rich-media", "card-action-callback", "resource-resume"} {
		if byName[unsupported] {
			t.Errorf("unsupported boundary %s was advertised", unsupported)
		}
	}
	for _, supported := range []string{"group-member-full-pagination", "group-owner-selection"} {
		if !byName[supported] {
			t.Errorf("supported boundary %s was hidden", supported)
		}
	}
}

func TestCrossPlatformCoverageMessagesSendStatusAliasPublishesWorkflowReceipt(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/query_message_send_status": `{"result":{"status":"SUCCESS","openTaskId":"task-1","openMessageId":"msg-1","openConversationId":"cid-1"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+messages-send-status", "--open-task-id", "task-1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "query_message_send_status" || fake.calls[0].args["openTaskId"] != "task-1" {
		t.Fatalf("calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["contractVersion"] != chatmsg.MessageSendStatusContractVersion || payload["readyForMessageActions"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	ref, _ := payload["messageRef"].(map[string]any)
	if ref["openMessageId"] != "msg-1" || ref["openConversationId"] != "cid-1" {
		t.Fatalf("messageRef = %#v", ref)
	}
}

func TestCrossPlatformCoverageMessageWorkflowFailureAndPreviewBranches(t *testing.T) {
	t.Run("send status lower error", func(t *testing.T) {
		fake := &larkAlignmentCaller{failProductTool: "im/query_message_send_status"}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{"chat", "+messages-query-send-status", "--open-task-id", "task-1"})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "fixture lower call failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("create only dry run", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-send-card", "--group", "cid", "--dry-run", "--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("dry-run made calls: %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["actionCount"] != float64(1) || payload["executed"] != false {
			t.Fatalf("payload = %#v", payload)
		}
	})

	for _, test := range []struct {
		name      string
		fake      *larkAlignmentCaller
		wantError string
	}{
		{
			name:      "create only lower error",
			fake:      &larkAlignmentCaller{failProductTool: "im/create_and_send_card"},
			wantError: "fixture lower call failed",
		},
		{
			name: "create only missing biz id",
			fake: &larkAlignmentCaller{responses: map[string]string{
				"im/create_and_send_card": `{"result":{"created":true}}`,
			}},
			wantError: "未返回后续更新所需的 bizId",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			helpers.InitDeps(test.fake)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{"chat", "+messages-send-card", "--group", "cid", "--yes"})
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want substring %q", err, test.wantError)
			}
		})
	}

	t.Run("update card lower error", func(t *testing.T) {
		fake := &larkAlignmentCaller{failProductTool: "im/update_streaming_card"}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{
			"chat", "+messages-update-card",
			"--biz-id", "biz-1", "--content", "完成", "--flow-status", "3", "--yes",
		})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "fixture lower call failed") {
			t.Fatalf("error = %v", err)
		}
	})

	createErr := cardCreateMissingBizIDError(map[string]any{"created": true})
	var typed *apperrors.Error
	if !errors.As(createErr, &typed) || typed.Reason != "streaming_card_reference_missing" {
		t.Fatalf("create error = %#v", createErr)
	}
	for _, test := range []struct {
		cause      error
		wantReason string
	}{
		{cause: chatmsg.ErrCardUpdateNotApplied, wantReason: "streaming_card_update_not_applied"},
		{cause: chatmsg.ErrCardUpdateBizIDDrift, wantReason: "streaming_card_update_biz_id_mismatch"},
	} {
		typed = nil
		err := cardUpdateVerificationError("biz-1", test.cause)
		if !errors.As(err, &typed) || typed.Reason != test.wantReason {
			t.Errorf("cardUpdateVerificationError(%v) = %#v", test.cause, err)
		}
	}
}

func TestCrossPlatformCoverageMessagesSendPublishesStatusQueryReceipt(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"chat/send_personal_message": `{"result":{"openTaskId":"task-send-1"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-send", "--as", "user", "--chat-id", "cid-1",
		"--text", "hello", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	receipt, _ := payload["sendReceipt"].(map[string]any)
	if receipt["contractVersion"] != chatmsg.MessageSendReceiptContractVersion || receipt["openTaskId"] != "task-send-1" {
		t.Fatalf("sendReceipt = %#v", receipt)
	}
	actions, _ := receipt["nextActions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("nextActions = %#v", actions)
	}
}

func TestCrossPlatformCoverageMessagesSendBotMultiGroupPublishesPerTargetLedger(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-send", "--as", "bot", "--robot-code", "robot",
		"--groups", "cid-a,cid-b,cid-a", "--markdown", "通知", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("multi-group calls = %#v", fake.calls)
	}
	for index, target := range []string{"cid-a", "cid-b"} {
		if fake.calls[index].tool != "send_robot_group_message" ||
			fake.calls[index].args["openConversationId"] != target {
			t.Fatalf("multi-group call[%d] = %#v", index, fake.calls[index])
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["contractVersion"] != "im.batch-write.v1" || payload["ok"] != true ||
		payload["requestedCount"] != float64(2) || payload["succeededCount"] != float64(2) ||
		payload["failedCount"] != float64(0) {
		t.Fatalf("multi-group ledger = %#v", payload)
	}
}

func TestCrossPlatformCoverageMessagesSendBotMultiGroupFailuresReturnNonzero(t *testing.T) {
	tests := []struct {
		name          string
		fake          *larkAlignmentCaller
		wantSucceeded float64
		wantFailed    float64
		wantPartial   bool
	}{
		{
			name: "partial failure",
			fake: &larkAlignmentCaller{failProductToolAt: map[string]int{
				"bot/send_robot_group_message": 2,
			}},
			wantSucceeded: 1,
			wantFailed:    1,
			wantPartial:   true,
		},
		{
			name:          "all failed",
			fake:          &larkAlignmentCaller{failProductTool: "bot/send_robot_group_message"},
			wantSucceeded: 0,
			wantFailed:    2,
			wantPartial:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.InitDeps(tt.fake)
			root := newPlatformCoverageRoot()
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs([]string{
				"chat", "+messages-send", "--as", "bot", "--robot-code", "robot",
				"--groups", "cid-a,cid-b", "--markdown", "通知", "--yes",
			})

			err := root.Execute()
			if err == nil {
				t.Fatal("failed multi-group delivery returned success")
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI ||
				typed.Reason != "batch_write_failed" || typed.ExitCode() == 0 {
				t.Fatalf("batch error = %#v (%v)", typed, err)
			}

			var payload map[string]any
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["ok"] != false || payload["partial"] != tt.wantPartial ||
				payload["succeededCount"] != tt.wantSucceeded ||
				payload["failedCount"] != tt.wantFailed {
				t.Fatalf("failure ledger = %#v", payload)
			}
		})
	}
}

func TestCrossPlatformCoverageMessagesSendBotMultiGroupPropagatesOutputFailure(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	wantErr := errors.New("fixture output failed")
	root := newPlatformCoverageRoot()
	root.SetOut(chatOutputErrorWriter{err: wantErr})
	root.SetArgs([]string{
		"chat", "+messages-send", "--as", "bot", "--robot-code", "robot",
		"--groups", "cid-a,cid-b", "--markdown", "通知", "--yes",
	})

	if err := root.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("output error = %v, want %v", err, wantErr)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("multi-group calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageMessagesSendBotGroupsFileUsesSafeDeduplicatedTargets(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("groups.txt", []byte("# comment\ncid-a,cid-b\ncid-a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{
		"chat", "+messages-send", "--as", "bot", "--robot-code", "robot",
		"--groups-file", "groups.txt", "--text", "通知", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("groups-file calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageChatCreateExplicitOwnerSkipsCurrentProfileAndDeduplicatesMember(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{
		"chat", "+chat-create", "--name", "测试群", "--users", fixtureCurrentDOpenID + ",user-1",
		"--owner-open-dingtalk-id", fixtureCurrentDOpenID, "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "create_group_conversation" {
		t.Fatalf("explicit owner calls = %#v", fake.calls)
	}
	create := fake.calls[0].args
	if create["ownerOpenDingTalkId"] != fixtureCurrentDOpenID {
		t.Fatalf("ownerOpenDingTalkId = %#v", create["ownerOpenDingTalkId"])
	}
	if got, want := create["groupMembers"], []string{fixtureCurrentDOpenID, "user-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groupMembers = %#v, want %#v", got, want)
	}
}

func TestCrossPlatformCoverageChatCreateOwnerQueryResolvesBeforeSingleCreate(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"contact/search_contact_by_key_word": `{"result":[{"name":"测试用户甲","userId":"owner-user","openDingTalkId":"` + fixtureCurrentDOpenID + `"}]}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{
		"chat", "+chat-create", "--name", "测试群", "--users", "user-1",
		"--owner-query", "测试用户甲", "--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].tool != "search_contact_by_key_word" ||
		fake.calls[1].tool != "create_group_conversation" {
		t.Fatalf("owner query calls = %#v", fake.calls)
	}
	create := fake.calls[1].args
	if create["ownerOpenDingTalkId"] != fixtureCurrentDOpenID {
		t.Fatalf("ownerOpenDingTalkId = %#v", create["ownerOpenDingTalkId"])
	}
	if got, want := create["groupMembers"], []string{fixtureCurrentDOpenID, "user-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groupMembers = %#v, want %#v", got, want)
	}
}

func TestCrossPlatformCoverageSafeResourceDownloadsStayReadOnly(t *testing.T) {
	for _, command := range []shortcut.Shortcut{MessagesMget, MessagesResourceDownload} {
		if command.Risk != shortcut.RiskRead {
			t.Errorf("%s risk contract = %q", command.Command, command.Risk)
		}
	}
}

func TestCrossPlatformCoverageMessagesSendCurrentUserImageAndUserResolution(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send",
		"--user", "user-id",
		"--msg-type", "image",
		"--media-id", "@image",
		"--idempotency-key", "image-key",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %#v, want contact resolution + send", fake.calls)
	}
	if fake.calls[0].product != "contact" || fake.calls[0].tool != "search_contact_by_key_word" {
		t.Fatalf("resolution call = %#v", fake.calls[0])
	}
	if got := fake.calls[0].args["keyword"]; got != "user-id" {
		t.Fatalf("resolution keyword = %#v, want user-id", got)
	}
	send := fake.calls[1]
	if send.product != "chat" || send.tool != "send_personal_message" {
		t.Fatalf("send call = %#v", send)
	}
	for key, want := range map[string]any{
		"receiverOpenDingTalkId": "D-resolved",
		"msgType":                "image",
		"uuid":                   "image-key",
	} {
		if !reflect.DeepEqual(send.args[key], want) {
			t.Errorf("%s = %#v, want %#v", key, send.args[key], want)
		}
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(send.args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["mediaId"] != "@image" {
		t.Fatalf("image content = %#v", content)
	}
}

func TestCrossPlatformCoverageMessagesSendCurrentUserLocalFileFlow(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("report.txt", []byte("gap-fill"), 0o600); err != nil {
		t.Fatal(err)
	}
	var uploaded []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("upload method = %s", r.Method)
		}
		var err error
		uploaded, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/init_conversation_file_upload":   `{"resourceUrl":"` + server.URL + `","uploadKey":"upload-key"}`,
		"im/commit_conversation_file_upload": `{"result":{"dentryId":11,"spaceId":22}}`,
		"chat/send_personal_message":         `{"result":{"openMessageId":"sent-file"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-send",
		"--group", "cid",
		"--msg-type", "audio",
		"--file-path", "./report.txt",
		"--uuid", "file-key",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if string(uploaded) != "gap-fill" || len(fake.calls) != 3 {
		t.Fatalf("uploaded = %q, calls = %#v", uploaded, fake.calls)
	}
	if fake.calls[0].tool != "init_conversation_file_upload" ||
		fake.calls[1].tool != "commit_conversation_file_upload" ||
		fake.calls[2].tool != "send_personal_message" {
		t.Fatalf("file flow calls = %#v", fake.calls)
	}
	send := fake.calls[2]
	if send.args["msgType"] != "file" || send.args["openConversationId"] != "cid" ||
		send.args["uuid"] != "file-key" {
		t.Fatalf("file send args = %#v", send.args)
	}
	var content map[string]any
	if err := json.Unmarshal([]byte(send.args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["dentryId"] != float64(11) || content["spaceId"] != float64(22) ||
		content["fileName"] != "report.txt" {
		t.Fatalf("file content = %#v", content)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["requestedMessageType"] != "audio" || payload["effectiveMessageType"] != "file" {
		t.Fatalf("file output = %#v", payload)
	}
}

func TestCrossPlatformCoverageMessagesSendCurrentUserLocalFileDryRunAndFailures(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("fixture.bin", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(uploadServer.Close)
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-send",
		"--open-dingtalk-id", fixtureCurrentDOpenID,
		"--file", "./fixture.bin",
		"--dry-run",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("file dry-run made lower calls: %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["actionCount"] != float64(2) || payload["preview_kind"] != "plan" {
		t.Fatalf("dry-run payload = %#v", payload)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unsafe path", []string{"--group", "cid", "--file", "../outside"}},
		{"missing file", []string{"--group", "cid", "--file", "./missing"}},
		{"media mismatch", []string{"--group", "cid", "--media-id", "@image", "--msg-type", "file"}},
		{"file mismatch", []string{"--group", "cid", "--file", "./fixture.bin", "--msg-type", "image"}},
		{"missing media", []string{"--group", "cid", "--text", "x", "--msg-type", "image"}},
		{"bot media", []string{"--identity", "bot", "--robot-code", "robot", "--group", "cid", "--media-id", "@image"}},
		{"webhook media", []string{"--identity", "webhook", "--webhook-token", "token", "--media-id", "@image"}},
		{"user target conflict", []string{"--group", "cid", "--user", "u1", "--text", "x"}},
		{"user media at", []string{"--group", "cid", "--media-id", "@image", "--at-all"}},
		{"text flag with markdown type", []string{"--group", "cid", "--text", "x", "--msg-type", "markdown"}},
		{"markdown flag with text type", []string{"--group", "cid", "--markdown", "x", "--msg-type", "text"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(&larkAlignmentCaller{})
			command := newPlatformCoverageRoot()
			args := append([]string{"chat", "+messages-send"}, tc.args...)
			args = append(args, "--yes")
			command.SetArgs(args)
			if err := command.Execute(); err == nil {
				t.Fatalf("invalid args succeeded: %v", tc.args)
			}
		})
	}

	for _, tc := range []struct {
		name string
		fake *larkAlignmentCaller
	}{
		{
			name: "upload init error",
			fake: &larkAlignmentCaller{failProductTool: "im/init_conversation_file_upload"},
		},
		{
			name: "commit response missing IDs",
			fake: &larkAlignmentCaller{responses: map[string]string{
				"im/init_conversation_file_upload":   `{"resourceUrl":"` + uploadServer.URL + `","uploadKey":"key"}`,
				"im/commit_conversation_file_upload": `{}`,
			}},
		},
		{
			name: "message send error",
			fake: &larkAlignmentCaller{
				failProductTool: "chat/send_personal_message",
				responses: map[string]string{
					"im/init_conversation_file_upload":   `{"resourceUrl":"` + uploadServer.URL + `","uploadKey":"key"}`,
					"im/commit_conversation_file_upload": `{"dentryId":1,"spaceId":2}`,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.fake)
			command := newPlatformCoverageRoot()
			command.SetArgs([]string{
				"chat", "+messages-send",
				"--group", "cid",
				"--file", "./fixture.bin",
				"--yes",
			})
			if err := command.Execute(); err == nil {
				t.Fatal("file failure scenario unexpectedly succeeded")
			}
		})
	}
}

func TestCrossPlatformCoverageMessagesSendUserResolutionFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		fake *larkAlignmentCaller
	}{
		{"lookup error", &larkAlignmentCaller{failProductTool: "contact/search_contact_by_key_word"}},
		{"no exact user id", &larkAlignmentCaller{responses: map[string]string{
			"contact/search_contact_by_key_word": `{"result":[{"userId":"other-user","openDingTalkId":"D-other"}]}`,
		}}},
		{"missing open id", &larkAlignmentCaller{responses: map[string]string{
			"contact/search_contact_by_key_word": `{"result":[{"userId":"user-id","name":"Resolved User"}]}`,
		}}},
		{"ambiguous open id", &larkAlignmentCaller{responses: map[string]string{
			"contact/search_contact_by_key_word": `{"result":[{"userId":"user-id","openDingTalkId":"D-one"},{"userId":"user-id","openDingTalkId":"D-two"}]}`,
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.fake)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{
				"chat", "+messages-send",
				"--user", "user-id",
				"--text", "hello",
				"--yes",
			})
			if err := root.Execute(); err == nil {
				t.Fatal("unresolved user was accepted")
			}
			if len(tc.fake.calls) != 1 {
				t.Fatalf("calls = %#v", tc.fake.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMessagesSendTextModesAndExecuteGuard(t *testing.T) {
	for _, args := range [][]string{
		{"--group", "cid", "--markdown", "## markdown"},
		{"--group", "cid", "--text", "plain", "--msg-type", "text"},
	} {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs(append(append([]string{"chat", "+messages-send"}, args...), "--yes"))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 1 || fake.calls[0].tool != "send_personal_message" {
			t.Fatalf("text mode calls = %#v", fake.calls)
		}
	}

	shortcut.Register(shortcut.Shortcut{
		Service: "chat",
		Command: "+gap-send-execute-guard",
		Flags:   MessagesSend.Flags,
		Execute: executeMessagesSend,
	})
	helpers.InitDeps(&larkAlignmentCaller{})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+gap-send-execute-guard",
		"--group", "cid",
		"--media-id", "@image",
		"--msg-type", "file",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("execute-time content mismatch was accepted")
	}
}

func TestCrossPlatformCoverageMessagesSendCardOneCallLifecycle(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/create_and_send_card":  `{"result":{"card":{"biz_id":"biz-1","atTag":"<a atId=D-one>甲</a> <a atId=D-two>乙</a> "}}}`,
		"im/update_streaming_card": `{"result":{"updated":true}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-send-card",
		"--group", "cid",
		"--at-open-dingtalk-ids", fixtureCurrentDOpenID + "," + fixtureCurrentDOpenID2 + "," + fixtureCurrentDOpenID,
		"--at-all",
		"--content", "完成",
		"--flow-status", "3",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].tool != "create_and_send_card" ||
		fake.calls[1].tool != "update_streaming_card" {
		t.Fatalf("card calls = %#v", fake.calls)
	}
	if got, want := fake.calls[0].args["atOpenDingTalkIds"], []string{fixtureCurrentDOpenID, fixtureCurrentDOpenID2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("card create atOpenDingTalkIds = %#v, want %#v", got, want)
	}
	if fake.calls[0].args["atAll"] != true {
		t.Fatalf("card create atAll = %#v", fake.calls[0].args["atAll"])
	}
	if _, exists := fake.calls[1].args["atOpenDingTalkIds"]; exists {
		t.Fatalf("card update leaked atOpenDingTalkIds: %#v", fake.calls[1].args)
	}
	if _, exists := fake.calls[1].args["atAll"]; exists {
		t.Fatalf("card update leaked atAll: %#v", fake.calls[1].args)
	}
	if fake.calls[1].args["bizId"] != "biz-1" ||
		fake.calls[1].args["msgContent"] != "<a atId=D-one>甲</a> <a atId=D-two>乙</a> 完成" ||
		fake.calls[1].args["flowStatus"] != 3 {
		t.Fatalf("card update args = %#v", fake.calls[1].args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["bizId"] != "biz-1" || payload["ok"] != true {
		t.Fatalf("card output = %#v", payload)
	}
}

func TestCrossPlatformCoverageMessagesSendCardKeepsContentWithoutAtTag(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/create_and_send_card":  `{"result":{"bizId":"biz-no-mention"}}`,
		"im/update_streaming_card": `{"result":{"updated":true}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send-card",
		"--group", "cid",
		"--content", "正文",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[1].args["msgContent"] != "正文" {
		t.Fatalf("card calls = %#v", fake.calls)
	}
}

func TestCrossPlatformCoverageMessagesSendCardRejectsMissingRequestedAtTag(t *testing.T) {
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/create_and_send_card": `{"result":{"bizId":"biz-missing-at-tag"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send-card",
		"--group", "cid",
		"--at-open-dingtalk-ids", fixtureCurrentDOpenID,
		"--content", "正文",
		"--yes",
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "biz-missing-at-tag") || !strings.Contains(err.Error(), "atTag") {
		t.Fatalf("error = %v, want recoverable missing-atTag error containing bizId", err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "create_and_send_card" {
		t.Fatalf("card calls = %#v, want create only", fake.calls)
	}
}

func TestCrossPlatformCoverageMessagesSendCardResolvesReceiverForLowerTool(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send-card",
		"--receiver", "d-user-id",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 ||
		fake.calls[0].product != "contact" ||
		fake.calls[0].tool != "search_contact_by_key_word" ||
		fake.calls[1].product != "im" ||
		fake.calls[1].tool != "create_and_send_card" {
		t.Fatalf("card receiver calls = %#v", fake.calls)
	}
	if got := fake.calls[1].args["receiverOpenDingTalkId"]; got != "D-resolved" {
		t.Fatalf("receiverOpenDingTalkId = %#v, want D-resolved", got)
	}
	if got := fake.calls[0].args["keyword"]; got != "d-user-id" {
		t.Fatalf("D/d-prefixed userId resolution args = %#v", fake.calls[0].args)
	}
	if _, exists := fake.calls[1].args["receiverUid"]; exists {
		t.Fatalf("obsolete receiverUid leaked to lower tool: %#v", fake.calls[1].args)
	}
}

func TestCrossPlatformCoverageMessagesSendCardUsesExplicitOpenReceiver(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-send-card",
		"--receiver-open-dingtalk-id", fixtureCurrentDOpenID2,
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 ||
		fake.calls[0].product != "im" ||
		fake.calls[0].tool != "create_and_send_card" {
		t.Fatalf("card open receiver calls = %#v", fake.calls)
	}
	if got := fake.calls[0].args["receiverOpenDingTalkId"]; got != fixtureCurrentDOpenID2 {
		t.Fatalf("receiverOpenDingTalkId = %#v, want D-direct", got)
	}
}

func TestCrossPlatformCoverageMessagesSendCardDryRunAndFailureBoundaries(t *testing.T) {
	t.Run("create only", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{
			"chat", "+messages-send-card",
			"--group", "cid",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 1 || fake.calls[0].tool != "create_and_send_card" {
			t.Fatalf("create-only calls = %#v", fake.calls)
		}
	})

	t.Run("dry run", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-send-card",
			"--receiver", "user",
			"--content", "处理中",
			"--flow-status", "1",
			"--dry-run",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 1 ||
			fake.calls[0].product != "contact" ||
			fake.calls[0].tool != "search_contact_by_key_word" {
			t.Fatalf("card dry-run receiver resolution calls = %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		actions, _ := payload["actions"].([]any)
		create, _ := actions[0].(map[string]any)
		arguments, _ := create["arguments"].(map[string]any)
		if payload["actionCount"] != float64(2) ||
			arguments["receiverOpenDingTalkId"] != "D-resolved" {
			t.Fatalf("card plan = %#v", payload)
		}
	})

	t.Run("dry run mentions only in create", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-send-card",
			"--group", "cid",
			"--at-open-dingtalk-ids", fixtureCurrentDOpenID,
			"--at-all",
			"--content", "处理中",
			"--dry-run",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("card group dry-run calls = %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		actions, _ := payload["actions"].([]any)
		create, _ := actions[0].(map[string]any)
		createArguments, _ := create["arguments"].(map[string]any)
		update, _ := actions[1].(map[string]any)
		updateArguments, _ := update["arguments"].(map[string]any)
		if !reflect.DeepEqual(createArguments["atOpenDingTalkIds"], []any{fixtureCurrentDOpenID}) || createArguments["atAll"] != true {
			t.Fatalf("card create dry-run mentions = %#v", createArguments)
		}
		if _, exists := updateArguments["atOpenDingTalkIds"]; exists {
			t.Fatalf("card update dry-run leaked atOpenDingTalkIds: %#v", updateArguments)
		}
		if _, exists := updateArguments["atAll"]; exists {
			t.Fatalf("card update dry-run leaked atAll: %#v", updateArguments)
		}
		if updateArguments["msgContent"] != "<atTag from create_and_send_card>处理中" {
			t.Fatalf("card update dry-run content = %#v", updateArguments["msgContent"])
		}
	})

	t.Run("receiver resolution error", func(t *testing.T) {
		fake := &larkAlignmentCaller{failProductTool: "contact/search_contact_by_key_word"}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{
			"chat", "+messages-send-card",
			"--receiver", "user-id",
			"--yes",
		})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "解析为 openDingTalkId 失败") {
			t.Fatalf("receiver resolution error = %v", err)
		}
		if len(fake.calls) != 1 || fake.calls[0].tool != "search_contact_by_key_word" {
			t.Fatalf("receiver resolution calls = %#v", fake.calls)
		}
	})

	for _, tc := range []struct {
		name      string
		fake      *larkAlignmentCaller
		wantError string
	}{
		{
			name:      "create error",
			fake:      &larkAlignmentCaller{failProductTool: "im/create_and_send_card"},
			wantError: "fixture lower call failed",
		},
		{
			name: "missing biz id",
			fake: &larkAlignmentCaller{responses: map[string]string{
				"im/create_and_send_card": `{"result":{"created":true}}`,
			}},
			wantError: "未返回 bizId",
		},
		{
			name: "update error preserves id",
			fake: &larkAlignmentCaller{
				failProductTool: "im/update_streaming_card",
				responses: map[string]string{
					"im/create_and_send_card": `{"bizId":"biz-preserved"}`,
				},
			},
			wantError: "biz-preserved",
		},
		{
			name: "unverified update preserves id",
			fake: &larkAlignmentCaller{responses: map[string]string{
				"im/create_and_send_card":  `{"bizId":"biz-unverified"}`,
				"im/update_streaming_card": `{"result":{"updated":false}}`,
			}},
			wantError: "biz-unverified",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDeps(tc.fake)
			root := newPlatformCoverageRoot()
			root.SetArgs([]string{
				"chat", "+messages-send-card",
				"--group", "cid",
				"--content", "完成",
				"--yes",
			})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantError)
			}
		})
	}

	t.Run("success acknowledgement completes composite update", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/create_and_send_card":  `{"bizId":"biz-acknowledged"}`,
			"im/update_streaming_card": `{"success":true,"errorCode":null}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-send-card",
			"--group", "cid",
			"--content", "完成",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 2 || fake.calls[1].tool != "update_streaming_card" {
			t.Fatalf("calls = %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["updateAccepted"] != true || payload["updateVerified"] != false || payload["updateWarning"] == "" {
			t.Fatalf("card payload = %#v", payload)
		}
	})

	for _, args := range [][]string{
		{"--group", "cid", "--content", "x", "--flow-status", "6"},
		{"--group", "cid", "--flow-status", "2"},
		{"--group", "cid", "--receiver-open-dingtalk-id", fixtureCurrentDOpenID2},
		{"--receiver", "user-id", "--receiver-open-dingtalk-id", fixtureCurrentDOpenID2},
		{"--receiver", "user-id", "--at-open-dingtalk-ids", fixtureCurrentDOpenID},
		{"--receiver-open-dingtalk-id", fixtureCurrentDOpenID2, "--at-all"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+messages-send-card"}, args...))
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid card args succeeded: %v", args)
		}
	}
}

func TestCrossPlatformCoverageExplicitOpenIDValidationEdges(t *testing.T) {
	if err := validateExplicitOpenIDs("--open-dingtalk-ids", []string{" ", fixtureCurrentDOpenID}); err != nil {
		t.Fatalf("blank and valid IDs: %v", err)
	}
	if err := validateExplicitOpenIDs("--open-dingtalk-ids", []string{fixtureCurrentDOpenID, "not-an-open-id"}); err == nil {
		t.Fatal("invalid explicit open ID unexpectedly accepted")
	}

	for _, args := range [][]string{
		{"chat", "+messages-send", "--open-dingtalk-id", "not-an-open-id", "--text", "x", "--yes"},
		{"chat", "+messages-send", "--open-dingtalk-ids", "not-an-open-id", "--text", "x", "--yes"},
		{"chat", "+messages-send", "--group", "cid", "--at-open-dingtalk-ids", "not-an-open-id", "--text", "x", "--yes"},
		{"chat", "+messages-send-card", "--receiver-open-dingtalk-id", "not-an-open-id", "--yes"},
		{"chat", "+messages-send-card", "--group", "cid", "--at-open-dingtalk-ids", "not-an-open-id", "--yes"},
		{"chat", "+conversation-info", "--open-dingtalk-id", "not-an-open-id"},
		{"chat", "+chat-members-get", "--id", "cid", "--users", "not-an-open-id"},
		{"chat", "+messages-send-by-bot", "--robot-code", "robot", "--group", "cid", "--title", "title", "--text", "text", "--at-open-dingtalk-ids", "not-an-open-id", "--yes"},
		{"chat", "+messages-batch-send-by-bot", "--robot-code", "robot", "--title", "title", "--text", "text", "--open-dingtalk-ids", "not-an-open-id", "--yes"},
		{"chat", "+messages-list-direct", "--open-dingtalk-id", "not-an-open-id", "--time", "2026-01-01 00:00:00"},
		{"chat", "+chat-create", "--name", "fixture", "--users", "user-1", "--owner-open-dingtalk-id", "not-an-open-id", "--yes"},
	} {
		root := newPlatformCoverageRoot()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid explicit open ID unexpectedly accepted: %v", args)
		}
	}
}

func TestCrossPlatformCoverageMessagesUpdateCardVerifiesSuccess(t *testing.T) {
	t.Run("agent shortcut owns confirmation boundary", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/update_streaming_card": `{"result":{"bizId":"biz-confirm","updated":true}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetIn(strings.NewReader(""))
		root.SetArgs([]string{
			"chat", "+messages-update-card",
			"--biz-id", "biz-confirm",
			"--content", "高层更新",
			"--flow-status", "3",
		})
		err := root.Execute()
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
			t.Fatalf("error = %#v, want confirmation_required", err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("unconfirmed shortcut reached MCP: %#v", fake.calls)
		}
	})

	t.Run("success acknowledgement is accepted but unverified", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/update_streaming_card": `{"success":true,"errorCode":null}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-update-card",
			"--biz-id", "中文乱串",
			"--content", "完成",
			"--flow-status", "3",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 1 || fake.calls[0].tool != "update_streaming_card" {
			t.Fatalf("calls = %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["accepted"] != true || payload["verified"] != false || payload["warning"] == "" {
			t.Fatalf("payload = %#v", payload)
		}
	})

	t.Run("explicit update evidence succeeds", func(t *testing.T) {
		fake := &larkAlignmentCaller{responses: map[string]string{
			"im/update_streaming_card": `{"result":{"bizId":"biz-verified","updated":true}}`,
		}}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{
			"chat", "+messages-update-card",
			"--biz-id", "biz-verified",
			"--content", "完成",
			"--flow-status", "3",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("placeholder fails before write", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		root.SetArgs([]string{
			"chat", "+messages-update-card",
			"--biz-id", "<bizId>",
			"--content", "完成",
			"--flow-status", "3",
			"--yes",
		})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "占位符") {
			t.Fatalf("error = %v, want placeholder validation", err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("invalid placeholder made calls: %#v", fake.calls)
		}
	})

	t.Run("dry run only publishes plan", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-update-card",
			"--biz-id", "biz-preview",
			"--content", "完成",
			"--flow-status", "3",
			"--dry-run",
			"--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("dry-run made calls: %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["executed"] != false || payload["verified"] != false {
			t.Fatalf("dry-run payload = %#v", payload)
		}
	})
}

func TestCrossPlatformCoverageFindCardBizIDResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{map[string]any{"bizId": "direct"}, "direct"},
		{map[string]any{"bizID": "caps"}, "caps"},
		{map[string]any{"biz_id": "snake"}, "snake"},
		{map[string]any{"result": []any{map[string]any{"bizId": "nested"}}}, "nested"},
		{map[string]any{
			"metadata": map[string]any{"bizId": "stale"},
			"result":   map[string]any{"bizId": "current"},
		}, "current"},
		{map[string]any{
			"extension": map[string]any{"bizId": "fallback"},
		}, "fallback"},
		{map[string]any{
			"bizId":  map[string]any{"x": 1},
			"result": map[string]any{"bizId": "typed"},
		}, "typed"},
		{map[string]any{"bizId": map[string]any{"x": 1}}, ""},
		{`{"result":{"bizId":"json"}}`, "json"},
		{`[{"bizId":"array-json"}]`, "array-json"},
		{`{"bizId":`, ""},
		{"plain", ""},
		{42, ""},
	} {
		if got := findCardBizID(tc.value); got != tc.want {
			t.Errorf("findCardBizID(%#v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageFindCardAtTagResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{map[string]any{"atTag": "<a atId=D-direct>甲</a> "}, "<a atId=D-direct>甲</a> "},
		{map[string]any{"result": map[string]any{"atTag": "<a atId=D-nested>乙</a> "}}, "<a atId=D-nested>乙</a> "},
		{`{"result":{"card":{"atTag":"<a atId=D-json>丙</a> "}}}`, "<a atId=D-json>丙</a> "},
		{map[string]any{"atTag": "  "}, ""},
		{map[string]any{"atTag": 42}, ""},
		{map[string]any{"result": map[string]any{"created": true}}, ""},
	} {
		if got := findCardAtTag(tc.value); got != tc.want {
			t.Errorf("findCardAtTag(%#v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestCrossPlatformCoverageMessageResourceDownloadKeepsNestedMessageContext(t *testing.T) {
	resetResourceDownloadHooks(t)
	t.Chdir(t.TempDir())
	resourceDownload = func(
		_ context.Context,
		_ *http.Client,
		_ string,
		_ map[string]string,
		dest string,
		_ bool,
	) (int64, error) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return 0, err
		}
		return 7, nil
	}
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/get_resource_download_url": `{"result":{"resourceUrl":"https://download.dingtalk.com/resource.bin"}}`,
	}}
	helpers.InitDeps(fake)
	message := map[string]any{
		"openMessageId": "msg",
		"content":       `{"mediaId":"@same","nested":{"mediaId":"@same"}}`,
		"quotedMessage": map[string]any{
			"openMessageId": "msg-quoted",
			"content":       `{"mediaId":"@quoted"}`,
		},
	}
	secondMessage := map[string]any{
		"openMessageId": "msg-2",
		"content":       `{"mediaId":"@same"}`,
	}
	var ledger map[string]any
	shortcut.Register(shortcut.Shortcut{
		Service: "chat",
		Command: "+gap-resource-download",
		Flags:   MessageResourceDownloadFlags(),
		Execute: func(rt *shortcut.RuntimeContext) error {
			ledger = DownloadMessageResources(rt, []map[string]any{message, message, secondMessage}, "cid-fallback")
			return nil
		},
	})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+gap-resource-download", "--output-dir", "./downloads"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if ledger["discoveredCount"] != 5 || ledger["requestedCount"] != 3 ||
		ledger["deduplicatedCount"] != 2 || ledger["downloadedCount"] != 3 {
		t.Fatalf("download ledger = %#v", ledger)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("resource lookup = %#v", fake.calls)
	}
	gotMessageIDs := map[string]bool{}
	for _, call := range fake.calls {
		if call.args["openConversationId"] != "cid-fallback" {
			t.Errorf("resource conversation = %#v", call.args)
		}
		gotMessageIDs[call.args["openMessageId"].(string)] = true
	}
	for _, messageID := range []string{"msg", "msg-quoted", "msg-2"} {
		if !gotMessageIDs[messageID] {
			t.Errorf("missing resource lookup for %q: %#v", messageID, fake.calls)
		}
	}
}

func TestCrossPlatformCoverageMessageFileResourceDownloadUsesDriveAndPreservesName(t *testing.T) {
	resetResourceDownloadHooks(t)
	t.Chdir(t.TempDir())
	resourceDownload = func(
		_ context.Context,
		_ *http.Client,
		_ string,
		_ map[string]string,
		dest string,
		_ bool,
	) (int64, error) {
		if err := os.WriteFile(dest, []byte("drive-file"), 0o600); err != nil {
			return 0, err
		}
		return int64(len("drive-file")), nil
	}
	fake := &larkAlignmentCaller{responses: map[string]string{
		"drive/download_file": `{"result":{"downloadUrl":"https://download.dingtalk.com/opaque","fileName":"fixture.txt"}}`,
	}}
	helpers.InitDeps(fake)

	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-resource-download",
		"--type", "fileId",
		"--resource-id", "drive-file",
		"--output", "./downloads/",
		"--yes",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join("downloads", "fixture.txt"))
	if err != nil || string(content) != "drive-file" {
		t.Fatalf("downloaded content = %q, err = %v", content, err)
	}
	if len(fake.calls) != 1 ||
		fake.calls[0].product != "drive" ||
		fake.calls[0].tool != "download_file" ||
		fake.calls[0].args["fileId"] != "drive-file" {
		t.Fatalf("drive call = %#v", fake.calls)
	}
	helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
		"drive/download_file": `{"result":{"downloadUrl":"https://download.dingtalk.com/opaque"}}`,
	}})

	var ledger map[string]any
	shortcut.Register(shortcut.Shortcut{
		Service: "chat",
		Command: "+gap-file-resource-download",
		Flags:   MessageResourceDownloadFlags(),
		Execute: func(rt *shortcut.RuntimeContext) error {
			ledger = DownloadMessageResources(rt, []map[string]any{{
				"content": "[文件] fixture.txt fileId: drive-file",
			}}, "")
			return nil
		},
	})
	root = newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+gap-file-resource-download", "--output-dir", "./batch-downloads"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if ledger["downloadedCount"] != 1 || ledger["failedCount"] != 0 {
		t.Fatalf("file download ledger = %#v", ledger)
	}
	downloads := ledger["downloads"].([]map[string]any)
	if downloads[0]["resourceType"] != "fileId" ||
		downloads[0]["localPath"] != "batch-downloads/fixture.txt" ||
		downloads[0]["messageId"] != "" {
		t.Fatalf("file download = %#v", downloads[0])
	}
}

func TestCrossPlatformCoverageMessageResourceDownloadDisambiguatesSameNames(t *testing.T) {
	resetResourceDownloadHooks(t)
	t.Chdir(t.TempDir())
	destinations := make([]string, 0, 2)
	resourceDownload = func(
		_ context.Context,
		_ *http.Client,
		_ string,
		_ map[string]string,
		dest string,
		_ bool,
	) (int64, error) {
		destinations = append(destinations, filepath.Base(dest))
		return 1, nil
	}
	helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
		"drive/download_file": `{"result":{"downloadUrl":"https://download.dingtalk.com/opaque","fileName":"fixture.txt"}}`,
	}})

	var ledger map[string]any
	shortcut.Register(shortcut.Shortcut{
		Service: "chat",
		Command: "+gap-colliding-resource-download",
		Flags:   MessageResourceDownloadFlags(),
		Execute: func(rt *shortcut.RuntimeContext) error {
			ledger = DownloadMessageResources(rt, []map[string]any{
				{"fileId": "file-a"},
				{"fileId": "file-b"},
			}, "")
			return nil
		},
	})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+gap-colliding-resource-download", "--output-dir", "./downloads"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(destinations, []string{"fixture.txt", "fixture (2).txt"}) {
		t.Fatalf("download destinations = %#v", destinations)
	}
	if ledger["downloadedCount"] != 2 || ledger["failedCount"] != 0 {
		t.Fatalf("download ledger = %#v", ledger)
	}
}

func TestCrossPlatformCoverageResourceDownloadFilenameCollisionSequence(t *testing.T) {
	used := map[string]bool{}
	first := disambiguateResourceDownloadFilename("a.txt", used)
	used[strings.ToLower(first)] = true
	second := disambiguateResourceDownloadFilename("a.txt", used)
	used[strings.ToLower(second)] = true
	third := disambiguateResourceDownloadFilename("a (2).txt", used)
	if first != "a.txt" || second != "a (2).txt" || third != "a (2) (2).txt" {
		t.Fatalf("collision sequence = %q, %q, %q", first, second, third)
	}
}

func TestCrossPlatformCoverageFailedResourceDownloadDoesNotConsumeFilename(t *testing.T) {
	resetResourceDownloadHooks(t)
	t.Chdir(t.TempDir())
	destinations := make([]string, 0, 2)
	attempt := 0
	resourceDownload = func(
		_ context.Context,
		_ *http.Client,
		_ string,
		_ map[string]string,
		dest string,
		_ bool,
	) (int64, error) {
		destinations = append(destinations, filepath.Base(dest))
		attempt++
		if attempt == 1 {
			return 0, errors.New("fixture download failed")
		}
		return 1, nil
	}
	helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
		"drive/download_file": `{"result":{"downloadUrl":"https://download.dingtalk.com/opaque","fileName":"fixture.txt"}}`,
	}})

	var ledger map[string]any
	shortcut.Register(shortcut.Shortcut{
		Service: "chat",
		Command: "+gap-failed-colliding-resource-download",
		Flags:   MessageResourceDownloadFlags(),
		Execute: func(rt *shortcut.RuntimeContext) error {
			ledger = DownloadMessageResources(rt, []map[string]any{
				{"fileId": "file-a"},
				{"fileId": "file-b"},
			}, "")
			return nil
		},
	})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{"chat", "+gap-failed-colliding-resource-download", "--output-dir", "./downloads"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(destinations, []string{"fixture.txt", "fixture.txt"}) {
		t.Fatalf("failed download consumed a filename: %#v", destinations)
	}
	if ledger["downloadedCount"] != 1 || ledger["failedCount"] != 1 {
		t.Fatalf("download ledger = %#v", ledger)
	}
}

func TestCrossPlatformCoverageMessageResourceDownloadRequiresMediaContextOnly(t *testing.T) {
	helpers.InitDeps(&larkAlignmentCaller{})
	root := newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-resource-download",
		"--resource-id", "@media",
	})
	if err := root.Execute(); err == nil ||
		!strings.Contains(err.Error(), "--type mediaId") {
		t.Fatalf("missing media context error = %v", err)
	}

	root = newPlatformCoverageRoot()
	root.SetArgs([]string{
		"chat", "+messages-resource-download",
		"--type", "fileId",
		"--resource-id", "drive-file",
		"--output", "../outside",
	})
	if err := root.Execute(); err == nil ||
		!strings.Contains(err.Error(), "--output") {
		t.Fatalf("unsafe output error = %v", err)
	}
}

func TestCrossPlatformCoverageReadHelperRejectsWriteToolDirectly(t *testing.T) {
	fake := &larkAlignmentCaller{}
	helpers.InitDeps(fake)
	if _, err := helpers.CallMCPReadToolTextOnServer("chat", "send_personal_message", nil); err == nil {
		t.Fatal("write tool was accepted by the helper read channel")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("rejected write reached lower caller: %#v", fake.calls)
	}
}
