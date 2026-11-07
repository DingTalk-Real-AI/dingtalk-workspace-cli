// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// oaImagePNGHeader / oaImageJPEGHeader 是让临时样本通过内容 magic bytes 校验的
// 真实文件头（非图片字节会被 loadImageUploadFile 拒绝）。
const (
	oaImagePNGHeader  = "\x89PNG\r\n\x1a\n"
	oaImageJPEGHeader = "\xff\xd8\xff"
)

func TestCrossPlatformCoverageOAImageUploadEndToEnd(t *testing.T) {
	const content = oaImagePNGHeader + "png-body-bytes"
	filePath, fileSize := writeOAAttachmentTempFile(t, "evidence.png", content)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2-media","bizType":"approval_image","imageType":"png","message":"图片上传成功。"}`},
	}}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}

	if caller.calls != 1 {
		t.Fatalf("MCP calls = %d, want 1", caller.calls)
	}
	if caller.serverLog[0] != "dingtalk-file" || caller.toolLog[0] != "upload_media" {
		t.Fatalf("call = %s/%s, want dingtalk-file/upload_media", caller.serverLog[0], caller.toolLog[0])
	}
	wantArgs := map[string]any{
		"content":   base64.StdEncoding.EncodeToString([]byte(content)),
		"imageType": "png",
		"bizType":   "approval_image",
	}
	gotArgs := caller.argsLog[0]
	if gotArgs["content"] != wantArgs["content"] || gotArgs["imageType"] != wantArgs["imageType"] || gotArgs["bizType"] != wantArgs["bizType"] {
		t.Fatalf("upload args = %#v, want %#v", gotArgs, wantArgs)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v", err)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" {
		t.Fatalf("unified envelope = %#v", envelope)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("unified data missing: %#v", envelope)
	}
	if data["mediaIdV2"] != "$v2-media" {
		t.Fatalf("data.mediaIdV2 = %#v, want $v2-media", data["mediaIdV2"])
	}
	if data["bizType"] != "approval_image" || data["imageType"] != "png" {
		t.Fatalf("data = %#v, want bizType approval_image / imageType png", data)
	}
	if data["size"] != float64(fileSize) {
		t.Fatalf("data.size = %#v, want %d", data["size"], fileSize)
	}
}

func TestCrossPlatformCoverageOAImageUploadKeepsAuxMediaIDV1(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.jpg", oaImageJPEGHeader+"jpeg-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV1":"@v1","mediaIdV2":"$v2$"}`},
	}}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v", err)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["mediaIdV2"] != "$v2$" || data["mediaIdV1"] != "@v1" {
		t.Fatalf("data = %#v, want mediaIdV2 $v2$ + mediaIdV1 @v1", data)
	}
}

func TestCrossPlatformCoverageOAImageUploadRejectsServerFailure(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":false,"errorCode":"invalidRequest.inputArgs.invalid","errorMsg":"不支持的业务场景：approval_image。当前仅支持：chat_emoticon（聊天表情）。"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err == nil {
		t.Fatalf("expected server failure error, got nil")
	}
	if !strings.Contains(err.Error(), "不支持的业务场景") || !strings.Contains(err.Error(), "invalidRequest.inputArgs.invalid") {
		t.Fatalf("error = %v, want server detail + errorCode", err)
	}
}

func TestCrossPlatformCoverageOAImageUploadRejectsMissingMediaIDV2(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV1":"@v1"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err == nil || !strings.Contains(err.Error(), "mediaIdV2") {
		t.Fatalf("error = %v, want missing mediaIdV2", err)
	}
}

func TestCrossPlatformCoverageOAImageUploadRejectsMalformedResponse(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `not-json`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err == nil || !strings.Contains(err.Error(), "无法解析") {
		t.Fatalf("error = %v, want parse failure", err)
	}
}

func TestCrossPlatformCoverageOAImageUploadValidatesLocalFile(t *testing.T) {
	oversized := oaImagePNGHeader + strings.Repeat("a", 2*1024*1024+1)
	oversizedPath, _ := writeOAAttachmentTempFile(t, "big.png", oversized)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"blank file", []string{"--file", "   "}, "不能为空"},
		{"missing file", []string{"--file", "no-such-dir/evidence.png"}, "无法读取"},
		{"directory", []string{"--file", mustOAImageTempDir(t)}, "目录"},
		{"unsupported extension", []string{"--file", mustWriteOAImageTempFile(t, "notes.txt", "text")}, "仅支持"},
		{"empty file", []string{"--file", mustWriteOAImageTempFile(t, "empty.png", "")}, "为空"},
		{"content mismatch", []string{"--file", mustWriteOAImageTempFile(t, "fake.png", "fake-png-bytes")}, "图片格式不符"},
		{"oversize compress fails", []string{"--file", oversizedPath}, "自动压缩失败"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
				{text: `{"success":true,"mediaIdV2":"$v2$"}`},
			}}
			args := append([]string{"approval", "image", "upload"}, test.args...)
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller, args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
			if caller.calls != 0 {
				t.Fatalf("MCP calls = %d, want 0 (local validation must reject first)", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageOAImageUploadCompressesOversize(t *testing.T) {
	oversized := oaImagePNGHeader + strings.Repeat("a", 2*1024*1024+1)
	filePath, _ := writeOAAttachmentTempFile(t, "big.png", oversized)
	testseam.Swap(t, &imageCompress, func(_ []byte, _ string) ([]byte, string, error) {
		return []byte("compressed-jpg-bytes"), "jpg", nil
	})

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2-compressed$"}`},
	}}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if got := caller.argsLog[0]["imageType"]; got != "jpg" {
		t.Fatalf("imageType = %#v, want jpg (compressed still image)", got)
	}
	wantContent := base64.StdEncoding.EncodeToString([]byte("compressed-jpg-bytes"))
	if got := caller.argsLog[0]["content"]; got != wantContent {
		t.Fatalf("content = %#v, want base64 of compressed bytes", got)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v", err)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["compressed"] != true || data["imageType"] != "jpg" || data["size"] != float64(len("compressed-jpg-bytes")) {
		t.Fatalf("data = %#v, want compressed=true / imageType=jpg / size of compressed bytes", data)
	}
}

func TestCrossPlatformCoverageOAImageUploadCompressesFailure(t *testing.T) {
	oversized := oaImagePNGHeader + strings.Repeat("a", 2*1024*1024+1)
	filePath, _ := writeOAAttachmentTempFile(t, "big.png", oversized)
	testseam.Swap(t, &imageCompress, func(_ []byte, _ string) ([]byte, string, error) {
		return nil, "", errOAImageUploadTestCompress
	})

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err == nil || !strings.Contains(err.Error(), "自动压缩失败") {
		t.Fatalf("error = %v, want 自动压缩失败", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0 (compression must succeed before upload)", caller.calls)
	}
}

var errOAImageUploadTestCompress = errors.New("mock compress failure")

// TestCrossPlatformCoverageOAImageUploadRejectsStillOversizeAfterCompress 覆盖
// 自动压缩后仍超过 MaxBytes 的拒绝分支。
func TestCrossPlatformCoverageOAImageUploadRejectsStillOversizeAfterCompress(t *testing.T) {
	oversized := oaImagePNGHeader + strings.Repeat("a", 2*1024*1024+1)
	filePath, _ := writeOAAttachmentTempFile(t, "big.png", oversized)
	testseam.Swap(t, &imageCompress, func(_ []byte, _ string) ([]byte, string, error) {
		return make([]byte, 2*1024*1024+1), "jpg", nil
	})

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err == nil || !strings.Contains(err.Error(), "自动压缩后仍超限") {
		t.Fatalf("error = %v, want 自动压缩后仍超限", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0 (still-oversize must reject before upload)", caller.calls)
	}
}

func TestCrossPlatformCoverageOAImageUploadExtensionCaseInsensitive(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "PHOTO.PNG", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	if _, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	); err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if got := caller.argsLog[0]["imageType"]; got != "png" {
		t.Fatalf("imageType = %#v, want png (lowercased extension)", got)
	}
}

func TestCrossPlatformCoverageOAImageUploadDryRunPlan(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", dry: true, steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", filePath,
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0 in dry-run", caller.calls)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v", err)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["preview_kind"] != "plan" || data["executed"] != false || data["operation"] != "image_upload" {
		t.Fatalf("dry-run data = %#v, want plan preview", data)
	}
	steps, _ := data["steps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("dry-run steps = %#v, want 1 planned step", steps)
	}
	step, _ := steps[0].(map[string]any)
	if step["tool"] != "dingtalk-file/upload_media" || step["status"] != "planned" {
		t.Fatalf("dry-run step = %#v, want dingtalk-file/upload_media planned", step)
	}
	args, _ := step["args"].(map[string]any)
	if args["bizType"] != "approval_image" || args["imageType"] != "png" {
		t.Fatalf("dry-run step args = %#v, want bizType approval_image / imageType png", args)
	}
}

func TestCrossPlatformCoverageOAImageUploadHelpSurface(t *testing.T) {
	caller := &scriptedToolCaller{format: "json"}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, "approval", "image", "upload", "--help")
	if err != nil {
		t.Fatalf("execute --help: %v", err)
	}
	for _, want := range []string{"DDPhotoField", "--file", "approval_image"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help output missing %q: %s", want, stdout)
		}
	}
}

// mustWriteOAImageTempFile 写临时文件，失败即终止测试；返回绝对路径。
func mustWriteOAImageTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path, _ := writeOAAttachmentTempFile(t, name, content)
	return path
}

// mustOAImageTempDir 建一个以 .png 结尾的目录（绕过扩展名校验后命中目录分支）。
func mustOAImageTempDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "fake.png")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

// TestCrossPlatformCoverageOAImageUploadContentMagicBytes 表驱动校验各格式的
// 内容 magic bytes 判定：改扩展名的非图片字节必须在直传路径被拒绝。
func TestCrossPlatformCoverageOAImageUploadContentMagicBytes(t *testing.T) {
	tests := []struct {
		imageType string
		data      string
		want      bool
	}{
		{"jpg", oaImageJPEGHeader + "\xe0rest", true},
		{"jpeg", oaImageJPEGHeader + "\xe1rest", true},
		{"png", oaImagePNGHeader + "body", true},
		{"gif", "GIF89a\"\x00\x00", true},
		{"gif", "GIF87a", true},
		{"webp", "RIFF\x00\x00\x00\x00WEBPVP8 ", true},
		{"bmp", "BM\x00\x00", true},
		{"png", "fake-png-bytes", false},
		{"jpg", oaImagePNGHeader, false},
		{"png", "", false},
		{"png", "\x89PNG", false},
		{"webp", "RIFF\x00\x00\x00\x00WEXX", false},
		{"gif", "GIF88a", false},
		{"bmp", "MB", false},
		{"tiff", oaImagePNGHeader, false},
	}
	for _, test := range tests {
		if got := imageUploadContentMatchesImageType([]byte(test.data), test.imageType); got != test.want {
			t.Errorf("imageUploadContentMatchesImageType(%q as %s) = %v, want %v", test.data, test.imageType, got, test.want)
		}
	}
}

// TestCrossPlatformCoverageOAImageUploadRejectsNonRegularAfterOpen 覆盖打开后
// descriptor 的 fstat 复核：路径 Stat 为普通文件、但 open 拿到的 descriptor 指向
// FIFO/设备文件时必须在读取前拒绝（防止 ReadAll 阻塞）。
func TestCrossPlatformCoverageOAImageUploadRejectsNonRegularAfterOpen(t *testing.T) {
	testseam.Swap(t, &imageUploadOpenFile, func(string) (imageUploadReadableFile, error) {
		return &personalEmotionTestFile{info: personalEmotionTestFileInfo{name: "evidence.png", size: 16, mode: os.ModeDevice | os.ModeCharDevice}}, nil
	})

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", mustWriteOAImageTempFile(t, "evidence.png", oaImagePNGHeader+"png-body"),
	)
	if err == nil || !strings.Contains(err.Error(), "非普通文件") {
		t.Fatalf("error = %v, want 非普通文件", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0 (non-regular descriptor must reject before upload)", caller.calls)
	}
}

// TestCrossPlatformCoverageOAImageUploadRejectsDirectoryAfterOpen 覆盖打开后
// descriptor 指向目录的 fstat 复核分支。
func TestCrossPlatformCoverageOAImageUploadRejectsDirectoryAfterOpen(t *testing.T) {
	testseam.Swap(t, &imageUploadOpenFile, func(string) (imageUploadReadableFile, error) {
		return &personalEmotionTestFile{info: personalEmotionTestFileInfo{name: "evidence.png", size: 16, mode: os.ModeDir}}, nil
	})

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", mustWriteOAImageTempFile(t, "evidence.png", oaImagePNGHeader+"png-body"),
	)
	if err == nil || !strings.Contains(err.Error(), "目录") {
		t.Fatalf("error = %v, want 目录", err)
	}
}

// TestCrossPlatformCoverageOAImageUploadRejectsFstatFailureAfterOpen 覆盖打开后
// descriptor fstat 失败的复核分支（descriptor 元数据不可读时在读取前拒绝）。
func TestCrossPlatformCoverageOAImageUploadRejectsFstatFailureAfterOpen(t *testing.T) {
	testseam.Swap(t, &imageUploadOpenFile, func(string) (imageUploadReadableFile, error) {
		return &personalEmotionTestFile{
			info:    personalEmotionTestFileInfo{name: "evidence.png", size: 16, mode: 0o644},
			statErr: errors.New("mock fstat failure"),
		}, nil
	})

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "image", "upload",
		"--file", mustWriteOAImageTempFile(t, "evidence.png", oaImagePNGHeader+"png-body"),
	)
	if err == nil || !strings.Contains(err.Error(), "无法读取本地图片") {
		t.Fatalf("error = %v, want 无法读取本地图片", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0 (fstat failure must reject before upload)", caller.calls)
	}
}
