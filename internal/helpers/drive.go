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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return driveHandler{}
	})
}

// driveHandler exposes the drive command surface that the service-discovery
// envelope may not express consistently in older/pre caches:
//
//   - upload — three-step composite: drive.get_upload_info → HTTP PUT to OSS →
//     drive.commit_upload. The envelope PipelineStep schema currently supports
//     type:"call" (MCP tool invocation) and type:"download" (HTTP GET sink),
//     but has no type:"upload" for streaming a local file to an OSS-signed
//     PUT URL with per-URL headers. Until the envelope schema grows that
//     capability, this helper is the canonical client-side glue.
//
// For the remaining leaves the helper keeps the same command names as the
// dynamic envelope, but sets a higher override priority so the local binary can
// provide stable validation and aliases without depending on shared config
// rollout timing.
type driveHandler struct{}

func (driveHandler) Name() string {
	return "drive"
}

func (driveHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "drive",
		Short:             "钉盘扩展命令（合并到 dws drive 命令树）",
		Long:              "钉盘扩展子命令: 文件列表、详情、下载、创建文件夹、上传流程、删除等。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(
		newDriveListCommand(runner),
		newDriveListSpacesCommand(runner),
		newDriveInfoCommand(runner),
		newDriveDownloadCommand(runner),
		newDriveMkdirCommand(runner),
		newDriveUploadInfoCommand(runner),
		newDriveCommitCommand(runner),
		newDriveUploadCommand(runner),
		newDriveDeleteCommand(runner),
		newDriveTreeCommand(runner),
	)
	return root
}

// ── dynamic-compatible leaves ──────────────────────────────

func newDriveListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "获取文件/文件夹列表",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			maxResults := driveIntFlagOrFallback(cmd, "max", "limit", "page-size")
			if maxResults <= 0 {
				maxResults = 20
			}
			params := map[string]any{"maxResults": float64(maxResults)}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			if parentID := driveFlagOrFallback(cmd, "parent-id", "folder"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			addDriveStringParam(cmd, params, "nextToken", "next-token", "cursor")
			addDriveStringParam(cmd, params, "orderBy", "order-by")
			addDriveStringParam(cmd, params, "order", "order")
			if thumbnail, _ := cmd.Flags().GetBool("thumbnail"); thumbnail {
				params["withThumbnail"] = true
			}
			return runDriveInvocation(cmd, runner, "drive", "list_files", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Int("max", 20, "每页返回数量，默认 20，最大 100")
	cmd.Flags().Int("limit", 0, "--max 的兼容别名")
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().Int("page-size", 0, "--max 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("space-id", "", "空间 ID，不传则使用「我的文件」")
	cmd.Flags().String("parent-id", "", "父节点 ID (dentryUuid)")
	addDriveHiddenStringFlag(cmd, "folder", "--parent-id 的兼容别名")
	cmd.Flags().String("next-token", "", "分页游标")
	addDriveHiddenStringFlag(cmd, "cursor", "--next-token 的兼容别名")
	cmd.Flags().String("order-by", "", "排序字段")
	cmd.Flags().String("order", "", "排序方向: asc|desc")
	cmd.Flags().Bool("thumbnail", false, "是否返回缩略图信息")
	return cmd
}

func newDriveListSpacesCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list-spaces",
		Short:             "获取钉盘空间列表",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			maxResults, _ := cmd.Flags().GetInt("max")
			if !cmd.Flags().Changed("max") {
				if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
					maxResults = limit
				}
			}
			if maxResults > 0 {
				params["maxResults"] = float64(maxResults)
			}
			addDriveStringParam(cmd, params, "spaceType", "space-type")
			addDriveStringParam(cmd, params, "nextToken", "next-token", "cursor")
			return runDriveInvocation(cmd, runner, "drive", "list_spaces", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Int("max", 20, "每页返回数量")
	cmd.Flags().Int("limit", 0, "--max 的兼容别名")
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("space-type", "", "空间类型: orgSpace|mySpace")
	cmd.Flags().String("next-token", "", "分页游标")
	addDriveHiddenStringFlag(cmd, "cursor", "--next-token 的兼容别名")
	return cmd
}

func newDriveInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "info",
		Short:             "获取文件元数据信息",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := driveRequiredFlagOrFallback(cmd, "file-id", "node")
			if err != nil {
				return err
			}
			params := map[string]any{"fileId": fileID}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			return runDriveInfo(cmd, runner, params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-id", "", "节点 ID (dentryUuid) (必填)")
	addDriveHiddenStringFlag(cmd, "node", "--file-id 的兼容别名")
	cmd.Flags().String("space-id", "", "节点所属空间 ID")
	return cmd
}

func newDriveDownloadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "download",
		Short:             "下载钉盘文件到本地",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDriveDownload(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-id", "", "文件 ID (dentryUuid) (必填)")
	addDriveHiddenStringFlag(cmd, "node", "--file-id 的兼容别名")
	cmd.Flags().String("space-id", "", "文件所属空间 ID")
	cmd.Flags().String("output", "", "本地保存路径 (文件路径或目录，必填)")
	return cmd
}

func newDriveMkdirCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "mkdir",
		Short:             "创建文件夹",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := driveRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			if parentID := driveFlagOrFallback(cmd, "parent-id", "folder"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			return runDriveInvocation(cmd, runner, "drive", "create_folder", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", "文件夹名称 (必填)")
	cmd.Flags().String("space-id", "", "目标空间 ID")
	cmd.Flags().String("parent-id", "", "父节点 ID (dentryUuid)")
	addDriveHiddenStringFlag(cmd, "folder", "--parent-id 的兼容别名")
	return cmd
}

func newDriveUploadInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "upload-info",
		Short:             "获取文件上传信息",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileName, err := driveRequiredFlag(cmd, "file-name")
			if err != nil {
				return err
			}
			fileSize, _ := cmd.Flags().GetInt64("file-size")
			if fileSize <= 0 {
				if size, _ := cmd.Flags().GetInt64("size"); size > 0 {
					fileSize = size
				}
			}
			if fileSize <= 0 {
				return apperrors.NewValidation("--file-size is required and must be a positive integer")
			}
			params := map[string]any{
				"fileName": fileName,
				"fileSize": float64(fileSize),
			}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			addDriveStringParam(cmd, params, "mimeType", "mime-type")
			if parentID := driveFlagOrFallback(cmd, "parent-id", "folder"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			return runDriveInvocation(cmd, runner, "drive", "get_upload_info", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-name", "", "文件名 (必填)")
	cmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	cmd.Flags().Int64("size", 0, "--file-size 的兼容别名")
	_ = cmd.Flags().MarkHidden("size")
	cmd.Flags().String("space-id", "", "目标空间 ID")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型")
	cmd.Flags().String("parent-id", "", "父节点 ID (dentryUuid)")
	addDriveHiddenStringFlag(cmd, "folder", "--parent-id 的兼容别名")
	return cmd
}

func newDriveTreeCommand(runner executor.Runner) *cobra.Command {
	tree := &cobra.Command{
		Use:               "tree",
		Short:             "目录树兼容命令",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	preferLegacyLeaf(tree)
	tree.AddCommand(newDriveTreeListCommand(runner))
	return tree
}

func newDriveTreeListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "按路径列目录（兼容别名）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			maxResults, _ := cmd.Flags().GetInt("max")
			if !cmd.Flags().Changed("max") {
				if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
					maxResults = limit
				}
			}
			if maxResults <= 0 {
				maxResults = 20
			}
			params := map[string]any{"maxResults": float64(maxResults)}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			if parentID := driveFlagOrFallback(cmd, "parent-id", "folder"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			addDriveStringParam(cmd, params, "nextToken", "next-token", "cursor")
			return runDriveInvocation(cmd, runner, "drive", "list_files", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("path", "", "目录路径；当前仅作为兼容输入，根路径 / 映射到默认列表")
	cmd.Flags().Int("max", 20, "每页返回数量，默认 20，最大 100")
	cmd.Flags().Int("limit", 0, "--max 的兼容别名")
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("space-id", "", "空间 ID")
	cmd.Flags().String("parent-id", "", "父节点 ID (dentryUuid)")
	addDriveHiddenStringFlag(cmd, "folder", "--parent-id 的兼容别名")
	cmd.Flags().String("next-token", "", "分页游标")
	addDriveHiddenStringFlag(cmd, "cursor", "--next-token 的兼容别名")
	return cmd
}

func newDriveCommitCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "commit",
		Short:             "提交文件上传",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileName, err := driveRequiredFlag(cmd, "file-name")
			if err != nil {
				return err
			}
			uploadID, err := driveRequiredFlag(cmd, "upload-id")
			if err != nil {
				return err
			}
			fileSize, _ := cmd.Flags().GetInt64("file-size")
			if fileSize <= 0 {
				return apperrors.NewValidation("--file-size is required and must be a positive integer")
			}
			params := map[string]any{
				"fileName": fileName,
				"fileSize": float64(fileSize),
				"uploadId": uploadID,
			}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			addDriveStringParam(cmd, params, "conflictHandler", "conflict-handler")
			if parentID := driveFlagOrFallback(cmd, "parent-id", "folder"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			return runDriveInvocation(cmd, runner, "drive", "commit_upload", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-name", "", "文件名 (必填)")
	cmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	cmd.Flags().String("upload-id", "", "上传 ID (必填)")
	cmd.Flags().String("space-id", "", "空间 ID")
	cmd.Flags().String("parent-id", "", "父节点 ID (dentryUuid)")
	addDriveHiddenStringFlag(cmd, "folder", "--parent-id 的兼容别名")
	cmd.Flags().String("conflict-handler", "", "冲突处理策略")
	return cmd
}

// ── upload (three-step composite) ───────────────────────────

func newDriveUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "上传本地文件到钉盘",
		Long: `将本地文件上传到钉盘（三步自动完成）：

  1. drive get_upload_info  → 获取 OSS 上传凭证 (resourceUrl + uploadId + headers)
  2. HTTP PUT 文件二进制   → OSS
  3. drive commit_upload    → 提交文件入库

--folder 指定父目录 dentryUuid，不传则上传到空间根目录。`,
		Example: `  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --folder <dentryUuid>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDriveUpload(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file", "", "本地文件路径 (必填)")
	cmd.Flags().String("file-name", "", "文件显示名称 (默认使用文件名)")
	addDriveHiddenStringFlag(cmd, "name", "--file-name 的兼容别名")
	cmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型，不传则自动推断")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则上传到空间根目录")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	return cmd
}

func runDriveUpload(cmd *cobra.Command, runner executor.Runner) error {
	filePath, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(filePath) == "" {
		return apperrors.NewValidation("--file is required")
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return apperrors.NewValidation("无法解析文件路径: " + err.Error())
	}
	fi, err := os.Stat(absPath)
	if err != nil {
		return apperrors.NewValidation("文件不存在或无法读取: " + absPath)
	}
	if fi.IsDir() {
		return apperrors.NewValidation("--file 不能是目录: " + absPath)
	}
	fileSize := fi.Size()
	if fileSize <= 0 {
		return apperrors.NewValidation("文件为空")
	}

	fileName := driveFlagOrFallback(cmd, "file-name", "name")
	if strings.TrimSpace(fileName) == "" {
		fileName = filepath.Base(absPath)
	}

	spaceID, _ := cmd.Flags().GetString("space-id")
	mimeType, _ := cmd.Flags().GetString("mime-type")
	if strings.TrimSpace(mimeType) == "" {
		mimeType = detectMIME(fileName)
	}
	parentID := driveFlagOrFallback(cmd, "folder", "parent-id")
	if err := validateDriveParentID(parentID); err != nil {
		return err
	}

	// Step 1 params
	step1Params := map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
	}
	if strings.TrimSpace(spaceID) != "" {
		step1Params["spaceId"] = spaceID
	}
	if strings.TrimSpace(mimeType) != "" {
		step1Params["mimeType"] = mimeType
	}
	if strings.TrimSpace(parentID) != "" {
		step1Params["parentId"] = parentID
	}

	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, map[string]any{
			"dry_run": true,
			"step_1_get_upload_info": executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "drive", "get_upload_info", step1Params,
			),
			"step_2_http_put_oss":  "PUT file bytes to resourceUrls[0].url with returned headers",
			"step_3_commit_upload": "drive commit_upload with uploadId from step 1",
			"file":                 absPath,
			"size":                 fileSize,
			"name":                 fileName,
		})
	}

	// Step 1: get_upload_info
	fmt.Fprintf(os.Stderr, "[1/3] 获取上传凭证 %s (%d 字节, %s)...\n", fileName, fileSize, mimeType)
	step1 := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "drive", "get_upload_info", step1Params,
	)
	step1Result, err := runner.Run(cmd.Context(), step1)
	if err != nil {
		return fmt.Errorf("获取上传凭证失败: %w", err)
	}

	resourceURL, uploadID, ossHeaders, err := parseDriveUploadInfo(step1Result.Response)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT to OSS
	fmt.Fprintln(os.Stderr, "[2/3] 上传文件到 OSS...")
	if err := httpPutDriveFile(cmd.Context(), resourceURL, ossHeaders, absPath, fileSize); err != nil {
		return err
	}

	// Step 3: commit_upload
	fmt.Fprintln(os.Stderr, "[3/3] 提交文件入库...")
	step3Params := map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"uploadId": uploadID,
	}
	if strings.TrimSpace(spaceID) != "" {
		step3Params["spaceId"] = spaceID
	}
	if strings.TrimSpace(parentID) != "" {
		step3Params["parentId"] = parentID
	}
	step3 := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "drive", "commit_upload", step3Params,
	)
	result, err := runner.Run(cmd.Context(), step3)
	if err != nil {
		return fmt.Errorf("提交文件入库失败: %w", err)
	}
	return writeCommandPayload(cmd, result)
}

// ── delete (drive surface routed to doc MCP server) ────────

func newDriveDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             "删除文件/文件夹到回收站",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := driveRequiredFlagOrFallback(cmd, "file-id", "node")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, "钉盘节点", fileID) {
				return nil
			}
			params := map[string]any{"nodeId": fileID}
			return runDriveInvocation(cmd, runner, "doc", "delete_document", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-id", "", "文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)")
	addDriveHiddenStringFlag(cmd, "node", "--file-id 的兼容别名")
	cmd.Flags().BoolP("yes", "y", false, "跳过确认直接删除")
	return cmd
}

func runDriveInfo(cmd *cobra.Command, runner executor.Runner, params map[string]any) error {
	result, err := driveInvocationResult(cmd, runner, "drive", "get_file_info", params)
	if err != nil {
		return err
	}
	content := driveResultContent(result)
	driveResult := driveInnerResult(content)
	if !isDriveDingTalkDocResult(driveResult) {
		return writeCommandPayload(cmd, result)
	}

	nodeID := driveStringFromMap(driveResult, "fileId")
	if nodeID == "" {
		nodeID, _ = params["fileId"].(string)
	}
	docResult, err := driveInvocationResult(cmd, runner, "doc", "get_document_info", map[string]any{"nodeId": nodeID})
	if err != nil {
		return writeCommandPayload(cmd, result)
	}
	docContent := driveResultContent(docResult)
	innerDoc := driveInnerResult(docContent)
	if len(innerDoc) == 0 {
		return writeCommandPayload(cmd, result)
	}
	for _, field := range []string{"dentryId", "path", "fileSize", "extension", "type", "fileId"} {
		if value, ok := driveResult[field]; ok {
			if _, exists := innerDoc[field]; !exists {
				innerDoc[field] = value
			}
		}
	}
	if _, hasWrapper := docContent["result"]; hasWrapper {
		docContent["result"] = innerDoc
		return writeCommandPayload(cmd, docContent)
	}
	return writeCommandPayload(cmd, map[string]any{"success": true, "result": innerDoc})
}

func runDriveDownload(cmd *cobra.Command, runner executor.Runner) error {
	fileID, err := driveRequiredFlagOrFallback(cmd, "file-id", "node")
	if err != nil {
		return err
	}
	outputPath, err := driveRequiredFlag(cmd, "output")
	if err != nil {
		return err
	}
	params := map[string]any{"fileId": fileID}
	addDriveStringParam(cmd, params, "spaceId", "space-id")

	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, map[string]any{
			"dry_run": true,
			"step_1_download_file": executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "drive", "download_file", params,
			),
			"step_2_http_get": "GET file bytes from returned downloadUrl/resourceUrl",
			"output":          outputPath,
		})
	}

	result, err := driveInvocationResult(cmd, runner, "drive", "download_file", params)
	if err != nil {
		return err
	}
	resourceURL, serverFilename, headers, err := parseDriveDownloadInfo(driveResultContent(result))
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(outputPath); statErr == nil && info.IsDir() {
		outputPath = filepath.Join(outputPath, driveDownloadFilename(serverFilename, resourceURL))
	}
	if err := httpGetDriveFile(cmd.Context(), resourceURL, headers, outputPath); err != nil {
		return err
	}
	return writeCommandPayload(cmd, map[string]any{
		"success":     true,
		"downloadUrl": resourceURL,
		"output":      outputPath,
	})
}

func runDriveInvocation(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) error {
	result, err := driveInvocationResult(cmd, runner, product, tool, params)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func driveInvocationResult(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) (executor.Result, error) {
	invocation := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), product, tool, params)
	invocation.DryRun = commandDryRun(cmd)
	return runner.Run(cmd.Context(), invocation)
}

func driveResultContent(result executor.Result) map[string]any {
	if content, ok := result.Response["content"].(map[string]any); ok {
		return content
	}
	if len(result.Response) > 0 {
		return result.Response
	}
	return map[string]any{}
}

func driveInnerResult(content map[string]any) map[string]any {
	if content == nil {
		return map[string]any{}
	}
	if result, ok := content["result"].(map[string]any); ok {
		return result
	}
	return content
}

func isDriveDingTalkDocResult(result map[string]any) bool {
	if len(result) == 0 {
		return false
	}
	extension := strings.ToLower(driveStringFromMap(result, "extension"))
	switch extension {
	case "adoc", "axls", "amind", "adraw":
		return true
	}
	return strings.Contains(driveStringFromMap(result, "message"), "钉钉文档")
}

func parseDriveDownloadInfo(content map[string]any) (resourceURL string, filename string, headers map[string]string, err error) {
	data := driveInnerResult(content)
	filename = driveStringFromMap(data, "fileName")
	if filename == "" {
		filename = driveStringFromMap(data, "name")
	}
	switch v := data["resourceUrl"].(type) {
	case string:
		resourceURL = v
	case []any:
		if len(v) > 0 {
			resourceURL, _ = v[0].(string)
		}
	}
	if resourceURL == "" {
		resourceURL, _ = data["downloadUrl"].(string)
	}
	if resourceURL == "" {
		err = apperrors.NewValidation("download_file 返回不完整: resourceUrl/downloadUrl 为空")
		return
	}

	headers = make(map[string]string)
	if h, ok := data["headers"].(map[string]any); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	return
}

func driveDownloadFilename(serverFilename, rawURL string) string {
	if name := cleanDriveFilename(serverFilename); name != "" {
		return name
	}
	return inferDriveFilename(rawURL)
}

func cleanDriveFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

func inferDriveFilename(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		name := strings.TrimSpace(filepath.Base(parsed.Path))
		if name != "" && name != "." && name != "/" {
			if decoded, decodeErr := url.PathUnescape(name); decodeErr == nil && decoded != "" {
				return decoded
			}
			return name
		}
	}
	return "download"
}

func httpGetDriveFile(ctx context.Context, resourceURL string, headers map[string]string, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, nil)
	if err != nil {
		return fmt.Errorf("构建下载请求失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("下载失败 HTTP %d: %s", resp.StatusCode, string(body))
	}
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	return nil
}

func driveStringFlag(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	if value, err := cmd.Flags().GetString(name); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, err := cmd.InheritedFlags().GetString(name); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func driveFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	if value := driveStringFlag(cmd, primary); value != "" {
		return value
	}
	for _, alias := range aliases {
		if value := driveStringFlag(cmd, alias); value != "" {
			return value
		}
	}
	return ""
}

func driveIntFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) int {
	if cmd == nil {
		return 0
	}
	if cmd.Flags().Changed(primary) {
		value, _ := cmd.Flags().GetInt(primary)
		return value
	}
	for _, alias := range aliases {
		if cmd.Flags().Changed(alias) {
			value, _ := cmd.Flags().GetInt(alias)
			return value
		}
	}
	value, _ := cmd.Flags().GetInt(primary)
	return value
}

func driveRequiredFlag(cmd *cobra.Command, name string) (string, error) {
	if value := driveStringFlag(cmd, name); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation(fmt.Sprintf("--%s is required", name))
}

func driveRequiredFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) (string, error) {
	if value := driveFlagOrFallback(cmd, primary, aliases...); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation(fmt.Sprintf("--%s is required", primary))
}

func addDriveStringParam(cmd *cobra.Command, params map[string]any, paramName string, flags ...string) {
	if value := driveFlagOrFallback(cmd, flags[0], flags[1:]...); value != "" {
		params[paramName] = value
	}
}

func addDriveHiddenStringFlag(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", usage)
	_ = cmd.Flags().MarkHidden(name)
}

func driveStringFromMap(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

// validateDriveParentID rejects pure-numeric IDs (which are dentryId values
// from the chat link namespace, not drive's dentryUuid).
func validateDriveParentID(parentID string) error {
	value := strings.TrimSpace(parentID)
	if value == "" {
		return nil
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return nil
		}
	}
	return apperrors.NewValidation(fmt.Sprintf(
		"invalid drive --folder %q: pure numeric IDs are usually dentryId values from chat links, not drive dentryUuid; use a parent folder dentryUuid from drive list, or omit --folder to use the space root",
		parentID,
	))
}

// parseDriveUploadInfo extracts resourceUrl / uploadId / OSS headers from the
// drive.get_upload_info response. The actual server payload is:
//
//	{
//	  "uploadId": "...",
//	  "resourceUrls": [
//	    { "url": "https://...", "headers": { ... } }
//	  ]
//	}
//
// MCP gateway may wrap the payload with a "content" or "result" envelope, so we
// peel one layer if present, and also accept legacy flat resourceUrl/uploadUrl
// fields as a fallback.
func parseDriveUploadInfo(resp map[string]any) (resourceURL, uploadID string, headers map[string]string, err error) {
	if resp == nil {
		err = apperrors.NewValidation("get_upload_info 返回为空")
		return
	}
	data := resp
	if content, ok := data["content"].(map[string]any); ok && len(content) > 0 {
		data = content
	}
	if result, ok := data["result"].(map[string]any); ok && len(result) > 0 {
		data = result
	}

	uploadID, _ = data["uploadId"].(string)

	if urls, ok := data["resourceUrls"].([]any); ok && len(urls) > 0 {
		if first, ok := urls[0].(map[string]any); ok {
			resourceURL, _ = first["url"].(string)
			headers = make(map[string]string)
			if h, ok := first["headers"].(map[string]any); ok {
				for k, v := range h {
					if s, ok := v.(string); ok {
						headers[k] = s
					}
				}
			}
		}
	}
	if resourceURL == "" {
		resourceURL, _ = data["resourceUrl"].(string)
	}
	if resourceURL == "" {
		resourceURL, _ = data["uploadUrl"].(string)
	}

	if resourceURL == "" || uploadID == "" {
		err = apperrors.NewValidation(fmt.Sprintf(
			"get_upload_info 返回不完整: resourceUrl=%q, uploadId=%q", resourceURL, uploadID,
		))
		return
	}

	if headers == nil {
		headers = make(map[string]string)
		if h, ok := data["headers"].(map[string]any); ok {
			for k, v := range h {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
		}
	}
	return
}

// httpPutDriveFile uploads the file at filePath to a DingTalk drive OSS presigned URL.
//
// PROTOCOL CONTRACT: the headers map is authoritative. It contains the exact
// and complete set of HTTP headers required for the upload. An empty map means
// "no client-side headers needed" (this is the normal case for DingTalk drive,
// where the signature is embedded in the URL query string).
//
// DO NOT add Content-Type or any other client-inferred header here. DingTalk
// drive uses OSS v1 presigned URLs whose StringToSign includes the Content-Type
// header that the server saw at signing time (typically empty). Any client-side
// addition of Content-Type makes the signature computed by OSS at PUT time
// differ from the server's presignature → 403 SignatureDoesNotMatch.
//
// If a future OSS endpoint requires header-based signing (Authorization header
// instead of URL signing), introduce a separate helper rather than reintroducing
// a fallback here. The aitable attachment upload helper in aitable.go does set
// Content-Type because its OSS endpoint uses a different signing mode where the
// server includes the client-declared mime in its signature computation; do not
// unify the two helpers without re-validating both endpoints.
func httpPutDriveFile(ctx context.Context, resourceURL string, headers map[string]string, filePath string, fileSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("无法打开文件: %w", err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, resourceURL, f)
	if err != nil {
		return fmt.Errorf("构建 OSS 上传请求失败: %w", err)
	}
	req.ContentLength = fileSize
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("OSS 上传失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("OSS 上传失败 HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
