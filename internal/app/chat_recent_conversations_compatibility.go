// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

// Keep old Schema CLI-path lookup bound to the new public tool, without
// making the hidden compatibility leaf a second identity declaration.
func annotateChatRecentConversationsCompatibility(commands []*cobra.Command) {
	for _, product := range commands {
		if product.Name() != "chat" {
			continue
		}
		primary, primaryRest, primaryErr := product.Find([]string{"+recent-conversations"})
		legacy, legacyRest, legacyErr := product.Find([]string{"+active-conversations"})
		if primaryErr != nil || legacyErr != nil || len(primaryRest) != 0 || len(legacyRest) != 0 || primary == legacy || primary.Name() != "+recent-conversations" || legacy.Name() != "+active-conversations" || !primary.Runnable() || !legacy.Runnable() || primary.Hidden || !legacy.Hidden {
			panic("chat recent-conversations requires its exact hidden executable compatibility leaf")
		}
		cli.AnnotateRuntimeCompatibilityEquivalence(primary, legacy, cli.RuntimeCompatibilityEquivalence{
			ID:       "chat.recent-conversations.legacy-active",
			Reason:   "Approved command_move #1336: both entries share the same Shortcut flags, constraints, safety, validateActiveConversations and executeActiveConversations; only the command name, discovery visibility and Agent contract ownership differ.",
			Reviewed: true,
		})
		return
	}
}
