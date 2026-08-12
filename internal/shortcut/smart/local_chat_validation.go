// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func defaultChatPageLimit(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func validChatTime(value string) bool {
	_, err := parseDingTalkMessageTime(strings.TrimSpace(value))
	return err == nil
}

func localChatOptionError(reason, message string, flags ...string) error {
	action := "修正参数后重试，或查看当前命令帮助"
	if flagText := strings.Join(flags, "、"); flagText != "" {
		action = "检查 " + flagText + " 后重试，或查看当前命令帮助"
	}
	return apperrors.NewValidation(
		message,
		apperrors.WithReason(reason),
		apperrors.WithActions(action),
	)
}
