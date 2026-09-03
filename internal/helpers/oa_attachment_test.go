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
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func executeOAAttachmentCommandCapturingOutput(t *testing.T, caller *scriptedToolCaller, args ...string) (string, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	testseam.Swap(t, &os.Args, []string{"dws", "oa"})
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard

	cmd := newOaCommand()
	cmd.PersistentFlags().Bool("yes", false, "跳过确认")
	cmd.PersistentFlags().Bool("dry-run", false, "仅预览不执行")
	cmd.PersistentFlags().String("format", caller.Format(), "输出格式")
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func TestCrossPlatformCoverageOAAttachmentDownloadURLPreservesSignedQuery(t *testing.T) {
	const response = `{"result":{"downloadUri":"https://example.test/file?Expires=1&OSSAccessKeyId=2&Signature=3"},"success":true}`
	commandArgs := []string{
		"approval", "attachment", "download-url",
		"--instance-id", "instance-1",
		"--file-id", "file-1",
	}
	wantArgs := map[string]any{
		"processInstanceId": "instance-1",
		"fileId":            "file-1",
	}

	t.Run("json", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: response}}}
		stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, commandArgs...)
		if err != nil {
			t.Fatalf("execute command: %v", err)
		}
		if caller.server != "oa" || caller.tool != "get_attachment_download_url" {
			t.Fatalf("called %s/%s, want oa/get_attachment_download_url", caller.server, caller.tool)
		}
		if !reflect.DeepEqual(caller.args, wantArgs) {
			t.Fatalf("args = %#v, want %#v", caller.args, wantArgs)
		}
		if !strings.Contains(stdout, "&OSSAccessKeyId=") || !strings.Contains(stdout, "&Signature=") {
			t.Fatalf("stdout does not preserve signed query separators: %q", stdout)
		}
		if strings.Contains(stdout, `\u0026`) {
			t.Fatalf("stdout contains escaped query separator: %q", stdout)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
			t.Fatalf("decode unified output: %v", err)
		}
		if envelope["ok"] != true || envelope["outcome"] != "success" {
			t.Fatalf("unified envelope = %#v", envelope)
		}
		data, ok := envelope["data"].(map[string]any)
		if !ok || !strings.Contains(data["downloadUri"].(string), "&Signature=") {
			t.Fatalf("unified data = %#v", envelope["data"])
		}
	})

	t.Run("raw compatibility", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "raw", steps: []scriptedToolStep{{text: response}}}
		stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, commandArgs...)
		if err != nil {
			t.Fatalf("execute command: %v", err)
		}
		const want = `{"downloadUri":"https://example.test/file?Expires=1&OSSAccessKeyId=2&Signature=3"}` + "\n"
		if stdout != want {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	})
}

func TestCrossPlatformCoverageOAAttachmentProjectsUnifiedBusinessData(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		response string
		wantData any
	}{
		{
			name:     "download authorization",
			args:     []string{"approval", "attachment", "authorize-download", "--file-infos", `[{"spaceId":27827223951,"fileId":"file-1"}]`},
			response: `{"result":true,"success":true,"dingOpenErrcode":0,"errorMsg":"ok"}`,
			wantData: true,
		},
		{
			name:     "preview authorization",
			args:     []string{"approval", "attachment", "authorize-preview", "--instance-id", "instance-1", "--file-ids", "file-1"},
			response: `{"result":{"spaceId":27827223951,"agentId":4115627346,"class":"com.dingtalk.bpms.oapi.vo.AppSpaceResponse"},"success":true,"dingOpenErrcode":0,"errorMsg":"ok"}`,
			wantData: map[string]any{
				"spaceId": float64(27827223951), "agentId": float64(4115627346),
				"class": "com.dingtalk.bpms.oapi.vo.AppSpaceResponse",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: test.response}}}
			stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller, test.args...)
			if err != nil {
				t.Fatalf("execute command: %v", err)
			}
			var envelope map[string]any
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatalf("decode unified output: %v", err)
			}
			if envelope["ok"] != true || envelope["outcome"] != "success" || !reflect.DeepEqual(envelope["data"], test.wantData) {
				t.Fatalf("unified envelope = %#v, want data %#v", envelope, test.wantData)
			}
		})
	}
}

func TestCrossPlatformCoverageOAAttachmentRejectsSuccessWithoutResult(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"success":true,"dingOpenErrcode":0,"errorMsg":"ok"}`}}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "authorize-download",
		"--file-infos", `[{"spaceId":27827223951,"fileId":"file-1"}]`,
	)
	if err == nil || !strings.Contains(err.Error(), "缺少 result") {
		t.Fatalf("execute error = %v, want missing result", err)
	}
}

func TestCrossPlatformCoverageOAAttachmentRejectsInvalidToolResult(t *testing.T) {
	tests := []struct {
		name string
		step scriptedToolStep
		want string
	}{
		{name: "non-object response", step: scriptedToolStep{text: `[]`}, want: "不是 JSON 对象"},
		{name: "tool error", step: scriptedToolStep{err: errors.New("tool unavailable")}, want: "tool unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{test.step}}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
				"approval", "attachment", "authorize-download",
				"--file-infos", `[{"spaceId":27827223951,"fileId":"file-1"}]`,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("execute error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageOAAttachmentPayloads(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantTool string
		check    func(*testing.T, map[string]any)
	}{
		{
			name: "download URL",
			args: []string{
				"approval", "attachment", "download-url",
				"--instance-id", "instance-1",
				"--file-id", "file-1",
				"--with-comment-attachment=false",
			},
			wantTool: "get_attachment_download_url",
			check: func(t *testing.T, got map[string]any) {
				t.Helper()
				if got["processInstanceId"] != "instance-1" || got["fileId"] != "file-1" {
					t.Fatalf("download identifiers = %#v", got)
				}
				if value, ok := got["withCommentAttachment"].(bool); !ok || value {
					t.Fatalf("withCommentAttachment = %#v, want explicit false", got["withCommentAttachment"])
				}
			},
		},
		{
			name: "download authorization",
			args: []string{
				"approval", "attachment", "authorize-download",
				"--file-infos", `[{"spaceId":27827223951,"fileId":"file-2"}]`,
			},
			wantTool: "auth_download_file",
			check: func(t *testing.T, got map[string]any) {
				t.Helper()
				infos, ok := got["fileInfos"].([]map[string]any)
				if !ok || len(infos) != 1 {
					t.Fatalf("fileInfos = %#v", got["fileInfos"])
				}
				if infos[0]["spaceId"] != json.Number("27827223951") || infos[0]["fileId"] != "file-2" {
					t.Fatalf("fileInfos[0] = %#v", infos[0])
				}
			},
		},
		{
			name: "preview authorization",
			args: []string{
				"approval", "attachment", "authorize-preview",
				"--instance-id", "instance-2",
				"--file-ids", "file-3,file-4",
				"--with-comment-attachment",
			},
			wantTool: "auth_preview_attachment",
			check: func(t *testing.T, got map[string]any) {
				t.Helper()
				if got["processInstanceId"] != "instance-2" || got["withCommentAttachment"] != true {
					t.Fatalf("preview scalar payload = %#v", got)
				}
				ids, ok := got["fileIdList"].([]string)
				if !ok || len(ids) != 2 || ids[0] != "file-3" || ids[1] != "file-4" {
					t.Fatalf("fileIdList = %#v", got["fileIdList"])
				}
			},
		},
		{
			name: "optional boolean omitted",
			args: []string{
				"approval", "attachment", "download-url",
				"--instance-id", "instance-3",
				"--file-id", "file-5",
			},
			wantTool: "get_attachment_download_url",
			check: func(t *testing.T, got map[string]any) {
				t.Helper()
				if _, exists := got["withCommentAttachment"]; exists {
					t.Fatalf("optional withCommentAttachment should be omitted: %#v", got)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{},"success":true}`}}}
			if _, err := executeOAAttachmentCommandCapturingOutput(t, caller, test.args...); err != nil {
				t.Fatalf("execute command: %v", err)
			}
			if caller.server != "oa" || caller.tool != test.wantTool {
				t.Fatalf("called %s/%s, want oa/%s", caller.server, caller.tool, test.wantTool)
			}
			test.check(t, caller.args)
		})
	}
}

func TestCrossPlatformCoverageOAAttachmentRejectsInvalidFileInfos(t *testing.T) {
	valid := `{"spaceId":27827223951,"fileId":"file-1"}`
	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed JSON", raw: `[{`},
		{name: "null", raw: `null`},
		{name: "not array", raw: `{}`},
		{name: "empty array", raw: `[]`},
		{name: "more than ten", raw: `[` + strings.Repeat(valid+`,`, 10) + valid + `]`},
		{name: "missing space ID", raw: `[{"fileId":"file-1"}]`},
		{name: "string space ID", raw: `[{"spaceId":"27827223951","fileId":"file-1"}]`},
		{name: "missing file ID", raw: `[{"spaceId":27827223951}]`},
		{name: "numeric file ID", raw: `[{"spaceId":27827223951,"fileId":232271651278}]`},
		{name: "blank file ID", raw: `[{"spaceId":27827223951,"fileId":"  "}]`},
		{name: "unknown property", raw: `[{"spaceId":27827223951,"fileId":"file-1","extra":true}]`},
		{name: "trailing JSON", raw: `[` + valid + `] {}`},
		{name: "malformed trailing JSON", raw: `[` + valid + `] {`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := executeOACommand(t, caller,
				"approval", "attachment", "authorize-download",
				"--file-infos", test.raw,
			)
			if err == nil {
				t.Fatalf("fileInfos %q unexpectedly succeeded", test.raw)
			}
			if caller.calls != 0 {
				t.Fatalf("invalid fileInfos made %d MCP call(s)", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageOAAttachmentRejectsInvalidPreviewFileIDs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "separator only", raw: `,`},
		{name: "blank item", raw: `file-1, ,file-2`},
		{name: "more than twenty", raw: strings.Repeat("file,", 20) + "file"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := executeOACommand(t, caller,
				"approval", "attachment", "authorize-preview",
				"--instance-id", "instance-1",
				"--file-ids", test.raw,
			)
			if err == nil {
				t.Fatalf("file IDs %q unexpectedly succeeded", test.raw)
			}
			if caller.calls != 0 {
				t.Fatalf("invalid file IDs made %d MCP call(s)", caller.calls)
			}
		})
	}
}

const oaUploadInitResponse = `{"result":{"uploadKey":"key-123","resourceUrls":["https://oss.example.test/upload"],"headers":{"x-oss-date":"20260820T000000Z","Authorization":"OSS signature"},"storageDriver":"oss"},"success":true}`

func oaUploadCommitResponse(size int64) string {
	return `{"result":{"spaceId":27827223951,"fileName":"合同.pdf","fileSize":` +
		strconv.FormatInt(size, 10) +
		`,"class":"com.dingtalk.oapi.response","fileType":"pdf","fileId":"file-abc"},"success":true}`
}

// writeOAAttachmentTempFile 落地一个临时文件，返回其绝对路径与字节数。
func writeOAAttachmentTempFile(t *testing.T, name, content string) (string, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path, int64(len(content))
}

// capturedPut 记录一次 httpPutFile 调用的入参，供断言 PUT 使用了解析出的
// resourceURL 与签名 headers。
type capturedPut struct {
	calls    int
	url      string
	headers  map[string]string
	filePath string
	fileSize int64
}

func mockOAAttachmentPut(t *testing.T) *capturedPut {
	t.Helper()
	put := &capturedPut{}
	SetHTTPPutFile(func(_ context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
		put.calls++
		put.url = url
		put.headers = headers
		put.filePath = filePath
		put.fileSize = fileSize
		return nil
	})
	t.Cleanup(func() { SetHTTPPutFile(nil) })
	return put
}

func TestCrossPlatformCoverageOAAttachmentUploadEndToEnd(t *testing.T) {
	const content = "pdf-bytes-content"
	filePath, fileSize := writeOAAttachmentTempFile(t, "source.pdf", content)
	put := mockOAAttachmentPut(t)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: oaUploadInitResponse},
		{text: oaUploadCommitResponse(fileSize)},
	}}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--file-name", "合同.pdf",
		"--md5", "d41d8cd98f00b204e9800998ecf8427e",
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}

	if caller.calls != 2 {
		t.Fatalf("MCP calls = %d, want 2 (init+commit)", caller.calls)
	}
	if caller.toolLog[0] != "init_attachment_upload_info" || caller.serverLog[0] != "oa" {
		t.Fatalf("first call = %s/%s, want oa/init_attachment_upload_info", caller.serverLog[0], caller.toolLog[0])
	}
	wantInit := map[string]any{
		"fileName": "合同.pdf",
		"fileSize": float64(fileSize),
		"md5":      "d41d8cd98f00b204e9800998ecf8427e",
	}
	if !reflect.DeepEqual(caller.argsLog[0], wantInit) {
		t.Fatalf("init args = %#v, want %#v", caller.argsLog[0], wantInit)
	}

	if put.calls != 1 {
		t.Fatalf("httpPutFile called %d times, want 1", put.calls)
	}
	if put.url != "https://oss.example.test/upload" {
		t.Fatalf("PUT url = %q, want parsed resourceURL", put.url)
	}
	if put.headers["Authorization"] != "OSS signature" || put.headers["x-oss-date"] != "20260820T000000Z" {
		t.Fatalf("PUT headers = %#v, want parsed signed headers", put.headers)
	}
	if put.filePath != filePath || put.fileSize != fileSize {
		t.Fatalf("PUT file = %q size = %d, want %q/%d", put.filePath, put.fileSize, filePath, fileSize)
	}

	if caller.toolLog[1] != "commit_attachment_upload_info" {
		t.Fatalf("second call = %s, want commit_attachment_upload_info", caller.toolLog[1])
	}
	wantCommit := map[string]any{
		"fileName":  "合同.pdf",
		"uploadKey": "key-123",
		"fileSize":  float64(fileSize),
	}
	if !reflect.DeepEqual(caller.argsLog[1], wantCommit) {
		t.Fatalf("commit args = %#v, want %#v", caller.argsLog[1], wantCommit)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v", err)
	}
	if envelope["ok"] != true || envelope["outcome"] != "success" {
		t.Fatalf("unified envelope = %#v", envelope)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok || data["fileId"] != "file-abc" {
		t.Fatalf("unified data = %#v, want fileId file-abc", envelope["data"])
	}
}

func TestCrossPlatformCoverageOAAttachmentUploadAutoMD5AndDefaultName(t *testing.T) {
	const content = "auto-md5-content"
	filePath, fileSize := writeOAAttachmentTempFile(t, "report.xlsx", content)
	wantMD5, err := fileMD5Hex(filePath)
	if err != nil {
		t.Fatalf("compute expected md5: %v", err)
	}
	mockOAAttachmentPut(t)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: oaUploadInitResponse},
		{text: oaUploadCommitResponse(fileSize)},
	}}
	// 既不传 --file-name 也不传 --md5：文件名应回退为 basename，md5 应自动计算。
	if _, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
	); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	if got := caller.argsLog[0]["fileName"]; got != "report.xlsx" {
		t.Fatalf("default fileName = %#v, want basename report.xlsx", got)
	}
	if got := caller.argsLog[0]["md5"]; got != wantMD5 {
		t.Fatalf("auto md5 = %#v, want computed %q", got, wantMD5)
	}
	if got := caller.argsLog[0]["fileSize"]; got != float64(fileSize) {
		t.Fatalf("fileSize = %#v, want number %d", got, fileSize)
	}
}

func TestCrossPlatformCoverageOAAttachmentUploadMalformedInitResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  string
	}{
		{
			name:     "missing result",
			response: `{"success":true}`,
			wantErr:  "缺少 result",
		},
		{
			name:     "missing uploadKey",
			response: `{"result":{"resourceUrls":["https://oss.example.test/upload"],"headers":{"Authorization":"sig","x-oss-date":"d"}},"success":true}`,
			wantErr:  "缺少 uploadKey",
		},
		{
			name:     "empty uploadKey",
			response: `{"result":{"uploadKey":"  ","resourceUrls":["https://oss.example.test/upload"],"headers":{"Authorization":"sig","x-oss-date":"d"}},"success":true}`,
			wantErr:  "缺少 uploadKey",
		},
		{
			name:     "empty resourceUrls",
			response: `{"result":{"uploadKey":"key-1","resourceUrls":[],"headers":{"Authorization":"sig","x-oss-date":"d"}},"success":true}`,
			wantErr:  "缺少 resourceUrls",
		},
		{
			name:     "empty resourceUrls element",
			response: `{"result":{"uploadKey":"key-1","resourceUrls":[""],"headers":{"Authorization":"sig","x-oss-date":"d"}},"success":true}`,
			wantErr:  "首个 resourceUrls 为空",
		},
		{
			name:     "missing headers",
			response: `{"result":{"uploadKey":"key-1","resourceUrls":["https://oss.example.test/upload"]},"success":true}`,
			wantErr:  "缺少 headers",
		},
		{
			name:     "headers missing Authorization",
			response: `{"result":{"uploadKey":"key-1","resourceUrls":["https://oss.example.test/upload"],"headers":{"x-oss-date":"20260820T000000Z"}},"success":true}`,
			wantErr:  "Authorization",
		},
		{
			name:     "headers missing x-oss-date",
			response: `{"result":{"uploadKey":"key-1","resourceUrls":["https://oss.example.test/upload"],"headers":{"Authorization":"OSS sig"}},"success":true}`,
			wantErr:  "x-oss-date",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filePath, _ := writeOAAttachmentTempFile(t, "test.pdf", "data")
			put := mockOAAttachmentPut(t)
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
				{text: test.response},
				// second step should never be reached
				{text: `{"result":{},"success":true}`},
			}}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
				"approval", "attachment", "upload",
				"--file", filePath,
				"--file-name", "test.pdf",
				"--md5", "d41d8cd98f00b204e9800998ecf8427e",
			)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
			if caller.calls != 1 {
				t.Fatalf("MCP calls = %d, want 1 (init only, no commit)", caller.calls)
			}
			if put.calls != 0 {
				t.Fatalf("httpPutFile called %d times, want 0 (should not PUT)", put.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageOAAttachmentUploadRequiredFlagValidation(t *testing.T) {
	existing, _ := writeOAAttachmentTempFile(t, "exists.bin", "x")
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing file", args: []string{"approval", "attachment", "upload"}},
		{name: "file does not exist", args: []string{"approval", "attachment", "upload", "--file", filepath.Join(existing, "missing.bin")}},
		{name: "file is directory", args: []string{"approval", "attachment", "upload", "--file", filepath.Dir(existing)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockOAAttachmentPut(t)
			caller := &scriptedToolCaller{}
			err := executeOACommand(t, caller, test.args...)
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded", test.name)
			}
			if caller.calls != 0 {
				t.Fatalf("invalid upload made %d MCP call(s)", caller.calls)
			}
		})
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadDryRunDoesNotCallRemoteOrPut 验证
// --dry-run 在任何远程调用（init/PUT/commit）之前 early return：零 MCP 调用、
// 零 PUT，且输出一个包含 3 个 planned step 的 plan 预览。
func TestCrossPlatformCoverageOAAttachmentUploadDryRunDoesNotCallRemoteOrPut(t *testing.T) {
	const content = "dry-run-preview-content"
	filePath, fileSize := writeOAAttachmentTempFile(t, "plan.pdf", content)
	wantMD5, err := fileMD5Hex(filePath)
	if err != nil {
		t.Fatalf("compute expected md5: %v", err)
	}
	put := mockOAAttachmentPut(t)

	// caller.dry 驱动 deps.Caller.DryRun()==true；同时传入 --dry-run 以镜像生产路径。
	caller := &scriptedToolCaller{format: "json", dry: true}
	stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run made %d MCP call(s), want 0", caller.calls)
	}
	if put.calls != 0 {
		t.Fatalf("dry-run made %d PUT call(s), want 0", put.calls)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode unified output: %v\noutput: %s", err, stdout)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("unified data missing/invalid: %#v", envelope["data"])
	}
	if data["dry_run"] != true || data["executed"] != false || data["preview_kind"] != "plan" {
		t.Fatalf("plan flags = %#v", data)
	}
	if data["operation"] != "attachment_upload" || data["source"] != "oa" {
		t.Fatalf("plan operation/source = %#v", data)
	}
	if data["file_name"] != "plan.pdf" || data["file_size"] != float64(fileSize) || data["md5"] != wantMD5 {
		t.Fatalf("plan file metadata = %#v (want file_name=plan.pdf size=%d md5=%s)", data, fileSize, wantMD5)
	}
	steps, ok := data["steps"].([]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("plan steps = %#v, want 3 planned steps", data["steps"])
	}
	wantTools := []string{"oa/init_attachment_upload_info", "HTTP PUT", "oa/commit_attachment_upload_info"}
	for i, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("step %d not an object: %#v", i, raw)
		}
		if step["tool"] != wantTools[i] {
			t.Fatalf("step %d tool = %#v, want %q", i, step["tool"], wantTools[i])
		}
		if step["status"] != "planned" {
			t.Fatalf("step %d status = %#v, want planned", i, step["status"])
		}
	}

	// The commit step (index 2) must NOT contain uploadKey in args (remote-only field)
	// and must declare the dependency via a "requires" entry.
	commitStep := steps[2].(map[string]any)
	commitArgs, _ := commitStep["args"].(map[string]any)
	if _, hasUploadKey := commitArgs["uploadKey"]; hasUploadKey {
		t.Fatalf("commit step args must not contain uploadKey, got: %#v", commitArgs)
	}
	requires, ok := commitStep["requires"].([]any)
	if !ok || len(requires) == 0 {
		t.Fatalf("commit step must have non-empty requires, got: %#v", commitStep["requires"])
	}
	found := false
	for _, r := range requires {
		if s, ok := r.(string); ok && (len(s) > 0 && strings.Contains(s, "uploadKey") && strings.Contains(s, "init")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("commit step requires should mention uploadKey and init, got: %#v", requires)
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadMD5Failure injects a failing computeFileMD5 to
// cover runOAAttachmentUpload's MD5 failure branch on all platforms (including Windows).
func TestCrossPlatformCoverageOAAttachmentUploadMD5Failure(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "test.bin", "hello")
	put := mockOAAttachmentPut(t)

	testseam.Swap(t, &computeFileMD5, func(string) (string, error) {
		return "", errors.New("permission denied")
	})

	caller := &scriptedToolCaller{format: "json"}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
	)
	if err == nil {
		t.Fatal("expected error from MD5 failure, got nil")
	}
	if !strings.Contains(err.Error(), "MD5") {
		t.Fatalf("error should mention MD5, got: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("expected 0 MCP calls when MD5 fails, got %d", caller.calls)
	}
	if put.calls != 0 {
		t.Fatalf("expected 0 PUT calls when MD5 fails, got %d", put.calls)
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadHTTPPutError 注入一个返回错误的 PUT，
// 覆盖 runOAAttachmentUpload 中 httpPutFile 失败分支：init 后报错（1 次 MCP 调用）且不提交。
func TestCrossPlatformCoverageOAAttachmentUploadHTTPPutError(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "put-fail.pdf", "data")
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
		return errors.New("oss put failed")
	})
	t.Cleanup(func() { SetHTTPPutFile(nil) })

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: oaUploadInitResponse},
		// commit 不应被达到
		{text: `{"result":{},"success":true}`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--file-name", "put-fail.pdf",
		"--md5", "d41d8cd98f00b204e9800998ecf8427e",
	)
	if err == nil || !strings.Contains(err.Error(), "oss put failed") {
		t.Fatalf("error = %v, want oss put failed", err)
	}
	if caller.calls != 1 {
		t.Fatalf("MCP calls = %d, want 1 (init only, no commit after PUT failure)", caller.calls)
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadMalformedCommitResponse 校验 commit 返回
// 缺少必需字段时命令报错（而不是包装为成功）。
func TestCrossPlatformCoverageOAAttachmentUploadMalformedCommitResponse(t *testing.T) {
	tests := []struct {
		name           string
		commitResponse string
		wantErr        string
	}{
		{
			name:           "result is null",
			commitResponse: `{"success":true,"result":null}`,
			wantErr:        "不是有效的 JSON 对象",
		},
		{
			name:           "result is empty object",
			commitResponse: `{"success":true,"result":{}}`,
			wantErr:        "spaceId",
		},
		{
			name:           "missing fileId",
			commitResponse: `{"success":true,"result":{"spaceId":"123","fileName":"a.pdf","fileSize":100}}`,
			wantErr:        "fileId",
		},
		{
			name:           "empty fileId",
			commitResponse: `{"success":true,"result":{"spaceId":"123","fileName":"a.pdf","fileSize":100,"fileId":""}}`,
			wantErr:        "fileId",
		},
		{
			name:           "missing spaceId",
			commitResponse: `{"success":true,"result":{"fileName":"a.pdf","fileSize":100,"fileId":"file-1"}}`,
			wantErr:        "spaceId",
		},
		{
			name:           "fileSize is zero",
			commitResponse: `{"success":true,"result":{"spaceId":"123","fileName":"a.pdf","fileSize":0,"fileId":"file-1"}}`,
			wantErr:        "fileSize",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filePath, _ := writeOAAttachmentTempFile(t, "commit-validate.pdf", "data")
			put := mockOAAttachmentPut(t)

			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
				{text: oaUploadInitResponse},
				{text: test.commitResponse},
			}}
			_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
				"approval", "attachment", "upload",
				"--file", filePath,
				"--file-name", "commit-validate.pdf",
				"--md5", "d41d8cd98f00b204e9800998ecf8427e",
			)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
			// init + commit = 2 MCP calls
			if caller.calls != 2 {
				t.Fatalf("MCP calls = %d, want 2 (init + commit)", caller.calls)
			}
			// PUT should have been called (init succeeded)
			if put.calls != 1 {
				t.Fatalf("httpPutFile called %d times, want 1", put.calls)
			}
		})
	}
}

// TestCrossPlatformCoverageOAAttachmentValidateCommitResultDirect 直接调用
// validateOAAttachmentCommitResult 覆盖所有分支路径，包括通过 end-to-end 流不可达的
// json.Number 类型分支。
func TestCrossPlatformCoverageOAAttachmentValidateCommitResultDirect(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr string // empty means expect nil error
	}{
		// result 不是 map
		{name: "result is string", input: "hello", wantErr: "不是有效的 JSON 对象"},
		{name: "result is array", input: []any{"x"}, wantErr: "不是有效的 JSON 对象"},
		{name: "result is number", input: float64(42), wantErr: "不是有效的 JSON 对象"},
		{name: "result is nil", input: nil, wantErr: "不是有效的 JSON 对象"},

		// spaceId — 空字符串
		{name: "spaceId empty string", input: map[string]any{
			"spaceId": "", "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: "spaceId"},
		{name: "spaceId whitespace only", input: map[string]any{
			"spaceId": "   ", "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: "spaceId"},

		// spaceId — json.Number（通过 UseNumber 解码的数字）
		{name: "spaceId json.Number", input: map[string]any{
			"spaceId": json.Number("27827223951"), "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: ""},

		// spaceId — float64（正常路径）
		{name: "spaceId float64", input: map[string]any{
			"spaceId": float64(123), "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: ""},

		// spaceId — 非法类型
		{name: "spaceId bool", input: map[string]any{
			"spaceId": true, "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: "spaceId"},

		// fileName — 非 string 类型
		{name: "fileName is number", input: map[string]any{
			"spaceId": "123", "fileName": float64(99), "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: "fileName"},
		{name: "fileName is nil", input: map[string]any{
			"spaceId": "123", "fileName": nil, "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: "fileName"},
		{name: "fileName empty", input: map[string]any{
			"spaceId": "123", "fileName": "", "fileSize": float64(100), "fileId": "file-1",
		}, wantErr: "fileName"},

		// fileSize — json.Number 有效
		{name: "fileSize json.Number valid", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": json.Number("200"), "fileId": "file-1",
		}, wantErr: ""},

		// fileSize — json.Number 无效（<= 0）
		{name: "fileSize json.Number zero", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": json.Number("0"), "fileId": "file-1",
		}, wantErr: "fileSize"},
		{name: "fileSize json.Number negative", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": json.Number("-5"), "fileId": "file-1",
		}, wantErr: "fileSize"},
		{name: "fileSize json.Number invalid", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": json.Number("abc"), "fileId": "file-1",
		}, wantErr: "fileSize"},

		// fileSize — 非法类型（default 分支）
		{name: "fileSize is string", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": "100", "fileId": "file-1",
		}, wantErr: "fileSize"},
		{name: "fileSize is nil", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": nil, "fileId": "file-1",
		}, wantErr: "fileSize"},

		// fileId — 非 string 类型
		{name: "fileId is number", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": float64(100), "fileId": float64(99),
		}, wantErr: "fileId"},
		{name: "fileId whitespace", input: map[string]any{
			"spaceId": "123", "fileName": "a.pdf", "fileSize": float64(100), "fileId": "  ",
		}, wantErr: "fileId"},

		// 全部通过
		{name: "all valid string types", input: map[string]any{
			"spaceId": "123", "fileName": "report.pdf", "fileSize": float64(512), "fileId": "file-abc",
		}, wantErr: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateOAAttachmentCommitResult(test.input)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadEmptyFilePath 覆盖 runOAAttachmentUpload
// 中 --file 传了但值为空字符串的分支（cobra MarkFlagRequired 只拦截未传，不拦截空值）。
func TestCrossPlatformCoverageOAAttachmentUploadEmptyFilePath(t *testing.T) {
	mockOAAttachmentPut(t)
	caller := &scriptedToolCaller{format: "json"}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", "",
	)
	if err == nil || !strings.Contains(err.Error(), "--file 不能为空") {
		t.Fatalf("error = %v, want --file empty error", err)
	}
	if caller.calls != 0 {
		t.Fatalf("MCP calls = %d, want 0", caller.calls)
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadInitCallError 覆盖 runOAAttachmentUpload
// 中 callMCPToolReturnTextOnServer（init 步）返回错误的分支。
func TestCrossPlatformCoverageOAAttachmentUploadInitCallError(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "init-err.pdf", "data")
	put := mockOAAttachmentPut(t)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{err: errors.New("injected init failure")},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--file-name", "init-err.pdf",
		"--md5", "d41d8cd98f00b204e9800998ecf8427e",
	)
	if err == nil {
		t.Fatal("expected error from init call failure, got nil")
	}
	if caller.calls != 1 {
		t.Fatalf("MCP calls = %d, want 1 (init attempted)", caller.calls)
	}
	if put.calls != 0 {
		t.Fatalf("PUT calls = %d, want 0", put.calls)
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadCommitNonObject 覆盖 runOAAttachmentUpload
// 中 commitData 不是 map[string]any 的分支（line 348）。
func TestCrossPlatformCoverageOAAttachmentUploadCommitNonObject(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "commit-nonobj.pdf", "data")
	mockOAAttachmentPut(t)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: oaUploadInitResponse},
		// commit 返回一个 JSON 字符串而非对象
		{text: `"not an object"`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--file-name", "commit-nonobj.pdf",
		"--md5", "d41d8cd98f00b204e9800998ecf8427e",
	)
	if err == nil || !strings.Contains(err.Error(), "不是 JSON 对象") {
		t.Fatalf("error = %v, want commit non-object error", err)
	}
	if caller.calls != 2 {
		t.Fatalf("MCP calls = %d, want 2 (init + commit)", caller.calls)
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadMalformedInitJSON 覆盖
// parseOAAttachmentUploadInfo 中 json.Unmarshal 失败分支（init 返回非法 JSON）。
func TestCrossPlatformCoverageOAAttachmentUploadMalformedInitJSON(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "bad-init.pdf", "data")
	put := mockOAAttachmentPut(t)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{not valid json`},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--file-name", "bad-init.pdf",
		"--md5", "d41d8cd98f00b204e9800998ecf8427e",
	)
	if err == nil || !strings.Contains(err.Error(), "解析 init_attachment_upload_info 返回失败") {
		t.Fatalf("error = %v, want parse failure", err)
	}
	if caller.calls != 1 {
		t.Fatalf("MCP calls = %d, want 1 (init only)", caller.calls)
	}
	if put.calls != 0 {
		t.Fatalf("PUT calls = %d, want 0", put.calls)
	}
}

// TestCrossPlatformCoverageOAAttachmentValidateCommitResultNormalization 验证
// validateOAAttachmentCommitResult 对 spaceId/fileSize 的归一化行为。
func TestCrossPlatformCoverageOAAttachmentValidateCommitResultNormalization(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		wantSpaceID any
		wantSize    any
		wantErr     string
	}{
		{
			name:        "string spaceId normalized to int64",
			input:       map[string]any{"spaceId": "27827223951", "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1"},
			wantSpaceID: int64(27827223951),
			wantSize:    float64(100),
		},
		{
			name:    "non-numeric string spaceId returns error",
			input:   map[string]any{"spaceId": "abc", "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1"},
			wantErr: "spaceId",
		},
		{
			name:    "float string spaceId returns error",
			input:   map[string]any{"spaceId": "12.5", "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1"},
			wantErr: "spaceId",
		},
		{
			name:        "json.Number spaceId normalized to int64",
			input:       map[string]any{"spaceId": json.Number("27827223951"), "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1"},
			wantSpaceID: int64(27827223951),
			wantSize:    float64(100),
		},
		{
			name:    "json.Number spaceId non-integer returns error",
			input:   map[string]any{"spaceId": json.Number("12.5"), "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1"},
			wantErr: "spaceId",
		},
		{
			name:        "json.Number fileSize normalized to int64",
			input:       map[string]any{"spaceId": float64(123), "fileName": "a.pdf", "fileSize": json.Number("2048"), "fileId": "file-1"},
			wantSpaceID: float64(123),
			wantSize:    int64(2048),
		},
		{
			name:        "float64 spaceId unchanged",
			input:       map[string]any{"spaceId": float64(27827223951), "fileName": "a.pdf", "fileSize": float64(100), "fileId": "file-1"},
			wantSpaceID: float64(27827223951),
			wantSize:    float64(100),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := validateOAAttachmentCommitResult(test.input)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if normalized["spaceId"] != test.wantSpaceID {
				t.Fatalf("spaceId = %v (%T), want %v (%T)", normalized["spaceId"], normalized["spaceId"], test.wantSpaceID, test.wantSpaceID)
			}
			if normalized["fileSize"] != test.wantSize {
				t.Fatalf("fileSize = %v (%T), want %v (%T)", normalized["fileSize"], normalized["fileSize"], test.wantSize, test.wantSize)
			}
		})
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadOutputContract 验证端到端上传命令
// 输出 JSON 中 spaceId 始终为 number（即使 commit 返回字符串 spaceId）。
func TestCrossPlatformCoverageOAAttachmentUploadOutputContract(t *testing.T) {
	tests := []struct {
		name           string
		commitResponse string
		wantSpaceID    float64 // JSON decode 后 number → float64
	}{
		{
			name:           "spaceId as number",
			commitResponse: `{"result":{"spaceId":27827223951,"fileName":"合同.pdf","fileSize":17,"fileType":"pdf","fileId":"file-abc"},"success":true}`,
			wantSpaceID:    float64(27827223951),
		},
		{
			name:           "spaceId as string",
			commitResponse: `{"result":{"spaceId":"27827223951","fileName":"合同.pdf","fileSize":17,"fileType":"pdf","fileId":"file-abc"},"success":true}`,
			wantSpaceID:    float64(27827223951),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const content = "output-contract-data"
			filePath, _ := writeOAAttachmentTempFile(t, "contract.pdf", content)
			mockOAAttachmentPut(t)

			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
				{text: oaUploadInitResponse},
				{text: test.commitResponse},
			}}
			stdout, err := executeOAAttachmentCommandCapturingOutput(t, caller,
				"approval", "attachment", "upload",
				"--file", filePath,
				"--file-name", "合同.pdf",
				"--md5", "d41d8cd98f00b204e9800998ecf8427e",
			)
			if err != nil {
				t.Fatalf("execute command: %v", err)
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
			// spaceId must be a number in the JSON output
			spaceID, ok := data["spaceId"].(float64)
			if !ok {
				t.Fatalf("spaceId is not a number: %v (%T)", data["spaceId"], data["spaceId"])
			}
			if spaceID != test.wantSpaceID {
				t.Fatalf("spaceId = %v, want %v", spaceID, test.wantSpaceID)
			}
			// fileSize must also be a number
			if _, ok := data["fileSize"].(float64); !ok {
				t.Fatalf("fileSize is not a number: %v (%T)", data["fileSize"], data["fileSize"])
			}
		})
	}
}

// TestCrossPlatformCoverageOAAttachmentUploadCommitError 脚本化 init 返回合法、
// commit 返回错误，覆盖 callOAAttachmentResultCtx（commit 步）失败分支：
// PUT 发生一次，init+commit 均尝试（共 2 次 MCP 调用），命令报错。
func TestCrossPlatformCoverageOAAttachmentUploadCommitError(t *testing.T) {
	filePath, _ := writeOAAttachmentTempFile(t, "commit-fail.pdf", "data")
	put := mockOAAttachmentPut(t)

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: oaUploadInitResponse},
		{err: errors.New("commit upload failed")},
	}}
	_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
		"approval", "attachment", "upload",
		"--file", filePath,
		"--file-name", "commit-fail.pdf",
		"--md5", "d41d8cd98f00b204e9800998ecf8427e",
	)
	if err == nil || !strings.Contains(err.Error(), "commit upload failed") {
		t.Fatalf("error = %v, want commit upload failed", err)
	}
	if put.calls != 1 {
		t.Fatalf("PUT calls = %d, want 1", put.calls)
	}
	if caller.calls != 2 {
		t.Fatalf("MCP calls = %d, want 2 (init + commit attempted)", caller.calls)
	}
	if caller.toolLog[0] != "init_attachment_upload_info" || caller.toolLog[1] != "commit_attachment_upload_info" {
		t.Fatalf("tool sequence = %v, want init then commit", caller.toolLog)
	}
}
