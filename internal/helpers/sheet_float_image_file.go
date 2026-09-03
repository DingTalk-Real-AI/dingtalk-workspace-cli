package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

const (
	floatImageCreateInputError  = "--file 与 --src 必须且只能提供一个"
	floatImageUpdateInputError  = "--file 与 --src 不能同时提供"
	floatImageUpdateFieldsError = "--file、--src、--range、--width、--height、--offset-x、--offset-y 至少必须提供一个"
	floatImagePlannedResource   = "<resourceUrl-after-upload>"
	floatImageUploadTimeout     = 5 * time.Minute
)

type floatImageLocalFile struct {
	file     *os.File
	path     string
	name     string
	mimeType string
	size     int64
}

type floatImageUploadInfo struct {
	uploadURL   string
	resourceID  string
	resourceURL string
}

func validateFloatImageCreateInput(cmd *cobra.Command) (string, string, error) {
	filePath, _ := cmd.Flags().GetString("file")
	src, _ := cmd.Flags().GetString("src")
	if cmd.Flags().Changed("file") && strings.TrimSpace(filePath) == "" {
		return "", "", fmt.Errorf("--file 不能为空")
	}
	hasFile := strings.TrimSpace(filePath) != ""
	hasSrc := strings.TrimSpace(src) != ""
	if hasFile == hasSrc {
		return "", "", fmt.Errorf("%s", floatImageCreateInputError)
	}
	return filePath, src, nil
}

func validateFloatImageUpdateInput(cmd *cobra.Command) (string, string, error) {
	filePath, _ := cmd.Flags().GetString("file")
	src, _ := cmd.Flags().GetString("src")
	if cmd.Flags().Changed("file") && cmd.Flags().Changed("src") {
		return "", "", fmt.Errorf("%s", floatImageUpdateInputError)
	}
	if cmd.Flags().Changed("file") && strings.TrimSpace(filePath) == "" {
		return "", "", fmt.Errorf("--file 不能为空")
	}
	return filePath, src, nil
}

func runFloatImageFileMode(cmd *cobra.Command, toolName, filePath string, toolArgs map[string]any) error {
	local, err := openFloatImageLocalFile(filePath)
	if err != nil {
		return err
	}
	defer local.file.Close()

	if deps.Caller.DryRun() {
		return printFloatImageFileDryRun(toolName, toolArgs, local)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	jsonMode := strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json")
	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("[1/3] 获取浮动图片上传凭证 (%s, %d bytes)...", local.name, local.size))
	}
	uploadInfo, err := resolveFloatImageUploadInfo(ctx, toolArgs["nodeId"], local)
	if err != nil {
		return err
	}
	if !jsonMode {
		deps.Out.PrintKeyValue("resourceId", uploadInfo.resourceID)
		deps.Out.PrintKeyValue("resourceUrl", uploadInfo.resourceURL)
		deps.Out.PrintInfo("[2/3] 上传本地图片...")
	}
	client := newFloatImageUploadClient()
	if err := putFloatImageFileWithClient(ctx, client, uploadInfo.uploadURL, local); err != nil {
		return err
	}

	if !jsonMode {
		deps.Out.PrintInfo("[3/3] 提交浮动图片变更...")
	}
	toolArgs["src"] = uploadInfo.resourceURL
	if err := callMCPToolContext(ctx, toolName, toolArgs); err != nil {
		return fmt.Errorf("图片已上传（resourceId=%s，resourceUrl=%s），但浮动图片操作失败；已有重试可能造成重复创建或更新，请先用 list/get/UI 核验后再重试: %w", uploadInfo.resourceID, uploadInfo.resourceURL, err)
	}
	return nil
}

func openFloatImageLocalFile(path string) (*floatImageLocalFile, error) {
	return openFloatImageLocalFileWith(path, os.Stat, os.Open)
}

func openFloatImageLocalFileWith(path string, stat func(string) (os.FileInfo, error), open func(string) (*os.File, error)) (*floatImageLocalFile, error) {
	info, err := stat(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取 --file %q: %w", path, err)
	}
	if err := validateFloatImageFileInfo(info); err != nil {
		return nil, err
	}
	file, err := open(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 --file %q: %w", path, err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("无法读取已打开的 --file %q: %w", path, err)
	}
	if err := validateFloatImageFileInfo(openedInfo); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &floatImageLocalFile{
		file:     file,
		path:     path,
		name:     filepath.Base(path),
		mimeType: inferMimeType(filepath.Base(path)),
		size:     openedInfo.Size(),
	}, nil
}

func validateFloatImageFileInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("--file 必须是普通文件")
	}
	if info.Size() <= 0 {
		return fmt.Errorf("--file 不能为空文件")
	}
	return nil
}

func printFloatImageFileDryRun(toolName string, toolArgs map[string]any, local *floatImageLocalFile) error {
	arguments := make(map[string]any, len(toolArgs)+1)
	for key, value := range toolArgs {
		arguments[key] = value
	}
	arguments["src"] = floatImagePlannedResource
	stages := []string{"validate_local_file", "get_upload_credentials", "upload_file", toolName}
	if strings.EqualFold(strings.TrimSpace(deps.Caller.Format()), "json") {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run":   true,
			"executed":  false,
			"tool":      toolName,
			"arguments": arguments,
			"local_file": map[string]any{
				"path": local.path, "name": local.name, "mime_type": local.mimeType, "size": local.size,
			},
			"stages": stages,
		})
	}
	deps.Out.PrintKeyValue("操作", "上传本地图片并提交浮动图片变更")
	deps.Out.PrintKeyValue("文件", local.path)
	deps.Out.PrintKeyValue("名称", local.name)
	deps.Out.PrintKeyValue("类型", local.mimeType)
	deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", local.size))
	deps.Out.PrintKeyValue("Tool", toolName)
	deps.Out.PrintKeyValue("阶段", strings.Join(stages, " -> "))
	return nil
}

func resolveFloatImageUploadInfo(ctx context.Context, nodeID any, local *floatImageLocalFile) (floatImageUploadInfo, error) {
	result, err := deps.Caller.CallTool(ctx, "doc", "get_doc_attachment_upload_info", map[string]any{
		"nodeId":   nodeID,
		"fileName": local.name,
		"fileSize": float64(local.size),
		"mimeType": local.mimeType,
	})
	if err != nil {
		return floatImageUploadInfo{}, WrapError(err)
	}
	return parseFloatImageUploadInfo(result)
}

func parseFloatImageUploadInfo(result *edition.ToolResult) (floatImageUploadInfo, error) {
	if result == nil {
		return floatImageUploadInfo{}, invalidFloatImageCredentialError()
	}
	text := ""
	for _, content := range result.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			text = content.Text
			break
		}
	}
	if text == "" {
		return floatImageUploadInfo{}, invalidFloatImageCredentialError()
	}
	type credential struct {
		UploadURL   string `json:"uploadUrl"`
		ResourceID  string `json:"resourceId"`
		ResourceURL string `json:"resourceUrl"`
	}
	var envelope struct {
		credential
		Result *credential `json:"result"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return floatImageUploadInfo{}, invalidFloatImageCredentialError()
	}
	parsed := envelope.credential
	if envelope.Result != nil {
		parsed = *envelope.Result
	}
	parsed.UploadURL = strings.TrimSpace(parsed.UploadURL)
	parsed.ResourceID = strings.TrimSpace(parsed.ResourceID)
	parsed.ResourceURL = strings.TrimSpace(parsed.ResourceURL)
	if parsed.UploadURL == "" || !validFloatImageResourceID(parsed.ResourceID) || !validFloatImageResourceURL(parsed.ResourceURL) {
		return floatImageUploadInfo{}, invalidFloatImageCredentialError()
	}
	if err := validateFloatImageUploadURL(parsed.UploadURL); err != nil {
		return floatImageUploadInfo{}, err
	}
	return floatImageUploadInfo{uploadURL: parsed.UploadURL, resourceID: parsed.ResourceID, resourceURL: parsed.ResourceURL}, nil
}

func invalidFloatImageCredentialError() error {
	return &CLIError{
		Code:       CodeMCPToolError,
		Message:    "获取浮动图片上传凭证失败：响应格式无效",
		Suggestion: "请重试；若持续失败，请检查 doc/get_doc_attachment_upload_info 服务",
		Operation:  "doc/get_doc_attachment_upload_info",
	}
}

func validFloatImageResourceID(value string) bool {
	return value != "" && len(value) <= 4096 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validFloatImageResourceURL(value string) bool {
	if value == "" || len(value) > 8192 || strings.IndexFunc(value, unicode.IsControl) >= 0 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

// Keep this policy aligned with internal/transport.Client.isEndpointTrusted.
// It stays Sheet-private in V1 so local-file float-image support does not
// refactor the shared transport boundary selected by unrelated products.
func validateFloatImageUploadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return invalidFloatImageUploadURLError()
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	if !strings.EqualFold(parsed.Scheme, "http") || os.Getenv("DWS_ALLOW_HTTP_ENDPOINTS") != "1" || !isFloatImageLoopbackHost(parsed.Hostname()) {
		return invalidFloatImageUploadURLError()
	}
	return nil
}

func isFloatImageLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func invalidFloatImageUploadURLError() error {
	return &CLIError{
		Code:       CodeInvalidParam,
		Message:    "浮动图片上传地址无效或不受信任",
		Suggestion: "上传地址必须是 HTTPS；本地开发仅允许显式开启的 loopback HTTP",
		Operation:  "sheet/float-image-upload",
	}
}

func newFloatImageUploadClient() *http.Client {
	return &http.Client{
		Timeout: floatImageUploadTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// This intentionally does not reuse doc.defaultHTTPPutFile or localio.PutFile:
// both have incompatible retry/status/host behavior, and the Doc helper can
// copy a signed URL or response body into errors. Keep this fixed-message PUT
// until a shared secure-upload contract can replace all upload implementations.
func putFloatImageFileWithClient(ctx context.Context, client *http.Client, rawURL string, local *floatImageLocalFile) error {
	if client == nil {
		return fmt.Errorf("浮动图片文件上传失败：HTTP 客户端不可用")
	}
	if _, err := local.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("浮动图片文件上传失败：无法读取本地文件")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, local.file)
	if err != nil {
		return fmt.Errorf("浮动图片文件上传失败：无法创建请求")
	}
	request.ContentLength = local.size
	request.Header.Set("Content-Type", local.mimeType)
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("浮动图片文件上传已取消: %w", ctxErr)
		}
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return fmt.Errorf("浮动图片文件上传失败：已超过传输时限")
		}
		return fmt.Errorf("浮动图片文件上传失败：网络请求失败")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("浮动图片文件上传失败：HTTP %d", response.StatusCode)
	}
	return nil
}
