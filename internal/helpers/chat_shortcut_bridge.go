// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
package helpers

import (
	"github.com/spf13/cobra"
	"strings"
)

// ValidateChatQuoteReply keeps shortcut Bot quote replies behind the same
// topic-container checks as the original native send-by-bot command.
func ValidateChatQuoteReply(cmd *cobra.Command, conversationID, messageID string) error {
	return guardTopicQuoteReply(cmd, conversationID, messageID)
}

// ParseChatA2UIMessages uses the same payload parser as the native card leaf.
func ParseChatA2UIMessages(raw string) ([]string, error) { return parseA2UIMessages(raw) }

// PrepareChatReplyMentions reuses the native reply's normalization and missing
// placeholder insertion before adapting the resulting body to the sender.
func PrepareChatReplyMentions(text string, ids []string, atAll, user bool) string {
	text = NormalizeMessageMentions(text, ids, atAll, true)
	text = addMissingCurrentUserMentionPlaceholders(text, strings.Join(ids, ","))
	if !user {
		text = NormalizeMessageMentions(text, ids, atAll, false)
	}
	return text
}
