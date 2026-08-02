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
)

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
