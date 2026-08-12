// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// runDocMediaUpload 上传绑定到文档 nodeId 的可复用媒体资源，但不插入正文块。
// 白板 Vector/SVG 使用返回的 resourceId 与 resourceUrl 引用同一文档下的资源。
func runDocMediaUpload(cmd *cobra.Command, _ []string) error {
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
		fileName += filepath.Ext(filePath)
	}
	mimeType, _ := cmd.Flags().GetString("mime-type")
	if mimeType == "" {
		mimeType = inferMimeType(fileName)
	}

	if deps.Caller.DryRun() {
		return callMCPToolOnServer("doc", "get_doc_attachment_upload_info", map[string]any{
			"nodeId":   nodeID,
			"fileName": fileName,
			"fileSize": float64(fileInfo.Size()),
			"mimeType": mimeType,
		})
	}

	// 用户确认由 DeclareLeafMetadata(user_required) 的 ConfirmSafety 门控接管：
	// 推迟到首次 deps.Caller.CallTool（下方 get_doc_attachment_upload_info），
	// 避免与门控双读 stdin。--yes / --dry-run 经 confirmationBypass 跳过。
	text, err := callMCPToolReturnTextOnServer(cmd.Context(), "doc", "get_doc_attachment_upload_info", map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileInfo.Size()),
		"mimeType": mimeType,
	})
	if err != nil {
		return err
	}
	uploadURL, resourceID, resourceURL, err := parseAttachmentUploadInfo(text)
	if err != nil {
		return err
	}
	if resourceURL == "" {
		return fmt.Errorf("incomplete attachment upload info: missing resourceUrl")
	}
	if err := httpPutFile(cmd.Context(), uploadURL, map[string]string{"Content-Type": mimeType}, filePath, fileInfo.Size()); err != nil {
		message := strings.ReplaceAll(err.Error(), uploadURL, "<redacted upload URL>")
		return fmt.Errorf("document media upload failed: %s", message)
	}
	return deps.Out.PrintJSON(map[string]any{
		"nodeId":      nodeID,
		"resourceId":  resourceID,
		"resourceUrl": resourceURL,
		"fileName":    fileName,
		"mimeType":    mimeType,
		"size":        fileInfo.Size(),
	})
}
