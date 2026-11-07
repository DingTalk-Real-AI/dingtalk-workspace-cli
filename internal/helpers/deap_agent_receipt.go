// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"errors"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

var errEmployeeReceiptNotVisible = errors.New("digital employee receipt is not visible yet")

// 该分类只用于已有发送任务的回执查询，不能据此重发或泛化重试其他接口。
func isEmployeeReceiptNotVisible(err error) bool {
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed == nil {
		return false
	}
	// IM 回执尚未落库时可能只返回 1001；仅按结构化错误码识别，不解析自由文本。
	if typed.ServerDiag.ServerErrorCode == "1001" {
		return true
	}
	if typed.ServerDiag.ServerErrorCode != "PARAM_ERROR" {
		return false
	}
	detail := strings.TrimSpace(typed.ServerDiag.TechnicalDetail)
	if detail == "" {
		// 业务 errorMsg 由 Runner 保留在 Message；不解析原始 JSON 或模糊匹配。
		detail = strings.TrimSpace(typed.Message)
	}
	return detail == "消息不存在"
}
