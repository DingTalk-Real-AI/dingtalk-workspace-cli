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
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func decodeSingleAsyncDryRunPreview(t *testing.T, output string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(output))
	var preview map[string]any
	if err := decoder.Decode(&preview); err != nil {
		t.Fatalf("dry-run stdout is not one JSON object: %v; stdout=%q", err, output)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("dry-run stdout contains data after the JSON object: err=%v stdout=%q", err, output)
	}
	if preview["dryRun"] != true {
		t.Fatalf("dryRun = %#v, want true; preview=%#v", preview["dryRun"], preview)
	}
	if preview["previewKind"] != "plan" {
		t.Fatalf("previewKind = %#v, want plan; preview=%#v", preview["previewKind"], preview)
	}
	return preview
}

func requireAsyncDryRunFields(t *testing.T, preview map[string]any, want map[string]any) {
	t.Helper()
	for key, value := range want {
		if preview[key] != value {
			t.Errorf("%s = %#v, want %#v; preview=%#v", key, preview[key], value, preview)
		}
	}
}

func TestDriveTaskGetDryRunJSONIsOneSideEffectFreeQueryPreview(t *testing.T) {
	caller := &driveParityCaller{dryRun: true, format: "json"}
	output, err := executeDriveParityCommand(t, caller, "",
		"task", "get", "--type", "export", "--id", "job-1", "--dry-run", "--format", "json")
	if err != nil {
		t.Fatalf("drive task get dry-run error = %v", err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("drive task get dry-run MCP calls = %#v, want zero", caller.calls)
	}
	preview := decodeSingleAsyncDryRunPreview(t, output)
	requireAsyncDryRunFields(t, preview, map[string]any{
		"operation": "drive_task_get",
		"taskType":  "export",
		"taskId":    "job-1",
	})
}

func TestDocAsyncTaskDryRunJSONPreviewsAreModeAccurateAndSideEffectFree(t *testing.T) {
	filePath := writeImportFixture(t, "md")
	previousHTTPGet := httpGetFile
	downloadCalls := 0
	httpGetFile = func(context.Context, string, map[string]string, string) error {
		downloadCalls++
		return nil
	}
	t.Cleanup(func() { httpGetFile = previousHTTPGet })
	uploadCalls := 0
	SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
		uploadCalls++
		return nil
	})
	t.Cleanup(func() { SetHTTPPutFile(nil) })

	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "historical export parent waits and downloads",
			args: []string{"export", "--node", "node-1", "--output", "out.docx", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_export", "taskType": "export", "mode": "sync_wait", "node": "node-1", "exportFormat": "docx", "output": "out.docx"},
		},
		{
			name: "export create async only submits",
			args: []string{"export", "create", "--node", "node-2", "--export-format", "pdf", "--async", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_export", "taskType": "export", "mode": "async_submit", "node": "node-2", "exportFormat": "pdf"},
		},
		{
			name: "export get task id",
			args: []string{"export", "get", "--task-id", "job-task", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_export_get", "taskType": "export", "taskId": "job-task"},
		},
		{
			name: "export get historical job id",
			args: []string{"export", "get", "--job-id", "job-legacy", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_export_get", "taskType": "export", "taskId": "job-legacy"},
		},
		{
			name: "historical import parent waits",
			args: []string{"import", "--file", filePath, "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_import", "taskType": "import", "mode": "sync_wait", "file": filePath, "fileFormat": "md"},
		},
		{
			name: "import create async only submits",
			args: []string{"import", "create", "--file", filePath, "--name", "preview-name", "--async", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_import", "taskType": "import", "mode": "async_submit", "file": filePath, "fileFormat": "md", "fileName": "preview-name"},
		},
		{
			name: "import get",
			args: []string{"import", "get", "--task-id", "task-1", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "doc_import_get", "taskType": "import", "taskId": "task-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &docAsyncRecordingCaller{dryRun: true, format: "json"}
			output, err := runDocAsyncCapturedCommand(t, context.Background(), caller, test.args...)
			if err != nil {
				t.Fatalf("doc dry-run error = %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("doc dry-run MCP calls = %#v, want zero", caller.calls)
			}
			preview := decodeSingleAsyncDryRunPreview(t, output)
			requireAsyncDryRunFields(t, preview, test.want)
			if _, leaked := preview["uploadUrl"]; leaked {
				t.Fatalf("doc dry-run leaked uploadUrl: %#v", preview)
			}
			if _, leaked := preview["downloadUrl"]; leaked {
				t.Fatalf("doc dry-run leaked downloadUrl: %#v", preview)
			}
		})
	}

	if uploadCalls != 0 || downloadCalls != 0 {
		t.Fatalf("doc dry-run upload/download calls = %d/%d, want zero/zero", uploadCalls, downloadCalls)
	}
}

func TestSheetExportDryRunJSONPreviewsAreModeAccurateAndSideEffectFree(t *testing.T) {
	previousHTTPGet := httpGetFile
	downloadCalls := 0
	httpGetFile = func(context.Context, string, map[string]string, string) error {
		downloadCalls++
		return nil
	}
	t.Cleanup(func() { httpGetFile = previousHTTPGet })

	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "historical parent waits and downloads",
			args: []string{"--node", "sheet-1", "--output", "out.xlsx", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "sheet_export", "taskType": "export", "mode": "sync_wait", "node": "sheet-1", "exportFormat": "xlsx", "output": "out.xlsx"},
		},
		{
			name: "create async only submits",
			args: []string{"create", "--node", "sheet-2", "--async", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "sheet_export", "taskType": "export", "mode": "async_submit", "node": "sheet-2", "exportFormat": "xlsx"},
		},
		{
			name: "get task id",
			args: []string{"get", "--task-id", "sheet-job", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "sheet_export_get", "taskType": "export", "taskId": "sheet-job"},
		},
		{
			name: "get historical job id",
			args: []string{"get", "--job-id", "sheet-legacy", "--dry-run", "--format", "json"},
			want: map[string]any{"operation": "sheet_export_get", "taskType": "export", "taskId": "sheet-legacy"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &sheetExportRecordingCaller{dryRun: true, format: "json"}
			output, err := executeSheetExportAsyncCommand(t, context.Background(), caller, test.args...)
			if err != nil {
				t.Fatalf("sheet export dry-run error = %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("sheet export dry-run MCP calls = %#v, want zero", caller.calls)
			}
			preview := decodeSingleAsyncDryRunPreview(t, output)
			requireAsyncDryRunFields(t, preview, test.want)
			if _, leaked := preview["downloadUrl"]; leaked {
				t.Fatalf("sheet export dry-run leaked downloadUrl: %#v", preview)
			}
		})
	}
	if downloadCalls != 0 {
		t.Fatalf("sheet export dry-run download calls = %d, want zero", downloadCalls)
	}
}

func TestAsyncTaskDryRunTablePreviewRemainsReadableAndModeAccurate(t *testing.T) {
	driveCaller := &driveParityCaller{dryRun: true, format: "table"}
	driveOutput, err := executeDriveParityCommand(t, driveCaller, "",
		"task", "get", "--type", "import", "--id", "task-table", "--dry-run", "--format", "table")
	if err != nil {
		t.Fatalf("drive table dry-run error = %v", err)
	}
	for _, fragment := range []string{"查询异步任务", "导入", "task-table", "未执行"} {
		if !strings.Contains(driveOutput, fragment) {
			t.Errorf("drive table dry-run missing %q: %q", fragment, driveOutput)
		}
	}

	docCaller := &docAsyncRecordingCaller{dryRun: true, format: "table"}
	docOutput, err := runDocAsyncCapturedCommand(t, context.Background(), docCaller,
		"export", "create", "--node", "node-table", "--async", "--dry-run", "--format", "table")
	if err != nil {
		t.Fatalf("doc table dry-run error = %v", err)
	}
	for _, fragment := range []string{"导出文档", "只提交", "node-table", "未执行"} {
		if !strings.Contains(docOutput, fragment) {
			t.Errorf("doc table dry-run missing %q: %q", fragment, docOutput)
		}
	}

	importFile := writeImportFixture(t, "md")
	importOutput, err := runDocAsyncCapturedCommand(t, context.Background(), docCaller,
		"import", "create", "--file", importFile, "--async", "--dry-run", "--format", "table")
	if err != nil {
		t.Fatalf("doc import async table dry-run error = %v", err)
	}
	for _, fragment := range []string{"导入本地文件", "创建会话", "上传", "确认", "不轮询", "未执行"} {
		if !strings.Contains(importOutput, fragment) {
			t.Errorf("doc import async table dry-run missing %q: %q", fragment, importOutput)
		}
	}
	if strings.Contains(importOutput, "下载") {
		t.Errorf("doc import async table dry-run incorrectly promises a download: %q", importOutput)
	}

	importSyncOutput, err := runDocAsyncCapturedCommand(t, context.Background(), docCaller,
		"import", "--file", importFile, "--dry-run", "--format", "table")
	if err != nil {
		t.Fatalf("doc import sync table dry-run error = %v", err)
	}
	for _, fragment := range []string{"创建会话", "上传", "确认", "等待转换完成"} {
		if !strings.Contains(importSyncOutput, fragment) {
			t.Errorf("doc import sync table dry-run missing %q: %q", fragment, importSyncOutput)
		}
	}
	if strings.Contains(importSyncOutput, "下载") {
		t.Errorf("doc import sync table dry-run incorrectly promises a download: %q", importSyncOutput)
	}

	sheetCaller := &sheetExportRecordingCaller{dryRun: true, format: "table"}
	sheetOutput, err := executeSheetExportAsyncCommand(t, context.Background(), sheetCaller,
		"--node", "sheet-table", "--output", "table.xlsx", "--dry-run", "--format", "table")
	if err != nil {
		t.Fatalf("sheet table dry-run error = %v", err)
	}
	for _, fragment := range []string{"导出表格", "等待完成", "table.xlsx", "未执行"} {
		if !strings.Contains(sheetOutput, fragment) {
			t.Errorf("sheet table dry-run missing %q: %q", fragment, sheetOutput)
		}
	}
}
