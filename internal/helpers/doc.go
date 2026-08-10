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
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ──────────────────────────────────────────────────────────
// dws doc — 钉钉文档
// ──────────────────────────────────────────────────────────

// httpPutFile uploads file content via HTTP PUT. Package-level for test injection.
var httpPutFile = defaultHTTPPutFile

// SetHTTPPutFile overrides the HTTP PUT function (for testing). Pass nil to restore default.
func SetHTTPPutFile(fn func(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error) {
	if fn == nil {
		httpPutFile = defaultHTTPPutFile
		return
	}
	httpPutFile = fn
}

func docVersionExists(ctx context.Context, nodeID string, version int) (bool, error) {
	// 注意: 不传 maxResults —— 服务端实际接受的上限小于 schema 声明的 1-50，
	// 传大值会直接报错 (与悟空实现一致: 默认分页大小 + 游标翻页)。
	cursor := ""
	for page := 0; page < 20; page++ {
		toolArgs := map[string]any{"nodeId": nodeID}
		if cursor != "" {
			toolArgs["nextCursor"] = cursor
		}
		text, err := callMCPReadToolReturnTextOnServer(ctx, "doc", "list_doc_versions", toolArgs)
		if err != nil {
			return false, err
		}
		var payload any
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			return false, fmt.Errorf("无法解析文档版本列表，已停止回滚以避免假成功: %w", err)
		}
		if docVersionPayloadContains(payload, version) {
			return true, nil
		}
		cursor = docVersionNextCursor(payload)
		if cursor == "" {
			break
		}
	}
	return false, nil
}

// docVersionNextCursor 从 list_doc_versions 响应中提取分页游标；没有下一页时返回 ""。
func docVersionNextCursor(v any) string {
	switch val := v.(type) {
	case map[string]any:
		if hasMore, ok := val["hasMore"].(bool); ok && !hasMore {
			return ""
		}
		for _, key := range []string{"nextCursor", "nextToken", "cursor"} {
			if s, ok := val[key].(string); ok && s != "" {
				return s
			}
		}
		for _, key := range []string{"result", "content", "data"} {
			if s := docVersionNextCursor(val[key]); s != "" {
				return s
			}
		}
		for _, item := range val {
			if s := docVersionNextCursor(item); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range val {
			if s := docVersionNextCursor(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func docVersionPayloadContains(v any, target int) bool {
	switch val := v.(type) {
	case map[string]any:
		for key, item := range val {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if normalized == "version" || normalized == "versionnumber" || normalized == "versionno" || normalized == "docversion" || normalized == "revision" {
				if docVersionNumberMatches(item, target) {
					return true
				}
			}
			if docVersionPayloadContains(item, target) {
				return true
			}
		}
	case []any:
		for _, item := range val {
			if docVersionPayloadContains(item, target) {
				return true
			}
		}
	}
	return false
}

func docVersionNumberMatches(v any, target int) bool {
	switch val := v.(type) {
	case float64:
		return int(val) == target && val == float64(target)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(val))
		return err == nil && n == target
	case json.Number:
		n, err := val.Int64()
		return err == nil && n == int64(target)
	default:
		return false
	}
}

func runDocUpload(cmd *cobra.Command, _ []string) error {
	if workspace := flagOrFallback(cmd, "workspace", "workspace-id"); workspace != "" {
		deps.Out.PrintWarning("⚠️  'dws doc upload --workspace' is deprecated, use 'dws drive upload --workspace <workspaceId>' instead.")
	}

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

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		name = filepath.Base(filePath)
	} else if filepath.Ext(name) == "" {
		if ext := filepath.Ext(filePath); ext != "" {
			name += ext
		}
	}
	fileSize := fi.Size()

	folder := docFolderFlag(cmd)
	workspace := flagOrFallback(cmd, "workspace", "workspace-id")
	if err := validateDocFolderID(folder); err != nil {
		return err
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "上传文件到钉钉文档")
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", name)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: get upload credentials
	step1Args := map[string]any{}
	if folder != "" {
		step1Args["folderId"] = folder
	}
	if workspace != "" {
		step1Args["workspaceId"] = workspace
	}

	text, err := callMCPToolReturnText(ctx, "get_file_upload_info", step1Args)
	if err != nil {
		return err
	}

	resourceURL, uploadKey, ossHeaders, err := parseUploadInfo(text)
	if err != nil {
		return err
	}

	if err := httpPutFile(ctx, resourceURL, ossHeaders, filePath, fileSize); err != nil {
		return err
	}

	commitArgs := map[string]any{
		"uploadKey": uploadKey,
		"name":      name,
		"fileSize":  float64(fileSize),
	}
	if folder != "" {
		commitArgs["folderId"] = folder
	}
	if workspace != "" {
		commitArgs["workspaceId"] = workspace
	}
	if convert, _ := cmd.Flags().GetBool("convert"); convert {
		commitArgs["convertToOnlineDoc"] = true
	}

	return callMCPTool("commit_uploaded_file", commitArgs)
}

// docSpaceUploadCommitText 执行文档空间三步上传（凭证 → PUT → 入库）并
// 返回 commit 响应原文，供 doc import 的白名单外回退链路组装结构化结果。
// 与 runDocUpload 的区别：不打印输出、不携带 doc upload 的 --workspace
// 兼容告警，调用方负责结果投影。
func docSpaceUploadCommitText(ctx context.Context, filePath, fileName string, fileSize int64, folder, workspace string) (string, error) {
	step1Args := map[string]any{}
	if folder != "" {
		step1Args["folderId"] = folder
	}
	if workspace != "" {
		step1Args["workspaceId"] = workspace
	}
	text, err := callMCPToolReturnText(ctx, "get_file_upload_info", step1Args)
	if err != nil {
		return "", err
	}
	resourceURL, uploadKey, ossHeaders, err := parseUploadInfo(text)
	if err != nil {
		return "", err
	}
	if err := httpPutFile(ctx, resourceURL, ossHeaders, filePath, fileSize); err != nil {
		return "", err
	}
	commitArgs := map[string]any{
		"uploadKey": uploadKey,
		"name":      fileName,
		"fileSize":  float64(fileSize),
	}
	if folder != "" {
		commitArgs["folderId"] = folder
	}
	if workspace != "" {
		commitArgs["workspaceId"] = workspace
	}
	return callMCPToolReturnText(ctx, "commit_uploaded_file", commitArgs)
}

// parseUploadInfo extracts resourceUrl, uploadKey and headers from the MCP tool response.
func parseUploadInfo(text string) (resourceURL, uploadKey string, headers map[string]string, err error) {
	var data map[string]any
	if err = json.Unmarshal([]byte(text), &data); err != nil {
		err = fmt.Errorf("failed to parse upload credentials JSON: %w", err)
		return
	}

	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	resourceURL, _ = data["resourceUrl"].(string)
	uploadKey, _ = data["uploadKey"].(string)

	if resourceURL == "" || uploadKey == "" {
		err = fmt.Errorf("incomplete upload credentials: resourceUrl=%q, uploadKey=%q", resourceURL, uploadKey)
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

func defaultHTTPPutFile(ctx context.Context, url string, headers map[string]string, filePath string, fileSize int64) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.ContentLength = fileSize
	req.Header.Del("Content-Type")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("file upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// typed httpStatusError 供上层按 401/403 分支重取凭证
		return fmt.Errorf("OSS upload failed: %w", &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)})
	}

	return nil
}

// httpGetFile downloads file content via HTTP GET. Package-level for test injection.
var (
	httpGetFile          = defaultHTTPGetFile
	docCreateDestination = os.Create
	docCopyContent       = io.Copy
)

// SetHTTPGetFile overrides the HTTP GET function (for testing). Pass nil to restore default.
func SetHTTPGetFile(fn func(ctx context.Context, url string, headers map[string]string, destPath string) error) {
	if fn == nil {
		httpGetFile = defaultHTTPGetFile
		return
	}
	httpGetFile = fn
}

func runDocDownload(cmd *cobra.Command, _ []string) error {
	nodeID := flagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if nodeID == "" {
		return fmt.Errorf("flag --node is required")
	}
	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		return fmt.Errorf("flag --output is required")
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "下载钉钉文件")
		deps.Out.PrintKeyValue("节点", nodeID)
		deps.Out.PrintKeyValue("输出", outputPath)
		return nil
	}

	ctx := context.Background()

	// Step 1: get download URL and signed headers
	deps.Out.PrintInfo("[1/2] 获取下载链接...")

	text, err := callMCPToolReturnText(ctx, "download_file", map[string]any{
		"nodeId": nodeID,
	})
	if err != nil {
		return err
	}

	resourceURL, dlHeaders, err := parseDownloadInfo(text)
	if err != nil {
		return err
	}

	// Resolve output path: if it's a directory, append inferred filename
	fi, statErr := os.Stat(outputPath)
	if statErr == nil && fi.IsDir() {
		filename := inferFilename(resourceURL)
		outputPath = filepath.Join(outputPath, filename)
	}

	// Step 2: HTTP GET to download file
	deps.Out.PrintInfo(fmt.Sprintf("[2/2] 下载文件到 %s ...", outputPath))

	if err := httpGetFile(ctx, resourceURL, dlHeaders, outputPath); err != nil {
		return err
	}

	deps.Out.PrintInfo(fmt.Sprintf("下载完成: %s", outputPath))
	return nil
}

// parseDownloadInfo extracts resourceUrl (first URL) and headers from the MCP tool response.
func parseDownloadInfo(text string) (resourceURL string, headers map[string]string, err error) {
	var data map[string]any
	if err = json.Unmarshal([]byte(text), &data); err != nil {
		err = fmt.Errorf("failed to parse download info JSON: %w", err)
		return
	}

	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	switch v := data["resourceUrl"].(type) {
	case string:
		resourceURL = v
	case []any:
		if len(v) > 0 {
			resourceURL, _ = v[0].(string)
		}
	}

	// drive download_file 返回 downloadUrl 而非 resourceUrl
	if resourceURL == "" {
		if v, ok := data["downloadUrl"].(string); ok {
			resourceURL = v
		}
	}

	if resourceURL == "" {
		err = fmt.Errorf("incomplete download info: resourceUrl is empty")
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

// inferFilename extracts a filename from a URL, falling back to "download" if unable.
func inferFilename(rawURL string) string {
	if idx := strings.LastIndex(rawURL, "/"); idx >= 0 && idx < len(rawURL)-1 {
		name := rawURL[idx+1:]
		if qIdx := strings.Index(name, "?"); qIdx >= 0 {
			name = name[:qIdx]
		}
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
		// 解码后可能含 %2F 还原出的路径分隔符（如 "ddmedia/xxx.png"），只取
		// 末段 base 名，避免拼出调用方未创建的子目录导致写文件失败。
		name = strings.ReplaceAll(name, "\\", "/")
		name = filepath.Base(name)
		if name != "" && name != "." && name != "/" {
			return name
		}
	}
	return "download"
}

func defaultHTTPGetFile(ctx context.Context, url string, headers map[string]string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// typed httpStatusError 供上层按 401/403 分支重取凭证
		return &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	outFile, err := docCreateDestination(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if _, err := docCopyContent(outFile, resp.Body); err != nil {
		return err
	}

	return nil
}

// runMediaInsert implements the three-step flow for inserting an attachment into a document:
//  1. get_doc_attachment_upload_info → obtain uploadUrl + resourceId
//  2. HTTP PUT file content to OSS
//  3. insert_document_block with attachment element
func runMediaInsert(cmd *cobra.Command, _ []string) error {
	nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if err != nil {
		return err
	}

	filePath := mustGetFlag(cmd, "file")
	if filePath == "" {
		return fmt.Errorf("flag --file is required")
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("%s is a directory, not a file", filePath)
	}

	fileName, _ := cmd.Flags().GetString("name")
	if fileName == "" {
		fileName = filepath.Base(filePath)
	} else if filepath.Ext(fileName) == "" {
		if ext := filepath.Ext(filePath); ext != "" {
			fileName += ext
		}
	}

	mimeType, _ := cmd.Flags().GetString("mime-type")
	if mimeType == "" {
		mimeType = inferMimeType(fileName)
	}

	fileSize := fileInfo.Size()

	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"contractVersion": "doc.operation.v1",
			"dry_run":         true,
			"preview_kind":    "plan",
			"ok":              true,
			"status":          "success",
			"complete":        true,
			"operation":       "doc.media_insert",
			"data": map[string]any{
				"executed": false, "nodeId": nodeID, "file": filePath,
				"fileName": fileName, "mimeType": mimeType, "sizeBytes": fileSize,
			},
			"steps": []map[string]any{{"name": "validate_local_file", "status": "success"}},
		})
	}

	ctx := cmd.Context()

	// Step 1: get upload credentials (uploadUrl + resourceId)
	deps.Out.PrintInfo(fmt.Sprintf("[1/3] 获取附件上传凭证 (%s, %d bytes)...", fileName, fileSize))

	credText, err := callMCPToolReturnText(ctx, "get_doc_attachment_upload_info", map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"mimeType": mimeType,
	})
	if err != nil {
		return err
	}

	uploadURL, resourceID, resourceURL, err := parseAttachmentUploadInfo(credText)
	if err != nil {
		return err
	}

	// Step 2: HTTP PUT file to OSS
	deps.Out.PrintInfo("[2/3] 上传文件到 OSS...")

	ossHeaders := map[string]string{
		"Content-Type": mimeType,
	}
	if err := httpPutFile(ctx, uploadURL, ossHeaders, filePath, fileSize); err != nil {
		return apperrors.NewAPI(
			"附件上传结果未知；尚未确认正文 block 已插入，请先检查文档媒体列表，禁止改用手写 HTTP",
			apperrors.WithOperation("doc.media_insert"),
			apperrors.WithReason("doc_media_upload_unknown"),
			apperrors.WithFailureStage("upload_oss"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(false),
			apperrors.WithActions("运行 dws doc +media-list 检查当前文档", "确认没有对应媒体后才重新执行 +media-insert", "不要 curl 上传地址或安装本地依赖"),
			apperrors.WithDetails(map[string]any{
				"contractVersion": "doc.operation.v1", "status": "unknown", "nodeId": nodeID,
				"resourceId": resourceID, "fileName": fileName, "stage": "upload_oss",
			}),
			apperrors.WithCause(err),
		)
	}

	// Step 3: insert block into document
	deps.Out.PrintInfo("[3/3] 插入块到文档...")

	const maxInlineImageSize = 20 * 1024 * 1024 // 20MB

	var element map[string]any
	if strings.HasPrefix(mimeType, "image/") && resourceURL != "" && fileSize <= maxInlineImageSize {
		// Image files: insert as inline image (paragraph + children image)
		element = map[string]any{
			"blockType": "paragraph",
			"paragraph": map[string]any{
				"text": "",
			},
			"children": []any{
				map[string]any{
					"elementType": "image",
					"properties": map[string]any{
						"src": resourceURL,
					},
				},
			},
		}
	} else {
		// Non-image files: insert as attachment block
		viewType := "preview"
		if mimeType == "text/markdown" {
			viewType = "summary"
		}
		element = map[string]any{
			"blockType": "attachment",
			"attachment": map[string]any{
				"resourceId": resourceID,
				"type":       mimeType,
				"name":       fileName,
				"viewType":   viewType,
			},
		}
	}

	insertArgs := map[string]any{
		"nodeId":  nodeID,
		"element": element,
	}
	if v, _ := cmd.Flags().GetInt("index"); cmd.Flags().Changed("index") {
		insertArgs["index"] = v
	}
	if v, _ := cmd.Flags().GetString("where"); v != "" {
		insertArgs["where"] = v
	}
	if v, _ := cmd.Flags().GetString("ref-block"); v != "" {
		insertArgs["referenceBlockId"] = v
	}

	if err := callMCPTool("insert_document_block", insertArgs); err != nil {
		return apperrors.NewAPI(
			"附件已上传，但正文 block 插入结果未知；请先检查媒体列表，不要重复上传或插入",
			apperrors.WithOperation("doc.media_insert"),
			apperrors.WithReason("doc_media_insert_partial"),
			apperrors.WithFailureStage("insert_block"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(false),
			apperrors.WithActions("运行 dws doc +media-list 检查 resourceId/blockId", "不要直接重试 +media-insert，不要使用 resourceUrl 手写请求"),
			apperrors.WithDetails(map[string]any{
				"contractVersion": "doc.operation.v1", "status": "partial_success", "nodeId": nodeID,
				"resourceId": resourceID, "resourceUrl": resourceURL, "fileName": fileName,
				"steps": []map[string]any{
					{"name": "resolve_upload", "status": "success"},
					{"name": "upload_oss", "status": "success"},
					{"name": "insert_block", "status": "unknown"},
				},
			}),
			apperrors.WithCause(err),
		)
	}

	return deps.Out.PrintJSON(map[string]any{
		"contractVersion": "doc.operation.v1",
		"ok":              true,
		"status":          "success",
		"complete":        true,
		"operation":       "doc.media_insert",
		"data": map[string]any{
			"nodeId": nodeID, "resourceId": resourceID, "resourceUrl": resourceURL,
			"fileName": fileName, "mimeType": mimeType, "sizeBytes": fileSize, "inserted": true,
		},
		"steps": []map[string]any{
			{"name": "resolve_upload", "status": "success"},
			{"name": "upload_oss", "status": "success"},
			{"name": "insert_block", "status": "success"},
		},
	})
}

// parseAttachmentUploadInfo extracts uploadUrl, resourceId and resourceUrl from the MCP tool response.
func parseAttachmentUploadInfo(text string) (uploadURL, resourceID, resourceURL string, err error) {
	var data map[string]any
	if err = json.Unmarshal([]byte(text), &data); err != nil {
		err = fmt.Errorf("failed to parse attachment upload info JSON: %w", err)
		return
	}

	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	uploadURL, _ = data["uploadUrl"].(string)
	resourceID, _ = data["resourceId"].(string)
	resourceURL, _ = data["resourceUrl"].(string)

	if uploadURL == "" || resourceID == "" {
		err = fmt.Errorf("incomplete attachment upload info: uploadUrl=%q, resourceId=%q", uploadURL, resourceID)
	}
	return
}

// inferMimeType guesses a MIME type from the file extension.
func inferMimeType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	mimeTypes := map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".webp": "image/webp",
		".mp4":  "video/mp4",
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".zip":  "application/zip",
		".gz":   "application/gzip",
		".tar":  "application/x-tar",
		".json": "application/json",
		".xml":  "application/xml",
		".csv":  "text/csv",
		".txt":  "text/plain",
		".html": "text/html",
		".md":   "text/markdown",
	}
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// buildBlockElement 从 flags 构建块元素 JSON 对象。
// 优先级: --element (原始 JSON) > --heading > --text
func buildBlockElement(cmd *cobra.Command) (any, error) {
	if raw, _ := cmd.Flags().GetString("element"); raw != "" {
		var obj any
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			return nil, fmt.Errorf("--element JSON parse failed: %w", err)
		}
		return obj, nil
	}
	if h, _ := cmd.Flags().GetString("heading"); h != "" {
		level, _ := cmd.Flags().GetInt("level")
		if level < 1 || level > 6 {
			level = 1
		}
		return map[string]any{
			"blockType": "heading",
			"heading":   map[string]any{"text": h, "level": level},
		}, nil
	}
	if t, _ := cmd.Flags().GetString("text"); t != "" {
		return map[string]any{
			"blockType": "paragraph",
			"paragraph": map[string]any{"text": t},
		}, nil
	}
	return nil, fmt.Errorf("block content required: --text / --heading / --element")
}

// buildNodeTransferRunE creates a RunE handler for copy/move commands.
// It extracts --node, --folder, --workspace flags and calls the specified MCP tool.
func buildNodeTransferRunE(mcpToolName string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
		if err != nil {
			return err
		}
		toolArgs := map[string]any{
			"nodeId": nodeID,
		}
		if v := docFolderFlag(cmd); v != "" {
			if err := validateDocFolderID(v); err != nil {
				return err
			}
			toolArgs["targetFolderId"] = v
		}
		if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
			toolArgs["workspaceId"] = v
		}
		return callMCPTool(mcpToolName, toolArgs)
	}
}

func docFolderFlag(cmd *cobra.Command, extraAliases ...string) string {
	aliases := append([]string{"parent-id", "parent-folder", "parent-node-id", "parent-folder-id"}, extraAliases...)
	return flagOrFallback(cmd, "folder", aliases...)
}

func validateDocFolderID(folderID string) error {
	value := strings.TrimSpace(folderID)
	if value == "" || strings.Contains(value, "alidocs.dingtalk.com") {
		return nil
	}

	for _, r := range value {
		if r < '0' || r > '9' {
			return nil
		}
	}

	return fmt.Errorf("invalid doc --folder %q: pure numeric IDs are usually drive dentryId/parent-id values, not DingTalk doc folder nodeId values; use a doc folder nodeId or alidocs folder URL, or omit --folder to use the default doc root", folderID)
}

// previewDocOverwriteDiff reads the current document content and prints a
// human-readable diff against the incoming markdown without calling the
// remote update API. Used by `dws doc update --mode overwrite --dry-run`.
func previewDocOverwriteDiff(ctx context.Context, cmd *cobra.Command, nodeID, newMarkdown string) error {
	text, err := callMCPToolReturnText(ctx, "get_document_content", map[string]any{"nodeId": nodeID})
	if err != nil {
		return fmt.Errorf("dry-run read failed: %w", err)
	}
	current := extractMarkdownField(text)
	out := cmd.OutOrStdout()
	fmt.Fprint(out, renderDocOverwriteDiff(nodeID, current, newMarkdown))
	return nil
}

// extractMarkdownField pulls the "markdown" string field out of the JSON
// returned by get_document_content. Falls back to the raw text when the JSON
// shape is not recognized.
func extractMarkdownField(jsonText string) string {
	var body struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(jsonText), &body); err == nil && body.Markdown != "" {
		return body.Markdown
	}
	return jsonText
}

// renderDocOverwriteDiff returns a unified-diff-style preview comparing the
// document's current markdown ("before") with the incoming overwrite content
// ("after"). The format is intentionally simple — sufficient for the agent or
// user to judge magnitude of the change before passing --yes.
func renderDocOverwriteDiff(nodeID, before, after string) string {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "[dry-run] dws doc update --mode overwrite --node %s\n", nodeID)
	fmt.Fprintf(&sb, "--- current  (%d lines, %d bytes)\n", len(beforeLines), len(before))
	fmt.Fprintf(&sb, "+++ incoming (%d lines, %d bytes)\n", len(afterLines), len(after))

	const headLines = 20
	fmt.Fprintln(&sb, "@@ current (head) @@")
	for i, line := range beforeLines {
		if i >= headLines {
			fmt.Fprintf(&sb, "  ... (%d more lines)\n", len(beforeLines)-headLines)
			break
		}
		fmt.Fprintf(&sb, "- %s\n", line)
	}
	fmt.Fprintln(&sb, "@@ incoming (head) @@")
	for i, line := range afterLines {
		if i >= headLines {
			fmt.Fprintf(&sb, "  ... (%d more lines)\n", len(afterLines)-headLines)
			break
		}
		fmt.Fprintf(&sb, "+ %s\n", line)
	}
	fmt.Fprint(&sb, "\nNo write performed. Rerun without --dry-run and add --yes to apply.\n")
	return sb.String()
}

func newDocCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/doc.json
	// products.doc). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "doc",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理钉钉在线文档的正文、块、评论、导入导出、模板与版本",
			UseWhen: []string{
				"创建、读取或编辑在线文档内容，或处理文档块、评论、导入导出、模板和版本时",
			},
			AvoidWhen: []string{
				"文件、目录、上传下载及节点权限已迁移到 drive；知识库空间和成员使用 wiki；不要用于搜索开放平台开发文档",
			},
		},
	})
	root := &cobra.Command{
		Use:   "doc",
		Short: "钉钉文档管理",
		Long: `管理钉钉文档：浏览、读写、块级编辑、导出、导入、模板管理。

命令结构:
  dws doc info                          获取文档元信息
  dws doc read                          读取文档内容 (Markdown)
  dws doc create                        创建文档
  dws doc update                        更新文档内容
  dws doc block [list|insert|update|delete]  块级编辑
  dws doc whiteboard insert             插入空白板卡片 (返回 blockId 与白板 partId)
  dws doc media [upload|download]       文档媒体资源 (上传可复用资源 / 下载附件)
  dws doc comment [list|create|reply|update|delete|create-inline]  文档评论管理
  dws doc export                        导出在线文档 (支持 docx / markdown / pdf，自动完成提交→轮询→下载)
  dws doc export get                    查询导出任务结果 (手动兜底)
  dws doc import                        导入本地文件为在线文档 (支持 docx / xlsx / md 等)
  dws doc import get                    查询导入任务结果 (手动兜底)
  dws doc template list                 获取文档模板列表
  dws doc template search               搜索文档模板
  dws doc template apply                应用文档模板创建新文档

文件管理（搜索/列表/上传/下载/复制/移动/重命名/删除/权限）已迁移到 dws drive。`,
		RunE: groupRunE,
	}

	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索文档",
		Long: `根据关键词搜索当前用户有权限访问的文档列表。不传关键词则返回最近访问的文档。

支持按扩展名、创建/访问时间范围、创建者、编辑者、@提及用户、知识库等维度过滤。
`,
		Example: `  dws doc search --query "会议纪要"
  dws doc search
  dws doc search --extensions pdf,docx
  dws doc search --query "方案" --created-from 1700000000000 --created-to 1710000000000
  dws doc search --creator-uids uid1,uid2
  dws doc search --workspace-ids wsId1,wsId2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				toolArgs["keyword"] = v
			} else if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if (cmd.Flags().Changed("query") || cmd.Flags().Changed("keyword")) && len(toolArgs) == 0 {
				fmt.Fprintf(os.Stderr, "hint: --query 值为空已忽略，将返回最近访问的文档\n")
			}
			if v, _ := cmd.Flags().GetStringSlice("extensions"); len(v) > 0 {
				toolArgs["extensions"] = v
			}
			if cmd.Flags().Changed("created-from") {
				if v, _ := cmd.Flags().GetInt64("created-from"); v > 0 {
					toolArgs["createdTimeFrom"] = v
				}
			}
			if cmd.Flags().Changed("created-to") {
				if v, _ := cmd.Flags().GetInt64("created-to"); v > 0 {
					toolArgs["createdTimeTo"] = v
				}
			}
			if cmd.Flags().Changed("visited-from") {
				if v, _ := cmd.Flags().GetInt64("visited-from"); v > 0 {
					toolArgs["visitedTimeFrom"] = v
				}
			}
			if cmd.Flags().Changed("visited-to") {
				if v, _ := cmd.Flags().GetInt64("visited-to"); v > 0 {
					toolArgs["visitedTimeTo"] = v
				}
			}
			if v, _ := cmd.Flags().GetStringSlice("creator-uids"); len(v) > 0 {
				toolArgs["creatorUserIds"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("editor-uids"); len(v) > 0 {
				toolArgs["editorUserIds"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("mentioned-uids"); len(v) > 0 {
				toolArgs["mentionedUserIds"] = v
			}
			if v, _ := cmd.Flags().GetStringSlice("workspace-ids"); len(v) > 0 {
				toolArgs["workspaceIds"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["pageSize"] = v
			} else if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["pageToken"] = v
			}
			return callMCPTool("search_documents", toolArgs)
		},
	}
	DeclareLeafMetadata(searchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "search_documents",
				CanonicalPath:  "doc.search_documents",
				CLIPath:        "doc search",
				PrimaryCLIPath: "doc search",
			},
			Description: "搜索文档（不传关键词返回最近访问）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "search_documents"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "搜索文档（不传关键词返回最近访问）",
				UseWhen:      []string{"兼容入口：按关键词搜文档时（已弃用，日常改用 drive search / wiki node search）"},
				AvoidWhen:    []string{"全局搜文件用 dws drive search；指定知识库内搜用 dws wiki node search"},
				Examples:     []string{"dws doc search --query \"会议纪要\" --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "created-from", Property: "createdTimeFrom"},
				{Name: "created-to", Property: "createdTimeTo"},
				{Name: "creator-uids", Property: "creatorUserIds"},
				{Name: "cursor", Property: "pageToken"},
				{Name: "editor-uids", Property: "editorUserIds"},
				{Name: "limit", Property: "pageSize"},
				{Name: "mentioned-uids", Property: "mentionedUserIds"},
				{Name: "query", Property: "keyword"},
				{Name: "visited-from", Property: "visitedTimeFrom"},
				{Name: "visited-to", Property: "visitedTimeTo"},
			},
		},
	})

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "遍历文件列表",
		Long: `列出文件夹或知识库下的直接子节点 (文件夹/文档/文件等)。
定位优先级: --folder > --workspace > 默认 (我的文档根目录)

跨语义组别名: --node / --file-id 在 list 场景等价于 --folder
  (当 nodeId 实际指向文件夹节点时, 直觉式调用 --node <FOLDER_NODE_ID> 也可正常工作; 推荐用 --folder 表意更清晰)。`,
		Example: `  dws doc list
  dws doc list --folder DOC_FOLDER_NODE_ID
  dws doc list --workspace WS_ID --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if folder := docFolderFlag(cmd, "node", "file-id"); folder != "" {
				if err := validateDocFolderID(folder); err != nil {
					return err
				}
				toolArgs["folderId"] = folder
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["pageSize"] = v
			} else if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token"); v != "" {
				toolArgs["pageToken"] = v
			}
			return callMCPTool("list_nodes", toolArgs)
		},
	}
	DeclareLeafMetadata(listCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "list_nodes",
				CanonicalPath:  "doc.list_nodes",
				CLIPath:        "doc list",
				PrimaryCLIPath: "doc list",
			},
			Description: "兼容入口：遍历文件夹或知识库的直接子节点；目录浏览能力已迁移到 drive/wiki。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "list_nodes"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：遍历文件夹或知识库的直接子节点；目录浏览能力已迁移到 drive/wiki。",
				UseWhen:      []string{"兼容入口：遍历文件夹或知识库直接子节点时（已弃用，日常改用 drive list / wiki node list）"},
				AvoidWhen:    []string{"日常浏览「我的文档」/钉盘用 dws drive list；知识库用 dws wiki node list"},
				Examples: []string{
					"dws doc list --format json",
					"dws doc list --folder <FOLDER_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "pageToken"},
				{Name: "folder", Property: "folderId"},
				{Name: "limit", Property: "pageSize"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "获取文档元信息",
		Long:  `获取文档标题、类型、创建者、创建时间、权限等元信息 (不含内容)。`,
		Example: `  dws doc info --node DOC_ID
  dws doc info --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPTool("get_document_info", map[string]any{"nodeId": nodeID})
		},
	}
	DeclareLeafMetadata(infoCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "get_document_info",
				CanonicalPath:  "doc.get_document_info",
				CLIPath:        "doc info",
				PrimaryCLIPath: "doc info",
			},
			Description: "获取文档元信息（标题/类型/创建者/权限等）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "get_document_info"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取文档元信息（标题/类型/创建者/权限等）",
				UseWhen: []string{
					"用户要查看文档/节点元信息（标题、类型、创建者、权限）时",
					"准备读内容前必须先看 contentType/extension 以路由到 read/sheet/aitable/download 时",
				},
				AvoidWhen: []string{
					"已确认是 adoc 且只要正文改用 dws doc read",
					"只要目录列表改用 dws drive list / wiki node list",
					"需要可靠文件大小 fileSize 时改用 dws drive info；文档元信息接口可能不返回大小",
				},
				Examples: []string{
					"dws doc info --node <DOC_ID> --format json",
					"dws doc info --node \"https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	readCmd := &cobra.Command{
		Use:   "read",
		Short: "读取文档内容 (Markdown)",
		Long:  `获取文档内容，以 Markdown 格式返回。支持传入文档 URL 或 ID。`,
		Example: `  dws doc read --node DOC_ID
  dws doc read --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateDocFormat(cmd, []string{"", "markdown", "jsonml"}, "doc read",
				"dws doc read --node DOC_ID --content-format jsonml"); err != nil {
				return err
			}
			format, _ := cmd.Flags().GetString("content-format")
			scope, _ := cmd.Flags().GetString("scope")
			tags, _ := cmd.Flags().GetString("tags")
			startBlockID, _ := cmd.Flags().GetString("start-block-id")
			endBlockID, _ := cmd.Flags().GetString("end-block-id")
			if scope != "" || tags != "" {
				if format != "jsonml" {
					return fmt.Errorf("--scope/--tags requires --content-format jsonml")
				}
				if scope == "" {
					return fmt.Errorf("--tags requires --scope tags")
				}
				switch scope {
				case "outline", "range", "section", "tags":
				default:
					return fmt.Errorf("invalid --scope %q: must be one of outline|range|section|tags", scope)
				}
				if tags != "" && scope != "tags" {
					return fmt.Errorf("--tags only works with --scope tags")
				}
				if scope == "tags" && tags == "" {
					return fmt.Errorf("--tags is required when --scope=tags")
				}
				if (scope == "range" || scope == "section") && startBlockID == "" {
					return fmt.Errorf("--start-block-id is required when --scope=%s", scope)
				}
				if endBlockID != "" && scope != "range" {
					return fmt.Errorf("--end-block-id only works with --scope=range")
				}
				maxDepth, _ := cmd.Flags().GetInt("max-depth")
				outputPath, _ := cmd.Flags().GetString("output")
				return runDocReadScope(
					nodeID,
					scope,
					tags,
					maxDepth,
					cmd.Flags().Changed("max-depth"),
					startBlockID,
					endBlockID,
					outputPath,
				)
			}
			if format == "jsonml" {
				outputPath, _ := cmd.Flags().GetString("output")
				return runDocReadJsonML(cmd, nodeID, outputPath)
			}
			return callMCPTool("get_document_content", map[string]any{
				"nodeId": nodeID,
			})
		},
	}
	DeclareLeafMetadata(readCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "get_document_content",
				CanonicalPath:  "doc.get_document_content",
				CLIPath:        "doc read",
				PrimaryCLIPath: "doc read",
			},
			Description: "读取完整文档内容，或按 outline/range/section/tags 获取 JSONML fragment",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "get_document_content"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "读取完整文档内容，或按 outline/range/section/tags 获取 JSONML fragment",
				UseWhen: []string{
					"用户要读取钉钉在线文字文档(adoc)正文（Markdown）时",
					"用户直接粘贴文档 URL 且无其他指令时（默认读内容）",
					"只需标题大纲、指定块区间/单块或特定 JSONML tags 时使用 --content-format jsonml 与 --scope",
				},
				AvoidWhen: []string{
					"非 adoc（表格/多维表/普通文件）不要用本命令；先 doc info 再路由",
					"要元信息用 doc info；要块结构用 doc block list",
					"Markdown 为有损投影：保形复制模板请用 doc copy，不要 read→create",
				},
				Examples: []string{
					"dws doc read --node <DOC_ID> --format json",
					"dws doc read --node <DOC_ID> --content-format jsonml --scope outline --max-depth 3",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "content-format", Property: "format", Required: boolPtr(false)},
				{Name: "end-block-id", Required: boolPtr(false)},
				{Name: "max-depth", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "scope", Required: boolPtr(false)},
				{Name: "start-block-id", Required: boolPtr(false), RequiredWhen: "--scope=range or --scope=section"},
				{Name: "tags", Required: boolPtr(false), RequiredWhen: "--scope=tags"},
			},
		},
	})

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文档",
		Long: `创建一篇新的钉钉在线文档。
创建位置优先级: --folder > --workspace > 默认 (我的文档根目录)

初始内容来源（--content-file 优先于 --content）:
  --content "..."       短文本字面量（仅推荐 <2KB 且无换行/表格）
  --content -           从 stdin 读取（可配合 heredoc/pipe）
  --content-file path   从 UTF-8 文件读取（推荐长/多行/表格内容，避免 shell escape）`,
		Example: `  dws doc create --name "项目周报"
  dws doc create --name "Q1 总结" --content "# Q1 总结" --folder DOC_FOLDER_NODE_ID
  dws doc create --name "知识库文档" --workspace WS_ID
  dws doc create --name "周报" --content-file ./weekly.md --folder DOC_FOLDER_NODE_ID
  cat report.md | dws doc create --name "月报" --content -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDocFormat(cmd, []string{"", "markdown", "jsonml"}, "doc create",
				"dws doc create --name \"demo\" --content-format jsonml --content-file body.json"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "name", "title"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"name": flagOrFallback(cmd, "name", "title"),
			}
			if v := docFolderFlag(cmd); v != "" {
				if err := validateDocFolderID(v); err != nil {
					return err
				}
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			md, err := resolveContentFromFlags(cmd)
			if err != nil {
				return err
			}
			format, _ := cmd.Flags().GetString("content-format")
			if format != "jsonml" && md != "" && sniffJsonMLLike(md) {
				deps.Out.PrintWarning(`输入内容看起来是 JSONML 结构（首字符 '[' 后紧跟 JSON 字符串）。`)
				deps.Out.PrintWarning(`若要按 JSONML 解析，请加 --content-format jsonml；否则将按 markdown 解析。`)
			}
			if format == "jsonml" && md != "" {
				jsonmlStr, err := prepareJsonMLBody(cmd, md)
				if err != nil {
					return err
				}
				createArgs := map[string]any{"name": toolArgs["name"]}
				if v, ok := toolArgs["folderId"]; ok {
					createArgs["folderId"] = v
				}
				if v, ok := toolArgs["workspaceId"]; ok {
					createArgs["workspaceId"] = v
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				resultText, err := callMCPToolReturnText(ctx, "create_document", createArgs)
				if err != nil {
					return err
				}
				newNodeID := extractNodeIDFromResult(resultText)
				if newNodeID == "" {
					return fmt.Errorf("创建文档成功但无法提取 nodeId")
				}
				return callMCPTool("update_document", map[string]any{
					"nodeId": newNodeID,
					"format": "jsonml",
					"jsonml": jsonmlStr,
					"mode":   "overwrite",
				})
			}
			if md != "" {
				if name, ok := toolArgs["name"].(string); ok && name != "" {
					md = stripDuplicateTitle(md, name)
				}
				toolArgs["markdown"] = md
			}
			if md != "" {
				return docWritePipeline(cmd, "create_document", toolArgs, md, "doc create")
			}
			return callMCPTool("create_document", toolArgs)
		},
	}
	DeclareLeafMetadata(createCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "create_document",
				CanonicalPath:  "doc.create_document",
				CLIPath:        "doc create",
				PrimaryCLIPath: "doc create",
			},
			Description: "创建一篇新的在线文档",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "create_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建一篇新的在线文档",
				UseWhen: []string{
					"用户要新建一篇文字在线文档(adoc)，可空文档或带初始 Markdown 时",
					"创建到指定文件夹 --folder、知识库根 --workspace，或默认「我的文档」根目录时",
				},
				AvoidWhen: []string{
					"创建表格/脑图/白板/多维表/演示改用 dws wiki node create --type <type>（勿用 doc create）",
					"在知识库建空节点实体也可用 wiki node create；本命令侧重可写初始内容的 adoc",
					"导入本地 Word/Markdown 为在线文档改用 dws doc import（若可用）或 upload --convert",
				},
				Examples: []string{
					"dws doc create --name \"项目周报\" --format json",
					"dws doc create --name \"Q1 总结\" --content \"# Q1 总结\" --folder <FOLDER_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "name", Required: boolPtr(true)},
				{Name: "folder", Property: "folderId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新文档内容",
		Long: `更新文档的 Markdown 内容。
  --mode overwrite: 覆盖 (清空原内容后重写)
  --mode append:    追加 (在末尾追加，最安全)
  注意: --mode 为必填参数，必须显式指定 overwrite 或 append。

WARNING: --mode overwrite 为破坏性写入，会清空原文档全部内容。
  - 必须显式传 --yes 才会执行覆盖；不传 --yes 时命令直接返回错误。
  - 可先用 --dry-run 预览待写入内容与当前内容的差异 (不调远端 update)。
  - 调用前建议先用 dws doc read 备份现状；调用后建议再次 read 校验。

内容来源（--content-file 优先于 --content）:
  --content "..."       短文本字面量（仅推荐 <2KB 且无换行/表格）
  --content -           从 stdin 读取（可配合 heredoc/pipe）
  --content-file path   从 UTF-8 文件读取（推荐长/多行/表格内容，避免 shell escape）

插入位置（仅 mode=append 生效）:
  --index N             将内容插入到文档第 N 个 block 之前（从 0 开始）。
                        不传时追加到末尾。block index 可通过 doc block list 获取。
                        插入成功后，该位置及之后所有 block 的 index 会依次 +1。`,
		Example: `  dws doc update --node DOC_ID --content "# 新内容" --mode append
  dws doc update --node DOC_ID --content "# 完整替换" --mode overwrite --yes
  dws doc update --node DOC_ID --content-file ./part1.md --mode overwrite --dry-run
  dws doc update --node DOC_ID --content-file ./part1.md --mode append
  dws doc update --node DOC_ID --content "# 插入到第3个block前" --mode append --index 2
  dws doc update --node DOC_ID --content-file ./body.json --content-format jsonml --revision 42 --mode overwrite
  cat part2.md | dws doc update --node DOC_ID --content - --mode append`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateDocFormat(cmd, []string{"", "markdown", "jsonml"}, "doc update",
				"dws doc update --node DOC_ID --content-format jsonml --content-file body.json --mode overwrite --yes"); err != nil {
				return err
			}
			md, err := resolveContentFromFlags(cmd)
			if err != nil {
				return err
			}
			if md == "" {
				return fmt.Errorf("必须通过 --content 或 --content-file 提供内容")
			}
			mode, _ := cmd.Flags().GetString("mode")
			if mode == "" {
				return fmt.Errorf("必须通过 --mode 指定更新模式（overwrite 或 append）")
			}
			yes, _ := cmd.Flags().GetBool("yes")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if mode == "overwrite" {
				if dryRun {
					return previewDocOverwriteDiff(cmd.Context(), cmd, nodeID, md)
				}
				if !yes {
					return fmt.Errorf("--mode overwrite 为破坏性写入，请加 --yes 显式确认，或加 --dry-run 预览差异 (不调远端)")
				}
			}
			format, _ := cmd.Flags().GetString("content-format")
			if format != "jsonml" && md != "" && sniffJsonMLLike(md) {
				deps.Out.PrintWarning(`输入内容看起来是 JSONML 结构（首字符 '[' 后紧跟 JSON 字符串）。`)
				deps.Out.PrintWarning(`若要按 JSONML 解析，请加 --content-format jsonml；否则将按 markdown 解析。`)
			}
			if format == "jsonml" {
				if mode == "append" {
					return fmt.Errorf("--content-format jsonml 当前仅支持 --mode overwrite，append 模式将在后续版本支持")
				}
				jsonmlStr, err := prepareJsonMLBody(cmd, md)
				if err != nil {
					return err
				}
				updateArgs := map[string]any{
					"nodeId": nodeID,
					"format": "jsonml",
					"jsonml": jsonmlStr,
					"mode":   mode,
				}
				if rev, _ := cmd.Flags().GetInt("revision"); cmd.Flags().Lookup("revision").Changed {
					updateArgs["revision"] = rev
				}
				return callMCPTool("update_document", updateArgs)
			}
			toolArgs := map[string]any{
				"nodeId":   nodeID,
				"markdown": md,
				"mode":     mode,
			}
			if idx, _ := cmd.Flags().GetInt("index"); cmd.Flags().Lookup("index").Changed {
				toolArgs["index"] = idx
			}
			return docWritePipeline(cmd, "update_document", toolArgs, md, "doc update")
		},
	}
	DeclareLeafMetadata(updateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "update_document",
				CanonicalPath:  "doc.update_document",
				CLIPath:        "doc update",
				PrimaryCLIPath: "doc update",
			},
			Description: "更新文档内容（追加 / 覆盖；覆盖需 --yes）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "update_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新文档内容（追加 / 覆盖；覆盖需 --yes）",
				UseWhen: []string{
					"用户要向已有 adoc 追加内容时用 --mode append（更安全）",
					"用户明确要求整篇覆盖替换时用 --mode overwrite（破坏性）",
					"append 且要插到第 N 个 block 前时加 --index N（先 block list）",
				},
				AvoidWhen: []string{
					"目标不是 adoc 或只要改单个块时改用 doc block update",
					"覆盖模式用户未确认前不要执行；可先 --dry-run 预览",
					"创建新文档用 doc create，不要用 update 冒充创建",
				},
				Examples: []string{
					"dws doc update --node <DOC_ID> --content \"# 追加内容\" --mode append --format json",
					"dws doc update --node <DOC_ID> --content-file ./body.md --mode overwrite --dry-run",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "content-format", Property: "format"},
			},
		},
	})

	fileCmd := &cobra.Command{Use: "file", Short: "文件管理", RunE: groupRunE}

	fileCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文件",
		Long: `在指定目录下新增文件。支持的文件类型 (--type):
  adoc    钉钉在线文档
  axls    钉钉表格
  appt    钉钉演示
  adraw   钉钉白板
  amind   钉钉脑图
  able    钉钉多维表
  folder  文件夹

兼容旧版 accessType: "0"=adoc "1"=axls "2"=appt "3"=adraw "6"=amind "7"=able "13"=folder
创建位置优先级: --folder > --workspace > 默认 (我的文档根目录)`,
		Example: `  dws doc file create --name "项目周报" --type adoc
  dws doc file create --name "数据统计" --type axls --folder DOC_FOLDER_NODE_ID
  dws doc file create --name "思维导图" --type amind --workspace WS_ID
  dws doc file create --name "子文件夹" --type folder`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "title"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "type"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"name": flagOrFallback(cmd, "name", "title"),
				"type": mustGetFlag(cmd, "type"),
			}
			if v := docFolderFlag(cmd); v != "" {
				if err := validateDocFolderID(v); err != nil {
					return err
				}
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPTool("create_file", toolArgs)
		},
	}
	DeclareLeafMetadata(fileCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "create_file",
				CanonicalPath:  "doc.create_file",
				CLIPath:        "doc file create",
				PrimaryCLIPath: "doc file create",
			},
			Description: "创建文件（文档/表格/脑图/白板/多维表/文件夹等）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "create_file"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建文件（文档/表格/脑图/白板/多维表/文件夹等）",
				UseWhen:      []string{"兼容入口：按类型创建文件节点时（已弃用，改用 wiki node create）"},
				AvoidWhen: []string{
					"请改用 dws wiki node create --workspace <id> --type <type>",
					"纯文字带内容创建用 doc create",
				},
				Examples: []string{"dws doc file create --name \"项目周报\" --type adoc --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "folderId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	folderCmd := &cobra.Command{Use: "folder", Short: "文件夹管理", RunE: groupRunE}

	folderCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文件夹",
		Long: `在指定位置创建文件夹。
创建位置优先级: --folder (父文件夹) > --workspace > 默认 (我的文档根目录)`,
		Example: `  dws doc folder create --name "项目资料"
  dws doc folder create --name "子文件夹" --folder PARENT_DOC_FOLDER_NODE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "title"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"name": flagOrFallback(cmd, "name", "title"),
			}
			if v := docFolderFlag(cmd); v != "" {
				if err := validateDocFolderID(v); err != nil {
					return err
				}
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPTool("create_folder", toolArgs)
		},
	}
	DeclareLeafMetadata(folderCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "create_folder",
				CanonicalPath:  "doc.create_folder",
				CLIPath:        "doc folder create",
				PrimaryCLIPath: "doc folder create",
			},
			Description: "创建文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "create_folder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建文件夹",
				UseWhen:      []string{"兼容入口：创建文件夹（已弃用）时"},
				AvoidWhen:    []string{"个人空间/钉盘改用 dws drive mkdir；知识库改用 wiki node create --type folder"},
				Examples:     []string{"dws doc folder create --name \"项目资料\" --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "folderId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "上传文件到钉钉文档或钉钉知识库",
		Long: `将本地文件上传到钉钉文档或钉钉知识库（三步上传流程）。

流程:
  1. 获取 OSS 上传凭证 (get_file_upload_info)
  2. HTTP PUT 上传文件二进制到 OSS
  3. 提交文件入库 (commit_uploaded_file)

上传位置优先级: --folder > --workspace > 默认 (我的文档根目录)`,
		Example: `  dws doc upload --file ./report.pdf
  dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder DOC_FOLDER_NODE_ID
  dws doc upload --file ./data.xlsx --workspace WS_ID --convert`,
		RunE: runDocUpload,
	}
	DeclareLeafMetadata(uploadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "upload",
				CanonicalPath:  "doc.upload",
				CLIPath:        "doc upload",
				PrimaryCLIPath: "doc upload",
			},
			Description: "兼容入口：上传本地文件到钉盘或文档空间；文件上传能力已迁移到 drive。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令包含多个 RPC、条件分派或本地 HTTP/文件步骤，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：上传本地文件到钉盘或文档空间；文件上传能力已迁移到 drive。",
				UseWhen:      []string{"把本地文件上传到文档空间/知识库（可 --convert 转在线文档）时"},
				AvoidWhen: []string{
					"钉盘/我的文件上传优先 dws drive upload",
					"插入文档正文附件用 media insert",
				},
				Examples: []string{
					"dws doc upload --file ./report.pdf --format json",
					"dws doc upload --file ./data.xlsx --workspace <WS_ID> --convert --format json",
				},
			},
		},
	})

	downloadCmd := &cobra.Command{
		Use:   "download",
		Short: "下载文件",
		Long: `下载钉钉文档空间中的文件到本地（两步下载流程）。

流程:
  1. 获取下载 URL 和签名请求头 (download_file)
  2. HTTP GET 下载文件二进制内容到本地`,
		Example: `  dws doc download --node NODE_ID --output ./download.bin
  dws doc download --node NODE_ID --output ./report.pdf
  dws doc download --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --output ~/downloads/`,
		RunE: runDocDownload,
	}
	DeclareLeafMetadata(downloadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "download_file",
				CanonicalPath:  "doc.download_file",
				CLIPath:        "doc download",
				PrimaryCLIPath: "doc download",
			},
			Description: "兼容入口：下载钉盘或文档空间已有文件；文件下载能力已迁移到 drive。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "download_file"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：下载钉盘或文档空间已有文件；文件下载能力已迁移到 drive。",
				UseWhen:      []string{"兼容入口：获取文件下载凭证（已迁移场景优先 drive download）时"},
				AvoidWhen: []string{
					"常规下载普通文件改用 dws drive download --output ...",
					"在线文档导出 docx 用 doc export",
				},
				Examples: []string{"dws doc download --node <NODE_ID> --output ./report.pdf --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	blockCmd := &cobra.Command{
		Use:   "block",
		Short: "块级编辑",
		Long:  `对文档进行块级别的精细编辑：查询、插入、更新、删除块元素。`,
		RunE:  groupRunE,
	}

	blockListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询块元素",
		Long:  `查询文档的一级块元素列表，支持按位置范围和块类型过滤。`,
		Example: `  dws doc block list --node DOC_ID
  dws doc block list --node DOC_ID --start-index 0 --end-index 5
  dws doc block list --node DOC_ID --block-type heading`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateDocFormat(cmd, []string{"", "element", "jsonml"}, "doc block list",
				"dws doc block list --node DOC_ID --content-format jsonml"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId": nodeID,
			}
			if v, _ := cmd.Flags().GetInt("start-index"); cmd.Flags().Changed("start-index") {
				toolArgs["startIndex"] = v
			}
			if v, _ := cmd.Flags().GetInt("end-index"); cmd.Flags().Changed("end-index") {
				toolArgs["endIndex"] = v
			}
			if v, _ := cmd.Flags().GetString("block-type"); v != "" {
				toolArgs["blockType"] = v
			}
			if v, _ := cmd.Flags().GetString("content-format"); v != "" {
				toolArgs["format"] = v
			}
			if v, _ := cmd.Flags().GetString("block-id"); v != "" {
				toolArgs["blockId"] = v
			}
			return callMCPTool("list_document_blocks", toolArgs)
		},
	}
	DeclareLeafMetadata(blockListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "list_document_blocks",
				CanonicalPath:  "doc.list_document_blocks",
				CLIPath:        "doc block list",
				PrimaryCLIPath: "doc block list",
			},
			Description: "查询文档一级块元素列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "list_document_blocks"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询文档一级块元素列表",
				UseWhen:      []string{"查看文档一级块结构、拿 blockId，供 insert/update/delete 或划词评论定位时"},
				AvoidWhen: []string{
					"只要全文 Markdown 用 doc read",
					"要改内容分别用 block insert/update/delete",
				},
				Examples: []string{
					"dws doc block list --node <DOC_ID> --format json",
					"dws doc block list --node <DOC_ID> --start-index 0 --end-index 5 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content-format", Property: "format"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	blockInsertCmd := &cobra.Command{
		Use:   "insert",
		Short: "插入块元素",
		Long: `向文档插入块元素。通过 --element 传入 JSON 格式的块元素。
可用 --text 快速插入段落，--heading + --level 快速插入标题。

块类型: paragraph, heading, blockquote, callout, columns,
        orderedList, unorderedList, table, sheet, attachment, slot`,
		Example: `  # 快捷插入段落
  dws doc block insert --node DOC_ID --text "这是一段文字"

  # 快捷插入标题
  dws doc block insert --node DOC_ID --heading "二级标题" --level 2

  # 高级: 用 JSON 插入任意块
  dws doc block insert --node DOC_ID --element '{"blockType":"paragraph","paragraph":{"text":"内容"}}'

  # 插入分栏块(columns)：2 栏，children 为每栏内容
  dws doc block insert --node DOC_ID --element '{"blockType":"columns","columns":{"size":2},"children":[{"blockType":"paragraph","paragraph":{"text":"左栏内容"}},{"blockType":"paragraph","paragraph":{"text":"右栏内容"}}]}'

  # 插入附件块(attachment)：resourceId 通过 media insert 上传后获得
  dws doc block insert --node DOC_ID --element '{"blockType":"attachment","attachment":{"resourceId":"<RESOURCE_ID>","type":"application/pdf","name":"报告.pdf","viewType":"preview"}}'

  # 插入有序列表(orderedList)：同一列表每次只插入一个 item，listId 相同则属于同一列表
  dws doc block insert --node DOC_ID --element '{"blockType":"orderedList","orderedList":{"list":{"listId":"list-1"}},"children":[{"text":"第一项"}]}'
  dws doc block insert --node DOC_ID --element '{"blockType":"orderedList","orderedList":{"list":{"listId":"list-1"}},"children":[{"text":"第二项"}]}'

  # 插入无序列表(unorderedList)：同一列表每次只插入一个 item，listId 相同则属于同一列表
  dws doc block insert --node DOC_ID --element '{"blockType":"unorderedList","unorderedList":{"list":{"listId":"list-2"}},"children":[{"text":"第一项"}]}'
  dws doc block insert --node DOC_ID --element '{"blockType":"unorderedList","unorderedList":{"list":{"listId":"list-2"}},"children":[{"text":"第二项"}]}'

  # 在指定位置之前插入
  dws doc block insert --node DOC_ID --text "插入内容" --ref-block BLOCK_ID --where before`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateDocFormat(cmd, []string{"", "element", "jsonml"}, "doc block insert",
				"dws doc block insert --node DOC_ID --content-format jsonml --element '[\"p\",{},\"hello\"]'"); err != nil {
				return err
			}
			format, _ := cmd.Flags().GetString("content-format")
			if format != "jsonml" {
				if el, _ := cmd.Flags().GetString("element"); el != "" && sniffJsonMLLike(el) {
					deps.Out.PrintWarning(`--element 内容看起来是 JSONML 结构（首字符 '[' 后紧跟 JSON 字符串）。`)
					deps.Out.PrintWarning(`若要按 JSONML 解析，请加 --content-format jsonml；否则将按 element 解析。`)
				}
			}
			if format == "jsonml" {
				elementStr := mustGetFlag(cmd, "element")
				normalized, err := prepareJsonMLNode(cmd, elementStr)
				if err != nil {
					return err
				}
				toolArgs := map[string]any{
					"nodeId": nodeID,
					"jsonml": normalized,
					"format": "jsonml",
				}
				if v, _ := cmd.Flags().GetString("ref-block"); v != "" {
					toolArgs["referenceBlockId"] = v
					where, _ := cmd.Flags().GetString("where")
					if where == "" {
						where = "after"
					}
					toolArgs["where"] = where
				}
				if v, _ := cmd.Flags().GetString("parent-block"); v != "" {
					toolArgs["referenceBlockId"] = v
				}
				if cmd.Flags().Changed("index") {
					idx, _ := cmd.Flags().GetInt("index")
					toolArgs["index"] = idx
				}
				return callMCPTool("insert_document_block", toolArgs)
			}
			element, err := buildBlockElement(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"element": element,
			}
			if v, _ := cmd.Flags().GetInt("index"); cmd.Flags().Changed("index") {
				toolArgs["index"] = v
			}
			if v, _ := cmd.Flags().GetString("where"); v != "" {
				toolArgs["where"] = v
			}
			if v, _ := cmd.Flags().GetString("ref-block"); v != "" {
				toolArgs["referenceBlockId"] = v
			}
			return callMCPTool("insert_document_block", toolArgs)
		},
	}
	DeclareLeafMetadata(blockInsertCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "insert_document_block",
				CanonicalPath:  "doc.insert_document_block",
				CLIPath:        "doc block insert",
				PrimaryCLIPath: "doc block insert",
			},
			Description: "向文档插入块元素",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "insert_document_block"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "向文档插入块元素",
				UseWhen:      []string{"在文档中插入新块（段落/标题等）；简单场景用 --text/--heading，复杂块用 --element JSON"},
				AvoidWhen: []string{
					"整篇追加 Markdown 优先 doc update --mode append",
					"插入本地文件附件优先 doc media insert",
					"删块用 block delete；改已有块用 block update",
				},
				Examples: []string{
					"dws doc block insert --node <DOC_ID> --text \"这是一段文字\" --format json",
					"dws doc block insert --node <DOC_ID> --heading \"二级标题\" --level 2 --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content-format", Property: "format"},
				{Name: "node", Property: "nodeId"},
				{Name: "parent-block", Property: "referenceBlockId"},
				{Name: "ref-block", Property: "referenceBlockId"},
			},
		},
	})

	blockUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新块元素",
		Long: `更新文档中指定块的内容或样式。需提供 --block-id 和块元素内容。
可用 --text 快速更新为段落，--heading + --level 快速更新为标题。`,
		Example: `  dws doc block update --node DOC_ID --block-id BLOCK_ID --text "新内容"    # 查询 nodeId: dws doc search --query "..." 或 dws doc list  # 查询 blockId: dws doc block list --node <nodeId>
  dws doc block update --node DOC_ID --block-id BLOCK_ID --element '{"blockType":"heading","heading":{"text":"新标题","level":1}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "block-id"); err != nil {
				return err
			}
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			blockID := mustGetFlag(cmd, "block-id")
			if err := validateDocFormat(cmd, []string{"", "element", "jsonml"}, "doc block update",
				"dws doc block update --node DOC_ID --block-id BLOCK_ID --content-format jsonml --element '[\"p\",{},\"new\"]'"); err != nil {
				return err
			}
			format, _ := cmd.Flags().GetString("content-format")
			if format != "jsonml" {
				if el, _ := cmd.Flags().GetString("element"); el != "" && sniffJsonMLLike(el) {
					deps.Out.PrintWarning(`--element 内容看起来是 JSONML 结构（首字符 '[' 后紧跟 JSON 字符串）。`)
					deps.Out.PrintWarning(`若要按 JSONML 解析，请加 --content-format jsonml；否则将按 element 解析。`)
				}
			}
			if format == "jsonml" {
				elementStr := mustGetFlag(cmd, "element")
				normalized, err := prepareJsonMLNode(cmd, elementStr)
				if err != nil {
					return err
				}
				return callMCPTool("update_document_block", map[string]any{
					"nodeId":  nodeID,
					"blockId": blockID,
					"jsonml":  normalized,
					"format":  "jsonml",
				})
			}
			element, err := buildBlockElement(cmd)
			if err != nil {
				return err
			}
			return callMCPTool("update_document_block", map[string]any{
				"nodeId":  nodeID,
				"blockId": blockID,
				"element": element,
			})
		},
	}
	DeclareLeafMetadata(blockUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "update_document_block",
				CanonicalPath:  "doc.update_document_block",
				CLIPath:        "doc block update",
				PrimaryCLIPath: "doc block update",
			},
			Description: "更新文档中的指定块",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "update_document_block"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新文档中的指定块",
				UseWhen:      []string{"修改已有块的文本/标题/样式（已知 blockId）时"},
				AvoidWhen: []string{
					"插入新块用 block insert；删除用 block delete",
					"改文档显示名用 rename；整篇覆盖用 update overwrite",
				},
				Examples: []string{"dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text \"新内容\" --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content-format", Property: "format"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	blockDeleteCmd := &cobra.Command{
		Use:     "delete",
		Short:   "删除块元素",
		Example: `  dws doc block delete --node DOC_ID --block-id BLOCK_ID --yes    # 查询 nodeId: dws doc search --query "..." 或 dws doc list  # 查询 blockId: dws doc block list --node <nodeId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "block-id"); err != nil {
				return err
			}
			blockID := mustGetFlag(cmd, "block-id")
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPTool("delete_document_block", map[string]any{
				"nodeId":  nodeID,
				"blockId": blockID,
			})
		},
	}
	DeclareLeafMetadata(blockDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "delete_document_block",
				CanonicalPath:  "doc.delete_document_block",
				CLIPath:        "doc block delete",
				PrimaryCLIPath: "doc block delete",
			},
			Description: "删除块元素（不可逆）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "delete_document_block"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除块元素（不可逆）",
				UseWhen:      []string{"用户确认后删除文档中指定块元素时"},
				AvoidWhen: []string{
					"未确认或 blockId 不明时不要删；先 block list",
					"删整篇文档用 doc/drive delete",
				},
				Examples: []string{"dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	copyCmd := &cobra.Command{
		Use:   "copy",
		Short: "复制文档/文件到指定位置",
		Long: `将文档或文件复制到指定文件夹或知识库。
--folder 指定目标文档文件夹 nodeId 或 alidocs 文件夹 URL，--workspace 指定目标知识库 ID。
不要把 drive/chat 链路返回的纯数字 dentryId/parent-id 传给 --folder。
不传 --folder 时复制到 --workspace 知识库根目录；都不传则默认到"我的文档"。

权限要求: 对源文档有"阅读"权限，且对目标文件夹有"编辑"权限。`,
		Example: `  dws doc copy --node DOC_ID --folder TARGET_DOC_FOLDER_NODE_ID
  dws doc copy --node DOC_ID --workspace TARGET_WS_ID
  dws doc copy --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --folder DOC_FOLDER_NODE_ID`,
		RunE: buildNodeTransferRunE("copy_document"),
	}
	DeclareLeafMetadata(copyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "copy_document",
				CanonicalPath:  "doc.copy_document",
				CLIPath:        "doc copy",
				PrimaryCLIPath: "doc copy",
			},
			Description: "兼容入口：复制文档或文件；文件复制能力已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "copy_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：复制文档或文件；文件复制能力已迁移到 drive。",
				UseWhen: []string{
					"用户要复制文档/文件并保留原位置时（尤其保形复制模板：copy + rename + block update）",
					"目标文件夹 --folder 或知识库根 --workspace；都不传则默认「我的文档」",
				},
				AvoidWhen: []string{
					"要搬走不留副本改用 dws doc move 或 dws drive move",
					"跨钉盘整理文件优先 dws drive copy",
				},
				Examples: []string{
					"dws doc copy --node <DOC_ID> --folder <TARGET_FOLDER_ID> --format json",
					"dws doc copy --node <DOC_ID> --workspace <TARGET_WS_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	moveCmd := &cobra.Command{
		Use:   "move",
		Short: "移动文档/文件到指定位置",
		Long: `将文档或文件移动到指定文件夹或知识库。移动后原位置的文档将不再存在。
--folder 指定目标文档文件夹 nodeId 或 alidocs 文件夹 URL，--workspace 指定目标知识库 ID。
不要把 drive/chat 链路返回的纯数字 dentryId/parent-id 传给 --folder。
不传 --folder 时移动到 --workspace 知识库根目录；都不传则默认到"我的文档"。

权限要求: 对源文档有"管理"权限，且对目标文件夹有"编辑"权限。`,
		Example: `  dws doc move --node DOC_ID --folder TARGET_DOC_FOLDER_NODE_ID
  dws doc move --node DOC_ID --workspace TARGET_WS_ID
  dws doc move --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --folder DOC_FOLDER_NODE_ID`,
		RunE: buildNodeTransferRunE("move_document"),
	}
	DeclareLeafMetadata(moveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "move_document",
				CanonicalPath:  "doc.move_document",
				CLIPath:        "doc move",
				PrimaryCLIPath: "doc move",
			},
			Description: "兼容入口：移动文档或文件；文件移动能力已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "move_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：移动文档或文件；文件移动能力已迁移到 drive。",
				UseWhen:      []string{"用户要移动文档/文件且原位置不再保留时"},
				AvoidWhen: []string{
					"要保留副本改用 doc/drive copy",
					"目标未确认时不要 move",
				},
				Examples: []string{
					"dws doc move --node <DOC_ID> --folder <TARGET_FOLDER_ID> --format json",
					"dws doc move --node <DOC_ID> --workspace <TARGET_WS_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "targetFolderId"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "重命名文档/文件",
		Long: `修改文档或文件的名称。用户说重命名、rename、改名、修改文档名称/标题时走 doc rename。
不要用 doc update 修改列表/链接展示名称，也不要重新 create。

权限要求: 对文档有"编辑"权限。`,
		Example: `  dws doc rename --node DOC_ID --name "新名称"
  dws doc rename --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --name "项目周报 v2"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "title"); err != nil {
				return err
			}
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPTool("rename_document", map[string]any{
				"nodeId":  nodeID,
				"newName": flagOrFallback(cmd, "name", "title"),
			})
		},
	}
	DeclareLeafMetadata(renameCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "rename_document",
				CanonicalPath:  "doc.rename_document",
				CLIPath:        "doc rename",
				PrimaryCLIPath: "doc rename",
			},
			Description: "兼容入口：重命名文档或文件；文件重命名能力已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "rename_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：重命名文档或文件；文件重命名能力已迁移到 drive。",
				UseWhen:      []string{"用户要改在线文档在列表与链接中展示的名称时"},
				AvoidWhen: []string{
					"改正文标题/章节标题改用 doc block update",
					"不要用 update 或重新 create 来改名",
					"文件或文件夹重命名优先用 dws drive rename，由该命令读取真实节点类型和当前扩展名",
				},
				Examples: []string{"dws doc rename --node <DOC_ID> --name \"新名称\" --format json"},
			},
			// No ParamDecl for --name here: the extension-stripping description
			// belongs to drive rename (shared RPC rename_document). doc rename
			// keeps the Cobra usage ("原样传给服务端").
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "newName"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除文档/文件到回收站",
		Long: `将文档或文件移入回收站。

注意: 这是一个危险操作，文档将被移入回收站。执行前需要确认，或传入 --yes 跳过确认。

权限要求: 对文档有"管理"权限。`,
		Example: `  dws doc delete --node DOC_ID --yes    # 查询 nodeId: dws doc search --query "..." 或 dws doc list
  dws doc delete --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --yes
  dws doc delete --node DOC_ID          # 交互式确认后删除`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPTool("delete_document", map[string]any{
				"nodeId": nodeID,
			})
		},
	}
	DeclareLeafMetadata(deleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "delete_document",
				CanonicalPath:  "doc.delete_document",
				CLIPath:        "doc delete",
				PrimaryCLIPath: "doc delete",
			},
			Description: "兼容入口：将文档或文件移入回收站；文件删除能力已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "delete_document"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：将文档或文件移入回收站；文件删除能力已迁移到 drive。",
				UseWhen:      []string{"用户明确要求用 doc delete 兼容入口将文档/文件移入回收站，且已确认目标时"},
				AvoidWhen: []string{
					"常规文件删除优先 dws drive delete；本入口仅为兼容",
					"用户未确认或目标不清时不要删",
					"删块用 doc block delete；删评论用 doc comment delete",
				},
				Examples: []string{"dws doc delete --node <DOC_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
			},
		},
	})

	// search
	searchCmd.Flags().String("query", "", "搜索关键词 (不传则返回最近访问)")
	searchCmd.Flags().String("keyword", "", "搜索关键词 (--query 的别名)")
	_ = searchCmd.Flags().MarkHidden("keyword")
	searchCmd.Flags().StringSlice("extensions", nil, "按文件扩展名过滤，不含点号，逗号分隔 (如 pdf,docx,png)。支持的在线文档类型后缀名: adoc=文字, axls=表格, appt=演示文稿, awbd=白板, adraw=画板, amind=脑图, able=多维表格, aform=收集表")
	searchCmd.Flags().Int64("created-from", 0, "创建时间起始 (毫秒时间戳，含)")
	searchCmd.Flags().Int64("created-to", 0, "创建时间截止 (毫秒时间戳，含)")
	searchCmd.Flags().Int64("visited-from", 0, "访问时间起始 (毫秒时间戳，含)")
	searchCmd.Flags().Int64("visited-to", 0, "访问时间截止 (毫秒时间戳，含)")
	searchCmd.Flags().StringSlice("creator-uids", nil, "按创建者用户 ID 过滤，逗号分隔")
	searchCmd.Flags().StringSlice("editor-uids", nil, "按编辑者用户 ID 过滤，逗号分隔")
	searchCmd.Flags().StringSlice("mentioned-uids", nil, "按 @提及的用户 ID 过滤，逗号分隔")
	searchCmd.Flags().StringSlice("workspace-ids", nil, "按知识库 ID 过滤，支持知识库 URL，逗号分隔")
	searchCmd.Flags().Int("limit", 0, "每页数量 (默认 10，最大 30)")
	searchCmd.Flags().Int("page-size", 0, "")
	_ = searchCmd.Flags().MarkHidden("page-size")
	searchCmd.Flags().String("cursor", "", "分页游标 (从上次结果的 nextPageToken 获取)")
	searchCmd.Flags().String("page-token", "", "")
	_ = searchCmd.Flags().MarkHidden("page-token")

	// list
	listCmd.Flags().String("folder", "", "文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	listCmd.Flags().String("workspace", "", "知识库 ID")
	listCmd.Flags().Int("limit", 0, "每页数量 (默认 50，最大 50)")
	listCmd.Flags().Int("page-size", 0, "")
	_ = listCmd.Flags().MarkHidden("page-size")
	listCmd.Flags().String("cursor", "", "分页游标 (从上次结果的 nextPageToken 获取)")
	listCmd.Flags().String("page-token", "", "")
	_ = listCmd.Flags().MarkHidden("page-token")
	// ── cross-product hidden aliases for list ──
	listCmd.Flags().String("parent-id", "", "")
	_ = listCmd.Flags().MarkHidden("parent-id")
	// 跨语义组别名: --node / --file-id 在 list 场景等价于 --folder。
	// 这两个 flag 属于 "节点标识" 语义组, 与 "父文件夹" 组独立,
	// 不会被 RegisterCrossProductAliases 自动注册, 故显式注册为 hidden。
	listCmd.Flags().String("node", "", "")
	_ = listCmd.Flags().MarkHidden("node")
	listCmd.Flags().String("file-id", "", "")
	_ = listCmd.Flags().MarkHidden("file-id")

	// info
	infoCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")

	// read
	readCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	readCmd.Flags().String("content-format", "", "输出格式: 默认为 markdown，可选 jsonml")
	readCmd.Flags().String("output", "", "输出到本地文件路径（仅 --content-format jsonml 时生效）")
	readCmd.Flags().String("scope", "", "按 scope 筛选节点(需 --content-format jsonml): outline(全部 h1-h6 标题)/range(区间)/section(单块)/tags(配合 --tags 自定义 tag)")
	readCmd.Flags().String("tags", "", "自定义 JSONML tag 列表(逗号分隔, 如 h1,h2,table); 仅在 --scope tags 时使用且必填")
	readCmd.Flags().Int("max-depth", 0, "筛选遍历最大深度, 0 表示不限(仅 --scope 时生效)")
	readCmd.Flags().String("start-block-id", "", "range/section 起始块 ID(节点 uuid); scope=range/section 时必填")
	readCmd.Flags().String("end-block-id", "", "range 结束块 ID(节点 uuid); \"-1\"或空=到文档末尾(仅 scope=range 生效)")
	cli.AnnotateRuntimeFlagEnum(readCmd, "scope", "outline", "range", "section", "tags")
	cli.AnnotateRuntimeFlagRequiredWhen(readCmd, "tags", "--scope=tags")
	cli.AnnotateRuntimeFlagRequiredWhen(readCmd, "start-block-id", "--scope=range or --scope=section")

	// create
	createCmd.Flags().String("name", "", "文档名称 (必填)")
	createCmd.Flags().String("folder", "", "目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	createCmd.Flags().String("workspace", "", "目标知识库 ID")
	createCmd.Flags().String("content", "", "文档初始内容（短文本字面量）；传 - 表示从 stdin 读取")
	createCmd.Flags().String("content-file", "", "从文件读取文档内容（UTF-8）。推荐长/多行/表格内容使用")
	createCmd.Flags().String("markdown", "", "已弃用，请使用 --content 代替")
	_ = createCmd.Flags().MarkHidden("markdown")
	createCmd.Flags().String("content-format", "", "内容格式: 默认为 markdown，可选 jsonml")
	addJsonMLFlags(createCmd)

	// update
	updateCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	updateCmd.Flags().String("content", "", "文档内容（短文本字面量）；传 - 表示从 stdin 读取")
	updateCmd.Flags().String("content-file", "", "从文件读取文档内容（UTF-8）。推荐长/多行/表格内容使用")
	updateCmd.Flags().String("markdown", "", "已弃用，请使用 --content 代替")
	_ = updateCmd.Flags().MarkHidden("markdown")
	updateCmd.Flags().String("mode", "", "更新模式: overwrite=覆盖, append=追加 (必填)")
	_ = updateCmd.MarkFlagRequired("mode")
	updateCmd.Flags().Int("index", -1, "插入位置（从 0 开始），仅在 mode=append 时生效。指定将内容插入到文档第几个 block 之前。不传时追加到末尾")
	updateCmd.Flags().Bool("yes", false, "确认执行破坏性写入 (仅 --mode overwrite 需要)")
	updateCmd.Flags().Bool("dry-run", false, "预览覆盖写入差异，不调用远端 update")
	updateCmd.Flags().String("content-format", "", "内容格式: 默认为 markdown，可选 jsonml")
	updateCmd.Flags().Int("revision", 0,
		"可选，文档编辑版本号（仅 --content-format jsonml 生效）；"+
			"传则触发并发检查（与服务端不一致时拒绝写入），不传则直接覆盖")
	addJsonMLFlags(updateCmd)

	fileCreateCmd.Flags().String("name", "", "文件名称 (必填)")
	fileCreateCmd.Flags().String("type", "", "文件类型: adoc/axls/appt/adraw/amind/able/folder (必填)")
	fileCreateCmd.Flags().String("folder", "", "目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	fileCreateCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL")
	fileCmd.AddCommand(fileCreateCmd)
	fileCmd.AddCommand(hintSubCmd("search", "use: dws doc search --query <关键词>"))

	// folder create
	folderCreateCmd.Flags().String("name", "", "文件夹名称 (必填)")
	folderCreateCmd.Flags().String("folder", "", "父文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	folderCreateCmd.Flags().String("workspace", "", "目标知识库 ID")
	folderCmd.AddCommand(folderCreateCmd)

	// upload
	uploadCmd.Flags().String("file", "", "本地文件路径 (必填)")
	uploadCmd.Flags().String("name", "", "文件显示名称 (默认使用文件名)")
	uploadCmd.Flags().String("folder", "", "目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	uploadCmd.Flags().String("workspace", "", "目标知识库 ID")
	uploadCmd.Flags().Bool("convert", false, "是否转换为钉钉在线文档")

	// download
	downloadCmd.Flags().String("node", "", "文件节点 ID 或 URL (必填)")
	downloadCmd.Flags().String("output", "", "本地保存路径 (文件路径或目录)")
	_ = downloadCmd.MarkFlagRequired("node")
	_ = downloadCmd.MarkFlagRequired("output")

	// block list
	blockListCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	blockListCmd.Flags().Int("start-index", 0, "起始位置 (从 0 开始)")
	blockListCmd.Flags().Int("end-index", 0, "终止位置 (含)")
	blockListCmd.Flags().String("block-type", "", "按块类型过滤")

	blockListCmd.Flags().String("content-format", "", "输出格式: 默认为 element，可选 jsonml（返回 JSONML 节点数组）")
	blockListCmd.Flags().String("block-id", "", "指定块 UUID（content-format=jsonml 时读取完整子树）")

	// block insert
	blockInsertCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	blockInsertCmd.Flags().String("text", "", "快捷: 段落文本内容")
	blockInsertCmd.Flags().String("heading", "", "快捷: 标题文本")
	blockInsertCmd.Flags().Int("level", 1, "标题级别 1-6 (配合 --heading)")
	blockInsertCmd.Flags().String("element", "", "块元素 JSON (高级)")
	blockInsertCmd.Flags().Int("index", 0, "参照位置索引 (从 0 开始)")
	blockInsertCmd.Flags().String("where", "", "插入方向: before / after (默认 after)")
	blockInsertCmd.Flags().String("ref-block", "", "参照块 ID (优先级高于 --index)")
	blockInsertCmd.Flags().String("content-format", "", "输入格式: 默认为 element，可选 jsonml")
	blockInsertCmd.Flags().String("parent-block", "", "父容器 UUID（容器内插入时使用，与 --index 配合）")
	addJsonMLFlags(blockInsertCmd)

	// block update
	blockUpdateCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	blockUpdateCmd.Flags().String("block-id", "", "目标块 ID (必填)")
	blockUpdateCmd.Flags().String("text", "", "快捷: 段落文本内容")
	blockUpdateCmd.Flags().String("heading", "", "快捷: 标题文本")
	blockUpdateCmd.Flags().Int("level", 1, "标题级别 1-6 (配合 --heading)")
	blockUpdateCmd.Flags().String("element", "", "块元素 JSON (高级)")
	blockUpdateCmd.Flags().String("content-format", "", "输入格式: 默认为 element，可选 jsonml")
	addJsonMLFlags(blockUpdateCmd)

	// block delete
	blockDeleteCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	blockDeleteCmd.Flags().String("block-id", "", "目标块 ID (必填)")

	blockCmd.AddCommand(blockListCmd, blockInsertCmd, blockUpdateCmd, blockDeleteCmd)

	// copy
	copyCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	copyCmd.Flags().String("folder", "", "目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	copyCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL (不传 --folder 时复制到该知识库根目录)")

	// move
	moveCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	moveCmd.Flags().String("folder", "", "目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id")
	moveCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL (不传 --folder 时移动到该知识库根目录)")

	// rename
	renameCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")
	renameCmd.Flags().String("name", "", "新名称 (必填；原样传给服务端，不做扩展名规范化；如需根据节点类型和当前后缀规范化，请使用 drive rename)")

	// delete
	deleteCmd.Flags().String("node", "", "文档/文件 ID 或 URL (必填)")

	// 别名注册: --node 的隐藏别名 (--url/--id/--node-id/--doc-id/--file-id)
	nodeAliasCmds := []*cobra.Command{
		infoCmd, readCmd, updateCmd, downloadCmd,
		blockListCmd, blockInsertCmd, blockUpdateCmd, blockDeleteCmd,
		copyCmd, moveCmd, renameCmd, deleteCmd,
	}
	for _, c := range nodeAliasCmds {
		c.Flags().String("url", "", "--node 的别名")
		c.Flags().String("id", "", "--node 的别名")
		c.Flags().String("node-id", "", "--node 的别名")
		c.Flags().String("doc-id", "", "--node 的别名")
		c.Flags().String("file-id", "", "--node 的别名 (跨产品兼容 drive)")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	// 别名注册: doc create/file create/folder create/rename --title → --name
	createCmd.Flags().String("title", "", "--name 的别名")
	_ = createCmd.Flags().MarkHidden("title")
	fileCreateCmd.Flags().String("title", "", "--name 的别名")
	_ = fileCreateCmd.Flags().MarkHidden("title")
	folderCreateCmd.Flags().String("title", "", "--name 的别名")
	_ = folderCreateCmd.Flags().MarkHidden("title")
	renameCmd.Flags().String("title", "", "--name 的别名")
	_ = renameCmd.Flags().MarkHidden("title")

	// ── media (文档媒体/附件) ────────────────────────────────
	mediaCmd := &cobra.Command{
		Use:   "media",
		Short: "文档媒体 / 附件管理",
		Long:  `管理钉钉文档中的媒体资源和附件：上传附件并插入文档、下载文档内的附件等。`,
		RunE:  groupRunE,
	}

	mediaDownloadCmd := &cobra.Command{
		Use:   "download",
		Short: "下载文档附件",
		Long: `获取钉钉文档中指定附件的 OSS 临时下载链接。

传入 nodeId（文档标识）和 resourceId（附件资源 ID），返回 downloadUrl。
resourceId 需通过 dws doc block list 获取：查询目标文档的块列表，
找到 blockType 为 attachment 的元素，取其 resourceId。`,
		Example: `  dws doc media download --node DOC_ID --resource-id RESOURCE_ID
  dws doc media download --node "https://alidocs.dingtalk.com/i/nodes/xxx" --resource-id RESOURCE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "resource-id"); err != nil {
				return err
			}
			return callMCPToolUnescaped("download_doc_attachment", map[string]any{
				"nodeId":     nodeID,
				"resourceId": mustGetFlag(cmd, "resource-id"),
			})
		},
	}
	DeclareLeafMetadata(mediaDownloadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "download_doc_attachment",
				CanonicalPath:  "doc.download_doc_attachment",
				CLIPath:        "doc media download",
				PrimaryCLIPath: "doc media download",
			},
			Description: "获取文档附件的临时下载链接",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "download_doc_attachment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取文档附件的临时下载链接",
				UseWhen:      []string{"获取文档正文中附件的临时下载 URL（resourceId 来自 block list attachment）时"},
				AvoidWhen:    []string{"下载钉盘普通文件用 drive download；导出在线文档用 doc export"},
				Examples:     []string{"dws doc media download --node <DOC_ID> --resource-id <RESOURCE_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	mediaDownloadCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	mediaDownloadCmd.Flags().String("resource-id", "", "附件资源 ID，可通过 dws doc block list 获取 (必填)")

	mediaUploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "上传可复用的文档媒体资源",
		Long: `将本地文件上传为绑定到目标 nodeId 的文档媒体资源，但不插入文档正文。

成功输出稳定的 resourceId 和 resourceUrl，可供同一 nodeId 下的白板 Vector/SVG
等后续写入使用；临时 uploadUrl 不会输出。`,
		Example: `  dws doc media upload --node DOC_ID --file ./icon.svg --mime-type image/svg+xml --format json`,
		RunE:    runDocMediaUpload,
	}
	DeclareLeafMetadata(mediaUploadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "media_upload",
				CanonicalPath:  "doc.media_upload",
				CLIPath:        "doc media upload",
				PrimaryCLIPath: "doc media upload",
			},
			Description: "上传可复用的文档媒体资源",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令先获取临时文档上传凭证，再在本地执行 OSS PUT，并仅暴露稳定的 node 绑定资源契约，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后上传绑定到文档 nodeId 的可复用媒体资源而不插入正文",
				UseWhen:      []string{"为同一文档内白板的 Vector/SVG 写入准备 resourceId 和 resourceUrl 时"},
				AvoidWhen:    []string{"需要把附件直接插入文档正文时用 doc media insert；不要跨 nodeId 复用资源"},
				Examples:     []string{"dws doc media upload --node <DOC_ID> --file ./icon.svg --mime-type image/svg+xml --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "file", Required: boolPtr(true)},
			},
		},
	})
	mediaUploadCmd.Flags().String("node", "", "绑定媒体资源的文档标识，支持传入 URL 或 ID (必填)")
	mediaUploadCmd.Flags().String("file", "", "本地文件路径 (必填)")
	mediaUploadCmd.Flags().String("name", "", "资源文件名 (默认使用本地文件名)")
	mediaUploadCmd.Flags().String("mime-type", "", "文件 MIME 类型 (默认根据扩展名推断)")
	mediaUploadCmd.Flags().Bool("yes", false, "确认上传可复用文档媒体资源")

	mediaInsertCmd := &cobra.Command{
		Use:   "insert",
		Short: "上传附件并插入文档",
		Long: `将本地文件作为附件上传并插入到钉钉文档中（三步自动完成）。

流程:
  1. 获取附件上传凭证 (get_doc_attachment_upload_info)
  2. HTTP PUT 上传文件到 OSS
  3. 插入附件块到文档 (insert_document_block)

--mime-type 可选，不指定时根据文件扩展名自动推断。`,
		Example: `  # 插入 PDF 附件
  dws doc media insert --node DOC_ID --file ./report.pdf

  # 指定名称和 MIME 类型
  dws doc media insert --node DOC_ID --file ./data.bin --name "数据文件.dat" --mime-type application/octet-stream

  # 在指定块之前插入
  dws doc media insert --node DOC_ID --file ./image.png --ref-block BLOCK_ID --where before`,
		RunE: runMediaInsert,
	}
	DeclareLeafMetadata(mediaInsertCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "media_insert",
				CanonicalPath:  "doc.media_insert",
				CLIPath:        "doc media insert",
				PrimaryCLIPath: "doc media insert",
			},
			Description: "上传本地文件并作为附件插入文档",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令包含多个 RPC、条件分派或本地 HTTP/文件步骤，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "上传本地文件并作为附件插入文档",
				UseWhen:      []string{"把本地文件/图片作为附件块插入文档正文（自动 prepare+PUT+insert）时"},
				AvoidWhen: []string{
					"上传为独立文件用 drive/doc upload，不要与正文附件混淆",
					"下载正文附件用 media download",
				},
				Examples: []string{"dws doc media insert --node <DOC_ID> --file ./report.pdf --format json"},
			},
		},
	})

	mediaInsertCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	mediaInsertCmd.Flags().String("file", "", "本地文件路径 (必填)")
	mediaInsertCmd.Flags().String("name", "", "附件显示名称 (默认使用文件名)")
	mediaInsertCmd.Flags().String("mime-type", "", "文件 MIME 类型 (默认根据扩展名推断)")
	mediaInsertCmd.Flags().Int("index", 0, "插入位置索引")
	mediaInsertCmd.Flags().String("where", "", "相对位置: before / after (配合 --ref-block)")
	mediaInsertCmd.Flags().String("ref-block", "", "参考块 ID (配合 --where)")

	// media 子命令的 --node 隐藏别名
	mediaNodeAliasCmds := []*cobra.Command{mediaDownloadCmd, mediaUploadCmd, mediaInsertCmd}
	for _, c := range mediaNodeAliasCmds {
		c.Flags().String("url", "", "--node 的别名")
		c.Flags().String("id", "", "--node 的别名")
		c.Flags().String("node-id", "", "--node 的别名")
		c.Flags().String("doc-id", "", "--node 的别名")
		c.Flags().String("file-id", "", "--node 的别名 (跨产品兼容 drive)")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	mediaCmd.AddCommand(mediaDownloadCmd, mediaUploadCmd, mediaInsertCmd)

	// ── comment (文档评论) ──────────────────────────────────
	commentCmd := &cobra.Command{
		Use:   "comment",
		Short: "文档评论 / 评论管理",
		Long:  `管理钉钉文档的评论：查询评论列表、创建评论、回复评论。`,
		RunE:  groupRunE,
	}

	commentListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询文档评论列表",
		Long: `查询指定文档的评论列表，支持分页、按评论类型和解决状态过滤。

评论类型 (--type):
  global   全文评论
  inline   划词评论
  不传返回所有评论

解决状态 (--resolve-status):
  resolved    已解决
  unresolved  未解决
  不传返回所有评论`,
		Example: `  dws doc comment list --node DOC_ID
  dws doc comment list --node "https://alidocs.dingtalk.com/i/nodes/xxx" --limit 20
  dws doc comment list --node DOC_ID --type inline --resolve-status unresolved
  dws doc comment list --node DOC_ID --cursor TOKEN_FROM_PREVIOUS_PAGE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId": nodeID,
			}
			if v, _ := cmd.Flags().GetInt("limit"); cmd.Flags().Changed("limit") {
				toolArgs["pageSize"] = v
			} else if v, _ := cmd.Flags().GetInt("page-size"); cmd.Flags().Changed("page-size") {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			if v, _ := cmd.Flags().GetString("type"); v != "" {
				toolArgs["commentType"] = v
			}
			if v, _ := cmd.Flags().GetString("resolve-status"); v != "" {
				toolArgs["resolveStatus"] = v
			}
			return callMCPToolOnServer("doc-comment", "list_comments", toolArgs)
		},
	}
	DeclareLeafMetadata(commentListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "list_comments",
				CanonicalPath:  "doc.list_comments",
				CLIPath:        "doc comment list",
				PrimaryCLIPath: "doc comment list",
			},
			Description: "查询文档评论列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc-comment", RPCName: "list_comments"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询文档评论列表",
				UseWhen:      []string{"查看文档评论列表，可按全文/划词、已解决/未解决过滤时"},
				AvoidWhen:    []string{"创建全文评论用 comment create；划词用 create-inline；回复用 reply；删除用 delete"},
				Examples: []string{
					"dws doc comment list --node <DOC_ID> --format json",
					"dws doc comment list --node <DOC_ID> --type inline --resolve-status unresolved --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "limit", Property: "pageSize"},
				{Name: "node", Property: "nodeId"},
				{Name: "type", Property: "commentType"},
			},
		},
	})

	commentListCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	commentListCmd.Flags().Int("limit", 50, "每页返回的评论数量，默认 50，最大 50")
	commentListCmd.Flags().Int("page-size", 0, "")
	_ = commentListCmd.Flags().MarkHidden("page-size")
	commentListCmd.Flags().String("cursor", "", "分页游标，从上一次请求的返回结果中获取 (首次请求不传)")
	commentListCmd.Flags().String("next-token", "", "")
	_ = commentListCmd.Flags().MarkHidden("next-token")
	commentListCmd.Flags().String("type", "", "按评论类型过滤: global (全文评论) / inline (划词评论)")
	commentListCmd.Flags().String("resolve-status", "", "按解决状态过滤: resolved (已解决) / unresolved (未解决)")

	commentCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文档评论",
		Long: `在指定文档上创建一条评论。

可通过 --mention 指定被 @ 的用户 uid 列表（逗号分隔），通过
--mentioned-open-conversation-id 指定被 @ 的群 openConversationId（可重复或逗号分隔）。
评论内容中会插入 @mention 节点并发送通知。
用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"`,
		Example: `  dws doc comment create --node DOC_ID --content "这里需要修改"
  dws doc comment create --node DOC_ID --content "请review" --mention uid1,uid2
  dws doc comment create --node DOC_ID --content "请群内同学关注" --mentioned-open-conversation-id openCid1 --mentioned-open-conversation-id openCid2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "content"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"content": mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("mention"); v != "" {
				toolArgs["mentionedUserIds"] = parseCommentMentionIds(v)
			}
			if err := appendCommentGroupMentions(cmd, toolArgs); err != nil {
				return err
			}
			return callMCPToolOnServer("doc-comment", "create_comment", toolArgs)
		},
	}
	DeclareLeafMetadata(commentCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "create_comment",
				CanonicalPath:  "doc.create_comment",
				CLIPath:        "doc comment create",
				PrimaryCLIPath: "doc comment create",
			},
			Description: "创建文档评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc-comment", RPCName: "create_comment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建文档评论",
				UseWhen:      []string{"在文档上创建不绑定具体划词位置的全文评论，可 @用户或通过 --mentioned-open-conversation-id @群"},
				AvoidWhen: []string{
					"针对某段选中文本的划词评论改用 create-inline（需 blockId+start/end）",
					"回复已有评论用 reply；删评论用 delete",
				},
				Examples: []string{
					"dws doc comment create --node <DOC_ID> --content \"这里需要修改\" --format json",
					"dws doc comment create --node <DOC_ID> --content \"请review\" --mention uid1,uid2 --mentioned-open-conversation-id <openConversationId> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "mentioned-open-conversation-id", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "mention", Property: "mentionedUserIds"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	commentCreateCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	commentCreateCmd.Flags().String("content", "", "评论的文字内容，纯文本 (必填)")
	commentCreateCmd.Flags().String("mention", "", "被 @ 的用户 uid 列表，逗号分隔")
	addCommentGroupMentionFlag(commentCreateCmd)

	commentReplyCmd := &cobra.Command{
		Use:   "reply",
		Short: "回复评论",
		Long: `回复指定文档中的一条评论。

--comment-key 为被回复评论的唯一标识（即 list 返回的 commentKey），格式：{13位毫秒时间戳}{32位UUID}，共45位。
commentKey可从 dws doc comment create 或 dws doc comment list 返回结果中获取。

可通过 --mention 指定被 @ 的用户 uid 列表（逗号分隔），通过
--mentioned-open-conversation-id 指定被 @ 的群 openConversationId（可重复或逗号分隔）。
评论内容中会插入 @mention 节点并发送通知。
用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"

设置 --emoji 时，本次回复将作为表情贴图回复，--content 填写表情名称。`,
		Example: `  dws doc comment reply --node DOC_ID --comment-key COMMENT_KEY --content "同意"
  dws doc comment reply --node DOC_ID --comment-key COMMENT_KEY --content "比心" --emoji
  dws doc comment reply --node DOC_ID --comment-key COMMENT_KEY --content "请确认" --mention uid1,uid2
  dws doc comment reply --node DOC_ID --comment-key COMMENT_KEY --content "请群内确认" --mentioned-open-conversation-id openCid1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "content", "comment-key"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":          nodeID,
				"content":         mustGetFlag(cmd, "content"),
				"replyCommentKey": mustGetFlag(cmd, "comment-key"),
			}
			if v, _ := cmd.Flags().GetBool("emoji"); v {
				groupMentions, err := commentGroupMentionIDs(cmd)
				if err != nil {
					return err
				}
				if len(groupMentions) > 0 {
					return fmt.Errorf("--emoji cannot be used with --mentioned-open-conversation-id: emoji replies do not support group mentions")
				}
				toolArgs["emoji"] = true
			}
			if v, _ := cmd.Flags().GetString("mention"); v != "" {
				toolArgs["mentionedUserIds"] = parseCommentMentionIds(v)
			}
			if err := appendCommentGroupMentions(cmd, toolArgs); err != nil {
				return err
			}
			return callMCPToolOnServer("doc-comment", "reply_comment", toolArgs)
		},
	}
	DeclareLeafMetadata(commentReplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "reply_comment",
				CanonicalPath:  "doc.reply_comment",
				CLIPath:        "doc comment reply",
				PrimaryCLIPath: "doc comment reply",
			},
			Description: "回复文档评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc-comment", RPCName: "reply_comment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "回复文档评论",
				UseWhen:      []string{"回复已有评论（文字、可 @用户/@群，或 --emoji 表情）；commentKey 来自 list/create"},
				AvoidWhen:    []string{"新建评论用 create/create-inline；删评论用 delete"},
				Examples: []string{
					"dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content \"同意\" --mentioned-open-conversation-id <openConversationId> --format json",
					"dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content \"比心\" --emoji --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "mentioned-open-conversation-id", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "comment-key", Property: "replyCommentKey"},
				{Name: "mention", Property: "mentionedUserIds"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	commentReplyCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	commentReplyCmd.Flags().String("content", "", "回复的文字内容，表情回复时填写表情名称 (必填)")
	commentReplyCmd.Flags().String("comment-key", "", "被回复评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)")
	commentReplyCmd.Flags().Bool("emoji", false, "设为 true 时作为表情贴图回复 (默认 false)")
	commentReplyCmd.Flags().String("mention", "", "被 @ 的用户 uid 列表，逗号分隔")
	addCommentGroupMentionFlag(commentReplyCmd)

	commentUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新文档评论",
		Long: `更新指定文档中的一条评论。

--comment-key 为待更新评论的唯一标识，可从 comment list、create 或 create-inline 的返回结果中获取。
可通过 --mention 指定更新后评论中被 @ 的用户 uid 列表，通过
--mentioned-open-conversation-id 指定被 @ 的群 openConversationId（可重复或逗号分隔）。`,
		Example: `  dws doc comment update --node DOC_ID --comment-key COMMENT_KEY --content "已按最新数据修正"
  dws doc comment update --node DOC_ID --comment-key COMMENT_KEY --content "请确认" --mention uid1,uid2
  dws doc comment update --node DOC_ID --comment-key COMMENT_KEY --content "请群内同学关注" --mentioned-open-conversation-id openCid1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "comment-key", "content"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId":     nodeID,
				"commentKey": mustGetFlag(cmd, "comment-key"),
				"content":    mustGetFlag(cmd, "content"),
			}
			if v, _ := cmd.Flags().GetString("mention"); v != "" {
				toolArgs["mentionedUserIds"] = parseCommentMentionIds(v)
			}
			if err := appendCommentGroupMentions(cmd, toolArgs); err != nil {
				return err
			}
			return callMCPToolOnServer("doc-comment", "update_comment", toolArgs)
		},
	}
	DeclareLeafMetadata(commentUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "update_comment",
				CanonicalPath:  "doc.update_comment",
				CLIPath:        "doc comment update",
				PrimaryCLIPath: "doc comment update",
			},
			Description: "更新指定文档评论的文字内容和可选 @用户/@群。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新指定文档评论的文字内容和可选 @用户/@群。",
				UseWhen:      []string{"修改已有评论正文；可选更新 --mention 或 --mentioned-open-conversation-id"},
				AvoidWhen:    []string{"删除评论用 delete；回复用 reply"},
				Examples: []string{
					"dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content \"已按最新数据修正\" --format json",
					"dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content \"请群内确认\" --mentioned-open-conversation-id <openConversationId>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "mention", Property: "mentionedUserIds", InterfaceType: "array"},
				{Name: "mentioned-open-conversation-id", Property: "mentionedOpenConversationIds", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	commentUpdateCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	commentUpdateCmd.Flags().String("comment-key", "", "待更新评论的 commentKey，可从 list/create/create-inline 结果获取 (必填)")
	commentUpdateCmd.Flags().String("content", "", "更新后的评论文字内容，纯文本 (必填)")
	commentUpdateCmd.Flags().String("mention", "", "被 @ 的用户 uid 列表，逗号分隔")
	addCommentGroupMentionFlag(commentUpdateCmd)

	commentDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除文档评论",
		Long: `删除指定文档中的一条评论。

这是不可恢复的危险操作。执行前需要交互确认，或在用户已明确同意后传入全局 --yes 跳过确认。`,
		Example: `  dws doc comment delete --node DOC_ID --comment-key COMMENT_KEY --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "comment-key"); err != nil {
				return err
			}
			commentKey := mustGetFlag(cmd, "comment-key")
			return callMCPToolOnServer("doc-comment", "delete_comment", map[string]any{
				"nodeId":     nodeID,
				"commentKey": commentKey,
			})
		},
	}
	DeclareLeafMetadata(commentDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "delete_comment",
				CanonicalPath:  "doc.delete_comment",
				CLIPath:        "doc comment delete",
				PrimaryCLIPath: "doc comment delete",
			},
			Description: "永久删除指定文档中的一条评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "永久删除指定文档中的一条评论",
				UseWhen:      []string{"用户明确要求永久删除指定文档中的某条评论（已有 commentKey）时"},
				AvoidWhen:    []string{"只需改文案用 update；回复用 reply；目标评论不明或未确认时不要删"},
				Examples:     []string{"dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	commentDeleteCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	commentDeleteCmd.Flags().String("comment-key", "", "待删除评论的 commentKey，可从 list/create/create-inline 结果获取 (必填)")

	commentCreateInlineCmd := &cobra.Command{
		Use:   "create-inline",
		Short: "创建划词评论",
		Long: `在指定文档的选中文本区域上创建一条划词评论。

需要指定评论标记所在的块 ID (--block-id)、起始偏移量 (--start) 和结束偏移量 (--end)。
块 ID 可通过 dws doc block list --node <nodeId> 获取。

可通过 --selected-text 传入选中文本内容，评论列表中会展示「引用原文：xxx」。
可通过 --mention 指定被 @ 的用户 uid 列表（逗号分隔），
评论内容中会插入 @mention 节点并发送通知。
用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"`,
		Example: `  dws doc comment create-inline --node DOC_ID --block-id BLOCK_ID --start 0 --end 10 --content "这里需要修改"
  dws doc comment create-inline --node DOC_ID --block-id BLOCK_ID --start 5 --end 20 --content "建议调整" --selected-text "被选中的原文"
  dws doc comment create-inline --node DOC_ID --block-id BLOCK_ID --start 0 --end 10 --content "请review" --mention uid1,uid2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "content", "block-id"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("start") || !cmd.Flags().Changed("end") {
				return fmt.Errorf("missing required flag(s): --start, --end")
			}
			start, _ := cmd.Flags().GetInt("start")
			end, _ := cmd.Flags().GetInt("end")
			toolArgs := map[string]any{
				"nodeId":  nodeID,
				"content": mustGetFlag(cmd, "content"),
				"blockId": mustGetFlag(cmd, "block-id"),
				"start":   start,
				"end":     end,
			}
			if v, _ := cmd.Flags().GetString("selected-text"); v != "" {
				toolArgs["selectedText"] = v
			}
			if v, _ := cmd.Flags().GetString("mention"); v != "" {
				toolArgs["mentionedUserIds"] = parseCommentMentionIds(v)
			}
			return callMCPToolOnServer("doc-comment", "create_inline_comment", toolArgs)
		},
	}
	DeclareLeafMetadata(commentCreateInlineCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "create_inline_comment",
				CanonicalPath:  "doc.create_inline_comment",
				CLIPath:        "doc comment create-inline",
				PrimaryCLIPath: "doc comment create-inline",
			},
			Description: "在文档选中文本范围创建划词评论",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc-comment", RPCName: "create_inline_comment"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在文档选中文本范围创建划词评论",
				UseWhen:      []string{"针对块内某段文本创建划词评论（必填 blockId、start、end）时"},
				AvoidWhen: []string{
					"不绑定位置的全文评论用 create",
					"先 block list 取 blockId 与纯文本偏移再调用",
				},
				Examples: []string{"dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content \"这里需要修改\" --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "mention", Property: "mentionedUserIds"},
				{Name: "node", Property: "nodeId"},
			},
		},
	})

	commentCreateInlineCmd.Flags().String("node", "", "目标文档的标识，支持传入 URL 或 ID (必填)")
	commentCreateInlineCmd.Flags().String("content", "", "评论的文字内容，纯文本 (必填)")
	commentCreateInlineCmd.Flags().String("block-id", "", "评论标记所在的块 ID，可通过 dws doc block list 获取 (必填)")
	commentCreateInlineCmd.Flags().Int("start", 0, "评论标记在块内文本中的起始字符偏移量，从 0 开始 (必填)")
	commentCreateInlineCmd.Flags().Int("end", 0, "评论标记在块内文本中的结束字符偏移量，必须大于 start (必填)")
	commentCreateInlineCmd.Flags().String("selected-text", "", "选中文本的内容，填写后评论列表中会展示「引用原文：xxx」")
	commentCreateInlineCmd.Flags().String("mention", "", "被 @ 的用户 uid 列表，逗号分隔")

	// comment 子命令的 --node 隐藏别名
	commentNodeAliasCmds := []*cobra.Command{commentListCmd, commentCreateCmd, commentReplyCmd, commentUpdateCmd, commentDeleteCmd, commentCreateInlineCmd}
	for _, c := range commentNodeAliasCmds {
		c.Flags().String("url", "", "--node 的别名")
		c.Flags().String("id", "", "--node 的别名")
		c.Flags().String("node-id", "", "--node 的别名")
		c.Flags().String("doc-id", "", "--node 的别名")
		c.Flags().String("file-id", "", "--node 的别名 (跨产品兼容 drive)")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	commentCmd.AddCommand(commentListCmd, commentCreateCmd, commentReplyCmd, commentUpdateCmd, commentDeleteCmd, commentCreateInlineCmd)

	// ── permission (文档协作权限) ────────────────────────────
	permissionCmd := &cobra.Command{
		Use:     "permission",
		Aliases: []string{"perm"},
		Short:   "文档协作权限管理",
		Long:    `管理钉钉文档的协作者权限：添加协作者、更新协作者权限、查询协作者列表。`,
		RunE:    groupRunE,
	}

	permissionAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加文档协作者",
		Long: `为指定文档（或文件夹/文件）添加一个或多个协作成员，并授予指定角色。

通过 --user 传入逗号分隔的 userId 列表，多个用户将被授予同一角色。

支持的角色 (--role)（必须大写）：
  MANAGER     管理员，可读写、管理成员
  EDITOR      编辑者，可查看、编辑、上传内容
  DOWNLOADER  查看下载者，可查看并下载内容
  READER      仅可查看者，仅可查看，不可下载

注意：
- OWNER 角色不可通过此接口添加。
- 单次请求最多 30 个成员，超出请分批调用。

用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"`,
		Example: `  dws doc permission add --node DOC_ID --users uid1 --role READER
  dws doc permission add --node DOC_ID --users uid1,uid2,uid3 --role EDITOR
  dws doc permission add --node "https://alidocs.dingtalk.com/i/nodes/xxx" --users uid1 --role MANAGER --workspace WS_ID`,
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
			return callMCPTool("add_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(permissionAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "add_permission",
				CanonicalPath:  "doc.add_permission",
				CLIPath:        "doc permission add",
				PrimaryCLIPath: "doc permission add",
			},
			Description: "兼容入口：为文档空间节点添加协作成员权限；文件管理权限命令已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "add_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：为文档空间节点添加协作成员权限；文件管理权限命令已迁移到 drive。",
				UseWhen:      []string{"给单篇文档做节点级授权（与 drive permission add 同能力的 doc 入口）时"},
				AvoidWhen: []string{
					"知识库容器授权用 wiki member add",
					"查/改/移除用 permission list/update/remove",
				},
				Examples: []string{"dws doc permission add --node <DOC_ID> --users uid1 --role READER --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "role", Property: "roleId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	permissionAddCmd.Flags().String("node", "", "目标节点的标识（文档/文件夹/文件），支持传入 URL 或 ID (必填)")
	permissionAddCmd.Flags().String("users", "", "被授权的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)")
	permissionAddCmd.Flags().String("user", "", "")
	_ = permissionAddCmd.Flags().MarkHidden("user")
	permissionAddCmd.Flags().String("role", "", "权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填，大小写不敏感)")
	permissionAddCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL（选填，仅用于辅助构造返回的 docUrl）")

	permissionUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新文档协作者权限",
		Long: `更新指定节点已有协作者的权限角色（仅支持 USER 类型成员）。

支持的角色 (--role)（必须大写）：
  MANAGER     管理员
  EDITOR      编辑者
  DOWNLOADER  查看下载者
  READER      仅可查看者

注意：
- OWNER 角色不可通过此接口变更。
- 同一成员在同一节点只能拥有一个角色，变更后旧角色自动替换。
- 若成员的角色来自父节点的权限继承（PASS_ON），且继承角色高于目标角色，接口会拒绝操作。

仅可更新已存在协作关系的用户，新增协作者请使用 dws doc permission add。`,
		Example: `  dws doc permission update --node DOC_ID --users uid1 --role EDITOR
  dws doc permission update --node DOC_ID --users uid1,uid2 --role READER`,
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
			return callMCPTool("update_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(permissionUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "update_permission",
				CanonicalPath:  "doc.update_permission",
				CLIPath:        "doc permission update",
				PrimaryCLIPath: "doc permission update",
			},
			Description: "兼容入口：更新文档空间节点的协作成员角色；文件权限命令已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "update_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：更新文档空间节点的协作成员角色；文件权限命令已迁移到 drive。",
				UseWhen:      []string{"变更文档节点上已有用户角色时"},
				AvoidWhen:    []string{"新授权用 add；移除用 remove"},
				Examples:     []string{"dws doc permission update --node <DOC_ID> --users uid1 --role EDITOR --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "role", Property: "roleId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	permissionUpdateCmd.Flags().String("node", "", "目标节点的标识（文档/文件夹/文件），支持传入 URL 或 ID (必填)")
	permissionUpdateCmd.Flags().String("users", "", "被更新的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)")
	permissionUpdateCmd.Flags().String("user", "", "")
	_ = permissionUpdateCmd.Flags().MarkHidden("user")
	permissionUpdateCmd.Flags().String("role", "", "新权限角色: MANAGER / EDITOR / DOWNLOADER / READER (必填，大小写不敏感)")
	permissionUpdateCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL（选填，仅用于辅助构造返回的 docUrl）")

	permissionListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查询文档协作者列表",
		Long: `查询指定节点的协作者列表，返回每位成员的 userId、姓名、角色等信息。

注意：底层不支持游标分页，--limit 仅控制单次返回的最大条数（最大 200）。
若结果被截断（出参 truncated=true），可通过 --filter-role 收窄查询范围。`,
		Example: `  dws doc permission list --node DOC_ID
  dws doc permission list --node DOC_ID --limit 100
  dws doc permission list --node DOC_ID --filter-role MANAGER,EDITOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"nodeId": nodeID,
			}
			limit := 0
			if cmd.Flags().Changed("limit") {
				limit, _ = cmd.Flags().GetInt("limit")
			} else if cmd.Flags().Changed("max-results") {
				limit, _ = cmd.Flags().GetInt("max-results")
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
			return callMCPTool("list_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(permissionListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "list_permission",
				CanonicalPath:  "doc.list_permission",
				CLIPath:        "doc permission list",
				PrimaryCLIPath: "doc permission list",
			},
			Description: "兼容入口：查询文档空间节点的协作者权限；文件权限命令已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "list_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：查询文档空间节点的协作者权限；文件权限命令已迁移到 drive。",
				UseWhen:      []string{"列出文档节点成员权限时"},
				AvoidWhen:    []string{"增删改权限用 add/update/remove；知识库成员用 wiki member list"},
				Examples:     []string{"dws doc permission list --node <DOC_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "filter-role", Property: "filterRoleIds"},
				{Name: "limit", Property: "maxResults"},
				{Name: "node", Property: "nodeId"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})

	permissionListCmd.Flags().String("node", "", "目标节点的标识（文档/文件夹/文件），支持传入 URL 或 ID (必填)")
	permissionListCmd.Flags().Int("limit", 30, "返回成员数上限，默认 30，最大 200")
	permissionListCmd.Flags().Int("max-results", 0, "")
	_ = permissionListCmd.Flags().MarkHidden("max-results")
	permissionListCmd.Flags().String("filter-role", "", "按角色过滤（逗号分隔）：OWNER / MANAGER / EDITOR / DOWNLOADER / READER")
	permissionListCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL（选填，仅用于辅助构造返回的 docUrl）")

	permissionRemoveCmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "移除文档协作者权限",
		Long: `从指定节点移除一个或多个协作成员的权限（仅支持 USER 类型）。

移除后相关用户将无法通过该节点的直接授权访问内容（若有父节点继承权限则仍可通过继承权限访问）。

注意：
- OWNER 角色不可通过此接口移除。
- 操作者需在该节点具备 EDITOR 及以上角色（OWNER / MANAGER / EDITOR）。
- 单次请求最多 30 个成员，超出请分批调用。

用户 uid 可通过「钉钉通讯录」相关命令检索，如:
  dws contact user search --keyword "姓名"`,
		Example: `  dws doc permission remove --node DOC_ID --users uid1
  dws doc permission remove --node DOC_ID --users uid1,uid2,uid3
  dws doc permission remove --node "https://alidocs.dingtalk.com/i/nodes/xxx" --users uid1`,
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
			return callMCPTool("remove_permission", toolArgs)
		},
	}
	DeclareLeafMetadata(permissionRemoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "remove_permission",
				CanonicalPath:  "doc.remove_permission",
				CLIPath:        "doc permission remove",
				PrimaryCLIPath: "doc permission remove",
			},
			Description: "兼容入口：移除文档空间节点的协作成员权限；文件权限命令已迁移到 drive。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "remove_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "兼容入口：移除文档空间节点的协作成员权限；文件权限命令已迁移到 drive。",
				UseWhen:      []string{"移除文档节点上指定用户权限时"},
				AvoidWhen:    []string{"改角色用 update；知识库撤成员用 wiki member remove"},
				Examples:     []string{"dws doc permission remove --node <DOC_ID> --users uid1 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "users", Property: "userIds"},
				{Name: "workspace", Property: "workspaceId"},
			},
		},
	})
	permissionRemoveCmd.Flags().String("node", "", "目标节点的标识（文档/文件夹/文件），支持传入 URL 或 ID (必填)")
	permissionRemoveCmd.Flags().String("users", "", "被移除权限的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)")
	permissionRemoveCmd.Flags().String("user", "", "")
	_ = permissionRemoveCmd.Flags().MarkHidden("user")
	permissionRemoveCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL（选填，仅用于辅助构造返回的 docUrl）")

	// permission 子命令的 --node 隐藏别名
	permissionNodeAliasCmds := []*cobra.Command{permissionAddCmd, permissionUpdateCmd, permissionListCmd, permissionRemoveCmd}
	for _, c := range permissionNodeAliasCmds {
		c.Flags().String("url", "", "--node 的别名")
		c.Flags().String("id", "", "--node 的别名")
		c.Flags().String("node-id", "", "--node 的别名")
		c.Flags().String("doc-id", "", "--node 的别名")
		c.Flags().String("file-id", "", "--node 的别名 (跨产品兼容 drive)")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	permissionCmd.AddCommand(permissionAddCmd, permissionUpdateCmd, permissionListCmd, permissionRemoveCmd)

	// ── export: 文档导出（一体化：提交→轮询→下载）──────────────
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出在线文档 (支持 docx / markdown / pdf)",
		Long: `将钉钉在线文档 (alidocs) 导出为本地文件。

支持的导出格式 (--export-format):
  docx       Word 文档 (默认)
  markdown   Markdown 文件 (.md)
  pdf        PDF 文档 (.pdf)

CLI 内部自动完成全部流程：
  1. 提交导出任务
  2. 渐进式退避轮询等待完成（最多约 5 分钟）
  3. 导出成功后自动下载文件到 --output 指定路径

如果轮询超时仍未完成，会输出 jobId 供后续手动查询：
  dws doc export get --job-id <jobId>`,
		Example: `  # 导出为 docx (默认)
  dws doc export --node "https://alidocs.dingtalk.com/i/nodes/xxx" --output ./exported.docx

  # 导出为 markdown
  dws doc export --node <DOC_ID> --export-format markdown --output ./exported.md

  # --output 传入目录时，根据 --export-format 自动追加扩展名
  dws doc export --node <DOC_ID> --export-format markdown --output ~/downloads/`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			node, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			outputPath, _ := cmd.Flags().GetString("output")
			if outputPath == "" {
				return fmt.Errorf("flag --output is required")
			}

			// 解析导出格式：优先 --export-format，兼容旧的 --format 别名
			// 注意：--format 与全局输出格式 flag 同名，需排除 json/table/raw/pretty 等全局值
			format, _ := cmd.Flags().GetString("export-format")
			if format == "" {
				if legacy, _ := cmd.Flags().GetString("format"); legacy != "" {
					// 排除全局输出格式值（当 --format json 来自 conftest 或用户误传时不应视为导出格式）
					globalFormats := map[string]bool{"json": true, "table": true, "raw": true, "pretty": true}
					if !globalFormats[strings.ToLower(legacy)] {
						format = legacy
					}
				}
			}
			if format == "" {
				format = "docx"
			}
			format = strings.ToLower(format)
			// 格式 → 文件扩展名映射（含别名）
			formatExtMap := map[string]string{
				"docx":     ".docx",
				"markdown": ".md",
				"md":       ".md",
				"pdf":      ".pdf",
			}
			fileExt, ok := formatExtMap[format]
			if !ok {
				return fmt.Errorf("unsupported --format %q, expected one of: docx, markdown (or md), pdf", format)
			}
			// 规范化为 MCP 接受的格式名（"md" → "markdown"）
			if format == "md" {
				format = "markdown"
			}

			submitArgs := map[string]any{
				"nodeId":       node,
				"exportFormat": format,
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "导出文档（提交+轮询+下载）")
				deps.Out.PrintKeyValue("文档", node)
				deps.Out.PrintKeyValue("输出", outputPath)
				deps.Out.PrintKeyValue("格式", format)
				return nil
			}

			ctx := context.Background()

			// ── Step 1: 提交导出任务 ──
			deps.Out.PrintInfo("[1/3] 提交导出任务...")
			submitText, err := callMCPToolReturnText(ctx, "submit_export_job", submitArgs)
			if err != nil {
				return fmt.Errorf("提交导出任务失败: %w", err)
			}

			var submitResult map[string]any
			if err := json.Unmarshal([]byte(submitText), &submitResult); err != nil {
				return fmt.Errorf("解析提交结果失败: %w", err)
			}
			jobID, _ := submitResult["jobId"].(string)
			if jobID == "" {
				deps.Out.PrintRaw(submitText)
				return fmt.Errorf("提交导出任务成功但未返回 jobId")
			}
			deps.Out.PrintInfo(fmt.Sprintf("    任务已提交，jobId: %s", jobID))

			// ── Step 2: 渐进式退避轮询 ──
			deps.Out.PrintInfo("[2/3] 等待导出完成...")
			downloadURL, err := pollDocExportJob(ctx, jobID)
			if err != nil {
				return err
			}

			// ── Step 3: 下载文件 ──
			fi, statErr := os.Stat(outputPath)
			if statErr == nil && fi.IsDir() {
				filename := inferFilename(downloadURL)
				if ext := filepath.Ext(filename); ext == "" {
					filename += fileExt
				} else if !strings.EqualFold(ext, fileExt) {
					// 推断到的扩展名与请求格式不一致时，统一使用请求格式的扩展名
					filename = strings.TrimSuffix(filename, ext) + fileExt
				}
				outputPath = filepath.Join(outputPath, filename)
			}

			deps.Out.PrintInfo(fmt.Sprintf("[3/3] 下载文件到 %s ...", outputPath))
			if err := httpGetFile(ctx, downloadURL, nil, outputPath); err != nil {
				return fmt.Errorf("文件下载失败 (jobId=%s): %w", jobID, err)
			}

			deps.Out.PrintInfo(fmt.Sprintf("导出完成: %s", outputPath))
			return nil
		},
	}
	exportCmd.Flags().String("node", "", "要导出的文档标识，支持文档 URL 或 dentryUuid (必填)")
	exportCmd.Flags().String("export-format", "docx", "导出格式：docx (默认) / markdown (或 md) / pdf")
	exportCmd.Flags().String("format", "", "--export-format 的别名 (向后兼容，与全局 --format 冲突时以 --export-format 为准)")
	_ = exportCmd.Flags().MarkHidden("format")
	exportCmd.Flags().String("output", "", "本地保存路径，文件路径或目录 (必填)")

	// export get: 手动查询已有任务状态（兜底用）
	exportGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询导出任务结果（手动兜底）",
		Long: `根据 jobId 查询文档导出任务的执行结果。
通常不需要手动调用，dws doc export 会自动完成轮询。
仅在导出命令超时或中断后，用于手动查询任务状态。

任务状态：
  PROCESSING  处理中
  SUCCESS     导出成功，返回 downloadUrl
  FAILED      导出失败`,
		Example: `  dws doc export get --job-id <JOB_ID>
  dws doc export get --task-id <TASK_ID>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Keep --job-id as the visible primary; --task-id is an add-only synonym.
			jobID, err := mustFlagOrFallback(cmd, "job-id", "task-id")
			if err != nil {
				return err
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "查询导出任务结果")
				deps.Out.PrintKeyValue("任务ID", jobID)
				return nil
			}

			ctx := context.Background()
			text, err := callMCPToolReturnText(ctx, "query_export_job", map[string]any{"jobId": jobID})
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
			normalizedStatus := strings.ToUpper(status)

			switch normalizedStatus {
			case "SUCCESS":
				deps.Out.PrintJSON(result)
				return nil
			case "PROCESSING":
				deps.Out.PrintJSON(result)
				return nil
			default:
				deps.Out.PrintJSON(result)
				if message != "" {
					return fmt.Errorf("导出任务失败 (status=%s): %s", status, message)
				}
				return fmt.Errorf("导出任务失败 (status=%s)", status)
			}
		},
	}
	DeclareLeafMetadata(exportGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "query_export_job",
				CanonicalPath:  "doc.query_export_job",
				CLIPath:        "doc export get",
				PrimaryCLIPath: "doc export get",
			},
			Description: "查询文档导出任务结果",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "doc", RPCName: "query_export_job"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询文档导出任务结果",
				UseWhen:      []string{"doc export 超时/中断后，用 jobId 查询导出任务状态与下载链接时"},
				AvoidWhen:    []string{"常规导出请直接 dws doc export（一体化提交+轮询+下载），不要先查 job"},
				Examples:     []string{"dws doc export get --job-id <JOB_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "job-id", Property: "jobId"},
			},
		},
	})
	exportGetCmd.Flags().String("job-id", "", "导出任务 ID (必填)")
	exportGetCmd.Flags().String("task-id", "", "--job-id 的别名")
	_ = exportGetCmd.Flags().MarkHidden("task-id")

	// --node 的隐藏别名（与 doc 下其他命令保持一致）
	exportCmd.Flags().String("url", "", "--node 的别名")
	exportCmd.Flags().String("id", "", "--node 的别名")
	exportCmd.Flags().String("node-id", "", "--node 的别名")
	exportCmd.Flags().String("doc-id", "", "--node 的别名")
	exportCmd.Flags().String("file-id", "", "--node 的别名 (跨产品兼容 drive)")
	_ = exportCmd.Flags().MarkHidden("url")
	_ = exportCmd.Flags().MarkHidden("id")
	_ = exportCmd.Flags().MarkHidden("node-id")
	_ = exportCmd.Flags().MarkHidden("doc-id")
	_ = exportCmd.Flags().MarkHidden("file-id")

	exportCmd.AddCommand(exportGetCmd)

	// ── import: 文件导入为在线文档（一体化：上传→转换→轮询）──────────────
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "导入本地文件为在线文档 (支持 docx / xlsx / md 等)",
		Long: `将本地文件导入为钉钉在线文档。

支持的文件格式 (按扩展名):
  docx, doc   → 文字文档
  xlsx, xls   → 电子表格
  md, txt     → 文字文档
  xmind, mark → 脑图
  其他格式（html/pdf/zip 等）→ 不做在线文档转换，自动改走文件上传链路，
  以原文件形式存入 --folder/--workspace 指定位置；如需在线文档请先转换
  为 md；上传到钉盘请用 dws drive upload

文件大小限制: 20MB

CLI 内部自动完成全部流程:
  1. 创建导入会话（获取 OSS 上传凭证）
  2. 上传文件到 OSS
  3. 确认导入（触发格式转换）
  4. 渐进式退避轮询等待完成（最多约 5 分钟）

如果轮询超时仍未完成，会输出 taskId 供后续手动查询:
  dws doc import get --task-id <taskId>`,
		Example: `  # 导入 Word 文档
  dws doc import --file ./report.docx

  # 导入到指定文件夹
  dws doc import --file ./notes.md --folder <FOLDER_ID>

  # 导入到知识库根目录
  dws doc import --file ./data.xlsx --workspace <WORKSPACE_ID>

  # 自定义导入后的文档名称
  dws doc import --file ./draft.md --name "项目周报"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportCommand(cmd, args, docImportFlowConfig())
		},
	}
	importCmd.Flags().String("file", "", "本地文件路径 (必填)")
	importCmd.Flags().String("folder", "", "目标文件夹 ID 或 URL (可选；folder/workspace 都不传时导入到默认根目录)")
	importCmd.Flags().String("workspace", "", "目标知识库 ID 或 URL (可选；folder/workspace 都不传时导入到默认根目录)")
	importCmd.Flags().StringP("name", "n", "", "导入后文档名称 (可选，默认取文件名)")
	importCmd.Flags().String("folder-id", "", "")
	_ = importCmd.Flags().MarkHidden("folder-id")
	importCmd.Flags().String("workspace-id", "", "")
	_ = importCmd.Flags().MarkHidden("workspace-id")

	importGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询导入任务结果（手动兜底）",
		Long: `根据 taskId 查询文档导入任务的执行结果。
通常不需要手动调用，dws doc import 会自动完成轮询。
仅在导入命令超时或中断后，用于手动查询任务状态。

任务状态:
  processing  转换中
  completed   导入成功，返回 documentUrl
  failed      导入失败`,
		Example: `  dws doc import get --task-id <TASK_ID>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImportGetCommand(cmd, docImportFlowConfig())
		},
	}
	DeclareLeafMetadata(importGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "import_get",
				CanonicalPath:  "doc.import_get",
				CLIPath:        "doc import get",
				PrimaryCLIPath: "doc import get",
			},
			Description: "根据 taskId 查询文档导入任务的执行结果",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据 taskId 查询文档导入任务的执行结果",
				UseWhen:      []string{"查询文档导入任务结果（已有 taskId，导入超时/中断后兜底）时"},
				AvoidWhen:    []string{"发起导入用 doc import（若入口可用）；不要用本命令代替导入"},
				Examples:     []string{"dws doc import get --task-id <TASK_ID> --format json"},
			},
		},
	})
	importGetCmd.Flags().String("task-id", "", "导入任务 ID (必填)")
	importCmd.AddCommand(importGetCmd)

	// ── doc version 子命令组 ──
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "文档历史版本管理",
		Long:  `管理钉钉在线文档（adoc）的历史版本：手动保存、查看版本列表、回滚到指定版本。`,
		RunE:  groupRunE,
	}

	versionSaveCmd := &cobra.Command{
		Use:     "save",
		Short:   "手动保存文档版本快照",
		Example: `  dws doc version save --node DOC_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("doc", "save_doc_version", map[string]any{
				"nodeId": nodeID,
			})
		},
	}
	DeclareLeafMetadata(versionSaveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "version_save",
				CanonicalPath:  "doc.version_save",
				CLIPath:        "doc version save",
				PrimaryCLIPath: "doc version save",
			},
			Description: "手动保存文档版本快照",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "手动保存文档版本快照",
				UseWhen:      []string{"手动保存当前文档版本快照时"},
				AvoidWhen:    []string{"回滚用 revert；只看历史用 list"},
				Examples:     []string{"dws doc version save --node <DOC_ID> --format json"},
			},
		},
	})
	versionSaveCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")

	versionListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "查看文档历史版本列表",
		Example: `  dws doc version list --node DOC_ID
  dws doc version list --node DOC_ID --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPToolOnServer("doc", "list_doc_versions", toolArgs)
		},
	}
	DeclareLeafMetadata(versionListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "version_list",
				CanonicalPath:  "doc.version_list",
				CLIPath:        "doc version list",
				PrimaryCLIPath: "doc version list",
			},
			Description: "查看文档历史版本列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查看文档历史版本列表",
				UseWhen:      []string{"查看文档历史版本列表以确认版本号时"},
				AvoidWhen:    []string{"回滚用 version revert（需确认）；保存快照用 version save"},
				Examples:     []string{"dws doc version list --node <DOC_ID> --format json"},
			},
		},
	})
	versionListCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	versionListCmd.Flags().Int("limit", 0, "返回版本数量上限")
	versionListCmd.Flags().String("cursor", "", "分页游标")

	versionRevertSafety := contract.SafetySpec{
		Effect:       "write",
		Risk:         "medium",
		Confirmation: "user_required",
		Idempotency:  "unknown",
	}
	versionRevertCmd := &cobra.Command{
		Use:     "revert",
		Short:   "回滚文档到指定版本",
		Example: `  dws doc version revert --node DOC_ID --version 3 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("version") {
				return fmt.Errorf("flag --version is required")
			}
			version, _ := cmd.Flags().GetInt("version")
			if !commandDryRun(cmd) {
				exists, err := docVersionExists(cmd.Context(), nodeID, version)
				if err != nil {
					return err
				}
				if !exists {
					return apperrors.NewValidation(
						fmt.Sprintf("文档版本 %d 不存在，已停止回滚", version),
						apperrors.WithReason("version_not_found"),
						apperrors.WithHint(fmt.Sprintf(
							"请先执行 dws doc version list --node %s --format json 获取可回滚版本",
							nodeID,
						)),
						apperrors.WithActions("查询可用文档版本", "选择存在的版本号后重新预览"),
					)
				}
			}
			return callMCPToolOnServer("doc", "revert_doc_version", map[string]any{
				"nodeId":  nodeID,
				"version": version,
			})
		},
	}
	DeclareLeafMetadata(versionRevertCmd, LeafSpec{
		Safety: versionRevertSafety,
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "version_revert",
				CanonicalPath:  "doc.version_revert",
				CLIPath:        "doc version revert",
				PrimaryCLIPath: "doc version revert",
			},
			Description: "将文档回滚到指定历史版本",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将文档回滚到指定历史版本",
				UseWhen:      []string{"用户明确要求将 adoc 回滚到指定历史版本（已从 version list 确认版本号）时"},
				AvoidWhen: []string{
					"只看版本列表用 version list；保存快照用 save",
					"版本号未确认或用户未同意回滚时不要执行",
				},
				Examples: []string{"dws doc version revert --node <DOC_ID> --version 3 --format json"},
			},
		},
	})
	versionRevertCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	versionRevertCmd.Flags().Int("version", 0, "目标版本号 (必填，从 list 获取)")

	// version 子命令 --node 隐藏别名
	for _, c := range []*cobra.Command{versionSaveCmd, versionListCmd, versionRevertCmd} {
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

	versionCmd.AddCommand(versionSaveCmd, versionListCmd, versionRevertCmd)

	// ── template 子命令组 ──────────────────────────────────────────────────────
	templateCmd := &cobra.Command{Use: "template", Short: "文档模板管理", RunE: groupRunE}

	templateListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取文档模板列表",
		Long:  `获取当前用户可用的文档模板列表，支持按来源筛选。`,
		Example: `  dws doc template list
  dws doc template list --source MY
  dws doc template list --source PUBLIC
  dws doc template list --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("source"); v != "" {
				toolArgs["templateSource"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPToolOnServer("doc", "list_doc_templates", toolArgs)
		},
	}
	DeclareLeafMetadata(templateListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "template_list",
				CanonicalPath:  "doc.template_list",
				CLIPath:        "doc template list",
				PrimaryCLIPath: "doc template list",
			},
			Description: "获取当前用户可用的文档模板列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前用户可用的文档模板列表",
				UseWhen:      []string{"列出当前用户可用的文档模板时"},
				AvoidWhen:    []string{"按关键词搜模板用 template search；套用模板用 template apply"},
				Examples: []string{
					"dws doc template list --format json",
					"dws doc template list --source MY --format json",
				},
			},
		},
	})
	templateListCmd.Flags().String("source", "", "模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY")
	templateListCmd.Flags().Int("limit", 0, "返回数量上限")
	templateListCmd.Flags().String("cursor", "", "分页游标")

	templateSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索文档模板",
		Long:  `根据关键词搜索文档模板。`,
		Example: `  dws doc template search --query "周报"
  dws doc template search --query "会议纪要" --limit 10
  dws doc template search --query "项目" --source PUBLIC`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, _ := cmd.Flags().GetString("query")
			if query == "" {
				query = flagOrFallback(cmd, "keyword", "name")
			}
			if query == "" {
				return fmt.Errorf("flag --query is required")
			}
			toolArgs := map[string]any{
				"searchName": query,
			}
			if v, _ := cmd.Flags().GetString("source"); v != "" {
				toolArgs["templateSource"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["maxResults"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "page-token", "next-token"); v != "" {
				toolArgs["nextCursor"] = v
			}
			return callMCPToolOnServer("doc", "search_doc_templates", toolArgs)
		},
	}
	DeclareLeafMetadata(templateSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "template_search",
				CanonicalPath:  "doc.template_search",
				CLIPath:        "doc template search",
				PrimaryCLIPath: "doc template search",
			},
			Description: "根据关键词搜索文档模板",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据关键词搜索文档模板",
				UseWhen:      []string{"按关键词搜索文档模板时"},
				AvoidWhen:    []string{"浏览全部模板用 template list；创建文档用 template apply"},
				Examples: []string{
					"dws doc template search --query \"周报\" --format json",
					"dws doc template search --query \"会议纪要\" --source PUBLIC --format json",
				},
			},
		},
	})
	templateSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	templateSearchCmd.Flags().String("keyword", "", "--query 的别名")
	_ = templateSearchCmd.Flags().MarkHidden("keyword")
	templateSearchCmd.Flags().String("name", "", "--query 的别名")
	_ = templateSearchCmd.Flags().MarkHidden("name")
	templateSearchCmd.Flags().String("source", "", "模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY")
	templateSearchCmd.Flags().Int("limit", 0, "返回数量上限")
	templateSearchCmd.Flags().String("cursor", "", "分页游标")

	templateApplyCmd := &cobra.Command{
		Use:   "apply",
		Short: "应用文档模板",
		Long:  `使用指定模板创建新文档。`,
		Example: `  dws doc template apply --template-id TPL_ID --name "我的周报"
  dws doc template apply --template-id TPL_ID --name "项目方案" --folder FOLDER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tplID := flagOrFallback(cmd, "template-id", "template", "tpl-id")
			if tplID == "" {
				return fmt.Errorf("flag --template-id is required")
			}
			toolArgs := map[string]any{
				"templateId": tplID,
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["name"] = v
			}
			if v := docFolderFlag(cmd); v != "" {
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPToolOnServer("doc", "apply_doc_template", toolArgs)
		},
	}
	DeclareLeafMetadata(templateApplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "template_apply",
				CanonicalPath:  "doc.template_apply",
				CLIPath:        "doc template apply",
				PrimaryCLIPath: "doc template apply",
			},
			Description: "使用指定模板创建新文档",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "使用指定模板创建新文档",
				UseWhen:      []string{"使用指定模板创建新文档时"},
				AvoidWhen:    []string{"先 list/search 拿到模板再 apply；普通空文档用 doc create"},
				Examples: []string{
					"dws doc template apply --template-id <TPL_ID> --name \"我的周报\" --format json",
					"dws doc template apply --template-id <TPL_ID> --name \"项目方案\" --folder <FOLDER_ID> --format json",
				},
			},
		},
	})
	templateApplyCmd.Flags().String("template-id", "", "模板 ID (必填)")
	templateApplyCmd.Flags().String("template", "", "--template-id 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("template")
	templateApplyCmd.Flags().String("tpl-id", "", "--template-id 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("tpl-id")
	templateApplyCmd.Flags().String("name", "", "新文档名称 (可选)")
	templateApplyCmd.Flags().String("folder", "", "目标文件夹 ID (可选)")
	templateApplyCmd.Flags().String("parent-id", "", "--folder 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("parent-id")
	templateApplyCmd.Flags().String("workspace", "", "知识库 ID (可选)")
	templateApplyCmd.Flags().String("workspace-id", "", "--workspace 的别名")
	_ = templateApplyCmd.Flags().MarkHidden("workspace-id")

	templateCmd.AddCommand(templateListCmd, templateSearchCmd, templateApplyCmd)

	// ── cross-product hidden aliases (auto-register for all leaf commands) ──
	// Note: listCmd already has manually registered Int-type aliases (--max, --limit)
	// and String aliases (--parent-id, --next-token) above; RegisterCrossProductAliases
	// will skip flags that already exist.
	for _, cmd := range []*cobra.Command{
		searchCmd, listCmd, createCmd, updateCmd, uploadCmd, downloadCmd,
		copyCmd, moveCmd, renameCmd, deleteCmd, exportCmd, importCmd,
	} {
		RegisterCrossProductAliases(cmd)
	}
	// sub-commands under block/comment/media/permission/file/folder/version/template
	for _, parent := range []*cobra.Command{blockCmd, commentCmd, mediaCmd, permissionCmd, fileCmd, folderCmd, versionCmd, templateCmd} {
		for _, child := range parent.Commands() {
			RegisterCrossProductAliases(child)
		}
	}

	// ── deprecated 标记：文件管理命令已迁移到 drive ──
	deprecatedDocToDrive := map[*cobra.Command]string{
		copyCmd:     "copy",
		moveCmd:     "move",
		renameCmd:   "rename",
		uploadCmd:   "upload",
		downloadCmd: "download",
		deleteCmd:   "delete",
	}
	for cmd, driveCmd := range deprecatedDocToDrive {
		wrapDocDeprecated(cmd, driveCmd)
		cmd.Hidden = true
	}
	// list → drive list --workspace / wiki node list
	wrapDocDeprecatedToTarget(listCmd, "drive list --workspace <workspaceId>' or 'dws wiki node list --workspace <workspaceId>")
	listCmd.Hidden = true
	// file create → wiki node create (空间管理层创建空文件节点)
	wrapDocDeprecatedToWiki(fileCreateCmd, "wiki node create --type <type>")
	// folder create → drive mkdir (钉盘) 或 wiki node create --type folder (知识库)
	wrapDocDeprecatedToTarget(folderCreateCmd, "drive mkdir' or 'dws wiki node create --workspace <workspaceId> --name \"名称\" --type folder")
	// permission 子命令
	wrapDocDeprecated(permissionAddCmd, "permission add")
	wrapDocDeprecated(permissionUpdateCmd, "permission update")
	wrapDocDeprecated(permissionListCmd, "permission list")
	wrapDocDeprecated(permissionRemoveCmd, "permission remove")
	// search → drive search / wiki node search
	wrapDocDeprecatedToTarget(searchCmd, "drive search' or 'dws wiki node search --workspace <id>")
	searchCmd.Hidden = true
	folderCmd.Hidden = true
	permissionCmd.Hidden = true

	root.AddCommand(searchCmd, listCmd, infoCmd, readCmd, createCmd, updateCmd, uploadCmd, downloadCmd, copyCmd, moveCmd, renameCmd, deleteCmd, fileCmd, folderCmd, blockCmd, commentCmd, mediaCmd, permissionCmd, exportCmd, importCmd, versionCmd, templateCmd, newDocStyleCommand(), newDocWhiteboardCommand())

	return root
}

// printDocDeprecationWarning emits a deprecation warning when shared deps are
// initialized. Schema declaration-only roots skip InitDeps; homology and other
// Execute probes must remain nil-safe on that path.
func printDocDeprecationWarning(msg string) {
	if deps == nil || deps.Out == nil {
		return
	}
	deps.Out.PrintWarning(msg)
}

// wrapDocDeprecated wraps a doc command's RunE to print a deprecation warning
// directing users to the corresponding drive command. The original command
// continues to function normally during the transition period.
func wrapDocDeprecated(cmd *cobra.Command, driveSubCmd string) {
	originalRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if strings.HasPrefix(c.CommandPath(), "dws doc ") {
			printDocDeprecationWarning(fmt.Sprintf(
				"⚠️  'dws doc %s' is deprecated, use 'dws drive %s' instead.",
				c.CommandPath()[8:], // strip "dws doc " prefix
				driveSubCmd,
			))
		}
		return originalRunE(c, args)
	}
}

// wrapDocDeprecatedToWiki wraps a doc command's RunE to print a deprecation warning
// directing users to the corresponding wiki command.
func wrapDocDeprecatedToWiki(cmd *cobra.Command, wikiSubCmd string) {
	originalRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if strings.HasPrefix(c.CommandPath(), "dws doc ") {
			printDocDeprecationWarning(fmt.Sprintf(
				"⚠️  'dws doc %s' is deprecated, use 'dws %s' instead.",
				c.CommandPath()[8:],
				wikiSubCmd,
			))
		}
		return originalRunE(c, args)
	}
}

// wrapDocDeprecatedToTarget wraps a doc command's RunE to print a deprecation warning
// directing users to a specified target command path.
func wrapDocDeprecatedToTarget(cmd *cobra.Command, targetCmd string) {
	originalRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if strings.HasPrefix(c.CommandPath(), "dws doc ") {
			printDocDeprecationWarning(fmt.Sprintf(
				"⚠️  'dws doc %s' is deprecated, use 'dws %s' instead.",
				c.CommandPath()[8:],
				targetCmd,
			))
		}
		return originalRunE(c, args)
	}
}

// resolveContentFromFlags 从 --content-file / --content-path / --content / --markdown 获取文档内容。
// 优先级：--content-file/--content-path > --content > --markdown（已弃用别名，向后兼容）。
func runDocReadJsonML(_ *cobra.Command, nodeID string, outputPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resultText, err := callMCPToolReturnText(ctx, "get_document_content", map[string]any{
		"nodeId": nodeID,
		"format": "jsonml",
	})
	if err != nil {
		return err
	}

	// MCP 响应: {"nodeId":"...", "jsonml":"{...}", "revision":N, "title":"...", ...}
	var mcpResp map[string]any
	if err := json.Unmarshal([]byte(resultText), &mcpResp); err != nil {
		return fmt.Errorf("failed to parse MCP response: %w", err)
	}
	jsonmlStr, _ := mcpResp["jsonml"].(string)
	if jsonmlStr == "" {
		return fmt.Errorf("MCP response does not contain jsonml field")
	}

	// 组装输出：{"revision": N, "jsonml": {...}}
	outputMap := map[string]any{
		"jsonml": json.RawMessage(jsonmlStr),
	}
	if v := mcpResp["revision"]; v != nil {
		switch ver := v.(type) {
		case float64:
			outputMap["revision"] = int(ver)
		case string:
			if ver != "" {
				var n int
				if _, err := fmt.Sscanf(ver, "%d", &n); err == nil {
					outputMap["revision"] = n
				}
			}
		}
	}
	outputBytes, err := json.MarshalIndent(outputMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}
	output := string(outputBytes)

	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write output file %s: %w", outputPath, err)
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] JSONML 已写入 %s", outputPath))
		return nil
	}

	deps.Out.PrintRaw(output)
	return nil
}

// runDocReadScope calls get_document_content with JSONML filtering parameters
// and preserves the returned read-only fragment container.
func runDocReadScope(nodeID, scope, tags string, maxDepth int, maxDepthSet bool, startBlockID, endBlockID, outputPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := map[string]any{"nodeId": nodeID, "format": "jsonml"}
	if scope != "" {
		args["scope"] = scope
	}
	if tags != "" {
		args["tags"] = tags
	}
	if maxDepthSet {
		args["maxDepth"] = maxDepth
	}
	if startBlockID != "" {
		args["startBlockId"] = startBlockID
	}
	if endBlockID != "" && scope == "range" {
		args["endBlockId"] = endBlockID
	}

	resultText, err := callMCPToolReturnTextOnServer(ctx, "doc", "get_document_content", args)
	if err != nil {
		return err
	}

	var mcpResp map[string]any
	if err := json.Unmarshal([]byte(resultText), &mcpResp); err != nil {
		return fmt.Errorf("failed to parse MCP response: %w", err)
	}
	fragmentJSON, _ := mcpResp["jsonml"].(string)
	if fragmentJSON == "" {
		if deps.Caller.Format() == "json" {
			return deps.Out.PrintJSON(map[string]any{
				"matched": false,
				"jsonml":  nil,
			})
		}
		deps.Out.PrintInfo("[INFO] 未匹配到节点")
		return nil
	}
	if !json.Valid([]byte(fragmentJSON)) {
		return fmt.Errorf("MCP response contained invalid JSONML fragment")
	}

	output := fragmentJSON
	if pretty, prettyErr := json.MarshalIndent(json.RawMessage(fragmentJSON), "", "  "); prettyErr == nil {
		output = string(pretty)
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(output), 0o644); err != nil {
			return fmt.Errorf("failed to write output file %s: %w", outputPath, err)
		}
		if deps.Caller.Format() == "json" {
			return deps.Out.PrintJSON(map[string]any{
				"success": true,
				"output":  outputPath,
			})
		}
		deps.Out.PrintInfo(fmt.Sprintf("[INFO] JSONML fragment 已写入 %s", outputPath))
		return nil
	}

	deps.Out.PrintRaw(output)
	return nil
}

// resolveContentFromFlags 从 --content-file / --content / --markdown 获取文档内容。
// 优先级：--content-file > --content > --markdown（已弃用别名，向后兼容）。
//
//	--content-file path → 从文件读取（UTF-8）
//	--content -         → 从 stdin 读取
//	--content "x"       → 字面值
//	--markdown "x"      → 已弃用，等同于 --content
func resolveContentFromFlags(cmd *cobra.Command) (string, error) {
	filePath := flagOrFallback(cmd, "content-file", "content-path")
	if filePath != "" {
		b, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("--content-file: 读取文件 %q 失败: %w", filePath, err)
		}
		return string(b), nil
	}

	raw, _ := cmd.Flags().GetString("content")
	if raw == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("--content: 读取 stdin 失败: %w", err)
		}
		return string(data), nil
	}

	// --markdown 是 --content 的已弃用别名，仅在 --content 和 --content-file 都未提供时 fallback
	if raw == "" {
		if md, _ := cmd.Flags().GetString("markdown"); md != "" {
			if md == "-" {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return "", fmt.Errorf("--markdown: 读取 stdin 失败: %w", err)
				}
				return string(data), nil
			}
			return unescapeLiteralContent(md), nil
		}
	}

	return unescapeLiteralContent(raw), nil
}

// unescapeLiteralContent 将命令行字面量中的转义序列转换为对应字符。
// 使用 strconv.Unquote 处理所有标准转义: \n \t \r \\ \uXXXX 等。
// 仅用于 --content / --markdown 字面量输入场景；文件和 stdin 输入已含真实换行，无需处理。
// 当发生转义时会打印 warning 提示用户。
func unescapeLiteralContent(s string) string {
	if s == "" || !strings.Contains(s, "\\") {
		return s
	}
	// strconv.Unquote 要求输入用双引号包裹，且内部双引号需转义
	quoted := `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		// 解析失败则原样返回，不破坏用户输入
		return s
	}
	if unquoted != s {
		deps.Out.PrintInfo("[WARN] 检测到转义序列 (\\n, \\t 等)，已自动转换为对应字符。如需保留字面反斜杠，请使用 \\\\ 或改用 --content-file")
	}
	return unquoted
}

// pollExportJob polls query_export_job with progressive back-off until the
// task completes or the retry limit is reached.
//
// Back-off schedule (aligned with lippi-doc-solution server-side guidance):
//
//	polls  1-5:  2s interval
//	polls  6-10: 5s interval
//	polls 11-20: 10s interval
//	polls 21-30: 15s interval
//	max 30 polls (~5 minutes total)
func pollDocExportJob(ctx context.Context, jobID string) (downloadURL string, err error) {
	const maxPolls = 30

	pollInterval := func(attempt int) time.Duration {
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
	}

	for attempt := 1; attempt <= maxPolls; attempt++ {
		interval := pollInterval(attempt)
		deps.Out.PrintInfo(fmt.Sprintf("    第 %d/%d 次查询，等待 %v ...", attempt, maxPolls, interval))

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("导出轮询被取消 (jobId=%s): %w", jobID, ctx.Err())
		case <-helperAfter(interval):
		}

		text, queryErr := callMCPToolReturnText(ctx, "query_export_job", map[string]any{"jobId": jobID})
		if queryErr != nil {
			return "", fmt.Errorf("查询导出任务失败 (jobId=%s): %w", jobID, queryErr)
		}

		var result map[string]any
		if parseErr := json.Unmarshal([]byte(text), &result); parseErr != nil {
			return "", fmt.Errorf("解析查询结果失败 (jobId=%s): %w", jobID, parseErr)
		}

		status, _ := result["status"].(string)
		message, _ := result["message"].(string)
		normalizedStatus := strings.ToUpper(status)

		switch normalizedStatus {
		case "SUCCESS":
			url, _ := result["downloadUrl"].(string)
			if url == "" {
				return "", fmt.Errorf("导出成功但 downloadUrl 为空 (jobId=%s)", jobID)
			}
			return url, nil
		case "PROCESSING":
			continue
		default:
			if message != "" {
				return "", fmt.Errorf("导出任务失败 (jobId=%s, status=%s): %s", jobID, status, message)
			}
			return "", fmt.Errorf("导出任务失败 (jobId=%s, status=%s)", jobID, status)
		}
	}

	return "", fmt.Errorf("导出任务超时：已轮询 %d 次仍在处理中 (jobId=%s)，请稍后使用 dws doc export get --job-id %s 手动查询", maxPolls, jobID, jobID)
}

// stripDuplicateTitle removes the leading H1 heading from markdown content
// when it matches the document name (set via --name). This prevents the title
// from appearing twice: once as document metadata and once in the body.
func stripDuplicateTitle(markdown, name string) string {
	trimmed := strings.TrimLeft(markdown, " \t\n\r")
	if !strings.HasPrefix(trimmed, "# ") {
		return markdown
	}
	newlineIdx := strings.Index(trimmed, "\n")
	var headingRaw string
	if newlineIdx < 0 {
		headingRaw = trimmed[2:]
	} else {
		headingRaw = trimmed[2:newlineIdx]
	}

	if normalizeHeadingText(headingRaw) != normalizeHeadingText(name) {
		return markdown
	}

	if newlineIdx < 0 {
		return ""
	}
	rest := trimmed[newlineIdx+1:]
	rest = strings.TrimLeft(rest, "\n")
	return rest
}

// normalizeHeadingText strips trailing ATX hashes, inline markdown formatting
// markers, then returns a lowercased, trimmed string for comparison.
func normalizeHeadingText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.LastIndexByte(s, ' '); i >= 0 {
		suffix := s[i+1:]
		if len(suffix) > 0 && strings.Trim(suffix, "#") == "" {
			s = strings.TrimSpace(s[:i])
		}
	}
	for _, m := range []string{"**", "__", "~~", "*", "_", "`"} {
		s = strings.ReplaceAll(s, m, "")
	}
	return strings.TrimSpace(strings.ToLower(s))
}

// parseCommentMentionIds splits a comma-separated string of user IDs into a slice.
func parseCommentMentionIds(raw string) []string {
	parts := strings.Split(raw, ",")
	userIds := make([]string, 0, len(parts))
	for _, p := range parts {
		uid := strings.TrimSpace(p)
		if uid != "" {
			userIds = append(userIds, uid)
		}
	}
	return userIds
}

func addCommentGroupMentionFlag(cmd *cobra.Command) {
	cmd.Flags().StringSlice(
		"mentioned-open-conversation-id",
		nil,
		"被 @ 的群 openConversationId，可重复指定或逗号分隔",
	)
}

// commentGroupMentionIDs validates, trims and stably de-duplicates group IDs.
// An explicitly supplied blank value is rejected so a requested mention is
// never silently downgraded to plain comment text.
func commentGroupMentionIDs(cmd *cobra.Command) ([]string, error) {
	raw, err := cmd.Flags().GetStringSlice("mentioned-open-conversation-id")
	if err != nil {
		return nil, err
	}
	if !cmd.Flags().Changed("mentioned-open-conversation-id") {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(raw))
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" {
			return nil, fmt.Errorf("--mentioned-open-conversation-id must not be empty or whitespace")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("--mentioned-open-conversation-id must include at least one non-empty openConversationId")
	}
	return ids, nil
}

func appendCommentGroupMentions(cmd *cobra.Command, args map[string]any) error {
	ids, err := commentGroupMentionIDs(cmd)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		args["mentionedOpenConversationIds"] = ids
	}
	return nil
}

// normalizePermissionRole canonicalises the --role flag to UPPERCASE so users
// can pass either "reader" or "READER". Trims whitespace as well.
// Empty input returns "" so the caller can validate as needed.
func normalizePermissionRole(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

// parseRoleList splits a comma-separated role list and uppercases each item.
// Used by --filter-role for list_permission / list_member.
func parseRoleList(raw string) []string {
	parts := strings.Split(raw, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		role := normalizePermissionRole(p)
		if role != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

// collectUserIDs reads the comma-separated --user flag and returns a flat
// userIds slice, ready to embed in the MCP tool args (add/update_permission
// and add/update_member all share this shape).
//
// MCP tools currently only accept the USER member type — ORG-level grants
// are blocked at the MCP gateway, so the dws layer does not need to filter
// or wrap members itself.
func collectUserIDs(cmd *cobra.Command) ([]string, error) {
	userRaw := flagOrFallback(cmd, "users", "user")
	userIds := parseCommentMentionIds(userRaw)
	if len(userIds) == 0 {
		return nil, fmt.Errorf("--users is required (at least one userId)")
	}
	return userIds, nil
}
