// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

func TestCrossPlatformCoverageInsertImageRejectsFileChangesAcrossPreviewAndUpload(t *testing.T) {
	for _, phase := range []string{"after-preview", "removed-after-preview", "during-upload"} {
		t.Run(phase, func(t *testing.T) {
			file := testPNG(t)
			receiptDir := t.TempDir()
			testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
			changeFile := func() {
				if phase == "removed-after-preview" {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
					return
				}
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, append(data, 'x'), 0600); err != nil {
					t.Fatal(err)
				}
			}
			caller := &whiteboardCoverageCaller{responses: map[string][]string{
				toolQuery:                        {validWhiteboardQueryResponse(`[]`)},
				"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
			}}
			if phase == "after-preview" || phase == "removed-after-preview" {
				caller.onCall = func(tool string) {
					if tool == toolQuery {
						changeFile()
					}
				}
			} else {
				caller.responses[toolQuery] = append(caller.responses[toolQuery], validWhiteboardQueryResponse(`[]`))
			}
			helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
				if phase == "during-upload" {
					changeFile()
				}
				return nil
			})
			t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
			err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--yes")
			if err == nil || phase != "removed-after-preview" && !strings.Contains(err.Error(), "图片在") {
				t.Fatalf("changed image accepted: %v", err)
			}
			for _, call := range caller.calls {
				if call.tool == toolUpdate {
					t.Fatal("changed image reached whiteboard mutation")
				}
			}
			entries, err := os.ReadDir(receiptDir)
			if err != nil {
				t.Fatal(err)
			}
			if phase != "during-upload" && len(entries) != 0 {
				t.Fatal("uploaded before rechecking preview image")
			}
			if phase == "during-upload" && len(entries) != 1 {
				t.Fatal("lost uploaded resource receipt")
			}
		})
	}
}

func TestCrossPlatformCoverageInsertImageUploadFailureStopsBeforeMutation(t *testing.T) {
	file := testPNG(t)
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery:                        {validWhiteboardQueryResponse(`[]`)},
		"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
	}}
	helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
		return errors.New("upload interrupted")
	})
	t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
	if err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--yes"); err == nil {
		t.Fatal("failed upload accepted")
	}
	for _, call := range caller.calls {
		if call.tool == toolUpdate {
			t.Fatal("failed upload reached whiteboard mutation")
		}
	}
}

func TestCrossPlatformCoverageInsertImageStopsWhenPrivateStagingUnavailable(t *testing.T) {
	file := testPNG(t)
	missing := filepath.Join(t.TempDir(), "missing-staging-parent")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, missing)
	}
	caller := &whiteboardCoverageCaller{responses: map[string][]string{toolQuery: {validWhiteboardQueryResponse(`[]`)}}}
	err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--yes")
	if err == nil || len(caller.calls) != 1 || caller.calls[0].tool != toolQuery {
		t.Fatalf("staging failure reached upload: %v, calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageInsertImageReceiptFailurePreservesRecoveryIdentity(t *testing.T) {
	file := testPNG(t)
	testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return "", errors.New("config directory unavailable") })
	uploads := 0
	helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
		uploads++
		return nil
	})
	t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery:                        {validWhiteboardQueryResponse(`[]`)},
		"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
	}}
	err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--yes")
	if err == nil || !strings.Contains(err.Error(), "图片资源已上传") || !strings.Contains(err.Error(), "res-1") || uploads != 1 {
		t.Fatalf("upload without receipt lost recovery identity: %v, uploads=%d", err, uploads)
	}
	for _, call := range caller.calls {
		if call.tool == toolUpdate {
			t.Fatal("receipt failure reached whiteboard mutation")
		}
	}
}

func TestCrossPlatformCoverageInsertImageDirectPreflightGuards(t *testing.T) {
	file := testPNG(t)
	for _, args := range [][]string{
		{"--node", "doc", "--part-id", "", "--file", file},
		{"--node", "doc", "--page-id", "", "--file", file},
	} {
		rt := directWhiteboardRuntime(t, InsertImage, &whiteboardCoverageCaller{}, args...)
		if err := preflightInsertImage(rt); err == nil {
			t.Fatalf("accepted empty explicit target: %v", args)
		}
	}
	rt := directWhiteboardRuntime(t, InsertImage, &whiteboardCoverageCaller{}, "--node", "doc", "--part-id", "part", "--file", file)
	if err := executeInsertImage(rt); err == nil {
		t.Fatal("execution accepted absent preflight state")
	}
	rt = directWhiteboardRuntime(t, InsertImage, &whiteboardCoverageCaller{}, "--node", "", "--part-id", "part", "--file", file)
	if err := preflightInsertImage(rt); err == nil {
		t.Fatal("query accepted empty node target")
	}
}

func TestCrossPlatformCoverageInsertImageRejectsNonComparableQueryBeforeUpload(t *testing.T) {
	pages := map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{
		map[string]any{"id": "page-1", "nodes": []any{}},
		map[string]any{"id": "page-2", "nodes": []any{}},
	}}
	encoded, err := json.Marshal(pages)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, tool, response string
		args                 []string
	}{
		{"embedded-multiple-pages", toolQuery, `{"success":true,"resultJson":` + mustJSONString(t, string(encoded)) + `,"resultSummary":{"nodeCount":0,"pageCount":2,"readOnlyNodeCount":0,"unknownNodeCount":0,"resultBytes":100,"resultSha256":"digest"}}`, []string{"--node", "doc", "--part-id", "part"}},
		{"standalone-download-only", toolQueryStandalone, `{"success":true,"nodeId":"wb","revision":3,"view":"page","resultDownloadUrl":"https://resource.example.test/snapshot","resultSummary":{"nodeCount":0}}`, []string{"--node", "wb", "--page-id", "page-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{tc.tool: {tc.response}}}
			args := append(append([]string{}, tc.args...), "--file", testPNG(t), "--yes")
			if err := runWhiteboardCoverage(t, InsertImage, caller, "", args...); err == nil {
				t.Fatal("non-comparable snapshot accepted")
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != tc.tool {
				t.Fatalf("remote calls after invalid snapshot: %#v", caller.calls)
			}
		})
	}
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCrossPlatformCoverageInsertImageDryRunRejectsOtherRoute(t *testing.T) {
	caller := &whiteboardCoverageCaller{dry: true}
	rt := directWhiteboardRuntime(t, InsertImage, caller, "--node", "doc", "--part-id", "part")
	rt.Command().Flags().Bool("dry-run", true, "")
	_, err := readInsertImageTarget(rt, whiteboardcore.Call{Tool: "write_whiteboard"})
	if err == nil || len(caller.calls) != 0 {
		t.Fatalf("dry-run accepted non-query route: %v, calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageInsertImageDryRunRejectsEmptyReadResult(t *testing.T) {
	caller := &whiteboardCoverageCaller{dry: true, responses: map[string][]string{toolQuery: {"null"}}}
	err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", testPNG(t), "--dry-run")
	if err == nil || len(caller.calls) != 1 || caller.calls[0].tool != toolQuery {
		t.Fatalf("empty dry-run read result reached write route: %v, calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageInsertImagePostUploadSnapshotFailuresKeepReceipt(t *testing.T) {
	twoPageSource, err := json.Marshal(map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{
		map[string]any{"id": "page-1", "nodes": []any{}}, map[string]any{"id": "page-2", "nodes": []any{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	twoPageResponse, err := json.Marshal(map[string]any{"success": true, "resultJson": string(twoPageSource), "resultSummary": map[string]any{
		"nodeCount": 0, "pageCount": 2, "readOnlyNodeCount": 0, "unknownNodeCount": 0,
		"resultBytes": len(twoPageSource), "resultSha256": "0123456789abcdef",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind, second string
	}{
		{"embedded-query-error", "embedded", ""},
		{"embedded-snapshot-invalid", "embedded", `{"success":true}`},
		{"embedded-page-count-changed", "embedded", string(twoPageResponse)},
		{"standalone-revision-changed", "standalone", validStandaloneWhiteboardQueryResponse("wb", "page", 4, `[]`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receiptDir := t.TempDir()
			testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
			helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
			t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
			queryTool, args, first := toolQuery, []string{"--node", "doc", "--part-id", "part"}, validWhiteboardQueryResponse(`[]`)
			if tc.kind == "standalone" {
				queryTool, args, first = toolQueryStandalone, []string{"--node", "wb", "--page-id", "page-1"}, validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`)
			}
			caller := &whiteboardCoverageCaller{responses: map[string][]string{
				queryTool:                        {first, tc.second},
				"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
			}}
			args = append(args, "--file", testPNG(t), "--yes")
			err := runWhiteboardCoverage(t, InsertImage, caller, "", args...)
			if err == nil || !strings.Contains(err.Error(), "图片资源已上传") {
				t.Fatalf("uncertain post-upload state accepted: %v", err)
			}
			entries, err := os.ReadDir(receiptDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("resource receipt lost: %#v, %v", entries, err)
			}
			for _, call := range caller.calls {
				if call.tool == toolUpdate || call.tool == toolUpdateStandalone {
					t.Fatal("wrote after stale post-upload snapshot")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageImageReceiptDefaultDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	dir, err := imageReceiptDirectory()
	if err != nil || !strings.Contains(dir, "whiteboard-image-receipts") {
		t.Fatalf("default receipt directory = %q, %v", dir, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("APPDATA", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := imageReceiptDirectory(); err == nil {
		t.Fatal("missing home unexpectedly yielded a receipt directory")
	}
}

func TestCrossPlatformCoverageInsertImageLocalFailuresStopRemoteCalls(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.png")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	wrongExtension := filepath.Join(dir, "photo.jpg")
	data, err := os.ReadFile(testPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrongExtension, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		file string
		args []string
	}{
		{"missing-file", filepath.Join(dir, "missing.png"), nil},
		{"directory", dir, nil},
		{"empty-file", empty, nil},
		{"wrong-extension", wrongExtension, nil},
		{"negative-width", testPNG(t), []string{"--width", "-1"}},
		{"excessive-height", testPNG(t), []string{"--height", "32769"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{}
			args := append([]string{"--node", "doc", "--part-id", "part", "--file", tc.file}, tc.args...)
			if err := runWhiteboardCoverage(t, InsertImage, caller, "", args...); err == nil {
				t.Fatal("invalid local input was accepted")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("local validation reached remote services: %#v", caller.calls)
			}
		})
	}

	jpegPath := filepath.Join(dir, "photo.jpeg")
	f, err := os.Create(jpegPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 12, 8)), nil); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	inspected, err := inspectLocalImage(jpegPath)
	if err != nil || inspected.mimeType != "image/jpeg" || inspected.intrinsicWidth != 12 {
		t.Fatalf("valid JPEG inspection: %#v, %v", inspected, err)
	}
	width, height, err := imageDisplaySize(120, 80, 0, 40, false, true)
	if err != nil || width != 60 || height != 40 {
		t.Fatalf("height-only scaling = %dx%d, %v", width, height, err)
	}
	if _, _, err := imageDisplaySize(1, 32768, 32768, 0, true, false); err == nil {
		t.Fatal("accepted derived size exceeding the supported bound")
	}
}

func TestCrossPlatformCoverageInsertImageRejectsOversizedPNGHeader(t *testing.T) {
	file := testPNG(t)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// PNG DecodeConfig reads IHDR without allocating the advertised pixel count.
	binary.BigEndian.PutUint32(data[16:20], uint32(maxImageSide+1))
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	caller := &whiteboardCoverageCaller{}
	err = runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--yes")
	if err == nil || !strings.Contains(err.Error(), "像素尺寸") || len(caller.calls) != 0 {
		t.Fatalf("oversized image reached remote call: %v, calls=%#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageInsertImageQueryFailurePreventsUpload(t *testing.T) {
	for _, response := range []string{"", `[]`, `{}`, `{"source":null}`} {
		t.Run(response, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{toolQuery: {response}}}
			err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", testPNG(t), "--yes")
			if err == nil {
				t.Fatal("invalid query response was accepted")
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != toolQuery {
				t.Fatalf("query failure reached upload or mutation: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageInsertImageWriteAndVerificationFailuresKeepReceipts(t *testing.T) {
	for _, kind := range []string{"embedded", "standalone"} {
		for _, failure := range []string{"write-error", "invalid-receipt", "readback-error", "invalid-readback", "missing-written-node", "revision-mismatch"} {
			if kind == "embedded" && failure == "revision-mismatch" {
				continue
			}
			t.Run(kind+"/"+failure, func(t *testing.T) {
				receiptDir := t.TempDir()
				testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
				testseam.Swap(t, &newImageNodeID, func() string { return "fixed" })
				helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
				t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
				queryTool, updateTool := toolQuery, toolUpdate
				initial := validWhiteboardQueryResponse(`[]`)
				update := validWhiteboardUpdateResponse("append", []string{"image-fixed"}, []string{"real-image"}, 0)
				args := []string{"--node", "doc", "--part-id", "part"}
				if kind == "standalone" {
					queryTool, updateTool = toolQueryStandalone, toolUpdateStandalone
					initial = validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`)
					update = validStandaloneWhiteboardUpdateResponse("wb", "append", 3, 4, []string{"image-fixed"}, []string{"real-image"}, 0)
					args = []string{"--node", "wb", "--page-id", "page-1"}
				}
				responses := map[string][]string{
					queryTool:                        {initial, initial},
					"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
				}
				if failure != "write-error" {
					responses[updateTool] = []string{update}
				}
				switch failure {
				case "invalid-receipt":
					if kind == "standalone" {
						responses[updateTool] = []string{`{"success":true,"nodeId":"wb","mode":"append"}`}
					} else {
						responses[updateTool] = []string{`{"success":true,"nodeId":"doc","partId":"part","resultJson":"{}"}`}
					}
				case "invalid-readback":
					responses[queryTool] = append(responses[queryTool], `{}`)
				case "missing-written-node", "revision-mismatch":
					readback := initial
					if kind == "standalone" && failure == "missing-written-node" {
						readback = validStandaloneWhiteboardQueryResponse("wb", "page", 4, `[]`)
					}
					responses[queryTool] = append(responses[queryTool], readback)
				}
				caller := &whiteboardCoverageCaller{responses: responses}
				args = append(args, "--file", testPNG(t), "--yes")
				err := runWhiteboardCoverage(t, InsertImage, caller, "", args...)
				var recoveryError *apperrors.Error
				if err == nil || !strings.Contains(err.Error(), "图片资源已上传") || !errors.As(err, &recoveryError) || !strings.Contains(recoveryError.Hint, "资源回执") {
					t.Fatalf("failure did not return recovery guidance: %v", err)
				}
				entries, err := os.ReadDir(receiptDir)
				if err != nil || len(entries) != 1 {
					t.Fatalf("recovery receipt missing: %v, %v", entries, err)
				}
				data, err := os.ReadFile(filepath.Join(receiptDir, entries[0].Name()))
				if err != nil {
					t.Fatal(err)
				}
				var receipt map[string]any
				if err := json.Unmarshal(data, &receipt); err != nil {
					t.Fatal(err)
				}
				committed := failure != "write-error" && failure != "invalid-receipt"
				if receipt["uploaded"] != true || receipt["committed"] != committed || receipt["verified"] != false {
					t.Fatalf("receipt lost the last confirmed state: %#v", receipt)
				}
				writes := 0
				for _, call := range caller.calls {
					if call.tool == updateTool {
						writes++
					}
				}
				if writes != 1 {
					t.Fatalf("mutation retried after uncertain result: %d writes", writes)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageInsertImageStopsWhenReceiptDisappearsAfterCommit(t *testing.T) {
	for _, kind := range []string{"embedded", "standalone"} {
		t.Run(kind, func(t *testing.T) {
			receiptDir := t.TempDir()
			testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
			testseam.Swap(t, &newImageNodeID, func() string { return "fixed" })
			helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
			t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
			queryTool, updateTool := toolQuery, toolUpdate
			initial := validWhiteboardQueryResponse(`[]`)
			update := validWhiteboardUpdateResponse("append", []string{"image-fixed"}, []string{"real-image"}, 0)
			args := []string{"--node", "doc", "--part-id", "part"}
			if kind == "standalone" {
				queryTool, updateTool = toolQueryStandalone, toolUpdateStandalone
				initial = validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`)
				update = validStandaloneWhiteboardUpdateResponse("wb", "append", 3, 4, []string{"image-fixed"}, []string{"real-image"}, 0)
				args = []string{"--node", "wb", "--page-id", "page-1"}
			}
			caller := &whiteboardCoverageCaller{responses: map[string][]string{
				queryTool:                        {initial, initial},
				"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
				updateTool:                       {update},
			}}
			caller.onCall = func(tool string) {
				if tool != updateTool {
					return
				}
				entries, err := os.ReadDir(receiptDir)
				if err != nil || len(entries) != 1 {
					t.Fatalf("missing uploaded receipt before commit: %#v, %v", entries, err)
				}
				if err := os.Remove(filepath.Join(receiptDir, entries[0].Name())); err != nil {
					t.Fatal(err)
				}
			}
			args = append(args, "--file", testPNG(t), "--yes")
			err := runWhiteboardCoverage(t, InsertImage, caller, "", args...)
			if err == nil || !strings.Contains(err.Error(), "图片资源已上传") {
				t.Fatalf("lost receipt after remote commit was accepted: %v", err)
			}
			writes := 0
			for _, call := range caller.calls {
				if call.tool == updateTool {
					writes++
				}
			}
			if writes != 1 {
				t.Fatalf("uncertain commit retried: %d writes", writes)
			}
		})
	}
}

func TestCrossPlatformCoverageInsertImageStopsWhenReceiptDisappearsBeforeVerificationFlag(t *testing.T) {
	receiptDir := t.TempDir()
	testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
	testseam.Swap(t, &newImageNodeID, func() string { return "fixed" })
	helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
	t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
	readback := `[{"id":"real-image","type":"image","x":0,"y":0,"width":120,"height":80,"resizeMode":"fixedAspectRatio","intrinsicWidth":120,"intrinsicHeight":80,"resource":{"kind":"managed","resourceId":"res-1","url":"https://resource.example.test/image.png"}}]`
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery:                        {validWhiteboardQueryResponse(`[]`), validWhiteboardQueryResponse(`[]`), validWhiteboardQueryResponse(readback)},
		"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
		toolUpdate:                       {validWhiteboardUpdateResponse("append", []string{"image-fixed"}, []string{"real-image"}, 0)},
	}}
	queries := 0
	caller.onCall = func(tool string) {
		if tool != toolQuery {
			return
		}
		queries++
		if queries != 3 {
			return
		}
		entries, err := os.ReadDir(receiptDir)
		if err != nil || len(entries) != 1 {
			t.Fatalf("expected committed receipt before readback: %#v, %v", entries, err)
		}
		data, err := os.ReadFile(filepath.Join(receiptDir, entries[0].Name()))
		if err != nil {
			t.Fatal(err)
		}
		var receipt map[string]any
		if err := json.Unmarshal(data, &receipt); err != nil || receipt["committed"] != true || receipt["verified"] != false {
			t.Fatalf("unexpected receipt before verification: %#v, %v", receipt, err)
		}
		if err := os.Remove(filepath.Join(receiptDir, entries[0].Name())); err != nil {
			t.Fatal(err)
		}
	}
	err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", testPNG(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "图片资源已上传") || queries != 3 {
		t.Fatalf("lost receipt after successful readback was accepted: %v, queries=%d", err, queries)
	}
}

func TestCrossPlatformCoverageImageReceiptCorruptionAndAtomicUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "receipt.json")
	if err := updateImageReceipt(path, true, false); err == nil {
		t.Fatal("missing receipt accepted")
	}
	for _, content := range []string{"invalid-json", "null", "[]"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := updateImageReceipt(path, true, false); err == nil {
			t.Fatalf("corrupt receipt accepted: %q", content)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != content {
			t.Fatalf("failed update modified the original receipt: %q, %v", data, err)
		}
	}
	if err := os.WriteFile(path, []byte(`{"uploaded":true,"committed":false,"resourceId":"res-1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := updateImageReceipt(path, true, false); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path + ".tmp"); err != nil || !info.IsDir() {
		t.Fatalf("unique temporary write disturbed existing sibling: %v, %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["resourceId"] != "res-1" || receipt["committed"] != true || receipt["verified"] != false {
		t.Fatalf("atomic update lost receipt fields: %#v", receipt)
	}

	rt := directWhiteboardRuntime(t, InsertImage, &whiteboardCoverageCaller{}, "--node", "doc", "--part-id", "part")
	testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return "", errors.New("receipt directory unavailable") })
	if _, err := persistImageReceipt(rt, &imagePreflight{}, nil); err == nil {
		t.Fatal("receipt directory failure accepted")
	}
}
