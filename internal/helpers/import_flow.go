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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
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
	exclusiveTarget      bool
	defaultTargetSource  string
	resolveDefaultTarget func(context.Context) (importTarget, error)
	serverID             string
	includeNodeID        bool
	verifyPlacement      bool
	timeoutAsResult      bool
	nextCommand          string
	poll                 importPollPolicy
	// uploadFallback 开启后，所有不在 supportedFormats 白名单内的文件
	// （html/pdf/zip/无扩展名等）不再报错断链，统一移交文档空间文件上传
	// 链路原样入库；白名单即后端转换能力的封闭集合，无需第二份格式枚举。
	// 回退共享 prepareImportFile 的存在性 / 20MB / 空文件校验。
	uploadFallback bool
}

type preparedImportFile struct {
	path         string
	name         string
	extension    string
	size         int64
	folder       string
	workspace    string
	targetSource string
}

type importTarget struct {
	folder    string
	workspace string
	source    string
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
		exclusiveTarget:      true,
		defaultTargetSource:  "default_org_root",
		resolveDefaultTarget: resolveDefaultDocImportTarget,
		includeNodeID:        true,
		verifyPlacement:      true,
		nextCommand:          "dws doc import get --task-id %s",
		poll:                 defaultImportPollPolicy(),
		// 白名单外的格式改走文档空间的文件上传链路
		// （与 drive upload --workspace 同一条 doc-space 上传原语），
		// 目标 flags（--folder/--workspace）与 import 同构，链路不中断。
		uploadFallback: true,
	}
}

func importTargetValidationError(message string, cause error) error {
	options := []apperrors.Option{
		apperrors.WithOperation("doc.import.resolve_target"),
		apperrors.WithReason("import_target_unresolved"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithActions("显式提供 --folder <目标文件夹ID> 或 --workspace <目标知识库ID> 后重新执行"),
	}
	if cause != nil {
		options = append(options, apperrors.WithCause(cause))
	}
	return apperrors.NewValidation(message, options...)
}

func parseDefaultDocImportTarget(text string) (importTarget, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return importTarget{}, importTargetValidationError("无法解析当前组织的默认文档根目录；请显式提供 --folder 或 --workspace", err)
	}
	if nextToken, exists := payload["nextToken"]; exists && nextToken != nil {
		token, ok := nextToken.(string)
		if !ok || strings.TrimSpace(token) != "" {
			return importTarget{}, importTargetValidationError("当前组织空间列表仍有下一页，无法证明默认导入位置唯一；请显式提供 --folder 或 --workspace", nil)
		}
	}
	if hasMore, exists := payload["hasMore"]; exists && hasMore != nil {
		more, ok := hasMore.(bool)
		if !ok || more {
			return importTarget{}, importTargetValidationError("当前组织空间列表仍有下一页，无法证明默认导入位置唯一；请显式提供 --folder 或 --workspace", nil)
		}
	}
	if result, ok := payload["result"].(map[string]any); ok {
		payload = result
	}
	if nextToken, exists := payload["nextToken"]; exists && nextToken != nil {
		token, ok := nextToken.(string)
		if !ok || strings.TrimSpace(token) != "" {
			return importTarget{}, importTargetValidationError("当前组织空间列表仍有下一页，无法证明默认导入位置唯一；请显式提供 --folder 或 --workspace", nil)
		}
	}
	if hasMore, exists := payload["hasMore"]; exists && hasMore != nil {
		more, ok := hasMore.(bool)
		if !ok || more {
			return importTarget{}, importTargetValidationError("当前组织空间列表仍有下一页，无法证明默认导入位置唯一；请显式提供 --folder 或 --workspace", nil)
		}
	}
	items, ok := payload["items"].([]any)
	if !ok {
		return importTarget{}, importTargetValidationError("当前组织空间响应缺少 items，无法确定默认导入位置；请显式提供 --folder 或 --workspace", nil)
	}
	if len(items) != 1 {
		return importTarget{}, importTargetValidationError(fmt.Sprintf("当前组织返回 %d 个 orgSpace，无法唯一确定默认导入位置；请显式提供 --folder 或 --workspace", len(items)), nil)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		return importTarget{}, importTargetValidationError("当前组织唯一 orgSpace 的返回结构无效；请显式提供 --folder 或 --workspace", nil)
	}
	if spaceType, _ := item["spaceType"].(string); spaceType != "" && spaceType != "orgSpace" {
		return importTarget{}, importTargetValidationError(fmt.Sprintf("默认空间类型为 %q，不是 orgSpace；请显式提供 --folder 或 --workspace", spaceType), nil)
	}
	rootFolderID, _ := item["rootFolderId"].(string)
	rootFolderID = strings.TrimSpace(rootFolderID)
	if rootFolderID == "" {
		return importTarget{}, importTargetValidationError("当前组织唯一 orgSpace 未返回 rootFolderId；请显式提供 --folder 或 --workspace", nil)
	}
	return importTarget{folder: rootFolderID, source: "default_org_root"}, nil
}

func resolveDefaultDocImportTarget(ctx context.Context) (importTarget, error) {
	text, err := callMCPToolReturnTextOnServer(ctx, "drive", "list_spaces", map[string]any{"spaceType": "orgSpace"})
	if err != nil {
		return importTarget{}, importTargetValidationError("无法读取当前组织的默认文档根目录；请显式提供 --folder 或 --workspace", err)
	}
	return parseDefaultDocImportTarget(text)
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
	// 非回退配置保持基线校验顺序：扩展名门禁先于导入目标校验
	// （sheet import 对无目标的非 Excel 文件必须先报 unsupported）。
	// uploadFallback 配置的白名单外文件继续走完共享校验，由
	// runImportCommand 分派到上传回退。
	if !cfg.supportedFormats[extension] && !cfg.uploadFallback {
		return preparedImportFile{}, fmt.Errorf("unsupported file format %q, supported: %s", extension, cfg.supportedFormatsText)
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		fileName := filepath.Base(filePath)
		name = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}
	folder := importFlagValue(cmd, cfg.folderFlags...)
	workspace := importFlagValue(cmd, cfg.workspaceFlags...)
	if cfg.exclusiveTarget && folder != "" && workspace != "" {
		return preparedImportFile{}, importTargetValidationError("--folder 与 --workspace 不能同时提供；请选择一个明确的导入目标", nil)
	}
	if cfg.requireTarget && folder == "" && workspace == "" {
		return preparedImportFile{}, fmt.Errorf("--folder-token 与 --workspace 至少需要提供一个（导入目标位置）")
	}
	targetSource := ""
	if folder != "" {
		targetSource = "explicit_folder"
	} else if workspace != "" {
		targetSource = "explicit_workspace"
	}

	return preparedImportFile{
		path:         filePath,
		name:         name,
		extension:    extension,
		size:         fileInfo.Size(),
		folder:       folder,
		workspace:    workspace,
		targetSource: targetSource,
	}, nil
}

func addImportTargetReceipt(result map[string]any, file preparedImportFile, deferredSource string) {
	if file.folder != "" {
		result["targetFolderId"] = file.folder
	}
	if file.workspace != "" {
		result["workspaceId"] = file.workspace
	}
	if file.targetSource != "" {
		result["targetSource"] = file.targetSource
		return
	}
	if deferredSource != "" {
		result["targetSource"] = deferredSource
		result["targetResolution"] = "deferred"
	}
}

func importString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func parseImportedDocumentInfo(text string) (map[string]any, error) {
	var root any
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return nil, fmt.Errorf("解析文档元信息响应失败: %w", err)
	}
	if info, ok := findImportedDocumentInfo(root); ok {
		return info, nil
	}
	return nil, fmt.Errorf("文档元信息响应缺少 nodeId/folderId/workspaceId")
}

func findImportedDocumentInfo(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	if importString(object, "nodeId", "fileId", "dentryUuid", "folderId", "workspaceId") != "" {
		return object, true
	}
	for _, key := range []string{"result", "data", "documentInfo", "document", "doc", "file"} {
		if nested, exists := object[key]; exists {
			if info, ok := findImportedDocumentInfo(nested); ok {
				return info, true
			}
		}
	}
	return nil, false
}

func canonicalImportTargetID(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme == "" && parsed.Host == "") {
		return raw
	}
	for _, key := range []string{"workspaceId", "spaceId", "folderId", "nodeId"} {
		if value := strings.TrimSpace(parsed.Query().Get(key)); value != "" {
			return value
		}
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for index, segment := range segments {
		if index+1 >= len(segments) {
			break
		}
		switch segment {
		case "nodes", "spaces", "folders":
			if value := strings.TrimSpace(segments[index+1]); value != "" {
				return value
			}
		}
	}
	return raw
}

func verifyImportedDocumentPlacement(ctx context.Context, file preparedImportFile, taskID, documentURL string) (string, map[string]any, error) {
	nodeID := extractNodeIDFromDocURL(documentURL)
	if nodeID == "" {
		return "", nil, docImportPlacementError(file, taskID, "", nil, fmt.Errorf("导入结果缺少可解析的 documentUrl"))
	}
	return verifyImportedNodePlacement(ctx, file, taskID, nodeID)
}

func verifyImportedNodePlacement(ctx context.Context, file preparedImportFile, taskID, nodeID string) (string, map[string]any, error) {
	text, err := callMCPToolReturnTextOnServer(ctx, "doc", "get_document_info", map[string]any{"nodeId": nodeID})
	if err != nil {
		return nodeID, nil, docImportPlacementError(file, taskID, nodeID, nil, err)
	}
	info, err := parseImportedDocumentInfo(text)
	if err != nil {
		return nodeID, nil, docImportPlacementError(file, taskID, nodeID, nil, err)
	}

	observedNodeID := importString(info, "nodeId", "fileId", "dentryUuid", "id")
	if observedNodeID != "" && observedNodeID != nodeID {
		return nodeID, info, docImportPlacementError(file, taskID, nodeID, info,
			fmt.Errorf("回读 nodeId=%s，与导入结果 nodeId=%s 不一致", observedNodeID, nodeID))
	}
	if file.folder != "" {
		expected := canonicalImportTargetID(file.folder)
		observed := importString(info, "folderId")
		if observed == "" || observed != expected {
			return nodeID, info, docImportPlacementError(file, taskID, nodeID, info,
				fmt.Errorf("目标文件夹验证失败：expected=%s observed=%s", expected, observed))
		}
	}
	if file.workspace != "" {
		expected := canonicalImportTargetID(file.workspace)
		observed := importString(info, "workspaceId")
		if observed == "" || observed != expected {
			return nodeID, info, docImportPlacementError(file, taskID, nodeID, info,
				fmt.Errorf("目标知识库验证失败：expected=%s observed=%s", expected, observed))
		}
	}
	return nodeID, compactImportedDocumentInfo(info, nodeID), nil
}

func compactImportedDocumentInfo(info map[string]any, nodeID string) map[string]any {
	result := map[string]any{"nodeId": nodeID}
	for _, key := range []string{"folderId", "workspaceId", "name", "contentType"} {
		if value := importString(info, key); value != "" {
			result[key] = value
		}
	}
	return result
}

func importTargetSummary(file preparedImportFile) map[string]any {
	result := map[string]any{"source": file.targetSource}
	if file.folder != "" {
		result["folderId"] = canonicalImportTargetID(file.folder)
	}
	if file.workspace != "" {
		result["workspaceId"] = canonicalImportTargetID(file.workspace)
	}
	return result
}

func docImportPlacementError(file preparedImportFile, taskID, nodeID string, observed map[string]any, cause error) error {
	details := map[string]any{
		"status":   "partial_success",
		"nodeId":   nodeID,
		"target":   importTargetSummary(file),
		"verified": false,
	}
	if taskID != "" {
		details["taskId"] = taskID
	}
	if observed != nil {
		details["observed"] = compactImportedDocumentInfo(observed, nodeID)
	}
	return apperrors.NewAPI(
		"导入或上传已经完成，但目标落点回读验证失败；为避免重复创建，请先按 nodeId 检查文档",
		apperrors.WithOperation("doc.import"),
		apperrors.WithReason("doc_import_placement_unverified"),
		apperrors.WithFailureStage("verify_placement"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithActions("按 nodeId 检查文档当前位置", "确认导入结果前不要重复执行"),
		apperrors.WithDetails(details),
		apperrors.WithCause(cause),
	)
}

func (cfg importFlowConfig) callTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	if cfg.serverID != "" {
		return callMCPToolReturnTextOnServer(ctx, cfg.serverID, toolName, args)
	}
	return callMCPToolReturnText(ctx, toolName, args)
}

// runImportUploadFallback 承接白名单外格式：不再报错断链，改走文档空间
// 文件上传链路原样入库。回退在 prepareImportFile 之后执行，共享存在性 /
// 20MB / 空文件校验；不复用 runDocUpload，避免携带 doc upload 的
// --workspace 兼容告警。移交事实通过 stderr 显式告知，机器可读结果统一
// 携带 fallback=upload / converted=false 标记，防止 Agent 误判已完成
// 在线文档转换。
func runImportUploadFallback(cmd *cobra.Command, cfg importFlowConfig, file preparedImportFile) error {
	label := file.extension
	if label == "" {
		label = "无扩展名"
	}
	deps.Out.PrintWarning(fmt.Sprintf(
		"%s 文件不支持转换为在线文档（支持: %s），已自动改走文件上传链路，以原文件形式存入解析出的文档目标位置；如需在线文档，请先将内容转换为 md 后重新执行 doc import；上传到钉盘请用 dws drive upload",
		label, cfg.supportedFormatsText))

	// prepareImportFile 的 name 去掉了扩展名；上传保留原始文件名形态
	uploadName := file.name
	if filepath.Ext(uploadName) == "" && file.extension != "" {
		uploadName += "." + file.extension
	}
	jsonMode := deps.Caller.Format() == "json"

	if deps.Caller.DryRun() {
		if jsonMode {
			result := map[string]any{
				"dry_run":             true,
				"executed":            false,
				"preview_kind":        "plan",
				"operation":           "上传文件到钉钉文档",
				"requested_operation": cfg.operation,
				"fallback":            "upload",
				"converted":           false,
				"file":                file.path,
				"name":                uploadName,
				"format":              file.extension,
				"size":                file.size,
			}
			addImportTargetReceipt(result, file, cfg.defaultTargetSource)
			return deps.Out.PrintJSON(result)
		}
		deps.Out.PrintKeyValue("操作", "上传文件到钉钉文档（doc import 回退）")
		deps.Out.PrintKeyValue("文件", file.path)
		deps.Out.PrintKeyValue("名称", uploadName)
		deps.Out.PrintKeyValue("格式", file.extension)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", file.size))
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if !jsonMode {
		deps.Out.PrintInfo("按原文件上传中（未转换为在线文档）...")
	}
	text, err := docSpaceUploadCommitText(ctx, file.path, uploadName, file.size, file.folder, file.workspace)
	if err != nil {
		return err
	}
	// fail-closed：commit 响应必须可解析且带文件标识才算成功；
	// 空响应（legacy ack）、非 JSON 或缺少标识都不得包装为 success
	commit, dentryID, err := parseUploadCommitResult(text)
	if err != nil {
		return err
	}
	var verification map[string]any
	if cfg.verifyPlacement {
		_, verification, err = verifyImportedNodePlacement(ctx, file, "", dentryID)
		if err != nil {
			return err
		}
	}
	result := map[string]any{
		"success":             true,
		"operation":           "上传文件到钉钉文档",
		"requested_operation": cfg.operation,
		"fallback":            "upload",
		"converted":           false,
		"name":                uploadName,
		"format":              file.extension,
		"dentry_id":           dentryID,
		"result":              commit,
	}
	addImportTargetReceipt(result, file, "")
	if cfg.verifyPlacement {
		result["verified"] = true
		result["target"] = importTargetSummary(file)
		result["verification"] = verification
	}
	return deps.Out.PrintJSON(result)
}

// uploadCommitIDKeys 是 commit_uploaded_file 响应中可作为文件标识的字段，
// 按优先级排列；服务端可能返回平铺对象或包一层 result envelope。
var uploadCommitIDKeys = []string{"dentryUuid", "dentryId", "nodeId", "fileId", "id"}

// parseUploadCommitResult 校验入库响应：拒绝空响应，要求 JSON 对象且
// 含文件标识，返回解析后的对象与标识值。任何不满足都返回错误，
// 由调用方向用户提示核对入库结果，而不是伪装成功。
func parseUploadCommitResult(text string) (map[string]any, string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, "", fmt.Errorf("上传入库未返回结果（commit_uploaded_file 响应为空），无法确认文件已入库；请用 dws doc list 核对目标位置")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, "", fmt.Errorf("上传入库响应无法解析为 JSON，无法确认文件已入库；原始响应: %s", trimmed)
	}
	payload := parsed
	if inner, ok := parsed["result"].(map[string]any); ok {
		payload = inner
	}
	for _, key := range uploadCommitIDKeys {
		if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
			return parsed, v, nil
		}
	}
	return nil, "", fmt.Errorf("上传入库响应缺少文件标识（%s 均为空），无法确认文件已入库；原始响应: %s", strings.Join(uploadCommitIDKeys, "/"), trimmed)
}

func runImportCommand(cmd *cobra.Command, args []string, cfg importFlowConfig) error {
	file, err := prepareImportFile(cmd, args, cfg)
	if err != nil {
		return err
	}
	uploadFallback := cfg.uploadFallback && !cfg.supportedFormats[file.extension]
	jsonMode := deps.Caller.Format() == "json"

	if deps.Caller.DryRun() {
		if uploadFallback {
			return runImportUploadFallback(cmd, cfg, file)
		}
		if jsonMode {
			result := map[string]any{
				"dry_run":      true,
				"executed":     false,
				"preview_kind": "plan",
				"operation":    cfg.operation,
				"file":         file.path,
				"name":         file.name,
				"format":       file.extension,
				"size":         file.size,
			}
			addImportTargetReceipt(result, file, cfg.defaultTargetSource)
			return deps.Out.PrintJSON(result)
		}
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
	if file.folder == "" && file.workspace == "" && cfg.resolveDefaultTarget != nil {
		target, err := cfg.resolveDefaultTarget(ctx)
		if err != nil {
			return err
		}
		if target.folder == "" && target.workspace == "" {
			return importTargetValidationError("默认导入目标解析结果为空；请显式提供 --folder 或 --workspace", nil)
		}
		file.folder = target.folder
		file.workspace = target.workspace
		file.targetSource = target.source
	}
	// 非回退配置的白名单外文件已在 prepareImportFile 中按基线顺序拒绝。
	// 回退上传与在线转换共享同一目标解析，避免仅因扩展名不同而绕过
	// 默认 orgSpace 唯一性校验或落入服务端隐式位置。
	if uploadFallback {
		return runImportUploadFallback(cmd, cfg, file)
	}

	if !jsonMode {
		deps.Out.PrintInfo("[1/4] 创建导入会话...")
	}
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
		if !jsonMode {
			deps.Out.PrintRaw(sessionText)
		}
		return fmt.Errorf("创建导入会话成功但缺少 sessionId 或 uploadUrl")
	}
	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("    会话已创建，sessionId: %s", sessionID))
	}

	if !jsonMode {
		deps.Out.PrintInfo("[2/4] 上传文件...")
	}
	if err := httpPutFile(ctx, uploadURL, nil, file.path, file.size); err != nil {
		return fmt.Errorf("文件上传失败 (sessionId=%s): %w", sessionID, err)
	}
	if !jsonMode {
		deps.Out.PrintInfo("    文件上传完成")
	}

	if !jsonMode {
		deps.Out.PrintInfo("[3/4] 确认导入，启动格式转换...")
	}
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
		if !jsonMode {
			deps.Out.PrintRaw(confirmText)
		}
		return fmt.Errorf("确认导入成功但未返回 taskId")
	}
	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("    转换任务已提交，taskId: %s", taskID))
	}

	if !jsonMode {
		deps.Out.PrintInfo("[4/4] 等待格式转换完成...")
	}
	result, err := pollImportTask(ctx, taskID, cfg)
	if err != nil {
		var timeoutErr *importPollTimeoutError
		if !errors.As(err, &timeoutErr) {
			return fmt.Errorf("%w；任务已经提交，可使用 %s 继续查询", err, importRecoveryCommand(cfg, taskID, file))
		}
		if cfg.timeoutAsResult {
			if !jsonMode {
				deps.Out.PrintInfo(timeoutErr.Error())
			}
			return deps.Out.PrintJSON(map[string]any{
				"success":      false,
				"timed_out":    true,
				"taskId":       taskID,
				"status":       "processing",
				"next_command": importRecoveryCommand(cfg, taskID, file),
			})
		}
		return fmt.Errorf("%s，请稍后使用 %s 手动查询", timeoutErr.Error(), importRecoveryCommand(cfg, taskID, file))
	}

	documentURL, _ := result["documentUrl"].(string)
	documentName, _ := result["documentName"].(string)
	documentType, _ := result["documentType"].(string)
	nodeID := extractNodeIDFromDocURL(documentURL)
	var verification map[string]any
	if cfg.verifyPlacement {
		var err error
		nodeID, verification, err = verifyImportedDocumentPlacement(ctx, file, taskID, documentURL)
		if err != nil {
			return err
		}
	}
	finalResult := map[string]any{
		"success":      true,
		"taskId":       taskID,
		"documentUrl":  documentURL,
		"documentName": documentName,
		"documentType": documentType,
	}
	addImportTargetReceipt(finalResult, file, "")
	if cfg.includeNodeID {
		finalResult["nodeId"] = nodeID
	}
	if cfg.verifyPlacement {
		finalResult["verified"] = true
		finalResult["target"] = importTargetSummary(file)
		finalResult["verification"] = verification
	}
	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("导入完成: %s", documentURL))
	}
	return deps.Out.PrintJSON(finalResult)
}

func importRecoveryCommand(cfg importFlowConfig, taskID string, file preparedImportFile) string {
	command := fmt.Sprintf(cfg.nextCommand, ShellQuoteArg(taskID))
	if !cfg.verifyPlacement {
		return command
	}
	if file.folder != "" {
		command += " --folder " + ShellQuoteArg(file.folder)
	}
	if file.workspace != "" {
		command += " --workspace " + ShellQuoteArg(file.workspace)
	}
	return command
}

func runImportGetCommand(cmd *cobra.Command, cfg importFlowConfig) error {
	taskID := mustGetFlag(cmd, "task-id")
	if taskID == "" {
		return fmt.Errorf("flag --task-id is required")
	}
	target := preparedImportFile{
		folder:    importFlagValue(cmd, cfg.folderFlags...),
		workspace: importFlagValue(cmd, cfg.workspaceFlags...),
	}
	if target.folder != "" {
		target.targetSource = "explicit_folder"
	} else if target.workspace != "" {
		target.targetSource = "explicit_workspace"
	}
	if deps.Caller.DryRun() {
		if deps.Caller.Format() == "json" {
			preview := map[string]any{
				"dry_run":      true,
				"executed":     false,
				"preview_kind": "plan",
				"operation":    cfg.queryOperation,
				"taskId":       taskID,
			}
			if cfg.verifyPlacement {
				preview["target"] = importTargetSummary(target)
			}
			return deps.Out.PrintJSON(preview)
		}
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
		return fmt.Errorf("解析导入任务响应失败 (taskId=%s)，请重试 %s: %w", taskID, importRecoveryCommand(cfg, taskID, target), err)
	}
	status, _ := result["status"].(string)
	message, _ := result["message"].(string)
	if strings.EqualFold(status, "completed") {
		documentURL, _ := result["documentUrl"].(string)
		nodeID := extractNodeIDFromDocURL(documentURL)
		if cfg.verifyPlacement {
			if target.folder == "" && target.workspace == "" {
				result["verified"] = false
			} else {
				var verification map[string]any
				nodeID, verification, err = verifyImportedDocumentPlacement(ctx, target, taskID, documentURL)
				if err != nil {
					return err
				}
				result["verified"] = true
				result["target"] = importTargetSummary(target)
				result["verification"] = verification
			}
		}
		if cfg.includeNodeID {
			result["nodeId"] = nodeID
		}
		return deps.Out.PrintJSON(result)
	}
	if strings.EqualFold(status, "processing") {
		return deps.Out.PrintJSON(result)
	}

	if err := deps.Out.PrintJSON(result); err != nil {
		return err
	}
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
		if deps.Caller.Format() != "json" {
			deps.Out.PrintInfo(fmt.Sprintf("    第 %d/%d 次查询，等待 %v ...", attempt, poll.maxPolls, interval))
		}
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
