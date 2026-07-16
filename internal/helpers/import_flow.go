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
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const importMaxFileSize int64 = 20 * 1024 * 1024

type importPollTimeoutError struct {
	taskID   string
	maxPolls int
}

func (e *importPollTimeoutError) Error() string {
	return fmt.Sprintf("导入任务超时：已轮询 %d 次仍在处理中 (taskId=%s)", e.maxPolls, e.taskID)
}

type importPollPolicy struct {
	maxPolls int
	interval func(attempt int) time.Duration
	wait     func(context.Context, time.Duration) error
}

type importFlowConfig struct {
	operation            string
	queryOperation       string
	supportedFormats     map[string]bool
	supportedFormatsText string
	folderFlags          []string
	workspaceFlags       []string
	requireTarget        bool
	serverID             string
	includeNodeID        bool
	timeoutAsResult      bool
	nextCommand          string
	poll                 importPollPolicy
}

type preparedImportFile struct {
	path      string
	name      string
	extension string
	size      int64
	folder    string
	workspace string
}

func defaultImportPollPolicy() importPollPolicy {
	return importPollPolicy{
		maxPolls: 30,
		interval: func(attempt int) time.Duration {
			switch {
			case attempt <= 5:
				return 2 * time.Second
			case attempt <= 10:
				return 5 * time.Second
			case attempt <= 20:
				return 10 * time.Second
			default:
				return 15 * time.Second
			}
		},
		wait: waitForImportPoll,
	}
}

func docImportFlowConfig() importFlowConfig {
	return importFlowConfig{
		operation:            "导入本地文件为在线文档",
		queryOperation:       "查询导入任务结果",
		supportedFormats:     map[string]bool{"docx": true, "doc": true, "xlsx": true, "xls": true, "md": true, "txt": true, "xmind": true, "mark": true},
		supportedFormatsText: "docx, doc, xlsx, xls, md, txt, xmind, mark",
		folderFlags:          []string{"folder", "folder-id"},
		workspaceFlags:       []string{"workspace", "workspace-id"},
		nextCommand:          "dws doc import get --task-id %s",
		poll:                 defaultImportPollPolicy(),
	}
}

func sheetImportFlowConfig() importFlowConfig {
	return importFlowConfig{
		operation:            "导入本地表格文件为在线电子表格",
		queryOperation:       "查询表格导入任务结果",
		supportedFormats:     map[string]bool{"xlsx": true, "xls": true},
		supportedFormatsText: "xlsx, xls",
		folderFlags:          []string{"folder-token", "folder"},
		workspaceFlags:       []string{"workspace"},
		requireTarget:        true,
		serverID:             "doc",
		includeNodeID:        true,
		timeoutAsResult:      true,
		nextCommand:          "dws sheet import get --task-id %s",
		poll:                 defaultImportPollPolicy(),
	}
}

func waitForImportPoll(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func importFlagValue(cmd *cobra.Command, names ...string) string {
	for _, name := range names {
		if cmd.Flags().Lookup(name) == nil {
			continue
		}
		if value, _ := cmd.Flags().GetString(name); value != "" {
			return value
		}
	}
	return ""
}

func prepareImportFile(cmd *cobra.Command, args []string, cfg importFlowConfig) (preparedImportFile, error) {
	filePath := mustGetFlag(cmd, "file")
	if filePath == "" && len(args) > 0 {
		filePath = args[0]
	}
	if filePath == "" {
		return preparedImportFile{}, fmt.Errorf("flag --file is required (or pass file path as argument)")
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return preparedImportFile{}, fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if fileInfo.IsDir() {
		return preparedImportFile{}, fmt.Errorf("%s is a directory, not a file", filePath)
	}
	if fileInfo.Size() > importMaxFileSize {
		return preparedImportFile{}, fmt.Errorf("file size %d bytes exceeds 20MB limit", fileInfo.Size())
	}
	if fileInfo.Size() == 0 {
		return preparedImportFile{}, fmt.Errorf("file is empty: %s", filePath)
	}

	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if !cfg.supportedFormats[extension] {
		return preparedImportFile{}, fmt.Errorf("unsupported file format %q, supported: %s", extension, cfg.supportedFormatsText)
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		fileName := filepath.Base(filePath)
		name = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}
	folder := importFlagValue(cmd, cfg.folderFlags...)
	workspace := importFlagValue(cmd, cfg.workspaceFlags...)
	if cfg.requireTarget && folder == "" && workspace == "" {
		return preparedImportFile{}, fmt.Errorf("--folder-token 与 --workspace 至少需要提供一个（导入目标位置）")
	}

	return preparedImportFile{
		path:      filePath,
		name:      name,
		extension: extension,
		size:      fileInfo.Size(),
		folder:    folder,
		workspace: workspace,
	}, nil
}

func (cfg importFlowConfig) callTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	if cfg.serverID != "" {
		return callMCPToolReturnTextOnServer(ctx, cfg.serverID, toolName, args)
	}
	return callMCPToolReturnText(ctx, toolName, args)
}

func runImportCommand(cmd *cobra.Command, args []string, cfg importFlowConfig) error {
	file, err := prepareImportFile(cmd, args, cfg)
	if err != nil {
		return err
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", cfg.operation)
		deps.Out.PrintKeyValue("文件", file.path)
		deps.Out.PrintKeyValue("名称", file.name)
		deps.Out.PrintKeyValue("格式", file.extension)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", file.size))
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	deps.Out.PrintInfo("[1/4] 创建导入会话...")
	sessionArgs := map[string]any{
		"fileName": file.name,
		"suffix":   file.extension,
		"fileSize": file.size,
	}
	if file.folder != "" {
		sessionArgs["targetFolderId"] = file.folder
	}
	if file.workspace != "" {
		sessionArgs["workspaceId"] = file.workspace
	}

	sessionText, err := cfg.callTool(ctx, "create_import_session", sessionArgs)
	if err != nil {
		return fmt.Errorf("创建导入会话失败: %w", err)
	}
	var sessionResult map[string]any
	if err := json.Unmarshal([]byte(sessionText), &sessionResult); err != nil {
		return fmt.Errorf("解析导入会话响应失败: %w", err)
	}
	sessionID, _ := sessionResult["sessionId"].(string)
	uploadURL, _ := sessionResult["uploadUrl"].(string)
	if sessionID == "" || uploadURL == "" {
		deps.Out.PrintRaw(sessionText)
		return fmt.Errorf("创建导入会话成功但缺少 sessionId 或 uploadUrl")
	}
	deps.Out.PrintInfo(fmt.Sprintf("    会话已创建，sessionId: %s", sessionID))

	deps.Out.PrintInfo("[2/4] 上传文件...")
	if err := httpPutFile(ctx, uploadURL, nil, file.path, file.size); err != nil {
		return fmt.Errorf("文件上传失败 (sessionId=%s): %w", sessionID, err)
	}
	deps.Out.PrintInfo("    文件上传完成")

	deps.Out.PrintInfo("[3/4] 确认导入，启动格式转换...")
	confirmText, err := cfg.callTool(ctx, "confirm_import", map[string]any{"sessionId": sessionID})
	if err != nil {
		return fmt.Errorf("确认导入失败 (sessionId=%s): %w", sessionID, err)
	}
	var confirmResult map[string]any
	if err := json.Unmarshal([]byte(confirmText), &confirmResult); err != nil {
		return fmt.Errorf("解析确认导入响应失败: %w", err)
	}
	taskID, _ := confirmResult["taskId"].(string)
	if taskID == "" {
		deps.Out.PrintRaw(confirmText)
		return fmt.Errorf("确认导入成功但未返回 taskId")
	}
	deps.Out.PrintInfo(fmt.Sprintf("    转换任务已提交，taskId: %s", taskID))

	deps.Out.PrintInfo("[4/4] 等待格式转换完成...")
	result, err := pollImportTask(ctx, taskID, cfg)
	if err != nil {
		var timeoutErr *importPollTimeoutError
		if !errors.As(err, &timeoutErr) {
			return err
		}
		if cfg.timeoutAsResult {
			deps.Out.PrintInfo(timeoutErr.Error())
			_ = deps.Out.PrintJSON(map[string]any{
				"success":      false,
				"timed_out":    true,
				"taskId":       taskID,
				"status":       "processing",
				"next_command": fmt.Sprintf(cfg.nextCommand, taskID),
			})
			return nil
		}
		return fmt.Errorf("%s，请稍后使用 %s 手动查询", timeoutErr.Error(), fmt.Sprintf(cfg.nextCommand, taskID))
	}

	documentURL, _ := result["documentUrl"].(string)
	documentName, _ := result["documentName"].(string)
	documentType, _ := result["documentType"].(string)
	finalResult := map[string]any{
		"success":      true,
		"taskId":       taskID,
		"documentUrl":  documentURL,
		"documentName": documentName,
		"documentType": documentType,
	}
	if cfg.includeNodeID {
		finalResult["nodeId"] = extractNodeIDFromDocURL(documentURL)
	}
	deps.Out.PrintInfo(fmt.Sprintf("导入完成: %s", documentURL))
	_ = deps.Out.PrintJSON(finalResult)
	return nil
}

func runImportGetCommand(cmd *cobra.Command, cfg importFlowConfig) error {
	taskID := mustGetFlag(cmd, "task-id")
	if taskID == "" {
		return fmt.Errorf("flag --task-id is required")
	}
	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", cfg.queryOperation)
		deps.Out.PrintKeyValue("任务ID", taskID)
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	text, err := cfg.callTool(ctx, "query_import_task", map[string]any{"taskId": taskID})
	if err != nil {
		return err
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		deps.Out.PrintRaw(text)
		return nil
	}
	status, _ := result["status"].(string)
	message, _ := result["message"].(string)
	if strings.EqualFold(status, "completed") {
		if cfg.includeNodeID {
			documentURL, _ := result["documentUrl"].(string)
			result["nodeId"] = extractNodeIDFromDocURL(documentURL)
		}
		_ = deps.Out.PrintJSON(result)
		return nil
	}
	if strings.EqualFold(status, "processing") {
		_ = deps.Out.PrintJSON(result)
		return nil
	}

	_ = deps.Out.PrintJSON(result)
	if message != "" {
		return fmt.Errorf("导入任务失败 (status=%s): %s", status, message)
	}
	return fmt.Errorf("导入任务失败 (status=%s)", status)
}

func pollImportTask(ctx context.Context, taskID string, cfg importFlowConfig) (map[string]any, error) {
	poll := cfg.poll
	if poll.maxPolls <= 0 || poll.interval == nil || poll.wait == nil {
		poll = defaultImportPollPolicy()
	}
	for attempt := 1; attempt <= poll.maxPolls; attempt++ {
		interval := poll.interval(attempt)
		deps.Out.PrintInfo(fmt.Sprintf("    第 %d/%d 次查询，等待 %v ...", attempt, poll.maxPolls, interval))
		if err := poll.wait(ctx, interval); err != nil {
			return nil, fmt.Errorf("导入轮询被取消 (taskId=%s): %w", taskID, err)
		}

		text, err := cfg.callTool(ctx, "query_import_task", map[string]any{"taskId": taskID})
		if err != nil {
			return nil, fmt.Errorf("查询导入任务失败 (taskId=%s): %w", taskID, err)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			return nil, fmt.Errorf("解析查询结果失败 (taskId=%s): %w", taskID, err)
		}
		status, _ := result["status"].(string)
		switch strings.ToLower(status) {
		case "completed":
			return result, nil
		case "processing":
			continue
		case "failed":
			message, _ := result["message"].(string)
			if message != "" {
				return nil, fmt.Errorf("导入任务失败 (taskId=%s): %s", taskID, message)
			}
			return nil, fmt.Errorf("导入任务失败 (taskId=%s)", taskID)
		}
	}
	return nil, &importPollTimeoutError{taskID: taskID, maxPolls: poll.maxPolls}
}

func extractNodeIDFromDocURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return ""
	}
	nodeID := path.Base(strings.TrimRight(parsed.Path, "/"))
	if nodeID == "." || nodeID == "/" {
		return ""
	}
	return nodeID
}
