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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type sheetImportCall struct {
	server string
	tool   string
	args   map[string]any
}

type forcedJSONWriteFailure struct{}

func (forcedJSONWriteFailure) Write([]byte) (int, error) {
	return 0, fmt.Errorf("forced JSON output failure")
}

type sheetImportCaller struct {
	calls     []sheetImportCall
	responses map[string][]string
	dryRun    bool
}

func (c *sheetImportCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, sheetImportCall{server: server, tool: tool, args: args})
	responses := c.responses[tool]
	if len(responses) == 0 {
		return nil, fmt.Errorf("unexpected tool call %s/%s", server, tool)
	}
	c.responses[tool] = responses[1:]
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: responses[0]}}}, nil
}

func (*sheetImportCaller) Format() string { return "json" }
func (c *sheetImportCaller) DryRun() bool { return c.dryRun }
func (*sheetImportCaller) Fields() string { return "" }
func (*sheetImportCaller) JQ() string     { return "" }

func fastSheetImportConfig() importFlowConfig {
	cfg := sheetImportFlowConfig()
	cfg.poll.maxPolls = 3
	cfg.poll.interval = func(int) time.Duration { return 0 }
	cfg.poll.wait = func(context.Context, time.Duration) error { return nil }
	return cfg
}

func executeSheetImportCommand(t *testing.T, caller *sheetImportCaller, cfg importFlowConfig, args ...string) (string, error) {
	t.Helper()
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
		SetHTTPPutFile(nil)
	})

	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = &output
	os.Args = append([]string{"dws", "sheet"}, args...)
	root := newSheetImportCmdWithConfig(cfg)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	err := root.Execute()
	return output.String(), err
}

func writeImportFixture(t *testing.T, ext string) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "sales."+ext)
	if err := os.WriteFile(filePath, []byte("test workbook"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return filePath
}

func TestSheetImportRejectsInvalidInputBeforeRemoteCall(t *testing.T) {
	tests := []struct {
		name    string
		fileExt string
		args    func(string) []string
		wantErr string
	}{
		{
			name:    "target is required",
			fileExt: "xlsx",
			args:    func(path string) []string { return []string{"--file", path} },
			wantErr: "--folder-token 与 --workspace 至少需要提供一个",
		},
		{
			name:    "only excel formats are accepted",
			fileExt: "csv",
			args:    func(path string) []string { return []string{"--file", path, "--workspace", "ws1"} },
			wantErr: `unsupported file format "csv", supported: xlsx, xls`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &sheetImportCaller{}
			_, err := executeSheetImportCommand(t, caller, fastSheetImportConfig(), tt.args(writeImportFixture(t, tt.fileExt))...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageSheetImportRunsSharedDocImportFlow(t *testing.T) {
	caller := &sheetImportCaller{responses: map[string][]string{
		"create_import_session": {`{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
		"confirm_import":        {`{"taskId":"task-1"}`},
		"query_import_task":     {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-1?from=test","documentName":"Sales","documentType":"1"}`},
	}}
	SetHTTPPutFile(func(_ context.Context, uploadURL string, _ map[string]string, _ string, size int64) error {
		if uploadURL != "https://upload.example.test/object" || size <= 0 {
			return fmt.Errorf("unexpected upload url=%s size=%d", uploadURL, size)
		}
		return nil
	})

	output, err := executeSheetImportCommand(t, caller, fastSheetImportConfig(),
		"--file", writeImportFixture(t, "xlsx"),
		"--workspace", "workspace-1",
		"--name", "Sales",
	)
	if err != nil {
		t.Fatalf("sheet import returned error: %v", err)
	}

	wantTools := []string{"create_import_session", "confirm_import", "query_import_task"}
	if len(caller.calls) != len(wantTools) {
		t.Fatalf("calls = %#v", caller.calls)
	}
	for i, call := range caller.calls {
		if call.server != "doc" || call.tool != wantTools[i] {
			t.Fatalf("call[%d] = %#v, want doc/%s", i, call, wantTools[i])
		}
	}
	wantSession := map[string]any{
		"fileName":    "Sales",
		"suffix":      "xlsx",
		"fileSize":    int64(len("test workbook")),
		"workspaceId": "workspace-1",
	}
	if !reflect.DeepEqual(caller.calls[0].args, wantSession) {
		t.Fatalf("session args = %#v, want %#v", caller.calls[0].args, wantSession)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("sheet import stdout must be one JSON document: %v\n%s", err, output)
	}
	if payload["nodeId"] != "node-1" || payload["success"] != true {
		t.Fatalf("output missing success contract: %#v", payload)
	}
}

func TestCrossPlatformCoverageSheetImportCreateLeafMatchesLegacyDryRun(t *testing.T) {
	filePath := writeImportFixture(t, "xlsx")
	legacy, err := executeSheetImportCommand(t, &sheetImportCaller{dryRun: true}, fastSheetImportConfig(),
		"--file", filePath, "--workspace", "workspace-1", "--name", "Sales")
	if err != nil {
		t.Fatalf("legacy sheet import dry-run returned error: %v", err)
	}
	leaf, err := executeSheetImportCommand(t, &sheetImportCaller{dryRun: true}, fastSheetImportConfig(),
		"create", "--file", filePath, "--workspace", "workspace-1", "--name", "Sales")
	if err != nil {
		t.Fatalf("sheet import create dry-run returned error: %v", err)
	}
	if leaf != legacy {
		t.Fatalf("create leaf output differs from legacy entry:\nlegacy:\n%s\nleaf:\n%s", legacy, leaf)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(legacy), &payload); err != nil {
		t.Fatalf("sheet import dry-run stdout must be one JSON document: %v\n%s", err, legacy)
	}
	if payload["dry_run"] != true || payload["executed"] != false || payload["preview_kind"] != "plan" || payload["operation"] != "导入本地表格文件为在线电子表格" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}

	root := newSheetImportCmdWithConfig(fastSheetImportConfig())
	create, _, err := root.Find([]string{"create"})
	if err != nil || create == nil {
		t.Fatalf("find create leaf: command=%v err=%v", create, err)
	}
	if !create.Runnable() || create.HasSubCommands() {
		t.Fatalf("create command must be a runnable leaf: runnable=%v hasSubcommands=%v", create.Runnable(), create.HasSubCommands())
	}
}

func TestCrossPlatformCoverageSheetImportTimeoutIsStructuredSuccessExit(t *testing.T) {
	cfg := fastSheetImportConfig()
	cfg.poll.maxPolls = 2
	caller := &sheetImportCaller{responses: map[string][]string{
		"create_import_session": {`{"sessionId":"session-1","uploadUrl":"https://upload.example.test/object"}`},
		"confirm_import":        {`{"taskId":"task-1"}`},
		"query_import_task": {
			`{"status":"processing"}`,
			`{"status":"processing"}`,
		},
	}}
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })

	output, err := executeSheetImportCommand(t, caller, cfg,
		"--file", writeImportFixture(t, "xls"), "--folder-token", "folder-1")
	if err != nil {
		t.Fatalf("timeout should exit successfully, got %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("timeout stdout must be one JSON document: %v\n%s", err, output)
	}
	if payload["success"] != false || payload["timed_out"] != true || payload["status"] != "processing" || payload["next_command"] != "dws sheet import get --task-id task-1" {
		t.Fatalf("unexpected timeout payload: %#v", payload)
	}
}

func TestSheetImportGetAddsNodeIDAndUsesDocServer(t *testing.T) {
	caller := &sheetImportCaller{responses: map[string][]string{
		"query_import_task": {`{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-2/","documentType":"1"}`},
	}}
	output, err := executeSheetImportCommand(t, caller, fastSheetImportConfig(), "get", "--task-id", "task-2")
	if err != nil {
		t.Fatalf("sheet import get returned error: %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].server != "doc" || caller.calls[0].tool != "query_import_task" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	if !reflect.DeepEqual(caller.calls[0].args, map[string]any{"taskId": "task-2"}) {
		t.Fatalf("args = %#v", caller.calls[0].args)
	}

	var result map[string]any
	if json.Unmarshal([]byte(output), &result) != nil {
		t.Fatalf("invalid JSON output:\n%s", output)
	}
	if result["nodeId"] != "node-2" {
		t.Fatalf("nodeId = %#v, want node-2", result["nodeId"])
	}
}

func TestSheetImportGetPropagatesJSONWriteFailure(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{name: "completed", response: `{"status":"completed","documentUrl":"https://alidocs.dingtalk.com/i/nodes/node-2/"}`},
		{name: "processing", response: `{"status":"processing"}`},
		{name: "failed", response: `{"status":"failed","message":"conversion failed"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousDeps := deps
			t.Cleanup(func() { deps = previousDeps })

			caller := &sheetImportCaller{responses: map[string][]string{
				"query_import_task": {tt.response},
			}}
			InitDeps(caller)
			deps.Out.w = forcedJSONWriteFailure{}

			root := newSheetImportCmdWithConfig(fastSheetImportConfig())
			root.SilenceErrors = true
			root.SilenceUsage = true
			root.SetArgs([]string{"get", "--task-id", "task-write-failure"})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "forced JSON output failure") {
				t.Fatalf("error = %v, want JSON write failure", err)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetImportGetDryRunJSONIsSingleDocument(t *testing.T) {
	caller := &sheetImportCaller{dryRun: true}
	output, err := executeSheetImportCommand(t, caller, fastSheetImportConfig(), "get", "--task-id", "task-dry-run")
	if err != nil {
		t.Fatalf("sheet import get dry-run returned error: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run remote calls = %#v, want none", caller.calls)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("sheet import get dry-run stdout must be one JSON document: %v\n%s", err, output)
	}
	if payload["dry_run"] != true || payload["executed"] != false || payload["preview_kind"] != "plan" || payload["taskId"] != "task-dry-run" || payload["operation"] != "查询表格导入任务结果" {
		t.Fatalf("unexpected sheet import get dry-run payload: %#v", payload)
	}
}

func TestDocImportConfigPreservesExistingContract(t *testing.T) {
	cfg := docImportFlowConfig()
	for _, ext := range []string{"docx", "doc", "xlsx", "xls", "md", "txt", "xmind", "mark"} {
		if !cfg.supportedFormats[ext] {
			t.Errorf("doc import no longer supports %s", ext)
		}
	}
	if cfg.requireTarget || cfg.includeNodeID || cfg.timeoutAsResult {
		t.Fatalf("doc import compatibility changed: %#v", cfg)
	}
}

func TestCrossPlatformCoverageDocImportDryRunStillAcceptsMarkdownWithoutTarget(t *testing.T) {
	previousDeps := deps
	previousArgs := os.Args
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
	})

	caller := &sheetImportCaller{dryRun: true}
	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	filePath := writeImportFixture(t, "md")
	os.Args = []string{"dws", "doc", "import", "--file", filePath}
	root := newDocCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs([]string{"import", "--file", filePath})
	if err := root.Execute(); err != nil {
		t.Fatalf("doc import dry-run changed behavior: %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run made %d remote calls", len(caller.calls))
	}
	var payload map[string]any
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("doc import dry-run stdout must be one JSON document: %v\n%s", err, output.String())
	}
	if payload["operation"] != "导入本地文件为在线文档" || payload["format"] != "md" || payload["dry_run"] != true || payload["preview_kind"] != "plan" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}

func TestExtractNodeIDFromDocURL(t *testing.T) {
	tests := map[string]string{
		"https://alidocs.dingtalk.com/i/nodes/node-1":              "node-1",
		"https://alidocs.dingtalk.com/i/nodes/node-2/?from=test":   "node-2",
		"https://alidocs.dingtalk.com/i/nodes/node-3#sheet=Sheet1": "node-3",
		"": "",
	}
	for rawURL, want := range tests {
		if got := extractNodeIDFromDocURL(rawURL); got != want {
			t.Errorf("extractNodeIDFromDocURL(%q) = %q, want %q", rawURL, got, want)
		}
	}
}
