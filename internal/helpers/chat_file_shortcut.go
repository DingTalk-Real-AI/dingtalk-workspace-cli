// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import "context"

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
