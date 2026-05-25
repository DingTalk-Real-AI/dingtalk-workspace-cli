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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
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
	media.AddCommand(newDocMediaInsertCommand(runner))

	root.AddCommand(media)
	return root
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
	cmd.Flags().String("node", "", i18n.T("目标文档的 nodeId 或 URL (必填)"))
	cmd.Flags().String("file", "", i18n.T("本地文件路径 (必填)"))
	cmd.Flags().String("name", "", i18n.T("附件显示名称（默认使用文件名）"))
	cmd.Flags().String("mime-type", "", i18n.T("文件 MIME 类型（默认根据扩展名推断）"))
	cmd.Flags().Int("index", 0, i18n.T("插入位置索引"))
	cmd.Flags().String("where", "", i18n.T("相对位置: before / after（配合 --ref-block）"))
	cmd.Flags().String("ref-block", "", i18n.T("参考块 ID（配合 --where）"))
	return cmd
}

const docMaxInlineImageSize = 20 * 1024 * 1024 // 20MB

func runDocMediaInsert(cmd *cobra.Command, runner executor.Runner) error {
	nodeID, _ := cmd.Flags().GetString("node")
	filePath, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(nodeID) == "" {
		return apperrors.NewValidation("--node is required")
	}
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
