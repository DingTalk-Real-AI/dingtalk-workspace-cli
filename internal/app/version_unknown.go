// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"

// isUnknownInvocationError 只消费框架解析边界的标记。业务错误即使包含
// unknown command / unknown flag 文案，也不能触发未知调用的版本检查。
func isUnknownInvocationError(err error) bool {
	return apperrors.IsUnknownInvocationError(err)
}
