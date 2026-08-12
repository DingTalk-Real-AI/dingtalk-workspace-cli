// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var (
	errUploadInfo = errors.New("upload info failed")
	errUploadPut  = errors.New("put failed")
)

func htmlFallbackCommand(t *testing.T, filePath string) *cobra.Command {
	t.Helper()
	// callMCPToolReturnText 从 os.Args 解析产品名（doc）
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	cmd := &cobra.Command{Use: "import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder", "", "")
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("folder-id", "", "")
	cmd.Flags().String("workspace-id", "", "")
	if filePath != "" {
		if err := cmd.Flags().Set("file", filePath); err != nil {
			t.Fatalf("set import file: %v", err)
		}
	}
	return cmd
}

func TestCrossPlatformCoverageDocImportHTMLUploadRedirect(t *testing.T) {
	uploadSteps := []scriptedToolStep{
		{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`},
		{text: `{"dentryUuid":"node-1","name":"sales.html"}`},
	}

	t.Run("html upload fallback emits marked json without legacy warnings", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: uploadSteps}
		installScriptedCaller(t, caller)
		var stdout, warnings bytes.Buffer
		deps.Out.w = &stdout
		deps.Out.errW = &warnings
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		if err := cmd.Flags().Set("workspace", "ws-1"); err != nil {
			t.Fatal(err)
		}
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v, want upload fallback success", err)
		}
		if caller.calls != 2 || caller.tool != "commit_uploaded_file" {
			t.Fatalf("fallback calls = %d last tool = %q, want 2 calls ending in commit_uploaded_file", caller.calls, caller.tool)
		}
		if got := caller.args["workspaceId"]; got != "ws-1" {
			t.Fatalf("commit workspaceId = %v, want ws-1", got)
		}
		if got := caller.args["name"]; got != "sales.html" {
			t.Fatalf("commit name = %v, want original file name with extension", got)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("fallback result must be one JSON document: %v\n%s", err, stdout.String())
		}
		if payload["success"] != true || payload["fallback"] != "upload" || payload["converted"] != false {
			t.Fatalf("fallback markers missing: %#v", payload)
		}
		if payload["dentry_id"] != "node-1" {
			t.Fatalf("dentry_id = %v, want node-1", payload["dentry_id"])
		}
		if payload["requested_operation"] != "导入本地文件为在线文档" {
			t.Fatalf("requested_operation = %v", payload["requested_operation"])
		}
		if !strings.Contains(warnings.String(), "文件上传链路") {
			t.Fatalf("fallback must announce the upload on stderr, got %q", warnings.String())
		}
		if strings.Contains(warnings.String(), "deprecated") {
			t.Fatalf("fallback must not emit the doc upload deprecation warning, got %q", warnings.String())
		}
	})

	t.Run("json dry run stays a single json document", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", dry: true}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		if caller.calls != 0 {
			t.Fatalf("dry run must not call MCP, calls = %d", caller.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("json dry-run output is not JSON: %v\n%s", err, stdout.String())
		}
		if payload["dry_run"] != true || payload["executed"] != false || payload["fallback"] != "upload" || payload["converted"] != false {
			t.Fatalf("dry-run fallback payload = %#v", payload)
		}
	})

	t.Run("any non-importable format is redirected, not enumerated", func(t *testing.T) {
		for _, ext := range []string{"pdf", "zip", "png", "mp4"} {
			caller := &scriptedToolCaller{steps: uploadSteps}
			installScriptedCaller(t, caller)
			SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
			t.Cleanup(func() { SetHTTPPutFile(nil) })

			cmd := htmlFallbackCommand(t, writeImportFixture(t, ext))
			if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
				t.Fatalf("runImportCommand(%s) error = %v, want upload fallback success", ext, err)
			}
			if caller.tool != "commit_uploaded_file" {
				t.Fatalf("%s last tool = %q, want commit_uploaded_file", ext, caller.tool)
			}
		}
	})

	t.Run("uppercase htm extension via positional argument", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: uploadSteps}
		installScriptedCaller(t, caller)
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		path := writeImportFixture(t, "HTM")
		cmd := htmlFallbackCommand(t, "")
		if err := runImportCommand(cmd, []string{path}, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v, want upload fallback success", err)
		}
		if caller.tool != "commit_uploaded_file" {
			t.Fatalf("last tool = %q, want commit_uploaded_file", caller.tool)
		}
	})

	t.Run("hidden folder-id alias reaches the upload chain", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: uploadSteps}
		installScriptedCaller(t, caller)
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		if err := cmd.Flags().Set("folder-id", "folder-abc"); err != nil {
			t.Fatal(err)
		}
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v, want upload fallback success", err)
		}
		if got := caller.args["folderId"]; got != "folder-abc" {
			t.Fatalf("commit folderId = %v, want folder-abc from --folder-id alias", got)
		}
	})

	t.Run("extensionless file is redirected with a readable label", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: uploadSteps}
		installScriptedCaller(t, caller)
		var warnings bytes.Buffer
		deps.Out.errW = &warnings
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		noExt := filepath.Join(t.TempDir(), "README")
		if err := os.WriteFile(noExt, []byte("plain"), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := htmlFallbackCommand(t, noExt)
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v, want upload fallback success", err)
		}
		if !strings.Contains(warnings.String(), "无扩展名") {
			t.Fatalf("warning must label the extensionless file, got %q", warnings.String())
		}
	})

	t.Run("fallback enforces the shared 20MB limit", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		big := filepath.Join(t.TempDir(), "big.html")
		f, err := os.Create(big)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(importMaxFileSize + 1); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		cmd := htmlFallbackCommand(t, big)
		err = runImportCommand(cmd, nil, docImportFlowConfig())
		if err == nil || !strings.Contains(err.Error(), "exceeds 20MB limit") {
			t.Fatalf("runImportCommand() error = %v, want 20MB limit rejection", err)
		}
	})

	t.Run("fallback enforces the shared empty-file guard", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		empty := filepath.Join(t.TempDir(), "empty.html")
		if err := os.WriteFile(empty, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := htmlFallbackCommand(t, empty)
		err := runImportCommand(cmd, nil, docImportFlowConfig())
		if err == nil || !strings.Contains(err.Error(), "file is empty") {
			t.Fatalf("runImportCommand() error = %v, want empty-file rejection", err)
		}
	})

	t.Run("importable formats keep the conversion path", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", dry: true}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "md"))
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("import dry-run output is not JSON: %v\n%s", err, stdout.String())
		}
		if payload["operation"] != "导入本地文件为在线文档" {
			t.Fatalf("operation = %v, want conversion path", payload["operation"])
		}
		if _, ok := payload["fallback"]; ok {
			t.Fatalf("importable format must not carry fallback marker: %#v", payload)
		}
	})

	t.Run("missing file falls through to the import required-flag error", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		cmd := htmlFallbackCommand(t, "")
		err := runImportCommand(cmd, nil, docImportFlowConfig())
		if err == nil || !strings.Contains(err.Error(), "--file is required") {
			t.Fatalf("runImportCommand() error = %v, want --file required", err)
		}
	})

	t.Run("text dry run prints the fallback plan as key values", func(t *testing.T) {
		caller := &scriptedToolCaller{dry: true}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		if caller.calls != 0 {
			t.Fatalf("dry run must not call MCP, calls = %d", caller.calls)
		}
		if !strings.Contains(stdout.String(), "doc import 回退") || !strings.Contains(stdout.String(), "sales.html") {
			t.Fatalf("text dry-run must print the fallback plan, got %q", stdout.String())
		}
	})

	t.Run("upload chain errors propagate", func(t *testing.T) {
		cases := []struct {
			name    string
			steps   []scriptedToolStep
			putErr  error
			wantErr string
		}{
			{name: "upload info request fails", steps: []scriptedToolStep{{err: errUploadInfo}}, wantErr: "upload info failed"},
			{name: "upload credentials incomplete", steps: []scriptedToolStep{{text: `{"resourceUrl":""}`}}, wantErr: "incomplete upload credentials"},
			{name: "http put fails", steps: []scriptedToolStep{{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`}}, putErr: errUploadPut, wantErr: "put failed"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				installScriptedCaller(t, &scriptedToolCaller{steps: tc.steps})
				SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return tc.putErr })
				t.Cleanup(func() { SetHTTPPutFile(nil) })

				cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
				err := runImportCommand(cmd, nil, docImportFlowConfig())
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("runImportCommand() error = %v, want %q", err, tc.wantErr)
				}
			})
		}
	})

	t.Run("commit responses that cannot prove success are rejected", func(t *testing.T) {
		cases := []struct {
			name    string
			commit  string
			wantErr string
		}{
			{name: "empty legacy ack", commit: "  ", wantErr: "响应为空"},
			{name: "non-json text", commit: "commit-ok-plain-text", wantErr: "无法解析为 JSON"},
			{name: "missing file identity", commit: `{"ok":true}`, wantErr: "缺少文件标识"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
					{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`},
					{text: tc.commit},
				}})
				SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
				t.Cleanup(func() { SetHTTPPutFile(nil) })

				cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
				err := runImportCommand(cmd, nil, docImportFlowConfig())
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("runImportCommand() error = %v, want %q", err, tc.wantErr)
				}
			})
		}
	})

	t.Run("nested result envelope yields the dentry id", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
			{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`},
			{text: `{"result":{"dentryUuid":"nested-node-9"}}`},
		}}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("fallback result must stay JSON: %v\n%s", err, stdout.String())
		}
		if payload["dentry_id"] != "nested-node-9" {
			t.Fatalf("dentry_id = %v, want nested-node-9", payload["dentry_id"])
		}
	})

	t.Run("sheet import keeps rejecting html without fallback", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		// 无导入目标时也必须先报 unsupported（基线校验顺序：扩展名先于目标）
		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		err := runImportCommand(cmd, nil, sheetImportFlowConfig())
		if err == nil || !strings.Contains(err.Error(), "unsupported file format") {
			t.Fatalf("sheet import (no target) error = %v, want unsupported file format", err)
		}

		cmd = htmlFallbackCommand(t, writeImportFixture(t, "html"))
		if err := cmd.Flags().Set("workspace", "ws-1"); err != nil {
			t.Fatal(err)
		}
		err = runImportCommand(cmd, nil, sheetImportFlowConfig())
		if err == nil || !strings.Contains(err.Error(), "unsupported file format") {
			t.Fatalf("sheet import (with target) error = %v, want unsupported file format", err)
		}
	})
}
