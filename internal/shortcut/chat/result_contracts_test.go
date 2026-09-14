// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageConversationDiscoveryResultAndNextActions(t *testing.T) {
	if _, err := contract.NormalizeResultSpec(
		ConversationList.Contract.Result,
		ConversationList.Contract.Identity.CanonicalPath,
	); err != nil {
		t.Fatalf("conversation-list Result contract: %v", err)
	}
	if ConversationList.OutputRollout != output.RolloutDualValidate || ConversationList.Contract.Pagination == nil {
		t.Fatalf("conversation-list rollout/pagination = %q / %#v", ConversationList.OutputRollout, ConversationList.Contract.Pagination)
	}

	actions := conversationDiscoveryNextActions(
		[]map[string]any{{"openConversationId": "cid-fixture-chat-0001"}},
		true,
		42,
		false,
		100,
		true,
	)
	if len(actions) != 2 || actions[0]["cliPath"] != "chat +chat-messages" ||
		actions[1]["cliPath"] != "chat +conversation-list" {
		t.Fatalf("next actions = %#v", actions)
	}
	if got := actions[1]["arguments"].(map[string]any)["cursor"]; got != int64(42) {
		t.Fatalf("continuation cursor = %#v", got)
	}
	if actions[1]["arguments"].(map[string]any)["exclude-muted"] != true {
		t.Fatalf("continuation lost exclude-muted: %#v", actions[1])
	}
	root := newPlatformCoverageRoot()
	for index, action := range actions[1:] {
		command, remaining, err := root.Find(strings.Fields(action["cliPath"].(string)))
		if err != nil || len(remaining) != 0 || command == nil || !command.Runnable() {
			t.Fatalf("continuation nextActions[%d] does not bind: command=%v remaining=%v err=%v", index, command, remaining, err)
		}
		for name := range action["arguments"].(map[string]any) {
			if command.Flags().Lookup(name) == nil && command.InheritedFlags().Lookup(name) == nil {
				t.Errorf("nextActions[%d] argument %q is not executable by %q", index, name, action["cliPath"])
			}
		}
	}
	if unsafe := conversationDiscoveryNextActions(nil, true, 0, true, 100, false); len(unsafe) != 0 {
		t.Fatalf("unsafe continuation published actions: %#v", unsafe)
	}
}

func TestCrossPlatformCoverageMessageResourceURLDeclaresSensitiveTemporaryCredentials(t *testing.T) {
	if MessagesResourceURL.OutputRollout != output.RolloutDualValidate {
		t.Fatalf("resource URL rollout = %q, want dual_validate", MessagesResourceURL.OutputRollout)
	}
	normalized, err := contract.NormalizeResultSpec(
		MessagesResourceURL.Contract.Result,
		MessagesResourceURL.Contract.Identity.CanonicalPath,
	)
	if err != nil {
		t.Fatalf("message resource URL Result contract: %v", err)
	}
	if normalized == nil || len(normalized.SensitivePaths) < 3 {
		t.Fatalf("temporary resource credentials are not protected: %#v", normalized)
	}
	if len(MessagesResourceURL.Contract.Selection.AvoidWhen) == 0 ||
		!strings.Contains(MessagesResourceURL.Contract.Selection.AvoidWhen[0], "+messages-resource-download") {
		t.Fatalf("resource URL selection does not route actual downloads safely: %#v", MessagesResourceURL.Contract.Selection)
	}
}
