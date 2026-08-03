// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package smart

import (
	"strings"
	"time"
	"unicode"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func looksLikeOpenConversationID(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "cid") {
		return false
	}
	return len(value) >= 24 || strings.ContainsAny(value, "/+=")
}

func looksLikeHumanGroupName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || looksLikeOpenConversationID(value) || strings.EqualFold(value, "cid") {
		return false
	}
	for _, r := range value {
		if unicode.Is(unicode.Han, r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func validChatTime(value string) bool {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func changedConversationIDFlag(rt *shortcut.RuntimeContext) string {
	for _, name := range []string{"group", "conversation-id", "id", "chat-id", "open-conversation-id"} {
		if rt.Changed(name) {
			return "--" + name
		}
	}
	return "会话 ID 参数"
}

func localChatOptionError(reason, message string, flags ...string) error {
	flagText := strings.Join(flags, "、")
	action := "修正参数后重试，或查看当前命令帮助"
	if flagText != "" {
		action = "检查 " + flagText + " 后重试，或查看当前命令帮助"
	}
	return apperrors.NewValidation(
		message,
		apperrors.WithReason(reason),
		apperrors.WithActions(action),
		apperrors.WithExamples("dws chat --help"),
	)
}
