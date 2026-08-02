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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestMessagesSendPublishesCompleteIdentityConstraintInputs(t *testing.T) {
	want := []string{
		"identity", "as", "group", "chat-id", "user", "open-dingtalk-id",
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
		"--open-dingtalk-id", "D-target",
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
		"im/create_and_send_card":  `{"result":{"card":{"biz_id":"biz-1"}}}`,
		"im/update_streaming_card": `{"result":{"updated":true}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-send-card",
		"--group", "cid",
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
	if fake.calls[1].args["bizId"] != "biz-1" || fake.calls[1].args["msgContent"] != "完成" ||
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
		"--receiver-open-dingtalk-id", "D-direct",
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
	if got := fake.calls[0].args["receiverOpenDingTalkId"]; got != "D-direct" {
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

	for _, args := range [][]string{
		{"--group", "cid", "--content", "x", "--flow-status", "6"},
		{"--group", "cid", "--flow-status", "2"},
		{"--group", "cid", "--receiver-open-dingtalk-id", "D-direct"},
		{"--receiver", "user-id", "--receiver-open-dingtalk-id", "D-direct"},
	} {
		helpers.InitDeps(&larkAlignmentCaller{})
		root := newPlatformCoverageRoot()
		root.SetArgs(append([]string{"chat", "+messages-send-card"}, args...))
		if err := root.Execute(); err == nil {
			t.Fatalf("invalid card args succeeded: %v", args)
		}
	}
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
