// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
)

type messageReadFakeCipher struct{}

func (messageReadFakeCipher) EncryptMessage(_ context.Context, _, _ string, plaintext []byte) ([]byte, error) {
	return []byte("safe:" + string(plaintext)), nil
}

func (messageReadFakeCipher) DecryptMessage(_ context.Context, _, _ string, ciphertext []byte) ([]byte, error) {
	return []byte("ding:" + string(ciphertext)), nil
}

type messageReadSessionCipher struct{}

func (messageReadSessionCipher) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (messageReadSessionCipher) DecryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (messageReadSessionCipher) Close() error { return nil }

type messageReadFailingCipher struct{}

func (messageReadFailingCipher) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (messageReadFailingCipher) DecryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("safechat failed")
}

func swapMessageReadCryptoClient(t *testing.T, client *messagecrypto.Client) {
	t.Helper()
	old := messageReadCryptoClient
	messageReadCryptoClient = client
	t.Cleanup(func() { messageReadCryptoClient = old })
}

func TestCrossPlatformCoverageListMessageRichProjection(t *testing.T) {
	rows := listMessagesProject(map[string]any{"result": map[string]any{"messages": []any{
		map[string]any{
			"openMessageId":      "msg",
			"openConversationId": "cid",
			"threadId":           "thread",
			"msgType":            "text",
			"createTime":         "1",
			"updateTime":         "2",
			"content":            `{"mediaId":"@image"}`,
			"quotedMessage": map[string]any{
				"openMessageId": "quoted",
				"content":       `{"mediaId":"@quoted-image"}`,
			},
		},
	}}})
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	for _, key := range []string{"threadId", "updateTime", "quotedMessage", "resourceRefs"} {
		if _, ok := rows[0][key]; !ok {
			t.Errorf("projection missing %s: %#v", key, rows[0])
		}
	}
	resources := rows[0]["resourceRefs"].([]map[string]any)
	if len(resources) != 2 {
		t.Fatalf("projected resources = %#v", resources)
	}
	quotedArgs := resources[1]["download"].(map[string]any)["arguments"].(map[string]any)
	if resources[1]["resourceId"] != "@quoted-image" ||
		quotedArgs["message-id"] != "quoted" ||
		quotedArgs["open-conversation-id"] != "cid" {
		t.Fatalf("quoted resource context = %#v", resources[1])
	}
}

func TestCrossPlatformCoverageMessagesMgetDecryptsEncryptedMessagesInBatch(t *testing.T) {
	swapMessageReadCryptoClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: messageReadFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_messages_by_ids":        `{"result":[{"openMessageId":"m1","openConversationId":"cid","content":"` + testCipher + `"},{"openMessageId":"m2","openConversationId":"cid","content":"plain"}]}`,
		"im/get_message_crypto_policy":   `{"result":{"mode":"required","reason":"admin-required"}}`,
		"im/batch_ding_decrypt_messages": `{"result":{"items":[{"messageId":"m1","status":"success","plaintextContent":"hello decrypted","keyVersion":8}]}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-mget",
		"--msg-ids", "m1,m2",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 ||
		fake.calls[0].tool != "list_messages_by_ids" ||
		fake.calls[1].tool != "get_message_crypto_policy" ||
		fake.calls[2].tool != "batch_ding_decrypt_messages" {
		t.Fatalf("calls = %#v, want list + policy + one batch decrypt", fake.calls)
	}
	items, _ := fake.calls[2].args["items"].([]map[string]any)
	if len(items) != 1 || items[0]["messageId"] != "m1" || items[0]["dingCiphertext"] != "ding:"+testCipher {
		t.Fatalf("batch decrypt args = %#v", fake.calls[2].args)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["decryptCandidateCount"] != float64(1) ||
		payload["decryptedCount"] != float64(1) ||
		payload["decryptFailedCount"] != float64(0) {
		t.Fatalf("decrypt ledger = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != "hello decrypted" {
		t.Fatalf("decrypted message = %#v", first)
	}
}

func TestCrossPlatformCoverageMessagesMgetFallsBackToOriginalWhenPolicyFails(t *testing.T) {
	swapMessageReadCryptoClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: messageReadFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	fake := &larkAlignmentCaller{
		responses: map[string]string{
			"im/list_messages_by_ids": `{"result":[{"openMessageId":"m1","openConversationId":"cid","content":"` + testCipher + `"}]}`,
		},
		failProductTool: "im/get_message_crypto_policy",
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+messages-mget", "--msg-ids", "m1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["decryptFailedCount"] != float64(1) || payload["partial"] != true {
		t.Fatalf("decrypt fallback ledger = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != testCipher || first["contentDecrypted"] == true {
		t.Fatalf("fallback message = %#v", first)
	}
}

func TestCrossPlatformCoverageMessagesMgetSkipsCryptoWhenBackendUnavailable(t *testing.T) {
	swapMessageReadCryptoClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			t.Fatal("identity lookup must not run without the SafeChat backend")
			return messagecrypto.Identity{}, nil
		},
		BackendReady: func() bool { return false },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_messages_by_ids": `{"result":[{"openMessageId":"m1","openConversationId":"cid","content":"` + testCipher + `"}]}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+messages-mget", "--msg-ids", "m1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].tool != "list_messages_by_ids" {
		t.Fatalf("calls = %#v, want message list only", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"decryptCandidateCount", "decryptedCount", "decryptFailedCount", "decryptFailures"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("unavailable backend emitted %q: %#v", key, payload)
		}
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != "[加密消息，无法解码]" || first["contentDecrypted"] == true {
		t.Fatalf("message changed without backend: %#v", first)
	}
}

func TestCrossPlatformCoverageMessagesMgetFallsBackToOriginalWhenBatchDecryptFails(t *testing.T) {
	swapMessageReadCryptoClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: messageReadFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	fake := &larkAlignmentCaller{
		responses: map[string]string{
			"im/list_messages_by_ids":      `{"result":[{"openMessageId":"m1","openConversationId":"cid","content":"` + testCipher + `"}]}`,
			"im/get_message_crypto_policy": `{"result":{"mode":"required","reason":"admin-required"}}`,
		},
		failProductTool: "im/batch_ding_decrypt_messages",
	}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+messages-mget", "--msg-ids", "m1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["decryptCandidateCount"] != float64(1) ||
		payload["decryptedCount"] != float64(0) ||
		payload["decryptFailedCount"] != float64(1) ||
		payload["partial"] != true {
		t.Fatalf("decrypt fallback ledger = %#v", payload)
	}
	messages, _ := payload["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["text"] != testCipher || first["contentDecrypted"] == true {
		t.Fatalf("fallback message = %#v", first)
	}
}

func TestCrossPlatformCoverageMessageDecryptHelperEdges(t *testing.T) {
	t.Run("new read crypto client default paths", func(t *testing.T) {
		oldIdentity := messageReadCurrentIdentity
		oldOpen := messageReadOpenSession
		oldAvailable := messageReadAvailable
		t.Cleanup(func() {
			messageReadCurrentIdentity = oldIdentity
			messageReadOpenSession = oldOpen
			messageReadAvailable = oldAvailable
		})
		messageReadCurrentIdentity = func(context.Context, string) (msgcrypto.Identity, error) {
			return msgcrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		messageReadOpenSession = func(context.Context, msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			return &msgcrypto.Session{Cipher: messageReadSessionCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		messageReadAvailable = func() bool { return true }
		client := newMessageReadCryptoClient()
		if client == nil || client.PolicyCache == nil {
			t.Fatalf("client = %#v", client)
		}
		if !client.BackendReady() {
			t.Fatal("BackendReady() = false")
		}
		identity, err := client.Identity(context.Background(), t.TempDir())
		if err != nil || identity.CorpID != "corp-1" || identity.StaffID != "staff-1" {
			t.Fatalf("identity = %#v, %v", identity, err)
		}
		session, err := client.OpenSession(context.Background(), messagecrypto.SessionOptions{
			ConfigDir:           t.TempDir(),
			KeyServer:           "https://key.example.test",
			AllowedRedirectHost: "redirect.example.test",
		})
		if err != nil {
			t.Fatal(err)
		}
		if session.CorpID != "corp-1" || session.StaffID != "staff-1" || session.Cipher == nil {
			t.Fatalf("session = %#v", session)
		}

		messageReadOpenSession = func(context.Context, msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			return nil, errors.New("open failed")
		}
		if _, err := client.OpenSession(context.Background(), messagecrypto.SessionOptions{}); err == nil || err.Error() != "open failed" {
			t.Fatalf("OpenSession error = %v", err)
		}
	})

	messages := []map[string]any{
		{
			"openMessageId":      "root",
			"openConversationId": "cid-root",
			"content":            testCipher,
			"forwardMessages": []any{
				map[string]any{"openMessageId": "child", "text": testCipher},
				"ignored",
				map[string]any{"openMessageId": "", "content": testCipher},
			},
		},
		{"messageId": "<nil>", "content": testCipher},
		{"openMessageId": "plain", "content": "plain text"},
	}
	items := collectEncryptedMessageItems(messages)
	if len(items) != 3 || items[0].MessageID != "root" || items[1].MessageID != "child" {
		t.Fatalf("items = %#v", items)
	}
	index := indexMessageMapsByID(messages)
	if len(index["root"]) != 1 || len(index["child"]) != 1 {
		t.Fatalf("index = %#v", index)
	}
	markMessageDecryptFailures(messages, []map[string]any{
		messageDecryptFailure("root", "cid-root", ""),
		{"messageId": ""},
	})
	if got := messages[0][messageDecryptFailedOriginalContentKey]; got != testCipher {
		t.Fatalf("original encrypted content = %#v", got)
	}
	if got := firstNonEmptyShortcutString("", " fallback "); got != "fallback" {
		t.Fatalf("firstNonEmptyShortcutString = %q", got)
	}
	if got := firstNonEmptyShortcutString("", " "); got != "" {
		t.Fatalf("firstNonEmptyShortcutString empty = %q", got)
	}
}

func TestCrossPlatformCoverageMessageProjectionHelperEdges(t *testing.T) {
	messageRows := listMessagesProjectWithReactions(map[string]any{"result": []any{
		"ignored",
		map[string]any{"openMessageId": "msg"},
	}}, false)
	if len(messageRows) != 1 || messageRows[0]["messageId"] != "msg" {
		t.Fatalf("message rows = %#v", messageRows)
	}

	rows := listUnreadConversationsProject(map[string]any{"result": map[string]any{"list": []any{
		map[string]any{"cid": "cid-1", "name": "群聊", "count": 3, "updateTime": "now"},
		"ignored",
		map[string]any{},
	}}})
	if len(rows) != 1 || rows[0]["conversationId"] != "cid-1" || rows[0]["title"] != "群聊" ||
		rows[0]["unreadCount"] != 3 || rows[0]["lastMessageTime"] != "now" {
		t.Fatalf("unread rows = %#v", rows)
	}
	rows = listUnreadConversationsProject(map[string]any{"items": []any{
		map[string]any{"openConversationId": "cid-2", "title": "标题"},
	}})
	if len(rows) != 1 || rows[0]["conversationId"] != "cid-2" {
		t.Fatalf("top-level unread rows = %#v", rows)
	}
	if got := listUnreadConversationsResolveList(map[string]any{"unexpected": []any{"x"}}); len(got) != 0 {
		t.Fatalf("unexpected unread list = %#v", got)
	}
	if _, ok := listUnreadConversationsFirst(map[string]any{}, "missing"); ok {
		t.Fatal("missing unread key should not resolve")
	}

	pins := listPinProject(map[string]any{"result": map[string]any{"pinMessages": []any{
		map[string]any{"msgId": "m1", "operatorId": "u1", "gmtCreate": "t1", "openConvId": "cid", "threadId": "thread"},
		"ignored",
		map[string]any{},
	}}})
	if len(pins) != 1 || pins[0]["messageId"] != "m1" || pins[0]["senderId"] != "u1" ||
		pins[0]["pinTime"] != "t1" || pins[0]["conversationId"] != "cid" || pins[0]["threadId"] != "thread" {
		t.Fatalf("pin rows = %#v", pins)
	}
	pins = listPinProject(map[string]any{"messages": []any{
		map[string]any{"openMessageId": "m2"},
	}})
	if len(pins) != 1 || pins[0]["messageId"] != "m2" {
		t.Fatalf("top-level pin rows = %#v", pins)
	}
	if got := listPinResolveList(map[string]any{"unexpected": []any{"x"}}); len(got) != 0 {
		t.Fatalf("unexpected pin list = %#v", got)
	}
}

func TestCrossPlatformCoverageMessagesMgetDecryptItemFailureEdges(t *testing.T) {
	swapMessageReadCryptoClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: messageReadFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_messages_by_ids": `{"result":[` +
			`{"openMessageId":"ok","openConversationId":"cid","content":"` + testCipher + `"},` +
			`{"openMessageId":"failed","openConversationId":"cid","content":"` + testCipher + `"},` +
			`{"openMessageId":"empty","openConversationId":"cid","content":"` + testCipher + `"}` +
			`]}`,
		"im/get_message_crypto_policy":   `{"result":{"mode":"required"}}`,
		"im/batch_ding_decrypt_messages": `{"result":{"items":[{"messageId":"ok","status":"success","plaintextContent":"ok text","keyVersion":8},{"messageId":"failed","status":"failed","reason":"bad_key"},{"messageId":"empty","status":"success","plaintextContent":"  "}],"failures":[{"messageId":"missing","conversationId":"cid","reason":"lost"}]}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+messages-mget", "--msg-ids", "ok,failed,empty"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["decryptedCount"] != float64(1) || payload["decryptFailedCount"] != float64(2) || payload["partial"] != true {
		t.Fatalf("decrypt item failure ledger = %#v", payload)
	}
}

func TestCrossPlatformCoverageMessagesMgetRecordsSafeChatFailures(t *testing.T) {
	swapMessageReadCryptoClient(t, &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{Cipher: messageReadFailingCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	})
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_messages_by_ids":      `{"result":[{"openMessageId":"m1","openConversationId":"cid","content":"` + testCipher + `"}]}`,
		"im/get_message_crypto_policy": `{"result":{"mode":"required"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"chat", "+messages-mget", "--msg-ids", "m1"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["decryptFailedCount"] != float64(1) || payload["partial"] != true {
		t.Fatalf("safechat failure ledger = %#v", payload)
	}
}

func TestCrossPlatformCoverageMgetResourceDownloadOutcomes(t *testing.T) {
	baseArgs := []string{"chat", "+messages-mget", "--msg-ids", "msg", "--download-resources", "--yes"}
	readyMget := `{"result":[{"openMessageId":"msg","openConversationId":"cid","content":"{\"mediaId\":\"@file\"}"}]}`
	missingContextMget := `{"result":[{"content":"{\"mediaId\":\"@file\"}"}]}`
	validInfo := `{"result":{"resourceUrl":"https://download.dingtalk.com/resource.bin"}}`

	t.Run("dry run", func(t *testing.T) {
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
			"im/list_messages_by_ids": readyMget,
		}})
		root := newPlatformCoverageRoot()
		root.SetArgs(append(append([]string{}, baseArgs...), "--dry-run"))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("getwd", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		resourceGetwd = func() (string, error) { return "", errors.New("getwd") }
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
			"im/list_messages_by_ids": readyMget,
		}})
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(baseArgs)
		if err := root.Execute(); err != nil {
			t.Fatalf("getwd ledger error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		ledger, _ := payload["resourceDownloads"].(map[string]any)
		if ledger["requestedCount"] != float64(1) ||
			ledger["failedCount"] != ledger["requestedCount"] {
			t.Fatalf("getwd ledger = %#v", ledger)
		}
	})
	t.Run("zero resources skip getwd", func(t *testing.T) {
		resetResourceDownloadHooks(t)
		getwdCalled := false
		resourceGetwd = func() (string, error) {
			getwdCalled = true
			return "", errors.New("getwd")
		}
		helpers.InitDeps(&larkAlignmentCaller{responses: map[string]string{
			"im/list_messages_by_ids": `{"result":[{"openMessageId":"msg","openConversationId":"cid","content":"plain text"}]}`,
		}})
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(baseArgs)
		if err := root.Execute(); err != nil {
			t.Fatalf("zero-resource download error = %v", err)
		}
		if getwdCalled {
			t.Fatal("zero-resource download unnecessarily read the working directory")
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		ledger, _ := payload["resourceDownloads"].(map[string]any)
		failures, _ := ledger["failures"].([]any)
		if ledger["ok"] != true ||
			ledger["requestedCount"] != float64(0) ||
			ledger["failedCount"] != float64(0) ||
			len(failures) != 0 {
			t.Fatalf("zero-resource ledger = %#v", ledger)
		}
	})

	cases := []struct {
		name            string
		mget            string
		info            string
		failProductTool string
		outputDir       string
		downloadErr     error
		pathErr         bool
	}{
		{name: "missing context", mget: missingContextMget, info: validInfo},
		{name: "resource lookup", mget: readyMget, failProductTool: "im/get_resource_download_url"},
		{name: "invalid info", mget: readyMget, info: `{"result":{}}`},
		{name: "path", mget: readyMget, info: validInfo, outputDir: "go.mod", pathErr: true},
		{name: "download", mget: readyMget, info: validInfo, downloadErr: errors.New("download")},
		{name: "success", mget: readyMget, info: validInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetResourceDownloadHooks(t)
			if tc.pathErr {
				resourceAbs = func(string) (string, error) { return "", errors.New("path") }
			}
			resourceDownload = func(
				_ context.Context,
				_ *http.Client,
				_ string,
				_ map[string]string,
				_ string,
				_ bool,
			) (int64, error) {
				return 4, tc.downloadErr
			}
			caller := &larkAlignmentCaller{
				failProductTool: tc.failProductTool,
				responses: map[string]string{
					"im/list_messages_by_ids":      tc.mget,
					"im/get_resource_download_url": tc.info,
				},
			}
			helpers.InitDeps(caller)
			root := newPlatformCoverageRoot()
			args := append([]string{}, baseArgs...)
			if tc.outputDir != "" {
				args = append(args, "--output-dir", tc.outputDir)
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCrossPlatformCoverageMgetDownloadRunsWithoutConfirmation(t *testing.T) {
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
		return 7, nil
	}
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_messages_by_ids":      `{"result":[{"openMessageId":"msg","openConversationId":"cid","content":"{\"mediaId\":\"@file\"}"}]}`,
		"im/get_resource_download_url": `{"result":{"resourceUrl":"https://download.dingtalk.com/resource.bin"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetIn(bytes.NewBuffer(nil))
	root.SetArgs([]string{
		"chat", "+messages-mget",
		"--msg-ids", "msg",
		"--download-resources",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 ||
		fake.calls[0].tool != "list_messages_by_ids" ||
		fake.calls[1].tool != "get_resource_download_url" {
		t.Fatalf("download calls = %#v", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	ledger, _ := payload["resourceDownloads"].(map[string]any)
	if ledger["downloadedCount"] != float64(1) || ledger["failedCount"] != float64(0) {
		t.Fatalf("download ledger = %#v", ledger)
	}
}
