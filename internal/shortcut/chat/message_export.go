// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ValidateMessageExportOutput applies the same workspace-relative and
// no-symlink boundary used by message resource downloads. The export target is
// always a file; directory-shaped paths are rejected instead of inventing a
// name.
func ValidateMessageExportOutput(output string) error {
	trimmed := strings.TrimSpace(output)
	if strings.HasSuffix(trimmed, "/") ||
		strings.HasSuffix(trimmed, string(os.PathSeparator)) {
		return apperrors.NewValidation("--output 必须是 JSON 文件路径，不能是目录")
	}
	if !strings.EqualFold(filepath.Ext(trimmed), ".json") {
		return apperrors.NewValidation("--output 必须使用 .json 文件扩展名")
	}
	return validateResourceDownloadOutputFlag(output, "--output")
}

// WriteMessageExportJSON atomically publishes the exact structured message
// ledger. It defaults to no-clobber and never follows a symlink outside the
// current working directory.
func WriteMessageExportJSON(output string, overwrite bool, payload any) (relativePath string, size int, err error) {
	if err := ValidateMessageExportOutput(output); err != nil {
		return "", 0, err
	}
	cwd, err := resourceGetwd()
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("读取工作目录失败: %v", err))
	}
	base, err := resourceAbs(cwd)
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("解析工作目录失败: %v", err))
	}
	realBase, err := resourceEvalSymlinks(base)
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("解析工作目录失败: %v", err))
	}

	target := filepath.Join(realBase, filepath.Clean(strings.TrimSpace(output)))
	parent := filepath.Dir(target)
	if err := ensureResourceDownloadParent(realBase, parent); err != nil {
		return "", 0, err
	}
	realParent, err := resourceEvalSymlinks(parent)
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("解析输出目录失败: %v", err))
	}
	parentRel, err := resourceRel(realBase, realParent)
	if err != nil || parentRel == ".." || strings.HasPrefix(parentRel, ".."+string(os.PathSeparator)) {
		return "", 0, apperrors.NewValidation("--output 解析后逃逸工作目录")
	}
	target = filepath.Join(realParent, filepath.Base(target))
	if info, statErr := resourceLstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", 0, apperrors.NewValidation("--output 目标不能是符号链接")
		}
		if info.IsDir() {
			return "", 0, apperrors.NewValidation("--output 目标是目录，无法写入 JSON 文件")
		}
		if !overwrite {
			return "", 0, apperrors.NewValidation("目标文件已存在；如确认覆盖请显式传 --overwrite")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("检查输出文件失败: %v", statErr))
	}

	rendered, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("编码消息导出失败: %v", err))
	}
	rendered = append(rendered, '\n')
	temp, err := resourceCreateTemp(realParent, "."+filepath.Base(target)+".part-*")
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("创建消息导出临时文件失败: %v", err))
	}
	tempPath := temp.Name()
	defer func() {
		_ = resourceTempClose(temp)
		_ = os.Remove(tempPath)
	}()
	if _, err := temp.Write(rendered); err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("写入消息导出失败: %v", err))
	}
	if err := resourceTempSync(temp); err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("同步消息导出失败: %v", err))
	}
	if err := resourceTempClose(temp); err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("关闭消息导出失败: %v", err))
	}
	if overwrite {
		if err := resourceRename(tempPath, target); err != nil {
			return "", 0, apperrors.NewInternal(fmt.Sprintf("发布消息导出失败: %v", err))
		}
	} else if err := resourceLink(tempPath, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", 0, apperrors.NewValidation("目标文件已存在；如确认覆盖请显式传 --overwrite")
		}
		return "", 0, apperrors.NewInternal(fmt.Sprintf("发布消息导出失败: %v", err))
	}
	relativePath, err = resourceRel(realBase, target)
	if err != nil {
		return "", 0, apperrors.NewInternal(fmt.Sprintf("解析输出相对路径失败: %v", err))
	}
	return filepath.ToSlash(relativePath), len(rendered), nil
}
