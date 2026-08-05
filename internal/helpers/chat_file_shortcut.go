// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// ConversationLocalFileMeta exposes the already-reviewed native chat upload
// metadata to built-in semantic Shortcuts. It remains an alias so the native
// Cobra leaf and Shortcut use exactly the same upload implementation.
type ConversationLocalFileMeta = conversationLocalFileMeta

// BuildConversationLocalFileMeta validates a local file and computes the
// metadata required by DingTalk's conversation-file upload flow.
func BuildConversationLocalFileMeta(filePath, fileName, md5Value string) (ConversationLocalFileMeta, error) {
	return buildConversationLocalFileMeta(filePath, fileName, md5Value)
}

// UploadConversationLocalFile executes the existing init -> HTTP upload ->
// commit flow and returns the commit response for message-content assembly.
func UploadConversationLocalFile(
	ctx context.Context,
	targetArgs map[string]any,
	meta ConversationLocalFileMeta,
	uuid string,
) (string, error) {
	return uploadConversationLocalFile(ctx, targetArgs, meta, uuid)
}

// ParseConversationFileSendIDs extracts the committed dentry and space IDs.
func ParseConversationFileSendIDs(text string) (int64, int64, error) {
	return parseConversationFileSendIDs(text)
}

// BuildConversationFileContent renders the exact file-message content accepted
// by send_personal_message.
func BuildConversationFileContent(
	dentryID, spaceID int64,
	meta ConversationLocalFileMeta,
) (string, error) {
	return buildConversationFileContent(dentryID, spaceID, meta)
}

// ValidateChatMediaID rejects values that are deterministically not an
// uploaded DingTalk mediaId before an MCP request is dispatched.
func ValidateChatMediaID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("mediaId 不能为空")
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "file://") || filepath.IsAbs(value) ||
		strings.ContainsAny(value, `/\\`) || strings.HasPrefix(lower, "dentry") {
		return fmt.Errorf("%q 是本地文件路径或文件标识，不是 mediaId；请使用可信上游返回的 mediaId，DWS CLI 当前不能把本地图片转换为群头像 mediaId", value)
	}
	if value[0] != '@' && value[0] != '$' {
		return fmt.Errorf("%q 不是有效 mediaId：群头像 mediaId 应以 @ 或 $ 开头；不要传本地路径、dentryId 或 uploadKey", value)
	}
	return nil
}
