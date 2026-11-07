// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

// executeImageUploadLeaf 按新业务接入方的正规形态构造叶子：NewLeafCommand 完全
// 托管声明 + Call 钩子接入 runImageUpload，验证非 OA 业务也能复用同一原语
// （bizType/上限/身份均按 spec 生效）。
func executeImageUploadLeaf(t *testing.T, caller *scriptedToolCaller, spec imageUploadSpec, args ...string) (string, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	testseam.Swap(t, &os.Args, []string{"dws", spec.Identity.ProductID})
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard

	root := newGroupCommand(&cobra.Command{Use: spec.Identity.ProductID, RunE: groupRunE})
	root.AddCommand(NewLeafCommand(LeafSpec{
		Use:           "upload",
		Short:         "上传本地图片",
		Long:          "上传本地图片（测试业务声明形态）。",
		Example:       "  dws " + spec.Identity.CLIPath + " --file ./a.png",
		OutputRollout: output.RolloutUnifiedActive,
		Flags:         []LeafFlag{{Name: "file", Usage: imageUploadFlagUsage(spec), MarkRequired: true}},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity:    spec.Identity,
			Description: "测试业务图片上传",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: imageUploadDataSchema(spec),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: 命令调用 dingtalk-file/upload_media，无可绑定的单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "测试业务图片上传",
				UseWhen:      []string{"测试业务需要上传图片时"},
				AvoidWhen:    []string{"图片已是公网 URL 时无需上传"},
				Examples:     []string{"dws " + spec.Identity.CLIPath + " --file ./a.png"},
			},
			Parameters: []contract.ParamDecl{{Name: "file", Required: boolPtr(true)}},
		},
		Call: func(cmd *cobra.Command, tool string, args map[string]any) error {
			return runImageUpload(spec, cmd)
		},
	}))
	root.PersistentFlags().Bool("yes", false, "跳过确认")
	root.PersistentFlags().Bool("dry-run", false, "仅预览不执行")
	root.PersistentFlags().String("format", caller.Format(), "输出格式")
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := corecmd.ExecuteForTest(root)
	return stdout.String(), err
}

func futureImageUploadSpec() imageUploadSpec {
	return imageUploadSpec{
		BizType:  "future_biz",
		MaxBytes: 1 * 1024 * 1024,
		Identity: contract.ToolIdentitySpec{
			ProductID:      "future",
			Name:           "image_upload",
			CanonicalPath:  "future.image_upload",
			CLIPath:        "future image upload",
			PrimaryCLIPath: "future image upload",
		},
		MediaUsage: "组装测试业务表单 value",
	}
}

func TestCrossPlatformCoverageImageUploadLeafHonorsSpec(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$future-v2$"}`},
	}}
	stdout, err := executeImageUploadLeaf(t, caller, futureImageUploadSpec(),
		"upload", "--file", filePath,
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if caller.serverLog[0] != "dingtalk-file" || caller.toolLog[0] != "upload_media" {
		t.Fatalf("call = %s/%s, want dingtalk-file/upload_media", caller.serverLog[0], caller.toolLog[0])
	}
	if got := caller.argsLog[0]["bizType"]; got != "future_biz" {
		t.Fatalf("bizType = %#v, want future_biz (spec-driven)", got)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v", err)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["mediaIdV2"] != "$future-v2$" || data["bizType"] != "future_biz" {
		t.Fatalf("data = %#v, want mediaIdV2 $future-v2$ / bizType future_biz", data)
	}
}

func TestCrossPlatformCoverageImageUploadLeafSpecMaxBytes(t *testing.T) {
	oversized := oaImagePNGHeader + strings.Repeat("a", 1*1024*1024+1)
	filePath, _ := writeOAAttachmentTempFile(t, "big.png", oversized)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	_, err := executeImageUploadLeaf(t, caller, futureImageUploadSpec(),
		"upload", "--file", filePath,
	)
	if err == nil || !strings.Contains(err.Error(), "1MB") {
		t.Fatalf("error = %v, want spec MaxBytes 1MB rejection", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0 (local validation must reject first)", caller.calls)
	}
}

func TestCrossPlatformCoverageImageUploadLeafSpecMediaUsageInError(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true}`},
	}}
	_, err := executeImageUploadLeaf(t, caller, futureImageUploadSpec(),
		"upload", "--file", filePath,
	)
	if err == nil || !strings.Contains(err.Error(), "组装测试业务表单 value") {
		t.Fatalf("error = %v, want spec MediaUsage in missing-mediaIdV2 error", err)
	}
}

var (
	errImageUploadTestOpen = errors.New("mock image upload open failure")
	errImageUploadTestRead = errors.New("mock image upload read failure")
)

// TestCrossPlatformCoverageImageUploadRejectsNonRegularFile 覆盖 stat 命中非普通
// 文件（如命名管道）时的本地校验拒绝分支。
func TestCrossPlatformCoverageImageUploadRejectsNonRegularFile(t *testing.T) {
	testseam.Swap(t, &imageUploadOSStat, func(string) (os.FileInfo, error) {
		return personalEmotionTestFileInfo{name: "pipe.png", mode: os.ModeNamedPipe}, nil
	})
	_, err := loadImageUploadFile(filepath.Join(t.TempDir(), "pipe.png"), futureImageUploadSpec())
	if err == nil || !strings.Contains(err.Error(), "非普通文件") {
		t.Fatalf("error = %v, want 非普通文件 rejection", err)
	}
}

// TestCrossPlatformCoverageImageUploadRejectsUnopenableFile 覆盖 stat 通过后
// open 被拒绝的分支。
func TestCrossPlatformCoverageImageUploadRejectsUnopenableFile(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")
	testseam.Swap(t, &imageUploadOpenFile, func(string) (imageUploadReadableFile, error) {
		return nil, errImageUploadTestOpen
	})
	_, err := loadImageUploadFile(filePath, futureImageUploadSpec())
	if err == nil || !strings.Contains(err.Error(), "无法读取本地图片") || !strings.Contains(err.Error(), errImageUploadTestOpen.Error()) {
		t.Fatalf("error = %v, want open failure detail", err)
	}
}

// TestCrossPlatformCoverageImageUploadRejectsUnreadableContent 覆盖 open 成功
// 后读取中途失败的分支。
func TestCrossPlatformCoverageImageUploadRejectsUnreadableContent(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "evidence.png", oaImagePNGHeader+"png-body")
	testseam.Swap(t, &imageUploadOpenFile, func(string) (imageUploadReadableFile, error) {
		return &personalEmotionTestFile{info: personalEmotionTestFileInfo{name: "evidence.png", size: 16, mode: 0o644}, readErr: errImageUploadTestRead}, nil
	})
	_, err := loadImageUploadFile(filePath, futureImageUploadSpec())
	if err == nil || !strings.Contains(err.Error(), "无法读取本地图片") || !strings.Contains(err.Error(), errImageUploadTestRead.Error()) {
		t.Fatalf("error = %v, want read failure detail", err)
	}
}

// TestCrossPlatformCoverageImageUploadUploadMediaNilContext 覆盖 ctx 为 nil 时
// 兑底 Background 的分支。
func TestCrossPlatformCoverageImageUploadUploadMediaNilContext(t *testing.T) {
	testseam.Protect(t, &deps)
	InitDeps(&scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2-nil-ctx$"}`},
	}})
	image := &imageUploadFile{imageType: "png", content: "cG5nLWJ5dGVz"}
	text, err := uploadImageMedia(nil, futureImageUploadSpec(), image)
	if err != nil {
		t.Fatalf("uploadImageMedia(nil ctx): %v", err)
	}
	if !strings.Contains(text, "$v2-nil-ctx$") {
		t.Fatalf("text = %q, want scripted mediaIdV2 response", text)
	}
}

// TestCrossPlatformCoverageImageUploadParseResultServerFailureDetail 表驱动覆盖
// 服务端 success=false 响应的错误详情组装分支（errorMsg/message 回退、errorCode、
// logId 附加与全空兑底）。
func TestCrossPlatformCoverageImageUploadParseResultServerFailureDetail(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantSub []string
	}{
		{"empty detail", `{"success":false}`, []string{"服务端未返回错误详情"}},
		{"message fallback", `{"success":false,"message":"服务端繁忙"}`, []string{"服务端繁忙"}},
		{"full detail", `{"success":false,"errorMsg":"图片超过配额","errorCode":"IMG_QUOTA","logId":"log-9"}`, []string{"图片超过配额", "errorCode=IMG_QUOTA", "logId=log-9"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseImageUploadResult(test.text, "组装测试业务表单 value")
			if err == nil {
				t.Fatalf("parse %s: expected error", test.text)
			}
			for _, want := range test.wantSub {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, want substring %q", err, want)
				}
			}
		})
	}
}
