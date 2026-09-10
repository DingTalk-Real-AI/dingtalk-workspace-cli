// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"os"
	"strings"
)

func validateDocCreateMedia(rt *shortcut.RuntimeContext) ([]string, error) {
	files := rt.StrSlice("media-files")
	if len(files) > 20 {
		return nil, apperrors.NewValidation("创建时最多追加20个媒体文件")
	}
	seen := map[string]bool{}
	for i, file := range files {
		file = strings.TrimSpace(file)
		if seen[file] {
			return nil, apperrors.NewValidation("media-files不能重复")
		}
		if err := validateWorkspaceInputPath("media-files", file); err != nil {
			return nil, err
		}
		st, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if !st.Mode().IsRegular() || st.Size() <= 0 {
			return nil, apperrors.NewValidation("媒体文件必须是非空普通文件")
		}
		seen[file], files[i] = true, file
	}
	return files, nil
}

func insertCreatedDocMedia(rt *shortcut.RuntimeContext, node string, files []string) ([]any, error) {
	receipts := []any{}
	for _, file := range files {
		err := helpers.InsertDocMediaFile(rt.Command(), node, file, func(result any) error { receipts = append(receipts, result); return nil })
		if err != nil {
			return nil, docPartialWriteError("doc.create", "doc_create_media_partial", "append_media", "文档和正文已创建，但媒体追加未全部完成；不要重新创建文档", err, map[string]any{"nodeId": node, "completedMedia": receipts, "failedFile": file}, nil, map[string]any{"available": true, "action": "inspect_then_append_remaining", "nodeId": node})
		}
	}
	return receipts, nil
}
