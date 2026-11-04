// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package helpers

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const defaultChatA2UISummary = "互动卡片"

// NormalizeChatA2UISummary validates the fallback summary supplied at creation
// through the named flag. The server shows it where a client cannot render the
// card; finished cards take their conversation summary from annotated
// artifacts. Omitted values use a compatibility default; explicit blank values
// are invalid.
func NormalizeChatA2UISummary(flag, value string, supplied bool) (string, error) {
	if strings.TrimSpace(value) == "" {
		if supplied {
			return "", apperrors.NewValidation("--" + flag + " must not be blank")
		}
		return defaultChatA2UISummary, nil
	}
	return value, nil
}
