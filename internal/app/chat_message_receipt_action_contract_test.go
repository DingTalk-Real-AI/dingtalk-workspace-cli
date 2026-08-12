// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

func TestCrossPlatformCoverageChatMessageReceiptActionsBindToRunnableCommands(t *testing.T) {
	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "send receipt awaiting status",
			payload: chatmsg.ProjectMessageSendReceipt(map[string]any{
				"openTaskId": "task-pending",
			}),
		},
		{
			name: "send receipt ready for message actions",
			payload: chatmsg.ProjectMessageSendReceipt(map[string]any{
				"openTaskId":         "task-ready",
				"openMessageId":      "message-ready",
				"openConversationId": "conversation-ready",
			}),
		},
		{
			name: "send status awaiting message reference",
			payload: chatmsg.ProjectMessageSendStatus(map[string]any{
				"status": "PENDING",
			}, "task-pending"),
		},
		{
			name: "send status ready for message actions",
			payload: chatmsg.ProjectMessageSendStatus(map[string]any{
				"openTaskId":         "task-ready",
				"openMessageId":      "message-ready",
				"openConversationId": "conversation-ready",
				"status":             "SUCCESS",
			}, "task-ready"),
		},
	}

	root := NewRootCommand()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actions, ok := testCase.payload["nextActions"].([]map[string]any)
			if !ok || len(actions) == 0 {
				t.Fatalf("nextActions = %#v, want non-empty []map[string]any", testCase.payload["nextActions"])
			}

			for index, action := range actions {
				cliPath, ok := action["cliPath"].(string)
				if !ok || strings.TrimSpace(cliPath) == "" {
					t.Fatalf("nextActions[%d].cliPath = %#v, want non-empty string", index, action["cliPath"])
				}

				command, remaining, err := root.Find(strings.Fields(cliPath))
				if err != nil {
					t.Fatalf("nextActions[%d].cliPath %q does not bind: %v", index, cliPath, err)
				}
				if command == nil || len(remaining) != 0 || !command.Runnable() {
					t.Fatalf("nextActions[%d].cliPath %q resolved to command=%v remaining=%v runnable=%v", index, cliPath, command, remaining, command != nil && command.Runnable())
				}

				arguments, ok := action["arguments"].(map[string]any)
				if !ok {
					t.Fatalf("nextActions[%d].arguments = %#v, want map[string]any", index, action["arguments"])
				}
				for name := range arguments {
					if command.Flags().Lookup(name) == nil && command.InheritedFlags().Lookup(name) == nil {
						t.Errorf("nextActions[%d] argument %q is not a flag of runnable command %q", index, name, cliPath)
					}
				}
			}
		})
	}
}
