// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func testPNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "photo.png")
	bitmap := image.NewRGBA(image.Rect(0, 0, 120, 80))
	bitmap.Set(0, 0, color.RGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, bitmap); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCrossPlatformCoverageInsertImageUploadAppendAndReadback(t *testing.T) {
	file := testPNG(t)
	receiptDir := t.TempDir()
	testseam.Swap(t, &newImageNodeID, func() string { return "fixed" })
	testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
	putCount := 0
	helpers.SetHTTPPutFile(func(_ context.Context, url string, headers map[string]string, path string, size int64) error {
		putCount++
		if url != "https://upload.example.test/token" || headers["Content-Type"] != "image/png" || filepath.Base(path) != filepath.Base(file) || path == file || size <= 0 {
			t.Fatalf("unexpected PUT: %q %#v %q %d", url, headers, path, size)
		}
		original, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, snapshot) {
			t.Fatal("private upload snapshot differs from validated image")
		}
		return nil
	})
	t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
	readback := `[{"id":"real-image","type":"image","x":10,"y":20,"width":60,"height":40,"resizeMode":"fixedAspectRatio","intrinsicWidth":120,"intrinsicHeight":80,"resource":{"kind":"managed","resourceId":"res-1","url":"https://resource.example.test/image.png"}}]`
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery:                        {validWhiteboardQueryResponse(`[]`), validWhiteboardQueryResponse(`[]`), validWhiteboardQueryResponse(readback)},
		"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
		toolUpdate:                       {validWhiteboardUpdateResponse("append", []string{"image-fixed"}, []string{"real-image"}, 0)},
	}}
	raw, err := runWhiteboardCoverageOutput(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--x", "10", "--y", "20", "--width", "60", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if putCount != 1 || len(caller.calls) != 5 {
		t.Fatalf("PUT=%d calls=%#v", putCount, caller.calls)
	}
	result := decodeWhiteboardBusinessData(t, raw)
	if result["uploaded"] != true || result["committed"] != true || result["verified"] != true {
		t.Fatalf("result=%#v", result)
	}
	path, ok := result["receiptFile"].(string)
	if !ok {
		t.Fatalf("missing receipt: %#v", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["committed"] != true || receipt["verified"] != true {
		t.Fatalf("receipt=%#v", receipt)
	}
}

func TestCrossPlatformCoverageInsertImageStandaloneUploadAppendAndReadback(t *testing.T) {
	file := testPNG(t)
	testseam.Swap(t, &newImageNodeID, func() string { return "fixed" })
	testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return t.TempDir(), nil })
	helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
	t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
	readback := `[{"id":"real-image","type":"image","x":0,"y":0,"width":120,"height":80,"resizeMode":"fixedAspectRatio","intrinsicWidth":120,"intrinsicHeight":80,"resource":{"kind":"managed","resourceId":"res-1","url":"https://resource.example.test/image.png?resourceId=res-1"}}]`
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQueryStandalone:              {validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`), validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`), validStandaloneWhiteboardQueryResponse("wb", "page", 4, readback)},
		"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
		toolUpdateStandalone:             {validStandaloneWhiteboardUpdateResponse("wb", "append", 3, 4, []string{"image-fixed"}, []string{"real-image"}, 0)},
	}}
	raw, err := runWhiteboardCoverageOutput(t, InsertImage, caller, "", "--node", "wb", "--page-id", "page-1", "--file", file, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	var submitted []map[string]any
	for _, call := range caller.calls {
		if call.tool != toolUpdateStandalone {
			continue
		}
		if err := json.Unmarshal([]byte(call.args["nodes"].(string)), &submitted); err != nil {
			t.Fatal(err)
		}
	}
	if len(submitted) != 1 || submitted[0]["type"] != "image" {
		t.Fatalf("submitted image nodes=%#v", submitted)
	}
	if _, exists := submitted[0]["resizeMode"]; exists {
		t.Fatalf("read-only resizeMode leaked into image update: %#v", submitted[0])
	}
	result := decodeWhiteboardBusinessData(t, raw)
	if result["uploaded"] != true || result["committed"] != true || result["verified"] != true {
		t.Fatalf("result=%#v", result)
	}
}

func TestCrossPlatformCoverageManagedResourceReadbackRejectsDifferentResourceID(t *testing.T) {
	expected := map[string]any{"kind": "managed", "resourceId": "res-1", "url": "/core/api/resources/img/asset"}
	actual := map[string]any{"kind": "managed", "resourceId": "res-1", "url": "/core/api/resources/img/asset?resourceId=res-2"}
	if err := requireRequestedValue(expected, actual, "node image.resource"); err == nil {
		t.Fatal("accepted readback URL with a different resource ID")
	}
}

func TestCrossPlatformCoverageManagedResourceReadbackAcceptsExistingQuery(t *testing.T) {
	expected := map[string]any{"kind": "managed", "resourceId": "res-1", "url": "/core/api/resources/img/asset?size=large"}
	actual := map[string]any{"kind": "managed", "resourceId": "res-1", "url": "/core/api/resources/img/asset?size=large&resourceId=res-1"}
	if err := requireRequestedValue(expected, actual, "node image.resource"); err != nil {
		t.Fatalf("managed resource readback with query was rejected: %v", err)
	}
}

func TestCrossPlatformCoverageInsertImageStopsAfterUploadWhenTargetChanges(t *testing.T) {
	file := testPNG(t)
	receiptDir := t.TempDir()
	testseam.Swap(t, &imageReceiptDirectory, func() (string, error) { return receiptDir, nil })
	putCount := 0
	helpers.SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { putCount++; return nil })
	t.Cleanup(func() { helpers.SetHTTPPutFile(nil) })
	caller := &whiteboardCoverageCaller{responses: map[string][]string{
		toolQuery:                        {validWhiteboardQueryResponse(`[]`), validWhiteboardQueryResponse(`[{"id":"other","type":"text","text":"changed"}]`)},
		"get_doc_attachment_upload_info": {`{"uploadUrl":"https://upload.example.test/token","resourceId":"res-1","resourceUrl":"https://resource.example.test/image.png"}`},
	}}
	err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", file, "--yes")
	if err == nil || !strings.Contains(err.Error(), "预览后发生变化") {
		t.Fatalf("expected stale target error, got %v", err)
	}
	if putCount != 1 || len(caller.calls) != 3 {
		t.Fatalf("PUT=%d calls=%#v", putCount, caller.calls)
	}
	entries, err := os.ReadDir(receiptDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("receipt entries=%v err=%v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(receiptDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["uploaded"] != true || receipt["committed"] != false || receipt["verified"] != false {
		t.Fatalf("receipt=%#v", receipt)
	}
}

func TestCrossPlatformCoverageInsertImageDryRunPreviewsTargetWithoutUploading(t *testing.T) {
	file := testPNG(t)
	for _, tc := range []struct {
		name     string
		tool     string
		response string
		flags    []string
	}{
		{"embedded", toolQuery, validWhiteboardQueryResponse(`[]`), []string{"--node", "doc", "--part-id", "part"}},
		{"standalone", toolQueryStandalone, validStandaloneWhiteboardQueryResponse("wb", "page", 3, `[]`), []string{"--node", "wb", "--page-id", "page-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{dry: true, responses: map[string][]string{tc.tool: {tc.response}}}
			args := append(append([]string{}, tc.flags...), "--file", file, "--x", "10", "--y", "20", "--width", "60", "--dry-run")
			raw, err := runWhiteboardCoverageOutput(t, InsertImage, caller, "", args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 || caller.calls[0].tool != tc.tool {
				t.Fatalf("dry-run calls=%#v", caller.calls)
			}
			data := decodeWhiteboardBusinessData(t, raw)
			if data["uploaded"] != false || data["committed"] != false || data["verified"] != false || data["dryRun"] != true {
				t.Fatalf("dry-run result=%#v", data)
			}
			preview := data["preview"].(map[string]any)
			if preview["width"] != float64(60) || preview["height"] != float64(40) {
				t.Fatalf("display size=%#v", preview)
			}
		})
	}
}

func TestCrossPlatformCoverageInsertImageRejectsInvalidFileAndAspectBeforeRemoteCall(t *testing.T) {
	file := testPNG(t)
	for _, args := range [][]string{
		{"--node", "doc", "--part-id", "part", "--file", file, "--width", "60", "--height", "60", "--dry-run"},
		{"--node", "doc", "--part-id", "part", "--page-id", "page", "--file", file, "--dry-run"},
	} {
		caller := &whiteboardCoverageCaller{dry: true}
		if err := runWhiteboardCoverage(t, InsertImage, caller, "", args...); err == nil {
			t.Fatalf("accepted invalid args %v", args)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("remote call before local validation: %#v", caller.calls)
		}
	}
	bad := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(bad, []byte("not a PNG"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectLocalImage(bad); err == nil || !strings.Contains(err.Error(), "PNG") {
		t.Fatalf("invalid content accepted: %v", err)
	}
}

func TestCrossPlatformCoverageInsertImageClassifiesDegenerateAndOversizedDisplay(t *testing.T) {
	makeExtremePNG := func(name string, width, height int) string {
		path := filepath.Join(t.TempDir(), name)
		var data bytes.Buffer
		if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	landscape := makeExtremePNG("landscape.png", maxImageSide, 1)
	portrait := makeExtremePNG("portrait.png", 1, maxImageSide)
	for _, tc := range []struct {
		name, file, dimension, size, want string
	}{
		{"width-derives-zero-height", landscape, "--width", "1", "退化为 0"},
		{"height-derives-zero-width", portrait, "--height", "1", "退化为 0"},
		{"width-derives-oversized-height", portrait, "--width", "2", "按原图比例计算的显示尺寸超出范围"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{dry: true}
			err := runWhiteboardCoverage(t, InsertImage, caller, "", "--node", "doc", "--part-id", "part", "--file", tc.file, tc.dimension, tc.size, "--dry-run")
			if err == nil || apperrors.ExitCode(err) != 3 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("size validation = %v, want exit 3 and %q", err, tc.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid size reached remote call: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageImageEXIFOrientation(t *testing.T) {
	makeTIFF := func(little bool, orientation byte) []byte {
		if little {
			return []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, orientation, 0, 0, 0}
		}
		return []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 1, 0x12, 0, 3, 0, 0, 0, 1, 0, orientation, 0, 0}
	}
	withEXIF := func(tiff []byte) []byte {
		segment := append([]byte("Exif\x00\x00"), tiff...)
		length := len(segment) + 2
		return append([]byte{0xff, 0xd8, 0xff, 0xe1, byte(length >> 8), byte(length)}, segment...)
	}
	for _, tc := range []struct {
		name string
		data []byte
		bad  bool
	}{
		{"little-default", withEXIF(makeTIFF(true, 1)), false},
		{"little-rotated", withEXIF(makeTIFF(true, 6)), true},
		{"big-default", withEXIF(makeTIFF(false, 1)), false},
		{"big-rotated", withEXIF(makeTIFF(false, 6)), true},
		{"short-tiff", withEXIF([]byte{'I'}), true},
		{"unknown-byte-order", withEXIF([]byte{'X', 'X', 0, 42, 0, 0, 0, 8}), true},
		{"invalid-magic", withEXIF([]byte{'I', 'I', 1, 0, 8, 0, 0, 0}), true},
		{"invalid-offset", withEXIF([]byte{'I', 'I', 42, 0, 255, 0, 0, 0}), true},
		{"truncated-directory", withEXIF([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}), true},
		{"no-orientation-tag", withEXIF([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 0, 0}), false},
		{"no-exif", []byte{0xff, 0xd8, 0xff, 0xe0, 0, 2}, false},
		{"truncated-segment", []byte{0xff, 0xd8, 0xff, 0xe1, 0, 20, 'E'}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNonDefaultEXIFOrientation(tc.data); got != tc.bad {
				t.Fatalf("hasNonDefaultEXIFOrientation=%v, want %v", got, tc.bad)
			}
		})
	}
	bitmap := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, bitmap, nil); err != nil {
		t.Fatal(err)
	}
	imageBytes := encoded.Bytes()
	rotated := append(withEXIF(makeTIFF(true, 6)), imageBytes[2:]...)
	path := filepath.Join(t.TempDir(), "rotated.jpg")
	if err := os.WriteFile(path, rotated, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectLocalImage(path); err == nil || !strings.Contains(err.Error(), "EXIF") {
		t.Fatalf("rotated JPEG accepted: %v", err)
	}
}
