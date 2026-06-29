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
		Short:             "钉盘文件管理",
		Long:              `钉盘：列出文件/文件夹、获取元数据、下载链接、创建文件夹、获取上传信息、提交上传。`,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	preferLegacyLeaf(root)
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
		newDriveSearchCommand(runner),
		newDriveCopyCommand(runner),
		newDriveMoveCommand(runner),
		newDriveRenameCommand(runner),
		newDrivePermissionCommand(runner),
		newDriveFolderCommand(runner),
	)
	return root
}

// ── dynamic-compatible leaves ──────────────────────────────

func newDriveListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "获取文件/文件夹列表",
		Example: `  dws drive list --limit 20
  dws drive list --limit 20 --folder <dentryUuid> --order-by name --order asc`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			maxResults := driveIntFlagOrFallback(cmd, "limit", "max", "page-size")
			if maxResults <= 0 {
				maxResults = 20
			}
			if workspaceID := driveFlagOrFallback(cmd, "workspace", "workspace-id"); workspaceID != "" {
				params := map[string]any{"workspaceId": workspaceID}
				if folderID := driveFlagOrFallback(cmd, "folder", "parent-id"); folderID != "" {
					params["folderId"] = normalizeDocNodeID(folderID)
				}
				if maxResults > 0 {
					params["pageSize"] = maxResults
				}
				addDriveStringParam(cmd, params, "pageToken", "cursor", "next-token")
				return runDriveInvocation(cmd, runner, "doc", "list_nodes", params)
			}
			params := map[string]any{"maxResults": float64(maxResults)}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			if parentID := driveFlagOrFallback(cmd, "folder", "parent-id"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			addDriveStringParam(cmd, params, "nextToken", "cursor", "next-token")
			addDriveStringParam(cmd, params, "orderBy", "order-by")
			addDriveStringParam(cmd, params, "order", "order")
			if thumbnail, _ := cmd.Flags().GetBool("thumbnail"); thumbnail {
				params["withThumbnail"] = true
			}
			return runDriveInvocation(cmd, runner, "drive", "list_files", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Int("limit", 20, "每页返回数量，默认 20，最大 100 (可选)")
	cmd.Flags().Int("max", 0, "--limit 的别名（向后兼容）")
	_ = cmd.Flags().MarkHidden("max")
	cmd.Flags().Int("page-size", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("space-id", "", "空间 ID，不传则使用「我的文件」对应 spaceId (可选)")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	cmd.Flags().String("workspace", "", "文档空间/知识库 ID，传入则路由到文档空间")
	addDriveHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	cmd.Flags().String("cursor", "", "分页游标，首次不传 (可选)")
	addDriveHiddenStringFlag(cmd, "next-token", "--cursor 的兼容别名")
	cmd.Flags().String("order-by", "", "排序字段: createTime|modifyTime|name (可选)")
	cmd.Flags().String("order", "", "排序方向: asc|desc，默认 desc (可选)")
	cmd.Flags().Bool("thumbnail", false, "是否返回缩略图信息 (可选)")
	return cmd
}

func newDriveListSpacesCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-spaces",
		Short: "获取钉盘空间列表",
		Long: `列出当前用户可访问的钉盘空间，返回 spaceId、spaceName、rootFolderId 等信息。

spaceType 筛选规则:
  orgSpace（默认）: 返回企业空间列表，支持 nextToken 分页
  mySpace: 返回用户的"我的文件"个人空间（单个）`,
		Example: `  dws drive list-spaces
  dws drive list-spaces --space-type mySpace
  dws drive list-spaces --space-type orgSpace --limit 20 --cursor <TOKEN>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: deprecated: use dws wiki space list instead.")
			params := map[string]any{}
			maxResults, _ := cmd.Flags().GetInt("limit")
			if !cmd.Flags().Changed("limit") {
				if limit, _ := cmd.Flags().GetInt("max"); limit > 0 {
					maxResults = limit
				}
			}
			if maxResults > 0 {
				params["maxResults"] = float64(maxResults)
			}
			addDriveStringParam(cmd, params, "spaceType", "space-type")
			addDriveStringParam(cmd, params, "nextToken", "cursor", "next-token")
			return runDriveInvocation(cmd, runner, "drive", "list_spaces", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Int("limit", 20, "每页返回数量 (默认 20，最大 50)，仅 spaceType 为 orgSpace 时有效")
	cmd.Flags().Int("max", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("max")
	cmd.Flags().String("space-type", "", "空间类型: orgSpace=企业空间(默认), mySpace=我的文件 (可选)")
	cmd.Flags().String("cursor", "", "分页游标，仅企业空间支持分页 (可选)")
	addDriveHiddenStringFlag(cmd, "next-token", "--cursor 的兼容别名")
	return cmd
}

func newDriveInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "获取文件元数据信息",
		Long: `获取钉盘文件/文件夹的元数据信息。

如果目标文件属于钉钉文档（在线文档/表格/脑图等），会自动跟进调用
钉钉文档接口获取更准确的文档信息（如真实文档名称），并合并输出。`,
		Example:           `  dws drive info --node <dentryUuid>  # 查询 fileId: dws drive list`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := driveRequiredFlagOrFallback(cmd, "node", "file-id")
			if err != nil {
				return err
			}
			params := map[string]any{"fileId": fileID}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			return runDriveInfo(cmd, runner, params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", "节点 ID (dentryUuid) (必填)")
	addDriveHiddenStringFlag(cmd, "file-id", "--node 的兼容别名")
	cmd.Flags().String("space-id", "", "节点所属空间 ID (可选)")
	return cmd
}

func newDriveDownloadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: "下载钉盘文件到本地",
		Long: `下载钉盘中的文件到本地（两步下载流程）。

流程:
  1. 获取下载 URL 和签名请求头 (download_file)
  2. HTTP GET 下载文件二进制内容到本地

--output 指定本地保存路径，可以是文件路径或目录。
如果指定目录，文件名从下载 URL 中自动推断。`,
		Example: `  dws drive download --node <dentryUuid> --output ./report.pdf
  dws drive download --node <dentryUuid> --output ~/downloads/`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDriveDownload(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("node", "", "文件 ID (dentryUuid) (必填)")
	addDriveHiddenStringFlag(cmd, "file-id", "--node 的兼容别名")
	cmd.Flags().String("space-id", "", "文件所属空间 ID (可选)")
	cmd.Flags().String("output", "", "本地保存路径 (文件路径或目录，必填)")
	return cmd
}

func newDriveMkdirCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mkdir",
		Short: "创建文件夹",
		Example: `  dws drive mkdir --name "项目资料"
  dws drive mkdir --name "子目录" --folder <dentryUuid>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := driveRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			if parentID := driveFlagOrFallback(cmd, "folder", "parent-id"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			return runDriveInvocation(cmd, runner, "drive", "create_folder", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", "文件夹名称，最长 50 字符 (必填)")
	cmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」 (可选)")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则在空间根目录下创建 (可选)")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	return cmd
}

func newDriveUploadInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload-info",
		Short: "获取文件上传信息",
		Example: `  dws drive upload-info --file-name "报告.pdf" --file-size 102400
  dws drive upload-info --file-name "readme.txt" --file-size 1024 --folder <dentryUuid>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileName, err := driveRequiredFlag(cmd, "file-name")
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
			}
			addDriveStringParam(cmd, params, "spaceId", "space-id")
			addDriveStringParam(cmd, params, "mimeType", "mime-type")
			if parentID := driveFlagOrFallback(cmd, "folder", "parent-id"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			return runDriveInvocation(cmd, runner, "drive", "get_upload_info", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-name", "", "文件名，须包含扩展名，如 报告.pdf (必填)")
	cmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	_ = cmd.MarkFlagRequired("file-size")
	cmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」 (可选)")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型，如 application/pdf，不传则自动推断 (可选)")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	return cmd
}

func newDriveCommitCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "commit",
		Short:             "提交文件上传",
		Example:           `  dws drive commit --file-name "报告.pdf" --file-size 102400 --upload-id <uploadId>`,
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
			if parentID := driveFlagOrFallback(cmd, "folder", "parent-id"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				params["parentId"] = parentID
			}
			return runDriveInvocation(cmd, runner, "drive", "commit_upload", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file-name", "", "文件名（含扩展名），须与 get_upload_info 时一致 (必填)")
	cmd.Flags().Int64("file-size", 0, "文件大小（字节），须与 get_upload_info 时一致 (必填)")
	_ = cmd.MarkFlagRequired("file-size")
	cmd.Flags().String("upload-id", "", "上传 ID，来自 get_upload_info 返回的 uploadId (必填)")
	cmd.Flags().String("space-id", "", "空间 ID，不传则使用「我的文件」 (可选)")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则提交到根目录 (可选)")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	return cmd
}

// ── upload (three-step composite) ───────────────────────────

func newDriveUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "上传本地文件到钉盘",
		Long: `将本地文件上传到钉盘（三步自动完成）。

流程:
  1. 获取 OSS 上传凭证 (get_upload_info)
  2. HTTP PUT 上传文件二进制到 OSS
  3. 提交文件入库 (commit_upload)

上传位置: --folder 指定父目录，不传则上传到空间根目录。`,
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
	cmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」 (可选)")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型，不传则自动推断 (可选)")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	cmd.Flags().String("workspace", "", "目标知识库 ID，传入时路由到文档空间上传")
	addDriveHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	cmd.Flags().Bool("convert", false, "是否转换为钉钉在线文档（文档空间上传时生效）")
	return cmd
}

func runDriveUpload(cmd *cobra.Command, runner executor.Runner) error {
	filePath, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(filePath) == "" {
		return apperrors.NewValidation("--file is required")
	}
	if workspaceID := driveFlagOrFallback(cmd, "workspace", "workspace-id"); workspaceID != "" {
		return runDriveUploadToDoc(cmd, runner, workspaceID)
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

func runDriveUploadToDoc(cmd *cobra.Command, runner executor.Runner, workspaceID string) error {
	filePath, _ := cmd.Flags().GetString("file")
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
	if fileName == "" {
		fileName = filepath.Base(absPath)
	}
	folderID := driveFlagOrFallback(cmd, "folder", "parent-id")
	if folderID != "" {
		folderID = normalizeDocNodeID(folderID)
	}

	step1Params := map[string]any{"workspaceId": workspaceID}
	if folderID != "" {
		step1Params["folderId"] = folderID
	}
	commitParams := map[string]any{
		"name":        fileName,
		"fileSize":    float64(fileSize),
		"workspaceId": workspaceID,
	}
	if folderID != "" {
		commitParams["folderId"] = folderID
	}
	if convert, _ := cmd.Flags().GetBool("convert"); convert {
		commitParams["convertToOnlineDoc"] = true
	}

	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, map[string]any{
			"dry_run": true,
			"step_1_get_file_upload_info": executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "get_file_upload_info", step1Params,
			),
			"step_2_http_put_oss": "PUT file bytes to resourceUrl with returned headers",
			"step_3_commit_uploaded_file": executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "commit_uploaded_file", commitParams,
			),
			"file": absPath,
			"name": fileName,
			"size": fileSize,
		})
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "[1/3] 获取文档空间上传凭证 %s (%d 字节)...\n", fileName, fileSize)
	step1Result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "get_file_upload_info", step1Params,
	))
	if err != nil {
		return fmt.Errorf("获取上传凭证失败: %w", err)
	}
	resourceURL, uploadKey, headers, err := extractDocFileUploadInfo(step1Result.Response)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "[2/3] 上传文件到 OSS...")
	if err := httpPutDriveFile(cmd.Context(), resourceURL, headers, absPath, fileSize); err != nil {
		return err
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "[3/3] 提交文件入库...")
	commitParams["uploadKey"] = uploadKey
	result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "commit_uploaded_file", commitParams,
	))
	if err != nil {
		return fmt.Errorf("提交文件入库失败: %w", err)
	}
	return writeCommandPayload(cmd, result)
}

// ── delete (drive surface routed to doc MCP server) ────────

func newDriveDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "删除文件/文件夹到回收站",
		Long: `将钉盘中的文件或文件夹移入回收站。

注意: 这是一个危险操作，文件将被移入回收站。执行前需要确认，或传入 --yes 跳过确认。
--node 对应 drive list 返回的 fileId 字段（即 dentryUuid）。

权限要求: 对文档有"管理"权限。`,
		Example: `  dws drive delete --node <dentryUuid> --yes    # 查询 fileId: dws drive list
  dws drive delete --node <dentryUuid>           # 交互式确认后删除`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := driveRequiredFlagOrFallback(cmd, "node", "file-id")
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
	cmd.Flags().String("node", "", "文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)")
	addDriveHiddenStringFlag(cmd, "file-id", "--node 的兼容别名")
	cmd.Flags().BoolP("yes", "y", false, "跳过确认直接删除")
	return cmd
}

func newDriveSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             "搜索文件（聚合钉盘和文档空间）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := driveFlagOrFallback(cmd, "query", "keyword")
			if keyword == "" {
				return apperrors.NewValidation("--query is required")
			}
			target := driveStringFlag(cmd, "target")
			driveParams := map[string]any{"keyword": keyword}
			if target != "" && target != "all" {
				driveParams["searchTarget"] = target
			}
			addDriveStringSliceParam(cmd, driveParams, "fileTypes", "file-types")
			addDriveStringSliceParam(cmd, driveParams, "extensions", "extensions")
			addDriveStringSliceParam(cmd, driveParams, "creatorUserIds", "creator-uids")
			addDriveInt64Param(cmd, driveParams, "createdTimeFrom", "created-from")
			addDriveInt64Param(cmd, driveParams, "createdTimeTo", "created-to")
			addDriveInt64Param(cmd, driveParams, "modifiedTimeFrom", "modified-from")
			addDriveInt64Param(cmd, driveParams, "modifiedTimeTo", "modified-to")
			if pageSize := driveIntFlagOrFallback(cmd, "limit", "page-size"); pageSize > 0 {
				driveParams["pageSize"] = float64(pageSize)
			}
			addDriveStringParam(cmd, driveParams, "pageToken", "cursor", "page-token")

			if target == "file" || target == "space" {
				return runDriveInvocation(cmd, runner, "drive", "search_files", driveParams)
			}

			driveResult, driveErr := driveInvocationResult(cmd, runner, "drive", "search_files", driveParams)
			docParams := map[string]any{"keyword": keyword}
			if pageSize := driveIntFlagOrFallback(cmd, "limit", "page-size"); pageSize > 0 {
				docParams["pageSize"] = pageSize
			}
			if extensions, ok := driveParams["extensions"]; ok {
				docParams["extensions"] = extensions
			}
			docResult, docErr := driveInvocationResult(cmd, runner, "doc", "search_documents", docParams)
			if driveErr != nil && docErr != nil {
				return fmt.Errorf("aggregated search failed: drive: %v; doc: %v", driveErr, docErr)
			}
			result := map[string]any{}
			if driveErr == nil {
				result["drive_results"] = driveResult.Response
			}
			if docErr == nil {
				result["doc_results"] = docResult.Response
			}
			return writeCommandPayload(cmd, map[string]any{"success": true, "result": result})
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	addDriveHiddenStringFlag(cmd, "keyword", "--query 的兼容别名")
	cmd.Flags().String("target", "", "搜索范围: all(默认) / file / space")
	cmd.Flags().StringSlice("file-types", nil, "按文件内容类型过滤")
	cmd.Flags().StringSlice("extensions", nil, "按文件扩展名过滤")
	cmd.Flags().StringSlice("creator-uids", nil, "按创建者 userId 过滤")
	cmd.Flags().Int64("created-from", 0, "创建时间起始毫秒时间戳")
	cmd.Flags().Int64("created-to", 0, "创建时间截止毫秒时间戳")
	cmd.Flags().Int64("modified-from", 0, "修改时间起始毫秒时间戳")
	cmd.Flags().Int64("modified-to", 0, "修改时间截止毫秒时间戳")
	cmd.Flags().Int("limit", 0, "每页返回数量")
	cmd.Flags().Int("page-size", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("cursor", "", "分页游标")
	addDriveHiddenStringFlag(cmd, "page-token", "--cursor 的兼容别名")
	return cmd
}

func newDriveCopyCommand(runner executor.Runner) *cobra.Command {
	return newDriveDocTransferCommand(runner, "copy", "copy_document")
}

func newDriveMoveCommand(runner executor.Runner) *cobra.Command {
	return newDriveDocTransferCommand(runner, "move", "move_document")
}

func newDriveDocTransferCommand(runner executor.Runner, use, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             use + " 文档空间节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := driveRequiredFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": normalizeDocNodeID(nodeID)}
			if folder := driveFlagOrFallback(cmd, "folder", "parent-id", "parent-node-id", "parent-folder-id"); folder != "" {
				params["targetFolderId"] = normalizeDocNodeID(folder)
			}
			addDriveStringParam(cmd, params, "workspaceId", "workspace", "workspace-id")
			return runDriveInvocation(cmd, runner, "doc", tool, params)
		},
	}
	preferLegacyLeaf(cmd)
	addDriveDocNodeFlags(cmd)
	cmd.Flags().String("folder", "", "目标文件夹 nodeId")
	addDriveHiddenStringFlag(cmd, "parent-id", "--folder 的兼容别名")
	addDriveHiddenStringFlag(cmd, "parent-node-id", "--folder 的兼容别名")
	addDriveHiddenStringFlag(cmd, "parent-folder-id", "--folder 的兼容别名")
	cmd.Flags().String("workspace", "", "目标知识库 ID")
	addDriveHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	return cmd
}

func newDriveRenameCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rename",
		Short:             "重命名文档空间节点",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := driveRequiredFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			name, err := driveRequiredFlagOrFallback(cmd, "name", "title")
			if err != nil {
				return err
			}
			return runDriveInvocation(cmd, runner, "doc", "rename_document", map[string]any{
				"nodeId":  normalizeDocNodeID(nodeID),
				"newName": name,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addDriveDocNodeFlags(cmd)
	cmd.Flags().String("name", "", "新名称 (必填)")
	addDriveHiddenStringFlag(cmd, "title", "--name 的兼容别名")
	return cmd
}

func newDrivePermissionCommand(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "permission",
		Aliases:           []string{"perm"},
		Short:             "文档空间节点权限管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(
		newDrivePermissionMutationCommand(runner, "add", "add_permission", true),
		newDrivePermissionMutationCommand(runner, "update", "update_permission", true),
		newDrivePermissionListCommand(runner),
		newDrivePermissionRemoveCommand(runner),
	)
	preferLegacyLeaf(root)
	return root
}

func newDrivePermissionMutationCommand(runner executor.Runner, use, tool string, requireRole bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             use + " 文档空间节点权限",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := driveRequiredFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			rawUsers := driveFlagOrFallback(cmd, "users", "user", "uid")
			if rawUsers == "" {
				return apperrors.NewValidation("--users is required")
			}
			userIDs, err := parseDocPermissionUsers(rawUsers)
			if err != nil {
				return err
			}
			params := map[string]any{
				"nodeId":  normalizeDocNodeID(nodeID),
				"userIds": userIDs,
			}
			if requireRole {
				rawRole, err := driveRequiredFlag(cmd, "role")
				if err != nil {
					return err
				}
				role, ok := normalizeDocPermissionRole(rawRole)
				if !ok {
					return apperrors.NewValidation(fmt.Sprintf("invalid --role: %s", rawRole))
				}
				params["roleId"] = role
			}
			addDriveStringParam(cmd, params, "workspaceId", "workspace", "workspace-id")
			return runDriveInvocation(cmd, runner, "doc", tool, params)
		},
	}
	preferLegacyLeaf(cmd)
	addDriveDocNodeFlags(cmd)
	cmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	addDriveHiddenStringFlag(cmd, "user", "--users 的兼容别名")
	addDriveHiddenStringFlag(cmd, "uid", "--users 的兼容别名")
	if requireRole {
		cmd.Flags().String("role", "", "权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填)")
	}
	cmd.Flags().String("workspace", "", "知识库 ID (选填)")
	addDriveHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	return cmd
}

func newDrivePermissionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Aliases:           []string{"ls"},
		Short:             "查询文档空间节点协作者",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := driveRequiredFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": normalizeDocNodeID(nodeID)}
			if limit := driveIntFlagOrFallback(cmd, "limit", "max-results", "page-size"); limit > 0 {
				params["maxResults"] = limit
			}
			if filterRole := driveStringFlag(cmd, "filter-role"); filterRole != "" {
				params["filterRoleIds"] = parseDriveRoleList(filterRole)
			}
			addDriveStringParam(cmd, params, "workspaceId", "workspace", "workspace-id")
			return runDriveInvocation(cmd, runner, "doc", "list_permission", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDriveDocNodeFlags(cmd)
	cmd.Flags().Int("limit", 30, "返回成员数上限")
	cmd.Flags().Int("max-results", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("max-results")
	cmd.Flags().Int("page-size", 0, "--limit 的兼容别名")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("filter-role", "", "按角色过滤，逗号分隔")
	cmd.Flags().String("workspace", "", "知识库 ID (选填)")
	addDriveHiddenStringFlag(cmd, "workspace-id", "--workspace 的兼容别名")
	return cmd
}

func newDrivePermissionRemoveCommand(runner executor.Runner) *cobra.Command {
	cmd := newDrivePermissionMutationCommand(runner, "remove", "remove_permission", false)
	cmd.Aliases = []string{"rm"}
	return cmd
}

func newDriveFolderCommand(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "folder",
		Short:             "文档空间文件夹兼容入口（deprecated）",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	create := &cobra.Command{
		Use:               "create",
		Short:             "创建文档空间文件夹（deprecated）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: deprecated: use dws wiki node create --type folder instead.")
			name, err := driveRequiredFlagOrFallback(cmd, "name", "title")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name}
			if folder := driveFlagOrFallback(cmd, "folder", "parent-id"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			addDriveStringParam(cmd, params, "workspaceId", "workspace", "workspace-id")
			return runDriveInvocation(cmd, runner, "doc", "create_folder", params)
		},
	}
	preferLegacyLeaf(root)
	preferLegacyLeaf(create)
	create.Flags().String("name", "", "文件夹名称 (必填)")
	addDriveHiddenStringFlag(create, "title", "--name 的兼容别名")
	create.Flags().String("folder", "", "父文件夹 nodeId 或 URL")
	addDriveHiddenStringFlag(create, "parent-id", "--folder 的兼容别名")
	create.Flags().String("workspace", "", "目标知识库 ID")
	addDriveHiddenStringFlag(create, "workspace-id", "--workspace 的兼容别名")
	root.AddCommand(create)
	root.Hidden = true
	return root
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
	fileID, err := driveRequiredFlagOrFallback(cmd, "node", "file-id")
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

func addDriveStringSliceParam(cmd *cobra.Command, params map[string]any, paramName, flag string) {
	if !cmd.Flags().Changed(flag) {
		return
	}
	values, err := cmd.Flags().GetStringSlice(flag)
	if err != nil || len(values) == 0 {
		return
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if item := strings.TrimSpace(part); item != "" {
				cleaned = append(cleaned, item)
			}
		}
	}
	if len(cleaned) > 0 {
		params[paramName] = cleaned
	}
}

func addDriveInt64Param(cmd *cobra.Command, params map[string]any, paramName, flag string) {
	if !cmd.Flags().Changed(flag) {
		return
	}
	value, err := cmd.Flags().GetInt64(flag)
	if err == nil && value > 0 {
		params[paramName] = value
	}
}

func addDriveDocNodeFlags(cmd *cobra.Command) {
	cmd.Flags().String("node", "", "节点 ID 或 URL (必填)")
	addDriveHiddenStringFlag(cmd, "url", "--node 的兼容别名")
	addDriveHiddenStringFlag(cmd, "id", "--node 的兼容别名")
	addDriveHiddenStringFlag(cmd, "node-id", "--node 的兼容别名")
	addDriveHiddenStringFlag(cmd, "doc-id", "--node 的兼容别名")
	addDriveHiddenStringFlag(cmd, "file-id", "--node 的兼容别名")
}

func parseDriveRoleList(raw string) []string {
	roles := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		role := strings.ToUpper(strings.TrimSpace(part))
		if role == "" {
			continue
		}
		if normalized, ok := normalizeDocPermissionRole(role); ok {
			roles = append(roles, normalized)
			continue
		}
		if role == "OWNER" {
			roles = append(roles, role)
		}
	}
	return roles
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
