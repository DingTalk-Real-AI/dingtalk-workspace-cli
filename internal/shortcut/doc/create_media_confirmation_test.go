// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageDocCreateMediaRequiresConfirmationBeforeCreateAndUpload(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("asset.txt", []byte("body"), 0600); err != nil {
		t.Fatal(err)
	}
	const resourceID = "12345678-1234-1234-1234-123456789012"
	caller := &docCoverageCaller{responses: map[string][]map[string]any{
		"get_document_content":           {{"markdown": "body"}},
		"get_doc_attachment_upload_info": {{"uploadUrl": "https://example.com/upload", "resourceId": resourceID, "resourceUrl": "/resources/known"}},
		"insert_document_block":          {{"blockId": "media"}},
		"list_document_blocks":           {{"blocks": []any{map[string]any{"blockId": "media", "jsonml": `["card",{"uuid":"media","resourceId":"12345678-1234-1234-1234-123456789012"}]`}}, "hasMore": false}},
	}}
	uploads := 0
	helpers.SwapHTTPPutFileForTest(t, func(_ context.Context, url string, _ map[string]string, path string, size int64) error {
		uploads++
		body, err := os.ReadFile(path)
		if err != nil || string(body) != "body" || size != 4 || url != "https://example.com/upload" {
			t.Fatalf("wrong upload: %q %d %q %v", url, size, body, err)
		}
		return nil
	})
	args := []string{"--name", "report", "--content", "body", "--folder", "folder-1", "--media-files", "asset.txt"}
	err := runDocCoverage(t, CreateWithMedia, caller, args...)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "confirmation_required" || caller.calls != 0 || uploads != 0 {
		t.Fatalf("unconfirmed creation: %v, RPCs=%d uploads=%d", err, caller.calls, uploads)
	}
	got := runDocCoverageEnvelope(t, CreateWithMedia, caller, append(args, "--dry-run")...)
	if caller.calls != 0 || uploads != 0 || got["data"].(map[string]any)["executed"] != false {
		t.Fatalf("preview wrote remotely: %#v", got)
	}
	got = runDocCoverageEnvelope(t, CreateWithMedia, caller, append(args, "--yes")...)
	var tools []string
	for _, c := range caller.history {
		tools = append(tools, c.tool)
	}
	wantTools := []string{"create_document", "get_document_content", "get_doc_attachment_upload_info", "insert_document_block", "list_document_blocks"}
	if !reflect.DeepEqual(tools, wantTools) || uploads != 1 {
		t.Fatalf("calls=%v uploads=%d", tools, uploads)
	}
	if !reflect.DeepEqual(caller.history[0].params, map[string]any{"name": "report", "markdown": "body", "folderId": "folder-1"}) {
		t.Fatalf("create params: %#v", caller.history[0].params)
	}
	wantUpload := map[string]any{"nodeId": "node-1", "fileName": "asset.txt", "fileSize": float64(4), "mimeType": "text/plain"}
	if !reflect.DeepEqual(caller.history[2].params, wantUpload) {
		t.Fatalf("upload params: %#v", caller.history[2].params)
	}
	data := got["data"].(map[string]any)
	media := data["media"].([]any)
	receipt := media[0].(map[string]any)["data"].(map[string]any)
	if data["nodeId"] != "node-1" || data["verified"] != true || receipt["resourceId"] != resourceID || receipt["blockId"] != "media" || receipt["verified"] != true {
		t.Fatalf("unverified creation receipt: %#v", got)
	}
}

func TestCrossPlatformCoverageDocPlainCreateCannotUploadMedia(t *testing.T) {
	caller := &docCoverageCaller{}
	err := runDocCoverage(t, Create, caller, "--media-files", "asset.txt", "--yes")
	if err == nil || !strings.Contains(err.Error(), "unknown flag") || caller.calls != 0 {
		t.Fatalf("plain create exposed media upload: %v", err)
	}
	err = runDocCoverage(t, CreateWithMedia, caller, "--media-files=", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--media-files 不能为空") || caller.calls != 0 {
		t.Fatalf("empty media selection: %v", err)
	}
}
