// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestMarkdownCreateThemeUploadCopyAndFinalSize(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		args         func(string) []string
		steps        []markdownDriveStep
		wantServer   string
		wantUploaded string
		wantTempCopy bool
	}{
		{
			name:   "omitted theme preserves source bytes and direct file path",
			source: "\ufeff# 原样\r\nbody\n",
			args: func(path string) []string {
				return []string{"markdown", "create", "--file", path, "--space-id", "space-1"}
			},
			steps: []markdownDriveStep{
				{text: `{"uploadId":"drive-key","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
				{text: `{"created":true}`},
			},
			wantServer:   "drive",
			wantUploaded: "\ufeff# 原样\r\nbody\n",
		},
		{
			name:   "explicit default themes a private file copy",
			source: "# source\n",
			args: func(path string) []string {
				return []string{"markdown", "create", "--file", path, "--theme", "default", "--space-id", "space-1"}
			},
			steps: []markdownDriveStep{
				{text: `{"uploadId":"drive-key","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
				{text: `{"created":true}`},
			},
			wantServer:   "drive",
			wantUploaded: "---\nx-we-markdown-theme: default\n---\n# source\n",
			wantTempCopy: true,
		},
		{
			name:   "content at-file leaves source unchanged and themes doc upload copy",
			source: "---\ntitle: 示例\n---\nbody",
			args: func(path string) []string {
				return []string{"markdown", "create", "--name", "notes.md", "--content", "@" + path, "--theme", "songyan", "--workspace", "workspace-1"}
			},
			steps: []markdownDriveStep{
				{text: `{"resourceUrl":"https://upload.test/doc","uploadKey":"doc-key"}`},
				{text: `{"created":true}`},
			},
			wantServer:   "doc",
			wantUploaded: "---\ntitle: 示例\nx-we-markdown-theme: songyan\n---\nbody",
			wantTempCopy: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{format: "json", steps: test.steps}
			installMarkdownDriveDeps(t, caller)
			sourcePath := writeMarkdownDriveFixture(t, "source.md", test.source)
			var uploadPath string
			var uploadSize int64
			httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, size int64) error {
				uploadPath = path
				uploadSize = size
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if test.wantTempCopy && info.Mode().Perm() != 0o600 {
					t.Fatalf("temporary upload mode = %o, want 600", info.Mode().Perm())
				}
				data, err := os.ReadFile(path)
				if err == nil && string(data) != test.wantUploaded {
					t.Fatalf("uploaded body = %q, want %q", string(data), test.wantUploaded)
				}
				return err
			}

			if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, test.args(sourcePath)...); err != nil {
				t.Fatal(err)
			}
			if uploadSize != int64(len(test.wantUploaded)) {
				t.Fatalf("HTTP PUT size = %d, want transformed size %d", uploadSize, len(test.wantUploaded))
			}
			if test.wantTempCopy && uploadPath == sourcePath {
				t.Fatal("theme mode uploaded the user source file instead of a private copy")
			}
			if !test.wantTempCopy && uploadPath != sourcePath {
				t.Fatalf("omitted theme upload path = %q, want original %q", uploadPath, sourcePath)
			}
			if test.wantTempCopy {
				if _, err := os.Stat(uploadPath); !os.IsNotExist(err) {
					t.Fatalf("temporary upload copy was not cleaned: %s, err=%v", uploadPath, err)
				}
			}
			if source, err := os.ReadFile(sourcePath); err != nil || string(source) != test.source {
				t.Fatalf("source file changed: data=%q err=%v", string(source), err)
			}
			if len(caller.calls) != 2 || caller.calls[0].server != test.wantServer {
				t.Fatalf("calls = %#v, want two %s upload calls", caller.calls, test.wantServer)
			}
			for _, call := range caller.calls {
				if _, leaked := call.args["theme"]; leaked {
					t.Fatalf("theme leaked to %s.%s: %#v", call.server, call.tool, call.args)
				}
				if got, ok := call.args["fileSize"].(float64); ok && got != float64(len(test.wantUploaded)) {
					t.Fatalf("%s.%s fileSize = %v, want %d", call.server, call.tool, got, len(test.wantUploaded))
				}
			}
		})
	}
}

func TestMarkdownOverwriteThemeIsFullReplacementWithoutRemoteMerge(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{text: `{"fileName":"replacement.md"}`},
			{text: `{"uploadId":"upload-1","resourceUrls":[{"url":"https://upload.test/drive"}]}`},
			{text: `{"updated":true}`},
		},
	}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	sourceText := "---\nx-we-markdown-theme: songyan\ntitle: local\n---\nlocal body"
	sourcePath := writeMarkdownDriveFixture(t, "replacement.md", sourceText)
	want := "---\nx-we-markdown-theme: juxia\ntitle: local\n---\nlocal body"
	var uploadPath string
	httpPutFile = func(_ context.Context, _ string, _ map[string]string, path string, size int64) error {
		uploadPath = path
		data, err := os.ReadFile(path)
		if err == nil && (string(data) != want || size != int64(len(want))) {
			t.Fatalf("overwrite PUT = %q (%d), want %q (%d)", string(data), size, want, len(want))
		}
		return err
	}

	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
		"markdown", "overwrite", "--node", "file-1", "--file", sourcePath,
		"--name", "replacement.md", "--space-id", "space-1", "--theme", "juxia", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 || caller.calls[0].tool != "get_file_info" ||
		caller.calls[1].tool != "get_upload_info" || caller.calls[2].tool != "commit_upload" {
		t.Fatalf("overwrite unexpectedly read or merged remote content: %#v", caller.calls)
	}
	for _, call := range caller.calls {
		if _, leaked := call.args["theme"]; leaked {
			t.Fatalf("theme leaked to downstream args: %#v", call)
		}
		if got, ok := call.args["fileSize"].(float64); ok && got != float64(len(want)) {
			t.Fatalf("post-theme fileSize = %#v, want %d", got, len(want))
		}
	}
	if source, err := os.ReadFile(sourcePath); err != nil || string(source) != sourceText {
		t.Fatalf("overwrite source changed: data=%q err=%v", string(source), err)
	}
	if _, err := os.Stat(uploadPath); !os.IsNotExist(err) {
		t.Fatalf("overwrite temp copy was not cleaned: %s, err=%v", uploadPath, err)
	}
	var response map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil || response["updated"] != true {
		t.Fatalf("server response changed: err=%v output=%q", err, stdout.String())
	}
}

func TestMarkdownOverwriteThemeDryRunShowsFinalAfter(t *testing.T) {
	caller := &markdownDriveCaller{
		format: "json",
		steps: []markdownDriveStep{
			{text: `{"fileName":"current.md"}`},
			{text: `{"downloadUrl":"https://download.test/current.md","fileName":"current.md"}`},
		},
	}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	installMarkdownHTTPGet(t, "---\ntitle: old\n---\nold body")
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
		t.Fatal("command-level dry-run performed HTTP PUT")
		return nil
	}

	err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil,
		"markdown", "overwrite", "--node", "file-1", "--content", "# new",
		"--name", "current.md", "--space-id", "space-1", "--theme", "qingya", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	var preview map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatalf("decode dry-run: %v; output=%q", err, stdout.String())
	}
	wantAfter := "---\nx-we-markdown-theme: qingya\n---\n# new"
	if preview["after"] != wantAfter || preview["before"] != "---\ntitle: old\n---\nold body" {
		t.Fatalf("dry-run preview = %#v, want themed final after", preview)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "get_file_info" || caller.calls[1].tool != "download_file" {
		t.Fatalf("dry-run calls = %#v", caller.calls)
	}
}

func TestMarkdownGlobalDryRunValidatesAndDisplaysThemeWithoutNetwork(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "create",
			args: []string{"markdown", "create", "--name", "new.md", "--content", "body", "--theme", "default", "--workspace", "workspace-1"},
		},
		{
			name: "overwrite",
			args: []string{"markdown", "overwrite", "--node", "file-1", "--content", "body", "--theme", "default"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{format: "json"}
			stdout, _ := installMarkdownDriveDeps(t, caller)
			if err := executeMarkdownGlobalDryRun(t, newMarkdownCommand(), test.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("global dry-run made network calls: %#v", caller.calls)
			}
			var preview map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil || preview["theme"] != "default" {
				t.Fatalf("global dry-run theme preview: err=%v payload=%#v output=%q", err, preview, stdout.String())
			}
		})
	}
}

func TestMarkdownInvalidThemeFailsBeforeAnyRemoteOrUploadCall(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "create",
			args: []string{"markdown", "create", "--file", "/missing/source.md", "--theme", "dark", "--space-id", "space-1"},
		},
		{
			name: "overwrite",
			args: []string{"markdown", "overwrite", "--node", "file-1", "--content", "body", "--theme", "QINGYA", "--yes"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &markdownDriveCaller{format: "json"}
			installMarkdownDriveDeps(t, caller)
			httpPutFile = func(context.Context, string, map[string]string, string, int64) error {
				t.Fatal("invalid theme reached HTTP PUT")
				return nil
			}
			err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, test.args...)
			if err == nil || !strings.Contains(err.Error(), "--theme") || !strings.Contains(err.Error(), "不合法") {
				t.Fatalf("invalid theme error = %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid theme made remote calls: %#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownThemeFilePreparationFailures(t *testing.T) {
	for _, operation := range []string{"create", "overwrite"} {
		for _, failure := range []string{"read", "temporary directory"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				caller := &markdownDriveCaller{format: "json"}
				wantCalls := 0
				if operation == "overwrite" {
					caller.steps = []markdownDriveStep{{text: `{"fileName":"source.md"}`}}
					wantCalls = 1
				}
				installMarkdownDriveDeps(t, caller)
				source := writeMarkdownDriveFixture(t, "source.md", "# unchanged")
				wantError := "创建临时目录失败"
				if failure == "read" {
					wantError = "无法读取文件"
					testseam.Swap(t, &textLocalReadFile, func(path string) ([]byte, error) {
						if path != source {
							t.Fatalf("read path = %q, want %q", path, source)
						}
						return nil, errors.New("source read failed")
					})
				} else {
					setMarkdownCIMissingTempDir(t, filepath.Join(t.TempDir(), "missing"))
				}
				testseam.Swap(t, &httpPutFile, func(context.Context, string, map[string]string, string, int64) error {
					t.Fatal("preparation failure must not upload a file")
					return nil
				})
				args := []string{"markdown", operation, "--file", source, "--theme", "qingya", "--space-id", "space-1"}
				if operation == "overwrite" {
					args = append(args, "--node", "file-1", "--yes")
				}
				err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, args...)
				if err == nil || !strings.Contains(err.Error(), wantError) {
					t.Fatalf("error = %v, want %q", err, wantError)
				}
				if len(caller.calls) != wantCalls {
					t.Fatalf("calls = %#v, want only %d metadata calls", caller.calls, wantCalls)
				}
				if content, err := os.ReadFile(source); err != nil || string(content) != "# unchanged" {
					t.Fatalf("source changed: %q, %v", content, err)
				}
			})
		}
	}
}

func TestMarkdownThemeTextDryRunShowsSelection(t *testing.T) {
	caller := &markdownDriveCaller{format: "table"}
	stdout, _ := installMarkdownDriveDeps(t, caller)
	err := executeMarkdownGlobalDryRun(t, newMarkdownCommand(),
		"markdown", "create", "--name", "preview.md", "--content", "# preview", "--theme", "qingya")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "主题") || !strings.Contains(stdout.String(), "qingya") {
		t.Fatalf("text preview omits theme: %q", stdout.String())
	}
	if len(caller.calls) != 0 {
		t.Fatalf("preview made remote calls: %#v", caller.calls)
	}
}
