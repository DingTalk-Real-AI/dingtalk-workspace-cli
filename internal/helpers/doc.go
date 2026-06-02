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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return docHandler{}
	})
}

type docHandler struct{}

func (docHandler) Name() string {
	return "doc"
}

func (docHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "doc",
		Short:             i18n.T("钉钉文档操作"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	media := &cobra.Command{
		Use:               "media",
		Short:             i18n.T("文档媒体 / 附件管理"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	media.AddCommand(newDocMediaDownloadCommand(runner))
	media.AddCommand(newDocMediaInsertCommand(runner))

	permission := &cobra.Command{
		Use:               "permission",
		Short:             i18n.T("文档权限协作管理（节点级）"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	permission.AddCommand(
		newDocPermissionAddCommand(runner),
		newDocPermissionUpdateCommand(runner),
		newDocPermissionListCommand(runner),
	)

	export := &cobra.Command{
		Use:               "export",
		Short:             i18n.T("文档导出（异步任务）"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocExport(cmd, runner)
		},
	}
	export.AddCommand(newDocExportGetCommand(runner))

	file := newDocGroup("file", i18n.T("文档文件管理"))
	file.AddCommand(newDocFileCreateCommand(runner))

	folder := newDocGroup("folder", i18n.T("文档文件夹管理"))
	folder.AddCommand(newDocFolderCreateCommand(runner))

	block := newDocGroup("block", i18n.T("文档块管理"))
	block.AddCommand(
		newDocBlockListCommand(runner),
		newDocBlockInsertCommand(runner),
		newDocBlockUpdateCommand(runner),
		newDocBlockDeleteCommand(runner),
	)

	comment := newDocGroup("comment", i18n.T("文档评论管理"))
	comment.AddCommand(
		newDocCommentListCommand(runner),
		newDocCommentCreateCommand(runner),
		newDocCommentReplyCommand(runner),
		newDocCommentCreateInlineCommand(runner),
	)

	root.AddCommand(file)
	root.AddCommand(folder)
	root.AddCommand(block)
	root.AddCommand(comment)
	root.AddCommand(media)
	root.AddCommand(permission)
	root.AddCommand(export)
	root.AddCommand(newDocSearchCommand(runner))
	root.AddCommand(newDocListCommand(runner))
	root.AddCommand(newDocInfoCommand(runner))
	root.AddCommand(newDocReadCommand(runner))
	root.AddCommand(newDocCreateCommand(runner))
	root.AddCommand(newDocUpdateCommand(runner))
	root.AddCommand(newDocUploadCommand(runner))
	root.AddCommand(newDocDownloadCommand(runner))
	root.AddCommand(newDocCopyCommand(runner))
	root.AddCommand(newDocMoveCommand(runner))
	root.AddCommand(newDocRenameCommand(runner))
	root.AddCommand(newDocDeleteCommand(runner))
	preferLegacyLeaf(export) // export 同时是一体化命令本身（含 RunE）
	doRegisterDocExportFlags(export)
	return root
}

func newDocGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

// doRegisterDocExportFlags 把 export 一体化命令的 flag 注册分离出来，
// 便于和 export get 子命令的 flag 分离管理。
func doRegisterDocExportFlags(cmd *cobra.Command) {
	addDocNodeFlags(cmd)
	cmd.Flags().String("output", "", i18n.T("本地落盘路径（必填；传目录时自动生成 docx 文件名）"))
	cmd.Flags().String("export-format", "", i18n.T("导出格式，当前仅支持 docx（悟空兼容）"))
	cmd.Flags().Int("timeout-sec", 300, i18n.T("整体轮询超时（秒），默认 300"))
}

func newDocSearchCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "search",
		Short:             i18n.T("搜索文档"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if keyword := docFlagOrFallback(cmd, "query", "keyword"); keyword != "" {
				params["keyword"] = keyword
			}
			addDocCSVParam(cmd, params, "extensions", "extensions")
			addDocIntParam(cmd, params, "createdTimeFrom", "created-from")
			addDocIntParam(cmd, params, "createdTimeTo", "created-to")
			addDocIntParam(cmd, params, "visitedTimeFrom", "visited-from")
			addDocIntParam(cmd, params, "visitedTimeTo", "visited-to")
			addDocCSVParam(cmd, params, "creatorUserIds", "creator-uids")
			addDocCSVParam(cmd, params, "editorUserIds", "editor-uids")
			addDocCSVParam(cmd, params, "mentionedUserIds", "mentioned-uids")
			addDocCSVParam(cmd, params, "workspaceIds", "workspace-ids", "workspace")
			if pageSize := docIntFlagOrFallback(cmd, "page-size", "limit"); pageSize > 0 {
				params["pageSize"] = pageSize
			}
			addDocStringParam(cmd, params, "pageToken", "page-token", "cursor")
			return runDocTool(cmd, runner, "doc", "search_documents", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("query", "", i18n.T("搜索关键词"))
	addDocHiddenStringFlag(cmd, "keyword", "--query alias")
	cmd.Flags().String("extensions", "", i18n.T("扩展名，逗号分隔"))
	cmd.Flags().Int64("created-from", 0, i18n.T("创建时间起点"))
	cmd.Flags().Int64("created-to", 0, i18n.T("创建时间终点"))
	cmd.Flags().Int64("visited-from", 0, i18n.T("访问时间起点"))
	cmd.Flags().Int64("visited-to", 0, i18n.T("访问时间终点"))
	cmd.Flags().String("creator-uids", "", i18n.T("创建人 userId，逗号分隔"))
	cmd.Flags().String("editor-uids", "", i18n.T("编辑人 userId，逗号分隔"))
	cmd.Flags().String("mentioned-uids", "", i18n.T("提及 userId，逗号分隔"))
	cmd.Flags().String("workspace-ids", "", i18n.T("Workspace ID，逗号分隔"))
	addDocHiddenStringFlag(cmd, "workspace", "--workspace-ids alias")
	cmd.Flags().Int("page-size", 0, i18n.T("每页数量"))
	cmd.Flags().Int("limit", 0, i18n.T("--page-size 的悟空兼容别名"))
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("page-token", "", i18n.T("分页 token"))
	addDocHiddenStringFlag(cmd, "cursor", "--page-token 的悟空兼容别名")
	return cmd
}

func newDocListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("列出文档节点"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if folder := docFlagOrFallback(cmd, "folder", "parent-id", "node", "file-id", "nodee"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			if workspace := docFlagOrFallback(cmd, "workspace", "workspace-id"); workspace != "" {
				params["workspaceId"] = workspace
			}
			if pageSize := docIntFlagOrFallback(cmd, "page-size", "limit"); pageSize > 0 {
				params["pageSize"] = pageSize
			}
			addDocStringParam(cmd, params, "pageToken", "page-token", "cursor")
			return runDocTool(cmd, runner, "doc", "list_nodes", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("folder", "", i18n.T("父文件夹 nodeId / URL"))
	addDocHiddenStringFlag(cmd, "parent-id", "--folder alias")
	addDocHiddenStringFlag(cmd, "node", "--folder alias")
	addDocHiddenStringFlag(cmd, "file-id", "--folder alias")
	addDocHiddenStringFlag(cmd, "nodee", "--folder alias")
	cmd.Flags().String("workspace", "", i18n.T("Workspace ID"))
	addDocHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	cmd.Flags().Int("page-size", 0, i18n.T("每页数量"))
	cmd.Flags().Int("limit", 0, i18n.T("--page-size 的悟空兼容别名"))
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("page-token", "", i18n.T("分页 token"))
	addDocHiddenStringFlag(cmd, "cursor", "--page-token 的悟空兼容别名")
	return cmd
}

func newDocReadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "read",
		Short:             i18n.T("读取文档内容"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID}
			format, err := docContentFormat(cmd, "markdown", "jsonml")
			if err != nil {
				return err
			}
			if format != "" {
				params["format"] = format
			}
			if output := docStringFlag(cmd, "output"); output != "" {
				params["__output__"] = output
			}
			return runDocTool(cmd, runner, "doc", "get_document_content", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("content-format", "", i18n.T("输出格式: markdown / jsonml"))
	cmd.Flags().String("output", "", i18n.T("输出到本地文件路径（JSONML 场景透传）"))
	return cmd
}

func newDocInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "info",
		Short:             i18n.T("获取文档元信息"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			return runDocTool(cmd, runner, "doc", "get_document_info", map[string]any{"nodeId": nodeID})
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	return cmd
}

func newDocCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建文档"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := docRequiredFlagOrFallback(cmd, "name", "title")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name}
			if folder := docFlagOrFallback(cmd, "folder", "parent-id", "parent-folder-id", "parent-folder"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			if workspace := docFlagOrFallback(cmd, "workspace", "workspace-id"); workspace != "" {
				params["workspaceId"] = workspace
			}
			content, err := docResolveContent(cmd)
			if err != nil {
				return err
			}
			if content != "" {
				format, err := docContentFormat(cmd, "markdown", "jsonml")
				if err != nil {
					return err
				}
				if format == "jsonml" {
					jsonml, err := prepareDocJSONMLBody(cmd, content)
					if err != nil {
						return err
					}
					createResult, err := docInvocationResult(cmd, runner, "doc", "create_document", params)
					if err != nil {
						return err
					}
					nodeID := docFirstStringRecursive(createResult.Response, "nodeId", "nodeID", "id")
					if nodeID == "" {
						return apperrors.NewValidation("创建文档成功但无法提取 nodeId")
					}
					updateResult, err := docInvocationResult(cmd, runner, "doc", "update_document", map[string]any{
						"nodeId": nodeID,
						"format": "jsonml",
						"jsonml": jsonml,
						"mode":   "overwrite",
					})
					if err != nil {
						return err
					}
					return writeCommandPayload(cmd, updateResult)
				} else {
					if sniffJsonMLLike(content) {
						fmt.Fprintln(cmd.ErrOrStderr(), `warning: 输入内容看起来是 JSONML 结构；若要按 JSONML 解析，请加 --content-format jsonml，否则将按 markdown 解析。`)
					}
					params["markdown"] = content
				}
			} else if format, err := docContentFormat(cmd, "markdown", "jsonml"); err != nil {
				return err
			} else if format != "" {
				params["format"] = format
			}
			return runDocTool(cmd, runner, "doc", "create_document", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", i18n.T("文档名称 (必填)"))
	addDocHiddenStringFlag(cmd, "title", "--name alias")
	cmd.Flags().String("folder", "", i18n.T("父文件夹 nodeId / URL"))
	addDocHiddenStringFlag(cmd, "parent-id", "--folder alias")
	addDocHiddenStringFlag(cmd, "parent-folder-id", "--folder alias")
	addDocHiddenStringFlag(cmd, "parent-folder", "--folder alias")
	cmd.Flags().String("workspace", "", i18n.T("Workspace ID"))
	addDocHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	cmd.Flags().String("content", "", i18n.T("初始 Markdown 内容"))
	cmd.Flags().String("content-file", "", i18n.T("从文件读取初始 Markdown"))
	addDocHiddenStringFlag(cmd, "content-path", "--content-file alias")
	addDocHiddenStringFlag(cmd, "markdown", "--content alias")
	cmd.Flags().String("content-format", "", i18n.T("内容格式: markdown / jsonml"))
	addDocJSONMLControlFlags(cmd)
	return cmd
}

func newDocUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新文档内容"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			mode, err := docRequiredFlag(cmd, "mode")
			if err != nil {
				return err
			}
			content, err := docResolveContent(cmd)
			if err != nil {
				return err
			}
			if content == "" {
				return apperrors.NewValidation("--content or --content-file is required")
			}
			if docModeRequiresYes(mode) && !docYesFlag(cmd) && !commandDryRun(cmd) {
				return apperrors.NewValidation("--mode overwrite requires --yes unless --dry-run is set")
			}
			params := map[string]any{
				"nodeId": nodeID,
				"mode":   strings.TrimSpace(mode),
			}
			format, err := docContentFormat(cmd, "markdown", "jsonml")
			if err != nil {
				return err
			}
			if format == "jsonml" {
				if strings.TrimSpace(mode) == "append" {
					return apperrors.NewValidation("--content-format jsonml 当前仅支持 --mode overwrite，append 模式将在后续版本支持")
				}
				jsonml, err := prepareDocJSONMLBody(cmd, content)
				if err != nil {
					return err
				}
				params["format"] = "jsonml"
				params["jsonml"] = jsonml
				addDocIntParam(cmd, params, "revision", "revision")
			} else {
				if sniffJsonMLLike(content) {
					fmt.Fprintln(cmd.ErrOrStderr(), `warning: 输入内容看起来是 JSONML 结构；若要按 JSONML 解析，请加 --content-format jsonml，否则将按 markdown 解析。`)
				}
				params["markdown"] = content
				addDocIntParam(cmd, params, "index", "index")
			}
			return runDocTool(cmd, runner, "doc", "update_document", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("content", "", i18n.T("Markdown 内容"))
	cmd.Flags().String("content-file", "", i18n.T("从文件读取 Markdown"))
	addDocHiddenStringFlag(cmd, "content-path", "--content-file alias")
	addDocHiddenStringFlag(cmd, "markdown", "--content alias")
	cmd.Flags().String("mode", "", i18n.T("更新模式: append / overwrite"))
	cmd.Flags().Int("index", 0, i18n.T("append 插入位置"))
	cmd.Flags().String("content-format", "", i18n.T("内容格式: markdown / jsonml"))
	cmd.Flags().Int("revision", 0, i18n.T("文档编辑版本号（JSONML 场景透传）"))
	addDocJSONMLControlFlags(cmd)
	return cmd
}

func newDocDownloadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "download",
		Short:             i18n.T("获取文档下载地址"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			if _, err := docRequiredFlag(cmd, "output"); err != nil {
				return err
			}
			return runDocTool(cmd, runner, "doc", "download_file", map[string]any{"nodeId": nodeID})
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("output", "", i18n.T("本地输出路径 (必填)"))
	return cmd
}

func newDocUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: i18n.T("上传文件到钉钉文档或知识库"),
		Long: i18n.T(`将本地文件上传到钉钉文档或知识库（三步自动完成）。

流程：
  1. 获取上传凭证 (get_file_upload_info)
  2. HTTP PUT 上传文件到 OSS
  3. 提交文件入库 (commit_uploaded_file)

上传位置优先级：--folder > --workspace > 默认文档根目录。`),
		Example: `  dws doc upload --file ./report.pdf
  dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder DOC_FOLDER_NODE_ID
  dws doc upload --file ./data.xlsx --workspace WS_ID --convert`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocUpload(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("file", "", i18n.T("本地文件路径 (必填)"))
	cmd.Flags().String("name", "", i18n.T("文件显示名称（默认使用本地文件名）"))
	cmd.Flags().String("folder", "", i18n.T("目标文档文件夹 nodeId 或 alidocs 文件夹 URL"))
	addDocHiddenStringFlag(cmd, "parent-id", "--folder alias")
	addDocHiddenStringFlag(cmd, "parent-folder", "--folder alias")
	addDocHiddenStringFlag(cmd, "parent-folder-id", "--folder alias")
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL"))
	addDocHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	cmd.Flags().Bool("convert", false, i18n.T("是否转换为钉钉在线文档"))
	return cmd
}

func runDocUpload(cmd *cobra.Command, runner executor.Runner) error {
	filePath, err := docRequiredFlag(cmd, "file")
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return apperrors.NewValidation(i18n.T("无法解析文件路径: ") + err.Error())
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return apperrors.NewValidation(i18n.T("文件不存在: ") + absPath)
	}
	if info.IsDir() {
		return apperrors.NewValidation(i18n.T("不是文件: ") + absPath)
	}
	fileSize := info.Size()
	if fileSize <= 0 {
		return apperrors.NewValidation(i18n.T("文件为空"))
	}

	name := docStringFlag(cmd, "name")
	if name == "" {
		name = filepath.Base(absPath)
	} else if filepath.Ext(name) == "" {
		if ext := filepath.Ext(absPath); ext != "" {
			name += ext
		}
	}

	folder := docFlagOrFallback(cmd, "folder", "parent-id", "parent-folder", "parent-folder-id")
	if folder != "" {
		folder = normalizeDocNodeID(folder)
	}
	workspace := docFlagOrFallback(cmd, "workspace", "workspace-id")
	step1Params := map[string]any{}
	if folder != "" {
		step1Params["folderId"] = folder
	}
	if workspace != "" {
		step1Params["workspaceId"] = workspace
	}

	commitParams := map[string]any{
		"name":     name,
		"fileSize": float64(fileSize),
	}
	if folder != "" {
		commitParams["folderId"] = folder
	}
	if workspace != "" {
		commitParams["workspaceId"] = workspace
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
			"name": name,
			"size": fileSize,
		})
	}

	progressOut := cmd.ErrOrStderr()
	fmt.Fprintf(progressOut, i18n.T("[1/3] 获取文件上传凭证 (%s, %d 字节)...\n"), name, fileSize)
	step1Result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "get_file_upload_info", step1Params,
	))
	if err != nil {
		return fmt.Errorf(i18n.T("获取上传凭证失败: %w"), err)
	}
	resourceURL, uploadKey, headers, err := extractDocFileUploadInfo(step1Result.Response)
	if err != nil {
		return err
	}

	fmt.Fprintln(progressOut, i18n.T("[2/3] 上传文件到 OSS..."))
	if err := httpPutDriveFile(cmd.Context(), resourceURL, headers, absPath, fileSize); err != nil {
		return err
	}

	fmt.Fprintln(progressOut, i18n.T("[3/3] 提交文件入库..."))
	commitParams["uploadKey"] = uploadKey
	result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "commit_uploaded_file", commitParams,
	))
	if err != nil {
		return fmt.Errorf(i18n.T("提交文件入库失败: %w"), err)
	}
	return writeCommandPayload(cmd, result)
}

func extractDocFileUploadInfo(resp map[string]any) (resourceURL, uploadKey string, headers map[string]string, err error) {
	data := unwrapDocUploadResp(resp)
	resourceURL, _ = data["resourceUrl"].(string)
	if resourceURL == "" {
		resourceURL, _ = data["uploadUrl"].(string)
	}
	if resourceURL == "" {
		if urls, ok := data["resourceUrls"].([]any); ok && len(urls) > 0 {
			if first, ok := urls[0].(map[string]any); ok {
				resourceURL, _ = first["url"].(string)
				if len(headers) == 0 {
					headers = stringMapFromAny(first["headers"])
				}
			}
		}
	}
	uploadKey, _ = data["uploadKey"].(string)
	if uploadKey == "" {
		uploadKey, _ = data["uploadId"].(string)
	}
	if len(headers) == 0 {
		headers = stringMapFromAny(data["headers"])
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if resourceURL == "" || uploadKey == "" {
		err = apperrors.NewValidation(fmt.Sprintf("get_file_upload_info 返回不完整: resourceUrl=%q, uploadKey=%q", resourceURL, uploadKey))
		return
	}
	return
}

func unwrapDocUploadResp(resp map[string]any) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	data := resp
	for {
		if content, ok := data["content"].(map[string]any); ok && len(content) > 0 {
			data = content
			continue
		}
		if result, ok := data["result"].(map[string]any); ok && len(result) > 0 {
			data = result
			continue
		}
		if inner, ok := data["data"].(map[string]any); ok && len(inner) > 0 {
			data = inner
			continue
		}
		return data
	}
}

func stringMapFromAny(raw any) map[string]string {
	headers := map[string]string{}
	if m, ok := raw.(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok && s != "" {
				headers[k] = s
			}
		}
	}
	if m, ok := raw.(map[string]string); ok {
		for k, v := range m {
			if v != "" {
				headers[k] = v
			}
		}
	}
	return headers
}

func newDocFileCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建文档文件"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := docRequiredFlagOrFallback(cmd, "name", "title")
			if err != nil {
				return err
			}
			fileType, err := docRequiredFlag(cmd, "type")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name, "type": fileType}
			if folder := docFlagOrFallback(cmd, "folder", "parent-id"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			if workspace := docFlagOrFallback(cmd, "workspace", "workspace-id"); workspace != "" {
				params["workspaceId"] = workspace
			}
			return runDocTool(cmd, runner, "doc", "create_file", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", i18n.T("文件名称 (必填)"))
	addDocHiddenStringFlag(cmd, "title", "--name alias")
	cmd.Flags().String("type", "", i18n.T("文件类型 (必填)"))
	cmd.Flags().String("folder", "", i18n.T("父文件夹 nodeId / URL"))
	addDocHiddenStringFlag(cmd, "parent-id", "--folder alias")
	cmd.Flags().String("workspace", "", i18n.T("Workspace ID"))
	addDocHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	return cmd
}

func newDocFolderCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建文档文件夹"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := docRequiredFlagOrFallback(cmd, "name", "title")
			if err != nil {
				return err
			}
			params := map[string]any{"name": name}
			if folder := docFlagOrFallback(cmd, "folder", "parent-id"); folder != "" {
				params["folderId"] = normalizeDocNodeID(folder)
			}
			if workspace := docFlagOrFallback(cmd, "workspace", "workspace-id"); workspace != "" {
				params["workspaceId"] = workspace
			}
			return runDocTool(cmd, runner, "doc", "create_folder", params)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("name", "", i18n.T("文件夹名称 (必填)"))
	addDocHiddenStringFlag(cmd, "title", "--name alias")
	cmd.Flags().String("folder", "", i18n.T("父文件夹 nodeId / URL"))
	addDocHiddenStringFlag(cmd, "parent-id", "--folder alias")
	cmd.Flags().String("workspace", "", i18n.T("Workspace ID"))
	addDocHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	return cmd
}

func newDocCopyCommand(runner executor.Runner) *cobra.Command {
	return newDocTransferCommand(runner, "copy", "copy_document")
}

func newDocMoveCommand(runner executor.Runner) *cobra.Command {
	return newDocTransferCommand(runner, "move", "move_document")
}

func newDocTransferCommand(runner executor.Runner, use, tool string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             i18n.T(use + " 文档节点"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID}
			if folder := docFlagOrFallback(cmd, "folder", "parent-id"); folder != "" {
				params["targetFolderId"] = normalizeDocNodeID(folder)
			}
			if workspace := docFlagOrFallback(cmd, "workspace", "workspace-id"); workspace != "" {
				params["workspaceId"] = workspace
			}
			return runDocTool(cmd, runner, "doc", tool, params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("folder", "", i18n.T("目标父文件夹 nodeId / URL"))
	addDocHiddenStringFlag(cmd, "parent-id", "--folder alias")
	cmd.Flags().String("workspace", "", i18n.T("Workspace ID"))
	addDocHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	return cmd
}

func newDocRenameCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rename",
		Short:             i18n.T("重命名文档节点"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			name, err := docRequiredFlagOrFallback(cmd, "name", "title")
			if err != nil {
				return err
			}
			return runDocTool(cmd, runner, "doc", "rename_document", map[string]any{
				"nodeId":  nodeID,
				"newName": name,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("name", "", i18n.T("新名称 (必填)"))
	addDocHiddenStringFlag(cmd, "title", "--name alias")
	return cmd
}

func newDocBlockListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("列出文档块"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID}
			addDocIntParam(cmd, params, "startIndex", "start-index")
			addDocIntParam(cmd, params, "endIndex", "end-index")
			addDocStringParam(cmd, params, "blockType", "block-type")
			format, err := docContentFormat(cmd, "element", "jsonml")
			if err != nil {
				return err
			}
			if format != "" {
				params["format"] = format
			}
			addDocStringParam(cmd, params, "blockId", "block-id")
			return runDocTool(cmd, runner, "doc", "list_document_blocks", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().Int("start-index", 0, i18n.T("起始块索引"))
	cmd.Flags().Int("end-index", 0, i18n.T("结束块索引"))
	cmd.Flags().String("block-type", "", i18n.T("块类型"))
	cmd.Flags().String("content-format", "", i18n.T("输出格式: element / jsonml"))
	cmd.Flags().String("block-id", "", i18n.T("块 ID（JSONML 场景透传）"))
	return cmd
}

func newDocBlockInsertCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "insert",
		Short:             i18n.T("插入文档块"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID}
			format, err := docContentFormat(cmd, "element", "jsonml")
			if err != nil {
				return err
			}
			if format == "jsonml" {
				jsonml, err := docRequiredFlag(cmd, "element")
				if err != nil {
					return err
				}
				normalized, err := prepareDocJSONMLNode(cmd, jsonml)
				if err != nil {
					return err
				}
				params["format"] = "jsonml"
				params["jsonml"] = normalized
				if refBlock := docStringFlag(cmd, "ref-block"); refBlock != "" {
					params["referenceBlockId"] = refBlock
					where := normalizeDocInsertWhere(docStringFlag(cmd, "where"))
					if where == "" {
						where = "after"
					}
					params["where"] = where
				}
				if parentBlock := docStringFlag(cmd, "parent-block"); parentBlock != "" {
					params["referenceBlockId"] = parentBlock
				}
				addDocIntParam(cmd, params, "index", "index")
			} else {
				if elementRaw := docStringFlag(cmd, "element"); elementRaw != "" && sniffJsonMLLike(elementRaw) {
					fmt.Fprintln(cmd.ErrOrStderr(), `warning: --element 内容看起来是 JSONML 结构；若要按 JSONML 解析，请加 --content-format jsonml，否则将按 element 解析。`)
				}
				element, err := docBuildBlockElement(cmd)
				if err != nil {
					return err
				}
				params["element"] = element
				addDocIntParam(cmd, params, "index", "index")
				if where := normalizeDocInsertWhere(docStringFlag(cmd, "where")); where != "" {
					params["where"] = where
				}
				addDocStringParam(cmd, params, "referenceBlockId", "ref-block")
			}
			return runDocTool(cmd, runner, "doc", "insert_document_block", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("element", "", i18n.T("块 element JSON"))
	cmd.Flags().String("text", "", i18n.T("段落文本快捷参数"))
	cmd.Flags().String("heading", "", i18n.T("标题文本快捷参数"))
	cmd.Flags().String("type", "", i18n.T("块类型快捷参数: paragraph/blockquote/callout/code/orderedList/unorderedList/columns/divider"))
	cmd.Flags().String("list-id", "", i18n.T("列表 ID；orderedList/unorderedList 多项连续插入时使用同一个 ID"))
	cmd.Flags().String("language", "", i18n.T("代码块语言；配合 --type code/codeBlock"))
	cmd.Flags().Int("columns", 2, i18n.T("分栏数量；配合 --type columns"))
	cmd.Flags().Int("level", 1, i18n.T("标题级别"))
	cmd.Flags().Int("index", 0, i18n.T("插入索引"))
	cmd.Flags().String("where", "", i18n.T("插入位置"))
	cmd.Flags().String("ref-block", "", i18n.T("参考块 ID"))
	cmd.Flags().String("content-format", "", i18n.T("输入格式: element / jsonml"))
	cmd.Flags().String("parent-block", "", i18n.T("父容器块 ID（JSONML 场景透传）"))
	addDocJSONMLControlFlags(cmd)
	return cmd
}

func newDocBlockUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新文档块"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			blockID, err := docRequiredFlag(cmd, "block-id")
			if err != nil {
				return err
			}
			params := map[string]any{
				"nodeId":  nodeID,
				"blockId": blockID,
			}
			format, err := docContentFormat(cmd, "element", "jsonml")
			if err != nil {
				return err
			}
			if format == "jsonml" {
				jsonml, err := docRequiredFlag(cmd, "element")
				if err != nil {
					return err
				}
				normalized, err := prepareDocJSONMLNode(cmd, jsonml)
				if err != nil {
					return err
				}
				params["format"] = "jsonml"
				params["jsonml"] = normalized
			} else {
				if elementRaw := docStringFlag(cmd, "element"); elementRaw != "" && sniffJsonMLLike(elementRaw) {
					fmt.Fprintln(cmd.ErrOrStderr(), `warning: --element 内容看起来是 JSONML 结构；若要按 JSONML 解析，请加 --content-format jsonml，否则将按 element 解析。`)
				}
				element, err := docBuildBlockElement(cmd)
				if err != nil {
					return err
				}
				params["element"] = element
			}
			return runDocTool(cmd, runner, "doc", "update_document_block", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("block-id", "", i18n.T("块 ID (必填)"))
	cmd.Flags().String("element", "", i18n.T("块 element JSON"))
	cmd.Flags().String("text", "", i18n.T("段落文本快捷参数"))
	cmd.Flags().String("heading", "", i18n.T("标题文本快捷参数"))
	cmd.Flags().String("type", "", i18n.T("块类型快捷参数: paragraph/blockquote/callout/code/orderedList/unorderedList/columns/divider"))
	cmd.Flags().String("list-id", "", i18n.T("列表 ID；orderedList/unorderedList 多项连续插入时使用同一个 ID"))
	cmd.Flags().String("language", "", i18n.T("代码块语言；配合 --type code/codeBlock"))
	cmd.Flags().Int("columns", 2, i18n.T("分栏数量；配合 --type columns"))
	cmd.Flags().Int("level", 1, i18n.T("标题级别"))
	cmd.Flags().String("content-format", "", i18n.T("输入格式: element / jsonml"))
	addDocJSONMLControlFlags(cmd)
	return cmd
}

func newDocBlockDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除文档块"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			blockID, err := docRequiredFlag(cmd, "block-id")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("文档块"), blockID) {
				return nil
			}
			return runDocTool(cmd, runner, "doc", "delete_document_block", map[string]any{
				"nodeId":  nodeID,
				"blockId": blockID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("block-id", "", i18n.T("块 ID (必填)"))
	return cmd
}

func newDocCommentListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("列出文档评论"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID}
			if pageSize := docIntFlagOrFallback(cmd, "page-size", "limit"); pageSize > 0 {
				params["pageSize"] = pageSize
			}
			addDocStringParam(cmd, params, "nextToken", "next-token", "cursor")
			addDocStringParam(cmd, params, "commentType", "type")
			addDocStringParam(cmd, params, "resolveStatus", "resolve-status")
			return runDocTool(cmd, runner, "doc-comment", "list_comments", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().Int("page-size", 0, i18n.T("每页数量"))
	cmd.Flags().Int("limit", 0, i18n.T("--page-size 的悟空兼容别名"))
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("next-token", "", i18n.T("下一页 token"))
	addDocHiddenStringFlag(cmd, "cursor", "--next-token 的悟空兼容别名")
	cmd.Flags().String("type", "", i18n.T("评论类型"))
	cmd.Flags().String("resolve-status", "", i18n.T("解决状态"))
	return cmd
}

func newDocCommentCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建文档评论"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			content, err := docRequiredFlag(cmd, "content")
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID, "content": content}
			addDocCSVParam(cmd, params, "mentionedUserIds", "mention")
			return runDocTool(cmd, runner, "doc-comment", "create_comment", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("content", "", i18n.T("评论内容 (必填)"))
	cmd.Flags().String("mention", "", i18n.T("提及 userId，逗号分隔"))
	return cmd
}

func newDocCommentReplyCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "reply",
		Short:             i18n.T("回复文档评论"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			content, err := docRequiredFlag(cmd, "content")
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID, "content": content}
			addDocStringParam(cmd, params, "replyCommentKey", "comment-key")
			if emoji, _ := cmd.Flags().GetBool("emoji"); emoji {
				params["emoji"] = true
			}
			addDocCSVParam(cmd, params, "mentionedUserIds", "mention")
			return runDocTool(cmd, runner, "doc-comment", "reply_comment", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("content", "", i18n.T("回复内容 (必填)"))
	cmd.Flags().String("comment-key", "", i18n.T("评论 key"))
	cmd.Flags().Bool("emoji", false, i18n.T("是否作为表情贴图回复"))
	cmd.Flags().String("mention", "", i18n.T("提及 userId，逗号分隔"))
	return cmd
}

func newDocCommentCreateInlineCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create-inline",
		Short:             i18n.T("创建行内评论"),
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			content, err := docRequiredFlag(cmd, "content")
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID, "content": content}
			addDocStringParam(cmd, params, "blockId", "block-id")
			addDocIntParam(cmd, params, "start", "start")
			addDocIntParam(cmd, params, "end", "end")
			addDocStringParam(cmd, params, "selectedText", "selected-text")
			addDocCSVParam(cmd, params, "mentionedUserIds", "mention")
			return runDocTool(cmd, runner, "doc-comment", "create_inline_comment", params)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("content", "", i18n.T("评论内容 (必填)"))
	cmd.Flags().String("block-id", "", i18n.T("块 ID"))
	cmd.Flags().Int("start", 0, i18n.T("起始位置"))
	cmd.Flags().Int("end", 0, i18n.T("结束位置"))
	cmd.Flags().String("selected-text", "", i18n.T("选中文本"))
	cmd.Flags().String("mention", "", i18n.T("提及 userId，逗号分隔"))
	return cmd
}

func docBuildBlockElement(cmd *cobra.Command) (any, error) {
	if raw := docStringFlag(cmd, "element"); raw != "" {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, apperrors.NewValidation(fmt.Sprintf("--element JSON parse failed: %v", err))
		}
		return value, nil
	}
	if heading := docStringFlag(cmd, "heading"); heading != "" {
		level, _ := cmd.Flags().GetInt("level")
		if level < 1 || level > 6 {
			level = 1
		}
		return map[string]any{
			"blockType": "heading",
			"heading": map[string]any{
				"text":  heading,
				"level": level,
			},
		}, nil
	}
	if blockType := docStringFlag(cmd, "type"); blockType != "" {
		return docBuildTypedBlockElement(cmd, blockType)
	}
	if text := docStringFlag(cmd, "text"); text != "" {
		return map[string]any{
			"blockType": "paragraph",
			"paragraph": map[string]any{
				"text": text,
			},
		}, nil
	}
	return nil, apperrors.NewValidation("block content required: --text, --heading, or --element")
}

func docBuildTypedBlockElement(cmd *cobra.Command, blockType string) (any, error) {
	text := docStringFlag(cmd, "text")
	normalized := normalizeDocBlockType(blockType)
	switch normalized {
	case "paragraph":
		if text == "" {
			return nil, apperrors.NewValidation("--text is required when --type paragraph")
		}
		return map[string]any{
			"blockType": "paragraph",
			"paragraph": map[string]any{"text": text},
		}, nil
	case "heading":
		if text == "" {
			return nil, apperrors.NewValidation("--text is required when --type heading; or use --heading")
		}
		level, _ := cmd.Flags().GetInt("level")
		if level < 1 || level > 6 {
			level = 1
		}
		return map[string]any{
			"blockType": "heading",
			"heading":   map[string]any{"text": text, "level": level},
		}, nil
	case "blockquote":
		if text == "" {
			return nil, apperrors.NewValidation("--text is required when --type blockquote")
		}
		return map[string]any{
			"blockType":  "blockquote",
			"blockquote": map[string]any{"text": text},
		}, nil
	case "callout":
		if text == "" {
			return nil, apperrors.NewValidation("--text is required when --type callout")
		}
		return map[string]any{
			"blockType": "callout",
			"callout":   map[string]any{"text": text},
		}, nil
	case "codeBlock":
		if text == "" {
			return nil, apperrors.NewValidation("--text is required when --type code")
		}
		codeBlock := map[string]any{"text": text}
		if language := docStringFlag(cmd, "language"); language != "" {
			codeBlock["language"] = language
		}
		return map[string]any{
			"blockType": "codeBlock",
			"codeBlock": codeBlock,
		}, nil
	case "orderedList", "unorderedList":
		if text == "" {
			return nil, apperrors.NewValidation("--text is required when --type orderedList/unorderedList")
		}
		listID := docStringFlag(cmd, "list-id")
		if listID == "" {
			listID = "list-1"
		}
		return map[string]any{
			"blockType": normalized,
			normalized:  map[string]any{"list": map[string]any{"listId": listID}},
			"children":  []any{map[string]any{"text": text}},
		}, nil
	case "columns":
		count, _ := cmd.Flags().GetInt("columns")
		if count < 2 {
			count = 2
		}
		parts := splitDocColumnsText(text, count)
		children := make([]any, 0, count)
		for i := 0; i < count; i++ {
			childText := ""
			if i < len(parts) {
				childText = parts[i]
			}
			children = append(children, map[string]any{
				"blockType": "paragraph",
				"paragraph": map[string]any{"text": childText},
			})
		}
		return map[string]any{
			"blockType": "columns",
			"columns":   map[string]any{"size": count},
			"children":  children,
		}, nil
	case "divider":
		return map[string]any{"blockType": "divider"}, nil
	default:
		return nil, apperrors.NewValidation(fmt.Sprintf("unsupported --type %q; use --element for advanced block JSON", blockType))
	}
}

func normalizeDocBlockType(blockType string) string {
	switch strings.ToLower(strings.TrimSpace(blockType)) {
	case "p", "text", "paragraph":
		return "paragraph"
	case "h", "heading", "title":
		return "heading"
	case "quote", "blockquote":
		return "blockquote"
	case "callout", "highlight", "notice":
		return "callout"
	case "code", "codeblock", "code_block", "code-block":
		return "codeBlock"
	case "orderedlist", "ordered-list", "ol", "numberedlist", "numbered-list":
		return "orderedList"
	case "unorderedlist", "unordered-list", "ul", "bulletlist", "bullet-list":
		return "unorderedList"
	case "columns", "column":
		return "columns"
	case "divider", "hr", "line":
		return "divider"
	default:
		return strings.TrimSpace(blockType)
	}
}

func splitDocColumnsText(text string, count int) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "||")
	if len(parts) == 1 {
		parts = strings.Split(text, "|")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) > count {
		parts = parts[:count]
	}
	return parts
}

func normalizeDocInsertWhere(where string) string {
	switch strings.ToLower(strings.TrimSpace(where)) {
	case "", "end", "tail", "append", "末尾":
		return ""
	default:
		return strings.TrimSpace(where)
	}
}

func runDocTool(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) error {
	result, err := docInvocationResult(cmd, runner, product, tool, params)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func docInvocationResult(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) (executor.Result, error) {
	invocation := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), product, tool, params)
	invocation.DryRun = commandDryRun(cmd)
	return runner.Run(cmd.Context(), invocation)
}

func addDocNodeFlags(cmd *cobra.Command) {
	cmd.Flags().String("node", "", i18n.T("文档 nodeId / URL"))
	addDocHiddenStringFlag(cmd, "url", "--node alias")
	addDocHiddenStringFlag(cmd, "id", "--node alias")
	addDocHiddenStringFlag(cmd, "node-id", "--node alias")
	addDocHiddenStringFlag(cmd, "doc-id", "--node alias")
	addDocHiddenStringFlag(cmd, "file-id", "--node alias")
}

func addDocJSONMLControlFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("fix-jsonml", false, i18n.T("启用全部 JSONML 修复（含 JSON 语法修复 + 结构修复），推荐 agent 调用时使用"))
	cmd.Flags().Bool("no-fix-jsonml", false, i18n.T("关闭全部 JSONML 修复（跳过 JSON 语法修复和结构修复），用于排查原始错误"))
}

func addDocHiddenStringFlag(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", usage)
	_ = cmd.Flags().MarkHidden(name)
}

func docRequiredNode(cmd *cobra.Command) (string, error) {
	nodeID, err := docRequiredFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if err != nil {
		return "", err
	}
	return normalizeDocNodeID(nodeID), nil
}

func docResolveContent(cmd *cobra.Command) (string, error) {
	if path := docFlagOrFallback(cmd, "content-file", "content-path"); path != "" {
		return docReadContent(path)
	}
	raw := docFlagOrFallback(cmd, "content", "markdown")
	if raw == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", apperrors.NewValidation("read stdin: " + err.Error())
		}
		return string(data), nil
	}
	return raw, nil
}

func docReadContent(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", apperrors.NewValidation("read stdin: " + err.Error())
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", apperrors.NewValidation("read " + path + ": " + err.Error())
	}
	return string(data), nil
}

func normalizeDocNodeID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || !strings.Contains(value, "://") {
		return value
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == '?' || r == '#'
	})
	for i, part := range parts {
		if part == "nodes" && i+1 < len(parts) && strings.TrimSpace(parts[i+1]) != "" {
			return strings.TrimSpace(parts[i+1])
		}
	}
	return value
}

func docStringFlag(cmd *cobra.Command, name string) string {
	if value, err := cmd.Flags().GetString(name); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func docFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	if value := docStringFlag(cmd, primary); value != "" {
		return value
	}
	for _, alias := range aliases {
		if value := docStringFlag(cmd, alias); value != "" {
			return value
		}
	}
	return ""
}

func docContentFormat(cmd *cobra.Command, allowed ...string) (string, error) {
	format := docStringFlag(cmd, "content-format")
	if format == "" {
		return "", nil
	}
	for _, candidate := range allowed {
		if format == candidate {
			return format, nil
		}
	}
	if format == "json" {
		return "", apperrors.NewValidation(fmt.Sprintf("--content-format json 无效。这里的 --content-format 指文档内容格式，不是 CLI 输出格式；CLI 输出格式由顶层 -f/--format 控制。本命令接受: %s", renderDocAllowedFormats(allowed)))
	}
	return "", apperrors.NewValidation(fmt.Sprintf("--content-format 取值 %q 无效，仅接受: %s", format, renderDocAllowedFormats(allowed)))
}

func renderDocAllowedFormats(allowed []string) string {
	parts := make([]string, 0, len(allowed))
	for _, allowedFormat := range allowed {
		if allowedFormat != "" {
			parts = append(parts, allowedFormat)
		}
	}
	return strings.Join(parts, ", ")
}

func docFirstStringRecursive(v any, keys ...string) string {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	var walk func(any) string
	walk = func(cur any) string {
		switch x := cur.(type) {
		case map[string]any:
			for _, key := range keys {
				if value, ok := x[key].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
			for key, value := range x {
				if _, direct := keySet[key]; direct {
					continue
				}
				if found := walk(value); found != "" {
					return found
				}
			}
		case []any:
			for _, item := range x {
				if found := walk(item); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(v)
}

func docRequiredFlag(cmd *cobra.Command, name string) (string, error) {
	if value := docStringFlag(cmd, name); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation("--" + name + " is required")
}

func docRequiredFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) (string, error) {
	if value := docFlagOrFallback(cmd, primary, aliases...); value != "" {
		return value, nil
	}
	return "", apperrors.NewValidation("--" + primary + " is required")
}

func docIntFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) int {
	names := append([]string{primary}, aliases...)
	for _, name := range names {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || !flag.Changed {
			continue
		}
		value, err := cmd.Flags().GetInt(name)
		if err == nil && value > 0 {
			return value
		}
	}
	for _, name := range names {
		value, err := cmd.Flags().GetInt(name)
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func docModeRequiresYes(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "overwrite")
}

func docYesFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if value, err := cmd.Flags().GetBool("yes"); err == nil {
		return value
	}
	if value, err := cmd.InheritedFlags().GetBool("yes"); err == nil {
		return value
	}
	if root := cmd.Root(); root != nil {
		if value, err := root.PersistentFlags().GetBool("yes"); err == nil {
			return value
		}
	}
	return false
}

func addDocStringParam(cmd *cobra.Command, params map[string]any, paramName string, flags ...string) {
	for _, flag := range flags {
		if value := docStringFlag(cmd, flag); value != "" {
			params[paramName] = value
			return
		}
	}
}

func addDocCSVParam(cmd *cobra.Command, params map[string]any, paramName string, flags ...string) {
	for _, flag := range flags {
		if value := docStringFlag(cmd, flag); value != "" {
			params[paramName] = docCSV(value)
			return
		}
	}
}

func addDocIntParam(cmd *cobra.Command, params map[string]any, paramName string, flags ...string) {
	for _, flag := range flags {
		if !cmd.Flags().Changed(flag) {
			continue
		}
		if value, err := cmd.Flags().GetInt(flag); err == nil {
			params[paramName] = value
			return
		}
		if value, err := cmd.Flags().GetInt64(flag); err == nil {
			params[paramName] = value
			return
		}
	}
}

func docCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// TRANSITIONAL: 等 mse 把 submit_export_job / query_export_job 加入
// doc toolOverrides 后，本节 helper 可整体删除。工单：plan/mse-yuyuan-patch.md
// 改动 2.2。
//
// 设计：用 pkg/asynctask.Submit 串起来：
//  1. 调 submit_export_job 拿 jobId
//  2. 渐进式退避轮询 query_export_job 直到 SUCCESS / FAILED / 超时
//  3. SUCCESS 后自动 GET downloadUrl 落盘到必填的 --output 路径。

func runDocExport(cmd *cobra.Command, runner executor.Runner) error {
	nodeID := docFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if strings.TrimSpace(nodeID) == "" {
		// 没传 --node 时按 group 命令处理（打 help）
		if !cmd.Flags().Changed("node") && !cmd.Flags().Changed("output") {
			return cmd.Help()
		}
		return apperrors.NewValidation("--node is required")
	}
	nodeID = normalizeDocNodeID(nodeID)
	output, _ := cmd.Flags().GetString("output")
	if strings.TrimSpace(output) == "" {
		return apperrors.NewValidation("--output is required")
	}
	timeoutSec, _ := cmd.Flags().GetInt("timeout-sec")
	// 进度文案走 stderr，与 aitable export 等同类异步命令一致；stdout 仅留给
	// writeCommandPayload 的结构化 JSON，否则会污染 agent / MCP / `| jq` 的解析。
	progressOut := cmd.ErrOrStderr()
	emitProgress := func(format string, args ...any) {
		msg := fmt.Sprintf(i18n.T(format), args...)
		fmt.Fprint(progressOut, msg)
	}
	exportFormat := strings.TrimSpace(docStringFlag(cmd, "export-format"))
	if exportFormat != "" && !strings.EqualFold(exportFormat, "docx") {
		return apperrors.NewValidation("--export-format only supports docx")
	}

	submitFn := func(ctx context.Context) (string, error) {
		params := map[string]any{"nodeId": nodeID}
		if exportFormat != "" {
			params["exportFormat"] = exportFormat
		}
		result, err := runner.Run(ctx, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", "submit_export_job", params,
		))
		if err != nil {
			return "", err
		}
		jobID := extractDocExportJobID(result.Response)
		if jobID != "" {
			emitProgress("    任务已提交，jobId: %s\n", jobID)
			emitProgress("[2/3] 等待导出完成...\n")
		}
		return jobID, nil
	}

	queryFn := func(ctx context.Context, jobID string) (asynctask.QueryResult, error) {
		result, err := runner.Run(ctx, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", "query_export_job",
			map[string]any{"jobId": jobID},
		))
		if err != nil {
			return asynctask.QueryResult{}, err
		}
		return parseDocExportQueryResult(result.Response), nil
	}

	if commandDryRun(cmd) {
		params := map[string]any{"nodeId": nodeID, "__async__": true, "__output__": output}
		if exportFormat != "" {
			params["exportFormat"] = exportFormat
		}
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", "submit_export_job",
			params,
		))
	}

	emitProgress("[1/3] 提交导出任务...\n")
	res, err := asynctask.Submit(cmd.Context(), submitFn, queryFn, asynctask.Options{
		Timeout: time.Duration(timeoutSec) * time.Second,
		ProgressFn: func(attempt int, status asynctask.Status, elapsed time.Duration) {
			emitProgress("    第 %d/30 次查询（状态=%s，已耗时 %s）\n",
				attempt, status, elapsed.Round(time.Second))
		},
	})
	if err != nil {
		return err
	}

	out := map[string]any{
		"jobId":  res.JobID,
		"status": string(res.Status),
	}
	if res.DownloadURL != "" {
		out["downloadUrl"] = res.DownloadURL
	}
	if res.Message != "" {
		out["message"] = res.Message
	}

	switch res.Status {
	case asynctask.StatusSuccess:
		if res.DownloadURL != "" {
			outputPath := resolveDocExportOutputPath(output, res.DownloadURL, res.JobID)
			emitProgress("[3/3] 下载文件到 %s ...\n", outputPath)
			if err := asynctask.Download(cmd.Context(), res.DownloadURL, outputPath); err != nil {
				return fmt.Errorf(i18n.T("download failed: %w"), err)
			}
			out["output"] = outputPath
			emitProgress("导出完成: %s\n", outputPath)
		}
	case asynctask.StatusFailed:
		return apperrors.NewValidation(fmt.Sprintf("export failed: %s", res.Message))
	case asynctask.StatusTimeout:
		emitProgress("任务超时，请用 dws doc export get --job-id %s 继续等待\n", res.JobID)
	}

	return writeCommandPayload(cmd, out)
}

func resolveDocExportOutputPath(outputPath, downloadURL, jobID string) string {
	if info, statErr := os.Stat(outputPath); statErr == nil && info.IsDir() {
		return filepath.Join(outputPath, docExportFilename(downloadURL, jobID))
	}
	return outputPath
}

func docExportFilename(downloadURL, jobID string) string {
	if parsed, err := url.Parse(downloadURL); err == nil {
		name := cleanDocExportFilename(filepath.Base(parsed.Path))
		if name != "" {
			if decoded, decodeErr := url.PathUnescape(name); decodeErr == nil {
				if decodedName := cleanDocExportFilename(decoded); decodedName != "" {
					return decodedName
				}
			}
			return name
		}
	}
	if jobID != "" {
		return "doc_export_" + jobID + ".docx"
	}
	return "doc_export.docx"
}

func cleanDocExportFilename(name string) string {
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

func newDocExportGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: i18n.T("查询导出任务结果（兜底，dws doc export 已自动轮询）"),
		Long: i18n.T(`根据 jobId 查询文档导出任务的执行结果。

通常不需要手动调用 —— dws doc export 已内置自动轮询。
仅在导出命令超时或中断后，用于手动查询任务状态/续等。

任务状态：
  PROCESSING  处理中
  SUCCESS     导出成功，返回 downloadUrl
  FAILED      导出失败`),
		Example:           "  dws doc export get --job-id <JOB_ID>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID, _ := cmd.Flags().GetString("job-id")
			if strings.TrimSpace(jobID) == "" {
				return apperrors.NewValidation("--job-id is required")
			}
			if !docExportJobIDLooksValid(jobID) {
				return apperrors.NewValidation("--job-id must be a numeric export job ID")
			}
			output, _ := cmd.Flags().GetString("output")
			timeoutSec, _ := cmd.Flags().GetInt("timeout-sec")

			queryFn := func(ctx context.Context, jobID string) (asynctask.QueryResult, error) {
				result, err := runner.Run(ctx, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "query_export_job",
					map[string]any{"jobId": jobID},
				))
				if err != nil {
					return asynctask.QueryResult{}, err
				}
				return parseDocExportQueryResult(result.Response), nil
			}

			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "query_export_job",
					map[string]any{"jobId": jobID},
				))
			}

			res, err := asynctask.Resume(cmd.Context(), jobID, queryFn, asynctask.Options{
				Timeout: time.Duration(timeoutSec) * time.Second,
			})
			if err != nil {
				return err
			}
			out := map[string]any{
				"jobId":  res.JobID,
				"status": string(res.Status),
			}
			if res.DownloadURL != "" {
				out["downloadUrl"] = res.DownloadURL
			}
			if res.Message != "" {
				out["message"] = res.Message
			}
			if res.Status == asynctask.StatusSuccess && output != "" && res.DownloadURL != "" {
				outputPath := resolveDocExportOutputPath(output, res.DownloadURL, res.JobID)
				if err := asynctask.Download(cmd.Context(), res.DownloadURL, outputPath); err != nil {
					return fmt.Errorf(i18n.T("download failed: %w"), err)
				}
				out["output"] = outputPath
			}
			if res.Status == asynctask.StatusFailed {
				return apperrors.NewValidation(fmt.Sprintf("export failed: %s", res.Message))
			}
			return writeCommandPayload(cmd, out)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("job-id", "", i18n.T("导出任务 ID (必填)"))
	cmd.Flags().String("output", "", i18n.T("本地落盘路径（可选，提供则自动下载 docx 到本地）"))
	cmd.Flags().Int("timeout-sec", 300, i18n.T("整体轮询超时（秒），默认 300"))
	return cmd
}

func docExportJobIDLooksValid(jobID string) bool {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return false
	}
	for _, r := range jobID {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// extractDocExportJobID 从 submit_export_job 响应里抽 jobId。
func extractDocExportJobID(resp map[string]any) string {
	src := unwrapDocResp(resp)
	if id, ok := src["jobId"].(string); ok && id != "" {
		return id
	}
	if id, ok := src["taskId"].(string); ok && id != "" {
		return id
	}
	return ""
}

// parseDocExportQueryResult 把 query_export_job 的响应转成 asynctask.QueryResult。
// 注意：必须复用 normalizeAsyncStatus（来自 aitable_export_import.go），否则服务端
// 返回 SUCCEED / DONE / FINISHED / FAILURE 等变体时，asynctask.Resume 会走 default
// 分支当成 PROCESSING 继续等到超时。
func parseDocExportQueryResult(resp map[string]any) asynctask.QueryResult {
	src := unwrapDocResp(resp)
	statusRaw, _ := src["status"].(string)
	msg, _ := src["message"].(string)
	url, _ := src["downloadUrl"].(string)
	return asynctask.QueryResult{
		Status:      normalizeAsyncStatus(statusRaw, url != ""),
		DownloadURL: url,
		Message:     msg,
		Raw:         src,
	}
}

// unwrapDocResp 处理两种包装层次：result.Response 直接含字段 / 含 content.data。
func unwrapDocResp(resp map[string]any) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	if content, ok := resp["content"].(map[string]any); ok && len(content) > 0 {
		resp = content
	}
	if data, ok := resp["data"].(map[string]any); ok && len(data) > 0 {
		return data
	}
	return resp
}

// TRANSITIONAL: 等 mse 把 add_permission / update_permission /
// list_permission 加入 doc toolOverrides（group: "permission"）后，
// 本节 3 个 helper 可整体删除。工单：plan/mse-yuyuan-patch.md 改动 2.2。
//
// 与 wiki member add 的关键区别：
//   - wiki member add —— 知识库（workspace）容器级授权
//   - doc permission —— 节点（document/file/folder）级授权

var docPermissionRoles = map[string]bool{
	"MANAGER":    true,
	"EDITOR":     true,
	"DOWNLOADER": true,
	"READER":     true,
}

// normalizeDocPermissionRole 把用户输入的 role 转为悟空兼容的大写形式。
// 返回 (规范化后的 role, 是否合法)。OWNER 不允许通过本接口设置。
func normalizeDocPermissionRole(raw string) (string, bool) {
	r := strings.ToUpper(strings.TrimSpace(raw))
	if r == "" {
		return "", false
	}
	if r == "OWNER" {
		// OWNER 不可通过 add/update 接口添加，统一拒绝
		return r, false
	}
	return r, docPermissionRoles[r]
}

// parseDocPermissionUsers 把逗号分隔的 userId 列表拆成数组，单次最多 30 个。
func parseDocPermissionUsers(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	users := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			users = append(users, p)
		}
	}
	if len(users) == 0 {
		return nil, apperrors.NewValidation("--user must contain at least 1 userId")
	}
	if len(users) > 30 {
		return nil, apperrors.NewValidation(fmt.Sprintf("--user supports at most 30 ids per call (got %d)", len(users)))
	}
	return users, nil
}

func newDocPermissionAddCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: i18n.T("添加文档协作者"),
		Long: i18n.T(`为指定节点（文档/文件夹/文件）添加一个或多个协作成员，并授予指定角色。

支持角色（--role 大小写不敏感，内部规范化为大写）：
  MANAGER     管理员，可读写、管理成员
  EDITOR      编辑者，可查看、编辑、上传内容
  DOWNLOADER  查看下载者，可查看并下载内容
  READER      仅可查看者

注意：
  - OWNER 角色不可通过本命令添加
  - 单次 --user 最多 30 个 id；超过请分批调用
  - 本命令是节点级授权，跟 dws wiki member add（容器级授权）不同`),
		Example: `  dws doc permission add --node DOC_ID --user uid1,uid2 --role READER
  dws doc permission add --node DOC_ID --user uid1 --role MANAGER`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocPermissionMutation(cmd, runner, "add_permission")
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("user", "", i18n.T("被授权用户 userId 列表，逗号分隔，单次最多 30 (必填)"))
	addDocHiddenStringFlag(cmd, "users", "--user alias")
	cmd.Flags().String("role", "", i18n.T("权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填，大小写不敏感)"))
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL（选填，辅助构造返回的 docUrl）"))
	return cmd
}

func newDocPermissionUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("更新文档协作者权限"),
		Long: i18n.T(`更新指定节点已有协作者的权限角色（仅支持 USER 类型成员）。

支持角色与限制同 dws doc permission add。

仅可更新已存在协作关系的用户；新增协作者请使用 add。`),
		Example: `  dws doc permission update --node DOC_ID --user uid1 --role EDITOR
  dws doc permission update --node DOC_ID --user uid1,uid2 --role READER`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocPermissionMutation(cmd, runner, "update_permission")
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("user", "", i18n.T("被更新用户 userId 列表，逗号分隔，单次最多 30 (必填)"))
	addDocHiddenStringFlag(cmd, "users", "--user alias")
	addDocHiddenStringFlag(cmd, "uid", "--user alias")
	cmd.Flags().String("role", "", i18n.T("新权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填)"))
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL（选填）"))
	return cmd
}

func newDocPermissionListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("查询文档协作者列表"),
		Long: i18n.T(`查询指定节点的协作者列表，返回每位成员的 userId、姓名、角色等信息。

底层不支持游标分页；--max-results 仅控制单次返回最大条数（默认 30，最大 200）。
若 truncated=true，可通过 --filter-role 收窄查询。`),
		Example: `  dws doc permission list --node DOC_ID
  dws doc permission list --node DOC_ID --max-results 100
  dws doc permission list --node DOC_ID --filter-role MANAGER,EDITOR`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			params := map[string]any{"nodeId": nodeID}
			if v := docIntFlagOrFallback(cmd, "max-results", "limit", "page-size"); v > 0 {
				if v < 1 || v > 200 {
					return apperrors.NewValidation("--max-results/--limit/--page-size must be between 1 and 200")
				}
				params["maxResults"] = v
			}
			if v, _ := cmd.Flags().GetString("filter-role"); v != "" {
				roles := make([]string, 0)
				for _, r := range strings.Split(v, ",") {
					norm, ok := normalizeDocPermissionRole(r)
					if !ok && strings.ToUpper(strings.TrimSpace(r)) != "OWNER" {
						return apperrors.NewValidation(fmt.Sprintf("--filter-role got invalid role: %s", r))
					}
					roles = append(roles, norm)
				}
				params["filterRoleIds"] = roles
			}
			if v, _ := cmd.Flags().GetString("workspace"); v != "" {
				params["workspaceId"] = v
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "list_permission", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "list_permission", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().Int("max-results", 30, i18n.T("返回成员数上限，默认 30，最大 200"))
	cmd.Flags().Int("limit", 0, i18n.T("--max-results 的兼容别名"))
	cmd.Flags().Int("page-size", 0, i18n.T("--max-results 的兼容别名"))
	_ = cmd.Flags().MarkHidden("limit")
	_ = cmd.Flags().MarkHidden("page-size")
	cmd.Flags().String("filter-role", "", i18n.T("按角色过滤（逗号分隔）: OWNER / MANAGER / EDITOR / DOWNLOADER / READER"))
	cmd.Flags().String("workspace", "", i18n.T("目标知识库 ID 或 URL（选填）"))
	return cmd
}

// runDocPermissionMutation 是 add / update 两个命令共用的执行体：
// 校验 → 规范化 → 调对应 MCP tool。
func runDocPermissionMutation(cmd *cobra.Command, runner executor.Runner, mcpTool string) error {
	nodeID, err := docRequiredNode(cmd)
	if err != nil {
		return err
	}
	rawUsers := docFlagOrFallback(cmd, "user", "users", "uid")
	if strings.TrimSpace(rawUsers) == "" {
		return apperrors.NewValidation("--user is required")
	}
	userIDs, err := parseDocPermissionUsers(rawUsers)
	if err != nil {
		return err
	}
	rawRole, _ := cmd.Flags().GetString("role")
	if strings.TrimSpace(rawRole) == "" {
		return apperrors.NewValidation("--role is required")
	}
	role, ok := normalizeDocPermissionRole(rawRole)
	if !ok {
		if role == "OWNER" {
			return apperrors.NewValidation("OWNER role cannot be set via permission add/update")
		}
		return apperrors.NewValidation(fmt.Sprintf("invalid --role: %s (expected MANAGER / EDITOR / DOWNLOADER / READER)", rawRole))
	}
	params := map[string]any{
		"nodeId":  nodeID,
		"roleId":  role,
		"userIds": userIDs,
	}
	if v, _ := cmd.Flags().GetString("workspace"); v != "" {
		params["workspaceId"] = v
	}
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", mcpTool, params,
		))
	}
	result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", mcpTool, params,
	))
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

// TRANSITIONAL: 等 mse 把 delete_document 加入 doc toolOverrides（含
// destructive_hint: true）后，本 helper 可删除——CLI discovery 会自动
// 生成等价命令。工单：plan/mse-yuyuan-patch.md 改动 2.2。
//
// 命名注意：必须挂在 doc 顶层（不在 block group 下），与现有 mse 中
// `dws doc block delete`（删块，调 delete_document_block）做区分。
func newDocDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: i18n.T("删除整篇文档/文件到回收站"),
		Long: i18n.T(`将文档或文件移入回收站（高风险、不可逆操作）。

权限要求：对目标节点有"管理"权限。
执行前需要确认（交互式输入 yes），或传入 --yes 跳过确认。

与 dws doc block delete 的区别：
  - dws doc delete       —— 删除整篇文档/文件（本命令）
  - dws doc block delete —— 删除文档内部的某个块`),
		Example: `  dws doc delete --node DOC_ID --yes
  dws doc delete --node "https://alidocs.dingtalk.com/i/nodes/<UUID>" --yes
  dws doc delete --node DOC_ID    # 交互式确认后删除`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("文档节点"), nodeID) {
				return nil
			}
			params := map[string]any{"nodeId": nodeID}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "delete_document", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "delete_document", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	return cmd
}

// TRANSITIONAL: 等 mse 把 download_doc_attachment 加入 doc toolOverrides
// (cliName: "download", group: "media") 后，本 helper 可删除。
// 工单：plan/mse-yuyuan-patch.md 改动 2.2。
//
// 单 MCP tool 包装，无本地文件 IO（只拿临时下载 URL，由调用方自行 GET）。
func newDocMediaDownloadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: i18n.T("获取文档附件的临时下载链接"),
		Long: i18n.T(`获取钉钉文档中指定附件的 OSS 临时下载链接，返回 downloadUrl 和过期时间。

resource-id 可通过 dws doc block list 获取：查询目标文档的块列表，找到
blockType 为 attachment 的元素，取其 resourceId。

本命令不下载文件到本地，仅返回 URL。如需落盘，调用方自行 GET 该 URL。`),
		Example: `  dws doc media download --node DOC_ID --resource-id RESOURCE_ID
  dws doc media download --node "https://alidocs.dingtalk.com/i/nodes/<UUID>" --resource-id <ID>`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := docRequiredNode(cmd)
			if err != nil {
				return err
			}
			resourceID, _ := cmd.Flags().GetString("resource-id")
			if strings.TrimSpace(resourceID) == "" {
				return apperrors.NewValidation("--resource-id is required")
			}
			params := map[string]any{
				"nodeId":     nodeID,
				"resourceId": resourceID,
			}
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "doc", "download_doc_attachment", params,
				))
			}
			result, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "doc", "download_doc_attachment", params,
			))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("resource-id", "", i18n.T("附件 resourceId，可通过 doc block list 获取 (必填)"))
	return cmd
}

// newDocMediaInsertCommand 把本地文件作为附件上传并插入文档，三步合一：
//  1. get_doc_attachment_upload_info → 获取 uploadUrl + resourceId
//  2. HTTP PUT 文件到 OSS
//  3. insert_document_block → 把附件块挂到文档
//
// 必须 helper 实现：第 2 步 HTTP PUT 是客户端文件 IO，无法用 mse toolOverrides 表达。
func newDocMediaInsertCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "insert",
		Short: i18n.T("上传本地文件并作为附件插入文档（3 步合一：prepare + PUT + insert）"),
		Long: i18n.T(`将本地文件作为附件上传并插入到钉钉文档中（三步自动完成）。

流程：
  1. 获取附件上传凭证 (get_doc_attachment_upload_info)
  2. HTTP PUT 上传文件到 OSS
  3. 插入附件块到文档 (insert_document_block)

图片文件（image/*）小于 20MB 时会作为内联图片插入；其他文件作为附件块插入。
--mime-type 可选，不指定时根据文件扩展名自动推断。`),
		Example: `  # 插入 PDF 附件
  dws doc media insert --node DOC_ID --file ./report.pdf

  # 指定名称和 MIME 类型
  dws doc media insert --node DOC_ID --file ./data.bin --name "数据.dat" --mime-type application/octet-stream

  # 在指定块之前插入
  dws doc media insert --node DOC_ID --file ./image.png --ref-block BLOCK_ID --where before`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocMediaInsert(cmd, runner)
		},
	}
	preferLegacyLeaf(cmd)
	addDocNodeFlags(cmd)
	cmd.Flags().String("file", "", i18n.T("本地文件路径 (必填)"))
	cmd.Flags().String("name", "", i18n.T("附件显示名称（默认使用文件名）"))
	cmd.Flags().String("mime-type", "", i18n.T("文件 MIME 类型（默认根据扩展名推断）"))
	addDocHiddenStringFlag(cmd, "mimetype", "--mime-type alias")
	cmd.Flags().Int("index", 0, i18n.T("插入位置索引"))
	cmd.Flags().String("where", "", i18n.T("相对位置: before / after（配合 --ref-block）"))
	cmd.Flags().String("ref-block", "", i18n.T("参考块 ID（配合 --where）"))
	return cmd
}

const docMaxInlineImageSize = 20 * 1024 * 1024 // 20MB

func runDocMediaInsert(cmd *cobra.Command, runner executor.Runner) error {
	nodeID, err := docRequiredNode(cmd)
	if err != nil {
		return err
	}
	filePath, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(filePath) == "" {
		return apperrors.NewValidation("--file is required")
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return apperrors.NewValidation(i18n.T("无法解析文件路径: ") + err.Error())
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return apperrors.NewValidation(i18n.T("文件不存在: ") + absPath)
	}
	if info.IsDir() {
		return apperrors.NewValidation(i18n.T("不是文件: ") + absPath)
	}
	fileSize := info.Size()
	if fileSize <= 0 {
		return apperrors.NewValidation(i18n.T("文件为空"))
	}
	if fileSize > config.MaxUploadFileSize {
		return apperrors.NewValidation(fmt.Sprintf(i18n.T("文件过大 (%d 字节，限制 %d 字节)"), fileSize, config.MaxUploadFileSize))
	}

	fileName, _ := cmd.Flags().GetString("name")
	if fileName == "" {
		fileName = filepath.Base(absPath)
	} else if filepath.Ext(fileName) == "" {
		if ext := filepath.Ext(absPath); ext != "" {
			fileName += ext
		}
	}

	mimeType, _ := cmd.Flags().GetString("mime-type")
	if mimeType == "" {
		mimeType = detectMIME(fileName)
	}

	// Step 1: 获取上传凭证
	fmt.Fprintf(os.Stderr, i18n.T("步骤 1/3: 获取附件上传凭证 (%s, %d 字节)...\n"), fileName, fileSize)
	step1Params := map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"mimeType": mimeType,
	}
	if commandDryRun(cmd) {
		return writeCommandPayload(cmd, executor.NewHelperInvocation(
			cobracmd.LegacyCommandPath(cmd), "doc", "get_doc_attachment_upload_info", step1Params,
		))
	}
	credResult, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "get_doc_attachment_upload_info", step1Params,
	))
	if err != nil {
		return fmt.Errorf(i18n.T("获取上传凭证失败: %w"), err)
	}

	uploadURL, resourceID, resourceURL, err := extractDocAttachmentUploadInfo(credResult.Response)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT
	fmt.Fprintln(os.Stderr, i18n.T("步骤 2/3: 上传文件到 OSS..."))
	f, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf(i18n.T("无法打开文件: %w"), err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPut, uploadURL, f)
	if err != nil {
		return fmt.Errorf(i18n.T("构建上传请求失败: %w"), err)
	}
	req.ContentLength = fileSize
	req.Header.Set("Content-Type", mimeType)

	httpClient := &http.Client{Timeout: 5 * time.Minute}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf(i18n.T("上传失败: %w"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf(i18n.T("OSS 上传失败 HTTP %d: %s"), resp.StatusCode, string(body))
	}

	// Step 3: 插入块到文档
	fmt.Fprintln(os.Stderr, i18n.T("步骤 3/3: 插入块到文档..."))
	element := buildDocAttachmentElement(mimeType, fileName, resourceID, resourceURL, fileSize)
	insertArgs := map[string]any{
		"nodeId":  nodeID,
		"element": element,
	}
	if cmd.Flags().Changed("index") {
		if v, _ := cmd.Flags().GetInt("index"); v >= 0 {
			insertArgs["index"] = v
		}
	}
	if v, _ := cmd.Flags().GetString("where"); v != "" {
		insertArgs["where"] = v
	}
	if v, _ := cmd.Flags().GetString("ref-block"); v != "" {
		insertArgs["referenceBlockId"] = v
	}
	insertResult, err := runner.Run(cmd.Context(), executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd), "doc", "insert_document_block", insertArgs,
	))
	if err != nil {
		return fmt.Errorf(i18n.T("插入块失败: %w"), err)
	}
	return writeCommandPayload(cmd, insertResult)
}

// extractDocAttachmentUploadInfo 从 get_doc_attachment_upload_info 的返回中
// 抽出 uploadUrl / resourceId / resourceUrl 三项。返回结构兼容 content.data
// 和 data 两种包装层次（开源 runner 与 wukong 实测均见过）。
func extractDocAttachmentUploadInfo(resp map[string]any) (uploadURL, resourceID, resourceURL string, err error) {
	if resp == nil {
		err = apperrors.NewValidation(i18n.T("get_doc_attachment_upload_info 返回空"))
		return
	}
	src := resp
	if content, ok := src["content"].(map[string]any); ok && len(content) > 0 {
		src = content
	}
	data, _ := src["data"].(map[string]any)
	if data == nil {
		data = src
	}
	uploadURL, _ = data["uploadUrl"].(string)
	resourceID, _ = data["resourceId"].(string)
	resourceURL, _ = data["resourceUrl"].(string)
	if uploadURL == "" || resourceID == "" {
		err = apperrors.NewValidation(i18n.T("返回数据缺少 uploadUrl 或 resourceId"))
		return
	}
	return
}

// buildDocAttachmentElement 按文件类型生成 insert_document_block 需要的 element 结构。
// 图片 ≤ 20MB 走内联图片，否则走附件块。
func buildDocAttachmentElement(mimeType, fileName, resourceID, resourceURL string, fileSize int64) map[string]any {
	if strings.HasPrefix(mimeType, "image/") && resourceURL != "" && fileSize <= docMaxInlineImageSize {
		return map[string]any{
			"blockType": "paragraph",
			"paragraph": map[string]any{"text": ""},
			"children": []any{
				map[string]any{
					"elementType": "image",
					"properties":  map[string]any{"src": resourceURL},
				},
			},
		}
	}
	viewType := "preview"
	if mimeType == "text/markdown" {
		viewType = "summary"
	}
	return map[string]any{
		"blockType": "attachment",
		"attachment": map[string]any{
			"resourceId": resourceID,
			"type":       mimeType,
			"name":       fileName,
			"viewType":   viewType,
		},
	}
}
