// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageConversationFileShortcutWrappersReuseNativeFlow(t *testing.T) {
	file := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := BuildConversationLocalFileMeta(file, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if meta.FileName != "report.txt" || meta.FileSize != 7 {
		t.Fatalf("meta = %#v", meta)
	}
	content, err := BuildConversationFileContent(11, 22, meta)
	if err != nil || !strings.Contains(content, `"dentryId":11`) {
		t.Fatalf("content = %q, %v", content, err)
	}
	dentryID, spaceID, err := ParseConversationFileSendIDs(`{"result":{"dentryId":11,"spaceId":22}}`)
	if err != nil || dentryID != 11 || spaceID != 22 {
		t.Fatalf("ids = %d/%d, %v", dentryID, spaceID, err)
	}

	oldPut := httpPutFile
	t.Cleanup(func() { httpPutFile = oldPut })
	var putPath string
	httpPutFile = func(_ context.Context, _ string, _ map[string]string, localPath string, size int64) error {
		putPath = localPath
		if size != 7 {
			t.Errorf("upload size = %d", size)
		}
		return nil
	}
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"resourceUrl":"https://upload.example.test/file","uploadKey":"key"}`},
		{text: `{"result":{"dentryId":11,"spaceId":22}}`},
	}}
	installScriptedCaller(t, caller)
	commit, err := UploadConversationLocalFile(
		context.Background(),
		map[string]any{"openConversationId": "cid"},
		meta,
		"uuid",
	)
	if err != nil || !strings.Contains(commit, `"dentryId":11`) || putPath != file || caller.calls != 2 {
		t.Fatalf("upload = %q, path=%q calls=%d err=%v", commit, putPath, caller.calls, err)
	}
}

func TestCrossPlatformCoverageReadToolNameContractAndHelperBoundary(t *testing.T) {
	for tool, want := range map[string]bool{
		" get_conversation ":               true,
		"LIST_MESSAGES":                    true,
		"query_send_status":                true,
		"search_messages":                  true,
		"unread_message_conversation_list": true,
		"send_personal_message":            false,
		"":                                 false,
	} {
		if got := IsReadToolName(tool); got != want {
			t.Errorf("IsReadToolName(%q) = %v, want %v", tool, got, want)
		}
	}

	caller := &helpersReadCaller{
		helpersCoreCaller: &helpersCoreCaller{format: "json", dry: true},
		readResult:        textToolResult(`{"success":true}`),
	}
	installHelpersCoreDeps(t, caller)
	if _, err := CallMCPReadToolTextOnServer("chat", "send_personal_message", nil); err == nil {
		t.Fatal("write tool was accepted by the read helper boundary")
	}
	if caller.readCalls != 0 || caller.calls != 0 {
		t.Fatalf("rejected write reached caller: read=%d regular=%d", caller.readCalls, caller.calls)
	}
}
