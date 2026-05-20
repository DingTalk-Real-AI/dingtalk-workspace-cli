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

// driveHandler exposes the one drive subcommand that the service-discovery
// envelope cannot express on its own:
//
//   - upload — three-step composite: drive.get_upload_info → HTTP PUT to OSS →
//     drive.commit_upload. The envelope PipelineStep schema currently supports
//     type:"call" (MCP tool invocation) and type:"download" (HTTP GET sink),
//     but has no type:"upload" for streaming a local file to an OSS-signed
//     PUT URL with per-URL headers. Until the envelope schema grows that
//     capability, this helper is the canonical client-side glue.
//
// Other drive-vs-wukong gaps (list-spaces and delete) are covered purely via
// envelope toolOverrides — list_spaces as a plain alias map, delete_document
// via serverOverride to route to the doc MCP server. No Go code needed for
// those two; see envelope/pre-discovery.pre.json drive entry.
//
// The dynamic envelope still owns the six base commands (list / info /
// download / mkdir / upload-info / commit); pickCommands.MergeHardcodedLeaves
// guarantees dynamic leaves win on collision so this helper only fills the
// upload gap.
type driveHandler struct{}

func (driveHandler) Name() string {
	return "drive"
}

func (driveHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "drive",
		Short:             "钉盘扩展命令（合并到 dws drive 命令树）",
		Long:              "钉盘扩展子命令: upload 本地文件一键上传（三步合成）。其余命令均由服务发现 envelope 提供。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(newDriveUploadCommand(runner))
	return root
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
	cmd.Flags().String("space-id", "", "目标空间 ID，不传则使用「我的文件」")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型，不传则自动推断")
	cmd.Flags().String("folder", "", "父节点 ID (dentryUuid)，不传则上传到空间根目录")
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

	fileName, _ := cmd.Flags().GetString("file-name")
	if strings.TrimSpace(fileName) == "" {
		fileName = filepath.Base(absPath)
	}

	spaceID, _ := cmd.Flags().GetString("space-id")
	mimeType, _ := cmd.Flags().GetString("mime-type")
	if strings.TrimSpace(mimeType) == "" {
		mimeType = detectMIME(fileName)
	}
	parentID, _ := cmd.Flags().GetString("folder")
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
	if err := httpPutDriveFile(cmd.Context(), resourceURL, ossHeaders, absPath, fileSize, mimeType); err != nil {
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

func httpPutDriveFile(ctx context.Context, resourceURL string, headers map[string]string, filePath string, fileSize int64, fallbackMIME string) error {
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
	hasContentType := false
	for k, v := range headers {
		req.Header.Set(k, v)
		if strings.EqualFold(k, "Content-Type") {
			hasContentType = true
		}
	}
	if !hasContentType && fallbackMIME != "" {
		req.Header.Set("Content-Type", fallbackMIME)
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
