package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws drive — 钉盘
// MCP tools: list_files, get_file_info, download_file, create_folder,
//            get_upload_info, commit_upload
// ──────────────────────────────────────────────────────────

func runDriveUpload(cmd *cobra.Command, _ []string) error {
	filePath := mustGetFlag(cmd, "file")
	if filePath == "" {
		return fmt.Errorf("flag --file is required")
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory, not a file", filePath)
	}

	fileName := flagOrFallback(cmd, "file-name", "name")
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	fileSize := fi.Size()

	// 路由判断：--workspace 存在时走文档空间上传流程
	workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
	if workspaceID != "" {
		return runDriveUploadToDocSpace(cmd, filePath, fileName, fileSize, workspaceID)
	}

	parentID := flagOrFallback(cmd, "folder", "parent-id")
	if err := validateDriveParentID(parentID); err != nil {
		return err
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "上传文件到钉盘")
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", fileName)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: get upload credentials
	step1Args := map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
	}
	if v, _ := cmd.Flags().GetString("space-id"); v != "" {
		step1Args["spaceId"] = v
	}
	if v, _ := cmd.Flags().GetString("mime-type"); v != "" {
		step1Args["mimeType"] = v
	}
	if parentID != "" {
		step1Args["parentId"] = parentID
	}

	text, err := callMCPToolReturnText(ctx, "get_upload_info", step1Args)
	if err != nil {
		return err
	}

	resourceURL, uploadID, ossHeaders, err := parseDriveUploadInfo(text)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT to OSS
	if err := httpPutFile(ctx, resourceURL, ossHeaders, filePath, fileSize); err != nil {
		return err
	}

	// Step 3: commit
	commitArgs := map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"uploadId": uploadID,
	}
	if v, _ := cmd.Flags().GetString("space-id"); v != "" {
		commitArgs["spaceId"] = v
	}
	if parentID != "" {
		commitArgs["parentId"] = parentID
	}

	return callMCPTool("commit_upload", commitArgs)
}

// runDriveUploadToDocSpace 处理文档空间上传流程（当 --workspace 存在时路由到此）。
// 使用 doc MCP server 的 get_file_upload_info + commit_uploaded_file 工具。
func runDriveUploadToDocSpace(cmd *cobra.Command, filePath, fileName string, fileSize int64, workspaceID string) error {
	folder := docFolderFlag(cmd)
	if err := validateDocFolderID(folder); err != nil {
		return err
	}

	// 补全文件名后缀
	if filepath.Ext(fileName) == "" {
		if ext := filepath.Ext(filePath); ext != "" {
			fileName += ext
		}
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "上传文件到文档空间")
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", fileName)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		deps.Out.PrintKeyValue("知识库", workspaceID)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: get upload credentials (doc MCP server)
	step1Args := map[string]any{
		"workspaceId": workspaceID,
	}
	if folder != "" {
		step1Args["folderId"] = folder
	}

	text, err := callMCPToolReturnTextOnServer(ctx, "doc", "get_file_upload_info", step1Args)
	if err != nil {
		return err
	}

	resourceURL, uploadKey, ossHeaders, err := parseUploadInfo(text)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT to OSS
	if err := httpPutFile(ctx, resourceURL, ossHeaders, filePath, fileSize); err != nil {
		return err
	}

	// Step 3: commit (doc MCP server)
	commitArgs := map[string]any{
		"uploadKey":   uploadKey,
		"name":        fileName,
		"fileSize":    float64(fileSize),
		"workspaceId": workspaceID,
	}
	if folder != "" {
		commitArgs["folderId"] = folder
	}
	if convert, _ := cmd.Flags().GetBool("convert"); convert {
		commitArgs["convertToOnlineDoc"] = true
	}

	return callMCPToolOnServer("doc", "commit_uploaded_file", commitArgs)
}

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
	return fmt.Errorf("invalid drive --folder %q: pure numeric IDs are usually dentryId values for chat --dentry-id, not drive parent dentryUuid values; use a parent folder dentryUuid from drive list or omit --folder to use the space root", parentID)
}

// parseDriveUploadInfo extracts the upload URL, uploadId and headers from the
// drive MCP tool response. The actual response format is:
//
//	{
//	  "uploadId": "...",
//	  "resourceUrls": [
//	    { "url": "https://...", "headers": { ... } }
//	  ]
//	}
func parseDriveUploadInfo(text string) (resourceURL, uploadID string, headers map[string]string, err error) {
	var data map[string]any
	if err = json.Unmarshal([]byte(text), &data); err != nil {
		err = fmt.Errorf("failed to parse drive upload credentials JSON: %w", err)
		return
	}

	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	uploadID, _ = data["uploadId"].(string)

	// Extract URL from resourceUrls array (primary format)
	if urls, ok := data["resourceUrls"].([]any); ok && len(urls) > 0 {
		if first, ok := urls[0].(map[string]any); ok {
			resourceURL, _ = first["url"].(string)
			// Extract per-URL headers
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

	// Fallback: try flat resourceUrl / uploadUrl fields
	if resourceURL == "" {
		resourceURL, _ = data["resourceUrl"].(string)
	}
	if resourceURL == "" {
		resourceURL, _ = data["uploadUrl"].(string)
	}

	if resourceURL == "" || uploadID == "" {
		err = fmt.Errorf("incomplete drive upload credentials: resourceUrl=%q, uploadId=%q", resourceURL, uploadID)
		return
	}

	// Fallback: top-level headers (if per-URL headers were empty)
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

func newDriveCommand() *cobra.Command {
	driveCmd := &cobra.Command{
		Use:   "drive",
		Short: "钉盘文件管理",
		Long:  `钉盘：列出文件/文件夹、获取元数据和统计信息、创建快捷方式、下载、上传及管理文件。`,
		RunE:  groupRunE,
	}

	driveListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取文件/文件夹列表（统一入口）",
		Long: `列出文件和文件夹。根据参数自动路由到钉盘或文档空间。

路由规则:
  默认（无 --workspace）       → 列出钉盘「我的文件」
  --space-id <纯数字>          → 列出钉盘指定空间
  --workspace <加密string/URL> → 列出文档空间/知识库文件（等同于原 list-docs）
  --folder <nodeId>            → 列出指定文件夹下的子节点`,
		Example: `  dws drive list --limit 20
  dws drive list --folder <dentryUuid> --order-by name --order asc
  dws drive list --workspace <workspaceId>
  dws drive list --workspace <workspaceId> --folder <folderId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 如果指定了 --workspace，路由到文档空间（doc MCP server）
			workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
			if workspaceID != "" {
				toolArgs := map[string]any{"workspaceId": workspaceID}
				if folder := docFolderFlag(cmd, "node", "file-id"); folder != "" {
					if err := validateDocFolderID(folder); err != nil {
						return err
					}
					toolArgs["folderId"] = folder
				}
				if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
					toolArgs["pageSize"] = v
				}
				if v := flagOrFallback(cmd, "cursor", "next-token", "page-token"); v != "" {
					toolArgs["pageToken"] = v
				}
				return callMCPToolOnServer("doc", "list_nodes", toolArgs)
			}

			// 默认路由：钉盘文件列表
			maxResults, _ := cmd.Flags().GetInt("limit")
			if !cmd.Flags().Changed("limit") {
				if v, _ := cmd.Flags().GetInt("max"); v > 0 {
					maxResults = v
				}
			}
			if maxResults <= 0 {
				maxResults = 20
			}
			if maxResults > 50 {
				maxResults = 50
			}
			argsMap := map[string]any{"maxResults": float64(maxResults)}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				argsMap["spaceId"] = v
			}
			if parentID := flagOrFallback(cmd, "folder", "parent-id"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				argsMap["parentId"] = parentID
			}
			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				argsMap["nextToken"] = v
			}
			if v, _ := cmd.Flags().GetString("order-by"); v != "" {
				argsMap["orderBy"] = v
			}
			if v, _ := cmd.Flags().GetString("order"); v != "" {
				argsMap["order"] = v
			}
			if v, _ := cmd.Flags().GetBool("thumbnail"); v {
				argsMap["withThumbnail"] = true
			}
			return callMCPTool("list_files", argsMap)
		},
	}

	driveInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "获取文件元数据信息",
		Long: `获取钉盘文件/文件夹的元数据信息。

如果目标文件属于钉钉文档（在线文档/表格/脑图等），会自动跟进调用
钉钉文档接口获取更准确的文档信息（如真实文档名称），并合并输出。`,
		Example: `  dws drive info --node <dentryUuid>  # 查询 fileId: dws drive list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID := flagOrFallback(cmd, "node", "file-id")
			if fileID == "" {
				return fmt.Errorf("flag --node is required")
			}
			argsMap := map[string]any{"fileId": fileID}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				argsMap["spaceId"] = v
			}
			return driveInfoWithDocFallback(fileID, argsMap)
		},
	}

	driveDownloadCmd := &cobra.Command{
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
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID := flagOrFallback(cmd, "node", "file-id")
			if fileID == "" {
				return fmt.Errorf("flag --node is required")
			}
			outputPath, _ := cmd.Flags().GetString("output")
			if outputPath == "" {
				return fmt.Errorf("flag --output is required")
			}

			argsMap := map[string]any{"fileId": fileID}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				argsMap["spaceId"] = v
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "下载钉盘文件")
				deps.Out.PrintKeyValue("文件ID", fileID)
				deps.Out.PrintKeyValue("输出", outputPath)
				return nil
			}

			ctx := context.Background()

			// Step 1: 获取下载 URL 和签名请求头
			deps.Out.PrintInfo("[1/2] 获取下载链接...")
			text, err := callMCPToolReturnText(ctx, "download_file", argsMap)
			if err != nil {
				return err
			}

			resourceURL, dlHeaders, err := parseDownloadInfo(text)
			if err != nil {
				return err
			}

			// 如果 output 是目录，优先从 MCP 返回的 fileName，fallback 到从 URL 推断
			fi, statErr := os.Stat(outputPath)
			if statErr == nil && fi.IsDir() {
				filename := extractFileNameFromResponse(text)
				if filename == "" {
					filename = inferFilename(resourceURL)
				}
				outputPath = filepath.Join(outputPath, filename)
			}

			// Step 2: HTTP GET 下载文件
			deps.Out.PrintInfo(fmt.Sprintf("[2/2] 下载文件到 %s ...", outputPath))
			if err := httpGetFile(ctx, resourceURL, dlHeaders, outputPath); err != nil {
				return err
			}

			deps.Out.PrintInfo(fmt.Sprintf("下载完成: %s", outputPath))
			return nil
		},
	}

	driveMkdirCmd := &cobra.Command{
		Use:   "mkdir",
		Short: "创建文件夹",
		Example: `  dws drive mkdir --name "项目资料"
  dws drive mkdir --name "子目录" --folder <dentryUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			argsMap := map[string]any{"name": mustGetFlag(cmd, "name")}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				argsMap["spaceId"] = v
			}
			if parentID := flagOrFallback(cmd, "folder", "parent-id"); parentID != "" {
				if err := validateDriveParentID(parentID); err != nil {
					return err
				}
				argsMap["parentId"] = parentID
			}
			return callMCPTool("create_folder", argsMap)
		},
	}

	driveUploadInfoCmd := &cobra.Command{
		Use:   "upload-info",
		Short: "获取文件上传信息",
		Example: `  dws drive upload-info --file-name "报告.pdf" --file-size 102400
  dws drive upload-info --file-name "readme.txt" --file-size 1024 --folder <dentryUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "file-name"); err != nil {
				return err
			}
			fileSize, _ := cmd.Flags().GetInt64("file-size")
			if fileSize <= 0 {
				return fmt.Errorf("flag --file-size is required and must be a positive integer")
			}
			argsMap := map[string]any{
				"fileName": mustGetFlag(cmd, "file-name"),
				"fileSize": float64(fileSize),
			}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				argsMap["spaceId"] = v
			}
			if v, _ := cmd.Flags().GetString("mime-type"); v != "" {
				argsMap["mimeType"] = v
			}
			if v := flagOrFallback(cmd, "folder", "parent-id"); v != "" {
				if err := validateDriveParentID(v); err != nil {
					return err
				}
				argsMap["parentId"] = v
			}
			return callMCPTool("get_upload_info", argsMap)
		},
	}

	driveCommitCmd := &cobra.Command{
		Use:     "commit",
		Short:   "提交文件上传",
		Example: `  dws drive commit --file-name "报告.pdf" --file-size 102400 --upload-id <uploadId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "file-name", "upload-id"); err != nil {
				return err
			}
			fileSize, _ := cmd.Flags().GetInt64("file-size")
			if fileSize <= 0 {
				return fmt.Errorf("flag --file-size is required and must be a positive integer")
			}
			argsMap := map[string]any{
				"fileName": mustGetFlag(cmd, "file-name"),
				"fileSize": float64(fileSize),
				"uploadId": mustGetFlag(cmd, "upload-id"),
			}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				argsMap["spaceId"] = v
			}
			if v := flagOrFallback(cmd, "folder", "parent-id"); v != "" {
				if err := validateDriveParentID(v); err != nil {
					return err
				}
				argsMap["parentId"] = v
			}
			return callMCPTool("commit_upload", argsMap)
		},
	}

	driveListCmd.Flags().Int("limit", 20, "每页返回数量，默认 20，最大 50")
	driveListCmd.Flags().Int("max", 0, "--limit 的别名（向后兼容）")
	_ = driveListCmd.Flags().MarkHidden("max")
	driveListCmd.Flags().String("space-id", "", "钉盘空间 ID (纯数字)，不传则使用「我的文件」(可选)")
	driveListCmd.Flags().String("workspace", "", "文档空间/知识库 ID (加密 string 或 URL)，传入则路由到文档空间 (可选)")
	driveListCmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)")
	driveListCmd.Flags().String("cursor", "", "分页游标，首次不传 (可选)")
	driveListCmd.Flags().String("order-by", "", "排序字段: createTime|modifyTime|name (可选，仅钉盘)")
	driveListCmd.Flags().String("order", "", "排序方向: asc|desc，默认 desc (可选，仅钉盘)")
	driveListCmd.Flags().Bool("thumbnail", false, "是否返回缩略图信息 (可选，仅钉盘)")

	driveInfoCmd.Flags().String("node", "", "节点 ID (dentryUuid) (必填)")
	driveInfoCmd.Flags().String("space-id", "", "节点所属空间 ID (可选)")

	driveDownloadCmd.Flags().String("node", "", "文件 ID (dentryUuid) (必填)")
	driveDownloadCmd.Flags().String("space-id", "", "文件所属空间 ID (可选)")
	driveDownloadCmd.Flags().String("output", "", "本地保存路径 (文件路径或目录，必填)")

	driveMkdirCmd.Flags().String("name", "", "文件夹名称，最长 50 字符 (必填)")
	driveMkdirCmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」 (可选)")
	driveMkdirCmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则在空间根目录下创建 (可选)")

	driveUploadInfoCmd.Flags().String("file-name", "", "文件名，须包含扩展名，如 报告.pdf (必填)")
	driveUploadInfoCmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	_ = driveUploadInfoCmd.MarkFlagRequired("file-size")
	driveUploadInfoCmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」 (可选)")
	driveUploadInfoCmd.Flags().String("mime-type", "", "文件 MIME 类型，如 application/pdf，不传则自动推断 (可选)")
	driveUploadInfoCmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)")

	driveCommitCmd.Flags().String("file-name", "", "文件名（含扩展名），须与 get_upload_info 时一致 (必填)")
	driveCommitCmd.Flags().Int64("file-size", 0, "文件大小（字节），须与 get_upload_info 时一致 (必填)")
	_ = driveCommitCmd.MarkFlagRequired("file-size")
	driveCommitCmd.Flags().String("upload-id", "", "上传 ID，来自 get_upload_info 返回的 uploadId (必填)")
	driveCommitCmd.Flags().String("space-id", "", "空间 ID，不传则使用「我的文件」 (可选)")
	driveCommitCmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则提交到根目录 (可选)")

	driveUploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "上传本地文件到钉盘或文档空间",
		Long: `将本地文件上传（三步自动完成）。

路由规则:
  默认（无 --workspace）  → 上传到钉盘（我的文件或指定空间）
  --workspace <id>        → 上传到文档空间/知识库

流程:
  1. 获取 OSS 上传凭证
  2. HTTP PUT 上传文件二进制到 OSS
  3. 提交文件入库

上传位置: --folder 指定父目录，不传则上传到空间根目录。`,
		Example: `  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --folder <dentryUuid>
  dws drive upload --file ./doc.pdf --workspace <workspaceId>
  dws drive upload --file ./data.xlsx --workspace <workspaceId> --convert`,
		RunE: runDriveUpload,
	}
	driveUploadCmd.Flags().String("file", "", "本地文件路径 (必填)")
	driveUploadCmd.Flags().String("file-name", "", "文件显示名称 (默认使用文件名)")
	driveUploadCmd.Flags().String("space-id", "", "目标钉盘空间 ID，不传则使用「我的文件」 (可选)")
	driveUploadCmd.Flags().String("mime-type", "", "文件 MIME 类型，不传则自动推断 (可选)")
	driveUploadCmd.Flags().String("folder", "", "父节点 ID，不传则上传到空间根目录 (可选)")
	driveUploadCmd.Flags().String("workspace", "", "目标知识库 ID，传入时路由到文档空间上传 (可选)")
	driveUploadCmd.Flags().Bool("convert", false, "是否转换为钉钉在线文档 (仅文档空间上传时生效)")

	driveListSpacesCmd := &cobra.Command{
		Use:   "list-spaces",
		Short: "获取钉盘空间列表 (deprecated → dws wiki space list --type orgSpace/mySpace)",
		Long: `⚠️  此命令已迁移到 wiki space list，请使用:
  dws wiki space list --type orgSpace    # 企业空间
  dws wiki space list --type mySpace     # 我的文件

列出当前用户可访问的钉盘空间，返回 spaceId、spaceName、rootFolderId 等信息。`,
		Example: `  dws wiki space list --type orgSpace     # 推荐
  dws wiki space list --type mySpace      # 推荐
  dws drive list-spaces                   # deprecated`,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps.Out.PrintWarning("⚠️  'dws drive list-spaces' is deprecated, use 'dws wiki space list --type orgSpace' or 'dws wiki space list --type mySpace' instead.")
			maxResults, _ := cmd.Flags().GetInt("limit")
			if !cmd.Flags().Changed("limit") {
				if v, _ := cmd.Flags().GetInt("max"); v > 0 {
					maxResults = v
				}
			}
			argsMap := map[string]any{}
			if maxResults > 0 {
				argsMap["maxResults"] = float64(maxResults)
			}
			if v, _ := cmd.Flags().GetString("space-type"); v != "" {
				argsMap["spaceType"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				argsMap["nextToken"] = v
			}
			return callMCPTool("list_spaces", argsMap)
		},
	}
	driveListSpacesCmd.Flags().Int("limit", 20, "每页返回数量 (默认 20，最大 50)，仅 spaceType 为 orgSpace 时有效")
	driveListSpacesCmd.Flags().Int("max", 0, "--limit 的别名（向后兼容）")
	_ = driveListSpacesCmd.Flags().MarkHidden("max")
	driveListSpacesCmd.Flags().String("space-type", "", "空间类型: orgSpace=企业空间(默认), mySpace=我的文件 (可选)")
	driveListSpacesCmd.Flags().String("cursor", "", "分页游标，仅企业空间支持分页 (可选)")

	driveSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索文件（聚合钉盘+文档空间）",
		Long: `全局搜索文件，默认同时搜索钉盘和文档空间，合并返回结果。

搜索范围 (--target):
  all   （默认）同时搜钉盘文件与文档空间，聚合返回
  file  只搜钉盘文件/文件夹，支持 --file-types / --extensions
  space 只搜钉盘团队空间

如果需要在某个知识库内搜索，请使用 dws wiki node search --workspace <workspaceId>。

结果中 source 字段区分来源：drive / doc。
提示：结果按相关性排序，首页未命中时优先调整关键词 / 过滤条件，而非反复翻页。`,
		Example: `  dws drive search --query "季度汇报"
  dws drive search --query "合同" --target file --extensions pdf,docx
  dws drive search --query "项目" --target space
  dws drive search --query "报告" --limit 30 --cursor <pageToken>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := flagOrFallback(cmd, "query", "keyword")
			if keyword == "" {
				return fmt.Errorf("flag --query is required")
			}

			target, _ := cmd.Flags().GetString("target")

			// 构建钉盘搜索参数
			argsMap := map[string]any{"keyword": keyword}
			if target != "" && target != "all" {
				argsMap["searchTarget"] = target
			}
			if v, _ := cmd.Flags().GetStringSlice("file-types"); len(v) > 0 {
				argsMap["fileTypes"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("extensions"); len(v) > 0 {
				argsMap["extensions"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("creator-uids"); len(v) > 0 {
				argsMap["creatorUserIds"] = v
			}
			if cmd.Flags().Changed("created-from") {
				if v, _ := cmd.Flags().GetInt64("created-from"); v > 0 {
					argsMap["createdTimeFrom"] = v
				}
			}
			if cmd.Flags().Changed("created-to") {
				if v, _ := cmd.Flags().GetInt64("created-to"); v > 0 {
					argsMap["createdTimeTo"] = v
				}
			}
			if cmd.Flags().Changed("modified-from") {
				if v, _ := cmd.Flags().GetInt64("modified-from"); v > 0 {
					argsMap["modifiedTimeFrom"] = v
				}
			}
			if cmd.Flags().Changed("modified-to") {
				if v, _ := cmd.Flags().GetInt64("modified-to"); v > 0 {
					argsMap["modifiedTimeTo"] = v
				}
			}
			pageSize, _ := cmd.Flags().GetInt("limit")
			if !cmd.Flags().Changed("limit") {
				if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
					pageSize = v
				}
			}
			if pageSize > 0 {
				argsMap["pageSize"] = float64(pageSize)
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				argsMap["pageToken"] = v
			}

			// --target file/space: 仅搜钉盘
			if target == "file" || target == "space" {
				return callMCPTool("search_files", argsMap)
			}

			// --target all (默认): 聚合搜索钉盘+文档空间
			ctx := context.Background()

			// 1) 钉盘搜索
			driveText, driveErr := callMCPToolReturnText(ctx, "search_files", argsMap)

			// 2) 文档空间搜索
			docArgs := map[string]any{"keyword": keyword}
			if pageSize > 0 {
				docArgs["pageSize"] = pageSize
			}
			docText, docErr := callMCPToolReturnTextOnServer(ctx, "doc", "search_documents", docArgs)

			// 合并输出
			// 双路全失败时返回 error；仅一路失败时静默忽略，只输出成功方结果
			if driveErr != nil && docErr != nil {
				return fmt.Errorf("aggregated search failed: drive: %v; doc: %v", driveErr, docErr)
			}

			result := map[string]any{}
			if driveErr == nil && driveText != "" {
				var driveResult any
				if json.Unmarshal([]byte(driveText), &driveResult) == nil {
					result["drive_results"] = driveResult
				} else {
					result["drive_results"] = driveText
				}
			}

			if docErr == nil && docText != "" {
				var docResult any
				if json.Unmarshal([]byte(docText), &docResult) == nil {
					result["doc_results"] = docResult
				} else {
					result["doc_results"] = docText
				}
			}

			merged, _ := json.MarshalIndent(result, "", "  ")
			deps.Out.PrintRaw(string(merged))
			return nil
		},
	}
	driveSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	driveSearchCmd.Flags().String("keyword", "", "--query 的别名（向后兼容）")
	_ = driveSearchCmd.Flags().MarkHidden("keyword")
	driveSearchCmd.Flags().String("target", "", "搜索范围: all(默认,聚合钉盘+文档空间) | file(仅钉盘文件) | space(仅钉盘空间) (可选)")
	driveSearchCmd.Flags().StringSlice("file-types", nil, "按文件内容类型过滤，逗号分隔: alidoc,document,image,video,audio,archive (仅 target=file/all 生效)")
	driveSearchCmd.Flags().StringSlice("extensions", nil, "按文件扩展名过滤，不含点号，逗号分隔 (如 pdf,docx,adoc)")
	driveSearchCmd.Flags().StringSlice("creator-uids", nil, "按创建者用户 ID 过滤，逗号分隔")
	driveSearchCmd.Flags().Int64("created-from", 0, "创建时间起始 (毫秒时间戳，含)")
	driveSearchCmd.Flags().Int64("created-to", 0, "创建时间截止 (毫秒时间戳，含)")
	driveSearchCmd.Flags().Int64("modified-from", 0, "修改时间起始 (毫秒时间戳，含)")
	driveSearchCmd.Flags().Int64("modified-to", 0, "修改时间截止 (毫秒时间戳，含)")
	driveSearchCmd.Flags().Int("limit", 0, "每页返回数量（默认 10，最大 30）")
	driveSearchCmd.Flags().Int("page-size", 0, "--limit 的别名（向后兼容）")
	_ = driveSearchCmd.Flags().MarkHidden("page-size")
	driveSearchCmd.Flags().String("cursor", "", "分页游标，从上次返回的 nextCursor 获取 (可选)")
	driveSearchCmd.Flags().String("page-token", "", "--cursor 的别名（向后兼容）")
	_ = driveSearchCmd.Flags().MarkHidden("page-token")

	driveDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除文件/文件夹到回收站",
		Long: `将钉盘中的文件或文件夹移入回收站。

注意: 这是一个危险操作，文件将被移入回收站。执行前需要确认，或传入 --yes 跳过确认。
--node 对应 drive list 返回的 fileId 字段（即 dentryUuid）。

权限要求: 对文档有"管理"权限。`,
		Example: `  dws drive delete --node <dentryUuid> --yes    # 查询 fileId: dws drive list
  dws drive delete --node <dentryUuid>           # 交互式确认后删除`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID := flagOrFallback(cmd, "node", "file-id")
			if fileID == "" {
				return fmt.Errorf("flag --node is required")
			}
			if !confirmDelete("钉盘节点", fileID) {
				return nil
			}
			// 同 dws doc delete：delete_document 工具仅注册在 doc MCP server 上，
			// 钉盘节点（fileId）与文档节点共用同一套 dentryUuid 体系，因此显式
			// 路由到 doc server 才能找到该工具。若使用 callMCPTool 让 resolveProductID
			// 路由，会被路由到 drive server，服务端会返回 PARAM_ERROR - 未找到指定工具。
			return callMCPToolOnServer("doc", "delete_document", map[string]any{
				"nodeId": fileID,
			})
		},
	}
	driveDeleteCmd.Flags().String("node", "", "文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)")

	// ── 文档空间代理命令（从 doc 迁入，显式路由到 doc MCP server）──

	driveCopyCmd := &cobra.Command{
		Use:   "copy",
		Short: "复制文件/文档到指定位置",
		Long: `将文档空间中的文档或文件复制到指定文件夹或知识库。
--folder 指定目标文件夹 nodeId，--workspace 指定目标知识库 ID。
不传 --folder 时复制到 --workspace 根目录；都不传则默认到"我的文档"。

权限要求: 对源文档有"阅读"权限，且对目标文件夹有"编辑"权限。`,
		Example: `  dws drive copy --node DOC_ID --folder TARGET_FOLDER_ID
  dws drive copy --node DOC_ID --workspace TARGET_WS_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if v := docFolderFlag(cmd); v != "" {
				if err := validateDocFolderID(v); err != nil {
					return err
				}
				toolArgs["targetFolderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "copy_document", toolArgs)
		},
	}
	driveCopyCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	driveCopyCmd.Flags().String("folder", "", "目标文件夹 nodeId")
	driveCopyCmd.Flags().String("workspace", "", "目标知识库 ID")

	driveMoveCmd := &cobra.Command{
		Use:   "move",
		Short: "移动文件/文档到指定位置",
		Long: `将文档空间中的文档或文件移动到指定文件夹或知识库。移动后原位置不再存在。
--folder 指定目标文件夹 nodeId，--workspace 指定目标知识库 ID。

权限要求: 对源文档有"管理"权限，且对目标文件夹有"编辑"权限。`,
		Example: `  dws drive move --node DOC_ID --folder TARGET_FOLDER_ID
  dws drive move --node DOC_ID --workspace TARGET_WS_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if v := docFolderFlag(cmd); v != "" {
				if err := validateDocFolderID(v); err != nil {
					return err
				}
				toolArgs["targetFolderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "move_document", toolArgs)
		},
	}
	driveMoveCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	driveMoveCmd.Flags().String("folder", "", "目标文件夹 nodeId")
	driveMoveCmd.Flags().String("workspace", "", "目标知识库 ID")

	driveRenameCmd := &cobra.Command{
		Use:   "rename",
		Short: "重命名文件/文档",
		Long: `修改文档空间中文档或文件的名称。

权限要求: 对文档有"编辑"权限。`,
		Example: `  dws drive rename --node DOC_ID --name "新名称"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "title"); err != nil {
				return err
			}
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("doc", "rename_document", map[string]any{
				"nodeId":  nodeID,
				"newName": flagOrFallback(cmd, "name", "title"),
			})
		},
	}
	driveRenameCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	driveRenameCmd.Flags().String("name", "", "新名称 (必填)")

	driveStatsCmd := &cobra.Command{
		Use:   "stats",
		Short: "获取节点统计信息",
		Long: `获取指定节点的统计数据，包括阅读人数、阅读次数、编辑次数、评论数、点赞数、预览次数和下载次数等。

不同文件类型返回的统计维度可能不同。--node 支持节点 ID 或文档 URL。`,
		Example: `  dws drive stats --node <dentryUuid>
  dws drive stats --node https://alidocs.dingtalk.com/i/nodes/<dentryUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "get_node_stats", map[string]any{"nodeId": nodeID})
		},
	}
	driveStatsCmd.Flags().String("node", "", "节点 ID 或文档 URL (必填)")

	driveShortcutCmd := &cobra.Command{
		Use:   "shortcut",
		Short: "为节点创建快捷方式",
		Long: `为指定源节点创建快捷方式，并放置到目标文件夹或知识库。

通过 --node 指定源节点。--folder 和 --workspace 均为可选；都不传时由服务端选择默认位置。
若同时指定，--folder 是目标文件夹，--workspace 用于指定其所属知识库。`,
		Example: `  dws drive shortcut --node <dentryUuid>
  dws drive shortcut --node <dentryUuid> --folder <targetFolderId>
  dws drive shortcut --node <dentryUuid> --workspace <workspaceId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if v := docFolderFlag(cmd); v != "" {
				if err := validateDocFolderID(v); err != nil {
					return err
				}
				toolArgs["targetFolderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("drive", "create_shortcut", toolArgs)
		},
	}
	driveShortcutCmd.Flags().String("node", "", "源节点 ID 或文档 URL (必填)")
	driveShortcutCmd.Flags().String("folder", "", "目标文件夹 nodeId (可选)")
	driveShortcutCmd.Flags().String("workspace", "", "目标知识库 ID (可选)")

	// ── drive permission (文档节点权限管理) ──
	drivePermissionCmd := &cobra.Command{
		Use:     "permission",
		Aliases: []string{"perm"},
		Short:   "文档节点权限管理",
		Long: `管理文档空间节点的协作权限：添加、更新、查询、移除协作者。
注意: 仅适用于文档空间节点，不适用于钉盘文件。`,
		RunE: groupRunE,
	}

	drivePermAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加协作者",
		Long: `为文档空间节点添加协作成员并授予指定角色。

支持的角色 (--role): MANAGER / EDITOR / DOWNLOADER / READER`,
		Example: `  dws drive permission add --node DOC_ID --users uid1 --role READER
  dws drive permission add --node DOC_ID --users uid1,uid2 --role EDITOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "role"); err != nil {
				return err
			}
			userIds, err := collectUserIDs(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"roleId":  normalizePermissionRole(mustGetFlag(cmd, "role")),
				"userIds": userIds,
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "add_permission", toolArgs)
		},
	}
	drivePermAddCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermAddCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	drivePermAddCmd.Flags().String("user", "", "")
	_ = drivePermAddCmd.Flags().MarkHidden("user")
	drivePermAddCmd.Flags().String("role", "", "角色: MANAGER / EDITOR / DOWNLOADER / READER (必填)")
	drivePermAddCmd.Flags().String("workspace", "", "知识库 ID (选填)")

	drivePermUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新协作者权限",
		Long: `更新文档空间节点已有协作者的权限角色。

支持的角色 (--role): MANAGER / EDITOR / DOWNLOADER / READER`,
		Example: `  dws drive permission update --node DOC_ID --users uid1 --role EDITOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "role"); err != nil {
				return err
			}
			userIds, err := collectUserIDs(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"roleId":  normalizePermissionRole(mustGetFlag(cmd, "role")),
				"userIds": userIds,
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "update_permission", toolArgs)
		},
	}
	drivePermUpdateCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermUpdateCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	drivePermUpdateCmd.Flags().String("user", "", "")
	_ = drivePermUpdateCmd.Flags().MarkHidden("user")
	drivePermUpdateCmd.Flags().String("role", "", "新角色: MANAGER / EDITOR / DOWNLOADER / READER (必填)")
	drivePermUpdateCmd.Flags().String("workspace", "", "知识库 ID (选填)")

	drivePermListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查询协作者列表",
		Long:    `查询文档空间节点的协作者列表。`,
		Example: `  dws drive permission list --node DOC_ID
  dws drive permission list --node DOC_ID --limit 100 --filter-role MANAGER,EDITOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			limit := 0
			if cmd.Flags().Changed("limit") {
				limit, _ = cmd.Flags().GetInt("limit")
			}
			if limit > 0 {
				toolArgs["maxResults"] = limit
			}
			if v := mustGetFlag(cmd, "filter-role"); v != "" {
				toolArgs["filterRoleIds"] = parseRoleList(v)
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "list_permission", toolArgs)
		},
	}
	drivePermListCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermListCmd.Flags().Int("limit", 30, "返回成员数上限，默认 30，最大 200")
	drivePermListCmd.Flags().String("filter-role", "", "按角色过滤: OWNER / MANAGER / EDITOR / DOWNLOADER / READER")
	drivePermListCmd.Flags().String("workspace", "", "知识库 ID (选填)")

	drivePermRemoveCmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "移除协作者权限",
		Long:    `从文档空间节点移除协作成员的权限。`,
		Example: `  dws drive permission remove --node DOC_ID --users uid1
  dws drive permission remove --node DOC_ID --users uid1,uid2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			userIds, err := collectUserIDs(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"userIds": userIds,
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "remove_permission", toolArgs)
		},
	}
	drivePermRemoveCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermRemoveCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (必填)")
	drivePermRemoveCmd.Flags().String("user", "", "")
	_ = drivePermRemoveCmd.Flags().MarkHidden("user")
	drivePermRemoveCmd.Flags().String("workspace", "", "知识库 ID (选填)")

	// permission 子命令 --node 隐藏别名（保持与迁移前 doc 命令一致）
	for _, c := range []*cobra.Command{drivePermAddCmd, drivePermUpdateCmd, drivePermListCmd, drivePermRemoveCmd} {
		c.Flags().String("url", "", "")
		c.Flags().String("id", "", "")
		c.Flags().String("node-id", "", "")
		c.Flags().String("doc-id", "", "")
		c.Flags().String("file-id", "", "")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	drivePermissionCmd.AddCommand(drivePermAddCmd, drivePermUpdateCmd, drivePermListCmd, drivePermRemoveCmd)

	// --node 隐藏别名（保持与迁移前 doc 命令一致）
	driveNodeAliasCmds := []*cobra.Command{
		driveCopyCmd, driveMoveCmd, driveRenameCmd, driveStatsCmd, driveShortcutCmd,
	}
	for _, c := range driveNodeAliasCmds {
		c.Flags().String("url", "", "")
		c.Flags().String("id", "", "")
		c.Flags().String("node-id", "", "")
		c.Flags().String("doc-id", "", "")
		c.Flags().String("file-id", "", "")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	// --name/--title 隐藏别名
	driveRenameCmd.Flags().String("title", "", "")
	_ = driveRenameCmd.Flags().MarkHidden("title")

	// ── drive recycle 子命令组 ──
	recycleCmd := &cobra.Command{
		Use:   "recycle",
		Short: "钉盘回收站管理",
		Long:  `管理钉盘回收站：查看回收站列表、还原回收项。`,
		RunE:  groupRunE,
	}

	recycleListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查看回收站文件列表",
		Example: `  dws drive recycle list
  dws drive recycle list --space-id 12345 --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("space-id"); v != "" {
				toolArgs["spaceId"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPTool("list_recycle_items", toolArgs)
		},
	}
	recycleListCmd.Flags().String("space-id", "", "钉盘空间 ID (选填，不传则返回所有空间)")
	recycleListCmd.Flags().Int("limit", 0, "返回条数上限 (默认20，最大50)")
	recycleListCmd.Flags().String("cursor", "", "分页游标")

	recycleRestoreCmd := &cobra.Command{
		Use:     "restore",
		Short:   "还原回收站中的文件",
		Example: `  dws drive recycle restore --id RECYCLE_ITEM_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			recycleItemID, err := mustFlagOrFallback(cmd, "id")
			if err != nil {
				return err
			}
			return callMCPTool("restore_recycle_item", map[string]any{
				"recycleItemId": recycleItemID,
			})
		},
	}
	recycleRestoreCmd.Flags().String("id", "", "回收项 ID (必填，从 recycle list 获取)")

	recycleCmd.AddCommand(recycleListCmd, recycleRestoreCmd)

	// ── deprecated 代理命令（Phase 2：从 doc 迁移，保留兼容，警告引导到新命令）──

	// folder create → dws wiki node create --type folder
	driveFolderCmd := &cobra.Command{Use: "folder", Short: "文件夹管理（deprecated）", RunE: groupRunE}
	driveFolderCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文件夹（deprecated）",
		Long:  `已废弃。请使用 'dws wiki node create --workspace <workspaceId> --name <name> --type folder'。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps.Out.PrintWarning("⚠️  'dws drive folder create' is deprecated, use 'dws wiki node create --workspace <workspaceId> --name <name> --type folder' instead.")
			if err := validateRequiredFlagWithAliases(cmd, "name", "title"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"name": flagOrFallback(cmd, "name", "title"),
			}
			if v := docFolderFlag(cmd); v != "" {
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "create_folder", toolArgs)
		},
	}
	driveFolderCreateCmd.Flags().String("name", "", "文件夹名称（必填）")
	driveFolderCreateCmd.Flags().String("title", "", "")
	_ = driveFolderCreateCmd.Flags().MarkHidden("title")
	driveFolderCreateCmd.Flags().String("folder", "", "父文件夹 nodeId 或 URL")
	driveFolderCreateCmd.Flags().String("workspace", "", "目标知识库 ID")
	driveFolderCmd.AddCommand(driveFolderCreateCmd)

	// ── drive publish (文件互联网公开发布管理) ──
	drivePublishCmd := &cobra.Command{
		Use:   "publish",
		Short: "文件互联网公开发布管理",
		Long:  `管理文件的互联网公开发布状态：设置公开、关闭公开、查询公开状态。`,
		RunE:  groupRunE,
	}

	drivePublishSetCmd := &cobra.Command{
		Use:   "set",
		Short: "[危险] 设置文件为互联网公开",
		Long: `[危险] 将文件设置为互联网公开发布。公开后任何人通过链接即可访问，无需登录钉钉。
操作者需要是该文件的管理员或拥有者。执行前需要确认，或传入 --yes 跳过确认。

公开权限 (--permission): READER(仅可查看) / DOWNLOADER(可查看和下载，默认) / EDITOR(可编辑)`,
		Example: `  dws drive publish set --node <fileId> --yes
  dws drive publish set --node <fileId> --permission READER --yes
  dws drive publish set --node <fileId>                        # 交互式确认`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			if err != nil {
				return err
			}
			if !confirmDelete("互联网公开设置", nodeID) {
				return nil
			}
			toolArgs := map[string]any{
				"fileId":    nodeID,
				"published": true,
			}
			if v := mustGetFlag(cmd, "permission"); v != "" {
				toolArgs["publishPermission"] = v
			}
			return callMCPTool("set_file_publish", toolArgs)
		},
	}
	drivePublishSetCmd.Flags().String("node", "", "目标文件 ID (dentryUuid) 或 URL (必填)")
	drivePublishSetCmd.Flags().String("permission", "", "公开后的权限: READER / DOWNLOADER(默认) / EDITOR")

	drivePublishUnsetCmd := &cobra.Command{
		Use:     "unset",
		Aliases: []string{"off", "close"},
		Short:   "[危险] 关闭文件互联网公开",
		Long:    `[危险] 关闭文件的互联网公开发布。关闭后外部用户将无法再通过链接访问。执行前需要确认，或传入 --yes 跳过确认。`,
		Example: `  dws drive publish unset --node <fileId> --yes
  dws drive publish unset --node <fileId>          # 交互式确认`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			if err != nil {
				return err
			}
			if !confirmDelete("互联网公开关闭", nodeID) {
				return nil
			}
			return callMCPTool("set_file_publish", map[string]any{
				"fileId":    nodeID,
				"published": false,
			})
		},
	}
	drivePublishUnsetCmd.Flags().String("node", "", "目标文件 ID (dentryUuid) 或 URL (必填)")

	drivePublishGetCmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"status"},
		Short:   "查询文件公开发布状态",
		Long:    `查询文件当前是否处于互联网公开发布状态，以及公开发布的权限设置。`,
		Example: `  dws drive publish get --node <fileId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPTool("get_file_publish_status", map[string]any{
				"fileId": nodeID,
			})
		},
	}
	drivePublishGetCmd.Flags().String("node", "", "目标文件 ID (dentryUuid) 或 URL (必填)")

	// publish 子命令 --node 隐藏别名
	for _, c := range []*cobra.Command{drivePublishSetCmd, drivePublishUnsetCmd, drivePublishGetCmd} {
		c.Flags().String("url", "", "")
		c.Flags().String("id", "", "")
		c.Flags().String("node-id", "", "")
		c.Flags().String("file-id", "", "")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	drivePublishCmd.AddCommand(drivePublishSetCmd, drivePublishUnsetCmd, drivePublishGetCmd)

	// ── cross-product hidden aliases ──
	for _, cmd := range []*cobra.Command{
		driveListCmd, driveListSpacesCmd, driveInfoCmd, driveDownloadCmd,
		driveMkdirCmd, driveUploadInfoCmd, driveCommitCmd, driveUploadCmd, driveDeleteCmd,
		driveSearchCmd, driveCopyCmd, driveMoveCmd, driveRenameCmd, driveStatsCmd, driveShortcutCmd,
		driveFolderCreateCmd,
	} {
		RegisterCrossProductAliases(cmd)
	}
	for _, parent := range []*cobra.Command{drivePermissionCmd, recycleCmd, drivePublishCmd} {
		for _, child := range parent.Commands() {
			RegisterCrossProductAliases(child)
		}
	}

	// ── recent 命令：获取最近访问/编辑的文档列表 ──
	driveRecentCmd := &cobra.Command{
		Use:   "recent",
		Short: "获取最近访问/编辑的文档列表",
		Long: `获取当前用户最近访问或编辑过的文档列表。

支持按文档类型、操作类型、创建人、所属组织过滤。所有过滤条件均为可选，不传则不过滤。

操作类型 (--operate-type):
  0   最近访问（含打开+编辑）（默认）
  1   最近编辑
  不传默认仅返回最近访问(0)

创建人类型 (--creator-type):
  0   全部 (默认)
  1   我创建的
  2   他人创建的`,
		Example: `  dws drive recent
  dws drive recent --operate-type 1
  dws drive recent --creator-type 1 --limit 10
  dws drive recent --file-types 0,1 --operate-type 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetIntSlice("file-types"); len(v) > 0 {
				toolArgs["fileTypes"] = v
			}
			if v, _ := cmd.Flags().GetIntSlice("operate-type"); len(v) > 0 {
				toolArgs["operateTypes"] = v
			}
			if cmd.Flags().Changed("creator-type") {
				if v, _ := cmd.Flags().GetInt("creator-type"); v >= 0 {
					toolArgs["creatorType"] = v
				}
			}
			if v, _ := cmd.Flags().GetIntSlice("org-ids"); len(v) > 0 {
				toolArgs["resourceOrgIds"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			} else if v, _ := cmd.Flags().GetInt("max-results"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			return callMCPToolOnServer("doc", "get_recent_list", toolArgs)
		},
	}
	driveRecentCmd.Flags().IntSlice("file-types", nil, "按文档类型过滤，逗号分隔 (参考 RecentAccessType 枚举)")
	driveRecentCmd.Flags().IntSlice("operate-type", nil, "按操作类型过滤: 0=最近访问(默认), 1=最近编辑; 不传默认仅最近访问")
	driveRecentCmd.Flags().Int("creator-type", 0, "按创建人过滤: 0=全部, 1=我创建, 2=他人创建")
	driveRecentCmd.Flags().IntSlice("org-ids", nil, "按资源所属组织 ID 过滤，逗号分隔")
	driveRecentCmd.Flags().Int("limit", 0, "每页数量 (默认 20，最大 20)")
	driveRecentCmd.Flags().Int("max-results", 0, "")
	_ = driveRecentCmd.Flags().MarkHidden("max-results")
	driveRecentCmd.Flags().String("cursor", "", "分页游标 (从上次结果的 nextCursor 获取)")
	driveRecentCmd.Flags().String("page-token", "", "")
	_ = driveRecentCmd.Flags().MarkHidden("page-token")

	driveCmd.AddCommand(
		driveListCmd,
		driveListSpacesCmd,
		driveInfoCmd,
		driveDownloadCmd,
		driveMkdirCmd,
		driveUploadInfoCmd,
		driveCommitCmd,
		driveUploadCmd,
		driveDeleteCmd,
		driveSearchCmd,
		driveRecentCmd,
		// 文档空间代理命令（Phase 1）
		driveCopyCmd,
		driveMoveCmd,
		driveRenameCmd,
		driveStatsCmd,
		driveShortcutCmd,
		drivePermissionCmd,
		drivePublishCmd,
		recycleCmd,
		// deprecated 兼容命令（Phase 2）— 隐藏，保留向后兼容
		driveFolderCmd,
	)

	// deprecated Phase 2 命令从 help 中隐藏（仍可执行，保留向后兼容）
	driveFolderCmd.Hidden = true

	return driveCmd
}

// driveInfoWithDocFallback 获取钉盘文件元数据，若检测到文件属于钉钉文档，
// 自动跟进调用 doc info 获取更准确的文档信息并合并输出。
//
// 判断依据：MCP 返回 result.message 包含"钉钉文档"关键词时，说明该文件
// 是在线文档/表格/脑图等，钉盘接口返回的元数据（如文件名称）可能不准确。
func driveInfoWithDocFallback(fileID string, driveArgs map[string]any) error {
	ctx := context.Background()

	// Step 1: 调用 drive get_file_info
	driveText, err := callMCPToolReturnText(ctx, "get_file_info", driveArgs)
	if err != nil {
		return err
	}

	// Step 2: 解析返回，检查是否需要跟进 doc info
	var driveResp map[string]any
	if err := json.Unmarshal([]byte(driveText), &driveResp); err != nil {
		// 解析失败，直接原样输出
		deps.Out.PrintRaw(driveText)
		return nil
	}

	driveResult, _ := driveResp["result"].(map[string]any)
	if driveResult == nil {
		return deps.Out.PrintJSON(driveResp)
	}

	message, _ := driveResult["message"].(string)
	extension, _ := driveResult["extension"].(string)
	if !strings.Contains(message, "钉钉文档") && !isDingTalkDocExtension(extension) {
		// 普通钉盘文件，直接输出 drive info 结果
		return deps.Out.PrintJSON(driveResp)
	}

	// Step 3: 文件属于钉钉文档，自动跟进调用 doc info
	nodeID, _ := driveResult["fileId"].(string)
	if nodeID == "" {
		nodeID = fileID
	}

	docText, err := callMCPToolReturnTextOnServer(ctx, "doc", "get_document_info", map[string]any{
		"nodeId": nodeID,
	})
	if err != nil {
		// doc info 调用失败，回退输出 drive info 的结果（附加提示）
		deps.Out.PrintInfo("提示: 自动获取文档详情失败，以下为钉盘元数据（文档名称可能不准确）")
		return deps.Out.PrintJSON(driveResp)
	}

	var docResp map[string]any
	if err := json.Unmarshal([]byte(docText), &docResp); err != nil {
		deps.Out.PrintInfo("提示: 自动获取文档详情失败，以下为钉盘元数据（文档名称可能不准确）")
		return deps.Out.PrintJSON(driveResp)
	}

	// Step 4: 合并输出 — 以 doc info 为主体，补充 drive info 的独有字段
	// doc info 可能返回扁平结构（无 result 包裹层）或带 result 包裹层
	docResult, hasResultWrapper := docResp["result"].(map[string]any)
	if !hasResultWrapper {
		// doc info 返回扁平结构，整个 docResp 就是文档信息
		docResult = docResp
	}
	if len(docResult) == 0 {
		return deps.Out.PrintJSON(driveResp)
	}

	// 从 drive info 补充 doc info 中没有的字段
	driveOnlyFields := []string{"dentryId", "path", "fileSize", "extension", "type"}
	for _, field := range driveOnlyFields {
		if val, ok := driveResult[field]; ok {
			if _, exists := docResult[field]; !exists {
				docResult[field] = val
			}
		}
	}

	// 统一输出格式：如果原始 doc 响应是扁平结构，包装成与 drive info 一致的格式
	if !hasResultWrapper {
		return deps.Out.PrintJSON(map[string]any{
			"result":  docResult,
			"success": true,
		})
	}
	return deps.Out.PrintJSON(docResp)
}

// extractFileNameFromResponse extracts the fileName field from MCP download_file response JSON.
// Returns empty string if not found.
func extractFileNameFromResponse(text string) string {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return ""
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	if name, ok := data["fileName"].(string); ok && name != "" {
		// Sanitize: remove path separators to prevent directory traversal
		name = strings.ReplaceAll(name, "/", "_")
		name = strings.ReplaceAll(name, "\\", "_")
		return name
	}
	return ""
}

// isDingTalkDocExtension 判断文件扩展名是否属于钉钉文档类型。
// 钉钉文档类型包括：adoc(在线文档)、axls(在线表格)、amind(脑图)、adraw(画图)。
func isDingTalkDocExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case "adoc", "axls", "amind", "adraw":
		return true
	default:
		return false
	}
}
