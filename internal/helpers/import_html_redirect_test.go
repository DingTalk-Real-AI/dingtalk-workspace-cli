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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
		{text: `{"nodeId":"node-1","workspaceId":"ws-1","folderId":"folder-abc","name":"sales.html"}`},
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
		if caller.calls != 3 || caller.tool != "get_document_info" {
			t.Fatalf("fallback calls = %d last tool = %q, want readback after commit", caller.calls, caller.tool)
		}
		if got := caller.argsLog[1]["workspaceId"]; got != "ws-1" {
			t.Fatalf("commit workspaceId = %v, want ws-1", got)
		}
		if got := caller.argsLog[1]["name"]; got != "sales.html" {
			t.Fatalf("commit name = %v, want original file name with extension", got)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("fallback result must be one JSON document: %v\n%s", err, stdout.String())
		}
		if payload["success"] != true || payload["fallback"] != "upload" || payload["converted"] != false || payload["verified"] != true {
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

	t.Run("fallback resolves and verifies the default personal workspace", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
			{text: `{"wikiSpaces":[{"workspaceId":"my-space"}]}`},
			{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`},
			{text: `{"dentryUuid":"node-default","name":"sales.pdf"}`},
			{text: `{"nodeId":"node-default","workspaceId":"my-space","name":"sales.pdf"}`},
		}}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "pdf"))
		if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v", err)
		}
		if strings.Join(caller.toolLog, ",") != "list_wikiSpaces,get_file_upload_info,commit_uploaded_file,get_document_info" {
			t.Fatalf("fallback calls = %#v", caller.toolLog)
		}
		if got := caller.argsLog[2]["workspaceId"]; got != "my-space" {
			t.Fatalf("default commit workspaceId = %#v", got)
		}
		var payload map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		target, _ := payload["target"].(map[string]any)
		if payload["verified"] != true || target["source"] != "default_personal_workspace" || target["workspaceId"] != "my-space" {
			t.Fatalf("fallback result = %#v", payload)
		}
	})

	t.Run("fallback does not report success when placement readback mismatches", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
			{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-1"}`},
			{text: `{"dentryUuid":"node-mismatch"}`},
			{text: `{"nodeId":"node-mismatch","workspaceId":"wrong-space"}`},
		}}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "pdf"))
		_ = cmd.Flags().Set("workspace", "expected-space")
		err := runImportCommand(cmd, nil, docImportFlowConfig())
		var structured *apperrors.Error
		if !errors.As(err, &structured) || structured.Reason != "doc_import_placement_unverified" {
			t.Fatalf("placement mismatch error = %#v", err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("placement mismatch emitted success output: %s", stdout.String())
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
			_ = cmd.Flags().Set("workspace", "ws-1")
			if err := runImportCommand(cmd, nil, docImportFlowConfig()); err != nil {
				t.Fatalf("runImportCommand(%s) error = %v, want upload fallback success", ext, err)
			}
			if caller.tool != "get_document_info" {
				t.Fatalf("%s last tool = %q, want get_document_info", ext, caller.tool)
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
		_ = cmd.Flags().Set("workspace", "ws-1")
		if err := runImportCommand(cmd, []string{path}, docImportFlowConfig()); err != nil {
			t.Fatalf("runImportCommand() error = %v, want upload fallback success", err)
		}
		if caller.tool != "get_document_info" {
			t.Fatalf("last tool = %q, want get_document_info", caller.tool)
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
		if got := caller.argsLog[1]["folderId"]; got != "folder-abc" {
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
		_ = cmd.Flags().Set("workspace", "ws-1")
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
				_ = cmd.Flags().Set("workspace", "ws-1")
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
				_ = cmd.Flags().Set("workspace", "ws-1")
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
			{text: `{"fileId":"nested-node-9","workspaceId":"ws-1"}`},
		}}
		installScriptedCaller(t, caller)
		var stdout bytes.Buffer
		deps.Out.w = &stdout
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })

		cmd := htmlFallbackCommand(t, writeImportFixture(t, "html"))
		_ = cmd.Flags().Set("workspace", "ws-1")
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
