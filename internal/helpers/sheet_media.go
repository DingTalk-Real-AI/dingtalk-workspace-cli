package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func runSheetMediaUpload(cmd *cobra.Command, _ []string) error {
	nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id")
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
		deps.Out.PrintKeyValue("操作", "上传附件到表格")
		deps.Out.PrintKeyValue("表格", nodeID)
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", fileName)
		deps.Out.PrintKeyValue("类型", mimeType)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		return nil
	}

	ctx := context.Background()

	// json 模式下进度提示会污染 stdout（PrintInfo/PrintKeyValue 都写 stdout），
	// 使得 agent 无法按 JSON 解析。故 json 模式抑制进度、末尾统一输出结果 JSON。
	jsonMode := deps.Caller.Format() == "json"

	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("[1/2] 获取附件上传凭证 (%s, %d bytes)...", fileName, fileSize))
	}

	result, err := deps.Caller.CallTool(ctx, "doc", "get_doc_attachment_upload_info", map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"mimeType": mimeType,
	})
	if err != nil {
		return WrapError(err)
	}

	var credText string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			credText = c.Text
			break
		}
	}

	uploadURL, resourceID, _, err := parseAttachmentUploadInfo(credText)
	if err != nil {
		return err
	}

	var resourceURL string
	var credData map[string]any
	if json.Unmarshal([]byte(credText), &credData) == nil {
		if result, ok := credData["result"].(map[string]any); ok {
			credData = result
		}
		resourceURL, _ = credData["resourceUrl"].(string)
	}

	if !jsonMode {
		deps.Out.PrintKeyValue("resourceId", resourceID)
		deps.Out.PrintKeyValue("resourceUrl", resourceURL)
		deps.Out.PrintInfo("[2/2] 上传文件到 OSS...")
	}

	ossHeaders := map[string]string{
		"Content-Type": mimeType,
	}
	if err := httpPutFile(ctx, uploadURL, ossHeaders, filePath, fileSize); err != nil {
		return err
	}

	if jsonMode {
		return deps.Out.PrintJSON(map[string]any{
			"success":     true,
			"resourceId":  resourceID,
			"resourceUrl": resourceURL,
			"fileName":    fileName,
			"fileSize":    fileSize,
		})
	}
	deps.Out.PrintInfo(fmt.Sprintf("附件已上传: %s (resourceId=%s)", fileName, resourceID))
	return nil
}

func runSheetWriteImage(cmd *cobra.Command, _ []string) error {
	nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id")
	if err != nil {
		return err
	}

	sheetID := mustGetFlag(cmd, "sheet-id")
	if sheetID == "" {
		return fmt.Errorf("flag --sheet-id is required")
	}

	rangeAddress := mustGetFlag(cmd, "range")
	if rangeAddress == "" {
		return fmt.Errorf("flag --range is required")
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
		deps.Out.PrintKeyValue("操作", "上传图片并写入表格")
		deps.Out.PrintKeyValue("表格", nodeID)
		deps.Out.PrintKeyValue("工作表", sheetID)
		deps.Out.PrintKeyValue("单元格", rangeAddress)
		deps.Out.PrintKeyValue("文件", filePath)
		deps.Out.PrintKeyValue("名称", fileName)
		deps.Out.PrintKeyValue("类型", mimeType)
		deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", fileSize))
		return nil
	}

	ctx := context.Background()

	// json 模式下进度提示会污染 stdout（PrintInfo/PrintKeyValue 都写 stdout），
	// 使 write_image 的 JSON 响应无法被单独解析。故 json 模式抑制进度。
	jsonMode := deps.Caller.Format() == "json"

	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("[1/3] 获取附件上传凭证 (%s, %d bytes)...", fileName, fileSize))
	}

	result, err := deps.Caller.CallTool(ctx, "doc", "get_doc_attachment_upload_info", map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"mimeType": mimeType,
	})
	if err != nil {
		return WrapError(err)
	}

	var credText string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			credText = c.Text
			break
		}
	}

	uploadURL, resourceID, _, err := parseAttachmentUploadInfo(credText)
	if err != nil {
		return err
	}

	var resourceURL string
	var credData map[string]any
	if json.Unmarshal([]byte(credText), &credData) == nil {
		if r, ok := credData["result"].(map[string]any); ok {
			credData = r
		}
		resourceURL, _ = credData["resourceUrl"].(string)
	}

	if !jsonMode {
		deps.Out.PrintKeyValue("resourceId", resourceID)
		deps.Out.PrintKeyValue("resourceUrl", resourceURL)
		deps.Out.PrintInfo("[2/3] 上传图片到 OSS...")
	}

	ossHeaders := map[string]string{
		"Content-Type": mimeType,
	}
	if err := httpPutFile(ctx, uploadURL, ossHeaders, filePath, fileSize); err != nil {
		return err
	}

	if !jsonMode {
		deps.Out.PrintInfo("[3/3] 写入图片到表格单元格...")
	}

	writeArgs := map[string]any{
		"nodeId":       nodeID,
		"sheetId":      sheetID,
		"rangeAddress": rangeAddress,
		"resourceId":   resourceID,
		"resourceUrl":  resourceURL,
	}
	if w, _ := cmd.Flags().GetInt("width"); cmd.Flags().Changed("width") {
		writeArgs["width"] = w
	}
	if h, _ := cmd.Flags().GetInt("height"); cmd.Flags().Changed("height") {
		writeArgs["height"] = h
	}

	if jsonMode {
		// json 模式：write_image 的 JSON 响应就是唯一 stdout 输出。
		return callMCPTool("write_image", writeArgs)
	}
	if err := callMCPTool("write_image", writeArgs); err != nil {
		return err
	}
	deps.Out.PrintInfo(fmt.Sprintf("图片已写入表格: %s → %s (resourceId=%s)", fileName, rangeAddress, resourceID))
	return nil
}

// ── media-upload + write-image 命令定义 ──────────────────────────────────────

func newMediaCmds() []*cobra.Command {
	mediaUploadCmd := &cobra.Command{
		Use:   "media-upload",
		Short: "上传附件到表格",
		Long: `将本地文件作为附件上传到钉钉表格（两步自动完成）。

流程:
  1. 获取附件上传凭证 (get_doc_attachment_upload_info)
  2. HTTP PUT 上传文件到 OSS

上传成功后返回 resourceId，可用于后续引用。
--mime-type 可选，不指定时根据文件扩展名自动推断。`,
		Example: `  # 上传 PDF 附件
  dws sheet media-upload --node SHEET_DOC_ID --file ./report.pdf

  # 指定名称和 MIME 类型
  dws sheet media-upload --node SHEET_DOC_ID --file ./data.bin --name "数据文件.dat" --mime-type application/octet-stream`,
		RunE: runSheetMediaUpload,
	}
	mediaUploadCmd.Flags().String("node", "", "目标表格文档的标识，支持传入 URL 或 ID (必填)")
	mediaUploadCmd.Flags().String("file", "", "本地文件路径 (必填)")
	mediaUploadCmd.Flags().String("name", "", "附件显示名称 (默认使用文件名)")
	mediaUploadCmd.Flags().String("mime-type", "", "文件 MIME 类型 (默认根据扩展名推断)")
	mediaUploadCmd.Flags().String("url", "", "--node 的别名")
	mediaUploadCmd.Flags().String("id", "", "--node 的别名")
	mediaUploadCmd.Flags().String("node-id", "", "--node 的别名")
	mediaUploadCmd.Flags().String("doc-id", "", "--node 的别名")
	_ = mediaUploadCmd.Flags().MarkHidden("url")
	_ = mediaUploadCmd.Flags().MarkHidden("id")
	_ = mediaUploadCmd.Flags().MarkHidden("node-id")
	_ = mediaUploadCmd.Flags().MarkHidden("doc-id")

	writeImageCmd := &cobra.Command{
		Use:   "write-image",
		Short: "上传图片并写入表格单元格",
		Long: `将本地图片上传并写入到钉钉表格的指定单元格（三步自动完成）。

流程:
  1. 获取附件上传凭证 (get_doc_attachment_upload_info)
  2. HTTP PUT 上传图片到 OSS
  3. 调用 write_image 将图片写入指定单元格

--mime-type 可选，不指定时根据文件扩展名自动推断。
--width / --height 可选，设置图片在单元格中的显示尺寸。`,
		Example: `  # 写入图片到 A1:A1 单元格
  dws sheet write-image --node SHEET_DOC_ID --sheet-id SHEET_ID --range A1:A1 --file ./chart.png

  # 指定显示尺寸
  dws sheet write-image --node SHEET_DOC_ID --sheet-id SHEET_ID --range B2:B2 --file ./logo.png --width 200 --height 100`,
		RunE: runSheetWriteImage,
	}
	writeImageCmd.Flags().String("node", "", "目标表格文档的标识，支持传入 URL 或 ID (必填)")
	writeImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	writeImageCmd.Flags().String("range", "", "目标单元格区域地址，如 A1:B3 (必填)")
	writeImageCmd.Flags().String("file", "", "本地图片文件路径 (必填)")
	writeImageCmd.Flags().String("name", "", "图片显示名称 (默认使用文件名)")
	writeImageCmd.Flags().String("mime-type", "", "文件 MIME 类型 (默认根据扩展名推断)")
	writeImageCmd.Flags().Int("width", 0, "图片显示宽度 (可选)")
	writeImageCmd.Flags().Int("height", 0, "图片显示高度 (可选)")
	writeImageCmd.Flags().String("url", "", "--node 的别名")
	writeImageCmd.Flags().String("id", "", "--node 的别名")
	writeImageCmd.Flags().String("node-id", "", "--node 的别名")
	writeImageCmd.Flags().String("doc-id", "", "--node 的别名")
	_ = writeImageCmd.Flags().MarkHidden("url")
	_ = writeImageCmd.Flags().MarkHidden("id")
	_ = writeImageCmd.Flags().MarkHidden("node-id")
	_ = writeImageCmd.Flags().MarkHidden("doc-id")

	return []*cobra.Command{mediaUploadCmd, writeImageCmd}
}
