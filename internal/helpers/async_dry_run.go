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
	"fmt"
	"strings"
)

const (
	asyncDryRunModeSubmit = "async_submit"
	asyncDryRunModeWait   = "sync_wait"
)

// asyncTaskDryRunPreview is the stable, side-effect-free preview shared only
// by the unified asynchronous task commands. It intentionally excludes
// credentials, upload URLs, and download URLs.
type asyncTaskDryRunPreview struct {
	DryRun       bool   `json:"dryRun"`
	PreviewKind  string `json:"previewKind"`
	Operation    string `json:"operation"`
	TaskType     string `json:"taskType"`
	Mode         string `json:"mode,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	Node         string `json:"node,omitempty"`
	File         string `json:"file,omitempty"`
	FileName     string `json:"fileName,omitempty"`
	FileFormat   string `json:"fileFormat,omitempty"`
	FileSize     int64  `json:"fileSize,omitempty"`
	Folder       string `json:"folder,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	ExportFormat string `json:"exportFormat,omitempty"`
	Output       string `json:"output,omitempty"`
}

func asyncDryRunMode(asyncMode bool) string {
	if asyncMode {
		return asyncDryRunModeSubmit
	}
	return asyncDryRunModeWait
}

func printAsyncTaskDryRunPreview(preview asyncTaskDryRunPreview) error {
	preview.DryRun = true
	preview.PreviewKind = "plan"
	if strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json") {
		return deps.Out.PrintJSON(preview)
	}

	operationLabels := map[string]string{
		"drive_task_get":   "查询异步任务",
		"doc_export":       "导出文档",
		"doc_export_get":   "查询文档导出任务",
		"doc_import":       "导入本地文件为在线文档",
		"doc_import_get":   "查询文档导入任务",
		"sheet_export":     "导出表格为 xlsx",
		"sheet_export_get": "查询表格导出任务结果",
	}
	taskTypeLabels := map[string]string{"export": "导出", "import": "导入"}
	operation := operationLabels[preview.Operation]
	if operation == "" {
		operation = preview.Operation
	}
	taskType := taskTypeLabels[preview.TaskType]
	if taskType == "" {
		taskType = preview.TaskType
	}
	deps.Out.PrintKeyValue("预览", "未执行（--dry-run）")
	deps.Out.PrintKeyValue("操作", operation)
	deps.Out.PrintKeyValue("任务类型", taskType)
	if preview.Mode != "" {
		mode := asyncDryRunModeLabel(preview.Operation, preview.Mode)
		if mode == "" {
			mode = preview.Mode
		}
		deps.Out.PrintKeyValue("模式", mode)
	}
	if preview.TaskID != "" {
		deps.Out.PrintKeyValue("任务ID", preview.TaskID)
	}
	if preview.Node != "" {
		deps.Out.PrintKeyValue("节点", preview.Node)
	}
	if preview.File != "" {
		deps.Out.PrintKeyValue("文件", preview.File)
	}
	if preview.FileName != "" {
		deps.Out.PrintKeyValue("名称", preview.FileName)
	}
	if preview.FileFormat != "" {
		deps.Out.PrintKeyValue("文件格式", preview.FileFormat)
	}
	if preview.FileSize > 0 {
		deps.Out.PrintKeyValue("文件大小", fmt.Sprintf("%d bytes", preview.FileSize))
	}
	if preview.Folder != "" {
		deps.Out.PrintKeyValue("目标文件夹", preview.Folder)
	}
	if preview.Workspace != "" {
		deps.Out.PrintKeyValue("目标知识库", preview.Workspace)
	}
	if preview.ExportFormat != "" {
		deps.Out.PrintKeyValue("导出格式", preview.ExportFormat)
	}
	if preview.Output != "" {
		deps.Out.PrintKeyValue("输出", preview.Output)
	}
	return nil
}

func asyncDryRunModeLabel(operation, mode string) string {
	if operation == "doc_import" {
		switch mode {
		case asyncDryRunModeSubmit:
			return "创建会话、上传文件并确认任务后返回，不轮询"
		case asyncDryRunModeWait:
			return "创建会话、上传文件并确认任务后等待转换完成"
		}
	}
	switch mode {
	case asyncDryRunModeSubmit:
		return "只提交任务，不轮询或下载"
	case asyncDryRunModeWait:
		return "提交后等待完成，并按需下载"
	default:
		return ""
	}
}
