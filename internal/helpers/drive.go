package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ──────────────────────────────────────────────────────────
// dws drive — 钉盘
// MCP tools: list_files, get_file_info, download_file, create_folder,
//            get_upload_info, commit_upload
// ──────────────────────────────────────────────────────────

func driveRenameBaseName(name, nodeType, currentExtension string) string {
	trimmed := strings.TrimSpace(name)
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "folder", "dir", "directory":
		return trimmed
	}

	extension := strings.TrimLeft(strings.TrimSpace(currentExtension), ".")
	if extension == "" {
		return trimmed
	}
	suffix := "." + extension
	if len(trimmed) <= len(suffix) || !strings.EqualFold(trimmed[len(trimmed)-len(suffix):], suffix) {
		return trimmed
	}
	return trimmed[:len(trimmed)-len(suffix)]
}

func resolveDriveRenameName(ctx context.Context, nodeID, name string) (string, error) {
	fileID := nodeID
	if parsedNodeID := extractNodeIDFromDocURL(nodeID); parsedNodeID != "" {
		fileID = parsedNodeID
	}
	text, err := callMCPToolReturnTextOnServer(ctx, "drive", "get_file_info", map[string]any{
		"fileId": fileID,
	})
	if err != nil {
		return "", fmt.Errorf("无法读取节点元数据，未执行重命名: %w", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		return "", fmt.Errorf("无法解析节点元数据，未执行重命名: %w", err)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("节点元数据缺少 result，未执行重命名")
	}
	return driveRenameBaseName(
		name,
		firstStringField(result, "type", "nodeType", "fileType"),
		firstStringField(result, "extension", "fileExtension", "ext"),
	), nil
}

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

	// --node switches the upload from create mode to overwrite mode.
	overwriteNodeID := flagOrFallback(cmd, "node", "node-id", "file-id", "doc-id")
	parentID := docFolderFlag(cmd)
	if overwriteNodeID != "" && parentID != "" {
		return fmt.Errorf("--node 与 --folder 互斥：--node 用于覆盖已有文件，--folder 用于上传到目录，不可同时指定")
	}

	// 路由判断：--workspace 存在时走文档空间上传流程
	workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
	spaceID, _ := cmd.Flags().GetString("space-id")
	if workspaceID != "" && spaceID != "" {
		return fmt.Errorf("--space-id 与 --workspace 互斥：请只指定钉盘空间或知识库中的一个目标域")
	}
	if workspaceID != "" {
		return runDriveUploadToDocSpace(cmd, filePath, fileName, fileSize, workspaceID)
	}

	if overwriteNodeID == "" {
		if err := validateDriveParentID(parentID); err != nil {
			return err
		}
	}

	if deps.Caller.DryRun() {
		if deps.Caller.Format() == "json" {
			return deps.Out.PrintJSON(map[string]any{
				"dry_run":      true,
				"executed":     false,
				"preview_kind": "plan",
				"operation":    "upload",
				"source":       "drive",
				"file":         filePath,
				"file_name":    fileName,
				"file_size":    fileSize,
				"space_id":     spaceID,
				"folder_id":    parentID,
				"node_id":      overwriteNodeID,
			})
		}
		deps.Out.PrintKeyValue("操作", "上传文件到钉盘")
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", fileName)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		if overwriteNodeID != "" {
			deps.Out.PrintKeyValue("覆盖目标", overwriteNodeID)
		}
		return nil
	}

	if overwriteNodeID != "" && !confirmDangerousAction(cmd, "overwrite drive file", overwriteNodeID) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	mimeType, _ := cmd.Flags().GetString("mime-type")
	return uploadToDrive(ctx, filePath, fileName, fileSize, spaceID, parentID, overwriteNodeID, mimeType)
}

// runDriveUploadToDocSpace 处理文档空间上传流程（当 --workspace 存在时路由到此）。
// 使用 doc MCP server 的 get_file_upload_info + commit_uploaded_file 工具。
func runDriveUploadToDocSpace(cmd *cobra.Command, filePath, fileName string, fileSize int64, workspaceID string) error {
	overwriteNodeID := flagOrFallback(cmd, "node", "node-id", "file-id", "doc-id")
	folder := docFolderFlag(cmd)
	if overwriteNodeID == "" {
		if err := validateDocFolderID(folder); err != nil {
			return err
		}
	}

	// 补全文件名后缀
	if filepath.Ext(fileName) == "" {
		if ext := filepath.Ext(filePath); ext != "" {
			fileName += ext
		}
	}

	if deps.Caller.DryRun() {
		if deps.Caller.Format() == "json" {
			return deps.Out.PrintJSON(map[string]any{
				"dry_run":      true,
				"executed":     false,
				"preview_kind": "plan",
				"operation":    "upload",
				"source":       "doc",
				"file":         filePath,
				"file_name":    fileName,
				"file_size":    fileSize,
				"workspace_id": workspaceID,
				"folder_id":    folder,
				"node_id":      overwriteNodeID,
			})
		}
		deps.Out.PrintKeyValue("操作", "上传文件到文档空间")
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", fileName)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		deps.Out.PrintKeyValue("知识库", workspaceID)
		if overwriteNodeID != "" {
			deps.Out.PrintKeyValue("覆盖目标", overwriteNodeID)
		}
		return nil
	}

	if overwriteNodeID != "" && !confirmDangerousAction(cmd, "overwrite document-space file", overwriteNodeID) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	convert, _ := cmd.Flags().GetBool("convert")
	return uploadToDocSpace(ctx, filePath, fileName, fileSize, workspaceID, folder, overwriteNodeID, convert)
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

// DriveUploadRequest describes the reusable Drive upload transaction used by
// the native leaf and the curated +upload shortcut. FilePath must already be
// resolved and validated by the caller.
type DriveUploadRequest struct {
	FilePath      string
	FileName      string
	FileSize      int64
	SpaceID       string
	ParentID      string
	OverwriteFile string
	MIMEType      string
}

// UploadDriveFileData runs credentials -> OSS PUT -> commit exactly once and
// returns the parsed commit response without rendering it. Unlike the legacy
// leaf helper, this path fails when the commit has no non-empty JSON response;
// the Shortcut can then require terminal success evidence and read back the
// created node before reporting success.
func UploadDriveFileData(ctx context.Context, request DriveUploadRequest) (map[string]any, error) {
	if strings.TrimSpace(request.FilePath) == "" || strings.TrimSpace(request.FileName) == "" || request.FileSize <= 0 {
		return nil, fmt.Errorf("invalid drive upload request")
	}
	step1Args := map[string]any{
		"fileName": request.FileName,
		"fileSize": float64(request.FileSize),
	}
	if request.SpaceID != "" {
		step1Args["spaceId"] = request.SpaceID
	}
	if request.MIMEType != "" {
		step1Args["mimeType"] = request.MIMEType
	}
	if request.OverwriteFile != "" {
		step1Args["overwriteFileId"] = request.OverwriteFile
	} else if request.ParentID != "" {
		step1Args["parentId"] = request.ParentID
	}

	credentialText, err := callMCPToolReturnTextOnServer(ctx, "drive", "get_upload_info", step1Args)
	if err != nil {
		return nil, err
	}
	uploadID, err := driveUploadPut(ctx, credentialText, func(refreshCtx context.Context) (string, error) {
		return callMCPToolReturnTextOnServer(refreshCtx, "drive", "get_upload_info", step1Args)
	}, request.FilePath, request.FileSize)
	if err != nil {
		return nil, err
	}

	commitArgs := map[string]any{
		"fileName": request.FileName,
		"fileSize": float64(request.FileSize),
		"uploadId": uploadID,
	}
	if request.SpaceID != "" {
		commitArgs["spaceId"] = request.SpaceID
	}
	if request.OverwriteFile != "" {
		commitArgs["overwriteFileId"] = request.OverwriteFile
	} else if request.ParentID != "" {
		commitArgs["parentId"] = request.ParentID
	}
	commitText, err := callMCPToolReturnTextOnServer(ctx, "drive", "commit_upload", commitArgs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(commitText) == "" {
		return nil, fmt.Errorf("commit_upload returned no business result; remote effect is unknown")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(commitText), &result); err != nil {
		return nil, fmt.Errorf("parse commit_upload response: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("commit_upload returned an empty JSON object; remote effect is unknown")
	}
	return result, nil
}

func newDriveCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/drive.json
	// products.drive). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "drive",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理钉盘及文档空间中的文件、目录、上传下载、回收站与公开发布",
			UseWhen: []string{
				"浏览、搜索、上传、下载或整理钉盘和文档空间文件时",
			},
			AvoidWhen: []string{
				"需要读取或编辑在线文档正文时使用 doc；需要管理知识库空间或成员时使用 wiki",
			},
		},
	})
	driveCmd := newGroupCommand(&cobra.Command{
		Use:   "drive",
		Short: "钉盘文件管理",
		Long:  `钉盘：列出文件/文件夹、获取元数据和统计信息、创建快捷方式、下载、上传及管理文件。`,
		RunE:  groupRunE,
	})
	installDocDelegationAuth(driveCmd)

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
  dws drive list --workspace <workspaceId> --folder <folderId>
  dws drive list --latest 5
  dws drive list --folder <dentryUuid> --latest 3 --pattern "*.docx"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern, _ := cmd.Flags().GetString("pattern")

			depth, _ := cmd.Flags().GetInt("depth")

			latest, _ := cmd.Flags().GetInt("latest")
			if cmd.Flags().Changed("latest") {
				if err := validateDriveListLatest(cmd, latest); err != nil {
					return err
				}
				if cmd.Flags().Changed("versions") {
					return &CLIError{Code: CodeInvalidParam, Message: "--latest 不能与 --versions 同时使用"}
				}
			}

			// --type/--start/--end 客户端过滤：激活即切 BFS 全量拉取后筛（两路由统一）；
			// 未启用返回零值，存量分支字节级不变。互斥/非法值/start>end 在此拒绝。
			filter, err := parseDriveListFilter(cmd)
			if err != nil {
				return err
			}

			// --versions 模式：列出文件历史版本（仅普通文件）
			// 先于 --depth 校验执行：versions 模式合法使用 --limit，
			// 不应被「--limit 与 --depth 不兼容」的误导性报错拦截。
			if cmd.Flags().Changed("versions") {
				if cmd.Flags().Changed("depth") && depth > 1 {
					return &CLIError{
						Code:    CodeInvalidParam,
						Message: "--versions 与 --depth 不能同时使用",
					}
				}
				if pattern != "" {
					return &CLIError{
						Code:    CodeInvalidParam,
						Message: "--versions 与 --pattern 不能同时使用",
					}
				}
				nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
				if err != nil {
					return err
				}
				toolArgs := map[string]any{"nodeId": nodeID}
				if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
					toolArgs["maxResults"] = v
				}
				if v := flagOrFallback(cmd, "cursor", "next-token", "page-token"); v != "" {
					toolArgs["nextCursor"] = v
				}
				return callMCPToolOnServer("drive", "list_file_versions", toolArgs)
			}

			if cmd.Flags().Changed("depth") {
				if err := validateDriveListDepth(cmd, depth); err != nil {
					return err
				}
			}

			// 如果指定了 --workspace，路由到文档空间（doc MCP server）
			workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
			if workspaceID != "" {
				// depth>1 时 --pattern 放开（先递归后过滤）；--order-by/--space-id/--thumbnail
				// 知识库无对应参数，静默忽略。
				// filter 激活时 depth==1 也走 BFS 退化态（全量拉取→CLI 侧筛）。
				if depth > 1 || latest > 0 || filter.active() {
					quiet, _ := cmd.Flags().GetBool("quiet")
					baseArgs := map[string]any{"workspaceId": workspaceID}
					rootFolder := docFolderFlag(cmd, "node", "file-id")
					if rootFolder != "" {
						if err := validateDocFolderID(rootFolder); err != nil {
							return err
						}
					}
					return runDriveListDepth(cmd, newDocDepthRoute(), baseArgs, rootFolder, depth, pattern, quiet, latest, filter)
				}
				if pattern != "" {
					return &CLIError{
						Code:    CodeInvalidParam,
						Message: "--pattern 仅适用于钉盘文件列表，不能与 --workspace 同时使用",
					}
				}
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

			// 钉盘：filter 激活即 depth=1 单层退化态（全量翻完当前目录后 CLI 侧筛）；
			// filter+latest 单层同走 BFS（先筛后排，不走 runDriveListLatest 凑够即停）。
			if depth > 1 || filter.active() {
				quiet, _ := cmd.Flags().GetBool("quiet")
				baseArgs := map[string]any{}
				if v, _ := cmd.Flags().GetString("space-id"); v != "" {
					baseArgs["spaceId"] = v
				}
				if v, _ := cmd.Flags().GetString("order-by"); v != "" {
					baseArgs["orderBy"] = v
				}
				if v, _ := cmd.Flags().GetString("order"); v != "" {
					baseArgs["order"] = v
				}
				if v, _ := cmd.Flags().GetBool("thumbnail"); v {
					baseArgs["withThumbnail"] = true
				}
				rootFolder := flagOrFallback(cmd, "folder", "parent-id")
				if rootFolder != "" {
					if err := validateDriveParentID(rootFolder); err != nil {
						return err
					}
				}
				return runDriveListDepth(cmd, newDrivePanDepthRoute(), baseArgs, rootFolder, depth, pattern, quiet, latest, filter)
			}

			// 默认路由：钉盘文件列表
			if latest > 0 {
				quiet, _ := cmd.Flags().GetBool("quiet")
				baseArgs := map[string]any{}
				if v, _ := cmd.Flags().GetString("space-id"); v != "" {
					baseArgs["spaceId"] = v
				}
				if v, _ := cmd.Flags().GetBool("thumbnail"); v {
					baseArgs["withThumbnail"] = true
				}
				rootFolder := flagOrFallback(cmd, "folder", "parent-id")
				if rootFolder != "" {
					if err := validateDriveParentID(rootFolder); err != nil {
						return err
					}
				}
				return runDriveListLatest(cmd, baseArgs, rootFolder, latest, pattern, quiet)
			}
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
			// --pattern 页内过滤（现状缺口附带修复：透传即打印无法夹过滤，取回解析后筛）；
			// 不带 pattern 时保持 callMCPTool 纯透传，存量行为不变。
			if pattern != "" {
				return callDriveListPageWithPattern(argsMap, pattern)
			}
			return callMCPTool("list_files", argsMap)
		},
	}
	DeclareLeafMetadata(driveListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "list_files",
				CanonicalPath:  "drive.list_files",
				CLIPath:        "drive list",
				PrimaryCLIPath: "drive list",
			},
			Description: "获取文件/文件夹列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The CLI command routes by --workspace between drive/list_files and doc/list_nodes, so the reviewed executable wrapper has no single direct MCP interface.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取文件/文件夹列表",
				UseWhen: []string{
					"用户要浏览「我的文件」/钉盘/网盘某目录下有哪些文件或文件夹时",
					"已知父文件夹 dentryUuid，要列出其子项以便继续 download/copy/move 时",
					"传 --workspace 时要列出文档空间/知识库根或子目录（与 wiki node list 场景重叠时，用户说钉盘/我的文件优先本命令）",
					"要在已知目录内按类型/修改时间做无关键词筛选时：--type file --start 7d（钉盘与知识库路由均可，CLI 侧过滤）",
				},
				AvoidWhen: []string{
					"只记得关键词、不知道所在目录时改用 dws drive search",
					"明确要在某个知识库内按目录浏览且已有 workspaceId 时可用 dws wiki node list",
					"要找最近打开/编辑过的文档改用 dws drive recent",
					"带关键词的过滤改用 dws drive search（--extensions/--modified-from 等已可用）",
				},
				Examples: []string{
					"dws drive list --limit 20 --format json",
					"dws drive list --latest 5 --format json",
				},
			},
		},
	})

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
	DeclareLeafMetadata(driveInfoCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_file_info",
				CanonicalPath:  "drive.get_file_info",
				CLIPath:        "drive info",
				PrimaryCLIPath: "drive info",
			},
			Description: "获取文件元数据信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "get_file_info"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取文件元数据信息",
				UseWhen: []string{
					"用户要查看钉盘文件/文件夹元信息（名称、类型、大小、路径、时间）时",
					"准备读内容前需先判断 extension/是否在线文档，再路由到 doc read / sheet / download 时",
				},
				AvoidWhen: []string{
					"要读在线文档正文改用 dws doc read（先本命令或 doc info 确认类型）",
					"要下载普通文件改用 dws drive download",
					"只要目录列表改用 dws drive list",
				},
				Examples: []string{"dws drive info --node <dentryUuid> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "fileId"},
			},
		},
	})

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
  dws drive download --node <dentryUuid> --output ~/downloads/
  dws drive download --node <dentryUuid> --output ./big.zip --part-size 32MB --parallel 8`,
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

			// fail-fast：分片下载参数校验
			dlOpts, err := driveDownloadOptionsFromFlags(cmd)
			if err != nil {
				return err
			}
			dlOpts.logf = func(format string, a ...any) {
				printJSONSafeInfo(fmt.Sprintf(format, a...))
			}

			if deps.Caller.DryRun() {
				if strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json") {
					return deps.Out.PrintJSON(map[string]any{
						"dry_run":      true,
						"executed":     false,
						"preview_kind": "plan",
						"operation":    "drive_download",
						"nodeId":       fileID,
						"savedPath":    outputPath,
					})
				}
				deps.Out.PrintKeyValue("操作", "下载钉盘文件")
				deps.Out.PrintKeyValue("文件ID", fileID)
				deps.Out.PrintKeyValue("输出", outputPath)
				return nil
			}

			ctx := cmd.Context()

			// Step 1: 获取下载 URL 和签名请求头
			printJSONSafeInfo("[1/2] 获取下载链接...")
			text, err := callMCPToolReturnText(ctx, "download_file", argsMap)
			if err != nil {
				return err
			}

			resourceURL, dlHeaders, err := parseDriveDownloadInfo(text)
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

			// Step 2: 分片下载（自动分派 + 401/403 凭证刷新重试）
			printJSONSafeInfo(fmt.Sprintf("[2/2] 下载文件到 %s ...", outputPath))
			dlOpts.knownSize = parseDownloadFileSize(text)
			dlOpts.nodeID = fileID
			dlOpts.version = parseDownloadFileVersion(text)
			fetchCred := func(fctx context.Context) (string, map[string]string, int, error) {
				t, ferr := callMCPToolReturnText(fctx, "download_file", argsMap)
				if ferr != nil {
					return "", nil, 0, ferr
				}
				u, h, perr := parseDriveDownloadInfo(t)
				if perr != nil {
					return "", nil, 0, perr
				}
				return u, h, parseDownloadFileVersion(t), nil
			}
			if err := driveTransferDownload(ctx, fetchCred, resourceURL, dlHeaders, outputPath, dlOpts); err != nil {
				if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
					partSize, _ := cmd.Flags().GetString("part-size")
					noResume, _ := cmd.Flags().GetBool("no-resume")
					if partSize != "" && !noResume {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n[INFO] 下载中断，已保存断点（可重新执行相同命令续传）\n")
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n[INFO] 下载中断\n")
					}
					cmd.SilenceErrors = true
					return err
				}
				return err
			}

			if strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json") {
				info, err := os.Stat(outputPath)
				if err != nil {
					return fmt.Errorf("读取下载产物信息失败: %w", err)
				}
				return deps.Out.PrintJSON(map[string]any{
					"success":   true,
					"nodeId":    fileID,
					"version":   dlOpts.version,
					"savedPath": outputPath,
					"sizeBytes": info.Size(),
				})
			}
			deps.Out.PrintInfo(fmt.Sprintf("下载完成: %s", outputPath))
			return nil
		},
	}
	DeclareLeafMetadata(driveDownloadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "download_file",
				CanonicalPath:  "drive.download_file",
				CLIPath:        "drive download",
				PrimaryCLIPath: "drive download",
			},
			Description: "下载钉盘或文档空间文件到本地",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "download_file"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "下载钉盘或文档空间文件到本地",
				UseWhen: []string{
					"用户要把钉盘普通文件（PDF/图片/Office 等非在线文档）下载到本地路径时",
					"已确认 contentType 非 ALIDOC，需要落盘本地查看时",
				},
				AvoidWhen: []string{
					"在线文档(adoc)要导出为 Word/docx 改用 dws doc export，不要用 download 代替导出",
					"只要临时下载链接语义且走文档附件块时用 dws doc media download",
					"未指定 --output 不要调用（CLI 必填）",
				},
				Examples: []string{
					"dws drive download --node <dentryUuid> --output ./report.pdf --format json",
					"dws drive download --node <dentryUuid> --output ~/downloads/ --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "fileId"},
				// CLI-local multipart transfer knobs (not MCP properties; see mapping exclusions).
				{Name: "part-size", Description: "分片下载的分片大小（如 8MB/16MB/1GB）"},
				{Name: "parallel", Description: "分片下载并发数（1-8）"},
				{Name: "no-resume", Description: "关闭断点续传"},
				// Wukong compat alias: routes to download-version; not a download_file property.
				{Name: "version", Description: "下载指定历史版本号（兼容别名，等价 download-version）"},
			},
		},
	})

	driveDownloadVersionCmd := &cobra.Command{
		Use:   "download-version",
		Short: "下载文件历史版本到本地",
		Long: `下载钉盘文件的指定历史版本到本地（两步下载流程）。

仅适用于普通文件（如 pdf、docx、xlsx、png 等）：
  钉钉在线文档（adoc）请使用 dws doc version 系列命令
  钉钉在线表格（axls）请使用 dws sheet version 系列命令

流程:
  1. 获取历史版本下载 URL 和签名请求头 (download_file_version)
  2. HTTP GET 下载文件二进制内容到本地

版本号从 dws drive list --node <dentryUuid> --versions 获取。`,
		Example: `  dws drive download-version --node <dentryUuid> --version 3 --output ./report_v3.pdf
  dws drive download-version --node <dentryUuid> --version 3 --output ~/downloads/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			versionNum, _ := cmd.Flags().GetInt("version")
			if versionNum <= 0 {
				return fmt.Errorf("--version 必须为正整数，当前值: %d（版本号从 drive list --versions 获取）", versionNum)
			}
			outputPath, _ := cmd.Flags().GetString("output")
			if outputPath == "" {
				return fmt.Errorf("flag --output is required")
			}

			// fail-fast：分片下载参数校验
			dlOpts, err := driveDownloadOptionsFromFlags(cmd)
			if err != nil {
				return err
			}
			dlOpts.logf = func(format string, a ...any) {
				printJSONSafeInfo(fmt.Sprintf(format, a...))
			}

			if deps.Caller.DryRun() {
				if strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json") {
					return deps.Out.PrintJSON(map[string]any{
						"dry_run":      true,
						"executed":     false,
						"preview_kind": "plan",
						"operation":    "drive_download_version",
						"nodeId":       fileID,
						"version":      versionNum,
						"savedPath":    outputPath,
					})
				}
				deps.Out.PrintKeyValue("操作", "下载文件历史版本")
				deps.Out.PrintKeyValue("节点ID", fileID)
				deps.Out.PrintKeyValue("版本号", fmt.Sprintf("%d", versionNum))
				deps.Out.PrintKeyValue("输出", outputPath)
				return nil
			}

			ctx := cmd.Context()
			printJSONSafeInfo("[1/2] 获取历史版本下载链接...")
			dlArgsMap := map[string]any{
				"nodeId":  fileID,
				"version": versionNum,
			}
			text, err := callMCPToolReturnTextOnServer(ctx, "drive", "download_file_version", dlArgsMap)
			if err != nil {
				return err
			}
			resourceURL, dlHeaders, err := parseDriveDownloadInfo(text)
			if err != nil {
				return err
			}
			if fi, statErr := os.Stat(outputPath); statErr == nil && fi.IsDir() {
				filename := extractFileNameFromResponse(text)
				if filename == "" {
					filename = inferFilename(resourceURL)
				}
				outputPath = filepath.Join(outputPath, filename)
			}
			printJSONSafeInfo(fmt.Sprintf("[2/2] 下载文件到 %s ...", outputPath))
			dlOpts.knownSize = parseDownloadFileSize(text)
			dlOpts.nodeID = fileID
			dlOpts.version = versionNum
			fetchCred := func(fctx context.Context) (string, map[string]string, int, error) {
				t, ferr := callMCPToolReturnTextOnServer(fctx, "drive", "download_file_version", dlArgsMap)
				if ferr != nil {
					return "", nil, 0, ferr
				}
				u, h, perr := parseDriveDownloadInfo(t)
				if perr != nil {
					return "", nil, 0, perr
				}
				return u, h, parseDownloadFileVersion(t), nil
			}
			if err := driveTransferDownload(ctx, fetchCred, resourceURL, dlHeaders, outputPath, dlOpts); err != nil {
				if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
					partSize, _ := cmd.Flags().GetString("part-size")
					noResume, _ := cmd.Flags().GetBool("no-resume")
					if partSize != "" && !noResume {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n[INFO] 下载中断，已保存断点（可重新执行相同命令续传）\n")
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n[INFO] 下载中断\n")
					}
					cmd.SilenceErrors = true
					return err
				}
				return err
			}
			if strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json") {
				info, err := os.Stat(outputPath)
				if err != nil {
					return fmt.Errorf("读取下载产物信息失败: %w", err)
				}
				return deps.Out.PrintJSON(map[string]any{
					"success":   true,
					"nodeId":    fileID,
					"version":   versionNum,
					"savedPath": outputPath,
					"sizeBytes": info.Size(),
				})
			}
			deps.Out.PrintInfo(fmt.Sprintf("下载完成: %s", outputPath))
			return nil
		},
	}
	DeclareLeafMetadata(driveDownloadVersionCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "download_file_version",
				CanonicalPath:  "drive.download_file_version",
				CLIPath:        "drive download-version",
				PrimaryCLIPath: "drive download-version",
			},
			Description: "下载钉盘普通文件的指定历史版本到本地（两步下载：取签名 URL 后 HTTP GET）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "下载钉盘普通文件的指定历史版本到本地（两步下载：取签名 URL 后 HTTP GET）",
				UseWhen: []string{
					"用户要下载文件的历史版本/旧版本",
					"版本号已通过 drive list --versions 获取",
				},
				AvoidWhen: []string{
					"下载最新版本用 drive download",
					"在线文档（adoc）历史版本用 doc version 系列命令",
					"在线表格（axls）历史版本用 sheet version 系列命令",
				},
				Examples: []string{"dws drive download-version --node <dentryUuid> --version 3 --output ./report_v3.pdf"},
			},
			Parameters: []contract.ParamDecl{
				// CLI-local multipart transfer knobs (not interface properties; see mapping exclusions).
				{Name: "part-size", Description: "分片下载的分片大小（如 8MB/16MB/1GB）"},
				{Name: "parallel", Description: "分片下载并发数（1-8）"},
				{Name: "no-resume", Description: "关闭断点续传"},
			},
		},
	})

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
	DeclareLeafMetadata(driveMkdirCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "create_folder",
				CanonicalPath:  "drive.create_folder",
				CLIPath:        "drive mkdir",
				PrimaryCLIPath: "drive mkdir",
			},
			Description: "创建文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "create_folder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建文件夹",
				UseWhen: []string{
					"用户要在钉盘/「我的文件」下新建普通文件夹时",
					"已知父目录 dentryUuid，要在其下建子目录时",
				},
				AvoidWhen: []string{
					"要在知识库内建文件夹改用 dws wiki node create --type folder --workspace <id>",
					"要创建在线文档(adoc)改用 dws doc create 或 wiki node create --type adoc",
				},
				Examples: []string{
					"dws drive mkdir --name \"项目资料\" --format json",
					"dws drive mkdir --name \"子目录\" --folder <dentryUuid> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "parentId"},
			},
		},
	})

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
	DeclareLeafMetadata(driveUploadInfoCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_upload_info",
				CanonicalPath:  "drive.get_upload_info",
				CLIPath:        "drive upload-info",
				PrimaryCLIPath: "drive upload-info",
			},
			Description: "获取文件上传信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "get_upload_info"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取文件上传信息",
				UseWhen:      []string{"仅当无法使用 drive upload 一条命令、需要自定义流式上传时，获取 OSS 预签名上传凭证"},
				AvoidWhen: []string{
					"普通上传请直接用 dws drive upload，不要手动走三步",
					"拿到凭证后须 HTTP PUT 再 dws drive commit；本命令本身不完成入库",
				},
				Examples: []string{"dws drive upload-info --file-name \"report.pdf\" --file-size 102400 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "parentId"},
			},
		},
	})

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
	DeclareLeafMetadata(driveCommitCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "commit_upload",
				CanonicalPath:  "drive.commit_upload",
				CLIPath:        "drive commit",
				PrimaryCLIPath: "drive commit",
			},
			Description: "提交文件上传",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "commit_upload"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "提交文件上传",
				UseWhen:      []string{"手动三步上传的最后一步：已 PUT 到 OSS，用 upload-info 返回的 uploadId 提交入库时"},
				AvoidWhen: []string{
					"普通上传请用 dws drive upload",
					"尚未 PUT 成功或 uploadId 过期时不要 commit；需重新 upload-info",
				},
				Examples: []string{"dws drive commit --file-name \"report.pdf\" --file-size 102400 --upload-id <UPLOAD_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "parentId"},
			},
		},
	})

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
	driveListCmd.Flags().Bool("versions", false, "列出文件历史版本而非文件列表 (需配合 --node)")
	driveListCmd.Flags().String("node", "", "文件 ID (dentryUuid) 或 URL (--versions 模式下必填)")
	driveListCmd.Flags().String("pattern", "", "按名称通配过滤结果，如 \"*日报*\" (客户端过滤) (可选)")
	driveListCmd.Flags().Int("depth", 1, "递归列出子目录层级，默认 1(仅当前层)，最大 5；与 --cursor/--limit 互斥；与 --workspace 组合时走知识库递归 (可选)")
	driveListCmd.Flags().Int("latest", 0, "按修改时间取最新 N 个文件（1~50）；与 --pattern 组合时表示名称匹配的文件中最新 N 个；可与 --workspace/--depth 组合；与 --order-by/--order/--limit/--cursor 互斥；扫描触发 2000 条上限或途中目录读取失败时报错，不产出不完整的 Top-N (可选)")
	driveListCmd.Flags().Bool("quiet", false, "关闭递归进度输出(stderr)，不影响 stdout JSON (--depth>1 或 --latest 多页扫描时有效) (可选)")
	driveListCmd.Flags().String("type", "", "按节点类型过滤: file|folder（客户端过滤：全量扫描后筛，钉盘/知识库均可用；与 --versions/--cursor/--order-by/--order/--limit 互斥）(可选)")
	driveListCmd.Flags().String("start", "", "按修改时间过滤·起始: 相对时间如 24h/7d/2w、RFC3339、YYYY-MM-DD（客户端过滤，互斥同 --type）(可选)")
	driveListCmd.Flags().String("end", "", "按修改时间过滤·截止: 语法同 --start（客户端过滤，互斥同 --type）(可选)")

	driveInfoCmd.Flags().String("node", "", "节点 ID (dentryUuid) (必填)")
	driveInfoCmd.Flags().String("space-id", "", "节点所属空间 ID (可选)")

	driveDownloadCmd.Flags().String("node", "", "文件 ID (dentryUuid) (必填)")
	driveDownloadCmd.Flags().String("space-id", "", "文件所属空间 ID (可选)")
	driveDownloadCmd.Flags().String("output", "", "本地保存路径 (文件路径或目录，必填)")
	driveDownloadCmd.Flags().Int("version", 0, "下载指定历史版本号（兼容别名，等价 download-version）")
	driveDownloadCmd.Flags().String("part-size", "16MB", "分片下载的分片大小，如 8MB/16MB/1GB，范围 1MB-1GB (可选)")
	driveDownloadCmd.Flags().Int("parallel", 4, "分片下载并发数，范围 1-8 (可选)")
	driveDownloadCmd.Flags().Bool("no-resume", false, "关闭断点续传 (可选)")

	driveDownloadVersionCmd.Flags().String("node", "", "文件 ID (dentryUuid) 或 URL (必填)")
	driveDownloadVersionCmd.Flags().Int("version", 0, "历史版本号 (必填，正整数，从 drive list --versions 获取)")
	driveDownloadVersionCmd.Flags().String("output", "", "本地保存路径 (文件路径或目录，必填)")
	driveDownloadVersionCmd.Flags().String("part-size", "16MB", "分片下载的分片大小，如 8MB/16MB/1GB，范围 1MB-1GB (可选)")
	driveDownloadVersionCmd.Flags().Int("parallel", 4, "分片下载并发数，范围 1-8 (可选)")
	driveDownloadVersionCmd.Flags().Bool("no-resume", false, "关闭断点续传 (可选)")
	for _, alias := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		driveDownloadVersionCmd.Flags().String(alias, "", "")
		_ = driveDownloadVersionCmd.Flags().MarkHidden(alias)
	}
	// Wukong compat: `drive download --version N` routes to download-version.
	origDriveDownloadRunE := driveDownloadCmd.RunE
	driveDownloadCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("version") {
			return driveDownloadVersionCmd.RunE(cmd, args)
		}
		return origDriveDownloadRunE(cmd, args)
	}

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
	DeclareLeafMetadata(driveUploadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "upload",
				CanonicalPath:  "drive.upload",
				CLIPath:        "drive upload",
				PrimaryCLIPath: "drive upload",
			},
			Description: "上传本地文件到钉盘或文档空间，或按节点 ID 确认覆盖已有文件",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令包含多个 RPC、条件分派或本地 HTTP/文件步骤，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "上传本地文件到钉盘或文档空间，或按节点 ID 确认覆盖已有文件",
				UseWhen: []string{
					"用户要把本地文件上传到钉盘/我的文件（首选一条命令自动完成凭证+PUT+提交）时",
					"上传到知识库/文档空间时加 --workspace；需要转在线文档时加 --convert",
					"用户明确要求用本地文件替换已有钉盘/文档空间文件时传 --node；该模式会覆盖远端内容并要求确认",
				},
				AvoidWhen: []string{
					"常规场景不要拆成 upload-info + 手动 PUT + commit；仅自定义流式上传才用三步",
					"要把文件作为文档正文附件插入改用 dws doc media insert",
					"用户明确说文档空间且走 doc 兼容入口时可用 dws doc upload，默认仍推荐本命令",
					"用户没有明确同意替换目标文件时不要使用 --node；新建上传应使用 --folder 或目标根目录",
				},
				Examples: []string{
					"dws drive upload --file ./report.pdf --format json",
					"dws drive upload --file ./README.md --node <dentryUuid> --format json",
				},
			},
			// Composite multi-step leaf (get_upload_info → PUT → commit_upload):
			// no single RPCName / interface_ref. Keep --node→nodeId on this leaf
			// only; do not hang it on upload-info or commit.
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(false)},
			},
		},
	})
	driveUploadCmd.Flags().String("file", "", "本地文件路径 (必填)")
	driveUploadCmd.Flags().String("file-name", "", "文件显示名称 (默认使用文件名)")
	driveUploadCmd.Flags().String("space-id", "", "目标钉盘空间 ID，不传则使用「我的文件」 (可选)")
	driveUploadCmd.Flags().String("mime-type", "", "文件 MIME 类型，不传则自动推断 (可选)")
	driveUploadCmd.Flags().String("folder", "", "父节点 ID，不传则上传到空间根目录 (可选，与 --node 互斥)")
	driveUploadCmd.Flags().String("workspace", "", "目标知识库 ID，传入时路由到文档空间上传 (可选)")
	driveUploadCmd.Flags().Bool("convert", false, "是否转换为钉钉在线文档 (仅文档空间上传时生效)")
	driveUploadCmd.Flags().String("node", "", "覆盖目标文件 ID，传入即覆盖已有文件（与 --folder 互斥）(可选)")

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
	DeclareLeafMetadata(driveListSpacesCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "list_spaces",
				CanonicalPath:  "drive.list_spaces",
				CLIPath:        "drive list-spaces",
				PrimaryCLIPath: "drive list-spaces",
			},
			Description: "兼容查询钉盘空间列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "list_spaces"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容查询钉盘空间列表",
				UseWhen:      []string{"兼容入口：枚举钉盘企业空间或「我的文件」空间以拿 spaceId/rootFolderId 时"},
				AvoidWhen: []string{
					"推荐改用 dws wiki space list --type orgSpace 或 --type mySpace（本命令已 deprecated）",
					"要列知识库列表改用 dws wiki space list（默认 orgWikiSpace）",
				},
				Examples: []string{
					"dws drive list-spaces --space-type orgSpace --limit 20 --format json",
					"dws drive list-spaces --space-type mySpace --format json",
				},
			},
			// MCP pins maxResults as number while the Cobra flag is integer. Publishing
			// interface_type=number is a merge-base contract change; declare integer so
			// resolution equals cobra_flag_type and omits a separate interface_type field.
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "maxResults", InterfaceType: "integer"},
				{Name: "cursor", Property: "nextToken"},
			},
		},
	})
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
	DeclareLeafMetadata(driveSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "search_files",
				CanonicalPath:  "drive.search_files",
				CLIPath:        "drive search",
				PrimaryCLIPath: "drive search",
			},
			Description: "全局搜索文件，默认同时搜索钉盘和文档空间，合并返回结果",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "search_files"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "全局搜索文件，默认同时搜索钉盘和文档空间，合并返回结果",
				UseWhen: []string{
					"用户要在钉盘/我的文件里按关键词找文件、文件夹或团队空间，且不知道具体路径时",
					"需要按扩展名/文件类型/创建者/时间缩小范围的全局搜索（默认 target=all 聚合钉盘+文档空间）",
					"明确搜团队空间名以拿 spaceId/rootFolderId 时用 --target space",
				},
				AvoidWhen: []string{
					"已明确知识库 workspaceId、只在该库内搜时改用 dws wiki node search --workspace <id>",
					"已知目录、只需浏览子项时改用 dws drive list，不要用搜索代替目录遍历",
					"首页未命中时优先改关键词/过滤条件，而不是反复翻页",
				},
				Examples: []string{
					"dws drive search --query \"季度汇报\" --format json",
					"dws drive search --query \"合同\" --target file --extensions pdf,docx --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "created-from", Property: "createdTimeFrom"},
				{Name: "created-to", Property: "createdTimeTo"},
				{Name: "creator-uids", Property: "creatorUserIds"},
				{Name: "cursor", Property: "pageToken"},
				{Name: "limit", Property: "pageSize"},
				{Name: "modified-from", Property: "modifiedTimeFrom"},
				{Name: "modified-to", Property: "modifiedTimeTo"},
				{Name: "query", Property: "keyword"},
				{Name: "target", Property: "searchTarget"},
			},
		},
	})
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
			// 同 dws doc delete：delete_document 工具仅注册在 doc MCP server 上，
			// 钉盘节点（fileId）与文档节点共用同一套 dentryUuid 体系，因此显式
			// 路由到 doc server 才能找到该工具。若使用 callMCPTool 让 resolveProductID
			// 路由，会被路由到 drive server，服务端会返回 PARAM_ERROR - 未找到指定工具。
			return callMCPToolOnServer("doc", "delete_document", map[string]any{
				"nodeId": fileID,
			})
		},
	}
	DeclareLeafMetadata(driveDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "delete_document",
				CanonicalPath:  "drive.delete_document",
				CLIPath:        "drive delete",
				PrimaryCLIPath: "drive delete",
			},
			Description: "将钉盘中的文件或文件夹移入回收站",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "delete_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将钉盘中的文件或文件夹移入回收站",
				UseWhen:      []string{"用户明确要求把钉盘/文档空间中的文件或文件夹移入回收站，且已确认目标节点时"},
				AvoidWhen: []string{
					"用户未确认删除目标或只是想移走位置时不要删；搬迁用 dws drive move",
					"要删整个知识库改用 dws wiki space delete",
					"要永久删评论改用 dws doc comment delete",
				},
				Examples: []string{"dws drive delete --node <dentryUuid> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
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
			// 服务端可能返回异步任务（taskId），此时自动轮询 query_task 直至终态。
			return runNodeTransferWithAsyncPoll(cmd.Context(), "copy_document", toolArgs)
		},
	}
	DeclareLeafMetadata(driveCopyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "copy_document",
				CanonicalPath:  "drive.copy_document",
				CLIPath:        "drive copy",
				PrimaryCLIPath: "drive copy",
			},
			Description: "将文件或文档复制到目标文件夹或知识库（保留原位置；默认「我的文档」）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "copy_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将文件或文档复制到目标文件夹或知识库（保留原位置；默认「我的文档」）",
				UseWhen: []string{
					"用户要复制/拷贝一份文件或文档到新位置，且原位置仍需保留副本时（copy≠move）",
					"目标是指定文件夹：传 --folder <目标文件夹 dentryUuid/fileId>",
					"目标是知识库根目录：只传 --workspace <workspaceId 或知识库 URL>，不传 --folder",
					"用户未指定目标时：默认落到当前组织「我的文档」；若需钉盘「我的文件」根目录，先 wiki space list --type mySpace 取 rootFolderId 再 --folder",
					"跨钉盘 space 复制到子文件夹：先 list 取目标文件夹 fileId，再 --folder 传入",
				},
				AvoidWhen: []string{
					"用户意图是搬走/迁移且原位置不再保留时改用 dws drive move（move 需更高源权限）",
					"只要快捷方式入口、不要独立副本时改用 dws drive shortcut",
					"已在明确知识库上下文内复制节点且需带 --workspace/--node 的 wiki 入口时可用 dws wiki node copy；跨产品默认用本命令",
					"目标文件夹或知识库尚未确认时不要复制",
				},
				Examples: []string{
					"dws drive copy --node <源dentryUuid> --folder <目标文件夹fileId> --format json",
					"dws drive copy --node <源dentryUuid> --workspace <TARGET_WS_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
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
			// 服务端可能返回异步任务（taskId），此时自动轮询 query_task 直至终态。
			return runNodeTransferWithAsyncPoll(cmd.Context(), "move_document", toolArgs)
		},
	}
	DeclareLeafMetadata(driveMoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "move_document",
				CanonicalPath:  "drive.move_document",
				CLIPath:        "drive move",
				PrimaryCLIPath: "drive move",
			},
			Description: "将文件或文档移动到目标文件夹或知识库（原位置不再保留）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "move_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将文件或文档移动到目标文件夹或知识库（原位置不再保留）",
				UseWhen: []string{
					"用户要移动/搬走文件或文档到新位置，且原位置不再保留时（move≠copy）",
					"目标文件夹用 --folder；目标知识库根用 --workspace；都不传则默认「我的文档」",
					"移动到其他钉盘 space 根目录时传目标 space 的 rootFolderId 到 --folder（通常不传 --workspace）",
				},
				AvoidWhen: []string{
					"需要保留原位置副本时改用 dws drive copy",
					"目标未确认或用户只是想复制时不要 move",
					"知识库内节点移动且走 wiki 入口时可用 dws wiki node move",
				},
				Examples: []string{
					"dws drive move --node <源dentryUuid> --folder <目标文件夹fileId> --format json",
					"dws drive move --node <源dentryUuid> --workspace <TARGET_WS_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	driveMoveCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	driveMoveCmd.Flags().String("folder", "", "目标文件夹 nodeId")
	driveMoveCmd.Flags().String("workspace", "", "目标知识库 ID")

	driveRenameCmd := &cobra.Command{
		Use:   "rename",
		Short: "重命名文件/文档",
		Long: `修改文档空间中文档、文件或文件夹的名称。
实际执行前会读取节点类型和当前扩展名：文件的新名称仅在末尾扩展名与当前扩展名一致时去掉一层，
避免 report.txt.txt；文件夹和扩展名不匹配的名称保持不变。dry-run 不读取远端元数据，因此保留输入名称。

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
			newName := flagOrFallback(cmd, "name", "title")
			if !commandDryRun(cmd) {
				newName, err = resolveDriveRenameName(cmd.Context(), nodeID, newName)
				if err != nil {
					return err
				}
			}
			return callMCPToolOnServer("doc", "rename_document", map[string]any{
				"nodeId":  nodeID,
				"newName": newName,
			})
		},
	}
	DeclareLeafMetadata(driveRenameCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "rename_document",
				CanonicalPath:  "drive.rename_document",
				CLIPath:        "drive rename",
				PrimaryCLIPath: "drive rename",
			},
			Description: "安全重命名文档空间中的文档、文件或文件夹，并按节点真实扩展名避免双后缀",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "rename_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "安全重命名文档空间中的文档、文件或文件夹，并按节点真实扩展名避免双后缀",
				UseWhen:      []string{"用户要重命名钉盘/文档空间中的文件、文档或文件夹时；实际执行会读取节点类型和当前扩展名，仅去掉完全匹配的一层后缀"},
				AvoidWhen: []string{
					"要改正文里的标题/章节 H1 改用 dws doc block update，不要用 rename",
					"只要复制或移动改用 copy/move",
					"dry-run 不读取节点元数据，输出名称尚未做基于当前扩展名的规范化",
				},
				Examples: []string{"dws drive rename --node <ID> --name \"新名称\" --format json"},
			},
			// Shared RPC rename_document with doc rename; keep this description
			// on the drive leaf only (doc rename does not strip extensions).
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "newName", Description: "新显示名称；实际执行前读取节点类型与当前扩展名，仅对非文件夹且末尾后缀与当前扩展名一致的名称去掉一层，避免双扩展名"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	driveRenameCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	driveRenameCmd.Flags().String("name", "", "新名称 (必填；实际执行时仅去掉与节点当前扩展名完全匹配的一层后缀)")

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
	DeclareLeafMetadata(driveStatsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_node_stats",
				CanonicalPath:  "drive.get_node_stats",
				CLIPath:        "drive stats",
				PrimaryCLIPath: "drive stats",
			},
			Description: "读取指定钉盘或文档空间节点的阅读、编辑、评论、点赞、预览与下载等统计。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "读取指定钉盘或文档空间节点的阅读、编辑、评论、点赞、预览与下载等统计。",
				UseWhen:      []string{"用户要查看节点阅读/编辑/评论/点赞/预览/下载等统计维度时"},
				AvoidWhen: []string{
					"要改文件内容或权限不要用本命令；本命令只读",
					"只要元信息（名称/类型）改用 dws drive info 或 dws doc info",
				},
				Examples: []string{"dws drive stats --node <NODE_ID_OR_URL> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	driveStatsCmd.Flags().String("node", "", "节点 ID 或文档 URL (必填)")

	// ── drive quota (查询企业存储容量) ──
	driveQuotaCmd := &cobra.Command{
		Use:   "quota",
		Short: "查询企业存储容量",
		Long: `查询企业存储容量，支持三个维度：
  不传参数               → 企业级总用量
  --app <appId>         → 应用级用量
  --space <spaceId>     → 空间级用量
--app 与 --space 互斥。`,
		Example: `  dws drive quota
  dws drive quota --app <appId>
  dws drive quota --space <spaceId>
  dws drive quota apps`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appID, _ := cmd.Flags().GetString("app")
			spaceID, _ := cmd.Flags().GetString("space")
			toolArgs := map[string]any{}
			if appID != "" {
				toolArgs["appId"] = appID
			}
			if spaceID != "" {
				toolArgs["spaceId"] = spaceID
			}
			return callMCPTool("get_storage_quota", toolArgs)
		},
	}
	driveQuotaCmd.Flags().String("app", "", "应用 ID (可选，与 --space 互斥)")
	driveQuotaCmd.Flags().String("space", "", "空间 ID (可选，与 --app 互斥)")
	driveQuotaCmd.MarkFlagsMutuallyExclusive("app", "space")
	DeclareLeafMetadata(driveQuotaCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_storage_quota",
				CanonicalPath:  "drive.get_storage_quota",
				CLIPath:        "drive quota",
				PrimaryCLIPath: "drive quota",
			},
			Description: "查询企业存储容量（企业级/应用级/空间级）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "get_storage_quota"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业存储容量（企业级/应用级/空间级）",
				UseWhen: []string{
					"用户问钉盘/企业盘存储容量、剩余空间时（企业级：不传参数）",
					"查询某个应用的存储用量时（--app）",
					"查询某个空间的存储用量时（--space）",
				},
				AvoidWhen: []string{
					"查应用列表及应用用量时用 dws drive quota apps",
					"查个人/单文件大小时用 drive info / drive stats",
				},
				Examples: []string{
					"dws drive quota --format json",
					"dws drive quota --app <APP_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "app", Property: "appId"},
				{Name: "space", Property: "spaceId"},
			},
		},
	})

	// ── drive quota apps (查询应用级存储用量列表) ──
	driveQuotaAppsCmd := &cobra.Command{
		Use:   "apps",
		Short: "查询应用级存储用量列表",
		Long: `查询企业下各应用的存储容量使用情况，返回应用列表及汇总用量。
支持分页和排序。`,
		Example: `  dws drive quota apps
  dws drive quota apps --limit 50
  dws drive quota apps --cursor <nextToken>
  dws drive quota apps --order-by used-quota --order desc`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// fail-fast 参数校验：无效值直接报错（帮助文本已声明合法值域），
			// 绝不静默丢弃或改写。--limit 用 Changed 区分显式传值与默认 20。
			if cmd.Flags().Changed("limit") {
				if v, _ := cmd.Flags().GetInt("limit"); v <= 0 || v > 50 {
					return fmt.Errorf("--limit 值无效：%d，必须为 1-50 之间的整数", v)
				}
			}
			if v, _ := cmd.Flags().GetString("order-by"); v != "" && mapOrderByToCamelCase(v) == "" {
				return fmt.Errorf("--order-by 值无效：%s，必须为 used-quota、standard-used-quota 或 exclusive-used-quota", v)
			}
			if v, _ := cmd.Flags().GetString("order"); v != "" && v != "asc" && v != "desc" {
				return fmt.Errorf("--order 值无效：%s，必须为 asc 或 desc", v)
			}

			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = float64(v)
			}
			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			if v, _ := cmd.Flags().GetString("order-by"); v != "" {
				mapped := mapOrderByToCamelCase(v)
				if mapped != "" {
					toolArgs["orderBy"] = mapped
				}
			}
			if v, _ := cmd.Flags().GetString("order"); v != "" {
				toolArgs["order"] = v
			}
			return callMCPTool("list_storage_apps", toolArgs)
		},
	}
	driveQuotaAppsCmd.Flags().Int("limit", 20, "每页返回数量，默认 20，最大 50")
	driveQuotaAppsCmd.Flags().String("cursor", "", "分页游标，从上次返回的 nextToken 获取 (可选)")
	driveQuotaAppsCmd.Flags().String("order-by", "", "排序字段：used-quota(总用量)/standard-used-quota(标准存储)/exclusive-used-quota(专属存储) (可选)")
	driveQuotaAppsCmd.Flags().String("order", "", "排序方向：asc/desc (可选，默认 desc)")
	DeclareLeafMetadata(driveQuotaAppsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "list_storage_apps",
				CanonicalPath:  "drive.list_storage_apps",
				CLIPath:        "drive quota apps",
				PrimaryCLIPath: "drive quota apps",
			},
			Description: "查询企业下各应用的存储用量列表（支持分页/排序）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "list_storage_apps"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询企业下各应用的存储用量列表（支持分页/排序）",
				UseWhen: []string{
					"盘点企业内哪些应用占用了钉盘存储、按用量排序找大户时",
					"翻页拉全应用用量列表时（--cursor 传上次 nextToken）",
				},
				AvoidWhen: []string{
					"只查单个应用的总量时用 dws drive quota --app",
				},
				Examples: []string{
					"dws drive quota apps --limit 50 --format json",
					"dws drive quota apps --order-by used-quota --order desc --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "limit", Property: "maxResults"},
				{Name: "order", Property: "order"},
				{Name: "order-by", Property: "orderBy"},
			},
		},
	})

	driveQuotaCmd.AddCommand(driveQuotaAppsCmd)
	newHybridGroupCommand(driveQuotaCmd)

	// ── drive task 子命令组（异步任务统一查询入口）──
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "异步任务状态查询（统一入口）",
		Long: `查询异步任务（导出/导入/复制/移动）的执行状态和结果，作为 drive 域异步任务的统一查询入口。

场景：
  - 导出任务超时或中断后，手动查询导出结果
  - 导入任务查询文件转换状态
  - 复制/移动异步任务超时后，手动查询任务状态

区分：
  dws drive task get   — 统一查询入口，支持 export/import/copy/move 多类型，返回归一化 TaskResult
  dws doc export get   — 产品级入口，仅支持导出任务，直接透传 MCP 原始响应`,
		Example: `  dws drive task get --type export --id <taskId>
  dws drive task get --type import --id <taskId>
  dws drive task get --type copy --id <taskId>
  dws drive task get --type move --id <taskId>`,
		RunE: groupRunE,
	}

	taskGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询单个异步任务状态",
		Long: `根据任务类型和 ID 查询单个异步任务的当前状态和结果。

任务状态：
  PENDING         排队中
  PROCESSING      处理中
  SUCCESS         任务成功，导出类型返回 resultUrl（下载链接）
  PARTIAL_FAILED  部分失败（终态，常见于批量复制/移动），返回部分失败说明
  FAILED          任务失败，返回错误信息
  TIMEOUT         任务超时`,
		Example: `  dws drive task get --type export --id <taskId>
  dws drive task get --type import --id <taskId>
  dws drive task get --type copy --id <taskId>
  dws drive task get --type move --id <taskId>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlags(cmd, "type", "id"); err != nil {
				return err
			}
			taskID := mustGetFlag(cmd, "id")
			taskType := mustGetFlag(cmd, "type")

			switch taskType {
			case "export", "import", "copy", "move":
			default:
				return fmt.Errorf("不支持的任务类型: %s，当前支持: export|import|copy|move", taskType)
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "查询异步任务")
				deps.Out.PrintKeyValue("类型", taskType)
				deps.Out.PrintKeyValue("ID", taskID)
				return nil
			}

			// query_task 工具注册在 drive (dingpan) MCP server 上，需显式路由。
			result, err := QueryTask(cmd.Context(), taskID, taskType)
			if err != nil {
				return err
			}
			return deps.Out.PrintJSON(result)
		},
	}
	taskGetCmd.Flags().String("type", "", "任务类型: export|import|copy|move (必填)")
	taskGetCmd.Flags().String("id", "", "任务 ID (必填)")
	DeclareLeafMetadata(taskGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "query_task",
				CanonicalPath:  "drive.query_task",
				CLIPath:        "drive task get",
				PrimaryCLIPath: "drive task get",
			},
			Description: "统一查询异步任务（export/import/copy/move）状态，返回归一化 TaskResult",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "query_task"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "统一查询异步任务（export/import/copy/move）状态，返回归一化 TaskResult",
				UseWhen: []string{
					"导出/导入任务超时或中断后手动查询结果时",
					"复制/移动异步任务超时后手动查询状态时",
				},
				AvoidWhen: []string{
					"仅在 doc 上下文查导出任务且需要 MCP 原始响应时用 dws doc export get",
					"任务尚未提交（没有 taskId）时先执行对应提交命令",
				},
				Examples: []string{
					"dws drive task get --type export --id <TASK_ID> --format json",
					"dws drive task get --type copy --id <TASK_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskId", Required: boolPtr(true)},
				{Name: "type", Property: "taskType", Required: boolPtr(true)},
			},
		},
	})
	taskCmd.AddCommand(taskGetCmd)
	newGroupCommand(taskCmd)

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
	DeclareLeafMetadata(driveShortcutCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "create_shortcut",
				CanonicalPath:  "drive.create_shortcut",
				CLIPath:        "drive shortcut",
				PrimaryCLIPath: "drive shortcut",
			},
			Description: "为钉盘或文档空间中的现有节点创建快捷方式。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为钉盘或文档空间中的现有节点创建快捷方式。",
				UseWhen:      []string{"用户要给已有节点建快捷方式/软链接到目标文件夹或知识库时"},
				AvoidWhen: []string{
					"要复制一份独立副本改用 dws drive copy（copy≠shortcut）",
					"要移动原文件改用 dws drive move",
				},
				Examples: []string{
					"dws drive shortcut --node <SOURCE_NODE> --format json",
					"dws drive shortcut --node <SOURCE_NODE> --folder <TARGET_FOLDER> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	driveShortcutCmd.Flags().String("node", "", "源节点 ID 或文档 URL (必填)")
	driveShortcutCmd.Flags().String("folder", "", "目标文件夹 nodeId (可选)")
	driveShortcutCmd.Flags().String("workspace", "", "目标知识库 ID (可选)")

	// ── drive permission (文档节点权限管理) ──
	drivePermissionCmd := newGroupCommand(&cobra.Command{
		Use:     "permission",
		Aliases: []string{"perm"},
		Short:   "文档节点权限管理",
		Long: `管理文档空间节点的协作权限：添加、更新、查询、移除协作者。
注意: 仅适用于文档空间节点，不适用于钉盘文件。`,
		RunE: groupRunE,
	})

	drivePermAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加协作者",
		Args:  cobra.NoArgs,
		Long: `为文档空间节点添加协作成员并授予指定角色。

两种传参方式（互斥）：
  旧格式：--users 传入逗号分隔的 userId 列表 + --role 指定统一角色（仅 USER 类型）
  新格式：--members 传入 JSON 数组，支持四种成员类型，每个 member 携带独立 roleId

成员类型说明：
  USER          用户，id 为用户 userId，需携带 corpId（标识用户所属组织）
  DEPT          部门，id 为部门 ID，需携带 corpId（标识部门所属组织）
  CONVERSATION  群聊，id 为群聊 conversationId（cid 开头），无需 corpId
  TAG           角色标签（也称角色组），id 为角色标签 ID，需携带 corpId。当用户要求"添加角色组"或"添加角色标签"时使用此类型

支持的角色: MANAGER / EDITOR / DOWNLOADER / READER
--notify 仅在 --members 新格式时生效，仅对 USER 和 CONVERSATION 类型成员发送通知（DEPT 和 TAG 不通知），默认 false。
省略 --notify 时 CLI 不向服务端发送该字段，服务端按不通知处理；需要通知请显式传 --notify。`,
		Example: `  dws drive permission add --node DOC_ID --users uid1 --role READER
  dws drive permission add --node DOC_ID --users uid1,uid2 --role EDITOR
  dws drive permission add --node DOC_ID --members '[{"type":"USER","id":"uid1","roleId":"READER","corpId":"xxx"}]' --notify
  dws drive permission add --node DOC_ID --members '[{"type":"CONVERSATION","id":"cidXXX","roleId":"READER"},{"type":"TAG","id":"tagId1","roleId":"EDITOR","corpId":"xxx"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateMembersExclusivity(cmd); err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			members, mErr := collectMembers(cmd, false)
			if mErr != nil {
				return mErr
			}
			if len(members) > 0 {
				toolArgs["members"] = members
				if cmd.Flags().Changed("notify") {
					notify, _ := cmd.Flags().GetBool("notify")
					toolArgs["notify"] = notify
				}
			} else {
				if err := validateRequiredFlags(cmd, "role"); err != nil {
					return err
				}
				userIds, err := collectUserIDs(cmd)
				if err != nil {
					return err
				}
				toolArgs["roleId"] = normalizePermissionRole(mustGetFlag(cmd, "role"))
				toolArgs["userIds"] = userIds
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "add_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(drivePermAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "add_permission",
				CanonicalPath:  "drive.add_permission",
				CLIPath:        "drive permission add",
				PrimaryCLIPath: "drive permission add",
			},
			Description: "为文档空间节点添加协作成员并授予指定角色",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "add_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为文档空间节点添加协作成员并授予指定角色",
				UseWhen: []string{
					"给单篇文档/文件夹/文件做节点级授权（USER + 角色 MANAGER/EDITOR/DOWNLOADER/READER）时",
					"用户说把某篇文档分享给某人看/可编辑时（含「我的文档」下的节点）",
				},
				AvoidWhen: []string{
					"给整个知识库加成员改用 dws wiki member add（「我的文档」不支持容器成员）",
					"只查已有权限改用 dws drive permission list",
					"改角色/移除分别用 permission update / remove",
				},
				Examples: []string{"dws drive permission add --node <ID> --users uid1,uid2 --role READER --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "members", Property: "members"},
				{Name: "node", Property: "nodeId"},
				{Name: "notify", Property: "notify"},
				{Name: "role", Property: "roleId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	drivePermAddCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermAddCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (旧格式)")
	drivePermAddCmd.Flags().String("user", "", "")
	_ = drivePermAddCmd.Flags().MarkHidden("user")
	drivePermAddCmd.Flags().String("role", "", "角色: MANAGER / EDITOR / DOWNLOADER / READER (旧格式必填)")
	drivePermAddCmd.Flags().String("workspace", "", "知识库 ID (选填)")
	drivePermAddCmd.Flags().String("members", "", "成员列表 JSON 数组（新格式），支持 USER/DEPT/CONVERSATION/TAG 类型（TAG=角色组），与 --users 互斥")
	drivePermAddCmd.Flags().Bool("notify", false, "是否通知被添加的成员（仅 --members 新格式时生效，需显式传入才通知）")

	drivePermUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新协作者权限",
		Args:  cobra.NoArgs,
		Long: `更新文档空间节点已有协作者的权限角色。

两种传参方式（互斥）：
  旧格式：--users 传入逗号分隔的 userId 列表 + --role 指定统一角色（仅 USER 类型）
  新格式：--members 传入 JSON 数组，支持四种成员类型，每个 member 携带独立 roleId

成员类型说明：
  USER          用户，id 为用户 userId，需携带 corpId
  DEPT          部门，id 为部门 ID，需携带 corpId
  CONVERSATION  群聊，id 为群聊 conversationId（cid 开头），无需 corpId
  TAG           角色标签（也称角色组），id 为角色标签 ID，需携带 corpId

支持的角色: MANAGER / EDITOR / DOWNLOADER / READER
--notify 仅在 --members 新格式时生效，仅对 USER 和 CONVERSATION 类型成员发送通知，默认 false。`,
		Example: `  dws drive permission update --node DOC_ID --users uid1 --role EDITOR
  dws drive permission update --node DOC_ID --members '[{"type":"USER","id":"uid1","roleId":"EDITOR","corpId":"xxx"}]' --notify=false
  dws drive permission update --node DOC_ID --members '[{"type":"TAG","id":"tagId1","roleId":"READER","corpId":"xxx"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateMembersExclusivity(cmd); err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			members, mErr := collectMembers(cmd, false)
			if mErr != nil {
				return mErr
			}
			if len(members) > 0 {
				toolArgs["members"] = members
				if cmd.Flags().Changed("notify") {
					notify, _ := cmd.Flags().GetBool("notify")
					toolArgs["notify"] = notify
				}
			} else {
				if err := validateRequiredFlags(cmd, "role"); err != nil {
					return err
				}
				userIds, err := collectUserIDs(cmd)
				if err != nil {
					return err
				}
				toolArgs["roleId"] = normalizePermissionRole(mustGetFlag(cmd, "role"))
				toolArgs["userIds"] = userIds
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "update_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(drivePermUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "permission_update",
				CanonicalPath:  "drive.permission_update",
				CLIPath:        "drive permission update",
				PrimaryCLIPath: "drive permission update",
			},
			Description: "更新文档空间节点已有协作者的权限角色",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "update_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新文档空间节点已有协作者的权限角色",
				UseWhen:      []string{"调整已有节点成员角色（如 READER→EDITOR）时"},
				AvoidWhen: []string{
					"成员尚无权限时改用 permission add",
					"要移除访问改用 permission remove",
				},
				Examples: []string{"dws drive permission update --node <ID> --users uid1 --role EDITOR --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "members", Property: "members"},
				{Name: "node", Property: "nodeId"},
				{Name: "notify", Property: "notify"},
				{Name: "role", Property: "roleId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	drivePermUpdateCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermUpdateCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (旧格式)")
	drivePermUpdateCmd.Flags().String("user", "", "")
	_ = drivePermUpdateCmd.Flags().MarkHidden("user")
	drivePermUpdateCmd.Flags().String("role", "", "新角色: MANAGER / EDITOR / DOWNLOADER / READER (旧格式必填)")
	drivePermUpdateCmd.Flags().String("workspace", "", "知识库 ID (选填)")
	drivePermUpdateCmd.Flags().String("members", "", "成员列表 JSON 数组（新格式），支持 USER/DEPT/CONVERSATION/TAG 类型（TAG=角色组），与 --users 互斥")
	drivePermUpdateCmd.Flags().Bool("notify", false, "是否通知被变更的成员（仅 --members 新格式时生效）")

	drivePermListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查询协作者列表",
		Long: `查询文档空间节点的协作者列表，支持分页和角色过滤。

底层一次性返回全量成员后在内存中按 pageSize 分页，支持通过 nextToken 翻页。
出参包含 totalCount、hasMore 和 nextToken。
当 hasMore 为 true 时，传入下一次请求的 --next-token 即可获取下一页。`,
		Example: `  dws drive permission list --node DOC_ID
  dws drive permission list --node DOC_ID --limit 50 --filter-role MANAGER,EDITOR
  dws drive permission list --node DOC_ID --next-token <上次返回的 nextToken>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if size, ok, err := permissionPageSizeFromFlags(cmd); err != nil {
				return err
			} else if ok {
				toolArgs["pageSize"] = size
			}
			if v := flagOrFallback(cmd, "next-token", "cursor", "page-token"); v != "" {
				toolArgs["nextToken"] = v
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
	DeclareLeafMetadata(drivePermListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "list_permission",
				CanonicalPath:  "drive.list_permission",
				CLIPath:        "drive permission list",
				PrimaryCLIPath: "drive permission list",
			},
			Description: "查询文档空间节点的协作者列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "list_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询文档空间节点的协作者列表",
				UseWhen:      []string{"查看某文档/文件夹节点当前有哪些成员及角色时"},
				AvoidWhen: []string{
					"要新增/修改/移除权限分别用 permission add/update/remove",
					"查知识库容器成员改用 dws wiki member list",
				},
				Examples: []string{"dws drive permission list --node <ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "filter-role", Property: "filterRoleIds"},
				// limit 不声明 Property：运行时经 cap 校验（1-50）转换为 pageSize，
				// 属 CLI 分页输入而非 1:1 RPC property（reviewed mapping exclusion）。
				{Name: "limit"},
				{Name: "next-token", Property: "nextToken"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "next-token"},
		},
	})
	drivePermListCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermListCmd.Flags().Int("limit", 30, "返回成员数上限，默认 30，最大 50")
	drivePermListCmd.Flags().Int("max-results", 0, "")
	_ = drivePermListCmd.Flags().MarkHidden("max-results")
	drivePermListCmd.Flags().String("filter-role", "", "按角色过滤: OWNER / MANAGER / EDITOR / DOWNLOADER / READER")
	drivePermListCmd.Flags().String("next-token", "", "分页游标，首次不传，后续传入上一次返回的 nextToken")
	drivePermListCmd.Flags().String("workspace", "", "知识库 ID (选填)")

	drivePermGetSettingCmd := &cobra.Command{
		Use:   "get-setting",
		Short: "查询节点权限设置",
		Long: `查询文档空间节点的权限设置，返回三部分配置：

- permissionMode: 权限模式（INHERITED 继承上级 / INDEPENDENT 独立管理）
- shareScope: 分享范围（可见范围、链接分享设置）
- policies: 权限策略列表（水印、组织外分享、成员邀请门槛等）

查询协作者列表请改用 permission list。`,
		Example: `  dws drive permission get-setting --node DOC_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "get_permission_setting", map[string]any{"nodeId": nodeID})
		},
	}
	DeclareLeafMetadata(drivePermGetSettingCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_permission_setting",
				CanonicalPath:  "drive.get_permission_setting",
				CLIPath:        "drive permission get-setting",
				PrimaryCLIPath: "drive permission get-setting",
			},
			Description: "查询文档空间节点的权限设置（权限模式/分享范围/权限策略）",
			Result: &contract.ResultSpec{
				Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{
  "type":"object",
  "description":"节点权限设置（权限模式/分享范围/权限策略）",
  "properties":{
    "docUrl":{"type":"string","description":"当前查询节点的文档访问链接，可直接在浏览器中打开"},
    "nodeId":{"type":"string","description":"当前查询节点的 nodeId（入参解析后的规范形式）"},
    "permissionMode":{"type":["string","null"],"enum":["INHERITED","INDEPENDENT",null],"description":"权限模式：INHERITED=继承上级权限配置，INDEPENDENT=独立管理权限；未知时为 null"},
    "shareScope":{
      "type":"object",
      "description":"分享范围设置",
      "properties":{
        "visibility":{"type":["string","null"],"enum":["PRIVATE","ORGANIZATION","PUBLIC",null],"description":"PRIVATE=仅指定成员可见，ORGANIZATION=组织内公开，PUBLIC=互联网公开；未知时为 null"},
        "partnerIncluded":{"type":"boolean","description":"仅 visibility=ORGANIZATION 时有意义，true 表示组织内公开范围包含合作伙伴（含生态组织外部协作成员）。其余场景为 false。"},
        "defaultRole":{"type":["string","null"],"enum":["READER","DOWNLOADER","EDITOR","MANAGER",null],"description":"仅 visibility=ORGANIZATION 时有意义，通过链接获得访问的默认角色；未下发或不在值域内时为 null"},
        "canSearch":{"type":"boolean","description":"仅 visibility=ORGANIZATION 时有意义。"},
        "canRecommend":{"type":"boolean","description":"仅 visibility=ORGANIZATION 时有意义。"},
        "linkShare":{
          "type":"object",
          "description":"链接分享设置；仅开启链接分享时返回，未开启时该字段不返回",
          "properties":{
            "requirePassword":{"type":"boolean","description":"true 表示通过链接访问需要提供密码。密码明文不会返回。"},
            "expireAt":{"type":["integer","null"],"description":"秒级 Unix 时间戳，未设置过期时为 null。"},
            "expireDays":{"type":["integer","null"],"description":"设置的有效天数，未设置时为 null。"},
            "forCurrentNode":{"type":"boolean","description":"true 表示该分享范围仅作用于当前节点；false 表示作用于当前节点及其子节点。"}
          },
          "additionalProperties":true
        }
      },
      "additionalProperties":true
    },
    "policies":{
      "type":"array",
      "description":"仅包含支持的策略项，未下发或不受支持的策略不会返回；node_spread_scope 仅文件夹类节点返回；allowedValues 为当前可设置的取值，disabledValues 为当前不可设置的取值及原因，两者互斥",
      "items":{
        "type":"object",
        "description":"权限策略项",
        "properties":{
          "code":{"type":"string","enum":["external_share","external_share_manager_only","member_invite","member_invite_org_only","comment","permission_apply","external_permission_apply","watermark","node_spread","online_content_copy","node_move_forbidden","node_spread_scope"],"description":"external_share=添加企业外协作者；external_share_manager_only=企业外协作者仅限管理员；member_invite=谁可以添加协作者；member_invite_org_only=仅企业内用户可添加协作者；comment=谁可以评论；permission_apply=权限申请；external_permission_apply=组织外权限申请；watermark=显示水印；node_spread=谁可以下载、创建副本、打印；online_content_copy=谁可以复制文档内容；node_move_forbidden=禁止移动；node_spread_scope=下载与传播生效范围（仅文件夹类节点）。"},
          "name":{"type":"string","description":"策略的中文名称，文案与产品权限设置页一致；为确定性字段，只要该策略返回就必带"},
          "description":{"type":"string","description":"策略的含义说明，解释该策略管控的行为及各取值的语义；为确定性字段，只要该策略返回就必带"},
          "value":{"type":["string","null"],"description":"取值随策略类型不同：开关型（external_share、external_share_manager_only、member_invite_org_only、permission_apply、external_permission_apply、watermark、node_move_forbidden）为 ENABLED/DISABLED；阈值型（member_invite、comment）为 READER_AND_ABOVE/DOWNLOADER_AND_ABOVE/EDITOR_AND_ABOVE/MANAGER_AND_ABOVE，阈值型（node_spread、online_content_copy）为 DOWNLOADER_AND_ABOVE/EDITOR_AND_ABOVE/MANAGER_AND_ABOVE/NOBODY，均表示不低于该角色才允许对应操作，NOBODY 表示所有人禁止；二值型（node_spread_scope）：ALL_NODES=下载与传播限制对所有文档生效，PREVIEWABLE_ONLY=仅对可预览的文档（在线文档、图片视频等）生效；未知值时为 null"},
          "disabledValues":{
            "type":"array",
            "description":"该策略当前不可设置的取值及禁用原因（与 allowedValues 互斥）；为确定性字段，恒返回，无被禁取值时为空数组",
            "items":{
              "type":"object",
              "properties":{
                "value":{"type":"string","description":"被禁档位的取值（与 value 同一值域）"},
                "reason":{"type":["string","null"],"description":"服务端按请求语言返回的禁用原因文案，仅供展示理解，可为 null"}
              },
              "required":["value"],
              "additionalProperties":true
            }
          },
          "allowedValues":{"type":["array","null"],"items":{"type":"string"},"description":"该策略当前可设置的取值（与 value 同一值域），未下发时为 null"}
        },
        "required":["code","name","description","disabledValues"],
        "additionalProperties":true
      }
    }
  },
  "required":["docUrl","nodeId","shareScope","policies"],
  "additionalProperties":true
}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "get_permission_setting"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询文档空间节点的权限设置（权限模式/分享范围/权限策略）",
				UseWhen:      []string{"查看节点权限模式/分享范围/水印等权限策略配置时"},
				AvoidWhen: []string{
					"查协作者清单用 permission list",
					"查可申请角色与审批人用 permission apply-info",
				},
				Examples: []string{"dws drive permission get-setting --node <ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	drivePermGetSettingCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")

	drivePermRemoveCmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "移除协作者权限",
		Long: `从文档空间节点移除协作成员的权限。

两种传参方式（互斥）：
  旧格式：--users 传入逗号分隔的 userId 列表（仅 USER 类型）
  新格式：--members 传入 JSON 数组，支持四种成员类型，只需 type 和 id（USER/DEPT/TAG 还需 corpId）

成员类型说明：
  USER          用户，id 为用户 userId，需携带 corpId
  DEPT          部门，id 为部门 ID，需携带 corpId
  CONVERSATION  群聊，id 为群聊 conversationId（cid 开头），无需 corpId
  TAG           角色标签（也称角色组），id 为角色标签 ID，需携带 corpId`,
		Example: `  dws drive permission remove --node DOC_ID --users uid1
  dws drive permission remove --node DOC_ID --users uid1,uid2
  dws drive permission remove --node DOC_ID --members '[{"type":"USER","id":"uid1","corpId":"xxx"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateMembersExclusivity(cmd); err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			members, mErr := collectMembers(cmd, true)
			if mErr != nil {
				return mErr
			}
			if len(members) > 0 {
				toolArgs["members"] = members
			} else {
				userIds, err := collectUserIDs(cmd)
				if err != nil {
					return err
				}
				toolArgs["userIds"] = userIds
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "remove_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(drivePermRemoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			// 批量移除（最多 30 个 USER/DEPT/CONVERSATION/TAG）会一次性撤销多个
			// 成员的访问，部门/群聊/角色组还可能间接影响大量用户，与删除同级的
			// destructive 入口，必须经过用户确认（--yes 或交互 yes）。
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "permission_remove",
				CanonicalPath:  "drive.permission_remove",
				CLIPath:        "drive permission remove",
				PrimaryCLIPath: "drive permission remove",
			},
			Description: "从文档空间节点移除协作成员的权限",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "remove_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "从文档空间节点移除协作成员的权限",
				UseWhen:      []string{"从节点移除指定用户的直接授权时"},
				AvoidWhen: []string{
					"要改角色用 permission update；要加权限用 add",
					"移除知识库容器成员用 dws wiki member remove",
				},
				Examples: []string{"dws drive permission remove --node <ID> --users uid1 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "members", Property: "members"},
				{Name: "node", Property: "nodeId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	drivePermRemoveCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermRemoveCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔 (旧格式)")
	drivePermRemoveCmd.Flags().String("user", "", "")
	_ = drivePermRemoveCmd.Flags().MarkHidden("user")
	drivePermRemoveCmd.Flags().String("members", "", "成员列表 JSON 数组（新格式），只需 type 和 id（USER/DEPT/TAG 还需 corpId），与 --users 互斥")
	drivePermRemoveCmd.Flags().String("workspace", "", "知识库 ID (选填)")

	// permission 子命令 --node 隐藏别名（保持与迁移前 doc 命令一致）
	for _, c := range []*cobra.Command{drivePermAddCmd, drivePermUpdateCmd, drivePermListCmd, drivePermGetSettingCmd, drivePermRemoveCmd} {
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

	drivePermTransferOwnerCmd := &cobra.Command{
		Use:   "transfer-owner",
		Short: "[危险] 转交所有者",
		Long: `转交文档或知识库的所有者给指定用户。此操作不可逆，执行前需要确认。

--node 和 --workspace 二选一。转交后原所有者保留角色由 --reserve-role 指定。
使用 --yes 跳过确认时，--reserve-role 和 --recursive 必须显式指定。`,
		Example: `  dws drive permission transfer-owner --node DOC_ID --new-owner uid123 --reserve-role EDITOR --recursive=false --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, _ := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			workspaceID := flagOrFallback(cmd, "workspace", "workspace-id")
			if nodeID == "" && workspaceID == "" {
				return fmt.Errorf("--node or --workspace is required")
			}
			// 不可逆高危操作：帮助文档承诺二选一，两者同传直接失败，绝不静默择一。
			if nodeID != "" && workspaceID != "" {
				return fmt.Errorf("--node and --workspace are mutually exclusive; specify exactly one")
			}
			if err := validateRequiredFlags(cmd, "new-owner"); err != nil {
				return err
			}
			newOwnerID := mustGetFlag(cmd, "new-owner")

			yesMode, _ := cmd.Flags().GetBool("yes")
			if yesMode {
				if !cmd.Flags().Changed("reserve-role") {
					return fmt.Errorf("--reserve-role is required when using --yes")
				}
				if !cmd.Flags().Changed("recursive") {
					return fmt.Errorf("--recursive is required when using --yes")
				}
			}

			if commandDryRun(cmd) {
				if deps.Caller.Format() == "json" {
					payload := map[string]any{
						"dry_run":    true,
						"executed":   false,
						"operation":  "转交所有者",
						"newOwnerId": newOwnerID,
					}
					if nodeID != "" {
						payload["nodeId"] = nodeID
					} else {
						payload["workspaceId"] = workspaceID
					}
					return deps.Out.PrintJSON(payload)
				}
				deps.Out.PrintKeyValue("操作", "转交所有者")
				deps.Out.PrintKeyValue("新所有者", newOwnerID)
				return nil
			}

			reserveRole := mustGetFlag(cmd, "reserve-role")
			recursive, _ := cmd.Flags().GetBool("recursive")

			target := nodeID
			if target == "" {
				target = workspaceID
			}

			toolArgs := map[string]any{"newOwnerId": newOwnerID}
			if nodeID != "" {
				toolArgs["nodeId"] = nodeID
			} else {
				toolArgs["workspaceId"] = workspaceID
			}
			if reserveRole != "" {
				toolArgs["reserveOldOwnerRole"] = reserveRole
			}
			if cmd.Flags().Changed("recursive") {
				toolArgs["recursiveChange"] = recursive
			}
			return callMCPToolOnServer("doc", "transfer_owner", toolArgs)
		},
	}
	DeclareLeafMetadata(drivePermTransferOwnerCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "transfer_owner",
				CanonicalPath:  "drive.transfer_owner",
				CLIPath:        "drive permission transfer-owner",
				PrimaryCLIPath: "drive permission transfer-owner",
			},
			Description: "转交文档或知识库所有者给指定用户（不可逆）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "转交文档或知识库所有者给指定用户（不可逆）",
				UseWhen:      []string{"用户明确要求转交文档/知识库所有权"},
				AvoidWhen:    []string{"普通协作权限变更用 permission add/update/remove"},
				Examples:     []string{"dws drive permission transfer-owner --node <DOC_ID> --new-owner <userId> --reserve-role EDITOR --recursive=false --format json"},
			},
		},
	})
	drivePermTransferOwnerCmd.Flags().String("node", "", "目标节点 ID 或 URL（与 --workspace 二选一）")
	drivePermTransferOwnerCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL（与 --node 二选一）")
	drivePermTransferOwnerCmd.Flags().String("new-owner", "", "新所有者的用户 userId (必填)")
	drivePermTransferOwnerCmd.Flags().String("reserve-role", "", "转交后原所有者保留角色: MANAGER / EDITOR / DOWNLOADER / READER / NONE")
	drivePermTransferOwnerCmd.Flags().Bool("recursive", false, "是否递归变更所有子节点的所有者")

	drivePermApplyInfoCmd := &cobra.Command{
		Use:     "apply-info",
		Short:   "查询节点可申请的角色与审批人",
		Example: `  dws drive permission apply-info --node DOC_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "query_permission_apply_info", map[string]any{"nodeId": nodeID})
		},
	}
	DeclareLeafMetadata(drivePermApplyInfoCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "query_permission_apply_info",
				CanonicalPath:  "drive.query_permission_apply_info",
				CLIPath:        "drive permission apply-info",
				PrimaryCLIPath: "drive permission apply-info",
			},
			Description: "查询节点可申请的权限角色列表与审批人列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询节点可申请的权限角色列表与审批人列表",
				UseWhen:      []string{"无权限访问文档时，先查可申请角色与审批人"},
				AvoidWhen:    []string{"实际发起申请用 apply_permission"},
				Examples:     []string{"dws drive permission apply-info --node <DOC_ID> --format json"},
			},
		},
	})
	drivePermApplyInfoCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")

	drivePermApplyCmd := &cobra.Command{
		Use:   "apply",
		Short: "发起权限申请",
		Long: `向指定节点的审批人发起权限申请。建议先用 apply-info 获取可申请角色与审批人。

注意: 本命令会真实通知审批人，Agent 必须先获得用户明确同意后再执行。`,
		Example: `  dws drive permission apply --node DOC_ID --role READER --users uid1
  dws drive permission apply --node DOC_ID --role EDITOR --users uid1,uid2 --reason "需要编辑该文档"`,
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
				"nodeId":    nodeID,
				"roleId":    normalizePermissionRole(mustGetFlag(cmd, "role")),
				"receivers": userIds,
			}
			if v := mustGetFlag(cmd, "notify-mode"); v != "" {
				toolArgs["notifyMode"] = v
			}
			if v := mustGetFlag(cmd, "reason"); v != "" {
				toolArgs["reason"] = v
			}
			return callMCPToolOnServer("drive", "apply_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(drivePermApplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "apply_permission",
				CanonicalPath:  "drive.apply_permission",
				CLIPath:        "drive permission apply",
				PrimaryCLIPath: "drive permission apply",
			},
			Description: "向审批人发起文档权限申请（会真实通知审批人）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "向审批人发起文档权限申请（会真实通知审批人）",
				UseWhen:      []string{"用户确认要为无权限文档发起权限申请"},
				AvoidWhen:    []string{"先用 apply-info 查可申请角色与审批人；未经用户确认不得自行提交"},
				Examples:     []string{"dws drive permission apply --node <DOC_ID> --role READER --users <审批人userId> --format json"},
			},
		},
	})
	drivePermApplyCmd.Flags().String("node", "", "目标节点 ID 或 URL (必填)")
	drivePermApplyCmd.Flags().String("role", "", "申请的角色: EDITOR / DOWNLOADER / READER (必填)")
	drivePermApplyCmd.Flags().String("users", "", "审批人 userId 列表，逗号分隔 (必填)")
	drivePermApplyCmd.Flags().String("user", "", "")
	_ = drivePermApplyCmd.Flags().MarkHidden("user")
	drivePermApplyCmd.Flags().String("notify-mode", "", "通知方式: DEFAULT / MSG_ACCOUNT / SINGLE_CHAT")
	drivePermApplyCmd.Flags().String("reason", "", "申请理由，最长 200 字符")

	drivePermissionCmd.AddCommand(drivePermAddCmd, drivePermUpdateCmd, drivePermListCmd, drivePermGetSettingCmd, drivePermRemoveCmd, drivePermTransferOwnerCmd, drivePermApplyInfoCmd, drivePermApplyCmd)

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
	recycleCmd := newGroupCommand(&cobra.Command{
		Use:   "recycle",
		Short: "钉盘回收站管理",
		Long:  `管理钉盘回收站：查看回收站列表、还原回收项。`,
		RunE:  groupRunE,
	})

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
	DeclareLeafMetadata(recycleListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "recycle_list",
				CanonicalPath:  "drive.recycle_list",
				CLIPath:        "drive recycle list",
				PrimaryCLIPath: "drive recycle list",
			},
			Description: "查看回收站文件列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查看回收站文件列表",
				UseWhen:      []string{"用户要查看钉盘回收站里有哪些已删文件时"},
				AvoidWhen: []string{
					"要从回收站还原改用 dws drive recycle restore",
					"要删除进回收站改用 dws drive delete",
				},
				Examples: []string{
					"dws drive recycle list --format json",
					"dws drive recycle list --space-id 12345 --limit 10 --format json",
				},
			},
		},
	})
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
	DeclareLeafMetadata(recycleRestoreCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "recycle_restore",
				CanonicalPath:  "drive.recycle_restore",
				CLIPath:        "drive recycle restore",
				PrimaryCLIPath: "drive recycle restore",
			},
			Description: "还原回收站中的文件",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "还原回收站中的文件",
				UseWhen:      []string{"用户要从回收站还原指定回收项（已从 recycle list 拿到 id）时"},
				AvoidWhen: []string{
					"尚未确认还原哪一项时先 dws drive recycle list",
					"要删除文件改用 dws drive delete，不要用 restore",
				},
				Examples: []string{"dws drive recycle restore --id <recycleItemId> --format json"},
			},
		},
	})
	recycleRestoreCmd.Flags().String("id", "", "回收项 ID (必填，从 recycle list 获取)")

	recycleCmd.AddCommand(recycleListCmd, recycleRestoreCmd)

	// ── deprecated 代理命令（Phase 2：从 doc 迁移，保留兼容，警告引导到新命令）──

	// folder create → dws wiki node create --type folder
	driveFolderCmd := newGroupCommand(&cobra.Command{Use: "folder", Short: "文件夹管理（deprecated）", RunE: groupRunE})
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
	drivePublishCmd := newGroupCommand(&cobra.Command{
		Use:   "publish",
		Short: "文件互联网公开发布管理",
		Long:  `管理文件的互联网公开发布状态：设置公开、关闭公开、查询公开状态。`,
		RunE:  groupRunE,
	})

	drivePublishSetCmd := &cobra.Command{
		Use:   "set",
		Short: "[危险] 设置文件为互联网公开",
		Long: `[危险] 将文件设置为互联网公开发布。公开后任何人通过链接即可访问，无需登录钉钉。
操作者需要是该文件的管理员或拥有者。执行前需要确认，或传入 --yes 跳过确认。

公开权限 (--permission): READER(仅可查看) / DOWNLOADER(可查看和下载，默认) / EDITOR(可编辑)
访问密码 (--password): 4位英文字母+数字（如 Ab12）。设置后访问公开链接需输入密码。
  显式传空（--password ""）可关闭已有密码保护；不传则不改变密码设置。
有效期 (--expire-days): 正整数=N天后过期，0=永久有效，不传=保持原值不变，负数会报错。
注意：密码和有效期的支持情况取决于节点类型和组织策略，不支持时服务端会返回友好提示。`,
		Example: `  dws drive publish set --node <fileId> --format json
  dws drive publish set --node <fileId> --password Ab12 --expire-days 7 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// LeafSpec.Validate 已在本 RunE 之前（先于确认门）完成 node/permission/
			// password/expire-days 校验，这里直接取值装配参数。
			nodeID := flagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			permVal := mustGetFlag(cmd, "permission")

			// 密码操作类型（三态：keep / set / clear）
			pwdChanged := cmd.Flags().Changed("password")
			pwdVal := mustGetFlag(cmd, "password")
			expireChanged := cmd.Flags().Changed("expire-days")

			toolArgs := map[string]any{
				"fileId":    nodeID,
				"published": true,
			}
			if permVal != "" {
				toolArgs["publishPermission"] = permVal
			}

			// 密码三值语义：传非空=设置密码，传空=清除密码，没传=不修改
			if pwdChanged {
				if pwdVal == "" {
					toolArgs["requirePassword"] = false
				} else {
					toolArgs["requirePassword"] = true
					toolArgs["password"] = pwdVal
				}
			}

			// expireDays 三值：0=永久, N=N天（负数已在 Validate fail-fast 拦截）
			if expireChanged {
				expireDaysVal, _ := cmd.Flags().GetInt("expire-days")
				toolArgs["expireDays"] = expireDaysVal
			}
			return callMCPTool("set_file_publish", toolArgs)
		},
	}
	DeclareLeafMetadata(drivePublishSetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			if err != nil {
				return err
			}
			// 以下三项校验与 node 校验同处 Validate（RunE 包装器内先于
			// ConfirmSafety 执行）：非法参数在触发确认或远端调用之前 fail-fast。

			// permission 枚举校验（fail-fast）
			permVal := mustGetFlag(cmd, "permission")
			if permVal != "" {
				validPermissions := map[string]bool{"READER": true, "DOWNLOADER": true, "EDITOR": true}
				if !validPermissions[permVal] {
					return fmt.Errorf("--permission 值无效：%s，必须为 READER、DOWNLOADER 或 EDITOR", permVal)
				}
			}

			// 密码格式校验（三态中的 set：非空才校验；空串=清除密码，合法）
			if cmd.Flags().Changed("password") {
				if pwdVal := mustGetFlag(cmd, "password"); pwdVal != "" {
					if !regexp.MustCompile(`^[A-Za-z0-9]{4}$`).MatchString(pwdVal) {
						return fmt.Errorf("密码必须为 4 位字母或数字组合（如 ab3D）")
					}
				}
			}

			// 负数有效期会导致 expireDays 字段缺失，服务端 PUT 语义下被设为永久公开。
			if cmd.Flags().Changed("expire-days") {
				if expireDaysVal, _ := cmd.Flags().GetInt("expire-days"); expireDaysVal < 0 {
					return fmt.Errorf("--expire-days 不能为负数，请传入正整数（如 7）或 0（表示永久有效）")
				}
			}
			return nil
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "publish_set",
				CanonicalPath:  "drive.publish_set",
				CLIPath:        "drive publish set",
				PrimaryCLIPath: "drive publish set",
			},
			Description: "开启文件的互联网公开发布",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "开启文件的互联网公开发布",
				UseWhen:      []string{"用户明确要求将文件设为互联网公开（任何人凭链接可访问，无需登录）时"},
				AvoidWhen: []string{
					"只查公开状态用 publish get；要关闭公开用 publish unset",
					"目标文件或公开权限范围未确认时不要开启",
					"只要企业内部同事权限用 permission add，不要用互联网公开",
				},
				Examples: []string{
					"dws drive publish set --node <fileId> --format json",
					"dws drive publish set --node <fileId> --permission READER --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "password", Property: "password"},
				{Name: "expire-days", Property: "expireDays"},
			},
		},
	})
	drivePublishSetCmd.Flags().String("node", "", "目标文件 ID (dentryUuid) 或 URL (必填)")
	drivePublishSetCmd.Flags().String("permission", "", "公开后的权限: READER / DOWNLOADER(默认) / EDITOR")
	drivePublishSetCmd.Flags().String("password", "", "访问密码：传非空值设置/修改密码，传空字符串清除密码，不传则不改变")
	drivePublishSetCmd.Flags().Int("expire-days", 0, "公开有效期天数：0 表示永久有效")

	drivePublishUnsetCmd := &cobra.Command{
		Use:     "unset",
		Aliases: []string{"off", "close"},
		Short:   "[危险] 关闭文件互联网公开",
		Long:    `[危险] 关闭文件的互联网公开发布。关闭后外部用户将无法再通过链接访问。执行前需要确认，或传入 --yes 跳过确认。`,
		Example: `  dws drive publish unset --node <fileId> --yes
  dws drive publish unset --node <fileId>          # 交互式确认`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// LeafSpec.Validate 已在本 RunE 之前确认 node 非空，这里直接取值。
			nodeID := flagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			return callMCPTool("set_file_publish", map[string]any{
				"fileId":    nodeID,
				"published": false,
			})
		},
	}
	DeclareLeafMetadata(drivePublishUnsetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Validate: func(cmd *cobra.Command, args []string) error {
			_, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "file-id")
			return err
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "publish_unset",
				CanonicalPath:  "drive.publish_unset",
				CLIPath:        "drive publish unset",
				PrimaryCLIPath: "drive publish unset",
			},
			Description: "关闭文件的互联网公开发布",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "关闭文件的互联网公开发布",
				UseWhen:      []string{"用户明确要求关闭文件的互联网公开发布，使外部链接失效时"},
				AvoidWhen: []string{
					"只查状态用 publish get；要开启用 publish set",
					"目标文件未确认时不要关闭",
				},
				Examples: []string{"dws drive publish unset --node <fileId> --format json"},
			},
		},
	})
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
	DeclareLeafMetadata(drivePublishGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "publish_get",
				CanonicalPath:  "drive.publish_get",
				CLIPath:        "drive publish get",
				PrimaryCLIPath: "drive publish get",
			},
			Description: "查询文件当前是否处于互联网公开发布状态",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询文件当前是否处于互联网公开发布状态",
				UseWhen:      []string{"查询文件是否已互联网公开发布及公开权限（READER/DOWNLOADER/EDITOR）时"},
				AvoidWhen: []string{
					"要开启公开改用 dws drive publish set（需确认）",
					"要关闭公开改用 dws drive publish unset（需确认）",
				},
				Examples: []string{"dws drive publish get --node <fileId> --format json"},
			},
		},
	})
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
		driveListCmd, driveListSpacesCmd, driveInfoCmd, driveDownloadCmd, driveDownloadVersionCmd,
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
	DeclareLeafMetadata(driveRecentCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "recent",
				CanonicalPath:  "drive.recent",
				CLIPath:        "drive recent",
				PrimaryCLIPath: "drive recent",
			},
			Description: "获取当前用户最近访问或编辑过的文档列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前用户最近访问或编辑过的文档列表",
				UseWhen: []string{
					"用户要看最近访问或最近编辑的文档列表时（默认最近访问 operate-type=0）",
					"需要按我创建/他人创建或文档类型过滤最近项时",
				},
				AvoidWhen: []string{
					"按关键词全局搜文件改用 dws drive search",
					"浏览某目录内容改用 dws drive list",
				},
				Examples: []string{
					"dws drive recent --format json",
					"dws drive recent --operate-type 1 --format json",
				},
			},
		},
	})
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

	// ── drive star (文档收藏管理) ──
	driveStarCmd := newGroupCommand(&cobra.Command{
		Use:   "star",
		Short: "文档收藏管理",
		RunE:  groupRunE,
	})
	driveStarAddCmd := &cobra.Command{
		Use:     "add",
		Short:   "收藏文档",
		Example: `  dws drive star add --node <nodeId_or_URL>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "mark_star", map[string]any{"nodeId": nodeID})
		},
	}
	DeclareLeafMetadata(driveStarAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "mark_star",
				CanonicalPath:  "drive.mark_star",
				CLIPath:        "drive star add",
				PrimaryCLIPath: "drive star add",
			},
			Description: "收藏文档/文件到当前用户收藏列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "收藏文档/文件到当前用户收藏列表",
				UseWhen:      []string{"用户说 收藏这个文档/加个收藏/标星"},
				AvoidWhen:    []string{"取消收藏用 unmark_star；查看收藏列表用 get_star_list"},
				Examples:     []string{"dws drive star add --node <nodeId> --format json"},
			},
		},
	})
	driveStarAddCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	driveStarRemoveCmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "取消收藏文档",
		Example: `  dws drive star remove --node <nodeId_or_URL>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "unmark_star", map[string]any{"nodeId": nodeID})
		},
	}
	DeclareLeafMetadata(driveStarRemoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "unmark_star",
				CanonicalPath:  "drive.unmark_star",
				CLIPath:        "drive star remove",
				PrimaryCLIPath: "drive star remove",
			},
			Description: "将文档/文件从当前用户收藏列表移除",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将文档/文件从当前用户收藏列表移除",
				UseWhen:      []string{"用户说 取消收藏/去掉收藏/不收藏了"},
				AvoidWhen:    []string{"添加收藏用 mark_star"},
				Examples:     []string{"dws drive star remove --node <nodeId> --format json"},
			},
		},
	})
	driveStarRemoveCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	driveStarListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取收藏列表",
		Example: `  dws drive star list
  dws drive star list --content-types doc,sheet --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			if v, _ := cmd.Flags().GetString("order-by"); v != "" {
				toolArgs["orderBy"] = v
			}
			if v, _ := cmd.Flags().GetString("sort"); v != "" {
				toolArgs["sortType"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("resource-types"); len(v) > 0 {
				toolArgs["supportResourceTypes"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("content-types"); len(v) > 0 {
				toolArgs["contentTypes"] = v
			}
			return callMCPToolOnServer("drive", "get_star_list", toolArgs)
		},
	}
	DeclareLeafMetadata(driveStarListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_star_list",
				CanonicalPath:  "drive.get_star_list",
				CLIPath:        "drive star list",
				PrimaryCLIPath: "drive star list",
			},
			Description: "获取当前用户的收藏列表，支持分页与按内容类型筛选",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前用户的收藏列表，支持分页与按内容类型筛选",
				UseWhen:      []string{"用户说 我的收藏/收藏列表/收藏了哪些文档"},
				AvoidWhen:    []string{"操作单个收藏用 mark_star/unmark_star"},
				Examples: []string{
					"dws drive star list --format json",
					"dws drive star list --content-types doc,sheet --limit 10 --format json",
				},
			},
		},
	})
	driveStarListCmd.Flags().Int("limit", 0, "每页条数 (默认 20，最大 20)")
	driveStarListCmd.Flags().String("cursor", "", "分页游标")
	driveStarListCmd.Flags().String("order-by", "", "排序字段: createTime")
	driveStarListCmd.Flags().String("sort", "", "排序方向: asc|desc")
	driveStarListCmd.Flags().StringSlice("resource-types", nil, "资源大类: DENTRY, TEAM, WORKSPACE")
	driveStarListCmd.Flags().StringSlice("content-types", nil, "内容类型: doc,sheet,ppt,whiteboard,mind,notable,pdf,other,folder")
	driveStarCmd.AddCommand(driveStarAddCmd, driveStarRemoveCmd, driveStarListCmd)

	// ── drive cover (获取节点封面地址) ──
	driveCoverCmd := &cobra.Command{
		Use:     "cover",
		Short:   "获取节点封面地址",
		Example: "  dws drive cover --node <dentryUuid>",
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("drive", "get_cover", map[string]any{"nodeId": nodeID})
		},
	}
	DeclareLeafMetadata(driveCoverCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "get_cover",
				CanonicalPath:  "drive.get_cover",
				CLIPath:        "drive cover",
				PrimaryCLIPath: "drive cover",
			},
			Description: "获取节点封面图片地址（文档首图/图片缩略图/类型图标）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取节点封面图片地址（文档首图/图片缩略图/类型图标）",
				UseWhen:      []string{"用户说 封面/封面图/缩略图/预览图"},
				AvoidWhen:    []string{"设置文档封面用 doc style cover set"},
				Examples:     []string{"dws drive cover --node <dentryUuid> --format json"},
			},
		},
	})
	driveCoverCmd.Flags().String("node", "", "节点 ID (dentryUuid) 或文档 URL (必填)")
	for _, alias := range []string{"url", "id"} {
		driveCoverCmd.Flags().String(alias, "", "--node 的别名")
		_ = driveCoverCmd.Flags().MarkHidden(alias)
	}
	RegisterCrossProductAliases(driveCoverCmd)

	// ── drive revert (回滚文件到指定历史版本) ──
	driveRevertCmd := &cobra.Command{
		Use:   "revert",
		Short: "[危险] 回滚文件到指定历史版本",
		Long: `将指定文件回滚到某个历史版本。仅支持普通文件（Word、Excel、PDF、图片等）。
在线文档请用 dws doc version revert，在线表格请用 dws sheet version revert。`,
		Example: `  dws drive revert --node <dentryUuid> --version 3 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			versionNum, err := cmd.Flags().GetInt("version")
			if err != nil || versionNum <= 0 {
				return fmt.Errorf("flag --version is required and must be a positive integer")
			}
			return callMCPToolOnServer("drive", "revert_file_version", map[string]any{
				"nodeId":  nodeID,
				"version": versionNum,
			})
		},
	}
	DeclareLeafMetadata(driveRevertCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "revert_file_version",
				CanonicalPath:  "drive.revert_file_version",
				CLIPath:        "drive revert",
				PrimaryCLIPath: "drive revert",
			},
			Description: "回滚普通文件到指定历史版本（生成新最新版本，历史不丢失）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "回滚普通文件到指定历史版本（生成新最新版本，历史不丢失）",
				UseWhen:      []string{"用户说 回滚版本/恢复到某个版本/版本回退，且目标是普通文件"},
				AvoidWhen:    []string{"在线文档回滚用 doc version revert；在线表格用 sheet version revert"},
				Examples:     []string{"dws drive revert --node <dentryUuid> --version 3 --format json"},
			},
		},
	})
	driveRevertCmd.Flags().String("node", "", "文件 ID (dentryUuid) 或 URL (必填)")
	driveRevertCmd.Flags().Int("version", 0, "要回滚到的历史版本号 (必填，正整数)")
	for _, alias := range []string{"url", "id"} {
		driveRevertCmd.Flags().String(alias, "", "--node 的别名")
		_ = driveRevertCmd.Flags().MarkHidden(alias)
	}
	RegisterCrossProductAliases(driveRevertCmd)

	for _, child := range driveStarCmd.Commands() {
		for _, alias := range []string{"url", "id"} {
			if child.Flags().Lookup(alias) == nil {
				child.Flags().String(alias, "", "--node 的别名")
				_ = child.Flags().MarkHidden(alias)
			}
		}
		RegisterCrossProductAliases(child)
	}

	driveStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "比较本地文件夹与钉盘文件夹的差异",
		Long: `比较本地文件夹与钉盘文件夹的差异：本地取 --local-folder（绝对路径），钉盘取
--remote-folder（文件夹 dentryUuid）指向的文件夹，按精确 MD5（默认）或快速
modified_time（--quick）逐文件比对。两侧各自递归遍历，rel_path 相对各自根目录。

输出五类差异：
  new_local   仅本地存在
  new_remote  仅钉盘存在
  modified    两侧都存在且本次检测判定为已变更
  unchanged   两侧都存在且本次检测判定为未变更
  unknown     两侧都存在，但 exact 模式下远端无可靠 MD5、无法核对内容（不判 unchanged/modified）

只比对钉盘 type=file 的二进制文件（跳过在线文档与快捷方式）；本地只比对常规文件。`,
		Example: `  dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --space-id xxxx
  dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --quick`,
		RunE: runDriveStatus,
	}
	driveStatusCmd.Flags().String("local-folder", "", "本地文件夹绝对路径 (必填)")
	driveStatusCmd.Flags().String("remote-folder", "", "钉盘文件夹 ID (dentryUuid) (必填)")
	driveStatusCmd.Flags().String("space-id", "", "钉盘空间 ID，不传则使用「我的文件」(可选)")
	driveStatusCmd.Flags().Bool("quick", false, "快速模式：只比较 modified_time，不计算 MD5 (可选)")

	drivePullCmd := &cobra.Command{
		Use:   "pull",
		Short: "把钉盘文件夹单向镜像到本地（Drive → 本地）",
		Long: `递归下载钉盘 --remote-folder 文件夹下所有 type=file 的文件到本地
--local-folder 对应路径（子目录自动创建），单向、文件级镜像。

已存在的本地文件按 --if-exists 处理：
  skip       默认，安全：本地已存在则保持不动，只新增
  smart      推荐增量同步：本地 modified_time 已 ≥ 远端时则跳过下载
  overwrite  总是下载覆盖（Drive 作为权威源）

该命令会写入本地文件系统，执行前需要用户确认；非交互环境先用 --dry-run
预览，确认后以相同参数追加 --yes 执行。

输出 summary（downloaded/skipped/failed）与逐文件 items。
若有文件下载失败，命令以非零退出码退出，结构化结果仍在 stdout。`,
		Example: `  dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart
  dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --space-id xxxx`,
		RunE: runDrivePull,
	}
	drivePullCmd.Flags().String("local-folder", "", "本地文件夹绝对路径 (必填)")
	drivePullCmd.Flags().String("remote-folder", "", "钉盘文件夹 ID (dentryUuid) (必填)")
	drivePullCmd.Flags().String("space-id", "", "钉盘空间 ID，不传则使用「我的文件」(可选)")
	drivePullCmd.Flags().String("if-exists", "skip", "本地文件已存在时的策略: skip|smart|overwrite；命令会写本地，执行需确认 (可选)")

	drivePushCmd := &cobra.Command{
		Use:   "push",
		Short: "把本地文件夹单向镜像到钉盘（本地 → Drive）",
		Long: `递归把本地 --local-folder 下的文件与子目录（含空目录）镜像到钉盘
--remote-folder 文件夹：缺失的目录按需创建（已存在则复用，不重建），文件按
--if-exists 处理。文件级镜像——只新增/覆盖，不删除远端多余文件。

已存在的远端文件按 --if-exists 处理：
  skip       默认，安全：已存在则保持不动，只新增
  smart      增量同步：远端 modified_time 已 ≥ 本地时跳过，否则走覆盖路径
  overwrite  覆盖远端同名文件

该命令会写入钉盘，执行前需要用户确认；非交互环境先用 --dry-run 预览，
确认后以相同参数追加 --yes 执行。

输出 summary（uploaded/skipped/failed，uploaded 含新建与覆盖）与逐条 items
（含 folder_created）。若有文件失败，命令以非零退出码退出，结构化结果仍在 stdout。`,
		Example: `  dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart
  dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists overwrite`,
		RunE: runDrivePush,
	}
	drivePushCmd.Flags().String("local-folder", "", "本地文件夹绝对路径 (必填)")
	drivePushCmd.Flags().String("remote-folder", "", "钉盘目标文件夹 ID (dentryUuid) (必填)")
	drivePushCmd.Flags().String("space-id", "", "钉盘空间 ID，不传则使用「我的文件」(可选)")
	drivePushCmd.Flags().String("if-exists", "skip", "远端文件已存在时的策略: skip|smart|overwrite；命令会写钉盘，执行需确认 (可选)")

	driveSyncCmd := &cobra.Command{
		Use:   "sync",
		Short: "本地文件夹与钉盘文件夹双向同步（本地 ⇄ Drive）",
		Long: `把本地 --local-folder 与钉盘 --remote-folder 做文件级双向同步：先按精确 MD5
（默认）或快速 modified_time（--quick）算出差异，再按方向执行：
  new_local   仅本地存在  → 上传到钉盘（缺失的远端目录按需创建）
  new_remote  仅钉盘存在  → 下载到本地
  modified    两侧都变更  → 按 --on-conflict 解决
  unchanged   两侧一致    → 不动

两侧都变更时的 --on-conflict 策略：
  skip         默认，两侧都不动并保留两边内容
  remote-wins  拉取远端覆盖本地
  local-wins   上传本地覆盖远端
  keep-both    本地文件改名保留，再拉取远端到原路径
  ask          交互式逐个询问

exact 模式下远端无可靠 MD5、内容无法核对的文件归入 unknown 并跳过（可改用 --quick）。
文件级同步——只新增/覆盖，不删除任何一侧的多余文件。输出 summary（pulled/pushed/
skipped/failed）、diff 与逐条 items；有失败则以非零退出码退出，结构化结果仍在 stdout。

该命令会同时写入本地与钉盘，执行前需要用户确认；非交互环境先用 --dry-run
预览，确认后以相同参数追加 --yes 执行。`,
		Example: `  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --on-conflict local-wins
  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --quick --on-conflict keep-both`,
		RunE: runDriveSync,
	}
	driveSyncCmd.Flags().String("local-folder", "", "本地文件夹绝对路径 (必填)")
	driveSyncCmd.Flags().String("remote-folder", "", "钉盘文件夹 ID (dentryUuid) (必填)")
	driveSyncCmd.Flags().String("space-id", "", "钉盘空间 ID，不传则使用「我的文件」(可选)")
	driveSyncCmd.Flags().String("on-conflict", "skip", "两侧都变更时的策略: skip|remote-wins|local-wins|keep-both|ask；命令会写双端，执行需确认 (可选)")
	driveSyncCmd.Flags().Bool("quick", false, "快速模式：只比较 modified_time，不计算 MD5 (可选)")

	DeclareLeafMetadata(driveStatusCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "folder_status",
				CanonicalPath:  "drive.folder_status",
				CLIPath:        "drive status",
				PrimaryCLIPath: "drive status",
			},
			Description: "比较本地文件夹与钉盘文件夹的差异，只读不落盘。",
			Result:      driveFolderStatusResultSpec(),
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed composite workflow: the command recursively lists the remote folder through drive/list_files, walks the local tree, and compares both sides by MD5 or modification time; no single pinned RPC represents the diff.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "比较本地文件夹与钉盘文件夹的差异，只读不落盘。",
				UseWhen:      []string{"需要先看清本地与钉盘之间哪些文件新增、变更或一致，再决定拉取还是推送时"},
				AvoidWhen:    []string{"只要单个文件的元数据用 drive info；要真正传输文件用 drive pull / push / sync"},
				Examples: []string{
					"dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid>",
					"dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --quick",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "local-folder", Required: boolPtr(true)},
				{Name: "remote-folder", Required: boolPtr(true)},
			},
		},
	})
	DeclareLeafMetadata(drivePullCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "folder_pull",
				CanonicalPath:  "drive.folder_pull",
				CLIPath:        "drive pull",
				PrimaryCLIPath: "drive pull",
			},
			Description: "把钉盘文件夹单向镜像到本地；写操作需确认，默认跳过本地既有文件。",
			DryRun: &contract.DryRunSpec{
				PreviewKind: contract.DryRunPreviewPlan,
				RemoteReads: true,
			},
			Result: driveFolderPullResultSpec(),
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed composite workflow: the command recursively lists the remote folder through drive/list_files and then downloads each file through drive/download_file plus an HTTP GET into a temporary file committed by an atomic rename; no single pinned RPC represents the mirror.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把钉盘文件夹单向镜像到本地；写操作需确认，默认跳过本地既有文件。",
				UseWhen:      []string{"需要把整个钉盘文件夹拉到本地目录时"},
				AvoidWhen:    []string{"只下载单个文件用 drive download；要把本地推到钉盘用 drive push；要双向对齐用 drive sync"},
				Examples: []string{
					"dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid>",
					"dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart --dry-run",
				},
				ExampleDispositions: driveFolderStatefulExampleDispositions(),
			},
			Parameters: []contract.ParamDecl{
				{Name: "local-folder", Required: boolPtr(true)},
				{Name: "remote-folder", Required: boolPtr(true)},
			},
		},
	})
	DeclareLeafMetadata(drivePushCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "folder_push",
				CanonicalPath:  "drive.folder_push",
				CLIPath:        "drive push",
				PrimaryCLIPath: "drive push",
			},
			Description: "把本地文件夹单向镜像到钉盘；写操作需确认，默认跳过远端既有文件。",
			DryRun: &contract.DryRunSpec{
				PreviewKind: contract.DryRunPreviewPlan,
				RemoteReads: true,
			},
			Result: driveFolderPushResultSpec(),
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed composite workflow: the command recursively lists the remote folder through drive/list_files, creates missing folders through drive/create_folder, and uploads each file through drive/get_upload_info plus an HTTP PUT and drive/commit_upload; no single pinned RPC represents the mirror.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把本地文件夹单向镜像到钉盘；写操作需确认，默认跳过远端既有文件。",
				UseWhen:      []string{"需要把整个本地目录推送到钉盘文件夹时"},
				AvoidWhen:    []string{"只上传单个文件用 drive upload；要把钉盘拉到本地用 drive pull；要双向对齐用 drive sync"},
				Examples: []string{
					"dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid>",
					"dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart --dry-run",
				},
				ExampleDispositions: driveFolderStatefulExampleDispositions(),
			},
			Parameters: []contract.ParamDecl{
				{Name: "local-folder", Required: boolPtr(true)},
				{Name: "remote-folder", Required: boolPtr(true)},
			},
		},
	})
	DeclareLeafMetadata(driveSyncCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "folder_sync",
				CanonicalPath:  "drive.folder_sync",
				CLIPath:        "drive sync",
				PrimaryCLIPath: "drive sync",
			},
			Description: "本地与钉盘文件夹双向同步；写操作需确认，默认跳过双端冲突。",
			DryRun: &contract.DryRunSpec{
				PreviewKind: contract.DryRunPreviewPlan,
				RemoteReads: true,
			},
			Result: driveFolderSyncResultSpec(),
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed composite workflow: the command computes the same diff as drive status and then resolves it in both directions through drive/download_file, drive/create_folder, drive/get_upload_info and drive/commit_upload according to --on-conflict; no single pinned RPC represents the bidirectional sync.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "本地与钉盘文件夹双向同步；写操作需确认，默认跳过双端冲突。",
				UseWhen:      []string{"需要让本地目录与钉盘文件夹互相补齐时"},
				AvoidWhen:    []string{"只需单方向镜像用 drive pull / push；只想看差异用 drive status"},
				Examples: []string{
					"dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid>",
					"dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --on-conflict remote-wins --dry-run",
				},
				ExampleDispositions: driveFolderStatefulExampleDispositions(),
			},
			Parameters: []contract.ParamDecl{
				{Name: "local-folder", Required: boolPtr(true)},
				{Name: "remote-folder", Required: boolPtr(true)},
			},
		},
	})

	driveCmd.AddCommand(
		driveListCmd,
		driveListSpacesCmd,
		driveInfoCmd,
		newDriveFileCommentCmd(),
		driveDownloadCmd,
		driveDownloadVersionCmd,
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
		driveQuotaCmd,
		taskCmd,
		newDriveExportCmd(),
		driveShortcutCmd,
		drivePermissionCmd,
		drivePublishCmd,
		recycleCmd,
		// 同步命令：status / pull / push / sync
		driveStatusCmd,
		drivePullCmd,
		drivePushCmd,
		driveSyncCmd,
		driveStarCmd,
		driveCoverCmd,
		driveRevertCmd,
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
	driveText, err := callMCPToolReturnTextOnServer(ctx, "drive", "get_file_info", driveArgs)
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
			current, exists := docResult[field]
			if (!exists || current == nil) && val != nil {
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

// mapOrderByToCamelCase 将 CLI kebab-case 排序字段映射为 MCP camelCase。
func mapOrderByToCamelCase(v string) string {
	switch v {
	case "used-quota":
		return "usedQuota"
	case "standard-used-quota":
		return "standardUsedQuota"
	case "exclusive-used-quota":
		return "exclusiveUsedQuota"
	default:
		return ""
	}
}

// sanitizeFileName removes all directory components and NUL bytes so remote
// metadata can never escape a caller-owned output or temporary directory.
func sanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" || name == "" {
		return "unnamed"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "" {
		return "unnamed"
	}
	return name
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
		return sanitizeFileName(name)
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

// uploadToDrive performs the complete drive upload workflow with explicit
// server routing. overwriteFileID changes both MCP steps to overwrite mode and
// deliberately excludes parentId.
func uploadToDrive(ctx context.Context, filePath, fileName string, fileSize int64, spaceID, folderID, overwriteFileID, mimeType string) error {
	step1Args := map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
	}
	if spaceID != "" {
		step1Args["spaceId"] = spaceID
	}
	if mimeType != "" {
		step1Args["mimeType"] = mimeType
	}
	if overwriteFileID != "" {
		step1Args["overwriteFileId"] = overwriteFileID
	} else if folderID != "" {
		step1Args["parentId"] = folderID
	}

	text, err := callMCPToolReturnTextOnServer(ctx, "drive", "get_upload_info", step1Args)
	if err != nil {
		return err
	}
	// HTTP PUT 上传文件（OSS 与中心协议同路径；headers 透传，401/403 重取凭证重试一次）
	uploadID, err := driveUploadPut(ctx, text, func(rctx context.Context) (string, error) {
		return callMCPToolReturnTextOnServer(rctx, "drive", "get_upload_info", step1Args)
	}, filePath, fileSize)
	if err != nil {
		return err
	}

	commitArgs := map[string]any{
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"uploadId": uploadID,
	}
	if spaceID != "" {
		commitArgs["spaceId"] = spaceID
	}
	if overwriteFileID != "" {
		commitArgs["overwriteFileId"] = overwriteFileID
	} else if folderID != "" {
		commitArgs["parentId"] = folderID
	}
	return callMCPToolOnServer("drive", "commit_upload", commitArgs)
}

// uploadToDocSpace performs the complete document-space upload workflow with
// explicit server routing. overwriteNodeID changes both MCP steps to overwrite
// mode and deliberately excludes folderId.
func uploadToDocSpace(ctx context.Context, filePath, fileName string, fileSize int64, workspaceID, folderID, overwriteNodeID string, convert bool) error {
	step1Args := map[string]any{}
	if workspaceID != "" {
		step1Args["workspaceId"] = workspaceID
	}
	if overwriteNodeID != "" {
		step1Args["overwriteNodeId"] = overwriteNodeID
		step1Args["name"] = fileName
	} else if folderID != "" {
		step1Args["folderId"] = folderID
	}

	text, err := callMCPToolReturnTextOnServer(ctx, "doc", "get_file_upload_info", step1Args)
	if err != nil {
		return err
	}
	resourceURL, uploadKey, headers, err := parseUploadInfo(text)
	if err != nil {
		return err
	}
	if err := httpPutFile(ctx, resourceURL, headers, filePath, fileSize); err != nil {
		return err
	}

	commitArgs := map[string]any{
		"uploadKey": uploadKey,
		"name":      fileName,
		"fileSize":  float64(fileSize),
	}
	if workspaceID != "" {
		commitArgs["workspaceId"] = workspaceID
	}
	if overwriteNodeID != "" {
		commitArgs["overwriteNodeId"] = overwriteNodeID
	} else if folderID != "" {
		commitArgs["folderId"] = folderID
	}
	if convert {
		commitArgs["convertToOnlineDoc"] = true
	}
	return callMCPToolOnServer("doc", "commit_uploaded_file", commitArgs)
}

func downloadFromDrive(ctx context.Context, fileID, spaceID string) (content, filename string, err error) {
	args := map[string]any{"fileId": fileID}
	if spaceID != "" {
		args["spaceId"] = spaceID
	}
	text, err := callMCPToolReturnTextOnServer(ctx, "drive", "download_file", args)
	if err != nil {
		return "", "", err
	}
	resourceURL, headers, err := parseDownloadInfo(text)
	if err != nil {
		return "", "", err
	}
	filename = resolveDownloadFilename(text, resourceURL)
	if filename == "" || filename == "unnamed" {
		filename = "download.md"
	}

	tmpDir, err := os.MkdirTemp("", "dws-drive-download-*")
	if err != nil {
		return "", "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	destPath := filepath.Join(tmpDir, sanitizeFileName(filename))
	if err := httpGetFile(ctx, resourceURL, headers, destPath); err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(destPath)
	if err != nil {
		return "", "", fmt.Errorf("读取下载内容失败: %w", err)
	}
	return string(data), sanitizeFileName(filename), nil
}

func downloadFromDoc(ctx context.Context, nodeID string) (content, filename string, err error) {
	text, err := callMCPToolReturnTextOnServer(ctx, "doc", "download_file", map[string]any{"nodeId": nodeID})
	if err != nil {
		return "", "", err
	}
	resourceURL, headers, err := parseDownloadInfo(text)
	if err != nil {
		return "", "", err
	}
	filename = resolveDownloadFilename(text, resourceURL)
	if filename == "" || filename == "unnamed" {
		filename = "download.md"
	}

	tmpDir, err := os.MkdirTemp("", "dws-doc-download-*")
	if err != nil {
		return "", "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	destPath := filepath.Join(tmpDir, sanitizeFileName(filename))
	if err := httpGetFile(ctx, resourceURL, headers, destPath); err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(destPath)
	if err != nil {
		return "", "", fmt.Errorf("读取下载内容失败: %w", err)
	}
	return string(data), sanitizeFileName(filename), nil
}

// resolveFileDomain probes both domains without guessing when neither probe
// succeeds. Explicit --space-id/--workspace routing bypasses this helper.
func resolveFileDomain(ctx context.Context, nodeID string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, driveErr := callMCPToolReturnTextOnServer(probeCtx, "drive", "get_file_info", map[string]any{"fileId": nodeID})
	if driveErr == nil {
		return "drive", nil
	}
	_, docErr := callMCPToolReturnTextOnServer(probeCtx, "doc", "get_document_info", map[string]any{"nodeId": nodeID})
	if docErr == nil {
		return "doc", nil
	}
	if isTimeoutCLIError(driveErr) || isTimeoutCLIError(docErr) {
		return "", fmt.Errorf("路由文件所在域超时，请重试或通过 --space-id（钉盘）/ --workspace（知识库）显式指定")
	}
	if isPermissionCLIError(driveErr) || isPermissionCLIError(docErr) {
		return "", fmt.Errorf("无权限访问该文件，请确认权限或通过 --space-id/--workspace 显式指定所在域")
	}
	return "", fmt.Errorf("文件 %s 在钉盘和知识库中均未找到，请确认 node ID 或显式指定 --space-id/--workspace", nodeID)
}

func fetchRemoteFileName(ctx context.Context, nodeID string, useDocServer bool) (string, error) {
	serverID, toolName, args := "drive", "get_file_info", map[string]any{"fileId": nodeID}
	if useDocServer {
		serverID, toolName, args = "doc", "get_document_info", map[string]any{"nodeId": nodeID}
	}
	text, err := callMCPToolReturnTextOnServer(ctx, serverID, toolName, args)
	if err != nil {
		return "", err
	}
	return parseRemoteFileName(text), nil
}

func parseRemoteFileName(text string) string {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return ""
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	if name, ok := data["fileName"].(string); ok && name != "" {
		return sanitizeFileName(name)
	}
	name, _ := data["name"].(string)
	ext, _ := data["extension"].(string)
	if name == "" {
		return ""
	}
	if ext == "" || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(ext)) {
		return sanitizeFileName(name)
	}
	return sanitizeFileName(name + "." + ext)
}

func isTimeoutCLIError(err error) bool {
	if err == nil {
		return false
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr.Code == CodeNetworkTimeout || cliErr.Code == CodeLockTimeout
	}
	return isTimeoutError(err.Error())
}

func isPermissionCLIError(err error) bool {
	if err == nil {
		return false
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr.Code == CodeAuthPermission
	}
	var patErr *PATError
	return errors.As(err, &patErr)
}

// parseDriveDownloadInfo 从 drive 的 download_file 返回里取下载 URL 与请求头。
// drive 返回形如 {"result":{"downloadUrl":"https://..."}}；OSS 预签名 URL 自带签名参数、
// 无额外请求头；中心协议（httpToCenterWithToken）返回的 headers 含 dentry-token，
// 需原样透传。对历史字段名（resourceUrl / resourceUrls[].url）做 fallback。
func parseDriveDownloadInfo(text string) (string, map[string]string, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "", nil, fmt.Errorf("解析 download_file 返回失败: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	headers := make(map[string]string)
	if h, ok := data["headers"].(map[string]any); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}

	dlURL, _ := data["downloadUrl"].(string)
	if dlURL == "" {
		dlURL, _ = data["resourceUrl"].(string)
	}
	if dlURL == "" {
		if arr, ok := data["resourceUrls"].([]any); ok && len(arr) > 0 {
			if first, ok := arr[0].(map[string]any); ok {
				dlURL, _ = first["url"].(string)
				if h, ok := first["headers"].(map[string]any); ok {
					for k, v := range h {
						if s, ok := v.(string); ok {
							headers[k] = s
						}
					}
				}
			}
		}
	}
	if dlURL == "" {
		return "", nil, fmt.Errorf("download_file 未返回下载链接（downloadUrl 为空）")
	}
	return dlURL, headers, nil
}
