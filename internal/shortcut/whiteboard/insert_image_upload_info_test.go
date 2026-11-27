// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageInsertImageRejectsMalformedUploadInfoBeforeUploading(t *testing.T) {
	for _, kind := range []string{"embedded", "standalone"} {
		for _, failure := range []string{"missing-resource-id", "blank-resource-url"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				dir := t.TempDir()
				testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return dir, nil })
				uploads := 0
				helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
					uploads++
					return nil
				})
				t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
				queryTool := toolQuery
				initial := validWhiteboardQueryResponse(`[]`)
				args := []string{"--node", "doc", "--part-id", "part"}
				if kind == "standalone" {
					queryTool = toolQueryStandalone
					initial = validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`)
					args = []string{"--node", "wb", "--page-id", "page-1"}
				}
				resourceID, resourceURL := "res-1", "https://resource.example.test/image.png"
				if failure == "missing-resource-id" {
					resourceID = ""
				} else {
					resourceURL = " "
				}
				uploadInfo, err := json.Marshal(map[string]any{"uploadUrl": "https://upload.example.test/token", "resourceId": resourceID, "resourceUrl": resourceURL})
				if err != nil {
					t.Fatal(err)
				}
				caller := &whiteboardCoverageCaller{responses: map[string][]string{
					queryTool:                        {initial},
					"get_doc_attachment_upload_info": {string(uploadInfo)},
				}}
				args = append(args, "--file", testPNG(t), "--yes")
				err = runWhiteboardCoverage(t, InsertImage, caller, "", args...)
				if uploads != 0 || err == nil {
					t.Fatalf("invalid upload info reached image transfer: uploads=%d error=%v", uploads, err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("rejected upload invented a completed-resource receipt: %#v, %v", entries, err)
				}
				for _, call := range caller.calls {
					if call.tool == toolUpdate || call.tool == toolUpdateStandalone {
						t.Fatal("invalid uploaded resource reached board mutation")
					}
				}
			})
		}
	}
}
